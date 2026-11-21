package types

import (
	"reflect"
	"testing"
)

// HMC377 fix round 5: the share_claims note carries each claim's typed holder
// references by board position; the projection turns them into the holder
// records' ids on the same rank board, and a malformed entry is dropped whole.
func TestTraceShareClaimsNoteRoundTrip(t *testing.T) {
	value := EncodeTraceShareClaimsNote([]TraceShareClaimNote{
		{Part: "anchored", Role: "chain_seat", ShareMS: 3.5984, Resolved: true, Exact: true, HolderPositions: []int{3}},
		{Part: "unwalked", Role: "residual_seat", ShareMS: 4, Resolved: false},
		{Part: "anchored", Role: "clipped_seat", ShareMS: 9, Resolved: true, Represented: true, HolderPositions: []int{2, 7}},
		{Part: "", Role: "chain_seat", ShareMS: 1},
	})
	if value != "anchored/chain_seat/1/1/0/3.598/3;unwalked/residual_seat/0/0/0/4.000/;anchored/clipped_seat/1/0/1/9.000/2+7" {
		t.Fatalf("note value drifted: %q", value)
	}
	got := ParseTraceShareClaimsNote(value+";broken/entry", "trace_query:win#root_cause_rank:5")
	want := []TraceCausalProjectionShareClaim{
		{Part: "anchored", Role: "chain_seat", ShareMS: 3.598, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"trace_query:win#root_cause_rank:3"}},
		{Part: "unwalked", Role: "residual_seat", ShareMS: 4},
		{Part: "anchored", Role: "clipped_seat", ShareMS: 9, Resolved: true, Represented: true,
			HolderEvidenceIDs: []string{"trace_query:win#root_cause_rank:2", "trace_query:win#root_cause_rank:7"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parse drifted:\n got %+v\nwant %+v", got, want)
	}
	// A record without a rank-board identity anchors no holder (the claim
	// then reads as compacted, never as a guessed seat).
	if claims := ParseTraceShareClaimsNote("anchored/chain_seat/1/1/0/3.598/3", "ledger:other"); len(claims) != 1 || len(claims[0].HolderEvidenceIDs) != 0 {
		t.Fatalf("a position needs the rank record identity: %+v", claims)
	}
	if ParseTraceShareClaimsNote("", "trace_query:win#root_cause_rank:5") != nil {
		t.Fatalf("an absent note parses to nothing")
	}
}

// HMC377 fix round 7 (R7-2): an unresolved claim whose holders are causal
// impacts' own seats carries the impacts' positions in an optional eighth
// field; the projection turns them into the ids of those impacts' chain-view
// records on the same publication. Every other entry keeps its seven fields
// (bytes unchanged), a resolved claim never carries the field, and a record
// without a rank-board identity anchors no impact record.
func TestTraceShareClaimsNoteCarriesTheHoldersCausalImpacts(t *testing.T) {
	value := EncodeTraceShareClaimsNote([]TraceShareClaimNote{
		{Part: "anchored", Role: "chain_seat", ShareMS: 1.903, HolderImpactPositions: []int{31}},
		{Part: "chain_account", Role: "chain_seat", ShareMS: 3.784, HolderImpactPositions: []int{9, 32}},
		{Part: "anchored", Role: "chain_seat", ShareMS: 2, Resolved: true, HolderPositions: []int{4}, HolderImpactPositions: []int{5}},
		{Part: "unwalked", Role: "unwalked_seat", ShareMS: 0.017},
	})
	if value != "anchored/chain_seat/0/0/0/1.903//31;chain_account/chain_seat/0/0/0/3.784//9+32;anchored/chain_seat/1/0/0/2.000/4;unwalked/unwalked_seat/0/0/0/0.017/" {
		t.Fatalf("note value drifted: %q", value)
	}
	got := ParseTraceShareClaimsNote(value, "trace_query:publication-x#root_cause_rank:22")
	want := []TraceCausalProjectionShareClaim{
		{Part: "anchored", Role: "chain_seat", ShareMS: 1.903, HolderImpactEvidenceIDs: []string{"trace_query:publication-x#wakeup_causal_impact:31"}},
		{Part: "chain_account", Role: "chain_seat", ShareMS: 3.784,
			HolderImpactEvidenceIDs: []string{"trace_query:publication-x#wakeup_causal_impact:9", "trace_query:publication-x#wakeup_causal_impact:32"}},
		{Part: "anchored", Role: "chain_seat", ShareMS: 2, Resolved: true, HolderEvidenceIDs: []string{"trace_query:publication-x#root_cause_rank:4"}},
		{Part: "unwalked", Role: "unwalked_seat", ShareMS: 0.017},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parse drifted:\n got %+v\nwant %+v", got, want)
	}
	if claims := ParseTraceShareClaimsNote("anchored/chain_seat/0/0/0/1.903//31", "ledger:other"); len(claims) != 1 || len(claims[0].HolderImpactEvidenceIDs) != 0 {
		t.Fatalf("an impact position needs the publication identity: %+v", claims)
	}
	if claims := ParseTraceShareClaimsNote("anchored/chain_seat/1/0/0/1.903/4/31", "trace_query:p#root_cause_rank:2"); len(claims) != 1 || len(claims[0].HolderImpactEvidenceIDs) != 0 {
		t.Fatalf("a resolved claim names its board holders only: %+v", claims)
	}
	if claims := ParseTraceShareClaimsNote("anchored/chain_seat/0/0/0/1.903//31/x", "trace_query:p#root_cause_rank:2"); len(claims) != 0 {
		t.Fatalf("a malformed entry is dropped whole: %+v", claims)
	}
}
