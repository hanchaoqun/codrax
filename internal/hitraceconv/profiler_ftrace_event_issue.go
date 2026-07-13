package hitraceconv

import (
	"strconv"
)

// profilerFtraceEventIssueSeverity distinguishes rows which cannot be
// published from rows whose canonical body remains authoritative after
// optional display metadata is omitted.
type profilerFtraceEventIssueSeverity uint8

const (
	profilerFtraceEventIssueHardReject profilerFtraceEventIssueSeverity = iota
	profilerFtraceEventIssueAdmittedDisplay
	profilerFtraceEventIssueSeverityCount
)

// profilerFtraceEventIssueKind is the closed event-diagnostic vocabulary.
// Parameterized protobuf field numbers live in PayloadField; they are never
// recovered from display labels.
type profilerFtraceEventIssueKind uint8

const (
	profilerFtraceEventIssueEnvelopeEventMalformedWire profilerFtraceEventIssueKind = iota
	profilerFtraceEventIssueEnvelopeTracePluginMalformedWire
	profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire
	profilerFtraceEventIssueEnvelopeEventContainerWrongWire
	profilerFtraceEventIssueEnvelopeOverwriteInvalid
	profilerFtraceEventIssueEnvelopeCPUDuplicate
	profilerFtraceEventIssueEnvelopeCPUWrongWire
	profilerFtraceEventIssueEnvelopeCPUOutOfRange
	profilerFtraceEventIssueEnvelopeTimestampDuplicate
	profilerFtraceEventIssueEnvelopeTimestampWrongWire
	profilerFtraceEventIssueEnvelopeTimestampOutOfRange
	profilerFtraceEventIssueEnvelopeTGIDDuplicate
	profilerFtraceEventIssueEnvelopeTGIDWrongWire
	profilerFtraceEventIssueEnvelopeTGIDOutOfRange
	profilerFtraceEventIssueEnvelopeCommDuplicate
	profilerFtraceEventIssueEnvelopeCommWrongWire
	profilerFtraceEventIssueEnvelopeCommInvalid
	profilerFtraceEventIssueEnvelopeCommonFieldsMissing
	profilerFtraceEventIssueEnvelopeCommonFieldsDuplicate
	profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire
	profilerFtraceEventIssueEnvelopeCommonFieldsMalformedWire
	profilerFtraceEventIssueEnvelopeCommonTypeDuplicate
	profilerFtraceEventIssueEnvelopeCommonTypeWrongWire
	profilerFtraceEventIssueEnvelopeCommonTypeSourceWidth
	profilerFtraceEventIssueEnvelopeCommonFlagsDuplicate
	profilerFtraceEventIssueEnvelopeCommonFlagsWrongWire
	profilerFtraceEventIssueEnvelopeCommonFlagsSourceWidth
	profilerFtraceEventIssueEnvelopeCommonPreemptCountDuplicate
	profilerFtraceEventIssueEnvelopeCommonPreemptCountWrongWire
	profilerFtraceEventIssueEnvelopeCommonPreemptCountSourceWidth
	profilerFtraceEventIssueEnvelopeCommonPIDDuplicate
	profilerFtraceEventIssueEnvelopeCommonPIDWrongWire
	profilerFtraceEventIssueEnvelopeCommonPIDOutOfRange
	profilerFtraceEventIssueEnvelopeOneofMissing
	profilerFtraceEventIssueEnvelopeOneofMultiple
	profilerFtraceEventIssueEnvelopeOneofWrongWire
	profilerFtraceEventIssueEnvelopeIdentityIncomplete

	profilerFtraceEventIssueCorePayloadMalformedWire
	profilerFtraceEventIssueCoreFieldWrongWire
	profilerFtraceEventIssueCoreFieldDuplicate
	profilerFtraceEventIssueCoreFieldOutOfRange
	profilerFtraceEventIssueCoreInvalidTransactionID
	profilerFtraceEventIssueCoreInvalidTransactionEndpoint
	profilerFtraceEventIssueCoreInvalidReply
	profilerFtraceEventIssueCoreMissingOrInvalidReason
	profilerFtraceEventIssueCoreMissingOrInvalidIRQ
	profilerFtraceEventIssueCoreMissingOrInvalidIRQName
	profilerFtraceEventIssueCoreMissingOrInvalidRet
	profilerFtraceEventIssueCoreMissingOrInvalidVec
	profilerFtraceEventIssueCoreMissingOrInvalidState
	profilerFtraceEventIssueCoreMissingOrInvalidCPUID
	profilerFtraceEventIssueCoreInvalidLimitsProfile
	profilerFtraceEventIssueCoreInvalidLimitsOrder
	profilerFtraceEventIssueCoreMissingOrInvalidPID
	profilerFtraceEventIssueCoreMissingOrInvalidPriority
	profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU
	profilerFtraceEventIssueCoreMissingOrInvalidIOWait
	profilerFtraceEventIssueCoreDisplayCommWrongWire
	profilerFtraceEventIssueCoreDisplayCommDuplicate
	profilerFtraceEventIssueCoreDisplayCommInvalid
	profilerFtraceEventIssueCoreDisplayCommUnavailable
	profilerFtraceEventIssueCoreDisplayCommOutOfProfile
	profilerFtraceEventIssueCoreDisplayCallerStrWrongWire
	profilerFtraceEventIssueCoreDisplayCallerStrDuplicate
	profilerFtraceEventIssueCoreDisplayCallerStrInvalid
	profilerFtraceEventIssueCoreInvalidCanonicalLine

	profilerFtraceEventIssueAuxPayloadMalformedWire
	profilerFtraceEventIssueAuxFieldWrongWire
	profilerFtraceEventIssueAuxFieldDuplicate
	profilerFtraceEventIssueAuxResponseWrongWire
	profilerFtraceEventIssueAuxResponseDuplicate
	profilerFtraceEventIssueAuxFieldOutOfRange
	profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf
	profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev
	profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno
	profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer
	profilerFtraceEventIssueAuxMissingOrInvalidMMCName
	profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile
	profilerFtraceEventIssueAuxInvalidCanonicalLine

	profilerFtraceEventIssueFilemapPayloadMalformedWire
	profilerFtraceEventIssueFilemapPFNInvalid
	profilerFtraceEventIssueFilemapInodeInvalid
	profilerFtraceEventIssueFilemapIndexInvalid
	profilerFtraceEventIssueFilemapDeviceInvalid
	profilerFtraceEventIssueFilemapOrderInvalid

	profilerFtraceEventIssueBlockPayloadMalformedWire
	profilerFtraceEventIssueBlockFieldMalformedWire
	profilerFtraceEventIssueBlockFieldWrongWire
	profilerFtraceEventIssueBlockFieldDuplicate
	profilerFtraceEventIssueBlockFieldOutOfRange
	profilerFtraceEventIssueBlockFieldMissingOrInvalid
	profilerFtraceEventIssueBlockInvalidCanonicalLine
	profilerFtraceEventIssueBlockCommMalformedWire
	profilerFtraceEventIssueBlockCommWrongWire
	profilerFtraceEventIssueBlockCommDuplicate
	profilerFtraceEventIssueBlockCommUnsafeOmitted
	profilerFtraceEventIssueBlockCmdMalformedWire
	profilerFtraceEventIssueBlockCmdWrongWire
	profilerFtraceEventIssueBlockCmdDuplicate
	profilerFtraceEventIssueBlockCmdUnsafeOmitted

	profilerFtraceEventIssueWirePayloadMalformedWire
	profilerFtraceEventIssueWireFieldMalformedWire
	profilerFtraceEventIssueWireFieldWrongWire
	profilerFtraceEventIssueWireFieldDuplicate
	profilerFtraceEventIssueWireFieldOutOfRange
	profilerFtraceEventIssueWireFieldMissingOrInvalid
	profilerFtraceEventIssueWireCPUIDMalformedWire
	profilerFtraceEventIssueWireCPUIDWrongWire
	profilerFtraceEventIssueWireCPUIDDuplicate
	profilerFtraceEventIssueWireCPUIDOutOfRange
	profilerFtraceEventIssueWireNextInfoMalformedWire
	profilerFtraceEventIssueWireNextInfoWrongWire
	profilerFtraceEventIssueWireNextInfoDuplicate
	profilerFtraceEventIssueWireInvalidCanonicalLine

	profilerFtraceEventIssueUnmappedField
	profilerFtraceEventIssueKindCount
)

