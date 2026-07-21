package llm

import (
	"testing"
	"time"
)

// TestFirstByteCapBelowReasoningFloor pins the §29.174 F5 件2 startup
// advisory predicate: precise typed-duration comparison against the
// §29.92.1 180s reasoning safety floor, gated by the (soft, name-based)
// reasoning-family roster. The witness configuration — MiniMax-M2.7 at
// a 40s cap — must fire; a floor-or-above cap, a zero/unknown cap, and
// a non-reasoning model must all stay silent.
func TestFirstByteCapBelowReasoningFloor(t *testing.T) {
	if !firstByteCapBelowReasoningFloor("MiniMax-M2.7", 40*time.Second) {
		t.Fatalf("witness shape (MiniMax-M2.7, 40s) must fire the advisory predicate")
	}
	if !firstByteCapBelowReasoningFloor("deepseek-r1", 179*time.Second) {
		t.Fatalf("sub-floor cap on a reasoning model must fire")
	}
	if firstByteCapBelowReasoningFloor("MiniMax-M2.7", 180*time.Second) {
		t.Fatalf("cap at the floor must NOT fire (default config is silent)")
	}
	if firstByteCapBelowReasoningFloor("MiniMax-M2.7", 0) {
		t.Fatalf("unknown/zero cap must NOT fire")
	}
	if firstByteCapBelowReasoningFloor("gpt-4o", 40*time.Second) {
		t.Fatalf("non-reasoning model name must NOT fire (soft roster gate)")
	}
	if firstByteCapBelowReasoningFloor("", 40*time.Second) {
		t.Fatalf("empty model name must NOT fire")
	}
}
