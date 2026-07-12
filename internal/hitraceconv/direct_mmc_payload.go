package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type mmcPayloadKind uint8

const (
	mmcPayloadStart mmcPayloadKind = iota + 1
	mmcPayloadDone
)

type mmcStartPayload struct {
	CmdOpcode    uint32
	CmdArg       uint32
	CmdFlags     uint32
	CmdRetries   uint32
	StopOpcode   uint32
	StopArg      uint32
	StopFlags    uint32
	StopRetries  uint32
	SBCOpcode    uint32
	SBCArg       uint32
	SBCFlags     uint32
	SBCRetries   uint32
	Blocks       uint32
	BlockAddr    uint32
	BlockSize    uint32
	DataFlags    uint32
	Tag          int64
	CanRetune    uint32
	DoingRetune  uint32
	RetuneNow    uint32
	NeedRetune   int64
	HoldRetune   int64
	RetunePeriod uint32
	MRQ          uint64
	Name         string
}

type mmcDonePayload struct {
	CmdOpcode         uint32
	CmdErr            int64
	CmdResponse       [4]uint32
	CmdResponseKnown  bool
	CmdRetries        uint32
	StopOpcode        uint32
	StopErr           int64
	StopResponse      [4]uint32
	StopResponseKnown bool
	StopRetries       uint32
	SBCOpcode         uint32
	SBCErr            int64
	SBCResponse       [4]uint32
	SBCResponseKnown  bool
	SBCRetries        uint32
	BytesXfered       uint32
	DataErr           int64
	Tag               int64
	CanRetune         uint32
	DoingRetune       uint32
	RetuneNow         uint32
	NeedRetune        int64
	HoldRetune        int64
	RetunePeriod      uint32
	MRQ               uint64
	Name              string
}

type mmcPayload struct {
	Kind             mmcPayloadKind
	Name             string
	HeaderTID        int64
	HeaderOwnerKnown bool
	Start            *mmcStartPayload
	Done             *mmcDonePayload
}

type directMMCFieldSpec struct {
	Name         string
	DeclaredName string
	Type         string
	Offset       int
	Size         int
	Signed       bool
}

func directMMCNameGoverned(name string) bool {
	return name == "mmc_request_start" || name == "mmc_request_done"
}

func directMMCNameCandidate(name string) bool {
	canonicalLooking := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(canonicalLooking, "mmc_") || strings.Contains(canonicalLooking, "mmc_request_")
}

func mmcPayloadTypedInput(payload mmcPayload) tracequery.PairingEndpointTypedInput {
	input := tracequery.PairingEndpointTypedInput{Name: payload.Name, HeaderTID: payload.HeaderTID}
	switch payload.Kind {
	case mmcPayloadStart:
		if payload.Start != nil {
			input.StorageIdentityKnown = payload.Start.Name != ""
			input.StoragePayloadAdmissionKnown = true
			input.StoragePayloadAdmitted = input.StorageIdentityKnown
			input.StorageDevice = payload.Start.Name
			input.StorageOperation = strconv.FormatUint(uint64(payload.Start.CmdOpcode), 10)
		}
	case mmcPayloadDone:
		if payload.Done != nil {
			input.StorageIdentityKnown = payload.Done.Name != ""
			input.StoragePayloadAdmissionKnown = true
			input.StoragePayloadAdmitted = input.StorageIdentityKnown
			input.StorageDevice = payload.Done.Name
			input.StorageOperation = strconv.FormatUint(uint64(payload.Done.CmdOpcode), 10)
		}
	}
	return input
}

func directMMCAudit(ev decodedEvent, payload mmcPayload) directPairLineAudit {
	headerTID, ownerKnown := directPairHeaderTID(ev)
	if payload.Name == "" {
		payload.Name = ev.format.Name
		payload.HeaderTID = headerTID
		payload.HeaderOwnerKnown = ownerKnown
	}
	input := mmcPayloadTypedInput(payload)
	if !ownerKnown {
		input.HeaderTID = -1
	}
	return directPairLineAudit{
		Governed: true, Kind: pairRenderMMC, HeaderTID: headerTID,
		HeaderOwnerKnown: ownerKnown, Verdict: fingerprintPairingEndpoint(input),
	}
}

