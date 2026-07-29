package hitraceconv

import (
	"bytes"
	"context"
	"math"
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

func TestSubmitTraceDBRawMarkerSyncRecoveryDiagnosesExactNullDurationClosureWithoutAdmittingDuration(t *testing.T) {
	ctx := context.Background()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	retained := syncSpans.recordNullDurationHint(
		traceDBCallstackNullDurationHint{
			RowID: 1, HeaderTID: 201, HeaderTGID: 200, MarkerPID: 777,
			CanonicalITID: 2, OwnerIPID: 2, Start: 1_000_000, Name: "frame",
		})
	if !retained {
		t.Fatal("record exact NULL-duration hint failed")
	}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(
			traceDBRawMarkerTestPair(201, 777, 1, "frame")),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metadata["null_duration_raw_closure_census"] != "complete" ||
		coverage.Metrics["null_duration_fence_hints_total"] != 1 ||
		coverage.Metrics["null_duration_fence_hints_retained"] != 1 ||
		coverage.Metrics["null_duration_hints_unique_exact_raw_closure"] != 1 {
		t.Fatalf("NULL-duration raw closure census drifted: %+v", coverage)
	}
	if syncSpans.fencedTotal != 0 ||
		syncSpans.superseded[traceDBSyncSpanProducerCallstack] != 0 {
		t.Fatalf("diagnostic census changed fence/admission authority: %+v", syncSpans)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryClassifiesNullDurationRawDisposition(t *testing.T) {
	t.Run("closed pair rejected by exact local validation", func(t *testing.T) {
		ctx := context.Background()
		syncSpans := newTraceDBTestSyncSpanAuthority(t)
		if !syncSpans.recordNullDurationHint(
			traceDBCallstackNullDurationHint{
				RowID: 1, HeaderTID: 201, HeaderTGID: 200, MarkerPID: 777,
				CanonicalITID: 2, OwnerIPID: 2, Start: 1_000_000, Name: "frame",
			}) {
			t.Fatal("record exact NULL-duration hint failed")
		}
		rows := traceDBRawMarkerTestPair(201, 777, 1, "frame")
		rows[0].CPU = int(maxTraceDBCPUIndex + 1)
		coverage, err := submitTraceDBRawMarkerSyncRecovery(
			ctx, traceDBRawMarkerTestInventory(rows),
			traceDBRawBlockedKeyTestAuthority(), syncSpans)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.Metrics["null_duration_hints_without_valid_raw_closure"] != 1 ||
			coverage.Metrics["null_duration_hints_unique_exact_raw_rejected_closed_pair"] != 1 ||
			coverage.Metrics["null_duration_hints_exact_raw_rejected_closed_pair_invalid_begin_cpu"] != 1 ||
			coverage.Metrics["null_duration_hints_unique_exact_raw_open_begin"] != 0 ||
			syncSpans.fencedTotal != 0 {
			t.Fatalf("rejected closed-pair disposition drifted: %+v", coverage)
		}
	})

	t.Run("exact raw begin remains open", func(t *testing.T) {
		ctx := context.Background()
		syncSpans := newTraceDBTestSyncSpanAuthority(t)
		if !syncSpans.recordNullDurationHint(
			traceDBCallstackNullDurationHint{
				RowID: 1, HeaderTID: 201, HeaderTGID: 200, MarkerPID: 777,
				CanonicalITID: 2, OwnerIPID: 2, Start: 1_000_000, Name: "frame",
			}) {
			t.Fatal("record exact NULL-duration hint failed")
		}
		rows := traceDBRawMarkerTestPair(201, 777, 1, "frame")[:1]
		coverage, err := submitTraceDBRawMarkerSyncRecovery(
			ctx, traceDBRawMarkerTestInventory(rows),
			traceDBRawBlockedKeyTestAuthority(), syncSpans)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.Metrics["null_duration_hints_without_valid_raw_closure"] != 1 ||
			coverage.Metrics["null_duration_hints_unique_exact_raw_open_begin"] != 1 ||
			coverage.Metrics["null_duration_hints_unique_exact_raw_rejected_closed_pair"] != 0 ||
			coverage.Metrics["raw_open_begins_withheld"] != 1 ||
			syncSpans.fencedTotal != 0 {
			t.Fatalf("open-begin disposition drifted: %+v", coverage)
		}
	})
}

func TestSubmitTraceDBRawMarkerSyncRecoveryRetainsWitnessesPerReason(t *testing.T) {
	invalidCPU := traceDBRawMarkerTestPair(201, 777, 1, "cpu-invalid")
	invalidCPU[0].CPU = int(maxTraceDBCPUIndex + 1)
	invalidName := traceDBRawMarkerTestPair(201, 777, 3, " invalid-name")
	invalidName[0].Buffer = "B|777| invalid-name"
	invalidName[0].TimestampNS = 5_000_000
	invalidName[1].TimestampNS = 6_000_000
	rows := append(invalidCPU, invalidName...)
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	witnesses := coverage.Metadata["raw_marker_local_validation_witnesses"]
	if coverage.Metrics["raw_pairs_withheld_local_validation"] != 2 ||
		coverage.Metrics["raw_marker_local_validation_witnesses_invalid_begin_cpu_emitted"] != 1 ||
		coverage.Metrics["raw_marker_local_validation_witnesses_invalid_span_name_emitted"] != 1 ||
		coverage.Metrics["raw_marker_local_validation_witnesses_emitted"] != 2 ||
		!strings.Contains(witnesses, "reason=invalid_begin_cpu") ||
		!strings.Contains(witnesses, "reason=invalid_span_name") ||
		coverage.Metrics["raw_pairs_submitted"] != 0 {
		t.Fatalf("per-reason local witnesses drifted: %+v", coverage)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryNormalizesOfficialStructuredTrailingSpace(t *testing.T) {
	ctx := context.Background()
	rows := traceDBRawMarkerTestPair(201, 777, 1, "frame ")
	rows[0].OpenHarmonyStructuredProfile = true
	rows[1].OpenHarmonyStructuredProfile = true
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_official_trailing_space_name_normalized"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		coverage.Metrics["raw_pairs_withheld_invalid_span_name"] != 0 {
		t.Fatalf("official trailing-space pair was not submitted: %+v", coverage)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if _, _, err := syncSpans.finalize(ctx, sink); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	if _, err := sink.prepareAndWriteForTest(ctx, &body); err != nil {
		t.Fatal(err)
	}
	text := body.String()
	if !strings.Contains(text, "B|777|frame\n") ||
		strings.Contains(text, "B|777|frame \n") {
		t.Fatalf("official trailing-space normalization mismatch: %q", text)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryDoesNotBroadenTrailingSpaceRule(t *testing.T) {
	tests := []struct {
		name       string
		span       string
		structured bool
	}{
		{name: "compact trailing space", span: "frame "},
		{name: "structured leading space", span: " frame", structured: true},
		{name: "structured trailing tab", span: "frame\t", structured: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := traceDBRawMarkerTestPair(201, 777, 1, test.span)
			rows[0].OpenHarmonyStructuredProfile = test.structured
			rows[1].OpenHarmonyStructuredProfile = test.structured
			syncSpans := newTraceDBTestSyncSpanAuthority(t)
			coverage, err := submitTraceDBRawMarkerSyncRecovery(
				context.Background(), traceDBRawMarkerTestInventory(rows),
				traceDBRawBlockedKeyTestAuthority(), syncSpans)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Metrics["raw_pairs_official_trailing_space_name_normalized"] != 0 ||
				coverage.Metrics["raw_pairs_submitted"] != 0 {
				t.Fatalf("unsupported name shape gained normalization: %+v",
					coverage)
			}
		})
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryNormalizesOfficialStructuredZeroPID(t *testing.T) {
	ctx := context.Background()
	rows := traceDBRawMarkerTestPair(201, 0, 1, "frame")
	rows[0].Buffer = "B|0|frame"
	rows[1].Buffer = "E|0|"
	rows[0].ZeroPIDUsesHeaderIdentity = true
	rows[1].ZeroPIDUsesHeaderIdentity = true
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_official_zero_pid_header_identity_normalized"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		coverage.Metrics["raw_pairs_withheld_local_validation"] != 0 {
		t.Fatalf("official zero-PID pair was not submitted: %+v", coverage)
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
	if report.ByProducer[traceDBSyncSpanProducerSourceRawMarker].EmittedEndpoints != 2 {
		t.Fatalf("official zero-PID report=%+v", report)
	}
	var body bytes.Buffer
	if _, err := sink.prepareAndWriteForTest(ctx, &body); err != nil {
		t.Fatal(err)
	}
	text := body.String()
	if !strings.Contains(text, "B|200|frame") ||
		!strings.Contains(text, "E|200|") ||
		strings.Contains(text, "B|0|") || strings.Contains(text, "E|0|") {
		t.Fatalf("zero-PID standard normalization mismatch: %s", text)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryWithholdsCompactZeroPID(t *testing.T) {
	ctx := context.Background()
	rows := traceDBRawMarkerTestPair(201, 0, 1, "frame")
	rows[0].Buffer = "B|0|frame"
	rows[1].Buffer = "E|0|"
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	witness := coverage.Metadata["raw_marker_local_validation_witnesses"]
	if coverage.Metrics["raw_pairs_withheld_zero_payload_pid_without_official_header_identity"] != 1 ||
		coverage.Metrics["raw_marker_local_validation_witnesses_emitted"] != 1 ||
		!strings.Contains(witness,
			"reason=zero_payload_pid_without_official_header_identity") ||
		coverage.Metrics["raw_pairs_submitted"] != 0 {
		t.Fatalf("compact zero-PID pair gained header identity: %+v", coverage)
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
		coverage.Metrics["raw_pairs_exact_semantic_unique_cpu_known_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_unique_cpu_known_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 0 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 0 {
		t.Fatalf("exact DB duplicate was not withheld: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryWithholdsNameDriftWithoutPoisoningDBLane(t *testing.T) {
	ctx := context.Background()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	existing := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 9, 201, 200,
		1_000_000, 4_000_000, "db-normalized-name")
	existing.CanonicalITID = 2
	existing.OwnerIPID = 2
	existing.MarkerPID, existing.MarkerPIDKnown = 777, true
	if err := syncSpans.submit(ctx, existing); err != nil {
		t.Fatal(err)
	}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(
			traceDBRawMarkerTestPair(201, 777, 1, "raw-physical-name")),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_withheld_exact_interval_name_drift"] != 1 ||
		coverage.Metrics["raw_pairs_name_drift_unique_cpu_known_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_unique_cpu_known_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_existing_db_candidate"] != 0 ||
		coverage.Metrics["raw_pairs_submitted"] != 0 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 0 {
		t.Fatalf("name drift was not localized: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
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
	db := report.ByProducer[traceDBSyncSpanProducerCallstack]
	if report.IdenticalLanes != 0 || report.PoisonedLanes != 0 ||
		db.SubmittedSpans != 1 || db.EmittedEndpoints != 2 ||
		db.SuppressedSpans != 0 {
		t.Fatalf("raw name drift suppressed DB baseline: %+v", report)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryReplacesUniqueNameUnrepresentableCollision(t *testing.T) {
	ctx := context.Background()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	existing := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 9, 201, 200,
		1_000_000, 4_000_000, "db-normalized|I42")
	existing.CanonicalITID = 2
	existing.OwnerIPID = 2
	existing.MarkerPID, existing.MarkerPIDKnown = 777, true
	if err := syncSpans.submit(ctx, existing); err != nil {
		t.Fatal(err)
	}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(
			traceDBRawMarkerTestPair(201, 777, 1, "raw-physical-name")),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_withheld_exact_interval_name_drift"] != 0 ||
		coverage.Metrics["raw_pairs_name_drift_unique_cpu_known_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_name_drift_name_unrepresentable_callstack_replaced"] != 1 ||
		coverage.Metrics["raw_pairs_name_unrepresentable_callstack_replaced"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
		t.Fatalf("name-unrepresentable raw collision was not replaced: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
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
	dbStats := report.ByProducer[traceDBSyncSpanProducerCallstack]
	rawStats := report.ByProducer[traceDBSyncSpanProducerSourceRawMarker]
	if dbStats.SubmittedSpans != 1 || dbStats.SupersededSpans != 1 ||
		dbStats.SupersededCPUUnavailableSpans != 0 ||
		dbStats.SupersededNameUnrepresentableSpans != 1 ||
		dbStats.SuppressedSpans != 1 || dbStats.EmittedEndpoints != 0 ||
		rawStats.SubmittedSpans != 1 || rawStats.SuppressedSpans != 0 ||
		rawStats.EmittedEndpoints != 2 {
		t.Fatalf("candidate-level raw name replacement report drifted: %+v", report)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryReplacesUniqueCPUUnavailableCollision(t *testing.T) {
	ctx := context.Background()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	existing := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 9, 201, 200,
		1_000_000, 4_000_000, "db-normalized-name")
	existing.CanonicalITID = 2
	existing.OwnerIPID = 2
	existing.MarkerPID, existing.MarkerPIDKnown = 777, true
	existing.StartCPU, existing.EndCPU = 0, 0
	existing.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownStart
	existing.StartCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	existing.EndCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	if err := syncSpans.submit(ctx, existing); err != nil {
		t.Fatal(err)
	}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(
			traceDBRawMarkerTestPair(201, 777, 1, "raw-physical-name")),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_withheld_exact_interval_name_drift"] != 0 ||
		coverage.Metrics["raw_pairs_name_drift_unique_cpu_unavailable_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_pairs_unique_cpu_unavailable_callstack_candidate"] != 1 ||
		coverage.Metrics["raw_collision_callstack_cpu_unavailable_candidate_rows"] != 1 ||
		coverage.Metrics["raw_pairs_name_drift_cpu_unavailable_callstack_replaced"] != 1 ||
		coverage.Metrics["raw_pairs_cpu_unavailable_callstack_replaced"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
		t.Fatalf("CPU-unavailable raw collision replacement drifted: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
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
	dbStats := report.ByProducer[traceDBSyncSpanProducerCallstack]
	rawStats := report.ByProducer[traceDBSyncSpanProducerSourceRawMarker]
	if dbStats.SubmittedSpans != 1 || dbStats.SupersededSpans != 1 ||
		dbStats.SupersededCPUUnavailableSpans != 1 ||
		dbStats.SupersededNameUnrepresentableSpans != 0 ||
		dbStats.SuppressedSpans != 1 || dbStats.EmittedEndpoints != 0 ||
		rawStats.SubmittedSpans != 1 || rawStats.SuppressedSpans != 0 ||
		rawStats.EmittedEndpoints != 2 {
		t.Fatalf("candidate-level raw CPU replacement report drifted: %+v", report)
	}
	var body bytes.Buffer
	if _, err := sink.prepareAndWriteForTest(ctx, &body); err != nil {
		t.Fatal(err)
	}
	text := body.String()
	if !strings.Contains(text, "[003]") || !strings.Contains(text, "[004]") ||
		!strings.Contains(text, "B|777|raw-physical-name") ||
		strings.Contains(text, "codrax_trace_mark_cpu_unavailable") ||
		strings.Contains(text, "db-normalized-name") {
		t.Fatalf("raw CPU replacement wire drifted:\n%s", text)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryKeepsDBWhenRawIntervalIsAmbiguous(t *testing.T) {
	ctx := context.Background()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	existing := traceDBTestSyncSpanCandidate(
		traceDBSyncSpanProducerCallstack, 9, 201, 200,
		1_000_000, 4_000_000, "db-normalized-name")
	existing.CanonicalITID = 2
	existing.OwnerIPID = 2
	existing.MarkerPID, existing.MarkerPIDKnown = 777, true
	existing.StartCPU, existing.EndCPU = 0, 0
	existing.CPUPlacement = traceDBSyncSpanCPUPlacementUnknownStart
	existing.StartCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	existing.EndCPUProvenance = traceDBSyncSpanCPUCallstackUnavailable
	if err := syncSpans.submit(ctx, existing); err != nil {
		t.Fatal(err)
	}
	first := traceDBRawMarkerTestPair(201, 777, 1, "raw-one")
	second := traceDBRawMarkerTestPair(201, 777, 2, "raw-two")
	second[1].PhysicalOrdinal = 3
	first[1].PhysicalOrdinal = 4
	rows := []traceDBRawMarkerRecord{first[0], second[0], second[1], first[1]}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_structurally_closed"] != 2 ||
		coverage.Metrics["raw_pairs_authoritative_replacement_withheld_ambiguous_raw_interval"] != 2 ||
		coverage.Metrics["raw_pairs_cpu_unavailable_callstack_replaced"] != 0 ||
		coverage.Metrics["raw_pairs_submitted"] != 0 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 0 {
		t.Fatalf("ambiguous raw interval replaced the DB candidate: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
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
	dbStats := report.ByProducer[traceDBSyncSpanProducerCallstack]
	if dbStats.SupersededSpans != 0 || dbStats.SuppressedSpans != 0 ||
		dbStats.EmittedEndpoints != 2 {
		t.Fatalf("ambiguous raw interval suppressed the typed DB fallback: %+v", report)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryUsesRawAlternativeWhenDBCandidateIsLocallyFenced(t *testing.T) {
	tests := []struct {
		name       string
		dbName     string
		rawName    string
		fenceITID  int64
		fenceStart int64
		fenceEnd   int64
		fenceKind  traceDBSyncSpanFenceKind
		wantMetric string
	}{
		{
			name:       "exact semantic duplicate",
			dbName:     "frame",
			rawName:    "frame",
			fenceITID:  2,
			fenceStart: 2_000_000,
			fenceEnd:   3_000_000,
			fenceKind:  traceDBSyncSpanFenceInterval,
			wantMetric: "raw_pairs_existing_db_candidate_locally_suppressed",
		},
		{
			name:       "exact interval name drift",
			dbName:     "db-normalized-name",
			rawName:    "raw-physical-name",
			fenceITID:  2,
			fenceStart: 2_000_000,
			fenceEnd:   3_000_000,
			fenceKind:  traceDBSyncSpanFenceInterval,
			wantMetric: "raw_pairs_interval_collision_locally_suppressed",
		},
		{
			name:       "same physical tid fence from different incarnation",
			dbName:     "frame",
			rawName:    "frame",
			fenceITID:  1,
			fenceStart: 2_000_000,
			fenceKind:  traceDBSyncSpanFenceSuffix,
			wantMetric: "raw_pairs_existing_db_candidate_locally_suppressed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			syncSpans := newTraceDBTestSyncSpanAuthority(t)
			existing := traceDBTestSyncSpanCandidate(
				traceDBSyncSpanProducerCallstack, 9, 201, 200,
				1_000_000, 4_000_000, test.dbName)
			existing.CanonicalITID = 2
			existing.OwnerIPID = 2
			existing.MarkerPID, existing.MarkerPIDKnown = 777, true
			if err := syncSpans.submit(ctx, existing); err != nil {
				t.Fatal(err)
			}
			if err := syncSpans.fenceExactLane(ctx, traceDBSyncSpanLaneFence{
				Producer:           traceDBSyncSpanProducerCallstack,
				HeaderTID:          201,
				CanonicalITID:      test.fenceITID,
				CanonicalITIDKnown: true,
				Start:              test.fenceStart,
				End:                test.fenceEnd,
				Kind:               test.fenceKind,
				Reason:             traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
			}); err != nil {
				t.Fatal(err)
			}
			coverage, err := submitTraceDBRawMarkerSyncRecovery(
				ctx, traceDBRawMarkerTestInventory(
					traceDBRawMarkerTestPair(201, 777, 1, test.rawName)),
				traceDBRawBlockedKeyTestAuthority(), syncSpans)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Metrics[test.wantMetric] != 1 ||
				coverage.Metrics["raw_pairs_existing_db_candidate"] != 0 ||
				coverage.Metrics["raw_pairs_withheld_exact_interval_name_drift"] != 0 ||
				coverage.Metrics["raw_pairs_submitted"] != 1 ||
				syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
				t.Fatalf("locally fenced DB candidate erased raw alternative: %+v", coverage)
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
			dbStats := report.ByProducer[traceDBSyncSpanProducerCallstack]
			rawStats := report.ByProducer[traceDBSyncSpanProducerSourceRawMarker]
			if dbStats.SuppressedSpans != 1 || dbStats.EmittedEndpoints != 0 ||
				rawStats.SuppressedSpans != 0 || rawStats.EmittedEndpoints != 2 {
				t.Fatalf("raw alternative did not survive the producer-local fence: %+v", report)
			}
		})
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryUsesRawAlternativeWhenDBCandidateLaneIsLocallyPoisoned(t *testing.T) {
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
	if err := syncSpans.poisonExactLane(ctx, traceDBSyncSpanLanePoison{
		Producer:           traceDBSyncSpanProducerCallstack,
		HeaderTID:          201,
		CanonicalITID:      2,
		CanonicalITIDKnown: true,
		Reason:             traceDBSyncSpanLanePoisonRejectedCallstackCandidate,
	}); err != nil {
		t.Fatal(err)
	}
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		ctx, traceDBRawMarkerTestInventory(
			traceDBRawMarkerTestPair(201, 777, 1, "frame")),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_existing_db_candidate_locally_suppressed"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 {
		t.Fatalf("locally poisoned DB candidate erased raw alternative: %+v", coverage)
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
	dbStats := report.ByProducer[traceDBSyncSpanProducerCallstack]
	rawStats := report.ByProducer[traceDBSyncSpanProducerSourceRawMarker]
	if dbStats.SuppressedSpans != 1 || dbStats.EmittedEndpoints != 0 ||
		rawStats.SuppressedSpans != 0 || rawStats.EmittedEndpoints != 2 {
		t.Fatalf("raw alternative did not survive producer-local poison: %+v", report)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryNameDriftFenceKeepsEveryIdentityDimension(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*traceDBSyncSpanCandidate)
	}{
		{name: "host tgid", mutate: func(candidate *traceDBSyncSpanCandidate) {
			candidate.HeaderTGID = 201
		}},
		{name: "marker pid", mutate: func(candidate *traceDBSyncSpanCandidate) {
			candidate.MarkerPID = 778
		}},
		{name: "canonical itid", mutate: func(candidate *traceDBSyncSpanCandidate) {
			candidate.CanonicalITID = 1
		}},
		{name: "owner ipid", mutate: func(candidate *traceDBSyncSpanCandidate) {
			candidate.OwnerIPID = 1
		}},
		{name: "start timestamp", mutate: func(candidate *traceDBSyncSpanCandidate) {
			candidate.Start++
		}},
		{name: "end timestamp", mutate: func(candidate *traceDBSyncSpanCandidate) {
			candidate.End--
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			syncSpans := newTraceDBTestSyncSpanAuthority(t)
			existing := traceDBTestSyncSpanCandidate(
				traceDBSyncSpanProducerCallstack, 9, 201, 200,
				1_000_000, 4_000_000, "db-normalized-name")
			existing.CanonicalITID = 2
			existing.OwnerIPID = 2
			existing.MarkerPID, existing.MarkerPIDKnown = 777, true
			test.mutate(&existing)
			if err := syncSpans.submit(ctx, existing); err != nil {
				t.Fatal(err)
			}
			coverage, err := submitTraceDBRawMarkerSyncRecovery(
				ctx, traceDBRawMarkerTestInventory(
					traceDBRawMarkerTestPair(201, 777, 1, "raw-physical-name")),
				traceDBRawBlockedKeyTestAuthority(), syncSpans)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Metrics["raw_pairs_withheld_exact_interval_name_drift"] != 0 ||
				coverage.Metrics["raw_pairs_submitted"] != 1 ||
				syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
				t.Fatalf("near identity was collapsed into name drift: %+v", coverage)
			}
		})
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

func TestSubmitTraceDBRawMarkerSyncRecoverySubMicrosecondPairIsRepresentable(t *testing.T) {
	tiny := traceDBRawMarkerTestPair(201, 777, 1, "tiny")
	tiny[0].TimestampNS = 1_000_000
	tiny[1].TimestampNS = 1_000_500
	good := traceDBRawMarkerTestPair(201, 777, 3, "good")
	good[0].TimestampNS = 2_000_000
	good[1].TimestampNS = 4_000_000

	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(append(tiny, good...)),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_withheld_unrepresentable_interval"] != 0 ||
		coverage.Metrics["raw_emitter_lanes_clean"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_poisoned"] != 0 ||
		coverage.Metrics["raw_pairs_submitted"] != 2 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 2 {
		t.Fatalf("sub-microsecond pair was not submitted exactly: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryLocalizesOrphanAndOpenEndpoints(t *testing.T) {
	orphan := traceDBRawMarkerTestPair(201, 777, 1, "orphan")[1]
	orphan.PhysicalOrdinal = 1
	orphan.TimestampNS = 500_000
	open := traceDBRawMarkerTestPair(201, 777, 2, "open")[0]
	open.PhysicalOrdinal = 2
	open.TimestampNS = 1_000_000
	good := traceDBRawMarkerTestPair(201, 777, 3, "good")
	good[0].TimestampNS = 2_000_000
	good[1].TimestampNS = 4_000_000
	rows := []traceDBRawMarkerRecord{orphan, open, good[0], good[1]}

	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(rows),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_orphan_endpoints_withheld"] != 1 ||
		coverage.Metrics["raw_open_begins_withheld"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_partial"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_partially_salvaged"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_poisoned"] != 0 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
		t.Fatalf("localized orphan/open endpoints erased a closed pair: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryLocalizesExactPairValidationFailure(t *testing.T) {
	bad := traceDBRawMarkerTestPair(201, 777, 1, "bad")
	bad[0].CPU = int(maxTraceDBCPUIndex + 1)
	good := traceDBRawMarkerTestPair(201, 777, 3, "good")
	good[0].TimestampNS = 5_000_000
	good[1].TimestampNS = 8_000_000

	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(append(bad, good...)),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_withheld_local_validation"] != 1 ||
		coverage.Metrics["raw_pairs_withheld_invalid_begin_cpu"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_partial"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_partially_salvaged"] != 1 ||
		coverage.Metrics["raw_emitter_lanes_poisoned"] != 0 ||
		coverage.Metrics["raw_pairs_submitted"] != 1 ||
		syncSpans.submitted[traceDBSyncSpanProducerSourceRawMarker] != 1 {
		t.Fatalf("one exact pair validation failure erased a proven sibling: coverage=%+v submitted=%+v",
			coverage, syncSpans.submitted)
	}
}

func TestTraceDBRawMarkerSyncCandidateSplitsEndpointValidationReasons(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]traceDBRawMarkerRecord)
		want   string
	}{
		{
			name: "begin header pid",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[0].HeaderPID = 0
			},
			want: "invalid_begin_header_pid",
		},
		{
			name: "header pid mismatch",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[1].HeaderPID++
			},
			want: "header_pid_mismatch",
		},
		{
			name: "begin payload pid",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[0].PayloadPID = 0
			},
			want: "invalid_begin_payload_pid",
		},
		{
			name: "begin decoded pid mismatch",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[0].PayloadPID++
			},
			want: "begin_payload_pid_mismatch",
		},
		{
			name: "begin decoded name mismatch",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[0].Name = "other"
			},
			want: "begin_payload_name_mismatch",
		},
		{
			name: "end payload rejected",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[1].Buffer = "not-an-endpoint"
			},
			want: "end_payload_not_admitted",
		},
		{
			name: "end decoded pid mismatch",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[1].PayloadPID++
			},
			want: "end_payload_pid_mismatch",
		},
		{
			name: "begin timestamp overflow",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[0].TimestampNS = math.MaxInt64 + 1
			},
			want: "begin_timestamp_overflow",
		},
		{
			name: "end cpu",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[1].CPU = int(maxTraceDBCPUIndex + 1)
			},
			want: "invalid_end_cpu",
		},
		{
			name: "begin flags",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[0].Flags = math.MaxUint8 + 1
			},
			want: "invalid_begin_flags",
		},
		{
			name: "end preempt count",
			mutate: func(rows []traceDBRawMarkerRecord) {
				rows[1].PreemptCount = math.MaxUint8 + 1
			},
			want: "invalid_end_preempt_count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rows := traceDBRawMarkerTestPair(201, 777, 1, "frame")
			test.mutate(rows)
			_, reason := traceDBRawMarkerSyncCandidate(
				traceDBRawMarkerPair{begin: rows[0], end: rows[1]},
				traceDBRawBlockedKeyTestAuthority())
			if reason != test.want {
				t.Fatalf("reason=%q want %q", reason, test.want)
			}
		})
	}
}

func TestSubmitTraceDBRawMarkerSyncRecoveryReportsBoundedLongestPairWitnesses(t *testing.T) {
	long := traceDBRawMarkerTestPair(201, 777, 1, "long pair")
	long[0].TimestampNS = 0
	long[1].TimestampNS = 2_000_000_000
	short := traceDBRawMarkerTestPair(201, 777, 3, "short")
	short[0].TimestampNS = 3_000_000_000
	short[1].TimestampNS = 3_100_000_000

	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := submitTraceDBRawMarkerSyncRecovery(
		context.Background(), traceDBRawMarkerTestInventory(append(long, short...)),
		traceDBRawBlockedKeyTestAuthority(), syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["raw_pairs_structurally_closed"] != 2 ||
		coverage.Metrics["raw_pairs_begin_timestamp_zero"] != 1 ||
		coverage.Metrics["raw_pairs_begin_at_marker_first_timestamp"] != 1 ||
		coverage.Metrics["raw_pairs_duration_ge_1s"] != 1 ||
		coverage.Metrics["raw_pairs_duration_ge_100ms"] != 2 ||
		coverage.Metrics["raw_pairs_cover_at_least_half_marker_window"] != 1 ||
		coverage.Metadata["raw_marker_first_timestamp_ns"] != "0" ||
		coverage.Metadata["raw_marker_last_timestamp_ns"] != "3100000000" {
		t.Fatalf("pair duration diagnostics drifted: %+v", coverage)
	}
	witnesses := coverage.Metadata["raw_marker_longest_pair_witnesses"]
	if !strings.Contains(witnesses,
		`emitter=201/start_ns=0/end_ns=2000000000/duration_ns=2000000000/name="long pair"`) ||
		strings.Index(witnesses, `name="long pair"`) >
			strings.Index(witnesses, `name="short"`) {
		t.Fatalf("longest pair witnesses drifted: %q", witnesses)
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
	if coverage.Metrics["raw_emitter_lanes_poisoned"] != 0 ||
		coverage.Metrics["raw_pairs_withheld_local_validation"] != 1 ||
		coverage.Metrics["raw_pairs_withheld_begin_absent_or_lifecycle_absent"] != 1 ||
		coverage.Metrics["raw_pairs_submitted"] != 0 {
		t.Fatalf("namespace-shaped header was rewritten: %+v", coverage)
	}
}
