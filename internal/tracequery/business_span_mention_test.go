package tracequery

// business_span_mention_test.go — SPANVIS-1 engine pins (user ruling
// 2026-07-19 定形原则). Covered:
//   ① admission double gate, positive and negative arms: typed on-chain
//     credential (self / chain member / R3 host edge, incl. the pre-edge
//     boundary discipline; a credential-less thread is NEVER mentioned) ×
//     significance floor (max(dust, window share), both components exercised);
//   ③ value identity: family Count/Σ/max/line-envelope are exactly the
//     inventory member set's values (µs identity — 提及行值=原始段值);
//   ④ truncation honesty: cap keeps the top families by Σ, OmittedFamilies
//     counts ONLY admitted (≥floor) families beyond the cap;
//   plus: fully-visible family admission with HiddenCount==0 (POOL2-1 件①
//     EVOLUTION, §29.160①: the former ≥1-hidden-member gate is removed —
//     提及义务限链上,含自身), fail-open exits (nil inventory / no window),
//     deterministic ordering, and the closed-set basis literal pin the
//     types-side parser mirrors.

import (
	"math"
	"testing"
)

func spanvisSpan(tid int, comm, name string, startTs, durMs float64, startLine, endLine int) TraceSpanSummary {
	return TraceSpanSummary{
		Thread:     ThreadRef{PID: tid, Comm: comm},
		Name:       name,
		StartTs:    startTs,
		EndTs:      startTs + durMs/1000,
		DurationMs: durMs,
		StartLine:  startLine,
		EndLine:    endLine,
	}
}

// spanvisChain — target 100, one chain node 300, one R3 host-edge credential
// for 200 (boundary at 10.050 inside the chain window).
func spanvisChain() ChainResult {
	target := ThreadRef{PID: 100, Comm: "app.main"}
	return ChainResult{
		Target: target,
		Window: TimeWindow{StartTs: 10.0, EndTs: 10.1},
		Nodes:  []ChainNode{{Thread: target}, {Thread: ThreadRef{PID: 300, Comm: "renderer"}}},
		WakeupEdgeCensus: []WakeupEdgeCensusPair{{
			Waker: ThreadRef{PID: 200, Comm: "binder_host"},
			Wakee: target,
			Count: 2, LastTs: 10.050,
		}},
	}
}

func spanvisStats(inventory, bounded []TraceSpanSummary) WindowStats {
	return WindowStats{traceSpanFullInventory: inventory, TraceSpans: bounded}
}

func spanvisQuery() Query {
	// 100ms window → floor = max(0.1, 1%×100) = 1.0ms.
	return Query{PID: 100, TimeStart: 10.0, TimeEnd: 10.1}
}

// TestBusinessSpanMentionAdmissionBases — 准入门①: the three typed credential
// arms admit; a credential-less thread never mentions (硬纪律).
func TestBusinessSpanMentionAdmissionBases(t *testing.T) {
	chain := spanvisChain()
	chainThreads := wakeupChainThreadSet(chain)
	inventory := []TraceSpanSummary{
		spanvisSpan(100, "app.main", "selfSpan", 10.001, 1.5, 10, 20),
		spanvisSpan(300, "renderer", "chainSpan", 10.002, 2.5, 30, 40),
		spanvisSpan(200, "binder_host", "hostSpan", 10.003, 3.5, 50, 60),
		spanvisSpan(999, "stranger", "strangerSpan", 10.004, 50.0, 70, 80),
	}
	res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads, spanvisStats(inventory, nil))
	if res == nil || len(res.Families) != 3 {
		t.Fatalf("expected exactly the three credentialed families, got %+v", res)
	}
	byName := map[string]BusinessSpanMention{}
	for _, fam := range res.Families {
		byName[fam.Name] = fam
	}
	if byName["selfSpan"].OnChainBasis != BusinessSpanMentionBasisSelf {
		t.Fatalf("self arm: %+v", byName["selfSpan"])
	}
	if byName["chainSpan"].OnChainBasis != BusinessSpanMentionBasisChainMember {
		t.Fatalf("chain-member arm: %+v", byName["chainSpan"])
	}
	if byName["hostSpan"].OnChainBasis != BusinessSpanMentionBasisHostWakeupEdge {
		t.Fatalf("host-edge arm: %+v", byName["hostSpan"])
	}
	if _, mentioned := byName["strangerSpan"]; mentioned {
		t.Fatalf("a credential-less thread must NEVER be mentioned regardless of magnitude (硬纪律): %+v", res.Families)
	}
}

