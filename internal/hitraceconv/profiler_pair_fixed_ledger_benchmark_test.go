package hitraceconv

import "testing"

func BenchmarkProfilerPairExistingLanePreflightCommit(b *testing.B) {
	sink, err := newTraceDBRowSink(b.TempDir(), 8)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sink.cleanup() })
	if err := sink.add(renderedRow{
		tsNS: 1, seq: 1, line: "seed", pairKind: pairRenderF2FS,
		pairLane: "lane", pairTable: "f2fs_write_begin",
	}); err != nil {
		b.Fatal(err)
	}
	baseLedger := sink.pairFixedLedger
	baseState := sink.pairLaneRegistries[pairRenderF2FS].states[0]
	row := renderedRow{
		tsNS: 2, seq: 2, line: "next", pairKind: pairRenderF2FS,
		pairLane: "lane", pairTable: "f2fs_write_end",
		profilerEndpointSlot: profilerPairEndpointF2FSWriteEnd, profilerLaneID: 1,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		sink.pairFixedLedger = baseLedger
		sink.pairLaneRegistries[pairRenderF2FS].states[0] = baseState
		if err := sink.preflightProfilerPairFixedMutation(&row, nil); err != nil {
			b.Fatal(err)
		}
		if !sink.commitProfilerPairFixedRow(row, true) {
			b.Fatal("existing exact lane was not retained")
		}
	}
}

func BenchmarkProfilerPairExactLanePoison(b *testing.B) {
	ledger := profilerPairFixedLedger{}
	stage, ok := ledger.planStageRow(
		pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true, false,
	)
	if !ok {
		b.Fatal("seed row plan failed")
	}
	stage.apply(&ledger)
	lane, ok := (profilerPairLaneState{}).stageEndpointRows(
		pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, 1, 1,
	)
	if !ok {
		b.Fatal("seed lane plan failed")
	}
	baseLedger, baseLane := ledger, lane
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		ledger, lane = baseLedger, baseLane
		plan, planned := ledger.planPoisonLane(pairRenderF2FS, lane)
		if !planned {
			b.Fatal("lane poison plan failed")
		}
		plan.apply(&ledger, &lane)
	}
}

func BenchmarkProfilerPairWholeFamilyPoison(b *testing.B) {
	ledger := profilerPairFixedLedger{}
	stage, ok := ledger.planStageRow(
		pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true, false,
	)
	if !ok {
		b.Fatal("seed row plan failed")
	}
	stage.apply(&ledger)
	base := ledger
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		ledger = base
		plan, planned := ledger.planPoisonFamily(pairRenderF2FS)
		if !planned {
			b.Fatal("family poison plan failed")
		}
		plan.apply(&ledger)
	}
}

func BenchmarkProfilerPairBlockExistingLaneClock(b *testing.B) {
	sink, err := newTraceDBRowSink(b.TempDir(), 8)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = sink.cleanup() })
	if err := sink.add(renderedRow{
		tsNS: 1, seq: 1, line: "seed", pairKind: pairRenderBlock,
		pairLane: "request", pairTable: "block_rq_issue",
	}); err != nil {
		b.Fatal(err)
	}
	base := sink.pairLaneRegistries[pairRenderBlock].states[0]
	row := renderedRow{
		tsNS: 2, seq: 2, line: "next", pairKind: pairRenderBlock,
		pairLane: "request", profilerLaneID: 1,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		sink.pairLaneRegistries[pairRenderBlock].states[0] = base
		sink.commitProfilerBlockLaneClock(row)
	}
}
