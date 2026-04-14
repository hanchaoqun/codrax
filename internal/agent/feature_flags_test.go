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
	SetAnswerDocumentMode("")
}

// ── P2.2 AnswerDocument flag ──────────────────────────────────────────

func TestAnswerDocumentMode_DefaultOff(t *testing.T) {
	resetFeatureFlagsForTest()
	if AnswerDocumentEnabled() {
		t.Fatal("default state must be off")
	}
	if AnswerDocumentMode() != AnswerDocumentModeOff {
		t.Errorf("AnswerDocumentMode() = %q, want %q", AnswerDocumentMode(), AnswerDocumentModeOff)
	}
}

func TestAnswerDocumentMode_OnByExplicitFlag(t *testing.T) {
	resetFeatureFlagsForTest()
	SetAnswerDocumentMode("on")
	if !AnswerDocumentEnabled() {
		t.Fatal("explicit on must enable")
	}
}

func TestAnswerDocumentMode_NormalizesUnknownToOff(t *testing.T) {
	resetFeatureFlagsForTest()
	SetAnswerDocumentMode("maybe")
	if AnswerDocumentEnabled() {
		t.Fatal("unknown values must collapse to off")
	}
	SetAnswerDocumentMode("  ON  ")
	if !AnswerDocumentEnabled() {
		t.Error("whitespace + uppercase must normalize to on")
	}
}

// TestAnswerDocumentMode_IndependentFromOthers pins that P2.2 does
// NOT escalate into or from the P1.1 / P2.1 flags. A deployment that
// turns on answer_document_mode alone must not force the explorer
// into two-turn mode, and turning on two-turn mode must not
// transitively flip the finalizer into AnswerDocument mode either.
func TestAnswerDocumentMode_IndependentFromOthers(t *testing.T) {
	resetFeatureFlagsForTest()
	SetAnswerDocumentMode("on")
	if EvidenceToolEnabled() {
		t.Error("answer_document_mode=on must not force evidence_tool_mode on")
	}
	if TwoTurnExplorerEnabled() {
		t.Error("answer_document_mode=on must not force two_turn_explorer_mode on")
	}

	resetFeatureFlagsForTest()
	SetTwoTurnExplorerMode("on")
	if AnswerDocumentEnabled() {
		t.Error("two_turn_explorer_mode=on must not force answer_document_mode on")
	}
}

// TestNewFinalizerAgent_AnswerDocumentFlag_SelectsEvaluator exercises
// the branch in NewFinalizerAgent. When AnswerDocumentEnabled() is
// false, the legacy finalizerEvaluator is used (legacy tests still
// pass byte-for-byte). When true, answerDocumentEvaluator is used.
// The evaluator types are not exported, so the test inspects
// behaviour via ShouldStop — finalizerEvaluator returns true on the
// first iteration in the no-AnswerSymbols + no-validator-covered-
// shape case, while answerDocumentEvaluator always returns false
// (soft-stop path).
func TestNewFinalizerAgent_AnswerDocumentFlag_SelectsEvaluator(t *testing.T) {
	defer resetFeatureFlagsForTest()

	// Flag off: legacy evaluator. Construct through NewFinalizerAgent
	// so we go through the branch under test.
	resetFeatureFlagsForTest()
	deps := &Dependencies{} // nil LLM fine; we only inspect construction
	legacy := NewFinalizerAgent(deps)
	if legacy == nil {
		t.Fatal("legacy: NewFinalizerAgent returned nil")
	}

	// Flag on: answer-document evaluator.
	SetAnswerDocumentMode("on")
	adoc := NewFinalizerAgent(deps)
	if adoc == nil {
		t.Fatal("answer_document: NewFinalizerAgent returned nil")
	}
	// Minimal smoke check: the two returned agents are distinct
	// constructions — if NewFinalizerAgent's new branch is wrong, the
	// ShouldStop behaviour will diverge between the two when driven
	// through BaseAgent. We validate the construction site picks the
	// expected evaluator type by inspecting the underlying BaseAgent.
	if legacyBase, ok := legacy.(*BaseAgent); ok {
		if _, ok := legacyBase.eval.(*finalizerEvaluator); !ok {
			t.Errorf("legacy: evaluator type = %T, want *finalizerEvaluator", legacyBase.eval)
		}
	} else {
		t.Errorf("legacy: unexpected agent type %T", legacy)
	}
	if adocBase, ok := adoc.(*BaseAgent); ok {
		if _, ok := adocBase.eval.(*answerDocumentEvaluator); !ok {
			t.Errorf("answer_document: evaluator type = %T, want *answerDocumentEvaluator", adocBase.eval)
		}
	} else {
		t.Errorf("answer_document: unexpected agent type %T", adoc)
	}
}
