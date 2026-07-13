package hitraceconv

import (
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

const profilerFtraceCoreIssuesPerEvent = 1

// profilerFtraceCoreIssueSet preserves the established core contract: one
// event publishes at most one dominant diagnostic. This differs deliberately
// from generic wire events, whose pre-migration contract accumulated multiple
// independent scalar failures after a completed scan.
type profilerFtraceCoreIssueSet struct {
	Count  uint8
	Issues [profilerFtraceCoreIssuesPerEvent]profilerFtraceEventIssue
}

func (set *profilerFtraceCoreIssueSet) validate(eventField int) error {
	if set == nil || int(set.Count) > len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_core_issue_count_invalid"}
	}
	for index, issue := range set.Issues {
		if index >= int(set.Count) {
			if issue != (profilerFtraceEventIssue{}) {
				return &traceDBOutputInvariantError{Reason: "profiler_core_issue_count_invalid"}
			}
			continue
		}
		if issue.Kind < profilerFtraceEventIssueCorePayloadMalformedWire ||
			issue.Kind > profilerFtraceEventIssueCoreInvalidCanonicalLine ||
			!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
			return &traceDBOutputInvariantError{Reason: "profiler_core_issue_schema_invalid"}
		}
	}
	return nil
}

func (set *profilerFtraceCoreIssueSet) add(eventField int, issue profilerFtraceEventIssue) error {
	if err := set.validate(eventField); err != nil {
		return err
	}
	if issue.Kind < profilerFtraceEventIssueCorePayloadMalformedWire ||
		issue.Kind > profilerFtraceEventIssueCoreInvalidCanonicalLine ||
		!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
		return &traceDBOutputInvariantError{Reason: "profiler_core_issue_schema_invalid"}
	}
	for index := 0; index < int(set.Count); index++ {
		if set.Issues[index] == issue {
			return &traceDBOutputInvariantError{Reason: "profiler_core_issue_duplicate"}
		}
	}
	if int(set.Count) == len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_core_issue_overflow"}
	}
	candidate := *set
	candidate.Issues[int(candidate.Count)] = issue
	candidate.Count++
	if err := candidate.validate(eventField); err != nil {
		return err
	}
	*set = candidate
	return nil
}

func (set *profilerFtraceCoreIssueSet) addFixed(eventField int, kind profilerFtraceEventIssueKind) error {
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_core_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceCoreIssueSet) addPayload(eventField int, kind profilerFtraceEventIssueKind, payloadField int) error {
	issue, ok := profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_core_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceCoreIssueSet) checked(eventField int) ([]profilerFtraceEventIssue, error) {
	if err := set.validate(eventField); err != nil {
		return nil, err
	}
	return append([]profilerFtraceEventIssue(nil), set.Issues[:int(set.Count)]...), nil
}

