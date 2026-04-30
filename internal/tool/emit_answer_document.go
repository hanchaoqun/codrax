package tool

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
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

type answerDocValidationError struct {
	code     string
	msg      string
	hint     string
	fields   []string
	metadata map[string]string
}

func (e *answerDocValidationError) Error() string { return e.msg }

func newAnswerDocValidationError(code, format string, args ...interface{}) *answerDocValidationError {
	return &answerDocValidationError{
		code: strings.TrimSpace(code),
		msg:  fmt.Sprintf(format, args...),
	}
}

func (e *answerDocValidationError) WithHint(hint string) *answerDocValidationError {
	if e == nil {
		return nil
	}
	e.hint = strings.TrimSpace(hint)
	return e
}

func (e *answerDocValidationError) WithFields(fields ...string) *answerDocValidationError {
	if e == nil {
		return nil
	}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		e.fields = append(e.fields, field)
	}
	return e
}

func (e *answerDocValidationError) WithMetadata(key, value string) *answerDocValidationError {
	if e == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return e
	}
	if e.metadata == nil {
		e.metadata = make(map[string]string, 1)
	}
	e.metadata[key] = value
	return e
}

func answerDocValidationCode(err error) string {
	var coded *answerDocValidationError
	if errors.As(err, &coded) {
		return coded.code
	}
	return ""
}

func answerDocValidationRepair(err error) *types.ToolRepair {
	var coded *answerDocValidationError
	if !errors.As(err, &coded) || coded == nil {
		return nil
	}
	repair := &types.ToolRepair{
		Code: strings.TrimSpace(coded.code),
		Hint: strings.TrimSpace(coded.hint),
	}
	if len(coded.fields) > 0 {
		repair.Fields = append([]string(nil), coded.fields...)
	}
	if len(coded.metadata) > 0 {
		repair.Metadata = make(map[string]string, len(coded.metadata))
		for k, v := range coded.metadata {
			repair.Metadata[k] = v
		}
	}
	if repair.Code == "" && repair.Hint == "" && len(repair.Fields) == 0 && len(repair.Metadata) == 0 {
		return nil
	}
	return repair
}

func renderAnswerDocRejectSummary(code, msg string) string {
	msg = strings.TrimSpace(msg)
	if strings.TrimSpace(code) == "" {
		return msg
	}
	return fmt.Sprintf("[answer_doc_reject:%s] %s", code, msg)
}

// computeAnswerDocAttemptShape captures the size profile of an
// emit_answer_document payload so the catastrophic-regression
// detector can compare it against the prior emit. Counts the
// non-empty footprint of every grounded field; flags presence (not
// content) of optional structured payloads.
func computeAnswerDocAttemptShape(p *emitAnswerDocumentParams) types.AnswerDocAttemptShape {
	if p == nil {
		return types.AnswerDocAttemptShape{}
	}
	return types.AnswerDocAttemptShape{
		CitationsCount:     len(p.Citations),
		StepsCount:         len(p.Steps),
		SymbolsCount:       len(p.Symbols),
		SummaryRunes:       utf8.RuneCountInString(p.Summary),
		HasValue:           p.Value != nil && (strings.TrimSpace(p.Value.Literal) != "" || strings.TrimSpace(p.Value.Key) != ""),
		HasBoolean:         p.Boolean != nil && strings.TrimSpace(p.Boolean.Decision) != "",
		HasExactResolution: p.ExactResolution != nil && strings.TrimSpace(string(p.ExactResolution.Status)) != "",
	}
}

