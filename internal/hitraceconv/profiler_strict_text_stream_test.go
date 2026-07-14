package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestProfilerStrictTextTwoPassProductionTopology(t *testing.T) {
	stageType := reflect.TypeOf(profilerStrictSystracePayloadStage{})
	if stageType.NumField() != 2 || stageType.Field(0).Name != "scan" ||
		stageType.Field(0).Type != reflect.TypeOf(profilerStrictSystracePayloadScan{}) ||
		stageType.Field(1).Name != "data" || stageType.Field(1).Type.Kind() != reflect.Slice ||
		stageType.Field(1).Type.Elem().Kind() != reflect.Uint8 {
		t.Fatalf("strict text stage is not exactly fixed scan plus input byte view: %v", stageType)
	}
	scanType := reflect.TypeOf(profilerStrictSystracePayloadScan{})
	if scanType.NumField() != 6 {
		t.Fatalf("strict text scan field count=%d, want fixed six: %v", scanType.NumField(), scanType)
	}
	for index := 0; index < scanType.NumField(); index++ {
		field := scanType.Field(index)
		wantKind := reflect.Bool
		if index < 2 {
			wantKind = reflect.Array
		} else if index == 2 {
			wantKind = reflect.Int
		}
		if field.Type.Kind() != wantKind {
			t.Fatalf("strict text scan regained retained field %s kind=%s want=%s: %v",
				field.Name, field.Type.Kind(), wantKind, scanType)
		}
	}

	authority := mustReadRendererSource(t, "profiler_ftrace_authority.go")
	stage := sourceBetween(t, authority,
		"type profilerStrictSystracePayloadStage struct {",
		"func addProfilerStrictSystraceStageContext(")
	for _, want := range []string{
		"scan profilerStrictSystracePayloadScan",
		"data []byte",
		"scanProfilerStrictSystracePayloadContext(ctx, data, nil)",
	} {
		if !strings.Contains(stage, want) {
			t.Fatalf("strict text first pass lost fixed-width/data-view authority %q:\n%s", want, stage)
		}
	}
	for _, forbidden := range []string{"[]renderedRow", "[]profilerPairAdmission", "append("} {
		if strings.Contains(stage, forbidden) {
			t.Fatalf("strict text stage regained payload-proportional retention %q:\n%s", forbidden, stage)
		}
	}

	add := sourceBetween(t, authority,
		"func addProfilerStrictSystraceStageContext(",
		"type profilerStrictSystracePayloadScan struct {")
	if strings.Count(add, "scanProfilerStrictSystracePayloadContext(ctx, stage.data,") != 1 ||
		strings.Count(add, "validateProfilerRowSequenceRange(seq, stage.scan.rows)") != 1 ||
		strings.Count(add, "sink.addSequencedProfilerEventContext(ctx, seq, row, delta)") != 1 ||
		strings.Contains(add, "sink.add(row)") || strings.Contains(add, "(*seq)++") {
		t.Fatalf("strict text second pass is not the unique Context row-sink publisher:\n%s", add)
	}
	sequenceRangeAt := strings.Index(add, "validateProfilerRowSequenceRange(seq, stage.scan.rows)")
	fixedPrePoisonCommentAt := strings.Index(add, "// This fixed-width pre-poison")
	fixedPrePoisonLoopAt := -1
	if fixedPrePoisonCommentAt >= 0 {
		if relative := strings.Index(add[fixedPrePoisonCommentAt:], "for _, kind := range profilerCaptureKinds {"); relative >= 0 {
			fixedPrePoisonLoopAt = fixedPrePoisonCommentAt + relative
		}
	}
	if sequenceRangeAt < 0 || fixedPrePoisonLoopAt < 0 || sequenceRangeAt >= fixedPrePoisonLoopAt {
		t.Fatalf("strict sequence capacity gate no longer dominates whole-kind pre-poison: range=%d pre_poison=%d\n%s",
			sequenceRangeAt, fixedPrePoisonLoopAt, add)
	}

	scan := sourceBetween(t, authority,
		"func scanProfilerStrictSystracePayloadContext(",
		"// profilerTextCommentPrefix")
	visitor := sourceBetween(t, authority,
		"type profilerStrictSystracePayloadVisitor",
		"type profilerStrictSystracePayloadScan struct {")
	if !strings.Contains(visitor,
		"type profilerStrictSystracePayloadVisitor func(renderedRow, profilerPairAdmission) error") ||
		!strings.Contains(scan, "visit profilerStrictSystracePayloadVisitor") ||
		strings.Count(scan, "visit(row, pair)") != 1 {
		t.Fatalf("strict text scanner visitor cannot propagate second-pass sink failure:\n%s\n%s", visitor, scan)
	}

	container := mustReadRendererSource(t, "profiler_container.go")
	extract := sourceBetween(t, container,
		"func extractProfilerTraceFileAtWithFrameLimit(",
		"func appendProfilerZeroFrameCensus(")
	if strings.Count(extract,
		"addProfilerStrictSystraceStageContext(ctx, strictStage, &seq, sink)") != 1 ||
		strings.Contains(extract, "addProfilerStrictSystraceStage(strictStage, &seq, sink)") {
		t.Fatalf("production exact-ftrace route bypassed the Context two-pass publisher:\n%s", extract)
	}
}

