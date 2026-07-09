package tracequery

// lock_contention_ownertid_blind2_test.go — BLIND-2 generalized owner-tid
// keyed arm pins (§29.2 + §29.7-1 ruling, real_trace_campaign_20260705.md,
// 2026-07-09; specimen: census_report_a.txt ⑦ InternTable 锁风暴形, 5600+
// rows, tid-0 witness 469 rows).
//
// Two-arm structure:
//   - preferred rich-grammar arm ("monitor contention with owner …") wins
//     whenever it matches — it carries holder_site/waiters/blocking-from/
//     hand-off, strictly more information;
//   - the generalized arm anchors on the `owner tid[:=]<N>` KEY (the carrying
//     signal), not on any prefix wording — the ART census form and future
//     vendor prefixes are both covered without per-flavor enumeration. The
//     known "Lock contention on …" spelling keeps its byte-identical richer
//     subject trimming as an in-family refinement.
//
// MUTATION self-checks:
//   - deleting the parseOwnerTidKeyedPayload default arm reds
//     TestBlind2GeneralizedOwnerTidArmNonARTPrefix;
//   - dropping the admission extension (spanNameCarriesOwnerTidKey) reds
//     TestBlind2KeyAdmitsWithoutBlockingVocabulary;
//   - reordering the switch (generalized before the rich arm) reds
//     TestBlind2RichGrammarArmStaysPreferred (holder_site would vanish).

import (
	"strings"
	"testing"
)

// TestBlind2ARTCensusVerbatimForm: the census verbatim span mints a
// lock-contention candidate with the payload-direct owner tid; the lock
// description keeps the InternTable proper name verbatim (词条尺子: trace
// 专有名词不翻译).
func TestBlind2ARTCensusVerbatimForm(t *testing.T) {
	info, ok := parseLockContentionPayload("Lock contention on InternTable lock (owner tid: 35047)")
	if !ok {
		t.Fatal("census ART form must parse")
	}
	if info.Kind != blockingKindLockContention {
		t.Fatalf("census form mints the lock-contention family kind, got %q", info.Kind)
	}
	if info.Owner.PID != 35047 || info.OwnerAbsent {
		t.Fatalf("owner tid must be payload-direct 35047, got %+v absent=%v", info.Owner, info.OwnerAbsent)
	}
	if info.WaitObject != "InternTable lock" {
		t.Fatalf("the lock description keeps the verbatim proper name, got %q", info.WaitObject)
	}
	if info.HolderSite != "" || info.BlockingFromSite != "" || info.Waiters != 0 {
		t.Fatalf("absent segments must stay empty (不造): %+v", info)
	}
}

// TestBlind2ARTCensusOwnerlessTidZero: the census tid-0 witness (469 rows) is
// the explicit no-holder sentinel — typed ownerless, never a Peer id.
func TestBlind2ARTCensusOwnerlessTidZero(t *testing.T) {
	info, ok := parseLockContentionPayload("Lock contention on InternTable lock (owner tid: 0)")
	if !ok {
		t.Fatal("tid-0 census form must still parse as contention")
	}
	if !info.OwnerAbsent || info.Owner.PID != 0 {
		t.Fatalf("tid 0 is the no-holder sentinel: absent=%v owner=%+v", info.OwnerAbsent, info.Owner)
	}
	if info.WaitObject != "InternTable lock" {
		t.Fatalf("ownerless rows keep the lock description, got %q", info.WaitObject)
	}
}

// TestBlind2GeneralizedOwnerTidArmNonARTPrefix proves the generalization: a
// NON-ART prefix (vendor free vocabulary) with the owner-tid key mints via
// the generalized arm — owner payload-direct, the WHOLE span text verbatim as
// the holder-point description, absent segments empty.
func TestBlind2GeneralizedOwnerTidArmNonARTPrefix(t *testing.T) {
	name := "MyRuntime waiting for heap lock (owner tid= 123)"
	info, ok := parseLockContentionPayload(name)
	if !ok {
		t.Fatal("generalized owner-tid key form must parse")
	}
	if info.Kind != blockingKindLockContention {
		t.Fatalf("generalized arm mints the lock-contention family kind, got %q", info.Kind)
	}
	if info.Owner.PID != 123 || info.OwnerAbsent {
		t.Fatalf("owner tid must be payload-direct 123 (separator '=', tolerated space), got %+v", info.Owner)
	}
	if info.WaitObject != name {
		t.Fatalf("the generalized arm keeps the VERBATIM whole span text as the description, got %q", info.WaitObject)
	}
	if info.HolderSite != "" || info.BlockingFromSite != "" || info.Waiters != 0 || info.OwnerHandoff != nil {
		t.Fatalf("no at/blocking-from/waiters/hand-off segments exist on this form — fields stay empty: %+v", info)
	}
	// tid-0 on the generalized arm: ownerless, no holder minted.
	zero, ok := parseLockContentionPayload("VendorZ queue drain (owner tid: 0)")
	if !ok || !zero.OwnerAbsent || zero.Owner.PID != 0 {
		t.Fatalf("generalized tid-0 must be typed ownerless: ok=%v %+v", ok, zero)
	}
}

