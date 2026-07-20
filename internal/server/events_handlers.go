package server

import (
	"net/http"
	"strconv"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
)

// handleListEvents returns a cursor-paginated page of the event log, newest
// first. It is admin-only: the log includes auth events that name users.
//
// Query params: cursor (opaque, from a page's nextCursor), limit, event (type),
// container and stack (service filters), q (search across title and body).
func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	page, err := s.store.ListEvents(store.EventQuery{
		Cursor:    q.Get("cursor"),
		Limit:     limit,
		Event:     api.NotificationEvent(q.Get("event")),
		Container: q.Get("container"),
		Stack:     q.Get("stack"),
		Search:    q.Get("q"),
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, page)
}
