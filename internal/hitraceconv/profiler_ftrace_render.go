package hitraceconv

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type profilerFtraceEventRecord struct {
	CPU                  int64
	TSNS                 uint64
	TGID                 int64
	PID                  int64
	CommonFlags          int64
	CommonPreemptCount   int64
	Comm                 string
	Field                int
	Payload              []byte
	EnvelopeDegradations []string
}

const profilerFtraceCPUDetailEnvelopeField = -1

func renderProfilerFtraceStructuredRows(data []byte, seq *int, sink *traceDBRowSink) (int, []TraceDBCoverage, error) {
	return renderProfilerFtraceStructuredResult(decodeProfilerTracePluginResult(data), seq, sink)
}

func renderProfilerFtraceStructuredResult(result profilerTracePluginResult, seq *int, sink *traceDBRowSink) (int, []TraceDBCoverage, error) {
	topLevelCoverage := profilerTracePluginResultCoverage(result)
	events, err := profilerTracePluginResultEvents(result)
	if err != nil {
		return 0, topLevelCoverage, err
	}
	coverageByField := map[int]*TraceDBCoverage{}
	degradationsByField := map[int]map[string]int{}
	rows := 0
	for _, event := range events {
		coverage := profilerFtraceEventRenderCoverage(coverageByField, event.Field)
		coverage.RowsRead++
		name, body, ok, degradations := renderProfilerFtraceEventBodyWithAudit(event)
		if len(degradations) > 0 {
			counts := degradationsByField[event.Field]
			if counts == nil {
				counts = map[string]int{}
				degradationsByField[event.Field] = counts
			}
			for _, reason := range degradations {
				counts[reason]++
			}
		}
		if !ok {
			if coverage.Skipped == "" {
				coverage.Skipped = "structured ftrace renderer pending"
			}
			continue
		}
		task := firstNonEmpty(event.Comm, "unknown")
		row, err := prepareTraceDBRenderedRowWithTraceFlags(int64(event.TSNS), *seq, task, event.PID,
			event.TGID, event.CPU, event.CommonFlags, event.CommonPreemptCount, name+": "+body)
		if err != nil {
			return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), err
		}
		if err := sink.add(row); err != nil {
			return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), err
		}
		(*seq)++
		rows++
		coverage.RowsEmitted++
	}
	for field, counts := range degradationsByField {
		coverage := profilerFtraceEventRenderCoverage(coverageByField, field)
		coverage.Skipped = traceDBCountSummary(counts)
		if coverage.FieldSources == nil {
			coverage.FieldSources = map[string]string{}
		}
		for reason, count := range counts {
			coverage.FieldSources["degraded_"+reason+"_rows"] = strconv.Itoa(count)
		}
	}
	return rows, append(topLevelCoverage, profilerFtraceEventRenderCoverageList(coverageByField)...), nil
}

func decodeProfilerFtraceStructuredEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	return profilerTracePluginResultEvents(decodeProfilerTracePluginResult(data))
}

func decodeProfilerFtraceCPUDetailEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	var cpu uint64
	cpuCount := 0
	cpuWrongWire := false
	var eventPayloads [][]byte
	eventWrongWire := 0
	overwriteCount := 0
	overwriteWrongWire := false
	var out []profilerFtraceEventRecord
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			cpuCount++
			if wire != 0 {
				cpuWrongWire = true
				break
			}
			cpu = v
		case 2:
			if wire != 2 {
				eventWrongWire++
				break
			}
			eventPayloads = append(eventPayloads, raw)
		case 3:
			overwriteCount++
			if wire != 0 {
				overwriteWrongWire = true
			}
		}
		return nil
	})
	if err != nil {
		return []profilerFtraceEventRecord{{
			Field:                profilerFtraceCPUDetailEnvelopeField,
			EnvelopeDegradations: []string{"envelope_cpu_detail_malformed_wire"},
		}}, nil
	}
	var cpuDegradations []string
	switch {
	case cpuCount > 1:
		cpuDegradations = append(cpuDegradations, "envelope_cpu_duplicate")
	case cpuWrongWire:
		cpuDegradations = append(cpuDegradations, "envelope_cpu_wrong_wire")
	case cpu > uint64(maxTraceDBCPUIndex):
		cpuDegradations = append(cpuDegradations, "envelope_cpu_out_of_range")
	}
	// FtraceParser and FlowController always call set_cpu. Proto3 omits an
	// exact zero, so absence is CPU 0 only for this pinned producer profile.
	for _, raw := range eventPayloads {
		event, decodeErr := decodeProfilerFtraceEventRecord(cpu, raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		event.EnvelopeDegradations = append(event.EnvelopeDegradations, cpuDegradations...)
		out = append(out, event)
	}
	if len(eventPayloads) == 0 && len(cpuDegradations) > 0 {
		out = append(out, profilerFtraceEventRecord{
			Field:                profilerFtraceCPUDetailEnvelopeField,
			EnvelopeDegradations: append([]string(nil), cpuDegradations...),
		})
	}
	if eventWrongWire > 0 {
		out = append(out, profilerFtraceEventRecord{
			Field:                profilerFtraceCPUDetailEnvelopeField,
			EnvelopeDegradations: []string{"envelope_event_container_wrong_wire"},
		})
	}
	if overwriteCount > 1 || overwriteWrongWire {
		out = append(out, profilerFtraceEventRecord{
			Field:                profilerFtraceCPUDetailEnvelopeField,
			EnvelopeDegradations: []string{"envelope_overwrite_invalid"},
		})
	}
	return out, nil
}

type profilerProtoEnvelopeField struct {
	count     int
	wrongWire bool
	uintValue uint64
	bytes     []byte
}

func decodeProfilerFtraceEventRecord(cpu uint64, data []byte) (profilerFtraceEventRecord, error) {
	record := profilerFtraceEventRecord{CPU: int64(cpu)}
	var timestamp, tgid, comm, common profilerProtoEnvelopeField
	oneofCount := 0
	oneofWrongWire := false
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			profilerProtoEnvelopeSetUint(&timestamp, wire, v)
		case 2:
			profilerProtoEnvelopeSetUint(&tgid, wire, v)
		case 3:
			profilerProtoEnvelopeSetBytes(&comm, wire, raw)
		case 50:
			profilerProtoEnvelopeSetBytes(&common, wire, raw)
		default:
			if field >= 100 {
				oneofCount++
				if wire != 2 {
					oneofWrongWire = true
					break
				}
				record.Field = field
				record.Payload = raw
			}
		}
		return nil
	})
	if err != nil {
		record.Field = 0
		record.Payload = nil
		record.EnvelopeDegradations = []string{"envelope_event_malformed_wire"}
		return record, nil
	}
	if reason := profilerProtoEnvelopeOptionalIssue("envelope_timestamp", timestamp); reason != "" {
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, reason)
	} else {
		record.TSNS = timestamp.uintValue
		if record.TSNS > math.MaxInt64 {
			record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_timestamp_out_of_range")
		}
	}
	if reason := profilerProtoEnvelopeOptionalIssue("envelope_tgid", tgid); reason != "" {
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, reason)
	} else if tgid.uintValue > math.MaxInt32 {
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_tgid_out_of_range")
	} else {
		record.TGID = int64(tgid.uintValue)
	}
	if reason := profilerProtoEnvelopeOptionalIssue("envelope_comm", comm); reason != "" {
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, reason)
	} else {
		record.Comm = string(comm.bytes)
		if comm.count == 1 && !traceDBSinglePhysicalLine(record.Comm, true) {
			record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_comm_invalid")
		}
	}
	if reason := profilerProtoEnvelopeRequiredIssue("envelope_common_fields", common); reason != "" {
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, reason)
	} else {
		pid, flags, preempt, reasons := decodeProfilerFtraceCommonFields(common.bytes)
		record.PID = pid
		record.CommonFlags = flags
		record.CommonPreemptCount = preempt
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, reasons...)
	}
	switch {
	case oneofCount == 0:
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_oneof_missing")
	case oneofCount > 1:
		record.Field = 0
		record.Payload = nil
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_oneof_multiple")
	case oneofWrongWire:
		record.Field = 0
		record.Payload = nil
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_oneof_wrong_wire")
	}
	// Upstream only resolves/sets TGID for a non-idle PID and may honestly
	// leave it at proto3 zero when the process map has no answer. Preserve that
	// as the systrace "-----" TGID, never fabricate TGID=TID. The inverse
	// shape (idle PID with a positive TGID) is not producer-reachable.
	if record.PID == 0 && record.TGID != 0 {
		record.EnvelopeDegradations = append(record.EnvelopeDegradations, "envelope_identity_incomplete")
	}
	return record, nil
}

