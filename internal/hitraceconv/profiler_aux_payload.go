package hitraceconv

import (
	"errors"
	"math"
	"strings"
)

type profilerAuxKind uint8

const (
	profilerAuxPrint profilerAuxKind = iota + 1
	profilerAuxF2FS
	profilerAuxMMCStart
	profilerAuxMMCDone
)

type profilerAuxPayload struct {
	Kind     profilerAuxKind
	Name     string
	Print    *markerPayload
	F2FS     *profilerF2FSPayload
	MMCStart *profilerMMCStartPayload
	MMCDone  *profilerMMCDonePayload
}

type profilerF2FSKind = f2fsPayloadKind

const (
	profilerF2FSSyncEnter  = f2fsPayloadSyncEnter
	profilerF2FSSyncExit   = f2fsPayloadSyncExit
	profilerF2FSWriteBegin = f2fsPayloadWriteBegin
	profilerF2FSWriteEnd   = f2fsPayloadWriteEnd
)

type profilerF2FSPayload = f2fsPayload

type profilerMMCStartPayload = mmcStartPayload

type profilerMMCDonePayload = mmcDonePayload

const maxProfilerMMCResponseBytes = 4 * 4 // source tracepoint is u32 response[4]

// profilerStructuredAuxSchemas pins the generated OpenHarmony default and
// 6.6.30 protobuf layouts at developtools_profiler 5bc8ef5. Field 1109 also
// carries default-profile tracing_mark_write records, while field 4011 has a
// profile-conditional flags field; those presence differences are handled by
// the typed decoder below, never by transport fallback.
var profilerStructuredAuxSchemas = map[int]map[int]int{
	1109: {1: 0, 2: 2},
	4009: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0, 8: 0},
	4010: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
	4011: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
	4012: {1: 0, 2: 0, 3: 0, 4: 0, 5: 0},
	4015: {
		1: 0, 2: 0, 3: 2, 4: 0, 5: 0, 6: 0, 7: 2, 8: 0, 9: 0, 10: 0, 11: 2, 12: 0,
		13: 0, 14: 0, 15: 0, 16: 0, 17: 0, 18: 0, 19: 0, 20: 0, 21: 0, 22: 0, 23: 2,
	},
	4016: {
		1: 0, 2: 0, 3: 0, 4: 0, 5: 0, 6: 0, 7: 0, 8: 0, 9: 0, 10: 0, 11: 0, 12: 0,
		13: 0, 14: 0, 15: 0, 16: 0, 17: 0, 18: 0, 19: 0, 20: 0, 21: 0, 22: 0, 23: 0, 24: 0, 25: 2,
	},
}

const profilerFtraceAuxIssuesPerEvent = 3

type profilerFtraceAuxIssueSet struct {
	Count  uint8
	Issues [profilerFtraceAuxIssuesPerEvent]profilerFtraceEventIssue
}

func (set *profilerFtraceAuxIssueSet) validate(eventField int) error {
	if set == nil || int(set.Count) > len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_count_invalid"}
	}
	if profilerStructuredAuxSchemas[eventField] == nil {
		return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_schema_invalid"}
	}
	displayOnly := set.Count > 1
	previousDisplayField := uint8(0)
	for index, issue := range set.Issues {
		if index >= int(set.Count) {
			if issue != (profilerFtraceEventIssue{}) {
				return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_count_invalid"}
			}
			continue
		}
		if issue.Kind < profilerFtraceEventIssueAuxPayloadMalformedWire ||
			issue.Kind > profilerFtraceEventIssueAuxInvalidCanonicalLine ||
			!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
			return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_schema_invalid"}
		}
		if displayOnly && issue.Severity != profilerFtraceEventIssueAdmittedDisplay {
			return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_arm_invalid"}
		}
		if issue.Severity == profilerFtraceEventIssueAdmittedDisplay {
			if !profilerFtraceAuxResponseField(eventField, int(issue.PayloadField)) ||
				issue.PayloadField <= previousDisplayField {
				return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_display_order_invalid"}
			}
			previousDisplayField = issue.PayloadField
		}
		for prior := 0; prior < index; prior++ {
			if set.Issues[prior] == issue || set.Issues[prior].PayloadField == issue.PayloadField {
				return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_duplicate"}
			}
		}
	}
	return nil
}

