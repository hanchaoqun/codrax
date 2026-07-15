package tool

// PTV8-LAD pins (§24.11 维度A 施工图 + §24.8 重要度分层总则, huadong_78
// 回访标本 cust_trace_huadong_78.txt:142-180, 2026-07-08).
//
// L1  run-length cycle folding: a consecutive k≤3 tuple repeating ≥2 times
//     inside the long-trunk folded middle renders as ONE "↺ 循环×N: A ⇄ B"
//     row (member names in full), children continue from the run end.
// L2  F2 展开一次: user-entity hits inside a cycle run merge into the cycle
//     row (which names them); hits outside cycle runs keep force-expanding.
// L3  ⊚中转 short token replaces the 18-cell 用户关注线程(中转) label.
// L4  indent cap (12 levels, "⋯ " leader) + 8-cell name identity floor;
//     subordinate-note vertical shredding dies at the source (lead bounded).
// L5  wrap atom compound table: 根因排序#N / 有效归因 / 下界 / caliber words
//     never bisect across wrapped lines.
// L6  comparison-overview note layering: 口径矛盾 > 窗基 > 披露, fold past 4
//     into the 对比注记明细 sibling block (full set kept whole).
// L7  D-4 ±10% single-artifact tree-head disclosure line (§24.14 补2).

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// ladHuadongLadderPath reconstructs the huadong_78 tree-base-window wakeup
// path (29 elements, audit line 700): the raw order carries oney⇄OS_FFRT
// first and oney⇄VSync ×5 after (复核勘误), which maps onto trunk[5:15) =
// (oney,VSync)×5, trunk[15]=oney, trunk[16]=OS_FFRT, trunk[17]=oney and the
// 7-node non-repeating tail trunk[18:25).
func ladHuadongLadderPath() []string {
	oney := "oney.hmn.berlin-42591"
	vs := "VSyncGenerator-2270"
	return []string{
		"gpu-work-server-2177", "gpu-pm-release-327", "RSUniRenderThre-1963",
		"Acquire Fence-2445", "gpu-pm-release-327", vs, "Acquire Fence-2445",
		vs, "hmfs_discard-26-562", vs,
		oney, "OS_FFRT_2_2-43037", oney, vs, oney, vs, oney, vs, oney, vs,
		oney, vs, oney,
		"tppmgr-idle-0-185", "gpu-work-server-2177", "tppmgr-idle-2-187",
		vs, "tppmgr-idle-4-189",
		oney,
	}
}

// ladHuadongLadderProjection is the huadong_78 ladder witness shape: the raw
// user-entity hits [10,12,...,22] plus the path-end root hit 28 (dropped by
// the trunk mapper's root guard), a VSync data row seating at trunk[1] and
// the E7 rank#9 node seating at the deep tail row trunk[25].
func ladHuadongLadderProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:               ladHuadongLadderPath(),
		WakeupPathUserElected:    true,
		WakeupPathUserEntityHits: []int{10, 12, 14, 16, 18, 20, 22, 28},
		WindowStartTs:            6793224.900,
		WindowEndTs:              6793225.001,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "E5",
				Subject: "VSyncGenerator-2270", Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain", ImpactMS: 6.288, Confidence: 0.8},
			{Role: types.TraceCausalRolePrimaryRootCause, EvidenceID: "E7",
				Subject: "RSUniRenderThre-1963", Object: "runnable_wait", StateKind: "runnable",
				ChainRelevance: "on_chain", Rank: 9, ImpactMS: 4.115,
				CumulativeImpactMS: 4.115, EffectiveImpactMS: 0.058, Confidence: 0.9},
		},
	}
}

func ladFence(t *testing.T, projection types.TraceCausalProjection) string {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	return runtimeTraceProjTreeFence(model, true)
}

// --- L1: the huadong_78 ladder folds into a named counted cycle row ------------

