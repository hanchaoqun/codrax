package tool

// answer_document_projection_disp2_test.go — DISP-2 (Wave-3.2 显示半场道,
// docs/design/real_trace_campaign_20260705.md §27.2 G2/G3, §27.5 G19,
// GAP-A P3-6, BLOCKFROM; 2026-07-09) display pins:
//
//   G2  盲区措辞按 kind 分形 — the ◇ inline disclosure forks on the typed
//       trace_gap_kind enum (no_eligible_wait → 窗内无≥阈值等待区间·链止 with
//       its own legend entry; no_sched_data/absent keep the 2026-07-07 用户
//       措辞裁定 wording byte-identically).
//   G19 全零折叠行一行注 — the ×N(0.000–0.000ms)取最大 claim is retired on the
//       all-zero shape; the honest 窗内无有效时长 note renders on the fence tag
//       AND the (a) table token (shared helper), value-bearing folds keep the
//       ×N取最大 form byte-identically.
//   G3  表列口径 — a ◇/▒ stanza row's 链上累计 cell renders "—" (the column
//       means on-chain accumulation) with a gated (a)-table legend line; the
//       value keeps its honest homes (stanza 累计(跨线程) tag / 窗口投影 cell).
//   G3  count 家族端到端 — tree 行1 value == (a) 窗口投影 == (a) 有效归因 ==
//       engine published value; the roster carries the 计数当量 marker; the
//       detail Σ note never prints the bare wall-clock 原始和 form.
//   P3-6 计数当量 legend entry + wrap atom.
//   BLOCKFROM 等待点 — the lock-row detail stanza renders the verbatim
//       waiter-side blocking call site beside 持有点; empty renders nothing.
//
// Mutation self-checks (each verified RED during development, then reverted):
//   M-1: dropping the TraceGapKind fork at the F5 tag site (always legacy
//        wording) → TestDisp2TraceGapKindWordingFork red.
//   M-2: reverting the all-zero fold arm to the ×N取最大 tag →
//        TestDisp2AllZeroFoldNoteFaces red (and the revisit76 bidirectional
//        fixture disp2_all_zero_fold red).
//   M-3: dropping the stanza-kind dash in the (a) 链上累计 cell →
//        TestDisp2StanzaChainTotalColumnDash red.
//   M-4: printing the count family's detail Σ note through the bare wall-clock
//        arm → TestDisp2CountFamilyEndToEnd red on the 计数当量 note pin.
//   M-5: removing the 等待点 line → TestDisp2BlockingFromSiteDetailLine red.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// --- G2: 盲区措辞按 kind 分形 ---------------------------------------------------

func TestDisp2TraceGapKindWordingFork(t *testing.T) {
	// no_eligible_wait: the honest below-floor wording + its own legend entry.
	below := ptv7SpnTraceGapBelowFloorProjection()
	model := buildRuntimeTraceProjTreeModel(below, nil, true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "窗内无≥阈值等待区间·链止") {
		t.Fatalf("no_eligible_wait row must carry the below-floor disclosure:\n%s", fence)
	}
	if strings.Contains(fence, "窗内无调度数据") {
		t.Fatalf("no_eligible_wait row must not over-claim 无调度数据 (复核 P3-5):\n%s", fence)
	}
	if !strings.Contains(fence, "数据盲区") {
		t.Fatalf("the 数据盲区 display word stays kind-invariant:\n%s", fence)
	}
	lead := runtimeTraceProjLeadText(below, model, "zh", true)
	if !strings.Contains(lead, "- `窗内无≥阈值等待区间` = 数据盲区判据之二:窗内有调度区间但均低于最小时长阈值,下钻链止。") {
		t.Fatalf("legend must teach the below-floor criterion:\n%s", lead)
	}
	// EN face.
	enModel := buildRuntimeTraceProjTreeModel(below, nil, false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "no in-window wait ≥ floor · chain ends") {
		t.Fatalf("EN no_eligible_wait wording missing:\n%s", enFence)
	}
	// Legacy fail-open: absent kind AND no_sched_data keep the 2026-07-07
	// wording byte-identically (the absent arm is already pinned by
	// TestPTV7SpnTraceGapRowWording; this pins the explicit no_sched_data arm).
	legacy := ptv7SpnTraceGapProjection()
	legacy.AdjacentCauses[0].TraceGapKind = "no_sched_data"
	legacyFence := runtimeTraceProjTreeFence(buildRuntimeTraceProjTreeModel(legacy, nil, true), true)
	if !strings.Contains(legacyFence, "窗内无调度数据·链止") {
		t.Fatalf("no_sched_data row keeps the ruling wording:\n%s", legacyFence)
	}
	if strings.Contains(legacyFence, "窗内无≥阈值等待区间") {
		t.Fatalf("no_sched_data row must not wear the below-floor wording:\n%s", legacyFence)
	}
}

