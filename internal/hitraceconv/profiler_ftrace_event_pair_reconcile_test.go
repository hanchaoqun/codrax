package hitraceconv

import "testing"

// P1-a2.2-B2 regression: event coverage belongs only to typed structured
// events. A text-compatible row can share the rendered table name, but its
// lane-local withholding must be charged only to its plugin publisher. Table
// text is not sufficient provenance for subtracting structured event rows.
func TestProfilerF2FSCoverageDoesNotChargeTextWithholdingToStructuredEvent(t *testing.T) {
	for _, textFirst := range []bool{false, true} {
		name := "structured_first"
		if textFirst {
			name = "text_first"
		}
		t.Run(name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()

			structured := renderedRow{
				tsNS: 1, seq: 1, line: "structured-f2fs-write-begin",
				pairKind: pairRenderF2FS, pairLane: "structured-lane-a",
				pairTable: "f2fs_write_begin", structuredPair: true, profilerEventField: 4011,
			}
			text := renderedRow{
				tsNS: 2, seq: 2, line: "text-f2fs-write-begin",
				pairKind: pairRenderF2FS, pairLane: "text-lane-b",
				pairTable: "f2fs_write_begin",
			}

			var textStaged profilerPairCensusSet
			addStructured := func() {
				t.Helper()
				if err := sink.add(structured); err != nil {
					t.Fatal(err)
				}
			}
			addText := func() {
				t.Helper()
				if !sink.beginPairRowCensus() {
					t.Fatal("text publisher census did not start")
				}
				if err := sink.add(text); err != nil {
					t.Fatal(err)
				}
				textStaged = sink.endPairRowCensus()
			}
			if textFirst {
				addText()
				addStructured()
			} else {
				addStructured()
				addText()
			}

			sink.poisonPairLane(pairRenderF2FS, "text-lane-b")
			if withheld := sink.withheldPairRowsForKind(pairRenderF2FS); withheld != 1 ||
				sink.withheldStructuredPairRowsForKind(pairRenderF2FS) != 0 || sink.publishableRows() != 1 {
				t.Fatalf("fixture did not isolate text-only withholding: withheld=%d structured=%d publishable=%d",
					withheld, sink.withheldStructuredPairRowsForKind(pairRenderF2FS), sink.publishableRows())
			}

			coverage := []TraceDBCoverage{
				{
					Family: "builtin_modern_ftrace:f2fs", Table: "f2fs_write_begin",
					Role: "query_ready_export", Found: true, RowsRead: 1, RowsEmitted: 1,
				},
				{
					Family: "builtin_modern_profiler", Table: "plugin:__other_text__",
					Role: "query_ready_export", Found: true, RowsRead: 1, RowsEmitted: 1,
					FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"},
				},
			}
			publishers := []profilerPairPublisherCensus{{coverageIndex: 1, staged: textStaged}}
			var eventIndexes profilerFtraceEventCoverageIndexes
			slot := profilerFtraceEventSlot(4011)
			eventIndexes.Present[slot] = true
			eventIndexes.Index[slot] = 0
			if err := reconcileProfilerF2FSCoverage(coverage, sink, publishers, eventIndexes); err != nil {
				t.Fatal(err)
			}

			if coverage[0].RowsEmitted != 1 || coverage[1].RowsEmitted != 0 {
				t.Fatalf("text-only withholding crossed provenance lanes: structured_event=%d text_plugin=%d coverage=%+v",
					coverage[0].RowsEmitted, coverage[1].RowsEmitted, coverage)
			}
			if got := coverage[1].FieldSources["complete_capture_withheld_rows"]; got != "1" {
				t.Fatalf("text plugin publisher lost its exact withheld count: got=%q coverage=%+v", got, coverage[1])
			}
		})
	}
}
