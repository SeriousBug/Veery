package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
)

// oldSuffix is appended to a container's name to park the previous instance
// during a transactional update so it can be restored on rollback.
const oldSuffix = "__veery_old"

// verifyTimeout is how long an updated container gets to prove itself healthy
// (running, and not unhealthy/exited) before the update is rolled back.
const verifyTimeout = 15 * time.Second

// Update pulls the latest image for a managed container and, if the digest
// changed, recreates the container from its snapshot on the new image. The
// whole operation runs through job() so progress is broadcast over the WS.
func (m *Manager) Update(ctx context.Context, managedID string) {
	mc, err := m.st.ManagedByID(managedID)
	if err != nil {
		// Allow resolving by container name as a fallback.
		if byName, nerr := m.st.ManagedByName(managedID); nerr == nil {
			mc = byName
		} else {
			m.job(ctx, "update", managedID, func(emit func(phase, msg string)) error {
				return fmt.Errorf("not a managed container: %s", managedID)
			})
			return
		}
	}
	m.job(ctx, "update", mc.ContainerName, func(emit func(phase, msg string)) error {
		updated, err := m.doUpdate(ctx, mc, emit)
		switch {
		case err != nil:
			m.notify(api.EventUpdateApplied, "Update failed: "+mc.ContainerName, err.Error())
		case updated:
			m.notify(api.EventUpdateApplied, "Updated "+mc.ContainerName, "The container is running a newer image and came up healthy.")
		}
		return err
	})
}

// doUpdate performs a transactional image update: the old container is parked
// (renamed + stopped, not removed) while the new one is created and verified.
// If the new container fails to come up healthy the change is rolled back to the
// parked container, so a bad image can never leave a dead service. It reports
// whether the container actually moved to a new image.
func (m *Manager) doUpdate(ctx context.Context, mc store.ManagedContainer, emit func(phase, msg string)) (bool, error) {
	lock := m.containerLock(mc.ContainerName)
	lock.Lock()
	defer lock.Unlock()

	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil {
		return false, err
	}
	ref := snap.Image
	if ref == "" {
		return false, fmt.Errorf("snapshot has no image reference")
	}
	name := mc.ContainerName

	oldInsp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", name, err)
	}
	oldImageID := oldInsp.Image

	emit("pulling", "Pulling "+ref)
	// Anonymous pull. Private registries would need per-registry credentials
	// pulled from settings and passed via image.PullOptions.RegistryAuth;
	// out of scope for now.
	rc, err := m.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return false, fmt.Errorf("pull %s: %w", ref, err)
	}
	if err := drainPull(rc, emit); err != nil {
		return false, err
	}

	newImg, err := m.cli.ImageInspect(ctx, ref)
	if err != nil {
		return false, fmt.Errorf("inspect image %s: %w", ref, err)
	}
	if newImg.ID == oldImageID {
		emit("up-to-date", "Already up to date")
		m.setUpdateAvailable(name, false)
		return false, nil
	}

	// Park the old container under a suffixed name so its original name is free
	// for the new one and it can be restored on rollback. Clear any leftover
	// parked container from a previously interrupted update first.
	oldParkedName := name + oldSuffix
	_ = m.cli.ContainerRemove(ctx, oldParkedName, container.RemoveOptions{Force: true})
	if err := m.cli.ContainerRename(ctx, oldInsp.ID, oldParkedName); err != nil {
		return false, fmt.Errorf("park old container: %w", err)
	}
	if oldInsp.State != nil && oldInsp.State.Running {
		_ = m.cli.ContainerStop(ctx, oldInsp.ID, container.StopOptions{})
	}

	emit("recreating", "Recreating "+name)
	newID, err := m.recreate(ctx, snap, ref)
	if err != nil {
		m.rollback(ctx, newID, oldInsp.ID, name, emit)
		return false, fmt.Errorf("update failed, rolled back: recreate on new image: %w", err)
	}

	emit("verifying", "Verifying "+name)
	if verr := m.verifyHealthy(ctx, newID); verr != nil {
		m.rollback(ctx, newID, oldInsp.ID, name, emit)
		return false, fmt.Errorf("update failed, rolled back: %w", verr)
	}

	// Success: drop the parked old container, refresh the snapshot, best-effort
	// prune the old image and clear the update flag.
	_ = m.cli.ContainerRemove(ctx, oldInsp.ID, container.RemoveOptions{Force: true})
	if insp, ierr := m.cli.ContainerInspect(ctx, newID); ierr == nil {
		fresh := snapshotFromInspect(insp)
		if js, merr := fresh.marshal(); merr == nil {
			_ = m.st.UpdateSnapshot(mc.ID, js)
		}
	}
	// Ignore "in use" errors: the old image may still back other containers.
	_, _ = m.cli.ImageRemove(ctx, oldImageID, image.RemoveOptions{PruneChildren: true})
	m.setUpdateAvailable(name, false)

	emit("updated", "Updated to "+shortID(newImg.ID))
	return true, nil
}

