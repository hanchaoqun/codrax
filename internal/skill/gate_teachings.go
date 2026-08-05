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
	Text: "Choose exactly one grounding lane before emitting each required expected-behavior contract: " +
		"(1) when the exact expected value appears verbatim in raw_request, a hard operator may use it; " +
		"(2) when the exact value comes from inspected evidence, use a hard operator only with a contract/placement evidence_ref naming where it was observed, or with a comparator whose expected value is grounded by raw_request or its own evidence_ref; " +
		"(3) when no exact value is grounded, do not invent an exact command result, output, status, or exception. Omit behavior_contracts[] when expected_outcomes[] already states the goal; otherwise use operator=satisfies only for essential soft behavior. " +
		"Hard operators are equals, not_equals, contains, not_contains, exists, not_exists, raises, not_raises, and returns. A value read during repository inspection counts only when its typed evidence_ref is emitted; the validator cannot read your reasoning history.",
}

// AllGateTeachings returns the closed universe the parity tripwires
// iterate. Register every new GateTeaching var here — an unlisted
// teaching is invisible to the tripwires and defeats the mechanism.
func AllGateTeachings() []GateTeaching {
	return []GateTeaching{
		GateTeachingWriteExactContractGrounding,
	}
}
