package tool

// answer_document_projection_uxa_rcrb_test.go — PTV8-RCR-B pins (UXA 全展示面
// 措辞横扫批, ledger docs/design/real_trace_campaign_20260705.md §24 ④ 全面
// UX 复审令 + UXA 审计改造表 2026-07-08):
//
//   1. 三族词表负向 pin (全渲染面扫描级): retired 窗族/归因族/根因族 words
//      never ship on any zh block of the projection cluster again.
//   2. 图例终形 verbatim pins for the UXA-rewritten catalog entries.
//   3. B3 重点项 正/负 pins: lead blocking_span zh label, 无唤醒记录 display
//      word + wordless-row subject form, 影响点 bare-state suppression.
//   4. Structure pins: coverage bullets, merged 因果位置 cell, 关系/影响点
//      split, identical-block merge, 置信 column, span 原文 at block end,
//      holder-site compaction, five-segment tag order.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// uxaRenderedZHFaces renders every customer-facing zh string of one
// projection's cluster (lead+fence / table legend+cells / blocks / evidence).
func uxaRenderedZHFaces(t *testing.T, projection types.TraceCausalProjection) []string {
	t.Helper()
	var faces []string
	for _, block := range runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{}) {
		faces = append(faces, block.Title, block.Text)
		faces = append(faces, block.Columns...)
		for _, item := range block.Items {
			faces = append(faces, item.Label, item.Text)
			faces = append(faces, item.Cells...)
		}
	}
	return faces
}

// TestUXARetiredWordsNeverShipZH is the 三族词表 "旧词禁出厂" negative pin:
// a full-face scan over representative shapes. EVOLUTION RECORD: these words
// were the UXA audit's 混乱第一贡献源 (one report spoke 8 window words); the
// unified families are 分析窗/查询窗/用户请求窗/数据实际覆盖 · 链上已归因/
// 未归因/有效归因/链上累计/累计(跨线程)/折算 · 主根因/根因排序#N/根因排序前三.
func TestUXARetiredWordsNeverShipZH(t *testing.T) {
	banned := []string{
		// 窗族
		"关注窗口", "投影窗", "锚窗", "选定窗", "实际对齐窗", "用户窗口", "聚焦子窗", "分析窗口", "重叠窗",
		// 归因族
		"on-chain", "残差",
		// 根因族 + 内部词
		"根因关注点", "候选影响", "候选根因", "(rank=", "无损纵排", "关键量表", "覆盖分子",
		"本轮", "本批",
		// PTV8-RCR-C (§24.14 D-5, 2026-07-08): the B#3 目标→关注线程 family
		// survivors (P0-A2 compare-face 发射点) join the negative scan.
		"目标睡眠", "目标症状时长",
	}
	fixtures := map[string]types.TraceCausalProjection{
		"opendir_node_group":  rcrOpendirProjection(),
		"ptv4_badges_merges":  revisit76PTV4BadgeMergeProjection(),
		"stanza_discount":     revisit76UXAStanzaDiscountProjection(),
		"windowless_fallback": revisit76UXAWindowlessProjection(),
		"flat_undrillable":    revisit76FlatUndrillableProjection(),
		"periodic_source":     revisit76PeriodicProjection(),
	}
	for name, projection := range fixtures {
		for _, face := range uxaRenderedZHFaces(t, projection) {
			for _, word := range banned {
				if strings.Contains(face, word) {
					t.Errorf("%s: retired word %q shipped on a zh face:\n%s", name, word, face)
				}
			}
		}
	}
}

