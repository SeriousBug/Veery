// Package docker wraps the Docker Engine API for discovery, adoption, lifecycle
// control, updates and stats. It talks to the local daemon over the mounted
// socket; no docker CLI or compose engine is used.
package docker

import (
	"context"
	"strings"
	"sync"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/client"
)

// Publisher receives messages to fan out to WS clients.
type Publisher interface {
	Broadcast(api.WSMessage)
}

// Manager is the Docker service used by the server.
type Manager struct {
	cli   *client.Client
	st    *store.Store
	pub   Publisher
	notif Notifier

	// availMu guards updateAvail, the in-memory "update available" flag per
	// container name, refreshed by the update-check poller. availBaseline says
	// whether the flags recorded by the previous run have been loaded.
	availMu       sync.Mutex
	updateAvail   map[string]bool
	availBaseline bool

	// statusMu guards the last-seen status of every managed container, which
	// the stack sweep diffs against to notify on transitions. statusBaseline
	// says whether a first sweep has landed.
	statusMu       sync.Mutex
	lastStatus     map[string]api.ContainerStatus
	statusBaseline bool

	// locksMu guards locks, a per-container-name mutex map so updates,
	// auto-updates and lifecycle actions never act on the same container
	// concurrently.
	locksMu sync.Mutex
	locks   map[string]*sync.Mutex

	// jobsMu guards activeJobs, the jobs currently in flight. A client that
	// connects while one is running is replayed them, so a page load in the
	// middle of an update still shows its progress.
	jobsMu     sync.Mutex
	activeJobs map[string]api.JobProgress

	// selfID is the container Veery itself runs in, resolved on first use. It is
	// empty when Veery runs on the host rather than in a container.
	selfOnce sync.Once
	selfID   string
}

// NewManager builds a Docker Manager from the ambient environment
// (DOCKER_HOST or /var/run/docker.sock).
func NewManager(st *store.Store, pub Publisher) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Manager{
		cli:         cli,
		st:          st,
		pub:         pub,
		updateAvail: map[string]bool{},
		lastStatus:  map[string]api.ContainerStatus{},
		locks:       map[string]*sync.Mutex{},
		activeJobs:  map[string]api.JobProgress{},
	}, nil
}

// containerLock returns the mutex for a container name, creating it on first use.
func (m *Manager) containerLock(name string) *sync.Mutex {
	m.locksMu.Lock()
	defer m.locksMu.Unlock()
	l, ok := m.locks[name]
	if !ok {
		l = &sync.Mutex{}
		m.locks[name] = l
	}
	return l
}

// withContainerLock resolves ref to a container name and runs fn under that
// container's lock. If the ref can't be inspected, ref itself is used as the key.
func (m *Manager) withContainerLock(ctx context.Context, ref string, fn func() error) error {
	name := ref
	if insp, err := m.cli.ContainerInspect(ctx, ref); err == nil {
		name = strings.TrimPrefix(insp.Name, "/")
	}
	l := m.containerLock(name)
	l.Lock()
	defer l.Unlock()
	return fn()
}

// setUpdateAvailable records whether a container has a newer image available.
func (m *Manager) setUpdateAvailable(name string, avail bool) {
	m.availMu.Lock()
	m.updateAvail[name] = avail
	m.availMu.Unlock()
}

// updateAvailableFor reports the last-known update-available flag for a container.
func (m *Manager) updateAvailableFor(name string) bool {
	m.availMu.Lock()
	defer m.availMu.Unlock()
	return m.updateAvail[name]
}

// Ping checks connectivity to the daemon.
func (m *Manager) Ping(ctx context.Context) error {
	_, err := m.cli.Ping(ctx)
	return err
}

// Close releases the client.
func (m *Manager) Close() error { return m.cli.Close() }

// Client exposes the underlying Docker client for the stats collector.
func (m *Manager) Client() *client.Client { return m.cli }
