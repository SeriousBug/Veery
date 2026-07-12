package server

import (
	"encoding/json"
	"net/http"

	"github.com/SeriousBug/Veery/internal/api"
)

// dockerReady guards handlers that need the Docker manager, returning 503 when
// it is absent (e.g. in unit tests or when the daemon is unavailable).
func (s *Server) dockerReady(w http.ResponseWriter) bool {
	if s.dkr == nil {
		writeErr(w, http.StatusServiceUnavailable, "docker unavailable")
		return false
	}
	return true
}

func (s *Server) handleListStacks(w http.ResponseWriter, r *http.Request) {
	if !s.dockerReady(w) {
		return
	}
	stacks, err := s.dkr.ListStacks(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stacks)
}

func (s *Server) handleAdoptStack(w http.ResponseWriter, r *http.Request) {
	if !s.dockerReady(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.dkr.Adopt(r.Context(), id); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleStackAction runs a job-wrapped stack lifecycle action. Progress is
// pushed over the WS; the HTTP call returns once the action completes.
func (s *Server) handleStackAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.dockerReady(w) {
			return
		}
		id := r.PathValue("id")
		ctx := r.Context()
		switch action {
		case "start":
			s.dkr.StartStackJob(ctx, id)
		case "stop":
			s.dkr.StopStackJob(ctx, id)
		case "restart":
			s.dkr.RestartStackJob(ctx, id)
		case "bringup":
			s.dkr.BringUpStackJob(ctx, id)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (s *Server) handleContainerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.dockerReady(w) {
			return
		}
		id := r.PathValue("id")
		ctx := r.Context()
		switch action {
		case "start":
			s.dkr.StartJob(ctx, id)
		case "stop":
			s.dkr.StopJob(ctx, id)
		case "restart":
			s.dkr.RestartJob(ctx, id)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (s *Server) handleContainerUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.dockerReady(w) {
		return
	}
	id := r.PathValue("id")
	s.dkr.Update(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetAutoUpdate(w http.ResponseWriter, r *http.Request) {
	var req api.SetAutoUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	mc, err := s.store.ResolveManaged(req.ContainerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "container not managed")
		return
	}
	if err := s.store.SetAutoUpdate(mc.ID, req.AutoUpdate); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.store.LoadSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var cfg api.Settings
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := s.store.SaveSettings(cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
