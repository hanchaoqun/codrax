package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// f2fsPayload is the source-neutral authority shared by direct RMQ and the
// structured 4009..4012 adapter.  Fields that do not exist in one producer
// profile keep an explicit presence bit; they are never materialized as zero.
type f2fsPayloadKind uint8

const (
	f2fsPayloadSyncEnter f2fsPayloadKind = iota + 1
	f2fsPayloadSyncExit
	f2fsPayloadDirectIOEnter
	f2fsPayloadDirectIOExit
	f2fsPayloadWriteBegin
	f2fsPayloadWriteEnd
)

type f2fsPayload struct {
	Kind             f2fsPayloadKind
	Name             string
	HeaderTID        int64
	HeaderOwnerKnown bool

	Dev  uint32
	Ino  uint64
	Pino uint64
	Mode uint32
	Size int64

	Nlink  uint32
	Blocks uint64
	Advise uint32

	CPReason int64
	DataSync int64
	Ret      int64

	Pos uint64
	Len uint64
	RW  int64

	KIFieldsPresent bool
	KIFlags         int64
	KIIoprio        uint32
	FlagsPresent    bool
	Flags           uint32
	Copied          uint32

	IdentityKnown   bool
	PayloadAdmitted bool
}

type directF2FSProfile uint8

const (
	directF2FSProfileSyncEnter directF2FSProfile = iota + 1
	directF2FSProfileSyncExit
	directF2FSProfileDirectIOEnter510
	directF2FSProfileDirectIOEnter66
	directF2FSProfileDirectIOExit
	directF2FSProfileWriteBegin510
	directF2FSProfileWriteBegin66
	directF2FSProfileWriteEnd
)

type directF2FSFieldSpec struct {
	Name   string
	Type   string
	Offset int
	Size   int
	Signed bool
}

func directF2FSNameGoverned(name string) bool {
	_, ok := directF2FSKindForName(name)
	return ok
}

// directF2FSNameCandidate is a thin converter boundary over tracequery's
// source-neutral negative gate. Keep the family/phase vocabulary there: a
// second local spelling classifier would let direct conversion and its
// consumer disagree about which near names are inventory-only.
func directF2FSNameCandidate(name string) bool {
	return tracequery.F2FSClosedEndpointNameCandidate(name)
}

func directF2FSKindForName(name string) (f2fsPayloadKind, bool) {
	switch name {
	case "f2fs_sync_file_enter":
		return f2fsPayloadSyncEnter, true
	case "f2fs_sync_file_exit":
		return f2fsPayloadSyncExit, true
	case "f2fs_direct_IO_enter":
		return f2fsPayloadDirectIOEnter, true
	case "f2fs_direct_IO_exit":
		return f2fsPayloadDirectIOExit, true
	case "f2fs_write_begin":
		return f2fsPayloadWriteBegin, true
	case "f2fs_write_end":
		return f2fsPayloadWriteEnd, true
	default:
		return 0, false
	}
}

