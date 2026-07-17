package hitraceconv

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
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

// decodeProfilerCorePayloadWithTypedAudit is the Background compatibility
// adapter. The Context variant below is the sole structured-core parser.
func decodeProfilerCorePayloadWithTypedAudit(event profilerFtraceEventRecord) (
	coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
) {
	return decodeProfilerCorePayloadWithTypedAuditContext(context.Background(), event)
}

// decodeProfilerCorePayloadWithTypedAuditContext observes every governed
// endpoint in one cancellable wire walk and publishes issues only afterward.
func decodeProfilerCorePayloadWithTypedAuditContext(ctx context.Context, event profilerFtraceEventRecord) (
	coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
) {
	schema, governed := profilerStructuredCoreSchemas[event.Field]
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, governed, err
	}
	if !governed {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, false, nil
	}
	rejectFixed := func(kind profilerFtraceEventIssueKind) (
		coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
	) {
		if err := ctx.Err(); err != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
		var set profilerFtraceCoreIssueSet
		if err := set.addFixed(event.Field, kind); err != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
		return coreRenderPayload{}, bodyRejected, set, true, nil
	}
	rejectPayload := func(kind profilerFtraceEventIssueKind, payloadField int) (
		coreRenderPayload, bodyAdmission, profilerFtraceCoreIssueSet, bool, error,
	) {
		if err := ctx.Err(); err != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
		var set profilerFtraceCoreIssueSet
		if err := set.addPayload(event.Field, kind, payloadField); err != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
		return coreRenderPayload{}, bodyRejected, set, true, nil
	}

	descriptor, ok := profilerFtraceEventDescriptors[event.Field]
	if !ok {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "missing_core_descriptor"}
	}
	if descriptor.Field != event.Field {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "mismatched_core_descriptor_field"}
	}
	kind, ok := coreRenderKindForName(descriptor.Name)
	if !ok {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "invalid_core_descriptor_name"}
	}
	for payloadField, expectedWire := range schema {
		if payloadField < 1 || payloadField > 7 || (expectedWire != 0 && expectedWire != 2) {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true,
				&traceDBOutputInvariantError{Reason: "profiler_core_schema_invalid"}
		}
	}

	var fields [8]profilerCoreProtoField
	if err := walkProfilerProtoFieldsContext(ctx, event.Payload, func(payloadField int, wire int, raw []byte, value uint64) error {
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
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
		if _, invariant := traceDBOutputInvariantReason(err); invariant {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
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
		if values[1] < 0 || values[2] <= 0 || values[3] < 0 {
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
		reason, valid, stringErr := profilerCoreReasonContext(ctx, fields[1])
		if stringErr != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, stringErr
		}
		if !valid {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidReason)
		}
		payload.Interrupt = &coreInterruptPayload{Reason: reason}
	case 1402:
		reason, valid, stringErr := profilerCoreReasonContext(ctx, fields[2])
		if stringErr != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, stringErr
		}
		if !valid {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidReason)
		}
		payload.Interrupt = &coreInterruptPayload{TargetMask: profilerCoreUint64(fields[1]), Reason: reason}
	case 1500:
		irq, valid := profilerCoreInt32(fields[1])
		if !valid || irq < 0 {
			return rejectFixed(profilerFtraceEventIssueCoreMissingOrInvalidIRQ)
		}
		name, valid, stringErr := profilerCoreTokenStringContext(ctx, fields[2])
		if stringErr != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, stringErr
		}
		if !valid {
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
		comm, issueKind, issuePresent, displayErr := profilerCoreDisplayCommContext(ctx, fields[1])
		if displayErr != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, displayErr
		}
		switch {
		case issuePresent:
		case comm == "":
			issueKind, issuePresent = profilerFtraceEventIssueCoreDisplayCommUnavailable, true
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
		caller, issueKind, issuePresent, displayErr := profilerCoreDisplayCallerContext(ctx, fields[4])
		if displayErr != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, displayErr
		}
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
		if err := ctx.Err(); err != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
		payload.Blocked = &blocked
	default:
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "unhandled_core_descriptor"}
	}
	var set profilerFtraceCoreIssueSet
	if displayPresent {
		if err := set.addFixed(event.Field, displayKind); err != nil {
			return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
		}
	}
	if err := ctx.Err(); err != nil {
		return coreRenderPayload{}, bodyUnsupported, profilerFtraceCoreIssueSet{}, true, err
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
	value, kind, present, _ := profilerCoreDisplayCommContext(context.Background(), field)
	return value, kind, present
}

