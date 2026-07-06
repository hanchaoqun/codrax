package tool

// answer_document_projection_ptv6c_test.go — PTV6-C 批 (#73, 词面 lane 收敛 +
// 指路句族, 用户裁定 2026-07-06; 标本归因 #3/#5/#6/#7/#8/#12/#13).
//
// 裁定与归因项:
//   A  (#5)  — Q1 有效归因常显 tag 限链宇宙行 (chain/cause/depthless); ◇/▒ 行
//              同数据改 累计(跨线程) 族词 (C00 词表按 row kind 分流, 单一
//              wording home: runtimeTraceProjCrossThreadCumWord/TagText).
//   B  (#12) — 反转影响 shape 词删除; cause 全词 优先级反转候选 占词位, 名称位
//              截断时 #12 全词保障把它放上从属行首条; D3 影响构成 必显不变.
//   C        — "见原始 trace_query 记录" 指路句族全审计: 发射点全部内联坐标 /
//              改 trace 源坐标 / 改说法; 中间记录文件不再作为用户面指路目标
//              (字面量级防回潮 pin 在本文件).
//   #3       — TypeToken→状态族 typed 映射 (显示层): d_state_or_io_wait 等带
//              状态语义的 type token 在 StateKind 空时给出状态词+状态 icon,
//              并抑制 ◦ 无主导态 chip.
//   #6/#13   — 近义收敛: StateKindLabel 吸收 ActionCell 同族词 (可运行等待 吸收
//              调度等待); 影响点 token 走 D4 中文（token）形态; 全词一处 dedupe.
//   #7       — resolved-peer 关系形态: "IO等待(对端 udk-irq-1-63)" 族
//              (ResolvedPeerText, 与 UnresolvedPeerText 同 wording home).
//   #8       — sleep 自身行走 StateKindLabel (☾ 睡眠等待); raw token 留 (a)/(b)
//              审计面.
//
// MUTATIONS pinned here (突变即红):
//   - 恢复 ◇/▒ 行 有效归因/链上累计 词面 → ruling-A 负向臂红;
//   - 反转影响 / "见原始 trace_query 记录" 等删除词以任何 string literal 回归
//     production 文件 → AST 字面量扫描红;
//   - 摘除 #3 类型 token 状态映射 → 无主导态 chip 回现红;
//   - 摘除 #12 全词保障 → 截断行丢全词红;
//   - 恢复 ActionCell 同族词双打 → 近义收敛负向臂红.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// --- ruling A (#5): ◇/▒ 行 累计(跨线程) 族词 ------------------------------------

func ptv6cStanzaNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Subject: "adj-5", Object: "running_burst", StateKind: "running",
		ImpactMS: 10, CumulativeImpactMS: 18, EffectiveImpactMS: 18, Confidence: 0.8,
	}
}

func ptv6cRowTagTexts(row runtimeTraceProjTreeRow) []string {
	_, tags := runtimeTraceProjRowMetricParts(row, 100, true, true)
	texts := make([]string, 0, len(tags))
	for _, tag := range tags {
		texts = append(texts, tag.Text)
	}
	return texts
}

func TestPTV6CRulingAStanzaRowsWearCrossThreadCumFamily(t *testing.T) {
	for _, kind := range []string{runtimeTraceProjTreeRowAdjacent, runtimeTraceProjTreeRowBackground} {
		row := runtimeTraceProjTreeRow{Node: ptv6cStanzaNode(), Kind: kind, HasData: true, marks: &runtimeTraceProjMarkSet{}}
		texts := ptv6cRowTagTexts(row)
		joined := strings.Join(texts, " · ")
		// 正向臂: 同数据以 累计(跨线程) 族词渲染; 等值 cum/effective 去重为一条.
		if strings.Count(joined, "累计(跨线程)18.000ms") != 1 {
			t.Fatalf("%s row must carry exactly one 累计(跨线程) tag: %v", kind, texts)
		}
		// 负向臂 (突变: 恢复链宇宙词面即红).
		for _, banned := range []string{"有效归因", "链上累计"} {
			if strings.Contains(joined, banned) {
				t.Fatalf("%s row must not wear the chain-universe word %q: %v", kind, banned, texts)
			}
		}
		if !row.marks.has(runtimeTraceProjMarkStanzaCrossThreadCum) {
			t.Fatalf("%s row must record the 累计(跨线程) legend mark", kind)
		}
	}
}

