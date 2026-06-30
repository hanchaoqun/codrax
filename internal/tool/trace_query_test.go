package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryExplicitPathProducesRuntimeArtifactSummary(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "sample.systrace",
		"view":       "wakeup_chain",
		"pid":        20,
		"time_start": 1.0,
		"time_end":   1.1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"origin=runtime_artifact", "artifact_kind=trace", "timestamp_unit=seconds", "priority_semantics=", "Wakeup chain", "waker-10"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if res.RawRef == "" {
		t.Fatalf("expected payload raw ref")
	}
}

func TestTraceQueryIndexLimitResultIsRecoverableScopeHint(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	p := traceQueryParams{
		View:      "root_cause_rank",
		Thread:    "android.haitong",
		TimeStart: traceSecondFromAutoWindow(1.0),
		TimeEnd:   traceSecondFromAutoWindow(2.0),
	}
	res, ok := (&TraceQuery{}).traceQueryIndexLimitResult(ctx, p, filepath.Join(dir, "dense.ftrace"), "path", &tracequery.IndexEventLimitError{
		Path:           filepath.Join(dir, "dense.ftrace"),
		MaxEvents:      3,
		Events:         3,
		Line:           42,
		ScannedLines:   42,
		Windowed:       true,
		IndexTimeStart: 1.0,
		IndexTimeEnd:   2.0,
	})
	if !ok {
		t.Fatalf("expected index limit result")
	}
	if !res.Success {
		t.Fatalf("event limit should be recoverable, got failure: %s", res.Summary)
	}
	for _, want := range []string{"mode=index_event_limit", "not evidence that the trace/ftrace format is unsupported", "do not retry the same heavy view"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("limit result missing %q:\nsummary=%s", want, res.Summary)
		}
	}
	if res.Refinement == nil || res.Refinement.ReasonCode != "trace_query_index_event_limit" {
		t.Fatalf("expected structured refinement, got %+v", res.Refinement)
	}
}

func TestTraceQueryLargeEventSearchWithWindowStreams(t *testing.T) {
	oldMin := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldMin }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "large_window.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.000000: print: B|20|BeforeWindow`,
		`app-20 (20) [001] .... 2.000000: print: B|20|InsideWindow`,
		`app-20 (20) [001] .... 2.010000: print: E|20`,
		`app-20 (20) [001] .... 3.000000: print: B|20|AfterWindow`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "large_window.systrace",
		"view":       "event_search",
		"time_start": 2.0,
		"time_end":   2.5,
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "streamed_event_search=true") {
		t.Fatalf("large windowed event_search should stream instead of building an index:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "BeforeWindow") || strings.Contains(res.Summary, "AfterWindow") {
		t.Fatalf("streamed window search leaked out-of-window rows:\n%s", res.Summary)
	}
}

func TestTraceQueryFtracePathParsesCompoundTimestampWindow(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "OHTrace_20260626_16.32.34.ftrace")
	trace := strings.Join([]string{
		`android.haitong-56023 (56023) [004] .... 1.501565915: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=android.haitong next_pid=56023 next_prio=52`,
		`Thread-10-56284 (56284) [005] .... 2.000000000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=Thread-10 next_pid=56284 next_prio=20`,
		`android.haitong-56023 (56023) [004] .... 3.116000000: sched_switch: prev_comm=android.haitong prev_pid=56023 prev_prio=52 prev_state=S ==> next_comm=idle/4 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "OHTrace_20260626_16.32.34.ftrace",
		"view":       "event_search",
		"pattern":    "android.haitong",
		"time_start": "1s 501ms 565μs 915ns",
		"time_end":   "3s 116ms",
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should accept .ftrace with compound timestamps: %s", res.Summary)
	}
	for _, want := range []string{
		"OHTrace_20260626_16.32.34.ftrace",
		"time_start=1.501566",
		"time_end=3.116000",
		"parsed_events=3",
		"matched_events=2",
		"android.haitong-56023",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf(".ftrace compound timestamp summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "unsupported") || strings.Contains(res.Summary, "future parser adapter") {
		t.Fatalf(".ftrace parse should not be narrated as unsupported:\n%s", res.Summary)
	}
}

func TestTraceQueryWindowedZeroEventsDoesNotImplyFtraceUnsupported(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "window_miss.ftrace")
	trace := `android.haitong-56023 (56023) [004] .... 10.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=android.haitong next_pid=56023 next_prio=52`
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	oldMinBytes := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	t.Cleanup(func() { traceQueryWindowedIndexMinBytes = oldMinBytes })

	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "window_miss.ftrace",
		"view":       "window_stats",
		"time_start": 1.0,
		"time_end":   2.0,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("empty bounded window should be a successful diagnostic result: %s", res.Summary)
	}
	for _, want := range []string{
		"index_windowed=true",
		"parse_diagnostic=zero_events_in_selected_index_window",
		"ftrace-compatible text is supported",
		"verify time_start/time_end",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("bounded zero-event diagnostic missing %q:\n%s", want, res.Summary)
		}
	}
	for _, bad := range []string{"future parser adapter", "ftrace is unsupported"} {
		if strings.Contains(res.Summary, bad) {
			t.Fatalf("bounded zero-event diagnostic must not imply unsupported ftrace via %q:\n%s", bad, res.Summary)
		}
	}
}

func TestTraceQueryExplicitPIDIsNotOverriddenByRequestModelTarget(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "explicit_pid.systrace")
	trace := strings.Join([]string{
		`target-42591 (42591) [004] .... 2.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42591 next_prio=52`,
		`peer-1494 (1494) [005] .... 2.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=1494 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{ExactTargets: []string{"42591"}},
		}},
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "explicit_pid.systrace",
		"view":       "event_search",
		"pid":        1494,
		"time_start": 2.0,
		"time_end":   2.1,
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "pid=1494") || !strings.Contains(res.Summary, "peer-1494") {
		t.Fatalf("explicit pid should win over request model target:\n%s", res.Summary)
	}
	if strings.Contains(res.Summary, "trace_query_target_inherited=true") || strings.Contains(res.Summary, "target-42591") {
		t.Fatalf("explicit pid query should not be overwritten by inherited target:\n%s", res.Summary)
	}
}

func TestTraceQueryDoesNotInheritAmbiguousRequestModelTargets(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "ambiguous_pid.systrace")
	trace := strings.Join([]string{
		`target-42591 (42591) [004] .... 2.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42591 next_prio=52`,
		`peer-1494 (1494) [005] .... 2.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=1494 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{ExactTargets: []string{"42591", "1494"}},
		}},
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "ambiguous_pid.systrace",
		"view":       "event_search",
		"time_start": 2.0,
		"time_end":   2.1,
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"target-42591", "peer-1494"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("ambiguous targets should not auto-narrow; missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "trace_query_target_inherited=true") ||
		strings.Contains(strings.SplitN(res.Summary, "\n", 2)[0], " pid=42591 ") {
		t.Fatalf("ambiguous targets should not inherit one pid:\n%s", res.Summary)
	}
}

func TestTraceQueryDoesNotInferDroppedPIDFromAnalyzerHintStringsWithTimestamps(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "untyped_pid.systrace")
	trace := strings.Join([]string{
		`target-42591 (42591) [004] .... 2.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42591 next_prio=52`,
		`peer-1494 (1494) [005] .... 2.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=1494 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{ExactTargets: []string{"42591", "6793222.031397627", "6793225.369801793"}},
		}},
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "untyped_pid.systrace",
		"view":       "event_search",
		"time_start": 2.0,
		"time_end":   2.1,
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"trace_query_target_not_inherited=true", "target-42591", "peer-1494", "matched_events=2"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("untyped analyzer strings should remain broad and include %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "trace_query_target_inherited=true") {
		t.Fatalf("untyped analyzer strings must not be inherited as trace filters:\n%s", res.Summary)
	}
}

func TestTraceQueryInheritsDroppedPIDFromTypedRuntimeTarget(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "typed_pid.systrace")
	trace := strings.Join([]string{
		`target-42591 (42591) [004] .... 2.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42591 next_prio=52`,
		`peer-1494 (1494) [005] .... 2.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=1494 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeTargets: []types.RuntimeTarget{{
				Kind:       types.RuntimeTargetKindProcess,
				PID:        42591,
				Source:     "user_explicit",
				Confidence: 0.95,
			}},
		}},
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "typed_pid.systrace",
		"view":       "event_search",
		"time_start": 2.0,
		"time_end":   2.1,
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	if target, ok := traceQuerySingleRuntimeTarget(ctx); !ok || target.PID != 42591 {
		t.Fatalf("expected unique typed runtime target, got target=%+v ok=%v", target, ok)
	}
	for _, want := range []string{"trace_query_target_inherited=true", "pid=42591", "source=user_explicit", "target-42591", "matched_events=1"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("omitted pid query should inherit unique typed runtime target and include %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "peer-1494") {
		t.Fatalf("inherited pid query should not include unrelated peer rows:\n%s", res.Summary)
	}
}

func TestTraceQueryRecordsExplicitPIDForLaterTypedInheritance(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "recorded_pid.systrace")
	trace := strings.Join([]string{
		`target-42591 (42591) [004] .... 2.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=target next_pid=42591 next_prio=52`,
		`peer-1494 (1494) [005] .... 2.010000: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=peer next_pid=1494 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	mu := types.NewMutableState("analyze pid 42591")
	initial := types.RequestModel{AnalyzerHints: types.AnalyzerHints{ExactTargets: []string{"42591"}}}
	mu.SetRequestModel(initial)
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: initial,
		},
	}
	explicitParams, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "recorded_pid.systrace",
		"view":       "event_search",
		"pid":        42591,
		"time_start": 2.0,
		"time_end":   2.1,
		"limit":      10,
	})
	if res, err := (&TraceQuery{}).Execute(ctx, explicitParams); err != nil || !res.Success {
		t.Fatalf("explicit pid trace_query failed: res=%+v err=%v", res, err)
	}
	if rm := mu.RequestModel(); rm == nil || len(rm.RuntimeTargets) != 1 || rm.RuntimeTargets[0].PID != 42591 {
		t.Fatalf("explicit trace_query pid should record typed runtime target, got %+v", rm)
	}
	followupParams, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "recorded_pid.systrace",
		"view":       "event_search",
		"time_start": 2.0,
		"time_end":   2.1,
		"limit":      10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, followupParams)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("follow-up trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"trace_query_target_inherited=true", "pid=42591", "source=trace_query_explicit_tool_call", "target-42591", "matched_events=1"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("follow-up query should inherit recorded typed target and include %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "peer-1494") {
		t.Fatalf("follow-up query should not include unrelated peer rows:\n%s", res.Summary)
	}
}