func decodeDirectF2FSPayload(ev decodedEvent) (f2fsPayload, bodyAdmission, string) {
	kind, governed := directF2FSKindForName(ev.format.Name)
	if !governed {
		return f2fsPayload{}, bodyUnsupported, ""
	}
	headerTID, headerOwnerKnown := directPairHeaderTID(ev)
	base := f2fsPayload{
		Kind: kind, Name: ev.format.Name, HeaderTID: headerTID,
		HeaderOwnerKnown: headerOwnerKnown,
	}
	partial := base
	partialKnown := false
	partialAmbiguous := false
	for _, profile := range directF2FSProfilesForName(ev.format.Name) {
		if !directF2FSPrintFmtMatches(profile, ev.format.PrintFmt) {
			continue
		}
		for _, wordSize := range []int{4, 8} {
			specs := directF2FSSpecs(profile, wordSize)
			if candidate, ok := directF2FSPartialIdentity(ev, specs, base); ok {
				if !partialKnown {
					partial, partialKnown = candidate, true
				} else if partial.Dev != candidate.Dev || partial.Ino != candidate.Ino || partial.RW != candidate.RW {
					partialAmbiguous = true
				}
			}
			fields, ok := directF2FSExactFields(ev, specs)
			if !ok {
				continue
			}
			payload := base
			payload.Dev = uint32(directF2FSUint(fields["dev"]))
			payload.Ino = directF2FSUint(fields["ino"])
			switch profile {
			case directF2FSProfileSyncEnter:
				payload.Pino = directF2FSUint(fields["pino"])
				payload.Mode = uint32(directF2FSUint(fields["mode"]))
				payload.Size = directF2FSInt(fields["size"])
				payload.Nlink = uint32(directF2FSUint(fields["nlink"]))
				payload.Blocks = directF2FSUint(fields["blocks"])
				payload.Advise = uint32(directF2FSUint(fields["advise"]))
			case directF2FSProfileSyncExit:
				payload.CPReason = directF2FSInt(fields["cp_reason"])
				payload.DataSync = directF2FSInt(fields["datasync"])
				payload.Ret = directF2FSInt(fields["ret"])
			case directF2FSProfileDirectIOEnter510:
				payload.Pos = uint64(directF2FSInt(fields["pos"]))
				payload.Len = directF2FSUint(fields["len"])
				payload.RW = directF2FSInt(fields["rw"])
			case directF2FSProfileDirectIOEnter66:
				payload.Pos = uint64(directF2FSInt(fields["ki_pos"]))
				payload.Len = directF2FSUint(fields["len"])
				payload.RW = directF2FSInt(fields["rw"])
				payload.KIFieldsPresent = true
				payload.KIFlags = directF2FSInt(fields["ki_flags"])
				payload.KIIoprio = uint32(directF2FSUint(fields["ki_ioprio"]))
			case directF2FSProfileDirectIOExit:
				payload.Pos = uint64(directF2FSInt(fields["pos"]))
				payload.Len = directF2FSUint(fields["len"])
				payload.RW = directF2FSInt(fields["rw"])
				payload.Ret = directF2FSInt(fields["ret"])
			case directF2FSProfileWriteBegin510:
				payload.Pos = uint64(directF2FSInt(fields["pos"]))
				payload.Len = directF2FSUint(fields["len"])
				payload.FlagsPresent = true
				payload.Flags = uint32(directF2FSUint(fields["flags"]))
			case directF2FSProfileWriteBegin66:
				payload.Pos = uint64(directF2FSInt(fields["pos"]))
				payload.Len = directF2FSUint(fields["len"])
			case directF2FSProfileWriteEnd:
				payload.Pos = uint64(directF2FSInt(fields["pos"]))
				payload.Len = directF2FSUint(fields["len"])
				payload.Copied = uint32(directF2FSUint(fields["copied"]))
			}
			finalizeF2FSPayloadAdmission(&payload)
			if !payload.IdentityKnown {
				return payload, bodyRejected, "invalid_f2fs_identity"
			}
			if !payload.PayloadAdmitted {
				return payload, bodyRejected, "invalid_f2fs_payload_range"
			}
			return payload, bodyAdmitted, ""
		}
	}
	if partialKnown && !partialAmbiguous {
		return partial, bodyRejected, "invalid_f2fs_descriptor_profile"
	}
	return base, bodyRejected, "invalid_f2fs_descriptor_profile"
}