// TestUXALegendFinalFormsVerbatim pins the UXA-rewritten catalog entries'
// exact zh bytes (改造表 CONFIRM/REVISE 终稿 — a single changed character
// bites here).
func TestUXALegendFinalFormsVerbatim(t *testing.T) {
	want := map[runtimeTraceProjMark]string{
		runtimeTraceProjMarkIconSleep:   "- `☾/sleep` = 睡眠等待(等事件/等唤醒);睡眠是症状而非根因,根因看它的下钻/唤醒子行。",
		runtimeTraceProjMarkIconTransit: "- `◦ 中转` = 唤醒链的中间经过节点,本报告未单独计量其影响。",
		// EVOLUTION RECORD (UXR-1 §29.36.1): ◦ 只留真正无类型词的行 — the
		// entry states both halves symmetrically.
		runtimeTraceProjMarkIconNoDominant: "- `◦`(数据行) = 未识别出具体影响类型且无主导调度状态的行;有形态词的行戴各自形态族记号,该行的已知信息见行内说明或明细。",
		runtimeTraceProjMarkBadge:// RULE3-1 件2+件3 (§29.181②③, 2026-07-21): the entry gains
		// the per-board TOP5 clause, the single-carrier clause and the
		// crown-vs-badge caliber sentence. 双复核修复 (冷读 P2-2 收窄形a,
		// 2026-07-21). EVOLUTION RECORD: 「➊=板内值序」 → the precise
		// engine-published effective-attribution wording.
		"- `➊..➎` = 根因排序前五(依有效归因),按板各发(每块查询板各自的 TOP5);佩章行行2不再复读 根因排序#N 词(徽章即序数;未佩章而有序数的行保留词形);标题主根因=选举权威(凭证强度参与),➊=按引擎发布的板内有效归因序(与树行显示口径可异),二者可不同(不同时标题括注注明口径)。",
		runtimeTraceProjMarkStateLabel:       "- 行内 sleep/runnable/running/iowait/D-state = 该行的主导调度状态。",
		runtimeTraceProjMarkUndrillable:      "- `⊘链止` = 窗口内无匹配唤醒事件(sched_wakeup),链止于此。",
		runtimeTraceProjMarkChainDepthChip:   "- `链上L#` = 该行在唤醒链上的层数(与明细「层级」行一致)。",
		runtimeTraceProjMarkBarScale:         "- 时长条:满格 = 树头标注的长度(本报告为分析窗全长);多窗合并行的时长条只作相对量级(见其专项条目)。",
		runtimeTraceProjMarkBarScaleFallback: "- 时长条:窗口未采集,满格 = 本报告最大时长(不显示占窗百分比);多窗合并行的时长条只作相对量级(见其专项条目)。",
		runtimeTraceProjMarkMergedMax:        "- `N线程取最大(单项a~b)` = N 个线程的同类行合并为一行;墙钟跨线程不可加和,数值取其中最大一项,a~b 为单项范围。",
		runtimeTraceProjMarkOverWindowShare:  "- 占窗>100% = 跨CPU/多段累计,可合法超过窗口长度(时长条已封顶);同一线程几乎相同的重复记录(差异≤3%)只计一次,明显不同的重叠段分段累计。",
		runtimeTraceProjMarkWholeWindowIdle:  "- `整窗等待` = 该行几乎覆盖整个窗口(≥99%),多为空闲或常驻等待线程,仅作背景参考。",
		runtimeTraceProjMarkAdjacentStanza:   "- `◇` = 邻近区段:与唤醒链时间相邻,不在唤醒链上。",
		runtimeTraceProjMarkBackgroundStanza: "- `▒` = 背景压力区段:环境证据,不计入链上归因,需结合链上证据解读。",
		// §29.61.6 (词面批 2026-07-14): the epistemic-status sentence is part
		// of the verbatim pin — 三要素: 非正常义 (未归因≠正常/无需解释)、可能
		// 构成 (未发现原因/未探查窗/未识别空闲,系统不判定)、已识别正常空闲另列.
		runtimeTraceProjMarkCoverageLine:            "- 已归因/未归因 = 树头覆盖句的口径:只统计第一层直接原因行对关注线程的影响;未归因 = 关注线程等待(或整窗)时长 − 已归因;各层时长在墙钟上互相包含,不能逐层相加;未归因≠正常/无需解释:是尚未被已发布原因覆盖的部分(可能含未发现原因/未探查窗/未识别空闲,系统不判定);已识别的正常空闲(如帧间空闲)另行单列。",
		runtimeTraceProjMarkStanzaCrossThreadCum:    "- `累计(跨线程)` = ◇/▒ 区段行的时长口径:多线程时间累计,不计入链上已归因。",
		runtimeTraceProjMarkStanzaDiscount:          "- `折算` = 该行折算后的有效值,仅在与累计值不同时并列显示。",
		runtimeTraceProjMarkCandidateShapeClass:     "- 无类型词的行 = 未识别出具体影响类型;逐行影响形态见明细。",
		runtimeTraceProjMarkEffectiveAttributionTag: "- `有效归因` = 该行计入根因排序的影响时长(完整口径见关键指标表)。",
	}
	got := map[runtimeTraceProjMark]string{}
	for _, entry := range runtimeTraceProjLegendCatalog() {
		got[entry.Mark] = entry.ZH
	}
	for mark, text := range want {
		if got[mark] != text {
			t.Errorf("legend entry for mark %d drifted:\n got %q\nwant %q", mark, got[mark], text)
		}
	}
}

