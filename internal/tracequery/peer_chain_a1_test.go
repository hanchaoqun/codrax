package tracequery

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// peer_chain_a1_test.go — P0-E2b mutation pins for the A1 bounded continuation
// (ledger docs/design/real_trace_campaign_20260705.md §12.3-5 ruling 5). Given
// a RESOLVED blocking counterpart (P0-E2a resolves the holder/peer), take ONE
// sub-goal hop further: the peer's OWN state decomposition + its single direct
// 1-hop blocker. The continuation is HARD-CAPPED at depth 1 — the peer's peer is
// never expanded — and a continuation off a wakeup-edge-inferred counterpart
// inherits presumptive confidence.

// A1 MAIN pin: the monitor payload names a holder tid (300) that IS present in
// the trace (payload-direct, drillable). The holder is itself sleep-dominated
// over the blocking window and is woken by an upstream thread (400). Expect:
//   - PeerChain present, hangs off the resolved holder (300);
//   - the holder's own dominant state = s_sleep (it was itself blocked);
//   - the holder's single DIRECT blocker = its waker (400), depth 1;
//   - NOT presumptive (the holder was payload-direct, not inferred);
//   - F2: the blocker itself is ALWAYS a wakeup-edge inference — it carries
//     DirectBlockerSource=wakeup_edge and the step confidence is demoted to the
//     inference ceiling even though the PEER was payload-direct (a direct peer
//     never lends its evidence grade to an inferred hop-2 name).
const a1PayloadDirectHolderSleptTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (300) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     holder-300 (300) [002] .... 5.000200: sched_switch: prev_comm=holder prev_pid=300 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
   upstream-400 (400) [002] .... 5.030000: sched_wakeup: comm=holder pid=300 prio=120 target_cpu=002
     holder-300 (300) [002] .... 5.030100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=holder next_pid=300 next_prio=120
     holder-300 (300) [002] .... 5.040000: sched_switch: prev_comm=holder prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [003] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestPeerChainPayloadDirectHolderNamesDirectBlocker(t *testing.T) {
	idx := buildTraceIndex(t, "a1_payload_direct.systrace", a1PayloadDirectHolderSleptTrace)
	if !idx.tidPresent(300) {
		t.Fatalf("holder tid 300 is scheduled in the trace and must be present (payload-direct)")
	}
	monitor := findMonitorContentionRow(t, idx, 100, 4.99, 5.06)
	if monitor.Peer.PID != 300 || monitor.HolderSource != CounterpartSourceContentionPayload {
		t.Fatalf("holder must resolve payload-direct to tid 300, got peer=%+v source=%q", monitor.Peer, monitor.HolderSource)
	}
	if monitor.PeerChain == nil {
		t.Fatalf("a resolved counterpart must carry the A1 continuation, got nil PeerChain")
	}
	step := monitor.PeerChain
	if step.Peer.PID != 300 {
		t.Fatalf("the continuation must hang off the resolved holder (300), got %+v", step.Peer)
	}
	if step.State == nil || step.State.DominantState != string(StateSSleep) {
		t.Fatalf("the sleep-dominated holder's own state must be decomposed as s_sleep, got %+v", step.State)
	}
	// One hop: the holder's own direct blocker is its upstream waker (400).
	if step.DirectBlocker.PID != 400 {
		t.Fatalf("the holder's single direct 1-hop blocker must be its waker (400), got %+v", step.DirectBlocker)
	}
	if step.Presumptive {
		t.Fatalf("a payload-direct holder's continuation must NOT be presumptive, got presumptive=true")
	}
	// F2: the hop-2 name is structurally a wakeup-edge inference — always
	// stamped and always demoted, regardless of the peer's own direct origin.
	if step.DirectBlockerSource != CounterpartSourceWakeupEdge {
		t.Fatalf("a named direct blocker must ALWAYS carry DirectBlockerSource=wakeup_edge, got %q", step.DirectBlockerSource)
	}
	if step.Confidence > counterpartWakeupEdgeConfidence+1e-9 {
		t.Fatalf("a step carrying an inferred hop-2 blocker must ride the inference ceiling %.2f, got %.3f", counterpartWakeupEdgeConfidence, step.Confidence)
	}
	if !strings.Contains(step.Summary, "via wakeup edge") || !strings.Contains(step.Summary, "inferred") {
		t.Fatalf("the blocker clause must be labelled as a wakeup-edge inference, got %q", step.Summary)
	}
	// The PEER's own origin label stays payload-direct — only the blocker
	// clause is inference-labelled.
	if !strings.Contains(step.Summary, "peer payload-direct") {
		t.Fatalf("the peer's own origin label must remain payload-direct, got %q", step.Summary)
	}
}