type profilerFtraceEventIssue struct {
	Kind         profilerFtraceEventIssueKind
	PayloadField uint8
	Severity     profilerFtraceEventIssueSeverity
}

// profilerFtraceEventFixedIssue is the only constructor for issues whose
// payload field is fixed by the closed issue vocabulary. Producers name the
// semantic kind; this constructor derives both PayloadField and Severity and
// rejects any event/kind combination outside the exact schema.
func profilerFtraceEventFixedIssue(eventField int, kind profilerFtraceEventIssueKind) (profilerFtraceEventIssue, bool) {
	payloadField, ok := profilerFtraceEventIssueFixedPayloadField(kind, eventField)
	if !ok || profilerFtraceEventIssueParameterizedKind(kind) {
		return profilerFtraceEventIssue{}, false
	}
	issue := profilerFtraceEventIssue{Kind: kind, PayloadField: payloadField}
	issue.Severity = issue.expectedSeverity()
	return issue, issue.validFor(eventField)
}

// profilerFtraceEventPayloadIssue is the only constructor for parameterized
// protobuf-field issues. The integer boundary is checked before narrowing to
// the wire-sized PayloadField representation.
func profilerFtraceEventPayloadIssue(eventField int, kind profilerFtraceEventIssueKind, payloadField int) (profilerFtraceEventIssue, bool) {
	if payloadField < 1 || payloadField > 255 || !profilerFtraceEventIssueParameterizedKind(kind) {
		return profilerFtraceEventIssue{}, false
	}
	issue := profilerFtraceEventIssue{Kind: kind, PayloadField: uint8(payloadField)}
	issue.Severity = issue.expectedSeverity()
	return issue, issue.validFor(eventField)
}

// profilerFtraceEventIssueLabels is the sole typed-to-legacy display adapter.
// Detection must never consume these labels; a malformed typed issue fails
// closed instead of creating a second string authority.
func profilerFtraceEventIssueLabels(eventField int, issues []profilerFtraceEventIssue) ([]string, bool) {
	labels := make([]string, 0, len(issues))
	for _, issue := range issues {
		label, ok := issue.label(eventField)
		if !ok {
			return nil, false
		}
		labels = append(labels, label)
	}
	return labels, true
}

func (issue profilerFtraceEventIssue) compare(other profilerFtraceEventIssue) int {
	if issue.Kind < other.Kind {
		return -1
	}
	if issue.Kind > other.Kind {
		return 1
	}
	if issue.PayloadField < other.PayloadField {
		return -1
	}
	if issue.PayloadField > other.PayloadField {
		return 1
	}
	if issue.Severity < other.Severity {
		return -1
	}
	if issue.Severity > other.Severity {
		return 1
	}
	return 0
}

