package tracequery

import (
	"math"
	"strings"
	"testing"
)

// binder_attribution_p9_test.go — P9 binder attribution write-off gate pins
// (CR-1 件①, §29.42 案1 BINDER-MISATTR,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12).
//
// Both fixtures are LINE-FOR-LINE extractions from the customer witness trace
// /Users/han/opt/donghu/donghu.ftrace (never hand-assembled): the false-
// attribution form reproduces the 41006 report's Rank1 15.758ms segment, the
// positive-preservation form reproduces the genuine 1.409ms synchronous wait
// the same report folded into the E2 family.

// donghuP9FalseAttributionTrace — donghu.ftrace lines 12560/12561/12565/
// 12616/12618/12628/22915/22920/22996/23069/23083/23091/24430/24544
// (verbatim). Story: the target sends synchronous txn 12145963 at
// 13762.894465; its reply 12145964 completes at 13762.895412/.895468
// (~1.0ms round trip). ~97ms later the target sends ONEWAY txn 12146103
// (flags=0x11) and sleeps 13762.992415→13763.008173 (15.758ms — one frame
// period), woken by the frame-signal dispatch thread app-9511 (tgid 9432,
// which also printed `I|227|vsync in 48.59ms` and whose previous wakeup of
// the target at 13762.991435 gives the 16.738ms tick cadence). The pre-P9
// classifier attributed this pacing sleep to the long-completed txn 12145963
// because its send fell inside the 100ms lookback.
const donghuP9FalseAttributionTrace = `
 .ugc.aweme.lite-17267 (17267) [012] .... 13762.894465: binder_transaction: transaction=12145963 dest_node=254759 dest_proc=9743 dest_thread=0 reply=0 flags=0x10 code=0x19
 .ugc.aweme.lite-17267 (17267) [012] .... 13762.894481: sched_wakeup: comm=binder:496_9 pid=10961 prio=53 target_cpu=012
    binder:496_9-10961 ( 9743) [012] .... 13762.894540: binder_transaction_received: transaction=12145963
    binder:496_9-10961 ( 9743) [004] .... 13762.895412: binder_transaction: transaction=12145964 dest_node=0 dest_proc=17267 dest_thread=17267 reply=1 flags=0x0 code=0x0
    binder:496_9-10961 ( 9743) [004] .... 13762.895420: sched_wakeup: comm=.ugc.aweme.lite pid=17267 prio=53 target_cpu=012
 .ugc.aweme.lite-17267 (17267) [012] .... 13762.895468: binder_transaction_received: transaction=12145964
             app-9511  ( 9432) [001] .... 13762.991435: sched_wakeup: comm=.ugc.aweme.lite pid=17267 prio=53 target_cpu=012
             app-9511  ( 9432) [001] .... 13762.991470: print: I|227|vsync in 48.59ms
 .ugc.aweme.lite-17267 (17267) [012] .... 13762.992042: sched_switch: prev_comm=tppmgr-idle-12 prev_pid=0 prev_prio=-2 prev_state=R+ ==> next_comm=.ugc.aweme.lite next_pid=17267 next_prio=53 next_info=3fff,372,3,0,2,0
 .ugc.aweme.lite-17267 (17267) [012] .... 13762.992333: binder_transaction: transaction=12146103 dest_node=12027373 dest_proc=9432 dest_thread=0 reply=0 flags=0x11 code=0x3
    binder:227_4-10625 ( 9432) [001] .... 13762.992385: binder_transaction_received: transaction=12146103
  tppmgr-idle-12-282   (    2) [012] .... 13762.992415: sched_switch: prev_comm=.ugc.aweme.lite prev_pid=17267 prev_prio=53 prev_state=S ==> next_comm=tppmgr-idle-12 next_pid=0 next_prio=-2 next_info=1000,0,0,0,0,0
             app-9511  ( 9432) [000] .... 13763.008173: sched_wakeup: comm=.ugc.aweme.lite pid=17267 prio=53 target_cpu=012
 .ugc.aweme.lite-17267 (17267) [012] .... 13763.008824: sched_switch: prev_comm=tppmgr-idle-12 prev_pid=0 prev_prio=-2 prev_state=R+ ==> next_comm=.ugc.aweme.lite next_pid=17267 next_prio=53 next_info=3fff,323,3,0,2,0
`

