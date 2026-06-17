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
