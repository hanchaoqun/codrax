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

// TestTraceQuerySchemaExposesWindowSweep pins the §4.7 W3 registration: the
// view enum accepts window_sweep and the bucket_ms parameter is declared with
// its default/clamp documented (schema must match the strict decoder).
func TestTraceQuerySchemaExposesWindowSweep(t *testing.T) {
	var params struct {
		Properties struct {
			View struct {
				Enum        []string `json:"enum"`
				Description string   `json:"description"`
			} `json:"view"`
			BucketMs struct {
				Type        string `json:"type"`
				Description string `json:"description"`
			} `json:"bucket_ms"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &params); err != nil {
		t.Fatalf("unmarshal trace_query parameters: %v", err)
	}
	found := false
	for _, view := range params.Properties.View.Enum {
		if view == tracequery.ViewWindowSweep {
			found = true
		}
	}
	if !found {
		t.Fatalf("view enum missing %q: %v", tracequery.ViewWindowSweep, params.Properties.View.Enum)
	}
	if !strings.Contains(params.Properties.View.Description, "window_sweep") ||
		!strings.Contains(params.Properties.View.Description, "NOT subject to the index event budget") {
		t.Fatalf("view description must teach window_sweep: %q", params.Properties.View.Description)
	}
	if params.Properties.BucketMs.Type != "number" {
		t.Fatalf("bucket_ms schema type = %q, want number", params.Properties.BucketMs.Type)
	}
	for _, want := range []string{"Default 100", "50..500"} {
		if !strings.Contains(params.Properties.BucketMs.Description, want) {
			t.Fatalf("bucket_ms description missing %q: %q", want, params.Properties.BucketMs.Description)
		}
	}
}

// TestTraceQueryWindowSweepExecutesStreaming pins the tool dispatch: a
// view=window_sweep call runs the streaming sweep (never the index build),
// renders the hotspot + coverage sections, and stays a Success result.
func TestTraceQueryWindowSweepExecutesStreaming(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sweep_exec.systrace")
	trace := strings.Join([]string{
		`      app-20  (   20) [001] .... 10.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=other next_pid=30 next_prio=120`,
		`      app-20  (   20) [001] .... 10.040000: sched_switch: prev_comm=other prev_pid=30 prev_prio=120 prev_state=D ==> next_comm=app next_pid=20 next_prio=120`,
		`    waker-10  (   10) [000] .... 10.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`      app-20  (   20) [001] .... 10.320000: sched_switch: prev_comm=app prev_pid=20 prev_prio=120 prev_state=S ==> next_comm=other next_pid=30 next_prio=120`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "sweep_exec.systrace",
		"view":       "window_sweep",
		"pid":        20,
		"time_start": 10.0,
		"time_end":   10.5,
		"bucket_ms":  100,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("window_sweep execute failed: %s", res.Summary)
	}
	for _, want := range []string{
		"view=window_sweep",
		"## Window sweep",
		"rank_basis=" + tracequery.WindowSweepRankBasisTargetPID,
		"- hotspot rank=1",
		"target_pid_switches=2",
		"suggested_views=",
		"- coverage window=",
		"streamed_window_sweep=true",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("window_sweep summary missing %q:\n%s", want, res.Summary)
		}
	}
}

