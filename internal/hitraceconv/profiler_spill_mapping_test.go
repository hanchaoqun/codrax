package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func profilerSpillMappingBlockRow() renderedRow {
	return renderedRow{
		tsNS: 1, seq: 11, line: "block_bio_queue-row",
		pairKind: pairRenderBlock, pairLane: "block-lane", pairTable: "block_bio_queue",
		structuredPair: true, profilerEventField: 204,
	}
}

func assertProfilerCompactStorageSidecarSamples(
	t *testing.T,
	manifest profilerSourceOrderSidecarManifest,
	rows int,
) {
	t.Helper()
	file, err := os.Open(manifest.path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, ordinal := range []uint64{0, uint64(rows / 2), uint64(rows - 1)} {
		offset, err := profilerSourceOrderSidecarRecordOffset(ordinal, uint64(rows))
		if err != nil {
			t.Fatal(err)
		}
		var wire [profilerSourceOrderSidecarRecordBytes]byte
		if _, err := file.ReadAt(wire[:], offset); err != nil {
			t.Fatal(err)
		}
		record, err := decodeProfilerSourceOrderSidecarRecord(wire[:])
		if err != nil {
			t.Fatal(err)
		}
		wantLaneID := uint32(1)
		wantEndpoint := profilerPairEndpointF2FSWriteBegin
		wantDisposition := profilerSourceOrderDispositionWithhold
		if ordinal%2 == 1 {
			wantLaneID = 2
			wantEndpoint = profilerPairEndpointF2FSWriteEnd
			wantDisposition = profilerSourceOrderDispositionPublish
		}
		if record.ordinalPlusOne != ordinal+1 || record.provenance.LaneID != wantLaneID ||
			record.provenance.TextMessageOrdinal != 1 ||
			record.provenance.PairKind != pairRenderF2FS ||
			record.provenance.EndpointSlot != wantEndpoint ||
			record.provenance.PublisherSlot != profilerPairPublisherOtherText ||
			record.provenance.Flags != profilerPairRowProvenanceText ||
			record.disposition != wantDisposition {
			t.Fatalf("compact storage sidecar sample ordinal=%d drifted: record=%+v want_lane=%d want_endpoint=%d want_disposition=%d",
				ordinal, record, wantLaneID, wantEndpoint, wantDisposition)
		}
	}
}

func rewriteProfilerSpillChunkRow(t *testing.T, path string, mutate func(*traceDBChunkRow)) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var row traceDBChunkRow
	if err := json.Unmarshal(bytes.TrimSpace(raw), &row); err != nil {
		t.Fatalf("decode spill row: %v\n%s", err, raw)
	}
	mutate(&row)
	rewritten, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	rewritten = append(rewritten, '\n')
	if err := os.WriteFile(path, rewritten, 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendProfilerSpillChunkRow(t *testing.T, path string, row traceDBChunkRow) {
	t.Helper()
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(append(raw, '\n')); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// refreshProfilerRunProof deliberately updates the test fixture's typed
// manifest after rewriting a run. This isolates producer-root validation from
// the stronger physical-digest layer; ordinary tamper tests never call it.
func refreshProfilerRunProof(t *testing.T, sink *traceDBRowSink, runIndex int) {
	t.Helper()
	raw, err := os.ReadFile(sink.runs[runIndex].path)
	if err != nil {
		t.Fatal(err)
	}
	oldSize := sink.runs[runIndex].size
	newSize := uint64(len(raw))
	sink.runs[runIndex].size = newSize
	sink.runs[runIndex].digest = sha256.Sum256(raw)
	if newSize >= oldSize {
		delta := newSize - oldSize
		sink.activeTempBytes += delta
		sink.liveTempBytes += delta
	} else {
		delta := oldSize - newSize
		sink.activeTempBytes -= delta
		sink.liveTempBytes -= delta
	}
	sink.stats.CurrentLiveTempBytes = sink.liveTempBytes
}

func requireProfilerInactiveStorageIntegrityError(t *testing.T, err error) {
	t.Helper()
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != profilerPairStorageIntegrityFailure {
		t.Fatalf("inactive storage drift error=%v reason=%q typed=%t want=%q",
			err, reason, ok, profilerPairStorageIntegrityFailure)
	}
}

func TestProfilerInactiveSpillWithoutDriftWritesNormally(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if sink.captureSourceFailure != "" || stats.RowsAccepted != 2 || stats.RowsWritten != 2 ||
		stats.RowsWithheld != 0 || !strings.Contains(output.String(), "block_bio_queue-row") ||
		!strings.Contains(output.String(), "ordinary-survives") {
		t.Fatalf("clean inactive spill was not published normally: source=%q stats=%+v\n%s",
			sink.captureSourceFailure, stats, output.String())
	}
}

func TestProfilerCompactStorageTinySpillLifecycle(t *testing.T) {
	const rows = 512
	source := profilerSourceLifecycleFile(t)
	options := traceDBRowSinkOptions{
		bufferBytes: 8 << 20, maxRunRowBytes: 2 << 20, mergeFanIn: 2,
		activeTempCap: 64 << 20, liveTempCap: 128 << 20,
	}
	sink, err := newTraceDBRowSinkWithOptions(t.TempDir(), 17, options)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin compact tiny-spill text publisher")
	}
	for index := 0; index < rows; index++ {
		row := renderedRow{tsNS: uint64(index + 1), seq: index, pairKind: pairRenderF2FS}
		if index%2 == 0 {
			row.line, row.pairLane, row.pairTable =
				"compact-tiny-begin", "compact-storage-lane-a", "f2fs_write_begin"
			row.profilerEndpointSlot = profilerPairEndpointF2FSWriteBegin
		} else {
			row.line, row.pairLane, row.pairTable =
				"compact-tiny-end", "compact-storage-lane-b", "f2fs_write_end"
			row.profilerEndpointSlot = profilerPairEndpointF2FSWriteEnd
		}
		if err := sink.add(row); err != nil {
			t.Fatalf("add compact tiny-spill row %d: %v", index, err)
		}
	}
	if err := sink.endProfilerTextMessage(rows); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	sink.poisonPairLane(pairRenderF2FS, "compact-storage-lane-a")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	registry := sink.pairLaneRegistries[pairRenderF2FS]
	wantSidecarBytes := uint64(profilerSourceOrderSidecarHeaderBytes) +
		uint64(rows)*uint64(profilerSourceOrderSidecarRecordBytes)
	if len(sink.runs) != 1 || sink.runs[0].rowCount != rows ||
		sink.stats.SpillChunks <= 1 || sink.stats.MergePasses <= 1 ||
		sink.stats.PeakOpenRunFDs > options.mergeFanIn+1 ||
		sink.sourceOrderSidecar.size != wantSidecarBytes ||
		len(registry.keys) != 2 || len(registry.states) != 2 ||
		!registry.states[0].poisoned || registry.states[1].poisoned {
		t.Fatalf("compact tiny-spill prepared shape drifted: registry=%+v runs=%+v sidecar=%+v stats=%+v",
			registry, sink.runs, sink.sourceOrderSidecar, sink.stats)
	}
	assertProfilerCompactStorageSidecarSamples(t, sink.sourceOrderSidecar, rows)
	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil || stats.RowsWritten != rows/2 || stats.RowsWithheld != rows/2 ||
		strings.Contains(output.String(), "compact-tiny-begin") ||
		!strings.Contains(output.String(), "compact-tiny-end") {
		t.Fatalf("compact tiny-spill publication drifted: err=%v stats=%+v\n%s",
			err, stats, output.String())
	}
}

func TestProfilerInactiveFinalRunIsAuthenticatedDuringPreflight(t *testing.T) {
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.add(renderedRow{tsNS: 1, seq: 1, line: "ordinary-a"}); err != nil {
		t.Fatal(err)
	}
	if len(sink.runs) != 1 {
		t.Fatalf("single-leaf fixture runs=%d want=1", len(sink.runs))
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.Line = "ordinary-b"
	})
	err = sink.prepareForPublication(context.Background())
	requireProfilerInactiveStorageIntegrityError(t, err)
	if sink.prepared || sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
		sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
		sink.stats.RowsAccepted != 1 || sink.stats.RowsWritten != 0 || sink.stats.RowsWithheld != 1 {
		t.Fatalf("inactive preflight authentication did not fail closed: prepared=%t authority=%q source=%q all=%t stats=%+v",
			sink.prepared, sink.pairAuthorityFailure, sink.captureSourceFailure,
			sink.allRowsFailClosed, sink.stats)
	}
}

func TestTraceDBInactiveOrdinarySinkRejectsProfilerRowsBeforeMutation(t *testing.T) {
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	err = sink.add(profilerSpillMappingBlockRow())
	if reason := traceDBInvariantReason(err); reason != "trace_row_sink_inactive_nonordinary_row" {
		t.Fatalf("inactive ordinary row gate reason=%q err=%v", reason, err)
	}
	if sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 || len(sink.runs) != 0 ||
		len(sink.pairLaneRegistries[pairRenderBlock].states) != 0 ||
		sink.profilerSourceProof.count != 0 {
		t.Fatalf("inactive profiler reject mutated sink: stats=%+v rows=%d runs=%d registry=%+v proof=%+v",
			sink.stats, len(sink.rows), len(sink.runs),
			sink.pairLaneRegistries[pairRenderBlock], sink.profilerSourceProof)
	}
	if sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		sink.beginProfilerTextMessage() || sink.pairCensusActive || sink.textMessageActive {
		t.Fatal("inactive ordinary sink opened a profiler publisher/message context")
	}
	if err := sink.endProfilerTextMessage(0); traceDBInvariantReason(err) != "trace_row_sink_inactive_profiler_mutation" {
		t.Fatalf("inactive ordinary sink accepted text-message end: %v", err)
	}
	sink.abortProfilerTextMessage()
	sink.abortPairRowCensus()
	census := sink.endPairRowCensus()
	for kind := pairRenderKind(0); kind < pairRenderKindCount; kind++ {
		if census[kind].total != 0 {
			t.Fatalf("inactive ordinary sink returned profiler census: %+v", census)
		}
	}
	zeroDelta := traceDBProfilerEventDelta{}
	err = sink.addContext(context.Background(), renderedRow{tsNS: 2, seq: 2, line: "ordinary"}, &zeroDelta, false)
	if reason := traceDBInvariantReason(err); reason != "trace_row_sink_inactive_nonordinary_row" {
		t.Fatalf("inactive zero-delta pointer bypass reason=%q err=%v", reason, err)
	}
	delta := traceDBProfilerEventDelta{}
	delta.poisonKinds[pairRenderF2FS] = true
	if err := sink.commitProfilerEventDeltaContext(context.Background(), delta); traceDBInvariantReason(err) != "trace_row_sink_inactive_profiler_mutation" {
		t.Fatalf("inactive detached delta bypassed ordinary gate: %v", err)
	}
	sink.markPairCaptureOpaque(pairRenderMMC)
	sink.poisonPairKind(pairRenderF2FS)
	sink.poisonPairLane(pairRenderBlock, "forbidden-lane")
	sink.failCloseAllRows()
	if len(sink.opaque) != 0 || len(sink.poisoned) != 0 || len(profilerTestPoisonedLanes(sink)) != 0 ||
		sink.allRowsFailClosed || sink.pairAuthorityFailure != "" {
		t.Fatalf("inactive profiler mutator changed ordinary authority: opaque=%v poison=%v lanes=%v all=%t authority=%q",
			sink.opaque, sink.poisoned, profilerTestPoisonedLanes(sink), sink.allRowsFailClosed,
			sink.pairAuthorityFailure)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 2, line: "ordinary"}); err != nil {
		t.Fatalf("ordinary row rejected after profiler negative control: %v", err)
	}
	if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); traceDBInvariantReason(err) != "profiler_capture_ordinary_sink_forbidden" {
		t.Fatalf("ordinary sink entered profiler capture: %v", err)
	}
}

