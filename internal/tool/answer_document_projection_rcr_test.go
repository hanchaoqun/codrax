package tool

// answer_document_projection_rcr_test.go — PTV8-RCR-A pins (ledger
// docs/design/real_trace_campaign_20260705.md §24/§24.1/§24.2/§24.3):
//
//   1. opendir_02 E4-E8 node-group fixture — the three cause-node terminal
//      forms VERBATIM (lock degenerate two-line / IO event-class / inversion
//      four-line with the task-ruled E7 mock bytes).
//   2. The §24.1 identity pins as MACHINE checks on the real compile chain:
//      Σ计入 == 有效归因 == 行3 right side (a non-balancing decomposition
//      REFUSES to render), and 行1 value == 窗口投影 == bar base (the
//      effective never replaces the projection on 行1).
//   3. The five §24 ② retirement negatives (同段rank行并入 / 机制构成 on
//      inversion nodes / 候选根因 / user-facing "gated" / the R1 ⧖ runnable
//      sub-row).
//   4. §24.3 glyph three hard rules: single display cell, no VS16, glyph and
//      行2 category from ONE typed table (two-column single source), legend
//      entries generated from the same table.
//   5. §24.1补 caliber-word legend entries render exactly on demand.
//   6. A5 hop-lane pseudo-residual arms (positive jitter arm / beyond-jitter
//      arm / legacy negative).
//   7. A6 F-5 single source: the semantic source-window ±1ms rebase judgment
//      is ONE helper for the % lane and the share-wording lane.

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// rcrOpendirProjection mirrors cust_trace_opendir_02.txt's E4-E8 node group
// with the specimen's real magnitudes: the E4 lock row (blocking_span,
// 112.223), the E5 IO event-class fold (io_latency ×6, Σ2.858, single max
// 0.568), the E6 block-IO row (1.136) and the E7/E8 same-segment twin pair
// (running 58.919 raw; gated 20.713 + 16.697 == effective 37.410, rank=2 on
// the folded rank lane). E1/E2 target sleep rows keep the evidence numbering
// aligned with the specimen (E4 onward for the cause group).
func rcrOpendirProjection() types.TraceCausalProjection {
	target := ".ugc.aweme.lite-16547"
	holder := "#RxComputationT-16816"
	actual := 59.050
	return types.TraceCausalProjection{
		WakeupPath:    []string{holder, target},
		WindowStartTs: 33872.289,
		WindowEndTs:   33872.409,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "op-e1",
				Subject: target, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 112.175,
				LineStart: 45450, LineEnd: 79134, Confidence: 0.78},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "op-e2",
				Subject: target, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 2.082, LineStart: 81277, LineEnd: 82860,
				Confidence: 0.78},
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "op-e3",
				Subject: target, ChainRelevance: "on_chain", ImpactMS: 2.082,
				LineStart: 81277, LineEnd: 82860,
				UndrillableReason: "missing_wakeup", Confidence: 0.70},
			// E4 — the lock main row (degenerate two-line form: 计入==原始).
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "op-e4",
				Subject: holder, Predicate: "root_cause_primary",
				Object: "monitor_contention", BlockingKind: "monitor_contention",
				BlockingSubjectIsHolder: true, BlockingPeer: target,
				BlockingHolderSite: "java.lang.String[] android.content.res.AssetManager.list(java.lang.String)(AssetManager.java:1258)",
				Rank:               1, Tier: "primary", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 112.223, CumulativeImpactMS: 112.223, EffectiveImpactMS: 112.223,
				LineStart: 45696, LineEnd: 79136, Confidence: 0.67},
			// E5 — the IO event-class fold (×6, effective == single max).
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "op-e5",
				Subject: holder, Predicate: "root_cause_primary", Object: "io_latency",
				Rank: 4, Tier: "primary", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 2.858, CumulativeImpactMS: 2.858, EffectiveImpactMS: 0.568,
				MergedCount: 6, MergedMinMS: 0.374, MergedMaxMS: 0.568,
				SecondaryObjects: []string{"udk-irq-10-90", "udk-irq-2-77", "udk-irq-6-84"},
				LineStart:        59938, LineEnd: 60012, Confidence: 0.86},
			// E6 — the block-IO row (degenerate two-line form).
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "op-e6",
				Subject: holder, Predicate: "root_cause_primary", Object: "block_io_by_inode",
				Rank: 3, Tier: "primary", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 1.136, CumulativeImpactMS: 1.136, EffectiveImpactMS: 1.136,
				LineStart: 59099, LineEnd: 77131, Confidence: 0.76},
			// E7 — the chain-lane running hop of the same-segment twin pair.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "op-e7",
				Subject: holder, Predicate: "wakeup_causal_impact", Object: "running",
				StateKind: "running", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 58.919, CumulativeImpactMS: 58.919, EffectiveImpactMS: 37.410,
				ActualImpactMS: actual, PriorityInversionCandidate: true,
				GatedRunnableMS: 20.713, GatedRunningDeficitMS: 16.697, RunnableMS: 20.713,
				MergedEvidenceIDs: []string{"op-e7-twin"},
				LineStart:         45689, LineEnd: 79142, Confidence: 0.78},
		},
		// E8 — the rank-lane twin (folds into E7; rank/confidence ride 行2,
		// its E# merges into 行1's bracket).
		PrimaryRootCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "op-e8",
				Subject: holder, Predicate: "root_cause_secondary",
				Object: "priority_inversion_candidate", Rank: 2, Tier: "secondary",
				ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 37.410, CumulativeImpactMS: 58.919, EffectiveImpactMS: 37.410,
				PriorityInversionCandidate: true,
				GatedRunnableMS:            20.713, GatedRunningDeficitMS: 16.697, RunnableMS: 20.713,
				LineStart: 45689, LineEnd: 79142, Confidence: 0.91},
		},
	}
}

