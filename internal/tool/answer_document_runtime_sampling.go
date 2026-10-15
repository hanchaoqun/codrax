package tool

import "github.com/hanchaoqun/codrax/internal/types"

const runtimeSamplingFactTeaching = "Published native sampling facts also support summary/table/diagram using the same observation_id/view pair, with no copied text/items/columns/diagram. Summary retains separate cohort rows and their own denominators; distribution is query-population quality, not hotspot-subset quality, and its counts measure sample records, not stack frames. Never add weights across events/units or infer elapsed time, calls or causes. A fact diagram contains no relationship edges."

func runtimeSamplingFactPairs(choices []types.RuntimeMeasurementTable) []any {
	var pairs []any
	for _, table := range choices {
		if table.HasNativeSamplingFacts() {
			pairs = append(pairs, map[string]any{"observation_id": table.ObservationID, "view": string(table.View)})
		}
	}
	return pairs
}

// This is shape normalization only. Exact current-pair binding subsequently
// rejects every non-event/non-sampling summary or diagram. No JSON can grant
// the private native sampling capability.
func runtimeMeasurementFactSurfaceCandidate(kind types.AnswerBlockKind, ref *types.AnswerRuntimeMeasurementReceipt) bool {
	return ref != nil && (kind == types.BlockSummary || kind == types.BlockTable || kind == types.BlockDiagram)
}

func runtimeMeasurementBoundFactSurface(block types.AnswerBlock) bool {
	r := block.RuntimeMeasurement
	return runtimeMeasurementFactSurfaceCandidate(block.Kind, r) && r.IsBound() &&
		(r.EventSelection != nil && r.BoundTable.EventData != nil || r.BoundTable.HasNativeSamplingFacts())
}

func runtimeSamplingSelectorAvailable(ref *types.AnswerRuntimeMeasurementReceipt, contracts ...*types.RuntimeMeasurementContract) bool {
	if ref == nil || len(contracts) != 1 {
		return false
	}
	copy := *ref
	return types.BindRuntimeMeasurementReceipt(&copy, contracts[0]) && copy.BoundTable.HasNativeSamplingFacts()
}

func runtimeMeasurementContractForBusContext(ctx *types.BusContext) *types.RuntimeMeasurementContract {
	return runtimeMeasurementContractForView(types.BuildAnswerSemanticViewForBusContext(ctx))
}
func runtimeMeasurementContractForView(view *types.AnswerSemanticView) *types.RuntimeMeasurementContract {
	if view == nil {
		return nil
	}
	return view.RuntimeMeasurementContract
}

// Recovery preserves an unbound pure selector, never a fact capability. A
// current-contract rebind is mandatory before an accepted display is rendered.
func normalizeRecoveredNativeFactBlock(raw emitAnswerBlockV2, path string) (types.AnswerBlock, error) {
	block, err := NormalizeEmitAnswerBlock(raw, path)
	if err == nil || raw.RuntimeMeasurement == nil || raw.RuntimeMeasurement.EventSelection != nil || (raw.Kind != "summary" && raw.Kind != "diagram") {
		return block, err
	}
	if check := validateEmitRuntimeMeasurementBlock(raw, path, true); check != nil {
		return block, check
	}
	kind := raw.Kind
	raw.Kind = "table"
	block, err = NormalizeEmitAnswerBlock(raw, path)
	block.Kind = types.AnswerBlockKind(kind)
	return block, err
}

func runtimeMeasurementFactRequiredSchema(events bool, sampling []any) map[string]any {
	if len(sampling) == 0 {
		return runtimeEventSelectorRequiredSchema()
	}
	s := map[string]any{"required": []string{"runtime_measurement"}, "properties": map[string]any{"runtime_measurement": map[string]any{"enum": sampling}}}
	return s
}
