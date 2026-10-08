package types

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// RuntimeWakeupThread is the producer's wire identity, not a source symbol.
// Keeping the carrier below tracequery lets answer-shape and edge validators
// consume the same observation authority without a types/tool import cycle.
type RuntimeWakeupThread struct {
	Comm string `json:"comm,omitempty"`
	PID  int    `json:"pid,omitempty"`
	TGID int    `json:"tgid,omitempty"`
}

// RuntimeWakeupThreadLabel preserves the existing tool-side display spelling.
// The label is checked against the typed carrier; it never establishes identity.
func RuntimeWakeupThreadLabel(t RuntimeWakeupThread) string {
	var b strings.Builder
	for _, r := range t.Comm {
		if r == '\n' || r == '\r' || r == '\t' || r < 0x20 {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	comm := b.String()
	if len(comm) > 200 {
		comm = CutPrefixRuneSafe(comm, 197) + "..."
	}
	switch {
	case comm != "" && t.PID > 0:
		return fmt.Sprintf("%s-%d", comm, t.PID)
	case comm != "":
		return comm
	case t.PID > 0:
		return fmt.Sprintf("pid=%d", t.PID)
	default:
		return "unknown-thread"
	}
}

type RuntimeWakeupEvent struct {
	IndexPath string                     `json:"index_path"`
	From      string                     `json:"from"`
	To        string                     `json:"to"`
	Waker     RuntimeWakeupThread        `json:"waker"`
	Wakee     RuntimeWakeupThread        `json:"wakee"`
	Timestamp float64                    `json:"timestamp"`
	Line      int                        `json:"line"`
	Branch    int                        `json:"branch"`
	Segment   int                        `json:"segment"`
	ScanScope *TraceEventSearchScanScope `json:"scan_scope,omitempty"`
}

// DecodeRuntimeWakeupEvent validates one producer-owned physical event. A
// census, summary, arbitrary note, or model-authored observation cannot qualify.
func DecodeRuntimeWakeupEvent(r ObservationRecord) (RuntimeWakeupEvent, bool) {
	var f RuntimeWakeupEvent
	ref := r.SourceRef
	chain := r.Predicate == "wakeup_chain_edge" && r.ProvenanceLane == ObservationProvenanceObservedDirectCause
	event := r.Predicate == "scheduler_wakeup_event" && r.ProvenanceLane == ObservationProvenanceArtifactSpan
	if r.Negative || (!chain && !event) || r.Origin != AnswerEvidenceOriginRuntimeArtifact ||
		!RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.GroundingPolicy != ClaimGroundingHard ||
		r.Role != AnswerAggregateRoleSupportingCoverage ||
		ref.Kind != ObservationSourceRuntimeArtifact || ref.QueryScopeID == "" || ref.PayloadRef == "" || len(r.SupportRefs) == 0 {
		return f, false
	}
	count := 0
	prefix := TraceNoteKeyWakeupEventInstance + "="
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, prefix) {
			count++
			if json.Unmarshal([]byte(strings.TrimPrefix(note, prefix)), &f) != nil {
				return f, false
			}
		}
	}
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	inside := ref.QueryWindowKnown && finite(ref.QueryWindowStartTs) && finite(ref.QueryWindowEndTs) &&
		ref.QueryWindowEndTs > ref.QueryWindowStartTs && f.Timestamp >= ref.QueryWindowStartTs && f.Timestamp < ref.QueryWindowEndTs
	if event {
		// Search selectors are inclusive. The user's half-open window is
		// enforced separately, never inferred from the matched envelope.
		s := f.ScanScope
		inside = s != nil && ValidateTraceEventSearchScanScope(s) &&
			(!s.TimeStartApplied || f.Timestamp >= s.TimeStart) && (!s.TimeEndApplied || f.Timestamp <= s.TimeEnd) &&
			(s.LineStart <= 0 || f.Line >= s.LineStart) && (s.LineEnd <= 0 || f.Line <= s.LineEnd)
	}
	return f, inside && count == 1 && f.IndexPath != "" && f.IndexPath == ref.Path && f.From != "" && f.To != "" && f.From != f.To &&
		f.Waker.PID > 0 && f.Wakee.PID > 0 && f.Line > 0 && f.Branch >= 0 && f.Segment >= 0 && finite(f.Timestamp) &&
		(!ref.QueryLineRangeKnown || ((ref.QueryLineStart <= 0 || f.Line >= ref.QueryLineStart) && (ref.QueryLineEnd <= 0 || f.Line <= ref.QueryLineEnd))) &&
		f.Line == r.Span.LineStart && f.Line == r.Span.LineEnd && f.Timestamp == r.Span.StartTs && f.Timestamp == r.Span.EndTs &&
		RuntimeWakeupThreadLabel(f.Waker) == r.Subject && RuntimeWakeupThreadLabel(f.Wakee) == r.Object &&
		((chain && r.Unit == "ms") || (event && r.Unit == "event" && r.Value == "1")) &&
		r.ClaimKey == r.Predicate+":"+r.Subject+"->"+r.Object
}

