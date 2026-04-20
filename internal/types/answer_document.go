package types

// answer_document.go — structured answer payload.
//
// AnswerDocument is the typed replacement for the finalizer's free
// prose output. The finalizer LLM emits an AnswerDocument via the
// emit_answer_document tool call, and a deterministic renderer
// (internal/render/answerdoc.go) converts the struct into
// user-visible prose keyed on BusContext language.
//
// Design target: close R1 at the finalizer layer. The four
// fake-green patterns become structurally impossible:
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

	// Snippets are short code excerpts (±2 lines around each
	// citation, clustered when adjacent) extracted deterministically
	// from the read_file gutter index. Populated by
	// emit_answer_document post-dispatch; the LLM never writes into
	// this field. Rendered below the Summary and above the Citations
	// pool so readers can see the relevant code without following
	// file:line links.
	Snippets []CodeSnippet `json:"snippets,omitempty"`
}

// CodeSnippet carries a contiguous code excerpt extracted from a
// file that was actually read during Turn A. File+line range keys
// it for display; Code is the verbatim text with a leading gutter
// matching the read_file format so diff-grep style searches on
// generated answers remain useful.
type CodeSnippet struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	// Language is a best-effort tag derived from the file extension
	// (go, py, js, ...) so the renderer can emit language-tagged
	// fenced blocks. Empty when no match; renderer falls back to
	// an untagged fence.
	Language string `json:"language,omitempty"`
	Code     string `json:"code"`
}

// AnswerDocumentMaxSummaryChars caps the LLM-authored Summary field
// for shapes where Summary is a lead-in, not the answer body. Kept as
// a top-level const for backwards compatibility with tests that
// explicitly reference the 500-char floor; new code should call
// SummaryCapFor(shape) which dispatches off SummaryCapByShape.
const AnswerDocumentMaxSummaryChars = 500

// SummaryCapByShape is the per-shape Summary length ceiling enforced
// by emit_answer_document. Different shapes use Summary for different
// purposes so a single number misserves some of them:
//
//   - ShapeExplanation: Summary IS the answer body. The LLM is
//     instructed to produce a thorough, multi-paragraph explanation
//     with code-level specifics, cross-file relationships, and
//     mechanism details. 2500 chars comfortably fits 4-6 paragraphs
//     plus a Mermaid diagram without opening summary as an unbounded
//     prose escape. The earlier 500-char limit contradicted the
//     skill prompt's "no character limit" clause and forced the
//     finalizer to burn retry budget trimming a 900-char answer down
//     to a 440-char summary that dropped half the content.
//
//   - ShapeValue / ShapeConfigValue: Summary is a 1-sentence lead-in
//     before a scalar literal. 300 chars keeps it tight.
//
//   - ShapeListOfSymbols / ShapeStepList / ShapeBoolean: Summary is
//     a 1-3 sentence lead-in before structured payload (the list,
//     the step sequence, the boolean+rationale). 500 chars matches
//     the old cap and works well for these shapes.
//
// ShapeNone is absent from the map by design — no-shape answers are
// rejected upstream.
var SummaryCapByShape = map[AnswerShape]int{
	ShapeExplanation: 2500,
	ShapeValue:       300,
	ShapeConfigValue: 300,
	ShapeListOfSymbols: AnswerDocumentMaxSummaryChars,
	ShapeStepList:      AnswerDocumentMaxSummaryChars,
	ShapeBoolean:       AnswerDocumentMaxSummaryChars,
}

// SummaryCapFor returns the Summary length ceiling for the given
// shape. Unknown shapes fall back to AnswerDocumentMaxSummaryChars so
// a future shape addition that forgets to extend the map does not
// silently uncap.
func SummaryCapFor(shape AnswerShape) int {
	if cap, ok := SummaryCapByShape[shape]; ok {
		return cap
	}
	return AnswerDocumentMaxSummaryChars
}

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
