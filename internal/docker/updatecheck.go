package docker

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
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
	m.seedUpdateAvail()
	changed := false
	for _, mc := range managed {
		if ctx.Err() != nil {
			return
		}
		avail := m.remoteHasNewImage(ctx, mc)
		if was := m.updateAvailableFor(mc.ContainerName); was != avail {
			changed = true
			// Auto-updating containers are left alone: the update runs on its
			// own and reports its outcome, so announcing it here is noise.
			if avail && !mc.AutoUpdate {
				m.notify(api.EventUpdateAvailable, "Update available for "+mc.ContainerName,
					"A newer image has been published. Auto-update is off for this container, so it will keep running the current image until you update it.",
					api.EventMeta{ContainerName: mc.ContainerName, StackID: mc.StackID})
			}
		}
		m.setUpdateAvailable(mc.ContainerName, avail)
	}
	if changed {
		m.persistUpdateAvail()
		m.BroadcastStacks(ctx)
	}
}

// seedUpdateAvail loads the update-available flags recorded by the last sweep,
// once per process. Without it every restart would re-announce every update
// that is still pending.
func (m *Manager) seedUpdateAvail() {
	m.availMu.Lock()
	defer m.availMu.Unlock()
	if m.availBaseline {
		return
	}
	saved, err := m.st.LoadNotifiedUpdates()
	if err != nil {
		log.Printf("update-check: load last update flags: %v", err)
		return
	}
	m.updateAvail = saved
	m.availBaseline = true
}

func (m *Manager) persistUpdateAvail() {
	m.availMu.Lock()
	snapshot := make(map[string]bool, len(m.updateAvail))
	for k, v := range m.updateAvail {
		snapshot[k] = v
	}
	m.availMu.Unlock()
	if err := m.st.SaveNotifiedUpdates(snapshot); err != nil {
		log.Printf("update-check: save update flags: %v", err)
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