// detectAnswerDocRegression returns (true, summary) when the new
// emit collapsed multiple fields vs the prior emit. Threshold heuristic:
//
//   - Prior had non-trivial content AND new is < 25% of prior in TWO
//     OR MORE of: citations, steps, symbols, summaryRunes
//
// Empty prior (first emit) returns false. The summary is a short,
// LLM-readable bullet list of the regressed fields used by the
// retry-hint composer.
func detectAnswerDocRegression(prior *types.AnswerDocAttemptShape, current *types.AnswerDocAttemptShape) (bool, string) {
	if prior == nil || current == nil {
		return false, ""
	}
	// Refuse to flag when prior was itself essentially empty —
	// otherwise a first-attempt empty emit followed by a still-
	// empty retry would falsely trigger.
	if prior.CitationsCount+prior.StepsCount+prior.SymbolsCount+prior.SummaryRunes < 16 {
		return false, ""
	}
	type drop struct {
		name    string
		prior   int
		current int
	}
	candidates := []drop{
		{"citations", prior.CitationsCount, current.CitationsCount},
		{"steps", prior.StepsCount, current.StepsCount},
		{"symbols", prior.SymbolsCount, current.SymbolsCount},
		{"summary runes", prior.SummaryRunes, current.SummaryRunes},
	}
	var dropped []string
	const collapseRatioPct = 25
	for _, c := range candidates {
		if c.prior < 4 {
			continue
		}
		if c.current*100 < c.prior*collapseRatioPct {
			dropped = append(dropped, fmt.Sprintf("%s %d→%d", c.name, c.prior, c.current))
		}
	}
	// Optional-structured presence flips: prior had it, current
	// dropped it. Each counts as one dropped field.
	if prior.HasValue && !current.HasValue {
		dropped = append(dropped, "value object dropped")
	}
	if prior.HasBoolean && !current.HasBoolean {
		dropped = append(dropped, "boolean object dropped")
	}
	if prior.HasExactResolution && !current.HasExactResolution {
		dropped = append(dropped, "exact_resolution dropped")
	}
	if len(dropped) < 2 {
		return false, ""
	}
	return true, strings.Join(dropped, "; ")
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
	Shape               string                       `json:"shape"`
	Summary             string                       `json:"summary"`
	ExactResolution     *types.AnswerExactResolution `json:"exact_resolution,omitempty"`
	Steps               []types.AnswerStep           `json:"steps,omitempty"`
	Symbols             []emitAnswerSymbolItem       `json:"symbols,omitempty"`
	SymbolsCompleteness string                       `json:"symbols_completeness,omitempty"`
	Value               *types.AnswerValue           `json:"value,omitempty"`
	Boolean             *emitAnswerDocumentBoolean   `json:"boolean,omitempty"`
	Citations           []types.Citation             `json:"citations,omitempty"`
	Caveats             []string                     `json:"caveats,omitempty"`
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
		"inside the per-trace WorkDir. 'summary' is the only LLM-prose field. For `shape=value` and `shape=config_value`, that summary is REQUIRED and must be non-empty; for `shape=explanation`, summary is the full answer body. " + capGuidance + " " +
		"When the prompt includes an Exact Resolution Contract, also fill `exact_resolution{status,anchor?,context_mode}` so the system can validate the exact-target disposition structurally instead of inferring it from prose. " +
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
		" REQUIRED for 'explanation', 'value', and 'config_value'. For 'step_list', 'list_of_symbols', and 'boolean', summary is the lead-in / framing prose and may also need to carry a required grounded diagram."
	// symbols[].kind enum is sourced from types.AnswerSymbolKindSchemaEnum
	// so schema stays in lockstep with emit_answer_symbol's validator.
	return json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "shape": {"type": "string", "enum": ["list_of_symbols", "step_list", "value", "boolean", "config_value", "explanation"], "description": "Closed enum of answer shapes. REQUIRED. Choose the shape declared in the prompt's Target answer shape section."},
    "summary": {"type": "string", "description": %q},
    "exact_resolution": {
      "type": "object",
      "description": "Structured exact-target disposition. REQUIRED when the prompt includes an Exact Resolution Contract. The system validates this against the current exact-resolution state; do not try to encode the status only in prose.",
      "properties": {
        "status": {"type": "string", "enum": ["exact_match", "alias_match", "absent"], "description": "exact_match = the requested exact target is grounded directly; alias_match = the repo contains explicit grounded alias/parser-mapping proof; absent = the requested exact target is absent and any nearby context is related context only."},
        "anchor": {"type": "string", "description": "OPTIONAL for exact_match, REQUIRED for alias_match. Name the grounded anchor symbol / key / path that resolves the target."},
        "context_mode": {"type": "string", "enum": ["none", "grounded_context_only"], "description": "How any nearby grounded context should be framed relative to the exact target. Use grounded_context_only when the summary adds same-family / same-directory context that is NOT itself the exact target."}
      },
      "required": ["status"]
    },
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

	// Capture the prior emit attempt's size profile BEFORE we run
	// any validation; the catastrophic-regression detector compares
	// this current emit against the prior one to flag "you dropped
	// most fields" failures (the iter=2 → iter=3 collapse trace at
	// /home/chatpp/pytest 2026-04-29 23:11). The new shape is
	// computed alongside so the persistence step at every exit point
	// has a value ready.
	priorShape := ctx.Mutable.LastAnswerDocAttemptShape()
	newShape := computeAnswerDocAttemptShape(&p)
	ctx.Mutable.SetLastAnswerDocAttemptShape(&newShape)

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
		target := types.EffectiveRequiredAnswerShape(ctx.AnalysisIR, ctx.Mutable)
		if target == types.ShapeNone {
			target = ""
		}
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
	failValidation := func(err error) (types.ToolResult, error) {
		repair := answerDocValidationRepair(err)
		// Catastrophic-regression upgrade: if the LLM's retry
		// emit dropped multiple grounded fields vs the prior
		// attempt, prepend a payload-restoration directive to the
		// existing field-specific Repair. The downstream
		// answer_document_evaluator's Observe path inspects
		// Repair.Code == "payload_regression" and surfaces
		// answerDocPayloadRegressionHint instead of the normal
		// field-correction hint, putting payload restoration
		// FIRST and field correction SECOND.
		if regression, summary := detectAnswerDocRegression(priorShape, &newShape); regression {
			if repair == nil {
				repair = &types.ToolRepair{}
			}
			// Preserve the original code as a sub-reason so
			// debugging logs still record what the underlying
			// rejection was; the new Code drives the hint.
			if repair.Code != "" && repair.Code != "payload_regression" {
				repair.Code = "payload_regression:" + repair.Code
			} else {
				repair.Code = "payload_regression"
			}
			// Preserve original Hint (field-specific) but tag the
			// repair with a regression-summary metadata so the
			// evaluator can surface the dropped-field summary in
			// its hint without re-deriving it.
			if repair.Metadata == nil {
				repair.Metadata = map[string]string{}
			}
			repair.Metadata["payload_regression_summary"] = summary
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   renderAnswerDocRejectSummary(answerDocValidationCode(err), err.Error()),
			Repair:    repair,
			Timestamp: now,
		}, nil
	}

	resolvedExact, resolvedSummary, err := resolveAnswerDocumentExactResolution(strings.TrimSpace(p.Summary), p.ExactResolution, ctx)
	if err != nil {
		return failValidation(err)
	}
	p.Summary = resolvedSummary

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
		return failValidation(err)
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
	stepCandidates := compiledStepCandidateNames(answerSurfacePlan(ctx))
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
	rawSurfaceCitations := append([]types.Citation(nil), p.Citations...)
	p.Citations = normalizeLogSourceDriftObservedCitations(p.Citations, ctx)
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
		return failValidation(cerr)
	}
	citations = pruneExplanationCitationsForSurface(shape, citations, rawSurfaceCitations, groundCtx, ctx)
	citations = normalizeFollowOnGroundedContextCitations(citations, resolvedExact, p.Summary, ctx)
	applyCitationRemap(&p, citationRemap)
	numCites := len(citations)
	if dropped := countDroppedCitations(citationRemap); dropped > 0 || len(citationWarnings) > 0 {
		logging.Warning("[emit_answer_document] citation grounding: kept=%d dropped=%d warnings=%d", numCites, dropped, len(citationWarnings))
	}
	p.Summary = normalizeRequiredDiagramSummarySurface(p.Summary, citations, groundCtx, ctx)
	p.Summary = normalizeMinimalRoleLocateSummarySurface(p.Summary, shape, &p, citations, ctx)
	p.Summary = normalizeConfigTraceAbsentSummarySurface(p.Summary, citations, groundCtx, ctx, resolvedExact)
	p.Summary = normalizeLogSourceDriftSummarySurface(p.Summary, citations, groundCtx, ctx)
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
			b.WriteString("\n  (c) for a citation whose file:line came from prior-stage prose like '[relationship] X calls Y – file:line', that line is X's CALL SITE for Y, not X's definition – do not cite it as X's location.")
		}
		code := ""
		var repair *types.ToolRepair
		if format == "%v" && len(args) == 1 {
			if err, ok := args[0].(error); ok {
				code = answerDocValidationCode(err)
				repair = answerDocValidationRepair(err)
			}
		}
		return types.ToolResult{
			ToolName:  t.Name(),
			Success:   false,
			Summary:   renderAnswerDocRejectSummary(code, b.String()),
			Repair:    repair,
			Timestamp: now,
		}, nil
	}

	doc := &types.AnswerDocument{
		Shape:           shape,
		Summary:         strings.TrimSpace(p.Summary),
		ExactResolution: resolvedExact,
		Citations:       citations,
	}

	// Mermaid → aligned ASCII. When the model emits a ```mermaid```
	// fenced block, the pgavlin/mermaid-ascii library re-lays it
	// out into deterministically-aligned ASCII so terminal readers
	// see a clean diagram regardless of locale / font / CJK content.
	// Failure modes (parse error, unsupported feature, panic) all
	// degrade to "leave block unchanged" — no regression risk. The
	// model's free ASCII art (no `mermaid` tag) passes through this
	// step untouched.
	doc.Summary = render.RenderMermaidBlocks(doc.Summary)

	// Session-22 fix F4.1 — diagram-block literal-grounding gate.
	//
	// The per-shape literal-grounding wrappers (value / steps / symbols
	// / boolean) check that each citation's ±3-line window overlaps
	// identifier tokens in the claim field, but they do NOT inspect
	// summary prose; and ShapeExplanation intentionally skips per-cite
	// corroboration because narrative prose shares identifier tokens
	// with citations ambiently. That exemption opened a hole:
	// diagrams (Mermaid blocks or ASCII call-chain / flow / sequence
	// art) rendered inside fenced code blocks ARE structural claims
	// about specific repo files, and the LLM can name a file there
	// (e.g. an arrow node labeled "explorer.go (ParseOutput)" pointing
	// at "buildAnalysisIR") without putting it in citations[].
	//
	// This gate scans every triple-backtick block in summary for
	// file-like tokens with known code extensions and rejects tokens
	// that do not appear in citations[] or the attached-log
	// ResolvedFiles allowlist. Applies to every shape — an explanation
	// or step_list answer can carry the same hallucinated diagram.
	//
	// The gate runs on p.Summary (the original LLM-emitted text),
	// BEFORE render.RenderMermaidBlocks would replace mermaid
	// fences with ASCII grids. That ordering is deliberate: gate the
	// authoritative source, not the rendered presentation. File
	// labels inside Mermaid `A["foo.go"]` syntax are caught by the
	// same diagramFileTokenRe regex used for ASCII art.
	if err := validateSummaryRequiredDiagram(p.Summary, ctx); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateSummaryDiagramGrounding(p.Summary, citations, groundCtx, ctx); err != nil {
		return failWithContext("%v", err)
	}
	// Config-trace fence-label structural gate. Replaces the prompt-
	// side literal-CLI prohibition with a two-channel grounding rule
	// (role label OR cited file path). Scopes itself to config-trace
	// questions via the AnswerSurfacePlan; non-config-trace turns are
	// no-ops.
	if err := validateSummaryConfigTraceFenceLabels(p.Summary, citations, groundCtx, ctx); err != nil {
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
	if err := validateAnswerDocumentExactResolutionProof(resolvedExact, citations, groundCtx, ctx); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateConfigTraceAbsenceCitationFocus(ctx, resolvedExact, citations, p.Summary); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateExactResolutionContextSurface(p.Summary, resolvedExact, ctx); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateAbsentExactConfigValueShape(shape, resolvedExact, ctx); err != nil {
		return failWithContext("%v", err)
	}
	if err := validateRequestedEnumerationBoundary(shape, &p, ctx); err != nil {
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
			sym, perr := buildEmitAnswerSymbolItem(in, i, workDir, docLogTriageBundle, groundCtx, stepCandidates)
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
		p.Steps = normalizeStepBackboneDescriptions(p.Steps, citations, groundCtx, ctx)
		p.Steps = normalizeDriftBoundedRootCauseSteps(p.Steps, ctx)
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
		if err := validateLogSourceDriftStepCitations(p.Steps, ctx); err != nil {
			return failWithContext("%v", err)
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
		if err := validateValueCitationFocus(ctx, p.Value, citations, groundCtx); err != nil {
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
		if !types.ExplanationAllowsAnchorSkeleton(ctx.AnalysisIR) {
			if note := scrubForbiddenNonZeroFields(&p, shape, forbidSymbols); note != "" {
				shapeCorrectionNote = joinNote(shapeCorrectionNote, note)
			}
		} else if len(p.Symbols) > 0 {
			built := make([]types.AnswerSymbol, 0, len(p.Symbols))
			for i, in := range p.Symbols {
				sym, perr := buildEmitAnswerSymbolItem(in, i, workDir, docLogTriageBundle, groundCtx, stepCandidates)
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
	doc.Caveats = appendLogSourceDriftCaveat(doc.Caveats, ctx)

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
	if citationCorroboratesClaim(claim, citeFile, citeLine, gc, cfg.extraClaims...) {
		return nil
	}
	msg := fmt.Sprintf("%s is not corroborated by %s (%s:%d): the cited line and ±%d-line window contain no identifier overlap with the claim. %s",
		cfg.claimLabel, cfg.citeLabel, citeFile, citeLine, corroborationWindow, cfg.escape)
	if cfg.code == "" {
		return fmt.Errorf("%s", msg)
	}
	return newAnswerDocValidationError(cfg.code, "%s", msg).
		WithFields(cfg.fields...).
		WithHint(cfg.hint)
}

type citationCorroborationState int

const (
	citationCorroborationUnavailable citationCorroborationState = iota
	citationCorroborationTokenless
	citationCorroborationCorroborated
	citationCorroborationMissing
)

func citationCorroboratesClaim(claim, citeFile string, citeLine int, gc *ground.Context, extraClaims ...string) bool {
	return citationCorroborationStatus(claim, citeFile, citeLine, gc, extraClaims...) != citationCorroborationMissing
}

func citationCorroborationStatus(claim, citeFile string, citeLine int, gc *ground.Context, extraClaims ...string) citationCorroborationState {
	if gc == nil || len(gc.LineIndex) == 0 {
		return citationCorroborationUnavailable
	}
	if citeFile == "" || citeLine <= 0 {
		return citationCorroborationUnavailable
	}
	fileLines, ok := gc.LineIndex[citeFile]
	if !ok || len(fileLines) == 0 {
		return citationCorroborationUnavailable
	}
	tokens := valueLiteralTokenRe.FindAllString(claim, -1)
	for _, extra := range extraClaims {
		tokens = append(tokens, valueLiteralTokenRe.FindAllString(extra, -1)...)
	}
	if len(tokens) == 0 {
		return citationCorroborationTokenless
	}
	wanted := make(map[string]bool, len(tokens))
	for _, tok := range tokens {
		wanted[tok] = true
	}
	sawWindowLine := false
	for line := citeLine - corroborationWindow; line <= citeLine+corroborationWindow; line++ {
		if line <= 0 {
			continue
		}
		text, ok := fileLines[line]
		if !ok {
			continue
		}
		sawWindowLine = true
		for _, tok := range valueLiteralTokenRe.FindAllString(text, -1) {
			if wanted[tok] {
				return citationCorroborationCorroborated
			}
		}
	}
	if !sawWindowLine {
		return citationCorroborationUnavailable
	}
	// Token-set route did not corroborate. For mixed-language claims
	// — e.g. `value.literal "用户已存在 ProcessRequest"` cited at a line
	// whose ASCII identifier portion happens not to match — try a
	// byte-level substring fallback against the same ±window. The
	// trimmed full claim must appear verbatim somewhere in the window
	// for the fallback to accept; otherwise the strict rejection
	// below stands.
	if trimmed := strings.TrimSpace(claim); trimmed != "" {
		for line := citeLine - corroborationWindow; line <= citeLine+corroborationWindow; line++ {
			if line <= 0 {
				continue
			}
			text, ok := fileLines[line]
			if !ok {
				continue
			}
			if strings.Contains(text, trimmed) {
				return citationCorroborationCorroborated
			}
		}
	}
	return citationCorroborationMissing
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
	code        string
	fields      []string
	hint        string
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
		return newAnswerDocValidationError("scalar_summary_required", "shape=value requires summary to name the subject (file path / symbol / directory / measurement target) AND the methodology (command, chain, lookup) that produced the literal — the bare literal %q alone is not a complete answer (current summary length %d runes, minimum %d). The renderer prints summary first, then the literal — readers need both to act on or audit the value. Examples of acceptable summary content: state what was measured (the entity from the question), how it was measured (the find/grep/wc/exec command or chain that produced the value), and any non-obvious scope notes (excluded files, traversal direction, etc.)",
			literal, len([]rune(trimmed)), minSummaryChars).
			WithFields("summary", "value.literal").
			WithHint("Re-emit `emit_answer_document` with the same scalar payload, keep the grounded literal and citation unchanged, and expand `summary` so it names the measured subject and how the value was obtained. Do not reopen files or change the answer shape.")
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
		code:   "literal_grounding",
		fields: []string{"value.literal", "value.citation_ref", "citations"},
		hint:   "Re-emit `emit_answer_document` with a citation whose corroboration window contains an identifier from `value.literal`, or set `citation_ref=-1` if the scalar comes only from external/log context. Keep the answer shape unless the tool explicitly tells you otherwise.",
	}
	if isConfigValue && v.Key != "" {
		cfg.extraClaims = []string{v.Key}
	}
	return requireCitationCorroboration(v.Literal, cite.File, cite.Line, gc, cfg)
}

func validateValueCitationFocus(ctx *types.BusContext, v *types.AnswerValue, citations []types.Citation, gc *ground.Context) error {
	if ctx == nil || v == nil || len(citations) <= 1 {
		return nil
	}
	subjectKind := valueCitationFocusSubjectKind(ctx)
	if !valueSubjectNeedsCitationFocus(subjectKind) {
		return nil
	}
	literalKey := valueCitationFocusKey(subjectKind, v.Literal)
	if literalKey == "" {
		return nil
	}
	pool := answerDocSurfaceEvidencePool(ctx)
	for idx, cite := range citations {
		if idx == v.CitationRef {
			continue
		}
		matched := matchingEvidenceForCitation(pool, cite)
		if valueCitationSupportsLiteral(subjectKind, literalKey, v.Literal, cite, matched, gc) {
			continue
		}
		return newAnswerDocValidationError(
			"scalar_citation_focus",
			"shape=value is a scalar lookup answer, so secondary citations must directly support the emitted literal %q. citations[%d] (%s:%d) does not directly define or reference that literal; keep the defining line and, if needed, one direct call/reference, but move broader background into summary without this citation.",
			v.Literal, idx, cite.File, cite.Line,
		).
			WithFields("value.literal", "value.citation_ref", fmt.Sprintf("citations[%d]", idx)).
			WithHint("Re-emit `emit_answer_document` with `shape=value` still intact, keep the literal's own defining citation, and drop any secondary citation that does not directly define or reference the same literal. Broader background belongs in `summary` without extra citations.")
	}
	return nil
}

func validateConfigTraceAbsenceCitationFocus(ctx *types.BusContext, exact *types.AnswerExactResolution, citations []types.Citation, summary string) error {
	if ctx == nil || exact == nil {
		return nil
	}
	if exact.Status != types.AnswerExactResolutionAbsent || exact.ContextMode != types.AnswerExactResolutionContextGroundedOnly {
		return nil
	}
	if ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace ||
		ctx.AnalysisIR.RequestModel.AnswerSubject.Kind != types.SubjectConfigKey ||
		ctx.Mutable == nil {
		return nil
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.ExactResolution == nil {
		return nil
	}
	contract := plan.ExactResolution
	pool := plan.SurfaceEvidence
	lead := renderAnswerDocumentExactResolutionLeadClean(contract, exact, requestedAnswerDocumentLanguage(ctx))
	body := strings.TrimSpace(exactContextSummaryBodyAfterLead(summary, lead))
	bodyNeedsLineageCitation := exactContextBodyNeedsStructuredGrounding(body)
	requiredFiles := plan.ExactContextRequiredFiles
	lineageCandidates := plan.RelatedContextCitationCandidates
	allowedAnchors := types.JoinExactContextSurfaceDisplays(plan.AllowedExactContextLabels)
	roleCoverage := formatConfigTraceRoleCoverage(plan)
	proseOnlyRelatedContext := configTraceNearbyContextIsProseOnly(plan)
	relatedContextCitations := 0
	for idx, cite := range citations {
		matched := matchingEvidenceForCitation(pool, cite)
		if configTraceAbsenceCitationAllowed(ctx, contract, matched) {
			if matched.ContextRole != types.EvidenceContextRoleAbsenceSupport &&
				types.ConfigTraceGroundedContextAnchorAllowedInFiles(contract, matched, requiredFiles) {
				relatedContextCitations++
			}
			continue
		}
		if types.ExactResolutionAnswerContextAnchorAllowedInFiles(contract, ctx.AnalysisIR.RequestModel.Scenario, true, matched, requiredFiles) {
			err := newAnswerDocValidationError(
				"config_trace_context_citation",
				"exact-absent config-trace answers may cite only (a) the grounded absence-proof sources for the missing key or (b) grounded related-context anchors that carry a validated precedence role. citations[%d] (%s:%d) is a grounded same-scope context anchor, but it is not citation-grade precedence evidence yet; remove it from `citations[]` and any fenced diagram nodes. It may stay as uncited prose-only nearby context if the visible answer also cites a validated precedence anchor for its lineage explanation.",
				idx, cite.File, cite.Line,
			).
				WithFields(fmt.Sprintf("citations[%d]", idx), "exact_resolution.context_mode").
				WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but remove this anchor from `citations[]` and from any fenced diagram nodes. You may keep it in `summary` as prose-only grounded nearby context, but if the user-visible answer still explains precedence / lineage, cite at least one validated default/config/runtime/override anchor. Here `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). If you keep multi-layer precedence on the surface, preserve at least one visible anchor for each available precedence role instead of collapsing everything to a single mechanism anchor.")
			if allowed := formatConfigTraceAllowedCitations(plan); allowed != "" {
				err = err.WithMetadata("allowed_citations", allowed)
			}
			if roleCoverage != "" {
				err = err.WithMetadata("precedence_role_anchors", roleCoverage)
			}
			if allowedAnchors != "" {
				err = err.WithMetadata("allowed_anchors", allowedAnchors)
			}
			if proseOnlyRelatedContext {
				err = err.WithMetadata("nearby_context_citation_mode", "prose_only")
				err = err.WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly))
			}
			err = err.WithMetadata("drop_citations", fmt.Sprintf("%s:%d", cite.File, cite.Line))
			if proseOnly := types.JoinExactContextSurfaceDisplays(types.ExactContextSurfaceLabelsForItem(contract, matched)); proseOnly != "" {
				err = err.WithMetadata("prose_only_anchors", proseOnly)
			}
			return err
		}
		err := newAnswerDocValidationError(
			"config_trace_context_citation",
			"exact-absent config-trace answers may cite only (a) the grounded absence-proof sources for the missing key or (b) grounded related-context anchors that carry a validated precedence role. citations[%d] (%s:%d) is broad same-family background rather than a precedence-capable lineage anchor; drop it from citations and keep that background out of the answer surface.",
			idx, cite.File, cite.Line,
		).
			WithFields(fmt.Sprintf("citations[%d]", idx), "exact_resolution.context_mode").
			WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but keep citations only for the missing-key proof sources and for grounded precedence anchors that already carry validated default/config/runtime/override roles. Here `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). Drop broad same-family structs, counters, or helper comments from `citations[]` and from the rendered answer. If you still explain multi-layer precedence, preserve one visible anchor per available precedence role when possible.")
		if allowed := formatConfigTraceAllowedCitations(plan); allowed != "" {
			err = err.WithMetadata("allowed_citations", allowed)
		}
		if roleCoverage != "" {
			err = err.WithMetadata("precedence_role_anchors", roleCoverage)
		}
		if allowedAnchors != "" {
			err = err.WithMetadata("allowed_anchors", allowedAnchors)
		}
		if proseOnlyRelatedContext {
			err = err.WithMetadata("nearby_context_citation_mode", "prose_only")
			err = err.WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly))
		}
		err = err.WithMetadata("drop_citations", fmt.Sprintf("%s:%d", cite.File, cite.Line))
		if forbidden := types.JoinExactContextSurfaceDisplays(types.ExactContextSurfaceLabelsForItem(contract, matched)); forbidden != "" {
			err = err.WithMetadata("forbidden_anchors", forbidden)
		}
		return err
	}
	if !bodyNeedsLineageCitation || relatedContextCitations > 0 || len(lineageCandidates) == 0 {
		return nil
	}
	err := newAnswerDocValidationError(
		"config_trace_context_citation",
		"exact-absent config-trace answers that keep grounded related context on the user-visible surface must cite at least one grounded precedence-capable lineage anchor. The current summary explains nearby precedence / context, but citations[] contains no validated default/config/runtime/override anchor for that explanation.",
	).WithFields("citations", "summary", "exact_resolution.context_mode").
		WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but if `summary` continues to explain nearby precedence / lineage context, keep at least one grounded precedence anchor in `citations[]` (for example a validated default/config/runtime/override anchor already named in the tool metadata). Here `config` means a grounded repo/user config-file layer (YAML/JSON/TOML/INI/etc.). If multiple precedence roles are available, preserve one visible anchor per role when possible instead of collapsing to a single mechanism anchor. If you do not want to cite nearby lineage context, drop that contextual explanation and keep only the renderer-generated absence lead.")
	if allowed := formatConfigTraceAllowedCitations(plan); allowed != "" {
		err = err.WithMetadata("allowed_citations", allowed)
	}
	if roleCoverage != "" {
		err = err.WithMetadata("precedence_role_anchors", roleCoverage)
	}
	if allowedAnchors != "" {
		err = err.WithMetadata("allowed_anchors", allowedAnchors)
	}
	if proseOnlyRelatedContext {
		err = err.WithMetadata("nearby_context_citation_mode", "prose_only")
		err = err.WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly))
	}
	return err
}

func configTraceNearbyContextIsProseOnly(plan *types.AnswerSurfacePlan) bool {
	if plan == nil || len(plan.ProseOnlyExactContextItems) == 0 {
		return false
	}
	for _, item := range plan.CitationGradeExactContextItems {
		if item.ContextRole != types.EvidenceContextRoleAbsenceSupport {
			return false
		}
	}
	return true
}

func normalizeFollowOnGroundedContextCitations(citations []types.Citation, exact *types.AnswerExactResolution, summary string, ctx *types.BusContext) []types.Citation {
	if len(citations) == 0 || exact == nil || exact.Status != types.AnswerExactResolutionAbsent || exact.ContextMode != types.AnswerExactResolutionContextGroundedOnly {
		if exact == nil || exact.Status != types.AnswerExactResolutionAbsent || exact.ContextMode != types.AnswerExactResolutionContextGroundedOnly {
			return citations
		}
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceFollowOnGroundedContext {
		return citations
	}
	contract := plan.ExactResolution
	lead := renderAnswerDocumentExactResolutionLeadClean(contract, exact, requestedAnswerDocumentLanguage(ctx))
	body := strings.TrimSpace(exactContextSummaryBodyAfterLead(summary, lead))
	mentionsAllowed := len(mentionedExactContextSurfaceLabelHits(body, plan.AllowedExactContextLabels)) > 0
	allowed := make(map[string]bool)
	citationGrade := make(map[string]bool)
	for _, item := range plan.CitationGradeExactContextItems {
		if key := explanationCitationLookupKey(item.Source, item.LineStart); key != "" {
			allowed[key] = true
			citationGrade[key] = true
		}
	}
	for _, candidate := range plan.RelatedContextCitationCandidates {
		if key := explanationCitationLookupKey(candidate.Source, candidate.Line); key != "" {
			allowed[key] = true
		}
	}
	if len(allowed) == 0 {
		return citations
	}
	pool := answerDocSurfaceEvidencePool(ctx)
	out := make([]types.Citation, 0, len(citations))
	changed := false
	hasCitationGrade := false
	for _, cite := range citations {
		matched := matchingEvidenceForCitation(pool, cite)
		if matched.ContextRole == types.EvidenceContextRoleAbsenceSupport {
			out = append(out, cite)
			continue
		}
		key := explanationCitationLookupKey(cite.File, cite.Line)
		if allowed[key] {
			out = append(out, cite)
			if citationGrade[key] {
				hasCitationGrade = true
			}
			continue
		}
		changed = true
	}
	if mentionsAllowed && !hasCitationGrade {
		existing := make(map[string]bool, len(out))
		for _, cite := range out {
			existing[explanationCitationLookupKey(cite.File, cite.Line)] = true
		}
		appended := 0
		for _, item := range plan.CitationGradeExactContextItems {
			key := explanationCitationLookupKey(item.Source, item.LineStart)
			if key == "" || existing[key] {
				continue
			}
			out = append(out, types.Citation{File: item.Source, Line: item.LineStart})
			existing[key] = true
			changed = true
			hasCitationGrade = true
			appended++
			if appended >= 2 {
				break
			}
		}
		if !hasCitationGrade {
			for _, candidate := range plan.RelatedContextCitationCandidates {
				key := explanationCitationLookupKey(candidate.Source, candidate.Line)
				if key == "" || existing[key] {
					continue
				}
				out = append(out, types.Citation{File: candidate.Source, Line: candidate.Line})
				existing[key] = true
				changed = true
				appended++
				if appended >= 2 {
					break
				}
			}
		}
	}
	if summaryDiagramFenceCount(summary) > 0 && len(plan.ConfigTraceDiagramAnchors) > 0 {
		existing := make(map[string]bool, len(out))
		for _, cite := range out {
			existing[explanationCitationLookupKey(cite.File, cite.Line)] = true
		}
		for _, anchor := range plan.ConfigTraceDiagramAnchors {
			key := explanationCitationLookupKey(anchor.Source, anchor.Line)
			if key == "" || existing[key] {
				continue
			}
			out = append(out, types.Citation{File: anchor.Source, Line: anchor.Line})
			existing[key] = true
			changed = true
		}
	}
	if !changed {
		return citations
	}
	return out
}

func exactContextBodyNeedsStructuredGrounding(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	if strings.Contains(body, "```") {
		return true
	}
	if strings.Contains(body, "\n###") || strings.HasPrefix(body, "###") {
		return true
	}
	if strings.Count(body, "\n\n") >= 1 {
		return true
	}
	return utf8.RuneCountInString(body) >= 180
}

func answerDocExactContextRequiredFiles(ctx *types.BusContext) []string {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	return ctx.Mutable.ExactContextRequiredFiles()
}

func answerDocSurfaceEvidencePool(ctx *types.BusContext) []types.EvidenceItem {
	if ctx == nil {
		return nil
	}
	var emitted []types.EvidenceItem
	if ctx.Mutable != nil {
		emitted = ctx.Mutable.EmittedEvidence()
	}
	return types.ExactResolutionSurfaceEvidencePool(emitted, ctx.EvidenceItems, ctx.AnswerChains)
}

func formatConfigTraceAllowedCitations(plan *types.AnswerSurfacePlan) string {
	if plan == nil {
		return ""
	}
	seen := make(map[string]bool)
	var out []string
	for _, item := range plan.CitationGradeExactContextItems {
		if item.Source == "" || item.LineStart <= 0 {
			continue
		}
		label := fmt.Sprintf("%s:%d", item.Source, item.LineStart)
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	for _, candidate := range plan.RelatedContextCitationCandidates {
		label := fmt.Sprintf("%s:%d", candidate.Source, candidate.Line)
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func formatCitationLocations(citations []types.Citation) string {
	seen := make(map[string]bool)
	var out []string
	for _, c := range citations {
		file := strings.TrimSpace(c.File)
		if file == "" || c.Line <= 0 {
			continue
		}
		label := fmt.Sprintf("%s:%d", file, c.Line)
		if seen[label] {
			continue
		}
		seen[label] = true
		out = append(out, label)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

func formatConfigTraceRoleCoverage(plan *types.AnswerSurfacePlan) string {
	if plan == nil || len(plan.ConfigTraceDiagramAnchors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(plan.ConfigTraceDiagramAnchors))
	for _, anchor := range plan.ConfigTraceDiagramAnchors {
		role := strings.TrimSpace(anchor.Role)
		label := types.ConfigTraceDiagramAnchorSupportLabel(anchor)
		if role == "" || label == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", role, label))
	}
	return strings.Join(parts, ", ")
}

func validateExactResolutionContextSurface(summary string, exact *types.AnswerExactResolution, ctx *types.BusContext) error {
	if exact == nil || ctx == nil || ctx.Mutable == nil || ctx.AnalysisIR == nil {
		return nil
	}
	if exact.Status != types.AnswerExactResolutionAbsent {
		return nil
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.ExactResolution == nil || len(plan.ExactResolution.Targets) == 0 {
		return nil
	}
	contract := plan.ExactResolution
	if !plan.StableAbsent {
		return nil
	}
	allowedAnchors := types.JoinExactContextSurfaceDisplays(plan.AllowedExactContextLabels)
	lead := renderAnswerDocumentExactResolutionLeadClean(contract, exact, requestedAnswerDocumentLanguage(ctx))
	body := exactContextSummaryBodyAfterLead(summary, lead)
	if repeated := repeatedExactTargetAfterLead(contract, body); repeated != "" {
		err := newAnswerDocValidationError(
			"exact_context_surface",
			"exact-absent answers with grounded related context must keep the exact target in the renderer-generated lead only. The current summary still reuses %s instead of staying on grounded nearby anchors.",
			repeated,
		).
			WithFields("summary").
			WithMetadata("repeated_target", repeated).
			WithMetadata("lead_source", "exact_resolution")
		if allowedAnchors != "" {
			err = err.WithMetadata("allowed_anchors", allowedAnchors)
		}
		return err.WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but treat `summary` as grounded nearby context only. Do not restate the exact target there: the renderer already prints the exact-absence lead. Keep later prose and diagrams on the already-allowed grounded anchors / mechanisms only.")
	}
	mentionedAllowedLabels := mentionedExactContextSurfaceLabelHits(body, plan.AllowedExactContextLabels)
	mentionedForbiddenLabels := filterShadowedForbiddenExactContextSurfaceLabels(
		mentionedAllowedLabels,
		mentionedExactContextSurfaceLabelHits(body, plan.ForbiddenExactContextLabels),
	)
	mentionedAllowed := exactContextSurfaceLabelDisplays(mentionedAllowedLabels)
	mentionedForbidden := exactContextSurfaceLabelDisplays(mentionedForbiddenLabels)
	if exact.ContextMode == types.AnswerExactResolutionContextNone {
		if strings.TrimSpace(body) == "" && len(mentionedAllowed) == 0 && len(mentionedForbidden) == 0 {
			return nil
		}
		err := newAnswerDocValidationError(
			"exact_resolution",
			"exact-resolution contract violated: exact_resolution.status=absent summary includes nearby grounded/background context beyond the renderer-generated absence lead, so exact_resolution.context_mode must be \"grounded_context_only\"",
		).WithFields("exact_resolution.context_mode", "summary").
			WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly)).
			WithMetadata("lead_source", "exact_resolution")
		if allowedAnchors != "" {
			err = err.WithMetadata("allowed_anchors", allowedAnchors)
		}
		if len(mentionedForbidden) > 0 {
			err = err.WithMetadata("forbidden_anchors", strings.Join(mentionedForbidden, ", "))
		}
		return err.WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but because the summary already includes nearby context, set `exact_resolution.context_mode=\"grounded_context_only\"` and keep that context on validated grounded anchors only. If you do not want contextual explanation, drop the nearby-context prose and keep only the renderer-generated absence lead.")
	}
	if exact.ContextMode != types.AnswerExactResolutionContextGroundedOnly {
		return nil
	}
	if plan.SummarySurfaceMode == types.AnswerSummarySurfaceFollowOnGroundedContext {
		bodyTrimmed := strings.TrimSpace(body)
		if bodyTrimmed == "" || len(mentionedAllowed) == 0 {
			err := newAnswerDocValidationError(
				"follow_on_grounded_context",
				"exact-absent answers in follow-on grounded-context mode must keep at least one validated nearby grounded anchor on the user-visible answer surface; the current summary collapses to the exact-absence lead without any grounded follow-on context.",
			).WithFields("summary", "exact_resolution.context_mode").
				WithMetadata("lead_source", "exact_resolution").
				WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly))
			if allowedAnchors != "" {
				err = err.WithMetadata("allowed_anchors", allowedAnchors)
			}
			return err.WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but keep the grounded nearby context visible after the renderer-generated lead. Name at least one validated nearby anchor in `summary`, keep it grounded, and do not collapse the answer to the exact-absence lead alone.")
		}
	}
	if len(mentionedForbidden) == 0 {
		return nil
	}
	err := newAnswerDocValidationError(
		"exact_context_surface",
		"exact-absent answers with grounded related context must keep background-only anchors out of the user-visible answer surface. The current summary still surfaces disallowed nearby-context anchor(s): %s.",
		strings.Join(mentionedForbidden, ", "),
	).
		WithFields("summary").
		WithMetadata("forbidden_anchors", strings.Join(mentionedForbidden, ", ")).
		WithMetadata("lead_source", "exact_resolution")
	if allowedAnchors != "" {
		err = err.WithMetadata("allowed_anchors", allowedAnchors)
	}
	return err.WithHint("Re-emit `emit_answer_document` with the same exact-absence conclusion, but keep `summary` on the renderer-produced exact lead's follow-on grounded context only. Drop illustrative/doc-example or broad same-family anchors from prose, diagrams, and citations instead of explaining them away.")
}

func mentionedExactContextSurfaceCandidates(summary string, candidates []types.ExactContextSurfaceLabel) []string {
	return exactContextSurfaceLabelDisplays(mentionedExactContextSurfaceLabelHits(summary, candidates))
}

func mentionedExactContextSurfaceLabelHits(summary string, candidates []types.ExactContextSurfaceLabel) []types.ExactContextSurfaceLabel {
	lowerSummary := strings.ToLower(summary)
	var mentioned []types.ExactContextSurfaceLabel
	for _, candidate := range candidates {
		if summaryMentionsExactContextSurfaceLabel(summary, lowerSummary, candidate) {
			mentioned = append(mentioned, candidate)
		}
	}
	return mentioned
}

func exactContextSurfaceLabelDisplays(labels []types.ExactContextSurfaceLabel) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		if display := strings.TrimSpace(label.Display); display != "" {
			out = append(out, display)
		}
	}
	return out
}

func filterShadowedForbiddenExactContextSurfaceLabels(allowed, forbidden []types.ExactContextSurfaceLabel) []types.ExactContextSurfaceLabel {
	if len(allowed) == 0 || len(forbidden) == 0 {
		return forbidden
	}
	var out []types.ExactContextSurfaceLabel
	for _, bad := range forbidden {
		if forbiddenExactContextLabelShadowedByAllowed(bad, allowed) {
			continue
		}
		out = append(out, bad)
	}
	return out
}

func forbiddenExactContextLabelShadowedByAllowed(bad types.ExactContextSurfaceLabel, allowed []types.ExactContextSurfaceLabel) bool {
	if bad.Kind != "symbol" || bad.LookupKey == "" {
		return false
	}
	for _, good := range allowed {
		if good.Kind != bad.Kind || good.LookupKey == "" || good.LookupKey == bad.LookupKey {
			continue
		}
		if strings.Contains(good.LookupKey, bad.LookupKey) {
			return true
		}
	}
	return false
}

func summaryMentionsExactContextSurfaceLabel(summary, lowerSummary string, candidate types.ExactContextSurfaceLabel) bool {
	if candidate.MatchLower == "" {
		return false
	}
	switch candidate.Kind {
	case "symbol":
		return containsSurfaceToken(lowerSummary, candidate.MatchLower)
	case "path":
		if strings.Contains(lowerSummary, candidate.MatchLower) {
			return true
		}
		if candidate.LookupKey == "" {
			return false
		}
		for _, token := range summarySurfaceTokens(summary) {
			if types.ExactResolutionLookupKey("path", token) == candidate.LookupKey {
				return true
			}
		}
		return false
	default:
		return strings.Contains(lowerSummary, candidate.MatchLower)
	}
}

func containsSurfaceToken(lowerSummary, needle string) bool {
	searchFrom := 0
	for {
		idx := strings.Index(lowerSummary[searchFrom:], needle)
		if idx < 0 {
			return false
		}
		start := searchFrom + idx
		end := start + len(needle)
		if isSurfaceTokenBoundary(lowerSummary, start-1) && isSurfaceTokenBoundary(lowerSummary, end) {
			return true
		}
		searchFrom = start + 1
	}
}

func isSurfaceTokenBoundary(s string, idx int) bool {
	if idx < 0 || idx >= len(s) {
		return true
	}
	b := s[idx]
	return !((b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_')
}

func summarySurfaceTokens(summary string) []string {
	var tokens []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, b.String())
		b.Reset()
	}
	for _, r := range summary {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case strings.ContainsRune("._-/\\:()", r):
			b.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func exactContextSummaryBodyAfterLead(summary, lead string) string {
	body := strings.TrimSpace(summary)
	lead = strings.TrimSpace(lead)
	if lead != "" && strings.HasPrefix(body, lead) {
		body = strings.TrimSpace(strings.TrimPrefix(body, lead))
	}
	return body
}

func repeatedExactTargetAfterLead(contract *types.ExactResolutionContract, body string) string {
	if contract == nil || len(contract.Targets) == 0 {
		return ""
	}
	for _, target := range contract.Targets {
		if types.ExactResolutionTextMentionsTarget(contract, body, target) {
			return "`" + strings.TrimSpace(target) + "`"
		}
	}
	return ""
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
			code:   "literal_grounding",
			fields: []string{fmt.Sprintf("symbols[%d]", i)},
			hint:   "Re-emit `emit_answer_document` with symbol entries whose cited file:line actually contains the symbol name, or drop external-only symbols instead of inventing a repo anchor.",
		}
		if err := requireCitationCorroboration(s.Name, s.File, s.Line, gc, cfg); err != nil {
			return err
		}
	}
	return nil
}

func valueCitationFocusSubjectKind(ctx *types.BusContext) types.AnswerSubjectKind {
	if ctx == nil {
		return types.SubjectUnknown
	}
	if ctx.AnalysisIR != nil {
		return ctx.AnalysisIR.RequestModel.AnswerSubject.Kind
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.AnswerSubject.Kind
		}
	}
	return types.SubjectUnknown
}

func configTraceAbsenceCitationAllowed(ctx *types.BusContext, contract *types.ExactResolutionContract, matched types.EvidenceItem) bool {
	return types.ConfigTraceGroundedContextAnchorAllowedInFiles(contract, matched, answerDocExactContextRequiredFiles(ctx))
}

func valueSubjectNeedsCitationFocus(kind types.AnswerSubjectKind) bool {
	switch kind {
	case types.SubjectFunctionName,
		types.SubjectTypeName,
		types.SubjectHandlerRoute,
		types.SubjectFilePath,
		types.SubjectStringLiteral,
		types.SubjectEnumValue,
		types.SubjectStructField,
		types.SubjectInterface:
		return true
	default:
		return false
	}
}

func valueCitationFocusKey(kind types.AnswerSubjectKind, literal string) string {
	if kind == types.SubjectFilePath {
		return types.ExactResolutionLookupKey("path", literal)
	}
	return types.ExactResolutionLookupKey("symbol", literal)
}

func valueCitationSupportsLiteral(kind types.AnswerSubjectKind, literalKey, literal string, cite types.Citation, matched types.EvidenceItem, gc *ground.Context) bool {
	if matched.Source != "" {
		if valueCitationTextMatchesLiteral(kind, literalKey, matched.Subject) ||
			valueCitationTextMatchesLiteral(kind, literalKey, matched.AnchorSymbol) ||
			valueCitationTextMatchesLiteral(kind, literalKey, matched.Object) {
			return true
		}
	}
	if citationLooksCommentOnly(cite, gc) {
		return false
	}
	if kind == types.SubjectFilePath && strings.TrimSpace(cite.File) != "" {
		return valueCitationTextMatchesLiteral(kind, literalKey, cite.File)
	}
	if strings.TrimSpace(cite.Quote) != "" {
		return valueCitationTextMatchesLiteral(kind, literalKey, cite.Quote)
	}
	return false
}

func citationLooksCommentOnly(cite types.Citation, gc *ground.Context) bool {
	if gc == nil || cite.File == "" || cite.Line <= 0 {
		return false
	}
	fileLines, ok := gc.LineIndex[cite.File]
	if !ok {
		return false
	}
	return ground.LineLooksCommentOnly(fileLines, cite.Line, cite.File)
}

func valueCitationTextMatchesLiteral(kind types.AnswerSubjectKind, literalKey, text string) bool {
	if literalKey == "" || strings.TrimSpace(text) == "" {
		return false
	}
	if kind == types.SubjectFilePath {
		return types.ExactResolutionLookupKey("path", text) == literalKey
	}
	return strings.Contains(types.ExactResolutionLookupKey("symbol", text), literalKey)
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
			code:   "literal_grounding",
			fields: []string{fmt.Sprintf("steps[%d].description", i), fmt.Sprintf("steps[%d].citation_ref", i)},
			hint:   "Re-emit `emit_answer_document` with each cited step grounded to a file:line that overlaps an identifier from that step description; if the step is an aggregate or external-only claim, keep it but set `citation_ref=-1`.",
		}
		if err := requireCitationCorroboration(s.Description, cite.File, cite.Line, gc, cfg); err != nil {
			return err
		}
	}
	return nil
}

