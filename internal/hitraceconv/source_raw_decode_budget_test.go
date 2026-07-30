package hitraceconv

import "testing"

func TestTraceDBRawDecodeRetentionBudgetReplacesOrdinalCap(t *testing.T) {
	const (
		printRows         = 175165
		traceMarkerRows   = 7518
		blockedRows       = 21566
		switchLiteRows    = 117226
		wakeupLiteRows    = 66736
		wakeupNewRows     = 120
		dmaClosedRows     = 493 + 305 + 494 + 495 + 149 + 149
		rpd1ClosedTargets = printRows + traceMarkerRows + blockedRows +
			switchLiteRows + wakeupLiteRows + wakeupNewRows + dmaClosedRows
	)
	if rpd1ClosedTargets != 390416 {
		t.Fatalf("RPD-1 closed target census drifted: got=%d", rpd1ClosedTargets)
	}
	const largATargetRows = 2760444
	if largATargetRows <= 1000000 || rpd1ClosedTargets >= largATargetRows {
		t.Fatalf("large-trace regression census is not beyond the retired ordinal cap")
	}
	if maxTraceDBRawDecodeRetainedBytes != 768<<20 {
		t.Fatalf("raw retained-byte budget changed without refreshing the bounded contract: %d",
			maxTraceDBRawDecodeRetainedBytes)
	}
}

func TestTraceDBRawDecodeRetentionBudgetFailsClosedWithoutStoppingCensus(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	acc.retainedByFamily[traceDBRawRetentionMarker] =
		traceDBRawRetentionFamilyBudgets[traceDBRawRetentionMarker] - 4
	if acc.reserveRetained("print", 5) {
		t.Fatal("retention beyond the byte budget was admitted")
	}
	if !acc.retentionCapped ||
		acc.coverage.Metrics["target_marker_retention_budget_exceeded"] != 1 {
		t.Fatalf("typed retention withdrawal mismatch: %+v", acc)
	}
}

func TestTraceDBRawDecodeFamilyRetentionAuthorityIsIndependent(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	acc.retainedByFamily[traceDBRawRetentionMarker] =
		traceDBRawRetentionFamilyBudgets[traceDBRawRetentionMarker] - 4
	if acc.reserveRetained("print", 5) {
		t.Fatal("over-budget marker record was retained")
	}
	acc.coverage.Metadata["decode_state"] =
		"strict_target_ledger_complete_with_family_retention_withdrawal"
	acc.coverage.Metadata["retention_"+traceDBRawRetentionMarker+"_state"] =
		"incomplete_byte_budget"
	acc.coverage.Metadata["retention_"+traceDBRawRetentionSwitchLite+"_state"] =
		traceDBRawRetentionFamilyComplete
	if traceDBRawDecodeFamilyComplete(acc.coverage, traceDBRawRetentionMarker) {
		t.Fatal("budget-withdrawn marker family retained publication authority")
	}
	if !traceDBRawDecodeFamilyComplete(acc.coverage, traceDBRawRetentionSwitchLite) {
		t.Fatal("independent complete scheduler family was globally withdrawn")
	}
	if !traceDBRawDecodeCensusComplete(acc.coverage) {
		t.Fatal("family retention withdrawal erased the completed raw census")
	}
	acc.coverage.Metadata["retention_"+traceDBRawRetentionMarker+"_state"] =
		traceDBRawRetentionFamilyComplete
	acc.coverage.Metadata["retention_"+traceDBRawRetentionSwitchLite+"_state"] =
		"incomplete_byte_budget"
	if !traceDBRawDecodeFamilyComplete(acc.coverage, traceDBRawRetentionMarker) {
		t.Fatal("complete marker family was withdrawn by scheduler retention failure")
	}
	if traceDBRawDecodeFamilyComplete(acc.coverage, traceDBRawRetentionSwitchLite) {
		t.Fatal("budget-withdrawn scheduler family retained publication authority")
	}
}
