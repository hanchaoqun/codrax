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
// fully retired. Only document_model="v2" (or empty / missing,
// which routes to V2 too) is accepted. The V2 schema is enforced
// in executeAnswerDocumentV2 (emit_answer_document_v2.go).
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
func (t *EmitAnswerDocument) Description() string {
	return "Emit the final answer as a structured block-only document via document_model=\"v2\" + blocks[]. " +
		"The answer is composed from one or more BLOCKS, each tagged by `kind`: " +
		"summary / section / ordered_list / bullet_list / scalar / decision / table / diagram / caveat. " +
		"Each block has a unique non-empty `id` and the kind-appropriate body fields " +
		"(Text for prose blocks, Items[] for list/table blocks, Diagram for diagram blocks). " +
		"Citations live in a shared `citations` pool; per-item `citation_ref` is a zero-based index into it (or -1 for no cite). " +
		"`exact_resolution`, `caveats[]`, `snippets[]` are document-level optional fields." +
		"\n\n" +
		"V1 carrier (top-level shape / steps / symbols / value / boolean / summary) is retired and rejected at runtime."
}

// Parameters returns the V2 JSON schema. Block kinds + per-block
// fields are LLM-facing; internal naming (BlockRequirement /
// AnswerSemanticView etc.) is intentionally absent.
func (t *EmitAnswerDocument) Parameters() json.RawMessage {
	const schema = `{
  "type": "object",
  "properties": {
    "document_model": {
      "type": "string",
      "enum": ["", "v2"],
      "description": "Carrier marker. Use \"v2\" (or omit) — only the block-only carrier is accepted."
    },
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
            "description": "Block kind. Required."
          },
          "title": {"type": "string", "description": "Optional sub-heading for section / table / diagram / caveat blocks."},
          "text": {"type": "string", "description": "Block body prose. Used by summary / section / scalar / decision / caveat. Markdown-flavoured."},
          "items": {
            "type": "array",
            "description": "Block items for ordered_list / bullet_list / table. For tables each item is one row.",
            "items": {
              "type": "object",
              "properties": {
                "id":           {"type": "string"},
                "label":        {"type": "string", "description": "Primary visible text / row header."},
                "text":         {"type": "string", "description": "Item body / row content."},
                "citation_ref": {"type": "integer", "description": "Zero-based index into citations[], or -1 when no cite backs the item."},
                "claim_use":    {"type": "object", "description": "Optional per-item claim annotation (claim_form / surface_role / facet_id)."}
              }
            }
          },
          "diagram": {
            "type": "object",
            "description": "Diagram payload for kind=diagram blocks. Body is the raw mermaid (or text) source.",
            "properties": {
              "kind":     {"type": "string", "description": "Diagram kind: flow / sequence / architecture / call_dag."},
              "language": {"type": "string", "description": "Diagram source language. Defaults to \"mermaid\"."},
              "body":     {"type": "string", "description": "Raw diagram source (the part inside fenced markers; the renderer adds the fences)."}
            }
          },
          "claim_uses":   {"type": "array", "description": "Optional block-level claim annotations."},
          "facet_ids":    {"type": "array", "items": {"type": "string"}, "description": "Optional facet ids this block covers."},
          "surface_role": {"type": "string", "enum": ["", "principal", "support", "prose_only", "diagram_only"]}
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
    "caveats":  {"type": "array", "items": {"type": "string"}, "description": "Optional document-level caveat strings (cross-block scope notes)."},
    "snippets": {"type": "array", "items": {"type": "object"}, "description": "Optional code snippets shown alongside the answer."}
  },
  "required": ["blocks"]
}`
	return json.RawMessage(schema)
}

// Execute routes the emit to the V2 validator + writer. V1 carrier
// is retired (B8-T3); empty / missing document_model is treated as
// V2 attempt and the V2 validator gives a clear error if the
// payload contains V1 fields.
func (t *EmitAnswerDocument) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	now := time.Now()
	if ctx == nil || ctx.Mutable == nil {
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   "emit_answer_document requires BusContext.Mutable; the caller did not provide one (sub-agents are not supported)",
			Timestamp: now,
		}, nil
	}
	if model, ok, err := peekDocumentModel(params); err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	} else if !ok || model == "" || model == "v2" {
		return executeAnswerDocumentV2(t.Name(), ctx, params, now)
	} else {
		return failEmit(t.Name(), now,
			"document_model=%q is not supported; only \"v2\" is accepted (V1 carrier retired at B8)", model)
	}
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
		types.ApplyAnswerSymbolStepBackbone(plan, ctx.AnswerSymbols, ctx.AnswerSymbolCompleteness)
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

