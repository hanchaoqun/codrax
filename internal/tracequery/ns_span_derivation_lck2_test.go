package tracequery

import (
	"strings"
	"testing"
)

// LCK-2 batch pins (real_trace_campaign_20260705.md §18.E / §18.E 增补 /
// §18.E.1). The lock-holder resolution ladder gains rung ② (ns-span
// derivation) between the payload-direct rung ① and the closing-wakeup rung
// ③, plus the ②×③ identity-unification declaration:
//
//	pin1  ②a self-reported ns-tid → thread-level holder, source
//	      ns_span_derivation, confidence 0.67, OwnerTidRaw kept.
//	pin2  ②b main-thread special case (owner ns-tid == ns-tgid) →
//	      host tid == host tgid, thread-level.
//	pin3  ②c thread mapping unavailable → PROCESS level: explicit downgrade
//	      disclosure, host tgid NEVER enters Peer.PID, holder_host_process
//	      display value carries the identity.
//	pin4  ② unavailable (ambiguous process map) → rung ③ fires in order
//	      (wakeup_edge, 0.62).
//	pin5  ① control: a payload owner tid present in this trace stays
//	      payload-direct byte-for-byte (0.72, no raw-tid audit, no ns notes).
//	pin6  ②×③ identity unification: rung ② and the closing waker INDEPENDENTLY
//	      name the same host thread → typed declaration listing both lanes,
//	      fusion confidence 0.70.
//	pin7  comm mismatch NEVER vetoes: payload owner comm ≠ derived host comm →
//	      resolution kept, soft "renamed" disclosure present.
//	pin8  ②×③ divergence: a closing waker that is NOT the derived thread never
//	      mints a unification declaration; the divergence is disclosed.
//	pin9  process-level + closing waker INSIDE the derived process → the
//	      §19 dual-disclosure shape (Peer=waker via wakeup_edge lifted to
//	      0.67, process identity on the display note).
//	pin10 the ②a extractor is fail-closed: contention payloads and compound
//	      "<word> tid:" keys are never harvested as self-reports.

// blockingSpanRowFor extracts the (single) blocking_span row of a
// critical_blocking run.
func blockingSpanRowFor(t *testing.T, idx *Index, q Query) *CriticalBlockingCandidate {
	t.Helper()
	res := Run(idx, q)
	if res.CriticalBlocking == nil {
		t.Fatalf("expected a critical_blocking result")
	}
	for i := range res.CriticalBlocking.Items {
		if res.CriticalBlocking.Items[i].Type == "blocking_span" {
			return &res.CriticalBlocking.Items[i]
		}
	}
	t.Fatalf("expected a blocking_span row: %+v", res.CriticalBlocking.Items)
	return nil
}

// ── pin1 + pin7: ②a self-reported ns-tid, thread level, comm never vetoes ──

