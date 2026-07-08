package tool

// answer_document_projection_ptv6d_test.go — PTV6-D 批⑤ (real-trace campaign
// 2026-07-06, 标本归因 #10: UX 纵向密度重构, 用户裁定 "打包非折叠" — 信息零
// 丢失). Four faces:
//
//   (a) 从属行流式打包 — runtimeTraceProjSubordinatePackedLines streams demoted
//       tags into " · "-separated 100-cell lines (T3 atom wrap kept), and the
//       main-line fit judgment loses its all-or-nothing cliff: the leading
//       ordinary tags that fit stay beside the Keep 记号, only the overflow
//       packs below (prefix discipline — reading order never reorders).
//   (b) 类别样板词降维 — the ◦ 无主导态 2-word chip (7×/标本1) and the generic
//       候选影响 shape word (5×/标本1) leave the row face; the ◦ icon + two
//       NEW-7 legend entries carry the class semantics, the detail table keeps
//       every full cell (候选根因 stayed: 2×/标本1, below the 5×/标本 bar).
//   (c) 等值 tag 合并 — a chain-universe row whose 链上累计 equals its 有效归因
//       folds the redundant cum copy (stanza-lane precedent 同折); the Q1 tag
//       survives (user ruling 2026-07-05: 常显 + badge sort key).
//   (d) ≈均值 加语义 — the irq family (irq_burst/irq_activity/ipi_activity)
//       density reads ≈窗内并发 X.X× on both the stanza suffix and the F3
//       compare cell (supply_pressure 族 ≈平均排队深度 先例).
//
// 差表 (从属行断言 逐行 → 打包序列, existing pins updated alongside):
//   - TestPTV6CSpecimen1KeyRowsAfter: "· 可运行等待" (own subordinate line) →
//     PTV7: the canonical runnable word rides the name cell whole; main-row
//     fill + packed-stream asserts follow the new geometry.
//   - TestPTV6CSpecimen2KeyRowsAfter: "· 优先级反转候选" → "优先级反转候选 ·
//     [E1(+1)]" (main-row slot).
//   - TestPTV6CCauseFullWordGuaranteeOnTruncatedName: first-subordinate-slot
//     arms → main-row first-tag-slot arms + packed-stream assert.
//   - TestRuntimeTraceProjCrossThreadAggregateRowRendering: irq arm ≈均值 0.1
//     → ≈窗内并发 0.1× (+ EN arm, + io_pressure neutral arm kept).
//   - revisit76LegendProbes: IconNoDominant fence probe "无主导态" → "" (chip
//     retired by design; direction A still asserts).
//   - TestRuntimeStateKindSwitchConsumerCoverage golden key: ImpactShapeCell →
//     ImpactShapeCellTyped (rename only, case set byte-identical).
//
// MUTATIONS pinned here:
//   - 恢复逐 tag 一行 → the packed-stream + line-ledger pins go red;
//   - 恢复全体下放悬崖 → the main-row tag-slot pins go red;
//   - 恢复 无主导态/候选影响 行内词 → the category-word negative pins go red;
//   - 摘除 (c) 等值折 → the exactly-one-attribution-tag pin goes red;
//   - 摘除 (d) 并发词 → the concurrency-token/wording pins go red.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// --- (a) 打包器几何 --------------------------------------------------------------

func TestPTV6DSubordinatePackedLinesGeometry(t *testing.T) {
	indent := "      "
	tags := []string{
		strings.Repeat("a", 30), strings.Repeat("b", 30),
		strings.Repeat("c", 30), strings.Repeat("d", 30),
	}
	lines := runtimeTraceProjSubordinatePackedLines(indent, tags)
	// 6(indent)+2(· )+30 +3+30 = 71; +3+30 = 104 > 100 → two tags per line.
	if len(lines) != 2 {
		t.Fatalf("4×30-cell tags must pack two per line, got %d lines: %q", len(lines), lines)
	}
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("packed line %d over the row cap (%d cells): %q", i, w, line)
		}
		if !strings.HasPrefix(line, indent+"· ") {
			t.Fatalf("every packed line starts at a tag boundary with the marker: %q", line)
		}
	}
	// 打包非折叠: every tag appears whole, in order.
	joined := strings.Join(lines, "\n")
	pos := 0
	for _, tag := range tags {
		idx := strings.Index(joined[pos:], tag)
		if idx < 0 {
			t.Fatalf("tag %q lost or reordered in the packed stream:\n%s", tag, joined)
		}
		pos += idx + len(tag)
	}
	// Tags within one line are " · "-separated.
	if !strings.Contains(lines[0], tags[0]+" · "+tags[1]) {
		t.Fatalf("in-line tags must be ' · '-separated: %q", lines[0])
	}
}