func profilerCoreDisplayCommContext(ctx context.Context, field profilerCoreProtoField) (
	string, profilerFtraceEventIssueKind, bool, error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	switch {
	case field.wrongWire:
		return "", profilerFtraceEventIssueCoreDisplayCommWrongWire, true, nil
	case field.count > 1:
		return "", profilerFtraceEventIssueCoreDisplayCommDuplicate, true, nil
	case field.count == 0:
		return "", profilerFtraceEventIssueKindCount, false, nil
	}
	valid, err := profilerSinglePhysicalLineBytesContext(ctx, field.bytesValue, true)
	if err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	if !valid {
		return "", profilerFtraceEventIssueCoreDisplayCommInvalid, true, nil
	}
	available, err := profilerCoreWakeCommBytesValidContext(ctx, field.bytesValue)
	if err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	if !available {
		return "", profilerFtraceEventIssueCoreDisplayCommUnavailable, true, nil
	}
	if len(field.bytesValue) > maxProfilerWakeCommBytes {
		return "", profilerFtraceEventIssueCoreDisplayCommOutOfProfile, true, nil
	}
	value, err := profilerCloneBytesStringContext(ctx, field.bytesValue)
	if err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	return value, profilerFtraceEventIssueKindCount, false, nil
}

func profilerCoreDisplayCaller(field profilerCoreProtoField) (string, profilerFtraceEventIssueKind, bool) {
	value, kind, present, _ := profilerCoreDisplayCallerContext(context.Background(), field)
	return value, kind, present
}

func profilerCoreDisplayCallerContext(ctx context.Context, field profilerCoreProtoField) (
	string, profilerFtraceEventIssueKind, bool, error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	switch {
	case field.wrongWire:
		return "", profilerFtraceEventIssueCoreDisplayCallerStrWrongWire, true, nil
	case field.count > 1:
		return "", profilerFtraceEventIssueCoreDisplayCallerStrDuplicate, true, nil
	case field.count == 0:
		return "", profilerFtraceEventIssueKindCount, false, nil
	}
	valid, err := profilerSinglePhysicalLineBytesContext(ctx, field.bytesValue, true)
	if err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	if !valid {
		return "", profilerFtraceEventIssueCoreDisplayCallerStrInvalid, true, nil
	}
	if len(field.bytesValue) > 512 {
		return "", profilerFtraceEventIssueCoreDisplayCallerStrInvalid, true, nil
	}
	value, err := profilerCloneBytesStringContext(ctx, field.bytesValue)
	if err != nil {
		return "", profilerFtraceEventIssueKindCount, false, err
	}
	return value, profilerFtraceEventIssueKindCount, false, nil
}