// --- G19: 全零折叠行一行注 ------------------------------------------------------

func TestDisp2AllZeroFoldNoteFaces(t *testing.T) {
	projection := disp2AllZeroFoldProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	t.Logf("all-zero fold render (zh fence):\n%s", fence)
	if !strings.Contains(fence, "其余 9 项(链上折叠)") {
		t.Fatalf("the fold row keeps its counted lane name:\n%s", fence)
	}
	if !strings.Contains(fence, "窗内无有效时长(数据盲区),见明细") {
		t.Fatalf("the all-zero fold must carry the honest one-line note:\n%s", fence)
	}
	if strings.Contains(fence, "0.000–0.000") || strings.Contains(fence, ")取最大") {
		t.Fatalf("the member-MAX claim over zeros is retired on this shape:\n%s", fence)
	}
	// (a) table: same note token (shared helper), and the legend flags fork —
	// the ×N取最大 gated line must not claim an absent notation.
	_, rows := runtimeTraceProjDetailTable(model, true)
	foldCell := ""
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "链上折叠") {
			foldCell = row.Cells[0]
		}
	}
	if !strings.Contains(foldCell, "窗内无有效时长(数据盲区),见明细") {
		t.Fatalf("(a) table token must mirror the fence note: %q", foldCell)
	}
	flags := runtimeTraceProjDetailTableLegendFlagsFor(model, true)
	if !flags.allZeroFold || flags.mergedMax {
		t.Fatalf("legend flags must fork the all-zero shape off mergedMax: %+v", flags)
	}
	// (b) block ×N 明细 line mirrors the no-claim form.
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "×9 跨线程折叠,成员窗内均无可计量时长(数据盲区),不作取最大声明") {
		t.Fatalf("(b) block must mirror the no-claim form:\n%s", detail)
	}
	// Control: a value-bearing fold keeps the ×N取最大 form byte-identically.
	control := revisit76PTV5FoldCaliberProjection()
	controlModel := buildRuntimeTraceProjTreeModel(control, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	controlFence := runtimeTraceProjTreeFence(controlModel, true)
	if !strings.Contains(controlFence, ")取最大") {
		t.Fatalf("value-bearing folds keep the member-MAX form:\n%s", controlFence)
	}
	if strings.Contains(controlFence, "窗内无有效时长") {
		t.Fatalf("value-bearing folds never wear the all-zero note:\n%s", controlFence)
	}
}

// --- G3: ◇/▒ 行 链上累计 表列口径 ------------------------------------------------