func TestTraceQueryTypedRuntimeTargetKeepsThreadPIDPairingSafe(t *testing.T) {
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RuntimeTargets: []types.RuntimeTarget{{
			Kind:       types.RuntimeTargetKindThread,
			PID:        56284,
			Thread:     "Thread-10 [56284]",
			Source:     "user_explicit",
			Confidence: 0.95,
		}},
	}}}
	target, ok := traceQuerySingleRuntimeTarget(ctx)
	if !ok || target.PID != 56284 || target.Thread != "Thread-10 [56284]" {
		t.Fatalf("expected pid/thread pair from typed runtime target, got target=%+v ok=%v", target, ok)
	}
	ctx.AnalysisIR.RequestModel.RuntimeTargets = []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindProcess, PID: 42591, Source: "user_explicit", Confidence: 0.9},
		{Kind: types.RuntimeTargetKindThread, Thread: "Thread-10", Source: "user_explicit", Confidence: 0.9},
	}
	if target, ok := traceQuerySingleRuntimeTarget(ctx); ok {
		t.Fatalf("multiple typed runtime targets should not auto-inherit one filter, got %+v", target)
	}
}

func TestTraceQueryAttachedTraceNormalizesDotPath(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	ctx := &types.BusContext{
		RepoRoot:        dir,
		WorkDir:         dir,
		AttachedHitrace: trace,
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       ".",
		"view":       "wakeup_chain",
		"pid":        20,
		"time_start": 1.0,
		"time_end":   1.1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should normalize dot path to attached_trace, got: %s", res.Summary)
	}
	for _, want := range []string{"source=attached_trace", "Wakeup chain", "waker-10"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("attached dot-path query missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryDirectoryPathWithoutAttachmentFailsFast(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source": "path",
		"path":   ".",
		"view":   "window_stats",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Fatalf("directory path without attachment should fail fast: %s", res.Summary)
	}
	for _, want := range []string{"source=path requires a trace file", "resolves to a directory", "source=\"attached_trace\""} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("directory refusal missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryExplicitFilePathIsNotOverriddenByAttachment(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "explicit.systrace")
	trace := strings.Join([]string{
		`explicit-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot:        dir,
		WorkDir:         dir,
		AttachedHitrace: `attached-30 (30) [000] .... 1.000000: sched_wakeup: comm=other pid=40 prio=53 target_cpu=001`,
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "explicit.systrace",
		"view":       "event_search",
		"pattern":    "explicit-10",
		"time_start": 1.0,
		"time_end":   1.03,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("explicit trace path failed: %s", res.Summary)
	}
	for _, want := range []string{"source=path", "explicit-10"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("explicit path query missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "source=attached_trace") {
		t.Fatalf("explicit file path should not be normalized to attached_trace:\n%s", res.Summary)
	}
}

func TestTraceQueryAttachedSourceHintControlsPrioritySemantics(t *testing.T) {
	dir := t.TempDir()
	trace := strings.Join([]string{
		` system_server-1000 (1000) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=system_server next_pid=1000 next_prio=98`,
		` SurfaceFlinger-2000 (2000) [001] .... 1.010000: sched_wakeup: comm=system_server pid=1000 prio=98 target_cpu=000`,
	}, "\n")
	ctx := &types.BusContext{
		RepoRoot:              dir,
		WorkDir:               dir,
		AttachedHitrace:       trace,
		AttachedHitraceSource: "android_atrace",
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "attached_trace",
		"view":       "window_stats",
		"time_start": 1.0,
		"time_end":   1.02,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "trace_flavor=android_atrace") ||
		!strings.Contains(res.Summary, "Android/atrace ftrace priority") ||
		strings.Contains(res.Summary, "1-40=CFS") {
		t.Fatalf("attached atrace hint should use Android/raw priority semantics:\n%s", res.Summary)
	}
}

func TestTraceQueryRunnableContextSummaryObservationsAndAliases(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "runnable_context.systrace")
	trace := strings.Join([]string{
		`idle/4-0 (0) [004] .... 1.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`bgA-200 (900) [000] .... 1.000000: cpu_frequency: state=900000 cpu_id=0`,
		`big-300 (901) [004] .... 1.000000: cpu_frequency: state=2400000 cpu_id=4`,
		`bgA-200 (900) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=bgA next_pid=200 next_prio=20`,
		`ctrl-300 (900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x3 cpuset=top-app target_cpu=0 policy=bind`,
		`ctrl-300 (900) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000`,
		`app-100 (100) [000] .... 1.010000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,1,0 cg=top-app`,
		`app-100 (100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":        "path",
		"path":          "runnable_context.systrace",
		"view":          "window_stats",
		"pid":           100,
		"time_start":    1.0,
		"time_end":      1.012,
		"trace_flavor":  "harmony_hitrace",
		"core_topology": "small=0-3,big=4-7",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"- thread_cpu_load thread=bgA-200",
		"- cpu_constraint thread=app-100",
		"- runnable_context thread=app-100",
		"top_background_threads=bgA-200/",
		"allowed_cpus=0,1",
		"core_class=small",
		"restricted_to_busy_or_small_cores",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("runnable context summary missing %q:\n%s", want, res.Summary)
		}
	}
	seenPredicates := map[string]bool{}
	for _, row := range res.Observations {
		seenPredicates[row.Predicate] = true
	}
	for _, want := range []string{"thread_cpu_load", "cpu_constraint", "runnable_context", "process_cpu_load"} {
		if !seenPredicates[want] {
			t.Fatalf("typed observations missing %s: %+v", want, res.Observations)
		}
	}
	searchParams, _ := json.Marshal(map[string]any{
		"source":      "path",
		"path":        "runnable_context.systrace",
		"view":        "event_search",
		"time_start":  1.0,
		"time_end":    1.012,
		"event_types": "cpuAffinity",
	})
	searchRes, err := (&TraceQuery{}).Execute(ctx, searchParams)
	if err != nil {
		t.Fatal(err)
	}
	if !searchRes.Success || !strings.Contains(searchRes.Summary, "matched_events=") ||
		!strings.Contains(searchRes.Summary, "cpu_constraint") {
		t.Fatalf("event_types alias cpuAffinity should find constraint evidence:\n%s", searchRes.Summary)
	}
}

func TestTraceQueryAttachedBlobPathInheritsAttachedSourceHint(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, promptctx.AttachedTraceBlobName)
	trace := strings.Join([]string{
		`  HeapTaskDaemon-2532  ( 2519) [000] d..3 168758.663107: sched_switch: prev_comm=HeapTaskDaemon prev_pid=2532 prev_prio=124 prev_state=R ==> next_comm=rcu_preempt next_pid=7 next_prio=98`,
		`     rcu_preempt-7     (    7) [000] d..3 168758.663126: sched_switch: prev_comm=rcu_preempt prev_pid=7 prev_prio=98 prev_state=S ==> next_comm=HeapTaskDaemon next_pid=2532 next_prio=124`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot:              dir,
		WorkDir:               dir,
		AttachedHitraceSource: "harmony_hitrace",
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       tracePath,
		"view":       "window_stats",
		"time_start": 168758.663,
		"time_end":   168758.664,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"trace_flavor=harmony_hitrace", "larger numeric value means higher priority", "1-40=CFS"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("attached blob path should inherit Harmony hint, missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryRawRequestPlatformWordsDoNotSelectFlavor(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := strings.Join([]string{
		`           ACCS0-2716  ( 2519) [000] d..6 168758.662898: sched_wakeup: comm=Binder:924_3 pid=1200 prio=120 target_cpu=001`,
		`  HeapTaskDaemon-2532  ( 2519) [000] d..3 168758.663107: sched_switch: prev_comm=HeapTaskDaemon prev_pid=2532 prev_prio=124 prev_state=R ==> next_comm=rcu_preempt next_pid=7 next_prio=98`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState("这是一段 OpenHarmony/鸿蒙 bytrace 文本，优先级语义按鸿蒙处理"),
	}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "sample.systrace",
		"view":       "window_stats",
		"time_start": 168758.662,
		"time_end":   168758.664,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, forbidden := range []string{
		"trace flavor was selected from explicit user request",
		"larger numeric value means higher priority",
		"1-40=CFS",
		"priority_rule=harmony_larger_numeric_higher_1_40_CFS_41_139_RT",
	} {
		if strings.Contains(res.Summary, forbidden) {
			t.Fatalf("raw user wording must not select trace flavor/platform, found %q:\n%s", forbidden, res.Summary)
		}
	}
}

func TestTraceQueryDonghuPlatformKeepsHarmonySemanticsWithAndroidSurface(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "donghu.systrace")
	trace := strings.Join([]string{
		`com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch: prev_comm=com.tencent.mm prev_pid=36379 prev_prio=53 prev_state=S ==> next_comm=OS_FFRT_0_0 next_pid=49634 next_prio=20 next_info=rtq cg=top-app`,
		`OS_FFRT_0_0-49634 (48679) [000] .... 2942.130000: sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=004`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "donghu.systrace",
		"view":       "event_search",
		"platform":   "donghu",
		"time_start": 2942.12,
		"time_end":   2942.14,
		"event_types": []string{
			"sched_switch",
			"sched_wakeup",
		},
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"platform=donghu",
		"framework_mode=process_isolated_mixed",
		"trace_flavor=harmony_hitrace",
		"larger numeric value means higher priority",
		"1-40=CFS",
		"framework_surfaces=android_framework",
		"harmony_framework",
		"next_info=rtq",
		"cgroup=top-app",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("donghu summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryEventSearchShowsFlavorPriorityClasses(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := strings.Join([]string{
		`           ACCS0-2716  ( 2519) [000] d..6 168758.662898: sched_wakeup: comm=Binder:924_3 pid=1200 prio=120 target_cpu=001`,
		`  HeapTaskDaemon-2532  ( 2519) [000] d..3 168758.663107: sched_switch: prev_comm=HeapTaskDaemon prev_pid=2532 prev_prio=124 prev_state=R ==> next_comm=rcu_preempt next_pid=7 next_prio=98`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState("这是一段 Harmony/鸿蒙 trace，优先级语义按鸿蒙处理"),
	}
	params, _ := json.Marshal(map[string]any{
		"source":       "path",
		"path":         "sample.systrace",
		"view":         "event_search",
		"time_start":   168758.662,
		"time_end":     168758.664,
		"trace_flavor": "harmony_hitrace",
		"event_types":  []string{"sched_wakeup", "sched_switch"},
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"wakee_prio=120/ohos_rt",
		"prev_prio=124/ohos_rt",
		"next_prio=98/ohos_rt",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing priority class %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryExplicitTraceFlavorParamOverridesContent(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.htrace")
	trace := `OS_FFRT_0_0-49634 (48679) [000] .... 928.081851: sched_wakeup: comm=udk-irq-0 pid=73 prio=301 target_cpu=000`
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":       "path",
		"path":         "sample.htrace",
		"view":         "event_search",
		"trace_flavor": "android_atrace",
		"time_start":   928.081,
		"time_end":     928.082,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "trace_flavor=android_atrace") ||
		!strings.Contains(res.Summary, "trace flavor was selected from explicit trace_query parameter") {
		t.Fatalf("explicit trace_flavor should be reflected in summary:\n%s", res.Summary)
	}
}

func TestTraceQueryPlatformAliasSurvivesStringWrappedCompat(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := `system_server-1000 (1000) [000] .... 1.000000: sched_wakeup: comm=system_server pid=1000 prio=98 target_cpu=000`
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`"{\"source\":\"path\",\"path\":\"sample.systrace\",\"view\":\"event_search\",\"platform\":\"android_atrace\",\"time_start\":\"1.0s\",\"time_end\":\"1.1s\",\"limit\":\"3\"}"`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should accept compat-repaired string-wrapped params: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "trace_flavor=android_atrace") ||
		!strings.Contains(res.Summary, "Android/atrace ftrace priority") {
		t.Fatalf("platform alias should flow through compat repair and flavor selection:\n%s", res.Summary)
	}
}

func TestTraceQueryNewParamsSurviveStructuredCompatAliases(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "sample.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.100000: print: B|20|Choreographer#doFrame`,
		`app-20 (20) [001] .... 1.120000: print: E|20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`"{\"source\":\"path\",\"path\":\"sample.systrace\",\"view\":\"interaction_stats\",\"pid\":\"20\",\"timeStart\":\"1.0s\",\"timeEnd\":\"1.2s\",\"pattern\":\"Choreographer\",\"spanName\":\"Choreographer#doFrame\",\"interactionDirection\":\"incoming\",\"recipeName\":\"jank\",\"traceFlavor\":\"android_atrace\"}"`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should accept compat-repaired camelCase params: %s", res.Summary)
	}
	for _, want := range []string{"trace_flavor=android_atrace", "pattern=Choreographer", "span_name=Choreographer#doFrame", "interaction_direction=incoming", "recipe_name=jank", "wake_to_target=1"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing compat-repaired %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryCoreTopologySurvivesCompatAndRenders(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "core.systrace")
	trace := strings.Join([]string{
		`freq-1 (1) [000] .... 10.000000: cpu_frequency: state=800000 cpu_id=0`,
		`freq-1 (1) [004] .... 10.000000: cpu_frequency: state=2200000 cpu_id=4`,
		`app-20 (20) [004] .... 10.010000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=65535 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [004] .... 10.050000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=80`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`"{\"source\":\"path\",\"path\":\"core.systrace\",\"view\":\"window_stats\",\"timeStart\":\"10.0s\",\"timeEnd\":\"10.06s\",\"coreTopology\":\"small=0-3,big=4-7\"}"`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should accept core topology compat params: %s", res.Summary)
	}
	for _, want := range []string{"core_class=big", "source=explicit", "compute_supply"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryPerfSampleEventTypesSurviveCompatAliases(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "samples.perftrace")
	trace := strings.Join([]string{
		`app-5678 (1234) [005] .... 20.000100: perf_sample: pid=1234 tid=5678 cpu=5 period=10000 event=cpu-cycles symbol=Foo::bar dso=libfoo.so callchain=main;A;Foo::bar`,
		`app-5678 (1234) [005] .... 20.000200: perf_sample: pid=1234 tid=5678 cpu=5 period=30000 event=cpu-cycles symbol=Foo::bar dso=libfoo.so callchain=main;A;Foo::bar`,
		`worker-6000 (1234) [006] .... 20.000300: sched_wakeup: comm=app pid=5678 prio=53 target_cpu=005`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`"{\"source\":\"path\",\"path\":\"samples.perftrace\",\"view\":\"event_search\",\"pattern\":\"Foo::bar\",\"eventTypes\":\"cpu sample,topSymbols\",\"timeStart\":\"20.0s\",\"timeEnd\":\"20.001s\"}"`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should accept perf sample aliases through compat repair: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "matched_events=2") {
		t.Fatalf("perf sample aliases should normalize to event_types=[perf_sample]:\n%s", res.Summary)
	}
}

func TestTraceQueryOfficialSystraceEventTypeAliasesNormalize(t *testing.T) {
	got := parseTraceQueryEventTypes([]string{
		"sched_wakeup_new",
		"sched_stat_iowait",
		"sched_stat_accounting",
		"ipi_raise",
		"ipi_activity",
		"block_rq_insert",
		"block_getrq",
		"block_bio_queue",
		"block_bio_complete",
		"print",
		"tracing_mark_write_xacct",
		"xacct_tracing_mark_write",
	})
	want := []tracequery.EventType{
		tracequery.EventSchedWakeup,
		tracequery.EventSchedStat,
		tracequery.EventSchedStat,
		tracequery.EventIPI,
		tracequery.EventIPI,
		tracequery.EventBlockIssue,
		tracequery.EventBlockIssue,
		tracequery.EventBlockIssue,
		tracequery.EventBlockComplete,
		tracequery.EventTraceMark,
		tracequery.EventTraceMark,
		tracequery.EventTraceMark,
	}
	if len(got) != len(want) {
		t.Fatalf("event type aliases length mismatch: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event type alias %d mismatch: got=%v want=%v", i, got, want)
		}
	}
}

func TestTraceQuerySchedStatAndIPIAreConsumable(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "schedstat_ipi.systrace")
	trace := strings.Join([]string{
		`worker-30 (30) [002] .... 3.000000: sched_stat_iowait: comm=worker pid=30 delay=3500000 [ns]`,
		`irq-2 (2) [002] .... 3.001000: ipi_raise: target_mask=0x10 (Rescheduling interrupts)`,
		`irq-2 (2) [004] .... 3.002000: ipi_entry: (Rescheduling interrupts)`,
		`irq-2 (2) [004] .... 3.003000: ipi_exit: (Rescheduling interrupts)`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{"source":"path","path":"schedstat_ipi.systrace","view":"window_stats","eventTypes":"sched_stat_iowait,ipi_raise","timeStart":"3.0s","timeEnd":"3.004s"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"sched_stat_accounting", "kind=iowait", "ipi_activity", "target_mask=0x10", "supply_pressure", "sched_stat_iowait=3.500", "ipi_events=3"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryPerfBundleViewAliasRendersPerfContext(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "bundle.perftrace")
	trace := strings.Join([]string{
		`app-5678 (1234) [005] .... 20.000100: sched_switch: prev_comm=idle/5 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=5678 next_prio=53`,
		`app-5678 (1234) [005] .... 20.000200: perf_sample: pid=1234 tid=5678 cpu=5 period=10000 event=cpu-cycles symbol=Foo::bar dso=libfoo.so callchain=main;A;Foo::bar`,
		`app-5678 (1234) [005] .... 20.001000: sched_switch: prev_comm=app prev_pid=5678 prev_prio=53 prev_state=R+ ==> next_comm=idle/5 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`"{\"source\":\"path\",\"path\":\"bundle.perftrace\",\"view\":\"perfBundle\",\"pid\":\"5678\",\"timeStart\":\"20.0s\",\"timeEnd\":\"20.002s\"}"`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query should accept perfBundle view alias: %s", res.Summary)
	}
	for _, want := range []string{"# Trace Query: trace_perf_bundle", "perf_top_symbol symbol=Foo::bar", "perf_quality", "quality=cpu_known=", "## Root cause rank"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("perf bundle summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQuerySummaryRendersFragmentedStateChurn(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "fragmented.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 11.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 11.000300: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=53`,
		`rival-30 (30) [001] .... 11.000800: sched_switch: prev_comm=rival prev_pid=30 prev_prio=53 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 11.001100: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=53`,
		`rival-30 (30) [001] .... 11.001600: sched_switch: prev_comm=rival prev_pid=30 prev_prio=53 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 11.001900: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=53`,
		`rival-30 (30) [001] .... 11.002400: sched_switch: prev_comm=rival prev_pid=30 prev_prio=53 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 11.002700: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=53`,
		`rival-30 (30) [001] .... 11.003200: sched_switch: prev_comm=rival prev_pid=30 prev_prio=53 prev_state=R+ ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 11.003500: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=rival next_pid=30 next_prio=53`,
		`rival-30 (30) [001] .... 11.004000: sched_switch: prev_comm=rival prev_pid=30 prev_prio=53 prev_state=S ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{"source":"path","path":"fragmented.systrace","view":"rootCauseRank","pid":"20","timeStart":"11.0s","timeEnd":"11.004s","limit":"6"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"type=fragmented_runnable_wait",
		"state_churn app-20 dominant_state=runnable",
		"max_segment=0.500ms",
		"next_step=inspect rival-30 on same CPU cpu=1",
		"sched_wakeup",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("fragmented state churn summary missing %q:\n%s", want, res.Summary)
		}
	}

	aliasParams := json.RawMessage(`{"source":"path","path":"fragmented.systrace","view":"stateChurn","pid":"20","timeStart":"11.0s","timeEnd":"11.004s","limit":"6"}`)
	aliasRes, err := (&TraceQuery{}).Execute(ctx, aliasParams)
	if err != nil {
		t.Fatal(err)
	}
	if !aliasRes.Success {
		t.Fatalf("trace_query should repair view=stateChurn to window_stats: %s", aliasRes.Summary)
	}
	for _, want := range []string{
		"# Trace Query: window_stats",
		"state_churn app-20 dominant_state=runnable",
		"next_step=inspect rival-30 on same CPU cpu=1",
		"sched_wakeup",
	} {
		if !strings.Contains(aliasRes.Summary, want) {
			t.Fatalf("stateChurn view alias summary missing %q:\n%s", want, aliasRes.Summary)
		}
	}
}

func TestTraceQuerySummaryRendersWakeupCausalImpactsAndRepairsAlias(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "causal.systrace")
	trace := strings.Join([]string{
		`app-100 (100) [001] .... 1.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`worker-200 (100) [002] .... 1.001000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120`,
		`net-300 (100) [003] .... 1.001200: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=002`,
		`worker-200 (100) [002] .... 1.009500: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20`,
		`worker-200 (100) [002] .... 1.010000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001`,
		`worker-200 (100) [002] .... 1.010020: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=app next_pid=100 next_prio=52`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{"source":"path","path":"causal.systrace","view":"rootCauseRank","pid":"100","timeStart":"1.0s","timeEnd":"1.010s","traceFlavor":"harmony_hitrace","minDurationMs":"0.05","limit":"6"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"# Trace Query: root_cause_rank",
		"causal_impact thread=worker-200",
		"dominant_state=runnable",
		"priority_relation=lower_priority_dependency",
		"priority_inversion_candidate=true",
		"causality=on_wakeup_chain",
		"chain_depth=1",
		"target_impact=",
		"projected_impact=",
		"actual_impact=",
		"actual_window=",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("wakeup causal summary missing %q:\n%s", want, res.Summary)
		}
	}

	aliasParams := json.RawMessage(`{"source":"path","path":"causal.systrace","view":"causalImpact","pid":"100","timeStart":"1.0s","timeEnd":"1.010s","traceFlavor":"harmony_hitrace","minDurationMs":"0.05","limit":"6"}`)
	aliasRes, err := (&TraceQuery{}).Execute(ctx, aliasParams)
	if err != nil {
		t.Fatal(err)
	}
	if !aliasRes.Success {
		t.Fatalf("trace_query should repair view=causalImpact to wakeup_chain: %s", aliasRes.Summary)
	}
	for _, want := range []string{
		"# Trace Query: wakeup_chain",
		"causal_impact thread=worker-200",
		"priority_relation=lower_priority_dependency",
	} {
		if !strings.Contains(aliasRes.Summary, want) {
			t.Fatalf("causalImpact view alias summary missing %q:\n%s", want, aliasRes.Summary)
		}
	}
}

func TestTraceQuerySummaryRendersInodeIOAndRepairsEventTypeAliases(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "inode_io.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 12.000000: android_fs_datawrite_start: entry_name=foo.db offset=0 bytes=4096 cmdline=app pid=20 i_size=8192 ino=0xb9b8e`,
		`app-20 (20) [001] .... 12.001000: android_fs_datawrite_end: ino=0xb9b8e offset=0 bytes=4096 ret=4096 latency_us=700`,
		`app-20 (20) [001] .... 12.002000: mm_filemap_add_to_page_cache: dev 260:136 ino 0xb9b8e page=0 pfn=1 ofs=0`,
		`app-20 (20) [001] .... 12.003000: mm_filemap_delete_from_page_cache: dev 260:136 ino 0xb9b8e page=0 pfn=1 ofs=0`,
		`app-20 (20) [001] .... 12.004000: scsi_dispatch_cmd_start: dev=12,80 op=write bytes=4096`,
		`app-20 (20) [001] .... 12.006000: scsi_dispatch_cmd_done: dev=12,80 op=write bytes=4096`,
		`app-20 (20) [001] .... 12.007000: sched_blocked_reason: pid=20 iowait=1 caller=fscache_page_wait_on_page_bit`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{"source":"path","path":"inode_io.systrace","view":"windowStats","timeStart":"12.0s","timeEnd":"12.01s","event_types":"inodeIO,pageCache,storageLayerLatency","limit":"8"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"# Trace Query: window_stats",
		"file_io inode=0xb9b8e",
		"name=foo.db",
		"completions=1",
		"ret=4096",
		"example=entry_name=foo.db offset=0 bytes=4096",
		"page_cache inode=0xb9b8e",
		"storage_latency layer=scsi",
		"example=dev=12,80 op=write bytes=4096",
		"io_pressure signal=",
		"iowait_blocked=1",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("inode IO summary missing %q:\n%s", want, res.Summary)
		}
	}

	searchParams := json.RawMessage(`{"source":"path","path":"inode_io.systrace","view":"eventSearch","pattern":"0xb9b8e","event_types":"pageCache","limit":"4"}`)
	search, err := (&TraceQuery{}).Execute(ctx, searchParams)
	if err != nil {
		t.Fatal(err)
	}
	if !search.Success || !strings.Contains(search.Summary, "matched_events=2") || !strings.Contains(search.Summary, "file_io dev=260:136 inode=0xb9b8e") {
		t.Fatalf("event_type alias pageCache should be normalized and surface inode details:\n%s", search.Summary)
	}
}

func TestTraceQueryFrameRootCauseBundleAliasSummaryAndObservations(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "bundle.systrace")
	trace := strings.Join([]string{
		`app-100 (100) [001] .... 10.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`logger-900 (900) [006] .... 10.000500: sched_switch: prev_comm=logger prev_pid=900 prev_prio=20 prev_state=D ==> next_comm=idle/6 next_pid=0 next_prio=120`,
		`threadpool-400 (100) [004] .... 10.001000: sched_switch: prev_comm=threadpool prev_pid=400 prev_prio=20 prev_state=D ==> next_comm=idle/4 next_pid=0 next_prio=120`,
		`io-2 (2) [004] .... 10.001100: sched_blocked_reason: pid=400 iowait=1 caller=f2fs_wait_on_block`,
		`threadpool-400 (100) [004] .... 10.002000: tracing_mark_write: B|400|NativeAsyncFileRead inode=0xabc`,
		`threadpool-400 (100) [004] .... 10.002100: android_fs_dataread_start: dev=259:1 ino=0xabc entry_name=foo.db offset=0 bytes=4096 rw=R`,
		`threadpool-400 (100) [004] .... 10.009100: android_fs_dataread_end: dev=259:1 ino=0xabc entry_name=foo.db offset=0 bytes=4096 rw=R ret=4096 latency_us=7000`,
		`threadpool-400 (100) [004] .... 10.013800: tracing_mark_write: E|400`,
		`irq-7 (7) [004] .... 10.003000: irq_handler_entry: irq=17 name=ufs`,
		`irq-7 (7) [004] .... 10.003700: irq_handler_exit: irq=17 name=ufs`,
		`wq-8 (8) [004] .... 10.004000: workqueue_execute_start: work=0xff function=flush_cookie`,
		`wq-8 (8) [004] .... 10.006000: workqueue_execute_end: work=0xff function=flush_cookie`,
		`clk-1 (1) [004] .... 10.004500: clock_set_rate: ddr_clk state=933000 cpu_id=4`,
		`network-300 (100) [003] .... 10.009000: sched_switch: prev_comm=network prev_pid=300 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120`,
		`cookie-200 (100) [002] .... 10.010000: sched_switch: prev_comm=cookie prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120`,
		`io-2 (2) [004] .... 10.014000: sched_wakeup: comm=threadpool pid=400 prio=20 target_cpu=004`,
		`threadpool-400 (100) [004] .... 10.015000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=threadpool next_pid=400 next_prio=20`,
		`threadpool-400 (100) [004] .... 10.016000: sched_wakeup: comm=network pid=300 prio=20 target_cpu=003`,
		`network-300 (100) [003] .... 10.017000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=network next_pid=300 next_prio=20`,
		`network-300 (100) [003] .... 10.018000: sched_wakeup: comm=cookie pid=200 prio=20 target_cpu=002`,
		`cookie-200 (100) [002] .... 10.019000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=cookie next_pid=200 next_prio=20`,
		`cookie-200 (100) [002] .... 10.020000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001`,
		`app-100 (100) [001] .... 10.020020: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{"source":"path","path":"bundle.systrace","view":"frameBundle","pid":"100","timeStart":"10.0s","timeEnd":"10.020s","traceFlavor":"harmony_hitrace","coreTopology":"small=0-3,big=4-7","limit":"12"}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"# Trace Query: frame_root_cause_bundle",
		"Frame root cause bundle",
		"bundle_top_cause",
		"chain_relevance=on_chain",
		"bundle_io_burst",
		"io_burst_episode",
		"supply_pressure",
		"trace_mark_category",
		"async_file_work",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("bundle summary missing %q:\n%s", want, res.Summary)
		}
	}
	seenPredicates := map[string]bool{}
	for _, row := range res.Observations {
		seenPredicates[row.Predicate] = true
	}
	for _, want := range []string{"root_cause_primary", "io_burst_episode", "irq_activity", "workqueue_activity", "async_file_work"} {
		if !seenPredicates[want] {
			t.Fatalf("typed observations missing %s: %+v", want, res.Observations)
		}
	}
}

func TestTraceQuerySchemaDocumentsViews(t *testing.T) {
	body := (&TraceQuery{}).Description() + "\n" + string((&TraceQuery{}).Parameters())
	for _, want := range []string{"wakeup_chain", "thread_timeline", "window_stats", "perf_stats", "perf_timeline", "trace_perf_bundle", "perf_bundle", "trace_plus_perf", "scheduler_latency_stats", "critical_blocking_calls", "direct blocking surfaces", "peer_state", "peer/on-chain evidence", "oneway", "sync_like", "blocking_candidate", "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "frame_root_cause_bundle", "frame_bundle", "recipe", "recipe_name", "ipc_graph", "event_search", "span_window", "root_cause_rank", "interaction_stats", "state_churn", "frequent short state switches", "not an independent view", "view=state_churn is accepted and treated as view=window_stats", "causal_impacts", "aggregated_impact", "aggregated_impacts", "occurrence_windows", "representative repeated windows", "view=causal_impact is accepted as wakeup_chain", "chain_relevance", "on_chain", "adjacent", "background", "dominant_state", "cumulative_impact_ms", "effective_impact_ms", "effective_impact", "same-chain primary", "compute-supply", "semantic span-work", "JIT compilation", "class verification", "shader compilation", "runtime compilation", "semantic_class", "perf_sample", "perf_samples", "perf_contexts", "candidate_thread", "target_running", "on_chain_dependency", "same_cpu_competitor", "cpu_pressure_top_running", "compute_supply_cpu", "top_symbols", "top_dso", "top_callchains", "source", "sample_kind", "sample_cpu_scope", "symbolization_status", "raw_perfdata_fallback", "unsymbolized", "lost_records", "lost_samples", "throttle_records", "aux_records", "ftrace-plugin structured metadata", "profiler plugin metadata", "dropped_events", "overrun", "commit_overrun", "overwrite", "trace_clock", "clock_details", "symbol_examples", "tracebundle is recommended context, not required input", "SQL-primary perf_sample rows embedded in systrace", "perf_thread_comm", "comm_source=trace_thread", "trace-aligned identity", "tracebundle_perf_capability", "tracebundle_perf_clock_alignment", "tracebundle_trace_provider", "tracebundle_trace_db_coverage", "tracebundle_trace_coverage", "tracebundle_trace_tool_gate", "SQL table coverage", "trace_query cross-validation", "role=resolver_index", "role=perftrace_text_output", "rows_emitted=0 is expected", "cpuSample", "perfSamples", "file_io_by_inode", "page_cache_by_inode", "storage_latency_by_layer", "block_io_by_inode", "io_burst_episodes", "io_pressure_summary", "irq_activity", "softirq_activity", "ipi_activity", "sched_stat_accounting", "workqueue_activity", "supply_pressure_summary", "trace_mark_categories", "async_file_work", "completion", "completions/ret/example", "file_io", "page_cache", "android_fs", "f2fs", "scsi", "mmc", "storage_latency", "io_pressure", "inode_io", "pageCache", "storageLayerLatency", "pattern", "not a regex", "B/E/C/S/F", "span_action", "span_pid", "span_value", "kind=sync|async", "same ftrace thread stack", "E|<pid>|<span_name>", "marker pid + name + cookie", "selected_window", "thread/pid alone", "span_name", "interaction_direction", "attached_trace", "trace_flavor", "android_atrace", "generic_ftrace", "typed platform hint", "Raw user wording is not re-parsed", "seconds", "microsecond precision", "81774 us", "larger numeric priority", "1-40=CFS", "raw scheduler priority", "cpu_frequency", "cpu_frequency_limits", "clock_set_rate", "core_topology", "small=0-3", "sched_wakeup_new", "sched_stat_wait", "sched_stat_iowait", "sched_stat_runtime", "ipi_raise", "ipi_entry", "ipi_exit", "block_rq_insert", "block_getrq", "block_bio_queue", "block_bio_complete", "tracing_mark_write_xacct", "xacct_tracing_mark_write", "block_bio_remap", "sched_blocked_reason", "binder_transaction_received", "binder_transaction_alloc_buf", "binder_lock", "softirq", "ipi", "storage", "filesystem", "eBPF BIO", "PageFault", "Ability", "XPower", "HiSystemEvent", "ability_monitor", "xpower", "hi_sysevent", "power", "workqueue", "dma_fence"} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace_query schema/description missing %q:\n%s", want, body)
		}
	}
}

func TestTraceQuerySchemaDocumentsFtraceAndCompoundTime(t *testing.T) {
	body := (&TraceQuery{}).Description() + "\n" + string((&TraceQuery{}).Parameters())
	for _, want := range []string{
		".ftrace",
		".trace",
		"ftrace-compatible text",
		"1s 501ms 565μs 915ns",
		"zero-event result in a bounded window",
		"not evidence that .ftrace is unsupported",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace_query ftrace/time teaching missing %q:\n%s", want, body)
		}
	}
}