func validateLogSourceDriftStepCitations(steps []types.AnswerStep, ctx *types.BusContext) error {
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return nil
	}
	for i, step := range steps {
		if step.CitationRef >= 0 {
			continue
		}
		return newAnswerDocValidationError(
			"log_source_drift_step_citation",
			"root-cause answers in log-source-drift mode must keep every step directly citation-backed; steps[%d] currently has citation_ref=-1 and can introduce an unsupported hypothesis beyond the grounded current mechanism.",
			i,
		).WithFields(fmt.Sprintf("steps[%d].citation_ref", i), "summary").
			WithHint("Re-emit `emit_answer_document` with the same grounded call chain and drift-bounded explanation, but either ground this step to a cited file:line or delete the unsupported hypothesis step. Do not use `citation_ref=-1` for root-cause steps in this drift-bounded mode.")
	}
	return nil
}

func normalizeDriftBoundedRootCauseSteps(steps []types.AnswerStep, ctx *types.BusContext) []types.AnswerStep {
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause || len(steps) == 0 {
		return steps
	}
	kept := make([]types.AnswerStep, 0, len(steps))
	for _, step := range steps {
		if step.CitationRef < 0 {
			continue
		}
		kept = append(kept, step)
	}
	if len(kept) == 0 || len(kept) == len(steps) {
		return steps
	}
	for i := range kept {
		kept[i].Index = i + 1
	}
	return kept
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

var diagramGroundingBodyReplacer = strings.NewReplacer(
	`\\n`, "\n",
	`\\r`, "\n",
	`<br/>`, "\n",
	`<br />`, "\n",
	`<br>`, "\n",
)

// fencedCodeBlockRe captures the body of every triple-backtick fenced
// block in the summary. The (?s) flag lets `.` cross newlines so the
// full block body is returned in submatch[1]. Anchoring on `\n` after
// the opening fence skips the optional language tag.
var fencedCodeBlockRe = regexp.MustCompile("(?s)```[^\n]*\n(.*?)```")

// fencedCodeBlockWithInfoRe is the same fence-finder but exposes the
// info-string (the language tag immediately after the opening
// triple-backticks). info-string lives in submatch[1]; body in
// submatch[2]. Used by the config-trace fence-label validator to
// scope its checks to ```mermaid``` blocks only.
var fencedCodeBlockWithInfoRe = regexp.MustCompile("(?s)```([^\n]*)\n(.*?)```")

// mermaidNodeLabelRe captures the contents of a Mermaid node-label
// bracket: the quoted form `id["raw label with spaces"]` or the bare
// form `id[raw_id]`. Two alternatives so a single regex covers both.
//
// Group 1: contents of the quoted-bracket form (no surrounding quotes).
// Group 2: contents of the bare-bracket form.
//
// Used by extractMermaidNodeLabels to pull every node label from a
// Mermaid fence body in O(n) — without writing a real Mermaid parser.
// We deliberately do not try to associate the label with its node id;
// the validator only cares about the LABEL text (what the user reads).
var mermaidNodeLabelRe = regexp.MustCompile(`\[\s*(?:"([^"]+)"|([^\]\n]+))\s*\]`)

// mermaidEdgeSplitRe splits a Mermaid line on edge operators so the
// extractor can also recover bare-id labels (a Mermaid node referenced
// without a bracket declaration uses its id as the visible label).
// Covers solid `-->`, dashed `-.->`, thick `==>`, plain `---`, and
// `<-->`.
var mermaidEdgeSplitRe = regexp.MustCompile(`-->|<-->|-\.->|==>|---|<---`)

// mermaidReservedWords are the Mermaid language reserved words that
// MUST never be treated as user-supplied node labels. These are
// language keywords (compiler-style: `subgraph` / `end` is the
// scoping construct, `flowchart` / `graph` is the diagram-type
// declaration, `LR` / `TD` / `RL` / `BT` are direction modifiers).
//
// This is NOT a "filter dictionary" of arbitrary disallowed words —
// it's the closed set of Mermaid grammar tokens (similar to Go's
// `package` / `import` / `func` keywords). Adding a new entry only
// happens when Mermaid's spec gains new syntax.
var mermaidReservedWords = map[string]bool{
	"flowchart": true,
	"graph":     true,
	"subgraph":  true,
	"end":       true,
	"classDef":  true,
	"class":     true,
	"click":     true,
	"linkStyle": true,
	"style":     true,
	"direction": true,
	"LR":        true,
	"RL":        true,
	"TD":        true,
	"TB":        true,
	"BT":        true,
	// sequenceDiagram / classDiagram families — header tokens that
	// appear on a body line by themselves when the LLM emits a
	// non-flowchart shape inside the same fence type.
	"sequenceDiagram": true,
	"classDiagram":    true,
	"stateDiagram":    true,
	"stateDiagram-v2": true,
	"erDiagram":       true,
}

// idTokenLeftOf returns the longest run of id-token characters
// (alnum / `_` / `-` / `.`) that ends at offset `end` in `s`. Used
// to recover the structural id-token glued to the left of a
// Mermaid `[...]` declaration so the validator can suppress those
// invisible-glue ids from the user-visible label set.
func idTokenLeftOf(s string, end int) string {
	start := end
	for start > 0 {
		b := s[start-1]
		if (b >= 'a' && b <= 'z') ||
			(b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9') ||
			b == '_' || b == '-' || b == '.' {
			start--
			continue
		}
		break
	}
	return s[start:end]
}

// extractMermaidNodeLabels walks a normalised Mermaid fence body and
// returns every distinct user-visible node label (in source order).
//
// Two-pass design:
//
//	pass 1 — collect every `id["label"]` / `id[label]` declaration's
//	         (id, label) pair. The id is structural glue (invisible
//	         to the reader); the label is what the user sees.
//	pass 2 — re-walk lines: emit bracketed labels (skipping their
//	         glued-id), then split bare segments on Mermaid edge
//	         operators. Bare segments that match a previously-
//	         declared id are SKIPPED (they are back-references to
//	         the labeled node, not new labels). Bare segments that
//	         have NEVER been declared are emitted as their own
//	         labels — Mermaid renders an unsubscripted id by
//	         showing the id text.
//
// Reserved words (mermaidReservedWords) and whitespace-bearing
// segments are dropped. Duplicate labels are deduped while
// preserving first-seen order so the validator's violation message
// is stable.
//
// The function is intentionally tolerant: malformed lines (parser
// error, partial label) are skipped silently rather than panicking,
// matching the rest of the validator's "fail open by leaving block
// untouched" contract.
func extractMermaidNodeLabels(body string) []string {
	out := make([]string, 0, 8)
	seen := make(map[string]bool, 8)
	emit := func(lbl string) {
		lbl = strings.TrimSpace(lbl)
		if lbl == "" {
			return
		}
		if seen[lbl] {
			return
		}
		seen[lbl] = true
		out = append(out, lbl)
	}
	// Pass 1: collect declared ids. We deliberately scan the whole
	// body once instead of accumulating per-line so a forward
	// reference (`A --> B; A["alpha"]`) still suppresses `A` from
	// the label set on the first line — Mermaid's parser is order-
	// independent for label attachment.
	declaredIds := make(map[string]bool, 8)
	for _, m := range mermaidNodeLabelRe.FindAllStringSubmatchIndex(body, -1) {
		if id := strings.TrimSpace(idTokenLeftOf(body, m[0])); id != "" {
			declaredIds[id] = true
		}
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Reserved-only header lines: skip entirely.
		first := strings.Fields(line)
		if len(first) > 0 && mermaidReservedWords[first[0]] {
			continue
		}
		// Bracketed labels (preferred channel — what
		// renderMermaidLinearFence emits). Emit every label, then
		// blank the `[id]…[/id]` spans + structural id glue so the
		// bare-id pass does not re-recover invisible ids.
		residual := line
		if matches := mermaidNodeLabelRe.FindAllStringSubmatchIndex(line, -1); len(matches) > 0 {
			for _, m := range matches {
				if m[2] >= 0 {
					emit(line[m[2]:m[3]])
				} else if m[4] >= 0 {
					emit(line[m[4]:m[5]])
				}
			}
			rb := []byte(line)
			for i := len(matches) - 1; i >= 0; i-- {
				m := matches[i]
				start := m[0]
				// Blank id-token glued to left of `[`.
				for start > 0 {
					b := rb[start-1]
					if (b >= 'a' && b <= 'z') ||
						(b >= 'A' && b <= 'Z') ||
						(b >= '0' && b <= '9') ||
						b == '_' || b == '-' || b == '.' {
						start--
						continue
					}
					break
				}
				for j := start; j < m[1]; j++ {
					rb[j] = ' '
				}
			}
			residual = string(rb)
		}
		// Bare-id channel: split on edge operators, pick segments
		// that are SHAPE-OK as a Mermaid node id (single token, no
		// whitespace inside, not a reserved word, not a previously
		// declared structural id).
		for _, seg := range mermaidEdgeSplitRe.Split(residual, -1) {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			if strings.ContainsAny(seg, " \t") {
				continue
			}
			if mermaidReservedWords[seg] {
				continue
			}
			if declaredIds[seg] {
				continue
			}
			emit(seg)
		}
	}
	return out
}

// diagramCueTokens is the lightweight structural heuristic for the
// missing-diagram gate. We intentionally keep the gate generic: any
// fenced block with at least two non-empty lines plus one of these
// cues counts as a diagram-like block, regardless of answer shape.
var diagramCueTokens = []string{
	"->", "<-", "=>", "<=", "──", "│", "├", "└", "┌", "┐", "┘", "┤",
	"┬", "┴", "◀", "▶", "▲", "▼",
}

func answerDiagramContract(ctx *types.BusContext) *types.DiagramContract {
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return nil
	}
	return plan.Diagram
}

func answerExactResolutionContract(ctx *types.BusContext) *types.ExactResolutionContract {
	if plan := answerSurfacePlan(ctx); plan != nil {
		return plan.ExactResolution
	}
	return nil
}

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

func normalizeStepBackboneDescriptions(steps []types.AnswerStep, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) []types.AnswerStep {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.StepBackbone) == 0 || len(steps) == 0 {
		return steps
	}
	anchors := make(map[string]types.StepSurfaceAnchor, len(plan.StepBackbone))
	for _, anchor := range plan.StepBackbone {
		file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		if file == "" || anchor.Line <= 0 {
			continue
		}
		anchors[fmt.Sprintf("%s:%d", file, anchor.Line)] = anchor
	}
	if len(anchors) == 0 {
		return steps
	}
	out := append([]types.AnswerStep(nil), steps...)
	for i, step := range out {
		if step.CitationRef < 0 || step.CitationRef >= len(citations) {
			continue
		}
		cite := citations[step.CitationRef]
		key := fmt.Sprintf("%s:%d", strings.TrimSpace(strings.ReplaceAll(cite.File, `\`, `/`)), cite.Line)
		anchor, ok := anchors[key]
		if !ok {
			continue
		}
		if stepDescriptionMentionsAnchor(step.Description, anchor) {
			continue
		}
		if citationCorroborationStatus(step.Description, cite.File, cite.Line, gc) == citationCorroborationCorroborated {
			continue
		}
		if desc := strings.TrimSpace(types.RenderStepSurfaceAnchorDescription(anchor)); desc != "" {
			out[i].Description = desc
		}
	}
	return out
}

func stepDescriptionMentionsAnchor(description string, anchor types.StepSurfaceAnchor) bool {
	name := strings.ToLower(strings.TrimSpace(anchor.Name))
	if name == "" {
		return false
	}
	return strings.Contains(strings.ToLower(description), name)
}

func normalizeLogSourceDriftObservedCitations(in []types.Citation, ctx *types.BusContext) []types.Citation {
	if len(in) == 0 {
		return in
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.LogObservedAnchors) == 0 {
		return in
	}
	remap := make(map[string]int, len(plan.LogObservedAnchors))
	for _, anchor := range plan.LogObservedAnchors {
		file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		if file == "" || anchor.ObservedLine <= 0 || anchor.AnchoredLine <= 0 {
			continue
		}
		remap[fmt.Sprintf("%s:%d", file, anchor.ObservedLine)] = anchor.AnchoredLine
	}
	if len(remap) == 0 {
		return in
	}
	out := append([]types.Citation(nil), in...)
	for i := range out {
		file := strings.TrimSpace(strings.ReplaceAll(out[i].File, `\`, `/`))
		if file == "" || out[i].Line <= 0 {
			continue
		}
		if anchored, ok := remap[fmt.Sprintf("%s:%d", file, out[i].Line)]; ok && anchored > 0 && anchored != out[i].Line {
			out[i].Line = anchored
			out[i].Quote = ""
		}
	}
	return out
}

func pruneExplanationCitationsForSurface(shape types.AnswerShape, citations, original []types.Citation, gc *ground.Context, ctx *types.BusContext) []types.Citation {
	if shape != types.ShapeExplanation || len(citations) == 0 {
		return citations
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return citations
	}
	originalQuotes := make(map[string]string, len(original))
	for _, cit := range original {
		key := explanationCitationLookupKey(cit.File, cit.Line)
		if key == "" || strings.TrimSpace(cit.Quote) == "" {
			continue
		}
		if _, exists := originalQuotes[key]; !exists {
			originalQuotes[key] = cit.Quote
		}
	}
	out := make([]types.Citation, 0, len(citations))
	for _, cit := range citations {
		if explanationCitationMatchesObservedAnchorLine(cit, plan) ||
			explanationCitationWithinObservedAnchorNeighborhood(cit, plan) ||
			explanationCitationMatchesRootCauseAnchorTokens(cit, originalQuotes, gc, plan) {
			out = append(out, cit)
		}
	}
	if len(out) == 0 {
		return citations
	}
	return out
}

func explanationCitationLookupKey(file string, line int) string {
	file = strings.TrimSpace(strings.ReplaceAll(file, `\`, `/`))
	if file == "" || line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func explanationCitationMatchesObservedAnchorLine(cit types.Citation, plan *types.AnswerSurfacePlan) bool {
	key := explanationCitationLookupKey(cit.File, cit.Line)
	if key == "" || plan == nil {
		return false
	}
	for _, anchor := range plan.LogObservedAnchors {
		file := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		if file == "" {
			continue
		}
		if anchor.ObservedLine > 0 && key == fmt.Sprintf("%s:%d", file, anchor.ObservedLine) {
			return true
		}
		if anchor.AnchoredLine > 0 && key == fmt.Sprintf("%s:%d", file, anchor.AnchoredLine) {
			return true
		}
	}
	return false
}

func explanationCitationMatchesRootCauseAnchorTokens(cit types.Citation, originalQuotes map[string]string, gc *ground.Context, plan *types.AnswerSurfacePlan) bool {
	tokens := explanationObservedAnchorTokens(plan)
	if len(tokens) == 0 {
		return false
	}
	if explanationCitationTextMentionsAnyToken(cit.Quote, tokens) {
		return true
	}
	if quote := originalQuotes[explanationCitationLookupKey(cit.File, cit.Line)]; explanationCitationTextMentionsAnyToken(quote, tokens) {
		return true
	}
	return explanationCitationWindowMentionsAnyToken(cit.File, cit.Line, tokens, gc)
}

func explanationCitationWithinObservedAnchorNeighborhood(cit types.Citation, plan *types.AnswerSurfacePlan) bool {
	file := strings.TrimSpace(strings.ReplaceAll(cit.File, `\`, `/`))
	if file == "" || cit.Line <= 0 || plan == nil {
		return false
	}
	for _, anchor := range plan.LogObservedAnchors {
		anchorFile := strings.TrimSpace(strings.ReplaceAll(anchor.File, `\`, `/`))
		if anchorFile != file || anchor.AnchoredLine <= 0 {
			continue
		}
		delta := cit.Line - anchor.AnchoredLine
		if delta < 0 {
			delta = -delta
		}
		if delta <= corroborationWindow+2 {
			return true
		}
	}
	return false
}

func explanationObservedAnchorTokens(plan *types.AnswerSurfacePlan) []string {
	if plan == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, anchor := range plan.LogObservedAnchors {
		tail := types.NormalizedSurfaceSymbolTail(anchor.Func)
		if tail == "" || seen[tail] {
			continue
		}
		seen[tail] = true
		out = append(out, tail)
	}
	return out
}

func explanationCitationTextMentionsAnyToken(text string, tokens []string) bool {
	if strings.TrimSpace(text) == "" || len(tokens) == 0 {
		return false
	}
	wanted := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		wanted[strings.ToLower(token)] = true
	}
	for _, tok := range valueLiteralTokenRe.FindAllString(text, -1) {
		if wanted[strings.ToLower(tok)] {
			return true
		}
	}
	return false
}

func explanationCitationWindowMentionsAnyToken(file string, line int, tokens []string, gc *ground.Context) bool {
	if gc == nil || len(gc.LineIndex) == 0 || line <= 0 || len(tokens) == 0 {
		return false
	}
	fileLines, ok := gc.LineIndex[file]
	if !ok || len(fileLines) == 0 {
		return false
	}
	wanted := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		wanted[strings.ToLower(token)] = true
	}
	for current := line - corroborationWindow; current <= line+corroborationWindow; current++ {
		if current <= 0 {
			continue
		}
		text, ok := fileLines[current]
		if !ok {
			continue
		}
		for _, tok := range valueLiteralTokenRe.FindAllString(text, -1) {
			if wanted[strings.ToLower(tok)] {
				return true
			}
		}
	}
	return false
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
	return newAnswerDocValidationError(
		"missing_diagram",
		"diagram required for this dispatch (preferred kinds: %s); summary must include at least %d grounded fenced diagram block(s). PREFERRED form: a ` ```mermaid ` fenced block using flowchart or sequenceDiagram. ASCII art is the fallback only when the Mermaid subset cannot express the shape. This obligation is independent of answer shape.",
		strings.Join(kinds, ", "), minimum,
	).WithFields("summary").WithHint("Re-emit `emit_answer_document` with the same answer shape and payload, but add at least one grounded ` ```mermaid ` flowchart block to `summary`. Reuse grounded labels only; do not reopen files or change unrelated fields.")
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
	if allow.empty() {
		return nil
	}

	var violations []string
	seen := make(map[string]bool)
	for _, block := range blocks {
		scanBody := diagramGroundingBodyReplacer.Replace(block[1])
		for _, tok := range diagramFileTokenRe.FindAllString(scanBody, -1) {
			bare := tok
			if idx := strings.LastIndex(bare, ":"); idx >= 0 {
				bare = bare[:idx]
			}
			ext := strings.ToLower(path.Ext(bare))
			if !diagramFileExtensions[ext] {
				continue
			}
			if allow.matches(bare) {
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
	allowed := allow.render(8)
	return newAnswerDocValidationError(
		"diagram_grounding",
		"summary fenced code block references file(s) not present in citations[] or attached-log frames: %s. "+
			"Diagrams (Mermaid blocks or ASCII art) are structural claims — every filename or path-like label named inside a triple-backtick block "+
			"must be grounded by a cited repo file, a cited line-text path literal, or an attached-log resolved frame. "+
			"Either add a citations[] entry for each named file (and reference it from steps / symbols / value as needed), "+
			"or remove the unsupported file name from the diagram and describe the relationship in prose. Allowed grounded labels for this dispatch: %s.",
		strings.Join(violations, ", "), allowed,
	).WithFields("summary").
		WithMetadata("invalid_labels", strings.Join(violations, ", ")).
		WithMetadata("allowed_labels", strings.Join(allow.labels, ", ")).
		WithHint("Re-emit `emit_answer_document` with the same grounded answer, but inside fenced diagrams keep file/path node labels to the exact grounded allowlist for this dispatch. If a node has no grounded label, remove it from the fence and explain that relationship in prose instead.")
}

// configTraceFenceRoleLabels returns the canonical 4-element role
// label set the config-trace fence validator accepts as channel-1
// grounded labels (the long-form, e.g. "operator override").
// Sourced via types.ConfigTraceDiagramRoleNodeLabel so adding a new
// EvidenceDiagramRole constant automatically extends the allowed
// label set without further wiring.
func configTraceFenceRoleLabels() map[string]bool {
	out := make(map[string]bool, 4)
	for _, role := range []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleOverride,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleRuntime,
		types.EvidenceDiagramRoleDefault,
	} {
		if lbl := strings.TrimSpace(types.ConfigTraceDiagramRoleNodeLabel(role)); lbl != "" {
			out[lbl] = true
		}
	}
	return out
}

// configTraceFenceRoleMarkers extends the bare role labels with the
// short-form role enum names (`override` / `config` / `runtime` /
// `default`). Used to validate the COMPOUND form
// `<content> (<role-marker>)` that 022f245's prompt allows as a
// "compiled role abstraction backed by grounded evidence". A label
// like `CLI flag (override)` is grounded because the parenthetical
// is one of the canonical role markers — the role marker carries
// the role binding, the content phrase is the human-readable label.
//
// Both long and short markers are accepted because the LLM tends
// to write either form interchangeably; sourcing the canonical
// strings from EvidenceDiagramRole keeps the registry data-driven.
func configTraceFenceRoleMarkers() map[string]bool {
	out := configTraceFenceRoleLabels()
	for _, role := range []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleOverride,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleRuntime,
		types.EvidenceDiagramRoleDefault,
	} {
		s := strings.ToLower(strings.TrimSpace(string(role)))
		if s != "" {
			out[s] = true
		}
	}
	return out
}

// roleMarkerSuffixRe captures a trailing `(role-marker)` parenthesis
// at the end of a label. Group 1 is the marker text. Whitespace
// between the content and the parenthesis is allowed but not
// trailing whitespace inside the parens (we Trim before matching).
var roleMarkerSuffixRe = regexp.MustCompile(`\s*\(([^()]+)\)\s*$`)

// labelHasParentheticalRoleMarker reports whether `label` ends with
// `... (<marker>)` where marker is a known role marker (long or
// short form). Returns true for the 022f245-style compound shape.
func labelHasParentheticalRoleMarker(label string, markers map[string]bool) bool {
	m := roleMarkerSuffixRe.FindStringSubmatch(strings.TrimSpace(label))
	if m == nil {
		return false
	}
	return markers[strings.ToLower(strings.TrimSpace(m[1]))]
}

// validateSummaryConfigTraceFenceLabels enforces the structural
// "two-channel grounding" rule for fenced ```mermaid``` blocks on
// config-trace questions:
//
//	S1 — the label is one of the four role-abstract labels emitted
//	     by ConfigTraceDiagramRoleNodeLabel ("operator override" /
//	     "config file" / "runtime binding" / "code default"). These
//	     labels carry their own anchor binding via the supporting-
//	     anchor block the prompt renders ABOVE the fence.
//	S2 — the label is path-shape (matches diagramFileTokenRe with a
//	     diagramFileExtensions extension) AND that path is in the
//	     existing summaryDiagramAllowlist (citations[] + per-cite
//	     evidence pool + log-triage ResolvedFiles).
//	S3 — anything else: reject, naming the offending labels.
//
// Scope gates (cheapest-first so non-config-trace turns pay zero
// cost):
//
//   - plan == nil OR plan.ConfigTraceDiagramAnchors empty → not a
//     config-trace question; skip.
//   - summary contains no fenced blocks → skip.
//   - per-block: info-string does NOT name `mermaid` → skip (this
//     validator deliberately does NOT inspect ```bash``` / ```json```
//     fences; their labels can mention `CLI` / `RPC` legitimately).
//
// Replaces the pre-fix prompt-side literal-CLI prohibition. The
// rule is now structural (any unknown label gets caught, not just
// the few words listed in a prompt example), so the prompt's
// example phrases (CLI / RPC / UI) become teaching aids rather
// than the actual decision dictionary.
func validateSummaryConfigTraceFenceLabels(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) error {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.ConfigTraceDiagramAnchors) == 0 {
		return nil
	}
	matches := fencedCodeBlockWithInfoRe.FindAllStringSubmatch(summary, -1)
	if len(matches) == 0 {
		return nil
	}
	roleLabels := configTraceFenceRoleLabels()
	roleMarkers := configTraceFenceRoleMarkers()
	allow := buildSummaryDiagramAllowlist(citations, gc, ctx)

	var violations []string
	seen := make(map[string]bool)
	for _, m := range matches {
		info := strings.TrimSpace(m[1])
		if !strings.EqualFold(info, "mermaid") &&
			!strings.HasPrefix(strings.ToLower(info), "mermaid ") {
			continue
		}
		body := diagramGroundingBodyReplacer.Replace(m[2])
		for _, label := range extractMermaidNodeLabels(body) {
			// S1a — exact role-abstract label.
			if roleLabels[label] {
				continue
			}
			// S1b — compound form `<content> (<role-marker>)`.
			// Allowed by 022f245's "compiled role abstraction
			// backed by grounded evidence" design: the role
			// marker is what binds the label to a precedence
			// tier, the content phrase is the human-readable
			// surface (typically the corresponding file or
			// concept already named in evidence). Marker set is
			// data-driven from the EvidenceDiagramRole enum.
			if labelHasParentheticalRoleMarker(label, roleMarkers) {
				continue
			}
			// S2 — label is already in citations[] /
			// per-cite evidence pool / log-triage ResolvedFiles.
			// We do NOT pre-gate by diagramFileExtensions because
			// the legitimate config-trace anchor set includes
			// non-code paths like `codrax.yaml.example` whose
			// extension is `.example`. allow.matches is itself
			// citation-grounded so a bare label like `CLI` cannot
			// accidentally satisfy it (no citation file is named
			// `CLI`).
			if allow.matches(label) {
				continue
			}
			if seen[label] {
				continue
			}
			seen[label] = true
			violations = append(violations, label)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	rolesList := make([]string, 0, len(roleLabels))
	for r := range roleLabels {
		rolesList = append(rolesList, r)
	}
	sort.Strings(rolesList)
	return newAnswerDocValidationError(
		"config_trace_fence_labels",
		"summary mermaid fence carries node label(s) that are neither a role-abstract label (`%s`), a `<content> (<role>)` compound form, nor a citation-grounded file path: %s. "+
			"For config-trace precedence diagrams, every node label must satisfy one of three structural channels: "+
			"(1) use a role label EXACTLY as listed in the supporting-anchor block above, "+
			"(2) write a compound `<content> (<role>)` label whose parenthetical is one of the canonical role markers (override / config / runtime / default, or their long forms), OR "+
			"(3) use a concrete file path / symbol that appears in citations[]. "+
			"Drop labels that do not fit any channel; describe their concept in prose outside the fence.",
		strings.Join(rolesList, "`, `"), strings.Join(violations, ", "),
	).WithFields("summary").
		WithMetadata("invalid_labels", strings.Join(violations, ", ")).
		WithMetadata("allowed_role_labels", strings.Join(rolesList, ", ")).
		WithHint("Re-emit emit_answer_document. Inside ```mermaid``` fences keep node labels to (a) the four role labels supplied above, (b) a compound `<content> (<role-marker>)` form where role-marker is override / config / runtime / default, OR (c) file/path tokens already present in citations[]. Supporting file:line anchors belong outside the fence: do not append `(<file>:<line>)`, `<br/>(<file>)`, or similar support suffixes to the node label unless that exact combined label is itself grounded. Anything else (one-word concepts like CLI/RPC/UI without a role marker, numbered tiers, architectural archetypes) belongs in prose, not as a node label.")
}

type summaryDiagramAllowlist struct {
	exact        map[string]bool
	short        map[string]bool
	labels       []string
	baseOwners   map[string]string
	baseConflict map[string]bool
}

func (a summaryDiagramAllowlist) empty() bool {
	return len(a.exact) == 0
}

func (a summaryDiagramAllowlist) matches(label string) bool {
	label = strings.ReplaceAll(strings.TrimSpace(label), `\`, `/`)
	if label == "" {
		return false
	}
	if a.exact[label] {
		return true
	}
	return a.short[path.Base(label)]
}

func (a summaryDiagramAllowlist) render(limit int) string {
	if len(a.labels) == 0 {
		return "(none)"
	}
	labels := a.labels
	if limit > 0 && len(labels) > limit {
		labels = labels[:limit]
	}
	return backtickJoin(labels)
}

func buildSummaryDiagramAllowlist(citations []types.Citation, gc *ground.Context, ctx *types.BusContext) summaryDiagramAllowlist {
	allow := summaryDiagramAllowlist{
		exact:        make(map[string]bool, len(citations)*4),
		short:        make(map[string]bool, len(citations)*2),
		baseOwners:   make(map[string]string, len(citations)*2),
		baseConflict: make(map[string]bool, len(citations)),
	}
	var evidencePool []types.EvidenceItem
	if ctx != nil {
		evidencePool = answerDocSurfaceEvidencePool(ctx)
	}
	for _, c := range citations {
		addDiagramAllowToken(&allow, c.File)
		if len(evidencePool) > 0 {
			addDiagramAllowFromMatchedEvidence(&allow, matchingEvidenceForCitation(evidencePool, c))
		}
	}
	if gc != nil && len(gc.LineIndex) > 0 {
		for _, c := range citations {
			addDiagramAllowFromCitationWindow(&allow, c, gc)
		}
	}
	if ctx != nil && ctx.Mutable != nil {
		if bundle := ctx.Mutable.LogTriage(); bundle != nil {
			for _, f := range bundle.ResolvedFiles {
				addDiagramAllowToken(&allow, f)
			}
		}
	}
	for base, owner := range allow.baseOwners {
		if base == "" || allow.baseConflict[base] {
			continue
		}
		if owner == base {
			continue
		}
		allow.short[base] = true
	}
	sort.Strings(allow.labels)
	return allow
}

func addDiagramAllowFromCitationWindow(allow *summaryDiagramAllowlist, c types.Citation, gc *ground.Context) {
	if allow == nil || gc == nil {
		return
	}
	file := strings.ReplaceAll(strings.TrimSpace(c.File), `\`, `/`)
	if file == "" || c.Line <= 0 {
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

func addDiagramAllowToken(allow *summaryDiagramAllowlist, token string) {
	if allow == nil {
		return
	}
	token = strings.ReplaceAll(strings.TrimSpace(token), `\`, `/`)
	if token == "" {
		return
	}
	if !allow.exact[token] {
		allow.exact[token] = true
		allow.labels = append(allow.labels, token)
	}
	if base := path.Base(token); base != "" && base != "." {
		if owner, ok := allow.baseOwners[base]; !ok {
			allow.baseOwners[base] = token
		} else if owner != token {
			allow.baseConflict[base] = true
		}
	}
}

func addDiagramAllowFromMatchedEvidence(allow *summaryDiagramAllowlist, item types.EvidenceItem) {
	if allow == nil || item.Source == "" {
		return
	}
	switch item.GroundingStatus {
	case types.GroundingGrounded, types.GroundingRecovered:
	default:
		return
	}
	addDiagramAllowPathLiterals(allow, item.AnchorSymbol)
	addDiagramAllowPathLiterals(allow, item.Subject)
	addDiagramAllowPathLiterals(allow, item.Object)
	addDiagramAllowPathLiterals(allow, item.Summary)
	addDiagramAllowPathLiterals(allow, item.Condition)
	addDiagramAllowPathLiterals(allow, item.Snippet)
}

func addDiagramAllowPathLiterals(allow *summaryDiagramAllowlist, text string) {
	if allow == nil {
		return
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
	errorTypes := types.LogBundleErrorTypes(bundle)
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
	return newAnswerDocValidationError(
		"log_triage_coverage",
		"summary does not acknowledge the attached log's error type(s): %s. "+
			"The log's structured Errors tree identifies these types (walk the top-level Errors and their Cause chain in the Log Triage section); "+
			"the answer must name each one by its class / exception identifier at least once so the reader can connect the log to your explanation. "+
			"Quote the Type literally from the structured extraction — do NOT paraphrase class names away or invent alternative stack traces. "+
			"If the question resolves to just one layer of the chain (e.g. 'what does this panic mean?' when the chain is trivial), still include the single Type's identifier. "+
			"Case-insensitive match; a single mention of the class / exception name anywhere in summary is sufficient coverage.",
		strings.Join(missing, ", ")).
		WithFields("summary").
		WithMetadata("missing_types", strings.Join(missing, ", ")).
		WithHint("Re-emit `emit_answer_document` and name each attached-log error type directly in `summary` at least once. Keep the same grounded explanation, but do not paraphrase the class/exception names away.")
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
	return newAnswerDocValidationError(
		"diagram_codename",
		"summary introduces codename label(s) not present in any citation's ±%d-line window: %s. "+
			"Short enumeration labels (S1/S2/Phase 0/Fallback X) are source-level identifiers — the exact token MUST "+
			"appear in the cited line range. Either cite the line where each label is defined, or remove the label "+
			"and describe the mechanism by its real behavior; do NOT extrapolate from an existing label by pattern "+
			"(e.g. 'S1 exists so S2 must too' is not admissible).",
		corroborationWindow, strings.Join(ungrounded, ", ")).
		WithFields("summary", "citations").
		WithMetadata("invalid_labels", strings.Join(ungrounded, ", ")).
		WithMetadata("allowed_citations", formatCitationLocations(citations)).
		WithHint("Re-emit `emit_answer_document` with the same grounded answer, but remove invented numbered / phase-style labels unless those exact tokens are cited. Use grounded files, symbols, config keys, or other evidenced entities as node labels instead.")
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
			return newAnswerDocValidationError(
				"exact_resolution",
				"exact-resolution contract violated: summary must explicitly name the requested exact %s %s. Resolve the exact target before discussing nearby context.",
				label, exactTargetListForError(missing))
		}
	}
	justification := strings.TrimSpace(ctx.Mutable.StableAbsenceJustification())
	pending := types.ExactResolutionPendingTargets(contract, unverifiedFindingsForCompletion(ctx))
	if !contract.AllowAbsence || justification == "" || len(pending) == 0 {
		return nil
	}
	if !looksLikeHonestZeroClaim(summary, "") {
		return newAnswerDocValidationError(
			"exact_resolution",
			"exact-resolution contract violated: summary names the requested exact %s but does not clearly state it is absent / not found. Lead with the exact absence before any nearby context. Absence-only is acceptable.",
			label)
	}
	if contract.AliasRequiresProof && containsPositiveAliasClaim(summary) {
		return newAnswerDocValidationError(
			"exact_resolution",
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

func backtickJoin(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, "`"+item+"`")
	}
	return strings.Join(out, ", ")
}

func resolveAnswerDocumentExactResolution(summary string, declared *types.AnswerExactResolution, ctx *types.BusContext) (*types.AnswerExactResolution, string, error) {
	summary = strings.TrimSpace(summary)
	contract := answerExactResolutionContract(ctx)
	if contract == nil || len(contract.Targets) == 0 {
		if declared != nil {
			return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution is only valid when the dispatch includes an Exact Resolution Contract").
				WithFields("exact_resolution").
				WithHint("Re-emit `emit_answer_document` without an `exact_resolution` object for this dispatch. Keep the grounded answer shape, citations, and payload fields unchanged unless another validator asked for a different fix.")
		}
		return nil, summary, nil
	}
	if declared == nil {
		return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution{status,...} is required for this dispatch").
			WithFields("exact_resolution").
			WithHint("Re-emit `emit_answer_document` with `exact_resolution{status, anchor?, context_mode}` filled in. Use `exact_match` only with grounded proof that explicitly names the exact target, `alias_match` only with explicit grounded mapping proof plus `anchor`, and `absent` only when the upstream investigation has already closed as absence.")
	}

	resolved := &types.AnswerExactResolution{
		Status:      normalizeAnswerExactResolutionStatus(declared.Status),
		Anchor:      strings.TrimSpace(declared.Anchor),
		ContextMode: normalizeAnswerExactResolutionContextMode(declared.ContextMode),
	}
	if resolved.Status == "" {
		return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status must be one of exact_match / alias_match / absent").
			WithFields("exact_resolution.status").
			WithHint("Re-emit `emit_answer_document` with `exact_resolution.status` set to exactly one of `exact_match`, `alias_match`, or `absent`. Pick the status from the already-grounded proof; do not reopen files or invent a substitute status.")
	}
	if declared.ContextMode != "" && resolved.ContextMode == "" {
		return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.context_mode must be one of none / grounded_context_only").
			WithFields("exact_resolution.context_mode").
			WithHint("Re-emit `emit_answer_document` with `exact_resolution.context_mode` set to either `none` or `grounded_context_only`. Use `grounded_context_only` when the summary keeps nearby grounded context beyond the exact-target proof itself.")
	}
	if resolved.ContextMode == "" {
		resolved.ContextMode = types.AnswerExactResolutionContextNone
	}
	plan := answerSurfacePlan(ctx)
	if resolved.Status == types.AnswerExactResolutionAbsent {
		summary = trimLeadingExactAbsenceRestatement(summary, contract, plan)
		summary = sanitizeExactContextSummarySurface(summary, contract, plan)
		summary = normalizeFollowOnGroundedContextSummarySurface(summary, contract, plan, ctx)
		summary = normalizeConfigTraceAbsentSummarySurface(summary, nil, nil, ctx, resolved)
	}
	if declared.ContextMode == "" &&
		resolved.Status == types.AnswerExactResolutionAbsent &&
		resolved.ContextMode == types.AnswerExactResolutionContextNone &&
		strings.TrimSpace(summary) != "" {
		resolved.ContextMode = types.AnswerExactResolutionContextGroundedOnly
	}
	if resolved.Status == types.AnswerExactResolutionAbsent &&
		strings.TrimSpace(summary) != "" &&
		plan != nil &&
		plan.PreferredExactResolution != nil &&
		plan.PreferredExactResolution.Status == types.AnswerExactResolutionAbsent &&
		plan.PreferredExactResolution.ContextMode == types.AnswerExactResolutionContextGroundedOnly &&
		resolved.ContextMode == types.AnswerExactResolutionContextNone {
		resolved.ContextMode = types.AnswerExactResolutionContextGroundedOnly
	}

	switch resolved.Status {
	case types.AnswerExactResolutionAliasMatch:
		if resolved.Anchor == "" {
			return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=alias_match requires exact_resolution.anchor naming the grounded alias / mapping anchor").
				WithFields("exact_resolution.status", "exact_resolution.anchor").
				WithHint("Re-emit `emit_answer_document` with `exact_resolution.anchor` naming the grounded alias / mapping anchor, or stop using `alias_match` if no explicit grounded mapping proof exists.")
		}
	case types.AnswerExactResolutionAbsent:
		if ctx == nil || ctx.Mutable == nil {
			return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: status=absent requires mutable exact-resolution state").
				WithFields("exact_resolution.status").
				WithHint("Re-emit `emit_answer_document` with an `exact_resolution.status` supported by the current grounded state. Use `absent` only when the upstream investigation has already closed with an exact-absence result.")
		}
		if !contract.AllowAbsence || ctx.Mutable.StableInvestigationResultKind() != "absence" || strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) == "" {
			return nil, "", newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: status=absent requires the exploration stage to close with emit_investigation_complete(result_kind=\"absence\", absence_justification=...)").
				WithFields("exact_resolution.status").
				WithHint("Re-emit `emit_answer_document` with `status=absent` only if the upstream investigation has already closed as absence. Otherwise choose `exact_match` or `alias_match` only when the existing grounded proof supports it; do not force `absent` from summary wording alone.")
		}
	}
	if ctx != nil && ctx.Mutable != nil &&
		ctx.Mutable.StableInvestigationResultKind() == "absence" &&
		strings.TrimSpace(ctx.Mutable.StableAbsenceJustification()) != "" &&
		resolved.Status != types.AnswerExactResolutionAbsent {
		err := newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: the upstream investigation already closed as absence, so finalizer output must keep exact_resolution.status=\"absent\" unless the investigation is reopened with new grounded proof").
			WithFields("exact_resolution.status", "exact_resolution.context_mode").
			WithMetadata("locked_status", string(types.AnswerExactResolutionAbsent)).
			WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly))
		err = err.WithHint("Re-emit `emit_answer_document` with `exact_resolution.status=\"absent\"`. If you keep any nearby grounded context, set `exact_resolution.context_mode=\"grounded_context_only\"`. Do not switch to `exact_match` or `alias_match` unless a newly cited grounded anchor explicitly proves the exact target or an explicit alias mapping.")
		return nil, "", err
	}
	lead := renderAnswerDocumentExactResolutionLeadClean(contract, resolved, requestedAnswerDocumentLanguage(ctx))
	return resolved, joinAnswerDocumentLead(lead, summary), nil
}

func validateAnswerDocumentExactResolutionProof(exact *types.AnswerExactResolution, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) error {
	if exact == nil {
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
	proof := collectExactResolutionProof(contract, citations, gc, ctx)

	switch exact.Status {
	case types.AnswerExactResolutionAbsent:
		if proof.AnyNonPrimaryCitationContext && exact.ContextMode != types.AnswerExactResolutionContextGroundedOnly {
			return newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=absent cites nearby grounded context beyond the primary absence-proof sources, so exact_resolution.context_mode must be \"grounded_context_only\"").
				WithFields("exact_resolution.status", "exact_resolution.context_mode", "citations", "summary").
				WithMetadata("preferred_context_mode", string(types.AnswerExactResolutionContextGroundedOnly)).
				WithHint("Re-emit `emit_answer_document` with the same `status=\"absent\"`, but set `exact_resolution.context_mode=\"grounded_context_only\"` because the answer surface already includes nearby grounded context beyond the primary absence-proof sources.")
		}
		if proof.TargetMentionContradictsAbsence {
			return newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=absent contradicts the grounded evidence/citations, which still name the requested exact %s %s", label, exactTargetListForError(contract.Targets)).
				WithFields("exact_resolution.status", "citations", "summary").
				WithMetadata("preferred_status", string(types.AnswerExactResolutionExactMatch)).
				WithHint("Re-emit `emit_answer_document` with `status=\"absent\"` only if you remove the contradictory exact-target proof from the answer surface. If the existing grounded proof truly defines the exact target, switch to `exact_match` instead of keeping `absent`.")
		}
	case types.AnswerExactResolutionExactMatch:
		if !proof.AnyDefiningTargetProof {
			if proof.RequiresProductionProof && proof.AnyTarget && !proof.AnyProductionTargetAnchor {
				return newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=exact_match for the requested exact %s %s requires production-grounded defining proof, not only nearby context, negative probes, test/spec/example mentions, or summary-level target text", label, exactTargetListForError(contract.Targets)).
					WithFields("exact_resolution.status", "citations", "summary").
					WithHint("Re-emit `emit_answer_document` with `status=\"exact_match\"` only when the exact target is grounded by a production-defining anchor or cited line. Nearby related context, negative absence-support items, and summary/background mentions of the target do not justify `exact_match`. Otherwise keep the exact target absent or contextual instead of upgrading it.")
			}
			return newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=exact_match requires at least one grounded defining evidence item or cited line that explicitly names the requested exact %s %s", label, exactTargetListForError(contract.Targets)).
				WithFields("exact_resolution.status", "citations", "summary").
				WithHint("Re-emit `emit_answer_document` with `status=\"exact_match\"` only when a grounded evidence item's defining anchor fields or a cited line explicitly define the requested exact target. Free-form summary/background prose and nearby context do not count as exact proof. Otherwise use `absent` only if the investigation closed as absence, or `alias_match` only with explicit grounded mapping proof.")
		}
	case types.AnswerExactResolutionAliasMatch:
		if !proof.anyPair(exact.Anchor) {
			return newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=alias_match requires explicit grounded proof that mentions both the requested exact %s %s and the claimed anchor `%s` together", label, exactTargetListForError(contract.Targets), exact.Anchor).
				WithFields("exact_resolution.status", "exact_resolution.anchor", "citations", "summary").
				WithHint("Re-emit `emit_answer_document` with `status=\"alias_match\"` only when one grounded proof item's structured anchor fields (`subject` / `object` / `anchor_symbol`) or one cited line mentions both the requested exact target and the claimed alias / mapping anchor together. Nearby-context explanation in `summary` does not count as alias proof. Otherwise keep `anchor` empty and use the status the current grounded proof actually supports.")
		}
		if proof.RequiresProductionProof && !proof.anyProductionPair(exact.Anchor) {
			return newAnswerDocValidationError("exact_resolution", "exact-resolution contract violated: exact_resolution.status=alias_match for the requested exact %s %s requires production-grounded proof for anchor `%s`, not only test/spec/example mentions", label, exactTargetListForError(contract.Targets), exact.Anchor).
				WithFields("exact_resolution.status", "exact_resolution.anchor", "citations", "summary").
				WithHint("Re-emit `emit_answer_document` with `status=\"alias_match\"` only when the alias / mapping proof is grounded in production code/config. Test/spec/example/documentation mentions of the alias are not enough on their own, and summary prose alone cannot supply the missing mapping proof.")
		}
	}
	return nil
}

func validateAbsentExactConfigValueShape(shape types.AnswerShape, exact *types.AnswerExactResolution, ctx *types.BusContext) error {
	if shape != types.ShapeConfigValue || exact == nil || ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	if exact.Status != types.AnswerExactResolutionAbsent {
		return nil
	}
	if !types.ExactResolutionTargetIsConfigKey(ctx.AnalysisIR.AnswerContract.ExactResolution) {
		return nil
	}
	msg := "exact absent config-key answers must not use shape=config_value with a synthetic missing literal; use shape=explanation so the answer can lead with the exact absence"
	if ctx.AnalysisIR.RequestModel.Scenario == types.ScenarioConfigTrace {
		msg += " and keep any grounded same-family precedence chain in prose"
	}
	return newAnswerDocValidationError("absent_exact_config_value_shape", "%s", msg).
		WithFields("shape", "exact_resolution", "summary").
		WithHint("Re-emit `emit_answer_document` with `shape=explanation`, keep `exact_resolution.status=\"absent\"`, and present any grounded same-family context as explanation only rather than a synthetic missing scalar.")
}

func normalizeAnswerExactResolutionStatus(status types.AnswerExactResolutionStatus) types.AnswerExactResolutionStatus {
	normalized := types.AnswerExactResolutionStatus(strings.ToLower(strings.TrimSpace(string(status))))
	switch normalized {
	case types.AnswerExactResolutionExactMatch, types.AnswerExactResolutionAliasMatch, types.AnswerExactResolutionAbsent:
		return normalized
	default:
		return ""
	}
}

func normalizeAnswerExactResolutionContextMode(mode types.AnswerExactResolutionContextMode) types.AnswerExactResolutionContextMode {
	normalized := types.AnswerExactResolutionContextMode(strings.ToLower(strings.TrimSpace(string(mode))))
	switch normalized {
	case types.AnswerExactResolutionContextNone, types.AnswerExactResolutionContextGroundedOnly:
		return normalized
	default:
		return ""
	}
}

func joinAnswerDocumentLead(lead, summary string) string {
	lead = strings.TrimSpace(lead)
	summary = strings.TrimSpace(summary)
	if lead == "" {
		return summary
	}
	if summary == "" {
		return lead
	}
	return lead + "\n\n" + summary
}

func trimLeadingExactAbsenceRestatement(summary string, contract *types.ExactResolutionContract, plan *types.AnswerSurfacePlan) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || contract == nil || len(contract.Targets) == 0 {
		return summary
	}
	heading, paragraph, rest := splitLeadingSummaryParagraph(summary)
	if paragraph == "" || repeatedExactTargetAfterLead(contract, paragraph) == "" {
		return summary
	}
	rest = strings.TrimSpace(rest)
	trimmedParagraph := trimLeadingExactTargetSentences(paragraph, contract)
	if trimmedParagraph != "" && trimmedParagraph != paragraph {
		return joinLeadingSummaryParts(heading, trimmedParagraph, rest)
	}
	if rest != "" {
		if trimmedParagraph == "" {
			return joinLeadingSummaryParts("", "", rest)
		}
		return rest
	}
	candidates := []types.ExactContextSurfaceLabel(nil)
	if plan != nil && len(plan.CitationGradeExactContextLabels) > 0 {
		candidates = plan.CitationGradeExactContextLabels
	} else if plan != nil {
		candidates = plan.AllowedExactContextLabels
	}
	if len(candidates) > 0 {
		if len(mentionedExactContextSurfaceCandidates(paragraph, candidates)) > 0 {
			return summary
		}
	}
	return ""
}

func sanitizeExactContextSummarySurface(summary string, contract *types.ExactResolutionContract, plan *types.AnswerSurfacePlan) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || contract == nil || plan == nil || len(plan.ForbiddenExactContextLabels) == 0 {
		return summary
	}
	paragraphs := splitSummaryParagraphs(summary)
	if len(paragraphs) == 0 {
		return summary
	}
	changed := false
	keptParagraphs := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "```") {
			keptParagraphs = append(keptParagraphs, trimmed)
			continue
		}
		sentences := splitSummarySentences(trimmed)
		if len(sentences) == 0 {
			keptParagraphs = append(keptParagraphs, trimmed)
			continue
		}
		keptSentences := make([]string, 0, len(sentences))
		for _, sentence := range sentences {
			if repeatedExactTargetAfterLead(contract, sentence) != "" {
				changed = true
				continue
			}
			mentionedAllowed := mentionedExactContextSurfaceLabelHits(sentence, plan.AllowedExactContextLabels)
			mentionedForbidden := filterShadowedForbiddenExactContextSurfaceLabels(
				mentionedAllowed,
				mentionedExactContextSurfaceLabelHits(sentence, plan.ForbiddenExactContextLabels),
			)
			if len(mentionedForbidden) > 0 {
				changed = true
				continue
			}
			keptSentences = append(keptSentences, sentence)
		}
		if len(keptSentences) == 0 {
			changed = true
			continue
		}
		rebuilt := strings.TrimSpace(strings.Join(keptSentences, " "))
		if rebuilt != trimmed {
			changed = true
		}
		keptParagraphs = append(keptParagraphs, rebuilt)
	}
	if !changed {
		return summary
	}
	rebuilt := strings.TrimSpace(strings.Join(keptParagraphs, "\n\n"))
	if rebuilt == "" {
		return summary
	}
	return rebuilt
}

