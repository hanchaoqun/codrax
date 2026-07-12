package hitraceconv

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type profilerCoreProtoField struct {
	count      int
	wrongWire  bool
	uintValue  uint64
	bytesValue []byte
}

// profilerStructuredCoreSchemas is the closed OpenHarmony structured-core
// matrix pinned to developtools_profiler 5bc8ef5 (default and 6.6.30). The
// inner map is payload field -> protobuf wire. Unknown future fields do not
// mint facts; every known field is still audited for wire and uniqueness.
var profilerStructuredCoreSchemas = map[int]map[int]int{
	113:  {1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0},
	119:  {1: 0},
	1400: {1: 2},
	1401: {1: 2},
	1402: {1: 0, 2: 2},
	1500: {1: 0, 2: 2},
	1501: {1: 0, 2: 0},
	1502: {1: 0},
	1503: {1: 0},
	1504: {1: 0},
	2003: {1: 0, 2: 0},
	2004: {1: 0, 2: 0, 3: 0},
	2005: {1: 0, 2: 0},
	2420: {1: 2, 2: 0, 3: 0, 4: 0, 5: 0},
	2421: {1: 2, 2: 0, 3: 0, 4: 0, 5: 0},
	2422: {1: 2, 2: 0, 3: 0, 4: 0, 5: 0},
	4002: {1: 0, 2: 0, 3: 0, 4: 2},
}

const maxProfilerWakeCommBytes = 15 // TASK_COMM_LEN (16) minus terminating NUL.