// rcrSupplyFoldDominantProjection exercises the §24.1补 big-cluster-fmax /
// lower-bound caliber legend marks through the Dominant supply-fold verdict
// (a running-dominant NON-inversion row — the clause lane the RCR batch keeps).
func rcrSupplyFoldDominantProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-9", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "sfdom-1",
			Subject: "worker-9", Object: "running", StateKind: "running",
			ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 20.0, CumulativeImpactMS: 20.0,
			SupplyFoldComputed: true, SupplyFoldDeficitMS: 5.0, SupplyFoldIdealMS: 15.0,
			SupplyFoldKnownMS: 20.0, Confidence: 0.8,
		}},
	}
}

func rcrOpendirFence(t *testing.T, zh bool) (string, runtimeTraceProjTreeModel) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(rcrOpendirProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	return runtimeTraceProjTreeFence(model, zh), model
}

// --- 1. the three cause-node terminal forms, VERBATIM ------------------------

func TestRCRCauseNodeTerminalFormsVerbatim(t *testing.T) {
	fence, _ := rcrOpendirFence(t, true)
	// E4 lock node — §24.2 degenerate two-line form: 行1 glyph ⊗ + badge,
	// 行2 = 类别·根因排序#1·置信 + folded 行3 tail (计入==原始 → 全额).
	for _, want := range []string{
		"❶⊗ #RxComputationT-16816 · 持锁阻塞",
		"· 锁竞争·持锁·根因排序#1·置信中·有效归因 112.223ms(全额)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("E4 lock terminal form missing %q:\n%s", want, fence)
		}
	}
	// E5 IO event-class node — ×N on 行1, the (a–b,共N次) range + 单次最大
	// caliber on 行3, the 影响点 roster as its own list line.
	for _, want := range []string{
		"⛓ IO延迟 ×6",
		"· IO阻塞候选·根因排序#4·置信高",
		"· 有效归因 0.568ms = 单次最大(0.374–0.568ms,共6次)",
		"· 影响点 udk-irq-10-90/udk-irq-2-77/udk-irq-6-84",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("E5 event-class terminal form missing %q:\n%s", want, fence)
		}
	}
	// E6 block-IO node — degenerate two-line form.
	for _, want := range []string{
		"⛓ 块设备IO(inode)",
		"· IO阻塞候选·根因排序#3·置信中·有效归因 1.136ms(全额)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("E6 terminal form missing %q:\n%s", want, fence)
		}
	}
	// E7 inversion node — the task-ruled four-line mock VERBATIM: 词位 =
	// runnable+running, 行2 with the folded rank twin's seat/confidence, 行3
	// identity breakdown, two 拆解子行; the twin's E# merged into 行1.
	// (Badges: PTV6 #11 one seat per subject — E4 already seats
	// #RxComputationT-16816, so E5-E8 wear no ❷/❸; the specimen agrees.)
	for _, want := range []string{
		"⇅ runnable+running",
		"⚠实际59.050ms · [E7(+1)+E8]",
		"· 优先级反转候选·根因排序#2·置信高",
		"· 有效归因 37.410ms = runnable(全额) 20.713ms + running(折算) 16.697ms",
		"· runnable 原始 20.713ms → 计入 20.713ms(全额)",
		"· running 原始 58.919ms → 计入 16.697ms(折算,按下游消费核)",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("E7 inversion terminal form missing %q:\n%s", want, fence)
		}
	}
}

