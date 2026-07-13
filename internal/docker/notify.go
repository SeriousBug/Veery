package docker

import (
	"fmt"
	"log"

	"github.com/SeriousBug/Veery/internal/api"
)

// Notifier receives notable events for delivery to the user's notification
// channels. It is optional: a nil notifier disables notifications.
type Notifier interface {
	Notify(ev api.NotificationEvent, title, body string)
}

// SetNotifier attaches the notifier. Like SetDocker on the server, it is set
// after construction so the constructor signature stays stable for tests.
func (m *Manager) SetNotifier(n Notifier) { m.notif = n }

func (m *Manager) notify(ev api.NotificationEvent, title, body string) {
	if m.notif != nil {
		m.notif.Notify(ev, title, body)
	}
}

// noteStatuses compares the statuses of managed containers against the previous
// sweep and notifies on each transition. Only managed containers are watched:
// an unadopted container is not something Veery promised to keep alive, and on
// a busy host they would be pure noise.
//
// The previous sweep is persisted, so a container that is newly seen — because
// this is the first sweep ever, or because it was just adopted — is only
// recorded, never announced.
func (m *Manager) noteStatuses(stacks []api.Stack) {
	m.statusMu.Lock()
	defer m.statusMu.Unlock()

	if !m.statusBaseline {
		saved, err := m.st.LoadNotifiedStatuses()
		if err != nil {
			log.Printf("notify: load last statuses: %v", err)
			return
		}
		m.lastStatus = saved
		m.statusBaseline = true
	}

	seen := map[string]api.ContainerStatus{}
	for _, st := range stacks {
		var changed, gone []api.Container
		managed := 0
		for _, c := range st.Containers {
			if !c.Managed {
				continue
			}
			managed++
			seen[c.ContainerName] = c.Status
			prev, known := m.lastStatus[c.ContainerName]
			// StatusUpdating is the transient state of an in-flight job, and
			// the update reports its own outcome.
			if !known || prev == c.Status || prev == api.StatusUpdating || c.Status == api.StatusUpdating {
				continue
			}
			if c.Status == api.StatusMissing {
				gone = append(gone, c)
				continue
			}
			changed = append(changed, c)
		}

		for _, c := range changed {
			if title, body := statusMessage(c, m.lastStatus[c.ContainerName]); title != "" {
				m.notify(api.EventContainerStatus, title, body)
			}
		}
		for _, msg := range removalMessages(st, gone, managed) {
			m.notify(api.EventContainerMissing, msg.title, msg.body)
		}
	}
	if sameStatuses(m.lastStatus, seen) {
		return
	}
	m.lastStatus = seen
	if err := m.st.SaveNotifiedStatuses(seen); err != nil {
		log.Printf("notify: save last statuses: %v", err)
	}
}

func sameStatuses(a, b map[string]api.ContainerStatus) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

type message struct{ title, body string }

// removalMessages describes the managed containers of one stack that were
// removed from the host since the last sweep.
//
// A whole stack going at once is a `compose down` or a stack the user deleted:
// one thing they did, so it is one message, not one per container. A container
// removed from a stack whose other parts are still there is the odd one out,
// and worth naming.
func removalMessages(st api.Stack, gone []api.Container, managed int) []message {
	switch {
	case len(gone) == 0:
		return nil
	case len(gone) > 1 && len(gone) == managed:
		return []message{{
			title: st.Name + " was taken down",
			body: fmt.Sprintf("All %d of its containers were removed from this machine. "+
				"Bring it back up to recreate them, or forget it if you meant to get rid of it.", len(gone)),
		}}
	}
	out := make([]message, 0, len(gone))
	for _, c := range gone {
		out = append(out, message{
			title: c.ContainerName + " was removed",
			body: "The container no longer exists on this machine. It may have been removed outside Veery. " +
				"Bring it back up to recreate it, or forget it if you meant to remove it.",
		})
	}
	return out
}

// statusMessage describes a status transition, or returns an empty title when
// the transition is not worth a notification. Removals are not handled here:
// they are reported per stack, by removalMessages.
func statusMessage(c api.Container, prev api.ContainerStatus) (title, body string) {
	switch c.Status {
	case api.StatusNeedsAttention:
		title = c.ContainerName + " needs attention"
		switch {
		case c.Health == "unhealthy":
			body = "The container is running but its health check is failing."
		case c.RestartCount > 0:
			body = fmt.Sprintf("The container is crash-looping (%d restarts). Last state: %s.", c.RestartCount, c.State)
		default:
			body = "The container stopped unexpectedly. Last state: " + c.State + "."
		}
	case api.StatusStopped:
		title = c.ContainerName + " stopped"
		body = "The container is no longer running."
	case api.StatusRunning:
		title = c.ContainerName + " is running"
		body = "The container is back up, after being " + friendlyStatus(prev) + "."
	}
	return title, body
}

func friendlyStatus(s api.ContainerStatus) string {
	switch s {
	case api.StatusNeedsAttention:
		return "in trouble"
	case api.StatusMissing:
		return "missing"
	default:
		return "stopped"
	}
}
