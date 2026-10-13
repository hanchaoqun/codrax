package tool

import (
	"crypto/sha256"
	"fmt"
	"reflect"

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
	used := map[string]bool{}
	selected := map[string][]types.RuntimeMeasurementTable{}
	for _, block := range doc.Blocks {
		if block.SystemGeneratedKind == types.AnswerSystemGeneratedNativeFacts {
			continue
		}
		retained = append(retained, block)
		used[block.ID] = true
		if block.Kind == types.BlockTable && block.RuntimeMeasurement != nil {
			receipt := *block.RuntimeMeasurement
			if types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
				// Group counts and other surfaces cannot replace the complete
				// member table. Match an ungrouped projection to its exact native
				// population below; a subset/field selection is not full coverage.
				if receipt.EventSelection != nil && len(receipt.EventSelection.GroupBy) > 0 {
					continue
				}
				canonical := receipt
				canonical.EventSelection = nil
				key := types.RuntimeMeasurementDisplayKey(canonical, view.RuntimeMeasurementContract)
				selected[key] = append(selected[key], *receipt.BoundTable)
			}
		}
	}
	doc.Blocks = retained
	for _, receipt := range types.RuntimeNativeFactDisplaySelections(rm, view.RuntimeMeasurementContract) {
		key := types.RuntimeMeasurementDisplayKey(receipt, view.RuntimeMeasurementContract)
		if !types.BindRuntimeMeasurementReceipt(&receipt, view.RuntimeMeasurementContract) {
			continue
		}
		covered := false
		for _, table := range selected[key] {
			if reflect.DeepEqual(table.Columns, receipt.BoundTable.Columns) && reflect.DeepEqual(table.Rows, receipt.BoundTable.Rows) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		digest := sha256.Sum256([]byte(receipt.ObservationID + "\x00" + string(receipt.View)))
		base := fmt.Sprintf("native-facts-%x", digest[:8])
		id := base
		for n := 1; used[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		used[id] = true
		selected[key] = append(selected[key], *receipt.BoundTable)
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: id, Kind: types.BlockTable, RuntimeMeasurement: &receipt,
			SystemGeneratedKind: types.AnswerSystemGeneratedNativeFacts})
	}
}
