package metrics

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
)

// MountUsage is one mounted filesystem's raw capacity plus a best-effort guess
// at whether it's noise the dashboard should hide unless asked otherwise.
type MountUsage struct {
	Mount  string
	Fstype string
	Device string
	Used   uint64
	Total  uint64
	Noise  bool
}

// DeviceIO is one physical device's throughput. Rate fields are zero when the
// enumeration is stateless (no previous sample to diff against).
type DeviceIO struct {
	Device     string
	ReadPerSec uint64
	WritePerSec uint64
}

// pseudoFS are filesystem types that don't represent real storage. They're
// dropped from enumeration entirely rather than merely defaulted to hidden.
var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true,
	"devpts": true, "cgroup": true, "cgroup2": true, "mqueue": true,
	"debugfs": true, "tracefs": true, "securityfs": true, "pstore": true,
	"bpf": true, "nsfs": true, "fusectl": true, "configfs": true,
	"binfmt_misc": true, "hugetlbfs": true, "efivarfs": true, "devfs": true,
}

// networkOrVirtualFS are real mounts but not local storage; hidden by default.
var networkOrVirtualFS = map[string]bool{
	"nfs": true, "nfs4": true, "smbfs": true, "cifs": true, "afpfs": true,
	"webdav": true, "ftp": true, "nullfs": true, "overlay": true,
	"autofs": true, "squashfs": true, "ramfs": true, "9p": true,
}

// excludeMountPrefixes are OS-internal mounts dropped from enumeration entirely
// (never worth listing as a toggle): system volumes, backup snapshots, per-user
// temp dirs, and kernel mounts.
var excludeMountPrefixes = []string{
	"/System/Volumes",
	"/Volumes/.timemachine",
	"/private/var/folders",
	"/dev", "/proc", "/sys", "/run", "/boot",
}

// excludeMount reports mounts that should never appear anywhere, not even as a
// hidden toggle. Time Machine local snapshots alone produce dozens of these.
func excludeMount(mount string) bool {
	if strings.HasSuffix(mount, ".backup") ||
		strings.Contains(mount, "com.apple.TimeMachine") ||
		strings.Contains(mount, ".localsnapshots") {
		return true
	}
	for _, p := range excludeMountPrefixes {
		if mount == p || strings.HasPrefix(mount, p+"/") {
			return true
		}
	}
	return false
}

// isNoiseMount is the default-hidden heuristic for mounts that survive
// excludeMount but usually aren't wanted: network/virtual filesystems and
// external/removable volumes under /Volumes (backup drives, disk images, etc.).
func isNoiseMount(mount, fstype string) bool {
	if networkOrVirtualFS[fstype] {
		return true
	}
	if mount == "/Volumes" || strings.HasPrefix(mount, "/Volumes/") {
		return true
	}
	lower := strings.ToLower(mount)
	if strings.Contains(lower, "backup") || strings.Contains(lower, "time machine") {
		return true
	}
	return false
}

