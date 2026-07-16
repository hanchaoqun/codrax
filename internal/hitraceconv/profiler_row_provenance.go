package hitraceconv

import "strconv"

// profilerPairEndpointSlot is the closed endpoint identity carried by
// Profiler pair-critical rows. It replaces free-form pairTable as the typed
// authority; display names are derived from this roster in one direction.
type profilerPairEndpointSlot uint8

const (
	profilerPairEndpointNone profilerPairEndpointSlot = iota
	profilerPairEndpointMMCRequestStart
	profilerPairEndpointMMCRequestDone
	profilerPairEndpointF2FSSyncFileEnter
	profilerPairEndpointF2FSSyncFileExit
	profilerPairEndpointF2FSDirectIOEnter
	profilerPairEndpointF2FSDirectIOExit
	profilerPairEndpointF2FSWriteBegin
	profilerPairEndpointF2FSWriteEnd
	profilerPairEndpointBlockBIOQueue
	profilerPairEndpointBlockBIOComplete
	profilerPairEndpointBlockRQIssue
	profilerPairEndpointBlockRQComplete
	profilerPairEndpointSlotCount
)

type profilerPairEndpointDescriptor struct {
	slot            profilerPairEndpointSlot
	kind            pairRenderKind
	name            string
	structuredField int
}

// profilerPairFamilyEndpointCapacity is the largest closed endpoint roster in
// one Profiler pair family. Family-local ledgers use this fixed width instead
// of retaining a dynamic endpoint map per exact lane.
const profilerPairFamilyEndpointCapacity = 6

type profilerPairFamilyEndpointOrdinal uint8

// profilerPairFamilyEndpointRange is derived from the numeric endpoint ABI:
// every family occupies one contiguous range in profilerPairEndpointRoster.
// Keeping only the first slot and count avoids a second endpoint descriptor
// authority while still allowing O(1) fixed-ledger indexing.
func profilerPairFamilyEndpointRange(kind pairRenderKind) (profilerPairEndpointSlot, uint8, bool) {
	switch kind {
	case pairRenderMMC:
		return profilerPairEndpointMMCRequestStart, 2, true
	case pairRenderF2FS:
		return profilerPairEndpointF2FSSyncFileEnter, 6, true
	case pairRenderBlock:
		return profilerPairEndpointBlockBIOQueue, 4, true
	default:
		return profilerPairEndpointNone, 0, false
	}
}

func profilerPairFamilyEndpointCount(kind pairRenderKind) (uint8, bool) {
	_, count, ok := profilerPairFamilyEndpointRange(kind)
	return count, ok
}

func (slot profilerPairEndpointSlot) familyOrdinal(kind pairRenderKind) (profilerPairFamilyEndpointOrdinal, bool) {
	first, count, ok := profilerPairFamilyEndpointRange(kind)
	if !ok || slot < first {
		return 0, false
	}
	offset := uint8(slot - first)
	if offset >= count || offset >= profilerPairFamilyEndpointCapacity {
		return 0, false
	}
	descriptor, descriptorOK := slot.descriptor()
	if !descriptorOK || descriptor.kind != kind {
		return 0, false
	}
	return profilerPairFamilyEndpointOrdinal(offset), true
}

func profilerPairEndpointForFamilyOrdinal(kind pairRenderKind, ordinal profilerPairFamilyEndpointOrdinal) (profilerPairEndpointSlot, bool) {
	first, count, ok := profilerPairFamilyEndpointRange(kind)
	if !ok || uint8(ordinal) >= count || uint8(ordinal) >= profilerPairFamilyEndpointCapacity {
		return profilerPairEndpointNone, false
	}
	slot := first + profilerPairEndpointSlot(ordinal)
	descriptor, descriptorOK := slot.descriptor()
	if !descriptorOK || descriptor.kind != kind {
		return profilerPairEndpointNone, false
	}
	return slot, true
}

