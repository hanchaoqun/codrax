package hitraceconv

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type traceDBRawSchedSwitchLiteRecord struct {
	TimestampNS  uint64
	CPU          int
	HeaderPID    int64
	Flags        int64
	PreemptCount int64
	PrevTID      int64
	PrevPriority int64
	PrevState    uint64
	NextTID      int64
	NextPriority int64
	NextInfo     uint64
}

type traceDBRawSchedWakeupLiteRecord struct {
	TimestampNS  uint64
	CPU          int
	HeaderPID    int64
	Flags        int64
	PreemptCount int64
	TargetTID    int64
	Priority     int64
	TargetCPU    int64
}

func decodeTraceDBRawSchedSwitchLite(
	ev decodedEvent,
) (traceDBRawSchedSwitchLiteRecord, string) {
	if !traceDBRawSchedulerLiteLayoutValid(ev,
		"prev_pid", "prev_prio", "prev_state", "next_pid", "next_prio", "next_info") {
		return traceDBRawSchedSwitchLiteRecord{}, "invalid_descriptor_layout"
	}
	prevTID, ok := directCoreSigned(ev, directWidths(4), "prev_pid")
	if !ok || prevTID < 0 || prevTID > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_prev_pid"
	}
	prevPriority, ok := traceDBRawSchedulerLitePriority(ev, "prev_prio")
	if !ok || prevPriority < math.MinInt32 || prevPriority > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_prev_priority"
	}
	prevState, ok := traceDBRawSchedulerLiteUnsigned64(ev, "prev_state")
	if !ok {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_prev_state"
	}
	nextTID, ok := directCoreSigned(ev, directWidths(4), "next_pid")
	if !ok || nextTID < 0 || nextTID > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_next_pid"
	}
	nextPriority, ok := traceDBRawSchedulerLitePriority(ev, "next_prio")
	if !ok || nextPriority < math.MinInt32 || nextPriority > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_next_priority"
	}
	nextInfo, ok := traceDBRawSchedulerLiteUnsigned64(ev, "next_info")
	if !ok {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_next_info"
	}
	return traceDBRawSchedSwitchLiteRecord{
		PrevTID: prevTID, PrevPriority: prevPriority, PrevState: prevState,
		NextTID: nextTID, NextPriority: nextPriority, NextInfo: nextInfo,
	}, ""
}

func decodeTraceDBRawSchedWakeupLite(
	ev decodedEvent,
) (traceDBRawSchedWakeupLiteRecord, string) {
	if !traceDBRawSchedulerLiteLayoutValid(ev, "pid", "prio", "target_cpu") {
		return traceDBRawSchedWakeupLiteRecord{}, "invalid_descriptor_layout"
	}
	targetTID, ok := directCoreSigned(ev, directWidths(4), "pid")
	if !ok || targetTID <= 0 || targetTID > math.MaxInt32 {
		return traceDBRawSchedWakeupLiteRecord{}, "missing_or_invalid_pid"
	}
	priority, ok := traceDBRawSchedulerLitePriority(ev, "prio")
	if !ok || priority < math.MinInt32 || priority > math.MaxInt32 {
		return traceDBRawSchedWakeupLiteRecord{}, "missing_or_invalid_priority"
	}
	targetCPU, ok := directCoreSigned(ev, directWidths(4), "target_cpu")
	if !ok || !validTraceDBCPUIndex(targetCPU) {
		return traceDBRawSchedWakeupLiteRecord{}, "missing_or_invalid_target_cpu"
	}
	return traceDBRawSchedWakeupLiteRecord{
		TargetTID: targetTID, Priority: priority, TargetCPU: targetCPU,
	}, ""
}

