package docker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
)

const projectLabel = "com.docker.compose.project"
const serviceLabel = "com.docker.compose.service"

func genID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ListStacks discovers containers, groups them into stacks, merges managed
// state from the DB, and computes friendly statuses.
func (m *Manager) ListStacks(ctx context.Context) ([]api.Stack, error) {
	summaries, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, err
	}
	managedAll, err := m.st.AllManaged()
	if err != nil {
		return nil, err
	}
	managedByName := map[string]store.ManagedContainer{}
	for _, mc := range managedAll {
		managedByName[mc.ContainerName] = mc
	}

	stacks := map[string]*api.Stack{}
	live := map[string]bool{}
	getStack := func(id string) *api.Stack {
		if s, ok := stacks[id]; ok {
			return s
		}
		s := &api.Stack{ID: id, Name: id}
		stacks[id] = s
		return s
	}

	for _, c := range summaries {
		if !isService(c) {
			continue
		}
		name := containerName(c.Names)
		live[name] = true
		proj := c.Labels[projectLabel]
		mc, isManaged := managedByName[name]
		if proj == "" {
			if isManaged {
				proj = mc.StackID
			} else {
				proj = name
			}
		}
		st := getStack(proj)
		cont := buildContainer(c, name, isManaged, mc)
		if isManaged {
			cont.UpdateAvailable = m.updateAvailableFor(name)
		}
		// RestartCount is only interesting for troubled containers, so inspect
		// (one extra call) just for those rather than for every container.
		if cont.Status == api.StatusNeedsAttention {
			if insp, ierr := m.cli.ContainerInspect(ctx, c.ID); ierr == nil {
				cont.RestartCount = insp.RestartCount
			}
		}
		st.Containers = append(st.Containers, cont)
	}

	// Managed containers not present live are "missing" (removed / host reboot).
	for _, mc := range managedAll {
		if live[mc.ContainerName] {
			continue
		}
		st := getStack(mc.StackID)
		st.Containers = append(st.Containers, api.Container{
			ID:            mc.ID,
			Name:          mc.ContainerName,
			ContainerName: mc.ContainerName,
			Status:        api.StatusMissing,
			State:         "missing",
			Managed:       true,
			AutoUpdate:    mc.AutoUpdate,
		})
	}

	out := make([]api.Stack, 0, len(stacks))
	for _, st := range stacks {
		finalizeStack(st)
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func containerName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.TrimPrefix(names[0], "/")
}

func buildContainer(c container.Summary, name string, managed bool, mc store.ManagedContainer) api.Container {
	friendly := name
	if svc := c.Labels[serviceLabel]; svc != "" {
		friendly = svc
	}
	health := ""
	lower := strings.ToLower(c.Status)
	if strings.Contains(lower, "unhealthy") {
		health = "unhealthy"
	} else if strings.Contains(lower, "healthy") {
		health = "healthy"
	}
	return api.Container{
		ID:            c.ID,
		Name:          friendly,
		ContainerName: name,
		Image:         c.Image,
		State:         c.State,
		Status:        mapStatus(c.State, health, exitCodeFromStatus(c.Status)),
		Health:        health,
		Managed:       managed,
		AutoUpdate:    mc.AutoUpdate,
		CreatedAt:     c.Created,
	}
}

// exitCodeFromStatus parses the exit code out of a summary status string like
// "Exited (137) 2 minutes ago". Returns -1 when no exit code is present.
func exitCodeFromStatus(status string) int {
	open := strings.IndexByte(status, '(')
	if open < 0 {
		return -1
	}
	close := strings.IndexByte(status[open:], ')')
	if close < 0 {
		return -1
	}
	code, err := strconv.Atoi(strings.TrimSpace(status[open+1 : open+close]))
	if err != nil {
		return -1
	}
	return code
}

func mapStatus(state, health string, exitCode int) api.ContainerStatus {
	switch state {
	case "running":
		if health == "unhealthy" {
			return api.StatusNeedsAttention
		}
		return api.StatusRunning
	case "restarting", "dead":
		return api.StatusNeedsAttention
	case "exited":
		// A non-zero exit is a crash: surface it as needing attention. A clean
		// exit (0) is an intentional stop.
		if exitCode > 0 {
			return api.StatusNeedsAttention
		}
		return api.StatusStopped
	case "paused", "created", "removing":
		return api.StatusStopped
	default:
		return api.StatusStopped
	}
}

