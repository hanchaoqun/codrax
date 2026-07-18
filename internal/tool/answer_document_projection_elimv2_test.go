package tool

// answer_document_projection_elimv2_test.go — ELIM-V2 方向分组制 pins (设计终稿
// scratchpad/elim_v2_spec.md, 用户授权 2026-07-18; ledger
// docs/design/real_trace_campaign_20260705.md):
//
//	pin①  节成员方向 typed census — every rendered chain member sits under
//	      exactly the section head whose word matches its engine-published
//	      FixDirection (显示侧零词面推断; unresolved → 未定 tail).
//	pin②  节序 = 节内最大可消 desc; 未定/复合 tail always last.
//	pin③  小计三档 — L1 µs identity (subtotal == Σ printed member values),
//	      L2 合计不可直加 + 小计 absence, L3 zero-arithmetic absence (missing
//	      envelope / merged carrier).
//	pin④  无跨方向总计 absence — 小计 bytes live on ▸ heads only; the head
//	      declaration 方向间收益不可相加 stands.
//	pin⑤  守恒尾行词形 — gated pass line / typed violation transcription,
//	      deduped, mutually exclusive.
//	pin⑥  ∩ chip both-with-tree — the ◎ chips and pair footnote render IFF
//	      the tree rows speak the full 互指句 (one resolved-clause source);
//	      载体缺席 → 双双不发.
//	pin⑧  ⛓ 块整体先于 ◇ 保序 (section heads + members before the ◇ block
//	      head + members).
//	pin⑨  ◎ 方向节零序数 (§29.132 根因排序护栏① 的 ◎ 节内半场).
//	pin⑬  section-head width discipline (new structural lines ≤ the row cap).
//
// pin⑦ (⌗/症状/▒ 脚注字节回归) and pin⑩ (闭合恒等式) live in the existing
// elim/elimgap pin files, which run against the SAME renderer — the byte
// assertions there are the regression pins.
//
// MUTATION self-checks (cp-copy recovery only):
//   - drop the 未定-last ordering arm → TestELIMV2SectionOrderMaxDesc red;
//   - sum the L2 section anyway → TestELIMV2SubtotalLadder red (小计 absence);
//   - render the chip without the tree clause source →
//     TestELIMV2CrossDirectionChipBothWithTree red.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// elimv2DirectionNode builds a chain rank node carrying the engine-published
// direction attribute plus a faithful typed envelope (the L1 carrier).
func elimv2DirectionNode(id, subject, typeToken, state string, rank int, eff float64, line int, direction string, startTs, endTs float64) types.TraceCausalProjectionNode {
	node := elimChainNode(id, subject, typeToken, state, rank, eff, line)
	node.FixDirection = direction
	node.StartTs, node.EndTs = startTs, endTs
	return node
}

// elimv2DirectionBoardProjection — the representative ELIM-V2 board: an L1
// scheduling-supply pair (disjoint envelopes), a single frequency seat that
// forms a resolved cross-direction ∩ pair with the L1 head seat (same thread,
// symmetric wire entries — the cust_span_runnable E26×E5 geometry family), an
// L2 io pair (overlapping envelopes) and one directed ◇ member (the fallback
// append). Window 200ms → the conservation pass line has a known ruler.
func elimv2DirectionBoardProjection() types.TraceCausalProjection {
	m1 := elimv2DirectionNode("Ev2-run-a", "worker-T-77", "runnable_wait", "runnable", 1, 6.0, 100, "scheduling_supply", 1000.010, 1000.016)
	m3 := elimv2DirectionNode("Ev2-freq", "worker-T-77", "running", "running", 3, 5.0, 200, "frequency_thermal", 1000.000, 1000.100)
	m1.CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{
		{OverlapMS: 2.5, LineStart: m3.LineStart, LineEnd: m3.LineEnd,
			Direction: "frequency_thermal", Basis: "self_running_intervals"},
	}
	m3.CrossDirectionOverlaps = []types.TraceCausalProjectionCrossDirectionOverlap{
		{OverlapMS: 2.5, LineStart: m1.LineStart, LineEnd: m1.LineEnd,
			Direction: "scheduling_supply", Basis: "runnable_intervals"},
	}
	m2 := elimv2DirectionNode("Ev2-run-b", "worker-B-88", "runnable_wait", "runnable", 2, 3.0, 300, "scheduling_supply", 1000.020, 1000.023)
	m4 := elimv2DirectionNode("Ev2-io-c", "worker-C-99", "d_state_or_io_wait", "d_sleep", 4, 2.0, 400, "io_dependency", 1000.050, 1000.060)
	m5 := elimv2DirectionNode("Ev2-io-d", "worker-D-11", "d_state_or_io_wait", "d_sleep", 5, 1.9, 500, "io_dependency", 1000.055, 1000.058)
	adj := elimv2DirectionNode("Ev2-adj", "adj-worker-55", "runnable_wait", "runnable", 1, 0.7, 600, "scheduling_supply", 1000.150, 1000.152)
	adj.Predicate = "root_cause_tertiary"
	adj.ChainRelevance = "adjacent"
	adj.Causality = "adjacent_to_wakeup_chain"
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"waker-1", "worker-T-77"},
		WindowStartTs:           1000.000, WindowEndTs: 1000.200,
		OnChainCauses:  []types.TraceCausalProjectionNode{m1, m2, m3, m4, m5},
		AdjacentCauses: []types.TraceCausalProjectionNode{adj},
	}
}

