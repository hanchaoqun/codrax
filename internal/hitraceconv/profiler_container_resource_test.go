package hitraceconv

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type profilerFillThenSuffixReader struct {
	remaining  int64
	fill       byte
	suffix     []byte
	maxRequest int
	totalRead  int64
	cancelAt   int64
	cancel     context.CancelFunc
}

func (reader *profilerFillThenSuffixReader) Read(dst []byte) (int, error) {
	if len(dst) > reader.maxRequest {
		reader.maxRequest = len(dst)
	}
	if reader.remaining > 0 {
		n := int64(len(dst))
		if n > reader.remaining {
			n = reader.remaining
		}
		for index := range dst[:int(n)] {
			dst[index] = reader.fill
		}
		reader.remaining -= n
		reader.totalRead += n
		if reader.cancel != nil && reader.cancelAt > 0 && reader.totalRead >= reader.cancelAt {
			reader.cancel()
			reader.cancel = nil
		}
		return int(n), nil
	}
	if len(reader.suffix) > 0 {
		n := copy(dst, reader.suffix)
		reader.suffix = reader.suffix[n:]
		reader.totalRead += int64(n)
		return n, nil
	}
	return 0, io.EOF
}

func scanProfilerResourceRecords(t testing.TB, input string, limit int) ([]profilerBoundedPhysicalLine, bool) {
	t.Helper()
	reader := bufio.NewReaderSize(strings.NewReader(input), 2)
	var records []profilerBoundedPhysicalLine
	stopped, err := scanProfilerBoundedSessionRecords(context.Background(), reader, limit,
		func(record profilerBoundedPhysicalLine) (bool, error) {
			record.Bytes = append([]byte(nil), record.Bytes...)
			records = append(records, record)
			return true, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return records, stopped
}

func TestProfilerBoundedSessionRecordBoundaries(t *testing.T) {
	const limit = 4
	tests := []struct {
		name      string
		input     string
		want      string
		present   bool
		oversized bool
		eof       bool
		count     int
	}{
		{name: "below LF", input: "abc\n", want: "abc", present: true, count: 1},
		{name: "below NUL", input: "abc\x00", want: "abc", present: true, count: 1},
		{name: "below CRLF", input: "abc\r\n", want: "abc", present: true, count: 1},
		{name: "exact LF", input: "abcd\n", want: "abcd", present: true, count: 1},
		{name: "exact NUL", input: "abcd\x00", want: "abcd", present: true, count: 1},
		{name: "exact CRLF", input: "abcd\r\n", want: "abcd", present: true, count: 1},
		{name: "above LF", input: "abcde\n", present: true, oversized: true, count: 1},
		{name: "above NUL", input: "abcde\x00", present: true, oversized: true, count: 1},
		{name: "exact EOF", input: "abcd", want: "abcd", present: true, eof: true, count: 1},
		{name: "above EOF", input: "abcde", present: true, oversized: true, eof: true, count: 1},
		{name: "empty LF record", input: "\n", present: true, count: 1},
		{name: "empty NUL record", input: "\x00", present: true, count: 1},
		{name: "empty input", input: "", count: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			records, stopped := scanProfilerResourceRecords(t, test.input, limit)
			if stopped || len(records) != test.count {
				t.Fatalf("bounded record count mismatch: stopped=%t records=%+v want=%d", stopped, records, test.count)
			}
			if test.count == 0 {
				return
			}
			record := records[0]
			if string(record.Bytes) != test.want || record.Present != test.present ||
				record.Oversized != test.oversized || record.EOF != test.eof {
				t.Fatalf("bounded record mismatch: got=%+v bytes=%q want=%q present=%t oversized=%t eof=%t",
					record, record.Bytes, test.want, test.present, test.oversized, test.eof)
			}
			if len(record.Bytes) > limit {
				t.Fatalf("bounded record retained beyond cap: len=%d limit=%d", len(record.Bytes), limit)
			}
		})
	}
}

func TestProfilerBoundedSessionRecordLFNULParity(t *testing.T) {
	lf, lfStopped := scanProfilerResourceRecords(t, "one\ntwo\r\nthree", 16)
	nul, nulStopped := scanProfilerResourceRecords(t, "one\x00two\r\x00three", 16)
	if lfStopped || nulStopped || len(lf) != len(nul) || len(lf) != 3 {
		t.Fatalf("LF/NUL record count parity drifted: lf=%+v stopped=%t nul=%+v stopped=%t", lf, lfStopped, nul, nulStopped)
	}
	for index := range lf {
		if string(lf[index].Bytes) != string(nul[index].Bytes) ||
			lf[index].Present != nul[index].Present || lf[index].Oversized != nul[index].Oversized ||
			lf[index].EOF != nul[index].EOF {
			t.Fatalf("LF/NUL record %d parity drifted: lf=%+v nul=%+v", index, lf[index], nul[index])
		}
	}
}

func TestProfilerBoundedSessionRecordDrainsHostileRecordWithRetainedCap(t *testing.T) {
	const (
		limit    = 1024
		lineSize = int64(8 << 20)
	)
	source := &profilerFillThenSuffixReader{remaining: lineSize, fill: 'x', suffix: []byte("\x00next\r\n")}
	reader := bufio.NewReaderSize(source, 256)
	var records []profilerBoundedPhysicalLine
	stopped, err := scanProfilerBoundedSessionRecords(context.Background(), reader, limit,
		func(record profilerBoundedPhysicalLine) (bool, error) {
			record.Bytes = append([]byte(nil), record.Bytes...)
			records = append(records, record)
			return true, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if stopped || len(records) != 2 || !records[0].Present || !records[0].Oversized ||
		records[0].EOF || len(records[0].Bytes) != 0 || cap(records[0].Bytes) > limit {
		t.Fatalf("hostile record was retained or not classified: stopped=%t records=%+v", stopped, records)
	}
	if source.maxRequest > 256 {
		t.Fatalf("bounded reader requested an unbounded source buffer: max_request=%d", source.maxRequest)
	}
	if next := records[1]; string(next.Bytes) != "next" || !next.Present || next.Oversized || next.EOF {
		t.Fatalf("hostile record drain lost the following CRLF sibling: %+v bytes=%q", next, next.Bytes)
	}
}

func TestProfilerBoundedSessionRecordDrains64MiBNoDelimiterWithRetainedCap(t *testing.T) {
	const streamBytes = int64(64 << 20)
	source := &profilerFillThenSuffixReader{remaining: streamBytes, fill: 'x'}
	reader := bufio.NewReaderSize(source, profilerSessionReaderBufBytes)
	visited := 0
	stopped, err := scanProfilerBoundedSessionRecords(context.Background(), reader, maxProfilerTextLineBytes,
		func(record profilerBoundedPhysicalLine) (bool, error) {
			visited++
			if !record.Present || !record.Oversized || !record.EOF || len(record.Bytes) != 0 ||
				cap(record.Bytes) > maxProfilerTextLineBytes+1 {
				t.Fatalf("64MiB no-delimiter record retained unbounded state: %+v cap=%d", record, cap(record.Bytes))
			}
			return false, nil
		})
	if err != nil || !stopped || visited != 1 || source.totalRead != streamBytes ||
		source.maxRequest > profilerSessionReaderBufBytes {
		t.Fatalf("64MiB bounded drain drifted: stopped=%t visited=%d read=%d max_request=%d err=%v",
			stopped, visited, source.totalRead, source.maxRequest, err)
	}
}

func TestProfilerBoundedSessionRecordCancellationInterruptsDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	source := &profilerFillThenSuffixReader{
		remaining: 8 << 20,
		fill:      'x',
		cancelAt:  2048,
		cancel:    cancel,
	}
	reader := bufio.NewReaderSize(source, 256)
	visited := 0
	stopped, err := scanProfilerBoundedSessionRecords(ctx, reader, 1024,
		func(profilerBoundedPhysicalLine) (bool, error) {
			visited++
			return true, nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("bounded record drain error=%v, want context.Canceled", err)
	}
	if stopped || visited != 0 {
		t.Fatalf("cancelled partial record leaked an authoritative result: stopped=%t visited=%d", stopped, visited)
	}
}

func TestProfilerBoundedSessionRecordAllocationDoesNotScaleWithHostileLength(t *testing.T) {
	allocBytes := func(size int64) int64 {
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				source := &profilerFillThenSuffixReader{remaining: size, fill: 'x', suffix: []byte("\n")}
				reader := bufio.NewReaderSize(source, 256)
				visited := 0
				stopped, err := scanProfilerBoundedSessionRecords(context.Background(), reader, 1024,
					func(record profilerBoundedPhysicalLine) (bool, error) {
						visited++
						if !record.Present || !record.Oversized || record.EOF || len(record.Bytes) != 0 {
							b.Fatalf("bounded allocation fixture drifted: record=%+v", record)
						}
						return true, nil
					})
				if err != nil || stopped || visited != 1 {
					b.Fatalf("bounded allocation scan drifted: stopped=%t visited=%d err=%v", stopped, visited, err)
				}
			}
		})
		return result.AllocedBytesPerOp()
	}
	small := allocBytes(64 << 10)
	large := allocBytes(2 << 20)
	t.Logf("bounded Session record allocated bytes/op: 64KiB=%d 2MiB=%d", small, large)
	if large > small+(64<<10) {
		t.Fatalf("hostile record allocation scales with input bytes: small=%d large=%d", small, large)
	}
}