func normalizeFollowOnGroundedContextSummarySurface(summary string, contract *types.ExactResolutionContract, plan *types.AnswerSurfacePlan, ctx *types.BusContext) string {
	summary = strings.TrimSpace(summary)
	if contract == nil || plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceFollowOnGroundedContext {
		return summary
	}
	if summary == "" {
		if fallback := renderFollowOnGroundedContextSummarySeed(plan, requestedAnswerDocumentLanguage(ctx)); fallback != "" {
			return fallback
		}
		return summary
	}
	lead := renderAnswerDocumentExactResolutionLeadClean(contract, &types.AnswerExactResolution{
		Status:      types.AnswerExactResolutionAbsent,
		ContextMode: types.AnswerExactResolutionContextGroundedOnly,
	}, requestedAnswerDocumentLanguage(ctx))
	summary = trimFollowOnGroundedContextRestatements(summary, contract, plan)
	body := exactContextSummaryBodyAfterLead(summary, lead)
	if followOnGroundedContextBodyMentionsAnchors(body, plan) {
		return summary
	}
	if repeatedExactTargetAfterLead(contract, body) != "" {
		if fallback := renderFollowOnGroundedContextSummarySeed(plan, requestedAnswerDocumentLanguage(ctx)); fallback != "" {
			return fallback
		}
		return summary
	}
	if fallback := renderFollowOnGroundedContextSummarySeed(plan, requestedAnswerDocumentLanguage(ctx)); fallback != "" {
		return fallback
	}
	return summary
}

