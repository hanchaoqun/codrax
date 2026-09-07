package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1604FinalizerMemberTeachingUsesAcceptedMembershipScope(t *testing.T) {
	rm := types.RequestModel{Intent: types.IntentEnumerate, PredicateAxis: types.AxisImplement,
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true}}
	evidence := []types.EvidenceItem{
		{ID: "member", Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: "src/output.go", LineStart: 8, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Output", Subject: "Output", Snippet: "type Output struct {}", GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
		{ID: "base", Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: "src/base.go", LineStart: 4, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Base", Subject: "Base", Snippet: "type Base interface {}", GroundingStatus: types.GroundingGrounded, Origin: types.ClaimOriginCurrentRepo},
	}
	m := types.NewMutableState("")
	for _, selected := range []bool{false, true} {
		if selected {
			m.SetEmittedAnswerSymbolsWithOrigin([]types.AnswerSymbol{{Name: "Output", File: "src/output.go", Line: 8, Kind: types.KindType}}, types.CompletenessComplete, types.AnswerSymbolSelectionExplicitItems)
		}
		ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: rm}, Mutable: m, EvidenceItems: evidence}
		support := types.BuildAnswerSupportPlanForAgentContext(ctx)
		got := renderAnswerDocPrincipalMemberObligations(support, answerDocPrincipalEnumerationRowCoverage{}, false)
		if !selected {
			if got != "" || support.PrincipalMemberCoverage != types.PrincipalMemberCoveragePolicyEnrichmentOnly {
				t.Fatalf("prompt invented relation-member requirements: %s / %+v", got, support)
			}
			facts := 0
			for _, lane := range support.Lanes {
				facts += len(lane.Entries)
			}
			if facts < 2 {
				t.Fatal("removing unproved member mandate deleted the evidence")
			}
		} else if !strings.Contains(got, "Output") || strings.Contains(got, `label="Base"`) || len(types.PrincipalSupportMemberObligations(support)) != 1 {
			t.Fatalf("accepted exact slate scope lost in real finalizer teaching: %s", got)
		}
	}
}
