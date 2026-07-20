package tracequery

// evalcase_dh_lock_pin_test.go — EVALCASE-DH batch, ART 锁 / 语义 span family
// engine pins on the committed donghu.ftrace (mining ledger
// evalcase_donghu_mining.md §B/§G; expectations re-collected at HEAD
// 1ada2c49f — post LOCKNS/XERR1-EXT — and hand-cross-checked).
//
// Cases:
//
//	DH-L1  形A monitor contention + 容器 ns owner (窗 13762.980500..985000,
//	       waiter LegoHandler-17585): the rich ART payload parses with owner
//	       ransmitThread(38414) — a CONTAINER-namespace tid absent from the
//	       host tid space; the carve folds the dual-print twin, keeps the
//	       payload owner on the typed audit lane (owner_tid_raw=38414,
//	       owner_tid_presence=absent), derives the HOST PROCESS ONLY via
//	       ns-span pairs (ns_pid=37722 → .ugc.aweme.lite-17267), and says
//	       in words that the holder THREAD stays underived — no fabricated
//	       host-thread mapping despite two same-named host candidates
//	       (ransmitThread 18130/18134).
//	DH-L2  形B sentinel owners (窗 13762.891000..894000): `owner tid: 0` and
//	       `owner tid: 18446744073709551615` are EXPLICIT no-holder
//	       sentinels — typed ownerless, never a Peer entity, never an
//	       owner_tid_raw audit number; µs-scale envelopes stay µs. The
//	       window's dominant word is the target's own running (2.715ms@cpu12,
//	       hand-recomputed) — lock "contention" here carries no wall-clock
//	       blocking narrative.
//	DH-S1  envelope≠wait 词面 (H:Waiting for PresentFence_0, same L1 window):
//	       the payload-less blocking span publishes the WAITER's
//	       Σ(sleep+d_state+io_wait)=3.661ms as its value with the 3.735ms
//	       envelope demoted to disclosure ("contains run time and is NOT the
//	       blocking wait") — the XERR1-FIX basis discipline on a live span.
//	LOCKSPAN-SEAT 现状 pin (候选 gap 调查, spec 踩点②): in this real window
//	       the ART lock spans are PAIRED and ADMITTED by the blocking-span
//	       carve (lexicon has NO gap), but the production window_stats
//	       trace_spans face keeps only the top-8 spans BY DURATION, and the
//	       µs–sub-ms lock spans are crowded out by ≥3.4ms envelope spans —
//	       so no lock seat is minted on the production face while the
//	       PresentFence envelope span (3.735ms) survives and mints. Pinned
//	       as documented current behavior; the adjudication material lives
//	       in the batch ledger.

import (
	"strings"
	"testing"
)

const (
	evalcaseL1Start = 13762.9805
	evalcaseL1End   = 13762.985
)

func evalcaseUnboundedSpans(t *testing.T, idx *Index, q Query, max int) []TraceSpanSummary {
	t.Helper()
	q = normalizeQuery(idx, q)
	spans, _, _ := computeTraceMarks(idx, q, max)
	return spans
}

// DH-L1 pairing + parse face.
func TestEvalcaseDHL1MonitorContentionParse(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := Query{PID: 17585, TimeStart: evalcaseL1Start, TimeEnd: evalcaseL1End, MinDurationMs: 0.0001}
	spans := evalcaseUnboundedSpans(t, idx, q, 500)
	var monitor *TraceSpanSummary
	var formB *TraceSpanSummary
	for i := range spans {
		s := &spans[i]
		if s.Thread.PID == 17585 && strings.HasPrefix(s.Name, "monitor contention with owner ransmitThread (38414)") && near(s.DurationMs, 0.295, 0.001) {
			monitor = s
		}
		if s.Thread.PID == 17457 && strings.HasPrefix(s.Name, "Lock contention on thread list lock (owner tid: 37864)") && near(s.DurationMs, 0.559, 0.001) {
			formB = s
		}
	}
	if monitor == nil {
		t.Fatalf("DH-L1: the 0.295ms 形A monitor span is missing from unbounded pairing")
	}
	if formB == nil {
		t.Fatalf("DH-L1: the 0.559ms 形B thread-list span (waiter #tp-io-4946-17457, owner 37864) is missing")
	}
	info, ok := parseLockContentionPayload(monitor.Name)
	if !ok || info.Kind != blockingKindMonitorContention || info.Morphology != "monitor_contention_owner" {
		t.Fatalf("DH-L1: 形A parse drifted: ok=%v %+v", ok, info)
	}
	if info.Owner.Comm != "ransmitThread" || info.Owner.PID != 38414 || info.OwnerAbsent {
		t.Fatalf("DH-L1: payload owner drifted: %+v", info.Owner)
	}
	if !strings.Contains(info.HolderSite, "MessageQueue.enqueueMessageLegacy") {
		t.Fatalf("DH-L1: holder_site drifted: %q", info.HolderSite)
	}
	if !strings.Contains(info.BlockingFromSite, "MessageQueue.nextLegacy") {
		t.Fatalf("DH-L1: blocking_from_site drifted: %q", info.BlockingFromSite)
	}
	// The container owner tid is a HOST-space ghost: 38414 never scheduled in
	// this trace (the honest-disclosure precondition).
	if idx.tidPresent(38414) {
		t.Fatalf("DH-L1: fixture fact drifted — owner tid 38414 must be absent from the host tid space")
	}
}