func TestProfilerSealedSpillWithoutDriftWritesNormally(t *testing.T) {
	source := t.TempDir() + "/capture.htrace"
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
		t.Fatal(err)
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extracted := profilerContainerExtraction{Detected: true, Kind: "openharmony_profiler_trace_file"}
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if extracted.SourceFailClosed || sink.captureSourceFailure != "" || stats.RowsAccepted != 2 ||
		stats.RowsWritten != 2 || stats.RowsWithheld != 0 ||
		!strings.Contains(output.String(), "block_bio_queue-row") ||
		!strings.Contains(output.String(), "ordinary-survives") {
		t.Fatalf("clean sealed spill was not published normally: extracted=%+v source=%q stats=%+v\n%s",
			extracted, sink.captureSourceFailure, stats, output.String())
	}
}

func TestProfilerRegisteredSpillReadFailuresFailClosed(t *testing.T) {
	corruptions := []struct {
		name  string
		apply func(*testing.T, string)
	}{
		{
			name: "invalid JSON truncation",
			apply: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "registered chunk deleted",
			apply: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, corruption := range corruptions {
		t.Run("inactive/"+corruption.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
				t.Fatal(err)
			}
			corruption.apply(t, sink.runs[0].path)

			counter := &profilerSpillWriteCounter{}
			stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
			requireProfilerInactiveStorageIntegrityError(t, err)
			if sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
				sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
				counter.calls != 0 || counter.bytes != 0 || stats.RowsAccepted != 2 ||
				stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
				t.Fatalf("inactive registered read failure escaped: reason=%q source=%q all=%t writes=%d/%d stats=%+v",
					sink.pairAuthorityFailure, sink.captureSourceFailure, sink.allRowsFailClosed,
					counter.calls, counter.bytes, stats)
			}
		})

		t.Run("sealed/"+corruption.name, func(t *testing.T) {
			source := t.TempDir() + "/capture.htrace"
			if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.openProfilerCapture(source); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
				t.Fatal(err)
			}
			corruption.apply(t, sink.runs[0].path)
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatalf("registered storage failure blocked source-failure disclosure: %v", err)
			}
			if sink.stats.RowsAccepted != 2 || sink.stats.RowsWritten != 0 || sink.stats.RowsWithheld != 2 {
				t.Fatalf("sealed source failure was not balanced before writeTo: %+v", sink.stats)
			}
			extracted := profilerContainerExtraction{Detected: true, Kind: "openharmony_profiler_trace_file"}
			if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
				t.Fatal(err)
			}
			counter := &profilerSpillWriteCounter{}
			stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
			if err != nil {
				t.Fatal(err)
			}
			barrierFound := false
			for _, coverage := range extracted.TraceCoverage {
				if coverage.Table == "__container_integrity_barrier__" &&
					coverage.FieldSources["shared_authority_failure"] == "profiler_pair_spill_integrity_mismatch" &&
					coverage.FieldSources["suppressed_rows"] == "2" {
					barrierFound = true
				}
			}
			if sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
				sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
				!extracted.SourceFailClosed || extracted.SourceFailReason != profilerPairStorageIntegrityFailure ||
				!barrierFound || counter.calls != 0 || counter.bytes != 0 || stats.RowsAccepted != 2 ||
				stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
				t.Fatalf("sealed registered read failure escaped: sink=%+v extracted=%+v writes=%d/%d stats=%+v",
					sink, extracted, counter.calls, counter.bytes, stats)
			}
		})
	}
}

