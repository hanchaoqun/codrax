package tool

// DISPLAY-WRAP batch pins (§29.104.18 + §29.104.18.1,
// docs/design/real_trace_campaign_20260705.md; catalog =
// docs/design/display_ux_catalog_20260716.md; witness =
// .codrax/output/20260716-183011.023-26159.md — the donghu 17267 single-board
// single-window report).
//
// 件② single-board chip elision (catalog B1 修根): the phantom UNNAMED board.
// The witness report wore 「窗13762.792~13763.025s·板锚 .ugc.aweme.lite-17267」
// on EVERY seat (39/38 same-value chips) although it holds exactly ONE board.
// Root cause (diagnosed live, sharper than the §29.104.18.1 suspicion): the
// display-level same-segment lane-twin fold transferred the rank twin's
// ORDINAL (node.Rank) onto the surviving chain row WITHOUT its board identity
// notes — three fold hosts then carried Rank>0 with RankBoardTarget=="", and
// the 件E census honestly counted them as an unnamed second board
// (targetsInWindow = {"", target} → boardSplit → chips everywhere). The fix
// is the XLANE-3 件1 aggregate-merge discipline applied to BOTH display-level
// folds (same-segment twins + SEM-LEAD semantic twins): the ordinal never
// travels without its board identity. The chip-wearing gate itself is
// UNCHANGED — on genuine mixed legacy/new boards (a REAL unnamed board) the
// named seats keep their chips (TestXLANE3MixedLegacyNewFormsUnnamedBoard).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// displayWrapStatedWordsProjection — 件③(a) representative shape (also wired
// into the NEW-7 bidirectional legend sweep): the witness E1 geometry — a
// SELF running supply-fold seat with comovement topology, whose stanza lines
// repeat BOTH long caliber phrases (行3 + 拆解子行 + mechanism clause), so
// the 2nd+ occurrences collapse to 按前述基准 / 分簇口径同前 and both
// repeat-suppression marks light.
func displayWrapStatedWordsProjection() types.TraceCausalProjection {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E-dwstated",
		Subject: "app-42", Object: "running", TypeToken: "running",
		StateKind: "running", ChainRelevance: "on_chain", OnChainBasis: "self_wall_clock_interval",
		Predicate: "root_cause_primary", Rank: 1, Tier: "primary",
		ImpactMS: 157.248, CumulativeImpactMS: 157.248, EffectiveImpactMS: 58.320,
		SupplyFoldComputed: true, SupplyFoldIdealMS: 98.928, SupplyFoldDeficitMS: 58.320,
		SupplyFoldKnownMS:          157.248,
		SupplyFoldCapabilitySource: runtimeTraceCapabilitySourceDefault,
		SupplyFoldTopologySource:   runtimeTraceCapabilityTopologyComovement,
		Confidence:                 0.86, LineStart: 10, LineEnd: 20,
	}
	// A plain bar-wearing chain hop keeps the fixture bidirectional on the
	// bar-scale probe (the self stanza alone renders the header's 满格=窗口
	// claim without any bar row).
	hop := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E-dwhop",
		Subject: "waker-1", Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
		Rank: 2, Tier: "secondary", Confidence: 0.8, LineStart: 30, LineEnd: 40,
	}
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app-42"},
		WindowStartTs: 100.0, WindowEndTs: 100.24,
		OnChainCauses: []types.TraceCausalProjectionNode{node, hop},
	}
}

