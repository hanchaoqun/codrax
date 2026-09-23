package criterion

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestToolDocumentationCriteriaDoNotMintEvidence(t *testing.T) {
	pure := &types.AnalysisIR{RequestModel: types.RequestModel{ToolDocumentationRequest: &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestOnly}}}
	for _, ready := range []bool{false, true} {
		env := Env{IR: pure, ToolDocumentationReady: ready, DraftAnswer: "Capabilities are documented; no measurements were taken.", Signals: types.ExecutionSignals{HasEnoughFacts: true}, InvestigationComplete: true}
		for _, c := range []types.Criterion{{Kind: types.CritToolDocumentationReady}, {Kind: types.CritExtractInputReady}, {Kind: types.CritHasEnoughFacts}, {Kind: types.CritEvidenceCount, Expr: ">=3"}, {Kind: types.CritCitationCountGE, Expr: "2"}, {Kind: types.CritContractSatisfied}} {
			if result := Eval(c, env); result.UnknownKind || result.Satisfied != ready {
				t.Errorf("%+v ready=%t: %+v", c, ready, result)
			}
		}
		if Eval(types.Criterion{Kind: types.CritNoRelevantEvidence}, env).Satisfied {
			t.Fatal("source absence became documentation absence")
		}
		if len(env.Evidence) != 0 || env.DraftCitations != 0 {
			t.Fatal("documentation minted source authority")
		}
	}
	for _, domain := range []string{"legacy", "mixed", "conflict"} {
		t.Run(domain, func(t *testing.T) {
			rm := types.RequestModel{}
			if domain == "mixed" {
				rm.ToolDocumentationRequest = &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestMixed, DimensionIndices: []int{1}}
				rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []types.RequestedAnswerDimension{{Index: 1, Required: true, Role: types.RequestedAnswerDimensionFunctionOrPurpose}}}
			}
			if domain == "conflict" {
				rm.ToolDocumentationRequest = &types.ToolDocumentationRequest{Scope: types.ToolDocumentationRequestOnly}
				rm.UserPinnedFiles = []string{"src/a.go"}
			}
			env := Env{IR: &types.AnalysisIR{RequestModel: rm}, ToolDocumentationReady: true}
			for _, c := range []types.Criterion{{Kind: types.CritEvidenceCount, Expr: ">=1"}, {Kind: types.CritCitationCountGE, Expr: "1"}, {Kind: types.CritExtractInputReady}} {
				if Eval(c, env).Satisfied {
					t.Errorf("%s erased independent source readiness %+v", domain, c)
				}
			}
			if !Eval(types.Criterion{Kind: types.CritNoRelevantEvidence}, env).Satisfied {
				t.Fatal("legacy/mixed evidence test changed")
			}
		})
	}
	ready := Env{IR: pure, ArtifactReadiness: &types.ArtifactReadinessView{ToolDocumentationReady: true}}
	if !Eval(types.Criterion{Kind: types.CritToolDocumentationReady}, ready).Satisfied {
		t.Fatal("L1-independent readiness projection was ignored")
	}
	ready.IR = &types.AnalysisIR{}
	ready.ArtifactReadiness.Active = true
	ready.ArtifactReadiness.ReasonCode = types.ArtifactReadinessProducerLineageMiss
	if Eval(types.Criterion{Kind: types.CritExtractInputReady}, ready).Satisfied {
		t.Fatal("documentation readiness bypassed unrelated runtime/source lineage")
	}
}
