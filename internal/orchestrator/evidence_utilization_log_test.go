package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFinalizerCitationCount_NilAnswerDocumentReturnsZero proves
// the helper degrades gracefully — finalize that aborted before
// AnswerDocument was installed must not panic.
func TestFinalizerCitationCount_NilAnswerDocumentReturnsZero(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("x")}}
	if got := finalizerCitationCount(o, nil); got != 0 {
		t.Errorf("nil AnswerDocument count = %d, want 0", got)
	}
}

// TestLogEvidenceUtilization_NilSafe just exercises the helper on a
// minimally-populated Orchestrator to confirm no panic. The actual
// log line is observability-only and does not affect behaviour.
func TestLogEvidenceUtilization_NilSafe(t *testing.T) {
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("x")}}
	logEvidenceUtilization(o, nil) // must not panic
	logEvidenceUtilization(nil, nil)
}
