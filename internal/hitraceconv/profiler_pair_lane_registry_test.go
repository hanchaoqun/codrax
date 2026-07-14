package hitraceconv

import (
	"os"
	"testing"
)

func TestProfilerPairLaneRegistryInternsOncePerFamily(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 16)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rows := []renderedRow{
		{tsNS: 1, seq: 1, line: "f2fs-start", pairKind: pairRenderF2FS,
			pairLane: "shared", pairTable: "f2fs_write_begin"},
		{tsNS: 2, seq: 2, line: "f2fs-done", pairKind: pairRenderF2FS,
			pairLane: "shared", pairTable: "f2fs_write_end"},
		{tsNS: 3, seq: 3, line: "mmc-start", pairKind: pairRenderMMC,
			pairLane: "shared", pairTable: "mmc_request_start"},
	}
	for _, row := range rows {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.pairLaneRegistries[pairRenderF2FS].states) != 1 ||
		len(sink.pairLaneRegistries[pairRenderMMC].states) != 1 ||
		sink.rows[0].provenance.LaneID != 1 || sink.rows[1].provenance.LaneID != 1 ||
		sink.rows[2].provenance.LaneID != 1 {
		t.Fatalf("lane registry did not intern once per family: f2fs=%+v mmc=%+v rows=%+v",
			sink.pairLaneRegistries[pairRenderF2FS], sink.pairLaneRegistries[pairRenderMMC], sink.rows)
	}
	if sink.pairLaneRegistries[pairRenderF2FS].keys[0] != "shared" ||
		sink.rows[0].provenance.EndpointSlot != profilerPairEndpointF2FSWriteBegin ||
		sink.rows[1].provenance.EndpointSlot != profilerPairEndpointF2FSWriteEnd {
		t.Fatal("compact stored provenance lost canonical lane or closed endpoint identity")
	}
	if sink.pairLaneRows[pairRenderF2FS]["shared"] != 2 || sink.pairLaneRows[pairRenderMMC]["shared"] != 1 {
		t.Fatalf("typed/legacy lane totals drifted: f2fs=%v mmc=%v",
			sink.pairLaneRows[pairRenderF2FS], sink.pairLaneRows[pairRenderMMC])
	}

	sink.poisonPairLane(pairRenderF2FS, "shared")
	f2fsState, ok := sink.pairLaneRegistries[pairRenderF2FS].state(1)
	if !ok || !f2fsState.poisoned || !sink.poisonedLanes[pairRenderF2FS]["shared"] ||
		sink.poisonedLanes[pairRenderMMC]["shared"] {
		t.Fatalf("lane poison crossed family or lost parity: state=%+v f2fs=%v mmc=%v",
			f2fsState, sink.poisonedLanes[pairRenderF2FS], sink.poisonedLanes[pairRenderMMC])
	}

	sink.poisonPairKind(pairRenderF2FS)
	if len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 ||
		len(sink.pairLaneRegistries[pairRenderF2FS].byKey) != 0 ||
		len(sink.pairLaneRegistries[pairRenderMMC].states) != 1 {
		t.Fatalf("family poison registry scope drifted: f2fs=%+v mmc=%+v",
			sink.pairLaneRegistries[pairRenderF2FS], sink.pairLaneRegistries[pairRenderMMC])
	}
}

func TestProfilerPairLaneRegistryDenseIDsSurviveSpill(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for index, lane := range []string{"lane-a", "lane-b", "lane-a"} {
		if err := sink.add(renderedRow{
			tsNS: uint64(index + 1), seq: index + 1, line: lane,
			pairKind: pairRenderF2FS, pairLane: lane, pairTable: "f2fs_write_begin",
		}); err != nil {
			t.Fatal(err)
		}
	}
	registry := &sink.pairLaneRegistries[pairRenderF2FS]
	if len(registry.byKey) != 2 || len(registry.keys) != 2 || len(registry.states) != 2 ||
		registry.keys[0] != "lane-a" || registry.keys[1] != "lane-b" || len(sink.runs) != 3 {
		t.Fatalf("dense spill registry drifted: registry=%+v runs=%d", registry, len(sink.runs))
	}
	wantIDs := []uint32{1, 2, 1}
	for index, wantID := range wantIDs {
		raw, err := os.ReadFile(sink.runs[index].path)
		if err != nil {
			t.Fatal(err)
		}
		record, err := decodeTraceDBRunRecord(raw, uint64(len(wantIDs)))
		if err != nil || record.row.provenance.LaneID != wantID {
			t.Fatalf("spill[%d] lane id=%d want=%d err=%v", index, record.row.provenance.LaneID, wantID, err)
		}
	}
	if err := sink.validateProfilerPairAccounting(); err != nil {
		t.Fatalf("dense spill parity failed: %v", err)
	}
}

