// Package docker wraps the Docker Engine API for discovery, adoption, lifecycle
// control, updates and stats. It talks to the local daemon over the mounted
// socket; no docker CLI or compose engine is used.
package docker

import (
	"context"

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
	cli *client.Client
	st  *store.Store
	pub Publisher
}

// NewManager builds a Docker Manager from the ambient environment
// (DOCKER_HOST or /var/run/docker.sock).
func NewManager(st *store.Store, pub Publisher) (*Manager, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Manager{cli: cli, st: st, pub: pub}, nil
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
