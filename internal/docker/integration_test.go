package docker

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
)

// TestIntegrationLifecycle exercises the Manager against the real local Docker
// daemon. It is skipped when no daemon is reachable so CI without docker passes.
func TestIntegrationLifecycle(t *testing.T) {
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "veery.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	m, err := NewManager(st, nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer m.Close()

	if err := m.Ping(ctx); err != nil {
		t.Skipf("docker daemon unreachable, skipping: %v", err)
	}

	const project = "veerytest"
	name := fmt.Sprintf("veerytest-%d", time.Now().UnixNano())

	ensureImage(t, m, ctx, "busybox:latest")

	created, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"sh", "-c", "sleep 3600"},
		Labels: map[string]string{
			projectLabel: project,
			serviceLabel: "sleeper",
		},
	}, &container.HostConfig{}, nil, nil, name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	cid := created.ID
	t.Cleanup(func() {
		_ = m.cli.ContainerRemove(context.Background(), cid, container.RemoveOptions{Force: true})
		_ = st.Unadopt(project)
	})
	if err := m.cli.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}

	// Discovery.
	stacks, err := m.ListStacks(ctx)
	if err != nil {
		t.Fatalf("list stacks: %v", err)
	}
	found := false
	for _, s := range stacks {
		if s.ID == project {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("stack %q not found in discovery", project)
	}

	// Adoption.
	if err := m.Adopt(ctx, project); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if _, err := m.st.ManagedByName(name); err != nil {
		t.Fatalf("container not persisted as managed: %v", err)
	}

	// Lifecycle.
	if err := m.Stop(ctx, cid); err != nil {
		t.Fatalf("stop: %v", err)
	}
	assertRunning(t, m, ctx, name, false)
	if err := m.Start(ctx, cid); err != nil {
		t.Fatalf("start: %v", err)
	}
	assertRunning(t, m, ctx, name, true)
	if err := m.Restart(ctx, cid); err != nil {
		t.Fatalf("restart: %v", err)
	}
	assertRunning(t, m, ctx, name, true)

	// Remove the container out from under the manager, then bring the stack
	// back up: it should be recreated from the snapshot.
	if err := m.cli.ContainerRemove(ctx, cid, container.RemoveOptions{Force: true}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := m.BringUpStack(ctx, project); err != nil {
		t.Fatalf("bring up stack: %v", err)
	}
	assertRunning(t, m, ctx, name, true)

	// Update cleanup handle to the recreated container.
	insp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		t.Fatalf("inspect recreated: %v", err)
	}
	newCID := insp.ID
	t.Cleanup(func() {
		_ = m.cli.ContainerRemove(context.Background(), newCID, container.RemoveOptions{Force: true})
	})
}

// TestIntegrationUpdateRollback verifies the transactional update path: pointing
// a managed container's snapshot at an image that exits immediately must fail
// verification and roll back to the original, still-running container.
func TestIntegrationUpdateRollback(t *testing.T) {
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "veery.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	m, err := NewManager(st, nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer m.Close()

	if err := m.Ping(ctx); err != nil {
		t.Skipf("docker daemon unreachable, skipping: %v", err)
	}

	const project = "veeryrollback"
	name := fmt.Sprintf("veeryrollback-%d", time.Now().UnixNano())

	ensureImage(t, m, ctx, "busybox:latest")
	ensureImage(t, m, ctx, "alpine:latest")

	created, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"sh", "-c", "sleep 3600"},
		Labels: map[string]string{
			projectLabel: project,
			serviceLabel: "sleeper",
		},
	}, &container.HostConfig{}, nil, nil, name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	cid := created.ID
	t.Cleanup(func() {
		_ = m.cli.ContainerRemove(context.Background(), cid, container.RemoveOptions{Force: true})
		_ = m.cli.ContainerRemove(context.Background(), name, container.RemoveOptions{Force: true})
		_ = m.cli.ContainerRemove(context.Background(), name+oldSuffix, container.RemoveOptions{Force: true})
		_ = st.Unadopt(project)
	})
	if err := m.cli.ContainerStart(ctx, cid, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}

	if err := m.Adopt(ctx, project); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	mc, err := m.st.ManagedByName(name)
	if err != nil {
		t.Fatalf("managed lookup: %v", err)
	}

	origInsp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		t.Fatalf("inspect original: %v", err)
	}
	origImageID := origInsp.Image

	// Point the snapshot at a different image whose command exits immediately so
	// the recreated container fails verification.
	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	snap.Image = "alpine:latest"
	snap.Config.Image = "alpine:latest"
	snap.Config.Cmd = []string{"sh", "-c", "exit 1"}
	js, err := snap.marshal()
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := m.st.UpdateSnapshot(mc.ID, js); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}
	mc, _ = m.st.ManagedByID(mc.ID)

	_, err = m.doUpdate(ctx, mc, genID(), func(phase, msg string) {})
	if err == nil {
		t.Fatalf("expected update to fail and roll back, got nil error")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got: %v", err)
	}

	// The original container must be back under its name, running, on its
	// original image, with no parked leftover.
	restored, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		t.Fatalf("inspect restored: %v", err)
	}
	if !restored.State.Running {
		t.Fatalf("original container not running after rollback (state %s)", restored.State.Status)
	}
	if restored.Image != origImageID {
		t.Fatalf("image not restored: got %s want %s", restored.Image, origImageID)
	}
	if _, err := m.cli.ContainerInspect(ctx, name+oldSuffix); err == nil {
		t.Fatalf("parked container %s should have been removed after rollback", name+oldSuffix)
	}
}

func ensureImage(t *testing.T, m *Manager, ctx context.Context, ref string) {
	t.Helper()
	if _, err := m.cli.ImageInspect(ctx, ref); err == nil {
		return
	}
	rc, err := m.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		t.Fatalf("pull %s: %v", ref, err)
	}
	_, _ = io.Copy(io.Discard, rc)
	rc.Close()
}

func assertRunning(t *testing.T, m *Manager, ctx context.Context, name string, want bool) {
	t.Helper()
	insp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		if want {
			t.Fatalf("inspect %s: %v", name, err)
		}
		return
	}
	if insp.State.Running != want {
		t.Fatalf("container %s running=%v, want %v", name, insp.State.Running, want)
	}
}
