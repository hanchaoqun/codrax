package tool

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
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

func TestTraceQueryExplicitUserRequestPlatformWinsWhenModelOmitsFlavor(t *testing.T) {
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
	for _, want := range []string{"trace_flavor=harmony_hitrace", "larger numeric value means higher priority", "trace flavor was selected from explicit user request"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("explicit user platform should win when model omits flavor, missing %q:\n%s", want, res.Summary)
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
		"source":      "path",
		"path":        "sample.systrace",
		"view":        "event_search",
		"time_start":  168758.662,
		"time_end":    168758.664,
		"event_types": []string{"sched_wakeup", "sched_switch"},
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

func TestTraceQuerySchemaDocumentsViews(t *testing.T) {
	body := (&TraceQuery{}).Description() + "\n" + string((&TraceQuery{}).Parameters())
	for _, want := range []string{"wakeup_chain", "thread_timeline", "window_stats", "scheduler_latency_stats", "critical_blocking_calls", "frame_window", "render_pipeline", "frame_timeline", "frame_flow", "recipe", "recipe_name", "ipc_graph", "event_search", "span_window", "root_cause_rank", "interaction_stats", "pattern", "not a regex", "span_name", "interaction_direction", "attached_trace", "trace_flavor", "android_atrace", "generic_ftrace", "seconds", "microsecond precision", "81774 us", "larger numeric priority", "1-40=CFS", "raw scheduler priority", "cpu_frequency", "cpu_frequency_limits", "clock_set_rate", "core_topology", "small=0-3", "block_bio_remap", "sched_blocked_reason", "binder_transaction_received", "binder_transaction_alloc_buf", "binder_lock", "softirq", "storage", "filesystem", "eBPF BIO", "PageFault", "Ability", "XPower", "HiSystemEvent", "ability_monitor", "xpower", "hi_sysevent", "power", "workqueue", "dma_fence", "鸿蒙", "东湖", "安卓"} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace_query schema/description missing %q:\n%s", want, body)
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
}

func TestTraceQueryLargeUnboundedJankRecipeUsesDiscoveryGuard(t *testing.T) {
	oldThreshold := traceQueryLargeRecipeDiscoveryMinBytes
	traceQueryLargeRecipeDiscoveryMinBytes = 1
	defer func() { traceQueryLargeRecipeDiscoveryMinBytes = oldThreshold }()

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
		t.Fatalf("trace_query discovery guard failed: %s", res.Summary)
	}
	for _, want := range []string{"mode=large_trace_recipe_discovery", "jank_frames=7", "primary=true", "next_call_hint", "line_start=1"} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("discovery summary missing %q:\n%s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "Root cause rank") || strings.Contains(res.Summary, "Window stats") {
		t.Fatalf("unbounded large jank discovery should not run heavy views:\n%s", res.Summary)
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
	for _, want := range []string{"IPC graph", "transaction=42", "client-20", "binder:100_1-101", "send_line=1", "receive_line=2"} {
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
