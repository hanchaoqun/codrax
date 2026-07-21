package tool

// answer_document_projection_elimgap_test.go — ELIM-GAP-FIX pins (§29.104.15,
// customer witness customlogs/cust_total_del.txt, 2026-07-16): the ◎ 窗内可
// 消除量总览 silently lost 根因排序#2 (E15(+2) 8.211ms, the R1 same-fact
// absorb Rank backfill riding an ×N merged chain row) plus every other
// TOP5-value-cut rank seat outside the semantic class (6 of 15 seats gone
// with zero disclosure).
//
//	件A  fourth 种群臂 carriage — bare Node.Rank (the badge/lead/成因门
//	     precise signal) admits every Rank carrier; witness E15 form enters
//	     as the second ⛓ bar. Structure pin: every valid-seat row is a
//	     population member and either board-rendered or footnote-disclosed
//	     (病族级不变量 — kills future carriers).
//	件B  per-channel cut-count disclosure (「⛓/◇ 持值行另有 N 行未入榜」—
//	     §29.112 P3③ word unification: the cut footnote speaks the ◎
//	     legend's population word 持值行, never a second word family)
//	     generalizing the RNB-2 件4 semantic-only count; zero cut → zero
//	     footnote; 不双计 (semantic cut seats count once, in their channel).
//	件C  overflow-fold word faces read the typed member truth
//	     (MergedAllDataGap) instead of the inherited seed predicate: mixed
//	     folds drop the 窗内无调度数据·链止 word / ◌ glyph / 数据盲区 detail
//	     word; pure-gap folds keep all three byte-identically.
//	件D  C5-guarded typed-producer rows (inversion gated / running deficit)
//	     whose eff sits above their own window projection wear the
//	     (发生段账目) caliber short word (witness E16/E18/E19).
//
// MUTATION self-checks (cp-copy recovery only): dropping the 件A arm reds
// TestElimGapRankCarriageSeatEntersOverview + every accounting walker call;
// dropping a 件B channel line reds the walker's closed accounting identity;
// re-reading the fold seed predicate reds TestElimGapMixedFoldWordFace.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// elimGapE15WitnessNode is the customer witness E15(+2) post-compile shape:
// the wakeup-chain view row that R1-absorbed its root_cause_secondary rank
// record (§29.67 seat backfill — survivor keeps the hop predicate, adopts
// Rank=2) and then R2-merged two same-(subject,object) occurrences
// (MergedCount=3; RNB fold refuses merged hosts, so RankFoldPeers stays
// empty). Carriages (a)-(c) of the 种群臂 all reject this form — only the
// 件A bare-Rank arm admits it.
func elimGapE15WitnessNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role:       types.TraceCausalRoleRootCauseContext,
		EvidenceID: "E-cold9",
		Subject:    "[GT]ColdPool#9-48667",
		Predicate:  "wakeup_causal_impact",
		Object:     "runnable",
		StateKind:  "runnable",
		Rank:       2, Tier: "secondary",
		MergedCount: 3, MergedMinMS: 0.412, MergedMaxMS: 8.211,
		ImpactMS: 8.211, CumulativeImpactMS: 10.061, EffectiveImpactMS: 8.211,
		EffectiveImpactPublished:   true,
		SecondaryObjects:           []string{"priority_inversion_candidate"},
		PriorityInversionCandidate: true,
		ChainRelevance:             "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: 100, LineEnd: 400, Confidence: 0.62,
	}
}

func elimGapE15WitnessProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"[GT]ColdPool#1-48598", "com.tencent.mm-48517"},
		WindowStartTs:           925.410, WindowEndTs: 925.610,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-cold1", "[GT]ColdPool#1-48598", "priority_inversion_candidate", "runnable", 1, 10.853, 100),
			elimGapE15WitnessNode(),
		},
	}
}

