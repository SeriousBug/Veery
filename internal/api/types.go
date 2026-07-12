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

// DiskUsage is per-filesystem usage.
type DiskUsage struct {
	Mount string `json:"mount"`
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// HostMetrics is a snapshot of host-level resource use.
type HostMetrics struct {
	CPUPercent           float64     `json:"cpuPercent"`
	MemUsed              uint64      `json:"memUsed"`
	MemTotal             uint64      `json:"memTotal"`
	Disks                []DiskUsage `json:"disks"`
	DiskReadBytesPerSec  uint64      `json:"diskReadBytesPerSec"`
	DiskWriteBytesPerSec uint64      `json:"diskWriteBytesPerSec"`
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
)

// WSMessage is the server→client push envelope. Exactly one payload is set.
type WSMessage struct {
	Type    WSMessageType    `json:"type"`
	Metrics *MetricsSnapshot `json:"metrics,omitempty"`
	Stacks  []Stack          `json:"stacks,omitempty"`
	Job     *JobProgress     `json:"job,omitempty"`
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
}

// APIError is the shape of error responses.
type APIError struct {
	Error string `json:"error"`
}
