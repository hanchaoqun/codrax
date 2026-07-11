package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// blockRenderPayload is the sole source-neutral authority used to render the
// block request/remap rows that tracequery later reparses. Raw RMQ events and
// structured profiler protobufs have different presence rules, so each source
// has a strict decoder; neither source is allowed to invent another field's
// value before entering this representation.
type blockRenderPayload struct {
	kind      blockRenderKind
	dev       uint32
	sector    uint64
	nrSector  uint32
	bytes     uint32
	errorCode int32
	rwbs      string
	cmd       string
	comm      string
	oldDev    uint32
	oldSector uint64
	nrBios    uint32
}

type blockRenderKind uint8

const (
	blockRenderBioComplete blockRenderKind = iota + 1
	blockRenderBioQueue
	blockRenderBioRemap
	blockRenderRQComplete
	blockRenderRQInsert
	blockRenderRQIssue
	blockRenderRQRemap
)

// validBlockRWBS is the source-neutral operation grammar shared by every
// block decoder and mirrored by tracequery: a bounded non-empty sequence of
// ASCII letters. Delimiters or numeric text could rewrite the positional
// ftrace grammar and therefore fail closed before rendering.
func validBlockRWBS(value string) bool {
	if len(value) == 0 || len(value) > 32 {
		return false
	}
	for i := 0; i < len(value); i++ {
		if (value[i] < 'A' || value[i] > 'Z') && (value[i] < 'a' || value[i] > 'z') {
			return false
		}
	}
	return true
}

func blockRenderKindForName(name string) (blockRenderKind, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "block_bio_complete":
		return blockRenderBioComplete, true
	case "block_bio_queue":
		return blockRenderBioQueue, true
	case "block_bio_remap":
		return blockRenderBioRemap, true
	case "block_rq_complete":
		return blockRenderRQComplete, true
	case "block_rq_insert":
		return blockRenderRQInsert, true
	case "block_rq_issue":
		return blockRenderRQIssue, true
	case "block_rq_remap":
		return blockRenderRQRemap, true
	default:
		return 0, false
	}
}

func blockRenderKindForProfilerField(field int) (blockRenderKind, string, bool) {
	switch field {
	case 202:
		return blockRenderBioComplete, "block_bio_complete", true
	case 204:
		return blockRenderBioQueue, "block_bio_queue", true
	case 205:
		return blockRenderBioRemap, "block_bio_remap", true
	case 209:
		return blockRenderRQComplete, "block_rq_complete", true
	case 210:
		return blockRenderRQInsert, "block_rq_insert", true
	case 211:
		return blockRenderRQIssue, "block_rq_issue", true
	case 212:
		return blockRenderRQRemap, "block_rq_remap", true
	default:
		return 0, "", false
	}
}

// renderCanonicalBlockPayload intentionally mirrors the pinned OpenHarmony
// ftrace formatter (and the customer Donghu reference trace): device numbers
// use major,minor and the field order is stable for every event profile.
func renderCanonicalBlockPayload(payload blockRenderPayload) string {
	dev := devMajorMinor(int64(payload.dev), ",")
	switch payload.kind {
	case blockRenderBioComplete:
		return fmt.Sprintf("%s %s %d + %d [%d]", dev, payload.rwbs, payload.sector,
			payload.nrSector, payload.errorCode)
	case blockRenderBioQueue:
		return fmt.Sprintf("%s %s %d + %d [%s]", dev, payload.rwbs, payload.sector,
			payload.nrSector, payload.comm)
	case blockRenderRQComplete:
		return fmt.Sprintf("%s %s (%s) %d + %d [%d]", dev, payload.rwbs, payload.cmd,
			payload.sector, payload.nrSector, payload.errorCode)
	case blockRenderRQInsert, blockRenderRQIssue:
		return fmt.Sprintf("%s %s %d (%s) %d + %d [%s]", dev, payload.rwbs, payload.bytes,
			payload.cmd, payload.sector, payload.nrSector, payload.comm)
	case blockRenderRQRemap:
		return fmt.Sprintf("%s %s %d + %d <- (%s) %d %d", dev, payload.rwbs, payload.sector,
			payload.nrSector, devMajorMinor(int64(payload.oldDev), ","), payload.oldSector, payload.nrBios)
	case blockRenderBioRemap:
		return fmt.Sprintf("%s %s %d + %d <- (%s) %d", dev, payload.rwbs, payload.sector,
			payload.nrSector, devMajorMinor(int64(payload.oldDev), ","), payload.oldSector)
	default:
		return ""
	}
}

