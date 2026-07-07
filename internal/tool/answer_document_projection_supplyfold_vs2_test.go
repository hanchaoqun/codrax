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

// Branch 高∧显著∧反转: deficit 5/20 (25% ≥ 20%, ≥1ms), runnable 150ms ≥ the
// shared 100ms gate, inversion row → three mechanisms joined, each with its
// own unit, never summed.
func TestSupplyFoldClauseTripleBranchZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	// Q4-G (§12.3/§15.D): the three-caliber perspective — no "+…+…共同作用"
	// summing tail, an explicit 「各口径独立、不可加和」 leader, no-space "·"
	// joiners (F3: the zh within-tag convention — e.g. 周期性信号源…·有效归因X —
	// visually distinct from the between-tag " · "), and each NUMBER's own
	// caliber inline (§15.A two-divisor disclosure; F1: the ruler sits on the
	// component it actually folds — supply deficit at big-cluster fmax, the
	// running-deficit COMPONENT at the downstream consumer core; the runnable
	// component is 全额 and the gated TOTAL wears only the gated-caliber word,
	// never a fold it did not undergo). Despaced pin: the fence wrap may split
	// the clause anywhere.
	want := "机制构成(各口径独立、不可加和):供给折算缺口5.000ms(按大核满频折算,下界)·调度压力(需求积压)runnable150.000ms(就绪排队积压口径)·优先级反转5.000ms(gated口径,内含runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算))"
	despaced := vs2Despace(md)
	if !strings.Contains(despaced, want) {
		t.Fatalf("triple-branch clause missing:\n%s", md)
	}
	// Conclusion line carries the clause attached to the lead fact.
	if !strings.Contains(md, "**主根因:** worker-200") {
		t.Fatalf("lead line missing:\n%s", md)
	}
	// The summing invitation must be gone entirely (§7.10 red line 2).
	if strings.Contains(md, "共同作用") {
		t.Fatalf("the summing tail 共同作用 must not appear (invites the加和 misread):\n%s", md)
	}
	// The top-level calibers join with "·", never "+" — F4: the despaced
	// compare covers BOTH the old no-space form (下界)+ ) and any spaced form
	// (下界) + ) with one pattern.
	if strings.Contains(despaced, "下界)+") || strings.Contains(despaced, "(就绪排队积压口径)+") {
		t.Fatalf("top-level mechanisms must join with '·', never '+':\n%s", md)
	}
	// F1: the gated total is a composite, not a folded value — it must not
	// wear a fold caliber (the consumer-core ruler lives on the running
	// component inside the parenthetical), and the 折算-suffixed mechanism
	// name is retired.
	if strings.Contains(despaced, "优先级反转折算") || strings.Contains(despaced, "优先级反转5.000ms(按下游消费核折算") {
		t.Fatalf("the gated total must not claim a fold it did not undergo:\n%s", md)
	}
	// No synthetic sum of the three mechanisms anywhere (禁合成总分).
	for _, banned := range []string{"157.000", "160.000", "10.000ms(合计", "合计 157", "共计", "小计"} {
		if strings.Contains(md, banned) {
			t.Fatalf("mechanisms must never sum (%q leaked):\n%s", banned, md)
		}
	}
}