// TestLADCycleFoldHuadongLadder is the acceptance witness: the 14-row / ~60-
// column zero-information ladder (7× forced transit + 7× 1-node fold, one
// indent level each) becomes 5 rows — one cycle row with FULL member names,
// two remaining forced rows, two plain folds. 突变自查 M1: disabling the
// run-length detection (bestK always 0) reds the cycle-row assert AND the
// ⊚中转-count assert (per-hit ladder returns).
func TestLADCycleFoldHuadongLadder(t *testing.T) {
	fence := ladFence(t, ladHuadongLadderProjection())
	// The cycle row: count + both member identities IN FULL (整名不截).
	if !strings.Contains(fence, "↺ 循环×5: oney.hmn.berlin-42591 ⇄ VSyncGenerator-2270") {
		t.Fatalf("the ladder must fold into one named counted cycle row, fence:\n%s", fence)
	}
	// F2 展开一次: the five hits inside the run merge into the cycle row; the
	// two hits outside it (trunk 15/17) keep their force-expanded rows.
	if got := strings.Count(fence, "⊚中转"); got != 2 {
		t.Fatalf("hits inside the cycle run must merge into the cycle row (want 2 forced rows, got %d):\n%s", got, fence)
	}
	// The interrupting hop and the non-repeating tail keep honest plain folds.
	if !strings.Contains(fence, "…省略1节点: OS_FFRT_2_2-43037") {
		t.Fatalf("the cycle-interrupting hop must keep its plain fold, fence:\n%s", fence)
	}
	if !strings.Contains(fence, "…省略7节点: ") {
		t.Fatalf("the non-repeating tail must keep its plain fold (no fuzzy folding), fence:\n%s", fence)
	}
	// The retired forms must not resurface: the long transit label and the
	// index-0 detector's note.
	for _, retired := range []string{"用户关注线程(中转)", "检测到"} {
		if strings.Contains(fence, retired) {
			t.Fatalf("retired form %q resurfaced, fence:\n%s", retired, fence)
		}
	}
	// Deep-row identity floor (L4): the E7 subject survives at the tail depth
	// with its pid tail (pre-LAD it evaporated to "…" / 1 cell).
	if !strings.Contains(fence, "-1963") {
		t.Fatalf("the deep E7 row must keep its pid identity tail, fence:\n%s", fence)
	}
	// L5 + L4 combined: the E7 grammar compounds render whole — never bisected
	// across wrapped subordinate lines (the specimen shredded 根因/排序#9 and
	// 有效归/因 at a 20-cell width).
	if !strings.Contains(fence, "根因排序#9") {
		t.Fatalf("根因排序#9 must render unbroken, fence:\n%s", fence)
	}
	for _, split := range []string{"有效归\n", "根因\n排序", "排序#\n"} {
		if strings.Contains(fence, split) {
			t.Fatalf("compound word bisected across lines (%q), fence:\n%s", split, fence)
		}
	}
	// The cycle row itself stays inside the row cap.
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "循环×") && runewidth.StringWidth(line) > runtimeTraceProjTreeRowMaxWidth {
			t.Fatalf("cycle row exceeds the row cap (%d cells): %q", runewidth.StringWidth(line), line)
		}
	}
}

