package hitraceconv

// These helpers reconstruct the retired B-d2a shadow views for assertions
// only. Production code must never retain these maps: the fixed ledger and
// the unique lane registry are the sole endpoint/lane authorities.

func profilerTestPairLaneRows(sink *traceDBRowSink) map[pairRenderKind]map[string]int {
	out := make(map[pairRenderKind]map[string]int)
	if sink == nil {
		return out
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		for index, lane := range sink.pairLaneRegistries[kind].keys {
			rows, _, ok := sink.pairLaneRegistries[kind].states[index].endpointTotals(kind)
			if !ok || rows == 0 {
				continue
			}
			if out[kind] == nil {
				out[kind] = make(map[string]int)
			}
			out[kind][lane] = int(rows)
		}
	}
	return out
}

func profilerTestPairTableRows(sink *traceDBRowSink) map[pairRenderKind]map[string]map[string]int {
	out := make(map[pairRenderKind]map[string]map[string]int)
	if sink == nil {
		return out
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		if !profilerPairBudgetKind(kind) {
			continue
		}
		for index, lane := range sink.pairLaneRegistries[kind].keys {
			state := sink.pairLaneRegistries[kind].states[index]
			for ordinal := profilerPairFamilyEndpointOrdinal(0); ; ordinal++ {
				slot, ok := profilerPairEndpointForFamilyOrdinal(kind, ordinal)
				if !ok {
					break
				}
				counts, ok := state.endpointCountsFor(kind, slot)
				if !ok || counts.rows == 0 {
					continue
				}
				descriptor, _ := slot.descriptor()
				if out[kind] == nil {
					out[kind] = make(map[string]map[string]int)
				}
				if out[kind][descriptor.name] == nil {
					out[kind][descriptor.name] = make(map[string]int)
				}
				out[kind][descriptor.name][lane] = int(counts.rows)
			}
		}
	}
	return out
}

func profilerTestPairTableTotals(sink *traceDBRowSink) map[pairRenderKind]map[string]int {
	out := make(map[pairRenderKind]map[string]int)
	if sink == nil {
		return out
	}
	for _, descriptor := range profilerPairEndpointRoster {
		counts := sink.pairFixedLedger.endpoints[descriptor.slot]
		if counts.staged == 0 {
			continue
		}
		if out[descriptor.kind] == nil {
			out[descriptor.kind] = make(map[string]int)
		}
		out[descriptor.kind][descriptor.name] = counts.staged
	}
	return out
}

func profilerTestPoisonedLanes(sink *traceDBRowSink) map[pairRenderKind]map[string]bool {
	out := make(map[pairRenderKind]map[string]bool)
	if sink == nil {
		return out
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		for index, lane := range sink.pairLaneRegistries[kind].keys {
			if !sink.pairLaneRegistries[kind].states[index].poisoned {
				continue
			}
			if out[kind] == nil {
				out[kind] = make(map[string]bool)
			}
			out[kind][lane] = true
		}
	}
	return out
}

func profilerTestStructuredLaneRows(sink *traceDBRowSink) map[pairRenderKind]map[string]int {
	out := make(map[pairRenderKind]map[string]int)
	if sink == nil {
		return out
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		for index, lane := range sink.pairLaneRegistries[kind].keys {
			_, structured, ok := sink.pairLaneRegistries[kind].states[index].endpointTotals(kind)
			if !ok || structured == 0 {
				continue
			}
			if out[kind] == nil {
				out[kind] = make(map[string]int)
			}
			out[kind][lane] = int(structured)
		}
	}
	return out
}

func profilerTestStructuredEventRows(sink *traceDBRowSink) map[pairRenderKind]map[int]int {
	out := make(map[pairRenderKind]map[int]int)
	if sink == nil {
		return out
	}
	for _, descriptor := range profilerPairEndpointRoster {
		if descriptor.structuredField == 0 {
			continue
		}
		count := sink.pairFixedLedger.endpoints[descriptor.slot].structured
		if count == 0 {
			continue
		}
		if out[descriptor.kind] == nil {
			out[descriptor.kind] = make(map[int]int)
		}
		out[descriptor.kind][descriptor.structuredField] = count
	}
	return out
}

func profilerTestStructuredEventLanes(sink *traceDBRowSink) map[pairRenderKind]map[int]map[string]int {
	out := make(map[pairRenderKind]map[int]map[string]int)
	if sink == nil {
		return out
	}
	for kind := pairRenderKind(1); kind < pairRenderKindCount; kind++ {
		for index, lane := range sink.pairLaneRegistries[kind].keys {
			state := sink.pairLaneRegistries[kind].states[index]
			for ordinal := profilerPairFamilyEndpointOrdinal(0); ; ordinal++ {
				slot, ok := profilerPairEndpointForFamilyOrdinal(kind, ordinal)
				if !ok {
					break
				}
				descriptor, _ := slot.descriptor()
				if descriptor.structuredField == 0 {
					continue
				}
				counts, ok := state.endpointCountsFor(kind, slot)
				if !ok || counts.structuredRows == 0 {
					continue
				}
				if out[kind] == nil {
					out[kind] = make(map[int]map[string]int)
				}
				if out[kind][descriptor.structuredField] == nil {
					out[kind][descriptor.structuredField] = make(map[string]int)
				}
				out[kind][descriptor.structuredField][lane] = int(counts.structuredRows)
			}
		}
	}
	return out
}

type profilerTestBlockLaneClock struct {
	seq  int
	tsNS uint64
}

func profilerTestBlockLaneClocks(sink *traceDBRowSink) map[string]profilerTestBlockLaneClock {
	out := make(map[string]profilerTestBlockLaneClock)
	if sink == nil {
		return out
	}
	registry := &sink.pairLaneRegistries[pairRenderBlock]
	for index, lane := range registry.keys {
		state := registry.states[index]
		if state.blockClockSeen {
			out[lane] = profilerTestBlockLaneClock{seq: state.lastBlockSeq, tsNS: state.lastBlockTSNS}
		}
	}
	return out
}
