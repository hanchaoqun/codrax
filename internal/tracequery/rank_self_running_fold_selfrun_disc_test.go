package tracequery

// rank_self_running_fold_selfrun_disc_test.go — SELFRUN-DISC pins (§29.192①
// (b) user ruling; A2 件11(b) handoff §29.194, 2026-07-21): the self
// supply-fold 「量不了」 absence disclosure distinguishes the two honest
// zeros of the ELIM-SELF-FIX zero-deficit path — "unmeasurable" (KnownMs==0 ∧
// UnknownMs>0: frequency data absent, every slice folded at ratio 1) vs the
// affirmative "no loss" (KnownMs>0, gap 0: true full-frequency running).
//
// MUTATION self-checks (突变 cp 纪律, each verified red by hand):
//   - dropping the disclosure mint (return nil on the unknown branch) reds
//     TestSelfRunningFoldUnmeasuredMintsOnAllUnknownBasis;
//   - flipping the criterion to KnownMs>=0 (判据翻位: minting on a KNOWN
//     zero-deficit basis) reds
//     TestSelfRunningFoldUnmeasuredAbsentOnTrueFullFrequency;
//   - minting the disclosure beside a positive deficit (moving the mint
//     above the deficit check) reds
//     TestSelfRunningFoldUnmeasuredAbsentWhenDeficitSeatMints.
//
// The three fixtures share ONE trace shape (target runs twice around a
// dep2-woken sleep — the chain universe the SELF-ALL admission requires) and
// differ ONLY in the cpu_frequency sample lines, so every arm's flip is the
// frequency-coverage criterion and nothing else.

import (
	"fmt"
	"math"
	"testing"
)

// selfrunDiscTraceBase: app-100 runs [5.000..5.010] and [5.0302..5.040] on
// cpu1 around an S sleep woken by dep2-200 (the wakeup edge = the chain
// universe). No frequency lines here — the arms append their own.
const selfrunDiscTraceBase = `
        app-100 (100) [001] .... 4.999000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       dep2-200 (200) [002] .... 5.020000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep2 next_pid=200 next_prio=40
       dep2-200 (200) [002] .... 5.030000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.030200: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
       dep2-200 (200) [002] .... 5.030500: sched_switch: prev_comm=dep2 prev_pid=200 prev_prio=40 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.040000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`

func selfrunDiscQuery() Query {
	return Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.041,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 12}
}

// 正臂: NO cpu_frequency line anywhere — the fold basis is entirely unknown
// (KnownMs==0 ∧ UnknownMs>0), the deficit is an honest zero, and the typed
// absence disclosure mints INSTEAD of a seat (「量不了」≠「无损失」).
func TestSelfRunningFoldUnmeasuredMintsOnAllUnknownBasis(t *testing.T) {
	idx := buildTraceIndex(t, "selfrun_disc_nofreq.systrace", selfrunDiscTraceBase)
	rank := BuildRootCauseRank(idx, selfrunDiscQuery())
	d := rank.SelfRunningFoldUnmeasured
	if d == nil {
		t.Fatalf("all-unknown basis must mint the typed absence disclosure: %+v", rank.Items)
	}
	if d.Thread.PID != 100 {
		t.Fatalf("the disclosure subject is the analysis target: %+v", d)
	}
	if d.RunningMs <= 0 || math.Abs(d.RunningMs-d.UnknownMs) > 1e-9 {
		t.Fatalf("fold identity (KnownMs==0 ⇒ running==unknown) drifted: running=%.6f unknown=%.6f", d.RunningMs, d.UnknownMs)
	}
	// The window projects 19.8ms of running wall clock (10.0 + 9.8).
	if got := fmt.Sprintf("%.3f", d.RunningMs); got != "19.800" {
		t.Fatalf("window-projected running drifted (want 19.800): %s", got)
	}
	if d.LineStart <= 0 || d.LineEnd < d.LineStart {
		t.Fatalf("the disclosure must carry its interval line envelope: %+v", d)
	}
	// No seat: zero authority is never seated (§29.93 discipline unchanged).
	if seat := findSelfRunningSeat(rank.Items, 100); seat != nil {
		t.Fatalf("no self running seat may mint on a zero deficit: %+v", seat)
	}
}

// 负臂 (真满频): the target's CPU is governed at the trace-global fmax — the
// basis is fully KNOWN with a zero deficit (real "no loss"), and NOTHING
// mints: no seat, no disclosure. 判据翻位 (KnownMs==0 → KnownMs>=0) reds here.
func TestSelfRunningFoldUnmeasuredAbsentOnTrueFullFrequency(t *testing.T) {
	trace := "  tppmgr-sched-in-5850  (    2) [001] .... 4.900000: cpu_frequency: state=2000000 cpu_id=1\n" +
		selfrunDiscTraceBase
	idx := buildTraceIndex(t, "selfrun_disc_fullfreq.systrace", trace)
	rank := BuildRootCauseRank(idx, selfrunDiscQuery())
	if rank.SelfRunningFoldUnmeasured != nil {
		t.Fatalf("a KNOWN full-frequency zero must NOT wear the unmeasurable disclosure: %+v", rank.SelfRunningFoldUnmeasured)
	}
	if seat := findSelfRunningSeat(rank.Items, 100); seat != nil {
		t.Fatalf("full-frequency running mints no deficit seat: %+v", seat)
	}
}

// 负臂 (席在): the target's CPU runs governed at HALF the trace-global fmax —
// the deficit seat mints and the disclosure stays nil (有席不发; the seat
// itself carries the fold accounting).
func TestSelfRunningFoldUnmeasuredAbsentWhenDeficitSeatMints(t *testing.T) {
	trace := "  tppmgr-sched-in-5850  (    2) [001] .... 4.900000: cpu_frequency: state=1000000 cpu_id=1\n" +
		"  tppmgr-sched-in-5850  (    2) [001] .... 4.900010: cpu_frequency: state=2000000 cpu_id=3\n" +
		selfrunDiscTraceBase
	idx := buildTraceIndex(t, "selfrun_disc_deficit.systrace", trace)
	rank := BuildRootCauseRank(idx, selfrunDiscQuery())
	seat := findSelfRunningSeat(rank.Items, 100)
	if seat == nil {
		t.Fatalf("the governed-below-fmax window must mint the deficit seat: %+v", rank.Items)
	}
	if seat.SupplyFoldDeficitMs <= 0 {
		t.Fatalf("seat must carry a positive fold deficit: %+v", seat)
	}
	if rank.SelfRunningFoldUnmeasured != nil {
		t.Fatalf("a minted deficit seat excludes the absence disclosure (有席不发): %+v", rank.SelfRunningFoldUnmeasured)
	}
}