// rollback restores the parked old container after a failed update: the new
// container (if any) is removed, the old one renamed back to its original name
// and started. Nothing is pruned and the stored snapshot is left untouched.
func (m *Manager) rollback(ctx context.Context, newID, oldID, name string, emit func(phase, msg string)) {
	emit("rollback", "Update failed, rolling back "+name)
	if newID != "" {
		_ = m.cli.ContainerRemove(ctx, newID, container.RemoveOptions{Force: true})
	}
	if err := m.cli.ContainerRename(ctx, oldID, name); err != nil {
		emit("rollback", "restore rename failed: "+err.Error())
	}
	_ = m.cli.ContainerStart(ctx, oldID, container.StartOptions{})
}

// verifyHealthy polls a freshly created container for up to verifyTimeout,
// returning an error if it exits, crash-loops or becomes unhealthy. A container
// with a healthcheck must reach "healthy"; one without only has to stay running
// for the window.
func (m *Manager) verifyHealthy(ctx context.Context, id string) error {
	deadline := time.Now().Add(verifyTimeout)
	for {
		insp, err := m.cli.ContainerInspect(ctx, id)
		if err != nil {
			return fmt.Errorf("inspect new container: %w", err)
		}
		st := insp.State
		switch {
		case st == nil:
			return fmt.Errorf("new container has no state")
		case st.Restarting:
			// Crash-looping under a restart policy; keep waiting for the window
			// to decide, since a slow-starting service also restarts.
		case !st.Running:
			return fmt.Errorf("new container exited (exit code %d)", st.ExitCode)
		case st.Health != nil:
			switch st.Health.Status {
			case container.Healthy:
				return nil
			case container.Unhealthy:
				return fmt.Errorf("new container became unhealthy")
			}
		}
		if time.Now().After(deadline) {
			if st.Health != nil && st.Health.Status != container.Healthy {
				return fmt.Errorf("new container did not become healthy within %s", verifyTimeout)
			}
			if !st.Running {
				return fmt.Errorf("new container exited (exit code %d)", st.ExitCode)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// drainPull consumes the pull progress stream, surfacing status lines.
func drainPull(rc io.ReadCloser, emit func(phase, msg string)) error {
	defer rc.Close()
	dec := json.NewDecoder(rc)
	for {
		var ev struct {
			Status string `json:"status"`
			Error  string `json:"error"`
		}
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if ev.Error != "" {
			return fmt.Errorf("pull: %s", ev.Error)
		}
		if ev.Status != "" {
			emit("pulling", ev.Status)
		}
	}
}

func shortID(id string) string {
	id = trimDigestPrefix(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func trimDigestPrefix(id string) string {
	const p = "sha256:"
	if len(id) > len(p) && id[:len(p)] == p {
		return id[len(p):]
	}
	return id
}
