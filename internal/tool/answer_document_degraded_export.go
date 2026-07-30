// Package tool — answer_document_degraded_export.go (XGAP-FIX ④,
// §29.104.8, witness 20260715-202022.323-89609).
//
// The deterministic, model-independent answer sections (◎ runtime trace
// observation board, causal projection tree + detail tables, metric
// snapshot, optimization points, next steps, perf quality, supplement
// disclosure) are all assembled at the persistMergedAnswerDocument
// chokepoint. A run whose finalizer exhausts retries without one accepted
// emit never reaches that chokepoint, so the recovery-rendered answer
// shipped WITHOUT any of them even though every typed observation was in
// hand (the witness fallback rendered 88 observations as proof). This
// export lets the finalizer's ParseOutput recovery lanes run the exact
// same materializer chain on the recovered document. Every materializer
// already carries its own idempotence guard, so re-running on a document
// that somehow carries a section is safe.
package tool

import (
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// MaterializeDeterministicAnswerSectionsForDegradedDoc runs the persist
// chokepoint's deterministic section materializers plus the deterministic
// citation-quote backfill (引用回填) and the runtime-artifact citation
// quote check on a recovery-lane document that never reached
// persistMergedAnswerDocument. It mutates doc in place and returns the
// human-readable names of the sections/defenses that actually produced
// output (for the degraded footer disclosure and the run log). It never
// touches MutableState and never validates: the caller owns the degraded
// disposition.
func MaterializeDeterministicAnswerSectionsForDegradedDoc(ctx *types.BusContext, doc *types.AnswerDocumentV2) []string {
	if ctx == nil || doc == nil {
		return nil
	}
	var produced []string
	record := func(name string, ok bool) {
		if ok {
			produced = append(produced, name)
			logging.Info("[answer_document/degraded_export] materialized deterministic section on recovery lane: %s", name)
		}
	}
	record("causal_projection", materializeRuntimeTraceCausalProjectionBlock(doc, ctx))
	record("semantic_optimization", materializeRuntimeTraceSemanticOptimizationBlock(doc, ctx))
	record("metric_snapshot", materializeRuntimeTraceMetricSnapshotBlock(doc, ctx))
	record("next_steps", materializeRuntimeTraceNextStepsBlock(doc, ctx))
	record("perf_quality", materializeRuntimeTracePerfQualityBlock(doc, ctx))
	record("observation_board", materializeRuntimeTraceObservationBlock(doc, ctx))
	record("supplement_disclosure", materializeRuntimeTraceSupplementDisclosureCaveat(doc, ctx))
	if moved := normalizeRuntimeTraceReportHierarchy(doc); moved > 0 {
		record("report_hierarchy", true)
	}
	// 引用回填 (XGAP-FIX ⑤ degraded-lane input): the same gated
	// deterministic current-source quote backfill the persist chokepoint
	// runs — the witness shipped three misplaced citations precisely
	// because this never ran on the recovery lane.
	//
	// EVALFIX-2E (CLASS 5): self_disclosing — 不入账. This lane already
	// discloses via the degraded footer's citation_quote_backfill entry
	// (degradedSectionDisplayNames); recordCitationQuoteRewriteDegradation
	// is deliberately NOT called here — booking it on the degradation
	// ledger too would double-disclose the same rewrites.
	if answerDocumentHasQuotelessCurrentSourceCitation(doc, ctx) {
		if fixed := normalizeCurrentSourceCitationQuotes(doc, ctx); fixed > 0 {
			logging.Warning("[answer_document/degraded_export] backfilled %d quoteless citation quote(s) from current source on recovery lane", fixed)
			produced = append(produced, "citation_quote_backfill")
		}
	}
	// Runtime-artifact citation quote check (XGAP-FIX ⑤ independent arm;
	// also wired on the healthy persist path): detect → disclose, never
	// reject.
	if flagged := verifyRuntimeArtifactCitationQuotes(doc, ctx); flagged > 0 {
		produced = append(produced, "artifact_quote_check")
	}
	return produced
}
