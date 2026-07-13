package docker

import (
	"context"
	"log"
	"time"
)

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
		log.Printf("auto-update: checking %s", mc.ContainerName)
		m.Update(ctx, mc.ID)
	}
}