func TestPTV6CRulingAChainUniverseKeepsAttributionWords(t *testing.T) {
	for _, kind := range []string{runtimeTraceProjTreeRowChain, runtimeTraceProjTreeRowCause, runtimeTraceProjTreeRowDepthless} {
		node := ptv6cStanzaNode()
		node.EffectiveImpactMS = 5 // != cum, both words must show
		row := runtimeTraceProjTreeRow{Node: node, Kind: kind, HasData: true, marks: &runtimeTraceProjMarkSet{}}
		joined := strings.Join(ptv6cRowTagTexts(row), " · ")
		for _, want := range []string{"链上累计18.000ms", "有效归因5.000ms"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("chain-universe kind %s must keep %q: %s", kind, want, joined)
			}
		}
		if strings.Contains(joined, "累计(跨线程)") {
			t.Fatalf("chain-universe kind %s must not wear the stanza family word: %s", kind, joined)
		}
	}
	// 限链宇宙: semantic rows carry NEITHER vocabulary (ruling A universe gate).
	node := ptv6cStanzaNode()
	node.CumulativeImpactMS = 0
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowSemantic, HasData: true, marks: &runtimeTraceProjMarkSet{}}
	joined := strings.Join(ptv6cRowTagTexts(row), " · ")
	for _, banned := range []string{"有效归因", "累计(跨线程)"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("semantic rows sit outside the Q1 universe (%q): %s", banned, joined)
		}
	}
}

func TestPTV6CRulingAFallbackCaliberWordSplitsByRowKind(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "bg-1", Object: "running_burst", StateKind: "running",
		CumulativeImpactMS: 7, Confidence: 0.8, // no window projection → fallback
	}
	if got := runtimeTraceProjRowFallbackCaliberWord(node, runtimeTraceProjTreeRowBackground, true); got != "累计(跨线程)" {
		t.Fatalf("stanza fallback caliber word must ride the family: %q", got)
	}
	if got := runtimeTraceProjRowFallbackCaliberWord(node, runtimeTraceProjTreeRowChain, true); got != "链上累计" {
		t.Fatalf("chain fallback caliber word unchanged: %q", got)
	}
	// 实际状态 is not an attribution word — stanza rows keep it.
	actual := types.TraceCausalProjectionNode{Subject: "bg-2", ActualImpactMS: 3, Confidence: 0.8}
	if got := runtimeTraceProjRowFallbackCaliberWord(actual, runtimeTraceProjTreeRowBackground, true); got != "实际状态" {
		t.Fatalf("actual-state caliber word must not remap: %q", got)
	}
}

// --- #3: TypeToken→状态族 显示层映射 ---------------------------------------------

func TestPTV6CTypeTokenStateFamilyMapping(t *testing.T) {
	cases := []struct {
		token, class, zhWord string
	}{
		{"d_state_or_io_wait", "d_state_or_io_wait", "D状态/IO等待"},
		{"fragmented_d_state_or_io_wait", "d_state_or_io_wait", "D状态/IO等待"},
		{"runnable_wait", "runnable", "可运行等待"},
		{"sleep_wait", "s_sleep", "睡眠等待"},
		{"running", "running", "运行占用"},
		{"io_wait", "io_wait", "IO阻塞"},
	}
	for _, tc := range cases {
		node := types.TraceCausalProjectionNode{Subject: "t-1", Object: "unknown-thread", TypeToken: tc.token}
		if got := runtimeTraceCausalProjectionTypeTokenStateClass(node); got != tc.class {
			t.Fatalf("%s class = %q, want %q", tc.token, got, tc.class)
		}
		if got := runtimeTraceCausalProjectionTypeTokenStateWord(runtimeTraceCausalProjectionTypeTokenStateClass(node), true); got != tc.zhWord {
			t.Fatalf("%s zh state word = %q, want %q", tc.token, got, tc.zhWord)
		}
		// ◦ 无主导态 chip 抑制 (负向臂: 摘除映射即回现).
		if runtimeTraceProjNoDominantStateRow(node, runtimeTraceProjTreeRowBackground) {
			t.Fatalf("%s row must not wear the 无主导态 chip", tc.token)
		}
	}
	// 边界负向臂: io_latency is a measurement family, NOT a state token — the
	// chip stays (precise membership; do not creep).
	ioLat := types.TraceCausalProjectionNode{Subject: "t-2", Object: "udk-irq-1-63", TypeToken: "io_latency"}
	if runtimeTraceCausalProjectionTypeTokenStateClass(ioLat) != "" {
		t.Fatalf("io_latency must stay out of the state-family map")
	}
	if !runtimeTraceProjNoDominantStateRow(ioLat, runtimeTraceProjTreeRowBackground) {
		t.Fatalf("io_latency row keeps the 无主导态 chip (boundary pin)")
	}
	// producer 不动: a published StateKind always wins over the type lane.
	published := types.TraceCausalProjectionNode{Subject: "t-3", TypeToken: "d_state_or_io_wait", StateKind: "runnable"}
	if runtimeTraceCausalProjectionTypeTokenStateClass(published) != "" {
		t.Fatalf("a producer-published StateKind must never be overridden by the type lane")
	}
}

