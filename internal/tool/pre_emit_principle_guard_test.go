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

func TestPrinciplePreEmitBlockHardGateUsesTypedBlockKindNotHintText(t *testing.T) {
	proseOnly := emitFixHint{
		Kind:          types.ViolBlockCoverageMissing,
		Field:         "blocks[].kind=diagram",
		ExpectedShape: "emit kind=diagram",
		Reason:        "prose alone must not control hard routing",
	}
	if preEmitMissingBlockRequiresSameTurnRetry(proseOnly) {
		t.Fatal("missing-block same-turn retry must not be driven by Field/ExpectedShape prose")
	}
	if preEmitHintHardByDefault(proseOnly) {
		t.Fatal("block coverage hint without typed block kind should follow registry soft default, not prose")
	}

	typed := emitFixHint{
		Kind:               types.ViolBlockCoverageMissing,
		Field:              "blocks[].kind=visual",
		ExpectedShape:      "emit the requested visual surface",
		ExpectedBlockKinds: []types.AnswerBlockKind{types.BlockDiagram},
		Reason:             "typed block kind controls this local hard gate",
	}
	if !preEmitMissingBlockRequiresSameTurnRetry(typed) {
		t.Fatal("typed diagram block requirement should require same-turn retry")
	}
	if !preEmitHintHardByDefault(typed) {
		t.Fatal("typed diagram block requirement should remain a same-turn hard gate")
	}
}

func TestPrinciplePreEmitSameTurnHardPolicyIsExplicit(t *testing.T) {
	rows := preEmitSameTurnHardPolicyRows()
	if len(rows) != 2 {
		t.Fatalf("same-turn hard policy should stay small and explicit, got %+v", rows)
	}
	want := map[preEmitSameTurnHardPolicyRow]bool{
		{Kind: types.ViolExhaustiveMemberSetCoverageDrift, Signal: preEmitHardSignalCompletePrincipalMemberSet}: true,
		{Kind: types.ViolBlockCoverageMissing, Signal: preEmitHardSignalTypedRequiredBlockKind}:                 true,
	}
	for _, row := range rows {
		if !want[row] {
			t.Fatalf("unexpected same-turn hard policy row: %+v", row)
		}
		delete(want, row)
	}
	if len(want) != 0 {
		t.Fatalf("missing same-turn hard policy rows: %+v", want)
	}
}

func TestPrincipleForceHardCannotPromotePresentationOrNoisyKinds(t *testing.T) {
	hint := emitFixHint{
		Kind:          types.ViolCitation,
		Field:         "citations[]",
		ExpectedShape: "align presentation citation pool",
		Reason:        "presentation carrier drift must stay advisory",
		ForceHard:     true,
	}
	if preEmitHintHardByDefault(hint) {
		t.Fatal("ForceHard must not promote citation/presentation carriers without an explicit typed hard-policy row")
	}

	hint.Kind = types.ViolCardinalityShort
	hint.Field = "blocks[].text/count claims"
	hint.ExpectedShape = "visible count parsing drift"
	if preEmitHintHardByDefault(hint) {
		t.Fatal("ForceHard must not promote noisy visible-count parsing without an explicit typed hard-policy row")
	}

	hint.Kind = types.ViolExhaustiveMemberSetCoverageDrift
	hint.Field = "blocks[].items[]"
	hint.ExpectedShape = "include complete principal member set"
	if !preEmitHintHardByDefault(hint) {
		t.Fatal("complete typed principal member-set coverage remains an allowed same-turn hard gate")
	}
}

func TestPrincipleUntaggedPreEmitHintsStayAdvisory(t *testing.T) {
	hint := emitFixHint{
		Field:         "blocks[].items[]",
		ExpectedShape: "fix an unclassified helper hint",
		Reason:        "missing violation kind means missing precision and fallback policy",
	}
	if preEmitHintHardByDefault(hint) {
		t.Fatal("untagged pre-emit hints must not hard-block without a closed violation kind")
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
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqEnumeration),
				Entities: []string{"Run", "Serve"},
			},
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all functions"},
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

func TestPrincipleVisibleCountDriftStaysAdvisoryAtPreEmitBoundary(t *testing.T) {
	members := []string{"AnalyzerAgent", "ExplorerAgent", "ExtractorAgent", "FinalizerAgent"}
	mu := types.NewMutableState("visible count principle guard")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "agent list",
		Value:   "4",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: members,
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	items := make([]types.AnswerBlockItem, 0, len(members))
	for _, member := range members {
		items = append(items, types.AnswerBlockItem{Label: member})
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "agent list 一共有 3 个，下面列全。",
	}, {
		ID:    "members",
		Kind:  types.BlockOrderedList,
		Items: items,
	}}}

	hints := preCheckAggregateCardinalityConsistency(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("visible count drift should still produce an advisory hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "expected_count=4") ||
		!strings.Contains(hints[0].ExpectedShape, "visible_count=3") {
		t.Fatalf("hint should report typed/cardinality mismatch details, got %+v", hints[0])
	}
	hard, advisory := splitPreEmitHintsByGate(tagPreEmitHints(types.ViolCardinalityShort, hints))
	if len(hard) != 0 || len(advisory) != 1 {
		t.Fatalf("visible prose/table count parsing is not precise enough to hard-block by default, hard=%+v advisory=%+v", hard, advisory)
	}
}
