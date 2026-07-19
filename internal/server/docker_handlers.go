package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/metrics"
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

// handleForgetContainer drops a container Veery manages but that no longer
// exists on the host, which is what removing a service from a compose file
// leaves behind. Nothing is deleted on the host; there is nothing left to
// delete. Veery stops tracking it, so it stops being reported as missing and
// bring-up stops recreating it.
func (s *Server) handleForgetContainer(w http.ResponseWriter, r *http.Request) {
	if !s.dockerReady(w) {
		return
	}
	mc, err := s.store.ResolveManaged(r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "container not managed")
		return
	}
	if err := s.store.DeleteManagedContainer(mc.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.dkr.BroadcastStacks(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleForgetStack stops managing a whole stack. Its containers, if any are
// still on the host, keep running: this drops Veery's records, not the service.
func (s *Server) handleForgetStack(w http.ResponseWriter, r *http.Request) {
	if !s.dockerReady(w) {
		return
	}
	if err := s.store.Unadopt(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.dkr.BroadcastStacks(r.Context())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleStackAction runs a job-wrapped stack lifecycle action. The action runs
// detached and reports over the WS; the HTTP call returns immediately.
func (s *Server) handleStackAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.dockerReady(w) {
			return
		}
		id := r.PathValue("id")
		ctx := detached(r)
		switch action {
		case "start":
			go s.dkr.StartStackJob(ctx, id)
		case "stop":
			go s.dkr.StopStackJob(ctx, id)
		case "restart":
			go s.dkr.RestartStackJob(ctx, id)
		case "bringup":
			go s.dkr.BringUpStackJob(ctx, id)
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
		ctx := detached(r)
		switch action {
		case "start":
			go s.dkr.StartJob(ctx, id)
		case "stop":
			go s.dkr.StopJob(ctx, id)
		case "restart":
			go s.dkr.RestartJob(ctx, id)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

func (s *Server) handleContainerUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.dockerReady(w) {
		return
	}
	id := r.PathValue("id")
	go s.dkr.Update(detached(r), id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// detached derives a context that outlives the request. A pull and health-check
// can run for minutes, and an update aborted halfway because someone closed a
// tab is exactly the state that leaves a service down. Progress reaches the UI
// over the WS, not this response.
func detached(r *http.Request) context.Context {
	return context.WithoutCancel(r.Context())
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

func (s *Server) handleListDisks(w http.ResponseWriter, r *http.Request) {
	overrides, err := s.store.LoadDiskVisibility()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	mounts, devices := metrics.Enumerate()
	items := metrics.BuildDiskItems(mounts, devices, overrides)
	if items == nil {
		items = []api.DiskItem{}
	}
	writeJSON(w, http.StatusOK, items)
}

// handleStartMdadmScan starts a data-scrub (check) on a RAID array. The
// refreshed status arrives on the next WS metrics tick, so there is no body.
func (s *Server) handleStartMdadmScan(w http.ResponseWriter, r *http.Request) {
	if err := metrics.StartMdadmCheck(r.PathValue("name")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleSetDiskVisibility(w http.ResponseWriter, r *http.Request) {
	var req api.SetDiskVisibilityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if req.Visibility == nil {
		req.Visibility = map[string]bool{}
	}
	if err := s.store.SaveDiskVisibility(req.Visibility); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.handleListDisks(w, r)
}