// A1 BOUNDEDNESS pin (the q1 L31-33 deep-chain-blowup lesson): the direct
// blocker (400) itself has an upstream waker (500) inside the window, but the
// continuation MUST NOT resolve it — depth is hard-capped at 1. The blocker's
// second-hop blocker is never named; only its bare dominant-state word rides
// DirectBlockerState.
const a1DepthCapTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (300) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     holder-300 (300) [002] .... 5.000200: sched_switch: prev_comm=holder prev_pid=300 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
   upstream-400 (400) [002] .... 5.010000: sched_switch: prev_comm=upstream prev_pid=400 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
   topmost-500 (500) [003] .... 5.020000: sched_wakeup: comm=upstream pid=400 prio=120 target_cpu=002
   upstream-400 (400) [002] .... 5.020100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=upstream next_pid=400 next_prio=120
   upstream-400 (400) [002] .... 5.030000: sched_wakeup: comm=holder pid=300 prio=120 target_cpu=002
     holder-300 (300) [002] .... 5.030100: sched_switch: prev_comm=upstream prev_pid=400 prev_prio=120 prev_state=R ==> next_comm=holder next_pid=300 next_prio=120
     holder-300 (300) [002] .... 5.040000: sched_switch: prev_comm=holder prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [004] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestPeerChainDepthCappedAtOneHop(t *testing.T) {
	idx := buildTraceIndex(t, "a1_depth_cap.systrace", a1DepthCapTrace)
	monitor := findMonitorContentionRow(t, idx, 100, 4.99, 5.06)
	if monitor.PeerChain == nil {
		t.Fatalf("expected the A1 continuation, got nil")
	}
	step := monitor.PeerChain
	// Exactly one hop: the holder's direct blocker is 400.
	if step.DirectBlocker.PID != 400 {
		t.Fatalf("the holder's single direct blocker must be 400, got %+v", step.DirectBlocker)
	}
	// F4: the depth cap is asserted on the FULL serialized face, not on string
	// spot-checks — the second-hop thread (topmost-500, the blocker's own waker,
	// present in the window) must be absent from EVERY field json.Marshal can
	// see, so a future field addition smuggling hop-2 data fails here.
	payload, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal continuation: %v", err)
	}
	serialized := string(payload)
	if strings.Contains(serialized, "topmost") || strings.Contains(serialized, `"pid":500`) {
		t.Fatalf("the second-hop blocker (topmost-500) must NEVER appear anywhere in the serialized depth-1 continuation, got:\n%s", serialized)
	}
	// The blocker's own state is a bare dominant word, not a recursive breakdown.
	if step.DirectBlockerState == "" {
		t.Fatalf("the direct blocker's bare dominant state should be recorded, got empty")
	}
}

// F4 companion golden: the PeerChainStep field set itself is pinned. Depth-1
// boundedness is a property of the SHAPE — any new field (e.g. a second-hop
// carrier, a nested step, a blocker breakdown) must consciously pass review
// here before it can exist, exactly like the registry goldens.
func TestPeerChainStepFieldSetGolden(t *testing.T) {
	want := []string{
		"Confidence",
		"DirectBlocker",
		"DirectBlockerSource",
		"DirectBlockerState",
		"Peer",
		"Presumptive",
		"State",
		"Summary",
	}
	typ := reflect.TypeOf(PeerChainStep{})
	got := make([]string, 0, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("PeerChainStep field set drifted — depth-1 boundedness is a shape property; review the new/removed field against the q1 L31-33 lesson, then update this golden.\n got: %v\nwant: %v", got, want)
	}
	// DirectBlockerState/DirectBlockerSource must stay bare strings and State a
	// single breakdown — a struct/slice hop-2 carrier is the exact blowup shape
	// the cap forbids.
	if f, _ := typ.FieldByName("DirectBlockerState"); f.Type.Kind() != reflect.String {
		t.Fatalf("DirectBlockerState must stay a bare string, got %s", f.Type)
	}
	if f, _ := typ.FieldByName("DirectBlockerSource"); f.Type.Kind() != reflect.String {
		t.Fatalf("DirectBlockerSource must stay a bare string, got %s", f.Type)
	}
}