func traceDBRawSchedulerLiteLayoutValid(ev decodedEvent, required ...string) bool {
	requiredCounts := make(map[string]int, len(required))
	for _, name := range required {
		if name == "" || requiredCounts[name] != 0 {
			return false
		}
		requiredCounts[name] = 0
	}
	commonCounts := map[string]int{
		"common_type": 0, "common_flags": 0, "common_preempt_count": 0, "common_pid": 0,
	}
	for _, field := range ev.format.Fields {
		name := directCoreFieldBaseName(field.Name)
		if _, ok := commonCounts[name]; ok {
			commonCounts[name]++
		} else if _, ok := requiredCounts[name]; ok {
			requiredCounts[name]++
		} else {
			return false
		}
		if field.Name != name || field.Offset < 0 || field.Size <= 0 ||
			field.Offset > math.MaxInt-field.Size {
			return false
		}
	}
	for _, count := range commonCounts {
		if count != 1 {
			return false
		}
	}
	for _, count := range requiredCounts {
		if count != 1 {
			return false
		}
	}
	for index, field := range ev.format.Fields {
		end := field.Offset + field.Size
		if raw, ok := ev.fields[field.Name]; !ok || len(raw) != field.Size {
			return false
		}
		for prior := 0; prior < index; prior++ {
			other := ev.format.Fields[prior]
			otherEnd := other.Offset + other.Size
			if field.Offset < otherEnd && other.Offset < end {
				return false
			}
		}
	}
	field, raw, ok := directCoreUniqueField(ev, "common_type")
	if !ok || field.Offset != 0 || field.Size != 2 || len(raw) != 2 ||
		field.Signed || directCoreArrayDeclared(field) {
		return false
	}
	switch normalizeFieldType(field.Type) {
	case "unsigned short", "unsigned short int", "short unsigned int",
		"uint16_t", "u16", "__u16":
		return true
	default:
		return false
	}
}

func traceDBRawSchedulerLiteUnsigned64(ev decodedEvent, name string) (uint64, bool) {
	field, raw, ok := directCoreUniqueField(ev, name)
	if !ok || field.Size != 8 || len(raw) != 8 ||
		!directCoreUnsignedWordTypeWidthAllowed(field, 8) {
		return 0, false
	}
	return uintFromSupportedWidth(raw)
}

func traceDBRawSchedulerLitePriority(ev decodedEvent, name string) (int64, bool) {
	field, raw, ok := directCoreUniqueField(ev, name)
	if !ok || field.Size != 2 || len(raw) != 2 {
		return 0, false
	}
	return directCoreSchedulerPriority(ev, name)
}

func traceDBRawSchedSwitchLiteDiagnosticBody(row traceDBRawSchedSwitchLiteRecord) string {
	return fmt.Sprintf(
		"prev_pid=%d prev_prio=%d prev_state=0x%x next_pid=%d next_prio=%d next_info=%s next_info_raw=0x%016x",
		row.PrevTID, row.PrevPriority, row.PrevState,
		row.NextTID, row.NextPriority, formatHarmonySchedInfo(row.NextInfo, true), row.NextInfo,
	)
}

func traceDBRawSchedSwitchLiteNextInfoUnknownTail(row traceDBRawSchedSwitchLiteRecord) bool {
	const knownBits = uint64(1)<<53 - 1
	return row.NextInfo & ^knownBits != 0
}

func traceDBRawSchedWakeupLiteDiagnosticBody(row traceDBRawSchedWakeupLiteRecord) string {
	return fmt.Sprintf("pid=%d prio=%d target_cpu=%03d",
		row.TargetTID, row.Priority, row.TargetCPU)
}

func traceDBAttachRawSchedulerLiteDiagnostics(
	out *TraceDBCoverage,
	decode TraceDBCoverage,
	eventName string,
) {
	if out == nil || !traceDBRawSchedulerLiteFormat(eventName) {
		return
	}
	if geometry := decode.Metadata["scheduler_lite_format_geometry_witnesses"]; geometry != "" {
		out.Metadata["scheduler_lite_format_geometry_witnesses"] = geometry
	}
	prefix := "target_" + eventName + "_"
	keys := make([]string, 0, len(decode.Metrics))
	for key := range decode.Metrics {
		if strings.HasPrefix(key, prefix) &&
			(key == prefix+"records" ||
				key == prefix+"body_admitted" ||
				key == prefix+"body_rejected" ||
				strings.HasPrefix(key, prefix+"reject_")) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts,
			fmt.Sprintf("%s=%d", strings.TrimPrefix(key, prefix), decode.Metrics[key]))
	}
	if len(parts) > 0 {
		out.Metadata["source_decoder_census"] = strings.Join(parts, ",")
	}
}
