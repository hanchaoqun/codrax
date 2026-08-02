package tool

// answer_document_projection_disphyg3_test.go — DISPHYG-3 批 pins
// (§29.150⑩⑪ user rulings + §29.155 P2 / FREQDIR 冷读 P3-1 / CALSIDE 备案 /
// §29.158 P3-2 / CLUSTER-FIX-2 D5 filings, 2026-07-20).
//
//	件1 C8 标点分制: system-minted non-bullet prose sentences carry FULL-WIDTH
//	     top-level clause marks (，／；) with 。; fence bodies stay half-width
//	     and never print full-width sentence punctuation. Per-region positive
//	     arm + the same-sentence mixing ban arm (paren-depth-0 scan).
//	件2 C4 折叠边词: stanza fold rows wear the tree-connector edge form
//	     (├─邻近─/├─背景─ — the ├─链上─ family); bare edge words are banned.
//	件3 CALSIDE P2 残形: a windowless board without a wall-clock scale anchor
//	     speaks the honest no-ruler clause — no 满格= claim, no fake
//	     本报告最大X.XXXms unit claim; anchored boards keep the legacy ruler.
//	件5 E29/E30 语序: the ⌗ caliber-side word follows the row's category word
//	     on 行2 (majority order — the grammar row / guarantee / ◎ footnote).
//	件6 RULER2 紧凑面: the two-ruler sentence's non-lead ordinal-less
//	     participant row wears the 根因排序#N cross-reference chip
//	     (typed unique host match; ranked rows and ambiguity stay silent).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// disphyg3DepthZeroHalfClauseMarks reports the half-width `,`/`;` occurrences
// at parenthesis depth 0 of a prose sentence (the C8 mixing-ban probe: the
// witnessed sin 「,共 233.190ms。」 is a half-width top-level clause mark in a
// 。-terminated sentence; parenthetical interiors legitimately keep the
// fence-shared half-width faces).
func disphyg3DepthZeroHalfClauseMarks(line string) int {
	depth, hits := 0, 0
	for _, r := range line {
		switch r {
		case '(', '（':
			depth++
		case ')', '）':
			if depth > 0 {
				depth--
			}
		case ',', ';':
			if depth == 0 {
				hits++
			}
		}
	}
	return hits
}

// 件1 P-arm: the zh window line speaks the full-width top-level comma and the
// legacy mixing bytes are gone; the whole lead prose sentence family scans
// clean at paren depth 0.
func TestDisphyg3C8WindowLineProseRegime(t *testing.T) {
	projection := types.TraceCausalProjection{WindowStartTs: 5.0, WindowEndTs: 5.007}
	model := runtimeTraceProjTreeModel{WindowMS: 7.0}
	line := runtimeTraceProjWindowLine(projection, model, true)
	first := strings.Split(line, "\n")[0]
	if !strings.HasPrefix(first, "分析窗 5.000~5.007s，共 7.000ms。") {
		t.Fatalf("the zh window line must carry the full-width clause comma: %q", first)
	}
	if strings.Contains(first, "s,共") {
		t.Fatalf("the C8 witness bytes (half comma before 共) must be gone: %q", first)
	}
	if disphyg3DepthZeroHalfClauseMarks(first) != 0 {
		t.Fatalf("the window sentence must carry no depth-0 half-width clause mark: %q", first)
	}
	// EN face untouched (half-width is native).
	if en := strings.Split(runtimeTraceProjWindowLine(projection, model, false), "\n")[0]; !strings.HasPrefix(en, "Analysis window 5.000~5.007s, 7.000ms total.") {
		t.Fatalf("the EN window line must keep its native form: %q", en)
	}
}

