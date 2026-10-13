package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// This label carries the native aggregation domain through older text and
// observation surfaces. It never merges cohorts or changes their denominator.
func traceQueryPerfScopeLabel(scope *tracequery.PerfCohortScope) string {
	if scope == nil {
		return ""
	}
	end := ")"
	if scope.Window.EndInclusive {
		end = "]"
	}
	text := fmt.Sprintf(" cohort_id=%s source_path=%q source_generation=%s producer=%s sample_kind=%s clock=%s cohort_window=[%.9f,%.9f%s end_inclusive=%t",
		scope.ID, scope.SourcePath, scope.SourceGeneration, sanitizeForBanner(scope.Producer), sanitizeForBanner(scope.SampleKind), sanitizeForBanner(scope.Clock), scope.Window.StartTs, scope.Window.EndTs, end, scope.Window.EndInclusive)
	if scope.ThreadIdentity != nil {
		text += fmt.Sprintf(" thread_tid=%d thread_generation=%d", scope.ThreadIdentity.TID, scope.ThreadIdentity.Generation)
	}
	return text
}

func traceQueryPerfCohortClaimKey(cohort tracequery.PerfCohort, hot tracequery.PerfHotspot) string {
	key := "perf_sample_top_symbol:" + firstNonEmptyTraceString(hot.Symbol, hot.DSO, hot.Event)
	if cohort.Scope != nil && cohort.Scope.ID != "" {
		raw, _ := json.Marshal([]string{hot.Symbol, hot.DSO, cohort.Event, cohort.WeightUnit, cohort.Scope.ID})
		key = fmt.Sprintf("perf_sample_top_symbol:cohort=%x", sha256.Sum256(raw))
	}
	return key
}

func traceQueryPerfCohortDenominatorLabel(cohort tracequery.PerfCohort) string {
	if cohort.Scope == nil {
		return ""
	}
	// Compact prompt summaries have a small prefix budget. Put the numeric
	// denominator and producer before optional long paths or display names.
	return fmt.Sprintf("denominator_weight=%d denominator_samples=%d producer=%s event=%s weight_unit=%s cohort_id=%s ", cohort.TotalPeriod, cohort.SampleCount, sanitizeForBanner(cohort.Scope.Producer), sanitizeForBanner(cohort.Event), sanitizeForBanner(cohort.WeightUnit), cohort.Scope.ID)
}

func traceQueryPerfCallchainCaliber(hot tracequery.PerfHotspot) string {
	identity := hot.CallchainIdentity
	if identity == nil {
		return " callchain_form=opaque_flat_label frame_reason=frame_identity_unavailable"
	}
	text := " callchain_form=" + sanitizeForBanner(identity.Form)
	if identity.Reason != "" {
		text += " frame_reason=" + sanitizeForBanner(identity.Reason)
	}
	if identity.Form == "exact_frames" {
		text += fmt.Sprintf(" path_id=%s exact_frame_count=%d", sanitizeForBanner(identity.ID), identity.FrameCount)
	}
	return text
}
