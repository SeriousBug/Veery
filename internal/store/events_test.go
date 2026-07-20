package store

import (
	"path/filepath"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
)

func openEventStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// appendAt records an event with a fixed timestamp so ordering is deterministic.
func appendAt(t *testing.T, st *Store, ev api.NotificationEvent, title, container string, at int64) api.Event {
	t.Helper()
	row, err := st.AppendEvent(api.Event{Event: ev, Title: title, ContainerName: container, CreatedAt: at})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	return row
}

func TestListEventsCursorPaging(t *testing.T) {
	st := openEventStore(t)
	for i := 0; i < 5; i++ {
		appendAt(t, st, api.EventContainerStatus, "e", "web", int64(100+i))
	}

	page1, err := st.ListEvents(EventQuery{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Items) != 2 {
		t.Fatalf("page1 items = %d, want 2", len(page1.Items))
	}
	// Newest first: created_at 104 then 103.
	if page1.Items[0].CreatedAt != 104 || page1.Items[1].CreatedAt != 103 {
		t.Fatalf("page1 order = %d,%d", page1.Items[0].CreatedAt, page1.Items[1].CreatedAt)
	}
	if page1.NextCursor == "" {
		t.Fatal("page1 nextCursor empty, want more")
	}

	page2, err := st.ListEvents(EventQuery{Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if page2.Items[0].CreatedAt != 102 || page2.Items[1].CreatedAt != 101 {
		t.Fatalf("page2 order = %d,%d", page2.Items[0].CreatedAt, page2.Items[1].CreatedAt)
	}

	page3, err := st.ListEvents(EventQuery{Limit: 2, Cursor: page2.NextCursor})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3.Items) != 1 || page3.Items[0].CreatedAt != 100 {
		t.Fatalf("page3 = %+v", page3.Items)
	}
	if page3.NextCursor != "" {
		t.Fatalf("page3 nextCursor = %q, want empty at end", page3.NextCursor)
	}
}

// Two events sharing a timestamp must not be skipped or duplicated: the cursor
// tie-breaks on id, so paging across the boundary sees each exactly once.
func TestListEventsCursorTieBreaksOnID(t *testing.T) {
	st := openEventStore(t)
	appendAt(t, st, api.EventAuth, "a", "", 200)
	appendAt(t, st, api.EventAuth, "b", "", 200)
	appendAt(t, st, api.EventAuth, "c", "", 200)

	seen := map[string]bool{}
	cursor := ""
	for range 3 {
		page, err := st.ListEvents(EventQuery{Limit: 1, Cursor: cursor})
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("items = %d", len(page.Items))
		}
		if seen[page.Items[0].Title] {
			t.Fatalf("duplicate row %q", page.Items[0].Title)
		}
		seen[page.Items[0].Title] = true
		cursor = page.NextCursor
	}
	if len(seen) != 3 {
		t.Fatalf("saw %d distinct rows, want 3", len(seen))
	}
}

func TestListEventsFilters(t *testing.T) {
	st := openEventStore(t)
	appendAt(t, st, api.EventContainerStatus, "web crashed", "web", 10)
	appendAt(t, st, api.EventUpdateApplied, "db updated", "db", 11)
	appendAt(t, st, api.EventAuth, "login", "", 12)

	byType, err := st.ListEvents(EventQuery{Event: api.EventUpdateApplied})
	if err != nil {
		t.Fatalf("byType: %v", err)
	}
	if len(byType.Items) != 1 || byType.Items[0].Title != "db updated" {
		t.Fatalf("byType = %+v", byType.Items)
	}

	byContainer, err := st.ListEvents(EventQuery{Container: "web"})
	if err != nil {
		t.Fatalf("byContainer: %v", err)
	}
	if len(byContainer.Items) != 1 || byContainer.Items[0].ContainerName != "web" {
		t.Fatalf("byContainer = %+v", byContainer.Items)
	}

	bySearch, err := st.ListEvents(EventQuery{Search: "updated"})
	if err != nil {
		t.Fatalf("bySearch: %v", err)
	}
	if len(bySearch.Items) != 1 || bySearch.Items[0].Title != "db updated" {
		t.Fatalf("bySearch = %+v", bySearch.Items)
	}
}

// A LIKE wildcard in the query must be matched literally, not as a wildcard.
func TestListEventsSearchEscapesWildcards(t *testing.T) {
	st := openEventStore(t)
	appendAt(t, st, api.EventContainerStatus, "disk 50% full", "web", 10)
	appendAt(t, st, api.EventContainerStatus, "all good", "web", 11)

	res, err := st.ListEvents(EventQuery{Search: "50%"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res.Items) != 1 || res.Items[0].Title != "disk 50% full" {
		t.Fatalf("search 50%% = %+v", res.Items)
	}
}

func TestPruneEventsOlderThan(t *testing.T) {
	st := openEventStore(t)
	appendAt(t, st, api.EventContainerStatus, "old", "web", 100)
	appendAt(t, st, api.EventContainerStatus, "new", "web", 200)

	n, err := st.PruneEventsOlderThan(150)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	page, err := st.ListEvents(EventQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "new" {
		t.Fatalf("remaining = %+v", page.Items)
	}

	// A non-positive cutoff keeps everything.
	if n, err := st.PruneEventsOlderThan(0); err != nil || n != 0 {
		t.Fatalf("prune(0) = %d, %v", n, err)
	}
}
