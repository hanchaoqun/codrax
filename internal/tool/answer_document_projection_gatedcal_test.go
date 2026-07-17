package tool

// answer_document_projection_gatedcal_test.go — GATED-CAL display pins
// (§29.104.16.1 M3/M4 + §29.104.17 裁定①, 2026-07-16; sweep formation B1/B2;
// witness = UX catalog A2 E24/E28 + cust_span_vs_prio headline + XLANE-1
// satellite shape).
//
//	件1① 退化臂精确门 — a gated composite value (GatedRunningDeficitMS>0)
//	  never wears the 「(全额)」 tail; deficit-free rows keep it byte-identically.
//	件1② 构成式放宽 — the 行3 "=" equation renders even when the running
//	  component's 原始 is unknowable (E28 shape); the sub-row speaks 原始未发布;
//	  a family row whose value is the gated product never wears the 合计 word.
//	件1③ 窗口投影列 — a cell whose value IS the gated composite wears the
//	  构成,见明细 annotation + the gated legend flag; genuine projections stay bare.
//	件1④ 裸 tag 保底 — the Q1 bare 有效归因X tag wears the typed-producer floor
//	  word when 行3 did not consume the value; (发生段账目) wins where it fires.
//	件1⑤ ◎ 类词臂推广+注记臂精确门 — a composite seat transcribes its 行2
//	  category word and the 构成,见明细 note; pure shapes stay byte-identical.
//	件2  自身/语义席头条 — self/semantic primaries headline 有效归因 (+ the
//	  行3-form family equation suffix); true chain seats keep 链上累计.
//	件3  裁定① — represented-by-chain-seat satellites leave the ◎ population
//	  with the dedicated disclosure footnote; the closure identity gains the lane.

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// gatedCalEngineRealMD renders one engine-real window end to end (§29.53 产线
// 实铸形 red line; fixture 取引擎实铸形 lesson — the repair-round 件A re-pin:
// synthetic unbalanced forms the engine cannot mint must not judge the word
// gates). Shared by the tieba E15 negative arm and the donghu A2 positive arm.
func gatedCalEngineRealMD(t *testing.T, trace string, pid int, start, end float64) string {
	t.Helper()
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: pid, TimeStart: start, TimeEnd: end,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "fixture", "p-"+view, "r", "", at)...)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "gatedcal witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	return render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
}

// gatedCalAssertCompositeWordSplitReachable — 修补轮 件C (双复核 P2-2,
// 2026-07-17) renderer-level structure pin: EVERY line wearing 「构成,见明细」
// binds to a row whose detail block actually carries the composition split
// (有效归因构成 line — the 行3 equation or its fail-open mirror). The word is a
// pointer; a wearer without a reachable split is the dangling-pointer disease
// (the pre-repair tieba E15 shape: the word on a row whose detail block had no
// composition at all). Mounted on the engine-real witness battery (常驻).
func gatedCalAssertCompositeWordSplitReachable(t *testing.T, md string) {
	t.Helper()
	word := "构成,见明细"
	rowTag := regexp.MustCompile(`\[(E\d+)(\(\+\d+\))?\]`)
	headTag := regexp.MustCompile(`^[├└].*\[(E\d+)(\(\+\d+\))?\]`)
	isDefinition := func(line string) bool {
		trimmed := strings.TrimSpace(line)
		return strings.HasPrefix(trimmed, "- `"+word+"`") ||
			strings.HasPrefix(trimmed, "- 优先级反转席的「窗口投影」列")
	}
	splitReachable := func(base string) bool {
		for _, opener := range []string{"**[" + base + "]", "**[" + base + "("} {
			at := strings.Index(md, opener)
			if at < 0 {
				continue
			}
			block := md[at:]
			if next := strings.Index(block[2:], "**["); next >= 0 {
				block = block[:next+2]
			}
			if strings.Contains(block, "有效归因构成") {
				return true
			}
		}
		return false
	}
	currentRow := ""
	for _, line := range strings.Split(md, "\n") {
		if m := headTag.FindStringSubmatch(line); m != nil {
			currentRow = m[1]
		}
		if !strings.Contains(line, word) || isDefinition(line) {
			continue
		}
		// Bind the wearer to its row: the line's own [E#] set (◎ member line /
		// key-metric table row), else the nearest preceding tree 行1's E#.
		var candidates []string
		for _, m := range rowTag.FindAllStringSubmatch(line, -1) {
			candidates = append(candidates, m[1])
		}
		if len(candidates) == 0 && currentRow != "" {
			candidates = []string{currentRow}
		}
		if len(candidates) == 0 {
			t.Fatalf("件C: a 构成,见明细 wearer must bind to a row E#: %q\n%s", line, md)
		}
		ok := false
		for _, base := range candidates {
			if splitReachable(base) {
				ok = true
				break
			}
		}
		if !ok {
			t.Fatalf("件C 悬空指针: wearer %q (rows %v) has no reachable 有效归因构成 split:\n%s", line, candidates, md)
		}
	}
}