// 件1 P-arm (conclusion family): every zh conclusion-line template joint is
// full-width at depth 0 — the no-primary pointer form and the semantic-lead
// fixed form scan clean.
func TestDisphyg3C8ConclusionSentencesScanClean(t *testing.T) {
	// The demoted-primary shape (leadsem L3 fixture family) drives the
	// no-primary pointer branch.
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "agg",
			Subject: "unknown-thread", Predicate: "root_cause_primary",
			Object: "supply_pressure", SubjectKind: "aggregate_metric",
			Rank: 1, ChainRelevance: "on_chain", ImpactMS: 100, Confidence: 0.4,
		}},
	}
	withBackground := runtimeTraceProjTreeModel{Background: []runtimeTraceProjTreeRow{{
		Node: types.TraceCausalProjectionNode{Subject: "bg-1", ImpactMS: 5}, Kind: runtimeTraceProjTreeRowBackground, HasData: true,
	}}}
	line := runtimeTraceProjConclusionLine(projection, withBackground, true)
	if line != "**主根因:** 窗口内未定位到链上主根因，见背景压力段。" {
		t.Fatalf("the no-primary pointer form must join with the full-width comma: %q", line)
	}
	if disphyg3DepthZeroHalfClauseMarks(line) != 0 {
		t.Fatalf("no depth-0 half-width clause mark on the conclusion sentence: %q", line)
	}
	node := types.TraceCausalProjectionNode{Subject: "t-1", Object: "jit_compile",
		SpanName: "JIT compiling", ImpactMS: 83.893}
	semantic := runtimeTraceProjSemanticLeadText(node, runtimeTraceProjTreeModel{WindowMS: 101}, true)
	if !strings.HasPrefix(semantic, "未定位到链上主根因；") {
		t.Fatalf("the semantic lead's top-level semicolon must be full-width: %q", semantic)
	}
	if disphyg3DepthZeroHalfClauseMarks(semantic) != 0 {
		t.Fatalf("no depth-0 half-width clause mark on the semantic lead: %q", semantic)
	}
}

// 件1 F-arm: fence bodies stay half-width — the rendered tree fence carries
// no full-width sentence punctuation (，。；). The user-ruled PTV7 alias
// parens （） are word faces outside the C8 class and stay untouched.
func TestDisphyg3C8FenceBodyStaysHalfWidth(t *testing.T) {
	for _, projection := range []types.TraceCausalProjection{
		uxr1FourOneSixFiveProjection(),
		ruler2TwoRulerProjection(),
	} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
		fence := runtimeTraceProjTreeFence(model, true)
		for _, banned := range []string{"，", "。", "；"} {
			if strings.Contains(fence, banned) {
				t.Fatalf("the fence body must stay half-width (found %q):\n%s", banned, fence)
			}
		}
	}
}

// 件2 C4: stanza fold rows wear the tree-connector edge form on both language
// faces; sibling non-fold stanza rows keep the bare glyph slot (no edge).
func TestDisphyg3C4StanzaFoldEdgeWearsTreeConnector(t *testing.T) {
	fold := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{MergedCount: 5, MergedSubjects: []string{"a-1", "b-2"}},
		Kind: runtimeTraceProjTreeRowBackground, HasData: true,
	}
	if got := runtimeTraceProjStanzaRowFixed(fold, nil, true); !strings.HasPrefix(got, "    ├─背景─ ") {
		t.Fatalf("the background fold edge must wear the tree connector: %q", got)
	}
	if got := runtimeTraceProjStanzaRowFixed(fold, nil, false); !strings.HasPrefix(got, "    ├─background─ ") {
		t.Fatalf("the EN background fold edge must wear the tree connector: %q", got)
	}
	fold.Kind = runtimeTraceProjTreeRowAdjacent
	if got := runtimeTraceProjStanzaRowFixed(fold, nil, true); !strings.HasPrefix(got, "    ├─邻近─ ") {
		t.Fatalf("the adjacent fold edge must wear the tree connector: %q", got)
	}
	if got := runtimeTraceProjStanzaRowFixed(fold, nil, false); !strings.HasPrefix(got, "    ├─adjacent─ ") {
		t.Fatalf("the EN adjacent fold edge must wear the tree connector: %q", got)
	}
	// Ban arm: the bare edge form (no connector) never renders again.
	for _, kind := range []string{runtimeTraceProjTreeRowBackground, runtimeTraceProjTreeRowAdjacent} {
		fold.Kind = kind
		for _, zh := range []bool{true, false} {
			got := runtimeTraceProjStanzaRowFixed(fold, nil, zh)
			for _, bare := range []string{"    背景─", "    邻近─", "    background─", "    adjacent─"} {
				if strings.HasPrefix(got, bare) {
					t.Fatalf("the bare edge form must be gone (%s zh=%v): %q", kind, zh, got)
				}
			}
		}
	}
	// Non-fold stanza rows keep the edgeless slot byte-identically.
	plain := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{Subject: "w-1", ImpactMS: 2},
		Kind: runtimeTraceProjTreeRowBackground, HasData: true,
	}
	if got := runtimeTraceProjStanzaRowFixed(plain, nil, true); strings.Contains(got, "─") {
		t.Fatalf("non-fold stanza rows carry no edge: %q", got)
	}
}