// directF2FSPartialIdentity recovers only the exact hard-key fields from the
// same pinned descriptor profile used by the full decoder. This is not a
// permissive payload parser: the row remains rejected, but a malformed
// non-key descriptor can quarantine its proven lane instead of unnecessarily
// closing every F2FS lane in the source. A bad/duplicate/overlapping key field
// still yields no identity and therefore fail-closes the source-local family.
func directF2FSPartialIdentity(ev decodedEvent, specs []directF2FSFieldSpec, base f2fsPayload) (f2fsPayload, bool) {
	byName := make(map[string]directF2FSFieldSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	want := []string{"dev", "ino"}
	if base.Kind == f2fsPayloadDirectIOEnter || base.Kind == f2fsPayloadDirectIOExit {
		want = append(want, "rw")
	}
	values := make(map[string][]byte, len(want))
	for _, name := range want {
		spec, present := byName[name]
		if !present {
			return base, false
		}
		matches := 0
		for index, field := range ev.format.Fields {
			if cleanFieldName(field.Name) != name {
				continue
			}
			matches++
			if !directF2FSFieldMatches(ev, index, field, spec) {
				return base, false
			}
			raw, ok := ev.fields[field.Name]
			if !ok || len(raw) != field.Size {
				return base, false
			}
			values[name] = raw
		}
		if matches != 1 {
			return base, false
		}
	}
	base.Dev = uint32(directF2FSUint(values["dev"]))
	base.Ino = directF2FSUint(values["ino"])
	if raw := values["rw"]; raw != nil {
		base.RW = directF2FSInt(raw)
	}
	base.IdentityKnown = base.Dev != 0 && base.Ino != 0
	if base.Kind == f2fsPayloadDirectIOEnter || base.Kind == f2fsPayloadDirectIOExit {
		base.IdentityKnown = base.IdentityKnown && (base.RW == 0 || base.RW == 1)
	}
	return base, base.IdentityKnown
}

func directF2FSPrintFmtMatches(profile directF2FSProfile, raw string) bool {
	literal := directF2FSPrintFmtLiteral(profile)
	value := strings.TrimSpace(raw)
	return literal != "" && (value == literal || strings.HasPrefix(value, literal+","))
}

func directF2FSPrintFmtLiteral(profile directF2FSProfile) string {
	switch profile {
	case directF2FSProfileSyncEnter:
		return `"dev = (%d,%d), ino = %lu, pino = %lu, i_mode = 0x%hx, i_size = %lld, i_nlink = %u, i_blocks = %llu, i_advise = 0x%x"`
	case directF2FSProfileSyncExit:
		return `"dev = (%d,%d), ino = %lu, cp_reason: %s, datasync = %d, ret = %d"`
	case directF2FSProfileDirectIOEnter510:
		return `"dev = (%d,%d), ino = %lu pos = %lld len = %lu rw = %d"`
	case directF2FSProfileDirectIOEnter66:
		return `"dev = (%d,%d), ino = %lu pos = %lld len = %lu ki_flags = %x ki_ioprio = %x rw = %d"`
	case directF2FSProfileDirectIOExit:
		return `"dev = (%d,%d), ino = %lu pos = %lld len = %lu rw = %d ret = %d"`
	case directF2FSProfileWriteBegin510:
		return `"dev = (%d,%d), ino = %lu, pos = %llu, len = %u, flags = %u"`
	case directF2FSProfileWriteBegin66:
		return `"dev = (%d,%d), ino = %lu, pos = %llu, len = %u"`
	case directF2FSProfileWriteEnd:
		return `"dev = (%d,%d), ino = %lu, pos = %llu, len = %u, copied = %u"`
	default:
		return ""
	}
}

func finalizeF2FSPayloadAdmission(payload *f2fsPayload) {
	if payload == nil {
		return
	}
	expectedKind, nameKnown := directF2FSKindForName(payload.Name)
	payload.IdentityKnown = nameKnown && expectedKind == payload.Kind && payload.Dev != 0 && payload.Ino != 0
	if payload.Kind == f2fsPayloadDirectIOEnter || payload.Kind == f2fsPayloadDirectIOExit {
		payload.IdentityKnown = payload.IdentityKnown && (payload.RW == 0 || payload.RW == 1)
	}
	payload.PayloadAdmitted = payload.IdentityKnown && payload.Len <= math.MaxInt64
	switch payload.Kind {
	case f2fsPayloadSyncEnter:
		payload.PayloadAdmitted = payload.PayloadAdmitted && payload.Size >= 0 && payload.Mode <= math.MaxUint16 && payload.Advise <= math.MaxUint8 &&
			!payload.KIFieldsPresent && payload.KIFlags == 0 && payload.KIIoprio == 0 && !payload.FlagsPresent && payload.Flags == 0
	case f2fsPayloadSyncExit:
		payload.PayloadAdmitted = payload.PayloadAdmitted && f2fsSignedInt32(payload.CPReason) && f2fsSignedInt32(payload.DataSync) && f2fsSignedInt32(payload.Ret) &&
			!payload.KIFieldsPresent && payload.KIFlags == 0 && payload.KIIoprio == 0 && !payload.FlagsPresent && payload.Flags == 0
	case f2fsPayloadDirectIOEnter:
		payload.PayloadAdmitted = payload.PayloadAdmitted && int64(payload.Pos) >= 0 &&
			(!payload.KIFieldsPresent && payload.KIFlags == 0 && payload.KIIoprio == 0 ||
				payload.KIFieldsPresent && f2fsSignedInt32(payload.KIFlags) && payload.KIIoprio <= math.MaxUint16) &&
			!payload.FlagsPresent && payload.Flags == 0
	case f2fsPayloadDirectIOExit:
		payload.PayloadAdmitted = payload.PayloadAdmitted && int64(payload.Pos) >= 0 && f2fsSignedInt32(payload.Ret) &&
			!payload.KIFieldsPresent && payload.KIFlags == 0 && payload.KIIoprio == 0 && !payload.FlagsPresent && payload.Flags == 0
	case f2fsPayloadWriteBegin:
		payload.PayloadAdmitted = payload.PayloadAdmitted && int64(payload.Pos) >= 0 &&
			!payload.KIFieldsPresent && payload.KIFlags == 0 && payload.KIIoprio == 0 &&
			(payload.FlagsPresent || payload.Flags == 0)
	case f2fsPayloadWriteEnd:
		payload.PayloadAdmitted = payload.PayloadAdmitted && int64(payload.Pos) >= 0 &&
			!payload.KIFieldsPresent && payload.KIFlags == 0 && payload.KIIoprio == 0 && !payload.FlagsPresent && payload.Flags == 0
	default:
		// A future kind must earn an explicit source-width and presence policy
		// before admission.
		payload.PayloadAdmitted = false
	}
}

func f2fsSignedInt32(value int64) bool {
	return value >= math.MinInt32 && value <= math.MaxInt32
}

func renderCanonicalF2FSPayload(payload f2fsPayload) (string, bool) {
	if !payload.PayloadAdmitted || !payload.IdentityKnown {
		return "", false
	}
	dev := devMajorMinor(int64(payload.Dev), ":")
	switch payload.Kind {
	case f2fsPayloadSyncEnter:
		return fmt.Sprintf("dev=%s ino=0x%x pino=0x%x i_mode=0x%x i_size=%d i_nlink=%d i_blocks=%d i_advise=0x%x",
			dev, payload.Ino, payload.Pino, payload.Mode, payload.Size, payload.Nlink, payload.Blocks, payload.Advise), true
	case f2fsPayloadSyncExit:
		return fmt.Sprintf("dev=%s ino=0x%x cp_reason=%d datasync=%d ret=%d",
			dev, payload.Ino, payload.CPReason, payload.DataSync, payload.Ret), true
	case f2fsPayloadDirectIOEnter:
		body := fmt.Sprintf("dev=%s ino=0x%x pos=%d len=%d", dev, payload.Ino, int64(payload.Pos), payload.Len)
		if payload.KIFieldsPresent {
			body += fmt.Sprintf(" ki_flags=0x%x ki_ioprio=0x%x", uint32(payload.KIFlags), payload.KIIoprio)
		}
		return body + " rw=" + f2fsRW(payload.RW), true
	case f2fsPayloadDirectIOExit:
		return fmt.Sprintf("dev=%s ino=0x%x pos=%d len=%d rw=%s ret=%d",
			dev, payload.Ino, int64(payload.Pos), payload.Len, f2fsRW(payload.RW), payload.Ret), true
	case f2fsPayloadWriteBegin:
		body := fmt.Sprintf("dev=%s ino=0x%x pos=%d len=%d", dev, payload.Ino, int64(payload.Pos), payload.Len)
		if payload.FlagsPresent {
			body += fmt.Sprintf(" flags=%d", payload.Flags)
		}
		return body, true
	case f2fsPayloadWriteEnd:
		return fmt.Sprintf("dev=%s ino=0x%x pos=%d len=%d copied=%d",
			dev, payload.Ino, int64(payload.Pos), payload.Len, payload.Copied), true
	default:
		return "", false
	}
}

func f2fsRW(value int64) string {
	if value == 0 {
		return "read"
	}
	if value == 1 {
		return "write"
	}
	return ""
}

func f2fsPayloadOperation(payload f2fsPayload) string {
	switch payload.Kind {
	case f2fsPayloadSyncEnter, f2fsPayloadSyncExit:
		return "sync"
	case f2fsPayloadWriteBegin, f2fsPayloadWriteEnd:
		return "write"
	case f2fsPayloadDirectIOEnter, f2fsPayloadDirectIOExit:
		return f2fsRW(payload.RW)
	default:
		return ""
	}
}

func f2fsPayloadTypedInput(payload f2fsPayload) tracequery.PairingEndpointTypedInput {
	return tracequery.PairingEndpointTypedInput{
		Name: payload.Name, HeaderTID: payload.HeaderTID,
		StorageIdentityKnown:         payload.IdentityKnown,
		StoragePayloadAdmissionKnown: true, StoragePayloadAdmitted: payload.PayloadAdmitted,
		StorageDeviceNumber: uint64(payload.Dev), StorageDeviceNumeric: payload.Dev != 0,
		StorageInodeNumber: payload.Ino, StorageInodeNumeric: payload.Ino != 0,
		StorageOperation: f2fsPayloadOperation(payload),
	}
}

func directF2FSAudit(ev decodedEvent, payload f2fsPayload) directPairLineAudit {
	headerTID, ownerKnown := directPairHeaderTID(ev)
	if payload.Name == "" {
		payload.Name = ev.format.Name
		payload.Kind, _ = directF2FSKindForName(ev.format.Name)
		payload.HeaderTID = headerTID
		payload.HeaderOwnerKnown = ownerKnown
	}
	input := f2fsPayloadTypedInput(payload)
	if !ownerKnown {
		input.HeaderTID = -1
	}
	return directPairLineAudit{
		Governed: true, Kind: pairRenderF2FS, HeaderTID: headerTID,
		HeaderOwnerKnown: ownerKnown, Verdict: fingerprintPairingEndpoint(input),
	}
}

func directF2FSWireParity(payload f2fsPayload, body string, typed tracequery.PairingEndpointVerdict) bool {
	wire := tracequery.DecodePairingEndpoint(payload.Name, body, payload.HeaderTID)
	return wire.Recognized == typed.Recognized && wire.KeyKnown == typed.KeyKnown &&
		wire.PayloadAdmitted == typed.PayloadAdmitted && wire.Family == typed.Family &&
		wire.Phase == typed.Phase && wire.SemanticKey == typed.SemanticKey &&
		wire.EmitterKnown == typed.EmitterKnown && wire.EmitterAdmitted == typed.EmitterAdmitted
}

func directF2FSProfilesForName(name string) []directF2FSProfile {
	switch name {
	case "f2fs_sync_file_enter":
		return []directF2FSProfile{directF2FSProfileSyncEnter}
	case "f2fs_sync_file_exit":
		return []directF2FSProfile{directF2FSProfileSyncExit}
	case "f2fs_direct_IO_enter":
		return []directF2FSProfile{directF2FSProfileDirectIOEnter510, directF2FSProfileDirectIOEnter66}
	case "f2fs_direct_IO_exit":
		return []directF2FSProfile{directF2FSProfileDirectIOExit}
	case "f2fs_write_begin":
		return []directF2FSProfile{directF2FSProfileWriteBegin510, directF2FSProfileWriteBegin66}
	case "f2fs_write_end":
		return []directF2FSProfile{directF2FSProfileWriteEnd}
	default:
		return nil
	}
}

func directF2FSSpecs(profile directF2FSProfile, wordSize int) []directF2FSFieldSpec {
	if wordSize != 4 && wordSize != 8 {
		return nil
	}
	spec := func(name, typ string, offset, size int, signed bool) directF2FSFieldSpec {
		return directF2FSFieldSpec{Name: name, Type: typ, Offset: offset, Size: size, Signed: signed}
	}
	dev := spec("dev", "dev_t", 8, 4, false)
	inoOffset := 12
	if wordSize == 8 {
		inoOffset = 16
	}
	ino := spec("ino", "ino_t", inoOffset, wordSize, false)
	switch profile {
	case directF2FSProfileSyncEnter:
		if wordSize == 4 {
			return []directF2FSFieldSpec{dev, ino, spec("pino", "ino_t", 16, 4, false),
				spec("mode", "umode_t", 20, 2, false), spec("size", "loff_t", 24, 8, true),
				spec("nlink", "unsigned int", 32, 4, false), spec("blocks", "blkcnt_t", 40, 8, false),
				spec("advise", "__u8", 48, 1, false)}
		}
		return []directF2FSFieldSpec{dev, ino, spec("pino", "ino_t", 24, 8, false),
			spec("mode", "umode_t", 32, 2, false), spec("size", "loff_t", 40, 8, true),
			spec("nlink", "unsigned int", 48, 4, false), spec("blocks", "blkcnt_t", 56, 8, false),
			spec("advise", "__u8", 64, 1, false)}
	case directF2FSProfileSyncExit:
		return []directF2FSFieldSpec{dev, ino,
			spec("cp_reason", "int", inoOffset+wordSize, 4, true),
			spec("datasync", "int", inoOffset+wordSize+4, 4, true),
			spec("ret", "int", inoOffset+wordSize+8, 4, true)}
	case directF2FSProfileDirectIOEnter510:
		posOffset := inoOffset + wordSize
		return []directF2FSFieldSpec{dev, ino, spec("pos", "loff_t", posOffset, 8, true),
			spec("len", "unsigned long", posOffset+8, wordSize, false),
			spec("rw", "int", posOffset+8+wordSize, 4, true)}
	case directF2FSProfileDirectIOEnter66:
		posOffset := inoOffset + wordSize
		lenOffset := 32
		if wordSize == 8 {
			lenOffset = 40
		}
		return []directF2FSFieldSpec{dev, ino, spec("ki_pos", "loff_t", posOffset, 8, true),
			spec("ki_flags", "int", posOffset+8, 4, true), spec("ki_ioprio", "u16", posOffset+12, 2, false),
			spec("len", "unsigned long", lenOffset, wordSize, false), spec("rw", "int", lenOffset+wordSize, 4, true)}
	case directF2FSProfileDirectIOExit:
		posOffset := inoOffset + wordSize
		return []directF2FSFieldSpec{dev, ino, spec("pos", "loff_t", posOffset, 8, true),
			spec("len", "unsigned long", posOffset+8, wordSize, false),
			spec("rw", "int", posOffset+8+wordSize, 4, true),
			spec("ret", "int", posOffset+12+wordSize, 4, true)}
	case directF2FSProfileWriteBegin510, directF2FSProfileWriteBegin66, directF2FSProfileWriteEnd:
		posOffset := inoOffset + wordSize
		fields := []directF2FSFieldSpec{dev, ino, spec("pos", "loff_t", posOffset, 8, true),
			spec("len", "unsigned int", posOffset+8, 4, false)}
		if profile == directF2FSProfileWriteBegin510 {
			fields = append(fields, spec("flags", "unsigned int", posOffset+12, 4, false))
		} else if profile == directF2FSProfileWriteEnd {
			fields = append(fields, spec("copied", "unsigned int", posOffset+12, 4, false))
		}
		return fields
	default:
		return nil
	}
}

func directF2FSExactFields(ev decodedEvent, specs []directF2FSFieldSpec) (map[string][]byte, bool) {
	if len(specs) == 0 {
		return nil, false
	}
	expected := make(map[string]directF2FSFieldSpec, len(specs))
	for _, item := range specs {
		expected[item.Name] = item
	}
	values := make(map[string][]byte, len(specs))
	common := make(map[string]bool, 4)
	for index, field := range ev.format.Fields {
		name := cleanFieldName(field.Name)
		if strings.HasPrefix(name, "common_") {
			item, ok := directF2FSCommonSpec(name)
			if !ok || common[name] || !directF2FSFieldMatches(ev, index, field, item) {
				return nil, false
			}
			common[name] = true
			continue
		}
		item, ok := expected[name]
		if !ok || values[name] != nil || !directF2FSFieldMatches(ev, index, field, item) {
			return nil, false
		}
		raw, ok := ev.fields[field.Name]
		if !ok || len(raw) != field.Size {
			return nil, false
		}
		values[name] = raw
	}
	return values, len(common) == 4 && len(values) == len(specs)
}

func directF2FSFieldMatches(ev decodedEvent, index int, field eventField, spec directF2FSFieldSpec) bool {
	return field.Name == spec.Name && strings.Join(strings.Fields(field.Type), " ") == spec.Type &&
		field.Offset == spec.Offset && field.Size == spec.Size && field.Signed == spec.Signed &&
		directDescriptorFieldIsolated(ev, index)
}

func directF2FSCommonSpec(name string) (directF2FSFieldSpec, bool) {
	switch name {
	case "common_type":
		return directF2FSFieldSpec{Name: name, Type: "unsigned short", Offset: 0, Size: 2}, true
	case "common_flags":
		return directF2FSFieldSpec{Name: name, Type: "unsigned char", Offset: 2, Size: 1}, true
	case "common_preempt_count":
		return directF2FSFieldSpec{Name: name, Type: "unsigned char", Offset: 3, Size: 1}, true
	case "common_pid":
		return directF2FSFieldSpec{Name: name, Type: "int", Offset: 4, Size: 4, Signed: true}, true
	default:
		return directF2FSFieldSpec{}, false
	}
}

func directF2FSUint(raw []byte) uint64 {
	switch len(raw) {
	case 1:
		return uint64(raw[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(raw))
	case 4:
		return uint64(binary.LittleEndian.Uint32(raw))
	case 8:
		return binary.LittleEndian.Uint64(raw)
	default:
		return 0
	}
}

func directF2FSInt(raw []byte) int64 {
	switch len(raw) {
	case 4:
		return int64(int32(binary.LittleEndian.Uint32(raw)))
	case 8:
		return int64(binary.LittleEndian.Uint64(raw))
	default:
		return 0
	}
}