// TestDisplayWrapSameNodeCaliberPhraseDedup — 件③(a) pins on the witness E1
// shape: the node's FIRST occurrence keeps each full phrase, later same-node
// occurrences collapse to the legend-taught reference words, both marks
// light, values stay byte-identical.
func TestDisplayWrapSameNodeCaliberPhraseDedup(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(displayWrapStatedWordsProjection(),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	// 首现全拼 ≤1 次 per phrase across the whole node stanza.
	if got := strings.Count(fence, "按全域最大核最高频"); got != 1 {
		t.Fatalf("按全域最大核最高频 must spell exactly once per node, got %d:\n%s", got, fence)
	}
	if got := strings.Count(fence, "按实测频点共动分簇折算"); got != 1 {
		t.Fatalf("按实测频点共动分簇折算 must spell exactly once per node, got %d:\n%s", got, fence)
	}
	if !strings.Contains(fence, "按前述基准") || !strings.Contains(fence, "分簇口径同前") {
		t.Fatalf("the repeat-suppression reference words must ride the 2nd+ occurrences:\n%s", fence)
	}
	// Values untouched.
	for _, v := range []string{"58.320ms", "157.248ms"} {
		if !strings.Contains(fence, v) {
			t.Fatalf("value %s must survive the dedup byte-identically:\n%s", v, fence)
		}
	}
	if !model.Marks.has(runtimeTraceProjMarkCaliberStatedBasis) ||
		!model.Marks.has(runtimeTraceProjMarkCaliberStatedClustering) {
		t.Fatalf("both repeat-suppression marks must record at the emission site")
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	if !strings.Contains(legend, "- `按前述基准` =") || !strings.Contains(legend, "- `分簇口径同前` =") {
		t.Fatalf("the reference words' legend entries must render with the words:\n%s", legend)
	}
	// Negative arm: a node WITHOUT repetition keeps every full phrase and no
	// reference word (the CAP2 comovement shape spells each phrase once).
	single := buildRuntimeTraceProjTreeModel(revisit76CAP2TopologyProjection(runtimeTraceCapabilityTopologyComovement, 0),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	singleFence := rspaFenceJoined(runtimeTraceProjTreeFence(single, true))
	if strings.Contains(singleFence, "按前述基准") || strings.Contains(singleFence, "分簇口径同前") {
		t.Fatalf("a repetition-free node must keep its full phrases:\n%s", singleFence)
	}
}

// --- 件① token-aware wrap (§29.104.18 形1..形4 + catalog A1/C1) ---------------

// TestDisplayWrapCJKWordRunNeverBisects — 件①(a) / catalog A1: the four
// witness word splits (「为主」「证据」「对象」「该簇」) and the paren pair
// (「(父节点未确认)」) survive whole at every width that can hold them —
// maximal CJK word runs are single atoms, and no line ends "(" / opens ")".
func TestDisplayWrapCJKWordRunNeverBisects(t *testing.T) {
	cases := []struct {
		text  string
		whole []string
	}{
		// witness L138-139 (「…折算)为\n主,」): the supply-fold clause tail.
		{"供给折算缺口 58.320ms(运行频点非最高)为主,running 时间含降频/小核导致的跑慢成分",
			[]string{"为主", "跑慢成分"}},
		// witness L148-149 (「;证\n据 E33」): the same-segment IO pointer tail
		// (证据 whole at every width; the word+ref pair pins below).
		{"同段IO另有 块设备层 2.694(综合评分,非墙钟) 等口径;证据 [E33]",
			[]string{"证据", "等口径"}},
		// witness L203-204 (「等待对\n象 dma_…」).
		{"D状态候选·置信中·等待对象 dma_fence_default_w", []string{"等待对象"}},
		// witness L247-248 (「窗内该\n簇受热限压至 2.34GHz」): the word run —
		// the 件①(d) word+value pair pins below at pair-holding widths.
		{"接近全域最大核最高频,缺口仅 0.039ms;窗内该簇受热限压至 2.34GHz",
			[]string{"该簇"}},
		// witness L277-278 (「链上L1(父节点未\n确认)」).
		{"优先级反转候选·置信高·链上L1(父节点未确认)", []string{"(父节点未确认)"}},
	}
	for _, tc := range cases {
		for width := 14; width <= 60; width++ {
			chunks := runtimeTraceProjWrapDisplay(tc.text, width)
			if strings.Join(chunks, "") != tc.text {
				t.Fatalf("width %d: wrap must stay byte-lossless: %q", width, chunks)
			}
			for _, word := range tc.whole {
				found := false
				for _, chunk := range chunks {
					if strings.Contains(chunk, word) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("width %d: %q bisected across chunks: %q", width, word, chunks)
				}
			}
		}
	}
	// 件①(d) word+value pairs stay whole at every width that can HOLD the
	// pair (narrower widths legitimately break at the word boundary instead —
	// the width-capped fusion keeps the rune-level hard split unreachable).
	pairs := []struct {
		text, pair string
	}{
		{"接近全域最大核最高频,缺口仅 0.039ms;窗内该簇受热限压至 2.34GHz", "受热限压至 2.34GHz"},
		// B12 (DISPLAY-HYG 二轮): the pointer tail now wears the bracket style —
		// the self-contained [E33] atom stays whole (E#-ref fusion).
		{"同段IO另有 块设备层 2.694(综合评分,非墙钟) 等口径;证据 [E33]", "[E33]"},
	}
	for _, tc := range pairs {
		for width := 30; width <= 90; width++ {
			for _, chunk := range runtimeTraceProjWrapDisplay(tc.text, width) {
				if strings.Contains(chunk, tc.pair) {
					goto next
				}
			}
			t.Fatalf("width %d: pair %q bisected", width, tc.pair)
		next:
		}
	}
}

// TestDisplayWrapChipChainSeparatorNeverDangles — 件①(b) / 形1: a `·` chip
// chain breaks ONLY at chip boundaries and the separator never dangles at
// EOL — the continuation opens with `·` (witness L134-135 「…板锚 X·\n置信高」
// ×6).
func TestDisplayWrapChipChainSeparatorNeverDangles(t *testing.T) {
	text := "IO阻塞候选·自身·墙钟席·根因排序#12·窗13762.792~13763.025s·板锚 .ugc.aweme.lite-17267·置信高·有效归因 1.347ms(全额)"
	// The widest chip pair (板锚 + thread label) is 26 cells — narrower
	// widths legitimately break at the pair's inner word boundary.
	for width := 28; width <= 90; width++ {
		chunks := runtimeTraceProjWrapDisplay(text, width)
		if strings.Join(chunks, "") != text {
			t.Fatalf("width %d: wrap must stay byte-lossless: %q", width, chunks)
		}
		for i, chunk := range chunks {
			if strings.HasSuffix(strings.TrimRight(chunk, " "), "·") {
				t.Fatalf("width %d: chip separator dangles at EOL (chunk %d): %q", width, i, chunks)
			}
		}
		for _, chip := range []string{"根因排序#12", "板锚 .ugc.aweme.lite-17267", "置信高", "有效归因 1.347ms(全额)"} {
			found := false
			for _, chunk := range chunks {
				if strings.Contains(chunk, chip) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("width %d: chip %q bisected: %q", width, chip, chunks)
			}
		}
	}
}

// TestDisplayWrapEvidenceListAndRefsStayWhole — 件①(d) / 形3: the 顿号 E#
// list and the bracketed [E#+E#] reference are single wrap atoms (witness
// L162-163 「E34、E35(+1)、\nE36(+1)」孤引).
func TestDisplayWrapEvidenceListAndRefsStayWhole(t *testing.T) {
	list := "同段IO另有 完成端到端·IO延迟（io_latency） 1.248/1.058/0.884ms 等口径;证据 [E34]、[E35(+1)]、[E36(+1)]"
	// The fused 证据+list claim is 27 cells — sweep the widths that hold it.
	for width := 28; width <= 90; width++ {
		chunks := runtimeTraceProjWrapDisplay(list, width)
		found := false
		for _, chunk := range chunks {
			if strings.Contains(chunk, "[E34]、[E35(+1)]、[E36(+1)]") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("width %d: the 顿号 E# list broke apart: %q", width, chunks)
		}
	}
	ref := "同段被合并的证据引用 [E13(+1)+E52] 数值不重复计入"
	for width := 16; width <= 60; width++ {
		chunks := runtimeTraceProjWrapDisplay(ref, width)
		found := false
		for _, chunk := range chunks {
			if strings.Contains(chunk, "[E13(+1)+E52]") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("width %d: the bracketed reference bisected: %q", width, chunks)
		}
	}
}

// TestDisplayWrapSubordinateLinesNeverEndWithSpace — 件①(f) / catalog C1: the
// emitted physical lines carry no trailing spaces (the witness THERM clause
// left the break space at EOL — L242 「…受热限压至 ␠\n1.88GHz」 — and the
// word+value pair itself never splits now).
func TestDisplayWrapSubordinateLinesNeverEndWithSpace(t *testing.T) {
	text := "供给折算缺口 0.933ms(运行频点非最高,已计入有效归因,按实测频点共动分簇折算);窗内该簇受热限压至 1.88GHz"
	lines := runtimeTraceProjSubordinateLines("│ ", text)
	if len(lines) < 2 {
		t.Fatalf("fixture drift: the THERM clause must wrap at the row cap: %q", lines)
	}
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("emitted line ends with a trailing space (C1): %q", line)
		}
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "受热限压至 1.88GHz") {
		t.Fatalf("the word+value pair must land whole on one line: %q", lines)
	}
}

// TestDisplayWrapIdentityGroupLines — 件①(c) / §29.104.18 修向② (用户所请
// 分行显示形): an over-wide 行2 breaks BETWEEN its semantic groups — group
// lines open with the `·` separator, the confidence tail moves down whole
// (形2 短尾防孤), and a fitting row keeps the single-line form byte-identically.
func TestDisplayWrapIdentityGroupLines(t *testing.T) {
	groups := []string{
		"算力供给候选·自身·墙钟席·根因排序#1",
		"窗13762.792~13763.025s·板锚 .ugc.aweme.lite-17267.with.a.long.board.target.label-99999",
		"置信高",
	}
	indent := strings.Repeat(" ", 8)
	lines := runtimeTraceProjIdentityGroupLines(indent, groups, true)
	if len(lines) < 2 {
		t.Fatalf("the over-wide identity row must split: %q", lines)
	}
	if !strings.HasPrefix(lines[0], indent+"· "+groups[0]) {
		t.Fatalf("line 1 must open the first group under the note marker: %q", lines)
	}
	for _, line := range lines[1:] {
		body := strings.TrimLeft(line, " ")
		if !strings.HasPrefix(body, "·") {
			t.Fatalf("a group continuation must open with the separator: %q", lines)
		}
	}
	for i, line := range lines {
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("group line %d over the row cap (%d cells): %q", i, w, line)
		}
		if strings.HasSuffix(strings.TrimRight(line, " "), "·") {
			t.Fatalf("a group line must never dangle its separator: %q", lines)
		}
	}
	// The confidence tail rides a group line, never a blind-fold orphan of a
	// wider line (形2): it must sit WITH its group opener `·`.
	last := strings.TrimLeft(lines[len(lines)-1], " ")
	if last == "置信高" {
		t.Fatalf("the confidence tail must keep its group separator (no bare orphan): %q", lines)
	}
	// Fitting rows keep the single-line form byte-identically.
	short := []string{"调度压力候选·根因排序#2", "窗10.000~10.200s·板锚 t-1", "置信中"}
	one := runtimeTraceProjIdentityGroupLines(indent, short, true)
	if len(one) != 1 || one[0] != indent+"· "+strings.Join(short, "·") {
		t.Fatalf("a fitting identity row must stay one line: %q", one)
	}
}

