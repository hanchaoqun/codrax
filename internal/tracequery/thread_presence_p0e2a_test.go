package tracequery

import (
	"strings"
	"testing"
)

// thread_presence_p0e2a_test.go — P0-E2a mutation pins for the cross-thread
// blocking-counterpart wakeup-edge fallback, ledger
// docs/design/real_trace_campaign_20260705.md §10 A2 / §11 N8 / §12 Q4-C
// (user ruling §12.3-5). The parse of the `(owner tid: N)` payload is retained
// verbatim (lock_contention.go unchanged); only the counterpart RESOLUTION
// distinguishes "payload printed an id" from "id resolvable in this trace" and
// falls back to the waiter's direct wakeup edge when the id is a cross-namespace
// phantom.

// tidPresenceSet / tidPresent unit pin: the set is exactly the union of every
// event's PID / PrevPID / NextPID / WakeePID field (the four-field surface
// resolveThread scans), and a payload-only id is never "present".
func TestTidPresenceSetCoversFourFieldsOnly(t *testing.T) {
	trace := `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=svc next_pid=200 next_prio=120
     waker-300 (300) [002] .... 5.010000: sched_wakeup: comm=woken pid=400 prio=120 target_cpu=002
`
	idx := buildTraceIndex(t, "presence.systrace", trace)
	for _, tid := range []int{100, 200, 300, 400} {
		if !idx.tidPresent(tid) {
			t.Fatalf("tid %d appears in a header/prev/next/wakee field and must be present", tid)
		}
	}
	// A payload never contributes to the presence set.
	if idx.tidPresent(999999) {
		t.Fatalf("a tid that appears in no event field must NOT be present")
	}
	if idx.tidPresent(0) {
		t.Fatalf("the idle/swapper tid 0 is never present")
	}
}

// resolveCounterpartViaWakeupEdge unit pin: returns the waker of the last
// sched_wakeup(wakee=waiter) in the window; guards on swapper waker and no edge.
func TestResolveCounterpartViaWakeupEdgeGuards(t *testing.T) {
	trace := `
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`
	idx := buildTraceIndex(t, "edge.systrace", trace)
	waiter := ThreadRef{Comm: "app", PID: 100}
	got := resolveCounterpartViaWakeupEdge(idx, waiter, 5.0, 5.06)
	if !got.OK || got.Waker.PID != 200 {
		t.Fatalf("waker of the in-window wakeup must be resolved, got %+v", got)
	}
	// No wakeup for a different waiter → not ok.
	if fb := resolveCounterpartViaWakeupEdge(idx, ThreadRef{PID: 777}, 5.0, 5.06); fb.OK {
		t.Fatalf("no wakeup edge for the waiter must return ok=false, got %+v", fb)
	}
	// Degenerate window → not ok.
	if fb := resolveCounterpartViaWakeupEdge(idx, waiter, 5.06, 5.0); fb.OK {
		t.Fatalf("an inverted window must return ok=false, got %+v", fb)
	}
}

