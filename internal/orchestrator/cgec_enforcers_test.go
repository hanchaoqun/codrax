package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDetectStallAndAct_NoFingerprintHistory_NoStall: a single
// fingerprint cannot be a stall. Returns false, no repair queued.
func TestDetectStallAndAct_NoFingerprintHistory_NoStall(t *testing.T) {
	o := newTestOrch(t)
	if o.detectStallAndAct() {
		t.Errorf("first call must not detect stall")
	}
	if reps := o.busCtx.Mutable.EvidenceClosure().PendingRepairs(); len(reps) > 0 {
		t.Errorf("expected no repairs queued, got %v", reps)
	}
}

// TestDetectStallAndAct_TwoIdenticalFingerprints_DoesNotForceComplete:
// two consecutive identical fingerprints triggers forced-read attempt
// but does NOT force-complete. The flag stays false.
func TestDetectStallAndAct_TwoIdenticalFingerprints_DoesNotForceComplete(t *testing.T) {
	o := newTestOrch(t)
	// Two identical rounds (no read, no evidence, no chains).
	o.detectStallAndAct()
	o.detectStallAndAct()
	if o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("two-fingerprint stall must not force-complete")
	}
}

// TestDetectStallAndAct_ThreeIdenticalFingerprints_HardStall:
// three consecutive identical fingerprints triggers hard stall →
// investigationComplete is forced and a force_complete_downgrade
// repair is queued.
func TestDetectStallAndAct_ThreeIdenticalFingerprints_HardStall(t *testing.T) {
	o := newTestOrch(t)
	o.detectStallAndAct()
	o.detectStallAndAct()
	hard := o.detectStallAndAct()
	if !hard {
		t.Errorf("third identical fingerprint should signal hard stall")
	}
	if !o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("hard stall must force-complete the investigation")
	}
	var found bool
	for _, r := range o.busCtx.Mutable.EvidenceClosure().PendingRepairs() {
		if r.Kind == types.RepairForceCompleteDowngrade {
			found = true
		}
	}
	if !found {
		t.Errorf("hard stall must queue RepairForceCompleteDowngrade repair")
	}
}

// TestDetectStallAndAct_ProgressBetweenRounds_NoStall: when a new
// piece of evidence is emitted between rounds, the fingerprint
// changes and no stall fires.
func TestDetectStallAndAct_ProgressBetweenRounds_NoStall(t *testing.T) {
	o := newTestOrch(t)
	o.detectStallAndAct()
	// Simulate progress: emit one new evidence item.
	o.busCtx.Mutable.AppendEvidence([]types.EvidenceItem{
		{
			ID:        types.StableEvidenceID(types.EvidenceConcrete, "foo", "p", "v", "", "f", 1, 1),
			Source:    "f",
			LineStart: 1,
		},
	})
	if o.detectStallAndAct() {
		t.Errorf("progress between rounds must not be a stall")
	}
	if o.busCtx.Mutable.IsInvestigationComplete() {
		t.Errorf("progress must not force-complete")
	}
}

// TestRunForcedReads_NoPending_Noop: with an empty PendingReads
// queue, runForcedReads returns 0 and changes nothing.
func TestRunForcedReads_NoPending_Noop(t *testing.T) {
	o := newTestOrch(t)
	if got := o.runForcedReads(); got != 0 {
		t.Errorf("expected 0 forced reads, got %d", got)
	}
}

// TestRunForcedReads_BudgetCap: even with many PendingReads, the
// per-round cap (cgecForcedReadsPerRound) is respected.
func TestRunForcedReads_BudgetCap(t *testing.T) {
	o := newTestOrch(t)
	closure := o.busCtx.Mutable.EvidenceClosure()
	// Queue more than the cap. Use a path that DOES NOT exist on
	// disk so the read fails — we just want to confirm the cap is
	// applied (the loop attempts only N before stopping).
	for i := 0; i < cgecForcedReadsPerRound+5; i++ {
		closure.AddPendingRead(types.PendingRead{
			File:      string(rune('a'+i)) + "/missing.go",
			Rationale: "test",
			Origin:    "test",
		})
	}
	// Calling runForcedReads with no real files just returns 0
	// successful reads. The important assertion is that PendingReads
	// is NOT entirely emptied — over-the-cap entries remain.
	o.runForcedReads()
	remaining := closure.PendingReads()
	if len(remaining) < 5 {
		t.Errorf("over-cap entries should remain unread; got %d remaining (started with %d)",
			len(remaining), cgecForcedReadsPerRound+5)
	}
}

// newTestOrch builds a minimal Orchestrator instance with a freshly-
// initialized BusContext + MutableState. Sufficient for the CGEC
// enforcer unit tests; does NOT wire the full pipeline.
func newTestOrch(t *testing.T) *Orchestrator {
	t.Helper()
	mut := types.NewMutableState("test objective")
	bus := &types.BusContext{
		Mutable:  mut,
		RepoRoot: t.TempDir(),
	}
	return &Orchestrator{busCtx: bus, emit: render.NopEmitter}
}