func decodeProfilerCorePayload(event profilerFtraceEventRecord) (coreRenderPayload, bodyAdmission, string, []string) {
	schema, governed := profilerStructuredCoreSchemas[event.Field]
	if !governed {
		return coreRenderPayload{}, bodyUnsupported, "", nil
	}
	descriptor, ok := profilerFtraceEventDescriptors[event.Field]
	if !ok {
		return coreRenderPayload{}, bodyRejected, "missing_core_descriptor", nil
	}
	if descriptor.Field != event.Field {
		return coreRenderPayload{}, bodyRejected, "mismatched_core_descriptor_field", nil
	}
	kind, ok := coreRenderKindForName(descriptor.Name)
	if !ok {
		return coreRenderPayload{}, bodyRejected, "invalid_core_descriptor_name", nil
	}
	fields, reason := decodeProfilerCoreProtoFields(event.Payload, schema)
	if reason != "" {
		return coreRenderPayload{}, bodyRejected, reason, nil
	}
	for field := 1; field <= 7; field++ {
		if _, expected := schema[field]; !expected || profilerCoreDisplayField(event.Field, field) {
			continue
		}
		if reason := profilerCoreFieldWireReason(fields[field], field); reason != "" {
			return coreRenderPayload{}, bodyRejected, reason, nil
		}
	}

	payload := coreRenderPayload{Kind: kind, Name: descriptor.Name}
	var degradations []string
	switch event.Field {
	case 113:
		values := [5]int64{}
		for index := range values {
			value, valid := profilerCoreInt32(fields[index+1])
			if !valid {
				return coreRenderPayload{}, bodyRejected, fmt.Sprintf("core_field%d_out_of_range", index+1), nil
			}
			values[index] = value
		}
		if values[0] <= 0 {
			return coreRenderPayload{}, bodyRejected, "invalid_transaction_id", nil
		}
		if values[1] < 0 || values[2] < 0 || values[3] < 0 {
			return coreRenderPayload{}, bodyRejected, "invalid_transaction_endpoint", nil
		}
		if values[4] != 0 && values[4] != 1 {
			return coreRenderPayload{}, bodyRejected, "invalid_reply", nil
		}
		code, valid := profilerCoreUint32(fields[6])
		if !valid {
			return coreRenderPayload{}, bodyRejected, "core_field6_out_of_range", nil
		}
		flags, valid := profilerCoreUint32(fields[7])
		if !valid {
			return coreRenderPayload{}, bodyRejected, "core_field7_out_of_range", nil
		}
		payload.Binder = &coreBinderPayload{
			Transaction: values[0], DestNode: values[1], DestProc: values[2], DestThread: values[3],
			Reply: values[4], Code: code, Flags: flags,
		}
	case 119:
		transaction, valid := profilerCoreInt32(fields[1])
		if !valid || transaction <= 0 {
			return coreRenderPayload{}, bodyRejected, "invalid_transaction_id", nil
		}
		payload.Binder = &coreBinderPayload{Transaction: transaction, Received: true}
	case 1400, 1401:
		reason, valid := profilerCoreString(fields[1])
		if !valid || !validCoreIPIReason(reason) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_reason", nil
		}
		payload.Interrupt = &coreInterruptPayload{Reason: reason}
	case 1402:
		reason, valid := profilerCoreString(fields[2])
		if !valid || !validCoreIPIReason(reason) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_reason", nil
		}
		payload.Interrupt = &coreInterruptPayload{TargetMask: profilerCoreUint64(fields[1]), Reason: reason}
	case 1500:
		irq, valid := profilerCoreInt32(fields[1])
		if !valid || irq < 0 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_irq", nil
		}
		name, valid := profilerCoreString(fields[2])
		if !valid || !traceDBSingleToken(name) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_irq_name", nil
		}
		payload.Interrupt = &coreInterruptPayload{IRQ: irq, IRQName: name}
	case 1501:
		irq, valid := profilerCoreInt32(fields[1])
		if !valid || irq < 0 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_irq", nil
		}
		ret, valid := profilerCoreInt32(fields[2])
		if !valid {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_ret", nil
		}
		payload.Interrupt = &coreInterruptPayload{IRQ: irq, Ret: ret}
	case 1502, 1503, 1504:
		vec, valid := profilerCoreUint32(fields[1])
		if !valid || vec > 9 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_vec", nil
		}
		payload.Interrupt = &coreInterruptPayload{Vec: vec}
	case 2003, 2005:
		state, valid := profilerCoreUint32(fields[1])
		if !valid {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_state", nil
		}
		cpuID, valid := profilerCoreUint32(fields[2])
		if !valid || cpuID > uint64(maxTraceDBCPUIndex) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_cpu_id", nil
		}
		payload.CPU = &coreCPUPayload{State: state, CPUID: cpuID}
	case 2004:
		minFreq, minValid := profilerCoreUint32(fields[1])
		maxFreq, maxValid := profilerCoreUint32(fields[2])
		cpuID, cpuValid := profilerCoreUint32(fields[3])
		if !minValid || !maxValid {
			return coreRenderPayload{}, bodyRejected, "invalid_limits_profile", nil
		}
		if minFreq > maxFreq {
			return coreRenderPayload{}, bodyRejected, "invalid_limits_order", nil
		}
		if !cpuValid || cpuID > uint64(maxTraceDBCPUIndex) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_cpu_id", nil
		}
		payload.CPU = &coreCPUPayload{Min: minFreq, Max: maxFreq, CPUID: cpuID, IsLimits: true}
	case 2420, 2421, 2422:
		comm, displayReason := profilerCoreDisplayString(fields[1], "comm")
		switch {
		case displayReason != "":
		case comm == "" || comm != strings.TrimSpace(comm) ||
			!traceDBSinglePhysicalLine(comm, false) || strings.ContainsAny(comm, "=|"):
			displayReason = "display_comm_unavailable"
		case len(comm) > maxProfilerWakeCommBytes:
			displayReason = "display_comm_out_of_profile"
		}
		if displayReason != "" {
			comm = "<...>"
			degradations = append(degradations, displayReason)
		}
		pid, valid := profilerCoreInt32(fields[2])
		if !valid || pid < 0 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_pid", nil
		}
		priority, valid := profilerCoreInt32(fields[3])
		if !valid {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_priority", nil
		}
		// success=4 exists in the proto schema but neither pinned producer lane
		// reads or sets it. It is audited above when present, never required or
		// used as a wakeup identity/value.
		if _, valid := profilerCoreInt32(fields[4]); !valid {
			return coreRenderPayload{}, bodyRejected, "core_field4_out_of_range", nil
		}
		targetCPU, valid := profilerCoreInt32(fields[5])
		if !valid || !validTraceDBCPUIndex(targetCPU) {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_target_cpu", nil
		}
		payload.Wakeup = &coreWakeupPayload{Comm: comm, PID: pid, Priority: priority, TargetCPU: targetCPU}
	case 4002:
		pid, valid := profilerCoreInt32(fields[1])
		if !valid || pid < 0 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_pid", nil
		}
		ioWait, valid := profilerCoreUint32(fields[3])
		if !valid || ioWait > 1 {
			return coreRenderPayload{}, bodyRejected, "missing_or_invalid_iowait", nil
		}
		blocked := coreBlockedPayload{PID: pid, CallerRaw: profilerCoreUint64(fields[2]), IOWait: ioWait}
		caller, displayReason := profilerCoreDisplayString(fields[4], "caller_str")
		if caller != "" {
			if safe, symbolized := safeProfilerStructuredBlockedCaller(caller); symbolized {
				blocked.Caller, blocked.CallerSymbolized = safe, true
			} else if displayReason == "" {
				displayReason = "display_caller_str_invalid"
			}
		}
		if displayReason != "" {
			degradations = append(degradations, displayReason)
		}
		payload.Blocked = &blocked
	default:
		return coreRenderPayload{}, bodyRejected, "unhandled_core_descriptor", nil
	}
	return payload, bodyAdmitted, "", degradations
}

