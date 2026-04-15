package tool

import (
	"sort"
	"strings"
	"sync"
)

// analysis_limits.go holds the runtime-tunable validation policy for
// emit_analysis. The tool's parameter schema is now purely structural
// (types + enums + required), so any "quality of classification"
// policy — keyword floors, generic-word entity filters — lives here
// as an explicit validator that Execute runs after json.Unmarshal.
//
// Why a separate file: the validator is the one part of emit_analysis
// that changes per deployment (a CI run might want strict rejection
// while interactive REPL wants a soft warning), while the schema
// rendering and normalizers are static. Isolating the validator keeps
// each file's scope obvious and makes the config entry point
// (SetAnalysisLimits) one grep away.

// AnalysisLimits is the runtime-tunable validation policy for
// emit_analysis. The zero value is NOT a safe default — use
// DefaultAnalysisLimits() to get a populated struct.
type AnalysisLimits struct {
	// WarnBelowKeywords is the soft floor for emit_analysis.keywords.
	// Execute attaches a "[warn: keywords=N below recommended M]" tag
	// to the ToolResult.Summary when len(keywords) < this value. Zero
	// disables the warning.
	WarnBelowKeywords int

	// RejectBelowKeywords is the hard floor. Execute fails the tool
	// call with Success=false when len(keywords) < this value. Zero
	// disables rejection entirely — only the warning fires. This is
	// the codrax default because rejecting a classification purely
	// for having 7 instead of 8 keywords wastes an LLM round trip
	// that the downstream IR builder could still make progress from.
	RejectBelowKeywords int

	// GenericEntityBlocklist is the set of words (lowercased) that
	// should never appear in emit_analysis.entities because they
	// poison ERM ranking — "count", "function", "agent", and friends
	// are generic nouns, not domain identifiers. Matches are case-
	// insensitive. The validator DROPS blocked words from the
	// normalized Entities slice and records the drops in the
	// warnings list so they surface in ToolResult.Summary and the
	// Debug trace. Set to nil to disable the filter entirely.
	GenericEntityBlocklist []string

	// RejectMultipleEmit decides the analyzer's policy when the LLM
	// calls emit_analysis more than once in a single analyze dispatch.
	// The tool's Execute method already accepts multiple calls (the
	// last write wins on Mutable.RequestModel), but the call-count
	// gate in analyzer.ParseOutput makes the repeat VISIBLE:
	//
	//   - false (default): log a warning, keep the last write,
	//     continue the pipeline. This matches the historical
	//     behavior — codrax prefers best-effort recovery over a
	//     wholesale abort because a wasted LLM round-trip is
	//     cheaper than a failed analyze stage.
	//   - true: write a descriptive message to StageOutput.Error so
	//     downstream tracing + eval harnesses see a loud failure.
	//     The IR is still populated from the last write so the
	//     rest of the pipeline can continue if the operator chooses
	//     to ignore the error signal.
	//
	// The 0-call case is handled separately (always falls back to
	// readOrSynthesizeRequestModel with a strong warning + a
	// structured `analysis_fallback_used` diagnostic), never gated
	// by this knob.
	RejectMultipleEmit bool

	// MaxPrescanRounds caps the number of analyzer pre-scan rounds
	// (iterations whose last-executed tool was `repo_map`, `grep`, or
	// `list_files`) before the analyzer's LoopController forces a
	// stop. The skill prompt asks for "1-2 rounds then emit_analysis",
	// and this is the runtime hard-enforcement of that ceiling: when
	// the LLM ignores the prompt and keeps pre-scanning, every extra
	// round burns an LLM round-trip that the explore stage was
	// supposed to own.
	//
	// Behavior:
	//   - N > 0: after `N` successful pre-scan rounds, the next
	//     observed pre-scan tool triggers a force-stop via
	//     LoopSignal{StopRequested: true} with a descriptive
	//     StopReason. The analyzer's ParseOutput then synthesises a
	//     zero-value RequestModel via readOrSynthesizeRequestModel
	//     (the same failsafe the 0-call branch uses), and the
	//     `analysis_prescan_budget_exhausted` diagnostic is surfaced
	//     on StageOutput.Data so operators see the cause.
	//   - N == 0: the runtime gate is DISABLED. Observe returns an
	//     empty signal regardless of how many pre-scan rounds fired.
	//     This is the escape hatch for tuning or debugging.
	//
	// Default is 2, matching the skill prompt's "1-2 rounds" language
	// literally. Mixed batches where `emit_analysis` and a pre-scan
	// tool run in the same iteration are not counted as pre-scan
	// rounds — the "last tool" is emit_analysis, which is the
	// desired end state.
	MaxPrescanRounds int
}

