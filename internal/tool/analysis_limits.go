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

	// WarnBelowKeywordHitRatio is the soft floor for the analyzer's
	// runtime quality probe: when fewer than this fraction (0.0-1.0)
	// of the emit_analysis.keywords were observed in any pre-scan
	// tool result's Summary, the validator attaches a "[warn:
	// keyword_hit_ratio=X < Y]" tag to the ToolResult.Summary. 0
	// disables the warning. Default 0 (disabled): the ratio is
	// computed and surfaced as a diagnostic but does not trip a
	// warning threshold. Operators raise this to enforce evidence
	// coupling between the pre-scan loop and the classification.
	WarnBelowKeywordHitRatio float64

	// WarnBelowEntityHitRatio is the same soft floor for
	// emit_analysis.entities. Same 0.0-1.0 scale, same disabled-at-0
	// default. Entities are higher-signal than keywords (they are
	// copied verbatim from the user question), so a missing entity
	// is a stronger "pre-scan didn't verify this term" signal than
	// a missing keyword.
	WarnBelowEntityHitRatio float64

	// Session 11 C0' ClassificationGrep limits — control the analyzer's
	// Round 2 line-level grep capability. The capability is off by
	// default unless analyzer triggers it (trigger logic lives in
	// analyzer.go). When triggered, the validator in agent.go admits
	// files_only=false grep calls but caps them at these budgets.
	// Setting ClassificationGrepEnabled=false disables the capability
	// entirely — the analyzer falls back to Round 1-only behavior.

	// ClassificationGrepEnabled turns the capability on/off. When
	// false, validateAnalyzerPrescanToolCall rejects every
	// files_only=false grep call in analyze stage regardless of
	// trigger state. Default true.
	ClassificationGrepEnabled bool

	// ClassificationGrepMaxCalls is the per-dispatch limit on
	// line-level calls. Default 3.
	ClassificationGrepMaxCalls int

	// ClassificationGrepMaxMatchesPerCall is the per-call cap on
	// match count. When the LLM's grep params omit max_count (or
	// provide a value above this limit), the validator auto-clamps
	// it before forwarding to the tool. Default 20.
	ClassificationGrepMaxMatchesPerCall int

	// ClassificationGrepMaxTotalBytes is the cumulative byte cap
	// across all line-level calls in one dispatch. Default 8192.
	ClassificationGrepMaxTotalBytes int

	// ClassificationGrepMinLLMSubjectConf is the LLM subject-
	// confidence threshold below which the analyzer's Round-2
	// trigger fires (one of several trigger conditions; see
	// analyzer.shouldTriggerClassificationGrep). Default 0.80.
	ClassificationGrepMinLLMSubjectConf float64

	// Session 11 C4 — strict shape handling. When true, the
	// emit_answer_document B2a auto-rescue path (which silently
	// switches the answer shape when canCorrect=true) is
	// disabled: mismatched shapes always return a failed
	// ToolResult so the LLM sees the Error and re-emits with
	// the correct shape. The G3 F4 HintComposer feeds the
	// structured retry hint so the LLM has enough signal to
	// self-correct on retry — no need for the silent save.
	// Default false to keep legacy tests stable; operators
	// flip to true in codrax.yaml once retry telemetry shows
	// the F4 hints are driving single-retry recovery reliably.
	ShapeSwapStrictMode bool

	// Session 11 R5 — ghost-anchor promotion threshold. When the
	// D2 chain-promotion enforcer has skipped N occurrences of the
	// SAME file as a ghost anchor and the file exists on disk, the
	// explorer adds the file to ScannedSet so the next D2 pass
	// accepts it. A conservative default of 3 balances false
	// promote (too eager → pollutes ScannedSet) against missed
	// recovery (too lax → the legitimate ranker gap never closes).
	// Zero disables the promotion entirely.
	GhostAnchorExpandSearchThreshold int

	// Session 12 — phase1-unread pre-complete gate. When the explorer
	// calls emit_investigation_complete while high-ranked pre-scan
	// files remain unread (the LLM skipped them) AND the declared
	// RequirementKind is a breadth-intent (mechanism / call_chain /
	// conditional — questions whose answer generally requires tracing
	// multi-file flow), the gate raises a RepairExpandSearch
	// directive, appends PendingReads for the top-K unread files, and
	// downgrades the complete call so the retry window forces the
	// LLM to read them. The top-K is capped by Phase1UnreadTopK;
	// Phase1UnreadMinUnread is the minimum number of unread top-K
	// files required to trigger (prevents single-miss noise).
	// Set Phase1UnreadTopK to 0 to disable the gate entirely.
	Phase1UnreadTopK      int
	Phase1UnreadMinUnread int
}