func TestProfilerSessionLineBudgetFailClosesSourceAndStopsSuffix(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	startLine := "io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody
	doneLine := "io-100 (100) [002] .... 1.001000: mmc_request_done: " + doneBody
	printBefore := "other-7 (7) [001] .... 1.002000: print: B|7|Before"
	printAfter := "other-7 (7) [001] .... 1.003000: print: B|7|After"
	limit := len(startLine)
	for _, line := range []string{doneLine, printBefore, printAfter, profilerSessionJSONTag} {
		if len(line) > limit {
			limit = len(line)
		}
	}
	limit += 8
	oversized := strings.Repeat("x", limit+1)
	payload := strings.Join([]string{
		profilerSessionJSONTag,
		startLine,
		doneLine,
		printBefore,
		oversized,
		printAfter, // Exercise the EOF-without-final-LF sibling as well.
	}, "\n")

	dir := t.TempDir()
	input := filepath.Join(dir, "session-line-budget.htrace")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerSessionPackageWithLineLimit(context.Background(), input, int64(len(payload)), sink, limit)
	if err != nil {
		t.Fatal(err)
	}
	if !extracted.Detected || extracted.RejectedMessages != 1 || extracted.TextRows != 0 ||
		!extracted.SourceFailClosed || extracted.SourceFailReason != "session_line_size_budget_exceeded" ||
		!coverageTableHasSkipped(extracted.TraceCoverage, "session:SessionJSON", "session_line_size_budget_exceeded=1") {
		t.Fatalf("Session line budget coverage drifted: extracted=%+v coverage=%+v", extracted, extracted.TraceCoverage)
	}
	if !sink.allRowsFailClosed || sink.stats.RowsAccepted != 3 || sink.publishableRows() != 0 {
		t.Fatalf("oversized Session record escaped source fail-close or scanned its suffix: accepted=%d publishable=%d fail_closed=%t",
			sink.stats.RowsAccepted, sink.publishableRows(), sink.allRowsFailClosed)
	}
	for _, item := range extracted.TraceCoverage {
		if item.Family == "builtin_modern_profiler" && item.RowsEmitted != 0 {
			t.Fatalf("source-failed Session retained emitted-row authority: %+v", item)
		}
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsWritten != 0 || stats.RowsWithheld != 3 ||
		strings.Contains(output.String(), "mmc_request_") || strings.Contains(output.String(), "print: B|7|") {
		t.Fatalf("Session source fail-close leaked prefix or suffix rows: stats=%+v\n%s", stats, output.String())
	}
}

func TestProfilerSessionLFAndNULDelimiterParity(t *testing.T) {
	records := []string{
		profilerSessionJSONTag,
		"other-7 (7) [001] .... 1.000000: print: B|7|One",
		"other-7 (7) [001] .... 1.001000: print: B|7|Two",
	}
	outputs := map[string]string{}
	for name, delimiter := range map[string]string{"LF": "\n", "NUL": "\x00"} {
		t.Run(name, func(t *testing.T) {
			payload := strings.Join(records, delimiter)
			input := filepath.Join(t.TempDir(), "session-parity.htrace")
			if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			extracted, err := extractProfilerSessionPackageWithLineLimit(
				context.Background(), input, int64(len(payload)), sink, maxProfilerTextLineBytes)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
			if err != nil {
				t.Fatal(err)
			}
			if !extracted.Detected || extracted.SourceFailClosed || extracted.TextRows != 2 || stats.RowsWritten != 2 {
				t.Fatalf("%s-delimited Session extraction drifted: extracted=%+v stats=%+v", name, extracted, stats)
			}
			outputs[name] = output.String()
		})
	}
	if outputs["LF"] != outputs["NUL"] {
		t.Fatalf("LF/NUL Session publication parity drifted:\nLF:\n%s\nNUL:\n%s", outputs["LF"], outputs["NUL"])
	}
}

func TestProfilerSessionNULOversizeFailClosesSourceAndStopsSuffix(t *testing.T) {
	const limit = 96
	prefix := "other-7 (7) [001] .... 1.000000: print: B|7|Before"
	suffix := "other-7 (7) [001] .... 1.001000: print: B|7|MustNotScan"
	payload := strings.Join([]string{
		profilerSessionJSONTag,
		prefix,
		strings.Repeat("x", limit+1),
		suffix,
	}, "\x00")
	input := filepath.Join(t.TempDir(), "session-nul-line-budget.htrace")
	if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerSessionPackageWithLineLimit(
		context.Background(), input, int64(len(payload)), sink, limit)
	if err != nil {
		t.Fatal(err)
	}
	if !extracted.SourceFailClosed || extracted.SourceFailReason != "session_line_size_budget_exceeded" ||
		extracted.RejectedMessages != 1 || sink.stats.RowsAccepted != 1 || sink.publishableRows() != 0 {
		t.Fatalf("NUL oversized Session record escaped source fail-close: extracted=%+v sink=%+v",
			extracted, sink.stats)
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 || stats.RowsWritten != 0 || stats.RowsWithheld != 1 ||
		stats.FirstTSNS != 0 || stats.LastTSNS != 0 {
		t.Fatalf("NUL oversized Session record leaked prefix/suffix publication: stats=%+v output=%q",
			stats, output.String())
	}
}

func TestProfilerSourceFailCloseSuppressesSpilledChunksBeforeHeader(t *testing.T) {
	tempDir := t.TempDir()
	sink, err := newTraceDBRowSink(tempDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	for index, ts := range []uint64{2_000_000_000, 1_000_000_000} {
		if err := sink.add(renderedRow{
			tsNS: ts,
			seq:  index,
			line: "other-7 (7) [001] .... 1.000000: print: B|7|Spilled",
		}); err != nil {
			sink.cleanup()
			t.Fatal(err)
		}
	}
	if len(sink.runs) == 0 || sink.stats.SpillChunks == 0 {
		sink.cleanup()
		t.Fatalf("spill fixture did not create persisted runs: stats=%+v runs=%v", sink.stats, sink.runs)
	}
	runPaths := make([]string, len(sink.runs))
	for index, run := range sink.runs {
		runPaths[index] = run.path
	}
	sink.failCloseAllRows()
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 || stats.RowsAccepted != 2 || stats.RowsWritten != 0 || stats.RowsWithheld != 2 ||
		stats.FirstTSNS != 0 || stats.LastTSNS != 0 {
		t.Fatalf("source fail-close leaked a spill header or row: stats=%+v output=%q", stats, output.String())
	}
	for _, path := range runPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("source fail-close did not clean spill chunk %s: err=%v", path, err)
		}
	}
}

func TestProfilerSessionResourceFailClosePreservesTerminalStandaloneSidecar(t *testing.T) {
	perfPayload := []byte("TERMINAL-PERF-DATA")
	session := strings.Join([]string{
		profilerSessionJSONTag,
		"other-7 (7) [001] .... 1.000000: print: B|7|Before",
		strings.Repeat("x", maxProfilerTextLineBytes+1),
		"other-7 (7) [001] .... 1.001000: print: B|7|MustNotScan",
	}, "\x00")
	body := append([]byte(session), syntheticStandaloneProfilerBlock(
		profilerDataTypeHiperf, "hiperf-plugin", "1.0", perfPayload)...)
	dir := t.TempDir()
	input := filepath.Join(dir, "session-resource-terminal-sidecar.htrace")
	output := filepath.Join(dir, "out.systrace")
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 ||
		!hasTraceDecisionReason(result.TraceDecisions, traceProviderNameBuiltinModern, "profiler_source_resource_fail_closed") {
		t.Fatalf("resource-failed Session claimed trace-body publication: %+v", result)
	}
	assertCleanSorterCoverage := func(label string, items []TraceDBCoverage) {
		t.Helper()
		matches := 0
		for _, item := range items {
			if item.Family != "builtin_modern_profiler" || item.Table != "__systrace_rows__" {
				continue
			}
			matches++
			if item.RowsRead != 1 || item.RowsEmitted != 0 || item.SpillChunks != 1 ||
				item.CurrentLiveTempBytes != 0 || item.PeakLiveTempBytes == 0 {
				t.Fatalf("%s retained stale zero-output sorter state: %+v", label, item)
			}
		}
		if matches != 1 {
			t.Fatalf("%s sorter coverage count=%d want=1: %+v", label, matches, items)
		}
	}
	assertCleanSorterCoverage("result", result.TraceCoverage)
	if result.BundlePath == "" {
		t.Fatalf("resource-failed Session lost trace bundle: %+v", result)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var bundleMeta struct {
		TraceCoverage []TraceDBCoverage `json:"trace_coverage"`
	}
	if err := json.Unmarshal(bundle, &bundleMeta); err != nil {
		t.Fatalf("decode zero-output trace bundle: %v", err)
	}
	assertCleanSorterCoverage("bundle", bundleMeta.TraceCoverage)
	var sidecar Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfData {
			sidecar = artifact
			break
		}
	}
	if sidecar.Path == "" {
		t.Fatalf("resource-failed Session lost its independent sidecar: %+v", result.Artifacts)
	}
	gotPayload, err := os.ReadFile(sidecar.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotPayload, perfPayload) {
		t.Fatalf("resource-failed Session corrupted terminal sidecar: got=%q want=%q", gotPayload, perfPayload)
	}
}

func TestProfilerSessionEmbeddedStandaloneSidecarFailsClosedWithoutReinterpretation(t *testing.T) {
	perfPayload := []byte("other-99 (99) [003] .... 1.500000: print: B|99|MustStaySidecar")
	prefix := profilerSessionJSONTag + "\nother-7 (7) [001] .... 1.000000: print: B|7|Before\n"
	sidecarBlock := syntheticStandaloneProfilerBlock(
		profilerDataTypeHiperf, "hiperf-plugin", "1.0", perfPayload)
	suffix := []byte("\nother-7 (7) [001] .... 1.001000: print: B|7|After\n")
	body := append(append([]byte(prefix), sidecarBlock...), suffix...)
	dir := t.TempDir()
	input := filepath.Join(dir, "session-embedded-sidecar.htrace")
	output := filepath.Join(dir, "out.systrace")
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 ||
		!hasTraceDecisionReason(result.TraceDecisions, traceProviderNameBuiltinModern, "profiler_source_integrity_fail_closed") ||
		!containsString(result.Caveats, "session_embedded_standalone_sidecar_ambiguity") ||
		!coverageTableHasSkipped(result.TraceCoverage, "__container_integrity_barrier__",
			"profiler_source_fail_closed=session_embedded_standalone_sidecar_ambiguity") {
		t.Fatalf("embedded sidecar bytes were reinterpreted as Session trace authority: %+v", result)
	}
	integrityCoverage := false
	for _, item := range result.TraceCoverage {
		if item.Table == "__container_integrity_barrier__" {
			integrityCoverage = item.FieldSources["failure_class"] == "integrity_ambiguity" &&
				item.FieldSources["scope"] == "complete_profiler_trace_body"
		}
	}
	if !integrityCoverage || coverageTableHasSkipped(result.TraceCoverage, "__container_resource_barrier__",
		"session_embedded_standalone_sidecar_ambiguity") {
		t.Fatalf("embedded sidecar integrity failure was mislabeled as a resource barrier: %+v", result.TraceCoverage)
	}
	var sidecar Artifact
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactPerfData {
			sidecar = artifact
			break
		}
	}
	if sidecar.Path == "" {
		t.Fatalf("embedded sidecar fail-close lost independent artifact: %+v", result.Artifacts)
	}
	gotPayload, err := os.ReadFile(sidecar.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotPayload, perfPayload) {
		t.Fatalf("embedded sidecar artifact corrupted: got=%q want=%q", gotPayload, perfPayload)
	}
}

