package orchestrator

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteAnalysisIRQualityRejectionRejectsUngroundedExactContract(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Array([]) fails while Matrix([]) works",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "shape",
			Kind:     types.WriteBehaviorInvariant,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "Array([]).shape",
			Operator: types.WriteBehaviorOpEquals,
			Expected: "()",
			Required: true,
		}},
	}}
	got := writeAnalysisIRQualityRejection(ir)
	if !strings.Contains(got, "behavior_contracts[0]") || !strings.Contains(got, "no grounded comparator") {
		t.Fatalf("expected ungrounded exact contract rejection, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionAcceptsComparatorGrounding(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Array([]) fails while Matrix([]) works",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "shape",
			Kind:     types.WriteBehaviorInvariant,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "Array([]).shape",
			Operator: types.WriteBehaviorOpEquals,
			Expected: "(0,)",
			Comparator: &types.WriteBehaviorComparator{
				Subject:     "Matrix([])",
				Relation:    types.WriteBehaviorComparatorRegressionBaseline,
				EvidenceRef: "issue:matrix-empty-baseline",
			},
			Required: true,
		}},
	}}
	if got := writeAnalysisIRQualityRejection(ir); got != "" {
		t.Fatalf("comparator-grounded exact contract should pass, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionRejectsSubjectOnlyComparator(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Array([]) fails while Matrix([]) works",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "shape",
			Kind:     types.WriteBehaviorInvariant,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "Array([]).shape",
			Operator: types.WriteBehaviorOpEquals,
			Expected: "(0,)",
			Comparator: &types.WriteBehaviorComparator{
				Subject:  "Matrix([])",
				Relation: types.WriteBehaviorComparatorRegressionBaseline,
			},
			Required: true,
		}},
	}}
	got := writeAnalysisIRQualityRejection(ir)
	if !strings.Contains(got, "no grounded comparator") {
		t.Fatalf("expected subject-only comparator rejection, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionRejectsUngroundedComparator(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Array([]) fails while Matrix([]) works",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "shape",
			Kind:     types.WriteBehaviorInvariant,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "Array([]).shape",
			Operator: types.WriteBehaviorOpEquals,
			Expected: "()",
			Comparator: &types.WriteBehaviorComparator{
				Subject:  "empty Array shape",
				Expected: "()",
				Relation: types.WriteBehaviorComparatorRegressionBaseline,
			},
			Required: true,
		}},
	}}
	got := writeAnalysisIRQualityRejection(ir)
	if !strings.Contains(got, "no grounded comparator") {
		t.Fatalf("expected fake comparator rejection, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionRejectsUngroundedNotRaisesPayload(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Array([]) raises ValueError",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "array-empty-no-raise",
			Kind:     types.WriteBehaviorException,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "Array([])",
			Operator: types.WriteBehaviorOpNotRaises,
			Expected: "does not raise and returns shape=()",
			Required: true,
		}},
	}}
	got := writeAnalysisIRQualityRejection(ir)
	if !strings.Contains(got, "array-empty-no-raise") {
		t.Fatalf("expected ungrounded not_raises payload rejection, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionAcceptsGroundedNotRaisesPayload(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Array([]) raises ValueError",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "array-empty-no-valueerror",
			Kind:     types.WriteBehaviorException,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "Array([])",
			Operator: types.WriteBehaviorOpNotRaises,
			Expected: "ValueError",
			Required: true,
		}},
	}}
	if got := writeAnalysisIRQualityRejection(ir); got != "" {
		t.Fatalf("grounded not_raises exception should pass, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionAcceptsGroundedRenderedTextPlacement(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Rendered line should show rainfall, in mm before float32",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "repr-unit-placement",
			Kind:     types.WriteBehaviorStdout,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "DataArray repr line",
			Operator: types.WriteBehaviorOpContains,
			Expected: ", in mm",
			Placement: &types.WriteRenderedTextPlacement{
				Surface:   types.WriteRenderedTextSurfaceRepr,
				Anchor:    "rainfall",
				Expected:  ", in mm",
				Relation:  types.WriteRenderedTextBetweenAnchorAndDelimiter,
				Delimiter: "float32",
			},
			Required: true,
		}},
	}}
	if got := writeAnalysisIRQualityRejection(ir); got != "" {
		t.Fatalf("grounded rendered-text placement should pass, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionRejectsIncompleteRenderedTextPlacement(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Rendered line should show rainfall, in mm before float32",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "repr-unit-placement",
			Kind:     types.WriteBehaviorStdout,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "DataArray repr line",
			Operator: types.WriteBehaviorOpContains,
			Expected: ", in mm",
			Placement: &types.WriteRenderedTextPlacement{
				Surface:  types.WriteRenderedTextSurfaceRepr,
				Expected: ", in mm",
				Relation: types.WriteRenderedTextBetweenAnchorAndDelimiter,
			},
			Required: true,
		}},
	}}
	got := writeAnalysisIRQualityRejection(ir)
	if !strings.Contains(got, "placement.anchor") || !strings.Contains(got, "placement.delimiter") {
		t.Fatalf("expected missing placement field rejection, got %q", got)
	}
}

