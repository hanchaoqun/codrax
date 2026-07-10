package tool

// answer_document_projection_rcm2_test.go — RCM-2 display-half pins (ledger
// docs/design/real_trace_campaign_20260705.md §24.7.1①/§24.10/§24.12 维度A
// 施工图/§24.22 F6, 2026-07-08): the family-merge contenders' display shape.
//
// Witness anchors (D5 代表性复放对照, dumps t.Log-ed inside the two witness
// tests): cmp_78_01 E27-E42 — sixteen same-thread VerifyClass rows (0.04-2.4ms,
// ≈7.124ms 同线程并集合计) rendered one per line while the comparison overview
// showed only the single largest 2.424ms; opendir_78 E5/E6 — two same-thread
// block_io_by_inode rows (1.136 #3 + 0.462 #8) never merged and never showed
// their inode keys. Post-RCM the engine publishes ONE family record each; this
// batch pins the display: one 行 (行1 类型词+×N+合计值, 行2 身份/榜位, 行3
// 有效归因 V = 合计(共N段,同线程), 子行 roster top-3+counted trailer), one
// key-metric row, one detail stanza (full roster + 区分键), one comparison
// cell (类校验 ×14 合计7.124ms(占其查询窗9%)) and one E# with member_count/
// member_fold_caliber audit tokens.
//
// Mutation self-checks (each verified RED during development, then reverted):
//   M-1 (D1 词禁用): re-enabling the 累计(跨线程) stanza word on family rows
//       (dropping the family case in the stanza cum switch) →
//       TestRCM2FamilyRowNeverWearsCrossThreadCumWord red.
//   M-2 (D3 车道误用): routing runtimeTraceProjLeadSelectionValue family rows
//       through the MergedCount>1 member-MAX discount arm →
//       TestRCM2LeadSelectionValueNeverTakesMergedDiscountLane red.
//   M-3 (D2 恒等破坏): rendering 行3 with V=display impact when the effective
//       channel disagrees at print precision (dropping the balanced guard) →
//       TestRCM2FamilyIdentityFailOpen red.
//   M-4 (roster 折叠去计数): emitting the roster fold trailer without the
//       (成员共N,列M) account → TestRCM2SemanticFamilyFourLineForm red on the
//       counted-trailer pin.
//   M-5 (复核 F-1 嵌合体): dropping the family-lane clear in
//       traceCausalProjectionAggregateSameKind →
//       TestRCM2R2AggregateChimeraCleared AND the types-level
//       TestTraceCausalProjectionR2AggregateClearsFamilyLane red.
//   M-6 (复核 F-2 披露漂移): re-planting the stanza's hand-written Σ note
//       (unconditional 重叠段已并 clause) over the single caliber-forked
//       source → TestRCM2DetailBlockFamilyStanza red.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// rcm2CmpSemanticFamilyProjection is the cmp_78_01 E27-E42 witness shape after
// the engine RCM fold: ONE semantic family record (×14 VerifyClass, 同线程
// sum_disjoint 合计 7.124ms) measured in its own 79.2ms query window while the
// tree is anchored on a different 200ms window (the E5 source-window share
// lane → 占其查询窗9%).
func rcm2CmpSemanticFamilyProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "cmp78-e27",
			Subject: "worker-9", Predicate: "trace_semantic_span",
			Object: "class_verification", SemanticClass: "class_verification",
			SpanName:    "VerifyClass com.demo.Big",
			SupportRefs: []string{"cmp78.systrace:1200-1600"},
			LineStart:   1200, LineEnd: 1600,
			ImpactMS:           7.124,
			EffectiveImpactMS:  7.124,
			BackgroundRank:     1,
			QueryWindowStartTs: 50.0, QueryWindowEndTs: 50.0792,
			FamilyMemberCount: 14, FamilyMemberMaxMS: 2.424, FamilyMemberMinMS: 0.040,
			FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{
				"VerifyClass com.demo.Big 2.424ms",
				"VerifyClass com.demo.Mid 1.900ms",
				"VerifyClass com.demo.Small 0.800ms",
				"VerifyClass com.demo.Tiny 0.500ms",
			},
			Confidence: 0.7,
		}},
	}
}

// rcm2OpendirInodeFamilyProjection is the opendir_78 E5/E6 witness shape after
// the engine RCM fold: ONE generic inode family rank contender (×2
// block_io_by_inode, sum_disjoint 1.136+0.462=1.598ms — the §24.22 M2 witness
// arithmetic) with the typed dev distinguishing key and the per-member inode
// keys on the roster.
func rcm2OpendirInodeFamilyProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"RxComputationT-16816", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "opendir-e5",
			Subject: "RxComputationT-16816", Object: "block_io_by_inode",
			TypeToken: "block_io_by_inode", ChainRelevance: "on_chain",
			ChainDepth: 1, Rank: 3,
			SupportRefs: []string{"opendir78.systrace:900-940"},
			LineStart:   900, LineEnd: 940,
			ImpactMS: 1.598, CumulativeImpactMS: 1.598, EffectiveImpactMS: 1.598,
			FamilyMemberCount: 2, FamilyMemberMaxMS: 1.136, FamilyMemberMinMS: 0.462,
			FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{
				"inode=286395 dev=254:2 1.136ms",
				"inode=300123 dev=254:2 0.462ms",
			},
			Dev: "254:2", Confidence: 0.8,
		}},
	}
}

