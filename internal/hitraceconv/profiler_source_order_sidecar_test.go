package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"strings"
	"testing"
)

func profilerSidecarGoldenBytes(t *testing.T, encoded string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestProfilerSourceOrderSidecarTypedDispositionKeepsOrdinarySibling(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{})
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin text publisher")
	}
	pairLine := "f2fs-pair-withheld"
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 1, seq: 1, line: pairLine,
		pairKind: pairRenderF2FS, pairLane: "f2fs-lane", pairTable: "f2fs_write_begin",
		profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.endProfilerTextMessage(1); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	ordinaryLine := "ordinary-published"
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 2, seq: 2, line: ordinaryLine})
	sink.poisonPairLane(pairRenderF2FS, "f2fs-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(sink.sourceOrderSidecar.path)
	if err != nil {
		t.Fatal(err)
	}
	dispositions := make([]profilerSourceOrderDisposition, 0, 2)
	for ordinal := 0; ordinal < 2; ordinal++ {
		start := int(profilerSourceOrderSidecarHeaderBytes) +
			ordinal*int(profilerSourceOrderSidecarRecordBytes)
		record, err := decodeProfilerSourceOrderSidecarRecord(
			raw[start : start+int(profilerSourceOrderSidecarRecordBytes)],
		)
		if err != nil {
			t.Fatal(err)
		}
		dispositions = append(dispositions, record.disposition)
	}
	if dispositions[0] != profilerSourceOrderDispositionWithhold ||
		dispositions[1] != profilerSourceOrderDispositionPublish {
		t.Fatalf("typed terminal dispositions drifted: %v", dispositions)
	}
	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil || stats.RowsWritten != 1 || stats.RowsWithheld != 1 ||
		!strings.Contains(output.String(), ordinaryLine) || strings.Contains(output.String(), pairLine) {
		t.Fatalf("typed disposition publication drifted: stats=%+v err=%v\n%s",
			stats, err, output.String())
	}
}

func TestProfilerSourceOrderSidecarZeroRowsCreatesNoArtifact(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{})
	defer sink.cleanup()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	count, root, err := sink.expectedProfilerSourceOrderProof()
	if err != nil || count != 0 || root != terminalProfilerSourceOrderDigest(0, profilerSourceOrderInitialState()) ||
		sink.sourceOrderSidecar.present() || len(sink.runs) != 0 || len(sink.artifacts) != 0 ||
		sink.activeTempBytes != 0 || sink.liveTempBytes != 0 {
		t.Fatalf("zero-row sidecar state drifted: count=%d root=%x err=%v sidecar=%+v runs=%v artifacts=%v active=%d live=%d",
			count, root, err, sink.sourceOrderSidecar, sink.runs, sink.artifacts,
			sink.activeTempBytes, sink.liveTempBytes)
	}
}

