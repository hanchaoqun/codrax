package orchestrator

import (
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

func TestRepairWriteAnalysisIRQualitySoftensOnlyUngroundedExactContracts(t *testing.T) {
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
		t.Fatalf("expected one contract repair, got %+v", repairs)
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
	if soft.Operator != types.WriteBehaviorOpSatisfies {
		t.Fatalf("ungrounded exact contract should become soft satisfies, got %+v", soft)
	}
	if !strings.Contains(soft.Source, "quality_repaired:softened_ungrounded_exact") {
		t.Fatalf("softened contract should be source-tagged, got %+v", soft)
	}
	if ir.Request.BehaviorContracts[1].Operator != types.WriteBehaviorOpNotRaises {
		t.Fatalf("repair should not mutate original IR, got %+v", ir.Request.BehaviorContracts[1])
	}
}

func TestRunWriteAnalyzePhaseRetriesUngroundedExactContract(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	dispatchCount := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			if dispatchCount == 1 {
				ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
					RawRequest: "Array([]) fails while Matrix([]) works",
					Task:       types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix array empty"},
					Risk:       types.WriteRiskProfile{Overall: types.RiskBandLow},
					BehaviorContracts: []types.WriteBehaviorContract{{
						ID:       "shape",
						Kind:     types.WriteBehaviorInvariant,
						Polarity: types.WriteBehaviorPolarityExpected,
						Subject:  "Array([]).shape",
						Operator: types.WriteBehaviorOpEquals,
						Expected: "()",
						Required: true,
					}},
				}})
				return &agent.StageOutput{StageReport: "bad exact contract"}, nil
			}
			if hint := ctx.Mutable.AnalyzerRetryHint(); !strings.Contains(hint, "under-grounded") {
				t.Fatalf("retry hint should explain exact-contract grounding rejection, got %q", hint)
			}
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{
				RawRequest: "Array([]) fails while Matrix([]) works",
				Task:       types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "fix array empty"},
				Risk:       types.WriteRiskProfile{Overall: types.RiskBandLow},
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
			}})
			return &agent.StageOutput{StageReport: "good comparator contract"}, nil
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
		t.Fatalf("write analysis retry should recover, got %v", err)
	}
	if used != 2 || dispatchCount != 2 {
		t.Fatalf("used=%d dispatch=%d, want 2 retry attempts", used, dispatchCount)
	}
	got := mu.WriteAnalysisIR()
	if got == nil || len(got.Request.BehaviorContracts) != 1 || got.Request.BehaviorContracts[0].Comparator == nil {
		t.Fatalf("final IR should carry comparator-grounded contract: %+v", got)
	}
}

func TestRunWriteAnalyzePhaseRepairsFinalAttemptUngroundedContractInsteadOfFallback(t *testing.T) {
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
		t.Fatalf("final-attempt contract quarantine should avoid fallback error, got %v", err)
	}
	if used != 2 || dispatchCount != 2 {
		t.Fatalf("used=%d dispatch=%d, want 2 retry attempts before quarantine", used, dispatchCount)
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
	if got.Request.BehaviorContracts[1].Operator != types.WriteBehaviorOpSatisfies ||
		!strings.Contains(got.Request.BehaviorContracts[1].Source, "quality_repaired:softened_ungrounded_exact") {
		t.Fatalf("ungrounded contract should be softened and tagged, got %+v", got.Request.BehaviorContracts[1])
	}
	if strings.Contains(got.Request.Task.Summary, "Follow the user's requested") {
		t.Fatalf("should not install fallback IR after partial contract repair: %+v", got.Request.Task)
	}
}
