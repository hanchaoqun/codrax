package tracequery

// LOCKNS-FIX batch pins (real_trace_campaign_20260705.md §29.104.10–.12 +
// §29.104.12.1 user ruling, 2026-07-16) — customer lock-holder tid
// identification generalized for containerized (pid-namespace) traces.
//
//	件1  G1 ns-divergence gate: on an ns-divergent contention row
//	     (SpanPID > 0 ∧ waiter host TGID > 0 ∧ SpanPID ≠ TGID) rung ①
//	     payload-direct resolution is SKIPPED — the payload owner tid is a
//	     container-namespace id by construction, so a same-numbered host
//	     thread is a numeric collision (裁定原文:「直解对 ns-divergent 行
//	     永远语义错误,撞对只是巧合」). Diverted rows enter the derivation
//	     ladder ②/③ and resolve correctly or stay honestly unresolved.
//	     Identity-ns rows keep rung ① byte-identically (pin5,
//	     ns_span_derivation_lck2_test.go); integer-less rows fail open (pin④
//	     here).
//	件2  形A vendor-prefix word-boundary locate (registry row
//	     monitor_contention_owner_embedded); "premonitor contention …" never
//	     matches.
//	件3  morphology registry migration (byte-equivalent dispatch) + unknown
//	     owner-vocabulary forms fail open to the payload-less lane with the
//	     「owner 未解析(形态未注册)」 disclosure.
//	件4  ownerless sentinel rows with NO closing wakeup edge disclose
//	     「持有者未解析」 on every branch (previously fb.OK-only); owned rows
//	     mint zero such disclosure.
//	件5  unresolved rows keep Confidence 0.72 — pinned as the SPAN-TYPING
//	     grade (the same 0.72 every blocking-like carve mints), NOT a holder
//	     grade: the holder claim is carried by Peer/HolderSource absence and
//	     the §12.3 未解析不准入 typed-pair gates. 现状裁定=正确,钉现状.
//	件6  ②×③ HolderNsUnification display consumption — pinned on the tool
//	     side (answer_document_projection_lockns_test.go).

import (
	"context"
	"strings"
	"testing"
)

// ── 件1 · collision forms (synthetic, 撞号形) ──────────────────────────────
//
// The contention span is emitted under container ns pid 43000 by host process
// 41905; the payload owner tid 51000 NUMERICALLY MATCHES an unrelated host
// thread (victim-51000 of process 50000). The 形B tid-only print carries no
// owner comm, so the pre-G1 rung ① resolved it payload-direct at 0.72 with
// zero disclosure (修前病形); post-G1 the row must enter the derivation
// ladder instead (修后=正确宿主线程或诚实 unresolved) — asserted per rung
// below.