func TestProfilerRegisteredSpillContextCancellationKeepsItsBoundary(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = sink.prepareAndWriteForTest(ctx, &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) || sink.captureSourceFailure != "" || sink.allRowsFailClosed {
		t.Fatalf("context cancellation was reclassified as storage drift: err=%v source=%q all=%t",
			err, sink.captureSourceFailure, sink.allRowsFailClosed)
	}
}

func TestProfilerSpillRejectsInternallyIncoherentTypedProvenanceAgainstProducerRoot(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-sibling"}); err != nil {
		t.Fatal(err)
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.ProfilerProvenance.EndpointSlot = profilerPairEndpointBlockBIOComplete
	})
	refreshProfilerRunProof(t, sink, 0)
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}

	counter := &profilerSpillWriteCounter{}
	stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
	if err != nil {
		t.Fatal(err)
	}
	if sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
		sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
		counter.calls != 0 || counter.bytes != 0 || stats.RowsAccepted != 2 ||
		stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
		t.Fatalf("internally incoherent provenance escaped source fail-close: authority=%q source=%q all=%t "+
			"writes=%d/%d stats=%+v", sink.pairAuthorityFailure, sink.captureSourceFailure,
			sink.allRowsFailClosed, counter.calls, counter.bytes, stats)
	}
}

