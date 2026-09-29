package metrics

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const healthyMdstat = `Personalities : [raid1] [raid6] [raid5] [raid4]
md0 : active raid1 sdb1[1] sda1[0]
      1953382464 blocks super 1.2 [2/2] [UU]

unused devices: <none>
`

const degradedMdstat = `Personalities : [raid1]
md0 : active raid1 sda1[0]
      1953382464 blocks super 1.2 [2/1] [U_]

unused devices: <none>
`

const checkingMdstat = `Personalities : [raid6] [raid5] [raid4]
md1 : active raid5 sdc1[3] sdd1[1] sde1[0]
      3906764800 blocks super 1.2 [3/3] [UUU]
      [====>................]  check = 12.6% (246804480/1953382464) finish=189.2min speed=149377K/sec

unused devices: <none>
`

func TestParseMdstatHealthy(t *testing.T) {
	raws := parseMdstat([]byte(healthyMdstat))
	if len(raws) != 1 {
		t.Fatalf("want 1 array, got %d", len(raws))
	}
	r := raws[0]
	if r.name != "md0" || r.level != "raid1" || r.state != "active" {
		t.Fatalf("bad header: %+v", r)
	}
	if r.total != 2 || r.up != 2 || !r.haveCount {
		t.Fatalf("bad count: total=%d up=%d have=%v", r.total, r.up, r.haveCount)
	}
	if len(r.members) != 2 {
		t.Fatalf("want 2 members, got %d", len(r.members))
	}
	for _, m := range r.members {
		if !m.Up {
			t.Fatalf("member %s should be up", m.Device)
		}
	}
	if got := rollupState(r, "idle"); got != "healthy" {
		t.Fatalf("state = %q, want healthy", got)
	}
}

func TestParseMdstatDegraded(t *testing.T) {
	raws := parseMdstat([]byte(degradedMdstat))
	if len(raws) != 1 {
		t.Fatalf("want 1 array, got %d", len(raws))
	}
	r := raws[0]
	if r.total != 2 || r.up != 1 {
		t.Fatalf("bad count: total=%d up=%d", r.total, r.up)
	}
	// Only sda1 (role 0) is listed and up; role 1 is missing.
	if len(r.members) != 1 || r.members[0].Device != "sda1" || !r.members[0].Up {
		t.Fatalf("bad members: %+v", r.members)
	}
	if got := rollupState(r, "idle"); got != "degraded" {
		t.Fatalf("state = %q, want degraded", got)
	}
}

func TestParseMdstatChecking(t *testing.T) {
	raws := parseMdstat([]byte(checkingMdstat))
	if len(raws) != 1 {
		t.Fatalf("want 1 array, got %d", len(raws))
	}
	r := raws[0]
	if r.name != "md1" || r.level != "raid5" {
		t.Fatalf("bad header: %+v", r)
	}
	if r.action != "check" {
		t.Fatalf("action = %q, want check", r.action)
	}
	if r.percent != 12.6 {
		t.Fatalf("percent = %v, want 12.6", r.percent)
	}
	if r.speedKBs != 149377 {
		t.Fatalf("speed = %d, want 149377", r.speedKBs)
	}
	if r.finish != "189.2min" {
		t.Fatalf("finish = %q, want 189.2min", r.finish)
	}
	if len(r.members) != 3 {
		t.Fatalf("want 3 members, got %d", len(r.members))
	}
	if got := rollupState(r, normalizeAction(r.action)); got != "recovering" {
		t.Fatalf("state = %q, want recovering", got)
	}
}

// raid6Mdstat is captured from a real 8-disk raid6 host: the count line carries
// extra text before the [n/m] field ("level 6, 512k chunk, algorithm 2"), a
// bitmap line follows, and member role indices don't match header order.
const raid6Mdstat = `Personalities : [raid4] [raid5] [raid6]
md127 : active raid6 sda1[6] sdg1[5] sde1[4] sdb1[0] sdc1[1] sdh1[2] sdf1[3] sdd1[7]
      23440416768 blocks super 1.2 level 6, 512k chunk, algorithm 2 [8/8] [UUUUUUUU]
      bitmap: 17/30 pages [68KB], 65536KB chunk

unused devices: <none>
`

func TestParseMdstatRaid6(t *testing.T) {
	raws := parseMdstat([]byte(raid6Mdstat))
	if len(raws) != 1 {
		t.Fatalf("want 1 array, got %d", len(raws))
	}
	r := raws[0]
	if r.name != "md127" || r.level != "raid6" || r.state != "active" {
		t.Fatalf("bad header: %+v", r)
	}
	if r.total != 8 || r.up != 8 || !r.haveCount {
		t.Fatalf("bad count: total=%d up=%d have=%v", r.total, r.up, r.haveCount)
	}
	if len(r.members) != 8 {
		t.Fatalf("want 8 members, got %d", len(r.members))
	}
	for _, m := range r.members {
		if !m.Up {
			t.Fatalf("member %s should be up", m.Device)
		}
	}
	// action is empty from mdstat (idle arrays drop the progress line); rollup
	// with the sysfs-reported "idle" must be healthy.
	if got := rollupState(r, "idle"); got != "healthy" {
		t.Fatalf("state = %q, want healthy", got)
	}
}

func TestParseMdstatEmpty(t *testing.T) {
	if raws := parseMdstat([]byte("Personalities : [raid1]\nunused devices: <none>\n")); len(raws) != 0 {
		t.Fatalf("want no arrays, got %d", len(raws))
	}
}

func fakeMdHost(t *testing.T, action string) string {
	t.Helper()
	root := t.TempDir()
	proc, md := filepath.Join(root, "proc"), filepath.Join(root, "sys", "block", "md0", "md")
	if err := os.MkdirAll(proc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(md, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "mdstat"), []byte(healthyMdstat), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(md, "sync_action")
	if err := os.WriteFile(path, []byte(action+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOST_PROC", proc)
	t.Setenv("HOST_SYS", filepath.Join(root, "sys"))
	return path
}

func TestStartMdadmCheckConfirmsAction(t *testing.T) {
	fakeMdHost(t, "idle")
	if err := StartMdadmCheck("md0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := StartMdadmCheck("../md0"); err == nil {
		t.Fatal("started a check on an array mdstat doesn't list")
	}
}

func TestStartMdadmCheckReportsUnwritableSysfs(t *testing.T) {
	path := fakeMdHost(t, "idle")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	err := StartMdadmCheck("md0")
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "SYS_ADMIN") {
		t.Fatalf("err = %v, want a permission error naming SYS_ADMIN", err)
	}
}

func TestExplainStartError(t *testing.T) {
	err := explainStartError("md0", &fs.PathError{Op: "open", Path: "sync_action", Err: syscall.EROFS})
	if !errors.Is(err, syscall.EROFS) || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("err = %v, want a read-only mount explanation", err)
	}
}