// 件1-a: rung ②a material exists → the derived host thread wins, never the
// colliding host tid.
const locknsCollisionSelfReportTrace = `
       victim-51000 (50000) [005] .... 4.800000: sched_switch: prev_comm=victim prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=idle/5 next_pid=0 next_prio=120
       nsworker-42500 (41905) [003] .... 4.900000: print: B|43000|H:asset queue take, tid: 51000
       nsworker-42500 (41905) [003] .... 4.900100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 51000)
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestLocknsGateCollisionResolvesViaNsSpanDerivation(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_g1_a.systrace", locknsCollisionSelfReportTrace)
	if !idx.tidPresent(51000) {
		t.Fatalf("precondition: the colliding host tid 51000 must be present (the collision shape)")
	}
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	// 修前梯①形: HolderSource=contention_payload, Peer.PID=51000 (victim,
	// wrong process), conf 0.72, zero disclosure. 修后:
	if row.HolderSource != CounterpartSourceNsSpanDerivation {
		t.Fatalf("G1 gate must divert the collision row into rung ②, got source=%q peer=%+v", row.HolderSource, row.Peer)
	}
	if row.Peer.PID != 42500 || row.Peer.PID == 51000 {
		t.Fatalf("the derived host thread must win, never the colliding host tid: %+v", row.Peer)
	}
	if row.OwnerTidRaw != 51000 {
		t.Fatalf("the container owner tid must be kept for audit, got %d", row.OwnerTidRaw)
	}
	if diff := row.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("derivation-ladder confidence must be %.2f (never the 0.72 direct grade), got %.3f", counterpartNsSpanDerivationConfidence, row.Confidence)
	}
	if !strings.Contains(row.Summary, "numeric collision") {
		t.Fatalf("collision disclosure missing: %s", row.Summary)
	}
	if strings.Contains(row.Summary, "not present in this trace") {
		t.Fatalf("the collision shape must not claim the tid is absent (it IS present, as an unrelated thread): %s", row.Summary)
	}
}

// 件1-b: rung ② unavailable (ambiguous ns process map) + closing wakeup edge
// exists → rung ③ names the waker with the collision-aware caveat.
const locknsCollisionWakeupTrace = `
       victim-51000 (50000) [005] .... 4.800000: sched_switch: prev_comm=victim prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=idle/5 next_pid=0 next_prio=120
          other-90001 (90000) [005] .... 4.700000: print: B|43000|H:other proc span
          other-90001 (90000) [005] .... 4.700100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 51000)
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       releaser-800 (800) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.090100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=aweme next_pid=41999 next_prio=110
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestLocknsGateCollisionFallsToClosingWakeup(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_g1_b.systrace", locknsCollisionWakeupTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceWakeupEdge || row.Peer.PID != 800 {
		t.Fatalf("collision row with an ambiguous ns map must fall to rung ③, got source=%q peer=%+v", row.HolderSource, row.Peer)
	}
	if row.OwnerTidRaw != 51000 {
		t.Fatalf("raw container owner tid must be kept, got %d", row.OwnerTidRaw)
	}
	if row.Confidence > counterpartWakeupEdgeConfidence+1e-9 {
		t.Fatalf("rung ③ ceiling is %.2f, got %.3f", counterpartWakeupEdgeConfidence, row.Confidence)
	}
	if !strings.Contains(row.Summary, "numeric collision") || !strings.Contains(row.Summary, "thread that woke the waiter") {
		t.Fatalf("collision-aware rung-③ caveat missing: %s", row.Summary)
	}
	if strings.Contains(row.Summary, "not present in this trace") {
		t.Fatalf("the collision shape must not claim absence: %s", row.Summary)
	}
}

// 件1-c + 件5: no derivation material, no wakeup edge → honestly unresolved
// with the collision disclosure; Confidence stays the 0.72 SPAN-TYPING grade
// (件5 现状裁定=正确: the same 0.72 every blocking-like carve mints — the
// holder claim is carried by the EMPTY Peer/HolderSource, which the §12.3
// typed-pair gates read as 未解析不准入, never by the confidence scalar).
const locknsCollisionUnresolvedTrace = `
       victim-51000 (50000) [005] .... 4.800000: sched_switch: prev_comm=victim prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=idle/5 next_pid=0 next_prio=120
          other-90001 (90000) [005] .... 4.700000: print: B|43000|H:other proc span
          other-90001 (90000) [005] .... 4.700100: print: E|43000
          aweme-41999 (41905) [002] .... 5.000000: print: B|43000|Lock contention on thread suspend count lock (owner tid: 51000)
          aweme-41999 (41905) [002] .... 5.100000: print: E|43000
`

func TestLocknsGateCollisionUnresolvedStaysHonest(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_g1_c.systrace", locknsCollisionUnresolvedTrace)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.Peer.PID != 0 || strings.TrimSpace(row.Peer.Comm) != "" || row.HolderSource != "" {
		t.Fatalf("no ladder material: the row must stay UNRESOLVED (never the colliding host tid), got source=%q peer=%+v", row.HolderSource, row.Peer)
	}
	if row.OwnerTidRaw != 51000 {
		t.Fatalf("raw container owner tid must be kept for audit, got %d", row.OwnerTidRaw)
	}
	if !strings.Contains(row.Summary, "holder unresolved") || !strings.Contains(row.Summary, "numeric collision") {
		t.Fatalf("honest-unresolved collision disclosure missing: %s", row.Summary)
	}
	// 件5 pin (现状钉住): 0.72 is the span-typing grade, not a holder grade.
	if diff := row.Confidence - 0.72; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("unresolved row keeps the 0.72 span-typing grade, got %.3f", row.Confidence)
	}
}

// ── 件1 pin④ · fail-open when either gate integer is unavailable ───────────

