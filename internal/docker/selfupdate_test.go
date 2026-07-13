package docker

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
)

// selfFixture stands up a managed, running container against the real daemon and
// makes the Manager believe it is the container Veery itself runs in, which is
// the only condition that routes an update down the handoff path.
type selfFixture struct {
	m    *Manager
	st   *store.Store
	name string
	id   string
	mc   store.ManagedContainer
	ctx  context.Context
}

func newSelfFixture(t *testing.T) *selfFixture {
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
	ensureImage(t, m, ctx, "alpine:latest")

	project := "veeryself"
	name := fmt.Sprintf("veeryself-%d", time.Now().UnixNano())

	// The entrypoint stands in for the real image's ["/veery"]: the helper has to
	// inherit it. "env" keeps this container running as `env sleep 3600` while
	// still being something the helper can be started with.
	created, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image:      "busybox:latest",
		Entrypoint: []string{"/bin/env"},
		Cmd:        []string{"sleep", "3600"},
		// Veery has to be run as a user that can open the Docker socket, which the
		// image's default (nonroot) cannot. The helper must inherit that.
		User:   "0",
		Labels: map[string]string{projectLabel: project, serviceLabel: "veery"},
	}, &container.HostConfig{GroupAdd: []string{"999"}}, nil, nil, name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = m.cli.ContainerRemove(bg, name, container.RemoveOptions{Force: true})
		_ = m.cli.ContainerRemove(bg, name+oldSuffix, container.RemoveOptions{Force: true})
		_ = st.Unadopt(project)
	})
	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	if err := m.Adopt(ctx, project); err != nil {
		t.Fatalf("adopt: %v", err)
	}
	mc, err := st.ManagedByName(name)
	if err != nil {
		t.Fatalf("managed lookup: %v", err)
	}

	return &selfFixture{m: m, st: st, name: name, id: created.ID, mc: mc, ctx: ctx}
}

// beSelf makes the Manager treat the fixture's container as its own.
func (f *selfFixture) beSelf() {
	f.m.selfOnce.Do(func() {})
	f.m.selfID = f.id
}

// retarget points the stored snapshot at a different image, so an update has
// somewhere to go.
func (f *selfFixture) retarget(t *testing.T, image string, cmd []string) {
	t.Helper()
	snap, err := parseSnapshot(f.mc.SnapshotJSON)
	if err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	snap.Image = image
	snap.Config.Image = image
	snap.Config.Cmd = cmd
	// The fixture's stand-in entrypoint does not exist in the target image.
	snap.Config.Entrypoint = nil
	js, err := snap.marshal()
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := f.st.UpdateSnapshot(f.mc.ID, js); err != nil {
		t.Fatalf("update snapshot: %v", err)
	}
	f.mc, _ = f.st.ManagedByID(f.mc.ID)
}

// Veery must never perform its own swap in-process: stopping the old container
// kills the process midway. It hands off to a detached helper instead, and
// leaves its own container untouched for the helper to deal with.
func TestSelfUpdateHandsOffToUpdater(t *testing.T) {
	f := newSelfFixture(t)
	f.beSelf()
	f.retarget(t, "alpine:latest", []string{"sh", "-c", "sleep 3600"})

	jobID := genID()
	_, err := f.m.doUpdate(f.ctx, f.mc, jobID, func(string, string) {})
	if !errors.Is(err, errHandedOff) {
		t.Fatalf("expected the update to hand off, got %v", err)
	}
	t.Cleanup(func() {
		_ = f.m.cli.ContainerRemove(context.Background(), updaterName(jobID), container.RemoveOptions{Force: true})
	})

	// Veery's own container must still be running: the helper, not Veery, is what
	// stops it.
	insp := f.m.mustInspect(t, f.ctx, f.name)
	if !insp.State.Running {
		t.Fatal("Veery stopped its own container instead of handing off")
	}

	// The helper here runs a stand-in entrypoint, so what matters is that it was
	// created and started with the right spec, not what it did once it ran.
	updater := f.m.mustInspect(t, f.ctx, updaterName(jobID))
	// It must run the image Veery is on now, not the one being updated to: a
	// broken new image would otherwise take the updater down with it.
	if updater.Image != insp.Image {
		t.Errorf("updater runs %s, want the current image %s", updater.Image, insp.Image)
	}
	// The entrypoint has to come from the running container, not the image's
	// default, or the helper never starts the Veery binary at all.
	if got := strings.Join(updater.Config.Entrypoint, " "); got != "/bin/env" {
		t.Errorf("updater entrypoint = %q, want it inherited from Veery's container", got)
	}
	if got := strings.Join(updater.Config.Cmd, " "); !strings.Contains(got, "apply-update") {
		t.Errorf("updater cmd = %q, want it to run apply-update", got)
	}
	if updater.Config.Labels[jobLabel] != jobID {
		t.Errorf("updater job label = %q, want %q", updater.Config.Labels[jobLabel], jobID)
	}
	// Whatever lets Veery reach the Docker socket has to reach the helper too, or
	// it cannot do the one thing it exists for. The image default is nonroot,
	// which cannot open the socket at all.
	if updater.Config.User != "0" {
		t.Errorf("updater user = %q, want it inherited from Veery's container", updater.Config.User)
	}
	if len(updater.HostConfig.GroupAdd) != 1 || updater.HostConfig.GroupAdd[0] != "999" {
		t.Errorf("updater GroupAdd = %v, want it inherited from Veery's container", updater.HostConfig.GroupAdd)
	}
	// Published ports would fail to bind against the Veery still running, and a
	// restart policy would resurrect the helper forever.
	if len(updater.HostConfig.PortBindings) != 0 {
		t.Errorf("updater must not publish ports, got %v", updater.HostConfig.PortBindings)
	}
	if updater.HostConfig.RestartPolicy.Name != container.RestartPolicyDisabled {
		t.Errorf("updater restart policy = %q, want none", updater.HostConfig.RestartPolicy.Name)
	}

	// The job stays in flight: the helper owns it now and reports the outcome.
	j, err := f.st.UpdateJobByID(jobID)
	if err == nil && j.Done {
		t.Error("job was marked done even though the helper had not finished it")
	}
}

