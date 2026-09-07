package tool

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceSleepInventoryPreviewCap = 4

// This transport is a state inventory, not another Binder classifier. In
// particular it must not use the older D/IO occurrence predicates: those have
// their own deliberately narrower counting and answer-contract consumers.
func traceSleepInventorySummary(inventory *tracequery.TargetWindowSleepInventory) string {
	// Put the amount, this query's head status and the authority boundary first:
	// shared finalizer/reviewer projections retain only 180 summary characters.
	return fmt.Sprintf("Target sleep intervals: %d, union=%.3fms; %s. State inventory does not prove cause or completion. Scope: constructed timeline only; S=%.3fms, D=%.3fms, scheduler IO=%.3fms; returned=%d/%d. Full scanning of this timeline does not certify full artifact coverage, Binder association, IO mechanism, or physical completion at the recorded interval ends.%s",
		inventory.Total, inventory.TotalMs, traceSleepInventoryHeadSummary(inventory.HeadState), inventory.SleepMs, inventory.DStateMs, inventory.IOWaitMs, inventory.Emitted, inventory.Total, traceSleepInventoryHeadDetail(inventory.HeadState))
}

func traceSleepInventoryHeadSummary(head *tracequery.TimelineHeadState) string {
	if head == nil {
		return "window-head state not assessed"
	}
	status := "unknown"
	switch head.Status {
	case "recovered":
		status = "recovered from prior history"
	case "observed_in_index":
		status = "observed"
	}
	return fmt.Sprintf("window-head state %s at %.6fs", status, head.BoundaryTs)
}

func traceSleepInventoryHeadDetail(head *tracequery.TimelineHeadState) string {
	if head == nil || head.Status != "recovered" {
		return ""
	}
	return fmt.Sprintf(" Window-head recovery: state=%s, recorded start=%.6fs, evidence line=%d.", head.State, head.ActualStartTs, head.SourceLine)
}

func traceSleepOccurrenceSummary(row tracequery.TargetWindowSleepOccurrence) string {
	state := "sleep state"
	switch row.State {
	case tracequery.StateSSleep:
		state = "interruptible sleep (S)"
	case tracequery.StateDSleep:
		state = "uninterruptible sleep (D)"
	case tracequery.StateIOWait:
		state = "scheduler-marked IO wait"
	}
	marker := "not recorded"
	if row.BlockedReasonIOWaitKnown {
		marker = strconv.FormatInt(int64(row.BlockedReasonIOWait), 10)
	}
	text := fmt.Sprintf("State only, not proof of cause or completion; #%d %s %.6f..%.6f = %.3fms; kernel IO marker=%s; evidence lines=%d-%d",
		row.Ordinal, state, row.StartTs, row.EndTs, row.DurationMs, marker, row.StartLine, row.EndLine)
	if row.WindowClamped() {
		text += fmt.Sprintf("; window-clipped from constructed interval %.6f..%.6f", row.ActualStartTs, row.ActualEndTs)
	}
	return text
}

func writeTraceSleepInventoryPreview(b *strings.Builder, account *tracequery.TargetWindowStateAccount, payloadRef string) {
	if b == nil || account == nil || account.SleepInventory == nil {
		return
	}
	inventory := account.SleepInventory
	fmt.Fprintln(b, traceSleepInventorySummary(inventory))
	visible := len(inventory.Occurrences)
	if visible > traceSleepInventoryPreviewCap {
		visible = traceSleepInventoryPreviewCap
	}
	for _, row := range inventory.Occurrences[:visible] {
		fmt.Fprintf(b, "- %s\n", traceSleepOccurrenceSummary(row))
	}
	if inventory.Total > visible {
		fmt.Fprintf(b, "- Sleep interval preview shows %d/%d; payload=%s retains %d/%d. Totals above precede both display caps.\n",
			visible, inventory.Total, sanitizeForBanner(payloadRef), inventory.Emitted, inventory.Total)
	}
}

func traceQuerySleepInventoryObservations(account *tracequery.TargetWindowStateAccount, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if account == nil || account.SleepInventory == nil {
		return nil
	}
	inventory := account.SleepInventory
	subject := traceThreadLabel(inventory.Thread)
	total := inventory.Total
	set := types.ObservationRecord{
		ID:     fmt.Sprintf("trace_query:%s#target_sleep_inventory", scope),
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		Span:     types.ObservationSpan{LineStart: account.LineStart, LineEnd: account.LineEnd, StartTs: inventory.Window.StartTs, EndTs: inventory.Window.EndTs},
		ClaimKey: "target_sleep_inventory:" + subject, Subject: subject,
		Predicate: "target_sleep_inventory", Object: "constructed_scheduler_intervals",
		Value: strconv.Itoa(inventory.Total), Unit: "occurrences", ResultCount: &total,
		Summary:     traceSleepInventorySummary(inventory),
		SupportRefs: traceQueryObservationSupportRefs(ref, account.LineStart, account.LineEnd), ObservedAt: at, Confidence: 0.95,
	}
	out := []types.ObservationRecord{set}
	for _, row := range inventory.Occurrences {
		out = append(out, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:%s#target_sleep_interval:%d", scope, row.Ordinal),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
			Span:     types.ObservationSpan{LineStart: row.StartLine, LineEnd: row.EndLine, StartTs: row.StartTs, EndTs: row.EndTs},
			ClaimKey: fmt.Sprintf("target_sleep_interval:%s:%d", subject, row.Ordinal), Subject: subject,
			Predicate: "target_sleep_interval", Object: string(row.State),
			Value: traceQueryObservationMSValue(row.DurationMs), Unit: "ms", Summary: traceSleepOccurrenceSummary(row),
			SupportRefs: traceQueryObservationSupportRefs(ref, row.StartLine, row.EndLine), ObservedAt: at, Confidence: 0.95,
		})
	}
	return out
}