// elimv2SectionHeads returns the fence's ▸ head lines in render order.
func elimv2SectionHeads(fence string) []string {
	var out []string
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") {
			out = append(out, line)
		}
	}
	return out
}

// --- pin① + pin③(L1/L2) + ∩/◇/守恒 word faces on the representative board ------

func TestELIMV2DirectionSectionsLayout(t *testing.T) {
	projection := elimv2DirectionBoardProjection()
	model, fence := elimRenderOverview(t, projection, true)
	heads := elimv2SectionHeads(fence)
	if len(heads) != 3 {
		t.Fatalf("expected exactly 3 direction sections, got %d:\n%s", len(heads), fence)
	}
	// pin②(此板): 节序 = 节内最大可消 desc (6.0 sched → 5.0 freq → 2.0 io).
	if !strings.Contains(heads[0], "▸ 调度供给 · 最大可消 6.000ms") ||
		!strings.Contains(heads[1], "▸ 频率与热治理 · 最大可消 5.000ms") ||
		!strings.Contains(heads[2], "▸ IO与依赖 · 最大可消 2.000ms") {
		t.Fatalf("section heads must order by max eliminable desc with verbatim maxima:\n%s", fence)
	}
	// pin③ L1: the disjoint-envelope pair publishes the µs subtotal.
	if !strings.Contains(heads[0], " · 2席 · 小计 9.000ms(区间互斥)") {
		t.Fatalf("L1: the disjoint scheduling pair must publish its subtotal:\n%s", heads[0])
	}
	// pin③ L2: the overlapping io pair refuses the sum.
	if !strings.Contains(heads[2], " · 2席 · 成员区间重叠,合计不可直加") || strings.Contains(heads[2], "小计") {
		t.Fatalf("L2: the overlapping io pair must refuse the subtotal:\n%s", heads[2])
	}
	// 单席节 (委托默认): no seat count, no subtotal on the frequency head.
	if strings.Contains(heads[1], "席") || strings.Contains(heads[1], "小计") {
		t.Fatalf("a single-seat section head carries neither seat count nor subtotal:\n%s", heads[1])
	}
	// pin①: every rendered chain member sits under its own direction's head.
	wantSection := map[string]string{
		"6.000ms": "调度供给", "3.000ms": "调度供给",
		"5.000ms": "频率与热治理",
		"2.000ms": "IO与依赖", "1.900ms": "IO与依赖",
	}
	current := ""
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") {
			current = line
			continue
		}
		if strings.HasPrefix(line, "◇ 邻近(") {
			current = "" // the ◇ block is unsectioned
			continue
		}
		if !strings.Contains(line, "⛓ 链上") || !strings.Contains(line, "█") {
			continue
		}
		for value, direction := range wantSection {
			if strings.Contains(line, value) && !strings.Contains(current, direction) {
				t.Fatalf("pin①: member %s must render under the %s head (got %q):\n%s", value, direction, current, fence)
			}
		}
	}
	// pin③ L1 µs identity: subtotal == Σ printed member values (逐µs).
	if 6000+3000 != 9000 {
		t.Fatalf("unreachable")
	}
	// ∩ chips on BOTH seats of the resolved pair + the merged footnote.
	var m1Line, m3Line string
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "6.000ms") {
			m1Line = line
		}
		if strings.Contains(line, "5.000ms") {
			m3Line = line
		}
	}
	if !strings.Contains(m1Line, "·∩[") || !strings.Contains(m3Line, "·∩[") {
		t.Fatalf("both seats of the resolved ∩ pair must wear the chip:\n%s\n%s", m1Line, m3Line)
	}
	if !strings.Contains(fence, "· ∩ 跨方向重叠对(修其一后另一席空间会缩,收益不叠加):") ||
		!strings.Contains(fence, "重叠 2.500ms · 全句见树行互指") {
		t.Fatalf("the merged ∩ pair footnote must transcribe the typed overlap:\n%s", fence)
	}
	// ◇ block head + the ◇ member's direction transcription word.
	if !strings.Contains(fence, "◇ 邻近(条件可消上界 · 不入方向守恒)") {
		t.Fatalf("the ◇ block head must separate the sections from the adjacent block:\n%s", fence)
	}
	adjLine := ""
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "0.700ms") {
			adjLine = line
		}
	}
	if !strings.Contains(adjLine, "◇ 邻近") || !strings.Contains(adjLine, "·方向=调度供给") {
		t.Fatalf("the ◇ member must wear the ·方向=X transcription word:\n%s", adjLine)
	}
	// 守恒尾行 pass form with the verbatim window.
	if !strings.Contains(fence, "· 守恒:各方向支撑区间并集皆 ≤ 窗 200.000ms(检查器)") {
		t.Fatalf("the conservation pass line must transcribe the checker ruler:\n%s", fence)
	}
	// The head declaration (三层防相加之一) rides the form promise.
	if !strings.Contains(fence, "方向间收益不可相加") {
		t.Fatalf("the head anti-addition declaration must stand:\n%s", fence)
	}
	// Marks record at the emission sites (词条-图例双向 rides the shared sweep).
	for _, mark := range []runtimeTraceProjMark{
		runtimeTraceProjMarkElimDirectionSection, runtimeTraceProjMarkElimSectionSubtotal,
		runtimeTraceProjMarkElimSectionNonAddable, runtimeTraceProjMarkElimCrossDirectionChip,
		runtimeTraceProjMarkElimAdjacentDirectionWord, runtimeTraceProjMarkElimAdjacentBlockHead,
		runtimeTraceProjMarkElimConservation,
	} {
		if !model.Marks.has(mark) {
			t.Fatalf("mark %d must record on the representative board", mark)
		}
	}
	// EN face mirrors the layout (spot probes).
	_, fenceEN := elimRenderOverview(t, projection, false)
	for _, want := range []string{
		"▸ scheduling supply · max eliminable 6.000ms · 2 seats · subtotal 9.000ms (disjoint intervals)",
		"▸ IO & dependency · max eliminable 2.000ms · 2 seats · member intervals overlap; do not add",
		"◇ adjacent (conditional upper bound · outside direction conservation)",
		"· direction=scheduling supply",
		"· conservation: every direction's support-interval union ≤ window 200.000ms (checker)",
		"gains never add across directions",
	} {
		if !strings.Contains(fenceEN, want) {
			t.Fatalf("EN face must mirror %q:\n%s", want, fenceEN)
		}
	}
}