func TestSupplyFoldClauseTripleBranchEN(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "en")
	// EN keeps its own within-tag " · " convention (F3 is per-face); despaced
	// pin for wrap safety. F1 mirrors: total = "gated caliber", components
	// carry "(in full)" / "(folded at the downstream consumer core)".
	want := "mechanism(eachcaliberisindependentandnotadditive):supply-folddeficit5.000ms(foldedatbig-clusterfmax,lowerbound)·schedulingpressure(demandbacklog)runnable150.000ms(ready-queuebacklogcaliber)·priorityinversion5.000ms(gatedcaliber,madeofrunnable2.000ms(infull)+discountedrunning3.000ms(foldedatthedownstreamconsumercore))"
	if !strings.Contains(vs2Despace(md), want) {
		t.Fatalf("EN triple-branch clause missing:\n%s", md)
	}
	if strings.Contains(md, "机制构成") {
		t.Fatalf("EN surface must not carry zh clause:\n%s", md)
	}
	if strings.Contains(md, "acting together") {
		t.Fatalf("EN summing tail 'acting together' must be gone:\n%s", md)
	}
	if strings.Contains(md, "priority-inversion discount") {
		t.Fatalf("EN gated total must not wear the retired discount name (F1):\n%s", md)
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
	if !strings.Contains(collapsed, "供给折算缺口 5.000ms(按大核满频折算,下界)为主,running 含跑慢成分") {
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
	if !strings.Contains(rn1CollapseContinuations(md), "已满频满核(或近满),running 属真实工作量") {
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
	if !strings.Contains(collapsed, "频点数据不全,无法折算") {
		t.Fatalf("unknown-basis honesty missing:\n%s", md)
	}
	if strings.Contains(collapsed, "已满频满核") {
		t.Fatalf("partial coverage must never make the affirmative claim:\n%s", md)
	}
}

// No fold notes → no clause anywhere (byte-stability control).
func TestSupplyFoldClauseAbsentWithoutFold(t *testing.T) {
	md := supplyFoldVS2Render(t, supplyFoldVS2Records("runnable=150.000"), "")
	for _, banned := range []string{"供给折算缺口", "已满频满核", "频点数据不全", "机制构成"} {
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

func TestSupplyFoldTripleSuppressesIndependentCompositionTagZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	// The inversion candidate's own gated total (gated 口径, F1 — the total
	// never claims a fold) precedes its internal split, clearly labelled as
	// that node's own decomposition (内含 …). The "+" is scoped to this
	// parenthetical only — the top-level calibers join with "·"; each
	// component wears its own ruler (runnable 全额 / running 折算 按下游消费核).
	if !strings.Contains(despaced, "优先级反转5.000ms(gated口径,内含runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算))") {
		t.Fatalf("triple clause must keep the embedded composition:\n%s", md)
	}
	if strings.Contains(md, "影响构成") {
		t.Fatalf("triple row must not render the independent composition tag too:\n%s", md)
	}
	if got := strings.Count(despaced, "runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算)"); got != 3 {
		t.Fatalf("composition text must appear exactly once per surface (conclusion+fence+table=3), got %d:\n%s", got, md)
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
	// The composition body appears exactly once per surface (conclusion +
	// fence + table = 3), inside the inversion candidate's own gated
	// parenthetical — the only additive "+" in the clause.
	if got := strings.Count(despaced, "runnable2.000ms(infull)+discountedrunning3.000ms(foldedatthedownstreamconsumercore)"); got != 3 {
		t.Fatalf("EN composition text must appear exactly once per surface, got %d:\n%s", got, md)
	}
	if !strings.Contains(despaced, "priorityinversion5.000ms(gatedcaliber,madeofrunnable2.000ms(infull)+discountedrunning3.000ms(foldedatthedownstreamconsumercore))") {
		t.Fatalf("EN triple clause must keep the embedded composition:\n%s", md)
	}
}

// F-4 guard: a folded inversion row whose verdict is NOT Triple (runnable
// under the shared significance gate → deficit-dominant branch, clause
// without composition) KEEPS the independent D3 composition tag — the split
// still has exactly one carrier.
func TestSupplyFoldNonTripleInversionKeepsCompositionTag(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=40.000",
		"gated_runnable=2.000", "gated_running_deficit=3.000")
	records[1].Object = "priority_inversion_candidate"
	md := supplyFoldVS2Render(t, records, "")
	despaced := vs2Despace(md)
	// F1: the independent tag rides the same single-source composition text,
	// so it carries the per-component calibers too (同款除数披露).
	if !strings.Contains(despaced, "影响构成:runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算)") {
		t.Fatalf("non-triple inversion row must keep the independent composition tag:\n%s", md)
	}
	if !strings.Contains(despaced, "供给折算缺口5.000ms(按大核满频折算,下界)为主") {
		t.Fatalf("deficit-dominant clause must render beside it:\n%s", md)
	}
	if got := strings.Count(despaced, "runnable2.000ms(全额)+running折算3.000ms(按下游消费核折算)"); got != 1 {
		t.Fatalf("composition text must appear exactly once (the independent tag), got %d:\n%s", got, md)
	}
}

// F2 (RCX² 复核): the clause's gated total is SAME-SOURCE as the row's
// 有效归因 tag — the engine's full-precision components (a=20.7126,
// b=16.6966) publish as %.3f notes (20.713/16.697) whose re-sum shows 37.410,
// while the engine's own gated total (a+b=37.4092 → the rank-lane mirror
// effective_impact_ms) publishes 37.409: round3(a)+round3(b) != round3(a+b),
// the S1/clamp dual-caliber-leak class. Same row, same quantity, two surfaces
// — both MUST show the engine's single-source 37.409 and the re-summed twin
// must appear nowhere.
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
	if !strings.Contains(despaced, "优先级反转37.409ms(gated口径,内含runnable20.713ms(全额)+running折算16.697ms(按下游消费核折算))") {
		t.Fatalf("clause total must consume the engine's single-source effective value:\n%s", md)
	}
	if !strings.Contains(despaced, "有效归因37.409ms") {
		t.Fatalf("attribution tag must render the same single-source value:\n%s", md)
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
