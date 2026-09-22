package oidc

import (
	"context"
	"net/url"
	"testing"

	"github.com/SeriousBug/Veery/internal/oidc/fakeidp"
)

const (
	testClientID = "veery-test"
	testRedirect = "https://veery.example.com/auth/oidc/callback"
)

func newManager(t *testing.T, issuer string, adminGroups ...string) *Manager {
	t.Helper()
	m, err := New(Config{
		Issuer: issuer, ClientID: testClientID, ClientSecret: "secret",
		RedirectURL: testRedirect, AdminGroups: adminGroups, ProviderName: "Test IDP",
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	return m
}

func TestLoginFlow(t *testing.T) {
	p := fakeidp.New(testClientID)
	defer p.Close()
	p.Claims = map[string]any{
		"sub": "user-123", "name": "Ada Lovelace",
		"email": "ada@example.com", "groups": []string{"users", "veery-admins"},
	}
	m := newManager(t, p.URL(), "veery-admins")

	authURL, state, err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("missing S256 PKCE, got %q", q.Get("code_challenge_method"))
	}
	if q.Get("state") != state {
		t.Errorf("state mismatch: url=%q returned=%q", q.Get("state"), state)
	}
	if q.Get("redirect_uri") != testRedirect {
		t.Errorf("redirect_uri = %q", q.Get("redirect_uri"))
	}
	p.Challenge = q.Get("code_challenge")
	p.Nonce = q.Get("nonce")

	ident, err := m.Finish(context.Background(), state, "authcode")
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !p.PKCEOK() {
		t.Error("provider did not receive the matching PKCE verifier")
	}
	if ident.Issuer != p.URL() {
		t.Errorf("issuer = %q", ident.Issuer)
	}
	if ident.Subject != "user-123" || ident.Email != "ada@example.com" || ident.Name != "Ada Lovelace" {
		t.Errorf("identity = %+v", ident)
	}
	if !m.IsAdmin(ident.Groups) {
		t.Errorf("expected admin for groups %v", ident.Groups)
	}
}

func TestFinishRejectsUnknownState(t *testing.T) {
	p := fakeidp.New(testClientID)
	defer p.Close()
	m := newManager(t, p.URL())
	if _, _, err := m.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := m.Finish(context.Background(), "not-a-real-state", "code"); err != ErrInvalidState {
		t.Fatalf("want ErrInvalidState, got %v", err)
	}
}

func TestFinishRejectsNonceMismatch(t *testing.T) {
	p := fakeidp.New(testClientID)
	defer p.Close()
	p.Claims = map[string]any{"sub": "user-1"}
	m := newManager(t, p.URL())
	authURL, state, err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	u, _ := url.Parse(authURL)
	p.Challenge = u.Query().Get("code_challenge")
	p.Nonce = "a-different-nonce"
	if _, err := m.Finish(context.Background(), state, "code"); err != ErrNonceMismatch {
		t.Fatalf("want ErrNonceMismatch, got %v", err)
	}
}

func TestDisplayNameFallback(t *testing.T) {
	if got := (claims{PreferredUsername: "ada"}).displayName(); got != "ada" {
		t.Errorf("preferred_username fallback = %q", got)
	}
	if got := (claims{Email: "a@b.c", Sub: "x"}).displayName(); got != "a@b.c" {
		t.Errorf("email fallback = %q", got)
	}
	if got := (claims{Sub: "x"}).displayName(); got != "x" {
		t.Errorf("sub fallback = %q", got)
	}
}

func TestAdminGroups(t *testing.T) {
	m, err := New(Config{Issuer: "https://idp", ClientID: "c", RedirectURL: "https://r"})
	if err != nil {
		t.Fatal(err)
	}
	if m.HasAdminGroups() {
		t.Error("no admin groups should be configured")
	}
	if m.IsAdmin([]string{"any"}) {
		t.Error("nobody is admin without a mapping")
	}
	// No admin groups means the groups scope is not forced onto the request.
	for _, s := range m.cfg.Scopes {
		if s == "groups" {
			t.Errorf("groups scope should not be requested without admin groups")
		}
	}

	m, err = New(Config{Issuer: "https://idp", ClientID: "c", RedirectURL: "https://r", AdminGroups: []string{"Admins"}})
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasAdminGroups() || !m.IsAdmin([]string{"admins"}) {
		t.Error("expected case-insensitive admin group match")
	}
	found := false
	for _, s := range m.cfg.Scopes {
		if s == "groups" {
			found = true
		}
	}
	if !found {
		t.Error("admin groups should auto-request the groups scope")
	}
}