func TestPTV6CTypeTokenStateFamilyIconAndShape(t *testing.T) {
	node := types.TraceCausalProjectionNode{Subject: "t-1", Object: "unknown-thread", TypeToken: "d_state_or_io_wait"}
	marks := &runtimeTraceProjMarkSet{}
	if icon := runtimeTraceProjStateIcon(node, runtimeTraceProjTreeRowBackground, true, marks); icon != "⛓" {
		t.Fatalf("d_state_or_io_wait row icon = %q, want ⛓", icon)
	}
	if !marks.has(runtimeTraceProjMarkIconDState) || marks.has(runtimeTraceProjMarkIconNoDominant) {
		t.Fatalf("icon mark must be the state family, never the ◦ no-dominant sense")
	}
	if got := runtimeTraceCausalProjectionImpactShapeCell(node, true); got != "D状态/IO等待" {
		t.Fatalf("shape cell must speak the state family word: %q", got)
	}
	if got := runtimeTraceCausalProjectionImpactShapeCell(node, false); got != "D-state/IO wait" {
		t.Fatalf("EN shape cell must mirror: %q", got)
	}
}

// --- #7: resolved-peer 关系形态 -------------------------------------------------

func TestPTV6CResolvedPeerRelationForm(t *testing.T) {
	cases := []struct {
		typeToken, peer, zh, en string
	}{
		{"io_latency", "udk-irq-1-63", "IO等待(对端 udk-irq-1-63)", "IO wait (peer udk-irq-1-63)"},
		{"d_state_or_io_wait", "udk-irq-3-65", "D状态/IO等待(对端 udk-irq-3-65)", "D-state/IO wait (peer udk-irq-3-65)"},
		{"blocking_span", "worker-7", "阻塞等待(对端 worker-7)", "blocking wait (peer worker-7)"},
	}
	for _, tc := range cases {
		node := types.TraceCausalProjectionNode{
			Subject: "sysevent_store-47924", Object: tc.peer,
			TypeToken: tc.typeToken, Predicate: "critical_blocking",
		}
		if got := runtimeTraceCausalProjectionDisplayCauseNameNode(node, true); got != tc.zh {
			t.Fatalf("%s resolved peer zh = %q, want %q", tc.typeToken, got, tc.zh)
		}
		if got := runtimeTraceCausalProjectionDisplayCauseNameNode(node, false); got != tc.en {
			t.Fatalf("%s resolved peer en = %q, want %q", tc.typeToken, got, tc.en)
		}
	}
	// 同 wording home: the unresolved arm is untouched (只增不减).
	unresolved := types.TraceCausalProjectionNode{
		Subject: "BdAsyncTask #8-59953", Object: "unknown-thread",
		TypeToken: "d_state_or_io_wait", Predicate: "critical_blocking",
	}
	if got := runtimeTraceCausalProjectionDisplayCauseNameNode(unresolved, true); got != "D状态/IO等待(对端未解析)" {
		t.Fatalf("unresolved arm drifted: %q", got)
	}
	// Guards: the Object must actually be a peer thread — state tokens, type
	// tokens and peer-kind echoes never convert (裸词位负向臂).
	for _, object := range []string{"runnable", "s_sleep", "io_latency", "d_state_or_io_wait", "priority_inversion_candidate"} {
		node := types.TraceCausalProjectionNode{
			Subject: "w-1", Object: object,
			TypeToken: "d_state_or_io_wait", Predicate: "critical_blocking",
		}
		if got := runtimeTraceCausalProjectionDisplayCauseNameNode(node, true); strings.Contains(got, "(对端 ") {
			t.Fatalf("non-thread object %q must not wear the resolved-peer form: %q", object, got)
		}
	}
}

// --- #8: sleep 自身行 StateKindLabel ---------------------------------------------

func TestPTV6CSleepSelfRowSpeaksStateKindLabel(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "com.baidu.tieba-59566", Object: "s_sleep",
			StateKind: "s_sleep", ImpactMS: 2.978, Confidence: 0.8,
		},
		Kind: runtimeTraceProjTreeRowSelf, HasData: true, marks: &runtimeTraceProjMarkSet{},
	}
	main, _ := runtimeTraceProjSelfRowParts(row, 2.992, true)
	if len(main) == 0 || main[0] != "☾ 睡眠等待" {
		t.Fatalf("zh sleep self row must lead with the StateKindLabel: %v", main)
	}
	mainEN, _ := runtimeTraceProjSelfRowParts(row, 2.992, false)
	if len(mainEN) == 0 || mainEN[0] != "☾ sleep wait" {
		t.Fatalf("en sleep self row must lead with the StateKindLabel: %v", mainEN)
	}
	// 防回潮: the raw scheduler token never rides the self line again.
	for _, part := range append(append([]string(nil), main...), mainEN...) {
		if strings.Contains(part, "s_sleep") {
			t.Fatalf("raw s_sleep token resurfaced on the self line: %v", part)
		}
	}
}

// --- #6: 近义收敛 (ActionCell 吸收 + 影响点 D4) ----------------------------------