// DH-L1 carve face — fold + ns derivation + honest thread-level abstention.
func TestEvalcaseDHL1CarveNsHonestDisclosure(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17585, TimeStart: evalcaseL1Start, TimeEnd: evalcaseL1End, MinDurationMs: 0.0001})
	spans := evalcaseUnboundedSpans(t, idx, q, 500)
	stats := ComputeWindowStats(idx, q)
	stats.TraceSpans = spans
	rows := collectBlockingSpanRows(idx, q, stats)
	var row *blockingSpanRow
	for i := range rows {
		if rows[i].cand.BlockingKind == blockingKindMonitorContention && rows[i].cand.Thread.PID == 17585 && near(rows[i].cand.SpanEnvelopeMs, 0.295, 0.001) {
			row = &rows[i]
		}
	}
	if row == nil {
		t.Fatalf("DH-L1: monitor carve row missing from the full-lane carve")
	}
	cand := row.cand
	// Dual-print fold: the same physical contention printed as the rich 形A
	// and the "Lock contention on a monitor lock" twin — folded, merged
	// lines recorded, richer form surviving.
	if len(cand.MergedLines) == 0 {
		t.Fatalf("DH-L1: dual-print twin must fold into the rich form (MergedLines empty)")
	}
	// Typed audit lane: the payload owner stays on owner_tid_raw with the
	// presence verdict absent — never minted as a host Peer identity.
	if cand.OwnerTidRaw != 38414 || cand.OwnerTidPresence != OwnerTidPresenceAbsent {
		t.Fatalf("DH-L1: payload-owner audit lane drifted: raw=%d presence=%q", cand.OwnerTidRaw, cand.OwnerTidPresence)
	}
	// ns-span derivation reaches PROCESS level only.
	if !strings.Contains(cand.HolderHostProcess, "ns_pid=37722") || !strings.Contains(cand.HolderHostProcess, "tgid=17267") {
		t.Fatalf("DH-L1: ns process derivation drifted: %q", cand.HolderHostProcess)
	}
	// Value basis: converged Σ(wait segments) with the envelope preserved as
	// disclosure (XERR1-EXT 裁定⑤ applies to payload rows too).
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments || !near(cand.DurationMs, 0.185, 0.001) || !near(cand.SpanEnvelopeMs, 0.295, 0.001) {
		t.Fatalf("DH-L1: value basis drifted: basis=%q dur=%.3f envelope=%.3f", cand.BlockingValueBasis, cand.DurationMs, cand.SpanEnvelopeMs)
	}
	// Honest abstention words: process-level narrowing is disclosed and the
	// specific holder thread is explicitly left underived (never a fabricated
	// pick between the two same-named host candidates 18130/18134).
	for _, want := range []string{
		"payload owner tid 38414 is a container-namespace id (ns pid 37722)",
		"no thread-level mapping material exists in this trace",
	} {
		if !strings.Contains(cand.Summary, want) {
			t.Fatalf("DH-L1: honest-disclosure word missing %q in %q", want, cand.Summary)
		}
	}
	// The wakeup-edge counterpart lane (a deterministic observed edge, not a
	// name mapping) is the only thread-level clue and is labelled as such.
	if cand.HolderSource != "wakeup_edge" {
		t.Fatalf("DH-L1: holder_source drifted: %q", cand.HolderSource)
	}
}

