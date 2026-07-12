// Package metrics collects host-level resource usage for the dashboard. It
// uses gopsutil, which honours the HOST_PROC / HOST_SYS environment variables
// so it can read the host's stats from inside a container.
package metrics

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Collector holds the state needed to compute rate-based metrics (CPU percent
// and disk I/O throughput) from deltas between successive Snapshot calls.
type Collector struct {
	lastCPUTotal float64
	lastCPUBusy  float64
	haveCPU      bool

	lastIORead  uint64
	lastIOWrite uint64
	lastIOAt    time.Time
	haveIO      bool
}

// New builds a Collector.
func New() *Collector { return &Collector{} }

// Snapshot returns a fresh host metrics reading. CPU percent and disk
// throughput are computed against the previous call, so the first call reports
// zero for those and later calls report real rates.
func (c *Collector) Snapshot() (api.HostMetrics, error) {
	var out api.HostMetrics

	out.CPUPercent = c.cpuPercent()

	if vm, err := mem.VirtualMemory(); err == nil {
		out.MemTotal = vm.Total
		out.MemUsed = vm.Used
	}

	out.Disks = diskUsage()

	rd, wr := c.diskThroughput()
	out.DiskReadBytesPerSec = rd
	out.DiskWriteBytesPerSec = wr

	return out, nil
}

// cpuPercent computes total CPU utilisation from cumulative CPU times deltas,
// avoiding the blocking sampling interval of cpu.Percent.
func (c *Collector) cpuPercent() float64 {
	times, err := cpu.Times(false)
	if err != nil || len(times) == 0 {
		return 0
	}
	t := times[0]
	busy := t.User + t.System + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	total := busy + t.Idle
	defer func() {
		c.lastCPUBusy = busy
		c.lastCPUTotal = total
		c.haveCPU = true
	}()
	if !c.haveCPU {
		return 0
	}
	totalDelta := total - c.lastCPUTotal
	busyDelta := busy - c.lastCPUBusy
	if totalDelta <= 0 {
		return 0
	}
	pct := (busyDelta / totalDelta) * 100.0
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

func (c *Collector) diskThroughput() (readPerSec, writePerSec uint64) {
	counters, err := disk.IOCounters()
	if err != nil {
		return 0, 0
	}
	var totalRead, totalWrite uint64
	for _, ct := range counters {
		totalRead += ct.ReadBytes
		totalWrite += ct.WriteBytes
	}
	now := time.Now()
	defer func() {
		c.lastIORead = totalRead
		c.lastIOWrite = totalWrite
		c.lastIOAt = now
		c.haveIO = true
	}()
	if !c.haveIO {
		return 0, 0
	}
	secs := now.Sub(c.lastIOAt).Seconds()
	if secs <= 0 {
		return 0, 0
	}
	if totalRead >= c.lastIORead {
		readPerSec = uint64(float64(totalRead-c.lastIORead) / secs)
	}
	if totalWrite >= c.lastIOWrite {
		writePerSec = uint64(float64(totalWrite-c.lastIOWrite) / secs)
	}
	return readPerSec, writePerSec
}

// pseudoFS are filesystem types that don't represent real storage.
var pseudoFS = map[string]bool{
	"proc": true, "sysfs": true, "tmpfs": true, "devtmpfs": true,
	"devpts": true, "cgroup": true, "cgroup2": true, "overlay": true,
	"mqueue": true, "debugfs": true, "tracefs": true, "securityfs": true,
	"pstore": true, "bpf": true, "autofs": true, "nsfs": true,
	"squashfs": true, "ramfs": true, "fusectl": true, "configfs": true,
	"binfmt_misc": true, "hugetlbfs": true, "efivarfs": true,
}

// noiseMountPrefixes are mount points that are real filesystems but not useful
// to surface as "storage" on the dashboard (OS-internal volumes, snapshots,
// per-user temp dirs). Time Machine local snapshots in particular mount dozens
// of these on macOS.
var noiseMountPrefixes = []string{
	"/System/Volumes",
	"/Volumes/.timemachine",
	"/private/var/folders",
	"/dev",
	"/proc", "/sys", "/run",
}

func skipMount(mountpoint string) bool {
	if strings.HasSuffix(mountpoint, ".backup") ||
		strings.Contains(mountpoint, "com.apple.TimeMachine") ||
		strings.Contains(mountpoint, ".localsnapshots") {
		return true
	}
	for _, p := range noiseMountPrefixes {
		if mountpoint == p || strings.HasPrefix(mountpoint, p+"/") {
			return true
		}
	}
	return false
}

// maxDisks bounds how many filesystems the dashboard shows, largest first.
const maxDisks = 12

func diskUsage() []api.DiskUsage {
	parts, err := disk.PartitionsWithContext(context.Background(), false)
	if err != nil {
		return nil
	}
	seenMount := map[string]bool{}
	seenDevice := map[string]bool{}
	var out []api.DiskUsage
	for _, p := range parts {
		if pseudoFS[p.Fstype] || strings.HasPrefix(p.Fstype, "fuse.") {
			continue
		}
		if skipMount(p.Mountpoint) || seenMount[p.Mountpoint] {
			continue
		}
		// Collapse multiple mounts backed by the same device (e.g. bind mounts,
		// docker volumes sharing the host root device).
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
		out = append(out, api.DiskUsage{
			Mount: p.Mountpoint,
			Used:  u.Used,
			Total: u.Total,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	if len(out) > maxDisks {
		out = out[:maxDisks]
	}
	return out
}
