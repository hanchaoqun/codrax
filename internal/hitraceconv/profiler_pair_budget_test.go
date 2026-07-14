package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestProfilerPairObservationBudgetFailsBothFamiliesBeforeSpillPublication(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{},
	)
	defer sink.cleanup()
	sink.legacyPairProof.maxObservations = 2
	sink.legacyPairProof.maxLaneKeys = 10
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherExactFtrace) {
		t.Fatal("pair census did not start")
	}
	rows := []renderedRow{
		{tsNS: 1, seq: 1, line: "f2fs-start", pairKind: pairRenderF2FS, pairLane: "f2fs-a", pairTable: "f2fs_write_begin", structuredPair: true, profilerEventField: 4011},
		{tsNS: 2, seq: 2, line: "mmc-start", pairKind: pairRenderMMC, pairLane: "mmc-a", pairTable: "mmc_request_start", structuredPair: true, profilerEventField: 4016},
		// cap+1 is accepted into scalar publication accounting, but it cannot
		// extend proof state and closes both critical families before output.
		{tsNS: 3, seq: 3, line: "f2fs-done", pairKind: pairRenderF2FS, pairLane: "f2fs-a", pairTable: "f2fs_write_end", structuredPair: true, profilerEventField: 4012},
		{tsNS: 4, seq: 4, line: "print-keep"},
	}
	for _, row := range rows {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	staged := sink.endPairRowCensus()
	if staged[pairRenderMMC].total != 1 || staged[pairRenderF2FS].total != 2 {
		t.Fatalf("publisher staged totals drifted: mmc=%d f2fs=%d",
			staged[pairRenderMMC].total, staged[pairRenderF2FS].total)
	}
	if sink.legacyPairProof.failureReason != "observations" || sink.legacyPairProof.observations != 2 ||
		!sink.poisoned[pairRenderMMC] || !sink.poisoned[pairRenderF2FS] {
		t.Fatalf("observation cap did not fail closed globally: failed=%t reason=%q observations=%d poisoned=%v",
			sink.legacyPairProof.failureReason != "", sink.legacyPairProof.failureReason, sink.legacyPairProof.observations, sink.poisoned)
	}
	if len(profilerTestPairLaneRows(sink)[pairRenderMMC]) != 0 || len(profilerTestPairLaneRows(sink)[pairRenderF2FS]) != 0 ||
		len(profilerTestPairTableRows(sink)[pairRenderMMC]) != 0 || len(profilerTestPairTableRows(sink)[pairRenderF2FS]) != 0 ||
		len(profilerTestStructuredLaneRows(sink)[pairRenderMMC]) != 0 || len(profilerTestStructuredLaneRows(sink)[pairRenderF2FS]) != 0 {
		t.Fatalf("budget failure retained subordinate proof maps: lanes=%v tables=%v structured=%v",
			profilerTestPairLaneRows(sink), profilerTestPairTableRows(sink), profilerTestStructuredLaneRows(sink))
	}
	if sink.withheldPairRowsForKind(pairRenderF2FS) != 2 || sink.withheldPairRowsForKind(pairRenderMMC) != 1 ||
		sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteBegin].withheld != 1 ||
		sink.pairFixedLedger.endpoints[profilerPairEndpointF2FSWriteEnd].withheld != 1 ||
		sink.pairFixedLedger.endpoints[profilerPairEndpointMMCRequestStart].withheld != 1 {
		t.Fatalf("fixed scalar/endpoint accounting drifted after map release: totals=%v tables=%v ledger=%+v",
			sink.pairRows, profilerTestPairTableTotals(sink), sink.pairFixedLedger)
	}
	if sink.stats.RowsAccepted != 4 || sink.publishableRows() != 1 || len(sink.runs) == 0 {
		t.Fatalf("budget barrier damaged spill/non-pair accounting: stats=%+v publishable=%d chunks=%d",
			sink.stats, sink.publishableRows(), len(sink.runs))
	}
	coverage := profilerF2FSPairBarrierCoverage(2, sink)
	if coverage.FieldSources["budget_fail_closed"] != "true" || coverage.FieldSources["budget_failure"] != "observations" ||
		!strings.Contains(profilerPairBudgetCaveat(sink, pairRenderF2FS), "budget_fail_closed=true reason=observations") {
		t.Fatalf("budget failure was not disclosed in coverage/caveat: coverage=%+v caveat=%q", coverage, profilerPairBudgetCaveat(sink, pairRenderF2FS))
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extraction := profilerContainerExtraction{
		Messages:                 1,
		StructuredFtrace:         1,
		StructuredRows:           4,
		publicationCaveatPending: true,
		TraceCoverage: []TraceDBCoverage{
			{Family: "builtin_modern_profiler", Table: "plugin:ftrace-plugin", RowsRead: 4, RowsEmitted: 4, FieldSources: map[string]string{
				profilerCoverageF2FSStagedRows: "2", profilerCoverageMMCStagedRows: "1",
			}},
			{Table: "f2fs_write_begin", RowsRead: 1, RowsEmitted: 1},
			{Table: "f2fs_write_end", RowsRead: 1, RowsEmitted: 1},
			{Table: "mmc_request_start", RowsRead: 1, RowsEmitted: 1},
		},
	}
	if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherExactFtrace, 0) {
		t.Fatal("record fixed structured publisher coverage")
	}
	for field, index := range map[int]int{4011: 1, 4012: 2, 4016: 3} {
		slot := profilerFtraceEventSlot(field)
		extraction.profilerEventCoverage.Present[slot] = true
		extraction.profilerEventCoverage.Index[slot] = index
	}
	terminal, err := applyProfilerTerminalPublication(&extraction, sink)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.rows != (profilerTerminalPublicationCounts{staged: 4, published: 1, withheld: 3}) ||
		terminal.structuredRows != terminal.rows ||
		terminal.publisherFamilies[profilerPairPublisherExactFtrace][pairRenderF2FS] !=
			(profilerTerminalPublicationCounts{staged: 2, withheld: 2}) ||
		terminal.publisherFamilies[profilerPairPublisherExactFtrace][pairRenderMMC] !=
			(profilerTerminalPublicationCounts{staged: 1, withheld: 1}) {
		t.Fatalf("budget terminal ledger drifted: %+v", terminal)
	}
	for index, item := range extraction.TraceCoverage[1:] {
		if item.RowsEmitted != 0 || item.RowsEmitted > item.RowsRead {
			t.Fatalf("terminal projection left a published pair row at event coverage[%d]: %+v", index, item)
		}
		if item.FieldSources["complete_capture_withheld_rows"] != "1" {
			t.Fatalf("terminal endpoint projection lost exact withheld row at event coverage[%d]: %+v", index, item)
		}
	}
	if extraction.TraceCoverage[0].RowsEmitted != 1 ||
		extraction.TraceCoverage[0].FieldSources["complete_capture_withheld_rows"] != "3" ||
		extraction.StructuredRows != 1 || !extraction.terminalPublicationApplied {
		t.Fatalf("terminal publisher projection did not account once: %+v", extraction)
	}
	var out bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsAccepted != 4 || stats.RowsWritten != 1 || stats.RowsWithheld != 3 ||
		!strings.Contains(out.String(), "print-keep") || strings.Contains(out.String(), "f2fs-") || strings.Contains(out.String(), "mmc-") {
		t.Fatalf("budget barrier output/reconciliation mismatch: stats=%+v\n%s", stats, out.String())
	}
}

