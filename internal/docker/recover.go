package docker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/docker/docker/api/types/container"
)

// Recover reconciles any update that was interrupted by a crash, a host reboot
// or a self-update handoff, and resumes reporting on updates still in flight.
// It is driven off Docker's own state rather than the DB, because Docker state
// is the only thing guaranteed to have survived: a container parked under the
// oldSuffix name means a swap was in progress, whatever the DB says.
//
// Call this at startup, before serving.
func (m *Manager) Recover(ctx context.Context) {
	updaters, err := m.runningUpdaters(ctx)
	if err != nil {
		log.Printf("recover: list updaters: %v", err)
		return
	}

	// A running helper owns the containers it is swapping, including the parked
	// old one it may still need to roll back to. Reconciling underneath it would
	// tear out its rollback target.
	if len(updaters) == 0 {
		if err := m.reconcileParked(ctx); err != nil {
			log.Printf("recover: %v", err)
		}
		m.pruneUpdaters(ctx)
	}

	m.resumeJobs(ctx, updaters)
}

// reconcileParked restores or retires every container left parked by an
// interrupted swap.
func (m *Manager) reconcileParked(ctx context.Context) error {
	list, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	byName := map[string]container.Summary{}
	for _, c := range list {
		byName[containerName(c.Names)] = c
	}

	for name, parked := range byName {
		orig, ok := strings.CutSuffix(name, oldSuffix)
		if !ok {
			continue
		}
		fresh, hasFresh := byName[orig]
		if err := m.reconcileOne(ctx, orig, parked, fresh, hasFresh); err != nil {
			log.Printf("recover %s: %v", orig, err)
		}
	}
	return nil
}

// reconcileOne decides the fate of one parked container. The new container is
// kept only if it exists and proves healthy; otherwise the parked original is
// put back, which is the same guarantee a completed update gives.
func (m *Manager) reconcileOne(ctx context.Context, orig string, parked, fresh container.Summary, hasFresh bool) error {
	self := m.SelfContainerID(ctx)

	switch {
	case hasFresh && fresh.ID == self:
		// The new container is the process running this code, so the swap got far
		// enough to start us. Drop the old one.
		log.Printf("recover %s: update completed, removing parked container", orig)
		return m.cli.ContainerRemove(ctx, parked.ID, container.RemoveOptions{Force: true})

	case parked.ID == self:
		// We are the parked container: someone started the old Veery back up by
		// hand after a failed self-update. Take our name back.
		if hasFresh && fresh.State == "running" && m.healthy(ctx, fresh.ID) {
			log.Printf("recover %s: a healthy container already holds the name, leaving %s parked", orig, parked.ID[:12])
			return nil
		}
		if hasFresh {
			if err := m.cli.ContainerRemove(ctx, fresh.ID, container.RemoveOptions{Force: true}); err != nil {
				return fmt.Errorf("remove failed replacement: %w", err)
			}
		}
		log.Printf("recover %s: restoring own name after an interrupted self-update", orig)
		return m.cli.ContainerRename(ctx, parked.ID, orig)

	case !hasFresh:
		// The swap died between parking the old container and creating the new
		// one. Nothing replaced it, so put it back.
		log.Printf("recover %s: no replacement was created, restoring", orig)
		if err := m.cli.ContainerRename(ctx, parked.ID, orig); err != nil {
			return fmt.Errorf("restore name: %w", err)
		}
		return m.cli.ContainerStart(ctx, parked.ID, container.StartOptions{})

	default:
		// A replacement exists but nobody verified it. Hold it to the same bar a
		// completed update would have.
		if err := m.verifyHealthy(ctx, fresh.ID); err != nil {
			log.Printf("recover %s: replacement is unhealthy (%v), rolling back", orig, err)
			if rbErr := m.rollback(ctx, fresh.ID, parked.ID, orig, func(string, string) {}); rbErr != nil {
				log.Printf("recover %s: rollback could not restart the old container: %v", orig, rbErr)
			}
			return nil
		}
		log.Printf("recover %s: replacement is healthy, removing parked container", orig)
		return m.cli.ContainerRemove(ctx, parked.ID, container.RemoveOptions{Force: true})
	}
}

