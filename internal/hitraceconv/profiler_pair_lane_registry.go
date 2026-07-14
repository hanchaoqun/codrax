package hitraceconv

import (
	"math"
	"strings"
)

// profilerPairLaneRegistry is the single typed identity registry for one pair
// family. A lane string is retained exactly once and rows carry only its dense
// source-local ID. Publication counters remain on the legacy oracle during
// P1-a2.4-A and move to the terminal fixed ledger in the following sub-batch.
type profilerPairLaneRegistry struct {
	byKey  map[string]uint32
	keys   []string
	states []profilerPairLaneState
}

type profilerPairLaneState struct {
	poisoned       bool
	blockClockSeen bool
	lastBlockSeq   int
	lastBlockTSNS  uint64
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
