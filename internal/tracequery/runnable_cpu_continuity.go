package tracequery

const (
	RunnableCPUContinuityVerified           = "verified"
	RunnableCPUContinuitySchedInMismatch    = "sched_in_cpu_mismatch"
	RunnableCPUContinuityMigrationMismatch  = "migration_origin_mismatch"
	RunnableCPUContinuityStartCPUUnknown    = "start_cpu_unknown"
	RunnableCPUContinuityOpenEnded          = "window_end_unverified"
	RunnableCPUContinuityGenerationReset    = "generation_reset_unverified"
	RunnableCPUContinuityWakeTargetConflict = "wakeup_target_conflict"
	runnableCPUContinuityWitnessCap         = 8
	runnableCPUContinuityBoundarySchedIn    = "sched_in"
	runnableCPUContinuityBoundaryMigration  = "migration"
	runnableCPUContinuityBoundaryWindowEnd  = "window_end"
	runnableCPUContinuityBoundaryGeneration = "generation_reset"
	runnableCPUProvenanceWakeTarget         = "wakeup_target"
	runnableCPUProvenanceWakeTargetConflict = "wakeup_target_conflict"
	runnableCPUProvenanceMigration          = "migration"
	runnableCPUProvenanceSchedSwitch        = "sched_switch"
	runnableCPUProvenanceCrossSourceUnknown = "cross_source_head_unproven"
)

// RunnableCPUContinuitySummary is the typed accounting boundary between a
// scheduler-proven runnable wall-clock interval and the narrower per-CPU
// attribution lane. A segment with unknown CPU remains in RunnableTop and the
// root-cause runnable account, but cannot feed pressure, frequency, CAP,
// same-CPU competition, or CPU-specific inversion evidence.
type RunnableCPUContinuitySummary struct {
	TotalSegments              int                            `json:"total_segments,omitempty"`
	SchedInSegments            int                            `json:"sched_in_segments,omitempty"`
	VerifiedSegments           int                            `json:"verified_segments,omitempty"`
	UnknownSegments            int                            `json:"unknown_segments,omitempty"`
	MismatchSegments           int                            `json:"mismatch_segments,omitempty"`
	OpenEndedSegments          int                            `json:"open_ended_segments,omitempty"`
	GenerationResetSegments    int                            `json:"generation_reset_segments,omitempty"`
	WakeTargetConflictSegments int                            `json:"wakeup_target_conflict_segments,omitempty"`
	ExactMigrationSegments     int                            `json:"exact_migration_segments,omitempty"`
	CheckedBoundarySegments    int                            `json:"checked_boundary_segments,omitempty"`
	VerifiedMs                 float64                        `json:"verified_ms,omitempty"`
	UnknownMs                  float64                        `json:"unknown_ms,omitempty"`
	MismatchMs                 float64                        `json:"mismatch_ms,omitempty"`
	MismatchRatio              float64                        `json:"mismatch_ratio,omitempty"`
	Witnesses                  []RunnableCPUContinuityWitness `json:"witnesses,omitempty"`
	WitnessOverflow            int                            `json:"witness_overflow,omitempty"`
}

// RunnableCPUContinuityWitness is a bounded, deterministic example of a
// segment whose thread-level duration survived while its CPU claim was
// withdrawn. Thread numeric identity and scheduler endpoints are retained;
// display names remain non-authoritative.
type RunnableCPUContinuityWitness struct {
	Thread      ThreadRef `json:"thread"`
	StartTs     float64   `json:"start_ts,omitempty"`
	EndTs       float64   `json:"end_ts,omitempty"`
	DurationMs  float64   `json:"duration_ms,omitempty"`
	ExpectedCPU int       `json:"expected_cpu"`
	ObservedCPU int       `json:"observed_cpu"`
	StartLine   int       `json:"start_line,omitempty"`
	EndLine     int       `json:"end_line,omitempty"`
	Reason      string    `json:"reason"`
}

type runnableCPUContinuityVerdict struct {
	cpu         int
	known       bool
	reason      string
	expectedCPU int
	observedCPU int
	boundary    string
	mismatch    bool
	checked     bool
}

// reconcileRunnableWakeTargetCPU is the single authority for duplicate wake
// rows that belong to one already-runnable attempt. sched_waking/sched_wakeup
// pairs are common and idempotent when their typed target_cpu agrees. A later
// precise target may fill an absent first value, but two different precise
// targets are contradictory: retain the thread-level runnable interval while
// withdrawing every CPU-specific claim. Proven migration/sched-switch state
// is outside this wake-target lane and is therefore left untouched.
func reconcileRunnableWakeTargetCPU(
	cpu int,
	known bool,
	provenance string,
	conflictExpected int,
	conflictObserved int,
	candidateCPU int,
	candidateKnown bool,
) (int, bool, string, int, int) {
	if provenance == runnableCPUProvenanceWakeTargetConflict {
		return cpu, known, provenance, conflictExpected, conflictObserved
	}
	if provenance != runnableCPUProvenanceWakeTarget {
		return cpu, known, provenance, conflictExpected, conflictObserved
	}
	if !candidateKnown || !validTraceCPUIndex(candidateCPU) {
		return cpu, known, provenance, conflictExpected, conflictObserved
	}
	if !known || !validTraceCPUIndex(cpu) {
		return candidateCPU, true, provenance, 0, 0
	}
	if cpu == candidateCPU {
		return cpu, true, provenance, conflictExpected, conflictObserved
	}
	return -1, false, runnableCPUProvenanceWakeTargetConflict, cpu, candidateCPU
}