// donghuP9TrueBinderWaitTrace — donghu.ftrace lines 4353/4354/4356/4369/
// 4442/4443/4446/4452 (verbatim): the genuine synchronous wait. The target
// sends txn 12145859 at 13762.835811, sleeps 13762.835861→13762.837270
// (1.409ms), the reply 12145860 lands INSIDE the segment (13762.837261) and
// the segment-ending waker binder:496_9-10961 belongs to the peer's process
// (tgid 9743). Attribution must be preserved byte-for-byte in spirit: same
// transaction, same duration, same peer.
const donghuP9TrueBinderWaitTrace = `
 .ugc.aweme.lite-17267 (17267) [004] .... 13762.835811: binder_transaction: transaction=12145859 dest_node=254759 dest_proc=9743 dest_thread=0 reply=0 flags=0x10 code=0x19
 .ugc.aweme.lite-17267 (17267) [004] .... 13762.835834: sched_wakeup: comm=binder:496_9 pid=10961 prio=53 target_cpu=004
    binder:496_9-10961 ( 9743) [004] .... 13762.835861: sched_switch: prev_comm=.ugc.aweme.lite prev_pid=17267 prev_prio=53 prev_state=S ==> next_comm=binder:496_9 next_pid=10961 next_prio=53 next_info=3fff,109,3,0,2,0
    binder:496_9-10961 ( 9743) [004] .... 13762.835943: binder_transaction_received: transaction=12145859
    binder:496_9-10961 ( 9743) [004] .... 13762.837261: binder_transaction: transaction=12145860 dest_node=0 dest_proc=17267 dest_thread=17267 reply=1 flags=0x0 code=0x0
    binder:496_9-10961 ( 9743) [004] .... 13762.837270: sched_wakeup: comm=.ugc.aweme.lite pid=17267 prio=53 target_cpu=004
 .ugc.aweme.lite-17267 (17267) [004] .... 13762.837301: sched_switch: prev_comm=idle/4 prev_pid=0 prev_prio=-1 prev_state=R ==> next_comm=.ugc.aweme.lite next_pid=17267 next_prio=53 next_info=3fff,149,3,0,2,0
 .ugc.aweme.lite-17267 (17267) [004] .... 13762.837338: binder_transaction_received: transaction=12145860
`

