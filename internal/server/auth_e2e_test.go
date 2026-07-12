package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeriousBug/Veery/internal/auth"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/descope/virtualwebauthn"
)

// testServer spins up a full Server backed by a temp SQLite DB and an httptest
// server, returning it plus the RP config matching the server URL.
func testServer(t *testing.T) (*httptest.Server, *store.Store, *http.Client) {
	ts, st, client, _ := testServerWith(t)
	return ts, st, client
}

// testServerWith is testServer, also handing back the Server so a test can
// attach dependencies like the notifier.
func testServerWith(t *testing.T) (*httptest.Server, *store.Store, *http.Client, *Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	ts := httptest.NewServer(nil)
	t.Cleanup(ts.Close)

	// RPID must be the host of the test origin (127.0.0.1).
	host := strings.TrimPrefix(ts.URL, "http://")
	rpID := host[:strings.IndexByte(host, ':')]

	mgr, err := auth.NewManager(st, auth.Config{RPID: rpID, Origin: ts.URL})
	if err != nil {
		t.Fatalf("auth manager: %v", err)
	}
	srv := New(st, mgr, Config{RPID: rpID, Origin: ts.URL})
	ts.Config.Handler = srv.Handler()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	return ts, st, client, srv
}

func post(t *testing.T, client *http.Client, url string, body []byte) (*http.Response, []byte) {
	t.Helper()
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}

// TestEnrollAndLogin exercises the full passkey ceremony end to end with a
// virtual authenticator: bootstrap invite -> enroll -> session -> logout ->
// usernameless login.
func TestEnrollAndLogin(t *testing.T) {
	ts, st, client := testServer(t)

	// Bootstrap the first admin invite (mirrors what main does on startup).
	inviteURL, err := auth.Bootstrap(st, ts.URL)
	if err != nil || inviteURL == "" {
		t.Fatalf("bootstrap: url=%q err=%v", inviteURL, err)
	}
	token := inviteURL[strings.Index(inviteURL, "token=")+len("token="):]

	host := strings.TrimPrefix(ts.URL, "http://")
	rpID := host[:strings.IndexByte(host, ':')]
	rp := virtualwebauthn.RelyingParty{Name: "Veery", ID: rpID, Origin: ts.URL}
	authenticator := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	// --- Registration ---
	enrollReq, _ := json.Marshal(map[string]string{"token": token, "name": "Alice"})
	resp, body := post(t, client, ts.URL+"/auth/register/begin", enrollReq)
	if resp.StatusCode != 200 {
		t.Fatalf("register/begin: %d %s", resp.StatusCode, body)
	}
	attOpts, err := virtualwebauthn.ParseAttestationOptions(string(body))
	if err != nil {
		t.Fatalf("parse attestation options: %v (body=%s)", err, body)
	}
	// Discoverable login requires the authenticator to return a user handle.
	authenticator.Options.UserHandle = []byte(attOpts.UserID)
	attResp := virtualwebauthn.CreateAttestationResponse(rp, authenticator, cred, *attOpts)
	resp, body = post(t, client, ts.URL+"/auth/register/finish", []byte(attResp))
	if resp.StatusCode != 200 {
		t.Fatalf("register/finish: %d %s", resp.StatusCode, body)
	}
	authenticator.AddCredential(cred)

	// Session cookie should now be set: /auth/me works.
	meResp, meBody := getReq(t, client, ts.URL+"/auth/me")
	if meResp.StatusCode != 200 {
		t.Fatalf("/auth/me after enroll: %d %s", meResp.StatusCode, meBody)
	}
	if !strings.Contains(string(meBody), "Alice") {
		t.Fatalf("/auth/me missing name: %s", meBody)
	}

	// --- Logout ---
	post(t, client, ts.URL+"/auth/logout", nil)
	if r, _ := getReq(t, client, ts.URL+"/auth/me"); r.StatusCode != 401 {
		t.Fatalf("/auth/me after logout should be 401, got %d", r.StatusCode)
	}

	// --- Usernameless login ---
	resp, body = post(t, client, ts.URL+"/auth/login/begin", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("login/begin: %d %s", resp.StatusCode, body)
	}
	asrOpts, err := virtualwebauthn.ParseAssertionOptions(string(body))
	if err != nil {
		t.Fatalf("parse assertion options: %v (body=%s)", err, body)
	}
	asrResp := virtualwebauthn.CreateAssertionResponse(rp, authenticator, cred, *asrOpts)
	resp, body = post(t, client, ts.URL+"/auth/login/finish", []byte(asrResp))
	if resp.StatusCode != 200 {
		t.Fatalf("login/finish: %d %s", resp.StatusCode, body)
	}
	if r, b := getReq(t, client, ts.URL+"/auth/me"); r.StatusCode != 200 || !strings.Contains(string(b), "Alice") {
		t.Fatalf("/auth/me after login: %d %s", r.StatusCode, b)
	}
}

func getReq(t *testing.T, client *http.Client, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, data
}
