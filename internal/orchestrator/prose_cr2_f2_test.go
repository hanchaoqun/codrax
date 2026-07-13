package orchestrator

// prose_cr2_f2_test.go — CR-2 组④ 软检测扩容 (ledger §29.49 F-2 附注覆盖诊断,
// 2026-07-12): the 133933 replay's appendix caught only two wording items
// while the prose board mis-seating went entirely unseen. Four gaps, four
// arms, ALL information-only (附注面, zero rewrite, zero hard block —
// §29.42.4/§29.47.1 verbatim):
//   ① P3a monotone arm parses the「N. **#N …」 markdown list form;
//   ② P3b board-member identity: a prose seat subject with no typed seat;
//   ③ P1 (subject, confidence) binding: a confidence transplanted onto a
//     different subject's row (the 0.91 转贴 form) — 修复轮 R-P2-1: rides the
//     ADVISORY (appendix-only) slice, never the misbound/violation lane;
//   ④ prose self-made aggregates: a value that grounds ONLY as the sum of
//     published values is disclosed as 自行加和.
// Every fixture is the 20260712-133933 witness form verbatim.

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// cr2F2BoardLedgerRecords builds the typed board the 133933 engine published:
// #1 CompThread (0.82), #2 JankManager (0.76), #3 CompThread-running (0.91) —
// app-9511 and DetectViewRect hold NO seats (context rows).
func cr2F2BoardLedgerRecords() []types.ObservationRecord {
	seat := func(pos int, subject string, conf float64, notes ...string) types.ObservationRecord {
		rec := psgTraceRecord("trace_query:t#root_cause_rank:"+string(rune('0'+pos)), "root_cause_primary", "36.757", notes...)
		rec.Subject = subject
		rec.Object = "d_state_or_io_wait"
		rec.Confidence = conf
		return rec
	}
	return []types.ObservationRecord{
		seat(1, "CompThread_0-2955", 0.82, "rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=36.757"),
		seat(2, "JankManager-9655", 0.76, "rank=2", "tier=secondary", "chain_relevance=on_chain", "effective_impact_ms=16.687"),
		seat(3, "CompThread_0-2955", 0.91, "rank=3", "tier=tertiary", "chain_relevance=on_chain", "effective_impact_ms=8.294"),
		// app-9511's own context row (seatless; its own confidence is 0.78).
		func() types.ObservationRecord {
			rec := psgTraceRecord("trace_query:t#root_cause_rank:4", "root_cause_context", "20.816",
				"tier=context_only", "effective_impact_ms=20.816")
			rec.Subject = "app-9511"
			rec.Object = "sleep_wait"
			rec.Confidence = 0.78
			return rec
		}(),
	}
}

// 133933 形① pin: the markdown-numbered prose board (「N. **#N …」) with a
// declared key (按 effective_attribution 排列) whose values run
// 36.757→16.687→21.153→14.302 is an internal contradiction the monotone arm
// must now parse.
func TestCR2F2MonotoneArmParsesMarkdownSeatList(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("**卡顿根因排序（按 effective_attribution 排列）**\n\n" +
		"1. **#1 CompThread_0-2955 D/IO 等待 36.757 ms** — channel=on_chain。\n" +
		"2. **#2 JankManager-9655 runnable_wait 16.687 ms** — channel=on_chain。\n" +
		"3. **#3 app-9511 优先级反转聚合 21.153 ms** — channel=on_chain。\n" +
		"4. **#4 DetectViewRect-17679 优先级反转 14.302 ms** — channel=on_chain。\n")
	findings := proseLexiconBoardResidualFindings(doc, bus, mut)
	var hit string
	for _, f := range findings {
		if strings.Contains(f.internal, "declares a sort key") {
			hit = f.internal
		}
	}
	if hit == "" {
		t.Fatalf("the md-list board with a declared key must trip the monotone arm, got %+v", findings)
	}
	if !strings.Contains(hit, "16.687") || !strings.Contains(hit, "21.153") {
		t.Fatalf("the finding must name the out-of-order pair: %s", hit)
	}
}

