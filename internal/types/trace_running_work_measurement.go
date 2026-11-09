package types

import (
	"encoding/hex"
	"math"
	"strconv"
	"strings"
)

// TraceRunningWorkStatus qualifies only the intersection of running time and
// admitted, paired semantic spans. It is not a claim about uninstrumented
// work, an additional wall-clock lane, or a causal qualification.
type TraceRunningWorkStatus string

const (
	TraceRunningWorkMeasured    TraceRunningWorkStatus = "measured"
	TraceRunningWorkNotMeasured TraceRunningWorkStatus = "not_measured"
	TraceRunningWorkConflict    TraceRunningWorkStatus = "conflicting_measurements"
)

// TraceRunningWorkValue keeps legacy positive intersections readable. The old
// wire wrote zero even when no semantic scan ran; only an explicit measured
// status can establish zero. Unknown or contradictory status never yields a value.
func TraceRunningWorkValue(status TraceRunningWorkStatus, ms float64) (float64, bool) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || ms < 0 ||
		(status != TraceRunningWorkMeasured && !(status == "" && ms > 0)) {
		return 0, false
	}
	return ms, true
}

func traceRunningWorkFromRecord(record ObservationRecord) (TraceRunningWorkStatus, float64) {
	statusNote, uniqueStatus := traceProjectionUniqueTypedNote(record.RichNotes, TraceNoteKeyDeterministicRunningStatus)
	valueNote, uniqueValue := traceProjectionUniqueTypedNote(record.RichNotes, TraceNoteKeyDeterministicRunning)
	if !uniqueStatus || !uniqueValue {
		return TraceRunningWorkNotMeasured, 0
	}
	status := TraceRunningWorkStatus(statusNote)
	ms, err := strconv.ParseFloat(valueNote, 64)
	// The base state lanes are published to the nearest microsecond.
	// A native intersection cannot exceed its own running denominator
	// except for that rounding, and an impossible row cannot refine a peer.
	if status == TraceRunningWorkMeasured {
		running := traceCausalProjectionRichNoteFloat(record.RichNotes, TraceNoteKeyRunning)
		if running < 0 || ms > running+0.001 {
			return TraceRunningWorkNotMeasured, 0
		}
	}
	if value, ok := TraceRunningWorkValue(status, ms); err == nil && ok {
		return status, value
	}
	if status == "" {
		return "", 0
	}
	return TraceRunningWorkNotMeasured, 0
}

// Only a producer-minted scope and one complete native partition can refine
// another observation. Display labels, equal totals, and window tolerance
// alone cannot join observations across source or generation.
func traceRunningWorkScope(record ObservationRecord) string {
	if !traceCausalProjectionTraceQueryRecord(record) ||
		record.SourceRef.Kind != ObservationSourceRuntimeArtifact ||
		strings.TrimSpace(record.SourceRef.Path) == "" ||
		strings.TrimSpace(record.SourceRef.QueryScopeID) == "" ||
		!TraceRuntimeAccountRecordsSameResult(record, record) ||
		record.MeasurementSources == nil || record.MeasurementSources.HasUnknown ||
		len(record.MeasurementSources.Domains) != 1 {
		return ""
	}
	domain := record.MeasurementSources.Domains[0]
	scope, unique := traceProjectionUniqueTypedNote(record.RichNotes, TraceNoteKeyDeterministicRunningScopeID)
	digest, hasPrefix := strings.CutPrefix(scope, "running_work:v1:")
	_, digestErr := hex.DecodeString(digest)
	artifact := TraceCausalProjectionRecordArtifactIdentity(record)
	if !unique || !hasPrefix || len(digest) != 64 || digestErr != nil || artifact == "" || domain.Version != 1 || domain.Status != "constructed_partition" ||
		domain.Method != "thread_timeline" || domain.TargetTID <= 0 || domain.PartitionID == "" ||
		!record.SourceRef.QueryWindowKnown || record.SourceRef.QueryWindowStartTs != domain.WindowStartTs ||
		record.SourceRef.QueryWindowEndTs != domain.WindowEndTs ||
		!record.SourceRef.QueryLineRangeKnown || record.SourceRef.QueryLineStart != domain.QueryLineStart ||
		record.SourceRef.QueryLineEnd != domain.QueryLineEnd {
		return ""
	}
	return artifact + "\x00" + scope + "\x00" + traceSchedulerMeasurementDomainKey(domain)
}

func traceSameRunningWorkPartition(a, b traceCausalProjectionTargetStateCandidate) bool {
	if a.RunningWorkScope == "" || a.RunningWorkScope != b.RunningWorkScope ||
		a.WindowStart != b.WindowStart || a.WindowEnd != b.WindowEnd {
		return false
	}
	x, y := a.Account, b.Account
	// Keep the account and its reference whole. Matching the independent
	// published base fields protects against contradictory producer receipts.
	return x.Subject == y.Subject && x.TotalMS == y.TotalMS &&
		x.RunningMS == y.RunningMS && x.RunnableMS == y.RunnableMS && x.SleepMS == y.SleepMS &&
		x.DStateMS == y.DStateMS && x.IOWaitMS == y.IOWaitMS && x.SleepIOWaitMS == y.SleepIOWaitMS &&
		x.HeadCarryMS == y.HeadCarryMS && x.HeadCarryState == y.HeadCarryState &&
		x.TailOpenMS == y.TailOpenMS && x.TailOpenState == y.TailOpenState
}

func traceRefineRunningWork(base traceCausalProjectionTargetStateCandidate, candidates []traceCausalProjectionTargetStateCandidate) TraceCausalProjectionTargetStateAccount {
	best := base.Account
	for _, candidate := range candidates {
		if candidate.Account.DeterministicRunningStatus != TraceRunningWorkMeasured ||
			!traceSameRunningWorkPartition(base, candidate) {
			continue
		}
		if best.DeterministicRunningStatus != TraceRunningWorkMeasured {
			best = candidate.Account // keep the measured record and locator intact
		} else if candidate.Account.DeterministicRunningMS != best.DeterministicRunningMS {
			// Retain the original partition and its locator, withholding only
			// the disagreeing secondary value. Never sum or elect a maximum.
			out := base.Account
			out.DeterministicRunningStatus = TraceRunningWorkConflict
			out.DeterministicRunningMS = 0
			return out
		}
	}
	return best
}
