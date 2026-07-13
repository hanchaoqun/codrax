package hitraceconv

import (
	"strconv"
	"strings"
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
// recovered from labels after this legacy ingress bridge.
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
	profilerFtraceEventIssueAuxFieldOutOfRange
	profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf
	profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev
	profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno
	profilerFtraceEventIssueAuxInvalidF2FSPayloadRange
	profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer
	profilerFtraceEventIssueAuxMissingOrInvalidMMCName
	profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile
	profilerFtraceEventIssueAuxInvalidCanonicalLine

	profilerFtraceEventIssueFilemapPFNInvalid
	profilerFtraceEventIssueFilemapInodeInvalid
	profilerFtraceEventIssueFilemapIndexInvalid
	profilerFtraceEventIssueFilemapDeviceInvalid
	profilerFtraceEventIssueFilemapOrderInvalid
	profilerFtraceEventIssueFilemapInvalidCanonicalLine

	profilerFtraceEventIssueBlockFieldMalformedWire
	profilerFtraceEventIssueBlockFieldWrongWire
	profilerFtraceEventIssueBlockFieldDuplicate
	profilerFtraceEventIssueBlockFieldOutOfRange
	profilerFtraceEventIssueBlockFieldMissingOrInvalid
	profilerFtraceEventIssueBlockCommMalformedWire
	profilerFtraceEventIssueBlockCommWrongWire
	profilerFtraceEventIssueBlockCommDuplicate
	profilerFtraceEventIssueBlockCommUnsafeOmitted
	profilerFtraceEventIssueBlockCmdMalformedWire
	profilerFtraceEventIssueBlockCmdWrongWire
	profilerFtraceEventIssueBlockCmdDuplicate
	profilerFtraceEventIssueBlockCmdUnsafeOmitted

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

	profilerFtraceEventIssueUnmappedField
	profilerFtraceEventIssueKindCount
)

type profilerFtraceEventIssue struct {
	Kind         profilerFtraceEventIssueKind
	PayloadField uint8
	Severity     profilerFtraceEventIssueSeverity
}