func profilerProtoEnvelopeSetUint(field *profilerProtoEnvelopeField, wire int, value uint64) {
	field.count++
	if wire != 0 {
		field.wrongWire = true
		return
	}
	field.uintValue = value
}

func profilerProtoEnvelopeSetBytes(field *profilerProtoEnvelopeField, wire int, value []byte) {
	field.count++
	if wire != 2 {
		field.wrongWire = true
		return
	}
	field.bytes = value
}

func profilerProtoEnvelopeOptionalIssue(prefix string, field profilerProtoEnvelopeField) string {
	if field.count > 1 {
		return prefix + "_duplicate"
	}
	if field.wrongWire {
		return prefix + "_wrong_wire"
	}
	return ""
}

func profilerProtoEnvelopeRequiredIssue(prefix string, field profilerProtoEnvelopeField) string {
	if field.count == 0 {
		return prefix + "_missing"
	}
	return profilerProtoEnvelopeOptionalIssue(prefix, field)
}

func decodeProfilerFtraceCommonFields(data []byte) (pid, flags, preempt int64, reasons []string) {
	var fields [5]profilerProtoEnvelopeField
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		if field >= 1 && field <= 4 {
			profilerProtoEnvelopeSetUint(&fields[field], wire, v)
		}
		_ = raw
		return nil
	})
	if err != nil {
		return 0, 0, 0, []string{"envelope_common_fields_malformed_wire"}
	}
	for _, item := range []struct {
		field  int
		prefix string
	}{
		{field: 1, prefix: "envelope_common_type"},
		{field: 2, prefix: "envelope_common_flags"},
		{field: 3, prefix: "envelope_common_preempt_count"},
		{field: 4, prefix: "envelope_common_pid"},
	} {
		if reason := profilerProtoEnvelopeOptionalIssue(item.prefix, fields[item.field]); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	if fields[1].count == 1 && !fields[1].wrongWire && fields[1].uintValue > math.MaxUint16 {
		reasons = append(reasons, "envelope_common_type_source_width")
	}
	if fields[2].uintValue > math.MaxUint8 {
		reasons = append(reasons, "envelope_common_flags_source_width")
	}
	if fields[3].uintValue > math.MaxUint8 {
		reasons = append(reasons, "envelope_common_preempt_count_source_width")
	}
	if fields[4].count == 1 && !fields[4].wrongWire && fields[4].uintValue > math.MaxInt32 {
		reasons = append(reasons, "envelope_common_pid_out_of_range")
	}
	return int64(fields[4].uintValue), int64(fields[2].uintValue), int64(fields[3].uintValue), reasons
}

