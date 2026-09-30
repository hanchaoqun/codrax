package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceWakeupEventNote = types.TraceNoteKeyWakeupEventInstance + "="

// The engine owns topology and event identity. This compact carrier transports
// only recorded edges, never a reconstructed label/path or aggregate pair count.
type traceWakeupEvent struct {
	IndexPath string                           `json:"index_path"`
	From      string                           `json:"from"`
	To        string                           `json:"to"`
	Waker     tracequery.ThreadRef             `json:"waker"`
	Wakee     tracequery.ThreadRef             `json:"wakee"`
	Timestamp float64                          `json:"timestamp"`
	Line      int                              `json:"line"`
	Branch    int                              `json:"branch"`
	Segment   int                              `json:"segment"`
	ScanScope *types.TraceEventSearchScanScope `json:"scan_scope,omitempty"`
}

func traceQueryWakeupEventNote(ref types.ObservationSourceRef, edge tracequery.WakeupEdge, scan ...*types.TraceEventSearchScanScope) string {
	f := traceWakeupEvent{IndexPath: ref.Path, From: edge.From, To: edge.To, Waker: edge.Waker, Wakee: edge.Wakee, Timestamp: edge.WakeupTs, Line: edge.WakeupLine, Branch: edge.Branch, Segment: edge.SegmentOrdinal}
	if len(scan) > 0 {
		f.ScanScope = types.CloneTraceEventSearchScanScope(scan[0])
	}
	b, _ := json.Marshal(f)
	return traceWakeupEventNote + string(b)
}

func decodeRuntimeWakeupEvent(r types.ObservationRecord) (traceWakeupEvent, bool) {
	var f traceWakeupEvent
	ref := r.SourceRef
	chain := r.Predicate == "wakeup_chain_edge" && r.ProvenanceLane == types.ObservationProvenanceObservedDirectCause
	event := r.Predicate == "scheduler_wakeup_event" && r.ProvenanceLane == types.ObservationProvenanceArtifactSpan
	if r.Negative || (!chain && !event) || r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
		!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.GroundingPolicy != types.ClaimGroundingHard ||
		r.Role != types.AnswerAggregateRoleSupportingCoverage ||
		ref.Kind != types.ObservationSourceRuntimeArtifact || ref.QueryScopeID == "" || ref.PayloadRef == "" ||
		len(r.SupportRefs) == 0 {
		return f, false
	}
	count := 0
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, traceWakeupEventNote) {
			count++
			if json.Unmarshal([]byte(strings.TrimPrefix(note, traceWakeupEventNote)), &f) != nil {
				return f, false
			}
		}
	}
	inside := ref.QueryWindowKnown && finiteRuntimeEventTime(ref.QueryWindowStartTs) && finiteRuntimeEventTime(ref.QueryWindowEndTs) &&
		ref.QueryWindowEndTs > ref.QueryWindowStartTs && f.Timestamp >= ref.QueryWindowStartTs && f.Timestamp < ref.QueryWindowEndTs
	if event {
		// A matched envelope is not a selector: singleton and final rows remain
		// valid. The executed engine selector is inclusive; requested half-open
		// membership is checked separately by the relation provider below.
		s := f.ScanScope
		inside = s != nil && types.ValidateTraceEventSearchScanScope(s) &&
			(!s.TimeStartApplied || f.Timestamp >= s.TimeStart) && (!s.TimeEndApplied || f.Timestamp <= s.TimeEnd) &&
			(s.LineStart <= 0 || f.Line >= s.LineStart) && (s.LineEnd <= 0 || f.Line <= s.LineEnd)
	}
	return f, inside && count == 1 && f.IndexPath != "" && f.IndexPath == ref.Path && f.From != "" && f.To != "" && f.From != f.To &&
		f.Waker.PID > 0 && f.Wakee.PID > 0 && f.Line > 0 && f.Branch >= 0 && f.Segment >= 0 && finiteRuntimeEventTime(f.Timestamp) &&
		(!ref.QueryLineRangeKnown || ((ref.QueryLineStart <= 0 || f.Line >= ref.QueryLineStart) && (ref.QueryLineEnd <= 0 || f.Line <= ref.QueryLineEnd))) &&
		f.Line == r.Span.LineStart && f.Line == r.Span.LineEnd && f.Timestamp == r.Span.StartTs && f.Timestamp == r.Span.EndTs &&
		traceThreadLabel(f.Waker) == r.Subject && traceThreadLabel(f.Wakee) == r.Object &&
		((chain && r.Unit == "ms") || (event && r.Unit == "event" && r.Value == "1")) &&
		r.ClaimKey == r.Predicate+":"+r.Subject+"->"+r.Object
}

