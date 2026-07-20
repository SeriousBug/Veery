package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
)

// genericURL turns an httptest server URL into a Shoutrrr generic webhook URL.
func genericURL(serverURL string) string {
	return "generic+" + serverURL
}

// jsonWebhookURL is the canonical generic form, which does read its query as
// config and so can post a JSON body with a separate title field.
func jsonWebhookURL(serverURL string) string {
	return "generic://" + strings.TrimPrefix(serverURL, "http://") + "/hook?template=json&disabletls=yes"
}

func TestSendPostsTitleAndBodyAsJSON(t *testing.T) {
	type payload struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	got := make(chan payload, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p payload
		json.Unmarshal(body, &p)
		got <- p
	}))
	defer srv.Close()

	if err := Send([]string{jsonWebhookURL(srv.URL)}, "Container died", "nginx exited (137)"); err != nil {
		t.Fatalf("send: %v", err)
	}
	p := <-got
	if p.Title != "Container died" {
		t.Errorf("title = %q, want %q", p.Title, "Container died")
	}
	if p.Message != "nginx exited (137)" {
		t.Errorf("message = %q, want %q", p.Message, "nginx exited (137)")
	}
}

// A plain webhook target only receives the message body, so the title has to
// travel inside it.
func TestSendFoldsTitleIntoBodyForPlainWebhooks(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- string(body)
	}))
	defer srv.Close()

	if err := Send([]string{genericURL(srv.URL)}, "Container died", "nginx exited (137)"); err != nil {
		t.Fatalf("send: %v", err)
	}
	body := <-got
	if !strings.Contains(body, "Container died") || !strings.Contains(body, "nginx exited (137)") {
		t.Errorf("body = %q, want it to carry both the title and the message", body)
	}
}

func TestSendReportsFailureWithoutLeakingTheURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := Send([]string{genericURL(srv.URL) + "?title=x&secrettoken=hunter2"}, "t", "b")
	if err == nil {
		t.Fatal("expected an error from a 500 response")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaks the target URL: %v", err)
	}
}

func TestValidateRejectsUnknownService(t *testing.T) {
	if err := Validate([]string{"discord://token@channel"}); err != nil {
		t.Errorf("valid discord URL rejected: %v", err)
	}
	if err := Validate([]string{"carrier-pigeon://roost"}); err == nil {
		t.Error("expected an unknown service to be rejected")
	}
}

func TestEnvEventsSelectsOnlyTheListedEvents(t *testing.T) {
	events := envEvents("auth, update_applied")
	cfg := api.NotificationConfig{Events: events}
	if !cfg.Enabled(api.EventAuth) || !cfg.Enabled(api.EventUpdateApplied) {
		t.Error("listed events should be enabled")
	}
	if cfg.Enabled(api.EventContainerStatus) || cfg.Enabled(api.EventUpdateAvailable) {
		t.Error("unlisted events should be disabled")
	}
}

func TestEmptyEnvEventsEnablesEverything(t *testing.T) {
	cfg := api.NotificationConfig{Events: envEvents("")}
	for _, ev := range api.AllNotificationEvents {
		if !cfg.Enabled(ev) {
			t.Errorf("%s should be enabled when no event list is given", ev)
		}
	}
}

func TestEnvConfigWins(t *testing.T) {
	t.Setenv(EnvURLs, "discord://token@channel  ntfy://ntfy.sh/topic")
	n := New(nil)
	cfg, err := n.Config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if !cfg.EnvManaged {
		t.Error("config from the environment should be marked env-managed")
	}
	if len(cfg.URLs) != 2 {
		t.Fatalf("URLs = %v, want 2 entries", cfg.URLs)
	}
	if err := n.Save(api.NotificationConfig{}); err != ErrEnvManaged {
		t.Errorf("Save into an env-managed config = %v, want ErrEnvManaged", err)
	}
}

// A muted event, or one with no delivery targets, must still be recorded and
// pushed to clients: the log is what makes it safe to turn delivery off.
func TestNotifyRecordsEvenWhenMuted(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "notify.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	n := New(st)
	bc := &fakeBroadcaster{}
	n.SetBroadcaster(bc)

	// No URLs configured and the event muted: nothing is delivered, but it is
	// still recorded and broadcast.
	if err := n.Save(api.NotificationConfig{Events: map[api.NotificationEvent]bool{api.EventContainerMissing: false}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	n.Notify(api.EventContainerMissing, "web was removed", "it is gone",
		api.EventMeta{ContainerName: "web", StackID: "site"})

	page, err := st.ListEvents(store.EventQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("recorded %d events, want 1", len(page.Items))
	}
	got := page.Items[0]
	if got.ContainerName != "web" || got.StackID != "site" || got.Event != api.EventContainerMissing {
		t.Fatalf("recorded row = %+v", got)
	}
	if len(bc.msgs) != 1 || bc.msgs[0].Type != api.WSTypeEvent || bc.msgs[0].Event == nil {
		t.Fatalf("broadcast = %+v, want one event message", bc.msgs)
	}
}

type fakeBroadcaster struct {
	msgs []api.WSMessage
}

func (f *fakeBroadcaster) Broadcast(m api.WSMessage) { f.msgs = append(f.msgs, m) }
