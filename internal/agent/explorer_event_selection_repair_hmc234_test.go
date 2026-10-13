package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC234EventSelectionRepairPreservesPartialFactsWithoutCompletion(t *testing.T) {
	ctx, preview := b1699ActualQueryContext(t)
	const request = "List every event with at least two frames and count them."
	ctx.Objective = request
	ctx.AnalysisIR.RequestModel = types.RequestModel{RawRequest: request, Predicates: types.SemanticPredicates{IsCountQuestion: true}, RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet}}
	ctx.TurnRouteHint.CurrentSourceEvidenceMode = types.TurnRouteCurrentSourceEvidenceOptional
	ctx.Mutable = types.NewMutableState(request)
	ctx.Mutable.SetPerfTrace(&types.PerfBundle{Observations: []types.PerfObservation{
		{Subject: "preview event", Summary: "one recorded operation", LineStart: 2, LineEnd: 2},
		{Subject: "preview event", Summary: "another recorded operation", LineStart: 3, LineEnd: 3},
	}})
	args, _ := json.Marshal(map[string]any{"source": "path", "path": ctx.AttachedHitrace, "view": "event_search", "time_start": 7, "time_end": 7.1, "event_field_filters": []map[string]any{{"field": "jank_frames", "op": "gte", "value": "2"}}})
	bad, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || bad.Repair == nil {
		t.Fatalf("native query repair: %v %+v", err, bad)
	}
	ctx.Mutable.AppendDispatchToolResult(bad)
	ctx.Mutable.ResetDispatchToolResults() // Automatic fact retry is still the same investigation.
	e := &explorerEvaluator{}
	_ = e.BuildInitialInstruction(ctx, nil)
	e.midLoopCompletionReadySent = true // A later rejected call revokes an earlier hint.
	sig := e.postExternalObservationSufficiencySignal(LoopObservation{AllToolResults: []types.ToolResult{preview}})
	if !sig.HintRequested || sig.HintKey != "explorer.mid-loop.runtime-population-query-pending" || e.midLoopCompletionReadySent {
		t.Fatalf("preview armed premature completion: %+v", sig)
	}
	if readiness := e.completionReadiness([]types.ToolResult{preview}, 1, false, false); readiness.HasEnough {
		t.Fatalf("single-source readiness bypassed pending execution: %+v", readiness)
	}
	ctx.Mutable.SetInvestigationComplete("prior unrelated completion cannot replace execution")
	out, err := e.ParseOutput(ctx, nil, []types.ToolResult{preview}, nil)
	if err != nil || out == nil || out.SignalUpdates == nil || out.SignalUpdates.HasEnoughFacts {
		t.Fatalf("honest partial handoff must remain usable without complete signal: %v %+v", err, out)
	}
	if ta := ctx.Mutable.TurnAArtifacts(); ta == nil || len(ta.ToolResults) == 0 {
		t.Fatal("pending query erased independently available native facts")
	}
}
