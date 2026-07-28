package store

import (
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
)

func TestRecordUpdateFailureCountsPerVersion(t *testing.T) {
	st := testStore(t)

	for i := 1; i <= 3; i++ {
		row, err := st.RecordUpdateFailure("web", "sha256:aaa", "exited (1)")
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if row.Failures != i {
			t.Errorf("after %d failures count = %d, want %d", i, row.Failures, i)
		}
	}
	// A different version starts its own count: a bad release must not poison
	// the one that comes after it.
	row, err := st.RecordUpdateFailure("web", "sha256:bbb", "unhealthy")
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if row.Failures != 1 {
		t.Errorf("new version count = %d, want 1", row.Failures)
	}
	if row.LastError != "unhealthy" {
		t.Errorf("last error = %q, want %q", row.LastError, "unhealthy")
	}

	rows, err := st.UpdateFailures("web")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
}

func TestUpdateFailuresAreScopedToContainer(t *testing.T) {
	st := testStore(t)
	if _, err := st.RecordUpdateFailure("web", "sha256:aaa", "boom"); err != nil {
		t.Fatalf("record: %v", err)
	}
	rows, err := st.UpdateFailures("db")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows for another container, want 0", len(rows))
	}
}

func TestClearUpdateFailures(t *testing.T) {
	st := testStore(t)
	if _, err := st.RecordUpdateFailure("web", "sha256:aaa", "boom"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := st.ClearUpdateFailures("web"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rows, err := st.UpdateFailures("web")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows after clear, want 0", len(rows))
	}
}

// A container that is no longer managed must not leave its failure counts
// behind for whatever takes its name next.
func TestDeleteManagedContainerDropsUpdateFailures(t *testing.T) {
	st := testStore(t)
	add(t, st, "m1", "web", "web-1")
	if _, err := st.RecordUpdateFailure("web-1", "sha256:aaa", "boom"); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := st.DeleteManagedContainer("m1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows, err := st.UpdateFailures("web-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows after delete, want 0", len(rows))
	}
}

// Who asked for the update, and which version it installs, have to survive the
// handoff to the updater container, which is a different process reading the
// row back.
func TestUpdateJobCarriesSourceAndTarget(t *testing.T) {
	st := testStore(t)
	if err := st.StartUpdateJob(UpdateJob{
		ID: "j1", ContainerName: "web", Phase: "start",
		Source: api.SourceAutomation, Target: "sha256:aaa",
	}); err != nil {
		t.Fatalf("start job: %v", err)
	}
	j, err := st.UpdateJobByID("j1")
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if j.Source != api.SourceAutomation || j.Target != "sha256:aaa" {
		t.Errorf("job source=%q target=%q, want %q/sha256:aaa", j.Source, j.Target, api.SourceAutomation)
	}
}

// Auto-update off because the user turned it off, and off because Veery gave
// up on it, are different states: the UI has to be able to say which.
func TestAutoUpdateRecordsWhoTurnedItOff(t *testing.T) {
	st := testStore(t)
	add(t, st, "m1", "web", "web-1")

	if err := st.SetAutoUpdate("m1", true, api.SourceUser); err != nil {
		t.Fatalf("set auto-update: %v", err)
	}
	if err := st.SetAutoUpdate("m1", false, api.SourceAutomation); err != nil {
		t.Fatalf("stop auto-update: %v", err)
	}
	mc, err := st.ManagedByID("m1")
	if err != nil {
		t.Fatalf("load managed: %v", err)
	}
	if mc.AutoUpdate || mc.AutoUpdateSource != api.SourceAutomation {
		t.Errorf("after Veery stopped it: autoUpdate=%v source=%q, want false/%q", mc.AutoUpdate, mc.AutoUpdateSource, api.SourceAutomation)
	}
	// The container is no longer polled either way.
	list, err := st.AutoUpdateContainers()
	if err != nil {
		t.Fatalf("list auto-update: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("got %d auto-updating containers, want 0", len(list))
	}

	// Turning it back on is the user taking it over again.
	if err := st.SetAutoUpdate("m1", true, api.SourceUser); err != nil {
		t.Fatalf("set auto-update: %v", err)
	}
	mc, err = st.ManagedByID("m1")
	if err != nil {
		t.Fatalf("load managed: %v", err)
	}
	if !mc.AutoUpdate || mc.AutoUpdateSource != api.SourceUser {
		t.Errorf("after the user turned it on: autoUpdate=%v source=%q, want true/%q", mc.AutoUpdate, mc.AutoUpdateSource, api.SourceUser)
	}

	// The user turning it off is their own choice, and must not read as Veery
	// giving up on it.
	if err := st.SetAutoUpdate("m1", false, api.SourceUser); err != nil {
		t.Fatalf("set auto-update: %v", err)
	}
	mc, err = st.ManagedByID("m1")
	if err != nil {
		t.Fatalf("load managed: %v", err)
	}
	if mc.AutoUpdate || mc.AutoUpdateSource != api.SourceUser {
		t.Errorf("after the user turned it off: autoUpdate=%v source=%q, want false/%q", mc.AutoUpdate, mc.AutoUpdateSource, api.SourceUser)
	}
}