// TestDisplayWrapRowTwoGroupSplitEndToEnd — 件①(c) end-to-end on a genuine
// two-board projection (XLANE-3 fixtures, long board targets): the rendered
// 行2 carries the board-identity group on its own `·`-opened continuation
// line, no line dangles a separator, no line exceeds the cap and no line
// carries a trailing space.
func TestDisplayWrapRowTwoGroupSplitEndToEnd(t *testing.T) {
	a := xlane3BoardSeat("dw-a", "workerX-300", "runnable", "runnable_wait",
		"com.example.userfacing.application.main-12345", "cccc3333", 1, 5.0, 10)
	b := xlane3BoardSeat("dw-b", "workerY-400", "runnable", "runnable_wait",
		"com.example.background.provider.worker-67890", "dddd4444", 1, 3.0, 30)
	model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	sawGroupLine := false
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("no fence line may end with a trailing space: %q", line)
		}
		if strings.HasSuffix(line, "·") {
			t.Fatalf("no fence line may dangle the chip separator: %q", line)
		}
		if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("fence line over the row cap (%d cells): %q", w, line)
		}
		body := strings.TrimLeft(line, "│ ")
		// Greedy whole-group packing: g1+g2 (identity + board chip) hold line
		// 1; the confidence/attribution tail group opens its own line WITH
		// the separator (never a bare 置信中 orphan — 形2).
		if strings.HasPrefix(body, "·置信中·链上L1·有效归因") {
			sawGroupLine = true
		}
	}
	if !sawGroupLine {
		t.Fatalf("the board-identity group must open its own continuation line:\n%s", fence)
	}
	// The chips themselves stay whole (multi-board semantics untouched).
	joined := rspaFenceJoined(fence)
	if !strings.Contains(joined, "板锚 com.example.userfacing.application.main-12345") ||
		!strings.Contains(joined, "板锚 com.example.background.provider.worker-67890") {
		t.Fatalf("both boards' anchors must survive the group split:\n%s", fence)
	}
}