func TestDisp2StanzaChainTotalColumnDash(t *testing.T) {
	// Local shape: an on-chain row WITH a cumulative (the control cell) plus a
	// ◇ adjacent row whose cum(18) differs from its projection(10) — the
	// stanza 累计(跨线程) tag renders as the value's lossless home.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
				Subject: "worker-9", Object: "running_burst", StateKind: "running",
				ChainRelevance: "on_chain", ImpactMS: 30, CumulativeImpactMS: 30,
				Confidence: 0.8},
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-adj",
				Subject: "adj-5", Object: "running_burst", StateKind: "running",
				ChainRelevance: "adjacent", ImpactMS: 10, CumulativeImpactMS: 18,
				EffectiveImpactMS: 7, Confidence: 0.8},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_, rows := runtimeTraceProjDetailTable(model, true)
	var adjacent, chain []string
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "adj-5") {
			adjacent = row.Cells
		}
		if strings.Contains(row.Cells[0], "worker-9") {
			chain = row.Cells
		}
	}
	if len(adjacent) == 0 || len(chain) == 0 {
		t.Fatalf("fixture rows missing from the (a) table: %+v", rows)
	}
	// Column order: 节点[E#](0) 窗口投影(1) 链上累计(2) 有效归因(3).
	if adjacent[2] != "—" {
		t.Fatalf("adjacent row's 链上累计 cell must be — (off-chain seat makes no on-chain accumulation): %q", adjacent[2])
	}
	if adjacent[1] != "10.000ms" {
		t.Fatalf("adjacent row keeps its window projection: %q", adjacent[1])
	}
	if adjacent[3] != "7.000ms" {
		t.Fatalf("adjacent row keeps its attribution cell (only 链上累计 dashes): %q", adjacent[3])
	}
	// Info-lossless home: the stanza row's 累计(跨线程) tag carries the value.
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "累计(跨线程)18.000ms") {
		t.Fatalf("the cumulative keeps its stanza-tag home:\n%s", fence)
	}
	// Control: the on-chain row keeps its chain-total cell.
	if chain[2] != "30.000ms" {
		t.Fatalf("on-chain rows keep the chain-total cell: %q", chain[2])
	}
	// Gated legend line rides the (a) table block exactly when the dash fires.
	flags := runtimeTraceProjDetailTableLegendFlagsFor(model, true)
	if !flags.stanzaChainTotal {
		t.Fatalf("stanzaChainTotal flag must gate the legend line: %+v", flags)
	}
	blocks := runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{})
	found := false
	for _, block := range blocks {
		if strings.HasSuffix(block.ID, "_detail") &&
			strings.Contains(block.Text, "◇/▒ 区段行不在唤醒链上,「链上累计」列为 —") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the gated stanza chain-total legend line must render with the dash")
	}
	// Byte-stability control: a stanza-less render carries neither dash nor line.
	control := revisit76UXAWindowlessProjection()
	controlFlags := runtimeTraceProjDetailTableLegendFlagsFor(
		buildRuntimeTraceProjTreeModel(control, newRuntimeTraceCausalProjectionEvidenceIndex(), true), true)
	if controlFlags.stanzaChainTotal {
		t.Fatalf("no stanza row → no gated line: %+v", controlFlags)
	}
}

// --- G3: count 家族端到端 (engine-real notes through the compile) ----------------

// disp2CountFamilyLedger is the opendir_79 页缓存抖动 shape AFTER the Wave-3.1
// engine half (G3): the count family publishes ONE capped value on ALL value
// channels (Cumulative==Effective==ImpactMs==41.671), the raw count-equivalent
// Σ rides member_sum_ms, and the roster values wear the engine's 计数当量
// marker verbatim (rootCauseCountEquivalentValue — fixture 取引擎实铸形).
func disp2CountFamilyLedger() types.ObservationLedger {
	return types.ObservationLedger{Records: []types.ObservationRecord{
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Subject: "app-100", Object: "worker-9 -> app-100",
		},
		{
			ID: "hop-1", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "wakeup_causal_impact", Subject: "worker-9", Object: "runnable_wait",
			Value: "30.000", Unit: "ms", Confidence: 0.8,
			RichNotes: []string{"causality=on_wakeup_chain", "chain_relevance=on_chain", "impact=30.000ms"},
		},
		{
			ID: "count-rank", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard,
			Predicate: "root_cause_context", ClaimKey: "root_cause_context:page_cache",
			Subject: "cache-worker-7", Object: "page_cache_churn",
			Value: "41.671", Unit: "ms", Confidence: 0.7,
			RichNotes: []string{
				"type=page_cache_churn", "rank=6",
				"causality=adjacent_to_wakeup_chain", "chain_relevance=adjacent",
				"impact=41.671ms", "cumulative_impact_ms=41.671", "effective_impact_ms=41.671",
				"member_count=2", "member_fold_caliber=count_sum",
				"member_max_ms=133.200", "member_min_ms=65.100", "member_sum_ms=198.300",
				"member_roster=inode=0x6a16 dev=254:2 计数当量133.200ms | inode=0x6a20 dev=254:2 计数当量65.100ms",
			},
		},
	}}
}

