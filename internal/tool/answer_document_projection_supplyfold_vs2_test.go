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
	want := "机制构成: 供给折算缺口 5.000ms(按大核满频折算,下界)+ 调度压力(需求积压)(runnable 150.000ms)+ 优先级反转(构成: 可运行等待 2.000ms + 运行折算 3.000ms)共同作用"
	if !strings.Contains(rn1CollapseContinuations(md), want) {
		t.Fatalf("triple-branch clause missing:\n%s", md)
	}
	// Conclusion line carries the clause attached to the lead fact.
	if !strings.Contains(md, "**主根因:** worker-200") {
		t.Fatalf("lead line missing:\n%s", md)
	}
	// No synthetic sum of the three mechanisms anywhere (禁合成总分).
	for _, banned := range []string{"157.000", "160.000", "10.000ms(合计", "合计 157"} {
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
	want := "mechanism: supply-fold deficit 5.000ms (folded at big-cluster fmax, lower bound) + scheduling pressure (demand backlog) (runnable 150.000ms) + priority inversion (composition: runnable 2.000ms + discounted running 3.000ms) acting together"
	if !strings.Contains(rn1CollapseContinuations(md), want) {
		t.Fatalf("EN triple-branch clause missing:\n%s", md)
	}
	if strings.Contains(md, "机制构成") {
		t.Fatalf("EN surface must not carry zh clause:\n%s", md)
	}
}

// Branch 高∧显著 (no inversion): first two mechanisms only.
func TestSupplyFoldClauseDemandBranchZH(t *testing.T) {
	records := supplyFoldVS2Records(
		"supply_fold_deficit_ms=5.000", "supply_fold_ideal_ms=15.000",
		"fold_basis=known=20.000ms,unknown=0.000ms",
		"runnable=150.000")
	md := supplyFoldVS2Render(t, records, "")
	collapsed := rn1CollapseContinuations(md)
	want := "机制构成: 供给折算缺口 5.000ms(按大核满频折算,下界)+ 调度压力(需求积压)(runnable 150.000ms)共同作用"
	if !strings.Contains(collapsed, want) {
		t.Fatalf("demand-branch clause missing:\n%s", md)
	}
	if strings.Contains(collapsed, "优先级反转(构成") {
		t.Fatalf("non-inversion row must not claim the inversion mechanism:\n%s", md)
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
	if !strings.Contains(despaced, "优先级反转(构成:可运行等待2.000ms+运行折算3.000ms)") {
		t.Fatalf("triple clause must keep the embedded composition:\n%s", md)
	}
	if strings.Contains(md, "影响构成") {
		t.Fatalf("triple row must not render the independent composition tag too:\n%s", md)
	}
	if got := strings.Count(despaced, "可运行等待2.000ms+运行折算3.000ms"); got != 3 {
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
	// The EN independent tag shares the "composition:" prefix with the
	// embedded clause text, so the pin counts the composition BODY: exactly
	// once per surface.
	if got := strings.Count(despaced, "runnable2.000ms+discountedrunning3.000ms"); got != 3 {
		t.Fatalf("EN composition text must appear exactly once per surface, got %d:\n%s", got, md)
	}
	if !strings.Contains(despaced, "priorityinversion(composition:runnable2.000ms+discountedrunning3.000ms)") {
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
	if !strings.Contains(despaced, "影响构成:可运行等待2.000ms+运行折算3.000ms") {
		t.Fatalf("non-triple inversion row must keep the independent composition tag:\n%s", md)
	}
	if !strings.Contains(despaced, "供给折算缺口5.000ms(按大核满频折算,下界)为主") {
		t.Fatalf("deficit-dominant clause must render beside it:\n%s", md)
	}
	if got := strings.Count(despaced, "可运行等待2.000ms+运行折算3.000ms"); got != 1 {
		t.Fatalf("composition text must appear exactly once (the independent tag), got %d:\n%s", got, md)
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

// The tag builder is Keep + NoTruncate + ContinuationLane — same sanctioned
// overflow class as the D3 composition and the RN-1 occupier roster.
func TestSupplyFoldTagUnit(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 5, SupplyFoldIdealMS: 15,
		SupplyFoldKnownMS: 20, RunnableMS: 150, StateKind: "running", Object: "running",
	}
	tag, ok := runtimeTraceProjSupplyFoldTag(node, 3000, true)
	if !ok || tag.DropOrder != runtimeTraceProjTagKeep || !tag.NoTruncate || !tag.ContinuationLane {
		t.Fatalf("supply-fold tag must be Keep + NoTruncate + ContinuationLane: ok=%t %+v", ok, tag)
	}
	if _, ok := runtimeTraceProjSupplyFoldTag(types.TraceCausalProjectionNode{}, 3000, true); ok {
		t.Fatalf("no fold → no tag")
	}
}
