package raidwatch

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
)

type fakeNotifier struct {
	events []api.NotificationEvent
	titles []string
}

func (f *fakeNotifier) Notify(ev api.NotificationEvent, title, body string, meta ...api.EventMeta) {
	f.events = append(f.events, ev)
	f.titles = append(f.titles, title)
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// testWatcher returns a watcher whose scan returns whatever the returned setter
// was last given, and which records scrub starts instead of writing sysfs.
func testWatcher(t *testing.T, st *store.Store) (*Watcher, *fakeNotifier, func([]api.MdArray), *[]string) {
	t.Helper()
	f := &fakeNotifier{}
	var cur []api.MdArray
	var started []string
	w := &Watcher{
		st:    st,
		notif: f,
		scan:  func() []api.MdArray { return cur },
		start: func(name string) error { started = append(started, name); return nil },
	}
	return w, f, func(a []api.MdArray) { cur = a }, &started
}

func array(name string, state api.MdArrayState, action api.MdSyncAction, members ...api.MdMember) api.MdArray {
	up := 0
	for _, m := range members {
		if m.Up {
			up++
		}
	}
	return api.MdArray{
		Name: name, Level: "raid1", State: state, SyncAction: action,
		Members: members, DevicesTotal: len(members), DevicesUp: up,
	}
}

func member(dev string, up bool) api.MdMember { return api.MdMember{Device: dev, Up: up} }

func lastEvents(f *fakeNotifier) []api.NotificationEvent { return f.events }

func TestFirstSweepIsSilent(t *testing.T) {
	st := openTestStore(t)
	w, f, set, _ := testWatcher(t, st)

	set([]api.MdArray{array("md0", api.MdDegraded, "idle", member("sda1", true), member("sdb1", false))})
	w.sweep(time.Now())

	if len(f.events) != 0 {
		t.Fatalf("first sweep emitted %v, want nothing", f.events)
	}
}

func TestHealthAndDiskTransitions(t *testing.T) {
	st := openTestStore(t)
	w, f, set, _ := testWatcher(t, st)

	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true), member("sdb1", true))})
	w.sweep(time.Now())

	// A disk drops: array goes degraded and the member goes down.
	set([]api.MdArray{array("md0", api.MdDegraded, "idle", member("sda1", true), member("sdb1", false))})
	w.sweep(time.Now())
	assertHas(t, f, api.EventRaidUnhealthy)
	assertHas(t, f, api.EventRaidDiskOffline)

	// Recovery back to healthy.
	f.events = nil
	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true), member("sdb1", true))})
	w.sweep(time.Now())
	assertHas(t, f, api.EventRaidUnhealthy)   // recovered message
	assertHas(t, f, api.EventRaidDiskOffline) // rejoined message
}

func TestScanStartFinishAndLastScan(t *testing.T) {
	st := openTestStore(t)
	w, f, set, _ := testWatcher(t, st)

	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true))})
	w.sweep(time.Now())

	set([]api.MdArray{array("md0", api.MdRecovering, "check", member("sda1", true))})
	w.sweep(time.Now())
	assertHas(t, f, api.EventRaidScanStarted)

	f.events = nil
	finishAt := time.Now()
	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true))})
	w.sweep(finishAt)
	assertHas(t, f, api.EventRaidScanFinished)

	ls, err := st.LoadMdadmLastScan()
	if err != nil {
		t.Fatal(err)
	}
	if ls["md0"] != finishAt.Unix() {
		t.Fatalf("last scan = %d, want %d", ls["md0"], finishAt.Unix())
	}
	// A scrub finishing must not fire a health "recovered" alert.
	for _, ev := range lastEvents(f) {
		if ev == api.EventRaidUnhealthy {
			t.Fatalf("scrub finish wrongly fired %s", ev)
		}
	}
}