func renderProfilerFtraceEventBody(event profilerFtraceEventRecord) (string, string, bool) {
	switch event.Field {
	case 202, 204, 205, 209, 210, 211, 212:
		name, body, ok, _ := renderProfilerBlockEvent(event)
		return name, body, ok
	case 410:
		// Field 410 is clk.proto ClkSetRateFormat{name, rate}; unlike the
		// power event at field 2002 it has no cpu_id. Keep the established
		// clock_set_rate text alias, but never synthesize a CPU dimension.
		return "clock_set_rate", fmt.Sprintf("%s state=%d", protoString(event.Payload, 1), protoUint(event.Payload, 2)), true
	case 2002:
		parts := []string{protoString(event.Payload, 1), fmt.Sprintf("state=%d", protoUint(event.Payload, 2))}
		cpuID, state, _ := protoScalarUint(event.Payload, 3)
		switch state {
		case protoScalarAbsent:
			// FtraceEventProcessor always sets ClockSetRateFormat.cpu_id.
			// Proto3 omits an exact zero scalar from the wire, so absence in
			// this pinned producer profile is authoritative CPU 0.
			parts = appendClockSetRateCPU(parts, 0)
		case protoScalarPresent:
			parts = appendClockSetRateCPU(parts, cpuID)
		}
		return "clock_set_rate", strings.Join(parts, " "), true
	case 1000:
		body, ok := renderProfilerFtraceFilemapPageCache(event.Payload)
		return "mm_filemap_add_to_page_cache", body, ok
	case 1001:
		body, ok := renderProfilerFtraceFilemapPageCache(event.Payload)
		return "mm_filemap_delete_from_page_cache", body, ok
	case 1109:
		return "print", protoString(event.Payload, 2), true
	case 2417:
		body := fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			protoString(event.Payload, 1), protoInt(event.Payload, 2), protoInt(event.Payload, 3),
			linuxPrevState(protoUint(event.Payload, 4)), protoString(event.Payload, 5), protoInt(event.Payload, 6), protoInt(event.Payload, 7))
		nextInfo, state, _ := protoScalarUint(event.Payload, 8)
		// The pinned producer writes MaxUint64 when the source format has no
		// next_info. Proto3 omits an exact packed zero, so wire absence is the
		// authoritative zero tuple only for this exact field-2417 profile.
		if state == protoScalarAbsent {
			nextInfo = 0
			state = protoScalarPresent
		}
		if state == protoScalarPresent && nextInfo != ^uint64(0) {
			body += " next_info=" + formatHarmonySchedInfo(nextInfo, true)
		}
		return "sched_switch", body, true
	case 4009:
		return "f2fs_sync_file_enter", renderProfilerFtraceF2FS(event.Payload, false, 2, 5, 0), true
	case 4010:
		return "f2fs_sync_file_exit", renderProfilerFtraceF2FS(event.Payload, true, 2, 0, 5), true
	case 4011:
		return "f2fs_write_begin", renderProfilerFtraceF2FS(event.Payload, false, 2, 4, 0), true
	case 4012:
		return "f2fs_write_end", renderProfilerFtraceF2FS(event.Payload, false, 2, 4, 0), true
	case 4015:
		return "mmc_request_done", fmt.Sprintf("%s tag=%d opcode=%d bytes_xfered=%d ret=%d", protoString(event.Payload, 23), protoInt(event.Payload, 15), protoUint(event.Payload, 1), protoUint(event.Payload, 13), protoInt(event.Payload, 14)), true
	case 4016:
		return "mmc_request_start", fmt.Sprintf("%s tag=%d opcode=%d blocks=%d block_size=%d blk_addr=%d", protoString(event.Payload, 25), protoInt(event.Payload, 17), protoUint(event.Payload, 1), protoUint(event.Payload, 13), protoUint(event.Payload, 15), protoUint(event.Payload, 14)), true
	default:
		return "", "", false
	}
}

func safeProfilerBlockedCaller(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || len(value) > 512 {
		return "", false
	}
	for _, r := range value {
		// caller= is one systrace key/value token. Whitespace, equals, pipes and
		// controls would either truncate the parser-visible reason or inject a
		// second trace field/line, so such payloads fail closed to opaque.
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '=' || r == '|' {
			return "", false
		}
	}
	return value, true
}

func renderProfilerFtraceFilemapPageCache(data []byte) (string, bool) {
	index := protoUint(data, 3)
	sDev := protoUint(data, 4)
	if sDev > math.MaxUint32 || index > ^uint64(0)>>12 {
		return "", false
	}
	// The OpenHarmony profiler schema exposes pfn/inode/index/device only; it
	// has no page pointer. Reuse the direct-ftrace formatter with pagePresent
	// false so no fabricated zero-valued page token can enter public systrace.
	return renderFilemapPageCacheBody(
		devMajorMinor(int64(sDev), ":"),
		protoUint(data, 2),
		0,
		false,
		protoUint(data, 1),
		index<<12,
	), true
}