var profilerPairEndpointRoster = [...]profilerPairEndpointDescriptor{
	{profilerPairEndpointMMCRequestStart, pairRenderMMC, "mmc_request_start", 4016},
	{profilerPairEndpointMMCRequestDone, pairRenderMMC, "mmc_request_done", 4015},
	{profilerPairEndpointF2FSSyncFileEnter, pairRenderF2FS, "f2fs_sync_file_enter", 4009},
	{profilerPairEndpointF2FSSyncFileExit, pairRenderF2FS, "f2fs_sync_file_exit", 4010},
	{profilerPairEndpointF2FSDirectIOEnter, pairRenderF2FS, "f2fs_direct_IO_enter", 0},
	{profilerPairEndpointF2FSDirectIOExit, pairRenderF2FS, "f2fs_direct_IO_exit", 0},
	{profilerPairEndpointF2FSWriteBegin, pairRenderF2FS, "f2fs_write_begin", 4011},
	{profilerPairEndpointF2FSWriteEnd, pairRenderF2FS, "f2fs_write_end", 4012},
	{profilerPairEndpointBlockBIOQueue, pairRenderBlock, "block_bio_queue", 204},
	{profilerPairEndpointBlockBIOComplete, pairRenderBlock, "block_bio_complete", 202},
	{profilerPairEndpointBlockRQIssue, pairRenderBlock, "block_rq_issue", 211},
	{profilerPairEndpointBlockRQComplete, pairRenderBlock, "block_rq_complete", 209},
}

func profilerPairEndpointForName(name string) (profilerPairEndpointSlot, bool) {
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.name == name {
			return descriptor.slot, true
		}
	}
	return profilerPairEndpointNone, false
}

func profilerPairEndpointForStructuredField(field int) (profilerPairEndpointSlot, bool) {
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.structuredField != 0 && descriptor.structuredField == field {
			return descriptor.slot, true
		}
	}
	return profilerPairEndpointNone, false
}

func (slot profilerPairEndpointSlot) descriptor() (profilerPairEndpointDescriptor, bool) {
	if slot == profilerPairEndpointNone || slot >= profilerPairEndpointSlotCount {
		return profilerPairEndpointDescriptor{}, false
	}
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.slot == slot {
			return descriptor, true
		}
	}
	return profilerPairEndpointDescriptor{}, false
}

// profilerPairPublisherSlot is a source-local closed publisher identity. The
// four outer slots map exactly to profilerPluginRoute; SessionJSON has its own
// slot and never pretends to be a plugin message.
type profilerPairPublisherSlot uint8

const (
	profilerPairPublisherNone profilerPairPublisherSlot = iota
	profilerPairPublisherExactFtrace
	profilerPairPublisherBytrace
	profilerPairPublisherNoncanonicalFtrace
	profilerPairPublisherOtherText
	profilerPairPublisherSession
	profilerPairPublisherSlotCount
)

func profilerPairPublisherForRoute(route profilerPluginRoute) (profilerPairPublisherSlot, bool) {
	switch route {
	case profilerPluginRouteExactFtrace:
		return profilerPairPublisherExactFtrace, true
	case profilerPluginRouteBytrace:
		return profilerPairPublisherBytrace, true
	case profilerPluginRouteNoncanonicalFtrace:
		return profilerPairPublisherNoncanonicalFtrace, true
	case profilerPluginRouteOtherText:
		return profilerPairPublisherOtherText, true
	default:
		return profilerPairPublisherNone, false
	}
}

func (slot profilerPairPublisherSlot) valid() bool {
	return slot < profilerPairPublisherSlotCount
}

func (slot profilerPairPublisherSlot) textCapable() bool {
	return slot == profilerPairPublisherExactFtrace || slot == profilerPairPublisherBytrace ||
		slot == profilerPairPublisherOtherText
}

type profilerPairRowProvenanceFlags uint8

const (
	profilerPairRowProvenanceText profilerPairRowProvenanceFlags = 1 << iota
	profilerPairRowProvenanceStructured
	profilerPairRowProvenanceFlagMask = profilerPairRowProvenanceText | profilerPairRowProvenanceStructured
)

// profilerTraceClass is the closed tracequery verdict minted only after the
// source-local publisher/text/structured provenance has been staged. Keeping
// it independent from pair flags prevents an ordinary structured row or a
// Session text row (both legitimately flags=0) from reopening a provenance
// inference path.
type profilerTraceClass uint8

const (
	profilerTraceClassNone profilerTraceClass = iota
	profilerTraceClassStructuredKnown
	profilerTraceClassTextKnown
	profilerTraceClassTextIntentionalUnknown
	profilerTraceClassCount
)

