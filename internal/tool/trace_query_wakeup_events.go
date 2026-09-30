package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Row-local scheduler facts can be queried without expanding a dependency tree.
// They prove only the recorded wake event. Never label them as chain edges or
// direct-root-cause evidence; duration thresholds and expansion caps do not
// alter the identity of an observed event_search row.
func traceQuerySchedulerWakeEventObservations(events []tracequery.EventView, ref types.ObservationSourceRef, scope, at string, coverage *tracequery.EventSearchCoverage) []types.ObservationRecord {
	if coverage == nil || coverage.ScanScope == nil || !types.ValidateTraceEventSearchScanScope(coverage.ScanScope) {
		return nil
	}
	var out []types.ObservationRecord
	for _, row := range events {
		e := row.Event
		if e.Type != tracequery.EventSchedWakeup || e.PID <= 0 || e.WakeePID <= 0 || e.Line <= 0 {
			continue
		}
		edge := tracequery.WakeupEdge{From: fmt.Sprintf("event:%d:waker", e.Line), To: fmt.Sprintf("event:%d:wakee", e.Line),
			Waker: tracequery.ThreadRef{PID: e.PID, Comm: e.Comm}, Wakee: tracequery.ThreadRef{PID: e.WakeePID, Comm: e.WakeeComm}, WakeupTs: e.Ts, WakeupLine: e.Line}
		subject, object := traceThreadLabel(edge.Waker), traceThreadLabel(edge.Wakee)
		out = append(out, types.ObservationRecord{
			ID: fmt.Sprintf("trace_query:%s#scheduler_wakeup_event:%d", scope, e.Line), Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan,
			SourceRef: ref, Span: types.ObservationSpan{LineStart: e.Line, LineEnd: e.Line, StartTs: e.Ts, EndTs: e.Ts},
			ClaimKey: "scheduler_wakeup_event:" + subject + "->" + object, Predicate: "scheduler_wakeup_event", Subject: subject, Object: object, Value: "1", Unit: "event",
			Summary:   fmt.Sprintf("Recorded scheduler wake event: %s -> %s at %s seconds; no chain membership or wait-duration attribution", subject, object, traceQueryDisplaySeconds(e.Ts)),
			RichNotes: []string{traceQueryWakeupEventNote(ref, edge, coverage.ScanScope)}, SupportRefs: traceQueryObservationSupportRefs(ref, e.Line, e.Line), ObservedAt: at, Confidence: 1,
		})
	}
	return out
}