// TestLADCycleFoldGenerality pins the no-overfit shape set: distinct folded
// middles never fold (异元组不折), a 1-tuple run folds, a 3-tuple run folds —
// the detector is generic run-length over canonical names, never a
// user-entity/VSync special case.
func TestLADCycleFoldGenerality(t *testing.T) {
	build := func(trunkRev []string) types.TraceCausalProjection {
		// WakeupPath = reverse(trunk) + target.
		path := make([]string, 0, len(trunkRev)+1)
		for i := len(trunkRev) - 1; i >= 0; i-- {
			path = append(path, trunkRev[i])
		}
		path = append(path, "target-1")
		return types.TraceCausalProjection{
			WakeupPath:    path,
			WindowStartTs: 100.0, WindowEndTs: 100.2,
			OnChainCauses: []types.TraceCausalProjectionNode{{
				Role: types.TraceCausalRoleRootCauseContext, Subject: "target-1",
				Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
				ImpactMS: 5, Confidence: 0.8,
			}},
		}
	}
	// (a) Distinct middle: 12-node trunk, all unique → plain fold only.
	distinct := []string{"h1-1", "h2-2", "h3-3", "h4-4", "h5-5",
		"m1-11", "m2-12", "m3-13", "m4-14", "t1-21", "t2-22", "t3-23"}
	fence := ladFence(t, build(distinct))
	if strings.Contains(fence, "循环×") {
		t.Fatalf("a non-repeating middle must never mint a cycle row, fence:\n%s", fence)
	}
	if !strings.Contains(fence, "…省略4节点") {
		t.Fatalf("the plain fold must stay byte-faithful, fence:\n%s", fence)
	}
	// (b) 1-tuple run: A,A,A in the middle → ↺ 循环×3: A.
	oneTuple := []string{"h1-1", "h2-2", "h3-3", "h4-4", "h5-5",
		"rep-9", "rep-9", "rep-9", "m4-14", "t1-21", "t2-22", "t3-23"}
	fence = ladFence(t, build(oneTuple))
	if !strings.Contains(fence, "↺ 循环×3: rep-9") {
		t.Fatalf("a 1-tuple run must fold with its count, fence:\n%s", fence)
	}
	// (c) 3-tuple run: A,B,C,A,B,C → ↺ 循环×2: A ⇄ B ⇄ C.
	threeTuple := []string{"h1-1", "h2-2", "h3-3", "h4-4", "h5-5",
		"a-1", "b-2", "c-3", "a-1", "b-2", "c-3", "t1-21", "t2-22", "t3-23"}
	fence = ladFence(t, build(threeTuple))
	if !strings.Contains(fence, "↺ 循环×2: a-1 ⇄ b-2 ⇄ c-3") {
		t.Fatalf("a 3-tuple run must fold with all members named, fence:\n%s", fence)
	}
}

// TestLADFoldSegmentsRunLength unit-pins the segmenter's typed output on the
// huadong trunk shape: one cycle segment [5,15) ×5, the two out-of-run hits
// expanded, the hop and tail as plain segments. 突变自查 M2: reverting F2 to
// per-hit expansion (splitting inside the run) reds the segment map.
func TestLADFoldSegmentsRunLength(t *testing.T) {
	path := ladHuadongLadderPath()
	var trunk []string
	for i := len(path) - 2; i >= 0; i-- {
		trunk = append(trunk, path[i])
	}
	forced := map[int]bool{5: true, 7: true, 9: true, 11: true, 13: true, 15: true, 17: true}
	segments, expanded := runtimeTraceProjFoldSegments(trunk, 5, 25, forced)
	if len(segments) != 3 {
		t.Fatalf("want cycle + 2 plain segments, got %+v", segments)
	}
	if seg := segments[5]; seg.End != 15 || seg.CycleLen != 2 || seg.CycleCount != 5 {
		t.Fatalf("cycle segment mismatch: %+v", seg)
	}
	if seg := segments[16]; seg.End != 17 || seg.CycleLen != 0 {
		t.Fatalf("hop plain segment mismatch: %+v", seg)
	}
	if seg := segments[18]; seg.End != 25 || seg.CycleLen != 0 {
		t.Fatalf("tail plain segment mismatch: %+v", seg)
	}
	if len(expanded) != 2 || !expanded[15] || !expanded[17] {
		t.Fatalf("only the out-of-run hits keep expanded rows: %+v", expanded)
	}
	// No-hit non-repeating range: exactly one plain segment (legacy parity).
	plain := []string{"a-1", "b-2", "c-3", "d-4", "e-5", "f-6", "g-7"}
	segments, expanded = runtimeTraceProjFoldSegments(plain, 2, 5, nil)
	if len(segments) != 1 || segments[2].End != 5 || segments[2].CycleLen != 0 || len(expanded) != 0 {
		t.Fatalf("no-hit non-repeating fold must stay one plain segment: %+v / %+v", segments, expanded)
	}
	// Short trunk: nil (byte-stable).
	if segments, expanded = runtimeTraceProjFoldSegments(plain, -1, -1, forced); segments != nil || expanded != nil {
		t.Fatalf("short trunk must stay nil: %+v / %+v", segments, expanded)
	}
}

