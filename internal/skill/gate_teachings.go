package skill

// gate_teachings.go — EVALFIX-2A (Class 1: gate-hint / initial-prompt
// teaching parity, design: docs/design/evalfix2_class_designs_20260730.md
// CLASS 1). A hard gate's reject hint that teaches the gate's typed
// escape lanes, while the agent's initial prompt teaches a drifted
// approximation, makes every first hit on the gate burn a full LLM
// round structurally. The teaching text therefore lives HERE, once;
// both the initial prompt surface (skill Workflow) and the retry-hint
// surface (the analyzer retry channel) assemble it by Go reference.
// Parity is enforced mechanically:
//
//   - Tripwire A (gate_teaching_parity_test.go): the named skill's
//     rendered corpus must contain Text verbatim.
//   - Tripwire B (orchestrator/gate_teaching_hint_census_test.go):
//     every analyzer retry-hint call site must reference a GateTeaching
//     or carry an explicit exemption with a rationale.
//
// Text is LLM-facing and must pass the prompt red-line checklist:
// typed lanes only, no internal Go/pipeline names, abstract
// placeholders, no answer values.

// GateTeaching is one hard gate's escape-lane teaching, single-sourced.
// Text is the only authority: both teaching surfaces splice it in by
// reference — never hand-copy the sentence.
type GateTeaching struct {
	// Key is the stable identifier used by tripwire exemption tables
	// and failure messages.
	Key string

	// SkillName names the skill whose rendered prompt corpus must
	// carry Text verbatim (the initial-prompt surface).
	SkillName string

	// Text is the LLM-facing teaching block. It enumerates the gate's
	// typed escape lanes exactly as the gate's predicate reads them —
	// not a prose approximation of the rule.
	Text string
}

// GateTeachingWriteExactContractGrounding teaches the three typed
// escape lanes of the write-analysis exact-contract grounding gate
// (orchestrator/write_analysis_quality.go). The gate can only see
// the emitted fields — a value the model read during inspection
// passes only when it lands in a typed evidence_ref field, which is
// exactly what this text says.
var GateTeachingWriteExactContractGrounding = GateTeaching{
	Key:       "write_exact_contract_grounding",
	SkillName: "write-analysis-skill",
	Text: "For hard operators (equals, not_equals, contains, not_contains, exists, not_exists, raises, not_raises, returns) on a required expected-behavior contract, the expected value must be verifiably grounded in one of three ways: (a) it appears verbatim in raw_request; (b) the contract (or its placement) carries evidence_ref naming where the value was observed, such as issue text or file:line; (c) a comparator is attached whose expected value appears verbatim in raw_request or carries its own evidence_ref. " +
		"A value you saw during repository inspection counts ONLY when you attach the evidence_ref — the validator checks the emitted fields, not your reading history. When none of these lanes apply, use operator=satisfies (soft behavior text) instead.",
}

// AllGateTeachings returns the closed universe the parity tripwires
// iterate. Register every new GateTeaching var here — an unlisted
// teaching is invisible to the tripwires and defeats the mechanism.
func AllGateTeachings() []GateTeaching {
	return []GateTeaching{
		GateTeachingWriteExactContractGrounding,
	}
}