func (set *profilerFtraceAuxIssueSet) add(eventField int, issue profilerFtraceEventIssue) error {
	if err := set.validate(eventField); err != nil {
		return err
	}
	if int(set.Count) == len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_overflow"}
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

func (set *profilerFtraceAuxIssueSet) addFixed(eventField int, kind profilerFtraceEventIssueKind) error {
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceAuxIssueSet) addPayload(eventField int, kind profilerFtraceEventIssueKind, payloadField int) error {
	issue, ok := profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_aux_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceAuxIssueSet) checked(eventField int) ([]profilerFtraceEventIssue, error) {
	if err := set.validate(eventField); err != nil {
		return nil, err
	}
	return append([]profilerFtraceEventIssue(nil), set.Issues[:int(set.Count)]...), nil
}

type profilerFtraceAuxTypedResult struct {
	Payload   profilerAuxPayload
	Admission bodyAdmission
	Issues    profilerFtraceAuxIssueSet
	Pair      profilerPairAdmission
	Handled   bool
}

func (result *profilerFtraceAuxTypedResult) rejectFixed(eventField int, kind profilerFtraceEventIssueKind) error {
	result.Payload = profilerAuxPayload{}
	result.Admission = bodyRejected
	result.Pair.Admitted = false
	result.Issues = profilerFtraceAuxIssueSet{}
	return result.Issues.addFixed(eventField, kind)
}

func (result *profilerFtraceAuxTypedResult) rejectPayload(eventField int, kind profilerFtraceEventIssueKind, payloadField int) error {
	result.Payload = profilerAuxPayload{}
	result.Admission = bodyRejected
	result.Pair.Admitted = false
	result.Issues = profilerFtraceAuxIssueSet{}
	return result.Issues.addPayload(eventField, kind, payloadField)
}

func decodeProfilerAuxPayloadWithTypedAudit(event profilerFtraceEventRecord) (profilerFtraceAuxTypedResult, error) {
	result := profilerFtraceAuxTypedResult{Admission: bodyUnsupported, Pair: profilerStructuredF2FSPairFamily(event.Field)}
	schema, governed := profilerStructuredAuxSchemas[event.Field]
	if !governed {
		return result, nil
	}
	result.Handled = true
	descriptor, ok := profilerFtraceEventDescriptors[event.Field]
	if !ok {
		return result, &traceDBOutputInvariantError{Reason: "missing_aux_descriptor"}
	}
	if descriptor.Field != event.Field {
		return result, &traceDBOutputInvariantError{Reason: "mismatched_aux_descriptor_field"}
	}
	expectedName, ok := profilerAuxDescriptorName(event.Field)
	if !ok || descriptor.Name != expectedName {
		return result, &traceDBOutputInvariantError{Reason: "mismatched_aux_descriptor_name"}
	}
	expectedFamily, ok := profilerAuxDescriptorFamily(event.Field)
	if !ok || descriptor.Family != expectedFamily {
		return result, &traceDBOutputInvariantError{Reason: "mismatched_aux_descriptor_family"}
	}
	for payloadField, expectedWire := range schema {
		if payloadField < 1 || payloadField > 25 || (expectedWire != 0 && expectedWire != 2) {
			return result, &traceDBOutputInvariantError{Reason: "profiler_aux_schema_invalid"}
		}
	}

	var fields [26]profilerCoreProtoField
	walkErr := walkProtoFields(event.Payload, func(payloadField int, wire int, raw []byte, value uint64) error {
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
	})
	result.Pair = profilerStructuredF2FSPairIdentity(event, descriptor.Name, &fields, walkErr)
	if walkErr != nil {
		var decodeErr *protoFieldDecodeError
		if !errors.As(walkErr, &decodeErr) {
			return result, &traceDBOutputInvariantError{Reason: "profiler_aux_wire_error_untyped"}
		}
		return result, result.rejectFixed(event.Field, profilerFtraceEventIssueAuxPayloadMalformedWire)
	}

	for payloadField := 1; payloadField <= 25; payloadField++ {
		if _, expected := schema[payloadField]; !expected || profilerFtraceAuxResponseField(event.Field, payloadField) {
			continue
		}
		switch {
		case fields[payloadField].wrongWire:
			return result, result.rejectPayload(event.Field, profilerFtraceEventIssueAuxFieldWrongWire, payloadField)
		case fields[payloadField].count > 1:
			return result, result.rejectPayload(event.Field, profilerFtraceEventIssueAuxFieldDuplicate, payloadField)
		}
	}

	payload := profilerAuxPayload{Name: descriptor.Name}
	var semanticKind profilerFtraceEventIssueKind
	semanticField := 0
	semanticFailed := false
	switch event.Field {
	case 1109:
		buffer, valid := normalizeMarkerBuffer(fields[2].bytesValue)
		if !valid {
			return result, result.rejectFixed(event.Field, profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf)
		}
		payload.Kind = profilerAuxPrint
		payload.Print = &markerPayload{IP: fields[1].uintValue, IPPresent: fields[1].count == 1, Buffer: buffer}
	case 4009:
		item, kind, field, failed := decodeProfilerF2FSSyncEnter(&fields)
		semanticKind, semanticField, semanticFailed = kind, field, failed
		item.Name = descriptor.Name
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4010:
		item, kind, field, failed := decodeProfilerF2FSSyncExit(&fields)
		semanticKind, semanticField, semanticFailed = kind, field, failed
		item.Name = descriptor.Name
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4011:
		item, kind, field, failed := decodeProfilerF2FSWriteBegin(&fields)
		semanticKind, semanticField, semanticFailed = kind, field, failed
		item.Name = descriptor.Name
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4012:
		item, kind, field, failed := decodeProfilerF2FSWriteEnd(&fields)
		semanticKind, semanticField, semanticFailed = kind, field, failed
		item.Name = descriptor.Name
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4015:
		item, kind, field, failed := decodeProfilerMMCDone(&fields)
		semanticKind, semanticField, semanticFailed = kind, field, failed
		payload.Kind, payload.MMCDone = profilerAuxMMCDone, &item
	case 4016:
		item, kind, field, failed := decodeProfilerMMCStart(&fields)
		semanticKind, semanticField, semanticFailed = kind, field, failed
		payload.Kind, payload.MMCStart = profilerAuxMMCStart, &item
	default:
		return result, &traceDBOutputInvariantError{Reason: "unhandled_aux_descriptor"}
	}
	if semanticFailed {
		if semanticField > 0 {
			return result, result.rejectPayload(event.Field, semanticKind, semanticField)
		}
		return result, result.rejectFixed(event.Field, semanticKind)
	}
	if payload.F2FS != nil {
		finalizeF2FSPayloadAdmission(payload.F2FS)
		if !payload.F2FS.IdentityKnown || !payload.F2FS.PayloadAdmitted {
			return result, &traceDBOutputInvariantError{Reason: "invalid_f2fs_payload_range"}
		}
	}

	var set profilerFtraceAuxIssueSet
	if event.Field == 4015 {
		for _, payloadField := range [...]int{3, 7, 11} {
			kind := profilerFtraceEventIssueKindCount
			switch {
			case fields[payloadField].wrongWire:
				kind = profilerFtraceEventIssueAuxResponseWrongWire
			case fields[payloadField].count > 1:
				kind = profilerFtraceEventIssueAuxResponseDuplicate
			case len(fields[payloadField].bytesValue) > maxProfilerMMCResponseBytes:
				kind = profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile
			}
			if kind != profilerFtraceEventIssueKindCount {
				if err := set.addPayload(event.Field, kind, payloadField); err != nil {
					return result, err
				}
			}
		}
	}
	result.Payload, result.Admission, result.Issues = payload, bodyAdmitted, set
	result.Pair.Admitted = !result.Pair.Governed || result.Pair.LaneKnown
	return result, nil
}

func decodeProfilerAuxPayload(event profilerFtraceEventRecord) (profilerAuxPayload, bodyAdmission, string) {
	result, err := decodeProfilerAuxPayloadWithTypedAudit(event)
	if err != nil {
		if reason, ok := traceDBOutputInvariantReason(err); ok {
			return profilerAuxPayload{}, bodyRejected, reason
		}
		return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_audit_failed"
	}
	if !result.Handled {
		return profilerAuxPayload{}, bodyUnsupported, ""
	}
	issues, checkErr := result.Issues.checked(event.Field)
	if checkErr != nil {
		return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_issue_invalid"
	}
	labels, valid := profilerFtraceEventIssueLabels(event.Field, issues)
	if !valid {
		return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_issue_invalid"
	}
	if result.Admission == bodyRejected {
		if len(labels) != 1 {
			return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_issue_invalid"
		}
		return profilerAuxPayload{}, bodyRejected, labels[0]
	}
	return result.Payload, result.Admission, ""
}

func decodeProfilerAuxPayloadWithPairAdmission(event profilerFtraceEventRecord) (profilerAuxPayload, bodyAdmission, string, profilerPairAdmission) {
	result, err := decodeProfilerAuxPayloadWithTypedAudit(event)
	if err != nil {
		if reason, ok := traceDBOutputInvariantReason(err); ok {
			return profilerAuxPayload{}, bodyRejected, reason, result.Pair
		}
		return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_audit_failed", result.Pair
	}
	if !result.Handled {
		return profilerAuxPayload{}, bodyUnsupported, "", result.Pair
	}
	issues, checkErr := result.Issues.checked(event.Field)
	if checkErr != nil {
		return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_issue_invalid", result.Pair
	}
	labels, valid := profilerFtraceEventIssueLabels(event.Field, issues)
	if !valid {
		return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_issue_invalid", result.Pair
	}
	if result.Admission == bodyRejected {
		if len(labels) != 1 {
			return profilerAuxPayload{}, bodyRejected, "profiler_aux_typed_issue_invalid", result.Pair
		}
		return profilerAuxPayload{}, bodyRejected, labels[0], result.Pair
	}
	return result.Payload, result.Admission, "", result.Pair
}

func profilerStructuredF2FSPairFamily(field int) profilerPairAdmission {
	if profilerStructuredPairEventField(pairRenderF2FS, field) {
		return profilerPairAdmission{Kind: pairRenderF2FS, Governed: true}
	}
	return profilerPairAdmission{}
}

// profilerStructuredF2FSPairIdentity recovers only the exact hard key from the
// already schema-audited protobuf fields. Non-key field failures retain this
// verdict so they quarantine one dev/inode/op/emitter lane; malformed key wire
// or owner identity leaves LaneKnown false and therefore closes the family.
func profilerStructuredF2FSPairIdentity(event profilerFtraceEventRecord, name string, fields *[26]profilerCoreProtoField, walkErr error) profilerPairAdmission {
	out := profilerStructuredF2FSPairFamily(event.Field)
	if !out.Governed || fields == nil || fields[1].wrongWire || fields[1].count != 1 ||
		fields[2].wrongWire || fields[2].count != 1 {
		return out
	}
	if walkErr != nil {
		var decodeErr *protoFieldDecodeError
		if !errors.As(walkErr, &decodeErr) || !decodeErr.FieldKnown || !decodeErr.Terminal ||
			decodeErr.Field == 1 || decodeErr.Field == 2 {
			return out
		}
	}
	// HeaderOwnerKnown is the single typed authority for the emitter. A PID
	// value in the decoded record is not itself proof that the producer emitted
	// one complete, unambiguous common_pid field; in particular, synthetic or
	// future callers may provide a plausible PID with the explicit authority bit
	// clear and no degradation strings. Never reconstruct that hard gate from
	// the absence of diagnostics.
	ownerKnown := event.HeaderOwnerKnown && event.PID >= 0 && event.PID <= math.MaxInt32
	if !ownerKnown {
		return out
	}
	dev, ino, _, _, failed := profilerAuxF2FSIdentity(fields)
	if failed {
		return out
	}
	kind, known := directF2FSKindForName(name)
	if !known {
		return out
	}
	payload := f2fsPayload{
		Kind: kind, Name: name, HeaderTID: event.PID,
		Dev: dev, Ino: ino, IdentityKnown: true,
	}
	verdict := fingerprintPairingEndpoint(f2fsPayloadTypedInput(payload))
	if !verdict.Recognized || !verdict.KeyKnown || !verdict.EmitterKnown ||
		!verdict.EmitterAdmitted || verdict.SemanticKey == "" {
		return out
	}
	out.LaneKnown = true
	out.Lane = verdict.SemanticKey
	return out
}

func profilerAuxDescriptorName(field int) (string, bool) {
	switch field {
	case 1109:
		return "print", true
	case 4009:
		return "f2fs_sync_file_enter", true
	case 4010:
		return "f2fs_sync_file_exit", true
	case 4011:
		return "f2fs_write_begin", true
	case 4012:
		return "f2fs_write_end", true
	case 4015:
		return "mmc_request_done", true
	case 4016:
		return "mmc_request_start", true
	default:
		return "", false
	}
}

func profilerAuxDescriptorFamily(field int) (string, bool) {
	switch field {
	case 1109:
		return "trace_marker", true
	case 4009, 4010, 4011, 4012:
		return "f2fs", true
	case 4015, 4016:
		return "mmc", true
	default:
		return "", false
	}
}

func profilerAuxUint32(fields *[26]profilerCoreProtoField, field int) (uint32, bool) {
	value, valid := profilerCoreUint32(fields[field])
	return uint32(value), valid
}

func profilerAuxDecodedInt32(fields *[26]profilerCoreProtoField, field int) (int64, bool) {
	return profilerCoreInt32(fields[field])
}

func profilerAuxF2FSIdentity(fields *[26]profilerCoreProtoField) (
	uint32, uint64, profilerFtraceEventIssueKind, int, bool,
) {
	rawDev := profilerCoreUint64(fields[1])
	if rawDev == 0 {
		return 0, 0, profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev, 0, true
	}
	if rawDev > math.MaxUint32 {
		return 0, 0, profilerFtraceEventIssueAuxFieldOutOfRange, 1, true
	}
	ino := profilerCoreUint64(fields[2])
	if ino == 0 {
		return 0, 0, profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno, 0, true
	}
	return uint32(rawDev), ino, profilerFtraceEventIssueKindCount, 0, false
}

func decodeProfilerF2FSSyncEnter(fields *[26]profilerCoreProtoField) (
	profilerF2FSPayload, profilerFtraceEventIssueKind, int, bool,
) {
	dev, ino, kind, field, failed := profilerAuxF2FSIdentity(fields)
	if failed {
		return profilerF2FSPayload{}, kind, field, true
	}
	valid := false
	mode, valid := profilerAuxUint32(fields, 4)
	if !valid || mode > math.MaxUint16 {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 4, true
	}
	nlink, valid := profilerAuxUint32(fields, 6)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 6, true
	}
	advise, valid := profilerAuxUint32(fields, 8)
	if !valid || advise > math.MaxUint8 {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 8, true
	}
	size := profilerCoreUint64(fields[5])
	if size > math.MaxInt64 {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 5, true
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSSyncEnter, Dev: dev, Ino: ino,
		Pino: profilerCoreUint64(fields[3]), Mode: mode, Size: int64(size),
		Nlink: nlink, Blocks: profilerCoreUint64(fields[7]), Advise: advise,
	}, profilerFtraceEventIssueKindCount, 0, false
}