func TestPTV6CStateLabelAbsorbsActionRestatement(t *testing.T) {
	families := []struct {
		state, stateWord, actionWord string
	}{
		{"runnable", "可运行等待", "调度等待"},
		{"running", "运行占用", "执行/算力"},
		{"io_wait", "IO阻塞", "阻塞/IO"},
	}
	for _, f := range families {
		node := types.TraceCausalProjectionNode{
			Subject: "w-1", Object: "some_cause_word", StateKind: f.state,
			ImpactMS: 5, Confidence: 0.8,
		}
		row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true, marks: &runtimeTraceProjMarkSet{}}
		joined := strings.Join(ptv6cRowTagTexts(row), " · ")
		if !strings.Contains(joined, f.stateWord) {
			t.Fatalf("state %s must keep its StateKindLabel %q: %s", f.state, f.stateWord, joined)
		}
		if strings.Contains(joined, f.actionWord) {
			t.Fatalf("state %s: action restatement %q must be absorbed (近义收敛): %s", f.state, f.actionWord, joined)
		}
	}
	// ActionCell 本词未删 (Q4 词面归 ActionCell home; 只在双打时被吸收).
	if got := runtimeTraceCausalProjectionActionCell(types.TraceCausalProjectionNode{Subject: "w", Object: "x", StateKind: "runnable"}, true); got != "调度等待" {
		t.Fatalf("ActionCell home wording must stay: %q", got)
	}
}

func TestPTV6CImpactPointD4CombinedForm(t *testing.T) {
	cases := []struct{ token, zh, en string }{
		{"runnable", "可运行等待（runnable）", "runnable"},
		{"s_sleep", "睡眠等待（s_sleep）", "s_sleep"},
		{"priority_inversion_runnable_wait", "可运行等待反转（priority_inversion_runnable_wait）", "priority_inversion_runnable_wait"},
		{"priority_inversion_runnable_wait/runnable", "可运行等待反转（priority_inversion_runnable_wait）/可运行等待（runnable）", "priority_inversion_runnable_wait/runnable"},
		{"udk-irq-10-90", "udk-irq-10-90", "udk-irq-10-90"}, // unmapped: verbatim, never fabricated
	}
	for _, tc := range cases {
		if got := runtimeTraceCausalProjectionImpactPointDisplay(tc.token, true); got != tc.zh {
			t.Fatalf("impact point zh(%s) = %q, want %q", tc.token, got, tc.zh)
		}
		if got := runtimeTraceCausalProjectionImpactPointDisplay(tc.token, false); got != tc.en {
			t.Fatalf("impact point en(%s) = %q, want %q", tc.token, got, tc.en)
		}
	}
}

// --- #12: cause 全词保障 (B 裁定融合) --------------------------------------------

func TestPTV6CCauseFullWordGuaranteeOnTruncatedName(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "CookieMonsterCl-59843", Object: "priority_inversion_candidate",
		StateKind: "runnable", PriorityInversionCandidate: true,
		ImpactMS: 1.661, CumulativeImpactMS: 2.262, Confidence: 0.9,
	}
	row := runtimeTraceProjTreeRow{
		Node: node, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
		Depth: 1, HasData: true, EvidenceTag: "E2", marks: &runtimeTraceProjMarkSet{},
	}
	line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelWidth, 2.992, true, true)
	lines := strings.Split(line, "\n")
	if len(lines) < 2 {
		t.Fatalf("truncated-name row must demote tags: %s", line)
	}
	if !strings.Contains(lines[0], "优先级反…") && !strings.Contains(lines[0], "优先级反转…") {
		t.Fatalf("fixture drift: the name cell should truncate across the cause word:\n%s", line)
	}
	// 正向臂: cause 全词整词保障仍在 — PTV6-D (a) 悬崖消除后它抢占主行首个
	// 普通 tag 槽位 (前: 从属行首条 "· 优先级反转候选" — 差表 in the PTV6-D
	// ledger); the injection stays the FIRST ordinary tag, so the whole word
	// rides the main line beside the Keep 记号.
	if !strings.Contains(lines[0], "优先级反转候选 · [E2]") {
		t.Fatalf("#12: the cause full word must ride the main row's first tag slot:\n%s", line)
	}
	// 全词一处: never twice.
	if strings.Count(line, "优先级反转候选") != 1 {
		t.Fatalf("cause full word must render exactly once on the row:\n%s", line)
	}
	// PTV6-D (a): the remaining demoted tags pack into ONE subordinate stream.
	if !strings.Contains(lines[1], "· 链上L1 · 调度等待 · 链上累计2.262ms") {
		t.Fatalf("demoted tags must pack into one subordinate stream:\n%s", line)
	}
	// B 裁定防回潮.
	if strings.Contains(line, "反转影响") {
		t.Fatalf("deleted 反转影响 shape word resurfaced:\n%s", line)
	}

	// 突变臂 (摘除 #12 注入必红): a NON-inversion row whose cause word is NOT
	// carried by any tag — the injection is the ONLY carrier of the full word
	// once the name cell truncates across it.
	ioNode := types.TraceCausalProjectionNode{
		Subject: "NetworkKit_AssetsUtil_Operate_0-42067", Object: "block_io_by_inode",
		StateKind: "io_wait", ImpactMS: 1.2, Confidence: 0.9,
	}
	ioRow := runtimeTraceProjTreeRow{
		Node: ioNode, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
		Depth: 1, HasData: true, EvidenceTag: "E9", marks: &runtimeTraceProjMarkSet{},
	}
	ioLine := runtimeTraceProjTreeRowLine(ioRow, runtimeTraceProjTreeLabelWidth, 2.992, true, true)
	// Fixture drift check, PTV6-D form: the long subject still mid-truncates
	// the NAME cell (pid tail kept, cause suffix cut) — the full word can only
	// come back through the #12 injection lane.
	if !strings.Contains(strings.Split(ioLine, "\n")[0], "…-42067") {
		t.Fatalf("fixture drift: the long subject should truncate the name cell:\n%s", ioLine)
	}
	// PTV6-D (a): the injected whole word rides the main row's first tag slot
	// (前: "· 块设备IO(inode)" 从属行 — 差表 in the PTV6-D ledger).
	if !strings.Contains(strings.Split(ioLine, "\n")[0], "块设备IO(inode) · [E9]") {
		t.Fatalf("#12: the truncated cause word must return WHOLE on the main tag slot:\n%s", ioLine)
	}
	if strings.Count(ioLine, "块设备IO(inode)") != 1 {
		t.Fatalf("cause full word must render exactly once:\n%s", ioLine)
	}
}