func TestScheduleSeedsThenFires(t *testing.T) {
	st := openTestStore(t)
	w, f, set, started := testWatcher(t, st)
	_ = f

	if err := st.SaveMdadmSchedules(api.MdadmScheduleConfig{Schedules: map[string]api.MdadmSchedule{
		"md0": {RRule: "FREQ=DAILY;BYHOUR=20;BYMINUTE=0", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}

	idle := []api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true))}

	// A Monday 19:00 local: first sweep seeds last-run, fires nothing.
	seed := time.Date(2024, 1, 1, 19, 0, 0, 0, time.Local)
	set(idle)
	w.sweep(seed)
	if len(*started) != 0 {
		t.Fatalf("seeding sweep started %v, want none", *started)
	}

	// 20:30 same day: the 20:00 occurrence is now in (seed, now], so it fires.
	set(idle)
	w.sweep(time.Date(2024, 1, 1, 20, 30, 0, 0, time.Local))
	if len(*started) != 1 || (*started)[0] != "md0" {
		t.Fatalf("started = %v, want [md0]", *started)
	}

	// 21:00: no new occurrence, no second fire.
	set(idle)
	w.sweep(time.Date(2024, 1, 1, 21, 0, 0, 0, time.Local))
	if len(*started) != 1 {
		t.Fatalf("started = %v, want still one fire", *started)
	}
}

func TestScheduleSkipsBusyArray(t *testing.T) {
	st := openTestStore(t)
	w, _, set, started := testWatcher(t, st)

	if err := st.SaveMdadmSchedules(api.MdadmScheduleConfig{Schedules: map[string]api.MdadmSchedule{
		"md0": {RRule: "FREQ=DAILY;BYHOUR=20;BYMINUTE=0", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	// Seed last-run in the past so an occurrence is due.
	if err := st.SaveMdadmLastRun(map[string]int64{"md0": time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local).Unix()}); err != nil {
		t.Fatal(err)
	}

	busy := []api.MdArray{array("md0", api.MdRecovering, "resync", member("sda1", true))}
	set(busy)
	w.sweep(time.Date(2024, 1, 1, 20, 30, 0, 0, time.Local))
	if len(*started) != 0 {
		t.Fatalf("started %v on a busy array, want none", *started)
	}
}

func TestDueSinceWindow(t *testing.T) {
	rule := "FREQ=WEEKLY;BYDAY=SU;BYHOUR=20;BYMINUTE=0"
	// 2024-01-07 is a Sunday.
	before := time.Date(2024, 1, 7, 19, 59, 0, 0, time.Local)
	after := time.Date(2024, 1, 7, 20, 1, 0, 0, time.Local)
	since := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local)

	if due, err := dueSince(rule, since, before); err != nil || due {
		t.Fatalf("before occurrence: due=%v err=%v, want false", due, err)
	}
	if due, err := dueSince(rule, since, after); err != nil || !due {
		t.Fatalf("after occurrence: due=%v err=%v, want true", due, err)
	}
}

func assertHas(t *testing.T, f *fakeNotifier, want api.NotificationEvent) {
	t.Helper()
	for _, ev := range f.events {
		if ev == want {
			return
		}
	}
	t.Fatalf("events %v missing %s", f.events, want)
}

func TestScheduledStartFailureRetriesThenGivesUp(t *testing.T) {
	st := openTestStore(t)
	w, f, set, _ := testWatcher(t, st)
	attempts := 0
	w.start = func(string) error { attempts++; return errors.New("permission denied") }

	if err := st.SaveMdadmSchedules(api.MdadmScheduleConfig{Schedules: map[string]api.MdadmSchedule{
		"md0": {RRule: "FREQ=DAILY;BYHOUR=20;BYMINUTE=0", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	seeded := time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local).Unix()
	if err := st.SaveMdadmLastRun(map[string]int64{"md0": seeded}); err != nil {
		t.Fatal(err)
	}
	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true))})

	now := time.Date(2024, 1, 1, 20, 30, 0, 0, time.Local)
	w.sweep(now)
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	assertHas(t, f, api.EventRaidScanFailed)

	// Polls before the retry is due must not try again.
	w.sweep(now.Add(30 * time.Second))
	if attempts != 1 {
		t.Fatalf("attempts = %d before retry was due, want 1", attempts)
	}

	for _, d := range retryDelays {
		now = now.Add(d)
		w.sweep(now)
	}
	if want := 1 + len(retryDelays); attempts != want {
		t.Fatalf("attempts = %d, want %d", attempts, want)
	}
	failures, err := st.LoadMdadmScanFailures()
	if err != nil {
		t.Fatal(err)
	}
	fail := failures["md0"]
	if fail.Attempts != attempts || fail.NextRetryAt != 0 || !fail.Scheduled {
		t.Fatalf("failure = %+v, want %d attempts and no further retry", fail, attempts)
	}
	// Only the first failure and the give-up are announced.
	if len(f.events) != 2 {
		t.Fatalf("events = %v, want two raid_scan_failed", f.events)
	}

	// Gave up: no more attempts until the next occurrence.
	w.sweep(now.Add(2 * time.Hour))
	if want := 1 + len(retryDelays); attempts != want {
		t.Fatalf("attempts = %d after giving up, want %d", attempts, want)
	}

	ls, err := st.LoadMdadmLastScan()
	if err != nil {
		t.Fatal(err)
	}
	if ls["md0"] != 0 {
		t.Fatalf("last scan = %d after only failed starts, want 0", ls["md0"])
	}
}

func TestScheduledRetrySuccessClearsFailure(t *testing.T) {
	st := openTestStore(t)
	w, _, set, _ := testWatcher(t, st)
	fail := true
	w.start = func(string) error {
		if fail {
			return errors.New("read-only file system")
		}
		return nil
	}

	if err := st.SaveMdadmSchedules(api.MdadmScheduleConfig{Schedules: map[string]api.MdadmSchedule{
		"md0": {RRule: "FREQ=DAILY;BYHOUR=20;BYMINUTE=0", Enabled: true},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveMdadmLastRun(map[string]int64{"md0": time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local).Unix()}); err != nil {
		t.Fatal(err)
	}
	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true))})

	now := time.Date(2024, 1, 1, 20, 30, 0, 0, time.Local)
	w.sweep(now)
	lastRun, _ := st.LoadMdadmLastRun()
	if lastRun["md0"] == now.Unix() {
		t.Fatal("last run advanced on a failed start")
	}

	fail = false
	w.sweep(now.Add(retryDelays[0]))
	failures, _ := st.LoadMdadmScanFailures()
	if _, ok := failures["md0"]; ok {
		t.Fatalf("failure still recorded after a successful retry: %+v", failures["md0"])
	}
	lastRun, _ = st.LoadMdadmLastRun()
	if lastRun["md0"] != now.Add(retryDelays[0]).Unix() {
		t.Fatalf("last run = %d, want the successful retry time", lastRun["md0"])
	}
}