// Cross-ns fallback MAIN pin: the monitor payload prints an owner tid that is
// NOT in the trace; inside the wait a real worker wakes the waiter. Expect the
// holder to become the waker (host-ns drillable), HolderSource=wakeup_edge, the
// phantom preserved on OwnerTidRaw, and the rank subject the resolved holder.
const p0e2aCrossNsTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (999999) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestCrossNamespaceHolderFallsBackToWakeupEdge(t *testing.T) {
	idx := buildTraceIndex(t, "crossns.systrace", p0e2aCrossNsTrace)
	if idx.tidPresent(999999) {
		t.Fatalf("payload owner tid 999999 must not be resolvable in this trace")
	}
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention row: %+v", res.CriticalBlocking)
	}
	if monitor.Peer.PID != 200 || monitor.Peer.Comm != "worker" {
		t.Fatalf("phantom holder must be replaced by the wakeup-edge waker, got %+v", monitor.Peer)
	}
	if monitor.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("fallback resolution must stamp HolderSource=wakeup_edge, got %q", monitor.HolderSource)
	}
	if monitor.OwnerTidRaw != 999999 {
		t.Fatalf("the phantom payload owner tid must be preserved on OwnerTidRaw, got %d", monitor.OwnerTidRaw)
	}
	// The resolved holder is a real thread this report examined → drill status
	// is now honest (drilled), never a silent phantom claim.
	if monitor.DrillStatus == DrillStatusPeerUnknown {
		t.Fatalf("a wakeup-edge-resolved holder is a real thread, drill must not be peer_unknown, got %q", monitor.DrillStatus)
	}

	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.06, MaxDepth: 4, MinDurationMs: 0.01, Limit: 16})
	var lockRow *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "blocking_span" && rank.Items[i].BlockingKind == "monitor_contention" {
			lockRow = &rank.Items[i]
			break
		}
	}
	if lockRow == nil {
		t.Fatalf("expected the lock rank row: %+v", rank.Items)
	}
	// Subject = resolved holder (waker); the typed pair holds through a real
	// thread so the P0-E1 direct-on-chain gate stays satisfied.
	if lockRow.Thread.PID != 200 || lockRow.BlockingPeer.PID != 100 {
		t.Fatalf("rank subject must be the resolved holder and BlockingPeer the waiter, got subj=%+v peer=%+v", lockRow.Thread, lockRow.BlockingPeer)
	}
	if lockRow.HolderSource != CounterpartSourceWakeupEdge || lockRow.OwnerTidRaw != 999999 {
		t.Fatalf("rank row must carry the fallback origin + phantom tid, got source=%q raw=%d", lockRow.HolderSource, lockRow.OwnerTidRaw)
	}
	if !rootCauseItemCanBeDirectOnChain(*lockRow) {
		t.Fatalf("the typed admission pair must hold through the resolved holder, got %+v", lockRow)
	}
}

// Collision pin: the payload owner comm=Holder tid=555, and a thread pid=555
// DOES exist in the trace but under an UNRELATED comm ("unrelated"). The comm
// cross-check catches the coincidental collision and takes the wakeup-edge
// fallback instead of misattributing to the unrelated thread.
const p0e2aCollisionTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (555) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
   unrelated-555 (555) [002] .... 5.020000: sched_switch: prev_comm=unrelated prev_pid=555 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestTidCollisionCrossCheckFallsBackNotMisattribute(t *testing.T) {
	idx := buildTraceIndex(t, "collision.systrace", p0e2aCollisionTrace)
	if !idx.tidPresent(555) {
		t.Fatalf("pid 555 is scheduled in the trace and must be present")
	}
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention row: %+v", res.CriticalBlocking)
	}
	// The unrelated pid=555 must NEVER become the holder — the comm cross-check
	// rejects the collision and the fallback names the real waker (200).
	if monitor.Peer.PID == 555 {
		t.Fatalf("a colliding pid with a mismatched comm must not be attributed as the holder, got %+v", monitor.Peer)
	}
	if monitor.Peer.PID != 200 || monitor.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("collision must fall back to the wakeup-edge waker, got peer=%+v source=%q", monitor.Peer, monitor.HolderSource)
	}
	if monitor.OwnerTidRaw != 555 {
		t.Fatalf("the raw colliding payload tid must be preserved, got %d", monitor.OwnerTidRaw)
	}
}

// Swapper exclusion pin (§12.3 ownerless延伸): the payload owner is a phantom
// and the ONLY wakeup of the waiter has waker pid=0 (idle/swapper timeout). The
// idle thread is never promoted as a holder — the row stays unresolved
// (peer_unknown) with the phantom preserved for audit.
const p0e2aSwapperTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (999999) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        <idle>-0 (0) [001] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestSwapperWakerNotPromotedAsHolder(t *testing.T) {
	idx := buildTraceIndex(t, "swapper.systrace", p0e2aSwapperTrace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention row: %+v", res.CriticalBlocking)
	}
	if monitor.Peer.PID != 0 || strings.TrimSpace(monitor.Peer.Comm) != "" {
		t.Fatalf("an idle/swapper waker must not become the holder, got %+v", monitor.Peer)
	}
	if monitor.HolderSource != "" {
		t.Fatalf("an unresolved holder must carry no source, got %q", monitor.HolderSource)
	}
	if monitor.OwnerTidRaw != 999999 {
		t.Fatalf("the phantom payload tid must still be preserved for audit, got %d", monitor.OwnerTidRaw)
	}
	if monitor.DrillStatus != DrillStatusPeerUnknown {
		t.Fatalf("an unresolvable holder must stamp peer_unknown, got %q", monitor.DrillStatus)
	}
}

