package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestStructuredMMCBarrierSealsAcrossMessagesAndSpill(t *testing.T) {
	cases := profilerAuxCasesByField()
	start := profilerMMCTestStructuredMessage(4016, cases[4016].values, 1_000)
	done := profilerMMCTestStructuredMessage(4015, cases[4015].values, 4_000)
	badValues := profilerAuxCloneValues(cases[4015].values)
	badValues[22] = profilerAuxVarint(0)
	bad := profilerMMCTestStructuredMessage(4015, badValues, 2_000)
	printValues := profilerAuxCasesByField()[1109].values
	print := profilerMMCTestStructuredMessage(1109, printValues, 3_000)

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, message := range [][]byte{start, bad, print, done} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 2 ||
		sink.withheldStructuredPairRows() != 2 || sink.publishableRows() != 1 || len(sink.runs) == 0 {
		t.Fatalf("structured stage did not retain source-wide spill barrier: accepted=%d withheld=%d structured=%d publishable=%d chunks=%d poisoned=%v",
			sink.stats.RowsAccepted, sink.withheldPairRows(), sink.withheldStructuredPairRows(), sink.publishableRows(), len(sink.runs), sink.poisoned)
	}
	var out bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if stats.RowsAccepted != 3 || stats.RowsWritten != 1 || stats.RowsWithheld != 2 ||
		strings.Contains(text, "mmc_request_start:") || strings.Contains(text, "mmc_request_done:") ||
		!strings.Contains(text, "print: B|7|Frame") {
		t.Fatalf("structured MMC barrier leaked/suppressed wrong rows: stats=%+v\n%s", stats, text)
	}
}