func renderDirectBlockEvent(ev decodedEvent, content []byte) (string, bool) {
	payload, _, ok := decodeDirectBlockPayload(ev, content)
	if !ok {
		return "", false
	}
	return renderCanonicalBlockPayload(payload), true
}

// decodeDirectBlockPayload is deliberately presence-sensitive. Unlike proto3,
// an absent raw field is not an encoded zero. Aliases are accepted only as one
// unique physical authority, and byte counts never substitute for sectors.
func decodeDirectBlockPayload(ev decodedEvent, content []byte) (blockRenderPayload, []string, bool) {
	kind, ok := blockRenderKindForName(ev.format.Name)
	if !ok {
		return blockRenderPayload{}, nil, false
	}
	payload := blockRenderPayload{kind: kind}

	readUint := func(label string, max uint64, widths []int, names ...string) (uint64, bool, []string) {
		value, reason, valid := directBlockUint(ev, label, max, widths, names...)
		if !valid {
			return 0, false, []string{reason}
		}
		return value, true, nil
	}
	dev, valid, reasons := readUint("dev", math.MaxUint32, []int{4, 8}, "dev", "dev_t")
	if !valid {
		return payload, reasons, false
	}
	payload.dev = uint32(dev)
	sector, valid, reasons := readUint("sector", math.MaxInt64, []int{8}, "sector", "lba")
	if !valid {
		return payload, reasons, false
	}
	payload.sector = sector
	nrSector, valid, reasons := readUint("nr_sector", math.MaxUint32, []int{4}, "nr_sector", "nr_sectors", "sectors")
	if !valid {
		return payload, reasons, false
	}
	payload.nrSector = uint32(nrSector)
	rwbs, reason, valid := directBlockString(ev, content, true, "rwbs", "rw", "op")
	if !valid {
		return payload, []string{reason}, false
	}
	if !validBlockRWBS(rwbs) {
		return payload, []string{"direct_rwbs_missing_or_invalid"}, false
	}
	payload.rwbs = rwbs

	switch kind {
	case blockRenderBioComplete, blockRenderRQComplete:
		errorCode, reason, valid := directBlockInt32(ev, "error", "error", "ret", "res")
		if !valid {
			return payload, []string{reason}, false
		}
		payload.errorCode = errorCode
	case blockRenderRQInsert, blockRenderRQIssue:
		bytesValue, valid, reasons := readUint("bytes", math.MaxUint32, []int{4}, "bytes")
		if !valid {
			return payload, reasons, false
		}
		payload.bytes = uint32(bytesValue)
	case blockRenderRQRemap, blockRenderBioRemap:
		oldDev, valid, reasons := readUint("old_dev", math.MaxUint32, []int{4, 8}, "old_dev", "from")
		if !valid {
			return payload, reasons, false
		}
		payload.oldDev = uint32(oldDev)
		oldSector, valid, reasons := readUint("old_sector", math.MaxInt64, []int{8}, "old_sector", "from_sector")
		if !valid {
			return payload, reasons, false
		}
		payload.oldSector = oldSector
		if kind == blockRenderRQRemap {
			nrBios, valid, reasons := readUint("nr_bios", math.MaxUint32, []int{4}, "nr_bios")
			if !valid {
				return payload, reasons, false
			}
			payload.nrBios = uint32(nrBios)
		}
	}

	var degradations []string
	if kind == blockRenderRQComplete || kind == blockRenderRQInsert || kind == blockRenderRQIssue {
		payload.cmd, degradations = directBlockOptionalDisplay(ev, content, "cmd", ')', degradations, "cmd")
	}
	if kind == blockRenderBioQueue || kind == blockRenderRQInsert || kind == blockRenderRQIssue {
		payload.comm, degradations = directBlockOptionalDisplay(ev, content, "comm", ']', degradations, "comm")
	}
	return payload, degradations, true
}