// elimGapCutBoardProjection is the witness 15-seat cut shape reduced: five
// larger chain rank rows fill TOP5, four ◇ adjacent rank rows leave three
// value-cut (only the ◇-max fallback renders), and one more chain rank row
// is value-cut too.
func elimGapCutBoardProjection() types.TraceCausalProjection {
	adjacent := func(id, subject string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
		node := elimChainNode(id, subject, "runnable_wait", "runnable", rank, eff, line)
		node.Predicate = "root_cause_tertiary"
		node.ChainRelevance = "adjacent"
		node.Causality = "adjacent_to_wakeup_chain"
		return node
	}
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"[GT]ColdPool#1-48598", "com.tencent.mm-48517"},
		WindowStartTs:           925.410, WindowEndTs: 925.610,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-c1", "chain-a-1", "runnable_wait", "runnable", 1, 10.853, 100),
			elimChainNode("E-c2", "chain-b-2", "d_state_or_io_wait", "d_sleep", 2, 2.939, 110),
			elimChainNode("E-c3", "chain-c-3", "runnable_wait", "runnable", 3, 2.825, 120),
			elimChainNode("E-c4", "chain-d-4", "runnable_wait", "runnable", 4, 2.350, 130),
			elimChainNode("E-c5", "chain-e-5", "d_state_or_io_wait", "d_sleep", 5, 1.962, 140),
			elimChainNode("E-c6", "chain-f-6", "runnable_wait", "runnable", 6, 1.131, 150),
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			adjacent("E-a1", "adj-cold1", 1, 2.540, 200),
			adjacent("E-a2", "adj-cold2", 2, 1.699, 210),
			adjacent("E-a3", "adj-cold3", 3, 0.728, 220),
			adjacent("E-a4", "adj-cold4", 4, 0.441, 230),
		},
	}
}

// elimGapAssertBoardAccounting is the 件A structure pin (病族级不变量): over
// one rendered model,
//
//	(1) carrier closure — every row the §29.30.1 valid-seat gate passes IS a
//	    ◎ population member (the 种群臂 recognizes every Rank carrier the
//	    badge/lead authority recognizes; a future carrier that seats on the
//	    tree face without entering the population reds here);
//	(2) disclosure closure — a valid-seat population row kept OFF the board
//	    by the caliber/tier arms must be NAMED in a disclosure footnote
//	    (排除≠消失);
//	(3) closed accounting identity — rendered member lines + the 件B
//	    per-channel cut counts == the full board population, per channel
//	    (静默消失=0 by arithmetic, not by inspection).
func elimGapAssertBoardAccounting(t *testing.T, projection types.TraceCausalProjection) {
	t.Helper()
	model, fence := elimRenderOverview(t, projection, true)
	board := runtimeTraceProjElimBoard(model)
	// (1)+(2) carrier and disclosure closure.
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for _, row := range rows {
			if _, ok := runtimeTraceProjRowValidSeat(row); !ok {
				continue
			}
			if !runtimeTraceProjElimRankItemRow(row) {
				t.Fatalf("carrier closure: valid-seat row must be a ◎ population member (Rank=%d peers=%d predicate=%q subject=%q)",
					row.Node.Rank, len(row.RankFoldPeers), row.Node.Predicate, row.Node.Subject)
			}
			if !runtimeTraceProjElimEligible(row) {
				tag := strings.TrimSpace(row.EvidenceTag)
				if tag == "" || !strings.Contains(fence, "["+tag+"]") {
					t.Fatalf("disclosure closure: board-excluded valid-seat row [%s] must be named in a footnote:\n%s", tag, fence)
				}
			}
		}
	}
	if len(board) == 0 {
		return
	}
	// (3) closed accounting identity.
	boardByChannel := map[int]int{}
	for _, entry := range board {
		boardByChannel[entry.channelRank]++
	}
	// OMGCLEAN-1 件8 (2026-07-20). EVOLUTION RECORD: with the per-row channel
	// word stripped, the channel of a bar row is its ZONE — bar rows before
	// the ◇ zone head are chain members, after it adjacent members.
	renderedByChannel := map[int]int{}
	inAdjacentZone := false
	for _, line := range strings.Split(fence, "\n") {
		if strings.HasPrefix(line, "◇ 邻近(") || strings.HasPrefix(line, "◇ adjacent (") {
			inAdjacentZone = true
			continue
		}
		if !strings.Contains(line, "█") && !strings.Contains(line, "░") {
			continue
		}
		if inAdjacentZone {
			renderedByChannel[1]++
		} else {
			renderedByChannel[0]++
		}
	}
	// 件4/件9: both per-channel cut counts live on the ONE 未入榜 aux row.
	cutByChannel := map[int]int{}
	for _, line := range strings.Split(fence, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "· 未入榜 ") || strings.HasPrefix(trimmed, "· 未入榜最大") {
			continue
		}
		var n int
		if i := strings.Index(trimmed, "⛓ "); i >= 0 {
			if _, err := fmt.Sscanf(trimmed[i:], "⛓ %d 行", &n); err == nil {
				cutByChannel[0] = n
			}
		}
		if i := strings.Index(trimmed, "◇ "); i >= 0 {
			if _, err := fmt.Sscanf(trimmed[i:], "◇ %d 行", &n); err == nil {
				cutByChannel[1] = n
			}
		}
	}
	for channel, total := range boardByChannel {
		if renderedByChannel[channel]+cutByChannel[channel] != total {
			t.Fatalf("closed accounting: channel %d rendered %d + cut %d != board %d (静默消失≠0):\n%s",
				channel, renderedByChannel[channel], cutByChannel[channel], total, fence)
		}
	}
	elimGapAssertRepresentedLane(t, model, fence)
}

