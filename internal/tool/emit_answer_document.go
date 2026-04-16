package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
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
	Decision    string `json:"decision"`
	Rationale   string `json:"rationale"`
	CitationRef int    `json:"citation_ref"`
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
		"inside the per-trace WorkDir. 'summary' is the only LLM-prose field, capped at 500 chars — " +
		"use it for the one-sentence lead-in, not for the answer body. Unknown fields or shape-field " +
		"mismatches are REJECTED with a clear error."
}

func (t *EmitAnswerDocument) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "shape": {"type": "string", "enum": ["list_of_symbols", "step_list", "value", "boolean", "config_value", "explanation"], "description": "Closed enum of answer shapes. REQUIRED. Choose the shape the analyzer declared in the prompt's AnswerContract section."},
    "summary": {"type": "string", "description": "LLM-authored lead-in prose, 1-2 sentences, ≤500 chars. The only prose field. REQUIRED for 'explanation' shape, optional for all others."},
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
      "description": "Answer-symbol list for shape=list_of_symbols. REQUIRED for list_of_symbols; must be empty for all other shapes.",
      "items": {
        "type": "object",
        "properties": {
          "name":      {"type": "string"},
          "file":      {"type": "string"},
          "line":      {"type": "integer"},
          "kind":      {"type": "string", "enum": ["function", "func", "method", "type", "struct", "class", "const", "var"]},
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
      "description": "Shared citation pool. Each entry is one file:line anchor with an optional verbatim quote. Zero-based indices; CitationRef=-1 means 'no citation'.",
      "items": {
        "type": "object",
        "properties": {
          "file":  {"type": "string", "description": "Repository-relative file path. MUST NOT live inside the per-trace WorkDir (blob directory)."},
          "line":  {"type": "integer", "description": "Gutter line number from read_file output. Must be > 0."},
          "quote": {"type": "string", "description": "Optional verbatim snippet from the cited location, ≤200 chars."}
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
}`)
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
	if ctx.AnalysisIR != nil {
		target := ctx.AnalysisIR.AnswerContract.RequiredAnswerShape
		if target != "" && target != shape {
			logging.Warning("[emit_answer_document] LLM chose shape=%s but AnalysisIR target is %s — auto-correcting", shape, target)
			// Check if the LLM provided the required fields for the
			// target shape. If not, fall back to explanation (which only
			// needs summary) instead of entering a retry loop where the
			// LLM keeps filling the wrong shape's fields.
			canCorrect := true
			switch target {
			case types.ShapeListOfSymbols:
				canCorrect = len(p.Symbols) > 0
			case types.ShapeStepList:
				canCorrect = len(p.Steps) > 0
			case types.ShapeValue, types.ShapeConfigValue:
				canCorrect = p.Value != nil && p.Value.Literal != ""
			case types.ShapeBoolean:
				canCorrect = p.Boolean != nil && p.Boolean.Decision != ""
			}
			if canCorrect {
				shape = target
			} else {
				logging.Warning("[emit_answer_document] target shape %s requires fields the LLM didn't fill — falling back to explanation", target)
				shape = types.ShapeExplanation
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

	if len(p.Summary) > types.AnswerDocumentMaxSummaryChars {
		return failEmit(t.Name(), now,
			"summary length %d exceeds cap %d — Summary is the only prose field and must stay brief",
			len(p.Summary), types.AnswerDocumentMaxSummaryChars)
	}

	workDir := strings.TrimSpace(ctx.WorkDir)
	citations, cerr := buildEmitAnswerDocumentCitations(p.Citations, workDir)
	if cerr != nil {
		return failEmit(t.Name(), now, "%v", cerr)
	}
	numCites := len(citations)

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
		scrubForbiddenFields(&p, shape, forbidSteps|forbidValue|forbidBoolean)
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
		scrubForbiddenFields(&p, shape, forbidSymbols|forbidValue|forbidBoolean)
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
		scrubForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidBoolean)
		if p.Value == nil {
			return failEmit(t.Name(), now, "shape=value requires value{literal, citation_ref}")
		}
		if err := validateValueField(p.Value, false, numCites); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		doc.Value = p.Value

	case types.ShapeConfigValue:
		scrubForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidBoolean)
		if p.Value == nil {
			return failEmit(t.Name(), now, "shape=config_value requires value{key, literal, citation_ref}")
		}
		if err := validateValueField(p.Value, true, numCites); err != nil {
			return failEmit(t.Name(), now, "%v", err)
		}
		doc.Value = p.Value

	case types.ShapeBoolean:
		scrubForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidValue)
		if p.Boolean == nil {
			return failEmit(t.Name(), now, "shape=boolean requires boolean{decision, rationale, citation_ref}")
		}
		bl, berr := buildEmitAnswerDocumentBoolean(p.Boolean, numCites)
		if berr != nil {
			return failEmit(t.Name(), now, "%v", berr)
		}
		doc.Boolean = bl

	case types.ShapeExplanation:
		scrubForbiddenFields(&p, shape, forbidSteps|forbidSymbols|forbidValue|forbidBoolean)
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
		Summary:   renderEmitAnswerDocumentSummary(doc),
		Timestamp: now,
	}, nil
}

// forbidBitset marks which AnswerDocument payload fields must be
// empty for a given shape. Each shape-dispatch branch ORs together
// the fields it does NOT accept and passes the mask to
// validateEmitAnswerDocumentNoForbidden. Keeping the rule as a
// bitset (instead of a per-shape switch inside the validator) means
// each shape's forbidden-field list is literally visible at its
// branch in Execute, so a shape's contract is one-grep obvious.
// Note: forbidden fields are silently SCRUBBED (not rejected) by
// scrubForbiddenFields to prevent LLM infinite retry loops.
type forbidBitset uint

const (
	forbidSteps   forbidBitset = 1 << iota
	forbidSymbols
	forbidValue
	forbidBoolean
)

// scrubForbiddenFields silently clears fields that are forbidden for
// the given shape. LLMs persistently include cross-shape fields
// (e.g. boolean{} on a list_of_symbols call) and cannot fix the
// issue through retries. Logging the scrub for diagnostics but
// accepting the call prevents infinite retry loops.
func scrubForbiddenFields(p *emitAnswerDocumentParams, shape types.AnswerShape, mask forbidBitset) {
	if mask&forbidSteps != 0 && len(p.Steps) > 0 {
		logging.Debug("[emit_answer_document] scrubbing forbidden steps[] (len=%d) for shape=%s", len(p.Steps), shape)
		p.Steps = nil
	}
	if mask&forbidSymbols != 0 && len(p.Symbols) > 0 {
		logging.Debug("[emit_answer_document] scrubbing forbidden symbols[] (len=%d) for shape=%s", len(p.Symbols), shape)
		p.Symbols = nil
	}
	if mask&forbidValue != 0 && p.Value != nil {
		logging.Debug("[emit_answer_document] scrubbing forbidden value{} for shape=%s", shape)
		p.Value = nil
	}
	if mask&forbidBoolean != 0 && p.Boolean != nil {
		logging.Debug("[emit_answer_document] scrubbing forbidden boolean{} for shape=%s", shape)
		p.Boolean = nil
	}
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
	if err := validateCitationRef("boolean", 0, in.CitationRef, numCites); err != nil {
		return nil, err
	}
	return &types.AnswerBoolean{
		Decision:    decision,
		Rationale:   rationale,
		CitationRef: in.CitationRef,
	}, nil
}

func buildEmitAnswerDocumentCitations(in []types.Citation, workDir string) ([]types.Citation, error) {
	if len(in) == 0 {
		return nil, nil
	}
	for i := range in {
		c := &in[i]
		c.File = strings.TrimSpace(c.File)
		if c.File == "" {
			return nil, fmt.Errorf("citations[%d]: file is required", i)
		}
		if !emitLooksLikePath(c.File) {
			return nil, fmt.Errorf("citations[%d]: file %q does not look like a repo-relative file path", i, c.File)
		}
		if isInsideWorkDir(c.File, workDir) {
			return nil, fmt.Errorf("citations[%d]: file %q lives inside the per-trace WorkDir (%s) — that is a tool-output blob, not a repo file", i, c.File, workDir)
		}
		if c.Line <= 0 {
			return nil, fmt.Errorf("citations[%d]: line must be > 0 (got %d) — Pattern 2 line-hallucination guard", i, c.Line)
		}
		c.Quote = strings.TrimSpace(c.Quote)
		if len(c.Quote) > types.CitationMaxQuoteChars {
			return nil, fmt.Errorf("citations[%d]: quote length %d exceeds cap %d", i, len(c.Quote), types.CitationMaxQuoteChars)
		}
	}
	return in, nil
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

func renderEmitAnswerDocumentSummary(doc *types.AnswerDocument) string {
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
	return b.String()
}
