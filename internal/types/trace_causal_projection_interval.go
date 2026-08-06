package types

// Shared ts-interval algebra for the Trace Causal Projection — the SINGLE
// authority for every interval consumer of the node-level StartTs/EndTs facts
// (§11 复核要点②, docs/design/real_trace_campaign_20260705.md): the R2
// cross-query-window ×N union caliber (N2, wired in this batch in
// trace_causal_projection_aggregate.go), the coverage-numerator union lower
// bound (N4) and the cross-predicate same-segment cross-reference (N6) — the
// latter two are FUTURE consumers and MUST import this file, never re-derive
// the algebra (the pre-N2 defect was exactly three consumers each ignoring
// the intervals already in hand).
//
// Everything here is pure interval arithmetic over trace timestamps in
// SECONDS — precise signals only (float comparisons on typed StartTs/EndTs
// endpoints), no heuristics, no tolerance bands. Interval validity follows
// the shared window-existence predicate below (§29.183 G8): EndTs > StartTs
// && StartTs >= 0. Invalid intervals are ignored (fail-open: an absent
// interval never manufactures overlap and never deducts value), never
// guessed.

// TraceCausalProjectionInterval is one ts interval in seconds.
type TraceCausalProjectionInterval struct {
	StartTs float64
	EndTs   float64
}

// TraceCausalProjectionWindowPresent is THE shared window/interval existence
// predicate of the projection/answer layer (§29.183 G8, 2026-07-21): a
// window/span exists when it has positive length and a non-negative start —
// end > start && start >= 0. Rationale: a rebased trace legitimately starts
// at ts=0 (re-based exports; explicit `--trace-window 0..X`), so start==0 is
// a REAL anchor, not absence — the former `start > 0` guard silently dropped
// the anchor window, the within-window markers, the window-% denominators
// and the elimination-subtotal eligibility for legal [0,end] traces (all
// conservative-direction degradation, but silent feature loss). The (0,0)
// zero-value ABSENCE encoding still fails `end > start`, so absence stays
// excluded without any start>0 test (the predicate is self-protecting).
// Scope boundary: engine Query-API params keep their 0=unset sentinel
// (`TimeStart > 0` idiom + the explicit TimeStartSet flag) — this predicate
// judges projection/answer-layer node & window FACTS only, never raw query
// params or un-normalized engine windows.
// WINFLAG-1 rider (§29.190④, 2026-07-21): the engine-window half of the
// start==0 ambiguity is now typed at its SOURCE — tracequery's
// TimeWindow.StartSet / StartDetermined is the flag-aware companion judge
// for engine RESULT windows, and the internal/tool mint helpers
// (traceQuerySelectedWindowNoteValue / traceQueryObservationWindowSpanTs)
// branch there, so an unset-0 window never reaches this layer as a fake
// (0,end) fact in NEW artifacts. This predicate deliberately stays
// flag-free: it keeps judging already-minted facts, and no-flag artifacts
// from older builds keep exactly this behavior (渐进兼容).
func TraceCausalProjectionWindowPresent(startTs, endTs float64) bool {
	return endTs > startTs && startTs >= 0
}

// traceCausalProjectionIntervalValid is the shared validity guard: a real
// trace interval has a non-negative start (§29.183 G8: ts==0 is a real
// timestamp in a rebased trace) and a strictly later end. Zero-length
// intervals are invalid — they carry no wall clock to union or overlap.
func traceCausalProjectionIntervalValid(startTs, endTs float64) bool {
	return TraceCausalProjectionWindowPresent(startTs, endTs)
}

// TraceCausalProjectionIntervalsOverlap reports whether two valid ts
// intervals intersect with positive length (strict inequalities: intervals
// that merely touch at an endpoint share no wall clock). Either interval
// invalid → false, never a guess. Exported for the display-layer interval
// consumers (N6 cross-predicate same-segment cross-reference lives in
// internal/tool); the types-layer twin traceCausalProjectionSpansOverlap
// delegates here so the predicate has exactly one home.
func TraceCausalProjectionIntervalsOverlap(aStartTs, aEndTs, bStartTs, bEndTs float64) bool {
	if !traceCausalProjectionIntervalValid(aStartTs, aEndTs) || !traceCausalProjectionIntervalValid(bStartTs, bEndTs) {
		return false
	}
	return aStartTs < bEndTs && bStartTs < aEndTs
}

