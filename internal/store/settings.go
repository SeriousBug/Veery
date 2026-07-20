package store

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/metrics"
)

// Default settings applied when a key is unset.
const (
	DefaultPollIntervalSeconds       = 5
	DefaultAutoUpdateDefault         = false
	DefaultAutoUpdateIntervalMinutes = 60
	DefaultEventLogRetentionDays     = 30
)

const (
	keyPollInterval       = "poll_interval_seconds"
	keyAutoUpdateDefault  = "auto_update_default"
	keyAutoUpdateInterval = "auto_update_interval_minutes"
	keyEventRetention     = "event_log_retention_days"
	keyDiskVisibility     = "disk_visibility"
	keyDiskPeaks          = "disk_io_peaks"
)

// LoadSettings reads app settings, applying defaults for unset keys.
func (s *Store) LoadSettings() (api.Settings, error) {
	out := api.Settings{
		PollIntervalSeconds:       DefaultPollIntervalSeconds,
		AutoUpdateDefault:         DefaultAutoUpdateDefault,
		AutoUpdateIntervalMinutes: DefaultAutoUpdateIntervalMinutes,
		EventLogRetentionDays:     DefaultEventLogRetentionDays,
		DiskVisibility:            map[string]bool{},
	}
	if v, err := s.getInt(keyPollInterval); err == nil {
		out.PollIntervalSeconds = v
	} else if !errors.Is(err, ErrNotFound) {
		return out, err
	}
	if v, err := s.GetSetting(keyAutoUpdateDefault); err == nil {
		out.AutoUpdateDefault = v == "1"
	} else if !errors.Is(err, ErrNotFound) {
		return out, err
	}
	if v, err := s.getInt(keyAutoUpdateInterval); err == nil {
		out.AutoUpdateIntervalMinutes = v
	} else if !errors.Is(err, ErrNotFound) {
		return out, err
	}
	if v, err := s.getInt(keyEventRetention); err == nil {
		out.EventLogRetentionDays = v
	} else if !errors.Is(err, ErrNotFound) {
		return out, err
	}
	vis, err := s.LoadDiskVisibility()
	if err != nil {
		return out, err
	}
	out.DiskVisibility = vis
	return out, nil
}

// SaveSettings persists app settings.
func (s *Store) SaveSettings(cfg api.Settings) error {
	if err := s.SetSetting(keyPollInterval, strconv.Itoa(cfg.PollIntervalSeconds)); err != nil {
		return err
	}
	if err := s.SetSetting(keyAutoUpdateInterval, strconv.Itoa(cfg.AutoUpdateIntervalMinutes)); err != nil {
		return err
	}
	if err := s.SetSetting(keyEventRetention, strconv.Itoa(cfg.EventLogRetentionDays)); err != nil {
		return err
	}
	v := "0"
	if cfg.AutoUpdateDefault {
		v = "1"
	}
	if err := s.SetSetting(keyAutoUpdateDefault, v); err != nil {
		return err
	}
	if cfg.DiskVisibility != nil {
		return s.SaveDiskVisibility(cfg.DiskVisibility)
	}
	return nil
}

// LoadDiskVisibility returns the per-disk shown/hidden overrides.
func (s *Store) LoadDiskVisibility() (map[string]bool, error) {
	out := map[string]bool{}
	v, err := s.GetSetting(keyDiskVisibility)
	if errors.Is(err, ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return map[string]bool{}, nil
	}
	return out, nil
}

// SaveDiskVisibility persists the per-disk shown/hidden overrides.
func (s *Store) SaveDiskVisibility(vis map[string]bool) error {
	b, err := json.Marshal(vis)
	if err != nil {
		return err
	}
	return s.SetSetting(keyDiskVisibility, string(b))
}

// LoadDiskPeaks returns the persisted per-device throughput highwater marks.
func (s *Store) LoadDiskPeaks() (map[string]metrics.DevicePeak, error) {
	out := map[string]metrics.DevicePeak{}
	v, err := s.GetSetting(keyDiskPeaks)
	if errors.Is(err, ErrNotFound) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return map[string]metrics.DevicePeak{}, nil
	}
	return out, nil
}

// SaveDiskPeaks persists the per-device throughput highwater marks.
func (s *Store) SaveDiskPeaks(peaks map[string]metrics.DevicePeak) error {
	b, err := json.Marshal(peaks)
	if err != nil {
		return err
	}
	return s.SetSetting(keyDiskPeaks, string(b))
}

func (s *Store) getInt(key string) (int, error) {
	v, err := s.GetSetting(key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, err
	}
	return n, nil
}