func rcm2RenderFence(t *testing.T, projection types.TraceCausalProjection, zh bool) (runtimeTraceProjTreeModel, string) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	return model, runtimeTraceProjTreeFence(model, zh)
}

// --- D2 语义形 (cmp_78 witness) + D5 end-to-end dump ---------------------------

func TestRCM2SemanticFamilyFourLineForm(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	model, fence := rcm2RenderFence(t, projection, true)
	t.Logf("cmp_78 E27-E42 witness render (zh fence):\n%s", fence)
	// 行1: 类型词 词位 + ×N 上移 + 合计 value stem + E# (one row for the
	// sixteen-row pre-RCM shape).
	for _, want := range []string{
		"✦ worker-9 · 类校验 ×14",
		"合计7.124ms",
		"[E1]",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("行1 must carry %q:\n%s", want, fence)
		}
	}
	// %基 = 源窗 E5 车道 (witness 9%: 7.124/79.2ms), never the 200ms anchor.
	if !strings.Contains(fence, "  9%") {
		t.Fatalf("the share must divide by the row's own query window (9%%):\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkSemanticSourceWindowShare) {
		t.Fatalf("the source-window share lane must engage (E5 车道)")
	}
	// 行2: 身份行 with the background board seat (§24.10 非链背景综合排序道).
	if !strings.Contains(fence, "语义优化候选·背景榜位#1·置信中") {
		t.Fatalf("行2 must carry 类别·背景榜位#N·置信:\n%s", fence)
	}
	// 行3: the fifth caliber word with the identity V == 发布值.
	if !strings.Contains(fence, "有效归因 7.124ms = 合计(共14段,同线程)") {
		t.Fatalf("行3 must carry the fifth caliber word with V == 发布值:\n%s", fence)
	}
	// 子行: roster top-3 + counted trailer (M-4: dropping the (成员共N,列M)
	// account bites here — roster 折叠必带计数披露).
	for _, want := range []string{
		"成员 VerifyClass com.demo.Big 2.424ms",
		"成员 VerifyClass com.demo.Mid 1.900ms",
		"成员 VerifyClass com.demo.Small 0.800ms",
		"其余 11 项见明细(成员共14,列3)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("family sub-rows must carry %q:\n%s", want, fence)
		}
	}
	// The 4th roster entry stays off the tree (top-3 cap) — its lossless home
	// is the detail stanza (asserted in TestRCM2DetailBlockFamilyStanza).
	if strings.Contains(fence, "com.demo.Tiny") {
		t.Fatalf("the tree roster caps at top-3; the 4th member belongs to the detail stanza:\n%s", fence)
	}
	// EN face symmetry (行3 wording).
	_, fenceEN := rcm2RenderFence(t, projection, false)
	for _, want := range []string{
		"×14",
		"total 7.124ms",
		"attribution 7.124ms = total (14 segments, same thread)",
		"11 more in the detail blocks (14 members, 3 listed)",
	} {
		if !strings.Contains(fenceEN, want) {
			t.Fatalf("EN family form must carry %q:\n%s", want, fenceEN)
		}
	}
}

// --- D2 generic 形 (opendir_78 witness) + D5 end-to-end dump -------------------

func TestRCM2GenericInodeFamilyForm(t *testing.T) {
	projection := rcm2OpendirInodeFamilyProjection()
	_, fence := rcm2RenderFence(t, projection, true)
	t.Logf("opendir_78 E5/E6 witness render (zh fence):\n%s", fence)
	for _, want := range []string{
		"块设备IO(inode) ×2",
		"合计1.598ms",
		"根因排序#3",
		"有效归因 1.598ms = 合计(共2段,同线程)",
		"成员 inode=286395 dev=254:2 1.136ms",
		"成员 inode=300123 dev=254:2 0.462ms",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("generic family form must carry %q:\n%s", want, fence)
		}
	}
	// ≤3 members: no fold trailer (the counted trailer is for real folds only).
	if strings.Contains(fence, "其余") {
		t.Fatalf("a fully-listed roster must not claim a fold:\n%s", fence)
	}
}

// --- D2 单成员退化形 (两行形 = pre-RCM byte-stable) ----------------------------

