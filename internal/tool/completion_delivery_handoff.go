package tool

import (
	"encoding/json"
	"strings"

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

const ManagedDeliveryNativeFactHandoffTeaching = "A named delivery is already accepted by the analysis pipeline and source-bound native measurements are available. Provide native_fact_handoff: select the applicable runtime_measurements observation_id/view pairs and put every independent conclusion, diagnostic uncertainty, negative finding, source/business clue and scope boundary in interpretation. Avoid duplicating values fully carried by the selected native views; preserve independent derived quantities, causal projections and on-chain business findings not carried by those views. Do not copy intermediate delivery status there. Reason is lifecycle rationale only; the terminal component supplies actual delivery status, execution counts and links. No publication or whole-question success is implied."

const optionalNativeFactHandoffTeaching = "When selecting native_fact_handoff, copy applicable current runtime_measurements observation_id/view pairs as whole objects and put every independent conclusion, diagnostic uncertainty, negative finding, source/business clue and scope boundary in interpretation. Reason is lifecycle rationale only with this contract. Avoid duplicating values fully carried by the selected native views; preserve independent derived quantities, causal projections and on-chain business findings not carried by those views. Do not copy intermediate delivery status into interpretation. Omit this optional handoff to retain the legacy reason path. Neither choice waives evidence, causal or completion requirements."

// NativeFactHandoffCompletionTeaching shares narrative ownership with schema,
// retries and navigation. It makes no claim that collection is complete.
func NativeFactHandoffCompletionTeaching(ctx *types.BusContext) string {
	if ManagedDeliveryRequiresNativeFactHandoff(ctx) {
		return ManagedDeliveryNativeFactHandoffTeaching
	}
	if len(NativeFactHandoffChoices(ctx)) > 0 {
		return optionalNativeFactHandoffTeaching
	}
	return ""
}

type nativeFactHandoffViolation struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

type managedDeliveryNativeHandoffError struct {
	required   bool
	violations []nativeFactHandoffViolation
	choices    []types.AnswerRuntimeMeasurementReceipt
}

func (e managedDeliveryNativeHandoffError) hint() string {
	var b strings.Builder
	for _, issue := range e.violations {
		b.WriteString(issue.Field + ": " + issue.Code + ". ")
	}
	if e.required {
		b.WriteString(ManagedDeliveryNativeFactHandoffTeaching)
	} else {
		b.WriteString(optionalNativeFactHandoffTeaching)
	}
	if len(e.choices) > 0 {
		choices, _ := json.Marshal(e.choices)
		b.WriteString(" Copy selected whole objects from these current pairs; do not reconstruct IDs, mix views, or copy every pair unless relevant (select 1–16 unique pairs): ")
		b.Write(choices)
	} else {
		b.WriteString(" No eligible native measurement pairs are currently available; do not invent a replacement. Retain independent analysis through the legacy reason path when the handoff is optional.")
	}
	return b.String()
}

func (e managedDeliveryNativeHandoffError) Error() string {
	return "emit_investigation_complete rejected: " + e.hint()
}

func managedDeliveryNativeHandoffRepair(err error) *types.ToolRepair {
	e, ok := err.(managedDeliveryNativeHandoffError)
	if !ok {
		return nil
	}
	code := "native_fact_handoff"
	if e.required {
		code = "managed_delivery_native_handoff"
	}
	choices, _ := json.Marshal(e.choices)
	violations, _ := json.Marshal(e.violations)
	fields := make([]string, 0, len(e.violations))
	for _, issue := range e.violations {
		fields = append(fields, issue.Field)
	}
	return attachToolJSONSurfaceMetadata("emit_investigation_complete", &types.ToolRepair{
		Code: code, Hint: e.hint(), Fields: fields,
		Metadata: map[string]string{"current_runtime_measurements": string(choices), "native_fact_handoff_violations": string(violations)},
	})
}