func TestProfilerPairLaneBudgetCountsInvalidOnlyPoisonAndFailsBothFamilies(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	sink.legacyPairProof.maxObservations = 10
	sink.legacyPairProof.maxLaneKeys = 2
	sink.poisonPairLane(pairRenderF2FS, "f2fs-a")
	sink.poisonPairLane(pairRenderF2FS, "f2fs-b")
	sink.poisonPairLane(pairRenderF2FS, "f2fs-cap-plus-one")
	if sink.legacyPairProof.failureReason != "lane_keys" || sink.legacyPairProof.laneKeys != 2 ||
		sink.legacyPairProof.observations != 3 || !sink.poisoned[pairRenderMMC] || !sink.poisoned[pairRenderF2FS] ||
		len(profilerTestPoisonedLanes(sink)[pairRenderMMC]) != 0 || len(profilerTestPoisonedLanes(sink)[pairRenderF2FS]) != 0 {
		t.Fatalf("invalid-only lane cap did not release maps/fail both families: observations=%d lanes=%d failed=%t reason=%q poisoned=%v lane_maps=%v",
			sink.legacyPairProof.observations, sink.legacyPairProof.laneKeys, sink.legacyPairProof.failureReason != "", sink.legacyPairProof.failureReason, sink.poisoned, profilerTestPoisonedLanes(sink))
	}
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "print-survives-invalid-only-cap"}); err != nil {
		t.Fatal(err)
	}
	if sink.stats.RowsAccepted != 1 || sink.publishableRows() != 1 {
		t.Fatalf("invalid-only pair cap damaged non-pair accounting: stats=%+v publishable=%d", sink.stats, sink.publishableRows())
	}
	coverage := profilerMMCPairBarrierCoverage(0, sink)
	if coverage.RowsRead != 0 || coverage.FieldSources["budget_failure"] != "lane_keys" {
		t.Fatalf("invalid-only budget failure disclosure drifted: %+v", coverage)
	}
}