// TestDisplayWrapSingleBoardWitnessWearsNoChips — 件② witness pin on the
// production donghu recipe (single step, target 17267 = the witness report's
// exact window/target): the single-board single-window report renders ZERO
// board/window chips, and the former chip wearers keep the bare legacy seat
// face. Pre-fix this render carried 板锚 ×40 / 窗13762.792~13763.025s ×38.
func TestDisplayWrapSingleBoardWitnessWearsNoChips(t *testing.T) {
	obs := xlane3DonghuTwoStepObservations(t, 17267)
	md, set := xlane3RenderTwoStep(t, obs)
	for _, token := range []string{"板锚", "·窗13762.792", "参数#"} {
		if strings.Contains(md, token) {
			t.Fatalf("single-board witness report must carry no board token %q:\n%s", token, md)
		}
	}
	// The seats stay seated — elision removed the chip, never the ordinal.
	for _, face := range []string{
		"算力供给候选·自身·墙钟席·根因排序#1·置信高",
		"优先级反转候选·供给缺口主导·根因排序#2·置信高·链上L3",
	} {
		if !strings.Contains(md, face) {
			t.Fatalf("the bare legacy seat face %q must survive chip elision:\n%s", face, md)
		}
	}
	// The board identity census sees exactly ONE named board: every ranked
	// population node (including the three former phantom fold hosts) carries
	// the step's target.
	if len(set.Projections) != 1 {
		t.Fatalf("single-step ledger must stay one projection, got %d", len(set.Projections))
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	foldHosts := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent} {
		for i := range rows {
			if !rows[i].HasData || rows[i].Node.Rank <= 0 {
				continue
			}
			if strings.TrimSpace(rows[i].Node.RankBoardTarget) == "" {
				t.Fatalf("ranked row %q (rank=%d) lost its board identity — phantom unnamed board re-minted",
					rows[i].Node.Subject, rows[i].Node.Rank)
			}
			if len(rows[i].RankFoldPeers) > 0 {
				foldHosts++
			}
		}
	}
	if foldHosts == 0 {
		t.Fatalf("fixture drift: the witness recipe must carry rank-fold host rows (the phantom board source)")
	}
	// 单点声明 verdict half: the board identity is declared exactly once by
	// the tree head (⊚ target + 满格=窗口) — no extra declaration token is
	// minted (the XLANE-3 single-board pin forbids any 板锚/窗 token here).
	if !strings.Contains(md, "⊚ .ugc.aweme.lite-17267") {
		t.Fatalf("the ⊚ tree head must declare the board target once:\n%s", md)
	}
}

