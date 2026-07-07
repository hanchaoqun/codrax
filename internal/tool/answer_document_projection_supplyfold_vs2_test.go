package tool

// answer_document_projection_supplyfold_vs2_test.go — VS-2 (§7.10)
// presentation pins: the four-branch supply-fold decision table renders as
// (a) the conclusion-line mechanism clause, (b) the node row's Keep/
// Continuation tail tag on the fence, and (c) the lossless detail-table
// mirror — all three from the SAME single-source clause helper. Ranking
// stays rank/attribution-driven (deficit 不参赛), and the RN-1 significance
// arm consumes the shared dual-basis gate (§7.10 同源).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// supplyFoldVS2Records builds a window-anchored (3000ms → shared significance
// gate min(300,100)=100ms) projection with ONE running-dominant folded lead.
func supplyFoldVS2Records(foldNotes ...string) []types.ObservationRecord {
	anchor := types.ObservationRecord{
		ID: "anchor", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard, Predicate: "frame_target_resolution", ClaimKey: "frame_target_resolution:f",
		Subject: "app-100", Object: "frame",
		Span:      types.ObservationSpan{StartTs: 100.0, EndTs: 103.0},
		RichNotes: []string{"window_source=query_window"},
	}
	notes := append([]string{
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running",
	}, foldNotes...)
	lead := projV3Obs("root-worker", "root_cause_primary", "root_cause_primary:worker",
		"worker-200", "running", "20.000", 20.0, 1000, 2000, notes...)
	return []types.ObservationRecord{anchor, lead}
}

func supplyFoldVS2Render(t *testing.T, records []types.ObservationRecord, lang string) string {
	t.Helper()
	return audit730Render(t, audit730Bus(lang), records, lang)
}

// Branch 高∧显著∧反转 — PTV8-RCR-A (§24 ②, 2026-07-08) EVOLUTION RECORD:
// the Triple mechanism sentence (机制构成…优先级反转…gated 口径…) is RETIRED
// on inversion cause nodes — the four-line grammar carries the composition:
// 行2 identity, 行3 「=」breakdown (Σ计入==V machine identity) and the two
// 拆解子行 with per-component calibers (§15.A two-divisor disclosure kept:
// runnable 全额 / running 折算,按下游消费核); the supply-fold deficit keeps a
// lossless home on the detail block's 供给折算 line. "gated" left the user
// face entirely (§24 ④; wire tokens untouched).
func TestSupplyFoldClauseTripleBranchZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	// 行2 renders; the fixture publishes NO engine effective note, so the 行3
	// 有效归因 claim REFUSES to render (显示≠归因: never fabricated from the
	// component sum) and the composition keeps its lossless detail home.
	if !strings.Contains(despaced, "优先级反转候选·根因排序#1·置信高") {
		t.Fatalf("行2 identity line missing:\n%s", md)
	}
	if strings.Contains(despaced, "有效归因5.000ms=") {
		t.Fatalf("unpublished effective must never mint a 行3 total:\n%s", md)
	}
	if !strings.Contains(despaced, "有效归因构成:runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算)") {
		t.Fatalf("composition must keep its lossless detail home:\n%s", md)
	}
	// The supply-fold deficit stays lossless on the detail block (unified
	// sub-row grammar, explicitly outside the attribution).
	if !strings.Contains(despaced, "running原始20.000ms→供给折算缺口5.000ms(折算,按大核满频,下界;独立口径,不计入有效归因)") {
		t.Fatalf("inversion node's supply-fold deficit must keep its lossless detail home:\n%s", md)
	}
	// Conclusion line carries the lead fact + the 行3-form breakdown.
	if !strings.Contains(md, "**主根因:** worker-200") {
		t.Fatalf("lead line missing:\n%s", md)
	}
	// Retirements bite: no mechanism sentence, no user-facing "gated", no
	// summing tail (§7.10 red line 2 unchanged).
	for _, banned := range []string{"机制构成", "gated 口径", "gated口径", "gated 分量", "共同作用", "影响构成"} {
		if strings.Contains(despaced, vs2Despace(banned)) {
			t.Fatalf("retired wording %q leaked:\n%s", banned, md)
		}
	}
	// No synthetic sum of different calibers anywhere (禁合成总分).
	for _, banned := range []string{"157.000", "160.000", "10.000ms(合计", "合计 157", "共计", "小计"} {
		if strings.Contains(md, banned) {
			t.Fatalf("mechanisms must never sum (%q leaked):\n%s", banned, md)
		}
	}
}