func profilerFtraceEventIssueFromLegacy(eventField int, legacySource profilerFtraceEventDegradationKind, token string) (profilerFtraceEventIssue, bool) {
	issue := profilerFtraceEventIssue{Severity: profilerFtraceEventIssueHardReject}
	set := func(kind profilerFtraceEventIssueKind) (profilerFtraceEventIssue, bool) {
		issue.Kind = kind
		payloadField, ok := profilerFtraceEventIssueFixedPayloadField(kind, eventField)
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		issue.PayloadField = payloadField
		return issue, issue.validFor(eventField)
	}
	fieldIssue := func(prefix string, suffixes map[string]profilerFtraceEventIssueKind) (profilerFtraceEventIssue, bool) {
		field, suffix, ok := profilerFtraceEventParameterizedToken(token, prefix)
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		kind, ok := suffixes[suffix]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		issue.Kind, issue.PayloadField = kind, field
		return issue, issue.validFor(eventField)
	}

	switch legacySource {
	case profilerFtraceEventDegradationEnvelope:
		kind, ok := profilerFtraceEnvelopeLegacyKinds[token]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		return set(kind)
	case profilerFtraceEventDegradationCorePayload:
		if parsed, ok := fieldIssue("core_field", map[string]profilerFtraceEventIssueKind{
			"wrong_wire":   profilerFtraceEventIssueCoreFieldWrongWire,
			"duplicate":    profilerFtraceEventIssueCoreFieldDuplicate,
			"out_of_range": profilerFtraceEventIssueCoreFieldOutOfRange,
		}); ok {
			return parsed, true
		}
		kind, ok := profilerFtraceCoreLegacyKinds[token]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		if profilerFtraceEventIssueDisplayKind(kind) {
			issue.Severity = profilerFtraceEventIssueAdmittedDisplay
		}
		return set(kind)
	case profilerFtraceEventDegradationAuxPayload:
		if parsed, ok := fieldIssue("core_field", map[string]profilerFtraceEventIssueKind{
			"wrong_wire":   profilerFtraceEventIssueAuxFieldWrongWire,
			"duplicate":    profilerFtraceEventIssueAuxFieldDuplicate,
			"out_of_range": profilerFtraceEventIssueAuxFieldOutOfRange,
		}); ok {
			return parsed, true
		}
		if field, suffix, ok := profilerFtraceEventParameterizedToken(token, "drop_response_field"); ok && suffix == "out_of_source_profile" {
			issue.Kind, issue.PayloadField = profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile, field
			issue.Severity = profilerFtraceEventIssueAdmittedDisplay
			return issue, issue.validFor(eventField)
		}
		kind, ok := profilerFtraceAuxLegacyKinds[token]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		return set(kind)
	case profilerFtraceEventDegradationFilemapPayload:
		kind, ok := profilerFtraceFilemapLegacyKinds[token]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		return set(kind)
	case profilerFtraceEventDegradationBlockPayload:
		if parsed, ok := fieldIssue("core_field", map[string]profilerFtraceEventIssueKind{
			"malformed_wire":     profilerFtraceEventIssueBlockFieldMalformedWire,
			"wrong_wire":         profilerFtraceEventIssueBlockFieldWrongWire,
			"duplicate":          profilerFtraceEventIssueBlockFieldDuplicate,
			"out_of_range":       profilerFtraceEventIssueBlockFieldOutOfRange,
			"missing_or_invalid": profilerFtraceEventIssueBlockFieldMissingOrInvalid,
		}); ok {
			return parsed, true
		}
		kind, ok := profilerFtraceBlockDisplayLegacyKinds[token]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		issue.Severity = profilerFtraceEventIssueAdmittedDisplay
		return set(kind)
	case profilerFtraceEventDegradationWireAudit:
		if parsed, ok := fieldIssue("core_field", map[string]profilerFtraceEventIssueKind{
			"malformed_wire":     profilerFtraceEventIssueWireFieldMalformedWire,
			"wrong_wire":         profilerFtraceEventIssueWireFieldWrongWire,
			"duplicate":          profilerFtraceEventIssueWireFieldDuplicate,
			"out_of_range":       profilerFtraceEventIssueWireFieldOutOfRange,
			"missing_or_invalid": profilerFtraceEventIssueWireFieldMissingOrInvalid,
		}); ok {
			return parsed, true
		}
		kind, ok := profilerFtraceWireDisplayLegacyKinds[token]
		if !ok {
			return profilerFtraceEventIssue{}, false
		}
		issue.Severity = profilerFtraceEventIssueAdmittedDisplay
		return set(kind)
	case profilerFtraceEventDegradationUnmappedField:
		if token != "unmapped structured ftrace event field" {
			return profilerFtraceEventIssue{}, false
		}
		return set(profilerFtraceEventIssueUnmappedField)
	default:
		// Final precise source classes are output-only at this migration bridge.
		return profilerFtraceEventIssue{}, false
	}
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

func profilerFtraceEventParameterizedToken(token, prefix string) (uint8, string, bool) {
	if !strings.HasPrefix(token, prefix) {
		return 0, "", false
	}
	rest := strings.TrimPrefix(token, prefix)
	separator := strings.IndexByte(rest, '_')
	if separator <= 0 {
		return 0, "", false
	}
	digits, suffix := rest[:separator], rest[separator+1:]
	value, err := strconv.ParseUint(digits, 10, 8)
	if err != nil || value == 0 || strconv.FormatUint(value, 10) != digits || suffix == "" {
		return 0, "", false
	}
	return uint8(value), suffix, true
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
		profilerFtraceEventIssueAuxInvalidF2FSPayloadRange,
		profilerFtraceEventIssueAuxInvalidCanonicalLine,
		profilerFtraceEventIssueFilemapInvalidCanonicalLine,
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
	case issue.Kind >= profilerFtraceEventIssueFilemapPFNInvalid && issue.Kind <= profilerFtraceEventIssueFilemapInvalidCanonicalLine:
		return eventField == 1000 || eventField == 1001
	case issue.Kind >= profilerFtraceEventIssueBlockFieldMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		return issue.validBlock(eventField)
	case issue.Kind >= profilerFtraceEventIssueWireFieldMalformedWire && issue.Kind <= profilerFtraceEventIssueWireNextInfoDuplicate:
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
		return true
	}
}