// TestUXALeadBlockingSpanZHLabel — UXA 域A #1 / 域D 漏审 S1: the narrative
// lane no longer leaks the bare wire token; the detail 类型 row keeps it
// (§22.2.1 兜底结构).
func TestUXALeadBlockingSpanZHLabel(t *testing.T) {
	if got := runtimeTraceCausalProjectionNarrativeCauseName("blocking_span", true); got != "持锁阻塞（blocking_span）" {
		t.Fatalf("zh narrative blocking_span = %q", got)
	}
	// RULE3-1 件8 (§29.182②): the EN narrative speaks the combined
	// label-(token) form; the detail 类型 row below keeps the raw token.
	if got := runtimeTraceCausalProjectionNarrativeCauseName("blocking_span", false); got != "lock-holder blocking (blocking_span)" {
		t.Fatalf("en narrative combined form drifted: %q", got)
	}
	if got := runtimeTraceCausalProjectionRawTypeToken(types.TraceCausalProjectionNode{Object: "blocking_span"}); got != "blocking_span" {
		t.Fatalf("类型 row must keep the raw token: %q", got)
	}
}

// TestUXAMissingWakeupDisplayWord — UXA 域A #22/#30 (任务令终词 无唤醒记录):
// the data-gap marker speaks zh on the display faces, the wordless self row
// names the fact, and a WORDED row keeps the bare ⊘链止 byte-identically.
func TestUXAMissingWakeupDisplayWord(t *testing.T) {
	if got := runtimeTraceCausalProjectionDisplayCauseName("missing_wakeup", true); got != "无唤醒记录" {
		t.Fatalf("zh display cause = %q", got)
	}
	if got := runtimeTraceCausalProjectionNarrativeCauseName("missing_wakeup", true); got != "无唤醒记录（missing_wakeup）" {
		t.Fatalf("zh narrative = %q", got)
	}
	fence, _ := rcrOpendirFence(t, true)
	if !strings.Contains(fence, "无唤醒记录·⊘链止") {
		t.Fatalf("wordless missing_wakeup self row must name the fact:\n%s", fence)
	}
	// Worded row (sleep wording present) keeps the bare marker.
	flat := revisit76FlatUndrillableProjection()
	model := buildRuntimeTraceProjTreeModel(flat, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	flatFence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(flatFence, "无唤醒记录·⊘链止") {
		t.Fatalf("a worded sleep row must keep the bare ⊘链止:\n%s", flatFence)
	}
	if !strings.Contains(flatFence, "⊘链止") {
		t.Fatalf("the ⊘链止 marker itself must survive:\n%s", flatFence)
	}
	// §22.2.1 兜底结构 (复核 pin 缺口 (i)): a row whose CAUSE word is the
	// translated missing_wakeup keeps the raw token on the block's 类型 line.
	tokenRow := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		AdjacentCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "adj-5",
				Object: "missing_wakeup", ChainRelevance: "adjacent",
				ImpactMS: 1.082, Confidence: 0.7, EvidenceID: "mw-1"},
		},
	}
	tokenModel := buildRuntimeTraceProjTreeModel(tokenRow, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	blocks := runtimeTraceProjDetailFullText(tokenModel, true)
	if !strings.Contains(blocks, "- 类型: missing_wakeup") {
		t.Fatalf("the raw token must survive on the 类型 lossless line:\n%s", blocks)
	}
	if !strings.Contains(blocks, "无唤醒记录") {
		t.Fatalf("the zh display word must lead the block name:\n%s", blocks)
	}
}

