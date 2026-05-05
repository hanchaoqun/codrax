package tool

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
//   1. Block-kind table reminder
//   2. Principal-block claim_use rule (with the ClaimForm enum)
//   3. Diagram payload rule (semantic family vs Mermaid keyword)
//   4. Citation pool semantics (citation_ref placement, -1 sentinel,
//      scalar / decision item-level anchor convention)
//   5. exact_resolution / caveats / snippets document-level fields
//   6. V1 carrier retirement reminder
//   7. Five worked examples (one per principal-block family)
//
// LLM-facing prose only — no internal Go terminology (R4 invariant).
// Maintenance: edits to block-level invariants live here so both
// tools see the change on the next build.
func BuildAnswerDocumentSemanticContractDescription() string {
	return "Block kinds (the user section's Required Answer Blocks list flags which kinds + counts your answer must include): " +
		"summary (block.text — a multi-paragraph explanation), section (title + text — per-bucket / per-layer chunks), " +
		"ordered_list / bullet_list (items[] each with id, optional label, text, optional top-level citation_ref, optional per-item claim_use, optional kind ∈ principal/flow/caveat for ordered_list), " +
		"scalar (block.text carries the literal; optional one-element items=[{citation_ref:N}] anchors the cite), " +
		"decision (block.text carries verdict + rationale; same one-element items pattern for the cite), " +
		"table (markdown table inside text, OR items[] with one item per row), " +
		"diagram (diagram{kind, language, body}), caveat (text only). " +
		"\n\n" +
		"Each block has an `id` (any non-empty string the LLM picks; load-bearing — your retry hints reference it back to you) and `kind` " +
		"(text for prose blocks, items[] for list/table blocks, diagram for diagram blocks). " +
		"\n\n" +
		"PRINCIPAL BLOCKS (the user-section's Required Answer Blocks list flags these as `surface_role=principal`) MUST carry a claim annotation when the contract's AcceptableClaimForms list is non-empty. " +
		"Allowed claim_form values: `definition_fact` (cited line establishes a typed fact: const, struct field, function signature, default value), `call_edge` (caller→callee call site), `guard_condition` (branch / condition gating the answer), `assignment_fact` (config / variable / field assignment), `return_fact` (return statement / function output), `absence_fact` (cited evidence carries Negative scope — search confirmed absent), `precedence_role` (cited evidence carries a layer / override role), `external_observation` (cited evidence is from runtime log / perf trace, not repo source), `import_edge` (module / package import edge). " +
		"Annotation placement: at BLOCK level use `claim_uses[]` (PLURAL ARRAY — single-form blocks emit a one-element array like `claim_uses=[{claim_form=definition_fact}]`); at ITEM level use `items[i].claim_use` (SINGULAR object). The schema does NOT have a singular `claim_use` at block level — emitting `block.claim_use` is rejected with `unknown field \"claim_use\"`. " +
		"\n\n" +
		"DIAGRAM BLOCKS — `diagram.kind` is the SEMANTIC FAMILY (`flow` / `sequence` / `architecture` / `call_dag`), NOT a Mermaid keyword. Mermaid syntax (`flowchart` / `sequenceDiagram`) goes inside `diagram.body` with `diagram.language=\"mermaid\"`. " +
		"\n\n" +
		"Citations live in a shared `citations` pool; per-item `citation_ref` (and per-claim_use `citation_ref`) is a zero-based index into it (or -1 for no cite). " +
		"`exact_resolution`, `caveats[]`, `snippets[]` are document-level optional fields. " +
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
		"   \"items\":[\n" +
		"    {\"id\":\"h1\",\"label\":\"Stage A\",\"text\":\"<what stage A does>\",\"kind\":\"principal\",\"claim_use\":{\"claim_form\":\"call_edge\",\"citation_ref\":0}},\n" +
		"    {\"id\":\"h2\",\"label\":\"Stage B\",\"text\":\"<what stage B does>\",\"kind\":\"principal\",\"claim_use\":{\"claim_form\":\"call_edge\",\"citation_ref\":1}}\n" +
		"   ]}\n" +
		"],\"citations\":[{\"file\":\"a.go\",\"line\":10},{\"file\":\"b.go\",\"line\":20}]}\n" +
		"```\n" +
		"\n" +
		"3) Enumeration slate (`ordered_list` block over enumeration members):\n" +
		"```json\n" +
		"{\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<frames what the list enumerates>\"},\n" +
		"  {\"id\":\"slate\",\"kind\":\"ordered_list\",\"surface_role\":\"principal\",\n" +
		"   \"items\":[\n" +
		"    {\"id\":\"m1\",\"label\":\"MemberA\",\"text\":\"<role / why it belongs>\",\"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":0}},\n" +
		"    {\"id\":\"m2\",\"label\":\"MemberB\",\"text\":\"<role / why it belongs>\",\"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":1}}\n" +
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
		"```"
}