// TestBlind2RichGrammarArmStaysPreferred: a span matching the rich grammar
// keeps its full parse even when an owner-tid key also appears in the payload
// — the same span never re-routes through the generalized arm.
func TestBlind2RichGrammarArmStaysPreferred(t *testing.T) {
	info, ok := parseLockContentionPayload(
		"monitor contention with owner #Holder (42) at void x.y()(F.java:1) waiters=1 blocking from void a.b(owner tid= 7)(G.java:2)")
	if !ok {
		t.Fatal("rich form must parse")
	}
	if info.Kind != blockingKindMonitorContention {
		t.Fatalf("rich arm keeps the monitor kind, got %q", info.Kind)
	}
	if info.Owner.PID != 42 || info.Owner.Comm != "Holder" {
		t.Fatalf("owner must come from the rich grammar's trailing (tid), never the embedded key: %+v", info.Owner)
	}
	if info.HolderSite != "void x.y()(F.java:1)" {
		t.Fatalf("rich arm keeps holder_site, got %q", info.HolderSite)
	}
	if info.Waiters != 1 {
		t.Fatalf("rich arm keeps waiters, got %d", info.Waiters)
	}
}

// TestBlind2GeneralizedNegatives: no key → no mint; a key followed by a
// non-integer → no mint (the generic blocking-vocabulary screen still governs
// such spans separately).
func TestBlind2GeneralizedNegatives(t *testing.T) {
	for _, name := range []string{
		"RenderFrame commit 123",                // ordinary span, no key
		"MyRuntime parked (owner tid: abc)",     // key followed by non-integer
		"VendorY spin (owner tid= )",            // key without a number
		"owner tidiness report 5",               // 'owner tid' only as a word fragment
		"H:ReceiveVsync name:WM_42591 now: 12t", // vsync cadence family
	} {
		if _, ok := parseLockContentionPayload(name); ok {
			t.Fatalf("must not mint a contention candidate for %q", name)
		}
	}
}

// TestBlind2KeyAdmitsWithoutBlockingVocabulary: the owner-tid key is the
// carrying signal on the ADMISSION face too — a vendor span without any
// lock/wait vocabulary still enters the blocking-span lane, while an ordinary
// span stays out.
func TestBlind2KeyAdmitsWithoutBlockingVocabulary(t *testing.T) {
	span := TraceSpanSummary{
		Name:    "InternTable stall (owner tid: 77)",
		Thread:  ThreadRef{Comm: "app.main", PID: 100, TGID: 100},
		StartTs: 10.0, EndTs: 10.005, DurationMs: 5.0,
		StartLine: 11, EndLine: 12,
	}
	cand, ok := blockingSpanCandidateFromTraceSpan(span)
	if !ok {
		t.Fatal("the owner-tid key must admit the span into the blocking lane")
	}
	if cand.BlockingKind != blockingKindLockContention || cand.Peer.PID != 77 {
		t.Fatalf("admitted span must carry the parsed contention semantics: %+v", cand)
	}
	if cand.WaitObject != "InternTable stall (owner tid: 77)" {
		t.Fatalf("generalized description is the verbatim span text, got %q", cand.WaitObject)
	}
	if _, ok := blockingSpanCandidateFromTraceSpan(TraceSpanSummary{
		Name: "RenderFrame commit 123", Thread: span.Thread,
		StartTs: 10.0, EndTs: 10.001, DurationMs: 1.0,
	}); ok {
		t.Fatal("ordinary spans stay out of the blocking lane")
	}
}

// TestBlind2SeparatorSpacingVariantsKeepOwner: the known "Lock contention on"
// spelling with a '=' separator or spacing variant keeps its payload-direct
// owner through the keyed fallback (previously degraded to ownerless), and
// the subject trimming stays clean.
func TestBlind2SeparatorSpacingVariantsKeepOwner(t *testing.T) {
	info, ok := parseLockContentionPayload("Lock contention on thread list lock (owner tid= 88)")
	if !ok || info.Owner.PID != 88 {
		t.Fatalf("separator variant must keep the payload-direct owner: ok=%v %+v", ok, info.Owner)
	}
	if info.WaitObject != "thread list lock" {
		t.Fatalf("subject trimming stays clean on the keyed fallback, got %q", info.WaitObject)
	}
	// 负向: the strict paren form stays byte-identical (subject + owner).
	strict, ok := parseLockContentionPayload("Lock contention on the InternTable lock (owner tid: 35047)")
	if !ok || strict.Owner.PID != 35047 || strict.WaitObject != "the InternTable lock" {
		t.Fatalf("strict paren form must stay byte-identical: ok=%v %+v", ok, strict)
	}
}