// 133933 形② pin: prose seats #3/#4 onto subjects holding NO typed board
// seat — the identity gate discloses each once (information only).
func TestCR2F2BoardSeatIdentityGate(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("**卡顿根因排序（按 effective_attribution 排列）**\n\n" +
		"1. **#1 CompThread_0-2955 D/IO 等待 36.757 ms**。\n" +
		"2. **#2 JankManager-9655 runnable_wait 16.687 ms**。\n" +
		"3. **#3 app-9511 优先级反转聚合 21.153 ms**。\n" +
		"4. **#4 DetectViewRect-17679 优先级反转 14.302 ms**。\n")
	findings := proseLexiconBoardResidualFindings(doc, bus, mut)
	var seatFindings []string
	for _, f := range findings {
		if strings.Contains(f.internal, "holds no seat on the measured board") {
			seatFindings = append(seatFindings, f.internal)
		}
	}
	if len(seatFindings) != 2 {
		t.Fatalf("want exactly 2 seat-identity findings (app-9511 + DetectViewRect), got %d: %+v", len(seatFindings), seatFindings)
	}
	joined := strings.Join(seatFindings, " | ")
	if !strings.Contains(joined, "app-9511") || !strings.Contains(joined, "DetectViewRect-17679") {
		t.Fatalf("findings must name the unseated subjects: %s", joined)
	}
	// Seated subjects never trip the gate.
	if strings.Contains(joined, "CompThread_0-2955") || strings.Contains(joined, "JankManager-9655") {
		t.Fatalf("seated subjects must not be flagged: %s", joined)
	}
}

// Control: a prose board whose every seat subject holds a typed seat stays
// silent on the identity gate.
func TestCR2F2BoardSeatIdentityGateSilentWhenSeated(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("根因排序:\n1. **#1 CompThread_0-2955 36.757 ms**。\n2. **#2 JankManager-9655 16.687 ms**。\n")
	for _, f := range proseLexiconBoardResidualFindings(doc, bus, mut) {
		if strings.Contains(f.internal, "holds no seat") {
			t.Fatalf("seated-only boards must stay silent: %s", f.internal)
		}
	}
}

// 133933 形③ pin: confidence=0.91 belongs to the CompThread rank row; prose
// attaches it to app-9511 — the (subject, confidence) binding arm discloses
// the transplant.
func TestCR2F2ConfidenceBindingMismatch(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("#3 app-9511 优先级反转聚合 20.816 ms — priority_inversion_candidate，confidence=0.91，两次出现聚合贡献量高。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range misbound {
		if strings.Contains(f.entry, "confidence 0.91") {
			hit = f.entry
		}
	}
	if hit == "" {
		t.Fatalf("the transplanted confidence must be disclosed, got %+v", misbound)
	}
	// CR-4 修复轮方向改造: fact form — the line states WHERE the confidence
	// is published; it never characterizes the prose's own binding.
	if !strings.Contains(hit, "CompThread_0-2955") {
		t.Fatalf("the finding must name the publishing subject: %s", hit)
	}
}

// Control: a confidence quoted for its own subject stays silent.
func TestCR2F2ConfidenceBindingOwnSubjectSilent(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("#1 CompThread_0-2955 D/IO 等待 36.757 ms — confidence=0.82，最强根因信号。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	for _, f := range misbound {
		if strings.Contains(f.entry, "confidence") {
			t.Fatalf("own-subject confidence must stay silent: %s", f.entry)
		}
	}
}

// 133933 形④ pin: 21.153 = 15.758 + 5.395 (both published) — the sum-only
// grounding now carries a disclosure obligation on the appendix face
// (information only; the grounding itself is unchanged, zero rewrite).
func TestCR2F2SelfSummedAggregateDisclosure(t *testing.T) {
	summed := psgTraceRecord("trace_query:t#wakeup_causal_impact:1", "wakeup_causal_impact:app", "15.758", "impact_ms=5.395")
	summed.Subject = "app-9511" // same-subject summands: the thread-mismatch arm stays silent
	records := append(cr2F2BoardLedgerRecords(), summed)
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	doc := psgProseDoc("app-9511 优先级反转聚合 21.153 ms，两次出现聚合贡献量高于单条。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range misbound {
		if strings.Contains(f.entry, "sum of published values") {
			hit = f.entry
		}
	}
	if hit == "" {
		t.Fatalf("a sum-only grounded value must be disclosed as self-summed, got %+v", misbound)
	}
	if !strings.Contains(hit, "21.153") {
		t.Fatalf("the disclosure must name the value: %s", hit)
	}
	// The grounding itself is unchanged: no ungrounded-token violation fires
	// for the summed value (zero rewrite, information face only).
	if got := psgViolations(runProseScalarGroundingCheck(doc, bus, mut)); len(got) != 0 {
		for _, v := range got {
			if strings.Contains(v.Detail, "21.153") && strings.Contains(v.Detail, "match nothing") {
				t.Fatalf("the sum exemption must keep grounding the value: %+v", got)
			}
		}
	}
}