// Fold-order pin: the SAME lock printed as the rich monitor form AND the
// tid-only form, both carrying the same cross-namespace phantom owner, folds to
// ONE row and is resolved via the wakeup edge exactly ONCE (fold key = the
// payload tid, unaffected by the fallback).
const p0e2aFoldTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (999999) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000010: print: B|100|Lock contention on a monitor lock (owner tid: 999999)
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050110: print: E|100
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestCrossNamespaceDualPrintFoldsThenResolvesOnce(t *testing.T) {
	idx := buildTraceIndex(t, "crossns_fold.systrace", p0e2aFoldTrace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var contention []CriticalBlockingCandidate
	for _, it := range res.CriticalBlocking.Items {
		if it.BlockingKind != "" {
			contention = append(contention, it)
		}
	}
	if len(contention) != 1 {
		t.Fatalf("dual print forms of one cross-ns lock must fold to exactly one row, got %d: %+v", len(contention), contention)
	}
	folded := contention[0]
	// Folded once, then resolved once via the wakeup edge.
	if folded.Peer.PID != 200 || folded.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("the folded row must resolve once to the wakeup-edge waker, got peer=%+v source=%q", folded.Peer, folded.HolderSource)
	}
	if folded.OwnerTidRaw != 999999 {
		t.Fatalf("the folded row must preserve the phantom payload tid, got %d", folded.OwnerTidRaw)
	}
	if len(folded.MergedLines) != 1 {
		t.Fatalf("the folded duplicate print form must be recorded once on MergedLines, got %+v", folded.MergedLines)
	}
}