func (class profilerTraceClass) valid() bool {
	return class < profilerTraceClassCount
}

func (class profilerTraceClass) known() bool {
	return class == profilerTraceClassStructuredKnown || class == profilerTraceClassTextKnown
}

// profilerPairRowProvenance is deliberately sixteen bytes wide. renderedRow
// stores its scalar members in existing alignment holes; the wire and the
// source-order proof use this canonical aggregate.
type profilerPairRowProvenance struct {
	LaneID             uint32
	TextMessageOrdinal uint32
	PairKind           pairRenderKind
	EndpointSlot       profilerPairEndpointSlot
	PublisherSlot      profilerPairPublisherSlot
	Flags              profilerPairRowProvenanceFlags
	TraceClass         profilerTraceClass
}

// MarshalJSON keeps the authenticated internal run wire compact without
// weakening required-field semantics. The seven positions are an ABI-pinned
// closed tuple; unlike an object, no field name is repeated for every row.
func (provenance profilerPairRowProvenance) MarshalJSON() ([]byte, error) {
	out := make([]byte, 0, 48)
	out = append(out, '[')
	values := [...]uint64{
		uint64(provenance.LaneID), uint64(provenance.TextMessageOrdinal), uint64(provenance.PairKind),
		uint64(provenance.EndpointSlot), uint64(provenance.PublisherSlot), uint64(provenance.Flags),
		uint64(provenance.TraceClass),
	}
	for index, value := range values {
		if index > 0 {
			out = append(out, ',')
		}
		out = strconv.AppendUint(out, value, 10)
	}
	out = append(out, ']')
	return out, nil
}

func (provenance *profilerPairRowProvenance) UnmarshalJSON(data []byte) error {
	values, ok := parseProfilerPairRowProvenanceTuple(data)
	if !ok || values[0] > uint64(^uint32(0)) || values[1] > uint64(^uint32(0)) ||
		values[2] > uint64(^uint8(0)) || values[3] > uint64(^uint8(0)) ||
		values[4] > uint64(^uint8(0)) || values[5] > uint64(^uint8(0)) ||
		values[6] > uint64(^uint8(0)) {
		return &traceDBOutputInvariantError{Reason: "profiler_row_provenance_wire_invalid"}
	}
	*provenance = profilerPairRowProvenance{
		LaneID: uint32(values[0]), TextMessageOrdinal: uint32(values[1]), PairKind: pairRenderKind(values[2]),
		EndpointSlot: profilerPairEndpointSlot(values[3]), PublisherSlot: profilerPairPublisherSlot(values[4]),
		Flags: profilerPairRowProvenanceFlags(values[5]), TraceClass: profilerTraceClass(values[6]),
	}
	return nil
}

func parseProfilerPairRowProvenanceTuple(data []byte) ([7]uint64, bool) {
	var values [7]uint64
	index := 0
	skipSpace := func() {
		for index < len(data) {
			switch data[index] {
			case ' ', '\t', '\r', '\n':
				index++
			default:
				return
			}
		}
	}
	skipSpace()
	if index >= len(data) || data[index] != '[' {
		return values, false
	}
	index++
	for slot := range values {
		skipSpace()
		if index >= len(data) || data[index] < '0' || data[index] > '9' {
			return values, false
		}
		var value uint64
		for index < len(data) && data[index] >= '0' && data[index] <= '9' {
			digit := uint64(data[index] - '0')
			if value > (^uint64(0)-digit)/10 {
				return values, false
			}
			value = value*10 + digit
			index++
		}
		values[slot] = value
		skipSpace()
		want := byte(',')
		if slot == len(values)-1 {
			want = ']'
		}
		if index >= len(data) || data[index] != want {
			return values, false
		}
		index++
	}
	skipSpace()
	return values, index == len(data)
}