// gatedCalCompositeNode — the UX catalog A2 witness E28 shape: an on-chain
// inversion seat whose published effective is the gated composite
// runnable(全额) 2.181 + running(折算) 1.248 = 3.429, whose supply fold never
// ran (no engine fold raw, runnable-dominant → no display fallback for the
// running 原始) and whose display impact print-equals the composite.
func gatedCalCompositeNode(rank int) types.TraceCausalProjectionNode {
	node := elimChainNode("E-cmp", "keva-1-17437", "priority_inversion_candidate", "runnable", rank, 3.429, 100)
	node.PriorityInversionCandidate = true
	node.GatedRunnableMS = 2.181
	node.GatedRunningDeficitMS = 1.248
	return node
}

func gatedCalProjection(node types.TraceCausalProjectionNode) types.TraceCausalProjection {
	projection := elimBoardProjection()
	projection.OnChainCauses[0] = node
	return projection
}

// --- 件1②/件1① positive: the E28 shape renders its true derivation ------------

func TestGatedCalCompositeEquationRendersWithoutRunningRaw(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(gatedCalProjection(gatedCalCompositeNode(1)),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	// 件1②: the 行3 "=" equation renders (Σ计入==V balances) even though the
	// running 原始 is unknowable — the witness form used to lose the whole row.
	if !strings.Contains(fence, "有效归因 3.429ms = runnable(全额) 2.181ms + running(折算) 1.248ms") {
		t.Fatalf("件1②: the balanced composite must render its 行3 equation:\n%s", fence)
	}
	// The unknown-raw sub-row speaks 原始未发布 — never a fabricated 0.000ms,
	// and the caliber parenthesis stays lossless on it.
	if !strings.Contains(fence, "running 原始未发布 → 计入 1.248ms(折算,按") {
		t.Fatalf("件1②: the unknown-raw sub-row must speak 原始未发布 with its full caliber:\n%s", fence)
	}
	if strings.Contains(fence, "原始 0.000ms") {
		t.Fatalf("件1②: a fabricated zero 原始 must never print:\n%s", fence)
	}
	// 件1① (A2 witness form): the composite never wears the 「(全额)」 tail.
	if strings.Contains(fence, "3.429ms(全额)") {
		t.Fatalf("件1①: the gated composite must not wear the 全额 word:\n%s", fence)
	}
	// The known-raw runnable sub-row keeps the established grammar.
	if !strings.Contains(fence, "runnable 原始 2.181ms → 计入 2.181ms(全额)") {
		t.Fatalf("件1②: the known-raw component keeps the 原始 → 计入 sub-row:\n%s", fence)
	}
	// EN face rides the same lane.
	modelEN := buildRuntimeTraceProjTreeModel(gatedCalProjection(gatedCalCompositeNode(1)),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "attribution 3.429ms = runnable(in full) 2.181ms + running(discounted) 1.248ms") ||
		!rspaFenceContains(fenceEN, "running raw unpublished → counted 1.248ms") {
		t.Fatalf("件1② en: equation + unpublished-raw sub-row missing:\n%s", fenceEN)
	}
}

// --- 件1① + 修补轮 件A: the inclusion-identity degenerate gate ------------------
//
// EVOLUTION RECORD (修补轮 件A, 双复核 P1 合流, 2026-07-17): the first cut of
// this pin used a synthetic unbalanced form the engine cannot mint and pinned
// the composite word ONTO it — nailing the lie as correct. Re-pinned on
// ENGINE-REAL forms (fixture 取引擎实铸形): the tieba E15 carrier (eff 8.049 ==
// gatedRunnable alone, deficit 0.073 published but NOT counted) keeps 「(全额)」
// — the truth restored; the donghu A2 composite (3.429 == 2.181+1.248) wears
// the composite word on its faces; the engine-unmintable neither-identity
// stale form folds BARE (宁缺勿造 — defense arm, unit-probed).
func TestGatedCalDegenerateArmCompositeNeverWearsFull(t *testing.T) {
	// 负臂 (tieba 61839 E15 实铸形): the published-but-uncounted deficit must
	// not flip 「全额」 — the pre-repair regression wore 构成,见明细 here.
	tieba := gatedCalEngineRealMD(t, elimSemanticTiebaTrace, 61839, 34579.470, 34579.520)
	if !strings.Contains(tieba, "有效归因 8.049ms(全额)") {
		t.Fatalf("件A 负臂: the E15 pure-runnable value keeps 「(全额)」 (truth restored):\n%s", tieba)
	}
	if strings.Contains(tieba, "8.049ms(构成,见明细)") {
		t.Fatalf("件A 负臂: a published-but-uncounted deficit must never mint the composite word:\n%s", tieba)
	}
	gatedCalAssertCompositeWordSplitReachable(t, tieba)
	// 正臂 (donghu A2 实铸形): the counted composite keeps its word faces —
	// the 行3 equation, the ◎ note and the projection-cell annotation.
	donghu := gatedCalEngineRealMD(t, elimSemanticDonghuTrace, 17267, 13762.791708, 13763.024898)
	if !strings.Contains(donghu, "有效归因 3.429ms = runnable(全额) 2.181ms + running(折算) 1.248ms") {
		t.Fatalf("件A 正臂: the A2 composite must render its equation:\n%s", donghu)
	}
	if (!strings.Contains(donghu, "优先级反转候选 ·构成,见明细") &&
		!strings.Contains(donghu, "优先级反转候选·供给缺口主导 ·构成,见明细")) ||
		!strings.Contains(donghu, "3.429ms(构成,见明细)") {
		t.Fatalf("件A 正臂: the A2 composite keeps its ◎ note and cell annotation:\n%s", donghu)
	}
	gatedCalAssertCompositeWordSplitReachable(t, donghu)
	// 宁缺勿造 defense arm (engine-unmintable stale form, unit probe): neither
	// inclusion identity balances → the degenerate tail folds BARE — no 全额,
	// no composite claim.
	stale := gatedCalCompositeNode(1)
	stale.GatedRunningDeficitMS = 1.000 // 2.181+1.000 != 3.429 != 2.181
	staleModel := buildRuntimeTraceProjTreeModel(gatedCalProjection(stale), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	staleFence := rspaFenceJoined(runtimeTraceProjTreeFence(staleModel, true))
	if !strings.Contains(staleFence, "有效归因 3.429ms") ||
		strings.Contains(staleFence, "有效归因 3.429ms(") {
		t.Fatalf("件A 宁缺臂: the neither-identity stale form folds bare (no caliber claim):\n%s", staleFence)
	}
	// 词条-图例双向 on the synthetic balanced form (the composite word's legend
	// entry rides its render — mark lit at the degenerate emission is covered
	// by the ◎/cell arms; here the balanced form goes through the equation).
	pure := gatedCalCompositeNode(1)
	pure.GatedRunnableMS = 3.429
	pure.GatedRunningDeficitMS = 0
	pureModel := buildRuntimeTraceProjTreeModel(gatedCalProjection(pure), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	pureFence := rspaFenceJoined(runtimeTraceProjTreeFence(pureModel, true))
	if !strings.Contains(pureFence, "有效归因 3.429ms(全额)") || strings.Contains(pureFence, "构成,见明细") {
		t.Fatalf("件A 负臂(deficit-free): the pure full-amount seat keeps 「(全额)」 byte-identically:\n%s", pureFence)
	}
}

// --- 件1② family half: a gated product never wears the 合计 word ---------------

func TestGatedCalFamilyGatedProductNeverWearsFamilyTotal(t *testing.T) {
	node := gatedCalCompositeNode(1)
	node.FamilyMemberCount = 5
	node.FamilyFoldCaliber = "sum_disjoint"
	node.ImpactMS = 4.991 // family union projection ≠ the gated product 3.429
	node.CumulativeImpactMS = 4.991
	projection := gatedCalProjection(node)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "有效归因 3.429ms = 合计(共5段,同线程)") {
		t.Fatalf("件1②: the 合计 word must never label a gated product:\n%s", fence)
	}
	// The value's TRUE derivation renders instead (the inversion equation).
	if !strings.Contains(fence, "有效归因 3.429ms = runnable(全额) 2.181ms + running(折算) 1.248ms") {
		t.Fatalf("件1②: the gated-product family row must render the inversion equation:\n%s", fence)
	}
	// Negative arm (字节保形): a family carrier whose value fails the inclusion
	// identity keeps the family lanes untouched — the DISPLAY-WRAP A3 E6 shape
	// (value == the family's runnable account; 修补轮 件D勘正: the identity, not
	// any deficit-free retype assumption, is the gate — tieba E15 proved folded
	// carriers can hold a published-yet-uncounted deficit).
	e6 := gatedCalCompositeNode(1)
	e6.PriorityInversionCandidate = false
	e6.Object = "priority_inversion_runnable_wait"
	e6.TypeToken = "priority_inversion_runnable_wait"
	e6.GatedRunnableMS = 2.116
	e6.GatedRunningDeficitMS = 0
	e6.EffectiveImpactMS = 2.116
	e6.ImpactMS = 4.991
	e6.CumulativeImpactMS = 4.991
	e6.FamilyMemberCount = 5
	e6.FamilyFoldCaliber = "sum_disjoint"
	e6Model := buildRuntimeTraceProjTreeModel(gatedCalProjection(e6), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	e6Fence := rspaFenceJoined(runtimeTraceProjTreeFence(e6Model, true))
	if strings.Contains(e6Fence, "构成,见明细") || strings.Contains(e6Fence, "= runnable(全额)") {
		t.Fatalf("件1② 负臂: the deficit-free retype must keep its existing lanes:\n%s", e6Fence)
	}
}

// --- 件1③: the window-projection cell + gated legend flag ----------------------

func TestGatedCalTableProjectionCellAnnotated(t *testing.T) {
	projection := gatedCalProjection(gatedCalCompositeNode(1))
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(model, true)
	_, rows := runtimeTraceProjDetailTable(model, true)
	cell := ""
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "keva-1-17437") {
			cell = row.Cells[1]
		}
	}
	if cell != "3.429ms(构成,见明细)" {
		t.Fatalf("件1③: the gated-composite projection cell must wear the annotation, got %q", cell)
	}
	if !runtimeTraceProjDetailTableLegendFlagsFor(model, true).gatedProjection {
		t.Fatalf("件1③: the gated legend flag must raise with the annotated cell (词条-图例双向)")
	}
	// Negative arm (字节保形): a genuine state projection beside gated fields
	// (the E13 shape: projection 8.294 vs gated 7.405) stays bare.
	genuine := gatedCalCompositeNode(1)
	genuine.ImpactMS = 8.294
	genuine.CumulativeImpactMS = 8.294
	genuine.EffectiveImpactMS = 7.405
	genuine.GatedRunnableMS = 0.109
	genuine.GatedRunningDeficitMS = 7.296
	genuineModel := buildRuntimeTraceProjTreeModel(gatedCalProjection(genuine), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(genuineModel, true)
	_, genuineRows := runtimeTraceProjDetailTable(genuineModel, true)
	for _, row := range genuineRows {
		if strings.Contains(row.Cells[0], "keva-1-17437") && row.Cells[1] != "8.294ms" {
			t.Fatalf("件1③ 负臂: a genuine projection cell stays bare, got %q", row.Cells[1])
		}
	}
	if runtimeTraceProjDetailTableLegendFlagsFor(genuineModel, true).gatedProjection {
		t.Fatalf("件1③ 负臂: no annotated cell → no gated legend flag")
	}
}

// --- 件1④ + 修补轮 件A/件B: the Q1 bare-tag typed-producer floor ----------------
//
// EVOLUTION RECORD (修补轮, 2026-07-17): the first cut pinned the composite
// floor word onto a synthetic unbalanced form — under the 件B inclusion
// identity that form is NOT a composite and the belt stays off (宁缺勿造).
// Re-pinned on ENGINE-REAL forms: the donghu running-deficit half is the live
// belt witness (binder 0.933 — bare pre-batch); tieba E15 is the identity-fail
// negative through the real path.
func TestGatedCalBareTagCompositeBelt(t *testing.T) {
	// 正臂 (donghu 实铸形): the §20.2 running-deficit row without a rank seat
	// shipped the naked 有效归因0.933ms — the belt now names its own caliber.
	donghu := gatedCalEngineRealMD(t, elimSemanticDonghuTrace, 17267, 13762.791708, 13763.024898)
	if !strings.Contains(donghu, "有效归因0.933ms(折算,按全域最大核最高频)") {
		t.Fatalf("件1④ 正臂: the running-deficit bare tag must wear its 折算 floor word:\n%s", donghu)
	}
	// 负臂 (tieba E15 实铸形): the identity-fail carrier keeps every face free
	// of the composite word (◎ bare word / no belt word / 全额 tail intact).
	tieba := gatedCalEngineRealMD(t, elimSemanticTiebaTrace, 61839, 34579.470, 34579.520)
	if strings.Contains(tieba, "构成,见明细") {
		t.Fatalf("件B 负臂: the identity-fail window must carry zero composite wearers:\n%s", tieba)
	}
	// 宁缺臂 (engine-unmintable stale form, unit probe): identity-fail composite
	// with eff≠impact — the belt stays off, the tag ships bare (never a false
	// claim; 前提改读 ConsumedEffective 实际 still holds — 行3 did not consume).
	node := gatedCalCompositeNode(1)
	node.GatedRunningDeficitMS = 1.000 // inclusion identity fails
	node.ImpactMS = 4.200
	node.CumulativeImpactMS = 4.200 // eff < cum → (发生段账目) stays off
	projection := gatedCalProjection(node)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "有效归因3.429ms") || strings.Contains(fence, "有效归因3.429ms(") {
		t.Fatalf("件1④ 宁缺臂: the identity-fail bare tag ships without a caliber claim:\n%s", fence)
	}
	// (发生段账目) precedence arm (ELIM-GAP 件D lanes that still run the bare
	// tag): eff > cum on the typed producer keeps the 件D word — no stacking.
	segment := gatedCalCompositeNode(1)
	segment.GatedRunningDeficitMS = 1.000
	segment.ImpactMS = 4.200
	segment.CumulativeImpactMS = 3.400 // eff > cum → 件D word fires
	segModel := buildRuntimeTraceProjTreeModel(gatedCalProjection(segment), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	segFence := rspaFenceJoined(runtimeTraceProjTreeFence(segModel, true))
	if !strings.Contains(segFence, "有效归因3.429ms(发生段账目)") {
		t.Fatalf("件1④ 负臂: the (发生段账目) word keeps precedence:\n%s", segFence)
	}
	if strings.Contains(segFence, "(发生段账目)(构成,见明细)") {
		t.Fatalf("件1④ 负臂: the floor word must never stack on 件D's word:\n%s", segFence)
	}
}

// --- 件1⑤: the ◎ class-word generalization + the precise note arm --------------

func TestGatedCalElimCompositeClassWordAndNote(t *testing.T) {
	projection := gatedCalProjection(gatedCalCompositeNode(1))
	model, fence := elimRenderOverview(t, projection, true)
	seatLine := ""
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "3.429ms") {
			seatLine = line
		}
	}
	if seatLine == "" {
		t.Fatalf("the composite seat must render on the ◎ board:\n%s", fence)
	}
	// 类词臂推广: the sub-threshold composite transcribes its 行2 category word
	// (转录同词 with runtimeTraceProjCauseCategoryWord) — never the bare state
	// word over a two-caliber value.
	if !strings.Contains(seatLine, "优先级反转候选") || strings.Contains(seatLine, "· runnable") {
		t.Fatalf("件1⑤: the composite seat must wear its 行2 category word:\n%s", seatLine)
	}
	// 注记臂精确门: the composite note replaces silence (eff==projection shape).
	if !strings.Contains(seatLine, "·构成,见明细") {
		t.Fatalf("件1⑤: the composite seat must carry the composite caliber note:\n%s", seatLine)
	}
	if !model.Marks.has(runtimeTraceProjMarkGatedCompositeCaliber) {
		t.Fatalf("件1⑤: the ◎ note must light the composite legend mark")
	}
	// Negative arm (字节保形): the pure gated-runnable inversion seat (deficit
	// 0 — the RNB-1 C-1 E31 shape) keeps the bare state word and no note.
	pure := gatedCalCompositeNode(1)
	pure.GatedRunnableMS = 3.429
	pure.GatedRunningDeficitMS = 0
	_, pureFence := elimRenderOverview(t, gatedCalProjection(pure), true)
	pureLine := ""
	for _, line := range elimOverviewMemberLines(pureFence) {
		if strings.Contains(line, "3.429ms") {
			pureLine = line
		}
	}
	if !strings.Contains(pureLine, "· runnable") || strings.Contains(pureLine, "构成,见明细") {
		t.Fatalf("件1⑤ 负臂: the pure full-amount seat keeps its word face byte-identically:\n%s", pureLine)
	}
	// 修补轮 件B 负臂 (双复核 P2-1, 2026-07-17): the inclusion identity fails
	// (components published beside a value they were not counted into) → NOT a
	// composite on ANY face — the ◎ keeps the bare word, no composite note
	// (stale-form defense; the engine-real identity-fail path is the tieba E15
	// window in the 件A battery).
	unproven := gatedCalCompositeNode(1)
	unproven.GatedRunningDeficitMS = 1.000 // 2.181+1.000 != 3.429
	_, unprovenFence := elimRenderOverview(t, gatedCalProjection(unproven), true)
	unprovenLine := ""
	for _, line := range elimOverviewMemberLines(unprovenFence) {
		if strings.Contains(line, "3.429ms") {
			unprovenLine = line
		}
	}
	if !strings.Contains(unprovenLine, "· runnable") || strings.Contains(unprovenLine, "构成,见明细") {
		t.Fatalf("件B 负臂: the identity-fail seat keeps the bare word on the ◎ face:\n%s", unprovenLine)
	}
}

