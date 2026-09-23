package context

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	toolDocumentationPromptMaxCount = 8
	toolDocumentationPromptMaxBytes = 128 << 10
)

// These are producer-authored contracts, not measurements or accepted source
// evidence. Never recover this lane by parsing ToolResult.Summary.
func toolDocumentationCarriers(results []types.ToolResult, explicit []types.ToolHandoffCarrier) []types.ToolHandoffCarrier {
	carriers := types.ToolHandoffCarriersFromTurnAInputs(results, nil, explicit)
	out := make([]types.ToolHandoffCarrier, 0, len(carriers))
	for _, carrier := range carriers {
		if carrier.Documentation != nil {
			out = append(out, carrier)
		}
	}
	return out
}

func formatToolDocumentation(ac *types.AgentContext) string {
	if ac == nil || (ac.Stage != types.StageExtract && ac.Stage != types.StageFinalize) {
		return ""
	}
	carriers := append([]types.ToolHandoffCarrier(nil), ac.ToolDocumentationCarriers...)
	if ac.Mutable != nil {
		if ta := ac.Mutable.TurnAArtifacts(); ta != nil {
			carriers = append(carriers, toolDocumentationCarriers(ta.ToolResults, ta.HandoffCarriers)...)
		}
	}
	carriers = toolDocumentationCarriers(nil, carriers)
	if len(carriers) == 0 {
		return ""
	}
	// Keep whole contracts, preferring later retained entries on overflow.
	// Deduplication is not an execution chronology. Head/tail excerpts could silently
	// discard prerequisites, units or missing-data policy mid-contract.
	kept := make([]string, 0, len(carriers))
	used := 0
	for i := len(carriers) - 1; i >= 0; i-- {
		carrier := carriers[i]
		doc := carrier.Documentation
		chunk := fmt.Sprintf("Producer: %s; schema: %s; document version: %d; selected view: %q; detail: %t\n```json\n%s\n```\n\n",
			carrier.ToolName, doc.Schema, doc.Version, doc.Selection.View, doc.Selection.Detail, doc.Content)
		if len(kept) == toolDocumentationPromptMaxCount || used+len(chunk) > toolDocumentationPromptMaxBytes {
			continue
		}
		kept = append(kept, chunk)
		used += len(chunk)
	}
	var b strings.Builder
	b.WriteString("The following are complete static contracts from successful tool calls in this investigation, not repository files and not runtime observations. They describe implemented capabilities, units, prerequisites and limitations; they do not prove capture availability, measured values, causal membership, source implementation or completed execution. Producer names identify earlier tools, not tools callable in this stage.\n\n")
	b.WriteString("Use these contracts directly for capability questions; do not reconstruct them from a prior model summary. Preserve distinctions between unsupported, missing, unknown and measured zero. Summary selections do not imply that omitted detail is unsupported. These are deduplicated retained contracts, not an execution chronology: do not merge conflicting versions or infer which is current from display order. For mixed questions, ground repository and runtime claims separately.\n\n")
	b.WriteString("Only retained contracts are shown: earlier handoff budgets may omit calls. Complete documents do not imply a complete call history.\n\n")
	b.WriteString("Answer JSON: these documents are not entries in citations[] or evidence_ids. Explain capabilities with ordinary uncited text/items, not invented file:line references or observation IDs. Use business-facing language; keep exact tool/parameter names only when useful to the user.\n\n")
	for i := len(kept) - 1; i >= 0; i-- {
		b.WriteString(kept[i])
	}
	if omitted := len(carriers) - len(kept); omitted > 0 {
		fmt.Fprintf(&b, "%d complete retained document(s) omitted to bound prompt size; this set is not exhaustive. No document was excerpted.\n", omitted)
	}
	return b.String()
}

func hasOnlyToolDocumentation(result types.ToolResult) bool {
	result = types.AttachToolHandoffCarrier(result)
	return result.Success && result.Handoff != nil && types.ToolHandoffCarrierIsDocumentationOnly(*result.Handoff) && len(result.Observations) == 0 && result.CommandMeasurement == nil && result.VCSHistory == nil
}
