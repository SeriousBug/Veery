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
	c, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		return u, false
	}
	user, err := s.store.SessionUser(c.Value)
	if err != nil {
		return u, false
	}
	u.user = user
	u.token = c.Value
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

// setSessionCookie issues the HttpOnly session cookie.
func (s *Server) setSessionCookie(w http.ResponseWriter, token string, exp time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   s.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

const ceremonyCookieName = "veery_ceremony"

func (s *Server) setCeremonyCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     ceremonyCookieName,
		Value:    id,
		Path:     "/auth",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   s.cfg.Secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) ceremonyID(r *http.Request) (string, error) {
	c, err := r.Cookie(ceremonyCookieName)
	if err != nil {
		return "", errors.New("no ceremony in progress")
	}
	return c.Value, nil
}

// isNotFound reports a store miss.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
