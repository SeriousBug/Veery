package docker

import (
	"errors"
	"strings"
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
)

// A container whose update is in flight must read as "updating", not as trouble:
// during a swap it is deliberately parked, stopped and recreated, and the raw
// Docker state for that window would otherwise flag a false "needs attention".
func TestUpdatingContainersTracksUpdateJobsOnly(t *testing.T) {
	m := &Manager{activeJobs: map[string]api.JobProgress{}}

	m.activeJobs["j1"] = api.JobProgress{ID: "j1", Kind: "update", Target: "nginx"}
	m.activeJobs["j2"] = api.JobProgress{ID: "j2", Kind: "restart", Target: "db"}

	names := m.updatingContainers()
	if !names["nginx"] {
		t.Errorf("nginx has an update in flight, want it marked updating")
	}
	if names["db"] {
		t.Errorf("db is only restarting, not updating; want it left alone")
	}
}

// rolledBack must tell "failed but the service is back" apart from "failed and
// the service is now down", because a rollback that could not restart the old
// container is the more urgent, and previously silent, outcome.
func TestRolledBackDistinguishesRestoredFromDown(t *testing.T) {
	cause := errors.New("recreate on new image: boom")

	clean := rolledBack(nil, "nginx", cause)
	if !strings.Contains(clean.Error(), "rolled back") {
		t.Errorf("clean rollback = %q, want it to say it rolled back", clean)
	}

	down := rolledBack(errors.New("restart nginx: no such shim"), "nginx", cause)
	if !strings.Contains(down.Error(), "is down") {
		t.Errorf("failed rollback = %q, want it to say the service is down", down)
	}
	if !errors.Is(down, cause) {
		t.Errorf("failed rollback should still wrap the original cause")
	}
}