func trimFollowOnGroundedContextRestatements(summary string, contract *types.ExactResolutionContract, plan *types.AnswerSurfacePlan) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || contract == nil || plan == nil {
		return summary
	}
	paragraphs := splitSummaryParagraphs(summary)
	if len(paragraphs) == 0 {
		return summary
	}
	var rebuilt []string
	var pendingHeadings []string
	changed := false
	for _, paragraph := range paragraphs {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			rebuilt = append(rebuilt, pendingHeadings...)
			pendingHeadings = nil
			rebuilt = append(rebuilt, trimmed)
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			pendingHeadings = append(pendingHeadings, trimmed)
			continue
		}
		trimmedParagraph, keep := trimFollowOnGroundedContextParagraph(trimmed, contract)
		if !keep {
			if len(pendingHeadings) > 0 {
				changed = true
				pendingHeadings = nil
			}
			changed = true
			continue
		}
		if trimmedParagraph != trimmed {
			changed = true
		}
		rebuilt = append(rebuilt, pendingHeadings...)
		pendingHeadings = nil
		rebuilt = append(rebuilt, trimmedParagraph)
	}
	if !changed {
		return summary
	}
	joined := strings.TrimSpace(strings.Join(rebuilt, "\n\n"))
	if joined == "" {
		return ""
	}
	return joined
}

func trimFollowOnGroundedContextParagraph(paragraph string, contract *types.ExactResolutionContract) (string, bool) {
	if paragraph == "" || contract == nil {
		return paragraph, paragraph != ""
	}
	sentences := splitSummarySentences(paragraph)
	if len(sentences) == 0 {
		if repeatedExactTargetAfterLead(contract, paragraph) != "" {
			return "", false
		}
		return paragraph, true
	}
	kept := make([]string, 0, len(sentences))
	changed := false
	for _, sentence := range sentences {
		if repeatedExactTargetAfterLead(contract, sentence) != "" {
			changed = true
			continue
		}
		kept = append(kept, sentence)
	}
	if !changed {
		return paragraph, true
	}
	if len(kept) == 0 {
		return "", false
	}
	return strings.TrimSpace(strings.Join(kept, " ")), true
}