// AnalysisQualityProbe captures runtime hit statistics from the
// analyzer's evidence-lite pre-scan. It is the shared data channel
// between `analyzerEvaluator.Observe` (which accumulates pre-scan
// summaries into `MutableState.PrescanSummaryBlob`) and
// `validateAnalysisInput` / `analyzer.ParseOutput` (which grade the
// quality of the LLM's classification against what the pre-scan
// actually found).
//
// The probe turns the pre-scan → validator relationship from
// prompt-level coordination ("spend 1-2 rounds verifying ...") into
// data-level coordination: keywords / entities the LLM emitted must
// actually appear in the pre-scan Summary blob, otherwise the
// validator can either warn or whitelist-exempt them from the
// generic-entity blocklist.
type AnalysisQualityProbe struct {
	// KeywordHits is the count of emit_analysis.keywords whose
	// lowercase form appears as a substring anywhere in the pre-scan
	// summary blob. Exact substring match, not tokenization, so
	// `analyze` matches `AnalyzeRequest` and `RunAnalyzer`.
	KeywordHits int

	// KeywordTotal is len(keywords) at probe time. A ratio can only
	// be computed when KeywordTotal > 0; otherwise callers should
	// treat it as "no data".
	KeywordTotal int

	// EntityHits is the same count for emit_analysis.entities.
	// Entities are the highest-signal terms — copied verbatim from
	// the user question — so their hit ratio is the strongest
	// measure of "the LLM extracted entities that actually exist
	// in the target repository".
	EntityHits int

	// EntityTotal is len(entities) at probe time.
	EntityTotal int

	// PrescanRounds is a copy of analyzerEvaluator.prescanRounds at
	// probe time, carried alongside the hit stats so downstream
	// consumers see the full runtime picture without juggling two
	// data sources.
	PrescanRounds int
}

// KeywordHitRatio returns KeywordHits/KeywordTotal as a 0.0-1.0
// fraction, or 0 when KeywordTotal is zero. Callers that care about
// distinguishing "0% hit ratio" from "no data" should check
// KeywordTotal independently.
func (p AnalysisQualityProbe) KeywordHitRatio() float64 {
	if p.KeywordTotal == 0 {
		return 0
	}
	return float64(p.KeywordHits) / float64(p.KeywordTotal)
}

// EntityHitRatio mirrors KeywordHitRatio for entities.
func (p AnalysisQualityProbe) EntityHitRatio() float64 {
	if p.EntityTotal == 0 {
		return 0
	}
	return float64(p.EntityHits) / float64(p.EntityTotal)
}

// ComputeAnalysisQualityProbe scans a pre-scan summary blob and
// counts how many of the given keywords / entities appear as a
// case-insensitive substring. The blob is expected to be the
// lowercased concatenation of every successful pre-scan
// ToolResult.Summary collected during the analyze dispatch (see
// `MutableState.PrescanSummaryBlob` / `AppendPrescanSummary`).
//
// This function is deterministic, allocation-bounded, and safe to
// call from both tool-Execute time (emit_analysis) and post-hoc
// (analyzer.ParseOutput). A zero probe is returned when the blob
// is empty — callers can still surface it as "probe was armed but
// found nothing".
func ComputeAnalysisQualityProbe(seenBlob string, keywords, entities []string, prescanRounds int) AnalysisQualityProbe {
	return AnalysisQualityProbe{
		KeywordHits:   countTermHitsInBlob(seenBlob, keywords),
		KeywordTotal:  len(keywords),
		EntityHits:    countTermHitsInBlob(seenBlob, entities),
		EntityTotal:   len(entities),
		PrescanRounds: prescanRounds,
	}
}

