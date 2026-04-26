package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/types"
)

// EmitAnswerDocument is the structured finalizer channel. The
// finalizer LLM makes exactly one emit_answer_document call per
// dispatch, supplying a typed AnswerDocument, and a deterministic
// renderer (internal/render/answerdoc.go) turns the struct into
// user-visible prose.
//
// Classified ReadOnly because IsWrite() is the filesystem-write
// boundary; mutating BusContext is not a filesystem write.
// Classified NonEvidenceTool: the payload is the final answer slate,
// not a repo fact. Mirrors emit_answer_symbol on both axes.
//
// The structural defenses:
//
//  1. Pattern 1 (step_list collapse): AnswerDocument.Steps is a typed
//     slice; a collapse would have to drop elements the schema
//     declared, which the validator rejects.
//  2. Pattern 2 (line hallucination): every Citation.Line must be
//     > 0 and the File must not live under BusContext.WorkDir. Same
//     schema rules as emit_answer_symbol.
//  3. Pattern 3 (sibling drift): citations live in one pool indexed
//     by CitationRef integers — a single grounded cite is referenced
//     from every step that needs it, so there is no "re-render the
//     cite per step and accidentally drift" surface.
//  4. Pattern 4 (prose → fact): every typed field is required to
//     come from the evidence. The ONE prose escape hatch is Summary,
//     and it is length-capped per shape via types.SummaryCapFor only
//     when summary_cap_enabled is true.
type EmitAnswerDocument struct {
	ReadOnly
	NonEvidenceTool
}

// EmitAnswerDocumentProducer is the producer string stamped into
// result summaries so downstream logs can identify the channel
// without grepping for a literal.
const EmitAnswerDocumentProducer = "finalizer.emit_answer_document"

// parseEmitAnswerDocumentShape coerces the JSON string into a typed
// AnswerShape. ShapeNone is rejected — a producer emitting "no shape"
// is a bug, not a valid state. Cross-checks IsEmittable so a future
// enum addition that forgets to flip IsEmittable on flows through to
// both the types test and the tool test.
func parseEmitAnswerDocumentShape(s string) (types.AnswerShape, bool) {
	candidate := types.AnswerShape(strings.ToLower(strings.TrimSpace(s)))
	if !candidate.IsEmittable() {
		return "", false
	}
	return candidate, true
}

// emitAnswerDocumentBooleanDecision is the closed enum for the
// boolean shape's decision field. Accepts the bilingual forms the
// legacy P0.2 prose validator recognized plus the raw Go boolean
// literals. Defined as a function rather than a map so the error
// path returns early without a second lookup for the bool value.
func emitAnswerDocumentBooleanDecision(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "是":
		return true, true
	case "false", "no", "否":
		return false, true
	}
	return false, false
}

// emitAnswerDocumentParams mirrors types.AnswerDocument one-to-one
// except for Boolean.Decision, which is transported as a closed-enum
// string on the wire and translated to a bool at validation time.
// Decoding directly into the types.* structs avoids a separate layer
// of wire-format shims; the only translation step left is Boolean.
type emitAnswerDocumentParams struct {
	Shape               string                     `json:"shape"`
	Summary             string                     `json:"summary"`
	Steps               []types.AnswerStep         `json:"steps,omitempty"`
	Symbols             []emitAnswerSymbolItem     `json:"symbols,omitempty"`
	SymbolsCompleteness string                     `json:"symbols_completeness,omitempty"`
	Value               *types.AnswerValue         `json:"value,omitempty"`
	Boolean             *emitAnswerDocumentBoolean `json:"boolean,omitempty"`
	Citations           []types.Citation           `json:"citations,omitempty"`
	Caveats             []string                   `json:"caveats,omitempty"`
}

// emitAnswerDocumentBoolean carries the JSON form of the boolean
// payload (Decision as string). Every other sub-struct decodes
// straight into types.AnswerDocument; this one cannot because
// types.AnswerBoolean.Decision is a bool.
type emitAnswerDocumentBoolean struct {
	Decision    string  `json:"decision"`
	Rationale   string  `json:"rationale"`
	CitationRef FlexInt `json:"citation_ref"`
}

func (t *EmitAnswerDocument) Name() string { return "emit_answer_document" }

func (t *EmitAnswerDocument) Description() string {
	capGuidance := emitAnswerDocumentSummaryCapGuidance(false)
	return "Emit the final answer as a structured AnswerDocument in ONE call per finalizer dispatch. " +
		"Choose 'shape' from: list_of_symbols, step_list, value, boolean, config_value, explanation. " +
		"Shape-specific required fields: list_of_symbols → symbols[] + symbols_completeness; " +
		"step_list → steps[]; value → value{literal}; config_value → value{key,literal}; " +
		"boolean → boolean{decision,rationale}; explanation → summary. The 'citations' array is a " +
		"SHARED POOL; every steps[].citation_ref / value.citation_ref / boolean.citation_ref / " +
		"symbols is an integer INDEX into that pool (zero-based), or -1 when no citation backs " +
		"the entry. Every citation MUST have file (repo-relative), line > 0, and file must NOT live " +
		"inside the per-trace WorkDir. 'summary' is the only LLM-prose field. " + capGuidance + " " +
		"\n\n" +
		"IMPORTANT — citation quote field: the quote is OPTIONAL but when provided it MUST be a " +
		"VERBATIM copy of the characters at file:line from the read_file gutter (exact whitespace " +
		"and punctuation, whatever the source language). It is NOT a field for natural-language " +
		"prose, rationale, or paraphrase. The grounder cross-checks every quote against the " +
		"actual line text; a quote whose identifier tokens do not overlap with the cited line is " +
		"AUTOMATICALLY CLEARED. So your choices are: (1) paste the literal source line you saw " +
		"in read_file (best — the reader sees code context), or (2) omit the quote entirely. " +
		"Writing a one-sentence summary in the quote field wastes tokens because it will be " +
		"stripped before the answer ships. Rule of thumb: if the text you want to put in 'quote' " +
		"did not appear character-for-character on the read_file line at file:line, leave the " +
		"field empty and put the sentence in 'summary' instead.\n" +
		"\n" +
		"Unknown fields or shape-field mismatches are REJECTED with a clear error."
}