func TestRCM2SingleMemberSemanticDegradation(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	span := &projection.SemanticSpans[0]
	// The engine's single-member family degrades to the plain per-span record
	// (§24.22 M1 单员族逐字退化) — no member_* notes reach the node.
	span.FamilyMemberCount = 0
	span.FamilyMemberMaxMS = 0
	span.FamilyMemberMinMS = 0
	span.FamilyFoldCaliber = ""
	span.FamilyMemberRoster = nil
	span.BackgroundRank = 0
	_, fence := rcm2RenderFence(t, projection, true)
	if !strings.Contains(fence, "VerifyClass com.demo.Big") {
		t.Fatalf("a single span keeps its span-name 词位:\n%s", fence)
	}
	for _, banned := range []string{"×14", "合计", "= 合计(", "成员 ", "背景榜位#"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("the degenerate single-member form must not carry %q (退化不变体):\n%s", banned, fence)
		}
	}
}

// --- D2 恒等式 fail-open (M-3) -------------------------------------------------

func TestRCM2FamilyIdentityFailOpen(t *testing.T) {
	projection := rcm2OpendirInodeFamilyProjection()
	// Disagreeing effective/impact channels at print precision: the "=" claim
	// must fail open (拒渲绝不造数) — no 行3, the plain effective tag stays.
	projection.OnChainCauses[0].EffectiveImpactMS = 2.0
	_, fence := rcm2RenderFence(t, projection, true)
	if strings.Contains(fence, "= 合计(") {
		t.Fatalf("an unbalanced family must not render the 行3 identity claim:\n%s", fence)
	}
	if !strings.Contains(fence, "有效归因2.000ms") {
		t.Fatalf("the fail-open shape keeps the honest plain effective tag:\n%s", fence)
	}
	// The 行1 value stem stays (the impact channel IS the family fold value).
	if !strings.Contains(fence, "合计1.598ms") {
		t.Fatalf("行1 keeps the family value stem on fail-open:\n%s", fence)
	}
}

// --- D1 禁旧词负向 (F6 收口, M-1) ----------------------------------------------

func TestRCM2FamilyRowNeverWearsCrossThreadCumWord(t *testing.T) {
	// The F6 witness shape: a family total seated on a ▒ background stanza row
	// whose display falls back to the attribution lanes — pre-RCM it wore the
	// 累计(跨线程) word (a same-thread total mislabeled cross-thread).
	family := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "f6-bg",
		Subject: "worker-9", Object: "class_verification",
		SemanticClass: "class_verification", ChainRelevance: "background",
		CumulativeImpactMS: 7.124, EffectiveImpactMS: 7.124,
		FamilyMemberCount: 14, FamilyFoldCaliber: "sum_disjoint",
		FamilyMemberRoster: []string{"VerifyClass a 2.424ms"},
		Confidence:         0.7,
	}
	projection := types.TraceCausalProjection{
		WakeupPath:       []string{"worker-9", "app-100"},
		WindowStartTs:    100.0,
		WindowEndTs:      100.2,
		BackgroundCauses: []types.TraceCausalProjectionNode{family},
	}
	model, fence := rcm2RenderFence(t, projection, true)
	if strings.Contains(fence, "累计(跨线程)") {
		t.Fatalf("F6 negative pin: a family row must NEVER wear the cross-thread word:\n%s", fence)
	}
	if model.Marks.has(runtimeTraceProjMarkStanzaCrossThreadCum) {
		t.Fatalf("F6 negative pin: the cross-thread legend mark must stay silent on family-only stanzas")
	}
	// The family caliber word takes the C00 fallback slot instead.
	if !strings.Contains(fence, "合计(共14段,同线程)") {
		t.Fatalf("the family caliber word must take the fallback slot:\n%s", fence)
	}
	// Same ban with a live window projection + differing cum (the stanza cum
	// tag lane).
	projection.BackgroundCauses[0].ImpactMS = 5.0
	_, fence = rcm2RenderFence(t, projection, true)
	if strings.Contains(fence, "累计(跨线程)") {
		t.Fatalf("F6 negative pin (cum-tag lane): family rows never wear the cross-thread word:\n%s", fence)
	}
}

// --- D1 第五词图例 verbatim + 紧邻 ×N取最大 ------------------------------------