func (provenance profilerPairRowProvenance) sourceValid() bool {
	if !profilerPairKindValid(provenance.PairKind) || !provenance.PublisherSlot.valid() ||
		provenance.Flags&^profilerPairRowProvenanceFlagMask != 0 ||
		provenance.Flags == profilerPairRowProvenanceFlagMask {
		return false
	}
	switch provenance.PublisherSlot {
	case profilerPairPublisherNone:
		// Inactive/source-neutral sorter callers carry no outer publisher. A
		// structured pair flag remains legal for source-neutral typed fixtures.
		if provenance.TextMessageOrdinal != 0 || provenance.Flags == profilerPairRowProvenanceText {
			return false
		}
	case profilerPairPublisherExactFtrace:
		// Exact ftrace can publish structured rows, strict compatibility text,
		// or ordinary non-pair structured events with no pair/text flag.
		if profilerPairBudgetKind(provenance.PairKind) && provenance.Flags == 0 {
			return false
		}
	case profilerPairPublisherBytrace, profilerPairPublisherOtherText:
		if provenance.TextMessageOrdinal == 0 || provenance.Flags != profilerPairRowProvenanceText {
			return false
		}
	case profilerPairPublisherNoncanonicalFtrace:
		// This route is coverage-only and never owns a rendered row.
		return false
	case profilerPairPublisherSession:
		if provenance.TextMessageOrdinal != 0 || provenance.Flags != 0 {
			return false
		}
	default:
		return false
	}
	if provenance.TextMessageOrdinal == 0 {
		if provenance.Flags&profilerPairRowProvenanceText != 0 {
			return false
		}
	} else if provenance.Flags != profilerPairRowProvenanceText ||
		provenance.PublisherSlot == profilerPairPublisherNone ||
		provenance.PublisherSlot == profilerPairPublisherSession {
		return false
	}
	if provenance.PairKind == pairRenderUnknown {
		return provenance.LaneID == 0 && provenance.EndpointSlot == profilerPairEndpointNone &&
			provenance.Flags&profilerPairRowProvenanceStructured == 0
	}
	if provenance.PairKind == pairRenderWorkqueue || provenance.PairKind == pairRenderDMAFence {
		return provenance.LaneID == 0 && provenance.EndpointSlot == profilerPairEndpointNone &&
			provenance.PublisherSlot == profilerPairPublisherNone && provenance.TextMessageOrdinal == 0 &&
			provenance.Flags == 0
	}
	descriptor, ok := provenance.EndpointSlot.descriptor()
	if !ok || descriptor.kind != provenance.PairKind {
		return false
	}
	if provenance.Flags&profilerPairRowProvenanceStructured != 0 {
		return provenance.TextMessageOrdinal == 0 && descriptor.structuredField != 0 &&
			(provenance.PublisherSlot == profilerPairPublisherNone ||
				provenance.PublisherSlot == profilerPairPublisherExactFtrace)
	}
	if provenance.PublisherSlot != profilerPairPublisherNone &&
		provenance.PublisherSlot != profilerPairPublisherSession &&
		provenance.Flags != profilerPairRowProvenanceText {
		return false
	}
	return true
}

func (provenance profilerPairRowProvenance) classifiedValid() bool {
	if !provenance.sourceValid() || !provenance.TraceClass.valid() {
		return false
	}
	switch provenance.PublisherSlot {
	case profilerPairPublisherNone:
		return provenance.TraceClass == profilerTraceClassNone
	case profilerPairPublisherExactFtrace:
		if provenance.TextMessageOrdinal != 0 || provenance.Flags&profilerPairRowProvenanceText != 0 {
			return provenance.TraceClass == profilerTraceClassTextKnown ||
				provenance.TraceClass == profilerTraceClassTextIntentionalUnknown
		}
		return provenance.TraceClass == profilerTraceClassStructuredKnown
	case profilerPairPublisherBytrace, profilerPairPublisherOtherText, profilerPairPublisherSession:
		return provenance.TraceClass == profilerTraceClassTextKnown ||
			provenance.TraceClass == profilerTraceClassTextIntentionalUnknown
	default:
		return false
	}
}

func (provenance profilerPairRowProvenance) valid() bool {
	return provenance.classifiedValid()
}

// storageValid admits an honest unclassified source tuple only for
// infrastructure-only sorter/proof fixtures. Production Profiler extraction
// enables trace classification before its first row and therefore uses
// valid(), which forbids an active publisher from carrying class None.
func (provenance profilerPairRowProvenance) storageValid() bool {
	return provenance.sourceValid() &&
		(provenance.TraceClass == profilerTraceClassNone || provenance.classifiedValid())
}