// TestDisplayWrapWitnessRegressionSweep — 件①(f)/件③/件④ end-to-end pins on
// the production witness recipe (donghu 17267 single step, the
// 20260716-183011 report's window/target):
//
//	件①(f) C1: zero trailing spaces, zero dangling `·` at EOL;
//	件③(a) B2: each long caliber phrase spells at most once per node
//	  (repeat occurrences wear 按前述基准/分簇口径同前);
//	件③(b) B3: the three rule-sentence bodies live ONLY in the legend —
//	  rows wear the legend-keyed chip words;
//	件④(a) A3: the authority-proven plain self-runnable family keeps its
//	  measured effective value after the retired nearest-based inversion seat
//	  disappears;
//	件④(b) A4: the surviving same-head self rows are distinguishable
//	  (调度延迟 / runnable), and an unproved self inversion is absent;
//	件④(c) A6: count-equivalent values carry their caliber at the point of
//	  reading (◎ footnote + detail 单段 range), never an ms suit.
func TestDisplayWrapWitnessRegressionSweep(t *testing.T) {
	obs := xlane3DonghuTwoStepObservations(t, 17267)
	md, _ := xlane3RenderTwoStep(t, obs)
	for _, line := range strings.Split(md, "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("C1: no rendered line may end with a trailing space: %q", line)
		}
		if strings.HasSuffix(line, "·") {
			t.Fatalf("形1: no rendered line may dangle the chip separator: %q", line)
		}
	}
	// 件③(b): rule bodies legend-only. The row-face byte forms are dead; the
	// legend's own wording (该行整席账目未能出示…) teaches the rule.
	for _, dead := range []string{
		"该席账目未能出示",  // R4 row sentence body (witness ×5)
		"凭证锚定段合计",   // bipartition rule clause (witness ×4)
		"两套账目覆盖集不同", // account-relation rule half (witness ×4)
	} {
		if strings.Contains(md, dead) {
			t.Fatalf("件③(b): rule body %q must live only in the legend:\n%s", dead, md)
		}
	}
	for _, chip := range []string{"无链上凭证(整席降道,见图例)", "账目关系(见图例):本行=", "同源二分对席:"} {
		if !strings.Contains(md, chip) {
			t.Fatalf("件③(b): the legend-keyed chip word %q must ride the rows:\n%s", chip, md)
		}
	}
	// 件③(a): the two long phrases spell once per node — the witness carried
	// 30/28 spellings, the node-deduped render carries exactly one per
	// wearing node (18/17 on this recipe) plus the reference words.
	if !strings.Contains(md, "按前述基准") || !strings.Contains(md, "分簇口径同前") {
		t.Fatalf("件③(a): the repeat-suppression words must ride the repeats:\n%s", md)
	}
	// TQ-PRIORITY-POINT-AUTHORITY evolution: the former 2.116ms self
	// inversion seat was minted from a non-point priority fallback. The strict
	// authority publishes no such row; the two independently measured
	// runnable faces remain distinct and lossless.
	if strings.Contains(md, "⧖ 自身·优先级反转·可运行等待") {
		t.Fatalf("件④(b): unproved self inversion must not survive the point-authority gate:\n%s", md)
	}
	for _, head := range []string{"⧖ 自身·调度延迟", "⧖ 自身·runnable"} {
		if !strings.Contains(md, head) {
			t.Fatalf("件④(b): self head %q must carry its type word (与表同名):\n%s", head, md)
		}
	}
	// 件④(a): removing the unproved subtype must not remove or discount the
	// target's independently measured full runnable account.
	if !strings.Contains(md, "⧖ 自身·runnable 合计5.604ms") || !strings.Contains(md, "· 有效归因 5.604ms =") {
		t.Fatalf("件④(a): the measured self-runnable seat/value must remain after fail-closed retyping:\n%s", md)
	}
	// 复核 P2-件3 negative arm: the lane's !ConsumedEffective gate is
	// load-bearing — with it deleted, the ⌗ count row (E46-shape, whose 行3
	// already consumed the effective in the suffix-free count-equivalent
	// form) would re-mint the bare Q1 tag with a FALSE wall-clock ms suit.
	if strings.Contains(md, "有效归因 81.616ms") {
		t.Fatalf("件④(a) 负臂: the ⌗ count row must never re-mint a bare ms-suited effective:\n%s", md)
	}
	// …and the count row's effective face renders EXACTLY once (the 行3
	// count-equivalent equation) — a doubled lane is the same mutation face.
	if got := strings.Count(md, "有效归因 计数当量81.616(非墙钟)"); got != 1 {
		t.Fatalf("件④(a) 负臂: the ⌗ count row's effective must render exactly once, got %d:\n%s", got, md)
	}
	// 件④(c): count-equivalent calibers at the point of reading.
	// CALSIDE-1 件1 (2026-07-19): the footnote seat wears its seat-row class
	// word (页缓存抖动) between subject and value — same single word source as
	// the ◎ board lines.
	if !strings.Contains(md, "不参与汇排(口径):.ugc.aweme.lite-17267 · 页缓存抖动 · 计数当量81.616(非墙钟)·⌗口径旁栏") {
		t.Fatalf("件④(c): the ◎ footnote value must carry the count-equivalent caliber:\n%s", md)
	}
	if !strings.Contains(md, "单段 计数当量34.800~84.300(非墙钟)") {
		t.Fatalf("件④(c): the count family's detail 单段 range must drop the ms suit:\n%s", md)
	}
	// …and the wall-clock families keep their ms ranges byte-identically.
	if !strings.Contains(md, "单段 0.865~1.248ms") {
		t.Fatalf("件④(c) negative arm: wall-clock 单段 ranges keep the ms suit:\n%s", md)
	}
}

