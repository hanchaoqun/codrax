package types

import (
	"fmt"
	"math"
	"strings"
)

// TraceSchedulerStateAccounting describes how a native cumulative state value
// was accumulated. End observations include partition boundaries (for example
// migration), not necessarily the end of the thread's state. Counts and time
// portions preserve open and unknown contributions; no endpoint is inferred
// from the enclosing query, a filled line number, or the cumulative envelope.
// This display metadata never grants causality, membership or additivity.
type TraceSchedulerStateAccounting struct {
	State                     string  `json:"state"`
	Caliber                   string  `json:"caliber"`
	SegmentCount              int     `json:"segment_count"`
	ObservedEndCount          int     `json:"observed_end_count"`
	ObservedEndMs             float64 `json:"observed_end_ms"`
	OpenTailCount             int     `json:"open_tail_count"`
	OpenTailMs                float64 `json:"open_tail_ms"`
	UnknownClosureCount       int     `json:"unknown_closure_count"`
	UnknownClosureMs          float64 `json:"unknown_closure_ms"`
	StartClippedCount         int     `json:"start_clipped_count"`
	EndClippedCount           int     `json:"end_clipped_count"`
	BoundaryContinuationCount int     `json:"boundary_continuation_count"`
}

func CloneTraceSchedulerStateAccounting(in *TraceSchedulerStateAccounting) *TraceSchedulerStateAccounting {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func CloneTraceSchedulerStateAccounts(in []TraceSchedulerStateAccounting) []TraceSchedulerStateAccounting {
	if in == nil {
		return nil
	}
	return append([]TraceSchedulerStateAccounting{}, in...)
}

func TraceSchedulerStateAccounts(in *TraceSchedulerStateAccounting) []TraceSchedulerStateAccounting {
	if in == nil {
		return nil
	}
	return []TraceSchedulerStateAccounting{*in}
}

// TraceSchedulerStateAccountingMeaning is a shared, short display projection.
// Malformed or legacy metadata stays unknown, never a zero closed population.
func TraceSchedulerStateAccountingMeaning(in *TraceSchedulerStateAccounting, zh bool) string {
	if !traceSchedulerStateAccountingKnown(in) {
		if zh {
			return "累计状态口径；闭合信息未知，不能认定单段持续或补齐实际终点"
		}
		return "cumulative state account; closure unknown, not a proven continuous interval or actual endpoint"
	}
	if zh {
		state := map[string]string{"running": "运行", "runnable": "就绪", "sleep": "睡眠", "s_sleep": "睡眠", "d_state": "不可中断等待", "d_sleep": "不可中断等待", "io_wait": "IO等待"}[in.State]
		if state == "" {
			state = in.State
		}
		return fmt.Sprintf("%s累计%d段（以下时长均为范围内计入量）：开放尾%d段/%.6gms，闭合未知%d段/%.6gms，已观测分段终点%d段/%.6gms；起/已知末端裁剪%d/%d段，边界续接%d段；包络非单段起止，分段终点不等于状态终止",
			state, in.SegmentCount, in.OpenTailCount, in.OpenTailMs, in.UnknownClosureCount, in.UnknownClosureMs, in.ObservedEndCount, in.ObservedEndMs,
			in.StartClippedCount, in.EndClippedCount, in.BoundaryContinuationCount)
	}
	return fmt.Sprintf("%s cumulative_segments=%d; open_tail=%d/%.6gms; closure_unknown=%d/%.6gms; observed_boundary=%d/%.6gms; start/known_end_clipped=%d/%d; boundary_continuations=%d; times are accounted contributions, envelope is not a continuous interval and boundary need not terminate the state",
		in.State, in.SegmentCount, in.OpenTailCount, in.OpenTailMs, in.UnknownClosureCount, in.UnknownClosureMs,
		in.ObservedEndCount, in.ObservedEndMs, in.StartClippedCount, in.EndClippedCount, in.BoundaryContinuationCount)
}

// Shard rollups do not combine endpoint counts into a physical population.
const TraceSchedulerStateShardAccountingMeaning = "cumulative accounts across query shards, not a continuous interval; physical closure is not established by this rollup: retain each source record's open/unknown contributions"

func traceSchedulerStateAccountingKnown(in *TraceSchedulerStateAccounting) bool {
	if in == nil || in.State == "" || in.Caliber != "cumulative_segments" || in.SegmentCount < 0 ||
		in.ObservedEndCount < 0 || in.OpenTailCount < 0 || in.UnknownClosureCount < 0 ||
		in.StartClippedCount < 0 || in.StartClippedCount > in.SegmentCount ||
		in.EndClippedCount < 0 || in.EndClippedCount > in.SegmentCount ||
		in.BoundaryContinuationCount < 0 || in.BoundaryContinuationCount > in.ObservedEndCount ||
		in.ObservedEndCount+in.OpenTailCount+in.UnknownClosureCount != in.SegmentCount {
		return false
	}
	for _, value := range []float64{in.ObservedEndMs, in.OpenTailMs, in.UnknownClosureMs} {
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	if (in.ObservedEndCount == 0 && in.ObservedEndMs != 0) || (in.OpenTailCount == 0 && in.OpenTailMs != 0) || (in.UnknownClosureCount == 0 && in.UnknownClosureMs != 0) {
		return false
	}
	return true
}

// TraceObservationHasStateAccounting selects exact cumulative producer
// families. Missing old metadata remains displayable with unknown closure.
func TraceObservationHasStateAccounting(record ObservationRecord) bool {
	if !RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
		return false
	}
	switch record.Predicate {
	case "state_drilldown", "state_churn", "running_time", "runnable_wait", "sleep_wait", "d_state_or_io_wait", "io_wait":
		return true
	}
	return false
}

func TraceObservationStateAccountingMeaning(record ObservationRecord, zh bool) string {
	if !TraceObservationHasStateAccounting(record) {
		return ""
	}
	return TraceSchedulerStateAccountsMeaning(record.StateAccounting, zh)
}

// TraceObservationStateAccountingCompact fits the existing 120-character
// history checkpoint. Counts describe state accounts, never additive state
// durations (IO wait may overlap another state). Inspect every account before
// projecting, so a five-state churn row cannot silently become its first state.
func TraceObservationStateAccountingCompact(record ObservationRecord) string {
	if !TraceObservationHasStateAccounting(record) {
		return ""
	}
	const prefix = "Cumulative contributions, not continuous/actual endpoints; "
	if len(record.StateAccounting) == 0 {
		return prefix + "closure unknown (no native metadata)"
	}
	open, unknown := 0, 0
	for i := range record.StateAccounting {
		a := &record.StateAccounting[i]
		if !traceSchedulerStateAccountingKnown(a) {
			unknown++
			continue
		}
		if a.OpenTailCount > 0 {
			open++
		}
		if a.UnknownClosureCount > 0 {
			unknown++
		}
	}
	return fmt.Sprintf("%sstates=%d; open-tail states=%d; unknown-closure states=%d", prefix, len(record.StateAccounting), open, unknown)
}

func TraceSchedulerStateAccountsMeaning(accounts []TraceSchedulerStateAccounting, zh bool) string {
	if len(accounts) == 0 {
		return TraceSchedulerStateAccountingMeaning(nil, zh)
	}
	parts := make([]string, 0, len(accounts))
	for i := range accounts {
		parts = append(parts, TraceSchedulerStateAccountingMeaning(&accounts[i], zh))
	}
	return strings.Join(parts, " | ")
}
