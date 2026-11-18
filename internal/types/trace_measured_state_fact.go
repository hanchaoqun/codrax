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
// MeasuredStateAccountKey where it is not.
//
// The two fields answer different questions. StateAccountKey alone decides
// one-seat convergence, so an estimate is never folded into a measurement.
// MeasuredStateFactKey answers only "do these rows show the same measured
// account?" for tables and fact lists.

// MeasuredStateFactKey returns the native identity of the measured state
// account this row shows, or "" when the producer proved none. Compare it only
// between rows of one artifact projection. It is not a rank, seat, merge or
// causal credential.
func (n TraceCausalProjectionNode) MeasuredStateFactKey() string {
	if key := strings.TrimSpace(n.StateAccountKey); key != "" {
		return key
	}
	return strings.TrimSpace(n.MeasuredStateAccountKey)
}

// TraceMeasuredStateFactSame reports that two rows of ONE artifact projection
// show the same measured scheduler-state account: the same producer-minted
// native identity AND the same published state occupancy. Identity alone never
// joins two different displayed numbers; an equal number, subject, state or
// window is never identity.
func TraceMeasuredStateFactSame(a, b TraceCausalProjectionNode) bool {
	key := a.MeasuredStateFactKey()
	if key == "" || key != b.MeasuredStateFactKey() {
		return false
	}
	av, aok := a.PublishedStateOccupancy()
	bv, bok := b.PublishedStateOccupancy()
	return aok && bok && math.Abs(av-bv) < TraceCausalProjectionSameValueTieMS
}