func followOnGroundedContextBodyMentionsAnchors(body string, plan *types.AnswerSurfacePlan) bool {
	if plan == nil {
		return false
	}
	if len(mentionedExactContextSurfaceLabelHits(body, plan.AllowedExactContextLabels)) > 0 {
		return true
	}
	items := mergedFollowOnGroundedContextItems(plan)
	if len(items) == 0 && len(plan.RelatedContextCitationCandidates) == 0 {
		return false
	}
	lower := strings.ToLower(body)
	for _, item := range items {
		if summaryMentionsFollowOnGroundedItem(body, lower, item) {
			return true
		}
	}
	for _, candidate := range plan.RelatedContextCitationCandidates {
		source := strings.TrimSpace(strings.ReplaceAll(candidate.Source, `\`, `/`))
		if source == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(source)) {
			return true
		}
		if candidate.Line > 0 && strings.Contains(body, fmt.Sprintf("%s:%d", source, candidate.Line)) {
			return true
		}
	}
	return false
}

func summaryMentionsFollowOnGroundedItem(body, lower string, item types.EvidenceItem) bool {
	if symbol := strings.TrimSpace(item.AnchorSymbol); symbol != "" && containsSurfaceToken(lower, strings.ToLower(symbol)) {
		return true
	}
	source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
	if source == "" {
		return false
	}
	if strings.Contains(lower, strings.ToLower(source)) {
		return true
	}
	if item.LineStart > 0 && strings.Contains(body, fmt.Sprintf("%s:%d", source, item.LineStart)) {
		return true
	}
	return false
}

func mergedFollowOnGroundedContextItems(plan *types.AnswerSurfacePlan) []types.EvidenceItem {
	if plan == nil {
		return nil
	}
	var out []types.EvidenceItem
	seen := make(map[string]bool)
	appendUnique := func(items []types.EvidenceItem) {
		for _, item := range items {
			key := fmt.Sprintf("%s:%d:%s:%s", strings.TrimSpace(item.Source), item.LineStart, strings.TrimSpace(item.AnchorSymbol), strings.TrimSpace(item.Summary))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, item)
		}
	}
	appendUnique(plan.ProseOnlyExactContextItems)
	appendUnique(plan.CitationGradeExactContextItems)
	appendUnique(plan.AllowedExactContextItems)
	return out
}

func renderFollowOnGroundedContextSummarySeed(plan *types.AnswerSurfacePlan, lang string) string {
	if plan == nil {
		return ""
	}
	if fallback := renderConfigTraceFollowOnGroundedContextSummarySeed(plan, lang); fallback != "" {
		return fallback
	}
	var labels []string
	seen := make(map[string]bool)
	appendLabels := func(items []types.ExactContextSurfaceLabel) {
		for _, item := range items {
			label := strings.TrimSpace(item.Display)
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			labels = append(labels, label)
			if len(labels) >= 2 {
				return
			}
		}
	}
	appendItemLabels := func(items []types.EvidenceItem) {
		if len(items) == 0 {
			return
		}
		sorted := append([]types.EvidenceItem(nil), items...)
		sort.SliceStable(sorted, func(i, j int) bool {
			si := followOnGroundedContextSeedScore(plan, sorted[i])
			sj := followOnGroundedContextSeedScore(plan, sorted[j])
			if si != sj {
				return si > sj
			}
			leni := len(strings.TrimSpace(sorted[i].AnchorSymbol))
			lenj := len(strings.TrimSpace(sorted[j].AnchorSymbol))
			if leni != lenj {
				return leni > lenj
			}
			return strings.TrimSpace(sorted[i].Source) < strings.TrimSpace(sorted[j].Source)
		})
		for _, kind := range []string{"symbol", "path"} {
			for _, item := range sorted {
				for _, label := range types.ExactContextSurfaceLabelsForItem(plan.ExactResolution, item) {
					if label.Kind != kind {
						continue
					}
					display := strings.TrimSpace(label.Display)
					if display == "" || seen[display] {
						continue
					}
					seen[display] = true
					labels = append(labels, display)
					if len(labels) >= 2 {
						return
					}
				}
			}
		}
	}
	// Summary fallback seeds are prose-oriented recovery aids, not
	// citation selectors. Pick the strongest nearby same-scope anchors
	// from the unified allowed pool, then fall back to the flattened
	// label set only when the item-backed pool is empty.
	appendItemLabels(mergedFollowOnGroundedContextItems(plan))
	if len(labels) < 2 {
		appendLabels(plan.AllowedExactContextLabels)
	}
	if len(labels) == 0 {
		return ""
	}
	return renderNearbyContextSummarySentence(labels, lang)
	if answerDocumentRequiresChinese(lang) {
		if len(labels) == 1 {
			return fmt.Sprintf("相关的已锚定上下文是 `%s`。", labels[0])
		}
		return fmt.Sprintf("相关的已锚定上下文包括 `%s` 和 `%s`。", labels[0], labels[1])
	}
	if len(labels) == 1 {
		return fmt.Sprintf("The grounded nearby context is `%s`.", labels[0])
	}
	return fmt.Sprintf("The grounded nearby context includes `%s` and `%s`.", labels[0], labels[1])
}

func renderConfigTraceFollowOnGroundedContextSummarySeed(plan *types.AnswerSurfacePlan, lang string) string {
	if plan == nil || plan.ExactResolution == nil || plan.ExactResolution.TargetKind != types.SubjectConfigKey {
		return ""
	}
	if len(plan.ConfigTraceDiagramAnchors) == 0 {
		return ""
	}
	labels := configTraceRoleSeedLabels(plan)
	if len(labels) == 0 {
		return ""
	}
	return renderNearbyContextSummarySentence(labels, lang)
}

func renderNearbyContextSummarySentence(labels []string, lang string) string {
	if len(labels) == 0 {
		return ""
	}
	if answerDocumentRequiresChinese(lang) {
		switch len(labels) {
		case 1:
			return fmt.Sprintf("相关的已锚定上下文是 `%s`。", labels[0])
		case 2:
			return fmt.Sprintf("相关的已锚定上下文包括 `%s` 和 `%s`。", labels[0], labels[1])
		default:
			head := make([]string, 0, len(labels)-1)
			for _, label := range labels[:len(labels)-1] {
				head = append(head, fmt.Sprintf("`%s`", label))
			}
			return fmt.Sprintf("相关的已锚定上下文包括 %s，以及 `%s`。", strings.Join(head, "、"), labels[len(labels)-1])
		}
	}
	switch len(labels) {
	case 1:
		return fmt.Sprintf("The grounded nearby context is `%s`.", labels[0])
	case 2:
		return fmt.Sprintf("The grounded nearby context includes `%s` and `%s`.", labels[0], labels[1])
	default:
		head := make([]string, 0, len(labels)-1)
		for _, label := range labels[:len(labels)-1] {
			head = append(head, fmt.Sprintf("`%s`", label))
		}
		return fmt.Sprintf("The grounded nearby context includes %s, and `%s`.", strings.Join(head, ", "), labels[len(labels)-1])
	}
}

func configTraceRoleSeedLabels(plan *types.AnswerSurfacePlan) []string {
	if plan == nil || plan.ExactResolution == nil {
		return nil
	}
	mergedItems := func(plan *types.AnswerSurfacePlan) []types.EvidenceItem {
		var out []types.EvidenceItem
		seenItems := make(map[string]bool)
		appendUnique := func(items []types.EvidenceItem) {
			for _, item := range items {
				key := fmt.Sprintf("%s:%d:%s:%s", strings.TrimSpace(item.Source), item.LineStart, strings.TrimSpace(item.AnchorSymbol), strings.TrimSpace(item.Summary))
				if key == "" || seenItems[key] {
					continue
				}
				seenItems[key] = true
				out = append(out, item)
			}
		}
		appendUnique(plan.ProseOnlyExactContextItems)
		appendUnique(plan.CitationGradeExactContextItems)
		appendUnique(plan.AllowedExactContextItems)
		return out
	}
	bestByRole := make(map[types.EvidenceDiagramRole]types.EvidenceItem)
	bestScore := make(map[types.EvidenceDiagramRole]int)
	softByRole := make(map[types.EvidenceDiagramRole]types.EvidenceItem)
	softScore := make(map[types.EvidenceDiagramRole]int)
	appendRoleItems := func(items []types.EvidenceItem) {
		for _, item := range items {
			role := types.ConfigTraceValidatedDiagramRoleInFiles(plan.ExactResolution, item, plan.ExactContextRequiredFiles)
			score := followOnGroundedContextSeedScore(plan, item)
			if role != types.EvidenceDiagramRoleUnknown {
				if cur, ok := bestScore[role]; ok && cur >= score {
					continue
				}
				bestScore[role] = score
				bestByRole[role] = item
			}
			softRole := configTraceFollowOnSeedRole(plan, item)
			if softRole == types.EvidenceDiagramRoleUnknown {
				continue
			}
			if cur, ok := softScore[softRole]; ok && cur >= score {
				continue
			}
			softScore[softRole] = score
			softByRole[softRole] = item
		}
	}
	appendRoleItems(plan.CitationGradeExactContextItems)
	appendRoleItems(plan.ProseOnlyExactContextItems)
	appendRoleItems(plan.AllowedExactContextItems)

	roleOrder := []types.EvidenceDiagramRole{
		types.EvidenceDiagramRoleDefault,
		types.EvidenceDiagramRoleConfig,
		types.EvidenceDiagramRoleRuntime,
		types.EvidenceDiagramRoleOverride,
	}
	var labels []string
	seen := make(map[string]bool)
	for _, role := range roleOrder {
		item, ok := bestByRole[role]
		if !ok {
			item, ok = softByRole[role]
		}
		if !ok {
			continue
		}
		label := configTraceRoleSeedLabel(role, item)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	if len(labels) >= 2 {
		return labels
	}
	supplemental := append([]types.EvidenceItem(nil), mergedItems(plan)...)
	sort.SliceStable(supplemental, func(i, j int) bool {
		si := followOnGroundedContextSeedScore(plan, supplemental[i])
		sj := followOnGroundedContextSeedScore(plan, supplemental[j])
		if si != sj {
			return si > sj
		}
		leni := len(strings.TrimSpace(supplemental[i].AnchorSymbol))
		lenj := len(strings.TrimSpace(supplemental[j].AnchorSymbol))
		if leni != lenj {
			return leni > lenj
		}
		return strings.TrimSpace(supplemental[i].Source) < strings.TrimSpace(supplemental[j].Source)
	})
	for _, item := range supplemental {
		if item.ContextRole == types.EvidenceContextRoleAbsenceSupport {
			continue
		}
		label := configTraceSupplementalSeedLabel(item)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
		if len(labels) >= 3 {
			return labels
		}
	}
	if len(labels) > 0 {
		return labels
	}
	for _, role := range roleOrder {
		for _, anchor := range plan.ConfigTraceDiagramAnchors {
			if strings.TrimSpace(anchor.Role) != string(role) {
				continue
			}
			label := strings.TrimSpace(anchor.Label)
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			labels = append(labels, label)
			break
		}
	}
	return labels
}

func configTraceFollowOnSeedRole(plan *types.AnswerSurfacePlan, item types.EvidenceItem) types.EvidenceDiagramRole {
	if plan == nil {
		return types.EvidenceDiagramRoleUnknown
	}
	switch item.GroundingStatus {
	case types.GroundingGrounded, types.GroundingRecovered:
	default:
		return types.EvidenceDiagramRoleUnknown
	}
	if item.Kind == types.EvidenceUnresolved || item.Kind == types.EvidenceTruncated {
		return types.EvidenceDiagramRoleUnknown
	}
	if item.ContextRole == types.EvidenceContextRoleAbsenceSupport || item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
		return types.EvidenceDiagramRoleUnknown
	}
	role := item.DiagramRole
	if role == types.EvidenceDiagramRoleUnknown {
		role = item.RequestedDiagramRole
	}
	switch role {
	case types.EvidenceDiagramRoleConfig:
		if types.LooksLikeConfigFilePath(item.Source) {
			return role
		}
	case types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleRuntime, types.EvidenceDiagramRoleOverride:
		source := strings.TrimSpace(strings.ReplaceAll(item.Source, `\`, `/`))
		if source == "" || types.LooksLikeConfigFilePath(source) || types.LooksLikeAuxiliaryEvidencePath(source) {
			return types.EvidenceDiagramRoleUnknown
		}
		if len(plan.ExactContextRequiredFiles) == 0 {
			return role
		}
		for _, required := range plan.ExactContextRequiredFiles {
			if strings.EqualFold(strings.TrimSpace(strings.ReplaceAll(required, `\`, `/`)), source) {
				return role
			}
		}
	}
	return types.EvidenceDiagramRoleUnknown
}

func configTraceRoleSeedLabel(role types.EvidenceDiagramRole, item types.EvidenceItem) string {
	trimmed := func(values ...string) string {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
		return ""
	}
	switch role {
	case types.EvidenceDiagramRoleConfig:
		return trimmed(item.Source, item.AnchorSymbol, item.Subject, item.Object)
	default:
		return trimmed(item.AnchorSymbol, item.Subject, item.Object, item.Source)
	}
}

func configTraceSupplementalSeedLabel(item types.EvidenceItem) string {
	for _, value := range []string{
		strings.TrimSpace(item.AnchorSymbol),
		strings.TrimSpace(item.Subject),
		strings.TrimSpace(item.Object),
		strings.TrimSpace(item.Source),
	} {
		if value != "" {
			return value
		}
	}
	return ""
}

func followOnGroundedContextSeedScore(plan *types.AnswerSurfacePlan, item types.EvidenceItem) int {
	score := 0
	if item.ContextRole == types.EvidenceContextRoleRelatedContext {
		score += 20
	}
	if item.GroundingStatus == types.GroundingGrounded {
		score += 6
	}
	if source := strings.TrimSpace(item.Source); source != "" {
		score += 4
		for _, file := range plan.ExactContextRequiredFiles {
			if strings.EqualFold(strings.ReplaceAll(strings.TrimSpace(file), `\`, `/`), strings.ReplaceAll(source, `\`, `/`)) {
				score += 30
				break
			}
		}
	}
	score += len(strings.TrimSpace(item.AnchorSymbol))
	switch item.DiagramRole {
	case types.EvidenceDiagramRoleDefault:
		score += 10
	case types.EvidenceDiagramRoleConfig:
		score += 8
	case types.EvidenceDiagramRoleOverride, types.EvidenceDiagramRoleRuntime:
		score += 4
	}
	switch item.AnchorKind {
	case types.AnchorAssignment, types.AnchorCall:
		score += 6
	case types.AnchorDefinition:
		score += 4
	case types.AnchorReturn, types.AnchorCondition:
		score += 2
	}
	return score
}

func normalizeConfigTraceAbsentSummarySurface(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext, exact *types.AnswerExactResolution) string {
	if ctx == nil || exact == nil || exact.Status != types.AnswerExactResolutionAbsent || exact.ContextMode != types.AnswerExactResolutionContextGroundedOnly {
		return summary
	}
	if ctx.AnalysisIR == nil ||
		ctx.AnalysisIR.RequestModel.Scenario != types.ScenarioConfigTrace ||
		ctx.AnalysisIR.RequestModel.AnswerSubject.Kind != types.SubjectConfigKey {
		return summary
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return summary
	}
	summary = normalizeInvalidSummaryDiagramFenceToCompiledSurface(summary, citations, gc, ctx, plan)
	if summaryContainsExplicitMermaidFence(summary) {
		return strings.TrimSpace(summary)
	}
	if summaryDiagramFenceCount(summary) > 0 {
		dc := answerDiagramContract(ctx)
		if dc == nil || !dc.Required {
			return strings.TrimSpace(summary)
		}
	}
	stripped := strings.TrimSpace(stripSummaryFencedBlocks(summary))
	fence := strings.TrimSpace(plan.CompiledDiagramFence)
	if fence == "" || plan.CompiledDiagramKind != types.DiagramFlow {
		fence = types.RenderConfigTraceDiagramFence(plan.ConfigTraceDiagramAnchors)
	}
	dc := answerDiagramContract(ctx)
	if dc == nil || !dc.Required {
		if fence == "" {
			return strings.TrimSpace(summary)
		}
		if stripped != "" {
			return strings.TrimSpace(stripped + "\n\n" + fence)
		}
		return fence
	}
	if fence == "" {
		if stripped != "" {
			return stripped
		}
		return summary
	}
	if stripped == "" {
		return fence
	}
	return strings.TrimSpace(stripped + "\n\n" + fence)
}

func summaryContainsExplicitMermaidFence(summary string) bool {
	return strings.Contains(strings.ToLower(summary), "```mermaid")
}

func normalizeRequiredDiagramSummarySurface(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return summary
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.Diagram == nil || !plan.Diagram.Required {
		return summary
	}
	if summaryDiagramFenceCount(summary) > 0 {
		return summary
	}
	fence := strings.TrimSpace(plan.CompiledDiagramFence)
	if fence == "" {
		return summary
	}
	if err := validateSummaryDiagramGrounding(fence, citations, gc, ctx); err != nil {
		return summary
	}
	return strings.TrimSpace(summary + "\n\n" + fence)
}

func normalizeInvalidSummaryDiagramFenceToCompiledSurface(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext, plan *types.AnswerSurfacePlan) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || plan == nil || strings.TrimSpace(plan.CompiledDiagramFence) == "" || !summaryContainsExplicitMermaidFence(summary) {
		return summary
	}
	if len(citations) == 0 || gc == nil {
		return summary
	}
	diagramErr := validateSummaryDiagramGrounding(summary, citations, gc, ctx)
	configErr := validateSummaryConfigTraceFenceLabels(summary, citations, gc, ctx)
	if diagramErr == nil && configErr == nil {
		return summary
	}
	compiled := strings.TrimSpace(plan.CompiledDiagramFence)
	if compiled == "" {
		return summary
	}
	if err := validateSummaryDiagramGrounding(compiled, citations, gc, ctx); err != nil {
		return summary
	}
	if err := validateSummaryConfigTraceFenceLabels(compiled, citations, gc, ctx); err != nil {
		return summary
	}
	body := strings.TrimSpace(stripSummaryMermaidFences(summary))
	if body == "" {
		return compiled
	}
	return strings.TrimSpace(body + "\n\n" + compiled)
}

func normalizeMinimalRoleLocateSummarySurface(summary string, shape types.AnswerShape, p *emitAnswerDocumentParams, citations []types.Citation, ctx *types.BusContext) string {
	summary = strings.TrimSpace(summary)
	if ctx == nil || p == nil {
		return summary
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceMinimalScalarRoleLocate {
		return summary
	}
	if shape != types.ShapeValue && shape != types.ShapeConfigValue {
		return summary
	}
	if p.Value == nil || strings.TrimSpace(p.Value.Literal) == "" {
		return summary
	}
	literal := strings.TrimSpace(p.Value.Literal)
	ref := p.Value.CitationRef
	if ref < 0 || ref >= len(citations) {
		return summary
	}
	cit := citations[ref]
	location := strings.TrimSpace(cit.File)
	if location != "" && cit.Line > 0 {
		location = fmt.Sprintf("%s:%d", cit.File, cit.Line)
	}
	if location == "" {
		return summary
	}
	return renderMinimalRoleLocateSummaryClean(ctx, literal, location)
}

var backtickedLabelRe = regexp.MustCompile("`([^`\n]+)`")

func normalizeLogSourceDriftSummarySurface(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) string {
	original := strings.TrimSpace(summary)
	summary = original
	plan := answerSurfacePlan(ctx)
	if plan != nil && plan.SummarySurfaceMode == types.AnswerSummarySurfaceDriftBoundedRootCause {
		allowed := driftBoundedSummaryAllowedLabels(citations, gc, plan)
		authoritative := driftBoundedSummaryAuthoritativeLabels(plan)
		sanitized := sanitizeDriftBoundedRootCauseSummary(summary, citations, gc, ctx)
		userFences := summaryDiagramFenceBlocks(sanitized)
		summary = driftBoundedSummaryPreservedProse(sanitized, citations, allowed, authoritative)
		if summary == "" {
			summary = renderStructuredDriftBoundedSummary(ctx)
		}
		if summary == "" {
			summary = renderDriftBoundedRootCauseFallbackSummary(ctx)
		}
		lead := renderLogSourceDriftLeadClean(ctx)
		blocks := []string{lead, summary}
		for _, fence := range userFences {
			if fence != "" {
				blocks = append(blocks, fence)
			}
		}
		if len(userFences) == 0 && (plan.CompiledDiagramKind == types.DiagramCallDAG || plan.CompiledDiagramKind == types.DiagramSequence) {
			if fence := strings.TrimSpace(plan.CompiledDiagramFence); fence != "" {
				blocks = append(blocks, fence)
			}
		}
		return preserveRequiredSummaryCoverageAcrossNormalization(original, joinDistinctSummaryBlocks(blocks...), ctx)
	}
	lead := renderLogSourceDriftLeadClean(ctx)
	if lead == "" {
		return preserveRequiredSummaryCoverageAcrossNormalization(original, summary, ctx)
	}
	if summary == "" {
		return preserveRequiredSummaryCoverageAcrossNormalization(original, lead, ctx)
	}
	return preserveRequiredSummaryCoverageAcrossNormalization(original, strings.TrimSpace(lead+"\n\n"+summary), ctx)
}

func preserveRequiredSummaryCoverageAcrossNormalization(original, candidate string, ctx *types.BusContext) string {
	original = strings.TrimSpace(original)
	candidate = strings.TrimSpace(candidate)
	if original == "" || candidate == "" || ctx == nil {
		return candidate
	}
	if original == candidate {
		return candidate
	}
	candidate = augmentSummaryLogTriageCoverage(candidate, ctx)
	if !summaryCoverageLostAcrossNormalization(original, candidate, ctx) {
		return candidate
	}
	if block := firstRequiredCoverageSummaryBlock(original, ctx); block != "" {
		repaired := joinDistinctSummaryBlocks(block, candidate)
		if !summaryCoverageLostAcrossNormalization(original, repaired, ctx) {
			return repaired
		}
	}
	if validateSummaryLogTriageCoverage(original, ctx) == nil {
		return original
	}
	return candidate
}

func augmentSummaryLogTriageCoverage(summary string, ctx *types.BusContext) string {
	summary = strings.TrimSpace(summary)
	if ctx == nil || ctx.Mutable == nil {
		return summary
	}
	errorTypes := types.LogBundleErrorTypes(ctx.Mutable.LogTriage())
	if len(errorTypes) == 0 {
		return summary
	}
	lowerSummary := strings.ToLower(summary)
	var missing []string
	for _, errType := range errorTypes {
		if !logTriageTypeCovered(errType, lowerSummary) {
			missing = append(missing, errType)
		}
	}
	if len(missing) == 0 {
		return summary
	}
	coverage := strings.TrimSpace(RenderDriftBoundedErrorTypeCoverageSummary(missing, requestedAnswerDocumentLanguage(ctx)))
	if coverage == "" {
		return summary
	}
	return joinDistinctSummaryBlocks(summary, coverage)
}

func summaryCoverageLostAcrossNormalization(original, candidate string, ctx *types.BusContext) bool {
	if validateSummaryLogTriageCoverage(original, ctx) == nil &&
		validateSummaryLogTriageCoverage(candidate, ctx) != nil {
		return true
	}
	return false
}

func firstRequiredCoverageSummaryBlock(summary string, ctx *types.BusContext) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || ctx == nil {
		return ""
	}
	blocks := strings.Split(summary, "\n\n")
	var cumulative []string
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "```") {
			continue
		}
		cumulative = append(cumulative, block)
		joined := strings.Join(cumulative, "\n\n")
		if validateSummaryLogTriageCoverage(joined, ctx) == nil {
			return joined
		}
	}
	return ""
}

func sanitizeDriftBoundedRootCauseSummary(summary string, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return strings.TrimSpace(summary)
	}
	allowed := driftBoundedSummaryAllowedLabels(citations, gc, plan)
	if len(allowed) == 0 {
		return strings.TrimSpace(summary)
	}
	blocks := strings.Split(strings.TrimSpace(summary), "\n\n")
	kept := make([]string, 0, len(blocks))
	proseBlocks := 0
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if strings.HasPrefix(block, "```") {
			if summaryDiagramFenceCount(block) > 0 {
				kept = append(kept, block)
			}
			continue
		}
		if isDriftBoundedDecorativeSummaryBlock(block) {
			continue
		}
		if driftBoundedSummaryBlockMentionsUnsupportedBacktickedLabel(block, allowed) {
			continue
		}
		kept = append(kept, block)
		proseBlocks++
	}
	if proseBlocks == 0 {
		var fences []string
		for _, block := range kept {
			if strings.HasPrefix(block, "```") {
				fences = append(fences, block)
			}
		}
		if len(fences) == 0 {
			return ""
		}
		return strings.TrimSpace(strings.Join(fences, "\n\n"))
	}
	return strings.TrimSpace(strings.Join(kept, "\n\n"))
}

func driftBoundedSummaryNeedsFallback(summary string) bool {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return false
	}
	if strings.Contains(summary, "```") {
		return true
	}
	blocks := 0
	for _, block := range strings.Split(summary, "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		blocks++
		if blocks > 1 {
			return true
		}
	}
	return false
}

func summaryDiagramFenceBlocks(summary string) []string {
	var out []string
	for _, block := range strings.Split(strings.TrimSpace(summary), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || !strings.HasPrefix(block, "```") {
			continue
		}
		if summaryDiagramFenceCount(block) == 0 {
			continue
		}
		out = append(out, block)
	}
	return out
}

