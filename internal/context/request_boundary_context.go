package context

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Compact request-side metadata shared by all read stages; it is not evidence
// and cannot overwrite the source request, causal authority, or write policy.
func appendRequestBoundaryContext(pc *types.PromptContext, ac *types.AgentContext) {
	if ac == nil || ac.Stage.IsWrite() {
		return
	}
	var b strings.Builder
	var request *types.RequestModel
	if ac.AnalysisIR != nil {
		request = &ac.AnalysisIR.RequestModel
	}
	if outcomes := types.EffectiveRequestRouteHint(request, ac.TurnRouteHint).RequiredOutcomes; outcomes != 0 {
		fmt.Fprintf(&b, "Requested result kinds: %q. Preserve each independently in analysis, evidence gathering and final answer.", outcomes.Names())
		if outcomes.Has(types.TurnOutcomeSourceExplanation) {
			b.WriteString(" A completed measurement does not satisfy a source explanation.")
		}
		b.WriteString(" These are obligations, not evidence or permission to write.\n")
	}
	if ac.AnalysisIR != nil && len(ac.AnalysisIR.RequestModel.RuntimeThreadLookups) > 0 {
		data, _ := json.Marshal(ac.AnalysisIR.RequestModel.RuntimeThreadLookups)
		fmt.Fprintf(&b, "Thread lookup inputs (not diagnostic/root-cause subjects): %s. Use them to locate the requested process/group/records. Do not invent the containing process or promote lookup identities into runtime_targets. Explicit query selectors and time windows remain authoritative.\n", data)
	}
	if b.Len() > 0 {
		pc.SystemSections = append(pc.SystemSections, types.PromptSection{Title: "Requested results and lookup inputs", Content: b.String()})
	}
}
