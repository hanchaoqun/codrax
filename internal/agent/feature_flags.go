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
const (
	EvidenceToolModeOff = "off"
	EvidenceToolModeOn  = "on"
)

var evidenceToolMode atomic.Pointer[string]

// SetEvidenceToolMode stores the runtime value for the explorer's
// evidence channel. Called from cmd/root.go after the codrax.yaml /
// runtime settings merge resolves the final value. Empty string,
// "off", and any unrecognised value all collapse to off so the
// default path is current behavior.
//
// P1.1: when set to "on", the explorer phase-2 prompt teaches the
// LLM to call emit_evidence and the cmd/root.go bootstrap registers
// that tool against the explore skill's ToolSuggestions list.
func SetEvidenceToolMode(s string) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v != EvidenceToolModeOn {
		v = EvidenceToolModeOff
	}
	evidenceToolMode.Store(&v)
}

// EvidenceToolMode returns the current value of the flag.
// Returns EvidenceToolModeOff when unset.
func EvidenceToolMode() string {
	if p := evidenceToolMode.Load(); p != nil {
		return *p
	}
	return EvidenceToolModeOff
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
// for the two_turn_explorer_mode flag. Compared as exact lower-case
// strings, mirror of EvidenceToolMode*.
const (
	TwoTurnExplorerModeOff = "off"
	TwoTurnExplorerModeOn  = "on"
)

var twoTurnExplorerMode atomic.Pointer[string]

// SetTwoTurnExplorerMode stores the runtime value for the explorer's
// turn topology. Called from cmd/root.go after the codrax.yaml
// merge resolves the final value. Empty string, "off", and any
// unrecognised value collapse to off so the default path is the
// current single-turn ReAct loop.
//
// P2.1: when set to "on", cmd/root.go also registers
// emit_answer_symbol and emit_hypothesis_verdict against the new
// extractor skill, scheduler.go inserts a StageExtract dispatch
// after the merged explorer window, and the orchestrator routes the
// extractor agent for that stage.
func SetTwoTurnExplorerMode(s string) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v != TwoTurnExplorerModeOn {
		v = TwoTurnExplorerModeOff
	}
	twoTurnExplorerMode.Store(&v)
}

// TwoTurnExplorerMode returns the current value of the flag.
// Returns TwoTurnExplorerModeOff when unset.
func TwoTurnExplorerMode() string {
	if p := twoTurnExplorerMode.Load(); p != nil {
		return *p
	}
	return TwoTurnExplorerModeOff
}

// TwoTurnExplorerEnabled is the boolean shorthand callers usually want.
func TwoTurnExplorerEnabled() bool {
	return TwoTurnExplorerMode() == TwoTurnExplorerModeOn
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
