package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/SeriousBug/Veery/internal/store"
	"github.com/docker/docker/api/types/container"
)

// TestE2ESelfUpdate is the only test that proves the thing self-update actually
// claims: a real Veery, in a real container, built from the real (distroless)
// image, replacing itself with a newer build and coming back up. Everything else
// stubs out either the container or the binary.
//
// It builds images and pushes them to a throwaway local registry, so it is far
// too slow for the normal suite. Run it deliberately:
//
//	VEERY_E2E=1 go test ./internal/docker/ -run TestE2ESelfUpdate -timeout 20m -v
func TestE2ESelfUpdate(t *testing.T) {
	if os.Getenv("VEERY_E2E") != "1" {
		t.Skip("set VEERY_E2E=1 to run the self-update end-to-end test")
	}
	ctx := context.Background()

	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}

	const (
		registryName = "veery-e2e-registry"
		registryPort = "5555"
		veeryName    = "veery-e2e"
	)
	ref := "localhost:" + registryPort + "/veery-e2e:latest"

	m := e2eManager(t)
	startRegistry(t, m, ctx, registryName, registryPort)

	// A named volume, the way Veery actually ships (the Dockerfile declares
	// VOLUME /data). A bind mount would be easier to poke at from the test, but
	// SQLite's WAL does not survive Docker Desktop's file sharing on macOS, and
	// the database would fail to open before any of this got started.
	const dataVolume = "veery-e2e-data"
	run(t, "docker", "volume", "rm", "-f", dataVolume)
	run(t, "docker", "volume", "create", dataVolume)
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", "-f", dataVolume).Run() })

	t.Log("building v1 image")
	buildAndPush(t, repoRoot, ref, "v1")

	t.Cleanup(func() {
		bg := context.Background()
		_ = m.cli.ContainerRemove(bg, veeryName, container.RemoveOptions{Force: true})
		_ = m.cli.ContainerRemove(bg, veeryName+oldSuffix, container.RemoveOptions{Force: true})
		m.pruneUpdaters(bg)
	})

	// Create the container without starting it, so it can be adopted (which needs
	// something to inspect) before it ever runs.
	//
	// --user 0: the image runs as nonroot (uid 65532), which cannot open
	// /var/run/docker.sock. Veery has to be given access to the socket somehow or
	// it can manage nothing at all, and the helper container has to inherit
	// whatever that is.
	run(t, "docker", "create",
		"--name", veeryName,
		"--user", "0",
		"-v", "/var/run/docker.sock:/var/run/docker.sock",
		"-v", dataVolume+":/data",
		ref,
	)

	// Seed the database with the container adopted and auto-update armed. The
	// update has to be *initiated from inside* the container or it never takes the
	// self-update path at all, and the poller is the one trigger that needs no
	// passkey.
	seedDB(t, ctx, m, veeryName, t.TempDir())

	run(t, "docker", "start", veeryName)
	waitFor(t, 60*time.Second, "veery to start", func() bool {
		insp, err := m.cli.ContainerInspect(ctx, veeryName)
		return err == nil && insp.State != nil && insp.State.Running
	})
	v1Insp, err := m.cli.ContainerInspect(ctx, veeryName)
	if err != nil {
		t.Fatal(err)
	}
	v1ID := v1Insp.ID
	t.Logf("veery v1 running as %s", v1ID[:12])

	// A second build with a different label is a genuinely different image under
	// the same tag: exactly what a released update looks like.
	t.Log("building v2 image")
	buildAndPush(t, repoRoot, ref, "v2")
	v2Image := imageID(t, m, ctx, ref)
	if v2Image == "" {
		t.Fatal("v2 image has no id")
	}

	t.Log("waiting for veery to update itself (auto-update poll is 1 minute)")
	var final container.InspectResponse
	waitFor(t, 8*time.Minute, "veery to come back on the new image", func() bool {
		insp, err := m.cli.ContainerInspect(ctx, veeryName)
		if err != nil || insp.State == nil || !insp.State.Running {
			return false
		}
		if insp.ID == v1ID || insp.Image != v2Image {
			return false
		}
		final = insp
		return true
	})

	t.Logf("veery replaced itself: %s -> %s", v1ID[:12], final.ID[:12])

	// The new Veery is up, but the helper is still verifying it and only retires
	// the parked container once that passes. Which is the point: the old container
	// is kept until the new one has proven itself.
	waitFor(t, 90*time.Second, "the parked container to be retired", func() bool {
		_, err := m.cli.ContainerInspect(ctx, veeryName+oldSuffix)
		return err != nil
	})

	// The update job must have been closed out, not left in flight forever: a job
	// still marked active is a client stuck on a spinner.
	var last store.UpdateJob
	waitFor(t, 90*time.Second, "the update job to be recorded as finished", func() bool {
		st := readDB(t, veeryName, t.TempDir())
		active, err := st.ActiveUpdateJobs()
		if err != nil || len(active) > 0 {
			return false
		}
		recent, err := st.RecentUpdateJobs(0)
		if err != nil || len(recent) == 0 {
			return false
		}
		last = recent[len(recent)-1]
		return true
	})

	if last.Error != "" {
		t.Errorf("the update reported an error: %s", last.Error)
	}
	if !last.Self {
		t.Error("the job was not recorded as a self-update")
	}
	t.Logf("update job %s finished: phase=%s message=%q", last.ID, last.Phase, last.Message)
}

