package hitraceconv

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// pairRenderKind is the closed direct-RMQ payload family whose exact rows can
// mint an elapsed Workqueue/DMA observation downstream. Inventory siblings do
// not enter this registry.
type pairRenderKind uint8

const (
	pairRenderUnknown pairRenderKind = iota
	pairRenderWorkqueue
	pairRenderDMAFence
)

type pairRenderPayload struct {
	Kind      pairRenderKind
	Name      string
	Workqueue *pairWorkqueuePayload
	DMAFence  *pairDMAFencePayload
}

type pairWorkqueuePayload struct {
	Work             uint64
	WorkKnown        bool
	Function         uint64
	FunctionKnown    bool
	FunctionRequired bool
}

type pairDMAFencePayload struct {
	Driver        string
	DriverKnown   bool
	Timeline      string
	TimelineKnown bool
	NumberBits    uint8
	Context       uint64
	ContextKnown  bool
	Seqno         uint64
	SeqnoKnown    bool
}

func directPairNameGoverned(name string) bool {
	_, ok := directPairKindForName(name)
	return ok
}

func directPairKindForName(name string) (pairRenderKind, bool) {
	switch name {
	case "workqueue_execute_start", "workqueue_execute_end":
		return pairRenderWorkqueue, true
	case "dma_fence_wait_start", "dma_fence_wait_end":
		return pairRenderDMAFence, true
	default:
		return pairRenderUnknown, false
	}
}

// decodeDirectPairPayload is the sole direct-RMQ descriptor decoder for the
// four exact pair-critical rows. On rejection it preserves only independently
// proven hard-key components so the complete-capture barrier can quarantine an
// exact lane; it never renders a partial payload.
func decodeDirectPairPayload(ev decodedEvent, content []byte) (pairRenderPayload, bodyAdmission, string) {
	kind, governed := directPairKindForName(ev.format.Name)
	if !governed {
		return pairRenderPayload{}, bodyUnsupported, ""
	}
	payload := pairRenderPayload{Kind: kind, Name: ev.format.Name}
	switch kind {
	case pairRenderWorkqueue:
		work := &pairWorkqueuePayload{FunctionRequired: ev.format.Name == "workqueue_execute_start"}
		payload.Workqueue = work
		value, width, ok := directPairPointer(ev, "work", 8, 0)
		if ok {
			work.Work, work.WorkKnown = value, true
		}
		if !ok || value == 0 {
			return payload, bodyRejected, "missing_or_invalid_work"
		}
		if directPairHasAlias(ev, "addr", "address") {
			// addr/address are alternative hard-key declarations. Once either
			// appears beside work, the physical row no longer proves which key
			// the producer meant; do not quarantine only the canonical-looking
			// work lane and leave the alias lane available for cross-hole rescue.
			work.Work, work.WorkKnown = 0, false
			return payload, bodyRejected, "mixed_or_invalid_workqueue_profile"
		}
		if directPairHasAlias(ev, "func") {
			return payload, bodyRejected, "mixed_or_invalid_workqueue_profile"
		}

		functionDeclarations := directPairCleanDeclarationCount(ev, "function")
		switch {
		case functionDeclarations == 0:
			if ev.format.Name != "workqueue_execute_end" ||
				!directPairExactPayloadFieldRoster(ev, "work") ||
				!directPairLegacyWorkqueueEndPrintFmt(ev.format.PrintFmt) {
				return payload, bodyRejected, "missing_or_invalid_function"
			}
			// Linux legacy execute_end is an independent work-only profile.
			return payload, bodyAdmitted, ""
		case functionDeclarations != 1:
			return payload, bodyRejected, "missing_or_invalid_function"
		}
		if !directPairExactPayloadFieldRoster(ev, "work", "function") {
			return payload, bodyRejected, "mixed_or_invalid_workqueue_profile"
		}
		work.FunctionRequired = true

		function, functionWidth, functionOK := directPairPointer(ev, "function", 8+width, width)
		if functionOK {
			work.Function, work.FunctionKnown = function, true
		}
		if !functionOK || function == 0 || functionWidth != width {
			return payload, bodyRejected, "missing_or_invalid_function"
		}
		return payload, bodyAdmitted, ""

	case pairRenderDMAFence:
		dma := &pairDMAFencePayload{NumberBits: 32}
		payload.DMAFence = dma
		if directPairHasAlias(ev, "drv", "tl", "ctx", "sequence", "id") {
			return payload, bodyRejected, "mixed_or_invalid_dma_fence_profile"
		}
		if !directPairExactPayloadFieldRoster(ev, "driver", "timeline", "context", "seqno") {
			return payload, bodyRejected, "mixed_or_invalid_dma_fence_profile"
		}
		driver, driverRange, driverOK := directPairDataLocScalar(ev, content, "driver", 8)
		timeline, timelineRange, timelineOK := directPairDataLocScalar(ev, content, "timeline", 12)
		context, contextOK := directPairUint32(ev, "context", 16)
		seqno, seqnoOK := directPairUint32(ev, "seqno", 20)
		if !driverOK || !timelineOK || !contextOK || !seqnoOK {
			return payload, bodyRejected, "missing_or_invalid_dma_fence_payload"
		}
		if driverRange.start < timelineRange.end && timelineRange.start < driverRange.end {
			return payload, bodyRejected, "overlapping_dma_fence_strings"
		}
		// Commit the hard tuple only after every component and the relation
		// between both dynamic ranges is proven. A rejected row must never
		// manufacture a different exact lane from overlapping bytes.
		dma.Driver, dma.DriverKnown = driver, true
		dma.Timeline, dma.TimelineKnown = timeline, true
		dma.Context, dma.ContextKnown = context, true
		dma.Seqno, dma.SeqnoKnown = seqno, true
		return payload, bodyAdmitted, ""
	}
	return payload, bodyRejected, "invalid_pair_kind"
}

