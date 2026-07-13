package orchestrator

// CR-3 件⑤ annotation-hygiene pins (CR-2 遗留, §29.50,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12):
//
//   a) block-level binding granularity — a binding-arm disclosure whose
//      thread/window came from OUTSIDE the value's own sentence (whole-
//      block fallback) carries 「（块内最近绑定，可能不准）」; a same-
//      sentence binding never does (the 70.338→app-9511 witness).
//   b) 约-值载体归属 — a token grounded only through the approx
//      relative-tolerance band discloses its actual carrier
//      「（约值，载体=发布值 …）」 on any binding finding it produces
//      (the 18.259撞18.283 witness).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cr3MisboundFixture publishes 70.338 for CompThread_0-2955 on a
// thread-bearing evidence row and returns findings for the given prose.
func cr3MisboundFindings(t *testing.T, prose string) []proseScalarBindingFinding {
	t.Helper()
	rec := psgTraceRecord("r1", "wakeup_causal_aggregate:CompThread_0-2955", "70.338")
	rec.Subject = "CompThread_0-2955"
	rec.RichNotes = []string{"state=s_sleep", "impact=70.338ms"}
	mut := psgTraceMutable(rec)
	bus := psgBus(mut)
	doc := psgProseDoc(prose)
	evidence := buildProseScalarEvidenceSet(doc, mut, compileLedgerForTest(bus))
	_, misbound, _, _ := scanProseScalarFindings(doc, evidence)
	return misbound
}

func compileLedgerForTest(bus *types.BusContext) types.ObservationLedger {
	return types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
}

// TestCR3BindingHygiene_BlockFallbackQualifier — the value's sentence
// names NO thread; the only thread token lives in another sentence of the
// same block. The mismatch still discloses (loose), but with the humility
// qualifier.
func TestCR3BindingHygiene_BlockFallbackQualifier(t *testing.T) {
	misbound := cr3MisboundFindings(t,
		"线程 app-9511 的表现值得关注。该睡眠合计 70.338ms 影响显著。")
	if len(misbound) != 1 {
		t.Fatalf("block-fallback misbinding must disclose once, got %+v", misbound)
	}
	zh := misbound[0].userReadable("zh")
	// CR-4 修复轮方向改造: fact form — the line names the value and its
	// PUBLISHING subject (the carrier), never the prose-side binding.
	if !strings.Contains(zh, "70.338ms") || !strings.Contains(zh, "CompThread_0-2955") {
		t.Fatalf("finding must name the token and its publishing subject:\n%s", zh)
	}
	if !strings.Contains(zh, "（块内最近绑定，可能不准）") {
		t.Fatalf("block-level fallback binding must carry the humility qualifier:\n%s", zh)
	}
	if en := misbound[0].userReadable("en"); !strings.Contains(en, "nearest binding within the block; may be imprecise") {
		t.Fatalf("en face must carry the qualifier too:\n%s", en)
	}
}

// TestCR3BindingHygiene_SameSentenceNoQualifier — a same-sentence binding
// stays qualifier-free (the qualifier must mean something).
func TestCR3BindingHygiene_SameSentenceNoQualifier(t *testing.T) {
	misbound := cr3MisboundFindings(t, "线程 app-9511 睡眠合计 70.338ms。")
	if len(misbound) != 1 {
		t.Fatalf("same-sentence misbinding must disclose once, got %+v", misbound)
	}
	if zh := misbound[0].userReadable("zh"); strings.Contains(zh, "块内最近绑定") {
		t.Fatalf("same-sentence binding must NOT carry the block-fallback qualifier:\n%s", zh)
	}
}

// TestCR3BindingHygiene_ApproxCarrierQualifier — the 18.259撞18.283 shape:
// an approx-marked self-derived value rides a near-miss published carrier;
// the binding disclosure names the carrier.
func TestCR3BindingHygiene_ApproxCarrierQualifier(t *testing.T) {
	rec := psgTraceRecord("r1", "wakeup_causal_aggregate:CompThread_0-2955", "18.283")
	rec.Subject = "CompThread_0-2955"
	mut := psgTraceMutable(rec)
	bus := psgBus(mut)
	doc := psgProseDoc("线程 keva-1-17437 的优先级反转合计 约18.259ms。")
	evidence := buildProseScalarEvidenceSet(doc, mut, compileLedgerForTest(bus))
	_, misbound, _, _ := scanProseScalarFindings(doc, evidence)
	if len(misbound) != 1 {
		t.Fatalf("approx-carrier misbinding must disclose once, got %+v", misbound)
	}
	zh := misbound[0].userReadable("zh")
	if !strings.Contains(zh, "约值") || !strings.Contains(zh, "18.283") {
		t.Fatalf("approx-grounded finding must name its actual carrier:\n%s", zh)
	}
	if en := misbound[0].userReadable("en"); !strings.Contains(en, "approximate value; carrier=published 18.283") {
		t.Fatalf("en face must name the carrier too:\n%s", en)
	}
}

// TestCR3BindingHygiene_ApproxAgreeingCarrierSilent — control: an
// approx-marked value whose carrier row AGREES with the sentence thread
// stays silent (the approx band widens detection input, never the
// mismatch criteria).
func TestCR3BindingHygiene_ApproxAgreeingCarrierSilent(t *testing.T) {
	rec := psgTraceRecord("r1", "wakeup_causal_aggregate:CompThread_0-2955", "18.283")
	rec.Subject = "CompThread_0-2955"
	mut := psgTraceMutable(rec)
	bus := psgBus(mut)
	doc := psgProseDoc("线程 CompThread_0-2955 的等待合计 约18.259ms。")
	evidence := buildProseScalarEvidenceSet(doc, mut, compileLedgerForTest(bus))
	_, misbound, _, _ := scanProseScalarFindings(doc, evidence)
	if len(misbound) != 0 {
		t.Fatalf("agreeing approx carrier must stay silent, got %+v", misbound)
	}
}