func TestStructuredMMCFamilyProvenanceSurvivesAmbiguousOneof(t *testing.T) {
	cases := profilerAuxCasesByField()
	event := protoMessage(2,
		protoVarint(1, 1_000), protoVarint(2, 40), protoBytes(3, []byte("mmc")),
		protoMessage(50, protoVarint(4, 40)),
		protoMessage(4015, profilerAuxEncodeValues(cases[4015].values)),
		protoMessage(2003, protoPayload(protoVarint(1, 1_550_000), protoVarint(2, 2))),
	)
	detail := protoPayload(protoVarint(1, 2), event)
	result := decodeProfilerTracePluginResult(protoMessage(2, detail))
	events, err := profilerTracePluginResultEvents(result)
	if err != nil || len(events) != 1 || events[0].Field != 0 ||
		events[0].PairFamilies&pairCriticalFormatFamilyMMC == 0 {
		t.Fatalf("ambiguous exact oneof lost MMC provenance: events=%+v err=%v", events, err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if rows, _, renderErr := renderProfilerFtraceStructuredResult(result, &seq, sink); renderErr != nil || rows != 0 || !sink.pairKindPoisoned(pairRenderMMC) {
		t.Fatalf("ambiguous MMC oneof did not close family: rows=%d poisoned=%v err=%v", rows, sink.poisoned, renderErr)
	}

	unknownEvent := protoMessage(2,
		protoVarint(1, 2_000), protoMessage(50, protoVarint(4, 40)), protoMessage(9999, []byte{0x80}),
	)
	unknownResult := decodeProfilerTracePluginResult(protoMessage(2, protoPayload(protoVarint(1, 2), unknownEvent)))
	cleanSink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanSink.cleanup()
	seq = 0
	_, _, _ = renderProfilerFtraceStructuredResult(unknownResult, &seq, cleanSink)
	if cleanSink.pairKindPoisoned(pairRenderMMC) {
		t.Fatal("unrelated unknown oneof guessed MMC family provenance")
	}
}

func TestStructuredMMCBarrierSurvivesMalformedAndTruncatedEnvelopes(t *testing.T) {
	cases := profilerAuxCasesByField()
	validStart := profilerMMCTestStructuredMessage(4016, cases[4016].values, 1_000)
	validDone := profilerMMCTestStructuredMessage(4015, cases[4015].values, 3_000)

	mmcPayload := profilerAuxEncodeValues(cases[4015].values)
	eventRecord := protoPayload(
		protoVarint(1, 2_000), protoVarint(2, 40), protoBytes(3, []byte("mmc")),
		protoMessage(50, protoVarint(4, 40)), protoMessage(4015, mmcPayload),
	)
	event := protoMessage(2, eventRecord)
	cpuDetail := protoPayload(protoVarint(1, 2), event)
	cpuDetailWithLateDamage := protoMessage(2, protoPayload(cpuDetail, []byte{0x80}))
	topLevelWithLateDamage := append(append([]byte(nil), profilerMMCTestStructuredMessage(4015, cases[4015].values, 2_000)...), 0x80)
	topLevelEarlyOpaque := protoPayload(protoBytes(7, []byte("v1")), []byte{0x0f}, protoMessage(2, cpuDetail))
	cpuDetailEarlyOpaque := protoMessage(2, protoPayload(protoVarint(1, 2), []byte{0x0f}, event))
	eventEarlyOpaque := protoMessage(2, protoPayload(protoVarint(1, 2), protoMessage(2,
		protoPayload(protoVarint(1, 2_000), []byte{0x0f}, protoMessage(4015, mmcPayload)))))
	topLevelTruncated := profilerMMCTruncatedMessage(2, cpuDetail)
	cpuDetailTruncated := protoMessage(2, protoPayload(protoVarint(1, 2), profilerMMCTruncatedMessage(2, eventRecord)))
	eventOneofTruncated := protoMessage(2, protoPayload(protoVarint(1, 2), protoMessage(2,
		protoPayload(protoVarint(1, 2_000), profilerMMCTruncatedMessage(4015, mmcPayload)))))

	for _, tc := range []struct {
		name      string
		malformed []byte
	}{
		{name: "top-level trailing malformed wire", malformed: topLevelWithLateDamage},
		{name: "cpu-detail trailing malformed wire", malformed: cpuDetailWithLateDamage},
		{name: "top-level early opaque wire", malformed: topLevelEarlyOpaque},
		{name: "cpu-detail early opaque wire", malformed: cpuDetailEarlyOpaque},
		{name: "event early opaque wire", malformed: eventEarlyOpaque},
		{name: "top-level truncated detail", malformed: topLevelTruncated},
		{name: "cpu-detail truncated event", malformed: cpuDetailTruncated},
		{name: "event truncated exact oneof", malformed: eventOneofTruncated},
	} {
		for _, order := range []struct {
			name     string
			messages [][]byte
		}{
			{name: "between endpoints", messages: [][]byte{validStart, tc.malformed, validDone}},
			{name: "before endpoints", messages: [][]byte{tc.malformed, validStart, validDone}},
		} {
			t.Run(tc.name+"/"+order.name, func(t *testing.T) {
				sink, err := newTraceDBRowSink(t.TempDir(), 1)
				if err != nil {
					t.Fatal(err)
				}
				defer sink.cleanup()
				seq := 0
				for _, message := range order.messages {
					if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
						t.Fatal(renderErr)
					}
				}
				if !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 2 || sink.publishableRows() != 0 {
					t.Fatalf("malformed envelope lost MMC capture barrier: accepted=%d withheld=%d opaque=%v poisoned=%v", sink.stats.RowsAccepted, sink.withheldPairRows(), sink.opaque, sink.poisoned)
				}
			})
		}
	}

	cleanSink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanSink.cleanup()
	seq := 0
	opaqueWithoutMMC := protoPayload(protoBytes(7, []byte("v1")), []byte{0x0f})
	print := profilerMMCTestStructuredMessage(1109, cases[1109].values, 4_000)
	for _, message := range [][]byte{opaqueWithoutMMC, print} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, cleanSink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if !cleanSink.opaque[pairRenderMMC] || cleanSink.pairKindPoisoned(pairRenderMMC) || cleanSink.publishableRows() != 1 {
		t.Fatalf("delayed structured opacity damaged a source with no MMC rows: opaque=%v poisoned=%v publishable=%d", cleanSink.opaque, cleanSink.poisoned, cleanSink.publishableRows())
	}
}

func TestProfilerContainerMMCCaptureCoverageMatchesPublishedRows(t *testing.T) {
	cases := profilerAuxCasesByField()
	badValues := profilerAuxCloneValues(cases[4015].values)
	badValues[22] = profilerAuxVarint(0)
	messages := [][]byte{
		syntheticProfilerPluginData("other-plugin", []byte("other-7 (7) [001] .... 0.900000: print: B|7|Other\n")),
		syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(4016, cases[4016].values, 1_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(4015, badValues, 2_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(1109, cases[1109].values, 3_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(4015, cases[4015].values, 4_000)),
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler-mmc.htrace")
	output := filepath.Join(dir, "profiler-mmc.ftrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(messages...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || strings.Contains(string(body), "mmc_request_") ||
		!strings.Contains(string(body), "print: B|7|Frame") || !strings.Contains(string(body), "print: B|7|Other") {
		t.Fatalf("profiler container barrier output mismatch: result=%+v\n%s", result, body)
	}
	pluginEmitted := 0
	otherPluginEmitted := 0
	startEmitted, doneEmitted := -1, -1
	barrierRows := -1
	for _, item := range result.TraceCoverage {
		if item.Family == "builtin_modern_profiler" && item.Table == "plugin:ftrace-plugin" {
			pluginEmitted += item.RowsEmitted
		}
		if item.Family == "builtin_modern_profiler" && item.Table == "plugin:__other_text__" {
			otherPluginEmitted += item.RowsEmitted
		}
		switch item.Table {
		case "mmc_request_start":
			startEmitted = item.RowsEmitted
		case "mmc_request_done":
			doneEmitted = item.RowsEmitted
		case "__complete_capture_barrier__":
			barrierRows = item.RowsRead
		}
	}
	joined := strings.Join(result.Caveats, "\n")
	if pluginEmitted != 1 || otherPluginEmitted != 1 || startEmitted != 0 || doneEmitted != 0 || barrierRows != 2 ||
		!strings.Contains(joined, "rendered 1 structured trace row(s)") ||
		!strings.Contains(joined, "withheld_rows=2") {
		t.Fatalf("profiler MMC coverage lied after seal: plugin=%d other=%d start=%d done=%d barrier=%d\n%s\n%+v",
			pluginEmitted, otherPluginEmitted, startEmitted, doneEmitted, barrierRows, joined, result.TraceCoverage)
	}
}

func TestProfilerOuterContainerOpacityCannotBridgeMMC(t *testing.T) {
	cases := profilerAuxCasesByField()
	hiddenDone := profilerMMCTestStructuredMessage(4015, cases[4015].values, 2_000)
	rejectedDuplicateData := protoPayload(
		protoBytes(1, []byte("ftrace-plugin")), protoVarint(2, 0),
		protoBytes(3, hiddenDone), protoBytes(3, hiddenDone),
	)
	for _, tc := range []struct {
		name   string
		hidden []byte
	}{
		{name: "rejected duplicate data frame", hidden: rejectedDuplicateData},
		{name: "case-drift ftrace plugin", hidden: syntheticProfilerPluginData("FTRACE-PLUGIN", hiddenDone)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := [][]byte{
				syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(4016, cases[4016].values, 1_000)),
				tc.hidden,
				syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(4015, cases[4015].values, 3_000)),
				syntheticProfilerPluginData("ftrace-plugin", profilerMMCTestStructuredMessage(1109, cases[1109].values, 4_000)),
			}
			dir := t.TempDir()
			input := filepath.Join(dir, "outer-opaque.htrace")
			output := filepath.Join(dir, "outer-opaque.ftrace")
			if err := os.WriteFile(input, syntheticProfilerTraceFile(messages...), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: "builtin"})
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			barrierRows := -1
			for _, item := range result.TraceCoverage {
				if item.Table == "__complete_capture_barrier__" {
					barrierRows = item.RowsRead
				}
			}
			if result.EventsWritten != 1 || barrierRows != 2 || strings.Contains(string(body), "mmc_request_") || !strings.Contains(string(body), "print: B|7|Frame") {
				t.Fatalf("outer opaque frame allowed MMC rescue: result=%+v barrier=%d\n%s", result, barrierRows, body)
			}
		})
	}
}

func TestProfilerTextMMCCoverageIsAttributedToAggregateBucket(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	badDoneLine := "io-100 (100) [002] .... 1.001000: mmc_request_done: " + doneBody + " extra=1"
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody

	messages := [][]byte{
		syntheticProfilerPluginData("other-plugin", []byte("other-7 (7) [001] .... 0.900000: print: B|7|Other\n")),
		syntheticProfilerPluginData("mmc-start-plugin", []byte(startLine+"\n")),
		syntheticProfilerPluginData("mmc-mixed-plugin", []byte(badDoneLine+"\nother-8 (8) [001] .... 1.001500: print: B|8|Keep\n")),
		syntheticProfilerPluginData("mmc-done-plugin", []byte(doneLine+"\n")),
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler-text-mmc.htrace")
	output := filepath.Join(dir, "profiler-text-mmc.ftrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(messages...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: "builtin"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || strings.Contains(string(body), "mmc_request_") ||
		!strings.Contains(string(body), "print: B|7|Other") || !strings.Contains(string(body), "print: B|8|Keep") {
		t.Fatalf("text MMC barrier output mismatch: result=%+v\n%s", result, body)
	}
	otherRows, otherRead := -1, -1
	barrierRows := -1
	for _, item := range result.TraceCoverage {
		if item.Table == "plugin:__other_text__" {
			otherRows, otherRead = item.RowsEmitted, item.RowsRead
		}
		if item.Table == "__complete_capture_barrier__" {
			barrierRows = item.RowsRead
		}
	}
	joined := strings.Join(result.Caveats, "\n")
	if otherRows != 2 || otherRead != 4 || barrierRows != 3 || !strings.Contains(joined, "extracted 2 systrace text row(s) from 2 profiler plugin message(s)") {
		t.Fatalf("text MMC aggregate coverage/counter ledger mismatch: rows=%d read=%d barrier=%d\n%s\n%+v", otherRows, otherRead, barrierRows, joined, result.TraceCoverage)
	}
}

func TestProfilerSessionMMCBarrierClosesMalformedAndOpaqueHoles(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody
	printLine := "other-7 (7) [001] .... 1.003000: print: B|7|Keep"

	for _, tc := range []struct {
		name         string
		hole         string
		withheldRows int
		sourceClosed bool
	}{
		{
			name:         "loose malformed exact header",
			hole:         "BROKEN [003] .... 1.001000: mmc_request_done: " + doneBody,
			withheldRows: 3,
		},
		{
			name:         "oversized opaque physical row",
			hole:         strings.Repeat("x", maxProfilerTextLineBytes+1),
			withheldRows: 2,
			sourceClosed: true,
		},
		{
			name:         "negative timestamp exact header",
			hole:         "BROKEN [003] .... -1.001000: mmc_request_done: " + doneBody,
			withheldRows: 2,
		},
		{
			name:         "overflow timestamp exact header",
			hole:         "BROKEN [003] .... " + strings.Repeat("9", 400) + ": mmc_request_done: " + doneBody,
			withheldRows: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "session.htrace")
			output := filepath.Join(dir, "session.ftrace")
			payload := strings.Join([]string{"SessionJSON-", startLine, tc.hole, doneLine, printLine, ""}, "\n")
			if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: "builtin"})
			if err != nil {
				t.Fatal(err)
			}
			if tc.sourceClosed {
				resourceDecision := false
				for _, decision := range result.TraceDecisions {
					resourceDecision = resourceDecision || decision.Reason == "profiler_source_resource_fail_closed"
				}
				if result.OutputPath != "" || result.EventsWritten != 0 ||
					!resourceDecision ||
					!coverageTableHasSkipped(result.TraceCoverage, "__container_resource_barrier__", "profiler_source_fail_closed=session_line_size_budget_exceeded") {
					t.Fatalf("oversized Session record did not fail-close the complete profiler trace body: %+v coverage=%+v", result, result.TraceCoverage)
				}
				if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
					t.Fatalf("source-failed Session unexpectedly created output: stat_err=%v", statErr)
				}
				return
			}
			body, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if result.EventsWritten != 1 || strings.Contains(string(body), "mmc_request_") || !strings.Contains(string(body), "print: B|7|Keep") {
				t.Fatalf("session MMC barrier leaked or suppressed sibling: result=%+v\n%s", result, body)
			}
			sessionRows, barrierRows := -1, -1
			for _, item := range result.TraceCoverage {
				switch item.Table {
				case "session:SessionJSON":
					sessionRows = item.RowsEmitted
				case "__complete_capture_barrier__":
					barrierRows = item.RowsRead
				}
			}
			if sessionRows != 1 || barrierRows != tc.withheldRows {
				t.Fatalf("session MMC ledger mismatch: session=%d barrier=%d want=%d coverage=%+v", sessionRows, barrierRows, tc.withheldRows, result.TraceCoverage)
			}
			index, err := tracequery.BuildIndex(context.Background(), output)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range tracequery.ComputeWindowStats(index, tracequery.Query{}).StorageLatencyByLayer {
				if row.Layer == "mmc" && row.Event == "mmc_request" && row.PairedCount > 0 {
					t.Fatalf("session hole rescued an MMC duration: %+v", row)
				}
			}
		})
	}
}