func TestProfilerStrictTextContainerDiagnosticsAndPairCensus(t *testing.T) {
	start, bad, done := profilerStrictTextF2FSFixture(t)
	print := traceDBFormatLine("worker", 7, 7, 1, 1_003_000_000, 0, 0, "print: B|7|Frame")
	payload := []byte(strings.Join([]string{start, bad, done, print, ""}, "\n"))
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData("ftrace-plugin", payload))
	dir := t.TempDir()
	input := filepath.Join(dir, "strict-container.htrace")
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	header, ok, err := readProfilerTraceHeaderAtPath(input, 0, info.Size())
	if err != nil || !ok {
		t.Fatalf("read strict container fixture header: ok=%t err=%v", ok, err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerTraceFileWithFrameLimit(
		context.Background(), input, info.Size(), header, sink, maxProfilerPluginFrameBytes)
	if err != nil {
		t.Fatal(err)
	}
	if !extracted.Detected || extracted.Messages != 1 || extracted.TextPluginMessages != 1 ||
		extracted.TextRows != 1 || extracted.StructuredFtrace != 0 || extracted.MalformedFtrace != 0 ||
		extracted.UnsupportedFtrace != 0 || extracted.RejectedMessages != 0 || extracted.SourceFailClosed ||
		len(extracted.pairPublishers) != 1 || len(extracted.textMessages) != 1 {
		t.Fatalf("strict container extraction ledger drifted: %+v", extracted)
	}
	publisher := extracted.pairPublishers[0]
	message := extracted.textMessages[0]
	if publisher.coverageIndex < 0 || publisher.coverageIndex >= len(extracted.TraceCoverage) ||
		publisher.staged[pairRenderF2FS].total != 3 || message.total != 4 ||
		message.staged[pairRenderF2FS].total != 3 || sink.pairCensusActive ||
		sink.stats.RowsAccepted != 4 || sink.withheldPairRowsForKind(pairRenderF2FS) != 3 ||
		sink.publishableRows() != 1 {
		t.Fatalf("strict container pair/message census drifted: publisher=%+v message=%+v sink=%+v",
			publisher, message, sink.stats)
	}
	coverage := extracted.TraceCoverage[publisher.coverageIndex]
	if coverage.Table != "plugin:ftrace-plugin" || coverage.RowsRead != 1 || coverage.RowsEmitted != 4 ||
		coverage.FieldSources["outcome_strict_legacy_text_frames"] != "1" {
		t.Fatalf("strict container coverage drifted: %+v", coverage)
	}
}

func profilerStrictTextMMCFixture(t *testing.T) (string, string, string) {
	t.Helper()
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, startOK := renderCanonicalMMCPayload(startPayload)
	doneBody, doneOK := renderCanonicalMMCPayload(donePayload)
	if !startOK || !doneOK {
		t.Fatal("canonical MMC fixture did not render")
	}
	return "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody,
		"io-100 (100) [002] .... 1.001000: mmc_request_done: dev=mmcblk0 op=read",
		"io-100 (100) [002] .... 1.002000: mmc_request_done: " + doneBody
}

func profilerStrictTextF2FSFixture(t *testing.T) (string, string, string) {
	t.Helper()
	startFixture := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	doneFixture := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)
	startPayload, _, _ := decodeDirectF2FSPayload(decodeEvent(startFixture.format, startFixture.content))
	donePayload, _, _ := decodeDirectF2FSPayload(decodeEvent(doneFixture.format, doneFixture.content))
	startBody, startOK := renderCanonicalF2FSPayload(startPayload)
	doneBody, doneOK := renderCanonicalF2FSPayload(donePayload)
	if !startOK || !doneOK {
		t.Fatal("canonical F2FS fixture did not render")
	}
	return "io-100 (100) [002] .... 1.000000: f2fs_write_begin: " + startBody,
		"io-100 (100) [002] .... 1.001000: f2fs_write_end: " + doneBody + " extra=1",
		"io-100 (100) [002] .... 1.002000: f2fs_write_end: " + doneBody
}

func profilerStrictTextBlockFixture() (string, string, string) {
	return traceDBFormatLine("worker", 40, 40, 2, 1_000_000_000, 0, 0,
			"block_rq_issue: 0,1 R 4 () 2 + 3 []"),
		traceDBFormatLine("worker", 41, 41, 2, 1_001_000_000, 0, 0,
			"block_rq_issue: 0,1 R 4294967296 () 2 + 3 []"),
		traceDBFormatLine("worker", 42, 42, 2, 1_002_000_000, 0, 0,
			"block_rq_complete: 0,1 R () 2 + 3 [0]")
}