func profilerCoreWakeCommBytesValidContext(ctx context.Context, raw []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(raw) == 0 {
		return false, nil
	}
	first, firstWidth := utf8.DecodeRune(raw)
	last, lastWidth := utf8.DecodeLastRune(raw)
	if firstWidth <= 0 || lastWidth <= 0 {
		return false, &traceDBOutputInvariantError{Reason: "profiler_core_comm_rune_width_invalid"}
	}
	if unicode.IsSpace(first) || unicode.IsSpace(last) {
		return false, nil
	}
	processed := uint64(0)
	for start := 0; start < len(raw); {
		end := min(start+profilerContextByteCheckpointBytes, len(raw))
		if err := profilerByteContextCheckpoint(ctx, &processed, uint64(end-start)); err != nil {
			return false, err
		}
		for _, value := range raw[start:end] {
			if value == '=' || value == '|' {
				return false, nil
			}
		}
		start = end
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
}

// renderProfilerFtraceCoreEventWithTypedAudit is the Background compatibility
// adapter over the Context parse/render authority below.
func renderProfilerFtraceCoreEventWithTypedAudit(event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error,
) {
	return renderProfilerFtraceCoreEventWithTypedAuditContext(context.Background(), event)
}

func renderProfilerFtraceCoreEventWithTypedAuditContext(ctx context.Context, event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	payload, admission, set, handled, err := decodeProfilerCorePayloadWithTypedAuditContext(ctx, event)
	if !handled || err != nil {
		return "", "", false, nil, handled, err
	}
	name, body, ok, issues, err = finalizeProfilerFtraceCoreEventWithTypedAuditContext(ctx, event, payload, admission, set)
	if err != nil {
		return "", "", false, nil, true, err
	}
	return name, body, ok, issues, true, err
}

func finalizeProfilerFtraceCoreEventWithTypedAudit(
	event profilerFtraceEventRecord,
	payload coreRenderPayload,
	admission bodyAdmission,
	set profilerFtraceCoreIssueSet,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	return finalizeProfilerFtraceCoreEventWithTypedAuditContext(
		context.Background(), event, payload, admission, set,
	)
}

// finalizeProfilerFtraceCoreEventWithTypedAuditContext is the sole
// structured-core finalizer. Internal renderer/schema failures remain typed
// invariants; only source-reachable payload and line failures become issues.
func finalizeProfilerFtraceCoreEventWithTypedAuditContext(
	ctx context.Context,
	event profilerFtraceEventRecord,
	payload coreRenderPayload,
	admission bodyAdmission,
	set profilerFtraceCoreIssueSet,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", "", false, nil, err
	}
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
		if err := ctx.Err(); err != nil {
			return "", "", false, nil, err
		}
		return "", "", false, issues, nil
	case bodyAdmitted:
		if len(issues) > 0 && issues[0].Severity != profilerFtraceEventIssueAdmittedDisplay {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_core_admitted_verdict_invalid"}
		}
		body, rendered := renderCanonicalCorePayload(payload)
		if err := ctx.Err(); err != nil {
			return "", "", false, nil, err
		}
		if !rendered {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_core_payload"}
		}
		canonicalValid, canonicalErr := profilerCanonicalLineValidContext(ctx, event, payload.Name, body)
		if canonicalErr != nil {
			return "", "", false, nil, canonicalErr
		}
		if !canonicalValid {
			if err := ctx.Err(); err != nil {
				return "", "", false, nil, err
			}
			var canonical profilerFtraceCoreIssueSet
			if err := canonical.addFixed(event.Field, profilerFtraceEventIssueCoreInvalidCanonicalLine); err != nil {
				return "", "", false, nil, err
			}
			issues, err = canonical.checked(event.Field)
			return "", "", false, issues, err
		}
		if err := ctx.Err(); err != nil {
			return "", "", false, nil, err
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
	value, valid, _ := profilerCoreStringContext(context.Background(), field)
	return value, valid
}

func profilerCoreStringContext(ctx context.Context, field profilerCoreProtoField) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if field.count == 0 {
		return "", true, nil
	}
	if field.wrongWire || field.count > 1 {
		return "", false, nil
	}
	valid, err := profilerSinglePhysicalLineBytesContext(ctx, field.bytesValue, true)
	if err != nil || !valid {
		return "", false, err
	}
	value, err := profilerCloneBytesStringContext(ctx, field.bytesValue)
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func profilerCoreReasonContext(ctx context.Context, field profilerCoreProtoField) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if field.count == 0 || field.wrongWire || field.count > 1 {
		return "", false, nil
	}
	physical, err := profilerSinglePhysicalLineBytesContext(ctx, field.bytesValue, true)
	if err != nil || !physical {
		return "", false, err
	}
	valid, err := validProfilerCoreIPIReasonBytesContext(ctx, field.bytesValue)
	if err != nil || !valid {
		return "", false, err
	}
	value, err := profilerCloneBytesStringContext(ctx, field.bytesValue)
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func profilerCoreTokenStringContext(ctx context.Context, field profilerCoreProtoField) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if field.count == 0 || field.wrongWire || field.count > 1 {
		return "", false, nil
	}
	valid, err := profilerSingleTokenBytesContext(ctx, field.bytesValue)
	if err != nil || !valid {
		return "", false, err
	}
	value, err := profilerCloneBytesStringContext(ctx, field.bytesValue)
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func validProfilerCoreIPIReasonBytesContext(ctx context.Context, raw []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if len(raw) == 0 {
		return false, nil
	}
	first, firstWidth := utf8.DecodeRune(raw)
	last, lastWidth := utf8.DecodeLastRune(raw)
	if firstWidth <= 0 || lastWidth <= 0 {
		return false, &traceDBOutputInvariantError{Reason: "profiler_core_reason_rune_width_invalid"}
	}
	if unicode.IsSpace(first) || unicode.IsSpace(last) {
		return false, nil
	}
	nextCheckpoint := profilerContextByteCheckpointBytes
	for offset := 0; offset < len(raw); {
		if offset >= nextCheckpoint {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			nextCheckpoint = (offset/profilerContextByteCheckpointBytes + 1) * profilerContextByteCheckpointBytes
		}
		r, width := utf8.DecodeRune(raw[offset:])
		if width <= 0 || width > len(raw)-offset {
			return false, &traceDBOutputInvariantError{Reason: "profiler_core_reason_rune_width_invalid"}
		}
		offset += width
		switch r {
		case '\\', '"', '(', ')', '=', '|':
			return false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
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
	valid, _ := profilerCanonicalLineValidContext(context.Background(), event, name, body)
	return valid
}
