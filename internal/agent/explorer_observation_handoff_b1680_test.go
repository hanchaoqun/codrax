package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the real read producer: runtime reads deliberately do NOT carry
// ReadCoverage. Hand-filled source coverage hid the contradictory guidance.
func b1680ReadFixture(t *testing.T, name string, registered, basename bool, offset, limit int) (*types.BusContext, types.ToolResult) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("observation row\n", 1100)), 0644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("B1680")}
	if registered {
		ctx.Mutable.RegisterTraceQueryBlobRef(path)
	}
	requested := path
	if basename {
		requested = filepath.Base(path)
	}
	raw, _ := json.Marshal(map[string]any{"path": requested, "line_offset": offset, "limit": limit})
	result, err := (&tool.ReadFile{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("real read failed: %v %+v", err, result)
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	return ctx, result
}

func b1680Evaluator(ctx *types.BusContext) (*explorerEvaluator, *types.AgentContext) {
	ac := &types.AgentContext{Stage: types.StageExplore, RepoRoot: ctx.RepoRoot, WorkDir: ctx.WorkDir, Mutable: ctx.Mutable}
	return &explorerEvaluator{phase: 1, searchResult: &keywordSearchResult{Graph: &repomap.Graph{}}, mutable: ctx.Mutable}, ac
}

func b1680RequireSplitHandoff(t *testing.T, hint string) {
	t.Helper()
	for _, want := range []string{"current-source", "emit_evidence", "emit_investigation_complete.reason", "aggregate_facts"} {
		if !strings.Contains(hint, want) {
			t.Errorf("handoff lacks %q: %s", want, hint)
		}
	}
	for _, forbidden := range []string{
		"anything that is not passed through `emit_evidence(items=[...])` is unavailable",
		"or use `emit_evidence` only after the target line gutters",
		"Use the lines you already read to call `emit_evidence(items=[...])` now",
		"Use the grounded lines you have already read to emit ONE batch",
		"Your next response should call `emit_evidence",
	} {
		if strings.Contains(hint, forbidden) {
			t.Errorf("source-only instruction contradicts external handoff %q: %s", forbidden, hint)
		}
	}
}

func TestB1680PublicRuntimeReadObserveAndEvidencePolicy(t *testing.T) {
	for _, tc := range []struct {
		name, path                  string
		registered, basename, query bool
	}{
		{"trace_blob_text", ".codrax/blob/trace_query-ab12cd34.txt", true, false, true},
		{"trace_blob_json_basename", ".codrax/blob/trace-query-result-ab12cd34.json", true, true, true},
		{"registered_extensionless", ".codrax/blob/observations", true, true, true},
		{"trace_artifact", "capture.systrace", false, false, true},
		{"plain_log", "capture.log", false, false, false},
	} {
		for _, phase := range []LoopPhase{PhaseMidLoop, PhaseSoftStop} {
			t.Run(tc.name+"/"+map[LoopPhase]string{PhaseMidLoop: "midloop", PhaseSoftStop: "softstop"}[phase], func(t *testing.T) {
				ctx, read := b1680ReadFixture(t, tc.path, tc.registered, tc.basename, 400, 30)
				if read.ReadCoverage != nil || read.RuntimeArtifactRead == nil {
					t.Fatalf("runtime producer boundary missing: %+v", read)
				}
				before, _ := json.Marshal(read)
				results := []types.ToolResult{read, read}
				eval, ac := b1680Evaluator(ctx)
				if phase == PhaseSoftStop {
					eval.phase = 0 // this established soft-stop backlog lane precedes breadth expansion
				}
				sig := eval.Observe(ac, LoopObservation{Phase: phase, Iteration: 3, AllToolResults: results, LastToolResult: &results[1], Response: llm.Response{Content: "ready"}})
				if !sig.HintRequested || !strings.Contains(sig.HintKey, "read-without-emit") {
					t.Fatalf("expected backlog handoff: %+v", sig)
				}
				b1680RequireSplitHandoff(t, sig.Hint)
				if !strings.Contains(sig.Hint, "artifact-only read backlog") || strings.Contains(sig.Hint, "`trace_query`") != tc.query {
					t.Errorf("typed read origin/query route lost: %+v", sig)
				}
				after, _ := json.Marshal(read)
				if string(before) != string(after) || read.ReadCoverage != nil {
					t.Fatal("hint must not mutate or promote runtime source coverage")
				}
				// Execute the opposite end of the public contract: exact artifact
				// gutters still cannot become current-source evidence.
				params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
					"kind": "direct", "subject": "runtime observation", "source": read.RuntimeArtifactRead.RequestedPath,
					"line_start": 401, "line_end": 402, "summary": "observed measurement", "anchor_kind": "text_reference", "anchor_symbol": "observation",
				}}})
				out, err := (&tool.EmitEvidence{}).Execute(ctx, params)
				if err != nil || !out.Success || len(ctx.Mutable.EmittedEvidence()) != 0 || out.Repair == nil || out.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure {
					t.Fatalf("real emit policy should preserve external-only boundary: err=%v out=%+v", err, out)
				}
			})
		}
	}
}