func TestProfilerStrictTextRejectsBeforePublishingButRetainsEndpointCensus(t *testing.T) {
	_, _, mmcDone := profilerStrictTextMMCFixture(t)
	printRow := "worker-7 (7) [001] .... 0.999000: print: B|7|Frame"

	for _, test := range []struct {
		name  string
		lines []string
	}{
		{name: "bad tail", lines: []string{printRow, mmcDone, "not-a-trace"}},
		{name: "bad prefix still scans backend endpoint", lines: []string{printRow, "not-a-trace", mmcDone}},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := []byte(strings.Join(test.lines, "\n") + "\n")
			scan := scanProfilerStrictSystracePayload(payload, nil)
			if !scan.rejected || !scan.originText || !scan.observed[pairRenderMMC] {
				t.Fatalf("whole-payload census lost rejected/backend MMC provenance: %+v", scan)
			}

			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 19
			rows, classified, err := addStrictSystraceRowsFromBytes(payload, &seq, sink)
			if err != nil {
				t.Fatalf("reject strict text payload: %v", err)
			}
			if rows != 0 || classified || seq != 19 || sink.stats.RowsAccepted != 0 ||
				len(sink.rows) != 0 || len(sink.runs) != 0 || len(sink.rowIngestOrdinals) != 0 ||
				sink.bufferedBytes != 0 {
				t.Fatalf("rejected first pass published row state: rows=%d classified=%t seq=%d stats=%+v buffered_rows=%d runs=%d ordinals=%d bytes=%d",
					rows, classified, seq, sink.stats, len(sink.rows), len(sink.runs),
					len(sink.rowIngestOrdinals), sink.bufferedBytes)
			}
			if !sink.poisoned[pairRenderMMC] || !sink.opaque[pairRenderMMC] {
				t.Fatalf("rejected source lost its precise MMC family/opacity barrier: poisoned=%v opaque=%v",
					sink.poisoned, sink.opaque)
			}
		})
	}

	anonymous := []byte("anonymous metadata\n" + mmcDone + "\n")
	scan := scanProfilerStrictSystracePayload(anonymous, nil)
	if !scan.rejected || scan.originText || !scan.observed[pairRenderMMC] {
		t.Fatalf("later endpoint overwrote first anonymous origin decision: %+v", scan)
	}
}

func TestProfilerStrictTextRejectedPairOrderingParity(t *testing.T) {
	f2fsStart, f2fsBad, f2fsDone := profilerStrictTextF2FSFixture(t)
	blockStart, blockBad, blockDone := profilerStrictTextBlockFixture()
	mmcStart, mmcBad, mmcDone := profilerStrictTextMMCFixture(t)

	fixtures := []struct {
		name          string
		kind          pairRenderKind
		start         string
		bad           string
		done          string
		wantLaneLocal bool
	}{
		{name: "f2fs known lane", kind: pairRenderF2FS, start: f2fsStart, bad: f2fsBad, done: f2fsDone, wantLaneLocal: true},
		{name: "block known lane", kind: pairRenderBlock, start: blockStart, bad: blockBad, done: blockDone, wantLaneLocal: true},
		{name: "mmc unknown lane", kind: pairRenderMMC, start: mmcStart, bad: mmcBad, done: mmcDone},
	}
	orders := []struct {
		name string
		make func(string, string, string) []string
	}{
		{name: "bad before", make: func(start, bad, done string) []string { return []string{bad, start, done} }},
		{name: "bad middle", make: func(start, bad, done string) []string { return []string{start, bad, done} }},
		{name: "bad after", make: func(start, bad, done string) []string { return []string{start, done, bad} }},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			badPair := profilerTextPairAdmission(fixture.bad)
			if !badPair.Governed || badPair.Kind != fixture.kind || badPair.Admitted ||
				badPair.LaneKnown != fixture.wantLaneLocal {
				t.Fatalf("fixture does not exercise intended rejected pair scope: %+v", badPair)
			}
			for _, order := range orders {
				order := order
				t.Run(order.name, func(t *testing.T) {
					sink, err := newTraceDBRowSink(t.TempDir(), 1)
					if err != nil {
						t.Fatal(err)
					}
					defer sink.cleanup()
					seq := 7
					payload := []byte(strings.Join(order.make(fixture.start, fixture.bad, fixture.done), "\n") + "\n")
					rows, classified, err := addStrictSystraceRowsFromBytes(payload, &seq, sink)
					if err != nil {
						t.Fatal(err)
					}
					if rows != 3 || !classified || seq != 10 || sink.stats.RowsAccepted != 3 ||
						sink.withheldPairRowsForKind(fixture.kind) != 3 || sink.publishableRows() != 0 {
						t.Fatalf("rejected pair ordering changed row ledger: rows=%d classified=%t seq=%d stats=%+v withheld=%d publishable=%d",
							rows, classified, seq, sink.stats,
							sink.withheldPairRowsForKind(fixture.kind), sink.publishableRows())
					}
					if fixture.wantLaneLocal {
						if sink.poisoned[fixture.kind] || len(profilerTestPoisonedLanes(sink)[fixture.kind]) != 1 {
							t.Fatalf("known-lane rejection widened to family: family=%v lanes=%v",
								sink.poisoned, profilerTestPoisonedLanes(sink))
						}
					} else if !sink.poisoned[fixture.kind] || len(profilerTestPoisonedLanes(sink)[fixture.kind]) != 0 {
						t.Fatalf("unknown-lane rejection did not close family: family=%v lanes=%v",
							sink.poisoned, profilerTestPoisonedLanes(sink))
					}
				})
			}
		})
	}
}

