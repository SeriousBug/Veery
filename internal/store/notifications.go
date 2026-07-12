package store

import (
	"encoding/json"
	"errors"

	"github.com/SeriousBug/Veery/internal/api"
)

const (
	keyNotifyURLs   = "notify_urls"
	keyNotifyEvents = "notify_events"
)

// LoadNotificationConfig reads the notification config. An unset config means
// no targets and every event enabled, so nothing is sent until a URL is added.
func (s *Store) LoadNotificationConfig() (api.NotificationConfig, error) {
	out := api.NotificationConfig{
		URLs:   []string{},
		Events: map[api.NotificationEvent]bool{},
	}
	if err := s.getJSON(keyNotifyURLs, &out.URLs); err != nil {
		return out, err
	}
	if err := s.getJSON(keyNotifyEvents, &out.Events); err != nil {
		return out, err
	}
	return out, nil
}

// SaveNotificationConfig persists the notification config.
func (s *Store) SaveNotificationConfig(cfg api.NotificationConfig) error {
	if err := s.setJSON(keyNotifyURLs, cfg.URLs); err != nil {
		return err
	}
	return s.setJSON(keyNotifyEvents, cfg.Events)
}

// The notifier remembers what it last told the user about, so a restart does
// not replay stale container statuses or re-announce updates it already
// announced. Only transitions against these are notified.
const (
	keyNotifiedStatuses = "notify_last_statuses"
	keyNotifiedUpdates  = "notify_last_update_available"
)

// LoadNotifiedStatuses returns the container statuses as of the last sweep.
func (s *Store) LoadNotifiedStatuses() (map[string]api.ContainerStatus, error) {
	out := map[string]api.ContainerStatus{}
	err := s.getJSON(keyNotifiedStatuses, &out)
	return out, err
}

// SaveNotifiedStatuses records the container statuses as of this sweep.
func (s *Store) SaveNotifiedStatuses(m map[string]api.ContainerStatus) error {
	return s.setJSON(keyNotifiedStatuses, m)
}

// LoadNotifiedUpdates returns the update-available flags as of the last sweep.
func (s *Store) LoadNotifiedUpdates() (map[string]bool, error) {
	out := map[string]bool{}
	err := s.getJSON(keyNotifiedUpdates, &out)
	return out, err
}

// SaveNotifiedUpdates records the update-available flags as of this sweep.
func (s *Store) SaveNotifiedUpdates(m map[string]bool) error {
	return s.setJSON(keyNotifiedUpdates, m)
}

// getJSON decodes a JSON-encoded setting into dst, leaving dst untouched when
// the key is unset or holds unparseable JSON from an older version.
func (s *Store) getJSON(key string, dst any) error {
	v, err := s.GetSetting(key)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(v), dst); err != nil {
		return nil
	}
	return nil
}

func (s *Store) setJSON(key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.SetSetting(key, string(b))
}
