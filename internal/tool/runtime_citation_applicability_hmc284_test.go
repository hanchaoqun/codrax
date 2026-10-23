package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC284NativeRuntimeCompletionDoesNotMintSourceFloorFromNavigation(t *testing.T) {
	for _, readSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "negative_navigation", true: "incidental_source_read"}[readSource], func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			ctx.AnalysisIR.AnswerContract.CitationReq = types.CitationReq{Required: true, MinCitations: 3, Granularity: "file_line"}
			path := filepath.Join(ctx.RepoRoot, "helper.go")
			if err := os.WriteFile(path, []byte("package fixture\n// unrelated available source\n"), 0600); err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct {
				tool Tool
				args string
			}{
				{&ListFiles{}, `{"path":"."}`},
				{&GrepTool{}, `{"path":".","pattern":"hmc284_no_such_symbol","files_only":true}`},
			} {
				result, err := step.tool.Execute(ctx, json.RawMessage(step.args))
				if err != nil || !result.Success {
					t.Fatalf("public navigation failed: %+v %v", result, err)
				}
				ctx.Mutable.AppendDispatchToolResult(result)
			}
			if readSource {
				args, _ := json.Marshal(map[string]any{"path": path})
				result, err := (&ReadFile{}).Execute(ctx, args)
				if err != nil || !result.Success {
					t.Fatalf("public source read failed: %+v %v", result, err)
				}
				ctx.Mutable.AppendDispatchToolResult(result)
			}
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			authority := runtimeSourceAnswerAuthorityForCompletion(ctx)
			if authority.CurrentSourceRequired || !authority.CurrentSourceSatisfied || authority.AddressableDeterministicRuntimeQueryCount == 0 {
				t.Fatalf("fixture must reproduce satisfied but optional source with native runtime support: %+v", authority)
			}
			before, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			result, err := (&EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"Native observations answer the measured request in their own recorded scope","confidence":"high","result_kind":"resolved"}`))
			if err != nil || !result.Success || !ctx.Mutable.IsInvestigationComplete() || strings.Contains(result.Summary, "DOWNGRADED") {
				t.Fatalf("optional source navigation must not create a source citation obligation: %+v %v", result, err)
			}
			after, _ := json.Marshal(ctx.Mutable.DispatchToolResults())
			if string(before) != string(after) || ctx.Mutable.StableEvidenceFloorWaiver().IsActive() {
				t.Fatal("completion mutated original facts or required a model waiver")
			}
		})
	}
}

func TestHMC284NativeRuntimeCitationBypassKeepsIndependentSourceClaims(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*types.BusContext)
		facts []types.AnswerAggregateFact
	}{
		{"source_explanation", func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.CurrentSourceExplanationProfile = &types.CurrentSourceExplanationProfile{IsCurrentSourceExplanationRequested: true, SourceQuotes: []string{"explain current source"}}
		}, nil},
		{"source_location", func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Role: types.RequestedAnswerDimensionSourceLocation, Required: true, SourceQuote: "source location"}}}
		}, nil},
		{"current_status", func(ctx *types.BusContext) {
			ctx.AnalysisIR.AnswerContract.CurrentStatusDiagnostic = &types.CurrentStatusDiagnosticContract{Required: true}
		}, nil},
		{"exact_source", func(ctx *types.BusContext) {
			ctx.AnalysisIR.AnswerContract.ExactResolution = &types.ExactResolutionContract{TargetLabel: "support.go:12"}
		}, nil},
		{"change_impact", func(ctx *types.BusContext) {
			ctx.AnalysisIR.RequestModel.ChangeImpactProfile = &types.ChangeImpactProfile{IsChangeImpact: true, Target: "support.go"}
		}, nil},
		{"explicit_source_fact", nil, []types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar, Label: "source value", Value: "3", Provenance: string(types.AnswerEvidenceOriginCurrentSource)}}},
		{"failed_native_query", func(ctx *types.BusContext) {
			results := ctx.Mutable.DispatchToolResults()
			ctx.Mutable = types.NewMutableState("failed query cannot waive source floor")
			for _, result := range results {
				result.Success = false
				ctx.Mutable.AppendDispatchToolResult(result)
			}
		}, nil},
		{"model_only_runtime", func(ctx *types.BusContext) { ctx.Mutable = types.NewMutableState("model label is not query proof") }, []types.AnswerAggregateFact{{Kind: types.AnswerAggregateScalar, Label: "reported value", Value: "3", Provenance: string(types.AnswerEvidenceOriginRuntimeArtifact)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := nativeMeasurementHandoffContext(t)
			ctx.AnalysisIR.AnswerContract.CitationReq = types.CitationReq{Required: true, MinCitations: 3}
			if tc.edit != nil {
				tc.edit(ctx)
			}
			if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "support.go"), []byte("package fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := (&GrepTool{}).Execute(ctx, json.RawMessage(`{"path":".","pattern":"hmc284_absent","files_only":true}`))
			if err != nil || !result.Success {
				t.Fatalf("negative navigation failed: %+v %v", result, err)
			}
			ctx.Mutable.AppendDispatchToolResult(result)
			ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: ctx.Mutable.DispatchToolResults()})
			if label, ok := traceQueryRuntimeObservationCompletionBypassLabel(ctx, tc.facts); ok {
				t.Fatalf("independent source obligation or missing native proof was bypassed: %s", label)
			}
			params, _ := json.Marshal(map[string]any{"reason": "Candidate facts retained for normal validation", "confidence": "high", "result_kind": "resolved", "aggregate_facts": tc.facts})
			result, err = (&EmitInvestigationComplete{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			if ctx.Mutable.IsInvestigationComplete() {
				t.Fatalf("public completion lost the independent proof obligation: %+v", result)
			}
		})
	}
}