type profilerStrictTextSpillLane struct {
	rows          int
	seq           int
	output        []byte
	ingest        traceDBRowSortStats
	final         traceDBRowSortStats
	pairRows      map[pairRenderKind]int
	poisoned      map[pairRenderKind]bool
	poisonedLanes map[pairRenderKind]map[string]bool
	withheld      int
}

func runProfilerStrictTextSpillLane(t *testing.T, payload []byte, threshold int,
	options traceDBRowSinkOptions,
) profilerStrictTextSpillLane {
	t.Helper()
	dir := t.TempDir()
	sink, err := newTraceDBRowSinkWithOptions(dir, threshold, options)
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = sink.cleanup()
		}
	}()

	seq := 31
	rows, classified, err := addStrictSystraceRowsFromBytes(payload, &seq, sink)
	if err != nil || !classified {
		t.Fatalf("ingest strict text spill fixture: rows=%d classified=%t err=%v", rows, classified, err)
	}
	ingest := sink.stats
	withheld := sink.withheldPairRowsForKind(pairRenderF2FS)
	var output bytes.Buffer
	final, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatalf("publish strict text spill fixture: %v", err)
	}
	lane := profilerStrictTextSpillLane{
		rows: rows, seq: seq, output: append([]byte(nil), output.Bytes()...), ingest: ingest, final: final,
		pairRows: clonePairRenderIntMap(sink.pairRows), poisoned: clonePairRenderBoolMap(sink.poisoned),
		poisonedLanes: clonePairRenderLaneBoolMap(profilerTestPoisonedLanes(sink)), withheld: withheld,
	}
	if err := sink.cleanup(); err != nil {
		t.Fatalf("cleanup strict text spill fixture: %v", err)
	}
	cleaned = true
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("inspect strict text spill temp directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("strict text spill cleanup retained artifacts: %v", entries)
	}
	return lane
}

func clonePairRenderIntMap(source map[pairRenderKind]int) map[pairRenderKind]int {
	out := make(map[pairRenderKind]int, len(source))
	for kind, value := range source {
		out[kind] = value
	}
	return out
}

func clonePairRenderBoolMap(source map[pairRenderKind]bool) map[pairRenderKind]bool {
	out := make(map[pairRenderKind]bool, len(source))
	for kind, value := range source {
		out[kind] = value
	}
	return out
}

func clonePairRenderLaneBoolMap(source map[pairRenderKind]map[string]bool) map[pairRenderKind]map[string]bool {
	out := make(map[pairRenderKind]map[string]bool, len(source))
	for kind, lanes := range source {
		cloned := make(map[string]bool, len(lanes))
		for lane, value := range lanes {
			cloned[lane] = value
		}
		out[kind] = cloned
	}
	return out
}

func TestProfilerStrictTextHighTinySpillParityAndCleanup(t *testing.T) {
	start, bad, done := profilerStrictTextF2FSFixture(t)
	const printRows = 512
	lines := make([]string, 0, printRows+3)
	lines = append(lines, start)
	for index := 0; index < printRows/2; index++ {
		ts := int64(3_000_000_000 + printRows - index)
		lines = append(lines, traceDBFormatLine("worker", 7, 7, 1, ts, 0, 0,
			"print: I|7|before"))
	}
	// The tiny lane has already externalized the start row when this exact-lane
	// rejection arrives. Publication must still withhold the persisted prefix.
	lines = append(lines, bad)
	for index := printRows / 2; index < printRows; index++ {
		ts := int64(3_000_000_000 + printRows - index)
		lines = append(lines, traceDBFormatLine("worker", 7, 7, 1, ts, 0, 0,
			"print: I|7|after"))
	}
	lines = append(lines, done)
	payload := []byte(strings.Join(lines, "\n") + "\n")

	high := runProfilerStrictTextSpillLane(t, payload, len(lines)+1,
		traceDBRowSinkOptions{bufferBytes: 8 << 20})
	tiny := runProfilerStrictTextSpillLane(t, payload, 4,
		traceDBRowSinkOptions{bufferBytes: 2 << 10})

	if high.rows != len(lines) || high.seq != 31+len(lines) || high.withheld != 3 ||
		high.final.RowsAccepted != len(lines) || high.final.RowsWritten != printRows ||
		high.final.RowsWithheld != 3 {
		t.Fatalf("strict text high-threshold reference ledger drifted: %+v", high)
	}
	if tiny.rows != high.rows || tiny.seq != high.seq || tiny.withheld != high.withheld ||
		!bytes.Equal(tiny.output, high.output) || !reflect.DeepEqual(tiny.pairRows, high.pairRows) ||
		!reflect.DeepEqual(tiny.poisoned, high.poisoned) ||
		!reflect.DeepEqual(tiny.poisonedLanes, high.poisonedLanes) ||
		tiny.final.RowsAccepted != high.final.RowsAccepted ||
		tiny.final.RowsWritten != high.final.RowsWritten ||
		tiny.final.RowsWithheld != high.final.RowsWithheld {
		t.Fatalf("tiny spill changed strict text output/sequence/pair final state:\nhigh=%+v\ntiny=%+v",
			high, tiny)
	}
	if high.ingest.SpillChunks != 0 {
		t.Fatalf("high-threshold reference spilled during ingestion: %+v", high.ingest)
	}
	if tiny.ingest.SpillChunks == 0 || tiny.ingest.PeakBufferedRows > 4 ||
		tiny.ingest.PeakBufferedBytes > 2<<10 {
		t.Fatalf("tiny strict text lane escaped configured spill bounds: %+v", tiny.ingest)
	}
	for name, lane := range map[string]profilerStrictTextSpillLane{"high": high, "tiny": tiny} {
		if lane.final.CurrentLiveTempBytes != 0 {
			t.Fatalf("%s strict text publication retained live temp bytes: %+v", name, lane.final)
		}
	}
}

