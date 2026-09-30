package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceWakeupEventNote = types.TraceNoteKeyWakeupEventInstance + "="

// The engine owns topology and event identity. This compact carrier transports
// only recorded edges, never a reconstructed label/path or aggregate pair count.
type traceWakeupEvent struct {
	IndexPath string               `json:"index_path"`
	From      string               `json:"from"`
	To        string               `json:"to"`
	Waker     tracequery.ThreadRef `json:"waker"`
	Wakee     tracequery.ThreadRef `json:"wakee"`
	Timestamp float64              `json:"timestamp"`
	Line      int                  `json:"line"`
	Branch    int                  `json:"branch"`
	Segment   int                  `json:"segment"`
}

func traceQueryWakeupEventNote(ref types.ObservationSourceRef, edge tracequery.WakeupEdge) string {
	f := traceWakeupEvent{ref.Path, edge.From, edge.To, edge.Waker, edge.Wakee, edge.WakeupTs, edge.WakeupLine, edge.Branch, edge.SegmentOrdinal}
	b, _ := json.Marshal(f)
	return traceWakeupEventNote + string(b)
}

func decodeRuntimeWakeupEvent(r types.ObservationRecord) (traceWakeupEvent, bool) {
	var f traceWakeupEvent
	ref := r.SourceRef
	if r.Negative || r.Predicate != "wakeup_chain_edge" || r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
		!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.GroundingPolicy != types.ClaimGroundingHard ||
		r.ProvenanceLane != types.ObservationProvenanceObservedDirectCause || r.Role != types.AnswerAggregateRoleSupportingCoverage ||
		ref.Kind != types.ObservationSourceRuntimeArtifact || ref.QueryScopeID == "" || ref.PayloadRef == "" ||
		!ref.QueryWindowKnown || !finiteRuntimeEventTime(ref.QueryWindowStartTs) || !finiteRuntimeEventTime(ref.QueryWindowEndTs) ||
		ref.QueryWindowEndTs <= ref.QueryWindowStartTs || len(r.SupportRefs) == 0 {
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
	return f, count == 1 && f.IndexPath != "" && f.IndexPath == ref.Path && f.From != "" && f.To != "" && f.From != f.To &&
		f.Waker.PID > 0 && f.Wakee.PID > 0 && f.Line > 0 && f.Branch >= 0 && f.Segment >= 0 && finiteRuntimeEventTime(f.Timestamp) &&
		f.Timestamp >= ref.QueryWindowStartTs && f.Timestamp < ref.QueryWindowEndTs &&
		(!ref.QueryLineRangeKnown || ((ref.QueryLineStart <= 0 || f.Line >= ref.QueryLineStart) && (ref.QueryLineEnd <= 0 || f.Line <= ref.QueryLineEnd))) &&
		f.Line == r.Span.LineStart && f.Line == r.Span.LineEnd && f.Timestamp == r.Span.StartTs && f.Timestamp == r.Span.EndTs &&
		traceThreadLabel(f.Waker) == r.Subject && traceThreadLabel(f.Wakee) == r.Object && r.Unit == "ms" &&
		r.ClaimKey == "wakeup_chain_edge:"+r.Subject+"->"+r.Object
}

func finiteRuntimeEventTime(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

type wakeupDiagramRelationProvider struct{ request *types.RequestModel }

func (p wakeupDiagramRelationProvider) Relations(ledger types.ObservationLedger) []RuntimeDiagramRelation {
	type candidate struct {
		row         RuntimeDiagramRelation
		fingerprint string
	}
	byEvent := map[string]candidate{}
	conflict := map[string]bool{}
	for _, r := range ledger.Records {
		f, ok := decodeRuntimeWakeupEvent(r)
		if !ok {
			continue
		}
		ref := r.SourceRef
		if scope := ledger.RuntimeArtifactScopeProfile; scope != nil && scope.HasExplicitTimeWindows() &&
			!scope.ContainsExplicitTimeWindow(ref.QueryWindowStartTs, ref.QueryWindowEndTs) {
			continue
		}
		// Match the query's selected target, not the waker. This retains native
		// upstream branches while unrelated queries cannot borrow the target.
		if runtimeDiagramHasNamedBoundedTarget(p.request) {
			selected := r
			selected.Subject = traceThreadLabel(tracequery.ThreadRef{PID: ref.QueryTargetPID, Comm: ref.QueryTargetThread})
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
			FromNode: runtimeDiagramNode(from), ToNode: runtimeDiagramNode(to), FromLabel: r.Subject, ToLabel: r.Object,
			ScopeLabel:  fmt.Sprintf("唤醒时刻=%s秒；查询窗口=[%s,%s)秒；来源=%s；这是一次唤醒，不表示唤醒者造成了全部等待", traceQueryDisplaySeconds(f.Timestamp), traceQueryDisplaySeconds(ref.QueryWindowStartTs), traceQueryDisplaySeconds(ref.QueryWindowEndTs), strings.Join(r.SupportRefs, "; ")),
			SupportRefs: append([]string(nil), r.SupportRefs...)}
		byEvent[key] = candidate{row, string(data)}
	}
	keys := make([]string, 0, len(byEvent))
	for key := range byEvent {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var rows []RuntimeDiagramRelation
	for _, key := range keys {
		if !conflict[key] {
			rows = append(rows, byEvent[key].row)
		}
	}
	return rows
}