func TestProfilerPairFamilyPoisonStopsAllLaterLaneMapGrowth(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 256)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "first", pairKind: pairRenderF2FS, pairLane: "first", pairTable: "f2fs_write_begin"}); err != nil {
		t.Fatal(err)
	}
	sink.poisonPairKind(pairRenderF2FS)
	for i := 0; i < 64; i++ {
		if err := sink.add(renderedRow{
			tsNS: uint64(i + 2), seq: i + 2, line: fmt.Sprintf("later-%d", i),
			pairKind: pairRenderF2FS, pairLane: fmt.Sprintf("lane-%d", i), pairTable: "f2fs_write_end",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if sink.legacyPairProof.observations != 1 || sink.legacyPairProof.laneKeys != 1 || len(profilerTestPairLaneRows(sink)[pairRenderF2FS]) != 0 ||
		len(profilerTestPairTableRows(sink)[pairRenderF2FS]) != 0 || len(profilerTestPoisonedLanes(sink)[pairRenderF2FS]) != 0 ||
		sink.pairRows[pairRenderF2FS] != 65 || profilerTestPairTableTotals(sink)[pairRenderF2FS]["f2fs_write_end"] != 64 {
		t.Fatalf("family poison continued growing subordinate maps or lost scalar totals: observations=%d lanes=%d laneRows=%v tableRows=%v totals=%v",
			sink.legacyPairProof.observations, sink.legacyPairProof.laneKeys, profilerTestPairLaneRows(sink), profilerTestPairTableRows(sink), profilerTestPairTableTotals(sink))
	}
}

// C-b1 removes the extraction consumer, but C-b2 owns retirement of this
// remaining sink census. Keep its direct bounded-walk contract until that
// separate structural deletion lands.
func TestProfilerWithheldCensusWalksOnlyPublisherRows(t *testing.T) {
	source := mustReadRendererSource(t, "streamerdb_sorter.go")
	censusBody := sourceBetweenProfilerPairFunctions(t, source,
		"func (s *traceDBRowSink) withheldPairRowsFromCensus", "func (s *traceDBRowSink) publishableRows")
	if !strings.Contains(censusBody, "for lane, count := range census.byLane") ||
		!strings.Contains(censusBody, "s.pairFixedLedger.family(kind)") ||
		!strings.Contains(censusBody, "pairLaneRegistries[kind].state") ||
		strings.Contains(censusBody, "s.poisoned[kind]") ||
		strings.Contains(censusBody, "range s.pairLaneRegistries") {
		t.Fatalf("publisher reconciliation must use the fixed family verdict and be O(publisher lanes), not O(global poison lanes):\n%s", censusBody)
	}

	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for i := 0; i < 32; i++ {
		sink.poisonPairLane(pairRenderF2FS, fmt.Sprintf("poison-%d", i))
	}
	if !sink.observeProfilerPairState(pairRenderF2FS, "clean") {
		t.Fatal("failed to register clean publisher lane")
	}
	census := profilerPairRowCensus{total: 18, byLane: map[string]int{"clean": 11, "poison-31": 7}}
	if got := sink.withheldPairRowsFromCensus(pairRenderF2FS, census); got != 7 {
		t.Fatalf("linear publisher census count=%d want=7", got)
	}
}

func sourceBetweenProfilerPairFunctions(t *testing.T, source, start, end string) string {
	t.Helper()
	from := strings.Index(source, start)
	if from < 0 {
		t.Fatalf("missing source marker %q", start)
	}
	to := strings.Index(source[from+len(start):], end)
	if to < 0 {
		t.Fatalf("missing source marker %q after %q", end, start)
	}
	return source[from : from+len(start)+to]
}
