package tool

import (
	"crypto/sha256"
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Refresh only authenticated system blocks. Model-authored tables/prose are
// never rewritten or scanned, and an exact model selector retains ownership.
func materializeNativeFactDisplay(doc *types.AnswerDocumentV2, ctx *types.BusContext, view *types.AnswerSemanticView) {
	if doc == nil || ctx == nil || view == nil {
		return
	}
	var rm *types.RequestModel
	if ctx.AnalysisIR != nil {
		rm = &ctx.AnalysisIR.RequestModel
	}
	if rm == nil && ctx.Mutable != nil {
		rm = ctx.Mutable.RequestModel()
	}
	retained := make([]types.AnswerBlock, 0, len(doc.Blocks))
	used, selected := map[string]bool{}, map[string]bool{}
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
			continue
		}
		retained = append(retained, block)
		used[block.ID] = true
		if block.RuntimeMeasurement != nil {
			receipt := *block.RuntimeMeasurement
			if types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
				selected[receipt.ObservationID+"\x00"+string(receipt.View)] = true
			}
		}
	}
	doc.Blocks = retained
	for _, receipt := range types.RuntimeNativeFactDisplaySelections(rm, view.RuntimeMeasurementContract) {
		key := receipt.ObservationID + "\x00" + string(receipt.View)
		if selected[key] {
			continue
		}
		if !types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
			continue
		}
		digest := sha256.Sum256([]byte(receipt.ObservationID + "\x00" + string(receipt.View)))
		base := fmt.Sprintf("native-facts-%x", digest[:8])
		id := base
		for n := 1; used[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		used[id], selected[key] = true, true
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: id, Kind: types.BlockTable, RuntimeMeasurement: &receipt,
			SystemGeneratedKind: types.AnswerSystemGeneratedNativeFacts})
	}
}
