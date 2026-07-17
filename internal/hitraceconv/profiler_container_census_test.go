package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type profilerCensusReaderAt struct {
	source      io.ReaderAt
	reads       int
	failAt      int
	failErr     error
	cancelAfter int
	cancel      context.CancelFunc
}

func (reader *profilerCensusReaderAt) ReadAt(dst []byte, off int64) (int, error) {
	reader.reads++
	if reader.failAt > 0 && reader.reads == reader.failAt {
		return 0, reader.failErr
	}
	n, err := reader.source.ReadAt(dst, off)
	if reader.cancel != nil && reader.cancelAfter > 0 && reader.reads == reader.cancelAfter {
		reader.cancel()
		reader.cancel = nil
	}
	return n, err
}

func profilerZeroFrameFixture(count int, siblings ...profilerResourceFrame) []byte {
	payloadBytes := count * 4
	for _, sibling := range siblings {
		payloadBytes += 4 + len(sibling.payload)
	}
	body := make([]byte, profilerTraceHeaderSize+payloadBytes)
	binary.LittleEndian.PutUint64(body[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	binary.LittleEndian.PutUint32(body[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(body[20:24], uint32((count+len(siblings))*2))
	binary.LittleEndian.PutUint32(body[56:60], profilerDataTypeProtobuf)
	offset := profilerTraceHeaderSize + count*4
	for _, sibling := range siblings {
		binary.LittleEndian.PutUint32(body[offset:offset+4], sibling.declared)
		offset += 4
		copy(body[offset:offset+len(sibling.payload)], sibling.payload)
		offset += len(sibling.payload)
	}
	digest := sha256.Sum256(body[profilerTraceHeaderSize:])
	copy(body[24:56], digest[:])
	return body
}

func extractProfilerCensusFixture(t testing.TB, body []byte) (profilerContainerExtraction, *traceDBRowSink) {
	t.Helper()
	header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
	if !ok {
		t.Fatal("read profiler census fixture header")
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	rootProof := profilerRootProofForTest(
		t, bytes.NewReader(body), int64(len(body)), header, maxProfilerPluginFrameBytes)
	extracted, err := extractProfilerTraceFileAtWithFrameLimit(
		context.Background(), bytes.NewReader(body), int64(len(body)), header, rootProof, sink, maxProfilerPluginFrameBytes)
	if err != nil {
		sink.cleanup()
		t.Fatal(err)
	}
	return extracted, sink
}

func profilerZeroFrameCoverage(extracted profilerContainerExtraction) (TraceDBCoverage, int) {
	var found TraceDBCoverage
	count := 0
	for _, item := range extracted.TraceCoverage {
		if item.Table == "plugin:__rejected__" && strings.Contains(item.Skipped, "plugin_frame_zero_length=") {
			found = item
			count++
		}
	}
	return found, count
}

func TestProfilerZeroFrameCensusRetainedStateIsConstant(t *testing.T) {
	for _, frameCount := range []int{1, 4_096, 1_000_000} {
		t.Run(strconv.Itoa(frameCount), func(t *testing.T) {
			body := profilerZeroFrameFixture(frameCount)
			extracted, sink := extractProfilerCensusFixture(t, body)
			defer sink.cleanup()
			coverage, coverageEntries := profilerZeroFrameCoverage(extracted)
			if extracted.Messages != frameCount || extracted.RejectedMessages != frameCount ||
				extracted.SourceFailClosed || sink.stats.RowsAccepted != 0 || coverageEntries != 1 ||
				coverage.RowsRead != frameCount || coverage.RowsEmitted != 0 ||
				coverage.FieldSources["observed_total"] != strconv.Itoa(frameCount) ||
				coverage.FieldSources["aggregation_policy"] != "exact_count_with_first_last_offset" {
				t.Fatalf("zero-frame census drifted for count=%d: extracted=%+v coverage=%+v entries=%d",
					frameCount, extracted, coverage, coverageEntries)
			}
			if len(extracted.TraceCoverage) != 1 || len(extracted.Caveats) != 2 ||
				profilerRootWriterProfile(extracted) != profilerRootWriterProfileSequential {
				t.Fatalf("zero-frame retained diagnostics grew for count=%d: caveats=%d coverage=%d",
					frameCount, len(extracted.Caveats), len(extracted.TraceCoverage))
			}
			first := int64(profilerTraceHeaderSize)
			last := first + int64(frameCount-1)*4
			if coverage.FieldSources["first_offset"] != strconv.FormatInt(first, 10) ||
				coverage.FieldSources["last_offset"] != strconv.FormatInt(last, 10) {
				t.Fatalf("zero-frame first/last offsets drifted for count=%d: %+v", frameCount, coverage.FieldSources)
			}
		})
	}
}

func TestProfilerZeroFramesPreserveValidSiblingsAndOrdering(t *testing.T) {
	first := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 2.000000: print: B|7|Late"))
	second := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 1.000000: print: B|7|Early"))
	frames := make([]profilerResourceFrame, 0, 4_098)
	frames = append(frames, profilerResourceFrame{declared: uint32(len(first)), payload: first})
	frames = append(frames, make([]profilerResourceFrame, 4_096)...)
	frames = append(frames, profilerResourceFrame{declared: uint32(len(second)), payload: second})
	body := profilerResourceTraceFile(frames...)
	extracted, sink := extractProfilerCensusFixture(t, body)
	defer sink.cleanup()
	coverage, entries := profilerZeroFrameCoverage(extracted)
	if extracted.Messages != 4_098 || extracted.RejectedMessages != 4_096 || extracted.TextRows != 2 ||
		entries != 1 || coverage.RowsRead != 4_096 || sink.stats.RowsAccepted != 2 {
		t.Fatalf("zero-frame siblings drifted: extracted=%+v coverage=%+v sink=%+v", extracted, coverage, sink.stats)
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	earlyIndex := strings.Index(output.String(), "B|7|Early")
	lateIndex := strings.Index(output.String(), "B|7|Late")
	if stats.RowsWritten != 2 || stats.FirstTSNS != 1_000_000_000 || stats.LastTSNS != 2_000_000_000 ||
		earlyIndex < 0 || lateIndex < 0 || earlyIndex >= lateIndex {
		t.Fatalf("zero-frame sibling ordering drifted: stats=%+v\n%s", stats, output.String())
	}
}

func TestProfilerZeroFrameCensusRequiresVerifiedSequentialSegments(t *testing.T) {
	body := profilerZeroFrameFixture(4_096)
	for _, segments := range []uint32{8_192, 0, 4_096, math.MaxUint32} {
		fixture := append([]byte(nil), body...)
		binary.LittleEndian.PutUint32(fixture[20:24], segments)
		extracted, sink := extractProfilerCensusFixture(t, fixture)
		coverage, entries := profilerZeroFrameCoverage(extracted)
		defer sink.cleanup()
		if segments == 8_192 {
			if entries != 1 || extracted.SourceFailClosed || sink.allRowsFailClosed ||
				profilerRootWriterProfile(extracted) != profilerRootWriterProfileSequential {
				t.Fatalf("verified sequential segments rejected: extracted=%+v coverage=%+v", extracted, coverage)
			}
		} else if entries != 0 || extracted.Messages != 0 || extracted.RejectedMessages != 1 ||
			!extracted.SourceFailClosed || extracted.SourceFailReason != "profiler_root_segments_mismatch" ||
			!sink.allRowsFailClosed || !profilerRootProfileIntegrityBarrier(extracted) {
			t.Fatalf("unverified segments=%d escaped whole-source integrity fail-close: extracted=%+v coverage=%+v",
				segments, extracted, coverage)
		}
	}
}

func TestProfilerZeroFrameCensusKeepsTerminalFrameClassification(t *testing.T) {
	for _, test := range []struct {
		name             string
		terminal         profilerResourceFrame
		max              uint64
		wantReason       string
		wantSourceClosed bool
	}{
		{
			name:             "truncated",
			terminal:         profilerResourceFrame{declared: 32, payload: make([]byte, 31)},
			max:              64,
			wantReason:       "profiler_root_frame_truncated",
			wantSourceClosed: true,
		},
		{
			name:             "oversized",
			terminal:         profilerResourceFrame{declared: 65, payload: make([]byte, 65)},
			max:              64,
			wantReason:       "plugin_frame_size_budget_exceeded",
			wantSourceClosed: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := profilerZeroFrameFixture(4_096, test.terminal)
			header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
			if !ok {
				t.Fatal("read zero+terminal fixture header")
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			rootProof := profilerRootProofForTest(
				t, bytes.NewReader(body), int64(len(body)), header, test.max)
			extracted, err := extractProfilerTraceFileAtWithFrameLimit(
				context.Background(), bytes.NewReader(body), int64(len(body)), header, rootProof, sink, test.max)
			if err != nil {
				t.Fatal(err)
			}
			zeroCoverage, entries := profilerZeroFrameCoverage(extracted)
			if extracted.Messages != 0 || extracted.RejectedMessages != 1 ||
				extracted.SourceFailClosed != test.wantSourceClosed || entries != 0 || zeroCoverage.RowsRead != 0 ||
				!coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", test.wantReason) {
				t.Fatalf("zero+%s classification drifted: extracted=%+v coverage=%+v", test.name, extracted, extracted.TraceCoverage)
			}
			if test.wantSourceClosed && (!sink.allRowsFailClosed || zeroCoverage.RowsEmitted != 0) {
				t.Fatalf("zero census escaped terminal source fail-close: sink=%+v coverage=%+v", sink.stats, zeroCoverage)
			}
		})
	}
}

func TestProfilerZeroCensusAndValidPrefixAreSuppressedByOversizedFrame(t *testing.T) {
	prefix := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 1.000000: print: B|7|PrefixMustBeWithheld"))
	suffix := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 2.000000: print: B|7|SuffixMustNotScan"))
	max := uint64(len(prefix) + 16)
	oversized := make([]byte, int(max+1))
	frames := []profilerResourceFrame{{declared: uint32(len(prefix)), payload: prefix}}
	frames = append(frames, make([]profilerResourceFrame, 4_096)...)
	frames = append(frames,
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
		profilerResourceFrame{declared: uint32(len(suffix)), payload: suffix})
	body := profilerResourceTraceFile(frames...)
	header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
	if !ok {
		t.Fatal("read zero source-fail fixture header")
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rootProof := profilerRootProofForTest(
		t, bytes.NewReader(body), int64(len(body)), header, max)
	extracted, err := extractProfilerTraceFileAtWithFrameLimit(
		context.Background(), bytes.NewReader(body), int64(len(body)), header, rootProof, sink, max)
	if err != nil {
		t.Fatal(err)
	}
	zeroCoverage, entries := profilerZeroFrameCoverage(extracted)
	if !extracted.SourceFailClosed || extracted.Messages != 0 || extracted.RejectedMessages != 1 ||
		entries != 0 || zeroCoverage.RowsRead != 0 || zeroCoverage.RowsEmitted != 0 ||
		sink.stats.RowsAccepted != 0 || sink.publishableRows() != 0 {
		t.Fatalf("zero/prefix rows escaped oversized-frame source fail-close: extracted=%+v coverage=%+v sink=%+v",
			extracted, zeroCoverage, sink.stats)
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 || stats.RowsWritten != 0 || stats.RowsWithheld != 0 {
		t.Fatalf("zero/prefix source fail-close leaked output: stats=%+v output=%q", stats, output.String())
	}
}

func TestProfilerZeroPrefixCancellationAndIOErrorDoNotPublishSuccess(t *testing.T) {
	body := profilerZeroFrameFixture(2)
	header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
	if !ok {
		t.Fatal("read zero interruption fixture header")
	}
	rootProof := profilerRootProofForTest(
		t, bytes.NewReader(body), int64(len(body)), header, maxProfilerPluginFrameBytes)
	t.Run("cancel after first prefix", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &profilerCensusReaderAt{
			source: bytes.NewReader(body), cancelAfter: 1, cancel: cancel,
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 128)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		extracted, err := extractProfilerTraceFileAtWithFrameLimit(
			ctx, reader, int64(len(body)), header, rootProof, sink, maxProfilerPluginFrameBytes)
		if !errors.Is(err, context.Canceled) || extracted.Detected || sink.stats.RowsAccepted != 0 {
			t.Fatalf("cancelled zero prefix was packaged as success: err=%v extracted=%+v sink=%+v",
				err, extracted, sink.stats)
		}
	})
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "reader error", err: errors.New("injected zero-prefix read error")},
		{name: "unexpected EOF", err: io.ErrUnexpectedEOF},
		{name: "EOF", err: io.EOF},
	} {
		t.Run(test.name+" after first prefix", func(t *testing.T) {
			reader := &profilerCensusReaderAt{
				source: bytes.NewReader(body), failAt: 2, failErr: test.err,
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			extracted, err := extractProfilerTraceFileAtWithFrameLimit(
				context.Background(), reader, int64(len(body)), header, rootProof, sink, maxProfilerPluginFrameBytes)
			if !errors.Is(err, test.err) || extracted.Detected || sink.stats.RowsAccepted != 0 {
				t.Fatalf("failed zero prefix was packaged as success: err=%v extracted=%+v sink=%+v",
					err, extracted, sink.stats)
			}
		})
	}
}

func TestProfilerZeroFrameCensusAllocationDoesNotScaleWithFrameCount(t *testing.T) {
	fixtures := []struct {
		name string
		body []byte
	}{
		{name: "one", body: profilerZeroFrameFixture(1)},
		{name: "four_thousand", body: profilerZeroFrameFixture(4_096)},
		{name: "million", body: profilerZeroFrameFixture(1_000_000)},
	}
	allocs := func(body []byte) float64 {
		tempDir := t.TempDir()
		header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
		if !ok {
			t.Fatal("read zero-frame allocation fixture header")
		}
		rootProof := profilerRootProofForTest(
			t, bytes.NewReader(body), int64(len(body)), header, maxProfilerPluginFrameBytes)
		best := math.MaxFloat64
		for sample := 0; sample < 3; sample++ {
			got := testing.AllocsPerRun(1, func() {
				sink, err := newTraceDBRowSink(tempDir, 128)
				if err != nil {
					panic(err)
				}
				_, err = extractProfilerTraceFileAtWithFrameLimit(
					context.Background(), bytes.NewReader(body), int64(len(body)), header, rootProof, sink, maxProfilerPluginFrameBytes)
				sink.cleanup()
				if err != nil {
					panic(err)
				}
			})
			if got < best {
				best = got
			}
		}
		return best
	}
	allocationCounts := make(map[string]float64, len(fixtures))
	for _, fixture := range fixtures {
		allocationCounts[fixture.name] = allocs(fixture.body)
	}
	allocatedBytes := func(body []byte) uint64 {
		tempDir := t.TempDir()
		header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
		if !ok {
			t.Fatal("read zero-frame allocation-byte fixture header")
		}
		rootProof := profilerRootProofForTest(
			t, bytes.NewReader(body), int64(len(body)), header, maxProfilerPluginFrameBytes)
		best := uint64(math.MaxUint64)
		for sample := 0; sample < 3; sample++ {
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			sink, err := newTraceDBRowSink(tempDir, 128)
			if err != nil {
				t.Fatal(err)
			}
			_, err = extractProfilerTraceFileAtWithFrameLimit(
				context.Background(), bytes.NewReader(body), int64(len(body)), header, rootProof, sink, maxProfilerPluginFrameBytes)
			sink.cleanup()
			if err != nil {
				t.Fatal(err)
			}
			runtime.ReadMemStats(&after)
			if got := after.TotalAlloc - before.TotalAlloc; got < best {
				best = got
			}
		}
		return best
	}
	allocationBytes := make(map[string]uint64, len(fixtures))
	for _, fixture := range fixtures {
		allocationBytes[fixture.name] = allocatedBytes(fixture.body)
	}
	t.Logf("zero-frame retained allocations: counts=%v bytes=%v", allocationCounts, allocationBytes)
	allocationCountSlack := float64(8)
	if profilerRaceInstrumentationEnabled {
		// The race runtime contributes a small, frame-count-independent set of
		// bookkeeping allocations as the loop runs long enough to cross more
		// instrumentation epochs. Retained bytes are the hard resource signal;
		// keep the normal-build count pin exact and bound the measured race
		// overhead rather than treating a dozen allocations per million frames
		// as per-frame growth.
		allocationCountSlack = 16
	}
	for _, name := range []string{"four_thousand", "million"} {
		if allocationCounts[name] > allocationCounts["one"]+allocationCountSlack ||
			allocationBytes[name] > allocationBytes["one"]+(64<<10) {
			t.Fatalf("zero-frame retained allocations scale at %s: counts=%v bytes=%v",
				name, allocationCounts, allocationBytes)
		}
	}
}

func TestProfilerContainerCounterConversionsAreChecked(t *testing.T) {
	counter := math.MaxInt - 1
	if !incrementProfilerContainerCounter(&counter) || counter != math.MaxInt ||
		incrementProfilerContainerCounter(&counter) || counter != math.MaxInt {
		t.Fatalf("container int counter overflow guard drifted: %d", counter)
	}
	if got, ok := profilerContainerCountToInt(uint64(math.MaxInt)); !ok || got != math.MaxInt {
		t.Fatalf("max int census conversion rejected: got=%d ok=%t", got, ok)
	}
	if math.MaxInt < math.MaxUint64 {
		if got, ok := profilerContainerCountToInt(uint64(math.MaxInt) + 1); ok || got != 0 {
			t.Fatalf("overflow census conversion admitted: got=%d ok=%t", got, ok)
		}
	}
}

func TestProfilerZeroFrameCensusStructurePinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve zero-frame census test source")
	}
	path := filepath.Join(filepath.Dir(current), "profiler_container.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var extractor *ast.FuncDecl
	censusFields := map[string]string{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "extractProfilerTraceFileAtWithFrameLimit" {
			extractor = function
		}
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range generic.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "profilerZeroFrameCensus" {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatal("zero-frame census is no longer a fixed scalar struct")
			}
			for _, field := range structure.Fields.List {
				typeName, ok := field.Type.(*ast.Ident)
				if !ok || len(field.Names) != 1 {
					t.Fatal("zero-frame census gained a dynamic retained field")
				}
				censusFields[field.Names[0].Name] = typeName.Name
			}
		}
	}
	if len(censusFields) != 3 || censusFields["count"] != "uint64" ||
		censusFields["firstOffset"] != "int64" || censusFields["lastOffset"] != "int64" {
		t.Fatalf("zero-frame census scalar shape drifted: %+v", censusFields)
	}
	if extractor == nil {
		t.Fatal("missing bounded TraceFile ReaderAt authority")
	}
	zeroBranches := 0
	ast.Inspect(extractor.Body, func(node ast.Node) bool {
		ifStatement, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		binary, ok := ifStatement.Cond.(*ast.BinaryExpr)
		if !ok || binary.Op != token.EQL {
			return true
		}
		ident, leftOK := binary.X.(*ast.Ident)
		literal, rightOK := binary.Y.(*ast.BasicLit)
		if !leftOK || !rightOK || ident.Name != "n" || literal.Value != "0" {
			return true
		}
		zeroBranches++
		appendCalls := 0
		observeCalls := 0
		ast.Inspect(ifStatement.Body, func(child ast.Node) bool {
			call, ok := child.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.Ident:
				if callee.Name == "append" {
					appendCalls++
				}
			case *ast.SelectorExpr:
				if callee.Sel.Name == "observe" {
					observeCalls++
				}
			}
			return true
		})
		if appendCalls != 0 || observeCalls != 1 {
			t.Fatalf("zero-frame branch regained per-frame retained state: append=%d observe=%d",
				appendCalls, observeCalls)
		}
		return true
	})
	if zeroBranches != 1 {
		t.Fatalf("zero-frame branch count=%d want=1", zeroBranches)
	}
	aggregates := 0
	ast.Inspect(extractor.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == "appendProfilerZeroFrameCensus" {
			aggregates++
		}
		return true
	})
	if aggregates != 1 {
		t.Fatalf("zero-frame census finalization sites=%d want=1", aggregates)
	}
}