func TestP9BinderWriteOffRejectsCompletedTransactionAndMintsPacingIdle(t *testing.T) {
	idx := buildTraceIndex(t, "donghu_p9_false.ftrace", donghuP9FalseAttributionTrace)
	q := Query{PID: 17267, TimeStart: 13762.894, TimeEnd: 13763.010, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1}
	chain := BuildWakeupChain(idx, q)
	// Arm a: the 15.758ms frame-pacing segment must NOT be attributed to the
	// long-completed txn 12145963 (reply 12145964 finished ~97ms before the
	// segment started) — and the oneway 12146103 never participates at all.
	for _, wait := range chain.BinderWaits {
		if wait.TransactionID == 12145963 || wait.TransactionID == 12146103 {
			t.Fatalf("written-off/oneway transaction must not be attributed: %+v", wait)
		}
	}
	// Arm c: the segment re-mints on the pacing_idle lane with the typed
	// frame-period witness (waker cadence 16.738ms vs segment 15.758ms).
	if len(chain.PacingIdles) != 1 {
		t.Fatalf("expected exactly one pacing-idle segment, got %+v", chain.PacingIdles)
	}
	p := chain.PacingIdles[0]
	if p.Thread.PID != 17267 || p.Waker.PID != 9511 {
		t.Fatalf("pacing idle must carry the sleeper and the vsync-dispatch waker: %+v", p)
	}
	if math.Abs(p.DurationMs-15.758) > 0.01 {
		t.Fatalf("pacing idle duration should be the 15.758ms donghu segment, got %.3f", p.DurationMs)
	}
	if math.Abs(p.FramePeriodMs-16.738) > 0.01 {
		t.Fatalf("frame period should be the waker's 16.738ms tick cadence, got %.3f", p.FramePeriodMs)
	}
	if p.PeriodSource != binderPacingPeriodSourceCadence {
		t.Fatalf("period provenance must be typed: %+v", p)
	}
	foundRejected := false
	for _, id := range p.RejectedTransactionIDs {
		if id == 12145963 {
			foundRejected = true
		}
	}
	if !foundRejected {
		t.Fatalf("pacing idle must keep the written-off transaction audit trail: %+v", p)
	}
	if !strings.Contains(p.Summary, "frame-pacing idle") || !strings.Contains(p.Summary, "waiting for the next frame") {
		t.Fatalf("pacing idle summary must speak the pacing wording: %q", p.Summary)
	}
	// The write-off is disclosed once at chain level (honest accounting).
	if !containsSubstring(chain.Caveats, "wrote off") {
		t.Fatalf("binder write-off must be disclosed in the chain caveats: %+v", chain.Caveats)
	}
	// The pacing lane reaches RootEvidence (its rank/observation carrier).
	foundRoot := false
	for _, root := range chain.RootEvidence {
		if root.Type == "pacing_idle" {
			foundRoot = true
			if math.Abs(root.StartTs-p.WindowStartTs) > 1e-9 || math.Abs(root.EndTs-p.WindowEndTs) > 1e-9 {
				t.Fatalf("pacing root evidence must retain its own exact value interval, got root=%+v pacing=%+v", root, p)
			}
		}
		if root.Type == "binder_wait" {
			t.Fatalf("no binder_wait root evidence may survive the write-off on this fixture: %+v", root)
		}
	}
	if !foundRoot {
		t.Fatalf("pacing idle must publish as root evidence: %+v", chain.RootEvidence)
	}
	// Rank face: the pacing row is context, never a contender — tier
	// context_only, no board seat, and no binder_wait row anywhere.
	rank := BuildRootCauseRank(idx, q)
	sawPacing := false
	for _, item := range rank.Items {
		switch item.Type {
		case "binder_wait":
			t.Fatalf("binder_wait rank row must not survive the write-off: %+v", item)
		case "pacing_idle":
			sawPacing = true
			if item.Tier != RootCauseTierContextOnly {
				t.Fatalf("pacing_idle rank row must carry the context-only tier: %+v", item)
			}
			if item.Rank != 0 {
				t.Fatalf("pacing_idle rank row must not take a board seat: %+v", item)
			}
			if math.Abs(item.StartTs-p.WindowStartTs) > 1e-9 || math.Abs(item.EndTs-p.WindowEndTs) > 1e-9 {
				t.Fatalf("pacing rank row must retain the same evidence interval instead of borrowing a nearest-chain anchor: %+v", item)
			}
			if math.Abs((item.EndTs-item.StartTs)*1000-item.ImpactMs) > 0.001 {
				t.Fatalf("pacing rank value and interval must describe the same member: %+v", item)
			}
		}
	}
	if !sawPacing {
		t.Fatalf("pacing_idle rank row must be published (context lane): %+v", rank.Items)
	}
}

// TestP9PacingPeriodPlausibilityBand — the arm-c frame period must sit in
// the plausible display-refresh band (4ms..50ms). Donghu focused-replay
// witness: a 2.058ms RenderThread wake cadence briefly recast a 1.805ms
// sleep as frame-pacing idle (no display runs at ~486Hz). Pure verdict-math
// pin over the engine-built audit of the false-attribution fixture.
func TestP9PacingPeriodPlausibilityBand(t *testing.T) {
	idx := buildTraceIndex(t, "donghu_p9_false_band.ftrace", donghuP9FalseAttributionTrace)
	q := Query{PID: 17267, TimeStart: 13762.894, TimeEnd: 13763.010, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1}
	chain := BuildWakeupChain(idx, q)
	audit := buildBinderAttributionAudit(idx, chain)
	var node ChainNode
	var wake WakeupEdge
	found := false
	for i, n := range chain.Nodes {
		if edge, ok := audit.wakeEdgeByNode[i]; ok && n.Thread.PID == 17267 && edge.Waker.PID == 9511 {
			node, wake, found = n, edge, true
			break
		}
	}
	if !found {
		t.Fatalf("fixture must produce the app-9511-terminated sleep node")
	}
	if _, _, kind, ok := audit.pacingVerdict(chain, node, wake); !ok || kind != binderIdleKindPacing {
		t.Fatalf("the in-band 16.738ms frame-chain cadence must pass the pacing verdict with the frame kind, got kind=%q ok=%v", kind, ok)
	}
	// Same node, sub-band cadence: shrink the segment so the bracketing
	// cadence math is bypassed and only the band check varies — feed the
	// verdict a waker whose cadence sits below 4ms by shrinking the tick
	// spacing through a copied audit.
	shrunk := *audit
	shrunk.wakeupTsByPair = map[[2]int][]float64{
		{9511, 17267}: {wake.WakeupTs - 0.002058, wake.WakeupTs},
	}
	short := node
	short.DurationMs = 1.805
	if _, _, _, ok := shrunk.pacingVerdict(chain, short, wake); ok {
		t.Fatalf("a ~2.058ms cadence is below the plausible frame-period band and must not reroute")
	}
}

