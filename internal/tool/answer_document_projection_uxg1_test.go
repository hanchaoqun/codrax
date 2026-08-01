package tool

// answer_document_projection_uxg1_test.go — UXG-1 M1/M3/T3a cross-package
// pins (ledger docs/design/real_trace_campaign_20260705.md §29.38 收敛机制;
// matrix spec info_contract_report.md §④). The five display-token tables now
// live in internal/tracefence; these pins push ENGINE-MINTED samples through
// the real preview renderer so a one-sided edit (either side abandoning the
// single source) is caught as a render regression, not a silent unstyle:
//
//   - M1-①/M3: every in-production state-mark glyph (the §24.3 form-table
//     glyph column + root/off-chain/undrillable/self-sleep constants) is a
//     member of the tracefence directory (keyset inclusion census — the
//     preview icon keyset derives from the same directory, so preview ⊇ tool
//     holds by construction), and each directory mark reaches the HTML face
//     as its envelope-slot class.
//   - M1-②: the engine mention-floor word (action-token head) is emphasized
//     at its grid width on the HTML face.
//   - M1-③: engine seat chips (根因排序#N / 邻近影响#N, zh+en) and the ➊
//     badge reach the HTML face as chip/ordinal spans (channel word single
//     source; extends the UXG-0 D2 pins).
//   - M1-④: the engine-built chapter blocks' H2 titles are recognized by the
//     preview section transformer (closed heading set single source).
//   - M1-⑤: the engine-built 树读法:/各列口径: blocks relocate into the
//     阅读参考 appendix (aux-marker single source).
//   - T3a: every tracefence state-mark glyph has a legend-catalog entry that
//     names it (图例 mark 目录哨兵扩展 — the fence-side marks sentinel
//     TestTraceProjectionLegendCatalogCoversEveryMark stays; this arm covers
//     the HTML-face directory, including the UXG-0/P0 additions ⊘ and ⧗).
//
// MUTATION self-checks (recorded in the batch report): removing a directory
// row from tracefence.StateMarks → census red; rewording a channel word on
// one side only is now impossible (one constant), rewording the CONSTANT
// without preview surgery keeps both faces in lockstep (asserted here).

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// uxg1InProductionGlyphs returns the glyph set the fence generator can emit
// at a state-mark slot, derived from the tool-side authorities themselves.
func uxg1InProductionGlyphs() []string {
	seen := map[string]bool{}
	var out []string
	add := func(glyph string) {
		if glyph != "" && !seen[glyph] {
			seen[glyph] = true
			out = append(out, glyph)
		}
	}
	add(runtimeTraceProjRootGlyph)
	add(runtimeTraceProjOffChainDStateGlyph)
	add(tracefence.GlyphUndrillable) // ⊘链止 keep-mark + flat banner head
	add(tracefence.GlyphSleep)       // self sleep-row icon
	// P2a rider 件2b (2026-07-13): the ↳ subordinate-component connector rides
	// a row-head slot on reseated self component rows.
	add(tracefence.GlyphSubordinate)
	for _, spec := range runtimeTraceProjImpactFormSpecs() {
		add(spec.Glyph)
	}
	return out
}

// TestUXG1StateMarkKeysetInclusion — M3: tool in-production glyphs ⊆
// tracefence directory (the preview icon classifier consumes the whole
// directory, so the preview keyset is a superset of the tool set whenever
// this census is green).
func TestUXG1StateMarkKeysetInclusion(t *testing.T) {
	for _, glyph := range uxg1InProductionGlyphs() {
		if tracefence.StateMarkClass([]rune(glyph)[0]) == "" {
			t.Errorf("in-production glyph %q is not in the tracefence state-mark directory — the preview face would leave it unboxed (M3 keyset inclusion)", glyph)
		}
	}
}