// profilerFtraceEventIssueFixedPayloadField assigns the exact protobuf field
// governed by a fixed issue. Zero is reserved for whole-message or multi-field
// invariants; it is never a substitute for an unknown field number.
func profilerFtraceEventIssueFixedPayloadField(kind profilerFtraceEventIssueKind, eventField int) (uint8, bool) {
	switch kind {
	case profilerFtraceEventIssueEnvelopeEventMalformedWire,
		profilerFtraceEventIssueEnvelopeTracePluginMalformedWire,
		profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire,
		profilerFtraceEventIssueEnvelopeOneofMissing,
		profilerFtraceEventIssueEnvelopeOneofMultiple,
		profilerFtraceEventIssueEnvelopeOneofWrongWire,
		profilerFtraceEventIssueCorePayloadMalformedWire,
		profilerFtraceEventIssueCoreInvalidLimitsProfile,
		profilerFtraceEventIssueCoreInvalidLimitsOrder,
		profilerFtraceEventIssueCoreInvalidCanonicalLine,
		profilerFtraceEventIssueAuxPayloadMalformedWire,
		profilerFtraceEventIssueAuxInvalidCanonicalLine,
		profilerFtraceEventIssueFilemapPayloadMalformedWire,
		profilerFtraceEventIssueBlockPayloadMalformedWire,
		profilerFtraceEventIssueBlockInvalidCanonicalLine,
		profilerFtraceEventIssueWirePayloadMalformedWire,
		profilerFtraceEventIssueWireInvalidCanonicalLine,
		profilerFtraceEventIssueUnmappedField:
		return 0, true
	case profilerFtraceEventIssueEnvelopeCPUDuplicate,
		profilerFtraceEventIssueEnvelopeCPUWrongWire,
		profilerFtraceEventIssueEnvelopeCPUOutOfRange,
		profilerFtraceEventIssueEnvelopeTimestampDuplicate,
		profilerFtraceEventIssueEnvelopeTimestampWrongWire,
		profilerFtraceEventIssueEnvelopeTimestampOutOfRange,
		profilerFtraceEventIssueEnvelopeCommonTypeDuplicate,
		profilerFtraceEventIssueEnvelopeCommonTypeWrongWire,
		profilerFtraceEventIssueEnvelopeCommonTypeSourceWidth,
		profilerFtraceEventIssueCoreInvalidTransactionID,
		profilerFtraceEventIssueCoreMissingOrInvalidIRQ,
		profilerFtraceEventIssueCoreMissingOrInvalidVec,
		profilerFtraceEventIssueCoreMissingOrInvalidState,
		profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev,
		profilerFtraceEventIssueFilemapPFNInvalid:
		return 1, true
	case profilerFtraceEventIssueEnvelopeEventContainerWrongWire,
		profilerFtraceEventIssueEnvelopeTGIDDuplicate,
		profilerFtraceEventIssueEnvelopeTGIDWrongWire,
		profilerFtraceEventIssueEnvelopeTGIDOutOfRange,
		profilerFtraceEventIssueEnvelopeCommonFlagsDuplicate,
		profilerFtraceEventIssueEnvelopeCommonFlagsWrongWire,
		profilerFtraceEventIssueEnvelopeCommonFlagsSourceWidth,
		profilerFtraceEventIssueCoreMissingOrInvalidIRQName,
		profilerFtraceEventIssueCoreMissingOrInvalidRet,
		profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf,
		profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno,
		profilerFtraceEventIssueFilemapInodeInvalid:
		return 2, true
	case profilerFtraceEventIssueEnvelopeOverwriteInvalid,
		profilerFtraceEventIssueEnvelopeCommDuplicate,
		profilerFtraceEventIssueEnvelopeCommWrongWire,
		profilerFtraceEventIssueEnvelopeCommInvalid,
		profilerFtraceEventIssueEnvelopeCommonPreemptCountDuplicate,
		profilerFtraceEventIssueEnvelopeCommonPreemptCountWrongWire,
		profilerFtraceEventIssueEnvelopeCommonPreemptCountSourceWidth,
		profilerFtraceEventIssueCoreMissingOrInvalidPriority,
		profilerFtraceEventIssueCoreMissingOrInvalidIOWait,
		profilerFtraceEventIssueFilemapIndexInvalid,
		profilerFtraceEventIssueWireCPUIDMalformedWire,
		profilerFtraceEventIssueWireCPUIDWrongWire,
		profilerFtraceEventIssueWireCPUIDDuplicate,
		profilerFtraceEventIssueWireCPUIDOutOfRange:
		return 3, true
	case profilerFtraceEventIssueEnvelopeCommonPIDDuplicate,
		profilerFtraceEventIssueEnvelopeCommonPIDWrongWire,
		profilerFtraceEventIssueEnvelopeCommonPIDOutOfRange,
		profilerFtraceEventIssueCoreDisplayCallerStrWrongWire,
		profilerFtraceEventIssueCoreDisplayCallerStrDuplicate,
		profilerFtraceEventIssueCoreDisplayCallerStrInvalid,
		profilerFtraceEventIssueFilemapDeviceInvalid:
		return 4, true
	case profilerFtraceEventIssueEnvelopeCommonFieldsMissing,
		profilerFtraceEventIssueEnvelopeCommonFieldsDuplicate,
		profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire,
		profilerFtraceEventIssueEnvelopeCommonFieldsMalformedWire:
		return 50, true
	case profilerFtraceEventIssueCoreInvalidReply,
		profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU,
		profilerFtraceEventIssueFilemapOrderInvalid:
		return 5, true
	case profilerFtraceEventIssueWireNextInfoMalformedWire,
		profilerFtraceEventIssueWireNextInfoWrongWire,
		profilerFtraceEventIssueWireNextInfoDuplicate:
		return 8, true
	case profilerFtraceEventIssueCoreInvalidTransactionEndpoint:
		// The endpoint invariant jointly covers dest_node/proc/thread.
		return 0, true
	case profilerFtraceEventIssueEnvelopeIdentityIncomplete:
		// The invariant jointly covers outer TGID and nested common PID.
		return 0, true
	case profilerFtraceEventIssueCoreMissingOrInvalidReason:
		if eventField == 1402 {
			return 2, true
		}
		return 1, eventField == 1400 || eventField == 1401
	case profilerFtraceEventIssueCoreMissingOrInvalidCPUID:
		if eventField == 2004 {
			return 3, true
		}
		return 2, eventField == 2003 || eventField == 2005
	case profilerFtraceEventIssueCoreMissingOrInvalidPID:
		if eventField == 4002 {
			return 1, true
		}
		return 2, eventField >= 2420 && eventField <= 2422
	case profilerFtraceEventIssueCoreDisplayCommWrongWire,
		profilerFtraceEventIssueCoreDisplayCommDuplicate,
		profilerFtraceEventIssueCoreDisplayCommInvalid,
		profilerFtraceEventIssueCoreDisplayCommUnavailable,
		profilerFtraceEventIssueCoreDisplayCommOutOfProfile:
		return 1, true
	case profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer:
		if eventField == 4015 {
			return 22, true
		}
		return 24, eventField == 4016
	case profilerFtraceEventIssueAuxMissingOrInvalidMMCName:
		if eventField == 4015 {
			return 23, true
		}
		return 25, eventField == 4016
	case profilerFtraceEventIssueBlockCommMalformedWire,
		profilerFtraceEventIssueBlockCommWrongWire,
		profilerFtraceEventIssueBlockCommDuplicate,
		profilerFtraceEventIssueBlockCommUnsafeOmitted:
		if eventField == 204 {
			return 5, true
		}
		return 6, eventField == 210 || eventField == 211
	case profilerFtraceEventIssueBlockCmdMalformedWire,
		profilerFtraceEventIssueBlockCmdWrongWire,
		profilerFtraceEventIssueBlockCmdDuplicate,
		profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		if eventField == 209 {
			return 6, true
		}
		return 7, eventField == 210 || eventField == 211
	default:
		return 0, false
	}
}

