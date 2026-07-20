// Package api holds the domain and transport types that are the single source
// of truth for both the Go backend and the TypeScript frontend. TS types are
// generated from this file with tygo (see tygo.yaml and `go generate`).
package api

//go:generate go run github.com/gzuidhof/tygo@v0.2.17 generate

// ContainerStatus is the friendly, jargon-light state shown in the UI.
type ContainerStatus string

const (
	StatusRunning        ContainerStatus = "running"
	StatusStopped        ContainerStatus = "stopped"
	StatusNeedsAttention ContainerStatus = "needs_attention"
	StatusUpdating       ContainerStatus = "updating"
	StatusMissing        ContainerStatus = "missing"
)

// User is an account. Passkey-only; no password ever stored.
type User struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsAdmin   bool   `json:"isAdmin"`
	CreatedAt int64  `json:"createdAt"`
}

// Credential is one registered passkey belonging to a user.
type Credential struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

// Invite is a single-use, expiring enrollment link.
type Invite struct {
	Token     string `json:"token"`
	IsAdmin   bool   `json:"isAdmin"`
	ExpiresAt int64  `json:"expiresAt"`
	UsedAt    int64  `json:"usedAt"`
	URL       string `json:"url"`
	// ForUserName is set on recovery invites: the display name of the existing
	// user this link re-enrolls a passkey for. Empty on normal invites.
	ForUserName string `json:"forUserName,omitempty"`
}

// Container is a single managed or discovered container.
type Container struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	ContainerName   string          `json:"containerName"`
	Image           string          `json:"image"`
	State           string          `json:"state"`
	Status          ContainerStatus `json:"status"`
	Health          string          `json:"health"`
	Managed         bool            `json:"managed"`
	AutoUpdate      bool            `json:"autoUpdate"`
	UpdateAvailable bool            `json:"updateAvailable"`
	RestartCount    int             `json:"restartCount"`
	CreatedAt       int64           `json:"createdAt"`
}

// Stack groups containers by compose project (or manual grouping).
type Stack struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Managed    bool            `json:"managed"`
	Status     ContainerStatus `json:"status"`
	Containers []Container     `json:"containers"`
}

// DiskUsage is per-filesystem capacity usage. Key is the stable visibility key
// (see DiskItem) so the UI can tie a gauge back to its config toggle.
type DiskUsage struct {
	Key   string `json:"key"`
	Mount string `json:"mount"`
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// DiskActivity is one physical device's read/write throughput. Peaks are the
// highest rates ever recorded for that device, persisted across restarts; the
// UI colours the current rate against these highwater marks.
type DiskActivity struct {
	Key    string `json:"key"`
	Device string `json:"device"`
	// Label names the volumes backed by this device (e.g. "Main disk"), derived
	// from /proc on Linux. Empty when the device can't be tied to a mount (e.g.
	// macOS synthesized APFS containers); the UI then falls back to Device.
	Label                string `json:"label"`
	ReadBytesPerSec      uint64 `json:"readBytesPerSec"`
	WriteBytesPerSec     uint64 `json:"writeBytesPerSec"`
	ReadPeakBytesPerSec  uint64 `json:"readPeakBytesPerSec"`
	WritePeakBytesPerSec uint64 `json:"writePeakBytesPerSec"`
}

// DiskKind distinguishes the two disconnected views a disk can appear in:
// capacity is per mount, activity is per physical device.
type DiskKind string

const (
	DiskCapacity DiskKind = "capacity"
	DiskActivityKind DiskKind = "activity"
)

// DiskItem is one configurable entry in the "which disks to show" settings. It
// covers both a capacity mount and an activity device; Kind says which.
type DiskItem struct {
	Key          string   `json:"key"`
	Kind         DiskKind `json:"kind"`
	Label        string   `json:"label"`
	Detail       string   `json:"detail"`
	Shown        bool     `json:"shown"`
	DefaultShown bool     `json:"defaultShown"`
}

// HostMetrics is a snapshot of host-level resource use. Disks and DiskActivity
// carry only the entries the current visibility settings leave shown.
type HostMetrics struct {
	CPUPercent   float64        `json:"cpuPercent"`
	MemUsed      uint64         `json:"memUsed"`
	MemTotal     uint64         `json:"memTotal"`
	Disks        []DiskUsage    `json:"disks"`
	DiskActivity []DiskActivity `json:"diskActivity"`
}

// ContainerMetrics is a snapshot of one container's resource use.
type ContainerMetrics struct {
	ID         string  `json:"id"`
	CPUPercent float64 `json:"cpuPercent"`
	MemUsed    uint64  `json:"memUsed"`
	MemLimit   uint64  `json:"memLimit"`
}

// MetricsSnapshot is pushed over the WS on an interval.
type MetricsSnapshot struct {
	Host       HostMetrics        `json:"host"`
	Containers []ContainerMetrics `json:"containers"`
	At         int64              `json:"at"`
}

// JobProgress reports the lifecycle of a long-running action over the WS.
type JobProgress struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Done    bool   `json:"done"`
	Error   string `json:"error"`
}