func directPairLegacyWorkqueueEndPrintFmt(printFmt string) bool {
	// Linux v5.4's workqueue_work event class emits this exact tracefs
	// contract. A merely absent `function` token is not evidence of that
	// legacy profile: arbitrary/corrupt PrintFmt text cannot authorize a
	// current descriptor with a lost function field to downgrade.
	return strings.TrimSpace(printFmt) == `"work struct %p", REC->work`
}

func renderCanonicalPairPayload(payload pairRenderPayload) (string, bool) {
	if payload.Name == "" {
		return "", false
	}
	kind, governed := directPairKindForName(payload.Name)
	if !governed || kind != payload.Kind {
		return "", false
	}
	switch payload.Kind {
	case pairRenderWorkqueue:
		item := payload.Workqueue
		if item == nil || !item.WorkKnown || item.Work == 0 {
			return "", false
		}
		body := fmt.Sprintf("work struct 0x%x", item.Work)
		if item.FunctionKnown {
			if item.Function == 0 {
				return "", false
			}
			body += fmt.Sprintf(": function 0x%x", item.Function)
		} else if item.FunctionRequired {
			return "", false
		}
		return body, true
	case pairRenderDMAFence:
		item := payload.DMAFence
		if item == nil || !item.DriverKnown || !item.TimelineKnown || !item.ContextKnown || !item.SeqnoKnown ||
			!directPairScalarValid(item.Driver) || !directPairScalarValid(item.Timeline) {
			return "", false
		}
		switch item.NumberBits {
		case 32:
			if item.Context > math.MaxUint32 || item.Seqno > math.MaxUint32 {
				return "", false
			}
		case 64:
		default:
			return "", false
		}
		return fmt.Sprintf("driver=%s timeline=%s context=%d seqno=%d", item.Driver, item.Timeline, item.Context, item.Seqno), true
	default:
		return "", false
	}
}