func TestProfilerSessionInputSizeExcludesAppendedSuffix(t *testing.T) {
	initial := strings.Join([]string{
		profilerSessionJSONTag,
		"other-7 (7) [001] .... 1.000000: print: B|7|Before",
	}, "\n")
	appended := "\nother-7 (7) [001] .... 1.001000: print: B|7|Appended"
	input := filepath.Join(t.TempDir(), "session-fixed-size.htrace")
	if err := os.WriteFile(input, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(input, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(appended); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerSessionPackageWithLineLimit(
		context.Background(), input, int64(len(initial)), sink, maxProfilerTextLineBytes)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if extracted.TextRows != 1 || stats.RowsWritten != 1 ||
		!strings.Contains(output.String(), "print: B|7|Before") || strings.Contains(output.String(), "Appended") {
		t.Fatalf("Session fixed input-size boundary drifted: extracted=%+v stats=%+v\n%s", extracted, stats, output.String())
	}
}

func TestProfilerContainerTraceBodySizeRequiresContiguousTerminalSidecars(t *testing.T) {
	const inputSize int64 = 1_000
	for _, test := range []struct {
		name          string
		artifacts     []Artifact
		want          int64
		wantAmbiguous bool
	}{
		{name: "none", want: inputSize},
		{name: "leading non-terminal sidecar is ambiguous", artifacts: []Artifact{{Type: ArtifactPerfData, SourceOffset: 0, SourceBytes: 100}}, want: inputSize, wantAmbiguous: true},
		{name: "whole-input terminal sidecar", artifacts: []Artifact{{Type: ArtifactPerfData, SourceOffset: 0, SourceBytes: inputSize}}, want: 0},
		{name: "middle is not a terminal boundary", artifacts: []Artifact{{Type: ArtifactPerfData, SourceOffset: 100, SourceBytes: 100}}, want: inputSize, wantAmbiguous: true},
		{name: "one terminal", artifacts: []Artifact{{Type: ArtifactPerfData, SourceOffset: 700, SourceBytes: 300}}, want: 700},
		{name: "contiguous terminal chain", artifacts: []Artifact{
			{Type: ArtifactPerfData, SourceOffset: 700, SourceBytes: 100},
			{Type: ArtifactPerfData, SourceOffset: 800, SourceBytes: 200},
		}, want: 700},
		{name: "overlapping terminal chain", artifacts: []Artifact{
			{Type: ArtifactPerfData, SourceOffset: 700, SourceBytes: 200},
			{Type: ArtifactPerfData, SourceOffset: 800, SourceBytes: 200},
		}, want: 700},
		{name: "nested and duplicate terminal ranges", artifacts: []Artifact{
			{Type: ArtifactPerfData, SourceOffset: 700, SourceBytes: 300},
			{Type: ArtifactPerfData, SourceOffset: 700, SourceBytes: 300},
			{Type: ArtifactPerfData, SourceOffset: 800, SourceBytes: 100},
		}, want: 700},
		{name: "gap before terminal is not skipped", artifacts: []Artifact{
			{Type: ArtifactPerfData, SourceOffset: 600, SourceBytes: 100},
			{Type: ArtifactPerfData, SourceOffset: 800, SourceBytes: 200},
		}, want: 800, wantAmbiguous: true},
		{name: "non-perf and overflowing ranges ignored", artifacts: []Artifact{
			{Type: ArtifactSystrace, SourceOffset: 700, SourceBytes: 300},
			{Type: ArtifactPerfData, SourceOffset: math.MaxInt64 - 10, SourceBytes: 20},
		}, want: inputSize},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := profilerContainerTraceBodySize(inputSize, test.artifacts); got != test.want {
				t.Fatalf("trace-body terminal sidecar boundary=%d want=%d", got, test.want)
			}
			gotBoundary, gotAmbiguous := profilerContainerSessionLayout(inputSize, test.artifacts)
			if gotBoundary != test.want || gotAmbiguous != test.wantAmbiguous {
				t.Fatalf("Session sidecar layout=(%d,%t) want=(%d,%t)",
					gotBoundary, gotAmbiguous, test.want, test.wantAmbiguous)
			}
		})
	}
}