// TestLADFirstHitExpandsOutsideCycle pins the L2 boundary: a hit BEFORE the
// run starts keeps its own force-expanded ⊚中转 row and the run folds behind
// it — the disclosure duty never silently moves.
func TestLADFirstHitExpandsOutsideCycle(t *testing.T) {
	// trunk[5]=hit (user entity), trunk[6..10)=(x,y)×2, trunk[10]=z.
	trunkRev := []string{"h1-1", "h2-2", "h3-3", "h4-4", "h5-5",
		"user-7", "x-8", "y-9", "x-8", "y-9", "z-10", "t1-21", "t2-22", "t3-23"}
	path := make([]string, 0, len(trunkRev)+1)
	for i := len(trunkRev) - 1; i >= 0; i-- {
		path = append(path, trunkRev[i])
	}
	path = append(path, "target-1")
	projection := types.TraceCausalProjection{
		WakeupPath: path,
		// user-7 sits at trunk idx 5 → raw path idx len(path)-2-5.
		WakeupPathUserEntityHits: []int{len(path) - 2 - 5},
		WindowStartTs:            100.0, WindowEndTs: 100.2,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, Subject: "target-1",
			Object: "sleep_wait", StateKind: "s_sleep", ChainRelevance: "on_chain",
			ImpactMS: 5, Confidence: 0.8,
		}},
	}
	fence := ladFence(t, projection)
	if !strings.Contains(fence, "user-7 ⊚中转") {
		t.Fatalf("a hit outside the run must keep its force-expanded row, fence:\n%s", fence)
	}
	if !strings.Contains(fence, "↺ 循环×2: x-8 ⇄ y-9") {
		t.Fatalf("the run behind the hit must still fold, fence:\n%s", fence)
	}
}

// --- L3: the ⊚中转 short token + legend --------------------------------------

// TestLADTransitShortTokenLegend pins the legend seat of the short token: the
// b1b transit shape renders `⊚中转` in the fence and the rewritten legend
// entry in the lead (verbatim key). The row-face pins live in the b1b anchor
// file (updated with an EVOLUTION RECORD).
func TestLADTransitShortTokenLegend(t *testing.T) {
	projection := b1bUserFocusFoldProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "⊚中转") {
		t.Fatalf("forced transit rows must wear the short token, fence:\n%s", fence)
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "- `⊚中转` = 折叠段内命中的用户关注线程强制单独成行") {
		t.Fatalf("the legend must teach the short token, lead:\n%s", lead)
	}
	if strings.Contains(lead, "`用户关注线程(中转)`") {
		t.Fatalf("the retired long-label legend key resurfaced, lead:\n%s", lead)
	}
}

// --- L4: indent cap + name identity floor + shred death ------------------------

