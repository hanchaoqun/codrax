package tracequery

import (
	"strings"
	"testing"
)

// §7.30.3 D1: deterministic parsing of structured ART/OHOS lock-contention
// print payloads — owner name/tid, waiters, holder site — pinned on the real
// customer payloads (aweme case, lines 45696/45697).

const lockContentionCustomerMonitorPayload = "monitor contention with owner #NetworkKit_GRS_GrsClient-Init_0 -->#NetworkKit_AssetsUtil_Operate_0 (42067) at java.lang.String[] android.content.res.AssetManager.list(java.lang.String)(AssetManager.java:1258) waiters=1 blocking from boolean android.content.res.AssetManager.getResourceValue(int, int, android.util.TypedValue, boolean)(AssetManager.java:761)"

const lockContentionCustomerLockPayload = "Lock contention on a monitor lock (owner tid: 42067)"

func TestParseLockContentionPayloadCustomerMonitorShape(t *testing.T) {
	info, ok := parseLockContentionPayload(lockContentionCustomerMonitorPayload)
	if !ok {
		t.Fatalf("customer monitor contention payload must parse")
	}
	if info.Kind != "monitor_contention" {
		t.Fatalf("kind should be monitor_contention, got %q", info.Kind)
	}
	// The "#A -->#B" hand-off chain names the FINAL holder last.
	if info.Owner.Comm != "NetworkKit_AssetsUtil_Operate_0" {
		t.Fatalf("owner name should be the final holder after -->, got %q", info.Owner.Comm)
	}
	if info.Owner.PID != 42067 {
		t.Fatalf("owner tid should be 42067, got %d", info.Owner.PID)
	}
	if info.Waiters != 1 {
		t.Fatalf("waiters should be 1, got %d", info.Waiters)
	}
	wantSite := "java.lang.String[] android.content.res.AssetManager.list(java.lang.String)(AssetManager.java:1258)"
	if info.HolderSite != wantSite {
		t.Fatalf("holder site mismatch:\n got %q\nwant %q", info.HolderSite, wantSite)
	}
}

// BLOCKFROM (§27.4 G13 配套, 2026-07-09): the "blocking from …" tail — the
// WAITER's own blocking call site — parses verbatim into the typed
// BlockingFromSite next to the holder site, on the opendir specimen's exact
// payload (the G13 witness: prose invented an "enqueueMessage 消息队列锁" wait
// point while the span payload carried this segment).
func TestParseLockContentionPayloadBlockingFromSiteVerbatim(t *testing.T) {
	info, ok := parseLockContentionPayload(lockContentionCustomerMonitorPayload)
	if !ok {
		t.Fatalf("customer monitor contention payload must parse")
	}
	want := "boolean android.content.res.AssetManager.getResourceValue(int, int, android.util.TypedValue, boolean)(AssetManager.java:761)"
	if info.BlockingFromSite != want {
		t.Fatalf("blocking-from site mismatch:\n got %q\nwant %q", info.BlockingFromSite, want)
	}
	// The signature's own parentheses/commas never truncate the value, and the
	// holder site keeps its own boundary (the two segments never bleed).
	if strings.Contains(info.HolderSite, "blocking from") || strings.Contains(info.BlockingFromSite, " at ") {
		t.Fatalf("holder/blocking-from segments must not bleed into each other: holder=%q from=%q", info.HolderSite, info.BlockingFromSite)
	}
	// Summary face carries the typed token next to holder_site.
	if suffix := lockContentionSummarySuffix(info); !strings.Contains(suffix, "blocking_from_site="+want) {
		t.Fatalf("summary suffix must carry blocking_from_site verbatim: %q", suffix)
	}

	// Absence never invents a wait point: a payload without the segment keeps
	// the field empty on both the monitor form and the owner-tid form.
	noFrom, ok := parseLockContentionPayload("monitor contention with owner #Worker (77) at void a.b.C.d()(C.java:5) waiters=2")
	if !ok || noFrom.BlockingFromSite != "" {
		t.Fatalf("payload without a blocking-from segment must keep the field empty: %+v", noFrom)
	}
	if noFrom.HolderSite != "void a.b.C.d()(C.java:5)" {
		t.Fatalf("holder site must stay intact on the no-blocking-from shape: %q", noFrom.HolderSite)
	}
	lockForm, ok := parseLockContentionPayload(lockContentionCustomerLockPayload)
	if !ok || lockForm.BlockingFromSite != "" {
		t.Fatalf("the owner-tid form carries no blocking-from segment: %+v", lockForm)
	}
}

