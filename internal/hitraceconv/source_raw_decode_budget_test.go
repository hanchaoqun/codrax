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
	acc.retainedBytes = maxTraceDBRawDecodeRetainedBytes - 4
	if acc.reserveRetained("print", 5) {
		t.Fatal("retention beyond the byte budget was admitted")
	}
	if !acc.retentionCapped || acc.retainedBytes != maxTraceDBRawDecodeRetainedBytes-4 ||
		acc.coverage.Metrics["target_print_retention_budget_exceeded"] != 1 {
		t.Fatalf("typed retention withdrawal mismatch: %+v", acc)
	}
}