func (issue profilerFtraceEventIssue) validFor(eventField int) bool {
	if issue.Kind >= profilerFtraceEventIssueKindCount || issue.Severity >= profilerFtraceEventIssueSeverityCount ||
		issue.Severity != issue.expectedSeverity() {
		return false
	}
	if !profilerFtraceEventIssueParameterizedKind(issue.Kind) {
		payloadField, ok := profilerFtraceEventIssueFixedPayloadField(issue.Kind, eventField)
		if !ok || issue.PayloadField != payloadField {
			return false
		}
	}
	switch {
	case issue.Kind <= profilerFtraceEventIssueEnvelopeIdentityIncomplete:
		switch issue.Kind {
		case profilerFtraceEventIssueEnvelopeTracePluginMalformedWire,
			profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire,
			profilerFtraceEventIssueEnvelopeEventContainerWrongWire,
			profilerFtraceEventIssueEnvelopeOverwriteInvalid:
			return eventField == profilerFtraceCPUDetailEnvelopeField
		case profilerFtraceEventIssueEnvelopeEventMalformedWire:
			return eventField == 0
		case profilerFtraceEventIssueEnvelopeOneofMissing,
			profilerFtraceEventIssueEnvelopeOneofMultiple,
			profilerFtraceEventIssueEnvelopeOneofWrongWire:
			return eventField == 0
		case profilerFtraceEventIssueEnvelopeCPUDuplicate,
			profilerFtraceEventIssueEnvelopeCPUWrongWire,
			profilerFtraceEventIssueEnvelopeCPUOutOfRange:
			return eventField == profilerFtraceCPUDetailEnvelopeField || eventField == 0 ||
				eventField >= 100 && eventField <= profilerFtraceUnknownEventAggregateField
		default:
			return eventField == 0 || eventField >= 100 && eventField <= profilerFtraceUnknownEventAggregateField
		}
	case issue.Kind >= profilerFtraceEventIssueCorePayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueCoreInvalidCanonicalLine:
		return issue.validCore(eventField)
	case issue.Kind >= profilerFtraceEventIssueAuxPayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueAuxInvalidCanonicalLine:
		return issue.validAux(eventField)
	case issue.Kind >= profilerFtraceEventIssueFilemapPayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueFilemapOrderInvalid:
		return eventField == 1000 || eventField == 1001
	case issue.Kind >= profilerFtraceEventIssueBlockPayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		return issue.validBlock(eventField)
	case issue.Kind >= profilerFtraceEventIssueWirePayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueWireInvalidCanonicalLine:
		return issue.validWire(eventField)
	case issue.Kind == profilerFtraceEventIssueUnmappedField:
		_, known := profilerFtraceEventDescriptors[eventField]
		return eventField >= 100 && eventField <= profilerFtraceUnknownEventAggregateField && !known
	default:
		return false
	}
}

func profilerFtraceEventIssueParameterizedKind(kind profilerFtraceEventIssueKind) bool {
	switch kind {
	case profilerFtraceEventIssueCoreFieldWrongWire, profilerFtraceEventIssueCoreFieldDuplicate,
		profilerFtraceEventIssueCoreFieldOutOfRange,
		profilerFtraceEventIssueAuxFieldWrongWire, profilerFtraceEventIssueAuxFieldDuplicate,
		profilerFtraceEventIssueAuxResponseWrongWire, profilerFtraceEventIssueAuxResponseDuplicate,
		profilerFtraceEventIssueAuxFieldOutOfRange, profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile,
		profilerFtraceEventIssueBlockFieldMalformedWire, profilerFtraceEventIssueBlockFieldWrongWire,
		profilerFtraceEventIssueBlockFieldDuplicate, profilerFtraceEventIssueBlockFieldOutOfRange,
		profilerFtraceEventIssueBlockFieldMissingOrInvalid,
		profilerFtraceEventIssueWireFieldMalformedWire, profilerFtraceEventIssueWireFieldWrongWire,
		profilerFtraceEventIssueWireFieldDuplicate, profilerFtraceEventIssueWireFieldOutOfRange,
		profilerFtraceEventIssueWireFieldMissingOrInvalid:
		return true
	default:
		return false
	}
}