func TestLocknsGateFailsOpenWithoutGateIntegers(t *testing.T) {
	// The owner tid IS host-present; the spans are hand-built so the gate
	// integers can be zeroed independently (the WindowStats direct-feed lane,
	// lock_lane_p0e_test.go precedent).
	idx := buildTraceIndex(t, "lockns_g1_d.systrace", `
        Worker-42067 (16547) [003] .... 100.050000: sched_switch: prev_comm=Worker prev_pid=42067 prev_prio=120 prev_state=R ==> next_comm=idle/3 next_pid=0 next_prio=120
	`)
	shapes := []struct {
		name    string
		spanPID int
		tgid    int
	}{
		{"span_pid_unavailable", 0, 16547},
		{"waiter_tgid_unavailable", 43000, 0},
	}
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			stats := WindowStats{TraceSpans: []TraceSpanSummary{{
				Name:    "Lock contention on thread suspend count lock (owner tid: 42067)",
				SpanPID: tc.spanPID,
				Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: tc.tgid},
				StartTs: 100.000, EndTs: 100.100, DurationMs: 100,
				StartLine: 10, EndLine: 20,
			}}}
			rows := collectBlockingSpanRows(idx, Query{}, stats)
			if len(rows) != 1 {
				t.Fatalf("expected one row, got %d", len(rows))
			}
			cand := rows[0].cand
			if cand.HolderSource != CounterpartSourceContentionPayload || cand.Peer.PID != 42067 {
				t.Fatalf("a gate integer is unavailable — rung ① must be preserved (fail-open), got source=%q peer=%+v", cand.HolderSource, cand.Peer)
			}
			if cand.OwnerTidRaw != 0 || strings.Contains(cand.Summary, "collision") {
				t.Fatalf("fail-open rows must carry no gate artifacts: raw=%d summary=%s", cand.OwnerTidRaw, cand.Summary)
			}
		})
	}
}

// ── 件2 · 形A vendor-prefix word-boundary locate ───────────────────────────

func TestLockContentionVendorPrefixMonitorFormParses(t *testing.T) {
	body := "monitor contention with owner #Worker (77) at void a.b.C.d()(C.java:5) waiters=2 blocking from void e.f.G.h()(G.java:9)"
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"colon-space vendor prefix", "vendor_xyz: " + body},
		{"pipe delimiter", "vend|" + body},
		{"bracket tag", "[art] " + body},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info, ok := parseLockContentionPayload(tc.in)
			if !ok {
				t.Fatalf("vendor-prefixed monitor form must parse: %q", tc.in)
			}
			if info.Morphology != "monitor_contention_owner_embedded" {
				t.Fatalf("embedded registry row must own the vendor form, got %q", info.Morphology)
			}
			if info.Kind != blockingKindMonitorContention || info.Owner.Comm != "Worker" || info.Owner.PID != 77 {
				t.Fatalf("owner fields lost through the vendor prefix: %+v", info)
			}
			if info.HolderSite != "void a.b.C.d()(C.java:5)" || info.Waiters != 2 {
				t.Fatalf("holder site / waiters lost: %+v", info)
			}
			if info.BlockingFromSite != "void e.f.G.h()(G.java:9)" {
				t.Fatalf("blocking-from lost: %+v", info)
			}
		})
	}

	// 词首边界负向: a WORD glued onto the grammar is never a hit (自由子串
	// 误伤禁止).
	for _, in := range []string{
		"premonitor contention with owner #W (5) at a()(b:1)",
		"Xmonitor contention with owner #W (5)",
	} {
		if _, ok := parseLockContentionPayload(in); ok {
			t.Fatalf("glued word must not match the monitor grammar: %q", in)
		}
	}

	// The prefix-anchored original keeps its own registry row byte-identically.
	info, ok := parseLockContentionPayload(lockContentionCustomerMonitorPayload)
	if !ok || info.Morphology != "monitor_contention_owner" {
		t.Fatalf("prefix-anchored 形A must ride its own registry row, got %+v ok=%v", info, ok)
	}
}

// ── 件3 · registry dispatch + unknown-form fail-open ───────────────────────

