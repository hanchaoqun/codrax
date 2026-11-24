package types

import (
	"math"
	"strings"
)

// trace_measured_state_fact.go — one identity for one measured scheduler-state
// account, for display consumers.
//
// A native state account (one thread, one scheduler state, one exact set of
// segments) can reach a reader through several typed publications: a rank
// seat priced at it, a rank seat priced differently, a wakeup view, a state
// drilldown. The producer mints one inventory hash per account. It travels as
// StateAccountKey where the seat's price IS that measurement, and as
// MeasuredStateAccountKey where it is not. An account that spans several
// occurrences has one hash over all their segments: its aggregate and rank seat
// carry it as MeasuredStateAccountKey, and each occurrence row names it as
// MeasuredOccurrenceGroupKey so a fold of the occurrences can show it.
//
// The two fields answer different questions. StateAccountKey alone decides
// one-seat convergence, so an estimate is never folded into a measurement.
// MeasuredStateFactKey answers only "do these rows show the same measured
// account?" for tables, fact lists and the tree's one-object display rule
// (HMC377 R-377-13 ① as amended by R-377-14 ①: two tree lines of one thread on
// one measured account are drawn as one row, or wear the non-additive tag when
// the value each draws counts time of the account's own state; no account,
// price or seat moves).

// MeasuredStateFactKey returns the native identity of the measured state
// account this row shows, or "" when the producer proved none. A ×N fold shows
// its occurrences together, so it answers with their shared account and never
// with the account of the member it was seeded from. Compare it only between
// rows of one artifact projection. It is not a rank, seat, merge or causal
// credential.
func (n TraceCausalProjectionNode) MeasuredStateFactKey() string {
	if n.MergedCount > 1 {
		return strings.TrimSpace(n.MeasuredOccurrenceGroupKey)
	}
	if key := strings.TrimSpace(n.StateAccountKey); key != "" {
		return key
	}
	return strings.TrimSpace(n.MeasuredStateAccountKey)
}

// MeasuredOccurrenceGroup returns the native identity of the multi-occurrence
// account this one occurrence row belongs to (its aggregate's measured
// account), or "" when the producer named none. It answers only "which drawn
// seat shows the account this occurrence is part of?" for display faces (the
// tree's constituent pointer, HMC377 R-377-13 ①); a ×N fold answers with
// MeasuredStateFactKey instead. It is not a rank, seat, merge or causal
// credential.
func (n TraceCausalProjectionNode) MeasuredOccurrenceGroup() string {
	if n.MergedCount > 1 {
		return ""
	}
	return strings.TrimSpace(n.MeasuredOccurrenceGroupKey)
}

// measuredStateFactValue is the occupancy a row shows for its measured account.
// A fold publishes no state partition of its own; when all its occurrences name
// one account, its folded value is that account's occupancy.
func (n TraceCausalProjectionNode) measuredStateFactValue() (float64, bool) {
	if n.MergedCount > 1 {
		value := n.ImpactMS
		return value, !n.OnChainOverflowFold && n.MeasuredStateFactKey() != "" &&
			value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
	}
	return n.PublishedStateOccupancy()
}

// TraceMeasuredStateFactSame reports that two rows of ONE artifact projection
// show the same measured scheduler-state account: the same producer-minted
// native identity AND the same shown occupancy. Identity alone never joins two
// different displayed numbers; an equal number, subject, state or window is
// never identity.
func TraceMeasuredStateFactSame(a, b TraceCausalProjectionNode) bool {
	key := a.MeasuredStateFactKey()
	if key == "" || key != b.MeasuredStateFactKey() {
		return false
	}
	av, aok := a.measuredStateFactValue()
	bv, bok := b.measuredStateFactValue()
	return aok && bok && math.Abs(av-bv) < TraceCausalProjectionSameValueTieMS
}

// traceCausalProjectionUnanimousOccurrenceGroupKey is the account a ×N fold
// shows: the one every member names, or "" when any member names another or
// none.
func traceCausalProjectionUnanimousOccurrenceGroupKey(nodes []TraceCausalProjectionNode, members []int) string {
	shared := ""
	for _, index := range members {
		key := strings.TrimSpace(nodes[index].MeasuredOccurrenceGroupKey)
		if key == "" || (shared != "" && key != shared) {
			return ""
		}
		shared = key
	}
	return shared
}