func TestPTV6DSubordinatePackedLinesSingleTagCompat(t *testing.T) {
	indent := "│   │ "
	for _, text := range []string{
		"链上L1",
		strings.Repeat("同段IO另有 io_burst_episode 226.153ms、io_wait 112.011/107.672ms 口径;", 3) + "证据 E2、E3、E4",
	} {
		packed := runtimeTraceProjSubordinatePackedLines(indent, []string{text})
		legacy := runtimeTraceProjSubordinateLines(indent, text)
		if strings.Join(packed, "\n") != strings.Join(legacy, "\n") {
			t.Fatalf("single-tag packing must equal the legacy wrap form:\npacked: %q\nlegacy: %q", packed, legacy)
		}
	}
}

func TestPTV6DSubordinatePackedLinesOverWideTagOwnsItsLines(t *testing.T) {
	indent := "      "
	wide := strings.Repeat("x", 200) // no break boundary — hard atom split
	tags := []string{strings.Repeat("a", 10), wide, strings.Repeat("b", 10)}
	lines := runtimeTraceProjSubordinatePackedLines(indent, tags)
	// a → line 1; wide → its own wrapped lines; b → a FRESH "· " line (a
	// packed neighbor never rides a mid-tag continuation line).
	last := lines[len(lines)-1]
	if last != indent+"· "+strings.Repeat("b", 10) {
		t.Fatalf("the tag after an atom-wrapped over-wide tag must open a fresh marker line: %q", lines)
	}
	var joined strings.Builder
	for _, line := range lines {
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("packed line over the row cap (%d cells): %q", w, line)
		}
		joined.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, indent+"· "), indent+"  "))
	}
	if !strings.Contains(joined.String(), wide) {
		t.Fatalf("over-wide tag content must survive the hard split byte-complete:\n%s", joined.String())
	}
}

// --- (a) 自身行前缀填充 ------------------------------------------------------------

func TestPTV6DSelfRowPrefixFillAndPacking(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "com.baidu.tieba-59566", StateKind: "s_sleep",
		ImpactMS: 25.806, ActualImpactMS: 25.806,
		DuplicatePublications: 2, MergedCount: 3,
		MergedMinMS: 1.0, MergedMaxMS: 12.0, Confidence: 0.9,
	}
	node.MergedSubjects = []string{"m-1", "m-2"}
	row := runtimeTraceProjTreeRow{
		Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		EvidenceTag: "E1", marks: &runtimeTraceProjMarkSet{},
	}
	lines := runtimeTraceProjSelfRowLines(row, 30.0, true)
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("self row line %d over the cap (%d cells): %q", i, w, line)
		}
	}
	// All parts fit → single line here; force overflow with a huge dedupe
	// count text is not possible via typed fields, so assert the fitting form
	// stays single-line (the cliff never demotes what fits).
	if len(lines) != 1 {
		t.Fatalf("fitting self row must stay a single line, got %d: %q", len(lines), lines)
	}
	for _, want := range []string{"☾ sleep", "25.806ms", "×2同值", "×3(1.000–12.000ms)", "[E1]"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("self row lost part %q: %q", want, lines[0])
		}
	}
	// Overflow arm ((d) 缩进常量核对): a 3-peer IO caliber note forces the
	// packed form — every subordinate line rides the SAME "│     " lead the
	// main self row uses (content column aligned), each line holds the cap,
	// and the leading demoted parts that still fit stay inline (prefix fill).
	row.IOFoldPeers = []runtimeTraceProjIOFoldPeer{
		{Token: "io_burst_episode", ImpactMS: 226.153, EvidenceTag: "E2"},
		{Token: "io_wait", ImpactMS: 112.011, EvidenceTag: "E3"},
		{Token: "block_io_by_inode", ImpactMS: 107.672, EvidenceTag: "E4"},
	}
	over := runtimeTraceProjSelfRowLines(row, 30.0, true)
	if len(over) < 2 {
		t.Fatalf("3-peer caliber note must overflow the self row: %q", over)
	}
	if !strings.HasPrefix(over[0], "│     ☾") {
		t.Fatalf("self main line lead drifted: %q", over[0])
	}
	for _, line := range over[1:] {
		if !strings.HasPrefix(line, "│     · ") && !strings.HasPrefix(line, "│       ") {
			t.Fatalf("self subordinate lines must align under the │ lead: %q", line)
		}
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("self subordinate line over the cap (%d): %q", w, line)
		}
	}
	// Prefix fill kept the short parts inline (the cliff would demote them).
	if !strings.Contains(over[0], "×2同值") {
		t.Fatalf("prefix fill must keep fitting parts on the self main line: %q", over)
	}
}