// TestTraceQueryIndexLimitRefinementSteersWindowSweepOnLongWindows pins the
// F2 channel-consistency rule: the TYPED refinement hint must give the same
// first move as the prose denial banner. Under the identical precise signal
// (explicit request window STRICTLY longer than 1s) PreferredParams steer to
// view=window_sweep over the SAME window, without the event_search fallback
// extras (event_types injection, state_cluster_first/parent_coverage, and
// narrowing suggestions — window_sweep means "do not narrow yet"). At or
// below 1s the historical event_search-shaped hint is unchanged.
func TestTraceQueryIndexLimitRefinementSteersWindowSweepOnLongWindows(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := func(end float64) traceQueryParams {
		return traceQueryParams{
			View:      "root_cause_rank",
			PID:       20,
			TimeStart: traceSecondFromAutoWindow(1.0),
			TimeEnd:   traceSecondFromAutoWindow(end),
		}
	}

	long := traceQueryIndexLimitRefinement(ctx, params(2.5), "path", filepath.Join(dir, "dense.ftrace"))
	if long == nil || long.ReasonCode != "trace_query_index_event_limit" {
		t.Fatalf("expected index-limit refinement, got %+v", long)
	}
	if got := long.PreferredParams["view"]; got != tracequery.ViewWindowSweep {
		t.Fatalf("long-window typed hint view = %q, want %q (must match the prose window_sweep_first steer)", got, tracequery.ViewWindowSweep)
	}
	if long.PreferredParams["time_start"] != "1.000000" || long.PreferredParams["time_end"] != "2.500000" {
		t.Fatalf("window_sweep steer must keep the SAME window: %+v", long.PreferredParams)
	}
	if got := long.PreferredParams["micro_window_policy"]; got != "sub_50ms_local_only" {
		t.Fatalf("micro window policy = %q, want sub_50ms_local_only", got)
	}
	for _, absent := range []string{"parent_coverage", "event_types", "pattern", "span_name"} {
		if val, ok := long.PreferredParams[absent]; ok {
			t.Fatalf("window_sweep branch must not attach %s (got %q): %+v", absent, val, long.PreferredParams)
		}
	}
	if got := strings.Join(long.RequiredFields, ","); got != "time_start,time_end" {
		t.Fatalf("window_sweep branch required fields = %q, want time_start,time_end", got)
	}
	if len(long.ParamNarrowingSuggestions) != 0 {
		t.Fatalf("window_sweep branch must not suggest narrowing (sweep the same window first): %+v", long.ParamNarrowingSuggestions)
	}

	// Exactly 1.0s: strictly-greater condition — historical event_search
	// fallback shape unchanged.
	short := traceQueryIndexLimitRefinement(ctx, params(2.0), "path", filepath.Join(dir, "dense.ftrace"))
	if short == nil || short.ReasonCode != "trace_query_index_event_limit" {
		t.Fatalf("expected short-window refinement, got %+v", short)
	}
	if got := short.PreferredParams["view"]; got != "event_search" {
		t.Fatalf("short-window typed hint view = %q, want event_search", got)
	}
	if got := short.PreferredParams["parent_coverage"]; got != tracequery.FallbackParentCoverageStateCluster {
		t.Fatalf("short-window parent coverage = %q, want %q", got, tracequery.FallbackParentCoverageStateCluster)
	}
	if !strings.Contains(strings.Join(short.RequiredFields, ","), "state_cluster_first") {
		t.Fatalf("short-window required fields must keep state_cluster_first: %+v", short.RequiredFields)
	}
	if len(short.ParamNarrowingSuggestions) == 0 {
		t.Fatalf("short-window branch must keep its narrowing suggestions: %+v", short)
	}
}

// TestTraceQueryIndexLimitSummaryWindowSweepFirst pins the §4.7 denial
// guidance at the tool layer: only an explicit request window STRICTLY longer
// than 1s puts the window_sweep-first line ahead of recovery_params.
func TestTraceQueryIndexLimitSummaryWindowSweepFirst(t *testing.T) {
	limitErr := &tracequery.IndexEventLimitError{
		Path:      "dense.ftrace",
		MaxEvents: 250000,
		Events:    250000,
		FirstTs:   1.0,
		LastTs:    1.2,
	}
	long := traceQueryIndexLimitSummary("dense.ftrace", "path", traceQueryParams{
		View:      "root_cause_rank",
		TimeStart: traceSecondFromAutoWindow(1.0),
		TimeEnd:   traceSecondFromAutoWindow(2.5),
	}, limitErr)
	for _, want := range []string{
		"window_sweep_first=requested window spans 1.500s",
		`trace_query(view="window_sweep") FIRST`,
		"NOT subject to this index event budget",
	} {
		if !strings.Contains(long, want) {
			t.Fatalf("long-window denial summary missing %q:\n%s", want, long)
		}
	}
	if strings.Index(long, "window_sweep_first=") > strings.Index(long, "recovery_params=") {
		t.Fatalf("window_sweep steer must come FIRST, before recovery_params:\n%s", long)
	}

	// Exactly 1.0s: strictly-greater condition — line absent.
	boundary := traceQueryIndexLimitSummary("dense.ftrace", "path", traceQueryParams{
		View:      "root_cause_rank",
		TimeStart: traceSecondFromAutoWindow(1.0),
		TimeEnd:   traceSecondFromAutoWindow(2.0),
	}, limitErr)
	if strings.Contains(boundary, "window_sweep_first=") {
		t.Fatalf("exactly-1s window must not emit the window_sweep steer:\n%s", boundary)
	}

	// No explicit window at all: line absent.
	unbounded := traceQueryIndexLimitSummary("dense.ftrace", "path", traceQueryParams{View: "root_cause_rank"}, limitErr)
	if strings.Contains(unbounded, "window_sweep_first=") {
		t.Fatalf("unbounded request must not emit the window_sweep steer:\n%s", unbounded)
	}
}