func e2eManager(t *testing.T) *Manager {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "host.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	m, err := NewManager(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if err := m.Ping(context.Background()); err != nil {
		t.Skipf("docker daemon unreachable: %v", err)
	}
	return m
}

// startRegistry runs a throwaway registry. The daemon, not the container, does
// the pulling, so "localhost" here is the host's localhost and needs no insecure
// registry configuration.
func startRegistry(t *testing.T, m *Manager, ctx context.Context, name, port string) {
	t.Helper()
	_ = m.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	ensureImage(t, m, ctx, "registry:2")

	run(t, "docker", "run", "-d", "--name", name, "-p", port+":5000", "registry:2")
	t.Cleanup(func() {
		_ = m.cli.ContainerRemove(context.Background(), name, container.RemoveOptions{Force: true})
	})
	waitFor(t, 30*time.Second, "the registry to accept connections", func() bool {
		return exec.Command("curl", "-sf", "http://localhost:"+port+"/v2/").Run() == nil
	})
}

func buildAndPush(t *testing.T, repoRoot, ref, marker string) {
	t.Helper()
	run(t, "docker", "build", "-t", ref, "--label", "veery.e2e="+marker, repoRoot)
	run(t, "docker", "push", ref)
}

// seedDB builds Veery's database on the host — where SQLite works — with its own
// container adopted and auto-update armed, then copies it into the data volume.
// The container must already exist (adoption snapshots it) but must not be
// running (nothing else may have the database open).
func seedDB(t *testing.T, ctx context.Context, m *Manager, name, tmp string) {
	t.Helper()
	dbPath := filepath.Join(tmp, "veery.db")

	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open seed db: %v", err)
	}
	hostMgr, err := NewManager(st, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := hostMgr.Adopt(ctx, name); err != nil {
		t.Fatalf("adopt veery: %v", err)
	}
	mc, err := st.ManagedByName(name)
	if err != nil {
		t.Fatalf("managed lookup: %v", err)
	}
	if err := st.SetAutoUpdate(mc.ID, true); err != nil {
		t.Fatalf("enable auto-update: %v", err)
	}
	cfg, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AutoUpdateIntervalMinutes = 1
	if err := st.SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	hostMgr.Close()
	// Close before copying: WAL contents have to be folded back into the file.
	if err := st.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	run(t, "docker", "cp", dbPath, name+":/data/veery.db")
}

// readDB copies Veery's database back out of the volume so the test can inspect
// what the update recorded. The whole directory goes, not just the .db file:
// recent writes are still sitting in the WAL alongside it.
func readDB(t *testing.T, name, tmp string) *store.Store {
	t.Helper()
	dir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, "docker", "cp", name+":/data/.", dir)
	st, err := store.Open(filepath.Join(dir, "veery.db"))
	if err != nil {
		t.Fatalf("open copied db: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func imageID(t *testing.T, m *Manager, ctx context.Context, ref string) string {
	t.Helper()
	insp, err := m.cli.ImageInspect(ctx, ref)
	if err != nil {
		t.Fatalf("inspect image %s: %v", ref, err)
	}
	return insp.ID
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

func waitFor(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out after %s waiting for %s", limit, what)
}