func directMMCWireParity(payload mmcPayload, body string, typed tracequery.PairingEndpointVerdict) bool {
	wire := tracequery.DecodePairingEndpoint(payload.Name, body, payload.HeaderTID)
	return wire.Recognized == typed.Recognized && wire.KeyKnown == typed.KeyKnown &&
		wire.PayloadAdmitted == typed.PayloadAdmitted && wire.Family == typed.Family &&
		wire.Phase == typed.Phase && wire.SemanticKey == typed.SemanticKey &&
		wire.EmitterKnown == typed.EmitterKnown && wire.EmitterAdmitted == typed.EmitterAdmitted
}

func decodeDirectMMCPayload(ev decodedEvent, content []byte) (mmcPayload, bodyAdmission, string) {
	if !directMMCNameGoverned(ev.format.Name) {
		return mmcPayload{}, bodyUnsupported, ""
	}
	kind := mmcPayloadStart
	if ev.format.Name == "mmc_request_done" {
		kind = mmcPayloadDone
	}
	for _, wordSize := range []int{4, 8} {
		specs := directMMCSpecs(kind, wordSize)
		fields, ok := directMMCExactFields(ev, specs)
		if !ok {
			continue
		}
		name, ok := directMMCName(ev, content, fields["name"])
		if !ok {
			return mmcPayload{}, bodyRejected, "invalid_mmc_name"
		}
		mrq := directMMCUint(fields["mrq"])
		if mrq == 0 {
			return mmcPayload{}, bodyRejected, "invalid_mmc_pointer"
		}
		headerTID, headerOwnerKnown := directPairHeaderTID(ev)
		switch kind {
		case mmcPayloadStart:
			item := mmcStartPayload{
				CmdOpcode: directMMCUint32(fields["cmd_opcode"]), CmdArg: directMMCUint32(fields["cmd_arg"]),
				CmdFlags: directMMCUint32(fields["cmd_flags"]), CmdRetries: directMMCUint32(fields["cmd_retries"]),
				StopOpcode: directMMCUint32(fields["stop_opcode"]), StopArg: directMMCUint32(fields["stop_arg"]),
				StopFlags: directMMCUint32(fields["stop_flags"]), StopRetries: directMMCUint32(fields["stop_retries"]),
				SBCOpcode: directMMCUint32(fields["sbc_opcode"]), SBCArg: directMMCUint32(fields["sbc_arg"]),
				SBCFlags: directMMCUint32(fields["sbc_flags"]), SBCRetries: directMMCUint32(fields["sbc_retries"]),
				Blocks: directMMCUint32(fields["blocks"]), BlockAddr: directMMCUint32(fields["blk_addr"]),
				BlockSize: directMMCUint32(fields["blksz"]), DataFlags: directMMCUint32(fields["data_flags"]),
				Tag: directMMCInt32(fields["tag"]), CanRetune: directMMCUint32(fields["can_retune"]),
				DoingRetune: directMMCUint32(fields["doing_retune"]), RetuneNow: directMMCUint32(fields["retune_now"]),
				NeedRetune: directMMCInt32(fields["need_retune"]), HoldRetune: directMMCInt32(fields["hold_retune"]),
				RetunePeriod: directMMCUint32(fields["retune_period"]), MRQ: mrq, Name: name,
			}
			return mmcPayload{Kind: kind, Name: ev.format.Name, HeaderTID: headerTID,
				HeaderOwnerKnown: headerOwnerKnown, Start: &item}, bodyAdmitted, ""
		case mmcPayloadDone:
			item := mmcDonePayload{
				CmdOpcode: directMMCUint32(fields["cmd_opcode"]), CmdErr: directMMCInt32(fields["cmd_err"]),
				CmdResponse: directMMCResponse(fields["cmd_resp"]), CmdResponseKnown: true,
				CmdRetries: directMMCUint32(fields["cmd_retries"]), StopOpcode: directMMCUint32(fields["stop_opcode"]),
				StopErr: directMMCInt32(fields["stop_err"]), StopResponse: directMMCResponse(fields["stop_resp"]),
				StopResponseKnown: true, StopRetries: directMMCUint32(fields["stop_retries"]),
				SBCOpcode: directMMCUint32(fields["sbc_opcode"]), SBCErr: directMMCInt32(fields["sbc_err"]),
				SBCResponse: directMMCResponse(fields["sbc_resp"]), SBCResponseKnown: true,
				SBCRetries: directMMCUint32(fields["sbc_retries"]), BytesXfered: directMMCUint32(fields["bytes_xfered"]),
				DataErr: directMMCInt32(fields["data_err"]), Tag: directMMCInt32(fields["tag"]),
				CanRetune: directMMCUint32(fields["can_retune"]), DoingRetune: directMMCUint32(fields["doing_retune"]),
				RetuneNow: directMMCUint32(fields["retune_now"]), NeedRetune: directMMCInt32(fields["need_retune"]),
				HoldRetune: directMMCInt32(fields["hold_retune"]), RetunePeriod: directMMCUint32(fields["retune_period"]),
				MRQ: mrq, Name: name,
			}
			return mmcPayload{Kind: kind, Name: ev.format.Name, HeaderTID: headerTID,
				HeaderOwnerKnown: headerOwnerKnown, Done: &item}, bodyAdmitted, ""
		}
	}
	return mmcPayload{}, bodyRejected, "invalid_mmc_descriptor_profile"
}