// --- 2. the §24.1 identity pins (machine, real compile chain) ----------------

// Σ计入 == 有效归因 == 行3 right side: a decomposition that does NOT balance
// at print precision REFUSES to render the 「=」row (fail-open to the plain
// single-source tag) — mutation guard: skew one component and the row is gone.
func TestRCRIdentitySumEqualsEffectiveOrNoBreakdown(t *testing.T) {
	balanced := types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "running", StateKind: "running",
		PriorityInversionCandidate: true, ChainRelevance: "on_chain",
		ImpactMS: 58.919, EffectiveImpactMS: 37.410,
		GatedRunnableMS: 20.713, GatedRunningDeficitMS: 16.697, Confidence: 0.9, Rank: 2,
	}
	row := runtimeTraceProjTreeRow{Node: balanced, Kind: runtimeTraceProjTreeRowChain,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	structured, ok := runtimeTraceProjCauseStructuredParts(row, true)
	if !ok || structured.Breakdown == "" {
		t.Fatalf("balanced components must render the 行3 breakdown: %+v", structured)
	}
	// The printed sides agree at %.3f: 20.713 + 16.697 == 37.410.
	if !strings.Contains(structured.Breakdown, "有效归因 37.410ms = runnable(全额) 20.713ms + running(折算) 16.697ms") {
		t.Fatalf("行3 must publish the balancing identity verbatim: %q", structured.Breakdown)
	}
	// Mutation arm: skew the running component by one printed unit — the
	// identity breaks and the 「=」row must refuse to render.
	skewed := balanced
	skewed.GatedRunningDeficitMS = 16.696
	skewedRow := runtimeTraceProjTreeRow{Node: skewed, Kind: runtimeTraceProjTreeRowChain,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	if structured, ok := runtimeTraceProjCauseStructuredParts(skewedRow, true); ok &&
		(structured.Breakdown != "" || len(structured.SubRows) > 0) {
		t.Fatalf("a non-balancing decomposition must never render the 「=」row: %+v", structured)
	}
	// The engine-published effective is the ONLY total source: an unpublished
	// effective never mints a 行3 total from the component sum.
	unpublished := balanced
	unpublished.EffectiveImpactMS = 0
	if _, _, ok := runtimeTraceProjInversionComponents(unpublished, true); ok {
		t.Fatalf("an unpublished effective must never mint a 行3 total")
	}
}

// 行1 value == 窗口投影 == bar base: the cause node's 行1 keeps the window
// projection (58.919) — the effective (37.410) lives on 行3 only, and the bar
// fills from the SAME projection value.
func TestRCRIdentityRow1KeepsWindowProjection(t *testing.T) {
	fence, model := rcrOpendirFence(t, true)
	var row1 string
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "runnable+running") && strings.Contains(line, "+E8]") {
			row1 = line
			break
		}
	}
	if row1 == "" {
		t.Fatalf("E7 行1 not found:\n%s", fence)
	}
	if !strings.Contains(row1, "58.919ms") || strings.Contains(row1, "37.410") {
		t.Fatalf("行1 must keep the window projection (58.919ms), never the effective: %q", row1)
	}
	// Bar base identity: the rendered bar is the SAME projection over the
	// SAME window denominator (compile-chain single source, re-derived here).
	if want := runtimeTraceProjBar(58.919, model.WindowMS, false); !strings.Contains(row1, want) {
		t.Fatalf("行1 bar must fill from the window projection (want %q): %q", want, row1)
	}
	if !strings.Contains(row1, " 49%") {
		t.Fatalf("行1 %% must divide the projection by the window: %q", row1)
	}
}

// --- 3. the five §24 ② retirement negatives ---------------------------------

func TestRCRRetiredWordingNeverReturns(t *testing.T) {
	projection := rcrOpendirProjection()
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	blocks := runtimeTraceCausalProjectionCluster(projection, "zh", runtimeTraceProjUserFocus{})
	var md strings.Builder
	for _, block := range blocks {
		md.WriteString(block.Text)
		md.WriteString("\n")
		for _, item := range block.Items {
			md.WriteString(item.Label + " " + item.Text + "\n")
		}
	}
	_ = evidence
	page := md.String()
	for name, banned := range map[string]string{
		"rank_fold_note":     "同段rank行并入",       // ① RNB note → native single node
		"mechanism_sentence": "机制构成",            // ② retired on inversion nodes (the only rows here that could carry it)
		"candidate_chip":     "候选根因",            // ③ empty-chip retirement
		"gated_word":         "gated",           // ④ user-facing gated (wire tokens keep their own surfaces)
		"runnable_subrow":    "就绪排队积压·gated 分量", // ⑤ R1 sub-row
		"runnable_subrow_2":  "不重复计入排序",         // ⑤ R1 sub-row tail
		"composition_tag":    "影响构成",            // D3 tag (absorbed into 行3)
		"en_candidate":       "candidate cause", // ③ EN twin
		"en_gated":           "gated caliber",   // ④ EN twin
		"en_gated_component": "gated component", // ⑤ EN twin
	} {
		if strings.Contains(page, banned) {
			t.Fatalf("%s: retired wording %q resurfaced:\n%s", name, banned, page)
		}
	}
}

