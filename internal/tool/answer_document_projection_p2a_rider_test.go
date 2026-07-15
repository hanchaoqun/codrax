package tool

// answer_document_projection_p2a_rider_test.go — P2a 显示 rider 五件套 pins
// (ledger docs/design/real_trace_campaign_20260705.md §29.55.3 处置更新 /
// §29.58.1 / §29.58.2, user rulings 2026-07-13):
//
//	件1  fold rows: 边词管车道 + 行名管折叠 + 记号位留形态族 (pinned in the
//	     PTS/PTV5/disp2/custom1g/b6 fold tests; this file adds the negative
//	     old-form tripwire).
//	件2a "· " side-note block one level deeper than the host self row
//	     (ptv6d indent pin) + the G11 stanza-level note (here).
//	件2b component rows reseated under their owning seat with the ↳
//	     connector (here).
//	件2c ∿ cadence-idle verified DISJOINT from the sleep seat's account
//	     (engine ENG-2 same-fact fold consumes the segment's sleep record)
//	     → stays a sibling, never ↳ (negative arm here).
//	件3  binder ⋈ glyph split + Mark split (F1 图例修真 — a binder-only
//	     report no longer lights the ◦ 数据行 entry) (here).
//	件4  optimization-table member/fold cells wear ↳ (rcm2 C4 pin).

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// p2aSelfCarveProjection is the production self-stanza shape (witness
// 20260713-062104 family): sleep seat + binder carve rows + a cadence-idle
// row + a drill hop so the tree renders.
func p2aSelfCarveProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"app-9511", ".ugc.aweme.lite-17267"},
		WindowStartTs: 13762.791708,
		WindowEndTs:   13763.024898,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-sleep",
				Subject: ".ugc.aweme.lite-17267", Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 35.351, CumulativeImpactMS: 35.351,
				Confidence: 0.9, LineStart: 100, LineEnd: 200},
			// Cadence idle: rides the s_sleep StateKind but is semantically its
			// own lane (件2c: account DISJOINT from the sleep seat — sibling).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-idle",
				Subject: ".ugc.aweme.lite-17267", Object: "pacing_idle", TypeToken: "pacing_idle",
				StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 15.758, CumulativeImpactMS: 15.758,
				Confidence: 0.85, LineStart: 210, LineEnd: 240},
			// The binder carve (WO-A1 carrier a).
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "self-binder-1",
				Subject: ".ugc.aweme.lite-17267", Object: "binder:496_9-10961", TypeToken: "binder_wait",
				ChainRelevance: "on_chain", ImpactMS: 1.409, CumulativeImpactMS: 1.409,
				Confidence: 0.8, LineStart: 110, LineEnd: 150},
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop-1",
				Subject: "app-9511", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 15.565, CumulativeImpactMS: 15.565,
				Confidence: 0.8, LineStart: 300, LineEnd: 320},
		},
	}
}