func TestRCM2FifthCaliberLegendVerbatimAndAdjacency(t *testing.T) {
	// Catalog adjacency (structural): the family entry sits IMMEDIATELY after
	// the ×N取最大 entry, same caliber group (§24.12 维度A ③ 消当面矛盾).
	catalog := runtimeTraceProjLegendCatalog()
	mergedMaxIdx, familyIdx := -1, -1
	for i, entry := range catalog {
		switch entry.Mark {
		case runtimeTraceProjMarkMergedMax:
			mergedMaxIdx = i
		case runtimeTraceProjMarkFamilyTotal:
			familyIdx = i
		}
	}
	if mergedMaxIdx < 0 || familyIdx != mergedMaxIdx+1 {
		t.Fatalf("the 合计(共N段,同线程) entry must sit immediately after ×N取最大 (got merged=%d family=%d)", mergedMaxIdx, familyIdx)
	}
	if catalog[familyIdx].Group != catalog[mergedMaxIdx].Group {
		t.Fatalf("the two entries must share the caliber group")
	}
	// Legend wording verbatim (§24.12 施工图 ③ 一字不改).
	wantZH := "- `合计(共N段,同线程)` = 同线程墙钟段求和(重叠段取并集),同线程可加;跨线程仍不可加和。"
	if catalog[familyIdx].ZH != wantZH {
		t.Fatalf("fifth caliber word legend must be the 施工图 verbatim:\n got %q\nwant %q", catalog[familyIdx].ZH, wantZH)
	}
	// Rendered adjacency: a shape carrying BOTH a family row and a cross-thread
	// ×N max fold renders the two entries on adjacent legend lines.
	projection := rcm2OpendirInodeFamilyProjection()
	projection.BackgroundCauses = []types.TraceCausalProjectionNode{{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "bg-maxfold",
		Object: "unknown-thread", ChainRelevance: "background",
		ImpactMS: 42.0, CumulativeImpactMS: 42.0,
		MergedCount: 4, MergedMinMS: 12.0, MergedMaxMS: 42.0,
		MergedSubjects: []string{"bd-1", "bd-2"},
		Confidence:     0.8,
	}}
	model, _ := rcm2RenderFence(t, projection, true)
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	lines := strings.Split(lead, "\n")
	maxLine, familyLine := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "`×N(a–b)取最大`") {
			maxLine = i
		}
		if strings.Contains(line, "`合计(共N段,同线程)`") {
			familyLine = i
		}
	}
	if maxLine < 0 || familyLine != maxLine+1 {
		t.Fatalf("rendered legend must keep the two entries adjacent (max=%d family=%d):\n%s", maxLine, familyLine, lead)
	}
}

// --- D1 union<Σ 行内披露 + max_overlap_fallback 形 ------------------------------

func TestRCM2UnionDisclosureAndMaxFallbackForm(t *testing.T) {
	projection := rcm2OpendirInodeFamilyProjection()
	node := &projection.OnChainCauses[0]
	node.FamilyFoldCaliber = "interval_union"
	node.FamilyMemberSumMS = 2.946
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 2.5, 2.5, 2.5
	_, fence := rcm2RenderFence(t, projection, true)
	t.Logf("union/max specimen form render (zh fence):\n%s", fence)
	if !strings.Contains(fence, "有效归因 2.500ms = 合计(共2段,同线程)(重叠段已并,原始和 2.946ms 见明细)") {
		t.Fatalf("union < Σ must disclose the raw sum inline:\n%s", fence)
	}
	// max_overlap_fallback: 成员最大 word + Σ disclosure + 行1 stem.
	node.FamilyFoldCaliber = "max_overlap_fallback"
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 1.136, 1.136, 1.136
	node.FamilyMemberSumMS = 1.598
	_, fence = rcm2RenderFence(t, projection, true)
	for _, want := range []string{
		"成员最大1.136ms",
		"有效归因 1.136ms = 成员最大(共2段,重叠未拆)(原始和 1.598ms 见明细)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("max-fallback form must carry %q:\n%s", want, fence)
		}
	}
	if strings.Contains(fence, "= 合计(") {
		t.Fatalf("a max-fallback family must never claim the 合计 word:\n%s", fence)
	}
}

// --- D3 lead-selection typed lane + 折价道隔离负向 (M-2) ------------------------

func TestRCM2LeadSelectionValueNeverTakesMergedDiscountLane(t *testing.T) {
	family := types.TraceCausalProjectionNode{
		EffectiveImpactMS: 7.124,
		FamilyMemberCount: 14, FamilyMemberMaxMS: 2.424,
		FamilyFoldCaliber: "sum_disjoint",
	}
	if got := runtimeTraceProjLeadSelectionValue(family); got != 7.124 {
		t.Fatalf("family rows compete with the published participation value, got %v", got)
	}
	// Isolation negative (施工图强制项 ①): even if the display Merged* lane
	// were populated alongside, the family lane wins — the member-MAX discount
	// arm (MergedMaxMS) must never see a family row.
	family.MergedCount = 3
	family.MergedMaxMS = 2.424
	if got := runtimeTraceProjLeadSelectionValue(family); got != 7.124 {
		t.Fatalf("family lane must sit ABOVE the Merged* member-MAX discount arm, got %v", got)
	}
	// Without a published effective the display impact is the published value.
	noEff := types.TraceCausalProjectionNode{
		ImpactMS:          1.598,
		FamilyMemberCount: 2, FamilyFoldCaliber: "sum_disjoint",
	}
	if got := runtimeTraceProjLeadSelectionValue(noEff); got != 1.598 {
		t.Fatalf("family rows without an effective compete with the display impact, got %v", got)
	}
}

// --- D3 对比总览 cell + 零链括注同步 -------------------------------------------

