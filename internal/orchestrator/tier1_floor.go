package orchestrator

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/render"
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
	contract := types.BuildExactResolutionContract(ir.RequestModel)
	stableAbsent := strings.EqualFold(strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()), "absence") &&
		strings.TrimSpace(o.busCtx.Mutable.StableAbsenceJustification()) != ""
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, o.busCtx.Mutable)
	tier1, total := countTier1Evidence(evidence, contract, ir.RequestModel.Scenario, stableAbsent, requiredFiles, ir.RequestModel)
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
	fmt.Fprintf(&b, "Line-text-grounded ratio %.0f%% (%d grounded-via-line_text / %d total) below floor %.0f%%.",
		ratio*100, tier1, total, floor*100)
	b.WriteString(" Citation grounding at answer-render time is stricter and will reject anchors that were never read via read_file — ")
	b.WriteString("call read_file on the recovered sources before declaring the investigation complete.")
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
func countTier1Evidence(
	evidence []types.EvidenceItem,
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm types.RequestModel,
) (tier1, total int) {
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(e, contract, scenario, stableAbsent, requiredFiles, rm) {
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

type groundingHealth struct {
	total     int
	accepted  int
	tier1     int
	recovered int
}

func (h groundingHealth) groundingRatio() float64 {
	if h.total == 0 {
		return 1
	}
	return float64(h.accepted) / float64(h.total)
}

func (h groundingHealth) tier1Ratio() float64 {
	if h.total == 0 {
		return 1
	}
	return float64(h.tier1) / float64(h.total)
}

func countGroundingHealth(
	evidence []types.EvidenceItem,
	contract *types.ExactResolutionContract,
	scenario types.Scenario,
	stableAbsent bool,
	requiredFiles []string,
	rm types.RequestModel,
) groundingHealth {
	var h groundingHealth
	for _, e := range evidence {
		if !types.EvidenceCountsTowardTier1FloorInContext(e, contract, scenario, stableAbsent, requiredFiles, rm) {
			continue
		}
		h.total++
		switch e.GroundingStatus {
		case types.GroundingGrounded:
			h.accepted++
			if e.GroundingTier == types.TierLineText {
				h.tier1++
			}
		case types.GroundingRecovered:
			h.accepted++
			h.recovered++
		case types.GroundingUngrounded:
			// Advisory warning target.
		default:
			// Legacy deterministic facts predate the grounding fields;
			// match the hard gates and count them as Tier-1.
			h.accepted++
			h.tier1++
		}
	}
	return h
}

func (o *Orchestrator) warnLowGroundingIfNeeded(ir *types.AnalysisIR, warned *bool) {
	if warned != nil && *warned {
		return
	}
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil || ir == nil {
		return
	}
	policy := tool.CurrentGroundingPolicy()
	if policy.WarnGroundingFloor <= 0 && policy.WarnTier1Floor <= 0 {
		return
	}
	evidence := o.busCtx.Mutable.EmittedEvidence()
	if len(evidence) == 0 {
		return
	}
	contract := types.BuildExactResolutionContract(ir.RequestModel)
	stableAbsent := strings.EqualFold(strings.TrimSpace(o.busCtx.Mutable.StableInvestigationResultKind()), "absence") &&
		strings.TrimSpace(o.busCtx.Mutable.StableAbsenceJustification()) != ""
	requiredFiles := types.ExactResolutionRequiredContextFiles(contract, o.busCtx.Mutable)
	health := countGroundingHealth(evidence, contract, ir.RequestModel.Scenario, stableAbsent, requiredFiles, ir.RequestModel)
	if health.total == 0 {
		return
	}
	groundingRatio := health.groundingRatio()
	tier1Ratio := health.tier1Ratio()
	lowGrounding := policy.WarnGroundingFloor > 0 && groundingRatio < policy.WarnGroundingFloor
	lowTier1 := policy.WarnTier1Floor > 0 && tier1Ratio < policy.WarnTier1Floor
	if !lowGrounding && !lowTier1 {
		return
	}
	if warned != nil {
		*warned = true
	}
	profile := strings.TrimSpace(string(policy.Profile))
	if profile == "" {
		profile = string(tool.GroundingProfileCustom)
	}
	groundingPct := int(groundingRatio*100 + 0.5)
	tier1Pct := int(tier1Ratio*100 + 0.5)
	logging.Warning("[orchestrator] low grounding health: profile=%s grounding_ratio=%.2f tier1_ratio=%.2f accepted=%d recovered=%d tier1=%d total=%d warn_grounding_floor=%.2f warn_tier1_floor=%.2f",
		profile, groundingRatio, tier1Ratio, health.accepted, health.recovered, health.tier1, health.total, policy.WarnGroundingFloor, policy.WarnTier1Floor)
	o.emit(render.Event{
		Kind:       render.EventOrchestratorNotice,
		Timestamp:  time.Now(),
		Agent:      "orchestrator",
		NoticeKind: render.NoticeLowGrounding,
		Reasoning:  lowGroundingWarningMessage(o.busCtx.Language, profile, groundingPct, tier1Pct),
	})
}
