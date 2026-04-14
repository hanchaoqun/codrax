package agent

import (
	"strings"
	"sync/atomic"
)

// Process-level feature flags read by the agent layer at runtime.
//
// These mirror entries in codrax.yaml and are set once from main.go
// before any pipeline run starts. The atomic.Pointer indirection lets
// the explorer's prompt builder read the current value without
// holding a lock and without paying the cost of plumbing a config
// pointer through five layers of constructors.
//
// Historical note: an `evidence_tool_mode` flag used to gate the
// explorer's structured emit_evidence channel behind an off/on
// switch. It was shipped default-on, then deleted in the 2026-04-14
// simplification pass: the tool is always registered, the Phase 1
// prompt always teaches it, and the markdown fallback parser
// (parseEvidenceItems) still runs as a secondary merge channel for
// LLMs that write markdown blocks anyway.

// TwoTurnExplorerModeOff / TwoTurnExplorerModeOn are the legal values
// for the two_turn_explorer_mode flag. Default is "on" — the
// explorer's Turn A hands its transcript to Turn B (the extractor)
// which drains it into structured emit_* channels the finalizer
// consumes. The "off" value is retained so a user who hits a
// regression can disable the two-turn topology without rebuilding.
const (
	TwoTurnExplorerModeOff = "off"
	TwoTurnExplorerModeOn  = "on"
)

var twoTurnExplorerMode atomic.Pointer[string]

// SetTwoTurnExplorerMode stores the runtime value for the explorer's
// turn topology. Called from cmd/root.go after the codrax.yaml
// merge resolves the final value. Only the literal "off" disables
// the two-turn path; every other value (including empty / unknown)
// collapses to "on".
func SetTwoTurnExplorerMode(s string) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v != TwoTurnExplorerModeOff {
		v = TwoTurnExplorerModeOn
	}
	twoTurnExplorerMode.Store(&v)
}

// TwoTurnExplorerMode returns the current value of the flag.
// Returns TwoTurnExplorerModeOn when unset (the default).
func TwoTurnExplorerMode() string {
	if p := twoTurnExplorerMode.Load(); p != nil {
		return *p
	}
	return TwoTurnExplorerModeOn
}

// TwoTurnExplorerEnabled is the boolean shorthand callers usually want.
func TwoTurnExplorerEnabled() bool {
	return TwoTurnExplorerMode() == TwoTurnExplorerModeOn
}

// AnswerDocumentModeOff / AnswerDocumentModeOn are the legal values
// for the answer_document_mode flag. Compared as exact lower-case
// strings, mirror of EvidenceToolMode* / TwoTurnExplorerMode*.
const (
	AnswerDocumentModeOff = "off"
	AnswerDocumentModeOn  = "on"
)

var answerDocumentMode atomic.Pointer[string]

// SetAnswerDocumentMode stores the runtime value for the finalizer's
// answer-payload channel. Called from cmd/root.go after the
// codrax.yaml merge resolves the final value. Empty string, "off",
// and any unrecognised value collapse to off so the default path is
// the current prose-composition finalizer.
//
// P2.2: when set to "on", cmd/root.go registers the
// emit_answer_document tool type against the finalize skill and
// NewFinalizerAgent constructs answerDocumentEvaluator instead of
// the legacy finalizerEvaluator. Flipping the default requires a
// grid run + manual inspection (same gating as P1.1 / P2.1).
func SetAnswerDocumentMode(s string) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v != AnswerDocumentModeOn {
		v = AnswerDocumentModeOff
	}
	answerDocumentMode.Store(&v)
}

// AnswerDocumentMode returns the current value of the flag.
// Returns AnswerDocumentModeOff when unset.
func AnswerDocumentMode() string {
	if p := answerDocumentMode.Load(); p != nil {
		return *p
	}
	return AnswerDocumentModeOff
}

// AnswerDocumentEnabled is the boolean shorthand callers usually want.
func AnswerDocumentEnabled() bool {
	return AnswerDocumentMode() == AnswerDocumentModeOn
}