func renderProfilerFtraceF2FS(data []byte, includeRet bool, inoField int, lenField int, retField int) string {
	parts := []string{
		fmt.Sprintf("dev=%s", devMajorMinor(int64(protoUint(data, 1)), ":")),
		fmt.Sprintf("ino=0x%x", protoUint(data, inoField)),
	}
	if offset := protoUint(data, 3); offset != 0 {
		parts = append(parts, fmt.Sprintf("offset=%d", offset))
	}
	if lenField != 0 {
		parts = append(parts, fmt.Sprintf("len=%d", protoUint(data, lenField)))
	}
	parts = append(parts, "rw=write")
	if includeRet {
		parts = append(parts, fmt.Sprintf("ret=%d", protoInt(data, retField)))
	}
	return strings.Join(parts, " ")
}

func protoUint(data []byte, field int) uint64 {
	var out uint64
	_ = walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f == field && wire == 0 {
			out = v
		}
		_ = raw
		return nil
	})
	return out
}

func protoInt(data []byte, field int) int64 {
	return int64(protoUint(data, field))
}

func protoString(data []byte, field int) string {
	var out string
	_ = walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f == field && wire == 2 {
			out = string(raw)
		}
		_ = v
		return nil
	})
	return out
}

func renderProfilerFtraceEventBodyWithAudit(event profilerFtraceEventRecord) (string, string, bool, []string) {
	if len(event.EnvelopeDegradations) > 0 {
		return "", "", false, append([]string(nil), event.EnvelopeDegradations...)
	}
	corePayload, admission, reason, degradations := decodeProfilerCorePayload(event)
	switch admission {
	case bodyAdmitted:
		body, ok := renderCanonicalCorePayload(corePayload)
		if !ok {
			return "", "", false, []string{"invalid_canonical_core_payload"}
		}
		if !profilerCoreCanonicalLineValid(event, corePayload.Name, body) {
			return "", "", false, []string{"invalid_canonical_core_line"}
		}
		return corePayload.Name, body, true, degradations
	case bodyRejected:
		return "", "", false, []string{reason}
	}
	if _, _, blockEvent := blockRenderKindForProfilerField(event.Field); blockEvent {
		return renderProfilerBlockEvent(event)
	}
	coreOK, degradations := profilerFtraceCoreWireAudit(event)
	if !coreOK {
		return "", "", false, degradations
	}
	name, body, ok := renderProfilerFtraceEventBody(event)
	if !ok {
		return name, body, ok, nil
	}
	switch event.Field {
	case 2002:
		cpuID, state, reason := protoScalarUint(event.Payload, 3)
		if state == protoScalarInvalid {
			degradations = append(degradations, "cpu_id_"+reason)
		} else if state == protoScalarPresent && cpuID > uint64(maxTraceDBCPUIndex) {
			degradations = append(degradations, "cpu_id_out_of_range")
		}
	case 2417:
		_, state, reason := protoScalarUint(event.Payload, 8)
		if state == protoScalarInvalid {
			degradations = append(degradations, "next_info_"+reason)
		}
	}
	return name, body, ok, degradations
}

func profilerFtraceCoreWireAudit(event profilerFtraceEventRecord) (bool, []string) {
	var stringFields []int
	var scalarFields []int
	switch event.Field {
	case 410, 2002:
		stringFields = []int{1}
		scalarFields = []int{2}
	case 2417:
		stringFields = []int{1, 5}
		scalarFields = []int{2, 3, 4, 6, 7}
	case 1000, 1001:
		scalarFields = []int{1, 2, 3, 4}
	default:
		return true, nil
	}
	var reasons []string
	for _, field := range stringFields {
		value, state, reason := protoScalarString(event.Payload, field)
		validValue := state == protoScalarPresent && traceDBSinglePhysicalLine(value, false)
		if validValue && (event.Field == 410 || event.Field == 2002) && field == 1 {
			validValue = traceDBSingleToken(value)
		}
		if state == protoScalarAbsent || (state == protoScalarPresent && !validValue) {
			reasons = append(reasons, fmt.Sprintf("core_field%d_missing_or_invalid", field))
			continue
		}
		if state == protoScalarInvalid {
			reasons = append(reasons, fmt.Sprintf("core_field%d_%s", field, reason))
		}
	}
	for _, field := range scalarFields {
		_, state, reason := protoScalarUint(event.Payload, field)
		// Proto3 scalar absence is the exact default zero under these pinned
		// generated producer profiles; only malformed/ambiguous wire is a
		// missing core authority.
		if state == protoScalarInvalid {
			reasons = append(reasons, fmt.Sprintf("core_field%d_%s", field, reason))
		}
	}
	if (event.Field == 1000 || event.Field == 1001) && len(reasons) == 0 {
		index, state, _ := protoScalarUint(event.Payload, 3)
		if state == protoScalarPresent && index > ^uint64(0)>>12 {
			reasons = append(reasons, "core_field3_out_of_range")
		}
		sDev, state, _ := protoScalarUint(event.Payload, 4)
		if state == protoScalarPresent && sDev > math.MaxUint32 {
			reasons = append(reasons, "core_field4_out_of_range")
		}
	}
	if event.Field == 2417 && len(reasons) == 0 {
		for _, field := range []int{2, 6} {
			value, state, _ := protoScalarUint(event.Payload, field)
			if state == protoScalarPresent && value > math.MaxInt32 {
				reasons = append(reasons, fmt.Sprintf("core_field%d_out_of_range", field))
			}
		}
		for _, field := range []int{3, 7} {
			value, state, _ := protoScalarUint(event.Payload, field)
			if state == protoScalarPresent {
				signed := int64(value)
				if signed < math.MinInt32 || signed > math.MaxInt32 {
					reasons = append(reasons, fmt.Sprintf("core_field%d_out_of_range", field))
				}
			}
		}
	}
	return len(reasons) == 0, reasons
}