// 件2b: the binder component row reseats DIRECTLY under the sleep seat (it
// used to sort by magnitude behind the ∿ row) and wears the ↳ connector; the
// ∿ cadence-idle row stays a sibling (件2c verdict) and never wears ↳.
func TestP2aSelfComponentReseatWearsSubordinateConnector(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	if len(model.SelfRows) < 3 {
		t.Fatalf("fixture drifted: want sleep+idle+binder self rows, got %d", len(model.SelfRows))
	}
	// Typed order: sleep seat first, its ↳ component directly after, the ∿
	// sibling behind them (pre-reseat magnitude order was sleep, idle, binder).
	if !model.SelfRows[0].Node.IsSleepState() || runtimeTraceProjIdleRowKind(model.SelfRows[0].Node) != "" {
		t.Fatalf("row 0 must be the sleep seat: %+v", model.SelfRows[0].Node)
	}
	if got := runtimeTraceCausalProjectionCanonicalNode(model.SelfRows[1].Node.TypeToken); got != "binder_wait" {
		t.Fatalf("row 1 must be the reseated binder component, got %q", got)
	}
	if !model.SelfRows[1].SubordinateComponentSeat {
		t.Fatalf("the reseated component row must carry the ↳ seat stamp: %+v", model.SelfRows[1])
	}
	if runtimeTraceProjIdleRowKind(model.SelfRows[2].Node) == "" {
		t.Fatalf("row 2 must be the ∿ sibling: %+v", model.SelfRows[2].Node)
	}
	if model.SelfRows[2].SubordinateComponentSeat {
		t.Fatalf("件2c: the cadence-idle row is a DISJOINT sibling — it must never take the ↳ treatment")
	}
	fence := runtimeTraceProjTreeFence(model, true)
	lines := strings.Split(fence, "\n")
	sleepAt, binderAt, idleAt := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "│     ☾ 自身·sleep"):
			sleepAt = i
		case strings.Contains(line, "自身·binder"):
			binderAt = i
		case strings.Contains(line, "帧间空闲"):
			idleAt = i
		}
	}
	if sleepAt < 0 || binderAt != sleepAt+1 || idleAt <= binderAt {
		t.Fatalf("component row must render directly under its owning seat (sleep=%d binder=%d idle=%d):\n%s",
			sleepAt, binderAt, idleAt, fence)
	}
	// §29.58.5 ① (user 精化裁定, 2026-07-13) — EVOLUTION RECORD: the component
	// row now indents ONE LEVEL DEEPER than its owning seat (lead + 2 cells),
	// the ↳ connector falls into the indent position and the row wears the
	// SINGLE form mark ⋈ (the pre-ruling form put ↳ at the host mark column,
	// where the two 2ch envelopes read as a double icon; 133136 witness). The
	// ∿ sibling keeps the bare host-level lead.
	if !strings.HasPrefix(lines[binderAt], "│       "+tracefence.GlyphSubordinate+" "+tracefence.GlyphBinderWait+" 自身·binder") {
		t.Fatalf("component row must indent one level deeper and lead ↳ ⋈: %q", lines[binderAt])
	}
	if !strings.HasPrefix(lines[sleepAt], "│     ☾") {
		t.Fatalf("the owning seat keeps the host-level lead: %q", lines[sleepAt])
	}
	if strings.Contains(lines[idleAt], tracefence.GlyphSubordinate) {
		t.Fatalf("∿ sibling must not wear ↳: %q", lines[idleAt])
	}
	// The containment word face stays (词面管语义).
	if !strings.Contains(fence, "不可相加·为[") {
		t.Fatalf("the component pointer word must survive the reseat:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkSubordinateComponent) {
		t.Fatalf("the ↳ legend mark must record at the emission site")
	}
}

// 件3 F1 图例修真: a binder-carve report's rows wear ⋈/IconBinderWait — the
// ◦ 数据行 entry (IconNoDominant) no longer lights on a binder-only shape
// (062916 witness form: the legend claimed 「有形态词的行戴各自形态族记号」
// while the binder row wore ◦ and lit the 数据行 entry).
func TestP2aBinderGlyphSplitStopsLightingNoDominantEntry(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, tracefence.GlyphBinderWait+" 自身·binder") {
		t.Fatalf("binder rows must wear the dedicated ⋈ glyph:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkIconBinderWait) {
		t.Fatalf("the ⋈ legend mark must record at the icon emission site")
	}
	if model.Marks.has(runtimeTraceProjMarkIconNoDominant) {
		t.Fatalf("F1 修真: this shape carries no genuinely word-less data row — the ◦ 数据行 entry must not light")
	}
	// The generated ⋈ legend entry exists in the catalog (form-table single
	// source) and speaks the IPC-wait semantics.
	found := false
	for _, entry := range runtimeTraceProjLegendCatalog() {
		if entry.Mark != runtimeTraceProjMarkIconBinderWait {
			continue
		}
		found = true
		if !strings.Contains(entry.ZH, tracefence.GlyphBinderWait) || !strings.Contains(entry.ZH, "binder IPC 等待") {
			t.Fatalf("⋈ legend entry drifted: %q", entry.ZH)
		}
	}
	if !found {
		t.Fatalf("the ⋈ mark must own a generated legend entry")
	}
}

// 件2a F3: the G11 stanza-level side-note rides the one-level-deeper note
// geometry (its mint point is independent of the per-row demoted stream).
func TestP2aG11StanzaNoteIndentsOneLevelDeeper(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	model.SelfWaitOverflowCount = 2
	model.SelfWaitOverflowMaxMS = 0.4
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "\n│       · 另有 2 条自身等待症状行(单条最大 0.400ms)") {
		t.Fatalf("G11 note must sit one level deeper than the self rows:\n%s", fence)
	}
	enFence := runtimeTraceProjTreeFence(model, false)
	if !strings.Contains(enFence, "\n│       · 2 more self wait-symptom rows") {
		t.Fatalf("EN G11 note must ride the same geometry:\n%s", enFence)
	}
}