func decodeProfilerF2FSSyncExit(fields *[26]profilerCoreProtoField) (
	profilerF2FSPayload, profilerFtraceEventIssueKind, int, bool,
) {
	dev, ino, kind, field, failed := profilerAuxF2FSIdentity(fields)
	if failed {
		return profilerF2FSPayload{}, kind, field, true
	}
	valid := false
	cpReason, valid := profilerAuxDecodedInt32(fields, 3)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 3, true
	}
	datasync, valid := profilerAuxDecodedInt32(fields, 4)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 4, true
	}
	ret, valid := profilerAuxDecodedInt32(fields, 5)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 5, true
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSSyncExit, Dev: dev, Ino: ino,
		CPReason: cpReason, DataSync: datasync, Ret: ret,
	}, profilerFtraceEventIssueKindCount, 0, false
}

func decodeProfilerF2FSWriteBegin(fields *[26]profilerCoreProtoField) (
	profilerF2FSPayload, profilerFtraceEventIssueKind, int, bool,
) {
	dev, ino, kind, field, failed := profilerAuxF2FSIdentity(fields)
	if failed {
		return profilerF2FSPayload{}, kind, field, true
	}
	valid := false
	length, valid := profilerAuxUint32(fields, 4)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 4, true
	}
	flags := uint32(0)
	flagsPresent := fields[5].count == 1
	if flagsPresent {
		flags, valid = profilerAuxUint32(fields, 5)
		if !valid {
			return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 5, true
		}
	}
	pos := profilerCoreUint64(fields[3])
	if pos > math.MaxInt64 {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 3, true
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSWriteBegin, Dev: dev, Ino: ino,
		Pos: pos, Len: uint64(length), Flags: flags, FlagsPresent: flagsPresent,
	}, profilerFtraceEventIssueKindCount, 0, false
}