// 件3: the windowless scale sentences fork on the typed wall-clock-anchor
// signal — honest no-ruler wording on the un-anchored shape (zero fake ms,
// zero 满格= claim), legacy bytes on anchored/mixed and windowed shapes.
func TestDisphyg3NoRulerScaleNoteForks(t *testing.T) {
	unanchored := runtimeTraceProjTreeModel{BarMaxMS: 81.616, BarScaleWallClockAnchored: false}
	got := runtimeTraceProjScaleNote(unanchored, true)
	if got != "窗口起止未采集·本板值均非墙钟·不设占比标尺(不显示占窗%)" {
		t.Fatalf("the un-anchored windowless head must speak the honest no-ruler clause: %q", got)
	}
	if strings.Contains(got, tracefence.ScaleMarkZH) || strings.Contains(got, "本报告最大") {
		t.Fatalf("the un-anchored head must mint no ruler claim: %q", got)
	}
	if en := runtimeTraceProjScaleNote(unanchored, false); en != "window bounds not captured; all board values are non-wall-clock · no bar ruler (no window %)" {
		t.Fatalf("the EN un-anchored head must fork too: %q", en)
	}
	// Degenerate zero-value board: no value claim, still no ruler.
	empty := runtimeTraceProjTreeModel{}
	if got := runtimeTraceProjScaleNote(empty, true); got != "窗口起止未采集·本板无持值行·不设占比标尺(不显示占窗%)" {
		t.Fatalf("the zero-value windowless head drops the value claim: %q", got)
	}
	// 混合板照旧: an anchored windowless board keeps the legacy ruler bytes.
	anchored := runtimeTraceProjTreeModel{BarMaxMS: 117.928, BarScaleWallClockAnchored: true}
	if got := runtimeTraceProjScaleNote(anchored, true); got != "窗口起止未采集·满格=本报告最大117.928ms(回退尺度,不显示占窗%)" {
		t.Fatalf("the anchored windowless head must keep the legacy ruler: %q", got)
	}
	// Windowed boards untouched.
	windowed := runtimeTraceProjTreeModel{WindowMS: 120}
	if got := runtimeTraceProjScaleNote(windowed, true); got != "满格=窗口120.000ms" {
		t.Fatalf("the windowed head is untouched: %q", got)
	}
	// The no-ruler stem is a registered scale-note marker (the preview
	// archive fallback derives from the same tracefence constants).
	markers := strings.Join(tracefence.ScaleNoteMarkers(), "|")
	if !strings.Contains(markers, tracefence.NoRulerMarkZH) || !strings.Contains(markers, tracefence.NoRulerMarkEN) {
		t.Fatalf("the no-ruler stems must join the closed marker set: %q", markers)
	}
}

