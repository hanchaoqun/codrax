package tool

// trace_query_aggregate_fold_tie_p21_test.go — P2-1 wire pins (第四跨线程
// 取最大点, §29.6 G12-ENG batch, 2026-07-09): the engine aggregate-trim fold's
// µs-tie roster rides the EXISTING same_value_members note (zero new keys) on
// the wakeup_causal_aggregate fold record, and the projection compile
// re-materializes it into node.SameValueMembers through the DIAG-A1 consumer
// chain unchanged.
//
// MUTATION self-checks:
//   - dropping the note emission in traceQueryWakeupCausalAggregateFoldRecord
//     reds TestP21AggregateFoldRecordCarriesTieNote;
//   - emitting the note for <2 ties reds
//     TestP21AggregateFoldRecordZeroDropsWithoutTies.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func p21TieFold() tracequery.WakeupCausalAggregateFold {
	return tracequery.WakeupCausalAggregateFold{
		Groups:      2,
		MinImpactMs: 14.272,
		MaxImpactMs: 14.272,
		Subjects:    []string{"hmfs_discard-26-562", "oney.hmn.berlin-42591"},
		LineStart:   1017021,
		LineEnd:     1625582,
		FirstTs:     6793224.9,
		LastTs:      6793225.0,
		SameValueMembers: []tracequery.WakeupCausalAggregateFoldTieMember{
			{Label: "hmfs_discard-26-562", LineStart: 1484314, LineEnd: 1625582},
			{Label: "oney.hmn.berlin-42591", LineStart: 1017021, LineEnd: 1043697},
		},
	}
}

func TestP21AggregateFoldRecordCarriesTieNote(t *testing.T) {
	record := traceQueryWakeupCausalAggregateFoldRecord("p21", types.ObservationSourceRef{Path: "berlin.systrace"},
		"2026-07-09T00:00:00Z", p21TieFold(), tracequery.TimeWindow{StartTs: 6793224.9, EndTs: 6793225.0})
	joined := strings.Join(record.RichNotes, "\n")
	want := "same_value_members=hmfs_discard-26-562@1484314-1625582,oney.hmn.berlin-42591@1017021-1043697"
	if !strings.Contains(joined, want) {
		t.Fatalf("aggregate fold record must carry the tie roster note:\n%s", joined)
	}
	// Consumer chain (DIAG A1, unchanged): the projection compile
	// re-materializes the note into node.SameValueMembers.
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{record})
	var fold *types.TraceCausalProjectionNode
	for _, bucket := range [][]types.TraceCausalProjectionNode{projection.OnChainCauses, projection.SupportingHops} {
		for i := range bucket {
			if bucket[i].OnChainOverflowFold {
				fold = &bucket[i]
			}
		}
	}
	if fold == nil {
		t.Fatalf("fold record must re-materialize as the overflow fold node: %+v", projection)
	}
	if len(fold.SameValueMembers) != 2 ||
		fold.SameValueMembers[0].Subject != "hmfs_discard-26-562" ||
		fold.SameValueMembers[0].LineStart != 1484314 ||
		fold.SameValueMembers[1].Subject != "oney.hmn.berlin-42591" ||
		fold.SameValueMembers[1].LineEnd != 1043697 {
		t.Fatalf("compile must re-materialize the tie roster losslessly: %+v", fold.SameValueMembers)
	}
}

func TestP21AggregateFoldRecordZeroDropsWithoutTies(t *testing.T) {
	fold := p21TieFold()
	fold.SameValueMembers = nil
	record := traceQueryWakeupCausalAggregateFoldRecord("p21", types.ObservationSourceRef{Path: "berlin.systrace"},
		"2026-07-09T00:00:00Z", fold, tracequery.TimeWindow{StartTs: 6793224.9, EndTs: 6793225.0})
	if joined := strings.Join(record.RichNotes, "\n"); strings.Contains(joined, "same_value_members=") {
		t.Fatalf("no ties → the note zero-drops:\n%s", joined)
	}
	// A single-entry roster (defensive: cannot be minted by the engine helper)
	// also zero-drops — one member at the max is just the max.
	fold.SameValueMembers = []tracequery.WakeupCausalAggregateFoldTieMember{{Label: "solo-1", LineStart: 1, LineEnd: 2}}
	record = traceQueryWakeupCausalAggregateFoldRecord("p21", types.ObservationSourceRef{Path: "berlin.systrace"},
		"2026-07-09T00:00:00Z", fold, tracequery.TimeWindow{StartTs: 6793224.9, EndTs: 6793225.0})
	if joined := strings.Join(record.RichNotes, "\n"); strings.Contains(joined, "same_value_members=") {
		t.Fatalf("<2 labeled ties → the note zero-drops:\n%s", joined)
	}
}
