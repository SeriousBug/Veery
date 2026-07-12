package docker

import (
	"context"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/docker/docker/api/types/container"
)

// job broadcasts progress for a long-running action and runs fn, reporting a
// final done/failed message and refreshing stacks afterward.
func (m *Manager) job(ctx context.Context, kind, target string, fn func(emit func(phase, msg string)) error) {
	id := genID()
	emit := func(phase, msg string) {
		m.publish(api.JobProgress{ID: id, Kind: kind, Target: target, Phase: phase, Message: msg})
	}
	emit("start", kind+" "+target)
	if err := fn(emit); err != nil {
		m.publish(api.JobProgress{ID: id, Kind: kind, Target: target, Phase: "failed", Done: true, Error: err.Error()})
		return
	}
	m.publish(api.JobProgress{ID: id, Kind: kind, Target: target, Phase: "done", Done: true, Message: "Done"})
	m.BroadcastStacks(ctx)
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