// elimGapAssertRepresentedLane — 裁定① lane of the closure identity
// (§29.104.17 ①, GATED-CAL 件3, 2026-07-16): every row the represented-
// satellite arm alone keeps out of the ◎ population is counted in the
// dedicated footnote AND named by its [E#] there (排除≠消失 extends to the
// new arm: rendered + cut counts + represented == the pre-exclusion
// population, by arithmetic). Zero represented rows ⟺ zero footnote.
func elimGapAssertRepresentedLane(t *testing.T, model runtimeTraceProjTreeModel, fence string) {
	t.Helper()
	census := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if !runtimeTraceProjElimRepresentedExcluded(rows[i]) {
				continue
			}
			census++
			if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" && !strings.Contains(fence, "["+tag+"]") {
				t.Fatalf("represented lane: excluded row [%s] must be named in the disclosure footnote:\n%s", tag, fence)
			}
		}
	}
	disclosed := -1
	for _, line := range strings.Split(fence, "\n") {
		var n int
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "· 已由链上席代表") {
			continue
		}
		if _, err := fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(trimmed, "· 已由链上席代表")), "%d 行(整席降道),见明细", &n); err == nil {
			disclosed = n
		}
		// 修补轮 件D② (冷读 P3⑥, 2026-07-17): the EN face counts too — an
		// EN render's footnote participates in the same closure identity.
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "· represented by the on-chain seat (whole-seat demotion): %d row(s)", &n); err == nil {
			disclosed = n
		}
	}
	switch {
	case census == 0 && disclosed != -1:
		t.Fatalf("represented lane: zero excluded rows must render zero footnote:\n%s", fence)
	case census > 0 && disclosed != census:
		t.Fatalf("represented lane: footnote count %d != excluded census %d (closure identity):\n%s", disclosed, census, fence)
	}
}

// --- 件A: the witness carriage enters; the structure pin holds -------------------

// TestElimGapRankCarriageSeatEntersOverview — the customer E15(+2) form: the
// tree/badge faces wear 根因排序#2 and the SAME row now holds the second ⛓
// board bar (8.211 verbatim) instead of vanishing silently.
func TestElimGapRankCarriageSeatEntersOverview(t *testing.T) {
	projection := elimGapE15WitnessProjection()
	model, fence := elimRenderOverview(t, projection, true)
	tree := runtimeTraceProjTreeFence(model, true)

	var cold9 *runtimeTraceProjTreeRow
	for i := range model.TreeRows {
		if strings.Contains(model.TreeRows[i].Node.Subject, "ColdPool#9") {
			cold9 = &model.TreeRows[i]
			break
		}
	}
	if cold9 == nil {
		t.Fatalf("witness row missing from the tree model; tree:\n%s", tree)
	}
	if seat, ok := runtimeTraceProjRowValidSeat(*cold9); !ok || seat != 2 {
		t.Fatalf("fixture: the badge/lead gate must seat the row at #2, got (%d,%v)", seat, ok)
	}
	if !strings.Contains(tree, "根因排序#2") {
		t.Fatalf("tree face must keep the seat chip:\n%s", tree)
	}
	// 件A: all four carriage arms agree with the seat gate on this form.
	if !runtimeTraceProjElimRankItemRow(*cold9) || !runtimeTraceProjElimEligible(*cold9) {
		t.Fatalf("件A: the bare-Rank carriage must enter the ◎ population")
	}
	members := elimOverviewMemberLines(fence)
	if len(members) != 2 {
		t.Fatalf("both chain seats must render, got %d:\n%s", len(members), fence)
	}
	if !strings.Contains(members[0], "10.853ms") || !strings.Contains(members[1], "8.211ms") ||
		!strings.Contains(members[1], "ColdPool#9") {
		t.Fatalf("the witness seat must be the second ⛓ bar with its verbatim value:\n%s", fence)
	}
	elimGapAssertBoardAccounting(t, projection)
}