// runnableCPUContinuityVerdictForSegment is the single hard policy shared by
// WindowStats and scheduler-latency construction. The start CPU is only a
// candidate until an exact migration origin or the first sched-in CPU closes
// the segment. Missing terminal evidence never promotes target_cpu into a
// continuous per-CPU claim.
func runnableCPUContinuityVerdictForSegment(expectedCPU int, expectedKnown bool, observedCPU int, observedKnown bool, boundary string) runnableCPUContinuityVerdict {
	if !expectedKnown || !validTraceCPUIndex(expectedCPU) {
		expectedCPU = -1
		expectedKnown = false
	}
	if !observedKnown || !validTraceCPUIndex(observedCPU) {
		observedCPU = -1
		observedKnown = false
	}
	verdict := runnableCPUContinuityVerdict{
		cpu:         -1,
		reason:      RunnableCPUContinuityStartCPUUnknown,
		expectedCPU: expectedCPU,
		observedCPU: observedCPU,
		boundary:    boundary,
	}
	verdict.checked = expectedKnown && observedKnown &&
		(boundary == runnableCPUContinuityBoundarySchedIn || boundary == runnableCPUContinuityBoundaryMigration)
	if expectedKnown && observedKnown && expectedCPU == observedCPU {
		verdict.cpu = expectedCPU
		verdict.known = true
		verdict.reason = RunnableCPUContinuityVerified
		return verdict
	}
	if expectedKnown && observedKnown {
		verdict.mismatch = true
		switch boundary {
		case runnableCPUContinuityBoundaryMigration:
			verdict.reason = RunnableCPUContinuityMigrationMismatch
		default:
			verdict.reason = RunnableCPUContinuitySchedInMismatch
		}
		return verdict
	}
	switch boundary {
	case runnableCPUContinuityBoundaryWindowEnd:
		verdict.reason = RunnableCPUContinuityOpenEnded
	case runnableCPUContinuityBoundaryGeneration:
		verdict.reason = RunnableCPUContinuityGenerationReset
	}
	return verdict
}

type runnableWaitSegment struct {
	thread        ThreadRef
	startTs       float64
	endTs         float64
	durationMs    float64
	startLine     int
	endLine       int
	cpu           int
	cpuKnown      bool
	cpuContinuity string
	priority      int
	priorityClass string
}

func (summary *RunnableCPUContinuitySummary) observe(segment runnableWaitSegment, verdict runnableCPUContinuityVerdict) {
	if summary == nil || segment.durationMs <= 0 {
		return
	}
	summary.TotalSegments++
	switch verdict.boundary {
	case runnableCPUContinuityBoundarySchedIn:
		summary.SchedInSegments++
	case runnableCPUContinuityBoundaryMigration:
		summary.ExactMigrationSegments++
	case runnableCPUContinuityBoundaryWindowEnd:
		summary.OpenEndedSegments++
	case runnableCPUContinuityBoundaryGeneration:
		summary.GenerationResetSegments++
	}
	if verdict.reason == RunnableCPUContinuityWakeTargetConflict {
		summary.WakeTargetConflictSegments++
	}
	if verdict.checked {
		summary.CheckedBoundarySegments++
	}
	if verdict.known {
		summary.VerifiedSegments++
		summary.VerifiedMs += segment.durationMs
	} else {
		summary.UnknownSegments++
		summary.UnknownMs += segment.durationMs
		if verdict.mismatch {
			summary.MismatchSegments++
			summary.MismatchMs += segment.durationMs
		}
		witness := RunnableCPUContinuityWitness{
			Thread:      segment.thread,
			StartTs:     segment.startTs,
			EndTs:       segment.endTs,
			DurationMs:  segment.durationMs,
			ExpectedCPU: verdict.expectedCPU,
			ObservedCPU: verdict.observedCPU,
			StartLine:   segment.startLine,
			EndLine:     segment.endLine,
			Reason:      verdict.reason,
		}
		if len(summary.Witnesses) < runnableCPUContinuityWitnessCap {
			summary.Witnesses = append(summary.Witnesses, witness)
		} else {
			summary.WitnessOverflow++
		}
	}
	if summary.CheckedBoundarySegments > 0 {
		summary.MismatchRatio = float64(summary.MismatchSegments) / float64(summary.CheckedBoundarySegments)
	}
}