// pin③ 可重构性 (原始值可见性三问③): the printed subtotal re-derives µs-for-µs
// from the printed member values — parsed back from the fence bytes alone.
func TestELIMV2SubtotalReconstructsFromMemberRows(t *testing.T) {
	_, fence := elimRenderOverview(t, elimv2DirectionBoardProjection(), true)
	heads := elimv2SectionHeads(fence)
	subRe := regexp.MustCompile(`小计 (\d+\.\d{3})ms\(区间互斥\)`)
	match := subRe.FindStringSubmatch(heads[0])
	if match == nil {
		t.Fatalf("the L1 head must carry the subtotal:\n%s", heads[0])
	}
	subtotal, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		t.Fatal(err)
	}
	valueRe := regexp.MustCompile(`(\d+\.\d{3})ms `)
	sumUs := int64(0)
	inSection := false
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") {
			inSection = strings.Contains(line, "调度供给")
			continue
		}
		if strings.HasPrefix(line, "◇ 邻近(") {
			inSection = false
		}
		if !inSection || !strings.Contains(line, "█") {
			continue
		}
		if m := valueRe.FindStringSubmatch(line); m != nil {
			v, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				t.Fatal(err)
			}
			sumUs += int64(v*1000 + 0.5)
		}
	}
	if sumUs != int64(subtotal*1000+0.5) {
		t.Fatalf("pin③: subtotal %.3f must equal the Σ of printed member values (%dµs):\n%s", subtotal, sumUs, fence)
	}
}

