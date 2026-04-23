package types

import "math"

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
//   - Summary: LLM-authored prose, length-capped per shape by
//     SummaryCapFor(shape, itemCount) — see SummaryCapConfig
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

// SummaryCapConfig carries the per-shape Summary length ceilings
// enforced by emit_answer_document. Each shape uses Summary for a
// different purpose so a single number misserves some of them:
//
//   - Explanation: Summary IS the answer body. Thorough multi-paragraph
//     prose with code-level specifics, cross-file relationships, and
//     mechanism details.
//   - Value / ConfigValue: Summary is a short lead-in before a scalar
//     literal. Kept tight.
//   - Boolean: Summary is a lead-in before YES/NO + rationale. A little
//     more room than Value because the rationale often justifies a
//     non-trivial claim.
//   - StepList: the user-visible answer is the step sequence; Summary
//     is a lead-in whose useful length scales with the number of steps
//     (more steps deserve a longer framing). Cap is
//     min(StepListMax, StepListBase + n*StepListPerItem).
//   - ListOfSymbols: same shape of scaling, slightly smaller per-item
//     slope because the symbols themselves carry most of the content.
//     Cap is min(SymbolsMax, SymbolsBase + n*SymbolsPerItem).
//
// Enabled is the master switch: when false (the default) every shape
// reports SummaryCapUnlimited and no length enforcement runs anywhere.
// Operators opt into length control by setting summary_cap_enabled:
// true in codrax.yaml — the per-shape numbers below are only
// consulted when the switch is on. Rationale: the finalizer's
// tendency to "compress on retry" hurts far more user-visible answers
// than runaway summaries do in practice, so the conservative default
// is to let the LLM's own emission length stand.
//
// All numeric fields are runtime-tunable via codrax.yaml (summary_cap_*).
// The package-level summaryCapConfig is the single source of truth for
// emit_answer_document, the shrinkage-salvage trimmer, and the
// per-shape test expectations; operators override it with
// SetSummaryCapConfig at startup.
type SummaryCapConfig struct {
	Enabled         bool
	Explanation     int
	Value           int
	ConfigValue     int
	Boolean         int
	StepListBase    int
	StepListPerItem int
	StepListMax     int
	SymbolsBase     int
	SymbolsPerItem  int
	SymbolsMax      int
	// Default applies to any shape not explicitly handled (currently
	// ShapeNone, which is rejected upstream; kept so a future shape
	// addition that forgets to extend this struct does not silently
	// uncap).
	Default int
}

// SummaryCapUnlimited is the sentinel returned when length control
// is disabled. math.MaxInt means every `len(s) > cap` comparison is
// false and every trim-to-cap branch is a no-op, so disabled mode
// behaves as "no cap" without special-casing every call site.
const SummaryCapUnlimited = math.MaxInt

// DefaultSummaryCapConfig returns the baseline caps used when no
// codrax.yaml override is present. Enabled defaults to false — the
// numeric fields describe what caps WOULD be applied if the switch
// were flipped on, and changing them here propagates to every call
// site automatically via summaryCapConfig once Enabled is set.
func DefaultSummaryCapConfig() SummaryCapConfig {
	return SummaryCapConfig{
		Enabled:         false,
		Explanation:     2500,
		Value:           500,
		ConfigValue:     500,
		Boolean:         800,
		StepListBase:    1000,
		StepListPerItem: 120,
		StepListMax:     2500,
		SymbolsBase:     1000,
		SymbolsPerItem:  100,
		SymbolsMax:      2500,
		Default:         500,
	}
}

var summaryCapConfig = DefaultSummaryCapConfig()

// SetSummaryCapConfig replaces the active per-shape caps. Called
// once at startup from cmd/root.go after codrax.yaml merges; no
// locking because later Runs do not mutate this.
func SetSummaryCapConfig(cfg SummaryCapConfig) { summaryCapConfig = cfg }

// SummaryCapForConfig returns the Summary length ceiling for the
// given shape and item count under an explicit config. itemCount is
// len(Steps) for ShapeStepList and len(Symbols) for ShapeListOfSymbols;
// it is ignored for scalar shapes. A negative itemCount is clamped to
// 0. Callers should prefer SummaryCapFor; this variant exists so tests
// and the trimmer can reason about caps against arbitrary configs.
func SummaryCapForConfig(cfg SummaryCapConfig, shape AnswerShape, itemCount int) int {
	if !cfg.Enabled {
		return SummaryCapUnlimited
	}
	if itemCount < 0 {
		itemCount = 0
	}
	switch shape {
	case ShapeExplanation:
		return cfg.Explanation
	case ShapeValue:
		return cfg.Value
	case ShapeConfigValue:
		return cfg.ConfigValue
	case ShapeBoolean:
		return cfg.Boolean
	case ShapeStepList:
		cap := cfg.StepListBase + itemCount*cfg.StepListPerItem
		if cap > cfg.StepListMax {
			cap = cfg.StepListMax
		}
		return cap
	case ShapeListOfSymbols:
		cap := cfg.SymbolsBase + itemCount*cfg.SymbolsPerItem
		if cap > cfg.SymbolsMax {
			cap = cfg.SymbolsMax
		}
		return cap
	default:
		return cfg.Default
	}
}

// SummaryCapFor dispatches through the package-level summaryCapConfig.
// itemCount is len(Steps) for ShapeStepList, len(Symbols) for
// ShapeListOfSymbols, ignored otherwise.
func SummaryCapFor(shape AnswerShape, itemCount int) int {
	return SummaryCapForConfig(summaryCapConfig, shape, itemCount)
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
// text; oversize Quotes are truncated on a UTF-8 boundary to the
// current CitationMaxQuoteChars(), useful when the renderer wants to
// surface the exact snippet the citation anchors on.
type Citation struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Quote string `json:"quote,omitempty"`
}

// DefaultCitationMaxQuoteChars is the baseline preview ceiling used
// when no codrax.yaml override is present. The prose-smuggling defence
// does not rely on this number — ground.GroundCitation's QuoteMatched
// token check clears any Quote whose tokens do not corroborate the
// cited line. This cap is purely the render-preview width; legit long
// source lines (deep package imports, multi-arg fmt.Errorf, long SQL
// or regex literals) routinely exceed 200 chars, so the default is
// generous enough to preserve most of them intact. Operators can raise
// it via citation_quote_max_chars for codebases with unusually long
// lines (Kotlin DSLs, Scala implicits, generated code).
const DefaultCitationMaxQuoteChars = 500

var citationMaxQuoteChars = DefaultCitationMaxQuoteChars

// CitationMaxQuoteChars returns the active preview ceiling. Callers
// that trim or compare Quote lengths must go through this helper
// rather than caching the value at init time — cmd/root.go replaces
// it after codrax.yaml merges.
func CitationMaxQuoteChars() int { return citationMaxQuoteChars }

// SetCitationMaxQuoteChars replaces the active preview ceiling. No
// locking: called once at startup from cmd/root.go; later Runs do not
// mutate it. Non-positive input is ignored so a partial or malformed
// override leaves the default intact.
func SetCitationMaxQuoteChars(n int) {
	if n <= 0 {
		return
	}
	citationMaxQuoteChars = n
}

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
