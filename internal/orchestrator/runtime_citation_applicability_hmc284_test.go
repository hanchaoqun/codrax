package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC284NativeRuntimeCitationFloorContractAndCriteriaAgree(t *testing.T) {
	for _, sourceRequired := range []bool{false, true} {
		t.Run(map[bool]string{false: "optional_after_negative_navigation", true: "independent_source_explanation"}[sourceRequired], func(t *testing.T) {
			contract := waiverSyncCitationFloorContract()
			ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("measured resource facts"),
				TurnRouteHint: types.TurnRouteHint{Source: "external_tool", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional},
				AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
					RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet},
				}, AnswerContract: contract}}
			tracePath, err := filepath.Abs(filepath.Join("..", "..", "eval", "fixtures", "hmosperf_io_request_latency_distribution", "events.systrace"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "available.go"), []byte("package fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			params, _ := json.Marshal(map[string]any{"source": "path", "path": tracePath, "view": "window_stats", "time_start": 1, "time_end": 14})
			for _, step := range []struct {
				tool tool.Tool
				args json.RawMessage
			}{
				{&tool.TraceQuery{}, params},
				{&tool.GrepTool{}, json.RawMessage(`{"path":".","pattern":"hmc284_absent","files_only":true}`)},
			} {
				result, err := step.tool.Execute(ctx, step.args)
				if err != nil || !result.Success {
					t.Fatalf("public evidence step failed: %+v %v", result, err)
				}
				ctx.Mutable.AppendDispatchToolResult(result)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			if sourceRequired {
				ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"explain current source"}}
			}
			before, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			o := &Orchestrator{busCtx: ctx}
			result := runContractCheck(&agent.StageOutput{FinalAnswer: "Measured values retain their native query provenance."}, contract, ctx.Mutable, o)
			if got := len(citationFloorViolations(result.Violations)) == 0; got == sourceRequired {
				t.Fatalf("citation floor applicability mismatch (sourceRequired=%v): %+v", sourceRequired, result.Violations)
			}
			for _, kind := range []string{types.CritCitationCountGE, types.CritContractSatisfied} {
				if got := finalizeCitationFloorSuccessCriterionWaived(kind, ctx.Mutable, o); got == sourceRequired {
					t.Fatalf("criterion %s disagrees with citation applicability: waived=%v sourceRequired=%v", kind, got, sourceRequired)
				}
			}
			if finalizeCitationFloorSuccessCriterionWaived(types.CritEvidenceCount, ctx.Mutable, o) {
				t.Fatal("unrelated criterion waived")
			}
			if !sourceRequired {
				currentContract := contract
				currentContract.CurrentStatusDiagnostic = &types.CurrentStatusDiagnosticContract{Required: true}
				checked := runContractCheck(&agent.StageOutput{FinalAnswer: "Native facts do not verify current source status."}, currentContract, ctx.Mutable, o)
				if len(citationFloorViolations(checked.Violations)) == 0 {
					t.Fatal("the actual checked contract must not be shadowed by an older optional-source bus contract")
				}
			}
			after, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			if string(before) != string(after) || ctx.Mutable.StableEvidenceFloorWaiver().IsActive() {
				t.Fatal("final contract changed original facts or minted a waiver")
			}
		})
	}
}
