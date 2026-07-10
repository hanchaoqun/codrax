package tool

// answer_document_projection_dispw1_test.go — Wave-1 DISP 批 pins (审计
// findings #5/#62/#63/#6/#56/#58/#59/#60/#66/#61/#64; §29.25 处置委托 +
// §29.26 待主会话落账, 2026-07-10):
//
//   #61  structural priority-inversion rewrite table prefix collision — the
//        候选-suffixed model phrasings must never double into 候选候选;
//   #58  64-block cap: the semantic-optimization slot reservation must never
//        evict MODEL blocks (caveat/decision/summary) — it evicts only a
//        system lossless detail block, else skips + discloses on the caveat
//        lane (系统不可代替 LLM 写用户面板答案);
//   #59  the final hierarchy sort re-orders ONLY system runtime-trace
//        supplements; model narrative keeps its relative order (系统不重排
//        模型叙事), and a model-authored next_steps-shaped ID is never
//        promoted by bare ID;
//   #63  the supplement insertion boundary is the first LOSSLESS detail block
//        — never a projection's own key-metric "_detail" table (§29.10-3
//        lead+关键指标 成对);
//   #60/#66  the deterministic-optimization display identity survives the
//        engine tier retirement on the typed SemanticClass lane;
//   #5   the semantic twin fold's value mirror compares rank participation
//        against the semantic record's typed intersection
//        (SemanticChainProjectedMS) — partial-overlap on-chain families fold
//        into ONE seat;
//   #62① the ✦ 行3 dual-caliber form: 链上计入 X (窗口投影合计 Y 见明细).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- #61: 候选候选 prefix collision ---------------------------------------------

func TestDispW1StructuralInversionCandidateNoDoubledSuffix(t *testing.T) {
	cases := []string{
		"worker-20 的根因是优先级反转候选（lower_priority_waker）",
		"worker-20 的根因候选是优先级反转候选（lower_priority_waker）",
	}
	for _, in := range cases {
		out := normalizeStructuralPriorityInversionCandidateLine(in)
		if strings.Contains(out, "候选候选") {
			t.Fatalf("#61 doubled 候选候选 on the structural lane: %q -> %q", in, out)
		}
		if !strings.Contains(out, "结构上存在低优先级依赖候选") {
			t.Fatalf("#61 structural downgrade wording missing: %q -> %q", in, out)
		}
		// Idempotence: re-running the normalizer never mutates the rewrite.
		if again := normalizeStructuralPriorityInversionCandidateLine(out); again != out {
			t.Fatalf("#61 rewrite must be idempotent: %q -> %q", out, again)
		}
	}
	// The bare-prefix entry keeps its legacy behavior byte-identically.
	if out := normalizeStructuralPriorityInversionCandidateLine("根因是优先级反转"); out != "结构上存在低优先级依赖候选" {
		t.Fatalf("#61 bare prefix entry drifted: %q", out)
	}
}

// --- #58: 64-block cap never evicts model blocks ---------------------------------

func dispW1DocAtCap(lastKind types.AnswerBlockKind, systemDetailID string) *types.AnswerDocumentV2 {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "lead", Kind: types.BlockSummary, Text: "结论"})
	for i := 1; i < maxBlocksPerDoc-2; i++ {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{
			ID: fmt.Sprintf("m%d", i), Kind: types.BlockSection, Text: "模型正文",
		})
	}
	if systemDetailID != "" {
		block := types.AnswerBlock{ID: systemDetailID, Kind: types.BlockBulletList, Text: "system"}
		markRuntimeTraceSystemBlock(&block)
		doc.Blocks = append(doc.Blocks, block)
	} else {
		doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "m_extra", Kind: types.BlockSection, Text: "模型正文"})
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{ID: "model_caveat", Kind: lastKind, Text: "低覆盖披露"})
	return doc
}

