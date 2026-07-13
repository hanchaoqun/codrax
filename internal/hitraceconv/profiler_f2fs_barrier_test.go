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

func TestStructuredF2FSBarrierSealsAcrossMessagesSpillAndOtherFamilies(t *testing.T) {
	cases := profilerAuxCasesByField()
	start := profilerF2FSTestStructuredMessage(4011, cases[4011].values, 1_000)
	done := profilerF2FSTestStructuredMessage(4012, cases[4012].values, 4_000)
	badValues := profilerAuxCloneValues(cases[4012].values)
	badValues[2] = profilerAuxVarint(0)
	bad := profilerF2FSTestStructuredMessage(4012, badValues, 2_000)
	print := profilerF2FSTestStructuredMessage(1109, cases[1109].values, 3_000)
	mmcStart := profilerMMCTestStructuredMessage(4016, cases[4016].values, 5_000)
	mmcDone := profilerMMCTestStructuredMessage(4015, cases[4015].values, 6_000)

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, message := range [][]byte{start, bad, print, done, mmcStart, mmcDone} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if !sink.pairKindPoisoned(pairRenderF2FS) || sink.pairKindPoisoned(pairRenderMMC) ||
		sink.withheldPairRowsForKind(pairRenderF2FS) != 2 || sink.withheldStructuredPairRowsForKind(pairRenderF2FS) != 2 ||
		sink.publishableRows() != 3 || len(sink.chunks) == 0 {
		t.Fatalf("structured F2FS stage isolation drifted: accepted=%d f2fs_withheld=%d publishable=%d chunks=%d poisoned=%v",
			sink.stats.RowsAccepted, sink.withheldPairRowsForKind(pairRenderF2FS), sink.publishableRows(), len(sink.chunks), sink.poisoned)
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if stats.RowsAccepted != 5 || stats.RowsWritten != 3 || stats.RowsWithheld != 2 ||
		strings.Contains(text, "f2fs_write_") || !strings.Contains(text, "mmc_request_start:") ||
		!strings.Contains(text, "mmc_request_done:") || !strings.Contains(text, "print: B|7|Frame") {
		t.Fatalf("structured F2FS barrier leaked/suppressed wrong rows: stats=%+v\n%s", stats, text)
	}
}

func TestStructuredF2FSKnownNonKeyFailureQuarantinesOnlyExactLaneAcrossSpill(t *testing.T) {
	cases := profilerAuxCasesByField()
	laneAStart := profilerAuxCloneValues(cases[4011].values)
	laneADone := profilerAuxCloneValues(cases[4012].values)
	laneBStart := profilerAuxCloneValues(cases[4011].values)
	laneBDone := profilerAuxCloneValues(cases[4012].values)
	laneBStart[2] = profilerAuxVarint(0x5678)
	laneBDone[2] = profilerAuxVarint(0x5678)
	badLaneA := profilerAuxCloneValues(laneADone)
	badLaneA[5] = profilerAuxVarint(uint64(^uint32(0)) + 1)

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, message := range [][]byte{
		profilerF2FSTestStructuredMessage(4011, laneAStart, 1_000),
		profilerF2FSTestStructuredMessage(4011, laneBStart, 2_000),
		profilerF2FSTestStructuredMessage(4012, badLaneA, 3_000),
		profilerF2FSTestStructuredMessage(1109, cases[1109].values, 4_000),
		profilerF2FSTestStructuredMessage(4012, laneADone, 5_000),
		profilerF2FSTestStructuredMessage(4012, laneBDone, 6_000),
		profilerMMCTestStructuredMessage(4016, cases[4016].values, 7_000),
		profilerMMCTestStructuredMessage(4015, cases[4015].values, 8_000),
	} {
		if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, sink); renderErr != nil {
			t.Fatal(renderErr)
		}
	}
	if sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 1 ||
		sink.pairKindPoisoned(pairRenderMMC) || sink.withheldPairRowsForKind(pairRenderF2FS) != 2 ||
		sink.withheldStructuredPairRowsForKind(pairRenderF2FS) != 2 || sink.publishableRows() != 5 ||
		len(sink.chunks) == 0 {
		t.Fatalf("known F2FS non-key failure escaped exact-lane spill quarantine: accepted=%d withheld=%d publishable=%d family=%v lanes=%v chunks=%d",
			sink.stats.RowsAccepted, sink.withheldPairRowsForKind(pairRenderF2FS), sink.publishableRows(), sink.poisoned, sink.poisonedLanes, len(sink.chunks))
	}
	var out bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &out)
	if err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if stats.RowsAccepted != 7 || stats.RowsWritten != 5 || stats.RowsWithheld != 2 ||
		strings.Contains(text, "ino=0x1234") || !strings.Contains(text, "ino=0x5678") ||
		!strings.Contains(text, "mmc_request_start:") || !strings.Contains(text, "mmc_request_done:") ||
		!strings.Contains(text, "print: B|7|Frame") {
		t.Fatalf("exact-lane spill filtering suppressed or leaked rows: stats=%+v\n%s", stats, text)
	}
}