func renderCanonicalMMCPayload(payload mmcPayload) (string, bool) {
	switch payload.Kind {
	case mmcPayloadStart:
		item := payload.Start
		if item == nil {
			return "", false
		}
		return fmt.Sprintf("%s: start struct mmc_request[0x%x]: cmd_opcode=%d cmd_arg=0x%x cmd_flags=0x%x cmd_retries=%d stop_opcode=%d stop_arg=0x%x stop_flags=0x%x stop_retries=%d sbc_opcode=%d sbc_arg=0x%x sbc_flags=0x%x sbc_retires=%d blocks=%d block_size=%d blk_addr=%d data_flags=0x%x tag=%d can_retune=%d doing_retune=%d retune_now=%d need_retune=%d hold_retune=%d retune_period=%d",
			item.Name, item.MRQ, item.CmdOpcode, item.CmdArg, item.CmdFlags, item.CmdRetries,
			item.StopOpcode, item.StopArg, item.StopFlags, item.StopRetries,
			item.SBCOpcode, item.SBCArg, item.SBCFlags, item.SBCRetries,
			item.Blocks, item.BlockSize, item.BlockAddr, item.DataFlags, item.Tag,
			item.CanRetune, item.DoingRetune, item.RetuneNow, item.NeedRetune,
			item.HoldRetune, item.RetunePeriod), true
	case mmcPayloadDone:
		item := payload.Done
		if item == nil {
			return "", false
		}
		responseCount := 0
		for _, known := range []bool{item.CmdResponseKnown, item.StopResponseKnown, item.SBCResponseKnown} {
			if known {
				responseCount++
			}
		}
		if responseCount != 0 && responseCount != 3 {
			return "", false
		}
		body := fmt.Sprintf("%s: end struct mmc_request[0x%x]: cmd_opcode=%d cmd_err=%d",
			item.Name, item.MRQ, item.CmdOpcode, item.CmdErr)
		if item.CmdResponseKnown {
			body += fmt.Sprintf(" cmd_resp=0x%x 0x%x 0x%x 0x%x",
				item.CmdResponse[0], item.CmdResponse[1], item.CmdResponse[2], item.CmdResponse[3])
		}
		body += fmt.Sprintf(" cmd_retries=%d stop_opcode=%d stop_err=%d", item.CmdRetries, item.StopOpcode, item.StopErr)
		if item.StopResponseKnown {
			body += fmt.Sprintf(" stop_resp=0x%x 0x%x 0x%x 0x%x",
				item.StopResponse[0], item.StopResponse[1], item.StopResponse[2], item.StopResponse[3])
		}
		body += fmt.Sprintf(" stop_retries=%d sbc_opcode=%d sbc_err=%d", item.StopRetries, item.SBCOpcode, item.SBCErr)
		if item.SBCResponseKnown {
			body += fmt.Sprintf(" sbc_resp=0x%x 0x%x 0x%x 0x%x",
				item.SBCResponse[0], item.SBCResponse[1], item.SBCResponse[2], item.SBCResponse[3])
		}
		body += fmt.Sprintf(" sbc_retries=%d bytes_xfered=%d data_err=%d tag=%d can_retune=%d doing_retune=%d retune_now=%d need_retune=%d hold_retune=%d retune_period=%d",
			item.SBCRetries, item.BytesXfered, item.DataErr, item.Tag, item.CanRetune,
			item.DoingRetune, item.RetuneNow, item.NeedRetune, item.HoldRetune, item.RetunePeriod)
		return body, true
	default:
		return "", false
	}
}