// TestP9PacingWordingForksOnFrameChainMembership (复核 P2-1, 2026-07-12):
// the frame promise words render ONLY for a frame-schedule-span waker. A
// generic periodic waker (typed VS-1 aggregate, no frame span — the
// timer/audio shape) reroutes on the periodic_idle lane with the periodic
// wording; without even the aggregate, a non-frame waker never reroutes at
// all (a bare two-wakeup cadence is not periodicity evidence).
func TestP9PacingWordingForksOnFrameChainMembership(t *testing.T) {
	idx := buildTraceIndex(t, "donghu_p9_fork.ftrace", donghuP9FalseAttributionTrace)
	q := Query{PID: 17267, TimeStart: 13762.894, TimeEnd: 13763.010, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1}
	chain := BuildWakeupChain(idx, q)
	audit := buildBinderAttributionAudit(idx, chain)
	var node ChainNode
	var wake WakeupEdge
	found := false
	for i, n := range chain.Nodes {
		if edge, ok := audit.wakeEdgeByNode[i]; ok && n.Thread.PID == 17267 && edge.Waker.PID == 9511 {
			node, wake, found = n, edge, true
			break
		}
	}
	if !found {
		t.Fatalf("fixture must produce the app-9511-terminated sleep node")
	}
	// Strip the waker's frame-chain membership: the cadence source becomes
	// inadmissible and the segment must NOT reroute (no aggregate evidence).
	noFrame := *audit
	noFrame.frameScheduleEmitters = map[int]bool{}
	if _, _, _, ok := noFrame.pacingVerdict(chain, node, wake); ok {
		t.Fatalf("a non-frame waker with only a two-wakeup cadence must not reroute")
	}
	// Give the same non-frame waker a typed VS-1 periodic aggregate: the
	// segment reroutes on the GENERIC periodic lane, never the frame lane.
	periodicChain := chain
	periodicChain.AggregatedImpacts = append(append([]WakeupCausalAggregate(nil), chain.AggregatedImpacts...), WakeupCausalAggregate{
		Thread:           wake.Waker,
		PeriodicSource:   true,
		DetectedPeriodMs: 16.738,
	})
	periodMs, _, kind, ok := noFrame.pacingVerdict(periodicChain, node, wake)
	if !ok || kind != binderIdleKindPeriodic {
		t.Fatalf("a measured periodic non-frame waker must reroute on the periodic lane, got kind=%q ok=%v", kind, ok)
	}
	if periodMs != 16.738 {
		t.Fatalf("the periodic lane's period must come from the typed aggregate, got %.3f", periodMs)
	}
	p := PacingIdleSummary{Thread: node.Thread, Waker: wake.Waker, DurationMs: node.DurationMs, FramePeriodMs: periodMs, Kind: kind}
	summary := renderPacingIdleSummary(p)
	if !strings.Contains(summary, "periodic idle") || !strings.Contains(summary, "waiting for the next periodic signal") {
		t.Fatalf("periodic lane summary must speak the periodic wording: %q", summary)
	}
	if strings.Contains(summary, "frame") {
		t.Fatalf("the frame promise words must never leak to a non-frame waker: %q", summary)
	}
}