func TestStructuredF2FSFamilyProvenanceSurvivesAmbiguousAndMalformedEnvelopes(t *testing.T) {
	cases := profilerAuxCasesByField()
	payload := profilerAuxEncodeValues(cases[4012].values)
	ambiguousEvent := protoMessage(2,
		protoVarint(1, 1_000), protoVarint(2, 40), protoBytes(3, []byte("f2fs")),
		protoMessage(50, protoVarint(4, 40)), protoMessage(4012, payload),
		protoMessage(2003, protoPayload(protoVarint(1, 1_550_000), protoVarint(2, 2))),
	)
	result := decodeProfilerTracePluginResult(protoMessage(2, protoPayload(protoVarint(1, 2), ambiguousEvent)))
	events, err := profilerTracePluginResultEvents(result)
	if err != nil || len(events) != 1 || events[0].Field != 0 || events[0].PairFamilies&pairCriticalFormatFamilyF2FS == 0 {
		t.Fatalf("ambiguous exact oneof lost F2FS provenance: events=%+v err=%v", events, err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	if rows, _, renderErr := renderProfilerFtraceStructuredResult(result, &seq, sink); renderErr != nil || rows != 0 || !sink.pairKindPoisoned(pairRenderF2FS) {
		t.Fatalf("ambiguous F2FS oneof did not close family: rows=%d poisoned=%v err=%v", rows, sink.poisoned, renderErr)
	}

	validStart := profilerF2FSTestStructuredMessage(4011, cases[4011].values, 1_000)
	validDone := profilerF2FSTestStructuredMessage(4012, cases[4012].values, 3_000)
	eventRecord := protoPayload(protoVarint(1, 2_000), protoVarint(2, 40), protoBytes(3, []byte("f2fs")), protoMessage(50, protoVarint(4, 40)), protoMessage(4012, payload))
	cpuDetail := protoPayload(protoVarint(1, 2), protoMessage(2, eventRecord))
	malformed := [][]byte{
		append(append([]byte(nil), profilerF2FSTestStructuredMessage(4012, cases[4012].values, 2_000)...), 0x80),
		protoMessage(2, protoPayload(protoVarint(1, 2), []byte{0x0f}, protoMessage(2, eventRecord))),
		profilerMMCTruncatedMessage(2, cpuDetail),
	}
	for index, hole := range malformed {
		t.Run(string(rune('a'+index)), func(t *testing.T) {
			local, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer local.cleanup()
			seq := 0
			for _, message := range [][]byte{validStart, hole, validDone} {
				if _, _, renderErr := renderProfilerFtraceStructuredRows(message, &seq, local); renderErr != nil {
					t.Fatal(renderErr)
				}
			}
			if !local.pairKindPoisoned(pairRenderF2FS) || local.withheldPairRowsForKind(pairRenderF2FS) != 2 || local.publishableRows() != 0 {
				t.Fatalf("malformed envelope lost F2FS capture barrier: accepted=%d withheld=%d opaque=%v poisoned=%v", local.stats.RowsAccepted, local.withheldPairRows(), local.opaque, local.poisoned)
			}
		})
	}
}

func TestStructuredF2FSUnknownOwnerClosesFamilyInsteadOfGuessingLane(t *testing.T) {
	cases := profilerAuxCasesByField()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	result := profilerTracePluginResult{
		Disposition: profilerFtracePayloadStructured,
		CPUDetails: [][]byte{protoPayload(
			protoVarint(1, 2),
			protoMessage(2, protoPayload(
				protoVarint(1, 1_000), protoVarint(2, 40), protoBytes(3, []byte("f2fs")),
				protoMessage(50, protoPayload(protoVarint(4, 40), protoVarint(4, 40))),
				protoMessage(4012, profilerAuxEncodeValues(cases[4012].values)),
			)),
		)},
	}
	if rows, _, renderErr := renderProfilerFtraceStructuredResult(result, &seq, sink); renderErr != nil || rows != 0 {
		t.Fatalf("unknown-owner fixture verdict rows=%d err=%v", rows, renderErr)
	}
	if !sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 0 {
		t.Fatalf("unknown F2FS owner did not close family exactly: family=%v lanes=%v", sink.poisoned, sink.poisonedLanes)
	}
}

func TestStructuredF2FSExplicitUnknownOwnerCannotBeReconstructedFromPID(t *testing.T) {
	cases := profilerAuxCasesByField()
	event := profilerFtraceEventRecord{
		Field: 4012, PID: 40, HeaderOwnerKnown: false,
		Payload: profilerAuxEncodeValues(cases[4012].values),
		// Explicitly no EnvelopeDegradations: absence of diagnostics is not an
		// owner witness and must not revive the prior PID fallback.
	}
	_, admission, reason, pair := decodeProfilerAuxPayloadWithPairAdmission(event)
	if admission != bodyAdmitted || reason != "" || !pair.Governed || pair.Kind != pairRenderF2FS || pair.LaneKnown {
		t.Fatalf("explicit unknown owner gained F2FS lane authority: admission=%v reason=%q pair=%+v", admission, reason, pair)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	pair.poison(sink)
	if !sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 0 {
		t.Fatalf("unknown owner must close the F2FS family, not guess a lane: family=%v lanes=%v", sink.poisoned, sink.poisonedLanes)
	}
}

func TestProfilerContainerF2FSCaptureCoverageMatchesPublishedRows(t *testing.T) {
	cases := profilerAuxCasesByField()
	badValues := profilerAuxCloneValues(cases[4012].values)
	badValues[2] = profilerAuxVarint(0)
	messages := [][]byte{
		syntheticProfilerPluginData("other-plugin", []byte("other-7 (7) [001] .... 0.900000: print: B|7|Other\n")),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4011, cases[4011].values, 1_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4012, badValues, 2_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(1109, cases[1109].values, 3_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4012, cases[4012].values, 4_000)),
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "profiler-f2fs.htrace"), filepath.Join(dir, "profiler-f2fs.ftrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(messages...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || strings.Contains(string(body), "f2fs_write_") ||
		!strings.Contains(string(body), "print: B|7|Frame") || !strings.Contains(string(body), "print: B|7|Other") {
		t.Fatalf("profiler container F2FS barrier output mismatch: result=%+v\n%s", result, body)
	}
	pluginEmitted, otherEmitted, startEmitted, doneEmitted, barrierRows := 0, 0, -1, -1, -1
	for _, item := range result.TraceCoverage {
		if item.Family == "builtin_modern_profiler" && item.Table == "plugin:ftrace-plugin" {
			pluginEmitted += item.RowsEmitted
		}
		if item.Family == "builtin_modern_profiler" && item.Table == "plugin:__other_text__" {
			otherEmitted += item.RowsEmitted
		}
		switch item.Table {
		case "f2fs_write_begin":
			startEmitted = item.RowsEmitted
		case "f2fs_write_end":
			doneEmitted = item.RowsEmitted
		case "__complete_capture_barrier__":
			if item.Family == "builtin_modern_ftrace:f2fs" {
				barrierRows = item.RowsRead
			}
		}
	}
	joined := strings.Join(result.Caveats, "\n")
	if pluginEmitted != 1 || otherEmitted != 1 || startEmitted != 0 || doneEmitted != 0 || barrierRows != 2 || !strings.Contains(joined, "F2FS full-capture") {
		t.Fatalf("profiler F2FS coverage lied after seal: plugin=%d other=%d start=%d done=%d barrier=%d\n%s\n%+v",
			pluginEmitted, otherEmitted, startEmitted, doneEmitted, barrierRows, joined, result.TraceCoverage)
	}
}

func TestProfilerContainerF2FSExactLaneCoverageMatchesPublishedRows(t *testing.T) {
	cases := profilerAuxCasesByField()
	laneAStart := profilerAuxCloneValues(cases[4011].values)
	laneADone := profilerAuxCloneValues(cases[4012].values)
	laneBStart := profilerAuxCloneValues(cases[4011].values)
	laneBDone := profilerAuxCloneValues(cases[4012].values)
	laneBStart[2] = profilerAuxVarint(0x5678)
	laneBDone[2] = profilerAuxVarint(0x5678)
	badLaneA := profilerAuxCloneValues(laneADone)
	badLaneA[5] = profilerAuxVarint(uint64(^uint32(0)) + 1)
	messages := [][]byte{
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4011, laneAStart, 1_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4011, laneBStart, 2_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4012, badLaneA, 3_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4012, laneADone, 4_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(4012, laneBDone, 5_000)),
		syntheticProfilerPluginData("ftrace-plugin", profilerF2FSTestStructuredMessage(1109, cases[1109].values, 6_000)),
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "profiler-f2fs-lanes.htrace"), filepath.Join(dir, "profiler-f2fs-lanes.ftrace")
	if err := os.WriteFile(input, syntheticProfilerTraceFile(messages...), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 3 || strings.Contains(string(body), "ino=0x1234") ||
		strings.Count(string(body), "ino=0x5678") != 2 || !strings.Contains(string(body), "print: B|7|Frame") {
		t.Fatalf("profiler exact-lane output mismatch: result=%+v\n%s", result, body)
	}
	pluginEmitted, startEmitted, doneEmitted, barrierRows := 0, 0, 0, -1
	for _, item := range result.TraceCoverage {
		if item.Family == "builtin_modern_profiler" && item.Table == "plugin:ftrace-plugin" {
			pluginEmitted += item.RowsEmitted
		}
		switch item.Table {
		case "f2fs_write_begin":
			startEmitted += item.RowsEmitted
		case "f2fs_write_end":
			doneEmitted += item.RowsEmitted
		case "__complete_capture_barrier__":
			if item.Family == "builtin_modern_ftrace:f2fs" {
				barrierRows = item.RowsRead
			}
		}
	}
	if pluginEmitted != 3 || startEmitted != 1 || doneEmitted != 1 || barrierRows != 2 {
		t.Fatalf("exact-lane coverage attribution drifted: plugin=%d start=%d done=%d barrier=%d coverage=%+v",
			pluginEmitted, startEmitted, doneEmitted, barrierRows, result.TraceCoverage)
	}
}

func TestProfilerTextAndSessionF2FSBarriersShareSourceSeal(t *testing.T) {
	startFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	doneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	startPayload, _, _ := decodeDirectF2FSPayload(decodeEvent(startFixture.format, startFixture.content))
	donePayload, _, _ := decodeDirectF2FSPayload(decodeEvent(doneFixture.format, doneFixture.content))
	startBody, _ := renderCanonicalF2FSPayload(startPayload)
	doneBody, _ := renderCanonicalF2FSPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: f2fs_write_begin: " + startBody
	badLine := "io-100 (100) [002] .... 1.001000: f2fs_write_end: " + doneBody + " extra=1"
	doneLine := "io-100 (100) [002] .... 1.002000: f2fs_write_end: " + doneBody

	for _, lane := range []struct {
		name string
		add  func(string, *int, *traceDBRowSink) error
	}{
		{name: "strict", add: func(line string, seq *int, sink *traceDBRowSink) error {
			_, _, err := addStrictSystraceRowsFromBytes([]byte(line+"\n"), seq, sink)
			return err
		}},
		{name: "generic", add: func(line string, seq *int, sink *traceDBRowSink) error {
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
			if !sink.pairKindPoisoned(pairRenderF2FS) || sink.withheldPairRowsForKind(pairRenderF2FS) != 3 || sink.publishableRows() != 0 {
				t.Fatalf("text publisher bypassed F2FS source barrier: accepted=%d withheld=%d poisoned=%v", sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned)
			}
			if sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 1 {
				t.Fatalf("known text hard key escalated beyond its exact F2FS lane: family=%v lanes=%v", sink.poisoned, sink.poisonedLanes)
			}
		})
	}

	dir := t.TempDir()
	input, output := filepath.Join(dir, "session.htrace"), filepath.Join(dir, "session.ftrace")
	payload := strings.Join([]string{"SessionJSON-", startLine, "BROKEN [003] .... 1.001000: f2fs_write_end: " + doneBody, doneLine, "other-7 (7) [001] .... 1.003000: print: B|7|Keep", ""}, "\n")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 1 || strings.Contains(string(body), "f2fs_write_") || !strings.Contains(string(body), "print: B|7|Keep") {
		t.Fatalf("session F2FS barrier leaked or suppressed sibling: result=%+v\n%s", result, body)
	}
	index, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range tracequery.ComputeWindowStats(index, tracequery.Query{}).StorageLatencyByLayer {
		if row.Layer == "f2fs" && row.PairedCount > 0 {
			t.Fatalf("session hole rescued an F2FS duration: %+v", row)
		}
	}
}

func TestProfilerGenericAndSessionMalformedTimestampCannotBridgeF2FS(t *testing.T) {
	startFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	doneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	startPayload, _, _ := decodeDirectF2FSPayload(decodeEvent(startFixture.format, startFixture.content))
	donePayload, _, _ := decodeDirectF2FSPayload(decodeEvent(doneFixture.format, doneFixture.content))
	startBody, _ := renderCanonicalF2FSPayload(startPayload)
	doneBody, _ := renderCanonicalF2FSPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: f2fs_write_begin: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: f2fs_write_end: " + doneBody

	for _, timestamp := range []string{"NaN", "1.2.3", "1e3"} {
		t.Run("generic_"+timestamp, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			bad := "io-100 (100) [002] .... " + timestamp + ": f2fs_write_end: " + doneBody
			for _, line := range []string{startLine, bad, doneLine, "other-7 (7) [001] .... 1.003000: print: B|7|Keep"} {
				if _, err := addSystraceRowsFromBytes([]byte(line+"\n"), &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 1 ||
				sink.withheldPairRowsForKind(pairRenderF2FS) != 2 || sink.publishableRows() != 1 {
				t.Fatalf("malformed %q F2FS endpoint became an invisible bridge: accepted=%d withheld=%d family=%v lanes=%v",
					timestamp, sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned, sink.poisonedLanes)
			}
		})
	}

	dir := t.TempDir()
	input, output := filepath.Join(dir, "session-malformed-ts.htrace"), filepath.Join(dir, "session-malformed-ts.ftrace")
	payload := strings.Join([]string{
		"SessionJSON-", startLine,
		"io-100 (100) [002] .... NaN: f2fs_write_end: " + doneBody,
		doneLine, "other-7 (7) [001] .... 1.003000: print: B|7|Keep", "",
	}, "\n")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 1 || strings.Contains(string(body), "f2fs_write_") || !strings.Contains(string(body), "print: B|7|Keep") {
		t.Fatalf("session malformed timestamp bridged F2FS endpoints: result=%+v\n%s", result, body)
	}
}

func TestProfilerTextPairHeaderPreservesBracketAndNumericSuffixComm(t *testing.T) {
	startFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	doneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	startPayload, _, _ := decodeDirectF2FSPayload(decodeEvent(startFixture.format, startFixture.content))
	donePayload, _, _ := decodeDirectF2FSPayload(decodeEvent(doneFixture.format, doneFixture.content))
	startBody, _ := renderCanonicalF2FSPayload(startPayload)
	doneBody, _ := renderCanonicalF2FSPayload(donePayload)
	line := func(comm, ts, event, body string) string {
		return comm + "-100 (100) [002] .... " + ts + ": " + event + ": " + body
	}

	for _, comm := range []string{"foo]bar", "x-1 [2] . 3: e:"} {
		t.Run("valid_header_like_comm_"+strings.ReplaceAll(comm, " ", "_"), func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			for _, row := range []string{
				line(comm, "1.000000", "f2fs_write_begin", startBody),
				line(comm, "1.002000", "f2fs_write_end", doneBody),
			} {
				if _, err := addSystraceRowsFromBytes([]byte(row+"\n"), &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if sink.pairKindPoisoned(pairRenderF2FS) || sink.pairRows[pairRenderF2FS] != 2 || sink.publishableRows() != 2 {
				t.Fatalf("legal header-like comm lost profiler pair: publishable=%d poisoned=%v lanes=%v",
					sink.publishableRows(), sink.poisoned, sink.poisonedLanes)
			}
		})
	}

	t.Run("malformed_timestamp_numeric_suffix_comm", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 1)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		seq := 0
		for _, row := range []string{
			line("worker-pool-12", "1.000000", "f2fs_write_begin", startBody),
			line("worker-pool-12", "NaN", "f2fs_write_end", doneBody),
			line("worker-pool-12", "1.002000", "f2fs_write_end", doneBody),
			"other-7 (7) [001] .... 1.003000: print: B|7|Keep",
		} {
			if _, err := addSystraceRowsFromBytes([]byte(row+"\n"), &seq, sink); err != nil {
				t.Fatal(err)
			}
		}
		if sink.poisoned[pairRenderF2FS] || len(sink.poisonedLanes[pairRenderF2FS]) != 1 ||
			sink.withheldPairRowsForKind(pairRenderF2FS) != 2 || sink.publishableRows() != 1 {
			t.Fatalf("numeric-suffix comm widened malformed endpoint quarantine: publishable=%d family=%v lanes=%v",
				sink.publishableRows(), sink.poisoned, sink.poisonedLanes)
		}
	})

	dir := t.TempDir()
	input, output := filepath.Join(dir, "session-bracket-comm.htrace"), filepath.Join(dir, "session-bracket-comm.ftrace")
	payload := strings.Join([]string{
		"SessionJSON-",
		line("foo]bar", "1.000000", "f2fs_write_begin", startBody),
		line("foo]bar", "1.002000", "f2fs_write_end", doneBody), "",
	}, "\n")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || strings.Count(string(body), "f2fs_write_") != 2 {
		t.Fatalf("SessionJSON legal bracket comm lost F2FS pair: result=%+v\n%s", result, body)
	}
}

func TestProfilerGenericRawMalformedF2FSScopeUsesProvenHardKeyOnly(t *testing.T) {
	startFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	doneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	startPayload, _, _ := decodeDirectF2FSPayload(decodeEvent(startFixture.format, startFixture.content))
	donePayload, _, _ := decodeDirectF2FSPayload(decodeEvent(doneFixture.format, doneFixture.content))
	startBody, _ := renderCanonicalF2FSPayload(startPayload)
	doneBody, _ := renderCanonicalF2FSPayload(donePayload)
	unknownKeyFields := strings.Fields(doneBody)
	for index := range unknownKeyFields {
		if strings.HasPrefix(unknownKeyFields[index], "ino=") {
			unknownKeyFields[index] = "ino=0x0"
		}
	}
	unknownKeyBody := strings.Join(unknownKeyFields, " ")
	startLine := "io-100 (100) [002] .... 1.000000: f2fs_write_begin: " + startBody
	doneLine := "io-100 (100) [002] .... 1.002000: f2fs_write_end: " + doneBody
	for _, test := range []struct {
		name       string
		bad        string
		wantFamily bool
		wantRows   int
	}{
		{
			name: "known key malformed CPU stays lane local",
			bad:  "io-100 (100) [bad] .... 1.001000: f2fs_write_end: " + doneBody,
			// systraceLineHeader can recover the timestamp/name, so this rejected
			// physical row is staged and then withheld with its two siblings.
			wantRows: 3,
		},
		{
			name:       "unknown key malformed timestamp closes family",
			bad:        "io-100 (100) [002] .... NaN: f2fs_write_end: " + unknownKeyBody,
			wantFamily: true,
			wantRows:   2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			for _, line := range []string{startLine, test.bad, doneLine} {
				if _, err := addSystraceRowsFromBytes([]byte(line+"\n"), &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if sink.poisoned[pairRenderF2FS] != test.wantFamily ||
				(!test.wantFamily && len(sink.poisonedLanes[pairRenderF2FS]) != 1) ||
				sink.withheldPairRowsForKind(pairRenderF2FS) != test.wantRows || sink.publishableRows() != 0 {
				t.Fatalf("raw malformed F2FS scope drifted: accepted=%d withheld=%d family=%v lanes=%v",
					sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned, sink.poisonedLanes)
			}
		})
	}
}

func TestProfilerGenericMalformedProbeDoesNotPoisonProsePrintOrNearNames(t *testing.T) {
	startFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	doneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	startPayload, _, _ := decodeDirectF2FSPayload(decodeEvent(startFixture.format, startFixture.content))
	donePayload, _, _ := decodeDirectF2FSPayload(decodeEvent(doneFixture.format, doneFixture.content))
	startBody, _ := renderCanonicalF2FSPayload(startPayload)
	doneBody, _ := renderCanonicalF2FSPayload(donePayload)
	lines := []string{
		"customer prose: NaN: f2fs_write_end: " + doneBody,
		"io-100 (100) [002] .... 0.999000: print: NaN: f2fs_write_end: " + doneBody,
		"io-100 (100) [002] .... NaN: f2fs_write_exit: " + doneBody,
		"io-100 (100) [002] .... NaN: F2FS_write_end: " + doneBody,
		"io-100 (100) [002] .... 1.000000: f2fs_write_begin: " + startBody,
		"io-100 (100) [002] .... 1.002000: f2fs_write_end: " + doneBody,
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, line := range lines {
		if _, err := addSystraceRowsFromBytes([]byte(line+"\n"), &seq, sink); err != nil {
			t.Fatal(err)
		}
	}
	if sink.pairKindPoisoned(pairRenderF2FS) || sink.withheldPairRowsForKind(pairRenderF2FS) != 0 {
		t.Fatalf("prose/print/near-name poisoned exact F2FS scope: accepted=%d family=%v lanes=%v", sink.stats.RowsAccepted, sink.poisoned, sink.poisonedLanes)
	}
}

func TestProfilerNestedFullHeadersInsidePrintProseDoNotPoisonPairFamilies(t *testing.T) {
	f2fsStartFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	f2fsDoneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	f2fsStart, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsStartFixture.format, f2fsStartFixture.content))
	f2fsDone, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsDoneFixture.format, f2fsDoneFixture.content))
	f2fsStartBody, _ := renderCanonicalF2FSPayload(f2fsStart)
	f2fsDoneBody, _ := renderCanonicalF2FSPayload(f2fsDone)
	mmcStart, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	mmcDone, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	mmcStartBody, _ := renderCanonicalMMCPayload(mmcStart)
	mmcDoneBody, _ := renderCanonicalMMCPayload(mmcDone)

	line := func(ts, name, body string) string {
		return "io-100 (100) [002] .... " + ts + ": " + name + ": " + body
	}
	for _, test := range []struct {
		name   string
		kind   pairRenderKind
		start  string
		done   string
		nested string
	}{
		{
			name: "f2fs", kind: pairRenderF2FS,
			start: line("1.000000", "f2fs_write_begin", f2fsStartBody),
			done:  line("1.002000", "f2fs_write_end", f2fsDoneBody),
			nested: "outer-7 (7) [001] .... 1.001000: print: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: f2fs_write_end: " + f2fsDoneBody,
		},
		{
			name: "mmc", kind: pairRenderMMC,
			start: line("1.000000", "mmc_request_start", mmcStartBody),
			done:  line("1.002000", "mmc_request_done", mmcDoneBody),
			nested: "outer-7 (7) [001] .... 1.001000: print: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: mmc_request_done: " + mmcDoneBody,
		},
	} {
		t.Run(test.name+"_generic", func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			for _, row := range []string{test.start, test.nested, test.done} {
				if _, err := addSystraceRowsFromBytes([]byte(row+"\n"), &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if sink.pairKindPoisoned(test.kind) || sink.withheldPairRowsForKind(test.kind) != 0 ||
				sink.publishableRows() != 3 {
				t.Fatalf("nested print prose poisoned %s: accepted=%d withheld=%d family=%v lanes=%v",
					test.name, sink.stats.RowsAccepted, sink.withheldPairRows(), sink.poisoned, sink.poisonedLanes)
			}
		})
	}

	// Exercise the SessionJSON production reader and top-level conversion, not
	// only the shared line helper. The quoted inner endpoint must remain print
	// prose while the real outer F2FS pair stays publishable.
	dir := t.TempDir()
	input := filepath.Join(dir, "session-nested-print.htrace")
	output := filepath.Join(dir, "session-nested-print.ftrace")
	f2fsStartLine := line("1.000000", "f2fs_write_begin", f2fsStartBody)
	f2fsDoneLine := line("1.002000", "f2fs_write_end", f2fsDoneBody)
	nestedPrint := "outer-7 (7) [001] .... 1.001000: print: customer prose embeds " +
		"inner-100 (100) [002] .... 2.000000: f2fs_write_end: " + f2fsDoneBody
	payload := strings.Join([]string{"SessionJSON-", f2fsStartLine, nestedPrint, f2fsDoneLine, ""}, "\n")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	joinedCaveats := strings.Join(result.Caveats, "\n")
	if result.EventsWritten != 3 || strings.Count(string(body), "f2fs_write_") != 3 ||
		!strings.Contains(string(body), ": print: customer prose embeds") ||
		strings.Contains(joinedCaveats, "profiler F2FS full-capture anti-rescue barrier failed") {
		t.Fatalf("SessionJSON nested print prose poisoned the real F2FS pair: result=%+v\n%s", result, body)
	}
}

func TestProfilerMalformedOuterPrintCannotPromoteOrPoisonNestedPairEndpoint(t *testing.T) {
	f2fsStartFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	f2fsDoneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	f2fsStart, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsStartFixture.format, f2fsStartFixture.content))
	f2fsDone, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsDoneFixture.format, f2fsDoneFixture.content))
	f2fsStartBody, _ := renderCanonicalF2FSPayload(f2fsStart)
	f2fsDoneBody, _ := renderCanonicalF2FSPayload(f2fsDone)
	mmcStart, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	mmcDone, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	mmcStartBody, _ := renderCanonicalMMCPayload(mmcStart)
	mmcDoneBody, _ := renderCanonicalMMCPayload(mmcDone)

	line := func(ts, name, body string) string {
		return "io-100 (100) [002] .... " + ts + ": " + name + ": " + body
	}
	for _, test := range []struct {
		name, start, malformedOuter, done string
		kind                              pairRenderKind
		wantPublished                     int
	}{
		{
			name: "f2fs", kind: pairRenderF2FS, wantPublished: 2,
			start: line("1.000000", "f2fs_write_begin", f2fsStartBody),
			malformedOuter: "outer-7 (7) [001] .... NaN: print: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: f2fs_write_end: " + f2fsDoneBody,
			done: line("1.002000", "f2fs_write_end", f2fsDoneBody),
		},
		{
			name: "mmc", kind: pairRenderMMC, wantPublished: 3,
			start: line("1.000000", "mmc_request_start", mmcStartBody),
			malformedOuter: "outer-7 (7) [bad] .... 1.001000: print: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: mmc_request_done: " + mmcDoneBody,
			done: line("1.002000", "mmc_request_done", mmcDoneBody),
		},
		{
			name: "f2fs_missing_right_bracket", kind: pairRenderF2FS, wantPublished: 2,
			start: line("1.000000", "f2fs_write_begin", f2fsStartBody),
			malformedOuter: "outer-7 (7) [bad .... NaN: print: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: f2fs_write_end: " + f2fsDoneBody,
			done: line("1.002000", "f2fs_write_end", f2fsDoneBody),
		},
		{
			name: "f2fs_vendor_slash", kind: pairRenderF2FS, wantPublished: 3,
			start: line("1.000000", "f2fs_write_begin", f2fsStartBody),
			malformedOuter: "outer-7 (7) [bad] .... 1.001000: vendor/foo: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: f2fs_write_end: " + f2fsDoneBody,
			done: line("1.002000", "f2fs_write_end", f2fsDoneBody),
		},
		{
			name: "mmc_print_event", kind: pairRenderMMC, wantPublished: 2,
			start: line("1.000000", "mmc_request_start", mmcStartBody),
			malformedOuter: "outer-7 (7) [bad] .... NaN: print-event: customer prose embeds " +
				"inner-100 (100) [002] .... 2.000000: mmc_request_done: " + mmcDoneBody,
			done: line("1.002000", "mmc_request_done", mmcDoneBody),
		},
	} {
		t.Run(test.name+"_generic", func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 0
			for _, row := range []string{test.start, test.malformedOuter, test.done} {
				if _, err := addSystraceRowsFromBytes([]byte(row+"\n"), &seq, sink); err != nil {
					t.Fatal(err)
				}
			}
			if sink.pairKindPoisoned(test.kind) || sink.withheldPairRowsForKind(test.kind) != 0 ||
				sink.pairRows[test.kind] != 2 || sink.publishableRows() != test.wantPublished {
				t.Fatalf("malformed outer print promoted or poisoned nested %s endpoint: accepted=%d publishable=%d withheld=%d poisoned=%v lanes=%v",
					test.name, sink.stats.RowsAccepted, sink.publishableRows(), sink.withheldPairRows(), sink.poisoned, sink.poisonedLanes)
			}
		})
	}

	dir := t.TempDir()
	input := filepath.Join(dir, "session-malformed-outer-print.htrace")
	output := filepath.Join(dir, "session-malformed-outer-print.ftrace")
	payload := strings.Join([]string{
		"SessionJSON-",
		line("1.000000", "f2fs_write_begin", f2fsStartBody),
		"outer-7 (7) [001] .... NaN: print: customer prose embeds " +
			"inner-100 (100) [002] .... 2.000000: f2fs_write_end: " + f2fsDoneBody,
		line("1.002000", "f2fs_write_end", f2fsDoneBody),
		"",
	}, "\n")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || strings.Count(string(body), "f2fs_write_") != 2 ||
		strings.Contains(strings.Join(result.Caveats, "\n"), "profiler F2FS full-capture anti-rescue barrier failed") {
		t.Fatalf("SessionJSON malformed outer print promoted or poisoned nested endpoint: result=%+v\n%s", result, body)
	}
}