// --- 4. §24.3 glyph hard rules -----------------------------------------------

func TestRCRImpactFormGlyphsSingleCellNoVS16(t *testing.T) {
	// 复核 F3 裁定: the root glyph joins the single-cell loop (it escaped the
	// form-table pin as a free-standing constant; ⊚ U+229A is EAW-Neutral —
	// the earlier ◎ U+25CE was Ambiguous and double-width on CJK terminals).
	rootAndForms := []runtimeTraceProjImpactFormSpec{{Glyph: runtimeTraceProjRootGlyph, Form: -1}}
	seen := map[string]bool{}
	for _, spec := range append(rootAndForms, runtimeTraceProjImpactFormSpecs()...) {
		if spec.Glyph == "" {
			t.Fatalf("form %d has no glyph", spec.Form)
		}

		if utf8.RuneCountInString(spec.Glyph) != 1 {
			t.Fatalf("glyph %q must be exactly one rune (text presentation, no VS16)", spec.Glyph)
		}
		if strings.ContainsRune(spec.Glyph, '️') || strings.ContainsRune(spec.Glyph, '︎') {
			t.Fatalf("glyph %q carries a variation selector — text presentation only", spec.Glyph)
		}
		if w := runewidth.StringWidth(spec.Glyph); w != 1 {
			t.Fatalf("glyph %q measures %d display cells, want 1 (等宽单格宽)", spec.Glyph, w)
		}
		if seen[spec.Glyph] {
			t.Fatalf("glyph %q assigned to two forms — the closed set is one glyph per form", spec.Glyph)
		}
		seen[spec.Glyph] = true
	}
	// F3 rationale pinned: the root glyph holds one cell even under an
	// East-Asian-width terminal (the retired ◎ U+25CE was EAW-Ambiguous and
	// measured 2 there). The pre-RCR marks ⛓/❶/◇/▒ are also Ambiguous but
	// pre-date this batch (PTV4 pinned) — re-litigating them needs a ruling.
	eaCond := runewidth.NewCondition()
	eaCond.EastAsianWidth = true
	if w := eaCond.StringWidth(runtimeTraceProjRootGlyph); w != 1 {
		t.Fatalf("root glyph %q measures %d cells under EastAsianWidth — pick an EAW-Neutral glyph", runtimeTraceProjRootGlyph, w)
	}
	// ⊚ replaced 🎯: the root header and its legend entry carry no emoji.
	if strings.Contains(runtimeTraceProjTreeHeaderLabel(runtimeTraceProjTreeModel{Target: "t-1"}, true), "🎯") {
		t.Fatalf("the root header must not carry the retired 🎯 emoji")
	}
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if strings.Contains(entry.ZH, "🎯") || strings.Contains(entry.EN, "🎯") {
			t.Fatalf("legend entry still carries the retired 🎯 emoji: %q", entry.ZH)
		}
	}
}

