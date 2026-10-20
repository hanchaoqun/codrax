package agent

import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Round-robin exact query groups only for the bounded flat sampling preview.
// Each group's original ranking is retained. This never changes the ledger,
// causal event order or source identity, and cannot make an oversized row fit.
func samplingMemberPresentationOrder(ledger types.ObservationLedger, p *answerDocNativePresentationReceipt, selected map[string]bool) []types.ObservationRecord {
	if p == nil {
		return nil
	}
	var groups [][]types.ObservationRecord
	indices := map[string]int{}
	total := 0
	for _, r := range ledger.Records {
		_, filtered, _ := tool.TraceQueryFilteredPerfMemberSelection(r)
		filtered = filtered && selected[filteredPerfMemberPresentationKey(r)]
		if r.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) ||
			(r.Predicate != "perf_sample_top_symbol" && r.Predicate != "perf_sample_top_callchain") ||
			(!filtered && !p.samplingSelections[nativePresentationQueryKey(r)]) {
			continue
		}
		key := nativePresentationQueryKey(r)
		index, exists := indices[key]
		if !exists {
			index = len(groups)
			indices[key] = index
			groups = append(groups, nil)
		}
		groups[index] = append(groups[index], r)
		total++
	}
	ordered := make([]types.ObservationRecord, 0, total)
	for offset := 0; len(ordered) < total; offset++ {
		for _, group := range groups {
			if offset < len(group) {
				ordered = append(ordered, group[offset])
			}
		}
	}
	return ordered
}