func TestPTV6CCauseWordDedupeWhenNameShowsIt(t *testing.T) {
	// Short name → the cause word survives whole on the name cell; the equal
	// state tag folds away (全词一处+数据一处).
	node := types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "runnable_wait", StateKind: "runnable",
		ImpactMS: 1.6, Confidence: 0.9,
	}
	row := runtimeTraceProjTreeRow{
		Node: node, Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
		Depth: 1, HasData: true, EvidenceTag: "E1", marks: &runtimeTraceProjMarkSet{},
	}
	line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelWidth, 2.992, true, true)
	if !strings.Contains(line, "w-1 · 可运行等待") {
		t.Fatalf("fixture drift: name cell should show the full cause word:\n%s", line)
	}
	if strings.Count(line, "可运行等待") != 1 {
		t.Fatalf("cause word must appear exactly once (dedupe): got %d in\n%s",
			strings.Count(line, "可运行等待"), line)
	}
}

// --- 修正轮 (2026-07-06, 12 confirmed): typed dedupe / 折算 / 空转门 pins ---------

// [Med 同根×2] the #6/#12 dedupe judges TYPED identity, never display-string
// containment: the EN state tag survives beside a merely-overlapping cause
// word (running ⊂ running_burst was eaten), and the EN runnable_wait double
// folds exactly like the zh face (双面同判).
func TestPTV6CTypedDedupeJudgesTokensNotDisplayStrings(t *testing.T) {
	// Tag-level probe (a fence-level Contains would be masked by the name
	// cell: "· running_burst" itself contains "· running").
	guaranteed := func(object, state string, zh bool) []string {
		row := runtimeTraceProjTreeRow{
			Node: types.TraceCausalProjectionNode{
				Subject: "w-1", Object: object, StateKind: state,
				ImpactMS: 5, Confidence: 0.9,
			},
			Kind: runtimeTraceProjTreeRowChain, Edge: runtimeTraceProjTreeEdgeDrill,
			Depth: 1, HasData: true, EvidenceTag: "E1", marks: &runtimeTraceProjMarkSet{},
		}
		// left simulates an UNTRUNCATED name cell (the dedupe arm, not the
		// injection arm) — per-language display cause word, as RowName composes.
		left := "└─下钻─ ⚙ w-1 · " + runtimeTraceCausalProjectionDisplayCauseNameNode(row.Node, zh)
		_, tags := runtimeTraceProjRowMetricParts(row, 100, true, zh)
		texts := []string{}
		for _, tag := range runtimeTraceProjApplyCauseWordGuarantee(left, row, tags, zh) {
			texts = append(texts, tag.Text)
		}
		return texts
	}
	contains := func(texts []string, want string) bool {
		for _, text := range texts {
			if text == want {
				return true
			}
		}
		return false
	}
	// EN face: "running" ⊂ "running_burst" as display strings, but the tokens
	// differ (running_burst has no state class) — the 裁定4 state tag MUST
	// survive on BOTH faces (突变臂: substring 判据回归即红).
	if texts := guaranteed("running_burst", "running", false); !contains(texts, "running") {
		t.Fatalf("EN running_burst row must keep its state tag: %v", texts)
	}
	if texts := guaranteed("running_burst", "running", true); !contains(texts, "运行占用") {
		t.Fatalf("zh running_burst row must keep its state tag: %v", texts)
	}
	// EN runnable_wait: tokens agree by state family (runnable_wait ≡
	// runnable) — the double folds on BOTH faces even though the display
	// strings never contain each other ("runnable wait" vs "runnable_wait").
	if texts := guaranteed("runnable_wait", "runnable", false); contains(texts, "runnable wait") {
		t.Fatalf("EN runnable_wait double must fold (typed state-family identity): %v", texts)
	}
	if texts := guaranteed("runnable_wait", "runnable", true); contains(texts, "可运行等待") {
		t.Fatalf("zh runnable_wait double must fold: %v", texts)
	}
}

