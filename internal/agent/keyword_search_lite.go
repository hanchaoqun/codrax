package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// keyword_search_lite.go is the agent-side helper for the analyzer's
// runtime quality probe. It is the deliberate "lite" counterpart of
// `internal/agent/keyword_search.go`:
//
//   - `keyword_search.go` owns the full explorer pipeline: ripgrep
//     invocation, repo_map structural ranking, IDF weighting, camel-
//     case / snake-case keyword expansion, and per-file ranking. It
//     is the HEAVY path that produces the explorer's `preScannedFiles`
//     list.
//   - `keyword_search_lite.go` (this file) is a post-hoc probe: it
//     reads the analyzer's already-collected pre-scan summaries
//     from `MutableState.PrescanSummaryBlob` plus the final
//     keyword / entity lists from the emitted RequestModel, and
//     returns an `AnalysisQualityProbe` describing how much of the
//     LLM's classification was actually verified by the pre-scan.
//
// Why a separate file instead of a method on analyzerEvaluator:
//
//   - The analyzer's Observe and ParseOutput methods are already the
//     two heavy entrypoints. Keeping the probe logic out of those
//     method bodies isolates the "runtime data channel" responsibility
//     and makes it independently testable.
//   - The probe is the exact surface the explorer stage will reuse
//     via `analysis_quality_probe` — having it in its own file means
//     future cross-stage callers have an obvious single entry point.
//
// The core computation (substring scan of the seen-blob) lives in
// `internal/tool.ComputeAnalysisQualityProbe` so the tool package
// can run it INSIDE emit_analysis.Execute at tool-execution time
// without importing the agent package. This file wraps that helper
// with agent-side conveniences: extracting the blob + term lists
// from the appropriate runtime objects, and a warning-log shim for
// threshold breaches.

// computeQualityProbeFromContext builds an AnalysisQualityProbe
// from the runtime state at analyzer.ParseOutput time: the
// MutableState's pre-scan summary blob, the analyzer evaluator's
// round counter, and the emit_analysis-populated keyword / entity
// lists. Returns a zero probe when any input is nil so callers can
// unconditionally surface the probe into StageOutput.Data.
//
// The function intentionally does NOT re-run grep or repo_map.
// Pre-scan has already fired once during the ReAct loop; touching
// disk again here would double the dispatch cost and drift from
// the whitelist data the emit_analysis tool already consumed. The
// probe is purely a post-hoc measurement of "what the LLM saw vs
// what the LLM emitted", not a second chance to discover evidence.
func computeQualityProbeFromContext(ctx *types.AgentContext, prescanRounds int, keywords, entities []string) tool.AnalysisQualityProbe {
	if ctx == nil || ctx.Mutable == nil {
		return tool.AnalysisQualityProbe{
			KeywordTotal:  len(keywords),
			EntityTotal:   len(entities),
			PrescanRounds: prescanRounds,
		}
	}
	seenBlob := ctx.Mutable.PrescanSummaryBlob()
	return tool.ComputeAnalysisQualityProbe(seenBlob, keywords, entities, prescanRounds)
}

// qualityProbeToMap serialises an AnalysisQualityProbe for inclusion
// in StageOutput.Data. Returned as a map[string]any rather than a
// typed struct so analyzer.ParseOutput can drop it directly into its
// existing `data := map[string]any{...}` block without a second
// json.Marshal round.
//
// Shape (matches the user's spec from the feature request):
//
//	{
//	  "keyword_hits": int,
//	  "keyword_total": int,
//	  "keyword_hit_ratio": float (rounded to 2dp),
//	  "entity_hits": int,
//	  "entity_total": int,
//	  "entity_hit_ratio": float (rounded to 2dp),
//	  "prescan_rounds": int
//	}
//
// The two ratios are rounded to 2 decimal places so the JSON
// output is stable across runs (repeated %.2f style) instead of
// carrying floating-point noise from the division.
func qualityProbeToMap(p tool.AnalysisQualityProbe) map[string]any {
	return map[string]any{
		"keyword_hits":      p.KeywordHits,
		"keyword_total":     p.KeywordTotal,
		"keyword_hit_ratio": round2(p.KeywordHitRatio()),
		"entity_hits":       p.EntityHits,
		"entity_total":      p.EntityTotal,
		"entity_hit_ratio":  round2(p.EntityHitRatio()),
		"prescan_rounds":    p.PrescanRounds,
	}
}

// round2 returns `f` rounded to 2 decimal places. Used for the
// probe's ratio fields so JSON output stays deterministic across
// runs instead of carrying the full IEEE-754 noise.
func round2(f float64) float64 {
	// Go's fmt.Sprintf("%.2f", f) followed by strconv.ParseFloat is
	// the standard no-dependency way to truncate a float at a fixed
	// decimal place. The arithmetic form below is equivalent for
	// the 0.0-1.0 range the probe produces and avoids the
	// allocation of a %.2f format string.
	return float64(int(f*100+0.5)) / 100
}

// seenBlobContainsTerm reports whether `term` appears as a
// case-insensitive substring in `seenBlob`. Exported for tests that
// want to assert the whitelist behavior directly without going
// through the full validator. `seenBlob` is expected to already be
// lowercased (the MutableState writer guarantees this) but the
// function defensively lowercases `term` so callers don't need to.
func seenBlobContainsTerm(seenBlob, term string) bool {
	if seenBlob == "" || term == "" {
		return false
	}
	return strings.Contains(seenBlob, strings.ToLower(strings.TrimSpace(term)))
}
