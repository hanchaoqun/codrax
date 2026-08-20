package tool

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

var _ = time.Now // ensure time stays imported when failEmit moved

// EmitAnswerDocument is the structured finalizer channel. The
// finalizer LLM makes exactly one emit_answer_document call per
// dispatch, supplying a typed AnswerDocumentV2 (block-only carrier),
// and a deterministic renderer (internal/render/answerdoc.go) turns
// the struct into user-visible prose.
//
// B8-T3+T4 (block_only_carrier.md §5.8, 2026-05-03): V1 carrier
// fully retired. The external tool contract is now a single
// block-only schema; callers supply `blocks[]` directly and do NOT
// need to name a carrier version. The executor still tolerates a
// legacy `document_model` field when present, but it is no longer
// surfaced in the LLM-facing schema or required by callers.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
// Classified NonEvidenceTool: the payload is the final answer slate,
// not a repo fact. Mirrors emit_answer_symbol on both axes.
type EmitAnswerDocument struct {
	ReadOnly
	NonEvidenceTool
}

func (t *EmitAnswerDocument) Name() string { return "emit_answer_document" }

// Description renders the LLM-facing tool description. Block-only
// V2 carrier — the LLM emits an array of blocks tagged by Kind,
// each carrying its own typed payload (Items, Diagram, Text, etc.).
// Per the feedback_no_internal_info_in_llm_prompts red line, this
// description avoids internal Go terminology (AnswerDocumentV2 /
// BlockRequirement etc.) and uses LLM-natural language only.
//
// v3 B4 (2026-05-04): the block-semantic contract body is shared
// with emit_answer_document_patch via
// BuildAnswerDocumentSemanticContractDescription so adding a new
// block field / kind / claim_form / worked example only edits one
// helper.
func (t *EmitAnswerDocument) Description() string {
	return types.AnswerDocumentJSONShapeFirstTeaching + "\n\n" +
		"Emit the FULL final answer document as a structured blocks[] array. " +
		"Treat the projected tool schema as the only authority for field names, value types, required fields, and enums in this dispatch. " +
		"Use this on first dispatches and whenever the answer needs a complete rewrite. On retry paths where only a few blocks need editing, prefer emit_answer_document_patch which protocol-level preserves typed annotation fields on blocks you do not touch.\n\n" +
		BuildAnswerDocumentSemanticContractDescription()
}

// Parameters returns the canonical (full) V2 JSON schema. Most
// callers should use ParametersFor(ctx) so the schema is projected
// onto the per-dispatch AnswerSemanticView (drop fields the
// dispatch will not need, restrict enums to the contract's allowed
// set). Parameters() stays as the test / no-context fallback and
// as the source of truth that the projection helper edits in
// place.
func (t *EmitAnswerDocument) Parameters() json.RawMessage {
	return t.canonicalParameters()
}