// TestUXAImpactPointBareStateSuppressed — UXA 域A #23: the 影响点 slot lists
// entities only; a bare scheduler-state token is suppressed on the tree tag
// while the detail block keeps the full roster (zero information loss).
func TestUXAImpactPointBareStateSuppressed(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "io_latency", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 30, Confidence: 0.8, EvidenceID: "ip-1",
				SecondaryObjects: []string{"sleep_wait", "udk-irq-10-90"}},
		},
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "影响点 udk-irq-10-90") {
		t.Fatalf("entity impact point must survive:\n%s", fence)
	}
	if strings.Contains(fence, "影响点 sleep") || strings.Contains(fence, "sleep（sleep_wait）/udk") {
		t.Fatalf("bare state token must not enter the 影响点 slot:\n%s", fence)
	}
	blocks := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(blocks, "- 影响点: sleep（sleep_wait） / udk-irq-10-90") {
		t.Fatalf("the detail block keeps the FULL roster:\n%s", blocks)
	}
	// All-state roster → the tag vanishes entirely (nothing left to name).
	allState := projection
	allState.OnChainCauses[0].SecondaryObjects = []string{"sleep_wait"}
	model2 := buildRuntimeTraceProjTreeModel(allState, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence2 := runtimeTraceProjTreeFence(model2, true); strings.Contains(fence2, "影响点") {
		t.Fatalf("an all-state roster must drop the tag:\n%s", fence2)
	}
}

// TestUXACoverageBulletsNoGlue — UXA 域A layout-⑤ / 域D layout-L4: the tree
// header speaks the window-family words, one fact per "- " line, and the
// "。 " glue gap is dead.
func TestUXACoverageBulletsNoGlue(t *testing.T) {
	projection := rcrOpendirProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.HasPrefix(line, "分析窗 33872.289~33872.409s，共 120.000ms。") {
		t.Fatalf("window head = %q", line)
	}
	if !strings.Contains(line, "\n- 关注线程等待(sleep/D-state/runnable)") {
		t.Fatalf("the coverage sentence must be its own bullet:\n%s", line)
	}
	if strings.Contains(line, "。 ") {
		t.Fatalf("the 。+space glue gap must be dead:\n%s", line)
	}
}

// TestUXADetailBlockStructure — 域B #13/#16/#17/#22-#24/S1: 置信 column,
// merged 因果位置, split 关系/影响点 clauses, identical-block merge, the
// 完整名称 line retired.
func TestUXADetailBlockStructure(t *testing.T) {
	projection := rcrOpendirProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	columns, rows := runtimeTraceProjDetailTable(model, true)
	if columns[len(columns)-1] != "置信" {
		t.Fatalf("last column = %q", columns[len(columns)-1])
	}
	for _, row := range rows {
		last := row.Cells[len(row.Cells)-1]
		switch last {
		case "高", "中", "低", "—":
		default:
			t.Fatalf("confidence cell must be the bare tier: %q", last)
		}
	}
	blocks := runtimeTraceProjDetailFullText(model, true)
	for _, want := range []string{
		"**[E1] [E2] .ugc.aweme.lite-16547 / sleep**", // identical blocks merged
		"- 因果位置: 关注线程自身",                              // self row fixed value
		"- 因果位置: 主根因(优先处理)",                           // merged vocab
		"- 关系: #RxComputationT-16816 的同窗状态构成",         // full clause (A2 件4①: 成因→构成)
		"- 影响点: udk-irq-10-90 / udk-irq-2-77 / udk-irq-6-84",
	} {
		if !strings.Contains(blocks, want) {
			t.Fatalf("detail blocks missing %q:\n%s", want, blocks)
		}
	}
	for _, banned := range []string{"因果位置·优先级", "主要关注", "重点关注", "支撑参考", "▸", "- 完整名称:"} {
		if strings.Contains(blocks, banned) {
			t.Fatalf("retired block form %q resurfaced:\n%s", banned, blocks)
		}
	}
	// 复核 pin 缺口 (ii): the tree tag compacts the holder site, but the
	// block face keeps the FULL signature verbatim (§22.2.1 无损承诺).
	if !strings.Contains(blocks, "- 持有点: java.lang.String[] android.content.res.AssetManager.list(java.lang.String)(AssetManager.java:1258)") {
		t.Fatalf("the block face must keep the full holder signature:\n%s", blocks)
	}
}