// TestCR3SelfSumPrefersSameThreadDecomposition — 件⑤ 伴生 (donghu 复放实证
// 2026-07-12): the prose sum 26.488 reproduces both as the coincidental
// cross-subject pair (0.044 + 26.444) and as the true same-thread pair
// (10.424 + 16.064, both published for CompThread's D groups). The
// disclosure formula must print the same-thread decomposition.
func TestCR3SelfSumPrefersSameThreadDecomposition(t *testing.T) {
	comp := func(id, value string) types.ObservationRecord {
		rec := psgTraceRecord(id, "critical_blocking:d_state_or_io_wait", value)
		rec.Subject = "CompThread_0-2955"
		return rec
	}
	other := func(id, subject, value string) types.ObservationRecord {
		rec := psgTraceRecord(id, "wakeup_causal_aggregate:"+subject, value)
		rec.Subject = subject
		return rec
	}
	mut := psgTraceMutable(
		comp("r1", "10.424"), comp("r2", "16.064"),
		other("r3", "udk-irq-12-92", "0.044"), other("r4", "JankManager-9655", "26.444"))
	bus := psgBus(mut)
	doc := psgProseDoc("CompThread_0-2955 两段 D 态阻塞合计 26.488ms。")
	evidence := buildProseScalarEvidenceSet(doc, mut, compileLedgerForTest(bus))
	_, _, _, advisory := scanProseScalarFindings(doc, evidence)
	if len(advisory) == 0 {
		t.Fatalf("self-sum disclosure expected")
	}
	zh := advisory[0].userReadable("zh")
	if !strings.Contains(zh, "10.424 + 16.064") {
		t.Fatalf("the same-thread decomposition must win over the coincidental pair:\n%s", zh)
	}
	if strings.Contains(zh, "0.044") {
		t.Fatalf("the coincidental cross-subject pair must not print:\n%s", zh)
	}
}

// TestCR3SelfSumPoolOnlyPairDegrades — 修复轮 P3③: a sum reproducible only
// through a pool-side value that sits on NO evidence row (row value-cap
// overflow lane) renders the degraded 「未能在证据面复算」 line — never a
// formula built from unverifiable sides.
func TestCR3SelfSumPoolOnlyPairDegrades(t *testing.T) {
	// One wide record: numerals beyond the per-row value cap stay in the
	// membership pool but never land on the row — the pool-only lane.
	wide := psgTraceRecord("r-wide", "wakeup_causal_aggregate:filler-1000", "1.000")
	wide.Subject = "filler-1000"
	var parts []string
	for i := 0; i < 70; i++ {
		parts = append(parts, fmt.Sprintf("k%d=%d.%03d", i, 900+i, i))
	}
	// The pair side 77.111 is the LAST numeral — beyond the 64-value row
	// cap, pool-member only.
	parts = append(parts, "tail=77.111")
	wide.RichNotes = parts
	carrier := psgTraceRecord("r-carrier", "wakeup_causal_aggregate:CompThread_0-2955", "10.000")
	carrier.Subject = "CompThread_0-2955"
	mut := psgTraceMutable(wide, carrier)
	bus := psgBus(mut)
	doc := psgProseDoc("CompThread_0-2955 合计 87.111ms。")
	evidence := buildProseScalarEvidenceSet(doc, mut, compileLedgerForTest(bus))
	_, _, _, advisory := scanProseScalarFindings(doc, evidence)
	if len(advisory) == 0 {
		t.Fatalf("self-sum disclosure expected")
	}
	zh := advisory[0].userReadable("zh")
	if !strings.Contains(zh, "未能在证据面复算") {
		t.Fatalf("pool-only pair must degrade to the no-formula line:\n%s", zh)
	}
	if strings.Contains(zh, "77.111") || strings.Contains(zh, "自行加和（") {
		t.Fatalf("no formula may print off an unverifiable side:\n%s", zh)
	}
}

// TestCR3SelfSumCrossSubjectVerifiedPairKeepsQualifier — 修复轮 P3③
// 正向半场: when the ONLY verified pair is cross-subject, the formula still
// renders (both sides row-verified) and carries the 跨主体 qualifier —
// the qualifier belongs to verified pairs exclusively.
func TestCR3SelfSumCrossSubjectVerifiedPairKeepsQualifier(t *testing.T) {
	a := psgTraceRecord("r-a", "wakeup_causal_aggregate:keva-1-17437", "4.795")
	a.Subject = "keva-1-17437"
	b := psgTraceRecord("r-b", "wakeup_causal_aggregate:keva-3-17439", "6.308")
	b.Subject = "keva-3-17439"
	mut := psgTraceMutable(a, b)
	bus := psgBus(mut)
	doc := psgProseDoc("两处同步读合计 11.103ms。")
	evidence := buildProseScalarEvidenceSet(doc, mut, compileLedgerForTest(bus))
	_, _, _, advisory := scanProseScalarFindings(doc, evidence)
	if len(advisory) == 0 {
		t.Fatalf("self-sum disclosure expected")
	}
	zh := advisory[0].userReadable("zh")
	if !strings.Contains(zh, "4.795 + 6.308") || !strings.Contains(zh, "跨主体") {
		t.Fatalf("a cross-subject VERIFIED pair renders with its qualifier:\n%s", zh)
	}
}
