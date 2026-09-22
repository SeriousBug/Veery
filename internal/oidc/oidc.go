// Package oidc implements the relying-party side of an OpenID Connect login. It
// runs the authorization-code flow with PKCE against a configured provider
// (pocket-id, Keycloak, Authentik, ...), verifies the returned ID token, and
// hands the server the claims it needs to find or provision the matching user.
//
// Passkeys remain the primary credential. OIDC is additive: when it is not
// configured the provider side stays disabled and the login page only offers
// passkeys.
package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config configures the provider. Issuer and ClientID are the only required
// fields; when either is empty OIDC is disabled.
type Config struct {
	Issuer       string   // e.g. https://id.example.com
	ClientID     string   // client id registered with the provider
	ClientSecret string   // leave empty for a public client using PKCE
	RedirectURL  string   // e.g. https://veery.example.com/auth/oidc/callback
	Scopes       []string // defaults to openid profile email; groups is added with AdminGroups
	AdminGroups  []string // provider group names that grant Veery admin
	ProviderName string   // shown on the login button, e.g. "Pocket ID"
}

// Identity is the subset of provider claims Veery acts on.
type Identity struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
	Groups  []string
}

// Manager holds the provider configuration, the cached discovery document, and
// the short-lived login flows that are in flight.
type Manager struct {
	cfg         Config
	adminGroups map[string]bool

	mu       sync.Mutex
	provider *gooidc.Provider
	oauth    *oauth2.Config
	flows    map[string]*loginFlow
}

// loginFlow is the server-side state for one authorization request, keyed by the
// OAuth state value. The nonce and PKCE verifier never leave the server.
type loginFlow struct {
	nonce    string
	verifier string
	expires  time.Time
}

// New validates cfg and returns a manager. It does not reach out to the
// provider; discovery happens on the first login.
func New(cfg Config) (*Manager, error) {
	if cfg.Issuer == "" && cfg.ClientID == "" {
		return nil, errors.New("oidc: issuer and client id are both empty")
	}
	if cfg.Issuer == "" || cfg.ClientID == "" {
		return nil, errors.New("oidc: issuer and client id must both be set")
	}
	if cfg.RedirectURL == "" {
		return nil, errors.New("oidc: redirect url is required")
	}
	if len(cfg.Scopes) == 0 {
		cfg.Scopes = []string{gooidc.ScopeOpenID, "profile", "email"}
	}
	groups := map[string]bool{}
	for _, g := range cfg.AdminGroups {
		if g = strings.TrimSpace(g); g != "" {
			groups[strings.ToLower(g)] = true
		}
	}
	// The groups claim is not part of core OIDC; it is only meaningful for the
	// admin mapping, so ask for it exactly when an admin group is configured.
	if len(groups) > 0 && !hasScope(cfg.Scopes, "groups") {
		cfg.Scopes = append(cfg.Scopes, "groups")
	}
	return &Manager{cfg: cfg, adminGroups: groups, flows: map[string]*loginFlow{}}, nil
}

func hasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// Enabled reports whether OIDC is configured.
func (m *Manager) Enabled() bool { return m != nil && m.cfg.Issuer != "" && m.cfg.ClientID != "" }

// Name is the human label for the provider, used on the login button.
func (m *Manager) Name() string {
	if m.cfg.ProviderName != "" {
		return m.cfg.ProviderName
	}
	return "SSO"
}

// Issuer returns the configured issuer URL (empty when disabled).
func (m *Manager) Issuer() string { return m.cfg.Issuer }

// IsAdmin reports whether any of the provider groups grant Veery admin.
func (m *Manager) IsAdmin(groups []string) bool {
	for _, g := range groups {
		if m.adminGroups[strings.ToLower(strings.TrimSpace(g))] {
			return true
		}
	}
	return false
}

// HasAdminGroups reports whether a group-to-admin mapping is configured. When it
// is not, OIDC logins leave existing admin flags alone rather than clearing them.
func (m *Manager) HasAdminGroups() bool { return len(m.adminGroups) > 0 }