// The waiter aweme-41999 (host tgid 41905) emits its contention span under
// container ns pid 43000; the owner tid 62020 is a container thread id absent
// from the host trace. nsworker-42500 (same host process) earlier
// self-reported `tid: 62020` on its own B|43000| span — the ②a emission-pair
// sample mapping (43000, 62020) → host tid 42500. No wakeup rows: the pure
// rung-② form.
const lck2SelfReportTrace = `
       nsworker-42500 (41905) [003] .... 4.900000: print: B|43000|H:asset queue take, tid: 62020
       nsworker-42500 (41905) [003] .... 4.900100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|monitor contention with owner #NetworkKit_Assets (62020) at AssetManager.list(AssetManager.java:1258) waiters=1
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanSelfReportedTidResolvesThreadLevel(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a1.systrace", lck2SelfReportTrace)
	if idx.tidPresent(62020) {
		t.Fatalf("precondition: owner tid 62020 must not be host-resolvable")
	}
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceNsSpanDerivation {
		t.Fatalf("rung ② must resolve this holder, got source=%q (peer=%+v)", row.HolderSource, row.Peer)
	}
	if row.Peer.PID != 42500 {
		t.Fatalf("②a must map ns-tid 62020 to host thread 42500, got %+v", row.Peer)
	}
	if row.OwnerTidRaw != 62020 {
		t.Fatalf("the raw container owner tid must be kept for audit, got %d", row.OwnerTidRaw)
	}
	if diff := row.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("ns-span confidence must be %.2f, got %.3f", counterpartNsSpanDerivationConfidence, row.Confidence)
	}
	if row.HolderNsUnification != "" {
		t.Fatalf("no wakeup edge in this trace — unification must NOT be declared: %q", row.HolderNsUnification)
	}
	if !strings.Contains(row.Summary, "ns-span") || !strings.Contains(row.Summary, nsSpanViaSelfReportedTid) {
		t.Fatalf("thread-level ns-span disclosure missing: %s", row.Summary)
	}
	// pin7 — the payload owner comm (NetworkKit_Assets) differs from the
	// derived host comm (nsworker): the derivation is KEPT and the mismatch is
	// a soft disclosure only (comm can be renamed at runtime).
	if !strings.Contains(row.Summary, "does NOT veto") || !strings.Contains(row.Summary, "renamed") {
		t.Fatalf("comm-mismatch soft disclosure missing (or worded as a veto): %s", row.Summary)
	}
}

// ── pin2: ②b main-thread special case ──

// Owner ns-tid equals the container ns pid (43000) ⇒ the owner is that
// process's MAIN thread ⇒ host tid == host tgid (41905). The main thread is
// present in the trace via its own registration marks.
const lck2MainThreadTrace = `
          aweme-41905 (41905) [000] .... 4.800000: print: B|43000|H:main init
          aweme-41905 (41905) [000] .... 4.800100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on the InternTable lock (owner tid: 43000)
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanMainThreadSpecialCase(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a2.systrace", lck2MainThreadTrace)
	if idx.tidPresent(43000) {
		t.Fatalf("precondition: container ns pid 43000 must not be a host tid here")
	}
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceNsSpanDerivation {
		t.Fatalf("②b must resolve the main-thread owner, got source=%q (peer=%+v)", row.HolderSource, row.Peer)
	}
	if row.Peer.PID != 41905 || row.Peer.TGID != 41905 {
		t.Fatalf("②b: owner ns-tid==ns-tgid must map to host main thread (tid==tgid==41905), got %+v", row.Peer)
	}
	if !strings.Contains(row.Summary, nsSpanViaMainThread) {
		t.Fatalf("②b via-path disclosure missing: %s", row.Summary)
	}
	if diff := row.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("ns-span confidence must be %.2f, got %.3f", counterpartNsSpanDerivationConfidence, row.Confidence)
	}
}

// ── pin3: process-level downgrade — explicit disclosure, tgid never a PID ──

