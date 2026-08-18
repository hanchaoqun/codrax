package skill

import (
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
//   3. surrounding prose teaching the edge_anchors schema
//
// All three derive from the same authority tables in
// internal/types/diagram_relation.go. Adding a new relation kind or
// keyword previously required editing prompt prose by hand. v3
// collapses the three into one generated section so prompt content
// tracks the typed authority automatically.
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
//   - edge_anchors[] schema: required from_node / to_node / relation_kind,
//     plus an optional producer-supplied from_identity / to_identity pair;
//     the relation enum is derived from AllDiagramRelationKinds.
//   - Legacy compatibility: when an edge label is provided without a
//     typed relation_kind, the validator infers from label vocabulary.
//     Generated bullet list reads from DiagramRelationKeywords so the
//     rendered prompt cannot drift from the typed authority.
func BuildDiagramRelationContractDoc() string {
	var b strings.Builder

	b.WriteString("Diagram edges declare a typed relation between two nodes. The TYPED declaration on `edge_anchors[]` is the authoritative surface; rendered edge labels exist for human readers.\n\n")

	b.WriteString("PREFERRED: declare the relation directly via `edge_anchors[]` on the diagram block (or any block whose items describe the endpoints). Each entry has the three required core fields `{from_node, to_node, relation_kind}`. The schema also supports the paired exact selectors `{from_identity, to_identity}` and the reader-facing `visible_label`; they are optional outside a stricter typed family contract and required when that contract says so:\n\n")
	b.WriteString("- `relation_kind`: the sole typed relation authority. One of " + BuildDiagramRelationKindList() + ". Do not add another field for its evidence shape. The rendered label remains presentation text for human readers.\n")
	b.WriteString("- `from_node` / `to_node` are the verbatim node identifiers as they appear in the diagram body.\n")
	b.WriteString("- `from_identity` / `to_identity` are exact endpoint selectors supplied by a typed capsule. They are not visible copy and do not prove a relation. Preserve both or omit both only outside a stricter contract; a grounded standalone call-chain contract requires the pair. They let visible labels use concise business/domain wording without changing evidence identity.\n")
	b.WriteString("- `visible_label` is the reader-facing edge wording. It is optional on generic relation anchors and required by the grounded standalone call-chain contract. It never changes endpoint identity or proves a relation.\n\n")
	b.WriteString(types.GroundedStandaloneCallChainRelationOwnershipContract + " The validator checks the same evidence kernel used for diagrams; changing block kind never changes relation truth.\n\n")
	b.WriteString("For `type_relation`, preserve the exact declared-type direction: subtype / implementing type / embedded type `->` superclass / interface / trait / protocol / embedded contract. It is the shared relation for inheritance, implementation, conformance, and embedding across supported languages. `type_relation` is typed-only and requires a same-direction parser-authored relationship; rendered words such as inherits or implements never mint that authority.\n\n")
	b.WriteString("For value/factory flow, choose the direction before drawing: `assignment` is the binding view, assigned receiver `->` bound value/type; `data_flow` is the execution-direction view of the same exact assignment/initializer, RHS value/source `->` LHS receiver; `return` is returning function `->` returned value/type. All are typed-only. `assignment` and `data_flow` require one citable exact assignment endpoint tuple, and `return` requires one citable return EvidenceItem. `data_flow` does not authorize conceptual, temporal, or cross-statement bridges. These relations must not be relabelled as `call`.\n\n")
	b.WriteString("For deferred/dynamic execution, use `callback` only when an exact source line passes a callable value as an argument to a receiving API. Its direction is receiving API/dispatcher `->` passed callable, and it proves handoff only—not that the callback later executed. This one shape covers function values, method references, function pointers, and closures across the supported languages; use `call` separately for every direct invocation.\n\n")
	b.WriteString("For an ordinary value passed as one complete call argument, use `argument_flow` only from exact `argument` evidence. Its direction is byte-exact complete argument expression `->` receiving API. It proves only that source-level handoff; it does not prove callee-side storage, mutation, execution, or a later return. Callable values remain `callback`.\n\n")
	b.WriteString("For branch logic, keep two evidence shapes separate. `guard` is only enclosing callable `->` condition and does not prove a branch-body effect. `control_flow` is exact parser-proved branch arm `->` call/return/assignment/exit effect. `control_flow` is typed-only: use only the exact endpoints supplied by a typed authoring capsule; source words, prose, adjacency, and rendered labels never mint it. This distinction is shared across all supported executable languages, including ArkTS and Cangjie.\n\n")

	b.WriteString("Example: for the labelled edge `Auth -->|invoke| Worker` (relation `call`), emit this valid JSON fragment on the diagram block:\n\n")
	b.WriteString("```json\n{\"edge_anchors\":[{\"from_node\":\"Auth\",\"to_node\":\"Worker\",\"relation_kind\":\"call\"}]}\n```\n\n")
	b.WriteString("`edge_anchors[]` is a separate top-level array — it NEVER lives inside a `claim_use` object.\n\n")
	b.WriteString("In every non-runtime-trace answer, an explicit `relation_kind=call` asserts a direct invocation and must preserve the exact direction of one grounded call-site EvidenceItem. This rule follows the typed relation rather than the question family, so a generic explanation, architecture diagram, comparison, or sibling edge carrier cannot use `call` to bypass source authority. When an arrow means a declared type relation, registration/binding, callback handoff, argument handoff, branch effect, assignment, data flow, return, observation, containment, or ordering instead of invocation, use the matching non-call relation. `type_relation`, `register`, `callback`, `argument_flow`, `control_flow`, `assignment`, `data_flow`, and `return` are typed-only and require their exact relationship evidence; rendered edge-label words never mint that authority. Runtime/root-cause trace diagrams use their separate typed causal-relation authority and do not enter this source-call contract.\n\n")
	b.WriteString("For a grounded call-chain `sequenceDiagram`, each invocation arrow (`caller->>callee: operation(...)`) needs its same-direction `relation_kind=call` anchor. A dashed reply `callee-->>caller` is a response/return, not a reverse call. For example, `callee-->>caller: result` is a reply only when it mirrors an invocation already drawn in the opposite direction; it is not a reverse source-code call, so do NOT add a call anchor for that reply. A standalone `-->>` edge does not self-declare as a reply and remains subject to call authority. Method-qualified participant labels are the clearest form. Short class/actor participants are also valid when the message begins with the exact callee operation and that owner+operation tuple resolves to one unique typed call edge; ambiguous class-only edges fail closed.\n\n")
	b.WriteString(types.GroundedSourceDiagramEdgeOwnershipContract + "\n\n")
	b.WriteString(types.GroundedSourceDiagramRelationEvidenceContract + "\n\n")
	b.WriteString("A `call_dag` invocation needs `relation_kind=call`; a real non-call edge (guard/control_flow/import/precedence/contain/type_relation/observe/register/callback/argument_flow/assignment/data_flow/return) needs its own honest typed relation instead. An edge label is presentation text and never mints typed authority. If structured principal-path items select both endpoints of a grounded typed call, keep that directed call visible in the diagram. Supporting calls outside the selected principal path do not have to be drawn.\n\n")

	b.WriteString("LEGACY ALTERNATIVE outside the strict grounded call-chain contract: when no `relation_kind` is declared, relation-vocabulary validation may infer a display relation from edge label vocabulary (case-folded substring match). This never creates an `edge_anchors[]` entry and never satisfies grounded invocation authority. Use it only when label wording is the authoritative display surface — typed declaration is preferred for any new content. Recognised keywords (kept for back-compat with label-only diagrams):\n\n")
	b.WriteString(BuildDiagramEdgeLabelVocabularyDoc())
	b.WriteString("\n")

	b.WriteString("Outside a strict grounded source call-chain diagram, an UNLABELLED edge (no `|...|` block, no trailing `: msg`) is legal — its endpoints only need to be referenced elsewhere in the answer. A label that matches NONE of the keywords above also counts as label-free and does not require an edge anchor. Inside a grounded source call-chain diagram, choose and declare the honest relation for every visible body edge; the only metadata-free exception is a dashed sequence reply structurally paired with its forward invocation.\n\n")

	b.WriteString("Some diagram contracts also require a minimum number of edges of a particular relation kind. Typed declarations always count toward the minimum; label-only declarations also count but the validator emits an advisory rendering the answer with a `relation_kind`-typed alternative.\n")

	return b.String()
}

// BuildDiagramEdgeAnchorWorkflowRule renders the compact workflow-tier copy
// from the same typed ownership sentence used by the full relation contract
// and canonical tool schema. The remainder explains authoring mechanics; it
// does not redefine applicability.
func BuildDiagramEdgeAnchorWorkflowRule() string {
	return "`edge_anchors` is the OPTIONAL block-level array for diagram-edge typed anchors. " +
		types.GroundedSourceDiagramEdgeOwnershipContract + " " +
		types.GroundedSourceDiagramRelationEvidenceContract + " " +
		types.GroundedStandaloneCallChainRelationOwnershipContract + " " +
		"Use it when this block contributes evidence about a directed relation in a diagram (typically when the block IS the diagram, or when its items describe edge endpoints of a diagram in a sibling block). Each entry has three required core fields: `{from_node: string, to_node: string, relation_kind: <one of " + BuildDiagramRelationKindList() + ">}`. The schema also supports the paired exact selectors `{from_identity: string, to_identity: string}` and the reader-facing `{visible_label: string}`; they are optional on generic anchors and required when the grounded standalone call-chain contract says so. When a diagram exists, from_node/to_node MUST be its verbatim node identifiers. Without a diagram, use the reader endpoint labels required by the standalone call-chain contract. Do not add claim_form or evidence fields here. Choose the semantic relation before drawing the arrow: call is a direct invocation; callback is a receiving API -> callable-value handoff and does not prove execution; argument_flow is one byte-exact complete non-callable argument -> receiving API and proves no callee-side use; guard is enclosing callable -> condition only; control_flow is exact parser-proved branch arm -> effect; assignment is assigned receiver -> bound value/type; data_flow is the same exact assignment in RHS value/source -> LHS receiver direction; return is returning function -> returned value/type. `type_relation`, `register`, `callback`, `argument_flow`, `control_flow`, `assignment`, `data_flow`, and `return` are typed-only and need their exact typed evidence; labels never mint them. Example: `{from_node:\"Factory\", to_node:\"ConsoleSink\", relation_kind:\"return\"}` represents a grounded factory return, not a call. In every non-runtime-trace answer, relation_kind=call needs one same-direction grounded call-site EvidenceItem. In a grounded sequenceDiagram, invocation `caller->>callee` needs that call anchor; dashed `callee-->>caller` is a response only when it mirrors an already-drawn opposite invocation, so do not anchor that reply as call. In a call_dag, every visible edge needs its correct typed anchor. Flow/architecture edges also need their honest relation owner and must not default every arrow to call. Prefer method-qualified participants; class/actor labels are accepted only when the exact message operation resolves to one unique typed call edge. Keep model-selected principal calls visible; supporting calls outside that path need not be drawn. Outside the grounded source call-chain contract, omit edge_anchors when the block has no typed diagram edge. Edge anchors NEVER live inside claim_uses."
}
