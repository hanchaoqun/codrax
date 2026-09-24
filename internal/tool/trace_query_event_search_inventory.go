package tool

import (
	"strings"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The effective query and engine result meet only at publication. A model
// aggregate, summary, or legacy call without a query cannot mint this receipt.
func traceQueryEventSearchInventoryObservation(result tracequery.Result, ref types.ObservationSourceRef, at string, queries []tracequery.Query) []types.ObservationRecord {
	if len(queries) != 1 || tracequery.CanonicalViewName(result.View) != "event_search" ||
		tracequery.CanonicalViewName(queries[0].View) != "event_search" || result.EventSearchCoverage == nil {
		return nil
	}
	q, c := queries[0], result.EventSearchCoverage
	names, namesErr := tracequery.NormalizeEventSearchNames(q.View, q.EventNames)
	if namesErr != nil {
		return nil
	}
	if tracequery.ValidateEventFieldFilters(q.View, q.EventFieldFilters) != nil || len(result.Events) != c.Emitted {
		return nil
	}
	i := &types.TraceEventSearchInventory{
		SchemaVersion: types.TraceEventSearchInventorySchemaVersion,
		QueryScopeID:  ref.QueryScopeID,
		Query: types.TraceEventSearchInventoryQuery{
			View: "event_search", Pattern: q.Pattern, Patterns: append([]string(nil), q.Patterns...),
			EventNames: names,
			PID:        q.PID, Thread: q.Thread, ThreadInput: q.ThreadInput, TargetScope: q.TargetScope,
			TimeStart: q.TimeStart, TimeEnd: q.TimeEnd, TimeStartSet: q.TimeStartSet, TimeEndSet: q.TimeEndSet,
			LineStart: q.LineStart, LineEnd: q.LineEnd, SpanName: q.SpanName, Limit: q.Limit,
		},
		Coverage: types.TraceEventSearchInventoryCoverage{
			ScopeKind: c.ScopeKind, ScopeTimeStart: c.ScopeTimeStart, ScopeTimeEnd: c.ScopeTimeEnd,
			ScopeTimestampRows: c.ScopeTimestampRows, ScopeComplete: c.ScopeComplete,
			MatchedTimeStart: c.MatchedTimeStart, MatchedTimeEnd: c.MatchedTimeEnd,
			MatchedTotal: c.MatchedTotal, Emitted: c.Emitted, EnumerationComplete: c.EnumerationComplete,
		},
		Rows: []types.TraceEventSearchInventoryRow{}, Caveats: append([]string(nil), result.Caveats...),
	}
	for _, v := range q.EventTypes {
		i.Query.EventTypes = append(i.Query.EventTypes, string(v))
	}
	for _, v := range q.TraceMarkActions {
		i.Query.TraceMarkActions = append(i.Query.TraceMarkActions, string(v))
	}
	for _, f := range q.EventFieldFilters {
		i.Query.EventFieldFilters = append(i.Query.EventFieldFilters, types.TraceEventSearchInventoryFilter{Field: f.Field, Op: f.Op, Value: string(f.Value)})
	}
	for _, event := range result.Events {
		if len(i.Rows) >= types.TraceEventSearchInventoryRowLimit {
			break
		}
		raw := event.Raw
		truncated := len(raw) > types.TraceEventSearchInventoryRawLimit
		if truncated {
			raw = raw[:types.TraceEventSearchInventoryRawLimit]
			for !utf8.ValidString(raw) && len(raw) > 0 {
				raw = raw[:len(raw)-1]
			}
		}
		// A failed affine inverse leaves SourceTs at canonical Ts in the
		// engine view. Preserve the exact unavailable state, not that fallback
		// number as a physical header. Identity-clock rows need no alignment.
		sourceTimeKnown := event.SourcePath != "" && event.LocalLine > 0 && event.RawUnavailableReason != "clock_inverse_unsafe"
		sourceTime := 0.0
		if sourceTimeKnown {
			sourceTime = event.SourceTs
		}
		row := types.TraceEventSearchInventoryRow{
			Line: event.Line, SourcePath: event.SourcePath, LocalLine: event.LocalLine,
			TraceTimeSeconds: event.Ts, SourceTimeSeconds: sourceTime,
			SourceTimeKnown: sourceTimeKnown,
			TimeDomain:      event.TimeDomain, CanonicalTimeDomain: event.CanonicalTimeDomain,
			EventType: string(event.Type), EventName: event.Name, Comm: event.Comm, EmitterTID: event.PID, EmitterTGID: event.TGID,
			MarkerPID: event.SpanPID, CPU: event.CPU, Raw: raw, RawTruncated: truncated,
			RawUnavailableReason: event.RawUnavailableReason,
		}
		if event.PluginFields != nil && event.PluginFields.JankEvent != nil {
			j := event.PluginFields.JankEvent
			row.JankEvent = &types.TraceEventSearchJankEvent{IssueReason: j.IssueReason, TimeDomainStatus: j.TimeDomainStatus}
			if v := j.Values; v != nil {
				row.JankEvent.Values = &types.TraceEventSearchJankValues{StartTSNS: v.StartTSNS, EndTSNS: v.EndTSNS, JankFrames: v.JankFrames, AppID: v.AppID, ReportedDurationNS: v.ReportedDurationNS}
			}
		}
		i.Rows = append(i.Rows, row)
	}
	i.HandoffRowsOmitted = c.Emitted - len(i.Rows)
	i.RowsComplete = c.ScopeComplete && c.EnumerationComplete && len(i.Rows) == c.MatchedTotal
	r := types.ObservationRecord{
		ID:     "trace_query:" + ref.QueryScopeID + "#" + types.TraceEventSearchInventoryPredicate,
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, ClaimAuthority: types.ObservationClaimAuthorityDirectObservation,
		SourceRef: ref, ObservedAt: at, Predicate: types.TraceEventSearchInventoryPredicate,
		ClaimKey: types.TraceEventSearchInventoryPredicate, Scope: strings.TrimSpace(c.ScopeKind),
		Summary:              "Producer-owned event_search inventory; counts and rows apply only to this query; no scheduler identity or causal conclusion.",
		EventSearchInventory: i,
	}
	if !types.IsValidTraceEventSearchInventoryRecord(r) {
		return nil
	}
	return []types.ObservationRecord{r}
}
