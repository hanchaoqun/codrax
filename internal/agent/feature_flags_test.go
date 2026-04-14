package agent

// P2.1 Session 1 Phase 4 — feature flag accessor tests.
//
// Pin the two invariants the rest of P2.1 leans on:
//
//  1. The default (unset / unknown / empty) state is OFF for both
//     flags, so any new test or production deployment that does not
//     opt in still gets the historical single-turn ReAct behavior.
//
//  2. TwoTurnExplorerEnabled() implies EvidenceToolEnabled() — the
//     escalation rule documented in feature_flags.go. Turn B has no
//     other evidence pipe, so a deployment that turns on two-turn
//     mode must transitively get the structured emit_evidence
//     channel, even if evidence_tool_mode is left blank in
//     codrax.yaml.

import "testing"

func TestEvidenceToolMode_DefaultOff(t *testing.T) {
	resetFeatureFlagsForTest()
	if EvidenceToolEnabled() {
		t.Fatal("default state must be off")
	}
	if EvidenceToolMode() != EvidenceToolModeOff {
		t.Errorf("EvidenceToolMode() = %q, want %q", EvidenceToolMode(), EvidenceToolModeOff)
	}
}

func TestEvidenceToolMode_OnByExplicitFlag(t *testing.T) {
	resetFeatureFlagsForTest()
	SetEvidenceToolMode("on")
	if !EvidenceToolEnabled() {
		t.Fatal("explicit on must enable")
	}
}

func TestEvidenceToolMode_NormalizesUnknownToOff(t *testing.T) {
	resetFeatureFlagsForTest()
	SetEvidenceToolMode("yes")
	if EvidenceToolEnabled() {
		t.Fatal("unknown values must collapse to off (closed enum)")
	}
	SetEvidenceToolMode("ON  ")
	if !EvidenceToolEnabled() {
		t.Errorf("uppercase + whitespace should normalize to on")
	}
}

func TestTwoTurnExplorerMode_DefaultOff(t *testing.T) {
	resetFeatureFlagsForTest()
	if TwoTurnExplorerEnabled() {
		t.Fatal("default state must be off")
	}
}

func TestTwoTurnExplorerMode_OnByExplicitFlag(t *testing.T) {
	resetFeatureFlagsForTest()
	SetTwoTurnExplorerMode("on")
	if !TwoTurnExplorerEnabled() {
		t.Fatal("explicit on must enable")
	}
}

func TestTwoTurnExplorer_ImpliesEvidenceTool(t *testing.T) {
	// The escalation rule: turning on two-turn mode ALSO turns on
	// the evidence tool path because Turn B has no other channel.
	// Even with evidence_tool_mode left at its default, Turn B must
	// see emit_evidence as a registered tool.
	resetFeatureFlagsForTest()
	if EvidenceToolEnabled() {
		t.Fatal("precondition: evidence tool must be off before flipping two-turn")
	}
	SetTwoTurnExplorerMode("on")
	if !EvidenceToolEnabled() {
		t.Error("TwoTurnExplorerEnabled() must imply EvidenceToolEnabled() — Turn B has no fallback channel")
	}
}

func TestTwoTurnExplorer_OffDoesNotForceEvidence(t *testing.T) {
	// The implication is one-way: turning two-turn back off must
	// not silently turn the evidence tool off too. Production
	// deployments running P1.1-only (evidence_tool_mode=on,
	// two_turn_explorer_mode=off) must keep working.
	resetFeatureFlagsForTest()
	SetEvidenceToolMode("on")
	SetTwoTurnExplorerMode("off")
	if !EvidenceToolEnabled() {
		t.Error("turning two-turn off must not undo an explicit evidence_tool_mode=on")
	}
	if TwoTurnExplorerEnabled() {
		t.Error("two-turn must be off")
	}
}

func TestTwoTurnExplorerMode_NormalizesUnknownToOff(t *testing.T) {
	resetFeatureFlagsForTest()
	SetTwoTurnExplorerMode("maybe")
	if TwoTurnExplorerEnabled() {
		t.Fatal("unknown values must collapse to off")
	}
}

// resetFeatureFlagsForTest restores the package-level atomic pointers
// to the unset state. The actual production reset path is "set the
// flag once at startup and never touch it again", but tests need to
// reach a clean baseline between table cases.
func resetFeatureFlagsForTest() {
	SetEvidenceToolMode("")
	SetTwoTurnExplorerMode("")
}
