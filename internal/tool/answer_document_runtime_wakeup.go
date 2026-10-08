package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceWakeupEventNote = types.TraceNoteKeyWakeupEventInstance + "="

type traceWakeupEvent = types.RuntimeWakeupEvent

func traceQueryWakeupEventNote(ref types.ObservationSourceRef, edge tracequery.WakeupEdge, scan ...*types.TraceEventSearchScanScope) string {
	f := traceWakeupEvent{IndexPath: ref.Path, From: edge.From, To: edge.To,
		Waker: types.RuntimeWakeupThread(edge.Waker), Wakee: types.RuntimeWakeupThread(edge.Wakee),
		Timestamp: edge.WakeupTs, Line: edge.WakeupLine, Branch: edge.Branch, Segment: edge.SegmentOrdinal}
	if len(scan) > 0 {
		f.ScanScope = types.CloneTraceEventSearchScanScope(scan[0])
	}
	b, _ := json.Marshal(f)
	return traceWakeupEventNote + string(b)
}

func decodeRuntimeWakeupEvent(r types.ObservationRecord) (traceWakeupEvent, bool) {
	return types.DecodeRuntimeWakeupEvent(r)
}

type wakeupDiagramRelationProvider struct{ request *types.RequestModel }

func (p wakeupDiagramRelationProvider) Relations(ledger types.ObservationLedger) []RuntimeDiagramRelation {
	var rows []RuntimeDiagramRelation
	for _, event := range types.RuntimeWakeupDiagramEvents(ledger, p.request) {
		r, f := event.Record, event.Event
		ref, coordinates, data := r.SourceRef, event.Coordinates, event.Fingerprint
		// Instance-local endpoints prevent borrowing a repeated pair at another
		// timestamp/branch or inventing transitive arrows across two events.
		from := fmt.Sprintf("runtime_instance_%x", sha256.Sum256(append(append([]byte(nil), coordinates...), []byte("\x00from\x00"+string(data))...)))
		to := fmt.Sprintf("runtime_instance_%x", sha256.Sum256(append(append([]byte(nil), coordinates...), []byte("\x00to\x00"+string(data))...)))
		row := RuntimeDiagramRelation{Kind: types.DiagramRelWakeup, FromIdentity: from, ToIdentity: to,
			FromNode: runtimeWakeupDisplayNode(r, tracequery.ThreadRef(f.Waker), from), ToNode: runtimeWakeupDisplayNode(r, tracequery.ThreadRef(f.Wakee), to), FromLabel: r.Subject, ToLabel: r.Object,
			ScopeLabel:  fmt.Sprintf("唤醒时刻=%s秒；来源=%s；这是一次唤醒，不表示唤醒者造成了全部等待", traceQueryDisplaySeconds(f.Timestamp), strings.Join(r.SupportRefs, "; ")),
			SupportRefs: append([]string(nil), r.SupportRefs...)}
		// Physical point + exact typed event fields distinguish repeated events,
		// files, clocks and endpoints. Query/branch coordinates remain in the
		// hard credentials above; they must not consume repeated display slots.
		if physical, _, ok := runtimeWakeupPhysicalPoint(r); ok {
			// TGID can be optional lookup enrichment in a chain view and absent
			// from the same event_search row. It is not needed to sample an
			// exact physical row, and remains intact in the authority carrier.
			point, _ := json.Marshal([]any{ref.Path, physical, ref.TimeDomain, ref.CanonicalTimeDomain, f.Timestamp, f.Waker.PID, f.Waker.Comm, f.Wakee.PID, f.Wakee.Comm})
			query, _ := json.Marshal([]string{ref.Path, ref.QueryScopeID, ref.PayloadRef})
			row.presentationKey, row.presentationQuery = string(point), string(query)
		}
		rows = append(rows, row)
	}
	return rows
}

// This is an observed thread lane, not an incarnation or causal permission.
// Share it only inside one producer query/source, with exact observed identity
// fields. Edge identities stay event-local even when the display lane repeats.
func runtimeWakeupDisplayNode(record types.ObservationRecord, thread tracequery.ThreadRef, fallback string) string {
	// Provenance maps bundle virtual lines back to source-local point refs.
	// Without one exact physical point locator, keep the event-local display.
	_, path, ok := runtimeWakeupPhysicalPoint(record)
	if !ok {
		return runtimeDiagramNode(fallback)
	}
	ref := record.SourceRef
	b, _ := json.Marshal([]any{ref.Path, ref.QueryScopeID, ref.PayloadRef, path, ref.TimeDomain, ref.CanonicalTimeDomain, thread})
	return fmt.Sprintf("rt_thread_%x", sha256.Sum256(b))
}

func runtimeWakeupPhysicalPoint(record types.ObservationRecord) (point, path string, ok bool) {
	if len(record.SupportRefs) != 1 {
		return "", "", false
	}
	point = record.SupportRefs[0]
	colon := strings.LastIndexByte(point, ':')
	if colon <= 0 {
		return "", "", false
	}
	line, err := strconv.Atoi(point[colon+1:])
	return point, point[:colon], err == nil && line > 0
}
