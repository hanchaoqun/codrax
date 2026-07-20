package tool

// answer_document_projection_c8prose1_test.go — C8PROSE-1 批 pins (§29.164
// 残余非 bullet prose 面清单收账, 2026-07-20).
//
// The DISPHYG-3 件1 C8 regime: system-minted non-bullet prose sentences carry
// FULL-WIDTH top-level clause marks (，／；) with 。; parenthetical interiors
// and fence-shared word-face tokens keep half-width; the half-width `:` stays
// (the DISPHYG-3 anchored-lead precedent); EN faces are native half-width and
// stay byte-identical. This file extends the disphyg3 mixed-mark ban scan
// (disphyg3DepthZeroHalfClauseMarks) to every face the §29.164 filing listed:
//
//	A 明细区 intro / B 证据索引容量截断句 / C 对比总览 intro /
//	D+E 分区边界 caveat 双句 / F+G coverage 降级句族 / H 证据索引 intro /
//	I 确定性优化表 intro / J 优化表跳过 caveat / K 对比注记明细 intro(census
//	「等」面). 建制面 (指标表读法 bullet 族、⚠ 注记行族) stay untouched — the
//	adjudication table lives in the batch report.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Faces A + B: the per-node detail intro and the evidence-index capacity
// truncation sentence, driven through the production cluster builder.
func TestC8Prose1DetailAndEvidenceIntroRegime(t *testing.T) {
	findBlock := func(blocks []types.AnswerBlock, suffix string) *types.AnswerBlock {
		for i := range blocks {
			if strings.HasSuffix(blocks[i].ID, suffix) {
				return &blocks[i]
			}
		}
		return nil
	}
	projection := revisit76ResidualProjection()
	blocks := runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{})
	detail := findBlock(blocks, "_detail_full")
	if detail == nil {
		t.Fatalf("fixture must render the detail block")
	}
	intro := strings.Split(detail.Text, "\n")[0]
	want := "每个节点一块，给出树和指标表中省略或压缩的全部属性；名称不截断；属性完全相同的同名节点共用一块(标题并列各自编号)。树中折叠的中间线程见树内省略行清单。块序与树区自上而下一致(非按 [E#] 连续编号)；按 [E#] 查找请用下方「" + tracefence.SectionEvidenceZH + "」区(按编号排列)。"
	if intro != want {
		t.Fatalf("face A: detail intro must speak the C8 regime:\n got %q\nwant %q", intro, want)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(intro); n != 0 {
		t.Fatalf("face A: %d depth-0 half-width clause marks survived: %q", n, intro)
	}
	for _, banned := range []string{"一块,给出", "属性;名称", "截断;属性", "编号);按"} {
		if strings.Contains(detail.Text, banned) {
			t.Fatalf("face A: legacy mixed-mark bytes %q must be gone", banned)
		}
	}
	// Face B: the truncation disclosure appended to the evidence-index intro.
	projection.CapacityTruncated = true
	truncated := findBlock(runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{}), "_evidence")
	if truncated == nil {
		t.Fatalf("fixture must render the evidence-index block")
	}
	if !strings.Contains(truncated.Text, " 部分查询结果超过单次返回上限:各自排序靠前的部分完整保留，超限的尾部行不在本索引内。") {
		t.Fatalf("face B: truncation sentence must speak the C8 comma:\n%s", truncated.Text)
	}
	if strings.Contains(truncated.Text, "保留,超限") {
		t.Fatalf("face B: legacy half comma survived:\n%s", truncated.Text)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(truncated.Text); n != 0 {
		t.Fatalf("face B/H: evidence intro carries %d depth-0 half marks:\n%s", n, truncated.Text)
	}
	// EN faces stay native.
	enBlocks := runtimeTraceCausalProjectionCluster(projection, "en", runtimeTraceProjUserFocus{})
	if enDetail := findBlock(enBlocks, "_detail_full"); enDetail == nil ||
		!strings.HasPrefix(enDetail.Text, "One block per node, carrying every attribute") {
		t.Fatalf("face A: EN detail intro must keep its native form: %+v", enDetail)
	}
}

