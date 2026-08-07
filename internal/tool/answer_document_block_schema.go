package tool

import (
	"fmt"
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
//  7. Dispatch-scoped schema reminder (fixed cross-scenario examples are
//     deliberately absent because projected schemas may reject their kinds)
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
		"table (complete markdown table inside text, OR structured rows: either columns[] + items[].cells[] with label omitted and one cell per column, or label as the first visible value plus cells[]/text for every remaining column; columns[] may omit only the synthetic label header; use label/text without columns only for the legacy two-column fallback), " +
		"diagram (diagram{kind, language, body}), caveat (text only). " +
		"\n\n" +
		"Each block has an `id` (any non-empty string the LLM picks; load-bearing — your retry hints reference it back to you) and `kind` " +
		"(text for prose blocks, items[] for list blocks, flexible text/columns/items carriers for table blocks, diagram for diagram blocks). " +
		"\n\n" +
		"List/table items may carry `candidate_role` when a row's category or scalar/literal role matters: function, method, type, constant, variable, field, package, file, test, generated, private, documentation, example, fixture, helper, agent, tool_name, config_key, route, import_path, literal_value, commit_hash, budget_cap, attempt_counter, guard_condition, or other. When the user explicitly excludes a candidate category, or when the user-section's typed answer-role contract requires a positive role, set this field on principal row items so the validator can enforce the contract from typed row metadata instead of prose. " +
		"For source-inventory enumerations only, a block intentionally scoped to one exact `surface_family` from `Principal Enumeration Rows` may copy that key into `source_inventory_family`; omit it for a global or mixed-family block. This is the only hard family-partition carrier: titles and prose are presentation, never family authority. " +
		"Principal decision blocks may carry `error_granularity_verdict` when the user-section's typed error-granularity contract requires a canonical failure-scope verdict. Allowed values: `per_item_rejection`, `whole_batch_failure`, `partial_success`, `fail_fast`, `collect_errors`, `not_enough_evidence`. Set this field on the decision block itself; do not encode the verdict only in prose. " +
		"Principal decision blocks may carry `current_status_verdict` when the user-section's typed current-status diagnostic contract requires a bounded status verdict. Allowed values: `still_present`, `fixed`, `not_enough_evidence`. Set this field on the decision block itself; do not encode the verdict only in prose. Use `still_present` when current cited code still exposes the comparable risk, `fixed` when current cited code blocks it, and `not_enough_evidence` only when current evidence cannot decide between those two. These verdicts assess the current checkout; without a separate typed revision/transition witness, `fixed` does not prove which change fixed a historical incident or that the captured build includes the current guard. " +
		"When the user section exposes a Trace causal-claim contract, " + TraceCausalClaimPrincipalSummaryShape(nil) + " Keep its wording within the selected scope. `no_causal_conclusion` reports observations without choosing a cause; `bounded_window_candidate` names a selected-window candidate/validation direction but not a proven frame cause; `typed_chain_cause` is bounded to a typed causal chain; `typed_frame_cause` requires typed frame/deadline causality. You choose the conclusion and this caliber; only the typed ceiling is checked, and neither is derived from prose or rewritten for you. " +
		"\n\n" +
		"NON-DECISION PRINCIPAL BLOCKS (the user-section's Required Answer Blocks list flags these as `surface_role=principal`) MUST carry a claim annotation when the user-section contract lists allowed `claim_form` values for that block. Principal `decision` blocks that carry `current_status_verdict` or `error_granularity_verdict` use that typed verdict field as the decision carrier; add `claim_uses[]` only when you have a clear extra evidence-shape annotation. " +
		"Allowed claim_form values come only from the sibling projected `claim_form.enum`; do not reconstruct or narrow that enum from prose. Semantic boundary: `call_edge` proves direct caller→callee invocation; `callback_handoff` proves a receiving API was passed a callable but not that it executed; `registration_edge` proves binding; literal/text-reference forms do not prove definition/call/assignment. " +
		"Annotation placement: claim annotations live ONLY at block level on `claim_uses[]` (a plural array, one entry per applicable claim form). Single-form blocks emit a one-element array like `claim_uses=[{claim_form=definition_fact}]`; when items inside the block contribute distinct claim forms (e.g. some hops are `call_edge`, others are `guard_condition`), list one entry per form. There is no per-item claim_use field. " +
		"For `call_edge` / `import_edge` list or table items, render the principal relation with an explicit edge surface such as `` `caller` -> `callee` `` or `` `file` -> `package` ``. Boundary / comparison / exclusion prose that merely mentions both endpoints should stay as prose without an arrow. " +
		"`relation_claims` is optional model-authored structured metadata and is never a document-level field. Use the typed Trace Decision Inputs to keep visible reasoning accurate. If you choose to publish relation metadata, place the exact claim on the model-authored `blocks[i].relation_claims`; the framework validates submitted claims against typed authority and asks you to retry only on an invalid submitted claim. It never requires a format-only copy and never rewrites your prose or conclusion. " +
		"\n\n" +
		"DIAGRAM BLOCKS — `diagram.kind` is the SEMANTIC FAMILY (`flow` / `sequence` / `architecture` / `call_dag`), NOT a Mermaid keyword. Mermaid syntax (`flowchart` / `sequenceDiagram`) goes inside `diagram.body` with `diagram.language=\"mermaid\"`. " +
		"\n\n" +
		"Citations live in a shared `citations` pool; per-item `citation_ref` is a zero-based index into it. Omit the field when no current-repo cite backs the item. This is an internal carrier only: do not mention `citation_ref` or `citations[]` in visible answer prose. `claim_use` / `claim_uses` never carry `citation_ref`. " +
		"`exact_resolution`, `missing_requested_roles[]`, `caveats[]`, `snippets[]` are document-level optional fields. Use `missing_requested_roles[]` only when the question explicitly asked for named config-precedence layers and one or more of those requested layers has NO grounded binding for the exact target. Each entry is `{role: default|config|runtime|override, label?: <user-facing bucket name>}`; the renderer materialises the explicit missing-layer prose from this typed field, so do not hide missing requested layers behind vague placeholders like `N/A`. " +
		"\n\n" +
		"Top-level fields shape / steps / symbols / value / boolean / summary are NOT accepted at runtime — the entire answer payload lives inside blocks[] only. " +
		"Do not copy a generic cross-scenario JSON example: the tool schema is projected for THIS dispatch and the user section's Required Answer Blocks list is the exact recipe. Emit only the block kinds and conditional fields those two surfaces expose." +
		buildPreEmitConstraintsSection()
}

// TraceCausalClaimPrincipalSummaryShape is the single JSON-shape teaching for
// the Trace causal declaration. The same wording is reused by the tool
// description, the dispatch-local final boundary, the projected field schema,
// and missing-block repair so those surfaces cannot disagree about block kind
// or field ownership. It names structure only and never supplies answer text.
func TraceCausalClaimPrincipalSummaryShape(allowed []types.TraceCausalClaimCaliber) string {
	value := "one value from the projected enum"
	if len(allowed) > 0 {
		values := make([]string, 0, len(allowed))
		for _, caliber := range allowed {
			if caliber != "" {
				values = append(values, string(caliber))
			}
		}
		if len(values) > 0 {
			value = "one of: " + strings.Join(values, ", ")
		}
	}
	return fmt.Sprintf("emit one `blocks[]` object shaped `{id: <non-empty id>, kind: \"summary\", surface_role: \"principal\", text: <your model-authored lead>, trace_causal_claim_caliber: <%s>}`. `trace_causal_claim_caliber` is invalid on every other block kind, including `section`; do not add it to a section that merely contains lead-like prose.", value)
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