func TestTraceQueryDescriptionDocumentsStructuredRequestTargetInheritanceBoundary(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		"set pid/thread explicitly in the tool call",
		"structured request model exposes exactly one runtime_targets entry",
		"trace_query_target_inherited",
		"does not infer omitted pid/thread values from raw request prose, analyzer entity strings",
		"does not infer omitted pid/thread values from raw request prose",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("trace_query description missing precision boundary %q:\n%s", want, description)
		}
	}
	for _, forbidden := range []string{
		"inherit omitted pid",
		"inherit omitted thread",
	} {
		if strings.Contains(description, forbidden) {
			t.Fatalf("trace_query description must not promise raw-request target inheritance %q:\n%s", forbidden, description)
		}
	}
}

func TestPerfQualitySampleCPUScope(t *testing.T) {
	tests := []struct {
		name string
		q    *tracequery.PerfQualitySummary
		want string
	}{
		{name: "nil", q: nil, want: "none"},
		{name: "none", q: &tracequery.PerfQualitySummary{}, want: "none"},
		{name: "known", q: &tracequery.PerfQualitySummary{CPUKnownCount: 2}, want: "known"},
		{name: "unknown", q: &tracequery.PerfQualitySummary{CPUUnknownCount: 1}, want: "unknown"},
		{name: "partial", q: &tracequery.PerfQualitySummary{CPUKnownCount: 2, CPUUnknownCount: 1}, want: "partial"},
	}
	for _, tt := range tests {
		if got := perfQualitySampleCPUScope(tt.q); got != tt.want {
			t.Fatalf("%s sample CPU scope = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestTraceQuerySummaryRendersAggregateOccurrenceWindows(t *testing.T) {
	result := tracequery.Result{
		View:       "root_cause_rank",
		SourcePath: "/tmp/frame.systrace",
		WakeupChain: &tracequery.ChainResult{
			AggregatedImpacts: []tracequery.WakeupCausalAggregate{{
				Thread:            tracequery.ThreadRef{Comm: "ThreadPoolForeg", PID: 60555},
				Path:              "ThreadPoolForeg-60555 -> NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566",
				ChainDepth:        3,
				OccurrenceCount:   3,
				DominantState:     string(tracequery.StateIOWait),
				DominantImpactMs:  9.149,
				ProjectedImpactMs: 9.149,
				TotalMs:           12.0,
				ProjectedTotalMs:  12.0,
				ActualImpactMs:    12.5,
				ActualTotalMs:     15.0,
				TargetBlockedMs:   27.9,
				IOWaitMs:          9.149,
				ActualIOWaitMs:    12.5,
				ActualFirstTs:     34579.520000,
				ActualLastTs:      34579.590000,
				LineStart:         100,
				LineEnd:           180,
				PriorityRelation:  "lower_priority_dependency",
				PriorityInversion: true,
				OccurrenceWindows: []tracequery.WakeupCausalOccurrence{
					{Window: tracequery.TimeWindow{StartTs: 34579.525319, EndTs: 34579.534164}, ActualWindow: tracequery.TimeWindow{StartTs: 34579.520000, EndTs: 34579.534164}, DominantState: string(tracequery.StateIOWait), DominantImpactMs: 3.1, ProjectedImpactMs: 3.1, TotalMs: 3.1, ProjectedTotalMs: 3.1, ActualImpactMs: 4.0, ActualTotalMs: 4.0, TargetBlockedMs: 8.8, IOWaitMs: 3.1, ActualIOWaitMs: 4.0, LineStart: 100, LineEnd: 120},
					{Window: tracequery.TimeWindow{StartTs: 34579.546416, EndTs: 34579.553415}, DominantState: string(tracequery.StateIOWait), DominantImpactMs: 3.0, TotalMs: 3.0, TargetBlockedMs: 7.0, IOWaitMs: 3.0, LineStart: 130, LineEnd: 150},
					{Window: tracequery.TimeWindow{StartTs: 34579.576702, EndTs: 34579.587805}, DominantState: string(tracequery.StateIOWait), DominantImpactMs: 3.049, TotalMs: 3.049, TargetBlockedMs: 11.1, IOWaitMs: 3.049, LineStart: 160, LineEnd: 180},
				},
				Summary: "ThreadPoolForeg repeated D/IO dependency on wakeup chain",
			}},
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Items: []tracequery.RootCauseRankItem{{
				Rank:               1,
				Tier:               "primary",
				Type:               "priority_inversion_candidate",
				Thread:             tracequery.ThreadRef{Comm: "ThreadPoolForeg", PID: 60555},
				StartTs:            34579.525319,
				EndTs:              34579.587805,
				DominantState:      string(tracequery.StateIOWait),
				IOWaitMs:           9.149,
				ImpactMs:           9.149,
				ProjectedImpactMs:  9.149,
				CumulativeImpactMs: 12.0,
				ActualImpactMs:     12.5,
				ActualTotalMs:      15.0,
				ActualStartTs:      34579.520000,
				ActualEndTs:        34579.590000,
				TargetImpactMs:     27.9,
				LineStart:          100,
				LineEnd:            180,
				Source:             "wakeup_chain.aggregated_impacts",
				Causality:          "on_wakeup_chain",
				ChainRelevance:     "on_chain",
				ChainDepth:         3,
				OccurrenceWindows: []tracequery.WakeupCausalOccurrence{
					{Window: tracequery.TimeWindow{StartTs: 34579.525319, EndTs: 34579.534164}, DominantState: string(tracequery.StateIOWait), DominantImpactMs: 3.1, TotalMs: 3.1, TargetBlockedMs: 8.8, IOWaitMs: 3.1, LineStart: 100, LineEnd: 120},
					{Window: tracequery.TimeWindow{StartTs: 34579.546416, EndTs: 34579.553415}, DominantState: string(tracequery.StateIOWait), DominantImpactMs: 3.0, TotalMs: 3.0, TargetBlockedMs: 7.0, IOWaitMs: 3.0, LineStart: 130, LineEnd: 150},
					{Window: tracequery.TimeWindow{StartTs: 34579.576702, EndTs: 34579.587805}, DominantState: string(tracequery.StateIOWait), DominantImpactMs: 3.049, TotalMs: 3.049, TargetBlockedMs: 11.1, IOWaitMs: 3.049, LineStart: 160, LineEnd: 180},
				},
				Summary: "ThreadPoolForeg repeated D/IO dependency on wakeup chain",
			}},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"occurrence_windows=34579.525319..34579.534164",
		"34579.546416..34579.553415",
		"34579.576702..34579.587805",
		"projected_impact=9.149ms",
		"actual_impact=12.500ms",
		"actual_total=15.000ms",
		"actual_window=34579.520000..34579.590000",
		"aggregate_occurrence thread=ThreadPoolForeg-60555",
		"rank_occurrence rank=1 thread=ThreadPoolForeg-60555",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing occurrence detail %q:\n%s", want, summary)
		}
	}
}

func TestTraceQuerySummaryShowsRootCauseEffectiveImpact(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
			Rank:               1,
			Tier:               "primary",
			Type:               "jit_compile",
			Thread:             tracequery.ThreadRef{Comm: "JitWorker", PID: 300},
			StartTs:            7.001,
			EndTs:              7.002,
			ImpactMs:           1.2,
			ProjectedImpactMs:  1.2,
			CumulativeImpactMs: 1.2,
			EffectiveImpactMs:  4.0,
			ActualImpactMs:     1.2,
			ActualTotalMs:      1.2,
			ActualStartTs:      7.001,
			ActualEndTs:        7.002,
			Score:              3.346,
			Confidence:         0.82,
			LineStart:          10,
			LineEnd:            11,
			Source:             "window_stats.trace_spans.semantic",
			Causality:          "on_wakeup_chain",
			ChainRelevance:     "on_chain",
			ChainDepth:         2,
			SpanName:           "JitCompileMethod",
			SpanKind:           "sync",
			SpanCategory:       "runtime_compile",
			SpanSubcategory:    "jit",
			SemanticClass:      "jit_compile",
			Summary:            "JIT compilation span was on the wakeup chain",
		}}},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"effective_impact=4.000ms",
		"projected_impact=1.200ms",
		"actual_impact=1.200ms",
		"span=name=JitCompileMethod effective_impact=4.000ms semantic_class=jit_compile category=runtime_compile subcategory=jit kind=sync",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing effective impact detail %q:\n%s", want, summary)
		}
	}
}

