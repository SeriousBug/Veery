package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/notify"
)

// notifyReady guards handlers that need the notifier, which is absent only if
// the server was built without one (tests).
func (s *Server) notifyReady(w http.ResponseWriter) bool {
	if s.notif == nil {
		writeErr(w, http.StatusServiceUnavailable, "notifications unavailable")
		return false
	}
	return true
}

// handleGetNotifications returns the notification config. Admin-only: the URLs
// carry webhook tokens and passwords.
func (s *Server) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	if !s.notifyReady(w) {
		return
	}
	cfg, err := s.notif.Config()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutNotifications(w http.ResponseWriter, r *http.Request) {
	if !s.notifyReady(w) {
		return
	}
	var cfg api.NotificationConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := s.notif.Save(cfg); err != nil {
		if errors.Is(err, notify.ErrEnvManaged) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	out, err := s.notif.Config()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTestNotification delivers a test message and waits for the result, so a
// broken webhook URL surfaces as an error the admin can read.
func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	if !s.notifyReady(w) {
		return
	}
	var req api.TestNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := s.notif.SendTest(req.URLs); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