func decodeProfilerF2FSWriteEnd(fields *[26]profilerCoreProtoField) (
	profilerF2FSPayload, profilerFtraceEventIssueKind, int, bool,
) {
	dev, ino, kind, field, failed := profilerAuxF2FSIdentity(fields)
	if failed {
		return profilerF2FSPayload{}, kind, field, true
	}
	valid := false
	length, valid := profilerAuxUint32(fields, 4)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 4, true
	}
	copied, valid := profilerAuxUint32(fields, 5)
	if !valid {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 5, true
	}
	pos := profilerCoreUint64(fields[3])
	if pos > math.MaxInt64 {
		return profilerF2FSPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, 3, true
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSWriteEnd, Dev: dev, Ino: ino,
		Pos: pos, Len: uint64(length), Copied: copied,
	}, profilerFtraceEventIssueKindCount, 0, false
}

func decodeProfilerMMCStart(fields *[26]profilerCoreProtoField) (
	profilerMMCStartPayload, profilerFtraceEventIssueKind, int, bool,
) {
	var item profilerMMCStartPayload
	uintTargets := []struct {
		field int
		out   *uint32
	}{
		{1, &item.CmdOpcode}, {2, &item.CmdArg}, {3, &item.CmdFlags}, {4, &item.CmdRetries},
		{5, &item.StopOpcode}, {6, &item.StopArg}, {7, &item.StopFlags}, {8, &item.StopRetries},
		{9, &item.SBCOpcode}, {10, &item.SBCArg}, {11, &item.SBCFlags}, {12, &item.SBCRetries},
		{13, &item.Blocks}, {14, &item.BlockAddr}, {15, &item.BlockSize}, {16, &item.DataFlags},
		{18, &item.CanRetune}, {19, &item.DoingRetune}, {20, &item.RetuneNow}, {23, &item.RetunePeriod},
	}
	for _, target := range uintTargets {
		value, valid := profilerAuxUint32(fields, target.field)
		if !valid {
			return profilerMMCStartPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, target.field, true
		}
		*target.out = value
	}
	intTargets := []struct {
		field int
		out   *int64
	}{{17, &item.Tag}, {21, &item.NeedRetune}, {22, &item.HoldRetune}}
	for _, target := range intTargets {
		value, valid := profilerAuxDecodedInt32(fields, target.field)
		if !valid {
			return profilerMMCStartPayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, target.field, true
		}
		*target.out = value
	}
	item.MRQ = profilerCoreUint64(fields[24])
	if item.MRQ == 0 {
		return profilerMMCStartPayload{}, profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer, 0, true
	}
	name, valid := profilerCoreString(fields[25])
	if !valid || !validProfilerMMCName(name) {
		return profilerMMCStartPayload{}, profilerFtraceEventIssueAuxMissingOrInvalidMMCName, 0, true
	}
	item.Name = name
	return item, profilerFtraceEventIssueKindCount, 0, false
}