// DH-L2 sentinel discipline.
func TestEvalcaseDHL2SentinelOwners(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17267, TimeStart: 13762.891, TimeEnd: 13762.894, MinDurationMs: 0.0001})
	spans := evalcaseUnboundedSpans(t, idx, q, 500)
	stats := ComputeWindowStats(idx, q)
	// Parse face: both sentinel spellings are typed ownerless.
	sentinels := 0
	realOwners := map[int]int{}
	for _, s := range spans {
		if !strings.HasPrefix(s.Name, "Lock contention on ") {
			continue
		}
		info, ok := parseLockContentionPayload(s.Name)
		if !ok {
			t.Fatalf("DH-L2: 形B payload failed to parse: %q", s.Name)
		}
		if s.DurationMs > 0.004 {
			t.Fatalf("DH-L2: lock span envelope must stay µs-scale (≤0.004ms), got %.4f for %q", s.DurationMs, s.Name)
		}
		if strings.Contains(s.Name, "owner tid: 18446744073709551615") || strings.Contains(s.Name, "owner tid: 0)") {
			sentinels++
			if !info.OwnerAbsent || info.Owner.PID != 0 {
				t.Fatalf("DH-L2: sentinel owner leaked as an entity: %+v for %q", info, s.Name)
			}
		} else if info.Owner.PID > 0 {
			realOwners[info.Owner.PID]++
		}
	}
	if sentinels < 5 {
		t.Fatalf("DH-L2: expected ≥5 sentinel rows (uint64-1 ×4 + 0 ×1 in this window), got %d", sentinels)
	}
	if realOwners[37858] == 0 || realOwners[37928] == 0 {
		t.Fatalf("DH-L2: the real container owners (37858/37928) went missing: %v", realOwners)
	}
	// Carve face: sentinel rows keep an EMPTY peer and no owner_tid_raw
	// audit value (the old Atoi clamp printed 9223372036854775807 garbage).
	stats.TraceSpans = spans
	rows := collectBlockingSpanRows(idx, q, stats)
	checked := 0
	for _, r := range rows {
		if !strings.Contains(r.spanName, "owner tid: 18446744073709551615") && !strings.Contains(r.spanName, "owner tid: 0)") {
			continue
		}
		checked++
		if r.cand.Peer.PID != 0 || r.cand.Peer.Comm != "" || r.cand.OwnerTidRaw != 0 {
			t.Fatalf("DH-L2: sentinel row minted an owner identity: peer=%+v raw=%d", r.cand.Peer, r.cand.OwnerTidRaw)
		}
		if r.cand.WaitObject == "" {
			t.Fatalf("DH-L2: ownerless row must keep its wait_object description: %+v", r.cand)
		}
	}
	if checked == 0 {
		t.Fatalf("DH-L2: no sentinel rows reached the carve")
	}
	// The window's dominant physical word: the target's own running
	// (hand-recomputed 2.715ms@cpu12) — µs lock spans must never grow a
	// wall-clock blocking narrative.
	top := stats.TopRunning[0]
	if top.Thread.PID != 17267 || top.CPU != 12 || !near(top.DurationMs, 2.715, 0.001) {
		t.Fatalf("DH-L2: dominant running seat drifted: %+v", top)
	}
}

// DH-S1: envelope≠wait 词面 on the live PresentFence span — this row IS on
// the production face (its 3.735ms envelope survives the top-8 bound).
func TestEvalcaseDHS1PresentFenceEnvelopeNotWait(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17585, TimeStart: evalcaseL1Start, TimeEnd: evalcaseL1End, MinDurationMs: 0.0001})
	stats := ComputeWindowStats(idx, q)
	rows := collectBlockingSpanRows(idx, q, stats)
	var row *blockingSpanRow
	for i := range rows {
		if strings.Contains(rows[i].spanName, "Waiting for PresentFence_0") {
			row = &rows[i]
		}
	}
	if row == nil {
		t.Fatalf("DH-S1: PresentFence blocking row missing from the production face")
	}
	cand := row.cand
	if cand.Confidence != 0.72 {
		t.Fatalf("DH-S1: confidence drifted: %v", cand.Confidence)
	}
	if cand.BlockingValueBasis != BlockingValueBasisWaitSegments ||
		!near(cand.DurationMs, 3.661, 0.001) || !near(cand.WaitSleepMs, 3.661, 0.001) ||
		!near(cand.SpanEnvelopeMs, 3.735, 0.001) {
		t.Fatalf("DH-S1: value convergence drifted: dur=%.3f sleep=%.3f envelope=%.3f", cand.DurationMs, cand.WaitSleepMs, cand.SpanEnvelopeMs)
	}
	for _, want := range []string{
		"Σ(sleep+d_state+io_wait)=3.661ms",
		"span envelope 3.735ms contains run time and is NOT the blocking wait",
		"the envelope is not a blocking-wait measure",
	} {
		if !strings.Contains(cand.Summary, want) {
			t.Fatalf("DH-S1: 词面 missing %q in %q", want, cand.Summary)
		}
	}
	if !cand.WaitBudgetExceeded || !near(cand.WaitBudgetNonRunningMs, 3.661, 0.001) {
		t.Fatalf("DH-S1: budget marker drifted: %+v", cand)
	}
}