func TestLockContentionMorphologyRegistryDispatch(t *testing.T) {
	cases := []struct {
		payload    string
		morphology string
		kind       string
		ownerPID   int
	}{
		{lockContentionCustomerMonitorPayload, "monitor_contention_owner", blockingKindMonitorContention, 42067},
		{lockContentionCustomerLockPayload, "lock_contention_on", blockingKindMonitorContention, 42067},
		{"vendor: monitor contention with owner #W (5) at a()(b:1) waiters=1", "monitor_contention_owner_embedded", blockingKindMonitorContention, 5},
		{"MyVendorLock owner tid=77", "owner_tid_keyed", blockingKindLockContention, 77},
	}
	for _, tc := range cases {
		info, ok := parseLockContentionPayload(tc.payload)
		if !ok {
			t.Fatalf("registered form must parse: %q", tc.payload)
		}
		if info.Morphology != tc.morphology || info.Kind != tc.kind || info.Owner.PID != tc.ownerPID {
			t.Fatalf("dispatch drift for %q: got morph=%q kind=%q owner=%d, want %q/%q/%d",
				tc.payload, info.Morphology, info.Kind, info.Owner.PID, tc.morphology, tc.kind, tc.ownerPID)
		}
		if c := lockContentionMorphologyConfidence(info.Morphology); c != 0.72 {
			t.Fatalf("all registered morphologies declare the 0.72 structured-payload grade, got %v for %q", c, info.Morphology)
		}
	}
	if c := lockContentionMorphologyConfidence("no_such_morphology"); c != 0 {
		t.Fatalf("unregistered morphology names must read 0 (caller keeps its base), got %v", c)
	}
	// Unknown owner-vocabulary form: NOT a contention payload.
	if _, ok := parseLockContentionPayload("lock owner=5 style=xyz"); ok {
		t.Fatalf("an unregistered owner form must not parse as contention")
	}
}

