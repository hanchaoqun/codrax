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

type traceDBRawSchedWakeupNewNameRecord struct {
	TimestampNS uint64
	TargetTID   int64
	Name        string
}

type traceDBRawSchedSwitchLiteFields struct {
	PrevTID      string
	PrevPriority string
	PrevState    string
	NextTID      string
	NextPriority string
	NextInfo     string
	Packed       bool
}

func decodeTraceDBRawSchedSwitchLite(
	ev decodedEvent,
) (traceDBRawSchedSwitchLiteRecord, string) {
	fields, ok := traceDBRawSchedSwitchLiteProfile(ev)
	if !ok {
		return traceDBRawSchedSwitchLiteRecord{}, "invalid_descriptor_layout"
	}
	prevTID, ok := directCoreSigned(ev, directWidths(4), fields.PrevTID)
	if !ok || prevTID < 0 || prevTID > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_prev_pid"
	}
	prevPriority, ok := traceDBRawSchedulerLitePriority(ev, fields.PrevPriority)
	if !ok || prevPriority < math.MinInt32 || prevPriority > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_prev_priority"
	}
	prevState, ok := traceDBRawSchedulerLiteState(ev, fields.PrevState, fields.Packed)
	if !ok {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_prev_state"
	}
	nextTID, ok := directCoreSigned(ev, directWidths(4), fields.NextTID)
	if !ok || nextTID < 0 || nextTID > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_next_pid"
	}
	nextPriority, ok := traceDBRawSchedulerLitePriority(ev, fields.NextPriority)
	if !ok || nextPriority < math.MinInt32 || nextPriority > math.MaxInt32 {
		return traceDBRawSchedSwitchLiteRecord{}, "missing_or_invalid_next_priority"
	}
	nextInfo, ok := traceDBRawSchedulerLiteNextInfo(ev, fields.NextInfo, fields.Packed)
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
	targetCPU, ok := directCoreSchedulerTargetCPU(ev, "target_cpu")
	if !ok || !validTraceDBCPUIndex(targetCPU) {
		return traceDBRawSchedWakeupLiteRecord{}, "missing_or_invalid_target_cpu"
	}
	return traceDBRawSchedWakeupLiteRecord{
		TargetTID: targetTID, Priority: priority, TargetCPU: targetCPU,
	}, ""
}

func traceDBRawSchedulerLiteLayoutValid(ev decodedEvent, required ...string) bool {
	return traceDBRawSchedulerLiteLayoutValidProfile(ev, false, required...)
}

func traceDBRawSchedulerLiteLayoutValidProfile(
	ev decodedEvent,
	allowPackedNextInfoArray bool,
	required ...string,
) bool {
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
		exactName := field.Name == name
		if allowPackedNextInfoArray && name == "ninfo" && field.Name == "ninfo[8]" {
			exactName = true
		}
		if !exactName || field.Offset < 0 || field.Size <= 0 ||
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

func traceDBRawSchedSwitchLiteProfile(
	ev decodedEvent,
) (traceDBRawSchedSwitchLiteFields, bool) {
	canonical := traceDBRawSchedSwitchLiteFields{
		PrevTID: "prev_pid", PrevPriority: "prev_prio", PrevState: "prev_state",
		NextTID: "next_pid", NextPriority: "next_prio", NextInfo: "next_info",
	}
	packed := traceDBRawSchedSwitchLiteFields{
		PrevTID: "prev_tid", PrevPriority: "pprio", PrevState: "pstate",
		NextTID: "next_tid", NextPriority: "nprio", NextInfo: "ninfo",
		Packed: true,
	}
	canonicalOK := traceDBRawSchedulerLiteLayoutValidProfile(ev, false,
		canonical.PrevTID, canonical.PrevPriority, canonical.PrevState,
		canonical.NextTID, canonical.NextPriority, canonical.NextInfo)
	packedOK := traceDBRawSchedulerLiteLayoutValidProfile(ev, true,
		packed.PrevTID, packed.PrevPriority, packed.PrevState,
		packed.NextTID, packed.NextPriority, packed.NextInfo)
	if canonicalOK == packedOK {
		return traceDBRawSchedSwitchLiteFields{}, false
	}
	if canonicalOK {
		return canonical, true
	}
	return packed, true
}

func traceDBRawSchedulerLiteUnsigned64(ev decodedEvent, name string) (uint64, bool) {
	field, raw, ok := directCoreUniqueField(ev, name)
	if !ok || field.Size != 8 || len(raw) != 8 ||
		!directCoreUnsignedWordTypeWidthAllowed(field, 8) {
		return 0, false
	}
	return uintFromSupportedWidth(raw)
}

func traceDBRawSchedulerLiteState(ev decodedEvent, name string, packed bool) (uint64, bool) {
	if !packed {
		return traceDBRawSchedulerLiteUnsigned64(ev, name)
	}
	field, raw, ok := directCoreUniqueField(ev, name)
	if !ok || field.Name != "pstate" || field.Size != 2 || len(raw) != 2 ||
		field.Signed || directCoreArrayDeclared(field) {
		return 0, false
	}
	switch normalizeFieldType(field.Type) {
	case "unsigned short", "unsigned short int", "short unsigned int",
		"uint16_t", "u16", "__u16":
		return uintFromSupportedWidth(raw)
	default:
		return 0, false
	}
}

func traceDBRawSchedulerLiteNextInfo(ev decodedEvent, name string, packed bool) (uint64, bool) {
	if !packed {
		return traceDBRawSchedulerLiteUnsigned64(ev, name)
	}
	field, raw, ok := directCoreUniqueField(ev, name)
	if !ok || field.Size != 8 || len(raw) != 8 || field.Signed ||
		normalizeFieldType(field.Type) != "unsigned char" ||
		(field.Name != "ninfo" && field.Name != "ninfo[8]") {
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
	return traceDBRawSchedSwitchLiteNextInfoUnknownTailBits(row) != 0
}

func traceDBRawSchedSwitchLiteNextInfoUnknownTailBits(
	row traceDBRawSchedSwitchLiteRecord,
) uint64 {
	const knownBits = uint64(1)<<53 - 1
	return row.NextInfo & ^knownBits
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
	if eventName == "sched_wakeup_lite" {
		if geometry := decode.Metadata["scheduler_wakeup_format_geometry_witnesses"]; geometry != "" {
			out.Metadata["scheduler_wakeup_format_geometry_witnesses"] = geometry
		}
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
