package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// busRenderKind is the exact direct-RMQ I2C/SMBus inventory profile. These
// events are not pair endpoints: I2C result is batch-wide and neither family
// carries a transaction identity that could support honest elapsed pairing.
type busRenderKind uint8

const (
	busRenderUnknown busRenderKind = iota
	busRenderI2CRead
	busRenderI2CWrite
	busRenderI2CReply
	busRenderI2CResult
	busRenderSMBusRead
	busRenderSMBusWrite
	busRenderSMBusReply
	busRenderSMBusResult
)

const (
	directBusI2CFixedTail   = 24
	directBusSMBusBufferLen = 34
	directBusSMBusBlockMax  = 32
)

type busRenderPayload struct {
	Kind      busRenderKind
	Name      string
	Adapter   int32
	Number    uint16
	Address   uint16
	Flags     uint16
	Length    uint16
	Return    int16
	Command   uint8
	Protocol  uint32
	ReadWrite uint8
	Result    int16
	Data      []byte
}

type directBusFieldSpec struct {
	Name          string
	DeclaredNames []string
	Type          string
	Offset        int
	Size          int
	Signed        bool
}

func directBusNameGoverned(name string) bool {
	_, ok := directBusKindForName(name)
	return ok
}

func directBusKindForName(name string) (busRenderKind, bool) {
	switch name {
	case "i2c_read":
		return busRenderI2CRead, true
	case "i2c_write":
		return busRenderI2CWrite, true
	case "i2c_reply":
		return busRenderI2CReply, true
	case "i2c_result":
		return busRenderI2CResult, true
	case "smbus_read":
		return busRenderSMBusRead, true
	case "smbus_write":
		return busRenderSMBusWrite, true
	case "smbus_reply":
		return busRenderSMBusReply, true
	case "smbus_result":
		return busRenderSMBusResult, true
	default:
		return busRenderUnknown, false
	}
}

func decodeDirectBusPayload(ev decodedEvent, content []byte) (busRenderPayload, bodyAdmission, string) {
	kind, governed := directBusKindForName(ev.format.Name)
	if !governed {
		return busRenderPayload{}, bodyUnsupported, ""
	}
	payload := busRenderPayload{Kind: kind, Name: ev.format.Name}
	fields, ok := directBusExactFields(ev, directBusSpecs(kind))
	if !ok {
		return payload, bodyRejected, "missing_or_invalid_bus_profile"
	}
	payload.Adapter = int32(binary.LittleEndian.Uint32(fields["adapter_nr"]))

	switch kind {
	case busRenderI2CRead, busRenderI2CWrite, busRenderI2CReply:
		payload.Number = binary.LittleEndian.Uint16(fields["msg_nr"])
		payload.Address = binary.LittleEndian.Uint16(fields["addr"])
		payload.Flags = binary.LittleEndian.Uint16(fields["flags"])
		payload.Length = binary.LittleEndian.Uint16(fields["len"])
		if kind == busRenderI2CRead {
			return payload, bodyAdmitted, ""
		}
		data, dataOK := directBusI2CData(ev, content, fields["buf"], payload.Length)
		if !dataOK {
			return payload, bodyRejected, "missing_or_invalid_i2c_buffer"
		}
		payload.Data = data
		return payload, bodyAdmitted, ""

	case busRenderI2CResult:
		payload.Number = binary.LittleEndian.Uint16(fields["nr_msgs"])
		payload.Return = int16(binary.LittleEndian.Uint16(fields["ret"]))
		return payload, bodyAdmitted, ""

	case busRenderSMBusRead:
		payload.Flags = binary.LittleEndian.Uint16(fields["flags"])
		payload.Address = binary.LittleEndian.Uint16(fields["addr"])
		payload.Command = fields["command"][0]
		payload.Protocol = binary.LittleEndian.Uint32(fields["protocol"])
		if payload.Protocol > 8 || len(fields["buf"]) != directBusSMBusBufferLen {
			return payload, bodyRejected, "missing_or_invalid_smbus_payload"
		}
		return payload, bodyAdmitted, ""

	case busRenderSMBusWrite, busRenderSMBusReply:
		payload.Address = binary.LittleEndian.Uint16(fields["addr"])
		payload.Flags = binary.LittleEndian.Uint16(fields["flags"])
		payload.Command = fields["command"][0]
		payload.Length = uint16(fields["len"][0])
		payload.Protocol = binary.LittleEndian.Uint32(fields["protocol"])
		buffer := fields["buf"]
		if payload.Protocol > 8 || len(buffer) != directBusSMBusBufferLen {
			return payload, bodyRejected, "missing_or_invalid_smbus_payload"
		}
		expected, expectedOK := directBusSMBusExpectedLength(kind, payload.Protocol, buffer)
		if !expectedOK || payload.Length != uint16(expected) {
			return payload, bodyRejected, "invalid_smbus_length"
		}
		payload.Data = append([]byte(nil), buffer[:expected]...)
		return payload, bodyAdmitted, ""

	case busRenderSMBusResult:
		payload.Address = binary.LittleEndian.Uint16(fields["addr"])
		payload.Flags = binary.LittleEndian.Uint16(fields["flags"])
		payload.ReadWrite = fields["read_write"][0]
		payload.Command = fields["command"][0]
		payload.Result = int16(binary.LittleEndian.Uint16(fields["res"]))
		payload.Protocol = binary.LittleEndian.Uint32(fields["protocol"])
		if payload.Protocol > 8 || payload.ReadWrite > 1 {
			return payload, bodyRejected, "missing_or_invalid_smbus_result"
		}
		return payload, bodyAdmitted, ""
	}
	return payload, bodyRejected, "invalid_bus_kind"
}

