package authority

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// HighestAuthorityFor scans the evidence pool for items whose anchor
// matches (file, line) and returns the STRONGEST AuthorityCeiling
// observed. Used by the renderer (and by the C6 contract checker) to
// answer "for this prose citation, what's the strongest claim we may
// state?" — a citation backed by a factual + a conditional item still
// permits factual prose, since at least one underlying anchor supports
// the strong claim.
//
// Strongest = highest authorityRank under types.WeakerOf's strict
// ordering.
//
// Inputs:
//   - items: usually bus.Mutable.EmittedEvidence() (defensive copy
//     already returned by EmittedEvidence so caller need not clone).
//   - file: repo-relative or normalisable path; matched by
//     normalizePath equality with item.Source.
//   - line: 1-indexed line number; matched against item.LineStart with
//     small slack to absorb mismatches between citation ref and
//     evidence anchor.
//
// Returns AuthorityUnknown when no items match (callers may treat
// Unknown as factual-equivalent for legacy passthrough).
func HighestAuthorityFor(items []types.EvidenceItem, file string, line int) types.AuthorityCeiling {
	target := normalizePath(file)
	if target == "" {
		return types.AuthorityUnknown
	}
	best := types.AuthorityUnknown
	matched := false
	for _, item := range items {
		if normalizePath(item.Source) != target {
			continue
		}
		if line > 0 && item.LineStart > 0 {
			gap := line - item.LineStart
			if gap < 0 {
				gap = -gap
			}
			// Allow modest slack — a citation pointing at line N may
			// reference an anchor at N±2 (off-by-one in the LLM's
			// line indexing, banner truncation, whitespace lines).
			if gap > 4 {
				continue
			}
		}
		if !matched {
			best = item.Authority
			matched = true
			continue
		}
		if isStrongerThan(item.Authority, best) {
			best = item.Authority
		}
	}
	return best
}

// LowestAuthorityFor is the dual: returns the WEAKEST AuthorityCeiling
// observed across matching items. Used when the contract checker
// needs to surface "this citation is at least this weak" — e.g.
// catching prose that uses strong language for an item whose only
// supporting evidence is conditional.
//
// Returns AuthorityUnknown when no items match.
func LowestAuthorityFor(items []types.EvidenceItem, file string, line int) types.AuthorityCeiling {
	target := normalizePath(file)
	if target == "" {
		return types.AuthorityUnknown
	}
	weakest := types.AuthorityUnknown
	matched := false
	for _, item := range items {
		if normalizePath(item.Source) != target {
			continue
		}
		if line > 0 && item.LineStart > 0 {
			gap := line - item.LineStart
			if gap < 0 {
				gap = -gap
			}
			if gap > 4 {
				continue
			}
		}
		if !matched {
			weakest = item.Authority
			matched = true
			continue
		}
		weakest = types.WeakerOf(weakest, item.Authority)
	}
	return weakest
}

// AuthorityHistogram returns a count of items by AuthorityCeiling
// value. Used by per-task summary logging and by the answer_taxonomy
// reviewer to decide whether the run produced enough hedged evidence
// to warrant a "drift_bounded answer" pattern entry. Items with
// AuthorityUnknown are counted under the empty-string key.
func AuthorityHistogram(items []types.EvidenceItem) map[types.AuthorityCeiling]int {
	out := make(map[types.AuthorityCeiling]int)
	for _, item := range items {
		out[item.Authority]++
	}
	return out
}

// isStrongerThan reports whether a outranks b under the strict
// ordering (factual > conditional > historical > illustrative). The
// AuthorityUnknown zero value is treated as factual-equivalent for
// the "stronger" comparison (legacy items shouldn't be ignored when
// computing best-case authority).
func isStrongerThan(a, b types.AuthorityCeiling) bool {
	return rank(a) > rank(b)
}

func rank(a types.AuthorityCeiling) int {
	switch a {
	case types.AuthorityIllustrative:
		return 1
	case types.AuthorityHistorical:
		return 2
	case types.AuthorityConditional:
		return 3
	case types.AuthorityFactual:
		return 4
	}
	// AuthorityUnknown — fail-safe to factual-equivalent rank to
	// preserve byte-identical legacy behaviour.
	return 4
}
