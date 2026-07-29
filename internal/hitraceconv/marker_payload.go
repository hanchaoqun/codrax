package hitraceconv

import (
	"context"
	"encoding/binary"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// markerPayload is the source-neutral authority shared by direct ftrace and
// structured-profiler print/trace-marker bodies. The IP is provenance only:
// the canonical public body is the producer's marker bytes, never an
// address-carved reconstruction.
type markerPayload struct {
	Buffer    string
	IP        uint64
	IPPresent bool
}

func directMarkerNameGoverned(name string) bool {
	switch name {
	case "print", "tracing_mark_write", "tracing_mark_write_xacct", "xacct_tracing_mark_write":
		return true
	default:
		return false
	}
}

// builtinMarkerPayloadProvenance is the only bridge from the strict direct
// marker decoder to the built-in writer's advisory ledger. Exact trace marks
// remain ordinary known rows; only canonical non-mark print payloads receive
// the opaque advisory provenance.
func builtinMarkerPayloadProvenance(eventName string, payload markerPayload) builtinRowProvenance {
	switch eventName {
	case "print", "tracing_mark_write":
	default:
		return builtinRowProvenanceNone
	}
	if tracequery.IsTraceMarkPayload(payload.Buffer) {
		return builtinRowProvenanceNone
	}
	return builtinRowProvenanceOpaqueMarkerAdvisory
}

// directMarkerCStringDescriptorAllowed is deliberately event-aware. A zero
// sized field is the kernel's special CSTRING tail profile (not an empty fixed
// field), and only governed marker buf descriptors may use it. decodeEvent
// continues to ignore size-zero fields; the typed marker decoder reads the raw
// record tail from the declared offset.
func directMarkerCStringDescriptorAllowed(eventName string, field eventField) bool {
	bufferName := cleanFieldName(field.Name)
	if !directMarkerBufferFieldNameAllowed(eventName, bufferName) {
		return false
	}
	if field.Name != bufferName && field.Name != bufferName+"[]" {
		return false
	}
	switch normalizeFieldType(field.Type) {
	case "char", "char[]", "char []":
		return true
	default:
		return false
	}
}

func directMarkerBufferFieldName(eventName string) (string, bool) {
	switch eventName {
	case "print":
		return "buf", true
	case "tracing_mark_write":
		// OpenHarmony/hmtrace's PRINT_FMT_TRACING_MARK_WRITE names the
		// physical __data_loc field "buffer". This is an exact producer
		// profile, not a generic buffer|buf|str alias list.
		return "buffer", true
	default:
		return "", false
	}
}

func directMarkerBufferFieldNameAllowed(eventName, fieldName string) bool {
	switch eventName {
	case "print":
		return fieldName == "buf" || fieldName == "buffer"
	case "tracing_mark_write":
		return fieldName == "buffer"
	default:
		return false
	}
}

func directMarkerSelectedBufferProfile(ev decodedEvent) (string, bool, bool) {
	switch ev.format.Name {
	case "print":
		bufCount := directMarkerDeclarationCount(ev, "buf")
		bufferCount := directMarkerDeclarationCount(ev, "buffer")
		switch {
		case bufCount == 1 && bufferCount == 0:
			return "buf", true, true
		case bufCount == 0 && bufferCount == 1 && ev.format.ID >= 1<<15:
			// OpenHarmony's high event-ID print profile omits IP and calls
			// the compact data-loc carrier `buffer`. The event ID plus exact
			// mutually-exclusive declaration set is the producer contract.
			return "buffer", false, true
		default:
			return "", false, false
		}
	case "tracing_mark_write":
		if directMarkerDeclarationCount(ev, "buffer") == 1 {
			return "buffer", false, true
		}
	}
	return "", false, false
}

// directMarkerOpenHarmonyStructuredProfile is deliberately narrower than the
// generic trace-mark grammar. The official OpenHarmony raw parser selects this
// pid/name/start producer for the large print/tracing_mark_write record and
// treats a source pid of zero as "no TGID override": slice ownership then
// remains the physical common_pid emitter. Keep that producer fact separate
// from compact/text B|0, which receives no such identity authority.
func directMarkerOpenHarmonyStructuredProfile(ev decodedEvent) bool {
	switch ev.format.Name {
	case "print", "tracing_mark_write":
	default:
		return false
	}
	stringCarrierCount := 0
	for _, name := range []string{"buf", "buffer", "str", "trace"} {
		stringCarrierCount += directMarkerDeclarationCount(ev, name)
	}
	return stringCarrierCount == 0 &&
		directMarkerDeclarationCount(ev, "ip") == 0 &&
		directMarkerDeclarationCount(ev, "start") == 1 &&
		directMarkerDeclarationCount(ev, "pid") == 1 &&
		directMarkerDeclarationCount(ev, "name") == 1
}

func decodeDirectMarkerPayload(ev decodedEvent, content []byte) (markerPayload, bodyAdmission, string) {
	if !directMarkerNameGoverned(ev.format.Name) {
		return markerPayload{}, bodyUnsupported, ""
	}
	if !directMarkerDescriptorLayoutValid(ev, content) {
		return markerPayload{}, bodyRejected, "invalid_marker_descriptor_layout"
	}

	bufferName, bufferRequiresIP, hasBufferProfile := directMarkerSelectedBufferProfile(ev)
	bufferCount := 0
	if hasBufferProfile {
		bufferCount = directMarkerDeclarationCount(ev, bufferName)
	}
	stringCarrierCount := 0
	for _, name := range []string{"buf", "buffer", "str", "trace"} {
		stringCarrierCount += directMarkerDeclarationCount(ev, name)
	}
	startCount := directMarkerDeclarationCount(ev, "start")
	pidCount := directMarkerDeclarationCount(ev, "pid")
	nameCount := directMarkerDeclarationCount(ev, "name")
	ipCount := directMarkerDeclarationCount(ev, "ip")
	legacyCount := startCount + pidCount + nameCount

	switch {
	case stringCarrierCount > 0:
		if !hasBufferProfile || bufferCount != 1 || stringCarrierCount != 1 || legacyCount != 0 {
			return markerPayload{}, bodyRejected, "mixed_or_invalid_marker_profile"
		}
		payload := markerPayload{}
		if bufferRequiresIP {
			if ipCount != 1 {
				return markerPayload{}, bodyRejected, "missing_or_invalid_marker_ip"
			}
			ip, ok := directMarkerIP(ev)
			if !ok {
				return markerPayload{}, bodyRejected, "missing_or_invalid_marker_ip"
			}
			payload.IP, payload.IPPresent = ip, true
		} else if ipCount != 0 {
			return markerPayload{}, bodyRejected, "mixed_or_invalid_marker_profile"
		}
		raw, ok := directMarkerStringCarrier(ev, content, bufferName)
		if !ok {
			return markerPayload{}, bodyRejected, "missing_or_invalid_marker_buf"
		}
		buffer, ok := normalizeMarkerBuffer(raw)
		if !ok {
			return markerPayload{}, bodyRejected, "missing_or_invalid_marker_buf"
		}
		payload.Buffer = buffer
		return payload, bodyAdmitted, ""

	case legacyCount > 0:
		// OpenHarmony TraceStreamer routes both high-ID `print` and
		// `tracing_mark_write` through this exact pid/name/start producer
		// profile when the body is larger than the compact buf form. The
		// complete declared fields, not the event spelling alone, select this
		// mutually-exclusive profile.
		if ipCount != 0 || stringCarrierCount != 0 || startCount != 1 || pidCount != 1 || nameCount != 1 {
			return markerPayload{}, bodyRejected, "mixed_or_invalid_marker_profile"
		}
		start, ok := directMarkerStart(ev)
		if !ok || (start != 0 && start != 1) {
			return markerPayload{}, bodyRejected, "missing_or_invalid_marker_start"
		}
		pid, ok := directCoreSigned(ev, directWidths(4), "pid")
		if !ok || pid < 0 || pid > math.MaxInt32 {
			return markerPayload{}, bodyRejected, "missing_or_invalid_marker_pid"
		}
		nameField, _, nameFieldOK := directMarkerDeclaredField(ev, "name")
		if !nameFieldOK || nameField.Name != "name[64]" || nameField.Size != 64 {
			return markerPayload{}, bodyRejected, "missing_or_invalid_marker_name"
		}
		payload := markerPayload{}
		if start == 1 {
			rawName, nameOK := directMarkerStringCarrier(ev, content, "name")
			if !nameOK {
				return markerPayload{}, bodyRejected, "missing_or_invalid_marker_name"
			}
			name := string(rawName)
			if !traceDBSinglePhysicalLine(name, false) {
				return markerPayload{}, bodyRejected, "missing_or_invalid_marker_name"
			}
			payload.Buffer = "B|" + strconv.FormatInt(pid, 10) + "|" + name
		} else {
			// E carries no logical name. The exact descriptor and record
			// bounds were audited above; uninitialized/non-NUL name storage is
			// unobservable and must not suppress or alter this endpoint.
			payload.Buffer = "E|" + strconv.FormatInt(pid, 10) + "|"
		}
		return payload, bodyAdmitted, ""

	default:
		return markerPayload{}, bodyRejected, "missing_marker_profile"
	}
}

func renderCanonicalMarkerPayload(payload markerPayload) (string, bool) {
	value, valid, err := renderCanonicalMarkerPayloadContext(context.Background(), payload)
	return value, valid && err == nil
}

func renderCanonicalMarkerPayloadContext(ctx context.Context, payload markerPayload) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	valid, err := profilerSinglePhysicalLineStringContext(ctx, payload.Buffer, false)
	if err != nil {
		return "", false, err
	}
	if !valid {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}
		return "", false, nil
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return payload.Buffer, true, nil
}

