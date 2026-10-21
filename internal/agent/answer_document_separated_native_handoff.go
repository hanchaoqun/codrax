package agent

import (
	"encoding/json"
	"fmt"
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
	h, accepted := currentInvestigationNarrativeHandoff(ctx)
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
	budget := 900
	if projection.ownsCurrentNarrative {
		// Reuse the former interpretation + lifecycle-synthesis slots. This
		// does not enlarge the total narrative budget or independent note slots.
		budget += 900
	}
	preview := previewInvestigationInterpretation(h, budget, sanitizePriorDraftForSummary)
	projection.sanitized = preview.sanitized
	projection.interpretationBytes = preview.totalBytes
	projection.publishedInterpretationBytes = preview.publishedBytes
	var prose strings.Builder
	if preview.text != "" {
		prose.WriteString("## Independent Investigation Interpretation\n\nModel-authored hypotheses, diagnostic boundaries and source/business clues; advisory only, not native measurements or proof of a causal relation.\n\n")
		prose.WriteString(preview.text + "\n\n")
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