func finalizeStack(st *api.Stack) {
	sort.Slice(st.Containers, func(i, j int) bool { return st.Containers[i].Name < st.Containers[j].Name })
	anyManaged := false
	needsAttention, updating, running, missing, total := false, false, 0, 0, len(st.Containers)
	for _, c := range st.Containers {
		if c.Managed {
			anyManaged = true
		}
		switch c.Status {
		case api.StatusNeedsAttention:
			needsAttention = true
		case api.StatusMissing:
			missing++
		case api.StatusUpdating:
			updating = true
		case api.StatusRunning:
			running++
		}
	}
	st.Managed = anyManaged
	switch {
	// Every container gone is a service that was taken down whole (a compose
	// down), which is a thing the user did on purpose and can undo with bring
	// up. Only a container missing from a service whose other parts are still
	// running is a surprise worth flagging.
	case total > 0 && missing == total:
		st.Status = api.StatusMissing
	case needsAttention || missing > 0:
		st.Status = api.StatusNeedsAttention
	case updating:
		st.Status = api.StatusUpdating
	case total > 0 && running == total:
		st.Status = api.StatusRunning
	default:
		st.Status = api.StatusStopped
	}
}

// --- Adoption ---

// Adopt captures create-spec snapshots for every container in a discovered
// stack and persists them as managed.
func (m *Manager) Adopt(ctx context.Context, stackID string) error {
	summaries, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return err
	}
	stack, err := m.st.UpsertStack(stackID)
	if err != nil {
		return err
	}
	found := 0
	for _, c := range summaries {
		name := containerName(c.Names)
		proj := c.Labels[projectLabel]
		if proj == "" {
			proj = name
		}
		if proj != stackID {
			continue
		}
		insp, err := m.cli.ContainerInspect(ctx, c.ID)
		if err != nil {
			return err
		}
		snap := snapshotFromInspect(insp)
		js, err := snap.marshal()
		if err != nil {
			return err
		}
		id := genID()
		if existing, err := m.st.ManagedByName(name); err == nil {
			id = existing.ID
		}
		if err := m.st.AddManagedContainer(store.ManagedContainer{
			ID:            id,
			StackID:       stack.ID,
			ContainerName: name,
			SnapshotJSON:  js,
			ContainerID:   insp.ID,
			CreatedAt:     time.Now().Unix(),
		}); err != nil {
			return err
		}
		found++
	}
	if found == 0 {
		return fmt.Errorf("no containers found for stack %q", stackID)
	}
	m.BroadcastStacks(ctx)
	return nil
}

// --- Lifecycle ---

// Start starts a container by id or name.
func (m *Manager) Start(ctx context.Context, id string) error {
	return m.cli.ContainerStart(ctx, id, container.StartOptions{})
}

// Stop stops a container.
func (m *Manager) Stop(ctx context.Context, id string) error {
	return m.cli.ContainerStop(ctx, id, container.StopOptions{})
}

// Restart restarts a container.
func (m *Manager) Restart(ctx context.Context, id string) error {
	return m.cli.ContainerRestart(ctx, id, container.StopOptions{})
}

// BringUpStack recreates missing containers from snapshot and starts stopped
// ones, bringing a managed stack to its good state.
func (m *Manager) BringUpStack(ctx context.Context, stackID string) error {
	managed, err := m.st.ManagedByStack(stackID)
	if err != nil {
		return err
	}
	if len(managed) == 0 {
		return fmt.Errorf("stack %q is not managed", stackID)
	}
	for _, mc := range managed {
		insp, err := m.cli.ContainerInspect(ctx, mc.ContainerName)
		if err != nil {
			// Missing: recreate from snapshot.
			snap, perr := parseSnapshot(mc.SnapshotJSON)
			if perr != nil {
				return perr
			}
			newID, rerr := m.recreate(ctx, snap, "")
			if rerr != nil {
				return fmt.Errorf("recreate %s: %w", mc.ContainerName, rerr)
			}
			// The spec is unchanged, but it now lives in a different container.
			if uerr := m.st.UpdateSnapshot(mc.ID, mc.SnapshotJSON, newID); uerr != nil {
				log.Printf("bringup %s: record new container: %v", mc.ContainerName, uerr)
			}
			continue
		}
		if !insp.State.Running {
			if err := m.cli.ContainerStart(ctx, insp.ID, container.StartOptions{}); err != nil {
				return fmt.Errorf("start %s: %w", mc.ContainerName, err)
			}
		}
	}
	m.BroadcastStacks(ctx)
	return nil
}

// BroadcastStacks pushes the current stack list to all WS clients, and notifies
// on any managed container that changed status since the last sweep.
func (m *Manager) BroadcastStacks(ctx context.Context) {
	stacks, err := m.ListStacks(ctx)
	if err != nil {
		return
	}
	m.noteStatuses(stacks)
	if m.pub == nil {
		return
	}
	m.pub.Broadcast(api.WSMessage{Type: api.WSTypeStacks, Stacks: stacks})
}