func TestSupplyFoldClauseTripleBranchEN(t *testing.T) {
	// PTV8-RCR-A EVOLUTION RECORD (§24 ②/④): EN mirrors the zh grammar —
	// identity line + breakdown + sub-rows; no mechanism sentence, no "gated".
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "en")
	despaced := vs2Despace(md)
	if !strings.Contains(despaced, "root-causerank#1") {
		t.Fatalf("EN identity line missing:\n%s", md)
	}
	if strings.Contains(despaced, "attribution5.000ms=") {
		t.Fatalf("unpublished effective must never mint a 行3 total (EN):\n%s", md)
	}
	if !strings.Contains(despaced, "runnable2.000ms(infull)+discountedrunning3.000ms(foldedatthedownstreamconsumercore)") {
		t.Fatalf("EN composition must keep its lossless detail home:\n%s", md)
	}
	if strings.Contains(md, "机制构成") {
		t.Fatalf("EN surface must not carry zh clause:\n%s", md)
	}
	for _, banned := range []string{"mechanism (each caliber", "gated caliber", "gated component", "acting together"} {
		if strings.Contains(md, banned) {
			t.Fatalf("retired EN wording %q leaked:\n%s", banned, md)
		}
	}
}

// Branch 高∧显著 (no inversion): first two mechanisms only.
func TestSupplyFoldClauseDemandBranchZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000")
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	want := "机制构成(各口径独立、不可加和):供给折算缺口5.000ms(按大核满频折算,下界)·调度压力(需求积压)runnable150.000ms(就绪排队积压口径)"
	if !strings.Contains(despaced, want) {
		t.Fatalf("demand-branch clause missing:\n%s", md)
	}
	if strings.Contains(despaced, "优先级反转") {
		t.Fatalf("non-inversion row must not claim the inversion mechanism:\n%s", md)
	}
	if strings.Contains(md, "共同作用") {
		t.Fatalf("demand branch must not carry the summing tail:\n%s", md)
	}
}

// Branch 高∧不显著: runnable 40ms sits under the shared 100ms gate on a
// 3000ms window — deficit-led wording, no scheduling-pressure claim.
func TestSupplyFoldClauseDeficitDominantBranchZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=40.000")
	md := supplyFoldVS2Render(t, records, "")
	collapsed := rn1CollapseContinuations(md)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: "running 含跑慢成分"
	// → "running 时间含降频/小核导致的跑慢成分" (供给折算族).
	if !strings.Contains(collapsed, "供给折算缺口 5.000ms(按大核满频折算,下界)为主,running 时间含降频/小核导致的跑慢成分") {
		t.Fatalf("deficit-dominant clause missing:\n%s", md)
	}
	if strings.Contains(collapsed, "调度压力(需求积压)(runnable") {
		t.Fatalf("insignificant runnable must not claim scheduling pressure:\n%s", md)
	}
}

// Branch 无缺口 (affirmative, fully-known basis only): the exclusion IS the
// finding — running is true workload.
func TestSupplyFoldClauseNoDeficitAffirmativeZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=0.000", "supply_fold_ideal_ms=20.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000")
	md := supplyFoldVS2Render(t, records, "")
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: "已满频满核(或近满),
	// running 属真实工作量" → "已按大核满频(或接近)运行,无供给缺口,running 为真实工作量" (供给折算族).
	if !strings.Contains(rn1CollapseContinuations(md), "已按大核满频(或接近)运行,无供给缺口,running 为真实工作量") {
		t.Fatalf("affirmative no-deficit annotation missing:\n%s", md)
	}
}

