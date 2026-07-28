package docker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
)

// maxVersionAttempts is how many times auto-update installs, and rolls back,
// one version before writing it off. A version that has failed this often is
// not going to start working on the next poll, and every retry takes the
// service down and brings it back for nothing.
const maxVersionAttempts = 3

// maxFailedVersions is how many versions in a row may be written off before
// auto-update is switched off for the container entirely. One bad release is
// the publisher's problem and the next one will fix it; several in a row means
// the container cannot come up on a new image at all, which is something only
// the user can sort out.
const maxFailedVersions = 3

// updateAttempt says who asked for an update and which version it installs.
//
// Auto is false for one a person asked for: those are never blocked and never
// counted, because someone is watching the outcome and can decide for
// themselves whether to try again.
//
// Target is the digest the registry serves for the container's image tag, which
// is what makes "this version keeps failing" a thing that can be counted. It is
// empty when the registry could not be asked, and a failure with no target is
// not counted: the update is about to fail for a reason that has nothing to do
// with the version.
type updateAttempt struct {
	Auto   bool
	Target string
}

// AutoUpdatePoller periodically updates every managed container that has
// auto-update enabled. The interval comes from Settings
// (AutoUpdateIntervalMinutes, default 60). It runs until ctx is cancelled.
func (m *Manager) AutoUpdatePoller(ctx context.Context) {
	interval := m.autoUpdateInterval()
	timer := time.NewTimer(interval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			m.runAutoUpdates(ctx)
			interval = m.autoUpdateInterval()
			timer.Reset(interval)
		}
	}
}

func (m *Manager) autoUpdateInterval() time.Duration {
	minutes := 60
	if cfg, err := m.st.LoadSettings(); err == nil && cfg.AutoUpdateIntervalMinutes > 0 {
		minutes = cfg.AutoUpdateIntervalMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func (m *Manager) runAutoUpdates(ctx context.Context) {
	containers, err := m.st.AutoUpdateContainers()
	if err != nil {
		log.Printf("auto-update: list managed: %v", err)
		return
	}
	for _, mc := range containers {
		if ctx.Err() != nil {
			return
		}
		// A container that no longer exists has nothing to update, and failing
		// on it every interval would notify the user about it every interval.
		// It shows as missing in the UI, which is where they deal with it.
		if _, err := m.cli.ContainerInspect(ctx, mc.ContainerName); err != nil {
			continue
		}
		target := m.targetVersion(ctx, mc)
		if m.writtenOff(mc.ContainerName, target) {
			log.Printf("auto-update: skipping %s, gave up on %s", mc.ContainerName, shortID(target))
			continue
		}
		log.Printf("auto-update: checking %s", mc.ContainerName)
		m.update(ctx, mc.ID, updateAttempt{Auto: true, Target: target})
	}
}

// targetVersion is the digest the registry currently serves for a container's
// image, which is what identifies the version an update would install. It is
// empty when the registry cannot be asked (private registry, no network); the
// per-version bookkeeping then sits out this attempt rather than guessing at a
// version and counting a network outage as a bad release.
func (m *Manager) targetVersion(ctx context.Context, mc store.ManagedContainer) string {
	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil || snap.Image == "" {
		return ""
	}
	dist, err := m.cli.DistributionInspect(ctx, snap.Image, "")
	if err != nil {
		return ""
	}
	return dist.Descriptor.Digest.String()
}

// writtenOff reports whether auto-update has already given up on installing
// this version, and so should leave it alone until a new one is published.
func (m *Manager) writtenOff(containerName, target string) bool {
	if target == "" {
		return false
	}
	rows, err := m.st.UpdateFailures(containerName)
	if err != nil {
		log.Printf("auto-update: load failures for %s: %v", containerName, err)
		return false
	}
	for _, r := range rows {
		if r.Target == target && r.Failures >= maxVersionAttempts {
			return true
		}
	}
	return false
}

// noteUpdateFailure counts one failed auto-update against the version it was
// installing, and gives up when there is no point trying again: on the version
// once it has failed maxVersionAttempts times, and on auto-update itself once
// maxFailedVersions versions have been written off. Both are announced, since
// a service quietly staying behind is exactly what auto-update was turned on to
// prevent.
//
// It reports whether auto-update was switched off, so the caller can refresh
// the clients that are showing the toggle.
func (m *Manager) noteUpdateFailure(mc store.ManagedContainer, at updateAttempt, cause error) bool {
	if !at.Auto || at.Target == "" {
		return false
	}
	row, err := m.st.RecordUpdateFailure(mc.ContainerName, at.Target, cause.Error())
	if err != nil {
		log.Printf("auto-update: record failure for %s: %v", mc.ContainerName, err)
		return false
	}
	// Only the attempt that crosses the threshold announces anything: below it
	// there is another try coming, and above it the news is already out.
	if row.Failures != maxVersionAttempts {
		return false
	}

	rows, err := m.st.UpdateFailures(mc.ContainerName)
	if err != nil {
		log.Printf("auto-update: load failures for %s: %v", mc.ContainerName, err)
		return false
	}
	writtenOff := 0
	for _, r := range rows {
		if r.Failures >= maxVersionAttempts {
			writtenOff++
		}
	}

	meta := api.EventMeta{ContainerName: mc.ContainerName, StackID: mc.StackID}
	if writtenOff < maxFailedVersions {
		m.notify(api.EventAutoUpdateStopped,
			"Gave up on the new version of "+mc.ContainerName,
			fmt.Sprintf("The update failed %d times in a row and was rolled back each time, so Veery has stopped retrying this version. "+
				"%s keeps running the version it is on, and Veery will try again when a newer one is published. Last error: %s",
				maxVersionAttempts, mc.ContainerName, cause),
			meta)
		return false
	}

	if err := m.st.SetAutoUpdate(mc.ID, false); err != nil {
		log.Printf("auto-update: turn off for %s: %v", mc.ContainerName, err)
		return false
	}
	m.notify(api.EventAutoUpdateStopped,
		"Auto-update turned off for "+mc.ContainerName,
		fmt.Sprintf("The last %d versions each failed to install %d times, so this is not one bad release. "+
			"Auto-update is now off for %s and it keeps running the version it is on. "+
			"Turn it back on once you have worked out why new versions will not start. Last error: %s",
			maxFailedVersions, maxVersionAttempts, mc.ContainerName, cause),
		meta)
	return true
}

// clearUpdateFailures forgets a container's failure history after an update
// lands: whatever was wrong with the versions before it, the container is no
// longer stuck.
func (m *Manager) clearUpdateFailures(containerName string) {
	if err := m.st.ClearUpdateFailures(containerName); err != nil {
		log.Printf("auto-update: clear failures for %s: %v", containerName, err)
	}
}
