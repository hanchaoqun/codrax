package tracequery

import (
	"strings"
	"testing"
)

// §19 LCK-1 batch pins (real_trace_campaign_20260705.md). Four concerns, all
// verified on the REAL parse / isBlockingLikeText / counterpart-resolution
// paths:
//
//	1. sentinel dead-corner: `owner tid: 0` and the uint64(-1) sentinel are typed
//	   ownerless — no phantom PID, no garbage OwnerTidRaw, wait_object preserved,
//	   and the ownerless row reaches the payload-less wakeup-edge fallback that it
//	   previously fell through (E2a dead corner).
//	2. word-list de-noise: `io` removed and the VSync cadence family no longer
//	   admitted by `sync`; real contention span names still admitted.
//	3. closing-wakeup take-last semantics pinned (span with mid + closing wakeups
//	   resolves the counterpart to the CLOSING wake, not the earlier ones).
//	4. uint64-max overflow sentinel → typed ownerless, never the clamped
//	   9223372036854775807 garbage id.

// ── pin①/④: owner-tid sentinels parse as typed ownerless, wait_object kept ──

func TestParseLockContentionOwnerlessSentinels(t *testing.T) {
	cases := []struct {
		name        string
		payload     string
		wantKind    string
		wantWaitObj string
	}{
		{
			// §19 语料 23/84: ART no-holder sentinel on the suspend count lock.
			name:        "owner tid 0 on thread suspend count lock",
			payload:     "Lock contention on thread suspend count lock (owner tid: 0)",
			wantKind:    "lock_contention",
			wantWaitObj: "thread suspend count lock",
		},
		{
			// §19 语料 7/84 + pin④: uint64(-1) sentinel. A signed Atoi would clamp
			// this 20-digit value to MaxInt64 (9223372036854775807) and print it as
			// a bogus owner; ParseUint recognises it as an explicit ownerless value.
			name:        "owner tid uint64(-1) on InternTable lock",
			payload:     "Lock contention on the InternTable lock (owner tid: 18446744073709551615)",
			wantKind:    "lock_contention",
			wantWaitObj: "the InternTable lock",
		},
		{
			name:        "owner tid 0 on a monitor lock keeps monitor kind",
			payload:     "Lock contention on a monitor lock (owner tid: 0)",
			wantKind:    "monitor_contention",
			wantWaitObj: "a monitor lock",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info, ok := parseLockContentionPayload(tc.payload)
			if !ok {
				t.Fatalf("sentinel payload must still parse as contention: %q", tc.payload)
			}
			if info.Kind != tc.wantKind {
				t.Fatalf("kind = %q, want %q", info.Kind, tc.wantKind)
			}
			if !info.OwnerAbsent {
				t.Fatalf("explicit ownerless sentinel must set OwnerAbsent: %+v", info)
			}
			// The whole point of pin④: the sentinel NEVER becomes a real owner id.
			if info.Owner.PID != 0 {
				t.Fatalf("ownerless sentinel must not produce an owner PID, got %d (garbage-number regression?)", info.Owner.PID)
			}
			// §19 清点②: the lock-object subject is preserved, not discarded.
			if info.WaitObject != tc.wantWaitObj {
				t.Fatalf("wait_object = %q, want %q", info.WaitObject, tc.wantWaitObj)
			}
		})
	}
}

// A real owner tid still parses as a concrete owner (no false ownerless).
func TestParseLockContentionRealOwnerNotSentinel(t *testing.T) {
	info, ok := parseLockContentionPayload("Lock contention on the InternTable lock (owner tid: 512)")
	if !ok || info.OwnerAbsent {
		t.Fatalf("a real owner tid must not be flagged ownerless: %+v ok=%v", info, ok)
	}
	if info.Owner.PID != 512 {
		t.Fatalf("owner PID = %d, want 512", info.Owner.PID)
	}
}

// ── pin①: the ownerless row reaches the wakeup-edge fallback (E2a dead corner
// closed) — the waiter's CLOSING wake becomes the counterpart, no phantom id,
// no garbage OwnerTidRaw, and the lock-object name survives as wait_object. ──

func TestOwnerlessContentionReachesWakeupFallback(t *testing.T) {
	// Waiter aweme-41999 blocks on an ownerless suspend-count-lock contention;
	// releaser-800 issues the CLOSING wakeup of the waiter just before the span
	// ends. Previously this row fell through both resolve branches (BlockingKind
	// set → skips branch 3; Peer.PID==0 → skips branch 1) into the dead corner.
	trace := `
        aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on thread suspend count lock (owner tid: 0)
       releaser-800 (800) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=120 target_cpu=002
        aweme-41999 (41905) [002] .... 5.100000: print: E|41905
	`
	idx := buildTraceIndex(t, "ownerless.systrace", trace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.9, TimeEnd: 5.2})
	if res.CriticalBlocking == nil {
		t.Fatalf("expected a critical_blocking result")
	}
	var row *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "blocking_span" {
			row = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("ownerless contention span must still be carved: %+v", res.CriticalBlocking.Items)
	}
	if row.BlockingKind != "lock_contention" {
		t.Fatalf("typed contention semantics must survive ownerless: kind=%q", row.BlockingKind)
	}
	// §19 清点②: lock-object name preserved.
	if row.WaitObject != "thread suspend count lock" {
		t.Fatalf("wait_object must be the parsed lock-object name, got %q", row.WaitObject)
	}
	// No garbage disclosure: the sentinel is not a real tid.
	if row.OwnerTidRaw != 0 {
		t.Fatalf("ownerless row must carry NO owner_tid_raw (garbage-number regression), got %d", row.OwnerTidRaw)
	}
	// The closing-wakeup fallback recovered the releaser as the counterpart.
	if row.Peer.PID != 800 {
		t.Fatalf("ownerless row should recover the closing waker (releaser-800) as peer, got %+v", row.Peer)
	}
	if row.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("recovered ownerless holder must be stamped wakeup_edge, got %q", row.HolderSource)
	}
	// Disclosure prints no numeric owner id.
	if strings.Contains(row.Summary, "9223372036854775807") {
		t.Fatalf("summary must never print the clamped garbage owner id: %s", row.Summary)
	}
	if !strings.Contains(strings.ToLower(row.Summary), "ownerless") {
		t.Fatalf("ownerless disclosure must be present in summary: %s", row.Summary)
	}
}

