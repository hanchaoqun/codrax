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
	p.observeMeasurementViewDetails(table, complete, complete)
}

// Row publication and producer-note publication have separate display
// boundaries. Neither implies capture completeness or semantic authority.
// The exact full-table key keeps this diagnostic local to its own selector.
type answerDocMeasurementViewPresentation struct {
	rowsComplete  bool
	notesComplete bool
}

func (p *answerDocNativePresentationReceipt) measurementViewPresentation(table types.RuntimeMeasurementTable) answerDocMeasurementViewPresentation {
	if p == nil {
		return answerDocMeasurementViewPresentation{}
	}
	key := nativeFactHandoffViewKey(table)
	if detail, found := p.measurementViewDetails[key]; found {
		return detail
	}
	complete := p.measurementViews[key]
	return answerDocMeasurementViewPresentation{complete, complete}
}

func (p *answerDocNativePresentationReceipt) observeMeasurementViewDetails(table types.RuntimeMeasurementTable, rowsComplete, notesComplete bool) {
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
	if _, found := p.measurementViews[key]; found {
		prior := p.measurementViewPresentation(table)
		rowsComplete = rowsComplete && prior.rowsComplete
		notesComplete = notesComplete && prior.notesComplete
	}
	if p.measurementViewDetails == nil {
		p.measurementViewDetails = map[string]answerDocMeasurementViewPresentation{}
	}
	p.measurementViewDetails[key] = answerDocMeasurementViewPresentation{rowsComplete, notesComplete}
	p.measurementViews[key] = rowsComplete && notesComplete
}

// Narrative ownership and native publication are separate facts. A bounded
// preview cannot turn an accepted lifecycle field back into a second diagnosis.
// Neither ownership nor this dispatch-local projection grants evidence authority.
type answerDocNativeNarrativeProjection struct {
	text                         string
	ownsCurrentNarrative         bool
	independentUnits             bool
	selectedViews                int
	completeViews                int
	rowsCompleteViews            int
	notesCompleteViews           int
	interpretationBytes          int
	publishedInterpretationBytes int
	sanitized                    bool
}

func (p answerDocNativeNarrativeProjection) nativeViewsComplete() bool {
	return p.selectedViews > 0 && p.completeViews == p.selectedViews
}

// The runtime-only lane continues to suppress unstructured synthesis. Only an
// explicitly independent, current accepted unit handoff can cross that lane,
// and only after its native references rebind to this dispatch's exact sources
// and query scope. This is advisory transport, not validation of the units or
// their proposed support_refs. Restored intent cannot acquire this exception.
func renderAnswerDocRuntimeInterpretationUnits(ctx *types.AgentContext, p *answerDocNativePresentationReceipt) string {
	projection := answerDocSeparatedNativeFactHandoff(ctx, p)
	if !projection.ownsCurrentNarrative || !projection.independentUnits {
		return ""
	}
	return projection.text
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
	// Eligibility and rendering use the same immutable handoff snapshot.
	projection.independentUnits = h.InterpretationRole == types.InvestigationIndependentAnalysis && len(h.InterpretationUnits) > 0
	nativeReferencesCurrent := false
	// Only the private current completion credential plus fresh source binding
	// can own the current narrative. Restored JSON and stale references retain
	// their conservative advisory/legacy path, independent of display budgets.
	if accepted {
		if view := types.BuildAnswerSemanticViewForAgentContext(ctx); view != nil {
			if selected, ok := types.BindInvestigationNativeFactHandoff(h, view.RuntimeMeasurementContract); ok {
				projection.ownsCurrentNarrative = true
				nativeReferencesCurrent = types.InvestigationNativeRowReferencesCurrent(h, view.RuntimeMeasurementContract)
				projection.selectedViews = len(selected)
				for _, r := range selected {
					if p != nil && p.measurementViews[nativeFactHandoffViewKey(*r.BoundTable)] {
						projection.completeViews++
					}
					detail := p.measurementViewPresentation(*r.BoundTable)
					if detail.rowsComplete {
						projection.rowsCompleteViews++
					}
					if detail.notesComplete {
						projection.notesCompleteViews++
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
	preview := previewInvestigationInterpretationWithNativeReferences(h, budget, sanitizePriorDraftForSummary, nativeReferencesCurrent)
	projection.sanitized = preview.sanitized
	projection.interpretationBytes = preview.totalBytes
	projection.publishedInterpretationBytes = preview.publishedBytes
	var prose strings.Builder
	if preview.text != "" {
		prose.WriteString("## Independent Investigation Interpretation\n\nModel-authored hypotheses, diagnostic boundaries and source/business clues; advisory only, not native measurements or proof of a causal relation.\n\n")
		if len(h.InterpretationUnits) > 0 {
			prose.WriteString("Read each unit together with all its boundaries. Its support_refs are proposed locators, not verified evidence. Completion acceptance and delivery do not verify the unit's claims; if a unit conflicts with source-bound native facts or typed causal boundaries, use those facts/boundaries and do not promote the unit into a fact, ordered relation or root cause.\n\n")
		}
		prose.WriteString(preview.text + "\n\n")
	}
	if projection.sanitized {
		prose.WriteString("Interpretation safety filtering changed the displayed material; the original remains audit-only, not a second answer narrative.\n\n")
	}
	if projection.ownsCurrentNarrative && !projection.nativeViewsComplete() {
		fmt.Fprintf(&prose, "Native reference publication: %d of %d selected views are completely represented in this prompt; %d of %d selected views have all retained rows represented, and %d of %d have all producer notes represented. These are separate prompt-publication checks, not capture completeness. A failed check means partial or unconfirmed publication for that component, not missing source facts; narrative ownership does not complete it.\n\n", projection.completeViews, projection.selectedViews, projection.rowsCompleteViews, projection.selectedViews, projection.notesCompleteViews, projection.selectedViews)
	}
	projection.text = prose.String()
	return projection
}
