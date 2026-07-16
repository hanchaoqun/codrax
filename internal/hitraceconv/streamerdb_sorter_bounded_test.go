package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unsafe"
)

// prepareAndWriteForTest mirrors the production preflight contract for tests
// that do not own an output-path caller. Tests for unprepared publication call
// writeTo directly and therefore cannot accidentally weaken that negative pin.
func (s *traceDBRowSink) prepareAndWriteForTest(ctx context.Context, writer io.Writer) (traceDBRowSortStats, error) {
	if err := s.prepareForPublication(ctx); err != nil {
		return s.stats, err
	}
	return s.writeTo(ctx, writer)
}

func traceDBBoundedTestOptions(bufferBytes, maximumRunRowBytes uint64, fanIn int) traceDBRowSinkOptions {
	active := uint64(64 << 20)
	if maximumRunRowBytes > active {
		active = maximumRunRowBytes
	}
	return traceDBRowSinkOptions{
		bufferBytes:    bufferBytes,
		maxRunRowBytes: maximumRunRowBytes,
		mergeFanIn:     fanIn,
		activeTempCap:  active,
		liveTempCap:    active * 2,
	}
}

func traceDBBoundedPhysicalRowSize(t *testing.T, row renderedRow, ingestOrdinal uint64) uint64 {
	t.Helper()
	raw, err := json.Marshal(traceDBChunkRowFor(traceDBBufferedRunRow{
		row: compactTraceDBStoredRow(row), ingestOrdinal: ingestOrdinal,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return uint64(len(raw) + 1)
}

func traceDBBoundedInvariantReason(t *testing.T, err error) string {
	t.Helper()
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok {
		t.Fatalf("error is not a typed trace DB invariant: %T %v", err, err)
	}
	return reason
}

func assertTraceDBBoundedFailureDisclosure(t *testing.T, sink *traceDBRowSink, reason string) {
	t.Helper()
	if sink.stats.FailureReason != reason || sink.stats.coverage().Error != reason {
		t.Fatalf("sorter failure disclosure=%q/%q want=%q",
			sink.stats.FailureReason, sink.stats.coverage().Error, reason)
	}
}

func assertTraceDBBoundedArtifactsRegistered(t *testing.T, sink *traceDBRowSink, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "rows-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		artifact := sink.artifacts[path]
		if artifact == nil || artifact.removed {
			t.Fatalf("physical run is not live in the cleanup registry: path=%q artifact=%+v", path, artifact)
		}
	}
	for path, artifact := range sink.artifacts {
		if artifact == nil {
			t.Fatalf("nil cleanup artifact for %q", path)
		}
		_, statErr := os.Stat(path)
		if artifact.removed && !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("retired artifact still exists: path=%q stat=%v", path, statErr)
		}
		if !artifact.removed && statErr != nil {
			t.Fatalf("live artifact is missing: path=%q stat=%v", path, statErr)
		}
	}
}

func assertTraceDBBoundedNoRunFiles(t *testing.T, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "rows-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Fatalf("orphan run files remain: %v", paths)
	}
}