// The unknown owner-vocabulary form fails open END TO END: payload-less lane
// (XERR1-FIX value-basis discipline), no holder minted from the numeric
// token, typed marker + disclosure present.
func TestLocknsUnknownOwnerFormFailsOpenWithDisclosure(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_unknown_form.systrace", `
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|lock owner=5 style=xyz
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
          aweme-41999 (41905) [002] .... 5.090100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=aweme next_pid=41999 next_prio=110
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.BlockingKind != "" {
		t.Fatalf("an unregistered owner form must stay payload-less, got kind=%q", row.BlockingKind)
	}
	if !row.OwnerKeyUnregistered {
		t.Fatalf("the typed unknown-morphology marker must ride the row: %+v", row)
	}
	if row.Peer.PID == 5 || row.HolderSource != "" {
		t.Fatalf("no holder may be minted from the unregistered numeric token: peer=%+v source=%q", row.Peer, row.HolderSource)
	}
	if row.BlockingValueBasis != BlockingValueBasisWaitSegments && row.BlockingValueBasis != BlockingValueBasisSpanEnvelope {
		t.Fatalf("the payload-less value-basis discipline must run unchanged, got basis=%q", row.BlockingValueBasis)
	}
	if !strings.Contains(row.Summary, "morphology unregistered") {
		t.Fatalf("owner-unresolved disclosure missing: %s", row.Summary)
	}

	// Negative: a REGISTERED keyed form never carries the marker.
	idx2 := buildTraceIndex(t, "lockns_registered_form.systrace", `
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|MyVendorLock owner tid=77
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`)
	row2 := blockingSpanRowFor(t, idx2, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row2.BlockingKind == "" || row2.OwnerKeyUnregistered || strings.Contains(row2.Summary, "morphology unregistered") {
		t.Fatalf("registered keyed form must not carry the unknown-morphology marker: kind=%q marker=%v summary=%s",
			row2.BlockingKind, row2.OwnerKeyUnregistered, row2.Summary)
	}
}

// ── 件4 · ownerless sentinel disclosure on ALL branches ────────────────────

func TestLocknsOwnerlessSentinelNoEdgeDisclosed(t *testing.T) {
	// owner tid: 0 sentinel, identity-ns emission, NO wakeup rows — the
	// previously-silent branch (§29.104.12 G3 静默口).
	idx := buildTraceIndex(t, "lockns_ownerless_silent.systrace", `
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on thread suspend count lock (owner tid: 0)
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.BlockingKind == "" || row.Peer.PID != 0 || row.HolderSource != "" || row.OwnerTidRaw != 0 {
		t.Fatalf("sentinel shape drifted: kind=%q peer=%+v source=%q raw=%d", row.BlockingKind, row.Peer, row.HolderSource, row.OwnerTidRaw)
	}
	if !strings.Contains(row.Summary, "holder unresolved") || !strings.Contains(row.Summary, "no closing wakeup edge") {
		t.Fatalf("件4 all-branch ownerless disclosure missing: %s", row.Summary)
	}
	// 值面零动: the value stays the whole-wait envelope (payload-typed lane).
	if diff := row.DurationMs - 100.0; diff > 1e-6 || diff < -1e-6 {
		t.Fatalf("the disclosure must not touch the value face, got %.6f", row.DurationMs)
	}
	// The sentinel is never a real id: no garbage number in the disclosure.
	if strings.Contains(row.Summary, "9223372036854775807") || strings.Contains(row.Summary, "18446744073709551615") {
		t.Fatalf("sentinel values must never surface as ids: %s", row.Summary)
	}
}

func TestLocknsOwnedRowMintsNoUnresolvedDisclosure(t *testing.T) {
	// 有主形零披露: a payload-direct row must carry NO 件4 sentence.
	idx := buildTraceIndex(t, "lockns_owned_negative.systrace", `
         holder-51000 (41905) [004] .... 4.950000: sched_switch: prev_comm=holder prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=other next_pid=51001 next_prio=120
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on thread suspend count lock (owner tid: 51000)
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceContentionPayload {
		t.Fatalf("precondition: owned form must resolve payload-direct, got %q", row.HolderSource)
	}
	if strings.Contains(row.Summary, "holder unresolved") {
		t.Fatalf("owned rows must mint zero unresolved disclosure: %s", row.Summary)
	}
}

// ── fixture regressions (pin③: 现行正确行为保持) ───────────────────────────

// donghu 形A container regression: the folded dual-print monitor contention
// (owner ransmitThread (38414), container ns pid 37722, host tgid 17267)
// resolves through the derivation ladder to host thread ransmitThread-18130 —
// rung ② PROCESS level (§18.E ②c) × rung ③ closing-waker corroboration
// (§18.E 增补 ②×③). The G1 gate is a NO-OP on this fixture (38414 is not a
// host tid), so this behavior is identical before and after 件1 — the pin
// guards the ladder itself.
func TestLocknsDonghuContainerFormARegression(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatalf("build donghu fixture: %v", err)
	}
	if idx.tidPresent(38414) {
		t.Fatalf("fixture drift: container owner tid 38414 must not be a host tid")
	}
	q := Query{PID: 17267, TimeStart: 13762.9805, TimeEnd: 13762.9820, MinDurationMs: 0.0001}
	spans, _, _ := computeTraceMarks(idx, q, 64)
	var contention []TraceSpanSummary
	for _, sp := range spans {
		if strings.Contains(sp.Name, "contention") {
			contention = append(contention, sp)
		}
	}
	rows := collectBlockingSpanRows(idx, q, WindowStats{TraceSpans: contention})
	var mon *CriticalBlockingCandidate
	for i := range rows {
		if rows[i].cand.BlockingKind == blockingKindMonitorContention && rows[i].cand.Thread.PID == 17585 {
			mon = &rows[i].cand
			break
		}
	}
	if mon == nil {
		t.Fatalf("LegoHandler monitor-contention row missing: %d rows", len(rows))
	}
	if len(mon.MergedLines) == 0 {
		t.Fatalf("the dual print (rich + tid-only) must fold into one row: %+v", mon)
	}
	if mon.OwnerTidRaw != 38414 {
		t.Fatalf("container owner tid must ride the audit field, got %d", mon.OwnerTidRaw)
	}
	if mon.Peer.PID != 18130 || mon.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("container 形A must resolve to host ransmitThread-18130 via the corroborated closing waker, got source=%q peer=%+v", mon.HolderSource, mon.Peer)
	}
	if diff := mon.Confidence - counterpartNsSpanDerivationConfidence; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("②×③ corroboration rides the 0.67 ns-span grade, got %.3f", mon.Confidence)
	}
	if !strings.Contains(mon.HolderHostProcess, "tgid=17267") || !strings.Contains(mon.HolderHostProcess, "ns_pid=37722") {
		t.Fatalf("process-level ns identity must ride the display note: %q", mon.HolderHostProcess)
	}
	if !strings.Contains(mon.Summary, "corroborate") {
		t.Fatalf("②×③ corroboration disclosure missing: %s", mon.Summary)
	}
}