// countTermHitsInBlob returns the number of entries in `terms`
// whose lowercase form is a substring of `seenBlob`. `seenBlob` is
// expected to already be lowercased — the caller (typically
// `MutableState.PrescanSummaryBlob`) owns the normalization so this
// helper stays a hot-path allocation-free loop.
func countTermHitsInBlob(seenBlob string, terms []string) int {
	if seenBlob == "" || len(terms) == 0 {
		return 0
	}
	hits := 0
	for _, t := range terms {
		needle := strings.ToLower(strings.TrimSpace(t))
		if needle == "" {
			continue
		}
		if strings.Contains(seenBlob, needle) {
			hits++
		}
	}
	return hits
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
//   - WarnBelowKeywordHitRatio / WarnBelowEntityHitRatio default to
//     0 so the quality probe is COMPUTED and SURFACED on every
//     dispatch but does not trip a warning threshold by default.
//     Operators opt into quality-gated warnings by raising these
//     floors in codrax.yaml.
func DefaultAnalysisLimits() AnalysisLimits {
	return AnalysisLimits{
		WarnBelowKeywords:                   8,
		RejectBelowKeywords:                 0,
		MaxPrescanRounds:                    2,
		WarnBelowKeywordHitRatio:            0,
		WarnBelowEntityHitRatio:             0,
		ClassificationGrepEnabled:           true,
		ClassificationGrepMaxCalls:          3,
		ClassificationGrepMaxMatchesPerCall: 20,
		ClassificationGrepMaxTotalBytes:     8192,
		ClassificationGrepMinLLMSubjectConf: 0.80,
		GhostAnchorExpandSearchThreshold:    3,
		Phase1UnreadTopK:                    5,
		Phase1UnreadMinUnread:               2,
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

	// KeptVerifiedEntities records entities that matched the generic
	// blocklist but were rescued by the pre-scan verified whitelist
	// (their lowercase form appears in the seen-blob). The LLM's
	// term survives but a warning is attached so the operator can
	// audit "we almost dropped `Agent` but saw it in repo grep
	// results". Separate from DroppedEntities so the two rescue
	// paths are independently observable.
	KeptVerifiedEntities []string

	// Warnings is a list of human-readable messages the caller should
	// append to the ToolResult.Summary. Present but non-fatal.
	Warnings []string

	// RejectReason is non-empty when Execute must report Success=false.
	// The caller returns immediately with this as the Summary.
	RejectReason string

	// Probe is the runtime quality probe computed against the
	// analyzer's pre-scan summary blob. When the seen-blob passed to
	// validateAnalysisInput is empty (e.g. the analyzer ran with no
	// pre-scan rounds), Probe is a zero value and should be treated
	// as "no data". Otherwise the KeywordHitRatio / EntityHitRatio
	// helpers return the usable fractions.
	Probe AnalysisQualityProbe
}

// validateAnalysisInput applies the runtime AnalysisLimits policy to
// the raw emit_analysis parameters. It is the single place that knows
// "what counts as a healthy classification" — the JSON schema only
// enforces structural validity.
//
// Contract:
//
//   - keywords slice is passed through unchanged; the function only
//     decides whether to warn or reject based on its length and
//     optionally on the pre-scan hit ratio.
//   - entities slice is returned with any blocklisted words removed
//     (case-insensitive match, preserving the original casing of
//     surviving words). Dropped words are listed in DroppedEntities
//     and a warning is appended mentioning them.
//   - `seenBlob` is the lowercase concatenation of every successful
//     pre-scan ToolResult.Summary (populated by
//     `MutableState.PrescanSummaryBlob` / `AppendPrescanSummary`).
//     When non-empty, it feeds two distinct mechanisms:
//       1. **Verified-entity whitelist**: a generic-blocklist match
//          whose lowercase term also appears as a substring in
//          seenBlob is KEPT (with a `kept_generic_verified_entities`
//          warning) instead of dropped. Fixes the "Agent / Handler
//          are in the repo as real symbols but the blocklist
//          deleted them anyway" regression.
//       2. **Hit-ratio quality probe**: compute
//          KeywordHits/Total and EntityHits/Total against seenBlob
//          and emit `[warn: keyword_hit_ratio=...]` when the
//          computed ratio is below the configured floor.
//     When `seenBlob` is empty (analyzer ran with no pre-scans),
//     neither mechanism fires and the historical behavior is
//     preserved byte-for-byte.
//   - `prescanRounds` is threaded into the returned Probe so
//     downstream consumers (analyzer.ParseOutput, eval harnesses)
//     see the full runtime picture in one struct.
//   - an empty result (no warnings, no reject reason, no dropped
//     entities, empty probe) is the happy path.
//
// The function is deliberately pure: it takes the policy as an argument
// so tests can exercise every branch without touching package state.
func validateAnalysisInput(keywords, entities []string, limits AnalysisLimits, seenBlob string, prescanRounds int) analysisValidationResult {
	var res analysisValidationResult

	// Entity filter with the verified-entity whitelist exception.
	// Generic words that the pre-scan saw in the target repo are
	// KEPT (with a separate warning) instead of dropped so real
	// symbols named `Agent` / `Handler` survive.
	res.FilteredEntities, res.DroppedEntities, res.KeptVerifiedEntities =
		filterGenericEntitiesWithWhitelist(entities, limits.GenericEntityBlocklist, seenBlob)
	if len(res.DroppedEntities) > 0 {
		// Stable sort for a deterministic Summary even when the LLM
		// emits entities in a different order across runs.
		dropped := append([]string(nil), res.DroppedEntities...)
		sort.Strings(dropped)
		res.Warnings = append(res.Warnings,
			"dropped_generic_entities: "+strings.Join(dropped, ", "))
	}
	if len(res.KeptVerifiedEntities) > 0 {
		kept := append([]string(nil), res.KeptVerifiedEntities...)
		sort.Strings(kept)
		res.Warnings = append(res.Warnings,
			"kept_generic_verified_entities: "+strings.Join(kept, ", "))
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

	// Runtime quality probe. Always computed when a seenBlob is
	// available so the operator can diff "what the LLM emitted" vs
	// "what the pre-scan saw" without enabling warnings. The warning
	// thresholds are separate knobs that fire conditionally.
	res.Probe = ComputeAnalysisQualityProbe(seenBlob, keywords, entities, prescanRounds)

	if seenBlob != "" {
		if limits.WarnBelowKeywordHitRatio > 0 && res.Probe.KeywordTotal > 0 {
			if ratio := res.Probe.KeywordHitRatio(); ratio < limits.WarnBelowKeywordHitRatio {
				res.Warnings = append(res.Warnings,
					formatHitRatioWarning("keyword_hit_ratio", ratio, limits.WarnBelowKeywordHitRatio,
						res.Probe.KeywordHits, res.Probe.KeywordTotal))
			}
		}
		if limits.WarnBelowEntityHitRatio > 0 && res.Probe.EntityTotal > 0 {
			if ratio := res.Probe.EntityHitRatio(); ratio < limits.WarnBelowEntityHitRatio {
				res.Warnings = append(res.Warnings,
					formatHitRatioWarning("entity_hit_ratio", ratio, limits.WarnBelowEntityHitRatio,
						res.Probe.EntityHits, res.Probe.EntityTotal))
			}
		}
	}

	return res
}

// formatHitRatioWarning produces a compact diagnostic line for the
// quality-probe warnings. Format: `keyword_hit_ratio=0.25 (1/4) below
// floor 0.50` — matches the `label=value (hits/total) below floor
// threshold` shape of the existing keyword-floor warnings.
func formatHitRatioWarning(label string, ratio, floor float64, hits, total int) string {
	var b strings.Builder
	b.WriteString(label)
	b.WriteString("=")
	writeFloat2(&b, ratio)
	b.WriteString(" (")
	itoa(&b, hits)
	b.WriteString("/")
	itoa(&b, total)
	b.WriteString(") below floor ")
	writeFloat2(&b, floor)
	return b.String()
}

// writeFloat2 writes a 0.00-formatted float to b. Used by
// formatHitRatioWarning to keep the validator warnings alloc-free
// on the hot path. Values outside [0,1] are written with one extra
// decimal of precision for debugging edge cases.
func writeFloat2(b *strings.Builder, f float64) {
	if f < 0 {
		b.WriteByte('-')
		f = -f
	}
	whole := int(f)
	itoa(b, whole)
	frac := int((f - float64(whole)) * 100)
	if frac < 0 {
		frac = -frac
	}
	b.WriteByte('.')
	if frac < 10 {
		b.WriteByte('0')
	}
	itoa(b, frac)
}

// filterGenericEntitiesWithWhitelist removes blocklist-matching
// words from ents in a single pass, with one exception: when a
// blocklisted word ALSO appears as a substring in seenBlob
// (lowercased concatenation of the analyzer's pre-scan summaries),
// it is KEPT in a separate `keptVerified` slice. The caller gets
// three disjoint buckets:
//
//   - `kept`: non-blocklisted entries (the happy path), plus the
//     rescued blocklist-matches that the pre-scan verified.
//   - `dropped`: blocklist-matches that the pre-scan did NOT verify.
//     Returned in input order, surfaced via the
//     `dropped_generic_entities` warning line.
//   - `keptVerified`: blocklist-matches rescued by the whitelist.
//     Also present in `kept` (so downstream consumers see a single
//     authoritative entity list), but separately listed here so
//     the caller can emit a distinct warning.
//
// When seenBlob is empty, the whitelist is inactive and the
// function reduces exactly to the pre-refactor `filterGenericEntities`
// behavior: blocklist matches are dropped unconditionally. This
// matters for callers that have no pre-scan history (tests,
// fallback paths) — they get the historical strict filtering.
//
// Match is case-insensitive on the trimmed word; surviving entries
// keep their original spelling so downstream ERM ranking sees the
// verbatim token from the user's input.
func filterGenericEntitiesWithWhitelist(ents, blocklist []string, seenBlob string) (kept, dropped, keptVerified []string) {
	if len(ents) == 0 || len(blocklist) == 0 {
		return append([]string(nil), ents...), nil, nil
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
			if seenBlob != "" && strings.Contains(seenBlob, norm) {
				// Pre-scan verified: keep the entry but record it in
				// keptVerified so a distinct warning fires. Append to
				// `kept` too so downstream consumers see the full
				// entity list in one place.
				kept = append(kept, e)
				keptVerified = append(keptVerified, e)
				continue
			}
			dropped = append(dropped, e)
			continue
		}
		kept = append(kept, e)
	}
	return kept, dropped, keptVerified
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
