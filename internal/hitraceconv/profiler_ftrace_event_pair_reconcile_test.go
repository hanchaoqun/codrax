package hitraceconv

import (
	"context"
	"testing"
)

// P1-a2.2-B2 regression: event coverage belongs only to typed structured
// events. A text-compatible row can share the rendered table name, but its
// terminal withholding must be projected only onto its fixed plugin publisher
// coverage. Table text is not sufficient provenance for subtracting structured
// event rows.
func TestProfilerF2FSTerminalPublicationDoesNotChargeTextWithholdingToStructuredEvent(t *testing.T) {
	for _, textFirst := range []bool{false, true} {
		name := "structured_first"
		if textFirst {
			name = "text_first"
		}
		t.Run(name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{},
			)
			defer sink.cleanup()

			addStructured := func() {
				t.Helper()
				if !sink.beginPairRowCensusForPublisher(profilerPairPublisherExactFtrace) {
					t.Fatal("structured publisher census did not start")
				}
				if err := sink.addProfilerEventContext(context.Background(), renderedRow{
					tsNS: 1, seq: 1, line: "structured-f2fs-write-begin",
					pairKind: pairRenderF2FS, pairLane: "structured-lane-a",
					pairTable: "f2fs_write_begin", structuredPair: true,
					profilerEventField: 4011,
				}, traceDBProfilerEventDelta{}); err != nil {
					t.Fatal(err)
				}
				staged := sink.endPairRowCensus()
				if staged[pairRenderF2FS].total != 1 {
					t.Fatalf("structured publisher staged rows=%d want=1", staged[pairRenderF2FS].total)
				}
			}
			addText := func() {
				t.Helper()
				if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
					!sink.beginProfilerTextMessage() {
					t.Fatal("text publisher message did not start")
				}
				if err := sink.addProfilerEventContext(context.Background(), renderedRow{
					tsNS: 2, seq: 2, line: "text-f2fs-write-begin",
					pairKind: pairRenderF2FS, pairLane: "text-lane-b",
					pairTable:            "f2fs_write_begin",
					profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
				}, traceDBProfilerEventDelta{}); err != nil {
					t.Fatal(err)
				}
				if err := sink.endProfilerTextMessage(1); err != nil {
					t.Fatal(err)
				}
				staged := sink.endPairRowCensus()
				if staged[pairRenderF2FS].total != 1 {
					t.Fatalf("text publisher staged rows=%d want=1", staged[pairRenderF2FS].total)
				}
			}
			if textFirst {
				addText()
				addStructured()
			} else {
				addStructured()
				addText()
			}

			sink.poisonPairLane(pairRenderF2FS, "text-lane-b")
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}

			extraction := profilerContainerExtraction{
				Messages:                 2,
				StructuredFtrace:         1,
				StructuredRows:           1,
				TextPluginMessages:       1,
				TextRows:                 1,
				publicationCaveatPending: true,
				TraceCoverage: []TraceDBCoverage{
					{
						Family: "builtin_modern_ftrace:f2fs", Table: "f2fs_write_begin",
						Role: "query_ready_export", Found: true, RowsRead: 1, RowsEmitted: 1,
					},
					{
						Family: "builtin_modern_profiler", Table: "plugin:ftrace-plugin",
						Role: "query_ready_export", Found: true, RowsRead: 1, RowsEmitted: 1,
						FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"},
					},
					{
						Family: "builtin_modern_profiler", Table: "plugin:__other_text__",
						Role: "query_ready_export", Found: true, RowsRead: 1, RowsEmitted: 1,
						FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"},
					},
				},
			}
			if !extraction.profilerPublisherCoverage.observe(profilerPairPublisherExactFtrace, 1) ||
				!extraction.profilerPublisherCoverage.observe(profilerPairPublisherOtherText, 2) {
				t.Fatal("record fixed publisher coverage")
			}
			slot := profilerFtraceEventSlot(4011)
			extraction.profilerEventCoverage.Present[slot] = true
			extraction.profilerEventCoverage.Index[slot] = 0

			terminal, err := applyProfilerTerminalPublication(&extraction, sink)
			if err != nil {
				t.Fatal(err)
			}
			if terminal.structuredEndpoints[profilerPairEndpointF2FSWriteBegin] !=
				(profilerTerminalPublicationCounts{staged: 1, published: 1}) ||
				terminal.publisherFamilies[profilerPairPublisherOtherText][pairRenderF2FS] !=
					(profilerTerminalPublicationCounts{staged: 1, withheld: 1}) {
				t.Fatalf("terminal provenance verdict drifted: %+v", terminal)
			}
			if extraction.TraceCoverage[0].RowsEmitted != 1 ||
				extraction.TraceCoverage[0].FieldSources["complete_capture_withheld_rows"] != "" ||
				extraction.TraceCoverage[1].RowsEmitted != 1 ||
				extraction.TraceCoverage[2].RowsEmitted != 0 ||
				extraction.TraceCoverage[2].FieldSources["complete_capture_withheld_rows"] != "1" {
				t.Fatalf("text-only withholding crossed terminal provenance lanes: %+v", extraction.TraceCoverage)
			}
			if extraction.StructuredRows != 1 || extraction.TextRows != 0 ||
				extraction.TextPluginMessages != 0 || extraction.publicationCaveatPending ||
				!extraction.terminalPublicationApplied {
				t.Fatalf("terminal public counters were not committed atomically: %+v", extraction)
			}

			invalid := profilerContainerExtraction{
				StructuredRows:           1,
				TextPluginMessages:       1,
				TextRows:                 1,
				publicationCaveatPending: true,
				TraceCoverage: []TraceDBCoverage{
					{RowsEmitted: 0},
					{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
					{RowsEmitted: 1, FieldSources: map[string]string{profilerCoverageF2FSStagedRows: "1"}},
				},
			}
			invalid.profilerPublisherCoverage = extraction.profilerPublisherCoverage
			invalid.profilerEventCoverage = extraction.profilerEventCoverage
			requireProfilerTerminalApplyErrorUnchanged(t, invalid, sink,
				"profiler_terminal_publication_event_coverage_mismatch")
		})
	}
}
