package hitraceconv

import (
	"math"
	"testing"
	"unsafe"
)

func TestProfilerPairFamilyEndpointOrdinalsClosedContinuousAndDescriptorConsistent(t *testing.T) {
	if profilerPairFamilyEndpointCapacity != 6 {
		t.Fatalf("family endpoint capacity=%d want=6", profilerPairFamilyEndpointCapacity)
	}
	wantCounts := [pairRenderKindCount]uint8{
		pairRenderMMC: 2, pairRenderF2FS: 6, pairRenderBlock: 4,
	}
	seenSlots := [profilerPairEndpointSlotCount]bool{}
	for _, kind := range []pairRenderKind{pairRenderMMC, pairRenderF2FS, pairRenderBlock} {
		count, ok := profilerPairFamilyEndpointCount(kind)
		if !ok || count != wantCounts[kind] {
			t.Fatalf("family %d endpoint count=%d,%t want=%d", kind, count, ok, wantCounts[kind])
		}
		for rawOrdinal := uint8(0); rawOrdinal < count; rawOrdinal++ {
			ordinal := profilerPairFamilyEndpointOrdinal(rawOrdinal)
			slot, slotOK := profilerPairEndpointForFamilyOrdinal(kind, ordinal)
			descriptor, descriptorOK := slot.descriptor()
			gotOrdinal, ordinalOK := slot.familyOrdinal(kind)
			if !slotOK || !descriptorOK || descriptor.kind != kind || descriptor.slot != slot ||
				!ordinalOK || gotOrdinal != ordinal || slot == profilerPairEndpointNone || seenSlots[slot] {
				t.Fatalf("family %d ordinal %d inconsistent: slot=%d descriptor=%+v flags=%t/%t/%t roundtrip=%d",
					kind, ordinal, slot, descriptor, slotOK, descriptorOK, ordinalOK, gotOrdinal)
			}
			seenSlots[slot] = true
		}
		if slot, ok := profilerPairEndpointForFamilyOrdinal(kind, profilerPairFamilyEndpointOrdinal(count)); ok || slot != profilerPairEndpointNone {
			t.Fatalf("family %d admitted ordinal at count %d as slot=%d", kind, count, slot)
		}
	}
	for _, descriptor := range profilerPairEndpointRoster {
		if !seenSlots[descriptor.slot] {
			t.Fatalf("descriptor slot %d (%s) absent from family-local roster", descriptor.slot, descriptor.name)
		}
	}
	for _, invalidKind := range []pairRenderKind{
		pairRenderUnknown, pairRenderWorkqueue, pairRenderDMAFence, pairRenderKindCount,
	} {
		if count, ok := profilerPairFamilyEndpointCount(invalidKind); ok || count != 0 {
			t.Fatalf("non-profiler family %d exposed count=%d,%t", invalidKind, count, ok)
		}
		if slot, ok := profilerPairEndpointForFamilyOrdinal(invalidKind, 0); ok || slot != profilerPairEndpointNone {
			t.Fatalf("non-profiler family %d exposed ordinal 0 as %d,%t", invalidKind, slot, ok)
		}
	}
	for _, mismatch := range []struct {
		kind pairRenderKind
		slot profilerPairEndpointSlot
	}{
		{pairRenderMMC, profilerPairEndpointF2FSSyncFileEnter},
		{pairRenderF2FS, profilerPairEndpointMMCRequestDone},
		{pairRenderBlock, profilerPairEndpointF2FSWriteEnd},
		{pairRenderBlock, profilerPairEndpointSlotCount},
	} {
		if ordinal, ok := mismatch.slot.familyOrdinal(mismatch.kind); ok || ordinal != 0 {
			t.Fatalf("family %d admitted foreign slot %d as ordinal=%d,%t", mismatch.kind, mismatch.slot, ordinal, ok)
		}
	}
}