func TestDispW1CapReservationNeverEvictsModelBlocks(t *testing.T) {
	doc := dispW1DocAtCap(types.BlockCaveat, "")
	if len(doc.Blocks) != maxBlocksPerDoc {
		t.Fatalf("fixture must sit exactly at the cap: %d", len(doc.Blocks))
	}
	if reserveRuntimeTraceSemanticOptimizationBlockSlot(doc) {
		t.Fatalf("#58: with no replaceable system block the reservation must SKIP, never evict model content")
	}
	if len(doc.Blocks) != maxBlocksPerDoc || doc.Blocks[len(doc.Blocks)-1].ID != "model_caveat" {
		t.Fatalf("#58: the model caveat block must survive untouched: %d blocks, last=%q",
			len(doc.Blocks), doc.Blocks[len(doc.Blocks)-1].ID)
	}
	// The skip discloses on the caveat lane, idempotently.
	runtimeTraceSemanticOptimizationSkipCaveat(doc, true)
	runtimeTraceSemanticOptimizationSkipCaveat(doc, true)
	found := 0
	for _, caveat := range doc.Caveats {
		if strings.Contains(caveat, "确定性优化点汇总表未插入") {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("#58: exactly one skip disclosure caveat expected, got %d: %v", found, doc.Caveats)
	}
}

func TestDispW1CapReservationEvictsOnlySystemLosslessDetail(t *testing.T) {
	// A system lossless detail block yields its seat.
	doc := dispW1DocAtCap(types.BlockCaveat, "runtime_trace_causal_projection_detail_full")
	if !reserveRuntimeTraceSemanticOptimizationBlockSlot(doc) {
		t.Fatalf("#58: a system lossless detail block must yield the slot")
	}
	for _, block := range doc.Blocks {
		if block.ID == "runtime_trace_causal_projection_detail_full" {
			t.Fatalf("#58: the system detail block should have been evicted")
		}
	}
	if doc.Blocks[len(doc.Blocks)-1].ID != "model_caveat" {
		t.Fatalf("#58: the model caveat must survive the system eviction")
	}
	// A system key-metric "_detail" table is a §29.10-3 decision surface —
	// never an eviction candidate (审计 #63 classifier evolution).
	doc = dispW1DocAtCap(types.BlockCaveat, "runtime_trace_causal_projection_detail")
	if reserveRuntimeTraceSemanticOptimizationBlockSlot(doc) {
		t.Fatalf("#58/#63: the key-metric table must not be evicted for the optimization table")
	}
}

// --- #58 复核 R3: skip-caveat reconcile ---------------------------------------------

func TestDispW1SkipCaveatLanguageFlipAndReconcile(t *testing.T) {
	doc := &types.AnswerDocumentV2{DocumentModel: "v2"}
	runtimeTraceSemanticOptimizationSkipCaveat(doc, true)
	runtimeTraceSemanticOptimizationSkipCaveat(doc, false)
	if len(doc.Caveats) != 1 || doc.Caveats[0] != runtimeTraceSemanticOptimizationSkipCaveatText(false) {
		t.Fatalf("R3: a zh↔en flip must upsert ONE current-language caveat, got %v", doc.Caveats)
	}
	doc.Caveats = append(doc.Caveats, "无关披露保留")
	runtimeTraceSemanticOptimizationSkipCaveatReconcile(doc)
	if len(doc.Caveats) != 1 || doc.Caveats[0] != "无关披露保留" {
		t.Fatalf("R3: reconcile must remove only the skip disclosure (both languages), got %v", doc.Caveats)
	}
}

func TestDispW1SkipCaveatRemovedOnceTableInserts(t *testing.T) {
	bus := semanticOptimizationFixtureBus("")
	doc := atCapDoc()
	if materializeRuntimeTraceSemanticOptimizationBlock(doc, bus) {
		t.Fatal("R3 fixture: the at-cap pass must skip")
	}
	if len(doc.Caveats) != 1 || doc.Caveats[0] != runtimeTraceSemanticOptimizationSkipCaveatText(true) {
		t.Fatalf("R3 fixture: the skip must disclose first, got %v", doc.Caveats)
	}
	// A later pass finds headroom (blocks trimmed below the cap): the table
	// inserts and the stale "表未插入" disclosure is reconciled away (§29.24
	// C15 upsert/reconcile precedent).
	doc.Blocks = doc.Blocks[:40]
	if !materializeRuntimeTraceSemanticOptimizationBlock(doc, bus) {
		t.Fatal("R3: the below-cap pass must insert the table")
	}
	if projectionClusterBlock(doc.Blocks, "runtime_trace_semantic_optimizations") == nil {
		t.Fatalf("R3: optimization table missing after the below-cap pass")
	}
	for _, caveat := range doc.Caveats {
		if caveat == runtimeTraceSemanticOptimizationSkipCaveatText(true) ||
			caveat == runtimeTraceSemanticOptimizationSkipCaveatText(false) {
			t.Fatalf("R3: the stale skip disclosure must not ship beside the inserted table: %v", doc.Caveats)
		}
	}
}

// --- #59: model narrative order preserved ----------------------------------------

func TestDispW1HierarchyPreservesModelNarrativeOrder(t *testing.T) {
	system := func(id string) types.AnswerBlock {
		block := types.AnswerBlock{ID: id, Kind: types.BlockBulletList, Text: "system"}
		markRuntimeTraceSystemBlock(&block)
		return block
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		system("runtime_trace_causal_projection_detail_full"), // system detail arrives first in raw order
		{ID: "m_summary", Kind: types.BlockSummary, Text: "结论,细节如下表"},
		{ID: "m_background", Kind: types.BlockSection, Text: "背景"},
		{ID: "next_steps", Kind: types.BlockSection, Text: "模型自己的建议"},
		{ID: "m_table", Kind: types.BlockTable, Text: "分析表"},
		system("runtime_trace_causal_projection"),
		{ID: "m_caveat", Kind: types.BlockCaveat, Text: "范围披露"},
	}}
	normalizeRuntimeTraceReportHierarchy(doc)
	var ids []string
	for _, block := range doc.Blocks {
		ids = append(ids, block.ID)
	}
	got := strings.Join(ids, ",")
	// Model narrative keeps its relative order as ONE body bucket (including
	// the model-authored next_steps-shaped ID — never promoted by bare ID),
	// system lead precedes system detail, trailing caveat closes.
	want := "m_summary,m_background,next_steps,m_table," +
		"runtime_trace_causal_projection,runtime_trace_causal_projection_detail_full,m_caveat"
	if got != want {
		t.Fatalf("#59 narrowed hierarchy broken:\n got %s\nwant %s", got, want)
	}
}

