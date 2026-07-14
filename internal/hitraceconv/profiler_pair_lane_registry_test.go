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
	if profilerTestPairLaneRows(sink)[pairRenderF2FS]["shared"] != 2 || profilerTestPairLaneRows(sink)[pairRenderMMC]["shared"] != 1 {
		t.Fatalf("typed/legacy lane totals drifted: f2fs=%v mmc=%v",
			profilerTestPairLaneRows(sink)[pairRenderF2FS], profilerTestPairLaneRows(sink)[pairRenderMMC])
	}

	sink.poisonPairLane(pairRenderF2FS, "shared")
	f2fsState, ok := sink.pairLaneRegistries[pairRenderF2FS].state(1)
	if !ok || !f2fsState.poisoned || !profilerTestPoisonedLanes(sink)[pairRenderF2FS]["shared"] ||
		profilerTestPoisonedLanes(sink)[pairRenderMMC]["shared"] {
		t.Fatalf("lane poison crossed family or lost parity: state=%+v f2fs=%v mmc=%v",
			f2fsState, profilerTestPoisonedLanes(sink)[pairRenderF2FS], profilerTestPoisonedLanes(sink)[pairRenderMMC])
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
		len(profilerTestPairLaneRows(sink)[pairRenderMMC]) != 0 || len(profilerTestPairTableRows(sink)[pairRenderMMC]) != 0 ||
		profilerTestPairTableTotals(sink)[pairRenderMMC]["mmc_request_start"] != 1 ||
		profilerTestStructuredEventRows(sink)[pairRenderMMC][4016] != 1 ||
		len(profilerTestStructuredEventLanes(sink)[pairRenderMMC][4016]) != 0 {
		t.Fatalf("unkeyed row forged an exact lane: registry=%+v lanes=%v tables=%v totals=%v events=%v event_lanes=%v",
			sink.pairLaneRegistries[pairRenderMMC], profilerTestPairLaneRows(sink)[pairRenderMMC],
			profilerTestPairTableRows(sink)[pairRenderMMC], profilerTestPairTableTotals(sink)[pairRenderMMC],
			profilerTestStructuredEventRows(sink)[pairRenderMMC], profilerTestStructuredEventLanes(sink)[pairRenderMMC])
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

func TestProfilerBlockLaneRegistryOwnsMonotonicClock(t *testing.T) {
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
	if !ok || !state.poisoned || !state.blockClockSeen ||
		state.lastBlockSeq != 2 || state.lastBlockTSNS != 10 ||
		!profilerTestPoisonedLanes(sink)[pairRenderBlock]["request"] {
		t.Fatalf("typed Block clock drifted: state=%+v poisoned=%v",
			state, profilerTestPoisonedLanes(sink)[pairRenderBlock])
	}

	sink.poisonPairKind(pairRenderBlock)
	if len(sink.pairLaneRegistries[pairRenderBlock].states) != 0 || len(profilerTestBlockLaneClocks(sink)) != 0 {
		t.Fatalf("Block family poison retained clock identity: registry=%+v clocks=%v",
			sink.pairLaneRegistries[pairRenderBlock], profilerTestBlockLaneClocks(sink))
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

	t.Run("state poison only", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairLaneRegistries[pairRenderF2FS].states[0].poisoned = true
		assertReason(t, sink, "profiler_pair_fixed_withheld_lane_mismatch")
	})
	t.Run("fixed withheld only", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairFixedLedger.families[pairRenderF2FS].withheld = 1
		sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin].withheld = 1
		assertReason(t, sink, "profiler_pair_fixed_withheld_lane_mismatch")
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
	t.Run("lane endpoint exceeds fixed", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairLaneRegistries[pairRenderF2FS].states[0].endpointCounts[4].rows++
		assertReason(t, sink, "profiler_pair_fixed_lane_exceeds_endpoint")
	})
	t.Run("wrong endpoint slot", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		state := &sink.pairLaneRegistries[pairRenderF2FS].states[0]
		state.endpointCounts[4].rows = 0
		state.endpointCounts[5].rows = 1
		assertReason(t, sink, "profiler_pair_fixed_lane_exceeds_endpoint")
	})
	t.Run("bounded scalar mismatch", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.pairRows[pairRenderF2FS]++
		assertReason(t, sink, "profiler_pair_fixed_ledger_family_mismatch")
	})

	t.Run("family reset rejects typed registry residue", func(t *testing.T) {
		sink := newFixture(t, pairRenderF2FS)
		sink.poisonPairKind(pairRenderF2FS)
		if _, ok := sink.pairLaneRegistries[pairRenderF2FS].intern("ghost"); !ok {
			t.Fatal("failed to create family-reset residue")
		}
		assertReason(t, sink, "profiler_pair_lane_registry_family_reset_mismatch")
	})
}