func (issue profilerFtraceEventIssue) validCore(eventField int) bool {
	schema := profilerStructuredCoreSchemas[eventField]
	if schema == nil {
		return false
	}
	switch issue.Kind {
	case profilerFtraceEventIssueCoreFieldWrongWire, profilerFtraceEventIssueCoreFieldDuplicate,
		profilerFtraceEventIssueCoreFieldOutOfRange:
		_, known := schema[int(issue.PayloadField)]
		if !known || profilerCoreDisplayField(eventField, int(issue.PayloadField)) {
			return false
		}
		if issue.Kind != profilerFtraceEventIssueCoreFieldOutOfRange {
			return true
		}
		return (eventField == 113 && issue.PayloadField >= 1 && issue.PayloadField <= 7) ||
			(eventField >= 2420 && eventField <= 2422 && issue.PayloadField == 4)
	default:
	}
	switch issue.Kind {
	case profilerFtraceEventIssueCorePayloadMalformedWire:
		return true
	case profilerFtraceEventIssueCoreInvalidCanonicalLine:
		// Only source-controlled, unbounded core strings can push an otherwise
		// valid canonical row over the 1 MiB line cap. Numeric bodies, wake comm
		// and blocked caller display are bounded before this endpoint.
		return eventField == 1400 || eventField == 1401 || eventField == 1402 || eventField == 1500
	case profilerFtraceEventIssueCoreInvalidTransactionID:
		return eventField == 113 || eventField == 119
	case profilerFtraceEventIssueCoreInvalidTransactionEndpoint, profilerFtraceEventIssueCoreInvalidReply:
		return eventField == 113
	case profilerFtraceEventIssueCoreMissingOrInvalidReason:
		return eventField >= 1400 && eventField <= 1402
	case profilerFtraceEventIssueCoreMissingOrInvalidIRQ:
		return eventField == 1500 || eventField == 1501
	case profilerFtraceEventIssueCoreMissingOrInvalidIRQName:
		return eventField == 1500
	case profilerFtraceEventIssueCoreMissingOrInvalidRet:
		return eventField == 1501
	case profilerFtraceEventIssueCoreMissingOrInvalidVec:
		return eventField >= 1502 && eventField <= 1504
	case profilerFtraceEventIssueCoreMissingOrInvalidState:
		return eventField == 2003 || eventField == 2005
	case profilerFtraceEventIssueCoreMissingOrInvalidCPUID:
		return eventField >= 2003 && eventField <= 2005
	case profilerFtraceEventIssueCoreInvalidLimitsProfile, profilerFtraceEventIssueCoreInvalidLimitsOrder:
		return eventField == 2004
	case profilerFtraceEventIssueCoreMissingOrInvalidPID:
		return eventField == 2420 || eventField == 2421 || eventField == 2422 || eventField == 4002
	case profilerFtraceEventIssueCoreMissingOrInvalidPriority, profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU,
		profilerFtraceEventIssueCoreDisplayCommWrongWire, profilerFtraceEventIssueCoreDisplayCommDuplicate,
		profilerFtraceEventIssueCoreDisplayCommInvalid, profilerFtraceEventIssueCoreDisplayCommUnavailable,
		profilerFtraceEventIssueCoreDisplayCommOutOfProfile:
		return eventField >= 2420 && eventField <= 2422
	case profilerFtraceEventIssueCoreMissingOrInvalidIOWait,
		profilerFtraceEventIssueCoreDisplayCallerStrWrongWire, profilerFtraceEventIssueCoreDisplayCallerStrDuplicate,
		profilerFtraceEventIssueCoreDisplayCallerStrInvalid:
		return eventField == 4002
	default:
		return false
	}
}

func (issue profilerFtraceEventIssue) validAux(eventField int) bool {
	schema := profilerStructuredAuxSchemas[eventField]
	if schema == nil {
		return false
	}
	switch issue.Kind {
	case profilerFtraceEventIssueAuxFieldWrongWire, profilerFtraceEventIssueAuxFieldDuplicate:
		_, known := schema[int(issue.PayloadField)]
		return known && !profilerFtraceAuxResponseField(eventField, int(issue.PayloadField))
	case profilerFtraceEventIssueAuxFieldOutOfRange:
		return profilerFtraceAuxRangeFieldKnown(eventField, int(issue.PayloadField))
	case profilerFtraceEventIssueAuxResponseWrongWire, profilerFtraceEventIssueAuxResponseDuplicate,
		profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile:
		return eventField == 4015 && (issue.PayloadField == 3 || issue.PayloadField == 7 || issue.PayloadField == 11)
	default:
	}
	switch issue.Kind {
	case profilerFtraceEventIssueAuxPayloadMalformedWire:
		return true
	case profilerFtraceEventIssueAuxInvalidCanonicalLine:
		return eventField == 1109
	case profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf:
		return eventField == 1109
	case profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev, profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno:
		return eventField >= 4009 && eventField <= 4012
	case profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer, profilerFtraceEventIssueAuxMissingOrInvalidMMCName:
		return eventField == 4015 || eventField == 4016
	default:
		return false
	}
}

func profilerFtraceAuxResponseField(eventField, payloadField int) bool {
	return eventField == 4015 && (payloadField == 3 || payloadField == 7 || payloadField == 11)
}

func profilerFtraceAuxRangeFieldKnown(eventField, payloadField int) bool {
	switch eventField {
	case 4009:
		return payloadField == 1 || payloadField == 4 || payloadField == 5 || payloadField == 6 || payloadField == 8
	case 4010, 4011, 4012:
		return payloadField == 1 || payloadField >= 3 && payloadField <= 5
	case 4015:
		switch payloadField {
		case 1, 2, 4, 5, 6, 8, 9, 10, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21:
			return true
		}
	case 4016:
		return payloadField >= 1 && payloadField <= 23
	}
	return false
}

func (issue profilerFtraceEventIssue) validBlock(eventField int) bool {
	_, _, known := blockRenderKindForProfilerField(eventField)
	if !known {
		return false
	}
	switch issue.Kind {
	case profilerFtraceEventIssueBlockPayloadMalformedWire:
		return true
	case profilerFtraceEventIssueBlockInvalidCanonicalLine:
		return eventField == 204 || eventField == 209 || eventField == 210 || eventField == 211
	}
	if issue.Kind >= profilerFtraceEventIssueBlockFieldMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockFieldMissingOrInvalid {
		if issue.PayloadField == 0 || !profilerFtraceBlockPayloadFieldKnown(eventField, int(issue.PayloadField)) {
			return false
		}
		switch issue.Kind {
		case profilerFtraceEventIssueBlockFieldOutOfRange:
			return !profilerFtraceBlockRWBSField(eventField, int(issue.PayloadField))
		case profilerFtraceEventIssueBlockFieldMissingOrInvalid:
			return profilerFtraceBlockRWBSField(eventField, int(issue.PayloadField))
		default:
			return true
		}
	}
	switch issue.Kind {
	case profilerFtraceEventIssueBlockCommMalformedWire, profilerFtraceEventIssueBlockCommWrongWire,
		profilerFtraceEventIssueBlockCommDuplicate, profilerFtraceEventIssueBlockCommUnsafeOmitted:
		role, _, roleKnown := profilerFtraceBlockFieldSchema(eventField, int(issue.PayloadField))
		return roleKnown && role == profilerFtraceBlockFieldComm
	case profilerFtraceEventIssueBlockCmdMalformedWire, profilerFtraceEventIssueBlockCmdWrongWire,
		profilerFtraceEventIssueBlockCmdDuplicate, profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		role, _, roleKnown := profilerFtraceBlockFieldSchema(eventField, int(issue.PayloadField))
		return roleKnown && role == profilerFtraceBlockFieldCmd
	default:
		return false
	}
}

