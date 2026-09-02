package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// validator_qualifier_test.go — SIDECAR-Q1 复核收编 (batch-one adversarial
// review, 2026-09-02): the legacy Required TraceFindingV1 lane reads the SAME
// typed closed-set ceiling the compiler derives (never the retired "unproven"
// literal), and the frozen candidate's seat-level causal_qualifier is
// system-owned: blank or spoofed values are rejected, and a frame-unproven
// seat caps its own status below proven.

func qualifierContract(qualifier string) *types.TraceFindingContract {
	contract := testContract()
	contract.CausalCeiling = qualifier
	contract.Candidates = []types.TraceFindingCandidateV1{{Decision: types.TraceCauseDecision{
		CandidateID: "candidate-1", Status: types.TraceCausalSupportedCandidate,
		Token:       types.TraceCausalTokenSnapshot{Token: "binder_wait", Lane: "wakeup_chain", Additivity: "wall_clock_per_thread", SubjectKind: "per_thread", FixDirection: "io_dependency", RegistryHash: "registry-v1"},
		SubjectRole: "ui_thread", CausalShape: "upstream_completion_wakes_target", Phase: "pre_wakeup_dependency", EvidenceRefs: []string{"evidence-1"},
		CausalQualifier: qualifier,
	}}}
	return contract
}

func TestValidatorCeilingReadsTypedFrameUnprovenQualifier(t *testing.T) {
	contract := qualifierContract(types.TraceCausalQualifierFrameUnproven)
	finding := testFinding()
	finding.PrimaryCause.CausalQualifier = types.TraceCausalQualifierFrameUnproven
	finding.PrimaryCause.Status = types.TraceCausalProven
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "exceeds causal ceiling frame_unproven") {
		t.Fatalf("a compiled frame_unproven ceiling must reject status=proven (the literal \"unproven\" arm was dead): %v", err)
	}
	// The retired literal is NOT a ceiling any more: nothing in the compiler
	// produces it, so it must not silently narrow either.
	contract.CausalCeiling = "unproven"
	finding.PrimaryCause.Status = types.TraceCausalSupportedCandidate
	if err := Validate(finding, contract); err != nil {
		t.Fatalf("supported_candidate under any ceiling is valid: %v", err)
	}
}

func TestValidatorCandidateQualifierIsSystemOwned(t *testing.T) {
	contract := qualifierContract(types.TraceCausalQualifierFrameUnproven)
	// Blank qualifier ⇒ rejected (never inferred from absence).
	finding := testFinding()
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "causal_qualifier") {
		t.Fatalf("a blank causal_qualifier must be rejected against the frozen candidate: %v", err)
	}
	// Spoofed qualifier ⇒ rejected.
	finding.PrimaryCause.CausalQualifier = types.TraceCausalQualifierProven
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "causal_qualifier") {
		t.Fatalf("a spoofed proven qualifier must be rejected: %v", err)
	}
	// Verbatim copy ⇒ accepted.
	finding.PrimaryCause.CausalQualifier = types.TraceCausalQualifierFrameUnproven
	if err := Validate(finding, contract); err != nil {
		t.Fatalf("verbatim qualifier copy must validate: %v", err)
	}
	// Seat-level cap: a frame-unproven seat cannot carry status=proven even
	// when the contract-wide ceiling is proven (another seat may be clean).
	contract = qualifierContract(types.TraceCausalQualifierFrameUnproven)
	contract.CausalCeiling = types.TraceCausalQualifierProven
	finding.PrimaryCause.Status = types.TraceCausalProven
	if err := Validate(finding, contract); err == nil || !strings.Contains(err.Error(), "seat-level causal qualifier") {
		t.Fatalf("status proven above the seat's own frame_unproven qualifier must be rejected: %v", err)
	}
}