// TestBusinessSpanMentionHostEdgeBoundary — R3 边前/边后, whole-segment
// strict (返工轮 P1-1): only members lying ENTIRELY before the latest
// credential edge carry it; a boundary-CROSSING span and a span starting AT
// the boundary are excluded whole (no bisected values on the verbatim face);
// a family whose every member is post-edge is not mentioned.
func TestBusinessSpanMentionHostEdgeBoundary(t *testing.T) {
	chain := spanvisChain() // boundary 10.050
	chainThreads := wakeupChainThreadSet(chain)
	inventory := []TraceSpanSummary{
		spanvisSpan(200, "binder_host", "hostSpan", 10.010, 1.2, 10, 20),  // wholly pre-edge (EndTs 10.0112)
		spanvisSpan(200, "binder_host", "hostSpan", 10.045, 20.0, 22, 28), // CROSSES the edge (EndTs 10.065) — excluded whole
		spanvisSpan(200, "binder_host", "hostSpan", 10.060, 9.0, 30, 40),  // post-edge (excluded)
		spanvisSpan(200, "binder_host", "postOnly", 10.070, 9.0, 50, 60),  // post-edge only family
		spanvisSpan(200, "binder_host", "atEdge", 10.050, 5.0, 70, 80),    // starts AT the boundary — excluded whole
	}
	res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads, spanvisStats(inventory, nil))
	if res == nil || len(res.Families) != 1 {
		t.Fatalf("only the wholly-pre-edge member set may mention: %+v", res)
	}
	fam := res.Families[0]
	if fam.Name != "hostSpan" || fam.Count != 1 || microsBSM(fam.TotalMs) != microsBSM(1.2) ||
		microsBSM(fam.MaxSingleMs) != microsBSM(1.2) || fam.StartLine != 10 || fam.EndLine != 20 {
		t.Fatalf("crossing/post-edge members must not join the family account (整段边前纪律): %+v", fam)
	}
}

// TestBusinessSpanMentionSignificanceFloor — 准入门②, both components:
// window-share floor (1% of window) and absolute dust floor (0.1ms).
func TestBusinessSpanMentionSignificanceFloor(t *testing.T) {
	chain := spanvisChain()
	chainThreads := wakeupChainThreadSet(chain)
	// Window 100ms → floor 1.0ms: a 0.8ms self family stays out, 1.2ms enters.
	inventory := []TraceSpanSummary{
		spanvisSpan(100, "app.main", "below", 10.001, 0.8, 10, 20),
		spanvisSpan(100, "app.main", "above", 10.002, 1.2, 30, 40),
	}
	res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads, spanvisStats(inventory, nil))
	if res == nil || len(res.Families) != 1 || res.Families[0].Name != "above" {
		t.Fatalf("window-share floor arm: %+v", res)
	}
	if res.OmittedFamilies != 0 {
		t.Fatalf("below-floor families never count into the omitted disclosure (微窗裁定): %+v", res)
	}
	// Micro window 2ms → share floor 0.02ms < dust floor 0.1ms: dust wins —
	// a 0.05ms family (2.5% share!) stays out, 0.15ms enters.
	micro := Query{PID: 100, TimeStart: 10.0, TimeEnd: 10.002}
	microChain := spanvisChain()
	microChain.Window = TimeWindow{StartTs: 10.0, EndTs: 10.002}
	microInventory := []TraceSpanSummary{
		spanvisSpan(100, "app.main", "dust", 10.0002, 0.05, 10, 20),
		spanvisSpan(100, "app.main", "real", 10.0004, 0.15, 30, 40),
	}
	microRes := computeBusinessSpanMentions(micro, microChain, wakeupChainThreadSet(microChain), spanvisStats(microInventory, nil))
	if microRes == nil || len(microRes.Families) != 1 || microRes.Families[0].Name != "real" {
		t.Fatalf("dust floor arm: %+v", microRes)
	}
}

