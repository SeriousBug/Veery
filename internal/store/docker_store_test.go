package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "veery.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func add(t *testing.T, st *Store, id, stack, name string) {
	t.Helper()
	if _, err := st.UpsertStack(stack); err != nil {
		t.Fatalf("upsert stack: %v", err)
	}
	if err := st.AddManagedContainer(ManagedContainer{
		ID: id, StackID: stack, ContainerName: name, SnapshotJSON: "{}", ContainerID: "c-" + id,
	}); err != nil {
		t.Fatalf("add managed: %v", err)
	}
}

// Forgetting one container of several leaves the rest of the stack managed.
func TestDeleteManagedContainerKeepsStack(t *testing.T) {
	st := testStore(t)
	add(t, st, "1", "blog", "blog-web")
	add(t, st, "2", "blog", "blog-db")

	if err := st.DeleteManagedContainer("1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := st.ManagedByName("blog-web"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected the container to be gone, got %v", err)
	}
	if _, err := st.ManagedByName("blog-db"); err != nil {
		t.Fatalf("expected its stack-mate to survive: %v", err)
	}
	if ok, err := st.StackExists("blog"); err != nil || !ok {
		t.Fatalf("expected the stack to survive, exists=%v err=%v", ok, err)
	}
}

// Forgetting the last container of a stack takes the stack with it: a service
// with no parts left is not something the UI can act on.
func TestDeleteManagedContainerDropsEmptyStack(t *testing.T) {
	st := testStore(t)
	add(t, st, "1", "blog", "blog-web")

	if err := st.DeleteManagedContainer("1"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if ok, err := st.StackExists("blog"); err != nil || ok {
		t.Fatalf("expected the stack to be gone, exists=%v err=%v", ok, err)
	}
}

func TestDeleteManagedContainerUnknown(t *testing.T) {
	st := testStore(t)
	if err := st.DeleteManagedContainer("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// The container id is what tells a snapshot apart from the container it was
// taken from, so it has to survive a round trip.
func TestUpdateSnapshotRecordsContainerID(t *testing.T) {
	st := testStore(t)
	add(t, st, "1", "blog", "blog-web")

	if err := st.UpdateSnapshot("1", `{"image":"nginx"}`, "newcontainer"); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}
	mc, err := st.ManagedByID("1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if mc.ContainerID != "newcontainer" || mc.SnapshotJSON != `{"image":"nginx"}` {
		t.Fatalf("snapshot not recorded: %+v", mc)
	}
}
