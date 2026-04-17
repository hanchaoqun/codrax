package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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
//     and it is length-capped at AnswerDocumentMaxSummaryChars.
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
	Shape               string                       `json:"shape"`
	Summary             string                       `json:"summary"`
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
	return "Emit the final answer as a structured AnswerDocument in ONE call per finalizer dispatch. " +
		"Choose 'shape' from: list_of_symbols, step_list, value, boolean, config_value, explanation. " +
		"Shape-specific required fields: list_of_symbols → symbols[] + symbols_completeness; " +
		"step_list → steps[]; value → value{literal}; config_value → value{key,literal}; " +
		"boolean → boolean{decision,rationale}; explanation → summary. The 'citations' array is a " +
		"SHARED POOL; every steps[].citation_ref / value.citation_ref / boolean.citation_ref / " +
		"symbols is an integer INDEX into that pool (zero-based), or -1 when no citation backs " +
		"the entry. Every citation MUST have file (repo-relative), line > 0, and file must NOT live " +
		"inside the per-trace WorkDir. 'summary' is the only LLM-prose field, with a per-shape " +
		"length cap: explanation → up to 2500 chars (Summary IS the answer body — write a " +
		"thorough multi-paragraph explanation), list_of_symbols / step_list / boolean → ≤500 " +
		"chars lead-in, value / config_value → ≤300 chars lead-in. " +
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
	// symbols[].kind enum is sourced from types.AnswerSymbolKindSchemaEnum
	// so schema stays in lockstep with emit_answer_symbol's validator.
	return json.RawMessage(fmt.Sprintf(`{
  "type": "object",
  "properties": {
    "shape": {"type": "string", "enum": ["list_of_symbols", "step_list", "value", "boolean", "config_value", "explanation"], "description": "Closed enum of answer shapes. REQUIRED. Choose the shape the analyzer declared in the prompt's AnswerContract section."},
    "summary": {"type": "string", "description": "LLM-authored prose. Per-shape cap: explanation ≤2500 chars (Summary IS the answer body — multi-paragraph OK), list_of_symbols / step_list / boolean ≤500 chars (lead-in only), value / config_value ≤300 chars (lead-in only). REQUIRED for 'explanation'; optional for all others."},
    "steps": {
      "type": "array",
      "description": "Ordered steps for shape=step_list. REQUIRED for step_list; must be empty for all other shapes.",
      "items": {
        "type": "object",
        "properties": {
          "index":         {"type": "integer", "description": "1-based step number. Must be positive."},
          "description":   {"type": "string", "description": "One-sentence step body drawn from evidence. Do not collapse two steps into one."},
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
          "rationale": {"type": "string"}
        },
        "required": ["name", "file", "line", "kind"]
      }
    },
    "symbols_completeness": {"type": "string", "enum": ["", "complete", "lower_bound", "unknown"], "description": "Set-level authority for the symbols slate. REQUIRED when shape=list_of_symbols. 'complete' is validated at finalizer-time against Turn A's TerminalEvidenceCount and analyzer's MustInclude; mismatches downgrade to 'lower_bound'."},
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
        "rationale":    {"type": "string", "description": "One-sentence evidence citation for the decision."},
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
          "quote": {"type": "string", "description": "OPTIONAL verbatim copy of the code at file:line from read_file, ≤200 chars. Rule: paste the literal source line, or LEAVE THIS FIELD EMPTY. Do NOT write prose, summaries, or paraphrases — the grounder compares quote tokens against the actual line text and strips any quote that does not overlap (prose will be automatically cleared). If you cannot paste the literal line, omit the field."}
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
}`, types.AnswerSymbolKindSchemaEnum()))
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
				}
				canCorrect = p.Value != nil && p.Value.Literal != ""
			case types.ShapeBoolean:
				canCorrect = p.Boolean != nil && p.Boolean.Decision != ""
			}
			if canCorrect {
				shape = target
				shapeCorrectionNote = fmt.Sprintf("shape auto-corrected: LLM chose %s → resolved to %s (analyzer target)", llmShape, shape)
			} else {
				logging.Warning("[emit_answer_document] target shape %s requires fields the LLM didn't fill — falling back to explanation", target)
				shape = types.ShapeExplanation
				shapeCorrectionNote = fmt.Sprintf("shape auto-corrected: LLM chose %s, target was %s but required fields empty → fell back to explanation", llmShape, target)
			}
			// Scrub fields forbidden by the resolved shape.
			switch shape {
			case types.ShapeListOfSymbols:
				p.Steps = nil; p.Value = nil; p.Boolean = nil
			case types.ShapeStepList:
				p.Symbols = nil; p.Value = nil; p.Boolean = nil
			case types.ShapeValue, types.ShapeConfigValue:
				p.Steps = nil; p.Symbols = nil; p.Boolean = nil
			case types.ShapeBoolean:
				p.Steps = nil; p.Symbols = nil; p.Value = nil
			case types.ShapeExplanation:
				p.Steps = nil; p.Symbols = nil; p.Value = nil; p.Boolean = nil
			}
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

	// Shape-tiered Summary cap. explanation shape uses Summary as
	// the answer body and gets a generous ceiling (2500 chars, room
	// for 4-6 paragraphs); other shapes use Summary as a lead-in
	// and keep the historical tighter limit. See SummaryCapByShape
	// for the per-shape table. The prior single-value 500-char cap
	// contradicted the skill prompt's "no character limit for
	// explanation" clause and forced the finalizer to burn retries
	// trimming legitimate prose.
	summaryCap := types.SummaryCapFor(shape)
	if len(p.Summary) > summaryCap {
		return failEmit(t.Name(), now,
			"summary length %d exceeds cap %d for shape=%s — shorten the summary; cap is per-shape (see SummaryCapByShape)",
			len(p.Summary), summaryCap, shape)
	}

	workDir := strings.TrimSpace(ctx.WorkDir)
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
	citations, citationRemap, citationWarnings, cerr := buildEmitAnswerDocumentCitations(p.Citations, workDir, groundCtx, readFiles)
	if cerr != nil {
		return failEmit(t.Name(), now, "%v", cerr)
	}
	applyCitationRemap(&p, citationRemap)
	numCites := len(citations)
	if dropped := countDroppedCitations(citationRemap); dropped > 0 || len(citationWarnings) > 0 {
		logging.Warning("[emit_answer_document] citation grounding: kept=%d dropped=%d warnings=%d", numCites, dropped, len(citationWarnings))
	}

	doc := &types.AnswerDocument{
		Shape:     shape,
		Summary:   strings.TrimSpace(p.Summary),
		Citations: citations,
	}

	// Shape-dispatch: each branch validates its own required fields,
	// rejects fields that do not belong to this shape, and populates
	// the AnswerDocument slot the renderer will read.
	switch shape {
	case types.ShapeListOfSymbols:
		if err := rejectForbiddenFields(&p, shape, forbidSteps|forbidValue|forbidBoolean); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		if len(p.Symbols) == 0 {
			return failEmit(t.Name(), now, "shape=list_of_symbols requires symbols[] with at least one entry")
		}
		claimRaw := strings.ToLower(strings.TrimSpace(p.SymbolsCompleteness))
		if claimRaw == "" {
			return failEmit(t.Name(), now, "shape=list_of_symbols requires symbols_completeness (one of: complete, lower_bound, unknown)")
		}
		claim, cok := emitAnswerSymbolAllowedCompleteness[claimRaw]
		if !cok {
			return failEmit(t.Name(), now, "unknown symbols_completeness value %q (allowed: complete, lower_bound, unknown)", p.SymbolsCompleteness)
		}
		built := make([]types.AnswerSymbol, 0, len(p.Symbols))
		for i, in := range p.Symbols {
			sym, perr := buildEmitAnswerSymbolItem(in, i, workDir)
			if perr != nil {
				return failEmit(t.Name(), now, "symbols[%d]: %v", i, perr)
			}
			built = append(built, sym)
		}
		doc.Symbols = built
		doc.SymbolsCompleteness = claim

	case types.ShapeStepList:
		if err := rejectForbiddenFields(&p, shape, forbidSymbols|forbidValue|forbidBoolean); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		if len(p.Steps) == 0 {
			return failEmit(t.Name(), now, "shape=step_list requires steps[] with at least one entry")
		}
		for i := range p.Steps {
			if err := validateStep(&p.Steps[i], i, numCites); err != nil {
				return failEmit(t.Name(), now, "%v", err)
			}
			p.Steps[i].Description = strings.TrimSpace(p.Steps[i].Description)
		}
		doc.Steps = p.Steps

	case types.ShapeValue:
		if err := rejectForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidBoolean); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		if p.Value == nil {
			return failEmit(t.Name(), now, "shape=value requires value{literal, citation_ref}")
		}
		if err := validateValueField(p.Value, false, numCites); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		doc.Value = p.Value

	case types.ShapeConfigValue:
		if err := rejectForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidBoolean); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		if p.Value == nil {
			return failEmit(t.Name(), now, "shape=config_value requires value{key, literal, citation_ref}")
		}
		if err := validateValueField(p.Value, true, numCites); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		doc.Value = p.Value

	case types.ShapeBoolean:
		if err := rejectForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidValue); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		if p.Boolean == nil {
			return failEmit(t.Name(), now, "shape=boolean requires boolean{decision, rationale, citation_ref}")
		}
		bl, berr := buildEmitAnswerDocumentBoolean(p.Boolean, numCites)
		if berr != nil {
			return failEmit(t.Name(), now, "%v", berr)
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
		if err := rejectForbiddenFields(&p, shape, forbidSteps|forbidValue|forbidBoolean); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		if len(p.Symbols) > 0 {
			built := make([]types.AnswerSymbol, 0, len(p.Symbols))
			for i, in := range p.Symbols {
				sym, perr := buildEmitAnswerSymbolItem(in, i, workDir)
				if perr != nil {
					return failEmit(t.Name(), now, "symbols[%d]: %v", i, perr)
				}
				built = append(built, sym)
			}
			doc.Symbols = built
			// Completeness is not part of the explanation-skeleton
			// contract; leave SymbolsCompleteness zero-valued so the
			// renderer can branch on "skeleton vs. enumeration".
		}
		if strings.TrimSpace(p.Summary) == "" {
			return failEmit(t.Name(), now, "shape=explanation requires a non-empty summary")
		}
	}

	if len(p.Caveats) > 0 {
		doc.Caveats = append([]string(nil), p.Caveats...)
	}

	ctx.Mutable.SetAnswerDocument(doc)

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   true,
		Summary:   renderEmitAnswerDocumentSummary(doc, citationWarnings, shapeCorrectionNote),
		Timestamp: now,
	}, nil
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
		set[strings.TrimSpace(f)] = true
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
	forbidSteps   forbidBitset = 1 << iota
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
//   Valid + QuoteMatched  → kept as-is.
//   Valid + !QuoteMatched → kept with Quote cleared (prevents a
//                           fabricated excerpt from reaching the user
//                           while preserving the real file:line anchor).
//   !Valid                → dropped from the pool; any downstream
//                           CitationRef that pointed at it is remapped
//                           to CitationRefUnset (-1) by the caller.
//
// Returns (kept, remap, warnings, err):
//   - kept:  the surviving citations in the new pool (may be shorter
//            than `in` if some were dropped).
//   - remap: old-index → new-index, or -1 for dropped entries. Always
//            len(remap) == len(in) so callers can mapRef() safely.
//   - warnings: per-citation messages surfaced in the tool Summary so
//            the LLM can correct on the next turn.
//   - err:   only set on a STRUCTURAL error (empty file, bad path,
//            line <= 0, quote too long, blob-leak) that should
//            reject the whole call; grounding failures never return
//            err — they are surfaced via drop/clear + warnings.
func buildEmitAnswerDocumentCitations(in []types.Citation, workDir string, gc *ground.Context, readFiles map[string]bool) ([]types.Citation, []int, []string, error) {
	if len(in) == 0 {
		return nil, nil, nil, nil
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
			return nil, nil, nil, fmt.Errorf("citations[%d]: file is required", i)
		}
		if !emitLooksLikePath(c.File) {
			return nil, nil, nil, fmt.Errorf("citations[%d]: file %q does not look like a repo-relative file path", i, c.File)
		}
		if isInsideWorkDir(c.File, workDir) {
			return nil, nil, nil, fmt.Errorf("citations[%d]: file %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file", i, c.File, workDir)
		}
		// Whitelist check (session 8): citation file must be one
		// Turn A actually read. Skipped when readFiles set is empty
		// (test path / no TurnA). Dropped item goes through the
		// standard per-citation drop path; the warning names both
		// the offending file AND the allowed set so the next turn
		// can correct in one shot.
		if len(readFiles) > 0 && !readFiles[c.File] {
			warnings = append(warnings,
				fmt.Sprintf("citations[%d] dropped (%s:%d) — file not in Turn A's ReadFiles list; %s. Cite ONLY files Turn A read, or reject the answer",
					i, c.File, c.Line, allowedHint))
			continue
		}
		if c.Line <= 0 {
			return nil, nil, nil, fmt.Errorf("citations[%d]: line must be > 0 (got %d) — Pattern 2 line-hallucination guard", i, c.Line)
		}
		c.Quote = strings.TrimSpace(c.Quote)
		if len(c.Quote) > types.CitationMaxQuoteChars {
			return nil, nil, nil, fmt.Errorf("citations[%d]: quote length %d exceeds cap %d", i, len(c.Quote), types.CitationMaxQuoteChars)
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
	return kept, remap, warnings, nil
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