// ApplyUpdate is what the helper runs. It performs the swap from outside the
// container being replaced, which is what makes a self-update survivable.
func TestApplyUpdateSwapsFromOutside(t *testing.T) {
	f := newSelfFixture(t)
	f.retarget(t, "alpine:latest", []string{"sh", "-c", "sleep 3600"})

	jobID := genID()
	if err := f.st.StartUpdateJob(store.UpdateJob{ID: jobID, ContainerName: f.name, Phase: "handoff"}); err != nil {
		t.Fatalf("start job: %v", err)
	}

	if err := f.m.ApplyUpdate(f.ctx, f.name, jobID); err != nil {
		t.Fatalf("apply update: %v", err)
	}

	alpine, err := f.m.cli.ImageInspect(f.ctx, "alpine:latest")
	if err != nil {
		t.Fatalf("inspect alpine: %v", err)
	}
	insp := f.m.mustInspect(t, f.ctx, f.name)
	if !insp.State.Running {
		t.Fatalf("container not running after the swap (state %s)", insp.State.Status)
	}
	if insp.Image != alpine.ID {
		t.Fatalf("container is on %s, want the new image %s", insp.Image, alpine.ID)
	}
	if insp.ID == f.id {
		t.Fatal("container was never recreated")
	}
	if _, err := f.m.cli.ContainerInspect(f.ctx, f.name+oldSuffix); err == nil {
		t.Fatal("the parked container should have been removed after a successful swap")
	}

	j, err := f.st.UpdateJobByID(jobID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if !j.Done || j.Error != "" {
		t.Fatalf("job should have finished cleanly, got done=%v err=%q", j.Done, j.Error)
	}
}

// A helper that cannot bring the new image up must put the old container back,
// which is the promise the confirmation dialog makes.
func TestApplyUpdateRollsBackABadImage(t *testing.T) {
	f := newSelfFixture(t)
	f.retarget(t, "alpine:latest", []string{"sh", "-c", "exit 1"})

	jobID := genID()
	if err := f.st.StartUpdateJob(store.UpdateJob{ID: jobID, ContainerName: f.name, Phase: "handoff"}); err != nil {
		t.Fatalf("start job: %v", err)
	}

	err := f.m.ApplyUpdate(f.ctx, f.name, jobID)
	if err == nil {
		t.Fatal("expected the update to fail on an image that dies immediately")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected a rollback, got: %v", err)
	}

	insp := f.m.mustInspect(t, f.ctx, f.name)
	if !insp.State.Running {
		t.Fatalf("the original container was not restored (state %s)", insp.State.Status)
	}
	if insp.ID != f.id {
		t.Fatal("the restored container is not the original")
	}

	j, err := f.st.UpdateJobByID(jobID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if !j.Done || j.Error == "" {
		t.Fatalf("job should have finished with an error, got done=%v err=%q", j.Done, j.Error)
	}
}

func (m *Manager) mustInspect(t *testing.T, ctx context.Context, name string) container.InspectResponse {
	t.Helper()
	insp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		t.Fatalf("inspect %s: %v", name, err)
	}
	return insp
}
