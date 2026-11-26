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
	return nativeFactHandoffChoices(view.RuntimeMeasurementContract)
}

func nativeFactHandoffChoices(contract *types.RuntimeMeasurementContract) []types.AnswerRuntimeMeasurementReceipt {
	var out []types.AnswerRuntimeMeasurementReceipt
	for _, table := range contract.Choices() {
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

// NativeFactHandoffSchemaChoices uses one evidence snapshot for the model's
// measurement and display selectors. Managed delivery rechecks authorization.
func NativeFactHandoffSchemaChoices(ctx *types.BusContext) (choices, display []types.AnswerRuntimeMeasurementReceipt, required bool) {
	if ctx == nil || ctx.Mutable == nil {
		return nil, nil, false
	}
	view := types.BuildAnswerSemanticViewForBusContext(ctx)
	if view == nil {
		return nil, nil, false
	}
	choices = nativeFactHandoffChoices(view.RuntimeMeasurementContract)
	if len(choices) > 0 && contextFromBus(ctx).Err() == nil {
		display = nativeFactAnswerDisplayChoices(ctx, view.RuntimeMeasurementContract, choices)
		required = managedDeliveryHasNativeHandoffRequest(ctx)
	}
	return choices, display, required
}

// ManagedDeliveryRequiresNativeFactHandoff reads private host state and fresh
// source authorization. A path, model claim, JSON receipt or stage label cannot
// enable it. Failed/cancelled requests retain their existing failure lane.
func ManagedDeliveryRequiresNativeFactHandoff(ctx *types.BusContext) bool {
	return managedDeliveryHasNativeHandoffRequest(ctx) && len(NativeFactHandoffChoices(ctx)) > 0
}

func managedDeliveryHasNativeHandoffRequest(ctx *types.BusContext) bool {
	if ctx == nil || ctx.PipelineExports == nil || contextFromBus(ctx).Err() != nil {
		return false
	}
	requests, err := AuthorizedPendingTraceCatalogExports(ctx)
	if err != nil {
		return false
	}
	for _, request := range requests {
		if request.Status == "pending_publication" || request.Status == "published" {
			return true
		}
	}
	return false
}

const ManagedDeliveryNativeFactHandoffTeaching = "A named delivery is already accepted by the analysis pipeline and source-bound native measurements are available. Provide native_fact_handoff: select the applicable runtime_measurements observation_id/view pairs and put every independent conclusion, diagnostic uncertainty, negative finding, source/business clue and scope boundary in interpretation_units or legacy interpretation. Avoid duplicating values fully carried by the selected native views; preserve independent derived quantities, causal projections and on-chain business findings not carried by those views. Do not copy intermediate delivery status there. Reason is lifecycle rationale only; the terminal component supplies actual delivery status, execution counts and links. No publication or whole-question success is implied. " + types.InvestigationInterpretationFormTeaching + " " + types.NativeFactInterpretationRoleTeaching

const optionalNativeFactHandoffTeaching = "When selecting native_fact_handoff, copy applicable current runtime_measurements observation_id/view pairs as whole objects and put every independent conclusion, diagnostic uncertainty, negative finding, source/business clue and scope boundary in interpretation_units or legacy interpretation. Reason is lifecycle rationale only with this contract. Avoid duplicating values fully carried by the selected native views; preserve independent derived quantities, causal projections and on-chain business findings not carried by those views. Do not copy intermediate delivery status into the analysis narrative. Omit this optional handoff to retain the legacy reason path. Neither choice waives evidence, causal or completion requirements. " + types.InvestigationInterpretationFormTeaching + " " + types.NativeFactInterpretationRoleTeaching

func UsesCompactNativeFactHandoff(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	_, prepared := ctx.AttachedLogCatalog.PreparedFrameFact()
	return prepared && ctx.AttachedTraceMaterial.SinglePhysicalSource()
}

func nativeFactHandoffTeaching(base string, compact bool) string {
	if compact {
		return strings.NewReplacer(
			"select the applicable runtime_measurements observation_id/view pairs", "select exact native_refs rows or applicable measurement_refs strings from the enum",
			"copy applicable current runtime_measurements observation_id/view pairs as whole objects", "select exact native_refs rows or applicable measurement_refs strings from the enum",
		).Replace(base)
	}
	return base
}

// NativeFactHandoffCompletionTeaching shares narrative ownership with schema,
// retries and navigation. It makes no claim that collection is complete.
func NativeFactHandoffCompletionTeaching(ctx *types.BusContext) string {
	if ManagedDeliveryRequiresNativeFactHandoff(ctx) {
		return nativeFactHandoffTeaching(ManagedDeliveryNativeFactHandoffTeaching, UsesCompactNativeFactHandoff(ctx))
	}
	if len(NativeFactHandoffChoices(ctx)) > 0 {
		return nativeFactHandoffTeaching(optionalNativeFactHandoffTeaching, UsesCompactNativeFactHandoff(ctx))
	}
	return ""
}

type nativeFactHandoffViolation struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

type managedDeliveryNativeHandoffError struct {
	required       bool
	compact        bool
	violations     []nativeFactHandoffViolation
	choices        []types.AnswerRuntimeMeasurementReceipt
	displayChoices []types.AnswerRuntimeMeasurementReceipt
	coordinates    nativeFactCoordinateRepairs
	invalidRows    []types.InvalidInvestigationNativeRow
	rowUnits       []types.InvestigationInterpretationUnit
}

func (e managedDeliveryNativeHandoffError) hint() string {
	var b strings.Builder
	for _, issue := range e.violations {
		b.WriteString(issue.Field + ": " + issue.Code + ". ")
	}
	teaching := optionalNativeFactHandoffTeaching
	if e.required {
		teaching = ManagedDeliveryNativeFactHandoffTeaching
	}
	b.WriteString(nativeFactHandoffTeaching(teaching, e.compact))
	if len(e.coordinates.Entries) > 0 || e.coordinates.Omitted > 0 {
		coordinates, _ := json.Marshal(e.coordinates)
		b.WriteString(" Field-local current coordinates (advice only; not aliases or an automatic replacement): ")
		b.Write(coordinates)
		b.WriteString(" Select only task-applicable whole current objects after checking their source and view; ambiguous or absent matches do not prove identity. Retry is validated by the unchanged current source contract.")
	}
	if e.compact && len(e.choices) > 0 {
		refs := make([]string, 0, len(e.choices))
		for _, choice := range e.choices {
			refs = append(refs, NativeFactMeasurementRef(choice))
		}
		encoded, _ := json.Marshal(refs)
		b.WriteString(" Select current strings in measurement_refs or cite exact native_refs rows. answer_display does not select measurements. Keep the existing analysis when correcting the selection. Current measurement_refs: ")
		b.Write(encoded)
	} else if len(e.choices) > 0 && !e.coordinates.focused() {
		choices, _ := json.Marshal(e.choices)
		b.WriteString(" Copy selected whole objects from these current pairs; do not reconstruct IDs, mix views, or copy every pair unless relevant (select 1–16 unique pairs): ")
		b.Write(choices)
	} else if len(e.choices) == 0 {
		b.WriteString(" No eligible native measurement pairs are currently available; do not invent a replacement. Retain independent analysis through the legacy reason path when the handoff is optional.")
	}
	for _, issue := range e.violations {
		if issue.Field == "native_fact_handoff.answer_display" {
			b.WriteString(" " + types.NativeAnswerDisplayTeaching + " Remove unsupported display choices while retaining their independent investigation interpretation; retry with the current schema's answer_display pairs only.")
			available, _ := json.Marshal(e.displayChoices)
			b.WriteString(" Current answer_display pairs: " + string(available) + ".")
			break
		}
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
	metadata := map[string]string{"current_runtime_measurements": string(choices), "native_fact_handoff_violations": string(violations)}
	if len(e.invalidRows) > 0 {
		rows, _ := json.Marshal(e.invalidRows)
		units, _ := json.Marshal(e.rowUnits)
		metadata["invalid_native_rows"], metadata["current_interpretation_units"] = string(rows), string(units)
	}
	if len(e.coordinates.Entries) > 0 || e.coordinates.Omitted > 0 {
		coordinates, _ := json.Marshal(e.coordinates)
		metadata["native_fact_coordinate_repairs"] = string(coordinates)
	}
	fields := make([]string, 0, len(e.violations))
	for _, issue := range e.violations {
		fields = append(fields, issue.Field)
	}
	return attachToolJSONSurfaceMetadata("emit_investigation_complete", &types.ToolRepair{
		Code: code, Hint: e.hint(), Fields: fields,
		Metadata: metadata,
	})
}
