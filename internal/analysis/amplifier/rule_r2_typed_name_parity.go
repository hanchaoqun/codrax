package amplifier

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// minAffixLen is the shortest common prefix or suffix that counts as
// a "shared parent" for R2's parallel-naming grouping. Empirically
// 4 chars filters out Go's two-letter prefix family ("Get", "Set",
// "io", "Err") that does not constitute a question-level subtopic
// boundary, while still admitting typical naming sets (Stage* /
// emit_ / Turn* / Agent*). Tuneable by experience; 5 is the next
// sensible step if 4 turns out to be too lenient.
const minAffixLen = 4

// minGroupSize is the minimum group size required to derive
// SubTopics. A "group of one" is just a single entity and not
// worth carving out as its own subtopic.
const minGroupSize = 2

// maxGroupSize caps how many entities one R2-derived SubTopic
// family can hold. Above 8 the family is almost always a global
// codebase pattern (Test* / Err*) that should not become a
// per-question subtopic axis.
const maxGroupSize = 8

// isIdentifierLike reports whether the trimmed surface looks like a
// code identifier (letter/underscore lead, then alnum/underscore/.
// dots permitted for path-like names such as `pkg.Symbol` or
// `cfg.section.key`). Pure alphabetic / unicode prose words ("the",
// "stage", "agent", "concept") fail this check because they
// typically come from analyzer-emitted concept lists rather than
// named code entities. Drops surfaces that have any whitespace or
// punctuation other than `.` / `_` so an emitted phrase can never
// pollute the affix-grouping pool.
func isIdentifierLike(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) == 0 {
		return false
	}
	for i, r := range t {
		if i == 0 {
			if !(r == '_' || isLetter(r)) {
				return false
			}
			continue
		}
		if !(r == '_' || r == '.' || isLetter(r) || isDigit(r)) {
			return false
		}
	}
	// Reject pure-lowercase prose words that pass the char-class
	// filter (e.g. "stage", "agent"). Heuristic: code identifiers
	// either contain at least one uppercase letter (CamelCase),
	// at least one underscore (snake_case), or at least one dot
	// (path-like). Pure lowercase with no separator is concept-prose.
	hasUpper := false
	hasSep := false
	for _, r := range t {
		if isLetter(r) && r >= 'A' && r <= 'Z' {
			hasUpper = true
		}
		if r == '_' || r == '.' {
			hasSep = true
		}
	}
	return hasUpper || hasSep
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// commonAffixGroups partitions the input entities into groups whose
// surfaces share a common prefix or suffix of at least minAffixLen
// characters. Groups with fewer than minGroupSize or more than
// maxGroupSize entries are dropped. The returned slice contains one
// member slice per qualifying group, each in the input's relative
// order.
//
// The implementation is naive O(N²) — fine for the small sizes (≤
// ~10 entities) that analyzer emits. Each entity goes into at most
// one group: we greedily assign by the longest shared prefix
// (preferred over suffix at ties).
func commonAffixGroups(entities []string) [][]string {
	if len(entities) < minGroupSize {
		return nil
	}
	cleaned := make([]string, 0, len(entities))
	for _, e := range entities {
		t := strings.TrimSpace(e)
		if !isIdentifierLike(t) {
			continue
		}
		cleaned = append(cleaned, t)
	}
	if len(cleaned) < minGroupSize {
		return nil
	}

	used := make([]bool, len(cleaned))
	var groups [][]string
	for i := 0; i < len(cleaned); i++ {
		if used[i] {
			continue
		}
		group := []string{cleaned[i]}
		used[i] = true
		for j := i + 1; j < len(cleaned); j++ {
			if used[j] {
				continue
			}
			if commonPrefixLen(cleaned[i], cleaned[j]) >= minAffixLen ||
				commonSuffixLen(cleaned[i], cleaned[j]) >= minAffixLen {
				group = append(group, cleaned[j])
				used[j] = true
			}
		}
		if len(group) >= minGroupSize && len(group) <= maxGroupSize {
			groups = append(groups, group)
		}
	}
	return groups
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func commonSuffixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[len(a)-1-i] != b[len(b)-1-i] {
			return i
		}
	}
	return n
}

// commonAffix picks a representative affix for an Observation's
// reason text. Returns the longest shared prefix or suffix across
// the group (whichever is longer). Used purely for telemetry; the
// rule's gate has already validated the group.
func commonAffix(group []string) string {
	if len(group) == 0 {
		return ""
	}
	pre := group[0]
	suf := group[0]
	for _, e := range group[1:] {
		// trim prefix
		n := commonPrefixLen(pre, e)
		if n < len(pre) {
			pre = pre[:n]
		}
		// trim suffix
		m := commonSuffixLen(suf, e)
		if m < len(suf) {
			suf = suf[len(suf)-m:]
		}
	}
	if len(pre) >= len(suf) {
		return pre
	}
	return suf
}

