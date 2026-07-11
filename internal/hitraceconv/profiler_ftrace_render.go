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
	CPU     int64
	TSNS    uint64
	TGID    int64
	PID     int64
	Comm    string
	Field   int
	Payload []byte
}

func renderProfilerFtraceStructuredRows(data []byte, seq *int, sink *traceDBRowSink) (int, []TraceDBCoverage, error) {
	events, err := decodeProfilerFtraceStructuredEvents(data)
	if err != nil {
		return 0, nil, err
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
		tid := event.PID
		if tid == 0 {
			tid = event.TGID
		}
		task := firstNonEmpty(event.Comm, "unknown")
		if event.TSNS > math.MaxInt64 {
			return rows, profilerFtraceEventRenderCoverageList(coverageByField),
				&traceDBOutputInvariantError{Reason: "invalid_timestamp"}
		}
		row, err := prepareTraceDBRenderedRow(int64(event.TSNS), *seq, task, tid,
			firstNonZero(event.TGID, tid), event.CPU, name+": "+body)
		if err != nil {
			return rows, profilerFtraceEventRenderCoverageList(coverageByField), err
		}
		if err := sink.add(row); err != nil {
			return rows, profilerFtraceEventRenderCoverageList(coverageByField), err
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
	return rows, profilerFtraceEventRenderCoverageList(coverageByField), nil
}

func decodeProfilerFtraceStructuredEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	var out []profilerFtraceEventRecord
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		if field != 2 || wire != 2 {
			return nil
		}
		events, err := decodeProfilerFtraceCPUDetailEvents(raw)
		if err != nil {
			return err
		}
		out = append(out, events...)
		_ = v
		return nil
	})
	return out, err
}

func decodeProfilerFtraceCPUDetailEvents(data []byte) ([]profilerFtraceEventRecord, error) {
	var cpu uint64
	var out []profilerFtraceEventRecord
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				cpu = v
			}
		case 2:
			if wire == 2 {
				events, err := decodeProfilerFtraceEventRecords(cpu, raw)
				if err != nil {
					return err
				}
				out = append(out, events...)
			}
		}
		return nil
	})
	return out, err
}

func decodeProfilerFtraceEventRecords(cpu uint64, data []byte) ([]profilerFtraceEventRecord, error) {
	base := profilerFtraceEventRecord{CPU: int64(cpu)}
	type payload struct {
		field int
		raw   []byte
	}
	var payloads []payload
	err := walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		switch field {
		case 1:
			if wire == 0 {
				base.TSNS = v
			}
		case 2:
			if wire == 0 {
				base.TGID = int64(v)
			}
		case 3:
			if wire == 2 {
				base.Comm = string(raw)
			}
		case 50:
			if wire == 2 {
				base.PID = decodeProfilerFtraceCommonPID(raw)
			}
		default:
			if field >= 100 && wire == 2 {
				payloads = append(payloads, payload{field: field, raw: raw})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]profilerFtraceEventRecord, 0, len(payloads))
	for _, item := range payloads {
		record := base
		record.Field = item.field
		record.Payload = item.raw
		out = append(out, record)
	}
	return out, nil
}

func decodeProfilerFtraceCommonPID(data []byte) int64 {
	var pid int64
	_ = walkProtoFields(data, func(field int, wire int, raw []byte, v uint64) error {
		if field == 4 && wire == 0 {
			pid = int64(v)
		}
		_ = raw
		return nil
	})
	return pid
}