// Owner ns-tid 55555 has no self-report sample and is not the ns main thread;
// no wakeup edge exists either. Rung ② can only narrow to the host PROCESS:
// the peer must stay UNRESOLVED (tgid is never stuffed into Peer.PID) and the
// process identity rides the holder_host_process display value with an
// explicit process-level downgrade disclosure.
const lck2ProcessLevelTrace = `
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 55555)
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanProcessLevelDowngradeDisclosed(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a3.systrace", lck2ProcessLevelTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.Peer.PID != 0 {
		t.Fatalf("process-level rung ② must keep the peer UNRESOLVED (tgid never enters Peer.PID), got %+v", row.Peer)
	}
	if row.HolderHostProcess == "" {
		t.Fatalf("process-level identity must ride the holder_host_process display value")
	}
	if !strings.Contains(row.HolderHostProcess, "tgid=41905") || !strings.Contains(row.HolderHostProcess, "ns_pid=43000") || !strings.Contains(row.HolderHostProcess, "level=process") {
		t.Fatalf("holder_host_process value malformed: %q", row.HolderHostProcess)
	}
	if !strings.Contains(row.Summary, "PROCESS-LEVEL") {
		t.Fatalf("explicit process-level downgrade disclosure missing: %s", row.Summary)
	}
	if row.OwnerTidRaw != 55555 {
		t.Fatalf("raw container owner tid must be kept for audit, got %d", row.OwnerTidRaw)
	}
}

// ── pin4: ② unavailable → rung ③ fires in ladder order ──

// The SAME container ns pid 43000 is emitted by TWO different host processes
// (tgid 41905 and tgid 90000) — the emission-pair process map is structurally
// AMBIGUOUS, rung ② hard-rejects (comm never disambiguates), and the ladder
// falls to the closing-wakeup rung ③ exactly as before LCK-2 (wakeup_edge,
// 0.62 ceiling).
const lck2AmbiguousProcessTrace = `
          other-90001 (90000) [005] .... 4.700000: print: B|43000|H:other proc span
          other-90001 (90000) [005] .... 4.700100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 55555)
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       releaser-800 (800) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.090100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=aweme next_pid=41999 next_prio=110
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanAmbiguousProcessMapFallsBackToWakeupEdge(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a4.systrace", lck2AmbiguousProcessTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("ambiguous ns process map must hard-reject rung ② and fall to rung ③, got source=%q", row.HolderSource)
	}
	if row.Peer.PID != 800 {
		t.Fatalf("rung ③ must recover the closing waker, got %+v", row.Peer)
	}
	if row.HolderHostProcess != "" || row.HolderNsUnification != "" {
		t.Fatalf("a rejected rung ② must publish NO ns-span identity notes: process=%q unification=%q", row.HolderHostProcess, row.HolderNsUnification)
	}
	if row.Confidence > counterpartWakeupEdgeConfidence+1e-9 {
		t.Fatalf("rung ③ confidence ceiling is %.2f, got %.3f", counterpartWakeupEdgeConfidence, row.Confidence)
	}
}

// ── pin5: rung ① control — payload-direct resolution is untouched ──

// The owner tid IS a host thread in this trace: rung ① resolves it directly
// and the LCK-2 machinery must not leave a single mark on the row, even
// though the contention span is ns-divergent and mapping material exists.
const lck2PayloadDirectTrace = `
       nsworker-42500 (41905) [003] .... 4.900000: print: B|43000|H:asset queue take, tid: 62020
       nsworker-42500 (41905) [003] .... 4.900100: print: E|43000
         holder-51000 (41905) [004] .... 4.950000: sched_switch: prev_comm=holder prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=other next_pid=51001 next_prio=120
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 51000)
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanNeverFiresWhenPayloadDirectResolves(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a5.systrace", lck2PayloadDirectTrace)
	if !idx.tidPresent(51000) {
		t.Fatalf("precondition: owner tid 51000 must be host-resolvable")
	}
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceContentionPayload {
		t.Fatalf("rung ① must stay payload-direct, got source=%q", row.HolderSource)
	}
	if row.Peer.PID != 51000 {
		t.Fatalf("payload-direct owner must be kept verbatim, got %+v", row.Peer)
	}
	if row.OwnerTidRaw != 0 || row.HolderHostProcess != "" || row.HolderNsUnification != "" {
		t.Fatalf("payload-direct rows must carry NO LCK-2 artifacts: raw=%d process=%q unification=%q", row.OwnerTidRaw, row.HolderHostProcess, row.HolderNsUnification)
	}
	if row.Confidence < 0.72-1e-9 {
		t.Fatalf("payload-direct confidence must stay 0.72, got %.3f", row.Confidence)
	}
}

// ── pin6: ②×③ identity unification ──

// Rung ② maps owner ns-tid 62020 to host thread nsworker-42500; the CLOSING
// wakeup of the waiter is issued by that same host thread. Two independent
// lanes name one physical thread → the typed unification declaration is
// published (both lanes listed) and confidence rides the 0.70 fusion ceiling.
// The rank row carries the declaration verbatim.
const lck2UnificationTrace = `
       nsworker-42500 (41905) [003] .... 4.900000: print: B|43000|H:asset queue take, tid: 62020
       nsworker-42500 (41905) [003] .... 4.900100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|monitor contention with owner #NetworkKit_Assets (62020) at AssetManager.list(AssetManager.java:1258) waiters=1
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       nsworker-42500 (41905) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.090100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=aweme next_pid=41999 next_prio=110
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanWakeupIdentityUnification(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a6.systrace", lck2UnificationTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceNsSpanDerivation {
		t.Fatalf("the deriving lane stays ns_span_derivation under unification, got %q", row.HolderSource)
	}
	if row.Peer.PID != 42500 {
		t.Fatalf("unified holder must be the derived host thread, got %+v", row.Peer)
	}
	if row.HolderNsUnification == "" {
		t.Fatalf("②×③ agreement must publish the typed unification declaration")
	}
	for _, want := range []string{"owner_ns_tid=62020", CounterpartSourceNsSpanDerivation, CounterpartSourceWakeupEdge} {
		if !strings.Contains(row.HolderNsUnification, want) {
			t.Fatalf("unification declaration must carry %q: %q", want, row.HolderNsUnification)
		}
	}
	if diff := row.Confidence - counterpartNsSpanFusionConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("fusion confidence must be %.2f, got %.3f", counterpartNsSpanFusionConfidence, row.Confidence)
	}
	if !strings.Contains(row.Summary, "identity unification") {
		t.Fatalf("unification prose disclosure missing: %s", row.Summary)
	}

	// The rank row ports the declaration verbatim (§18.E.1 display half rides
	// the same typed value on both faces).
	rank := BuildRootCauseRank(idx, Query{PID: 41905, TimeStart: 4.99, TimeEnd: 5.2, MaxDepth: 4, MinDurationMs: 0.01, Limit: 16})
	var lockRow *RootCauseRankItem
	for i := range rank.Items {
		if rank.Items[i].Type == "blocking_span" && rank.Items[i].HolderSource == CounterpartSourceNsSpanDerivation {
			lockRow = &rank.Items[i]
			break
		}
	}
	if lockRow == nil {
		t.Fatalf("expected a ns-span lock rank row: %+v", rank.Items)
	}
	if lockRow.HolderNsUnification != row.HolderNsUnification {
		t.Fatalf("rank row must port the unification declaration verbatim: %q vs %q", lockRow.HolderNsUnification, row.HolderNsUnification)
	}
}

