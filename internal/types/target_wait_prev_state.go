package types

import (
	"strconv"
	"strings"
)

// DisplayLine adds optional native state evidence without changing the
// canonical measurement identity used by roster matching and fingerprints.
func (r TargetWaitOccurrenceAuthorityRow) DisplayLine() string {
	line := r.CanonicalLine()
	if r.PrevStateRaw != "" && !r.prevStateRawConflicted {
		line += " prev_state_raw=" + r.PrevStateRaw
	}
	return line
}

// Conflicts are sticky across note, duplicate leaf, preview/full and same-scope
// merges. Missing metadata does not contradict a known value. Neither case
// changes the original roster's measurement or admission.
type targetWaitOccurrenceRawState struct {
	value      string
	conflicted bool
}

func (a targetWaitOccurrenceRawState) merge(b targetWaitOccurrenceRawState) targetWaitOccurrenceRawState {
	if a.conflicted || b.conflicted || (a.value != "" && b.value != "" && a.value != b.value) {
		return targetWaitOccurrenceRawState{conflicted: true}
	}
	if a.value == "" {
		return b
	}
	return a
}

func targetWaitOccurrenceRawFromTokens(tokens []string, prefix string) targetWaitOccurrenceRawState {
	var state targetWaitOccurrenceRawState
	for _, token := range tokens {
		value, ok := strings.CutPrefix(strings.TrimSpace(token), prefix)
		if !ok {
			continue
		}
		// A scheduler state is a single native token. Malformed optional
		// metadata may suppress its display, but never the base measurement.
		value = strings.TrimSpace(value)
		next := targetWaitOccurrenceRawState{value: value}
		if len(strings.Fields(value)) > 1 {
			next = targetWaitOccurrenceRawState{conflicted: true}
		}
		state = state.merge(next)
	}
	return state
}

func (r TargetWaitOccurrenceAuthorityRow) prevStateRaw() targetWaitOccurrenceRawState {
	return targetWaitOccurrenceRawState{value: r.PrevStateRaw, conflicted: r.prevStateRawConflicted}
}

func (r *TargetWaitOccurrenceAuthorityRow) setPrevStateRaw(state targetWaitOccurrenceRawState) {
	r.PrevStateRaw, r.prevStateRawConflicted = state.value, state.conflicted
}

// Callers already established the original same-result/scope boundary. Only
// exact canonical rows can exchange optional metadata within that boundary.
func mergeTargetWaitOccurrenceRawRows(rows, other []TargetWaitOccurrenceAuthorityRow) []TargetWaitOccurrenceAuthorityRow {
	if len(rows) == 0 || len(other) == 0 {
		return rows
	}
	states := make(map[string]targetWaitOccurrenceRawState, len(other))
	for _, row := range other {
		key := row.CanonicalLine()
		states[key] = states[key].merge(row.prevStateRaw())
	}
	out := append([]TargetWaitOccurrenceAuthorityRow(nil), rows...)
	for i := range out {
		out[i].setPrevStateRaw(out[i].prevStateRaw().merge(states[out[i].CanonicalLine()]))
	}
	return out
}

func targetWaitOccurrenceRawRosterMatches(preview TargetWaitOccurrenceAuthority, full TraceTargetWaitSummaryAuthority) bool {
	if preview.Count != full.Count || len(preview.Rows) != len(full.Occurrences) ||
		strconv.FormatFloat(preview.SumMS, 'f', 3, 64) != strconv.FormatFloat(full.WallClockMS, 'f', 3, 64) {
		return false
	}
	for i, row := range preview.Rows {
		if row.CanonicalLine() != full.Occurrences[i].CanonicalLine() {
			return false
		}
	}
	return true
}

func targetWaitOccurrenceRawPreviewRows(record ObservationRecord, full TraceTargetWaitSummaryAuthority) []TargetWaitOccurrenceAuthorityRow {
	preview, valid := targetWaitOccurrenceAuthorityFromRecord(record)
	if !valid || !targetWaitOccurrenceRawRosterMatches(preview, full) {
		return nil
	}
	return preview.Rows
}

// This receipt is only for optional metadata, never observation hiding. Keep
// the original result identity after the independent safe-ID filter removes a
// colliding ID, so a known metadata conflict cannot reappear in a preview.
type targetWaitOccurrenceRawReceipt struct {
	record ObservationRecord
	scope  TraceRuntimeAccountScope
}

type targetWaitOccurrenceRawMatch struct {
	fullIndex int
	source    targetWaitOccurrenceRawReceipt
}

func newTargetWaitOccurrenceRawReceipt(record ObservationRecord) targetWaitOccurrenceRawReceipt {
	return targetWaitOccurrenceRawReceipt{
		record: ObservationRecord{ID: record.ID, Producer: record.Producer, SourceRef: record.SourceRef, ObservedAt: record.ObservedAt},
		scope:  TraceRuntimeAccountRecordScope(record),
	}
}

func (r targetWaitOccurrenceRawReceipt) matches(record ObservationRecord) bool {
	scope := TraceRuntimeAccountRecordScope(record)
	return r.record.ID == record.ID && r.scope.Complete() && scope.Complete() &&
		r.scope.ArtifactKey == scope.ArtifactKey && r.scope.Subject == scope.Subject &&
		r.scope.WindowStartTs == scope.WindowStartTs && r.scope.WindowEndTs == scope.WindowEndTs &&
		TraceRuntimeAccountRecordsSameResult(r.record, record)
}
