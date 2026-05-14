package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// answer_document_block_schema.go — B4 v3 (2026-05-04). Single-source-
// of-truth helpers for the answer-document tool surfaces shared by
// emit_answer_document (full) and emit_answer_document_patch
// (delta).
//
// Pre-v3, the two tool's Description() return values duplicated the
// block-semantic contract prose: both explained block kinds, claim_use
// rules, citation pool semantics, and the worked examples. Adding a
// new block kind / field / annotation rule required editing both
// strings by hand. v3 collapses the shared prose into
// BuildAnswerDocumentSemanticContractDescription so the two callers
// stay in lock-step automatically.
//
// W3 (2026-05-05) extends the prose with auto-collected
// SchemaDescriptionFragment entries from ViolKindSpec, so the LLM
// sees the post-emit validator constraints AT EMIT TIME rather
// than learning them from a retry hint. See
// docs/design/iteration_inflation_remediation.md §3 source #1.
//
// JSON Schema sharing is a separate concern — the patch tool keeps
// replace_blocks / add_blocks as opaque object arrays today; the
// full tool inlines a detailed schema. A follow-up PR can extract
// shared schema fragments into a helper here once the patch tool
// surfaces detailed block-field validation. This file's contract
// for now is the prose surface only.

// BuildAnswerDocumentSemanticContractDescription returns the canonical
// answer-document block-semantic contract description. Both Description()
// methods (full + patch) prepend an entry-specific lead-in (e.g. "Full
// emit:" / "Delta emit:") and append this shared body so readers see
// one consistent contract regardless of which tool they call.
//
// Output sections (in order):
//  1. Block-kind table reminder
//  2. Principal-block claim_use rule (with the ClaimForm enum)
//  3. Diagram payload rule (semantic family vs Mermaid keyword)
//  4. Citation pool semantics (citation_ref placement, -1 sentinel,
//     scalar / decision item-level anchor convention)
//  5. exact_resolution / caveats / snippets document-level fields
//  6. V1 carrier retirement reminder
//  7. Five worked examples (one per principal-block family)
//
// LLM-facing prose only — no internal Go terminology (R4 invariant).
// Maintenance: edits to block-level invariants live here so both
// tools see the change on the next build.
func BuildAnswerDocumentSemanticContractDescription() string {
	return "Block kinds (the user section's Required Answer Blocks list flags which kinds + counts your answer must include): " +
		"summary (block.text — a multi-paragraph explanation), section (title + text — per-bucket / per-layer chunks), " +
		"ordered_list / bullet_list (items[] each with id, optional label, text, optional top-level citation_ref), " +
		"scalar (block.text carries the literal; optional one-element items=[{citation_ref:N}] anchors the cite), " +
		"decision (block.text carries verdict + rationale; same one-element items pattern for the cite), " +
		"table (markdown table inside text, OR items[] with one item per row), " +
		"diagram (diagram{kind, language, body}), caveat (text only). " +
		"\n\n" +
		"Each block has an `id` (any non-empty string the LLM picks; load-bearing — your retry hints reference it back to you) and `kind` " +
		"(text for prose blocks, items[] for list/table blocks, diagram for diagram blocks). " +
		"\n\n" +
		"List/table items may carry `candidate_role` when a row's category or scalar/literal role matters: function, method, type, constant, variable, field, package, file, test, generated, private, documentation, example, fixture, helper, tool_name, config_key, route, import_path, literal_value, commit_hash, budget_cap, attempt_counter, guard_condition, or other. When the user explicitly excludes a candidate category, or when the user-section's typed answer-role contract requires a positive role, set this field on principal row items so the validator can enforce the contract from typed row metadata instead of prose. " +
		"\n\n" +
		"PRINCIPAL BLOCKS (the user-section's Required Answer Blocks list flags these as `surface_role=principal`) MUST carry a claim annotation when the user-section contract lists allowed `claim_form` values for that block. " +
		"Allowed claim_form values: `definition_fact` (cited line establishes a typed fact: const, struct field, function signature, default value), `call_edge` (caller→callee call site), `guard_condition` (branch / condition gating the answer), `assignment_fact` (config / variable / field assignment), `return_fact` (return statement / function output), `absence_fact` (cited evidence carries Negative scope — search confirmed absent), `precedence_role` (cited evidence carries a layer / override role), `external_observation` (cited evidence is from runtime log / perf trace, not repo source), `import_edge` (module / package import edge), `text_reference_fact` (visible source/config/doc/comment text itself; not definition/call/assignment proof). " +
		"Annotation placement: claim annotations live ONLY at block level on `claim_uses[]` (a plural array, one entry per applicable claim form). Single-form blocks emit a one-element array like `claim_uses=[{claim_form=definition_fact}]`; when items inside the block contribute distinct claim forms (e.g. some hops are `call_edge`, others are `guard_condition`), list one entry per form. There is no per-item claim_use field. " +
		"For `call_edge` / `import_edge` list or table items, render the principal relation with an explicit edge surface such as `` `caller` -> `callee` `` or `` `file` -> `package` ``. Boundary / comparison / exclusion prose that merely mentions both endpoints should stay as prose without an arrow. " +
		"\n\n" +
		"DIAGRAM BLOCKS — `diagram.kind` is the SEMANTIC FAMILY (`flow` / `sequence` / `architecture` / `call_dag`), NOT a Mermaid keyword. Mermaid syntax (`flowchart` / `sequenceDiagram`) goes inside `diagram.body` with `diagram.language=\"mermaid\"`. " +
		"\n\n" +
		"Citations live in a shared `citations` pool; per-item `citation_ref` is a zero-based index into it (or -1 for no cite). `claim_use` / `claim_uses` never carry `citation_ref`. " +
		"`exact_resolution`, `missing_requested_roles[]`, `caveats[]`, `snippets[]` are document-level optional fields. Use `missing_requested_roles[]` only when the question explicitly asked for named config-precedence layers and one or more of those requested layers has NO grounded binding for the exact target. Each entry is `{role: default|config|runtime|override, label?: <user-facing bucket name>}`; the renderer materialises the explicit missing-layer prose from this typed field, so do not hide missing requested layers behind vague placeholders like `N/A`. " +
		"\n\n" +
		"Top-level fields shape / steps / symbols / value / boolean / summary are NOT accepted at runtime — the entire answer payload lives inside blocks[] only." +
		"\n\n" +
		"WORKED EXAMPLES (minimal happy-path emits — each shows one principal-block family):\n" +
		"\n" +
		"1) Summary-only explanation (single principal `summary` block):\n" +
		"```json\n" +
		"{\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<multi-paragraph answer body>\",\"surface_role\":\"principal\",\"claim_uses\":[{\"claim_form\":\"definition_fact\"}],\"items\":[{\"id\":\"c0\",\"citation_ref\":0}]}\n" +
		"],\"citations\":[{\"file\":\"foo/bar.go\",\"line\":42}]}\n" +
		"```\n" +
		"\n" +
		"2) Hop-chain (`ordered_list` block over mechanism steps):\n" +
		"```json\n" +
		"{\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<lead-in framing the chain>\"},\n" +
		"  {\"id\":\"hops\",\"kind\":\"ordered_list\",\"surface_role\":\"principal\",\n" +
		"   \"claim_uses\":[{\"claim_form\":\"call_edge\"}],\n" +
		"   \"items\":[\n" +
		"    {\"id\":\"h1\",\"label\":\"Stage A\",\"text\":\"<what stage A does>\",\"citation_ref\":0},\n" +
		"    {\"id\":\"h2\",\"label\":\"Stage B\",\"text\":\"<what stage B does>\",\"citation_ref\":1}\n" +
		"   ]}\n" +
		"],\"citations\":[{\"file\":\"a.go\",\"line\":10},{\"file\":\"b.go\",\"line\":20}]}\n" +
		"```\n" +
		"\n" +
		"3) Enumeration slate (`ordered_list` block over enumeration members):\n" +
		"```json\n" +
		"{\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<frames what the list enumerates>\"},\n" +
		"  {\"id\":\"slate\",\"kind\":\"ordered_list\",\"surface_role\":\"principal\",\n" +
		"   \"claim_uses\":[{\"claim_form\":\"definition_fact\"}],\n" +
		"   \"items\":[\n" +
		"    {\"id\":\"m1\",\"label\":\"MemberA\",\"text\":\"<role / why it belongs>\",\"citation_ref\":0},\n" +
		"    {\"id\":\"m2\",\"label\":\"MemberB\",\"text\":\"<role / why it belongs>\",\"citation_ref\":1}\n" +
		"   ]}\n" +
		"],\"citations\":[{\"file\":\"x.go\",\"line\":1},{\"file\":\"y.go\",\"line\":1}]}\n" +
		"```\n" +
		"\n" +
		"4) Single-literal scalar (`scalar` block — literal in block.text, citation via one-element items[]):\n" +
		"```json\n" +
		"{\"blocks\":[\n" +
		"  {\"id\":\"v1\",\"kind\":\"scalar\",\"text\":\"42\",\"surface_role\":\"principal\",\n" +
		"   \"items\":[{\"id\":\"vcite\",\"citation_ref\":0}],\n" +
		"   \"claim_uses\":[{\"claim_form\":\"definition_fact\"}]}\n" +
		"],\"citations\":[{\"file\":\"const.go\",\"line\":7}]}\n" +
		"```\n" +
		"\n" +
		"5) Architecture diagram (`diagram` block with semantic family `architecture`):\n" +
		"```json\n" +
		"{\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<overall architecture lead-in>\",\"surface_role\":\"principal\",\"claim_uses\":[{\"claim_form\":\"definition_fact\"}],\"items\":[{\"id\":\"c0\",\"citation_ref\":0}]},\n" +
		"  {\"id\":\"d1\",\"kind\":\"diagram\",\n" +
		"   \"diagram\":{\"kind\":\"architecture\",\"language\":\"mermaid\",\"body\":\"flowchart TD\\n    A[\\\"<grounded node A>\\\"] --> B[\\\"<grounded node B>\\\"]\"}}\n" +
		"],\"citations\":[{\"file\":\"main.go\",\"line\":1}]}\n" +
		"```" +
		buildPreEmitConstraintsSection()
}

// buildPreEmitConstraintsSection assembles the W3 pre-emit
// constraints block from every registered ViolKindSpec that has a
// non-empty SchemaDescriptionFragment. Output ordering follows
// AllViolKindSpecs registration order so the constraint set is
// stable across builds.
//
// Empty when no kind has registered a fragment (test harnesses
// using a stripped registry). Returns empty string in that case so
// the schema description is byte-identical to pre-W3.
func buildPreEmitConstraintsSection() string {
	specs := types.AllViolKindSpecs()
	fragments := make([]string, 0, len(specs))
	for _, s := range specs {
		f := strings.TrimSpace(s.SchemaDescriptionFragment)
		if f == "" {
			continue
		}
		fragments = append(fragments, "- "+f)
	}
	if len(fragments) == 0 {
		return ""
	}
	return "\n\nPRE-EMIT CONSTRAINTS (the contract validator enforces these at emit time; addressing them in the FIRST emit avoids retry rounds):\n\n" +
		strings.Join(fragments, "\n")
}
