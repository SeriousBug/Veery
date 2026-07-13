package docker

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/docker/docker/api/types/container"
)

// errHandedOff ends a job without a terminal message: another process (the
// self-update helper container) has taken ownership of it and will report the
// outcome. This process is about to be stopped.
var errHandedOff = errors.New("handed off to the updater")

// job broadcasts progress for a long-running action and runs fn, reporting a
// final done/failed message and refreshing stacks afterward.
func (m *Manager) job(ctx context.Context, kind, target string, fn func(emit func(phase, msg string)) error) {
	m.jobWithID(ctx, genID(), kind, target, fn)
}

// jobWithID is job() with a caller-supplied id, so an update can persist its
// progress under an id a later process can pick back up.
func (m *Manager) jobWithID(ctx context.Context, id, kind, target string, fn func(emit func(phase, msg string)) error) {
	emit := func(phase, msg string) {
		m.track(api.JobProgress{ID: id, Kind: kind, Target: target, Phase: phase, Message: msg})
	}
	emit("start", kind+" "+target)

	err := fn(emit)
	if errors.Is(err, errHandedOff) {
		return
	}
	m.forget(id)
	if err != nil {
		m.publish(api.JobProgress{ID: id, Kind: kind, Target: target, Phase: "failed", Done: true, Error: err.Error()})
		return
	}
	m.publish(api.JobProgress{ID: id, Kind: kind, Target: target, Phase: "done", Done: true, Message: "Done"})
	m.BroadcastStacks(ctx)
}

// track records a job as in flight and broadcasts it, so a client that connects
// mid-job can be replayed the progress it missed.
func (m *Manager) track(j api.JobProgress) {
	m.jobsMu.Lock()
	m.activeJobs[j.ID] = j
	m.jobsMu.Unlock()
	m.publish(j)
}

func (m *Manager) forget(id string) {
	m.jobsMu.Lock()
	delete(m.activeJobs, id)
	m.jobsMu.Unlock()
}

// ActiveJobs returns the jobs currently in flight, for replay to a new client.
func (m *Manager) ActiveJobs() []api.JobProgress {
	m.jobsMu.Lock()
	defer m.jobsMu.Unlock()
	out := make([]api.JobProgress, 0, len(m.activeJobs))
	for _, j := range m.activeJobs {
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// recentWindow is how far back a finished update stays interesting to a client
// that just connected. It only has to cover the gap a Veery restart leaves.
const recentWindow = 5 * time.Minute

// JobSnapshot is the job picture handed to a client on connect: everything in
// flight, plus updates that finished recently enough that the client may have
// been disconnected for them (a self-update finishes with nobody listening).
func (m *Manager) JobSnapshot() []api.JobProgress {
	out := m.ActiveJobs()
	active := map[string]bool{}
	for _, j := range out {
		active[j.ID] = true
	}

	recent, err := m.st.RecentUpdateJobs(time.Now().Add(-recentWindow).Unix())
	if err != nil {
		return out
	}
	for _, j := range recent {
		if active[j.ID] {
			continue
		}
		out = append(out, api.JobProgress{
			ID: j.ID, Kind: "update", Target: j.ContainerName,
			Phase: j.Phase, Message: j.Message, Error: j.Error, Done: true,
		})
	}
	return out
}

func (m *Manager) publish(j api.JobProgress) {
	if m.pub != nil {
		m.pub.Broadcast(api.WSMessage{Type: api.WSTypeJob, Job: &j})
	}
}

// containerIDsInStack returns live container ids belonging to a stack.
func (m *Manager) containerIDsInStack(ctx context.Context, stackID string) ([]string, error) {
	summaries, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range summaries {
		name := containerName(c.Names)
		proj := c.Labels[projectLabel]
		if proj == "" {
			proj = name
		}
		if proj == stackID {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}

// StartStack starts every container in a stack.
func (m *Manager) StartStack(ctx context.Context, stackID string) error {
	ids, err := m.containerIDsInStack(ctx, stackID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := m.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// StopStack stops every container in a stack.
func (m *Manager) StopStack(ctx context.Context, stackID string) error {
	ids, err := m.containerIDsInStack(ctx, stackID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := m.cli.ContainerStop(ctx, id, container.StopOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// RestartStack restarts every container in a stack.
func (m *Manager) RestartStack(ctx context.Context, stackID string) error {
	ids, err := m.containerIDsInStack(ctx, stackID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := m.cli.ContainerRestart(ctx, id, container.StopOptions{}); err != nil {
			return err
		}
	}
	return nil
}
