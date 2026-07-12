package metrics

import "testing"

func TestWholeDiskOf(t *testing.T) {
	cases := map[string]string{
		"/dev/sda2": "sda", "/dev/sda": "sda", "/dev/nvme0n1p1": "nvme0n1",
		"/dev/nvme0n1": "nvme0n1", "/dev/mmcblk0p2": "mmcblk0", "/dev/vdb3": "vdb",
		"/dev/disk3s1s1": "disk3s1s1", "OrbStack:/OrbStack": "OrbStack",
	}
	for in, want := range cases {
		if got := wholeDiskOf(in); got != want {
			t.Errorf("wholeDiskOf(%q)=%q want %q", in, got, want)
		}
	}
}

func TestDeviceLabels(t *testing.T) {
	mounts := []MountUsage{
		{Mount: "/", Device: "/dev/sda2"},
		{Mount: "/home", Device: "/dev/sda3"},
		{Mount: "/data", Device: "/dev/nvme0n1p1"},
	}
	devices := []DeviceIO{{Device: "sda"}, {Device: "nvme0n1"}, {Device: "sdb"}}
	got := deviceLabels(mounts, devices)
	if got["sda"] != "Main disk, Home disk" {
		t.Errorf("sda label = %q", got["sda"])
	}
	if got["nvme0n1"] != "Data disk" {
		t.Errorf("nvme0n1 label = %q", got["nvme0n1"])
	}
	if _, ok := got["sdb"]; ok {
		t.Errorf("sdb should have no label, got %q", got["sdb"])
	}
}