// pin④ isolated: the uint64(-1) form never leaks the clamped MaxInt64 garbage id
// anywhere on the resolved row (no in-trace waker → row stays unresolved but
// still carries no garbage number and keeps its wait_object).
func TestOwnerlessUint64MaxNeverLeaksGarbageId(t *testing.T) {
	trace := `
        aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on the InternTable lock (owner tid: 18446744073709551615)
        aweme-41999 (41905) [002] .... 5.100000: print: E|41905
	`
	idx := buildTraceIndex(t, "u64max.systrace", trace)
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.9, TimeEnd: 5.2})
	if res.CriticalBlocking == nil {
		t.Fatalf("expected a critical_blocking result")
	}
	var row *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "blocking_span" {
			row = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("uint64-max ownerless contention span must still be carved: %+v", res.CriticalBlocking.Items)
	}
	if row.Peer.PID == 9223372036854775807 || row.OwnerTidRaw == 9223372036854775807 {
		t.Fatalf("MaxInt64-clamped garbage id leaked onto the row: peer=%+v owner_tid_raw=%d", row.Peer, row.OwnerTidRaw)
	}
	if row.WaitObject != "the InternTable lock" {
		t.Fatalf("wait_object must survive the uint64-max sentinel, got %q", row.WaitObject)
	}
}

// ── pin②: word-list de-noise (§19 F1). `io` removed; VSync cadence family no
// longer admitted by `sync`; real contention span names still admitted. ──

func TestIsBlockingLikeTextDeNoise(t *testing.T) {
	// De-noise: these pure-CPU / cadence spans must NOT screen blocking-like.
	notBlocking := []string{
		// former `io` false positives (Audio DSP + animation + context families)
		"AudioRenderSink",
		"AudioVolume",
		"ValueAnimator",
		"animation#action",
		"TimerIteration",
		"H:application.Context",
		// former `sync` VSync-cadence false positives
		"Choreographer#onVsync",
		"requestNextVsync",
		"jank_event_sync",
	}
	for _, name := range notBlocking {
		if isBlockingLikeText(name) {
			t.Errorf("de-noise: %q must NOT be blocking-like", name)
		}
	}

	// Still admitted: genuine lock/futex/fence contention span names.
	blocking := []string{
		"Lock contention on thread suspend count lock (owner tid: 0)",
		"monitor contention with owner #Foo (512) at Bar",
		"FutexWait",
		"futex_wait_queue",
		"binder transaction",
		"pthread_mutex_lock",
		"blocked on semaphore",
	}
	for _, name := range blocking {
		if !isBlockingLikeText(name) {
			t.Errorf("real blocking span %q must still be admitted", name)
		}
	}
}

// A `sync` name that ALSO carries a real blocking token is still admitted (the
// vsync carve-out is scoped to pure cadence spans, not a blanket vsync ban).
func TestIsBlockingLikeTextVsyncWithRealTokenStillAdmitted(t *testing.T) {
	if !isBlockingLikeText("vsync futex wait") {
		t.Fatalf("a vsync-named span that also contends a futex must still be admitted")
	}
}

// ── pin③: closing-wakeup take-last. A span with an EARLIER (mid) wakeup and a
// LATER (closing) wakeup of the same waiter resolves the counterpart to the
// CLOSING wake — the thread that ended the wait — not the mid one. ──

func TestFindWakeupTakesClosingWake(t *testing.T) {
	// Waiter aweme-41999 is woken twice inside the span window: mid by early-700,
	// then closing by closer-900 right before the span ends. The counterpart must
	// be closer-900.
	trace := `
        aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on thread suspend count lock (owner tid: 0)
          early-700 (700) [003] .... 5.030000: sched_wakeup: comm=aweme pid=41999 prio=120 target_cpu=002
         closer-900 (900) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=120 target_cpu=002
        aweme-41999 (41905) [002] .... 5.100000: print: E|41905
	`
	idx := buildTraceIndex(t, "closing.systrace", trace)

	// Direct pin on the primitive: findWakeupFor overwrites best with each match
	// (no early-exit) → returns the LAST (closing) wakeup in the window.
	ev, _ := findWakeupFor(idx, ThreadRef{Comm: "aweme", PID: 41999}, 5.0, 5.1)
	if ev == nil {
		t.Fatalf("expected a wakeup for the waiter in the window")
	}
	if ev.PID != 900 {
		t.Fatalf("closing-wakeup semantics: counterpart must be the LAST waker (closer-900), got waker pid=%d ts=%.3f", ev.PID, ev.Ts)
	}

	// And end-to-end through the ownerless counterpart resolution.
	res := Run(idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.9, TimeEnd: 5.2})
	var row *CriticalBlockingCandidate
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "blocking_span" {
			row = &res.CriticalBlocking.Items[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("expected the ownerless blocking span row")
	}
	if row.Peer.PID != 900 {
		t.Fatalf("resolved counterpart must be the closing waker (closer-900), got %+v", row.Peer)
	}
}