func TestTraceDBRowSorterRetainedByteGateBoundaries(t *testing.T) {
	const bufferCap = uint64(512)
	for _, target := range []uint64{bufferCap - 1, bufferCap, bufferCap + 1} {
		t.Run(fmt.Sprintf("retained-%d", target), func(t *testing.T) {
			lineBytes := target - traceDBBufferedRowMetadataBytes
			row := renderedRow{tsNS: 1, seq: 1, line: strings.Repeat("x", int(lineBytes))}
			sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), math.MaxInt,
				traceDBBoundedTestOptions(bufferCap, 2<<20, 4))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sink.cleanup() }()
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
			if sink.stats.PeakBufferedBytes != target {
				t.Fatalf("peak buffered bytes=%d want=%d", sink.stats.PeakBufferedBytes, target)
			}
			if target < bufferCap && (len(sink.rows) != 1 || len(sink.runs) != 0) {
				t.Fatalf("cap-1 spilled early: rows=%d runs=%d", len(sink.rows), len(sink.runs))
			}
			if target >= bufferCap && (len(sink.rows) != 0 || len(sink.runs) != 1) {
				t.Fatalf("cap/cap+1 did not form one atomic run: rows=%d runs=%d", len(sink.rows), len(sink.runs))
			}
			var output bytes.Buffer
			stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
			if err != nil {
				t.Fatal(err)
			}
			if stats.RowsAccepted != 1 || stats.RowsWritten != 1 || stats.RowsWithheld != 0 ||
				!strings.Contains(output.String(), row.line) {
				t.Fatalf("boundary row did not publish: stats=%+v bytes=%d", stats, output.Len())
			}
		})
	}

	t.Run("append preflush counts metadata before mutation", func(t *testing.T) {
		const capBytes = uint64(300)
		first := renderedRow{tsNS: 1, seq: 1,
			line: strings.Repeat("a", int(capBytes-traceDBBufferedRowMetadataBytes-1))}
		second := renderedRow{tsNS: 2, seq: 2, line: "b"}
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), math.MaxInt,
			traceDBBoundedTestOptions(capBytes, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		if err := sink.add(first); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(second); err != nil {
			t.Fatal(err)
		}
		secondBytes, _ := traceDBStoredRowRetainedBytes(compactTraceDBStoredRow(second))
		if len(sink.runs) != 1 || len(sink.rows) != 1 || sink.rows[0].line != "b" ||
			sink.bufferedBytes != secondBytes || sink.stats.PeakBufferedBytes != capBytes-1 {
			t.Fatalf("append gate was applied after mutation: runs=%d rows=%d buffered=%d peak=%d",
				len(sink.runs), len(sink.rows), sink.bufferedBytes, sink.stats.PeakBufferedBytes)
		}
	})

	t.Run("metadata participates in byte gate", func(t *testing.T) {
		row := renderedRow{
			tsNS: 1, seq: 1,
			line: strings.Repeat("x", int(bufferCap-traceDBBufferedRowMetadataBytes)),
		}
		retained, ok := traceDBStoredRowRetainedBytes(compactTraceDBStoredRow(row))
		if !ok || retained != bufferCap {
			t.Fatalf("fixture retained bytes=%d ok=%t want=%d", retained, ok, bufferCap)
		}
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), math.MaxInt,
			traceDBBoundedTestOptions(bufferCap, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
		if len(sink.runs) != 1 || sink.stats.PeakBufferedBytes != bufferCap {
			t.Fatalf("metadata bytes did not trigger exact cap: runs=%d stats=%+v", len(sink.runs), sink.stats)
		}
	})

	t.Run("slice spare capacity remains inside fixed metadata charge", func(t *testing.T) {
		if traceDBBufferedRowMetadataBytes != 256 {
			t.Fatalf("fixed metadata charge=%d want=256", traceDBBufferedRowMetadataBytes)
		}
		row := renderedRow{tsNS: 1, seq: 1, line: "x"}
		rowBytes, ok := traceDBStoredRowRetainedBytes(compactTraceDBStoredRow(row))
		if !ok {
			t.Fatal("retained byte fixture overflowed")
		}
		// A sink-local equivalent of the 64MiB boundary uses the 65th-row
		// growth edge that made a 128-byte charge unsafe. The conservative
		// 256-byte charge must cover both backing-slice capacities plus payload.
		const cohortRows = 65
		capBytes := rowBytes*cohortRows + 1
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), math.MaxInt,
			traceDBBoundedTestOptions(capBytes, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		for index := 0; index < cohortRows; index++ {
			row.tsNS, row.seq = uint64(index), index
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		if cap(sink.rows) <= len(sink.rows) || cap(sink.rowIngestOrdinals) <= len(sink.rowIngestOrdinals) {
			t.Fatalf("fixture did not exercise spare slice capacity: rows=%d/%d ordinals=%d/%d",
				len(sink.rows), cap(sink.rows), len(sink.rowIngestOrdinals), cap(sink.rowIngestOrdinals))
		}
		physicalEstimate := uint64(cap(sink.rows))*uint64(unsafe.Sizeof(traceDBStoredRow{})) +
			uint64(cap(sink.rowIngestOrdinals))*uint64(unsafe.Sizeof(uint64(0))) +
			uint64(unsafe.Sizeof(sink.rows)+unsafe.Sizeof(sink.rowIngestOrdinals)) + cohortRows
		if sink.bufferedBytes != rowBytes*cohortRows || sink.bufferedBytes > capBytes ||
			sink.bufferedBytes < physicalEstimate {
			t.Fatalf("logical/fixed charge drifted: buffered=%d cap=%d row=%d", sink.bufferedBytes, capBytes, rowBytes)
		}
		row.tsNS, row.seq = cohortRows, cohortRows
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
		if len(sink.runs) != 1 || len(sink.rows) != 1 || sink.stats.PeakBufferedBytes > capBytes {
			t.Fatalf("near-cap cohort crossed byte gate: runs=%d rows=%d stats=%+v", len(sink.runs), len(sink.rows), sink.stats)
		}
	})

	t.Run("controlled capacity growth proves retained metadata bound", func(t *testing.T) {
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), math.MaxInt,
			traceDBBoundedTestOptions(1<<20, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		growth := map[int]int{}
		previousCapacity := 0
		for index := 0; index < 130; index++ {
			if err := sink.add(renderedRow{tsNS: uint64(index), seq: index, line: "x"}); err != nil {
				t.Fatal(err)
			}
			length := len(sink.rows)
			capacity := cap(sink.rows)
			if capacity != cap(sink.rowIngestOrdinals) || length != len(sink.rowIngestOrdinals) ||
				capacity >= 2*length {
				t.Fatalf("controlled capacity invariant drifted: rows=%d/%d ordinals=%d/%d",
					length, capacity, len(sink.rowIngestOrdinals), cap(sink.rowIngestOrdinals))
			}
			if capacity != previousCapacity {
				growth[length] = capacity
				previousCapacity = capacity
			}
			metadataBytes, ok := traceDBBufferedCapacityBytes(capacity)
			if !ok || sink.bufferedBytes < metadataBytes+uint64(length) {
				t.Fatalf("capacity escaped retained charge: len=%d cap=%d buffered=%d metadata=%d ok=%t",
					length, capacity, sink.bufferedBytes, metadataBytes, ok)
			}
		}
		for length, capacity := range map[int]int{1: 1, 2: 2, 3: 4, 5: 8, 9: 16, 17: 32, 33: 64, 65: 128, 129: 256} {
			if growth[length] != capacity {
				t.Fatalf("capacity growth at len=%d got=%d want=%d; all=%+v", length, growth[length], capacity, growth)
			}
		}
	})

	t.Run("single maximum line forms one-row run", func(t *testing.T) {
		row := renderedRow{tsNS: 1, seq: 1, line: strings.Repeat("m", maxTraceDBSystraceLineBytes)}
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), math.MaxInt,
			traceDBBoundedTestOptions(4<<10, 8<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
		if len(sink.runs) != 1 || sink.stats.PeakBufferedRows != 1 ||
			sink.stats.PeakBufferedBytes != uint64(maxTraceDBSystraceLineBytes)+traceDBBufferedRowMetadataBytes {
			t.Fatalf("maximum line did not remain a one-row run: runs=%d stats=%+v", len(sink.runs), sink.stats)
		}
		var output bytes.Buffer
		stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
		if err != nil {
			t.Fatal(err)
		}
		if stats.RowsWritten != 1 || !bytes.Contains(output.Bytes(), []byte(row.line)) {
			t.Fatalf("maximum legal line did not publish: stats=%+v bytes=%d", stats, output.Len())
		}
	})

	t.Run("checked overflow precedes run creation", func(t *testing.T) {
		dir := t.TempDir()
		sink, err := newTraceDBRowSinkWithOptions(dir, math.MaxInt,
			traceDBBoundedTestOptions(bufferCap, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		sink.bufferedBytes = math.MaxUint64
		err = sink.add(renderedRow{tsNS: 1, seq: 1, line: "overflow"})
		if reason := traceDBBoundedInvariantReason(t, err); reason != "trace_row_sort_buffer_accounting_overflow" {
			t.Fatalf("overflow reason=%q", reason)
		}
		assertTraceDBBoundedFailureDisclosure(t, sink, "trace_row_sort_buffer_accounting_overflow")
		if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.runs) != 0 {
			t.Fatalf("overflow mutated sink: stats=%+v rows=%d runs=%d", sink.stats, len(sink.rows), len(sink.runs))
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	t.Run("small substrings do not retain a giant parent", func(t *testing.T) {
		parent := strings.Repeat("p", 2<<20)
		line := parent[17:49]
		lane := parent[1<<20 : (1<<20)+32]
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 100,
			traceDBBoundedTestOptions(8<<20, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		if err := sink.add(renderedRow{
			tsNS: 1, seq: 1, line: line, pairKind: pairRenderF2FS,
			pairLane: lane, pairTable: "f2fs_write_begin",
		}); err != nil {
			t.Fatal(err)
		}
		stored := sink.rows[0]
		canonicalLane, ok := sink.pairLaneRegistries[pairRenderF2FS].key(stored.provenance.LaneID)
		if !ok {
			t.Fatalf("stored lane provenance is not registered: %+v", stored.provenance)
		}
		if unsafe.StringData(stored.line) == unsafe.StringData(line) ||
			unsafe.StringData(canonicalLane) == unsafe.StringData(lane) {
			t.Fatalf("sink retained parent-backed substring: stored=%+v", stored)
		}
		if stored.line != line || canonicalLane != lane ||
			stored.provenance.EndpointSlot != profilerPairEndpointF2FSWriteBegin {
			t.Fatalf("cloning changed row bytes: stored=%+v", stored)
		}
		retained, ok := traceDBStoredRowRetainedBytes(stored)
		if !ok || sink.bufferedBytes != retained {
			t.Fatalf("cloned row byte accounting drifted: buffered=%d retained=%d ok=%t", sink.bufferedBytes, retained, ok)
		}
	})
}

func TestTraceDBRowSorterDuplicateTimestampSequenceStableAcrossLevels(t *testing.T) {
	rows := make([]renderedRow, 20)
	for index := range rows {
		rows[index] = renderedRow{tsNS: 7, seq: 9, line: fmt.Sprintf("stable-duplicate-%02d", index)}
	}
	tests := []struct {
		name       string
		threshold  int
		wantPasses int
	}{
		{name: "single leaf", threshold: 100, wantPasses: 0},
		{name: "leaf merge", threshold: 4, wantPasses: 2},
		{name: "multi-level", threshold: 1, wantPasses: 3},
	}
	var baseline []byte
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), test.threshold,
				traceDBBoundedTestOptions(8<<20, 2<<20, 3))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sink.cleanup() }()
			for _, row := range rows {
				if err := sink.add(row); err != nil {
					t.Fatal(err)
				}
			}
			var output bytes.Buffer
			stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
			if err != nil {
				t.Fatal(err)
			}
			if stats.RowsWritten != len(rows) || stats.RowsWithheld != 0 || stats.MergePasses != test.wantPasses {
				t.Fatalf("stable fixture stats=%+v wantPasses=%d", stats, test.wantPasses)
			}
			if baseline == nil {
				baseline = append([]byte(nil), output.Bytes()...)
			} else if !bytes.Equal(output.Bytes(), baseline) {
				t.Fatalf("same ts+seq output drifted across levels\nbase=%q\ngot =%q", baseline, output.Bytes())
			}
			last := -1
			for index := range rows {
				position := bytes.Index(output.Bytes(), []byte(fmt.Sprintf("stable-duplicate-%02d", index)))
				if position <= last {
					t.Fatalf("ingest stability drifted at index=%d position=%d last=%d", index, position, last)
				}
				last = position
			}
		})
	}
}

