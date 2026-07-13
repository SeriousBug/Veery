package docker

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
)

// reconcileFixture is a managed one-container stack on the real daemon, standing
// in for a compose project the user owns.
type reconcileFixture struct {
	m       *Manager
	st      *store.Store
	project string
	name    string
}

func newReconcileFixture(t *testing.T, ctx context.Context, env []string) *reconcileFixture {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "veery.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	m, err := NewManager(st, nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(func() { m.Close() })
	if err := m.Ping(ctx); err != nil {
		t.Skipf("docker daemon unreachable, skipping: %v", err)
	}

	f := &reconcileFixture{
		m:       m,
		st:      st,
		project: fmt.Sprintf("veeryrec%d", time.Now().UnixNano()),
	}
	f.name = f.project + "-app"
	ensureImage(t, m, ctx, "busybox:latest")

	f.create(t, ctx, "busybox:latest", env)
	if err := m.Adopt(ctx, f.project); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	t.Cleanup(func() {
		_ = m.cli.ContainerRemove(context.Background(), f.name, container.RemoveOptions{Force: true})
		_ = st.Unadopt(f.project)
	})
	return f
}

// create makes the container the way the user's compose file would: same name,
// same project, whatever spec they wrote this time.
func (f *reconcileFixture) create(t *testing.T, ctx context.Context, image string, env []string) string {
	t.Helper()
	created, err := f.m.cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Cmd:   []string{"sh", "-c", "sleep 3600"},
		Env:   env,
		Labels: map[string]string{
			projectLabel: f.project,
			serviceLabel: "app",
		},
	}, &container.HostConfig{}, nil, nil, f.name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	if err := f.m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	return created.ID
}

// recreate is `docker compose up -d` after an edit: the old container is
// removed and a new one takes its name, carrying the new spec.
func (f *reconcileFixture) recreate(t *testing.T, ctx context.Context, image string, env []string) string {
	t.Helper()
	if err := f.m.cli.ContainerRemove(ctx, f.name, container.RemoveOptions{Force: true}); err != nil {
		t.Fatalf("remove container: %v", err)
	}
	return f.create(t, ctx, image, env)
}

func (f *reconcileFixture) snapshot(t *testing.T) (store.ManagedContainer, Snapshot) {
	t.Helper()
	mc, err := f.st.ManagedByName(f.name)
	if err != nil {
		t.Fatalf("load managed: %v", err)
	}
	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	return mc, snap
}

func hasEnv(snap Snapshot, want string) bool {
	for _, e := range snap.Config.Env {
		if e == want {
			return true
		}
	}
	return false
}

// The user edits their compose file and brings the stack back up. Veery has to
// notice the container is not the one it snapshotted and record the new spec,
// or the next update will recreate the container from the spec it replaced.
func TestReconcileRecordsExternalRecreate(t *testing.T) {
	ctx := context.Background()
	f := newReconcileFixture(t, ctx, []string{"MODE=old"})

	_, snap := f.snapshot(t)
	if !hasEnv(snap, "MODE=old") {
		t.Fatalf("adopt should have captured the original env, got %v", snap.Config.Env)
	}

	newID := f.recreate(t, ctx, "busybox:latest", []string{"MODE=new"})
	f.m.Reconcile(ctx)

	mc, snap := f.snapshot(t)
	if !hasEnv(snap, "MODE=new") {
		t.Fatalf("snapshot still holds the old spec: %v", snap.Config.Env)
	}
	if mc.ContainerID != newID {
		t.Fatalf("container id not re-recorded: got %q, want %q", mc.ContainerID, newID)
	}
}