// TestUXG1StateMarksReachHTMLEnvelopeSlots — M1-①: each directory mark,
// placed at a generator-shaped row-head slot inside a typed-token fence,
// wears its envelope-slot class on the HTML face.
func TestUXG1StateMarksReachHTMLEnvelopeSlots(t *testing.T) {
	var lines []string
	lines = append(lines, "⊚ app-1 ‹用户关注线程› 满格=窗口16.667ms")
	for _, mark := range tracefence.StateMarks() {
		if mark.Glyph == tracefence.RootGlyph {
			continue // the header line above carries the root slot
		}
		lines = append(lines, "│ "+mark.Glyph+" row-"+mark.Class+" 1.000ms")
	}
	fence := tracefence.Opener + "\n" + strings.Join(lines, "\n") + "\n```"
	html, err := preview.RenderMarkdownHTML([]byte(fence + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mark := range tracefence.StateMarks() {
		want := `trace-icon trace-icon-` + mark.Class + `"><span class="trace-ink">` + mark.Glyph + ` </span>`
		if !strings.Contains(html, want) {
			t.Errorf("directory mark %q (class %s) missing its HTML envelope slot:\n%s", mark.Glyph, mark.Class, html)
		}
	}
}

// TestUXG1ActionWordEmphasizedAtGridWidth — M1-②: the engine mention-floor
// word's action head (zh and en, single tracefence source) is emphasized at
// its original grid width.
func TestUXG1ActionWordEmphasizedAtGridWidth(t *testing.T) {
	for _, tc := range []struct {
		zh    bool
		token string
		width int
	}{
		{true, tracefence.ActionWordZH, 6},
		{false, tracefence.ActionWordEN, 18},
	} {
		row := runtimeTraceProjTreeRow{MentionFloorOnChain: true, MentionFloorTopN: 5}
		word := runtimeTraceProjMentionFloorWord(row, tc.zh)
		if !strings.HasPrefix(word, tc.token) {
			t.Fatalf("engine mention-floor word %q no longer leads with the action token %q", word, tc.token)
		}
		fence := tracefence.Opener + "\n⊚ app-1 ‹用户关注线程› 满格=窗口16.667ms\n│ ✦ VerifyClass · " + word + "\n```"
		html, err := preview.RenderMarkdownHTML([]byte(fence + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf(`<span class="trace-action-token trace-action-width-%d">%s</span>`, tc.width, tc.token)
		if !strings.Contains(html, want) {
			t.Errorf("action token %q not emphasized at width %d:\n%s", tc.token, tc.width, html)
		}
	}
}

// TestUXG1ActionCellWordsDeriveFromTable — F5 drift pin: the EN action cell
// and the zh action cell of a semantic row lead with the table-② words
// (ActionWordENShort has NO other byte pin — the preview emphasis set
// excludes it by waiver, so this is its one red face).
func TestUXG1ActionCellWordsDeriveFromTable(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleSemanticSpan, SemanticClass: "class_verification",
	}
	if cell, _ := runtimeTraceCausalProjectionActionCellWithFamily(node, false); !strings.HasPrefix(cell, tracefence.ActionWordENShort+"·") {
		t.Errorf("EN semantic action cell %q must lead with the table-② short word %q", cell, tracefence.ActionWordENShort)
	}
	if cell, _ := runtimeTraceCausalProjectionActionCellWithFamily(node, true); !strings.HasPrefix(cell, tracefence.ActionWordZH+"·") {
		t.Errorf("zh semantic action cell %q must lead with the table-② word %q", cell, tracefence.ActionWordZH)
	}
	bare := types.TraceCausalProjectionNode{Role: types.TraceCausalRoleSemanticSpan}
	if cell, _ := runtimeTraceCausalProjectionActionCellWithFamily(bare, false); cell != tracefence.ActionWordEN {
		t.Errorf("EN classless semantic action cell %q must be the table-② word %q", cell, tracefence.ActionWordEN)
	}
}

// TestUXG1SeatChipWordsReachHTMLChips — M1-③: engine chip words for both
// ordinal channels (zh+en) and the seat-1 badge reach the HTML face as
// chip/ordinal spans. The words come from the ONE constructor
// runtimeTraceProjSeatChannelWord whose bytes are tracefence constants — the
// same constants the preview classifier reads.
func TestUXG1SeatChipWordsReachHTMLChips(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for _, tc := range []struct {
			kind string
			row  runtimeTraceProjTreeRow
		}{
			{"chain", runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowCause}},
			{"adjacent", runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowAdjacent}},
		} {
			chip, ok := runtimeTraceProjSeatChipWord(tc.row, 1, zh)
			if !ok {
				t.Fatalf("%s zh=%v: engine chip word unavailable", tc.kind, zh)
			}
			badge := runtimeTraceProjBadgeGlyph(1)
			fence := tracefence.Opener + "\n⊚ app-1 ‹用户关注线程› 满格=窗口16.667ms\n│ " + badge + " ⚙ worker-7 1.000ms\n│ · 算力供给候选·" + chip + "\n```"
			html, err := preview.RenderMarkdownHTML([]byte(fence + "\n"))
			if err != nil {
				t.Fatal(err)
			}
			ordinalClass := `trace-rank-ordinal trace-rank-1`
			if tc.kind == "adjacent" {
				ordinalClass = `trace-rank-ordinal trace-rank-adjacent`
			}
			if !strings.Contains(html, ordinalClass) {
				t.Errorf("%s zh=%v: chip word %q did not reach the HTML ordinal class %q:\n%s", tc.kind, zh, chip, ordinalClass, html)
			}
			// CAL-1 件⑥a (2026-07-12): the badge and its D5 companion space
			// ride the 2ch envelope pill (the 1ch chip form is retired).
			if !strings.Contains(html, `trace-slot trace-rank-pill trace-rank-1`) {
				t.Errorf("zh=%v: badge %q did not reach the HTML envelope pill class:\n%s", zh, badge, html)
			}
		}
	}
}

