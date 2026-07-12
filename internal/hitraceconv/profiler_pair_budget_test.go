package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestProfilerPairObservationBudgetFailsBothFamiliesBeforeSpillPublication(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	sink.pairObservationLimit = 2
	sink.pairLaneLimit = 10
	if !sink.beginPairRowCensus() {
		t.Fatal("pair census did not start")
	}
	rows := []renderedRow{
		{tsNS: 1, seq: 1, line: "f2fs-start", pairKind: pairRenderF2FS, pairLane: "f2fs-a", pairTable: "f2fs_write_begin", structuredPair: true},
		{tsNS: 2, seq: 2, line: "mmc-start", pairKind: pairRenderMMC, pairLane: "mmc-a", pairTable: "mmc_request_start", structuredPair: true},
		// cap+1 is accepted into scalar publication accounting, but it cannot
		// extend proof state and closes both critical families before output.
		{tsNS: 3, seq: 3, line: "f2fs-done", pairKind: pairRenderF2FS, pairLane: "f2fs-a", pairTable: "f2fs_write_end", structuredPair: true},
		{tsNS: 4, seq: 4, line: "print-keep"},
	}
	for _, row := range rows {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	mmcCensus, f2fsCensus := sink.endPairRowCensus()
	if !sink.pairBudgetFailed || sink.pairBudgetFailure != "observations" || sink.pairObservations != 2 ||
		!sink.poisoned[pairRenderMMC] || !sink.poisoned[pairRenderF2FS] {
		t.Fatalf("observation cap did not fail closed globally: failed=%t reason=%q observations=%d poisoned=%v",
			sink.pairBudgetFailed, sink.pairBudgetFailure, sink.pairObservations, sink.poisoned)
	}
	if len(sink.pairLaneRows[pairRenderMMC]) != 0 || len(sink.pairLaneRows[pairRenderF2FS]) != 0 ||
		len(sink.pairTableRows[pairRenderMMC]) != 0 || len(sink.pairTableRows[pairRenderF2FS]) != 0 ||
		len(sink.structuredLaneRows[pairRenderMMC]) != 0 || len(sink.structuredLaneRows[pairRenderF2FS]) != 0 {
		t.Fatalf("budget failure retained subordinate proof maps: lanes=%v tables=%v structured=%v",
			sink.pairLaneRows, sink.pairTableRows, sink.structuredLaneRows)
	}
	if sink.withheldPairRowsForKind(pairRenderF2FS) != 2 || sink.withheldPairRowsForKind(pairRenderMMC) != 1 ||
		sink.withheldPairRowsForTable(pairRenderF2FS, "f2fs_write_begin") != 1 ||
		sink.withheldPairRowsForTable(pairRenderF2FS, "f2fs_write_end") != 1 ||
		sink.withheldPairRowsForTable(pairRenderMMC, "mmc_request_start") != 1 ||
		sink.withheldPairRowsFromCensus(pairRenderF2FS, f2fsCensus) != 2 ||
		sink.withheldPairRowsFromCensus(pairRenderMMC, mmcCensus) != 1 {
		t.Fatalf("scalar/table/census accounting drifted after map release: totals=%v tables=%v mmc=%+v f2fs=%+v",
			sink.pairRows, sink.pairTableTotals, mmcCensus, f2fsCensus)
	}
	if sink.stats.RowsAccepted != 4 || sink.publishableRows() != 1 || len(sink.chunks) == 0 {
		t.Fatalf("budget barrier damaged spill/non-pair accounting: stats=%+v publishable=%d chunks=%d",
			sink.stats, sink.publishableRows(), len(sink.chunks))
	}
	coverage := profilerF2FSPairBarrierCoverage(2, sink)
	if coverage.FieldSources["budget_fail_closed"] != "true" || coverage.FieldSources["budget_failure"] != "observations" ||
		!strings.Contains(profilerPairBudgetCaveat(sink), "budget_fail_closed=true reason=observations") {
		t.Fatalf("budget failure was not disclosed in coverage/caveat: coverage=%+v caveat=%q", coverage, profilerPairBudgetCaveat(sink))
	}
	ledger := []TraceDBCoverage{
		{Family: "builtin_modern_profiler", Table: "plugin:ftrace-plugin", RowsRead: 1, RowsEmitted: 3, FieldSources: map[string]string{
			profilerCoverageF2FSStagedRows: "2", profilerCoverageMMCStagedRows: "1",
		}},
		{Table: "f2fs_write_begin", RowsRead: 1, RowsEmitted: 1},
		{Table: "f2fs_write_end", RowsRead: 1, RowsEmitted: 1},
		{Table: "mmc_request_start", RowsRead: 1, RowsEmitted: 1},
	}
	publishers := []profilerPairPublisherCensus{{coverageIndex: 0, mmc: mmcCensus, f2fs: f2fsCensus}}
	if err := reconcileProfilerMMCCoverage(ledger, sink, publishers); err != nil {
		t.Fatal(err)
	}
	if err := reconcileProfilerF2FSCoverage(ledger, sink, publishers); err != nil {
		t.Fatal(err)
	}
	for index, item := range ledger {
		if item.RowsEmitted != 0 || item.RowsEmitted > item.RowsRead {
			t.Fatalf("budget reconciliation left a published pair row at ledger[%d]: %+v", index, item)
		}
	}
	if ledger[0].FieldSources["complete_capture_withheld_rows"] != "3" {
		t.Fatalf("publisher RowsRead/RowsEmitted reconciliation did not account once: %+v", ledger[0])
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
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
	sink.pairObservationLimit = 10
	sink.pairLaneLimit = 2
	sink.poisonPairLane(pairRenderF2FS, "f2fs-a")
	sink.poisonPairLane(pairRenderF2FS, "f2fs-b")
	sink.poisonPairLane(pairRenderF2FS, "f2fs-cap-plus-one")
	if !sink.pairBudgetFailed || sink.pairBudgetFailure != "lane_keys" || sink.pairUniqueLanes != 2 ||
		sink.pairObservations != 3 || !sink.poisoned[pairRenderMMC] || !sink.poisoned[pairRenderF2FS] ||
		len(sink.poisonedLanes[pairRenderMMC]) != 0 || len(sink.poisonedLanes[pairRenderF2FS]) != 0 {
		t.Fatalf("invalid-only lane cap did not release maps/fail both families: observations=%d lanes=%d failed=%t reason=%q poisoned=%v lane_maps=%v",
			sink.pairObservations, sink.pairUniqueLanes, sink.pairBudgetFailed, sink.pairBudgetFailure, sink.poisoned, sink.poisonedLanes)
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
	if sink.pairObservations != 1 || sink.pairUniqueLanes != 1 || len(sink.pairLaneRows[pairRenderF2FS]) != 0 ||
		len(sink.pairTableRows[pairRenderF2FS]) != 0 || len(sink.poisonedLanes[pairRenderF2FS]) != 0 ||
		sink.pairRows[pairRenderF2FS] != 65 || sink.pairTableTotals[pairRenderF2FS]["f2fs_write_end"] != 64 {
		t.Fatalf("family poison continued growing subordinate maps or lost scalar totals: observations=%d lanes=%d laneRows=%v tableRows=%v totals=%v",
			sink.pairObservations, sink.pairUniqueLanes, sink.pairLaneRows, sink.pairTableRows, sink.pairTableTotals)
	}
}

func TestProfilerWithheldCensusAndTableWalkPublisherRowsNotGlobalPoisonSet(t *testing.T) {
	source := mustReadRendererSource(t, "streamerdb_sorter.go")
	censusBody := sourceBetweenProfilerPairFunctions(t, source,
		"func (s *traceDBRowSink) withheldPairRowsFromCensus", "func (s *traceDBRowSink) withheldPairRowsForTable")
	if !strings.Contains(censusBody, "for lane, count := range census.byLane") || strings.Contains(censusBody, "range s.poisonedLanes") {
		t.Fatalf("publisher reconciliation must be O(publisher lanes), not O(global poison lanes):\n%s", censusBody)
	}
	tableBody := sourceBetweenProfilerPairFunctions(t, source,
		"func (s *traceDBRowSink) withheldPairRowsForTable", "func (s *traceDBRowSink) publishableRows")
	if !strings.Contains(tableBody, "for lane, count := range lanes") || strings.Contains(tableBody, "range s.poisonedLanes") {
		t.Fatalf("table reconciliation must be O(table lanes), not O(global poison lanes):\n%s", tableBody)
	}

	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	for i := 0; i < 32; i++ {
		sink.poisonPairLane(pairRenderF2FS, fmt.Sprintf("poison-%d", i))
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