// --- (b) 类别样板词降维 ------------------------------------------------------------

func TestPTV6DGenericCandidateShapeLeavesRowFace(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "bg-1", Object: "workqueue_activity", ImpactMS: 1.2, Confidence: 0.8,
	}
	row := runtimeTraceProjTreeRow{
		Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 候选影响(类) →
	// 图例「无类型词的行」/明细「未分类(该行无具体状态/类型词)」 (根因族;
	// 候选影响 retired from EVERY face — the row-face ban below stays and now
	// covers the legend and detail faces too).
	joined := strings.Join(ptv6cRowTagTexts(row), " · ")
	if strings.Contains(joined, "候选影响") || strings.Contains(joined, "无主导态") {
		t.Fatalf("category words must stay off the row face: %s", joined)
	}
	if !row.marks.has(runtimeTraceProjMarkCandidateShapeClass) {
		t.Fatalf("generic shape suppression must record the legend mark")
	}
	if row.marks.has(runtimeTraceProjMarkStateLabel) {
		t.Fatalf("a suppressed generic shape must not claim the state-label lane")
	}
	// Legend carrier renders exactly on the mark (NEW-7).
	legend := strings.Join(runtimeTraceProjLegendGroupLines(row.marks, true), "\n")
	if !strings.Contains(legend, "- 无类型词的行 = 未识别出具体影响类型;逐行影响形态见明细。") {
		t.Fatalf("the candidate-class legend entry must self-explain the type-less rows:\n%s", legend)
	}
	if strings.Contains(legend, "候选影响") {
		t.Fatalf("the retired 候选影响 word must stay off the legend face:\n%s", legend)
	}
	// Block face: the detail-table cell self-describes; the retired word is
	// banned there too.
	got := runtimeTraceCausalProjectionImpactShapeCell(node, true)
	if got != "未分类(该行无具体状态/类型词)" {
		t.Fatalf("detail shape cell must self-describe the unclassified row: %q", got)
	}
	if strings.Contains(got, "候选影响") {
		t.Fatalf("the retired 候选影响 word must stay off the block face: %q", got)
	}
	// 负向臂: a TYPED shape keeps the state-label lane untouched.
	irq := node
	irq.Object, irq.TypeToken = "irq_burst", "irq_burst"
	irqRow := runtimeTraceProjTreeRow{
		Node: irq, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	irqJoined := strings.Join(ptv6cRowTagTexts(irqRow), " · ")
	if !strings.Contains(irqJoined, "IRQ突发") {
		t.Fatalf("typed shapes keep their inline word: %s", irqJoined)
	}
	if irqRow.marks.has(runtimeTraceProjMarkCandidateShapeClass) || !irqRow.marks.has(runtimeTraceProjMarkStateLabel) {
		t.Fatalf("typed shapes must ride the state-label lane, not the generic mark")
	}
}

func TestPTV6DSelfRowGenericShapeSuppressed(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "self-1", Object: "workqueue_activity", ImpactMS: 2.0, Confidence: 0.8,
	}
	row := runtimeTraceProjTreeRow{
		Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	main, demoted := runtimeTraceProjSelfRowParts(row, 0, true)
	all := strings.Join(append(append([]string(nil), main...), demoted...), " · ")
	if strings.Contains(all, "候选影响") {
		t.Fatalf("self rows drop the generic category word too: %s", all)
	}
	if !row.marks.has(runtimeTraceProjMarkCandidateShapeClass) {
		t.Fatalf("self-row suppression must record the same legend mark")
	}
	// 负向臂: a typed state label stays on the self row.
	typed := node
	typed.StateKind = "runnable"
	typedRow := runtimeTraceProjTreeRow{
		Node: typed, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	_, typedDemoted := runtimeTraceProjSelfRowParts(typedRow, 0, true)
	if !strings.Contains(strings.Join(typedDemoted, " · "), "runnable") {
		t.Fatalf("typed self-row state labels are untouched: %v", typedDemoted)
	}
}

func TestPTV6DNoDominantChipRetiredIconAndLegendCarry(t *testing.T) {
	// PTV8-RCR-A §24.3 EVOLUTION RECORD: workqueue_activity now belongs to
	// the ↯ interrupt-activity family (typed token 归族) — the ◦ fallback arm
	// needs a row with NO state and NO typed family.
	interrupt := types.TraceCausalProjectionNode{
		Subject: "bg-1", Object: "workqueue_activity", ImpactMS: 1.2, Confidence: 0.8,
	}
	if icon := runtimeTraceProjStateIcon(interrupt, runtimeTraceProjTreeRowBackground, true, &runtimeTraceProjMarkSet{}); icon != "↯" {
		t.Fatalf("workqueue row icon = %q, want ↯ (§24.3 中断活动族)", icon)
	}
	node := types.TraceCausalProjectionNode{
		Subject: "bg-1", ImpactMS: 1.2, Confidence: 0.8,
	}
	marks := &runtimeTraceProjMarkSet{}
	if icon := runtimeTraceProjStateIcon(node, runtimeTraceProjTreeRowBackground, true, marks); icon != "◦" {
		t.Fatalf("stateless data row icon = %q, want ◦", icon)
	}
	if !marks.has(runtimeTraceProjMarkIconNoDominant) {
		t.Fatalf("the ◦ no-dominant mark records at the icon emission arm")
	}
	// PTV8-RCR-B (UXA 横扫批, 2026-07-08). EVOLUTION RECORD: 无主导态 →
	// 该行无主导调度状态 (图例记号族: chip class word 直陈, 行内仍禁词).
	legend := strings.Join(runtimeTraceProjLegendGroupLines(marks, true), "\n")
	if !strings.Contains(legend, "`◦`(数据行) = 该行无主导调度状态;具体影响形态见行内说明或明细。") {
		t.Fatalf("the ◦ legend entry must carry the retired chip's class semantics:\n%s", legend)
	}
}

// --- (c) 链宇宙等值折 --------------------------------------------------------------

func TestPTV6DChainEqualCumEffFoldsToAttribution(t *testing.T) {
	for _, kind := range []string{runtimeTraceProjTreeRowChain, runtimeTraceProjTreeRowCause, runtimeTraceProjTreeRowDepthless} {
		row := runtimeTraceProjTreeRow{
			Node: ptv6cStanzaNode(), // impact 10, cum 18, eff 18 — equal pair
			Kind: kind, HasData: true, marks: &runtimeTraceProjMarkSet{},
		}
		joined := strings.Join(ptv6cRowTagTexts(row), " · ")
		if strings.Contains(joined, "链上累计") {
			t.Fatalf("kind %s: the equal-value 链上累计 copy must fold (one measurement, one tag): %s", kind, joined)
		}
		if strings.Count(joined, "有效归因18.000ms") != 1 {
			t.Fatalf("kind %s: the Q1 有效归因 tag is the surviving carrier: %s", kind, joined)
		}
	}
	// 边界: a periodic source keeps its cum tag — the VS-1 tag's embedded
	// 有效归因 is a DISCOUNTED caliber, an equal cum is a different-caliber
	// coincidence, never the same measurement.
	periodic := ptv6cStanzaNode()
	periodic.PeriodicSource = true
	periodic.DetectedPeriodMS = 5
	row := runtimeTraceProjTreeRow{
		Node: periodic, Kind: runtimeTraceProjTreeRowChain, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	joined := strings.Join(ptv6cRowTagTexts(row), " · ")
	if !strings.Contains(joined, "链上累计18.000ms") {
		t.Fatalf("periodic rows keep the cum tag (VS-1 caliber is discounted, not equal): %s", joined)
	}
}

// --- (d) irq 族密度词 --------------------------------------------------------------

func TestPTV6DCrossThreadConcurrencyTokenUniverse(t *testing.T) {
	mk := func(token string) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{TypeToken: token}
	}
	for _, token := range []string{"irq_burst", "irq_activity", "ipi_activity"} {
		if !runtimeTraceProjCrossThreadConcurrencyToken(mk(token)) {
			t.Fatalf("%s must ride the concurrency density word", token)
		}
	}
	for _, token := range []string{"io_pressure", "cpu_frequency_limit", "supply_pressure", "cpu_pressure"} {
		if runtimeTraceProjCrossThreadConcurrencyToken(mk(token)) {
			t.Fatalf("%s must stay off the concurrency fork", token)
		}
	}
	// The queue-depth lane is untouched (先例 face).
	if !runtimeTraceProjCrossThreadQueueDepthToken(mk("supply_pressure")) {
		t.Fatalf("supply_pressure keeps the queue-depth word")
	}
}

// --- (e) 标本重放行数账 + 信息零丢失自证 ---------------------------------------------

func TestPTV6DSpecimenReplayLineLedger(t *testing.T) {
	type ledger struct {
		name        string
		records     []types.ObservationRecord
		lines       int // fence total incl. the two ``` markers
		tree        int
		adjacent    int
		background  int
		evidence    []string
		inventory   []string // every surviving tag/word — 打包非折叠 self-proof
		beforeLines int      // pre-PTV6-D total (ledger record, not asserted)
	}
	cases := []ledger{
		{
			name: "specimen1", records: ptv6Specimen1Records(),
			// PTV8-RCR-A EVOLUTION RECORD (§24.1/§24.2): +3 lines = the three
			// ranked rows' 行2 identity lines (类别·根因排序#N·置信); the
			// retired 候选根因 chip left the inventory, the degenerate
			// 有效归因 tail gained its (全额) caliber.
			lines: 30, tree: 1, adjacent: 2, background: 7, beforeLines: 46,
			evidence: []string{"[E1(+1)]", "[E2]", "[E3]", "[E4]", "[E5]", "[E6]", "[E7]", "[E8(+1)]", "[E9]", "[E10]"},
			inventory: []string{
				"runnable", "链上L1", "×2同值", "有效归因 1.661ms(全额)",
				"就绪排队候选·根因排序#1·置信高",
				"IRQ突发·根因排序#4·置信高", "IRQ活动·根因排序#12·置信高", "累计(跨线程)1.997ms",
				"IO等待(对端 udk-irq-3-65)", "D-state/iowait(对端未解析)",
				"IO等待(对端 udk-irq-1-63)", "×2(0.081–1.302ms)取最大",
				"IO等待(对端 udk-irq-4-67)",
			},
		},
		{
			name: "specimen2", records: ptv6Specimen2Records(),
			// PTV8-RCR-A (§24.1): +2 lines = the two ranked rows' 行2 lines.
			lines: 17, tree: 2, adjacent: 0, background: 3, beforeLines: 23,
			evidence: []string{"[E1(+1)]", "[E2]", "[E3]", "[E4]", "[E5]"},
			// b3 第三标本修 (2026-07-06): 调度等待 leaves the inversion trunk
			// row at SOURCE (ActionCell category word suppressed on inversion
			// rows) — not a packing loss; the negative arm lives in the ptv6c
			// specimen pins.
			inventory: []string{
				"优先级反转候选·根因排序#1·置信高·链上L1·有效归因 1.661ms(全额)",
				"影响点 可运行等待反转（priority_inversion_runnable_wait）",
				"IO等待(对端 udk-irq-3-65)", "D-state/iowait(对端未解析)",
				"IO等待(对端 udk-irq-1-63)",
			},
		},
	}
	for _, tc := range cases {
		projection := types.TraceCausalProjectionFromObservationRecords(tc.records)
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		fence := runtimeTraceProjTreeFence(model, true)
		lines := strings.Split(fence, "\n")
		// 行数账: the packed geometry (specimen1 46→27, specimen2 23→15 incl.
		// fence markers). A regression to per-tag lines re-inflates this.
		if len(lines) != tc.lines {
			t.Fatalf("%s: fence is %d lines, ledger pins %d (was %d pre-PTV6-D):\n%s",
				tc.name, len(lines), tc.lines, tc.beforeLines, fence)
		}
		// 100 列全保 across every packed form.
		for i, line := range lines {
			if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("%s: line %d over the row cap (%d cells): %q", tc.name, i, w, line)
			}
		}
		// 节点数不变 = 信息零丢失自证 (packing moves lines, never rows).
		if len(model.TreeRows) != tc.tree || len(model.Adjacent) != tc.adjacent || len(model.Background) != tc.background {
			t.Fatalf("%s: row census drifted: tree=%d adjacent=%d background=%d",
				tc.name, len(model.TreeRows), len(model.Adjacent), len(model.Background))
		}
		for _, tag := range tc.evidence {
			if !strings.Contains(fence, tag) {
				t.Fatalf("%s: evidence tag %s lost in packing:\n%s", tc.name, tag, fence)
			}
		}
		for _, word := range tc.inventory {
			if !strings.Contains(fence, word) {
				t.Fatalf("%s: tag content %q lost in packing (打包非折叠 red line):\n%s", tc.name, word, fence)
			}
		}
		// (b) sanctioned category-word removals — carried by the legend, off
		// the fence; the 空 │ 分隔行 survives (1-行成本裁定).
		for _, banned := range []string{"无主导态", "候选影响"} {
			if strings.Contains(fence, banned) {
				t.Fatalf("%s: category word %q back on the fence:\n%s", tc.name, banned, fence)
			}
		}
		if !strings.Contains(fence, "\n│\n") {
			t.Fatalf("%s: the │ separator line must survive:\n%s", tc.name, fence)
		}
	}
}
