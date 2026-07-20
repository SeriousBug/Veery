// Package raidwatch turns the stateless mdadm reader into a stateful watcher:
// it polls RAID health, notifies on transitions (a scan starting or finishing,
// an array going unhealthy, a disk dropping), records when scrubs finish so the
// UI can show a last-checked time, and fires scheduled scrubs on an RRULE.
package raidwatch

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/metrics"
	"github.com/SeriousBug/Veery/internal/store"
)

// Notifier delivers events to the user's notification channels. A nil notifier
// disables notifications, matching the docker manager's optional notifier.
type Notifier interface {
	Notify(ev api.NotificationEvent, title, body string, meta ...api.EventMeta)
}

// Watcher polls mdadm health and drives alerts, last-scan tracking and the
// scheduler. scan and start are injectable so tests can drive it without a real
// /proc and /sys.
type Watcher struct {
	st    *store.Store
	notif Notifier
	scan  func() []api.MdArray
	start func(name string) error
}

// New builds a Watcher backed by the real mdadm reader.
func New(st *store.Store, notif Notifier) *Watcher {
	return &Watcher{
		st:    st,
		notif: notif,
		scan:  metrics.ScanMdadm,
		start: metrics.StartMdadmCheck,
	}
}

// Poller runs the watcher until ctx is cancelled, on the metrics poll interval
// so scan-start alerts are as fresh as the dashboard. It does an immediate pass
// first, like UpdateCheckPoller.
func (w *Watcher) Poller(ctx context.Context) {
	w.sweep(time.Now())
	for {
		t := time.NewTimer(w.interval())
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			w.sweep(time.Now())
		}
	}
}

// interval reuses the configured metrics poll interval, defaulting to 5s.
func (w *Watcher) interval() time.Duration {
	secs := store.DefaultPollIntervalSeconds
	if cfg, err := w.st.LoadSettings(); err == nil && cfg.PollIntervalSeconds > 0 {
		secs = cfg.PollIntervalSeconds
	}
	return time.Duration(secs) * time.Second
}

// sweep is one pass: read health, notify on transitions, run due schedules.
func (w *Watcher) sweep(now time.Time) {
	arrays := w.scan()
	// A nil scan is the feature being off or a transient mdstat read miss;
	// either way, don't touch the persisted baseline or we'd re-seed and miss
	// the next real transition.
	if len(arrays) == 0 {
		return
	}
	w.checkTransitions(arrays, now)
	w.runSchedules(arrays, now)
}

func (w *Watcher) notify(ev api.NotificationEvent, title, body string) {
	if w.notif != nil {
		w.notif.Notify(ev, title, body)
	}
}

// checkTransitions compares each array against the last sweep and notifies on
// the changes worth a message. An array not yet in the baseline is newly seen
// (first sweep ever, or an array just added), so it is only recorded — never
// announced — matching how noteStatuses treats a newly seen container.
func (w *Watcher) checkTransitions(arrays []api.MdArray, now time.Time) {
	baseline, err := w.st.LoadMdadmBaseline()
	if err != nil {
		log.Printf("raidwatch: load baseline: %v", err)
		return
	}
	lastScan, err := w.st.LoadMdadmLastScan()
	if err != nil {
		log.Printf("raidwatch: load last scan: %v", err)
		return
	}

	next := make(map[string]store.MdArrayBaseline, len(arrays))
	scanChanged := false
	for _, a := range arrays {
		cur := snapshot(a)
		prev, known := baseline[a.Name]
		if known {
			if w.notifyScan(a, prev) {
				lastScan[a.Name] = now.Unix()
				scanChanged = true
			}
			w.notifyHealth(a, prev)
			w.notifyDisks(a, prev)
		}
		next[a.Name] = cur
	}

	if err := w.st.SaveMdadmBaseline(next); err != nil {
		log.Printf("raidwatch: save baseline: %v", err)
	}
	if scanChanged {
		if err := w.st.SaveMdadmLastScan(lastScan); err != nil {
			log.Printf("raidwatch: save last scan: %v", err)
		}
	}
}