// --- CR-2 修复轮 (复核+冷读 SHIP-WITH-FIXES, 2026-07-12) --------------------

// R-P2-1 pin: the confidence-binding arm is INFORMATION ONLY — it renders on
// the appendix inputs but never mints a ViolProseScalarUngrounded (which
// would drive a soft rewrite round; §29.47.1 soft-only zero-rewrite).
func TestCR2FixConfidenceBindingNeverMintsViolation(t *testing.T) {
	mut := psgTraceMutable(cr2F2BoardLedgerRecords()...)
	bus := psgBus(mut)
	doc := psgProseDoc("#3 app-9511 优先级反转聚合 20.816 ms — confidence=0.91。")
	if got := psgViolations(runProseScalarGroundingCheck(doc, bus, mut)); len(got) != 0 {
		t.Fatalf("R-P2-1: the confidence-binding arm must never reach the violation lane, got %+v", got)
	}
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	found := false
	for _, f := range misbound {
		if strings.Contains(f.entry, "confidence 0.91") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the finding must still reach the appendix inputs: %+v", misbound)
	}
}

// cr2FixRowCapFiller builds a record whose leading numerals exhaust the
// per-row value cap so the trailing numerals become POOL-ONLY members (the
// shape behind the cold-read C2 fabricated decompositions).
func cr2FixPoolOnlyRecord(id string, poolOnly ...string) types.ObservationRecord {
	notes := make([]string, 0, 70)
	for i := 0; i < 64; i++ {
		notes = append(notes, "filler_"+string(rune('a'+i%26))+"="+strconv.Itoa(90000+i)+".111ms")
	}
	for i, v := range poolOnly {
		notes = append(notes, "tail_"+strconv.Itoa(i)+"="+v+"ms")
	}
	rec := psgTraceRecord(id, "window_stats:x", "", notes...)
	rec.Subject = "filler-77777"
	return rec
}

// C-1① pin (冷读 C2, r2 11.103 形): the rendered formula must be the
// ROW-VERIFIED pair (6.308 + 4.795), never the coincidental pool-only pair
// (0.102 + 11.001) that sorts first.
func TestCR2FixSelfSumFormulaMustBeRowVerified(t *testing.T) {
	records := []types.ObservationRecord{
		cr2FixPoolOnlyRecord("trace_query:t#window_stats:9", "0.102", "11.001"),
		psgTraceRecord("trace_query:t#state_drilldown:1", "state_drilldown:a", "6.308"),
		psgTraceRecord("trace_query:t#state_drilldown:2", "state_drilldown:b", "4.795"),
	}
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	doc := psgProseDoc("间接唤醒总阻塞 11.103ms。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range misbound {
		if strings.Contains(f.entry, "sum of published values") {
			hit = f.entry
		}
	}
	if hit == "" {
		t.Fatalf("the self-sum disclosure must fire, got %+v", misbound)
	}
	if strings.Contains(hit, "0.102") || strings.Contains(hit, "11.001") {
		t.Fatalf("a pool-only coincidental pair must never render as the formula: %s", hit)
	}
	if !strings.Contains(hit, "4.795") || !strings.Contains(hit, "6.308") {
		t.Fatalf("the row-verified pair must render: %s", hit)
	}
}

// C-1③ pin (冷读 C2, 18.259 形): when NO pair is row-verifiable the line
// degrades to 「未能在证据面复算」 with no formula.
func TestCR2FixSelfSumDegradesWhenUnprovable(t *testing.T) {
	records := []types.ObservationRecord{
		cr2FixPoolOnlyRecord("trace_query:t#window_stats:9", "0.260", "17.999"),
	}
	mut := psgTraceMutable(records...)
	bus := psgBus(mut)
	doc := psgProseDoc("总阻塞 18.259ms。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range misbound {
		if strings.Contains(f.entryZH, "18.259") {
			hit = f.entryZH
		}
	}
	if hit == "" {
		t.Fatalf("the disclosure must fire, got %+v", misbound)
	}
	if !strings.Contains(hit, "未能在证据面复算") {
		t.Fatalf("the unprovable decomposition must degrade to the re-derivation wording: %s", hit)
	}
	if strings.Contains(hit, "+") || strings.Contains(hit, "0.260") {
		t.Fatalf("no formula may render for an unprovable decomposition: %s", hit)
	}
}

