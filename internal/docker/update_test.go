package docker

import (
	"io"
	"strings"
	"testing"
)

// collectPull runs drainPull over a canned pull stream, ignoring the throttle by
// reading every emitted message.
func collectPull(t *testing.T, stream string) ([]string, error) {
	t.Helper()
	var msgs []string
	err := drainPull(io.NopCloser(strings.NewReader(stream)), func(phase, msg string) {
		if phase != "pulling" {
			t.Fatalf("unexpected phase %q", phase)
		}
		msgs = append(msgs, msg)
	})
	return msgs, err
}

func TestDrainPullReportsAggregateBytes(t *testing.T) {
	// Two layers downloading concurrently; bytes must be summed across both.
	stream := `
{"status":"Pulling from library/nginx"}
{"id":"a1","status":"Downloading","progressDetail":{"current":0,"total":100000000}}
{"id":"b2","status":"Downloading","progressDetail":{"current":0,"total":50000000}}
{"id":"a1","status":"Downloading","progressDetail":{"current":50000000,"total":100000000}}
`
	msgs, err := collectPull(t, stream)
	if err != nil {
		t.Fatalf("drainPull: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected progress messages, got none")
	}
	last := msgs[len(msgs)-1]
	if want := "Downloading 50.0 MB / 150.0 MB"; last != want {
		t.Errorf("last message = %q, want %q", last, want)
	}
}

func TestDrainPullReportsExtractingOnceDownloaded(t *testing.T) {
	stream := `
{"id":"a1","status":"Downloading","progressDetail":{"current":10,"total":100}}
{"id":"a1","status":"Download complete","progressDetail":{"current":100,"total":100}}
{"id":"b2","status":"Already exists"}
{"id":"a1","status":"Extracting","progressDetail":{"current":30,"total":100}}
`
	msgs, err := collectPull(t, stream)
	if err != nil {
		t.Fatalf("drainPull: %v", err)
	}
	last := msgs[len(msgs)-1]
	// a1 is still extracting, b2 already counts as complete.
	if want := "Extracting 1 / 2 layers"; last != want {
		t.Errorf("last message = %q, want %q", last, want)
	}
}

func TestDrainPullSurfacesStreamError(t *testing.T) {
	stream := `{"error":"manifest unknown"}`
	if _, err := collectPull(t, stream); err == nil {
		t.Fatal("expected an error, got nil")
	} else if !strings.Contains(err.Error(), "manifest unknown") {
		t.Errorf("error = %v, want it to mention the pull failure", err)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		512:        "512 B",
		1500:       "1.5 kB",
		150000000:  "150.0 MB",
		2500000000: "2.5 GB",
	}
	for in, want := range cases {
		if got := formatBytes(in); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
