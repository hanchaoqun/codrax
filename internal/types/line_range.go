package types

import "sort"

// LineRange is a 1-based inclusive [Start, End] line interval. It is
// the row-level companion to the file-level ReadSet on EvidenceClosure:
// CGEC I1 chain promotion uses HasReadLine to answer "is this anchor
// line inside a range we already fetched?" instead of only "is this
// file in the set?". Start == End represents a single line. The zero
// value is an invalid empty range (callers must set Start ≥ 1).
type LineRange struct {
	Start, End int
}

// mergeLineRanges sorts the input by Start, drops malformed entries
// (Start ≤ 0, End < Start), and coalesces any overlapping or adjacent
// ranges. Adjacent pairs ([10, 20], [21, 30]) are merged into [10, 30]
// so the search path in rangesContain has fewer buckets and so the
// diagnostic dumps are easier to read. Idempotent on already-merged
// input. Returns a freshly allocated slice; the caller may keep or
// discard the input.
func mergeLineRanges(in []LineRange) []LineRange {
	if len(in) == 0 {
		return nil
	}
	cleaned := make([]LineRange, 0, len(in))
	for _, r := range in {
		if r.Start <= 0 || r.End < r.Start {
			continue
		}
		cleaned = append(cleaned, r)
	}
	if len(cleaned) == 0 {
		return nil
	}
	sort.Slice(cleaned, func(i, j int) bool {
		if cleaned[i].Start != cleaned[j].Start {
			return cleaned[i].Start < cleaned[j].Start
		}
		return cleaned[i].End < cleaned[j].End
	})
	out := make([]LineRange, 0, len(cleaned))
	cur := cleaned[0]
	for i := 1; i < len(cleaned); i++ {
		next := cleaned[i]
		if next.Start <= cur.End+1 {
			if next.End > cur.End {
				cur.End = next.End
			}
			continue
		}
		out = append(out, cur)
		cur = next
	}
	out = append(out, cur)
	return out
}

// rangesContain reports whether `line` is covered by any range in
// `sorted`. Assumes the slice is the output of mergeLineRanges
// (sorted, non-overlapping). Binary search gives O(log n) lookup — the
// chain promotion enforcer calls this once per chain anchor per round.
func rangesContain(sorted []LineRange, line int) bool {
	if line <= 0 || len(sorted) == 0 {
		return false
	}
	i := sort.Search(len(sorted), func(i int) bool {
		return sorted[i].End >= line
	})
	if i >= len(sorted) {
		return false
	}
	return sorted[i].Start <= line
}

// cloneLineRanges returns a defensive copy so callers cannot mutate
// closure internals after reading them out.
func cloneLineRanges(src []LineRange) []LineRange {
	if len(src) == 0 {
		return nil
	}
	out := make([]LineRange, len(src))
	copy(out, src)
	return out
}

// LineToReadFileOffset converts a 1-based line number into the
// 0-based offset value the read_file tool expects. Used by every
// system-emitted "Use read_file(offset=N)" suggestion so a 1-based
// LineRange demand (e.g. {Start: 1322, End: 1322}) maps to the
// correct read_file invocation that actually fetches line 1322.
//
// 2026-05-10 forensic anchor: customer-reported phantom forced-read
// loop on context.go:1322. The system was emitting offset=1322 for
// 1-based line 1322; read_file then computed sliceStart=1322 and
// returned banner "showing lines 1323-…", leaving line 1322 in the
// missing set forever. The LLM correctly identified the off-by-one
// at iter=17 ("I've been reading offset=1322 which gives line
// 1323") but kept following the system's wrong instructions until
// the read_file budget exhausted.
//
// Clamps line ≤ 0 to 0 so the helper is safe on uninitialised
// LineRange.Start values without panicking.
func LineToReadFileOffset(line int) int {
	if line <= 0 {
		return 0
	}
	return line - 1
}