func profilerFtraceBlockPayloadFieldKnown(eventField, payloadField int) bool {
	role, _, known := profilerFtraceBlockFieldSchema(eventField, payloadField)
	return known && !profilerFtraceBlockDisplayRole(role)
}

func profilerFtraceBlockRWBSField(eventField, payloadField int) bool {
	role, _, known := profilerFtraceBlockFieldSchema(eventField, payloadField)
	return known && role == profilerFtraceBlockFieldRWBS
}

func (issue profilerFtraceEventIssue) validWire(eventField int) bool {
	switch issue.Kind {
	case profilerFtraceEventIssueWirePayloadMalformedWire, profilerFtraceEventIssueWireInvalidCanonicalLine:
		return eventField == 410 || eventField == 2002 || eventField == 2417
	case profilerFtraceEventIssueWireFieldMalformedWire, profilerFtraceEventIssueWireFieldWrongWire,
		profilerFtraceEventIssueWireFieldDuplicate, profilerFtraceEventIssueWireFieldOutOfRange,
		profilerFtraceEventIssueWireFieldMissingOrInvalid:
		known := false
		switch eventField {
		case 410, 2002:
			known = issue.PayloadField == 1 || issue.PayloadField == 2
		case 2417:
			known = issue.PayloadField >= 1 && issue.PayloadField <= 7
		}
		if !known {
			return false
		}
		switch issue.Kind {
		case profilerFtraceEventIssueWireFieldMissingOrInvalid:
			return (eventField == 410 || eventField == 2002) && issue.PayloadField == 1 ||
				eventField == 2417 && (issue.PayloadField == 1 || issue.PayloadField == 5)
		case profilerFtraceEventIssueWireFieldOutOfRange:
			return eventField == 2417 && (issue.PayloadField == 2 || issue.PayloadField == 3 ||
				issue.PayloadField == 6 || issue.PayloadField == 7)
		default:
			return true
		}
	case profilerFtraceEventIssueWireCPUIDMalformedWire, profilerFtraceEventIssueWireCPUIDWrongWire,
		profilerFtraceEventIssueWireCPUIDDuplicate, profilerFtraceEventIssueWireCPUIDOutOfRange:
		return eventField == 2002
	case profilerFtraceEventIssueWireNextInfoMalformedWire, profilerFtraceEventIssueWireNextInfoWrongWire,
		profilerFtraceEventIssueWireNextInfoDuplicate:
		return eventField == 2417
	default:
		return false
	}
}

func (issue profilerFtraceEventIssue) expectedSeverity() profilerFtraceEventIssueSeverity {
	if profilerFtraceEventIssueDisplayKind(issue.Kind) ||
		(issue.Kind >= profilerFtraceEventIssueBlockCommMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockCmdUnsafeOmitted) ||
		(issue.Kind >= profilerFtraceEventIssueWireCPUIDMalformedWire && issue.Kind <= profilerFtraceEventIssueWireNextInfoDuplicate) {
		return profilerFtraceEventIssueAdmittedDisplay
	}
	return profilerFtraceEventIssueHardReject
}

func profilerFtraceEventIssueDisplayKind(kind profilerFtraceEventIssueKind) bool {
	return (kind >= profilerFtraceEventIssueCoreDisplayCommWrongWire && kind <= profilerFtraceEventIssueCoreDisplayCallerStrInvalid) ||
		kind == profilerFtraceEventIssueAuxResponseWrongWire || kind == profilerFtraceEventIssueAuxResponseDuplicate ||
		kind == profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile
}

func (issue profilerFtraceEventIssue) sourceClass() profilerFtraceEventDegradationKind {
	switch {
	case issue.Kind <= profilerFtraceEventIssueEnvelopeIdentityIncomplete:
		return profilerFtraceEventDegradationEnvelope
	case issue.Kind >= profilerFtraceEventIssueCorePayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueCoreInvalidCanonicalLine:
		if issue.Kind >= profilerFtraceEventIssueCoreDisplayCommWrongWire && issue.Kind <= profilerFtraceEventIssueCoreDisplayCallerStrInvalid {
			return profilerFtraceEventDegradationCoreDisplay
		}
		return profilerFtraceEventDegradationCorePayload
	case issue.Kind >= profilerFtraceEventIssueAuxPayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueAuxInvalidCanonicalLine:
		if profilerFtraceEventIssueDisplayKind(issue.Kind) {
			return profilerFtraceEventDegradationAuxDisplay
		}
		return profilerFtraceEventDegradationAuxPayload
	case issue.Kind >= profilerFtraceEventIssueFilemapPayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueFilemapOrderInvalid:
		return profilerFtraceEventDegradationFilemapPayload
	case issue.Kind >= profilerFtraceEventIssueBlockPayloadMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockInvalidCanonicalLine:
		return profilerFtraceEventDegradationBlockPayload
	case issue.Kind >= profilerFtraceEventIssueBlockCommMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		return profilerFtraceEventDegradationBlockDisplay
	case issue.Kind == profilerFtraceEventIssueWirePayloadMalformedWire ||
		issue.Kind == profilerFtraceEventIssueWireInvalidCanonicalLine:
		return profilerFtraceEventDegradationWireAudit
	case issue.Kind >= profilerFtraceEventIssueWireFieldMalformedWire && issue.Kind <= profilerFtraceEventIssueWireFieldMissingOrInvalid:
		return profilerFtraceEventDegradationWireAudit
	case issue.Kind >= profilerFtraceEventIssueWireCPUIDMalformedWire && issue.Kind <= profilerFtraceEventIssueWireNextInfoDuplicate:
		return profilerFtraceEventDegradationFieldAudit
	case issue.Kind == profilerFtraceEventIssueUnmappedField:
		return profilerFtraceEventDegradationUnmappedField
	default:
		return profilerFtraceEventDegradationKindCount
	}
}