// TestLADIndentCapRails pins the "⋯ " track on BOTH rail faces (one shared
// builder): ≤12 ancestor levels render byte-identically to the legacy rails;
// beyond the cap the shallowest levels collapse into the fixed 2-cell leader
// so the lead is bounded at every depth. 突变自查 M3: removing the cap (rails
// grow forever) reds the width equalities.
func TestLADIndentCapRails(t *testing.T) {
	ancestors := func(n int) []bool {
		out := make([]bool, n)
		for i := range out {
			out[i] = true
		}
		return out
	}
	// At the cap: legacy bytes.
	at := runtimeTraceProjTreeRow{Ancestors: ancestors(12), Last: true}
	if got, want := runtimeTraceProjTreePrefix(at), strings.Repeat("│   ", 12)+"└─"; got != want {
		t.Fatalf("≤cap rails must stay byte-identical: %q", got)
	}
	// Past the cap: fixed leader + the DEEPEST 12 levels only.
	deep := runtimeTraceProjTreeRow{Ancestors: ancestors(19), Last: true}
	prefix := runtimeTraceProjTreePrefix(deep)
	if !strings.HasPrefix(prefix, "⋯ ") {
		t.Fatalf("past-cap rails must open with the ⋯ leader: %q", prefix)
	}
	if got, want := runewidth.StringWidth(prefix), 2+12*4+2; got != want {
		t.Fatalf("past-cap prefix width must be bounded (%d, want %d): %q", got, want, prefix)
	}
	// The continuation face is capped by the SAME builder.
	cont := runtimeTraceProjRowContinuationIndent(deep)
	if !strings.HasPrefix(cont, "⋯ ") {
		t.Fatalf("subordinate rails must share the capped builder: %q", cont)
	}
	if got, want := runewidth.StringWidth(cont), 2+12*4+2; got != want {
		t.Fatalf("past-cap continuation width must be bounded (%d, want %d)", got, want)
	}
	// Shred death (维度A P1-4): the subordinate payload width at ANY depth now
	// floors at 100 − boundedLead = 44 cells — the huadong E7 notes wrapped in
	// a 20-cell column under a ~80-cell lead.
	lines := runtimeTraceProjSubordinateLines(cont, strings.Repeat("宽", 44))
	if len(lines) > 3 {
		t.Fatalf("deep subordinate notes must not shred into a narrow column: %d lines\n%s", len(lines), strings.Join(lines, "\n"))
	}
}

// TestLADNameBudgetIdentityFloor pins the cap-vs-floor conflict arm: the name
// budget keeps the 8-cell pid-tail identity floor at every fixed width (the
// former clamp-1 arm carried an "unreachable" comment that huadong_78
// falsified — deep-row names evaporated to "…").
func TestLADNameBudgetIdentityFloor(t *testing.T) {
	if got := runtimeTraceProjTreeNameBudget(60); got != 8 {
		t.Fatalf("deep fixed width must keep the 8-cell identity floor, got %d", got)
	}
	if got := runtimeTraceProjTreeNameBudget(46); got != 8 {
		t.Fatalf("the conflict band (fixedW 42..50) floors at 8, got %d", got)
	}
	// Shallow rows keep the legacy budget bytes.
	if got := runtimeTraceProjTreeNameBudget(10); got != 34 {
		t.Fatalf("shallow budget must stay legacy (44-fixedW), got %d", got)
	}
	if got := runtimeTraceProjTreeNameBudget(28); got != 20 {
		t.Fatalf("the 20-cell readability floor must stay, got %d", got)
	}
	// The floor keeps a pid tail through the T2 mid-truncation.
	if got := runtimeTraceProjMidTruncateKeepPid("RSUniRenderThre-1963", 8); !strings.HasSuffix(got, "-1963") {
		t.Fatalf("the 8-cell floor must preserve the pid tail: %q", got)
	}
}

// --- L5: the wrap compound boundary table --------------------------------------

// TestLADWrapAtomCompounds pins the unbreakable compounds at hostile widths:
// every registered grammar word survives whole inside a single chunk and the
// wrap stays byte-lossless (DL2 先例扩展; the huadong E7 notes bisected
// 根因排序#9 / 有效归因 / 下界 at a 20-cell width).
func TestLADWrapAtomCompounds(t *testing.T) {
	text := "优先级反转候选·根因排序#9·置信高·有效归因 0.058ms(全额)"
	chunks := runtimeTraceProjWrapDisplay(text, 20)
	if strings.Join(chunks, "") != text {
		t.Fatalf("wrap must stay byte-lossless: %q", chunks)
	}
	for _, compound := range []string{"根因排序#9", "置信高", "有效归因", "(全额)"} {
		whole := false
		for _, chunk := range chunks {
			if strings.Contains(chunk, compound) {
				whole = true
				break
			}
		}
		if !whole {
			t.Fatalf("compound %q bisected across chunks: %q", compound, chunks)
		}
	}
	// R5 (§29.88.12) word family: the widest compound atom is now the
	// 18-cell 按全域最大核最高频 — the wholeness guarantee holds at any width
	// the atom fits (the over-wide lane hard-splits below that, by design);
	// the stress width moves 12→20 with the atom (the old 10-cell 按大核满频
	// fit at 12).
	supply := "供给折算缺口 1.853ms(运行频点非最高,按全域最大核最高频折算,下界)为主"
	chunks = runtimeTraceProjWrapDisplay(supply, 20)
	if strings.Join(chunks, "") != supply {
		t.Fatalf("wrap must stay byte-lossless: %q", chunks)
	}
	for _, compound := range []string{"按全域最大核最高频", "运行频点非最高", "下界"} {
		whole := false
		for _, chunk := range chunks {
			if strings.Contains(chunk, compound) {
				whole = true
				break
			}
		}
		if !whole {
			t.Fatalf("caliber compound %q bisected across chunks: %q", compound, chunks)
		}
	}
}