// fingerprintPairingEndpoint is the one package-local bridge to tracequery's
// family/phase/key authority. SQL raw and direct RMQ adapters must call this
// wrapper rather than growing source-specific hard-key constructors.
func fingerprintPairingEndpoint(input tracequery.PairingEndpointTypedInput) tracequery.PairingEndpointVerdict {
	return tracequery.FingerprintPairingEndpoint(input)
}

func pairingEndpointLaneKey(verdict tracequery.PairingEndpointVerdict, source string) (string, bool) {
	return verdict.LaneKey(source)
}

func pairPayloadTypedInput(payload pairRenderPayload, headerTID int64) tracequery.PairingEndpointTypedInput {
	input := tracequery.PairingEndpointTypedInput{Name: payload.Name, HeaderTID: headerTID}
	switch payload.Kind {
	case pairRenderWorkqueue:
		if payload.Workqueue != nil && payload.Workqueue.WorkKnown {
			input.WorkAddress = payload.Workqueue.Work
			input.WorkAddressKnown = true
		}
	case pairRenderDMAFence:
		if payload.DMAFence != nil {
			item := payload.DMAFence
			if item.DriverKnown {
				input.Driver = item.Driver
			}
			if item.TimelineKnown {
				input.Timeline = item.Timeline
			}
			if item.ContextKnown {
				input.ContextNumber, input.ContextNumberKnown = item.Context, true
			}
			if item.SeqnoKnown {
				input.SeqnoNumber, input.SeqnoNumberKnown = item.Seqno, true
			}
		}
	}
	return input
}

func pairPayloadWireParity(payload pairRenderPayload, body string, headerTID int64, typed tracequery.PairingEndpointVerdict) bool {
	wire := tracequery.DecodePairingEndpoint(payload.Name, body, headerTID)
	return wire.Recognized == typed.Recognized && wire.KeyKnown == typed.KeyKnown &&
		wire.PayloadAdmitted == typed.PayloadAdmitted && wire.Family == typed.Family &&
		wire.Phase == typed.Phase && wire.SemanticKey == typed.SemanticKey &&
		wire.EmitterKnown == typed.EmitterKnown && wire.EmitterAdmitted == typed.EmitterAdmitted
}

type directPairLineAudit struct {
	Governed         bool
	Kind             pairRenderKind
	Payload          pairRenderPayload
	HeaderTID        int64
	HeaderOwnerKnown bool
	Verdict          tracequery.PairingEndpointVerdict
	EndpointAdmitted bool
}

func newDirectPairLineAudit(ev decodedEvent, payload pairRenderPayload) directPairLineAudit {
	kind, governed := directPairKindForName(ev.format.Name)
	if !governed {
		return directPairLineAudit{}
	}
	headerTID, ownerKnown := directPairHeaderTID(ev)
	fingerprintTID := int64(-1)
	if ownerKnown {
		fingerprintTID = headerTID
	}
	if payload.Name == "" {
		payload = pairRenderPayload{Kind: kind, Name: ev.format.Name}
	}
	verdict := fingerprintPairingEndpoint(pairPayloadTypedInput(payload, fingerprintTID))
	return directPairLineAudit{
		Governed: governed, Kind: kind, Payload: payload,
		HeaderTID: headerTID, HeaderOwnerKnown: ownerKnown, Verdict: verdict,
	}
}

func directPairHeaderTID(ev decodedEvent) (int64, bool) {
	index, field, _, ok := directPairExactField(ev, "common_pid")
	if !ok || field.Name != "common_pid" || !directPairFieldIsolated(ev, index) {
		return 0, false
	}
	value, ok := directFtraceCommonField(ev, "common_pid", 4, 4, true)
	if !ok || value < 0 || value > math.MaxInt32 {
		return 0, false
	}
	return value, true
}