func TestProfilerSpillAuthenticatesEveryCompactProvenanceScalar(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*profilerPairRowProvenance)
	}{
		{name: "lane id", mutate: func(value *profilerPairRowProvenance) { value.LaneID++ }},
		{name: "message ordinal", mutate: func(value *profilerPairRowProvenance) { value.TextMessageOrdinal++ }},
		{name: "pair kind", mutate: func(value *profilerPairRowProvenance) {
			value.PairKind = pairRenderMMC
		}},
		{name: "endpoint slot", mutate: func(value *profilerPairRowProvenance) {
			value.EndpointSlot = profilerPairEndpointF2FSWriteEnd
		}},
		{name: "publisher slot", mutate: func(value *profilerPairRowProvenance) {
			value.PublisherSlot = profilerPairPublisherBytrace
		}},
		{name: "flags", mutate: func(value *profilerPairRowProvenance) { value.Flags = 0 }},
		{name: "trace class", mutate: func(value *profilerPairRowProvenance) {
			value.TraceClass = profilerTraceClassTextKnown
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := profilerSourceLifecycleFile(t)
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.openProfilerCapture(source); err != nil {
				t.Fatal(err)
			}
			if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
				!sink.beginProfilerTextMessage() {
				t.Fatal("begin text publisher")
			}
			if err := sink.add(renderedRow{
				tsNS: 1, seq: 11, line: "f2fs-text-pair", pairKind: pairRenderF2FS,
				pairLane: "lane", pairTable: "f2fs_write_begin",
				profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
			}); err != nil {
				t.Fatal(err)
			}
			if err := sink.endProfilerTextMessage(1); err != nil {
				t.Fatal(err)
			}
			_ = sink.endPairRowCensus()
			if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-sibling"}); err != nil {
				t.Fatal(err)
			}
			if len(sink.runs) != 2 {
				t.Fatalf("text provenance fixture runs=%d want=2", len(sink.runs))
			}
			rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
				test.mutate(&row.ProfilerProvenance)
			})
			refreshProfilerRunProof(t, sink, 0)
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}

			counter := &profilerSpillWriteCounter{}
			stats, writeErr := sink.prepareAndWriteForTest(context.Background(), counter)
			if writeErr != nil || sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
				sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
				stats.RowsWritten != 0 || stats.RowsWithheld != 2 || counter.calls != 0 || counter.bytes != 0 {
				t.Fatalf("compact %s scalar escaped producer-root fail-close: err=%v authority=%q source=%q all=%t writes=%d/%d stats=%+v",
					test.name, writeErr, sink.pairAuthorityFailure, sink.captureSourceFailure,
					sink.allRowsFailClosed, counter.calls, counter.bytes, stats)
			}
		})
	}
}