// TestElimGapCarriageFamilySweep — the 种群臂 admits every rendered Rank
// carriage (predicate seat / RNB fold peer / bare R1-backfill Rank / semantic
// adopted seat) and still rejects the honest non-population forms.
func TestElimGapCarriageFamilySweep(t *testing.T) {
	base := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
		Node: elimChainNode("E-p", "w-1", "runnable_wait", "runnable", 1, 5.0, 10)}
	if !runtimeTraceProjElimRankItemRow(base) {
		t.Fatalf("carriage (a): the root_cause_ predicate seat must enter")
	}
	folded := base
	folded.Node.Predicate = "wakeup_causal_impact"
	folded.Node.Rank = 0
	folded.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 3, Confidence: 0.7}}
	if !runtimeTraceProjElimRankItemRow(folded) {
		t.Fatalf("carriage (b): the RNB fold-peer carriage must enter")
	}
	backfill := base
	backfill.Node.Predicate = "wakeup_causal_impact"
	backfill.Node.Rank = 2
	backfill.Node.MergedCount = 3
	if !runtimeTraceProjElimRankItemRow(backfill) {
		t.Fatalf("carriage (d) 件A: the bare-Rank R1-backfill carriage must enter")
	}
	adopted := base
	adopted.Kind = runtimeTraceProjTreeRowSemantic
	adopted.Node.Predicate = "trace_semantic_span"
	adopted.Node.SemanticClass = "class_verification"
	adopted.Node.Rank = 0
	adopted.Node.BackgroundRank = 4
	if !runtimeTraceProjElimRankItemRow(adopted) {
		t.Fatalf("carriage (c): the semantic adopted-seat carriage must enter")
	}
	// Honest non-population forms stay OUT (fail-honest, zero minting).
	hop := base
	hop.Node.Predicate = "wakeup_causal_impact"
	hop.Node.Rank = 0
	if runtimeTraceProjElimRankItemRow(hop) {
		t.Fatalf("a bare hop row (no rank identity on any carriage) must stay out")
	}
	seatless := adopted
	seatless.Node.BackgroundRank = 0
	if runtimeTraceProjElimRankItemRow(seatless) {
		t.Fatalf("a seatless ✦ semantic row must stay out")
	}
}

// --- 件B: per-channel cut-count disclosure ---------------------------------------

// TestElimGapPerChannelCutCounts — the witness cut shape: one non-semantic ⛓
// seat and three ◇ seats fall past TOP5 — each channel gets its counted line
// (formerly: ⛓ non-semantic and ◇ cuts were silent); the accounting identity
// closes; en mirror rides the same lane.
func TestElimGapPerChannelCutCounts(t *testing.T) {
	projection := elimGapCutBoardProjection()
	_, fence := elimRenderOverview(t, projection, true)
	// OMGCLEAN-1 件4/件9 (§29.175.9/.14, 2026-07-20). EVOLUTION RECORD: the ◇
	// zone renders its own TOP3 (2.540/1.699/0.728 now render; only 0.441
	// stays cut), the per-channel counts merge into ONE 未入榜 aux row, and
	// the ◇ zone carries its own tail count (展示外全额).
	if !strings.Contains(fence, "· 未入榜") || !strings.Contains(fence, "⛓ 1 行") {
		t.Fatalf("件B: the ⛓ non-semantic cut seat must be counted:\n%s", fence)
	}
	if !strings.Contains(fence, "◇ 1 行,见明细") {
		t.Fatalf("件B: the ◇ cut seat must be counted:\n%s", fence)
	}
	if !strings.Contains(fence, "  另有 1 行见明细") {
		t.Fatalf("§29.175.9: the ◇ zone must carry its own tail count:\n%s", fence)
	}
	// The ◇ TOP3 renders; the remainder stays value-cut.
	for _, want := range []string{"2.540ms", "1.699ms", "0.728ms"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("the ◇ TOP3 must render %s:\n%s", want, fence)
		}
	}
	for _, cut := range []string{"0.441", "1.131"} {
		for _, line := range elimOverviewMemberLines(fence) {
			if strings.Contains(line, cut) {
				t.Fatalf("fixture: %s must be value-cut, not rendered:\n%s", cut, fence)
			}
		}
	}
	elimGapAssertBoardAccounting(t, projection)
	// en mirror.
	_, fenceEN := elimRenderOverview(t, projection, false)
	if !strings.Contains(fenceEN, "· unranked") ||
		!strings.Contains(fenceEN, "⛓ 1 row(s) · ◇ 1 row(s) — see the detail table") {
		t.Fatalf("en cut counts missing:\n%s", fenceEN)
	}
}

