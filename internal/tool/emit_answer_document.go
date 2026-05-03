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
		"(text for prose blocks, items[] for list/table blocks, diagram for diagram blocks). " +
		"\n\n" +
		"PRINCIPAL BLOCKS (the user-section's Required Answer Blocks list flags these as `surface_role=principal`) MUST carry a `claim_use` annotation when the contract's AcceptableClaimForms list is non-empty. " +
		"Allowed claim_form values: `definition_fact` (cited line establishes a typed fact), `mechanism_step` (one hop in a behavior chain), `enumeration_member` (one member of a closed set), `derivation_step` (intermediate computation), `call_edge` (caller→callee call site), `assignment` (config / variable assignment), `guard_condition` (branch / condition gating the answer), `observed_artifact_fact` (runtime / log fact), `divergence_caveat` (code-vs-comment divergence). " +
		"Use a single block-level `claim_use` for whole-block annotation; per-item `claim_use` (inside items[i].claim_use) when individual items carry distinct claim forms; block-level `claim_uses[]` array for blocks legitimately spanning multiple claim forms. " +
		"\n\n" +
		"DIAGRAM BLOCKS — `diagram.kind` is the SEMANTIC FAMILY (`flow` / `sequence` / `architecture` / `call_dag`), NOT a Mermaid keyword. Mermaid syntax (`flowchart` / `sequenceDiagram`) goes inside `diagram.body` with `diagram.language=\"mermaid\"`. " +
		"\n\n" +
		"Citations live in a shared `citations` pool; per-item `citation_ref` (and per-claim_use `citation_ref`) is a zero-based index into it (or -1 for no cite). " +
		"`exact_resolution`, `caveats[]`, `snippets[]` are document-level optional fields. " +
		"\n\n" +
		"V1 carrier (top-level shape / steps / symbols / value / boolean / summary) is retired and rejected at runtime." +
		"\n\n" +
		"WORKED EXAMPLES (minimal happy-path emits — each shows one principal-block family):\n" +
		"\n" +
		"1) Summary-only explanation (single principal BlockSummary):\n" +
		"```json\n" +
		"{\"document_model\":\"v2\",\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<multi-paragraph answer body>\",\"surface_role\":\"principal\",\"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":0}}\n" +
		"],\"citations\":[{\"file\":\"foo/bar.go\",\"line\":42}]}\n" +
		"```\n" +
		"\n" +
		"2) Hop-chain (BlockOrderedList over mechanism steps):\n" +
		"```json\n" +
		"{\"document_model\":\"v2\",\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<lead-in framing the chain>\"},\n" +
		"  {\"id\":\"hops\",\"kind\":\"ordered_list\",\"surface_role\":\"principal\",\n" +
		"   \"items\":[\n" +
		"    {\"id\":\"h1\",\"label\":\"Stage A\",\"text\":\"<what stage A does>\",\"kind\":\"principal\",\"claim_use\":{\"claim_form\":\"mechanism_step\",\"citation_ref\":0}},\n" +
		"    {\"id\":\"h2\",\"label\":\"Stage B\",\"text\":\"<what stage B does>\",\"kind\":\"principal\",\"claim_use\":{\"claim_form\":\"mechanism_step\",\"citation_ref\":1}}\n" +
		"   ]}\n" +
		"],\"citations\":[{\"file\":\"a.go\",\"line\":10},{\"file\":\"b.go\",\"line\":20}]}\n" +
		"```\n" +
		"\n" +
		"3) Enumeration slate (BlockOrderedList over enumeration members):\n" +
		"```json\n" +
		"{\"document_model\":\"v2\",\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<frames what the list enumerates>\"},\n" +
		"  {\"id\":\"slate\",\"kind\":\"ordered_list\",\"surface_role\":\"principal\",\n" +
		"   \"items\":[\n" +
		"    {\"id\":\"m1\",\"label\":\"MemberA\",\"text\":\"<role / why it belongs>\",\"claim_use\":{\"claim_form\":\"enumeration_member\",\"citation_ref\":0}},\n" +
		"    {\"id\":\"m2\",\"label\":\"MemberB\",\"text\":\"<role / why it belongs>\",\"claim_use\":{\"claim_form\":\"enumeration_member\",\"citation_ref\":1}}\n" +
		"   ]}\n" +
		"],\"citations\":[{\"file\":\"x.go\",\"line\":1},{\"file\":\"y.go\",\"line\":1}]}\n" +
		"```\n" +
		"\n" +
		"4) Single-literal scalar (BlockScalar):\n" +
		"```json\n" +
		"{\"document_model\":\"v2\",\"blocks\":[\n" +
		"  {\"id\":\"v1\",\"kind\":\"scalar\",\"text\":\"<names the subject + how the value was obtained>\",\"surface_role\":\"principal\",\n" +
		"   \"value\":{\"literal\":\"42\",\"citation_ref\":0},\n" +
		"   \"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":0}}\n" +
		"],\"citations\":[{\"file\":\"const.go\",\"line\":7}]}\n" +
		"```\n" +
		"\n" +
		"5) Architecture diagram (BlockDiagram with semantic family `architecture`):\n" +
		"```json\n" +
		"{\"document_model\":\"v2\",\"blocks\":[\n" +
		"  {\"id\":\"s1\",\"kind\":\"summary\",\"text\":\"<overall architecture lead-in>\",\"surface_role\":\"principal\",\"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":0}},\n" +
		"  {\"id\":\"d1\",\"kind\":\"diagram\",\n" +
		"   \"diagram\":{\"kind\":\"architecture\",\"language\":\"mermaid\",\"body\":\"flowchart TD\\n    A[\\\"<grounded node A>\\\"] --> B[\\\"<grounded node B>\\\"]\"}}\n" +
		"],\"citations\":[{\"file\":\"main.go\",\"line\":1}]}\n" +
		"```"
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
              "kind":     {"type": "string", "enum": ["flow", "sequence", "architecture", "call_dag"], "description": "SEMANTIC family of the diagram, NOT a Mermaid keyword. Use the family the contract names: flow (branches/guards), sequence (actor-to-actor over time), architecture (layered components), call_dag (one-to-many dispatch). Mermaid syntax tokens like \"flowchart\" and \"sequenceDiagram\" belong inside diagram.body, NOT here."},
              "language": {"type": "string", "description": "Diagram source language. Defaults to \"mermaid\" — the only currently rendered subset."},
              "body":     {"type": "string", "description": "Raw diagram source (the part inside fenced markers; the renderer adds the fences). For diagram.kind=flow/architecture/call_dag use Mermaid \"flowchart\" syntax (direction LR/TD/RL/BT); for diagram.kind=sequence use Mermaid \"sequenceDiagram\"."}
            }
          },
          "claim_use":    {"type": "object", "description": "Optional block-level claim annotation. REQUIRED on principal blocks (surface_role=principal) when the contract's AcceptableClaimForms list is non-empty. Shape: {claim_form: <one of definition_fact|mechanism_step|enumeration_member|derivation_step|call_edge|assignment|guard_condition|observed_artifact_fact|divergence_caveat>, citation_ref: <int>, surface_role?, facet_id?}. Use this for whole-block annotation (most scalar/decision/diagram blocks)."},
          "claim_uses":   {"type": "array", "description": "Optional block-level claim annotations array. Use this when one block legitimately spans multiple claim forms (e.g. an ordered_list mixing mechanism_step and guard_condition). Each entry has the same shape as claim_use."},
          "facet_ids":    {"type": "array", "items": {"type": "string"}, "description": "Optional facet ids this block covers — read these from the user section's Required Answer Blocks list."},
          "surface_role": {"type": "string", "enum": ["", "principal", "support", "prose_only", "diagram_only"], "description": "Block's role in the answer surface. principal = carries the answer payload (must usually attach claim_use); support = corroborates principal (e.g. anchor skeleton); prose_only = lead-in / framing; diagram_only = purely visual. The user-section's block list flags which role each Required Block expects."}
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