func renderProfilerFtraceEventBody(event profilerFtraceEventRecord) (string, string, bool) {
	switch event.Field {
	case 113:
		return "binder_transaction", fmt.Sprintf("transaction=%d dest_node=%d dest_proc=%d dest_thread=%d reply=%d flags=0x%x code=0x%x",
			protoInt(event.Payload, 1), protoInt(event.Payload, 2), protoInt(event.Payload, 3), protoInt(event.Payload, 4),
			protoInt(event.Payload, 5), protoUint(event.Payload, 7), protoUint(event.Payload, 6)), true
	case 119:
		return "binder_transaction_received", fmt.Sprintf("transaction=%d", protoInt(event.Payload, 1)), true
	case 209:
		return "block_rq_complete", fmt.Sprintf("%s %s (%s) %d + %d [%d]",
			devMajorMinor(int64(protoUint(event.Payload, 1)), ":"), firstNonEmpty(protoString(event.Payload, 5), "RW"),
			protoString(event.Payload, 6), protoUint(event.Payload, 2), protoUint(event.Payload, 3), protoInt(event.Payload, 4)), true
	case 210, 211:
		name := "block_rq_insert"
		if event.Field == 211 {
			name = "block_rq_issue"
		}
		return name, fmt.Sprintf("%s %s %d (%s) %d + %d [%s]",
			devMajorMinor(int64(protoUint(event.Payload, 1)), ":"), firstNonEmpty(protoString(event.Payload, 5), "RW"),
			protoUint(event.Payload, 4), protoString(event.Payload, 7), protoUint(event.Payload, 2), protoUint(event.Payload, 3),
			protoString(event.Payload, 6)), true
	case 212:
		return "block_rq_remap", fmt.Sprintf("%s %s %d + %d <- (%s) %d",
			devMajorMinor(int64(protoUint(event.Payload, 1)), ":"), firstNonEmpty(protoString(event.Payload, 7), "RW"),
			protoUint(event.Payload, 2), protoUint(event.Payload, 3), devMajorMinor(int64(protoUint(event.Payload, 4)), ":"),
			protoUint(event.Payload, 5)), true
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
	case 1500:
		return "irq_handler_entry", fmt.Sprintf("irq=%d name=%s", protoInt(event.Payload, 1), protoString(event.Payload, 2)), true
	case 1501:
		ret := "unhandled"
		if protoInt(event.Payload, 2) != 0 {
			ret = "handled"
		}
		return "irq_handler_exit", fmt.Sprintf("irq=%d ret=%s", protoInt(event.Payload, 1), ret), true
	case 1502:
		return "softirq_entry", fmt.Sprintf("vec=%d", protoUint(event.Payload, 1)), true
	case 1503:
		return "softirq_exit", fmt.Sprintf("vec=%d", protoUint(event.Payload, 1)), true
	case 1504:
		return "softirq_raise", fmt.Sprintf("vec=%d", protoUint(event.Payload, 1)), true
	case 2003:
		return "cpu_frequency", fmt.Sprintf("state=%d cpu_id=%d", protoUint(event.Payload, 1), protoUint(event.Payload, 2)), true
	case 2004:
		return "cpu_frequency_limits", fmt.Sprintf("min=%d max=%d cpu_id=%d", protoUint(event.Payload, 1), protoUint(event.Payload, 2), protoUint(event.Payload, 3)), true
	case 2005:
		return "cpu_idle", fmt.Sprintf("state=%d cpu_id=%d", protoUint(event.Payload, 1), protoUint(event.Payload, 2)), true
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
	case 2420, 2421, 2422:
		name := "sched_wakeup"
		if event.Field == 2421 {
			name = "sched_wakeup_new"
		} else if event.Field == 2422 {
			name = "sched_waking"
		}
		return name, fmt.Sprintf("comm=%s pid=%d prio=%d target_cpu=%03d",
			protoString(event.Payload, 1), protoInt(event.Payload, 2), protoInt(event.Payload, 3), protoInt(event.Payload, 5)), true
	case 4002:
		return "sched_blocked_reason", renderProfilerFtraceSchedBlockedReason(event.Payload), true
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

// renderProfilerFtraceSchedBlockedReason preserves two separate authorities:
// caller_str is the ftrace plugin's symbolized caller and may feed the
// semantic `caller=` token only when it is a bounded single token; the raw
// uint64 address is provenance only. When symbolization is absent or unsafe,
// publishing caller=unknown keeps tracequery's blocked-reason aggregation from
// fragmenting into one pseudo-reason per address while pid/iowait remain usable.
func renderProfilerFtraceSchedBlockedReason(data []byte) string {
	pid := protoInt(data, 1)
	rawCaller := protoUint(data, 2)
	ioWait := protoUint(data, 3)
	caller, symbolized := safeProfilerBlockedCaller(protoString(data, 4))
	if symbolized {
		return fmt.Sprintf("pid=%d iowait=%d caller=%s caller_raw=0x%x caller_quality=symbolized",
			pid, ioWait, caller, rawCaller)
	}
	return fmt.Sprintf("pid=%d iowait=%d caller=unknown caller_raw=0x%x caller_quality=opaque",
		pid, ioWait, rawCaller)
}

func safeProfilerBlockedCaller(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" || value != raw || len(value) > 512 {
		return "", false
	}
	for _, r := range value {
		// caller= is one systrace key/value token. Whitespace, pipes and
		// controls would either truncate the parser-visible reason or inject a
		// second trace field/line, so such payloads fail closed to opaque.
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '|' {
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
	desc, ok := profilerFtraceEventDescriptors[field]
	coverage := TraceDBCoverage{Found: true, Role: "query_ready_export"}
	if ok {
		coverage.Family = "builtin_modern_ftrace:" + desc.Family
		coverage.Table = desc.Name
		switch field {
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