func TestWriteAnalysisIRQualityRejectionRejectsUngroundedRenderedTextPlacement(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Rendered line should show the units before the dtype",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "repr-unit-placement",
			Kind:     types.WriteBehaviorStdout,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "DataArray repr line",
			Operator: types.WriteBehaviorOpContains,
			Expected: ", in mm",
			Placement: &types.WriteRenderedTextPlacement{
				Surface:   types.WriteRenderedTextSurfaceRepr,
				Anchor:    "rainfall",
				Expected:  ", in mm",
				Relation:  types.WriteRenderedTextBetweenAnchorAndDelimiter,
				Delimiter: "float32",
			},
			Required: true,
		}},
	}}
	got := writeAnalysisIRQualityRejection(ir)
	if !strings.Contains(got, "rendered-text placement") || !strings.Contains(got, "not both present") {
		t.Fatalf("expected ungrounded placement rejection, got %q", got)
	}

	ir.Request.BehaviorContracts[0].Placement.EvidenceRef = "file:xarray/core/formatting.py:42"
	if got := writeAnalysisIRQualityRejection(ir); got != "" {
		t.Fatalf("placement evidence_ref should ground the typed placement, got %q", got)
	}
}

func TestRepairWriteAnalysisIRQualityCalibratesOnlyUngroundedExactAuthority(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest:       "Desired output contains rainfall, in mm (time, y, x) float32 ... and Array([]) raises ValueError",
		Task:             types.WriteTask{Kind: types.WriteTaskFeature, Scope: types.ScopePackage, Summary: "show units"},
		ScopeAnchors:     []string{"xarray/core/formatting.py"},
		ExpectedOutcomes: []string{"units are visible"},
		BehaviorContracts: []types.WriteBehaviorContract{
			{
				ID:       "grounded-output",
				Kind:     types.WriteBehaviorStdout,
				Polarity: types.WriteBehaviorPolarityExpected,
				Subject:  "Dataset.__repr__",
				Operator: types.WriteBehaviorOpContains,
				Expected: "rainfall, in mm (time, y, x) float32 ...",
				Required: true,
				Source:   "write_analyzer",
			},
			{
				ID:       "ungrounded-no-raise",
				Kind:     types.WriteBehaviorException,
				Polarity: types.WriteBehaviorPolarityExpected,
				Subject:  "Dataset.__repr__ without units",
				Operator: types.WriteBehaviorOpNotRaises,
				Expected: "no new exception thrown when units attr is absent",
				Required: true,
				Source:   "write_analyzer",
			},
		},
	}}

	repaired, repairs := repairWriteAnalysisIRQuality(ir)

	if len(repairs) != 1 || !strings.Contains(repairs[0], "ungrounded-no-raise") {
		t.Fatalf("expected only planning-only authority calibration, got %+v", repairs)
	}
	if got := writeAnalysisIRQualityRejection(repaired); got != "" {
		t.Fatalf("repaired IR should satisfy quality gate, got %q", got)
	}
	if repaired.Request.Task.Summary != "show units" || len(repaired.Request.ScopeAnchors) != 1 || len(repaired.Request.ExpectedOutcomes) != 1 {
		t.Fatalf("repair should preserve useful IR fields: %+v", repaired.Request)
	}
	if repaired.Request.BehaviorContracts[0].Operator != types.WriteBehaviorOpContains {
		t.Fatalf("grounded exact contract should remain hard, got %+v", repaired.Request.BehaviorContracts[0])
	}
	soft := repaired.Request.BehaviorContracts[1]
	if soft.Operator != types.WriteBehaviorOpNotRaises {
		t.Fatalf("planning guidance must retain its original negative operator, got %+v", soft)
	}
	if soft.Required || !types.IsPlanningOnlyWriteBehaviorContract(soft) {
		t.Fatalf("ungrounded analyzer example must remain planning guidance, not a verifier target: %+v", soft)
	}
	if ir.Request.BehaviorContracts[1].Operator != types.WriteBehaviorOpNotRaises {
		t.Fatalf("repair should not mutate original IR, got %+v", ir.Request.BehaviorContracts[1])
	}
}

