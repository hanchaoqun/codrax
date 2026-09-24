package types

// RebindRuntimeAnswerReceipts restores system-owned facts after a saved answer
// crossed a JSON boundary. Selectors remain model-visible; bound rows/tables
// must come from the current accepted evidence, never from the saved answer.
// All receipts are rebound on private copies before any block is changed. A
// missing or stale choice therefore cannot partially replace an accepted draft.
func RebindRuntimeAnswerReceipts(doc *AnswerDocumentV2, view *AnswerSemanticView) bool {
	if doc == nil {
		return false
	}
	type binding struct {
		index       int
		measurement *AnswerRuntimeMeasurementReceipt
		work        *AnswerRuntimeWorkRelationReceipt
	}
	var bindings []binding
	for i, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil && block.RuntimeWorkRelation == nil {
			continue
		}
		if view == nil || (block.RuntimeMeasurement != nil && block.RuntimeWorkRelation != nil) {
			return false
		}
		item := binding{index: i}
		if block.RuntimeMeasurement != nil {
			item.measurement = block.RuntimeMeasurement.Clone()
			if !BindRuntimeMeasurementReceipt(item.measurement, view.RuntimeMeasurementContract) {
				return false
			}
		}
		if block.RuntimeWorkRelation != nil {
			receipt := *block.RuntimeWorkRelation
			// Discard private state even when the current candidate was retained
			// in memory: an earlier successful binding is not current authority.
			receipt.BoundRow = RuntimeWorkRelationRow{}
			if !BindRuntimeWorkRelationReceipt(&receipt, view.RuntimeWorkRelationContract) {
				return false
			}
			receipt.BoundRow.AllowedConclusions = append([]RuntimeWorkRelationConclusion(nil), receipt.BoundRow.AllowedConclusions...)
			item.work = &receipt
		}
		bindings = append(bindings, item)
	}
	for _, item := range bindings {
		doc.Blocks[item.index].RuntimeMeasurement = item.measurement
		doc.Blocks[item.index].RuntimeWorkRelation = item.work
	}
	return true
}