func directMMCSpecs(kind mmcPayloadKind, wordSize int) []directMMCFieldSpec {
	if wordSize != 4 && wordSize != 8 {
		return nil
	}
	spec := func(name, typ string, offset, size int, signed bool) directMMCFieldSpec {
		return directMMCFieldSpec{Name: name, DeclaredName: name, Type: typ, Offset: offset, Size: size, Signed: signed}
	}
	if kind == mmcPayloadStart {
		fields := []directMMCFieldSpec{
			spec("cmd_opcode", "u32", 8, 4, false), spec("cmd_arg", "u32", 12, 4, false),
			spec("cmd_flags", "unsigned int", 16, 4, false), spec("cmd_retries", "unsigned int", 20, 4, false),
			spec("stop_opcode", "u32", 24, 4, false), spec("stop_arg", "u32", 28, 4, false),
			spec("stop_flags", "unsigned int", 32, 4, false), spec("stop_retries", "unsigned int", 36, 4, false),
			spec("sbc_opcode", "u32", 40, 4, false), spec("sbc_arg", "u32", 44, 4, false),
			spec("sbc_flags", "unsigned int", 48, 4, false), spec("sbc_retries", "unsigned int", 52, 4, false),
			spec("blocks", "unsigned int", 56, 4, false), spec("blk_addr", "unsigned int", 60, 4, false),
			spec("blksz", "unsigned int", 64, 4, false), spec("data_flags", "unsigned int", 68, 4, false),
			spec("tag", "int", 72, 4, true), spec("can_retune", "unsigned int", 76, 4, false),
			spec("doing_retune", "unsigned int", 80, 4, false), spec("retune_now", "unsigned int", 84, 4, false),
			spec("need_retune", "int", 88, 4, true), spec("hold_retune", "int", 92, 4, true),
			spec("retune_period", "unsigned int", 96, 4, false),
			spec("mrq", "struct mmc_request *", 100, wordSize, false),
			spec("name", "__data_loc char[]", 100+wordSize, 4, false),
		}
		if wordSize == 8 {
			fields[len(fields)-2].Offset = 104
			fields[len(fields)-1].Offset = 112
		}
		return fields
	}
	array := func(name string, offset int) directMMCFieldSpec {
		item := spec(name, "u32", offset, 16, false)
		item.DeclaredName = name + "[4]"
		return item
	}
	fields := []directMMCFieldSpec{
		spec("cmd_opcode", "u32", 8, 4, false), spec("cmd_err", "int", 12, 4, true),
		array("cmd_resp", 16), spec("cmd_retries", "unsigned int", 32, 4, false),
		spec("stop_opcode", "u32", 36, 4, false), spec("stop_err", "int", 40, 4, true),
		array("stop_resp", 44), spec("stop_retries", "unsigned int", 60, 4, false),
		spec("sbc_opcode", "u32", 64, 4, false), spec("sbc_err", "int", 68, 4, true),
		array("sbc_resp", 72), spec("sbc_retries", "unsigned int", 88, 4, false),
		spec("bytes_xfered", "unsigned int", 92, 4, false), spec("data_err", "int", 96, 4, true),
		spec("tag", "int", 100, 4, true), spec("can_retune", "unsigned int", 104, 4, false),
		spec("doing_retune", "unsigned int", 108, 4, false), spec("retune_now", "unsigned int", 112, 4, false),
		spec("need_retune", "int", 116, 4, true), spec("hold_retune", "int", 120, 4, true),
		spec("retune_period", "unsigned int", 124, 4, false),
		spec("mrq", "struct mmc_request *", 128, wordSize, false),
		spec("name", "__data_loc char[]", 128+wordSize, 4, false),
	}
	return fields
}

