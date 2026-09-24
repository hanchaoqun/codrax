package types

import (
	"fmt"
	"strings"
)

// TraceSchedulerStateAccountingSummary is the transport-sized counterpart to
// Meaning. It retains every nonzero closure class, but does not repeat the
// complete per-state ruler on every Top-N/drilldown row. Precise contributed
// times, clipping and continuation counts remain in the native JSON and ledger.
func TraceSchedulerStateAccountingSummary(account *TraceSchedulerStateAccounting) string {
	return "account=cumulative(" + traceSchedulerStateAccountingCounts(account) + ")"
}

// Churn owns several state accounts. Keep them separate rather than collapsing
// them to the dominant state or adding overlapping accounting categories.
func TraceSchedulerStateAccountsSummary(accounts []TraceSchedulerStateAccounting) string {
	if len(accounts) == 0 {
		return TraceSchedulerStateAccountingSummary(nil)
	}
	parts := make([]string, 0, len(accounts))
	for i := range accounts {
		parts = append(parts, accounts[i].State+":"+traceSchedulerStateAccountingCounts(&accounts[i]))
	}
	return "account=cumulative(" + strings.Join(parts, ";") + ")"
}

func traceSchedulerStateAccountingCounts(account *TraceSchedulerStateAccounting) string {
	if !traceSchedulerStateAccountingKnown(account) {
		return "closure=unknown"
	}
	parts := make([]string, 0, 3)
	for _, count := range []struct {
		label string
		value int
		ms    float64
	}{{"boundary", account.ObservedEndCount, account.ObservedEndMs}, {"open_tail", account.OpenTailCount, account.OpenTailMs}, {"unknown", account.UnknownClosureCount, account.UnknownClosureMs}} {
		if count.value > 0 {
			part := fmt.Sprintf("%s=%d", count.label, count.value)
			if count.label != "boundary" {
				part += fmt.Sprintf("/%.6gms", count.ms)
			}
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return "segments=0"
	}
	return strings.Join(parts, ",")
}

// Render once per result, not once per state/row. A boundary can be migration,
// and an open contribution can reach a query boundary without an observed end.
const TraceSchedulerStateAccountingSummaryGuidance = "state accounts show segment counts and accounted ms, not continuous intervals or actual endpoints; boundary is an observed segment boundary, not necessarily state termination. Complete times, clipping and continuations remain in payload_ref."