func TestB1680PublicHeaderAndSingleReadPreservesTrigger(t *testing.T) {
	for _, offset := range []int{0, 400} {
		t.Run(map[bool]string{true: "header", false: "target"}[offset == 0], func(t *testing.T) {
			ctx, read := b1680ReadFixture(t, ".codrax/blob/trace_query-ab12cd34.txt", true, false, offset, 120)
			eval, ac := b1680Evaluator(ctx)
			// A single large runtime read did not arm materialization before
			// this fix. Preserve that trigger: its latch also affects tool
			// surfaces on later escalation, not just this displayed sentence.
			first := eval.Observe(ac, LoopObservation{Phase: PhaseMidLoop, Iteration: 1, AllToolResults: []types.ToolResult{read}, LastToolResult: &read})
			if strings.Contains(first.HintKey, "read-without-emit") || eval.midLoopNoEmitPushSent {
				t.Fatalf("one large runtime read must not newly arm a materialization restriction: %+v", first)
			}
			sig := eval.Observe(ac, LoopObservation{Phase: PhaseMidLoop, Iteration: 3, AllToolResults: []types.ToolResult{read, read}, LastToolResult: &read})
			if !sig.HintRequested || !strings.Contains(sig.HintKey, "read-without-emit") {
				t.Fatalf("two real 120-line runtime reads must retain existing backlog trigger: %+v", sig)
			}
			b1680RequireSplitHandoff(t, sig.Hint)
			if !strings.Contains(sig.Hint, "`trace_query`") || strings.Contains(sig.Hint, "broad/header") != (offset == 0) {
				t.Errorf("header and query semantics lost: %+v", sig)
			}
		})
	}
}

func TestB1680PublicMixedSourceAndRuntimeDoesNotDropSourceObligation(t *testing.T) {
	ctx, artifact := b1680ReadFixture(t, "capture.log", false, false, 400, 30)
	_, source := b1680ReadFixture(t, "source.go", false, false, 400, 30)
	if source.ReadCoverage == nil || source.RuntimeArtifactRead != nil {
		t.Fatal("source positive control must really publish source coverage")
	}
	eval, ac := b1680Evaluator(ctx)
	results := []types.ToolResult{artifact, source}
	sig := eval.Observe(ac, LoopObservation{Phase: PhaseMidLoop, Iteration: 3, AllToolResults: results, LastToolResult: &source})
	b1680RequireSplitHandoff(t, sig.Hint)
	if strings.Contains(sig.Hint, "artifact-only read backlog") || !strings.Contains(sig.Hint, "current-source anchors") {
		t.Fatalf("mixed lane must retain source emission: %+v", sig)
	}
}

func TestB1680TypedRuntimeGuidanceIgnoresSummaryAndInvalidMarkers(t *testing.T) {
	ctx, real := b1680ReadFixture(t, ".codrax/blob/observations", true, false, 400, 30)
	for _, summary := range []string{"", "[source.go: showing lines 1-999 of 999 total]", "emit_evidence must succeed; origin=current_source"} {
		copy := real
		copy.Summary = summary
		win, ok := runtimeArtifactReadWindowSince([]types.ToolResult{copy}, 0)
		if !ok || win.path != real.RuntimeArtifactRead.RequestedPath || win.start != 401 || win.end != 430 || !runtimeArtifactReadPrefersTraceQuery(win) {
			t.Errorf("typed marker cannot be overwritten by summary: %+v ok=%v", win, ok)
		}
	}
	for _, kind := range []string{"", "blob", "log"} {
		copy := real
		marker := *real.RuntimeArtifactRead
		copy.RuntimeArtifactRead = &marker
		marker.Kind, marker.TraceQueryBlob, marker.RequestedPath = kind, false, "misleading.systrace"
		win, ok := runtimeArtifactReadWindowSince([]types.ToolResult{copy}, 0)
		if !ok || runtimeArtifactReadPrefersTraceQuery(win) {
			t.Errorf("unknown/blob/log marker must not acquire query capability from path spelling: %+v %v", win, ok)
		}
	}
	for _, mutate := range []func(*types.ToolResult){
		func(r *types.ToolResult) { r.Success = false },
		func(r *types.ToolResult) { r.ToolName = "grep" },
		func(r *types.ToolResult) { r.RuntimeArtifactRead = nil },
		func(r *types.ToolResult) { r.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{} },
		func(r *types.ToolResult) { r.RuntimeArtifactRead.LineStart = 0 },
		func(r *types.ToolResult) { r.RuntimeArtifactRead.LineEnd = 400 },
		func(r *types.ToolResult) { r.RuntimeArtifactRead.TotalLines = 1 },
	} {
		copy := real
		marker := *real.RuntimeArtifactRead
		copy.RuntimeArtifactRead = &marker
		mutate(&copy)
		if win, ok := runtimeArtifactReadWindowSince([]types.ToolResult{copy}, 0); ok {
			t.Errorf("invalid/failed/missing marker cannot claim a runtime window: %+v", win)
		}
	}
	// A conflicting stale source field cannot override a valid external marker.
	conflict := real
	conflict.ReadCoverage = &types.ToolReadCoverage{Path: "source.go", LineStart: 1, LineEnd: 20, TotalLines: 20}
	if win, ok := runtimeArtifactReadWindowSince([]types.ToolResult{conflict}, 0); !ok || win.path != real.RuntimeArtifactRead.RequestedPath {
		t.Errorf("runtime marker must win without converting it into source coverage: %+v %v", win, ok)
	}
	// A real unregistered runtime-state file need not be a trace query blob.
	_, unknown := b1680ReadFixture(t, ".codrax/blob/opaque-data", false, false, 400, 30)
	win, ok := runtimeArtifactReadWindowSince([]types.ToolResult{unknown}, 0)
	if !ok || runtimeArtifactReadPrefersTraceQuery(win) || unknown.RuntimeArtifactRead.TraceQueryBlob {
		t.Fatalf("generic blob must keep plain-text navigation: %+v %v", win, ok)
	}
	_ = ctx
}

