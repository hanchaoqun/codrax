package tool

import (
	"github.com/hanchaoqun/codrax/internal/types"
)

// NativeFactHandoffChoices is the shared bounded schema/repair roster. These
// are current source-bound measurements, not copied values or query previews.
func NativeFactHandoffChoices(ctx *types.BusContext) []types.AnswerRuntimeMeasurementReceipt {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return nil
	}
	var out []types.AnswerRuntimeMeasurementReceipt
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.EventData != nil {
			continue
		}
		out = append(out, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		if len(out) == 32 {
			break
		}
	}
	return out
}

// ManagedDeliveryRequiresNativeFactHandoff reads private host state and fresh
// source authorization. A path, model claim, JSON receipt or stage label cannot
// enable it. Failed/cancelled requests retain their existing failure lane.
func ManagedDeliveryRequiresNativeFactHandoff(ctx *types.BusContext) bool {
	if ctx == nil || ctx.PipelineExports == nil || contextFromBus(ctx).Err() != nil {
		return false
	}
	requests, err := AuthorizedPendingTraceCatalogExports(ctx)
	if err != nil {
		return false
	}
	for _, request := range requests {
		if request.Status == "pending_publication" || request.Status == "published" {
			return len(NativeFactHandoffChoices(ctx)) > 0
		}
	}
	return false
}

const ManagedDeliveryNativeFactHandoffTeaching = "A named delivery is already accepted by the analysis pipeline and source-bound native measurements are available. Provide native_fact_handoff: select the applicable runtime_measurements observation_id/view pairs and put every independent conclusion, diagnostic uncertainty, negative finding, source/business clue and scope boundary in interpretation. Do not copy native values or intermediate delivery status there. Reason is lifecycle rationale only; the terminal component supplies actual delivery status, execution counts and links. No publication or whole-question success is implied."

type managedDeliveryNativeHandoffError struct{}

func (managedDeliveryNativeHandoffError) Error() string {
	return "emit_investigation_complete rejected: " + ManagedDeliveryNativeFactHandoffTeaching
}

func managedDeliveryNativeHandoffRepair(err error) *types.ToolRepair {
	if _, ok := err.(managedDeliveryNativeHandoffError); !ok {
		return nil
	}
	return attachToolJSONSurfaceMetadata("emit_investigation_complete", &types.ToolRepair{
		Code: "managed_delivery_native_handoff", Hint: ManagedDeliveryNativeFactHandoffTeaching,
		Fields: []string{"native_fact_handoff.runtime_measurements", "native_fact_handoff.interpretation", "reason"},
	})
}
