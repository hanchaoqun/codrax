package tracequery

import "testing"

// NEXTINFO-AUTH (2026-07-26): next_info is emitted by the HarmonyOS kernel.
// Its affinity field is an authoritative cpuset snapshot. The group-name
// suffix (cg=) remains a separate provenance lane and cannot downgrade or
// replace that authority.
func TestNextInfoAuthorityUsesTraceGlobalCPUUniverse(t *testing.T) {
	idx := buildTraceIndex(t, "next_info_authority.systrace", `
       idle/4-0   (    0) [004] .... 0.900000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=other next_pid=400 next_prio=120
       idle/0-0   (    0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,0,0 cg=top-app
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{
		PID:             100,
		TimeStart:       1.000,
		TimeEnd:         1.010,
		TraceFlavorHint: TraceFlavorHarmonyHitrace,
		MinDurationMs:   0.05,
		Limit:           8,
	}
	stats := ComputeWindowStats(idx, q)
	var got *CPUConstraintSummary
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			got = &stats.CPUConstraints[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected next_info constraint summary: %+v", stats.CPUConstraints)
	}
	if got.AllowedCPUsAuthority != CPUConstraintAllowedCPUsAuthorityKernelNextInfo {
		t.Fatalf("next_info affinity must retain kernel cpuset authority: %+v", got)
	}
	if got.RestrictionProof != CPUConstraintRestrictionProofAllowedMaskExcludesUniverse {
		t.Fatalf("mask {0,1} must exclude trace-global cpu4 even though cpu4 is outside the query window: %+v", got)
	}
	if len(got.ExcludedCPUs) != 1 || got.ExcludedCPUs[0] != 4 {
		t.Fatalf("restriction proof must carry its exact trace-global exclusion set: %+v", got)
	}
	if got.CPUSet != "top-app" || got.CPUSetIsBinding {
		t.Fatalf("cg= remains a non-binding group-name proxy beside the authoritative mask: %+v", got)
	}
	if !cpuConstraintRestrictsExecution(*got) {
		t.Fatalf("the shared typed restriction proof must be consumable by the root gate: %+v", got)
	}
	if confidence := cpuConstraintRankConfidence(*got); confidence != 0.72 {
		t.Fatalf("an authoritative kernel mask with an exclusion proof deserves the precise-evidence confidence arm, got %.2f", confidence)
	}
}

func TestRunnableContextVerdictReadsSharedRestrictionProof(t *testing.T) {
	withoutProof := CPUConstraintSummary{
		AllowedCPUs:          []int{0, 1},
		AllowedCPUsAuthority: CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
		ConstraintCount:      1,
	}
	ctx := RunnableContextSummary{
		CPUConstraint:  &withoutProof,
		OtherCPUIdleMs: 3,
	}
	if verdict, _ := runnableContextVerdict(ctx); verdict == "restricted_to_busy_or_small_cores" {
		t.Fatalf("a nonempty allowed mask without a universe-exclusion proof must not mint the strong restriction verdict")
	}

	withProof := withoutProof
	withProof.RestrictionProof = CPUConstraintRestrictionProofAllowedMaskExcludesUniverse
	withProof.ExcludedCPUs = []int{4}
	withProof.ExcludedCPUIdleMs = 3
	ctx.CPUConstraint = &withProof
	if verdict, confidence := runnableContextVerdict(ctx); verdict != "restricted_to_busy_or_small_cores" || confidence != 0.84 {
		t.Fatalf("the secondary verdict must reuse the same typed proof, got verdict=%q confidence=%.2f", verdict, confidence)
	}
}