func renderCanonicalBusPayload(payload busRenderPayload) (string, bool) {
	kind, governed := directBusKindForName(payload.Name)
	if !governed || kind != payload.Kind {
		return "", false
	}
	switch kind {
	case busRenderI2CRead:
		if len(payload.Data) != 0 {
			return "", false
		}
		return fmt.Sprintf("i2c-%d #%d a=%03x f=%04x l=%d", payload.Adapter, payload.Number,
			payload.Address, payload.Flags, payload.Length), true
	case busRenderI2CWrite, busRenderI2CReply:
		if len(payload.Data) != int(payload.Length) {
			return "", false
		}
		data, ok := renderDirectBusHex(payload.Data)
		if !ok {
			return "", false
		}
		return fmt.Sprintf("i2c-%d #%d a=%03x f=%04x l=%d %s", payload.Adapter, payload.Number,
			payload.Address, payload.Flags, payload.Length, data), true
	case busRenderI2CResult:
		if len(payload.Data) != 0 {
			return "", false
		}
		return fmt.Sprintf("i2c-%d n=%d ret=%d", payload.Adapter, payload.Number, payload.Return), true
	case busRenderSMBusRead:
		protocol, ok := directBusSMBusProtocolName(payload.Protocol)
		if !ok || payload.Length != 0 || len(payload.Data) != 0 {
			return "", false
		}
		return fmt.Sprintf("i2c-%d a=%03x f=%04x c=%x %s", payload.Adapter, payload.Address,
			payload.Flags, payload.Command, protocol), true
	case busRenderSMBusWrite, busRenderSMBusReply:
		protocol, ok := directBusSMBusProtocolName(payload.Protocol)
		if !ok || len(payload.Data) != int(payload.Length) {
			return "", false
		}
		expected, expectedOK := directBusSMBusExpectedLength(kind, payload.Protocol, payload.Data)
		if !expectedOK || expected != len(payload.Data) {
			return "", false
		}
		data, dataOK := renderDirectBusHex(payload.Data)
		if !dataOK {
			return "", false
		}
		return fmt.Sprintf("i2c-%d a=%03x f=%04x c=%x %s l=%d %s", payload.Adapter,
			payload.Address, payload.Flags, payload.Command, protocol, payload.Length, data), true
	case busRenderSMBusResult:
		protocol, ok := directBusSMBusProtocolName(payload.Protocol)
		if !ok || payload.ReadWrite > 1 || payload.Length != 0 || len(payload.Data) != 0 {
			return "", false
		}
		direction := "rd"
		if payload.ReadWrite == 0 {
			direction = "wr"
		}
		return fmt.Sprintf("i2c-%d a=%03x f=%04x c=%x %s %s res=%d", payload.Adapter,
			payload.Address, payload.Flags, payload.Command, protocol, direction, payload.Result), true
	default:
		return "", false
	}
}

