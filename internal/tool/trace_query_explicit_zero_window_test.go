package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// An omitted zero in the short call banner must not be mistaken for a lost
// query boundary. Exercise the public decoder and native observation producer,
// not just an engine Query constructed with TimeStartSet by a test.
func TestTraceQueryExplicitZeroWindowReachesNativeObservations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero-window.systrace")
	trace := " app-10 (10) [000] .... 1.100000: tracing_mark_write: B|10|work\n" +
		" app-10 (10) [000] .... 1.200000: tracing_mark_write: E|10\n"
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"event_search", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{"path": path, "view": view, "time_start": 0, "time_end": 14})
			result, err := (&TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, params)
			if err != nil || !result.Success {
				t.Fatalf("public query failed: %+v, %v", result, err)
			}
			found := false
			for _, observation := range result.Observations {
				if view == "event_search" && observation.EventSearchInventory != nil {
					query := observation.EventSearchInventory.Query
					if !query.TimeStartSet || query.TimeStart != 0 || !query.TimeEndSet || query.TimeEnd != 14 {
						t.Fatalf("public event search lost the explicit zero window: %+v", query)
					}
					found = true
				}
				if view == "window_stats" {
					for _, note := range observation.RichNotes {
						if note == "selected_window=0.000000..14.000000" {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatalf("no native %s observation retained the explicit zero window; summary=%s", view, strings.TrimSpace(result.Summary))
			}
		})
	}
}
