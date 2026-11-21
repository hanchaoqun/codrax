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