func TestProfilerSourceOrderSidecarConstructionFaultsRollback(t *testing.T) {
	for _, point := range []string{
		"sidecar_create", "sidecar_fstat", "sidecar_preallocate", "sidecar_write",
		"sidecar_close", "sidecar_stat", "sidecar_open", "sidecar_read",
	} {
		t.Run(point, func(t *testing.T) {
			want := errors.New("fault-" + point)
			fired := false
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1,
				traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(got, _ string) error {
					if got == point && !fired {
						fired = true
						return want
					}
					return nil
				}}})
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "construction-fault"})
			err := sink.sealProfilerCapture()
			if !fired || !errors.Is(err, want) || sink.sourceOrderSidecar.present() {
				t.Fatalf("%s construction fault escaped: fired=%t err=%v sidecar=%+v",
					point, fired, err, sink.sourceOrderSidecar)
			}
			if cleanupErr := sink.cleanup(); cleanupErr != nil {
				t.Fatalf("%s cleanup: %v", point, cleanupErr)
			}
			entries, readErr := os.ReadDir(sink.tempDir)
			if !errors.Is(readErr, os.ErrNotExist) && (readErr != nil || len(entries) != 0) {
				t.Fatalf("%s leaked temp state: entries=%v err=%v", point, entries, readErr)
			}
			if sink.liveTempBytes != 0 || sink.activeTempBytes != 0 || sink.openRunFDs != 0 {
				t.Fatalf("%s leaked accounting: active=%d live=%d open=%d",
					point, sink.activeTempBytes, sink.liveTempBytes, sink.openRunFDs)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarShortIOAndPreallocateFaultsRollback(t *testing.T) {
	tests := []struct {
		name string
		ops  traceDBRowSinkOps
		want error
	}{
		{
			name: "preallocate",
			want: errors.New("preallocate-sentinel"),
		},
		{name: "short-write", want: io.ErrShortWrite},
		{name: "short-read", want: io.ErrUnexpectedEOF},
	}
	for index := range tests {
		test := &tests[index]
		switch test.name {
		case "preallocate":
			test.ops.truncate = func(*os.File, int64) error { return test.want }
		case "short-write":
			test.ops.writeAt = func(_ *os.File, data []byte, _ int64) (int, error) {
				return len(data) - 1, nil
			}
		case "short-read":
			test.ops.readAt = func(_ *os.File, data []byte, _ int64) (int, error) {
				return len(data) - 1, nil
			}
		}
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1,
				traceDBRowSinkOptions{ops: test.ops})
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "short-io"})
			err := sink.sealProfilerCapture()
			if !errors.Is(err, test.want) || sink.sourceOrderSidecar.present() {
				t.Fatalf("%s err=%v want=%v sidecar=%+v", test.name, err, test.want,
					sink.sourceOrderSidecar)
			}
			if cleanupErr := sink.cleanup(); cleanupErr != nil ||
				sink.liveTempBytes != 0 || sink.openRunFDs != 0 {
				t.Fatalf("%s cleanup=%v live=%d open=%d",
					test.name, cleanupErr, sink.liveTempBytes, sink.openRunFDs)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarConstructionSemanticCorruptionStaysLocal(t *testing.T) {
	tests := []struct {
		name       string
		wantReason string
		corrupt    func(offset int64, data []byte) bool
	}{
		{
			name:       "header ABI",
			wantReason: "profiler_source_order_sidecar_header_invalid",
			corrupt: func(offset int64, data []byte) bool {
				if offset == 0 && len(data) == int(profilerSourceOrderSidecarHeaderBytes) {
					data[0] ^= 0x80
					return true
				}
				return false
			},
		},
		{
			name:       "record reserved byte",
			wantReason: "profiler_source_order_sidecar_record_invalid",
			corrupt: func(offset int64, data []byte) bool {
				if offset >= int64(profilerSourceOrderSidecarHeaderBytes) &&
					len(data) >= int(profilerSourceOrderSidecarRecordBytes) {
					data[53] = 1
					return true
				}
				return false
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			corrupted := false
			options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{writeAt: func(
				file *os.File, data []byte, offset int64,
			) (int, error) {
				writeData := data
				if !corrupted {
					writeData = append([]byte(nil), data...)
					corrupted = test.corrupt(offset, writeData)
				}
				return file.WriteAt(writeData, offset)
			}}}
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, options)
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "construction-semantic-corruption"})
			err := sink.sealProfilerCapture()
			reason, ok := traceDBOutputInvariantReason(err)
			if !corrupted || !ok || reason != test.wantReason ||
				traceDBRunInputIntegrityOnly(err) || sink.captureSourceFailure != "" ||
				sink.prepared || sink.sourceOrderSidecar.present() {
				t.Fatalf("construction corruption was laundered: corrupted=%t reason=%q ok=%t only=%t source=%q prepared=%t sidecar=%+v err=%v",
					corrupted, reason, ok, traceDBRunInputIntegrityOnly(err),
					sink.captureSourceFailure, sink.prepared, sink.sourceOrderSidecar, err)
			}
			if err := sink.cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarCombinedTempBudgetsFailAtomically(t *testing.T) {
	for _, budget := range []string{"active", "live"} {
		t.Run(budget, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "sidecar-budget"})
			if err := sink.flushChunkContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			size, err := profilerSourceOrderSidecarSize(1)
			if err != nil {
				t.Fatal(err)
			}
			beforeActive := sink.activeTempBytes
			beforeLive := sink.liveTempBytes
			wantReason := "trace_row_sort_active_temp_budget_exceeded"
			if budget == "active" {
				sink.options.activeTempCap = beforeActive + size - 1
				sink.options.liveTempCap = math.MaxUint64
			} else {
				sink.options.activeTempCap = math.MaxUint64
				sink.options.liveTempCap = beforeLive + size - 1
				wantReason = "trace_row_sort_live_temp_budget_exceeded"
			}
			err = sink.sealProfilerCapture()
			reason, ok := traceDBOutputInvariantReason(err)
			if !ok || reason != wantReason || sink.sourceOrderSidecar.present() ||
				sink.activeTempBytes != beforeActive || sink.liveTempBytes != beforeLive ||
				sink.stats.SourceSidecarLogicalBytes != 0 ||
				sink.stats.SourceSidecarPhysicalBytes != 0 || sink.captureSourceFailure != "" {
				t.Fatalf("%s budget was not atomic: reason=%q ok=%t err=%v active=%d/%d live=%d/%d sidecar=%+v stats=%+v source=%q",
					budget, reason, ok, err, sink.activeTempBytes, beforeActive,
					sink.liveTempBytes, beforeLive, sink.sourceOrderSidecar, sink.stats,
					sink.captureSourceFailure)
			}
			if err := sink.cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarPublicationFaultsPrecedeHeader(t *testing.T) {
	for _, point := range []string{"sidecar_open", "sidecar_fstat", "sidecar_read", "open", "fstat", "read", "seek"} {
		t.Run(point, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1,
				traceDBRowSinkOptions{})
			defer sink.cleanup()
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "publication-fault"})
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			want := errors.New("publication-" + point)
			fired := false
			sink.options.ops.fault = func(got, _ string) error {
				if got == point && !fired {
					fired = true
					return want
				}
				return nil
			}
			var output bytes.Buffer
			_, err := sink.writeTo(context.Background(), &output)
			if !fired || !errors.Is(err, want) || output.Len() != 0 {
				t.Fatalf("%s publication fault escaped: fired=%t err=%v bytes=%d",
					point, fired, err, output.Len())
			}
		})
	}
}

func TestProfilerSourceOrderSidecarPublicationMixedCloseFailureStaysHard(t *testing.T) {
	tests := []struct {
		name         string
		primaryPoint string
		closePoint   string
	}{
		{name: "sidecar fstat and close", primaryPoint: "sidecar_fstat", closePoint: "sidecar_close"},
		{name: "run fstat and close", primaryPoint: "fstat", closePoint: "close"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
			defer sink.cleanup()
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "publication-mixed-close"})
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			primaryErr := errors.New("publication-mixed-primary")
			closeErr := errors.New("publication-mixed-close")
			primaryFired := false
			closeFired := false
			sink.options.ops.fault = func(point, _ string) error {
				if point == test.primaryPoint && !primaryFired {
					primaryFired = true
					return primaryErr
				}
				if point == test.closePoint && !closeFired {
					closeFired = true
					return closeErr
				}
				return nil
			}
			var output bytes.Buffer
			_, err := sink.writeTo(context.Background(), &output)
			if !primaryFired || !closeFired || !errors.Is(err, primaryErr) ||
				!errors.Is(err, closeErr) || traceDBRunInputIntegrityOnly(err) ||
				output.Len() != 0 || sink.captureSourceFailure != "" {
				t.Fatalf("mixed publication error was laundered: primary=%t close=%t only=%t source=%q err=%v bytes=%d",
					primaryFired, closeFired, traceDBRunInputIntegrityOnly(err),
					sink.captureSourceFailure, err, output.Len())
			}
		})
	}
}