func TestRepairWriteAnalysisIRQualitySoftensInvalidPlacementContract(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "Rendered line should show rainfall, in mm before float32",
		BehaviorContracts: []types.WriteBehaviorContract{{
			ID:       "repr-unit-placement",
			Kind:     types.WriteBehaviorStdout,
			Polarity: types.WriteBehaviorPolarityExpected,
			Subject:  "DataArray repr line",
			Operator: types.WriteBehaviorOpContains,
			Expected: ", in mm",
			Placement: &types.WriteRenderedTextPlacement{
				Surface:  types.WriteRenderedTextSurfaceRepr,
				Expected: ", in mm",
				Relation: types.WriteRenderedTextBetweenAnchorAndDelimiter,
			},
			Required: true,
		}},
	}}

	repaired, repairs := repairWriteAnalysisIRQuality(ir)

	if len(repairs) != 1 || !strings.Contains(repairs[0], "authority=planning_only") {
		t.Fatalf("expected invalid placement repair, got %+v", repairs)
	}
	got := repaired.Request.BehaviorContracts[0]
	if got.Placement == nil || *got.Placement != *ir.Request.BehaviorContracts[0].Placement {
		t.Fatalf("partial placement must retain its original local meaning: %+v", got.Placement)
	}
	if got.Operator != types.WriteBehaviorOpContains || got.Expected != ", in mm" {
		t.Fatalf("authority calibration must not rewrite the proposed operator/expected value: %+v", got)
	}
	if !types.IsPlanningOnlyWriteBehaviorContract(got) {
		t.Fatalf("partial placement should use the existing planning-only authority marker: %+v", got)
	}
	if got.Required || writeAnalysisIRQualityRejection(repaired) != "" {
		t.Fatalf("grounded expected text alone must not turn incomplete local placement into a required target: %+v", got)
	}
	if ir.Request.BehaviorContracts[0].Placement == nil {
		t.Fatalf("repair should not mutate original placement")
	}
}

func TestRepairWriteAnalysisIRQualityMarksDirectUngroundedSatisfiesAsPlanningOnly(t *testing.T) {
	ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
		RawRequest: "collapse each consecutive newline run before normal merging",
		BehaviorContracts: []types.WriteBehaviorContract{
			{
				ID: "invented-singleton", Kind: types.WriteBehaviorObservable,
				Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies,
				Subject: "one isolated newline", Expected: "one rank token", Required: true, Source: "write_analyzer",
			},
			{
				ID: "grounded-run", Kind: types.WriteBehaviorObservable,
				Polarity: types.WriteBehaviorPolarityExpected, Operator: types.WriteBehaviorOpSatisfies,
				Subject: "consecutive newline run", Expected: "one rank token", EvidenceRef: "tests/tokenizer.py:11",
				Required: true, Source: "write_analyzer",
			},
		},
	}}
	repaired, repairs := repairWriteAnalysisIRQuality(ir)
	if len(repairs) != 1 || !strings.Contains(repairs[0], "authority=planning_only") {
		t.Fatalf("expected only the ungrounded example to be demoted, got %+v", repairs)
	}
	planning := repaired.Request.BehaviorContracts[0]
	if planning.Required || !types.IsPlanningOnlyWriteBehaviorContract(planning) {
		t.Fatalf("invented boundary example retained completion authority: %+v", planning)
	}
	grounded := repaired.Request.BehaviorContracts[1]
	if !grounded.Required || types.IsPlanningOnlyWriteBehaviorContract(grounded) {
		t.Fatalf("typed evidence-backed contract lost completion authority: %+v", grounded)
	}
	if len(types.RequiredWriteBehaviorContractIDs(repaired.Request.BehaviorContracts, true)) != 1 {
		t.Fatalf("planning-only example leaked into required contract IDs: %+v", repaired.Request.BehaviorContracts)
	}
}

