package types

import (
	"reflect"
	"testing"
)

func TestInvestigationRelationClaimsSurviveResetAndExploreForkMerge(t *testing.T) {
	value := 5.149
	first := []AnswerRelationClaim{{
		AuthorityID: "trace:first:wall", MemberRefs: []string{"#4", "#13"},
		PhysicalRelation: AnswerPhysicalRelationUnresolved, Addition: AnswerRelationAdditionAuthorized,
		SubtotalValue: &value, SubtotalUnit: "ms",
	}}
	mu := NewMutableState("relation context")
	mu.SetInvestigationRelationClaims(first)
	mu.SetInvestigationComplete("accepted first relation")
	mu.RetainInvestigationRelationClaims()
	mu.ResetInvestigationComplete()
	if got := mu.StableInvestigationRelationClaims(); !AnswerRelationClaimsEqual(got, first) {
		t.Fatalf("retained claims lost across completion reset: %+v", got)
	}

	fork := mu.ForkForExploreDispatch()
	second := []AnswerRelationClaim{{
		AuthorityID: "trace:second:cross", MemberRefs: []string{"#4", "#10"},
		PhysicalRelation: AnswerPhysicalRelationUnresolved, Addition: AnswerRelationAdditionForbidden,
	}}
	fork.SetInvestigationRelationClaims(second)
	fork.SetInvestigationComplete("accepted second relation")
	fork.RetainInvestigationRelationClaims()
	mu.MergeExploreFork(fork)
	if got := mu.StableInvestigationRelationClaims(); !AnswerRelationClaimsEqual(got, second) {
		t.Fatalf("later accepted fork claims did not win: %+v", got)
	}
}

func TestAnswerDocumentRelationClaimsAreDeepCloned(t *testing.T) {
	value := 3.3
	doc := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "s", Kind: BlockSummary, RelationClaims: []AnswerRelationClaim{{
			AuthorityID: "trace:wall", MemberRefs: []string{"#4", "#9"},
			PhysicalRelation: AnswerPhysicalRelationUnresolved, Addition: AnswerRelationAdditionAuthorized,
			SubtotalValue: &value, SubtotalUnit: "ms",
		}},
	}}}
	mu := NewMutableState("answer clone")
	mu.SetAnswerDocumentV2WithMutation(MutationReplaceAll, doc)
	doc.Blocks[0].RelationClaims[0].MemberRefs[0] = "corrupted"
	*doc.Blocks[0].RelationClaims[0].SubtotalValue = 99
	got := mu.AnswerDocumentV2()
	if got == nil || got.Blocks[0].RelationClaims[0].MemberRefs[0] != "#4" || *got.Blocks[0].RelationClaims[0].SubtotalValue != 3.3 {
		t.Fatalf("stored relation claims alias caller memory: %+v", got)
	}
	got.Blocks[0].RelationClaims[0].MemberRefs[0] = "reader-corruption"
	again := mu.AnswerDocumentV2()
	if again.Blocks[0].RelationClaims[0].MemberRefs[0] != "#4" {
		t.Fatalf("returned relation claims alias mutable state: %+v", again)
	}
}

func TestAnswerDocumentSourceInventoryFamilySurvivesMutableRoundTrip(t *testing.T) {
	doc := &AnswerDocumentV2{DocumentModel: "v2", Blocks: []AnswerBlock{{
		ID: "classes", Kind: BlockTable, SourceInventoryFamily: "public class",
		Items: []AnswerBlockItem{{
			ID: "cart", Label: "Cart", SourceInventoryRowID: "enum-set-row-cart-class", EvidenceIDs: []string{"ev-class"},
			CitationRefsModelSubmitted: true, CitationRefsModelSubmittedValues: []int{2, 4}, CitationRefsEvidenceIDAdoptionRequired: true,
		}},
	}}}
	mu := NewMutableState("answer family clone")
	mu.SetAnswerDocumentV2WithMutation(MutationReplaceAll, doc)
	doc.Blocks[0].Items[0].EvidenceIDs[0] = "corrupted"
	got := mu.AnswerDocumentV2()
	if got == nil || len(got.Blocks) != 1 || got.Blocks[0].SourceInventoryFamily != "public class" ||
		len(got.Blocks[0].Items) != 1 || got.Blocks[0].Items[0].SourceInventoryRowID != "enum-set-row-cart-class" ||
		len(got.Blocks[0].Items[0].EvidenceIDs) != 1 || got.Blocks[0].Items[0].EvidenceIDs[0] != "ev-class" ||
		!got.Blocks[0].Items[0].CitationRefsModelSubmitted ||
		!reflect.DeepEqual(got.Blocks[0].Items[0].CitationRefsModelSubmittedValues, []int{2, 4}) ||
		!got.Blocks[0].Items[0].CitationRefsEvidenceIDAdoptionRequired {
		t.Fatalf("source-inventory family/row identity was dropped by mutable-state clone: %+v", got)
	}
}