// DefaultAnalysisLimits returns the populated default policy. Callers
// that want to override one field should start from this value and
// mutate the returned struct; SetAnalysisLimits() then installs it.
//
// The defaults encode codrax's historical expectations:
//
//   - WarnBelowKeywords = 8 because the analyzer's keyword generation
//     guidance in the analysis-skill asks for three rounds producing
//     ≥8 stems.
//   - RejectBelowKeywords = 0 so a short keyword list becomes a
//     visible warning but does not drop the classification.
//   - GenericEntityBlocklist mirrors the words the analyzer's
//     Prohibitions call out explicitly ("count, function, thing,
//     agent, handler, module") plus a handful of nearby generic
//     nouns that historically poisoned ranking.
func DefaultAnalysisLimits() AnalysisLimits {
	return AnalysisLimits{
		WarnBelowKeywords:   8,
		RejectBelowKeywords: 0,
		MaxPrescanRounds:    2,
		GenericEntityBlocklist: []string{
			"agent", "agents",
			"class", "classes",
			"config", "configs",
			"count", "counts",
			"field", "fields",
			"function", "functions",
			"handler", "handlers",
			"item", "items",
			"key", "keys",
			"list", "lists",
			"method", "methods",
			"module", "modules",
			"name", "names",
			"object", "objects",
			"system", "systems",
			"thing", "things",
			"tool", "tools",
			"type", "types",
			"value", "values",
		},
	}
}

var (
	analysisLimitsMu sync.RWMutex
	analysisLimits   = DefaultAnalysisLimits()
)

// SetAnalysisLimits installs a new validation policy for emit_analysis.
// main.go calls this once after loading codrax.yaml; tests call it via
// t.Cleanup to restore the defaults between runs. Thread-safe: the
// validator reads under the same RWMutex so a concurrent overwrite
// cannot tear a slice.
func SetAnalysisLimits(l AnalysisLimits) {
	analysisLimitsMu.Lock()
	defer analysisLimitsMu.Unlock()
	analysisLimits = l
}

// CurrentAnalysisLimits returns a snapshot of the current policy. Used
// by the validator and by the startup banner logger in cmd/root.go.
func CurrentAnalysisLimits() AnalysisLimits {
	analysisLimitsMu.RLock()
	defer analysisLimitsMu.RUnlock()
	// Shallow copy is enough — callers only read the scalars and
	// iterate the blocklist without mutating it.
	out := analysisLimits
	if out.GenericEntityBlocklist != nil {
		out.GenericEntityBlocklist = append([]string(nil), out.GenericEntityBlocklist...)
	}
	return out
}

// analysisValidationResult is the structured output of the emit_analysis
// validator. A zero value means "no issues". Execute consumes this to
// build its ToolResult.Summary and to decide Success.
type analysisValidationResult struct {
	// FilteredEntities is the entities slice with blocklist words
	// removed. The validator always returns the filtered slice even
	// when no words were dropped, so Execute does not need to branch.
	FilteredEntities []string

	// DroppedEntities records which input words were filtered out,
	// in input order, for the Summary diagnostic.
	DroppedEntities []string

	// Warnings is a list of human-readable messages the caller should
	// append to the ToolResult.Summary. Present but non-fatal.
	Warnings []string

	// RejectReason is non-empty when Execute must report Success=false.
	// The caller returns immediately with this as the Summary.
	RejectReason string
}