func TestRunWriteAnalyzePhaseRepairsUngroundedExactContractWithoutWholeIRRetry(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	dispatchCount := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				RawRequest: "Array([]) fails while Matrix([]) works",
				Task:       types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix array empty"},
				Risk:       types.WriteRiskProfile{Overall: types.RiskBandLow},
				Constraints: []types.WriteConstraint{{
					Kind: "preserve_behavior", Target: "Matrix([])", Note: "keep the working baseline unchanged",
				}},
				ExpectedOutcomes: []string{"keep `(status = callback()) != 0` unchanged"},
				BehaviorContracts: []types.WriteBehaviorContract{{
					ID:       "shape",
					Kind:     types.WriteBehaviorInvariant,
					Polarity: types.WriteBehaviorPolarityExpected,
					Subject:  "Array([]).shape",
					Operator: types.WriteBehaviorOpEquals,
					Expected: "()",
					Required: true,
				}},
			}, PhaseProposal: types.PhaseProposal{Split: "single"}, PitfallsApplied: []string{"shape_regression"}})
			return &agent.StageOutput{StageReport: "one under-grounded exact contract"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mu := types.NewMutableState("fix array empty")
	o.busCtx = &types.BusContext{
		Mode:       types.ModeApply,
		Mutable:    mu,
		AnalysisIR: readIR,
	}

	used, err := o.runWriteAnalyzePhase()
	if err != nil {
		t.Fatalf("deterministic contract calibration should recover, got %v", err)
	}
	if used != 1 || dispatchCount != 1 {
		t.Fatalf("used=%d dispatch=%d, want one write-analyzer dispatch", used, dispatchCount)
	}
	got := mu.WriteAnalysisIR()
	if got == nil || len(got.Request.BehaviorContracts) != 1 ||
		got.Request.BehaviorContracts[0].Operator != types.WriteBehaviorOpEquals ||
		got.Request.BehaviorContracts[0].Required ||
		!types.IsPlanningOnlyWriteBehaviorContract(got.Request.BehaviorContracts[0]) {
		t.Fatalf("final IR should preserve exact semantics without ungrounded verifier authority: %+v", got)
	}
	if got.Request.Task.Summary != "fix array empty" || len(got.Request.Constraints) != 1 ||
		len(got.Request.ExpectedOutcomes) != 1 || len(got.PitfallsApplied) != 1 {
		t.Fatalf("quality calibration lost unrelated typed fields: %+v", got)
	}
}

func TestRunWriteAnalyzePhaseCalibratesStructurallyValidUngroundedSatisfiesContract(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	dispatchCount := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				RawRequest: "collapse each consecutive newline run before normal merging",
				Task:       types.WriteTask{Kind: types.WriteTaskFeature, Scope: types.ScopeMicro, Summary: "collapse newline runs"},
				Risk:       types.WriteRiskProfile{Overall: types.RiskBandLow},
				ExpectedOutcomes: []string{
					"preserve ordinary merging",
				},
				BehaviorContracts: []types.WriteBehaviorContract{{
					ID:       "invented-isolated-newline",
					Kind:     types.WriteBehaviorObservable,
					Polarity: types.WriteBehaviorPolarityExpected,
					Subject:  "one isolated newline",
					Operator: types.WriteBehaviorOpSatisfies,
					Expected: "one rank token",
					Required: true,
					Source:   "write_analyzer",
				}},
			}})
			return &agent.StageOutput{StageReport: "structurally valid but ungrounded satisfies contract"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mu := types.NewMutableState("collapse newline runs")
	o.busCtx = &types.BusContext{Mode: types.ModeApply, Mutable: mu, AnalysisIR: readIR}

	used, err := o.runWriteAnalyzePhase()
	if err != nil {
		t.Fatalf("ungrounded satisfies authority calibration should recover, got %v", err)
	}
	if used != 1 || dispatchCount != 1 {
		t.Fatalf("used=%d dispatch=%d, want one write-analyzer dispatch", used, dispatchCount)
	}
	got := mu.WriteAnalysisIR()
	if got == nil || len(got.Request.BehaviorContracts) != 1 {
		t.Fatalf("calibrated IR missing: %+v", got)
	}
	contract := got.Request.BehaviorContracts[0]
	if contract.Required || !types.IsPlanningOnlyWriteBehaviorContract(contract) {
		t.Fatalf("ungrounded satisfies contract retained completion authority: %+v", contract)
	}
	if len(types.RequiredWriteBehaviorContractIDs(got.Request.BehaviorContracts, true)) != 0 {
		t.Fatalf("ungrounded satisfies contract entered required ids: %+v", got.Request.BehaviorContracts)
	}
}