func decodeProfilerCoreProtoFields(data []byte, schema map[int]int) (map[int]profilerCoreProtoField, string) {
	fields := make(map[int]profilerCoreProtoField, len(schema))
	err := walkProtoFields(data, func(field int, wire int, raw []byte, value uint64) error {
		expectedWire, known := schema[field]
		if !known {
			return nil
		}
		item := fields[field]
		item.count++
		if wire != expectedWire {
			item.wrongWire = true
		} else if wire == 2 {
			item.bytesValue = raw
		} else {
			item.uintValue = value
		}
		fields[field] = item
		return nil
	})
	if err != nil {
		return nil, "core_payload_malformed_wire"
	}
	return fields, ""
}

func profilerCoreDisplayField(eventField, payloadField int) bool {
	return (eventField == 2420 || eventField == 2421 || eventField == 2422) && payloadField == 1 ||
		eventField == 4002 && payloadField == 4
}

func profilerCoreFieldWireReason(field profilerCoreProtoField, number int) string {
	if field.wrongWire {
		return fmt.Sprintf("core_field%d_wrong_wire", number)
	}
	if field.count > 1 {
		return fmt.Sprintf("core_field%d_duplicate", number)
	}
	return ""
}

func profilerCoreInt32(field profilerCoreProtoField) (int64, bool) {
	value := field.uintValue
	low := uint32(value)
	signed := int64(int32(low))
	return signed, field.count == 0 || value == uint64(low) || value == uint64(signed)
}

func profilerCoreUint32(field profilerCoreProtoField) (uint64, bool) {
	return field.uintValue, field.count == 0 || field.uintValue <= math.MaxUint32
}

func profilerCoreUint64(field profilerCoreProtoField) uint64 {
	return field.uintValue
}

func profilerCoreString(field profilerCoreProtoField) (string, bool) {
	if field.count == 0 {
		return "", true
	}
	if field.wrongWire || field.count > 1 {
		return "", false
	}
	value := string(field.bytesValue)
	return value, traceDBSinglePhysicalLine(value, true)
}

func profilerCoreDisplayString(field profilerCoreProtoField, label string) (string, string) {
	if field.count == 0 {
		return "", ""
	}
	if field.wrongWire {
		return "", "display_" + label + "_wrong_wire"
	}
	if field.count > 1 {
		return "", "display_" + label + "_duplicate"
	}
	value := string(field.bytesValue)
	if !traceDBSinglePhysicalLine(value, true) {
		return "", "display_" + label + "_invalid"
	}
	return value, ""
}

// safeProfilerStructuredBlockedCaller admits only the exact caller_str shape
// constructed by the pinned OpenHarmony ftrace producer:
//
//	funcName+0x<lower-hex-offset>/0x<lower-hex-size>[module]
//
// The protobuf string is optional display metadata, not an independent symbol
// authority. A merely token-safe arbitrary string therefore degrades to the
// raw caller address instead of minting a semantic blocked-reason label.
func safeProfilerStructuredBlockedCaller(raw string) (string, bool) {
	value, safe := safeProfilerBlockedCaller(raw)
	if !safe || !strings.HasSuffix(value, "]") {
		return "", false
	}
	plus := strings.Index(value, "+0x")
	if plus <= 0 {
		return "", false
	}
	rest := value[plus+3:]
	slash := strings.Index(rest, "/0x")
	if slash <= 0 {
		return "", false
	}
	rest = rest[slash+3:]
	open := strings.IndexByte(rest, '[')
	if open <= 0 || strings.IndexByte(rest[open+1:], '[') >= 0 {
		return "", false
	}
	function := value[:plus]
	offset := value[plus+3 : plus+3+slash]
	size := rest[:open]
	module := rest[open+1 : len(rest)-1]
	if strings.ContainsAny(function, "+/[]") || strings.ContainsAny(module, "+/[]") ||
		!profilerCoreCanonicalLowerHex(offset) || !profilerCoreCanonicalLowerHex(size) {
		return "", false
	}
	return value, true
}

func profilerCoreCanonicalLowerHex(value string) bool {
	parsed, err := strconv.ParseUint(value, 16, 64)
	return err == nil && strconv.FormatUint(parsed, 16) == value
}

// profilerCanonicalLineValid keeps a governed malformed event local to
// its coverage row. The structured row loop uses the same endpoint primitive
// again with the real sequence number; validating here prevents an unsafe
// display string or oversized canonical body from turning one source event
// into a conversion-wide invariant error.
func profilerCanonicalLineValid(event profilerFtraceEventRecord, name, body string) bool {
	if event.TSNS > math.MaxInt64 {
		return false
	}
	task := firstNonEmpty(event.Comm, "unknown")
	_, err := prepareTraceDBRenderedRowWithTraceFlags(
		int64(event.TSNS), 0, task, event.PID, event.TGID, event.CPU,
		event.CommonFlags, event.CommonPreemptCount, name+": "+body,
	)
	return err == nil
}