// --- 件2: the self/semantic headline fork --------------------------------------

func TestGatedCalHeadlineSelfSemanticSeat(t *testing.T) {
	// Semantic family primary (the cust_span_vs_prio witness shape): the
	// headline speaks 有效归因 + the 行3-form family equation — never 链上累计.
	sem := elimChainNode("E-sem", "unimportant-100", "class_verification", "running", 1, 9.586, 100)
	sem.Role = types.TraceCausalRolePrimaryRootCause
	sem.SemanticClass = "class_verification"
	sem.FamilyMemberCount = 8
	sem.FamilyFoldCaliber = "sum_disjoint"
	ms, word, periodic, windowSource := runtimeTraceProjConclusionMagnitude(sem, true)
	if word != "有效归因" || ms != 9.586 || periodic || windowSource {
		t.Fatalf("件2: the semantic seat must headline 有效归因, got (%v, %q, %v, %v)", ms, word, periodic, windowSource)
	}
	if suffix := runtimeTraceProjConclusionFamilyCaliberSuffix(sem, true); suffix != " = 合计(共8段,同线程)" {
		t.Fatalf("件2: the family equation suffix must ride the headline, got %q", suffix)
	}
	if _, wordEN, _, _ := runtimeTraceProjConclusionMagnitude(sem, false); wordEN != "attribution" {
		t.Fatalf("件2 en: got %q", wordEN)
	}
	if suffixEN := runtimeTraceProjConclusionFamilyCaliberSuffix(sem, false); suffixEN != " = total (8 segments, same thread)" {
		t.Fatalf("件2 en suffix: got %q", suffixEN)
	}
	// End-to-end: the conclusion line carries the fork's bytes.
	projection := gatedCalProjection(sem)
	projection.PrimaryRootCause = &sem
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(model, true)
	lead := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(lead, "有效归因 9.586ms = 合计(共8段,同线程)") || strings.Contains(lead, "链上累计") {
		t.Fatalf("件2: the semantic lead line must speak the family accounting, never 链上累计: %q", lead)
	}
	// Self wall-clock seat (typed OnChainBasis): same fork, no family suffix.
	self := elimChainNode("E-slf", "target-1", "runnable_wait", "runnable", 1, 5.000, 100)
	self.OnChainBasis = "self_wall_clock_interval"
	self.CumulativeImpactMS = 7.000
	if ms, word, _, _ := runtimeTraceProjConclusionMagnitude(self, true); word != "有效归因" || ms != 5.000 {
		t.Fatalf("件2: the self-basis seat must headline 有效归因, got (%v, %q)", ms, word)
	}
	// Negative arm (字节保形): a true drill-down chain seat keeps 链上累计.
	chain := elimChainNode("E-chn", "RxComputationT-16612", "priority_inversion_candidate", "running", 1, 37.410, 100)
	chain.PriorityInversionCandidate = true
	chain.CumulativeImpactMS = 58.919
	if ms, word, _, _ := runtimeTraceProjConclusionMagnitude(chain, true); word != "链上累计" || ms != 58.919 {
		t.Fatalf("件2 负臂: the chain drill seat keeps 链上累计 byte-identically, got (%v, %q)", ms, word)
	}
	// Gated-product family (件1② twin): the suffix refuses the 合计 claim.
	gated := gatedCalCompositeNode(1)
	gated.SemanticClass = "class_verification"
	gated.FamilyMemberCount = 5
	gated.FamilyFoldCaliber = "sum_disjoint"
	if suffix := runtimeTraceProjConclusionFamilyCaliberSuffix(gated, true); suffix != "" {
		t.Fatalf("件2: the 合计 suffix must never label a gated product, got %q", suffix)
	}
}