type profilerStrictTextCompletedPrefixContext struct {
	context.Context
	sink       *traceDBRowSink
	cancelRows int
	err        error
	polls      int
}

type profilerStrictTextConvertCancellationContext struct {
	context.Context
	err         error
	sawAdd      bool
	addPolls    int
	cancelPolls int
}

func (ctx *profilerStrictTextConvertCancellationContext) Err() error {
	var callers [64]uintptr
	frames := runtime.CallersFrames(callers[:runtime.Callers(2, callers[:])])
	inAdd := false
	inScan := false
	for {
		frame, more := frames.Next()
		if strings.HasSuffix(frame.Function, ".addContext") {
			inAdd = true
		}
		if strings.HasSuffix(frame.Function, ".scanProfilerStrictSystracePayloadContext") {
			inScan = true
		}
		if !more {
			break
		}
	}
	if inAdd {
		ctx.sawAdd = true
		ctx.addPolls++
	} else if inScan && ctx.sawAdd && ctx.err != nil {
		ctx.cancelPolls++
		return ctx.err
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

func (ctx *profilerStrictTextCompletedPrefixContext) Err() error {
	ctx.polls++
	if ctx.err != nil && ctx.sink != nil && ctx.sink.stats.RowsAccepted >= ctx.cancelRows {
		return ctx.err
	}
	if ctx.Context != nil {
		return ctx.Context.Err()
	}
	return nil
}

func profilerStrictTextOrdinaryPayload(rows int) []byte {
	lines := make([]string, 0, rows)
	for index := 0; index < rows; index++ {
		lines = append(lines, traceDBFormatLine("worker", 7, 7, 1,
			int64(1_000_000_000+index), 0, 0, "print: I|7|strict-stream"))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func TestProfilerStrictTextSecondPassCancellationKeepsCompletedPrefixAtomic(t *testing.T) {
	payload := profilerStrictTextOrdinaryPayload(3)
	stage, err := stageProfilerStrictSystracePayloadContext(context.Background(), payload)
	if err != nil || stage.scan.rejected || !stage.scan.originText || stage.scan.rows != 3 {
		t.Fatalf("stage strict cancellation fixture: scan=%+v err=%v", stage.scan, err)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			ctx := &profilerStrictTextCompletedPrefixContext{
				Context: context.Background(), sink: sink, cancelRows: 1, err: want,
			}
			seq := 41
			rows, classified, err := addProfilerStrictSystraceStageContext(ctx, stage, &seq, sink)
			if err != want || !classified || rows != 1 || seq != 42 || ctx.polls == 0 {
				t.Fatalf("second-pass cancellation result drifted: rows=%d classified=%t seq=%d polls=%d err=%T %v want=%v",
					rows, classified, seq, ctx.polls, err, err, want)
			}
			if sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 || len(sink.runs) != 0 ||
				sink.rows[0].seq != 41 || sink.nextIngestOrdinal != 1 || len(sink.pairRows) != 0 {
				t.Fatalf("second-pass cancellation split completed/current row state: rows=%+v stats=%+v runs=%d next=%d pair=%v",
					sink.rows, sink.stats, len(sink.runs), sink.nextIngestOrdinal, sink.pairRows)
			}
		})
	}
}

func TestProfilerStrictTextConvertSecondPassCancellationLeavesNoCustomerArtifact(t *testing.T) {
	body := syntheticProfilerTraceFile(syntheticProfilerPluginData(
		"ftrace-plugin", profilerStrictTextOrdinaryPayload(3)))
	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		t.Run(want.Error(), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "strict-second-pass.htrace")
			output := filepath.Join(dir, "strict-second-pass.systrace")
			if err := os.WriteFile(input, body, 0o600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &profilerStrictTextConvertCancellationContext{
				Context: context.Background(), err: want,
			}
			result, err := ConvertFile(ctx, Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
			})
			if err != want || !reflect.DeepEqual(result, Result{}) || !ctx.sawAdd ||
				ctx.addPolls == 0 || ctx.cancelPolls != 1 {
				t.Fatalf("strict second-pass conversion cancellation drifted: result=%+v saw_add=%t add_polls=%d cancel_polls=%d err=%T %v want=%v",
					result, ctx.sawAdd, ctx.addPolls, ctx.cancelPolls, err, err, want)
			}
			if _, err := os.Lstat(output); !os.IsNotExist(err) {
				t.Fatalf("strict second-pass cancellation left output: %v", err)
			}
			after, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			got, readErr := os.ReadFile(input)
			if !os.SameFile(before, after) || before.Mode() != after.Mode() ||
				before.Size() != after.Size() || readErr != nil || !bytes.Equal(got, body) {
				t.Fatalf("strict second-pass cancellation changed protected input: same=%t mode=%v/%v size=%d/%d bytes_equal=%t read_err=%v",
					os.SameFile(before, after), before.Mode(), after.Mode(), before.Size(), after.Size(),
					bytes.Equal(got, body), readErr)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(input) {
				t.Fatalf("strict second-pass cancellation left customer artifacts: entries=%v err=%v", entries, err)
			}
		})
	}
}

