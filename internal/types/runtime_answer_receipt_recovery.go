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
		if len(block.RequestedDimensionIndices) > 0 {
			if view == nil {
				return false
			}
			if _, valid := BoundNativeDisplayDimensions(block, view.RuntimeMeasurementContract); !valid {
				return false
			}
		}
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
			// A recovered pair identifies a table, not permission to change its
			// surface. Only current native fact capabilities may supply prose or
			// a fact diagram; legacy measurements remain table-only.
			candidate := block
			candidate.RuntimeMeasurement = item.measurement
			if !runtimeRecoveredMeasurementSurfaceValid(candidate) {
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

func runtimeRecoveredMeasurementSurfaceValid(block AnswerBlock) bool {
	r := block.RuntimeMeasurement
	if r == nil || !r.IsBound() {
		return false
	}
	native := r.EventSelection != nil && r.BoundTable.EventData != nil || r.BoundTable.HasNativeFixedFacts()
	if block.Kind != BlockTable && (!native || block.Kind != BlockSummary && block.Kind != BlockDiagram) {
		return false
	}
	if !native {
		return true // Preserve the existing ordinary-table recovery contract.
	}
	return block.Text == "" && len(block.Items) == 0 && len(block.Columns) == 0 && block.Diagram == nil && block.RuntimeWorkRelation == nil &&
		len(block.EdgeAnchors) == 0 && len(block.ParticipantBoundaries) == 0 && len(block.RelationClaims) == 0 && len(block.ClaimUses) == 0 && block.RequestedRelationScope == "" &&
		(block.TraceCausalClaimCaliber == "" || block.Kind == BlockSummary && block.TraceCausalClaimCaliber == TraceCausalClaimNoConclusion)
}
