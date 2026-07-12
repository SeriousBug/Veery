package docker

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

// Snapshot is the full create-spec captured from `docker inspect`, enough to
// recreate a container identically (possibly with a new image on update).
type Snapshot struct {
	Name             string                     `json:"name"`
	Image            string                     `json:"image"`
	Project          string                     `json:"project"`
	Service          string                     `json:"service"`
	Config           *container.Config          `json:"config"`
	HostConfig       *container.HostConfig      `json:"hostConfig"`
	NetworkingConfig *network.NetworkingConfig  `json:"networkingConfig"`
}

// snapshotFromInspect builds a Snapshot from an inspect response.
func snapshotFromInspect(insp container.InspectResponse) Snapshot {
	netCfg := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	if insp.NetworkSettings != nil {
		for name, ep := range insp.NetworkSettings.Networks {
			netCfg.EndpointsConfig[name] = ep
		}
	}
	var labels map[string]string
	if insp.Config != nil {
		labels = insp.Config.Labels
	}
	return Snapshot{
		Name:             strings.TrimPrefix(insp.Name, "/"),
		Image:            insp.Config.Image,
		Project:          labels["com.docker.compose.project"],
		Service:          labels["com.docker.compose.service"],
		Config:           insp.Config,
		HostConfig:       insp.HostConfig,
		NetworkingConfig: netCfg,
	}
}

func (s Snapshot) marshal() (string, error) {
	b, err := json.Marshal(s)
	return string(b), err
}

func parseSnapshot(js string) (Snapshot, error) {
	var s Snapshot
	err := json.Unmarshal([]byte(js), &s)
	return s, err
}

// recreate creates and starts a container from a snapshot. If newImage is
// non-empty it overrides the snapshot's image (used by updates). Returns the
// new container id.
func (m *Manager) recreate(ctx context.Context, snap Snapshot, newImage string) (string, error) {
	cfg := *snap.Config
	if newImage != "" {
		cfg.Image = newImage
	}

	// ContainerCreate accepts a single primary network in NetworkingConfig on
	// older API versions; connect any extra networks after creation.
	primary := &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{}}
	var extra []struct {
		name string
		ep   *network.EndpointSettings
	}
	first := true
	for name, ep := range snap.NetworkingConfig.EndpointsConfig {
		if first {
			primary.EndpointsConfig[name] = ep
			first = false
			continue
		}
		extra = append(extra, struct {
			name string
			ep   *network.EndpointSettings
		}{name, ep})
	}

	resp, err := m.cli.ContainerCreate(ctx, &cfg, snap.HostConfig, primary, nil, snap.Name)
	if err != nil {
		return "", err
	}
	for _, e := range extra {
		_ = m.cli.NetworkConnect(ctx, e.name, resp.ID, e.ep)
	}
	if err := m.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return resp.ID, err
	}
	return resp.ID, nil
}
