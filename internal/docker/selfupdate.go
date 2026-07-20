package docker

import (
	"context"
	"fmt"
	"log"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/docker/docker/api/types/container"
)

// jobLabel carries the update job id on the helper container, so a restarted
// Veery can match a running helper back to the job row it is finishing.
const jobLabel = "veery.job"

// updaterName is the container name for the helper performing a self-update.
func updaterName(jobID string) string { return "veery-updater-" + jobID }

// handOff starts a detached helper container that will perform the swap on
// Veery's behalf. Veery cannot update itself in-process: parking the old
// container means stopping it, which kills the very process that would go on to
// create and verify the new one, leaving the container parked and the service
// down. The helper is a separate process that survives that stop.
//
// It runs the image Veery is running *now*, not the image being updated to: the
// helper's job is to launch and verify the new version, and betting that job on
// the new image being able to run it is how a bad image turns into a permanent
// outage. The current image is already known to work.
func (m *Manager) handOff(ctx context.Context, selfInsp container.InspectResponse, jobID string, emit func(phase, msg string)) error {
	if selfInsp.Config == nil || selfInsp.HostConfig == nil {
		return fmt.Errorf("cannot self-update: own container has no config")
	}
	name := containerName([]string{selfInsp.Name})

	if err := m.st.MarkUpdateJobSelf(jobID); err != nil {
		log.Printf("self-update: mark job: %v", err)
	}

	cfg := &container.Config{
		Image: selfInsp.Image,
		// Take the entrypoint from the container we are running in rather than
		// assuming the image's default: it is by definition the command that
		// starts this binary.
		Entrypoint: selfInsp.Config.Entrypoint,
		Cmd:        []string{"apply-update", "--container", name, "--job", jobID},
		Env:        selfInsp.Config.Env,
		// The helper has to reach the Docker socket, and reaching it is exactly
		// what the user's choice of uid/gid on *this* container is for. The image
		// default is not enough: it runs as nonroot, which cannot open the socket.
		User:       selfInsp.Config.User,
		WorkingDir: selfInsp.Config.WorkingDir,
		Labels: map[string]string{
			updaterLabel: updaterRole,
			jobLabel:     jobID,
		},
		// The image's healthcheck probes the HTTP server, which the helper does
		// not run.
		Healthcheck: &container.HealthConfig{Test: []string{"NONE"}},
	}
	// Carry over the mounts (the Docker socket and the data volume) but nothing
	// that would collide with the Veery container still running right now:
	// published ports would fail to bind, and a restart policy would resurrect
	// the helper forever.
	host := &container.HostConfig{
		Binds:       selfInsp.HostConfig.Binds,
		Mounts:      selfInsp.HostConfig.Mounts,
		VolumesFrom: selfInsp.HostConfig.VolumesFrom,
		// Whatever grants this container access to the Docker socket has to grant
		// the helper the same, or it cannot do the one thing it exists to do.
		GroupAdd:      selfInsp.HostConfig.GroupAdd,
		Privileged:    selfInsp.HostConfig.Privileged,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
	}

	hname := updaterName(jobID)
	_ = m.cli.ContainerRemove(ctx, hname, container.RemoveOptions{Force: true})
	created, err := m.cli.ContainerCreate(ctx, cfg, host, nil, nil, hname)
	if err != nil {
		return fmt.Errorf("create updater: %w", err)
	}
	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		_ = m.cli.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("start updater: %w", err)
	}

	emit("handoff", "Veery is restarting to finish its own update")
	return nil
}

// ApplyUpdate performs the swap for a container from *outside* it. This is what
// the helper container runs; the Veery that scheduled the update is stopped
// partway through, by this very call.
func (m *Manager) ApplyUpdate(ctx context.Context, name, jobID string) error {
	mc, err := m.st.ManagedByName(name)
	if err != nil {
		return fmt.Errorf("not a managed container: %s", name)
	}

	oldInsp, err := m.cli.ContainerInspect(ctx, name)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", name, err)
	}

	// The helper is the only thing touching this container, so no lock is taken;
	// the Veery that handed off is on its way out and does nothing more to it.
	mc, snap, err := m.refreshIfRecreated(mc, oldInsp)
	if err != nil {
		return err
	}
	ref := snap.Image
	if ref == "" {
		return fmt.Errorf("snapshot has no image reference")
	}

	emit := func(phase, msg string) {
		log.Printf("apply-update: %s: %s", phase, msg)
		if jobID != "" {
			_ = m.st.SetUpdateJobPhase(jobID, phase, msg)
		}
	}

	// The scheduling Veery already pulled this, but it may have died between the
	// pull and the handoff, and the image is what the whole update is for.
	newImg, err := m.cli.ImageInspect(ctx, ref)
	if err != nil {
		if newImg, err = m.pullImage(ctx, ref, emit); err != nil {
			return m.finishApply(jobID, name, err)
		}
	}
	if newImg.ID == oldInsp.Image {
		emit("up-to-date", "Already up to date")
		return m.finishApply(jobID, name, nil)
	}

	err = m.swap(ctx, mc, snap, ref, oldInsp, newImg.ID, emit)
	return m.finishApply(jobID, name, err)
}

// finishApply records the outcome of a helper-run update and notifies.
func (m *Manager) finishApply(jobID, name string, err error) error {
	if err != nil {
		if jobID != "" {
			_ = m.st.FinishUpdateJob(jobID, "failed", "", err.Error())
		}
		m.notify(api.EventUpdateApplied, "Update failed: "+name, err.Error(),
			api.EventMeta{ContainerName: name})
		return err
	}
	if jobID != "" {
		_ = m.st.FinishUpdateJob(jobID, "done", "Updated", "")
	}
	m.notify(api.EventUpdateApplied, "Updated "+name, "The container is running a newer image and came up healthy.",
		api.EventMeta{ContainerName: name})
	return nil
}

// runningUpdaters returns the helper containers currently performing a swap,
// keyed by the job id they are finishing. While one is running it owns the
// containers it is swapping, and startup recovery must keep its hands off them.
func (m *Manager) runningUpdaters(ctx context.Context) (map[string]container.Summary, error) {
	list, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	out := map[string]container.Summary{}
	for _, c := range list {
		if c.Labels[updaterLabel] != updaterRole || c.State != "running" {
			continue
		}
		out[c.Labels[jobLabel]] = c
	}
	return out, nil
}

// pruneUpdaters removes finished helper containers. Their logs are the only
// record of a failed self-update, so they are left behind until the next start.
func (m *Manager) pruneUpdaters(ctx context.Context) {
	list, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return
	}
	for _, c := range list {
		if c.Labels[updaterLabel] == updaterRole && c.State != "running" {
			_ = m.cli.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
		}
	}
}