func TestProfilerMintedTraceClassSurvivesTinySpillRunAndSidecar(t *testing.T) {
	source := profilerSourceLifecycleFile(t)
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if err := sink.enableProfilerTraceClassification(); err != nil {
		t.Fatal(err)
	}
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin classified text publisher")
	}
	lines := []string{
		traceDBFormatLine("worker", 7, 7, 1, 1_000_000_000, 0, 0,
			"sched_wakeup: comm=app pid=42 prio=120 target_cpu=1"),
		traceDBFormatLine("worker", 7, 7, 1, 1_001_000_000, 0, 0,
			"vendor_private_event: opaque=1"),
	}
	for index, line := range lines {
		if err := sink.add(renderedRow{tsNS: uint64(1_000_000_000 + index*1_000_000), seq: index, line: line}); err != nil {
			t.Fatalf("add classified row %d: %v", index, err)
		}
	}
	if err := sink.endProfilerTextMessage(len(lines)); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	if len(sink.runs) != 1 || sink.runs[0].rowCount != uint64(len(lines)) ||
		!sink.sourceOrderSidecar.present() || sink.sourceOrderSidecar.rowCount != uint64(len(lines)) {
		t.Fatalf("classified spill topology drifted: runs=%+v sidecar=%+v", sink.runs, sink.sourceOrderSidecar)
	}
	rawRun, err := os.ReadFile(sink.runs[0].path)
	if err != nil {
		t.Fatal(err)
	}
	runLines := bytes.Split(bytes.TrimSpace(rawRun), []byte{'\n'})
	wantClasses := []profilerTraceClass{profilerTraceClassTextKnown, profilerTraceClassTextIntentionalUnknown}
	if len(runLines) != len(wantClasses) {
		t.Fatalf("run rows=%d want=%d", len(runLines), len(wantClasses))
	}
	for index, raw := range runLines {
		var chunk traceDBChunkRow
		if err := json.Unmarshal(raw, &chunk); err != nil {
			t.Fatalf("decode run row %d: %v", index, err)
		}
		if chunk.ProfilerProvenance.TraceClass != wantClasses[index] ||
			!chunk.ProfilerProvenance.classifiedValid() {
			t.Fatalf("run class[%d]=%+v want=%d", index, chunk.ProfilerProvenance, wantClasses[index])
		}
	}
	rawSidecar, err := os.ReadFile(sink.sourceOrderSidecar.path)
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range wantClasses {
		start := int(profilerSourceOrderSidecarHeaderBytes) + index*int(profilerSourceOrderSidecarRecordBytes)
		record, err := decodeProfilerSourceOrderSidecarRecord(
			rawSidecar[start : start+int(profilerSourceOrderSidecarRecordBytes)],
		)
		if err != nil || record.provenance.TraceClass != want || !record.provenance.classifiedValid() {
			t.Fatalf("sidecar class[%d]=%+v want=%d err=%v", index, record.provenance, want, err)
		}
	}
	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil || stats.RowsWritten != len(lines) {
		t.Fatalf("classified spill publication drifted: stats=%+v err=%v", stats, err)
	}
}