// pin1FoldPressureProjection (PIN-1 B1, §29.65, 2026-07-13) is the
// production width-pressure shape: a published rank seat whose subject is a
// LONG worker name (donghu OS_FFRT_* family form) plus an on-chain overflow
// fold whose roster HEAD is that same subject — the B6 annotator then mints
// the 见榜位#N pointer suffix onto the fold roster head, and the composed
// fold name overflows the shared label column.
func pin1FoldPressureProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"ranked-10", "target-1"},
		WindowStartTs: 5.0,
		WindowEndTs:   5.1,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "rank-seat-e1",
				Subject: "OS_FFRT_2_2_long_worker_name-43037", Predicate: "root_cause_secondary", Object: "running",
				TypeToken: "running", StateKind: "running", ChainRelevance: "on_chain",
				ChainDepth: 1, Rank: 2, ImpactMS: 20, CumulativeImpactMS: 20,
				EffectiveImpactMS: 20, Confidence: 0.8,
			},
			{
				Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "fold-e2",
				Predicate: "root_cause_context", ChainRelevance: "on_chain",
				OnChainOverflowFold: true, MergedCount: 4,
				MergedSubjects: []string{"OS_FFRT_2_2_long_worker_name-43037", "hidden-11", "hidden-12"},
				MergedMinMS:    1, MergedMaxMS: 3, ImpactMS: 3, CumulativeImpactMS: 3,
				Confidence: 0.6,
			},
		},
	}
}

// R9 (§29.93.2 用户裁定, 2026-07-15) — fold-row line-1 slimming pins.
// EVOLUTION RECORD: the PIN-1 B1 protected-width floor
// (runtimeTraceProjFoldNameProtectedWidth) and its math pin are RETIRED with
// the inline preview itself — line 1 keeps ONLY the bare counted label, so
// the 「计数+头名永不截断」 promise holds by construction: the count stem
// always fits the standard column and the head member + B6 pointer live
// whole on the subordinate line 2.
//
// Pin 1 (行1 标签不超标准列宽): under the SAME width-pressure fixture that
// used to blow the label column, the fold row's line 1 stays the bare label
// and its bar cell aligns with the sibling rank row's (0-cell tolerance).
// Pin 2 (负向, 内联成员名回潮→红): NO member name and NO 榜位 pointer on
// line 1.
// Pin 3 (信息零损只换行): the member preview + pointer render whole on the
// subordinate 「· 成员 …」 line with the counted 见明细 trailer.
func TestP2aFoldRowLine1SlimsAndMemberSinksToLine2(t *testing.T) {
	projection := pin1FoldPressureProjection()

	zhModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	zhFence := runtimeTraceProjTreeFence(zhModel, true)
	foldLine, siblingLine := "", ""
	for _, line := range strings.Split(zhFence, "\n") {
		if strings.Contains(line, "其余 4 项(折叠)") {
			foldLine = line
		}
		if strings.Contains(line, "OS_FFRT_2_2_long…-43037") {
			siblingLine = line
		}
	}
	if foldLine == "" || siblingLine == "" {
		t.Fatalf("width-pressure fixture drifted (fold=%q sibling=%q):\n%s", foldLine, siblingLine, zhFence)
	}
	// Pin 2 — 负向: the inline member preview must never creep back onto
	// line 1 (member name, roster parenthesis or 榜位 pointer ⇒ red).
	for _, banned := range []string{"OS_FFRT", "hidden-1", "见榜位", "(折叠)("} {
		if strings.Contains(foldLine, banned) {
			t.Fatalf("R9 负向 pin: inline member preview crept back onto line 1 (%q):\n%s", banned, foldLine)
		}
	}
	// Pin 1 — bar-grid alignment: the fold row's bar starts at the SAME cell
	// column as the sibling rank row's (label column not blown).
	barCol := func(line string) int {
		i := strings.IndexAny(line, "█░▒")
		if i < 0 {
			return -1
		}
		return runewidth.StringWidth(line[:i])
	}
	if fc, sc := barCol(foldLine), barCol(siblingLine); fc < 0 || fc != sc {
		t.Fatalf("R9 行1 标签不超标准列宽: fold bar column %d must align with sibling %d:\nfold: %s\nsibling: %s", fc, sc, foldLine, siblingLine)
	}
	// Pin 3 — the sink line carries the whole head member + pointer + counted
	// trailer on the subordinate stream.
	if !strings.Contains(zhFence, "· 成员 OS_FFRT_2_2_long_worker_name-43037(见榜位#2) · 其余 3 项见明细") {
		t.Fatalf("R9 行2: member preview + pointer + counted trailer must sink whole:\n%s", zhFence)
	}

	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	enFold := ""
	for _, line := range strings.Split(enFence, "\n") {
		if strings.Contains(line, "4 more (folded)") {
			enFold = line
		}
	}
	if enFold == "" {
		t.Fatalf("EN fold row missing:\n%s", enFence)
	}
	for _, banned := range []string{"OS_FFRT", "see root-cause rank"} {
		if strings.Contains(enFold, banned) {
			t.Fatalf("R9 负向 pin (EN): inline member preview crept back onto line 1 (%q):\n%s", banned, enFold)
		}
	}
	if !strings.Contains(enFence, "· member OS_FFRT_2_2_long_worker_name-43037 (see root-cause rank #2) · 3 more in the detail blocks") {
		t.Fatalf("R9 行2 (EN): member sink line must render whole:\n%s", enFence)
	}
}