type profilerResourceFrame struct {
	declared uint32
	payload  []byte
}

type profilerRecordingReaderAt struct {
	source io.ReaderAt
	reads  []struct {
		off   int64
		bytes int
	}
}

func (reader *profilerRecordingReaderAt) ReadAt(dst []byte, off int64) (int, error) {
	reader.reads = append(reader.reads, struct {
		off   int64
		bytes int
	}{off: off, bytes: len(dst)})
	return reader.source.ReadAt(dst, off)
}

func profilerResourceTraceFile(frames ...profilerResourceFrame) []byte {
	var payload bytes.Buffer
	for _, frame := range frames {
		var prefix [4]byte
		binary.LittleEndian.PutUint32(prefix[:], frame.declared)
		payload.Write(prefix[:])
		payload.Write(frame.payload)
	}
	body := make([]byte, profilerTraceHeaderSize+payload.Len())
	binary.LittleEndian.PutUint64(body[0:8], profilerTraceHeaderMagic)
	binary.LittleEndian.PutUint64(body[8:16], uint64(len(body)))
	binary.LittleEndian.PutUint32(body[16:20], 0x00010000)
	binary.LittleEndian.PutUint32(body[20:24], uint32(len(frames)*2))
	binary.LittleEndian.PutUint32(body[56:60], profilerDataTypeProtobuf)
	copy(body[profilerTraceHeaderSize:], payload.Bytes())
	return body
}