// TestUXADeepIndentMultiNoteTagsSurvive — 复核 pin 缺口 (iii): a deeply
// indented row carrying several notes keeps EVERY note whole (demote-not-
// elide) even when the indent eats most of the width budget.
func TestUXADeepIndentMultiNoteTagsSurvive(t *testing.T) {
	path := []string{"p9-9", "p8-8", "p7-7", "p6-6", "p5-5", "p4-4", "p3-3", "p2-2", "p1-1", "target-7"}
	deep := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, Subject: "p9-9",
		Object: "io_latency", ChainRelevance: "on_chain", ChainDepth: 9,
		ImpactMS: 12, CumulativeImpactMS: 30, EffectiveImpactMS: 8,
		SecondaryObjects: []string{"udk-irq-10-90"},
		Confidence:       0.8, EvidenceID: "deep-1",
	}
	projection := types.TraceCausalProjection{
		WakeupPath:    path,
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{deep},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"影响点 udk-irq-10-90", "链上L9", "链上累计30.000ms", "有效归因 8.000ms",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("deep-indent multi-note row lost %q:\n%s", want, fence)
		}
	}
	for _, line := range strings.Split(fence, "\n") {
		if w := runewidth.StringWidth(line); w > 100 {
			t.Fatalf("row width cap broken (%d cells): %q", w, line)
		}
	}
}

// TestUXASpanSourceClosesBlock — 域B #18: the verbatim span text renders
// last, code-styled, under the renamed span 原文 key.
func TestUXASpanSourceClosesBlock(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, Subject: "worker-9",
				Object: "monitor_contention", BlockingKind: "monitor_contention",
				SpanName:       "monitor contention with owner #X at java.foo(Bar.java:1)",
				ChainRelevance: "on_chain", ChainDepth: 1, ImpactMS: 30,
				Confidence: 0.8, EvidenceID: "sp-1"},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	blocks := runtimeTraceProjDetailFullText(model, true)
	stanza := blocks
	if idx := strings.Index(blocks, "\n\n"); idx > 0 {
		stanza = blocks[:idx]
	}
	lines := strings.Split(stanza, "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "- span 原文: `") || !strings.HasSuffix(last, "`") {
		t.Fatalf("span source must close the block in code style, got %q\n%s", last, stanza)
	}
}

// TestUXAHolderSiteCompaction — 域A #29: signature-aware compaction keeps
// 类.方法(文件:行) inside the 40-cell budget; the raw signature stays whole
// on the lossless faces (asserted by the detail-block pin above).
func TestUXAHolderSiteCompaction(t *testing.T) {
	site := "java.lang.String[] android.content.res.AssetManager.list(java.lang.String)(AssetManager.java:1258)"
	got := runtimeTraceProjHolderSiteCompact(site, 40)
	if runewidth.StringWidth(got) > 40 {
		t.Fatalf("budget exceeded: %q (%d cells)", got, runewidth.StringWidth(got))
	}
	if !strings.Contains(got, "AssetManager.java:1258)") {
		t.Fatalf("the file:line coordinate must survive: %q", got)
	}
	if strings.Contains(got, "java.lang.String[]") {
		t.Fatalf("the return type is the least useful part and must go: %q", got)
	}
	// Short sites pass through verbatim.
	if got := runtimeTraceProjHolderSiteCompact("foo.c:12", 40); got != "foo.c:12" {
		t.Fatalf("short site must pass through: %q", got)
	}
}