func TestProfilerTextMMCPublishersShareSourceBarrier(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	badLine := "io-100 (100) [002] .... 1.001000: mmc_request_done: " + doneBody + " extra=1"
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody

	for _, lane := range []struct {
		name string
		add  func(string, *int, *traceDBRowSink) error
	}{
		{name: "strict ftrace-plugin", add: func(line string, seq *int, sink *traceDBRowSink) error {
			_, accepted, err := addStrictSystraceRowsFromBytes([]byte(line+"\n"), seq, sink)
			if err == nil && !accepted {
				t.Fatalf("strict publisher rejected fixture line: %q", line)
			}
			return err
		}},
		{name: "generic plugin", add: func(line string, seq *int, sink *traceDBRowSink) error {
			_, err := addSystraceRowsFromBytes([]byte(line+"\n"), seq, sink)
			return err
		}},
	} {
		t.Run(lane.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			for _, line := range []string{startLine, badLine, doneLine} {
				if err := lane.add(line, &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 3 || sink.publishableRows() != 0 {
				t.Fatalf("text publisher bypassed source barrier: accepted=%d withheld=%d poisoned=%v", sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned)
			}
		})
	}
}

func TestProfilerGenericMalformedTimestampCannotBridgeMMC(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody
	for _, timestamp := range []string{"NaN", "1.2.3", "1e3"} {
		t.Run(timestamp, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			bad := "io-100 (100) [002] .... " + timestamp + ": mmc_request_done: " + doneBody
			for _, line := range []string{startLine, bad, doneLine, "other-7 (7) [001] .... 1.003000: print: B|7|Keep"} {
				if _, err := addSystraceRowsFromBytes([]byte(line+"\n"), &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if !sink.poisoned[pairRenderMMC] || sink.withheldPairRowsForKind(pairRenderMMC) != 2 || sink.publishableRows() != 1 {
				t.Fatalf("malformed %q MMC endpoint became an invisible bridge: accepted=%d withheld=%d poisoned=%v",
					timestamp, sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned)
			}
		})
	}
}

func TestStrictProfilerTextRejectStillPoisonsObservedMMCSource(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(startLine+"\n"), &seq, sink); err != nil || !accepted {
		t.Fatalf("valid start message rejected: accepted=%t err=%v", accepted, err)
	}
	// This whole compatibility payload is rejected because of its second
	// fragment. The exact done line it already exposed is still precise family
	// provenance and must close the source before a later done can rescue it.
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(doneLine+"\nnot-a-trace\n"), &seq, sink); err != nil || accepted {
		t.Fatalf("mixed strict payload verdict: accepted=%t err=%v", accepted, err)
	}
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(doneLine+"\n"), &seq, sink); err != nil || !accepted {
		t.Fatalf("valid trailing done message rejected: accepted=%t err=%v", accepted, err)
	}
	if !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 2 || sink.publishableRows() != 0 {
		t.Fatalf("rejected strict payload created a cross-message hole: accepted=%d withheld=%d poisoned=%v", sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned)
	}
}