func TestParseLockContentionPayloadOwnerTidShapes(t *testing.T) {
	info, ok := parseLockContentionPayload(lockContentionCustomerLockPayload)
	if !ok || info.Kind != "monitor_contention" {
		t.Fatalf("monitor-lock payload must parse as monitor_contention: %+v ok=%v", info, ok)
	}
	if info.Owner.PID != 42067 || info.Owner.Comm != "" {
		t.Fatalf("owner tid: form should yield tid-only owner, got %+v", info.Owner)
	}

	// Runtime-internal lock forms keep the generic lock_contention kind.
	for _, payload := range []string{
		"Lock contention on the thread list lock (owner tid: 512)",
		"Lock contention on the InternTable lock (owner tid: 512)",
	} {
		info, ok := parseLockContentionPayload(payload)
		if !ok || info.Kind != "lock_contention" {
			t.Fatalf("%q must parse as lock_contention: %+v ok=%v", payload, info, ok)
		}
		if info.Owner.PID != 512 {
			t.Fatalf("%q owner tid should be 512, got %d", payload, info.Owner.PID)
		}
	}

	// Ownerless contention keeps the typed kind with a zero owner (labeled
	// fallback), and non-contention spans do not parse at all.
	info, ok = parseLockContentionPayload("Lock contention on InternTable lock")
	if !ok || info.Kind != "lock_contention" || info.Owner.PID != 0 || info.Owner.Comm != "" {
		t.Fatalf("ownerless contention should keep typed kind with zero owner: %+v ok=%v", info, ok)
	}
	if _, ok := parseLockContentionPayload("Choreographer#doFrame"); ok {
		t.Fatalf("non-contention span must not parse as contention")
	}
}

// The customer's two real print lines, synthesized as B/E spans: the
// critical_blocking view must publish the parsed owner as the row's peer plus
// the typed blocking semantics (§7.30.3 D1 data half).
func TestCriticalBlockingCarriesMonitorContentionOwnerPeer(t *testing.T) {
	// Holder tid 42067 is a genuine in-trace thread (woken on cpu3), so the
	// payload owner is host-namespace-resolvable and the resolution stays
	// payload-direct (P0-E2a byte-stability witness).
	trace := `
        aweme-41999 (41905) [002] .... 5.000000: print: B|41905|` + lockContentionCustomerMonitorPayload + `
          other-777 (777) [003] .... 5.010000: sched_wakeup: comm=NetworkKit_AssetsUtil_Operate_0 pid=42067 prio=120 target_cpu=003
        aweme-41999 (41905) [002] .... 5.112223: print: E|41905
        aweme-41999 (41905) [002] .... 5.200000: print: B|41905|` + lockContentionCustomerLockPayload + `
        aweme-41999 (41905) [002] .... 5.260000: print: E|41905
	`
	idx := buildTraceIndex(t, "contention.systrace", trace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.9, TimeEnd: 5.3})
	if res.CriticalBlocking == nil || len(res.CriticalBlocking.Items) == 0 {
		t.Fatalf("expected critical blocking candidates: %+v", res.CriticalBlocking)
	}
	var monitor, lock *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		item := &res.CriticalBlocking.Items[i]
		if item.Type != "blocking_span" {
			continue
		}
		switch item.BlockingKind {
		case "monitor_contention":
			if item.HolderSite != "" {
				monitor = item
			} else {
				lock = item
			}
		}
	}
	if monitor == nil {
		t.Fatalf("monitor contention candidate missing: %+v", res.CriticalBlocking.Items)
	}
	if monitor.Peer.Comm != "NetworkKit_AssetsUtil_Operate_0" || monitor.Peer.PID != 42067 {
		t.Fatalf("monitor contention peer must be the parsed owner, got %+v", monitor.Peer)
	}
	if monitor.Waiters != 1 || !strings.Contains(monitor.HolderSite, "AssetManager.java:1258") {
		t.Fatalf("monitor contention typed fields missing: %+v", monitor)
	}
	for _, want := range []string{"blocking_kind=monitor_contention", "owner=NetworkKit_AssetsUtil_Operate_0-42067", "holder_site=", "waiters=1"} {
		if !strings.Contains(monitor.Summary, want) {
			t.Fatalf("monitor contention summary missing %q: %s", want, monitor.Summary)
		}
	}
	if lock == nil {
		t.Fatalf("owner-tid contention candidate missing: %+v", res.CriticalBlocking.Items)
	}
	if lock.Peer.PID != 42067 || lock.Peer.Comm != "" {
		t.Fatalf("owner-tid contention peer must carry the tid, got %+v", lock.Peer)
	}
	if !strings.Contains(lock.Summary, "owner=pid=42067") {
		t.Fatalf("owner-tid contention summary must carry the owner id: %s", lock.Summary)
	}
}