// 件3 (lead prose twin): the windowless lead window-line forks on the same
// typed signal; the anchored shape keeps its legacy wording (件1 comma only).
func TestDisphyg3NoRulerWindowLineForks(t *testing.T) {
	projection := types.TraceCausalProjection{}
	unanchored := runtimeTraceProjTreeModel{BarMaxMS: 81.616}
	got := runtimeTraceProjWindowLine(projection, unanchored, true)
	if got != "分析窗起止未采集: 本板值均非墙钟·不设占比标尺，不显示占窗百分比、不画时长条(系统不估算窗口)。" {
		t.Fatalf("the un-anchored lead line must speak the honest no-ruler sentence: %q", got)
	}
	if strings.Contains(got, "本报告最大") || strings.Contains(got, tracefence.ScaleMarkZH) {
		t.Fatalf("the un-anchored lead line must mint no ruler claim: %q", got)
	}
	anchored := runtimeTraceProjTreeModel{BarMaxMS: 117.928, BarScaleWallClockAnchored: true}
	if got := runtimeTraceProjWindowLine(projection, anchored, true); got != "分析窗起止未采集: 不显示占窗百分比，树内时长条满格=本报告最大投影(回退尺度,系统不估算窗口)。" {
		t.Fatalf("the anchored lead line keeps the legacy wording (C8 comma only): %q", got)
	}
	if en := runtimeTraceProjWindowLine(projection, unanchored, false); !strings.Contains(en, "no bar ruler") || strings.Contains(en, "largest projection") {
		t.Fatalf("the EN un-anchored lead line must fork too: %q", en)
	}
}

// 件5: the ⌗ caliber-side word follows the row's CATEGORY word on 行2 — the
// unified majority order (category first, ⌗ qualifier after).
func TestDisphyg3CaliberSideWordFollowsCategoryWord(t *testing.T) {
	row := runtimeTraceProjTreeRow{
		Node: types.TraceCausalProjectionNode{
			Subject: "ThreadPoolForeg-60555", Object: "page_cache_churn",
			TypeToken: "page_cache_churn", Tier: types.TraceCausalTierCaliberSide,
			ImpactMS: 7.2, EffectiveImpactMS: 7.2,
		},
		Kind: runtimeTraceProjTreeRowAdjacent, HasData: true,
		marks: &runtimeTraceProjMarkSet{},
	}
	line := runtimeTraceProjRowLineWithMetrics("    ⌗ ThreadPoolForeg-60555 · 页缓存抖动", row, 114.940, true, true)
	category := strings.Index(line, "页缓存抖动 · ⌗口径旁栏·计数当量(非墙钟,不占序数)")
	if category < 0 {
		t.Fatalf("行2 must speak category-then-⌗ in the unified order:\n%s", line)
	}
	// Ban arm: the old ⌗-first order is gone.
	if strings.Contains(line, "⌗口径旁栏·计数当量(非墙钟,不占序数) · 页缓存抖动") {
		t.Fatalf("the retired ⌗-first order must not render:\n%s", line)
	}
}