// Two-column single source: the 行2 category word for a table-sourced form IS
// the table cell, and the generated legend entry carries the SAME glyph —
// mutation guard: editing either column alone (hand-forked copy) goes red.
func TestRCRImpactFormTableSingleSource(t *testing.T) {
	catalog := runtimeTraceProjLegendCatalog()
	entryFor := func(mark runtimeTraceProjMark) *runtimeTraceProjLegendEntry {
		for i := range catalog {
			if catalog[i].Mark == mark {
				return &catalog[i]
			}
		}
		return nil
	}
	for _, spec := range runtimeTraceProjImpactFormSpecs() {
		entry := entryFor(spec.Mark)
		if entry == nil {
			t.Fatalf("form %d (glyph %q) has no legend entry for its mark", spec.Form, spec.Glyph)
		}
		if !strings.Contains(entry.ZH, spec.Glyph) && !strings.Contains(entry.EN, spec.Glyph) {
			t.Fatalf("legend entry for form %d must carry the table glyph %q: %q", spec.Form, spec.Glyph, entry.ZH)
		}
		if spec.GeneratedLegend {
			if !strings.Contains(entry.ZH, spec.SemanticsZH) || !strings.Contains(entry.EN, spec.SemanticsEN) {
				t.Fatalf("generated legend entry must interpolate the table semantics (form %d): %q", spec.Form, entry.ZH)
			}
		}
	}
	// The 行2 category resolver reads the table column for a pure-state form
	// (running family) — never a hand-written copy.
	running := types.TraceCausalProjectionNode{Subject: "w-1", Object: "running", StateKind: "running"}
	word, relocated := runtimeTraceProjCauseCategoryWord(running, runtimeTraceProjTreeRowChain, true)
	spec, _ := runtimeTraceProjImpactFormSpecFor(runtimeTraceProjImpactFormRunning)
	if relocated || word != spec.CategoryZH {
		t.Fatalf("行2 category must read the table column (%q), got %q (relocated=%v)", spec.CategoryZH, word, relocated)
	}
	// The classifier and the glyph renderer agree (three-surface consistency).
	marks := &runtimeTraceProjMarkSet{}
	if icon := runtimeTraceProjStateIcon(running, runtimeTraceProjTreeRowChain, true, marks); icon != spec.Glyph {
		t.Fatalf("glyph renderer must read the same table cell: got %q want %q", icon, spec.Glyph)
	}
}

// --- 5. §24.1补 caliber legend entries render exactly on demand ---------------

func TestRCRCaliberLegendEntriesOnDemand(t *testing.T) {
	// The opendir fixture emits 全额/按下游消费核/单次最大 (行2 tails + 行3 +
	// 拆解子行) but no supply-fold clause → no 按大核满频/下界 entries.
	_, model := rcrOpendirFence(t, true)
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	for _, want := range []string{"- `全额` =", "- `折算,按下游消费核` =", "- `单次最大(a–b,共N次)` ="} {
		if !strings.Contains(legend, want) {
			t.Fatalf("caliber legend entry %q must render on demand:\n%s", want, legend)
		}
	}
	for _, absent := range []string{"- `按大核满频折算` =", "- `下界` ="} {
		if strings.Contains(legend, absent) {
			t.Fatalf("caliber legend entry %q must stay off (word never rendered):\n%s", absent, legend)
		}
	}
	// The Dominant supply-fold shape emits exactly the other pair, with the
	// ledger-verbatim 下界 explanation (§24.1补 用户问"下界"何意).
	foldModel := buildRuntimeTraceProjTreeModel(rcrSupplyFoldDominantProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_ = runtimeTraceProjTreeFence(foldModel, true)
	foldLegend := strings.Join(runtimeTraceProjLegendGroupLines(foldModel.Marks, true), "\n")
	for _, want := range []string{
		"- `按大核满频折算` =",
		"- `下界` = 保守最小值:频率数据缺失的片段计 0,折算未计大核单周期优势;真实可消除量只多不少。",
	} {
		if !strings.Contains(foldLegend, want) {
			t.Fatalf("supply-fold caliber legend entry %q must render:\n%s", want, foldLegend)
		}
	}
	for _, absent := range []string{"- `单次最大(a–b,共N次)` =", "- `折算,按下游消费核` ="} {
		if strings.Contains(foldLegend, absent) {
			t.Fatalf("caliber legend entry %q must stay off this shape:\n%s", absent, foldLegend)
		}
	}
}

// --- 6. A5: the hop-lane pseudo-residual arms (§15.D gap③, UXA 域A 条4) ------

// rcrHopOnlyProjection: the opendir hop-only coverage shape — the target
// publishes NO state-view symptom row; its hop-view sleep total is hopSleep
// and the depth-1 attribution is attributed.
func rcrHopOnlyProjection(hopSleep, attributed float64) types.TraceCausalProjection {
	target := ".ugc.aweme.lite-16547"
	holder := "#RxComputationT-16816"
	return types.TraceCausalProjection{
		WakeupPath:    []string{holder, target},
		WindowStartTs: 33872.289,
		WindowEndTs:   33872.409,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop-t",
				Subject: target, Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: hopSleep, Confidence: 0.78},
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "hop-h",
				Subject: holder, Predicate: "root_cause_primary", Object: "monitor_contention",
				BlockingKind: "monitor_contention", BlockingSubjectIsHolder: true,
				Rank: 1, ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: attributed, CumulativeImpactMS: attributed, EffectiveImpactMS: attributed,
				Confidence: 0.67},
		},
	}
}

