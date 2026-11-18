package tracequery

import "testing"

// An occurrence that republishes a segment without carrying its inventory
// leaves the aggregate without an account, even though the other occurrence's
// segments add up to the published time on their own.
func TestWakeupAggregateMeasuredStateAccountRefusesRepublishedOccurrenceWithoutInventory(t *testing.T) {
	segment := foldInterval{start: 11.000, end: 11.002}
	complete := measuredAggregateOccurrence(11.000, 11.003, segment)
	missing := measuredAggregateOccurrence(11.000, 11.003, segment)
	missing.stateAccountIntervals = nil

	chain := ChainResult{CausalImpacts: []WakeupCausalImpact{complete, missing}}
	chain.AggregatedImpacts = aggregateWakeupCausalImpacts(&chain)
	if len(chain.AggregatedImpacts) != 1 {
		t.Fatalf("fixture must aggregate into one account, got %d", len(chain.AggregatedImpacts))
	}
	stampStateAccountPublicationKeys(&chain, nil)

	if got := chain.AggregatedImpacts[0].MeasuredStateAccountKey; got != "" {
		t.Errorf("aggregate identity=%q, want none", got)
	}
	for i, impact := range chain.CausalImpacts {
		if impact.MeasuredOccurrenceGroupKey != "" {
			t.Errorf("occurrence %d: group identity=%q, want none", i, impact.MeasuredOccurrenceGroupKey)
		}
	}
}
