package tool

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// materializeRuntimeTraceFrequencyAuthorityCaveat keeps frequency transition
// activity on the background lane even when the model prose promoted a large
// count into a low-frequency/throttling claim. The count itself never
// authorizes supply causality; any stronger wording must bind to the separate
// typed supply-evidence roster.
func materializeRuntimeTraceFrequencyAuthorityCaveat(doc *types.AnswerDocumentV2, ctx *types.BusContext) bool {
	if doc == nil || ctx == nil {
		return false
	}
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	results := append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	count := 0
	evidenceSet := map[string]bool{}
	for _, result := range results {
		if strings.TrimSpace(result.ToolName) != "trace_query" || result.TraceEvidenceAuthority == nil {
			continue
		}
		authority := result.TraceEvidenceAuthority
		count = max(count, authority.FrequencyTransitionEventCount)
		for _, token := range authority.FrequencyTypedSupplyEvidence {
			token = strings.TrimSpace(token)
			if token != "" {
				evidenceSet[token] = true
			}
		}
	}
	if count <= 0 {
		return false
	}
	evidence := make([]string, 0, len(evidenceSet))
	for token := range evidenceSet {
		evidence = append(evidence, token)
	}
	sort.Strings(evidence)
	conclusion := "unproven_from_transition_count"
	if len(evidence) > 0 {
		conclusion = "bounded_by_typed_supply_evidence"
	}
	zh := runtimeTraceCausalProjectionUseChinese(requestedAnswerDocumentLanguage(ctx))
	var caveat string
	if zh {
		caveat = fmt.Sprintf(
			"频率证据权限：transition_events=%d，transition_authority=background_only；事件计数只证明调频活动，不单独证明低频、降频、限频或计算供给不足。frequency_supply_conclusion=%s",
			count, conclusion,
		)
		if len(evidence) == 0 {
			caveat += "；typed_supply_evidence=none，任何低频/供给因果措辞均未获当前计数授权"
		} else {
			caveat += "；typed_supply_evidence=" + strings.Join(evidence, ",") +
				"；低频/供给措辞只能绑定这些 typed 证据及其链/排序口径，不能归因于 transition count"
		}
	} else {
		caveat = fmt.Sprintf(
			"Frequency evidence authority: transition_events=%d, transition_authority=background_only; the event count proves frequency-change activity only and does not by itself prove low frequency, throttling, a frequency limit, or compute-supply shortage. frequency_supply_conclusion=%s",
			count, conclusion,
		)
		if len(evidence) == 0 {
			caveat += "; typed_supply_evidence=none, so no low-frequency or supply-causal wording is authorized by the count"
		} else {
			caveat += "; typed_supply_evidence=" + strings.Join(evidence, ",") +
				"; low-frequency/supply wording must bind to that typed evidence and its chain/rank caliber, never to the transition count"
		}
	}
	for _, existing := range doc.Caveats {
		if strings.TrimSpace(existing) == caveat {
			return false
		}
	}
	doc.Caveats = append(doc.Caveats, caveat)
	return true
}