// LOCKSPAN-SEAT 现状 pin: the lexicon admits the ART lock spans (no词表缺口),
// the pairing mints them, the carve accepts them — but the production
// trace_spans face is a top-8-by-duration bound and the µs–sub-ms lock spans
// are crowded out by long envelope spans, so no lock seat reaches the
// production blocking lane in this window while the 3.735ms PresentFence
// envelope span does. Documented CURRENT behavior (adjudication pending);
// when the bound or the carve feed changes, this pin reddens deliberately.
func TestEvalcaseLockSpanSeatCurrentStateLOCKSPANSEAT(t *testing.T) {
	idx := evalcaseIndex(t, evalcaseDonghuFixture)
	q := normalizeQuery(idx, Query{PID: 17585, TimeStart: evalcaseL1Start, TimeEnd: evalcaseL1End, MinDurationMs: 0.0001})
	// ① Unbounded pairing DOES mint the lock spans (≥7 contention spans in
	// this window) and the carve admits every one of them.
	unbounded := evalcaseUnboundedSpans(t, idx, q, 500)
	lockSpans := 0
	for _, s := range unbounded {
		if !strings.Contains(s.Name, "contention") {
			continue
		}
		lockSpans++
		if _, ok := blockingSpanCandidateFromTraceSpan(s); !ok {
			t.Fatalf("LOCKSPAN-SEAT: carve rejected a lock span (词表缺口?): %q", s.Name)
		}
	}
	if lockSpans < 7 {
		t.Fatalf("LOCKSPAN-SEAT: expected ≥7 contention spans in the unbounded pairing, got %d", lockSpans)
	}
	// ② The production face keeps only the top-8 spans by duration — all
	// ≥3.4ms envelopes here — so ZERO contention spans survive the bound.
	stats := ComputeWindowStats(idx, q)
	if len(stats.TraceSpans) != 8 {
		t.Fatalf("LOCKSPAN-SEAT: production span face bound drifted: %d spans", len(stats.TraceSpans))
	}
	for _, s := range stats.TraceSpans {
		if strings.Contains(s.Name, "contention") {
			t.Fatalf("LOCKSPAN-SEAT: current-state drifted — a contention span reached the production face: %q (adjudication material stale)", s.Name)
		}
		if s.DurationMs < 3.4 {
			t.Fatalf("LOCKSPAN-SEAT: bound shape drifted — production span below 3.4ms: %q %.3f", s.Name, s.DurationMs)
		}
	}
	// ③ Consequence: the production blocking lane carries the PresentFence
	// envelope seat but NO lock seat.
	rows := collectBlockingSpanRows(idx, q, stats)
	for _, r := range rows {
		if r.cand.BlockingKind != "" {
			t.Fatalf("LOCKSPAN-SEAT: current-state drifted — a lock-kind row reached the production lane: %+v", r.cand)
		}
	}
	// ④ EVOLUTION (SPANVIS-1 修向 C, user ruling 2026-07-19 定形原则): the
	// adjudication-pool item resolved as MENTION-NOT-SEAT — ①②③ above stay
	// the pinned seat-machinery behavior byte-identically (the carve keeps
	// consuming the bounded view), and the evidence-value inversion closes on
	// the ADVISORY face instead: the same window's business-span mention face
	// carries the crowded-out ART lock families (waiter LegoHandler-17585 =
	// self, both the rich 形A monitor form and its 形B twin), minted from the
	// FULL span inventory.
	chain := BuildWakeupChain(idx, q)
	rank := buildRootCauseRankFromWithCache(idx, q, chain, stats, nil)
	if rank.BusinessSpanMentions == nil {
		t.Fatalf("LOCKSPAN-SEAT ④: the mention face must carry the crowded-out lock families")
	}
	sawFormA, sawFormB := false, false
	for _, fam := range rank.BusinessSpanMentions.Families {
		if fam.Thread.PID != 17585 || fam.OnChainBasis != BusinessSpanMentionBasisSelf {
			continue
		}
		if strings.HasPrefix(fam.Name, "monitor contention with owner ransmitThread") {
			sawFormA = true
			if fam.HiddenCount < 1 {
				t.Fatalf("LOCKSPAN-SEAT ④: the mentioned 形A family must carry cap-hidden members: %+v", fam)
			}
		}
		if strings.HasPrefix(fam.Name, "Lock contention on a monitor lock") {
			sawFormB = true
		}
	}
	if !sawFormA || !sawFormB {
		t.Fatalf("LOCKSPAN-SEAT ④: expected the self ART lock 形A+形B mention families, got %+v", rank.BusinessSpanMentions.Families)
	}
}
