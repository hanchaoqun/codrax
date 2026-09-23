package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestIOInFlightPublicAbsentFamilyAddsNoObservations(t *testing.T) {
	const body = "app-40 (40) [003] .... 1.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=40 next_prio=120\n" +
		"app-40 (40) [003] .... 1.007000: sched_wakeup: comm=app pid=40 prio=120 target_cpu=3\n"
	for _, view := range []string{"window_stats", "root_cause_rank"} {
		t.Run(view, func(t *testing.T) {
			result, native := ioInFlightPresencePublicQuery(t, body, view)
			if native.WindowStats == nil {
				t.Fatal("public query did not expose its actual window-statistics payload")
			}
			if native.WindowStats.EventCounts[tracequery.EventSchedSwitch] != 1 || native.WindowStats.EventCounts[tracequery.EventSchedWakeup] != 1 {
				t.Fatalf("non-IO fixture did not preserve its real scheduler events: %+v", native.WindowStats.EventCounts)
			}
			if native.WindowStats.IOInFlight != nil {
				t.Errorf("no IO observation or pairing diagnostic must not add an empty measurement face: %+v", native.WindowStats.IOInFlight)
			}
			if strings.Contains(result.Summary, "- io_inflight ") {
				t.Error("non-IO summary acquired an empty IO section")
			}
			for _, row := range result.Observations {
				if row.Predicate == "io_inflight" || row.Predicate == "io_inflight_coverage" {
					t.Errorf("non-IO query minted additional observation %s: %+v", row.Predicate, row)
				}
			}
		})
	}
}

func TestIOInFlightPublicRejectedOnlyEndpointsKeepDiagnostic(t *testing.T) {
	const body = "io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R -1 () 8 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.003000: block_rq_complete: 8,0 R () 8 + 8 [0]\n"
	result, native := ioInFlightPresencePublicQuery(t, body, "window_stats")
	if native.WindowStats == nil || native.WindowStats.IOInFlight == nil {
		t.Fatal("rejected endpoint diagnostics were dropped with the empty population")
	}
	stats := native.WindowStats.IOInFlight
	if len(stats.Groups) != 0 {
		t.Fatalf("malformed fixture unexpectedly produced an admitted group: %+v", stats.Groups)
	}
	found := false
	for _, c := range stats.Coverage {
		found = found || c.RejectedEndpointRows > 0 && c.Status != tracequery.IOInFlightCoverageAvailable
	}
	if !found {
		t.Fatalf("public native rejection prerequisite missing: %+v", stats.Coverage)
	}
	found = false
	for _, row := range result.Observations {
		found = found || row.Predicate == "io_inflight_coverage" && row.Subject == "block"
		if row.Predicate == "io_inflight" {
			t.Fatalf("rejected endpoints gained a measured occupancy row: %+v", row)
		}
	}
	if !found {
		t.Fatal("typed projection lost the real rejection diagnostic")
	}
}

func TestIOInFlightPublicCanonicalWindowPrecision(t *testing.T) {
	const body = "io-40 (40) [003] .... 1.000000100: block_rq_issue: 8,0 R 4096 () 8 + 8 [io]\n" +
		"irq-2 (2) [003] .... 1.000000400: block_rq_complete: 8,0 R () 8 + 8 [0]\n"
	for _, tc := range []struct {
		name, want string
		start, end float64
	}{
		{"six_decimal_minimum", "selected_window=1.000000..1.007000", 1, 1.007},
		{"finer_precision_retained", "selected_window=1.0000001..1.0000004", 1.0000001, 1.0000004},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, _, _, _ := ioInFlightPrecisionPublicQuery(t, body, tc.start, tc.end, 0, 0)
			seen := 0
			for _, row := range result.Observations {
				if row.Predicate != "io_inflight" && row.Predicate != "io_inflight_coverage" {
					continue
				}
				for _, note := range row.RichNotes {
					if strings.HasPrefix(note, "selected_window=") {
						seen++
						if note != tc.want {
							t.Errorf("canonical window note %q != %q", note, tc.want)
						}
					}
				}
			}
			if seen == 0 {
				t.Fatal("real occupancy payload omitted canonical query scope")
			}
		})
	}
}

func ioInFlightPresencePublicQuery(t *testing.T, body, view string) (types.ToolResult, tracequery.Result) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "presence.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("Describe the recorded observations")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": view, "time_start": 1, "time_end": 1.007})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("real trace query failed: %v %+v", err, result)
	}
	for _, row := range result.Observations {
		if row.SourceRef.PayloadRef == "" {
			continue
		}
		data, err := os.ReadFile(row.SourceRef.PayloadRef)
		var native tracequery.Result
		if err != nil || json.Unmarshal(data, &native) != nil {
			t.Fatalf("cannot read actual trace query JSON: %v", err)
		}
		unchanged, err := os.ReadFile(path)
		if err != nil || string(unchanged) != body {
			t.Fatal("query changed input bytes")
		}
		return result, native
	}
	t.Fatal("real query omitted its typed payload reference")
	return result, tracequery.Result{}
}