// --- pin② 节序 + 未定尾节 ---------------------------------------------------------

func TestELIMV2SectionOrderMaxDesc(t *testing.T) {
	projection := elimv2DirectionBoardProjection()
	// Invert the magnitudes: io becomes the dominant direction; add an
	// UNRESOLVED member LARGER than every resolved one — the tail section must
	// still render last (fail-open material never outranks).
	projection.OnChainCauses[3].EffectiveImpactMS = 8.0
	projection.OnChainCauses[3].ImpactMS = 8.0
	projection.OnChainCauses[3].CumulativeImpactMS = 8.0
	unresolved := elimChainNode("Ev2-unres", "worker-U-22", "custom_wait", "runnable", 6, 9.5, 700)
	projection.OnChainCauses = append(projection.OnChainCauses, unresolved)
	_, fence := elimRenderOverview(t, projection, true)
	heads := elimv2SectionHeads(fence)
	if len(heads) < 3 {
		t.Fatalf("expected ≥3 sections, got %d:\n%s", len(heads), fence)
	}
	if !strings.Contains(heads[0], "▸ IO与依赖 · 最大可消 8.000ms") {
		t.Fatalf("pin②: the dominant io section must lead:\n%s", fence)
	}
	last := heads[len(heads)-1]
	if !strings.Contains(last, "▸ 方向未定/复合 · 最大可消 9.500ms") {
		t.Fatalf("pin②/⑪: the unresolved tail section renders LAST despite holding the largest value:\n%s", fence)
	}
	// 未定节零算术: no seat count, no subtotal, no overlap word.
	if strings.Contains(last, "席") || strings.Contains(last, "小计") || strings.Contains(last, "不可直加") {
		t.Fatalf("the unresolved tail publishes no arithmetic:\n%s", last)
	}
	// pin⑧: every chain member and section head renders before the ◇ block.
	adjHead := strings.Index(fence, "◇ 邻近(")
	if adjHead < 0 {
		t.Fatalf("the ◇ block head must render:\n%s", fence)
	}
	if lastHead := strings.LastIndex(fence, "▸ "); lastHead > adjHead {
		t.Fatalf("pin⑧: every ▸ section must precede the ◇ block:\n%s", fence)
	}
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "⛓ 链上") && strings.Index(fence, line) > adjHead {
			t.Fatalf("pin⑧: chain members must precede the ◇ block:\n%s", fence)
		}
	}
}

// --- pin③ L3 载体缺席 + 跨板 -----------------------------------------------------

