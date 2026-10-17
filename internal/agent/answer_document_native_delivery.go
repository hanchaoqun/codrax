package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func renderAcceptedNativeAnswerDisplay(ctx *types.AgentContext, view *types.AnswerSemanticView) string {
	if ctx == nil || ctx.Mutable == nil || view == nil || ctx.Ctx != nil && ctx.Ctx.Err() != nil {
		return ""
	}
	rm := ctx.Mutable.RequestModel()
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	selected, unavailable := types.ResolveInvestigationNativeAnswerDisplay(ctx.Mutable.AcceptedInvestigationNativeFactHandoff(), rm, view.RuntimeMeasurementContract)
	if len(selected) == 0 && unavailable == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("### Accepted native answer display\n\nThe investigation explicitly selected these current native views for final display. The system preserves every retained row and all notes, independently of the prompt preview budget. Do not retype the values. You may position the same complete view through its supported runtime_measurement surface; a different view, subset, grouped summary or unrelated diagram does not replace it. This display plan grants no dimension, complete-member, source-code or causal authority and does not remove independent interpretation.\n")
	for _, r := range selected {
		fmt.Fprintf(&b, "- observation_id=%q view=%q; final_retained_rows=%d; all_notes=%d\n", r.ObservationID, r.View, len(r.BoundTable.Rows), len(r.BoundTable.Notes))
	}
	if unavailable > 0 {
		fmt.Fprintf(&b, "- Previously selected views no longer current: %d. Do not reconstruct them from old prose or preview values; the final display discloses unavailability.\n", unavailable)
	}
	return b.String() + "\n"
}