func decodeProfilerMMCDone(fields *[26]profilerCoreProtoField) (
	profilerMMCDonePayload, profilerFtraceEventIssueKind, int, bool,
) {
	var item profilerMMCDonePayload
	uintTargets := []struct {
		field int
		out   *uint32
	}{
		{1, &item.CmdOpcode}, {4, &item.CmdRetries}, {5, &item.StopOpcode}, {8, &item.StopRetries},
		{9, &item.SBCOpcode}, {12, &item.SBCRetries}, {13, &item.BytesXfered},
		{16, &item.CanRetune}, {17, &item.DoingRetune}, {18, &item.RetuneNow}, {21, &item.RetunePeriod},
	}
	for _, target := range uintTargets {
		value, valid := profilerAuxUint32(fields, target.field)
		if !valid {
			return profilerMMCDonePayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, target.field, true
		}
		*target.out = value
	}
	intTargets := []struct {
		field int
		out   *int64
	}{
		{2, &item.CmdErr}, {6, &item.StopErr}, {10, &item.SBCErr}, {14, &item.DataErr},
		{15, &item.Tag}, {19, &item.NeedRetune}, {20, &item.HoldRetune},
	}
	for _, target := range intTargets {
		value, valid := profilerAuxDecodedInt32(fields, target.field)
		if !valid {
			return profilerMMCDonePayload{}, profilerFtraceEventIssueAuxFieldOutOfRange, target.field, true
		}
		*target.out = value
	}
	item.MRQ = profilerCoreUint64(fields[22])
	if item.MRQ == 0 {
		return profilerMMCDonePayload{}, profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer, 0, true
	}
	name, valid := profilerCoreString(fields[23])
	if !valid || !validProfilerMMCName(name) {
		return profilerMMCDonePayload{}, profilerFtraceEventIssueAuxMissingOrInvalidMMCName, 0, true
	}
	item.Name = name
	return item, profilerFtraceEventIssueKindCount, 0, false
}