type protoScalarState uint8

const (
	protoScalarAbsent protoScalarState = iota
	protoScalarPresent
	protoScalarInvalid
)

// protoScalarUint reads one singular proto scalar without collapsing three
// distinct states: proto3 default omission, a present value (including an
// explicitly encoded zero), and malformed/ambiguous input. Callers interpret
// the default only when their pinned producer profile proves it.
func protoScalarUint(data []byte, field int) (uint64, protoScalarState, string) {
	var value uint64
	count := 0
	wrongWire := false
	err := walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f != field {
			return nil
		}
		count++
		if wire != 0 {
			wrongWire = true
			return nil
		}
		value = v
		_ = raw
		return nil
	})
	if err != nil {
		return 0, protoScalarInvalid, "malformed_wire"
	}
	if wrongWire {
		return 0, protoScalarInvalid, "wrong_wire"
	}
	if count > 1 {
		return 0, protoScalarInvalid, "duplicate"
	}
	if count == 0 {
		return 0, protoScalarAbsent, ""
	}
	return value, protoScalarPresent, ""
}

func protoScalarString(data []byte, field int) (string, protoScalarState, string) {
	var value string
	count := 0
	wrongWire := false
	err := walkProtoFields(data, func(f int, wire int, raw []byte, v uint64) error {
		if f != field {
			return nil
		}
		count++
		if wire != 2 {
			wrongWire = true
			return nil
		}
		value = string(raw)
		_ = v
		return nil
	})
	if err != nil {
		return "", protoScalarInvalid, "malformed_wire"
	}
	if wrongWire {
		return "", protoScalarInvalid, "wrong_wire"
	}
	if count > 1 {
		return "", protoScalarInvalid, "duplicate"
	}
	if count == 0 {
		return "", protoScalarAbsent, ""
	}
	return value, protoScalarPresent, ""
}