// [Med] ruling-A cum≠eff stanza double: the second value wears the 折算
// discriminator word; equal values still fold to ONE tag.
func TestPTV6CStanzaDiscountedDiscriminator(t *testing.T) {
	node := ptv6cStanzaNode()
	node.EffectiveImpactMS = 5 // ≠ cum 18
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, marks: &runtimeTraceProjMarkSet{}}
	joined := strings.Join(ptv6cRowTagTexts(row), " · ")
	for _, want := range []string{"累计(跨线程)18.000ms", "折算 5.000ms"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cum≠eff stanza row must discriminate the second value (%q): %s", want, joined)
		}
	}
	if strings.Count(joined, "累计(跨线程)") != 1 {
		t.Fatalf("同词异值无判别 shape resurfaced: %s", joined)
	}
	// EN 对等.
	_, tags := runtimeTraceProjRowMetricParts(row, 100, true, false)
	var en []string
	for _, tag := range tags {
		en = append(en, tag.Text)
	}
	if joinedEN := strings.Join(en, " · "); !strings.Contains(joinedEN, "discounted 5.000ms") {
		t.Fatalf("EN stanza discriminator missing: %s", joinedEN)
	}
	// Equal values keep folding to one tag (existing ruling-A pin re-asserted
	// here as the discriminator's negative arm).
	equal := runtimeTraceProjTreeRow{Node: ptv6cStanzaNode(), Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, marks: &runtimeTraceProjMarkSet{}}
	if joinedEq := strings.Join(ptv6cRowTagTexts(equal), " · "); strings.Contains(joinedEq, "折算") ||
		strings.Count(joinedEq, "累计(跨线程)18.000ms") != 1 {
		t.Fatalf("equal cum/eff must fold to one family tag: %s", joinedEq)
	}
}

// [Low] 承自归因 is chain-universe vocabulary: a stanza row with an inherited
// effective value falls to the 累计(跨线程) family instead.
func TestPTV6CInheritedTagGatedToChainUniverse(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "bg-9", Object: "runnable_wait", StateKind: "runnable",
		ImpactMS: 2, CumulativeImpactMS: 0.1, EffectiveImpactMS: 12, Confidence: 0.8, // eff > 10×cum → inherited
	}
	chain := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true, marks: &runtimeTraceProjMarkSet{}}
	if joined := strings.Join(ptv6cRowTagTexts(chain), " · "); !strings.Contains(joined, "承自归因12.000ms") {
		t.Fatalf("chain inherited row keeps 承自归因: %s", joined)
	}
	stanza := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowBackground, HasData: true, marks: &runtimeTraceProjMarkSet{}}
	joined := strings.Join(ptv6cRowTagTexts(stanza), " · ")
	if strings.Contains(joined, "承自归因") {
		t.Fatalf("stanza rows must not wear 承自归因: %s", joined)
	}
	if !strings.Contains(joined, "折算 12.000ms") && !strings.Contains(joined, "累计(跨线程)12.000ms") {
		t.Fatalf("stanza inherited data must survive under the family words: %s", joined)
	}
}

// [Low] cross-thread aggregates already carry the cpu·ms unit suffix — the
// plain-ms cum/eff/inherited tag family idles entirely (同 caliber word 门).
func TestPTV6CAggregateRowsIdleTheCumEffTagFamily(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject: "unknown-thread", Object: "irq_burst", TypeToken: "irq_burst",
		SubjectKind: types.TraceCausalSubjectKindAggregateMetric,
		ImpactMS:    6.417, CumulativeImpactMS: 2.947, EffectiveImpactMS: 1.997, Confidence: 0.8,
	}
	if !runtimeTraceProjCrossThreadAggregateType(node) {
		t.Fatalf("fixture drift: node must classify as a cross-thread aggregate")
	}
	row := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowAdjacent, HasData: true, marks: &runtimeTraceProjMarkSet{}}
	joined := strings.Join(ptv6cRowTagTexts(row), " · ")
	for _, banned := range []string{"累计(跨线程)", "折算", "有效归因", "链上累计", "承自归因"} {
		if strings.Contains(joined, banned) {
			t.Fatalf("aggregate row must not stack a plain-ms tag (%q) on its cpu·ms caliber: %s", banned, joined)
		}
	}
}

// [Low] actual_window inline endpoints go through the strict shared parser —
// a malformed note (end < start) renders no endpoints, never a fabricated
// window; the state values still inline.
func TestPTV6CActualWindowInlineStrictParse(t *testing.T) {
	record := types.ObservationRecord{
		RichNotes: []string{"actual_window=3681.2..3679.1", "actual_running=0.987"},
	}
	if got := runtimeTraceMetricSnapshotActualInline(record, true); got != "实际对齐窗: 运行 0.987ms" {
		t.Fatalf("malformed window must drop endpoints only: %q", got)
	}
	record.RichNotes[0] = "actual_window=3679.899436..3681.129875"
	if got := runtimeTraceMetricSnapshotActualInline(record, true); got != "实际对齐窗 3679.899s–3681.130s: 运行 0.987ms" {
		t.Fatalf("strict-parsed window must inline at %%.3f: %q", got)
	}
}

