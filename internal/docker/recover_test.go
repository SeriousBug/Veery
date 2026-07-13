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

// recoverFixture stands up a managed, running busybox container against the real
// daemon, then parks it exactly the way an interrupted swap would: renamed to
// the oldSuffix name and stopped. What it leaves behind under the original name
// is up to the caller, which is the whole point of the recovery decision.
type recoverFixture struct {
	m      *Manager
	st     *store.Store
	name   string
	parked string
	ctx    context.Context
}

func newRecoverFixture(t *testing.T) *recoverFixture {
	t.Helper()
	ctx := context.Background()

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

	ensureImage(t, m, ctx, "busybox:latest")

	project := "veeryrecover"
	name := fmt.Sprintf("veeryrecover-%d", time.Now().UnixNano())
	parked := name + oldSuffix

	created, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image:  "busybox:latest",
		Cmd:    []string{"sh", "-c", "sleep 3600"},
		Labels: map[string]string{projectLabel: project, serviceLabel: "sleeper"},
	}, &container.HostConfig{}, nil, nil, name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = m.cli.ContainerRemove(bg, name, container.RemoveOptions{Force: true})
		_ = m.cli.ContainerRemove(bg, parked, container.RemoveOptions{Force: true})
		_ = st.Unadopt(project)
	})
	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	if err := m.Adopt(ctx, project); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	// Park it: this is the state a crash between the rename and the verify leaves
	// on the host.
	if err := m.cli.ContainerRename(ctx, name, parked); err != nil {
		t.Fatalf("park: %v", err)
	}
	if err := m.cli.ContainerStop(ctx, parked, container.StopOptions{}); err != nil {
		t.Fatalf("stop parked: %v", err)
	}

	return &recoverFixture{m: m, st: st, name: name, parked: parked, ctx: ctx}
}

// startReplacement creates a container under the original name, standing in for
// the new container a swap had gotten as far as creating. cmd decides whether it
// comes up healthy or dies.
func (f *recoverFixture) startReplacement(t *testing.T, cmd string) string {
	t.Helper()
	created, err := f.m.cli.ContainerCreate(f.ctx, &container.Config{
		Image: "busybox:latest",
		Cmd:   []string{"sh", "-c", cmd},
	}, &container.HostConfig{}, nil, nil, f.name)
	if err != nil {
		t.Fatalf("create replacement: %v", err)
	}
	if err := f.m.cli.ContainerStart(f.ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start replacement: %v", err)
	}
	return created.ID
}

func (f *recoverFixture) mustInspect(t *testing.T, name string) container.InspectResponse {
	t.Helper()
	insp, err := f.m.cli.ContainerInspect(f.ctx, name)
	if err != nil {
		t.Fatalf("inspect %s: %v", name, err)
	}
	return insp
}

func (f *recoverFixture) mustBeGone(t *testing.T, name string) {
	t.Helper()
	if _, err := f.m.cli.ContainerInspect(f.ctx, name); err == nil {
		t.Fatalf("container %s should have been removed", name)
	}
}

// A crash between parking the old container and creating the new one leaves the
// service down with nothing under its name. Recovery must put it back.
func TestRecoverRestoresWhenNoReplacementExists(t *testing.T) {
	f := newRecoverFixture(t)

	f.m.Recover(f.ctx)

	insp := f.mustInspect(t, f.name)
	if !insp.State.Running {
		t.Fatalf("container not running after recovery (state %s)", insp.State.Status)
	}
	f.mustBeGone(t, f.parked)
}

// A crash during verification can leave a replacement that never came up. It has
// to be held to the same bar a completed update would have applied: roll back.
func TestRecoverRollsBackUnhealthyReplacement(t *testing.T) {
	f := newRecoverFixture(t)
	broken := f.startReplacement(t, "exit 1")

	f.m.Recover(f.ctx)

	insp := f.mustInspect(t, f.name)
	if !insp.State.Running {
		t.Fatalf("original not running after rollback (state %s)", insp.State.Status)
	}
	if insp.ID == broken {
		t.Fatal("the dead replacement is still holding the name")
	}
	f.mustBeGone(t, f.parked)
}

// If the replacement is up and healthy the update effectively succeeded, so the
// parked container is just garbage to collect.
func TestRecoverKeepsHealthyReplacement(t *testing.T) {
	f := newRecoverFixture(t)
	fresh := f.startReplacement(t, "sleep 3600")

	f.m.Recover(f.ctx)

	insp := f.mustInspect(t, f.name)
	if insp.ID != fresh {
		t.Fatalf("healthy replacement was replaced: got %s want %s", insp.ID[:12], fresh[:12])
	}
	if !insp.State.Running {
		t.Fatal("replacement should still be running")
	}
	f.mustBeGone(t, f.parked)
}

// A running helper owns the swap, including the parked container it may still
// need to roll back to. Recovery must not touch anything underneath it.
func TestRecoverLeavesSwapAloneWhileUpdaterRuns(t *testing.T) {
	f := newRecoverFixture(t)

	updater := "veery-updater-test-" + fmt.Sprint(time.Now().UnixNano())
	created, err := f.m.cli.ContainerCreate(f.ctx, &container.Config{
		Image:  "busybox:latest",
		Cmd:    []string{"sh", "-c", "sleep 60"},
		Labels: map[string]string{updaterLabel: updaterRole, jobLabel: "job-1"},
	}, &container.HostConfig{}, nil, nil, updater)
	if err != nil {
		t.Fatalf("create updater: %v", err)
	}
	t.Cleanup(func() {
		_ = f.m.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
	})
	if err := f.m.cli.ContainerStart(f.ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start updater: %v", err)
	}

	f.m.Recover(f.ctx)

	// The parked container is the helper's rollback target; tearing it out would
	// strand the service if the new image turns out to be bad.
	if _, err := f.m.cli.ContainerInspect(f.ctx, f.parked); err != nil {
		t.Fatal("recovery removed the parked container while an updater was still running")
	}
}

// An unfinished job row whose worker is gone must be settled, not left spinning.
func TestRecoverSettlesOrphanedJob(t *testing.T) {
	f := newRecoverFixture(t)
	f.startReplacement(t, "sleep 3600")

	if err := f.st.StartUpdateJob(store.UpdateJob{
		ID: "orphan-1", ContainerName: f.name, Phase: "verifying",
	}); err != nil {
		t.Fatalf("start job: %v", err)
	}

	f.m.Recover(f.ctx)

	j, err := f.st.UpdateJobByID("orphan-1")
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if !j.Done {
		t.Fatal("orphaned job was left in flight; a client would spin on it forever")
	}
	if len(f.m.ActiveJobs()) != 0 {
		t.Fatalf("orphaned job still reported as active: %v", f.m.ActiveJobs())
	}
}
