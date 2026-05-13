package tool

import (
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
	return "Emit the FULL final answer document as a structured blocks[] array. " +
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
            "description": "Block kind. Required. Each kind expects a specific payload field — the wrong payload location is the most common shape error: summary/section/scalar/decision/caveat use block.text; ordered_list/bullet_list use block.items[]; table may use visible items[] rows OR a markdown table in block.text (do not add citation-only empty items when using text); **kind=diagram REQUIRES the sibling diagram object below (with kind, language, body) — diagram body NEVER goes in block.text**."
          },
          "title": {"type": "string", "description": "Optional sub-heading for section / table / diagram / caveat blocks."},
          "text": {"type": "string", "description": "Block body prose. Used by summary / section / scalar / decision / caveat. Markdown-flavoured. NEVER use this field on diagram blocks — diagram body lives in diagram.body."},
          "items": {
            "type": "array",
            "description": "Block items for ordered_list / bullet_list / table. For tables each item is one visible row with label/text; if the table is already a markdown table in block.text, leave items empty rather than adding citation-only placeholder rows.",
            "items": {
              "type": "object",
              "properties": {
                "id":           {"type": "string"},
                "label":        {"type": "string", "description": "Primary visible text / row header. For enumeration items, use a verbatim identifier copied from evidence anchor_symbol / subject / object values, OR a verbatim user-named bucket label, OR a typed runtime-artifact label from log/trace triage. Fabricated code labels that are not grounded by those typed channels are rejected at validation time."},
                "text":         {"type": "string", "description": "Item body / row content."},
                "citation_ref": {"type": "integer", "description": "Top-level field on the item; zero-based index into citations[], or -1 when no cite backs the item. For scalar / decision blocks (where the literal / verdict sits in block.text), anchor the citation by attaching a one-element items=[{id:\"x\", citation_ref: N}] — there is no top-level value/boolean field on the block."}
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
          "claim_uses":   {"type": "array", "description": "Block-level claim annotations array. REQUIRED on principal blocks (surface_role=principal) when the user-section contract lists allowed claim_form values for the block. Each entry has EXACTLY 3 fields: {claim_form: <one of definition_fact|call_edge|guard_condition|assignment_fact|return_fact|absence_fact|precedence_role|external_observation|import_edge|text_reference_fact>, facet_id?: string, evidence_id?: string}. Single-form blocks emit a one-element array (claim_uses=[{claim_form=definition_fact}]); when items inside the block contribute distinct claim forms, list one entry per form. Each entry does NOT carry citation_ref — citations live on the enclosing carrier (items[i].citation_ref). Each entry does NOT carry from_node / to_node — those live in the block-level edge_anchors[] field below. text_reference_fact means the visible source/config/doc/comment text itself is the evidence and must not be described as a definition/call/assignment proof. For call_edge/import_edge list items, render the principal relation with an explicit edge surface such as caller -> callee; boundary/comparison/exclusion prose that only mentions both endpoints should not use an arrow."},
          "edge_anchors": {"type": "array", "description": "Optional block-level array of typed edge-anchor entries that bind diagram edges to typed claim shapes. Use this when the block contributes evidence about a directed relation in a diagram (typically when the block is a diagram or when its items describe edge endpoints of a diagram in another block). Each entry shape: {from_node: string, to_node: string, relation_kind?: <one of call|guard|import|precedence|contain|observe>}. Both from_node and to_node MUST be the verbatim node identifier strings as they appear in the diagram body. PREFERRED: set relation_kind directly — when present it authoritatively names the basic semantic relation, and the rendered Mermaid label is then free prose for readability. When relation_kind is omitted, the validator falls back to recognising the relation from the rendered label vocabulary (see the diagram-edge label vocabulary section in the user message for the recognised keywords). Empty / absent = no edge anchor on this block (legitimate for non-diagram-edge blocks).", "items": {"type": "object"}},
          "facet_ids":    {"type": "array", "items": {"type": "string"}, "description": "Optional facet ids this block covers — read these from the user section's Required Answer Blocks list."},
          "surface_role": {"type": "string", "enum": ["principal"], "description": "Set to \"principal\" when this block carries the main-line answer payload (the user-section's Required Answer Blocks list flags which blocks expect principal). OMIT this field on supporting context, framing prose, and diagram-only contributions — absence is equivalent to not-principal."}
        },
        "required": ["id", "kind"]
      }
    },
    "citations": {
      "type": "array",
      "description": "Shared pool of file-line citations. Per-block / per-item citation_ref values are zero-based indexes into this array.",
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
	return json.RawMessage(schema)
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