func TestRCM2CompareOptimizationCellFamilyForm(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	model, _ := rcm2RenderFence(t, projection, true)
	cell := runtimeTraceProjCompareOptimizationCell(model, true)
	want := "类校验 ×14 合计7.124ms(占其查询窗9%)"
	if cell != want {
		t.Fatalf("确定性优化点 cell:\n got %q\nwant %q", cell, want)
	}
	note := runtimeTraceProjCompareOptimizationPresenceNote(model, true)
	if !strings.Contains(note, "类校验 ×14 合计7.124ms") {
		t.Fatalf("零链括注 must share the family wording: %q", note)
	}
	// EN symmetry.
	modelEN, _ := rcm2RenderFence(t, projection, false)
	cellEN := runtimeTraceProjCompareOptimizationCell(modelEN, false)
	if !strings.Contains(cellEN, "×14 total 7.124ms") || !strings.Contains(cellEN, "of its query window") {
		t.Fatalf("EN cell must carry the family form: %q", cellEN)
	}
}

// --- D3 关键指标表 family 一行 --------------------------------------------------

func TestRCM2KeyMetricTableFamilyRow(t *testing.T) {
	projection := rcm2OpendirInodeFamilyProjection()
	model, _ := rcm2RenderFence(t, projection, true)
	columns, rows := runtimeTraceProjDetailTable(model, true)
	if len(columns) != 6 || len(rows) == 0 {
		t.Fatalf("detail table must render")
	}
	var familyCell string
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "×2合计") {
			familyCell = row.Cells[0]
		}
	}
	if familyCell == "" {
		t.Fatalf("the (a) table must carry the ×N合计 family token: %+v", rows)
	}
	flags := runtimeTraceProjDetailTableLegendFlagsFor(model, true)
	if !flags.family {
		t.Fatalf("the family legend flag must raise")
	}
	if flags.mergedSum || flags.mergedMax {
		t.Fatalf("family rows must not raise the Merged* legend flags (isolated lanes)")
	}
}

// --- D3 明细块 family 一块 (全量 roster + 区分键 + SumMs + caliber + 窗) --------

func TestRCM2DetailBlockFamilyStanza(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	model, _ := rcm2RenderFence(t, projection, true)
	detail := runtimeTraceProjDetailFullText(model, true)
	t.Logf("cmp_78 witness detail stanza (zh):\n%s", detail)
	for _, want := range []string{
		"家族合并: 合计(共14段,同线程)",
		"单段 0.040–2.424ms",
		"成员: (共14,列4)VerifyClass com.demo.Big 2.424ms;VerifyClass com.demo.Mid 1.900ms;VerifyClass com.demo.Small 0.800ms;VerifyClass com.demo.Tiny 0.500ms",
		"家族窗: 50.000–50.079s",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail stanza must carry %q:\n%s", want, detail)
		}
	}
	// Generic family: distinguishing keys + Σ disclosure.
	// EVOLUTION RECORD (复核 F-2, 2026-07-08, 引 §24.10/§24.22): this pin
	// originally froze the stanza's hand-written Σ note — 「成员最大(共2段,
	// 重叠未拆);原始和 1.598ms 供对照(重叠段已并)」, one line claiming both
	// 未拆 and 已并. The stanza now consumes the single caliber-forked source
	// (runtimeTraceProjFamilySumDetailNote): max arm = bare 供对照, union arm
	// keeps the deduplication clause (both pinned below).
	projection2 := rcm2OpendirInodeFamilyProjection()
	node := &projection2.OnChainCauses[0]
	node.FamilyFoldCaliber = "max_overlap_fallback"
	node.FamilyMemberSumMS = 1.598
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 1.136, 1.136, 1.136
	model2, _ := rcm2RenderFence(t, projection2, true)
	detail2 := runtimeTraceProjDetailFullText(model2, true)
	for _, want := range []string{
		"家族合并: 成员最大(共2段,重叠未拆);原始和 1.598ms 供对照;单段 0.462–1.136ms",
		"成员: (共2,列2)inode=286395 dev=254:2 1.136ms;inode=300123 dev=254:2 0.462ms",
		"区分键: dev=254:2",
	} {
		if !strings.Contains(detail2, want) {
			t.Fatalf("generic family stanza must carry %q:\n%s", want, detail2)
		}
	}
	if strings.Contains(detail2, "重叠段已并") {
		t.Fatalf("F-2 negative: the max arm must not claim 重叠段已并 beside 重叠未拆:\n%s", detail2)
	}
	// Union arm: the deduplication clause stays (single source, caliber fork).
	node.FamilyFoldCaliber = "interval_union"
	node.FamilyMemberSumMS = 2.946
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 2.5, 2.5, 2.5
	model3, _ := rcm2RenderFence(t, projection2, true)
	detail3 := runtimeTraceProjDetailFullText(model3, true)
	if !strings.Contains(detail3, "家族合并: 合计(共2段,同线程);原始和 2.946ms 供对照(重叠段已并);单段 0.462–1.136ms") {
		t.Fatalf("union family stanza must keep the deduplication clause:\n%s", detail3)
	}
}

// --- D4 证据索引 family 审计 token ----------------------------------------------