func extractProfilerResourceTraceFile(t *testing.T, body []byte, max uint64) (profilerContainerExtraction, *traceDBRowSink) {
	t.Helper()
	dir := t.TempDir()
	input := filepath.Join(dir, "resource-profiler.htrace")
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	header, ok, err := readProfilerTraceHeaderAtPath(input, 0, info.Size())
	if err != nil || !ok {
		t.Fatalf("read resource fixture header: ok=%t err=%v", ok, err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractProfilerTraceFileWithFrameLimit(context.Background(), input, info.Size(), header, sink, max)
	if err != nil {
		sink.cleanup()
		t.Fatalf("extract resource profiler fixture: %v", err)
	}
	return extracted, sink
}

func TestProfilerTraceFileIgnoresSessionSidecarBoundary(t *testing.T) {
	message := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 1.000000: print: B|7|TraceFile"))
	body := profilerResourceTraceFile(profilerResourceFrame{declared: uint32(len(message)), payload: message})
	input := filepath.Join(t.TempDir(), "trace-file-session-boundary.htrace")
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerContainerSystraceRowsWithSessionLimit(
		context.Background(), input, int64(len(body)), profilerTraceHeaderSize, sink)
	if err != nil {
		t.Fatal(err)
	}
	if !extracted.Detected || extracted.Kind != "openharmony_profiler_trace_file" ||
		extracted.TextRows != 1 || sink.stats.RowsAccepted != 1 {
		t.Fatalf("TraceFile was truncated by a Session-only sidecar boundary: extracted=%+v sink=%+v",
			extracted, sink.stats)
	}
}

func TestProfilerFrameSizeBudgetExactAndAbove(t *testing.T) {
	line := "worker-7 (7) [001] .... 1.000000: print: B|7|Good"
	message := syntheticProfilerPluginData("bytrace_plugin", []byte(line))
	for _, test := range []struct {
		name         string
		max          uint64
		wantRows     int
		wantRejected int
	}{
		{name: "exact", max: uint64(len(message)), wantRows: 1},
		{name: "one below", max: uint64(len(message) - 1), wantRejected: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			extracted, sink := extractProfilerResourceTraceFile(t, profilerResourceTraceFile(
				profilerResourceFrame{declared: uint32(len(message)), payload: message},
			), test.max)
			defer sink.cleanup()
			if extracted.TextRows != test.wantRows || extracted.RejectedMessages != test.wantRejected {
				t.Fatalf("frame cap boundary mismatch: extracted=%+v", extracted)
			}
			if test.wantRejected > 0 {
				if !extracted.SourceFailClosed || extracted.SourceFailReason != "plugin_frame_size_budget_exceeded" ||
					!coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", "plugin_frame_size_budget_exceeded") ||
					!sink.allRowsFailClosed || sink.publishableRows() != 0 {
					t.Fatalf("frame cap rejection lost typed source fail-close: extracted=%+v coverage=%+v sink=%+v",
						extracted, extracted.TraceCoverage, sink.stats)
				}
			} else if extracted.SourceFailClosed || sink.allRowsFailClosed {
				t.Fatalf("exact frame cap spuriously failed its source closed: extracted=%+v", extracted)
			}
		})
	}
}

func TestProfilerOversizedFrameReadsZeroPayloadBytes(t *testing.T) {
	const max = uint64(64)
	body := profilerResourceTraceFile(profilerResourceFrame{
		declared: uint32(max + 1),
		payload:  make([]byte, int(max+1)),
	})
	header, ok := readProfilerTraceHeaderAt(bytes.NewReader(body), 0, int64(len(body)))
	if !ok {
		t.Fatal("read oversized ReaderAt fixture header")
	}
	reader := &profilerRecordingReaderAt{source: bytes.NewReader(body)}
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	extracted, err := extractProfilerTraceFileAtWithFrameLimit(
		context.Background(), reader, int64(len(body)), header, sink, max)
	if err != nil {
		t.Fatal(err)
	}
	if !extracted.SourceFailClosed || extracted.SourceFailReason != "plugin_frame_size_budget_exceeded" {
		t.Fatalf("oversized ReaderAt fixture did not fail closed: %+v", extracted)
	}
	payloadOffset := int64(profilerTraceHeaderSize + 4)
	for _, read := range reader.reads {
		if read.off == payloadOffset || read.off > int64(profilerTraceHeaderSize) {
			t.Fatalf("oversized frame payload/suffix was read: payload_offset=%d reads=%+v", payloadOffset, reader.reads)
		}
	}
}

func TestProfilerOversizedFrameStopsWithoutTrustingPrefixSiblingAndFailClosesSource(t *testing.T) {
	startPayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_start")
	donePayload, _, _ := decodeDirectMMCPayloadFromFixtureForTest(t, "mmc_request_done")
	startBody, _ := renderCanonicalMMCPayload(startPayload)
	doneBody, _ := renderCanonicalMMCPayload(donePayload)
	printBefore := "other-7 (7) [001] .... 1.002000: print: B|7|Before"
	beforePayload := strings.Join([]string{
		"io-100 (100) [002] .... 1.000000: mmc_request_start: " + startBody,
		"io-100 (100) [002] .... 1.001000: mmc_request_done: " + doneBody,
		printBefore,
	}, "\n")
	before := syntheticProfilerPluginData("bytrace_plugin", []byte(beforePayload))
	after := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 1.003000: print: B|7|MustNotScan"))
	max := uint64(len(before) + 16)
	oversized := make([]byte, int(max+1))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: uint32(len(before)), payload: before},
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
		// These bytes form a complete legal frame only if the untrusted oversized
		// prefix is used as a sibling boundary. P1-a must not do that.
		profilerResourceFrame{declared: uint32(len(after)), payload: after},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, max)
	defer sink.cleanup()
	if extracted.Messages != 2 || extracted.RejectedMessages != 1 || extracted.TextRows != 0 ||
		!extracted.SourceFailClosed || extracted.SourceFailReason != "plugin_frame_size_budget_exceeded" ||
		extracted.PluginMessages["bytrace_plugin"] != 1 ||
		!coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", "plugin_frame_size_budget_exceeded") {
		t.Fatalf("oversized frame stop/coverage mismatch: extracted=%+v coverage=%+v", extracted, extracted.TraceCoverage)
	}
	if !sink.allRowsFailClosed || sink.stats.RowsAccepted != 3 || sink.publishableRows() != 0 {
		t.Fatalf("oversized frame escaped source fail-close: accepted=%d publishable=%d fail_closed=%t",
			sink.stats.RowsAccepted, sink.publishableRows(), sink.allRowsFailClosed)
	}
	bucketFailClosed := false
	for _, item := range extracted.TraceCoverage {
		if item.Family == "builtin_modern_profiler" && item.RowsEmitted != 0 {
			t.Fatalf("source-failed frame retained emitted-row authority: %+v", item)
		}
		if item.Family == "builtin_modern_profiler" && item.Table == "plugin:bytrace_plugin" &&
			item.RowsRead == 1 && item.FieldSources["observed_messages"] == "1" &&
			item.FieldSources["profiler_trace_body_source_fail_closed"] == "plugin_frame_size_budget_exceeded" {
			bucketFailClosed = true
		}
	}
	if !bucketFailClosed {
		t.Fatalf("source fail-close did not preserve and zero the fixed plugin bucket audit: %+v", extracted.TraceCoverage)
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsWritten != 0 || stats.RowsWithheld != 3 ||
		strings.Contains(output.String(), "mmc_request_") || strings.Contains(output.String(), "print: B|7|") {
		t.Fatalf("oversized frame leaked trace-body prefix or suffix rows: stats=%+v\n%s", stats, output.String())
	}
}