// displayWrapGroupSplitSeat builds one board seat whose 行2 identity is
// over-wide by construction (60-char board target): g1 alone fits, g1+窗chip
// would fit too (the generic-wrap bypass discriminator), but g1+板身份组
// overflows — so the GROUP renderer must break exactly at the group boundary.
func displayWrapGroupSplitSeat(evidence, subject, target string, selfBasis bool) types.TraceCausalProjectionNode {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: evidence,
		Subject: subject, Object: "runnable_wait", TypeToken: "runnable_wait",
		StateKind: "runnable", ChainRelevance: "on_chain", ChainDepth: 1,
		ImpactMS: 5.0, CumulativeImpactMS: 5.0, EffectiveImpactMS: 5.0,
		QueryWindowStartTs: 10.0, QueryWindowEndTs: 10.2,
		RankBoardTarget: target, RankBoardParamsFingerprint: "aaaa1111",
		Rank: 1, Tier: "primary", Confidence: 0.72,
		LineStart: 10, LineEnd: 20,
	}
	if selfBasis {
		node.OnChainBasis = "self_wall_clock_interval"
		// SYM-2 typed flag (engine mints it on self-runnable seats only): the
		// RT suffix rides the LAST identity group — the 复核-named 禁形
		// (「…\n(优先级低于RT)」 opening a line) is the bypass shape.
		node.RunnableBelowRTPreempted = true
	}
	return node
}

// displayWrapAssertGroupBoundaryLines — 复核 P2-件1 shared assertions: the
// board-identity group (窗…·板锚 <target>) breaks as ONE unit on its own
// `·`-opened line (never packed after g1 the way the generic chip-boundary
// wrap would), the anchor half never dangles without its target, and the
// RT suffix never opens a line.
func displayWrapAssertGroupBoundaryLines(t *testing.T, fence, target string, wantRT bool) {
	t.Helper()
	groupLine, rtWithTier := false, false
	for _, line := range strings.Split(fence, "\n") {
		body := strings.TrimLeft(line, "│ ")
		// The over-wide seat's board-identity group opens its OWN
		// `·`-prefixed line, whole (窗 half + anchor + full target) — the
		// generic bypass would instead pack 「…根因排序#1·窗10.000~10.200s·板锚」
		// onto the g1 line and orphan the target below.
		if strings.HasPrefix(body, "·窗10.000~10.200s·板锚 "+target) {
			groupLine = true
			if strings.Contains(body, "根因排序") {
				t.Fatalf("件1: the group line must be the board-identity group ALONE, not a mid-chain wrap:\n%s", fence)
			}
		}
		// Bypass shape ①: a dangling anchor word (the generic wrap breaks at
		// the 板锚/target inner space once the pair exceeds the fusion cap).
		if strings.HasSuffix(strings.TrimRight(body, " "), "板锚") {
			t.Fatalf("件1: the anchor half must never dangle without its target:\n%s", fence)
		}
		// Bypass shape ②: an orphaned target (its line must carry the anchor
		// word — the pair is one board-identity claim).
		if strings.Contains(body, target) && !strings.Contains(body, "板锚 "+target) {
			t.Fatalf("件1: the board target must never split from its anchor word:\n%s", fence)
		}
		// Bypass shape ③ (复核-named 禁形): the RT suffix opening a line.
		if strings.HasPrefix(body, "(优先级低于RT)") {
			t.Fatalf("件1 禁形: the RT suffix must never open a line:\n%s", fence)
		}
		if strings.Contains(body, "置信中") && strings.Contains(body, "(优先级低于RT)") {
			rtWithTier = true
		}
	}
	if !groupLine {
		t.Fatalf("件1: the `·`-opened whole board-identity group line must render:\n%s", fence)
	}
	if wantRT && !rtWithTier {
		t.Fatalf("件1: the RT suffix must ride the confidence group's line:\n%s", fence)
	}
}

