package hitraceconv

import (
	"fmt"
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
	Kind         profilerAuxKind
	Name         string
	Degradations []string
	Print        *markerPayload
	F2FS         *profilerF2FSPayload
	MMCStart     *profilerMMCStartPayload
	MMCDone      *profilerMMCDonePayload
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

func decodeProfilerAuxPayload(event profilerFtraceEventRecord) (profilerAuxPayload, bodyAdmission, string) {
	payload, admission, reason, _ := decodeProfilerAuxPayloadWithNormalizer(event, func(buffer []byte) (string, bool) {
		return normalizeMarkerBuffer(buffer)
	})
	return payload, admission, reason
}

func decodeProfilerAuxPayloadWithPairAdmission(event profilerFtraceEventRecord) (profilerAuxPayload, bodyAdmission, string, profilerPairAdmission) {
	return decodeProfilerAuxPayloadWithNormalizer(event, normalizeMarkerBuffer)
}

func decodeProfilerAuxPayloadWithNormalizer(event profilerFtraceEventRecord, normalizeMarker func([]byte) (string, bool)) (profilerAuxPayload, bodyAdmission, string, profilerPairAdmission) {
	pair := profilerStructuredF2FSPairFamily(event.Field)
	schema, governed := profilerStructuredAuxSchemas[event.Field]
	if !governed {
		return profilerAuxPayload{}, bodyUnsupported, "", pair
	}
	descriptor, ok := profilerFtraceEventDescriptors[event.Field]
	if !ok {
		return profilerAuxPayload{}, bodyRejected, "missing_aux_descriptor", pair
	}
	if descriptor.Field != event.Field {
		return profilerAuxPayload{}, bodyRejected, "mismatched_aux_descriptor_field", pair
	}
	expectedName, ok := profilerAuxDescriptorName(event.Field)
	if !ok || descriptor.Name != expectedName {
		return profilerAuxPayload{}, bodyRejected, "mismatched_aux_descriptor_name", pair
	}
	fields, reason := decodeProfilerCoreProtoFields(event.Payload, schema)
	if reason != "" {
		if reason == "core_payload_malformed_wire" {
			reason = "aux_payload_malformed_wire"
		}
		return profilerAuxPayload{}, bodyRejected, reason, pair
	}
	if pair.Governed {
		pair = profilerStructuredF2FSPairIdentity(event, descriptor.Name, fields)
	}
	maxField := 0
	for field := range schema {
		if field > maxField {
			maxField = field
		}
	}
	for field := 1; field <= maxField; field++ {
		if _, expected := schema[field]; !expected {
			continue
		}
		if reason := profilerCoreFieldWireReason(fields[field], field); reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
	}

	payload := profilerAuxPayload{Name: descriptor.Name}
	switch event.Field {
	case 1109:
		buffer, ok := normalizeMarker(fields[2].bytesValue)
		if !ok {
			return profilerAuxPayload{}, bodyRejected, "missing_or_invalid_print_buf", pair
		}
		payload.Kind = profilerAuxPrint
		payload.Print = &markerPayload{
			IP: fields[1].uintValue, IPPresent: fields[1].count == 1, Buffer: buffer,
		}
	case 4009:
		item, reason := decodeProfilerF2FSSyncEnter(fields)
		if reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
		item.Name = descriptor.Name
		finalizeF2FSPayloadAdmission(&item)
		if !item.IdentityKnown || !item.PayloadAdmitted {
			return profilerAuxPayload{}, bodyRejected, "invalid_f2fs_payload_range", pair
		}
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4010:
		item, reason := decodeProfilerF2FSSyncExit(fields)
		if reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
		item.Name = descriptor.Name
		finalizeF2FSPayloadAdmission(&item)
		if !item.IdentityKnown || !item.PayloadAdmitted {
			return profilerAuxPayload{}, bodyRejected, "invalid_f2fs_payload_range", pair
		}
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4011:
		item, reason := decodeProfilerF2FSWriteBegin(fields)
		if reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
		item.Name = descriptor.Name
		finalizeF2FSPayloadAdmission(&item)
		if !item.IdentityKnown || !item.PayloadAdmitted {
			return profilerAuxPayload{}, bodyRejected, "invalid_f2fs_payload_range", pair
		}
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4012:
		item, reason := decodeProfilerF2FSWriteEnd(fields)
		if reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
		item.Name = descriptor.Name
		finalizeF2FSPayloadAdmission(&item)
		if !item.IdentityKnown || !item.PayloadAdmitted {
			return profilerAuxPayload{}, bodyRejected, "invalid_f2fs_payload_range", pair
		}
		payload.Kind, payload.F2FS = profilerAuxF2FS, &item
	case 4015:
		for _, field := range []int{3, 7, 11} {
			// The pinned raw descriptor is u32[4]. The producer incorrectly
			// routes those 16 bytes through ParseStrField, so content is not a
			// trustworthy string or four recoverable words; only its source
			// footprint is authoritative.
			if len(fields[field].bytesValue) > maxProfilerMMCResponseBytes {
				payload.Degradations = append(payload.Degradations,
					fmt.Sprintf("drop_response_field%d_out_of_source_profile", field))
			}
		}
		item, reason := decodeProfilerMMCDone(fields)
		if reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
		payload.Kind, payload.MMCDone = profilerAuxMMCDone, &item
	case 4016:
		item, reason := decodeProfilerMMCStart(fields)
		if reason != "" {
			return profilerAuxPayload{}, bodyRejected, reason, pair
		}
		payload.Kind, payload.MMCStart = profilerAuxMMCStart, &item
	default:
		return profilerAuxPayload{}, bodyRejected, "unhandled_aux_descriptor", pair
	}
	pair.Admitted = !pair.Governed || pair.LaneKnown
	return payload, bodyAdmitted, "", pair
}

func profilerStructuredF2FSPairFamily(field int) profilerPairAdmission {
	if field >= 4009 && field <= 4012 {
		return profilerPairAdmission{Kind: pairRenderF2FS, Governed: true}
	}
	return profilerPairAdmission{}
}

// profilerStructuredF2FSPairIdentity recovers only the exact hard key from the
// already schema-audited protobuf fields. Non-key field failures retain this
// verdict so they quarantine one dev/inode/op/emitter lane; malformed key wire
// or owner identity leaves LaneKnown false and therefore closes the family.
func profilerStructuredF2FSPairIdentity(event profilerFtraceEventRecord, name string, fields map[int]profilerCoreProtoField) profilerPairAdmission {
	out := profilerStructuredF2FSPairFamily(event.Field)
	if !out.Governed || profilerCoreFieldWireReason(fields[1], 1) != "" ||
		profilerCoreFieldWireReason(fields[2], 2) != "" {
		return out
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
	dev, ino, reason := profilerAuxF2FSIdentity(fields)
	if reason != "" {
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

func profilerAuxUint32(fields map[int]profilerCoreProtoField, field int) (uint32, bool) {
	value, valid := profilerCoreUint32(fields[field])
	return uint32(value), valid
}

func profilerAuxDecodedInt32(fields map[int]profilerCoreProtoField, field int) (int64, bool) {
	return profilerCoreInt32(fields[field])
}

func profilerAuxF2FSIdentity(fields map[int]profilerCoreProtoField) (uint32, uint64, string) {
	rawDev := profilerCoreUint64(fields[1])
	if rawDev == 0 {
		return 0, 0, "missing_or_invalid_f2fs_dev"
	}
	if rawDev > math.MaxUint32 {
		return 0, 0, profilerAuxRangeReason(1)
	}
	ino := profilerCoreUint64(fields[2])
	if ino == 0 {
		return 0, 0, "missing_or_invalid_f2fs_ino"
	}
	return uint32(rawDev), ino, ""
}

func profilerAuxRangeReason(field int) string {
	return fmt.Sprintf("core_field%d_out_of_range", field)
}

func decodeProfilerF2FSSyncEnter(fields map[int]profilerCoreProtoField) (profilerF2FSPayload, string) {
	dev, ino, reason := profilerAuxF2FSIdentity(fields)
	if reason != "" {
		return profilerF2FSPayload{}, reason
	}
	valid := false
	mode, valid := profilerAuxUint32(fields, 4)
	if !valid || mode > math.MaxUint16 {
		return profilerF2FSPayload{}, profilerAuxRangeReason(4)
	}
	nlink, valid := profilerAuxUint32(fields, 6)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(6)
	}
	advise, valid := profilerAuxUint32(fields, 8)
	if !valid || advise > math.MaxUint8 {
		return profilerF2FSPayload{}, profilerAuxRangeReason(8)
	}
	size := profilerCoreUint64(fields[5])
	if size > math.MaxInt64 {
		return profilerF2FSPayload{}, profilerAuxRangeReason(5)
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSSyncEnter, Dev: dev, Ino: ino,
		Pino: profilerCoreUint64(fields[3]), Mode: mode, Size: int64(size),
		Nlink: nlink, Blocks: profilerCoreUint64(fields[7]), Advise: advise,
	}, ""
}

func decodeProfilerF2FSSyncExit(fields map[int]profilerCoreProtoField) (profilerF2FSPayload, string) {
	dev, ino, reason := profilerAuxF2FSIdentity(fields)
	if reason != "" {
		return profilerF2FSPayload{}, reason
	}
	valid := false
	cpReason, valid := profilerAuxDecodedInt32(fields, 3)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(3)
	}
	datasync, valid := profilerAuxDecodedInt32(fields, 4)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(4)
	}
	ret, valid := profilerAuxDecodedInt32(fields, 5)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(5)
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSSyncExit, Dev: dev, Ino: ino,
		CPReason: cpReason, DataSync: datasync, Ret: ret,
	}, ""
}