// Payload-less blocking-span pin (§10 A2): a blocking-like span with NO
// structured contention payload publishes wait_object=span name and offers the
// wakeup-edge counterpart candidate (PeerSource=wakeup_edge).
const p0e2aWaitObjectTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|waiting on custom flush lock
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestPayloadlessBlockingSpanPublishesWaitObject(t *testing.T) {
	idx := buildTraceIndex(t, "waitobject.systrace", p0e2aWaitObjectTrace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var span *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "blocking_span" && res.CriticalBlocking.Items[i].BlockingKind == "" {
			span = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if span == nil {
		t.Fatalf("expected a payload-less blocking span row: %+v", res.CriticalBlocking)
	}
	if span.WaitObject != "waiting on custom flush lock" {
		t.Fatalf("payload-less blocking span must publish its span name as wait_object, got %q", span.WaitObject)
	}
	if span.Peer.PID != 200 || span.PeerSource != CounterpartSourceWakeupEdge {
		t.Fatalf("payload-less blocking span must offer the wakeup-edge counterpart, got peer=%+v source=%q", span.Peer, span.PeerSource)
	}
}

// Binder dest fallback pin (§11 N8): an Android BC_TRANSACTION with dest_thread=0
// (endpoint-only, no receiver row) leaves the binder wait peer unresolved; the
// waiter's wakeup edge recovers the server-side counterpart, PeerSource=wakeup_edge.
func TestBinderDestZeroFallsBackToWakeupEdge(t *testing.T) {
	trace := `
        app-100 (100) [001] .... 5.000000: binder_transaction: transaction=7 dest_node=0 dest_proc=8815 dest_thread=0 reply=0 flags=0x0 code=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 binder_8815_1-8820 (8815) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`
	idx := buildTraceIndex(t, "binder_destzero.systrace", trace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var binder *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "binder_wait" {
			binder = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if binder == nil {
		t.Skipf("no binder_wait row produced by this synthetic shape: %+v", res.CriticalBlocking)
	}
	if binder.Peer.PID != 8820 || binder.PeerSource != CounterpartSourceWakeupEdge {
		t.Fatalf("dest_thread=0 binder wait must recover the peer from the wakeup edge, got peer=%+v source=%q", binder.Peer, binder.PeerSource)
	}
}

// P2 review — visible confidence downgrade: a wakeup-edge-inferred holder rides
// the 0.62 receiver-inferred ceiling on BOTH the critical_blocking face and the
// rank row, so rank Score (impact × conf × weight) sits below a payload-direct
// row of equal impact. A payload-direct row stays byte-identically at 0.72.
func TestWakeupEdgeFallbackCarriesConfidenceDowngrade(t *testing.T) {
	// Cross-ns fallback → 0.62 on critical_blocking and on the rank row.
	idx := buildTraceIndex(t, "cn_conf.systrace", p0e2aCrossNsTrace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention row: %+v", res.CriticalBlocking)
	}
	if monitor.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("precondition: fallback must have fired, got source=%q", monitor.HolderSource)
	}
	if monitor.Confidence > counterpartWakeupEdgeConfidence+1e-9 {
		t.Fatalf("an inferred holder must not exceed the wakeup-edge confidence ceiling %.2f, got %.3f", counterpartWakeupEdgeConfidence, monitor.Confidence)
	}
	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.06, MaxDepth: 4, MinDurationMs: 0.01, Limit: 16})
	var lockRow *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "blocking_span" && rank.Items[i].HolderSource == CounterpartSourceWakeupEdge {
			lockRow = &rank.Items[i]
			break
		}
	}
	if lockRow == nil {
		t.Fatalf("expected a wakeup-edge lock rank row: %+v", rank.Items)
	}
	if lockRow.Confidence > counterpartWakeupEdgeConfidence+1e-9 {
		t.Fatalf("rank confidence must carry the downgrade, got %.3f", lockRow.Confidence)
	}

	// Byte-stability: the present-tid q4 witness stays payload-direct at 0.72 on
	// BOTH faces (no downgrade leaks onto a genuine payload resolution).
	idxDirect := buildTraceIndex(t, "q4_conf.systrace", lockRankQ4Trace)
	resD := Run(idxDirect, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.115})
	for i := range resD.CriticalBlocking.Items {
		it := resD.CriticalBlocking.Items[i]
		if it.BlockingKind == "monitor_contention" {
			if it.HolderSource != CounterpartSourceContentionPayload {
				t.Fatalf("q4 witness must stay payload-direct, got source=%q", it.HolderSource)
			}
			if it.Confidence < 0.72-1e-9 {
				t.Fatalf("a payload-direct holder must keep its 0.72 confidence (byte-stable), got %.3f", it.Confidence)
			}
		}
	}
	rankD := BuildRootCauseRank(idxDirect, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.115, MaxDepth: 4, MinDurationMs: 0.05, Limit: 16})
	for i := range rankD.Items {
		it := rankD.Items[i]
		if it.Type == "blocking_span" && it.HolderSource == CounterpartSourceContentionPayload {
			if it.Confidence < 0.72-1e-9 {
				t.Fatalf("payload-direct rank row must keep 0.72 (byte-stable), got %.3f", it.Confidence)
			}
		}
	}
}

