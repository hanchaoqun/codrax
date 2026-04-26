package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// checkTier1Floor runs the Tier-1-proven-ratio gate against the
// accumulated evidence buffer, right before the orchestrator would
// dispatch the pre-finalize extract + finalize. It is the
// orchestrator-level mirror of tool.tier1GateReject: that gate only
// fires when the LLM explicitly calls emit_investigation_complete,
// but the explorer can also exit via ShouldStop / idle force-stop /
// soft-stop acceptance, and those paths bypass the tool entirely.
// This check catches them all — every exit path converges here
// because we gate the extract + finalize dispatch.
//
// Return values:
//
//	msg      — human-readable diagnostic suitable for pendingViolation
//	proceed  — true when the gate passes; caller continues to extract
//	exhausted — true when the gate fails AND the retry budget has no
//	           more room. Caller falls through to finalize fail-loud.
//
// When proceed=false and exhausted=false, the caller is expected to
// requeue nodes + record a retry + set pendingViolation=msg and
// continue the main loop.
func (o *Orchestrator) checkTier1Floor(ir *types.AnalysisIR, state *graphState) (msg string, proceed bool, exhausted bool) {
	floor := tool.CurrentGroundingPolicy().Tier1Floor
	if floor <= 0 {
		return "", true, false
	}
	if o.busCtx == nil || o.busCtx.Mutable == nil {
		return "", true, false
	}
	evidence := o.busCtx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		// No evidence emitted at all — tool-only investigation
		// (exec_command / grep-only answer). Accept; downstream
		// absence checks will handle.
		return "", true, false
	}
	tier1, total := countTier1Evidence(evidence)
	if total == 0 {
		return "", true, false
	}
	ratio := float64(tier1) / float64(total)
	if ratio >= floor {
		return "", true, false
	}
	logging.Info("[orchestrator] pre-finalize Tier-1 floor: ratio=%.0f%% (%d/%d) < floor=%.0f%% — will requeue explorer",
		ratio*100, tier1, total, floor*100)
	var b strings.Builder
	fmt.Fprintf(&b, "Tier-1 proven ratio %.0f%% (%d grounded-via-line_text / %d total) below floor %.0f%%.",
		ratio*100, tier1, total, floor*100)
	b.WriteString(" The finalizer's citation grounder will reject anchors the explorer never read_file'd — ")
	b.WriteString("explorer must call read_file on the recovered sources before the investigation can complete.")
	if exhausted := state.retryBudgetExhausted(); exhausted {
		return b.String(), false, true
	}
	return b.String(), false, false
}

// countTier1Evidence returns (tier1, total) where tier1 is the count
// of items the LLM grounded via TierLineText (actually read the
// file) and total is the overall evidence count. Legacy empty-
// GroundingStatus items (deterministic concrete_value scans) count
// toward tier1 — they are facts extracted from source, not LLM
// speculation, and should not push the floor down.
func countTier1Evidence(evidence []types.EvidenceItem) (tier1, total int) {
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1Floor(e) {
			continue
		}
		total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			if e.GroundingTier == types.TierLineText {
				tier1++
			}
		case types.GroundingRecovered, types.GroundingUngrounded:
			// not counted toward Tier-1
		default:
			// legacy / deterministic
			tier1++
		}
	}
	return tier1, total
}