func TestDisp2CountFamilyEndToEnd(t *testing.T) {
	projection := types.CompileTraceCausalProjection(disp2CountFamilyLedger())
	if len(projection.AdjacentCauses) != 1 {
		t.Fatalf("count family row must land on its adjacent seat: %+v", projection.AdjacentCauses)
	}
	node := projection.AdjacentCauses[0]
	// Engine identity (Wave-3.1 G3): one published value on every channel.
	if node.ImpactMS != 41.671 || node.CumulativeImpactMS != 41.671 || node.EffectiveImpactMS != 41.671 {
		t.Fatalf("count family channels must all carry the published value: %+v", node)
	}
	if node.FamilyMemberCount != 2 || node.FamilyFoldCaliber != "count_sum" || node.FamilyMemberSumMS != 198.3 {
		t.Fatalf("count family typed lane must survive the compile: %+v", node)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	t.Logf("count family end-to-end render (zh fence):\n%s", fence)
	// Tree 行1 value == published value under the 合计 stem.
	if !strings.Contains(fence, "合计41.671ms") {
		t.Fatalf("tree 行1 must carry the published value under the 合计 stem:\n%s", fence)
	}
	// Roster carries the engine 计数当量 marker verbatim.
	if !strings.Contains(fence, "计数当量133.200ms") {
		t.Fatalf("the roster's count-equivalent marker must reach the fence:\n%s", fence)
	}
	// (a) table: 窗口投影 == 有效归因 == published value; 链上累计 = — (G3 表列).
	_, rows := runtimeTraceProjDetailTable(model, true)
	var cells []string
	for _, row := range rows {
		if strings.Contains(row.Cells[0], "cache-worker-7") {
			cells = row.Cells
		}
	}
	if len(cells) == 0 {
		t.Fatalf("count family row missing from the (a) table: %+v", rows)
	}
	if cells[1] != "41.671ms" || cells[3] != "41.671ms" {
		t.Fatalf("(a) 窗口投影/有效归因 must equal the published value: %+v", cells)
	}
	if cells[2] != "—" {
		t.Fatalf("(a) 链上累计 must be — on the adjacent seat: %+v", cells)
	}
	// Detail Σ note: count-equivalent form only — never the bare wall-clock
	// 原始和 (Wave-3.1 X2; this pin guards the whole chain against regression).
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "原始和 计数当量198.300ms 供对照(计数类,非墙钟)") {
		t.Fatalf("detail Σ note must speak the count-equivalent form:\n%s", detail)
	}
	if strings.Contains(detail, "原始和 198.300ms") {
		t.Fatalf("the bare wall-clock 原始和 form is banned on count families:\n%s", detail)
	}
	// Legend: the 计数当量 entry rides the count-family render (P3-6).
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "- `计数当量Xms` = 计数类数值的对照写法:按计数换算的当量毫秒,非墙钟时长,不与时长行相加。") {
		t.Fatalf("legend must teach the 计数当量 marker:\n%s", lead)
	}
	if !strings.Contains(lead, "计数合计") {
		t.Fatalf("the count caliber word entry must ride along:\n%s", lead)
	}
}