// tieba 形B sentinel regression: the uint64(-1) ownerless rows keep the typed
// contention semantics + wait_object, never mint a garbage id, and (件4) now
// disclose the unresolved holder when no closing wakeup edge exists.
func TestLocknsTiebaSentinelRegression(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu_tieba_frame.systrace")
	if err != nil {
		t.Fatalf("build tieba fixture: %v", err)
	}
	q := Query{PID: 59566, TimeStart: 34579.4530, TimeEnd: 34579.4560, MinDurationMs: 0.0001}
	spans, _, _ := computeTraceMarks(idx, q, 128)
	var contention []TraceSpanSummary
	for _, sp := range spans {
		if strings.Contains(sp.Name, "contention") {
			contention = append(contention, sp)
		}
	}
	rows := collectBlockingSpanRows(idx, q, WindowStats{TraceSpans: contention})
	if len(rows) == 0 {
		t.Fatalf("expected sentinel contention rows in the probe window")
	}
	for _, row := range rows {
		c := row.cand
		if c.BlockingKind != blockingKindLockContention {
			t.Fatalf("sentinel row must keep the typed contention kind, got %q", c.BlockingKind)
		}
		if c.WaitObject != "ClassLinker classes lock" {
			t.Fatalf("wait object must survive the sentinel parse, got %q", c.WaitObject)
		}
		if c.Peer.PID != 0 || c.OwnerTidRaw != 0 {
			t.Fatalf("uint64(-1) sentinel must never mint an owner id: peer=%+v raw=%d", c.Peer, c.OwnerTidRaw)
		}
		if strings.Contains(c.Summary, "9223372036854775807") {
			t.Fatalf("the MaxInt64 clamp garbage must never resurface: %s", c.Summary)
		}
		if !strings.Contains(c.Summary, "holder unresolved") {
			t.Fatalf("件4 disclosure missing on the edge-less sentinel row: %s", c.Summary)
		}
	}
}

// ── 修补轮 件A+件B (冷读 P2-F1+P3-F7 同族 + P3-F6, 2026-07-16) ──────────────
//
//	件A  every rung-①-diverged row carries the typed OwnerTidPresence verdict
//	     (absent / present_collision / present_comm_mismatch), minted from the
//	     SAME engine determination bits that diverted it — the wire note the
//	     detail 持有者来历 presence clause forks on. Rung-①-resolved rows
//	     carry NO verdict (empty = legacy/fail-open).
//	件B  the collision Summary words attribution semantics ("not a
//	     holder-attribution basis (a collision can only coincidentally
//	     match)") — the categorical "never the holder" overclaimed the ruling
//	     (裁定原文只说「撞对只是巧合」, a coincidental physical match is
//	     possible).

func TestLocknsRepairPresenceCollisionShapes(t *testing.T) {
	shapes := []struct {
		name    string
		fixture string
		trace   string
		source  string
	}{
		{"rung2_thread_level", "lockns_rep_a.systrace", locknsCollisionSelfReportTrace, CounterpartSourceNsSpanDerivation},
		{"rung3_wakeup_edge", "lockns_rep_b.systrace", locknsCollisionWakeupTrace, CounterpartSourceWakeupEdge},
		{"unresolved", "lockns_rep_c.systrace", locknsCollisionUnresolvedTrace, ""},
	}
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			idx := buildTraceIndex(t, tc.fixture, tc.trace)
			row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
			if row.HolderSource != tc.source {
				t.Fatalf("precondition drift: source=%q want %q", row.HolderSource, tc.source)
			}
			// 件A: the collision shape mints the typed presence verdict on
			// EVERY fallback outcome (②/③/unresolved).
			if row.OwnerTidPresence != OwnerTidPresenceCollision {
				t.Fatalf("collision shape must mint presence=%q, got %q", OwnerTidPresenceCollision, row.OwnerTidPresence)
			}
			// 件B: attribution wording only — the categorical claim is dead.
			if strings.Contains(row.Summary, "never the holder") {
				t.Fatalf("件B: the categorical \"never the holder\" must not surface: %s", row.Summary)
			}
			if strings.Contains(row.Summary, "numeric collision") &&
				!strings.Contains(row.Summary, "not a holder-attribution basis (a collision can only coincidentally match") {
				t.Fatalf("件B: the collision sentence must carry the attribution wording: %s", row.Summary)
			}
		})
	}
}

// 件A absent shape: identity-ns row whose payload owner tid names no thread
// this trace scheduled — presence=absent and the legacy "not present in this
// trace" sentence stays byte-identically (it is TRUE here).
const locknsRepairAbsentTrace = `
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on a monitor lock (owner tid: 77777)
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       releaser-800 (800) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.090100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=aweme next_pid=41999 next_prio=110
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`