func TestProfilerStrictTextCurrentRowCancellationLeavesNoRowOrSequenceDelta(t *testing.T) {
	_, rejectedLane, _ := profilerStrictTextF2FSFixture(t)
	payload := []byte(rejectedLane + "\n")
	stage, err := stageProfilerStrictSystracePayloadContext(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	calibrationSink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	calibration := &profilerSummaryMetadataCancelContext{
		Context: context.Background(), targetSuffix: ".addContext",
	}
	calibrationSeq := 0
	if _, _, err := addProfilerStrictSystraceStageContext(
		calibration, stage, &calibrationSeq, calibrationSink); err != nil {
		t.Fatal(err)
	}
	calibrationPolls := calibration.polls
	if err := calibrationSink.cleanup(); err != nil {
		t.Fatal(err)
	}
	if calibrationPolls < 2 {
		t.Fatalf("strict current-row transaction exposed only %d polls", calibrationPolls)
	}

	for _, want := range []error{context.Canceled, context.DeadlineExceeded} {
		for cancelAt := 1; cancelAt <= calibrationPolls; cancelAt++ {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			seq := 17
			ctx := &profilerSummaryMetadataCancelContext{
				Context: context.Background(), targetSuffix: ".addContext", cancelAt: cancelAt, err: want,
			}
			rows, classified, err := addProfilerStrictSystraceStageContext(ctx, stage, &seq, sink)
			if err != want || !classified || rows != 0 || seq != 17 || ctx.polls != cancelAt {
				sink.cleanup()
				t.Fatalf("cancel=%d/%d strict current-row result drifted: polls=%d rows=%d classified=%t seq=%d err=%T %v",
					cancelAt, calibrationPolls, ctx.polls, rows, classified, seq, err, err)
			}
			if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.runs) != 0 ||
				sink.nextIngestOrdinal != 0 || len(sink.pairRows) != 0 || len(sink.poisoned) != 0 ||
				len(profilerTestPoisonedLanes(sink)) != 0 || sink.legacyPairProof.observations != 0 ||
				sink.legacyPairProof.laneKeys != 0 || sink.pairCensusActive ||
				sink.activePairPublisher != profilerPairPublisherNone || sink.textMessageActive ||
				sink.activeTextMessage != 0 || sink.activeTextRows != 0 || sink.nextTextMessage != 0 ||
				len(sink.pairLaneRegistries[pairRenderF2FS].byKey) != 0 ||
				len(sink.pairLaneRegistries[pairRenderF2FS].keys) != 0 ||
				len(sink.pairLaneRegistries[pairRenderF2FS].states) != 0 {
				sink.cleanup()
				t.Fatalf("cancel=%d/%d strict current row partially committed: stats=%+v rows=%d runs=%d next=%d pair=%v poisoned=%v lanes=%v proof=%+v",
					cancelAt, calibrationPolls, sink.stats, len(sink.rows), len(sink.runs),
					sink.nextIngestOrdinal, sink.pairRows, sink.poisoned, profilerTestPoisonedLanes(sink), sink.legacyPairProof)
			}
			if err := sink.cleanup(); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestProfilerStrictTextPairPoisonScopeUsesOneAuthority(t *testing.T) {
	for _, test := range []struct {
		name      string
		admission profilerPairAdmission
		whole     bool
	}{
		{name: "known lane", admission: profilerPairAdmission{
			Kind: pairRenderF2FS, Governed: true, LaneKnown: true, Lane: "ino=7",
		}},
		{name: "empty claimed lane", admission: profilerPairAdmission{
			Kind: pairRenderF2FS, Governed: true, LaneKnown: true,
		}, whole: true},
		{name: "unknown lane", admission: profilerPairAdmission{
			Kind: pairRenderF2FS, Governed: true,
		}, whole: true},
		{name: "mmc always whole", admission: profilerPairAdmission{
			Kind: pairRenderMMC, Governed: true, LaneKnown: true, Lane: "tag=7",
		}, whole: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.admission.poisonsWholeKind(); got != test.whole {
				t.Fatalf("pair poison scope=%t want=%t admission=%+v", got, test.whole, test.admission)
			}
			var delta traceDBProfilerEventDelta
			delta.poisonAdmission(test.admission)
			if delta.poisonKinds[test.admission.Kind] != test.whole {
				t.Fatalf("transaction delta forked pair poison authority: delta=%+v admission=%+v", delta, test.admission)
			}
			if !test.whole && delta.poisonLanes[test.admission.Kind] != test.admission.Lane {
				t.Fatalf("transaction delta lost exact lane: delta=%+v admission=%+v", delta, test.admission)
			}
		})
	}
}

func TestProfilerStrictTextWholeKindPrePoisonIsNoneOrAll(t *testing.T) {
	_, mmcBad, _ := profilerStrictTextMMCFixture(t)
	blockBad := traceDBFormatLine("worker", 41, 41, 2, 1_001_000_000, 0, 0,
		"block_rq_issue: malformed")
	for _, line := range []string{mmcBad, blockBad} {
		admission := profilerTextPairAdmission(line)
		if !admission.Governed || admission.Admitted || !admission.poisonsWholeKind() {
			t.Fatalf("whole-kind fixture is not an exact rejected endpoint: line=%q admission=%+v", line, admission)
		}
	}
	stage, err := stageProfilerStrictSystracePayloadContext(
		context.Background(), []byte(mmcBad+"\n"+blockBad+"\n"))
	if err != nil || !stage.scan.wholeKindPoison[pairRenderMMC] ||
		!stage.scan.wholeKindPoison[pairRenderBlock] {
		t.Fatalf("strict first pass lost fixed whole-kind bits: scan=%+v err=%v", stage.scan, err)
	}

	for _, test := range []struct {
		name       string
		cancelAt   int
		wantPoison bool
	}{
		{name: "before fixed apply", cancelAt: 2},
		{name: "after fixed apply", cancelAt: 3, wantPoison: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			ctx := &profilerSummaryMetadataCancelContext{
				Context: context.Background(), targetSuffix: ".addProfilerStrictSystraceStageContext",
				cancelAt: test.cancelAt, err: context.Canceled,
			}
			seq := 9
			rows, classified, err := addProfilerStrictSystraceStageContext(ctx, stage, &seq, sink)
			if err != context.Canceled || rows != 0 || !classified || seq != 9 ||
				ctx.polls != test.cancelAt {
				t.Fatalf("whole-kind cancel boundary drifted: rows=%d classified=%t seq=%d polls=%d err=%T %v",
					rows, classified, seq, ctx.polls, err, err)
			}
			mmc := sink.poisoned[pairRenderMMC]
			block := sink.poisoned[pairRenderBlock]
			if mmc != test.wantPoison || block != test.wantPoison || mmc != block {
				t.Fatalf("fixed whole-kind mutation was partial: mmc=%t block=%t want=%t poisoned=%v",
					mmc, block, test.wantPoison, sink.poisoned)
			}
		})
	}
}

func TestProfilerStrictTextKnownLanePairProofBudgetBoundary(t *testing.T) {
	start, bad, done := profilerStrictTextF2FSFixture(t)
	payload := []byte(start + "\n" + bad + "\n" + done + "\n")
	for _, test := range []struct {
		name       string
		maxObserve int64
		wantFamily bool
	}{
		{name: "exact budget", maxObserve: 4},
		{name: "one short", maxObserve: 3, wantFamily: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			sink.legacyPairProof.maxObservations = test.maxObserve
			sink.legacyPairProof.maxLaneKeys = 1
			if !sink.beginPairRowCensus() {
				t.Fatal("begin strict pair census")
			}
			seq := 0
			rows, classified, err := addStrictSystraceRowsFromBytes(payload, &seq, sink)
			census := sink.endPairRowCensus()
			if err != nil || !classified || rows != 3 || seq != 3 ||
				census[pairRenderF2FS].total != 3 || sink.withheldPairRowsForKind(pairRenderF2FS) != 3 {
				t.Fatalf("pair proof boundary row/census drifted: rows=%d classified=%t seq=%d census=%+v withheld=%d err=%v",
					rows, classified, seq, census[pairRenderF2FS],
					sink.withheldPairRowsForKind(pairRenderF2FS), err)
			}
			if sink.poisoned[pairRenderF2FS] != test.wantFamily {
				t.Fatalf("pair proof boundary family=%t want=%t lanes=%v proof=%+v",
					sink.poisoned[pairRenderF2FS], test.wantFamily,
					profilerTestPoisonedLanes(sink)[pairRenderF2FS], sink.legacyPairProof)
			}
			if test.wantFamily {
				if sink.legacyPairProof.failureReason != "observations" ||
					census[pairRenderF2FS].byLane != nil {
					t.Fatalf("short proof budget did not fail-close fixed state: proof=%+v census=%+v",
						sink.legacyPairProof, census[pairRenderF2FS])
				}
			} else if sink.legacyPairProof.failureReason != "" ||
				len(profilerTestPoisonedLanes(sink)[pairRenderF2FS]) != 1 ||
				len(census[pairRenderF2FS].byLane) != 1 {
				t.Fatalf("exact proof budget widened known-lane rejection: proof=%+v lanes=%v census=%+v",
					sink.legacyPairProof, profilerTestPoisonedLanes(sink)[pairRenderF2FS], census[pairRenderF2FS])
			}
		})
	}
}

