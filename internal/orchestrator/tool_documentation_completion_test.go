package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/analysis/criterion"
	"github.com/hanchaoqun/codrax/internal/analysis/hdp"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func documentationContractBus(t *testing.T, accepted bool) *types.BusContext {
	t.Helper()
	rm := types.RequestModel{Intent: types.IntentExplain, Language: "en",
		ToolDocumentationRequest: &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestOnly}}
	compiled := compiler.Compile(rm, budget.BudgetSignals{})
	m := types.NewMutableState("Explain documented tool capabilities")
	m.SetRequestModel(rm)
	bus := &types.BusContext{Mutable: m, AnalysisIR: &types.AnalysisIR{
		RequestModel: rm, TaskGraph: compiled.TaskGraph, EvidencePlan: compiled.EvidencePlan,
		AnswerContract: compiled.AnswerContract, HypothesisSet: hdp.Plan(rm),
	}}
	if accepted {
		r, err := (&tool.TraceCapabilities{}).Execute(bus, json.RawMessage(`{"view":"window_stats","detail":true}`))
		if err != nil || !r.Success {
			t.Fatalf("read: %+v %v", r, err)
		}
		m.AppendDispatchToolResult(r)
		closed, err := (&tool.EmitInvestigationComplete{}).Execute(bus, json.RawMessage(`{"reason":"The complete window statistics contract was read, including units and missing-data boundaries.","confidence":"high","result_kind":"resolved"}`))
		if err != nil || !closed.Success || !m.HasAcceptedToolDocumentationCompletion(&rm) {
			t.Fatalf("complete: %+v %v", closed, err)
		}
		m.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{r, closed}})
	}
	return bus
}

func TestToolDocumentationFinalContractRequiresAcceptedCurrentRead(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_read", true: "accepted"}[accepted], func(t *testing.T) {
			bus := documentationContractBus(t, accepted)
			o := &Orchestrator{busCtx: bus}
			out := &agent.StageOutput{FinalAnswer: "Window statistics describe supported measurements; missing events do not establish a measured zero."}
			result := runContractCheck(out, bus.AnalysisIR.AnswerContract, bus.Mutable, o, contractCheckSkipLLMReview())
			var missingRead bool
			for _, v := range result.Violations {
				missingRead = missingRead || strings.Contains(v.ClusterKey, "acceptance:tool_documentation")
			}
			if missingRead == accepted {
				t.Fatalf("accepted=%v, result=%+v", accepted, result)
			}
			if accepted && !result.Passed {
				t.Fatalf("uncited static explanation failed final contract: %+v", result)
			}
			view := o.buildArtifactReadinessView(bus.AnalysisIR)
			env := criterion.Env{IR: bus.AnalysisIR, ArtifactReadiness: view}
			got := criterion.Eval(types.Criterion{Kind: types.CritToolDocumentationReady}, env)
			if got.Satisfied != accepted {
				t.Fatalf("existing scheduler adapter lost receipt: %+v", got)
			}
			if len(bus.Mutable.EmittedEvidence()) != 0 || bus.Mutable.TraceQueryRuntimeObservationCount() != 0 {
				t.Fatal("documentation became source/runtime evidence")
			}
		})
	}
}

func TestToolDocumentationSourceNamingOracleRequiresPureAcceptedScope(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		accepted, reset, source bool
	}{
		{"accepted_pure", true, false, false},
		{"unread", false, false, false},
		{"reset", true, true, false},
		{"source_obligation_added", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bus := documentationContractBus(t, tc.accepted)
			if tc.reset {
				bus.Mutable.ResetInvestigationComplete()
			}
			if tc.source {
				rm := *bus.Mutable.RequestModel()
				rm.UserPinnedFiles = []string{"src/main.go"}
				bus.Mutable.SetRequestModel(rm)
			}
			doc := sourceOracleScopeDocument(nil)
			denials := types.NewTypedDenialSet()
			vs := runV2BlockOraclesWithOracleContext(context.Background(), doc,
				&types.AnswerSemanticView{Family: types.QFEnumeration}, bus.Mutable, denialStubOracle{}, denials, bus)
			n := 0
			for _, v := range vs {
				if sourceOracleScopeViolation(v.Kind) {
					n++
				}
			}
			wantSource := !tc.accepted || tc.reset || tc.source
			if wantSource != (n > 0) {
				t.Fatalf("source applicability=%v, violations=%+v", wantSource, vs)
			}
		})
	}
}
