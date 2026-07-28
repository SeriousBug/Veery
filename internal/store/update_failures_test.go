package store

import "testing"

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

// The attempt kind has to survive the handoff to the updater container, which
// is a different process reading the row back.
func TestUpdateJobCarriesAttempt(t *testing.T) {
	st := testStore(t)
	if err := st.StartUpdateJob(UpdateJob{
		ID: "j1", ContainerName: "web", Phase: "start", Auto: true, Target: "sha256:aaa",
	}); err != nil {
		t.Fatalf("start job: %v", err)
	}
	j, err := st.UpdateJobByID("j1")
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if !j.Auto || j.Target != "sha256:aaa" {
		t.Errorf("job auto=%v target=%q, want true/sha256:aaa", j.Auto, j.Target)
	}
}