// TestUXAFiveSegmentTagOrder — 域D #36: qualitative → 链上L# → magnitudes →
// 有效归因-last on non-cause rows (the specimen's E4/E5 orders drifted).
func TestUXAFiveSegmentTagOrder(t *testing.T) {
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, Subject: "worker-9",
				Object: "io_latency", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 30, CumulativeImpactMS: 42, EffectiveImpactMS: 12,
				SecondaryObjects: []string{"udk-irq-10-90"},
				Confidence:       0.8, EvidenceID: "seg-1"},
		},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	idx := func(tok string) int {
		i := strings.Index(fence, tok)
		if i < 0 {
			t.Fatalf("token %q missing:\n%s", tok, fence)
		}
		return i
	}
	impact, chip, cum, eff := idx("影响点 udk-irq-10-90"), idx("链上L1"), idx("链上累计42.000ms"), idx("有效归因 12.000ms")
	if !(impact < chip && chip < cum && cum < eff) {
		t.Fatalf("five-segment order violated (影响点@%d 链上L@%d 累计@%d 有效归因@%d):\n%s",
			impact, chip, cum, eff, fence)
	}
}

// TestUXASubordinateLinesPackAtMostTwo — 域A layout-⑥: no subordinate "·"
// line glues more than two notes.
func TestUXASubordinateLinesPackAtMostTwo(t *testing.T) {
	lines := runtimeTraceProjSubordinatePackedLines("  ", []string{"甲甲", "乙乙", "丙丙", "丁丁", "戊戊"})
	for _, line := range lines {
		body := strings.TrimPrefix(strings.TrimLeft(line, " "), "· ")
		if strings.Count(body, " · ") > 1 {
			t.Fatalf("a packed line holds more than two notes: %q", line)
		}
	}
	if got := len(lines); got != 3 {
		t.Fatalf("five short notes must pack into 3 lines, got %d: %v", got, lines)
	}
}

// TestUXAWrapNeverStrandsParens — 域D layout-L2 + 复核 M6: the wrap does not
// open a line with closing punctuation nor end one with an opening bracket —
// including ASCII-adjacent brackets/commas that used to hide inside ASCII
// runs ("1.853ms(" as one atom). The supply-fold Dominant clause is the
// witness shape at the review-given widths.
func TestUXAWrapNeverStrandsParens(t *testing.T) {
	check := func(text string, width int) {
		t.Helper()
		chunks := runtimeTraceProjWrapDisplay(text, width)
		if strings.Join(chunks, "") != text {
			t.Fatalf("width %d: byte concatenation broken: %v", width, chunks)
		}
		for _, chunk := range chunks {
			if strings.HasPrefix(chunk, ")") || strings.HasPrefix(chunk, "）") ||
				strings.HasPrefix(chunk, ",") || strings.HasPrefix(chunk, "，") {
				t.Fatalf("width %d: a line opens with closing punctuation: %v", width, chunks)
			}
			trimmed := strings.TrimRight(chunk, " ")
			if strings.HasSuffix(trimmed, "(") || strings.HasSuffix(trimmed, "（") {
				t.Fatalf("width %d: a line ends with an opening paren: %v", width, chunks)
			}
		}
	}
	for width := 6; width <= 20; width++ {
		check("调度压力(需求积压口径说明文字很长很长很长很长)", width)
	}
	// Full width sweep (复核 M6 verification widths 22/24/46/50 included):
	// some width always lands a break exactly at a bracket/comma boundary, so
	// gutting either punct table cannot stay green.
	dominant := "供给折算缺口 1.853ms(运行频点非最高,按全域最大核最高频折算,下界)为主,running 时间含降频/小核导致的跑慢成分"
	for width := 8; width <= 60; width++ {
		check(dominant, width)
	}
}

