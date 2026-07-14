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

func setProfilerSpillChunkPairTuple(row *traceDBChunkRow, slot profilerPairEndpointSlot, structured bool) {
	descriptor, ok := slot.descriptor()
	if !ok {
		panic("test fixture uses an invalid profiler pair endpoint")
	}
	row.PairKind = descriptor.kind
	row.PairTable = descriptor.name
	row.StructuredPair = structured
	row.ProfilerEventField = 0
	row.ProfilerProvenance.PairKind = descriptor.kind
	row.ProfilerProvenance.EndpointSlot = slot
	row.ProfilerProvenance.Flags = 0
	if structured {
		if descriptor.structuredField == 0 {
			panic("test fixture uses a text-only endpoint as structured")
		}
		row.ProfilerEventField = descriptor.structuredField
		row.ProfilerProvenance.Flags = profilerPairRowProvenanceStructured
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
// manifest after rewriting a run. This isolates semantic row-map validation
// from the stronger physical-digest layer; ordinary tamper tests never call it.
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

func TestProfilerSpillReadbackValidatesCompletePairRowMapping(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*traceDBChunkRow)
	}{
		{name: "kind", mutate: func(row *traceDBChunkRow) {
			setProfilerSpillChunkPairTuple(row, profilerPairEndpointF2FSWriteBegin, true)
		}},
		{name: "lane", mutate: func(row *traceDBChunkRow) { row.PairLane = "other-lane" }},
		{name: "table", mutate: func(row *traceDBChunkRow) {
			setProfilerSpillChunkPairTuple(row, profilerPairEndpointBlockBIOComplete, true)
		}},
		{name: "structured", mutate: func(row *traceDBChunkRow) {
			setProfilerSpillChunkPairTuple(row, profilerPairEndpointBlockBIOQueue, false)
		}},
		{name: "event field", mutate: func(row *traceDBChunkRow) {
			setProfilerSpillChunkPairTuple(row, profilerPairEndpointBlockRQIssue, true)
		}},
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
			if len(sink.runs) != 2 {
				t.Fatalf("threshold-one fixture did not spill both rows: runs=%d", len(sink.runs))
			}
			rewriteProfilerSpillChunkRow(t, sink.runs[0].path, test.mutate)
			refreshProfilerRunProof(t, sink, 0)

			var output bytes.Buffer
			stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
			if err != nil {
				t.Fatal(err)
			}
			if sink.pairAuthorityFailure != "published_row_mapping_mismatch" ||
				sink.captureSourceFailure != "" || sink.allRowsFailClosed ||
				stats.RowsAccepted != 2 || stats.RowsWritten != 1 || stats.RowsWithheld != 1 ||
				!strings.Contains(output.String(), "ordinary-survives") || strings.Contains(output.String(), "block_bio_queue-row") {
				t.Fatalf("spill mapping drift did not close pair publication: reason=%q stats=%+v\n%s",
					sink.pairAuthorityFailure, stats, output.String())
			}
		})
	}
}