func TestProfilerOversizedFrameClearsStructuredPrefixCoverage(t *testing.T) {
	structuredPayload := protoMessage(2,
		protoVarint(1, 0),
		syntheticTracePluginFtraceEvent(
			5_000_000_000, 7, 7, "worker", 1109, protoBytes(2, []byte("B|7|StructuredPrefix"))),
	)
	prefix := syntheticProfilerPluginData("ftrace-plugin", structuredPayload)
	max := uint64(len(prefix) + 16)
	oversized := make([]byte, int(max+1))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: uint32(len(prefix)), payload: prefix},
		profilerResourceFrame{declared: uint32(len(oversized)), payload: oversized},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, max)
	defer sink.cleanup()
	if extracted.StructuredFtrace != 1 || extracted.StructuredRows != 0 ||
		!extracted.SourceFailClosed || sink.stats.RowsAccepted != 1 || sink.publishableRows() != 0 {
		t.Fatalf("structured prefix escaped oversized-frame source fail-close: extracted=%+v sink=%+v",
			extracted, sink.stats)
	}
	for _, item := range extracted.TraceCoverage {
		if item.RowsEmitted != 0 {
			t.Fatalf("structured subcoverage retained emitted authority after source fail-close: %+v", item)
		}
	}
}

func TestProfilerMaxUint32FramePrefixIsTruncatedBeforeBudgetClassification(t *testing.T) {
	after := syntheticProfilerPluginData("bytrace_plugin", []byte(
		"other-7 (7) [001] .... 1.000000: print: B|7|MustNotScan"))
	body := profilerResourceTraceFile(
		profilerResourceFrame{declared: math.MaxUint32},
		profilerResourceFrame{declared: uint32(len(after)), payload: after},
	)
	extracted, sink := extractProfilerResourceTraceFile(t, body, 64)
	defer sink.cleanup()
	if extracted.Messages != 1 || extracted.RejectedMessages != 1 || extracted.TextRows != 0 ||
		sink.stats.RowsAccepted != 0 || extracted.SourceFailClosed || sink.allRowsFailClosed ||
		coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", "plugin_frame_size_budget_exceeded") ||
		!coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", "plugin_frame_truncated") {
		t.Fatalf("MaxUint32 incomplete frame did not preserve truncated-before-budget classification: extracted=%+v coverage=%+v sink=%+v",
			extracted, extracted.TraceCoverage, sink.stats)
	}
}

func TestProfilerWithinCapIncompleteFrameIsTruncatedBeforeAllocation(t *testing.T) {
	const (
		declared = uint32(32)
		max      = uint64(64)
	)
	body := profilerResourceTraceFile(profilerResourceFrame{
		declared: declared,
		payload:  make([]byte, int(declared)-1),
	})
	extracted, sink := extractProfilerResourceTraceFile(t, body, max)
	defer sink.cleanup()
	if extracted.Messages != 1 || extracted.RejectedMessages != 1 || extracted.SourceFailClosed ||
		sink.allRowsFailClosed || sink.stats.RowsAccepted != 0 ||
		!coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", "plugin_frame_truncated") ||
		coverageTableHasSkipped(extracted.TraceCoverage, "plugin:__rejected__", "plugin_frame_size_budget_exceeded") {
		t.Fatalf("within-cap incomplete frame did not preserve truncated-before-allocation classification: extracted=%+v coverage=%+v sink=%+v",
			extracted, extracted.TraceCoverage, sink.stats)
	}
}

