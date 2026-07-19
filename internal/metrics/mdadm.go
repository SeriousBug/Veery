package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/SeriousBug/Veery/internal/api"
)

// hostProc / hostSys mirror the env vars gopsutil honours, so mdadm reads the
// host's /proc and /sys through the same mounts the rest of metrics needs.
func hostProc() string {
	if v := os.Getenv("HOST_PROC"); v != "" {
		return v
	}
	return "/proc"
}

func hostSys() string {
	if v := os.Getenv("HOST_SYS"); v != "" {
		return v
	}
	return "/sys"
}

// ScanMdadm reads Linux software RAID health from /proc/mdstat and sysfs. It is
// stateless, like Enumerate. Returns nil when mdstat is absent or lists no
// arrays, which is what disables the feature on hosts without RAID (or without
// /proc mounted in).
func ScanMdadm() []api.MdArray {
	data, err := os.ReadFile(filepath.Join(hostProc(), "mdstat"))
	if err != nil {
		return nil
	}
	raws := parseMdstat(data)
	if len(raws) == 0 {
		return nil
	}
	out := make([]api.MdArray, 0, len(raws))
	for _, r := range raws {
		a := api.MdArray{
			Name:         r.name,
			Level:        r.level,
			DevicesTotal: r.total,
			DevicesUp:    r.up,
			Members:      r.members,
			SyncAction:   normalizeAction(r.action),
			SyncPercent:  r.percent,
			SyncSpeedKBs: r.speedKBs,
			SyncFinish:   r.finish,
		}
		if !r.haveCount {
			a.DevicesTotal = len(r.members)
		}
		// mdstat drops the progress line when idle; sysfs still reports the
		// action ("idle" or a running one) and the last-check mismatch count.
		if action := readSysAttr(r.name, "sync_action"); action != "" && a.SyncAction == "" {
			a.SyncAction = normalizeAction(action)
		}
		if a.SyncAction == "" {
			a.SyncAction = api.MdSyncAction("idle")
		}
		if cnt := readSysAttr(r.name, "mismatch_cnt"); cnt != "" {
			if n, err := strconv.ParseUint(cnt, 10, 64); err == nil {
				a.MismatchCnt = n
			}
		}
		a.State = rollupState(r, a.SyncAction)
		out = append(out, a)
	}
	return out
}

// StartMdadmCheck kicks off a data-scrub (check) on an array by writing to its
// sysfs sync_action. The name is validated against the arrays mdstat actually
// reports, so a request can't be used to write to an arbitrary sysfs path.
func StartMdadmCheck(name string) error {
	found := false
	for _, a := range ScanMdadm() {
		if a.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no such array %q", name)
	}
	path := filepath.Join(hostSys(), "block", name, "md", "sync_action")
	if err := os.WriteFile(path, []byte("check\n"), 0); err != nil {
		return fmt.Errorf("start check on %s: %w (is /sys mounted writable?)", name, err)
	}
	return nil
}