// ParametersFor returns the V2 schema projected onto the
// per-dispatch AnswerSemanticView compiled from ctx. Falls back to
// the canonical schema when no view is available (e.g. ctx==nil
// or AnalysisIR missing).
func (t *EmitAnswerDocument) ParametersFor(ctx *types.AgentContext) json.RawMessage {
	if ctx == nil {
		return t.canonicalParameters()
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	return BuildAnswerDocumentParametersFor(view)
}

func (t *EmitAnswerDocument) canonicalParameters() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "blocks": {
      "type": "array",
      "description": "Ordered list of answer blocks. REQUIRED; must be non-empty. Each block is a structured payload tagged by kind.",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string", "description": "Stable unique identifier for this block (any non-empty string the LLM picks)."},
          "kind": {
            "type": "string",
            "enum": ["summary", "section", "ordered_list", "bullet_list", "scalar", "decision", "table", "diagram", "caveat"],
            "description": "Block kind. Required. Each kind expects a specific payload field — the wrong payload location is the most common shape error: summary/scalar/decision/caveat use block.text; section uses block.text for narrative and may also use block.items[] for structured or cited rows; ordered_list/bullet_list use block.items[]; table is intentionally flexible but MUST contain a visible table payload: use block.text when you already authored a complete markdown table, OR columns[] + at least one items[].cells[] structured row, OR at least one items[].label/text row when a two-column fallback is sufficient. Never emit a header-only table with columns[] and zero visible rows; it cannot render. **kind=diagram REQUIRES the sibling diagram object below (with kind, language, body) — diagram body NEVER goes in block.text**."
          },
          "title": {"type": "string", "description": "Optional sub-heading for section / table / diagram / caveat blocks."},
          "text": {"type": "string", "description": "Block body prose. Used by summary / section / scalar / decision / caveat. For table blocks, this may carry the complete model-authored markdown table. Markdown-flavoured. NEVER use this field on diagram blocks — diagram body lives in diagram.body."},
          "error_granularity_verdict": {"type": "string", "enum": ["per_item_rejection", "whole_batch_failure", "partial_success", "fail_fast", "collect_errors", "not_enough_evidence"], "description": "Optional canonical verdict for principal decision blocks that answer failure-scope / batch-vs-item / fail-fast-vs-collect questions. Use only on kind=decision blocks when the user-section's typed error-granularity contract requires it; do not encode this only in prose."},
          "current_status_verdict": {"type": "string", "enum": ["still_present", "fixed", "not_enough_evidence"], "description": "Optional canonical verdict for principal decision blocks that answer diagnostic current-status questions. Use only on kind=decision blocks when the user-section's typed current-status contract requires it; do not encode this only in prose. Semantics: still_present means current cited code still exposes the comparable risk, fixed means current cited code blocks/removes it, and not_enough_evidence means current evidence cannot decide between those two. This verdict is scoped to the current checkout; without typed revision/transition evidence it does not prove which change fixed a historical incident or that the captured build includes the current guard."},
          "trace_causal_claim_caliber": {"type": "string", "enum": ["no_causal_conclusion", "bounded_window_candidate", "typed_chain_cause", "typed_frame_cause"], "description": "Model-authored causal strength for the principal Trace summary. This field is projected in only when a full typed Trace causal report has publication-grade rows; copy one value from the dispatch-specific enum. Keep the summary wording within that declared scope. The system validates the typed evidence ceiling but never infers this value from prose and never writes or replaces your conclusion."},
          "scope_disclosure": {"type": "string", "enum": ["inactive_scope_named", "out_of_active_scope", "requires_workspace_adjust"], "description": "Optional typed declaration that this block explains why a principal answer is bounded by the active sub-repo set. Use only when the user-section flags a multi-repo workspace with inactive sub-repos AND the answer is bounded (absence, empty principal slate, or scope-limited enumeration). Values: inactive_scope_named (this block cites a specific inactive sub-repo by RootRel name), out_of_active_scope (this block asserts the target is outside the active sub-repo set without naming a specific RootRel), requires_workspace_adjust (this block recommends the operator adjust the workspace scope, e.g. via /repos focus, before retrying). Any block kind may carry this; typically a caveat or decision block. If omitted when needed, the system appends a separate supplemental scope note rather than forcing a rewrite."},
          "source_inventory_family": {"type": "string", "description": "Optional exact typed partition key for a principal source-inventory block. Use only when Principal Enumeration Rows exposes surface_family and this block intentionally carries one such family; copy that value exactly. Omit it for a global/mixed-family block. Never infer it from the block title, item prose, path, language, or nearby rows."},
          "columns": {"type": "array", "items": {"type": "string"}, "description": "Optional table headers for kind=table structured rows. Do not use when block.text already contains the markdown table, and never emit columns[] without at least one visible items[] row. Preferred low-mind row contract: omit item.label and item.text, then emit exactly one items[].cells value per columns[] entry. Legacy label-first rows remain accepted only when label deliberately owns the first visible column and cells/text supply every remaining column; columns may include that label header or omit only it (the renderer then adds a neutral header)."},
          "items": {
            "type": "array",
            "description": "Block items for section / ordered_list / bullet_list / table. Section/list items use label/text; table items may use cells[] for a structured multi-column row, or label/text for a simple two-column fallback. A table without a complete Markdown table in block.text must have at least one visible items[] row; citation-only or wholly empty items do not count. If the table is already a markdown table in block.text, leave items empty unless the row needs a citation_ref carrier.",
            "items": {
              "type": "object",
              "properties": {
                "id":           {"type": "string"},
                "label":        {"type": "string", "description": "Primary visible text / row header. For enumeration items, use a verbatim identifier copied from evidence anchor_symbol / subject / object values, OR a verbatim user-named bucket label, OR a typed runtime-artifact label from log/trace triage. Fabricated code labels that are not grounded by those typed channels are rejected at validation time."},
                "text":         {"type": "string", "description": "Item body / row content."},
                "cells":        {"type": "array", "items": {"type": "string"}, "description": "Optional table cells for kind=table structured rows. Preferred form: omit item.label and item.text and put exactly one positional string per columns[] entry; an intentionally unavailable last value may be an empty string and still occupies that column. Use the legacy label-first form only when label deliberately represents the first visible column; then cells/text supply every remaining value and columns[] may omit only that synthetic label header. Keep a complete authored markdown table in block.text instead."},
                "candidate_role": {"type": "string", "enum": ["function", "method", "type", "constant", "variable", "field", "package", "file", "test", "generated", "private", "documentation", "example", "fixture", "helper", "agent", "tool_name", "config_file", "config_key", "route", "import_path", "literal_value", "commit_hash", "budget_cap", "attempt_counter", "guard_condition", "other"], "description": "Optional typed category of the principal item represented by this row. Use when the user excluded a candidate category, when answer_role_profile requires a positive principal role, when the row category matters, or when an exact scalar/literal role such as agent, tool_name, or budget_cap must stay structural. Do not encode only in prose, and do not label a handler/type row as route merely because route is one related column."},
                "source_inventory_row_id": {"type": "string", "description": "Exact row_id copied from Principal Enumeration Rows. REQUIRED for every structured principal source-inventory item, including rows with unique or display-decorated labels; this one identity binds the exact member/family/location citation, so citation_ref may be omitted. Omit for non-source-inventory items."},
				"evidence_ids": {"type": "array", "items": {"type": "string"}, "description": "Optional exact accepted current-source evidence IDs for this one model-authored item, copied verbatim from evidence= rows in the current handoff. Use one ID per independently grounded source fact in stable evidence order and omit manual citation_ref/citation_refs arithmetic; the system binds those exact IDs to citations without changing item text or conclusions. Omit for synthesized/unsupported items and for source-inventory rows, which use source_inventory_row_id instead. Never invent an ID."},
                "citation_ref": {"type": "integer", "description": "Optional primary anchor on the item; zero-based index into citations[]. Omit this field when no current-repo cite backs the item. This is a structural carrier only: never mention citation_ref/citation_refs or citations[] in visible answer text. For scalar / decision blocks (where the literal / verdict sits in block.text), anchor one citation by attaching a one-element items=[{id:\"x\", citation_ref: N}] — there is no top-level value/boolean field on the block."},
				"citation_refs": {"type": "array", "items": {"type": "integer"}, "description": "Optional additional zero-based citation indexes for one visible item that states several independently grounded facts. When citation_ref is present, do not repeat that primary index here; keep additional indexes in stable evidence order. If citation_ref is omitted, normalization promotes this array's first index to primary. For a single anchor prefer citation_ref. This typed array is structural only and never authorizes facts by itself."}
              }
            }
          },
          "diagram": {
            "type": "object",
            "description": "Diagram payload object — REQUIRED whenever kind=diagram (the validator rejects a diagram-kind block that omits this field). Three required sub-fields: {kind: <semantic family enum below>, language: \"mermaid\", body: <raw mermaid source>}. Diagram body goes here, in body — NOT in the block-level text field. Do not emit this field on non-diagram blocks.",
            "properties": {
              "kind":     {"type": "string", "enum": ["flow", "sequence", "architecture", "call_dag"], "description": "SEMANTIC family of the diagram, NOT a Mermaid keyword. Use the family the contract names: flow (branches/guards), sequence (actor-to-actor over time), architecture (layered components), call_dag (one-to-many dispatch). Mermaid syntax tokens like \"flowchart\" and \"sequenceDiagram\" belong inside diagram.body, NOT here."},
              "language": {"type": "string", "enum": ["mermaid"], "description": "Diagram source language. Always \"mermaid\" — the only currently rendered subset."},
              "body":     {"type": "string", "description": "Raw diagram source (the part inside fenced markers; the renderer adds the fences). For diagram.kind=flow/architecture/call_dag use Mermaid \"flowchart\" syntax (direction LR/TD/RL/BT); for diagram.kind=sequence use Mermaid \"sequenceDiagram\"."}
            }
          },
          "claim_uses":   {"type": "array", "description": "Block-level claim annotations array. REQUIRED on non-decision principal blocks (surface_role=principal) when the user-section contract lists allowed claim_form values for the block. Principal decision blocks that carry current_status_verdict or error_granularity_verdict use that typed verdict field as the decision carrier; add claim_uses[] only when you have a clear extra evidence-shape annotation. Each entry has EXACTLY 3 fields: {claim_form, optional facet_id, optional evidence_id}. Single-form blocks emit a one-element array using the one value exposed by the sibling projected claim_form.enum; when the block's items legitimately span multiple exposed forms, list one entry per form. The sibling projected claim_form.enum is the sole availability authority: never emit a form merely because prose elsewhere explains its semantics. Each entry does NOT carry citation_ref or citation_refs — citations live on the enclosing item (items[i].citation_ref plus optional items[i].citation_refs). Each entry does NOT carry from_node / to_node — those live in the block-level edge_anchors[] field below. Keep every selected form within its narrow evidence boundary. When a relation form is available and selected for list/table items, render the principal relation with an explicit edge surface such as caller -> callee; boundary/comparison/exclusion prose that only mentions both endpoints should not use an arrow.", "items": {"type": "object", "properties": {"claim_form": {"type": "string", "enum": ["__ALL_CLAIM_FORMS__"]}, "facet_id": {"type": "string"}, "evidence_id": {"type": "string"}}, "required": ["claim_form"]}},
          "edge_anchors": {"type": "array", "description": "Optional block-level array of typed relation entries. On a diagram (or its sibling endpoint carrier), from_node/to_node are verbatim diagram node ids. __GROUNDED_STANDALONE_CALL_CHAIN_RELATION_OWNERSHIP__ __DIAGRAM_VISIBLE_LABEL_CONSISTENCY__ Each entry requires {from_node, to_node, relation_kind}; preserve identity fields together. On a non-diagram principal relation list/table, use from_node/to_node as concise reader-facing endpoint labels and also set visible_label to reader-facing relation wording in the answer language; the renderer displays these model-authored fields, while the label itself grants no relation authority. Keep exact technical endpoints separately in from_identity/to_identity. Diagram blocks already express the relation visibly in diagram.body and may omit visible_label. relation_kind is the sole typed relation authority and never creates evidence. relation_kind=type_relation uses the exact declared-type direction: subtype / implementing type / embedded type -> superclass / interface / trait / protocol / embedded contract. guard is enclosing callable -> condition only; control_flow is exact parser-proved branch arm -> call/return/assignment/exit effect. type_relation, register, callback, control_flow, assignment, data_flow, return, and temporal are typed-only; label words never mint them. temporal means measured runtime ordering/adjacency without causality. callback represents receiving API/dispatcher -> passed callable and does not prove execution; assignment represents assigned receiver -> bound value/type; data_flow represents the same exact assignment in execution direction, RHS value/source -> LHS receiver; return represents returning function -> returned value/type. Omit edge_anchors when no typed edge is needed; outside strict grounded call-chain contracts, legacy rendered-label vocabulary may still describe a display relation without an anchor.", "items": {"type": "object", "properties": {"from_node": {"type": "string", "description": "Diagram node id; on a standalone non-diagram carrier, the model-authored reader-facing source endpoint label."}, "to_node": {"type": "string", "description": "Diagram node id; on a standalone non-diagram carrier, the model-authored reader-facing destination endpoint label."}, "visible_label": {"type": "string", "description": "Model-authored reader-facing relation wording. Required by the pre-emit contract only for a standalone non-diagram principal relation carrier. On a diagram, when supplied it must exactly match the corresponding Mermaid edge/message label. It never creates relation authority."}, "from_identity": {"type": "string"}, "to_identity": {"type": "string"}, "relation_kind": {"type": "string", "enum": ["__ALL_DIAGRAM_RELATIONS__"]}}, "required": ["from_node", "to_node", "relation_kind"]}},
          "participant_boundaries": {"type": "array", "description": "BLOCK-LEVEL sibling of diagram and edge_anchors; NEVER place it inside the diagram object. Exact shape example: {kind:\"diagram\", diagram:{kind,language,body}, participant_boundaries:[{participant,status:\"unproven\"}]}. Typed model-authored boundary decisions for incident_required participants that have no evidence-backed incident relation in this diagram. Omit or emit [] when every required participant has a typed visible incident edge. If any required participant lacks such an edge, add exactly one row per uncovered participant and keep that participant visible as a disconnected node. This field never authorizes or creates an edge; do not list context_only, unknown, or already-connected participants.", "items": {"type": "object", "properties": {"participant": {"type": "string"}, "status": {"type": "string", "enum": ["unproven"]}}, "required": ["participant", "status"]}},
          "relation_claims": {"type": "array", "description": "Optional model-authored typed declarations for value relations used by this block. This field lives at blocks[i].relation_claims, never at document-level $.relation_claims. Trace Decision Inputs are precise reasoning context but do not create a format-only copy obligation. If you choose to publish a relation claim here, reproduce its typed authority object exactly and keep block.text consistent with it. Each entry is {authority_id, member_refs[], physical_relation: unresolved|mutually_exclusive|overlap|contains|contained_by, addition: authorized_to_published_subtotal|forbidden, subtotal_value?: number, subtotal_unit?: string}. Never invent an authority id and never put a cross-ruler total on a forbidden authority.", "items": {"type": "object", "properties": {"authority_id":{"type":"string"},"member_refs":{"type":"array","items":{"type":"string"}},"physical_relation":{"type":"string","enum":["unresolved","mutually_exclusive","overlap","contains","contained_by"]},"addition":{"type":"string","enum":["authorized_to_published_subtotal","forbidden"]},"subtotal_value":{"type":"number"},"subtotal_unit":{"type":"string"}},"required":["authority_id","member_refs","physical_relation","addition"]}},
          "facet_ids":    {"type": "array", "items": {"type": "string"}, "description": "Optional facet ids this block covers — read these from the user section's Required Answer Blocks list."},
          "surface_role": {"type": "string", "enum": ["principal"], "description": "Set to \"principal\" when this block carries the main-line answer payload (the user-section's Required Answer Blocks list flags which blocks expect principal). OMIT this field on supporting context, framing prose, and diagram-only contributions — absence is equivalent to not-principal."}
        },
        "required": ["id", "kind"]
      }
    },
    "citations": {
      "type": "array",
      "description": "Shared pool of file-line citations. Per-item citation_ref and citation_refs values are zero-based indexes into this array.",
      "items": {"type": "object"}
    },
    "exact_resolution": {"type": "object", "description": "Optional exact-resolution contract result (status / anchor / context_mode)."},
    "missing_requested_roles": {
      "type": "array",
      "description": "Optional typed disclosure for user-requested config-precedence layers that have NO grounded binding for the exact target in this dispatch. Use this when the question explicitly asked for layers such as code default / config file / CLI and one or more of those requested layers remained absent. Each entry is {role: one of default|config|runtime|override, label?: string}. role is the abstract precedence role; label is the user-facing bucket name from the question (for example CLI). The renderer turns these entries into explicit missing-layer prose, so do NOT hide them behind N/A / not applicable / vague placeholders.",
      "items": {
        "type": "object",
        "properties": {
          "role": {"type": "string", "enum": ["default", "config", "runtime", "override"]},
          "label": {"type": "string"}
        },
        "required": ["role"]
      }
    }
  },
  "required": ["blocks"]
}`
	raw := []byte(schema)
	raw = bytes.Replace(raw, []byte(`["__ALL_CLAIM_FORMS__"]`), marshalClaimFormEnum(), 1)
	raw = bytes.Replace(raw, []byte(`["__ALL_DIAGRAM_RELATIONS__"]`), marshalDiagramRelationEnum(), 1)
	raw = bytes.Replace(raw,
		[]byte("__GROUNDED_STANDALONE_CALL_CHAIN_RELATION_OWNERSHIP__"),
		[]byte(types.GroundedStandaloneCallChainRelationOwnershipContract),
		1)
	raw = bytes.Replace(raw,
		[]byte("__DIAGRAM_VISIBLE_LABEL_CONSISTENCY__"),
		[]byte(types.DiagramVisibleLabelConsistencyContract),
		1)
	raw = bytes.Replace(raw,
		[]byte("Omit edge_anchors when no typed edge is needed; outside strict grounded call-chain contracts, legacy rendered-label vocabulary may still describe a display relation without an anchor."),
		[]byte(types.GroundedSourceDiagramEdgeOwnershipContract+" "+types.GroundedSourceDiagramRelationEvidenceContract+" Outside this strict contract, omit edge_anchors when no typed edge is needed; legacy rendered-label vocabulary may still describe a display relation without an anchor."),
		1)
	return json.RawMessage(raw)
}

func marshalClaimFormEnum() []byte {
	forms := types.AllClaimForms()
	values := make([]string, 0, len(forms))
	for _, form := range forms {
		values = append(values, string(form))
	}
	raw, _ := json.Marshal(values)
	return raw
}

func marshalDiagramRelationEnum() []byte {
	relations := types.AllDiagramRelationKinds()
	values := make([]string, 0, len(relations))
	for _, relation := range relations {
		values = append(values, string(relation))
	}
	raw, _ := json.Marshal(values)
	return raw
}

// Execute routes the emit to the block-only validator + writer.
// V1 carrier is retired; there is only one external answer
// contract now. Historical payloads may still include
// `document_model`, but it is ignored rather than required.
func (t *EmitAnswerDocument) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_answer_document is not available in this dispatch (the framework call site did not initialise its writable state). This is a framework-side issue, not something to fix in your emit.",
			Timestamp: now,
		}, nil
	}
	// v3.1 (2026-05-05): the dispatcher no longer inspects
	// document_model at all — there is only one executor path, and
	// the LLM-facing schema no longer mentions the field. The
	// executor itself silently tolerates whatever value (or absence)
	// the LLM supplies; nothing here is worth a round-trip rejection.
	return executeAnswerDocumentV2(t.Name(), ctx, params, now)
}

// requestedAnswerDocumentLanguage returns the requested answer
// language for downstream tool helpers (log_source_drift_surface,
// emit_evidence). Reads AnswerContract.Language first, then
// BusContext.Language fallback. Lowercased + trimmed.
func requestedAnswerDocumentLanguage(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	if ctx.AnalysisIR != nil {
		if lang := strings.ToLower(strings.TrimSpace(ctx.AnalysisIR.AnswerContract.Language)); lang != "" {
			return lang
		}
	}
	return strings.ToLower(strings.TrimSpace(ctx.Language))
}

// answerDocumentRequiresChinese reports whether the requested
// language matches the Chinese family.
func answerDocumentRequiresChinese(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese":
		return true
	}
	return false
}

// answerSurfacePlan compiles an AnswerSurfacePlan from the
// BusContext + applies extractor's answer-symbol step backbone.
func answerSurfacePlan(ctx *types.BusContext) *types.AnswerSurfacePlan {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	plan := types.BuildAnswerSurfacePlanForBusContext(ctx)
	if plan != nil && len(ctx.AnswerSymbols) > 0 {
		types.ApplyAnswerSymbolStepBackbone(plan, ctx.AnalysisIR, ctx.AnswerSymbols, ctx.AnswerSymbolCompleteness)
	}
	if plan != nil {
		plan.StableAggregateFacts = normalizeAggregateFactsForTypedExclusion(ctx, plan.StableAggregateFacts)
	}
	return plan
}

// answerExactResolutionContract pulls the ExactResolution contract
// from the surface plan.
func answerExactResolutionContract(ctx *types.BusContext) *types.ExactResolutionContract {
	if plan := answerSurfacePlan(ctx); plan != nil {
		return plan.ExactResolution
	}
	return nil
}