func TestProfilerContainerResourceCancellationDoesNotStageRows(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Session", func(t *testing.T) {
		dir := t.TempDir()
		input := filepath.Join(dir, "cancel-session.htrace")
		payload := profilerSessionJSONTag + "\nother-7 (7) [001] .... 1.000000: print: B|7|No\n"
		if err := os.WriteFile(input, []byte(payload), 0o644); err != nil {
			t.Fatal(err)
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 128)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		_, err = extractProfilerSessionPackageWithLineLimit(ctx, input, int64(len(payload)), sink, maxProfilerTextLineBytes)
		if !errors.Is(err, context.Canceled) || sink.stats.RowsAccepted != 0 {
			t.Fatalf("cancelled Session extraction err=%v sink=%+v", err, sink.stats)
		}
	})

	t.Run("TraceFile", func(t *testing.T) {
		message := syntheticProfilerPluginData("bytrace_plugin", []byte(
			"other-7 (7) [001] .... 1.000000: print: B|7|No"))
		body := profilerResourceTraceFile(profilerResourceFrame{declared: uint32(len(message)), payload: message})
		dir := t.TempDir()
		input := filepath.Join(dir, "cancel-profiler.htrace")
		if err := os.WriteFile(input, body, 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(input)
		if err != nil {
			t.Fatal(err)
		}
		header, ok, err := readProfilerTraceHeaderAtPath(input, 0, info.Size())
		if err != nil || !ok {
			t.Fatalf("read cancellation fixture header: ok=%t err=%v", ok, err)
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 128)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		_, err = extractProfilerTraceFileWithFrameLimit(ctx, input, info.Size(), header, sink, maxProfilerPluginFrameBytes)
		if !errors.Is(err, context.Canceled) || sink.stats.RowsAccepted != 0 {
			t.Fatalf("cancelled TraceFile extraction err=%v sink=%+v", err, sink.stats)
		}
	})
}