// healthy reports whether a container is running and not failing its healthcheck.
func (m *Manager) healthy(ctx context.Context, id string) bool {
	insp, err := m.cli.ContainerInspect(ctx, id)
	if err != nil || insp.State == nil || !insp.State.Running {
		return false
	}
	return insp.State.Health == nil || insp.State.Health.Status != container.Unhealthy
}

// resumeJobs picks up update jobs that outlived the process that started them.
// A job whose helper is still running is watched to completion; one with no
// helper left is settled from the state the container actually ended up in, so
// a client never sits on a spinner that will never resolve.
func (m *Manager) resumeJobs(ctx context.Context, updaters map[string]container.Summary) {
	jobs, err := m.st.ActiveUpdateJobs()
	if err != nil {
		log.Printf("recover: load active jobs: %v", err)
		return
	}
	for _, j := range jobs {
		if _, running := updaters[j.ID]; running {
			m.track(api.JobProgress{
				ID: j.ID, Kind: "update", Target: j.ContainerName,
				Phase: j.Phase, Message: j.Message,
			})
			go m.watchJob(ctx, j.ID, j.ContainerName)
			continue
		}
		m.settleJob(ctx, j.ID, j.ContainerName)
	}
}

// watchJob follows a job being run by a helper container, republishing its
// progress (the helper writes to the DB; only this process has the WS) until it
// reaches a terminal state.
func (m *Manager) watchJob(ctx context.Context, id, name string) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}

		j, err := m.st.UpdateJobByID(id)
		if err != nil {
			return
		}
		if !j.Done {
			// The helper is gone but never finished the row: it was killed
			// mid-swap. The next start will reconcile from Docker state.
			if _, running, _ := m.updaterFor(ctx, id); !running {
				m.settleJob(ctx, id, name)
				return
			}
			m.track(api.JobProgress{
				ID: id, Kind: "update", Target: name,
				Phase: j.Phase, Message: j.Message,
			})
			continue
		}

		m.forget(id)
		final := api.JobProgress{ID: id, Kind: "update", Target: name, Phase: j.Phase, Done: true}
		if j.Error != "" {
			final.Error = j.Error
		} else {
			final.Message = j.Message
		}
		m.publish(final)
		m.BroadcastStacks(ctx)
		return
	}
}

func (m *Manager) updaterFor(ctx context.Context, jobID string) (container.Summary, bool, error) {
	updaters, err := m.runningUpdaters(ctx)
	if err != nil {
		return container.Summary{}, false, err
	}
	c, ok := updaters[jobID]
	return c, ok, nil
}

// settleJob closes out an update job whose worker is gone, reporting the outcome
// the container actually ended up with rather than guessing.
func (m *Manager) settleJob(ctx context.Context, id, name string) {
	m.forget(id)

	onNewImage, err := m.onSnapshotImage(ctx, name)
	switch {
	case err != nil:
		_ = m.st.FinishUpdateJob(id, "failed", "", err.Error())
		m.publish(api.JobProgress{ID: id, Kind: "update", Target: name, Phase: "failed", Done: true, Error: err.Error()})
	case onNewImage:
		_ = m.st.FinishUpdateJob(id, "done", "Updated", "")
		m.publish(api.JobProgress{ID: id, Kind: "update", Target: name, Phase: "done", Done: true, Message: "Updated"})
	default:
		const msg = "Veery restarted before the update finished; the container was left on its previous image"
		_ = m.st.FinishUpdateJob(id, "failed", "", msg)
		m.publish(api.JobProgress{ID: id, Kind: "update", Target: name, Phase: "failed", Done: true, Error: msg})
	}
}

// onSnapshotImage reports whether a container is running the image its snapshot
// asks for, which is what a finished update leaves behind.
func (m *Manager) onSnapshotImage(ctx context.Context, name string) (bool, error) {
	mc, err := m.st.ManagedByName(name)
	if err != nil {
		return false, fmt.Errorf("no longer managed: %s", name)
	}
	snap, err := parseSnapshot(mc.SnapshotJSON)
	if err != nil {
		return false, err
	}
	insp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		return false, fmt.Errorf("container is gone: %s", name)
	}
	img, err := m.cli.ImageInspect(ctx, snap.Image)
	if err != nil {
		return false, nil
	}
	return img.ID == insp.Image, nil
}