// TestUXABoundaryTruncateNoMidWordResidue — 域A #24/漏审A: the composed-name
// suffix cuts only at word boundaries; a token whose first word cannot fit
// yields whole ("s_s…" never ships again), while the A批 E4 form
// ("持锁阻塞…") survives byte-for-byte.
func TestUXABoundaryTruncateNoMidWordResidue(t *testing.T) {
	if got := runtimeTraceProjBoundaryTruncate("持锁阻塞(等待方 .ugc.aweme.lite-16547)", 12); got != "持锁阻塞…" {
		t.Fatalf("boundary cut = %q", got)
	}
	if got := runtimeTraceProjBoundaryTruncate("s_sleep", 4); got != "" {
		t.Fatalf("a first word that cannot fit must yield, got %q", got)
	}
	if got := runtimeTraceProjBoundaryTruncate("D-state/iowait(对端未解析)", 16); got != "D-state/iowait…" {
		t.Fatalf("slash-family boundary cut = %q", got)
	}
	name := runtimeTraceProjTruncateName("irq/209-thp-388 · s_sleep", 20)
	if strings.Contains(name, "s_s…") {
		t.Fatalf("mid-word residue shipped: %q", name)
	}
}

// TestUXAConclusionLeadSpeaksZhBlockingWord — end-to-end: a rank-lane lead
// whose Object is the bare blocking_span token renders the zh label + token
// pair on the conclusion line (fixture mirrors the opendir lead shape).
func TestUXAConclusionLeadSpeaksZhBlockingWord(t *testing.T) {
	projection := rcrHopOnlyProjection(112.175, 112.223)
	projection.PrimaryRootCauses = []types.TraceCausalProjectionNode{{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "bs-1",
		Subject: "#RxComputationT-16816", Predicate: "root_cause_primary",
		Object: "blocking_span", Rank: 1, Tier: "primary",
		ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 112.223, CumulativeImpactMS: 112.223, EffectiveImpactMS: 112.223,
		Confidence: 0.67,
	}}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	line := runtimeTraceProjConclusionLine(projection, model, true)
	if !strings.Contains(line, "持锁阻塞（blocking_span）") {
		t.Fatalf("the lead must speak the zh label with the token in parens: %q", line)
	}
	if strings.Contains(strings.ReplaceAll(line, "持锁阻塞（blocking_span）", ""), "blocking_span") {
		t.Fatalf("no bare wire token outside the labeled pair: %q", line)
	}
}