// Branch 数据不全: unknown coverage > 0 without a high deficit — the honest
// non-verdict, never the affirmative claim.
func TestSupplyFoldClauseUnknownBasisZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=0.400", "supply_fold_ideal_ms=19.600",
		"fold_basis=known=10.000ms,unknown=10.000ms",
		"runnable=150.000")
	md := supplyFoldVS2Render(t, records, "")
	collapsed := rn1CollapseContinuations(md)
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: "频点数据不全" →
	// "CPU 频率数据不全" (供给折算族); negative pin migrates to the new
	// affirmative form 已按大核满频 (old 已满频满核 no longer renders anywhere).
	if !strings.Contains(collapsed, "CPU 频率数据不全,无法折算") {
		t.Fatalf("unknown-basis honesty missing:\n%s", md)
	}
	if strings.Contains(collapsed, "已按大核满频") {
		t.Fatalf("partial coverage must never make the affirmative claim:\n%s", md)
	}
}

// No fold notes → no clause anywhere (byte-stability control).
func TestSupplyFoldClauseAbsentWithoutFold(t *testing.T) {
	md := supplyFoldVS2Render(t, supplyFoldVS2Records("runnable=150.000"), "")
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: banned clause words
	// migrate with the clause — 已满频满核→已按大核满频, 频点数据不全→CPU 频率数据不全.
	for _, banned := range []string{"供给折算缺口", "已按大核满频", "CPU 频率数据不全", "机制构成"} {
		if strings.Contains(md, banned) {
			t.Fatalf("clause must only render when the fold ran (%q leaked):\n%s", banned, md)
		}
	}
}

// F-4 (统一复核 2026-07-04): a Triple-branch row's fold clause already embeds
// the D3 inversion composition — the independent "影响构成" tag on the same
// row was a double render (two ↳ continuation lines carrying the same split,
// H5-class inflation). Pin: the triple-mechanism row carries the composition
// text EXACTLY once per surface (conclusion line + fence tag + detail-table
// mirror = 3 total), and the independent tag wording never appears.
// vs2Despace strips ASCII spaces after continuation collapse: the fence wrap
// may split the clause anywhere and the collapse rejoin does not restore the
// boundary space, so exact-count pins compare space-less byte sequences.
func vs2Despace(s string) string {
	return strings.ReplaceAll(rn1CollapseContinuations(s), " ", "")
}

// PTV8-RCR-A EVOLUTION RECORD (§24 ②, supersedes the F-4 suppression pins):
// the independent 影响构成 tag and the Triple clause's embedded composition
// are BOTH retired — the 行3 「=」breakdown is the single composition carrier
// on the fence/conclusion, and the detail block mirrors it via 有效归因构成 +
// 拆解 (single-source builders).
func TestSupplyFoldTripleSuppressesIndependentCompositionTagZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	if strings.Contains(md, "影响构成") || strings.Contains(md, "机制构成") {
		t.Fatalf("retired composition carriers must not render:\n%s", md)
	}
	// No engine effective published → the composition has exactly ONE carrier:
	// the detail block's 有效归因构成 component text (no total claimed).
	if got := strings.Count(despaced, "runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算)"); got != 1 {
		t.Fatalf("composition must appear exactly once (detail block), got %d:\n%s", got, md)
	}
}

func TestSupplyFoldTripleSuppressesIndependentCompositionTagEN(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "en")
	despaced := vs2Despace(md)
	if strings.Contains(md, "composition:") || strings.Contains(md, "mechanism (each caliber") {
		t.Fatalf("retired EN composition carriers must not render:\n%s", md)
	}
	if got := strings.Count(despaced, "runnable2.000ms(infull)+discountedrunning3.000ms(foldedatthedownstreamconsumercore)"); got != 1 {
		t.Fatalf("EN composition must appear exactly once (detail block), got %d:\n%s", got, md)
	}
}

// PTV8-RCR-A EVOLUTION RECORD (supersedes the F-4 non-Triple guard): a
// non-Triple inversion cause node renders the SAME four-line grammar — the
// composition has exactly one fence carrier (行3), the mechanism clause stays
// suppressed on inversion nodes (its deficit lives on the detail 供给折算
// line), and the retired tags never return.
func TestSupplyFoldNonTripleInversionKeepsCompositionTag(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=40.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	if !strings.Contains(despaced, "有效归因构成:runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算)") {
		t.Fatalf("non-triple inversion cause node must keep the composition on the detail block:\n%s", md)
	}
	if strings.Contains(md, "影响构成") || strings.Contains(md, "机制构成") {
		t.Fatalf("retired composition carriers must not render:\n%s", md)
	}
	if !strings.Contains(despaced, "供给折算缺口5.000ms(折算,按大核满频,下界;独立口径,不计入有效归因)") {
		t.Fatalf("the deficit must keep its lossless detail home:\n%s", md)
	}
}