// A1 PRESUMPTION-INHERITANCE pin (§12.3-5): when the counterpart itself was only
// wakeup-edge-inferred (the payload owner tid was a cross-ns phantom), the whole
// continuation inherits presumptive confidence — an inference built on an
// inference is never presented as direct evidence.
const a1PresumptiveTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (999999) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
   upstream-400 (400) [003] .... 5.020000: sched_wakeup: comm=worker pid=200 prio=120 target_cpu=002
     worker-200 (200) [002] .... 5.020100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=120
     worker-200 (200) [002] .... 5.049000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestPeerChainInheritsPresumptiveFromInferredCounterpart(t *testing.T) {
	idx := buildTraceIndex(t, "a1_presumptive.systrace", a1PresumptiveTrace)
	if idx.tidPresent(999999) {
		t.Fatalf("the phantom payload owner tid 999999 must not be present")
	}
	monitor := findMonitorContentionRow(t, idx, 100, 4.99, 5.06)
	// The holder was recovered from the waiter's wakeup edge (worker-200), an
	// inference, not a payload-direct resolution.
	if monitor.HolderSource != CounterpartSourceWakeupEdge || monitor.Peer.PID != 200 {
		t.Fatalf("holder must be wakeup-edge-inferred to 200, got peer=%+v source=%q", monitor.Peer, monitor.HolderSource)
	}
	if monitor.PeerChain == nil {
		t.Fatalf("expected the A1 continuation off the inferred holder, got nil")
	}
	step := monitor.PeerChain
	if !step.Presumptive {
		t.Fatalf("a continuation off a wakeup-edge-inferred counterpart MUST be presumptive")
	}
	// Presumptive continuation confidence never exceeds the inference ceiling.
	if step.Confidence > counterpartWakeupEdgeConfidence+1e-9 {
		t.Fatalf("a presumptive continuation must ride the inference ceiling %.2f, got %.3f", counterpartWakeupEdgeConfidence, step.Confidence)
	}
	if !strings.Contains(step.Summary, "inferred") {
		t.Fatalf("a presumptive continuation summary must say it is inferred, got %q", step.Summary)
	}
}

// A1 NON-BLOCKED-PEER pin: a running/runnable-dominant peer was NOT itself
// sleep-blocked, so the continuation legitimately terminates — no DirectBlocker
// is fabricated (the chain ends because the peer was busy, not because we
// refused to look).
const a1RunningHolderTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (300) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     holder-300 (300) [002] .... 5.000200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=holder next_pid=300 next_prio=120
     holder-300 (300) [002] .... 5.049000: sched_switch: prev_comm=holder prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [003] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestPeerChainRunningHolderTerminatesWithoutBlocker(t *testing.T) {
	idx := buildTraceIndex(t, "a1_running_holder.systrace", a1RunningHolderTrace)
	monitor := findMonitorContentionRow(t, idx, 100, 4.99, 5.06)
	if monitor.Peer.PID != 300 {
		t.Fatalf("holder must resolve to 300, got %+v", monitor.Peer)
	}
	if monitor.PeerChain == nil {
		t.Fatalf("even a busy holder gets a continuation carrying its own state, got nil")
	}
	step := monitor.PeerChain
	if step.State == nil || step.State.DominantState == string(StateSSleep) {
		t.Fatalf("a running-dominant holder must not be decomposed as s_sleep, got %+v", step.State)
	}
	// The busy holder was not itself sleep-blocked — no fabricated direct blocker.
	if step.DirectBlocker.PID != 0 || strings.TrimSpace(step.DirectBlocker.Comm) != "" {
		t.Fatalf("a non-sleep-dominated holder must name no direct blocker, got %+v", step.DirectBlocker)
	}
	// F2 counter-face: with NO blocker aboard and a payload-direct peer, the
	// state decomposition is pure timeline fact — confidence is NOT demoted.
	if step.Confidence <= counterpartWakeupEdgeConfidence {
		t.Fatalf("a blocker-less payload-direct continuation is direct evidence and must not ride the inference ceiling, got %.3f", step.Confidence)
	}
	if step.DirectBlockerSource != "" {
		t.Fatalf("no blocker → no blocker source, got %q", step.DirectBlockerSource)
	}
}

