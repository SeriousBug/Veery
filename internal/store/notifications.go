package store

import (
	"encoding/json"
	"errors"

	"github.com/SeriousBug/Veery/internal/api"
)

const (
	keyNotifyTargets = "notify_targets"
	// Written by versions before events became per-target: a URL list and one
	// event map shared by all of them. Read once, then overwritten on the next
	// save so the old shape cannot come back.
	keyNotifyURLs   = "notify_urls"
	keyNotifyEvents = "notify_events"
)

// LoadNotificationConfig reads the notification config. An unset config means
// no targets, so nothing is sent until one is added.
func (s *Store) LoadNotificationConfig() (api.NotificationConfig, error) {
	out := api.NotificationConfig{Targets: []api.NotificationTarget{}}
	if err := s.getJSON(keyNotifyTargets, &out.Targets); err != nil {
		return out, err
	}
	if len(out.Targets) > 0 {
		return out, nil
	}
	return s.legacyNotificationConfig()
}

// legacyNotificationConfig rebuilds targets from the pre-per-target keys, giving
// every URL the event map they all used to share.
func (s *Store) legacyNotificationConfig() (api.NotificationConfig, error) {
	out := api.NotificationConfig{Targets: []api.NotificationTarget{}}
	var urls []string
	events := map[api.NotificationEvent]bool{}
	if err := s.getJSON(keyNotifyURLs, &urls); err != nil {
		return out, err
	}
	if err := s.getJSON(keyNotifyEvents, &events); err != nil {
		return out, err
	}
	for _, u := range urls {
		copied := make(map[api.NotificationEvent]bool, len(events))
		for ev, on := range events {
			copied[ev] = on
		}
		out.Targets = append(out.Targets, api.NotificationTarget{URL: u, Events: copied})
	}
	return out, nil
}

// SaveNotificationConfig persists the notification config. It clears the legacy
// keys so removing every target cannot resurrect the pre-per-target URLs.
func (s *Store) SaveNotificationConfig(cfg api.NotificationConfig) error {
	if err := s.setJSON(keyNotifyTargets, cfg.Targets); err != nil {
		return err
	}
	if err := s.setJSON(keyNotifyURLs, []string{}); err != nil {
		return err
	}
	return s.setJSON(keyNotifyEvents, map[api.NotificationEvent]bool{})
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
