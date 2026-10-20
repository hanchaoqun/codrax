package types

import (
	"reflect"
	"strconv"
)

// This is a display selection over an executed inventory, not a new scheduler
// identity or causal witness. Generic event_search PID remains participation.
// A finite direct-waker question instead displays the recorded receiver.
func TraceEventDisplayRequestsDirectWaker(rm *RequestModel) bool {
	return rm != nil && rm.RuntimeQuestionProfile.BoundedFactSet() &&
		rm.RuntimeQuestionProfile.RequestsFactFamily(RuntimeQuestionFactDirectWaker)
}

func traceWakeupRowMatchesReceiver(record ObservationRecord, row TraceEventSearchInventoryRow, rm *RequestModel) bool {
	if !IsValidTraceEventSearchInventoryRecord(record) || row.Semantics == nil {
		return false
	}
	member := false
	for _, original := range record.EventSearchInventory.Rows {
		member = member || reflect.DeepEqual(original, row)
	}
	if !member {
		return false
	}
	receiver := 0
	comm := ""
	for _, field := range row.Semantics.Fields {
		if field.Key == "scheduler.wakee.comm" && field.Status == "known" && field.Value != nil {
			comm = *field.Value
		}
		if field.Key != "scheduler.wakee.tid" || field.Status != "known" || field.Value == nil {
			continue
		}
		var err error
		receiver, err = strconv.Atoi(*field.Value)
		if err != nil || receiver <= 0 || receiver > RuntimeTargetMaxPID {
			return false
		}
	}
	if receiver <= 0 {
		return false
	}
	for _, target := range rm.RuntimeTargets {
		if target.Kind == RuntimeTargetKindProcess || RuntimeTargetIsExplorationCursorSource(target.Source) {
			continue
		}
		if traceEventDisplayThreadMatches(receiver, comm, target) {
			return true
		}
	}
	return false
}