// F1 pin (P0-E2b review, the most severe finding): sync request-reply shape —
// the waiter (app-100) WAKES the holder inside its own blocking window (the
// request), then sleeps until the holder replies. The holder's last in-window
// wakeup edge therefore names the WAITER itself. Publishing "the holder's
// direct blocker is app-100 (and app-100 is sleeping)" would be a causal
// inversion loop (A blocked on B, "B blocked on A") the model would narrate as
// a fake deadlock — the self-referential edge is DISCARDED outright: the
// continuation carries the holder's state but NO direct blocker.
const a1SyncRequestReplyTrace = `
        app-100 (100) [001] .... 5.000000: print: B|100|monitor contention with owner #Holder (300) at Foo.list(Foo.java:12) waiters=1
        app-100 (100) [001] .... 5.000020: sched_wakeup: comm=holder pid=300 prio=120 target_cpu=002
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     holder-300 (300) [002] .... 5.000200: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=holder next_pid=300 next_prio=120
     holder-300 (300) [002] .... 5.001000: sched_switch: prev_comm=holder prev_pid=300 prev_prio=120 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
     worker-200 (200) [003] .... 5.050000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.050100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.050200: print: E|100
`

func TestPeerChainSyncRequestReplyNeverNamesWaiterAsBlocker(t *testing.T) {
	idx := buildTraceIndex(t, "a1_sync_selfloop.systrace", a1SyncRequestReplyTrace)
	monitor := findMonitorContentionRow(t, idx, 100, 4.99, 5.06)
	if monitor.Peer.PID != 300 {
		t.Fatalf("holder must resolve to 300, got %+v", monitor.Peer)
	}
	if monitor.PeerChain == nil {
		t.Fatalf("the continuation itself must still exist (holder state is real), got nil")
	}
	step := monitor.PeerChain
	if step.State == nil || step.State.DominantState != string(StateSSleep) {
		t.Fatalf("the holder is sleep-dominated in this shape, got %+v", step.State)
	}
	// The ONLY in-window wakeup of the holder came from the waiter itself — the
	// self-referential edge must be discarded, never published as a blocker.
	if step.DirectBlocker.PID != 0 || strings.TrimSpace(step.DirectBlocker.Comm) != "" {
		t.Fatalf("the waiter must NEVER be named as its own blocker's blocker (causal inversion), got %+v", step.DirectBlocker)
	}
	if step.DirectBlockerSource != "" || step.DirectBlockerState != "" {
		t.Fatalf("a discarded self-loop edge must leave no blocker residue, got source=%q state=%q", step.DirectBlockerSource, step.DirectBlockerState)
	}
	// Serialized face: the WAITER (app-100) must appear NOWHERE in the
	// continuation — not as blocker, not in any field json.Marshal can see
	// (note: an empty direct_blocker struct key still serializes; the identity
	// absence is the load-bearing check).
	payload, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	serialized := string(payload)
	if strings.Contains(serialized, `"pid":100`) || strings.Contains(serialized, "app") {
		t.Fatalf("the waiter's identity must never serialize inside the self-loop-discarded continuation, got:\n%s", serialized)
	}
}