func TestRunWriteAnalyzePhaseRepairsFirstIRUngroundedContractInsteadOfRetryOrFallback(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	dispatchCount := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{
				Request: types.WriteRequestModel{
					RawRequest: "Feature request: show units in dataset overview. Desired: rainfall, in mm (time, y, x) float32 ...",
					Task:       types.WriteTask{Kind: types.WriteTaskFeature, Scope: types.ScopePackage, Summary: "show units"},
					Risk:       types.WriteRiskProfile{Overall: types.RiskBandLow},
					ScopeAnchors: []string{
						"xarray/core/formatting.py",
						"xarray/core/formatting_html.py",
					},
					ExpectedOutcomes: []string{"units are visible in repr"},
					BehaviorContracts: []types.WriteBehaviorContract{
						{
							ID:       "datavar-unit-display",
							Kind:     types.WriteBehaviorStdout,
							Polarity: types.WriteBehaviorPolarityExpected,
							Subject:  "Dataset.__repr__ data variable line",
							Operator: types.WriteBehaviorOpContains,
							Expected: "rainfall, in mm (time, y, x) float32 ...",
							Required: true,
							Source:   "write_analyzer",
						},
						{
							ID:       "no-unit-attr-safe",
							Kind:     types.WriteBehaviorException,
							Polarity: types.WriteBehaviorPolarityExpected,
							Subject:  "repr without units",
							Operator: types.WriteBehaviorOpNotRaises,
							Expected: "no new exception thrown when a variable has no units attr",
							Required: true,
							Source:   "write_analyzer",
						},
					},
				},
				PhaseProposal: types.PhaseProposal{Split: "sequential", Phases: []types.PhaseSeed{{
					Goal:             "update text repr",
					RoughTargetPaths: []string{"xarray/core/formatting.py"},
				}}},
			})
			return &agent.StageOutput{StageReport: "partial contract quality issue"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mu := types.NewMutableState("show units")
	o.busCtx = &types.BusContext{
		Mode:       types.ModeApply,
		Mutable:    mu,
		AnalysisIR: readIR,
	}

	used, err := o.runWriteAnalyzePhase()
	if err != nil {
		t.Fatalf("first-IR contract calibration should avoid retry/fallback, got %v", err)
	}
	if used != 1 || dispatchCount != 1 {
		t.Fatalf("used=%d dispatch=%d, want one attempt before deterministic calibration", used, dispatchCount)
	}
	got := mu.WriteAnalysisIR()
	if got == nil {
		t.Fatal("expected repaired WriteAnalysisIR")
	}
	if got.Request.Task.Kind != types.WriteTaskFeature || len(got.Request.ScopeAnchors) != 2 || len(got.PhaseProposal.Phases) != 1 {
		t.Fatalf("repaired IR should preserve task/scope/phase fields, got request=%+v phase=%+v", got.Request, got.PhaseProposal)
	}
	if len(got.Request.BehaviorContracts) != 2 {
		t.Fatalf("repaired IR should preserve both contracts, got %+v", got.Request.BehaviorContracts)
	}
	if got.Request.BehaviorContracts[0].Operator != types.WriteBehaviorOpContains {
		t.Fatalf("grounded output contract should remain hard, got %+v", got.Request.BehaviorContracts[0])
	}
	if got.Request.BehaviorContracts[1].Operator != types.WriteBehaviorOpNotRaises ||
		got.Request.BehaviorContracts[1].Required ||
		!types.IsPlanningOnlyWriteBehaviorContract(got.Request.BehaviorContracts[1]) {
		t.Fatalf("ungrounded contract should retain negative semantics as planning-only guidance, got %+v", got.Request.BehaviorContracts[1])
	}
	if strings.Contains(got.Request.Task.Summary, "Follow the user's requested") {
		t.Fatalf("should not install fallback IR after partial contract repair: %+v", got.Request.Task)
	}
}

