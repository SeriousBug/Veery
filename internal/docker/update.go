package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
)

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
		return m.doUpdate(ctx, mc, emit)
	})
}

func (m *Manager) doUpdate(ctx context.Context, mc store.ManagedContainer, emit func(phase, msg string)) error {
	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil {
		return err
	}
	ref := snap.Image
	if ref == "" {
		return fmt.Errorf("snapshot has no image reference")
	}

	emit("pulling", "Pulling "+ref)
	// Anonymous pull. Private registries would need per-registry credentials
	// pulled from settings and passed via image.PullOptions.RegistryAuth;
	// out of scope for now.
	rc, err := m.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull %s: %w", ref, err)
	}
	if err := drainPull(rc, emit); err != nil {
		return err
	}

	newImg, err := m.cli.ImageInspect(ctx, ref)
	if err != nil {
		return fmt.Errorf("inspect image %s: %w", ref, err)
	}

	// Current image id of the running container (if it still exists).
	var currentImageID string
	if insp, ierr := m.cli.ContainerInspect(ctx, mc.ContainerName); ierr == nil {
		currentImageID = insp.Image
	}

	if currentImageID != "" && currentImageID == newImg.ID {
		emit("up-to-date", "Already up to date")
		return nil
	}

	emit("recreating", "Recreating "+mc.ContainerName)
	if insp, ierr := m.cli.ContainerInspect(ctx, mc.ContainerName); ierr == nil {
		if insp.State != nil && insp.State.Running {
			_ = m.cli.ContainerStop(ctx, insp.ID, container.StopOptions{})
		}
		if err := m.cli.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); err != nil {
			return fmt.Errorf("remove old container: %w", err)
		}
	}

	newID, err := m.recreate(ctx, snap, ref)
	if err != nil {
		return fmt.Errorf("recreate on new image: %w", err)
	}

	// Refresh the stored snapshot from the new container.
	if insp, ierr := m.cli.ContainerInspect(ctx, newID); ierr == nil {
		fresh := snapshotFromInspect(insp)
		if js, merr := fresh.marshal(); merr == nil {
			_ = m.st.UpdateSnapshot(mc.ID, js)
		}
	}

	// Prune the old image (best effort).
	if currentImageID != "" && currentImageID != newImg.ID {
		_, _ = m.cli.ImageRemove(ctx, currentImageID, image.RemoveOptions{PruneChildren: true})
	}

	emit("updated", "Updated to "+shortID(newImg.ID))
	return nil
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
