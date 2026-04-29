package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestFinalizerCitationCount_DedupesSameAnchor locks the
// distinct-(file, line) semantic. Two citations at the same anchor
// must count once.
func TestFinalizerCitationCount_DedupesSameAnchor(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Citations: []types.Citation{
			{File: "a.go", Line: 10},
			{File: "a.go", Line: 10}, // duplicate, should dedup
			{File: "b.go", Line: 20},
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
	if got := finalizerCitationCount(o, nil); got != 2 {
		t.Errorf("dedup citation count = %d, want 2", got)
	}
}

// TestFinalizerCitationCount_RejectsBadAnchors guards against
// counting citations with blank file or non-positive line — those
// fail downstream grounding and should not appear in the
// utilization metric either.
func TestFinalizerCitationCount_RejectsBadAnchors(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.SetAnswerDocument(&types.AnswerDocument{
		Citations: []types.Citation{
			{File: "", Line: 5},          // blank file
			{File: "a.go", Line: 0},      // line 0
			{File: "a.go", Line: -1},     // negative line
			{File: "real.go", Line: 100}, // real
		},
	})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut}}
	if got := finalizerCitationCount(o, nil); got != 1 {
		t.Errorf("count = %d, want 1 (only real.go:100 is valid)", got)
	}
}

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
