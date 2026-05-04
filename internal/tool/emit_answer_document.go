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
// fully retired. Only document_model="v2" is accepted. B3-F1
// (post_shape_retirement_consolidated_audit.md §8 Batch B3,
// 2026-05-04) tightened the gate further: empty / missing
// document_model is now rejected at the executor with an explicit
// "document_model must equal \"v2\"" error so schema / executor /
// type-level contract all say the same thing. The V2 schema is
// enforced in executeAnswerDocumentV2 (emit_answer_document_v2.go).
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
		"Allowed claim_form values: `definition_fact` (cited line establishes a typed fact: const, struct field, function signature, default value), `call_edge` (caller→callee call site), `guard_condition` (branch / condition gating the answer), `assignment_fact` (config / variable / field assignment), `return_fact` (return statement / function output), `absence_fact` (cited evidence carries Negative scope — search confirmed absent), `precedence_role` (cited evidence carries a layer / override role), `external_observation` (cited evidence is from runtime log / perf trace, not repo source), `import_edge` (module / package import edge). " +
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
		"    {\"id\":\"h1\",\"label\":\"Stage A\",\"text\":\"<what stage A does>\",\"kind\":\"principal\",\"claim_use\":{\"claim_form\":\"call_edge\",\"citation_ref\":0}},\n" +
		"    {\"id\":\"h2\",\"label\":\"Stage B\",\"text\":\"<what stage B does>\",\"kind\":\"principal\",\"claim_use\":{\"claim_form\":\"call_edge\",\"citation_ref\":1}}\n" +
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
		"    {\"id\":\"m1\",\"label\":\"MemberA\",\"text\":\"<role / why it belongs>\",\"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":0}},\n" +
		"    {\"id\":\"m2\",\"label\":\"MemberB\",\"text\":\"<role / why it belongs>\",\"claim_use\":{\"claim_form\":\"definition_fact\",\"citation_ref\":1}}\n" +
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
      "enum": ["v2"],
      "description": "Carrier marker. MUST equal \"v2\" (the only accepted carrier — V1 is retired). Empty / missing is rejected."
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
                "label":        {"type": "string", "description": "Primary visible text / row header. For enumeration items, MUST be the verbatim identifier copied from one of the evidence pool's anchor_symbol / subject / object values — fabricated labels are rejected by validateEnumerationItemLabelGrounding."},
                "text":         {"type": "string", "description": "Item body / row content."},
                "citation_ref": {"type": "integer", "description": "Top-level field on the item; zero-based index into citations[], or -1 when no cite backs the item. NEVER place citation_ref inside claim_use — it is rejected with 'unknown field \"citation_ref\"'."},
                "claim_use":    {"type": "object", "description": "Optional per-item claim annotation. Shape: {claim_form: <enum>, facet_id?: string, evidence_id?: string, surface_role?: <enum>}. Does NOT carry citation_ref (citation_ref is top-level on the item, not inside this object)."}
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
          "claim_uses":   {"type": "array", "description": "Block-level claim annotations array (the singular form claim_use does NOT exist at block level — only inside items[i].claim_use). REQUIRED on principal blocks (surface_role=principal) when the contract's AcceptableClaimForms list is non-empty. Each entry shape: {claim_form: <one of definition_fact|call_edge|guard_condition|assignment_fact|return_fact|absence_fact|precedence_role|external_observation|import_edge>, facet_id?: string, evidence_id?: string, surface_role?: <enum>}. Single-form blocks emit a one-element array (claim_uses=[{claim_form=definition_fact}]). Each entry does NOT carry citation_ref — citations live on the enclosing carrier (value.citation_ref / boolean.citation_ref / per-item items[i].citation_ref)."},
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
  "required": ["document_model", "blocks"]
}`
	return json.RawMessage(schema)
}

// Execute routes the emit to the V2 validator + writer. V1 carrier
// is retired (B8-T3); B3-F1 (2026-05-04) tightened the contract so
// empty / missing document_model is now an explicit fail-fast
// rejection here at the dispatch boundary, not silently routed to
// V2 to be rejected later by the executor — keeps schema /
// dispatch / executor / type-level all saying the same thing.
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
	model, ok, err := peekDocumentModel(params)
	if err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}
	if !ok {
		return failEmit(t.Name(), now,
			"document_model is required and must equal \"v2\" — V1 carrier is retired and empty / missing is rejected at the dispatch boundary")
	}
	if model != "v2" {
		return failEmit(t.Name(), now,
			"document_model=%q is not supported; only \"v2\" is accepted (V1 carrier retired at B8)", model)
	}
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

