package hitraceconv

import "testing"

func TestTraceDBRawDecodeBudgetCoversRPD1ClosedTargetCensus(t *testing.T) {
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
	if maxTraceDBRawDecodeTargetRows <= rpd1ClosedTargets {
		t.Fatalf("raw decode cap=%d cannot complete RPD-1 target census=%d",
			maxTraceDBRawDecodeTargetRows, rpd1ClosedTargets)
	}
	if maxTraceDBRawDecodeTargetRows != 1000000 {
		t.Fatalf("raw decode budget changed without refreshing the bounded contract: %d",
			maxTraceDBRawDecodeTargetRows)
	}
}