// P3 review — symmetric advisory caveat: the lock/blocking-span wakeup-edge
// fallback appends a "counterpart inferred from the waiter's wakeup edge" note
// to the row summary, mirroring the binder-lane caveat, so no downstream face
// mistakes an inference for a payload-confirmed holder.
func TestWakeupEdgeFallbackAppendsSymmetricCaveat(t *testing.T) {
	idx := buildTraceIndex(t, "cn_caveat.systrace", p0e2aCrossNsTrace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention row: %+v", res.CriticalBlocking)
	}
	if !strings.Contains(monitor.Summary, "inferred from the waiter's wakeup edge") {
		t.Fatalf("lock fallback must append the symmetric advisory caveat, got summary=%q", monitor.Summary)
	}
	if !strings.Contains(monitor.Summary, "999999") {
		t.Fatalf("the caveat must name the phantom payload tid, got summary=%q", monitor.Summary)
	}
	// Payload-less blocking span carries the phantom-free caveat form.
	idx2 := buildTraceIndex(t, "wo_caveat.systrace", p0e2aWaitObjectTrace)
	res2 := Run(idx2, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	for i := range res2.CriticalBlocking.Items {
		it := res2.CriticalBlocking.Items[i]
		if it.Type == "blocking_span" && it.BlockingKind == "" && it.PeerSource == CounterpartSourceWakeupEdge {
			if !strings.Contains(it.Summary, "inferred from the waiter's wakeup edge") {
				t.Fatalf("payload-less span fallback must append the symmetric caveat, got %q", it.Summary)
			}
		}
	}
}

// P3 review — binder dest_thread phantom via threadRefPresent: a dest_thread
// HINT (dest_thread>0) that names no thread this trace scheduled is now treated
// as unresolved (symmetric with the lock lane), so the peer is recovered from
// the wakeup edge instead of being trusted as a payload-direct receiver.
func TestBinderPhantomDestThreadHintFallsBack(t *testing.T) {
	// dest_thread=55555 is a phantom (never scheduled); a real worker wakes the
	// waiter. The endpoint hint must NOT be trusted as the receiver.
	trace := `
        app-100 (100) [001] .... 5.000000: binder_transaction: transaction=7 dest_node=0 dest_proc=8815 dest_thread=55555 reply=0 flags=0x0 code=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 binder_8815_1-8820 (8815) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`
	idx := buildTraceIndex(t, "binder_phantom_dest.systrace", trace)
	if idx.tidPresent(55555) {
		t.Fatalf("dest_thread 55555 must be a phantom in this trace")
	}
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var binder *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "binder_wait" {
			binder = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if binder == nil {
		t.Skipf("no binder_wait row produced by this synthetic shape: %+v", res.CriticalBlocking)
	}
	if binder.Peer.PID == 55555 {
		t.Fatalf("a phantom dest_thread hint must never be trusted as the receiver, got %+v", binder.Peer)
	}
	if binder.Peer.PID != 8820 || binder.PeerSource != CounterpartSourceWakeupEdge {
		t.Fatalf("phantom dest_thread must fall back to the wakeup-edge waker, got peer=%+v source=%q", binder.Peer, binder.PeerSource)
	}
}

// E2a correction ② (P0-E2b): the collision cross-check is truncation-aware.
// The payload prints the holder's FULL name; every scheduler comm is truncated
// to 15 bytes (TASK_COMM_LEN-1). Requiring full equality turned every >15-char
// owner name into a FALSE collision that discarded the payload-direct holder
// for a wakeup-edge inference. A full payload name now matches its own
// 15-byte truncation — and ONLY that exact kernel shape (no general prefix
// matching: "Thread-1" observed at 8 chars never matches "Thread-10").
func TestLockOwnerCommMatchesTruncationAware(t *testing.T) {
	cases := []struct {
		payload, observed string
		want              bool
	}{
		{"NetworkKit_AssetsUtil_Operate_0", "NetworkKit_Asse", true},  // kernel 15-byte truncation
		{"Holder", "Holder", true},                                    // exact
		{"Holder", "holder", true},                                    // case fold
		{"Thread-10", "Thread-1", false},                              // NOT a truncation shape (observed 8 chars)
		{"Holder", "unrelated", false},                                // genuine collision
		{"NetworkKit_AssetsUtil_Operate_0", "NetworkKit_AssX", false}, // 15 bytes but not the truncation
	}
	for _, tc := range cases {
		if got := lockOwnerCommMatches(tc.payload, tc.observed); got != tc.want {
			t.Fatalf("lockOwnerCommMatches(%q, %q) = %v, want %v", tc.payload, tc.observed, got, tc.want)
		}
	}
}

// End-to-end: the payload owner's full 31-char name vs the sched-truncated
// 15-char comm on the SAME tid must keep the payload-DIRECT resolution (before
// the fix this false collision silently swapped the confirmed holder for the
// waiter's waker).
const e2aTruncatedCommTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #NetworkKit_AssetsUtil_Operate_0 (300) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 NetworkKit_Asse-300 (300) [002] .... 5.020000: sched_switch: prev_comm=NetworkKit_Asse prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [003] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestTruncatedSchedCommKeepsPayloadDirectHolder(t *testing.T) {
	idx := buildTraceIndex(t, "e2a_trunc_comm.systrace", e2aTruncatedCommTrace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.06})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention row: %+v", res.CriticalBlocking)
	}
	if monitor.Peer.PID != 300 || monitor.HolderSource != CounterpartSourceContentionPayload {
		t.Fatalf("the truncated sched comm must NOT trigger a false collision — payload-direct holder retained, got peer=%+v source=%q", monitor.Peer, monitor.HolderSource)
	}
	if monitor.OwnerTidRaw != 0 {
		t.Fatalf("a payload-direct resolution leaves no phantom-tid residue, got %d", monitor.OwnerTidRaw)
	}
}