// RCX③ engine pin (§12.3-1 ③ + F3 rework): BuildFrameRootCauseBundle assembles
// a typed causal skeleton whose target_state layer is the TARGET's own dominant
// wait (its timeline decomposition — never a rank-row fragment, never the
// holder via a rank-head fallback), whose direct_blocker layer names the
// resolved lock holder with its typed source, and whose nodes stay in
// causal-LAYER order (never re-sorted by ms).
func TestBuildCausalSkeletonLayersFromRealBundle(t *testing.T) {
	idx := buildTraceIndex(t, "skeleton_engine.systrace", a1PayloadDirectHolderSleptTrace)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.06, MaxDepth: 4, MinDurationMs: 0.01, Limit: 16})
	if bundle.Skeleton == nil {
		t.Fatalf("a bundle with a resolved lock holder must carry a causal skeleton")
	}
	skel := bundle.Skeleton
	// F3 pin (q4 shape): the target_state layer is the TARGET's real dominant
	// wait — app-100 sleeps ~50ms of the window behind the lock — never a
	// sub-ms fragment and NEVER another thread (the rank-head fallback that
	// could put the holder here is deleted).
	var targetState *CausalSkeletonNode
	for i := range skel.Nodes {
		if skel.Nodes[i].Layer == CausalSkeletonLayerTargetState {
			targetState = &skel.Nodes[i]
			break
		}
	}
	if targetState == nil {
		t.Fatalf("skeleton must carry the target_state layer, got nodes: %+v", skel.Nodes)
	}
	if targetState.Thread.PID != 100 {
		t.Fatalf("target_state must be the TARGET's own thread (100), never another subject, got %+v", targetState.Thread)
	}
	if targetState.State != string(StateSSleep) {
		t.Fatalf("target_state must name the target's real dominant wait (s_sleep), got %q", targetState.State)
	}
	if targetState.MeasuredMs < 10 {
		t.Fatalf("target_state must carry the real dominant-wait magnitude (~50ms here), not a sub-ms fragment, got %.3fms", targetState.MeasuredMs)
	}
	// The direct_blocker layer must exist and name the resolved holder (300) with
	// its payload-direct source.
	var blocker *CausalSkeletonNode
	for i := range skel.Nodes {
		if skel.Nodes[i].Layer == CausalSkeletonLayerDirectBlocker {
			blocker = &skel.Nodes[i]
			break
		}
	}
	if blocker == nil {
		t.Fatalf("skeleton must carry a direct_blocker layer, got nodes: %+v", skel.Nodes)
	}
	if blocker.Thread.PID != 300 {
		t.Fatalf("direct_blocker must name the resolved lock holder (300), got %+v", blocker.Thread)
	}
	if blocker.CounterpartSource != CounterpartSourceContentionPayload {
		t.Fatalf("payload-direct holder must carry counterpart_source=contention_payload, got %q", blocker.CounterpartSource)
	}
	// Layer ordering invariant: layers appear in causal order, never re-sorted by
	// ms. Confirm the target_state layer (if present) precedes direct_blocker,
	// which precedes any background layer.
	layerPos := map[CausalSkeletonLayer]int{}
	for i, n := range skel.Nodes {
		if _, seen := layerPos[n.Layer]; !seen {
			layerPos[n.Layer] = i
		}
	}
	order := []CausalSkeletonLayer{
		CausalSkeletonLayerTargetState, CausalSkeletonLayerDirectBlocker,
		CausalSkeletonLayerUpstreamChain, CausalSkeletonLayerAdjacent,
		CausalSkeletonLayerBackground,
	}
	prev := -1
	for _, layer := range order {
		if pos, ok := layerPos[layer]; ok {
			if pos < prev {
				t.Fatalf("skeleton layers must stay in causal order, %s at %d is before an earlier layer at %d", layer, pos, prev)
			}
			prev = pos
		}
	}
}

func findMonitorContentionRow(t *testing.T, idx *Index, pid int, start, end float64) *CriticalBlockingCandidate {
	t.Helper()
	res := Run(idx, Query{View: "critical_blocking_calls", PID: pid, TimeStart: start, TimeEnd: end})
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			return &res.CriticalBlocking.Items[i]
		}
	}
	t.Fatalf("expected a monitor_contention row: %+v", res.CriticalBlocking)
	return nil
}