// Face H: the evidence-index intro sentences — full-width top-level joints,
// half-width audit k=v token roster and parenthetical interiors untouched
// (共享词面 token 单点纪律: the tokens mirror the trace_query wire bytes).
func TestC8Prose1EvidenceIndexIntroRegime(t *testing.T) {
	got := runtimeTraceCausalProjectionEvidenceText(true)
	want := "正文用 E1、E2 等编号引用证据；本索引给出每条证据在 trace 中的位置(行号或时间区间)与审计字段。" +
		"审计字段为 trace_query 原文 token，便于回溯核对:tier=证据层级、causality=因果位置、rank=根因排序、confidence=置信度、predicate=判定类型、span=span 名、merged_*=合并明细、member_*=同线程家族合并明细、same_value_*=跨线程取最大折叠中同值到微秒的成员及各自行区间(供核对是否同段)、origin=记录出处(system_supplement=成文前确定性补采所得,非模型查询)；其余字段同为原文 token。"
	if got != want {
		t.Fatalf("face H: evidence intro must speak the C8 regime:\n got %q\nwant %q", got, want)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(got); n != 0 {
		t.Fatalf("face H: %d depth-0 half-width clause marks survived: %q", n, got)
	}
	// Parenthetical interior keeps the wire-shared half-width comma.
	if !strings.Contains(got, "(system_supplement=成文前确定性补采所得,非模型查询)") {
		t.Fatalf("face H: the parenthetical token interior must keep half-width: %q", got)
	}
	for _, banned := range []string{"证据;本索引", "token,便于", ");其余"} {
		if strings.Contains(got, banned) {
			t.Fatalf("face H: legacy mixed-mark bytes %q must be gone", banned)
		}
	}
	if en := runtimeTraceCausalProjectionEvidenceText(false); !strings.HasPrefix(en, "The answer cites evidence by the E1/E2 numbers; ") ||
		!strings.Contains(en, "any other field is likewise a raw token.") {
		t.Fatalf("face H: EN intro must keep its native form: %q", en)
	}
}

// Faces D + E: the partition-boundary caveat sentences.
func TestC8Prose1PartitionCaveatRegime(t *testing.T) {
	set := types.TraceCausalProjectionSet{
		Projections:                  []types.TraceCausalProjection{{}},
		UnattributedObservationCount: 1,
		OmittedArtifactLabels:        []string{"t-b.trace"},
	}
	block := runtimeTraceProjPartitionCaveatBlock(set, true)
	if block == nil {
		t.Fatalf("fixture must mint the partition caveat")
	}
	want := "1 条观测无法归属到任一 trace 文件，未纳入投影。 trace 文件分区数超过上限，仅保留观测最多的 1 个；未展示: t-b.trace。"
	if block.Text != want {
		t.Fatalf("faces D/E: partition caveat must speak the C8 regime:\n got %q\nwant %q", block.Text, want)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(block.Text); n != 0 {
		t.Fatalf("faces D/E: %d depth-0 half-width clause marks survived: %q", n, block.Text)
	}
	for _, banned := range []string{"文件,未纳入", "上限,仅保留", "个;未展示"} {
		if strings.Contains(block.Text, banned) {
			t.Fatalf("faces D/E: legacy mixed-mark bytes %q must be gone", banned)
		}
	}
	en := runtimeTraceProjPartitionCaveatBlock(set, false)
	if en == nil || en.Text != "1 observation(s) carried no trace-file identity and were left out of every projection. Trace-file partitions exceeded the cap; the 1 with the most observations are shown. Omitted: t-b.trace." {
		t.Fatalf("faces D/E: EN caveat must keep its native form: %+v", en)
	}
}

// Faces F + F2 + G: the coverage degrade prose. F2 (结构化原因 roster joint)
// already spoke the full-width ；and is pinned unchanged.
func TestC8Prose1CoverageTextRegime(t *testing.T) {
	got := runtimeTraceCausalProjectionCoverageText([]string{"原因A", "原因B"}, true)
	want := "本报告已获得 trace_query 的结构化执行记录，但没有产出有数据支撑的 root_cause/wakeup_chain/semantic 行，因此未生成分层因果表。" +
		" 结构化原因: 原因A；原因B。" +
		" 这不是“没有背景影响”的结论；只表示当前证据没有给出可审计的因果/背景统计，可追问一次根因/窗口/交互统计分析(root_cause_rank、window_stats 或 interaction_stats)补齐。"
	if got != want {
		t.Fatalf("faces F/G: coverage prose must speak the C8 regime:\n got %q\nwant %q", got, want)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(got); n != 0 {
		t.Fatalf("faces F/G: %d depth-0 half-width clause marks survived: %q", n, got)
	}
	for _, banned := range []string{"记录,但", "行,因此", "结论;只", "统计,可追问"} {
		if strings.Contains(got, banned) {
			t.Fatalf("faces F/G: legacy mixed-mark bytes %q must be gone", banned)
		}
	}
	if en := runtimeTraceCausalProjectionCoverageText([]string{"r1"}, false); !strings.HasPrefix(en, "This report has structured trace_query execution records, ") {
		t.Fatalf("faces F/G: EN coverage must keep its native form: %q", en)
	}
}

// Faces C + K: the comparison overview intro and the notes-detail intro; the
// joined ⚠ note lines below the K intro are an institutional family and keep
// their own bytes.
func TestC8Prose1CompareOverviewAndNotesIntroRegime(t *testing.T) {
	blocks := runtimeTraceProjCompareOverviewBlocks([]types.TraceCausalProjection{
		enClosedSetCompareProjection("a.trace", 5.0, 5.1),
		enClosedSetCompareProjection("b.trace", 6.0, 6.3),
	}, types.ObservationLedger{}, "zh", true, runtimeTraceProjUserFocus{})
	if len(blocks) == 0 {
		t.Fatalf("two projections must render the zh comparison overview")
	}
	lead := strings.Split(blocks[0].Text, "\n")[0]
	if lead != "跨 trace 对比总览:数值来自各份 trace 独立的投影，跨线程累计值带单位标注，详情见各 trace 分段。" {
		t.Fatalf("face C: overview intro must speak the C8 regime: %q", lead)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(lead); n != 0 {
		t.Fatalf("face C: %d depth-0 half-width clause marks survived: %q", n, lead)
	}
	if strings.Contains(lead, "投影,跨线程") || strings.Contains(lead, "标注,详情") {
		t.Fatalf("face C: legacy mixed-mark bytes survived: %q", lead)
	}
	// Face K (the census 「等」 face): the notes-detail section intro.
	notes := runtimeTraceProjCompareNotesDetailBlock([]string{"⚠ 注记一,含半角"}, true)
	if notes == nil {
		t.Fatalf("notes-detail block must render")
	}
	kIntro := strings.Split(notes.Text, "\n")[0]
	if kIntro != "对比总览的全部注记(含总览下已显示的条目)，按重要度分层排序:口径矛盾 > 窗基 > 披露。" {
		t.Fatalf("face K: notes intro must speak the C8 regime: %q", kIntro)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(kIntro); n != 0 {
		t.Fatalf("face K: %d depth-0 half-width clause marks survived: %q", n, kIntro)
	}
	// The institutional note lines ride below byte-identically.
	if !strings.Contains(notes.Text, "\n\n⚠ 注记一,含半角") {
		t.Fatalf("face K: note-lane entries must keep their own bytes: %q", notes.Text)
	}
	if en := runtimeTraceProjCompareNotesDetailBlock([]string{"⚠ n1"}, false); en == nil ||
		!strings.HasPrefix(en.Text, "Every comparison-overview note (including the ones already shown under the table), layered by importance: ") {
		t.Fatalf("face K: EN notes intro must keep its native form: %+v", en)
	}
}

// Faces I + J: the deterministic-optimization table intro and the at-cap skip
// caveat (mint and reconcile share the single wording source, so the identity
// key evolves atomically).
func TestC8Prose1SemanticOptimizationFacesRegime(t *testing.T) {
	bus := semanticOptimizationFixtureBus("")
	doc := atCapDoc()
	doc.Blocks = doc.Blocks[:40]
	if !materializeRuntimeTraceSemanticOptimizationBlock(doc, bus) {
		t.Fatalf("below-cap pass must insert the optimization table")
	}
	block := projectionClusterBlock(doc.Blocks, "runtime_trace_semantic_optimizations")
	if block == nil {
		t.Fatalf("optimization table missing")
	}
	want := "trace 中的确定性语义优化 span(类校验/JIT编译/着色器编译/运行时编译/纹理上传/GC暂停等,来自 typed semantic_class 通道):每行都是可直接落地的优化点；时长与 E# 证据均可经证据索引定位到 trace 行号区间。"
	if block.Text != want {
		t.Fatalf("face I: optimization intro must speak the C8 regime:\n got %q\nwant %q", block.Text, want)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(block.Text); n != 0 {
		t.Fatalf("face I: %d depth-0 half-width clause marks survived: %q", n, block.Text)
	}
	// The parenthetical span-class roster keeps its half-width interior.
	if !strings.Contains(block.Text, "等,来自 typed semantic_class 通道") {
		t.Fatalf("face I: parenthetical interior must keep half-width: %q", block.Text)
	}
	if strings.Contains(block.Text, "优化点;时长") {
		t.Fatalf("face I: legacy mixed-mark bytes survived: %q", block.Text)
	}
	// Face J.
	skip := runtimeTraceSemanticOptimizationSkipCaveatText(true)
	wantSkip := fmt.Sprintf("文档已达 %d 块上限且无可让位的系统明细块:确定性优化点汇总表未插入；语义优化 span(类校验/JIT/着色器编译/Texture upload/GC暂停等)仍完整保留在 trace 因果投影区块中。", maxBlocksPerDoc)
	if skip != wantSkip {
		t.Fatalf("face J: skip caveat must speak the C8 regime:\n got %q\nwant %q", skip, wantSkip)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(skip); n != 0 {
		t.Fatalf("face J: %d depth-0 half-width clause marks survived: %q", n, skip)
	}
	if strings.Contains(skip, "未插入;语义优化") {
		t.Fatalf("face J: legacy mixed-mark bytes survived: %q", skip)
	}
	if en := runtimeTraceSemanticOptimizationSkipCaveatText(false); !strings.HasPrefix(en, "The document is at the ") {
		t.Fatalf("face J: EN skip caveat must keep its native form: %q", en)
	}
}
