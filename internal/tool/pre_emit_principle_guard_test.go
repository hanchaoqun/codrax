package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPrincipleCitationCarrierPreEmitHintsStayAdvisoryAtBoundary(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "answer",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "run",
				Label:       "Run",
				Text:        "Run is the visible answer member.",
				CitationRef: 2,
			}},
		}},
	}

	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil)
	if len(hints) == 0 {
		t.Fatal("out-of-range citation_ref should produce a presentation-layer repair hint")
	}
	for _, hint := range hints {
		if hint.Kind != types.ViolCitation {
			t.Fatalf("citation carrier boundary should emit ViolCitation only, got %+v", hints)
		}
	}
	hard, advisory := splitPreEmitHintsByGate(hints)
	if len(hard) != 0 || len(advisory) != len(hints) {
		t.Fatalf("citation carrier hints must remain advisory after real pre-emit gate split, hard=%+v advisory=%+v", hard, advisory)
	}
}

func TestPrincipleCompleteTypedPrincipalRowSetMissingMemberRemainsHard(t *testing.T) {
	mu := types.NewMutableState("source inventory principle guard")
	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Complete: true,
		Scopes:   []string{"."},
		SourceClasses: []types.SourceInventorySourceClassCount{
			{Role: types.SourcePathRoleProduction, Count: 1, Complete: true},
			{Role: types.SourcePathRoleThirdParty, Count: 1, Complete: true},
		},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
			Count:    2,
			Total:    2,
			Members: []types.SourceInventoryObservationMember{
				{Name: "Run", Role: types.AnswerCandidateRoleFunction, File: "thirdparty/cangjie/run.cj", Line: 7, Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved},
				{Name: "Serve", Role: types.AnswerCandidateRoleFunction, File: "src/serve.cj", Line: 12, Language: "cangjie", CoverageState: types.SourceInventoryCoverageObserved},
			},
		}},
	})
	mu.SetInvestigationComplete("complete typed source inventory")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "当前 source inventory 包含 Run。",
	}}}

	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	var memberSetHints []emitFixHint
	for _, hint := range hints {
		if strings.Contains(hint.ExpectedShape, `member="Serve"`) {
			memberSetHints = append(memberSetHints, hint)
		}
	}
	if len(memberSetHints) != 1 {
		t.Fatalf("complete typed principal row-set should require the missing member, got %+v", hints)
	}
	hard, advisory := splitPreEmitHintsByGate(memberSetHints)
	if len(hard) != 1 || len(advisory) != 0 {
		t.Fatalf("complete typed principal row-set omission should remain hard, hard=%+v advisory=%+v", hard, advisory)
	}
}
