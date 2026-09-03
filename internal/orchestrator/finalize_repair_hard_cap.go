package orchestrator

// finalize_repair_hard_cap.go — the P6 finalize repair hard cap (moved
// out of orchestrator.go under the IR delivery hot-file ratchet, §40.43
// R1 fold-in). The reachability advisory that reads it together with the
// other finalize-loop bounds lives in finalize_loop_gate_advisory.go
// (§40.43 F-orch 三轮复核 finding S).

// FinalizeRepairHardCapDefault is the conservative default cap on
// finalize-stage repair-loop iterations. P6 (2026-05-10): 2 means
// "after two repair attempts the answer ships with a residual-
// concerns caveat instead of a third LLM round".
const FinalizeRepairHardCapDefault = 2

// SetFinalizeRepairHardCap installs the operator-tunable hard cap.
// 0 (or out-of-range) → FinalizeRepairHardCapDefault.
func (o *Orchestrator) SetFinalizeRepairHardCap(n int) {
	if o == nil {
		return
	}
	if n <= 0 {
		n = FinalizeRepairHardCapDefault
	}
	o.finalizeRepairHardCap = n
}

// finalizeRepairHardCapValue returns the effective cap, falling
// back to FinalizeRepairHardCapDefault when the field is unset.
func (o *Orchestrator) finalizeRepairHardCapValue() int {
	if o == nil || o.finalizeRepairHardCap <= 0 {
		return FinalizeRepairHardCapDefault
	}
	return o.finalizeRepairHardCap
}
