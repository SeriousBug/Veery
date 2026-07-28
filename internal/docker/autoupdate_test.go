package docker

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
)

// failingContainer sets up a managed container with auto-update on, and returns
// the manager, its notifier and the stored record.
func failingContainer(t *testing.T) (*Manager, *fakeNotifier, store.ManagedContainer) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "veery.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.UpsertStack("web"); err != nil {
		t.Fatalf("upsert stack: %v", err)
	}
	mc := store.ManagedContainer{
		ID: "m1", StackID: "web", ContainerName: "web-1",
		SnapshotJSON: `{"image":"nginx:latest"}`, AutoUpdate: true,
	}
	if err := st.AddManagedContainer(mc); err != nil {
		t.Fatalf("add managed: %v", err)
	}
	m, notif := newTestManager(t, st)
	return m, notif, mc
}

// fail runs n failed auto-update attempts at one version, as the poller would:
// skipping the ones the manager has already given up on.
func fail(t *testing.T, m *Manager, mc store.ManagedContainer, target string, n int) int {
	t.Helper()
	attempts := 0
	for range n {
		if m.writtenOff(mc.ContainerName, target) {
			continue
		}
		attempts++
		m.noteUpdateFailure(mc, api.SourceAutomation, target, errors.New("new container exited (exit code 1)"))
	}
	return attempts
}

func TestAutoUpdateGivesUpOnAVersionAfterMaxAttempts(t *testing.T) {
	m, notif, mc := failingContainer(t)

	tried := fail(t, m, mc, "sha256:aaa", maxVersionAttempts+5)
	if tried != maxVersionAttempts {
		t.Errorf("tried %d times, want %d", tried, maxVersionAttempts)
	}
	if !m.writtenOff(mc.ContainerName, "sha256:aaa") {
		t.Error("version should be written off after the attempts ran out")
	}
	// Exactly one alert, on the attempt that crossed the threshold.
	if len(notif.events) != 1 || notif.events[0] != api.EventAutoUpdateStopped {
		t.Fatalf("events = %v, want one %s", notif.events, api.EventAutoUpdateStopped)
	}
	if !strings.Contains(notif.titles[0], mc.ContainerName) {
		t.Errorf("title %q does not name the container", notif.titles[0])
	}

	// A newly published version is a clean slate, not something already given up
	// on.
	if m.writtenOff(mc.ContainerName, "sha256:bbb") {
		t.Error("a version that has never failed must not be written off")
	}
}

func TestAutoUpdateTurnsItselfOffAfterEnoughFailedVersions(t *testing.T) {
	m, notif, mc := failingContainer(t)

	for _, target := range []string{"sha256:aaa", "sha256:bbb", "sha256:ccc"} {
		fail(t, m, mc, target, maxVersionAttempts)
	}

	got, err := m.st.ManagedByID(mc.ID)
	if err != nil {
		t.Fatalf("load managed: %v", err)
	}
	if got.AutoUpdate {
		t.Error("auto-update should be off after every version failed")
	}
	// Off because Veery gave up, not because the user chose to: the UI shows the
	// two differently.
	if got.AutoUpdateSource != api.SourceAutomation {
		t.Errorf("auto-update source = %q, want %q", got.AutoUpdateSource, api.SourceAutomation)
	}
	// One alert per written-off version, and the last one is the switch-off.
	if len(notif.events) != maxFailedVersions {
		t.Fatalf("got %d alerts, want %d", len(notif.events), maxFailedVersions)
	}
	last := notif.titles[len(notif.titles)-1]
	if !strings.Contains(last, "turned off") {
		t.Errorf("last alert %q does not say auto-update was turned off", last)
	}
	for _, ev := range notif.events {
		if ev != api.EventAutoUpdateStopped {
			t.Errorf("alert event = %s, want %s", ev, api.EventAutoUpdateStopped)
		}
	}
	if notif.metas[0].ContainerName != mc.ContainerName || notif.metas[0].StackID != mc.StackID {
		t.Errorf("alert meta = %+v, want the container and its stack", notif.metas[0])
	}
}

func TestAutoUpdateCountsOnlyConsecutiveFailedVersions(t *testing.T) {
	m, notif, mc := failingContainer(t)

	fail(t, m, mc, "sha256:aaa", maxVersionAttempts)
	fail(t, m, mc, "sha256:bbb", maxVersionAttempts)
	// A version that installs clears the history: whatever was wrong before, the
	// container is not stuck any more.
	m.clearUpdateFailures(mc.ContainerName)
	fail(t, m, mc, "sha256:ccc", maxVersionAttempts)

	got, err := m.st.ManagedByID(mc.ID)
	if err != nil {
		t.Fatalf("load managed: %v", err)
	}
	if !got.AutoUpdate {
		t.Error("auto-update should stay on: only one version failed since the last success")
	}
	if len(notif.events) != 3 {
		t.Errorf("got %d alerts, want one per written-off version", len(notif.events))
	}
	// The written-off versions from before the success are forgotten too, so the
	// same version would be attempted again.
	if m.writtenOff(mc.ContainerName, "sha256:aaa") {
		t.Error("a cleared version must not stay written off")
	}
}

// An update a person asked for is one they are watching the outcome of. It is
// never counted, so it can neither write off a version nor switch auto-update
// off.
func TestUserUpdateFailuresAreNotCounted(t *testing.T) {
	m, notif, mc := failingContainer(t)

	for range maxVersionAttempts * maxFailedVersions {
		m.noteUpdateFailure(mc, api.SourceUser, "sha256:aaa", errors.New("boom"))
	}
	if m.writtenOff(mc.ContainerName, "sha256:aaa") {
		t.Error("a user's failures must not write off a version")
	}
	if len(notif.events) != 0 {
		t.Errorf("got %d alerts from user-asked updates, want 0", len(notif.events))
	}
	got, _ := m.st.ManagedByID(mc.ID)
	if !got.AutoUpdate {
		t.Error("a user's failures must not turn auto-update off")
	}
}

// When the registry cannot be reached there is no version to blame, and the
// update is about to fail for a reason that is not the release's fault.
func TestFailuresWithNoKnownVersionAreNotCounted(t *testing.T) {
	m, notif, mc := failingContainer(t)

	for range maxVersionAttempts * maxFailedVersions {
		m.noteUpdateFailure(mc, api.SourceAutomation, "", errors.New("pull nginx:latest: no such host"))
	}
	if len(notif.events) != 0 {
		t.Errorf("got %d alerts, want 0", len(notif.events))
	}
	got, _ := m.st.ManagedByID(mc.ID)
	if !got.AutoUpdate {
		t.Error("auto-update must stay on when the failures cannot be tied to a version")
	}
}
