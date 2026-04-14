package agent

// Feature flag accessor tests.
//
// Pin the invariants the rest of the pipeline leans on:
//
//  1. two_turn_explorer_mode defaults to ON — unset / unknown /
//     empty all collapse to "on" so the default runtime state is
//     the Turn A / Turn B two-agent topology.
//
//  2. answer_document_mode defaults to OFF — the P2.2 structured
//     finalizer is opt-in until its grid verification ships.
//
// The evidence_tool_mode flag was deleted in the 2026-04-14
// simplification pass — the emit_evidence channel is unconditionally
// registered and taught to the explorer. Nothing to test here.

import "testing"

func TestTwoTurnExplorerMode_DefaultOn(t *testing.T) {
	resetFeatureFlagsForTest()
	if !TwoTurnExplorerEnabled() {
		t.Fatal("default state must be on")
	}
}

func TestTwoTurnExplorerMode_ExplicitOffDisables(t *testing.T) {
	resetFeatureFlagsForTest()
	SetTwoTurnExplorerMode("off")
	if TwoTurnExplorerEnabled() {
		t.Fatal("explicit off must disable")
	}
}

func TestTwoTurnExplorerMode_NormalizesUnknownToOn(t *testing.T) {
	resetFeatureFlagsForTest()
	SetTwoTurnExplorerMode("maybe")
	if !TwoTurnExplorerEnabled() {
		t.Fatal("unknown values must collapse to on (default-on)")
	}
}

// resetFeatureFlagsForTest restores the package-level atomic pointers
// to the unset state. Production resets the flag once at startup and
// never touches it again; tests need a clean baseline between cases.
func resetFeatureFlagsForTest() {
	// Clear the atomic pointers back to nil so the getters return the
	// documented unset defaults (on for two-turn, off for answer_document).
	twoTurnExplorerMode.Store(nil)
	answerDocumentMode.Store(nil)
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
// NOT escalate into or from the two_turn_explorer flag.
// answer_document_mode=on must not force the explorer into any
// particular topology, and two_turn_explorer_mode=on must not
// transitively flip the finalizer into AnswerDocument mode.
func TestAnswerDocumentMode_IndependentFromOthers(t *testing.T) {
	resetFeatureFlagsForTest()
	SetTwoTurnExplorerMode("off")
	SetAnswerDocumentMode("on")
	if TwoTurnExplorerEnabled() {
		t.Error("answer_document_mode=on must not force two_turn_explorer_mode on")
	}

	resetFeatureFlagsForTest()
	SetAnswerDocumentMode("off")
	SetTwoTurnExplorerMode("on")
	if AnswerDocumentEnabled() {
		t.Error("two_turn_explorer_mode=on must not force answer_document_mode on")
	}
}

// TestNewFinalizerAgent_AnswerDocumentFlag_SelectsEvaluator exercises
// the branch in NewFinalizerAgent. Off → legacy finalizerEvaluator,
// on → answerDocumentEvaluator.
func TestNewFinalizerAgent_AnswerDocumentFlag_SelectsEvaluator(t *testing.T) {
	defer resetFeatureFlagsForTest()

	resetFeatureFlagsForTest()
	deps := &Dependencies{}
	legacy := NewFinalizerAgent(deps)
	if legacy == nil {
		t.Fatal("legacy: NewFinalizerAgent returned nil")
	}

	SetAnswerDocumentMode("on")
	adoc := NewFinalizerAgent(deps)
	if adoc == nil {
		t.Fatal("answer_document: NewFinalizerAgent returned nil")
	}
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
