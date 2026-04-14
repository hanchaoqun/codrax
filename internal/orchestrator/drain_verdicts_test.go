package orchestrator

// P2.1 Phase 10 — hypothesis verdict drain hook tests.
//
// The drain hook runs after a successful StageExtract dispatch and
// applies MarkHypothesis for every verdict the extractor emitted,
// while LEAVING the buffer populated so the finalizer's prompt
// builder can render the rationale / citation text back to the
// user. These tests pin all four branches of the hook:
//
//   1. Happy path — valid verdicts update IR + buffer retained
//   2. Unknown hypothesis id — warning, skip, continue
//   3. Nil AnalysisIR — no-op (fail closed)
//   4. Empty buffer — no-op (no crash)

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func newOrchestratorForDrain(ir *types.AnalysisIR, verdicts []types.HypothesisVerdict) *Orchestrator {
	mu := types.NewMutableState("")
	mu.AppendEmittedHypothesisVerdicts(verdicts)
	return &Orchestrator{
		busCtx: &types.BusContext{
			Mutable:    mu,
			AnalysisIR: ir,
		},
	}
}

func TestDrainHypothesisVerdicts_UpdatesIRAndRetainsBuffer(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{ID: "H1", Statement: "Foo calls Bar", Status: types.HypUnknown},
			{ID: "H2", Statement: "Bar returns true", Status: types.HypUnknown},
		},
	}
	verdicts := []types.HypothesisVerdict{
		{HypothesisID: "H1", Status: types.HypConfirmed, Rationale: "direct call", Citation: "a.go:10"},
		{HypothesisID: "H2", Status: types.HypRejected, Rationale: "returns false", Citation: "b.go:20"},
	}
	o := newOrchestratorForDrain(ir, verdicts)

	o.drainHypothesisVerdicts()

	// IR updated
	if ir.HypothesisSet[0].Status != types.HypConfirmed {
		t.Errorf("H1 status = %q, want confirmed", ir.HypothesisSet[0].Status)
	}
	if ir.HypothesisSet[1].Status != types.HypRejected {
		t.Errorf("H2 status = %q, want rejected", ir.HypothesisSet[1].Status)
	}
	// Buffer retained — finalizer still sees rationale + citation
	retained := o.busCtx.Mutable.EmittedHypothesisVerdicts()
	if len(retained) != 2 {
		t.Errorf("buffer should be retained after drain, got %d items", len(retained))
	}
	if retained[0].Rationale != "direct call" || retained[0].Citation != "a.go:10" {
		t.Errorf("buffer contents corrupted after drain: %+v", retained[0])
	}
}

func TestDrainHypothesisVerdicts_UnknownIDSkipsAndContinues(t *testing.T) {
	// A hallucinated hypothesis id must be skipped with a warning,
	// not abort the drain. Subsequent valid verdicts still apply.
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{
			{ID: "H1", Status: types.HypUnknown},
		},
	}
	verdicts := []types.HypothesisVerdict{
		{HypothesisID: "H_HALLUCINATED", Status: types.HypConfirmed, Citation: "x.go:1"},
		{HypothesisID: "H1", Status: types.HypRejected, Citation: "y.go:2"},
	}
	o := newOrchestratorForDrain(ir, verdicts)

	o.drainHypothesisVerdicts()

	if ir.HypothesisSet[0].Status != types.HypRejected {
		t.Errorf("H1 status should still apply after hallucinated id skip, got %q",
			ir.HypothesisSet[0].Status)
	}
}

func TestDrainHypothesisVerdicts_NilIR_NoOp(t *testing.T) {
	verdicts := []types.HypothesisVerdict{
		{HypothesisID: "H1", Status: types.HypConfirmed, Citation: "a.go:1"},
	}
	mu := types.NewMutableState("")
	mu.AppendEmittedHypothesisVerdicts(verdicts)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, AnalysisIR: nil}}

	// Must not panic; buffer must be preserved (no consumer yet).
	o.drainHypothesisVerdicts()

	retained := o.busCtx.Mutable.EmittedHypothesisVerdicts()
	if len(retained) != 1 {
		t.Errorf("nil IR must leave buffer untouched, got %d items", len(retained))
	}
}

func TestDrainHypothesisVerdicts_EmptyBuffer_NoOp(t *testing.T) {
	ir := &types.AnalysisIR{
		HypothesisSet: []types.Hypothesis{{ID: "H1", Status: types.HypUnknown}},
	}
	o := newOrchestratorForDrain(ir, nil)

	// Must not panic and must not change pre-existing IR state.
	o.drainHypothesisVerdicts()

	if ir.HypothesisSet[0].Status != types.HypUnknown {
		t.Errorf("empty buffer drain should not change IR, got status %q",
			ir.HypothesisSet[0].Status)
	}
}

func TestDrainHypothesisVerdicts_NilMutable_NoOp(t *testing.T) {
	// Defensive: a BusContext with nil Mutable must not panic. The
	// orchestrator never constructs one this way in production, but
	// the hook is the right place for the guard.
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: nil, AnalysisIR: &types.AnalysisIR{}}}
	o.drainHypothesisVerdicts()
}
