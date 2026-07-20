package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
)

// DefaultEventLimit is used when a page request does not ask for a size.
const DefaultEventLimit = 50

// MaxEventLimit caps a page so one request cannot pull the whole table.
const MaxEventLimit = 200

// EventQuery selects and pages the event log. The zero value returns the newest
// DefaultEventLimit events across every type and service.
type EventQuery struct {
	// Cursor is the opaque token from a previous page's NextCursor. Empty starts
	// at the newest event.
	Cursor string
	// Limit bounds the page size; clamped to [1, MaxEventLimit].
	Limit int
	// Event, when set, keeps only rows of that type.
	Event api.NotificationEvent
	// Container and Stack, when set, keep only rows naming that service.
	Container string
	Stack     string
	// Search, when set, keeps only rows whose title or body contains it (case
	// insensitive).
	Search string
}

// AppendEvent records one event and returns it with its assigned id and
// timestamp filled in. CreatedAt is set to now when zero.
func (s *Store) AppendEvent(ev api.Event) (api.Event, error) {
	if ev.CreatedAt == 0 {
		ev.CreatedAt = time.Now().Unix()
	}
	res, err := s.db.Exec(`INSERT INTO events(event,title,body,container_name,stack_id,created_at)
		VALUES(?,?,?,?,?,?)`,
		string(ev.Event), ev.Title, ev.Body, ev.ContainerName, ev.StackID, ev.CreatedAt)
	if err != nil {
		return ev, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ev, err
	}
	ev.ID = id
	return ev, nil
}

// ListEvents returns a page of events newest first, plus the cursor for the
// next (older) page. NextCursor is empty once the oldest event has been
// returned.
func (s *Store) ListEvents(q EventQuery) (api.EventPage, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultEventLimit
	}
	if limit > MaxEventLimit {
		limit = MaxEventLimit
	}

	var where []string
	var args []any
	if q.Event != "" {
		where = append(where, "event = ?")
		args = append(args, string(q.Event))
	}
	if q.Container != "" {
		where = append(where, "container_name = ?")
		args = append(args, q.Container)
	}
	if q.Stack != "" {
		where = append(where, "stack_id = ?")
		args = append(args, q.Stack)
	}
	if q.Search != "" {
		// LIKE is enough at the row counts a home server produces; the endpoint
		// can move to FTS5 behind the same signature if that ever changes.
		like := "%" + escapeLike(q.Search) + "%"
		where = append(where, "(title LIKE ? ESCAPE '\\' OR body LIKE ? ESCAPE '\\')")
		args = append(args, like, like)
	}
	if q.Cursor != "" {
		createdAt, id, err := decodeEventCursor(q.Cursor)
		if err != nil {
			return api.EventPage{}, err
		}
		where = append(where, "(created_at < ? OR (created_at = ? AND id < ?))")
		args = append(args, createdAt, createdAt, id)
	}

	query := `SELECT id,event,title,body,container_name,stack_id,created_at FROM events`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	// Fetch one extra row to tell whether an older page exists without a count.
	query += " ORDER BY created_at DESC, id DESC LIMIT ?"
	args = append(args, limit+1)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return api.EventPage{}, err
	}
	defer rows.Close()

	out := api.EventPage{Items: []api.Event{}}
	for rows.Next() {
		var ev api.Event
		var kind string
		if err := rows.Scan(&ev.ID, &kind, &ev.Title, &ev.Body, &ev.ContainerName, &ev.StackID, &ev.CreatedAt); err != nil {
			return api.EventPage{}, err
		}
		ev.Event = api.NotificationEvent(kind)
		out.Items = append(out.Items, ev)
	}
	if err := rows.Err(); err != nil {
		return api.EventPage{}, err
	}
	if len(out.Items) > limit {
		last := out.Items[limit-1]
		out.Items = out.Items[:limit]
		out.NextCursor = encodeEventCursor(last.CreatedAt, last.ID)
	}
	return out, nil
}

// PruneEventsOlderThan deletes events recorded before the cutoff unix time and
// returns how many were removed. A non-positive cutoff keeps everything.
func (s *Store) PruneEventsOlderThan(cutoff int64) (int64, error) {
	if cutoff <= 0 {
		return 0, nil
	}
	res, err := s.db.Exec(`DELETE FROM events WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// The cursor is (created_at, id), the tie-break pair the page orders on. It is
// not signed or opaque on purpose: it points at a row the caller already saw,
// so there is nothing to hide, and a plain form keeps it debuggable.
func encodeEventCursor(createdAt, id int64) string {
	return fmt.Sprintf("%d_%d", createdAt, id)
}

func decodeEventCursor(cursor string) (createdAt, id int64, err error) {
	a, b, ok := strings.Cut(cursor, "_")
	if !ok {
		return 0, 0, fmt.Errorf("bad cursor %q", cursor)
	}
	createdAt, err = strconv.ParseInt(a, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad cursor %q", cursor)
	}
	id, err = strconv.ParseInt(b, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad cursor %q", cursor)
	}
	return createdAt, id, nil
}

// escapeLike neutralizes the LIKE wildcards so a search for "50%" does not match
// everything. Paired with ESCAPE '\' in the query.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