func directBlockUint(ev decodedEvent, label string, max uint64, widths []int, names ...string) (uint64, string, bool) {
	field, raw, reason, ok := directBlockField(ev, label, names...)
	if !ok {
		return 0, reason, false
	}
	if !blockDirectNumericTypeAllowed(field) {
		return 0, "direct_" + label + "_wrong_type", false
	}
	if field.Signed {
		return 0, "direct_" + label + "_wrong_sign", false
	}
	widthOK := false
	for _, width := range widths {
		if field.Size == width && len(raw) == width {
			widthOK = true
			break
		}
	}
	if !widthOK {
		return 0, "direct_" + label + "_wrong_width", false
	}
	value, valid := uintFromSupportedWidth(raw)
	if !valid {
		return 0, "direct_" + label + "_wrong_width", false
	}
	if value > max {
		return 0, "direct_" + label + "_out_of_range", false
	}
	return value, "", true
}

func directBlockInt32(ev decodedEvent, label string, names ...string) (int32, string, bool) {
	field, raw, reason, ok := directBlockField(ev, label, names...)
	if !ok {
		return 0, reason, false
	}
	if !blockDirectNumericTypeAllowed(field) {
		return 0, "direct_" + label + "_wrong_type", false
	}
	if !field.Signed {
		return 0, "direct_" + label + "_wrong_sign", false
	}
	if field.Size != 4 || len(raw) != 4 {
		return 0, "direct_" + label + "_wrong_width", false
	}
	value := intFromBytes(raw, true)
	if value < math.MinInt32 || value > math.MaxInt32 {
		return 0, "direct_" + label + "_out_of_range", false
	}
	return int32(value), "", true
}

func directBlockField(ev decodedEvent, label string, names ...string) (eventField, []byte, string, bool) {
	count := 0
	var selected eventField
	for _, field := range ev.format.Fields {
		if cleanNameIn(cleanFieldName(field.Name), names) {
			selected = field
			count++
		}
	}
	switch {
	case count == 0:
		return eventField{}, nil, "direct_" + label + "_missing_field", false
	case count > 1:
		return eventField{}, nil, "direct_" + label + "_duplicate_alias", false
	}
	raw, present := ev.fields[selected.Name]
	if !present || len(raw) != selected.Size {
		return eventField{}, nil, "direct_" + label + "_truncated_field", false
	}
	return selected, raw, "", true
}

func blockDirectNumericTypeAllowed(field eventField) bool {
	lowerType := normalizeFieldType(field.Type)
	if lowerType == "" || !numericFieldTypeAllowed(field) || strings.ContainsAny(lowerType, "*[]") {
		return false
	}
	for _, denied := range []string{"struct ", "union ", "enum ", "float", "double", "bool", "__data_loc", "__rel_loc"} {
		if strings.Contains(lowerType, denied) {
			return false
		}
	}
	return true
}