// --- P2-2 (复核, 2026-07-12): single-arm independent witnesses -------------
//
// The donghu false-attribution fixture is killed by BOTH arms (defense in
// depth), so neither arm alone had a red witness. The two constructed shapes
// below isolate one arm each: disabling that arm (and only it) turns its
// fixture red.

// p9ArmAOnlyTrace — arm-a independent witness: the stale sync candidate's
// peer PROCESS equals the terminating waker's process (arm b passes), and
// only the reply write-off can reject. Client sends sync txn 100 to
// binder:900_1 (tgid 900), the reply completes at 5.001; the client later
// sleeps 5.050→5.060 (inside the 100ms send lookback) and is woken by
// binder:900_2 — ANOTHER thread of the SAME process 900.
const p9ArmAOnlyTrace = `
     client-20   (   20) [001] .... 5.000000: binder_transaction: transaction=100 dest_node=0 dest_proc=900 dest_thread=901 reply=0 flags=0x10 code=0x1
 binder:900_1-901 (  900) [002] .... 5.000100: binder_transaction_received: transaction=100
 binder:900_1-901 (  900) [002] .... 5.001000: binder_transaction: transaction=101 dest_node=0 dest_proc=20 dest_thread=20 reply=1 flags=0x0 code=0x0
 binder:900_1-901 (  900) [002] .... 5.001010: sched_wakeup: comm=client pid=20 prio=20 target_cpu=001
     client-20   (   20) [001] .... 5.001100: binder_transaction_received: transaction=101
     client-20   (   20) [001] .... 5.050000: sched_switch: prev_comm=client prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
 binder:900_2-902 (  900) [002] .... 5.060000: sched_wakeup: comm=client pid=20 prio=20 target_cpu=001
     client-20   (   20) [001] .... 5.060100: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=20
`

// p9ArmBOnlyTrace — arm-b independent witness: NO reply row exists anywhere
// (arm a fails open), and the terminating waker belongs to a DIFFERENT
// process than the binder peer. Client sends sync txn 200 to binder:900_1
// (tgid 900, received, never replied), sleeps 6.010→6.020 and is woken by
// timerd-555 (tgid 555).
const p9ArmBOnlyTrace = `
     client-20   (   20) [001] .... 6.000000: binder_transaction: transaction=200 dest_node=0 dest_proc=900 dest_thread=901 reply=0 flags=0x10 code=0x1
 binder:900_1-901 (  900) [002] .... 6.000100: binder_transaction_received: transaction=200
     client-20   (   20) [001] .... 6.010000: sched_switch: prev_comm=client prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
      timerd-555 (  555) [002] .... 6.020000: sched_wakeup: comm=client pid=20 prio=20 target_cpu=001
     client-20   (   20) [001] .... 6.020100: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=20
`

// p9ArmBExceptionTrace — the arm-b DISCLOSED exception: the waker process
// differs from the peer's, but the reply verifiably arrived INSIDE the
// segment — attribution stands with the typed mismatch caveat. Client sends
// sync txn 300 at 7.000, sleeps 7.001; the reply lands at 7.005; an
// unrelated notifier-333 (tgid 333) delivers the terminating wakeup 7.006.
const p9ArmBExceptionTrace = `
     client-20   (   20) [001] .... 7.000000: binder_transaction: transaction=300 dest_node=0 dest_proc=900 dest_thread=901 reply=0 flags=0x10 code=0x1
 binder:900_1-901 (  900) [002] .... 7.000100: binder_transaction_received: transaction=300
     client-20   (   20) [001] .... 7.001000: sched_switch: prev_comm=client prev_pid=20 prev_prio=20 prev_state=S ==> next_comm=swapper/1 next_pid=0 next_prio=120
 binder:900_1-901 (  900) [002] .... 7.005000: binder_transaction: transaction=301 dest_node=0 dest_proc=20 dest_thread=20 reply=1 flags=0x0 code=0x0
    notifier-333 (  333) [002] .... 7.006000: sched_wakeup: comm=client pid=20 prio=20 target_cpu=001
     client-20   (   20) [001] .... 7.006100: sched_switch: prev_comm=swapper/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=20
`