// TestBusinessSpanMentionValueIdentity — ③: the family row's values are the
// inventory member set's values exactly (µs identity, 原始值 verbatim).
func TestBusinessSpanMentionValueIdentity(t *testing.T) {
	chain := spanvisChain()
	chainThreads := wakeupChainThreadSet(chain)
	members := []TraceSpanSummary{
		spanvisSpan(100, "app.main", "fam", 10.001, 0.7, 40, 45),
		spanvisSpan(100, "app.main", "fam", 10.010, 1.3, 10, 20),
		spanvisSpan(100, "app.main", "fam", 10.020, 0.4, 90, 95),
	}
	res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads, spanvisStats(members, nil))
	if res == nil || len(res.Families) != 1 {
		t.Fatalf("one family expected: %+v", res)
	}
	fam := res.Families[0]
	if fam.Count != 3 {
		t.Fatalf("count identity: %+v", fam)
	}
	if microsBSM(fam.TotalMs) != microsBSM(0.7+1.3+0.4) {
		t.Fatalf("Σ identity (µs): %+v", fam)
	}
	if microsBSM(fam.MaxSingleMs) != microsBSM(1.3) {
		t.Fatalf("max identity: %+v", fam)
	}
	if fam.StartLine != 10 || fam.EndLine != 95 {
		t.Fatalf("line envelope identity: %+v", fam)
	}
	if fam.HiddenCount != 3 {
		t.Fatalf("no bounded view → every member hidden: %+v", fam)
	}
}

// TestBusinessSpanMentionFullyVisibleFamilyAdmitted — POOL2-1 件① EVOLUTION
// (§29.160/§29.160.1 user ruling 2026-07-20 verbatim: 「这里的 "某个span过长…
// 则需要提及" 指的是链上的(注意 自身也属于链上),如果非链上,对优化方向和建议,
// 以及优化收益都没有太大直接关系,属于噪音。」). This pin REPLACES the former
// TestBusinessSpanMentionHiddenMemberGate: the third admission gate (≥1
// cap-hidden member) is removed, so a fully-visible significant on-chain
// family — here a SELF family (自身也属于链上, the self arm passes exactly
// like the other credential arms) — MUST mention, with HiddenCount==0
// informational; a partially-hidden family keeps the FULL inventory account
// unchanged. Re-adding the old gate turns this red (mutation arm M①).
func TestBusinessSpanMentionFullyVisibleFamilyAdmitted(t *testing.T) {
	chain := spanvisChain()
	chainThreads := wakeupChainThreadSet(chain)
	a := spanvisSpan(100, "app.main", "visible", 10.001, 2.0, 10, 20)
	b := spanvisSpan(100, "app.main", "partial", 10.002, 2.0, 30, 40)
	c := spanvisSpan(100, "app.main", "partial", 10.003, 1.5, 50, 60)
	res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads,
		spanvisStats([]TraceSpanSummary{a, b, c}, []TraceSpanSummary{a, b}))
	if res == nil || len(res.Families) != 2 {
		t.Fatalf("the fully-visible self family must mention post-§29.160① (提及义务限链上,含自身;bounded 视图非提及面): %+v", res)
	}
	byName := map[string]BusinessSpanMention{}
	for _, fam := range res.Families {
		byName[fam.Name] = fam
	}
	visible := byName["visible"]
	if visible.Count != 1 || microsBSM(visible.TotalMs) != microsBSM(2.0) ||
		visible.HiddenCount != 0 || visible.OnChainBasis != BusinessSpanMentionBasisSelf {
		t.Fatalf("fully-visible self family must carry its verbatim account with HiddenCount==0: %+v", visible)
	}
	partial := byName["partial"]
	if partial.Count != 2 || microsBSM(partial.TotalMs) != microsBSM(3.5) || partial.HiddenCount != 1 {
		t.Fatalf("partially-hidden family keeps the FULL inventory account: %+v", partial)
	}
}

