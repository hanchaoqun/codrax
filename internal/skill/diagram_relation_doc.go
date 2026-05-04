package skill

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// diagram_relation_doc.go — B3 v3 (2026-05-04). Single-source-of-truth
// renderer for the diagram-edge contract section in the
// answer-document-skill prompt.
//
// Pre-v3, the prompt hand-rendered three coupled fragments:
//   1. label keyword vocabulary list (BuildDiagramEdgeLabelVocabularyDoc)
//   2. typed RelationKind list       (BuildDiagramRelationKindList)
//   3. surrounding prose teaching the relation→claim_form mapping +
//      edge_anchors schema
//
// All three derive from the same authority tables in
// internal/types/diagram_relation.go (DiagramRelationKeywords +
// AllDiagramRelationKinds + ClaimFormForRelation). Adding a new
// relation kind / keyword / claim_form mapping previously required
// editing prompt prose by hand. v3 collapses the three into one
// generated section so prompt content tracks the typed authority
// automatically.
//
// Output sections:
//   1. Typed-first PREFERRED guidance (relation_kind primary surface)
//   2. edge_anchors[] schema description
//   3. Legacy label vocabulary (compatibility fallback)
//
// The output is rendered into the answer-document-skill OutputFormat
// at the position where ## Diagram-edge label vocabulary lived
// pre-v3. Both LLM-facing surfaces (defaults.go OutputFormat + any
// reviewer prompt) read the same generated string.

// BuildDiagramRelationContractDoc renders the canonical diagram-edge
// contract section. The output is markdown — the caller is expected
// to wrap with the section heading
// (types.SectionDiagramEdgeLabelVocabulary).
//
// Section flow:
//   - Lead paragraph: typed declaration is the authoritative surface;
//     keep label vocabulary aligned for reader clarity.
//   - edge_anchors[] schema: from_node / to_node / relation_kind /
//     claim_form fields, derived dynamically from
//     AllDiagramRelationKinds + ClaimFormForRelation.
//   - Legacy compatibility: when an edge label is provided without a
//     typed relation_kind, the validator infers from label vocabulary.
//     Generated bullet list reads from DiagramRelationKeywords so the
//     rendered prompt cannot drift from the typed authority.
func BuildDiagramRelationContractDoc() string {
	var b strings.Builder

	b.WriteString("Diagram edges declare a typed relation between two nodes. The TYPED declaration on `edge_anchors[]` is the authoritative surface; rendered edge labels exist for human readers.\n\n")

	b.WriteString("PREFERRED: declare the relation directly via `edge_anchors[]` on the diagram block (or any block whose items describe the endpoints). Each entry is `{from_node, to_node, relation_kind?, claim_form?}`:\n\n")
	b.WriteString("- `relation_kind` (PREFERRED): the typed relation. One of " + BuildDiagramRelationKindList() + ". Set this and the validator treats your declaration as ground truth — the rendered label is free prose for readability.\n")
	b.WriteString("- `claim_form` matches the relation's expected typed evidence shape (see the relation→claim_form mapping below).\n")
	b.WriteString("- `from_node` / `to_node` are the verbatim node identifiers as they appear in the diagram body.\n\n")

	b.WriteString("Each typed relation maps to a claim_form the supporting evidence must declare:\n\n")
	for _, kind := range types.AllDiagramRelationKinds() {
		claim := types.ClaimFormForRelation(kind)
		var qualifier string
		if claim == types.ClaimUnknown {
			qualifier = "block-level facet, no edge claim_form required"
		} else {
			qualifier = fmt.Sprintf("claim_form `%s`", claim)
		}
		fmt.Fprintf(&b, "- relation `%s` (%s)\n", kind, qualifier)
	}
	b.WriteString("\n")

	b.WriteString("Example: for the labelled edge `Auth -->|invoke| Worker` (relation `call`, expected claim_form `call_edge`), emit on the diagram block:\n\n")
	b.WriteString("```json\nedge_anchors: [{from_node: \"Auth\", to_node: \"Worker\", relation_kind: \"call\", claim_form: \"call_edge\"}]\n```\n\n")
	b.WriteString("`edge_anchors[]` is a separate top-level array — it NEVER lives inside a `claim_use` object.\n\n")

	b.WriteString("LEGACY ALTERNATIVE: when no `relation_kind` is declared, the validator infers the relation from edge label vocabulary (case-folded substring match). Use this only when label wording is the authoritative surface — typed declaration is preferred for any new content. Recognised keywords (kept for back-compat with label-only diagrams):\n\n")
	b.WriteString(BuildDiagramEdgeLabelVocabularyDoc())
	b.WriteString("\n")

	b.WriteString("An UNLABELLED edge (no `|...|` block, no trailing `: msg`) is legal — its endpoints only need to be referenced elsewhere in the answer. A label that matches NONE of the keywords above also counts as label-free and does not require a claim_use anchor.\n\n")

	b.WriteString("Some diagram contracts also require a minimum number of edges of a particular relation kind. Typed declarations always count toward the minimum; label-only declarations also count but the validator emits an advisory rendering the answer with a `relation_kind`-typed alternative.\n")

	return b.String()
}
