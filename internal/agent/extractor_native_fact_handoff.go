package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Extraction has no native publication receipt. Keep the lifecycle reason,
// but do not call it an answer conclusion. Current selector rebinding only
// identifies the separate lanes; it neither suppresses facts nor waives gates.
func renderExtractorNativeFactHandoff(ctx *types.AgentContext, reason string, facts []types.AnswerAggregateFact) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if h == nil || view == nil {
		return ""
	}
	if _, valid := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); !valid {
		return ""
	}
	var b strings.Builder
	limit := 900
	if suppressUnstructuredClosureReasonForPrincipalMemberSets(ctx, facts) {
		limit = 700
	}
	reason = sanitizeAggregateExcludedCandidatesForPrompt(ctx, reason, facts)
	fmt.Fprintf(&b, "- lifecycle rationale only (not an answer conclusion or evidence): %s\n", truncateExtractorPromptText(sanitizeExtractorStageHandoffText(reason), limit))
	refs, _ := json.Marshal(h.RuntimeMeasurements)
	fmt.Fprintf(&b, "- Native measurement references (rebind current observations; these do not certify publication, completeness or causality): %s\n", refs)
	// Address freshness and prompt publication are separate contracts. Use the
	// same private source/query/table check as finalization; extraction's lack
	// of a publication receipt must not turn a current address into an unverified one.
	nativeReferencesCurrent := types.InvestigationNativeRowReferencesCurrent(h, view.RuntimeMeasurementContract)
	preview := previewInvestigationInterpretationWithNativeReferences(h, 900, func(s string) string {
		s = sanitizePriorDraftForSummary(sanitizeAggregateExcludedCandidatesForPrompt(ctx, s, facts))
		if len(h.InterpretationUnits) == 0 {
			// Legacy prose retains stage-navigation formatting. Structured
			// advisory units preserve opaque references and tool names: a
			// display-label rewrite is not removal of unsafe content and must
			// not silently discard the entire finding and its boundaries.
			s = sanitizeExtractorStageHandoffText(s)
		}
		return s
	}, nativeReferencesCurrent)
	if preview.text != "" {
		fmt.Fprintf(&b, "- Independent interpretation (model-authored advisory; typed evidence remains authoritative): %s\n", preview.text)
	}
	return b.String()
}

// Restored intent can restrict copying its own advisory text, but cannot restore
// private completion acceptance or source authority. Consumers that need those
// still check the returned credential and rebind current native references.
func currentInvestigationNarrativeHandoff(ctx *types.AgentContext) (*types.InvestigationNativeFactHandoff, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, false
	}
	if h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff(); h != nil {
		return h, true
	}
	if ta := ctx.Mutable.TurnAArtifacts(); ta != nil && strings.TrimSpace(ta.AcceptedClosureReason) == ctx.Mutable.StableInvestigationCompleteReason() {
		return ta.AcceptedNativeFactHandoff.Clone(), false
	}
	return nil, false
}

func projectInvestigationNarrativeNotes(ctx *types.AgentContext, notes []string) []string {
	h, _ := currentInvestigationNarrativeHandoff(ctx)
	out := make([]string, 0, len(notes))
	for _, note := range notes {
		// Byte identity only, never keywords, substring or semantic inference.
		if !h.IsAuditOnlyInterpretationCopy(note) {
			out = append(out, note)
		}
	}
	return out
}
