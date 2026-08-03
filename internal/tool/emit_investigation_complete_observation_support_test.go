package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func groundedCallObservation(id, subject, object, source string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		ID:              id,
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          source,
		LineStart:       line,
		LineEnd:         line,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    object,
		Subject:         subject,
		Predicate:       "calls",
		Object:          object,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_CanonicalizesObservationAxis(t *testing.T) {
	const source = "internal/types/typed_relation_hint.go"
	evidence := []types.EvidenceItem{
		groundedCallObservation("call-294", "BuildTypedRelationQueryWithResolvedSources", "appendKinds", source, 294),
		groundedCallObservation("call-295", "BuildTypedRelationQueryWithResolvedSources", "appendKinds", source, 295),
		groundedCallObservation("call-321", "TypedRelationKindsForRequest", "appendKinds", source, 321),
	}
	mut := types.NewMutableState("enumerate every caller")
	mut.AppendEvidence(evidence)
	fact := types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "callers",
		Value:   "2",
		Members: []string{"BuildTypedRelationQueryWithResolvedSources (lines 294-295)", "TypedRelationKindsForRequest (line 321)"},
		SupportRefs: []string{
			source + ":294",
			source + ":295",
			source + ":321",
		},
	}

	got := enrichCompletionAggregateFactsWithMemberSupportWithEvidence(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{fact}, evidence)
	want := []string{source + ":294", source + ":321"}
	if len(got) != 1 || !reflect.DeepEqual(got[0].SupportRefs, want) {
		t.Fatalf("support_refs = %+v, want %+v", got, want)
	}
	support := buildAggregateMemberSupportIndexWithEvidence(&types.BusContext{Mutable: mut}, evidence)
	for idx, member := range got[0].Members {
		if !aggregateMemberSetMemberUsableAt(got[0], member, idx, support) {
			t.Fatalf("member %d %q is not usable with canonical ref %q", idx, member, got[0].SupportRefs[idx])
		}
	}
}

func TestEmitInvestigationComplete_CanonicalizesObservationAxisBeforeStableCarrier(t *testing.T) {
	previous := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(previous) })

	const source = "internal/types/typed_relation_hint.go"
	evidence := []types.EvidenceItem{
		groundedCallObservation("call-294", "BuildTypedRelationQueryWithResolvedSources", "appendKinds", source, 294),
		groundedCallObservation("call-295", "BuildTypedRelationQueryWithResolvedSources", "appendKinds", source, 295),
		groundedCallObservation("call-321", "TypedRelationKindsForRequest", "appendKinds", source, 321),
	}
	mut := types.NewMutableState("enumerate every caller")
	mut.AppendEvidence(evidence)
	params := json.RawMessage(`{
		"reason":"all grounded callers were enumerated",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"callers",
			"value":"2",
			"role":"principal_answer",
			"members":[
				"BuildTypedRelationQueryWithResolvedSources (lines 294-295)",
				"TypedRelationKindsForRequest (line 321)"
			],
			"support_refs":[
				"internal/types/typed_relation_hint.go:294",
				"internal/types/typed_relation_hint.go:295",
				"internal/types/typed_relation_hint.go:321"
			]
		}]
	}`)
	result, err := (&EmitInvestigationComplete{}).Execute(&types.BusContext{Mutable: mut}, params)
	if err != nil || !result.Success {
		t.Fatalf("completion failed: result=%+v err=%v", result, err)
	}
	facts := mut.StableInvestigationAggregateFacts()
	want := []string{source + ":294", source + ":321"}
	if len(facts) != 1 || !reflect.DeepEqual(facts[0].SupportRefs, want) {
		t.Fatalf("stable support_refs = %+v, want %+v", facts, want)
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_PreservesAmbiguousObservationAxis(t *testing.T) {
	const source = "internal/example/callers.go"
	evidence := []types.EvidenceItem{
		groundedCallObservation("shared", "FirstCaller", "SecondCaller", source, 10),
		groundedCallObservation("shared-again", "FirstCaller", "SecondCaller", source, 11),
		groundedCallObservation("shared-third", "FirstCaller", "SecondCaller", source, 12),
	}
	mut := types.NewMutableState("enumerate every caller")
	mut.AppendEvidence(evidence)
	fact := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "callers",
		Value:       "2",
		Members:     []string{"FirstCaller", "SecondCaller"},
		SupportRefs: []string{source + ":10", source + ":11", source + ":12"},
	}

	got := enrichCompletionAggregateFactsWithMemberSupportWithEvidence(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{fact}, evidence)
	if len(got) != 1 || !reflect.DeepEqual(got[0].SupportRefs, fact.SupportRefs) {
		t.Fatalf("ambiguous observation refs must be preserved, got %+v want %+v", got, fact.SupportRefs)
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_PreservesIncompleteObservationAxis(t *testing.T) {
	const source = "internal/example/callers.go"
	evidence := []types.EvidenceItem{
		groundedCallObservation("first-a", "FirstCaller", "target", source, 10),
		groundedCallObservation("first-b", "FirstCaller", "target", source, 11),
		groundedCallObservation("first-c", "FirstCaller", "target", source, 13),
	}
	mut := types.NewMutableState("enumerate every caller")
	mut.AppendEvidence(evidence)
	fact := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "callers",
		Value:       "2",
		Members:     []string{"FirstCaller", "SecondCaller"},
		SupportRefs: []string{source + ":10", source + ":11", source + ":13"},
	}

	got := enrichCompletionAggregateFactsWithMemberSupportWithEvidence(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{fact}, evidence)
	if len(got) != 1 || !reflect.DeepEqual(got[0].SupportRefs, fact.SupportRefs) {
		t.Fatalf("incomplete observation refs must be preserved, got %+v want %+v", got, fact.SupportRefs)
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_PreservesExistingPositionalRefs(t *testing.T) {
	const source = "internal/example/callers.go"
	evidence := []types.EvidenceItem{
		groundedCallObservation("first", "FirstCaller", "target", source, 10),
		groundedCallObservation("second", "SecondCaller", "target", source, 12),
	}
	mut := types.NewMutableState("enumerate every caller")
	mut.AppendEvidence(evidence)
	fact := types.AnswerAggregateFact{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "callers",
		Value:       "2",
		Members:     []string{"FirstCaller", "SecondCaller"},
		SupportRefs: []string{source + ":10", source + ":12"},
	}

	got := enrichCompletionAggregateFactsWithMemberSupportWithEvidence(&types.BusContext{Mutable: mut}, []types.AnswerAggregateFact{fact}, evidence)
	if len(got) != 1 || !reflect.DeepEqual(got[0].SupportRefs, fact.SupportRefs) {
		t.Fatalf("positional refs changed: got %+v want %+v", got, fact.SupportRefs)
	}
}