// R9 修复轮 P2-3 (对抗官 MUT-D2, 2026-07-15): the chain-fold pin above left
// the OTHER two emission faces unguarded — re-inlining the roster on the
// STANZA mint (背景─/邻近─ = the ◇ 区) or the legacy 合并 mint survived the
// whole suite. Per-face line-1 slimming pins: exact bare-label equality plus
// the explicit negative arms (member name / roster parenthesis — zh 「(折叠)(」
// and EN "(folded)(" bracket forms) so any suffix regrowth reddens here.
func TestR9StanzaAndLegacyFoldLine1StaysBare(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		MergedCount:    5,
		MergedSubjects: []string{"sysevent_store-47924", "b-2"},
	}
	for _, kind := range []string{runtimeTraceProjTreeRowBackground, runtimeTraceProjTreeRowAdjacent} {
		row := runtimeTraceProjTreeRow{Node: node, Kind: kind, HasData: true}
		if got := runtimeTraceProjRowName(row, true); got != "其余 5 项(折叠)" {
			t.Fatalf("stanza fold (%s) line 1 must stay the bare counted label, got %q", kind, got)
		}
		if got := runtimeTraceProjRowName(row, false); got != "5 more (folded)" {
			t.Fatalf("stanza fold (%s, EN) line 1 must stay the bare counted label, got %q", kind, got)
		}
		for _, lang := range []bool{true, false} {
			got := runtimeTraceProjRowName(runtimeTraceProjTreeRow{Node: node, Kind: kind, HasData: true}, lang)
			for _, banned := range []string{"sysevent_store", "(折叠)(", "(folded)(", "(folded) ("} {
				if strings.Contains(got, banned) {
					t.Fatalf("R9 负向 (stanza %s): inline member preview crept back (%q): %q", kind, banned, got)
				}
			}
		}
	}
	// Legacy 合并 face (non-stanza subjectless fold): same negative arms.
	legacy := runtimeTraceProjTreeRow{Node: node, Kind: runtimeTraceProjTreeRowChain, HasData: true}
	for _, lang := range []bool{true, false} {
		got := runtimeTraceProjRowName(legacy, lang)
		for _, banned := range []string{"sysevent_store", "合并(", "folded (", "folded("} {
			if strings.Contains(got, banned) {
				t.Fatalf("R9 负向 (legacy 合并): inline member preview crept back (%q): %q", banned, got)
			}
		}
	}
	if got := runtimeTraceProjRowName(legacy, true); got != "其余 5 项合并" {
		t.Fatalf("legacy fold line 1 must stay the bare counted label, got %q", got)
	}
	if got := runtimeTraceProjRowName(legacy, false); got != "5 more folded" {
		t.Fatalf("legacy fold (EN) line 1 must stay the bare counted label, got %q", got)
	}
}

