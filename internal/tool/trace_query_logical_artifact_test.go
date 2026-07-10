package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryLogicalPerfArtifactIDAutoResolvesToSingleAttachedTrace(t *testing.T) {
	repo := t.TempDir()
	work := t.TempDir()
	trace := strings.Join([]string{
		`app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`worker-200 (200) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001`,
	}, "\n")
	attachedPath := filepath.Join(work, promptctx.AttachedTraceBlobName)
	if err := os.WriteFile(attachedPath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	mutable := types.NewMutableState("trace only")
	mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "systrace"}})
	ctx := &types.BusContext{
		RepoRoot:              repo,
		WorkDir:               work,
		Mutable:               mutable,
		AttachedHitraceSource: "harmony_hitrace",
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "(inline)", Carrier: "attachment",
			}},
		},
	}

	item := logicalArtifactSelectionItemBySource(t, ctx, "perf_trace:systrace")
	// This is the exact id copied by the 2026-07-10 false-green run.
	if item.ID != "runtime_artifact:6ec4e6d42cc3a0a1" {
		t.Fatalf("perf logical id drifted: got %q", item.ID)
	}
	adapted, adaptation, reject := traceQueryAdaptLogicalArtifactPath(ctx, traceQueryParams{Source: "path", Path: item.ID})
	if reject != nil || adaptation == nil {
		t.Fatalf("logical attachment adaptation missing: adapted=%+v adaptation=%+v reject=%+v", adapted, adaptation, reject)
	}
	if adapted.Source != "attached_trace" || adapted.Path != "" || adaptation.CanonicalSource != "attached_trace" {
		t.Fatalf("downstream params were not canonicalized for refinements: adapted=%+v adaptation=%+v", adapted, adaptation)
	}

	params, err := json.Marshal(map[string]any{
		"source":     "path",
		"path":       item.ID,
		"view":       "event_search",
		"pattern":    "sched_switch",
		"time_start": 5.0,
		"time_end":   5.01,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("logical typed attachment id should auto-resolve, got: %s", result.Summary)
	}
	for _, want := range []string{
		"source compatibility",
		"logical_id=" + item.ID,
		"auto_resolved=true",
		"resolved_source=attached_trace",
		`canonical_next_call=source="attached_trace" path=<omit>`,
		"source=attached_trace",
		"sched_switch",
	} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("adapted result missing %q:\n%s", want, result.Summary)
		}
	}
	if strings.Contains(result.Summary, "failed to parse") || strings.Contains(result.Summary, filepath.Join(repo, item.ID)) {
		t.Fatalf("logical id must never reach filesystem parsing:\n%s", result.Summary)
	}
}

func TestResolveTraceQuerySourceUnknownLogicalArtifactIDFailsClosed(t *testing.T) {
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, promptctx.AttachedTraceBlobName), []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot:              t.TempDir(),
		WorkDir:               work,
		AttachedHitraceSource: "harmony_hitrace",
	}
	unknown := "runtime_artifact:0000000000000000"
	path, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: unknown})
	if reject == nil {
		t.Fatal("unknown logical id must fail closed")
	}
	if path != "" {
		t.Fatalf("unknown logical id escaped to filesystem path %q", path)
	}
	if reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_unknown" {
		t.Fatalf("unknown-id typed repair missing: %+v", reject.Repair)
	}
	for _, want := range []string{"did not treat logical path", "current typed runtime-artifact selection", "source=\"attached_trace\""} {
		if !strings.Contains(reject.Summary+reject.Repair.Hint, want) {
			t.Fatalf("unknown-id rejection missing %q: %+v", want, reject)
		}
	}
}

func TestResolveTraceQuerySourceLogicalArtifactWrongKindAndUnresolvedFailClosed(t *testing.T) {
	t.Run("log item", func(t *testing.T) {
		ctx := &types.BusContext{AttachedLog: "panic"}
		item := logicalArtifactSelectionItemBySource(t, ctx, "attached_log")
		path, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: item.ID})
		if path != "" || reject == nil || reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_wrong_kind" {
			t.Fatalf("log logical id must fail with typed wrong-kind repair: path=%q reject=%+v", path, reject)
		}
		if !strings.Contains(reject.Summary, "kind=\"log\", not trace") {
			t.Fatalf("wrong-kind summary is not actionable: %s", reject.Summary)
		}
	})

	t.Run("trace alias without physical carrier", func(t *testing.T) {
		ctx := &types.BusContext{RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "typed_trace_alias", Carrier: "request_path",
			}},
		}}
		item := logicalArtifactSelectionItemBySource(t, ctx, "typed_trace_alias")
		path, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: item.ID})
		if path != "" || reject == nil || reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_unresolved" {
			t.Fatalf("unresolved logical trace id must fail closed: path=%q reject=%+v", path, reject)
		}
		if !strings.Contains(reject.Summary, "no stat-verified attached blob or trace path") {
			t.Fatalf("unresolved summary is not actionable: %s", reject.Summary)
		}
	})
}

func TestResolveTraceQuerySourceLogicalItemWithMultiplePhysicalMappingsRejects(t *testing.T) {
	repo := t.TempDir()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "capture.systrace"), []byte("repo capture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, promptctx.AttachedTraceBlobName), []byte("attached capture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot:              repo,
		WorkDir:               work,
		AttachedHitraceSource: "capture.systrace",
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "capture.systrace", Carrier: "request_path",
			}},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, "capture.systrace")

	path, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: item.ID})
	if reject == nil {
		t.Fatal("one logical item mapping to two physical files must fail closed")
	}
	if path != "" {
		t.Fatalf("ambiguous logical item escaped to path %q", path)
	}
	for _, want := range []string{"multiple physical trace files", "refuses to guess", `source="attached_trace"`, `source="path"`} {
		if !strings.Contains(reject.Summary, want) {
			t.Fatalf("ambiguous mapping rejection missing %q:\n%s", want, reject.Summary)
		}
	}
	if reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_ambiguous" {
		t.Fatalf("ambiguous mapping typed repair missing: %+v", reject.Repair)
	}
}

func TestResolveTraceQuerySourceLogicalPerfAliasWithMultiplePhysicalTracesRejects(t *testing.T) {
	repo := t.TempDir()
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "other.systrace"), []byte("other capture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, promptctx.AttachedTraceBlobName), []byte("attached capture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mutable := types.NewMutableState("trace only")
	mutable.SetPerfTrace(&types.PerfBundle{Meta: types.PerfMeta{Source: "systrace"}})
	ctx := &types.BusContext{
		RepoRoot:              repo,
		WorkDir:               work,
		Mutable:               mutable,
		AttachedHitraceSource: "harmony_hitrace",
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "(inline)", Carrier: "attachment"},
				{Kind: "trace", Source: "other.systrace", Carrier: "request_path"},
			},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, "perf_trace:systrace")

	path, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: item.ID})
	if reject == nil {
		t.Fatal("generic perf alias must not choose among multiple physical traces")
	}
	if path != "" {
		t.Fatalf("ambiguous perf alias escaped to path %q", path)
	}
	for _, want := range []string{"logical perf alias", "multiple physical trace files", "producer alias does not identify one capture", "refuses to guess"} {
		if !strings.Contains(reject.Summary, want) {
			t.Fatalf("multi-trace perf-alias rejection missing %q:\n%s", want, reject.Summary)
		}
	}
}

func TestResolveTraceQuerySourceLogicalFilesystemItemUsesTypedSource(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "customer.systrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "customer.systrace", Carrier: "request_path",
			}},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, "customer.systrace")

	gotPath, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: item.ID})
	if reject != nil {
		t.Fatalf("single stat-verified typed path should resolve: %s", reject.Summary)
	}
	if source != "path" || filepath.Clean(gotPath) != filepath.Clean(path) {
		t.Fatalf("resolved source=%q path=%q, want path %q", source, gotPath, path)
	}
}

func TestTraceQueryLogicalFilesystemItemReportsCanonicalNextCall(t *testing.T) {
	repo := t.TempDir()
	traceName := "customer.systrace"
	tracePath := filepath.Join(repo, traceName)
	trace := strings.Join([]string{
		`app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120`,
		`worker-200 (200) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001`,
	}, "\n")
	if err := os.WriteFile(tracePath, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		WorkDir:  t.TempDir(),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: traceName, Carrier: "request_path",
			}},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, traceName)
	params, err := json.Marshal(map[string]any{
		"source":     "path",
		"path":       item.ID,
		"view":       "event_search",
		"pattern":    "sched_switch",
		"time_start": 5.0,
		"time_end":   5.01,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatalf("logical filesystem item should auto-resolve: %s", result.Summary)
	}
	firstLine := strings.SplitN(result.Summary, "\n", 2)[0]
	for _, want := range []string{
		"source compatibility",
		"logical_id=" + item.ID,
		"auto_resolved=true",
		"resolved_source=path",
		`canonical_next_call=source="path" path="` + traceName + `"`,
	} {
		if !strings.Contains(firstLine, want) {
			t.Fatalf("filesystem compatibility feedback missing %q in first line:\n%s", want, result.Summary)
		}
	}
}

func TestResolveTraceQuerySourceDeletedLogicalFilesystemItemFailsClosed(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{
		RepoRoot: repo,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "deleted.systrace", Carrier: "request_path",
			}},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, "deleted.systrace")
	path, _, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "path", Path: item.ID})
	if path != "" || reject == nil || reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_unresolved" {
		t.Fatalf("deleted typed trace must fail closed before filesystem parsing: path=%q reject=%+v", path, reject)
	}
}

func TestResolveTraceQuerySourceLogicalIDWithUnknownSourceNeverReachesFilesystem(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{
		RepoRoot: repo,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "customer.systrace", Carrier: "request_path",
			}},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, "customer.systrace")
	// Even an attacker-created file with the logical token as its basename must
	// not turn an invalid source enum into a filesystem lane.
	if err := os.WriteFile(filepath.Join(repo, item.ID), []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "file", Path: item.ID})
	if path != "" || source != "file" || reject == nil || reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_invalid_source" {
		t.Fatalf("invalid source must reject logical id before filesystem resolution: path=%q source=%q reject=%+v", path, source, reject)
	}
	if !strings.Contains(reject.Summary, `source="file" is not a supported trace source`) {
		t.Fatalf("invalid-source repair is not actionable: %s", reject.Summary)
	}
}

func TestResolveTraceQuerySourceAttachedLogicalIDWithoutBlobNeverFallsBackToSameNamedFile(t *testing.T) {
	repo := t.TempDir()
	ctx := &types.BusContext{
		RepoRoot: repo,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind: "trace", Source: "missing.systrace", Carrier: "request_path",
			}},
		},
	}
	item := logicalArtifactSelectionItemBySource(t, ctx, "missing.systrace")
	if err := os.WriteFile(filepath.Join(repo, item.ID), []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, source, reject := resolveTraceQuerySource(ctx, traceQueryParams{Source: "attached_trace", Path: item.ID})
	if path != "" || source != "attached_trace" || reject == nil || reject.Repair == nil || reject.Repair.Code != "trace_query_runtime_artifact_id_unresolved" {
		t.Fatalf("attached logical id without a typed physical carrier must reject before same-named filesystem fallback: path=%q source=%q reject=%+v", path, source, reject)
	}
}

func logicalArtifactSelectionItemBySource(t *testing.T, ctx *types.BusContext, source string) types.RuntimeArtifactSelectionItem {
	t.Helper()
	view := traceQueryRuntimeArtifactSelectionView(ctx)
	for _, item := range view.Items {
		if item.Source == source {
			return item
		}
	}
	t.Fatalf("typed artifact source %q not found in %+v", source, view)
	return types.RuntimeArtifactSelectionItem{}
}
