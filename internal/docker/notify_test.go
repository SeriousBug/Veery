package docker

import (
	"path/filepath"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/SeriousBug/Veery/internal/store"
)

type fakeNotifier struct {
	titles []string
}

func (f *fakeNotifier) Notify(ev api.NotificationEvent, title, body string) {
	f.titles = append(f.titles, title)
}

// stacksOf builds a one-stack snapshot from container name/status pairs.
func stacksOf(containers ...api.Container) []api.Stack {
	return []api.Stack{{ID: "web", Name: "web", Containers: containers}}
}

func managed(name string, status api.ContainerStatus) api.Container {
	return api.Container{Name: name, ContainerName: name, Status: status, Managed: true, State: string(status)}
}

func newTestManager(t *testing.T, st *store.Store) (*Manager, *fakeNotifier) {
	t.Helper()
	f := &fakeNotifier{}
	return &Manager{st: st, notif: f, lastStatus: map[string]api.ContainerStatus{}}, f
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestNoteStatusesNotifiesOnlyOnTransitions(t *testing.T) {
	st := openTestStore(t)
	m, f := newTestManager(t, st)

	// First sweep is a baseline: a container Veery has never seen before is not
	// news, however it happens to be doing.
	m.noteStatuses(stacksOf(managed("nginx", api.StatusRunning), managed("db", api.StatusNeedsAttention)))
	if len(f.titles) != 0 {
		t.Fatalf("first sweep notified %v, want nothing", f.titles)
	}

	m.noteStatuses(stacksOf(managed("nginx", api.StatusNeedsAttention), managed("db", api.StatusNeedsAttention)))
	if len(f.titles) != 1 || f.titles[0] != "nginx needs attention" {
		t.Fatalf("titles = %v, want one notification for nginx", f.titles)
	}

	// Steady state: no repeat.
	m.noteStatuses(stacksOf(managed("nginx", api.StatusNeedsAttention), managed("db", api.StatusNeedsAttention)))
	if len(f.titles) != 1 {
		t.Fatalf("titles = %v, want no repeat of an unchanged status", f.titles)
	}

	m.noteStatuses(stacksOf(managed("nginx", api.StatusRunning), managed("db", api.StatusNeedsAttention)))
	if len(f.titles) != 2 || f.titles[1] != "nginx is running" {
		t.Fatalf("titles = %v, want a recovery notification", f.titles)
	}
}

func TestNoteStatusesIgnoresUnmanagedContainers(t *testing.T) {
	st := openTestStore(t)
	m, f := newTestManager(t, st)

	unmanaged := api.Container{Name: "ci-job", ContainerName: "ci-job", Status: api.StatusRunning}
	m.noteStatuses(stacksOf(unmanaged))
	unmanaged.Status = api.StatusNeedsAttention
	m.noteStatuses(stacksOf(unmanaged))

	if len(f.titles) != 0 {
		t.Fatalf("titles = %v, want nothing for a container Veery does not manage", f.titles)
	}
}

// A restart must not replay the state of the world as if it had just happened,
// and must still catch what changed while Veery was down.
func TestNoteStatusesSurvivesRestart(t *testing.T) {
	st := openTestStore(t)

	first, _ := newTestManager(t, st)
	first.noteStatuses(stacksOf(managed("nginx", api.StatusRunning)))
	first.noteStatuses(stacksOf(managed("nginx", api.StatusRunning)))

	restarted, f := newTestManager(t, st)
	restarted.noteStatuses(stacksOf(managed("nginx", api.StatusRunning)))
	if len(f.titles) != 0 {
		t.Fatalf("titles = %v, want nothing when nothing changed across the restart", f.titles)
	}

	restarted.noteStatuses(stacksOf(managed("nginx", api.StatusMissing)))
	if len(f.titles) != 1 || f.titles[0] != "nginx has gone missing" {
		t.Fatalf("titles = %v, want the missing container reported after the restart", f.titles)
	}
}

func TestNoteStatusesIgnoresInFlightUpdates(t *testing.T) {
	st := openTestStore(t)
	m, f := newTestManager(t, st)

	m.noteStatuses(stacksOf(managed("nginx", api.StatusRunning)))
	m.noteStatuses(stacksOf(managed("nginx", api.StatusUpdating)))
	m.noteStatuses(stacksOf(managed("nginx", api.StatusRunning)))

	if len(f.titles) != 0 {
		t.Fatalf("titles = %v, want nothing: the update reports its own outcome", f.titles)
	}
}
