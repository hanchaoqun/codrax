package tracediag

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// This independently enumerated, confirmed SUBSET stays in the account's
// original detail position. It does not take a key-first/root-rank seat. The
// engine owns matching and its 32-row return cap; bodySink separately owns
// report-line trimming. Incomplete scans, unresolved candidates and measured
// zero confirmed waits must not become an absent field or a capture verdict.
func renderTargetBinderWaitInventoryDetail(inventory tracequery.TargetWindowBinderWaitInventory, path string, emit func(string), depth int, policy *detailRenderPolicy) {
	emit(fmt.Sprintf("- %s: thread=%s window=%s scope=%s scan_status=%s output_status=%s target_sleep_count=%d confirmed_count=%d confirmed_ms=%s unresolved_candidate_count=%d remaining_unassociated_count=%d emitted=%d causal_attribution_status=%s",
		path, formatInlineStruct(reflect.ValueOf(inventory.Thread)), formatInlineStruct(reflect.ValueOf(inventory.Window)),
		clampToken(inventory.Scope), clampToken(inventory.ScanStatus), clampToken(inventory.OutputStatus),
		inventory.TargetSleepCount, inventory.ConfirmedCount, formatMsToken(inventory.ConfirmedMs),
		inventory.UnresolvedCandidateCount, inventory.RemainingUnassociatedCount, inventory.Emitted, clampToken(inventory.CausalAttributionStatus)))
	if inventory.HeadState != nil {
		walkDetailWithPolicy(reflect.ValueOf(inventory.HeadState), path+".head_state", emit, depth+1, policy)
	}
	walkDetailWithPolicy(reflect.ValueOf(inventory.UnresolvedReasons), path+".unresolved_reasons", emit, depth+1, policy)
	for i, occurrence := range inventory.Occurrences {
		// Keep request/reply/physical closure on the SAME row as the original
		// state interval. Summary does not replace its independent coordinates.
		tokens := binderInventoryScalarTokens(reflect.ValueOf(occurrence)) + " " + binderInventoryScalarTokens(reflect.ValueOf(occurrence.Interval))
		if !occurrence.BlockedReasonIOWaitKnown {
			tokens += " blocked_reason_io_wait_known=false"
		} else if occurrence.BlockedReasonIOWait == 0 {
			tokens += " blocked_reason_io_wait=0"
		}
		emit(fmt.Sprintf("- %s.occurrences[%d]: %s", path, i, tokens))
	}
}

// Preserve explicitly serialized scalar zero values, including a legitimate
// timestamp origin. Optional absent/unknown fields keep their original omission
// semantics; existing tag formatters own milliseconds and fixed-point seconds.
func binderInventoryScalarTokens(v reflect.Value) string {
	tokens := structScalarTokens(v)
	for i := 0; i < v.NumField(); i++ {
		field, value := v.Type().Field(i), v.Field(i)
		if field.PkgPath != "" || jsonExcluded(field) || !isScalarKind(value.Kind()) ||
			!isZeroValue(value) || strings.Contains(field.Tag.Get("json"), ",omitempty") {
			continue
		}
		tag := jsonTagName(field)
		tokens += " " + tag + "=" + formatScalarForTag(value, tag)
	}
	return strings.TrimSpace(tokens)
}
