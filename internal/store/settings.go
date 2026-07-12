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
	keyDiskReadPeak       = "disk_read_peak_bytes_per_sec"
	keyDiskWritePeak      = "disk_write_peak_bytes_per_sec"
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

// LoadDiskIOPeaks returns the persisted highwater marks for disk read/write
// throughput, defaulting to 0 when unset.
func (s *Store) LoadDiskIOPeaks() (read, write uint64, err error) {
	read, err = s.getUint(keyDiskReadPeak)
	if err != nil {
		return 0, 0, err
	}
	write, err = s.getUint(keyDiskWritePeak)
	if err != nil {
		return 0, 0, err
	}
	return read, write, nil
}

// SaveDiskIOPeaks persists the highwater marks for disk read/write throughput.
func (s *Store) SaveDiskIOPeaks(read, write uint64) error {
	if err := s.SetSetting(keyDiskReadPeak, strconv.FormatUint(read, 10)); err != nil {
		return err
	}
	return s.SetSetting(keyDiskWritePeak, strconv.FormatUint(write, 10))
}

func (s *Store) getUint(key string) (uint64, error) {
	v, err := s.GetSetting(key)
	if errors.Is(err, ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return 0, nil
	}
	return n, nil
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