func TestProfilerStrictTextTailSpillFailurePreservesConcreteErrorAndCompletedRow(t *testing.T) {
	for _, point := range []string{"create", "write", "stat"} {
		t.Run(point, func(t *testing.T) {
			want := errors.New("strict-text-" + point + "-sentinel")
			dir := t.TempDir()
			sink, err := newTraceDBRowSinkWithOptions(dir, 1, traceDBRowSinkOptions{
				bufferBytes: 1 << 20,
				ops: traceDBRowSinkOps{fault: func(got, _ string) error {
					if got == point {
						return want
					}
					return nil
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			stage, err := stageProfilerStrictSystracePayloadContext(
				context.Background(), profilerStrictTextOrdinaryPayload(1))
			if err != nil {
				t.Fatal(err)
			}
			seq := 23
			rows, classified, err := addProfilerStrictSystraceStageContext(
				context.Background(), stage, &seq, sink)
			if !errors.Is(err, want) || !classified || rows != 1 || seq != 24 {
				t.Fatalf("strict tail spill failure identity/result drifted: rows=%d classified=%t seq=%d err=%T %v",
					rows, classified, seq, err, err)
			}
			if sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 || len(sink.runs) != 0 ||
				sink.nextIngestOrdinal != 1 || sink.stats.SpillChunks != 0 ||
				sink.activeTempBytes != 0 || sink.liveTempBytes != 0 {
				t.Fatalf("strict tail spill failure split completed row state: rows=%+v stats=%+v runs=%d next=%d active=%d live=%d",
					sink.rows, sink.stats, len(sink.runs), sink.nextIngestOrdinal,
					sink.activeTempBytes, sink.liveTempBytes)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("strict tail spill failure retained artifact: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestProfilerStrictTextPrefixSpillFailureCannotCommitCurrentPairRow(t *testing.T) {
	_, rejectedLane, _ := profilerStrictTextF2FSFixture(t)
	prefix := string(profilerStrictTextOrdinaryPayload(1))
	payload := []byte(prefix + rejectedLane + "\n")
	want := errors.New("strict-text-prefix-write-sentinel")
	dir := t.TempDir()
	sink, err := newTraceDBRowSinkWithOptions(dir, 1, traceDBRowSinkOptions{
		bufferBytes: 1 << 20,
		ops: traceDBRowSinkOps{fault: func(point, _ string) error {
			if point == "write" {
				return want
			}
			return nil
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	stage, err := stageProfilerStrictSystracePayloadContext(context.Background(), payload)
	if err != nil || stage.scan.rows != 2 {
		t.Fatalf("stage prefix-spill fixture: scan=%+v err=%v", stage.scan, err)
	}
	seq := 70
	rows, classified, err := addProfilerStrictSystraceStageContext(
		context.Background(), stage, &seq, sink)
	if !errors.Is(err, want) || !classified || rows != 1 || seq != 71 {
		t.Fatalf("prefix spill failure identity/result drifted: rows=%d classified=%t seq=%d err=%T %v",
			rows, classified, seq, err, err)
	}
	if sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 || sink.rows[0].seq != 70 ||
		len(sink.runs) != 0 || sink.nextIngestOrdinal != 1 || sink.stats.SpillChunks != 0 ||
		len(sink.pairRows) != 0 || len(sink.poisoned) != 0 || len(profilerTestPoisonedLanes(sink)) != 0 ||
		sink.legacyPairProof.observations != 0 || sink.activeTempBytes != 0 || sink.liveTempBytes != 0 {
		t.Fatalf("prefix spill failure committed current pair delta: rows=%+v stats=%+v runs=%d next=%d pair=%v poisoned=%v lanes=%v proof=%+v active=%d live=%d",
			sink.rows, sink.stats, len(sink.runs), sink.nextIngestOrdinal, sink.pairRows,
			sink.poisoned, profilerTestPoisonedLanes(sink), sink.legacyPairProof,
			sink.activeTempBytes, sink.liveTempBytes)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("prefix spill failure retained artifact: entries=%v err=%v", entries, readErr)
	}
}
