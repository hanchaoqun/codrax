package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func answerDocIOWindowScopedPredicate(predicate string) bool {
	return answerDocBoundedRuntimeIOLatencyPredicate(predicate) || predicate == "io_inflight" || predicate == "io_inflight_coverage"
}

// Split only the final answer's display pool. Unknown/line-selected IO stays
// available as separately scoped background; known outside-window IO remains
// in the full audit ledger and causal inputs, not in the requested fact pool.
// Neither a precise request span nor a matching issuer recovers query scope.
func answerDocIOWindowObservationRecords(ctx *types.AgentContext, records []types.ObservationRecord) (principal, background []types.ObservationRecord, outside int) {
	if ctx == nil || (ctx.AgentName != types.AgentFinalizer && ctx.Stage != types.StageFinalize) || ctx.AnalysisIR == nil ||
		!ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.HasExplicitTimeWindows() {
		return records, nil, 0
	}
	requested := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	for _, r := range records {
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
			!answerDocIOWindowScopedPredicate(strings.TrimSpace(r.Predicate)) {
			principal = append(principal, r)
			continue
		}
		start, end, known := types.TraceObservationContinuousQueryWindow(r.SourceRef)
		if !known {
			background = append(background, r)
		} else if requested.ContainsExplicitTimeWindow(start, end) {
			principal = append(principal, r)
		} else {
			outside++
		}
	}
	return principal, background, outside
}

// Query-population scope and a proved local wait are independent rulers. Keep
// validated, in-scope issuer waits in their existing dedicated evidence lanes,
// without putting their source rows back into the requested-statistics pool.
// Reuse the existing authority compiler: a predicate or closure flag alone is
// not proof of the target identity, source, measured interval, or duration.
func answerDocIndependentIOWaitRecords(ctx *types.AgentContext, records []types.ObservationRecord) []types.ObservationRecord {
	if ctx == nil || ctx.AnalysisIR == nil {
		return nil
	}
	rm := &ctx.AnalysisIR.RequestModel
	requested := rm.RuntimeArtifactScopeProfile
	if requested == nil || !requested.HasExplicitTimeWindows() {
		return nil
	}
	records, _ = answerDocSelectedWindowObservationRecords(ctx, records)
	var out []types.ObservationRecord
	for _, record := range records {
		if record.Predicate != "io_latency" {
			continue
		}
		start, end, selected := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
		if !selected || !requested.ContainsExplicitTimeWindow(start, end) {
			continue
		}
		authorities := types.BuildTraceBlockingWallClockAuthorities(types.ObservationLedger{Records: []types.ObservationRecord{record}}, rm)
		if len(authorities) != 1 || authorities[0].Type != "block_io_completion_closed_issuer_wait" || len(authorities[0].Occurrences) != 1 {
			continue
		}
		wait := authorities[0].Occurrences[0]
		if !requested.ContainsExplicitTimeWindow(wait.StartTs, wait.EndTs) {
			continue // Preserve the full interval as background; never clip it.
		}
		out = append(out, record)
	}
	return out
}

// A complete list of validated occurrences does not establish complete query
// coverage when any contributing receipt has no continuous time population.
func answerDocIOWaitHasUnverifiedPopulation(authority types.TraceBlockingWallClockAuthority, ledger types.ObservationLedger) bool {
	if authority.Type != "block_io_completion_closed_issuer_wait" {
		return false
	}
	ids := make(map[string]bool)
	for _, occurrence := range authority.Occurrences {
		for _, id := range occurrence.RecordIDs {
			ids[id] = true
		}
	}
	for _, record := range ledger.Records {
		if ids[record.ID] {
			if _, _, known := types.TraceObservationContinuousQueryWindow(record.SourceRef); !known {
				return true
			}
		}
	}
	return false
}

func renderAnswerDocSeparateIOQueryContext(ctx *types.AgentContext, records []types.ObservationRecord) string {
	if ctx == nil || ctx.AnalysisIR == nil || len(records) == 0 {
		return ""
	}
	const limit = 10
	rm := &ctx.AnalysisIR.RequestModel
	lang := extractAnswerDocLang(ctx)
	var b strings.Builder
	b.WriteString("### Separately Scoped IO Background\n\n")
	b.WriteString("- These observations have no proven continuous query window for the user's requested interval. A line selection takes precedence over accompanying time arguments; a point query is not a positive-width population. Preserve individual source observations and their original measurement meanings, but do not use their counts, distributions or durations as requested-window statistics. Matching the named thread does not repair the time scope or establish response impact. Unknown occupancy is not measured zero.\n")
	b.WriteString("- This background card grants neither target-window ownership nor a root cause. Do not clip full request residence to manufacture an account, combine independent query populations, or infer missing endpoints from the observed event span. For requested-window statistics, use an existing matching continuous-time query or query that window explicitly.\n")
	shown := min(len(records), limit)
	for _, r := range records[:shown] {
		row := answerDocRuntimeFactAuthorityRowWithOwnerScope(r, rm, lang, "separate_query_context")
		if r.Predicate != "io_inflight" && r.Predicate != "io_inflight_coverage" {
			row += fmt.Sprintf("; summary=%q", r.Summary)
		}
		fmt.Fprintf(&b, "  - %s; source_path=%q; query_result=%q; scope_boundary=%q\n",
			row, r.SourceRef.Path, r.SourceRef.PayloadRef,
			types.ResolveTraceObservationQueryWindowScope(rm.RuntimeArtifactScopeProfile, r.SourceRef).Format(lang))
	}
	fmt.Fprintf(&b, "- rendered_background_rows=%d; omitted_background_rows=%d; original observations remain unchanged in the audit ledger.\n\n", shown, len(records)-shown)
	return b.String()
}