func directBusSpecs(kind busRenderKind) []directBusFieldSpec {
	adapter := directBusFieldSpec{Name: "adapter_nr", Type: "int", Offset: 8, Size: 4, Signed: true}
	switch kind {
	case busRenderI2CRead:
		return []directBusFieldSpec{
			adapter,
			{Name: "msg_nr", Type: "__u16", Offset: 12, Size: 2},
			{Name: "addr", Type: "__u16", Offset: 14, Size: 2},
			{Name: "flags", Type: "__u16", Offset: 16, Size: 2},
			{Name: "len", Type: "__u16", Offset: 18, Size: 2},
		}
	case busRenderI2CWrite, busRenderI2CReply:
		return []directBusFieldSpec{
			adapter,
			{Name: "msg_nr", Type: "__u16", Offset: 12, Size: 2},
			{Name: "addr", Type: "__u16", Offset: 14, Size: 2},
			{Name: "flags", Type: "__u16", Offset: 16, Size: 2},
			{Name: "len", Type: "__u16", Offset: 18, Size: 2},
			{Name: "buf", Type: "__data_loc __u8[]", Offset: 20, Size: 4},
		}
	case busRenderI2CResult:
		return []directBusFieldSpec{
			adapter,
			{Name: "nr_msgs", Type: "__u16", Offset: 12, Size: 2},
			{Name: "ret", Type: "__s16", Offset: 14, Size: 2, Signed: true},
		}
	case busRenderSMBusRead:
		return []directBusFieldSpec{
			adapter,
			{Name: "flags", Type: "__u16", Offset: 12, Size: 2},
			{Name: "addr", Type: "__u16", Offset: 14, Size: 2},
			{Name: "command", Type: "__u8", Offset: 16, Size: 1},
			{Name: "protocol", Type: "__u32", Offset: 20, Size: 4},
			{Name: "buf", DeclaredNames: []string{"buf[32 + 2]", "buf[34]"}, Type: "__u8", Offset: 24, Size: directBusSMBusBufferLen},
		}
	case busRenderSMBusWrite, busRenderSMBusReply:
		return []directBusFieldSpec{
			adapter,
			{Name: "addr", Type: "__u16", Offset: 12, Size: 2},
			{Name: "flags", Type: "__u16", Offset: 14, Size: 2},
			{Name: "command", Type: "__u8", Offset: 16, Size: 1},
			{Name: "len", Type: "__u8", Offset: 17, Size: 1},
			{Name: "protocol", Type: "__u32", Offset: 20, Size: 4},
			{Name: "buf", DeclaredNames: []string{"buf[32 + 2]", "buf[34]"}, Type: "__u8", Offset: 24, Size: directBusSMBusBufferLen},
		}
	case busRenderSMBusResult:
		return []directBusFieldSpec{
			adapter,
			{Name: "addr", Type: "__u16", Offset: 12, Size: 2},
			{Name: "flags", Type: "__u16", Offset: 14, Size: 2},
			{Name: "read_write", Type: "__u8", Offset: 16, Size: 1},
			{Name: "command", Type: "__u8", Offset: 17, Size: 1},
			{Name: "res", Type: "__s16", Offset: 18, Size: 2, Signed: true},
			{Name: "protocol", Type: "__u32", Offset: 20, Size: 4},
		}
	default:
		return nil
	}
}