// WSMessageType tags the WS envelope.
type WSMessageType string

const (
	WSTypeMetrics WSMessageType = "metrics"
	WSTypeStacks  WSMessageType = "stacks"
	WSTypeJob     WSMessageType = "job"
	// WSTypeJobs is the whole job picture, sent once when a client connects: the
	// updates in flight plus the ones that just finished. A client that was away
	// (or was never here) has no other way to learn about an update that started,
	// or finished, without it.
	WSTypeJobs WSMessageType = "jobs"
	// WSTypeEvent carries one freshly recorded event-log entry, pushed as it is
	// written so the log page can prepend it live. It is only sent to admin
	// clients, since the log includes auth events that name users.
	WSTypeEvent WSMessageType = "event"
)

// WSMessage is the server→client push envelope. Exactly one payload is set.
type WSMessage struct {
	Type    WSMessageType    `json:"type"`
	Metrics *MetricsSnapshot `json:"metrics,omitempty"`
	Stacks  []Stack          `json:"stacks,omitempty"`
	Job     *JobProgress     `json:"job,omitempty"`
	Jobs    []JobProgress    `json:"jobs,omitempty"`
	Event   *Event           `json:"event,omitempty"`
}

// --- HTTP request/response bodies ---

// SessionInfo is returned by /auth/me for the current session.
type SessionInfo struct {
	User        User  `json:"user"`
	Credentials []Credential `json:"credentials"`
}

// CreateInviteRequest asks for a new enrollment link.
type CreateInviteRequest struct {
	IsAdmin bool `json:"isAdmin"`
}

// EnrollRequest carries the invite token and chosen passkey/user name.
type EnrollRequest struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// AdoptRequest adopts a discovered stack into management.
type AdoptRequest struct {
	StackID string `json:"stackId"`
}

// SetAutoUpdateRequest toggles auto-update for a container.
type SetAutoUpdateRequest struct {
	ContainerID string `json:"containerId"`
	AutoUpdate  bool   `json:"autoUpdate"`
}

// Settings are the mutable app settings.
type Settings struct {
	PollIntervalSeconds int  `json:"pollIntervalSeconds"`
	AutoUpdateDefault   bool `json:"autoUpdateDefault"`
	AutoUpdateIntervalMinutes int `json:"autoUpdateIntervalMinutes"`
	// EventLogRetentionDays bounds how long recorded events are kept. A
	// crash-looping container can generate events without end, so the log is
	// pruned to this many days on write. Zero means keep forever.
	EventLogRetentionDays int `json:"eventLogRetentionDays"`
	// DiskVisibility overrides the default shown/hidden state per disk key. Keys
	// absent here fall back to the built-in heuristic. Applies to all users.
	DiskVisibility map[string]bool `json:"diskVisibility"`
}

