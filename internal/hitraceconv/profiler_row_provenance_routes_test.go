package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfilerRowProvenanceRealPublisherRoutes(t *testing.T) {
	t.Run("exact strict text", func(t *testing.T) {
		start, _, done := profilerStrictTextF2FSFixture(t)
		ordinary := traceDBFormatLine("worker", 7, 7, 1, 1_001_000_000, 0, 0,
			"print: B|7|Strict")
		payload := []byte(strings.Join([]string{start, ordinary, done, ""}, "\n"))
		extracted, sink := extractSyntheticProfilerContainer(t,
			syntheticProfilerPluginData("ftrace-plugin", payload))
		defer sink.cleanup()

		if extracted.TextRows != 3 || extracted.TextPluginMessages != 1 ||
			extracted.StructuredRows != 0 || sink.stats.RowsAccepted != 3 {
			t.Fatalf("exact strict production route drifted: extracted=%+v stats=%+v", extracted, sink.stats)
		}
		begin := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteBegin)
		end := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteEnd)
		profilerAssertRouteProvenance(t, begin, profilerPairPublisherExactFtrace,
			profilerPairRowProvenanceText, 1, pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true)
		profilerAssertRouteProvenance(t, end, profilerPairPublisherExactFtrace,
			profilerPairRowProvenanceText, 1, pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, true)
		profilerAssertSameRouteLane(t, begin, end)
		profilerAssertRouteProvenance(t, profilerRouteLineRow(t, sink.rows, "B|7|Strict"),
			profilerPairPublisherExactFtrace, profilerPairRowProvenanceText, 1,
			pairRenderUnknown, profilerPairEndpointNone, false)
		profilerAssertRouteContextsClosed(t, sink, 1)
	})

	t.Run("exact structured", func(t *testing.T) {
		cases := profilerAuxCasesByField()
		structured := protoMessage(2, protoPayload(
			protoVarint(1, 2),
			syntheticTracePluginFtraceEvent(2_000, 40, 40, "f2fs", 4011,
				profilerAuxEncodeValues(cases[4011].values)),
			syntheticTracePluginFtraceEvent(3_000, 40, 40, "worker", 1109,
				profilerAuxEncodeValues(cases[1109].values)),
			syntheticTracePluginFtraceEvent(4_000, 40, 40, "f2fs", 4012,
				profilerAuxEncodeValues(cases[4012].values)),
		))
		extracted, sink := extractSyntheticProfilerContainer(t,
			syntheticProfilerPluginData("ftrace-plugin", structured))
		defer sink.cleanup()

		if extracted.StructuredRows != 3 || extracted.TextRows != 0 ||
			extracted.TextPluginMessages != 0 || sink.stats.RowsAccepted != 3 {
			t.Fatalf("exact structured production route drifted: extracted=%+v stats=%+v", extracted, sink.stats)
		}
		begin := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteBegin)
		end := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteEnd)
		profilerAssertRouteProvenance(t, begin, profilerPairPublisherExactFtrace,
			profilerPairRowProvenanceStructured, 0, pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true)
		profilerAssertRouteProvenance(t, end, profilerPairPublisherExactFtrace,
			profilerPairRowProvenanceStructured, 0, pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, true)
		if begin.profilerEventField != 4011 || end.profilerEventField != 4012 ||
			!begin.structuredPair || !end.structuredPair {
			t.Fatalf("structured endpoint identity drifted: begin=%+v end=%+v", begin, end)
		}
		profilerAssertSameRouteLane(t, begin, end)
		ordinary := profilerRouteLineRow(t, sink.rows, "print: B|7|Frame")
		profilerAssertRouteProvenance(t, ordinary, profilerPairPublisherExactFtrace, 0, 0,
			pairRenderUnknown, profilerPairEndpointNone, false)
		if ordinary.structuredPair || ordinary.profilerEventField != 0 {
			t.Fatalf("ordinary structured event acquired pair identity: %+v", ordinary)
		}
		profilerAssertRouteContextsClosed(t, sink, 0)
	})

	t.Run("bytrace other text and message ordinal", func(t *testing.T) {
		start, _, done := profilerStrictTextF2FSFixture(t)
		mixedOrdinary := traceDBFormatLine("worker", 7, 7, 1, 1_001_000_000, 0, 0,
			"print: B|7|Mixed")
		bytraceOrdinary := traceDBFormatLine("worker", 7, 7, 1, 1_003_000_000, 0, 0,
			"print: B|7|Bytrace")
		messages := [][]byte{
			syntheticProfilerPluginData("other-plugin", []byte("# tracer: nop\n")),
			syntheticProfilerPluginData("other-plugin",
				[]byte(strings.Join([]string{start, mixedOrdinary, done, ""}, "\n"))),
			syntheticProfilerPluginData("bytrace_plugin", []byte(bytraceOrdinary+"\n")),
		}
		extracted, sink := extractSyntheticProfilerContainer(t, messages...)
		defer sink.cleanup()

		if extracted.Messages != 3 || extracted.TextPluginMessages != 2 || extracted.TextRows != 4 ||
			sink.stats.RowsAccepted != 4 || len(sink.rows) != 4 {
			t.Fatalf("real text publisher/ordinal route drifted: extracted=%+v stats=%+v rows=%+v",
				extracted, sink.stats, sink.rows)
		}
		begin := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteBegin)
		end := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteEnd)
		profilerAssertRouteProvenance(t, begin, profilerPairPublisherOtherText,
			profilerPairRowProvenanceText, 1, pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true)
		profilerAssertRouteProvenance(t, end, profilerPairPublisherOtherText,
			profilerPairRowProvenanceText, 1, pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, true)
		profilerAssertSameRouteLane(t, begin, end)
		profilerAssertRouteProvenance(t, profilerRouteLineRow(t, sink.rows, "B|7|Mixed"),
			profilerPairPublisherOtherText, profilerPairRowProvenanceText, 1,
			pairRenderUnknown, profilerPairEndpointNone, false)
		profilerAssertRouteProvenance(t, profilerRouteLineRow(t, sink.rows, "B|7|Bytrace"),
			profilerPairPublisherBytrace, profilerPairRowProvenanceText, 2,
			pairRenderUnknown, profilerPairEndpointNone, false)
		// The first, rowless physical message must not consume ordinal 1. All
		// three rows in the mixed second message share 1; the next row-bearing
		// physical message advances exactly once to 2.
		profilerAssertRouteContextsClosed(t, sink, 2)
	})

	t.Run("noncanonical ftrace is coverage only", func(t *testing.T) {
		hidden := traceDBFormatLine("worker", 7, 7, 1, 1_000_000_000, 0, 0,
			"print: B|7|MustNotPublish")
		visible := traceDBFormatLine("worker", 7, 7, 1, 1_001_000_000, 0, 0,
			"print: B|7|AfterNoncanonical")
		extracted, sink := extractSyntheticProfilerContainer(t,
			syntheticProfilerPluginData("FTRACE-PLUGIN", []byte(hidden+"\n")),
			syntheticProfilerPluginData("other-plugin", []byte(visible+"\n")),
		)
		defer sink.cleanup()

		if extracted.Messages != 2 || extracted.UnsupportedFtrace != 1 ||
			extracted.TextPluginMessages != 1 || extracted.TextRows != 1 ||
			sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 ||
			strings.Contains(sink.rows[0].line, "MustNotPublish") {
			t.Fatalf("noncanonical ftrace acquired a publisher row: extracted=%+v stats=%+v rows=%+v",
				extracted, sink.stats, sink.rows)
		}
		profilerAssertRouteProvenance(t, profilerRouteLineRow(t, sink.rows, "B|7|AfterNoncanonical"),
			profilerPairPublisherOtherText, profilerPairRowProvenanceText, 1,
			pairRenderUnknown, profilerPairEndpointNone, false)
		// The coverage-only noncanonical frame must not enter text-message state
		// or consume the first row-bearing message ordinal.
		profilerAssertRouteContextsClosed(t, sink, 1)
	})

	t.Run("session", func(t *testing.T) {
		start, _, done := profilerStrictTextF2FSFixture(t)
		ordinary := traceDBFormatLine("worker", 7, 7, 1, 1_001_000_000, 0, 0,
			"print: B|7|Session")
		payload := strings.Join([]string{profilerSessionJSONTag, start, ordinary, done, ""}, "\n")
		input := filepath.Join(t.TempDir(), "session-provenance.htrace")
		if err := os.WriteFile(input, []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 128)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		extracted, err := extractProfilerContainerSystraceRows(
			context.Background(), input, int64(len(payload)), sink)
		if err != nil {
			t.Fatal(err)
		}
		if !extracted.Detected || extracted.Kind != "openharmony_profiler_session_package" ||
			extracted.TextRows != 3 || sink.stats.RowsAccepted != 3 {
			t.Fatalf("Session production extraction route drifted: extracted=%+v stats=%+v", extracted, sink.stats)
		}
		begin := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteBegin)
		end := profilerRoutePairRow(t, sink.rows, profilerPairEndpointF2FSWriteEnd)
		profilerAssertRouteProvenance(t, begin, profilerPairPublisherSession, 0, 0,
			pairRenderF2FS, profilerPairEndpointF2FSWriteBegin, true)
		profilerAssertRouteProvenance(t, end, profilerPairPublisherSession, 0, 0,
			pairRenderF2FS, profilerPairEndpointF2FSWriteEnd, true)
		profilerAssertSameRouteLane(t, begin, end)
		profilerAssertRouteProvenance(t, profilerRouteLineRow(t, sink.rows, "B|7|Session"),
			profilerPairPublisherSession, 0, 0, pairRenderUnknown, profilerPairEndpointNone, false)
		profilerAssertRouteContextsClosed(t, sink, 0)
	})
}

