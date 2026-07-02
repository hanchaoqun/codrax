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
	trace := `
        aweme-41999 (41905) [002] .... 5.000000: print: B|41905|` + lockContentionCustomerMonitorPayload + `
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