func TestProfilerSourceOrderSidecarCancellationPreservesSentinel(t *testing.T) {
	for _, phase := range []string{"before-build", "during-write", "during-readback", "final-fstat"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			fstatCalls := 0
			fault := func(point, _ string) error {
				if phase == "during-write" && point == "sidecar_write" ||
					phase == "during-readback" && point == "sidecar_read" {
					cancel()
				}
				if phase == "final-fstat" && point == "sidecar_fstat" {
					fstatCalls++
					if fstatCalls == 3 {
						cancel()
					}
				}
				return nil
			}
			sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1,
				traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: fault}})
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "cancel-sidecar"})
			if phase == "before-build" {
				cancel()
			}
			err := sink.sealProfilerCaptureContext(ctx)
			if !errors.Is(err, context.Canceled) || sink.sourceOrderSidecar.present() {
				t.Fatalf("%s cancellation identity/state drifted: err=%v sidecar=%+v",
					phase, err, sink.sourceOrderSidecar)
			}
			if cleanupErr := sink.cleanup(); cleanupErr != nil || sink.liveTempBytes != 0 || sink.openRunFDs != 0 {
				t.Fatalf("%s cleanup=%v live=%d open=%d",
					phase, cleanupErr, sink.liveTempBytes, sink.openRunFDs)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarPublicationFinalCancellationPreservesSentinel(t *testing.T) {
	tests := []struct {
		name  string
		point string
	}{
		{name: "final audit fstat", point: "sidecar_fstat"},
		{name: "run close", point: "close"},
		{name: "sidecar close", point: "sidecar_close"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
			defer sink.cleanup()
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "publication-final-cancel"})
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			calls := 0
			sink.options.ops.fault = func(point, _ string) error {
				if point != test.point {
					return nil
				}
				calls++
				if test.point != "sidecar_fstat" || calls == 4 {
					cancel()
				}
				return nil
			}
			var output bytes.Buffer
			_, err := sink.writeTo(ctx, &output)
			if !errors.Is(err, context.Canceled) || output.Len() != 0 ||
				(test.point == "sidecar_fstat" && calls != 4) || calls == 0 {
				t.Fatalf("%s cancellation drifted: calls=%d err=%v bytes=%d",
					test.name, calls, err, output.Len())
			}
		})
	}
}

