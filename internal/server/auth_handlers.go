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
	s.notifyAuth("Passkey enrolled", userID, "An invite link was redeemed and a passkey was enrolled.")
	s.issueSession(w, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleAddDeviceBegin starts enrolling an additional passkey for the logged-in
// user. No invite is needed; the ceremony is bound to the current session user.
func (s *Server) handleAddDeviceBegin(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	creds, err := s.store.CredentialsByUser(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	opts, cid, err := s.auth.BeginAddDevice(u.ID, u.Name, creds)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.setCeremonyCookie(w, cid)
	writeJSON(w, http.StatusOK, opts)
}

// handleAddDeviceFinish stores the new passkey against the logged-in user.
func (s *Server) handleAddDeviceFinish(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	cid, err := s.ceremonyID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.auth.FinishAddDevice(cid, u.ID, r); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.notifyAuth("Passkey added", u.ID, "An additional passkey was enrolled on this account.")
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
	s.notifyAuth("Signed in", userID, "A passkey was used to sign in.")
	s.issueSession(w, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// notifyAuth sends an account event, naming the user it happened to. The name
// is looked up rather than passed in because most call sites only have the id.
func (s *Server) notifyAuth(title, userID, detail string) {
	who := userID
	if u, err := s.store.GetUser(userID); err == nil {
		who = u.Name
	}
	s.notify(api.EventAuth, title+": "+who, detail)
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
	token, exp, err := auth.NewInvite(s.store, u.ID, "", req.IsAdmin)
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
	names := map[string]string{}
	if users, err := s.store.ListUsers(); err == nil {
		for _, u := range users {
			names[u.ID] = u.Name
		}
	}
	out := []api.Invite{}
	for _, iv := range rows {
		out = append(out, api.Invite{
			Token:       iv.Token,
			IsAdmin:     iv.IsAdmin,
			ExpiresAt:   iv.ExpiresAt,
			UsedAt:      iv.UsedAt,
			URL:         auth.InviteURL(s.cfg.Origin, iv.Token),
			ForUserName: names[iv.ForUser],
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleResetUser mints a single-use recovery invite bound to an existing user.
// Enrolling on the returned link adds a fresh passkey to that user, restoring
// access without changing their identity or admin status.
func (s *Server) handleResetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	u, err := s.store.GetUser(id)
	if err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "user not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	token, exp, err := auth.NewInvite(s.store, "", u.ID, u.IsAdmin)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, api.Invite{
		Token:       token,
		IsAdmin:     u.IsAdmin,
		ExpiresAt:   exp.Unix(),
		URL:         auth.InviteURL(s.cfg.Origin, token),
		ForUserName: u.Name,
	})
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if err := s.store.DeleteInvite(token); err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "invite not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	target, err := s.store.GetUser(id)
	if err != nil {
		if isNotFound(err) {
			writeErr(w, http.StatusNotFound, "user not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Refuse to remove the last admin so the instance can't be locked out.
	if target.IsAdmin {
		admins, err := s.store.CountAdmins()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		if admins <= 1 {
			writeErr(w, http.StatusBadRequest, "cannot remove the last admin")
			return
		}
	}
	if err := s.store.DeleteUser(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