func directBusExactFields(ev decodedEvent, specs []directBusFieldSpec) (map[string][]byte, bool) {
	if len(specs) == 0 {
		return nil, false
	}
	expected := make(map[string]directBusFieldSpec, len(specs))
	for _, spec := range specs {
		expected[spec.Name] = spec
	}
	values := make(map[string][]byte, len(specs))
	for index, field := range ev.format.Fields {
		if strings.HasPrefix(cleanFieldName(field.Name), "common_") {
			continue
		}
		logicalName := cleanFieldName(field.Name)
		spec, found := expected[logicalName]
		if !found || values[logicalName] != nil || !directBusDeclaredNameAllowed(spec, field.Name) ||
			normalizeFieldType(field.Type) != spec.Type ||
			field.Offset != spec.Offset || field.Size != spec.Size || field.Signed != spec.Signed ||
			!directDescriptorFieldIsolated(ev, index) {
			return nil, false
		}
		raw, present := ev.fields[field.Name]
		if !present || len(raw) != field.Size {
			return nil, false
		}
		values[logicalName] = raw
	}
	if len(values) != len(specs) {
		return nil, false
	}
	return values, true
}

func directBusDeclaredNameAllowed(spec directBusFieldSpec, declared string) bool {
	if len(spec.DeclaredNames) == 0 {
		return declared == spec.Name
	}
	for _, allowed := range spec.DeclaredNames {
		if declared == allowed {
			return true
		}
	}
	return false
}

func directBusI2CData(ev decodedEvent, content, locator []byte, length uint16) ([]byte, bool) {
	if len(locator) != 4 {
		return nil, false
	}
	fixedTail, ok := directDescriptorFixedTail(ev)
	if !ok || fixedTail != directBusI2CFixedTail {
		return nil, false
	}
	location := binary.LittleEndian.Uint32(locator)
	offset := int(location & 0xffff)
	physicalLength := int(location >> 16)
	wantLength := int(length)
	if offset != fixedTail || physicalLength != wantLength || offset > len(content) ||
		physicalLength > len(content)-offset {
		return nil, false
	}
	return append([]byte(nil), content[offset:offset+physicalLength]...), true
}

func directBusSMBusExpectedLength(kind busRenderKind, protocol uint32, data []byte) (int, bool) {
	if protocol > 8 {
		return 0, false
	}
	blockLength := func() (int, bool) {
		if len(data) == 0 || data[0] > directBusSMBusBlockMax {
			return 0, false
		}
		return int(data[0]) + 1, true
	}
	switch kind {
	case busRenderSMBusWrite:
		switch protocol {
		case 2:
			return 1, true
		case 3, 4:
			return 2, true
		case 5, 7, 8:
			return blockLength()
		case 0, 1, 6:
			return 0, true
		}
	case busRenderSMBusReply:
		switch protocol {
		case 1, 2:
			return 1, true
		case 3, 4:
			return 2, true
		case 5, 7, 8:
			return blockLength()
		case 0, 6:
			return 0, true
		}
	}
	return 0, false
}

func directBusSMBusProtocolName(protocol uint32) (string, bool) {
	names := [...]string{
		"QUICK", "BYTE", "BYTE_DATA", "WORD_DATA", "PROC_CALL", "BLOCK_DATA",
		"I2C_BLOCK_BROKEN", "BLOCK_PROC_CALL", "I2C_BLOCK_DATA",
	}
	if protocol >= uint32(len(names)) {
		return "", false
	}
	return names[protocol], true
}

func renderDirectBusHex(data []byte) (string, bool) {
	// [xx-yy] is 3*n+1 bytes for n>0. Bound before multiplication/Grow so a
	// future wider carrier cannot turn an untrusted length into allocation or
	// CPU amplification; the complete line is checked again at publication.
	if len(data) > (maxTraceDBSystraceLineBytes-1)/3 {
		return "", false
	}
	size := 2
	if len(data) > 0 {
		size = 3*len(data) + 1
	}
	if size > maxTraceDBSystraceLineBytes {
		return "", false
	}
	const digits = "0123456789abcdef"
	var out strings.Builder
	out.Grow(size)
	out.WriteByte('[')
	for index, value := range data {
		if index > 0 {
			out.WriteByte('-')
		}
		out.WriteByte(digits[value>>4])
		out.WriteByte(digits[value&0xf])
	}
	out.WriteByte(']')
	return out.String(), true
}