func TestProfilerSourceOrderSidecarWireGolden(t *testing.T) {
	var root [sha256.Size]byte
	var runDigest [sha256.Size]byte
	var leaf [sha256.Size]byte
	for index := range root {
		root[index] = byte(index)
		runDigest[index] = byte(0xa0 + index)
		leaf[index] = byte(0x20 + index)
	}
	header := encodeProfilerSourceOrderSidecarHeader(profilerSourceOrderSidecarHeader{
		rowCount: 2, producerRoot: root, boundRunDigest: runDigest,
	})
	wantHeader := profilerSidecarGoldenBytes(t,
		"636f647261782d70736f2d7369646500"+
			"0100010060003800"+
			"0200000000000000"+
			"000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"+
			"a0a1a2a3a4a5a6a7a8a9aaabacadaeafb0b1b2b3b4b5b6b7b8b9babbbcbdbebf")
	if !bytes.Equal(header[:], wantHeader) || len(header) != 96 {
		t.Fatalf("header ABI drifted:\n got=%x\nwant=%x", header, wantHeader)
	}
	decodedHeader, err := decodeProfilerSourceOrderSidecarHeader(header[:])
	if err != nil || decodedHeader.rowCount != 2 || decodedHeader.producerRoot != root ||
		decodedHeader.boundRunDigest != runDigest {
		t.Fatalf("header roundtrip drifted: decoded=%+v err=%v", decodedHeader, err)
	}

	provenance := profilerPairRowProvenance{
		LaneID: 0x11223344, TextMessageOrdinal: 0x55667788,
		PairKind: pairRenderF2FS, EndpointSlot: profilerPairEndpointF2FSWriteBegin,
		PublisherSlot: profilerPairPublisherExactFtrace,
		Flags:         profilerPairRowProvenanceText,
	}
	record, err := encodeProfilerSourceOrderSidecarRecord(
		7, leaf, provenance, profilerSourceOrderDispositionWithhold,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantRecord := profilerSidecarGoldenBytes(t,
		"0800000000000000"+
			"202122232425262728292a2b2c2d2e2f303132333435363738393a3b3c3d3e3f"+
			"4433221188776655"+
			"0407010102000000")
	if !bytes.Equal(record[:], wantRecord) || len(record) != 56 {
		t.Fatalf("record ABI drifted:\n got=%x\nwant=%x", record, wantRecord)
	}
	decodedRecord, err := decodeProfilerSourceOrderSidecarRecord(record[:])
	if err != nil || decodedRecord.ordinalPlusOne != 8 || decodedRecord.leaf != leaf ||
		decodedRecord.provenance != provenance ||
		decodedRecord.disposition != profilerSourceOrderDispositionWithhold {
		t.Fatalf("record roundtrip drifted: decoded=%+v err=%v", decodedRecord, err)
	}
	partial := record
	partial[52] = byte(profilerSourceOrderDispositionInvalid)
	if _, err := decodeProfilerSourceOrderSidecarRecord(partial[:]); err == nil {
		t.Fatal("zero/unwritten disposition was accepted")
	}
	reserved := record
	reserved[55] = 1
	if _, err := decodeProfilerSourceOrderSidecarRecord(reserved[:]); err == nil {
		t.Fatal("nonzero reserved byte was accepted")
	}
}

func addProfilerSidecarOrdinaryRow(t testing.TB, sink *traceDBRowSink, row renderedRow) {
	t.Helper()
	if err := sink.addProfilerEventContext(context.Background(), row, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
}

func rewriteProfilerSpillChunkRows(
	t *testing.T,
	path string,
	mutate func([]traceDBChunkRow),
) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(raw), []byte{'\n'})
	rows := make([]traceDBChunkRow, len(lines))
	for index := range lines {
		if err := json.Unmarshal(lines[index], &rows[index]); err != nil {
			t.Fatalf("decode spill row %d: %v\n%s", index, err, lines[index])
		}
	}
	mutate(rows)
	var rewritten bytes.Buffer
	encoder := json.NewEncoder(&rewritten)
	for index := range rows {
		if err := encoder.Encode(rows[index]); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(path, rewritten.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestProfilerSourceOrderSidecarReplaysReverseTimestampRowsAndCleansUp(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{})
	defer sink.cleanup()
	first := renderedRow{tsNS: 20, seq: 10, line: "source-first-ts-late"}
	second := renderedRow{tsNS: 10, seq: 11, line: "source-second-ts-early"}
	addProfilerSidecarOrdinaryRow(t, sink, first)
	addProfilerSidecarOrdinaryRow(t, sink, second)
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	manifest := sink.sourceOrderSidecar
	wantSize := uint64(profilerSourceOrderSidecarHeaderBytes) +
		2*uint64(profilerSourceOrderSidecarRecordBytes)
	if sink.captureLifecycle != profilerCaptureSealed || !manifest.present() ||
		manifest.size != wantSize || manifest.rowCount != 2 || len(sink.runs) != 1 ||
		manifest.boundRunDigest != sink.runs[0].digest ||
		sink.liveTempBytes != sink.activeTempBytes+wantSize ||
		sink.stats.CurrentLiveTempBytes != sink.liveTempBytes ||
		sink.stats.SourceSidecarLogicalBytes != wantSize ||
		sink.stats.SourceSidecarPhysicalBytes != wantSize {
		t.Fatalf("prepared sidecar state drifted: manifest=%+v runs=%+v active=%d live=%d stats=%+v",
			manifest, sink.runs, sink.activeTempBytes, sink.liveTempBytes, sink.stats)
	}
	raw, err := os.ReadFile(manifest.path)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(raw)) != wantSize || sha256.Sum256(raw) != manifest.digest {
		t.Fatalf("sidecar physical seal drifted: bytes=%d manifest=%+v", len(raw), manifest)
	}
	leafBuilder := newProfilerSourceOrderLeafBuilder()
	for ordinal, row := range []renderedRow{first, second} {
		start := int(profilerSourceOrderSidecarHeaderBytes) +
			ordinal*int(profilerSourceOrderSidecarRecordBytes)
		record, err := decodeProfilerSourceOrderSidecarRecord(
			raw[start : start+int(profilerSourceOrderSidecarRecordBytes)],
		)
		if err != nil {
			t.Fatal(err)
		}
		wantLeaf, err := leafBuilder.leafContext(context.Background(), row, uint64(ordinal))
		if err != nil || record.ordinalPlusOne != uint64(ordinal+1) || record.leaf != wantLeaf ||
			record.provenance != row.profilerProvenance() ||
			record.disposition != profilerSourceOrderDispositionPublish {
			t.Fatalf("source slot %d drifted: record=%+v leaf=%x err=%v", ordinal, record, wantLeaf, err)
		}
	}

	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if stats.RowsWritten != 2 || stats.RowsWithheld != 0 ||
		strings.Index(text, second.line) < 0 || strings.Index(text, first.line) < 0 ||
		strings.Index(text, second.line) > strings.Index(text, first.line) ||
		sink.liveTempBytes != 0 || sink.activeTempBytes != 0 || sink.openRunFDs != 0 ||
		sink.sourceOrderSidecar.present() {
		t.Fatalf("publication/cleanup drifted: stats=%+v active=%d live=%d open=%d sidecar=%+v\n%s",
			stats, sink.activeTempBytes, sink.liveTempBytes, sink.openRunFDs,
			sink.sourceOrderSidecar, text)
	}
	coverage := stats.coverage()
	if coverage.FieldSources["profiler_source_order_sidecar"] !=
		"208_logical_bytes+208_physical_bytes" {
		t.Fatalf("sidecar disclosure missing: %+v", coverage.FieldSources)
	}
}

func TestProfilerSourceOrderSidecarRejectsLateRunDigestRefreshBeforeHeader(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{})
	defer sink.cleanup()
	addProfilerSidecarOrdinaryRow(t, sink, renderedRow{tsNS: 1, seq: 1, line: "ordinary-before-tamper"})
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	bound := sink.sourceOrderSidecar.boundRunDigest
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.Line = "ordinary-after-tamper"
	})
	refreshProfilerRunProof(t, sink, 0)
	if sink.runs[0].digest == bound {
		t.Fatal("late-tamper fixture did not change the final-run digest")
	}
	var output bytes.Buffer
	_, err := sink.writeTo(context.Background(), &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("late refreshed run escaped before-header gate: err=%v bytes=%d output=%q",
			err, output.Len(), output.String())
	}
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_source_order_sidecar_bound_run_mismatch" {
		t.Fatalf("late refreshed run reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func refreshProfilerSourceOrderSidecarPhysicalDigest(t *testing.T, sink *traceDBRowSink) {
	t.Helper()
	raw, err := os.ReadFile(sink.sourceOrderSidecar.path)
	if err != nil {
		t.Fatal(err)
	}
	sink.sourceOrderSidecar.digest = sha256.Sum256(raw)
}

