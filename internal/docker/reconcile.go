package docker

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
)

// Reconcile brings the managed records back in line with the containers Docker
// actually has. Veery neither creates nor edits containers, so the compose file
// (or a docker run) stays the source of truth, and the user is free to change
// it at any time, including while Veery is not running.
//
// Two things drift as a result. A container the user recreated carries a spec
// Veery has never seen, and the snapshot it holds -- which is what an update or
// a bring-up recreates from -- would silently undo their change. And a service
// the user added to an already-managed stack is not managed at all, so nothing
// watches or updates it.
//
// A recreate is spotted by container id: Docker never reuses one, and starting,
// stopping or restarting a container keeps it, so an id that no longer matches
// means the container was made anew by something other than Veery.
//
// Call this before serving and on the status sweep.
func (m *Manager) Reconcile(ctx context.Context) {
	summaries, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		log.Printf("reconcile: list containers: %v", err)
		return
	}
	managed, err := m.st.AllManaged()
	if err != nil {
		log.Printf("reconcile: list managed: %v", err)
		return
	}
	byName := map[string]store.ManagedContainer{}
	for _, mc := range managed {
		byName[mc.ContainerName] = mc
	}

	changed := false
	for _, c := range summaries {
		if !isService(c) {
			continue
		}
		name := containerName(c.Names)
		mc, ok := byName[name]
		if !ok {
			changed = m.adoptNew(ctx, c, name) || changed
			continue
		}
		if mc.ContainerID == c.ID {
			continue
		}
		changed = m.rerecord(ctx, mc, c.ID) || changed
	}

	if changed {
		m.BroadcastStacks(ctx)
	}
}

// isService reports whether a container is one of the user's services rather
// than Veery's own scaffolding: the helper that performs a self-update, or a
// container parked mid-swap under its suffixed name.
func isService(c container.Summary) bool {
	return c.Labels[updaterLabel] != updaterRole &&
		!strings.HasSuffix(containerName(c.Names), oldSuffix)
}

// adoptNew takes over a container that has appeared in a stack Veery already
// manages, which is what adding a service to a compose file looks like from
// here. A container in no managed stack is left alone: adopting is the user's
// call, and the dashboard already offers it.
func (m *Manager) adoptNew(ctx context.Context, c container.Summary, name string) bool {
	stackID := c.Labels[projectLabel]
	if stackID == "" {
		stackID = name
	}
	managed, err := m.st.StackExists(stackID)
	if err != nil || !managed {
		return false
	}

	insp, err := m.cli.ContainerInspect(ctx, c.ID)
	if err != nil {
		return false
	}
	js, err := snapshotFromInspect(insp).marshal()
	if err != nil {
		log.Printf("reconcile %s: marshal snapshot: %v", name, err)
		return false
	}
	if err := m.st.AddManagedContainer(store.ManagedContainer{
		ID:            genID(),
		StackID:       stackID,
		ContainerName: name,
		SnapshotJSON:  js,
		ContainerID:   insp.ID,
		CreatedAt:     time.Now().Unix(),
	}); err != nil {
		log.Printf("reconcile %s: adopt: %v", name, err)
		return false
	}
	log.Printf("reconcile %s: new container in managed stack %s, now managing it", name, stackID)
	// Veery adopts this on its own, without anyone asking it to, so it says so.
	m.notify(api.EventContainerAdopted, "Now managing "+name,
		"It showed up in "+stackID+", which Veery manages, so Veery is watching it too. "+
			"Auto-update is off for it until you turn it on.",
		api.EventMeta{ContainerName: name, StackID: stackID})
	return true
}

// rerecord replaces the snapshot of a container that was recreated outside
// Veery with the spec it is actually running.
func (m *Manager) rerecord(ctx context.Context, mc store.ManagedContainer, liveID string) bool {
	lock := m.containerLock(mc.ContainerName)
	if !lock.TryLock() {
		// An update or a lifecycle action holds this container. It is recreating
		// the container itself and records what it created, so there is nothing
		// here to fix and the sweep must not race it.
		return false
	}
	defer lock.Unlock()

	// The row may have been written between listing and taking the lock.
	mc, err := m.st.ManagedByName(mc.ContainerName)
	if err != nil || mc.ContainerID == liveID {
		return false
	}

	insp, err := m.cli.ContainerInspect(ctx, liveID)
	if err != nil {
		return false
	}
	if !settled(insp) {
		// Recording a spec that has not proved itself would give bring-up and
		// update rollback a broken container to restore. The previous snapshot
		// stays until this one settles, or until the user replaces it again.
		return false
	}

	updated, err := m.recordSpec(mc, insp)
	if err != nil {
		log.Printf("reconcile %s: %v", mc.ContainerName, err)
		return false
	}
	if mc.ContainerID == "" {
		log.Printf("reconcile %s: recording the container it is running", mc.ContainerName)
	} else {
		log.Printf("reconcile %s: recreated outside Veery, re-recording its configuration", mc.ContainerName)
	}

	// The update-available flag was worked out against the image the old
	// snapshot asked for, which is not necessarily the image this container now
	// runs: a changed tag makes the flag meaningless in both directions.
	m.setUpdateAvailable(mc.ContainerName, m.remoteHasNewImage(ctx, updated))
	return true
}

// refreshIfRecreated returns the container's real spec, re-recording it first
// when the live container is not the one the stored snapshot was taken from.
// Every path that recreates a container goes through this, so a recreate can
// never be built from a spec the user has since replaced.
//
// The caller must hold the container's lock.
func (m *Manager) refreshIfRecreated(mc store.ManagedContainer, insp container.InspectResponse) (store.ManagedContainer, Snapshot, error) {
	if insp.ID != mc.ContainerID {
		updated, err := m.recordSpec(mc, insp)
		if err != nil {
			return mc, Snapshot{}, err
		}
		mc = updated
	}
	snap, err := parseSnapshot(mc.SnapshotJSON)
	return mc, snap, err
}

// recordSpec snapshots a container as it is now and persists it, returning the
// updated record.
func (m *Manager) recordSpec(mc store.ManagedContainer, insp container.InspectResponse) (store.ManagedContainer, error) {
	js, err := snapshotFromInspect(insp).marshal()
	if err != nil {
		return mc, err
	}
	if err := m.st.UpdateSnapshot(mc.ID, js, insp.ID); err != nil {
		return mc, err
	}
	mc.SnapshotJSON = js
	mc.ContainerID = insp.ID
	return mc, nil
}

// settled reports whether a container has shown its spec to be good enough to
// keep: it is up and passing its health check, or it is deliberately not
// running. One that is crash-looping, dead, or exited on an error has proved
// nothing, and a spec that cannot run is not one to fall back on.
func settled(insp container.InspectResponse) bool {
	st := insp.State
	switch {
	case st == nil, st.Restarting, st.Dead:
		return false
	case st.Running:
		if st.Health == nil {
			return true
		}
		return st.Health.Status == container.Healthy || st.Health.Status == container.NoHealthcheck
	default:
		// created, paused, or stopped by hand. Only a crash rules the spec out.
		return st.ExitCode == 0
	}
}