func profilerRoutePairRow(t testing.TB, rows []renderedRow, endpoint profilerPairEndpointSlot) renderedRow {
	t.Helper()
	var found renderedRow
	count := 0
	for _, row := range rows {
		if row.profilerEndpointSlot == endpoint {
			found = row
			count++
		}
	}
	if count != 1 {
		t.Fatalf("endpoint=%d row count=%d rows=%+v", endpoint, count, rows)
	}
	return found
}

func profilerRouteLineRow(t testing.TB, rows []renderedRow, marker string) renderedRow {
	t.Helper()
	var found renderedRow
	count := 0
	for _, row := range rows {
		if strings.Contains(row.line, marker) {
			found = row
			count++
		}
	}
	if count != 1 {
		t.Fatalf("marker=%q row count=%d rows=%+v", marker, count, rows)
	}
	return found
}

func profilerAssertRouteProvenance(t testing.TB, row renderedRow,
	publisher profilerPairPublisherSlot, flags profilerPairRowProvenanceFlags, ordinal uint32,
	kind pairRenderKind, endpoint profilerPairEndpointSlot, wantLane bool,
) {
	t.Helper()
	got := row.profilerProvenance()
	if !got.valid() || got.PublisherSlot != publisher || got.Flags != flags ||
		got.TextMessageOrdinal != ordinal || got.PairKind != kind || got.EndpointSlot != endpoint {
		t.Fatalf("route provenance=%+v want publisher=%d flags=%d ordinal=%d kind=%d endpoint=%d row=%+v",
			got, publisher, flags, ordinal, kind, endpoint, row)
	}
	if wantLane {
		descriptor, ok := endpoint.descriptor()
		if !ok || descriptor.kind != kind || row.pairTable != descriptor.name ||
			got.LaneID == 0 || row.pairLane == "" {
			t.Fatalf("pair route lost typed endpoint/lane identity: provenance=%+v row=%+v descriptor=%+v ok=%t",
				got, row, descriptor, ok)
		}
		return
	}
	if got.LaneID != 0 || row.pairLane != "" || row.pairTable != "" {
		t.Fatalf("ordinary route acquired pair lane/table identity: provenance=%+v row=%+v", got, row)
	}
}

func profilerAssertSameRouteLane(t testing.TB, left, right renderedRow) {
	t.Helper()
	if left.profilerLaneID == 0 || left.profilerLaneID != right.profilerLaneID ||
		left.pairLane == "" || left.pairLane != right.pairLane {
		t.Fatalf("paired endpoints do not share one typed lane: left=%+v right=%+v", left, right)
	}
}

func profilerAssertRouteContextsClosed(t testing.TB, sink *traceDBRowSink, wantNextOrdinal uint32) {
	t.Helper()
	if sink.pairCensusActive || sink.activePairPublisher != profilerPairPublisherNone ||
		sink.textMessageActive || sink.activeTextMessage != 0 || sink.activeTextRows != 0 ||
		sink.nextTextMessage != wantNextOrdinal {
		t.Fatalf("route returned with open/drifted publisher context: census=%t publisher=%d text=%t active=%d rows=%d next=%d want_next=%d",
			sink.pairCensusActive, sink.activePairPublisher, sink.textMessageActive,
			sink.activeTextMessage, sink.activeTextRows, sink.nextTextMessage, wantNextOrdinal)
	}
}
