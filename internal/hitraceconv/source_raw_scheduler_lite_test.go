package hitraceconv

import (
	"encoding/binary"
	"strings"
	"testing"
)

func traceDBRawSchedulerLiteTestEvent(name string, fields []eventField, content []byte) decodedEvent {
	return decodeEvent(eventFormat{ID: 1, Name: name, Fields: fields}, content)
}

func traceDBRawSchedulerLiteCommonFields() []eventField {
	return []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
}

func TestDecodeTraceDBRawSchedulerLiteUsesClosedOfficialProfiles(t *testing.T) {
	switchFields := append(traceDBRawSchedulerLiteCommonFields(),
		eventField{Type: "int", Name: "prev_pid", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "short", Name: "prev_prio", Offset: 12, Size: 2, Signed: true},
		eventField{Type: "unsigned long long", Name: "prev_state", Offset: 16, Size: 8},
		eventField{Type: "int", Name: "next_pid", Offset: 24, Size: 4, Signed: true},
		eventField{Type: "short", Name: "next_prio", Offset: 28, Size: 2, Signed: true},
		eventField{Type: "unsigned long long", Name: "next_info", Offset: 32, Size: 8},
	)
	content := make([]byte, 40)
	binary.LittleEndian.PutUint32(content[8:12], 10)
	binary.LittleEndian.PutUint16(content[12:14], 0xfffe)
	binary.LittleEndian.PutUint64(content[16:24], 0x100)
	binary.LittleEndian.PutUint32(content[24:28], 20)
	binary.LittleEndian.PutUint16(content[28:30], 53)
	binary.LittleEndian.PutUint64(content[32:40], 0x000e780000003fff)
	row, reason := decodeTraceDBRawSchedSwitchLite(
		traceDBRawSchedulerLiteTestEvent("sched_switch_lite", switchFields, content),
	)
	if reason != "" || row.PrevTID != 10 || row.PrevPriority != -2 ||
		row.PrevState != 0x100 || row.NextTID != 20 ||
		row.NextPriority != 53 || row.NextInfo != 0x000e780000003fff ||
		!strings.Contains(traceDBRawSchedSwitchLiteDiagnosticBody(row), "next_info=3fff,") ||
		!strings.Contains(traceDBRawSchedSwitchLiteDiagnosticBody(row),
			"next_info_raw=0x000e780000003fff") ||
		traceDBRawSchedSwitchLiteNextInfoUnknownTail(row) {
		t.Fatalf("official sched_switch_lite decode mismatch: row=%+v reason=%q", row, reason)
	}
	row.NextInfo |= uint64(1) << 60
	if !traceDBRawSchedSwitchLiteNextInfoUnknownTail(row) {
		t.Fatal("future packed next_info tail bits were silently discarded")
	}

	wakeupFields := append(traceDBRawSchedulerLiteCommonFields(),
		eventField{Type: "int", Name: "pid", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "short", Name: "prio", Offset: 12, Size: 2, Signed: true},
		eventField{Type: "int", Name: "target_cpu", Offset: 16, Size: 4, Signed: true},
	)
	wakeupContent := make([]byte, 20)
	binary.LittleEndian.PutUint32(wakeupContent[8:12], 20)
	binary.LittleEndian.PutUint16(wakeupContent[12:14], 53)
	binary.LittleEndian.PutUint32(wakeupContent[16:20], 3)
	wakeup, reason := decodeTraceDBRawSchedWakeupLite(
		traceDBRawSchedulerLiteTestEvent("sched_wakeup_lite", wakeupFields, wakeupContent),
	)
	if reason != "" || wakeup.TargetTID != 20 || wakeup.Priority != 53 || wakeup.TargetCPU != 3 {
		t.Fatalf("official sched_wakeup_lite decode mismatch: row=%+v reason=%q", wakeup, reason)
	}
}

func TestDecodeTraceDBRawSchedulerLiteRejectsNearProfiles(t *testing.T) {
	valid := append(traceDBRawSchedulerLiteCommonFields(),
		eventField{Type: "int", Name: "pid", Offset: 8, Size: 4, Signed: true},
		eventField{Type: "short", Name: "prio", Offset: 12, Size: 2, Signed: true},
		eventField{Type: "int", Name: "target_cpu", Offset: 16, Size: 4, Signed: true},
	)
	for _, mutate := range []func([]eventField) []eventField{
		func(fields []eventField) []eventField {
			fields[0].Type, fields[0].Signed = "short", true
			return fields
		},
		func(fields []eventField) []eventField {
			fields[5].Type, fields[5].Size = "int", 4
			return fields
		},
		func(fields []eventField) []eventField {
			fields = append(fields, eventField{Type: "int", Name: "success", Offset: 20, Size: 4, Signed: true})
			return fields
		},
		func(fields []eventField) []eventField {
			fields[6].Offset = 12
			return fields
		},
		func(fields []eventField) []eventField {
			fields[4].Name = "wakee_pid"
			return fields
		},
	} {
		fields := append([]eventField(nil), valid...)
		content := make([]byte, 24)
		if _, reason := decodeTraceDBRawSchedWakeupLite(
			traceDBRawSchedulerLiteTestEvent("sched_wakeup_lite", mutate(fields), content),
		); reason == "" {
			t.Fatalf("near sched_wakeup_lite profile gained authority: %+v", fields)
		}
	}
}