// BLOCKFROM (§27.4 G13, 2026-07-09) e2e: the parsed waiter-side call site
// travels verbatim onto BOTH engine faces minted from the same folded lane —
// the critical_blocking candidate and the blocking_span rank row — exactly
// like holder_site (same mint funnel, no second parse).
func TestBlockingFromSiteRidesCriticalBlockingAndRankRows(t *testing.T) {
	trace := `
        app-100 (100) [001] .... 5.000000: print: B|100|` + lockContentionCustomerMonitorPayload + `
        app-100 (100) [001] .... 5.000100: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (100) [002] .... 5.112000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        app-100 (100) [001] .... 5.112223: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.112300: print: E|100
 NetworkKit_AssetsUtil_Operate_0-42067 (600) [003] .... 5.200000: sched_switch: prev_comm=NetworkKit_AssetsUtil_Operate_0 prev_pid=42067 prev_prio=120 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
`
	wantFrom := "boolean android.content.res.AssetManager.getResourceValue(int, int, android.util.TypedValue, boolean)(AssetManager.java:761)"
	idx := buildTraceIndex(t, "blocking_from.systrace", trace)

	res := Run(idx, Query{View: "critical_blocking_calls", PID: 100, TimeStart: 4.99, TimeEnd: 5.115})
	var monitor *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].BlockingKind == "monitor_contention" {
			monitor = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if monitor == nil {
		t.Fatalf("expected the monitor contention blocking row: %+v", res.CriticalBlocking)
	}
	if monitor.BlockingFromSite != wantFrom {
		t.Fatalf("critical_blocking must carry the waiter-side call site verbatim:\n got %q\nwant %q", monitor.BlockingFromSite, wantFrom)
	}

	rank := BuildRootCauseRank(idx, Query{PID: 100, TimeStart: 4.99, TimeEnd: 5.115, MaxDepth: 4, MinDurationMs: 0.05, Limit: 16})
	var lockRow *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "blocking_span" && rank.Items[i].BlockingKind == "monitor_contention" {
			lockRow = &rank.Items[i]
			break
		}
	}
	if lockRow == nil {
		t.Fatalf("expected the blocking_span rank row: %+v", rank.Items)
	}
	if lockRow.BlockingFromSite != wantFrom {
		t.Fatalf("rank row must carry the waiter-side call site verbatim:\n got %q\nwant %q", lockRow.BlockingFromSite, wantFrom)
	}
	if !strings.Contains(lockRow.Summary, "blocking_from_site="+wantFrom) {
		t.Fatalf("rank summary must name the waiter-side call site: %s", lockRow.Summary)
	}
	// The holder-side field is untouched next to it (two sites, two lanes).
	if !strings.Contains(lockRow.HolderSite, "AssetManager.java:1258") {
		t.Fatalf("holder site must survive unchanged next to blocking_from_site: %+v", lockRow)
	}
}