// TestDisp2CountEquivalentWrapAtom pins the P3-6 wrap atom: at no wrap width
// that can hold the word (≥ its own 8 cells — narrower widths legitimately
// hard-split ANY over-wide atom by rune) may a chunk boundary bisect 计数当量
// mid-claim (G8 折行劈词 family — the width sweep is the mutation-strength
// form: a single width can dodge the split by luck). The word's runes appear
// nowhere else in the probe string, so any chunk touching one of them must
// carry the whole word.
func TestDisp2CountEquivalentWrapAtom(t *testing.T) {
	for width := 8; width <= 30; width++ {
		for _, chunk := range runtimeTraceProjWrapDisplay("成员 inode=0x6a16 dev=254:2 计数当量133.200ms", width) {
			if strings.ContainsAny(chunk, "计数当量") && !strings.Contains(chunk, "计数当量") {
				t.Fatalf("width %d: wrap must not bisect 计数当量: %q", width, chunk)
			}
		}
	}
}

// --- BLOCKFROM: 等待点 detail line ----------------------------------------------

func TestDisp2BlockingFromSiteDetailLine(t *testing.T) {
	site := "monitor contention with owner OS_FFRT_2_2-43037 blocking from AssetManager.getResourceValue(AssetManager.java:761) waiters=1"
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "blk-1",
			Subject: "worker-9", Object: "blocking_span", TypeToken: "blocking_span",
			ChainRelevance: "on_chain", ImpactMS: 12.0, CumulativeImpactMS: 12.0,
			BlockingKind:       "monitor_contention",
			BlockingHolderSite: "SharedLibrary::GetStrings(art_dex.cc:120)",
			BlockingFromSite:   site,
			Confidence:         0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	detail := runtimeTraceProjDetailFullText(model, true)
	// Verbatim, untruncated, beside the 持有点 line.
	if !strings.Contains(detail, "- 等待点: "+site) {
		t.Fatalf("等待点 line must render the verbatim blocking-from site:\n%s", detail)
	}
	if !strings.Contains(detail, "- 持有点: SharedLibrary::GetStrings(art_dex.cc:120)") {
		t.Fatalf("持有点 line keeps its seat:\n%s", detail)
	}
	// EN face.
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enDetail := runtimeTraceProjDetailFullText(enModel, false)
	if !strings.Contains(enDetail, "- blocking from: "+site) {
		t.Fatalf("EN 等待点 line missing:\n%s", enDetail)
	}
	// Empty field renders nothing — absence never fabricates a site.
	projection.OnChainCauses[0].BlockingFromSite = ""
	bareModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if bare := runtimeTraceProjDetailFullText(bareModel, true); strings.Contains(bare, "等待点") {
		t.Fatalf("empty blocking-from must render no 等待点 line:\n%s", bare)
	}
}

// --- TEX 复核 F2: texture_upload 影响形态第五类臂 + universe 覆盖 pin ------------