func TestTraceDBRowSorterFanInBoundariesAndFDPeak(t *testing.T) {
	const fanIn = 4
	for _, test := range []struct {
		name       string
		runs       int
		wantPasses int
	}{
		{name: "fanIn-1", runs: fanIn - 1, wantPasses: 1},
		{name: "fanIn", runs: fanIn, wantPasses: 1},
		{name: "fanIn+1", runs: fanIn + 1, wantPasses: 2},
		{name: "greater-than-fanIn-squared", runs: fanIn*fanIn + 1, wantPasses: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1,
				traceDBBoundedTestOptions(8<<20, 2<<20, fanIn))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sink.cleanup() }()
			for index := 0; index < test.runs; index++ {
				row := renderedRow{tsNS: uint64(test.runs - index), seq: index, line: fmt.Sprintf("fan-row-%02d", index)}
				if err := sink.add(row); err != nil {
					t.Fatal(err)
				}
			}
			if sink.stats.SpillChunks != test.runs || len(sink.runs) != test.runs {
				t.Fatalf("one-row leaf fixture drifted: chunks=%d runs=%d want=%d", sink.stats.SpillChunks, len(sink.runs), test.runs)
			}
			if err := sink.prepareForPublication(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(sink.runs) != 1 || sink.runs[0].rowCount != uint64(test.runs) ||
				sink.stats.MergePasses != test.wantPasses || sink.stats.PeakOpenRunFDs > fanIn+1 || sink.openRunFDs != 0 {
				t.Fatalf("fan-in bound drifted: runs=%+v open=%d stats=%+v", sink.runs, sink.openRunFDs, sink.stats)
			}
			var output bytes.Buffer
			stats, err := sink.writeTo(context.Background(), &output)
			if err != nil {
				t.Fatal(err)
			}
			if stats.RowsWritten != test.runs {
				t.Fatalf("fan-in output rows=%d want=%d", stats.RowsWritten, test.runs)
			}
		})
	}
}

func TestTraceDBRowSorterCoverageReportsDistinctResourceMetrics(t *testing.T) {
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1,
		traceDBBoundedTestOptions(8<<20, 2<<20, 2))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.cleanup() }()
	for index := 0; index < 5; index++ {
		if err := sink.add(renderedRow{tsNS: uint64(index), seq: index, line: fmt.Sprintf("coverage-%d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	if sink.stats.ElapsedUS <= 0 {
		t.Fatalf("add-triggered spill elapsed timing missing: %+v", sink.stats)
	}
	if err := sink.prepareForPublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	preflightElapsed := sink.stats.ElapsedUS
	if preflightElapsed <= 0 {
		t.Fatalf("preflight elapsed timing missing: %+v", sink.stats)
	}
	finalSize := sink.runs[0].size
	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	coverage := stats.coverage()
	if coverage.PeakBufferedBytes != stats.PeakBufferedBytes ||
		coverage.CurrentLiveTempBytes != 0 || coverage.PeakLiveTempBytes != stats.PeakLiveTempBytes ||
		coverage.PeakOpenRunFDs != stats.PeakOpenRunFDs || coverage.MergePasses != stats.MergePasses {
		t.Fatalf("sorter resource metrics collapsed: stats=%+v coverage=%+v", stats, coverage)
	}
	if stats.CurrentLiveTempBytes != 0 || stats.SpillChunks != 5 || stats.TempBytes <= int64(finalSize) {
		t.Fatalf("leaf/cumulative/live semantics drifted: final=%d stats=%+v", finalSize, stats)
	}
	if stats.ElapsedUS <= preflightElapsed {
		t.Fatalf("publication timing did not accumulate after preflight: preflight=%d final=%d", preflightElapsed, stats.ElapsedUS)
	}
	if coverage.FieldSources["row_buffer_limits"] != fmt.Sprintf("%d_bytes+%d_rows", defaultTraceDBRowBufferBytes, defaultTraceDBRowSinkThreshold) ||
		coverage.FieldSources["merge_limits"] != fmt.Sprintf("%d_input_runs+%d_total_run_fds", defaultTraceDBRowMergeFanIn, defaultTraceDBRowMergeFanIn+1) ||
		coverage.FieldSources["temp_limits"] != fmt.Sprintf("%d_active_bytes+%d_live_bytes", defaultTraceDBActiveTempBytes, defaultTraceDBLiveTempBytes) {
		t.Fatalf("coverage constants drifted from product constants: %+v", coverage.FieldSources)
	}
}

func TestTraceDBRowSorterCleanupCoverageZerosLiveBytesAndPreservesTypedFailure(t *testing.T) {
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1,
		traceDBBoundedTestOptions(8<<20, 2<<20, 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "live-run-before-preflight-failure"}); err != nil {
		t.Fatal(err)
	}
	const reason = "trace_row_sort_run_read_failed"
	sink.recordSorterFailure(&traceDBOutputInvariantError{Reason: reason})
	before := sink.stats.coverage()
	if before.CurrentLiveTempBytes == 0 || before.Error != reason {
		t.Fatalf("fixture did not expose live typed failure coverage: %+v", before)
	}
	if err := sink.cleanup(); err != nil {
		t.Fatal(err)
	}
	after := sink.stats.coverage()
	if after.CurrentLiveTempBytes != 0 || after.Error != reason {
		t.Fatalf("cleanup coverage lost resource/failure truth: before=%+v after=%+v", before, after)
	}
}

type traceDBBoundedWriteCounter struct {
	calls int
	bytes int
}

type traceDBBoundedFailingWriter struct{ err error }

func (writer traceDBBoundedFailingWriter) Write([]byte) (int, error) { return 0, writer.err }

func (counter *traceDBBoundedWriteCounter) Write(data []byte) (int, error) {
	counter.calls++
	counter.bytes += len(data)
	return len(data), nil
}

func TestTraceDBRowSorterRequiresPreflightAndFreezesMutation(t *testing.T) {
	t.Run("elapsed overflow fails preflight before publication", func(t *testing.T) {
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 100,
			traceDBBoundedTestOptions(8<<20, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "elapsed-overflow"}); err != nil {
			t.Fatal(err)
		}
		sink.stats.ElapsedUS = math.MaxInt64
		err = sink.prepareForPublication(context.Background())
		if reason := traceDBBoundedInvariantReason(t, err); reason != "trace_row_sort_elapsed_overflow" {
			t.Fatalf("elapsed overflow reason=%q", reason)
		}
		if reason := traceDBBoundedInvariantReason(t, sink.prepareFailure); reason != "trace_row_sort_elapsed_overflow" {
			t.Fatalf("elapsed overflow was not frozen as preflight failure: %q", reason)
		}
		assertTraceDBBoundedFailureDisclosure(t, sink, "trace_row_sort_elapsed_overflow")
		counter := &traceDBBoundedWriteCounter{}
		if _, err := sink.writeTo(context.Background(), counter); traceDBBoundedInvariantReason(t, err) != "trace_row_sort_write_before_prepare" {
			t.Fatalf("overflowed preflight allowed publication: %v", err)
		}
		if counter.calls != 0 || counter.bytes != 0 {
			t.Fatalf("overflowed preflight reached destination: calls=%d bytes=%d", counter.calls, counter.bytes)
		}
	})

	t.Run("unprepared write consumes nothing and remains recoverable", func(t *testing.T) {
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 100,
			traceDBBoundedTestOptions(8<<20, 2<<20, 4))
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "unprepared"}); err != nil {
			t.Fatal(err)
		}
		counter := &traceDBBoundedWriteCounter{}
		if _, err := sink.writeTo(context.Background(), counter); traceDBBoundedInvariantReason(t, err) != "trace_row_sort_write_before_prepare" {
			t.Fatalf("unprepared write reason drifted: %v", err)
		}
		if counter.calls != 0 || counter.bytes != 0 {
			t.Fatalf("unprepared write reached destination: calls=%d bytes=%d", counter.calls, counter.bytes)
		}
		if sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 || len(sink.runs) != 0 {
			t.Fatalf("unprepared write consumed state: stats=%+v rows=%d runs=%d", sink.stats, len(sink.rows), len(sink.runs))
		}
		var output bytes.Buffer
		stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
		if err != nil {
			t.Fatal(err)
		}
		if stats.RowsWritten != 1 || !strings.Contains(output.String(), "unprepared") {
			t.Fatalf("sink did not recover after precondition rejection: stats=%+v\n%s", stats, output.String())
		}
	})

	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 100,
		traceDBBoundedTestOptions(8<<20, 2<<20, 4))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.cleanup() }()
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "before-prepare"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.prepareForPublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !sink.prepared || len(sink.runs) != 1 {
		t.Fatalf("preflight did not freeze one final run: prepared=%t runs=%d", sink.prepared, len(sink.runs))
	}
	manifest := sink.runs[0]
	if err := sink.prepareForPublication(context.Background()); err != nil {
		t.Fatalf("idempotent preflight failed: %v", err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 2, line: "after-prepare"}); traceDBBoundedInvariantReason(t, err) != "trace_row_sink_add_after_prepare" {
		t.Fatalf("post-prepare add reason drifted: %v", err)
	}
	sink.poisonPairKind(pairRenderMMC)
	sink.markPairCaptureOpaque(pairRenderF2FS)
	if len(sink.runs) != 1 || sink.runs[0] != manifest ||
		sink.poisoned[pairRenderMMC] || sink.opaque[pairRenderF2FS] {
		t.Fatalf("post-prepare mutation changed frozen authority: runs=%+v poison=%v opaque=%v",
			sink.runs, sink.poisoned, sink.opaque)
	}
	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsWritten != 1 || !strings.Contains(output.String(), "before-prepare") ||
		strings.Contains(output.String(), "after-prepare") {
		t.Fatalf("frozen publication drifted: stats=%+v\n%s", stats, output.String())
	}
}

