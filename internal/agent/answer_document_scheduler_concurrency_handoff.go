package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocSchedulerConcurrencyPredicate(predicate string) bool {
	return predicate == "scheduler_concurrency" || predicate == "scheduler_concurrency_coverage"
}

// This bounded account has its own display budget, never a causal/IO seat.
// Distinct query receipts, sources and windows are not merged for display.
func answerDocSchedulerConcurrencyRows(ledger types.ObservationLedger, rm *types.RequestModel) ([]types.ObservationRecord, int) {
	const limit = 10
	var records []types.ObservationRecord
	for _, r := range ledger.Records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
			r.GroundingPolicy != types.ClaimGroundingHard || !answerDocSchedulerConcurrencyPredicate(r.Predicate) ||
			r.SourceRef.QueryScopeID == "" || r.SourceRef.PayloadRef == "" {
			continue
		}
		if rm != nil && rm.RuntimeArtifactScopeProfile != nil && rm.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() &&
			(!r.SourceRef.QueryWindowKnown || !rm.RuntimeArtifactScopeProfile.ContainsExplicitTimeWindow(r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs)) {
			continue
		}
		records = append(records, r)
	}
	var selected []types.ObservationRecord
	seen := map[string]bool{}
	add := func(r types.ObservationRecord) {
		key, _ := json.Marshal(r)
		if len(selected) < limit && !seen[string(key)] {
			seen[string(key)] = true
			selected = append(selected, r)
		}
	}
	// Preserve both distinct state rulers and an exclusion receipt before
	// additional source/query rows. No selected row is rewritten or summed.
	for _, state := range []string{"runnable", "running"} {
		for _, r := range records {
			if r.Predicate == "scheduler_concurrency" && r.Subject == state {
				add(r)
				break
			}
		}
	}
	for _, r := range records {
		if r.Predicate == "scheduler_concurrency_coverage" {
			add(r)
			break
		}
	}
	for _, r := range records {
		if r.Predicate == "scheduler_concurrency" {
			add(r)
		}
	}
	for _, r := range records {
		add(r)
	}
	return selected, len(records) - len(selected)
}

func renderAnswerDocSchedulerConcurrencyMeasurements(ctx *types.AgentContext, ledger types.ObservationLedger) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := &ctx.AnalysisIR.RequestModel
	rows, omitted := answerDocSchedulerConcurrencyRows(ledger, rm)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Confirmed Scheduler Concurrency Measurements\n\n")
	b.WriteString("- These source-window statistics use only confirmed closed scheduler intervals of positive TIDs. Runnable and Running are separate populations; source groups cannot be added. Query target identity is context, not a population filter or target ownership. Keep them as resource context: they do not establish a dependency, target waiting, priority inversion or a root cause.\n")
	b.WriteString("- Each thread's overlapping intervals are unioned before half-open concurrency counting. Peak is simultaneous threads; mean uses the entire selected window, including zero contribution of this confirmed population. Busy time is its interval union; thread·ms is integrated concurrency, not one response's wall-clock delay. A zero segment does not prove that the system was idle. Open ends, excluded/unknown identities and missing source evidence are not filled with zero; absence of a value is unmeasured. Coverage does not promise complete capture.\n")
	b.WriteString("- Preserve each row's original source, query receipt and exact window. Do not substitute a containing query's average or peak for a smaller user window. Translate field names and status codes into reader language such as 同时就绪线程数、同时运行线程数、全窗平均、已确认闭合区间; retain omission and coverage limits.\n")
	fmt.Fprintf(&b, "- rendered_source_rows=%d omitted_source_rows=%d; independent display budget, not a whole-capture census.\n", len(rows), omitted)
	for _, r := range rows {
		fmt.Fprintf(&b, "  - %s; source_path=%q; query_result=%q\n", answerDocBoundedRuntimeFactAuthorityRow(r, rm, extractAnswerDocLang(ctx)), r.SourceRef.Path, r.SourceRef.PayloadRef)
	}
	b.WriteByte('\n')
	return b.String()
}

func answerDocSchedulerConcurrencyDisplayParts(r types.ObservationRecord) []string {
	parts := []string{fmt.Sprintf("summary=%q", r.Summary)}
	for _, key := range []string{types.TraceNoteKeySchedulerConcurrencyGroup, types.TraceNoteKeySchedulerConcurrencyBasis,
		types.TraceNoteKeySchedulerConcurrencyCoverage, types.TraceNoteKeySchedulerConcurrencyExclusions,
		types.TraceNoteKeySchedulerConcurrencyTimeline, types.TraceNoteKeySchedulerConcurrencyScope} {
		if value := traceQueryObservationSupplementNoteValue(r, key); value != "" {
			parts = append(parts, fmt.Sprintf("%s=%q", key, value))
		}
	}
	return parts
}
