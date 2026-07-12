package store

import (
	"errors"
	"strconv"

	"github.com/SeriousBug/Veery/internal/api"
)

// Default settings applied when a key is unset.
const (
	DefaultPollIntervalSeconds       = 5
	DefaultAutoUpdateDefault         = false
	DefaultAutoUpdateIntervalMinutes = 60
)

const (
	keyPollInterval       = "poll_interval_seconds"
	keyAutoUpdateDefault  = "auto_update_default"
	keyAutoUpdateInterval = "auto_update_interval_minutes"
)

// LoadSettings reads app settings, applying defaults for unset keys.
func (s *Store) LoadSettings() (api.Settings, error) {
	out := api.Settings{
		PollIntervalSeconds:       DefaultPollIntervalSeconds,
		AutoUpdateDefault:         DefaultAutoUpdateDefault,
		AutoUpdateIntervalMinutes: DefaultAutoUpdateIntervalMinutes,
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
	v := "0"
	if cfg.AutoUpdateDefault {
		v = "1"
	}
	return s.SetSetting(keyAutoUpdateDefault, v)
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
