package types

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/threadidentity"
)

// TraceEventParticipantDisplayRoster admits a recorded participant to an event
// display, not to ownership, a causal chain, or an execution interval. Its
// private entries come only from independently validated row-local receipts.
type TraceEventParticipantDisplayRoster struct {
	events map[string]RuntimeWakeupEvent
}

func NewTraceEventParticipantDisplayRoster(records []ObservationRecord) TraceEventParticipantDisplayRoster {
	out := TraceEventParticipantDisplayRoster{events: map[string]RuntimeWakeupEvent{}}
	conflicts := map[string]bool{}
	for _, r := range records {
		if r.Predicate != "scheduler_wakeup_event" {
			continue // A chain edge is not a receipt for a search inventory row.
		}
		f, ok := DecodeRuntimeWakeupEvent(r)
		if !ok || f.Waker.PID > RuntimeTargetMaxPID || f.Wakee.PID > RuntimeTargetMaxPID {
			continue
		}
		key := traceEventParticipantDisplayKey(r.SourceRef, f.Line, f.Timestamp, "sched_wakeup")
		if prior, exists := out.events[key]; exists && !reflect.DeepEqual(prior, f) {
			conflicts[key] = true
		}
		out.events[key] = f
	}
	for key := range conflicts {
		delete(out.events, key)
	}
	return out
}

func traceEventParticipantDisplayKey(source ObservationSourceRef, line int, timestamp float64, eventType string) string {
	data, _ := json.Marshal([]any{source, line, timestamp, eventType})
	return string(data)
}

// TraceEventRowMatchesDisplayTarget extends only the finite event display. A
// complete source/query/point match and the same executed scan are required;
// semantic display fields and raw text cannot create a participant receipt.
func TraceEventRowMatchesDisplayTarget(record ObservationRecord, row TraceEventSearchInventoryRow, rm *RequestModel, roster TraceEventParticipantDisplayRoster) bool {
	if row.EventType == "sched_wakeup" && TraceEventDisplayRequestsDirectWaker(rm) && traceEventDisplayHasUserTarget(rm) {
		// An idle/unknown emitter may still carry an exact positive wakee TID.
		// Keep that row; the separate causal/diagram receipt may remain absent.
		return traceWakeupRowMatchesReceiver(record, row, rm)
	}
	if TraceEventRowMatchesEmitterTarget(row, rm) {
		return true
	}
	if !IsValidTraceEventSearchInventoryRecord(record) || row.EventType != "sched_wakeup" ||
		row.EmitterTIDKnown == nil || !*row.EmitterTIDKnown || row.EmitterTID <= 0 {
		return false
	}
	member := false
	for _, original := range record.EventSearchInventory.Rows {
		member = member || reflect.DeepEqual(original, row)
	}
	if !member {
		return false
	}
	f, ok := roster.events[traceEventParticipantDisplayKey(record.SourceRef, row.Line, row.TraceTimeSeconds, row.EventType)]
	if !ok || f.Waker.PID != row.EmitterTID || !reflect.DeepEqual(f.ScanScope, record.EventSearchInventory.Coverage.ScanScope) ||
		(row.Comm != "" && f.Waker.Comm != "" && row.Comm != f.Waker.Comm) || !traceEventParticipantFieldsConsistent(row, f) {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if RuntimeTargetIsExplorationCursorSource(target.Source) || target.Kind == RuntimeTargetKindProcess {
			continue // A wakee TID is not its process ID.
		}
		if traceEventDisplayThreadMatches(f.Wakee.PID, f.Wakee.Comm, target) {
			return true
		}
	}
	return false
}

// Retained display fields never create a witness, but an explicit conflicting
// typed value must not be displayed under a different participant's selection.
func traceEventParticipantFieldsConsistent(row TraceEventSearchInventoryRow, f RuntimeWakeupEvent) bool {
	if row.Semantics != nil {
		for _, field := range row.Semantics.Fields {
			if field.Status != "known" || field.Value == nil {
				continue
			}
			switch field.Key {
			case "scheduler.wakee.tid":
				if *field.Value != strconv.Itoa(f.Wakee.PID) {
					return false
				}
			case "scheduler.wakee.comm":
				if *field.Value != f.Wakee.Comm {
					return false
				}
			}
		}
	}
	return true
}

// TraceEventRowMatchesEmitterTarget preserves the original owner-only test for
// callers that actually require the observed emitter. Do not replace those
// callers with the participant display matcher.
func TraceEventRowMatchesEmitterTarget(row TraceEventSearchInventoryRow, rm *RequestModel) bool {
	if !traceEventDisplayHasUserTarget(rm) {
		return true
	}
	for _, target := range rm.RuntimeTargets {
		if RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		if target.Kind == RuntimeTargetKindProcess {
			if row.EmitterTGIDKnown != nil && *row.EmitterTGIDKnown && target.PID > 0 && target.PID <= RuntimeTargetMaxPID && row.EmitterTGID == target.PID {
				return true
			}
			continue
		}
		if row.EmitterTIDKnown != nil && *row.EmitterTIDKnown && row.EmitterTID > 0 && traceEventDisplayThreadMatches(row.EmitterTID, row.Comm, target) {
			return true
		}
	}
	return false
}

func traceEventDisplayThreadMatches(tid int, comm string, target RuntimeTarget) bool {
	if target.PID > 0 || threadidentity.Parse(target.Thread).HasPID {
		return ObservationRecordMatchesUserRuntimeTarget(ObservationRecord{Subject: strconv.Itoa(tid)}, &RequestModel{RuntimeTargets: []RuntimeTarget{target}})
	}
	name := threadidentity.CleanName(target.Thread)
	return name != "" && strings.EqualFold(name, strings.TrimSpace(comm))
}

func traceEventDisplayHasUserTarget(rm *RequestModel) bool {
	if rm != nil {
		for _, target := range rm.RuntimeTargets {
			if !RuntimeTargetIsExplorationCursorSource(target.Source) && ((target.PID > 0 && target.PID <= RuntimeTargetMaxPID) || strings.TrimSpace(target.Thread) != "") {
				return true
			}
		}
	}
	return false
}
