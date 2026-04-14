package types

// answer_document.go — P2.2 structured answer payload.
//
// AnswerDocument is the typed replacement for the finalizer's free
// prose output. Under answer_document_mode=on, the finalizer LLM emits
// an AnswerDocument via the emit_answer_document tool call instead of
// writing prose directly, and a deterministic renderer
// (internal/render/answerdoc.go) converts the struct into user-visible
// prose keyed on BusContext language.
//
// Design target (docs/architecture-root-cause-remediation.md §6 P2.2):
// close R1 at the finalizer layer. The four fake-green patterns become
// structurally impossible:
//
//   1. step_list collapse → Steps is a slice; LLM cannot collapse a
//      slice into a paragraph without dropping items the schema
//      requires
//   2. line hallucination → Citation.Line must be > 0 and its File
//      must be in BusContext.AllowedFiles / outside WorkDir
//   3. sibling drift → Citations are a central pool; CitationRef
//      integers index into it, so every cited file/line is grounded
//      once, not re-rendered per step
//   4. prose → fact → every structural field except Summary is
//      required to come from evidence; Summary is the one LLM-authored
//      prose escape hatch, length-capped
//
// The Summary field is intentionally the ONLY prose escape hatch. All
// other user-visible content is assembled by the renderer from the
// typed fields below.

// AnswerDocument is the structured final-answer payload the finalizer
// emits via the emit_answer_document tool. Each shape-specific field
// is populated only when AnswerDocument.Shape matches that shape; the
// emit_answer_document schema validator enforces the per-shape
// required-field contract.
//
// Field conventions:
//
//   - Shape: closed enum drawn from types.AnswerShape
//   - Summary: LLM-authored 1-2 sentence prose, max 500 chars
//   - Steps: non-empty for ShapeStepList, empty for all other shapes
//   - Symbols: non-empty for ShapeListOfSymbols
//   - SymbolsCompleteness: set-level authority (see CompletenessClaim);
//     only meaningful when Shape == ShapeListOfSymbols. Reuses the
//     P2.1 three-level ladder so the existing cardinality validator
//     (extractor.validateCompletenessClaim) can be replayed verbatim
//     at finalize-time against AnswerDocument.Symbols.
//   - Value: non-nil for ShapeValue and ShapeConfigValue
//   - Boolean: non-nil for ShapeBoolean
//   - Citations: shared pool of file:line pointers. Each typed field
//     above carries an integer CitationRef index into this slice so
//     the same citation can be referenced from multiple places
//     without duplication
//   - Caveats: honesty/ungrounded markers the renderer surfaces at
//     the bottom of the prose
//
// ShapeExplanation is the free-form fallback: only Summary +
// Citations are required, Steps/Symbols/Value/Boolean are all empty.
// Use it for general-explanation questions that do not fit any of the
// structured shapes.
type AnswerDocument struct {
	Shape   AnswerShape `json:"shape"`
	Summary string      `json:"summary,omitempty"`

	Steps []AnswerStep `json:"steps,omitempty"`

	Symbols             []AnswerSymbol    `json:"symbols,omitempty"`
	SymbolsCompleteness CompletenessClaim `json:"symbols_completeness,omitempty"`

	Value   *AnswerValue   `json:"value,omitempty"`
	Boolean *AnswerBoolean `json:"boolean,omitempty"`

	Citations []Citation `json:"citations,omitempty"`
	Caveats   []string   `json:"caveats,omitempty"`
}

// AnswerDocumentMaxSummaryChars caps the LLM-authored Summary field.
// The finalizer's prompt instructs the LLM to keep Summary to one or
// two sentences; this cap is the structural enforcement. Values above
// the cap are rejected by the emit_answer_document schema validator.
//
// 500 chars is chosen because two typical sentences in English or
// Chinese fit comfortably under 400; 500 gives room for dense
// technical phrasing without opening Summary as a prose escape.
const AnswerDocumentMaxSummaryChars = 500

