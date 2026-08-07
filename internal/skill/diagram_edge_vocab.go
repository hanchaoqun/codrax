package skill

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// BuildDiagramEdgeLabelVocabularyDoc renders the LLM-facing
// markdown bullet list of the canonical diagram-edge label
// vocabulary directly from the typed dictionary in
// internal/types/diagram_relation.go (single source of truth).
//
// SST red line: the keyword set lives ONLY in
// internal/types/diagram_relation.go. This helper reads that table
// at build time so a future edit to the dictionary flows through to
// the LLM prompt without manual sync. Evidence-shape mapping remains
// internal because relation_kind is the sole LLM-authored authority.
// Mirrors the pattern of analysis_contract.go's renderEnumTable —
// LLM-facing prose is generated from the typed authority, not
// hand-mirrored.
//
// Output shape (one bullet per non-Unknown relation kind, one
// trailing newline):
//
//   - relation `<kind>`: keywords `kw1`, `kw2`, ...
func BuildDiagramEdgeLabelVocabularyDoc() string {
	dict := types.DiagramRelationKeywords()
	grouped := make(map[types.DiagramRelationKind][]string)
	order := make([]types.DiagramRelationKind, 0)
	for _, e := range dict {
		if _, seen := grouped[e.Kind]; !seen {
			order = append(order, e.Kind)
		}
		grouped[e.Kind] = append(grouped[e.Kind], e.Keyword)
	}
	// Sort alphabetically so the rendered list is deterministic
	// across builds; the dictionary's scanning-priority order is an
	// internal detail that should not bleed into the LLM prompt.
	sort.Slice(order, func(i, j int) bool {
		return string(order[i]) < string(order[j])
	})

	var b strings.Builder
	for _, kind := range order {
		quoted := make([]string, 0, len(grouped[kind]))
		for _, kw := range grouped[kind] {
			quoted = append(quoted, fmt.Sprintf("`%s`", kw))
		}
		fmt.Fprintf(&b, "- relation `%s`: keywords %s\n",
			kind, strings.Join(quoted, ", "))
	}
	return b.String()
}

// BuildDiagramRelationKindList returns the canonical comma-separated
// list of typed RelationKind values, rendered for inline embedding in
// LLM-facing prompts (e.g. "`call` / `guard` / `import` / ..."). The
// list is sorted alphabetically for prompt-stability and rendered
// directly from types.AllDiagramRelationKinds() so the skill prompt
// cannot drift from the typed enum (SST red line).
//
// G3 (post_v2_runtime_gap_remediation, 2026-05-04) — replaces the
// hand-mirrored "call / guard / import / ..." string that would
// otherwise have to be kept in sync by hand.
func BuildDiagramRelationKindList() string {
	kinds := types.AllDiagramRelationKinds()
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, fmt.Sprintf("`%s`", string(k)))
	}
	sort.Strings(names)
	return strings.Join(names, " / ")
}

// BuildClaimFormList renders the canonical schema vocabulary from the same
// typed enum used by emit_answer_document. Prompt prose must not hand-copy a
// partial second enum: that contradiction increases model JSON failures and
// can make a schema-valid answer look impossible to repair.
func BuildClaimFormList() string {
	forms := types.AllClaimForms()
	names := make([]string, 0, len(forms))
	for _, form := range forms {
		names = append(names, fmt.Sprintf("`%s`", string(form)))
	}
	sort.Strings(names)
	return strings.Join(names, " / ")
}