// TestBusinessSpanMentionCapAndOmitted — ④: the cap keeps the largest
// totals, OmittedFamilies counts exactly the admitted overflow; ordering is
// total desc with StartLine ties. EVOLUTION RECORD (RULE3-1 件11, §29.185③,
// 2026-07-21): cap 3 → 5 — the fixture grows two overflow families so the
// tail-count arm still bites.
func TestBusinessSpanMentionCapAndOmitted(t *testing.T) {
	chain := spanvisChain()
	chainThreads := wakeupChainThreadSet(chain)
	var inventory []TraceSpanSummary
	names := []string{"f1", "f2", "f3", "f4", "f5", "f6", "f7"}
	for i, name := range names {
		inventory = append(inventory, spanvisSpan(100, "app.main", name, 10.001+float64(i)/1000, float64(7-i)+1.0, 10*(i+1), 10*(i+1)+5))
	}
	// Below-floor extra family — must count nowhere.
	inventory = append(inventory, spanvisSpan(100, "app.main", "dust", 10.09, 0.2, 900, 905))
	res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads, spanvisStats(inventory, nil))
	if res == nil || len(res.Families) != BusinessSpanMentionFamilyCap {
		t.Fatalf("cap must bound the face: %+v", res)
	}
	if res.Families[0].Name != "f1" || res.Families[1].Name != "f2" || res.Families[4].Name != "f5" {
		t.Fatalf("ordering must be total desc: %+v", res.Families)
	}
	if res.OmittedFamilies != 2 {
		t.Fatalf("omitted must count ONLY admitted overflow (f6,f7; dust excluded): %+v", res)
	}
}

// TestBusinessSpanMentionFailOpenExits — nil inventory / no bounded window /
// nothing admissible → nil (absence, never a partial face).
func TestBusinessSpanMentionFailOpenExits(t *testing.T) {
	chain := spanvisChain()
	chainThreads := wakeupChainThreadSet(chain)
	if res := computeBusinessSpanMentions(spanvisQuery(), chain, chainThreads, WindowStats{}); res != nil {
		t.Fatalf("nil inventory must fail open: %+v", res)
	}
	inv := []TraceSpanSummary{spanvisSpan(100, "app.main", "s", 10.001, 5, 10, 20)}
	if res := computeBusinessSpanMentions(Query{PID: 100}, chain, chainThreads, spanvisStats(inv, nil)); res != nil {
		t.Fatalf("windowless query must fail open (no window, no share floor): %+v", res)
	}
	empty := ChainResult{}
	if res := computeBusinessSpanMentions(spanvisQuery(), empty, map[int]bool{}, spanvisStats(inv, nil)); res != nil {
		t.Fatalf("no chain universe → no credential → absence: %+v", res)
	}
}

// TestBusinessSpanMentionBasisLiteralsPinned — the closed-set literals the
// types-side strict parser mirrors (it sits below this package and cannot
// import these constants).
func TestBusinessSpanMentionBasisLiteralsPinned(t *testing.T) {
	if BusinessSpanMentionBasisSelf != "self" ||
		BusinessSpanMentionBasisChainMember != "chain_member" ||
		BusinessSpanMentionBasisHostWakeupEdge != "host_wakeup_edge" {
		t.Fatalf("basis literals drifted — the projection parser's closed set must be updated in lockstep")
	}
	// Floor/cap constants are load-bearing exported contract values.
	// 件11 (§29.185③): the cap is the ruled TOP5.
	if BusinessSpanMentionDustFloorMs != 0.1 || BusinessSpanMentionWindowShareFloor != 0.01 || BusinessSpanMentionFamilyCap != 5 {
		t.Fatalf("significance floor / cap constants drifted (floors measured on the donghu/tieba name census; cap ruled §29.185③ — re-adjudicate before moving)")
	}
}

func microsBSM(ms float64) int64 { return int64(math.Round(ms * 1000)) }

// --- OMGCLEAN-1 件5 rider3 (§29.175.5 定谳, 2026-07-20) ------------------------

