package hitraceconv

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
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

// directBlockRenderKindForName is the byte-exact raw-RMQ descriptor registry.
// SQL compatibility owns its own normalized adapter; case, whitespace and
// suffix drift in an events_format name are inventory, never direct authority.
func directBlockRenderKindForName(name string) (blockRenderKind, bool) {
	switch name {
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

func directBlockNameGoverned(name string) bool {
	_, ok := directBlockRenderKindForName(name)
	return ok
}

func directBlockPairEndpointName(name string) bool {
	switch name {
	case "block_bio_complete", "block_bio_queue", "block_rq_complete", "block_rq_issue":
		return true
	default:
		return false
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

type directBlockDecodeDecision struct {
	Payload       blockRenderPayload
	Degradations  []string
	Admission     bodyAdmission
	Reason        string
	IdentityKnown bool
	Governed      bool
}

// decodeDirectBlockPayload is deliberately presence-sensitive. Unlike proto3,
// an absent raw field is not an encoded zero. Aliases are accepted only as one
// unique physical authority, and byte counts never substitute for sectors.
func decodeDirectBlockPayload(ev decodedEvent, content []byte) (blockRenderPayload, []string, bool) {
	decision := decodeDirectBlockPayloadDecision(ev, content)
	return decision.Payload, decision.Degradations, decision.Admission == bodyAdmitted
}

func decodeDirectBlockPayloadDecision(ev decodedEvent, content []byte) directBlockDecodeDecision {
	kind, ok := directBlockRenderKindForName(ev.format.Name)
	if !ok {
		return directBlockDecodeDecision{Admission: bodyUnsupported}
	}
	payload := blockRenderPayload{kind: kind}
	reject := func(reasons []string, identityKnown bool) directBlockDecodeDecision {
		reason := "direct_block_payload_rejected"
		if len(reasons) > 0 && reasons[0] != "" {
			reason = reasons[0]
		}
		return directBlockDecodeDecision{
			Payload: payload, Degradations: reasons, Admission: bodyRejected,
			Reason: reason, IdentityKnown: identityKnown, Governed: true,
		}
	}

	readUint := func(label string, max uint64, widths []int, names ...string) (uint64, bool, []string) {
		value, reason, valid := directBlockUint(ev, label, max, widths, names...)
		if !valid {
			return 0, false, []string{reason}
		}
		return value, true, nil
	}
	dev, valid, reasons := readUint("dev", math.MaxUint32, []int{4, 8}, "dev", "dev_t")
	if !valid {
		return reject(reasons, false)
	}
	payload.dev = uint32(dev)
	sector, valid, reasons := readUint("sector", math.MaxInt64, []int{8}, "sector", "lba")
	if !valid {
		return reject(reasons, false)
	}
	payload.sector = sector
	nrSector, valid, reasons := readUint("nr_sector", math.MaxUint32, []int{4}, "nr_sector", "nr_sectors", "sectors")
	if !valid {
		return reject(reasons, false)
	}
	payload.nrSector = uint32(nrSector)
	rwbs, reason, valid := directBlockString(ev, content, true, "rwbs", "rw", "op")
	if !valid {
		return reject([]string{reason}, false)
	}
	if !validBlockRWBS(rwbs.value) {
		return reject([]string{"direct_rwbs_missing_or_invalid"}, false)
	}
	payload.rwbs = rwbs.value
	identityKnown := true

	switch kind {
	case blockRenderBioComplete, blockRenderRQComplete:
		errorCode, reason, valid := directBlockInt32(ev, "error", "error", "ret", "res")
		if !valid {
			return reject([]string{reason}, identityKnown)
		}
		payload.errorCode = errorCode
	case blockRenderRQInsert, blockRenderRQIssue:
		bytesValue, valid, reasons := readUint("bytes", math.MaxUint32, []int{4}, "bytes")
		if !valid {
			return reject(reasons, identityKnown)
		}
		payload.bytes = uint32(bytesValue)
	case blockRenderRQRemap, blockRenderBioRemap:
		oldDev, valid, reasons := readUint("old_dev", math.MaxUint32, []int{4, 8}, "old_dev", "from")
		if !valid {
			return reject(reasons, identityKnown)
		}
		payload.oldDev = uint32(oldDev)
		oldSector, valid, reasons := readUint("old_sector", math.MaxInt64, []int{8}, "old_sector", "from_sector")
		if !valid {
			return reject(reasons, identityKnown)
		}
		payload.oldSector = oldSector
		if kind == blockRenderRQRemap {
			nrBios, valid, reasons := readUint("nr_bios", math.MaxUint32, []int{4}, "nr_bios")
			if !valid {
				return reject(reasons, identityKnown)
			}
			payload.nrBios = uint32(nrBios)
		}
	}

	var (
		cmdValue, commValue directBlockStringValue
		degradations        []string
	)
	if kind == blockRenderRQComplete || kind == blockRenderRQInsert || kind == blockRenderRQIssue {
		cmdValue, degradations = directBlockOptionalDisplay(ev, content, "cmd", ')', degradations, "cmd")
	}
	if kind == blockRenderBioQueue || kind == blockRenderRQInsert || kind == blockRenderRQIssue {
		commValue, degradations = directBlockOptionalDisplay(ev, content, "comm", ']', degradations, "comm")
	}
	// A __data_loc hard-key witness is authoritative only when its target bytes
	// are not simultaneously claimed by another admitted dynamic field. Merely
	// reparsing the canonical text cannot detect that physical aliasing.
	if directBlockRangesOverlap(rwbs.dynamicRange, cmdValue.dynamicRange) ||
		directBlockRangesOverlap(rwbs.dynamicRange, commValue.dynamicRange) {
		return reject([]string{"direct_rwbs_dynamic_overlap"}, false)
	}
	// cmd/comm are display-only. Ambiguity confined to those fields degrades the
	// presentation but must not erase an otherwise exact request identity.
	if directBlockRangesOverlap(cmdValue.dynamicRange, commValue.dynamicRange) {
		cmdValue.value = ""
		commValue.value = ""
		degradations = append(degradations, "direct_cmd_dynamic_overlap", "direct_comm_dynamic_overlap")
	}
	payload.cmd = cmdValue.value
	payload.comm = commValue.value
	return directBlockDecodeDecision{
		Payload: payload, Degradations: degradations, Admission: bodyAdmitted,
		IdentityKnown: identityKnown, Governed: true,
	}
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
	selectedIndex := -1
	var selected eventField
	for index, field := range ev.format.Fields {
		if cleanNameIn(cleanFieldName(field.Name), names) {
			selected = field
			selectedIndex = index
			count++
		}
	}
	switch {
	case count == 0:
		return eventField{}, nil, "direct_" + label + "_missing_field", false
	case count > 1:
		return eventField{}, nil, "direct_" + label + "_duplicate_alias", false
	}
	if !directDescriptorFieldIsolated(ev, selectedIndex) {
		return eventField{}, nil, "direct_" + label + "_overlapping_field", false
	}
	raw, present := ev.fields[selected.Name]
	if !present || len(raw) != selected.Size {
		return eventField{}, nil, "direct_" + label + "_truncated_field", false
	}
	return selected, raw, "", true
}

func blockDirectNumericTypeAllowed(field eventField) bool {
	lowerType := normalizeFieldType(field.Type)
	if lowerType == "" || !numericFieldTypeAllowed(field) || strings.ContainsAny(lowerType, "*[]") ||
		strings.ContainsAny(field.Name, "[]") {
		return false
	}
	for _, denied := range []string{"struct ", "union ", "enum ", "float", "double", "bool", "__data_loc", "__rel_loc"} {
		if strings.Contains(lowerType, denied) {
			return false
		}
	}
	return true
}

type directBlockStringValue struct {
	value        string
	dynamicRange directPairByteRange
}

func directBlockString(ev decodedEvent, content []byte, required bool, names ...string) (directBlockStringValue, string, bool) {
	label := names[0]
	count := 0
	selectedIndex := -1
	var selected eventField
	for index, field := range ev.format.Fields {
		if cleanNameIn(cleanFieldName(field.Name), names) {
			selected = field
			selectedIndex = index
			count++
		}
	}
	if count == 0 {
		if required {
			return directBlockStringValue{}, "direct_" + label + "_missing_field", false
		}
		return directBlockStringValue{}, "", true
	}
	if count > 1 {
		return directBlockStringValue{}, "direct_" + label + "_duplicate_alias", false
	}
	if !directDescriptorFieldIsolated(ev, selectedIndex) {
		return directBlockStringValue{}, "direct_" + label + "_overlapping_field", false
	}
	raw, present := ev.fields[selected.Name]
	if !present || len(raw) != selected.Size {
		return directBlockStringValue{}, "direct_" + label + "_truncated_field", false
	}
	lowerType := normalizeFieldType(selected.Type)
	isDataLoc := lowerType == "__data_loc char[]"
	if !isDataLoc && !directBlockFixedCharArray(selected) {
		return directBlockStringValue{}, "direct_" + label + "_wrong_type", false
	}
	decoded := directBlockStringValue{}
	if isDataLoc {
		if selected.Size != 4 || len(raw) != 4 {
			return directBlockStringValue{}, "direct_" + label + "_wrong_width", false
		}
		fixedTail, ok := directDescriptorFixedTail(ev)
		if !ok {
			return directBlockStringValue{}, "direct_" + label + "_invalid_descriptor", false
		}
		loc := binary.LittleEndian.Uint32(raw)
		offset := int(loc & 0xffff)
		length := int(loc >> 16)
		if offset < fixedTail || length <= 0 || offset > len(content) || length > len(content)-offset {
			return directBlockStringValue{}, "direct_" + label + "_truncated_field", false
		}
		stringRaw := content[offset : offset+length]
		decoded.dynamicRange = directPairByteRange{start: offset, end: offset + length}
		if stringRaw[len(stringRaw)-1] != 0 || bytesIndexNUL(stringRaw[:len(stringRaw)-1]) >= 0 {
			return decoded, "direct_" + label + "_truncated_field", false
		}
		decoded.value = string(stringRaw[:len(stringRaw)-1])
	} else {
		if selected.Size <= 0 {
			return directBlockStringValue{}, "direct_" + label + "_wrong_width", false
		}
		nul := bytesIndexNUL(raw)
		if nul < 0 {
			return directBlockStringValue{}, "direct_" + label + "_truncated_field", false
		}
		raw = raw[:nul]
		decoded.value = string(raw)
	}
	if !utf8.ValidString(decoded.value) || !traceDBSinglePhysicalLine(decoded.value, true) ||
		decoded.value != strings.TrimSpace(decoded.value) {
		return decoded, "direct_" + label + "_invalid_string", false
	}
	return decoded, "", true
}

func directBlockRangesOverlap(left, right directPairByteRange) bool {
	return left.end > left.start && right.end > right.start && left.start < right.end && right.start < left.end
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

func directBlockOptionalDisplay(ev decodedEvent, content []byte, label string, forbidden rune, degradations []string, names ...string) (directBlockStringValue, []string) {
	value, reason, ok := directBlockString(ev, content, false, names...)
	if !ok {
		value.value = ""
		return value, append(degradations, reason)
	}
	if value.value != "" && strings.ContainsRune(value.value, forbidden) {
		value.value = ""
		return value, append(degradations, "direct_"+label+"_unsafe_omitted")
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

type profilerFtraceBlockFieldRole uint8

const (
	profilerFtraceBlockFieldUnknown profilerFtraceBlockFieldRole = iota
	profilerFtraceBlockFieldDev
	profilerFtraceBlockFieldSector
	profilerFtraceBlockFieldNRSector
	profilerFtraceBlockFieldError
	profilerFtraceBlockFieldRWBS
	profilerFtraceBlockFieldBytes
	profilerFtraceBlockFieldOldDev
	profilerFtraceBlockFieldOldSector
	profilerFtraceBlockFieldNRBios
	profilerFtraceBlockFieldComm
	profilerFtraceBlockFieldCmd
)

type profilerFtraceBlockFieldState struct {
	Count       uint8 // saturates at two: absent, singular, or duplicate
	WrongWire   bool
	Malformed   bool
	UintValue   uint64
	RawValue    []byte
	StringValue string
}

// profilerBlockRWBSBytesContext applies the common byte-backed token gate
// before the source-neutral block operation grammar. The protobuf view stays
// byte-backed until the value is known to be safe for pair/render publication.
func profilerBlockRWBSBytesContext(ctx context.Context, raw []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	valid, err := profilerSingleTokenBytesContext(ctx, raw)
	if err != nil || !valid || len(raw) > 32 {
		return false, err
	}
	for _, value := range raw {
		if (value < 'A' || value > 'Z') && (value < 'a' || value > 'z') {
			return false, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return true, nil
}

// profilerBlockDisplayBytesContext preserves the historical display omission
// grammar without constructing a string for rejected evidence. The shared
// physical-line scanner owns UTF-8/control/line and size validation; this
// adapter adds exact strings.TrimSpace edge parity plus the formatter-specific
// closing delimiter check.
func profilerBlockDisplayBytesContext(ctx context.Context, raw []byte, forbidden byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	valid, err := profilerSinglePhysicalLineBytesContext(ctx, raw, true)
	if err != nil || !valid {
		return false, err
	}
	if len(raw) > 0 {
		first, firstWidth := utf8.DecodeRune(raw)
		last, lastWidth := utf8.DecodeLastRune(raw)
		if firstWidth <= 0 || lastWidth <= 0 {
			return false, &traceDBOutputInvariantError{Reason: "profiler_block_display_rune_width_invalid"}
		}
		if unicode.IsSpace(first) || unicode.IsSpace(last) {
			return false, nil
		}
	}
	processed := uint64(0)
	for start := 0; start < len(raw); {
		end := min(start+profilerContextByteCheckpointBytes, len(raw))
		if err := profilerByteContextCheckpoint(ctx, &processed, uint64(end-start)); err != nil {
			return false, err
		}
		for _, value := range raw[start:end] {
			if value == forbidden {
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

func profilerFtraceBlockFieldSchema(eventField, payloadField int) (profilerFtraceBlockFieldRole, int, bool) {
	switch payloadField {
	case 1:
		return profilerFtraceBlockFieldDev, 0, true
	case 2:
		return profilerFtraceBlockFieldSector, 0, true
	case 3:
		return profilerFtraceBlockFieldNRSector, 0, true
	}
	switch eventField {
	case 202:
		switch payloadField {
		case 4:
			return profilerFtraceBlockFieldError, 0, true
		case 5:
			return profilerFtraceBlockFieldRWBS, 2, true
		}
	case 204:
		switch payloadField {
		case 4:
			return profilerFtraceBlockFieldRWBS, 2, true
		case 5:
			return profilerFtraceBlockFieldComm, 2, true
		}
	case 205:
		switch payloadField {
		case 4:
			return profilerFtraceBlockFieldOldDev, 0, true
		case 5:
			return profilerFtraceBlockFieldOldSector, 0, true
		case 6:
			return profilerFtraceBlockFieldRWBS, 2, true
		}
	case 209:
		switch payloadField {
		case 4:
			return profilerFtraceBlockFieldError, 0, true
		case 5:
			return profilerFtraceBlockFieldRWBS, 2, true
		case 6:
			return profilerFtraceBlockFieldCmd, 2, true
		}
	case 210, 211:
		switch payloadField {
		case 4:
			return profilerFtraceBlockFieldBytes, 0, true
		case 5:
			return profilerFtraceBlockFieldRWBS, 2, true
		case 6:
			return profilerFtraceBlockFieldComm, 2, true
		case 7:
			return profilerFtraceBlockFieldCmd, 2, true
		}
	case 212:
		switch payloadField {
		case 4:
			return profilerFtraceBlockFieldOldDev, 0, true
		case 5:
			return profilerFtraceBlockFieldOldSector, 0, true
		case 6:
			return profilerFtraceBlockFieldNRBios, 0, true
		case 7:
			return profilerFtraceBlockFieldRWBS, 2, true
		}
	}
	return profilerFtraceBlockFieldUnknown, 0, false
}

func profilerFtraceBlockDisplayRole(role profilerFtraceBlockFieldRole) bool {
	return role == profilerFtraceBlockFieldComm || role == profilerFtraceBlockFieldCmd
}

const profilerFtraceBlockIssuesPerEvent = 2

type profilerFtraceBlockIssueSet struct {
	Count  uint8
	Issues [profilerFtraceBlockIssuesPerEvent]profilerFtraceEventIssue
}

func (set *profilerFtraceBlockIssueSet) validate(eventField int) error {
	if set == nil || int(set.Count) > len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_block_issue_count_invalid"}
	}
	if _, _, governed := blockRenderKindForProfilerField(eventField); !governed {
		return &traceDBOutputInvariantError{Reason: "profiler_block_issue_schema_invalid"}
	}
	lastDisplayField := uint8(0)
	for index, issue := range set.Issues {
		if index >= int(set.Count) {
			if issue != (profilerFtraceEventIssue{}) {
				return &traceDBOutputInvariantError{Reason: "profiler_block_issue_count_invalid"}
			}
			continue
		}
		if issue.Kind < profilerFtraceEventIssueBlockPayloadMalformedWire ||
			issue.Kind > profilerFtraceEventIssueBlockCmdUnsafeOmitted ||
			!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
			return &traceDBOutputInvariantError{Reason: "profiler_block_issue_schema_invalid"}
		}
		for prior := 0; prior < index; prior++ {
			if set.Issues[prior] == issue {
				return &traceDBOutputInvariantError{Reason: "profiler_block_issue_duplicate"}
			}
			if set.Issues[prior].PayloadField == issue.PayloadField {
				return &traceDBOutputInvariantError{Reason: "profiler_block_issue_endpoint_conflict"}
			}
		}
		if issue.Severity == profilerFtraceEventIssueHardReject {
			if set.Count != 1 {
				return &traceDBOutputInvariantError{Reason: "profiler_block_issue_arm_invalid"}
			}
			continue
		}
		if issue.PayloadField <= lastDisplayField {
			return &traceDBOutputInvariantError{Reason: "profiler_block_issue_order_invalid"}
		}
		lastDisplayField = issue.PayloadField
	}
	return nil
}

func (set *profilerFtraceBlockIssueSet) add(eventField int, issue profilerFtraceEventIssue) error {
	if err := set.validate(eventField); err != nil {
		return err
	}
	if issue.Kind < profilerFtraceEventIssueBlockPayloadMalformedWire ||
		issue.Kind > profilerFtraceEventIssueBlockCmdUnsafeOmitted ||
		!issue.validFor(eventField) || issue.Severity != issue.expectedSeverity() {
		return &traceDBOutputInvariantError{Reason: "profiler_block_issue_schema_invalid"}
	}
	for index := 0; index < int(set.Count); index++ {
		if set.Issues[index] == issue {
			return &traceDBOutputInvariantError{Reason: "profiler_block_issue_duplicate"}
		}
		if set.Issues[index].PayloadField == issue.PayloadField {
			return &traceDBOutputInvariantError{Reason: "profiler_block_issue_endpoint_conflict"}
		}
	}
	if int(set.Count) == len(set.Issues) {
		return &traceDBOutputInvariantError{Reason: "profiler_block_issue_overflow"}
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

func (set *profilerFtraceBlockIssueSet) addFixed(eventField int, kind profilerFtraceEventIssueKind) error {
	issue, ok := profilerFtraceEventFixedIssue(eventField, kind)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_block_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceBlockIssueSet) addPayload(eventField int, kind profilerFtraceEventIssueKind, payloadField int) error {
	issue, ok := profilerFtraceEventPayloadIssue(eventField, kind, payloadField)
	if !ok {
		return &traceDBOutputInvariantError{Reason: "profiler_block_issue_schema_invalid"}
	}
	return set.add(eventField, issue)
}

func (set *profilerFtraceBlockIssueSet) checked(eventField int) ([]profilerFtraceEventIssue, error) {
	if err := set.validate(eventField); err != nil {
		return nil, err
	}
	return append([]profilerFtraceEventIssue(nil), set.Issues[:int(set.Count)]...), nil
}

func profilerStructuredBlockPairFamily(eventField int) profilerPairAdmission {
	if profilerStructuredPairEventField(pairRenderBlock, eventField) {
		return profilerPairAdmission{Kind: pairRenderBlock, Governed: true}
	}
	return profilerPairAdmission{}
}

func profilerFtraceBlockPairKeyRole(role profilerFtraceBlockFieldRole) bool {
	switch role {
	case profilerFtraceBlockFieldDev, profilerFtraceBlockFieldSector,
		profilerFtraceBlockFieldNRSector, profilerFtraceBlockFieldRWBS:
		return true
	default:
		return false
	}
}

// profilerStructuredBlockPairAdmission derives the elapsed-request identity
// from the very same fixed field-state populated by the renderer's one wire
// walk. Rendering intentionally preserves the producer's proto3 absent->zero
// compatibility, but absence is never a pairing witness: all four hard-key
// fields and the physical common_pid must be explicitly singular.
func profilerStructuredBlockPairAdmission(
	event profilerFtraceEventRecord,
	canonicalName string,
	fields [8]profilerFtraceBlockFieldState,
	walkErr error,
	payloadAdmitted bool,
) profilerPairAdmission {
	pair := profilerStructuredBlockPairFamily(event.Field)
	if !pair.Governed {
		return pair
	}

	hardKeySafe := true
	if walkErr != nil {
		var decodeErr *protoFieldDecodeError
		if !errors.As(walkErr, &decodeErr) || !decodeErr.Terminal || !decodeErr.FieldKnown {
			hardKeySafe = false
		} else if role, _, known := profilerFtraceBlockFieldSchema(event.Field, decodeErr.Field); known && profilerFtraceBlockPairKeyRole(role) {
			hardKeySafe = false
		}
	}

	var dev, sector, nrSector uint64
	var rwbs string
	for payloadField := 1; payloadField <= 7 && hardKeySafe; payloadField++ {
		role, expectedWire, known := profilerFtraceBlockFieldSchema(event.Field, payloadField)
		if !known || !profilerFtraceBlockPairKeyRole(role) {
			continue
		}
		state := fields[payloadField]
		if state.Count != 1 || state.WrongWire || state.Malformed {
			hardKeySafe = false
			break
		}
		switch role {
		case profilerFtraceBlockFieldDev:
			if expectedWire != 0 || state.UintValue > math.MaxUint32 {
				hardKeySafe = false
			} else {
				dev = state.UintValue
			}
		case profilerFtraceBlockFieldSector:
			if expectedWire != 0 || state.UintValue > math.MaxInt64 {
				hardKeySafe = false
			} else {
				sector = state.UintValue
			}
		case profilerFtraceBlockFieldNRSector:
			if expectedWire != 0 || state.UintValue > math.MaxUint32 {
				hardKeySafe = false
			} else {
				nrSector = state.UintValue
			}
		case profilerFtraceBlockFieldRWBS:
			if expectedWire != 2 || !validBlockRWBS(state.StringValue) {
				hardKeySafe = false
			} else {
				rwbs = state.StringValue
			}
		}
	}

	headerTID := int64(-1)
	pair.HeaderOwnerKnown = event.HeaderOwnerKnown && event.HeaderOwnerPresent &&
		event.PID >= 0 && event.PID <= math.MaxInt32
	if pair.HeaderOwnerKnown {
		headerTID = event.PID
	}
	pair.Verdict = fingerprintPairingEndpoint(tracequery.PairingEndpointTypedInput{
		Name: canonicalName, HeaderTID: headerTID,
		BlockIdentityKnown: hardKeySafe, BlockPayloadAdmissionKnown: true,
		BlockPayloadAdmitted: payloadAdmitted,
		BlockDeviceNumber:    dev, BlockDeviceNumeric: hardKeySafe,
		BlockOperation: rwbs, BlockSector: int64(sector), BlockLength: int64(nrSector),
	})
	pair.LaneKnown = pair.HeaderOwnerKnown && pair.Verdict.KeyKnown &&
		pair.Verdict.EmitterKnown && pair.Verdict.SemanticKey != ""
	if pair.LaneKnown {
		pair.Lane = pair.Verdict.SemanticKey
	}
	pair.Admitted = pair.LaneKnown && pair.Verdict.PayloadAdmitted && pair.Verdict.EmitterAdmitted
	return pair
}

// decodeProfilerBlockPayloadWithTypedAuditIntoContext is the single structured
// block payload authority. Every known endpoint is observed during one bounded
// wire walk; diagnostics and the optional pair sidecar are derived from that
// same byte-backed state. pairOut is a prospective transaction: cancellation,
// deadline and invariant errors preserve the caller's prior value, while one
// ordinary source verdict publishes the complete sidecar exactly once.
func decodeProfilerBlockPayloadWithTypedAuditIntoContext(
	ctx context.Context,
	event profilerFtraceEventRecord,
	pairOut *profilerPairAdmission,
) (
	payloadResult blockRenderPayload,
	admissionResult bodyAdmission,
	setResult profilerFtraceBlockIssueSet,
	handledResult bool,
	errResult error,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	kind, canonicalName, governed := blockRenderKindForProfilerField(event.Field)
	prospectivePair := profilerStructuredBlockPairFamily(event.Field)
	defer func() {
		if errResult != nil {
			payloadResult = blockRenderPayload{}
			admissionResult = bodyUnsupported
			setResult = profilerFtraceBlockIssueSet{}
			handledResult = governed
			return
		}
		if err := ctx.Err(); err != nil {
			payloadResult = blockRenderPayload{}
			admissionResult = bodyUnsupported
			setResult = profilerFtraceBlockIssueSet{}
			handledResult = governed
			errResult = err
			return
		}
		if pairOut != nil {
			*pairOut = prospectivePair
		}
	}()
	if err := ctx.Err(); err != nil {
		return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, governed, err
	}
	if !governed {
		return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, nil
	}
	rejectFixed := func(issueKind profilerFtraceEventIssueKind) (
		blockRenderPayload, bodyAdmission, profilerFtraceBlockIssueSet, bool, error,
	) {
		var set profilerFtraceBlockIssueSet
		err := set.addFixed(event.Field, issueKind)
		return blockRenderPayload{}, bodyRejected, set, true, err
	}
	rejectPayload := func(issueKind profilerFtraceEventIssueKind, payloadField int) (
		blockRenderPayload, bodyAdmission, profilerFtraceBlockIssueSet, bool, error,
	) {
		var set profilerFtraceBlockIssueSet
		err := set.addPayload(event.Field, issueKind, payloadField)
		return blockRenderPayload{}, bodyRejected, set, true, err
	}

	descriptor, ok := profilerFtraceEventDescriptors[event.Field]
	if !ok {
		return blockRenderPayload{}, bodyRejected, profilerFtraceBlockIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "missing_block_descriptor"}
	}
	if descriptor.Field != event.Field {
		return blockRenderPayload{}, bodyRejected, profilerFtraceBlockIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "mismatched_block_descriptor_field"}
	}
	descriptorKind, descriptorKnown := blockRenderKindForName(descriptor.Name)
	if descriptor.Family != "block" || descriptor.Name != canonicalName || !descriptorKnown || descriptorKind != kind {
		return blockRenderPayload{}, bodyRejected, profilerFtraceBlockIssueSet{}, true,
			&traceDBOutputInvariantError{Reason: "invalid_block_descriptor"}
	}

	var fields [8]profilerFtraceBlockFieldState
	walkErr := walkProfilerProtoFieldsContext(ctx, event.Payload, func(payloadField int, wire int, raw []byte, value uint64) error {
		role, expectedWire, known := profilerFtraceBlockFieldSchema(event.Field, payloadField)
		if !known {
			return nil
		}
		state := &fields[payloadField]
		if state.Count < 2 {
			state.Count++
		}
		if wire != expectedWire {
			state.WrongWire = true
			return nil
		}
		if role == profilerFtraceBlockFieldRWBS || profilerFtraceBlockDisplayRole(role) {
			state.RawValue = raw
		} else {
			state.UintValue = value
		}
		return nil
	})
	if errors.Is(walkErr, context.Canceled) || errors.Is(walkErr, context.DeadlineExceeded) {
		return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, true, walkErr
	}
	if _, invariant := traceDBOutputInvariantReason(walkErr); walkErr != nil && invariant {
		return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, true, walkErr
	}
	// Pair identity consumes the same validated RWBS clone as rendering. It is
	// prepared before source-error classification so a display-only malformed
	// tail preserves the historical hard-key pairing verdict.
	for payloadField := 1; payloadField <= 7; payloadField++ {
		role, expectedWire, known := profilerFtraceBlockFieldSchema(event.Field, payloadField)
		if !known || role != profilerFtraceBlockFieldRWBS {
			continue
		}
		state := &fields[payloadField]
		if expectedWire != 2 || state.Count != 1 || state.WrongWire || state.Malformed {
			continue
		}
		valid, err := profilerBlockRWBSBytesContext(ctx, state.RawValue)
		if err != nil {
			return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, err
		}
		if !valid {
			continue
		}
		state.StringValue, err = profilerCloneBytesStringContext(ctx, state.RawValue)
		if err != nil {
			return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, err
		}
	}
	prospectivePair = profilerStructuredBlockPairAdmission(event, canonicalName, fields, walkErr, false)
	if walkErr != nil {
		var decodeErr *protoFieldDecodeError
		if !errors.As(walkErr, &decodeErr) {
			return blockRenderPayload{}, bodyRejected, profilerFtraceBlockIssueSet{}, true,
				&traceDBOutputInvariantError{Reason: "profiler_block_wire_error_untyped"}
		}
		role, _, known := profilerFtraceBlockFieldSchema(event.Field, decodeErr.Field)
		localized := decodeErr.Failure == protoFieldDecodeMalformedValue ||
			decodeErr.Failure == protoFieldDecodeUnsupportedWire
		if !localized || !decodeErr.FieldKnown || !known ||
			(profilerFtraceBlockDisplayRole(role) && !decodeErr.Terminal) {
			return rejectFixed(profilerFtraceEventIssueBlockPayloadMalformedWire)
		}
		if !profilerFtraceBlockDisplayRole(role) {
			return rejectPayload(profilerFtraceEventIssueBlockFieldMalformedWire, decodeErr.Field)
		}
		fields[decodeErr.Field].Malformed = true
	}

	for payloadField := 1; payloadField <= 7; payloadField++ {
		role, _, known := profilerFtraceBlockFieldSchema(event.Field, payloadField)
		if !known || profilerFtraceBlockDisplayRole(role) {
			continue
		}
		state := fields[payloadField]
		switch {
		case state.WrongWire:
			return rejectPayload(profilerFtraceEventIssueBlockFieldWrongWire, payloadField)
		case state.Count > 1:
			return rejectPayload(profilerFtraceEventIssueBlockFieldDuplicate, payloadField)
		}
	}

	payload := blockRenderPayload{kind: kind}
	for payloadField := 1; payloadField <= 7; payloadField++ {
		role, _, known := profilerFtraceBlockFieldSchema(event.Field, payloadField)
		if !known || profilerFtraceBlockDisplayRole(role) {
			continue
		}
		state := fields[payloadField]
		switch role {
		case profilerFtraceBlockFieldDev:
			if state.UintValue > math.MaxUint32 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.dev = uint32(state.UintValue)
		case profilerFtraceBlockFieldSector:
			if state.UintValue > math.MaxInt64 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.sector = state.UintValue
		case profilerFtraceBlockFieldNRSector:
			if state.UintValue > math.MaxUint32 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.nrSector = uint32(state.UintValue)
		case profilerFtraceBlockFieldError:
			signed := int64(state.UintValue)
			if signed < math.MinInt32 || signed > math.MaxInt32 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.errorCode = int32(signed)
		case profilerFtraceBlockFieldRWBS:
			if !validBlockRWBS(state.StringValue) {
				return rejectPayload(profilerFtraceEventIssueBlockFieldMissingOrInvalid, payloadField)
			}
			payload.rwbs = state.StringValue
		case profilerFtraceBlockFieldBytes:
			if state.UintValue > math.MaxUint32 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.bytes = uint32(state.UintValue)
		case profilerFtraceBlockFieldOldDev:
			if state.UintValue > math.MaxUint32 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.oldDev = uint32(state.UintValue)
		case profilerFtraceBlockFieldOldSector:
			if state.UintValue > math.MaxInt64 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.oldSector = state.UintValue
		case profilerFtraceBlockFieldNRBios:
			if state.UintValue > math.MaxUint32 {
				return rejectPayload(profilerFtraceEventIssueBlockFieldOutOfRange, payloadField)
			}
			payload.nrBios = uint32(state.UintValue)
		default:
			return blockRenderPayload{}, bodyRejected, profilerFtraceBlockIssueSet{}, true,
				&traceDBOutputInvariantError{Reason: "profiler_block_schema_invalid"}
		}
	}

	var set profilerFtraceBlockIssueSet
	for payloadField := 1; payloadField <= 7; payloadField++ {
		role, _, known := profilerFtraceBlockFieldSchema(event.Field, payloadField)
		if !known || !profilerFtraceBlockDisplayRole(role) {
			continue
		}
		state := &fields[payloadField]
		value := ""
		var issueKind profilerFtraceEventIssueKind
		issuePresent := true
		switch role {
		case profilerFtraceBlockFieldComm:
			switch {
			case state.Malformed:
				issueKind = profilerFtraceEventIssueBlockCommMalformedWire
			case state.WrongWire:
				issueKind = profilerFtraceEventIssueBlockCommWrongWire
			case state.Count > 1:
				issueKind = profilerFtraceEventIssueBlockCommDuplicate
			default:
				valid, err := profilerBlockDisplayBytesContext(ctx, state.RawValue, ']')
				if err != nil {
					return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, err
				}
				if !valid {
					issueKind = profilerFtraceEventIssueBlockCommUnsafeOmitted
				} else if state.Count == 1 {
					value, err = profilerCloneBytesStringContext(ctx, state.RawValue)
					if err != nil {
						return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, err
					}
					state.StringValue = value
					issuePresent = false
				} else {
					issuePresent = false
				}
			}
			payload.comm = value
		case profilerFtraceBlockFieldCmd:
			switch {
			case state.Malformed:
				issueKind = profilerFtraceEventIssueBlockCmdMalformedWire
			case state.WrongWire:
				issueKind = profilerFtraceEventIssueBlockCmdWrongWire
			case state.Count > 1:
				issueKind = profilerFtraceEventIssueBlockCmdDuplicate
			default:
				valid, err := profilerBlockDisplayBytesContext(ctx, state.RawValue, ')')
				if err != nil {
					return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, err
				}
				if !valid {
					issueKind = profilerFtraceEventIssueBlockCmdUnsafeOmitted
				} else if state.Count == 1 {
					value, err = profilerCloneBytesStringContext(ctx, state.RawValue)
					if err != nil {
						return blockRenderPayload{}, bodyUnsupported, profilerFtraceBlockIssueSet{}, false, err
					}
					state.StringValue = value
					issuePresent = false
				} else {
					issuePresent = false
				}
			}
			payload.cmd = value
		}
		if issuePresent {
			if err := set.addFixed(event.Field, issueKind); err != nil {
				return blockRenderPayload{}, bodyRejected, profilerFtraceBlockIssueSet{}, true, err
			}
		}
	}
	prospectivePair = profilerStructuredBlockPairAdmission(event, canonicalName, fields, walkErr, true)
	return payload, bodyAdmitted, set, true, nil
}

// decodeProfilerBlockPayloadWithTypedAuditInto preserves the pre-Context local
// API. Context-aware production callers use the authority above directly.
func decodeProfilerBlockPayloadWithTypedAuditInto(event profilerFtraceEventRecord, pairOut *profilerPairAdmission) (
	blockRenderPayload, bodyAdmission, profilerFtraceBlockIssueSet, bool, error,
) {
	return decodeProfilerBlockPayloadWithTypedAuditIntoContext(context.Background(), event, pairOut)
}

// decodeProfilerBlockPayloadWithTypedAudit preserves the local renderer API;
// production pairing uses the IntoContext variant so rendering and admission
// still share exactly one protobuf walk.
func decodeProfilerBlockPayloadWithTypedAudit(event profilerFtraceEventRecord) (
	blockRenderPayload, bodyAdmission, profilerFtraceBlockIssueSet, bool, error,
) {
	return decodeProfilerBlockPayloadWithTypedAuditIntoContext(context.Background(), event, nil)
}

func renderProfilerFtraceBlockEventWithTypedAudit(event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error,
) {
	return renderProfilerFtraceBlockEventWithTypedAuditContext(context.Background(), event)
}

func renderProfilerFtraceBlockEventWithTypedAuditContext(ctx context.Context, event profilerFtraceEventRecord) (
	name, body string, ok bool, issues []profilerFtraceEventIssue, handled bool, err error,
) {
	_, _, governed := blockRenderKindForProfilerField(event.Field)
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		// Route ownership is a schema fact, not a successfully decoded value.
		// Preserve it across governed cancellation so callers cannot fall
		// through to a second renderer after this authority stopped.
		return "", "", false, nil, governed, err
	}
	payload, admission, set, handled, err :=
		decodeProfilerBlockPayloadWithTypedAuditIntoContext(ctx, event, nil)
	if !handled || err != nil {
		return "", "", false, nil, handled, err
	}
	name, body, ok, issues, err = finalizeProfilerFtraceBlockEventWithTypedAuditContext(ctx, event, payload, admission, set)
	return name, body, ok, issues, true, err
}

func finalizeProfilerFtraceBlockEventWithTypedAudit(
	event profilerFtraceEventRecord,
	payload blockRenderPayload,
	admission bodyAdmission,
	set profilerFtraceBlockIssueSet,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	return finalizeProfilerFtraceBlockEventWithTypedAuditContext(
		context.Background(), event, payload, admission, set,
	)
}

func finalizeProfilerFtraceBlockEventWithTypedAuditContext(
	ctx context.Context,
	event profilerFtraceEventRecord,
	payload blockRenderPayload,
	admission bodyAdmission,
	set profilerFtraceBlockIssueSet,
) (name, body string, ok bool, issues []profilerFtraceEventIssue, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", "", false, nil, err
	}
	defer func() {
		if err != nil {
			return
		}
		if contextErr := ctx.Err(); contextErr != nil {
			name = ""
			body = ""
			ok = false
			issues = nil
			err = contextErr
		}
	}()
	issues, err = set.checked(event.Field)
	if err != nil {
		return "", "", false, nil, err
	}
	switch admission {
	case bodyRejected:
		if payload != (blockRenderPayload{}) || len(issues) != 1 ||
			issues[0].Severity != profilerFtraceEventIssueHardReject {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "profiler_block_rejected_verdict_invalid"}
		}
		return "", "", false, issues, nil
	case bodyAdmitted:
		for _, issue := range issues {
			if issue.Severity != profilerFtraceEventIssueAdmittedDisplay {
				return "", "", false, nil,
					&traceDBOutputInvariantError{Reason: "profiler_block_admitted_verdict_invalid"}
			}
		}
		kind, canonicalName, governed := blockRenderKindForProfilerField(event.Field)
		if !governed || payload.kind != kind {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_block_payload"}
		}
		body = renderCanonicalBlockPayload(payload)
		if body == "" {
			return "", "", false, nil,
				&traceDBOutputInvariantError{Reason: "invalid_canonical_block_payload"}
		}
		canonicalValid, canonicalErr := profilerCanonicalLineValidContext(ctx, event, canonicalName, body)
		if canonicalErr != nil {
			return "", "", false, nil, canonicalErr
		}
		if !canonicalValid {
			var canonical profilerFtraceBlockIssueSet
			if err := canonical.addFixed(event.Field, profilerFtraceEventIssueBlockInvalidCanonicalLine); err != nil {
				return "", "", false, nil, err
			}
			issues, err = canonical.checked(event.Field)
			return "", "", false, issues, err
		}
		return canonicalName, body, true, issues, nil
	default:
		return "", "", false, nil,
			&traceDBOutputInvariantError{Reason: "profiler_block_admission_invalid"}
	}
}
