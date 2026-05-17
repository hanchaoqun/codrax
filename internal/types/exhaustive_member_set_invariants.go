package types

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// ExhaustiveMemberSetDefaultThreshold is the default principal-member
// count above which the deterministic reviewer replaces the
// self-consistency + semantic-quality LLM dispatchers. Calibrated
// from u8b (94 enum types) where the two LLM reviewers exceeded the
// 1200s wall budget despite the answer being structurally correct.
//
// Zero means deterministic-only is disabled; configurable via
// codrax.yaml :: pipeline_exhaustive_deterministic_review_threshold.
const ExhaustiveMemberSetDefaultThreshold = 30

// LargestExhaustiveMemberSet returns the model-authored
// member_set aggregate fact with the largest, self-consistent
// member list above threshold. "Self-consistent" means
// value == len(members) — the closed-set typed signature the model
// uses to declare a complete principal member slate. Below
// threshold or when no such fact exists, returns (zero, false).
//
// This is the typed-only entrance to the deterministic reviewer.
// It deliberately does NOT consider total_count / unique_count /
// grouped_count aggregates: those declare a count but may have
// samples in members rather than the full slate, so a deterministic
// set-equality check on them would be wrong.
func LargestExhaustiveMemberSet(facts []AnswerAggregateFact, threshold int) (AnswerAggregateFact, bool) {
	if threshold <= 0 {
		return AnswerAggregateFact{}, false
	}
	var best AnswerAggregateFact
	bestSize := 0
	for _, fact := range facts {
		if fact.Kind != AnswerAggregateMemberSet {
			continue
		}
		size := len(fact.Members)
		if size < threshold || size <= bestSize {
			continue
		}
		// value must equal len(members) — exhaustive signature.
		v := strings.TrimSpace(fact.Value)
		if v == "" || v != strconv.Itoa(size) {
			continue
		}
		best = fact
		bestSize = size
	}
	if bestSize == 0 {
		return AnswerAggregateFact{}, false
	}
	return best, true
}

// ExhaustiveMemberCoverage holds the deterministic projection of an
// exhaustive aggregate fact against the rendered answer document. It
// is consumed by the deterministic reviewer to emit structural
// violations without an LLM dispatch.
type ExhaustiveMemberCoverage struct {
	MemberCount         int
	PrincipalItemCount  int
	MissingMembers      []string // typed members with no matching principal item
	UnexpectedItems     []string // principal items not in the typed member set
	DuplicateCitations  []int    // citation_ref values appearing more than once
	InvalidCitationRefs []int    // citation_ref outside [0, len(citations))
}

// HasFailures reports whether the deterministic coverage projection
// surfaced any structural mismatch worth a violation.
func (c ExhaustiveMemberCoverage) HasFailures() bool {
	return len(c.MissingMembers) > 0 ||
		len(c.UnexpectedItems) > 0 ||
		len(c.DuplicateCitations) > 0 ||
		len(c.InvalidCitationRefs) > 0
}

// ComputeExhaustiveMemberCoverage runs the deterministic invariants
// for the largest exhaustive member set against a rendered
// AnswerDocumentV2. All comparisons are typed-only:
//   - members ↔ principal item.labels: set equality after
//     ExhaustiveMemberKey normalization (lowercase + collapse internal
//     whitespace; same normalization the aggregate parser uses).
//   - citation_ref values: must be in range and unique within the
//     principal block carrying the member rows.
//
// No prose scan: the comparison operates on typed `label` field and
// integer `citation_ref` field of model-emitted blocks.
func ComputeExhaustiveMemberCoverage(doc *AnswerDocumentV2, fact AnswerAggregateFact) ExhaustiveMemberCoverage {
	cov := ExhaustiveMemberCoverage{MemberCount: len(fact.Members)}
	if doc == nil || len(fact.Members) == 0 {
		return cov
	}
	memberKeys := make(map[string]string, len(fact.Members))
	memberTailIndex := make(map[string]string, len(fact.Members))
	for _, m := range fact.Members {
		key := exhaustiveMemberKey(m)
		if key == "" {
			continue
		}
		memberKeys[key] = strings.TrimSpace(m)
		// Index by qualified tail so a member "pkg.Foo" is also
		// reachable by bare label "Foo". Skip when tail collides
		// to keep the deterministic check strict-typed: ambiguous
		// tails return the same key and won't satisfy any member.
		if tail := exhaustiveMemberTail(key); tail != "" {
			if existing, ok := memberTailIndex[tail]; ok && existing != key {
				memberTailIndex[tail] = ""
				continue
			}
			memberTailIndex[tail] = key
		}
	}
	if len(memberKeys) == 0 {
		return cov
	}

	citationCount := len(doc.Citations)
	citationSeen := make(map[int]int, citationCount)
	matched := make(map[string]bool, len(memberKeys))
	var unexpected []string

	for _, block := range doc.Blocks {
		if !exhaustiveBlockCarriesPrincipalMembers(block) {
			continue
		}
		for _, item := range block.Items {
			labels := exhaustiveItemMemberSurfaces(item)
			if len(labels) == 0 {
				continue
			}
			cov.PrincipalItemCount++
			if item.CitationRef >= 0 {
				if item.CitationRef >= citationCount {
					cov.InvalidCitationRefs = append(cov.InvalidCitationRefs, item.CitationRef)
				} else {
					citationSeen[item.CitationRef]++
				}
			}
			itemMatched := false
			for _, label := range labels {
				key := exhaustiveMemberKey(label)
				if key == "" {
					continue
				}
				if _, ok := memberKeys[key]; ok {
					matched[key] = true
					itemMatched = true
					break
				}
				// Qualified-tail relaxation in both directions, so
				// the deterministic checker tolerates the C++/Java/
				// Cangjie/Go selector renderings the aggregate parser
				// already accepts.
				//   item="Foo" + member="pkg.Foo" → tailIndex[Foo]=pkg.foo
				//   item="pkg.Foo" + member="Foo" → exhaustiveMemberTail(pkg.foo)=foo
				if memberKey, ok := memberTailIndex[key]; ok && memberKey != "" {
					matched[memberKey] = true
					itemMatched = true
					break
				}
				if tail := exhaustiveMemberTail(key); tail != "" {
					if _, ok := memberKeys[tail]; ok {
						matched[tail] = true
						itemMatched = true
						break
					}
				}
			}
			if itemMatched {
				continue
			}
			unexpected = append(unexpected, labels[0])
		}
	}

	for key, orig := range memberKeys {
		if !matched[key] {
			cov.MissingMembers = append(cov.MissingMembers, orig)
		}
	}
	sort.Strings(cov.MissingMembers)
	cov.UnexpectedItems = dedupSortedStrings(unexpected)
	cov.DuplicateCitations = duplicateCitationRefs(citationSeen)
	sort.Ints(cov.InvalidCitationRefs)
	return cov
}