func directMMCExactFields(ev decodedEvent, specs []directMMCFieldSpec) (map[string][]byte, bool) {
	if len(specs) == 0 {
		return nil, false
	}
	expected := make(map[string]directMMCFieldSpec, len(specs))
	for _, item := range specs {
		expected[item.Name] = item
	}
	values := make(map[string][]byte, len(specs))
	common := make(map[string]bool, 4)
	for index, field := range ev.format.Fields {
		name := cleanFieldName(field.Name)
		if strings.HasPrefix(name, "common_") {
			item, ok := directMMCCommonSpec(name)
			if !ok || common[name] || !directMMCFieldMatches(ev, index, field, item) {
				return nil, false
			}
			common[name] = true
			continue
		}
		item, ok := expected[name]
		if !ok || values[name] != nil || !directMMCFieldMatches(ev, index, field, item) {
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

func directMMCFieldMatches(ev decodedEvent, index int, field eventField, spec directMMCFieldSpec) bool {
	return field.Name == spec.DeclaredName && directMMCFieldType(field.Type) == spec.Type &&
		field.Offset == spec.Offset && field.Size == spec.Size && field.Signed == spec.Signed &&
		directDescriptorFieldIsolated(ev, index)
}

func directMMCFieldType(value string) string {
	// C identifiers and typedef names are case-sensitive. Collapse only
	// descriptor whitespace; lowercasing would authorize an unrelated `U32`
	// typedef as the pinned producer's exact `u32`.
	return strings.Join(strings.Fields(value), " ")
}

func directMMCCommonSpec(name string) (directMMCFieldSpec, bool) {
	switch name {
	case "common_type":
		return directMMCFieldSpec{Name: name, DeclaredName: name, Type: "unsigned short", Offset: 0, Size: 2}, true
	case "common_flags":
		return directMMCFieldSpec{Name: name, DeclaredName: name, Type: "unsigned char", Offset: 2, Size: 1}, true
	case "common_preempt_count":
		return directMMCFieldSpec{Name: name, DeclaredName: name, Type: "unsigned char", Offset: 3, Size: 1}, true
	case "common_pid":
		return directMMCFieldSpec{Name: name, DeclaredName: name, Type: "int", Offset: 4, Size: 4, Signed: true}, true
	default:
		return directMMCFieldSpec{}, false
	}
}

func directMMCName(ev decodedEvent, content, locator []byte) (string, bool) {
	if len(locator) != 4 {
		return "", false
	}
	tail, ok := directDescriptorFixedTail(ev)
	if !ok {
		return "", false
	}
	location := binary.LittleEndian.Uint32(locator)
	offset, length := int(location&0xffff), int(location>>16)
	if offset < tail || length < 2 || offset > len(content) || length > len(content)-offset {
		return "", false
	}
	raw := content[offset : offset+length]
	if raw[len(raw)-1] != 0 || bytesIndexNUL(raw[:len(raw)-1]) >= 0 {
		return "", false
	}
	name := string(raw[:len(raw)-1])
	return name, len(name) <= 256 && validProfilerMMCName(name)
}

func directMMCUint32(raw []byte) uint32 { return binary.LittleEndian.Uint32(raw) }

func directMMCInt32(raw []byte) int64 { return int64(int32(binary.LittleEndian.Uint32(raw))) }

func directMMCUint(raw []byte) uint64 {
	if len(raw) == 4 {
		return uint64(binary.LittleEndian.Uint32(raw))
	}
	return binary.LittleEndian.Uint64(raw)
}

func directMMCResponse(raw []byte) [4]uint32 {
	return [4]uint32{
		binary.LittleEndian.Uint32(raw[0:4]), binary.LittleEndian.Uint32(raw[4:8]),
		binary.LittleEndian.Uint32(raw[8:12]), binary.LittleEndian.Uint32(raw[12:16]),
	}
}