func finiteRuntimeEventTime(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

type wakeupDiagramRelationProvider struct{ request *types.RequestModel }

func (p wakeupDiagramRelationProvider) Relations(ledger types.ObservationLedger) []RuntimeDiagramRelation {
	type candidate struct {
		row         RuntimeDiagramRelation
		fingerprint string
		timestamp   float64
		line        int
	}
	byEvent := map[string]candidate{}
	conflict := map[string]bool{}
	for _, r := range ledger.Records {
		f, ok := decodeRuntimeWakeupEvent(r)
		if !ok {
			continue
		}
		ref := r.SourceRef
		if scope := ledger.RuntimeArtifactScopeProfile; scope != nil && scope.HasExplicitTimeWindows() {
			inside := scope.ContainsExplicitTimeWindow(ref.QueryWindowStartTs, ref.QueryWindowEndTs)
			if r.Predicate == "scheduler_wakeup_event" {
				// event_search may expand only its lookup boundary. An individual
				// point fact uses exact requested half-open membership, not the
				// wider query envelope or an aggregate denominator.
				inside = false
				for _, window := range scope.ExplicitTimeWindows() {
					inside = inside || (f.Timestamp >= *window.TimeStart && f.Timestamp < *window.TimeEnd)
				}
			}
			if !inside {
				continue
			}
		}
		// Match the query's selected target, not the waker. This retains native
		// upstream branches while unrelated queries cannot borrow the target.
		if runtimeDiagramHasNamedBoundedTarget(p.request) {
			selected := r
			selected.Subject = traceThreadLabel(tracequery.ThreadRef{PID: ref.QueryTargetPID, Comm: ref.QueryTargetThread})
			if r.Predicate == "scheduler_wakeup_event" {
				// Unfiltered event_search has no selected query target. This one
				// event belongs to its exact wakee, never every nearby thread.
				selected.Subject = r.Object
			}
			if !types.ObservationRecordMatchesUserRuntimeTarget(selected, p.request) {
				continue
			}
		}
		// Do not include mutable enrichment (capture provenance) in identity.
		coordinates, _ := json.Marshal([]any{ref.QueryScopeID, ref.PayloadRef, f.IndexPath, ref.QueryWindowStartTs, ref.QueryWindowEndTs,
			ref.QueryTargetPID, ref.QueryTargetThread, ref.QueryTargetScope, ref.QueryLineRangeKnown, ref.QueryLineStart, ref.QueryLineEnd, f.Line, f.Branch, f.Segment})
		key := string(coordinates)
		data, _ := json.Marshal(f)
		if prior, exists := byEvent[key]; exists {
			if prior.fingerprint != string(data) {
				conflict[key] = true
			}
			continue
		}
		// Instance-local endpoints prevent borrowing a repeated pair at another
		// timestamp/branch or inventing transitive arrows across two events.
		from := fmt.Sprintf("runtime_instance_%x", sha256.Sum256(append(append([]byte(nil), coordinates...), []byte("\x00from\x00"+string(data))...)))
		to := fmt.Sprintf("runtime_instance_%x", sha256.Sum256(append(append([]byte(nil), coordinates...), []byte("\x00to\x00"+string(data))...)))
		row := RuntimeDiagramRelation{Kind: types.DiagramRelWakeup, FromIdentity: from, ToIdentity: to,
			FromNode: runtimeWakeupDisplayNode(r, f.Waker, from), ToNode: runtimeWakeupDisplayNode(r, f.Wakee, to), FromLabel: r.Subject, ToLabel: r.Object,
			ScopeLabel:  fmt.Sprintf("唤醒时刻=%s秒；来源=%s；这是一次唤醒，不表示唤醒者造成了全部等待", traceQueryDisplaySeconds(f.Timestamp), strings.Join(r.SupportRefs, "; ")),
			SupportRefs: append([]string(nil), r.SupportRefs...)}
		byEvent[key] = candidate{row, string(data), f.Timestamp, f.Line}
	}
	keys := make([]string, 0, len(byEvent))
	for key := range byEvent {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := byEvent[keys[i]], byEvent[keys[j]]
		if a.timestamp != b.timestamp {
			return a.timestamp < b.timestamp
		}
		if a.line != b.line {
			return a.line < b.line
		}
		return keys[i] < keys[j]
	})
	var rows []RuntimeDiagramRelation
	for _, key := range keys {
		if !conflict[key] {
			rows = append(rows, byEvent[key].row)
		}
	}
	return rows
}

// This is an observed thread lane, not an incarnation or causal permission.
// Share it only inside one producer query/source, with exact observed identity
// fields. Edge identities stay event-local even when the display lane repeats.
func runtimeWakeupDisplayNode(record types.ObservationRecord, thread tracequery.ThreadRef, fallback string) string {
	// Provenance maps bundle virtual lines back to source-local point refs.
	// Without one exact physical point locator, keep the event-local display.
	if len(record.SupportRefs) != 1 {
		return runtimeDiagramNode(fallback)
	}
	support := record.SupportRefs[0]
	colon := strings.LastIndexByte(support, ':')
	if colon <= 0 {
		return runtimeDiagramNode(fallback)
	}
	line, err := strconv.Atoi(support[colon+1:])
	if err != nil || line <= 0 {
		return runtimeDiagramNode(fallback)
	}
	ref := record.SourceRef
	b, _ := json.Marshal([]any{ref.Path, ref.QueryScopeID, ref.PayloadRef, support[:colon], ref.TimeDomain, ref.CanonicalTimeDomain, thread})
	return fmt.Sprintf("rt_thread_%x", sha256.Sum256(b))
}