// ── pin8: ②×③ divergence never mints a unification declaration ──

// Rung ② derives nsworker-42500, but the closing waker is a DIFFERENT thread
// of the same host process (mediator-42900): no unification — the divergence
// is disclosed and the ns-span identity is kept at the single-lane 0.67.
const lck2DivergenceTrace = `
       nsworker-42500 (41905) [003] .... 4.900000: print: B|43000|H:asset queue take, tid: 62020
       nsworker-42500 (41905) [003] .... 4.900100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 62020)
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       mediator-42900 (41905) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanWakeupDivergenceDisclosedNotUnified(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a7.systrace", lck2DivergenceTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceNsSpanDerivation || row.Peer.PID != 42500 {
		t.Fatalf("the ns-span identity must be kept under divergence, got source=%q peer=%+v", row.HolderSource, row.Peer)
	}
	if row.HolderNsUnification != "" {
		t.Fatalf("a divergent waker must NEVER mint a unification declaration: %q", row.HolderNsUnification)
	}
	if !strings.Contains(row.Summary, "intermediary wakeup") {
		t.Fatalf("divergence disclosure missing: %s", row.Summary)
	}
	if diff := row.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("divergence keeps the single-lane 0.67, got %.3f", row.Confidence)
	}
}

// ── pin9: process-level ② × thread-level ③ corroboration (the §19 42067 shape) ──

// Rung ② narrows only to the host process; the closing waker BELONGS to that
// derived process — the two lanes corroborate: the waker is published as the
// peer (wakeup_edge lane), lifted to the 0.67 ns-span grade, and the
// process-level identity rides the display note beside it (并列双披露).
const lck2CorroborationTrace = `
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 55555)
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       releaser-42600 (41905) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanProcessLevelCorroboratedByClosingWaker(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a8.systrace", lck2CorroborationTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceWakeupEdge || row.Peer.PID != 42600 {
		t.Fatalf("the thread identity comes from rung ③ here, got source=%q peer=%+v", row.HolderSource, row.Peer)
	}
	if row.HolderHostProcess == "" || !strings.Contains(row.HolderHostProcess, "tgid=41905") {
		t.Fatalf("the process-level ns-span identity must ride beside the waker: %q", row.HolderHostProcess)
	}
	if !strings.Contains(row.Summary, "corroborate") {
		t.Fatalf("②×③ corroboration disclosure missing: %s", row.Summary)
	}
	if diff := row.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("corroborated waker rides the 0.67 ns-span grade, got %.3f", row.Confidence)
	}
	if row.HolderNsUnification != "" {
		t.Fatalf("process-membership corroboration is NOT identity unification: %q", row.HolderNsUnification)
	}
}

