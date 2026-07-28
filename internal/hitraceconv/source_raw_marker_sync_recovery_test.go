package hitraceconv

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func traceDBRawMarkerTestPair(headerPID, payloadPID int64, ordinal int64, name string) []traceDBRawMarkerRecord {
	return []traceDBRawMarkerRecord{
		{
			PhysicalOrdinal: ordinal, TimestampNS: 1_000_000,
			CPU: 3, HeaderPID: headerPID, Flags: 1, PreemptCount: 2,
			Buffer: "B|777|" + name, Action: "B", PayloadPID: payloadPID,
			Name: name, Admitted: true,
		},
		{
			PhysicalOrdinal: ordinal + 1, TimestampNS: 4_000_000,
			CPU: 4, HeaderPID: headerPID, Flags: 2, PreemptCount: 3,
			Buffer: "E|777|", Action: "E", PayloadPID: payloadPID,
			Admitted: true,
		},
	}
}

func traceDBRawMarkerTestInventory(rows []traceDBRawMarkerRecord) *traceDBSourceNameInventory {
	syncRecords, poisonRecords, carrierRejected := int64(0), int64(0), int64(0)
	for _, row := range rows {
		switch {
		case row.Action == "B" || row.Action == "E":
			syncRecords++
		case strings.HasPrefix(row.RejectReason, "carrier_"):
			carrierRejected++
		default:
			poisonRecords++
		}
	}
	return &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state":                     "strict_target_ledger_complete",
				"marker_format_geometry_witnesses": "print#32886[pid|name|start]",
			},
			Metrics: map[string]int64{
				"target_marker_sync_records_retained":        syncRecords,
				"target_marker_sync_poison_records_retained": poisonRecords,
				"target_marker_carrier_rejections_retained":  carrierRejected,
			},
		},
		RawMarkers: append([]traceDBRawMarkerRecord(nil), rows...),
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryPublishesDBDisjointExactPair(t *testing.T) {
	ctx := context.Background()
	rows := traceDBRawMarkerTestPair(201, 777, 1, "frame")
	rows[0].Buffer = "B|777|frame|D0001"
	rows[1].Buffer = "E|777|D0001"
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_submitted"] != 1 ||
		coverage.Metadata["publication_state"] != "submitted_to_shared_sync_authority" ||
		coverage.Metadata["marker_format_geometry_witnesses"] !=
			"print#32886[pid|name|start]" {
		t.Fatalf("raw pair was not submitted: %+v", coverage)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	report, _, err := syncSpans.finalize(ctx, sink)
	if err != nil {
		t.Fatal(err)
	}
	stats := report.ByProducer[traceDBSyncSpanProducerSourceRawMarker]
	if stats.SubmittedSpans != 1 || stats.EmittedEndpoints != 2 {
		t.Fatalf("raw producer report=%+v", report)
	}
	reconciled := []TraceDBCoverage{coverage}
	if err := reconcileTraceDBSyncSpanCoverage(reconciled, report); err != nil {
		t.Fatal(err)
	}
	if reconciled[0].RowsEmitted != 2 ||
		reconciled[0].Metrics["sync_endpoints_emitted"] != 2 ||
		reconciled[0].Metrics["sync_spans_submitted"] != 1 {
		t.Fatalf("raw marker coverage was not reconciled: %+v", reconciled[0])
	}
	var body bytes.Buffer
	if _, err := sink.prepareAndWriteForTest(ctx, &body); err != nil {
		t.Fatal(err)
	}
	text := body.String()
	if !strings.Contains(text, "[003]") || !strings.Contains(text, "[004]") ||
		!strings.Contains(text, traceFlagsToStr(1, 2)) ||
		!strings.Contains(text, traceFlagsToStr(2, 3)) ||
		!strings.Contains(text, "B|777|frame|D0001") ||
		!strings.Contains(text, "E|777|D0001") {
		t.Fatalf("exact raw marker endpoints missing: %s", text)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoverySkipsExactExistingDBCandidate(t *testing.T) {
	ctx := context.Background()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	existing := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 9, 201, 200,
		1_000_000, 4_000_000, "frame")
	existing.CanonicalITID = 2
	existing.OwnerIPID = 2
	existing.MarkerPID, existing.MarkerPIDKnown = 777, true
	if err := syncSpans.submit(ctx, existing); err != nil {
		t.Fatal(err)
	}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(
			traceDBRawMarkerTestPair(201, 777, 1, "frame")),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_existing_db_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 0 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 0 {
		t.Fatalf("exact DB duplicate was not withheld: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryPoisonsOnlyAffectedEmitter(t *testing.T) {
	bad := traceDBRawMarkerTestPair(201, 777, 1, "bad")
	bad[1].Admitted = false
	bad[1].RejectReason = "schema_invalid_end_tag"
	clean := traceDBRawMarkerTestPair(101, 888, 3, "clean")
	clean[0].Buffer, clean[1].Buffer = "B|888|clean", "E|888|"
	rows := append(bad, clean...)
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_emitter_lanes_poisoned"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
		t.Fatalf("poison escaped its raw emitter lane: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryRejectsNamespaceHeaderWithoutRewritingPayload(t *testing.T) {
	rows := traceDBRawMarkerTestPair(32788, 777, 1, "namespace")
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_emitter_lanes_poisoned"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 0 {
		t.Fatalf("namespace-shaped header was rewritten: %+v", coverage)
	}
}