// r2TypedNameParitySubTopics fires when the LLM's analyzer emit
// shows a parallel-naming family (Stage* / emit_* / Agent*) but
// SubTopics is empty. Derives one SubTopic per entity in the
// qualifying family so downstream evidence-DAG construction treats
// them as distinct subtopics.
//
// See docs/design/analyzer_amplifier_layer.md §5 R2.
//
// Gates:
//
//  1. rm.SubTopics is empty (red line #2: never override
//     LLM-emitted SubTopics).
//  2. NOT (rm.Predicates.IsCategoryEnumeration AND
//     !rm.Predicates.IsCrossComponent). This precisely mirrors the
//     downstream axis_collapse gate in
//     internal/analysis/gate/coherence.go::R1.4. When the question
//     is a single-axis enumeration (IsCategoryEnumeration=true)
//     and is NOT cross-component, derived SubTopics that share a
//     repomap Domain WILL trigger axis_collapse and force the
//     analyzer into a retry storm. The right answer shape for
//     single-axis enumeration is empty SubTopics + finalizer
//     ordered_list, not affix-derived SubTopics. Empirical:
//     2026-05-05 qf_arch run-1 with 4 Stage* entities + LLM
//     cat=true + cross_component=false produced 4 same-domain
//     SubTopics → axis_collapse → 4 retries exhausted → eval FAIL.
//     Safe to skip here because R1 only flips IsCategoryEnumeration
//     to true when entities ≥ 2; if the user wanted multi-subtopic
//     they would have set IsCrossComponent=true (which legitimately
//     spans ≥ 2 subsystems and bypasses axis_collapse).
//  3. distinctEntityCount(rm.AnalyzerHints.Entities) ≥ 2
//     (an empty or single-entity model has nothing to group).
//  4. commonAffixGroups returns at least one qualifying group.
//
// Source choice — like R1, R2 reads rm.AnalyzerHints.Entities
// rather than rm.TermGraph.Canonical. The original design used
// TermGraph (with its TermKind filter) but TermGraph is populated
// from RawRequest surfaces only, so Chinese / natural-language
// questions never expose LLM-emitted entity names there.
// AnalyzerHints.Entities is the direct typed slot for "subjects
// the LLM identified". The TermKind filter is approximated by
// isIdentifierLike (CamelCase / snake_case / dot-path shape) —
// enough to drop concept words like "stage" / "agent" without
// requiring a dual-gate normalizer pass.
//
// Action: for EVERY entity in EVERY qualifying group, append a
// SubTopic{Summary: entity, Entities: [entity]}. So a Stage*
// family of four becomes four SubTopics. This matches the design
// doc §5 R2 action: "为每个共享 parent 的 entity 生成
// SubTopic{Summary: <Surface>, Entities: [<Surface>]}". The
// truncation block in buildAnalysisIR caps total SubTopics at 5,
// which sets a soft upper bound across all groups.
func r2TypedNameParitySubTopics(in types.RequestModel, out *types.RequestModel) *Observation {
	if len(out.SubTopics) > 0 {
		return nil
	}
	// Axis-collapse alignment: skip when the downstream gate would
	// reject any same-domain SubTopics we derive. See gate #2 in
	// the doc above and internal/analysis/gate/coherence.go R1.4.
	if out.Predicates.IsCategoryEnumeration && !out.Predicates.IsCrossComponent {
		return nil
	}
	if distinctEntityCount(out.AnalyzerHints.Entities) < minGroupSize {
		return nil
	}
	groups := commonAffixGroups(out.AnalyzerHints.Entities)
	if len(groups) == 0 {
		return nil
	}

	var derived []types.SubTopic
	var groupAffixes []string
	for _, g := range groups {
		groupAffixes = append(groupAffixes, commonAffix(g))
		for _, entity := range g {
			derived = append(derived, types.SubTopic{
				Summary:  entity,
				Entities: []string{entity},
			})
		}
	}
	if len(derived) == 0 {
		return nil
	}
	out.SubTopics = derived
	return &Observation{
		Rule:   "R2_typed_name_parity_subtopics",
		Field:  "sub_topics",
		Before: "0",
		After:  fmt.Sprintf("%d", len(derived)),
		Reason: fmt.Sprintf("AnalyzerHints.Entities has %d affix-grouped families: %s",
			len(groups), strings.Join(groupAffixes, ", ")),
	}
}

func init() {
	preCompileRules = append(preCompileRules, r2TypedNameParitySubTopics)
}