// TestUXAEvidenceIndexGroupedLocatorForms — 域C #6/#7: grouped entries speak
// 行 X–Y (en-dash) and the coordinate tail joins the 定位 field without the
// basename; the audit-token legend sentence leads the intro.
func TestUXAEvidenceIndexGroupedLocatorForms(t *testing.T) {
	projection := rcrOpendirProjection()
	for i := range projection.OnChainCauses {
		node := &projection.OnChainCauses[i]
		node.SupportRefs = []string{fmt.Sprintf("/blob/record_trace.sys.ftrace:%d-%d", node.LineStart, node.LineEnd)}
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	_ = runtimeTraceProjTreeFence(model, true)
	intro, items := runtimeTraceProjEvidenceBlockParts(evidence, true)
	// C8PROSE-1 (§29.164, 2026-07-20). EVOLUTION RECORD: depth-0 半角 , → 全角 ，.
	if !strings.Contains(intro, "全部证据位于 `record_trace.sys.ftrace`，各条只标注行号或时间区间。") {
		t.Fatalf("grouped intro must declare the artifact once: %q", intro)
	}
	if !strings.Contains(intro, "tier=证据层级、causality=因果位置、rank=根因排序") {
		t.Fatalf("audit-token legend sentence missing from the intro: %q", intro)
	}
	found := false
	for _, item := range items {
		if strings.Contains(item.Text, "定位: 行 ") && strings.Contains(item.Text, "–") {
			found = true
		}
		if strings.Contains(item.Text, "lines=") {
			t.Fatalf("raw machine locator shipped: %q", item.Text)
		}
		if strings.Contains(item.Text, "定位: :") {
			t.Fatalf("bare colon locator shipped: %q", item.Text)
		}
	}
	if !found {
		t.Fatalf("grouped entries must speak 行 X–Y: %#v", items)
	}
}

// TestUXASleepGuidanceYieldsToRenderedUpstream — 域A #25: the per-row
// 睡眠症状→查上游 tag renders only when the drilldown target is NOT itself a
// rendered tree row.
func TestUXASleepGuidanceYieldsToRenderedUpstream(t *testing.T) {
	base := func(target string) types.TraceCausalProjection {
		return types.TraceCausalProjection{
			WakeupPath:    []string{"waker-2", "app-100"},
			WindowStartTs: 100.0,
			WindowEndTs:   100.2,
			OnChainCauses: []types.TraceCausalProjectionNode{
				{Role: types.TraceCausalRoleCausalHop, Subject: "waker-2",
					Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
					ChainDepth: 1, ImpactMS: 30, Confidence: 0.8, EvidenceID: "sl-1",
					DrilldownTarget: target},
			},
		}
	}
	rendered := base("app-100") // the target IS a rendered row (the root)
	model := buildRuntimeTraceProjTreeModel(rendered, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence := runtimeTraceProjTreeFence(model, true); strings.Contains(fence, "睡眠症状→查上游") {
		t.Fatalf("guidance must yield when the upstream is rendered:\n%s", fence)
	}
	offTree := base("ghost-77") // upstream NOT in the tree → guidance stays
	model2 := buildRuntimeTraceProjTreeModel(offTree, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence := runtimeTraceProjTreeFence(model2, true); !strings.Contains(fence, "睡眠症状→查上游") {
		t.Fatalf("guidance must stay for an un-rendered upstream:\n%s", fence)
	}
}

// TestUXASupplyFoldFinalWordings — 域A #26/#27/#28 终稿 (typed verdict
// inputs drive the three branches; wording asserted verbatim).
func TestUXASupplyFoldFinalWordings(t *testing.T) {
	dominant := types.TraceCausalProjectionNode{
		Object: "running", StateKind: "running",
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 1.853,
		SupplyFoldKnownMS: 10, ImpactMS: 5,
	}
	clause, _, ok := runtimeTraceProjSupplyFoldClause(dominant, 100, true)
	if !ok || clause != "供给折算缺口 1.853ms(运行频点非最高,按全域最大核最高频折算,下界)为主,running 时间含降频/小核导致的跑慢成分" {
		t.Fatalf("dominant clause = %q (ok=%v)", clause, ok)
	}
	noDeficit := types.TraceCausalProjectionNode{
		Object: "running", StateKind: "running",
		SupplyFoldComputed: true, SupplyFoldKnownMS: 10, ImpactMS: 5,
	}
	clause, _, ok = runtimeTraceProjSupplyFoldClause(noDeficit, 100, true)
	if !ok || clause != "已按全域最大核最高频(或接近)运行·无供给折算" {
		t.Fatalf("no-deficit clause = %q (ok=%v)", clause, ok)
	}
	unknown := types.TraceCausalProjectionNode{
		Object: "running", StateKind: "running",
		SupplyFoldComputed: true, SupplyFoldUnknownMS: 3, ImpactMS: 5,
	}
	clause, _, ok = runtimeTraceProjSupplyFoldClause(unknown, 100, true)
	if !ok || clause != "CPU 频率数据不全,无法折算" {
		t.Fatalf("unknown-basis clause = %q (ok=%v)", clause, ok)
	}
}

// TestUXAFixtureRenderSnapshot re-renders the opendir node group end to end
// and pins a handful of cross-face key lines in one place (cheap smoke for
// the whole批; the per-face pins above give the precise diagnostics).
func TestUXAFixtureRenderSnapshot(t *testing.T) {
	blocks := runtimeTraceCausalProjectionCluster(rcrOpendirProjection(), "zh", runtimeTraceProjUserFocus{})
	var all strings.Builder
	for _, block := range blocks {
		all.WriteString(block.Title + "\n" + block.Text + "\n")
		for _, item := range block.Items {
			all.WriteString(item.Label + " " + item.Text + "\n")
			all.WriteString(strings.Join(item.Cells, "|") + "\n")
		}
	}
	text := all.String()
	for _, want := range []string{
		"因果投影关键指标",
		"因果投影明细(逐节点完整属性)",
		"各列口径:",
		"分析窗 33872.289~33872.409s，共 120.000ms。",
		"- 记号:",
		"- 口径:",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("cluster missing %q", want)
		}
	}
	if strings.Contains(text, fmt.Sprintf("%c%c", 'E', '1'+0)+" · ") {
		t.Fatalf("the duplicated E# confidence cell resurfaced")
	}
}