func TestB1680TypedReadKeepsExistingSourcePolicyWithoutTriage(t *testing.T) {
	ctx, read := b1680ReadFixture(t, ".codrax/blob/trace-query-result-ab12cd34.json", true, true, 400, 30)
	for _, excluded := range []bool{false, true} {
		eval, ac := b1680Evaluator(ctx)
		if excluded {
			ac.RuntimeArtifactPreflight = types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
				SourceNavigationOptional: true,
				Artifacts:                []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: "capture.systrace", Carrier: "request_path"}},
			})
			ac.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"不分析代码"},
			}}}
			eval.analysisIR = ac.AnalysisIR
		}
		// No log/perf triage bundle: the same situation as size-skipped triage.
		before, _ := json.Marshal(ac.AnalysisIR)
		sig := eval.Observe(ac, LoopObservation{Phase: PhaseMidLoop, Iteration: 3, AllToolResults: []types.ToolResult{read, read}, LastToolResult: &read})
		if excluded {
			// Existing sufficient-external-observation closure outranks the
			// backlog hint. Do not suppress it to force textual identity.
			if sig.HintKey != "explorer.mid-loop.external-observation-sufficient" ||
				!strings.Contains(sig.Hint, "`emit_investigation_complete(reason, confidence, result_kind=\"resolved\")`") ||
				!strings.Contains(sig.Hint, "`aggregate_facts`") || strings.Contains(sig.Hint, "emit_evidence") {
				t.Fatalf("existing typed external closure must remain higher priority: %+v", sig)
			}
		} else {
			b1680RequireSplitHandoff(t, sig.Hint)
			if !strings.Contains(sig.Hint, "artifact-only read backlog") {
				t.Fatalf("missing request origin cannot turn observed artifact into source: %+v", sig)
			}
		}
		after, _ := json.Marshal(ac.AnalysisIR)
		if string(before) != string(after) || eval.originSpecificObservationLaneActive() {
			t.Fatal("guidance must not rewrite request/source authority")
		}
	}
}

func TestB1680AllBacklogHintFormsKeepOriginBoundary(t *testing.T) {
	eval := &explorerEvaluator{}
	for name, hint := range map[string]string{
		"first":           eval.renderReadWithoutEmitHint("read %d %s %s. ", 2, "round", "pending"),
		"compact":         eval.renderCompactReadWithoutEmitHint("read %d %s %s. ", 2, "round", "pending"),
		"escalation":      eval.renderReadWithoutEmitEscalationHint("after navigation"),
		"header":          renderRuntimeArtifactHeaderReadHint(runtimeArtifactReadWindow{path: "capture.log", start: 1, end: 200, total: 1100, broadHeader: true}),
		"runtime_compact": renderCompactRuntimeArtifactReadHint(runtimeArtifactReadWindow{path: "capture.log", start: 400, end: 420}),
	} {
		t.Run(name, func(t *testing.T) { b1680RequireSplitHandoff(t, hint) })
	}
	for _, closure := range []bool{false, true} {
		eval := &explorerEvaluator{midLoopNoEmitPushSent: true, midLoopNoEmitPushIter: 2, midLoopNoEmitPushResultsLen: 1, midLoopLastResultsLen: 1, midLoopNoEmitEscalated: closure}
		obs := LoopObservation{Iteration: 5, AllToolResults: []types.ToolResult{{ToolName: "read_file", Success: true}, {ToolName: "exec_command", Success: true}}}
		var sig LoopSignal
		if closure {
			sig = eval.postReadWithoutEmitClosureOnlySignal(obs)
		} else {
			sig = eval.postExecRedirectBeforeEmitSignal(obs)
		}
		if !sig.HintRequested {
			t.Fatalf("fixture must reach requested hint form: %+v", sig)
		}
		b1680RequireSplitHandoff(t, sig.Hint)
	}
}