func TestTraceDBRowSorterActiveAndLiveTempBoundaries(t *testing.T) {
	activeRows := []renderedRow{
		{tsNS: 1, seq: 1, line: "active-cap-a"},
		{tsNS: 2, seq: 2, line: "active-cap-b"},
	}
	activeSizes := []uint64{
		traceDBBoundedPhysicalRowSize(t, activeRows[0], 0),
		traceDBBoundedPhysicalRowSize(t, activeRows[1], 1),
	}
	activeTotal := activeSizes[0] + activeSizes[1]
	maximumRow := max(activeSizes[0], activeSizes[1])

	t.Run("active exact cap and pending live exact cap", func(t *testing.T) {
		options := traceDBRowSinkOptions{
			bufferBytes: 1, maxRunRowBytes: maximumRow, mergeFanIn: 2,
			activeTempCap: activeTotal, liveTempCap: activeTotal * 2,
		}
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1, options)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		for _, row := range activeRows {
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		if sink.activeTempBytes != activeTotal || sink.liveTempBytes != activeTotal {
			t.Fatalf("leaf exact cap drifted: active=%d live=%d want=%d", sink.activeTempBytes, sink.liveTempBytes, activeTotal)
		}
		if err := sink.prepareForPublication(context.Background()); err != nil {
			t.Fatal(err)
		}
		if sink.activeTempBytes != activeTotal || sink.liveTempBytes != activeTotal ||
			sink.stats.CurrentLiveTempBytes != activeTotal || sink.stats.PeakLiveTempBytes != activeTotal*2 ||
			len(sink.runs) != 1 {
			t.Fatalf("exact merge temp accounting drifted: active=%d live=%d runs=%d stats=%+v",
				sink.activeTempBytes, sink.liveTempBytes, len(sink.runs), sink.stats)
		}
		var output bytes.Buffer
		stats, err := sink.writeTo(context.Background(), &output)
		if err != nil {
			t.Fatal(err)
		}
		if stats.RowsWritten != len(activeRows) {
			t.Fatalf("exact-cap rows written=%d want=%d", stats.RowsWritten, len(activeRows))
		}
	})

	t.Run("active cap plus one fails before second leaf create", func(t *testing.T) {
		created := 0
		activeCap := activeTotal - 1
		options := traceDBRowSinkOptions{
			bufferBytes: 1, maxRunRowBytes: maximumRow, mergeFanIn: 2,
			activeTempCap: activeCap, liveTempCap: activeCap * 2,
			ops: traceDBRowSinkOps{createTemp: func(dir, pattern string) (*os.File, error) {
				created++
				return os.CreateTemp(dir, pattern)
			}},
		}
		dir := t.TempDir()
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.add(activeRows[0]); err != nil {
			t.Fatal(err)
		}
		err = sink.add(activeRows[1])
		if reason := traceDBBoundedInvariantReason(t, err); reason != "trace_row_sort_active_temp_budget_exceeded" {
			t.Fatalf("active +1 reason=%q", reason)
		}
		assertTraceDBBoundedFailureDisclosure(t, sink, "trace_row_sort_active_temp_budget_exceeded")
		if created != 1 || len(sink.runs) != 1 || sink.activeTempBytes != activeSizes[0] ||
			sink.liveTempBytes != activeSizes[0] || sink.openRunFDs != 0 {
			t.Fatalf("active +1 created a partial leaf: created=%d runs=%d active=%d live=%d open=%d",
				created, len(sink.runs), sink.activeTempBytes, sink.liveTempBytes, sink.openRunFDs)
		}
		assertTraceDBBoundedArtifactsRegistered(t, sink, dir)
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	liveRows := make([]renderedRow, 5)
	var liveTotal uint64
	var liveMaximum uint64
	for index := range liveRows {
		liveRows[index] = renderedRow{tsNS: uint64(10 - index), seq: index, line: fmt.Sprintf("live-level-%d", index)}
		size := traceDBBoundedPhysicalRowSize(t, liveRows[index], uint64(index))
		liveTotal += size
		liveMaximum = max(liveMaximum, size)
	}
	for _, test := range []struct {
		name      string
		cap       uint64
		wantError bool
	}{
		{name: "multi-level pending output exact", cap: liveTotal * 2},
		{name: "multi-level pending output plus one", cap: liveTotal*2 - 1, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := traceDBRowSinkOptions{
				bufferBytes: 1, maxRunRowBytes: liveMaximum, mergeFanIn: 2,
				activeTempCap: liveTotal, liveTempCap: liveTotal * 2,
			}
			dir := t.TempDir()
			sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sink.cleanup() }()
			for _, row := range liveRows {
				if err := sink.add(row); err != nil {
					t.Fatal(err)
				}
			}
			// The product constructor deliberately requires live >= 2*active.
			// This sink-local fault fixture narrows only the already-constructed
			// instance to make the otherwise redundant +1 defense reachable.
			sink.options.liveTempCap = test.cap
			err = sink.prepareForPublication(context.Background())
			if test.wantError {
				if reason := traceDBBoundedInvariantReason(t, err); reason != "trace_row_sort_live_temp_budget_exceeded" {
					t.Fatalf("live +1 reason=%q", reason)
				}
				if sink.prepared || sink.openRunFDs != 0 || sink.liveTempBytes != liveTotal ||
					sink.activeTempBytes != liveTotal || sink.stats.FailureReason != "trace_row_sort_live_temp_budget_exceeded" {
					t.Fatalf("live +1 did not fail atomically: prepared=%t open=%d active=%d live=%d stats=%+v",
						sink.prepared, sink.openRunFDs, sink.activeTempBytes, sink.liveTempBytes, sink.stats)
				}
				assertTraceDBBoundedArtifactsRegistered(t, sink, dir)
				if err := sink.cleanup(); err != nil {
					t.Fatal(err)
				}
				assertTraceDBBoundedNoRunFiles(t, dir)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if sink.stats.PeakLiveTempBytes != liveTotal*2 || sink.stats.MergePasses != 3 ||
				sink.liveTempBytes != liveTotal || sink.activeTempBytes != liveTotal || len(sink.runs) != 1 {
				t.Fatalf("multi-level exact live accounting drifted: active=%d live=%d runs=%d stats=%+v",
					sink.activeTempBytes, sink.liveTempBytes, len(sink.runs), sink.stats)
			}
		})
	}
}

func TestTraceDBRowSorterContextAndFaultCleanup(t *testing.T) {
	t.Run("single publication error preserves concrete identity", func(t *testing.T) {
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1,
			traceDBBoundedTestOptions(8<<20, 2<<20, 2))
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "writer-identity"}); err != nil {
			t.Fatal(err)
		}
		if err := sink.prepareForPublication(context.Background()); err != nil {
			t.Fatal(err)
		}
		writeFault := errors.New("single-writer-fault")
		if _, err := sink.writeTo(context.Background(), traceDBBoundedFailingWriter{err: writeFault}); err != writeFault {
			t.Fatalf("single writer error identity changed: got=%T %v want=%p", err, err, writeFault)
		}
	})

	t.Run("canceled before preflight creates no run", func(t *testing.T) {
		dir := t.TempDir()
		sink, err := newTraceDBRowSinkWithOptions(dir, 100,
			traceDBBoundedTestOptions(8<<20, 2<<20, 3))
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "cancel-before"}); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := sink.prepareForPublication(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("preflight cancellation=%v want context.Canceled", err)
		}
		if sink.prepared || len(sink.runs) != 0 || sink.openRunFDs != 0 {
			t.Fatalf("canceled preflight mutated run authority: prepared=%t runs=%d open=%d",
				sink.prepared, len(sink.runs), sink.openRunFDs)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	t.Run("bound context cancellation reaches add-triggered flush", func(t *testing.T) {
		dir := t.TempDir()
		sink, err := newTraceDBRowSinkWithOptions(dir, 1,
			traceDBBoundedTestOptions(8<<20, 2<<20, 3))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		if err := sink.bindContext(ctx); err != nil {
			t.Fatal(err)
		}
		cancel()
		err = sink.add(renderedRow{tsNS: 1, seq: 1, line: "add-cancel"})
		if !errors.Is(err, context.Canceled) || len(sink.runs) != 0 || sink.openRunFDs != 0 {
			t.Fatalf("add-triggered flush ignored bound cancellation: err=%v runs=%d open=%d",
				err, len(sink.runs), sink.openRunFDs)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	t.Run("canceled during merge closes readers and leaves registered inputs", func(t *testing.T) {
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		triggered := false
		options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
		options.ops.fault = func(point, _ string) error {
			if point == "read" && !triggered {
				triggered = true
				cancel()
			}
			return nil
		}
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < 4; index++ {
			if err := sink.add(renderedRow{tsNS: uint64(index), seq: index, line: fmt.Sprintf("cancel-%d", index)}); err != nil {
				t.Fatal(err)
			}
		}
		if err := sink.prepareForPublication(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("merge cancellation=%v want context.Canceled", err)
		}
		if !triggered || sink.prepared || sink.openRunFDs != 0 {
			t.Fatalf("merge cancellation leaked state: triggered=%t prepared=%t open=%d", triggered, sink.prepared, sink.openRunFDs)
		}
		assertTraceDBBoundedArtifactsRegistered(t, sink, dir)
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	for _, point := range []string{"create", "write", "flush", "close", "stat"} {
		t.Run("leaf-"+point, func(t *testing.T) {
			dir := t.TempDir()
			fault := errors.New("leaf-" + point)
			triggered := false
			options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
			options.ops.fault = func(got, _ string) error {
				if got == point && !triggered {
					triggered = true
					return fault
				}
				return nil
			}
			sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
			if err != nil {
				t.Fatal(err)
			}
			err = sink.add(renderedRow{tsNS: 1, seq: 1, line: "leaf-fault"})
			if !triggered || !errors.Is(err, fault) {
				t.Fatalf("leaf fault point=%s triggered=%t err=%v", point, triggered, err)
			}
			if sink.openRunFDs != 0 || sink.liveTempBytes != 0 || sink.activeTempBytes != 0 {
				t.Fatalf("leaf fault leaked resources: open=%d live=%d active=%d", sink.openRunFDs, sink.liveTempBytes, sink.activeTempBytes)
			}
			assertTraceDBBoundedNoRunFiles(t, dir)
			if err := sink.cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}

	mergeFaults := []struct {
		name         string
		point        string
		pathContains string
		storageInput bool
	}{
		{name: "create-output", point: "create"},
		{name: "open-input", point: "open", pathContains: "rows-leaf-", storageInput: true},
		{name: "stat-input", point: "stat", pathContains: "rows-leaf-", storageInput: true},
		{name: "read-input", point: "read", pathContains: "rows-leaf-", storageInput: true},
		{name: "decode-input", point: "decode", pathContains: "rows-leaf-", storageInput: true},
		{name: "close-input", point: "close", pathContains: "rows-leaf-", storageInput: true},
		{name: "write-output", point: "write", pathContains: "rows-merge-"},
		{name: "flush-output", point: "flush", pathContains: "rows-merge-"},
		{name: "close-output", point: "close", pathContains: "rows-merge-"},
		{name: "stat-output", point: "stat", pathContains: "rows-merge-"},
		{name: "remove-input", point: "remove", pathContains: "rows-leaf-"},
	}
	for _, test := range mergeFaults {
		t.Run("merge-"+test.name, func(t *testing.T) {
			dir := t.TempDir()
			fault := errors.New("merge-" + test.name)
			armed := false
			triggered := false
			options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
			options.ops.fault = func(point, path string) error {
				if armed && !triggered && point == test.point &&
					(test.pathContains == "" || strings.Contains(filepath.Base(path), test.pathContains)) {
					triggered = true
					return fault
				}
				return nil
			}
			sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
			if err != nil {
				t.Fatal(err)
			}
			for index := 0; index < 4; index++ {
				if err := sink.add(renderedRow{tsNS: uint64(index), seq: index, line: fmt.Sprintf("fault-%d", index)}); err != nil {
					t.Fatal(err)
				}
			}
			armed = true
			err = sink.prepareForPublication(context.Background())
			if !triggered {
				t.Fatalf("fault point did not trigger: %+v", test)
			}
			if test.storageInput {
				if reason := traceDBBoundedInvariantReason(t, err); reason != profilerPairStorageIntegrityFailure {
					t.Fatalf("registered input fault reason=%q", reason)
				}
				if sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed {
					t.Fatalf("registered input fault did not close source: source=%q all=%t",
						sink.captureSourceFailure, sink.allRowsFailClosed)
				}
			} else if !errors.Is(err, fault) {
				t.Fatalf("merge fault point=%s err=%v want=%v", test.name, err, fault)
			}
			if sink.prepared || sink.openRunFDs != 0 || sink.stats.PeakOpenRunFDs > sink.options.mergeFanIn+1 {
				t.Fatalf("merge fault leaked/fan-in drifted: prepared=%t open=%d peak=%d fan=%d",
					sink.prepared, sink.openRunFDs, sink.stats.PeakOpenRunFDs, sink.options.mergeFanIn)
			}
			assertTraceDBBoundedArtifactsRegistered(t, sink, dir)
			if err := sink.cleanup(); err != nil {
				t.Fatal(err)
			}
			assertTraceDBBoundedNoRunFiles(t, dir)
		})
	}

	t.Run("mixed output write and input close faults do not become source drift", func(t *testing.T) {
		dir := t.TempDir()
		closeFault := errors.New("final-input-close")
		writeFault := errors.New("user-output-write")
		armed := false
		triggered := false
		options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
		options.ops.fault = func(point, path string) error {
			if armed && !triggered && point == "close" && strings.Contains(filepath.Base(path), "rows-") {
				triggered = true
				return closeFault
			}
			return nil
		}
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: strings.Repeat("w", 300<<10)}); err != nil {
			t.Fatal(err)
		}
		if err := sink.prepareForPublication(context.Background()); err != nil {
			t.Fatal(err)
		}
		armed = true
		_, err = sink.writeTo(context.Background(), traceDBBoundedFailingWriter{err: writeFault})
		if !errors.Is(err, writeFault) || !errors.Is(err, closeFault) || !triggered {
			t.Fatalf("mixed output/input fault lost a branch: err=%v triggered=%t", err, triggered)
		}
		if sink.captureSourceFailure != "" || sink.allRowsFailClosed || sink.pairAuthorityFailure != "" {
			t.Fatalf("mixed caller/output failure was reclassified as source drift: source=%q all=%t authority=%q",
				sink.captureSourceFailure, sink.allRowsFailClosed, sink.pairAuthorityFailure)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	t.Run("nth input remove failure preserves committed output authority", func(t *testing.T) {
		dir := t.TempDir()
		removeFault := errors.New("second-input-remove")
		armed := false
		removeCalls := 0
		options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
		options.ops.fault = func(point, path string) error {
			if armed && point == "remove" && strings.Contains(filepath.Base(path), "rows-leaf-") {
				removeCalls++
				if removeCalls == 2 {
					return removeFault
				}
			}
			return nil
		}
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		leafPaths := make([]string, 0, 3)
		for index := 0; index < 3; index++ {
			if err := sink.add(renderedRow{tsNS: uint64(index), seq: index, line: fmt.Sprintf("commit-%d", index)}); err != nil {
				t.Fatal(err)
			}
			leafPaths = append(leafPaths, sink.runs[len(sink.runs)-1].path)
		}
		armed = true
		err = sink.prepareForPublication(context.Background())
		if !errors.Is(err, removeFault) || removeCalls != 2 || sink.prepared {
			t.Fatalf("nth retirement fault=%v calls=%d prepared=%t", err, removeCalls, sink.prepared)
		}
		if len(sink.runs) != 1 || !strings.Contains(filepath.Base(sink.runs[0].path), "rows-merge-") ||
			sink.runs[0].rowCount != 3 || sink.artifacts[sink.runs[0].path] == nil || sink.artifacts[sink.runs[0].path].removed {
			t.Fatalf("authenticated output was not retained as authority: runs=%+v artifacts=%+v", sink.runs, sink.artifacts)
		}
		if _, err := os.Stat(sink.runs[0].path); err != nil {
			t.Fatalf("committed output was discarded: %v", err)
		}
		if artifact := sink.artifacts[leafPaths[0]]; artifact == nil || !artifact.removed {
			t.Fatalf("first input retirement did not commit: %+v", artifact)
		}
		for _, path := range leafPaths[1:] {
			if artifact := sink.artifacts[path]; artifact == nil || artifact.removed {
				t.Fatalf("unretired input lost cleanup authority: path=%q artifact=%+v", path, artifact)
			}
		}
		reader, err := sink.openAuthenticatedRunReader(sink.runs[0])
		if err != nil {
			t.Fatal(err)
		}
		rowsRead := 0
		for {
			_, ok, readErr := reader.next(context.Background())
			if readErr != nil {
				t.Fatal(readErr)
			}
			if !ok {
				break
			}
			rowsRead++
		}
		if err := reader.close(); err != nil {
			t.Fatal(err)
		}
		if rowsRead != 3 {
			t.Fatalf("committed output row count=%d want=3", rowsRead)
		}
		assertTraceDBBoundedArtifactsRegistered(t, sink, dir)
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	t.Run("cleanup remove failure is visible and retryable", func(t *testing.T) {
		dir := t.TempDir()
		fault := errors.New("cleanup-remove")
		armed := false
		triggered := false
		options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
		options.ops.fault = func(point, _ string) error {
			if armed && !triggered && point == "remove" {
				triggered = true
				return fault
			}
			return nil
		}
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "cleanup"}); err != nil {
			t.Fatal(err)
		}
		armed = true
		if err := sink.cleanup(); !errors.Is(err, fault) {
			t.Fatalf("cleanup remove error=%v want=%v", err, fault)
		}
		assertTraceDBBoundedArtifactsRegistered(t, sink, dir)
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		assertTraceDBBoundedNoRunFiles(t, dir)
	})

	t.Run("cleanup remove-all failure is visible and retryable", func(t *testing.T) {
		fault := errors.New("cleanup-remove-all")
		triggered := false
		options := traceDBBoundedTestOptions(8<<20, 2<<20, 3)
		options.ops.fault = func(point, _ string) error {
			if !triggered && point == "remove_all" {
				triggered = true
				return fault
			}
			return nil
		}
		sink, err := newTraceDBRowSinkWithOptions("", 100, options)
		if err != nil {
			t.Fatal(err)
		}
		ownDir := sink.ownDir
		if err := sink.cleanup(); !errors.Is(err, fault) {
			t.Fatalf("cleanup remove-all error=%v want=%v", err, fault)
		}
		if _, err := os.Stat(ownDir); err != nil {
			t.Fatalf("failed remove-all unexpectedly removed own dir: %v", err)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(ownDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retry did not remove own dir: %v", err)
		}
	})

	t.Run("leaf live cap plus one is typed and disclosed", func(t *testing.T) {
		row := renderedRow{tsNS: 1, seq: 1, line: "leaf-live"}
		size := traceDBBoundedPhysicalRowSize(t, row, 0)
		options := traceDBRowSinkOptions{
			bufferBytes: 1, maxRunRowBytes: size, mergeFanIn: 2,
			activeTempCap: size * 2, liveTempCap: size * 4,
		}
		dir := t.TempDir()
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		sink.options.liveTempCap = size - 1
		err = sink.add(row)
		if reason := traceDBBoundedInvariantReason(t, err); reason != "trace_row_sort_live_temp_budget_exceeded" {
			t.Fatalf("leaf live +1 reason=%q", reason)
		}
		assertTraceDBBoundedFailureDisclosure(t, sink, "trace_row_sort_live_temp_budget_exceeded")
		assertTraceDBBoundedNoRunFiles(t, dir)
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cumulative temp overflow is typed before create", func(t *testing.T) {
		row := renderedRow{tsNS: 1, seq: 1, line: "cumulative"}
		size := traceDBBoundedPhysicalRowSize(t, row, 0)
		options := traceDBRowSinkOptions{
			bufferBytes: 1, maxRunRowBytes: size, mergeFanIn: 2,
			activeTempCap: size * 2, liveTempCap: size * 4,
		}
		dir := t.TempDir()
		sink, err := newTraceDBRowSinkWithOptions(dir, 1, options)
		if err != nil {
			t.Fatal(err)
		}
		sink.stats.TempBytes = math.MaxInt64 - int64(size) + 1
		err = sink.add(row)
		if reason := traceDBBoundedInvariantReason(t, err); reason != "trace_row_sort_cumulative_temp_overflow" {
			t.Fatalf("cumulative overflow reason=%q", reason)
		}
		assertTraceDBBoundedFailureDisclosure(t, sink, "trace_row_sort_cumulative_temp_overflow")
		assertTraceDBBoundedNoRunFiles(t, dir)
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTraceDBRowSorterMultiLevelCompactProvenanceAndWithholdParity(t *testing.T) {
	rows := []renderedRow{
		{tsNS: 100, seq: 0, line: "ordinary-first"},
		{tsNS: 200, seq: 1, line: "block-drop-start", pairKind: pairRenderBlock, pairLane: "block-drop", pairTable: "block_bio_queue", structuredPair: true, profilerEventField: 204},
		{tsNS: 300, seq: 2, line: "mmc-drop-start", pairKind: pairRenderMMC, pairLane: "mmc-drop", pairTable: "mmc_request_start", structuredPair: true, profilerEventField: 4016},
		{tsNS: 400, seq: 3, line: "f2fs-keep-start", pairKind: pairRenderF2FS, pairLane: "f2fs-keep", pairTable: "f2fs_write_begin", structuredPair: true, profilerEventField: 4011},
		{tsNS: 250, seq: 4, line: "ordinary-out-of-order"},
		{tsNS: 500, seq: 5, line: "block-drop-done", pairKind: pairRenderBlock, pairLane: "block-drop", pairTable: "block_bio_complete", structuredPair: true, profilerEventField: 202},
		{tsNS: 600, seq: 6, line: "mmc-drop-done", pairKind: pairRenderMMC, pairLane: "mmc-drop", pairTable: "mmc_request_done", structuredPair: true, profilerEventField: 4015},
		{tsNS: 700, seq: 7, line: "f2fs-keep-done", pairKind: pairRenderF2FS, pairLane: "f2fs-keep", pairTable: "f2fs_write_end", structuredPair: true, profilerEventField: 4012},
		{tsNS: 450, seq: 8, line: "ordinary-stable-a"},
		{tsNS: 450, seq: 8, line: "ordinary-stable-b"},
	}
	run := func(t *testing.T, threshold, fanIn int) ([]byte, traceDBRowSortStats) {
		t.Helper()
		sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), threshold,
			traceDBBoundedTestOptions(8<<20, 2<<20, fanIn))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = sink.cleanup() }()
		for _, row := range rows {
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		sink.poisonPairLane(pairRenderBlock, "block-drop")
		sink.poisonPairKind(pairRenderMMC)
		if err := sink.prepareForPublication(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(sink.runs) != 1 || sink.runs[0].rowCount != uint64(len(rows)) ||
			sink.stats.PeakOpenRunFDs > fanIn+1 {
			t.Fatalf("pair merge manifest drifted: runs=%+v stats=%+v", sink.runs, sink.stats)
		}
		reader, err := sink.openAuthenticatedRunReader(sink.runs[0])
		if err != nil {
			t.Fatal(err)
		}
		pairRows := 0
		for {
			record, ok, readErr := reader.next(context.Background())
			if readErr != nil {
				_ = reader.close()
				t.Fatal(readErr)
			}
			if !ok {
				break
			}
			if record.row.provenance.PairKind == pairRenderUnknown {
				continue
			}
			pairRows++
			if record.row.provenance.LaneID != 1 ||
				record.row.provenance.EndpointSlot == profilerPairEndpointNone ||
				record.row.provenance.Flags != profilerPairRowProvenanceStructured {
				_ = reader.close()
				t.Fatalf("compact pair provenance drifted: %+v", record.row.provenance)
			}
		}
		if err := reader.close(); err != nil {
			t.Fatal(err)
		}
		if pairRows != 6 {
			t.Fatalf("compact pair rows=%d want=6", pairRows)
		}
		var output bytes.Buffer
		stats, err := sink.writeTo(context.Background(), &output)
		if err != nil {
			t.Fatal(err)
		}
		if stats.RowsAccepted != len(rows) || stats.RowsWritten != len(rows)-4 || stats.RowsWithheld != 4 {
			t.Fatalf("pair withhold accounting drifted: %+v", stats)
		}
		body := output.String()
		for _, absent := range []string{"block-drop-start", "block-drop-done", "mmc-drop-start", "mmc-drop-done"} {
			if strings.Contains(body, absent) {
				t.Fatalf("withheld row %q escaped:\n%s", absent, body)
			}
		}
		for _, present := range []string{"ordinary-first", "ordinary-out-of-order", "ordinary-stable-a", "ordinary-stable-b", "f2fs-keep-start", "f2fs-keep-done"} {
			if !strings.Contains(body, present) {
				t.Fatalf("clean row %q disappeared:\n%s", present, body)
			}
		}
		return append([]byte(nil), output.Bytes()...), stats
	}

	single, singleStats := run(t, 100, 32)
	multi, multiStats := run(t, 1, 2)
	if !bytes.Equal(single, multi) {
		t.Fatalf("pair publication drifted across merge levels\nsingle=%q\nmulti =%q", single, multi)
	}
	if singleStats.MergePasses != 0 || multiStats.MergePasses != 4 || multiStats.SpillChunks != len(rows) {
		t.Fatalf("fixture did not cover leaf/multi-level paths: single=%+v multi=%+v", singleStats, multiStats)
	}
}

func TestTraceDBRowSorterPreparedAllWithheldBalancesWithoutWrite(t *testing.T) {
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1,
		traceDBBoundedTestOptions(8<<20, 2<<20, 2))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.cleanup() }()
	for _, row := range []renderedRow{
		{tsNS: 1, seq: 1, line: "mmc-withheld-start", pairKind: pairRenderMMC, pairLane: "mmc", pairTable: "mmc_request_start", structuredPair: true, profilerEventField: 4016},
		{tsNS: 2, seq: 2, line: "mmc-withheld-done", pairKind: pairRenderMMC, pairLane: "mmc", pairTable: "mmc_request_done", structuredPair: true, profilerEventField: 4015},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	sink.poisonPairKind(pairRenderMMC)
	if err := sink.prepareForPublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	publishable, err := sink.profilerPublishableRows()
	if err != nil {
		t.Fatal(err)
	}
	if publishable != 0 || sink.stats.RowsAccepted != 2 || sink.stats.RowsWritten != 0 ||
		sink.stats.RowsWithheld != 2 || sink.stats.ElapsedUS <= 0 {
		t.Fatalf("prepared all-withheld sink was not balanced before writeTo: publishable=%d stats=%+v",
			publishable, sink.stats)
	}
}

func TestTraceDBRowSorterRegisteredStorageFailureBalancesWithoutWrite(t *testing.T) {
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 1,
		traceDBBoundedTestOptions(8<<20, 2<<20, 2))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.cleanup() }()
	for _, row := range []renderedRow{
		{
			tsNS: 1, seq: 1, line: "block-registered-storage-row",
			pairKind: pairRenderBlock, pairLane: "block", pairTable: "block_bio_queue",
			structuredPair: true, profilerEventField: 204,
		},
		{tsNS: 2, seq: 2, line: "ordinary-row"},
	} {
		if err := sink.add(row); err != nil {
			t.Fatal(err)
		}
	}
	if len(sink.runs) != 2 {
		t.Fatalf("fixture did not create registered leaf runs: %d", len(sink.runs))
	}
	if err := os.Remove(sink.runs[0].path); err != nil {
		t.Fatal(err)
	}
	err = sink.prepareForPublication(context.Background())
	if reason := traceDBBoundedInvariantReason(t, err); reason != profilerPairStorageIntegrityFailure {
		t.Fatalf("registered storage failure reason=%q want=%q", reason, profilerPairStorageIntegrityFailure)
	}
	if sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
		sink.stats.RowsAccepted != 2 || sink.stats.RowsWritten != 0 || sink.stats.RowsWithheld != 2 {
		t.Fatalf("registered storage failure was not balanced before any output write: source=%q all=%t stats=%+v",
			sink.captureSourceFailure, sink.allRowsFailClosed, sink.stats)
	}
}

func TestTraceDBRowSorterStructureForbidsFullCohortAndAllRunReaderAllocation(t *testing.T) {
	if traceDBBufferedRowMetadataBytes != 256 {
		t.Fatalf("fixed row metadata charge=%d want=256", traceDBBufferedRowMetadataBytes)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), "streamerdb_sorter.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range parsed.Imports {
		if strings.Contains(imported.Path.Value, "tracequery") {
			t.Fatalf("row sorter acquired trace semantic parser dependency: %s", imported.Path.Value)
		}
	}
	forbiddenLineTokens := map[string]bool{"B|": true, "E|": true, "tracing_mark_write": true}
	for _, declaration := range parsed.Decls {
		ast.Inspect(declaration, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr == nil && forbiddenLineTokens[value] {
				t.Fatalf("row sorter acquired trace-line semantic token %q", value)
			}
			return true
		})
	}
	containsLineField := func(expression ast.Expr) bool {
		found := false
		ast.Inspect(expression, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "line" {
				found = true
				return false
			}
			return !found
		})
		return found
	}
	allowedLineConsumers := map[string]bool{
		"traceDBSinglePhysicalLine":               true,
		"profilerSinglePhysicalLineStringContext": true,
		"profilerCloneStringContext":              true,
		"Clone":                                   true,
		"len":                                     true,
		"WriteString":                             true,
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		usesLine := false
		for _, argument := range call.Args {
			usesLine = usesLine || containsLineField(argument)
		}
		if !usesLine {
			if identifier, ok := call.Fun.(*ast.Ident); ok && identifier.Name == "append" && len(call.Args) > 0 {
				if selector, ok := call.Args[0].(*ast.SelectorExpr); ok &&
					(selector.Sel.Name == "rows" || selector.Sel.Name == "rowIngestOrdinals") {
					t.Fatalf("row buffer bypassed controlled capacity allocator at %d", call.Pos())
				}
			}
			return true
		}
		name := ""
		switch function := call.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		}
		if !allowedLineConsumers[name] {
			t.Fatalf("row.line acquired semantic consumer %q at %d", name, call.Pos())
		}
		return true
	})
	isNamedType := func(expression ast.Expr, name string) bool {
		identifier, ok := expression.(*ast.Ident)
		return ok && identifier.Name == name
	}
	isLenOfRunSet := func(expression ast.Expr) bool {
		call, ok := expression.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return false
		}
		function, ok := call.Fun.(*ast.Ident)
		if !ok || function.Name != "len" {
			return false
		}
		selector, ok := call.Args[0].(*ast.SelectorExpr)
		return ok && (selector.Sel.Name == "runs" || selector.Sel.Name == "chunks")
	}
	forbiddenCompat := map[string]bool{
		"chunks": true, "chunkDigests": true, "traceDBChunkReader": true,
		"openTraceDBChunkReader": true, "openTraceDBChunkProofReader": true,
	}
	var writeTo *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "writeTo" {
			writeTo = function
		}
	}
	if writeTo == nil {
		t.Fatal("writeTo declaration missing")
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok && forbiddenCompat[identifier.Name] {
			t.Errorf("sorter retained compatibility storage authority %q", identifier.Name)
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		function, ok := call.Fun.(*ast.Ident)
		if !ok || function.Name != "make" || len(call.Args) == 0 {
			return true
		}
		array, ok := call.Args[0].(*ast.ArrayType)
		if !ok {
			return true
		}
		if isNamedType(array.Elt, "traceDBBufferedRunRow") {
			t.Errorf("sorter reintroduced a full []traceDBBufferedRunRow cohort copy")
			return true
		}
		pointer, ok := array.Elt.(*ast.StarExpr)
		if !ok || !isNamedType(pointer.X, "traceDBAuthenticatedRunReader") {
			return true
		}
		for _, argument := range call.Args[1:] {
			if isLenOfRunSet(argument) {
				t.Errorf("sorter allocated readers for the complete run set instead of one fan-in group")
			}
		}
		return true
	})
	forbiddenWriteCalls := map[string]bool{
		"prepareForPublication": true, "flushChunk": true, "flushChunkContext": true,
		"mergeRunsLeveled": true, "mergeRunGroup": true, "createPendingRun": true,
	}
	ast.Inspect(writeTo.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && forbiddenWriteCalls[selector.Sel.Name] {
			t.Errorf("writeTo recreated/mutated run state through %s", selector.Sel.Name)
		}
		return true
	})
	sorterSource, err := os.ReadFile("streamerdb_sorter.go")
	if err != nil {
		t.Fatal(err)
	}
	writeStart := bytes.Index(sorterSource, []byte("func (s *traceDBRowSink) writeTo"))
	if writeStart < 0 {
		t.Fatal("writeTo source declaration missing")
	}
	preparedGate := bytes.Index(sorterSource[writeStart:], []byte("if !s.prepared || s.prepareFailure != nil"))
	headerWrite := bytes.Index(sorterSource[writeStart:], []byte("writeSystraceHeader"))
	if preparedGate < 0 || headerWrite <= preparedGate {
		t.Errorf("writeTo prepared gate does not dominate publication: start=%d gate=%d header=%d",
			writeStart, preparedGate, headerWrite)
	}
	for _, file := range []struct {
		path      string
		preflight string
		open      string
	}{
		{path: "streamerdb_export.go", preflight: "sink.prepareForPublication(ctx)", open: "os.OpenFile(target.StagingPath"},
		{path: "profiler_container.go", preflight: "sink.sealProfilerCaptureContext(ctx)", open: "writeValidatedOwnedProfilerSystraceWithLedger("},
	} {
		source, err := os.ReadFile(file.path)
		if err != nil {
			t.Fatal(err)
		}
		preflight := bytes.Index(source, []byte(file.preflight))
		outputOpen := bytes.Index(source, []byte(file.open))
		if preflight < 0 || outputOpen <= preflight {
			t.Errorf("%s reaches output publication before sorter preflight: preflight=%d publication=%d", file.path, preflight, outputOpen)
		}
	}
	exportSource, err := os.ReadFile("streamerdb_export.go")
	if err != nil {
		t.Fatal(err)
	}
	deferStart := bytes.Index(exportSource, []byte("defer func() {"))
	if deferStart < 0 {
		t.Fatal("SQL export sorter cleanup defer missing")
	}
	deferred := exportSource[deferStart:]
	cleanup := bytes.Index(deferred, []byte("err = traceDBJoinPreservingSingle(err, sink.cleanup())"))
	refresh := bytes.Index(deferred, []byte("refreshed := sink.stats.coverage()"))
	preserveError := bytes.Index(deferred, []byte("refreshed.Error = item.Error"))
	if cleanup < 0 || refresh <= cleanup || preserveError <= refresh {
		t.Errorf("SQL export does not refresh sorter coverage after deferred cleanup while preserving its typed error: cleanup=%d refresh=%d preserve=%d",
			cleanup, refresh, preserveError)
	}
	preflightStart := bytes.Index(exportSource, []byte("if err := sink.prepareForPublication(ctx); err != nil {"))
	if preflightStart < 0 {
		t.Fatal("SQL export sorter preflight branch missing")
	}
	preflightBranch := exportSource[preflightStart:]
	appendCoverage := bytes.Index(preflightBranch, []byte("coverage = append(coverage, sorterCoverage)"))
	returnFailure := bytes.Index(preflightBranch, []byte("return traceDBSystraceExport{Coverage: coverage}, err"))
	if appendCoverage < 0 || returnFailure <= appendCoverage {
		t.Errorf("SQL preflight failure does not return typed sorter coverage: append=%d return=%d",
			appendCoverage, returnFailure)
	}
	if t.Failed() {
		return
	}
}
