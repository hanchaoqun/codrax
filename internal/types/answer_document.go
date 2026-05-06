package types

import "math"

// answer_document.go — supporting types for the V2 block-only answer
// carrier (AnswerDocumentV2 lives in answer_document_v2.go). After
// the B8 retirement of the V1 carrier, this file holds shared
// payload types: per-shape Summary length caps + the Citation pool +
// ExactResolution status enums + CodeSnippet.
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

// The V1 AnswerDocument struct that this comment block once
// described was retired together with AnswerShape. The V2 carrier
// (AnswerDocumentV2 in answer_document_v2.go) replaces it: instead
// of one shape-tagged struct with optional Steps / Symbols / Value /
// Boolean fields, the V2 form is a list of typed Block values whose
// presence is dictated by the AnswerSemanticView's required-block
// contract. The supporting types below (CodeSnippet,
// SummaryCapConfig, ExactResolutionStatus) are still active.
//
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

// SummaryCapForViewConfig returns the Summary length ceiling for
// the compiled AnswerSemanticView and its principal item count
// under an explicit config. principalCount is the number of
// principal blocks the answer carries (0 for scalar / explanation
// answers). A negative principalCount is clamped to 0. Callers
// should prefer SummaryCapForView; this variant exists so tests
// and the trimmer can reason about caps against arbitrary configs.
//
// Replaces the legacy SummaryCapForConfig(shape, itemCount) per
// docs/migration/answer_shape_retirement.md. The view-driven gate
// reads RequiredBlocks (BlockOrderedList / BlockSection / etc.)
// and the family classification rather than the legacy shape enum.
func SummaryCapForViewConfig(cfg SummaryCapConfig, view *AnswerSemanticView, principalCount int) int {
	if !cfg.Enabled {
		return SummaryCapUnlimited
	}
	if principalCount < 0 {
		principalCount = 0
	}
	if view == nil {
		return cfg.Default
	}
	switch {
	case view.NeedsEnumerationSlate():
		cap := cfg.SymbolsBase + principalCount*cfg.SymbolsPerItem
		if cap > cfg.SymbolsMax {
			cap = cfg.SymbolsMax
		}
		return cap
	case view.NeedsBoundedMechanismList() || view.NeedsOrderedPrincipalList():
		cap := cfg.StepListBase + principalCount*cfg.StepListPerItem
		if cap > cfg.StepListMax {
			cap = cfg.StepListMax
		}
		return cap
	case view.NeedsPrincipalScalar():
		switch view.Family {
		case QFConfigPrecedence:
			return cfg.ConfigValue
		default:
			return cfg.Value
		}
	}
	switch view.Family {
	case QFGeneric, QFArchitecture:
		return cfg.Explanation
	}
	return cfg.Default
}

// SummaryCapForView dispatches through the package-level
// summaryCapConfig. principalCount is the count of items in the
// rendered ordered_list / bullet_list / table block; ignored for
// scalar / explanation answers.
func SummaryCapForView(view *AnswerSemanticView, principalCount int) int {
	return SummaryCapForViewConfig(summaryCapConfig, view, principalCount)
}

// CitationRefUnset is the sentinel value used when an item has no
// backing citation in the pool. The renderer renders it as "no
// citation"; the tool schema allows it ONLY when the producing LLM
// explicitly sets -1 via the JSON input. Zero is NOT the sentinel
// because zero is a valid index into a single-citation pool — this
// distinction is critical for the schema validator to flag "forgot
// to set citation_ref" errors.
const CitationRefUnset = -1

// The renderer prefixes Rationale with a language-specific YES/NO
// word drawn from Decision, so the LLM cannot hedge by supplying
// "it depends" in Rationale.
//
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

	// 2026-05+ scope-axis. A Citation may anchor at one of six
	// shapes (line / line_range / section / file / crossfile /
	// negative); the renderer dispatches on Scope to format
	// `file:line` vs `file:start-end` vs `file [section: x]` vs
	// `file [layer: ...]` vs cross-file contract summary vs
	// `file [absence: ...]`. Empty Scope falls back to the
	// historical "file:line when both set, else file" rendering.
	Scope            EvidenceScope `json:"scope,omitempty"`
	LineEnd          int           `json:"line_end,omitempty"`
	SectionPath      string        `json:"section_path,omitempty"`
	FileRoleLabel    FileRoleLabel `json:"file_role_label,omitempty"`
	CrossfileSummary string        `json:"crossfile_summary,omitempty"`
	NegativePattern  string        `json:"negative_pattern,omitempty"`
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

// AnswerExactResolutionStatus is the finalizer's structured judgment
// for an exact-target question. The LLM recommends one of these enum
// values; emit_answer_document validates it against the current
// AnswerContract + evidence state.
type AnswerExactResolutionStatus string

const (
	AnswerExactResolutionExactMatch AnswerExactResolutionStatus = "exact_match"
	AnswerExactResolutionAliasMatch AnswerExactResolutionStatus = "alias_match"
	AnswerExactResolutionAbsent     AnswerExactResolutionStatus = "absent"
)

// AnswerExactResolutionContextMode tells the renderer/validators how
// any nearby grounded context should be framed relative to the exact
// target status.
type AnswerExactResolutionContextMode string

const (
	AnswerExactResolutionContextNone         AnswerExactResolutionContextMode = "none"
	AnswerExactResolutionContextGroundedOnly AnswerExactResolutionContextMode = "grounded_context_only"
)

// AnswerExactResolution is the structured exact-target disposition
// attached to AnswerDocument. Anchor is optional for exact_match and
// required for alias_match; absent answers usually leave it empty.
type AnswerExactResolution struct {
	Status      AnswerExactResolutionStatus      `json:"status"`
	Anchor      string                           `json:"anchor,omitempty"`
	ContextMode AnswerExactResolutionContextMode `json:"context_mode,omitempty"`
}