// --- #63: supplement insertion boundary ------------------------------------------

func TestDispW1SupplementBoundaryNeverSplitsLeadMetricPair(t *testing.T) {
	system := func(id string) types.AnswerBlock {
		block := types.AnswerBlock{ID: id, Kind: types.BlockBulletList, Text: "system"}
		markRuntimeTraceSystemBlock(&block)
		return block
	}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{
		{ID: "m_summary", Kind: types.BlockSummary, Text: "结论"},
		system("runtime_trace_causal_projection_a1"),
		system("runtime_trace_causal_projection_a1_detail"),
		system("runtime_trace_causal_projection_a2"),
		system("runtime_trace_causal_projection_a2_detail"),
		system("runtime_trace_causal_projection_a1_detail_full"),
		system("runtime_trace_causal_projection_a1_evidence"),
	}}
	if got := answerDocumentInsertionIndexBeforeRuntimeTraceDetails(doc); got != 5 {
		t.Fatalf("#63: supplements must insert before the first LOSSLESS detail (index 5), got %d", got)
	}
	// The key-metric table is not a detail id; the lossless ones are.
	if runtimeTraceCausalProjectionDetailBlockID("runtime_trace_causal_projection_a1_detail") {
		t.Fatalf("#63: _detail (key metrics) must not classify as a lossless detail id")
	}
	for _, id := range []string{
		"runtime_trace_causal_projection_a1_detail_full",
		"runtime_trace_causal_projection_a1_evidence",
		"runtime_trace_causal_projection_detail_full",
		"runtime_trace_causal_projection_evidence",
		"runtime_trace_causal_projection_compare_notes",
		"runtime_trace_causal_projection_partition",
	} {
		if !runtimeTraceCausalProjectionDetailBlockID(id) {
			t.Fatalf("#63: %q must classify as a lossless detail id", id)
		}
	}
	// The metric classifier still recognizes both forms.
	if !runtimeTraceCausalProjectionMetricBlockID("runtime_trace_causal_projection_a1_detail") ||
		!runtimeTraceCausalProjectionMetricBlockID("runtime_trace_causal_projection_detail") {
		t.Fatalf("#63: metric classifier must keep recognizing the key-metric ids")
	}
}

// --- #60/#66: deterministic-optimization display identity -------------------------

func TestDispW1SemanticIdentityCellsSurviveTierRetirement(t *testing.T) {
	// Post-retirement rank-lane semantic row: predicate root_cause_primary,
	// no semantic Role — the typed SemanticClass carries the identity.
	postRetirement := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Predicate: "root_cause_primary",
		Tier: "primary", SemanticClass: "texture_upload", ChainRelevance: "on_chain",
	}
	if got := runtimeTraceCausalProjectionPriorityCell(postRetirement, true); got != "确定优化" {
		t.Fatalf("#60/#66: rank-lane semantic row lost the priority identity word: %q", got)
	}
	if got := runtimeTraceCausalProjectionLayerCell(postRetirement, true); got != "确定性优化点" {
		t.Fatalf("#60/#66: rank-lane semantic row lost the layer identity word: %q", got)
	}
	// Legacy persisted tier records keep their words byte-identically.
	legacy := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Predicate: "root_cause_deterministic_optimization",
		Tier: "deterministic_optimization",
	}
	if got := runtimeTraceCausalProjectionPriorityCell(legacy, true); got != "确定优化" {
		t.Fatalf("#60/#66: legacy tier row regressed: %q", got)
	}
	if got := runtimeTraceCausalProjectionLayerCell(legacy, true); got != "确定性优化点" {
		t.Fatalf("#60/#66: legacy tier row regressed: %q", got)
	}
	// A non-semantic primary row keeps the root-cause word.
	plain := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, Predicate: "root_cause_primary", Tier: "primary",
	}
	if got := runtimeTraceCausalProjectionPriorityCell(plain, true); got != "主要关注" {
		t.Fatalf("#60/#66: non-semantic primary row must keep 主要关注: %q", got)
	}
}