// TestUXG1GeneratedChapterTitlesRecognizedByPreview — M1-④: the H2 titles
// the engine block builders mint (detail / evidence via the projection
// cluster, optimization via the semantic-optimization materializer) are
// recognized by the preview section transformer.
func TestUXG1GeneratedChapterTitlesRecognizedByPreview(t *testing.T) {
	assertSection := func(title, sectionClass string) {
		t.Helper()
		md := "## " + title + "\n\nintro paragraph.\n\n- one item\n"
		html, err := preview.RenderMarkdownHTML([]byte(md))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html, `<section class="`+sectionClass+`">`) {
			t.Errorf("engine chapter title %q not recognized as %s:\n%s", title, sectionClass, html)
		}
	}
	for _, lang := range []string{"zh", "en"} {
		blocks := runtimeTraceCausalProjectionCluster(uxr1FourOneSixFiveProjection(), lang, runtimeTraceProjUserFocus{})
		var detail, evidence string
		for _, block := range blocks {
			for _, base := range tracefence.SectionDetailTitles() {
				if block.Title == base || strings.HasPrefix(block.Title, base+" — ") {
					detail = block.Title
				}
			}
			for _, base := range tracefence.SectionEvidenceTitles() {
				if block.Title == base || strings.HasPrefix(block.Title, base+" — ") {
					evidence = block.Title
				}
			}
		}
		if detail == "" || evidence == "" {
			t.Fatalf("%s: engine cluster lost its detail/evidence chapter titles (detail=%q evidence=%q)", lang, detail, evidence)
		}
		assertSection(detail, "trace-projection-detail")
		assertSection(evidence, "trace-projection-evidence")
	}
	// Optimization chapter: engine-real via the materializer fixture.
	bus := semanticOptimizationFixtureBus("")
	doc := &types.AnswerDocumentV2{}
	if !materializeRuntimeTraceSemanticOptimizationBlock(doc, bus) {
		t.Fatal("optimization fixture no longer materializes its block")
	}
	var optimization string
	for _, block := range doc.Blocks {
		for _, base := range tracefence.SectionOptimizationTitles() {
			if block.Title == base || strings.HasPrefix(block.Title, base+" — ") {
				optimization = block.Title
			}
		}
	}
	if optimization == "" {
		t.Fatalf("optimization block title drifted off the tracefence closed set: %+v", doc.Blocks)
	}
	assertSection(optimization, "trace-action-optimization")
}

