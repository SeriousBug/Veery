package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/auth"
	"github.com/SeriousBug/Veery/internal/store"
)

// currentUser resolves the session cookie to a user, or returns ok=false.
func (s *Server) currentUser(r *http.Request) (u userFromSession, ok bool) {
	token, ok := s.cookie(r, auth.SessionCookieName)
	if !ok {
		return u, false
	}
	user, err := s.store.SessionUser(token)
	if err != nil {
		return u, false
	}
	u.user = user
	u.token = token
	return u, true
}

type userFromSession struct {
	user  api.User
	token string
}

// requireAuth gates a handler behind a valid session.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey{}, u.user)
		next(w, r.WithContext(ctx))
	}
}

// requireAdmin gates a handler behind a valid admin session.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := s.currentUser(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if !u.user.IsAdmin {
			writeErr(w, http.StatusForbidden, "admin only")
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey{}, u.user)
		next(w, r.WithContext(ctx))
	}
}

// cookieName adds the __Host- prefix under HTTPS. Browsers then reject a cookie
// of that name set by a sibling subdomain (Domain=.example.com), so a same-site
// page cannot plant its own session or flow state in a victim's browser.
func (s *Server) cookieName(base string) string {
	if s.cfg.Secure {
		return "__Host-" + base
	}
	return base
}

// setCookie issues an HttpOnly cookie. __Host- cookies must use Path=/.
func (s *Server) setCookie(w http.ResponseWriter, base, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookieName(base),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter, base string) {
	s.setCookie(w, base, "", -1)
}

func (s *Server) cookie(r *http.Request, base string) (string, bool) {
	c, err := r.Cookie(s.cookieName(base))
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string, exp time.Time) {
	s.setCookie(w, auth.SessionCookieName, token, int(time.Until(exp).Seconds()))
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	s.clearCookie(w, auth.SessionCookieName)
}

const (
	ceremonyCookieName = "veery_ceremony"
	inviteCookieName   = "veery_invite"
)

func (s *Server) setCeremonyCookie(w http.ResponseWriter, id string) {
	s.setCookie(w, ceremonyCookieName, id, 300)
}

func (s *Server) ceremonyID(r *http.Request) (string, error) {
	id, ok := s.cookie(r, ceremonyCookieName)
	if !ok {
		return "", errors.New("no ceremony in progress")
	}
	return id, nil
}

// isNotFound reports a store miss.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
