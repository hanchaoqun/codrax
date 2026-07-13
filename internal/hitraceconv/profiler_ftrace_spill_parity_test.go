package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"
)

const profilerStructuredSpillParityRows = 4_096

type profilerStructuredSpillParityLane struct {
	Rows        int
	Seq         int
	Summary     profilerFtraceSummary
	Batch       profilerFtraceEventBatchCensus
	Recognized  bool
	Output      []byte
	IngestStats traceDBRowSortStats
	FinalStats  traceDBRowSortStats
}

func profilerStructuredPrintStreamResult(rows int) profilerTracePluginResult {
	var detail bytes.Buffer
	detail.Write(protoVarint(1, 2))
	for index := 0; index < rows; index++ {
		// Reverse timestamps force both lanes through the same external-sort
		// semantics instead of accidentally proving insertion-order parity only.
		ts := uint64(5_000_000_000 + rows - index)
		detail.Write(syntheticTracePluginFtraceEvent(
			ts, 7, 7, "worker", 1109,
			protoBytes(2, []byte("I|7|event_"+strconv.Itoa(index))),
		))
	}
	return decodeProfilerTracePluginResult(protoBytes(2, detail.Bytes()))
}

func runProfilerStructuredSpillParityLane(t *testing.T, result profilerTracePluginResult,
	threshold int, options traceDBRowSinkOptions,
) profilerStructuredSpillParityLane {
	t.Helper()
	tempDir := t.TempDir()
	sink, err := newTraceDBRowSinkWithOptions(tempDir, threshold, options)
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = sink.cleanup()
		}
	}()

	var batch profilerFtraceEventBatchCensus
	seq := 0
	rows, coverage, summary, recognized, err := renderProfilerFtraceStructuredResultConsumerContext(
		context.Background(), result, &seq, sink, false, &batch, true)
	if err != nil {
		t.Fatalf("render fused structured profiler stream: %v", err)
	}
	if len(coverage) != 0 {
		t.Fatalf("container fused path materialized direct coverage: %+v", coverage)
	}
	ingestStats := sink.stats

	var output bytes.Buffer
	finalStats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatalf("publish fused structured profiler stream: %v", err)
	}
	if err := sink.cleanup(); err != nil {
		t.Fatalf("idempotent sorter cleanup: %v", err)
	}
	cleaned = true
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("inspect sorter temp directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("sorter cleanup retained %d temp artifacts: %v", len(entries), entries)
	}

	return profilerStructuredSpillParityLane{
		Rows: rows, Seq: seq, Summary: summary, Batch: batch, Recognized: recognized,
		Output: append([]byte(nil), output.Bytes()...), IngestStats: ingestStats, FinalStats: finalStats,
	}
}

func TestProfilerFtraceStructuredPrintStreamSpillParity(t *testing.T) {
	result := profilerStructuredPrintStreamResult(profilerStructuredSpillParityRows)
	noSpill := runProfilerStructuredSpillParityLane(t, result,
		profilerStructuredSpillParityRows+1,
		traceDBRowSinkOptions{bufferBytes: 8 << 20})
	tinySpill := runProfilerStructuredSpillParityLane(t, result,
		8,
		traceDBRowSinkOptions{bufferBytes: 8 << 10})

	if noSpill.Rows != profilerStructuredSpillParityRows ||
		noSpill.Seq != profilerStructuredSpillParityRows ||
		!noSpill.Recognized ||
		noSpill.Summary.DetailMessages != 1 ||
		noSpill.Summary.DetailEventCount != profilerStructuredSpillParityRows {
		t.Fatalf("legal structured print stream census drifted: rows=%d seq=%d recognized=%t summary=%+v",
			noSpill.Rows, noSpill.Seq, noSpill.Recognized, noSpill.Summary)
	}
	printSlot := noSpill.Batch.Slots[profilerFtraceEventSlot(1109)]
	if noSpill.Batch.Overflow || printSlot.RowsRead != profilerStructuredSpillParityRows ||
		printSlot.RowsEmitted != profilerStructuredSpillParityRows || printSlot.IssueCount != 0 {
		t.Fatalf("legal structured print stream fixed batch drifted: overflow=%t slot=%+v",
			noSpill.Batch.Overflow, printSlot)
	}
	if tinySpill.Rows != noSpill.Rows || tinySpill.Seq != noSpill.Seq ||
		tinySpill.Recognized != noSpill.Recognized ||
		!reflect.DeepEqual(tinySpill.Summary, noSpill.Summary) ||
		!reflect.DeepEqual(tinySpill.Batch, noSpill.Batch) ||
		!bytes.Equal(tinySpill.Output, noSpill.Output) {
		t.Fatalf("tiny-threshold spill changed fused structured output or accounting:\nno_spill=%+v\ntiny_spill=%+v",
			noSpill, tinySpill)
	}

	// SpillChunks counts physical runs. At ingest completion the high lane must
	// still be memory-only, while the tiny lane must already have externalized
	// chunks. Publication subsequently creates the high lane's sole final run.
	if noSpill.IngestStats.SpillChunks != 0 {
		t.Fatalf("high-threshold lane spilled during ingestion: %+v", noSpill.IngestStats)
	}
	if tinySpill.IngestStats.SpillChunks == 0 ||
		tinySpill.IngestStats.PeakBufferedRows > 8 ||
		tinySpill.IngestStats.PeakBufferedBytes > 8<<10 {
		t.Fatalf("tiny-threshold lane escaped configured row/byte bounds: %+v", tinySpill.IngestStats)
	}
	for name, lane := range map[string]profilerStructuredSpillParityLane{
		"no_spill":   noSpill,
		"tiny_spill": tinySpill,
	} {
		if lane.FinalStats.RowsWritten != profilerStructuredSpillParityRows ||
			lane.FinalStats.RowsWithheld != 0 || lane.FinalStats.CurrentLiveTempBytes != 0 {
			t.Fatalf("%s publication accounting or live-temp cleanup drifted: %+v", name, lane.FinalStats)
		}
	}
}