func TestRCM2EvidenceIndexFamilyAuditTokens(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	blocks := runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{})
	var evidence *types.AnswerBlock
	for i := range blocks {
		if strings.HasSuffix(blocks[i].ID, "_evidence") {
			evidence = &blocks[i]
		}
	}
	if evidence == nil {
		t.Fatalf("evidence index must render")
	}
	joined := evidence.Text
	for _, item := range evidence.Items {
		joined += "\n" + item.Text
	}
	for _, want := range []string{
		"member_count=14",
		"member_fold_caliber=sum_disjoint",
		"member_*=同线程家族合并明细",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("evidence index must carry %q:\n%s", want, joined)
		}
	}
}

// --- D5 C4 系统块 family 分组 ---------------------------------------------------

func TestRCM2C4BlockFamilyGrouping(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	buildRuntimeTraceProjTreeModel(projection, evidence, true)
	_, rows := runtimeTraceSemanticOptimizationParts(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(rows) != 5 {
		t.Fatalf("family grouping = header + 3 member rows + counted fold, got %d rows: %+v", len(rows), rows)
	}
	header := rows[0].Cells
	if header[0] != "类校验 ×14" || header[3] != "合计7.124ms" {
		t.Fatalf("header row must carry 类型词 ×N + 合计 cost: %+v", header)
	}
	if !strings.HasPrefix(rows[1].Cells[0], "· 成员 VerifyClass com.demo.Big") {
		t.Fatalf("member rows follow the header: %+v", rows[1].Cells)
	}
	fold := rows[4].Cells[0]
	if !strings.Contains(fold, "其余 11 项(家族折叠,成员共14,列3") {
		t.Fatalf("the fold row must carry the counted account: %q", fold)
	}
}

// --- 计数语义词 (count_sum 形) ---------------------------------------------------

func TestRCM2CountSumFamilyKeepsCountingWord(t *testing.T) {
	projection := rcm2OpendirInodeFamilyProjection()
	node := &projection.OnChainCauses[0]
	node.Object, node.TypeToken = "state_churn", "state_churn"
	node.FamilyFoldCaliber = "count_sum"
	node.FamilyMemberCount = 3
	node.FamilyMemberRoster = []string{"churn a 2", "churn b 2", "churn c 1"}
	node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS = 5.0, 5.0, 5.0
	node.FamilyMemberSumMS = 0
	node.FamilyMemberMaxMS, node.FamilyMemberMinMS = 0, 0
	_, fence := rcm2RenderFence(t, projection, true)
	if !strings.Contains(fence, "有效归因 5.000ms = 计数合计(共3项,同线程)") {
		t.Fatalf("count_sum families keep the counting-semantics word:\n%s", fence)
	}
	if strings.Contains(fence, "段,同线程") {
		t.Fatalf("the wall-clock segment word must not gloss a count family:\n%s", fence)
	}
}

// --- 行1 词位/×N chip survives the width fit (KeepSuffix lane) -------------------

func TestRCM2FamilyXNChipSurvivesNameSqueeze(t *testing.T) {
	projection := rcm2OpendirInodeFamilyProjection()
	node := &projection.OnChainCauses[0]
	node.Subject = "an-extremely-long-thread-name-that-overflows-the-cell-99123"
	projection.WakeupPath = []string{node.Subject, "app-100"}
	_, fence := rcm2RenderFence(t, projection, true)
	if !strings.Contains(fence, " ×2") {
		t.Fatalf("the family ×N chip is grammar and survives the name squeeze:\n%s", fence)
	}
	if !strings.Contains(fence, "合计1.598ms") {
		t.Fatalf("the value stem survives the squeeze too:\n%s", fence)
	}
}

// --- D1/D3 对比背景 cell family 形 (COV-2 回退臂的 F6 收口) ----------------------

func TestRCM2CompareBackgroundTopRowCellFamilyForm(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		BackgroundCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "bg-fam",
			Subject: "worker-9", Object: "class_verification",
			SemanticClass: "class_verification", ChainRelevance: "background",
			CumulativeImpactMS: 7.124, EffectiveImpactMS: 7.124,
			FamilyMemberCount: 14, FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{"VerifyClass a 2.424ms"},
			Confidence:         0.7,
		}},
	}
	model, _ := rcm2RenderFence(t, projection, true)
	cell, ok := runtimeTraceProjCompareBackgroundTopRowCell(model, true)
	if !ok {
		t.Fatalf("the background fallback cell must render")
	}
	if strings.Contains(cell, "累计(跨线程)") || strings.Contains(cell, "单项最大") {
		t.Fatalf("F6: a family total must wear neither 累计(跨线程) nor 单项最大: %q", cell)
	}
	for _, want := range []string{"×14", "7.124ms", "合计(共14段,同线程)"} {
		if !strings.Contains(cell, want) {
			t.Fatalf("the family background cell must carry %q: %q", want, cell)
		}
	}
}

// --- lead conclusion family form (零链语义回退句同步) ---------------------------