func validProfilerMMCName(name string) bool {
	return len(name) <= 256 && traceDBSingleToken(name) && !strings.ContainsAny(name, ":[]=,|'\"")
}

func renderCanonicalProfilerAuxPayload(payload profilerAuxPayload) (string, bool) {
	switch payload.Kind {
	case profilerAuxPrint:
		if payload.Print == nil {
			return "", false
		}
		return renderCanonicalMarkerPayload(*payload.Print)
	case profilerAuxF2FS:
		if payload.F2FS == nil {
			return "", false
		}
		return renderCanonicalF2FSPayload(*payload.F2FS)
	case profilerAuxMMCStart:
		if payload.MMCStart == nil {
			return "", false
		}
		return renderCanonicalMMCPayload(mmcPayload{Kind: mmcPayloadStart, Name: payload.Name, Start: payload.MMCStart})
	case profilerAuxMMCDone:
		if payload.MMCDone == nil {
			return "", false
		}
		// The pinned producer loses the source u32[4] response shape by
		// serializing it as NUL-terminated strings. Audit those fields above,
		// but never rebuild four zero-padded response words from lost bytes.
		return renderCanonicalMMCPayload(mmcPayload{Kind: mmcPayloadDone, Name: payload.Name, Done: payload.MMCDone})
	default:
		return "", false
	}
}