func TestProfilerSourceOrderSidecarRejectsRefreshedPhysicalTamperBeforeHeader(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]byte)
		reason string
	}{
		{
			name:   "header bound run digest",
			mutate: func(raw []byte) { raw[64] ^= 0x40 },
			reason: "profiler_source_order_sidecar_header_proof_mismatch",
		},
		{
			name: "record canonical leaf",
			mutate: func(raw []byte) {
				raw[int(profilerSourceOrderSidecarHeaderBytes)+8] ^= 0x40
			},
			reason: "profiler_source_order_sidecar_root_mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 1, traceDBRowSinkOptions{})
			defer sink.cleanup()
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "ordinary-sidecar-tamper"})
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(sink.sourceOrderSidecar.path)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(raw)
			if err := os.WriteFile(sink.sourceOrderSidecar.path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			refreshProfilerSourceOrderSidecarPhysicalDigest(t, sink)
			var output bytes.Buffer
			_, err = sink.writeTo(context.Background(), &output)
			if err == nil || output.Len() != 0 {
				t.Fatalf("refreshed sidecar tamper escaped: err=%v bytes=%d", err, output.Len())
			}
			reason, ok := traceDBOutputInvariantReason(err)
			if !ok || reason != test.reason {
				t.Fatalf("tamper reason=%q ok=%t want=%q err=%v", reason, ok, test.reason, err)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarBuildRejectsRefreshedRunSemanticDrift(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
	defer sink.cleanup()
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 1, seq: 1, line: "ordinary-before-build-tamper"})
	if err := sink.flushChunkContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.runs) != 1 {
		t.Fatalf("fixture final run count=%d", len(sink.runs))
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.Line = "ordinary-after-build-tamper"
	})
	refreshProfilerRunProof(t, sink, 0)

	// The registered run is physically self-consistent but disagrees with the
	// producer-frozen root. Profiler must surface a source-wide empty result,
	// not misclassify this as a local sidecar construction failure.
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatalf("semantic run drift was not fail-closed: %v", err)
	}
	if !sink.prepared || !sink.allRowsFailClosed ||
		sink.captureSourceFailure != profilerPairStorageIntegrityFailure ||
		sink.stats.RowsWritten != 0 || sink.stats.RowsWithheld != 1 ||
		sink.sourceOrderSidecar.present() {
		t.Fatalf("semantic run drift state: prepared=%t failClosed=%t source=%q stats=%+v sidecar=%+v",
			sink.prepared, sink.allRowsFailClosed, sink.captureSourceFailure,
			sink.stats, sink.sourceOrderSidecar)
	}
}

func TestProfilerSourceOrderSidecarOrdinalCorruptionFailsClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]traceDBChunkRow)
	}{
		{
			name: "duplicate leaves a hole",
			mutate: func(rows []traceDBChunkRow) {
				rows[1].IngestOrdinal = rows[0].IngestOrdinal
			},
		},
		{
			name: "out of range",
			mutate: func(rows []traceDBChunkRow) {
				rows[1].IngestOrdinal = uint64(len(rows))
			},
		},
		{
			name: "swapped source identity",
			mutate: func(rows []traceDBChunkRow) {
				rows[0].IngestOrdinal, rows[1].IngestOrdinal =
					rows[1].IngestOrdinal, rows[0].IngestOrdinal
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
			defer sink.cleanup()
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "ordinal-first"})
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 2, seq: 2, line: "ordinal-second"})
			if err := sink.flushChunkContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(sink.runs) != 1 {
				t.Fatalf("fixture final run count=%d", len(sink.runs))
			}
			rewriteProfilerSpillChunkRows(t, sink.runs[0].path, test.mutate)
			refreshProfilerRunProof(t, sink, 0)
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatalf("ordinal corruption was not fail-closed: %v", err)
			}
			if !sink.allRowsFailClosed ||
				sink.captureSourceFailure != profilerPairStorageIntegrityFailure ||
				sink.stats.RowsWithheld != 2 || sink.sourceOrderSidecar.present() {
				t.Fatalf("ordinal corruption state: source=%q stats=%+v sidecar=%+v",
					sink.captureSourceFailure, sink.stats, sink.sourceOrderSidecar)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarSourceWideDispositionIsRecorded(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
	defer sink.cleanup()
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 1, seq: 1, line: "source-wide-withheld"})
	// Model a source-wide authority verdict discovered before registered-run
	// validation. The run itself remains authentic, so B-c must record the
	// terminal WITHHOLD bit rather than skipping its sidecar.
	sink.markRegisteredRunStorageIntegrityFailure()
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	if !sink.sourceOrderSidecar.present() || sink.stats.RowsWithheld != 1 {
		t.Fatalf("source-wide sidecar missing: sidecar=%+v stats=%+v",
			sink.sourceOrderSidecar, sink.stats)
	}
	raw, err := os.ReadFile(sink.sourceOrderSidecar.path)
	if err != nil {
		t.Fatal(err)
	}
	record, err := decodeProfilerSourceOrderSidecarRecord(
		raw[int(profilerSourceOrderSidecarHeaderBytes):],
	)
	if err != nil || record.disposition != profilerSourceOrderDispositionWithhold {
		t.Fatalf("source-wide disposition=%v err=%v", record.disposition, err)
	}
}

func TestProfilerSourceOrderSidecarMixedSemanticAndCleanupFailureStaysHard(t *testing.T) {
	wantRemove := errors.New("sidecar-remove-after-semantic-mismatch")
	armed := false
	fired := false
	sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 8,
		traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(point, _ string) error {
			if armed && !fired && point == "remove" {
				fired = true
				return wantRemove
			}
			return nil
		}}})
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 1, seq: 1, line: "mixed-error-before"})
	if err := sink.flushChunkContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	rewriteProfilerSpillChunkRow(t, sink.runs[0].path, func(row *traceDBChunkRow) {
		row.Line = "mixed-error-after"
	})
	refreshProfilerRunProof(t, sink, 0)
	armed = true
	err := sink.sealProfilerCapture()
	if !fired || !errors.Is(err, wantRemove) || traceDBRunInputIntegrityOnly(err) ||
		sink.captureSourceFailure != "" || sink.prepared || sink.sourceOrderSidecar.present() {
		t.Fatalf("mixed error was laundered: fired=%t only=%t err=%v failClosed=%t source=%q prepared=%t sidecar=%+v",
			fired, traceDBRunInputIntegrityOnly(err), err, sink.allRowsFailClosed,
			sink.captureSourceFailure, sink.prepared, sink.sourceOrderSidecar)
	}
	if cleanupErr := sink.cleanup(); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
}