func TestRCM2SemanticLeadTextFamilyForm(t *testing.T) {
	projection := rcm2CmpSemanticFamilyProjection()
	model, _ := rcm2RenderFence(t, projection, true)
	node, _, ok := runtimeTraceProjSemanticTopSpan(model)
	if !ok || node == nil {
		t.Fatalf("the off-chain semantic family must remain reachable through the shared optimization selector: %+v", model)
	}
	text := runtimeTraceProjSemanticLeadText(*node, model, true)
	if !strings.Contains(text, "类校验 ×14 合计7.124ms") {
		t.Fatalf("the semantic-fallback conclusion must speak the family form: %q", text)
	}
	if strings.Contains(text, fmt.Sprintf("VerifyClass com.demo.Big %.3fms", 7.124)) {
		t.Fatalf("one member's span name must not impersonate the family: %q", text)
	}
}

// --- 复核 F-1: R2 ×N 嵌合体负向 (双 ×N 车道禁同行) -------------------------------

// TestRCM2R2AggregateChimeraCleared (复核 F-1, 2026-07-08): ≥3 same-(subject,
// object) rows — two of them ENGINE family contenders (multi-window
// same-(thread,type) families make this production-reachable) — R2-fold into
// ONE ×N SUM row. Pre-fix the group-first seed's FamilyMember*/caliber/roster/
// BackgroundRank/Inode/Dev survived the fold wholesale and the render carried
// BOTH ×N lanes (行1 「×2 合计6.598」 beside the subordinate
// 「×3(1.598–3.000ms)」). Post-fix the merged row is a PURE R2 form: no 合计
// stem, no roster sub-rows, no seat/keys — the family lane is cleared at the
// aggregation site (DuplicatePublications/SupplyFold precedent).
// Mutation M-5 (verified red, then reverted): dropping the family-lane clear
// in traceCausalProjectionAggregateSameKind reds every arm below.
func TestRCM2R2AggregateChimeraCleared(t *testing.T) {
	familyNotes := func(rank int) []string {
		return []string{
			fmt.Sprintf("rank=%d", rank),
			"type=block_io_by_inode",
			"chain_relevance=on_chain",
			"member_count=2",
			"member_max_ms=1.136",
			"member_min_ms=0.462",
			"member_fold_caliber=sum_disjoint",
			"member_roster=inode=286395 dev=254:2 1.136ms | inode=300123 dev=254:2 0.462ms",
			"dev=254:2",
			"background_rank=2",
		}
	}
	record := func(id string, value string, lineStart, lineEnd int, notes []string) types.ObservationRecord {
		return types.ObservationRecord{
			ID:              id,
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Subject:         "RxComputationT-16816",
			Predicate:       "root_cause_secondary",
			ClaimKey:        "root_cause_secondary",
			Object:          "block_io_by_inode",
			Value:           value,
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: lineStart, LineEnd: lineEnd},
			SupportRefs:     []string{fmt.Sprintf("cmp78.systrace:%d-%d", lineStart, lineEnd)},
			RichNotes:       notes,
		}
	}
	// The FAMILY record carries the LARGEST impact so the impact-major bucket
	// sort makes it the R2 group-first seed — exactly the chimera-inheritance
	// shape the review caught (a plain group-first would mask the leak).
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{
		record("trace_query:w1#root_cause_rank:1", "3.598", 100, 120, familyNotes(3)),
		record("trace_query:w2#root_cause_rank:1", "2.000", 300, 320, familyNotes(5)),
		record("trace_query:w3#root_cause_rank:1", "1.000", 500, 520, []string{
			"rank=7", "type=block_io_by_inode", "chain_relevance=on_chain"}),
	}}
	projection := types.CompileTraceCausalProjection(ledger)
	var merged *types.TraceCausalProjectionNode
	for i := range projection.OnChainCauses {
		if projection.OnChainCauses[i].MergedCount == 3 {
			merged = &projection.OnChainCauses[i]
		}
	}
	if merged == nil {
		t.Fatalf("the three same-kind rows must R2-fold: %+v", projection.OnChainCauses)
	}
	if merged.FamilyMemberCount != 0 || merged.FamilyFoldCaliber != "" ||
		len(merged.FamilyMemberRoster) != 0 || merged.FamilyMemberSumMS != 0 ||
		merged.BackgroundRank != 0 || merged.Inode != "" || merged.Dev != "" {
		t.Fatalf("CHIMERA: the R2 aggregate must clear the family lane wholesale: %+v", merged)
	}
	_, fence := rcm2RenderFence(t, projection, true)
	if !strings.Contains(fence, "×3(1.000–3.598ms)") {
		t.Fatalf("the merged row keeps the pure R2 ×3 form:\n%s", fence)
	}
	for _, banned := range []string{"合计", "成员 ", "背景榜位", "×2"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("chimera token %q must not render on the R2 fold:\n%s", banned, fence)
		}
	}
}

// --- 复核 F-3: 审计 token 最坏前缀存活 -------------------------------------------