// TestElimGapTopNWordFaceMatchesConstant — §29.112 P3④ (DISPLAY-HYG 二轮):
// the cut-footnote word face spells the population bound as a LITERAL
// ("TOP5 值切" / "cut by TOP5") while the slice reads
// runtimeTraceProjElimTopN — this identity pin ties the two, so a K
// re-ruling (design R-c: badge TOP N and the overview bound move only
// together) cannot leave a stale literal on the report face. Two arms:
// the constant↔literal identity, and the RENDERED footnote carrying the
// constant-derived word (face-level, survives literal refactors).
func TestElimGapTopNWordFaceMatchesConstant(t *testing.T) {
	want := fmt.Sprintf("TOP%d", runtimeTraceProjElimTopN)
	if want != "TOP5" {
		t.Fatalf("§29.112 P3④: runtimeTraceProjElimTopN moved (now %d) — update every TOP5 word face (grep TOP5) and re-pin", runtimeTraceProjElimTopN)
	}
	// OMGCLEAN-1 件4 (§29.175.9, 2026-07-20). EVOLUTION RECORD: the rendered
	// 「TOP5 值切」/"cut by TOP5" literal RETIRED with the merged 未入榜 aux
	// row (◇ counts follow the zone's own TOP3 now, so a single TOP5 claim
	// would lie about the ◇ channel); the identity pin above still ties any
	// future K literal to the constant, and the negative arm below keeps the
	// stale literal off the face.
	_, fence := elimRenderOverview(t, elimGapCutBoardProjection(), true)
	if strings.Contains(fence, want+" 值切") {
		t.Fatalf("§29.175.9: the retired TOP5 值切 literal must stay off the face:\n%s", fence)
	}
	_, fenceEN := elimRenderOverview(t, elimGapCutBoardProjection(), false)
	if strings.Contains(fenceEN, "cut by "+want) {
		t.Fatalf("§29.175.9: the retired cut-by literal must stay off the EN face:\n%s", fenceEN)
	}
}

// TestElimGapZeroCutZeroFootnote — 零切席时零脚注: a board whose population
// fits TOP5 renders neither channel count line.
func TestElimGapZeroCutZeroFootnote(t *testing.T) {
	_, fence := elimRenderOverview(t, elimBoardProjection(), true)
	if strings.Contains(fence, "行另有") {
		t.Fatalf("no cut seats → no cut-count footnote:\n%s", fence)
	}
	elimGapAssertBoardAccounting(t, elimBoardProjection())
}

// TestElimGapSemanticCutCountsOnceInChannelLine — 不双计 pin (主会话选形):
// a cut SEMANTIC seat counts once, inside its channel line; the superseded
// 语义类持席行 wording never renders beside it. (The semantic FALLBACK SEAT
// mechanism — the appended largest off-board semantic member — is untouched
// and its seat is never counted as cut.)
func TestElimGapSemanticCutCountsOnceInChannelLine(t *testing.T) {
	projection := elimSemanticFallbackProjection(4.0)
	second := elimSemanticSeatedNode("E-sem2", "host-8", 8, 3.0, 170)
	second.SemanticClass = "jit_compile"
	second.TypeToken = "jit_compile"
	second.Object = "jit_compile"
	projection.SemanticSpans = append(projection.SemanticSpans, second)
	_, fence := elimRenderOverview(t, projection, true)
	// Cut set = the plain E-c6 5.5 chain row + the E-sem2 3.0 semantic member
	// (the 4.0 member renders as the semantic fallback seat) — ONE line, both
	// counted, semantic wording superseded.
	if !strings.Contains(fence, "· 未入榜") || !strings.Contains(fence, "⛓ 2 行,见明细") {
		t.Fatalf("the channel line must count plain and semantic cut seats together:\n%s", fence)
	}
	if strings.Contains(fence, "语义类持席行") {
		t.Fatalf("不双计: the semantic-only count line must not render:\n%s", fence)
	}
	if strings.Count(fence, "· 未入榜 ") != 1 {
		t.Fatalf("exactly one ⛓ cut-count line:\n%s", fence)
	}
	elimGapAssertBoardAccounting(t, projection)
}