// CitationRefUnset is the sentinel value used when a typed field
// (AnswerStep, AnswerValue, AnswerBoolean) has no backing citation in
// the pool. The renderer renders it as "no citation"; the tool schema
// allows it ONLY when the producing LLM explicitly sets -1 via the
// JSON input. Zero is NOT the sentinel because zero is a valid index
// into a single-citation pool — this distinction is critical for the
// schema validator to flag "forgot to set citation_ref" errors.
const CitationRefUnset = -1

// AnswerStep is one numbered entry in a ShapeStepList answer. The
// Description field carries LLM-authored prose for the step's body;
// the CitationRef integer points at a Citation in the document-level
// pool. Index is 1-based and set by the LLM (the renderer preserves
// it verbatim to allow out-of-order declarations).
type AnswerStep struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	CitationRef int    `json:"citation_ref"`
}

// AnswerValue carries a single concrete value answer. Literal is the
// verbatim string from the evidence (the renderer emits it in quotes
// when appropriate). Key is the config-key path for ShapeConfigValue;
// empty for plain ShapeValue answers.
type AnswerValue struct {
	Key         string `json:"key,omitempty"`
	Literal     string `json:"literal"`
	CitationRef int    `json:"citation_ref"`
}

// AnswerBoolean carries a YES/NO answer plus a one-sentence rationale.
// The renderer prefixes Rationale with a language-specific YES/NO
// word drawn from Decision, so the LLM cannot hedge by supplying
// "it depends" in Rationale.
type AnswerBoolean struct {
	Decision    bool   `json:"decision"`
	Rationale   string `json:"rationale"`
	CitationRef int    `json:"citation_ref"`
}

// Citation is one entry in the AnswerDocument-level citation pool.
// The emit_answer_document schema requires File to be non-empty and
// Line > 0; files inside BusContext.WorkDir are rejected (the blob
// leak gate shared with emit_answer_symbol). Quote is optional free
// text ≤ CitationMaxQuoteChars, useful when the renderer wants to
// surface the exact snippet the citation anchors on.
type Citation struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Quote string `json:"quote,omitempty"`
}

// CitationMaxQuoteChars bounds the optional Quote field. Long quotes
// defeat the structural goal by giving the LLM a prose escape route;
// 200 chars is enough for one dense code line.
const CitationMaxQuoteChars = 200

// IsZero reports whether the document is fully empty — no shape set
// and no fields populated. Used by the finalizer's ParseOutput to
// decide whether a missing emit_answer_document call counts as "LLM
// refused to emit" (retry required) or "genuinely empty answer"
// (leaves FinalAnswer empty).
func (d *AnswerDocument) IsZero() bool {
	if d == nil {
		return true
	}
	if d.Shape != "" {
		return false
	}
	if d.Summary != "" {
		return false
	}
	if len(d.Steps) > 0 || len(d.Symbols) > 0 || len(d.Citations) > 0 || len(d.Caveats) > 0 {
		return false
	}
	if d.Value != nil || d.Boolean != nil {
		return false
	}
	return true
}

// CloneAnswerDocument returns a defensive deep copy of d. Used by the
// MutableState accessor so external callers cannot mutate the
// internal snapshot in place. Nil input returns nil.
func CloneAnswerDocument(d *AnswerDocument) *AnswerDocument {
	if d == nil {
		return nil
	}
	out := *d
	if d.Steps != nil {
		out.Steps = append([]AnswerStep(nil), d.Steps...)
	}
	if d.Symbols != nil {
		out.Symbols = append([]AnswerSymbol(nil), d.Symbols...)
	}
	if d.Citations != nil {
		out.Citations = append([]Citation(nil), d.Citations...)
	}
	if d.Caveats != nil {
		out.Caveats = append([]string(nil), d.Caveats...)
	}
	if d.Value != nil {
		v := *d.Value
		out.Value = &v
	}
	if d.Boolean != nil {
		bl := *d.Boolean
		out.Boolean = &bl
	}
	return &out
}