func TestLocknsRepairPresenceAbsentKeepsLegacySentence(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_rep_d.systrace", locknsRepairAbsentTrace)
	if idx.tidPresent(77777) {
		t.Fatalf("precondition: owner tid 77777 must be absent from the trace")
	}
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceWakeupEdge || row.OwnerTidRaw != 77777 {
		t.Fatalf("precondition drift: source=%q raw=%d", row.HolderSource, row.OwnerTidRaw)
	}
	if row.OwnerTidPresence != OwnerTidPresenceAbsent {
		t.Fatalf("absent shape must mint presence=%q, got %q", OwnerTidPresenceAbsent, row.OwnerTidPresence)
	}
	// The legacy sentence is TRUE on this shape and stays byte-identically.
	if !strings.Contains(row.Summary, "payload owner tid 77777 is not present in this trace") {
		t.Fatalf("absent shape must keep the legacy caveat: %s", row.Summary)
	}
}

// 件A comm-mismatch shape (P3-F7 同族): the rich 形A payload names owner tid
// 51000 with comm "GhostOwner", tid 51000 IS present but only ever observed as
// "victim" — the comm cross-check rejects rung ① and the row resolves via
// rung ③ carrying presence=present_comm_mismatch.
const locknsRepairCommMismatchTrace = `
       victim-51000 (50000) [005] .... 4.800000: sched_switch: prev_comm=victim prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=idle/5 next_pid=0 next_prio=120
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|monitor contention with owner #GhostOwner (51000) at void a.b.C.d()(C.java:5) waiters=1
          aweme-41999 (41905) [002] .... 5.000100: sched_switch: prev_comm=aweme prev_pid=41999 prev_prio=110 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
       releaser-800 (800) [003] .... 5.090000: sched_wakeup: comm=aweme pid=41999 prio=110 target_cpu=002
          aweme-41999 (41905) [002] .... 5.090100: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=aweme next_pid=41999 next_prio=110
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`

func TestLocknsRepairPresenceCommMismatch(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_rep_e.systrace", locknsRepairCommMismatchTrace)
	if !idx.tidPresent(51000) {
		t.Fatalf("precondition: tid 51000 must be present (as the unrelated victim thread)")
	}
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceWakeupEdge || row.Peer.PID != 800 || row.OwnerTidRaw != 51000 {
		t.Fatalf("comm-collision row must fall to rung ③: source=%q peer=%+v raw=%d", row.HolderSource, row.Peer, row.OwnerTidRaw)
	}
	if row.OwnerTidPresence != OwnerTidPresenceCommMismatch {
		t.Fatalf("comm-mismatch shape must mint presence=%q, got %q", OwnerTidPresenceCommMismatch, row.OwnerTidPresence)
	}
	// 件E: the ENGINE Summary (LLM-facing) stops claiming absence on this
	// shape — the new comm-mismatch sentence lands and the old false claim
	// is dead (it contradicted the same report's detail-face sentence).
	if !strings.Contains(row.Summary, "payload owner tid 51000 is present in this trace but its thread name never matches the payload's owner comm") {
		t.Fatalf("件E: comm-mismatch engine sentence missing: %s", row.Summary)
	}
	if strings.Contains(row.Summary, "not present in this trace") {
		t.Fatalf("件E: the false absence claim must die on the comm-mismatch Summary: %s", row.Summary)
	}
}

// 件A negative: a rung-①-resolved row carries NO presence verdict (empty =
// the display fail-open lane; legacy sentences never fork on resolved rows).
func TestLocknsRepairPresenceEmptyOnPayloadDirect(t *testing.T) {
	idx := buildTraceIndex(t, "lockns_rep_f.systrace", `
         holder-51000 (41905) [004] .... 4.950000: sched_switch: prev_comm=holder prev_pid=51000 prev_prio=120 prev_state=R ==> next_comm=other next_pid=51001 next_prio=120
          aweme-41999 (41905) [002] .... 5.000000: print: B|41905|Lock contention on thread suspend count lock (owner tid: 51000)
          aweme-41999 (41905) [002] .... 5.100000: print: E|41905
`)
	row := blockingSpanRowFor(t, idx, Query{View: "critical_blocking_calls", PID: 41905, TimeStart: 4.99, TimeEnd: 5.2})
	if row.HolderSource != CounterpartSourceContentionPayload {
		t.Fatalf("precondition: owned form must resolve payload-direct, got %q", row.HolderSource)
	}
	if row.OwnerTidPresence != "" {
		t.Fatalf("payload-direct rows must carry no presence verdict, got %q", row.OwnerTidPresence)
	}
}

