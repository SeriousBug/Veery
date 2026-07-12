// Package server wires HTTP handlers, the SPA, auth middleware and the WS hub.
package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/auth"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/SeriousBug/Veery/web"
)

// Config holds server-wide settings derived from env.
type Config struct {
	RPID   string
	Origin string
	Secure bool
}

// Server holds shared dependencies for handlers.
type Server struct {
	store *store.Store
	auth  *auth.Manager
	cfg   Config
	spa   fs.FS
	hub   *Hub
	mux   *http.ServeMux
}

// New builds a Server with routes registered.
func New(st *store.Store, mgr *auth.Manager, cfg Config) *Server {
	s := &Server{
		store: st,
		auth:  mgr,
		cfg:   cfg,
		spa:   web.DistFS(),
		hub:   newHub(),
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

// Hub exposes the WS fan-out hub for producers (metrics, status, jobs).
func (s *Server) Hub() *Hub { return s.hub }

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// Auth (public).
	s.mux.HandleFunc("POST /auth/register/begin", s.handleRegisterBegin)
	s.mux.HandleFunc("POST /auth/register/finish", s.handleRegisterFinish)
	s.mux.HandleFunc("POST /auth/login/begin", s.handleLoginBegin)
	s.mux.HandleFunc("POST /auth/login/finish", s.handleLoginFinish)
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
	s.mux.HandleFunc("GET /auth/me", s.requireAuth(s.handleMe))

	// Invites (admin).
	s.mux.HandleFunc("GET /api/invites", s.requireAdmin(s.handleListInvites))
	s.mux.HandleFunc("POST /api/invites", s.requireAdmin(s.handleCreateInvite))
	s.mux.HandleFunc("GET /api/users", s.requireAdmin(s.handleListUsers))

	// Live push.
	s.mux.HandleFunc("GET /ws", s.handleWS)

	// SPA + static assets fallback.
	s.mux.HandleFunc("/", s.serveSPA)
}

// serveSPA serves embedded static files, falling back to index.html for client
// routes so deep links work.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.spa.Open(p); err == nil {
		f.Close()
		http.FileServer(http.FS(s.spa)).ServeHTTP(w, r)
		return
	}
	data, err := fs.ReadFile(s.spa, "index.html")
	if err != nil {
		http.Error(w, "SPA not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, api.APIError{Error: msg})
}

// userCtxKey carries the authenticated user through the request context.
type userCtxKey struct{}

func userFrom(ctx context.Context) (api.User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(api.User)
	return u, ok
}