func rcrWindowLine(t *testing.T, projection types.TraceCausalProjection) string {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	return runtimeTraceProjWindowLine(projection, model, true)
}

func TestRCRHopLaneOvershootJitterArmKillsPseudoResidual(t *testing.T) {
	// opendir_02 witness: attributed 112.223 overshoots the hop sleep 112.175
	// by 0.048ms (inside the §15.D jitter band) — the old whole-window arm
	// fabricated "未归因残差 7.777ms/6%" for a fully explained wait.
	// PTV8-RCR-B (UXA 域D #33/#34 + 域B #3 verify, 2026-07-08). EVOLUTION
	// RECORD: the arm STRUCTURE is unchanged; the pinned words evolved with
	// the word families — 目标睡眠→关注线程睡眠, 关注窗口→分析窗,
	// (链上)归因口径合计→各链上口径合计, on-chain 已归因→链上已归因, the
	// 残差 statistics word retired (未归因).
	// COV 批 (§24.11 C-3 后半场, 2026-07-08). EVOLUTION RECORD: 各链上口径合计 →
	// 链上单项最大 (名实对齐: the numerator is a single-row MAX,墙钟不可求和;
	// huadong_78 "各链上口径合计 4.431ms" = 单行 E15 冒名).
	line := rcrWindowLine(t, rcrHopOnlyProjection(112.175, 112.223))
	if !strings.Contains(line, "关注线程睡眠 112.175ms 已全部由链上解释(链上单项最大 112.223ms,略超 0.048ms,属状态段边界抖动);占分析窗 94%。") {
		t.Fatalf("jitter arm must render the UXA final wording:\n%s", line)
	}
	if strings.Contains(line, "未归因") {
		t.Fatalf("the pseudo-residual must be dead on the jitter arm:\n%s", line)
	}
	// Beyond the jitter band: both magnitudes, no percentage, no residual.
	beyond := rcrWindowLine(t, rcrHopOnlyProjection(100.000, 112.223))
	if !strings.Contains(beyond, "关注线程睡眠 100.000ms;链上单项最大 112.223ms,超出关注线程睡眠 12.223ms") ||
		!strings.Contains(beyond, "不给出覆盖百分比,差值不计为未归因") {
		t.Fatalf("beyond-jitter arm must disclose both magnitudes without arithmetic:\n%s", beyond)
	}
	if strings.Contains(beyond, "%,未归因") {
		t.Fatalf("beyond-jitter arm must not fabricate coverage arithmetic:\n%s", beyond)
	}
	// Legacy negative: attributed within the hop sleep keeps the whole-window
	// coverage sentence + the hop info line (word families applied, structure
	// byte-stable).
	legacy := rcrWindowLine(t, rcrHopOnlyProjection(112.175, 100.000))
	if !strings.Contains(legacy, "链上已归因 100.000ms(83%),未归因 20.000ms(17%)。") ||
		!strings.Contains(legacy, "关注线程睡眠 112.175ms 中 100.000ms 已由链上解释。") {
		t.Fatalf("the legacy within-sleep arm must keep its shape:\n%s", legacy)
	}
}

// --- 7. A6: the F-5 source-window rebase single source ------------------------

func TestRCRSemanticSourceWindowRebaseSingleSource(t *testing.T) {
	node := func(delta float64) types.TraceCausalProjectionNode {
		return types.TraceCausalProjectionNode{
			Subject: "t-1", Object: "jit_compile", SpanName: "span-x",
			ImpactMS:           50,
			QueryWindowStartTs: 100.0, QueryWindowEndTs: 100.0 + (101.0+delta)/1000,
		}
	}
	model := runtimeTraceProjTreeModel{WindowMS: 101}
	// Inside the ±1ms tolerance both lanes stay legacy (anchor-based; 0.5
	// keeps clear of float noise at the exact boundary).
	atBoundary := node(0.5)
	if _, ok := runtimeTraceProjSemanticSourceWindowRebaseMS(atBoundary, model.WindowMS); ok {
		t.Fatalf("±1ms boundary must stay on the anchor base")
	}
	row := runtimeTraceProjTreeRow{Kind: runtimeTraceProjTreeRowSemantic, Node: atBoundary}
	if _, ok := runtimeTraceProjSemanticSourceWindowShareBaseMS(row, model.WindowMS); ok {
		t.Fatalf("%% lane must agree at the boundary (single helper)")
	}
	if text := runtimeTraceProjSemanticSpanShareText(atBoundary, 50, model, true); !strings.Contains(text, "占窗") {
		t.Fatalf("share wording must stay anchor-based at the boundary: %q", text)
	}
	// Beyond the tolerance both lanes rebase together.
	beyond := node(2.0)
	base, ok := runtimeTraceProjSemanticSourceWindowRebaseMS(beyond, model.WindowMS)
	if !ok || base <= 0 {
		t.Fatalf("beyond ±1ms must rebase on the source window")
	}
	row.Node = beyond
	if laneBase, laneOK := runtimeTraceProjSemanticSourceWindowShareBaseMS(row, model.WindowMS); !laneOK || laneBase != base {
		t.Fatalf("%% lane must read the SAME rebase judgment: %v %v", laneBase, laneOK)
	}
	if text := runtimeTraceProjSemanticSpanShareText(beyond, 50, model, true); !strings.Contains(text, "占其查询窗") {
		t.Fatalf("share wording must rebase with the %% lane: %q", text)
	}
}