func decodeProfilerF2FSWriteBegin(fields map[int]profilerCoreProtoField) (profilerF2FSPayload, string) {
	dev, ino, reason := profilerAuxF2FSIdentity(fields)
	if reason != "" {
		return profilerF2FSPayload{}, reason
	}
	valid := false
	length, valid := profilerAuxUint32(fields, 4)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(4)
	}
	flags := uint32(0)
	flagsPresent := fields[5].count == 1
	if flagsPresent {
		flags, valid = profilerAuxUint32(fields, 5)
		if !valid {
			return profilerF2FSPayload{}, profilerAuxRangeReason(5)
		}
	}
	pos := profilerCoreUint64(fields[3])
	if pos > math.MaxInt64 {
		return profilerF2FSPayload{}, profilerAuxRangeReason(3)
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSWriteBegin, Dev: dev, Ino: ino,
		Pos: pos, Len: uint64(length), Flags: flags, FlagsPresent: flagsPresent,
	}, ""
}

func decodeProfilerF2FSWriteEnd(fields map[int]profilerCoreProtoField) (profilerF2FSPayload, string) {
	dev, ino, reason := profilerAuxF2FSIdentity(fields)
	if reason != "" {
		return profilerF2FSPayload{}, reason
	}
	valid := false
	length, valid := profilerAuxUint32(fields, 4)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(4)
	}
	copied, valid := profilerAuxUint32(fields, 5)
	if !valid {
		return profilerF2FSPayload{}, profilerAuxRangeReason(5)
	}
	pos := profilerCoreUint64(fields[3])
	if pos > math.MaxInt64 {
		return profilerF2FSPayload{}, profilerAuxRangeReason(3)
	}
	return profilerF2FSPayload{
		Kind: profilerF2FSWriteEnd, Dev: dev, Ino: ino,
		Pos: pos, Len: uint64(length), Copied: copied,
	}, ""
}