func directBlockString(ev decodedEvent, content []byte, required bool, names ...string) (string, string, bool) {
	label := names[0]
	count := 0
	var selected eventField
	for _, field := range ev.format.Fields {
		if cleanNameIn(cleanFieldName(field.Name), names) {
			selected = field
			count++
		}
	}
	if count == 0 {
		if required {
			return "", "direct_" + label + "_missing_field", false
		}
		return "", "", true
	}
	if count > 1 {
		return "", "direct_" + label + "_duplicate_alias", false
	}
	raw, present := ev.fields[selected.Name]
	if !present || len(raw) != selected.Size {
		return "", "direct_" + label + "_truncated_field", false
	}
	lowerType := normalizeFieldType(selected.Type)
	isDataLoc := lowerType == "__data_loc char[]"
	if !isDataLoc && !directBlockFixedCharArray(selected) {
		return "", "direct_" + label + "_wrong_type", false
	}
	var value string
	if isDataLoc {
		if selected.Size != 4 || len(raw) != 4 {
			return "", "direct_" + label + "_wrong_width", false
		}
		loc := binary.LittleEndian.Uint32(raw)
		offset := int(loc & 0xffff)
		length := int(loc >> 16)
		if offset <= 0 || length <= 0 || offset > len(content) || length > len(content)-offset {
			return "", "direct_" + label + "_truncated_field", false
		}
		stringRaw := content[offset : offset+length]
		nul := bytesIndexNUL(stringRaw)
		if nul < 0 {
			return "", "direct_" + label + "_truncated_field", false
		}
		value = string(stringRaw[:nul])
	} else {
		if selected.Size <= 0 {
			return "", "direct_" + label + "_wrong_width", false
		}
		nul := bytesIndexNUL(raw)
		if nul < 0 {
			return "", "direct_" + label + "_truncated_field", false
		}
		raw = raw[:nul]
		value = string(raw)
	}
	if !utf8.ValidString(value) || !traceDBSinglePhysicalLine(value, true) || value != strings.TrimSpace(value) {
		return "", "direct_" + label + "_invalid_string", false
	}
	return value, "", true
}

func directBlockFixedCharArray(field eventField) bool {
	if normalizeFieldType(field.Type) != "char" || field.Size <= 0 {
		return false
	}
	name := strings.TrimSpace(field.Name)
	open := strings.LastIndexByte(name, '[')
	if open <= 0 || !strings.HasSuffix(name, "]") || strings.ContainsAny(name[:open], "[]") {
		return false
	}
	widthText := name[open+1 : len(name)-1]
	if !traceDBBlockDecimal(widthText) {
		return false
	}
	width, err := strconv.Atoi(widthText)
	return err == nil && width == field.Size
}

func directBlockOptionalDisplay(ev decodedEvent, content []byte, label string, forbidden rune, degradations []string, names ...string) (string, []string) {
	value, reason, ok := directBlockString(ev, content, false, names...)
	if !ok {
		return "", append(degradations, reason)
	}
	if value != "" && strings.ContainsRune(value, forbidden) {
		return "", append(degradations, "direct_"+label+"_unsafe_omitted")
	}
	return value, degradations
}