// --- 件3 (裁定① §29.104.17): represented satellites leave the ◎ population -----

func TestGatedCalRepresentedSatelliteLeavesOverview(t *testing.T) {
	projection := xlaneRepresentedSatelliteProjection()
	projection.RootCauseFamilyObserved = true
	model, fence := elimRenderOverview(t, projection, true)
	// The full-value ◇ bar is out of the population (值面零动 — the exclusion
	// is board membership only).
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "23.471ms") {
			t.Fatalf("件3: the represented satellite must not hold a ◎ bar:\n%s", fence)
		}
	}
	// The chain seat keeps its member line.
	members := elimOverviewMemberLines(fence)
	if len(members) != 1 || !strings.Contains(members[0], "17.635ms") {
		t.Fatalf("件3: the on-chain seat keeps its bar, got %d members:\n%s", len(members), fence)
	}
	// The dedicated disclosure footnote names the excluded row (排除≠消失).
	satTag := ""
	for _, row := range model.Adjacent {
		if row.Node.EvidenceID == "xlane-satellite" {
			satTag = strings.TrimSpace(row.EvidenceTag)
		}
	}
	if satTag == "" {
		t.Fatalf("fixture drifted: the satellite row must render in the adjacent stanza")
	}
	if !strings.Contains(fence, "· 已由链上席代表(降道):1 行,见明细 ["+satTag+"]") {
		t.Fatalf("件3: the disclosure footnote must count and name the excluded row:\n%s", fence)
	}
	// EN face.
	enModel, enFence := elimRenderOverview(t, projection, false)
	if !strings.Contains(enFence, "· represented by the on-chain seat (whole-seat demotion): 1 row(s) — see the detail blocks") {
		t.Fatalf("件3 en: disclosure footnote missing:\n%s", enFence)
	}
	_ = enModel
	// 树面零动: the seat and its honest sentence stay on the tree face.
	tree := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(tree, "23.471ms") || !strings.Contains(tree, "锚定份由") {
		t.Fatalf("件3: the tree face keeps the satellite seat and its sentence:\n%s", tree)
	}
	// The closure identity gains the represented lane (structure pin).
	elimGapAssertBoardAccounting(t, projection)
	// Negative arm: non-represented ◇ seats keep entering the population
	// (the base board's adjacent members are unaffected) — zero footnote.
	_, plainFence := elimRenderOverview(t, elimBoardProjection(), true)
	if strings.Contains(plainFence, "已由链上席代表(降道)") {
		t.Fatalf("件3 负臂: zero represented rows → zero footnote:\n%s", plainFence)
	}
}