func decodeProfilerCorePayloadWithTypedAudit(event profilerFtraceEventRecord) (
	coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
) {
	schema, governed := profilerStructuredCoreSchemas[event.Field]
	if !governed {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, false, nil
	}
	rejectFixed := func(kind profilerFtraceEventIssueKind) (
		coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
	) {
		var set profilerFtraceCoreIssueSet
		err := set.addFixed(event.Field, kind)
		return coreRenderPayload{}, bodyRejected, set, true, err
	}
	rejectPayload := func(kind profilerFtraceEventIssueKind, payloadField int) (
		coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
	) {
		var set profilerFtraceCoreIssueSet
		err := set.addPayload(event.Field, kind, payloadField)
		return coreRenderPayload{}, bodyRejected, set, true, err
	}

	descriptor, ok := profilerFtraceEventDescriptors[event.Field]
	if !ok {
		return coreRenderPayload{}, bodyRejected, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "missing_core_descriptor"}
	}
	if descriptor.Field != event.Field {
		return coreRenderPayload{}, bodyRejected, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "mismatched_core_descriptor_field"}
	}
	kind, ok := coreRenderKindForName(descriptor.Name)
	if !ok {
		return coreRenderPayload{}, bodyRejected, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "invalid_core_descriptor_name"}
	}
	for payloadField, expectedWire := range schema {
		if payloadField < 1 || payloadField > 7 || (expectedWire != 0 && expectedWire != 2) {
			return coreRenderPayload{}, bodyRejected, profilerFtraceCoreIssueSet{}, true,
				&traceDBOutputInvariantError{Reason: "profiler_core_schema_invalid"}
		}
	}

	var fields [8]profilerCoreProtoField
	if err := walkProtoFields(event.Payload, func(payloadField int, wire int, raw []byte, value uint64) error {
		expectedWire, known := schema[payloadField]
		if !known {
			return nil
		}
		field := &fields[payloadField]
		if field.count < 2 {
			field.count++
		}
		if wire != expectedWire {
			field.wrongWire = true
			return nil
		}
		if wire == 2 {
			field.bytesValue = raw
		} else {
			field.uintValue = value
		}
		return nil
	}); err != nil {
		return rejectFixed(profilerFtraceEventIssueCorePayloadMalformedWire)
	}
	for field := 1; field <= 7; field++ {
		if _, expected := schema[field]; !expected || profilerCoreDisplayField(event.Field, field) {
			continue
		}
		switch {
		case fields[field].wrongWire:
			return rejectPayload(profilerFtraceEventIssueCoreFieldWrongWire, field)
		case fields[field].count > 1:
			return rejectPayload(profilerFtraceEventIssueCoreFieldDuplicate, field)
		}
	}

	payload := coreRenderPayload{Kind: kind, Name: descriptor.Name}
	displayKind := profilerFtraceEventIssueKindCount
	displayPresent := false
	switch event.Field {
	case 113:
		values := [5]int64{}
		for index := range values {
			value, valid := profilerCoreInt32(fields[index+1])
			if !valid {
				return rejectPayload(profilerFtraceEventIssueCoreFieldOutOfRange, index+1)
			}
			values[index] = value
		}
		if values[0] <= 0 {
			return rejectFixed(profilerFtraceEventIssueCoreInvalidTransactionID)
		}
		if values[1] < 0 || values[2] < 0 || values[3] < 0 {
			return rejectFixed(profilerFtraceEventIssueCoreInvalidTransactionEndpoint)
		}
		if values[4] != 0 && values[4] != 1 {
			return rejectFixed(profilerFtraceEventIssueCoreInvalidReply)
		}
		code, valid := profilerCoreUint32(fields[6])
		if !valid {
			return rejectPayload(profilerFtraceEventIssueCoreFieldOutOfRange, 6)
		}
		flags, valid := profilerCoreUint32(fields[7])
		if !valid {
			return rejectPayload(profilerFtraceEventIssueCoreFieldOutOfRange, 7)
		}
		payload.Binder = &coreBinderPayload{
			Transaction: values[0], DestNode: values[1], DestProc: values[2], DestThread: values[3],
			Reply: values[4], Code: code, Flags: flags,
		}
	case 119:
		transaction, valid := profilerCoreInt32(fields[1])
		if !valid || transaction <= 0 {
			return rejectFixed(profilerFtraceEventIssueCoreInvalidTransactionID)
		}
		payload.Binder = &coreBinderPayload{Transaction: transaction, Received: true}
	case 1400, 1401:
		reason, valid := profilerCoreString(fields[1])
		if !valid || !validCoreIPIReason(reason) {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidReason)
		}
		payload.Interrupt = &coreInterruptPayload{Reason: reason}
	case 1402:
		reason, valid := profilerCoreString(fields[2])
		if !valid || !validCoreIPIReason(reason) {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidReason)
		}
		payload.Interrupt = &coreInterruptPayload{TargetMask: profilerCoreUint64(fields[1]), Reason: reason}
	case 1500:
		irq, valid := profilerCoreInt32(fields[1])
		if !valid || irq < 0 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidIRQ)
		}
		name, valid := profilerCoreString(fields[2])
		if !valid || !traceDBSingleToken(name) {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidIRQName)
		}
		payload.Interrupt = &coreInterruptPayload{IRQ: irq, IRQName: name}
	case 1501:
		irq, valid := profilerCoreInt32(fields[1])
		if !valid || irq < 0 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidIRQ)
		}
		ret, valid := profilerCoreInt32(fields[2])
		if !valid {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidRet)
		}
		payload.Interrupt = &coreInterruptPayload{IRQ: irq, Ret: ret}
	case 1502, 1503, 1504:
		vec, valid := profilerCoreUint32(fields[1])
		if !valid || vec > 9 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidVec)
		}
		payload.Interrupt = &coreInterruptPayload{Vec: vec}
	case 2003, 2005:
		state, valid := profilerCoreUint32(fields[1])
		if !valid {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidState)
		}
		cpuID, valid := profilerCoreUint32(fields[2])
		if !valid || cpuID > uint64(maxTraceDBCPUIndex) {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidCPUID)
		}
		payload.CPU = &coreCPUPayload{State: state, CPUID: cpuID}
	case 2004:
		minFreq, minValid := profilerCoreUint32(fields[1])
		maxFreq, maxValid := profilerCoreUint32(fields[2])
		cpuID, cpuValid := profilerCoreUint32(fields[3])
		if !minValid || !maxValid {
			return rejectFixed(profilerFtraceEventIssueCoreInvalidLimitsProfile)
		}
		if minFreq > maxFreq {
			return rejectFixed(profilerFtraceEventIssueCoreInvalidLimitsOrder)
		}
		if !cpuValid || cpuID > uint64(maxTraceDBCPUIndex) {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidCPUID)
		}
		payload.CPU = &coreCPUPayload{Min: minFreq, Max: maxFreq, CPUID: cpuID, IsLimits: true}
	case 2420, 2421, 2422:
		comm, issueKind, issuePresent := profilerCoreDisplayComm(fields[1])
		switch {
		case issuePresent:
		case comm == "" || comm != strings.TrimSpace(comm) ||
			!traceDBSinglePhysicalLine(comm, false) || strings.ContainsAny(comm, "=|"):
			issueKind, issuePresent = profilerFtraceEventIssueCoreDisplayCommUnavailable, true
		case len(comm) > maxProfilerWakeCommBytes:
			issueKind, issuePresent = profilerFtraceEventIssueCoreDisplayCommOutOfProfile, true
		}
		if issuePresent {
			comm = "<...>"
			displayKind, displayPresent = issueKind, true
		}
		pid, valid := profilerCoreInt32(fields[2])
		if !valid || pid < 0 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidPID)
		}
		priority, valid := profilerCoreInt32(fields[3])
		if !valid {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidPriority)
		}
		// success=4 exists in the proto schema but neither pinned producer lane
		// reads or sets it. It is audited above when present, never required or
		// used as a wakeup identity/value.
		if _, valid := profilerCoreInt32(fields[4]); !valid {
			return rejectPayload(profilerFtraceEventIssueCoreFieldOutOfRange, 4)
		}
		targetCPU, valid := profilerCoreInt32(fields[5])
		if !valid || !validTraceDBCPUIndex(targetCPU) {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU)
		}
		payload.Wakeup = &coreWakeupPayload{Comm: comm, PID: pid, Priority: priority, TargetCPU: targetCPU}
	case 4002:
		pid, valid := profilerCoreInt32(fields[1])
		if !valid || pid < 0 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidPID)
		}
		ioWait, valid := profilerCoreUint32(fields[3])
		if !valid || ioWait > 1 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidIOWait)
		}
		blocked := coreBlockedPayload{PID: pid, CallerRaw: profilerCoreUint64(fields[2]), IOWait: ioWait}
		caller, issueKind, issuePresent := profilerCoreDisplayCaller(fields[4])
		if caller != "" {
			if safe, symbolized := safeProfilerStructuredBlockedCaller(caller); symbolized {
				blocked.Caller, blocked.CallerSymbolized = safe, true
			} else if !issuePresent {
				issueKind, issuePresent = profilerFtraceEventIssueCoreDisplayCallerStrInvalid, true
			}
		}
		if issuePresent {
			displayKind, displayPresent = issueKind, true
		}
		payload.Blocked = &blocked
	default:
		return coreRenderPayload{}, bodyRejected, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "unhandled_core_descriptor"}
	}
	var set profilerFtraceCoreIssueSet
	if displayPresent {
		if err := set.addFixed(event.Field, displayKind); err != nil {
			return coreRenderPayload{}, bodyRejected, profilerFtraceCoreIssueSet{}, true, err
		}
	}
	return payload, bodyAdmitted, set, true, nil
}