// TestDisplayWrapGroupSplitChainOwnLineSite — 复核 P2-件1 (chain OwnLine
// emission site): the flat chain row's over-wide 行2 renders through the
// group renderer, pinned by group-boundary semantics a generic-wrap bypass
// cannot coincidentally reproduce.
func TestDisplayWrapGroupSplitChainOwnLineSite(t *testing.T) {
	const target = "com.example.board.target.with.a.deliberately.long.label-1234"
	a := displayWrapGroupSplitSeat("dw-site-a", "workerX-300", target, false)
	b := displayWrapGroupSplitSeat("dw-site-b", "workerY-400", "other.board.target-2", false)
	b.Object, b.TypeToken = "scheduler_latency", "scheduler_latency"
	model := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	displayWrapAssertGroupBoundaryLines(t, runtimeTraceProjTreeFence(model, true), target, false)
}

// TestDisplayWrapGroupSplitSelfStanzaSite — 复核 P2-件1 (self stanza emission
// site): the SELF wall-clock seat's over-wide 行2 (自身·墙钟席 qualifier + RT
// suffix) renders through the same group renderer at the stanza wiring.
func TestDisplayWrapGroupSplitSelfStanzaSite(t *testing.T) {
	const target = "com.example.board.target.with.a.deliberately.long.label-1234"
	self := displayWrapGroupSplitSeat("dw-site-self", "selftarget-42", target, true)
	other := displayWrapGroupSplitSeat("dw-site-other", "workerY-400", "other.board.target-2", false)
	other.Object, other.TypeToken = "scheduler_latency", "scheduler_latency"
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "selftarget-42"},
		WindowStartTs: 10.0, WindowEndTs: 10.2,
		OnChainCauses: []types.TraceCausalProjectionNode{self, other},
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "自身·墙钟席") {
		t.Fatalf("fixture drift: the self seat must render in the stanza with its qualifier:\n%s", fence)
	}
	displayWrapAssertGroupBoundaryLines(t, fence, target, true)
}

// TestDisplayWrapENFenceNeverDanglesSeparator — 复核 P2-件2: the EN face's
// dangling-`·` guard (the space-pop carry inside breakLine is the EN
// load-bearing mechanism — its separators are spaced " · "). Scans BOTH the
// production witness recipe's EN render and the synthetic over-wide
// two-board EN fence for any line ending "·" (pre- or post-trim shape).
func TestDisplayWrapENFenceNeverDanglesSeparator(t *testing.T) {
	scan := func(name, fence string) {
		t.Helper()
		for _, line := range strings.Split(fence, "\n") {
			if strings.HasSuffix(strings.TrimRight(line, " "), "·") {
				t.Fatalf("%s: an EN line dangles the separator at EOL: %q", name, line)
			}
		}
	}
	// Production recipe, EN faces (tree + ◎ overview).
	obs := xlane3DonghuTwoStepObservations(t, 17267)
	_, set := xlane3RenderTwoStep(t, obs)
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	scan("donghu-en-tree", runtimeTraceProjTreeFence(model, false))
	scan("donghu-en-elim", runtimeTraceProjElimOverviewFence(set.Projections[0], model, false))
	// Synthetic over-wide EN identity rows (guaranteed " · " wrap pressure).
	const target = "com.example.board.target.with.a.deliberately.long.label-1234"
	a := displayWrapGroupSplitSeat("dw-en-a", "workerX-300", target, false)
	b := displayWrapGroupSplitSeat("dw-en-b", "workerY-400", "other.board.target-2", false)
	b.Object, b.TypeToken = "scheduler_latency", "scheduler_latency"
	enModel := buildRuntimeTraceProjTreeModel(xlane3GateProjection(a, b), newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	scan("synthetic-en-tree", runtimeTraceProjTreeFence(enModel, false))
}

// TestDisplayWrapSelfHeadRawTokenFallback — 件④(b) negative arm: the runnable
// state-label override survives for its original purpose — a self row whose
// display name IS the raw causal token (unmapped) still reads 自身·runnable.
func TestDisplayWrapSelfHeadRawTokenFallback(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			EvidenceID: "E-raw",
			Subject:    "app-42", Object: "runnable_witness_token", TypeToken: "runnable_witness_token",
			StateKind: "runnable", ChainRelevance: "on_chain",
			ImpactMS: 1.0, CumulativeImpactMS: 1.0,
			LineStart: 10, LineEnd: 20,
		},
		Kind: runtimeTraceProjTreeRowSelf, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	main, _, _, _ := runtimeTraceProjSelfRowParts(row, 100.0, true)
	// 复核 P2-件4: EXACT head form — a Contains probe stayed green even with
	// the override deleted (the raw token contains no state word, but the
	// probe matched the state tag elsewhere); the equality pin reds any
	// wording drift on this arm.
	if len(main) == 0 || main[0] != "⧖ 自身·runnable" {
		t.Fatalf("an unmapped raw token must keep the exact state-label head ⧖ 自身·runnable: %q", main)
	}
}