// B1589: requirement authority and the model's proposed meaning are separate
// axes. Unsupported exactness removes verifier authority, not negation (or any
// other operator). This matrix deliberately covers all exact operators rather
// than teaching the repair about one request, language, or expected string.
func TestB1589UngroundedExactCalibrationPreservesSemantics(t *testing.T) {
	operators := []types.WriteBehaviorOperator{
		types.WriteBehaviorOpEquals, types.WriteBehaviorOpNotEquals,
		types.WriteBehaviorOpContains, types.WriteBehaviorOpNotContains,
		types.WriteBehaviorOpExists, types.WriteBehaviorOpNotExists,
		types.WriteBehaviorOpRaises, types.WriteBehaviorOpNotRaises,
		types.WriteBehaviorOpReturns,
	}
	for _, operator := range operators {
		for _, polarity := range []types.WriteBehaviorPolarity{types.WriteBehaviorPolarityExpected, types.WriteBehaviorPolarityForbidden} {
			t.Run(string(operator)+"/"+string(polarity), func(t *testing.T) {
				contract := types.WriteBehaviorContract{
					ID: "candidate", Kind: types.WriteBehaviorObservable, Polarity: polarity,
					Subject: "candidate behavior", Operator: operator, Expected: "opaque-exact-value",
					Required: true, Source: "write_analyzer",
				}
				ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{
					RawRequest: "repair behavior", BehaviorContracts: []types.WriteBehaviorContract{contract},
				}}
				before, err := json.Marshal(ir)
				if err != nil {
					t.Fatal(err)
				}
				if rejection := writeAnalysisIRQualityRejection(ir); rejection == "" {
					t.Fatal("ungrounded required exact contract must still fail the existing quality check")
				}

				repaired, repairs := repairWriteAnalysisIRQuality(ir)
				if len(repairs) != 1 || !strings.Contains(repairs[0], "authority=planning_only") {
					t.Fatalf("want one authority-only calibration, got %v", repairs)
				}
				want := contract
				want.Required = false
				want.Source += ";" + types.WriteBehaviorContractSourcePlanningOnlyUngrounded
				if got := repaired.Request.BehaviorContracts[0]; !reflect.DeepEqual(got, want) {
					t.Fatalf("calibration changed more than authority: got %+v, want %+v", got, want)
				}
				after, err := json.Marshal(ir)
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != string(before) {
					t.Fatalf("repair mutated the analyzer's original IR: before=%s after=%s", before, after)
				}
				if rejection := writeAnalysisIRQualityRejection(repaired); rejection != "" {
					t.Fatalf("planning-only exact meaning must not become a hard rejection: %s", rejection)
				}

				// The persisted marker must survive omission of required=false in
				// JSON and normalization; preserving an operator cannot promote it
				// back into the source-contract/proof completion lane.
				data, err := json.Marshal(repaired)
				if err != nil {
					t.Fatal(err)
				}
				var reloaded types.WriteAnalysisIR
				if err := json.Unmarshal(data, &reloaded); err != nil {
					t.Fatal(err)
				}
				contracts := types.NormalizeWriteBehaviorContracts(reloaded.Request.BehaviorContracts, nil)
				if len(contracts) != 1 || !reflect.DeepEqual(contracts[0], want) {
					t.Fatalf("reload changed authority or meaning: %+v", contracts)
				}
				if len(types.RequiredWriteBehaviorContractIDs(contracts, true)) != 0 ||
					len(types.HardRequiredWriteBehaviorContractIDs(contracts)) != 0 ||
					types.IsHardRequiredWriteBehaviorContract(contracts[0]) {
					t.Fatalf("planning-only contract acquired proof completion authority: %+v", contracts)
				}
				pack := types.WriteContextPackFromWriteAnalysisIR(repaired)
				plannerView := pack.View(types.WriteConsumerPlanner, 10)
				if len(plannerView.Items) != 1 || plannerView.Items[0].Priority != types.WriteContextP1 ||
					!strings.Contains(plannerView.Items[0].Text, "operator="+string(operator)) ||
					!strings.Contains(plannerView.Items[0].Text, "polarity="+string(polarity)) ||
					!strings.Contains(plannerView.Items[0].Text, "expected="+contract.Expected) ||
					!strings.Contains(plannerView.Items[0].Text, "planning_only=true") {
					t.Fatalf("planner lost the original proposed meaning or its advisory boundary: %+v", plannerView)
				}
				if verifierView := pack.View(types.WriteConsumerVerifier, 10); len(verifierView.Items) != 0 {
					t.Fatalf("planning-only proposal entered verifier context: %+v", verifierView)
				}
				again, againRepairs := repairWriteAnalysisIRQuality(repaired)
				if len(againRepairs) != 0 || !reflect.DeepEqual(again, repaired) {
					t.Fatalf("calibration is not idempotent: repairs=%v got=%+v", againRepairs, again)
				}
			})
		}
	}
}