// traceDBRawPairPayload adapts an already type-checked SQL argset into the
// same source-neutral canonical payload used by direct RMQ. SQL aliases remain
// an adapter concern and never expand the exact direct producer profile.
func traceDBRawPairPayload(name string, args map[string]traceDBValue, invalidKeys map[string]bool) (pairRenderPayload, bool) {
	kind, governed := directPairKindForName(name)
	if !governed {
		return pairRenderPayload{}, false
	}
	payload := pairRenderPayload{Kind: kind, Name: name}
	switch kind {
	case pairRenderWorkqueue:
		work, workOK := traceDBRawTypedPointer(args, invalidKeys, "work", "addr", "address")
		item := &pairWorkqueuePayload{Work: work, WorkKnown: workOK}
		payload.Workqueue = item
		functionPresent := traceDBRawAliasPresence(args, "function", "func")
		if functionPresent {
			function, functionOK := traceDBRawTypedPointer(args, invalidKeys, "function", "func")
			if !functionOK {
				return payload, false
			}
			item.Function, item.FunctionKnown = function, true
		}
		if !workOK {
			return payload, false
		}
		return payload, true
	case pairRenderDMAFence:
		driver, driverOK := traceDBRawTypedWireText(args, invalidKeys, true, "driver")
		timeline, timelineOK := traceDBRawTypedWireText(args, invalidKeys, true, "timeline")
		context, contextOK := traceDBRawTypedUnsignedInt(args, invalidKeys, true, "context")
		seqno, seqnoOK := traceDBRawTypedUnsignedInt(args, invalidKeys, true, "seqno")
		payload.DMAFence = &pairDMAFencePayload{
			Driver: driver, DriverKnown: driverOK, Timeline: timeline, TimelineKnown: timelineOK,
			NumberBits: 64, Context: context, ContextKnown: contextOK, Seqno: seqno, SeqnoKnown: seqnoOK,
		}
		return payload, driverOK && timelineOK && contextOK && seqnoOK
	default:
		return payload, false
	}
}

type directPairByteRange struct{ start, end int }

func directPairPointer(ev decodedEvent, name string, expectedOffset, expectedWidth int) (uint64, int, bool) {
	index, field, raw, ok := directPairExactField(ev, name)
	if !ok || field.Offset != expectedOffset || field.Signed || (field.Size != 4 && field.Size != 8) ||
		(expectedWidth != 0 && field.Size != expectedWidth) || len(raw) != field.Size ||
		!directPairFieldIsolated(ev, index) {
		return 0, 0, false
	}
	typeName := normalizeFieldType(field.Type)
	if typeName != "void *" && typeName != "void*" {
		return 0, 0, false
	}
	value, ok := uintFromSupportedWidth(raw)
	return value, field.Size, ok
}

func directPairUint32(ev decodedEvent, name string, expectedOffset int) (uint64, bool) {
	index, field, raw, ok := directPairExactField(ev, name)
	if !ok || field.Offset != expectedOffset || field.Signed || field.Size != 4 || len(raw) != 4 ||
		normalizeFieldType(field.Type) != "unsigned int" || !directPairFieldIsolated(ev, index) {
		return 0, false
	}
	return uint64(binary.LittleEndian.Uint32(raw)), true
}