// C-1④ pin (冷读 B3 形): verified components sourced from a pacing-idle
// publication and disjoint subjects disclose the qualifier.
func TestCR2FixSelfSumDisclosesPacingAndCrossSubject(t *testing.T) {
	pacing := psgTraceRecord("trace_query:t#root_evidence:3", "root_evidence:pacing_idle", "15.758")
	pacing.Object = "pacing_idle"
	pacing.Subject = "aweme-17267"
	other := psgTraceRecord("trace_query:t#state_drilldown:5", "state_drilldown:c", "4.795")
	other.Subject = "keva-1-17437"
	mut := psgTraceMutable(pacing, other)
	bus := psgBus(mut)
	doc := psgProseDoc("间接阻塞合计 20.553ms。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	var hit string
	for _, f := range misbound {
		if strings.Contains(f.entryZH, "20.553") {
			hit = f.entryZH
		}
	}
	if hit == "" {
		t.Fatalf("the disclosure must fire, got %+v", misbound)
	}
	if !strings.Contains(hit, "成分含帧间空闲") || !strings.Contains(hit, "跨主体") {
		t.Fatalf("the pacing/cross-subject qualifiers must render: %s", hit)
	}
}

// C-1② pin (冷读 C1 形): a PERCENT token never enters the thread-binding arm
// — the carrier match is unit-blind (prose 18.2% vs an 18.2ms-family row).
func TestCR2FixPercentTokenSkipsThreadBinding(t *testing.T) {
	rec := psgTraceRecord("trace_query:t#thread_cpu_load:1", "thread_cpu_load:x", "18.2")
	rec.Subject = "OS_FFRT_2_0-19627"
	mut := psgTraceMutable(rec)
	bus := psgBus(mut)
	doc := psgProseDoc("app-9511 占比 18.2%。")
	_, misbound := proseScalarResidualAppendixInputs(doc, bus, mut)
	for _, f := range misbound {
		if strings.Contains(f.entry, "18.2") && strings.Contains(f.entry, "published for thread") {
			t.Fatalf("percent tokens must not thread-bind against unit-blind carriers: %s", f.entry)
		}
	}
	if got := psgViolations(runProseScalarGroundingCheck(doc, bus, mut)); len(got) != 0 {
		for _, v := range got {
			if strings.Contains(v.Detail, "18.2") && strings.Contains(v.Detail, "binds") {
				t.Fatalf("percent thread-binding must not reach the violation lane: %+v", got)
			}
		}
	}
}

// C-1 伴生 pin (复放实锤 2026-07-12): the percent recompute-window arm must
// not claim 「仅能按窗口 ≈0ms 复算」 when the sentence's own window reproduces
// the percent — the denominator collection walks the ascending pool and tiny
// known windows can exhaust its cap before the true denominator (67.4% vs
// 233.190ms witness). Direct helper pin: the guard consults the sentence
// windows against the full value pool, independent of collection order.
func TestCR2FixPercentRecomputeChecksSentenceWindowDirectly(t *testing.T) {
	denominators := []float64{0.046, 0.052, 0.058} // tiny KNOWN windows only (cap-truncated shape)
	windowLengths := []float64{0.046, 0.052, 0.058, 233.190}
	sentWindows := []proseScalarWindowRef{{LengthMS: 233.190, Raw: "233.190ms 的窗口"}}
	values := []float64{0.031, 0.035, 0.039, 157.248} // sorted ascending; 157.248/233.190×100 = 67.43
	if published, bad := proseScalarRecomputeWindowMismatch(denominators, windowLengths, sentWindows, values, 67.4, 0.05); bad {
		t.Fatalf("the sentence window reproduces the percent — no mismatch may be claimed: %s", published)
	}
	// Control (the guard must not swallow TRUE mismatches): without any pool
	// value reproducing the percent against the sentence window, the
	// tiny-denominator mismatch still discloses.
	noMatch := []float64{0.031, 0.035, 0.039} // 157.248 absent
	if _, bad := proseScalarRecomputeWindowMismatch(denominators, windowLengths, sentWindows, noMatch, 67.4, 0.05); !bad {
		t.Fatalf("a genuinely non-reproducible percent must keep its disclosure")
	}
}
