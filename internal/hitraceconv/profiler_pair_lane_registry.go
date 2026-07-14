package hitraceconv

import (
	"math"
	"strings"
)

// profilerPairLaneRegistry is the single typed identity registry for one pair
// family. A lane string is retained exactly once and rows carry only its dense
// source-local ID. B-d2a stores the fixed endpoint counters in the same state
// while the legacy keyed maps remain only as a fail-loud parity oracle; B-d2b
// removes that oracle after transition/resource validation.
type profilerPairLaneRegistry struct {
	byKey  map[string]uint32
	keys   []string
	states []profilerPairLaneState
}

type profilerPairLaneEndpointCounts struct {
	rows           uint32
	structuredRows uint32
}

type profilerPairLaneState struct {
	endpointCounts [profilerPairFamilyEndpointCapacity]profilerPairLaneEndpointCounts
	poisoned       bool
	blockClockSeen bool
	lastBlockSeq   int
	lastBlockTSNS  uint64
}

// checkedProfilerPairLaneCounterAdd is deliberately uint32: an unpoisoned
// exact-lane state is dominated by the 4,000,000-observation proof domain.
// endpointCountsValid additionally proves that aggregate bound across all six
// fixed slots, so a wider or dynamically allocated per-lane counter is neither
// necessary nor permitted.
func checkedProfilerPairLaneCounterAdd(current, delta uint32) (uint32, bool) {
	if delta > math.MaxUint32-current {
		return 0, false
	}
	return current + delta, true
}

func (state profilerPairLaneState) endpointCountsValid(kind pairRenderKind) bool {
	count, ok := profilerPairFamilyEndpointCount(kind)
	if !ok || count == 0 || count > profilerPairFamilyEndpointCapacity {
		return false
	}
	var rowsTotal, structuredTotal uint64
	for ordinal, counts := range state.endpointCounts {
		if ordinal >= int(count) {
			if counts != (profilerPairLaneEndpointCounts{}) {
				return false
			}
			continue
		}
		if counts.structuredRows > counts.rows {
			return false
		}
		rowsTotal += uint64(counts.rows)
		structuredTotal += uint64(counts.structuredRows)
	}
	return structuredTotal <= rowsTotal && rowsTotal <= uint64(profilerPairBarrierMaxObservations)
}

// stageEndpointRows returns a complete next lane state or no state at all. It
// never mutates the receiver on overflow, a foreign endpoint, an invalid
// structured subset, an already-corrupt unused tail, or a breach of the
// observation proof bound. Callers may therefore preflight a row/event delta
// and make the final commit tail infallible.
func (state profilerPairLaneState) stageEndpointRows(
	kind pairRenderKind,
	endpoint profilerPairEndpointSlot,
	rows uint32,
	structuredRows uint32,
) (profilerPairLaneState, bool) {
	if structuredRows > rows || !state.endpointCountsValid(kind) {
		return profilerPairLaneState{}, false
	}
	ordinal, ok := endpoint.familyOrdinal(kind)
	if !ok {
		return profilerPairLaneState{}, false
	}
	counts := state.endpointCounts[ordinal]
	nextRows, rowsOK := checkedProfilerPairLaneCounterAdd(counts.rows, rows)
	nextStructured, structuredOK := checkedProfilerPairLaneCounterAdd(counts.structuredRows, structuredRows)
	if !rowsOK || !structuredOK {
		return profilerPairLaneState{}, false
	}
	state.endpointCounts[ordinal] = profilerPairLaneEndpointCounts{
		rows: nextRows, structuredRows: nextStructured,
	}
	if !state.endpointCountsValid(kind) {
		return profilerPairLaneState{}, false
	}
	return state, true
}

func (state profilerPairLaneState) endpointCountsFor(
	kind pairRenderKind,
	endpoint profilerPairEndpointSlot,
) (profilerPairLaneEndpointCounts, bool) {
	if !state.endpointCountsValid(kind) {
		return profilerPairLaneEndpointCounts{}, false
	}
	ordinal, ok := endpoint.familyOrdinal(kind)
	if !ok {
		return profilerPairLaneEndpointCounts{}, false
	}
	return state.endpointCounts[ordinal], true
}

func (state profilerPairLaneState) endpointTotals(kind pairRenderKind) (uint32, uint32, bool) {
	if !state.endpointCountsValid(kind) {
		return 0, 0, false
	}
	count, _ := profilerPairFamilyEndpointCount(kind)
	var rows, structuredRows uint32
	for ordinal := uint8(0); ordinal < count; ordinal++ {
		counts := state.endpointCounts[ordinal]
		var rowsOK, structuredOK bool
		rows, rowsOK = checkedProfilerPairLaneCounterAdd(rows, counts.rows)
		structuredRows, structuredOK = checkedProfilerPairLaneCounterAdd(structuredRows, counts.structuredRows)
		if !rowsOK || !structuredOK {
			return 0, 0, false
		}
	}
	return rows, structuredRows, true
}

func (registry *profilerPairLaneRegistry) idFor(key string) (uint32, bool) {
	if registry == nil || key == "" || registry.byKey == nil ||
		len(registry.byKey) != len(registry.states) {
		return 0, false
	}
	id, ok := registry.byKey[key]
	if !ok || id == 0 || uint64(id) > uint64(len(registry.states)) ||
		len(registry.keys) != len(registry.states) || registry.keys[id-1] != key {
		return 0, false
	}
	return id, true
}

func (registry *profilerPairLaneRegistry) intern(key string) (uint32, bool) {
	return registry.internKey(key, true)
}

// internOwned accepts a string which the sink has already cloned away from
// the untrusted frame buffer. It avoids a second unique-lane allocation while
// preserving intern() for delta/poison callers without an ownership promise.
func (registry *profilerPairLaneRegistry) internOwned(key string) (uint32, bool) {
	return registry.internKey(key, false)
}

func (registry *profilerPairLaneRegistry) internKey(key string, clone bool) (uint32, bool) {
	if registry == nil || key == "" {
		return 0, false
	}
	if len(registry.keys) != len(registry.states) ||
		(registry.byKey == nil) != (len(registry.states) == 0) || len(registry.byKey) != len(registry.states) {
		return 0, false
	}
	if id, ok := registry.idFor(key); ok {
		return id, true
	}
	if _, corruptDuplicate := registry.byKey[key]; corruptDuplicate {
		return 0, false
	}
	if uint64(len(registry.states)) >= uint64(math.MaxUint32) {
		return 0, false
	}
	if registry.byKey == nil {
		registry.byKey = make(map[string]uint32)
	}
	owned := key
	if clone {
		owned = strings.Clone(key)
	}
	id := uint32(len(registry.states) + 1)
	registry.states = append(registry.states, profilerPairLaneState{})
	registry.keys = append(registry.keys, owned)
	registry.byKey[owned] = id
	return id, true
}

func (registry *profilerPairLaneRegistry) state(id uint32) (*profilerPairLaneState, bool) {
	if registry == nil || id == 0 || uint64(id) > uint64(len(registry.states)) ||
		len(registry.keys) != len(registry.states) {
		return nil, false
	}
	return &registry.states[id-1], true
}

func (registry *profilerPairLaneRegistry) key(id uint32) (string, bool) {
	if registry == nil || id == 0 || uint64(id) > uint64(len(registry.keys)) ||
		len(registry.keys) != len(registry.states) {
		return "", false
	}
	return registry.keys[id-1], true
}

func (registry *profilerPairLaneRegistry) reset() {
	if registry == nil {
		return
	}
	registry.byKey = nil
	registry.keys = nil
	registry.states = nil
}