func decodeProfilerMMCStart(fields map[int]profilerCoreProtoField) (profilerMMCStartPayload, string) {
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
			return profilerMMCStartPayload{}, profilerAuxRangeReason(target.field)
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
			return profilerMMCStartPayload{}, profilerAuxRangeReason(target.field)
		}
		*target.out = value
	}
	item.MRQ = profilerCoreUint64(fields[24])
	if item.MRQ == 0 {
		return profilerMMCStartPayload{}, "missing_or_invalid_mmc_pointer"
	}
	name, valid := profilerCoreString(fields[25])
	if !valid || !validProfilerMMCName(name) {
		return profilerMMCStartPayload{}, "missing_or_invalid_mmc_name"
	}
	item.Name = name
	return item, ""
}

func decodeProfilerMMCDone(fields map[int]profilerCoreProtoField) (profilerMMCDonePayload, string) {
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
			return profilerMMCDonePayload{}, profilerAuxRangeReason(target.field)
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
			return profilerMMCDonePayload{}, profilerAuxRangeReason(target.field)
		}
		*target.out = value
	}
	item.MRQ = profilerCoreUint64(fields[22])
	if item.MRQ == 0 {
		return profilerMMCDonePayload{}, "missing_or_invalid_mmc_pointer"
	}
	name, valid := profilerCoreString(fields[23])
	if !valid || !validProfilerMMCName(name) {
		return profilerMMCDonePayload{}, "missing_or_invalid_mmc_name"
	}
	item.Name = name
	return item, ""
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