// The spec of a container that cannot run is not worth keeping: bring-up and
// update rollback build from the snapshot, and handing them a broken spec is
// how a service stays down.
func TestReconcileKeepsSpecOfContainerThatWontRun(t *testing.T) {
	ctx := context.Background()
	f := newReconcileFixture(t, ctx, []string{"MODE=old"})

	if err := f.m.cli.ContainerRemove(ctx, f.name, container.RemoveOptions{Force: true}); err != nil {
		t.Fatalf("remove container: %v", err)
	}
	created, err := f.m.cli.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"sh", "-c", "exit 1"},
		Env:   []string{"MODE=broken"},
		Labels: map[string]string{
			projectLabel: f.project,
			serviceLabel: "app",
		},
	}, &container.HostConfig{}, nil, nil, f.name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	if err := f.m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	waitExited(t, ctx, f.m, created.ID)

	f.m.Reconcile(ctx)

	_, snap := f.snapshot(t)
	if !hasEnv(snap, "MODE=old") {
		t.Fatalf("a container that exited 1 should not have replaced the known-good spec, got %v", snap.Config.Env)
	}
}

// An update must never undo a change the user made to their compose file. The
// container here has been moved to a different image, so the update has nothing
// to do; the bug this guards against is Veery pulling the image the snapshot
// remembers and recreating the container on it.
func TestUpdateDoesNotRevertAnExternalChange(t *testing.T) {
	ctx := context.Background()
	f := newReconcileFixture(t, ctx, []string{"MODE=old"})
	ensureImage(t, f.m, ctx, "alpine:latest")

	f.recreate(t, ctx, "alpine:latest", []string{"MODE=new"})

	mc, _ := f.snapshot(t)
	updated, err := f.m.doUpdate(ctx, mc, genID(), func(string, string) {})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated {
		t.Fatalf("the container is already on the image its compose file asks for; nothing to update")
	}

	insp, err := f.m.cli.ContainerInspect(ctx, f.name)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	img, err := f.m.cli.ImageInspect(ctx, "alpine:latest")
	if err != nil {
		t.Fatalf("inspect image: %v", err)
	}
	if insp.Image != img.ID {
		t.Fatalf("the update moved the container off the image the user chose")
	}
	if _, snap := f.snapshot(t); snap.Image != "alpine:latest" {
		t.Fatalf("snapshot still points at the old image: %q", snap.Image)
	}
}

// A service added to a compose file is a container Veery has never seen, in a
// stack it already manages. Nothing else would ever adopt it.
func TestReconcileAdoptsNewContainerInManagedStack(t *testing.T) {
	ctx := context.Background()
	f := newReconcileFixture(t, ctx, nil)

	sidecar := f.project + "-sidecar"
	created, err := f.m.cli.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"sh", "-c", "sleep 3600"},
		Labels: map[string]string{
			projectLabel: f.project,
			serviceLabel: "sidecar",
		},
	}, &container.HostConfig{}, nil, nil, sidecar)
	if err != nil {
		t.Fatalf("create sidecar: %v", err)
	}
	t.Cleanup(func() {
		_ = f.m.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	})
	if err := f.m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start sidecar: %v", err)
	}

	f.m.Reconcile(ctx)

	mc, err := f.st.ManagedByName(sidecar)
	if err != nil {
		t.Fatalf("the new container was not adopted: %v", err)
	}
	if mc.StackID != f.project || mc.ContainerID != created.ID {
		t.Fatalf("adopted into the wrong place: %+v", mc)
	}
}

// A container outside every managed stack stays the user's to adopt.
func TestReconcileLeavesUnmanagedContainersAlone(t *testing.T) {
	ctx := context.Background()
	f := newReconcileFixture(t, ctx, nil)

	other := fmt.Sprintf("veeryrec-loner-%d", time.Now().UnixNano())
	created, err := f.m.cli.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"sh", "-c", "sleep 3600"},
	}, &container.HostConfig{}, nil, nil, other)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		_ = f.m.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	})

	f.m.Reconcile(ctx)

	if _, err := f.st.ManagedByName(other); err == nil {
		t.Fatalf("a container in no managed stack must not be adopted on its own")
	}
}

func waitExited(t *testing.T, ctx context.Context, m *Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		insp, err := m.cli.ContainerInspect(ctx, id)
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if insp.State != nil && !insp.State.Running && !insp.State.Restarting {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("container %s never exited", id)
}