func TestProfilerPairLaneRegistryKeepsCoarseUnkeyedRowsOutOfExactDomain(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.add(renderedRow{
		tsNS: 1, seq: 1, line: "mmc-source-scoped", pairKind: pairRenderMMC,
		structuredPair: true, profilerEventField: 4016,
	}); err != nil {
		t.Fatal(err)
	}
	if len(sink.pairLaneRegistries[pairRenderMMC].states) != 0 ||
		len(sink.pairLaneRows[pairRenderMMC]) != 0 || len(sink.pairTableRows[pairRenderMMC]) != 0 ||
		sink.pairTableTotals[pairRenderMMC]["mmc_request_start"] != 1 ||
		sink.structuredEventRows[pairRenderMMC][4016] != 1 ||
		len(sink.structuredEventLanes[pairRenderMMC][4016]) != 0 {
		t.Fatalf("unkeyed row forged an exact lane: registry=%+v lanes=%v tables=%v totals=%v events=%v event_lanes=%v",
			sink.pairLaneRegistries[pairRenderMMC], sink.pairLaneRows[pairRenderMMC],
			sink.pairTableRows[pairRenderMMC], sink.pairTableTotals[pairRenderMMC],
			sink.structuredEventRows[pairRenderMMC], sink.structuredEventLanes[pairRenderMMC])
	}
	if err := sink.validateProfilerPairAccounting(); err != nil {
		t.Fatalf("coarse unkeyed row failed typed parity: %v", err)
	}
}

func TestProfilerPairLaneRegistryNeverHealsCorruptIdentity(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*profilerPairLaneRegistry)
	}{
		{name: "invalid existing id", mutate: func(registry *profilerPairLaneRegistry) {
			registry.byKey["lane"] = 2
		}},
		{name: "ghost map key", mutate: func(registry *profilerPairLaneRegistry) {
			registry.byKey["ghost"] = 2
		}},
		{name: "missing reverse key", mutate: func(registry *profilerPairLaneRegistry) {
			registry.keys = nil
		}},
		{name: "missing map", mutate: func(registry *profilerPairLaneRegistry) {
			registry.byKey = nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var registry profilerPairLaneRegistry
			if id, ok := registry.intern("lane"); !ok || id != 1 {
				t.Fatalf("seed identity=%d,%t", id, ok)
			}
			test.mutate(&registry)
			beforeKeys, beforeStates, beforeMap := len(registry.keys), len(registry.states), len(registry.byKey)
			if id, ok := registry.intern("lane"); ok || id != 0 {
				t.Fatalf("corrupt registry was healed as id=%d,%t: %+v", id, ok, registry)
			}
			if len(registry.keys) != beforeKeys || len(registry.states) != beforeStates || len(registry.byKey) != beforeMap {
				t.Fatalf("failed corruption check mutated registry: before=%d/%d/%d after=%d/%d/%d",
					beforeKeys, beforeStates, beforeMap, len(registry.keys), len(registry.states), len(registry.byKey))
			}
		})
	}
}

