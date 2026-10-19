package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

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

// Narrative ownership and native publication are separate facts. A bounded
// preview cannot turn an accepted lifecycle field back into a second diagnosis.
// Neither ownership nor this dispatch-local projection grants evidence authority.
type answerDocNativeNarrativeProjection struct {
	text                         string
	ownsCurrentNarrative         bool
	selectedViews                int
	completeViews                int
	interpretationBytes          int
	publishedInterpretationBytes int
	sanitized                    bool
}

func (p answerDocNativeNarrativeProjection) nativeViewsComplete() bool {
	return p.selectedViews > 0 && p.completeViews == p.selectedViews
}

func answerDocSeparatedNativeFactHandoff(ctx *types.AgentContext, p *answerDocNativePresentationReceipt) answerDocNativeNarrativeProjection {
	var projection answerDocNativeNarrativeProjection
	if ctx == nil || ctx.Mutable == nil {
		return projection
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
		return projection
	}
	// Only the private current completion credential plus fresh source binding
	// can own the current narrative. Restored JSON and stale references retain
	// their conservative advisory/legacy path, independent of display budgets.
	if accepted {
		if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil {
			if selected, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); ok {
				projection.ownsCurrentNarrative = true
				projection.selectedViews = len(selected)
				for _, r := range selected {
					if p != nil && p.measurementViews[nativeFactHandoffViewKey(*r.BoundTable)] {
						projection.completeViews++
					}
				}
			}
		}
	}
	originalInterpretation := strings.TrimSpace(h.Interpretation)
	interpretation := sanitizePriorDraftForSummary(originalInterpretation)
	projection.sanitized = interpretation != originalInterpretation
	projection.interpretationBytes = len(interpretation)
	budget := 900
	if projection.ownsCurrentNarrative {
		// Reuse the former interpretation + lifecycle-synthesis slots. This
		// does not enlarge the total narrative budget or independent note slots.
		budget += 900
	}
	end := min(len(interpretation), budget)
	for end > 0 && !utf8.ValidString(interpretation[:end]) {
		end--
	}
	publishedInterpretation := interpretation[:end]
	projection.publishedInterpretationBytes = end
	var prose strings.Builder
	if interpretation != "" {
		prose.WriteString("## Independent Investigation Interpretation\n\nModel-authored hypotheses, diagnostic boundaries and source/business clues; advisory only, not native measurements or proof of a causal relation.\n\n")
		prose.WriteString(publishedInterpretation + "\n\n")
		if end < len(interpretation) {
			fmt.Fprintf(&prose, "Interpretation is partial: %d of %d sanitized UTF-8 bytes shown; %d bytes omitted. The full original is retained in the accepted completion's native_fact_handoff.interpretation for audit; its omitted tail is not present in this view.\n\n", end, len(interpretation), len(interpretation)-end)
		}
	}
	if projection.sanitized {
		prose.WriteString("Interpretation safety filtering changed the displayed material; the original remains audit-only, not a second answer narrative.\n\n")
	}
	if projection.ownsCurrentNarrative && !projection.nativeViewsComplete() {
		fmt.Fprintf(&prose, "Native reference publication: %d of %d selected views are completely represented in this prompt. Other selected views remain partial or absent; narrative ownership does not make their publication complete.\n\n", projection.completeViews, projection.selectedViews)
	}
	projection.text = prose.String()
	return projection
}