func directPairDataLocScalar(ev decodedEvent, content []byte, name string, expectedOffset int) (string, directPairByteRange, bool) {
	index, field, raw, ok := directPairExactField(ev, name)
	// signed describes the char element profile for __data_loc char[]; the
	// four physical locator bytes themselves are always decoded as a u32 tuple.
	if !ok || field.Offset != expectedOffset || field.Size != 4 || len(raw) != 4 ||
		normalizeFieldType(field.Type) != "__data_loc char[]" || !directPairFieldIsolated(ev, index) {
		return "", directPairByteRange{}, false
	}
	fixedTail, ok := directPairFixedTail(ev)
	if !ok {
		return "", directPairByteRange{}, false
	}
	location := binary.LittleEndian.Uint32(raw)
	offset := int(location & 0xffff)
	length := int(location >> 16)
	if offset < fixedTail || length <= 0 || offset > len(content) || length > len(content)-offset {
		return "", directPairByteRange{}, false
	}
	dynamic := content[offset : offset+length]
	if len(dynamic) < 2 || dynamic[len(dynamic)-1] != 0 || bytesIndexNUL(dynamic[:len(dynamic)-1]) >= 0 {
		return "", directPairByteRange{}, false
	}
	value := string(dynamic[:len(dynamic)-1])
	if !directPairScalarValid(value) {
		return "", directPairByteRange{}, false
	}
	return value, directPairByteRange{start: offset, end: offset + length}, true
}

func directPairScalarValid(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) || value != strings.TrimSpace(value) ||
		strings.HasSuffix(value, ",") || !traceDBSinglePhysicalLine(value, false) {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || r == '=' || r == '\'' || r == '"' {
			return false
		}
	}
	return true
}

func directPairExactField(ev decodedEvent, name string) (int, eventField, []byte, bool) {
	exactIndex := -1
	cleanCount := 0
	for index, field := range ev.format.Fields {
		if cleanFieldName(field.Name) != name {
			continue
		}
		cleanCount++
		if field.Name == name {
			exactIndex = index
		}
	}
	if cleanCount != 1 || exactIndex < 0 {
		return -1, eventField{}, nil, false
	}
	field := ev.format.Fields[exactIndex]
	raw, ok := ev.fields[field.Name]
	if !ok || len(raw) != field.Size {
		return -1, eventField{}, nil, false
	}
	return exactIndex, field, raw, true
}

func directPairFieldIsolated(ev decodedEvent, selectedIndex int) bool {
	if selectedIndex < 0 || selectedIndex >= len(ev.format.Fields) {
		return false
	}
	selected := ev.format.Fields[selectedIndex]
	if selected.Offset < 0 || selected.Size <= 0 || selected.Offset > math.MaxInt-selected.Size {
		return false
	}
	selectedEnd := selected.Offset + selected.Size
	for index, other := range ev.format.Fields {
		if index == selectedIndex {
			continue
		}
		if other.Offset < 0 || other.Size <= 0 || other.Offset > math.MaxInt-other.Size {
			return false
		}
		otherEnd := other.Offset + other.Size
		if selected.Offset < otherEnd && other.Offset < selectedEnd {
			return false
		}
	}
	return true
}

func directPairFixedTail(ev decodedEvent) (int, bool) {
	tail := 0
	for _, field := range ev.format.Fields {
		if field.Offset < 0 || field.Size <= 0 || field.Offset > math.MaxInt-field.Size {
			return 0, false
		}
		if end := field.Offset + field.Size; end > tail {
			tail = end
		}
	}
	return tail, true
}

func directPairCleanDeclarationCount(ev decodedEvent, name string) int {
	count := 0
	for _, field := range ev.format.Fields {
		if cleanFieldName(field.Name) == name {
			count++
		}
	}
	return count
}

func directPairExactPayloadFieldRoster(ev decodedEvent, expected ...string) bool {
	counts := make(map[string]int, len(expected))
	for _, name := range expected {
		counts[name] = 0
	}
	for _, field := range ev.format.Fields {
		name := cleanFieldName(field.Name)
		if strings.HasPrefix(name, "common_") {
			continue
		}
		if field.Name != name {
			return false
		}
		if _, ok := counts[name]; !ok {
			return false
		}
		counts[name]++
	}
	for _, count := range counts {
		if count != 1 {
			return false
		}
	}
	return true
}

func directPairHasAlias(ev decodedEvent, names ...string) bool {
	for _, field := range ev.format.Fields {
		if cleanNameIn(cleanFieldName(field.Name), names) {
			return true
		}
	}
	return false
}