func normalizeMarkerBuffer(raw []byte) (string, bool) {
	value, valid, err := normalizeMarkerBufferContext(context.Background(), raw)
	return value, valid && err == nil
}

func normalizeMarkerBufferContext(ctx context.Context, raw []byte) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	value := raw
	// OpenHarmony's canonical formatter removes at most one terminal LF. A
	// lone LF remains invalid, as do CR, internal/double LF and all controls.
	if len(value) > 1 && value[len(value)-1] == '\n' {
		value = value[:len(value)-1]
	}
	valid, err := profilerSinglePhysicalLineBytesContext(ctx, value, false)
	if err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !valid {
		return "", false, nil
	}
	cloned, err := profilerCloneBytesStringContext(ctx, value)
	if err != nil {
		return "", false, err
	}
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return cloned, true, nil
}

func directMarkerDeclarationCount(ev decodedEvent, cleanName string) int {
	count := 0
	for _, field := range ev.format.Fields {
		if cleanFieldName(field.Name) == cleanName {
			count++
		}
	}
	return count
}

func directMarkerDeclaredField(ev decodedEvent, cleanName string) (eventField, []byte, bool) {
	var selected eventField
	count := 0
	for _, field := range ev.format.Fields {
		if cleanFieldName(field.Name) != cleanName {
			continue
		}
		selected = field
		count++
	}
	if count != 1 {
		return eventField{}, nil, false
	}
	if selected.Size == 0 {
		return selected, nil, true
	}
	raw, ok := ev.fields[selected.Name]
	if !ok || len(raw) != selected.Size {
		return eventField{}, nil, false
	}
	return selected, raw, true
}