// notifyScan handles scrub start/finish. It returns true when a scrub just
// finished, so the caller records the completion time. Because it is driven by
// the sync_action transition, it fires whoever started the scrub.
func (w *Watcher) notifyScan(a api.MdArray, prev store.MdArrayBaseline) (finished bool) {
	const check = api.MdSyncAction("check")
	was, now := prev.SyncAction == check, a.SyncAction == check
	switch {
	case !was && now:
		w.notify(api.EventRaidScanStarted, a.Name+" scan started",
			"A data-scrub (check) started on "+a.Name+" ("+a.Level+"). This can take hours and adds disk load.")
	case was && !now:
		w.notify(api.EventRaidScanFinished, a.Name+" scan finished",
			fmt.Sprintf("The data-scrub on %s finished. Mismatch count: %d.", a.Name, a.MismatchCnt))
		return true
	}
	return false
}

// notifyHealth alerts when an array crosses into or out of a bad state. A scrub
// (recovering) is not a bad state, so a scan starting or finishing does not fire
// a health alert.
func (w *Watcher) notifyHealth(a api.MdArray, prev store.MdArrayBaseline) {
	wasBad, isBad := badState(prev.State), badState(a.State)
	switch {
	case !wasBad && isBad:
		w.notify(api.EventRaidUnhealthy, a.Name+" is "+string(a.State),
			fmt.Sprintf("%s (%s) is %s: %d of %d member disks are up.", a.Name, a.Level, a.State, a.DevicesUp, a.DevicesTotal))
	case wasBad && !isBad:
		w.notify(api.EventRaidUnhealthy, a.Name+" recovered",
			a.Name+" is healthy again.")
	}
}

// notifyDisks alerts on member disks dropping out or coming back. Only devices
// present in the previous snapshot are compared, so a member appearing for the
// first time is not announced.
func (w *Watcher) notifyDisks(a api.MdArray, prev store.MdArrayBaseline) {
	for _, m := range a.Members {
		wasUp, known := prev.Members[m.Device]
		switch {
		case known && wasUp && !m.Up:
			w.notify(api.EventRaidDiskOffline, m.Device+" dropped from "+a.Name,
				"Member disk "+m.Device+" of "+a.Name+" is no longer up. The array is running without it.")
		case known && !wasUp && m.Up:
			w.notify(api.EventRaidDiskOffline, m.Device+" rejoined "+a.Name,
				"Member disk "+m.Device+" of "+a.Name+" is back up.")
		}
	}
}

// runSchedules fires a scrub on any array whose schedule has an occurrence due
// since it last ran and that is currently idle.
func (w *Watcher) runSchedules(arrays []api.MdArray, now time.Time) {
	cfg, err := w.st.LoadMdadmSchedules()
	if err != nil {
		log.Printf("raidwatch: load schedules: %v", err)
		return
	}
	if len(cfg.Schedules) == 0 {
		return
	}
	lastRun, err := w.st.LoadMdadmLastRun()
	if err != nil {
		log.Printf("raidwatch: load last run: %v", err)
		return
	}

	changed := false
	for _, a := range arrays {
		sc, ok := cfg.Schedules[a.Name]
		if !ok || !sc.Enabled || sc.RRule == "" {
			continue
		}
		since, seeded := lastRun[a.Name]
		if !seeded {
			// First time we see this schedule: anchor "last run" to now so a
			// new schedule doesn't fire for occurrences already in the past.
			lastRun[a.Name] = now.Unix()
			changed = true
			continue
		}
		// idle covers "no scan running"; a blank action is the pre-enrichment
		// default and also means idle.
		if a.SyncAction != "idle" && a.SyncAction != "" {
			continue
		}
		due, err := dueSince(sc.RRule, time.Unix(since, 0), now)
		if err != nil {
			log.Printf("raidwatch: bad schedule for %s (%q): %v", a.Name, sc.RRule, err)
			continue
		}
		if !due {
			continue
		}
		if err := w.start(a.Name); err != nil {
			log.Printf("raidwatch: scheduled scan on %s: %v", a.Name, err)
			continue
		}
		log.Printf("raidwatch: started scheduled scrub on %s", a.Name)
		lastRun[a.Name] = now.Unix()
		changed = true
	}

	if changed {
		if err := w.st.SaveMdadmLastRun(lastRun); err != nil {
			log.Printf("raidwatch: save last run: %v", err)
		}
	}
}

func snapshot(a api.MdArray) store.MdArrayBaseline {
	members := make(map[string]bool, len(a.Members))
	for _, m := range a.Members {
		members[m.Device] = m.Up
	}
	return store.MdArrayBaseline{State: a.State, SyncAction: a.SyncAction, Members: members}
}

func badState(s api.MdArrayState) bool {
	return s == api.MdDegraded || s == api.MdFailed
}