func TestELIMV2SubtotalLadderCarrierAbsent(t *testing.T) {
	// L3a: one member of the L1 pair loses its typed envelope → zero
	// arithmetic (no seat count, no subtotal, no overlap claim).
	missing := elimv2DirectionBoardProjection()
	missing.OnChainCauses[1].StartTs = 0
	missing.OnChainCauses[1].EndTs = 0
	_, fence := elimRenderOverview(t, missing, true)
	for _, head := range elimv2SectionHeads(fence) {
		if strings.Contains(head, "调度供给") {
			if strings.Contains(head, "席") || strings.Contains(head, "小计") || strings.Contains(head, "不可直加") {
				t.Fatalf("L3: a carrier-less member kills every arithmetic claim:\n%s", head)
			}
			if !strings.Contains(head, "最大可消 6.000ms") {
				t.Fatalf("L3: the max eliminable stays (恒发):\n%s", head)
			}
		}
	}
	// L3b: a merged carrier (envelope may understate the account) steps the
	// section down the same way.
	merged := elimv2DirectionBoardProjection()
	merged.OnChainCauses[1].MergedCount = 3
	_, fence = elimRenderOverview(t, merged, true)
	for _, head := range elimv2SectionHeads(fence) {
		if strings.Contains(head, "调度供给") && (strings.Contains(head, "小计") || strings.Contains(head, "不可直加")) {
			t.Fatalf("L3: a merged carrier must not publish section arithmetic:\n%s", head)
		}
	}
	// 跨板: members on different typed boards publish no arithmetic either.
	boards := elimv2DirectionBoardProjection()
	boards.OnChainCauses[0].RankBoardTarget = "worker-T-77"
	boards.OnChainCauses[1].RankBoardTarget = "other-board-1"
	_, fence = elimRenderOverview(t, boards, true)
	for _, head := range elimv2SectionHeads(fence) {
		if strings.Contains(head, "调度供给") && (strings.Contains(head, "小计") || strings.Contains(head, "不可直加")) {
			t.Fatalf("跨板: cross-board members must not publish section arithmetic:\n%s", head)
		}
	}
	// 修补轮 件3 混合形负臂: ONE member with a named board identity plus one
	// with a MISSING identity on the same named board ({空,具名} mixed) — the
	// bare seat could belong to any board, so the single-board premise is
	// unproven and the section publishes no arithmetic (缺席不进算术).
	mixed := elimv2DirectionBoardProjection()
	mixed.OnChainCauses[0].RankBoardTarget = "worker-T-77" // == ruler subject; multiBoardRuler stays false
	_, fence = elimRenderOverview(t, mixed, true)
	if strings.Contains(fence, "跨板不可相加") {
		t.Fatalf("fixture: the mixed form must keep the single-board head:\n%s", fence)
	}
	for _, head := range elimv2SectionHeads(fence) {
		if strings.Contains(head, "调度供给") && (strings.Contains(head, "小计") || strings.Contains(head, "不可直加")) {
			t.Fatalf("件3 混合形: an identity-less member beside a named board kills the arithmetic:\n%s", head)
		}
	}
	// 修补轮 件3 跨板尺负臂: a multi-board RULER (a foreign-board row anywhere
	// on the board) suspends every section's arithmetic — even one whose own
	// members share a single named board.
	foreign := elimv2DirectionBoardProjection()
	foreign.OnChainCauses[0].RankBoardTarget = "worker-T-77"
	foreign.OnChainCauses[1].RankBoardTarget = "worker-T-77"
	foreign.OnChainCauses[3].RankBoardTarget = "other-board-1" // io member → multiBoardRuler
	_, fence = elimRenderOverview(t, foreign, true)
	if !strings.Contains(fence, "跨板不可相加") {
		t.Fatalf("fixture: the foreign-board row must flip the multi-board ruler head:\n%s", fence)
	}
	for _, head := range elimv2SectionHeads(fence) {
		if strings.Contains(head, "调度供给") && (strings.Contains(head, "小计") || strings.Contains(head, "不可直加")) {
			t.Fatalf("件3 跨板尺: a multi-board ruler suspends section arithmetic everywhere:\n%s", head)
		}
	}
}

// 修补轮 件6③: the subtotal's one rounding authority is the PRINTED %.3f face
// of the member rows — a binary float in the .0005 neighbourhood prints one
// way and raw-rounds the other (0.0045 → face 0.004 / raw 5µs), and only the
// printed path keeps the head µs-identical to its own rows.
func TestELIMV2SubtotalUsesPrintedFaceRounding(t *testing.T) {
	dust := 0.0044999999999999997 // %.3f face "0.004"; int64(v*1000+0.5) says 5
	if runtimeTraceProjElimPrintedUs(dust) != 4 {
		t.Fatalf("printed path must follow the %%.3f face (4µs), got %d", runtimeTraceProjElimPrintedUs(dust))
	}
	if raw := int64(dust*1000 + 0.5); raw != 5 {
		t.Fatalf("witness drifted: the raw path must disagree (5µs) for this pin to bite, got %d", raw)
	}
	projection := elimv2DirectionBoardProjection()
	projection.OnChainCauses[1].ImpactMS = dust
	projection.OnChainCauses[1].CumulativeImpactMS = dust
	projection.OnChainCauses[1].EffectiveImpactMS = dust
	_, fence := elimRenderOverview(t, projection, true)
	member := ""
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "worker-B-88") {
			member = line
		}
	}
	if !strings.Contains(member, "0.004ms") {
		t.Fatalf("fixture: the dust member must print the 0.004 face:\n%s", fence)
	}
	for _, head := range elimv2SectionHeads(fence) {
		if !strings.Contains(head, "调度供给") {
			continue
		}
		if !strings.Contains(head, "小计 6.004ms(区间互斥)") {
			t.Fatalf("件6③: the subtotal must sum the PRINTED member faces (6.000+0.004), got:\n%s", head)
		}
	}
}

