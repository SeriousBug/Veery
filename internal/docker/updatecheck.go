package docker

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/store"
)

// UpdateCheckPoller periodically refreshes the in-memory update-availability
// map for every managed container. It runs until ctx is cancelled.
func (m *Manager) UpdateCheckPoller(ctx context.Context) {
	m.CheckUpdates(ctx)
	timer := time.NewTimer(m.autoUpdateInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.CheckUpdates(ctx)
			timer.Reset(m.autoUpdateInterval())
		}
	}
}

// CheckUpdates inspects every managed container's remote image digest (without
// pulling) and records whether a newer image is available. Registry failures
// (private registries, anonymous denied, network) are treated as "unknown" and
// leave the flag cleared rather than failing the whole sweep.
func (m *Manager) CheckUpdates(ctx context.Context) {
	managed, err := m.st.AllManaged()
	if err != nil {
		log.Printf("update-check: list managed: %v", err)
		return
	}
	changed := false
	for _, mc := range managed {
		if ctx.Err() != nil {
			return
		}
		avail := m.remoteHasNewImage(ctx, mc)
		if m.updateAvailableFor(mc.ContainerName) != avail {
			changed = true
		}
		m.setUpdateAvailable(mc.ContainerName, avail)
	}
	if changed {
		m.BroadcastStacks(ctx)
	}
}

// remoteHasNewImage reports whether the registry serves a manifest digest that
// differs from the running container's local image. Any inability to determine
// this (parse error, registry error, no local repo digest) returns false.
func (m *Manager) remoteHasNewImage(ctx context.Context, mc store.ManagedContainer) bool {
	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil || snap.Image == "" {
		return false
	}
	dist, err := m.cli.DistributionInspect(ctx, snap.Image, "")
	if err != nil {
		return false
	}
	remote := dist.Descriptor.Digest.String()
	if remote == "" {
		return false
	}
	insp, err := m.cli.ContainerInspect(ctx, mc.ContainerName)
	if err != nil {
		return false
	}
	img, err := m.cli.ImageInspect(ctx, insp.Image)
	if err != nil || len(img.RepoDigests) == 0 {
		return false
	}
	for _, rd := range img.RepoDigests {
		if _, digest, ok := strings.Cut(rd, "@"); ok && digest == remote {
			return false
		}
	}
	return true
}