// NotificationEvent names a kind of event that can be delivered to the
// configured notification channels.
type NotificationEvent string

const (
	// EventUpdateApplied fires when an auto-update or a manual update finishes,
	// whether it succeeded or was rolled back.
	EventUpdateApplied NotificationEvent = "update_applied"
	// EventUpdateAvailable fires when a newer image appears for a managed
	// container that does not have auto-update enabled.
	EventUpdateAvailable NotificationEvent = "update_available"
	// EventContainerStatus fires when a managed container changes status, e.g.
	// starts crash-looping, goes unhealthy, stops, or recovers.
	EventContainerStatus NotificationEvent = "container_status"
	// EventContainerMissing fires when a managed container is removed from the
	// host. It is separate from EventContainerStatus because removing one is
	// usually the user's own doing (a compose file edit, a compose down), so it
	// is the event most worth turning off on a host that changes often.
	EventContainerMissing NotificationEvent = "container_missing"
	// EventContainerAdopted fires when Veery takes over a container that
	// appeared in a stack it already manages, which it does on its own.
	EventContainerAdopted NotificationEvent = "container_adopted"
	// EventAuth fires on passkey enrollment, logins and other account changes.
	EventAuth NotificationEvent = "auth"
)

// AllNotificationEvents lists every event in display order.
var AllNotificationEvents = []NotificationEvent{
	EventContainerStatus,
	EventContainerMissing,
	EventContainerAdopted,
	EventUpdateApplied,
	EventUpdateAvailable,
	EventAuth,
}

// Event is one recorded entry in the event log: a copy of something Veery
// notified about, kept whether or not it was actually delivered. Muting a
// channel stops delivery, not recording, so the log stays a complete history of
// what happened to each service.
type Event struct {
	ID    int64             `json:"id"`
	Event NotificationEvent `json:"event"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	// ContainerName and StackID tie the entry to the service it concerns, so the
	// UI can link a row back to it. Both are empty for events that name no
	// service (e.g. auth events).
	ContainerName string `json:"containerName"`
	StackID       string `json:"stackId"`
	CreatedAt     int64  `json:"createdAt"`
}

// EventMeta is the optional service context an event carries into the notifier,
// which records it on the log row. Kept separate from Event so a caller only
// has to name the container or stack, not build a whole row.
type EventMeta struct {
	ContainerName string
	StackID       string
}

// EventPage is a cursor-paginated slice of the event log, newest first.
// NextCursor is empty once the oldest recorded event has been returned.
type EventPage struct {
	Items      []Event `json:"items"`
	NextCursor string  `json:"nextCursor"`
}

// NotificationConfig is where notifications go and which events are sent.
type NotificationConfig struct {
	// URLs are Shoutrrr service URLs, one per target, e.g.
	// "discord://token@channel" or "ntfy://ntfy.sh/my-topic". They carry
	// credentials, so this config is admin-only.
	URLs []string `json:"urls"`
	// Events maps each NotificationEvent to whether it is delivered. Events
	// absent from the map are treated as enabled.
	Events map[NotificationEvent]bool `json:"events"`
	// EnvManaged reports that the config comes from VEERY_NOTIFY_URLS and so
	// cannot be edited through the UI.
	EnvManaged bool `json:"envManaged"`
}

// Enabled reports whether an event should be delivered.
func (c NotificationConfig) Enabled(ev NotificationEvent) bool {
	on, ok := c.Events[ev]
	return !ok || on
}

// TestNotificationRequest sends a test message. URLs, when non-empty, are used
// instead of the saved ones so a channel can be checked before it is saved.
type TestNotificationRequest struct {
	URLs []string `json:"urls"`
}

// SetDiskVisibilityRequest updates which disks are shown, for all users.
type SetDiskVisibilityRequest struct {
	Visibility map[string]bool `json:"visibility"`
}

// APIError is the shape of error responses.
type APIError struct {
	Error string `json:"error"`
}