// TraceCausalProjectionFaithfulEnvelopeOverlapToleranceMS is the shared
// microsecond-scale threshold for declaring two node envelopes physically
// overlapping. Smaller intersections are float dust and carry no relation
// authority.
const TraceCausalProjectionFaithfulEnvelopeOverlapToleranceMS = 0.001

// TraceCausalProjectionFaithfulEnvelopeOverlapMS returns the measured overlap
// of two faithful, unmerged typed node envelopes. This is the single relation
// predicate shared by the post-final deterministic eliminable board and the
// pre-final model decision handoff. Missing envelopes and merged carriers
// fail closed: they cannot manufacture a pair relation.
func TraceCausalProjectionFaithfulEnvelopeOverlapMS(a, b TraceCausalProjectionNode) (float64, bool) {
	if a.MergedCount > 1 || b.MergedCount > 1 ||
		!TraceCausalProjectionWindowPresent(a.StartTs, a.EndTs) ||
		!TraceCausalProjectionWindowPresent(b.StartTs, b.EndTs) {
		return 0, false
	}
	lo, hi := a.StartTs, a.EndTs
	if b.StartTs > lo {
		lo = b.StartTs
	}
	if b.EndTs < hi {
		hi = b.EndTs
	}
	overlapMS := (hi - lo) * 1000
	if overlapMS <= TraceCausalProjectionFaithfulEnvelopeOverlapToleranceMS {
		return 0, false
	}
	return overlapMS, true
}

// TraceCausalProjectionIntervalSet is a union of ts intervals kept sorted and
// disjoint (Add merges overlapping/touching spans). The zero value is an
// empty, ready-to-use set.
type TraceCausalProjectionIntervalSet struct {
	spans []TraceCausalProjectionInterval
}

// Add unions one interval into the set; invalid intervals are ignored.
func (s *TraceCausalProjectionIntervalSet) Add(startTs, endTs float64) {
	if !traceCausalProjectionIntervalValid(startTs, endTs) {
		return
	}
	cur := TraceCausalProjectionInterval{StartTs: startTs, EndTs: endTs}
	merged := make([]TraceCausalProjectionInterval, 0, len(s.spans)+1)
	placed := false
	for _, span := range s.spans {
		switch {
		case span.EndTs < cur.StartTs:
			// Strictly before the new interval (spans are ascending).
			merged = append(merged, span)
		case cur.EndTs < span.StartTs:
			// Strictly after: the new interval settles first.
			if !placed {
				merged = append(merged, cur)
				placed = true
			}
			merged = append(merged, span)
		default:
			// Overlapping or touching: absorb into the new interval and keep
			// scanning — later spans may chain-merge.
			if span.StartTs < cur.StartTs {
				cur.StartTs = span.StartTs
			}
			if span.EndTs > cur.EndTs {
				cur.EndTs = span.EndTs
			}
		}
	}
	if !placed {
		merged = append(merged, cur)
	}
	s.spans = merged
}

// Spans returns a copy of the set's sorted disjoint intervals (copy so no
// consumer can bend the invariant from outside).
func (s TraceCausalProjectionIntervalSet) Spans() []TraceCausalProjectionInterval {
	if len(s.spans) == 0 {
		return nil
	}
	out := make([]TraceCausalProjectionInterval, len(s.spans))
	copy(out, s.spans)
	return out
}

// Empty reports whether the set holds no wall clock.
func (s TraceCausalProjectionIntervalSet) Empty() bool {
	return len(s.spans) == 0
}

// OverlapSeconds returns the total length (seconds) of the set's coverage
// INSIDE the given interval — the precise "already counted elsewhere" amount
// the N2 union deduction consumes. Invalid query interval → 0.
func (s TraceCausalProjectionIntervalSet) OverlapSeconds(startTs, endTs float64) float64 {
	if !traceCausalProjectionIntervalValid(startTs, endTs) {
		return 0
	}
	total := 0.0
	for _, span := range s.spans {
		lo, hi := span.StartTs, span.EndTs
		if lo < startTs {
			lo = startTs
		}
		if hi > endTs {
			hi = endTs
		}
		if hi > lo {
			total += hi - lo
		}
	}
	return total
}

// TotalSeconds returns the union length (seconds) of the whole set — the N4
// coverage-numerator union lower bound reads THIS, never a member sum.
func (s TraceCausalProjectionIntervalSet) TotalSeconds() float64 {
	total := 0.0
	for _, span := range s.spans {
		total += span.EndTs - span.StartTs
	}
	return total
}