// --- L6: comparison-note layering ----------------------------------------------

// TestLADCompareNoteLayering unit-pins the layered fold: importance classes
// sort stably (same class adjacent, in-class order kept), ≤4 notes render
// whole with NO detail set, >4 fold the lowest-importance tail behind ONE
// pointer line and return the full layered set. 突变自查 M4: disabling the
// class sort or the fold reds the order/count asserts.
func TestLADCompareNoteLayering(t *testing.T) {
	notes := []runtimeTraceProjCompareNote{
		{Class: runtimeTraceProjCompareNoteWindowBase, Text: "⚠ base-1"},
		{Class: runtimeTraceProjCompareNoteDisclosure, Text: "⚠ disc-1"},
		{Class: runtimeTraceProjCompareNoteCaliberConflict, Text: "⚠ conflict-1"},
		{Class: runtimeTraceProjCompareNoteWindowBase, Text: "⚠ base-2"},
		{Class: runtimeTraceProjCompareNoteDisclosure, Text: "⚠ disc-2"},
	}
	visible, all := runtimeTraceProjCompareLayerNotes(notes, true)
	if len(visible) != runtimeTraceProjCompareNoteVisibleCap+1 {
		t.Fatalf("visible face must be cap + pointer, got %v", visible)
	}
	wantVisible := []string{"⚠ conflict-1", "⚠ base-1", "⚠ base-2", "⚠ disc-1",
		"⚠ 其余 1 条注记见下方「对比注记明细」"}
	for i, want := range wantVisible {
		if visible[i] != want {
			t.Fatalf("layered order mismatch at %d: got %q want %q (%v)", i, visible[i], want, visible)
		}
	}
	if len(all) != 5 || all[4] != "⚠ disc-2" {
		t.Fatalf("the detail set must carry EVERY note layered: %v", all)
	}
	// ≤cap: everything visible, no fold, no detail set (no sibling block).
	visible, all = runtimeTraceProjCompareLayerNotes(notes[:4], true)
	if len(visible) != 4 || all != nil {
		t.Fatalf("≤cap notes must render whole with no detail set: %v / %v", visible, all)
	}
	if visible[0] != "⚠ conflict-1" {
		t.Fatalf("importance order applies below the cap too: %v", visible)
	}
	if block := runtimeTraceProjCompareNotesDetailBlock(nil, true); block != nil {
		t.Fatalf("no folded set → no sibling block: %+v", block)
	}
}

// TestLADCompareNotesBlockFamily pins the sibling block's id plumbing: it is a
// member of the system block-id family (idempotence guard + PSG evidence
// surface) and the cap-degrade lane never picks it as the survivor.
func TestLADCompareNotesBlockFamily(t *testing.T) {
	if !runtimeTraceCausalProjectionFamilyBlockID(runtimeTraceCausalProjectionCompareNotesBlockID) {
		t.Fatalf("the notes-detail block must join the projection block-id family")
	}
	if !RuntimeTraceSystemBlockID(runtimeTraceCausalProjectionCompareNotesBlockID) {
		t.Fatalf("the notes-detail block is a system evidence surface (PSG)")
	}
	cluster := []types.AnswerBlock{
		{ID: runtimeTraceCausalProjectionCompareBlockID},
		{ID: runtimeTraceCausalProjectionCompareNotesBlockID},
		{ID: runtimeTraceCausalProjectionBlockIDBase + "_a1"},
	}
	lead := runtimeTraceCausalProjectionDegradeLeadBlock(cluster)
	if lead == nil || lead.ID != runtimeTraceCausalProjectionBlockIDBase+"_a1" {
		t.Fatalf("cap degrade must skip the overview AND its notes sibling, got %+v", lead)
	}
}

