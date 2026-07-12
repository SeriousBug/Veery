package docker

import (
	"context"
	"encoding/json"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/docker/docker/api/types/container"
)

// ContainerStats returns a one-shot resource snapshot for every running
// container. CPU percent and memory are computed the same way the docker CLI
// does for `docker stats --no-stream`.
func (m *Manager) ContainerStats(ctx context.Context) ([]api.ContainerMetrics, error) {
	summaries, err := m.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]api.ContainerMetrics, 0, len(summaries))
	for _, c := range summaries {
		if c.State != "running" {
			continue
		}
		cm, err := m.containerStat(ctx, c.ID)
		if err != nil {
			continue
		}
		out = append(out, cm)
	}
	return out, nil
}

func (m *Manager) containerStat(ctx context.Context, id string) (api.ContainerMetrics, error) {
	// stream=false makes the daemon include precpu_stats from ~1s earlier so a
	// meaningful CPU percentage can be computed from a single response.
	resp, err := m.cli.ContainerStats(ctx, id, false)
	if err != nil {
		return api.ContainerMetrics{}, err
	}
	defer resp.Body.Close()
	var s container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return api.ContainerMetrics{}, err
	}
	return api.ContainerMetrics{
		ID:         id,
		CPUPercent: cpuPercent(s),
		MemUsed:    memUsed(s.MemoryStats),
		MemLimit:   s.MemoryStats.Limit,
	}, nil
}

func cpuPercent(s container.StatsResponse) float64 {
	cpuDelta := float64(s.CPUStats.CPUUsage.TotalUsage) - float64(s.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(s.CPUStats.SystemUsage) - float64(s.PreCPUStats.SystemUsage)
	if sysDelta <= 0 || cpuDelta < 0 {
		return 0
	}
	cpus := float64(s.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(s.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpus == 0 {
		cpus = 1
	}
	return (cpuDelta / sysDelta) * cpus * 100.0
}

// memUsed subtracts cache like the docker CLI does when the stat is available.
func memUsed(mem container.MemoryStats) uint64 {
	used := mem.Usage
	if cache, ok := mem.Stats["inactive_file"]; ok && cache <= used {
		used -= cache
	}
	return used
}