// 件B word-face unit pins: all four collision sentences (rung-③ caveat,
// honest-unresolved caveat, rung-② presence clause, process-level append)
// speak attribution semantics and never the categorical claim.
func TestLocknsRepairCollisionWordFace(t *testing.T) {
	faces := []string{
		counterpartNsDivergentWakeupEdgeCaveat(51000),
		nsDivergentUnresolvedCaveat(51000),
		nsSpanOwnerPresenceClause(OwnerTidPresenceCollision),
		nsSpanProcessLevelCaveat(51000, nsSpanOwnerResolution{HostTGID: 41905, HostProcessComm: "aweme", NsPID: 43000, OK: true}, true),
	}
	for i, face := range faces {
		if strings.Contains(face, "never the holder") {
			t.Fatalf("face %d must not carry the categorical claim: %s", i, face)
		}
		if !strings.Contains(face, "not a holder-attribution basis (a collision can only coincidentally match") {
			t.Fatalf("face %d must word attribution semantics: %s", i, face)
		}
	}
	// The absent / legacy presence clause stays byte-identical (件E fail-open
	// arm included: an unknown future value never invents a new sentence).
	for _, presence := range []string{OwnerTidPresenceAbsent, "", "some_future_value"} {
		if got := nsSpanOwnerPresenceClause(presence); got != "not present in this trace" {
			t.Fatalf("presence=%q clause must stay legacy: %q", presence, got)
		}
	}
}

// ── 修补轮 件E · 引擎 Summary presence 句分叉 (件A 引擎面半边, 2026-07-16) ──
//
// The two rung-②/③ engine presence sentences fork on the SAME typed 件A
// verdict: comm-mismatch stops claiming absence (the claim was false and
// contradicted the same report's detail face), the true absent shape keeps
// the legacy sentence byte-identically, and the collision shape rides the
// 件1/件B collision caveats unchanged.
func TestLocknsRepairEngineCommMismatchWordFace(t *testing.T) {
	// Rung-③ caveat fork (unit pins, both arms byte-exact).
	legacy := "counterpart inferred from the waiter's wakeup edge because the payload owner tid 51000 is not present in this trace; it is the thread that woke the waiter, not a payload-confirmed holder"
	for _, presence := range []string{OwnerTidPresenceAbsent, "", "some_future_value"} {
		if got := counterpartWakeupEdgeCaveat(51000, presence); got != legacy {
			t.Fatalf("presence=%q must keep the legacy rung-③ sentence byte-identically: %q", presence, got)
		}
	}
	mm := counterpartWakeupEdgeCaveat(51000, OwnerTidPresenceCommMismatch)
	if !strings.Contains(mm, "payload owner tid 51000 is present in this trace but its thread name never matches the payload's owner comm") ||
		!strings.Contains(mm, "not a holder-attribution basis") {
		t.Fatalf("comm-mismatch rung-③ sentence must word presence + attribution semantics: %q", mm)
	}
	if strings.Contains(mm, "not present in this trace") {
		t.Fatalf("comm-mismatch rung-③ sentence must not claim absence: %q", mm)
	}
	// The payload-less form ignores presence (no owner tid to describe).
	if got := counterpartWakeupEdgeCaveat(0, ""); !strings.Contains(got, "no structured owner in the payload") {
		t.Fatalf("payload-less caveat drifted: %q", got)
	}
	// Rung-② presence clause fork (the thread-level caveat's middle segment).
	clause := nsSpanOwnerPresenceClause(OwnerTidPresenceCommMismatch)
	if !strings.Contains(clause, "present but its thread name never matches the payload's owner comm") ||
		!strings.Contains(clause, "not a holder-attribution basis") {
		t.Fatalf("comm-mismatch rung-② clause must word presence + attribution semantics: %q", clause)
	}
	if strings.Contains(clause, "not present in this trace") || strings.Contains(clause, "never the holder") {
		t.Fatalf("comm-mismatch rung-② clause must not claim absence or the categorical claim: %q", clause)
	}
}
