package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1686InitialTraceTeachingMatchesActualExternalReadAndEmit(t *testing.T) {
	for _, fixture := range []struct {
		name, path string
		registered bool
	}{
		{"raw_trace", ".codrax/blob/capture/attached_trace.txt", false},
		{"query_result", ".codrax/blob/trace-query-result-ab12cd34.json", true},
	} {
		for _, causal := range []bool{false, true} {
			for _, language := range []string{"zh", "en"} {
				t.Run(fmt.Sprintf("%s/causal=%t/%s", fixture.name, causal, language), func(t *testing.T) {
					bus, read := b1680ReadFixture(t, fixture.path, fixture.registered, false, 400, 30)
					if read.ReadCoverage != nil || read.RuntimeArtifactRead == nil {
						t.Fatalf("actual artifact read lost external origin: %+v", read)
					}
					params, _ := json.Marshal(map[string]any{"items": []map[string]any{{
						"scope": "line", "evidence_kind": "direct", "subject": "observation",
						"source": read.RuntimeArtifactRead.RequestedPath, "line_start": 401,
						"anchor_kind": "text_reference", "anchor_symbol": "observation", "summary": "Observed trace row.",
					}}})
					result, err := (&tool.EmitEvidence{}).Execute(bus, params)
					if err != nil || !result.Success || result.Repair == nil || result.Repair.Code != types.ToolRepairCodeEvidenceExternalObservationToClosure || len(bus.Mutable.EmittedEvidence()) != 0 {
						t.Fatalf("actual emit must retain external-only handoff: %v %+v", err, result)
					}
					ctx := blobEscapeObservationOnlyContext(t, bus.Mutable)
					ctx.RepoRoot, ctx.WorkDir, ctx.Language = bus.RepoRoot, bus.WorkDir, language
					ctx.AttachedHitrace = read.RuntimeArtifactRead.RequestedPath
					ctx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{}
					if !causal {
						ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}
					}
					before, _ := json.Marshal(ctx.AnalysisIR)
					dispatchBefore, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
					eval := &explorerEvaluator{mutable: ctx.Mutable, analysisIR: ctx.AnalysisIR}
					schemas := []llm.ToolSchema{{Name: "read_file"}, {Name: "trace_query"}, {Name: "emit_evidence"}, {Name: "emit_investigation_complete"}}
					// Initial routing changes the existing breadth phase to trace;
					// compare permissions only after that established transition.
					prompt := eval.BuildInitialInstruction(ctx, nil)
					permissions := eval.FilterToolSchemas(ctx, schemas)
					if !strings.Contains(prompt, "Explicit Runtime Trace Path Start") || !strings.Contains(prompt, "Start with `trace_query`") || eval.phase != 1 {
						t.Fatal("public initial trace routing premise absent")
					}
					if strings.Contains(prompt, "Use `emit_evidence` only for load-bearing trace line gutters") {
						t.Error("initial teaching directs manually read external trace rows into a source emitter that rejects them")
					}
					if ctx.AnalysisIR.RequestModel.CurrentSourceLaneDecision() != types.CurrentSourceLaneExcluded {
						t.Fatal("fixture must retain typed source exclusion")
					}
					if strings.Count(prompt, explorerExternalObservationHandoffGuidance()) != 1 {
						t.Error("initial trace teaching must retain exactly one shared external-observation handoff, including reason and aggregate_facts")
					}
					if !strings.Contains(prompt, "`emit_investigation_complete(reason, confidence, result_kind)`") ||
						!strings.Contains(prompt, "Current-source inspection is excluded") ||
						strings.Contains(prompt, "Emit one `emit_evidence(items=[...])` batch only for real current-source anchors") {
						t.Error("source-excluded teaching must preserve closure fields without suggesting current-source emission")
					}
					if strings.Contains(prompt, "first establish the target thread's state priority") != causal {
						t.Fatal("handoff teaching changed requested fact/causal scope")
					}
					_ = eval.BuildInitialInstruction(ctx, nil)
					after, _ := json.Marshal(ctx.AnalysisIR)
					dispatchAfter, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
					if string(before) != string(after) || string(dispatchBefore) != string(dispatchAfter) || !reflect.DeepEqual(permissions, eval.FilterToolSchemas(ctx, schemas)) || ctx.Mutable.IsInvestigationComplete() || len(ctx.Mutable.EmittedEvidence()) != 0 {
						t.Fatal("teaching changed evidence, current request, completion or tool authority")
					}
				})
			}
		}
	}
}