// TestUXG1AuxMarkerBlocksRelocateToAppendix — M1-⑤: the engine 树读法: lead
// legend and 各列口径: glossary blocks (single tracefence marker source)
// relocate into the HTML 阅读参考 appendix.
func TestUXG1AuxMarkerBlocksRelocateToAppendix(t *testing.T) {
	blocks := runtimeTraceCausalProjectionCluster(uxr1FourOneSixFiveProjection(), "zh", runtimeTraceProjUserFocus{})
	for _, marker := range tracefence.AuxRefMarkers() {
		var carrier string
		for _, block := range blocks {
			if strings.Contains(block.Text, marker+"\n- ") {
				carrier = block.Text
				break
			}
		}
		if carrier == "" {
			t.Fatalf("no engine cluster block carries the %q marker block", marker)
		}
		// Feed the engine text through the real HTML pipeline: the marker
		// paragraph + list must relocate (pointer at site, appendix at end).
		html, err := preview.RenderMarkdownHTML([]byte(carrier + "\n"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(html, `<section class="aux">`) || !strings.Contains(html, "阅读参考") {
			t.Errorf("engine %q block did not relocate to the aux appendix:\n%s", marker, html)
		}
	}
}

// TestUXG1LegendCatalogNamesEveryStateMark — T3a: every HTML-face state mark
// (the tracefence directory, including the UXG-0 D3 ⊘ and the UXR-1 ⧗
// additions) is named by at least one legend catalog entry — a displayed
// mark without a legend line is a broken promise surface (图例是承诺面).
func TestUXG1LegendCatalogNamesEveryStateMark(t *testing.T) {
	entries := runtimeTraceProjLegendCatalog()
	for _, mark := range tracefence.StateMarks() {
		found := false
		for _, entry := range entries {
			if strings.Contains(entry.ZH, mark.Glyph) || strings.Contains(entry.EN, mark.Glyph) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("state mark %q (class %s) has no legend catalog entry naming it", mark.Glyph, mark.Class)
		}
	}
}

// --- T3b/T3c · legend promise census (承诺句必须有数据背书,杀形C/OM-1 病) ------
//
// Divergence note vs the matrix spec §④: the spec sketched a typed
// PromisedComponents field ON runtimeTraceProjLegendEntry; UXG-1 keeps the
// promise table in test data instead (identical drift mechanization, zero
// production-surface churn — the batch promises no display-emitting code
// change beyond the M1 byte-identical single-sourcing). Escalate to the typed
// field when the OM-1 fix batch first CONSUMES a promise registration.

type uxg1LegendPromise struct {
	// Components — Node contract fields the entry's wording names as numeric
	// components; each must be displayed or ledgered known_gap (挂账形:
	// OM-1 stays green here until its fix batch flips the contract row).
	Components []string
	// Reason — for semantic-only marker sentences (caliber/dedup wording):
	// why no component registration is due.
	Reason string
}

var uxg1LegendPromises = map[runtimeTraceProjMark]uxg1LegendPromise{
	runtimeTraceProjMarkPeriodicSource: {
		Components: []string{"PeriodicLatenessMS"},
		Reason:     "承诺「只计 runnable 与信号迟到量」:迟到量分量数据车道已铸、词面未接(OM-1);runnable 分量载体缺 producer note(OM-1② IC-A)",
	},
	runtimeTraceProjMarkMergedMax: {
		Reason: "「数值取其中最大一项」=×N 口径词(MergedMaxMS displayed),非数值分解承诺",
	},
	runtimeTraceProjMarkOverWindowShare: {
		Reason: "「只计一次」=重复记录去重口径词,非数值分解承诺",
	},
	runtimeTraceProjMarkFamilyChainIntersection: {
		Components: []string{"SemanticChainProjectedMS"},
		Reason:     "「只计成员段与链窗精确交集」——交集载体已显示(行3 链上计入)",
	},
	runtimeTraceProjMarkEffectiveBreakdown: {
		Components: []string{"EffectiveImpactMS"},
		Reason:     "「有效归因的构成」承诺由 Σ计入==V 机器 pin 背书(§24.1 恒等式)",
	},
	// INV-SUPPLY 件① (§29.61.11, 2026-07-14): the compound suffix promises
	// the typed ratio of two DISPLAYED values (行3/供给折算 line carry both).
	runtimeTraceProjMarkSupplyGapDominant: {
		Components: []string{"SupplyFoldDeficitMS", "EffectiveImpactMS"},
		Reason:     "「席位影响以…成分为主」= 两已显示 typed 值的纯比较(types.TraceSupplyGapDominant);构成拆解指向既有行3面",
	},
	// INV-SUPPLY 件③ (§29.61.11, 2026-07-14): the ◎ leverage note transcribes
	// the SAME balance-gated component pair 行3 displays.
	runtimeTraceProjMarkElimComposition: {
		Components: []string{"GatedRunnableMS", "GatedRunningDeficitMS"},
		Reason:     "「可消除构成」= 行3 拆解分量的杠杆转录(runtimeTraceProjInversionComponents 同一 balance-gated 构建器)",
	},
	// GATED-CAL 件1 (§29.104.16.1 M3, 2026-07-16): the composite caliber word
	// names the SAME typed component pair the 行3 equation displays — the word
	// exists exactly because those two displayed components are of two calibers.
	runtimeTraceProjMarkGatedCompositeCaliber: {
		Components: []string{"GatedRunnableMS", "GatedRunningDeficitMS"},
		Reason:     "「构成,见明细」= 指向该行行3「有效归因 V = …」分解行/明细块的既有分量显示面(同一 typed 分量对,精确门 GatedRunnable/GatedRunningDeficit>0)",
	},
	// XLANE-2 件2 (裁定④ 披露式拆分, 2026-07-17): the 「其中 X ms 与语义席
	// [E#]重叠」 clause transcribes the engine-typed per-partner intersection
	// roster verbatim — X and its [E#] partner are both displayed on the
	// clause line itself (行内), backed by the typed
	// Node.SelfGapSemanticOverlaps carrier.
	runtimeTraceProjMarkSelfGapSemanticOverlap: {
		Components: []string{"SelfGapSemanticOverlaps"},
		Reason:     "「其中 X ms 与语义席[E#]重叠」= 引擎 typed 区间交集清单的逐字转录(runtimeTraceProjStampSelfGapSemanticOverlaps 行包络 verbatim 解析),X 与 [E#] 均在 clause 行自身显示",
	},
	// LEVELMERGE-1 件2 (方案 P 区间分账, 2026-07-18): the split sentence
	// displays both typed halves on the row itself; claimed+residual==full is
	// engine-pinned (TestGatedShareSplitIdentityOrdinary).
	runtimeTraceProjMarkGatedShareSplit: {
		Components: []string{"GatedShareClaimedMS", "GatedShareFullMS"},
		Reason:     "「已计入份+残余份==修前全账」= 引擎 A+B 恒等 pin 背书;两分量与残余(全-已计入)均在行2句面自身显示",
	},
	// LEVELMERGE-1 件2 fail-open (裁定④): X is the engine-typed overlap
	// measure displayed on the clause itself; no value decomposition claimed.
	runtimeTraceProjMarkGatedShareOverlap: {
		Components: []string{"GatedShareOverlapDisclosureMS"},
		Reason:     "「其中 X ms 与反转席[E#]重叠」= 引擎 typed 真段重叠测度的逐字转录(下界),主值零动、无拆分承诺",
	},
	// LEVELMERGE-1 件3 (两向互指, 2026-07-18): a pointer-pair promise (构成段
	// 陈列指向), not a value decomposition — the seat value is untouched and
	// no component arithmetic is claimed.
	runtimeTraceProjMarkAggregateMemberCrossRef: {
		Reason: "「构成段见[E#…]/本行为构成段,不另计」= 两向互指指针词(ORD-A typed 成员谓词解析),非数值分解承诺",
	},
	// A2 件4① (§29.174 UX-16①, 2026-07-21): the renamed state-makeup edge
	// word — a same-thread same-window state DISTRIBUTION relation (原 成因),
	// not a value decomposition; the child rows are ordinary state rows and
	// no component arithmetic is claimed.
	runtimeTraceProjMarkEdgeCause: {
		Reason: "「├─构成─」= 同线程同窗状态分布关系词(件4① 成因 改名),非数值分解承诺",
	},
	// SPANTOP-1 (§29.131, 2026-07-18): the constituent top-3 decomposition —
	// every displayed component (per-member 单段 value, 行a..b range) is the
	// verbatim typed carrier, and the 前3+余行==席行合计 identity is enforced
	// as the block's own emission gate (µs arithmetic; the remainder is
	// derived as 席行合计−Σ(top3), so the printed identity holds by
	// construction — TestSpanTop* pins).
	runtimeTraceProjMarkFamilySpanTop: {
		Components: []string{"FamilyMemberWallMS", "FamilyMemberLineRanges"},
		Reason:     "「前3单段+余行合计==席行合计」= 显示门自身以 µs 整数恒等为发块前提(不满足整块不发,席行现状);单段/行区间均为 typed 载体逐字转录",
	},
}

// uxg1ProsePromises — decomposition-promise sentences on the NON-catalog
// promise faces (阅读参考 line, detail (a) cell), pinned verbatim: when the
// OM-1 fix batch changes these faces it must re-register here and flip the
// contract row (账实一致).
var uxg1ProsePromises = []struct {
	Verbatim   string
	Components []string
}{
	{"- 周期性信号源行:有效归因 = runnable 全额 + 信号迟到量", []string{"PeriodicLatenessMS"}},
	{"%.3fms(可运行+迟到量)", []string{"PeriodicLatenessMS"}},
}

func TestUXG1LegendPromisesBacked(t *testing.T) {
	markers := []string{"只计", "构成", "其中"}
	seen := map[runtimeTraceProjMark]bool{}
	for _, entry := range runtimeTraceProjLegendCatalog() {
		hasMarker := false
		for _, marker := range markers {
			if strings.Contains(entry.ZH, marker) {
				hasMarker = true
				break
			}
		}
		if !hasMarker {
			continue
		}
		seen[entry.Mark] = true
		promise, ok := uxg1LegendPromises[entry.Mark]
		if !ok {
			t.Errorf("图例条 mark=%d 含分解词面(%v)但未登记承诺表(新承诺必先接数据或明示豁免):%s", entry.Mark, markers, entry.ZH)
			continue
		}
		for _, component := range promise.Components {
			d, ok := nodeFieldContract[component]
			if !ok {
				t.Errorf("图例条 mark=%d 承诺分量 %s 不在 Node 契约表", entry.Mark, component)
				continue
			}
			if d.Status != "displayed" && d.Status != "known_gap" {
				t.Errorf("图例条 mark=%d 承诺分量 %s 契约状态=%s(OM-1 承诺-数据脱节形:分量既非 displayed 亦非挂账 known_gap)", entry.Mark, component, d.Status)
			}
		}
	}
	for mark := range uxg1LegendPromises {
		if !seen[mark] {
			t.Errorf("承诺表幽灵行 mark=%d:图例条已无分解词面,删行", mark)
		}
	}
	// Prose promise faces: verbatim presence + component backing.
	src := readDisplayAuthoritySources(t)
	for _, prose := range uxg1ProsePromises {
		if !strings.Contains(src, prose.Verbatim) {
			t.Errorf("散文承诺句已漂移/消失(修改承诺面必须同步承诺表+契约翻状态):%q", prose.Verbatim)
		}
		for _, component := range prose.Components {
			d, ok := nodeFieldContract[component]
			if !ok || (d.Status != "displayed" && d.Status != "known_gap") {
				t.Errorf("散文承诺句分量 %s 无数据背书(status=%v)", component, d)
			}
		}
	}
}

// --- UXG-1 修复轮 F2 · tool-side display-authority literal tripwire ------------
//
// The FUNCTIONAL emitters of every display-table word (ordinal chips, wrap
// atoms, action cells, column headers, LLM-face position words, H2 titles,
// aux markers) now DERIVE from internal/tracefence. What legitimately remains
// in the display-authority sources is PROSE: legend teaching lines that quote
// the chip form verbatim, see-also references to a chapter name, board-name
// prose, layer-cell words that textually coincide with the chapter title, and
// one log line. This scan pins those residues per file × literal with the
// counts below — a NEW occurrence (someone hand-spelling a table word instead
// of consuming the constant) reddens, and a composition that removes one
// reddens too (stale allowlist rows die loudly).
//
// Scope note: the state-mark GLYPH directory is deliberately outside this
// scan — legend teaching lines quote glyphs pervasively by design, and the
// functional glyph face is governed by the §24.3 typed form table plus the
// M3 keyset census instead. tracefence.ActionWordENShort is also excluded:
// "optimize" is a substring of ordinary prose ("optimization", …).
//
// F4 counting blind spot (documented): a same-file swap that removes one
// legitimate prose occurrence and adds one hand-copied emitter of the SAME
// literal keeps the count unchanged and is not caught — 漏报可能,报红必真;
// the cross-package render pins and byte-pinned fixtures are the second net.
var uxg1ToolAuthorityLiteralAllowlist = map[string]map[string]int{
	"answer_document_mutation_runtime_tree.go": {
		// Legend teaching quotes (催化行 725/761-762/920-921/996-997/1005),
		// layer words (2975-3001), state-action prose (7383), flat-board
		// fallback phrase (9107), badge legend line (➊..➎), and see-also
		// chapter references. +1 (2026-07-12, P9 §29.42 案1): the pacing_idle
		// legend teaching entry's 不计入根因排序 clause — legend prose quoting
		// the channel word, not a new hand-copied board emitter. +1
		// (2026-07-12, 复核 P2-1): the periodic_idle fork's twin clause. +2
		// (2026-07-12, CAL-1 件⑤): the ∿ cadence-idle legend entry's
		// 不参与根因排序 clause + the pacing 行2 word 节拍吻合·上下文(不参与
		// 根因排序) — both prose quoting the channel word inside a context
		// disclaimer, not board emitters. +1 (2026-07-12, V2-P0): the ⌗
		// 口径旁栏 legend entry's 不参与根因排序 clause (same prose family).
		// +1 (2026-07-13, SELF-SEM §29.61.1): the 目标自身·确定性优化 legend
		// entry's 参与根因排序 clause — legend prose quoting the channel word,
		// not a new hand-copied board emitter.
		// +1 (2026-07-13, SELF-ALL §29.61.2): the 目标自身·墙钟席 legend entry's
		// 参与根因排序 clause — same legend-prose family as the SELF-SEM line.
		// +1 证据索引 (2026-07-15, RNB-5B 件⑦): the 其余N项微额锚定席 legend
		// entry's clause — legend prose pointing at the section, not a new
		// hand-copied section-title emitter. 修复轮 P2-4 rewrote that clause to
		// 「证据经 [E#] 索引」 (honest reach), returning the count to 5.
		// +1 (2026-07-19, SPANVIS-1): the ◈ 业务span提示 legend entry's
		// 不参与根因排序 clause — legend prose quoting the channel word inside
		// a context disclaimer (⌗ 口径旁栏 precedent family), not a new
		// hand-copied board emitter.
		// +2 (2026-07-20, DISPHYG-3 件6): the two-ruler legend entry's
		// participant cross-reference clause quotes 根因排序#N on BOTH
		// language faces (legend prose teaching the chip word; the chip
		// itself composes from tracefence.SeatChannelChain* — not a new
		// hand-copied board emitter).
		// −1 (2026-07-21, RULE3-1 件2): the ➊..➎ legend entry re-composes
		// its 根因排序 mentions from tracefence.SeatChannelChainZH; the
		// 成因行身份行 entry's two-source clause composes the same way.
		// +2 「➊」 (RULE3-1 件2+件3): the same legend entry's 「➊=板内值序」
		// crown-caliber clause quotes the glyph on both language faces —
		// legend prose, not a new badge emitter.
		"根因排序": 22, "邻近影响": 1, "root-cause rank": 3, "adjacent-impact": 1,
		"优化点": 2, "optimization point": 1, "确定性优化点": 3, "证据索引": 5,
		// +2 「➊」/+2 「➎」 (A2 件2, 2026-07-21): the tree-head mini legend's
		// closed glyph-key table row (「➊..➎该板TOP5」 zh + en) — a key entry
		// derived from the badge mark, not a new board emitter.
		"➊": 6, "➎": 4,
	},
	"answer_document_mutation_runtime.go": {
		// Metric-table glossary prose, audit-token glossary (rank=根因排序),
		// layer/cell words coinciding with the chapter title, optimization
		// prose + at-cap caveat, one log line, see-also chapter references.
		// +1 (2026-07-19, SPANVIS-1): the ◈ 阅读参考 dual-lever entry's
		// 不参与根因排序 clause — reading-reference prose quoting the channel
		// word (SCORE-DERIV entry family), not a board emitter.
		// +1 (2026-07-25, GAP-B1 §13.3): the 目标窗内状态账 coverage line's
		// 不作为可消除影响参与根因排序席位 mechanism clause — coverage prose
		// quoting the channel word, not a board emitter.
		// −1 zh/en (2026-08-01, EVAL-B22-SEMAXIS1): the optimization-table
		// intro now describes separate raw/effective axes instead of repeating
		// the action-word noun; the typed table header remains authoritative.
		"根因排序": 5, "root-cause rank": 1, "优化点": 2, "optimization point": 3,
		"确定性优化点": 7, "Deterministic Optimization Points": 1, "证据索引": 2,
	},
	"answer_document_mutation_runtime_rcm.go": {
		// SPANVIS-1 (2026-07-19): the file joined the display-authority list
		// with the ◈ tree-block head's 不参与根因排序 clause — prose quoting
		// the channel word inside the advisory-block disclaimer, not a
		// hand-copied chip emitter.
		"根因排序": 1,
	},
	"answer_document_mutation_runtime_elim.go": {
		// ELIM-V2 (2026-07-18): the file joined the display-authority list
		// with its PRE-EXISTING footnote clause quoting the channel word
		// (◇ O-5 pointer 不在根因排序) — a prose reference to the board, not a
		// hand-copied chip emitter. 双复核 件13 (2026-07-21): the ⛓ semantic
		// census 未入根因排序 clause sank onto the aux legend entry (2 → 1).
		"根因排序": 1,
	},
}

func TestUXG1ToolAuthorityKeepsNoUncountedTableLiteral(t *testing.T) {
	literals := []string{
		tracefence.SeatChannelChainZH, tracefence.SeatChannelChainEN,
		tracefence.SeatChannelAdjacentZH, tracefence.SeatChannelAdjacentEN,
		tracefence.ActionWordZH, tracefence.ActionWordEN,
		tracefence.SectionOptimizationZH, tracefence.SectionOptimizationEN,
		tracefence.SectionDetailZH, tracefence.SectionDetailEN,
		tracefence.SectionEvidenceZH, tracefence.SectionEvidenceEN,
		// UX-ANCHOR 件a (2026-07-14): the projection lead-section title joined
		// table ④ — every tool emitter (base title + the 对比总览/分区边界/
		// 覆盖边界 compositions) derives from the constant, so the allowlist
		// carries no residue rows for it.
		tracefence.SectionProjectionZH, tracefence.SectionProjectionEN,
		tracefence.AuxTreeLegendMarker, tracefence.AuxColumnGlossaryMarker,
	}
	literals = append(literals, tracefence.BadgeGlyphs()...)
	count := func(src, literal string) int {
		n := strings.Count(src, literal)
		switch literal {
		case tracefence.SeatChannelChainEN:
			// "root-cause ranking" prose is NOT the chip word.
			n -= strings.Count(src, tracefence.SeatChannelChainEN+"ing")
		case tracefence.ActionWordZH:
			// 确定性优化点 (chapter/layer word) is counted as its own literal.
			n -= strings.Count(src, tracefence.SectionOptimizationZH)
		case tracefence.ActionWordEN:
			// The capitalized column header "Optimization point" is a
			// different byte form (not this literal); no adjustment needed.
		}
		return n
	}
	for _, name := range infoContractDisplayAuthorityFiles {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := uxg1StripComments(string(raw))
		for _, literal := range literals {
			got := count(src, literal)
			want := uxg1ToolAuthorityLiteralAllowlist[name][literal]
			if got != want {
				t.Errorf("%s: 表词面 %q 出现 %d 次,allowlist 记 %d——新手抄发射面(改用 tracefence 常量/组合)或 allowlist 行过期,人工对账后同步(F2 tripwire)", name, literal, got, want)
			}
		}
	}
}