func TestStrictProfilerTextCensusScansPastEarlyReject(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(startLine+"\n"), &seq, sink); err != nil || !accepted {
		t.Fatalf("valid start message rejected: accepted=%t err=%v", accepted, err)
	}
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte("not-a-trace\n"+doneLine+"\n"), &seq, sink); err != nil || accepted {
		t.Fatalf("early-malformed payload verdict: accepted=%t err=%v", accepted, err)
	}
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(doneLine+"\n"), &seq, sink); err != nil || !accepted {
		t.Fatalf("valid trailing done rejected: accepted=%t err=%v", accepted, err)
	}
	if !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 2 || sink.publishableRows() != 0 {
		t.Fatalf("early reject hid later exact MMC provenance: accepted=%d withheld=%d poisoned=%v", sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned)
	}
}

func TestStrictProfilerRejectedOpaquePayloadCannotBridgeStructuredMMC(t *testing.T) {
	cases := profilerAuxCasesByField()
	start := profilerMMCTestStructuredMessage(4016, cases[4016].values, 1_000)
	done := profilerMMCTestStructuredMessage(4015, cases[4015].values, 3_000)
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if _, _, err := renderProfilerFtraceStructuredRows(start, &seq, sink); err != nil {
		t.Fatal(err)
	}
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte{0x00, 0x80}, &seq, sink); err != nil || accepted {
		t.Fatalf("opaque exact-plugin payload verdict: accepted=%t err=%v", accepted, err)
	}
	if _, _, err := renderProfilerFtraceStructuredRows(done, &seq, sink); err != nil {
		t.Fatal(err)
	}
	if !sink.opaque[pairRenderMMC] || !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 2 || sink.publishableRows() != 0 {
		t.Fatalf("opaque exact-plugin payload allowed structured rescue: accepted=%d opaque=%v poisoned=%v", sink.stats.RowsAccepted, sink.opaque, sink.poisoned)
	}
}