func TestProfilerContainerResourceStructurePinned(t *testing.T) {
	if maxProfilerPluginFrameBytes != 64<<20 {
		t.Fatalf("production profiler frame cap=%d want=%d", maxProfilerPluginFrameBytes, 64<<20)
	}
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve resource test source")
	}
	path := filepath.Join(filepath.Dir(current), "profiler_container.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	functions := map[string]*ast.FuncDecl{}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			functions[function.Name.Name] = function
		}
	}
	required := []string{
		"extractProfilerSessionPackage", "extractProfilerSessionPackageWithLineLimit",
		"scanProfilerBoundedSessionRecords",
		"extractProfilerTraceFile", "extractProfilerTraceFileWithFrameLimit", "extractProfilerTraceFileAtWithFrameLimit",
		"extractProfilerContainerSystraceRowsWithSessionLimit", "tryConvertProfilerContainer",
	}
	for _, name := range required {
		if functions[name] == nil {
			t.Fatalf("profiler resource closure missing %s", name)
		}
	}
	if functions["readProfilerBoundedPhysicalLine"] != nil {
		t.Fatal("a second LF-only Session record reader bypasses the LF/NUL authority")
	}
	callSites := func(function *ast.FuncDecl, name string) []*ast.CallExpr {
		var out []*ast.CallExpr
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.Ident:
				if callee.Name == name {
					out = append(out, call)
				}
			case *ast.SelectorExpr:
				if callee.Sel.Name == name {
					out = append(out, call)
				}
			}
			return true
		})
		return out
	}
	isIdent := func(expression ast.Expr, name string) bool {
		ident, ok := expression.(*ast.Ident)
		return ok && ident.Name == name
	}

	sessionWrapper := callSites(functions["extractProfilerSessionPackage"], "extractProfilerSessionPackageWithLineLimit")
	if len(sessionWrapper) != 1 || len(sessionWrapper[0].Args) != 5 ||
		!isIdent(sessionWrapper[0].Args[2], "inputSize") ||
		!isIdent(sessionWrapper[0].Args[len(sessionWrapper[0].Args)-1], "maxProfilerTextLineBytes") {
		t.Fatalf("Session wrapper does not pass the fixed input size and production line cap exactly once: %+v", sessionWrapper)
	}
	traceWrapper := callSites(functions["extractProfilerTraceFile"], "extractProfilerTraceFileWithFrameLimit")
	if len(traceWrapper) != 1 || len(traceWrapper[0].Args) == 0 ||
		!isIdent(traceWrapper[0].Args[len(traceWrapper[0].Args)-1], "maxProfilerPluginFrameBytes") {
		t.Fatalf("TraceFile wrapper does not pass the production frame cap exactly once: %+v", traceWrapper)
	}
	traceReaderWrapper := callSites(functions["extractProfilerTraceFileWithFrameLimit"], "extractProfilerTraceFileAtWithFrameLimit")
	if len(traceReaderWrapper) != 1 || len(traceReaderWrapper[0].Args) == 0 ||
		!isIdent(traceReaderWrapper[0].Args[len(traceReaderWrapper[0].Args)-1], "maxFrameBytes") {
		t.Fatalf("TraceFile path wrapper does not delegate its frame cap to the ReaderAt authority exactly once: %+v",
			traceReaderWrapper)
	}
	for _, functionName := range []string{
		"extractProfilerSessionPackage", "extractProfilerSessionPackageWithLineLimit", "scanProfilerBoundedSessionRecords",
	} {
		for _, forbidden := range []string{"ReadBytes", "ReadString", "ReadAll"} {
			if calls := callSites(functions[functionName], forbidden); len(calls) != 0 {
				t.Fatalf("%s retained unbounded %s calls: %+v", functionName, forbidden, calls)
			}
		}
	}
	sessionLimited := functions["extractProfilerSessionPackageWithLineLimit"]
	sessionOpen := callSites(sessionLimited, "Open")
	sessionMarker := callSites(sessionLimited, "profilerSessionJSONMarkerOffsetAt")
	sessionSections := callSites(sessionLimited, "NewSectionReader")
	sessionScans := callSites(sessionLimited, "scanProfilerBoundedSessionRecords")
	if len(sessionOpen) != 1 || len(sessionMarker) != 1 || len(sessionMarker[0].Args) < 2 ||
		!isIdent(sessionMarker[0].Args[0], "f") || !isIdent(sessionMarker[0].Args[1], "inputSize") ||
		len(sessionSections) != 1 || len(sessionSections[0].Args) != 3 ||
		!isIdent(sessionSections[0].Args[0], "f") || !isIdent(sessionSections[0].Args[2], "inputSize") ||
		len(sessionScans) != 1 || len(callSites(sessionLimited, "profilerSessionJSONMarkerOffset")) != 0 {
		t.Fatalf("Session extraction lost its single-FD fixed-size scanner: open=%v marker=%v sections=%v scans=%v",
			sessionOpen, sessionMarker, sessionSections, sessionScans)
	}
	sessionFailClose := callSites(sessionLimited, "failCloseAllRows")
	if len(sessionFailClose) != 1 {
		t.Fatalf("Session size gate fail-close sites=%d want=1", len(sessionFailClose))
	}
	sessionStop := token.NoPos
	ast.Inspect(sessionLimited.Body, func(node ast.Node) bool {
		ifStatement, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		oversized := false
		ast.Inspect(ifStatement.Cond, func(child ast.Node) bool {
			selector, ok := child.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "Oversized" {
				oversized = true
			}
			return true
		})
		if !oversized {
			return true
		}
		ast.Inspect(ifStatement.Body, func(child ast.Node) bool {
			result, ok := child.(*ast.ReturnStmt)
			if ok && len(result.Results) > 0 && isIdent(result.Results[0], "false") {
				sessionStop = result.Pos()
			}
			return true
		})
		return true
	})
	if sessionStop == token.NoPos || sessionFailClose[0].Pos() >= sessionStop {
		t.Fatalf("Session failCloseAllRows no longer dominates suffix stop: fail_close=%v stop=%d",
			sessionFailClose, sessionStop)
	}

	traceLimited := functions["extractProfilerTraceFileAtWithFrameLimit"]
	maxParameter := ""
	if fields := traceLimited.Type.Params.List; len(fields) > 0 {
		last := fields[len(fields)-1]
		if len(last.Names) == 1 {
			maxParameter = last.Names[0].Name
		}
	}
	if maxParameter == "" {
		t.Fatal("bounded frame reader lost its named frame-limit parameter")
	}
	makeCalls := callSites(traceLimited, "make")
	if len(makeCalls) != 1 {
		t.Fatalf("bounded frame reader allocation sites=%d want=1", len(makeCalls))
	}
	allocationPos := makeCalls[0].Pos()
	gatePos := token.NoPos
	gateEnd := token.NoPos
	ast.Inspect(traceLimited.Body, func(node ast.Node) bool {
		ifStatement, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		hasN, hasMax := false, false
		ast.Inspect(ifStatement.Cond, func(child ast.Node) bool {
			if ident, ok := child.(*ast.Ident); ok {
				hasN = hasN || ident.Name == "n"
				hasMax = hasMax || ident.Name == maxParameter
			}
			return true
		})
		if !hasN || !hasMax {
			return true
		}
		breaks := 0
		failClose := 0
		breakPos := token.NoPos
		failClosePos := token.NoPos
		offMutation := false
		ast.Inspect(ifStatement.Body, func(child ast.Node) bool {
			switch typed := child.(type) {
			case *ast.BranchStmt:
				if typed.Tok == token.BREAK {
					breaks++
					breakPos = typed.Pos()
				}
			case *ast.AssignStmt:
				for _, lhs := range typed.Lhs {
					offMutation = offMutation || isIdent(lhs, "off")
				}
			case *ast.CallExpr:
				if selector, ok := typed.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "failCloseAllRows" {
					failClose++
					failClosePos = typed.Pos()
				}
			}
			return true
		})
		if breaks == 1 && failClose == 1 && failClosePos < breakPos && !offMutation {
			gatePos, gateEnd = ifStatement.Pos(), ifStatement.End()
		}
		return true
	})
	if gatePos == token.NoPos || gateEnd >= allocationPos {
		t.Fatalf("frame size gate does not dominate allocation: gate=%d..%d allocation=%d", gatePos, gateEnd, allocationPos)
	}
	bodyReadPositions := []token.Pos{}
	for _, call := range callSites(traceLimited, "ReadAt") {
		if len(call.Args) != 2 {
			continue
		}
		binary, ok := call.Args[1].(*ast.BinaryExpr)
		if !ok || binary.Op != token.ADD || !isIdent(binary.X, "off") {
			continue
		}
		literal, ok := binary.Y.(*ast.BasicLit)
		if ok && literal.Kind == token.INT && literal.Value == "4" {
			bodyReadPositions = append(bodyReadPositions, call.Pos())
		}
	}
	if len(bodyReadPositions) != 1 || gateEnd >= bodyReadPositions[0] {
		t.Fatalf("frame size gate does not dominate the unique frame-body ReadAt: gate=%d..%d body_reads=%v",
			gatePos, gateEnd, bodyReadPositions)
	}

	publisher := functions["tryConvertProfilerContainerWithLedger"]
	extractCalls := callSites(publisher, "extractProfilerContainerSystraceRowsWithSessionLimit")
	openCalls := callSites(publisher, "OpenFile")
	writeCalls := callSites(publisher, "writeTo")
	if len(extractCalls) != 1 || len(openCalls) != 1 || len(writeCalls) != 1 ||
		!(extractCalls[0].Pos() < openCalls[0].Pos() && openCalls[0].Pos() < writeCalls[0].Pos()) {
		t.Fatalf("profiler first publication is no longer dominated by full extraction: extract=%v open=%v write=%v",
			extractCalls, openCalls, writeCalls)
	}
	if len(extractCalls[0].Args) != 5 || !isIdent(extractCalls[0].Args[2], "inputSize") ||
		!isIdent(extractCalls[0].Args[3], "sessionBodySize") {
		t.Fatalf("profiler route lost distinct full TraceFile and Session-only input bounds: %+v", extractCalls[0].Args)
	}
	writeOwners := map[string]int{}
	for functionName, function := range functions {
		if calls := callSites(function, "writeTo"); len(calls) > 0 {
			writeOwners[functionName] = len(calls)
		}
	}
	if len(writeOwners) != 1 || writeOwners["tryConvertProfilerContainerWithLedger"] != 1 {
		t.Fatalf("profiler trace-body publication authority is no longer unique: %+v", writeOwners)
	}
	for _, functionName := range []string{"extractProfilerTraceFileAtWithFrameLimit", "extractProfilerSessionPackageWithLineLimit"} {
		if calls := callSites(functions[functionName], "writeTo"); len(calls) != 0 {
			t.Fatalf("resource extractor %s acquired publication authority: %+v", functionName, calls)
		}
		if calls := callSites(functions[functionName], "failCloseProfilerTraceBody"); len(calls) != 1 {
			t.Fatalf("resource extractor %s fail-close reconciliation sites=%d want=1", functionName, len(calls))
		}
	}
}