// --- 复核 FAIL-1: the suppressed-clause inversion shape keeps its caliber
// --- legend entries (§24.1补 — the 下界 explanation the user personally
// --- asked for must be present wherever the word reaches the reader) --------

// rcrInversionSuppressedClauseProjection: a Triple-verdict inversion cause
// node — the 机制构成 clause is suppressed (§24 ②), the deficit reaches the
// reader through the detail block's 供给折算 line, and the 行3 breakdown
// balances (2+3==5).
func rcrInversionSuppressedClauseProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"worker-200", "app-100"},
		WindowStartTs: 100.0,
		WindowEndTs:   103.0,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "sup-1",
			Subject: "worker-200", Predicate: "root_cause_primary",
			Object: "priority_inversion_candidate", StateKind: "running",
			Rank: 1, ChainRelevance: "on_chain", ChainDepth: 1,
			ImpactMS: 20.0, CumulativeImpactMS: 20.0, EffectiveImpactMS: 5.0,
			PriorityInversionCandidate: true,
			GatedRunnableMS:            2.0, GatedRunningDeficitMS: 3.0, RunnableMS: 150.0,
			SupplyFoldComputed: true, SupplyFoldDeficitMS: 5.0, SupplyFoldIdealMS: 15.0,
			SupplyFoldKnownMS: 20.0, Confidence: 0.9,
		}},
	}
}

func TestRCRSuppressedClauseShapeKeepsCaliberLegend(t *testing.T) {
	projection := rcrInversionSuppressedClauseProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if strings.Contains(fence, "机制构成") {
		t.Fatalf("the mechanism sentence must stay suppressed on the inversion cause node:\n%s", fence)
	}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "供给折算缺口 5.000ms(折算,按大核满频,下界;独立口径,不计入有效归因)") {
		t.Fatalf("the deficit must keep its detail-block home:\n%s", detail)
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(model.Marks, true), "\n")
	for _, want := range []string{
		"- `按大核满频折算` =",
		"- `下界` = 保守最小值:频率数据缺失的片段计 0,折算未计大核单周期优势;真实可消除量只多不少。",
		"- `全额` =",
		"- `折算,按下游消费核` =",
	} {
		if !strings.Contains(legend, want) {
			t.Fatalf("suppressed-clause shape lost its caliber legend entry %q (FAIL-1 图例破洞):\n%s", want, legend)
		}
	}
	// Fail-open variant (identity cannot balance): the detail composition
	// text still carries 全额/按下游消费核折算 — their entries must follow.
	skewed := projection
	skewed.PrimaryRootCauses = append([]types.TraceCausalProjectionNode(nil), projection.PrimaryRootCauses...)
	skewed.PrimaryRootCauses[0].EffectiveImpactMS = 5.001
	skewedModel := buildRuntimeTraceProjTreeModel(skewed, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	_ = runtimeTraceProjTreeFence(skewedModel, true)
	skewedDetail := runtimeTraceProjDetailFullText(skewedModel, true)
	if !strings.Contains(skewedDetail, "runnable 2.000ms(全额)+ running 折算 3.000ms(按下游消费核折算)") {
		t.Fatalf("fail-open composition text must stay lossless:\n%s", skewedDetail)
	}
	skewedLegend := strings.Join(runtimeTraceProjLegendGroupLines(skewedModel.Marks, true), "\n")
	for _, want := range []string{"- `全额` =", "- `折算,按下游消费核` ="} {
		if !strings.Contains(skewedLegend, want) {
			t.Fatalf("fail-open shape lost its caliber legend entry %q:\n%s", want, skewedLegend)
		}
	}
}

// --- 复核 FAIL-2: the conclusion equation is the SHARED template, pinned
// --- inside the **主根因:** line itself (行域限定 — a forked private copy
// --- with a drifted joiner can no longer hide in whole-page contains) --------

