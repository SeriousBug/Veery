package docker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/notify"
	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
)

// TestIntegrationNotifiesOnContainerStop drives the whole notification chain
// against the real Docker daemon: a managed container stops, the stack sweep
// notices, and a message reaches the configured webhook. Skipped when no daemon
// is reachable.
func TestIntegrationNotifiesOnContainerStop(t *testing.T) {
	ctx := context.Background()

	st, err := store.Open(filepath.Join(t.TempDir(), "veery.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	m, err := NewManager(st, nil)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer m.Close()
	if err := m.Ping(ctx); err != nil {
		t.Skipf("docker daemon unreachable, skipping: %v", err)
	}

	delivered := make(chan string, 4)
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		delivered <- string(body)
	}))
	defer webhook.Close()

	if err := st.SaveNotificationConfig(api.NotificationConfig{URLs: []string{"generic+" + webhook.URL}}); err != nil {
		t.Fatalf("save notification config: %v", err)
	}
	m.SetNotifier(notify.New(st))

	const project = "veerynotifytest"
	name := fmt.Sprintf("veerynotifytest-%d", time.Now().UnixNano())
	ensureImage(t, m, ctx, "busybox:latest")

	created, err := m.cli.ContainerCreate(ctx, &container.Config{
		Image:  "busybox:latest",
		Cmd:    []string{"sh", "-c", "sleep 3600"},
		Labels: map[string]string{projectLabel: project, serviceLabel: "sleeper"},
	}, &container.HostConfig{}, nil, nil, name)
	if err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() {
		_ = m.cli.ContainerRemove(context.Background(), created.ID, container.RemoveOptions{Force: true})
		_ = st.Unadopt(project)
	})
	if err := m.cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		t.Fatalf("start container: %v", err)
	}
	if err := m.Adopt(ctx, project); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	// Adoption itself is not news, and neither is a container that was already
	// running when Veery first looked.
	m.BroadcastStacks(ctx)
	logContainers(t, m, ctx, "baseline sweep")
	select {
	case msg := <-delivered:
		t.Fatalf("unexpected notification for a container that just sat there: %q", msg)
	case <-time.After(500 * time.Millisecond):
	}

	stopStart := time.Now()
	if err := m.Stop(ctx, created.ID); err != nil {
		t.Fatalf("stop container: %v", err)
	}
	t.Logf("stop returned after %s", time.Since(stopStart))
	if insp, err := m.cli.ContainerInspect(ctx, created.ID); err == nil {
		t.Logf("inspect after stop: status=%q running=%v exit=%d finished=%s",
			insp.State.Status, insp.State.Running, insp.State.ExitCode, insp.State.FinishedAt)
	} else {
		t.Logf("inspect after stop: %v", err)
	}
	m.BroadcastStacks(ctx)
	logContainers(t, m, ctx, "sweep after stop")

	select {
	case msg := <-delivered:
		if !strings.Contains(msg, name) || !strings.Contains(msg, "stopped") {
			t.Fatalf("notification = %q, want it to name %s and say it stopped", msg, name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification arrived after the container stopped")
	}
}

func logContainers(t *testing.T, m *Manager, ctx context.Context, when string) {
	t.Helper()
	summaries, err := m.cli.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Logf("%s: container list: %v", when, err)
		return
	}
	for _, c := range summaries {
		name := containerName(c.Names)
		if strings.HasPrefix(name, "veerynotifytest-") {
			t.Logf("%s: %s summary state=%q status=%q", when, name, c.State, c.Status)
		}
	}
	stacks, err := m.ListStacks(ctx)
	if err != nil {
		t.Logf("%s: list stacks: %v", when, err)
		return
	}
	for _, st := range stacks {
		for _, c := range st.Containers {
			t.Logf("%s: %s state=%q status=%q managed=%v", when, c.ContainerName, c.State, c.Status, c.Managed)
		}
	}
}
