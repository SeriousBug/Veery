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
		for _, c := range st.Containers {
			if !c.Managed {
				continue
			}
			seen[c.ContainerName] = c.Status
			prev, known := m.lastStatus[c.ContainerName]
			// StatusUpdating is the transient state of an in-flight job, and
			// the update reports its own outcome.
			if !known || prev == c.Status || prev == api.StatusUpdating || c.Status == api.StatusUpdating {
				continue
			}
			if title, body := statusMessage(c, prev); title != "" {
				m.notify(api.EventContainerStatus, title, body)
			}
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

// statusMessage describes a status transition, or returns an empty title when
// the transition is not worth a notification.
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
	case api.StatusMissing:
		title = c.ContainerName + " has gone missing"
		body = "The container no longer exists on the host. It may have been removed outside Veery."
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