// decodeTraceDBBlockPayload is the SQL/raw source adapter for the same typed
// authority used by direct RMQ and structured profiler events. SQL absence is
// not proto3 zero: every identity/measurement field must be present with its
// exact args datatype, aliases must resolve to one physical key, and byte
// counts never substitute for sectors.
func decodeTraceDBBlockPayload(name string, args map[string]traceDBValue, invalidKeys map[string]bool) (blockRenderPayload, bool) {
	kind, ok := blockRenderKindForName(name)
	if !ok {
		return blockRenderPayload{}, false
	}
	payload := blockRenderPayload{kind: kind}

	readUint := func(max uint64, names ...string) (uint64, bool) {
		value, present, valid := traceDBBlockArg(args, invalidKeys, names...)
		if !present || !valid || value.Datatype != 0 || value.Text != strings.TrimSpace(value.Text) {
			return 0, false
		}
		parsed, err := strconv.ParseUint(value.Text, 10, 64)
		return parsed, err == nil && parsed <= max && strconv.FormatUint(parsed, 10) == value.Text
	}
	readInt32 := func(names ...string) (int32, bool) {
		value, present, valid := traceDBBlockArg(args, invalidKeys, names...)
		if !present || !valid || value.Datatype != 0 || value.Text != strings.TrimSpace(value.Text) {
			return 0, false
		}
		parsed, err := strconv.ParseInt(value.Text, 10, 32)
		return int32(parsed), err == nil && strconv.FormatInt(parsed, 10) == value.Text
	}
	readRequiredToken := func(names ...string) (string, bool) {
		value, present, valid := traceDBBlockArg(args, invalidKeys, names...)
		if !present || !valid || value.Datatype != 1 || value.Text != strings.TrimSpace(value.Text) || !validBlockRWBS(value.Text) {
			return "", false
		}
		return value.Text, true
	}
	readOptionalDisplay := func(forbidden rune, names ...string) (string, bool) {
		value, present, valid := traceDBBlockArg(args, invalidKeys, names...)
		if !valid {
			return "", false
		}
		if !present {
			return "", true
		}
		if value.Datatype != 1 || value.Text != strings.TrimSpace(value.Text) ||
			!traceDBSinglePhysicalLine(value.Text, true) || strings.ContainsRune(value.Text, forbidden) {
			return "", false
		}
		return value.Text, true
	}

	dev, ok := traceDBBlockDevice(args, invalidKeys, "dev", "dev_t")
	if !ok {
		return payload, false
	}
	payload.dev = dev
	sector, ok := readUint(math.MaxInt64, "sector", "lba")
	if !ok {
		return payload, false
	}
	payload.sector = sector
	nrSector, ok := readUint(math.MaxUint32, "nr_sector", "nr_sectors", "sectors")
	if !ok {
		return payload, false
	}
	payload.nrSector = uint32(nrSector)

	switch kind {
	case blockRenderBioComplete, blockRenderRQComplete:
		payload.errorCode, ok = readInt32("error", "ret", "res")
		if !ok {
			return payload, false
		}
		payload.rwbs, ok = readRequiredToken("rwbs", "rw", "op")
		if !ok {
			return payload, false
		}
		if kind == blockRenderRQComplete {
			payload.cmd, ok = readOptionalDisplay(')', "cmd", "opcode")
		}
	case blockRenderBioQueue:
		payload.rwbs, ok = readRequiredToken("rwbs", "rw", "op")
		if ok {
			payload.comm, ok = readOptionalDisplay(']', "comm")
		}
	case blockRenderRQInsert, blockRenderRQIssue:
		bytesValue, valid := readUint(math.MaxUint32, "bytes")
		if !valid {
			return payload, false
		}
		payload.bytes = uint32(bytesValue)
		payload.rwbs, ok = readRequiredToken("rwbs", "rw", "op")
		if ok {
			payload.comm, ok = readOptionalDisplay(']', "comm")
		}
		if ok {
			payload.cmd, ok = readOptionalDisplay(')', "cmd", "opcode")
		}
	case blockRenderBioRemap, blockRenderRQRemap:
		payload.oldDev, ok = traceDBBlockDevice(args, invalidKeys, "old_dev", "from")
		if !ok {
			return payload, false
		}
		payload.oldSector, ok = readUint(math.MaxInt64, "old_sector", "from_sector")
		if !ok {
			return payload, false
		}
		if kind == blockRenderRQRemap {
			nrBios, valid := readUint(math.MaxUint32, "nr_bios")
			if !valid {
				return payload, false
			}
			payload.nrBios = uint32(nrBios)
		}
		payload.rwbs, ok = readRequiredToken("rwbs", "rw", "op")
	}
	if !ok {
		return payload, false
	}
	return payload, true
}

func renderTraceDBBlockEvent(name string, args map[string]traceDBValue, invalidKeys map[string]bool) (string, bool) {
	payload, ok := decodeTraceDBBlockPayload(name, args, invalidKeys)
	if !ok {
		return "", false
	}
	canonicalName := strings.ToLower(strings.TrimSpace(name))
	return canonicalName + ": " + renderCanonicalBlockPayload(payload), true
}