// decodeProfilerCorePayload is a test/compatibility adapter over the typed
// decoder. Production structured-core verdicts consume
// decodeProfilerCorePayloadWithTypedAudit directly.
func decodeProfilerCorePayload(event profilerFtraceEventRecord) (coreRenderPayload, bodyAdmission, string, []string) {
	payload, admission, set, handled, err := decodeProfilerCorePayloadWithTypedAudit(event)
	if !handled {
		return coreRenderPayload{}, bodyUnsupported, "", nil
	}
	if err != nil {
		if reason, ok := traceDBOutputInvariantReason(err); ok {
			return coreRenderPayload{}, bodyRejected, reason, nil
		}
		return coreRenderPayload{}, bodyRejected, "profiler_core_typed_audit_failed", nil
	}
	issues, err := set.checked(event.Field)
	if err != nil {
		return coreRenderPayload{}, bodyRejected, "profiler_core_typed_issue_invalid", nil
	}
	labels, ok := profilerFtraceEventIssueLabels(event.Field, issues)
	if !ok {
		return coreRenderPayload{}, bodyRejected, "profiler_core_typed_issue_invalid", nil
	}
	if admission == bodyRejected {
		if len(labels) != 1 {
			return coreRenderPayload{}, bodyRejected, "profiler_core_typed_issue_invalid", nil
		}
		return coreRenderPayload{}, bodyRejected, labels[0], nil
	}
	return payload, admission, "", labels
}

