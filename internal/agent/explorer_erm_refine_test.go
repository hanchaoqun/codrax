package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// explorer_erm_refine_test.go — tests for the two ERM increments
// (DependsOn sequential ordering + retry-triggered refinement)
// originally written for the investigation planner, now migrated
// to the ERM API per the audit at
// issues/investigation-planner-duplicates-erm.md.

// TestErmReadyRequirements verifies the DependsOn gating: only
// requirements whose prerequisites are all satisfied appear in
// the ready list.
func TestErmReadyRequirements(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: types.ReqRegistration, Entities: []string{"explorer"}, Status: "unsatisfied"},
		{Kind: types.ReqMechanism, Entities: []string{"explorer", "subagent"},
			Status: "unsatisfied", DependsOn: []string{"registration:explorer"}},
	}

	// Initially only registration is ready (mechanism depends on it).
	ready := ermReadyRequirements(reqs)
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready requirement, got %d", len(ready))
	}
	if ready[0].Kind != types.ReqRegistration {
		t.Errorf("ready[0].Kind = %q, want registration", ready[0].Kind)
	}

	// After satisfying registration, mechanism becomes ready.
	reqs[0].Status = "satisfied"
	ready = ermReadyRequirements(reqs)
	if len(ready) != 1 || ready[0].Kind != types.ReqMechanism {
		t.Errorf("after dep satisfied, expected mechanism ready, got %v", ready)
	}

	// After satisfying both, nothing is ready.
	reqs[1].Status = "satisfied"
	ready = ermReadyRequirements(reqs)
	if len(ready) != 0 {
		t.Errorf("all satisfied: expected 0 ready, got %d", len(ready))
	}
}

// TestErmMaybeRefine_MultiEntitySplit verifies that a multi-entity
// requirement is split into per-entity sub-requirements when the
// retry threshold is exceeded.
func TestErmMaybeRefine_MultiEntitySplit(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: types.ReqMechanism, Entities: []string{"explorer", "subagent"},
			Status: "unsatisfied", RetryCount: replanRetryThreshold},
	}

	result, changed := ermMaybeRefine(reqs)
	if !changed {
		t.Fatal("expected refinement to occur")
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 per-entity sub-requirements, got %d", len(result))
	}
	if result[0].Entities[0] != "explorer" || result[1].Entities[0] != "subagent" {
		t.Errorf("entities = [%v, %v], want [explorer, subagent]",
			result[0].Entities, result[1].Entities)
	}
	if result[0].Status != "unsatisfied" || result[1].Status != "unsatisfied" {
		t.Error("sub-requirements should start unsatisfied")
	}
}

// TestErmMaybeRefine_NoStall — below threshold, no refinement.
func TestErmMaybeRefine_NoStall(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: types.ReqMechanism, Entities: []string{"explorer", "subagent"},
			Status: "unsatisfied", RetryCount: 1},
	}

	_, changed := ermMaybeRefine(reqs)
	if changed {
		t.Error("should not refine when retry count is below threshold")
	}
}

// TestErmMaybeRefine_SingleEntityNoSplit — a single-entity
// requirement should NOT be split (nothing to split into).
func TestErmMaybeRefine_SingleEntityNoSplit(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: types.ReqMechanism, Entities: []string{"explorer"},
			Status: "unsatisfied", RetryCount: replanRetryThreshold},
	}

	_, changed := ermMaybeRefine(reqs)
	if changed {
		t.Error("should not split a single-entity requirement")
	}
}

// TestErmMaybeRefine_AlreadySatisfiedSkipped — satisfied
// requirements are never refined regardless of retry count.
func TestErmMaybeRefine_AlreadySatisfiedSkipped(t *testing.T) {
	reqs := []EvidenceRequirement{
		{Kind: types.ReqMechanism, Entities: []string{"a", "b"},
			Status: "satisfied", RetryCount: 10},
	}

	_, changed := ermMaybeRefine(reqs)
	if changed {
		t.Error("should not refine a satisfied requirement")
	}
}

// TestErmRequirementID verifies the stable key generation.
func TestErmRequirementID(t *testing.T) {
	r := EvidenceRequirement{
		Kind:     types.ReqMechanism,
		Entities: []string{"explorer", "subagent"},
	}
	id := ermRequirementID(r)
	want := "mechanism:explorer,subagent"
	if id != want {
		t.Errorf("id = %q, want %q", id, want)
	}
}