// TestBlind2KeyedFallbackKeepsMonitorKind (复核 P3-1): the monitor
// reclassification reads the TRIMMED subject — the keyed fallback used to
// compare the raw subject with its wrapping "(" and silently dropped the
// monitor kind on "a monitor lock (owner tid= 5)". The strict form's kind is
// unchanged (负向).
func TestBlind2KeyedFallbackKeepsMonitorKind(t *testing.T) {
	keyed, ok := parseLockContentionPayload("Lock contention on a monitor lock (owner tid= 5)")
	if !ok || keyed.Kind != blockingKindMonitorContention || keyed.Owner.PID != 5 {
		t.Fatalf("keyed-fallback monitor form must keep the monitor kind + owner: ok=%v %+v", ok, keyed)
	}
	if keyed.WaitObject != "a monitor lock" {
		t.Fatalf("keyed-fallback subject trimming must stay clean: %q", keyed.WaitObject)
	}
	strict, ok := parseLockContentionPayload("Lock contention on a monitor lock (owner tid: 5)")
	if !ok || strict.Kind != blockingKindMonitorContention || strict.Owner.PID != 5 {
		t.Fatalf("strict monitor form stays byte-identical: ok=%v %+v", ok, strict)
	}
}

// TestBlind2GeneralizedRowsRideP24_9CMachinery verifies §24.9-C 按构造复用 on
// generalized-arm candidates end-to-end through collectBlockingSpanRows:
//   - waiter-identity fold key: two DIFFERENT waiters on the same owner tid
//     never fold (no chimera), the same waiter's dual print still folds;
//   - self-contradiction guard: an inferred closing-wake holder that was
//     itself queued on the same payload owner for ≥50% of the span withdraws
//     (typed witness + unresolved demotion).
//
// (移交链 '-->' segments do not exist on the owner-tid key form — the
// generalized arm records none by construction; pinned in the non-ART arm
// test above via OwnerHandoff==nil.)
func TestBlind2GeneralizedRowsRideP24_9CMachinery(t *testing.T) {
	stats := WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:    "MyRuntime waiting for heap lock (owner tid= 42067)",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.000, EndTs: 100.115944, DurationMs: 115.944,
			StartLine: 44100, EndLine: 79196,
		},
		{
			Name:    "MyRuntime waiting for heap lock (owner tid= 42067)",
			Thread:  ThreadRef{Comm: "ugc.aweme.lite", PID: 16547, TGID: 16547},
			StartTs: 100.003, EndTs: 100.115226, DurationMs: 112.223,
			StartLine: 45696, EndLine: 79136,
		},
	}}
	rows := collectBlockingSpanRows(opendirChimeraIndex(t), stats)
	if len(rows) != 2 {
		t.Fatalf("waiter-identity fold key: two different waiters must keep two rows, got %d", len(rows))
	}
	var lego, victim *blockingSpanRow
	for i := range rows {
		switch rows[i].cand.Thread.PID {
		case 16865:
			lego = &rows[i]
		case 16547:
			victim = &rows[i]
		}
	}
	if lego == nil || victim == nil {
		t.Fatalf("both waiter rows expected: %+v", rows)
	}
	// Self-contradiction guard: LegoHandler's closing waker (the main thread)
	// was itself queued on owner 42067 for ~97% of the span → withdrawn.
	if lego.cand.Peer.PID != 0 || lego.cand.HolderSource != "" {
		t.Fatalf("generalized-arm rows must ride the self-contradiction guard: peer=%+v source=%q",
			lego.cand.Peer, lego.cand.HolderSource)
	}
	if lego.cand.HolderSelfContradiction == "" || !strings.Contains(lego.cand.HolderSelfContradiction, "42067") {
		t.Fatalf("typed contradiction witness expected, got %q", lego.cand.HolderSelfContradiction)
	}
	if lego.cand.OwnerTidRaw != 42067 {
		t.Fatalf("the payload tid stays on the audit field, got %d", lego.cand.OwnerTidRaw)
	}
	// The victim row keeps its own clean inference (no same-lock span of the
	// closing waker rx-777).
	if victim.cand.Peer.PID != 777 || victim.cand.HolderSource != CounterpartSourceWakeupEdge {
		t.Fatalf("victim row inference must survive, got peer=%+v source=%q", victim.cand.Peer, victim.cand.HolderSource)
	}

	// Same-waiter dual print still folds (the fold key carries the waiter).
	dual := WindowStats{TraceSpans: []TraceSpanSummary{
		{
			Name:    "MyRuntime waiting for heap lock (owner tid= 42067)",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.000, EndTs: 100.115944, DurationMs: 115.944,
			StartLine: 44100, EndLine: 79196,
		},
		{
			Name:    "VendorAlias runtime lock stall (owner tid: 42067)",
			Thread:  ThreadRef{Comm: "LegoHandler", PID: 16865, TGID: 16547},
			StartTs: 100.001, EndTs: 100.110000, DurationMs: 109.0,
			StartLine: 44102, EndLine: 79190,
		},
	}}
	folded := collectBlockingSpanRows(opendirChimeraIndex(t), dual)
	if len(folded) != 1 {
		t.Fatalf("same-waiter same-owner dual print must fold to one row, got %d", len(folded))
	}
}