func driftBoundedSummaryPreservedProse(summary string, citations []types.Citation, allowed map[string]bool, authoritative map[string]bool) string {
	summary = strings.TrimSpace(summary)
	if summary == "" || len(allowed) == 0 || len(authoritative) == 0 {
		return ""
	}
	hasUserFence := len(summaryDiagramFenceBlocks(summary)) > 0
	var kept []string
	for _, block := range strings.Split(summary, "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "```") || isDriftBoundedDecorativeSummaryBlock(block) {
			continue
		}
		preserve := driftBoundedSummaryBlockHasPreservableAuthority(block, citations, authoritative)
		if hasUserFence && !preserve && driftBoundedSummaryBlockMentionsCitationLine(block, citations) && driftBoundedSummaryBlockSupportedLabelHits(block, allowed) > 0 {
			preserve = true
		}
		if !preserve {
			continue
		}
		kept = append(kept, block)
	}
	if len(kept) == 0 {
		return ""
	}
	if hasUserFence {
		return strings.TrimSpace(strings.Join(kept, "\n\n"))
	}
	return strings.TrimSpace(kept[0])
}

func renderStructuredDriftBoundedSummary(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.SummarySurfaceMode != types.AnswerSummarySurfaceDriftBoundedRootCause {
		return ""
	}
	primary := strings.TrimSpace(normalizeLogSourceDriftCompletionReason(ctx, plan.StableInvestigationReason))
	if primary == "" {
		primary = strings.TrimSpace(renderDriftBoundedCurrentRootCauseSummary(ctx))
	}
	detail := strings.TrimSpace(renderDriftBoundedCurrentCodeDetailSummary(ctx))
	coverage := strings.TrimSpace(RenderDriftBoundedLogBundleSurfaceSummary(ctx.Mutable.LogTriage(), requestedAnswerDocumentLanguage(ctx)))
	return joinDistinctSummaryBlocks(coverage, primary, detail)
}

func driftBoundedSummaryAllowedLabels(citations []types.Citation, gc *ground.Context, plan *types.AnswerSurfacePlan) map[string]bool {
	allowed := make(map[string]bool)
	add := func(label string) {
		label = strings.TrimSpace(strings.ReplaceAll(label, `\`, `/`))
		if label == "" {
			return
		}
		allowed["exact|"+strings.ToLower(label)] = true
		if base := strings.TrimSpace(path.Base(label)); base != "" && base != "." {
			allowed["exact|"+strings.ToLower(base)] = true
		}
		if tail := types.NormalizedSurfaceSymbolTail(label); tail != "" {
			allowed["sym|"+tail] = true
		}
	}
	addTokens := func(text string) {
		for _, token := range valueLiteralTokenRe.FindAllString(text, -1) {
			add(token)
		}
	}
	if plan != nil {
		for _, anchor := range plan.LogObservedAnchors {
			add(anchor.Func)
			add(anchor.File)
			add(anchor.OriginalFunc)
			add(anchor.OriginalFile)
		}
		for _, anchor := range plan.LogSourceDriftAnchors {
			add(anchor.Func)
			add(anchor.File)
			add(anchor.OriginalFunc)
			add(anchor.OriginalFile)
		}
		for _, seed := range plan.ExternalObservationSeeds {
			add(seed.Raw)
			add(seed.Func)
			add(seed.File)
			add(seed.AnchoredFile)
		}
	}
	for _, cite := range citations {
		add(cite.File)
		addTokens(cite.Quote)
		if gc == nil || cite.Line <= 0 {
			continue
		}
		file := strings.TrimSpace(strings.ReplaceAll(cite.File, `\`, `/`))
		lines := gc.LineIndex[file]
		for current := cite.Line - corroborationWindow; current <= cite.Line+corroborationWindow; current++ {
			if current <= 0 {
				continue
			}
			if text, ok := lines[current]; ok {
				addTokens(text)
			}
		}
	}
	return allowed
}

func driftBoundedSummaryAuthoritativeLabels(plan *types.AnswerSurfacePlan) map[string]bool {
	allowed := make(map[string]bool)
	if plan == nil {
		return allowed
	}
	add := func(label string) {
		label = strings.TrimSpace(strings.ReplaceAll(label, `\`, `/`))
		if label == "" {
			return
		}
		allowed["exact|"+strings.ToLower(label)] = true
		if base := strings.TrimSpace(path.Base(label)); base != "" && base != "." {
			allowed["exact|"+strings.ToLower(base)] = true
		}
		if tail := types.NormalizedSurfaceSymbolTail(label); tail != "" {
			allowed["sym|"+tail] = true
		}
	}
	for _, anchor := range plan.LogObservedAnchors {
		add(anchor.Func)
		add(anchor.File)
		add(anchor.OriginalFunc)
		add(anchor.OriginalFile)
	}
	for _, anchor := range plan.LogSourceDriftAnchors {
		add(anchor.Func)
		add(anchor.File)
		add(anchor.OriginalFunc)
		add(anchor.OriginalFile)
	}
	return allowed
}

func driftBoundedSummaryBlockHasPreservableAuthority(block string, citations []types.Citation, allowed map[string]bool) bool {
	hits := driftBoundedSummaryBlockSupportedLabelHits(block, allowed)
	if hits >= 2 {
		return true
	}
	return hits >= 1 && driftBoundedSummaryBlockMentionsCitationLine(block, citations)
}

func driftBoundedSummaryBlockSupportedLabelHits(block string, allowed map[string]bool) int {
	if len(allowed) == 0 {
		return 0
	}
	seen := make(map[string]bool)
	hits := 0
	for _, match := range backtickedLabelRe.FindAllStringSubmatch(block, -1) {
		if len(match) < 2 {
			continue
		}
		label := strings.TrimSpace(match[1])
		if label == "" {
			continue
		}
		if driftBoundedSummaryLabelAllowed(label, allowed) {
			key := strings.ToLower(label)
			if seen[key] {
				continue
			}
			seen[key] = true
			hits++
			continue
		}
		if !driftBoundedSummaryLabelLooksLikeCodeExpression(label) {
			continue
		}
		for _, token := range valueLiteralTokenRe.FindAllString(label, -1) {
			if token == "" || !driftBoundedSummaryLabelAllowed(token, allowed) {
				continue
			}
			key := strings.ToLower(token)
			if seen[key] {
				continue
			}
			seen[key] = true
			hits++
		}
	}
	return hits
}

func driftBoundedSummaryBlockMentionsCitationLine(block string, citations []types.Citation) bool {
	for _, cite := range citations {
		if cite.Line <= 0 {
			continue
		}
		pattern := regexp.MustCompile(fmt.Sprintf(`(^|[^0-9])%d([^0-9]|$)`, cite.Line))
		if pattern.MatchString(block) {
			return true
		}
	}
	return false
}

func driftBoundedSummaryBlockMentionsUnsupportedBacktickedLabel(block string, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return false
	}
	for _, match := range backtickedLabelRe.FindAllStringSubmatch(block, -1) {
		if len(match) < 2 {
			continue
		}
		label := strings.TrimSpace(match[1])
		if label == "" || driftBoundedSummaryLabelAllowed(label, allowed) || driftBoundedSummaryLabelLooksLikeCodeExpression(label) || !driftBoundedSummaryLabelLooksLikeSurfaceAnchor(label) {
			continue
		}
		return true
	}
	return false
}

func isDriftBoundedDecorativeSummaryBlock(block string) bool {
	block = strings.TrimSpace(block)
	if block == "" || strings.Contains(block, "\n") {
		return false
	}
	if strings.HasPrefix(block, "#") {
		return true
	}
	if strings.HasPrefix(block, "**") && strings.HasSuffix(block, "**") {
		inner := strings.TrimSpace(strings.Trim(block, "*"))
		inner = strings.TrimSpace(strings.TrimRight(inner, ":："))
		return inner != ""
	}
	return false
}

func driftBoundedSummaryLabelAllowed(label string, allowed map[string]bool) bool {
	label = strings.TrimSpace(strings.ReplaceAll(label, `\`, `/`))
	if label == "" {
		return true
	}
	lower := strings.ToLower(label)
	if allowed["exact|"+lower] {
		return true
	}
	if base := strings.TrimSpace(path.Base(label)); base != "" && base != "." && allowed["exact|"+strings.ToLower(base)] {
		return true
	}
	if tail := types.NormalizedSurfaceSymbolTail(label); tail != "" && allowed["sym|"+tail] {
		return true
	}
	return false
}

func driftBoundedSummaryLabelLooksLikeCodeExpression(label string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	return strings.ContainsAny(label, "()[]=<>!|&+-*/{},;")
}

func driftBoundedSummaryLabelLooksLikeSurfaceAnchor(label string) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	if strings.Contains(label, "/") || strings.Contains(label, `\`) || strings.Contains(label, "::") {
		return true
	}
	if strings.Contains(label, ".") || strings.Contains(label, "_") {
		return true
	}
	for _, r := range label {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

func renderDriftBoundedRootCauseFallbackSummary(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil {
		return ""
	}
	funcs := driftBoundedObservedFunctions(plan)
	if len(funcs) == 0 {
		return ""
	}
	zh := emitAnswerDocIsZh(ctx)
	if len(funcs) >= 2 {
		inner := funcs[0]
		caller := funcs[1]
		if zh {
			return fmt.Sprintf("当前仓库里已经锚定的崩溃路径是 `%s` 调用 `%s`。由于运行日志对应的是旧构建，现有证据只能确认这条当前代码路径以及附近已经引用锚定的机制，不能把旧构建中的更深层历史解引用点断言到未被当前引用锚定的辅助函数上。", caller, inner)
		}
		return fmt.Sprintf("The current repo only grounds the verified path where `%s` calls `%s`. Because the runtime log came from an older build, the current evidence can confirm this anchored code path and its nearby cited mechanism, but it cannot pin the older historical dereference onto an auxiliary function that is not itself grounded by the current citations.", caller, inner)
	}
	if zh {
		return fmt.Sprintf("当前仓库里已经锚定的崩溃函数是 `%s`。由于运行日志对应的是旧构建，现有证据只能确认当前代码里这条已锚定路径附近的机制，不能把旧构建中的更深层历史解引用点断言到未被当前引用锚定的辅助函数上。", funcs[0])
	}
	return fmt.Sprintf("The current repo only grounds the failure function `%s`. Because the runtime log came from an older build, the current evidence can confirm the nearby anchored mechanism in the repo, but it cannot pin the older historical dereference onto an auxiliary function that is not itself grounded by the current citations.", funcs[0])
}

func driftBoundedObservedFunctions(plan *types.AnswerSurfacePlan) []string {
	if plan == nil {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	appendName := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for _, anchor := range plan.LogObservedAnchors {
		appendName(anchor.Func)
	}
	for _, anchor := range plan.LogSourceDriftAnchors {
		name := strings.TrimSpace(anchor.OriginalFunc)
		if name == "" || seen[name] {
			name = strings.TrimSpace(anchor.Func)
		}
		appendName(name)
	}
	return out
}

func joinDistinctSummaryBlocks(parts ...string) string {
	seen := make(map[string]bool, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		out = append(out, part)
	}
	return strings.Join(out, "\n\n")
}

func appendLogSourceDriftCaveat(caveats []string, ctx *types.BusContext) []string {
	caveat := renderLogSourceDriftCaveatClean(ctx)
	if caveat == "" {
		return caveats
	}
	for _, existing := range caveats {
		if strings.TrimSpace(existing) == caveat {
			return caveats
		}
	}
	return append(caveats, caveat)
}

func renderLogSourceDriftLeadClean(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.LogSourceDriftAnchors) == 0 {
		return ""
	}
	zh := emitAnswerDocIsZh(ctx)
	// Single-anchor case: pick prose by Reason so Tier 2 / Tier 3
	// drifts get clearer attribution than the generic "lines don't
	// align" framing. Multi-anchor case falls back to the generic
	// summary because per-anchor reasons may differ.
	if len(plan.LogSourceDriftAnchors) == 1 {
		anchor := plan.LogSourceDriftAnchors[0]
		if anchor.File != "" && anchor.ObservedLine > 0 && anchor.AnchoredLine > 0 {
			switch anchor.Reason {
			case types.DriftReasonTailRename:
				if zh {
					return fmt.Sprintf("运行日志里的函数名 `%s` 看起来已被重命名为当前仓库里的 `%s`(同文件 `%s`)。日志指向 `%s:%d`,当前对应位置在 `%s:%d`。下面的解释以当前代码中已经验证的函数/调用链为准。",
						anchor.OriginalFunc, anchor.Func, anchor.File,
						anchor.File, anchor.ObservedLine,
						anchor.File, anchor.AnchoredLine)
				}
				return fmt.Sprintf("The function name in the runtime log (`%s`) appears to have been renamed to `%s` in the current checkout (same file `%s`). The log points at `%s:%d`; the corresponding location now is `%s:%d`. The explanation below is anchored to the current verified function.",
					anchor.OriginalFunc, anchor.Func, anchor.File,
					anchor.File, anchor.ObservedLine,
					anchor.File, anchor.AnchoredLine)
			case types.DriftReasonFileMoved:
				if zh {
					return fmt.Sprintf("运行日志里的文件 `%s` 看起来已被搬到当前仓库的 `%s`(同名函数 `%s`)。日志指向 `%s:%d`,当前对应位置在 `%s:%d`。下面的解释以当前代码中已经验证的位置为准。",
						anchor.OriginalFile, anchor.File, anchor.Func,
						anchor.OriginalFile, anchor.ObservedLine,
						anchor.File, anchor.AnchoredLine)
				}
				return fmt.Sprintf("The file in the runtime log (`%s`) appears to have moved to `%s` in the current checkout (same function `%s`). The log points at `%s:%d`; the corresponding location now is `%s:%d`. The explanation below is anchored to the current verified location.",
					anchor.OriginalFile, anchor.File, anchor.Func,
					anchor.OriginalFile, anchor.ObservedLine,
					anchor.File, anchor.AnchoredLine)
			default: // DriftReasonLineDrift or empty (back-compat)
				if zh {
					return fmt.Sprintf("运行日志里的源码行号和当前代码仓并不完全对齐：日志指向 `%s:%d`，但当前仓库里同名函数最近的已锚定位置是 `%s:%d`。下面的解释以当前代码中已经验证的调用路径为准，而不是声称精确还原旧二进制里的那一行。",
						anchor.File, anchor.ObservedLine, anchor.File, anchor.AnchoredLine)
				}
				return fmt.Sprintf("The runtime log's source line does not fully align with the current checkout: the log points at `%s:%d`, while the closest grounded anchor for the same function in the current repo is `%s:%d`. The explanation below is therefore anchored to the current verified code path, not a byte-for-byte reconstruction of the older logged line.",
					anchor.File, anchor.ObservedLine, anchor.File, anchor.AnchoredLine)
			}
		}
	}
	if zh {
		return "运行日志里的源码行号和当前代码仓并不完全对齐。下面的解释以当前仓库中已经验证的函数/调用链为准，而不是声称精确还原旧日志里的每一个行号。"
	}
	return "The runtime log's source lines do not fully align with the current checkout. The explanation below is therefore anchored to the current verified function / call-chain in the repo, not a byte-for-byte reconstruction of the older logged lines."
}

func renderLogSourceDriftCaveatClean(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.LogSourceDriftAnchors) == 0 {
		return ""
	}
	if emitAnswerDocIsZh(ctx) {
		return "运行日志里的行号与当前仓库代码存在偏移，所以这份答案解释的是当前代码里最近、且已经锚定的机制，而不是声称精确复原旧构建中的每一行。"
	}
	return "The runtime log line numbers differ from the current checkout, so this answer explains the nearest current grounded mechanism rather than claiming a byte-exact reconstruction of the older build."
}

func renderMinimalRoleLocateSummaryClean(ctx *types.BusContext, literal, location string) string {
	kind := types.SubjectUnknown
	if ctx != nil && ctx.AnalysisIR != nil {
		kind = ctx.AnalysisIR.RequestModel.AnswerSubject.Kind
	}
	if emitAnswerDocIsZh(ctx) {
		switch kind {
		case types.SubjectFunctionName:
			return fmt.Sprintf("对应的函数是 `%s`，位置在 `%s`。", literal, location)
		case types.SubjectTypeName, types.SubjectInterface:
			return fmt.Sprintf("对应的类型是 `%s`，位置在 `%s`。", literal, location)
		case types.SubjectFilePath:
			return fmt.Sprintf("对应的文件是 `%s`，相关锚点见 `%s`。", literal, location)
		case types.SubjectHandlerRoute:
			return fmt.Sprintf("对应的路由是 `%s`，锚点在 `%s`。", literal, location)
		case types.SubjectStringLiteral:
			return fmt.Sprintf("对应的字符串字面量是 `%s`，锚点在 `%s`。", literal, location)
		case types.SubjectEnumValue:
			return fmt.Sprintf("对应的枚举值是 `%s`，锚点在 `%s`。", literal, location)
		case types.SubjectStructField:
			return fmt.Sprintf("对应的字段是 `%s`，锚点在 `%s`。", literal, location)
		default:
			return fmt.Sprintf("对应的结果是 `%s`，锚点在 `%s`。", literal, location)
		}
	}
	switch kind {
	case types.SubjectFunctionName:
		return fmt.Sprintf("The matching function is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectTypeName, types.SubjectInterface:
		return fmt.Sprintf("The matching type is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectFilePath:
		return fmt.Sprintf("The matching file is `%s`; the grounding anchor is `%s`.", literal, location)
	case types.SubjectHandlerRoute:
		return fmt.Sprintf("The matching route is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectStringLiteral:
		return fmt.Sprintf("The matching string literal is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectEnumValue:
		return fmt.Sprintf("The matching enum value is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectStructField:
		return fmt.Sprintf("The matching field is `%s`, anchored at `%s`.", literal, location)
	default:
		return fmt.Sprintf("The matching result is `%s`, anchored at `%s`.", literal, location)
	}
}

func renderLogSourceDriftLead(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.LogSourceDriftAnchors) == 0 {
		return ""
	}
	if emitAnswerDocIsZh(ctx) {
		anchor := plan.LogSourceDriftAnchors[0]
		if len(plan.LogSourceDriftAnchors) == 1 && anchor.File != "" && anchor.ObservedLine > 0 && anchor.AnchoredLine > 0 {
			return fmt.Sprintf("杩愯鏃ュ織涓殑婧愮爜琛屽彿涓庡綋鍓嶄粨搴撶増鏈苟鏈畬鍏ㄥ榻愶細鏃ュ織鎸囧悜 `%s:%d`锛屼絾褰撳墠浠撳簱涓凡閿氬畾鐨勫悓鍚嶅嚱鏁版満鍒堕敋鐐规洿鎺ヨ繎 `%s:%d`銆備笅闈㈢殑瑙ｉ噴浠ュ綋鍓嶄粨搴撻噷宸查敋瀹氱殑浠ｇ爜璺緞涓哄噯锛岃€屼笉鏄鏃ュ織鏃х増琛屽彿鐨勯€愬瓧杩樺師銆?",
				anchor.File, anchor.ObservedLine, anchor.File, anchor.AnchoredLine)
		}
		return "杩愯鏃ュ織涓殑婧愮爜琛屽彿涓庡綋鍓嶄粨搴撶増鏈苟鏈畬鍏ㄥ榻愩€備笅闈㈢殑瑙ｉ噴浠ュ綋鍓嶄粨搴撻噷宸查敋瀹氱殑鍚屼竴鍑芥暟 / 璋冪敤閾句负鍑嗭紝鐢ㄦ潵璇存槑鏈€鎺ヨ繎鐨勭幇琛屾満鍒躲€?"
	}
	anchor := plan.LogSourceDriftAnchors[0]
	if len(plan.LogSourceDriftAnchors) == 1 && anchor.File != "" && anchor.ObservedLine > 0 && anchor.AnchoredLine > 0 {
		return fmt.Sprintf("The runtime log's source line does not fully align with the current checkout: the log points at `%s:%d`, while the closest grounded anchor for the same function in the current repo is `%s:%d`. The explanation below is therefore anchored to the current verified code path, not a byte-for-byte reconstruction of the older logged line.",
			anchor.File, anchor.ObservedLine, anchor.File, anchor.AnchoredLine)
	}
	return "The runtime log's source lines do not fully align with the current checkout. The explanation below is therefore anchored to the current verified function / call-chain in the repo, not a byte-for-byte reconstruction of the older logged lines."
}

func renderLogSourceDriftCaveat(ctx *types.BusContext) string {
	plan := answerSurfacePlan(ctx)
	if plan == nil || len(plan.LogSourceDriftAnchors) == 0 {
		return ""
	}
	if emitAnswerDocIsZh(ctx) {
		return "杩愯鏃ュ織鐨勮鍙蜂笌褰撳墠浠撳簱浠ｇ爜鏈夊亸绉伙紝鏈瓟妗堜互褰撳墠宸查敋瀹氱殑浠ｇ爜璺緞瑙ｉ噴鏈€鎺ヨ繎鐨勭幇琛屾満鍒躲€?"
	}
	return "The runtime log line numbers differ from the current checkout, so this answer explains the nearest current grounded mechanism rather than claiming a byte-exact reconstruction of the older build."
}

func renderMinimalRoleLocateSummary(ctx *types.BusContext, literal, location string) string {
	kind := types.SubjectUnknown
	if ctx != nil && ctx.AnalysisIR != nil {
		kind = ctx.AnalysisIR.RequestModel.AnswerSubject.Kind
	}
	if emitAnswerDocIsZh(ctx) {
		switch kind {
		case types.SubjectFunctionName:
			return fmt.Sprintf("对应的函数是 `%s`，位置在 `%s`。", literal, location)
		case types.SubjectTypeName, types.SubjectInterface:
			return fmt.Sprintf("对应的类型是 `%s`，位置在 `%s`。", literal, location)
		case types.SubjectFilePath:
			return fmt.Sprintf("对应的文件是 `%s`。相关锚点见 `%s`。", literal, location)
		case types.SubjectHandlerRoute:
			return fmt.Sprintf("对应的路由是 `%s`，锚点在 `%s`。", literal, location)
		case types.SubjectStringLiteral:
			return fmt.Sprintf("对应的字符串字面量是 `%s`，锚点在 `%s`。", literal, location)
		case types.SubjectEnumValue:
			return fmt.Sprintf("对应的枚举值是 `%s`，锚点在 `%s`。", literal, location)
		case types.SubjectStructField:
			return fmt.Sprintf("对应的字段是 `%s`，锚点在 `%s`。", literal, location)
		default:
			return fmt.Sprintf("对应的结果是 `%s`，锚点在 `%s`。", literal, location)
		}
	}
	switch kind {
	case types.SubjectFunctionName:
		return fmt.Sprintf("The matching function is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectTypeName, types.SubjectInterface:
		return fmt.Sprintf("The matching type is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectFilePath:
		return fmt.Sprintf("The matching file is `%s`; the grounding anchor is `%s`.", literal, location)
	case types.SubjectHandlerRoute:
		return fmt.Sprintf("The matching route is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectStringLiteral:
		return fmt.Sprintf("The matching string literal is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectEnumValue:
		return fmt.Sprintf("The matching enum value is `%s`, anchored at `%s`.", literal, location)
	case types.SubjectStructField:
		return fmt.Sprintf("The matching field is `%s`, anchored at `%s`.", literal, location)
	default:
		return fmt.Sprintf("The matching result is `%s`, anchored at `%s`.", literal, location)
	}
}

func emitAnswerDocIsZh(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	lang := strings.ToLower(strings.TrimSpace(ctx.Language))
	return strings.HasPrefix(lang, "zh") || lang == "cn" || lang == "chinese"
}

func stripSummaryFencedBlocks(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	out := fencedCodeBlockRe.ReplaceAllString(summary, "")
	out = strings.TrimSpace(out)
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return out
}

func stripSummaryMermaidFences(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	out := fencedCodeBlockWithInfoRe.ReplaceAllStringFunc(summary, func(block string) string {
		m := fencedCodeBlockWithInfoRe.FindStringSubmatch(block)
		if len(m) < 3 {
			return block
		}
		info := strings.TrimSpace(m[1])
		if strings.EqualFold(info, "mermaid") || strings.HasPrefix(strings.ToLower(info), "mermaid ") {
			return ""
		}
		return block
	})
	out = strings.TrimSpace(out)
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	return out
}

func trimLeadingExactTargetSentences(paragraph string, contract *types.ExactResolutionContract) string {
	paragraph = strings.TrimSpace(paragraph)
	if paragraph == "" || contract == nil {
		return paragraph
	}
	sentences := splitSummarySentences(paragraph)
	if len(sentences) == 0 {
		return paragraph
	}
	dropUntil := 0
	for dropUntil < len(sentences) && repeatedExactTargetAfterLead(contract, sentences[dropUntil]) != "" {
		dropUntil++
	}
	if dropUntil == 0 || dropUntil >= len(sentences) {
		return ""
	}
	return strings.TrimSpace(strings.Join(sentences[dropUntil:], " "))
}

func splitLeadingSummaryParagraph(summary string) (heading, paragraph, rest string) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", "", ""
	}
	lines := strings.Split(summary, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "#") {
		heading = strings.TrimSpace(lines[i])
		i++
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
	}
	start := i
	for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
		i++
	}
	paragraph = strings.TrimSpace(strings.Join(lines[start:i], "\n"))
	rest = strings.TrimSpace(strings.Join(lines[i:], "\n"))
	return heading, paragraph, rest
}

func splitSummaryParagraphs(summary string) []string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	parts := strings.Split(summary, "\n\n")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func joinLeadingSummaryParts(heading, paragraph, rest string) string {
	var parts []string
	heading = strings.TrimSpace(heading)
	paragraph = strings.TrimSpace(paragraph)
	rest = strings.TrimSpace(rest)
	if heading != "" {
		parts = append(parts, heading)
	}
	if paragraph != "" {
		parts = append(parts, paragraph)
	}
	if rest != "" {
		parts = append(parts, rest)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func splitSummarySentences(paragraph string) []string {
	paragraph = strings.TrimSpace(paragraph)
	if paragraph == "" {
		return nil
	}
	runes := []rune(paragraph)
	var sentences []string
	start := 0
	for i, r := range runes {
		if !isSummarySentenceTerminator(r) || !isSummarySentenceBoundary(runes, i) {
			continue
		}
		sentence := strings.TrimSpace(string(runes[start : i+1]))
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
		start = i + 1
	}
	if tail := strings.TrimSpace(string(runes[start:])); tail != "" {
		sentences = append(sentences, tail)
	}
	return sentences
}

func isSummarySentenceTerminator(r rune) bool {
	switch r {
	case '.', '!', '?', ';', '\u3002', '\uff01', '\uff1f', '\uff1b':
		return true
	default:
		return false
	}
}

func isSummarySentenceBoundary(runes []rune, idx int) bool {
	if idx < 0 || idx >= len(runes) {
		return false
	}
	switch runes[idx] {
	case '\u3002', '\uff01', '\uff1f', '\uff1b':
		return true
	}
	if idx == len(runes)-1 {
		return true
	}
	next := runes[idx+1]
	if unicode.IsSpace(next) {
		return true
	}
	switch next {
	case '`', '"', '\'', ')', ']', '}', '>', '\u300d', '\u300f', '\u201d', '\u2019':
		return true
	default:
		return false
	}
}