// TestP9ArmAIndependentReplyWriteOff — arm a alone must kill the stale
// candidate (the waker process MATCHES the peer, so arm b is silent).
func TestP9ArmAIndependentReplyWriteOff(t *testing.T) {
	idx := buildTraceIndex(t, "p9_arm_a_only.ftrace", p9ArmAOnlyTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 5.0, TimeEnd: 5.07, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1})
	if len(chain.BinderWaits) != 0 {
		t.Fatalf("the completed transaction must be written off even when the waker process matches the peer: %+v", chain.BinderWaits)
	}
	if !containsSubstring(chain.Caveats, "reply had already completed before the sleep segment started") {
		t.Fatalf("the reply-arm write-off must be disclosed: %+v", chain.Caveats)
	}
}

// TestP9ArmBIndependentWakerMismatchWriteOff — arm b alone must kill the
// candidate (no reply row exists, so arm a is silent), and the chain caveat
// speaks the waker branch.
func TestP9ArmBIndependentWakerMismatchWriteOff(t *testing.T) {
	idx := buildTraceIndex(t, "p9_arm_b_only.ftrace", p9ArmBOnlyTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 6.0, TimeEnd: 6.03, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1})
	if len(chain.BinderWaits) != 0 {
		t.Fatalf("a reply-less candidate whose segment-ending waker belongs to another process must be written off: %+v", chain.BinderWaits)
	}
	if !containsSubstring(chain.Caveats, "segment-ending waker belongs to a different process than the binder peer") {
		t.Fatalf("the waker-arm write-off must be disclosed: %+v", chain.Caveats)
	}
}

// TestP9ArmBExceptionKeepsAttributionWithDisclosure — the reply-in-segment
// exception: attribution stands and the typed mismatch caveat discloses the
// waker/peer process difference verbatim.
func TestP9ArmBExceptionKeepsAttributionWithDisclosure(t *testing.T) {
	idx := buildTraceIndex(t, "p9_arm_b_exception.ftrace", p9ArmBExceptionTrace)
	chain := BuildWakeupChain(idx, Query{PID: 20, TimeStart: 7.0, TimeEnd: 7.01, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1})
	if len(chain.BinderWaits) != 1 {
		t.Fatalf("a reply-inside-segment wait keeps its attribution despite the waker mismatch: %+v", chain.BinderWaits)
	}
	wait := chain.BinderWaits[0]
	if wait.TransactionID != 300 {
		t.Fatalf("attribution must stay on txn 300: %+v", wait)
	}
	if !containsSubstring(wait.Caveats, "belongs to a different process than binder peer") ||
		!containsSubstring(wait.Caveats, "the reply arrived inside the segment") {
		t.Fatalf("the exception must be disclosed on the wait itself: %+v", wait.Caveats)
	}
	if containsSubstring(chain.Caveats, "wrote off") {
		t.Fatalf("a disclosed exception is not a write-off: %+v", chain.Caveats)
	}
}

func TestP9BinderWriteOffPreservesGenuineSynchronousWait(t *testing.T) {
	idx := buildTraceIndex(t, "donghu_p9_true.ftrace", donghuP9TrueBinderWaitTrace)
	q := Query{PID: 17267, TimeStart: 13762.8355, TimeEnd: 13762.8375, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1}
	chain := BuildWakeupChain(idx, q)
	if len(chain.BinderWaits) != 1 {
		t.Fatalf("the genuine 1.409ms synchronous wait must keep its attribution, got %+v", chain.BinderWaits)
	}
	wait := chain.BinderWaits[0]
	if wait.TransactionID != 12145859 {
		t.Fatalf("attribution must stay on txn 12145859: %+v", wait)
	}
	if math.Abs(wait.DurationMs-1.409) > 0.01 {
		t.Fatalf("preserved wait should be the 1.409ms donghu segment, got %.3f", wait.DurationMs)
	}
	if wait.Peer.PID != 10961 || wait.Peer.TGID != 9743 {
		t.Fatalf("preserved wait must keep its peer identity: %+v", wait)
	}
	if len(chain.PacingIdles) != 0 {
		t.Fatalf("a genuine binder wait never reroutes to the pacing lane: %+v", chain.PacingIdles)
	}
	if containsSubstring(chain.Caveats, "wrote off") {
		t.Fatalf("no write-off disclosure may appear on a clean attribution: %+v", chain.Caveats)
	}
}