// traceDBBlockArg accepts exactly one physical args key from an alias group.
// The shared args loader has already poisoned duplicate canonical keys; this
// layer additionally rejects cross-alias ambiguity instead of choosing one.
func traceDBBlockArg(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) (traceDBValue, bool, bool) {
	var selected traceDBValue
	present := false
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || invalidKeys[key] {
			return traceDBValue{}, false, false
		}
		value, exists := args[key]
		if !exists {
			continue
		}
		if present || !value.Valid {
			return traceDBValue{}, false, false
		}
		selected = value
		present = true
	}
	return selected, present, true
}

func traceDBBlockDevice(args map[string]traceDBValue, invalidKeys map[string]bool, names ...string) (uint32, bool) {
	value, present, valid := traceDBBlockArg(args, invalidKeys, names...)
	if !present || !valid || value.Text != strings.TrimSpace(value.Text) || value.Text == "" {
		return 0, false
	}
	if value.Datatype == 0 {
		packed, err := strconv.ParseUint(value.Text, 10, 32)
		return uint32(packed), err == nil && strconv.FormatUint(packed, 10) == value.Text
	}
	if value.Datatype != 1 {
		return 0, false
	}
	separator := byte(0)
	if strings.Count(value.Text, ",") == 1 && !strings.Contains(value.Text, ":") {
		separator = ','
	} else if strings.Count(value.Text, ":") == 1 && !strings.Contains(value.Text, ",") {
		separator = ':'
	}
	if separator == 0 {
		packed, err := strconv.ParseUint(value.Text, 10, 32)
		return uint32(packed), err == nil && traceDBBlockDecimal(value.Text)
	}
	majorText, minorText, _ := strings.Cut(value.Text, string(separator))
	if !traceDBBlockDecimal(majorText) || !traceDBBlockDecimal(minorText) {
		return 0, false
	}
	major, majorErr := strconv.ParseUint(majorText, 10, 12)
	minor, minorErr := strconv.ParseUint(minorText, 10, 20)
	if majorErr != nil || minorErr != nil {
		return 0, false
	}
	return uint32(major<<20 | minor), true
}