// ── pin10: the ②a extractor is fail-closed ──

func TestNsSpanSelfReportedTidExtractorFailClosed(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
		ok   bool
	}{
		{"standalone comma field", "H:asset queue take, tid: 62020", 62020, true},
		{"pipe delimiter", "work|tid=77", 77, true},
		{"leading key", "tid: 5, phase 2", 5, true},
		{"colon delimiter", "H:tid:42067", 42067, true},
		{"same value twice", "queue, tid: 9; retry, tid: 9", 9, true},
		// Other-reports and compounds — never harvested.
		{"lock contention payload excluded wholesale", "Lock contention on thread suspend count lock (owner tid: 62020)", 0, false},
		{"monitor form excluded wholesale", "monitor contention with owner #X (555) at Y.z(Y.java:1) waiters=2", 0, false},
		{"compound word key", "send to tid: 99", 0, false},
		{"glued word key", "vtid: 5", 0, false},
		// Ambiguity and non-pid shapes — fail closed.
		{"two different self-claims", "a, tid: 3, b, tid: 4", 0, false},
		{"digit run too long for a pid", "tid: 123456789", 0, false},
		{"value glued to a word", "tid: 12ms", 0, false},
		{"zero is not a tid", "tid: 0", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := nsSpanSelfReportedTid(tc.in)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("nsSpanSelfReportedTid(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// ── structural-uniqueness hard reject on the ②a THREAD map ──

// Two different host tids self-claim the same (ns pid, ns tid): the thread
// entry is ambiguous, ②a refuses (comm never disambiguates), and — the ns
// process map still being unique — the resolution degrades to the
// process-level form instead of picking either claimant.
const lck2AmbiguousThreadTrace = `
       claimant1-42500 (41905) [003] .... 4.900000: print: B|43000|H:queue take, tid: 62020
       claimant1-42500 (41905) [003] .... 4.900100: print: E|43000
       claimant2-42700 (41905) [004] .... 4.910000: print: B|43000|H:queue take, tid: 62020
       claimant2-42700 (41905) [004] .... 4.910100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 62020)
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestNsSpanAmbiguousThreadMapDegradesToProcessLevel(t *testing.T) {
	idx := buildTraceIndex(t, "lck2_a9.systrace", lck2AmbiguousThreadTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.Peer.PID == 42500 || row.Peer.PID == 42700 {
		t.Fatalf("an ambiguous self-report entry must never elect a claimant, got %+v", row.Peer)
	}
	if row.HolderHostProcess == "" || !strings.Contains(row.Summary, "PROCESS-LEVEL") {
		t.Fatalf("ambiguous thread map must degrade to the disclosed process-level form: process=%q summary=%s", row.HolderHostProcess, row.Summary)
	}
}

// ── presumption inheritance rides the set form ──

func TestCounterpartSourceIsInferredSetMembership(t *testing.T) {
	if !counterpartSourceIsInferred(CounterpartSourceWakeupEdge) || !counterpartSourceIsInferred(CounterpartSourceNsSpanDerivation) {
		t.Fatalf("both inference lanes must read inferred")
	}
	if counterpartSourceIsInferred(CounterpartSourceContentionPayload) || counterpartSourceIsInferred("") {
		t.Fatalf("payload-direct / unresolved must never read inferred")
	}
}