// --- #5: twin fold intersection mirror --------------------------------------------

func dispW1SemNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, Subject: "worker-200",
		Predicate: "trace_semantic_span", SemanticClass: "texture_upload",
		ChainRelevance: "on_chain", ImpactMS: 9.3, CumulativeImpactMS: 9.3,
		SemanticChainProjectedMS: 5.5, FamilyMemberCount: 2,
		LineStart: 3, LineEnd: 7, EvidenceID: "sem",
	}
}

func dispW1RankNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-200",
		Predicate: "root_cause_primary", SemanticClass: "texture_upload",
		ChainRelevance: "on_chain", ImpactMS: 5.5, EffectiveImpactMS: 5.5,
		CumulativeImpactMS: 9.3, FamilyMemberCount: 2, Rank: 1, Tier: "primary",
		LineStart: 3, LineEnd: 7, EvidenceID: "rank",
	}
}

func TestDispW1PartialOverlapTwinFoldsOnTypedIntersection(t *testing.T) {
	outRank, outSem, peers := runtimeTraceProjFoldSemanticRankLaneTwins(
		[]types.TraceCausalProjectionNode{dispW1RankNode()},
		[]types.TraceCausalProjectionNode{dispW1SemNode()})
	if len(outRank) != 0 || len(peers) != 1 {
		t.Fatalf("#5: the partial-overlap twin (rank participation == typed intersection) must fold: rank=%+v peers=%+v", outRank, peers)
	}
	if outSem[0].Rank != 1 || !runtimeTraceProjRound3Equal(outSem[0].EffectiveImpactMS, 5.5) {
		t.Fatalf("#5: the surviving ✦ seat must adopt the rank ordinal + intersection effective: %+v", outSem[0])
	}
	if !runtimeTraceProjRound3Equal(outSem[0].ImpactMS, 9.3) {
		t.Fatalf("#5: the ✦ row keeps the lossless union on the window-projection lane: %+v", outSem[0])
	}

	// Typed intersection differing from rank participation — never folds.
	sem := dispW1SemNode()
	sem.SemanticChainProjectedMS = 4.4
	if outRank, _, _ := runtimeTraceProjFoldSemanticRankLaneTwins(
		[]types.TraceCausalProjectionNode{dispW1RankNode()},
		[]types.TraceCausalProjectionNode{sem}); len(outRank) != 1 {
		t.Fatalf("#5: an intersection mismatch must fail open")
	}
	// Rank effective disagreeing with the typed intersection — never folds.
	rank := dispW1RankNode()
	rank.EffectiveImpactMS = 6.6
	rank.ImpactMS = 6.6
	if outRank, _, _ := runtimeTraceProjFoldSemanticRankLaneTwins(
		[]types.TraceCausalProjectionNode{rank},
		[]types.TraceCausalProjectionNode{dispW1SemNode()}); len(outRank) != 1 {
		t.Fatalf("#5: a rank-participation mismatch must fail open")
	}
	// No typed intersection → the legacy display mirror decides (partial
	// overlap then honestly keeps two seats — the pre-#5 conservative form).
	sem = dispW1SemNode()
	sem.SemanticChainProjectedMS = 0
	if outRank, _, _ := runtimeTraceProjFoldSemanticRankLaneTwins(
		[]types.TraceCausalProjectionNode{dispW1RankNode()},
		[]types.TraceCausalProjectionNode{sem}); len(outRank) != 1 {
		t.Fatalf("#5: without the typed intersection the legacy display mirror must fail open on union≠intersection")
	}
}

