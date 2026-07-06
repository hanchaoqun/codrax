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
// the projection-wide span guard pinned by traceCausalProjectionSpansOverlap
// since R4: StartTs > 0 && EndTs > StartTs. Invalid intervals are ignored
// (fail-open: an absent interval never manufactures overlap and never
// deducts value), never guessed.

// TraceCausalProjectionInterval is one ts interval in seconds.
type TraceCausalProjectionInterval struct {
	StartTs float64
	EndTs   float64
}

// traceCausalProjectionIntervalValid is the shared validity guard: a real
// trace interval has a positive start and a strictly later end. Zero-length
// intervals are invalid — they carry no wall clock to union or overlap.
func traceCausalProjectionIntervalValid(startTs, endTs float64) bool {
	return startTs > 0 && endTs > startTs
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
