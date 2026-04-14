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

// EvidenceToolModeOff / EvidenceToolModeOn are the legal values for
// the evidence_tool_mode flag. Compared as exact lower-case strings.
// Default is "on" — the structured emit_evidence channel is the
// preferred path. The "off" value is retained so a user who hits
// a regression can disable the channel without rebuilding.
const (
	EvidenceToolModeOff = "off"
	EvidenceToolModeOn  = "on"
)

var evidenceToolMode atomic.Pointer[string]

// SetEvidenceToolMode stores the runtime value for the explorer's
// evidence channel. Called from cmd/root.go after the codrax.yaml /
// runtime settings merge resolves the final value. Only the literal
// "off" disables the channel; every other value (including empty /
// unknown) collapses to "on" so the default path is the structured
// channel.
func SetEvidenceToolMode(s string) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v != EvidenceToolModeOff {
		v = EvidenceToolModeOn
	}
	evidenceToolMode.Store(&v)
}

// EvidenceToolMode returns the current value of the flag.
// Returns EvidenceToolModeOn when unset (the default).
func EvidenceToolMode() string {
	if p := evidenceToolMode.Load(); p != nil {
		return *p
	}
	return EvidenceToolModeOn
}

// EvidenceToolEnabled is the boolean shorthand callers usually want.
// P2.1 escalation: when TwoTurnExplorerEnabled() is true the structured
// tool channel MUST also be available (Turn B has no other evidence
// pipe), so callers see EvidenceToolEnabled() == true even if the user
// only set two_turn_explorer_mode in codrax.yaml. Setting them
// independently is supported (P1.1-only is the documented pre-P2.1
// path); the implication is one-way.
func EvidenceToolEnabled() bool {
	if EvidenceToolMode() == EvidenceToolModeOn {
		return true
	}
	return TwoTurnExplorerEnabled()
}

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

// phase2EvidenceChannelInstructions returns the explorer phase-2 prompt
// fragment that teaches the LLM how to record evidence. When the
// emit_evidence tool is enabled it instructs a tool call as the
// preferred channel; the markdown format below the call site stays
// available as a fallback so a model that refuses tools still produces
// usable output. When disabled, returns the plain " (use the markdown
// format below)" leader so the existing prompt reads cleanly.
func phase2EvidenceChannelInstructions() string {
	if !EvidenceToolEnabled() {
		return ":"
	}
	return ".\n\n" +
		"**Preferred channel: call the `emit_evidence` tool.** After reading a file, " +
		"call `emit_evidence(items=[...])` with one item per fact you want the synthesis " +
		"layer to see. Send the full batch in ONE call per file — do not invoke the tool " +
		"per item. Each item is an object with these fields:\n" +
		"  - `kind`: one of `direct`, `conditional`, `registration`, `mechanism`, `relationship`, `absent`\n" +
		"  - `subject`: the primary symbol (function/type/key) the fact is about\n" +
		"  - `object`: the secondary symbol (REQUIRED for `relationship`)\n" +
		"  - `source`: repository-relative file path (REQUIRED, must contain `/` or `.`)\n" +
		"  - `line_start`: integer line number taken EXACTLY from the read_file gutter (omit if no specific line)\n" +
		"  - `line_end`: optional end of range, defaults to line_start\n" +
		"  - `condition`: the IF clause for `conditional` items\n" +
		"  - `summary`: free-text rationale\n" +
		"Unknown fields and unknown kinds are REJECTED with a clear error — fix and resend rather than retry blind.\n\n" +
		"**Fallback channel: markdown blocks.** If you cannot use the tool for a particular item, write the markdown shape below in your assistant message. The two channels are merged downstream and deduplicated, so it is also safe to use both — just do not duplicate the same fact verbatim across them.\n\n" +
		"Markdown shape:"
}