func (issue profilerFtraceEventIssue) validAux(eventField int) bool {
	schema := profilerStructuredAuxSchemas[eventField]
	if schema == nil {
		return false
	}
	switch issue.Kind {
	case profilerFtraceEventIssueAuxFieldWrongWire, profilerFtraceEventIssueAuxFieldDuplicate,
		profilerFtraceEventIssueAuxFieldOutOfRange:
		_, known := schema[int(issue.PayloadField)]
		if !known {
			return false
		}
		if issue.Kind != profilerFtraceEventIssueAuxFieldOutOfRange {
			return true
		}
		return profilerFtraceAuxRangeFieldKnown(eventField, int(issue.PayloadField))
	case profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile:
		return eventField == 4015 && (issue.PayloadField == 3 || issue.PayloadField == 7 || issue.PayloadField == 11)
	default:
	}
	switch issue.Kind {
	case profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf:
		return eventField == 1109
	case profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev, profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno,
		profilerFtraceEventIssueAuxInvalidF2FSPayloadRange:
		return eventField >= 4009 && eventField <= 4012
	case profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer, profilerFtraceEventIssueAuxMissingOrInvalidMMCName:
		return eventField == 4015 || eventField == 4016
	default:
		return true
	}
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
		return eventField == 204 || eventField == 210 || eventField == 211
	case profilerFtraceEventIssueBlockCmdMalformedWire, profilerFtraceEventIssueBlockCmdWrongWire,
		profilerFtraceEventIssueBlockCmdDuplicate, profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		return eventField == 209 || eventField == 210 || eventField == 211
	default:
		return false
	}
}

func profilerFtraceBlockPayloadFieldKnown(eventField, payloadField int) bool {
	max := 0
	switch eventField {
	case 202:
		max = 5
	case 204:
		max = 4
	case 205:
		max = 6
	case 209, 210, 211:
		max = 5
	case 212:
		max = 7
	}
	return payloadField >= 1 && payloadField <= max
}

func profilerFtraceBlockRWBSField(eventField, payloadField int) bool {
	switch eventField {
	case 202, 209, 210, 211:
		return payloadField == 5
	case 204:
		return payloadField == 4
	case 205:
		return payloadField == 6
	case 212:
		return payloadField == 7
	default:
		return false
	}
}

