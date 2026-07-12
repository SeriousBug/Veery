package metrics

import (
	"sort"
	"strings"

	"github.com/SeriousBug/Veery/internal/api"
)

// maxDisks bounds how many capacity gauges the dashboard shows, largest first.
const maxDisks = 12

// DevicePeak is the highwater read/write throughput for one device.
type DevicePeak struct {
	Read  uint64
	Write uint64
}

// mountLabel is a friendly name for a mount point, mirroring the frontend.
func mountLabel(mount string) string {
	if mount == "/" {
		return "Main disk"
	}
	seg := mount
	if i := strings.LastIndex(strings.TrimRight(mount, "/"), "/"); i >= 0 {
		seg = mount[i+1:]
	}
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return "Disk"
	}
	return strings.ToUpper(seg[:1]) + seg[1:] + " disk"
}

// BuildHostMetrics assembles the dashboard payload from a raw sample, keeping
// only the disks the visibility settings leave shown and attaching per-device
// throughput peaks.
func BuildHostMetrics(s Sample, overrides map[string]bool, peaks map[string]DevicePeak) api.HostMetrics {
	eff := Effective(DefaultVisibility(s.Mounts, s.Devices), overrides)

	out := api.HostMetrics{
		CPUPercent: s.CPUPercent,
		MemUsed:    s.MemUsed,
		MemTotal:   s.MemTotal,
	}

	for _, m := range s.Mounts {
		key := MountKey(m.Mount)
		if !eff[key] {
			continue
		}
		out.Disks = append(out.Disks, api.DiskUsage{
			Key:   key,
			Mount: m.Mount,
			Used:  m.Used,
			Total: m.Total,
		})
	}
	sort.Slice(out.Disks, func(i, j int) bool { return out.Disks[i].Total > out.Disks[j].Total })
	if len(out.Disks) > maxDisks {
		out.Disks = out.Disks[:maxDisks]
	}

	labels := deviceLabels(s.Mounts, s.Devices)
	for _, d := range s.Devices {
		key := DeviceKey(d.Device)
		if !eff[key] {
			continue
		}
		p := peaks[d.Device]
		out.DiskActivity = append(out.DiskActivity, api.DiskActivity{
			Key:                  key,
			Device:               deviceLabel(d.Device),
			Label:                labels[d.Device],
			ReadBytesPerSec:      d.ReadPerSec,
			WriteBytesPerSec:     d.WritePerSec,
			ReadPeakBytesPerSec:  p.Read,
			WritePeakBytesPerSec: p.Write,
		})
	}
	return out
}

// UpdatePeaks raises the stored highwater marks to cover any new maxima in the
// sample, returning whether anything changed (and so needs persisting).
func UpdatePeaks(peaks map[string]DevicePeak, devices []DeviceIO) bool {
	changed := false
	for _, d := range devices {
		p := peaks[d.Device]
		if d.ReadPerSec > p.Read {
			p.Read = d.ReadPerSec
			changed = true
		}
		if d.WritePerSec > p.Write {
			p.Write = d.WritePerSec
			changed = true
		}
		peaks[d.Device] = p
	}
	return changed
}

// BuildDiskItems lists every configurable disk with its effective and default
// shown state, for the settings UI.
func BuildDiskItems(mounts []MountUsage, devices []DeviceIO, overrides map[string]bool) []api.DiskItem {
	defaults := DefaultVisibility(mounts, devices)
	eff := Effective(defaults, overrides)

	var items []api.DiskItem
	for _, m := range mounts {
		key := MountKey(m.Mount)
		detail := m.Mount
		if m.Fstype != "" {
			detail = m.Mount + " · " + m.Fstype
		}
		items = append(items, api.DiskItem{
			Key:          key,
			Kind:         api.DiskCapacity,
			Label:        mountLabel(m.Mount),
			Detail:       detail,
			Shown:        eff[key],
			DefaultShown: defaults[key],
		})
	}
	labels := deviceLabels(mounts, devices)
	for _, d := range devices {
		key := DeviceKey(d.Device)
		detail := "Read / write activity"
		if v := labels[d.Device]; v != "" {
			detail = v
		}
		items = append(items, api.DiskItem{
			Key:          key,
			Kind:         api.DiskActivityKind,
			Label:        deviceLabel(d.Device),
			Detail:       detail,
			Shown:        eff[key],
			DefaultShown: defaults[key],
		})
	}
	return items
}
