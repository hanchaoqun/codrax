package tracediag

import (
	"fmt"
	"reflect"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// The new scheduler inventory stays at its existing nested detail position,
// not a key-first/root-rank position. This exact typed renderer keeps measured
// zero counts and complete coordinates: the generic Summary-bearing Interval
// path would hide its timestamps and split ordinal/measurement into two lines.
// It neither reclassifies S/D/IO nor evaluates closure/Binder/cause. The engine
// already capped Occurrences at 32; the ordinary bodySink independently owns
// report-line trimming and its omitted-line disclosure.
func renderTargetSleepInventoryDetail(inventory tracequery.TargetWindowSleepInventory, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	emit(fmt.Sprintf("- %s: thread=%s window=%s scope=%s scan_status=%s output_status=%s total=%d emitted=%d total_ms=%s sleep_ms=%s d_state_ms=%s io_wait_ms=%s state_closure_status=%s binder_association_status=%s causal_attribution_status=%s",
		path, formatInlineStruct(reflect.ValueOf(inventory.Thread)), formatInlineStruct(reflect.ValueOf(inventory.Window)),
		clampToken(inventory.Scope), clampToken(inventory.ScanStatus), clampToken(inventory.OutputStatus), inventory.Total, inventory.Emitted,
		formatMsToken(inventory.TotalMs), formatMsToken(inventory.SleepMs), formatMsToken(inventory.DStateMs), formatMsToken(inventory.IOWaitMs),
		clampToken(inventory.StateClosureStatus), clampToken(inventory.BinderAssociationStatus), clampToken(inventory.CausalAttributionStatus)))
	if inventory.HeadState != nil {
		walkDetailWithPolicy(reflect.ValueOf(inventory.HeadState), path+".head_state", emit, depth+1, policy)
	}
	for i, occurrence := range inventory.Occurrences {
		// Reuse the existing scalar/tag formatter for EVERY original Interval
		// field, including its unmodified Summary. Direct scalar rendering is
		// intentional: Summary must not replace the physical coordinates here.
		tokens := structScalarTokens(reflect.ValueOf(occurrence.Interval))
		if !occurrence.BlockedReasonIOWaitKnown {
			tokens += " blocked_reason_io_wait_known=false"
		} else if occurrence.BlockedReasonIOWait == 0 {
			tokens += " blocked_reason_io_wait=0"
		}
		emit(fmt.Sprintf("- %s.occurrences[%d]: ordinal=%d %s", path, i, occurrence.Ordinal, tokens))
	}
}