func renderAnswerDocumentExactResolutionLeadClean(contract *types.ExactResolutionContract, exact *types.AnswerExactResolution, lang string) string {
	if contract == nil || exact == nil || len(contract.Targets) == 0 {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	label := localizedExactResolutionTargetLabel(strings.TrimSpace(contract.TargetLabel), zh)
	targetRef := formatExactResolutionTargetReference(label, contract.Targets, zh)
	switch exact.Status {
	case types.AnswerExactResolutionAbsent:
		if zh {
			lead := fmt.Sprintf("仓库里没有找到 %s。", targetRef)
			if exact.ContextMode == types.AnswerExactResolutionContextGroundedOnly {
				lead += " 下面如果补充同族、且已经落地的上下文，也只是帮助理解背景，不表示它就是你问的那个目标。"
			}
			return lead
		}
		lead := fmt.Sprintf("The repository does not contain %s.", targetRef)
		if exact.ContextMode == types.AnswerExactResolutionContextGroundedOnly {
			lead += " Any grounded same-family context below is only there to explain the background; it is not the requested target itself."
		}
		return lead
	case types.AnswerExactResolutionAliasMatch:
		if zh {
			return fmt.Sprintf("已引用的证据表明，%s 明确映射到 `%s`。", targetRef, exact.Anchor)
		}
		return fmt.Sprintf("Grounded evidence shows that %s resolves explicitly through `%s`.", targetRef, exact.Anchor)
	case types.AnswerExactResolutionExactMatch:
		if zh {
			return fmt.Sprintf("已在引用证据中直接找到 %s。", targetRef)
		}
		return fmt.Sprintf("%s appears directly in the cited evidence.", targetRef)
	default:
		return ""
	}
}

func renderAnswerDocumentExactResolutionLead(contract *types.ExactResolutionContract, exact *types.AnswerExactResolution, lang string) string {
	if contract == nil || exact == nil || len(contract.Targets) == 0 {
		return ""
	}
	zh := answerDocumentRequiresChinese(lang)
	label := localizedExactResolutionTargetLabel(strings.TrimSpace(contract.TargetLabel), zh)
	targetRef := formatExactResolutionTargetReference(label, contract.Targets, zh)
	switch exact.Status {
	case types.AnswerExactResolutionAbsent:
		if zh {
			lead := fmt.Sprintf("仓库里没有找到%s。", targetRef)
			if exact.ContextMode == types.AnswerExactResolutionContextGroundedOnly {
				lead += " 下面如果补充同族的已落地线索，也只是帮助理解背景，不表示它就是你问的目标。"
			}
			return lead
		}
		lead := fmt.Sprintf("The repository does not contain %s.", targetRef)
		if exact.ContextMode == types.AnswerExactResolutionContextGroundedOnly {
			lead += " Any grounded same-family context below is only there to explain the background; it is not the requested target itself."
		}
		return lead
	case types.AnswerExactResolutionAliasMatch:
		if zh {
			return fmt.Sprintf("已引用的证据表明，%s明确对应 `%s`。", targetRef, exact.Anchor)
		}
		return fmt.Sprintf("Grounded evidence shows that %s resolves explicitly through `%s`.", targetRef, exact.Anchor)
	case types.AnswerExactResolutionExactMatch:
		if zh {
			return fmt.Sprintf("已在引用证据中直接找到%s。", targetRef)
		}
		return fmt.Sprintf("%s appears directly in the cited evidence.", targetRef)
	default:
		return ""
	}
}

func localizedExactResolutionTargetLabel(label string, zh bool) string {
	label = strings.TrimSpace(label)
	if !zh {
		if label == "" {
			return "target"
		}
		return label
	}
	switch strings.ToLower(label) {
	case "", "target":
		return "目标项"
	case "config key":
		return "配置项"
	case "symbol":
		return "符号"
	case "file path":
		return "文件路径"
	case "directory":
		return "目录"
	case "route":
		return "路由"
	case "env var", "environment variable":
		return "环境变量"
	case "cli flag", "flag":
		return "命令行参数"
	default:
		return label
	}
}

func formatExactResolutionTargetReference(label string, targets []string, zh bool) string {
	names := backtickJoin(targets)
	if zh {
		if len(targets) == 1 {
			return fmt.Sprintf("名为 %s 的%s", names, label)
		}
		return fmt.Sprintf("这些%s：%s", label, names)
	}
	if len(targets) == 1 {
		return fmt.Sprintf("the %s %s", label, names)
	}
	return fmt.Sprintf("these %s: %s", label, names)
}

type exactResolutionProofEntry struct {
	LineText     string
	Source       string
	Subject      string
	AnchorSymbol string
	AnchorKind   types.AnchorKind
	Object       string
	ContextRole  types.EvidenceContextRole
	Grounded     bool
	Production   bool
	DirectAnchor bool
	FromEvidence bool
}

type exactResolutionProof struct {
	Contract                        *types.ExactResolutionContract
	Entries                         []exactResolutionProofEntry
	AnyTarget                       bool
	AnyProductionTarget             bool
	AnyProductionTargetAnchor       bool
	AnyDefiningTargetProof          bool
	AnyNonPrimaryCitationContext    bool
	TargetMentionContradictsAbsence bool
	RequiresProductionProof         bool
}

func (p exactResolutionProof) anyPair(anchor string) bool {
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return false
	}
	for _, entry := range p.Entries {
		if entryCanEstablishAliasPair(p.Contract, entry, anchor) {
			return true
		}
	}
	return false
}

func (p exactResolutionProof) anyProductionPair(anchor string) bool {
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return false
	}
	for _, entry := range p.Entries {
		if entry.Production && entryCanEstablishAliasPair(p.Contract, entry, anchor) {
			return true
		}
	}
	return false
}

func collectExactResolutionProof(contract *types.ExactResolutionContract, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) exactResolutionProof {
	proof := exactResolutionProof{
		Contract:                contract,
		Entries:                 exactResolutionProofEntries(contract, citations, gc, ctx),
		RequiresProductionProof: types.ExactResolutionRequiresDefiningPrimaryProof(contract),
	}
	for _, entry := range proof.Entries {
		if !entry.FromEvidence && entry.Grounded {
			switch entry.ContextRole {
			case types.EvidenceContextRoleRelatedContext, types.EvidenceContextRoleDefining:
				proof.AnyNonPrimaryCitationContext = true
			case types.EvidenceContextRoleUnknown:
				if !entry.Production {
					proof.AnyNonPrimaryCitationContext = true
				}
			}
		}
		if !entry.Grounded {
			continue
		}
		if !entryMentionsAnyTarget(contract, entry) {
			continue
		}
		proof.AnyTarget = true
		if entry.Production {
			proof.AnyProductionTarget = true
			if entryCountsAsDefiningTargetProof(contract, entry) {
				proof.AnyProductionTargetAnchor = true
			}
		}
		if entryCountsAsDefiningTargetProof(contract, entry) {
			proof.AnyDefiningTargetProof = true
		}
	}
	proof.TargetMentionContradictsAbsence = proof.AnyDefiningTargetProof
	return proof
}

func exactResolutionProofEntries(contract *types.ExactResolutionContract, citations []types.Citation, gc *ground.Context, ctx *types.BusContext) []exactResolutionProofEntry {
	var out []exactResolutionProofEntry
	evidencePool := answerDocSurfaceEvidencePool(ctx)
	for _, item := range evidencePool {
		out = append(out, exactResolutionProofEntry{
			Source:       item.Source,
			Subject:      item.Subject,
			AnchorSymbol: item.AnchorSymbol,
			AnchorKind:   item.AnchorKind,
			Object:       item.Object,
			ContextRole:  item.ContextRole,
			Grounded:     item.GroundingStatus == types.GroundingGrounded || item.GroundingStatus == types.GroundingRecovered,
			Production:   types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, item.Source) && item.ContextRole != types.EvidenceContextRoleIllustrativeOnly && item.ContextRole != types.EvidenceContextRoleAbsenceSupport,
			DirectAnchor: types.ExactResolutionProofAnchorMatchesAnyTarget(contract, item.Subject, item.AnchorSymbol, item.Object),
			FromEvidence: true,
		})
	}
	for _, c := range citations {
		if c.File == "" || c.Line <= 0 {
			continue
		}
		lineText := strings.TrimSpace(c.Quote)
		if lineText == "" && gc != nil {
			if fileLines, ok := gc.LineIndex[c.File]; ok {
				lineText = strings.TrimSpace(fileLines[c.Line])
			}
		}
		if lineText != "" {
			matched := matchingEvidenceForCitation(evidencePool, c)
			out = append(out, exactResolutionProofEntry{
				LineText:     lineText,
				Source:       c.File,
				Subject:      matched.Subject,
				Object:       matched.Object,
				AnchorSymbol: matched.AnchorSymbol,
				AnchorKind:   matched.AnchorKind,
				ContextRole:  matched.ContextRole,
				Grounded:     true,
				Production:   citationCountsAsPrimaryProofSource(contract, c.File, matched),
				DirectAnchor: citationDirectlyAnchorsAnyTarget(contract, c, lineText, matched),
			})
		}
	}
	return out
}

func entryMentionsAnyTarget(contract *types.ExactResolutionContract, entry exactResolutionProofEntry) bool {
	if contract == nil {
		return false
	}
	if entryDirectlyAnchorsAnyTarget(contract, entry) {
		return true
	}
	if types.ExactResolutionTextsMentionAnyTarget(contract, exactResolutionProofIdentityTexts(entry)...) {
		return true
	}
	return entry.LineText != "" && types.ExactResolutionTextsMentionAnyTarget(contract, entry.LineText)
}

func entryMentionsAnchor(contract *types.ExactResolutionContract, entry exactResolutionProofEntry, anchor string) bool {
	if contract == nil {
		return false
	}
	for _, text := range exactResolutionProofIdentityTexts(entry) {
		if types.ExactResolutionTextMentionsTarget(contract, text, anchor) {
			return true
		}
	}
	return entry.LineText != "" && types.ExactResolutionTextMentionsTarget(contract, entry.LineText, anchor)
}

func entryDirectlyAnchorsAnyTarget(contract *types.ExactResolutionContract, entry exactResolutionProofEntry) bool {
	return entry.DirectAnchor || types.ExactResolutionProofAnchorMatchesAnyTarget(contract, entry.Subject, entry.AnchorSymbol, entry.Object)
}

func exactResolutionProofIdentityTexts(entry exactResolutionProofEntry) []string {
	return compactNonEmptyStrings(entry.Subject, entry.AnchorSymbol, entry.Object)
}

func entryCanEstablishAliasPair(contract *types.ExactResolutionContract, entry exactResolutionProofEntry, anchor string) bool {
	if contract == nil || !entry.Grounded || strings.TrimSpace(anchor) == "" {
		return false
	}
	if types.ExactResolutionRequiresDefiningPrimaryProof(contract) && !entry.Production {
		return false
	}
	if entry.FromEvidence && strings.TrimSpace(string(entry.AnchorKind)) == "" {
		return false
	}
	switch entry.ContextRole {
	case types.EvidenceContextRoleIllustrativeOnly,
		types.EvidenceContextRoleAbsenceSupport,
		types.EvidenceContextRoleRelatedContext:
		return false
	}
	identityTexts := exactResolutionProofIdentityTexts(entry)
	if types.ExactResolutionTextsMentionAnyTarget(contract, identityTexts...) && entryMentionsAnchor(contract, entry, anchor) {
		return true
	}
	if entry.LineText == "" {
		return false
	}
	if !types.ExactResolutionTextsMentionAnyTarget(contract, entry.LineText) || !types.ExactResolutionTextMentionsTarget(contract, entry.LineText, anchor) {
		return false
	}
	return entry.ContextRole == types.EvidenceContextRoleDefining || entryDirectlyAnchorsAnyTarget(contract, entry) || len(identityTexts) > 0
}

func compactNonEmptyStrings(items ...string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func entryCountsAsDefiningTargetProof(contract *types.ExactResolutionContract, entry exactResolutionProofEntry) bool {
	if contract == nil || !entry.Grounded {
		return false
	}
	if !entryMentionsAnyTarget(contract, entry) {
		return false
	}
	if types.ExactResolutionRequiresDefiningPrimaryProof(contract) && !entry.Production {
		return false
	}
	switch entry.ContextRole {
	case types.EvidenceContextRoleIllustrativeOnly,
		types.EvidenceContextRoleAbsenceSupport,
		types.EvidenceContextRoleRelatedContext:
		return false
	}
	if entry.FromEvidence && strings.TrimSpace(string(entry.AnchorKind)) == "" {
		return false
	}
	if entryDirectlyAnchorsAnyTarget(contract, entry) {
		return true
	}
	if entry.FromEvidence || entry.ContextRole != types.EvidenceContextRoleDefining {
		return false
	}
	return entry.LineText != "" && types.ExactResolutionTextsMentionAnyTarget(contract, entry.LineText)
}

func matchingEvidenceForCitation(items []types.EvidenceItem, c types.Citation) types.EvidenceItem {
	bestScore := math.MinInt
	best := types.EvidenceItem{}
	for _, item := range items {
		if item.Source != c.File || item.LineStart <= 0 {
			continue
		}
		lineEnd := item.LineEnd
		if lineEnd <= 0 {
			lineEnd = item.LineStart
		}
		if c.Line >= item.LineStart && c.Line <= lineEnd {
			score := 0
			switch item.GroundingStatus {
			case types.GroundingGrounded:
				score += 40
			case types.GroundingRecovered:
				score += 30
			case types.GroundingUngrounded:
				score += 5
			}
			if c.Line == item.LineStart {
				score += 8
			}
			if item.ContextRole == types.EvidenceContextRoleIllustrativeOnly {
				score -= 16
			}
			if item.ContextRole == types.EvidenceContextRoleAbsenceSupport {
				score += 4
			}
			if item.DiagramRole != types.EvidenceDiagramRoleUnknown {
				score += 6
			}
			if item.AnchorKind == types.AnchorDefinition {
				score += 2
			}
			if score > bestScore {
				bestScore = score
				best = item
			}
		}
	}
	return best
}

func citationCountsAsPrimaryProofSource(contract *types.ExactResolutionContract, source string, matched types.EvidenceItem) bool {
	if matched.Source != "" {
		return types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, matched.Source) &&
			matched.ContextRole != types.EvidenceContextRoleIllustrativeOnly &&
			matched.ContextRole != types.EvidenceContextRoleAbsenceSupport
	}
	return types.ExactResolutionSourceIsDefiningPrimaryProofLike(contract, source)
}

func citationDirectlyAnchorsAnyTarget(contract *types.ExactResolutionContract, c types.Citation, lineText string, matched types.EvidenceItem) bool {
	if matched.Source != "" {
		return types.ExactResolutionProofAnchorMatchesAnyTarget(contract, matched.Subject, matched.AnchorSymbol, matched.Object)
	}
	if contract == nil {
		return false
	}
	if contract.TargetKind == types.SubjectFilePath {
		for _, target := range contract.Targets {
			if types.ExactResolutionTextMentionsTarget(contract, c.File, target) {
				return true
			}
		}
	}
	return false
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
			"Re-emit `emit_answer_document` using only file:line anchors from the allowed list. If the current grounded evidence does not support an allowed citation for a point, either drop/renumber the offending citation_ref fields or keep that fact uncited in summary. Do not treat this finalizer retry as permission to reopen files or switch stages; a later exploration retry can surface missing files if the answer truly needs them.",
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
				Rationale: fmt.Sprintf("emit_answer_document cited %s:%d but file is not in the investigation's read-files list — a later exploration retry must read it before the final answer can cite it", c.File, c.Line),
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
	seenCitations := make(map[string]int, len(in))
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
		key := fmt.Sprintf("%s|%d|%s", p.cit.File, p.cit.Line, p.cit.Quote)
		if existing, ok := seenCitations[key]; ok {
			remap[i] = existing
			continue
		}
		remap[i] = len(kept)
		seenCitations[key] = remap[i]
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
	fieldPath := fmt.Sprintf("%s[%d].citation_ref", field, index)
	if field == "value" || field == "boolean" {
		fieldPath = field + ".citation_ref"
	}
	if ref < 0 {
		hint := fmt.Sprintf(
			"Re-emit `emit_answer_document` and fix ONLY `%s`: citation_ref is zero-based. Use `-1` for 'no citation' or a valid citations[] index from `0` to `%d`. Keep all other grounded fields unchanged and do not reopen files.",
			fieldPath, max(numCites-1, 0),
		)
		if numCites == 0 {
			hint = fmt.Sprintf(
				"Re-emit `emit_answer_document` and fix ONLY `%s`: the current citations[] pool is empty, so this field must be `-1` unless you can reuse already-grounded evidence to add citations[] entries in the same tool call and renumber every citation_ref against that zero-based pool. Do not reopen files.",
				fieldPath,
			)
		}
		return newAnswerDocValidationError(
			"citation_ref_range",
			"%s %d is negative; use -1 for 'no citation' or a valid zero-based citations[] index",
			fieldPath, ref,
		).WithFields(fieldPath).
			WithMetadata("citation_ref_field", fieldPath).
			WithMetadata("citation_ref_actual", strconv.Itoa(ref)).
			WithMetadata("citation_ref_pool_size", strconv.Itoa(numCites)).
			WithHint(hint)
	}
	if ref >= numCites {
		hint := fmt.Sprintf(
			"Re-emit `emit_answer_document` and fix ONLY `%s`: citation_ref is zero-based. The current citations[] pool has %d entries, so valid indices are `0` through `%d`, or `-1` for 'no citation'. Keep the existing grounded evidence and renumber only the offending citation_ref fields; do not reopen files.",
			fieldPath, numCites, max(numCites-1, 0),
		)
		if numCites == 0 {
			hint = fmt.Sprintf(
				"Re-emit `emit_answer_document` and fix ONLY `%s`: the current citations[] pool is empty, so this field must be `-1` unless you can reuse already-grounded evidence to add citations[] entries in the same tool call and then renumber the citation_ref against that zero-based pool. Do not reopen files.",
				fieldPath,
			)
		}
		return newAnswerDocValidationError(
			"citation_ref_range",
			"%s %d is out of range (citations pool has %d entries)",
			fieldPath, ref, numCites,
		).WithFields(fieldPath).
			WithMetadata("citation_ref_field", fieldPath).
			WithMetadata("citation_ref_actual", strconv.Itoa(ref)).
			WithMetadata("citation_ref_pool_size", strconv.Itoa(numCites)).
			WithHint(hint)
	}
	return nil
}

func validateRequestedEnumerationBoundary(shape types.AnswerShape, p *emitAnswerDocumentParams, ctx *types.BusContext) error {
	if p == nil || ctx == nil {
		return nil
	}
	plan := answerSurfacePlan(ctx)
	if plan == nil || plan.RequestedEnumerationBoundary == nil || plan.RequestedEnumerationBoundary.DeclaredCount <= 0 {
		return nil
	}
	boundary := plan.RequestedEnumerationBoundary
	switch shape {
	case types.ShapeStepList:
		if len(p.Steps) <= boundary.DeclaredCount {
			return nil
		}
		return newAnswerDocValidationError(
			"requested_set_boundary",
			"the user explicitly requested a bounded principal set `%s` (%d item(s)), so shape=step_list must keep steps[] to at most %d principal item(s). Move extra adjacent guards/helpers into summary prose or a short caveat instead of extending the main ordered list to %d item(s).",
			boundary.SourceQuote, boundary.DeclaredCount, boundary.DeclaredCount, len(p.Steps),
		).
			WithFields("steps", "summary").
			WithHint("Re-emit `emit_answer_document` with the same shape and evidence, but keep the main `steps[]` sequence within the user-declared boundary. Do NOT simply truncate to the first N chronological lines when grounded comments or summaries indicate some nearby items are auxiliary guards/coherence/repair checks. Keep the principal ordered set in `steps[]`; move auxiliary adjacent items into `summary` as a caveat if they are still relevant.")
	case types.ShapeListOfSymbols:
		if len(p.Symbols) <= boundary.DeclaredCount {
			return nil
		}
		return newAnswerDocValidationError(
			"requested_set_boundary",
			"the user explicitly requested a bounded principal set `%s` (%d item(s)), so shape=list_of_symbols must keep symbols[] to at most %d principal item(s). Move extra adjacent symbols into summary prose instead of extending the main slate to %d item(s).",
			boundary.SourceQuote, boundary.DeclaredCount, boundary.DeclaredCount, len(p.Symbols),
		).
			WithFields("symbols", "summary", "symbols_completeness").
			WithHint("Re-emit `emit_answer_document` with the same shape, but keep `symbols[]` within the user-declared boundary. Do NOT simply take the first N nearby names if grounded summaries/comments mark some of them as auxiliary or caveat-only; keep the principal bounded set in `symbols[]` and move secondary neighbors into `summary` if needed.")
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