// Start begins a login: it discovers the provider, mints state/nonce/PKCE, and
// returns the URL to redirect the browser to. The matching state is kept in
// memory until Finish consumes it or it expires.
func (m *Manager) Start(ctx context.Context) (authURL, state string, err error) {
	_, oauth, err := m.discover(ctx)
	if err != nil {
		return "", "", err
	}
	state = randToken(24)
	nonce := randToken(24)
	verifier := oauth2.GenerateVerifier()

	m.mu.Lock()
	m.flows[state] = &loginFlow{nonce: nonce, verifier: verifier, expires: time.Now().Add(10 * time.Minute)}
	m.gcLocked()
	m.mu.Unlock()

	authURL = oauth.AuthCodeURL(state, gooidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
	return authURL, state, nil
}

// Finish completes a login: it exchanges the code, verifies the ID token
// (signature, issuer, audience, expiry and nonce), and returns the identity.
func (m *Manager) Finish(ctx context.Context, state, code string) (Identity, error) {
	flow, ok := m.takeFlow(state)
	if !ok {
		return Identity{}, ErrInvalidState
	}
	provider, oauth, err := m.discover(ctx)
	if err != nil {
		return Identity{}, err
	}
	tok, err := oauth.Exchange(ctx, code, oauth2.VerifierOption(flow.verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange code: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return Identity{}, ErrMissingIDToken
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: m.cfg.ClientID})
	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		return Identity{}, fmt.Errorf("verify id token: %w", err)
	}
	if idToken.Nonce != flow.nonce {
		return Identity{}, ErrNonceMismatch
	}

	var c claims
	if err := idToken.Claims(&c); err != nil {
		return Identity{}, fmt.Errorf("read id token claims: %w", err)
	}
	// Some providers only release groups (and sometimes email) through the
	// userinfo endpoint. Merge it in best-effort; the ID token has already been
	// verified, and its claims win.
	if ui, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil {
		var uc claims
		if ui.Claims(&uc) == nil {
			c.merge(uc)
		}
	}

	ident := Identity{
		Issuer:  idToken.Issuer,
		Subject: c.Sub,
		Email:   c.Email,
		Name:    c.displayName(),
		Groups:  c.Groups,
	}
	if ident.Subject == "" {
		return Identity{}, ErrMissingSubject
	}
	return ident, nil
}

// discover fetches and caches the provider discovery document, returning a
// provider and an oauth2 config pointed at its endpoints. A failed discovery is
// not cached, so a provider that is briefly down can recover without a restart.
func (m *Manager) discover(ctx context.Context) (*gooidc.Provider, *oauth2.Config, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.provider != nil {
		return m.provider, m.oauth, nil
	}
	p, err := gooidc.NewProvider(ctx, m.cfg.Issuer)
	if err != nil {
		return nil, nil, fmt.Errorf("oidc discovery: %w", err)
	}
	m.provider = p
	m.oauth = &oauth2.Config{
		ClientID:     m.cfg.ClientID,
		ClientSecret: m.cfg.ClientSecret,
		Endpoint:     p.Endpoint(),
		RedirectURL:  m.cfg.RedirectURL,
		Scopes:       m.cfg.Scopes,
	}
	return m.provider, m.oauth, nil
}

func (m *Manager) takeFlow(state string) (*loginFlow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.flows[state]
	if ok {
		delete(m.flows, state)
	}
	if ok && time.Now().After(f.expires) {
		return nil, false
	}
	return f, ok
}

func (m *Manager) gcLocked() {
	now := time.Now()
	for k, v := range m.flows {
		if now.After(v.expires) {
			delete(m.flows, k)
		}
	}
}

// claims is the union of the OIDC claims Veery reads. Unknown claims are
// ignored.
type claims struct {
	Sub               string   `json:"sub"`
	Name              string   `json:"name"`
	PreferredUsername string   `json:"preferred_username"`
	DisplayName       string   `json:"display_name"`
	Email             string   `json:"email"`
	Groups            []string `json:"groups"`
}

// displayName picks the friendliest available name for a user.
func (c claims) displayName() string {
	for _, s := range []string{c.Name, c.DisplayName, c.PreferredUsername, c.Email, c.Sub} {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// merge fills empty fields and unions groups from a lower-priority source
// (the userinfo response) without overwriting what the ID token had.
func (c *claims) merge(other claims) {
	if c.Sub == "" {
		c.Sub = other.Sub
	}
	if c.Name == "" {
		c.Name = other.Name
	}
	if c.DisplayName == "" {
		c.DisplayName = other.DisplayName
	}
	if c.PreferredUsername == "" {
		c.PreferredUsername = other.PreferredUsername
	}
	if c.Email == "" {
		c.Email = other.Email
	}
	if len(other.Groups) > 0 {
		seen := map[string]bool{}
		for _, g := range c.Groups {
			seen[g] = true
		}
		for _, g := range other.Groups {
			if !seen[g] {
				c.Groups = append(c.Groups, g)
				seen[g] = true
			}
		}
	}
}

// Errors surfaced to handlers.
var (
	ErrInvalidState   = errors.New("login expired or state mismatch")
	ErrMissingIDToken = errors.New("provider returned no id token")
	ErrNonceMismatch  = errors.New("id token nonce mismatch")
	ErrMissingSubject = errors.New("id token has no subject")
)

func randToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