func TestProfilerTimestampEventProseCannotMintPairAuthority(t *testing.T) {
	f2fsStartFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	f2fsDoneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	f2fsStart, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsStartFixture.format, f2fsStartFixture.content))
	f2fsDone, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsDoneFixture.format, f2fsDoneFixture.content))
	f2fsStartBody, _ := renderCanonicalF2FSPayload(f2fsStart)
	f2fsDoneBody, _ := renderCanonicalF2FSPayload(f2fsDone)

	line := func(ts, name, body string) string {
		return "io-100 (100) [002] .... " + ts + ": " + name + ": " + body
	}
	start := line("1.000000", "f2fs_write_begin", f2fsStartBody)
	done := line("1.002000", "f2fs_write_end", f2fsDoneBody)
	prose := "customer 1.001000: f2fs_write_end: " + f2fsDoneBody

	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	for _, row := range []string{start, prose, done} {
		if _, err := addSystraceRowsFromBytes([]byte(row+"\n"), &seq, sink); err != nil {
			t.Fatal(err)
		}
	}
	if sink.pairKindPoisoned(pairRenderF2FS) || sink.pairRows[pairRenderF2FS] != 2 || sink.publishableRows() != 2 {
		t.Fatalf("timestamp:event prose minted profiler pair authority: accepted=%d publishable=%d poisoned=%v lanes=%v",
			sink.stats.RowsAccepted, sink.publishableRows(), sink.poisoned, sink.poisonedLanes)
	}

	dir := t.TempDir()
	input, output := filepath.Join(dir, "session-prose.htrace"), filepath.Join(dir, "session-prose.ftrace")
	payload := strings.Join([]string{"SessionJSON-", start, prose, done, ""}, "\n")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 2 || strings.Count(string(body), "f2fs_write_") != 2 ||
		strings.Contains(strings.Join(result.Caveats, "\n"), "profiler F2FS full-capture anti-rescue barrier failed") {
		t.Fatalf("SessionJSON timestamp:event prose suppressed or minted F2FS evidence: result=%+v\n%s", result, body)
	}
}

