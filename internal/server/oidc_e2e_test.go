package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/oidc"
	"github.com/SeriousBug/Veery/internal/oidc/fakeidp"
)

// newOIDCClient returns a client that keeps cookies but does not follow
// redirects, so a test can inspect the Location and drive each hop itself.
func newOIDCClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// oidcStart drives GET /auth/oidc/start, configures the fake provider with the
// nonce and PKCE challenge from the redirect, and returns the state.
func oidcStart(t *testing.T, base string, c *http.Client, p *fakeidp.Provider) string {
	t.Helper()
	resp, err := c.Get(base + "/auth/oidc/start")
	if err != nil {
		t.Fatalf("oidc start: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("oidc start status = %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse start redirect: %v", err)
	}
	q := loc.Query()
	p.Challenge = q.Get("code_challenge")
	p.Nonce = q.Get("nonce")
	return q.Get("state")
}

// oidcCallback completes the flow with the given state.
func oidcCallback(t *testing.T, base string, c *http.Client, state string) *http.Response {
	t.Helper()
	resp, err := c.Get(base + "/auth/oidc/callback?code=test-code&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatalf("oidc callback: %v", err)
	}
	resp.Body.Close()
	return resp
}

// oidcLogin runs a full fake-IdP login and returns the redirect response.
func oidcLogin(t *testing.T, base string, c *http.Client, p *fakeidp.Provider) *http.Response {
	t.Helper()
	state := oidcStart(t, base, c, p)
	return oidcCallback(t, base, c, state)
}

func TestOIDCProvidersEndpoint(t *testing.T) {
	ts, _, client, srv := testServerWith(t)
	// Disabled by default.
	resp, body := getReq(t, client, ts.URL+"/auth/providers")
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"oidc":false`) {
		t.Fatalf("providers (disabled): %d %s", resp.StatusCode, body)
	}

	p := fakeidp.New("veery")
	defer p.Close()
	mgr, err := oidc.New(oidc.Config{
		Issuer: p.URL(), ClientID: "veery",
		RedirectURL: ts.URL + "/auth/oidc/callback", ProviderName: "Pocket ID",
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetOIDC(mgr)

	_, body = getReq(t, client, ts.URL+"/auth/providers")
	var got api.AuthProviders
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode providers: %v (%s)", err, body)
	}
	if !got.OIDC || got.OIDCName != "Pocket ID" {
		t.Fatalf("providers = %+v", got)
	}
}

func TestOIDCLoginProvisionsAndReusesUser(t *testing.T) {
	ts, st, _, srv := testServerWith(t)
	p := fakeidp.New("veery")
	defer p.Close()
	// No admin groups configured; on a fresh instance the first user is admin.
	mgr, err := oidc.New(oidc.Config{Issuer: p.URL(), ClientID: "veery", RedirectURL: ts.URL + "/auth/oidc/callback"})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetOIDC(mgr)

	c := newOIDCClient(t)
	p.Claims = map[string]any{"sub": "external-1", "name": "Alice", "email": "alice@example.com", "groups": []string{"users"}}

	resp := oidcLogin(t, ts.URL, c, p)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/" {
		t.Fatalf("callback: %d -> %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if !p.PKCEOK() {
		t.Error("PKCE verifier did not match")
	}

	meResp, meBody := getReq(t, c, ts.URL+"/auth/me")
	if meResp.StatusCode != 200 {
		t.Fatalf("/auth/me: %d %s", meResp.StatusCode, meBody)
	}
	var session api.SessionInfo
	if err := json.Unmarshal(meBody, &session); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if session.User.Name != "Alice" || !session.User.IsAdmin {
		t.Fatalf("provisioned user = %+v", session.User)
	}

	// Logging in again must reuse the same account, not create another.
	if resp := oidcLogin(t, ts.URL, c, p); resp.StatusCode != http.StatusFound {
		t.Fatalf("second login: %d", resp.StatusCode)
	}
	if n, _ := st.CountUsers(); n != 1 {
		t.Fatalf("expected 1 user after repeat login, got %d", n)
	}
	idents, err := st.OIDCIdentitiesByUser(session.User.ID)
	if err != nil || len(idents) != 1 {
		t.Fatalf("identities = %+v, err=%v", idents, err)
	}
}

func TestOIDCAdminGroupSync(t *testing.T) {
	ts, st, _, srv := testServerWith(t)
	// A pre-existing passkey admin so admin changes cannot hit the last-admin guard.
	if _, err := st.CreateUser("boot", "Boot", true); err != nil {
		t.Fatal(err)
	}
	p := fakeidp.New("veery")
	defer p.Close()
	mgr, err := oidc.New(oidc.Config{
		Issuer: p.URL(), ClientID: "veery", RedirectURL: ts.URL + "/auth/oidc/callback",
		AdminGroups: []string{"Veery Admins"},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetOIDC(mgr)

	c := newOIDCClient(t)
	p.Claims = map[string]any{"sub": "external-2", "name": "Bob", "groups": []string{"Veery Admins"}}
	if resp := oidcLogin(t, ts.URL, c, p); resp.StatusCode != http.StatusFound {
		t.Fatalf("login as admin: %d", resp.StatusCode)
	}
	user := meUser(t, ts.URL, c)
	if !user.IsAdmin {
		t.Fatalf("expected admin for group member, got %+v", user)
	}

	// Dropping out of the group demotes on the next login.
	p.Claims = map[string]any{"sub": "external-2", "name": "Bob", "groups": []string{"users"}}
	if resp := oidcLogin(t, ts.URL, c, p); resp.StatusCode != http.StatusFound {
		t.Fatalf("login as non-admin: %d", resp.StatusCode)
	}
	if user = meUser(t, ts.URL, c); user.IsAdmin {
		t.Fatalf("expected demotion after leaving admin group, got %+v", user)
	}

	// Rejoining promotes again.
	p.Claims = map[string]any{"sub": "external-2", "name": "Bob", "groups": []string{"veery admins"}}
	if resp := oidcLogin(t, ts.URL, c, p); resp.StatusCode != http.StatusFound {
		t.Fatalf("login as admin again: %d", resp.StatusCode)
	}
	if user = meUser(t, ts.URL, c); !user.IsAdmin {
		t.Fatalf("expected promotion after rejoining group, got %+v", user)
	}
}

func TestOIDCCallbackRejectsMissingStateCookie(t *testing.T) {
	ts, _, _, srv := testServerWith(t)
	p := fakeidp.New("veery")
	defer p.Close()
	mgr, err := oidc.New(oidc.Config{Issuer: p.URL(), ClientID: "veery", RedirectURL: ts.URL + "/auth/oidc/callback"})
	if err != nil {
		t.Fatal(err)
	}
	srv.SetOIDC(mgr)

	// No start first, so there is no state cookie to match.
	c := newOIDCClient(t)
	resp := oidcCallback(t, ts.URL, c, "forged-state")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("callback status = %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); !strings.Contains(loc, "error=oidc_state") {
		t.Fatalf("expected state error redirect, got %q", loc)
	}
	if r, _ := getReq(t, c, ts.URL+"/auth/me"); r.StatusCode != 401 {
		t.Fatalf("no session should be issued, /auth/me = %d", r.StatusCode)
	}
}

func meUser(t *testing.T, base string, c *http.Client) api.User {
	t.Helper()
	resp, body := getReq(t, c, base+"/auth/me")
	if resp.StatusCode != 200 {
		t.Fatalf("/auth/me: %d %s", resp.StatusCode, body)
	}
	var session api.SessionInfo
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	return session.User
}
