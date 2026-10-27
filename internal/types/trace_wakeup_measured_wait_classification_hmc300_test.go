package types

import "testing"

func TestHMC300WakeupMeasuredWaitSoftConsumerClassification(t *testing.T) {
	row, ok := TraceNoteKeyLookup(TraceNoteKeyWakeupMeasuredWait)
	if !ok || row.Family != "chain_path" || row.Carrier != TraceNoteCarrierSoftConsumer {
		t.Fatalf("typed display reader requires soft_consumer, never causal compiler authority: %+v", row)
	}
}
