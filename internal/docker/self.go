package docker

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"
)

// updaterLabel marks the throwaway helper container that performs a self-update.
// Containers carrying it are never adopted or shown, and are pruned on startup.
const updaterLabel = "veery.role"

const updaterRole = "updater"

// containerIDPattern matches the 64-hex container id docker embeds in the
// cgroup and mount paths of a container's own /proc entries.
var containerIDPattern = regexp.MustCompile(`\b[0-9a-f]{64}\b`)

// SelfContainerID returns the id of the container Veery itself runs in, or ""
// if it isn't containerized (a dev run on the host), which disables the
// self-update path. The id read out of /proc is confirmed against the daemon,
// so a stale or foreign id never passes for us.
func (m *Manager) SelfContainerID(ctx context.Context) string {
	m.selfOnce.Do(func() {
		m.selfID = m.resolveSelf(ctx)
	})
	return m.selfID
}

func (m *Manager) resolveSelf(ctx context.Context) string {
	// An explicit override wins: it is the escape hatch for runtimes whose
	// /proc layout we don't recognize.
	if id := os.Getenv("VEERY_CONTAINER"); id != "" {
		if insp, err := m.cli.ContainerInspect(ctx, id); err == nil {
			return insp.ID
		}
	}
	for _, id := range procContainerIDs() {
		if insp, err := m.cli.ContainerInspect(ctx, id); err == nil {
			return insp.ID
		}
	}
	// Fall back to the hostname, which docker sets to the short container id
	// unless it was overridden. Only trust it when it really is a prefix of the
	// id it resolves to, so a custom hostname colliding with some other
	// container's name can't make us mistake that container for ourselves.
	host, err := os.Hostname()
	if err != nil || host == "" {
		return ""
	}
	insp, err := m.cli.ContainerInspect(ctx, host)
	if err != nil {
		return ""
	}
	if !strings.HasPrefix(insp.ID, host) {
		return ""
	}
	return insp.ID
}

// procContainerIDs scrapes candidate container ids out of this process's cgroup
// and mountinfo, which name the container's own id under both cgroup v1 and v2.
func procContainerIDs() []string {
	var out []string
	for _, path := range []string{"/proc/self/mountinfo", "/proc/self/cgroup"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			for _, id := range containerIDPattern.FindAllString(sc.Text(), -1) {
				out = append(out, id)
			}
		}
		f.Close()
	}
	return out
}

// IsSelf reports whether a container id is Veery's own container.
func (m *Manager) IsSelf(ctx context.Context, id string) bool {
	self := m.SelfContainerID(ctx)
	return self != "" && self == id
}