// --- ruling C: 指路句族字面量防回潮 (AST string literals, production files) -------

var ptv6cRetiredPointerLiterals = []string{
	// zh 家族 (指路半句, 逐字).
	"其余见原始 trace_query 记录",
	"完整清单见原始 trace_query 记录",
	"完整明细见原始 trace_query 记录",
	"完整定位见原始 trace_query 记录",
	"实际对齐窗数值见原始 trace_query 记录",
	"完整链路见原始 trace_query 记录",
	"完整关系见原始 trace_query 记录",
	"完整定位以原始 trace_query 结构化记录为准",
	// en 变体.
	"the rest remain in the raw trace_query records",
	"the full list remains in the raw trace_query records",
	"the full detail remains in the original trace_query records",
	"full locator remains in the original trace_query record",
	"aligned actual-window values remain in the raw trace_query record",
	"the full chain remains in the trace_query record",
	"the full relation stays in the trace_query record",
	"The original trace_query record remains the full locator authority",
}

// TestPTV6CRetiredPointerLiteralsStayRetired walks every PRODUCTION .go file
// of internal/tool and internal/agent and fails on ANY string literal carrying
// a retired pointer sentence — the ruling-C 防回潮 hard pin (删除词回现即红).
// Comments and test files are exempt (this file itself quotes the family).
// The single LLM-facing coverage line (answer_document_evaluator.go, the
// finalizer handoff prompt's "remains available in the raw trace_query
// records and the observation ledger") is NOT in the retired list: it is not
// a user-panel surface and the ledger reference is accurate for that reader.
func TestPTV6CRetiredPointerLiteralsStayRetired(t *testing.T) {
	for _, dir := range []string{".", "../agent"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				for _, banned := range ptv6cRetiredPointerLiterals {
					if strings.Contains(value, banned) {
						t.Errorf("%s: retired pointer literal resurfaced (%q) — PTV6-C ruling C: the intermediate trace_query record is not a user-facing pointer target; inline the data or give trace source coordinates (详见 <basename> 行 X–Y)",
							fset.Position(lit.Pos()), banned)
					}
				}
				return true
			})
		}
	}
}

// TestPTV6CEvidenceCoordinateTail pins the ruling-C replacement tail: the tail
// renders EXACTLY when the display dropped the line range, and it carries the
// trace source coordinate (basename + 行 X–Y), never the retired pointer.
func TestPTV6CEvidenceCoordinateTail(t *testing.T) {
	entry := runtimeTraceCausalProjectionEvidenceEntry{
		ID: "E1", Ref: "/x/y/berlin.systrace:824646-1624260", Window: "[6793222.031–6793225.370s]",
	}
	// Window-preferred display dropped the lines → coordinate tail.
	if got := runtimeTraceProjEvidenceCoordinateTail(entry, "berlin.systrace [6793222.031–6793225.370s]", true); got != "；详见 berlin.systrace 行 824646–1624260" {
		t.Fatalf("zh coordinate tail = %q", got)
	}
	if got := runtimeTraceProjEvidenceCoordinateTail(entry, "berlin.systrace [6793222.031–6793225.370s]", false); got != "; see berlin.systrace lines 824646–1624260" {
		t.Fatalf("en coordinate tail = %q", got)
	}
	// Display already shows the line range → no tail (path-directory trims are
	// deliberate, not information loss).
	if got := runtimeTraceProjEvidenceCoordinateTail(entry, ":824646-1624260", true); got != "" {
		t.Fatalf("line-range display must not grow a tail: %q", got)
	}
	// CMP-7b synthetic-line entries never claim a line coordinate.
	synthetic := entry
	synthetic.SyntheticLine = true
	if got := runtimeTraceProjEvidenceCoordinateTail(synthetic, "berlin.systrace [6793222.031–6793225.370s]", true); got != "" {
		t.Fatalf("synthetic entries must not claim line coordinates: %q", got)
	}
}

// --- 标本重放对照 (两标本三处关键行, replay fixtures shared with PTV6-A/B) --------