func TestProfilerSpillRejectsInternallyIncoherentTypedProvenanceSourceWide(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
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

	counter := &profilerSpillWriteCounter{}
	stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
	requireProfilerInactiveStorageIntegrityError(t, err)
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
		name       string
		sourceWide bool
		mutate     func(*profilerPairRowProvenance)
	}{
		{name: "lane id", mutate: func(value *profilerPairRowProvenance) { value.LaneID++ }},
		{name: "message ordinal", mutate: func(value *profilerPairRowProvenance) { value.TextMessageOrdinal++ }},
		{name: "pair kind", sourceWide: true, mutate: func(value *profilerPairRowProvenance) {
			value.PairKind = pairRenderMMC
		}},
		{name: "endpoint slot", sourceWide: true, mutate: func(value *profilerPairRowProvenance) {
			value.EndpointSlot = profilerPairEndpointF2FSWriteEnd
		}},
		{name: "publisher slot", mutate: func(value *profilerPairRowProvenance) {
			value.PublisherSlot = profilerPairPublisherBytrace
		}},
		{name: "flags", sourceWide: true, mutate: func(value *profilerPairRowProvenance) { value.Flags = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
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

			counter := &profilerSpillWriteCounter{}
			stats, writeErr := sink.prepareAndWriteForTest(context.Background(), counter)
			if test.sourceWide {
				requireProfilerInactiveStorageIntegrityError(t, writeErr)
				if sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
					counter.calls != 0 || counter.bytes != 0 || stats.RowsWritten != 0 || stats.RowsWithheld != 2 {
					t.Fatalf("invalid %s scalar escaped source fail-close: err=%v sink=%+v writes=%d/%d stats=%+v",
						test.name, writeErr, sink, counter.calls, counter.bytes, stats)
				}
				return
			}
			if writeErr != nil || sink.pairAuthorityFailure != "published_row_mapping_mismatch" ||
				sink.captureSourceFailure != "" || sink.allRowsFailClosed || stats.RowsWritten != 1 ||
				stats.RowsWithheld != 1 || counter.calls == 0 || counter.bytes == 0 {
				t.Fatalf("coherent %s scalar drift was not localized: err=%v authority=%q source=%q all=%t writes=%d/%d stats=%+v",
					test.name, writeErr, sink.pairAuthorityFailure, sink.captureSourceFailure,
					sink.allRowsFailClosed, counter.calls, counter.bytes, stats)
			}
		})
	}
}

func TestProfilerSealCompletesSpillMappingProofBeforePublication(t *testing.T) {
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
			row.PairLane = "drifted-at-rest"
		})
		refreshProfilerRunProof(t, sink, 0)
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		if sink.captureLifecycle != profilerCaptureSealed ||
			sink.pairAuthorityFailure != "published_row_mapping_mismatch" ||
			sink.captureSourceFailure != "" || sink.allRowsFailClosed || sink.publishableRows() != 1 {
			t.Fatalf("seal did not freeze spill mapping failure: lifecycle=%d reason=%q publishable=%d",
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
		if sink.pairAuthorityFailure != "" || len(sink.rows) != 0 || len(sink.runs) != 1 || len(sink.pairRowMappings) != 2 {
			t.Fatalf("seal did not join spill and tail proof: reason=%q runs=%d rows=%d mappings=%d",
				sink.pairAuthorityFailure, len(sink.runs), len(sink.rows), len(sink.pairRowMappings))
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
					row.PairKind = pairRenderUnknown
					row.PairLane = ""
					row.PairTable = ""
					row.StructuredPair = false
					row.ProfilerEventField = 0
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

func TestProfilerPreexistingAuthorityFailureStillDetectsSpillCompositeDrift(t *testing.T) {
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	pair := profilerSpillMappingBlockRow()
	if err := sink.add(pair); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(pair); err != nil {
		t.Fatal(err)
	}
	if err := sink.add(renderedRow{tsNS: 2, seq: 12, line: "genuine-ordinary-sibling"}); err != nil {
		t.Fatal(err)
	}
	if sink.pairAuthorityFailure != "duplicate_published_seq" || len(sink.runs) != 3 {
		t.Fatalf("fixture did not establish preexisting authority failure: reason=%q runs=%d",
			sink.pairAuthorityFailure, len(sink.runs))
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[1].path, func(row *traceDBChunkRow) {
		row.Seq = 993
		row.PairKind = pairRenderUnknown
		row.PairLane = ""
		row.PairTable = ""
		row.StructuredPair = false
		row.ProfilerEventField = 0
	})
	counter := &profilerSpillWriteCounter{}
	stats, err := sink.prepareAndWriteForTest(context.Background(), counter)
	requireProfilerInactiveStorageIntegrityError(t, err)
	if sink.pairAuthorityFailure != "duplicate_published_seq" ||
		sink.captureSourceFailure != profilerPairStorageIntegrityFailure || !sink.allRowsFailClosed ||
		counter.calls != 0 || counter.bytes != 0 || stats.RowsAccepted != 3 ||
		stats.RowsWritten != 0 || stats.RowsWithheld != 3 {
		t.Fatalf("preexisting authority let at-rest composite drift escape: reason=%q source=%q all=%t writes=%d/%d stats=%+v",
			sink.pairAuthorityFailure, sink.captureSourceFailure, sink.allRowsFailClosed, counter.calls, counter.bytes, stats)
	}
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
		row.PairKind = pairRenderUnknown
		row.PairLane = ""
		row.PairTable = ""
		row.StructuredPair = false
		row.ProfilerEventField = 0
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
		barrier.FieldSources["shared_authority_failure"] != "published_row_mapping_missing" ||
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
	open := bytes.Index(source, []byte("out, err := os.OpenFile(output"))
	if seal < 0 || bridge <= seal || open <= bridge {
		t.Fatalf("storage source-failure order drifted: seal=%d bridge=%d output-open=%d", seal, bridge, open)
	}
}

func TestProfilerSpillReadbackMissingAndDuplicateMappingsFailClosed(t *testing.T) {
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
				t.Fatalf("%s spill mapping did not fail closed: reason=%q stats=%+v\n%s",
					test.name, sink.pairAuthorityFailure, stats, output.String())
			}
		})
	}
}