// --- L7: D-4 single-artifact tree-head deviation line ---------------------------

// TestLADSoloUserWindowDeviationLine pins the §24.14 补2 single-artifact face:
// >±10% deviation between the analysis window and the user's stated duration
// renders ONE tree-head sentence (both lengths + %.1f deviation + the
// construction clause); ≤±10% stays silent; the <50% slice shape keeps the
// richer R2 relation line INSTEAD (no double disclosure); the comparison lane
// (SoloArtifact=false) stays silent — COV-2 owns that face.
func TestLADSoloUserWindowDeviationLine(t *testing.T) {
	base := func(winEnd float64) (types.TraceCausalProjection, runtimeTraceProjTreeModel) {
		projection := types.TraceCausalProjection{
			WakeupPath:    []string{"waker-2", "main-1"},
			WindowStartTs: 100.0, WindowEndTs: winEnd,
			OnChainCauses: []types.TraceCausalProjectionNode{{
				Role: types.TraceCausalRoleRootCauseContext, Subject: "waker-2",
				Object: "runnable_wait", StateKind: "runnable", ChainRelevance: "on_chain",
				ImpactMS: 5, Confidence: 0.8,
			}},
		}
		model := buildRuntimeTraceProjTreeModel(projection, nil, true)
		model.SoloArtifact = true
		model.UserWindowStart, model.UserWindowEnd = 100.0, 100.884
		return projection, model
	}
	// Longer arm: 1645ms vs the stated 884ms (huadong/cmp 形, 86.1%).
	projection, model := base(101.645)
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "- 分析窗 1645.000ms,较你指定的 884.000ms 长 86.1%:窗口按数据边界对齐构造") {
		t.Fatalf("solo longer-window deviation must disclose: %q", line)
	}
	// Shorter arm inside the (50%,90%) band.
	projection, model = base(100.700)
	line = runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "- 分析窗 700.000ms,较你指定的 884.000ms 短 20.8%:窗口按数据边界对齐构造") {
		t.Fatalf("solo shorter-window deviation must disclose: %q", line)
	}
	// ≤±10%: silent (absence never guesses).
	projection, model = base(100.930)
	if line = runtimeTraceProjWindowLine(projection, model, true); strings.Contains(line, "窗口按数据边界对齐构造") {
		t.Fatalf("within-tolerance must stay silent: %q", line)
	}
	// <50% slice: the R2 relation line renders INSTEAD of the deviation form.
	projection, model = base(100.300)
	line = runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "本因果树的分析窗只取其中一段") || strings.Contains(line, "窗口按数据边界对齐构造") {
		t.Fatalf("the slice shape keeps the richer R2 line only: %q", line)
	}
	// Comparison lane: per-side tree heads never repeat the COV-2 note.
	projection, model = base(101.645)
	model.SoloArtifact = false
	if line = runtimeTraceProjWindowLine(projection, model, true); strings.Contains(line, "窗口按数据边界对齐构造") {
		t.Fatalf("multi-artifact tree heads must not repeat the D-4 face: %q", line)
	}
	// EN face.
	projection, model = base(101.645)
	if line = runtimeTraceProjWindowLine(projection, model, false); !strings.Contains(line, "The analysis window 1645.000ms is 86.1% longer than your requested 884.000ms: the window is constructed by aligning to data boundaries") {
		t.Fatalf("EN deviation line mismatch: %q", line)
	}
}