// readSysAttr reads one sysfs md attribute, trimmed. Best effort: any error
// (attribute missing, /sys not mounted) yields "".
func readSysAttr(name, attr string) string {
	data, err := os.ReadFile(filepath.Join(hostSys(), "block", name, "md", attr))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// normalizeAction maps mdstat/sysfs action words to the API vocabulary. mdstat
// prints "recovery" where sysfs uses "recover"; everything else matches.
func normalizeAction(action string) api.MdSyncAction {
	switch action {
	case "":
		return ""
	case "recovery":
		return api.MdSyncAction("recover")
	default:
		return api.MdSyncAction(action)
	}
}

// rollupState derives the health badge: failed when the array is inactive,
// recovering while a sync/scrub runs, degraded when a member is down, else
// healthy.
func rollupState(r mdRaw, action api.MdSyncAction) api.MdArrayState {
	if r.state == "inactive" {
		return api.MdFailed
	}
	if action != "" && action != "idle" {
		return api.MdRecovering
	}
	if r.haveCount && r.up < r.total {
		return api.MdDegraded
	}
	for _, m := range r.members {
		if !m.Up {
			return api.MdDegraded
		}
	}
	return api.MdHealthy
}

// mdRaw is one array as parsed from mdstat, before sysfs enrichment and rollup.
type mdRaw struct {
	name      string
	level     string
	state     string
	members   []api.MdMember
	roles     []int // RAID role index per member, aligned with members
	total     int
	up        int
	haveCount bool
	action    string
	percent   float64
	speedKBs  uint64
	finish    string
}

var (
	headerRe   = regexp.MustCompile(`^(md\S+)\s*:\s*(.*)$`)
	countRe    = regexp.MustCompile(`\[(\d+)/(\d+)\]`)
	bitmapRe   = regexp.MustCompile(`\[([U_]+)\]`)
	progressRe = regexp.MustCompile(`(check|resync|recovery|reshape)\s*=\s*([0-9.]+)%.*?speed=(\d+)K/sec`)
	finishRe   = regexp.MustCompile(`finish=(\S+)`)
	memberRe   = regexp.MustCompile(`^(\S+?)\[(\d+)\](\([A-Z]+\))?$`)
)

// parseMdstat turns a /proc/mdstat dump into per-array raw records. Pure so it
// can be unit-tested against captured fixtures without a real /proc.
func parseMdstat(data []byte) []mdRaw {
	var out []mdRaw
	var cur *mdRaw
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(string(data), "\n") {
		if m := headerRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = parseHeader(m[1], m[2])
			continue
		}
		if cur == nil {
			continue
		}
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		if cm := countRe.FindStringSubmatch(line); cm != nil {
			cur.total, _ = strconv.Atoi(cm[1])
			cur.up, _ = strconv.Atoi(cm[2])
			cur.haveCount = true
			if bm := bitmapRe.FindStringSubmatch(line); bm != nil {
				applyBitmap(cur, bm[1])
			}
			continue
		}
		if pm := progressRe.FindStringSubmatch(line); pm != nil {
			cur.action = pm[1]
			cur.percent, _ = strconv.ParseFloat(pm[2], 64)
			speed, _ := strconv.ParseUint(pm[3], 10, 64)
			cur.speedKBs = speed
			if fm := finishRe.FindStringSubmatch(line); fm != nil {
				cur.finish = fm[1]
			}
			continue
		}
	}
	flush()
	return out
}

// parseHeader reads the array header remainder ("active raid1 sdb1[1] sda1[0]"
// or "inactive sdb1[1](S)") into name, state, level and members.
func parseHeader(name, rest string) *mdRaw {
	r := &mdRaw{name: name}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return r
	}
	r.state = fields[0]
	i := 1
	// The level token (raid1, raid5, ...) sits between the state and the member
	// list, but only when the array is assembled; inactive arrays omit it and go
	// straight to devices, which always carry a "[role]".
	if i < len(fields) && !strings.Contains(fields[i], "[") {
		r.level = fields[i]
		i++
	}
	for ; i < len(fields); i++ {
		if mm := memberRe.FindStringSubmatch(fields[i]); mm != nil {
			m := api.MdMember{Device: mm[1], Up: true}
			// (S) spare and (F) faulty are not active members.
			if mm[3] != "" {
				m.Up = false
			}
			role, _ := strconv.Atoi(mm[2])
			r.members = append(r.members, m)
			r.roles = append(r.roles, role)
		}
	}
	return r
}

// applyBitmap sets member up/down from the [UU]/[U_] field. Each member's role
// index (the number after its device name) selects its slot in the bitmap; the
// header order need not match slot order. Devices already flagged faulty/spare
// stay down.
func applyBitmap(r *mdRaw, bitmap string) {
	for i := range r.members {
		role := r.roles[i]
		if role >= 0 && role < len(bitmap) && bitmap[role] != 'U' {
			r.members[i].Up = false
		}
	}
}