func TestTraceQuerySummaryShowsRootCausePerfRoleContexts(t *testing.T) {
	result := tracequery.Result{
		View: "root_cause_rank",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
			Rank:     1,
			Tier:     "primary",
			Type:     "cpu_pressure",
			ImpactMs: 8,
			PerfContext: &tracequery.PerfContext{
				SampleCount: 1,
				TotalPeriod: 41000,
				TopSymbols:  []tracequery.PerfHotspot{{Symbol: "Rival::hot", DSO: "librival.so", Period: 41000}},
			},
			PerfContexts: []tracequery.RootCausePerfRoleContext{{
				Role:   "cpu_pressure_top_running",
				Thread: tracequery.ThreadRef{Comm: "rival", PID: 300},
				CPU:    0,
				Window: tracequery.TimeWindow{StartTs: 2.0, EndTs: 2.012},
				Reason: "top running thread on pressure CPU",
				PerfContext: &tracequery.PerfContext{
					SampleCount: 1,
					TotalPeriod: 41000,
					TopSymbols: []tracequery.PerfHotspot{{
						Symbol:              "Rival::hot",
						DSO:                 "librival.so",
						Source:              "hiperf_proto",
						SymbolizationStatus: "symbolized",
						Period:              41000,
						SampleCount:         1,
					}},
					TopCallchains: []tracequery.PerfHotspot{{
						Callchain:           "main;Rival::hot",
						Symbol:              "Rival::hot",
						DSO:                 "librival.so",
						Source:              "hiperf_proto",
						SymbolizationStatus: "symbolized",
						Period:              41000,
						SampleCount:         1,
						LineStart:           12,
						LineEnd:             12,
					}},
				},
			}},
			Summary: "cpu pressure on cpu=0",
		}}},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "root_cause_rank"}, "path", "/tmp/payload.json")
	for _, want := range []string{
		"perf_contexts=cpu_pressure_top_running",
		"top_symbol=Rival::hot",
		"rank_perf_context rank=1 role=cpu_pressure_top_running",
		"rank_perf_top_callchain role=cpu_pressure_top_running",
		"source=hiperf_proto",
		"symbolization_status=symbolized",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing perf role detail %q:\n%s", want, summary)
		}
	}
}