func directMarkerDescriptorLayoutValid(ev decodedEvent, content []byte) bool {
	if !directMarkerFormatLayoutValid(ev.format) {
		return false
	}
	for _, field := range ev.format.Fields {
		if field.Offset > len(content) {
			return false
		}
		if field.Size == 0 {
			if field.Offset >= len(content) {
				return false
			}
			continue
		}
		if field.Size > len(content)-field.Offset {
			return false
		}
	}
	return true
}

func directMarkerFormatLayoutValid(format eventFormat) bool {
	maxInt := int(^uint(0) >> 1)
	fixedTail := 0
	type interval struct {
		start int
		end   int
	}
	intervals := make([]interval, 0, len(format.Fields))
	for _, field := range format.Fields {
		if field.Offset < 0 || field.Size < 0 {
			return false
		}
		if field.Size == 0 {
			if !directMarkerCStringDescriptorAllowed(format.Name, field) {
				return false
			}
			continue
		}
		if field.Offset > maxInt-field.Size {
			return false
		}
		end := field.Offset + field.Size
		if end > fixedTail {
			fixedTail = end
		}
		intervals = append(intervals, interval{start: field.Offset, end: end})
	}
	sort.Slice(intervals, func(left, right int) bool {
		if intervals[left].start == intervals[right].start {
			return intervals[left].end < intervals[right].end
		}
		return intervals[left].start < intervals[right].start
	})
	for index := 1; index < len(intervals); index++ {
		if intervals[index].start < intervals[index-1].end {
			return false
		}
	}
	for _, field := range format.Fields {
		if field.Size == 0 && field.Offset < fixedTail {
			return false
		}
	}
	return true
}