// validateAnalysisInput applies the runtime AnalysisLimits policy to
// the raw emit_analysis parameters. It is the single place that knows
// "what counts as a healthy classification" — the JSON schema only
// enforces structural validity.
//
// Contract:
//
//   - keywords slice is passed through unchanged; the function only
//     decides whether to warn or reject based on its length.
//   - entities slice is returned with any blocklisted words removed
//     (case-insensitive match, preserving the original casing of
//     surviving words). Dropped words are listed in DroppedEntities
//     and a warning is appended mentioning them.
//   - an empty result (no warnings, no reject reason, no dropped
//     entities) is the happy path.
//
// The function is deliberately pure: it takes the policy as an argument
// so tests can exercise every branch without touching package state.
func validateAnalysisInput(keywords, entities []string, limits AnalysisLimits) analysisValidationResult {
	var res analysisValidationResult

	// Entity filter first so keyword counting operates on already-
	// sanitized data (though keywords and entities are independent,
	// deterministic pass order makes the Summary stable).
	res.FilteredEntities, res.DroppedEntities = filterGenericEntities(entities, limits.GenericEntityBlocklist)
	if len(res.DroppedEntities) > 0 {
		// Stable sort for a deterministic Summary even when the LLM
		// emits entities in a different order across runs.
		dropped := append([]string(nil), res.DroppedEntities...)
		sort.Strings(dropped)
		res.Warnings = append(res.Warnings,
			"dropped generic entities: "+strings.Join(dropped, ", "))
	}

	// Keyword floor. RejectBelowKeywords wins over WarnBelowKeywords
	// when both trigger so the hard signal is visible to the caller.
	kwCount := len(keywords)
	switch {
	case limits.RejectBelowKeywords > 0 && kwCount < limits.RejectBelowKeywords:
		res.RejectReason = formatKeywordFloorMsg("keywords below hard floor",
			kwCount, limits.RejectBelowKeywords)
	case limits.WarnBelowKeywords > 0 && kwCount < limits.WarnBelowKeywords:
		res.Warnings = append(res.Warnings,
			formatKeywordFloorMsg("keywords below recommended floor",
				kwCount, limits.WarnBelowKeywords))
	}

	return res
}

// filterGenericEntities removes blocklist-matching words from ents in
// a single pass. Match is case-insensitive on the trimmed word; the
// surviving entries keep their original spelling so downstream ERM
// ranking sees the verbatim token from the user's input.
//
// Returns (kept, dropped). Either slice may be nil when empty. The
// blocklist is materialized into a map for O(1) lookup — the list is
// small enough that the allocation is cheap and the helper is called
// at most once per emit_analysis dispatch.
func filterGenericEntities(ents, blocklist []string) (kept, dropped []string) {
	if len(ents) == 0 || len(blocklist) == 0 {
		return append([]string(nil), ents...), nil
	}
	deny := make(map[string]bool, len(blocklist))
	for _, w := range blocklist {
		deny[strings.ToLower(strings.TrimSpace(w))] = true
	}
	for _, e := range ents {
		norm := strings.ToLower(strings.TrimSpace(e))
		if norm == "" {
			continue
		}
		if deny[norm] {
			dropped = append(dropped, e)
			continue
		}
		kept = append(kept, e)
	}
	return kept, dropped
}

// formatKeywordFloorMsg renders a compact "keywords=N<M" diagnostic
// that both the warning path and the reject path reuse so the Summary
// wording is identical across severities.
func formatKeywordFloorMsg(label string, got, want int) string {
	var b strings.Builder
	b.WriteString(label)
	b.WriteString(" (got=")
	itoa(&b, got)
	b.WriteString(" want≥")
	itoa(&b, want)
	b.WriteString(")")
	return b.String()
}

// itoa is a zero-alloc int→decimal helper used by the Summary builder
// to avoid pulling in fmt just for %d. Microscopic, but the validator
// sits in a hot path of every analyzer dispatch and fmt.Sprintf
// allocates a fresh []byte per call.
func itoa(b *strings.Builder, n int) {
	if n == 0 {
		b.WriteByte('0')
		return
	}
	if n < 0 {
		b.WriteByte('-')
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	b.Write(buf[pos:])
}