func exhaustiveItemMemberSurfaces(item AnswerBlockItem) []string {
	var out []string
	if label := strings.TrimSpace(item.Label); label != "" {
		out = append(out, label)
	}
	if len(out) == 0 {
		for _, cell := range item.Cells {
			cell = strings.TrimSpace(cell)
			if cell == "" {
				continue
			}
			out = append(out, cell)
			break
		}
	}
	return out
}

func exhaustiveBlockCarriesPrincipalMembers(block AnswerBlock) bool {
	switch block.Kind {
	case BlockOrderedList, BlockBulletList, BlockTable:
	default:
		return false
	}
	if block.SurfaceRole == SurfacePrincipal {
		return true
	}
	for _, facet := range block.FacetIDs {
		if strings.TrimSpace(facet) == string(FacetEnumerationItem) {
			return true
		}
	}
	// Enumeration answers frequently use the principal list block
	// before annotations are filled in. Coverage check treats any
	// list/table block as a potential member carrier; mismatches are
	// still flagged because the principal slate is the typed member
	// set, not the block facet annotation.
	return true
}

// exhaustiveMemberKey canonicalizes a label/member string for set
// comparison. It uses the same normalization the aggregate parser
// already trusts elsewhere: trim, collapse internal whitespace,
// lowercase. Code identifiers, file paths, and relation labels
// (`pkg → Entry`) all flow through the same key.
func exhaustiveMemberKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Join(strings.Fields(s), " ")
	return strings.ToLower(s)
}

// exhaustiveMemberTail returns the trailing identifier segment after
// the rightmost qualifier separator. It accepts the same separators
// the existing aggregate relation parser accepts: `.`, `::`, `/`,
// `->`, `→`, `=>`. Returns "" when no separator is found.
func exhaustiveMemberTail(key string) string {
	if key == "" {
		return ""
	}
	for _, sep := range []string{"->", "=>", "::", "→", "/", "."} {
		if idx := strings.LastIndex(key, sep); idx >= 0 && idx+len(sep) < len(key) {
			tail := strings.TrimSpace(key[idx+len(sep):])
			if tail != "" {
				return tail
			}
		}
	}
	return ""
}

func dedupSortedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func duplicateCitationRefs(seen map[int]int) []int {
	var out []int
	for ref, n := range seen {
		if n > 1 {
			out = append(out, ref)
		}
	}
	sort.Ints(out)
	return out
}

// FormatExhaustiveMissingMembersHint renders a bounded user-vocab
// summary of missing typed members for prompt/repair surfaces. The
// system never invents members; this only echoes model-authored
// typed values.
func FormatExhaustiveMissingMembersHint(members []string, maxShown int) string {
	if len(members) == 0 {
		return ""
	}
	if maxShown <= 0 {
		maxShown = 6
	}
	shown := members
	suffix := ""
	if len(members) > maxShown {
		shown = members[:maxShown]
		suffix = fmt.Sprintf(" (+%d more)", len(members)-maxShown)
	}
	quoted := make([]string, 0, len(shown))
	for _, m := range shown {
		quoted = append(quoted, fmt.Sprintf("%q", m))
	}
	return strings.Join(quoted, ", ") + suffix
}

// exhaustiveMemberSetKnownPath is unused but kept here as a guard
// against future drift: any path-shaped member must NOT trigger the
// deterministic-only switch if downstream code is going to consume it
// as a citation rather than a label. ext check is centralized in
// HasCodeOrConfigPathSuffix.
func exhaustiveMemberSetKnownPath(member string) bool {
	member = strings.TrimSpace(member)
	if member == "" {
		return false
	}
	if !strings.ContainsAny(member, "/.\\") {
		return false
	}
	return HasCodeOrConfigPathSuffix(path.Clean(member))
}