// F2 (RCX² 复核) → PTV8-RCR-A EVOLUTION RECORD: the identity pin now GUARDS
// this corner structurally — the engine's single-source total (37.409) does
// not balance against the %.3f components (20.713+16.697=37.410), so the
// 「=」breakdown REFUSES to render (fail-open, §24.1 恒等式 pin doing its
// job): the row keeps the plain single-source 有效归因37.409ms tag, the
// detail block keeps the composition text (no total claimed), and the
// re-summed 37.410 twin appears nowhere.
func TestSupplyFoldTripleTotalSameSourceAsAttributionTag(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=20.713", "gated_running_deficit=16.697",
		"effective_impact_ms=37.409")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	if strings.Contains(despaced, "ms=runnable(") {
		t.Fatalf("a non-balancing decomposition must never render the 「=」row:\n%s", md)
	}
	if !strings.Contains(despaced, "有效归因37.409ms") {
		t.Fatalf("attribution tag must render the single-source value:\n%s", md)
	}
	// Lossless fallback: the component text (no total claim) on the detail block.
	if !strings.Contains(despaced, "runnable20.713ms(全额)+running折算16.697ms(按下游消费核折算)") {
		t.Fatalf("fail-open shape must keep the composition text on the detail block:\n%s", md)
	}
	if strings.Contains(md, "37.410") {
		t.Fatalf("the re-summed 37.410 twin must not appear anywhere (dual-source 0.001 divergence):\n%s", md)
	}
}

// F2 unit corners: the total consumes EffectiveImpactMS ONLY where the engine
// rank-lane mirror guarantees Effective==gated (gated>0 ∧ non-periodic ∧
// effective published); everywhere else the component sum stands — a periodic
// row's Effective is the VS-1 discount (authoritative even at 0, never the
// gated composite) and a gated=0 inversion's Effective is raw TotalMs.
func TestSupplyFoldInversionGatedTotalSourceUnit(t *testing.T) {
	base := types.TraceCausalProjectionNode{GatedRunnableMS: 20.713, GatedRunningDeficitMS: 16.697}
	n := base
	n.EffectiveImpactMS = 37.409
	if got := runtimeTraceProjInversionGatedTotalMS(n); got != 37.409 {
		t.Fatalf("gated total must mirror the effective single source, got %v", got)
	}
	if got := runtimeTraceProjInversionGatedTotalMS(base); got != base.GatedRunnableMS+base.GatedRunningDeficitMS {
		t.Fatalf("missing effective note must fall back to the component sum, got %v", got)
	}
	p := n
	p.PeriodicSource = true
	if got := runtimeTraceProjInversionGatedTotalMS(p); got != p.GatedRunnableMS+p.GatedRunningDeficitMS {
		t.Fatalf("periodic row must not consume the VS-1 effective as the gated total, got %v", got)
	}
	z := types.TraceCausalProjectionNode{EffectiveImpactMS: 16.0}
	if got := runtimeTraceProjInversionGatedTotalMS(z); got != 0 {
		t.Fatalf("gated=0 must not borrow the raw-TotalMs effective, got %v", got)
	}
}

// 排序不变 (§7.10 red line): a rank=2 row with a huge deficit never outranks
// the engine's rank=1 lead — the fold words a row, it never re-ranks rows.
func TestSupplyFoldDeficitNeverRanks(t *testing.T) {
	records := supplyFoldVS2Records()
	folded := projV3Obs("root-folded", "root_cause_primary", "root_cause_primary:folded",
		"folded-300", "running", "18.000", 18.0, 3000, 4000,
		"rank=2", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain",
		"chain_depth=1", "dominant_state=running",
		"supply_fold_deficit_ms=17.000", "supply_fold_ideal_ms=1.000",
		"fold_basis=known=18.000ms,unknown=0.000ms", "runnable=150.000")
	records = append(records, folded)
	md := supplyFoldVS2Render(t, records, "")
	lead := strings.Index(md, "**主根因:** worker-200")
	if lead < 0 {
		t.Fatalf("rank=1 row must stay the lead regardless of the rank=2 deficit:\n%s", md)
	}
}

