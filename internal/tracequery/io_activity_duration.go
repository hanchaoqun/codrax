package tracequery

import (
	"fmt"
	"math"
	"strconv"
)

// IOActivityDurationBoundary qualifies the operands of the composite index,
// not independent native measurements (which can have a supported zero).
// Scheduler operands sum the admitted state groups, not device or target
// wall-clock time. The absence of an index operand never proves absence of
// the underlying phenomenon, even when other operands have positive values.
const IOActivityDurationBoundary = "Only admitted values contribute to this background index. An unaccounted duration is not a measured zero. These inputs do not establish device queue state, target blocking time or a cause of delay."

// IOActivityDurationContribution returns only an accounted positive input.
// In this composite a zero Go field is the additive identity, not a separate
// measurement. Keep a positive sub-microsecond input from rounding to zero.
func IOActivityDurationContribution(ms float64) string {
	if !(ms > 0) || math.IsInf(ms, 0) {
		return ""
	}
	if ms < 0.0005 {
		return strconv.FormatFloat(ms, 'g', -1, 64)
	}
	return fmt.Sprintf("%.3f", ms)
}

// IOActivityDurationText presents one operand with its unit, or its absence.
func IOActivityDurationText(ms float64, absent string) string {
	if value := IOActivityDurationContribution(ms); value != "" {
		return value + "ms"
	}
	return absent
}

// IOActivityIndexSummaryParts separates the account and its optional comparison
// method from the required interpretation boundary. Consumers with a text
// budget can bound the variable parts independently and keep the boundary on
// the same admitted row, without parsing or duplicating the producer summary.
type IOActivityIndexSummaryParts struct {
	Account    string
	Comparison string
}

func (parts IOActivityIndexSummaryParts) String() string {
	account := parts.Account
	if parts.Comparison != "" {
		account += " " + parts.Comparison
	}
	return account + "; " + IOActivityDurationBoundary
}

func (item IOPressureSummary) ActivityIndexSummaryParts() IOActivityIndexSummaryParts {
	const absent = "not_accounted"
	parts := IOActivityIndexSummaryParts{
		Account: fmt.Sprintf("io activity signal=%s activity_index=%.3f evidence_quality=%s score_caliber=%s block_max=%s storage_max=%s file_bytes=%d file_events=%d page_cache_churn=%d iowait_blocked=%d d_state=%s io_wait=%s top_inode=%s",
			item.Signal, item.Score, item.EvidenceQuality, item.ScoreCaliber,
			IOActivityDurationText(item.BlockMaxLatencyMs, absent), IOActivityDurationText(item.StorageMaxLatencyMs, absent),
			item.FileIOBytes, item.FileIOEvents, item.PageCacheChurn, item.IOWaitBlockedCount,
			IOActivityDurationText(item.DStateMs, absent), IOActivityDurationText(item.IOWaitMs, absent), firstNonEmpty(item.TopInode, "unknown")),
	}
	if item.Signal == "blocked_reason_iowait_count_only" {
		parts.Comparison = fmt.Sprintf("score_breakdown=iowait_blocked_count(%d)*5=%.3f comparison_scope=same_score_caliber_capture_conditions_and_window_duration absolute_level=not_defined", item.IOWaitBlockedCount, item.Score)
	}
	return parts
}