func TestManualStartFailureIsRecordedAndNotRetried(t *testing.T) {
	st := openTestStore(t)
	w, f, set, _ := testWatcher(t, st)
	attempts := 0
	w.start = func(string) error { attempts++; return errors.New("permission denied") }
	set([]api.MdArray{array("md0", api.MdHealthy, "idle", member("sda1", true))})

	now := time.Now()
	if err := w.Start("md0", now); err == nil {
		t.Fatal("Start returned nil for a failed start")
	}
	assertHas(t, f, api.EventRaidScanFailed)
	failures, _ := st.LoadMdadmScanFailures()
	if got := failures["md0"]; got.Scheduled || got.NextRetryAt != 0 || got.Attempts != 1 {
		t.Fatalf("failure = %+v, want one unscheduled attempt", got)
	}

	w.sweep(now.Add(time.Hour))
	if attempts != 1 {
		t.Fatalf("manual failure was retried: %d attempts", attempts)
	}

	// Seeing a scrub run (e.g. started from the host) clears the warning.
	set([]api.MdArray{array("md0", api.MdRecovering, "check", member("sda1", true))})
	w.sweep(now.Add(2 * time.Hour))
	failures, _ = st.LoadMdadmScanFailures()
	if _, ok := failures["md0"]; ok {
		t.Fatal("failure still recorded while a scrub is running")
	}

	if err := w.Start("md9", now); err == nil {
		t.Fatal("Start accepted an unknown array")
	}
	failures, _ = st.LoadMdadmScanFailures()
	if _, ok := failures["md9"]; ok {
		t.Fatal("recorded a failure for an unknown array")
	}
}