// --- 件C: fold word faces read the typed member truth ----------------------------

func elimGapMixedFoldNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role:                types.TraceCausalRoleRootCauseContext,
		EvidenceID:          "E-fold",
		Predicate:           "trace_gap", // inherited verbatim from overflow[0]
		ChainRelevance:      "on_chain",
		OnChainOverflowFold: true,
		MergedCount:         3, MergedValuelessCount: 1,
		MergedMinMS: 10.822, MergedMaxMS: 24.000,
		CumulativeImpactMS: 24.000,
		MergedAllDataGap:   false,
		MergedSubjects:     []string{"OS_IPC_5_2838-2838", "RenderThread-48660", "[GT]ColdPool#1-48598"},
	}
}

// TestElimGapMixedFoldWordFace — the witness E22(+2) form (seed member =
// trace_gap row, 2 valued members, one holding 榜位#12): the fold's word
// faces stop reading the inherited seed predicate —
//
//	tag:    no 窗内无调度数据·链止 (the ×N valued-split accounting stays);
//	glyph:  neutral fold form, never ◌ (§24.3 记号位 carries true information);
//	detail: family word never claims 数据盲区.
//
// The PURE-gap fold keeps all three faces byte-identically (negative arm).
func TestElimGapMixedFoldWordFace(t *testing.T) {
	mixed := elimGapMixedFoldNode()
	if runtimeTraceProjTraceGapNode(mixed) {
		t.Fatalf("件C: a mixed fold must not classify trace_gap off the seed predicate")
	}
	row := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain, Node: mixed}
	_, tags := runtimeTraceProjRowMetricParts(row, 199.992, true, true)
	for _, tag := range tags {
		if strings.Contains(tag.Text, "窗内无调度数据") {
			t.Fatalf("件C: the mixed fold must not wear the blind-spot word: %q", tag.Text)
		}
	}
	if form := runtimeTraceProjImpactFormForNode(mixed, runtimeTraceProjTreeRowChain); form == runtimeTraceProjImpactFormBlindSpot {
		t.Fatalf("件C: the mixed fold must not resolve the ◌ blind-spot form")
	}
	if word := runtimeTraceProjImpactFormFamilyWord(mixed, true); strings.Contains(word, "数据盲区") {
		t.Fatalf("件C: the mixed fold's detail family word must not claim 数据盲区: %q", word)
	}
	// Negative arm: the pure-gap fold keeps every blind-spot face.
	pure := elimGapMixedFoldNode()
	pure.MergedValuelessCount = 3
	pure.MergedMinMS, pure.MergedMaxMS, pure.CumulativeImpactMS = 0, 0, 0
	pure.MergedAllDataGap = true
	if !runtimeTraceProjTraceGapNode(pure) {
		t.Fatalf("件C negative: the pure-gap fold keeps the trace_gap classification")
	}
	if form := runtimeTraceProjImpactFormForNode(pure, runtimeTraceProjTreeRowChain); form != runtimeTraceProjImpactFormBlindSpot {
		t.Fatalf("件C negative: the pure-gap fold keeps the ◌ form, got %v", form)
	}
	if word := runtimeTraceProjImpactFormFamilyWord(pure, true); !strings.Contains(word, "数据盲区") {
		t.Fatalf("件C negative: the pure-gap fold keeps the 数据盲区 family word: %q", word)
	}
	// A STANDALONE trace_gap row (non-fold) keeps the inline disclosure
	// byte-identically (the F5 用户措辞裁定 wording is untouched).
	standalone := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
		Node: types.TraceCausalProjectionNode{
			EvidenceID: "E-gap", Subject: "OS_IPC_5_2838-2838",
			Predicate: "trace_gap", Object: "trace_gap", TypeToken: "trace_gap",
			ChainRelevance: "on_chain", LineStart: 900, LineEnd: 910, Confidence: 0.6,
		}}
	_, gapTags := runtimeTraceProjRowMetricParts(standalone, 199.992, true, true)
	found := false
	for _, tag := range gapTags {
		if tag.Text == "窗内无调度数据·链止" {
			found = true
		}
	}
	if !found {
		t.Fatalf("件C negative: the standalone trace_gap row keeps its inline disclosure: %+v", gapTags)
	}
}