// TestDisp2TextureUploadImpactFormFifthArm (DISP-2 收尾, TEX 复核 F2 P1,
// 2026-07-09): texture_upload rides the deterministic-optimization impact-form
// family with the EXACT same treatment as the four sibling semantic classes —
// the missing switch arm dropped texture rows to FormNone (影响形态/族词车道
// 缺席, §24.12 C7 病理形: a 未分类 claim beside 类型: texture_upload).
func TestDisp2TextureUploadImpactFormFifthArm(t *testing.T) {
	// Typed helper parity with a sibling class (form + family word).
	if got := runtimeTraceProjImpactFormTokenFamily("texture_upload"); got != runtimeTraceProjImpactFormDeterministicOpt {
		t.Fatalf("texture_upload must ride the deterministic-opt impact form: %d", got)
	}
	texture := types.TraceCausalProjectionNode{TypeToken: "texture_upload"}
	sibling := types.TraceCausalProjectionNode{TypeToken: "class_verification"}
	if got, want := runtimeTraceProjImpactFormFamilyWord(texture, true), runtimeTraceProjImpactFormFamilyWord(sibling, true); got == "" || got != want {
		t.Fatalf("texture_upload family word must match the sibling classes: %q vs %q", got, want)
	}
	// Render parity: the semantic family row wears the ✦ glyph + the four-line
	// grammar the four classes wear (类型词行1词位 + 合计 stem).
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "tex-1",
			Subject: "RenderThread-51342", Predicate: "trace_semantic_span",
			Object: "texture_upload", SemanticClass: "texture_upload",
			SpanName: "Texture upload (15283) 512x194",
			ImpactMS: 1.062, EffectiveImpactMS: 1.062, BackgroundRank: 1,
			FamilyMemberCount: 9, FamilyMemberMaxMS: 0.118, FamilyMemberMinMS: 0.076,
			FamilyFoldCaliber: "sum_disjoint",
			FamilyMemberRoster: []string{
				"Texture upload (15283) 512x194 0.118ms",
				"Texture upload (15284) 512x194 0.114ms",
				"Texture upload (15285) 256x128 0.096ms",
			},
			Confidence: 0.7,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	t.Logf("texture_upload semantic family render (zh fence):\n%s", fence)
	if !strings.Contains(fence, "✦ Texture upload ×9") {
		t.Fatalf("texture family row must wear the ✦ glyph + class word + ×N (四类同待遇):\n%s", fence)
	}
	if !strings.Contains(fence, "合计1.062ms") {
		t.Fatalf("texture family row must carry the 合计 value stem:\n%s", fence)
	}
}

// TestDisp2ImpactFormSwitchCoversSemanticClassLane is the TEX 复核 F2
// mechanisation (precedent: TestRootCauseTypeZHLabelCoversWeightUniverse):
// EVERY token the causal-token registry marks as a semantic-class family
// (CausalFamilyFoldSemanticClass — the mechanically enumerable source, §28.1
// user ruling on the fifth class) MUST ride the deterministic-opt arm of the
// rcr.go impact-form switch. A sixth class added to the registry lane but
// missed in the switch goes red here instead of falling to FormNone silently.
// Two layers: the runtime universe walk (behaviour-level) plus the fold-lane
// SOURCE scan (catches a semantic entry whose token never entered the spec
// universe).
func TestDisp2ImpactFormSwitchCoversSemanticClassLane(t *testing.T) {
	semantic := 0
	for _, token := range tracequery.CausalTokenUniverse() {
		if tracequery.CausalTokenFamilyFoldLane(token) != tracequery.CausalFamilyFoldSemanticClass {
			continue
		}
		semantic++
		if got := runtimeTraceProjImpactFormTokenFamily(token); got != runtimeTraceProjImpactFormDeterministicOpt {
			t.Errorf("registry semantic-class token %q missing from the impact-form deterministic-opt case (got form %d) — extend the rcr.go switch", token, got)
		}
	}
	if semantic < 5 {
		t.Fatalf("semantic-class universe scan looks broken: only %d tokens", semantic)
	}
	src, err := os.ReadFile(filepath.Join("..", "tracequery", "causal_token_registry.go"))
	if err != nil {
		t.Fatalf("read causal token registry source: %v", err)
	}
	entry := regexp.MustCompile("\"([a-z0-9_]+)\":\\s*CausalFamilyFoldSemanticClass")
	found := 0
	for _, m := range entry.FindAllStringSubmatch(string(src), -1) {
		found++
		if got := runtimeTraceProjImpactFormTokenFamily(m[1]); got != runtimeTraceProjImpactFormDeterministicOpt {
			t.Errorf("fold-lane source token %q missing from the impact-form deterministic-opt case (got form %d)", m[1], got)
		}
	}
	if found < 5 {
		t.Fatalf("fold-lane source scan looks broken: only %d entries", found)
	}
}