func traceDBBlockDecimal(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func renderProfilerBlockEvent(event profilerFtraceEventRecord) (string, string, bool, []string) {
	kind, name, ok := blockRenderKindForProfilerField(event.Field)
	if !ok {
		return "", "", false, nil
	}
	payload, degradations, valid := decodeProfilerBlockPayload(kind, event.Payload)
	if !valid {
		return name, "", false, degradations
	}
	return name, renderCanonicalBlockPayload(payload), true, degradations
}

// decodeProfilerBlockPayload interprets scalar wire absence as exact zero only
// because the pinned FtraceFieldParser calls every set_* method. Singular
// wrong-wire, duplicate and malformed fields are never defaulted.
func decodeProfilerBlockPayload(kind blockRenderKind, data []byte) (blockRenderPayload, []string, bool) {
	payload := blockRenderPayload{kind: kind}
	readUint := func(field int, max uint64) (uint64, []string, bool) {
		value, state, reason := protoScalarUint(data, field)
		if state == protoScalarInvalid {
			return 0, []string{fmt.Sprintf("core_field%d_%s", field, reason)}, false
		}
		// protoScalarAbsent is exact zero under this pinned producer profile.
		if value > max {
			return 0, []string{fmt.Sprintf("core_field%d_out_of_range", field)}, false
		}
		return value, nil, true
	}
	readRWBS := func(field int) (string, []string, bool) {
		value, state, reason := protoScalarString(data, field)
		if state == protoScalarInvalid {
			return "", []string{fmt.Sprintf("core_field%d_%s", field, reason)}, false
		}
		// String absence is the exact empty value; empty cannot author a block
		// request identity and is therefore a semantic hard failure.
		if !validBlockRWBS(value) {
			return "", []string{fmt.Sprintf("core_field%d_missing_or_invalid", field)}, false
		}
		return value, nil, true
	}
	readOptional := func(field int, label string, forbidden rune, degradations []string) (string, []string) {
		value, state, reason := protoScalarString(data, field)
		if state == protoScalarInvalid {
			return "", append(degradations, label+"_"+reason)
		}
		if value == "" {
			return "", degradations
		}
		if value != strings.TrimSpace(value) || !traceDBSinglePhysicalLine(value, true) || strings.ContainsRune(value, forbidden) {
			return "", append(degradations, label+"_unsafe_omitted")
		}
		return value, degradations
	}

	dev, reasons, ok := readUint(1, math.MaxUint32)
	if !ok {
		return payload, reasons, false
	}
	payload.dev = uint32(dev)
	sector, reasons, ok := readUint(2, math.MaxInt64)
	if !ok {
		return payload, reasons, false
	}
	payload.sector = sector
	nrSector, reasons, ok := readUint(3, math.MaxUint32)
	if !ok {
		return payload, reasons, false
	}
	payload.nrSector = uint32(nrSector)

	var degradations []string
	switch kind {
	case blockRenderBioComplete, blockRenderRQComplete:
		rawError, state, reason := protoScalarUint(data, 4)
		if state == protoScalarInvalid {
			return payload, []string{"core_field4_" + reason}, false
		}
		signedError := int64(rawError)
		if signedError < math.MinInt32 || signedError > math.MaxInt32 {
			return payload, []string{"core_field4_out_of_range"}, false
		}
		payload.errorCode = int32(signedError)
		payload.rwbs, reasons, ok = readRWBS(5)
		if !ok {
			return payload, reasons, false
		}
		if kind == blockRenderRQComplete {
			payload.cmd, degradations = readOptional(6, "cmd", ')', degradations)
		}
	case blockRenderBioQueue:
		payload.rwbs, reasons, ok = readRWBS(4)
		if !ok {
			return payload, reasons, false
		}
		payload.comm, degradations = readOptional(5, "comm", ']', degradations)
	case blockRenderBioRemap:
		oldDev, fieldReasons, fieldOK := readUint(4, math.MaxUint32)
		if !fieldOK {
			return payload, fieldReasons, false
		}
		payload.oldDev = uint32(oldDev)
		oldSector, fieldReasons, fieldOK := readUint(5, math.MaxInt64)
		if !fieldOK {
			return payload, fieldReasons, false
		}
		payload.oldSector = oldSector
		payload.rwbs, reasons, ok = readRWBS(6)
		if !ok {
			return payload, reasons, false
		}
	case blockRenderRQInsert, blockRenderRQIssue:
		bytesValue, fieldReasons, fieldOK := readUint(4, math.MaxUint32)
		if !fieldOK {
			return payload, fieldReasons, false
		}
		payload.bytes = uint32(bytesValue)
		payload.rwbs, reasons, ok = readRWBS(5)
		if !ok {
			return payload, reasons, false
		}
		payload.comm, degradations = readOptional(6, "comm", ']', degradations)
		payload.cmd, degradations = readOptional(7, "cmd", ')', degradations)
	case blockRenderRQRemap:
		oldDev, fieldReasons, fieldOK := readUint(4, math.MaxUint32)
		if !fieldOK {
			return payload, fieldReasons, false
		}
		payload.oldDev = uint32(oldDev)
		oldSector, fieldReasons, fieldOK := readUint(5, math.MaxInt64)
		if !fieldOK {
			return payload, fieldReasons, false
		}
		payload.oldSector = oldSector
		nrBios, fieldReasons, fieldOK := readUint(6, math.MaxUint32)
		if !fieldOK {
			return payload, fieldReasons, false
		}
		payload.nrBios = uint32(nrBios)
		payload.rwbs, reasons, ok = readRWBS(7)
		if !ok {
			return payload, reasons, false
		}
	default:
		return payload, nil, false
	}
	return payload, degradations, true
}
