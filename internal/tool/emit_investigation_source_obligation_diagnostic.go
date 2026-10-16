package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// completionDeclaredSourceObligationDiagnostic explains already-declared
// inputs after a gate has rejected completion. It neither decides the gate nor
// infers/corrects evidence domains from request, quote, label, or answer prose.
func completionDeclaredSourceObligationDiagnostic(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	rm := &ctx.AnalysisIR.RequestModel
	var seats []string
	hint := types.EffectiveRequestRouteHint(rm, ctx.TurnRouteHint)
	if hint.CurrentSourceEvidenceMode == types.TurnRouteCurrentSourceEvidenceRequired {
		seats = append(seats, "current_source_evidence_mode=required")
	}
	if hint.RequiredOutcomes.Has(types.TurnOutcomeSourceExplanation) {
		seats = append(seats, "required_outcomes includes source_explanation")
	}
	if profile := rm.RequestedAnswerDimensions; profile.Active() {
		shown, omitted := 0, 0
		for _, dim := range profile.Dimensions {
			if !dim.Required || dim.Role != types.RequestedAnswerDimensionSourceLocation {
				continue
			}
			if shown == 3 {
				omitted++
				continue
			}
			quote := []rune(dim.SourceQuote)
			if len(quote) > 160 {
				quote = append(quote[:160], '…')
			}
			seats = append(seats, fmt.Sprintf("dimension index=%d role=%s source_quote=%q", dim.Index, dim.Role, string(quote)))
			shown++
		}
		if omitted > 0 {
			seats = append(seats, fmt.Sprintf("%d additional required source_location dimension(s) not shown", omitted))
		}
	}
	if len(seats) == 0 {
		return ""
	}
	return "Declared current-source inputs: " + strings.Join(seats, "; ") + ".\n" +
		"These declarations require current-checkout proof, not merely recorded artifact callsites or paths. Check the declared evidence domain; this diagnostic does not change it or waive the citation requirement.\n"
}
