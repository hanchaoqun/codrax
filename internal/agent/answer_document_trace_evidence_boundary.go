package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Evidence gaps constrain a particular observation, not every relation of its
// subject. This is display-only: it does not change query/rank/gate authority.
func traceDecisionEvidenceBoundaryKind(node types.TraceCausalProjectionNode) string {
	for _, token := range []string{node.TypeToken, node.Object, node.Predicate} {
		switch strings.TrimSpace(token) {
		case "trace_gap", "missing_wakeup":
			return strings.TrimSpace(token)
		}
	}
	if strings.TrimSpace(node.UndrillableReason) == "missing_wakeup" {
		return "missing_wakeup"
	}
	return "unspecified_evidence_gap"
}

func traceDecisionWriteEvidenceBoundary(b *strings.Builder, node types.TraceCausalProjectionNode) {
	kind := traceDecisionEvidenceBoundaryKind(node)
	status, domain := "unspecified_evidence_gap", "unspecified"
	if kind == "missing_wakeup" {
		status, domain = "no_matching_sched_wakeup_row_in_selected_window", "sleep_interval_wakeup_match"
	} else if kind == "trace_gap" {
		switch strings.TrimSpace(node.TraceGapKind) {
		case tracequery.TraceGapKindNoSchedData:
			status, domain = "no_scheduler_intervals_in_aligned_scope", "scheduler_timeline_intervals"
		case tracequery.TraceGapKindNoEligibleWait:
			status, domain = "no_interval_meets_min_duration_in_aligned_scope", "eligible_scheduler_interval_candidates"
		}
	}
	fmt.Fprintf(b, "- evidence_boundary: subject=`%s`; kind=`%s`; status=`%s`; evidence_domain=`%s`; positive_blocker_authority=`not_provided`",
		strings.TrimSpace(node.Subject), kind, status, domain)
	if kind == "trace_gap" {
		gapKind := strings.TrimSpace(node.TraceGapKind)
		if gapKind == "" {
			gapKind = "unspecified"
		}
		// RootEvidence folds zero-value gaps across recursive visits. Neither
		// the enclosing query nor nearest_chain_window identifies every checked
		// interval; do not manufacture an absence interval from those locators.
		fmt.Fprintf(b, "; gap_kind=`%s`; aligned_scope_bounds=`not_published`", gapKind)
	}
	if kind == "missing_wakeup" {
		b.WriteString("; causal_identity=`unresolved`; causal_identity_scope=`matching_waker_for_this_sleep_interval`")
		if types.TraceCausalProjectionWindowPresent(node.StartTs, node.EndTs) {
			fmt.Fprintf(b, "; observed_sleep_interval=`%.6f..%.6f`; interval_ms=%.3f", node.StartTs, node.EndTs, (node.EndTs-node.StartTs)*1000)
		} else if node.ImpactMS > 0 {
			fmt.Fprintf(b, "; observed_sleep_interval=`unavailable`; interval_ms=%.3f", node.ImpactMS)
		}
	}
	if start, end, ok := traceDecisionNodeQueryWindow(node); ok {
		fmt.Fprintf(b, "; source_query_window=`%.6f..%.6f`", start, end)
	} else {
		b.WriteString("; source_query_window=`not_published`")
	}
	fmt.Fprintf(b, "; evidence_id=`%s`; source_lane=`%s`\n", strings.TrimSpace(node.EvidenceID), traceDecisionNodeSourceLane(node))
}

func traceDecisionEvidenceBoundaryKey(node types.TraceCausalProjectionNode) string {
	// Only a complete producer receipt can collapse two display copies. Keep
	// unknown origins separate by evidence ID; never join by subject text.
	refs := make([]types.ObservationSourceRef, 0, len(node.MeasurementOrigins))
	known := len(node.MeasurementOrigins) > 0
	for _, origin := range node.MeasurementOrigins {
		ref := origin.SourceRef
		known = known && ref.Path != "" && ref.QueryScopeID != "" && ref.PayloadRef != ""
		refs = append(refs, ref)
	}
	data, err := json.Marshal(refs)
	if err != nil {
		return ""
	}
	identity := string(data)
	if !known {
		if strings.TrimSpace(node.EvidenceID) == "" {
			return ""
		}
		identity += "\x00" + node.EvidenceID
	}
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%.9f\x00%.9f\x00%.6f\x00%d\x00%d\x00%.9f\x00%.9f\x00%.9f\x00%.9f",
		identity, strings.TrimSpace(node.Subject), traceDecisionEvidenceBoundaryKind(node), strings.TrimSpace(node.TraceGapKind),
		node.StartTs, node.EndTs, node.ImpactMS, node.LineStart, node.LineEnd,
		node.QueryWindowStartTs, node.QueryWindowEndTs, node.RankQueryWindowStartTs, node.RankQueryWindowEndTs)
}