func TestB1589ExactCalibrationPreservesGroundedAndNonRequiredContracts(t *testing.T) {
	for _, operator := range []types.WriteBehaviorOperator{
		types.WriteBehaviorOpEquals, types.WriteBehaviorOpNotEquals,
		types.WriteBehaviorOpContains, types.WriteBehaviorOpNotContains,
		types.WriteBehaviorOpExists, types.WriteBehaviorOpNotExists,
		types.WriteBehaviorOpRaises, types.WriteBehaviorOpNotRaises,
		types.WriteBehaviorOpReturns,
	} {
		for _, witness := range []string{"request", "evidence_ref", "comparator_ref", "comparator_request", "observed", "not_required"} {
			t.Run(string(operator)+"/"+witness, func(t *testing.T) {
				contract := types.WriteBehaviorContract{
					ID: "preserved", Kind: types.WriteBehaviorObservable, Polarity: types.WriteBehaviorPolarityExpected,
					Subject: "behavior", Operator: operator, Expected: "opaque-exact-value",
					Required: true, Source: "write_analyzer",
				}
				raw := "repair behavior; compare with baseline-value"
				switch witness {
				case "request":
					raw += "; opaque-exact-value"
				case "evidence_ref":
					contract.EvidenceRef = "tests/behavior_test.go:11"
				case "comparator_ref":
					contract.Comparator = &types.WriteBehaviorComparator{EvidenceRef: "tests/behavior_test.go:12"}
				case "comparator_request":
					contract.Comparator = &types.WriteBehaviorComparator{Expected: "baseline-value"}
				case "observed":
					contract.Polarity = types.WriteBehaviorPolarityObserved
				case "not_required":
					contract.Required = false
				}
				ir := &types.WriteAnalysisIR{Request: types.WriteRequestModel{RawRequest: raw, BehaviorContracts: []types.WriteBehaviorContract{contract}}}
				repaired, repairs := repairWriteAnalysisIRQuality(ir)
				if len(repairs) != 0 || !reflect.DeepEqual(repaired, ir) || !reflect.DeepEqual(repaired.Request.BehaviorContracts[0], contract) {
					t.Fatalf("unaffected contract was changed: repairs=%v got=%+v want=%+v", repairs, repaired, contract)
				}
				if rejection := writeAnalysisIRQualityRejection(repaired); rejection != "" {
					t.Fatalf("unaffected contract should retain existing quality admission: %s", rejection)
				}
				if witness != "observed" && witness != "not_required" {
					if !types.IsHardRequiredWriteBehaviorContract(repaired.Request.BehaviorContracts[0]) ||
						len(types.RequiredWriteBehaviorContractIDs(repaired.Request.BehaviorContracts, true)) != 1 {
						t.Fatalf("grounded exact contract lost its hard requirement: %+v", repaired.Request.BehaviorContracts[0])
					}
				}
			})
		}
	}
}