func profilerCoreDisplayComm(field profilerCoreProtoField) (string, profilerFtraceEventIssueKind, bool) {
	switch {
	case field.wrongWire:
		return "", profilerFtraceEventIssueCoreDisplayCommWrongWire, true
	case field.count > 1:
		return "", profilerFtraceEventIssueCoreDisplayCommDuplicate, true
	case field.count == 0:
		return "", profilerFtraceEventIssueKindCount, false
	}
	value := string(field.bytesValue)
	if !traceDBSinglePhysicalLine(value, true) {
		return "", profilerFtraceEventIssueCoreDisplayCommInvalid, true
	}
	return value, profilerFtraceEventIssueKindCount, false
}

func profilerCoreDisplayCaller(field profilerCoreProtoField) (string, profilerFtraceEventIssueKind, bool) {
	switch {
	case field.wrongWire:
		return "", profilerFtraceEventIssueCoreDisplayCallerStrWrongWire, true
	case field.count > 1:
		return "", profilerFtraceEventIssueCoreDisplayCallerStrDuplicate, true
	case field.count == 0:
		return "", profilerFtraceEventIssueKindCount, false
	}
	value := string(field.bytesValue)
	if !traceDBSinglePhysicalLine(value, true) {
		return "", profilerFtraceEventIssueCoreDisplayCallerStrInvalid, true
	}
	return value, profilerFtraceEventIssueKindCount, false
}

// renderProfilerFtraceCoreEventWithTypedAudit is the single structured-core
// parse/render authority shared by the typed production path and the direct
// compatibility adapter. Internal renderer/schema failures remain typed
// invariants; only source-reachable payload and line failures become issues.
func renderProfilerFtraceCoreEventWithTypedAudit(event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error,
) {
	payload, admission, set, handled, err := decodeProfilerCorePayloadWithTypedAudit(event)
	if !handled || err != nil {
		return "", "", false, nil, handled, err
	}
	name, body, ok, issues, err = finalizeProfilerFtraceCoreEventWithTypedAudit(event, payload, admission, set)
	return name, body, ok, issues, true, err
}

func finalizeProfilerFtraceCoreEventWithTypedAudit(
	event profilerFtraceEventRecord,
	payload coreRenderPayload,
	admission bodyAdmission,
	set profilerFtraceCoreIssueSet,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	// Typed-set corruption must dominate every source verdict, including a
	// simultaneous canonical-line failure. Validate before rendering anything.
	issues, err = set.checked(event.Field)
	if err != nil {
		return "", "", false, nil, err
	}
	switch admission {
	case bodyRejected:
		if payload != (coreRenderPayload{}) || len(issues) != 1 ||
			issues[0].Severity != profilerFtraceEventIssueHardReject {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_core_rejected_verdict_invalid"}
		}
		return "", "", false, issues, nil
	case bodyAdmitted:
		if len(issues) > 0 && issues[0].Severity != profilerFtraceEventIssueAdmittedDisplay {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_core_admitted_verdict_invalid"}
		}
		body, rendered := renderCanonicalCorePayload(payload)
		if !rendered {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_core_payload"}
		}
		if !profilerCanonicalLineValid(event, payload.Name, body) {
			var canonical profilerFtraceCoreIssueSet
			if err := canonical.addFixed(event.Field, profilerFtraceEventIssueCoreInvalidCanonicalLine); err != nil {
				return "", "", false, nil, err
			}
			issues, err = canonical.checked(event.Field)
			return "", "", false, issues, err
		}
		return payload.Name, body, true, issues, nil
	default:
		return "", "", false, nil,
			&traceDBOutputInvariantError{Reason: "profiler_core_admission_invalid"}
	}
}

func profilerCoreDisplayField(eventField, payloadField int) bool {
	return (eventField == 2420 || eventField == 2421 || eventField == 2422) && payloadField == 1 ||
		eventField == 4002 && payloadField == 4
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
