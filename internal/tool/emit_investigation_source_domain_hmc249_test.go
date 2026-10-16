package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC249CitationRejectionNamesDeclaredSourceSeat(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	for _, required := range []bool{true, false} {
		t.Run(map[bool]string{true: "declared_source", false: "same_text_not_required"}[required], func(t *testing.T) {
			mut := types.NewMutableState("分配、释放及调用位置")
			bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{
				RequestModel: types.RequestModel{Intent: types.IntentExplain, RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
					IsDimensionedAnswer: true, Confidence: 1, Dimensions: []types.RequestedAnswerDimension{{Index: 3, Label: "调用栈/位置", Role: types.RequestedAnswerDimensionSourceLocation, SourceQuote: "分配、释放及调用位置", Required: required}},
				}}, AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: true, MinCitations: 3}},
			}}
			res, err := (&EmitInvestigationComplete{}).Execute(bus, json.RawMessage(`{"reason":"native observations are available","confidence":"high","result_kind":"resolved"}`))
			if err != nil || !res.Success {
				t.Fatalf("execute: %v %+v", err, res)
			}
			if mut.IsInvestigationComplete() || !strings.Contains(res.Summary, "citation preflight failed") || !strings.Contains(res.Summary, "≥3") {
				t.Fatalf("citation floor changed: %s", res.Summary)
			}
			if required {
				for _, want := range []string{"dimension index=3", "role=source_location", `source_quote="分配、释放及调用位置"`} {
					if !strings.Contains(res.Summary, want) {
						t.Errorf("rejection hides %q: %s", want, res.Summary)
					}
				}
			} else if strings.Contains(res.Summary, "dimension index=") {
				t.Fatalf("non-required text minted source obligation: %s", res.Summary)
			}
			if bus.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions[0].Role != types.RequestedAnswerDimensionSourceLocation {
				t.Fatal("diagnostic silently corrected typed role")
			}
		})
	}
}

func TestHMC249DeclaredSourceDiagnosticIsBoundedAndNonMutating(t *testing.T) {
	quote := "recorded callsite\n\"opaque quote\" " + strings.Repeat("位", 200)
	profile := &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1}
	for i := 1; i <= 8; i++ {
		profile.Dimensions = append(profile.Dimensions, types.RequestedAnswerDimension{Index: i, Required: true,
			Role: types.RequestedAnswerDimensionSourceLocation, SourceQuote: quote})
	}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RequestedAnswerDimensions: profile}},
		TurnRouteHint: types.TurnRouteHint{CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceRequired, RequiredOutcomes: types.TurnOutcomeSourceExplanation}}
	before, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	got := completionDeclaredSourceObligationDiagnostic(ctx)
	for _, want := range []string{"current_source_evidence_mode=required", "required_outcomes includes source_explanation", "5 additional required source_location dimension(s) not shown", `callsite\n\"opaque quote\"`, "…"} {
		if !strings.Contains(got, want) {
			t.Errorf("bounded diagnosis lost %q: %s", want, got)
		}
	}
	if strings.Count(got, "dimension index=") != 3 || strings.Contains(got, quote) {
		t.Fatal("unbounded raw quote/roster leaked")
	}
	after, _ := json.Marshal(ctx.AnalysisIR.RequestModel)
	if string(before) != string(after) {
		t.Fatal("diagnostic changed accepted obligations")
	}
	for i := range profile.Dimensions {
		profile.Dimensions[i].Role = types.RequestedAnswerDimensionObservedValue
	}
	ctx.TurnRouteHint = types.TurnRouteHint{Source: "artifact", CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
	if got := completionDeclaredSourceObligationDiagnostic(ctx); got != "" {
		t.Fatalf("observation text created source diagnosis: %s", got)
	}
	if got := completionDeclaredSourceObligationDiagnostic(nil); got != "" {
		t.Fatal(got)
	}
}

func TestHMC249PublicNativeAndMixedSourceCompletionRetainGates(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed=%t", mixed), func(t *testing.T) {
			mu := types.NewMutableState("native and current source")
			perf := &types.PerfBundle{Meta: types.PerfMeta{Source: "hitrace"}, Observations: []types.PerfObservation{{Kind: "trace_query", Subject: "thread-41", Summary: "native callsite", LineStart: 3}}}
			mu.SetPerfTrace(perf)
			rm := types.RequestModel{Intent: types.IntentRootCause, PerfTrace: perf}
			hint := types.TurnRouteHint{Route: "repo", Source: "artifact", NeedsRepoAccess: true, CurrentSourceEvidenceMode: types.TurnRouteCurrentSourceEvidenceOptional}
			if mixed {
				hint.Source, hint.CurrentSourceEvidenceMode = "mixed", types.TurnRouteCurrentSourceEvidenceRequired
				hint.RequiredOutcomes = types.TurnOutcomeMeasurement | types.TurnOutcomeSourceExplanation
				rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Confidence: 1,
					Dimensions: []types.RequestedAnswerDimension{{Index: 2, Required: true, Role: types.RequestedAnswerDimensionSourceLocation, SourceQuote: "current source location"}}}
			}
			bus := &types.BusContext{Mutable: mu, TurnRouteHint: hint, AnalysisIR: &types.AnalysisIR{RequestModel: rm,
				AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: true, MinCitations: 3}}}}
			res, err := (&EmitInvestigationComplete{}).Execute(bus, json.RawMessage(`{"reason":"recorded facts retained","confidence":"high","result_kind":"resolved"}`))
			if err != nil || !res.Success {
				t.Fatalf("public completion: %v %s", err, res.Summary)
			}
			if mixed {
				if mu.IsInvestigationComplete() || !strings.Contains(res.Summary, "dimension index=2 role=source_location") {
					t.Fatalf("independent source obligation was waived or hidden: %s", res.Summary)
				}
			} else if !mu.IsInvestigationComplete() {
				t.Fatalf("native-only completion lost existing gate behavior: %s", res.Summary)
			}
		})
	}
}