// --- 件D: the occurrence-segment account caliber word ----------------------------

// elimGapE16WitnessNode — the witness E16(+1) form: an inversion gated
// composite whose published eff (7.510) sits above its own window projection
// (7.486); the C5 typed-producer guard keeps it OFF the 承自归因 lane, and
// its gated split does NOT balance against eff at print precision, so the
// 行3 breakdown refuses (拒渲绝不造数) and the effective ships through the
// bare Q1 tag — exactly the witness's wordless band.
func elimGapE16WitnessNode() types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role:        types.TraceCausalRoleRootCauseContext,
		EvidenceID:  "E-cold1b",
		Subject:     "[GT]ColdPool#1-48598",
		Predicate:   "wakeup_causal_impact",
		Object:      "runnable",
		StateKind:   "runnable",
		TypeToken:   "priority_inversion_candidate",
		MergedCount: 2, MergedMinMS: 3.336, MergedMaxMS: 7.486,
		ImpactMS: 7.486, CumulativeImpactMS: 7.486, EffectiveImpactMS: 7.510,
		PriorityInversionCandidate: true,
		GatedRunnableMS:            0.289,
		ChainRelevance:             "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: 500, LineEnd: 540, Confidence: 0.62,
	}
}

// TestElimGapOccurrenceSegmentCaliberWord — 件D: the E16/E18/E19 band wears
// (发生段账目) beside the 有效归因 value; print-equal and eff<cum forms stay
// wordless byte-identically; the running-deficit twin rides the same lane.
func TestElimGapOccurrenceSegmentCaliberWord(t *testing.T) {
	node := elimGapE16WitnessNode()
	if runtimeTraceProjEffectiveInherited(node) {
		t.Fatalf("fixture: the C5 typed-producer guard must keep the row off the 承自 lane")
	}
	word, ok := runtimeTraceProjOccurrenceSegmentAccountWord(node, true)
	if !ok || word != "(发生段账目)" {
		t.Fatalf("件D: the guarded eff>cum row must mint the caliber word, got %q ok=%v", word, ok)
	}
	if wordEN, okEN := runtimeTraceProjOccurrenceSegmentAccountWord(node, false); !okEN ||
		wordEN != " (occurrence-segment account)" {
		t.Fatalf("件D en face: got %q ok=%v", wordEN, okEN)
	}
	row := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain, Node: node}
	_, tags := runtimeTraceProjRowMetricParts(row, 199.992, true, true)
	found := ""
	for _, tag := range tags {
		if strings.Contains(tag.Text, "有效归因") {
			found = tag.Text
		}
	}
	if !strings.Contains(found, "有效归因 7.510ms(发生段账目)") {
		t.Fatalf("件D: the inline tag must carry the caliber word, got %q (all: %+v)", found, tags)
	}
	// Negative arms: print-equal eff==cum stays wordless; eff<cum stays
	// wordless; an unguarded row (no typed producer) is the 承自 lane's
	// business and never reaches this word.
	equal := node
	equal.EffectiveImpactMS = 7.486
	if _, ok := runtimeTraceProjOccurrenceSegmentAccountWord(equal, true); ok {
		t.Fatalf("件D negative: print-equal pairs stay wordless")
	}
	below := node
	below.EffectiveImpactMS = 5.0
	if _, ok := runtimeTraceProjOccurrenceSegmentAccountWord(below, true); ok {
		t.Fatalf("件D negative: eff<cum rows stay wordless")
	}
	plain := node
	plain.PriorityInversionCandidate = false
	plain.TypeToken = "runnable_wait"
	plain.GatedRunnableMS = 0
	if _, ok := runtimeTraceProjOccurrenceSegmentAccountWord(plain, true); ok {
		t.Fatalf("件D negative: an unguarded row never mints the word (承自 lane owns it)")
	}
	if !runtimeTraceProjEffectiveInherited(plain) {
		t.Fatalf("fixture: the unguarded eff>cum row belongs to the 承自 lane")
	}
	// Running-deficit twin (the second C5 guard arm).
	deficit := types.TraceCausalProjectionNode{
		EvidenceID: "E-run", Subject: "worker-1", StateKind: "running",
		Object: "running", TypeToken: "running",
		ImpactMS: 3.860, CumulativeImpactMS: 3.100, EffectiveImpactMS: 3.175,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 3.175,
		SupplyFoldIdealMS: 0.685, SupplyFoldKnownMS: 3.860,
		ChainRelevance: "on_chain", LineStart: 600, LineEnd: 620, Confidence: 0.7,
	}
	if word, ok := runtimeTraceProjOccurrenceSegmentAccountWord(deficit, true); !ok || word != "(发生段账目)" {
		t.Fatalf("件D: the running-deficit arm must ride the same lane, got %q ok=%v", word, ok)
	}
}

