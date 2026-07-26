package store

import (
	"path/filepath"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
)

func openStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// A config written before events were per-target must come back as targets that
// each carry what used to be the one shared event map.
func TestLoadNotificationConfigUpgradesLegacyKeys(t *testing.T) {
	st := openStore(t)
	if err := st.setJSON(keyNotifyURLs, []string{"discord://token@channel", "ntfy://ntfy.sh/topic"}); err != nil {
		t.Fatalf("seed urls: %v", err)
	}
	if err := st.setJSON(keyNotifyEvents, map[api.NotificationEvent]bool{api.EventAuth: false}); err != nil {
		t.Fatalf("seed events: %v", err)
	}

	cfg, err := st.LoadNotificationConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Targets) != 2 {
		t.Fatalf("Targets = %+v, want 2", cfg.Targets)
	}
	for _, target := range cfg.Targets {
		if target.Enabled(api.EventAuth) {
			t.Errorf("%s: auth should have stayed switched off", target.URL)
		}
		if !target.Enabled(api.EventContainerStatus) {
			t.Errorf("%s: unlisted events should stay enabled", target.URL)
		}
	}
}

// Saving must clear the legacy keys, or deleting every target would bring the
// old URLs back on the next load.
func TestSaveNotificationConfigClearsLegacyKeys(t *testing.T) {
	st := openStore(t)
	if err := st.setJSON(keyNotifyURLs, []string{"discord://token@channel"}); err != nil {
		t.Fatalf("seed urls: %v", err)
	}

	if err := st.SaveNotificationConfig(api.NotificationConfig{Targets: []api.NotificationTarget{}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	cfg, err := st.LoadNotificationConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Targets) != 0 {
		t.Fatalf("Targets = %+v, want none", cfg.Targets)
	}
}

func TestSaveAndLoadNotificationTargets(t *testing.T) {
	st := openStore(t)
	want := api.NotificationConfig{Targets: []api.NotificationTarget{
		{URL: "discord://token@channel", Events: map[api.NotificationEvent]bool{api.EventAuth: false}},
		{URL: "ntfy://ntfy.sh/topic", Events: map[api.NotificationEvent]bool{}},
	}}
	if err := st.SaveNotificationConfig(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := st.LoadNotificationConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got.Targets) != 2 || got.Targets[0].URL != want.Targets[0].URL {
		t.Fatalf("Targets = %+v, want %+v", got.Targets, want.Targets)
	}
	if got.Targets[0].Enabled(api.EventAuth) {
		t.Error("the first target should not want auth events")
	}
	if !got.Targets[1].Enabled(api.EventAuth) {
		t.Error("the second target should want auth events")
	}
}
