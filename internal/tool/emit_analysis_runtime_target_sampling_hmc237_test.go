package tool

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTargetProvenanceRepairToSamplingPublicHMC237(t *testing.T) {
	const request = "请看附件中1到2秒文档工作线程（tid 11）的采样调用路径。"
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState(request)}
	// Rejecting a new model-owned object must not partially replace an existing
	// request. The successful repair below separately checks its scope/breadth.
	ctx.Mutable.SetRequestModel(types.RequestModel{RawRequest: request, RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 77, Source: "user_explicit"}}})
	old, _ := json.Marshal(ctx.Mutable.RequestModel())
	payload := b1793TargetRosterPayload(t, true)
	payload["runtime_artifact_scope_profile"] = map[string]any{"requested_scope": "explicit_time_window", "time_start": 1, "time_end": 2, "source_quote": "1到2秒", "confidence": .95}
	payload["runtime_question_profile"] = map[string]any{"scope": "bounded_fact_set", "runtime_work_relation_requested": false, "frame_causality_requested": false, "fact_families": []string{"count_or_duration"}, "confidence": .95}
	payload["runtime_target_profile"] = map[string]any{"declaration": "named_target", "source_quote": "tid 11", "confidence": .95}
	row := runtimeTargetProvenanceRowHMC237("thread", 10, "document-worker")
	payload["runtime_targets"] = []any{row}
	payload["runtime_thread_lookups"] = []types.RuntimeThreadLookup{{PID: 11, Thread: "document-worker", SourceQuote: "（tid 11）"}}
	emit := func() types.ToolResult {
		data, _ := json.Marshal(payload)
		before := append([]byte(nil), data...)
		out, err := (&EmitAnalysis{}).Execute(ctx, data)
		if err != nil || !bytes.Equal(before, data) {
			t.Fatalf("public emit error or payload mutation: %v", err)
		}
		return out
	}
	if bad := emit(); bad.Success {
		t.Fatal("unproven PID was accepted")
	}
	after, _ := json.Marshal(ctx.Mutable.RequestModel())
	if !bytes.Equal(old, after) {
		t.Fatal("failed identity validation polluted the previous request")
	}
	row["pid"] = 11
	if repaired := emit(); !repaired.Success {
		t.Fatalf("same-request model-owned repair failed: %s", repaired.Summary)
	}
	rm := ctx.Mutable.RequestModel()
	start, end, scoped := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow()
	if !scoped || start != 1 || end != 2 || !rm.RuntimeQuestionProfile.BoundedFactSet() {
		t.Fatal("identity repair changed independent scope/answer breadth")
	}
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: *rm}
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	// No explicit pid or thread: exercise actual request-model supplementation,
	// not a private helper or an explicitly correct selector bypass.
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 1, "time_end": 2})
	result, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("sampling inheritance failed: %v %+v", err, result)
	}
	decoded := 0
	for _, record := range result.Observations {
		if record.Predicate != TracePerfSamplingTreePredicate {
			continue
		}
		p, ok := DecodeTracePerfSamplingTree(record)
		if !ok || p.Status != "available" || p.TargetPID != 11 || p.TargetThread != "document-worker" || p.TargetScope != "thread" || p.SourcePath != path || p.Window.StartTs != 1 || p.Window.EndTs != 2 || p.Window.EndInclusive || p.SampleCount != 5 || p.CohortCount != 3 {
			t.Fatalf("public native handoff changed identity/window/population: %+v decoded=%t", p, ok)
		}
		text := TracePerfSamplingTreeText(p)
		for _, want := range []string{
			`producer="raw_perfdata_fallback" event="cycles"`, `producer="raw_perfdata_fallback" event="instructions"`, `producer="simpleperf_report_sample" event="cycles"`,
			"total_weight=35 stack_weight=30 unavailable_weight=5", "total_weight=7 stack_weight=7 unavailable_weight=0", "total_weight=100 stack_weight=100 unavailable_weight=0",
			`symbol="sys;root" dso="/system/lib.so" ip="0x1" inclusive=30 self=0`,
			`symbol="work" dso="/app/A.so" ip="0x2" inclusive=10 self=10`, `symbol="work" dso="/app/B.so" ip="0x3" inclusive=20 self=20`,
			`unavailable_stacks=1 reasons={"legacy_frame_boundaries_unavailable":1}`,
		} {
			if !strings.Contains(text, want) {
				t.Errorf("native handoff lacks %q: %s", want, text)
			}
		}
		if strings.Contains(text, "inclusive=900") || record.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatal("out-of-window sample or causal authority leaked")
		}
		decoded++
	}
	if decoded != 1 {
		t.Fatalf("native sampling observations=%d want1", decoded)
	}
	if decided, allowed := types.RuntimeTraceReportShapeAuthority(rm); !decided || allowed {
		t.Fatal("bounded sampling acquired causal report authority")
	}
	// Correct explicit tool selection remains a navigation cursor, not a way
	// to overwrite or silently correct the user's separately anchored subject.
	p, _ := traceQueryApplyRequestModelTarget(ctx, traceQueryParams{View: "perf_sampling_tree", PID: 12, Thread: "peer"})
	if p.PID.Int() != 12 || p.Thread != "peer" || ctx.Mutable.RequestModel().RuntimeTargets[0].PID != 11 {
		t.Fatal("explicit query selector or anchored subject overwritten")
	}
}
