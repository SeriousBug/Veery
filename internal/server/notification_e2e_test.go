package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/auth"
	"github.com/SeriousBug/Veery/internal/notify"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/descope/virtualwebauthn"
)

// enroll redeems an invite token with a fresh virtual authenticator, leaving
// client holding that user's session.
func enroll(t *testing.T, ts *httptest.Server, client *http.Client, token, name string) {
	t.Helper()
	host := strings.TrimPrefix(ts.URL, "http://")
	rp := virtualwebauthn.RelyingParty{Name: "Veery", ID: host[:strings.IndexByte(host, ':')], Origin: ts.URL}
	authenticator := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	req, _ := json.Marshal(map[string]string{"token": token, "name": name})
	resp, body := post(t, client, ts.URL+"/auth/register/begin", req)
	if resp.StatusCode != 200 {
		t.Fatalf("register/begin: %d %s", resp.StatusCode, body)
	}
	attOpts, err := virtualwebauthn.ParseAttestationOptions(string(body))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	authenticator.Options.UserHandle = []byte(attOpts.UserID)
	attResp := virtualwebauthn.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	if resp, body = post(t, client, ts.URL+"/auth/register/finish", []byte(attResp)); resp.StatusCode != 200 {
		t.Fatalf("register/finish: %d %s", resp.StatusCode, body)
	}
	authenticator.AddCredential(cred)
}

func put(t *testing.T, client *http.Client, url string, body any) (*http.Response, []byte) {
	t.Helper()
	js, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(js))
	if err != nil {
		t.Fatalf("build PUT %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", url, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

// TestNotificationConfigE2E drives the notification endpoints through a real
// passkey session: an admin saves a config, sends a test that lands on a real
// webhook, and a non-admin is refused the credentials the config holds.
func TestNotificationConfigE2E(t *testing.T) {
	ts, st, admin, srv := testServerWith(t)
	srv.SetNotifier(notify.New(st))

	delivered := make(chan string, 8)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		delivered <- string(body)
	}))
	defer webhook.Close()
	target := "generic+" + webhook.URL

	inviteURL, err := auth.Bootstrap(st, ts.URL)
	if err != nil || inviteURL == "" {
		t.Fatalf("bootstrap: url=%q err=%v", inviteURL, err)
	}
	enroll(t, ts, admin, inviteURL[strings.Index(inviteURL, "token=")+len("token="):], "Alice")

	// A garbage URL must be refused rather than silently stored and never sent.
	if resp, _ := put(t, admin, ts.URL+"/api/notifications", api.NotificationConfig{
		URLs: []string{"carrier-pigeon://roost"},
	}); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT with an unknown service = %d, want 400", resp.StatusCode)
	}

	resp, body := put(t, admin, ts.URL+"/api/notifications", api.NotificationConfig{
		URLs:   []string{target},
		Events: map[api.NotificationEvent]bool{api.EventUpdateAvailable: false},
	})
	if resp.StatusCode != 200 {
		t.Fatalf("PUT /api/notifications: %d %s", resp.StatusCode, body)
	}

	var got api.NotificationConfig
	_, body = getReq(t, admin, ts.URL+"/api/notifications")
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode config: %v (%s)", err, body)
	}
	if len(got.URLs) != 1 || got.URLs[0] != target {
		t.Errorf("URLs = %v, want the saved target", got.URLs)
	}
	if got.Enabled(api.EventUpdateAvailable) {
		t.Error("update_available should have been switched off")
	}
	if !got.Enabled(api.EventAuth) {
		t.Error("events left out of the map should stay enabled")
	}
	if got.EnvManaged {
		t.Error("a config saved through the API is not env-managed")
	}

	if resp, body = post(t, admin, ts.URL+"/api/notifications/test", []byte(`{}`)); resp.StatusCode != 200 {
		t.Fatalf("POST /api/notifications/test: %d %s", resp.StatusCode, body)
	}
	select {
	case msg := <-delivered:
		if !strings.Contains(msg, "test") {
			t.Errorf("test notification = %q, want it to say it is a test", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the test notification never reached the webhook")
	}

	// Signing in is an auth event, and must reach the webhook on its own.
	member := invitedMember(t, ts, st, admin)
	select {
	case msg := <-delivered:
		if !strings.Contains(msg, "Bob") {
			t.Errorf("auth notification = %q, want it to name the user", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("enrolling a passkey did not notify")
	}

	// The URLs carry webhook tokens, so a signed-in non-admin must not see them.
	if resp, _ = getReq(t, member, ts.URL+"/api/notifications"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("GET /api/notifications as a non-admin = %d, want 403", resp.StatusCode)
	}
	if resp, _ = put(t, member, ts.URL+"/api/notifications", api.NotificationConfig{URLs: []string{target}}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("PUT /api/notifications as a non-admin = %d, want 403", resp.StatusCode)
	}
}

// invitedMember has the admin mint a normal invite, then enrolls a second user
// on it, returning a client holding that user's session.
func invitedMember(t *testing.T, ts *httptest.Server, st *store.Store, admin *http.Client) *http.Client {
	t.Helper()
	req, _ := json.Marshal(api.CreateInviteRequest{IsAdmin: false})
	resp, body := post(t, admin, ts.URL+"/api/invites", req)
	if resp.StatusCode != 200 {
		t.Fatalf("POST /api/invites: %d %s", resp.StatusCode, body)
	}
	var invite api.Invite
	if err := json.Unmarshal(body, &invite); err != nil {
		t.Fatalf("decode invite: %v (%s)", err, body)
	}
	jar, _ := cookiejar.New(nil)
	member := &http.Client{Jar: jar}
	enroll(t, ts, member, invite.Token, "Bob")
	return member
}

func TestEnvManagedNotificationsAreReadOnly(t *testing.T) {
	t.Setenv(notify.EnvURLs, "discord://token@channel")
	ts, st, admin, srv := testServerWith(t)
	srv.SetNotifier(notify.New(st))

	inviteURL, err := auth.Bootstrap(st, ts.URL)
	if err != nil || inviteURL == "" {
		t.Fatalf("bootstrap: url=%q err=%v", inviteURL, err)
	}
	enroll(t, ts, admin, inviteURL[strings.Index(inviteURL, "token=")+len("token="):], "Alice")

	var cfg api.NotificationConfig
	_, body := getReq(t, admin, ts.URL+"/api/notifications")
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatalf("decode config: %v (%s)", err, body)
	}
	if !cfg.EnvManaged || len(cfg.URLs) != 1 {
		t.Fatalf("config = %+v, want the single env target marked env-managed", cfg)
	}

	resp, _ := put(t, admin, ts.URL+"/api/notifications", api.NotificationConfig{URLs: []string{"discord://other@channel"}})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("PUT over an env-managed config = %d, want 409", resp.StatusCode)
	}
}