func directMarkerFixedTail(ev decodedEvent) (int, bool) {
	maxInt := int(^uint(0) >> 1)
	fixedTail := 0
	for _, field := range ev.format.Fields {
		if field.Offset < 0 || field.Size < 0 || field.Offset > maxInt-field.Size {
			return 0, false
		}
		if field.Size == 0 {
			continue
		}
		if end := field.Offset + field.Size; end > fixedTail {
			fixedTail = end
		}
	}
	return fixedTail, true
}

func directMarkerStringCarrier(ev decodedEvent, content []byte, cleanName string) ([]byte, bool) {
	field, raw, ok := directMarkerDeclaredField(ev, cleanName)
	if !ok {
		return nil, false
	}
	// The pinned OpenHarmony parser classifies char/data_loc carriers without
	// consulting the descriptor's signed bit. Treat signed:0 and signed:1 as
	// the same byte-string profile; signedness remains a hard gate only for the
	// numeric start/pid/IP fields below.
	typeName := normalizeFieldType(field.Type)
	if typeName == "__data_loc char[]" || typeName == "__data_loc char []" {
		if field.Name != cleanName || field.Size != 4 || len(raw) != 4 {
			return nil, false
		}
		fixedTail, ok := directMarkerFixedTail(ev)
		if !ok {
			return nil, false
		}
		location := binary.LittleEndian.Uint32(raw)
		offset := int(location & 0xffff)
		length := int(location >> 16)
		if offset < fixedTail || length <= 0 || offset > len(content) || length > len(content)-offset {
			return nil, false
		}
		source := content[offset : offset+length]
		if bytesIndexNUL(source) != len(source)-1 {
			return nil, false
		}
		return source[:len(source)-1], true
	}

	if field.Size == 0 {
		if !directMarkerCStringDescriptorAllowed(ev.format.Name, field) {
			return nil, false
		}
		fixedTail, ok := directMarkerFixedTail(ev)
		if !ok || field.Offset < fixedTail || field.Offset >= len(content) {
			return nil, false
		}
		source := content[field.Offset:]
		nul := bytesIndexNUL(source)
		if nul < 0 {
			return nil, false
		}
		return source[:nul], true
	}

	if typeName != "char" || !directMarkerFixedArrayName(field.Name, cleanName, field.Size) || len(raw) != field.Size {
		return nil, false
	}
	nul := bytesIndexNUL(raw)
	if nul < 0 {
		return nil, false
	}
	return raw[:nul], true
}

func directMarkerFixedArrayName(rawName, cleanName string, size int) bool {
	prefix := cleanName + "["
	if !strings.HasPrefix(rawName, prefix) || !strings.HasSuffix(rawName, "]") {
		return false
	}
	declared, err := strconv.Atoi(rawName[len(prefix) : len(rawName)-1])
	return err == nil && declared > 0 && declared == size
}

func directMarkerIP(ev decodedEvent) (uint64, bool) {
	field, raw, ok := directMarkerDeclaredField(ev, "ip")
	if !ok || field.Size != len(raw) || !directCoreUnsignedWordTypeWidthAllowed(field, len(raw)) {
		return 0, false
	}
	return uintFromSupportedWidth(raw)
}

func directMarkerStart(ev decodedEvent) (int64, bool) {
	field, raw, ok := directMarkerDeclaredField(ev, "start")
	if !ok || field.Size != 4 || len(raw) != 4 {
		return 0, false
	}
	if field.Signed {
		if !directCoreSigned32TypeWidthAllowed(field, 4) {
			return 0, false
		}
		return intFromBytes(raw, true), true
	}
	if !directCoreUnsigned32TypeWidthAllowed(field, 4) {
		return 0, false
	}
	return int64(binary.LittleEndian.Uint32(raw)), true
}
