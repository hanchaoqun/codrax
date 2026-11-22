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
	// HMC377 round 11 (R-377-11 ①): an Extended entry carries the per-segment
	// resolution in an eighth field; every other entry keeps seven fields.
	t.Run("extended entry", func(t *testing.T) {
		value := EncodeTraceShareClaimsNote([]TraceShareClaimNote{
			{Part: "anchored", Role: "chain_seat", ShareMS: 1.903, HolderPositions: []int{3}, Extended: true, ResolvedMS: 0.853, ReturnedMS: 1.05},
			{Part: "gated_share", Role: "gated_share_seat", ShareMS: 1.903, Resolved: true, Exact: true, HolderPositions: []int{5}, Extended: true, ResolvedMS: 1.903, Successor: true},
		})
		if value != "anchored/chain_seat/0/0/0/1.903/3/r=0.853,t=1.050,s=0;gated_share/gated_share_seat/1/1/0/1.903/5/r=1.903,t=0.000,s=1" {
			t.Fatalf("extended note value drifted: %q", value)
		}
		got := ParseTraceShareClaimsNote(value+";anchored/chain_seat/0/0/0/1.000//r=x,t=0,s=0;anchored/chain_seat/0/0/0/1.000//r=1,s=0", "trace_query:win#root_cause_rank:5")
		want := []TraceCausalProjectionShareClaim{
			{Part: "anchored", Role: "chain_seat", ShareMS: 1.903, HolderEvidenceIDs: []string{"trace_query:win#root_cause_rank:3"}, ResolvedMS: 0.853, ReturnedMS: 1.05},
			{Part: "gated_share", Role: "gated_share_seat", ShareMS: 1.903, Resolved: true, Exact: true, HolderEvidenceIDs: []string{"trace_query:win#root_cause_rank:5"}, ResolvedMS: 1.903, Successor: true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("extended parse drifted (malformed extensions are dropped whole):\n got %+v\nwant %+v", got, want)
		}
	})
}