// The detail table mirrors the SAME clause losslessly (single-source helper).
func TestSupplyFoldDetailTableMirror(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000")
	md := supplyFoldVS2Render(t, records, "")
	// The clause renders on ALL THREE surfaces: conclusion line + fence tag +
	// detail-table shape cell — one single-source helper, three carriers.
	if got := strings.Count(rn1CollapseContinuations(md), "供给折算缺口 5.000ms(按大核满频折算,下界)"); got < 3 {
		t.Fatalf("clause must carry on conclusion+fence+table (got %d):\n%s", got, md)
	}
}

// Verdict unit pins, including the edges the e2e shapes cannot reach: the
// share/floor boundaries are exact comparisons, a high deficit with partial
// coverage stays a high (lower-bound) verdict, and no anchor window disables
// only the scheduling-pressure arm.
func TestSupplyFoldVerdictUnit(t *testing.T) {
	node := func(deficit, ideal, known, unknown, runnable float64, inversion bool) types.TraceCausalProjectionNode {
		n := types.TraceCausalProjectionNode{
			SupplyFoldComputed: true, SupplyFoldDeficitMS: deficit, SupplyFoldIdealMS: ideal,
			SupplyFoldKnownMS: known, SupplyFoldUnknownMS: unknown, RunnableMS: runnable,
			StateKind: "running", Object: "running",
		}
		if inversion {
			n.Object = "priority_inversion_candidate"
		}
		return n
	}
	if got := runtimeTraceProjSupplyFoldVerdictFor(types.TraceCausalProjectionNode{}, 3000); got != runtimeTraceProjSupplyFoldNone {
		t.Fatalf("no fold → none, got %d", got)
	}
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(5, 15, 20, 0, 150, true), 3000); got != runtimeTraceProjSupplyFoldTriple {
		t.Fatalf("triple branch, got %d", got)
	}
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(5, 15, 20, 0, 150, false), 3000); got != runtimeTraceProjSupplyFoldWithDemand {
		t.Fatalf("demand branch, got %d", got)
	}
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(5, 15, 20, 0, 99.999, false), 3000); got != runtimeTraceProjSupplyFoldDominant {
		t.Fatalf("runnable under the shared gate → dominant branch, got %d", got)
	}
	// Exactly at the 20% share and the 1ms floor: high (≥, precise).
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(4.0, 16.0, 20, 0, 0, false), 3000); got != runtimeTraceProjSupplyFoldDominant {
		t.Fatalf("deficit exactly at 20%% must be high, got %d", got)
	}
	// Just under the share: not high; fully-known basis → affirmative.
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(3.999, 16.001, 20, 0, 0, false), 3000); got != runtimeTraceProjSupplyFoldNoDeficit {
		t.Fatalf("deficit under 20%% on a known basis → affirmative, got %d", got)
	}
	// Under the 1ms floor even at a high share: not high (tiny running).
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(0.9, 0.1, 1.0, 0, 0, false), 3000); got != runtimeTraceProjSupplyFoldNoDeficit {
		t.Fatalf("deficit under the 1ms floor is not high, got %d", got)
	}
	// High deficit with partial coverage: STAYS high (lower bound).
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(5, 15, 10, 10, 0, false), 3000); got != runtimeTraceProjSupplyFoldDominant {
		t.Fatalf("partial coverage must not erase a high deficit, got %d", got)
	}
	// Not high + any unknown coverage: the honest non-verdict.
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(0.4, 19.6, 10, 10, 0, false), 3000); got != runtimeTraceProjSupplyFoldUnknownBasis {
		t.Fatalf("unknown coverage without a high deficit → 数据不全, got %d", got)
	}
	// No anchor window: the runnable arm is off; deficit wording survives.
	if got := runtimeTraceProjSupplyFoldVerdictFor(node(5, 15, 20, 0, 500, false), 0); got != runtimeTraceProjSupplyFoldDominant {
		t.Fatalf("no window → no scheduling-pressure claim, got %d", got)
	}
}

