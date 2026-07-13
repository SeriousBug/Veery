package docker

import (
	"context"
	"encoding/json"
	"errors"
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

	emit("pulling", "Downloading "+ref)
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

	emit("recreating", "Installing the new image")
	newID, err := m.recreate(ctx, snap, ref)
	if err != nil {
		m.rollback(ctx, newID, oldInsp.ID, name, emit)
		return false, fmt.Errorf("update failed, rolled back: recreate on new image: %w", err)
	}

	emit("verifying", "Restarting and waiting for it to come up")
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

// pullInterval is the minimum gap between progress messages while pulling. The
// daemon emits a status event per layer several times a second; without this
// every one of them would become a WS broadcast.
const pullInterval = 300 * time.Millisecond

// layerProgress is the download/extract state of a single image layer, tracked
// so the whole pull can be reported as one figure instead of per-layer noise.
type layerProgress struct {
	status     string
	downloaded int64
	size       int64
}

// drainPull consumes the pull progress stream, aggregating the per-layer events
// into an overall "Downloading 45.2 MB / 120 MB" then "Extracting 3 / 5 layers".
func drainPull(rc io.ReadCloser, emit func(phase, msg string)) error {
	defer rc.Close()
	dec := json.NewDecoder(rc)
	layers := map[string]*layerProgress{}
	// pending holds the newest message the throttle has held back, so the final
	// state of the pull is always reported even if it lands inside the window.
	var pending, emitted string
	var lastAt time.Time
	for {
		var ev struct {
			ID       string `json:"id"`
			Status   string `json:"status"`
			Error    string `json:"error"`
			Progress struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
		}
		if err := dec.Decode(&ev); err != nil {
			if errors.Is(err, io.EOF) {
				if pending != "" {
					emit("pulling", pending)
				}
				return nil
			}
			return err
		}
		if ev.Error != "" {
			return fmt.Errorf("pull: %s", ev.Error)
		}
		// Events with no id are stream-level lines ("Pulling from library/nginx")
		// and carry no progress.
		if ev.ID == "" {
			continue
		}

		l := layers[ev.ID]
		if l == nil {
			l = &layerProgress{}
			layers[ev.ID] = l
		}
		l.status = ev.Status
		switch ev.Status {
		case "Downloading":
			// progressDetail is reused by the extract phase, so only trust its
			// byte counts while the layer is actually downloading.
			l.downloaded = ev.Progress.Current
			if ev.Progress.Total > 0 {
				l.size = ev.Progress.Total
			}
		case "Download complete", "Already exists", "Pull complete":
			l.downloaded = l.size
		}

		msg := pullMessage(layers)
		if msg == "" || msg == emitted {
			continue
		}
		pending = msg
		if time.Since(lastAt) < pullInterval {
			continue
		}
		emit("pulling", msg)
		pending, emitted, lastAt = "", msg, time.Now()
	}
}

// pullMessage renders the aggregate state of every layer seen so far. Layers
// download and extract concurrently, so bytes take priority: extraction is only
// reported once everything with a known size has arrived.
func pullMessage(layers map[string]*layerProgress) string {
	var downloaded, size int64
	var complete int
	for _, l := range layers {
		downloaded += l.downloaded
		size += l.size
		if l.status == "Pull complete" || l.status == "Already exists" {
			complete++
		}
	}
	switch {
	case size > 0 && downloaded < size:
		return fmt.Sprintf("Downloading %s / %s", formatBytes(downloaded), formatBytes(size))
	case complete < len(layers):
		return fmt.Sprintf("Extracting %d / %d layers", complete, len(layers))
	default:
		return ""
	}
}

func formatBytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGT"[exp])
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