func TestProfilerSharedRowAuthoritySurvivesPriorFamilyPoison(t *testing.T) {
	row := func(seq int, lane string) renderedRow {
		return renderedRow{
			tsNS: uint64(seq + 10), seq: seq, line: "block-row",
			pairKind: pairRenderBlock, pairLane: lane, pairTable: "block_bio_queue",
		}
	}
	tests := []struct {
		name       string
		wantReason string
		prepare    func(*traceDBRowSink) error
	}{
		{
			name: "unknown lane remains mappable after family poison",
			prepare: func(sink *traceDBRowSink) error {
				return sink.add(row(1, ""))
			},
		},
		{
			name:       "negative sequence",
			wantReason: "published_row_mapping_missing",
			prepare: func(sink *traceDBRowSink) error {
				return sink.add(row(-1, "lane"))
			},
		},
		{
			name:       "duplicate sequence",
			wantReason: "duplicate_published_seq",
			prepare: func(sink *traceDBRowSink) error {
				if err := sink.add(row(1, "lane")); err != nil {
					return err
				}
				return sink.add(row(1, "lane"))
			},
		},
		{
			name:       "shared capacity",
			wantReason: "shared_row_capacity",
			prepare: func(sink *traceDBRowSink) error {
				sink.pairRowCapacity = 1
				if err := sink.add(row(1, "lane-a")); err != nil {
					return err
				}
				return sink.add(row(2, "lane-b"))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink(t.TempDir(), 32)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			sink.poisonPairKind(pairRenderBlock)
			if err := test.prepare(sink); err != nil {
				t.Fatal(err)
			}
			if sink.pairAuthorityFailure != test.wantReason {
				t.Fatalf("authority reason=%q want=%q mappings=%+v", sink.pairAuthorityFailure, test.wantReason, sink.pairRowMappings)
			}
			if test.wantReason == "" {
				mapping, ok := sink.pairRowMappings[1]
				if !ok || mapping.kind != pairRenderBlock || mapping.lane != "" {
					t.Fatalf("family-poisoned unknown lane lost its expected mapping: %+v", sink.pairRowMappings)
				}
				return
			}
			for _, kind := range []pairRenderKind{
				pairRenderWorkqueue, pairRenderDMAFence, pairRenderMMC, pairRenderF2FS, pairRenderBlock,
			} {
				if !sink.poisoned[kind] {
					t.Fatalf("global authority failure left family %d open: poisoned=%v", kind, sink.poisoned)
				}
			}
		})
	}
}
