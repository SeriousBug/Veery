package server

import (
	"encoding/json"
	"net/http"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/auth"
)

func (s *Server) handleRegisterBegin(w http.ResponseWriter, r *http.Request) {
	var req api.EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	opts, cid, _, err := s.auth.BeginRegistration(req.Token, req.Name)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.setCeremonyCookie(w, cid)
	// Stash the invite token in a cookie so finish can consume it.
	http.SetCookie(w, &http.Cookie{
		Name: "veery_invite", Value: req.Token, Path: "/auth", MaxAge: 300,
		HttpOnly: true, Secure: s.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) handleRegisterFinish(w http.ResponseWriter, r *http.Request) {
	cid, err := s.ceremonyID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inviteCookie, err := r.Cookie("veery_invite")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "no invite in progress")
		return
	}
	userID, err := s.auth.FinishRegistration(cid, inviteCookie.Value, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.issueSession(w, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLoginBegin(w http.ResponseWriter, r *http.Request) {
	opts, cid, err := s.auth.BeginLogin()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setCeremonyCookie(w, cid)
	writeJSON(w, http.StatusOK, opts)
}

func (s *Server) handleLoginFinish(w http.ResponseWriter, r *http.Request) {
	cid, err := s.ceremonyID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	userID, err := s.auth.FinishLogin(cid, r)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err.Error())
		return
	}
	s.issueSession(w, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) issueSession(w http.ResponseWriter, userID string) {
	token, exp, err := auth.NewSession(s.store, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	s.setSessionCookie(w, token, exp)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookieName); err == nil {
		s.store.DeleteSession(c.Value)
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	creds, err := s.store.CredentialsByUser(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := api.SessionInfo{User: u}
	for _, c := range creds {
		out.Credentials = append(out.Credentials, api.Credential{
			ID: c.ID, Name: c.Name, CreatedAt: c.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	var req api.CreateInviteRequest
	json.NewDecoder(r.Body).Decode(&req)
	token, exp, err := auth.NewInvite(s.store, u.ID, req.IsAdmin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, api.Invite{
		Token:     token,
		IsAdmin:   req.IsAdmin,
		ExpiresAt: exp.Unix(),
		URL:       auth.InviteURL(s.cfg.Origin, token),
	})
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListPendingInvites()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := []api.Invite{}
	for _, iv := range rows {
		out = append(out, api.Invite{
			Token:     iv.Token,
			IsAdmin:   iv.IsAdmin,
			ExpiresAt: iv.ExpiresAt,
			UsedAt:    iv.UsedAt,
			URL:       auth.InviteURL(s.cfg.Origin, iv.Token),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.store.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if users == nil {
		users = []api.User{}
	}
	writeJSON(w, http.StatusOK, users)
}
