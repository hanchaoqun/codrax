package orchestrator

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// finalize_repair_hard_cap.go — the P6 finalize repair hard cap (moved
// out of orchestrator.go under the IR delivery hot-file ratchet, §40.43
// R1 fold-in) and the advisory that reads it together with the other
// finalize-loop bounds.

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

// clusterClosureExitReachability reports whether the cluster-closure
// fail-loud exit (AdvanceRepairExecutionPlan: a stuck deepest owner after
// ClusterStableBudget() consecutive stable attempts) can ever fire under
// the caps that precede it in the finalize retry loop (§40.43 R1 finding
// C). Both comparisons are exact integer facts about the loop order:
//
//   - The stuck exit fires on failure number stableBudget+1, when the
//     scheduler's retryUsed equals stableBudget. The P6 hard cap breaks
//     the loop BEFORE AdvanceRepairExecutionPlan whenever
//     retryUsed >= hardCap, so the exit is unreachable when
//     stableBudget >= hardCap.
//   - The W2.6 per-root cap counts the failing attempt itself (also
//     before Advance) and ships when the same root has reached
//     perRootCap attempts, so the exit is unreachable when
//     stableBudget+1 >= perRootCap.
//
// At the defaults (stable 2, hard cap 2, per-root 3) the P6 cap pre-empts
// on the third failure; the cluster exit is a backstop for raised caps.
// This is ADVISORY ONLY — an operator log line, never a gate.
func clusterClosureExitReachability(stableBudget, hardCap, perRootCap int) (unreachable bool, reason string) {
	switch {
	case hardCap > 0 && stableBudget >= hardCap:
		return true, fmt.Sprintf("finalize repair hard cap %d <= cluster stable budget %d (P6 accepts the draft on failure %d, the cluster exit needs failure %d)",
			hardCap, stableBudget, hardCap+1, stableBudget+1)
	case perRootCap > 0 && stableBudget+1 >= perRootCap:
		return true, fmt.Sprintf("cross-scope per-root attempt cap %d <= cluster stable budget %d + 1 (W2.6 ships on failure %d, the cluster exit needs failure %d)",
			perRootCap, stableBudget, perRootCap, stableBudget+1)
	}
	return false, ""
}

// logClusterClosureExitReachabilityOnce emits the advisory once per
// Orchestrator at scheduler entry (the first point where the per-run
// hard cap, the package-level cluster stable budget and per-root cap
// are all resolved). Returns the advisory text and whether it was
// emitted by this call, for the pin.
func (o *Orchestrator) logClusterClosureExitReachabilityOnce() (string, bool) {
	if o == nil || o.clusterClosureExitAdvisoryLogged {
		return "", false
	}
	o.clusterClosureExitAdvisoryLogged = true
	unreachable, reason := clusterClosureExitReachability(ClusterStableBudget(), o.finalizeRepairHardCapValue(), maxRepairAttemptsPerRootValue)
	if !unreachable {
		return "", false
	}
	msg := "[orchestrator] advisory: the cluster-closure fail-loud exit is unreachable under the current caps — " + reason
	logging.Info("%s", msg)
	return msg, true
}