// TestDisplayWrapSameSegmentFoldCarriesBoardIdentity — 件② unit pin on the
// same-segment lane-twin fold: the adopted ordinal carries its board identity
// (empty-slot caller); a host that already owns board identity keeps it.
func TestDisplayWrapSameSegmentFoldCarriesBoardIdentity(t *testing.T) {
	rank := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "u-rank",
		Subject: "sysr-8", Predicate: "root_cause_primary", Object: "priority_inversion_candidate",
		Rank: 2, Tier: "primary", ChainRelevance: "on_chain", PriorityInversionCandidate: true,
		RankBoardTarget: "target.a-100", RankBoardParamsFingerprint: "aaaa1111",
		ImpactMS: 0.813, EffectiveImpactMS: 0.813, LineStart: 1000, LineEnd: 2000, Confidence: 0.91,
	}
	chain := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "u-chain",
		Subject: "sysr-8", Predicate: "wakeup_causal_impact", Object: "running",
		StateKind: "running", ChainRelevance: "on_chain", PriorityInversionCandidate: true,
		ImpactMS: 2.770, EffectiveImpactMS: 0.813, LineStart: 1000, LineEnd: 2000, Confidence: 0.78,
	}
	kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || len(peers) != 1 {
		t.Fatalf("control pair must fold: kept=%d peers=%d", len(kept), len(peers))
	}
	if kept[0].Rank != 2 || kept[0].RankBoardTarget != "target.a-100" ||
		kept[0].RankBoardParamsFingerprint != "aaaa1111" {
		t.Fatalf("the adopted ordinal must carry its board identity: %+v", kept[0])
	}
	// Seated survivor keeps its OWN board fields untouched (priority-scan
	// doctrine — same as the XLANE-3 件1 aggregate arm).
	seated := chain
	seated.Rank = 5
	seated.RankBoardTarget = "target.b-200"
	seated.RankBoardParamsFingerprint = "bbbb2222"
	kept, _ = runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, seated})
	if len(kept) != 1 || kept[0].Rank != 5 || kept[0].RankBoardTarget != "target.b-200" ||
		kept[0].RankBoardParamsFingerprint != "bbbb2222" {
		t.Fatalf("a seated survivor must keep its own board identity: %+v", kept)
	}
}

// TestDisplayWrapSemanticFoldCarriesBoardIdentity — 件② unit pin on the
// SEM-LEAD semantic twin fold: the on-chain arm's adopted ordinal carries the
// board identity the same way.
func TestDisplayWrapSemanticFoldCarriesBoardIdentity(t *testing.T) {
	rank := dispW1RankNode()
	rank.RankBoardTarget = "target.a-100"
	rank.RankBoardParamsFingerprint = "aaaa1111"
	outRank, outSem, peers := runtimeTraceProjFoldSemanticRankLaneTwins(
		[]types.TraceCausalProjectionNode{rank},
		[]types.TraceCausalProjectionNode{dispW1SemNode()})
	if len(outRank) != 0 || len(peers) != 1 {
		t.Fatalf("control twin must fold: rank=%+v", outRank)
	}
	if outSem[0].Rank != 1 || outSem[0].RankBoardTarget != "target.a-100" ||
		outSem[0].RankBoardParamsFingerprint != "aaaa1111" {
		t.Fatalf("the surviving ✦ seat must adopt ordinal + board identity together: %+v", outSem[0])
	}
}
