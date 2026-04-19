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