// --- pin④ 无跨方向总计 ------------------------------------------------------------

func TestELIMV2NoCrossDirectionTotal(t *testing.T) {
	for _, zh := range []bool{true, false} {
		_, fence := elimRenderOverview(t, elimv2DirectionBoardProjection(), zh)
		// 小计/subtotal bytes live on ▸ section heads ONLY — never a region
		// grand total, never on any other line family.
		word := "小计"
		if !zh {
			word = "subtotal"
		}
		for _, line := range strings.Split(fence, "\n") {
			if strings.Contains(line, word) && !strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") {
				t.Fatalf("pin④: %q outside a ▸ section head (zh=%v):\n%s", line, zh, fence)
			}
		}
		for _, banned := range []string{"总计", "Σ", "grand total"} {
			if strings.Contains(fence, banned) {
				t.Fatalf("pin④: the region must never print %q:\n%s", banned, fence)
			}
		}
	}
}

// --- pin⑤ 守恒尾行 ---------------------------------------------------------------

func TestELIMV2ConservationLines(t *testing.T) {
	// Violation form: the typed finding transcribes per direction, deduped
	// across its member seats, and the pass line yields. 修补轮 件6①: the
	// engine mints one finding per (thread, direction) group, so the dedup
	// fixture is engine-faithful — the two carrying seats share ONE thread.
	projection := elimv2DirectionBoardProjection()
	projection.OnChainCauses[1].Subject = "worker-T-77"
	finding := &types.TraceCausalProjectionDirectionConservation{
		Direction: "scheduling_supply", SumMS: 250.0, WindowMS: 200.0, SeatCount: 2,
	}
	projection.OnChainCauses[0].DirectionConservationExcess = finding
	projection.OnChainCauses[1].DirectionConservationExcess = finding
	_, fence := elimRenderOverview(t, projection, true)
	want := "· 守恒违例:方向 调度供给 支撑区间并集合计 250.000ms > 窗 200.000ms(2席,同线程)——同段物理时间重复计费(检查器,仅披露不改值)"
	if strings.Count(fence, "守恒违例") != 1 {
		t.Fatalf("pin⑤: the identical finding dedupes to ONE violation line:\n%s", fence)
	}
	// The `· ` line may wrap through the width governor — judge on the
	// despaced surface (§29.114 discipline).
	if !strings.Contains(vs2Despace(fence), vs2Despace(want)) {
		t.Fatalf("pin⑤: the violation line must transcribe the typed finding:\n%s", fence)
	}
	if strings.Contains(fence, "· 守恒:各方向支撑区间并集皆") {
		t.Fatalf("pin⑤: the pass line must yield to the violation form:\n%s", fence)
	}
	// The violation value channel stays untouched (纯披露).
	if !strings.Contains(fence, "6.000ms") || !strings.Contains(fence, "3.000ms") {
		t.Fatalf("pin⑤: member values stay untouched:\n%s", fence)
	}
	// Gate negative: a board whose chain members carry NO published direction
	// (legacy replay — the checker generation never ran) renders NEITHER form.
	legacy := elimBoardProjection()
	_, legacyFence := elimRenderOverview(t, legacy, true)
	if strings.Contains(legacyFence, "· 守恒") {
		t.Fatalf("pin⑤: a direction-less legacy board must not claim the checker ran:\n%s", legacyFence)
	}
	// 修补轮 件6① pin: two DIFFERENT-thread engine groups whose tuples are
	// coincidentally identical keep BOTH disclosure lines — the dedup key
	// carries the seat-thread anchor beside the tuple (engine 键形=(thread,
	// direction); tuple-only dedup would swallow one group's line).
	twoGroups := elimv2DirectionBoardProjection()
	twoGroups.OnChainCauses[0].DirectionConservationExcess = &types.TraceCausalProjectionDirectionConservation{
		Direction: "scheduling_supply", SumMS: 250.0, WindowMS: 200.0, SeatCount: 2,
	}
	twoGroups.OnChainCauses[1].DirectionConservationExcess = &types.TraceCausalProjectionDirectionConservation{
		Direction: "scheduling_supply", SumMS: 250.0, WindowMS: 200.0, SeatCount: 2,
	}
	_, twoFence := elimRenderOverview(t, twoGroups, true)
	if strings.Count(twoFence, "守恒违例") != 2 {
		t.Fatalf("件6①: identical tuples on two threads (worker-T-77 / worker-B-88) must keep two lines:\n%s", twoFence)
	}
	// 修补轮 件7: a violating group whose every carrier is DISPLAY-EXCLUDED
	// (here the gated-share constituent arm) must still transcribe its
	// violation line and suppress the pass claim — the checker verdict is
	// engine truth about the direction population, and 排除≠消失 extends to
	// the 守恒 tail (pre-件7 the pass line printed over the excluded finding).
	excluded := elimv2DirectionBoardProjection()
	carrier := elimv2DirectionNode("Ev2-constituent", "worker-E-33", "runnable_wait", "runnable", 6, 1.2, 800, "scheduling_supply", 1000.170, 1000.172)
	carrier.GatedShareConstituentSeat = true
	carrier.DirectionConservationExcess = &types.TraceCausalProjectionDirectionConservation{
		Direction: "scheduling_supply", SumMS: 260.0, WindowMS: 200.0, SeatCount: 3,
	}
	excluded.OnChainCauses = append(excluded.OnChainCauses, carrier)
	_, exFence := elimRenderOverview(t, excluded, true)
	for _, line := range elimOverviewMemberLines(exFence) {
		if strings.Contains(line, "worker-E-33") {
			t.Fatalf("fixture: the constituent carrier must stay off the value board:\n%s", exFence)
		}
	}
	if strings.Count(exFence, "守恒违例") != 1 || !strings.Contains(exFence, "260.000ms") {
		t.Fatalf("件7: the excluded carrier's violation must transcribe:\n%s", exFence)
	}
	if strings.Contains(exFence, "· 守恒:各方向支撑区间并集皆") {
		t.Fatalf("件7: the pass line must yield to the excluded-carrier violation:\n%s", exFence)
	}
}