func TestTraceDBRunInputIntegrityRejectsNestedMixedGraph(t *testing.T) {
	integrity := traceDBRunInputIntegrity(
		&traceDBOutputInvariantError{Reason: "nested-integrity"},
	)
	cleanup := traceDBSorterOperationError("remove", errors.New("nested-cleanup"))
	mixed := errors.Join(integrity, cleanup)
	if traceDBRunInputIntegrityOnly(mixed) {
		t.Fatal("mixed graph was classified as integrity-only")
	}
	if got := traceDBRunInputIntegrity(mixed); got != mixed || traceDBRunInputIntegrityOnly(got) {
		t.Fatalf("mixed graph was rewrapped: got=%T %v", got, got)
	}
	forcedNested := &traceDBRunInputIntegrityError{cause: mixed}
	if traceDBRunInputIntegrityOnly(forcedNested) {
		t.Fatalf("nested mixed graph was laundered: %v", forcedNested)
	}
	wrapped := errors.Join(nil, integrity)
	if !traceDBRunInputIntegrityOnly(wrapped) {
		t.Fatalf("single wrapped integrity lost classification: %v", wrapped)
	}
}

func TestProfilerSourceOrderSidecarManifestTamperPrecedesHeader(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*profilerSourceOrderSidecarManifest)
	}{
		{name: "size", mutate: func(manifest *profilerSourceOrderSidecarManifest) { manifest.size++ }},
		{name: "count", mutate: func(manifest *profilerSourceOrderSidecarManifest) { manifest.rowCount++ }},
		{name: "producer root", mutate: func(manifest *profilerSourceOrderSidecarManifest) {
			manifest.producerRoot[0] ^= 0x80
		}},
		{name: "bound run digest", mutate: func(manifest *profilerSourceOrderSidecarManifest) {
			manifest.boundRunDigest[0] ^= 0x80
		}},
		{name: "physical digest", mutate: func(manifest *profilerSourceOrderSidecarManifest) {
			manifest.digest[0] ^= 0x80
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
			defer sink.cleanup()
			addProfilerSidecarOrdinaryRow(t, sink,
				renderedRow{tsNS: 1, seq: 1, line: "manifest-tamper"})
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			test.mutate(&sink.sourceOrderSidecar)
			var output bytes.Buffer
			_, err := sink.writeTo(context.Background(), &output)
			if err == nil || output.Len() != 0 {
				t.Fatalf("manifest tamper escaped: err=%v bytes=%d", err, output.Len())
			}
		})
	}
}

func TestProfilerSourceOrderSidecarDispositionSwapPreservingTotalsIsRejected(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
	defer sink.cleanup()
	if !sink.beginPairRowCensusForPublisher(profilerPairPublisherOtherText) ||
		!sink.beginProfilerTextMessage() {
		t.Fatal("begin pair fixture")
	}
	if err := sink.addProfilerEventContext(context.Background(), renderedRow{
		tsNS: 1, seq: 1, line: "disposition-withheld",
		pairKind: pairRenderF2FS, pairLane: "disposition-lane", pairTable: "f2fs_write_begin",
		profilerEndpointSlot: profilerPairEndpointF2FSWriteBegin,
	}, traceDBProfilerEventDelta{}); err != nil {
		t.Fatal(err)
	}
	if err := sink.endProfilerTextMessage(1); err != nil {
		t.Fatal(err)
	}
	_ = sink.endPairRowCensus()
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 2, seq: 2, line: "disposition-published"})
	sink.poisonPairLane(pairRenderF2FS, "disposition-lane")
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(sink.sourceOrderSidecar.path)
	if err != nil {
		t.Fatal(err)
	}
	first := int(profilerSourceOrderSidecarHeaderBytes) + 52
	second := first + int(profilerSourceOrderSidecarRecordBytes)
	raw[first], raw[second] = raw[second], raw[first]
	if err := os.WriteFile(sink.sourceOrderSidecar.path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	refreshProfilerSourceOrderSidecarPhysicalDigest(t, sink)
	var output bytes.Buffer
	_, err = sink.writeTo(context.Background(), &output)
	if err == nil || output.Len() != 0 {
		t.Fatalf("disposition swap escaped: err=%v bytes=%d", err, output.Len())
	}
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_source_order_sidecar_disposition_mismatch" {
		t.Fatalf("disposition swap reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerSourceOrderSidecarCommittedCleanupIsRetryable(t *testing.T) {
	t.Run("sidecar remove", func(t *testing.T) {
		armed := false
		fired := false
		want := errors.New("committed-sidecar-remove")
		var sidecarPath string
		sink := newProfilerSourceLifecycleCapture(t, profilerSourceLifecycleFile(t), 8,
			traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(point, path string) error {
				if armed && !fired && point == "remove" && path == sidecarPath {
					fired = true
					return want
				}
				return nil
			}}})
		addProfilerSidecarOrdinaryRow(t, sink,
			renderedRow{tsNS: 1, seq: 1, line: "cleanup-sidecar"})
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		manifest := sink.sourceOrderSidecar
		sidecarPath = manifest.path
		live := sink.liveTempBytes
		armed = true
		if err := sink.cleanup(); !errors.Is(err, want) {
			t.Fatalf("cleanup err=%v want=%v", err, want)
		}
		artifact := sink.artifacts[sidecarPath]
		if !fired || sink.sourceOrderSidecar != manifest || sink.liveTempBytes != live ||
			artifact == nil || artifact.removed || sink.profilerSourceProof.retired ||
			!sink.profilerSourceProof.frozen {
			t.Fatalf("failed cleanup half-cleared sidecar: fired=%t manifest=%+v live=%d/%d artifact=%+v proof=%+v",
				fired, sink.sourceOrderSidecar, sink.liveTempBytes, live, artifact,
				sink.profilerSourceProof)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		if sink.sourceOrderSidecar.present() || sink.liveTempBytes != 0 ||
			!sink.profilerSourceProof.retired {
			t.Fatalf("cleanup retry incomplete: sidecar=%+v live=%d proof=%+v",
				sink.sourceOrderSidecar, sink.liveTempBytes, sink.profilerSourceProof)
		}
	})

	t.Run("remove all", func(t *testing.T) {
		armed := false
		fired := false
		want := errors.New("committed-sidecar-remove-all")
		options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{fault: func(point, _ string) error {
			if armed && !fired && point == "remove_all" {
				fired = true
				return want
			}
			return nil
		}}}
		sink, err := newTraceDBRowSinkWithOptions("", 8, options)
		if err != nil {
			t.Fatal(err)
		}
		if err := sink.openProfilerCapture(profilerSourceLifecycleFile(t)); err != nil {
			t.Fatal(err)
		}
		addProfilerSidecarOrdinaryRow(t, sink,
			renderedRow{tsNS: 1, seq: 1, line: "cleanup-remove-all"})
		if err := sink.sealProfilerCapture(); err != nil {
			t.Fatal(err)
		}
		manifest := sink.sourceOrderSidecar
		live := sink.liveTempBytes
		ownDir := sink.ownDir
		armed = true
		if err := sink.cleanup(); !errors.Is(err, want) {
			t.Fatalf("cleanup err=%v want=%v", err, want)
		}
		if !fired || sink.sourceOrderSidecar != manifest || sink.liveTempBytes != live ||
			sink.profilerSourceProof.retired {
			t.Fatalf("remove-all failure half-cleared state: manifest=%+v live=%d/%d proof=%+v",
				sink.sourceOrderSidecar, sink.liveTempBytes, live, sink.profilerSourceProof)
		}
		if _, err := os.Stat(ownDir); err != nil {
			t.Fatalf("remove-all fault removed own dir: %v", err)
		}
		if err := sink.cleanup(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(ownDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retry did not remove own dir: %v", err)
		}
	})
}