func TestProfilerPairLaneStateFixedEndpointCountersAreTransactional(t *testing.T) {
	if got := unsafe.Sizeof(profilerPairLaneEndpointCounts{}); got != 8 {
		t.Fatalf("endpoint counts size=%d want=8", got)
	}
	if got := unsafe.Sizeof(profilerPairLaneState{}); got > 80 {
		t.Fatalf("fixed lane state size=%d exceeds the 80-byte commercial bound", got)
	}
	if got := len((profilerPairLaneState{}).endpointCounts); got != profilerPairFamilyEndpointCapacity {
		t.Fatalf("lane endpoint slots=%d want=%d", got, profilerPairFamilyEndpointCapacity)
	}
	state := profilerPairLaneState{}
	next, ok := state.stageEndpointRows(pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, 3, 2)
	if !ok || state != (profilerPairLaneState{}) {
		t.Fatalf("first staged mutation failed or changed receiver: ok=%t before=%+v after=%+v", ok, state, next)
	}
	next, ok = next.stageEndpointRows(pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, 4, 1)
	if !ok {
		t.Fatal("second fixed endpoint mutation failed")
	}
	begin, beginOK := next.endpointCountsFor(pairRenderF2FS, profilerPairEndpointF2FSWriteBegin)
	end, endOK := next.endpointCountsFor(pairRenderF2FS, profilerPairEndpointF2FSWriteEnd)
	rows, structuredRows, totalsOK := next.endpointTotals(pairRenderF2FS)
	if !beginOK || !endOK || !totalsOK || begin != (profilerPairLaneEndpointCounts{3, 2}) ||
		end != (profilerPairLaneEndpointCounts{4, 1}) || rows != 7 || structuredRows != 3 {
		t.Fatalf("fixed counts drifted: begin=%+v/%t end=%+v/%t totals=%d/%d/%t state=%+v",
			begin, beginOK, end, endOK, rows, structuredRows, totalsOK, next)
	}
	if _, ok := next.endpointCountsFor(pairRenderMMC, profilerPairEndpointF2FSWriteBegin); ok {
		t.Fatal("foreign family read a fixed endpoint slot")
	}

	for _, test := range []struct {
		name       string
		state      profilerPairLaneState
		kind       pairRenderKind
		endpoint   profilerPairEndpointSlot
		rows       uint32
		structured uint32
	}{
		{name: "structured exceeds row delta", state: next, kind: pairRenderF2FS,
			endpoint: profilerPairEndpointF2FSWriteBegin, rows: 1, structured: 2},
		{name: "foreign endpoint", state: next, kind: pairRenderMMC,
			endpoint: profilerPairEndpointF2FSWriteBegin, rows: 1},
		{name: "non profiler family", state: next, kind: pairRenderWorkqueue,
			endpoint: profilerPairEndpointF2FSWriteBegin, rows: 1},
		{name: "counter overflow", state: func() profilerPairLaneState {
			corrupt := profilerPairLaneState{}
			corrupt.endpointCounts[0].rows = math.MaxUint32
			return corrupt
		}(), kind: pairRenderMMC, endpoint: profilerPairEndpointMMCRequestStart, rows: 1},
		{name: "unused tail residue", state: func() profilerPairLaneState {
			corrupt := profilerPairLaneState{}
			corrupt.endpointCounts[2].rows = 1
			return corrupt
		}(), kind: pairRenderMMC, endpoint: profilerPairEndpointMMCRequestStart, rows: 1},
		{name: "structured exceeds existing endpoint rows", state: func() profilerPairLaneState {
			corrupt := profilerPairLaneState{}
			corrupt.endpointCounts[0] = profilerPairLaneEndpointCounts{rows: 1, structuredRows: 2}
			return corrupt
		}(), kind: pairRenderMMC, endpoint: profilerPairEndpointMMCRequestStart, rows: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.state
			if staged, ok := test.state.stageEndpointRows(test.kind, test.endpoint, test.rows, test.structured); ok || staged != (profilerPairLaneState{}) || test.state != before {
				t.Fatalf("invalid mutation admitted or changed receiver: ok=%t staged=%+v before=%+v after=%+v",
					ok, staged, before, test.state)
			}
		})
	}
}

func TestProfilerPairLaneStateObservationBoundDominatesUint32Counters(t *testing.T) {
	if profilerPairBarrierMaxObservations != 4_000_000 ||
		uint64(profilerPairBarrierMaxObservations) > uint64(math.MaxUint32) {
		t.Fatalf("lane counter proof bound=%d is not the pinned uint32-safe 4M domain",
			profilerPairBarrierMaxObservations)
	}
	state := profilerPairLaneState{}
	state.endpointCounts[0].rows = uint32(profilerPairBarrierMaxObservations)
	if !state.endpointCountsValid(pairRenderF2FS) {
		t.Fatal("exact 4M lane observation bound rejected")
	}
	before := state
	if staged, ok := state.stageEndpointRows(pairRenderF2FS, profilerPairEndpointF2FSSyncFileExit, 1, 0); ok || staged != (profilerPairLaneState{}) || state != before {
		t.Fatalf("aggregate lane observation overflow admitted: ok=%t staged=%+v", ok, staged)
	}

	state = profilerPairLaneState{}
	state.endpointCounts[0] = profilerPairLaneEndpointCounts{rows: 2_000_000, structuredRows: 1_000_000}
	state.endpointCounts[1] = profilerPairLaneEndpointCounts{rows: 2_000_000, structuredRows: 1_000_000}
	if rows, structured, ok := state.endpointTotals(pairRenderF2FS); !ok || rows != 4_000_000 || structured != 2_000_000 {
		t.Fatalf("bounded multi-endpoint totals=%d/%d,%t", rows, structured, ok)
	}
	state.endpointCounts[2].rows = 1
	if state.endpointCountsValid(pairRenderF2FS) {
		t.Fatal("aggregate count above the proof domain remained valid")
	}
}