func TestTraceQueryEventSearchPatternFindsFrameID(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "frame.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [000] .... 9.000000: print: B|20|Choreographer#doFrame 1917295`,
		`app-20 (20) [000] .... 9.001000: print: E|20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":       "path",
		"path":         "frame.systrace",
		"view":         "event_search",
		"pattern":      "1917295",
		"limit":        5,
		"platform":     "donghu",
		"trace_flavor": "harmony_hitrace",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query event_search pattern failed: %s", res.Summary)
	}
	for _, want := range []string{"pattern=1917295", "matched_events=1", "Choreographer#doFrame 1917295"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("event_search pattern summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryEventSearchPatternEmptyGivesRecoveryHint(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "frame_empty.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.100000: print: B|20|Choreographer#doFrame 1917295`,
		`app-20 (20) [001] .... 1.120000: print: E|20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "frame_empty.systrace",
		"view":    "event_search",
		"pattern": "1919999",
		"limit":   5,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"matched_events=0",
		"pattern_no_match_hint",
		"literal substring, not a regex",
		"next_pattern_call_hint=try trace_query(view=\"event_search\"",
		"event_types=[\"trace_mark\"]",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("event_search pattern empty summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryEventSearchBroadPatternWithObjectiveFrameIDGivesExactHint(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "frame_broad.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.100000: print: B|20|Choreographer#doFrame 170048`,
		`app-20 (20) [001] .... 1.120000: print: E|20`,
		`app-20 (20) [001] .... 2.100000: print: B|20|Choreographer#doFrame 173073`,
		`app-20 (20) [001] .... 2.120000: print: E|20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState(`分析 Choreographer#doFrame 173073 这一帧丢帧的深层次原因`),
	}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "frame_broad.systrace",
		"view":    "event_search",
		"pattern": "Choreographer#doFrame",
		"limit":   1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"event_search_limit_reached=true",
		"objective_exact_frame_hint",
		`pattern "Choreographer#doFrame" does not include requested token "173073"`,
		"not evidence that frame 173073 is absent",
		`trace_query(view="frame_window", pattern="173073"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("broad event_search should warn against absence inference, missing %q:\n%s", want, res.Summary)
		}
	}
	if res.Refinement == nil {
		t.Fatalf("broad event_search should attach a typed refinement hint")
	}
	refinement := types.NormalizeToolRefinementHint(*res.Refinement)
	if refinement.ReasonCode != "trace_query_event_search_limit_reached" || !refinement.ResultTruncated {
		t.Fatalf("unexpected refinement: %+v", refinement)
	}
	if refinement.PreferredNextTool != "trace_query" {
		t.Fatalf("preferred next tool = %q", refinement.PreferredNextTool)
	}
	for key, want := range map[string]string{
		"source":  "path",
		"path":    "frame_broad.systrace",
		"view":    "event_search",
		"pattern": "Choreographer#doFrame",
		"limit":   "1",
	} {
		if got := refinement.PreferredParams[key]; got != want {
			t.Fatalf("refinement preferred param %s=%q, want %q in %+v", key, got, want, refinement.PreferredParams)
		}
	}
}

func TestTraceQueryEventSearchBroadPatternWithObjectiveSpanKeywordGivesExactHint(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "span_broad.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.100000: print: B|20|DecodeOther`,
		`app-20 (20) [001] .... 1.120000: print: E|20`,
		`app-20 (20) [001] .... 2.100000: print: B|20|DecodeBitmap`,
		`app-20 (20) [001] .... 2.140000: print: E|20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState(`分析 DecodeBitmap 这个 span 的耗时为什么异常`),
	}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "span_broad.systrace",
		"view":    "event_search",
		"pattern": "Decode",
		"limit":   1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"event_search_limit_reached=true",
		"objective_exact_span_hint",
		`span/marker token "DecodeBitmap"`,
		`pattern "Decode" does not include it`,
		`trace_query(view="span_window", span_name="DecodeBitmap"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("broad event_search should warn for exact span token, missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryEventSearchBroadPatternWithObjectiveKVTokenGivesExactHint(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "kv_broad.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.100000: print: C|20|jank_frames=1|1`,
		`app-20 (20) [001] .... 2.100000: print: C|20|jank_frames=7|1`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState(`分析 jank_frames=7 这一帧为什么丢帧`),
	}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "kv_broad.systrace",
		"view":    "event_search",
		"pattern": "jank_frames",
		"limit":   1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"event_search_limit_reached=true",
		"objective_exact_token_hint",
		`exact token "jank_frames=7" (kind=kv)`,
		`pattern "jank_frames" does not include it`,
		`trace_query(view="event_search", pattern="jank_frames=7"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("broad event_search should warn for exact kv token, missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryLargeExplicitTimeWindowUsesWindowedIndex(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "windowed.systrace")
	trace := strings.Join([]string{
		`old-1 (1) [000] .... 1.000000: sched_wakeup: comm=old pid=1 prio=20 target_cpu=000`,
		`app-20 (20) [001] .... 2.000000: print: B|20|Choreographer#doFrame 1917295`,
		`app-20 (20) [001] .... 2.050000: print: E|20`,
		`new-3 (3) [000] .... 3.000000: sched_wakeup: comm=new pid=3 prio=20 target_cpu=000`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":      "path",
		"path":        "windowed.systrace",
		"view":        "recipe",
		"recipe_name": "jank",
		"time_start":  2.0,
		"time_end":    2.1,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"index_windowed=true",
		"windowed_index_parse=true",
		"parsed_events=2",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("windowed index summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryLargeEventSearchUsesStreamingScan(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "streamed.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [000] .... 9.000000: print: B|20|Choreographer#doFrame 173073`,
		`app-20 (20) [000] .... 9.001000: print: E|20`,
		`app-20 (20) [000] .... 9.010000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "streamed.systrace",
		"view":    "event_search",
		"pattern": "173073",
		"limit":   10,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"matched_events=1", "Choreographer#doFrame 173073", "streamed_event_search=true"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("streaming event_search summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryLargeFrameWindowPatternAutoNarrowsToWindow(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "frame_guard.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [000] .... 9.000000: print: B|20|Choreographer#doFrame 173073`,
		`app-20 (20) [000] .... 9.001000: print: E|20`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "frame_guard.systrace",
		"view":    "frame_window",
		"pattern": "173073",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Trace Query: frame_window",
		"index_windowed=true",
		"auto_window_from_pattern=true",
		"Choreographer#doFrame 173073",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("frame_window auto-window summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "mode=large_trace_heavy_view_guard") {
		t.Fatalf("frame_window pattern should auto-narrow before guard:\n%s", res.Summary)
	}
}

func TestTraceQueryLargeHeavyViewWithoutWindowUsesGuard(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "heavy_guard.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 2.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 2.050000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source": "path",
		"path":   "heavy_guard.systrace",
		"view":   "scheduler_latency_stats",
		"thread": "app",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"mode=large_trace_heavy_view_guard",
		"thread or pid alone",
		"window_carryover_hint",
		`trace_query(view="event_search"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("heavy view guard summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "parsed_events=") {
		t.Fatalf("guard should return before parsing the heavy trace:\n%s", res.Summary)
	}
	if res.Refinement == nil {
		t.Fatalf("heavy view guard should attach typed refinement")
	}
	refinement := types.NormalizeToolRefinementHint(*res.Refinement)
	if refinement.ReasonCode != "trace_query_heavy_view_requires_scope" || !refinement.ResultTruncated {
		t.Fatalf("unexpected heavy guard refinement: %+v", refinement)
	}
	if refinement.PreferredNextTool != "trace_query" ||
		refinement.PreferredParams["view"] != "event_search" ||
		refinement.PreferredParams["path"] != "heavy_guard.systrace" ||
		refinement.PreferredParams["thread"] != "app" ||
		refinement.PreferredParams["event_types"] != "trace_mark" ||
		refinement.PreferredParams["limit"] != "40" {
		t.Fatalf("heavy guard preferred params should describe a bounded trace_query narrowing call: %+v", refinement.PreferredParams)
	}
	if !sameStringSliceForTest(refinement.RequiredFields, []string{"pattern"}) {
		t.Fatalf("heavy guard should require a narrowing pattern, got %v", refinement.RequiredFields)
	}
}

func TestTraceQueryLargeUnboundedNewHeavyViewsUseGuard(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "new_heavy_guard.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 2.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 2.050000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=R+ ==> next_comm=worker next_pid=30 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	for _, view := range []string{"thread_timeline", "ipc_graph", "wakeup_chain", "interaction_stats"} {
		params, _ := json.Marshal(map[string]any{
			"source": "path",
			"path":   "new_heavy_guard.systrace",
			"view":   view,
			"pid":    20,
		})
		res, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Summary, "mode=large_trace_heavy_view_guard") {
			t.Fatalf("unbounded %s on a large trace must hit the heavy view guard:\n%s", view, res.Summary)
		}
		if strings.Contains(res.Summary, "parsed_events=") {
			t.Fatalf("guard should return before parsing the heavy trace for %s:\n%s", view, res.Summary)
		}
	}
}

func TestTraceQueryLargeNewHeavyViewsBoundedStillRun(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "new_heavy_bounded.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 2.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 2.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 2.050000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=worker next_pid=30 next_prio=20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	for _, view := range []string{"thread_timeline", "ipc_graph", "wakeup_chain", "interaction_stats"} {
		params, _ := json.Marshal(map[string]any{
			"source":     "path",
			"path":       "new_heavy_bounded.systrace",
			"view":       view,
			"pid":        20,
			"time_start": 1.0,
			"time_end":   3.0,
		})
		res, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Success {
			t.Fatalf("bounded %s must run: %s", view, res.Summary)
		}
		if strings.Contains(res.Summary, "mode=large_trace_heavy_view_guard") {
			t.Fatalf("bounded %s must not be guarded:\n%s", view, res.Summary)
		}
		if !strings.Contains(res.Summary, "# Trace Query: "+view) {
			t.Fatalf("bounded %s must produce its view result:\n%s", view, res.Summary)
		}
	}
}

func TestTraceQueryLargeNewHeavyViewWithSpanScopeSkipsGuard(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "new_heavy_span.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 2.000000: print: B|20|FrameJob`,
		`app-20 (20) [001] .... 2.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=worker next_pid=30 next_prio=20`,
		`app-20 (20) [001] .... 2.040000: print: E|20`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":    "path",
		"path":      "new_heavy_span.systrace",
		"view":      "wakeup_chain",
		"pid":       20,
		"span_name": "FrameJob",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("span-scoped wakeup_chain must run: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "mode=large_trace_heavy_view_guard") {
		t.Fatalf("span-scoped call is not genuinely unbounded and must not be guarded:\n%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "# Trace Query: wakeup_chain") {
		t.Fatalf("span-scoped wakeup_chain must produce its view result:\n%s", res.Summary)
	}
}

func TestTraceQuerySummaryReportsParseQualityCounters(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "quality.systrace")
	trace := strings.Join([]string{
		`junk line one with no trace format`,
		`junk line two with no trace format`,
		`junk line three with no trace format`,
		`app-20 (20) [000] .... 9.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [000] .... 9.010000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source": "path",
		"path":   "quality.systrace",
		"view":   "event_search",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"scanned_lines=5 parsed_events=2 unparsed_lines=3 parse_line_panics=0 clock_regressions=0",
		"3 of 5 scanned lines did not match any known trace format; coverage may be incomplete",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("parse-quality summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryLargeRecipeDiscoveryNoMarkerSurfacesTypedRefinement(t *testing.T) {
	oldThreshold := traceQueryLargeRecipeDiscoveryMinBytes
	traceQueryLargeRecipeDiscoveryMinBytes = 1
	defer func() { traceQueryLargeRecipeDiscoveryMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "recipe_discovery_empty.systrace")
	trace := strings.Join([]string{
		`worker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":      "path",
		"path":        "recipe_discovery_empty.systrace",
		"view":        "recipe",
		"recipe_name": "jank",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"mode=large_trace_recipe_discovery",
		"no_marker_advisory",
		"Provide pattern with one exact literal frame id/span label/marker token",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("recipe discovery summary missing %q:\n%s", want, res.Summary)
		}
	}
	if res.Refinement == nil {
		t.Fatalf("recipe discovery no-marker result should attach typed refinement")
	}
	refinement := types.NormalizeToolRefinementHint(*res.Refinement)
	if refinement.ReasonCode != "trace_query_recipe_discovery_needs_scope" {
		t.Fatalf("unexpected recipe discovery refinement: %+v", refinement)
	}
	if refinement.PreferredParams["view"] != "event_search" ||
		refinement.PreferredParams["path"] != "recipe_discovery_empty.systrace" ||
		refinement.PreferredParams["event_types"] != "trace_mark" ||
		refinement.PreferredParams["limit"] != "40" {
		t.Fatalf("recipe discovery refinement should point to a narrow event_search call: %+v", refinement.PreferredParams)
	}
	if !sameStringSliceForTest(refinement.RequiredFields, []string{"pattern"}) {
		t.Fatalf("recipe discovery should require a narrowing pattern, got %v", refinement.RequiredFields)
	}
}

func TestTraceQueryLargeUnboundedJankRecipeAutoAnalyzesTopMarker(t *testing.T) {
	oldRecipeThreshold := traceQueryLargeRecipeDiscoveryMinBytes
	oldWindowThreshold := traceQueryWindowedIndexMinBytes
	traceQueryLargeRecipeDiscoveryMinBytes = 1
	traceQueryWindowedIndexMinBytes = 1
	defer func() {
		traceQueryLargeRecipeDiscoveryMinBytes = oldRecipeThreshold
		traceQueryWindowedIndexMinBytes = oldWindowThreshold
	}()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "large.systrace")
	var lines []string
	for i := 0; i < 55; i++ {
		lines = append(lines, `app-20 (20) [000] .... 8.000000: print: C|20|jank_noise=1|1`)
	}
	lines = append(lines,
		`app-20 (20) [000] .... 9.000000: print: C|20|jank_frames=7|1`,
		`app-20 (20) [000] .... 9.001000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=worker next_pid=10 next_prio=20`,
	)
	trace := strings.Join(lines, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: dir,
		WorkDir:  dir,
		Mutable:  types.NewMutableState(`这个东湖trace, "jank_frames=7" 这一帧，丢帧的原因是什么？`),
	}
	params, _ := json.Marshal(map[string]any{
		"source":       "path",
		"path":         "large.systrace",
		"view":         "recipe",
		"recipe_name":  "jank",
		"platform":     "donghu",
		"trace_flavor": "harmony_hitrace",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query auto-window recipe failed: %s", res.Summary)
	}
	for _, want := range []string{
		"mode=large_trace_recipe_auto_windows",
		"# Trace Query: auto window candidates",
		"candidate_windows=1",
		"jank_frames=7",
		"primary=true",
		"index_windowed=true",
		"Root cause rank",
		"Window stats",
		"auto_window_candidate=true",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("auto-window recipe summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "large_trace_recipe_guard") {
		t.Fatalf("timestamped jank marker should auto-window before discovery guard:\n%s", res.Summary)
	}
}

func TestTraceQueryLargeSpanKeywordAutoAnalyzesFewWindows(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "span_multi.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [000] .... 5.000000: print: B|20|DecodeBitmap`,
		`app-20 (20) [000] .... 5.020000: print: E|20`,
		`app-20 (20) [000] .... 9.000000: print: B|20|DecodeBitmap`,
		`app-20 (20) [000] .... 9.030000: print: E|20`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":    "path",
		"path":      "span_multi.systrace",
		"view":      "span_window",
		"span_name": "DecodeBitmap",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query span auto-window failed: %s", res.Summary)
	}
	for _, want := range []string{
		"mode=large_trace_pattern_auto_windows",
		"candidate_windows=2",
		"DecodeBitmap",
		"index_windowed=true",
		"span app-20 \"DecodeBitmap\" 5.000000..5.020000",
		"span app-20 \"DecodeBitmap\" 9.000000..9.030000",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("span auto-window summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryLargeNewHeavyViewsWithPatternAutoWindow(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "new_heavy_pattern.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [000] .... 3.000000: print: B|20|TargetFrame`,
		`waker-10 (10) [000] .... 3.005000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 3.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`client-20 (20) [001] .... 3.012000: binder_transaction: transaction=42 dest_proc=100 dest_thread=101 reply=1 flags=0x0 code=0x3`,
		`binder:100_1-101 (100) [002] .... 3.014000: binder_transaction_received: transaction=42`,
		`app-20 (20) [001] .... 3.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`app-20 (20) [000] .... 3.030000: print: E|20`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	for _, view := range []string{"thread_timeline", "ipc_graph", "wakeup_chain", "interaction_stats"} {
		params, _ := json.Marshal(map[string]any{
			"source":  "path",
			"path":    "new_heavy_pattern.systrace",
			"view":    view,
			"pid":     20,
			"pattern": "TargetFrame",
		})
		res, err := (&TraceQuery{}).Execute(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Success {
			t.Fatalf("pattern-scoped %s must run: %s", view, res.Summary)
		}
		for _, want := range []string{
			"# Trace Query: " + view,
			"auto_window_from_pattern=true",
			"index_windowed=true",
		} {
			if !strings.Contains(res.Summary, want) {
				t.Fatalf("pattern-scoped %s summary missing %q:\n%s", view, want, res.Summary)
			}
		}
		if strings.Contains(res.Summary, "mode=large_trace_heavy_view_guard") {
			t.Fatalf("pattern-scoped %s must auto-window instead of heavy-guarding:\n%s", view, res.Summary)
		}
	}
}

func TestTraceQueryLargeNewHeavyViewsPatternMultiCandidateBounded(t *testing.T) {
	oldThreshold := traceQueryWindowedIndexMinBytes
	traceQueryWindowedIndexMinBytes = 1
	defer func() { traceQueryWindowedIndexMinBytes = oldThreshold }()

	dir := t.TempDir()
	tracePath := filepath.Join(dir, "new_heavy_pattern_multi.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [000] .... 3.000000: print: B|20|TargetFrame`,
		`waker-10 (10) [000] .... 3.005000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 3.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 3.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`app-20 (20) [000] .... 8.000000: print: B|20|TargetFrame`,
		`waker-10 (10) [000] .... 8.005000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 8.010000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
		`app-20 (20) [001] .... 8.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		"",
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":  "path",
		"path":    "new_heavy_pattern_multi.systrace",
		"view":    "wakeup_chain",
		"pid":     20,
		"pattern": "TargetFrame",
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("multi-candidate pattern-scoped wakeup_chain must run: %s", res.Summary)
	}
	for _, want := range []string{
		"mode=large_trace_pattern_auto_windows",
		"candidate_windows=2",
		"auto_window_candidate=true",
		"index_windowed=true",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("multi-candidate new-heavy auto-window summary missing %q:\n%s", want, res.Summary)
		}
	}
	if res.Refinement == nil {
		t.Fatalf("multi-candidate auto-window should attach typed refinement")
	}
	refinement := types.NormalizeToolRefinementHint(*res.Refinement)
	if refinement.ReasonCode != "trace_query_auto_window_candidate" || !refinement.ResultTruncated {
		t.Fatalf("unexpected auto-window refinement: %+v", refinement)
	}
	if refinement.PreferredParams["view"] != "wakeup_chain" ||
		refinement.PreferredParams["path"] != "new_heavy_pattern_multi.systrace" ||
		refinement.PreferredParams["pid"] != "20" ||
		refinement.PreferredParams["time_start"] != "2.750000" ||
		refinement.PreferredParams["time_end"] != "4.000000" {
		t.Fatalf("auto-window refinement should carry the first padded bounded candidate: %+v", refinement.PreferredParams)
	}
}

func TestTraceQueryIPCGraphSummary(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "ipc.systrace")
	trace := strings.Join([]string{
		`client-20 (20) [001] .... 3.010000: binder_transaction: transaction=42 dest_proc=100 dest_thread=101 reply=1 flags=0x0 code=0x3`,
		`binder:100_1-101 (100) [002] .... 3.012000: binder_transaction_received: transaction=42`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":     "path",
		"path":       "ipc.systrace",
		"view":       "ipc_graph",
		"pid":        20,
		"time_start": 3.0,
		"time_end":   3.02,
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"IPC graph", "transaction=42", "client-20", "binder:100_1-101", "send_line=1", "receive_line=2", "oneway=false", "sync_like=true", "blocking_candidate=true"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryAcceptsTimestampStringsAndAppliesTinyTolerance(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "time.systrace")
	trace := strings.Join([]string{
		`app-20 (20) [001] .... 1.010004: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.020000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":      "path",
		"path":        "time.systrace",
		"view":        "event_search",
		"time_start":  "1.01000s",
		"time_end":    "1.01000 秒",
		"event_types": []string{"sched_wakeup"},
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{"line=1", "time_start=1.010000", "time_end=1.010000", "normalized=1.010000", "query_tolerance_seconds"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryAcceptsStringifiedScalarAndEventTypeParams(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "stringy.systrace")
	trace := strings.Join([]string{
		`waker-10 (10) [000] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.010000: sched_switch: prev_comm=app prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`waker-10 (10) [000] .... 1.050000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001`,
		`app-20 (20) [001] .... 1.080000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params := json.RawMessage(`{
		"source": "path",
		"path": "stringy.systrace",
		"view": "event_search",
		"pid": "20",
		"time_start": "1.00000s",
		"time_end": "1.08000 秒",
		"line_start": "1",
		"line_end": "4",
		"event_types": "sched_wakeup, sched_switch",
		"max_depth": "4",
		"max_branches": "2",
		"min_duration_ms": "1ms",
		"include_window_stats": "true",
		"limit": "2"
	}`)
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("trace_query failed: %s", res.Summary)
	}
	for _, want := range []string{
		"line_start=1",
		"line_end=4",
		"line=1 ts=1.000000 type=sched_wakeup",
		"line=2 ts=1.010000 type=sched_switch",
		"query_tolerance_seconds",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
}

func TestTraceQueryNormalizesCustomerThreadSelectors(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "customer.systrace")
	trace := strings.Join([]string{
		`com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch: prev_comm=com.tencent.mm prev_pid=36379 prev_prio=53 prev_state=S ==> next_comm=[D]#worker next_pid=36625 next_prio=20`,
		`[GT]ColdPool#5-36624 (36379) [000] .... 2942.260210: sched_wakeup: comm=com.tencent.mm pid=36379 prio=53 target_cpu=004`,
		`com.tencent.mm-36379 (36379) [004] .... 2942.260220: sched_switch: prev_comm=JSAdBrandSer#4 prev_pid=37145 prev_prio=28 prev_state=R+ ==> next_comm=com.tencent.mm next_pid=36379 next_prio=53`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	for _, thread := range []string{
		"com.tencent.mm-36379",
		"com.tencent.mm 36379",
		"com.tencent.mm [36379]",
		"com.tencent.mm (36379)",
		"pid=36379",
		"36379",
	} {
		t.Run(thread, func(t *testing.T) {
			params, _ := json.Marshal(map[string]any{
				"source":      "path",
				"path":        "customer.systrace",
				"view":        "event_search",
				"thread":      thread,
				"time_start":  2942.124416,
				"time_end":    2942.260210,
				"event_types": []string{"sched_switch", "sched_wakeup"},
			})
			res, err := (&TraceQuery{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			if !res.Success {
				t.Fatalf("trace_query failed: %s", res.Summary)
			}
			for _, want := range []string{
				"matched_events=",
				"line=1 ts=2942.124416 type=sched_switch",
				"line=2 ts=2942.260210 type=sched_wakeup",
				"pid-bearing scheduler fields are used for matching",
			} {
				if !strings.Contains(res.Summary, want) {
					t.Fatalf("summary missing %q for thread %q:\n%s", want, thread, res.Summary)
				}
			}
		})
	}
}

func TestTraceQueryEmptyEventSearchGivesRecoveryHint(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "empty.systrace")
	trace := `com.tencent.mm-36379 (36379) [004] .... 2942.124416: sched_switch: prev_comm=com.tencent.mm prev_pid=36379 prev_prio=53 prev_state=S ==> next_comm=[D]#worker next_pid=36625 next_prio=20`
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir}
	params, _ := json.Marshal(map[string]any{
		"source":      "path",
		"path":        "empty.systrace",
		"view":        "event_search",
		"thread":      "com.tencent.mm [99999]",
		"time_start":  2942.124416,
		"time_end":    2942.260210,
		"event_types": []string{"sched_wakeup"},
	})
	res, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"matched_events=0",
		"thread selector normalized",
		"pid=99999",
		"next_call_hint=try trace_query(view=\"thread_timeline\", pid=99999",
		"not absence proof",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, res.Summary)
		}
	}
	if res.Refinement == nil {
		t.Fatalf("empty event_search should attach a typed refinement hint")
	}
	refinement := types.NormalizeToolRefinementHint(*res.Refinement)
	if refinement.ReasonCode != "trace_query_event_search_zero_match" || refinement.ResultTruncated {
		t.Fatalf("unexpected refinement: %+v", refinement)
	}
	if refinement.PreferredParams["view"] != "event_search" ||
		refinement.PreferredParams["path"] != "empty.systrace" ||
		refinement.PreferredParams["thread"] != "com.tencent.mm [99999]" ||
		refinement.PreferredParams["event_types"] != "sched_wakeup" {
		t.Fatalf("unexpected preferred params: %+v", refinement.PreferredParams)
	}
}

func TestTraceQueryCompactedResultSurfacesRefinement(t *testing.T) {
	refinement := traceQueryRefinement(tracequery.Result{
		View:       "root_cause_rank",
		SourcePath: "trace.systrace",
		Caveats:    []string{"root_cause_rank compacted from 20 to 5 candidate(s)"},
	}, tracequery.Query{
		View:      "root_cause_rank",
		PID:       123,
		TimeStart: 1.0,
		TimeEnd:   2.0,
		Limit:     5,
	}, traceQueryParams{
		Source: "path",
		Path:   "trace.systrace",
		View:   "root_cause_rank",
	}, "path")
	if refinement == nil {
		t.Fatalf("compacted trace result should attach a refinement")
	}
	got := types.NormalizeToolRefinementHint(*refinement)
	if got.ReasonCode != "trace_query_result_compacted" || !got.ResultTruncated {
		t.Fatalf("unexpected refinement: %+v", got)
	}
	for key, want := range map[string]string{
		"source":     "path",
		"path":       "trace.systrace",
		"view":       "root_cause_rank",
		"pid":        "123",
		"time_start": "1.000000",
		"time_end":   "2.000000",
		"limit":      "5",
	} {
		if value := got.PreferredParams[key]; value != want {
			t.Fatalf("preferred param %s=%q, want %q in %+v", key, value, want, got.PreferredParams)
		}
	}
}

func TestTraceSecondParsesMilliseconds(t *testing.T) {
	var holder struct {
		T TraceSecond `json:"t"`
	}
	if err := json.Unmarshal([]byte(`{"t":"1010ms"}`), &holder); err != nil {
		t.Fatal(err)
	}
	if math.Abs(holder.T.Seconds()-1.01) > 0.000000001 {
		t.Fatalf("expected milliseconds to normalize to seconds, got %.9f", holder.T.Seconds())
	}
	if holder.T.QueryToleranceSeconds() <= 0 || holder.T.QueryToleranceSeconds() > 0.0005 {
		t.Fatalf("unexpected tolerance: %.9f", holder.T.QueryToleranceSeconds())
	}
}

func TestTraceSecondParsesCompoundTimestampUnits(t *testing.T) {
	tests := map[string]float64{
		`"1s 501ms 565μs 915ns"`: 1.501565915,
		`"1s501ms565µs915ns"`:    1.501565915,
		`"3s 116ms"`:             3.116,
		`"2秒 3毫秒 4微秒 5纳秒"`:       2.003004005,
	}
	for raw, want := range tests {
		var holder struct {
			T TraceSecond `json:"t"`
		}
		if err := json.Unmarshal([]byte(`{"t":`+raw+`}`), &holder); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if math.Abs(holder.T.Seconds()-want) > 0.000000001 {
			t.Fatalf("%s normalized to %.12f, want %.12f", raw, holder.T.Seconds(), want)
		}
	}
}

func TestFlexTraceQueryDurationParsesUnitsToMilliseconds(t *testing.T) {
	for raw, want := range map[string]float64{
		`"1ms"`:   1,
		`"0.5s"`:  500,
		`"250us"`: 0.25,
		`2`:       2,
	} {
		var got FlexFloat
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if math.Abs(got.Float64()-want) > 0.000000001 {
			t.Fatalf("%s: got %.9f want %.9f", raw, got.Float64(), want)
		}
	}
}
