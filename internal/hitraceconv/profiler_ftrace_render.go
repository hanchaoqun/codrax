package hitraceconv

import (
	"fmt"
	"sort"
	"strings"
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
	rows := 0
	for _, event := range events {
		coverage := profilerFtraceEventRenderCoverage(coverageByField, event.Field)
		coverage.RowsRead++
		name, body, ok := renderProfilerFtraceEventBody(event)
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
		line := traceDBFormatLine(task, tid, firstNonZero(event.TGID, tid), event.CPU, int64(event.TSNS), name+": "+body)
		if err := sink.add(renderedRow{tsNS: event.TSNS, seq: *seq, line: line}); err != nil {
			return rows, profilerFtraceEventRenderCoverageList(coverageByField), err
		}
		(*seq)++
		rows++
		coverage.RowsEmitted++
		coverage.Skipped = ""
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
		return "clock_set_rate", fmt.Sprintf("%s state=%d cpu_id=0", protoString(event.Payload, 1), protoUint(event.Payload, 2)), true
	case 1000:
		return "mm_filemap_add_to_page_cache", renderProfilerFtraceFilemapPageCache(event.Payload), true
	case 1001:
		return "mm_filemap_delete_from_page_cache", renderProfilerFtraceFilemapPageCache(event.Payload), true
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
		return "sched_switch", fmt.Sprintf("prev_comm=%s prev_pid=%d prev_prio=%d prev_state=%s ==> next_comm=%s next_pid=%d next_prio=%d",
			protoString(event.Payload, 1), protoInt(event.Payload, 2), protoInt(event.Payload, 3),
			linuxPrevState(protoUint(event.Payload, 4)), protoString(event.Payload, 5), protoInt(event.Payload, 6), protoInt(event.Payload, 7)), true
	case 2420, 2421, 2422:
		name := "sched_wakeup"
		if event.Field == 2421 {
			name = "sched_wakeup_new"
		} else if event.Field == 2422 {
			name = "sched_waking"
		}
		return name, fmt.Sprintf("comm=%s pid=%d prio=%d target_cpu=%03d",
			protoString(event.Payload, 1), protoInt(event.Payload, 2), protoInt(event.Payload, 3), protoInt(event.Payload, 5)), true
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

func renderProfilerFtraceFilemapPageCache(data []byte) string {
	index := protoUint(data, 3)
	return fmt.Sprintf("dev %s ino 0x%x page=0x0 pfn=%d ofs=%d",
		devMajorMinor(int64(protoUint(data, 4)), ":"), protoUint(data, 2), protoUint(data, 1), index<<12)
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

func profilerFtraceEventRenderCoverage(coverageByField map[int]*TraceDBCoverage, field int) *TraceDBCoverage {
	if coverage, ok := coverageByField[field]; ok {
		return coverage
	}
	desc, ok := profilerFtraceEventDescriptors[field]
	coverage := TraceDBCoverage{Found: true}
	if ok {
		coverage.Family = "builtin_modern_ftrace:" + desc.Family
		coverage.Table = desc.Name
	} else {
		coverage.Family = "builtin_modern_ftrace:unknown"
		coverage.Table = fmt.Sprintf("event_field:%d", field)
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
