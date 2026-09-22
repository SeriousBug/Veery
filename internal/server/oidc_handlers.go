package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log"
	"net/http"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/oidc"
	"github.com/SeriousBug/Veery/internal/store"
)

const oidcStateCookieName = "veery_oidc_state"

// handleAuthProviders reports which login methods are available so the login
// page can show the right buttons. Public.
func (s *Server) handleAuthProviders(w http.ResponseWriter, r *http.Request) {
	out := api.AuthProviders{}
	if s.oidc.Enabled() {
		out.OIDC = true
		out.OIDCName = s.oidc.Name()
	}
	writeJSON(w, http.StatusOK, out)
}

// handleOIDCStart kicks off the authorization-code flow: it builds the provider
// URL and redirects there. The state is bound to a short-lived cookie so a flow
// started in another browser cannot be completed here (login CSRF).
func (s *Server) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	if !s.oidc.Enabled() {
		writeErr(w, http.StatusNotFound, "OIDC is not configured")
		return
	}
	authURL, state, err := s.oidc.Start(r.Context())
	if err != nil {
		log.Printf("oidc: start: %v", err)
		writeErr(w, http.StatusBadGateway, "could not reach the identity provider")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookieName, Value: state, Path: "/auth/oidc", MaxAge: 600,
		HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleOIDCCallback completes the flow: validate the state, exchange the code,
// verify the ID token, find or provision the matching user, and issue a session.
func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if !s.oidc.Enabled() {
		writeErr(w, http.StatusNotFound, "OIDC is not configured")
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		// The provider refused or the user cancelled. Do not echo the raw
		// description into the URL; log it and send a short code.
		log.Printf("oidc: provider error: %s: %s", e, q.Get("error_description"))
		s.redirectOIDCError(w, r, "oidc_denied")
		return
	}
	state := q.Get("state")
	if !s.validOIDCState(r, state) {
		log.Printf("oidc: state mismatch or expired")
		s.redirectOIDCError(w, r, "oidc_state")
		return
	}
	s.clearOIDCStateCookie(w)

	ident, err := s.oidc.Finish(r.Context(), state, q.Get("code"))
	if err != nil {
		log.Printf("oidc: finish: %v", err)
		s.redirectOIDCError(w, r, "oidc_failed")
		return
	}
	userID, err := s.resolveOIDCUser(ident)
	if err != nil {
		log.Printf("oidc: resolve user: %v", err)
		s.redirectOIDCError(w, r, "oidc_failed")
		return
	}
	s.notifyAuth("Signed in", userID, "Signed in via "+s.oidc.Name()+".")
	s.issueSession(w, userID)
	http.Redirect(w, r, "/", http.StatusFound)
}

// resolveOIDCUser maps an external identity to a Veery user, provisioning one on
// first sight. Admin rights come from the configured provider groups; when no
// admin groups are configured the flag is left alone, so the bootstrap admin
// (the first user on a fresh instance) is not demoted by accident.
func (s *Server) resolveOIDCUser(ident oidc.Identity) (string, error) {
	if link, err := s.store.OIDCIdentityBySubject(ident.Issuer, ident.Subject); err == nil {
		u, err := s.store.GetUser(link.UserID)
		if err != nil {
			return "", err
		}
		s.syncOIDCAdmin(u, ident)
		return u.ID, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}

	isAdmin := s.oidc.IsAdmin(ident.Groups)
	// Mirror the passkey bootstrap: on a fresh instance the first user to arrive
	// is the admin, whichever method they used.
	if n, err := s.store.CountUsers(); err == nil && n == 0 {
		isAdmin = true
	}
	name := ident.Name
	if name == "" {
		name = ident.Email
	}
	u, err := s.store.CreateUser(newUserID(), name, isAdmin)
	if err != nil {
		return "", err
	}
	if err := s.store.LinkOIDCIdentity(u.ID, ident.Issuer, ident.Subject, ident.Email); err != nil {
		return "", err
	}
	s.notify(api.EventAuth, "Account created: "+u.Name, "First sign-in via "+s.oidc.Name()+".")
	return u.ID, nil
}

// syncOIDCAdmin aligns a linked user's admin flag with the current provider
// groups. It never removes the last admin, so a misconfigured group mapping
// cannot lock the instance out.
func (s *Server) syncOIDCAdmin(u api.User, ident oidc.Identity) {
	if !s.oidc.HasAdminGroups() {
		return
	}
	want := s.oidc.IsAdmin(ident.Groups)
	if u.IsAdmin == want {
		return
	}
	if !want {
		if n, err := s.store.CountAdmins(); err == nil && n <= 1 {
			return
		}
	}
	if err := s.store.SetUserAdmin(u.ID, want); err != nil {
		log.Printf("oidc: set admin for %s: %v", u.ID, err)
	}
}

func (s *Server) validOIDCState(r *http.Request, state string) bool {
	if state == "" {
		return false
	}
	c, err := r.Cookie(oidcStateCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(state)) == 1
}

func (s *Server) clearOIDCStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookieName, Value: "", Path: "/auth/oidc", MaxAge: -1,
		HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
}

// redirectOIDCError sends the browser back to the login page with a short code
// the SPA turns into a friendly message.
func (s *Server) redirectOIDCError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?error="+code, http.StatusFound)
}

// newUserID mints the random id used as a user's primary key and WebAuthn handle.
func newUserID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