// Producer→consumer round trip: the engine's typed fields publish as exactly
// the three typed notes (zeros included — the affirmative fact must survive),
// a nil basis publishes nothing, and the projection parse reconstructs the
// node fields from those notes verbatim.
func TestSupplyFoldNotesRoundTrip(t *testing.T) {
	impact := tracequery.WakeupCausalImpact{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, OnChain: true,
		ChainDepth: 1, DominantState: "running", RunningMs: 20.0, TotalMs: 20.0,
		SupplyFoldDeficitMs: 0.0, SupplyFoldIdealMs: 20.0,
		SupplyFoldBasis: &tracequery.SupplyFoldBasis{KnownMs: 20.0, UnknownMs: 0.0},
	}
	notes := traceQueryTypedCausalImpactRichNotes(impact)
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"supply_fold_deficit_ms=0.000",
		"supply_fold_ideal_ms=20.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("causal impact notes missing %q:\n%s", want, joined)
		}
	}
	record := types.ObservationRecord{
		Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary", ClaimKey: "root_cause_primary:w",
		Subject: "worker-200", Object: "running", Value: "20.000", Unit: "ms",
		RichNotes: append([]string{"rank=1", "runnable=150.000"}, notes...),
	}
	projection := types.TraceCausalProjectionFromObservationRecords([]types.ObservationRecord{record})
	if len(projection.PrimaryRootCauses) != 1 {
		t.Fatalf("expected one node: %+v", projection)
	}
	node := projection.PrimaryRootCauses[0]
	if !node.SupplyFoldComputed || node.SupplyFoldDeficitMS != 0 || node.SupplyFoldIdealMS != 20.0 ||
		node.SupplyFoldKnownMS != 20.0 || node.SupplyFoldUnknownMS != 0 || node.RunnableMS != 150.0 {
		t.Fatalf("node must reconstruct the fold accounting from typed notes: %+v", node)
	}

	// nil basis → not a single fold note, and the parse keeps Computed=false.
	impact.SupplyFoldBasis = nil
	for _, note := range traceQueryTypedCausalImpactRichNotes(impact) {
		if strings.HasPrefix(note, "supply_fold_") || strings.HasPrefix(note, "fold_basis=") {
			t.Fatalf("nil basis must publish no fold note, got %q", note)
		}
	}

	// Aggregate face uses the same producer helper.
	agg := tracequery.WakeupCausalAggregate{
		Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, ChainDepth: 1,
		OccurrenceCount: 2, DominantState: "running",
		SupplyFoldDeficitMs: 7.5, SupplyFoldIdealMs: 22.5,
		SupplyFoldBasis: &tracequery.SupplyFoldBasis{KnownMs: 28.0, UnknownMs: 2.0},
	}
	aggJoined := strings.Join(traceQueryTypedCausalAggregateRichNotes(agg), "\n")
	if !strings.Contains(aggJoined, "fold_basis=known=28.000ms,unknown=2.000ms") {
		t.Fatalf("aggregate notes missing fold basis:\n%s", aggJoined)
	}
}

// The tag builder emits a demotable tag (PTV4 T1: never elided or shaved —
// on width pressure it moves intact to a subordinate line; same class as the
// D3 composition and the RN-1 occupier roster).
func TestSupplyFoldTagUnit(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 5, SupplyFoldIdealMS: 15,
		SupplyFoldKnownMS: 20, RunnableMS: 150, StateKind: "running", Object: "running",
	}
	tag, ok := runtimeTraceProjSupplyFoldTag(node, 3000, true)
	if !ok || tag.MainRow || strings.TrimSpace(tag.Text) == "" {
		t.Fatalf("supply-fold tag must be a demotable subordinate-lane tag: ok=%t %+v", ok, tag)
	}
	if _, ok := runtimeTraceProjSupplyFoldTag(types.TraceCausalProjectionNode{}, 3000, true); ok {
		t.Fatalf("no fold → no tag")
	}
}