func (t *EmitAnswerDocument) Parameters() json.RawMessage {
	summaryDescription := "LLM-authored prose. " + emitAnswerDocumentSummaryCapGuidance(true) +
		" REQUIRED for 'explanation'; optional for all others."
	// symbols[].kind enum is sourced from types.AnswerSymbolKindSchemaEnum
	// so schema stays in lockstep with emit_answer_symbol's validator.
	return json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "shape": {"type": "string", "enum": ["list_of_symbols", "step_list", "value", "boolean", "config_value", "explanation"], "description": "Closed enum of answer shapes. REQUIRED. Choose the shape declared in the prompt's Target answer shape section."},
    "summary": {"type": "string", "description": %q},
    "steps": {
      "type": "array",
      "description": "Ordered steps for shape=step_list. REQUIRED for step_list; must be empty for all other shapes.",
      "items": {
        "type": "object",
        "properties": {
          "index":         {"type": "integer", "description": "1-based step number. Must be positive."},
          "description":   {"type": "string", "description": "The step body — describe what this step DOES in terms of behavior and outcome, reference load-bearing identifiers with inline `+"`"+`code`+"`"+`, and give the reader enough context to understand why it matters in the overall mechanism. Use as many sentences as accuracy and clarity require; one step is one logical hop, do not collapse two."},
          "citation_ref":  {"type": "integer", "description": "Index into citations[], or -1 when no citation backs this step."}
        },
        "required": ["index", "description", "citation_ref"]
      }
    },
    "symbols": {
      "type": "array",
      "description": "Answer-symbol list. REQUIRED for shape=list_of_symbols (the primary payload). OPTIONAL for shape=explanation when the analyzer produced sub_topics: emit ONE anchor symbol per sub-topic naming the load-bearing identifier, and the renderer will draw a Key Anchors skeleton beneath the summary. Must be empty for step_list / value / config_value / boolean.",
      "items": {
        "type": "object",
        "properties": {
          "name":      {"type": "string"},
          "file":      {"type": "string"},
          "line":      {"type": "integer"},
          "kind":      {"type": "string", "enum": [%s], "description": "Closed cross-language taxonomy — see types.AllAnswerSymbolKinds. Use 'literal' when the answer terminal is a value (string/number/bool) rather than a code identifier."},
          "chain":     {"type": "string"},
          "rationale": {"type": "string", "description": "Natural-prose description of what this symbol is and what role it plays in the mechanism. Reference load-bearing identifiers with inline `+"`"+`code`+"`"+`. Do not duplicate the location column — \"Defined at X. Used by Y\" is a regression, since file:line is already rendered as its own column in the output table."}
        },
        "required": ["name", "file", "line", "kind"]
      }
    },
    "symbols_completeness": {"type": "string", "enum": ["", "complete", "lower_bound", "unknown"], "description": "Set-level authority for the symbols slate. REQUIRED when shape=list_of_symbols. 'complete' is cross-checked against the expected answer count (the larger of: how many items the investigation found, and how many the classification declared required); mismatches downgrade to 'lower_bound'."},
    "value": {
      "type": "object",
      "description": "Concrete value payload. REQUIRED for shape=value (literal only) and shape=config_value (key + literal).",
      "properties": {
        "key":          {"type": "string", "description": "Config key path. Required for config_value, empty for plain value."},
        "literal":      {"type": "string", "description": "Verbatim value literal from evidence."},
        "citation_ref": {"type": "integer", "description": "Index into citations[], or -1 when no citation backs the value."}
      },
      "required": ["literal", "citation_ref"]
    },
    "boolean": {
      "type": "object",
      "description": "Boolean decision payload. REQUIRED for shape=boolean.",
      "properties": {
        "decision":     {"type": "string", "enum": ["true", "false", "yes", "no", "是", "否"], "description": "Boolean decision in closed form — no hedging."},
        "rationale":    {"type": "string", "description": "The reasoning behind the decision — name the invariant or guard that forces the answer and explain the mechanism at whatever depth the subtlety requires. Reference load-bearing identifiers with inline `+"`"+`code`+"`"+`. A terse rationale on a non-trivial decision is a regression."},
        "citation_ref": {"type": "integer", "description": "Index into citations[], or -1 when no citation backs the decision."}
      },
      "required": ["decision", "rationale", "citation_ref"]
    },
    "citations": {
      "type": "array",
      "description": "Shared citation pool. Each entry is one file:line anchor with an optional VERBATIM code quote. Zero-based indices; CitationRef=-1 means 'no citation'. The grounder validates quote tokens against the cited line — prose quotes are auto-cleared.",
      "items": {
        "type": "object",
        "properties": {
          "file":  {"type": "string", "description": "Repository-relative file path. MUST NOT live inside the per-trace WorkDir (blob directory)."},
          "line":  {"type": "integer", "description": "Gutter line number from read_file output. Must be > 0."},
          "quote": {"type": "string", "description": "OPTIONAL verbatim copy of the code at file:line from read_file, ≤%d chars (longer previews are truncated on a UTF-8 boundary; file:line is always preserved). Rule: paste the literal source line, or LEAVE THIS FIELD EMPTY. Do NOT write prose, summaries, or paraphrases — the grounder compares quote tokens against the actual line text and strips any quote that does not overlap (prose will be automatically cleared). If you cannot paste the literal line, omit the field."}
        },
        "required": ["file", "line"]
      }
    },
    "caveats": {
      "type": "array",
      "description": "Honesty markers the renderer surfaces at the bottom of the prose (ungrounded claims, incomplete investigation, conflicting evidence).",
      "items": {"type": "string"}
    }
  },
  "required": ["shape"]
}`, summaryDescription, types.AnswerSymbolKindSchemaEnum(), types.CitationMaxQuoteChars()))
}

func emitAnswerDocumentSummaryCapGuidance(schemaText bool) string {
	explanation := types.SummaryCapFor(types.ShapeExplanation, 0)
	value := types.SummaryCapFor(types.ShapeValue, 0)
	configValue := types.SummaryCapFor(types.ShapeConfigValue, 0)
	boolean := types.SummaryCapFor(types.ShapeBoolean, 0)
	stepOne := types.SummaryCapFor(types.ShapeStepList, 1)
	symbolOne := types.SummaryCapFor(types.ShapeListOfSymbols, 1)
	if explanation == types.SummaryCapUnlimited &&
		value == types.SummaryCapUnlimited &&
		configValue == types.SummaryCapUnlimited &&
		boolean == types.SummaryCapUnlimited &&
		stepOne == types.SummaryCapUnlimited &&
		symbolOne == types.SummaryCapUnlimited {
		if schemaText {
			return "No hard length cap is active in the current runtime config (default summary_cap_enabled=false); do not self-shorten below what accuracy and clarity require."
		}
		return "No hard length cap is active in the current runtime config (default summary_cap_enabled=false); write as much summary prose as the answer needs, and do not self-shorten for a pre-set budget."
	}
	return fmt.Sprintf(
		"Active hard summary caps: explanation <= %d chars; value <= %d; config_value <= %d; boolean <= %d; step_list and list_of_symbols scale with item count (1 item caps: %d / %d). If exceeded, the tool rejects with the exact cap.",
		explanation, value, configValue, boolean, stepOne, symbolOne)
}

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

	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	var p emitAnswerDocumentParams
	if err := dec.Decode(&p); err != nil {
		return failEmit(t.Name(), now, "invalid params: %v", err)
	}

	shape, ok := parseEmitAnswerDocumentShape(p.Shape)
	if !ok {
		return failEmit(t.Name(), now,
			"shape %q is not one of: list_of_symbols, step_list, value, boolean, config_value, explanation",
			p.Shape)
	}

	// Shape auto-correction: when the AnalysisIR declares a target
	// shape and the LLM chose a different one, silently override to
	// the target shape AND scrub fields that belong to the old shape.
	// This prevents infinite retry loops where the LLM consistently
	// picks the wrong shape (e.g. "boolean" for a list_of_symbols
	// question) and fills the wrong shape's required fields.
	if ctx.AnalysisIR == nil {
		logging.Debug("[emit_answer_document] AnalysisIR is nil — shape auto-correct disabled")
	}
	// shapeCorrectionNote tracks any auto-correct decision so it can
	// be surfaced in the tool Summary (not just DEBUG/WARN logs) —
	// the LLM on retry and the REPL operator both see the correction
	// happened and what the resolved shape is. Empty on no-correction.
	var shapeCorrectionNote string
	if ctx.AnalysisIR != nil {
		target := ctx.AnalysisIR.AnswerContract.RequiredAnswerShape
		logging.Debug("[emit_answer_document] AnalysisIR present, target shape=%s, LLM shape=%s", target, shape)
		if target != "" && target != shape {
			logging.Warning("[emit_answer_document] LLM chose shape=%s but AnalysisIR target is %s — auto-correcting", shape, target)
			llmShape := shape
			// Check if the LLM provided the required fields for the
			// target shape. If not, try to rescue from the extractor's
			// prior slate before falling back to explanation — the
			// extractor's emit_answer_symbol slate lives on
			// ctx.Mutable.EmittedAnswerSymbols and is frozen by the
			// time the finalizer runs.
			//
			// The rescue path exists because LLM finalizers sometimes
			// interpret "how many X" questions as boolean decisions
			// ("is the count > 1?") even when the analyzer resolved
			// target shape to list_of_symbols. Without the rescue, an
			// empty p.Symbols degrades to explanation with a one-line
			// summary — a silent regression from a fully-populated
			// prior slate. With the rescue, the symbols[] array is
			// populated from the slate and the shape-correct render
			// proceeds normally.
			canCorrect := true
			switch target {
			case types.ShapeListOfSymbols:
				if len(p.Symbols) == 0 && ctx.Mutable != nil {
					slate, _ := ctx.Mutable.EmittedAnswerSymbols()
					if len(slate) > 0 {
						logging.Warning("[emit_answer_document] rescuing symbols[] from extractor prior slate (%d items)", len(slate))
						p.Symbols = slateToEmittedSymbols(slate)
					}
				}
				canCorrect = len(p.Symbols) > 0
			case types.ShapeStepList:
				canCorrect = len(p.Steps) > 0
			case types.ShapeValue, types.ShapeConfigValue:
				// Numeric-enumeration bridge. "有几个 X" / "how many X"
				// can reasonably be answered as a count (value shape)
				// or as a list (list_of_symbols). When the analyzer
				// picked value but the LLM answered with symbols[],
				// synthesise value.literal from the symbol set
				// instead of falling back to explanation:
				//   - 1 symbol  → value.literal = symbols[0].name
				//   - N symbols → value.literal = N (count)
				// Either way the user's numeric intent is preserved.
				// CitationRef stays unset because the pool mapping
				// from symbols to value-anchor would be guesswork.
				if (p.Value == nil || strings.TrimSpace(p.Value.Literal) == "") && len(p.Symbols) > 0 {
					logging.Warning("[emit_answer_document] rescuing value{} from symbols[] (%d items)", len(p.Symbols))
					var literal string
					if len(p.Symbols) == 1 {
						literal = p.Symbols[0].Name
					} else {
						literal = strconv.Itoa(len(p.Symbols))
					}
					if p.Value == nil {
						p.Value = &types.AnswerValue{}
					}
					p.Value.Literal = literal
					p.Value.CitationRef = types.CitationRefUnset
					// Consume the source. symbols[] is forbidden under
					// the resolved value shape; leaving it populated
					// would trip rejectForbiddenFields downstream even
					// though its data was already absorbed into Value.
					p.Symbols = nil
				}
				canCorrect = p.Value != nil && p.Value.Literal != ""
			case types.ShapeBoolean:
				canCorrect = p.Boolean != nil && p.Boolean.Decision != ""
			}
			// Session 11 C4 — strict mode short-circuits the
			// silent rescue. When enabled, every shape mismatch is a
			// rejection so the LLM gets a structured retry hint from
			// F4 HintComposer on the next turn instead of a hidden
			// shape swap. Legacy mode (default false) preserves the
			// historical canCorrect / fallback-to-explanation path.
			if CurrentAnalysisLimits().ShapeSwapStrictMode {
				return types.ToolResult{
					ToolName: "emit_answer_document",
					Success:  false,
					Summary: fmt.Sprintf(
						"emit_answer_document REJECTED (C4 strict mode): "+
							"shape=%s but contract requires %s. "+
							"Re-emit with shape=%s and the fields that shape mandates.",
						llmShape, target, target),
					Timestamp: now,
				}, nil
			}
			if canCorrect {
				shape = target
				shapeCorrectionNote = fmt.Sprintf("shape auto-corrected: LLM chose %s → resolved to %s (analyzer target)", llmShape, shape)
			} else {
				logging.Warning("[emit_answer_document] target shape %s requires fields the LLM didn't fill — falling back to explanation", target)
				shape = types.ShapeExplanation
				shapeCorrectionNote = fmt.Sprintf("shape auto-corrected: LLM chose %s, target was %s but required fields empty → fell back to explanation", llmShape, target)
			}
			// CGEC B2a: also emit a structured RepairSwapShape so the
			// retry hint surfaces the mismatch explicitly. Keeping the
			// silent auto-correct as a per-round safety net — a run
			// that passes contract on the first try still ships. The
			// Repair only materialises into the next explore prompt if
			// a retry actually fires (ConsumeRepairs). Subject encodes
			// "from=X,to=Y" so the extractor (B3) can read it and
			// steer Turn B's answer_shape hint.
			if ctx.Mutable != nil {
				rationale := fmt.Sprintf("LLM chose shape=%s but AnalysisIR target is %s", llmShape, target)
				if !canCorrect {
					rationale += " (required fields missing, fell back to explanation)"
				}
				closure := ctx.Mutable.EvidenceClosure()
				closure.AddRepair(types.RepairDirective{
					Kind:      types.RepairSwapShape,
					Subject:   fmt.Sprintf("from=%s,to=%s", llmShape, target),
					Rationale: rationale,
					Origin:    "emit_answer_document.shape_mismatch",
				})
				logging.Info("[CGEC] B2a shape_swap: from=%s to=%s can_correct=%v", llmShape, target, canCorrect)
				// Session 11 F1: record structured ledger entry so F2
				// aggregator can promote repeated shape_swap events into
				// an IRPatch on answer_shape (e.g. the LLM consistently
				// emits boolean when contract says value → reconcile
				// toward value or downgrade to explanation). Confidence
				// 0.85 reflects that a shape_swap directly diagnoses the
				// answer_shape field with high specificity.
				closure.AppendViolation(types.Violation{
					Kind:   types.ViolShapeSwap,
					Detail: fmt.Sprintf("LLM shape=%s but contract requires %s (can_correct=%v)", llmShape, target, canCorrect),
					Stage:  string(types.StageFinalize),
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "answer_shape",
						Reason:     fmt.Sprintf("finalizer picked %s; contract wants %s", llmShape, target),
						Confidence: 0.85,
					},
				})
			}
			// Previously this block silently scrubbed every field the
			// resolved shape forbids. That let the LLM's "shotgun"
			// response (chose shape=boolean, filled boolean + steps +
			// symbols + value all at once — trace 1776448040358685830
			// iter=0) slip through: the tool returned ok=true with
			// everything except the single required field zeroed out,
			// the LLM never saw feedback, and the next retry pattern-
			// repeated. Session-8 behaviour: let the downstream shape
			// switch's rejectForbiddenFields fire. Non-zero forbidden
			// fields now fail the call with a specific message
			// ("shape=X forbids Y") so the LLM corrects on the next
			// turn. Zero-value forbidden objects are still silent-
			// nil'd by the sweep below (Value.Literal=="" → nil,
			// Boolean.Decision=="" → nil) — those are JSON noise, not
			// semantic conflict.
		}
	}

	// Scrub zero-value forbidden-field objects that the LLM tends to
	// include even when shape was NOT auto-corrected. An empty
	// value{literal=""} or boolean{decision=""} is semantically
	// absent; clearing the pointer prevents the forbidden-field
	// validator from rejecting the call.
	if p.Value != nil && p.Value.Literal == "" {
		p.Value = nil
	}
	if p.Boolean != nil && p.Boolean.Decision == "" {
		p.Boolean = nil
	}

	// Shape-tiered Summary cap. When summary_cap_enabled=false,
	// SummaryCapFor returns SummaryCapUnlimited and this branch is a
	// no-op. When enabled, scalar shapes carry fixed ceilings; step_list
	// and list_of_symbols scale with item count so an 8-step answer is
	// not compressed to the same budget as a 2-step one. Per-shape
	// numbers live in types.SummaryCapConfig (runtime-tunable via
	// codrax.yaml summary_cap_*).
	itemCount := len(p.Steps) + len(p.Symbols)
	summaryCap := types.SummaryCapFor(shape, itemCount)
	if len(p.Summary) > summaryCap {
		return failEmit(t.Name(), now,
			"summary length %d exceeds cap %d for shape=%s — shorten the summary",
			len(p.Summary), summaryCap, shape)
	}

	if err := validateAnswerDocumentNaturalLanguage(ctx, shape, p); err != nil {
		return failEmit(t.Name(), now, "%s", err.Error())
	}

	workDir := strings.TrimSpace(ctx.WorkDir)
	// Log-triage bundle snapshot — passed to per-item validators so
	// their rejection paths can redirect at the proper whole-shape
	// escape (symbols_completeness=unknown for external-source logs)
	// rather than emit a bland line-hallucination message that sends
	// the LLM into retry-loop territory. Nil-safe: external-source
	// gate on a nil bundle returns false, so non-log questions see
	// the historical behaviour unchanged.
	var docLogTriageBundle *types.LogBundle
	if ctx.Mutable != nil {
		docLogTriageBundle = ctx.Mutable.LogTriage()
	}
	// Grounding context: read_file gutter index + repomap graph from
	// Mutable.SearchGraph. Citations that fail grounding are either
	// dropped (file:line not in any source of truth) or keep the
	// anchor but have their Quote cleared (file:line exists but the
	// quote is not corroborated by the line text).
	groundCtx := ground.BuildContext(ctx)
	// Session-8 whitelist: citations must reference a file Turn A
	// actually read. Addresses the trace 1776448040358685830 case
	// where the finalizer LLM cited internal/agent/subagent.go:63 but
	// Turn A had read internal/types/subagent.go — two similar paths,
	// both indexed, LLM picked wrong. Whitelist rejects this at the
	// earliest point. Empty/nil set = skip (sub-agent / test paths).
	readFiles := turnAReadFileSet(ctx)
	// CGEC G1 pre-finalize dry-run. Before doing the (relatively
	// expensive) grounding pass — which reads file gutters, runs
	// symbol-table lookups, scans quote tokens — do a cheap
	// whitelist preflight: count how many citations are in
	// readFiles AT ALL. When ZERO pass the whitelist AND the LLM
	// supplied ≥1 citation, the grounder will drop every citation
	// and the answer will ship with 0 grounded references — a
	// guaranteed contract-check failure. Return a LOUD error now
	// so the finalizer's internal correction retry path kicks in
	// BEFORE we write out the full AnswerDocument. Also emits the
	// same RepairReadFile directives the grounder would have
	// emitted, so even if this early path doesn't trigger a
	// correction retry (empty readFiles, test path, ...), the
	// retry-hint renderer still surfaces the forced-read list.
	if dryMsg := simulateCitationGrounding(p.Citations, readFiles, groundCtx, ctx); dryMsg != "" {
		return types.ToolResult{
			ToolName:  t.Name(),
			Summary:   dryMsg,
			Timestamp: now,
			Success:   false,
		}, nil
	}
	// Session 11 C5 — literal form check. When the answer shape is
	// `value` and the contract knows the AnswerSubject.Kind, the
	// emitted value.literal must match the kind's regex form (e.g.
	// skill_name → must end with "-skill"). Catching the mismatch
	// here — before the AnswerDocument is written and the grounder
	// runs — gives the LLM a precise retry hint via the F4 composer
	// (the ViolLiteralFormFailed ledger entry drives it).
	if ctx.AnalysisIR != nil && shape == types.ShapeValue {
		if formErr := validateLiteralFormForSubject(&p, ctx.AnalysisIR.RequestModel.AnswerSubject.Kind); formErr != nil {
			if ctx.Mutable != nil {
				ctx.Mutable.EvidenceClosure().AppendViolation(types.Violation{
					Kind:   types.ViolLiteralFormFailed,
					Detail: formErr.Error(),
					Stage:  string(types.StageFinalize),
					SuspectedRoot: types.SuspectedRoot{
						IRField:    "answer_subject.kind",
						Reason:     fmt.Sprintf("emitted literal does not match kind=%s form", ctx.AnalysisIR.RequestModel.AnswerSubject.Kind),
						Confidence: 0.70,
					},
				})
			}
			return types.ToolResult{
				ToolName:  t.Name(),
				Summary:   "emit_answer_document REJECTED (C5 literal form check): " + formErr.Error(),
				Timestamp: now,
				Success:   false,
			}, nil
		}
	}
	citations, citationRemap, citationWarnings, citationRepairs, cerr := buildEmitAnswerDocumentCitations(p.Citations, workDir, groundCtx, readFiles)
	if cerr != nil {
		return failEmit(t.Name(), now, "%v", cerr)
	}
	applyCitationRemap(&p, citationRemap)
	numCites := len(citations)
	if dropped := countDroppedCitations(citationRemap); dropped > 0 || len(citationWarnings) > 0 {
		logging.Warning("[emit_answer_document] citation grounding: kept=%d dropped=%d warnings=%d", numCites, dropped, len(citationWarnings))
	}
	// CGEC D1: push the per-citation RepairDirectives onto the
	// closure so the orchestrator's renderWindowHint can drain them
	// into the next explore round's Forced Read List. AddRepair is
	// dedup-safe so repeated drops of the same file collapse into
	// one directive.
	if len(citationRepairs) > 0 && ctx.Mutable != nil {
		closure := ctx.Mutable.EvidenceClosure()
		for _, r := range citationRepairs {
			closure.AddRepair(r)
		}
	}
	// CGEC C5: record every kept citation onto the closure's
	// CitedRefs map so the convergence detector and pre-complete
	// check can read the per-Run citation pool without re-walking
	// the AnswerDocument.
	if numCites > 0 && ctx.Mutable != nil {
		closure := ctx.Mutable.EvidenceClosure()
		for _, c := range citations {
			closure.RecordCitation(c.File, c.Line)
		}
	}

	// Fix α (trace 1776450670620195562): failure paths must surface
	// every validation signal collected so far, not just the first
	// structural error that fires. Previously the shape-switch
	// rejectForbiddenFields error drowned out the citation warnings
	// ("kept=0 dropped=3 warnings=6") — the LLM on retry saw only
	// "forbids boolean{}", fixed nothing else, and resent a
	// byte-identical payload that got the SAME error. Two iterations
	// of that pattern burned the OpenAI quota.
	// failWithContext concatenates:
	//   (1) the primary error (usually a forbidden-field or
	//       required-field violation)
	//   (2) the shape-correction note if the auto-correct path fired
	//   (3) every citation-grounding warning (whitelist miss, Tier 2
	//       dead-zone, fabricated-quote drop, …)
	// so one retry round has a shot at fixing all of it.
	// Fix E: kept=0 escape-path block. When every LLM-supplied
	// citation was dropped by grounding, the LLM has no read_file
	// tool at finalize-time and cannot recover the cites by reading
	// new files. Without explicit guidance the LLM tends to retry
	// with byte-similar payloads (different framing, same wrong
	// file:line) and burn the iteration budget into an API
	// bad-request once a malformed-JSON retry slips through. Emit a
	// dedicated section that names the escape paths so the next-iter
	// retry has a direction.
	failWithContext := func(format string, args ...interface{}) (types.ToolResult, error) {
		var b strings.Builder
		fmt.Fprintf(&b, format, args...)
		if shapeCorrectionNote != "" {
			b.WriteString("\n\nShape correction: ")
			b.WriteString(shapeCorrectionNote)
		}
		if len(citationWarnings) > 0 {
			b.WriteString("\n\nCitation grounding (fix these on the same retry):")
			for _, w := range citationWarnings {
				b.WriteString("\n  - ")
				b.WriteString(w)
			}
		}
		if len(p.Citations) > 0 && numCites == 0 {
			b.WriteString("\n\nALL citations failed grounding. The finalizer agent has no read_file tool — you cannot recover by reading more files in this retry. Pick one of the escape paths instead of resubmitting the same file:line addresses (which will fail the same way):")
			b.WriteString("\n  (a) drop the un-citable items from the structured field (symbols[] / steps[] / value{} / boolean{}) and resubmit a smaller set with the appropriate completeness — the answer ships with whatever survives;")
			b.WriteString("\n  (b) when nothing survives at all, set symbols_completeness='unknown' (for list_of_symbols), or switch to shape='explanation' with a prose summary that names the subject by behaviour rather than by file:line;")
			b.WriteString("\n  (c) for a citation whose file:line came from prior-stage prose like '[relationship] X calls Y — file:line', that line is X's CALL SITE for Y, not X's definition — do not cite it as X's location.")
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   b.String(),
			Timestamp: now,
		}, nil
	}

	doc := &types.AnswerDocument{
		Shape:     shape,
		Summary:   strings.TrimSpace(p.Summary),
		Citations: citations,
	}

	// Session-22 fix F4.1 — diagram-block literal-grounding gate.
	//
	// The per-shape literal-grounding wrappers (value / steps / symbols
	// / boolean) check that each citation's ±3-line window overlaps
	// identifier tokens in the claim field, but they do NOT inspect
	// summary prose; and ShapeExplanation intentionally skips per-cite
	// corroboration because narrative prose shares identifier tokens
	// with citations ambiently. That exemption opened a hole: ASCII
	// call-chain / flow / sequence diagrams rendered inside fenced
	// code blocks ARE structural claims about specific repo files, and
	// the LLM can name a file there (e.g. "explorer.go (ParseOutput)
	// └─▶ buildAnalysisIR") without putting it in citations[].
	//
	// This gate scans every triple-backtick block in summary for
	// file-like tokens with known code extensions and rejects tokens
	// that do not appear in citations[] or the attached-log
	// ResolvedFiles allowlist. Applies to every shape — an explanation
	// or step_list answer can carry the same hallucinated diagram.
	if err := validateSummaryRequiredDiagram(p.Summary, ctx); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateSummaryDiagramGrounding(p.Summary, citations, groundCtx, ctx); err != nil {
		return failWithContext("%v", err)
	}

	// Log-triage coverage gate.
	//
	// When the user attached a runtime log and the log_triage
	// pre-stage extracted structured errors with a Caused-by /
	// Cause recursion chain, the answer MUST acknowledge every link
	// in the chain by naming each error Type. A summary that names
	// none of the real Types is almost certainly a hallucination.
	//
	// Generalisation: works across every signal family (panic /
	// crash / oom / timeout / db / network / validation / logic),
	// every answer shape (explanation / step_list / value / boolean /
	// list_of_symbols — coverage is checked against the `summary`
	// text which every shape carries), and every runtime language
	// (Java / Python / Go / Rust / ... — identifier-shape Type
	// tokens survive translation into any natural language because
	// class / exception names are not translated).
	if err := validateSummaryLogTriageCoverage(p.Summary, ctx); err != nil {
		return failWithContext("%v", err)
	}

	// Session-24 — codename-grounding gate.
	//
	// The LLM pattern-completes sequence labels from its prior: given
	// `Fallback S1` in the source it will often write `Fallback S2`
	// in the answer, populating an invented semantic ("max iterations
	// reached" / "phase==1 accepts stop") for the non-existent label.
	// End-to-end trace (2026-04-22) confirmed the finalizer USER prompt
	// contains zero evidence of the extended label in these failing
	// runs — so the confabulation is generation-side, not channel-leak.
	// This gate scans summary prose for codename-shape tokens and
	// rejects those absent from every citation's ±3-line window.
	// See project_session24_codename_confab_trace for trace evidence.
	if err := validateSummaryCodenameGrounding(p.Summary, citations, groundCtx); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateSummaryExactResolution(p.Summary, ctx); err != nil {
		return failWithContext("%v", err)
	}

	// Shape-dispatch: each branch validates its own required fields,
	// rejects fields that do not belong to this shape, and populates
	// the AnswerDocument slot the renderer will read.
	switch shape {
	case types.ShapeListOfSymbols:
		// Required-first gate: if required fields are missing, reject
		// with full context so the LLM knows the shape is unsatisfied.
		// If required fields are filled, forbidden fields are noise —
		// silent scrub with a surfaced note.
		if len(p.Symbols) == 0 {
			return failWithContext("shape=list_of_symbols requires symbols[] with at least one entry")
		}
		claimRaw := strings.ToLower(strings.TrimSpace(p.SymbolsCompleteness))
		if claimRaw == "" {
			return failWithContext("shape=list_of_symbols requires symbols_completeness (one of: complete, lower_bound, unknown)")
		}
		if note := scrubForbiddenNonZeroFields(&p, shape, forbidSteps|forbidValue|forbidBoolean); note != "" {
			shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
		}
		claim, cok := emitAnswerSymbolAllowedCompleteness[claimRaw]
		if !cok {
			return failWithContext("unknown symbols_completeness value %q (allowed: complete, lower_bound, unknown)", p.SymbolsCompleteness)
		}
		built := make([]types.AnswerSymbol, 0, len(p.Symbols))
		for i, in := range p.Symbols {
			sym, perr := buildEmitAnswerSymbolItem(in, i, workDir, docLogTriageBundle, groundCtx)
			if perr != nil {
				return failWithContext("symbols[%d]: %v", i, perr)
			}
			built = append(built, sym)
		}
		if err := validateSymbolsLiteralGrounding(built, groundCtx); err != nil {
			return failWithContext("%v", err)
		}
		doc.Symbols = built
		doc.SymbolsCompleteness = claim

	case types.ShapeStepList:
		if len(p.Steps) == 0 {
			return failWithContext("shape=step_list requires steps[] with at least one entry")
		}
		for i := range p.Steps {
			if err := validateStep(&p.Steps[i], i, numCites); err != nil {
				return failWithContext("%v", err)
			}
			p.Steps[i].Description = strings.TrimSpace(p.Steps[i].Description)
		}
		if err := validateStepsLiteralGrounding(p.Steps, citations, groundCtx); err != nil {
			return failWithContext("%v", err)
		}
		// Session-24 codename gate on step descriptions. Each step's
		// prose is a mini-summary and tends to carry the fabricated
		// label (u3a-20260422-010419 run-5 had `S2回退...` inside
		// steps[3].description, not in the top-level summary).
		for i, s := range p.Steps {
			if err := validateSummaryCodenameGrounding(s.Description, citations, groundCtx); err != nil {
				return failWithContext("steps[%d]: %v", i, err)
			}
		}
		if note := scrubForbiddenNonZeroFields(&p, shape, forbidSymbols|forbidValue|forbidBoolean); note != "" {
			shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
		}
		doc.Steps = p.Steps

	case types.ShapeValue:
		if p.Value == nil {
			return failWithContext("shape=value requires value{literal, citation_ref}")
		}
		if err := validateValueField(p.Value, false, numCites); err != nil {
			return failWithContext("%v", err)
		}
		if err := validateValueLiteralGrounding(p.Value, citations, groundCtx, false); err != nil {
			return failWithContext("%v", err)
		}
		// Fix G1: hard-reject empty / too-short summary on shape=value.
		// The Fix F prompt change ("ALWAYS fill summary with 1-2 sentences
		// naming subject + methodology") was advisory and the LLM honoured
		// it ~50% of the time; the bare-literal answer "值为 \`<literal>\`."
		// rendered as 10 chars and failed every downstream readability gate.
		// Hard validator forces a fill so the answer body always names
		// what was measured and how, not just the scalar.
		if err := validateValueShapeSummary(p.Summary, p.Value); err != nil {
			return failWithContext("%v", err)
		}
		if note := scrubForbiddenNonZeroFields(&p, shape, forbidSteps|forbidSymbols|forbidBoolean); note != "" {
			shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
		}
		doc.Value = p.Value

	case types.ShapeConfigValue:
		if p.Value == nil {
			return failWithContext("shape=config_value requires value{key, literal, citation_ref}")
		}
		if err := validateValueField(p.Value, true, numCites); err != nil {
			return failWithContext("%v", err)
		}
		if err := validateValueLiteralGrounding(p.Value, citations, groundCtx, true); err != nil {
			return failWithContext("%v", err)
		}
		// Fix G1 mirror: same summary requirement for config_value.
		if err := validateValueShapeSummary(p.Summary, p.Value); err != nil {
			return failWithContext("%v", err)
		}
		if note := scrubForbiddenNonZeroFields(&p, shape, forbidSteps|forbidSymbols|forbidBoolean); note != "" {
			shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
		}
		doc.Value = p.Value

	case types.ShapeBoolean:
		if p.Boolean == nil {
			return failWithContext("shape=boolean requires boolean{decision, rationale, citation_ref}")
		}
		bl, berr := buildEmitAnswerDocumentBoolean(p.Boolean, numCites)
		if berr != nil {
			return failWithContext("%v", berr)
		}
		if err := validateBooleanLiteralGrounding(bl, citations, groundCtx); err != nil {
			return failWithContext("%v", err)
		}
		if note := scrubForbiddenNonZeroFields(&p, shape, forbidSteps|forbidSymbols|forbidValue); note != "" {
			shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
		}
		doc.Boolean = bl

	case types.ShapeExplanation:
		// 2026-04-17: explanation shape allows symbols[] as an anchor
		// skeleton for multi-topic answers. The extractor emits one
		// answer_symbol per sub-topic naming the load-bearing
		// identifier, and the renderer draws a Key Anchors block
		// beneath the summary prose. symbols_completeness is NOT
		// required in this mode — the skeleton is auxiliary, not the
		// answer body. steps / value / boolean remain forbidden.
		if strings.TrimSpace(p.Summary) == "" {
			return failWithContext("shape=explanation requires a non-empty summary")
		}
		if len(p.Symbols) > 0 {
			built := make([]types.AnswerSymbol, 0, len(p.Symbols))
			for i, in := range p.Symbols {
				sym, perr := buildEmitAnswerSymbolItem(in, i, workDir, docLogTriageBundle, groundCtx)
				if perr != nil {
					return failWithContext("symbols[%d]: %v", i, perr)
				}
				built = append(built, sym)
			}
			doc.Symbols = built
			// Completeness is not part of the explanation-skeleton
			// contract; leave SymbolsCompleteness zero-valued so the
			// renderer can branch on "skeleton vs. enumeration".
		}
		if note := scrubForbiddenNonZeroFields(&p, shape, forbidSteps|forbidValue|forbidBoolean); note != "" {
			shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
		}
	}

	if len(p.Caveats) > 0 {
		doc.Caveats = append([]string(nil), p.Caveats...)
	}

	// Populate deterministic render-only code snippets from the
	// read_file gutter index at each citation line. The LLM never
	// writes into doc.Snippets; it is verifiable against ground
	// truth. See extractCodeSnippets for the algorithm.
	doc.Snippets = extractCodeSnippets(ctx, doc, 5)

	ctx.Mutable.SetAnswerDocument(doc)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitAnswerDocumentSummary(doc, citationWarnings, shapeCorrectionNote),
		Timestamp: now,
	}, nil
}

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

func answerDocumentRequiresChinese(lang string) bool {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "zh", "zh-cn", "cn", "chinese":
		return true
	}
	return false
}

func containsHanRune(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func validateAnswerDocumentNaturalLanguage(ctx *types.BusContext, shape types.AnswerShape, p emitAnswerDocumentParams) error {
	if !answerDocumentRequiresChinese(requestedAnswerDocumentLanguage(ctx)) {
		return nil
	}
	checkChinese := func(field, value string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if containsHanRune(value) {
			return nil
		}
		return fmt.Errorf("%s must be written in Simplified Chinese for this request; keep code identifiers as-is but rewrite the natural-language prose in Chinese", field)
	}

	if err := checkChinese("summary", p.Summary); err != nil {
		return err
	}
	for i, step := range p.Steps {
		if err := checkChinese(fmt.Sprintf("steps[%d].description", i), step.Description); err != nil {
			return err
		}
	}
	for i, sym := range p.Symbols {
		if err := checkChinese(fmt.Sprintf("symbols[%d].rationale", i), sym.Rationale); err != nil {
			return err
		}
	}
	if p.Boolean != nil {
		if err := checkChinese("boolean.rationale", p.Boolean.Rationale); err != nil {
			return err
		}
	}
	for i, caveat := range p.Caveats {
		if err := checkChinese(fmt.Sprintf("caveats[%d]", i), caveat); err != nil {
			return err
		}
	}
	_ = shape
	return nil
}

// countDroppedCitations returns the number of -1 entries in a
// citation remap — i.e. the count of citations that failed grounding
// and were pulled from the pool. Zero when every citation grounded
// successfully.
// turnAReadFileSet returns the set of file paths Turn A actually read.
// Drives the citation-whitelist check in buildEmitAnswerDocumentCitations.
// Returns nil for contexts that predate the TurnA snapshot (sub-agent
// dispatches, unit tests with bare Mutable) so those paths stay
// backward-compatible and skip the whitelist.
func turnAReadFileSet(ctx *types.BusContext) map[string]bool {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	ta := ctx.Mutable.TurnAArtifacts()
	if ta == nil || len(ta.ReadFiles) == 0 {
		return nil
	}
	set := make(map[string]bool, len(ta.ReadFiles))
	for _, f := range ta.ReadFiles {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		// Canonicalise so the whitelist compares repo-relative against
		// repo-relative regardless of whether the LLM read the file via
		// relative or absolute path. Must stay in sync with the
		// per-citation canonicalisation in buildEmitAnswerDocumentCitations
		// — both sides of the `!readFiles[c.File]` lookup have to agree.
		set[ground.CanonicalRepoRelative(f, ctx.RepoRoot)] = true
	}
	return set
}

func countDroppedCitations(remap []int) int {
	n := 0
	for _, r := range remap {
		if r < 0 {
			n++
		}
	}
	return n
}

// forbidBitset marks which AnswerDocument payload fields must be
// empty for a given shape. Each shape-dispatch branch ORs together
// the fields it does NOT accept and passes the mask to
// rejectForbiddenFields. Keeping the rule as a bitset (instead of a
// per-shape switch inside the validator) means each shape's
// forbidden-field list is literally visible at its branch in Execute,
// so a shape's contract is one-grep obvious. Non-zero forbidden
// fields reject the call with a specific error; zero-valued objects
// are tolerated (sweep above the shape switch clears them silently).
type forbidBitset uint

const (
	forbidSteps forbidBitset = 1 << iota
	forbidSymbols
	forbidValue
	forbidBoolean
)

// rejectForbiddenFields rejects the call when a payload field is
// forbidden for the given shape AND carries non-zero content. Zero
// value objects (already nil'd by the empty-struct sweep above the
// shape switch) are silently tolerated — LLMs habitually include an
// empty `value{}` or `boolean{}` as a JSON-default even on unrelated
// shapes, and penalising that noise would reject every call.
//
// The earlier behaviour silently scrubbed non-zero forbidden fields
// and logged a debug line, on the theory that LLMs "cannot fix the
// issue through retries". In practice the silent scrub LET the LLM
// keep the wrong field for three consecutive iterations (trace
// 1776439797257469553: boolean{decision:"否",rationale:"..."} kept
// on an explanation answer) because it never saw the feedback.
// Failing loudly gives the LLM a structured error it can correct on
// the next turn; the finalizer's soft-stop correction budget (see
// agent_finalizer_max_correction_retries) bounds the retry loop.
func rejectForbiddenFields(p *emitAnswerDocumentParams, shape types.AnswerShape, mask forbidBitset) error {
	if mask&forbidSteps != 0 && len(p.Steps) > 0 {
		return fmt.Errorf("shape=%s forbids steps[]; remove the %d step(s) and retry", shape, len(p.Steps))
	}
	if mask&forbidSymbols != 0 && len(p.Symbols) > 0 {
		return fmt.Errorf("shape=%s forbids symbols[]; remove the %d symbol(s) and retry", shape, len(p.Symbols))
	}
	if mask&forbidValue != 0 && p.Value != nil {
		return fmt.Errorf("shape=%s forbids value{}; remove the field and retry", shape)
	}
	if mask&forbidBoolean != 0 && p.Boolean != nil {
		return fmt.Errorf("shape=%s forbids boolean{}; remove the field and retry", shape)
	}
	return nil
}

// joinNote concatenates two Summary notes with a "; " separator,
// skipping empties. Used to layer the shape-auto-correct message
// and the scrubbed-forbidden-fields message without fragile string
// assembly at each call site.
func joinNote(existing, added string) string {
	existing = strings.TrimSpace(existing)
	added = strings.TrimSpace(added)
	if existing == "" {
		return added
	}
	if added == "" {
		return existing
	}
	return existing + "; " + added
}

// scrubForbiddenNonZeroFields nil's non-zero forbidden fields and
// returns a human-readable description of what was scrubbed (empty
// when nothing needed scrubbing). Session-8 Fix α' (trace
// 1776453454793969437): once the shape's required fields are
// already satisfied, forbidden fields are JSON-noise — the answer
// is valid without them. Rejecting the whole call forces the LLM
// to re-emit from scratch, which it may never figure out (the trace
// showed 18 iterations of "shape=step_list forbids boolean{}"
// with steps[] filled correctly on every one). Silently scrubbing
// and appending a note to the success Summary keeps forward
// progress while still telling the LLM what we dropped.
//
// The reject path (rejectForbiddenFields) is the right call when
// required fields are MISSING — that's a shape confusion the LLM
// should fix. The scrub path is for "answer's there, just cluttered".
func scrubForbiddenNonZeroFields(p *emitAnswerDocumentParams, shape types.AnswerShape, mask forbidBitset) string {
	var scrubbed []string
	if mask&forbidSteps != 0 && len(p.Steps) > 0 {
		scrubbed = append(scrubbed, fmt.Sprintf("steps[] (%d items)", len(p.Steps)))
		p.Steps = nil
	}
	if mask&forbidSymbols != 0 && len(p.Symbols) > 0 {
		scrubbed = append(scrubbed, fmt.Sprintf("symbols[] (%d items)", len(p.Symbols)))
		p.Symbols = nil
	}
	if mask&forbidValue != 0 && p.Value != nil {
		scrubbed = append(scrubbed, "value{}")
		p.Value = nil
	}
	if mask&forbidBoolean != 0 && p.Boolean != nil {
		scrubbed = append(scrubbed, "boolean{}")
		p.Boolean = nil
	}
	if len(scrubbed) == 0 {
		return ""
	}
	return fmt.Sprintf("shape=%s scrubbed unused field(s): %s (answer body is valid without them)",
		shape, strings.Join(scrubbed, ", "))
}

func validateStep(s *types.AnswerStep, index int, numCites int) error {
	if strings.TrimSpace(s.Description) == "" {
		return fmt.Errorf("steps[%d]: description is required", index)
	}
	if s.Index <= 0 {
		return fmt.Errorf("steps[%d]: index must be > 0 (got %d)", index, s.Index)
	}
	return validateCitationRef("steps", index, s.CitationRef, numCites)
}

func validateValueField(v *types.AnswerValue, requireKey bool, numCites int) error {
	v.Literal = strings.TrimSpace(v.Literal)
	if v.Literal == "" {
		return fmt.Errorf("value.literal is required")
	}
	v.Key = strings.TrimSpace(v.Key)
	if requireKey && v.Key == "" {
		return fmt.Errorf("shape=config_value requires value.key (the config-key path)")
	}
	return validateCitationRef("value", 0, v.CitationRef, numCites)
}

// valueLiteralTokenRe extracts identifier-shaped tokens from claim
// text / cited line text for the session-22 literal-corroboration
// gate. Matches ASCII identifier shape (letters / digits /
// underscore, must start with letter/underscore, min 3 chars).
// Filters out trivial substrings that would false-match almost
// anything.
var valueLiteralTokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// corroborationWindow is the ±N-line slack the literal-grounding
// gate inspects around the cited line. Three captures a
// reasonable "the cite points at a symbol's opening / definition
// block" range (doc comment, signature, first body lines) without
// opening the gate to false-positives on unrelated neighbouring
// code.
const corroborationWindow = 3

// requireCitationCorroboration is the SHAPE-AGNOSTIC
// literal-grounding gate shipped as part of the session-22 citation
// fabrication defence. Given a text blob that represents a claim
// (value.Literal, symbol.Name, step.Description, boolean.Rationale)
// and the file:line the LLM chose to back it, the helper reports an
// error when the cited ±3-line window contains NO identifier token
// present in the claim text.
//
// Shape coverage (all 5 citation-carrying shapes):
//
//	ShapeValue         → validateValueLiteralGrounding
//	ShapeConfigValue   → validateValueLiteralGrounding (unions Key)
//	ShapeListOfSymbols → validateSymbolsLiteralGrounding (per-item)
//	ShapeStepList      → validateStepsLiteralGrounding  (per-item)
//	ShapeBoolean       → validateBooleanLiteralGrounding
//
// ShapeExplanation intentionally skipped — summary is freeform prose
// and citations[] are ad-lib references; per-citation corroboration
// would over-match on identifier tokens the LLM mentions in narrative
// unrelated to any specific cite.
//
// Contract (identical across all wrappers):
//
//   - unseedable anchor (citationRef == -1 for ref-based shapes, or
//     File == "" / Line <= 0 for direct-anchor shapes like AnswerSymbol)
//     → bypass. These are the LLM's honest escape hatches.
//   - empty LineIndex or unindexed file → skip (degrades gracefully
//     for sub-agent paths and unit tests without a line index).
//   - claim text has no identifier-shape tokens → skip (numeric
//     literals, short sentinels, prose with only noise words).
//   - at least one token overlap in the ±3-line window → grounded.
//   - otherwise → return a formatted error with the schema-legal
//     escape spelled out.
//
// The caller supplies the shape-specific labels (what to call the
// claim in the error message, how to name the escape route) so the
// LLM sees actionable guidance no matter which shape it is emitting.
func requireCitationCorroboration(claim, citeFile string, citeLine int, gc *ground.Context, cfg corroborationCfg) error {
	if gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}
	if citeFile == "" || citeLine <= 0 {
		return nil
	}
	fileLines, ok := gc.LineIndex[citeFile]
	if !ok || len(fileLines) == 0 {
		return nil
	}
	tokens := valueLiteralTokenRe.FindAllString(claim, -1)
	for _, extra := range cfg.extraClaims {
		tokens = append(tokens, valueLiteralTokenRe.FindAllString(extra, -1)...)
	}
	if len(tokens) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		wanted[tok] = true
	}
	for line := citeLine - corroborationWindow; line <= citeLine+corroborationWindow; line++ {
		if line <= 0 {
			continue
		}
		text, ok := fileLines[line]
		if !ok {
			continue
		}
		for _, tok := range valueLiteralTokenRe.FindAllString(text, -1) {
			if wanted[tok] {
				return nil
			}
		}
	}
	return fmt.Errorf("%s is not corroborated by %s (%s:%d): the cited line and ±%d-line window contain no identifier overlap with the claim. %s",
		cfg.claimLabel, cfg.citeLabel, citeFile, citeLine, corroborationWindow, cfg.escape)
}

// corroborationCfg holds the shape-specific strings the
// requireCitationCorroboration helper interpolates into error
// messages. Kept on its own struct so every shape wrapper is a
// single call with its own copy of labels and escape text.
type corroborationCfg struct {
	claimLabel  string   // e.g. `value.literal "X"`, `symbols[2].name "Y"`
	citeLabel   string   // e.g. `citations[0]`, `symbols[2].file/line`
	escape      string   // shape-specific retry guidance
	extraClaims []string // additional tokens to union into the claim set (e.g. value.Key for ConfigValue)
}

// validateValueShapeSummary enforces a non-empty Summary on shape=value
// and shape=config_value answers. The bare literal alone is rarely
// useful — the reader needs to know WHAT was measured (the subject
// path / symbol / directory) and HOW (the command, chain, lookup) so
// the answer body is auditable. Fix G1 hard-gates this contract:
// the prompt-only Fix F instruction was advisory and the LLM honoured
// it ~50% of the time; a hard validator brings the failure mode to
// 0% by forcing the LLM to either fill summary or accept the reject.
//
// The threshold (40 visible chars) is set so a one-sentence summary
// containing the subject + verb + measurement target clears it
// (typical: "internal/tool 目录下所有非测试 .go 文件总行数为 17677，
// 由 find+wc 命令测得" = ~40+ chars). Bare "值为 \`X\`." (10 chars)
// fails. Single-word qualifiers like "总行数" alone (3 chars) fail.
func validateValueShapeSummary(summary string, v *types.AnswerValue) error {
	trimmed := strings.TrimSpace(summary)
	const minSummaryChars = 40
	if len([]rune(trimmed)) < minSummaryChars {
		literal := ""
		if v != nil {
			literal = v.Literal
		}
		return fmt.Errorf("shape=value requires summary to name the subject (file path / symbol / directory / measurement target) AND the methodology (command, chain, lookup) that produced the literal — the bare literal %q alone is not a complete answer (current summary length %d runes, minimum %d). The renderer prints summary first, then the literal — readers need both to act on or audit the value. Examples of acceptable summary content: state what was measured (the entity from the question), how it was measured (the find/grep/wc/exec command or chain that produced the value), and any non-obvious scope notes (excluded files, traversal direction, etc.)",
			literal, len([]rune(trimmed)), minSummaryChars)
	}
	return nil
}

// validateValueLiteralGrounding is the ShapeValue / ShapeConfigValue
// wrapper. Retained for source compatibility — call sites in
// Execute still call this by name.
func validateValueLiteralGrounding(v *types.AnswerValue, citations []types.Citation, gc *ground.Context, isConfigValue bool) error {
	if v == nil || v.CitationRef < 0 {
		return nil
	}
	if v.CitationRef >= len(citations) {
		return nil
	}
	cite := citations[v.CitationRef]
	cfg := corroborationCfg{
		claimLabel: fmt.Sprintf("value.literal %q", v.Literal),
		citeLabel:  fmt.Sprintf("citations[%d]", v.CitationRef),
		escape: "If this literal originates from the attached log / external source rather than repo code, " +
			"set citation_ref=-1 and state in summary that the answer is derived from log semantics (no grounded repo source). " +
			"Otherwise cite a real file:line where the literal appears.",
	}
	if isConfigValue && v.Key != "" {
		cfg.extraClaims = []string{v.Key}
	}
	return requireCitationCorroboration(v.Literal, cite.File, cite.Line, gc, cfg)
}

// validateSymbolsLiteralGrounding is the ShapeListOfSymbols wrapper.
// Each items[i] carries its own File/Line (no CitationRef indirection
// on AnswerSymbol). The Name IS the literal being claimed — the
// cited line should contain Name as an identifier token for the
// citation to be grounded.
//
// Customer-trace pattern this closes: goroutine_dump paste declared
// `main.writeSession` at `internal/agent/analyzer.go:100` in the
// frames, but `writeSession` does not exist in the repo. The
// path/line pair passed os.Stat + the grounder's Tier-1 check
// because analyzer.go really does have a line 100 — but the line's
// content has zero overlap with `writeSession`, so this gate fires.
func validateSymbolsLiteralGrounding(symbols []types.AnswerSymbol, gc *ground.Context) error {
	for i, s := range symbols {
		if s.File == "" || s.Line <= 0 || s.Name == "" {
			continue
		}
		cfg := corroborationCfg{
			claimLabel: fmt.Sprintf("symbols[%d].name %q", i, s.Name),
			citeLabel:  fmt.Sprintf("symbols[%d].file/line", i),
			escape: "If this symbol name is drawn from the attached log / an external trace rather than real repo code, " +
				"drop the item (or set answer_symbols_completeness='unknown') — do NOT invent a repo file:line for a symbol the repo does not define. " +
				"Otherwise cite a real file:line where the symbol appears.",
		}
		if err := requireCitationCorroboration(s.Name, s.File, s.Line, gc, cfg); err != nil {
			return err
		}
	}
	return nil
}

// validateStepsLiteralGrounding is the ShapeStepList wrapper. Each
// steps[i].CitationRef points into citations[], and the step's
// Description should contain identifier tokens the cited line also
// mentions. Purely narrative steps ("the request is processed
// asynchronously" with no identifiers) bypass via the
// no-identifier-token skip.
func validateStepsLiteralGrounding(steps []types.AnswerStep, citations []types.Citation, gc *ground.Context) error {
	for i, s := range steps {
		if s.CitationRef < 0 || s.CitationRef >= len(citations) {
			continue
		}
		cite := citations[s.CitationRef]
		cfg := corroborationCfg{
			claimLabel: fmt.Sprintf("steps[%d].description %q", i, s.Description),
			citeLabel:  fmt.Sprintf("citations[%d]", s.CitationRef),
			escape: "If this step paraphrases an attached log / external source without a repo anchor, " +
				"set citation_ref=-1 so the renderer drops the suffix. " +
				"Otherwise cite a real file:line that contains an identifier named in the step's description.",
		}
		if err := requireCitationCorroboration(s.Description, cite.File, cite.Line, gc, cfg); err != nil {
			return err
		}
	}
	return nil
}

// validateBooleanLiteralGrounding is the ShapeBoolean wrapper. The
// Rationale sentence is the claim; the cited line should mention
// at least one identifier the rationale names. Fire / bypass rules
// match the value branch.
func validateBooleanLiteralGrounding(b *types.AnswerBoolean, citations []types.Citation, gc *ground.Context) error {
	if b == nil || b.CitationRef < 0 {
		return nil
	}
	if b.CitationRef >= len(citations) {
		return nil
	}
	cite := citations[b.CitationRef]
	cfg := corroborationCfg{
		claimLabel: fmt.Sprintf("boolean.rationale %q", b.Rationale),
		citeLabel:  fmt.Sprintf("citations[%d]", b.CitationRef),
		escape: "If the rationale draws on attached-log / external-source content rather than repo code, " +
			"set citation_ref=-1 and let the rationale stand on its own. " +
			"Otherwise cite a real file:line that contains an identifier named in the rationale.",
	}
	return requireCitationCorroboration(b.Rationale, cite.File, cite.Line, gc, cfg)
}

// diagramFileExtensions is the allow-list of code-file extensions the
// fenced-block grounding gate treats as load-bearing. A token like
// `analyzer.go` or `config.yaml` inside an ASCII diagram is a concrete
// claim about a specific repo file, so the gate verifies it. Tokens
// with extensions outside this set (log lines, stdout pastes, generic
// nouns that happen to contain a dot) are skipped to avoid
// false-positive rejections on auxiliary prose.
var diagramFileExtensions = map[string]bool{
	".go": true, ".py": true, ".java": true, ".js": true, ".ts": true,
	".tsx": true, ".jsx": true, ".rs": true, ".rb": true, ".cpp": true,
	".cc": true, ".cxx": true, ".c": true, ".h": true, ".hpp": true,
	".kt": true, ".kts": true, ".swift": true, ".scala": true,
	".php": true, ".sql": true, ".proto": true, ".yaml": true, ".yml": true,
	".toml": true, ".json": true, ".sh": true, ".mk": true,
}

// diagramFileTokenRe matches file-like tokens inside fenced code
// blocks: an optional slash-delimited path, a base name, a literal
// dot, and a 1–6 letter extension, with an optional `:N` line suffix.
// The extension arm requires letters only so identifier accesses like
// `foo.Bar` or numeric patterns like `1.0.0` do not match.
var diagramFileTokenRe = regexp.MustCompile(`[A-Za-z0-9_./-]+\.[A-Za-z]{1,6}(?::\d+)?`)

// fencedCodeBlockRe captures the body of every triple-backtick fenced
// block in the summary. The (?s) flag lets `.` cross newlines so the
// full block body is returned in submatch[1]. Anchoring on `\n` after
// the opening fence skips the optional language tag.
var fencedCodeBlockRe = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

// diagramCueTokens is the lightweight structural heuristic for the
// missing-diagram gate. We intentionally keep the gate generic: any
// fenced block with at least two non-empty lines plus one of these
// cues counts as a diagram-like block, regardless of answer shape.
var diagramCueTokens = []string{
	"->", "<-", "=>", "<=", "──", "│", "├", "└", "┌", "┐", "┘", "┤",
	"┬", "┴", "◀", "▶", "▲", "▼",
}

func answerDiagramContract(ctx *types.BusContext) *types.DiagramContract {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.AnswerContract.Diagram == nil {
		return nil
	}
	return ctx.AnalysisIR.AnswerContract.Diagram
}

func answerExactResolutionContract(ctx *types.BusContext) *types.ExactResolutionContract {
	if ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.AnswerContract.ExactResolution == nil {
		return nil
	}
	return ctx.AnalysisIR.AnswerContract.ExactResolution
}

func validateSummaryRequiredDiagram(summary string, ctx *types.BusContext) error {
	dc := answerDiagramContract(ctx)
	if dc == nil || !dc.Required {
		return nil
	}
	minimum := dc.Minimum
	if minimum <= 0 {
		minimum = 1
	}
	if summaryDiagramFenceCount(summary) >= minimum {
		return nil
	}
	kinds := make([]string, 0, len(dc.PreferredKinds))
	for _, kind := range dc.PreferredKinds {
		if kind == types.DiagramNone {
			continue
		}
		kinds = append(kinds, string(kind))
	}
	if len(kinds) == 0 {
		kinds = append(kinds, string(types.DiagramFlow))
	}
	return fmt.Errorf(
		"diagram required for this dispatch (preferred kinds: %s); summary must include at least %d grounded triple-backtick diagram block(s). This obligation is independent of answer shape.",
		strings.Join(kinds, ", "), minimum,
	)
}

func summaryDiagramFenceCount(summary string) int {
	blocks := fencedCodeBlockRe.FindAllStringSubmatch(summary, -1)
	count := 0
	for _, block := range blocks {
		if len(block) < 2 {
			continue
		}
		if isDiagramLikeFence(block[1]) {
			count++
		}
	}
	return count
}

func isDiagramLikeFence(body string) bool {
	lines := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines < 2 {
		return false
	}
	for _, cue := range diagramCueTokens {
		if strings.Contains(body, cue) {
			return true
		}
	}
	return false
}

// validateSummaryDiagramGrounding scans every fenced code block in
// the summary for file-like tokens and rejects any that are not
// corroborated by citations[] or the attached-log ResolvedFiles
// allowlist. Complements the per-shape literal-grounding wrappers,
// which do not see summary prose.
//
// Design: the per-cite wrappers skip ShapeExplanation because prose
// shares identifier tokens with citations ambiently; but ASCII
// call-chain / sequence / architecture diagrams are structural
// claims, so a filename tossed into a diagram without citation is
// hallucination. The gate trades a pinhole false-positive risk
// (citation-free block that names an unmentioned-but-real file) for
// a clean system-level stop on the observed failure mode (see the
// logtri_go run where the LLM wrote `explorer.go (ParseOutput) └─▶
// buildAnalysisIR` despite the panic frames explicitly resolving at
// `analyzer.go:320`).
//
// Contract:
//   - no fenced blocks in summary → nil.
//   - empty allowlist (zero citations, zero ResolvedFiles) → nil. This
//     is the sub-agent / unit-test path; we refuse to mass-reject
//     without a ground-truth set to check against.
//   - fenced block contains only tokens whose extensions are outside
//     diagramFileExtensions → nil.
//   - any diagramFileTokenRe match whose basename and full-path form
//     are both absent from the allowlist → error naming every
//     offending token, with escape guidance.
func validateSummaryDiagramGrounding(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) error {
	blocks := fencedCodeBlockRe.FindAllStringSubmatch(summary, -1)
	if len(blocks) == 0 {
		return nil
	}
	allow := buildSummaryDiagramAllowlist(citations, gc, ctx)
	if len(allow) == 0 {
		return nil
	}

	var violations []string
	seen := make(map[string]bool)
	for _, block := range blocks {
		body := block[1]
		for _, tok := range diagramFileTokenRe.FindAllString(body, -1) {
			bare := tok
			if idx := strings.LastIndex(bare, ":"); idx >= 0 {
				bare = bare[:idx]
			}
			ext := strings.ToLower(path.Ext(bare))
			if !diagramFileExtensions[ext] {
				continue
			}
			if allow[bare] || allow[path.Base(bare)] {
				continue
			}
			if seen[bare] {
				continue
			}
			seen[bare] = true
			violations = append(violations, bare)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	return fmt.Errorf(
		"summary fenced code block references file(s) not present in citations[] or attached-log frames: %s. "+
			"ASCII diagrams are structural claims — every filename or path-like label named inside a triple-backtick block "+
			"must be grounded by a cited repo file, a cited line-text path literal, or an attached-log resolved frame. "+
			"Either add a citations[] entry for each named file (and reference it from steps / symbols / value as needed), "+
			"or remove the unsupported file name from the diagram and describe the relationship in prose.",
		strings.Join(violations, ", "),
	)
}

func buildSummaryDiagramAllowlist(citations []types.Citation, gc *ground.Context, ctx *types.BusContext) map[string]bool {
	allow := make(map[string]bool, len(citations)*4)
	for _, c := range citations {
		addDiagramAllowToken(allow, c.File)
	}
	if gc != nil && len(gc.LineIndex) > 0 {
		for _, c := range citations {
			addDiagramAllowFromCitationWindow(allow, c, gc)
		}
	}
	if ctx != nil && ctx.Mutable != nil {
		if bundle := ctx.Mutable.LogTriage(); bundle != nil {
			for _, f := range bundle.ResolvedFiles {
				addDiagramAllowToken(allow, f)
			}
		}
	}
	return allow
}

func addDiagramAllowFromCitationWindow(allow map[string]bool, c types.Citation, gc *ground.Context) {
	if len(allow) == 0 && gc == nil {
		return
	}
	file := strings.ReplaceAll(strings.TrimSpace(c.File), `\`, `/`)
	if file == "" || c.Line <= 0 || gc == nil {
		return
	}
	fileLines, ok := gc.LineIndex[file]
	if !ok || len(fileLines) == 0 {
		return
	}
	for line := c.Line - corroborationWindow; line <= c.Line+corroborationWindow; line++ {
		if line <= 0 {
			continue
		}
		text, ok := fileLines[line]
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		for _, tok := range diagramFileTokenRe.FindAllString(text, -1) {
			bare := tok
			if idx := strings.LastIndex(bare, ":"); idx >= 0 {
				bare = bare[:idx]
			}
			ext := strings.ToLower(path.Ext(bare))
			if !diagramFileExtensions[ext] {
				continue
			}
			addDiagramAllowToken(allow, bare)
		}
	}
}

func addDiagramAllowToken(allow map[string]bool, token string) {
	token = strings.ReplaceAll(strings.TrimSpace(token), `\`, `/`)
	if token == "" {
		return
	}
	allow[token] = true
	if base := path.Base(token); base != "" && base != "." {
		allow[base] = true
	}
}

// logTriageTypeIdentifierRe picks identifier-shape sub-tokens from
// an error Type string. `java.io.IOException` yields
// {"java", "io", "IOException"}; `runtime error: invalid memory
// address or nil pointer dereference` yields every word. The
// downstream coverage check filters for length ≥ 5 so single short
// words do not dominate matching.
var logTriageTypeIdentifierRe = regexp.MustCompile(`[A-Za-z][A-Za-z0-9_]*`)

// logTriageCoverageMinTokenLen is the minimum identifier-length an
// error-type sub-token must have to qualify as a coverage anchor.
// Five filters out common short words (err, the, and, ...) and
// keeps exception / class names (IOException, NullPointerException,
// ValueError, runtime, pointer, dereference). Tuned by eyeballing
// the eval corpus's real Types across Java / Go / Python / Rust.
const logTriageCoverageMinTokenLen = 5

// validateSummaryLogTriageCoverage enforces that when the attached
// log's structured bundle identifies specific error Types, the
// answer summary acknowledges each one. A summary that names NONE
// of a given Type's identifier-shape tokens of length ≥ 5 — after
// walking the full Cause chain depth-first — is rejected as
// inconsistent with the log's structured extraction.
//
// This is the system-level contract complement to the LLM-facing
// skill directive: skill tells the LLM WHAT to do, this gate
// verifies it was done. Mirrors the shape-agnostic design of
// validateSummaryDiagramGrounding (F4.1) — runs once, checks the
// summary field, redirects to the structured Log Triage section
// on failure.
//
// Scope rules:
//
//   - no bundle OR no Errors → nil (nothing to cover)
//   - a Type with zero identifier-shape tokens of length ≥ 5 →
//     fall back to substring match on the full trimmed Type
//   - ANY token (or the fallback substring) appearing in summary
//     satisfies coverage for that Type
//   - EVERY Type in the Cause-chain traversal must be covered;
//     the first Type that fails the check triggers the reject
//     with a redirect that names every missed Type
//
// The "one token per Type" threshold is the minimal correct check:
// it catches the pure-hallucination case (summary shares ZERO
// tokens with the log's Types) without false-rejecting legitimate
// answers that paraphrase or abbreviate. Case-insensitive match
// because exception names may be referenced in lowercase in
// non-technical answer prose (e.g. "a nullpointerexception is
// raised when ...") — the semantic contract is presence, not
// exact capitalisation.
func validateSummaryLogTriageCoverage(summary string, ctx *types.BusContext) error {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	bundle := ctx.Mutable.LogTriage()
	if bundle == nil || len(bundle.Errors) == 0 {
		return nil
	}
	errorTypes := collectLogTriageTypes(bundle)
	if len(errorTypes) == 0 {
		return nil
	}
	lowerSummary := strings.ToLower(summary)
	var missing []string
	for _, t := range errorTypes {
		if !logTriageTypeCovered(t, lowerSummary) {
			missing = append(missing, t)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf(
		"summary does not acknowledge the attached log's error type(s): %s. "+
			"The log's structured Errors tree identifies these types (walk the top-level Errors and their Cause chain in the Log Triage section); "+
			"the answer must name each one by its class / exception identifier at least once so the reader can connect the log to your explanation. "+
			"Quote the Type literally from the structured extraction — do NOT paraphrase class names away or invent alternative stack traces. "+
			"If the question resolves to just one layer of the chain (e.g. 'what does this panic mean?' when the chain is trivial), still include the single Type's identifier. "+
			"Case-insensitive match; a single mention of the class / exception name anywhere in summary is sufficient coverage.",
		strings.Join(missing, ", "))
}

// collectLogTriageTypes walks every top-level Error in the bundle
// and its Cause recursion, returning the ordered list of Type
// strings. Duplicates are preserved in declaration order so the
// reject message reads naturally ("missing: RuntimeException,
// NullPointerException, IOException"). Empty Types are skipped.
func collectLogTriageTypes(bundle *types.LogBundle) []string {
	if bundle == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	var walk func(*types.LogError)
	walk = func(e *types.LogError) {
		if e == nil {
			return
		}
		t := strings.TrimSpace(e.Type)
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
		walk(e.Cause)
	}
	for i := range bundle.Errors {
		walk(&bundle.Errors[i])
	}
	return out
}

// logTriageTypeCovered reports whether lowerSummary mentions at
// least one identifier-shape token of length ≥ 5 extracted from
// the error Type, or (for Types with no such token) the trimmed
// Type itself as a substring. Cross-checked case-insensitively —
// lowerSummary is pre-lowered by the caller.
func logTriageTypeCovered(errType, lowerSummary string) bool {
	matches := logTriageTypeIdentifierRe.FindAllString(errType, -1)
	hadQualifying := false
	for _, m := range matches {
		if len(m) < logTriageCoverageMinTokenLen {
			continue
		}
		hadQualifying = true
		if strings.Contains(lowerSummary, strings.ToLower(m)) {
			return true
		}
	}
	if hadQualifying {
		return false
	}
	// Fallback for Types with only short tokens — require the full
	// trimmed Type substring (case-insensitive). Defensive path:
	// most real Types have at least one long token, but this keeps
	// the gate from silently skipping exotic single-word Types.
	trimmed := strings.TrimSpace(errType)
	if trimmed == "" {
		return true
	}
	return strings.Contains(lowerSummary, strings.ToLower(trimmed))
}

// codenameTokenRe matches "opaque enumeration label" tokens — the
// narrow class of identifiers a reader cannot verify by form alone,
// only by source lookup. Three sub-forms (composite alternation):
//
//  1. Prefix-wrapped codename: `Fallback S2` / `Check 6` / `Gate 3`
//  2. Enum-word + digit: `Phase 0` / `Tier 2` / `Step 3` / `Layer 1`
//  3. Letter+digit codename: `S1` / `T11` / `P0` / `Q2`
//
// Not matched: CamelCase identifiers (`ShouldStop`), snake_case
// (`erm_all_satisfied`), multi-letter acronyms (`HTTP2`, `SHA256`)
// because `\b[A-Z]\d+\b` requires word boundary after the digits.
//
// Theory: these are Kripke-rigid designators — meaning derives purely
// from source referent, not compositional morphology. So verification
// MUST go through source lookup; pattern-completion from the LLM's
// prior (`S1 exists → S2 must too`) is not admissible.
//
// See project_session24_codename_confab_trace for the empirical
// failure mode this gate defends against.
var codenameTokenRe = regexp.MustCompile(
	`\b(?:` +
		`(?:Fallback|Check|Gate|Case)\s+[A-Z]?\d+` +
		`|` +
		`(?:Phase|Stage|Step|Tier|Level|Round|Pass|Layer|Rule)\s+\d+` +
		`|` +
		`[A-Z]\d+` +
		`)\b`)

// codenameGroundedInWindow reports whether the exact token substring
// appears inside `file`'s LineIndex in the inclusive range
// [lineStart-corroborationWindow .. max(lineStart,lineEnd)+corroborationWindow].
// lineEnd may be 0 (single-line cite) — treated as lineStart.
// Returns false on unknown file / empty index (degrade safely).
func codenameGroundedInWindow(token, file string, lineStart, lineEnd int, gc *ground.Context) bool {
	if gc == nil {
		return false
	}
	fileLines, ok := gc.LineIndex[file]
	if !ok || len(fileLines) == 0 {
		return false
	}
	lo, hi := lineStart, lineEnd
	if hi <= 0 {
		hi = lo
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	for line := lo - corroborationWindow; line <= hi+corroborationWindow; line++ {
		if line <= 0 {
			continue
		}
		if text, ok := fileLines[line]; ok && strings.Contains(text, token) {
			return true
		}
	}
	return false
}

// validateSummaryCodenameGrounding scans the summary text for
// codename-shape tokens (see codenameTokenRe) and rejects any token
// absent from every citation's ±corroborationWindow-line range in the
// ground-truth LineIndex.
//
// Why: LLM pattern-completion produces sequence extensions (S1 → S2,
// Phase 0 → Phase 1 / 2 / ...) from prior alone, even when the input
// prompt contains zero evidence of the extended label. Information
// hiding at context-assembly time cannot prevent this; emission-time
// rejection forces a retry without the fabricated label. Complementary
// to the literal-grounding wrappers (which only inspect structural
// fields, not summary prose) and the diagram-grounding gate (which
// only inspects fenced blocks).
//
// Contract:
//
//   - gc == nil OR empty LineIndex → skip (sub-agent / test path)
//   - zero codename tokens in summary → skip (no claim to verify)
//   - every token present in at least one citation's ±3-line window → OK
//   - otherwise → reject with violations list and escape guidance
//
// Scope: applies across every shape that carries a summary — the
// hallucination is prose-level (not structural-field-level) so the
// gate runs on summary universally, paralleling the diagram and
// log-triage coverage gates.
func validateSummaryCodenameGrounding(summary string, citations []types.Citation, gc *ground.Context) error {
	if gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}
	matches := codenameTokenRe.FindAllString(summary, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	ordered := make([]string, 0, len(matches))
	for _, t := range matches {
		if !seen[t] {
			seen[t] = true
			ordered = append(ordered, t)
		}
	}
	var ungrounded []string
	for _, tok := range ordered {
		grounded := false
		for _, c := range citations {
			if c.File == "" || c.Line <= 0 {
				continue
			}
			if codenameGroundedInWindow(tok, c.File, c.Line, 0, gc) {
				grounded = true
				break
			}
		}
		if !grounded {
			ungrounded = append(ungrounded, tok)
		}
	}
	if len(ungrounded) == 0 {
		return nil
	}
	return fmt.Errorf(
		"summary introduces codename label(s) not present in any citation's ±%d-line window: %s. "+
			"Short enumeration labels (S1/S2/Phase 0/Fallback X) are source-level identifiers — the exact token MUST "+
			"appear in the cited line range. Either cite the line where each label is defined, or remove the label "+
			"and describe the mechanism by its real behavior; do NOT extrapolate from an existing label by pattern "+
			"(e.g. 'S1 exists so S2 must too' is not admissible).",
		corroborationWindow, strings.Join(ungrounded, ", "))
}

// validateEvidenceSummaryCodenameGrounding is the emit_evidence-side
// sibling: it inspects a single evidence item's Summary text and
// rejects any codename token absent from the item's own
// Source:[LineStart..LineEnd] ±corroborationWindow range in the
// LineIndex. This closes the upstream leak point — explorer itself
// confabulates numbered labels inside emit_evidence items (observed
// in u3a-20260422-014410/run-2 where the explorer emitted an invented
// `S2 终止条件` evidence item that extract/finalize had to filter out).
//
// Contract mirrors validateSummaryCodenameGrounding; returns nil on
// degraded context (no LineIndex, no Source, no codename tokens).
var exactTargetSubstituteCues = []string{
	"closest equivalent",
	"nearest equivalent",
	"closest matching",
	"equivalent field",
	"equivalent key",
	"equivalent setting",
	"equivalent knob",
	"substitute for",
	"same as",
	"对应字段",
	"等价字段",
	"等价配置",
	"最接近的等价",
	"最接近的字段",
	"最近邻字段",
	"替代字段",
	"替代配置",
	"以该最近邻字段为准",
}

func validateSummaryExactResolution(summary string, ctx *types.BusContext) error {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	contract := answerExactResolutionContract(ctx)
	if contract == nil || len(contract.Targets) == 0 {
		return nil
	}
	label := strings.TrimSpace(contract.TargetLabel)
	if label == "" {
		label = "target"
	}
	if contract.RequireTargetMention {
		var missing []string
		for _, target := range contract.Targets {
			if !types.ExactResolutionTextMentionsTarget(contract, summary, target) {
				missing = append(missing, target)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf(
				"exact-resolution contract violated: summary must explicitly name the requested exact %s %s. Resolve the exact target before discussing nearby context.",
				label, exactTargetListForError(missing))
		}
	}
	justification := strings.TrimSpace(ctx.Mutable.AbsenceJustification())
	pending := types.ExactResolutionPendingTargets(contract, unverifiedFindingsForCompletion(ctx))
	if !contract.AllowAbsence || justification == "" || len(pending) == 0 {
		return nil
	}
	if !looksLikeHonestZeroClaim(summary, "") {
		return fmt.Errorf(
			"exact-resolution contract violated: summary names the requested exact %s but does not clearly state it is absent / not found. Lead with the exact absence before any nearby context. Absence-only is acceptable.",
			label)
	}
	if contract.AliasRequiresProof && containsPositiveAliasClaim(summary) {
		return fmt.Errorf(
			"exact-resolution contract violated: the investigation concluded the exact %s is absent, so nearby knobs / symbols may appear only as related context, not as equivalents, aliases, or substitutes for the requested target without explicit proof.",
			label)
	}
	return nil
}

func containsPositiveAliasClaim(summary string) bool {
	lower := strings.ToLower(summary)
	englishPositive := []string{
		"is an alias",
		"alias of",
		"equivalent to",
		"equivalent field",
		"equivalent key",
		"equivalent setting",
		"equivalent knob",
		"closest equivalent",
		"closest match",
		"nearest equivalent",
		"nearest match",
		"substitute for",
		"same as",
	}
	for _, cue := range englishPositive {
		idx := strings.Index(lower, cue)
		for idx >= 0 {
			windowStart := idx - 24
			if windowStart < 0 {
				windowStart = 0
			}
			window := lower[windowStart:idx]
			if !strings.Contains(window, "not ") && !strings.Contains(window, "n't ") && !strings.Contains(window, "without ") {
				return true
			}
			next := strings.Index(lower[idx+len(cue):], cue)
			if next < 0 {
				break
			}
			idx += len(cue) + next
		}
	}
	chinesePositive := []string{"别名", "等价", "替代", "对应字段", "对应配置"}
	for _, cue := range chinesePositive {
		idx := strings.Index(summary, cue)
		for idx >= 0 {
			prefixRunes := []rune(summary[:idx])
			if len(prefixRunes) > 20 {
				prefixRunes = prefixRunes[len(prefixRunes)-20:]
			}
			window := string(prefixRunes)
			if !strings.Contains(window, "不") && !strings.Contains(window, "非") {
				return true
			}
			next := strings.Index(summary[idx+len(cue):], cue)
			if next < 0 {
				break
			}
			idx += len(cue) + next
		}
	}
	return false
}

func exactTargetListForError(targets []string) string {
	if len(targets) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(targets))
	for _, target := range targets {
		quoted = append(quoted, "`"+target+"`")
	}
	return strings.Join(quoted, ", ")
}

func validateEvidenceSummaryCodenameGrounding(item *types.EvidenceItem, gc *ground.Context) error {
	if gc == nil || len(gc.LineIndex) == 0 {
		return nil
	}
	if item == nil || item.Source == "" || item.LineStart <= 0 || item.Summary == "" {
		return nil
	}
	matches := codenameTokenRe.FindAllString(item.Summary, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	var ungrounded []string
	for _, t := range matches {
		if seen[t] {
			continue
		}
		seen[t] = true
		if !codenameGroundedInWindow(t, item.Source, item.LineStart, item.LineEnd, gc) {
			ungrounded = append(ungrounded, t)
		}
	}
	if len(ungrounded) == 0 {
		return nil
	}
	return fmt.Errorf(
		"summary introduces codename(s) %s absent from %s:%d-%d±%d. "+
			"Either remove the fabricated label or re-anchor on a line range where the token actually appears; "+
			"do NOT extrapolate by pattern from other labels you've seen.",
		strings.Join(ungrounded, ", "), item.Source, item.LineStart, max0(item.LineEnd, item.LineStart), corroborationWindow)
}

// max0 returns the first non-zero-positive of a, b (with b as fallback).
// Internal helper to render evidence line ranges honestly: LineEnd can
// be zero when the item is single-line; the error message should then
// show `lineStart-lineStart` not `lineStart-0`.
func max0(a, b int) int {
	if a > 0 {
		return a
	}
	return b
}

func buildEmitAnswerDocumentBoolean(in *emitAnswerDocumentBoolean, numCites int) (*types.AnswerBoolean, error) {
	rationale := strings.TrimSpace(in.Rationale)
	if rationale == "" {
		return nil, fmt.Errorf("boolean.rationale is required")
	}
	decision, ok := emitAnswerDocumentBooleanDecision(in.Decision)
	if !ok {
		return nil, fmt.Errorf("boolean.decision %q is not one of: true, false, yes, no, 是, 否", in.Decision)
	}
	citationRef := in.CitationRef.Int()
	if err := validateCitationRef("boolean", 0, citationRef, numCites); err != nil {
		return nil, err
	}
	return &types.AnswerBoolean{
		Decision:    decision,
		Rationale:   rationale,
		CitationRef: citationRef,
	}, nil
}

// buildEmitAnswerDocumentCitations validates every incoming citation
// and runs the grounder against the current BusContext. Three-way
// outcome per citation:
//
//	Valid + QuoteMatched  → kept as-is.
//	Valid + !QuoteMatched → kept with Quote cleared (prevents a
//	                        fabricated excerpt from reaching the user
//	                        while preserving the real file:line anchor).
//	!Valid                → dropped from the pool; any downstream
//	                        CitationRef that pointed at it is remapped
//	                        to CitationRefUnset (-1) by the caller.
//
// Returns (kept, remap, warnings, err):
//   - kept:  the surviving citations in the new pool (may be shorter
//     than `in` if some were dropped).
//   - remap: old-index → new-index, or -1 for dropped entries. Always
//     len(remap) == len(in) so callers can mapRef() safely.
//   - warnings: per-citation messages surfaced in the tool Summary so
//     the LLM can correct on the next turn.
//   - err:   only set on a STRUCTURAL error (empty file, bad path,
//     line <= 0, quote too long, blob-leak) that should
//     reject the whole call; grounding failures never return
//     err — they are surfaced via drop/clear + warnings.
//
// simulateCitationGrounding is the CGEC G1 pre-finalize dry-run. It
// runs the ReadSet whitelist check against every incoming citation
// without touching the gutter / symbol-table / quote layers. When
// ZERO citations can pass the whitelist AND the LLM supplied at
// least one citation, the real grounder will drop them all and the
// contract check will fail on 0-citations — wasting the dispatch.
// The early error surfaces the whitelist to the LLM, triggers the
// finalizer's correction retry path, and emits the same
// RepairReadFile directives the grounder would have emitted (so
// even if the finalizer's internal retry does not re-call
// emit_answer_document, the retry-hint renderer surfaces the forced-
// read list to the next explore round).
//
// Returns an empty string when the call should proceed to the real
// grounder (i.e. at least one citation is whitelist-eligible OR
// there are no citations at all OR the readFiles whitelist is
// empty / unset).
func simulateCitationGrounding(citations []types.Citation, readFiles map[string]bool, gc *ground.Context, ctx *types.BusContext) string {
	if len(citations) == 0 || len(readFiles) == 0 {
		return ""
	}
	// Canonicalise each citation file to match the readFiles whitelist's
	// canonical form. buildEmitAnswerDocumentCitations does this at
	// line 893 via ground.CanonicalRepoRelative; we replicate the call
	// here so the dry-run accepts the same inputs the real grounder
	// would have accepted (e.g. absolute-path citation against
	// relative-path readFiles bridged through repoRoot).
	var missing []string
	var matched int
	repoRoot := ""
	if gc != nil {
		repoRoot = gc.RepoRoot
	}
	canonical := make([]string, len(citations))
	for i, c := range citations {
		canonical[i] = c.File
		if c.File != "" && repoRoot != "" {
			canonical[i] = ground.CanonicalRepoRelative(c.File, repoRoot)
		}
	}
	for i, c := range citations {
		if c.File == "" {
			continue
		}
		if readFiles[canonical[i]] {
			matched++
			continue
		}
		missing = append(missing, fmt.Sprintf("%s:%d", c.File, c.Line))
	}
	if matched > 0 {
		return ""
	}
	if len(missing) == 0 {
		return ""
	}
	// Emit RepairReadFile directives so the retry-hint renderer
	// surfaces the forced-read list even if the finalizer's
	// correction retry does not re-fire emit_answer_document.
	// AddRepair is dedup-safe so repeated drops of the same file
	// collapse into one directive, and A1 bridge mirrors each
	// RepairReadFile into the PendingReads queue for Lazy Auto-Read.
	if ctx != nil && ctx.Mutable != nil {
		closure := ctx.Mutable.EvidenceClosure()
		seen := make(map[string]bool)
		for i, c := range citations {
			if c.File == "" || readFiles[canonical[i]] || seen[canonical[i]] {
				continue
			}
			seen[canonical[i]] = true
			closure.AddRepair(types.RepairDirective{
				Kind:      types.RepairReadFile,
				Files:     []string{canonical[i]},
				Rationale: fmt.Sprintf("pre-finalize dry-run: citation %s:%d points at a file the investigation did not read", c.File, c.Line),
				Origin:    "emit_answer_document.dry_run",
			})
		}
	}
	allowed := make([]string, 0, len(readFiles))
	for f := range readFiles {
		allowed = append(allowed, f)
	}
	sort.Strings(allowed)
	allowedCap := 10
	if len(allowed) > allowedCap {
		allowed = append(allowed[:allowedCap], fmt.Sprintf("...and %d more", len(readFiles)-allowedCap))
	}
	logging.Warning("[CGEC] G1 pre_finalize_dry_run: rejecting emit_answer_document — 0/%d citations match whitelist", len(citations))
	// Session 11 F1: record structured ledger entry so F2 aggregator
	// can spot repeated ReadSet-miss patterns (→ R5 expand_search
	// or SourceMix IRPatch). Confidence 0.90 because 0/N cite-hit
	// is an unambiguous signal that finalizer + Turn A retrieval are
	// out of sync.
	if ctx != nil && ctx.Mutable != nil {
		refs := make([]string, 0, len(missing))
		for _, m := range missing {
			refs = append(refs, m)
		}
		ctx.Mutable.EvidenceClosure().AppendViolation(types.Violation{
			Kind:         types.ViolCitation,
			Detail:       fmt.Sprintf("0/%d citations inside Turn A ReadSet at pre-finalize dry-run", len(citations)),
			Stage:        string(types.StageFinalize),
			EvidenceRefs: refs,
			SuspectedRoot: types.SuspectedRoot{
				IRField:    "ScannedSet",
				Reason:     "finalizer citations anchored outside Turn A ReadSet — ranker/explorer missed target",
				Confidence: 0.90,
			},
		})
	}
	return fmt.Sprintf(
		"emit_answer_document REJECTED: none of your %d citation(s) point to a file the investigation actually read, so the grounder will drop every one and the answer contract will fail.\n\n"+
			"Your citations: %s\n\n"+
			"Allowed files (the investigation's read-files list): %s\n\n"+
			"Re-emit emit_answer_document with file:line in the allowed list. If none of the allowed files contain the real answer anchor, call read_file on the missing file BEFORE re-emitting, or accept an absence answer via emit_investigation_complete(absence_justification=...).",
		len(citations),
		strings.Join(missing, ", "),
		strings.Join(allowed, ", "))
}

// buildEmitAnswerDocumentCitations now also returns a slice of
// CGEC RepairDirective values the caller pushes onto the per-Run
// EvidenceClosure. One RepairReadFile entry is generated per
// citation dropped by the Turn-A ReadSet whitelist; the orchestrator
// then surfaces those to the next explore-window prompt as a Forced
// Read List so the LLM stops re-citing the same unread files.
func buildEmitAnswerDocumentCitations(in []types.Citation, workDir string, gc *ground.Context, readFiles map[string]bool) ([]types.Citation, []int, []string, []types.RepairDirective, error) {
	if len(in) == 0 {
		return nil, nil, nil, nil, nil
	}
	// Two-pass pipeline. Pass 1 validates structurally and grounds
	// each citation, recording the tier that accepted it and whether
	// the quote was cleared. Pass 2 enforces the "quote-cleared
	// citations need a Tier 1 proven peer in the surviving pool"
	// rule: a pool where every citation has empty/cleared quote AND
	// none were Tier 1 means the LLM never actually read the files
	// it claims to cite — drop the quote-cleared ones.
	//
	// readFiles (session 8) is the set of file paths Turn A actually
	// read (Mutable.TurnAArtifacts().ReadFiles). When non-nil,
	// citations pointing at files NOT in the set are dropped
	// immediately with a warning that lists the allowed paths — this
	// catches the LLM picking similar-but-wrong paths
	// (internal/agent/subagent.go vs internal/types/subagent.go) at
	// the earliest point, before grounder / Tier-1 peer logic runs.
	// When nil or empty (pre-TurnA tests, sub-agent contexts), the
	// check is skipped.
	type perCitation struct {
		keep         bool
		cit          types.Citation
		tier         types.GroundingTier
		quoteCleared bool
	}
	scratch := make([]perCitation, len(in))
	var anyRealTier bool // did any citation get a non-empty Tier? false in no-context test mode.
	var warnings []string
	var repairs []types.RepairDirective
	// Precompute allowed-path hint once — rendered into every
	// whitelist rejection warning. Keeps the LLM from having to
	// reconstruct the set from scattered prompt sections.
	allowedHint := ""
	if len(readFiles) > 0 {
		allowed := make([]string, 0, len(readFiles))
		for f := range readFiles {
			allowed = append(allowed, f)
		}
		sort.Strings(allowed)
		// Soft cap so the warning stays scannable.
		if len(allowed) > 12 {
			allowedHint = fmt.Sprintf("allowed files (first 12 of %d): %s", len(allowed), strings.Join(allowed[:12], ", "))
		} else {
			allowedHint = "allowed files: " + strings.Join(allowed, ", ")
		}
	}
	for i := range in {
		c := in[i]
		c.File = strings.TrimSpace(c.File)
		if c.File == "" {
			return nil, nil, nil, nil, fmt.Errorf("citations[%d]: file is required", i)
		}
		if !emitLooksLikePath(c.File) {
			return nil, nil, nil, nil, fmt.Errorf("citations[%d]: file %q does not look like a repo-relative file path", i, c.File)
		}
		if isInsideWorkDir(c.File, workDir) {
			return nil, nil, nil, nil, fmt.Errorf("citations[%d]: file %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file", i, c.File, workDir)
		}
		// Canonicalise to the same repo-relative form the readFiles
		// whitelist and the grounder's LineIndex / FileIndex use. The
		// persisted citation carries the canonical form too — downstream
		// renderers see a consistent path regardless of LLM choice.
		if gc != nil {
			c.File = ground.CanonicalRepoRelative(c.File, gc.RepoRoot)
		}
		// Whitelist check (session 8): citation file must be one
		// Turn A actually read. Skipped when readFiles set is empty
		// (test path / no TurnA). Dropped item goes through the
		// standard per-citation drop path; the warning names both
		// the offending file AND the allowed set so the next turn
		// can correct in one shot.
		if len(readFiles) > 0 && !readFiles[c.File] {
			warnings = append(warnings,
				fmt.Sprintf("citations[%d] dropped (%s:%d) — file not in the investigation's read-files list; %s. Cite ONLY files the investigation read, or reject the answer",
					i, c.File, c.Line, allowedHint))
			// CGEC D1: structured RepairDirective so the orchestrator
			// can render a Forced Read List in the next explore round.
			// One repair per dropped file (de-dup'd downstream by
			// EvidenceClosure.AddRepair).
			repairs = append(repairs, types.RepairDirective{
				Kind:      types.RepairReadFile,
				Files:     []string{c.File},
				Rationale: fmt.Sprintf("emit_answer_document cited %s:%d but file is not in the investigation's read-files list — read it before next emit_investigation_complete", c.File, c.Line),
				Origin:    "emit_answer_document.grounder",
			})
			continue
		}
		if c.Line <= 0 {
			return nil, nil, nil, nil, fmt.Errorf("citations[%d]: line must be > 0 (got %d) — Pattern 2 line-hallucination guard", i, c.Line)
		}
		c.Quote = strings.TrimSpace(c.Quote)
		quoteCap := types.CitationMaxQuoteChars()
		if len(c.Quote) > quoteCap {
			// Downgrade from hard-reject to silent truncate: the grounder's
			// QuoteMatched token check (below) is the real anti-prose guard.
			// A single oversize Quote used to abort the whole emit, taking
			// every peer citation with it on retry — but legit long source
			// lines (deep imports, long fmt.Errorf, long regex/SQL literals)
			// routinely exceed the preview ceiling. Preserve file:line,
			// truncate the preview on a UTF-8 boundary, and let grounder
			// decide whether the remainder corroborates the source line.
			// Prose Quotes fail the token check and are cleared below; long
			// legit code keeps its anchor with a truncated preview.
			end := quoteCap
			for end > 0 && !utf8.RuneStart(c.Quote[end]) {
				end--
			}
			warnings = append(warnings,
				fmt.Sprintf("citations[%d] quote truncated (%s:%d) — %d chars exceeded %d-char preview ceiling",
					i, c.File, c.Line, len(c.Quote), quoteCap))
			c.Quote = c.Quote[:end]
		}

		// Grounding — pure function, never returns err.
		rep := ground.GroundCitation(c, gc)
		if rep.Tier != "" {
			anyRealTier = true
		}
		if !rep.Valid {
			warnings = append(warnings,
				fmt.Sprintf("citations[%d] dropped (%s:%d): %s", i, c.File, c.Line, rep.Reason))
			continue
		}
		cleared := false
		if c.Quote != "" && !rep.QuoteMatched {
			warnings = append(warnings,
				fmt.Sprintf("citations[%d] quote cleared (%s:%d) — Quote tokens do not corroborate the cited line", i, c.File, c.Line))
			c.Quote = ""
			cleared = true
		}
		scratch[i] = perCitation{keep: true, cit: c, tier: rep.Tier, quoteCleared: cleared}
	}

	// Pool-level defence: require at least one Tier 1 (line_text)
	// peer when any citation had its quote cleared. This catches the
	// fabricated-quote pattern — the LLM guesses a line number in an
	// indexed file and fabricates a prose quote. Tier 2 would let the
	// line through without the guard; the grounder already clears the
	// quote but kept the line. The rule: if the LLM guessed at every
	// citation (no Tier 1 anywhere) AND at least one had a fabricated
	// quote, drop all quote-cleared citations as unsafe. Tier 2-only
	// citations with an EMPTY original quote are honest (LLM knows it
	// can't quote what it didn't read) and keep their file:line.
	var tier1Proven bool
	if anyRealTier {
		for _, p := range scratch {
			if p.keep && p.tier == types.TierLineText {
				tier1Proven = true
				break
			}
		}
	} else {
		// No grounding context at all (unit test / pre-index path).
		// Skip the rule — legacy behaviour preserves all cites.
		tier1Proven = true
	}

	kept := make([]types.Citation, 0, len(in))
	remap := make([]int, len(in))
	for i, p := range scratch {
		if !p.keep {
			remap[i] = types.CitationRefUnset
			continue
		}
		if p.quoteCleared && !tier1Proven {
			warnings = append(warnings,
				fmt.Sprintf("citations[%d] dropped (%s:%d) — quote was fabricated and no peer citation is Tier 1 proven (the LLM never read any cited file)", i, p.cit.File, p.cit.Line))
			remap[i] = types.CitationRefUnset
			continue
		}
		remap[i] = len(kept)
		kept = append(kept, p.cit)
	}
	return kept, remap, warnings, repairs, nil
}

// applyCitationRemap rewrites every CitationRef in p.Steps, p.Value,
// p.Boolean according to the old-index → new-index map returned by
// buildEmitAnswerDocumentCitations. A remap entry of -1 means the
// citation was dropped; the corresponding ref is set to
// CitationRefUnset so the renderer treats it as "no citation" rather
// than pointing at a different (shifted) pool entry.
//
// emit_answer_symbol's symbols[] carry their own File/Line and do NOT
// use CitationRef into this pool, so they are left alone.
func applyCitationRemap(p *emitAnswerDocumentParams, remap []int) {
	if len(remap) == 0 {
		return
	}
	mapRef := func(ref int) int {
		if ref == types.CitationRefUnset {
			return ref
		}
		// Structural range errors (negative refs other than -1, or
		// refs pointing past the ORIGINAL citations slice) are left
		// intact so validateCitationRef downstream can reject them
		// with the canonical "out of range" message. We only rewrite
		// refs whose target exists but was dropped by the grounder.
		if ref < 0 || ref >= len(remap) {
			return ref
		}
		if remap[ref] < 0 {
			return types.CitationRefUnset
		}
		return remap[ref]
	}
	for i := range p.Steps {
		p.Steps[i].CitationRef = mapRef(p.Steps[i].CitationRef)
	}
	if p.Value != nil {
		p.Value.CitationRef = mapRef(p.Value.CitationRef)
	}
	if p.Boolean != nil {
		p.Boolean.CitationRef = FlexInt(mapRef(p.Boolean.CitationRef.Int()))
	}
}

// validateCitationRef checks that ref is either CitationRefUnset (-1)
// or a valid index into a citations pool of length numCites. Zero is
// a VALID index (the first pool entry) — callers that want "no cite"
// must explicitly pass -1. This distinction is the foot-gun the
// sentinel design is meant to catch.
func validateCitationRef(field string, index int, ref int, numCites int) error {
	if ref == types.CitationRefUnset {
		return nil
	}
	if ref < 0 {
		return fmt.Errorf("%s[%d]: citation_ref %d is negative; use -1 for 'no citation' or a valid pool index", field, index, ref)
	}
	if ref >= numCites {
		return fmt.Errorf("%s[%d]: citation_ref %d is out of range (citations pool has %d entries)", field, index, ref, numCites)
	}
	return nil
}

func renderEmitAnswerDocumentSummary(doc *types.AnswerDocument, citationWarnings []string, shapeCorrectionNote string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "emit_answer_document accepted: shape=%s citations=%d", doc.Shape, len(doc.Citations))
	switch doc.Shape {
	case types.ShapeListOfSymbols:
		fmt.Fprintf(&b, " symbols=%d completeness=%s", len(doc.Symbols), string(doc.SymbolsCompleteness))
	case types.ShapeStepList:
		fmt.Fprintf(&b, " steps=%d", len(doc.Steps))
	case types.ShapeValue, types.ShapeConfigValue:
		if doc.Value != nil {
			fmt.Fprintf(&b, " literal=%q", doc.Value.Literal)
		}
	case types.ShapeBoolean:
		if doc.Boolean != nil {
			fmt.Fprintf(&b, " decision=%v", doc.Boolean.Decision)
		}
	case types.ShapeExplanation:
		fmt.Fprintf(&b, " summary_len=%d", len(doc.Summary))
	}
	if len(doc.Caveats) > 0 {
		fmt.Fprintf(&b, " caveats=%d", len(doc.Caveats))
	}
	// Citation grounding feedback. Surfaced in the tool Summary so
	// the LLM (and eval harness) sees which citations were filtered
	// out and why. When the LLM's next turn is the finalizer retry,
	// it can pick different file:line anchors rather than guessing.
	if shapeCorrectionNote != "" {
		// Shape-correction notice is surfaced before citation
		// warnings so the LLM (and the REPL operator) sees it as the
		// first adjustment the tool made. Keeps the root cause of
		// any surprising final shape one glance away from the
		// Summary rather than buried in the log file.
		b.WriteString("\n\nShape correction: ")
		b.WriteString(shapeCorrectionNote)
	}
	if len(citationWarnings) > 0 {
		b.WriteString("\n\nCitation grounding:")
		for _, w := range citationWarnings {
			b.WriteString("\n  - ")
			b.WriteString(w)
		}
	}
	return b.String()
}

// slateToEmittedSymbols converts the extractor's stored AnswerSymbol
// slate into the finalizer-tool wire shape. Used by the shape
// auto-correct rescue path when the LLM's finalizer dispatch left
// p.Symbols empty on a list_of_symbols question but the extractor
// had already produced a populated slate.
func slateToEmittedSymbols(slate []types.AnswerSymbol) []emitAnswerSymbolItem {
	if len(slate) == 0 {
		return nil
	}
	out := make([]emitAnswerSymbolItem, 0, len(slate))
	for _, s := range slate {
		out = append(out, emitAnswerSymbolItem{
			Name:      s.Name,
			File:      s.File,
			Line:      FlexInt(s.Line),
			Kind:      string(s.Kind),
			Chain:     s.Chain,
			Rationale: s.Rationale,
		})
	}
	return out
}

// validateLiteralFormForSubject is a language- and domain-neutral
// structural sanity check on the emitted value.literal. It does
// NOT enforce kind-specific patterns (e.g. "must end with -skill"
// for skill names) — that would overfit to a specific project's
// naming conventions. Instead, it only catches structural
// impossibilities that apply to any identifier-like answer:
//
//   - empty literal (already covered by shape checks, but belt+braces)
//   - whitespace-only literal (same)
//   - literal containing embedded newlines (must be one line)
//   - literal containing unbalanced quotes (source quoting leaked)
//
// Specific per-kind validation is the subject taxonomy's job
// (internal/analysis/subject), which the chain ranker and the
// pre-complete simulator already consult via subject.Score. This
// keeps emit_answer_document clean of domain knowledge and avoids
// double-gating the same pattern with two different regex tables.
func validateLiteralFormForSubject(p *emitAnswerDocumentParams, kind types.AnswerSubjectKind) error {
	_ = kind // kind-specific checks live in the subject package
	if p == nil || p.Value == nil {
		return nil
	}
	lit := strings.TrimSpace(p.Value.Literal)
	if lit == "" {
		return nil
	}
	if strings.ContainsAny(lit, "\n\r") {
		return fmt.Errorf("literal %q contains embedded newline — value answers must be a single line", lit)
	}
	// Unbalanced quote count is a universal signal of a stray
	// paste: either the LLM half-quoted the token or copied a
	// trailing quote from the source line. Reject so the LLM
	// re-emits without the trailing artefact.
	if cnt := strings.Count(lit, `"`); cnt%2 != 0 {
		return fmt.Errorf("literal %q has unbalanced double quotes", lit)
	}
	if cnt := strings.Count(lit, `'`); cnt%2 != 0 {
		return fmt.Errorf("literal %q has unbalanced single quotes", lit)
	}
	return nil
}
