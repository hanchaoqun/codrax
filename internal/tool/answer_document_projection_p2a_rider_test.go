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
	// ↳ + ⋈ on the component row; the ∿ sibling keeps the bare lead.
	if !strings.HasPrefix(lines[binderAt], "│     "+tracefence.GlyphSubordinate+" "+tracefence.GlyphBinderWait+" 自身·binder") {
		t.Fatalf("component row must lead ↳ ⋈: %q", lines[binderAt])
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

// PIN-1 B1 (§29.65 回归口, 2026-07-13) — direct floor math pin for
// runtimeTraceProjFoldNameProtectedWidth: the protected prefix is everything
// up to (excluding) the FIRST roster member separator — zh 「、」 or en ", ",
// earliest wins — plus one cell for the truncation ellipsis; a single-member
// roster protects the whole name. Deleting the floor call in
// runtimeTraceProjRowNameFitted is caught by the width-pressure pin below.
func TestP2aFoldNameProtectedWidthMath(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want int
	}{
		// zh separator: cut before 、 — "count(head" is 10 ASCII cells, +1.
		{"zh separator", "count(head、tail)", 11},
		// en separator: cut before ", ".
		{"en separator", "count(head, tail)", 11},
		// earliest separator wins in both directions.
		{"en before zh", "a, b、c", 2},
		{"zh before en", "a、b, c", 2},
		// single member: whole name (16 cells) + 1.
		{"single member", "count(only-one)…", 17},
		// production zh fold head with a B6 pointer suffix INSIDE the head
		// member: the pointer is part of the protected prefix.
		// 其余(4) + " 4 "(3) + 项(2) + "(折叠)"(6) + "("(1)
		// + "mem-000001"(10) + "(见榜位#2)"(10) = 36, +1 = 37.
		{"pointer suffix protected", "其余 4 项(折叠)(mem-000001(见榜位#2)、b)", 37},
	} {
		if got := runtimeTraceProjFoldNameProtectedWidth(tc.in); got != tc.want {
			t.Fatalf("%s: runtimeTraceProjFoldNameProtectedWidth(%q) = %d, want %d", tc.name, tc.in, got, tc.want)
		}
	}
}

// PIN-1 B1 behavior pin (§29.65 / §29.55.3 PTS 「计数+头名永不截断」承诺面):
// under REAL width pressure the fold row's name keeps the count stem plus the
// WHOLE first roster member including its B6 见榜位#N pointer suffix, while
// the width governor proves the pressure on the same fence by (a) mid-cutting
// the very same subject on the sibling rank row and (b) ellipsizing the fold
// roster TAIL. Removing the protected-width floor from
// runtimeTraceProjRowNameFitted reddens the head-member assertions
// (mutation-verified: the head then cuts like the sibling row).
func TestP2aFoldRowHeadNameSurvivesWidthPressure(t *testing.T) {
	projection := pin1FoldPressureProjection()

	zhModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	zhFence := runtimeTraceProjTreeFence(zhModel, true)
	// (1) Head member + pointer whole on the fold row.
	if !strings.Contains(zhFence, "其余 4 项(折叠)(OS_FFRT_2_2_long_worker_name-43037(见榜位#2)") {
		t.Fatalf("fold row must keep count stem + whole head member + pointer under width pressure:\n%s", zhFence)
	}
	// (2) Pressure witness A: the SAME subject on the sibling rank row is
	// mid-truncated by the shared column (proves the budget squeeze is real —
	// without the floor the fold head would cut the same way).
	if !strings.Contains(zhFence, "OS_FFRT_2_2_long…-43037") {
		t.Fatalf("width-pressure fixture drifted: the sibling rank row must show the mid-cut form:\n%s", zhFence)
	}
	// (3) Pressure witness B: the fold roster TAIL is ellipsized right after
	// the protected prefix (the floor protects the head, not the whole roster).
	if !strings.Contains(zhFence, "(见榜位#2)…") {
		t.Fatalf("fold roster tail must ellipsize after the protected head:\n%s", zhFence)
	}

	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjTreeFence(enModel, false)
	if !strings.Contains(enFence, "4 more (folded) (OS_FFRT_2_2_long_worker_name-43037 (see root-cause rank #2)") {
		t.Fatalf("EN fold row must keep the whole head member + pointer:\n%s", enFence)
	}
	if !strings.Contains(enFence, "(see root-cause rank #2)…") {
		t.Fatalf("EN fold roster tail must ellipsize after the protected head:\n%s", enFence)
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