func TestProfilerTextOversizedOpaqueRowCannotBridgeMMC(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody
	opaque := strings.Repeat("x", maxProfilerTextLineBytes+1)

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(opaque+"\n"), &seq, sink); err != nil || accepted {
		t.Fatalf("oversized opaque payload verdict: accepted=%t err=%v", accepted, err)
	}
	for _, line := range []string{startLine, doneLine} {
		if _, accepted, err := addStrictSystraceRowsFromBytes([]byte(line+"\n"), &seq, sink); err != nil || !accepted {
			t.Fatalf("valid endpoint rejected after opaque row: accepted=%t err=%v", accepted, err)
		}
	}
	if !sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRows() != 2 || sink.publishableRows() != 0 {
		t.Fatalf("opaque oversized row allowed endpoint rescue: accepted=%d withheld=%d poisoned=%v", sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned)
	}
}

func TestMMCStructuredPublisherCensusUsesOneBarrier(t *testing.T) {
	for _, file := range []string{"profiler_ftrace_render.go", "profiler_ftrace_authority.go", "profiler_container.go"} {
		source := mustReadRendererSource(t, file)
		if !strings.Contains(source, "profilerTextPairAdmission(") && file != "profiler_ftrace_render.go" {
			t.Fatalf("profiler text publisher %s bypasses pair admission", file)
		}
	}
}

func profilerMMCTestStructuredMessage(field int, values map[int]profilerAuxTestValue, ts uint64) []byte {
	detail := protoPayload(
		protoVarint(1, 2),
		syntheticTracePluginFtraceEvent(ts, 40, 40, "mmc", field, profilerAuxEncodeValues(values)),
	)
	return protoMessage(2, detail)
}

func profilerMMCTruncatedMessage(field int, payload []byte) []byte {
	var out bytes.Buffer
	writeProtoVarint(&out, uint64(field<<3|2))
	writeProtoVarint(&out, uint64(len(payload)+1))
	out.Write(payload)
	return out.Bytes()
}

func decodeDirectMMCPayloadFromFixtureForTest(t *testing.T, name string) (mmcPayload, bodyAdmission, string) {
	t.Helper()
	fixture := directMMCTestFixtureFor(name, 8)
	return decodeDirectMMCPayload(decodeEvent(fixture.format, fixture.content), fixture.content)
}