func TestPTV6CSpecimen1KeyRowsAfter(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(ptv6Specimen1Records())
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	fence := runtimeTraceProjTreeFence(model, true)
	// 关键行一 (trunk): 名称截断 → 全词 可运行等待 保障仍在; PTV6-D (a) 悬崖
	// 消除后全词升上主行 (前: 独占一条 "· 可运行等待" 从属行 — 差表 in the
	// PTV6-D ledger); 调度等待 双打消失 (前: · 可运行等待 + · 调度等待).
	if !strings.Contains(fence, "可运行等待 · [E1(+1)]") {
		t.Fatalf("trunk row must keep the cause full word (main-row slot after PTV6-D prefix fill):\n%s", fence)
	}
	if strings.Contains(fence, "调度等待") {
		t.Fatalf("近义双打回现 (调度等待 next to 可运行等待):\n%s", fence)
	}
	// PTV6-D (a): the trunk's demoted tags PACK into one "· " stream line
	// (前: 三条逐 tag 从属行 · 链上L1 / · ×2同值 / · 有效归因1.661ms).
	if !strings.Contains(fence, "· 链上L1 · ×2同值 · 有效归因1.661ms") {
		t.Fatalf("trunk demoted tags must pack into one subordinate stream:\n%s", fence)
	}
	// PTV6-D (b): the category words left the row face (legend carries them);
	// every non-category tag above is still present — 打包非折叠.
	for _, banned := range []string{"无主导态", "候选影响"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("category word %q must stay off the fence rows (legend-carried):\n%s", banned, fence)
		}
	}
	// 关键行二 (◇ 邻近): 有效归因 词面在 ◇/▒ 内消失, 改 累计(跨线程)
	// (前: · 有效归因1.997ms on the irq_burst adjacent row).
	if stanzaStart := strings.Index(fence, "◇"); stanzaStart >= 0 {
		stanza := fence[stanzaStart:]
		if strings.Contains(stanza, "有效归因") || strings.Contains(stanza, "链上累计") {
			t.Fatalf("ruling A: stanza rows must not wear chain-universe words:\n%s", stanza)
		}
		if !strings.Contains(stanza, "累计(跨线程)1.997ms") {
			t.Fatalf("ruling A: the same data must render under the family word:\n%s", stanza)
		}
	} else {
		t.Fatalf("specimen 1 replay lost its ◇ stanza:\n%s", fence)
	}
	// 关键行三 (▒ 背景): resolved peer 关系形态 + d_state_or_io_wait 状态族
	// (前: "· udk-irq…-63" 裸词位 + ◦ 无主导态 chip on D状态/IO等待 rows).
	for _, want := range []string{"IO等待(对端 udk-irq-1-63)", "D状态/IO等待(对端未解析)", "⛓ BdAsyncTask #8-59953"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("background stanza missing %q:\n%s", want, fence)
		}
	}
	for _, row := range append(append([]runtimeTraceProjTreeRow(nil), model.Background...), model.Adjacent...) {
		if strings.EqualFold(strings.TrimSpace(row.Node.TypeToken), "d_state_or_io_wait") &&
			runtimeTraceProjNoDominantStateRow(row.Node, row.Kind) {
			t.Fatalf("#3: d_state_or_io_wait row still wears the 无主导态 chip: %+v", row.Node)
		}
	}
	// 证据索引: 指路句族退场, trace 源坐标口径.
	intro, items := runtimeTraceProjEvidenceBlockParts(evidence, true)
	if !strings.Contains(intro, "trace 源坐标") {
		t.Fatalf("evidence intro must declare the coordinate caliber: %s", intro)
	}
	for _, item := range items {
		if strings.Contains(item.Text, "见原始 trace_query 记录") {
			t.Fatalf("retired pointer resurfaced on %s: %s", item.Label, item.Text)
		}
	}
}

func TestPTV6CSpecimen2KeyRowsAfter(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(ptv6Specimen2Records())
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// 关键行一 (trunk): B 裁定 — 反转影响 删除, cause 全词占位, 影响点 D4 形态
	// (前: · 反转影响 + · 影响点 priority_inversion_runnable_wait/runnable;
	// PTV6-D (a) 悬崖消除后全词升上主行 — 差表 in the PTV6-D ledger).
	if !strings.Contains(fence, "优先级反转候选 · [E1(+1)]") {
		t.Fatalf("trunk row must carry the cause full word (main-row slot after PTV6-D prefix fill):\n%s", fence)
	}
	if strings.Contains(fence, "反转影响") {
		t.Fatalf("deleted 反转影响 resurfaced:\n%s", fence)
	}
	if !strings.Contains(fence, "影响点 可运行等待反转（priority_inversion_runnable_wait）") {
		t.Fatalf("影响点 must ride the D4 combined form:\n%s", fence)
	}
	// 关键行二 (成因 row): 名称即全词 → 重复 chip 融掉 (前: 成因行下再打一条
	// · 可运行等待 + · 调度等待).
	causeRowSeen := false
	for _, row := range model.TreeRows {
		if row.Kind != runtimeTraceProjTreeRowCause {
			continue
		}
		causeRowSeen = true
		line := runtimeTraceProjTreeRowLine(row, runtimeTraceProjTreeLabelWidth, 2.992, true, true)
		if strings.Count(line, "可运行等待") != 1 {
			t.Fatalf("cause row must carry its word exactly once:\n%s", line)
		}
		if strings.Contains(line, "调度等待") {
			t.Fatalf("cause row must not re-wear the absorbed action word:\n%s", line)
		}
	}
	if !causeRowSeen {
		t.Fatalf("specimen 2 replay must keep its 成因 child (fixture drift?)")
	}
	// 关键行三 (▒ 背景): 同标本1 关系形态.
	if !strings.Contains(fence, "IO等待(对端 udk-irq-1-63)") {
		t.Fatalf("background peer relation form missing:\n%s", fence)
	}
}