func (issue profilerFtraceEventIssue) label(eventField int) (string, bool) {
	if !issue.validFor(eventField) {
		return "", false
	}
	if label, ok := profilerFtraceEventFixedIssueLabels[issue.Kind]; ok {
		return label, true
	}
	field := strconv.Itoa(int(issue.PayloadField))
	switch issue.Kind {
	case profilerFtraceEventIssueCoreFieldWrongWire, profilerFtraceEventIssueAuxFieldWrongWire,
		profilerFtraceEventIssueAuxResponseWrongWire,
		profilerFtraceEventIssueBlockFieldWrongWire, profilerFtraceEventIssueWireFieldWrongWire:
		return "core_field" + field + "_wrong_wire", true
	case profilerFtraceEventIssueCoreFieldDuplicate, profilerFtraceEventIssueAuxFieldDuplicate,
		profilerFtraceEventIssueAuxResponseDuplicate,
		profilerFtraceEventIssueBlockFieldDuplicate, profilerFtraceEventIssueWireFieldDuplicate:
		return "core_field" + field + "_duplicate", true
	case profilerFtraceEventIssueCoreFieldOutOfRange, profilerFtraceEventIssueAuxFieldOutOfRange,
		profilerFtraceEventIssueBlockFieldOutOfRange, profilerFtraceEventIssueWireFieldOutOfRange:
		return "core_field" + field + "_out_of_range", true
	case profilerFtraceEventIssueBlockFieldMalformedWire, profilerFtraceEventIssueWireFieldMalformedWire:
		return "core_field" + field + "_malformed_wire", true
	case profilerFtraceEventIssueBlockFieldMissingOrInvalid, profilerFtraceEventIssueWireFieldMissingOrInvalid:
		return "core_field" + field + "_missing_or_invalid", true
	case profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile:
		return "drop_response_field" + field + "_out_of_source_profile", true
	default:
		return "", false
	}
}