// --- pin⑥ ∩ both-with-tree ------------------------------------------------------

func TestELIMV2CrossDirectionChipBothWithTree(t *testing.T) {
	projection := elimv2DirectionBoardProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	overview := runtimeTraceProjElimOverviewFence(projection, model, true)
	if !strings.Contains(tree, "同段重叠 2.500ms") || !strings.Contains(tree, "收益不叠加") {
		t.Fatalf("fixture: the tree rows must speak the full 互指句:\n%s", tree)
	}
	if !strings.Contains(overview, "·∩[") || !strings.Contains(overview, "· ∩ 跨方向重叠对") {
		t.Fatalf("pin⑥: with the tree clause in place the ◎ transcribes chip + footnote:\n%s", overview)
	}
	// 载体缺席不发: a one-sided wire roster prunes the tree pair AND every ◎
	// transcription (chips + footnote) — both-or-neither across both faces.
	oneSided := elimv2DirectionBoardProjection()
	oneSided.OnChainCauses[2].CrossDirectionOverlaps = nil
	model = buildRuntimeTraceProjTreeModel(oneSided, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree = runtimeTraceProjTreeFence(model, true)
	overview = runtimeTraceProjElimOverviewFence(oneSided, model, true)
	if strings.Contains(tree, "收益不叠加") {
		t.Fatalf("fixture: the one-sided roster must prune the tree pair:\n%s", tree)
	}
	if strings.Contains(overview, "∩") {
		t.Fatalf("pin⑥: no tree clause → no ◎ chip, no footnote (载体缺席不发):\n%s", overview)
	}
}

// --- pin⑨ ◎ 方向节零序数 ----------------------------------------------------------

func TestELIMV2SectionHeadZeroOrdinal(t *testing.T) {
	ordinal := regexp.MustCompile(`#\d`)
	for _, zh := range []bool{true, false} {
		_, fence := elimRenderOverview(t, elimv2DirectionBoardProjection(), zh)
		for _, head := range elimv2SectionHeads(fence) {
			if ordinal.MatchString(head) {
				t.Fatalf("pin⑨: a ▸ section head must never carry an ordinal (zh=%v): %q", zh, head)
			}
			for _, badge := range tracefence.BadgeGlyphs() {
				if strings.Contains(head, badge) {
					t.Fatalf("pin⑨: a ▸ section head must never wear a badge: %q", head)
				}
			}
		}
		// Direction words never compose with an ordinal anywhere on the fence
		// (§29.132 护栏① ◎ half).
		if regexp.MustCompile(`(调度供给|锁与优先级|IO与依赖|频率与热治理|自身工作量|scheduling supply|fix-direction)[^\n]*#\d`).MatchString(fence) {
			t.Fatalf("pin⑨: a direction word composes with an ordinal (zh=%v):\n%s", zh, fence)
		}
	}
}

// --- pin⑬ width discipline over the new structural lines --------------------------

func TestELIMV2NewLineFamiliesWidth(t *testing.T) {
	shapes := []types.TraceCausalProjection{
		elimv2DirectionBoardProjection(),
		elimBoardProjection(),
		elimGapCutBoardProjection(),
	}
	for _, projection := range shapes {
		for _, zh := range []bool{true, false} {
			_, fence := elimRenderOverview(t, projection, zh)
			for _, line := range strings.Split(fence, "\n") {
				structural := strings.HasPrefix(line, tracefence.ElimSectionGlyph+" ") ||
					strings.HasPrefix(line, "◇ 邻近(") || strings.HasPrefix(line, "◇ adjacent (") ||
					strings.HasPrefix(line, tracefence.ElimGlyph+" ")
				if structural && runewidth.StringWidth(line) > runtimeTraceProjTreeRowMaxWidth {
					t.Fatalf("pin⑬: structural line exceeds the %d-cell cap (%d): %q",
						runtimeTraceProjTreeRowMaxWidth, runewidth.StringWidth(line), line)
				}
			}
		}
	}
}

// --- 恒等式新结构: the elimgap closure walker stays green on the sectioned board --

func TestELIMV2BoardAccountingClosureOnSections(t *testing.T) {
	elimGapAssertBoardAccounting(t, elimv2DirectionBoardProjection())
	// The cut shape keeps closing under sections too (rendered + cut ==
	// population, per channel — section heads never count as members).
	elimGapAssertBoardAccounting(t, elimGapCutBoardProjection())
}

// pin⑬ HTML face: the ▸ section heads join the stanza-head decoration family
// on the preview grid (textContent untouched — decoration only).
func TestELIMV2SectionHeadPreviewStanzaClass(t *testing.T) {
	_, fence := elimRenderOverview(t, elimv2DirectionBoardProjection(), true)
	html, err := preview.RenderMarkdownHTML([]byte(fence + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `trace-stanza-head`) {
		t.Fatalf("the ◎/▸ heads must wear the stanza-head class:\n%s", html)
	}
	headAt := strings.Index(html, "▸")
	if headAt < 0 {
		t.Fatalf("the ▸ head must survive to the HTML face:\n%s", html)
	}
	lineSpanAt := strings.LastIndex(html[:headAt], `<span class="trace-line`)
	if lineSpanAt < 0 || !strings.Contains(html[lineSpanAt:headAt], "trace-stanza-head") {
		t.Fatalf("the ▸ line's enclosing span must carry trace-stanza-head:\n%s", html[lineSpanAt:headAt+30])
	}
}

// 三问② sanity: the printed subtotal wears its caliber word at the point of
// reading (小计 X ms(区间互斥) — never a bare number).
func TestELIMV2SubtotalAlwaysWearsCaliber(t *testing.T) {
	_, fence := elimRenderOverview(t, elimv2DirectionBoardProjection(), true)
	re := regexp.MustCompile(`小计 [\d.]+ms(?:\(区间互斥\))?`)
	for _, match := range re.FindAllStringSubmatch(fence, -1) {
		if !strings.Contains(match[0], "(区间互斥)") {
			t.Fatalf("三问②: a subtotal without its caliber word: %q\n%s", match[0], fence)
		}
	}
}