// 件1 negative tripwire: the retired lane-in-name forms never resurface on
// any face (fence, detail table, lossless block).
func TestP2aRetiredFoldWordFormsStayRetired(t *testing.T) {
	fold := types.TraceCausalProjectionNode{
		ChainRelevance: "on_chain", OnChainOverflowFold: true,
		MergedCount: 3, MergedSubjects: []string{"of-1", "of-2"},
		CumulativeImpactMS: 12.0, Confidence: 0.6,
	}
	for name, got := range map[string]string{
		"tree row zh":  runtimeTraceProjRowName(runtimeTraceProjTreeRow{Node: fold, Kind: runtimeTraceProjTreeRowDepthless, HasData: true}, true),
		"tree row en":  runtimeTraceProjRowName(runtimeTraceProjTreeRow{Node: fold, Kind: runtimeTraceProjTreeRowDepthless, HasData: true}, false),
		"block zh":     runtimeTraceProjDetailFullName(fold, true),
		"block en":     runtimeTraceProjDetailFullName(fold, false),
		"table cell":   runtimeTraceCausalProjectionNodeSubjectCell(fold, true),
		"table cellEN": runtimeTraceCausalProjectionNodeSubjectCell(fold, false),
	} {
		for _, banned := range []string{"链上折叠", "on-chain fold"} {
			if strings.Contains(got, banned) {
				t.Fatalf("%s: retired fold word %q resurfaced: %q", name, banned, got)
			}
		}
		if !strings.Contains(got, "(折叠)") && !strings.Contains(got, "(folded)") {
			t.Fatalf("%s: fold face must carry the dedup stem: %q", name, got)
		}
	}
}

// ---------------------------------------------------------------------------
// PIN-1 DOM 几何机械化 (§29.65 候选 M1/M2/M4, 2026-07-13): the §29.63 DOM
// review verified marker-column alignment (0.00px), side-note depth (+2ch)
// and md↔html fence text identity BY HAND — these sweeps mechanize the same
// three invariants as rune-width string geometry on ENGINE-MINTED fences
// (zero new dependencies; runewidth is already the renderer's own ruler).
// ---------------------------------------------------------------------------

