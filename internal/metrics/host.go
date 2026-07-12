// Package metrics collects host-level resource usage for the dashboard. It
// uses gopsutil, which honours the HOST_PROC / HOST_SYS environment variables
// so it can read the host's stats from inside a container.
package metrics

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
)

// Collector holds the state needed to compute rate-based metrics (CPU percent
// and per-device disk I/O throughput) from deltas between successive Sample
// calls.
type Collector struct {
	lastCPUTotal float64
	lastCPUBusy  float64
	haveCPU      bool

	lastDev map[string]devSample
}

type devSample struct {
	read  uint64
	write uint64
	at    time.Time
}

// New builds a Collector.
func New() *Collector { return &Collector{lastDev: map[string]devSample{}} }

// Sample is a raw host reading before visibility filtering. CPU percent and
// device throughput are computed against the previous call, so the first call
// reports zero for those and later calls report real rates.
type Sample struct {
	CPUPercent float64
	MemUsed    uint64
	MemTotal   uint64
	Mounts     []MountUsage
	Devices    []DeviceIO
}

// Sample returns a fresh host reading.
func (c *Collector) Sample() Sample {
	var s Sample
	s.CPUPercent = c.cpuPercent()
	if vm, err := mem.VirtualMemory(); err == nil {
		s.MemTotal = vm.Total
		s.MemUsed = vm.Used
	}
	s.Mounts = enumerateMounts()
	s.Devices = c.deviceIO()
	return s
}

// Enumerate lists the mounts and devices without rate data. It's stateless, for
// building the "which disks to show" settings list.
func Enumerate() ([]MountUsage, []DeviceIO) {
	mounts := enumerateMounts()
	var devices []DeviceIO
	for _, name := range listDeviceNames() {
		devices = append(devices, DeviceIO{Device: name})
	}
	return mounts, devices
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

// deviceIO computes per-device throughput from cumulative byte counter deltas
// against the previous Sample call.
func (c *Collector) deviceIO() []DeviceIO {
	counters, err := disk.IOCounters()
	if err != nil {
		return nil
	}
	now := time.Now()
	var out []DeviceIO
	for _, name := range listDeviceNames() {
		ct, ok := counters[name]
		if !ok {
			continue
		}
		d := DeviceIO{Device: name}
		if prev, seen := c.lastDev[name]; seen {
			secs := now.Sub(prev.at).Seconds()
			if secs > 0 {
				if ct.ReadBytes >= prev.read {
					d.ReadPerSec = uint64(float64(ct.ReadBytes-prev.read) / secs)
				}
				if ct.WriteBytes >= prev.write {
					d.WritePerSec = uint64(float64(ct.WriteBytes-prev.write) / secs)
				}
			}
		}
		c.lastDev[name] = devSample{read: ct.ReadBytes, write: ct.WriteBytes, at: now}
		out = append(out, d)
	}
	return out
}