// TestRCM2AuditTokensSurviveWorstCasePrefix (复核 F-3, 2026-07-08): the worst
// REAL prefix — tier=deterministic_optimization + causality=
// adjacent_to_wakeup_chain + rank + confidence — plus the longest caliber
// token (max_overlap_fallback) must survive the widened 160-rune FamilyAudit
// ceiling whole; member_* sits right after confidence, BEFORE the free-length
// predicate/span parts (the fix direction: reorder over ceiling growth,
// keeping the display line bounded).
func TestRCM2AuditTokensSurviveWorstCasePrefix(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Tier:              "deterministic_optimization",
		Causality:         "adjacent_to_wakeup_chain",
		Rank:              12,
		Confidence:        0.75,
		Predicate:         "trace_semantic_span",
		SpanName:          "VerifyClass com.example.app.startup.VeryLongGeneratedClassName$Inner",
		SemanticClass:     "class_verification",
		FamilyMemberCount: 14,
		FamilyFoldCaliber: "max_overlap_fallback",
	}
	details := runtimeTraceCausalProjectionAuditDetail(node, true, false)
	capped := runtimeTraceCausalProjectionAuditCellText(details, 160)
	for _, want := range []string{"member_count=14", "member_fold_caliber=max_overlap_fallback"} {
		if !strings.Contains(capped, want) {
			t.Fatalf("the family audit tokens must survive the worst-case prefix cut:\ncapped: %q\nfull:  %q", capped, details)
		}
	}
	// The ptv7 audit order pin holds: predicate still precedes span.
	if strings.Index(details, "predicate=") > strings.Index(details, "span=") {
		t.Fatalf("predicate part must still precede the span part: %q", details)
	}
}

// --- 复核 F-4: ✦ 背景榜位产线可铸端到端 ------------------------------------------

// TestRCM2BackgroundSeatMintableEndToEnd (复核 F-4, 2026-07-08): the ✦
// observation channel's family record now EMITS the background_rank note
// (traceQuerySemanticSpanFamilyObservation) — pre-fix the note was never
// published, node.BackgroundRank stayed 0 in production, and the witness
// 「背景榜位#1」 seat was unmintable. Full mint path: producer record →
// registered-note parse (CompileTraceCausalProjection) → 行2 render.
func TestRCM2BackgroundSeatMintableEndToEnd(t *testing.T) {
	fam := tracequery.SemanticSpanFamily{
		Thread:        tracequery.ThreadRef{Comm: "worker", PID: 9},
		SemanticClass: "class_verification",
		OnChain:       false,
		TotalMs:       7.124, SumMs: 0, MaxMs: 4.700, MinMs: 2.424,
		StartTs: 50.0, EndTs: 50.0792,
		StartLine: 1200, EndLine: 1600,
		FoldCaliber: "sum_disjoint",
		Members: []tracequery.TraceSpanSummary{
			{Thread: tracequery.ThreadRef{Comm: "worker", PID: 9}, Name: "VerifyClass com.demo.Big",
				SemanticClass: "class_verification", DurationMs: 4.700, StartTs: 50.0, EndTs: 50.0047,
				StartLine: 1200, EndLine: 1300},
			{Thread: tracequery.ThreadRef{Comm: "worker", PID: 9}, Name: "VerifyClass com.demo.Mid",
				SemanticClass: "class_verification", DurationMs: 2.424, StartTs: 50.01, EndTs: 50.0124,
				StartLine: 1400, EndLine: 1600},
		},
	}
	record := traceQuerySemanticSpanFamilyObservation(fam, nil, tracequery.WindowStats{},
		types.ObservationSourceRef{Path: "cmp78.systrace"}, "w1", "2026-07-08T00:00:00Z", 1, 1)
	joined := strings.Join(record.RichNotes, "\n")
	for _, want := range []string{"background_rank=1", "member_count=2", "member_fold_caliber=sum_disjoint"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("the family observation record must emit %q:\n%s", want, joined)
		}
	}
	compiled := types.CompileTraceCausalProjection(types.ObservationLedger{
		Records: []types.ObservationRecord{record},
	})
	if len(compiled.SemanticSpans) != 1 {
		t.Fatalf("the family record must compile into one semantic span: %+v", compiled.SemanticSpans)
	}
	node := compiled.SemanticSpans[0]
	if node.BackgroundRank != 1 || node.FamilyMemberCount != 2 {
		t.Fatalf("the typed seat/count must survive the compile: %+v", node)
	}
	// Render the production-minted node on the witness tree shell: the 行2
	// seat and the 行3 family caliber are now end-to-end mintable.
	projection := rcm2CmpSemanticFamilyProjection()
	projection.SemanticSpans = []types.TraceCausalProjectionNode{node}
	_, fence := rcm2RenderFence(t, projection, true)
	for _, want := range []string{"背景榜位#1", "= 合计(共2段,同线程)", "×2"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("the production-minted family must render %q:\n%s", want, fence)
		}
	}
}