func (issue profilerFtraceEventIssue) validWire(eventField int) bool {
	switch issue.Kind {
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
		if issue.Kind == profilerFtraceEventIssueAuxDropResponseOutOfSourceProfile {
			return profilerFtraceEventDegradationAuxDisplay
		}
		return profilerFtraceEventDegradationAuxPayload
	case issue.Kind >= profilerFtraceEventIssueFilemapPFNInvalid && issue.Kind <= profilerFtraceEventIssueFilemapInvalidCanonicalLine:
		return profilerFtraceEventDegradationFilemapPayload
	case issue.Kind >= profilerFtraceEventIssueBlockFieldMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockFieldMissingOrInvalid:
		return profilerFtraceEventDegradationBlockPayload
	case issue.Kind >= profilerFtraceEventIssueBlockCommMalformedWire && issue.Kind <= profilerFtraceEventIssueBlockCmdUnsafeOmitted:
		return profilerFtraceEventDegradationBlockDisplay
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
		profilerFtraceEventIssueBlockFieldWrongWire, profilerFtraceEventIssueWireFieldWrongWire:
		return "core_field" + field + "_wrong_wire", true
	case profilerFtraceEventIssueCoreFieldDuplicate, profilerFtraceEventIssueAuxFieldDuplicate,
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

var profilerFtraceEnvelopeLegacyKinds = map[string]profilerFtraceEventIssueKind{
	"envelope_event_malformed_wire":              profilerFtraceEventIssueEnvelopeEventMalformedWire,
	"envelope_trace_plugin_malformed_wire":       profilerFtraceEventIssueEnvelopeTracePluginMalformedWire,
	"envelope_cpu_detail_malformed_wire":         profilerFtraceEventIssueEnvelopeCPUDetailMalformedWire,
	"envelope_event_container_wrong_wire":        profilerFtraceEventIssueEnvelopeEventContainerWrongWire,
	"envelope_overwrite_invalid":                 profilerFtraceEventIssueEnvelopeOverwriteInvalid,
	"envelope_cpu_duplicate":                     profilerFtraceEventIssueEnvelopeCPUDuplicate,
	"envelope_cpu_wrong_wire":                    profilerFtraceEventIssueEnvelopeCPUWrongWire,
	"envelope_cpu_out_of_range":                  profilerFtraceEventIssueEnvelopeCPUOutOfRange,
	"envelope_timestamp_duplicate":               profilerFtraceEventIssueEnvelopeTimestampDuplicate,
	"envelope_timestamp_wrong_wire":              profilerFtraceEventIssueEnvelopeTimestampWrongWire,
	"envelope_timestamp_out_of_range":            profilerFtraceEventIssueEnvelopeTimestampOutOfRange,
	"envelope_tgid_duplicate":                    profilerFtraceEventIssueEnvelopeTGIDDuplicate,
	"envelope_tgid_wrong_wire":                   profilerFtraceEventIssueEnvelopeTGIDWrongWire,
	"envelope_tgid_out_of_range":                 profilerFtraceEventIssueEnvelopeTGIDOutOfRange,
	"envelope_comm_duplicate":                    profilerFtraceEventIssueEnvelopeCommDuplicate,
	"envelope_comm_wrong_wire":                   profilerFtraceEventIssueEnvelopeCommWrongWire,
	"envelope_comm_invalid":                      profilerFtraceEventIssueEnvelopeCommInvalid,
	"envelope_common_fields_missing":             profilerFtraceEventIssueEnvelopeCommonFieldsMissing,
	"envelope_common_fields_duplicate":           profilerFtraceEventIssueEnvelopeCommonFieldsDuplicate,
	"envelope_common_fields_wrong_wire":          profilerFtraceEventIssueEnvelopeCommonFieldsWrongWire,
	"envelope_common_fields_malformed_wire":      profilerFtraceEventIssueEnvelopeCommonFieldsMalformedWire,
	"envelope_common_type_duplicate":             profilerFtraceEventIssueEnvelopeCommonTypeDuplicate,
	"envelope_common_type_wrong_wire":            profilerFtraceEventIssueEnvelopeCommonTypeWrongWire,
	"envelope_common_type_source_width":          profilerFtraceEventIssueEnvelopeCommonTypeSourceWidth,
	"envelope_common_flags_duplicate":            profilerFtraceEventIssueEnvelopeCommonFlagsDuplicate,
	"envelope_common_flags_wrong_wire":           profilerFtraceEventIssueEnvelopeCommonFlagsWrongWire,
	"envelope_common_flags_source_width":         profilerFtraceEventIssueEnvelopeCommonFlagsSourceWidth,
	"envelope_common_preempt_count_duplicate":    profilerFtraceEventIssueEnvelopeCommonPreemptCountDuplicate,
	"envelope_common_preempt_count_wrong_wire":   profilerFtraceEventIssueEnvelopeCommonPreemptCountWrongWire,
	"envelope_common_preempt_count_source_width": profilerFtraceEventIssueEnvelopeCommonPreemptCountSourceWidth,
	"envelope_common_pid_duplicate":              profilerFtraceEventIssueEnvelopeCommonPIDDuplicate,
	"envelope_common_pid_wrong_wire":             profilerFtraceEventIssueEnvelopeCommonPIDWrongWire,
	"envelope_common_pid_out_of_range":           profilerFtraceEventIssueEnvelopeCommonPIDOutOfRange,
	"envelope_oneof_missing":                     profilerFtraceEventIssueEnvelopeOneofMissing,
	"envelope_oneof_multiple":                    profilerFtraceEventIssueEnvelopeOneofMultiple,
	"envelope_oneof_wrong_wire":                  profilerFtraceEventIssueEnvelopeOneofWrongWire,
	"envelope_identity_incomplete":               profilerFtraceEventIssueEnvelopeIdentityIncomplete,
}

var profilerFtraceCoreLegacyKinds = map[string]profilerFtraceEventIssueKind{
	"core_payload_malformed_wire":   profilerFtraceEventIssueCorePayloadMalformedWire,
	"invalid_transaction_id":        profilerFtraceEventIssueCoreInvalidTransactionID,
	"invalid_transaction_endpoint":  profilerFtraceEventIssueCoreInvalidTransactionEndpoint,
	"invalid_reply":                 profilerFtraceEventIssueCoreInvalidReply,
	"missing_or_invalid_reason":     profilerFtraceEventIssueCoreMissingOrInvalidReason,
	"missing_or_invalid_irq":        profilerFtraceEventIssueCoreMissingOrInvalidIRQ,
	"missing_or_invalid_irq_name":   profilerFtraceEventIssueCoreMissingOrInvalidIRQName,
	"missing_or_invalid_ret":        profilerFtraceEventIssueCoreMissingOrInvalidRet,
	"missing_or_invalid_vec":        profilerFtraceEventIssueCoreMissingOrInvalidVec,
	"missing_or_invalid_state":      profilerFtraceEventIssueCoreMissingOrInvalidState,
	"missing_or_invalid_cpu_id":     profilerFtraceEventIssueCoreMissingOrInvalidCPUID,
	"invalid_limits_profile":        profilerFtraceEventIssueCoreInvalidLimitsProfile,
	"invalid_limits_order":          profilerFtraceEventIssueCoreInvalidLimitsOrder,
	"missing_or_invalid_pid":        profilerFtraceEventIssueCoreMissingOrInvalidPID,
	"missing_or_invalid_priority":   profilerFtraceEventIssueCoreMissingOrInvalidPriority,
	"missing_or_invalid_target_cpu": profilerFtraceEventIssueCoreMissingOrInvalidTargetCPU,
	"missing_or_invalid_iowait":     profilerFtraceEventIssueCoreMissingOrInvalidIOWait,
	"display_comm_wrong_wire":       profilerFtraceEventIssueCoreDisplayCommWrongWire,
	"display_comm_duplicate":        profilerFtraceEventIssueCoreDisplayCommDuplicate,
	"display_comm_invalid":          profilerFtraceEventIssueCoreDisplayCommInvalid,
	"display_comm_unavailable":      profilerFtraceEventIssueCoreDisplayCommUnavailable,
	"display_comm_out_of_profile":   profilerFtraceEventIssueCoreDisplayCommOutOfProfile,
	"display_caller_str_wrong_wire": profilerFtraceEventIssueCoreDisplayCallerStrWrongWire,
	"display_caller_str_duplicate":  profilerFtraceEventIssueCoreDisplayCallerStrDuplicate,
	"display_caller_str_invalid":    profilerFtraceEventIssueCoreDisplayCallerStrInvalid,
	"invalid_canonical_core_line":   profilerFtraceEventIssueCoreInvalidCanonicalLine,
}

var profilerFtraceAuxLegacyKinds = map[string]profilerFtraceEventIssueKind{
	"aux_payload_malformed_wire":     profilerFtraceEventIssueAuxPayloadMalformedWire,
	"missing_or_invalid_print_buf":   profilerFtraceEventIssueAuxMissingOrInvalidPrintBuf,
	"missing_or_invalid_f2fs_dev":    profilerFtraceEventIssueAuxMissingOrInvalidF2FSDev,
	"missing_or_invalid_f2fs_ino":    profilerFtraceEventIssueAuxMissingOrInvalidF2FSIno,
	"invalid_f2fs_payload_range":     profilerFtraceEventIssueAuxInvalidF2FSPayloadRange,
	"missing_or_invalid_mmc_pointer": profilerFtraceEventIssueAuxMissingOrInvalidMMCPointer,
	"missing_or_invalid_mmc_name":    profilerFtraceEventIssueAuxMissingOrInvalidMMCName,
	"invalid_canonical_aux_line":     profilerFtraceEventIssueAuxInvalidCanonicalLine,
}

var profilerFtraceFilemapLegacyKinds = map[string]profilerFtraceEventIssueKind{
	"filemap_pfn_invalid":            profilerFtraceEventIssueFilemapPFNInvalid,
	"filemap_inode_invalid":          profilerFtraceEventIssueFilemapInodeInvalid,
	"filemap_index_invalid":          profilerFtraceEventIssueFilemapIndexInvalid,
	"filemap_device_invalid":         profilerFtraceEventIssueFilemapDeviceInvalid,
	"filemap_order_invalid":          profilerFtraceEventIssueFilemapOrderInvalid,
	"invalid_canonical_filemap_line": profilerFtraceEventIssueFilemapInvalidCanonicalLine,
}

var profilerFtraceBlockDisplayLegacyKinds = map[string]profilerFtraceEventIssueKind{
	"comm_malformed_wire": profilerFtraceEventIssueBlockCommMalformedWire,
	"comm_wrong_wire":     profilerFtraceEventIssueBlockCommWrongWire,
	"comm_duplicate":      profilerFtraceEventIssueBlockCommDuplicate,
	"comm_unsafe_omitted": profilerFtraceEventIssueBlockCommUnsafeOmitted,
	"cmd_malformed_wire":  profilerFtraceEventIssueBlockCmdMalformedWire,
	"cmd_wrong_wire":      profilerFtraceEventIssueBlockCmdWrongWire,
	"cmd_duplicate":       profilerFtraceEventIssueBlockCmdDuplicate,
	"cmd_unsafe_omitted":  profilerFtraceEventIssueBlockCmdUnsafeOmitted,
}

var profilerFtraceWireDisplayLegacyKinds = map[string]profilerFtraceEventIssueKind{
	"cpu_id_malformed_wire":    profilerFtraceEventIssueWireCPUIDMalformedWire,
	"cpu_id_wrong_wire":        profilerFtraceEventIssueWireCPUIDWrongWire,
	"cpu_id_duplicate":         profilerFtraceEventIssueWireCPUIDDuplicate,
	"cpu_id_out_of_range":      profilerFtraceEventIssueWireCPUIDOutOfRange,
	"next_info_malformed_wire": profilerFtraceEventIssueWireNextInfoMalformedWire,
	"next_info_wrong_wire":     profilerFtraceEventIssueWireNextInfoWrongWire,
	"next_info_duplicate":      profilerFtraceEventIssueWireNextInfoDuplicate,
}

var profilerFtraceEventFixedIssueLabels = func() map[profilerFtraceEventIssueKind]string {
	out := make(map[profilerFtraceEventIssueKind]string,
		len(profilerFtraceEnvelopeLegacyKinds)+len(profilerFtraceCoreLegacyKinds)+len(profilerFtraceAuxLegacyKinds)+
			len(profilerFtraceFilemapLegacyKinds)+len(profilerFtraceBlockDisplayLegacyKinds)+len(profilerFtraceWireDisplayLegacyKinds)+1)
	for label, kind := range profilerFtraceEnvelopeLegacyKinds {
		out[kind] = label
	}
	for label, kind := range profilerFtraceCoreLegacyKinds {
		out[kind] = label
	}
	for label, kind := range profilerFtraceAuxLegacyKinds {
		out[kind] = label
	}
	for label, kind := range profilerFtraceFilemapLegacyKinds {
		out[kind] = label
	}
	for label, kind := range profilerFtraceBlockDisplayLegacyKinds {
		out[kind] = label
	}
	for label, kind := range profilerFtraceWireDisplayLegacyKinds {
		out[kind] = label
	}
	out[profilerFtraceEventIssueUnmappedField] = "unmapped structured ftrace event field"
	return out
}()