func TestProfilerSealCompletesSpillProducerProofBeforePublication(t *testing.T) {
	source := t.TempDir() + "/capture.htrace"
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("readback mismatch closes global authority at seal", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 1)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(source); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
			t.Fatal(err)
		}
		rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
			row.ProfilerProvenance.LaneID++
		})
		refreshProfilerRunProof(t, sink, 0)
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		if sink.captureLifecycle != profilerCaptureSealed ||
			sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
			sink.captureSourceFailure != profilerPairStorageIntegrityFailure ||
			!sink.allRowsFailClosed || sink.publishableRows() != 0 {
			t.Fatalf("seal did not freeze producer-root failure: lifecycle=%d reason=%q publishable=%d",
				sink.captureLifecycle, sink.pairAuthorityFailure, sink.publishableRows())
		}
	})

	t.Run("pending in-memory tail joins the same proof", func(t *testing.T) {
		sink, err := newTraceDBRowSink(t.TempDir(), 2)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(source); err != nil {
			t.Fatal(err)
		}
		first := profilerSpillMappingBlockRow()
		second := first
		second.tsNS, second.seq, second.line = 3, 13, "block_bio_complete-row"
		second.pairLane, second.pairTable, second.profilerEventField = "other-lane", "block_bio_complete", 202
		for _, row := range []renderedRow{first, {tsNS: 2, seq: 12, line: "ordinary"}, second} {
			if err := sink.add(row); err != nil {
				t.Fatal(err)
			}
		}
		if len(sink.runs) != 1 || len(sink.rows) != 1 {
			t.Fatalf("fixture did not split spill and tail: runs=%d rows=%d", len(sink.runs), len(sink.rows))
		}
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		if sink.pairAuthorityFailure != "" || len(sink.rows) != 0 || len(sink.runs) != 1 ||
			sink.sourceOrderSidecar.rowCount != 3 {
			t.Fatalf("seal did not join spill and tail proof: reason=%q runs=%d rows=%d sidecar_rows=%d",
				sink.pairAuthorityFailure, len(sink.runs), len(sink.rows), sink.sourceOrderSidecar.rowCount)
		}
	})
}

type profilerSpillWriteCounter struct {
	calls int
	bytes int
}

func (counter *profilerSpillWriteCounter) Write(data []byte) (int, error) {
	counter.calls++
	counter.bytes += len(data)
	return len(data), nil
}

func TestProfilerSpillUnassociatedRowsFailSourceBeforeAnyWrite(t *testing.T) {
	corruptions := []struct {
		name       string
		wantReason string
		apply      func(*testing.T, string)
	}{
		{
			name:       "pair seq and kind jointly drift to ordinary",
			wantReason: "profiler_pair_spill_integrity_mismatch",
			apply: func(t *testing.T, path string) {
				rewriteProfilerSpillChunkRow(t, path, func(row *traceDBChunkRow) {
					row.Seq = 991
					row.ProfilerProvenance = profilerPairRowProvenance{}
				})
			},
		},
		{
			name:       "extra forged ordinary row",
			wantReason: "profiler_pair_spill_integrity_mismatch",
			apply: func(t *testing.T, path string) {
				appendProfilerSpillChunkRow(t, path, traceDBChunkRow{
					TSNS: 9, Seq: 992, Line: "forged-ordinary-with-pair-text",
				})
			},
		},
	}
	for _, corruption := range corruptions {
		t.Run("inactive write/"+corruption.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "genuine-ordinary-sibling"}); err != nil {
				t.Fatal(err)
			}
			corruption.apply(t, sink.runs[0].path)

			counter := &profilerSpillWriteCounter{}
			stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
			requireProfilerInactiveStorageIntegrityError(t, err)
			if sink.pairAuthorityFailure != corruption.wantReason || !sink.allRowsFailClosed ||
				sink.captureSourceFailure != profilerPairStorageIntegrityFailure ||
				counter.calls != 0 || counter.bytes != 0 || stats.RowsAccepted != 2 ||
				stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
				t.Fatalf("unassociated spill row escaped before fail-close: reason=%q all=%t writes=%d/%d stats=%+v",
					sink.pairAuthorityFailure, sink.allRowsFailClosed, counter.calls, counter.bytes, stats)
			}
		})
	}

	t.Run("sealed capture suppresses destination before open eligibility", func(t *testing.T) {
		source := t.TempDir() + "/capture.htrace"
		if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
			t.Fatal(err)
		}
		sink, err := newTraceDBRowSink(t.TempDir(), 1)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		if err := sink.openProfilerCapture(source); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
			t.Fatal(err)
		}
		if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "genuine-ordinary-sibling"}); err != nil {
			t.Fatal(err)
		}
		corruptions[0].apply(t, sink.runs[0].path)
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		publishable, err := sink.profilerPublishableRows()
		if err != nil {
			t.Fatal(err)
		}
		counter := &profilerSpillWriteCounter{}
		stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
		if err != nil {
			t.Fatal(err)
		}
		if publishable != 0 || !sink.allRowsFailClosed || counter.calls != 0 || counter.bytes != 0 ||
			sink.captureSourceFailure != profilerPairStorageIntegrityFailure ||
			stats.RowsAccepted != 2 || stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
			t.Fatalf("sealed composite drift became output-eligible: publishable=%d all=%t writes=%d/%d stats=%+v",
				publishable, sink.allRowsFailClosed, counter.calls, counter.bytes, stats)
		}
	})
}

