package tracequery

// query_cpu_constraint_rnb2_test.go — RNB-2 件5 AFF-EVID engine pins
// (§29.88.6, ledger docs/design/real_trace_campaign_20260705.md, 2026-07-15):
//
//	(a) the cpu_affinity_or_cpuset rank seat carries the typed judgment
//	    payload (kind / cpuset / policy / allowed / observed-excluded);
//	(b) the constraint summary's evidence span covers only DECISIVE events —
//	    repeated identical next_info noise no longer widens it to the whole
//	    window (W5 witness E29: 行1308–27460 全窗 span 充数).

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func rnb2ConstraintFixture(t *testing.T) *Index {
	t.Helper()
	// cpu4 hosts activity so the observed CPU set exceeds the allowed set
	// {0,1} (mask=0x3) — the very comparison cpuConstraintRestrictsExecution
	// decides on; app-100 waits runnable 1.001→1.010 on cpu0.
	return buildTraceIndex(t, "rnb2_constraint.systrace", `
       idle/4-0   (    0) [004] .... 1.000000: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=big next_pid=301 next_prio=120
        bgA-200   (  900) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=bgA next_pid=200 next_prio=20
       ctrl-300   (  900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x3 cpuset=top-app target_cpu=0 policy=bind
       ctrl-300   (  900) [001] .... 1.001000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,4,1,1,0 cg=top-app
        app-100   (  100) [000] .... 1.011000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=bgA next_pid=200 next_prio=20
       ctrl-300   (  900) [001] .... 1.011200: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.011500: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,9,1,1,0 cg=top-app
        app-100   (  100) [000] .... 1.011700: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=bgA next_pid=200 next_prio=20
       ctrl-300   (  900) [001] .... 1.011800: sched_wakeup: comm=app pid=100 prio=52 target_cpu=000
        app-100   (  100) [000] .... 1.011900: sched_switch: prev_comm=bgA prev_pid=200 prev_prio=20 prev_state=R+ ==> next_comm=app next_pid=100 next_prio=52 next_info=3,7,1,1,0 cg=top-app
        app-100   (  100) [000] .... 1.012000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
}

// (a) the affinity seat carries the typed judgment payload.
func TestRNB2AffinitySeatCarriesJudgmentPayload(t *testing.T) {
	idx := rnb2ConstraintFixture(t)
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.012, CoreTopology: "small=0-3,big=4-7", TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	rank := BuildRootCauseRank(idx, q)
	var seat *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "cpu_affinity_or_cpuset" && rank.Items[i].Thread.PID == 100 {
			seat = &rank.Items[i]
			break
		}
	}
	if seat == nil {
		t.Fatalf("the affinity seat must mint for the constrained runnable thread: %+v", rank.Items)
	}
	if seat.CPUConstraintKind == "" {
		t.Fatalf("the judgment-basis kind must ride the seat: %+v", seat)
	}
	if seat.CPUConstraintCPUSet != "top-app" {
		t.Fatalf("the cpuset group must ride the seat, got %q", seat.CPUConstraintCPUSet)
	}
	if len(seat.CPUConstraintAllowedCPUs) == 0 || seat.CPUConstraintAllowedCPUs[0] != 0 ||
		seat.CPUConstraintAllowedCPUs[len(seat.CPUConstraintAllowedCPUs)-1] != 1 {
		t.Fatalf("the allowed union {0,1} must ride the seat, got %v", seat.CPUConstraintAllowedCPUs)
	}
	// cpu4 hosted in-window activity and is absent from the allowed set — the
	// exclusion the restriction gate compared.
	excludedHas4 := false
	for _, cpu := range seat.CPUConstraintExcludedCPUs {
		if cpu == 4 {
			excludedHas4 = true
		}
		if cpu == 0 || cpu == 1 {
			t.Fatalf("an allowed CPU may never appear in the exclusion list: %v", seat.CPUConstraintExcludedCPUs)
		}
	}
	if !excludedHas4 {
		t.Fatalf("the observed-but-excluded cpu4 must ride the exclusion list, got %v", seat.CPUConstraintExcludedCPUs)
	}
	if !strings.Contains(seat.CPUConstraintPolicy, "restricted=true") && seat.CPUConstraintPolicy != "bind" {
		t.Fatalf("the policy string must ride verbatim, got %q", seat.CPUConstraintPolicy)
	}
}

// (c) ENGINE-REAL witness (§29.88.8 SCAN-4 B锚点候选2 / §29.88.5 W5; donghu
// fixture, zero LLM): the JankManager-9655 mask=ffb seat carries the payload
// the ledger predicted — allowed [0,1,3..11], observed-excluded [2,12,13]
// (「AllowedCoreClasses 面写 [small,middle,big] 无法表达该排除」 was the filed
// gap; the exclusion list expresses it) — and the evidence span narrows to
// the decisive constraint event instead of the near-whole-trace envelope
// (W5 E29 form: 行1308–27460).
func TestRNB2AffinityDonghuRealPayloadAndNarrowedEvidence(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	check := func(pid int, start, end float64, wantSeatPID int, wantDemoted bool) {
		t.Helper()
		q := Query{PID: pid, TimeStart: start, TimeEnd: end, MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
		rank := BuildRootCauseRank(idx, q)
		var seat *RootCauseRankItem
		for i := range rank.Items {
			if rank.Items[i].Type == "cpu_affinity_or_cpuset" && rank.Items[i].Thread.PID == wantSeatPID {
				seat = &rank.Items[i]
				break
			}
		}
		if seat == nil {
			t.Fatalf("expected the affinity seat for pid=%d: %+v", wantSeatPID, rank.Items)
		}
		if seat.CPUConstraintKind != "sched_switch_next_info" {
			t.Fatalf("judgment-basis kind drifted: %q", seat.CPUConstraintKind)
		}
		if got := fmt.Sprintf("%v", seat.CPUConstraintAllowedCPUs); got != "[0 1 3 4 5 6 7 8 9 10 11]" {
			t.Fatalf("mask=ffb allowed set drifted: %v", seat.CPUConstraintAllowedCPUs)
		}
		if got := fmt.Sprintf("%v", seat.CPUConstraintExcludedCPUs); got != "[2 12 13]" {
			t.Fatalf("observed-excluded set drifted: %v", seat.CPUConstraintExcludedCPUs)
		}
		if seat.ChainCredentialLaneDemoted != wantDemoted {
			t.Fatalf("R4 demotion drifted (want %v): %+v", wantDemoted, seat)
		}
		// Narrowed evidence: the span covers decisive constraint events only —
		// never a five-digit near-whole-trace envelope.
		if seat.LineEnd-seat.LineStart > 100 {
			t.Fatalf("evidence span must stay on the decisive events (got lines %d-%d)", seat.LineStart, seat.LineEnd)
		}
	}
	// §29.88.8 候选2: the target's own on-chain mask=ffb seat.
	check(9655, 13762.934161, 13763.024898, 9655, false)
	// §29.88.5 W5: the logd.writer seat (RNB-1 R4 demoted to ◇).
	check(2955, 13762.791708, 13763.024898, 9163, true)
}

// (b) evidence narrowing: repeated identical next_info events (volatile load
// numbers only) do not widen the span; the decisive events do.
func TestRNB2ConstraintEvidenceSpanCoversDecisiveEventsOnly(t *testing.T) {
	idx := rnb2ConstraintFixture(t)
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.012, CoreTopology: "small=0-3,big=4-7", TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	var constraint *CPUConstraintSummary
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			constraint = &stats.CPUConstraints[i]
			break
		}
	}
	if constraint == nil {
		t.Fatalf("expected a cpu constraint for app-100: %+v", stats.CPUConstraints)
	}
	if constraint.ConstraintCount < 4 {
		t.Fatalf("fixture: the summary must have absorbed the repeated next_info events, got count=%d", constraint.ConstraintCount)
	}
	// Decisive events: the sched_setaffinity line (4) and the FIRST next_info
	// switch line (6 — restricted-flag first appearance). The later next_info
	// repeats (lines 9/12, volatile load only) must NOT widen the span.
	// (buildTraceIndex trims the leading newline; the fixture's first event
	// line is line 1.)
	if constraint.LineStart == 0 || constraint.LineEnd >= 9 {
		t.Fatalf("the evidence span must cover only decisive events (want end < 9), got lines %d-%d",
			constraint.LineStart, constraint.LineEnd)
	}
}