// pin1DomFixtureFences mints the representative production shapes the sweeps
// walk: the width-pressure fold board, the self-carve stanza (with the G11
// overflow note), and the deep opendir engine board.
func pin1DomFixtureFences(t *testing.T, zh bool) map[string]string {
	t.Helper()
	foldModel := buildRuntimeTraceProjTreeModel(pin1FoldPressureProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	selfModel := buildRuntimeTraceProjTreeModel(p2aSelfCarveProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	selfModel.SelfWaitOverflowCount = 2
	selfModel.SelfWaitOverflowMaxMS = 0.4
	opendirFence, _ := rcrOpendirFence(t, zh)
	return map[string]string{
		"fold pressure": runtimeTraceProjTreeFence(foldModel, zh),
		"self carve":    runtimeTraceProjTreeFence(selfModel, zh),
		"opendir":       opendirFence,
	}
}

// pin1EdgeRowPattern matches a tree row's leading rails plus its branch+edge
// segment (e.g. "├─链上─ "): capture 1 = the 2-cell rail units, capture 2 =
// the branch connector through the edge word's trailing space — everything
// left of the marker/badge cell.
var pin1EdgeRowPattern = regexp.MustCompile(`^((?:│ |  )*)([├└]─[^─ ]+─ )`)

// TestPin1FoldMarkerColumnAlignsWithSiblings — DOM M1 (§29.55.3 用户裁定形
// 「链上─ 与兄弟行同列」): on the zh face every edge-bearing row within one
// rail group puts its marker at the SAME display column (the zh edge-word
// closed set 下钻/唤醒/链上/成因/语义/自身/邻近/背景 is uniformly 2 CJK =
// 4 cells wide, so the promise is an exact rune-width equality, 0-cell
// tolerance). The fold row is additionally REQUIRED to be edge-bearing —
// the pre-rider regression form (edge word omitted, marker drifting ~5
// cells left) fails the explicit fold-row arm, not just the group sweep.
// EN edge words vary in width by design (lane words self-report), so the
// column-equality promise is a zh-face invariant.
func TestPin1FoldMarkerColumnAlignsWithSiblings(t *testing.T) {
	for name, fence := range pin1DomFixtureFences(t, true) {
		groups := map[string]int{}
		matched := 0
		for _, line := range strings.Split(fence, "\n") {
			m := pin1EdgeRowPattern.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			matched++
			col := runewidth.StringWidth(m[1]) + runewidth.StringWidth(m[2])
			if prev, ok := groups[m[1]]; ok {
				if prev != col {
					t.Fatalf("%s: marker column drifted within rail group %q (want %d, got %d): %q\n%s",
						name, m[1], prev, col, line, fence)
				}
			} else {
				groups[m[1]] = col
			}
		}
		if matched == 0 {
			t.Fatalf("%s: fixture drifted — no edge-bearing rows matched:\n%s", name, fence)
		}
	}
	// Explicit fold-row arm: the fold row itself must be edge-bearing and sit
	// in the same rail group / column as its rank-row sibling.
	foldModel := buildRuntimeTraceProjTreeModel(pin1FoldPressureProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(foldModel, true)
	var foldCol, siblingCol int
	for _, line := range strings.Split(fence, "\n") {
		m := pin1EdgeRowPattern.FindStringSubmatch(line)
		if m == nil || m[1] != "" {
			continue
		}
		col := runewidth.StringWidth(m[2])
		if strings.Contains(line, "项(折叠)") {
			if !strings.Contains(m[2], "链上─") {
				t.Fatalf("fold row must carry its lane edge word: %q", line)
			}
			foldCol = col
		} else if strings.Contains(line, "唤醒─") {
			siblingCol = col
		}
	}
	if foldCol == 0 || siblingCol == 0 || foldCol != siblingCol {
		t.Fatalf("fold row marker must share the sibling column (fold=%d sibling=%d):\n%s", foldCol, siblingCol, fence)
	}
}

// pin1SweepSideNoteDepth walks one fence and asserts every "· " side-note
// line sits EXACTLY 2 display cells deeper than its host row's structure
// start (the first non-rail cell). Wrapped note continuations (which align
// under the note text, note-dot + 2, per runtimeTraceProjSubordinateLines)
// are recognized positionally and never treated as hosts. Returns the number
// of note lines checked so callers can assert the sweep actually bit.
func pin1SweepSideNoteDepth(t *testing.T, name, fence string) int {
	t.Helper()
	hostStart, lastNoteDot, checked := -1, -1, 0
	for _, line := range strings.Split(fence, "\n") {
		start, lead := -1, rune(0)
		col := 0
		for _, r := range line {
			if r == '│' || r == ' ' {
				col += runewidth.RuneWidth(r)
				continue
			}
			start, lead = col, r
			break
		}
		if start < 0 {
			// Blank / rail-only line: a following "· " line belongs to the
			// next host block, and any wrap stream is over.
			lastNoteDot = -1
			continue
		}
		if lead == '·' {
			if hostStart < 0 {
				t.Fatalf("%s: side note with no host row: %q\n%s", name, line, fence)
			}
			if start != hostStart+2 {
				t.Fatalf("%s: side note must sit exactly 2 cells deeper than its host (host=%d note=%d): %q\n%s",
					name, hostStart, start, line, fence)
			}
			lastNoteDot = start
			checked++
			continue
		}
		if lastNoteDot >= 0 && start == lastNoteDot+2 && lead != '├' && lead != '└' {
			continue // wrapped note continuation — not a host
		}
		lastNoteDot = -1
		hostStart = start
	}
	return checked
}

// TestPin1SideNotesSitTwoCellsDeeperThanHost — DOM M2 (§29.58.1 / P2a 件2a
// 裁定「旁注比宿主深恰 2ch」): the point pins (ptv6d packed lines, G11) are
// promoted to a fence-wide sweep across the representative shapes, both
// languages — every "· " note line on every board obeys the +2 geometry.
func TestPin1SideNotesSitTwoCellsDeeperThanHost(t *testing.T) {
	for _, zh := range []bool{true, false} {
		total := 0
		for name, fence := range pin1DomFixtureFences(t, zh) {
			total += pin1SweepSideNoteDepth(t, name, fence)
		}
		if total < 5 {
			t.Fatalf("zh=%v: sweep checked only %d side notes — fixtures drifted away from the note-bearing shapes", zh, total)
		}
	}
}

// TestPin1P2aShapesHTMLTextContentByteIdentity — DOM M4 (§29.65 候选; UXG-0
// acceptance ③ extended): the md↔html byte-identity pin walks the P2a-era
// shapes too (fold rows with lane edge words, ↳ component rows, ∿ siblings,
// G11 notes, deep 成因 ladders) — the HTML decoration layer may wrap these
// fences in spans/anchors but its <pre> textContent must equal the md fence
// body byte for byte (the UXG-0 head census only exercised head forms).
func TestPin1P2aShapesHTMLTextContentByteIdentity(t *testing.T) {
	for _, zh := range []bool{true, false} {
		for name, fence := range pin1DomFixtureFences(t, zh) {
			html := uxg0RenderFenceHTML(t, fence)
			fenceBody := strings.TrimPrefix(fence, tracefence.Opener+"\n")
			fenceBody = strings.TrimSuffix(fenceBody, "```")
			if got := uxg0PreTextContent(t, html); got != fenceBody {
				t.Fatalf("%s (zh=%v): HTML textContent drifted from fence bytes\n--- fence body ---\n%q\n--- textContent ---\n%q",
					name, zh, fenceBody, got)
			}
		}
	}
}
