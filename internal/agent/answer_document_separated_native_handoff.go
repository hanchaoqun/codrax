package agent

import (
	"encoding/json"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exact full public projection plus fresh contract binding, never the selector
// ID alone. This local receipt is written only after rows and all notes survive
// the roster's actual budgets. It confers no completeness or causal authority.
func nativeFactHandoffViewKey(table types.RuntimeMeasurementTable) string {
	b, err := json.Marshal(table)
	if err != nil || !table.IsValid() {
		return ""
	}
	return string(b)
}

func (p *answerDocNativePresentationReceipt) observeMeasurementView(table types.RuntimeMeasurementTable, complete bool) {
	if p == nil {
		return
	}
	key := nativeFactHandoffViewKey(table)
	if key == "" {
		return
	}
	if p.measurementViews == nil {
		p.measurementViews = map[string]bool{}
	}
	if prior, found := p.measurementViews[key]; found {
		complete = complete && prior
	}
	p.measurementViews[key] = complete
}

func answerDocSeparatedNativeFactHandoff(ctx *types.AgentContext, p *answerDocNativePresentationReceipt) (string, bool) {
	if ctx == nil || ctx.Mutable == nil {
		return "", false
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	accepted := h != nil
	if h == nil {
		// Restored JSON keeps model interpretation as advisory audit context,
		// but cannot restore the private accepted-completion credential.
		if ta := ctx.Mutable.TurnAArtifacts(); ta != nil && strings.TrimSpace(ta.AcceptedClosureReason) == ctx.Mutable.StableInvestigationCompleteReason() {
			h = ta.AcceptedNativeFactHandoff.Clone()
		}
	}
	if h == nil {
		return "", false
	}
	originalInterpretation := strings.TrimSpace(h.Interpretation)
	interpretation := sanitizePriorDraftForSummary(originalInterpretation)
	publishedInterpretation := truncateAnswerDocPromptText(interpretation, 900)
	var prose string
	if interpretation != "" {
		prose = "## Independent Investigation Interpretation\n\nModel-authored hypotheses, diagnostic boundaries and source/business clues; advisory only, not native measurements or proof of a causal relation.\n\n" + publishedInterpretation + "\n\n"
		if publishedInterpretation != interpretation {
			prose += "Interpretation exceeds this bounded view; lifecycle context is retained below.\n\n"
		}
	}
	if !accepted || p == nil || publishedInterpretation != originalInterpretation {
		return prose, false
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	if view == nil {
		return prose, false
	}
	selected, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract)
	if !ok {
		return prose, false
	}
	for _, r := range selected {
		if !p.measurementViews[nativeFactHandoffViewKey(*r.BoundTable)] {
			return prose, false
		}
	}
	return prose, true
}
