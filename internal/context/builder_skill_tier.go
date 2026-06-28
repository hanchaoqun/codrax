// Package context — builder_skill_tier.go (P5-B step 3, 2026-05-10).
//
// Bridges the dispatch's runtime AgentContext into the skill
// package's AppliesToContext / TierBItem.ShouldRender API. Renders
// the WorkflowTierB / ProhibitionsTierB items inline with their
// Tier A counterparts when the applicability filter matches, OR
// when a matching ViolationKind is present in the prior emit's
// retry state.
//
// Skills that declare zero Tier B items (every skill except
// answer-document-skill at this commit) keep behaviour byte-
// identical to pre-P5 — the helpers walk the empty Tier B slice
// and return the unmodified Tier A list.
//
// Design doc: docs/design/finalizer_skill_restructure.md §6.2.
package context

import (
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// skillTierAwareWorkflow returns the rendered workflow list for the
// current dispatch — Tier A items always, plus matching Tier B
// items based on the applicability filter and retry violations.
//
// Order: Tier A first (preserved order), then matching Tier B in
// declaration order. The order matters for LLM attention — Tier A
// items get the structural-priority slots while Tier B fills the
// remaining capacity.
func skillTierAwareWorkflow(ac *types.AgentContext, sk *skill.Config) []string {
	if sk == nil {
		return nil
	}
	out := append([]string(nil), sk.Workflow...)
	if len(sk.WorkflowTierB) == 0 {
		return out
	}
	dispatchCtx := buildAppliesToContext(ac)
	for _, item := range sk.WorkflowTierB {
		if item.ShouldRender(dispatchCtx) {
			out = append(out, item.Body)
		}
	}
	return out
}

// skillTierAwareProhibitions mirrors skillTierAwareWorkflow for the
// Prohibitions surface. Identical filter / order semantics.
func skillTierAwareProhibitions(ac *types.AgentContext, sk *skill.Config) []string {
	if sk == nil {
		return nil
	}
	out := append([]string(nil), sk.Prohibitions...)
	if len(sk.ProhibitionsTierB) == 0 {
		return out
	}
	dispatchCtx := buildAppliesToContext(ac)
	for _, item := range sk.ProhibitionsTierB {
		if item.ShouldRender(dispatchCtx) {
			out = append(out, item.Body)
		}
	}
	return out
}

// buildAppliesToContext projects the dispatch's runtime state into
// the typed AppliesToContext the TierBItem filter consults. Reads
// only stable fields (no mutation). Conservative defaults: when
// data is missing, the corresponding flag is false (the Tier B
// item with that requirement stays hidden — fail-loud over
// fail-open).
func buildAppliesToContext(ac *types.AgentContext) skill.AppliesToContext {
	if ac == nil {
		return skill.AppliesToContext{}
	}
	out := skill.AppliesToContext{}

	// Intent + principal kind from the AnalysisIR / view.
	if ac.AnalysisIR != nil {
		out.Intent = ac.AnalysisIR.RequestModel.Intent
	}
	if view := types.BuildAnswerSemanticViewForAgentContext(ac); view != nil {
		// PrincipalKind = the kind of the first RequiredBlock with
		// SurfaceRoleHint == principal. When no principal-tagged
		// requirement exists, fall back to the first RequiredBlock's
		// kind (defensive — every family has at least one block req).
		for _, req := range view.RequiredBlocks {
			if req.SurfaceRoleHint == types.SurfacePrincipal {
				out.PrincipalKind = req.Kind
				break
			}
		}
		if out.PrincipalKind == "" && len(view.RequiredBlocks) > 0 {
			out.PrincipalKind = view.RequiredBlocks[0].Kind
		}
		// Diagram requirement.
		if view.DiagramPlan != nil && view.DiagramPlan.Required {
			out.HasDiagram = true
		}
	}

	// Log-triage attached.
	if ac.Mutable != nil {
		if ac.Mutable.LogTriage() != nil || ac.Mutable.PerfTrace() != nil {
			out.HasLog = true
		}
		if ac.Mutable.PerfTrace() != nil {
			out.HasTrace = true
		}
	}
	if ac.PerfTrace != nil || ac.AttachedHitrace != "" || ac.AttachedHitraceSource != "" {
		out.HasTrace = true
	}
	if ac.AnalysisIR != nil && ac.AnalysisIR.RequestModel.PerfTrace != nil {
		out.HasTrace = true
	}

	// Buckets — projected from RequestModel.Buckets when non-empty.
	if ac.AnalysisIR != nil && len(ac.AnalysisIR.RequestModel.Buckets) > 0 {
		out.HasBuckets = true
	}

	// Absence-contract path — when the analyzer's ExactResolution
	// contract carries AllowAbsence=true, the question is
	// structurally absence-shaped (the answer MAY conclude the
	// target doesn't exist). The Tier B absence-citation rule
	// applies in that mode.
	if ac.AnalysisIR != nil {
		if er := ac.AnalysisIR.AnswerContract.ExactResolution; er != nil && er.AllowAbsence {
			out.IsAbsence = true
		}
	}

	// Retry violations — read from RetryState's ActiveViolations
	// when present. The TierBItem.OnViolation gate consults this
	// to admit rules that just fired in the prior emit, even when
	// the AppliesTo gate would hide them.
	if ac.Mutable != nil {
		if rs := ac.Mutable.RetryState(); rs != nil && len(rs.ActiveViolations) > 0 {
			seen := make(map[types.ViolationKind]bool, len(rs.ActiveViolations))
			for _, sv := range rs.ActiveViolations {
				if !seen[sv.Kind] {
					seen[sv.Kind] = true
					out.RetryViolations = append(out.RetryViolations, sv.Kind)
				}
			}
		}
	}

	return out
}