// enumerateMounts lists real mounted filesystems, deduped by device, each tagged
// with the default-hidden heuristic.
func enumerateMounts() []MountUsage {
	parts, err := disk.PartitionsWithContext(context.Background(), false)
	if err != nil {
		return nil
	}
	seenMount := map[string]bool{}
	seenDevice := map[string]bool{}
	var out []MountUsage
	for _, p := range parts {
		if pseudoFS[p.Fstype] || strings.HasPrefix(p.Fstype, "fuse.") {
			continue
		}
		if excludeMount(p.Mountpoint) {
			continue
		}
		if seenMount[p.Mountpoint] {
			continue
		}
		if p.Device != "" && seenDevice[p.Device] {
			continue
		}
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		seenMount[p.Mountpoint] = true
		if p.Device != "" {
			seenDevice[p.Device] = true
		}
		out = append(out, MountUsage{
			Mount:  p.Mountpoint,
			Fstype: p.Fstype,
			Device: p.Device,
			Used:   u.Used,
			Total:  u.Total,
			Noise:  isNoiseMount(p.Mountpoint, p.Fstype),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}

// partitionSuffix matches names that are a partition of a whole disk (sda1,
// nvme0n1p2, mmcblk0p1) so we can keep only whole physical devices.
var partitionSuffix = regexp.MustCompile(`^(?:sd[a-z]+|vd[a-z]+|hd[a-z]+|xvd[a-z]+)[0-9]+$|^(?:nvme[0-9]+n[0-9]+|mmcblk[0-9]+|loop[0-9]+)p[0-9]+$`)

func isWholeDisk(name string) bool {
	return !partitionSuffix.MatchString(name)
}

// listDeviceNames returns the whole physical devices that report I/O counters.
func listDeviceNames() []string {
	counters, err := disk.IOCounters()
	if err != nil {
		return nil
	}
	var names []string
	for name := range counters {
		if isWholeDisk(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// partitionToWhole reduces a partition name to its whole disk. Each pattern
// captures the whole-disk portion (sda1→sda, nvme0n1p2→nvme0n1, mmcblk0p1→mmcblk0).
var partitionToWhole = []*regexp.Regexp{
	regexp.MustCompile(`^([a-z]+vd[a-z]+|sd[a-z]+|vd[a-z]+|hd[a-z]+)[0-9]+$`),
	regexp.MustCompile(`^(nvme[0-9]+n[0-9]+)p[0-9]+$`),
	regexp.MustCompile(`^(mmcblk[0-9]+)p[0-9]+$`),
}

// wholeDiskOf reduces a mount's device path to the whole physical device name
// that IOCounters reports for it (/dev/sda2 → sda). Names it can't reduce (like
// macOS synthesized APFS devices) are returned as-is and simply won't match.
func wholeDiskOf(device string) string {
	name := path.Base(strings.TrimPrefix(device, "/dev/"))
	for _, re := range partitionToWhole {
		if m := re.FindStringSubmatch(name); m != nil {
			return m[1]
		}
	}
	return name
}

// deviceLabels ties each physical device to the volumes mounted on it, so the
// dashboard can show "Main disk" instead of a bare "sda". Devices that can't be
// tied to a mount are absent from the result (the UI falls back to the name).
func deviceLabels(mounts []MountUsage, devices []DeviceIO) map[string]string {
	byDevice := map[string][]string{}
	seen := map[string]bool{}
	for _, m := range mounts {
		if m.Device == "" {
			continue
		}
		whole := wholeDiskOf(m.Device)
		label := mountLabel(m.Mount)
		if k := whole + "\x00" + label; !seen[k] {
			seen[k] = true
			byDevice[whole] = append(byDevice[whole], label)
		}
	}
	out := map[string]string{}
	for _, d := range devices {
		if names := byDevice[d.Device]; len(names) > 0 {
			out[d.Device] = strings.Join(names, ", ")
		}
	}
	return out
}

// Visibility key helpers keep capacity mounts and activity devices in separate
// namespaces so they never collide in the overrides map.
func MountKey(mount string) string   { return "mount:" + mount }
func DeviceKey(device string) string { return "dev:" + device }

// DefaultVisibility computes the built-in shown/hidden state per disk key. Real
// (non-noise) mounts and all whole devices are shown; if every mount is noise,
// the largest one is unhidden so the dashboard never goes empty by default.
func DefaultVisibility(mounts []MountUsage, devices []DeviceIO) map[string]bool {
	def := map[string]bool{}
	anyMountShown := false
	for _, m := range mounts {
		shown := !m.Noise
		def[MountKey(m.Mount)] = shown
		anyMountShown = anyMountShown || shown
	}
	if !anyMountShown && len(mounts) > 0 {
		largest := mounts[0]
		for _, m := range mounts[1:] {
			if m.Total > largest.Total {
				largest = m
			}
		}
		def[MountKey(largest.Mount)] = true
	}
	for _, d := range devices {
		def[DeviceKey(d.Device)] = true
	}
	return def
}

// Effective merges user overrides onto the defaults.
func Effective(defaults, overrides map[string]bool) map[string]bool {
	eff := map[string]bool{}
	for k, v := range defaults {
		eff[k] = v
	}
	for k, v := range overrides {
		if _, known := eff[k]; known {
			eff[k] = v
		}
	}
	return eff
}

// deviceLabel is a friendly-ish name for a physical device.
func deviceLabel(name string) string {
	return path.Base(name)
}