// elimGapOccurrenceAccountProjection is the representative legend fixture for
// the 件D mark: the E15 witness board plus a running-deficit C5-arm row whose
// eff sits above its own window projection (the SECOND guard arm — its supply
// caliber words render on the fence itself, so every co-lit mark stays
// fence-visible for the bidirectional sweep; the inversion-arm twin keeps its
// dedicated pins below), plus the witness E20 D-state seat so the ⛓ icon
// lights its own legend entry beside the ◎ channel glyph (the two ⛓ senses
// share one byte).
func elimGapOccurrenceAccountProjection() types.TraceCausalProjection {
	projection := elimGapE15WitnessProjection()
	// The representative sweep probes the MergedSum mark on its CANONICAL
	// verbatim token (case3d4 precedent) — the legend fixture re-values the
	// witness merged range onto it; the dedicated pins keep witness values.
	projection.OnChainCauses[1].MergedMinMS = 10.000
	projection.OnChainCauses[1].MergedMaxMS = 30.000
	deficit := types.TraceCausalProjectionNode{
		Role:       types.TraceCausalRoleRootCauseContext,
		EvidenceID: "E-run", Subject: "worker-1", StateKind: "running",
		Object: "running", TypeToken: "running",
		ImpactMS: 3.860, CumulativeImpactMS: 3.100, EffectiveImpactMS: 3.175,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 3.175,
		SupplyFoldIdealMS: 0.685, SupplyFoldKnownMS: 3.860,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: 600, LineEnd: 620, Confidence: 0.7,
	}
	projection.OnChainCauses = append(projection.OnChainCauses,
		deficit,
		elimChainNode("E-rt", "RenderThread-48660", "d_state_or_io_wait", "d_sleep", 3, 2.939, 700))
	return projection
}

// TestElimGapOccurrenceSegmentLegendLockstep — 词条-图例双向: the word's
// on-demand legend entry renders exactly when the word does.
func TestElimGapOccurrenceSegmentLegendLockstep(t *testing.T) {
	projection := elimGapE15WitnessProjection()
	projection.OnChainCauses = append(projection.OnChainCauses, elimGapE16WitnessNode())
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(tree, "(发生段账目)") {
		t.Fatalf("the E16 row must wear the caliber word on the tree face:\n%s", tree)
	}
	if !model.Marks.has(runtimeTraceProjMarkOccurrenceSegmentAccount) {
		t.Fatalf("the word must light its legend mark")
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`(发生段账目)`") {
		t.Fatalf("the word's legend entry must render with it:\n%s", lead)
	}
	// DISPLAY-HYG 二轮 (§29.112 P3②): the entry never re-grows an unbacked
	// magnitude adverb — the producer gate is eff>cum with NO upper bound, so
	// 「略高于」-class smallness claims are banned; the entry states direction
	// plus the two-column difference reference instead.
	if strings.Contains(lead, "略高于") {
		t.Fatalf("§29.112 P3② pin: the (发生段账目) legend entry must not claim 「略高于」 (no typed upper bound):\n%s", lead)
	}
	if !strings.Contains(lead, "可高于窗口投影列的落窗裁剪值(高出幅度以两列实值之差为准,不设固定上界)") {
		t.Fatalf("§29.112 P3② pin: the entry states direction + printed-difference reference:\n%s", lead)
	}
	// Negative half: a board without the word renders no entry.
	bare := elimGapE15WitnessProjection()
	bareModel := buildRuntimeTraceProjTreeModel(bare, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	runtimeTraceProjTreeFence(bareModel, true)
	if bareLead := runtimeTraceProjLeadText(bare, bareModel, "zh", true); strings.Contains(bareLead, "发生段账目") {
		t.Fatalf("no word → no legend entry:\n%s", bareLead)
	}
}
