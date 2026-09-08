package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const traceBinderWaitInventoryPreviewCap = 4

func traceBinderWaitInventorySummary(i *tracequery.TargetWindowBinderWaitInventory) string {
	scan := "not complete"
	if i.ScanStatus == "complete" {
		scan = "complete"
	}
	// The compact shared handoff keeps 180 characters. Put the pre-cap
	// measurement and unresolved population before the extra audit details.
	return fmt.Sprintf("Verified closed Binder waits: %d, union=%.3fms; unresolved=%d, unassociated=%d. Index scan %s; not all waits or roots. Among %d constructed target sleep intervals; returned=%d/%d. Counts partition the retained target intervals, not all possible waits in the capture. Unassociated sleep is not evidence of non-Binder or voluntary sleep. The selected causal-chain candidates use a different selection and proof scope; do not substitute their lower bound for this verified inventory or add the two accounts. %s.",
		i.ConfirmedCount, i.ConfirmedMs, i.UnresolvedCandidateCount, i.RemainingUnassociatedCount, scan,
		i.TargetSleepCount, i.Emitted, i.ConfirmedCount, traceSleepInventoryHeadSummary(i.HeadState))
}

func traceBinderWaitOccurrenceSummary(r tracequery.TargetWindowBinderWaitOccurrence) string {
	return fmt.Sprintf("Verified Binder wait #%d for %s: %.6f..%.6f = %.3fms; not a root seat. Request=%d, reply=%d; evidence request=%d/%d, reply=%d/%d, wake=%d at %.6f; constructed interval=%.6f..%.6f, state lines=%d-%d.",
		r.Ordinal, traceThreadLabel(r.Peer), r.StartTs, r.EndTs, r.DurationMs, r.RequestTransactionID, r.ReplyTransactionID,
		r.RequestSendLine, r.RequestReceiveLine, r.ReplySendLine, r.ReplyReceiveLine, r.ClosureLine, r.ClosureTs,
		r.ActualStartTs, r.ActualEndTs, r.StartLine, r.EndLine)
}

func writeTraceBinderWaitInventoryPreview(b *strings.Builder, account *tracequery.TargetWindowStateAccount, payloadRef string) {
	if b == nil || account == nil || account.BinderWaitInventory == nil {
		return
	}
	i := account.BinderWaitInventory
	fmt.Fprintln(b, traceBinderWaitInventorySummary(i))
	n := len(i.Occurrences)
	if n > traceBinderWaitInventoryPreviewCap {
		n = traceBinderWaitInventoryPreviewCap
	}
	for _, r := range i.Occurrences[:n] {
		fmt.Fprintf(b, "- %s\n", traceBinderWaitOccurrenceSummary(r))
	}
	if i.ConfirmedCount > n {
		fmt.Fprintf(b, "- Verified Binder wait preview shows %d/%d; payload=%s retains %d/%d. The union above precedes both display caps.\n", n, i.ConfirmedCount, sanitizeForBanner(payloadRef), i.Emitted, i.ConfirmedCount)
	}
}

func traceQueryBinderWaitInventoryObservations(account *tracequery.TargetWindowStateAccount, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if account == nil || account.BinderWaitInventory == nil {
		return nil
	}
	i := account.BinderWaitInventory
	subject := traceThreadLabel(i.Thread)
	count := i.ConfirmedCount
	notes := traceQueryTypedKVNotes([][2]string{{types.TraceNoteKeySelectedWindow, fmt.Sprintf("%.6f..%.6f", i.Window.StartTs, i.Window.EndTs)}})
	set := types.ObservationRecord{
		ID:     fmt.Sprintf("trace_query:%s#target_binder_wait_inventory", scope),
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
		ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
		Span:     types.ObservationSpan{LineStart: account.LineStart, LineEnd: account.LineEnd, StartTs: i.Window.StartTs, EndTs: i.Window.EndTs},
		ClaimKey: "target_binder_wait_inventory:" + subject, Subject: subject,
		Predicate: "target_binder_wait_inventory", Object: "indexed_target_verified_closed_waits",
		// Zero confirmed is a measured inventory result, not a missing value.
		// The older duration helper intentionally omits zero and cannot serve
		// this account's zero/unresolved disclosure.
		Value: fmt.Sprintf("%.3f", i.ConfirmedMs), Unit: "ms", ResultCount: &count,
		Summary: traceBinderWaitInventorySummary(i), RichNotes: notes,
		SupportRefs: traceQueryObservationSupportRefs(ref, account.LineStart, account.LineEnd), ObservedAt: at, Confidence: 1,
	}
	out := []types.ObservationRecord{set}
	for _, r := range i.Occurrences {
		refs := traceQueryObservationSupportRefs(ref, r.StartLine, r.EndLine)
		for _, line := range []int{r.RequestSendLine, r.RequestReceiveLine, r.ReplySendLine, r.ReplyReceiveLine, r.ClosureLine} {
			refs = append(refs, traceQueryObservationSupportRefs(ref, line, line)...)
		}
		out = append(out, types.ObservationRecord{
			ID:     fmt.Sprintf("trace_query:%s#target_binder_wait_interval:%d", scope, r.Ordinal),
			Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard,
			ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref,
			Span:     types.ObservationSpan{LineStart: r.StartLine, LineEnd: r.ClosureLine, StartTs: r.StartTs, EndTs: r.EndTs},
			ClaimKey: fmt.Sprintf("target_binder_wait_interval:%s:%d", subject, r.Ordinal), Subject: subject,
			Predicate: "target_binder_wait_interval", Object: "verified_reply_wakeup",
			Value: traceQueryObservationMSValue(r.DurationMs), Unit: "ms", Summary: traceBinderWaitOccurrenceSummary(r), RichNotes: notes,
			SupportRefs: refs, ObservedAt: at, Confidence: 1,
		})
	}
	return out
}
