package context

import (
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const factObservationBoundary = "These are recorded tool-output previews or unlinked legacy notes, not independently verified interpretations. Tool execution success and tool-category confidence do not prove that a calculation, explanation or root cause is correct. Typed facts retain their own source/window/unit and evidence contracts in the dedicated sections; these previews do not replace or override them. source_ref is the recorded navigation locator, evidence_ref locates the original result when available, and fact_slot is this list's index, not a measurement identity.\n\n"

// A single display projection for main/sub investigation facts. It neither
// matches prose to tool history nor alters evidence floors/causal qualification.
func formatRelevantFact(index int, fact types.RepoFact) string {
	value := trimKnownFactValue(fact.Key, fact.Value)
	if len(value) > maxFactValueLen {
		value = types.CutPrefixRuneSafe(value, maxFactValueLen) + "... [truncated]"
	}
	var provenance strings.Builder
	fmt.Fprintf(&provenance, "fact_slot=%d; source_ref=%q; ", index, fact.Source)
	if fact.EvidenceRef != "" {
		fmt.Fprintf(&provenance, "evidence_ref=%q; ", fact.EvidenceRef)
	} else {
		provenance.WriteString("evidence_ref=unavailable; ")
	}
	if observation, ok := fact.ToolObservation(); ok {
		fmt.Fprintf(&provenance, "recorded_tool_output=%q; typed_records=%d; ", observation.ToolName, observation.TypedRecords)
		if len(observation.Origins) > 0 {
			fmt.Fprintf(&provenance, "typed_origins=%q; ", observation.Origins)
		}
		if !observation.ObservedAt.IsZero() {
			fmt.Fprintf(&provenance, "observed_at=%q; ", observation.ObservedAt.Format(time.RFC3339Nano))
		}
	} else {
		provenance.WriteString("legacy_unlinked_note; ")
	}
	provenance.WriteString("interpretation_unverified")
	return fmt.Sprintf("[%s] %s = %s (%s)", fact.Key, fact.Key, value, provenance.String())
}