func TestRCRConclusionEquationSharedTemplateLineScoped(t *testing.T) {
	projection := rcrInversionSuppressedClauseProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	var conclusion string
	for _, line := range strings.Split(lead, "\n") {
		if strings.Contains(line, "**主根因:**") {
			conclusion = line
			break
		}
	}
	if conclusion == "" {
		t.Fatalf("no conclusion line rendered:\n%s", lead)
	}
	components, total, ok := runtimeTraceProjInversionComponents(projection.PrimaryRootCauses[0], true)
	if !ok {
		t.Fatalf("fixture components must balance")
	}
	// The line carries the helper's output VERBATIM — the equation has one
	// template (runtimeTraceProjAttributionEquation), three consumers.
	if want := "有效归因 " + runtimeTraceProjAttributionEquation(total, components); !strings.Contains(conclusion, want) {
		t.Fatalf("conclusion line must embed the shared equation template %q:\n%s", want, conclusion)
	}
	if !strings.Contains(conclusion, "有效归因 5.000ms = runnable(全额) 2.000ms + running(折算) 3.000ms") {
		t.Fatalf("conclusion equation drifted from the ruled form:\n%s", conclusion)
	}
}

// --- 复核 F4: the §24.1 degeneration gate, positive and negative arms --------

func TestRCRInversionSingleFullComponentDegenerates(t *testing.T) {
	single := types.TraceCausalProjectionNode{
		Subject: "w-1", Object: "priority_inversion_candidate", StateKind: "running",
		PriorityInversionCandidate: true, ChainRelevance: "on_chain",
		ImpactMS: 0.500, EffectiveImpactMS: 0.277,
		GatedRunnableMS: 0.277, GatedRunningDeficitMS: 0, Rank: 3, Confidence: 0.9,
	}
	row := runtimeTraceProjTreeRow{Node: single, Kind: runtimeTraceProjTreeRowChain,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	structured, ok := runtimeTraceProjCauseStructuredParts(row, true)
	if !ok {
		t.Fatalf("cause node must build")
	}
	// Positive arm: two-line form — 行3 folded into 行2's tail, no sub-rows.
	if structured.Breakdown != "" || len(structured.SubRows) != 0 {
		t.Fatalf("single-full composite must degenerate (§24.1 允许退化读作强制): %+v", structured)
	}
	if !strings.Contains(structured.IdentityRow, "·有效归因 0.277ms(全额)") {
		t.Fatalf("行2 tail must carry the degenerate value: %q", structured.IdentityRow)
	}
	// Negative arm 1: two live components keep the four-line form.
	twoComp := single
	twoComp.GatedRunningDeficitMS = 0.100
	twoComp.EffectiveImpactMS = 0.377
	row2 := runtimeTraceProjTreeRow{Node: twoComp, Kind: runtimeTraceProjTreeRowChain,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	if structured, ok := runtimeTraceProjCauseStructuredParts(row2, true); !ok ||
		structured.Breakdown == "" || len(structured.SubRows) != 2 {
		t.Fatalf("two components must keep the four-line form: %+v", structured)
	}
	// Negative arm 2: a single DISCOUNTED component (计入≠原始) keeps 行3 +
	// its sub-row — degeneration is strictly the 计入==原始 shape.
	discounted := single
	discounted.GatedRunnableMS = 0
	discounted.GatedRunningDeficitMS = 0.277
	row3 := runtimeTraceProjTreeRow{Node: discounted, Kind: runtimeTraceProjTreeRowChain,
		HasData: true, marks: &runtimeTraceProjMarkSet{}}
	if structured, ok := runtimeTraceProjCauseStructuredParts(row3, true); !ok ||
		structured.Breakdown == "" || len(structured.SubRows) != 1 {
		t.Fatalf("a discounted single component must keep the 行3 form: %+v", structured)
	}
}

// --- E# numbering sanity for the fixture (keeps the E4-E8 verbatim pins
// --- anchored on the specimen's own numbering) --------------------------------

func TestRCROpendirFixtureEvidenceNumbering(t *testing.T) {
	fence, _ := rcrOpendirFence(t, true)
	for _, want := range []string{"[E4]", "[E5]", "[E6]", "[E7(+1)+E8]"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("fixture numbering drifted (want %q):\n%s", want, fence)
		}
	}
	if !strings.Contains(fence, fmt.Sprintf("满格=窗口%.3fms", 120.0)) {
		t.Fatalf("fixture window drifted:\n%s", fence)
	}
}