// renderProfilerFtraceAuxEventWithTypedAudit is the single structured-aux
// parse/render authority. The pair sidecar is returned from the same decode so
// container callers never need a second payload walk to recover F2FS identity.
func renderProfilerFtraceAuxEventWithTypedAudit(event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue,
	pair profilerPairAdmission, handled bool, err error,
) {
	result, err := decodeProfilerAuxPayloadWithTypedAudit(event)
	if !result.Handled || err != nil {
		return "", "", false, nil, result.Pair, result.Handled, err
	}
	name, body, ok, issues, err = finalizeProfilerFtraceAuxEventWithTypedAudit(event, result)
	return name, body, ok, issues, result.Pair, true, err
}

func finalizeProfilerFtraceAuxEventWithTypedAudit(
	event profilerFtraceEventRecord,
	result profilerFtraceAuxTypedResult,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	// Typed-set corruption must dominate source verdicts and canonical checks.
	issues, err = result.Issues.checked(event.Field)
	if err != nil {
		return "", "", false, nil, err
	}
	if !result.Handled {
		return "", "", false, nil,
			&traceDBOutputInvariantError{Reason: "profiler_aux_finalize_unhandled"}
	}
	if !profilerFtraceAuxPairAdmissionValid(event.Field, result.Admission, result.Pair) {
		return "", "", false, nil,
			&traceDBOutputInvariantError{Reason: "profiler_aux_pair_verdict_invalid"}
	}
	switch result.Admission {
	case bodyRejected:
		if result.Payload != (profilerAuxPayload{}) || len(issues) != 1 ||
			issues[0].Severity != profilerFtraceEventIssueHardReject || result.Pair.Admitted {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_aux_rejected_verdict_invalid"}
		}
		return "", "", false, issues, nil
	case bodyAdmitted:
		for _, issue := range issues {
			if issue.Severity != profilerFtraceEventIssueAdmittedDisplay {
				return "", "", false, nil,
					&traceDBOutputInvariantError{Reason: "profiler_aux_admitted_verdict_invalid"}
			}
		}
		body, rendered := renderCanonicalProfilerAuxPayload(result.Payload)
		if !rendered {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_aux_payload"}
		}
		if !profilerCanonicalLineValid(event, result.Payload.Name, body) {
			if event.Field != 1109 {
				return "", "", false, nil,
					&traceDBOutputInvariantError{Reason: "invalid_canonical_aux_line"}
			}
			var canonical profilerFtraceAuxIssueSet
			if err := canonical.addFixed(event.Field, profilerFtraceEventIssueAuxInvalidCanonicalLine); err != nil {
				return "", "", false, nil, err
			}
			issues, err = canonical.checked(event.Field)
			return "", "", false, issues, err
		}
		return result.Payload.Name, body, true, issues, nil
	default:
		return "", "", false, nil,
			&traceDBOutputInvariantError{Reason: "profiler_aux_admission_invalid"}
	}
}

func profilerFtraceAuxPairAdmissionValid(eventField int, admission bodyAdmission, pair profilerPairAdmission) bool {
	expected := profilerStructuredF2FSPairFamily(eventField)
	if expected.Governed {
		if !pair.Governed || pair.Kind != expected.Kind || pair.LaneKnown != (pair.Lane != "") {
			return false
		}
		switch admission {
		case bodyRejected:
			return !pair.Admitted
		case bodyAdmitted:
			return pair.Admitted == pair.LaneKnown
		default:
			return false
		}
	}
	if pair.Governed || pair.Kind != pairRenderUnknown || pair.LaneKnown || pair.Lane != "" {
		return false
	}
	return pair.Admitted == (admission == bodyAdmitted)
}
