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
	if interpretation := sanitizeExtractorStageHandoffText(sanitizePriorDraftForSummary(sanitizeAggregateExcludedCandidatesForPrompt(ctx, h.Interpretation, facts))); interpretation != "" {
		fmt.Fprintf(&b, "- Independent interpretation (model-authored advisory; typed evidence remains authoritative): %s\n", truncateExtractorPromptText(interpretation, 900))
	}
	return b.String()
}
