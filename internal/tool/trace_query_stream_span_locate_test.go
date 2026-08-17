package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryLargeExplicitSpanWindowStreamsRemoteEnd(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	t.Cleanup(func() { traceQueryWindowedIndexMinBytes = oldThreshold })

	dir := t.TempDir()
	path := filepath.Join(dir, "long-span.systrace")
	trace := strings.Join([]string{
		`app-42 (42) [001] .... 1.000000: print: B|42|Choreographer#doFrame 8002384`,
		`worker-7 (7) [002] .... 2.000000: sched_switch: prev_comm=worker prev_pid=7 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120`,
		`app-42 (42) [001] .... 4.250000: print: E`,
	}, "\n")
	if err := os.WriteFile(path, []byte(trace+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	params, err := json.Marshal(map[string]any{
		"source": "path", "path": path, "view": "span_window",
		"span_name": "Choreographer#doFrame 8002384", "pid": 42,
		"time_start": 0.99, "time_end": 1.01, "limit": 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("stream span locate failed: %s", result.Summary)
	}
	for _, want := range []string{
		"streamed_span_locate=true", "Choreographer#doFrame 8002384", "duration=3250.000ms", "selected_window=0.990000..4.250000",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("stream span result missing %q:\n%s", want, result.Summary)
		}
	}
	if strings.Contains(result.Summary, "trace_span_begin_unpaired=true") {
		t.Fatalf("remote exact E was still presented as unpaired:\n%s", result.Summary)
	}
}

func TestTraceQuerySmallExplicitSpanWindowKeepsIndexedLane(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1 << 30
	t.Cleanup(func() { traceQueryWindowedIndexMinBytes = oldThreshold })
	var p traceQueryParams
	if err := json.Unmarshal([]byte(`{"view":"span_window","span_name":"work","pid":42,"time_start":1,"time_end":2}`), &p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "small.systrace")
	if err := os.WriteFile(path, []byte("app-42 (42) [001] .... 1.000000: print: B|42|work\napp-42 (42) [001] .... 1.100000: print: E\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if traceQueryShouldStreamSpanLocate(p, path) {
		t.Fatal("small traces should keep the existing full-index span lane")
	}
}