func TestProfilerSourceOrderSidecarPublicationReusesAuthenticatedFDs(t *testing.T) {
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 8, traceDBRowSinkOptions{})
	defer sink.cleanup()
	const originalLine = "same-fd-original-row"
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 1, seq: 1, line: originalLine})
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	runPath := sink.runs[0].path
	sidecarPath := sink.sourceOrderSidecar.path
	replaced := false
	sink.options.ops.fault = func(point, _ string) error {
		if point != "seek" || replaced {
			return nil
		}
		replaced = true
		for _, replacement := range []struct {
			path string
			data []byte
		}{
			{path: runPath, data: []byte("replacement-run-must-not-be-opened\n")},
			{path: sidecarPath, data: []byte("replacement-sidecar-must-not-be-opened")},
		} {
			if err := os.Remove(replacement.path); err != nil {
				return err
			}
			if err := os.WriteFile(replacement.path, replacement.data, 0o600); err != nil {
				return err
			}
		}
		return nil
	}
	var output bytes.Buffer
	stats, err := sink.writeTo(context.Background(), &output)
	if err != nil || !replaced || stats.RowsWritten != 1 ||
		!strings.Contains(output.String(), originalLine) ||
		strings.Contains(output.String(), "replacement") {
		t.Fatalf("same-FD publication drifted: replaced=%t stats=%+v err=%v output=%q",
			replaced, stats, err, output.String())
	}
}

func TestProfilerSourceOrderSidecarAppendDuringValidationPrecedesHeader(t *testing.T) {
	armed := false
	fired := false
	options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{readAt: func(
		file *os.File, data []byte, offset int64,
	) (int, error) {
		read, err := file.ReadAt(data, offset)
		if armed && !fired {
			fired = true
			appendFile, openErr := os.OpenFile(file.Name(), os.O_WRONLY|os.O_APPEND, 0)
			if openErr != nil {
				return read, openErr
			}
			_, writeErr := appendFile.Write([]byte{0xa5})
			closeErr := appendFile.Close()
			if writeErr != nil {
				return read, writeErr
			}
			if closeErr != nil {
				return read, closeErr
			}
		}
		return read, err
	}}}
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), 8, options)
	defer sink.cleanup()
	addProfilerSidecarOrdinaryRow(t, sink,
		renderedRow{tsNS: 1, seq: 1, line: "append-during-validation"})
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	armed = true
	var output bytes.Buffer
	_, err := sink.writeTo(context.Background(), &output)
	if !fired || err == nil || output.Len() != 0 {
		t.Fatalf("sidecar append escaped EOF proof: fired=%t err=%v bytes=%d",
			fired, err, output.Len())
	}
	reason, ok := traceDBOutputInvariantReason(err)
	if !ok || reason != "profiler_source_order_sidecar_size_mismatch" {
		t.Fatalf("append reason=%q ok=%t err=%v", reason, ok, err)
	}
}

func TestProfilerSourceOrderSidecarScatterCoalescesContiguousOrdinals(t *testing.T) {
	rows := 2*profilerSourceOrderSidecarPageRecords + 1
	for _, reverse := range []bool{false, true} {
		name := "ascending"
		if reverse {
			name = "descending"
		}
		t.Run(name, func(t *testing.T) {
			writeCalls := 0
			var writtenBytes uint64
			options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{writeAt: func(
				file *os.File, data []byte, offset int64,
			) (int, error) {
				writeCalls++
				writtenBytes += uint64(len(data))
				return file.WriteAt(data, offset)
			}}}
			sink := newProfilerSourceLifecycleCapture(
				t, profilerSourceLifecycleFile(t), rows+1, options)
			defer sink.cleanup()
			for index := 0; index < rows; index++ {
				ts := uint64(index)
				if reverse {
					ts = uint64(rows - index)
				}
				addProfilerSidecarOrdinaryRow(t, sink,
					renderedRow{tsNS: ts, seq: index, line: "coalesced-row"})
			}
			if err := sink.sealProfilerCapture(); err != nil {
				t.Fatal(err)
			}
			batches := (rows + profilerSourceOrderSidecarPageRecords - 1) /
				profilerSourceOrderSidecarPageRecords
			maxCalls := 1 + 2*batches // header + at most two ring slices per batch.
			if writeCalls > maxCalls || writtenBytes != sink.sourceOrderSidecar.size {
				t.Fatalf("scatter amplification: calls=%d max=%d bytes=%d size=%d",
					writeCalls, maxCalls, writtenBytes, sink.sourceOrderSidecar.size)
			}
		})
	}
}

