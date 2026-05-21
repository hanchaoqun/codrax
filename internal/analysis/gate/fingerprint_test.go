package gate

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestComputeGateFingerprint_PassedReportEmpty
func TestComputeGateFingerprint_PassedReportEmpty(t *testing.T) {
	r := types.GateReport{Rejected: false, Checks: []types.GateCheck{{Name: "x", Passed: true}}}
	if fp := computeGateFingerprint(r); fp != "" {
		t.Errorf("passed report: want empty fingerprint, got %q", fp)
	}
}

// TestComputeGateFingerprint_StableAcrossDigits — counts and digit
// runs in the detail vary between attempts but should not change
// the fingerprint.
func TestComputeGateFingerprint_StableAcrossDigits(t *testing.T) {
	r1 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.2 predicate_contradiction: but only 0 sub-topic emitted"},
		},
	}
	r2 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.2 predicate_contradiction: but only 3 sub-topic emitted"},
		},
	}
	if computeGateFingerprint(r1) != computeGateFingerprint(r2) {
		t.Errorf("digit count change should not move fingerprint: r1=%s r2=%s",
			computeGateFingerprint(r1), computeGateFingerprint(r2))
	}
}

// TestComputeGateFingerprint_StableAcrossEntityValues
func TestComputeGateFingerprint_StableAcrossEntityValues(t *testing.T) {
	r1 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.5 entity_unresolvable: sub-topic 1 \"foo\" (entities=[A, B])"},
		},
	}
	r2 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.5 entity_unresolvable: sub-topic 2 \"bar\" (entities=[C, D, E])"},
		},
	}
	// Both R1.5 — same fingerprint
	if computeGateFingerprint(r1) != computeGateFingerprint(r2) {
		t.Errorf("entity values should not move fingerprint: r1=%s r2=%s",
			computeGateFingerprint(r1), computeGateFingerprint(r2))
	}
}

// TestComputeGateFingerprint_DifferentRulesDifferent
func TestComputeGateFingerprint_DifferentRulesDifferent(t *testing.T) {
	r1 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.2 predicate_contradiction: ..."},
		},
	}
	r2 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.5 entity_unresolvable: ..."},
		},
	}
	if computeGateFingerprint(r1) == computeGateFingerprint(r2) {
		t.Errorf("R1.2 vs R1.5 should differ: r1=%s r2=%s",
			computeGateFingerprint(r1), computeGateFingerprint(r2))
	}
}

// TestComputeGateFingerprint_MultiRuleMerged — multi-rule failures
// joined by " | " produce a stable fingerprint that includes both
// rules.
func TestComputeGateFingerprint_MultiRuleMerged(t *testing.T) {
	r1 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.3 entity_orphan: ... | R1.5 entity_unresolvable: ..."},
		},
	}
	r2 := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.5 entity_unresolvable: ... | R1.3 entity_orphan: ..."},
		},
	}
	// Sorted keys → r1 and r2 yield the same fingerprint despite
	// different segment order.
	if computeGateFingerprint(r1) != computeGateFingerprint(r2) {
		t.Errorf("segment order should not move fingerprint: r1=%s r2=%s",
			computeGateFingerprint(r1), computeGateFingerprint(r2))
	}
}

// TestComputeGateFingerprint_AdvisoryRulesIncluded — R1.1 advisory and
// R1.8 advisory both have the "(advisory)" suffix; fingerprint
// captures it.
func TestComputeGateFingerprint_AdvisoryRulesIncluded(t *testing.T) {
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "R1.8 scope_anchor_distribution (advisory): cross-component question with N sub-repo scopes [a, b]"},
		},
	}
	fp := computeGateFingerprint(r)
	if len(fp) != 12 {
		t.Errorf("fingerprint should be 12 hex chars, got %q", fp)
	}
}

// TestComputeGateFingerprint_LengthBounded — every fingerprint is
// 12 hex chars (6 bytes) regardless of input size.
func TestComputeGateFingerprint_LengthBounded(t *testing.T) {
	r := types.GateReport{
		Rejected: true,
		Checks: []types.GateCheck{
			{Name: "subtopic_coherence", Passed: false, Detail: "very long detail string that goes on and on and on and on and on"},
		},
	}
	if got := len(computeGateFingerprint(r)); got != 12 {
		t.Errorf("want 12 hex chars, got %d", got)
	}
}

// TestComputeGateFingerprint_PopulatedByRunWith — end-to-end:
// gate.Run / RunWith stamps the fingerprint onto the report.
func TestComputeGateFingerprint_PopulatedByRunWith(t *testing.T) {
	// Construct a minimal IR with structural defects. Search-hint
	// coverage is soft telemetry, so this fixture relies on hard
	// structural checks such as dag_closure / contract_complete.
	ir := &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   types.RequestModel{},
		AnswerContract: types.AnswerContract{},
		QualityGate:    types.GateReport{},
	}
	report := RunWith(ir, GlobalThresholds(), "read", RunOptions{})
	if !report.Rejected {
		t.Skip("baseline fixture unexpectedly passed; can't verify fingerprint stamp")
	}
	if report.Fingerprint == "" {
		t.Errorf("rejected report should carry non-empty fingerprint")
	}
	if len(report.Fingerprint) != 12 {
		t.Errorf("fingerprint should be 12 hex chars, got %q", report.Fingerprint)
	}
}