func TestProfilerBlockLaneRegistryClockMatchesLegacyOracle(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for _, row := range []renderedRow{
		{tsNS: 20, seq: 1, line: "queue", pairKind: pairRenderBlock,
			pairLane: "request", pairTable: "block_bio_queue"},
		{tsNS: 10, seq: 2, line: "complete", pairKind: pairRenderBlock,
			pairLane: "request", pairTable: "block_bio_complete"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	state, ok := sink.pairLaneRegistries[pairRenderBlock].state(1)
	legacy, legacyOK := sink.blockLaneClocks["request"]
	if !ok || !legacyOK || !state.poisoned || !state.blockClockSeen ||
		state.lastBlockSeq != legacy.seq || state.lastBlockTSNS != legacy.tsNS ||
		legacy.seq != 2 || legacy.tsNS != 10 || !sink.poisonedLanes[pairRenderBlock]["request"] {
		t.Fatalf("typed/legacy Block clock drifted: state=%+v legacy=%+v poisoned=%v",
			state, legacy, sink.poisonedLanes[pairRenderBlock])
	}

	sink.poisonPairKind(pairRenderBlock)
	if len(sink.pairLaneRegistries[pairRenderBlock].states) != 0 || sink.blockLaneClocks != nil {
		t.Fatalf("Block family poison retained clock identity: registry=%+v clocks=%v",
			sink.pairLaneRegistries[pairRenderBlock], sink.blockLaneClocks)
	}
}

func TestProfilerPairLaneRegistryParityRejectsSplitBrain(t *testing.T) {
	newFixture := func(t *testing.T, kind pairRenderKind) *traceDBRowSink {
		t.Helper()
		sink, err := newTraceDBRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = sink.cleanup() })
		row := renderedRow{tsNS: 1, seq: 1, line: "row", pairKind: kind, pairLane: "lane"}
		switch kind {
		case pairRenderF2FS:
			row.pairTable = "f2fs_write_begin"
		case pairRenderBlock:
			row.pairTable = "block_rq_issue"
		default:
			t.Fatalf("unsupported fixture kind %d", kind)
		}
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
		if err := sink.validateProfilerPairAccounting(); err != nil {
			t.Fatalf("valid fixture failed parity: %v", err)
		}
		return sink
	}
	assertReason := func(t *testing.T, sink *traceDBRowSink, want string) {
		t.Helper()
		if got := traceDBInvariantReason(sink.validateProfilerPairAccounting()); got != want {
			t.Fatalf("parity reason=%q want=%q", got, want)
		}
	}

	t.Run("typed poison only", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairLaneRegistries[pairRenderF2FS].states[0].poisoned = true
		assertReason(t, sink, "profiler_pair_lane_registry_poison_mismatch")
	})
	t.Run("legacy poison only", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.poisonedLanes[pairRenderF2FS] = map[string]bool{"lane": true}
		assertReason(t, sink, "profiler_pair_lane_registry_poison_mismatch")
	})
	t.Run("false legacy poison key", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.poisonedLanes[pairRenderF2FS] = map[string]bool{"lane": false}
		assertReason(t, sink, "profiler_pair_lane_registry_false_poison_key")
	})
	t.Run("empty key", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairLaneRegistries[pairRenderF2FS].keys[0] = ""
		assertReason(t, sink, "profiler_pair_lane_registry_empty_key")
	})
	t.Run("dense id mismatch", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairLaneRegistries[pairRenderF2FS].byKey["lane"] = 2
		assertReason(t, sink, "profiler_pair_lane_registry_id_mismatch")
	})
	t.Run("foreign nonblock clock", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairLaneRegistries[pairRenderF2FS].states[0].blockClockSeen = true
		assertReason(t, sink, "profiler_pair_lane_registry_foreign_clock")
	})
	t.Run("block clock residue", func(t *testing.T) {
		sink := newFixture(t, pairRenderBlock)
		state := &sink.pairLaneRegistries[pairRenderBlock].states[0]
		state.blockClockSeen = false
		assertReason(t, sink, "profiler_block_lane_registry_clock_residue")
	})
	t.Run("block clock mismatch", func(t *testing.T) {
		sink := newFixture(t, pairRenderBlock)
		clock := sink.blockLaneClocks["lane"]
		clock.tsNS++
		sink.blockLaneClocks["lane"] = clock
		assertReason(t, sink, "profiler_block_lane_registry_clock_mismatch")
	})
	t.Run("table ghost lane", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		delete(sink.pairTableRows[pairRenderF2FS]["f2fs_write_begin"], "lane")
		sink.pairTableRows[pairRenderF2FS]["f2fs_write_begin"]["ghost"] = 1
		assertReason(t, sink, "profiler_pair_lane_registry_table_lane_missing")
	})
	t.Run("table total mismatch", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairTableRows[pairRenderF2FS]["f2fs_write_begin"]["lane"]++
		assertReason(t, sink, "profiler_pair_lane_registry_table_total_mismatch")
	})
	t.Run("structured ghost lane", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.structuredLaneRows[pairRenderF2FS] = map[string]int{"ghost": 1}
		assertReason(t, sink, "profiler_pair_lane_registry_structured_lane_missing")
	})
	t.Run("structured event ghost lane", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.structuredEventRows[pairRenderF2FS] = map[int]int{4011: 1}
		sink.structuredEventLanes[pairRenderF2FS] = map[int]map[string]int{4011: {"ghost": 1}}
		assertReason(t, sink, "profiler_pair_lane_registry_event_lane_missing")
	})
	t.Run("active census ghost lane", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.activePairCensus[pairRenderF2FS].byLane = map[string]int{"ghost": 1}
		assertReason(t, sink, "profiler_pair_lane_registry_census_lane_missing")
	})

	for _, test := range []struct {
		name   string
		mutate func(*traceDBRowSink)
	}{
		{name: "pair table lanes", mutate: func(sink *traceDBRowSink) {
			sink.pairTableRows[pairRenderF2FS] = map[string]map[string]int{"f2fs_write_begin": {"lane": 1}}
		}},
		{name: "structured lanes", mutate: func(sink *traceDBRowSink) {
			sink.structuredLaneRows[pairRenderF2FS] = map[string]int{"lane": 1}
		}},
		{name: "structured event lanes", mutate: func(sink *traceDBRowSink) {
			sink.structuredEventLanes[pairRenderF2FS] = map[int]map[string]int{4011: {"lane": 1}}
		}},
		{name: "active census lanes", mutate: func(sink *traceDBRowSink) {
			sink.activePairCensus[pairRenderF2FS].byLane = map[string]int{"lane": 1}
		}},
	} {
		t.Run("family reset/"+test.name, func(t *testing.T) {
			sink := newFixture(t, pairRenderF2FS)
			sink.poisonPairKind(pairRenderF2FS)
			test.mutate(sink)
			assertReason(t, sink, "profiler_pair_lane_registry_family_reset_mismatch")
		})
	}
}
