package store

import "github.com/SeriousBug/Veery/internal/api"

const (
	keyMdadmSchedules = "mdadm_schedules"
	keyMdadmLastScan  = "mdadm_last_scan"
	keyMdadmLastRun   = "mdadm_last_run"
	keyMdadmBaseline  = "mdadm_notify_baseline"
)

// LoadMdadmSchedules reads the per-array scrub schedules. An unset config means
// no schedules, so nothing runs automatically until one is added.
func (s *Store) LoadMdadmSchedules() (api.MdadmScheduleConfig, error) {
	out := api.MdadmScheduleConfig{Schedules: map[string]api.MdadmSchedule{}}
	if err := s.getJSON(keyMdadmSchedules, &out.Schedules); err != nil {
		return out, err
	}
	if out.Schedules == nil {
		out.Schedules = map[string]api.MdadmSchedule{}
	}
	return out, nil
}

// SaveMdadmSchedules persists the per-array scrub schedules.
func (s *Store) SaveMdadmSchedules(cfg api.MdadmScheduleConfig) error {
	return s.setJSON(keyMdadmSchedules, cfg.Schedules)
}

// LoadMdadmLastScan returns, per array, the Unix time of the last data-scrub
// Veery saw finish. Veery tracks this itself because the kernel keeps no such
// timestamp.
func (s *Store) LoadMdadmLastScan() (map[string]int64, error) {
	out := map[string]int64{}
	err := s.getJSON(keyMdadmLastScan, &out)
	return out, err
}

// SaveMdadmLastScan records the last-scrub-finished times.
func (s *Store) SaveMdadmLastScan(m map[string]int64) error {
	return s.setJSON(keyMdadmLastScan, m)
}

// LoadMdadmLastRun returns, per array, the Unix time the scheduler last fired a
// scrub. It is the dedup marker that keeps one schedule occurrence from firing
// on every poll.
func (s *Store) LoadMdadmLastRun() (map[string]int64, error) {
	out := map[string]int64{}
	err := s.getJSON(keyMdadmLastRun, &out)
	return out, err
}

// SaveMdadmLastRun records when the scheduler last fired per array.
func (s *Store) SaveMdadmLastRun(m map[string]int64) error {
	return s.setJSON(keyMdadmLastRun, m)
}

// MdArrayBaseline is the last-seen health of one array, compared against the
// next sweep to decide which transitions to notify on.
type MdArrayBaseline struct {
	State      api.MdArrayState `json:"state"`
	SyncAction api.MdSyncAction `json:"syncAction"`
	Members    map[string]bool  `json:"members"` // device -> up
}

// LoadMdadmBaseline returns the arrays' health as of the last sweep. An empty
// map means no baseline yet, so the first sweep only records without notifying.
func (s *Store) LoadMdadmBaseline() (map[string]MdArrayBaseline, error) {
	out := map[string]MdArrayBaseline{}
	err := s.getJSON(keyMdadmBaseline, &out)
	return out, err
}

// SaveMdadmBaseline records the arrays' health as of this sweep.
func (s *Store) SaveMdadmBaseline(m map[string]MdArrayBaseline) error {
	return s.setJSON(keyMdadmBaseline, m)
}