// dispW1SemanticIntersectionProjection — the legend-bidirectional probe
// fixture for the dual-caliber word (#62 ①): a partial-overlap on-chain
// semantic family (intersection 5.500 < union 9.300) whose ✦ 行3 renders
// 链上计入(共2段,同线程) + the 窗口投影合计 disclosure.
func dispW1SemanticIntersectionProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-200", "app-100"},
		WindowStartTs: 5.0,
		WindowEndTs:   5.010,
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "dw1-sem",
			Subject: "worker-200", Predicate: "trace_semantic_span",
			Object: "texture_upload", SemanticClass: "texture_upload",
			SpanName:       "Texture upload(15563) 1140x1140",
			ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
			ImpactMS: 9.3, CumulativeImpactMS: 9.3, EffectiveImpactMS: 5.5,
			SemanticChainProjectedMS: 5.5, Rank: 1,
			FamilyMemberCount: 2, FamilyMemberMaxMS: 7.2, FamilyMemberMinMS: 2.1,
			FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{
				"Texture upload(15563) 1140x1140 7.200ms",
				"Texture upload(15573) 1140x1856 2.100ms",
			},
			LineStart: 3, LineEnd: 7, Confidence: 0.7,
		}},
	}
}

// --- #62 ①: dual-caliber 行3 + published value -------------------------------------

func TestDispW1SemanticDualCaliberRowThreeAndPublishedMS(t *testing.T) {
	folded := dispW1SemNode()
	folded.Rank = 1
	folded.EffectiveImpactMS = 5.5
	folded.FamilyFoldCaliber = "sum_disjoint"
	folded.FamilyMemberRoster = []string{"Texture upload(15563) 1140x1140 7.200ms", "Texture upload(15573) 1140x1856 2.100ms"}
	row := runtimeTraceProjTreeRow{Node: folded, Kind: runtimeTraceProjTreeRowSemantic, HasData: true}
	structured, ok := runtimeTraceProjCauseStructuredParts(row, true)
	if !ok {
		t.Fatalf("#62: the folded ✦ family row must build the cause grammar")
	}
	if !strings.Contains(structured.Breakdown, "有效归因 5.500ms = 链上计入(共2段,同线程)") ||
		!strings.Contains(structured.Breakdown, "(窗口投影合计 9.300ms 见明细)") {
		t.Fatalf("#62: 行3 must speak the dual-caliber form, got %q", structured.Breakdown)
	}
	if strings.Contains(structured.Breakdown, "有效归因 9.300") {
		t.Fatalf("#62/#5: the union must never wear the 有效归因 label: %q", structured.Breakdown)
	}

	// Unfolded remnant (no engine effective): the published value is the typed
	// intersection — never the bare union.
	unfolded := folded
	unfolded.Rank = 0
	unfolded.EffectiveImpactMS = 0
	if v := runtimeTraceProjFamilyPublishedMS(unfolded); !runtimeTraceProjRound3Equal(v, 5.5) {
		t.Fatalf("#5: published participation of an unfolded on-chain semantic family must be the intersection, got %.3f", v)
	}
	structured, ok = runtimeTraceProjCauseStructuredParts(
		runtimeTraceProjTreeRow{Node: unfolded, Kind: runtimeTraceProjTreeRowSemantic, HasData: true}, true)
	if !ok || !strings.Contains(structured.Breakdown, "有效归因 5.500ms = 链上计入(共2段,同线程)") {
		t.Fatalf("#62: the unfolded remnant must keep the dual-caliber intersection claim, got ok=%v %q", ok, structured.Breakdown)
	}

	// Full-overlap control (intersection == union): the legacy fifth caliber
	// word renders byte-identically (§29.22 textup 102.172 witness form).
	full := folded
	full.ImpactMS = 5.5
	full.CumulativeImpactMS = 5.5
	full.SemanticChainProjectedMS = 5.5
	structured, ok = runtimeTraceProjCauseStructuredParts(
		runtimeTraceProjTreeRow{Node: full, Kind: runtimeTraceProjTreeRowSemantic, HasData: true}, true)
	if !ok || !strings.Contains(structured.Breakdown, "有效归因 5.500ms = 合计(共2段,同线程)") {
		t.Fatalf("#62: full-overlap families keep the legacy 合计 word, got ok=%v %q", ok, structured.Breakdown)
	}
	if strings.Contains(structured.Breakdown, "链上计入") {
		t.Fatalf("#62: the dual-caliber word must not fire on full overlap: %q", structured.Breakdown)
	}

	// Engine effective disagreeing with the typed intersection: no "=" claim
	// is fabricated (拒渲绝不造数).
	conflicted := folded
	conflicted.EffectiveImpactMS = 6.6
	structured, _ = runtimeTraceProjCauseStructuredParts(
		runtimeTraceProjTreeRow{Node: conflicted, Kind: runtimeTraceProjTreeRowSemantic, HasData: true}, true)
	if structured.Breakdown != "" {
		t.Fatalf("#62: a conflicted effective must fail open without an equation, got %q", structured.Breakdown)
	}
}
