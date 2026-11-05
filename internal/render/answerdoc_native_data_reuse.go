package render

import (
	"sort"

	"github.com/hanchaoqun/codrax/internal/types"
)

type nativeMeasurementDataPlacement struct {
	number    int
	reference bool
}

// Plan whole displayed bodies, not textual matches. Only an authenticated
// system-owned table may use an earlier complete native table; model-owned
// blocks, summaries, graphs and partial selections are never suppressed.
// Original document/receipts stay untouched and each query keeps its notes.
func answerDocumentNativeDataReuse(doc *types.AnswerDocumentV2) map[int]nativeMeasurementDataPlacement {
	out := map[int]nativeMeasurementDataPlacement{}
	first := map[string]int{}
	groups := map[int][]int{}
	for index, block := range doc.Blocks {
		if block.Kind != types.BlockTable || block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() || block.RuntimeMeasurement.EventSelection != nil {
			continue
		}
		key := block.RuntimeMeasurement.BoundTable.NativeDisplayDataKey()
		if key == "" {
			continue
		}
		prior, found := first[key]
		if !found {
			first[key] = index
		} else if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
			groups[prior] = append(groups[prior], index)
		}
	}
	indices := make([]int, 0, len(groups))
	for index := range groups {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	for i, index := range indices {
		out[index] = nativeMeasurementDataPlacement{number: i + 1}
		for _, duplicate := range groups[index] {
			out[duplicate] = nativeMeasurementDataPlacement{number: i + 1, reference: true}
		}
	}
	return out
}