// RuntimeWakeupDiagramEvent is one eligible instance after scope and conflict
// checks. Coordinates/fingerprint preserve the existing edge credential bytes.
type RuntimeWakeupDiagramEvent struct {
	Record      ObservationRecord
	Event       RuntimeWakeupEvent
	Coordinates []byte
	Fingerprint []byte
}

func RuntimeDiagramHasNamedBoundedTarget(rm *RequestModel) bool {
	if rm == nil || !rm.RuntimeQuestionProfile.CarriesBoundedFactFamilies() {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if !RuntimeTargetIsExplorationCursorSource(target.Source) &&
			((target.PID > 0 && target.PID <= RuntimeTargetMaxPID) || strings.TrimSpace(target.Thread) != "") {
			return true
		}
	}
	return false
}

// RuntimeWakeupDiagramEvents is shared by answer-shape support and actual edge
// validation. It grants presentation of recorded events, never cause/rank rights.
func RuntimeWakeupDiagramEvents(ledger ObservationLedger, rm *RequestModel) []RuntimeWakeupDiagramEvent {
	if rm != nil && rm.RuntimeArtifactScopeProfile != nil {
		ledger.RuntimeArtifactScopeProfile = rm.RuntimeArtifactScopeProfile
	}
	byEvent := map[string]RuntimeWakeupDiagramEvent{}
	conflict := map[string]bool{}
	for _, r := range ledger.Records {
		f, ok := DecodeRuntimeWakeupEvent(r)
		if !ok {
			continue
		}
		ref := r.SourceRef
		if scope := ledger.RuntimeArtifactScopeProfile; scope != nil && scope.HasExplicitTimeWindows() {
			inside := scope.ContainsExplicitTimeWindow(ref.QueryWindowStartTs, ref.QueryWindowEndTs)
			if r.Predicate == "scheduler_wakeup_event" {
				inside = false
				for _, window := range scope.ExplicitTimeWindows() {
					inside = inside || (f.Timestamp >= *window.TimeStart && f.Timestamp < *window.TimeEnd)
				}
			}
			if !inside {
				continue
			}
		}
		if RuntimeDiagramHasNamedBoundedTarget(rm) {
			selected := r
			selected.Subject = RuntimeWakeupThreadLabel(RuntimeWakeupThread{PID: ref.QueryTargetPID, Comm: ref.QueryTargetThread})
			if r.Predicate == "scheduler_wakeup_event" {
				selected.Subject = r.Object
			}
			if !ObservationRecordMatchesUserRuntimeTarget(selected, rm) {
				continue
			}
		}
		coordinates, _ := json.Marshal([]any{ref.QueryScopeID, ref.PayloadRef, f.IndexPath, ref.QueryWindowStartTs, ref.QueryWindowEndTs,
			ref.QueryTargetPID, ref.QueryTargetThread, ref.QueryTargetScope, ref.QueryLineRangeKnown, ref.QueryLineStart, ref.QueryLineEnd, f.Line, f.Branch, f.Segment})
		key := string(coordinates)
		data, _ := json.Marshal(f)
		if prior, exists := byEvent[key]; exists {
			if string(prior.Fingerprint) != string(data) {
				conflict[key] = true
			}
			continue
		}
		byEvent[key] = RuntimeWakeupDiagramEvent{Record: r, Event: f, Coordinates: coordinates, Fingerprint: data}
	}
	keys := make([]string, 0, len(byEvent))
	for key := range byEvent {
		if !conflict[key] {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := byEvent[keys[i]].Event, byEvent[keys[j]].Event
		if a.Timestamp != b.Timestamp {
			return a.Timestamp < b.Timestamp
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return keys[i] < keys[j]
	})
	rows := make([]RuntimeWakeupDiagramEvent, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, byEvent[key])
	}
	return rows
}