func TestProfilerStorageIntegrityFailureBecomesDisclosedSourceFailure(t *testing.T) {
	source := t.TempDir() + "/capture.htrace"
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "genuine-ordinary-sibling"}); err != nil {
		t.Fatal(err)
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.Seq = 994
		row.ProfilerProvenance = profilerPairRowProvenance{}
	})
	refreshProfilerRunProof(t, sink, 0)
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	if sink.captureBreach != "" {
		t.Fatalf("storage source failure was misclassified as a seal breach: %q", sink.captureBreach)
	}
	extracted := profilerContainerExtraction{
		Detected: true, Kind: "openharmony_profiler_trace_file",
		TextRows: 1, StructuredRows: 1, TextPluginMessages: 1,
		TraceCoverage: []TraceDBCoverage{{
			Family: "builtin_modern_profiler", Table: "plugin:test", Role: "test",
			Found: true, RowsRead: 2, RowsEmitted: 2,
		}},
	}
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		t.Fatal(err)
	}
	coverageCount := len(extracted.TraceCoverage)
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		t.Fatal(err)
	}
	if len(extracted.TraceCoverage) != coverageCount {
		t.Fatalf("source failure disclosure was not idempotent: entries=%d want=%d", len(extracted.TraceCoverage), coverageCount)
	}
	var barrier *TraceDBCoverage
	for index := range extracted.TraceCoverage {
		item := &extracted.TraceCoverage[index]
		if item.Table == "__container_integrity_barrier__" {
			barrier = item
		}
	}
	if !extracted.SourceFailClosed || extracted.SourceFailReason != profilerPairStorageIntegrityFailure ||
		extracted.TextRows != 0 || extracted.StructuredRows != 0 || extracted.TextPluginMessages != 0 ||
		barrier == nil || barrier.RowsRead != 2 ||
		!strings.Contains(barrier.Skipped, "suppressed_rows=2") ||
		barrier.FieldSources["failure_class"] != "storage_integrity_failure" ||
		barrier.FieldSources["shared_authority_failure"] != "profiler_pair_spill_integrity_mismatch" ||
		barrier.FieldSources["suppressed_rows"] != "2" ||
		barrier.FieldSources["seal_before_output_open"] != "true" || sink.captureBreach != "" {
		t.Fatalf("storage source failure disclosure drifted: extracted=%+v barrier=%+v breach=%q",
			extracted, barrier, sink.captureBreach)
	}
}

func TestProfilerSpillHashCoversTimestampPairLineAndOrdinaryLine(t *testing.T) {
	tests := []struct {
		name       string
		chunkIndex int
		mutate     func(*traceDBChunkRow)
	}{
		{
			name: "pair timestamp only", chunkIndex: 0,
			mutate: func(row *traceDBChunkRow) { row.TSNS++ },
		},
		{
			name: "pair line only", chunkIndex: 0,
			mutate: func(row *traceDBChunkRow) { row.Line = "drifted-pair-line" },
		},
		{
			name: "ordinary line becomes exact block endpoint", chunkIndex: 1,
			mutate: func(row *traceDBChunkRow) {
				row.Line = traceDBFormatLine("worker", 40, 40, 2, 5_000_000_000, 0, 0,
					"block_rq_issue: 0,1 R 4 () 2 + 3 []")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := t.TempDir() + "/capture.htrace"
			if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.openProfilerCapture(source); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "genuine-ordinary-sibling"}); err != nil {
				t.Fatal(err)
			}
			rewriteProfilerSpillChunkRow(t, sink.runs[test.chunkIndex].path, test.mutate)
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			extracted := profilerContainerExtraction{
				Detected: true, Kind: "openharmony_profiler_trace_file", TextRows: 1, StructuredRows: 1,
				TraceCoverage: []TraceDBCoverage{{
					Family: "builtin_modern_profiler", Table: "plugin:test", Role: "test",
					Found: true, RowsRead: 2, RowsEmitted: 2,
				}},
			}
			if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
				t.Fatal(err)
			}
			counter := &profilerSpillWriteCounter{}
			stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
			if err != nil {
				t.Fatal(err)
			}
			barrierFound := false
			for _, coverage := range extracted.TraceCoverage {
				if coverage.Table == "__container_integrity_barrier__" && coverage.RowsRead == 2 &&
					coverage.FieldSources["shared_authority_failure"] == "profiler_pair_spill_integrity_mismatch" &&
					coverage.FieldSources["suppressed_rows"] == "2" {
					barrierFound = true
				}
			}
			if sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
				sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
				!extracted.SourceFailClosed || extracted.SourceFailReason != profilerPairStorageIntegrityFailure ||
				!barrierFound || counter.calls != 0 || counter.bytes != 0 ||
				stats.RowsAccepted != 2 || stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
				t.Fatalf("spill hash gap escaped: sink=%+v extracted=%+v writes=%d/%d stats=%+v",
					sink, extracted, counter.calls, counter.bytes, stats)
			}
		})
	}
}