func profilerFtraceEventRenderCoverage(coverageByField map[int]*TraceDBCoverage, field int) *TraceDBCoverage {
	if coverage, ok := coverageByField[field]; ok {
		return coverage
	}
	if field == 0 {
		coverage := TraceDBCoverage{
			Family:  "builtin_modern_ftrace:envelope",
			Table:   "__event_envelope__",
			Role:    "unsupported_input",
			Found:   true,
			Skipped: "malformed or ambiguous structured ftrace event envelope",
			FieldSources: map[string]string{
				"schema_profile": "TracePluginResult.ftrace_cpu_detail -> FtraceCpuDetailMsg -> exactly one FtraceEvent oneof payload",
			},
		}
		coverageByField[field] = &coverage
		return &coverage
	}
	if field == profilerFtraceCPUDetailEnvelopeField {
		coverage := TraceDBCoverage{
			Family:  "builtin_modern_ftrace:cpu_detail_envelope",
			Table:   "__cpu_detail_envelope__",
			Role:    "unsupported_input",
			Found:   true,
			Skipped: "malformed or ambiguous FtraceCpuDetailMsg envelope",
			FieldSources: map[string]string{
				"schema_profile": "FtraceCpuDetailMsg{cpu=1 singular,event=2 repeated,overwrite=3 singular}",
			},
		}
		coverageByField[field] = &coverage
		return &coverage
	}
	desc, ok := profilerFtraceEventDescriptors[field]
	coverage := TraceDBCoverage{Found: true, Role: "query_ready_export"}
	if ok {
		coverage.Family = "builtin_modern_ftrace:" + desc.Family
		coverage.Table = desc.Name
		switch field {
		case 202:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockBioCompleteFormat{dev=1,sector=2,nr_sector=3,error=4,rwbs=5}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; error never substitutes",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 204:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockBioQueueFormat{dev=1,sector=2,nr_sector=3,rwbs=4,comm=5}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; comm never substitutes",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 205:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockBioRemapFormat{dev=1,sector=2,nr_sector=3,old_dev=4,old_sector=5,rwbs=6}",
				"pairing_identity": "remap inventory only; no other field substitutes",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 209:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockRqCompleteFormat{dev=1,sector=2,nr_sector=3,error=4,rwbs=5,cmd=6}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; bytes/cmd/comm/error never substitute",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 210, 211:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockRqInsert/IssueFormat{dev=1,sector=2,nr_sector=3,bytes=4,rwbs=5,comm=6,cmd=7}",
				"pairing_identity": "dev+rwbs+sector+nr_sector; bytes/cmd/comm/error never substitute",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 212:
			coverage.FieldSources = map[string]string{
				"schema_profile":   "block.proto BlockRqRemapFormat{dev=1,sector=2,nr_sector=3,old_dev=4,old_sector=5,nr_bios=6,rwbs=7}",
				"pairing_identity": "remap inventory only; bytes/cmd/comm/error never substitute",
				"scalar_presence":  "pinned producer set_* profile: proto3 scalar absence is exact zero; malformed/wrong-wire/duplicate fail closed",
			}
		case 410:
			coverage.FieldSources = map[string]string{
				"schema_profile": "clk.proto ClkSetRateFormat{name=1,rate=2}",
				"cpu_id":         "not present in field-410 schema; omitted",
			}
		case 2002:
			coverage.FieldSources = map[string]string{
				"schema_profile": "power.proto ClockSetRateFormat{name=1,state=2,cpu_id=3}",
				"cpu_id":         "field3 uint64; proto3 wire absence is CPU0 under the pinned producer; valid range 0..4095",
			}
		case 2417:
			coverage.FieldSources = map[string]string{
				"schema_profile": "sched.proto SchedSwitchFormat{prev=1..4,next=5..7,next_info=8}",
				"next_info":      "field8 packed uint64; proto3 wire absence is exact zero; MaxUint64 is producer missing sentinel",
			}
		case 1000, 1001:
			coverage.FieldSources = map[string]string{
				"schema_profile": "filemap proto exposes pfn=1,i_ino=2,index=3,s_dev=4",
				"page_pointer":   "not present in profiler schema; page token omitted",
			}
		}
	} else {
		coverage.Family = "builtin_modern_ftrace:unknown"
		coverage.Table = fmt.Sprintf("event_field:%d", field)
		coverage.Role = "unsupported_input"
		coverage.Skipped = "unmapped structured ftrace event field"
	}
	coverageByField[field] = &coverage
	return &coverage
}

func profilerFtraceEventRenderCoverageList(coverageByField map[int]*TraceDBCoverage) []TraceDBCoverage {
	fields := make([]int, 0, len(coverageByField))
	for field := range coverageByField {
		fields = append(fields, field)
	}
	sort.Ints(fields)
	out := make([]TraceDBCoverage, 0, len(fields))
	for _, field := range fields {
		out = append(out, *coverageByField[field])
	}
	return out
}

func profilerFtraceCoverageHasSkipped(coverage []TraceDBCoverage) bool {
	for _, item := range coverage {
		if strings.TrimSpace(item.Skipped) != "" {
			return true
		}
	}
	return false
}
