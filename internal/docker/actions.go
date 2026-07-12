package docker

import "context"

// Job-wrapped lifecycle actions. Each runs the operation through job() so
// progress (start/done/failed) is broadcast over the WS and stacks are
// refreshed afterward.

// StartJob starts a container by id/name, reporting progress.
func (m *Manager) StartJob(ctx context.Context, id string) {
	m.job(ctx, "start", id, func(emit func(phase, msg string)) error {
		return m.withContainerLock(ctx, id, func() error { return m.Start(ctx, id) })
	})
}

// StopJob stops a container, reporting progress.
func (m *Manager) StopJob(ctx context.Context, id string) {
	m.job(ctx, "stop", id, func(emit func(phase, msg string)) error {
		return m.withContainerLock(ctx, id, func() error { return m.Stop(ctx, id) })
	})
}

// RestartJob restarts a container, reporting progress.
func (m *Manager) RestartJob(ctx context.Context, id string) {
	m.job(ctx, "restart", id, func(emit func(phase, msg string)) error {
		return m.withContainerLock(ctx, id, func() error { return m.Restart(ctx, id) })
	})
}

// StartStackJob starts every container in a stack, reporting progress.
func (m *Manager) StartStackJob(ctx context.Context, stackID string) {
	m.job(ctx, "start", stackID, func(emit func(phase, msg string)) error {
		return m.StartStack(ctx, stackID)
	})
}

// StopStackJob stops every container in a stack, reporting progress.
func (m *Manager) StopStackJob(ctx context.Context, stackID string) {
	m.job(ctx, "stop", stackID, func(emit func(phase, msg string)) error {
		return m.StopStack(ctx, stackID)
	})
}

// RestartStackJob restarts every container in a stack, reporting progress.
func (m *Manager) RestartStackJob(ctx context.Context, stackID string) {
	m.job(ctx, "restart", stackID, func(emit func(phase, msg string)) error {
		return m.RestartStack(ctx, stackID)
	})
}

// BringUpStackJob brings a managed stack to its good state, reporting progress.
func (m *Manager) BringUpStackJob(ctx context.Context, stackID string) {
	m.job(ctx, "bringup", stackID, func(emit func(phase, msg string)) error {
		return m.BringUpStack(ctx, stackID)
	})
}