func TestProfilerSourceOrderSidecarInterleavedLookupHasBoundedReadAmplification(t *testing.T) {
	rows := 2 * profilerSourceOrderSidecarPageRecords
	measure := false
	readCalls := 0
	var readBytes uint64
	options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{readAt: func(
		file *os.File, data []byte, offset int64,
	) (int, error) {
		if measure {
			readCalls++
			readBytes += uint64(len(data))
		}
		return file.ReadAt(data, offset)
	}}}
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), rows+1, options)
	defer sink.cleanup()
	for ordinal := 0; ordinal < rows; ordinal++ {
		var ts int
		if ordinal < profilerSourceOrderSidecarPageRecords {
			ts = 2 * ordinal
		} else {
			ts = 2*(ordinal-profilerSourceOrderSidecarPageRecords) + 1
		}
		addProfilerSidecarOrdinaryRow(t, sink,
			renderedRow{tsNS: uint64(ts), seq: ordinal, line: "interleaved-row"})
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	sidecarSize := sink.sourceOrderSidecar.size
	measure = true
	stats, err := sink.writeTo(context.Background(), io.Discard)
	if err != nil || stats.RowsWritten != rows {
		t.Fatalf("interleaved publication: stats=%+v err=%v", stats, err)
	}
	// Publication performs three complete fixed-page audits and two row-order
	// passes. Non-local rows must use exact 56-byte reads; a one-page-thrash
	// implementation exceeds this bound by orders of magnitude.
	if readBytes > 8*sidecarSize || readCalls > 2*rows+32 {
		t.Fatalf("interleaved read amplification: calls=%d rows=%d bytes=%d sidecar=%d",
			readCalls, rows, readBytes, sidecarSize)
	}
}

func TestProfilerSourceOrderSidecarAdjacentPairThrashHasBoundedReadAmplification(t *testing.T) {
	pages := 4
	rows := pages * profilerSourceOrderSidecarPageRecords
	measure := false
	readCalls := 0
	var readBytes uint64
	options := traceDBRowSinkOptions{ops: traceDBRowSinkOps{readAt: func(
		file *os.File, data []byte, offset int64,
	) (int, error) {
		if measure {
			readCalls++
			readBytes += uint64(len(data))
		}
		return file.ReadAt(data, offset)
	}}}
	sink := newProfilerSourceLifecycleCapture(
		t, profilerSourceLifecycleFile(t), rows+1, options)
	defer sink.cleanup()
	timestampByOrdinal := make([]uint64, rows)
	position := uint64(0)
	for offset := 0; offset < profilerSourceOrderSidecarPageRecords; offset += 2 {
		for page := 0; page < pages; page++ {
			first := page*profilerSourceOrderSidecarPageRecords + offset
			timestampByOrdinal[first] = position
			position++
			if first+1 < (page+1)*profilerSourceOrderSidecarPageRecords {
				timestampByOrdinal[first+1] = position
				position++
			}
		}
	}
	if position != uint64(rows) {
		t.Fatalf("pair-thrash fixture positions=%d rows=%d", position, rows)
	}
	for ordinal := 0; ordinal < rows; ordinal++ {
		addProfilerSidecarOrdinaryRow(t, sink, renderedRow{
			tsNS: timestampByOrdinal[ordinal], seq: ordinal, line: "adjacent-pair-thrash",
		})
	}
	if err := sink.sealProfilerCapture(); err != nil {
		t.Fatal(err)
	}
	sidecarSize := sink.sourceOrderSidecar.size
	measure = true
	stats, err := sink.writeTo(context.Background(), io.Discard)
	if err != nil || stats.RowsWritten != rows {
		t.Fatalf("pair-thrash publication: stats=%+v err=%v", stats, err)
	}
	if readBytes > 8*sidecarSize || readCalls > 2*rows+32 {
		t.Fatalf("adjacent-pair read amplification: calls=%d rows=%d bytes=%d sidecar=%d",
			readCalls, rows, readBytes, sidecarSize)
	}
}

func TestProfilerSourceOrderSidecarScatterEncodeHasZeroPerRowAllocations(t *testing.T) {
	sink := &traceDBRowSink{options: traceDBRowSinkOptions{ops: traceDBRowSinkOps{
		writeAt: func(_ *os.File, data []byte, _ int64) (int, error) { return len(data), nil },
	}}}
	writtenFile := new(os.File)
	writer := &profilerSourceOrderSidecarScatterWriter{
		sink: sink, file: writtenFile, path: "allocation-proof",
		rowCount: uint64(2 * profilerSourceOrderSidecarPageRecords),
	}
	ctx := context.Background()
	provenance := profilerPairRowProvenance{PairKind: pairRenderUnknown}
	var leaf [sha256.Size]byte
	var runErr error
	allocations := testing.AllocsPerRun(20, func() {
		if runErr != nil {
			return
		}
		for ordinal := uint64(0); ordinal < writer.rowCount; ordinal++ {
			runErr = writer.encodeAndAdd(
				ctx, ordinal, leaf, provenance, profilerSourceOrderDispositionPublish,
			)
			if runErr != nil {
				return
			}
		}
		runErr = writer.flush(ctx)
	})
	if runErr != nil {
		t.Fatal(runErr)
	}
	if allocations != 0 {
		t.Fatalf("scatter encode allocations/run=%f want=0", allocations)
	}
}