// 件6: the two-ruler participant chip — an ordinal-less compact participant
// row wears 根因排序#N; ranked participants and ambiguous hosts stay silent.
func TestDisphyg3TwoRulerParticipantChip(t *testing.T) {
	// Stamp-level positive arm on a hand-built model (the donghu E5 compact
	// shape: an ordinal-less merged state row absorbed the #5 wall seat; it
	// keeps its OWN causality token — the stamp deliberately ignores it).
	record := ruler2TwoRulerProjection().SelfRunnableTwoRulerAccountings[0]
	lead := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		Subject: "app-100", StateKind: "runnable", ChainRelevance: "on_chain",
		Causality: "self_wall_clock", Rank: 2, EffectiveImpactMS: 3.0}, HasData: true}
	compact := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		Subject: "app-100", StateKind: "runnable", ChainRelevance: "on_chain",
		Causality: "on_wakeup_chain", Rank: 0, EffectiveImpactMS: 1.0}, HasData: true}
	edge := runtimeTraceProjTreeRow{Node: types.TraceCausalProjectionNode{
		Subject: "app-100", StateKind: "runnable", ChainRelevance: "on_chain",
		Causality: "on_wakeup_chain", Rank: 4, EffectiveImpactMS: 2.0}, HasData: true}
	model := runtimeTraceProjTreeModel{
		SelfRows:                        []runtimeTraceProjTreeRow{lead, compact, edge},
		SelfRunnableTwoRulerAccountings: []types.TraceCausalProjectionSelfRunnableTwoRuler{record},
	}
	runtimeTraceProjStampSelfRunnableTwoRuler(&model)
	if model.SelfRows[1].SelfTwoRulerParticipantRank != 5 {
		t.Fatalf("the ordinal-less compact participant must take the #5 stamp: %+v", model.SelfRows[1].SelfTwoRulerParticipantRank)
	}
	if model.SelfRows[0].SelfTwoRulerParticipantRank != 0 || model.SelfRows[2].SelfTwoRulerParticipantRank != 0 {
		t.Fatalf("the lead row and ranked participants never take the chip stamp")
	}
	if chip := runtimeTraceProjSelfTwoRulerParticipantChip(model.SelfRows[1], true); chip != "根因排序#5" {
		t.Fatalf("the zh chip must be the channel-worded board ordinal (禁裸 #N): %q", chip)
	}
	if chip := runtimeTraceProjSelfTwoRulerParticipantChip(model.SelfRows[1], false); chip != "root-cause rank #5" {
		t.Fatalf("the EN chip must be channel-worded too: %q", chip)
	}
	// The compact self-row face renders the chip on its tail.
	lines := runtimeTraceProjSelfRowLines(model.SelfRows[1], 10.0, true)
	if len(lines) == 0 || !strings.Contains(strings.Join(lines, "\n"), "根因排序#5") {
		t.Fatalf("the compact face must wear the chip:\n%s", strings.Join(lines, "\n"))
	}
	// The legend teaches the chip through the two-ruler entry (production
	// fixture path — the sentence renders, the entry follows the mark).
	full := buildRuntimeTraceProjTreeModel(ruler2TwoRulerProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if fence := runtimeTraceProjTreeFence(full, true); fence == "" {
		t.Fatalf("the fixture must render")
	}
	legend := strings.Join(runtimeTraceProjLegendGroupLines(full.Marks, true), "\n")
	if !strings.Contains(legend, "根因排序#N 对照记号") {
		t.Fatalf("the two-ruler legend entry must teach the participant chip:\n%s", legend)
	}
	// Negative 1: ranked participants (own 行2 ordinal) never double-chip —
	// the pristine fixture renders no chip beyond the rows' own identity
	// lines (stamp requires Rank==0).
	pristine := buildRuntimeTraceProjTreeModel(ruler2TwoRulerProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	prow := disphyg3FindParticipantRow(pristine, 1.0)
	if prow != nil && prow.SelfTwoRulerParticipantRank != 0 {
		t.Fatalf("a ranked participant row must not take the chip stamp")
	}
	// Negative 2: ambiguity stamps nothing (two µs-equal ordinal-less rows
	// at the stamp level — the production build would dedupe true twins, so
	// the arm exercises the stamp's own uniqueness gate directly).
	compactTwin := compact
	ambModel := runtimeTraceProjTreeModel{
		SelfRows:                        []runtimeTraceProjTreeRow{lead, compact, compactTwin, edge},
		SelfRunnableTwoRulerAccountings: []types.TraceCausalProjectionSelfRunnableTwoRuler{record},
	}
	runtimeTraceProjStampSelfRunnableTwoRuler(&ambModel)
	for i := range ambModel.SelfRows {
		if ambModel.SelfRows[i].SelfTwoRulerParticipantRank != 0 {
			t.Fatalf("an ambiguous host match must stamp nothing (宁漏勿假指)")
		}
	}
}

func disphyg3FindParticipantRow(model runtimeTraceProjTreeModel, eff float64) *runtimeTraceProjTreeRow {
	for _, rows := range [][]runtimeTraceProjTreeRow{model.TreeRows, model.SelfRows, model.Adjacent, model.Background} {
		for i := range rows {
			if rows[i].Node.EffectiveImpactMS == eff {
				return &rows[i]
			}
		}
	}
	return nil
}

// TestDisphyg3ProseClauseRegimeSupplyFold — 收编 P2-1 (2026-07-20): the
// fence-shared supply-fold clause was the last same-sentence mixed-mark
// flagship line (donghu_2955 conclusion: half-width ,为主 and ;窗内 beside
// the lead's full-width ，). The prose regime transform swaps depth-0 ,/; to
// ，/； while parenthetical interiors (fence-shared tokens) keep half-width;
// the fence 行2 face keeps legacy bytes by construction (raw clause
// untouched).
func TestDisphyg3ProseClauseRegimeSupplyFold(t *testing.T) {
	raw := "供给折算缺口 65.912ms(运行频点非最高,按全域最高频折算,下界)为主,running 时间含降频/小核导致的跑慢成分;窗内该簇受热限压至 1.53GHz"
	prose := runtimeTraceProjProseClauseRegime(raw)
	want := "供给折算缺口 65.912ms(运行频点非最高,按全域最高频折算,下界)为主，running 时间含降频/小核导致的跑慢成分；窗内该簇受热限压至 1.53GHz"
	if prose != want {
		t.Fatalf("prose regime transform wrong:\n got %q\nwant %q", prose, want)
	}
	for _, banned := range []string{")为主,", "成分;"} {
		if strings.Contains(prose, banned) {
			t.Fatalf("depth-0 half-width mark survived the prose regime: %q in %q", banned, prose)
		}
	}
	if !strings.Contains(prose, "(运行频点非最高,按全域最高频折算,下界)") {
		t.Fatalf("parenthetical interior must keep the fence half-width face, got %q", prose)
	}
	if runtimeTraceProjProseClauseRegime("(a,b);c") != "(a,b)；c" {
		t.Fatalf("nested depth accounting broken")
	}

	// Lead-embedding tooth (突变课: the helper pin alone left the :14128
	// call site mutable — a bypass mutant survived both prior arms): drive
	// the PRODUCTION conclusion builder with a supply-fold deficit-dominant
	// primary and assert the rendered sentence carries zero depth-0
	// half-width clause marks while the clause's parenthetical keeps its
	// fence half-width interior.
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		PrimaryRootCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E1",
			Subject: "ui-thread-2955", Predicate: "root_cause_primary",
			Object: "running", SubjectKind: "thread",
			Rank: 1, ChainRelevance: "on_chain",
			ImpactMS: 65.912, EffectiveImpactMS: 65.912, Confidence: 0.9,
			SupplyFoldComputed: true, SupplyFoldKnownMS: 75.912,
			SupplyFoldDeficitMS: 65.912, SupplyFoldIdealMS: 10.0,
			SupplyFoldCapabilitySource: "freq_only",
			GovernanceCapKHz:           1530000,
			GovernanceCapMechanism:     tracequery.SupplyFoldGovernanceCapThermalRail,
			GovernanceCapWitnessed:     true,
		}},
	}
	lead := runtimeTraceProjConclusionLine(projection, runtimeTraceProjTreeModel{WindowMS: 233.190}, true)
	if !strings.Contains(lead, "供给折算缺口") {
		t.Fatalf("fixture must exercise the supply-fold clause on the lead, got %q", lead)
	}
	if n := disphyg3DepthZeroHalfClauseMarks(lead); n != 0 {
		t.Fatalf("the supply-fold lead sentence must carry zero depth-0 half-width clause marks, got %d in %q", n, lead)
	}
}