// TestBusinessSpanMentionSemanticClassExcluded — 排除臂 (内容互斥定谳): a
// typed semantic_class hit family (VerifyClass etc.) never enters the ◈
// mention face — the value dimension holds seats and the optimization table;
// the name dimension carries only the business spans outside the closed set.
// MUTATION self-check: dropping the exclusion key in
// computeBusinessSpanMentions lets the semantic family seat a mention row —
// this pin reds (语义类混入=红).
func TestBusinessSpanMentionSemanticClassExcluded(t *testing.T) {
	verify := spanvisSpan(100, "app.main", "VerifyClass com.example.Adapter", 10.010, 7.0, 100, 200)
	if TraceSpanSemanticClass(verify.Name) == "" {
		t.Fatalf("fixture: the VerifyClass span must classify (typed exclusion key)")
	}
	business := spanvisSpan(100, "app.main", "Choreographer#doFrame", 10.020, 4.5, 300, 400)
	stamped := spanvisSpan(100, "app.main", "someBusinessWork", 10.030, 3.0, 500, 600)
	stamped.SemanticClass = "jit_compile" // pre-stamped field form of the same key
	inventory := []TraceSpanSummary{verify, business, stamped}
	result := computeBusinessSpanMentions(spanvisQuery(), spanvisChain(), nil, spanvisStats(inventory, nil))
	if result == nil || len(result.Families) != 1 || result.Families[0].Name != "Choreographer#doFrame" {
		t.Fatalf("rider3 排除臂: only the non-semantic business family may mention, got %+v", result)
	}
	if result.OmittedFamilies != 0 {
		t.Fatalf("rider3: excluded semantic families never count into the tail disclosure, got %d", result.OmittedFamilies)
	}
}

// TestBusinessSpanMentionDualCriterionUnion — 双准则并集正臂: the selection is
// 单次最长 TOP5 ∪ 合计最长 TOP5 (deduped) — a family whose SINGLE span is the
// longest but whose total sits below the TotalMs TOP5 still enters (and vice
// versa); reading order stays TotalMs desc. EVOLUTION RECORD (RULE3-1 件11,
// §29.185③, 2026-07-21): the 3+3 union fixture grows to the ruled 5+5 form —
// six high-total families + the single-longest outlier; the family outside
// BOTH TOP5s stays out and counts into the tail.
func TestBusinessSpanMentionDualCriterionUnion(t *testing.T) {
	var inventory []TraceSpanSummary
	// Six high-total families (many small members each).
	totals := []struct {
		name  string
		total float64
	}{{"totalA", 20.0}, {"totalB", 18.0}, {"totalC", 16.0}, {"totalD", 14.0}, {"totalE", 12.0}, {"totalF", 10.0}}
	line := 100
	for _, fam := range totals {
		for i := 0; i < 4; i++ {
			inventory = append(inventory,
				spanvisSpan(100, "app.main", fam.name, 10.001+float64(line)/1e6, fam.total/4, line, line+10))
			line += 20
		}
	}
	// One low-total family carrying the single longest span (9.0 > every
	// member above, total 9.0 < totalF's 10.0).
	inventory = append(inventory, spanvisSpan(100, "app.main", "singleMax", 10.050, 9.0, 5000, 5100))
	result := computeBusinessSpanMentions(spanvisQuery(), spanvisChain(), nil, spanvisStats(inventory, nil))
	if result == nil {
		t.Fatalf("rider3: the union selection must mention")
	}
	names := map[string]bool{}
	for _, fam := range result.Families {
		names[fam.Name] = true
	}
	// TotalMs TOP5 = totalA..E; MaxSingle TOP5 = singleMax(9.0) + totalA(5.0)
	// + totalB(4.5) + totalC(4.0) + totalD(3.5) → union = A..E,singleMax;
	// totalF stays out and counts.
	for _, want := range []string{"totalA", "totalB", "totalC", "totalD", "totalE", "singleMax"} {
		if !names[want] {
			t.Fatalf("件11 并集臂: %s must be selected, got %+v", want, result.Families)
		}
	}
	if names["totalF"] {
		t.Fatalf("件11 并集臂: totalF sits outside both TOP5s and must not render, got %+v", result.Families)
	}
	if result.OmittedFamilies != 1 {
		t.Fatalf("件11 尾部计数臂: admitted−selected must count (want 1), got %d", result.OmittedFamilies)
	}
	// Reading order stays the TotalMs desc order (not an ordinal).
	if result.Families[0].Name != "totalA" || result.Families[len(result.Families)-1].Name != "singleMax" {
		t.Fatalf("件11: reading order must stay TotalMs desc, got %+v", result.Families)
	}
}