func TestProfilerMissingSpillHashFailsSourceBeforeWrite(t *testing.T) {
	source := t.TempDir() + "/capture.htrace"
	if err := os.WriteFile(source, []byte("capture"), 0o600); err != nil {
		t.Fatal(err)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := sink.openProfilerCapture(source); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "genuine-ordinary-sibling"}); err != nil {
		t.Fatal(err)
	}
	sink.runs[0].digest = [sha256.Size]byte{}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	extracted := profilerContainerExtraction{Detected: true, Kind: "openharmony_profiler_trace_file"}
	if err := applyProfilerCaptureSourceFailure(&extracted, sink); err != nil {
		t.Fatal(err)
	}
	counter := &profilerSpillWriteCounter{}
	stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
	if err != nil {
		t.Fatal(err)
	}
	if sink.pairAuthorityFailure != "profiler_pair_spill_integrity_mismatch" ||
		sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
		!extracted.SourceFailClosed || counter.calls != 0 || counter.bytes != 0 ||
		stats.RowsAccepted != 2 || stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
		t.Fatalf("missing spill proof reached output: reason=%q source=%q all=%t extracted=%+v writes=%d/%d stats=%+v",
			sink.pairAuthorityFailure, sink.captureSourceFailure, sink.allRowsFailClosed,
			extracted, counter.calls, counter.bytes, stats)
	}
}

func TestProfilerStorageFailureBridgePrecedesOutputOpen(t *testing.T) {
	source, err := os.ReadFile("profiler_container.go")
	if err != nil {
		t.Fatal(err)
	}
	seal := bytes.Index(source, []byte("if err := sink.sealProfilerCaptureContext(ctx)"))
	bridge := bytes.Index(source, []byte("if err := applyProfilerCaptureSourceFailure"))
	terminal := bytes.Index(source, []byte("terminal, err := applyProfilerTerminalPublication"))
	result := bytes.Index(source, []byte("result = Result{"))
	open := bytes.Index(source, []byte("out, err := os.OpenFile(output"))
	if seal < 0 || bridge <= seal || terminal <= bridge || result <= terminal || open <= result {
		t.Fatalf("terminal publication order drifted: seal=%d bridge=%d terminal=%d result=%d output-open=%d",
			seal, bridge, terminal, result, open)
	}
}

func TestProfilerSpillReadbackMissingAndDuplicateRecordsFailClosed(t *testing.T) {
	tests := []struct {
		name         string
		wantReason   string
		wantWritten  int
		wantWithheld int
		wantOrdinary bool
		corrupt      func(*testing.T, string)
	}{
		{
			name: "missing", wantReason: "profiler_pair_spill_integrity_mismatch",
			wantWritten: 0, wantWithheld: 2,
			corrupt: func(t *testing.T, path string) {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "duplicate", wantReason: "profiler_pair_spill_integrity_mismatch",
			wantWritten: 0, wantWithheld: 2,
			corrupt: func(t *testing.T, path string) {
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(append([]byte(nil), raw...), raw...), 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			if err := sink.add(profilerSpillMappingBlockRow()); err != nil {
				t.Fatal(err)
			}
			if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "ordinary-survives"}); err != nil {
				t.Fatal(err)
			}
			test.corrupt(t, sink.runs[0].path)

			var output bytes.Buffer
			stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
			requireProfilerInactiveStorageIntegrityError(t, err)
			if sink.pairAuthorityFailure != test.wantReason || stats.RowsAccepted != 2 ||
				stats.RowsWritten != test.wantWritten || stats.RowsWithheld != test.wantWithheld ||
				strings.Contains(output.String(), "ordinary-survives") != test.wantOrdinary {
				t.Fatalf("%s spill record did not fail closed: reason=%q stats=%+v\n%s",
					test.name, sink.pairAuthorityFailure, stats, output.String())
			}
		})
	}
}
