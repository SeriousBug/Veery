package docker

import (
	"testing"

	"github.com/SeriousBug/Veery/internal/api"
	"github.com/docker/docker/api/types/container"
)

func stackOf(statuses ...api.ContainerStatus) *api.Stack {
	st := &api.Stack{ID: "blog", Name: "blog"}
	for i, s := range statuses {
		st.Containers = append(st.Containers, api.Container{
			Name:    string(rune('a' + i)),
			Status:  s,
			Managed: true,
		})
	}
	return st
}

func TestFinalizeStackStatus(t *testing.T) {
	cases := []struct {
		name       string
		containers []api.ContainerStatus
		want       api.ContainerStatus
	}{{
		name:       "all running",
		containers: []api.ContainerStatus{api.StatusRunning, api.StatusRunning},
		want:       api.StatusRunning,
	}, {
		name:       "one stopped",
		containers: []api.ContainerStatus{api.StatusRunning, api.StatusStopped},
		want:       api.StatusStopped,
	}, {
		// The whole service was taken down (a compose down removes containers).
		// The user did that on purpose, so it is not a problem to flag.
		name:       "every container removed",
		containers: []api.ContainerStatus{api.StatusMissing, api.StatusMissing},
		want:       api.StatusMissing,
	}, {
		// One part removed while the rest of the service runs. Nothing the user
		// did to the whole service explains that, so it wants a look.
		name:       "one container removed",
		containers: []api.ContainerStatus{api.StatusRunning, api.StatusMissing},
		want:       api.StatusNeedsAttention,
	}, {
		name:       "crashed container",
		containers: []api.ContainerStatus{api.StatusRunning, api.StatusNeedsAttention},
		want:       api.StatusNeedsAttention,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := stackOf(tc.containers...)
			finalizeStack(st)
			if st.Status != tc.want {
				t.Fatalf("got %q, want %q", st.Status, tc.want)
			}
		})
	}
}

// settled decides whether a container's spec has earned the right to replace
// the one Veery holds. Anything that has not shown it can run must not, because
// that spec is what bring-up and update rollback build from.
func TestSettled(t *testing.T) {
	state := func(s *container.State) container.InspectResponse {
		return container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{State: s}}
	}
	health := func(status string) *container.Health { return &container.Health{Status: status} }

	cases := []struct {
		name string
		insp container.InspectResponse
		want bool
	}{
		{"running, no healthcheck", state(&container.State{Running: true}), true},
		{"running and healthy", state(&container.State{Running: true, Health: health(container.Healthy)}), true},
		{"still starting", state(&container.State{Running: true, Health: health(container.Starting)}), false},
		{"unhealthy", state(&container.State{Running: true, Health: health(container.Unhealthy)}), false},
		{"crash-looping", state(&container.State{Restarting: true}), false},
		{"dead", state(&container.State{Dead: true}), false},
		{"stopped by hand", state(&container.State{Status: "exited", ExitCode: 0}), true},
		{"crashed", state(&container.State{Status: "exited", ExitCode: 1}), false},
		{"created but never started", state(&container.State{Status: "created"}), true},
		{"no state at all", state(nil), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := settled(tc.insp); got != tc.want {
				t.Fatalf("settled = %v, want %v", got, tc.want)
			}
		})
	}
}