func TestProfilerMixedTextMessageUsesIndependentF2FSAndMMCCaptureScopes(t *testing.T) {
	f2fsStartFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	f2fsDoneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	f2fsStart, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsStartFixture.format, f2fsStartFixture.content))
	f2fsDone, _, _ := decodeDirectF2FSPayload(decodeEvent(f2fsDoneFixture.format, f2fsDoneFixture.content))
	f2fsStartBody, _ := renderCanonicalF2FSPayload(f2fsStart)
	f2fsDoneBody, _ := renderCanonicalF2FSPayload(f2fsDone)
	badF2FSDoneBody := f2fsDoneBody[:strings.LastIndex(f2fsDoneBody, "copied=")] + "copied=4294967296"
	mmcStart, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	mmcDone, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	mmcStartBody, _ := renderCanonicalMMCPayload(mmcStart)
	mmcDoneBody, _ := renderCanonicalMMCPayload(mmcDone)
	line := func(ts int64, name, body string) string {
		return traceDBFormatLine("io", 100, 100, 2, ts, 0, 0, name+": "+body)
	}
	f2fsStartLine := line(1_000_000_000, "f2fs_write_begin", f2fsStartBody)
	f2fsBadLine := line(1_001_000_000, "f2fs_write_end", badF2FSDoneBody)
	f2fsDoneLine := line(1_002_000_000, "f2fs_write_end", f2fsDoneBody)
	mmcStartLine := line(1_003_000_000, "mmc_request_start", mmcStartBody)
	mmcBadLine := line(1_004_000_000, "mmc_request_done", mmcDoneBody+" extra=1")
	mmcDoneLine := line(1_005_000_000, "mmc_request_done", mmcDoneBody)
	printLine := line(1_006_000_000, "print", "B|100|Keep")

	for _, test := range []struct {
		name          string
		lines         []string
		wantRows      int
		wantF2FS      bool
		wantMMC       bool
		wantTextCount string
	}{
		{
			name:     "known F2FS lane poison keeps MMC and print",
			lines:    []string{f2fsStartLine, f2fsBadLine, f2fsDoneLine, mmcStartLine, mmcDoneLine, printLine},
			wantRows: 3, wantMMC: true, wantTextCount: "extracted 3 systrace text row(s) from 1 profiler plugin message(s)",
		},
		{
			name:     "both family verdicts subtract once",
			lines:    []string{f2fsStartLine, f2fsBadLine, f2fsDoneLine, mmcStartLine, mmcBadLine, mmcDoneLine, printLine},
			wantRows: 1, wantTextCount: "extracted 1 systrace text row(s) from 1 profiler plugin message(s)",
		},
		{
			name:     "MMC poison keeps clean F2FS and print",
			lines:    []string{f2fsStartLine, f2fsDoneLine, mmcStartLine, mmcBadLine, mmcDoneLine, printLine},
			wantRows: 3, wantF2FS: true, wantTextCount: "extracted 3 systrace text row(s) from 1 profiler plugin message(s)",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "mixed.htrace"), filepath.Join(dir, "mixed.ftrace")
			message := syntheticProfilerPluginData("other-plugin", []byte(strings.Join(test.lines, "\n")+"\n"))
			if err := os.WriteFile(input, syntheticProfilerTraceFile(message), 0o644); err != nil {
				t.Fatal(err)
			}
			result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			if result.EventsWritten != test.wantRows || strings.Contains(text, "copied=4294967296") ||
				(strings.Contains(text, "f2fs_write_") != test.wantF2FS) ||
				(strings.Contains(text, "mmc_request_") != test.wantMMC) || !strings.Contains(text, "print: B|100|Keep") ||
				!strings.Contains(strings.Join(result.Caveats, "\n"), test.wantTextCount) {
				t.Fatalf("mixed profiler scope/accounting drifted: result=%+v\n%s", result, text)
			}
			pluginRows := -1
			for _, item := range result.TraceCoverage {
				if item.Family == "builtin_modern_profiler" && item.Table == "plugin:__other_text__" {
					pluginRows = item.RowsEmitted
				}
			}
			if pluginRows != test.wantRows {
				t.Fatalf("mixed publisher coverage=%d want=%d: %+v", pluginRows, test.wantRows, result.TraceCoverage)
			}
		})
	}
}

func profilerF2FSTestStructuredMessage(field int, values map[int]profilerAuxTestValue, ts uint64) []byte {
	detail := protoPayload(protoVarint(1, 2), syntheticTracePluginFtraceEvent(ts, 40, 40, "f2fs", field, profilerAuxEncodeValues(values)))
	return protoMessage(2, detail)
}