// profilerFtraceEventFixedIssueLabels is the sole fixed diagnostic label
// authority. It is intentionally one-way: producers construct typed issues,
// and only the output boundary renders labels.
var profilerFtraceEventFixedIssueLabels = map[profilerFtraceEventIssueKind]string{
	profilerFtraceEventIssueEnvelopeEventMalformedWire:            "envelope_event_malformed_wire",
	profilerFtraceEventIssueEnvelopeTracePluginMalformedWire:      "envelope_trace_plugin_malformed_wire",
	profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire:        "envelope_cpu_detail_malformed_wire",
	profilerFtraceEventIssueEnvelopeEventContainerWrongWire:       "envelope_event_container_wrong_wire",
	profilerFtraceEventIssueEnvelopeOverwriteInvalid:              "envelope_overwrite_invalid",
	profilerFtraceEventIssueEnvelopeCPUDuplicate:                  "envelope_cpu_duplicate",
	profilerFtraceEventIssueEnvelopeCPUWrongWire:                  "envelope_cpu_wrong_wire",
	profilerFtraceEventIssueEnvelopeCPUOutOfRange:                 "envelope_cpu_out_of_range",
	profilerFtraceEventIssueEnvelopeTimestampDuplicate:            "envelope_timestamp_duplicate",
	profilerFtraceEventIssueEnvelopeTimestampWrongWire:            "envelope_timestamp_wrong_wire",
	profilerFtraceEventIssueEnvelopeTimestampOutOfRange:           "envelope_timestamp_out_of_range",
	profilerFtraceEventIssueEnvelopeTGIDDuplicate:                 "envelope_tgid_duplicate",
	profilerFtraceEventIssueEnvelopeTGIDWrongWire:                 "envelope_tgid_wrong_wire",
	profilerFtraceEventIssueEnvelopeTGIDOutOfRange:                "envelope_tgid_out_of_range",
	profilerFtraceEventIssueEnvelopeCommDuplicate:                 "envelope_comm_duplicate",
	profilerFtraceEventIssueEnvelopeCommWrongWire:                 "envelope_comm_wrong_wire",
	profilerFtraceEventIssueEnvelopeCommInvalid:                   "envelope_comm_invalid",
	profilerFtraceEventIssueEnvelopeCommonFieldsMissing:           "envelope_common_fields_missing",
	profilerFtraceEventIssueEnvelopeCommonFieldsDuplicate:         "envelope_common_fields_duplicate",
	profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire:         "envelope_common_fields_wrong_wire",
	profilerFtraceEventIssueEnvelopeCommonFieldsMalformedWire:     "envelope_common_fields_malformed_wire",
	profilerFtraceEventIssueEnvelopeCommonTypeDuplicate:           "envelope_common_type_duplicate",
	profilerFtraceEventIssueEnvelopeCommonTypeWrongWire:           "envelope_common_type_wrong_wire",
	profilerFtraceEventIssueEnvelopeCommonTypeSourceWidth:         "envelope_common_type_source_width",
	profilerFtraceEventIssueEnvelopeCommonFlagsDuplicate:          "envelope_common_flags_duplicate",
	profilerFtraceEventIssueEnvelopeCommonFlagsWrongWire:          "envelope_common_flags_wrong_wire",
	profilerFtraceEventIssueEnvelopeCommonFlagsSourceWidth:        "envelope_common_flags_source_width",
	profilerFtraceEventIssueEnvelopeCommonPreemptCountDuplicate:   "envelope_common_preempt_count_duplicate",
	profilerFtraceEventIssueEnvelopeCommonPreemptCountWrongWire:   "envelope_common_preempt_count_wrong_wire",
	profilerFtraceEventIssueEnvelopeCommonPreemptCountSourceWidth: "envelope_common_preempt_count_source_width",
	profilerFtraceEventIssueEnvelopeCommonPIDDuplicate:            "envelope_common_pid_duplicate",
	profilerFtraceEventIssueEnvelopeCommonPIDWrongWire:            "envelope_common_pid_wrong_wire",
	profilerFtraceEventIssueEnvelopeCommonPIDOutOfRange:           "envelope_common_pid_out_of_range",
	profilerFtraceEventIssueEnvelopeOneofMissing:                  "envelope_oneof_missing",
	profilerFtraceEventIssueEnvelopeOneofMultiple:                 "envelope_oneof_multiple",
	profilerFtraceEventIssueEnvelopeOneofWrongWire:                "envelope_oneof_wrong_wire",
	profilerFtraceEventIssueEnvelopeIdentityIncomplete:            "envelope_identity_incomplete",
	profilerFtraceEventIssueCorePayloadMalformedWire:              "core_payload_malformed_wire",
	profilerFtraceEventIssueCoreInvalidTransactionID:              "invalid_transaction_id",
	profilerFtraceEventIssueCoreInvalidTransactionEndpoint:        "invalid_transaction_endpoint",
	profilerFtraceEventIssueCoreInvalidReply:                      "invalid_reply",
	profilerFtraceEventIssueCoreMissingOrInvalidReason:            "missing_or_invalid_reason",
	profilerFtraceEventIssueCoreMissingOrInvalidIRQ:               "missing_or_invalid_irq",
	profilerFtraceEventIssueCoreMissingOrInvalidIRQName:           "missing_or_invalid_irq_name",
	profilerFtraceEventIssueCoreMissingOrInvalidRet:               "missing_or_invalid_ret",
	profilerFtraceEventIssueCoreMissingOrInvalidVec:               "missing_or_invalid_vec",
	profilerFtraceEventIssueCoreMissingOrInvalidState:             "missing_or_invalid_state",
	profilerFtraceEventIssueCoreMissingOrInvalidCPUID:             "missing_or_invalid_cpu_id",
	profilerFtraceEventIssueCoreInvalidLimitsProfile:              "invalid_limits_profile",
	profilerFtraceEventIssueCoreInvalidLimitsOrder:                "invalid_limits_order",
	profilerFtraceEventIssueCoreMissingOrInvalidPID:               "missing_or_invalid_pid",
	profilerFtraceEventIssueCoreMissingOrInvalidPriority:          "missing_or_invalid_priority",
	profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU:         "missing_or_invalid_target_cpu",
	profilerFtraceEventIssueCoreMissingOrInvalidIOWait:            "missing_or_invalid_iowait",
	profilerFtraceEventIssueCoreDisplayCommWrongWire:              "display_comm_wrong_wire",
	profilerFtraceEventIssueCoreDisplayCommDuplicate:              "display_comm_duplicate",
	profilerFtraceEventIssueCoreDisplayCommInvalid:                "display_comm_invalid",
	profilerFtraceEventIssueCoreDisplayCommUnavailable:            "display_comm_unavailable",
	profilerFtraceEventIssueCoreDisplayCommOutOfProfile:           "display_comm_out_of_profile",
	profilerFtraceEventIssueCoreDisplayCallerStrWrongWire:         "display_caller_str_wrong_wire",
	profilerFtraceEventIssueCoreDisplayCallerStrDuplicate:         "display_caller_str_duplicate",
	profilerFtraceEventIssueCoreDisplayCallerStrInvalid:           "display_caller_str_invalid",
	profilerFtraceEventIssueCoreInvalidCanonicalLine:              "invalid_canonical_core_line",
	profilerFtraceEventIssueAuxPayloadMalformedWire:               "aux_payload_malformed_wire",
	profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf:           "missing_or_invalid_print_buf",
	profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev:            "missing_or_invalid_f2fs_dev",
	profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno:            "missing_or_invalid_f2fs_ino",
	profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer:         "missing_or_invalid_mmc_pointer",
	profilerFtraceEventIssueAuxMissingOrInvalidMMCName:            "missing_or_invalid_mmc_name",
	profilerFtraceEventIssueAuxInvalidCanonicalLine:               "invalid_canonical_aux_line",
	profilerFtraceEventIssueFilemapPayloadMalformedWire:           "filemap_payload_malformed_wire",
	profilerFtraceEventIssueFilemapPFNInvalid:                     "filemap_pfn_invalid",
	profilerFtraceEventIssueFilemapInodeInvalid:                   "filemap_inode_invalid",
	profilerFtraceEventIssueFilemapIndexInvalid:                   "filemap_index_invalid",
	profilerFtraceEventIssueFilemapDeviceInvalid:                  "filemap_device_invalid",
	profilerFtraceEventIssueFilemapOrderInvalid:                   "filemap_order_invalid",
	profilerFtraceEventIssueBlockPayloadMalformedWire:             "block_payload_malformed_wire",
	profilerFtraceEventIssueBlockInvalidCanonicalLine:             "invalid_canonical_block_line",
	profilerFtraceEventIssueBlockCommMalformedWire:                "comm_malformed_wire",
	profilerFtraceEventIssueBlockCommWrongWire:                    "comm_wrong_wire",
	profilerFtraceEventIssueBlockCommDuplicate:                    "comm_duplicate",
	profilerFtraceEventIssueBlockCommUnsafeOmitted:                "comm_unsafe_omitted",
	profilerFtraceEventIssueBlockCmdMalformedWire:                 "cmd_malformed_wire",
	profilerFtraceEventIssueBlockCmdWrongWire:                     "cmd_wrong_wire",
	profilerFtraceEventIssueBlockCmdDuplicate:                     "cmd_duplicate",
	profilerFtraceEventIssueBlockCmdUnsafeOmitted:                 "cmd_unsafe_omitted",
	profilerFtraceEventIssueWirePayloadMalformedWire:              "wire_payload_malformed_wire",
	profilerFtraceEventIssueWireInvalidCanonicalLine:              "invalid_canonical_wire_line",
	profilerFtraceEventIssueWireCPUIDMalformedWire:                "cpu_id_malformed_wire",
	profilerFtraceEventIssueWireCPUIDWrongWire:                    "cpu_id_wrong_wire",
	profilerFtraceEventIssueWireCPUIDDuplicate:                    "cpu_id_duplicate",
	profilerFtraceEventIssueWireCPUIDOutOfRange:                   "cpu_id_out_of_range",
	profilerFtraceEventIssueWireNextInfoMalformedWire:             "next_info_malformed_wire",
	profilerFtraceEventIssueWireNextInfoWrongWire:                 "next_info_wrong_wire",
	profilerFtraceEventIssueWireNextInfoDuplicate:                 "next_info_duplicate",
	profilerFtraceEventIssueUnmappedField:                         "unmapped structured ftrace event field",
}
