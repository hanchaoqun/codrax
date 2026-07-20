package tool

// answer_document_projection_elim_test.go — ELIM-1 「◎ 窗内可消除量总览」 pins
// (rank_order_v2_design_20260712.md GREENLIT 2026-07-12, R16 六款 + §9 pin
// plan; RANK-U Stage 2 commits C+D, 2026-07-13).
//
// MUTATION self-checks (cp-copy recovery only, never git checkout):
//   - M-A drop the channel arm (admit ▒) → TestElimGateChannelArm red
//     (measured: exactly this one — TestElimOverviewE30BackgroundNeverEnters
//     stays green against THIS vector because the board collector never
//     scans model.Background; the E30 pin guards the OTHER edit class, a
//     future "collect the Background bucket" change — defense in depth,
//     each pin owns its own vector);
//   - M-B drop the caliber/value-shape arm → TestElimGateCaliberArm red
//     (count row joins the ranking);
//   - M-C sort by eff×confidence → TestElimBoardPureEffOrder red;
//   - M-D print a seat ordinal/badge in the overview →
//     TestElimOverviewZeroOrdinalZeroBadge red;
//   - M-E weaken the B1 shared arm (drop eff>0) → the existing badge/lead
//     valid-seat pins red (shared implementation is the point).

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// elimChainNode builds a chain-channel rank node (typed root_cause_ predicate
// — the rank-item population lane).
func elimChainNode(id, subject, typeToken, state string, rank int, eff float64, line int) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleRootCauseContext, EvidenceID: id,
		Subject: subject, Object: typeToken, TypeToken: typeToken, StateKind: state,
		Predicate: "root_cause_" + rootCauseTierNameForTest(rank), Rank: rank, Tier: rootCauseTierNameForTest(rank),
		ImpactMS: eff, CumulativeImpactMS: eff, EffectiveImpactMS: eff,
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1,
		LineStart: line, LineEnd: line + 5, Confidence: 0.8,
	}
}

func rootCauseTierNameForTest(rank int) string {
	switch rank {
	case 1:
		return "primary"
	case 2:
		return "secondary"
	default:
		return "tertiary"
	}
}

// elimBoardProjection — the combined ELIM-1 gate/board shape: a resolved
// two-node chain, two chain-channel rank rows, ◇ adjacent rank rows (wall
// clock + a composite-score ⌗ form + a count ⌗ form), the target's own
// wait-symptom row, and the 4165/E30 ▒ magnitude-dominant background pair.
// RootCauseFamilyObserved mirrors the production compile (rank family ran).
func elimBoardProjection() types.TraceCausalProjection {
	self := elimChainNode("E-self", "VSyncGenerator-2270", "binder_wait", "sleep", 0, 15.758, 300)
	self.Tier = types.TraceCausalTierTargetSelfState
	self.Predicate = "root_cause_target_self_state"
	self.Rank = 0
	return types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WakeupPath:              []string{"tppmgr-300", "VSyncGenerator-2270"},
		WindowStartTs:           2942.100, WindowEndTs: 2942.300,
		OnChainCauses: []types.TraceCausalProjectionNode{
			elimChainNode("E-run", "shadowhook-64305", "runnable_wait", "runnable", 1, 26.392, 100),
			elimChainNode("E-dst", "CompThread-2955", "d_state_or_io_wait", "d_sleep", 2, 13.006, 200),
			self,
		},
		AdjacentCauses: []types.TraceCausalProjectionNode{
			uxr1AdjacentNode("E-blk", "block_io_by_inode", "block_io_by_inode", 1, 0.198, 110),
			uxr1AdjacentNode("E-lat", "io_latency", "io_latency", 2, 0.710, 120),
			uxr1AdjacentNode("E-pgc", "page_cache_churn", "page_cache_churn", 3, 0.600, 130),
		},
		BackgroundCauses: []types.TraceCausalProjectionNode{
			uxr1StaleRankBackgroundNode(),
			uxr1BackgroundNode("E-bg2", "wc_srvinit_0-37254", "d_state_or_io_wait", "d_state_or_io_wait", "d_sleep", 54.608, 210),
		},
	}
}

func elimRenderOverview(t *testing.T, projection types.TraceCausalProjection, zh bool) (runtimeTraceProjTreeModel, string) {
	t.Helper()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
	runtimeTraceProjTreeFence(model, zh)
	return model, runtimeTraceProjElimOverviewFence(projection, model, zh)
}

// §29.61.12 ① (INV-SUPPLY 件④, 2026-07-14). EVOLUTION RECORD: the channel
// identity words gained their glyph-word space (`⛓链上` → `⛓ 链上` etc.) —
// this helper and every byte pin below moved in lockstep with the ONE
// emitter (runtimeTraceProjElimChannelWord). The header promise line also
// carries the ⛓ 链上 bytes now, so member scanning additionally requires the
// value/bar shape (a leading value cell) to keep matching member rows only.
func elimOverviewMemberLines(fence string) []string {
	var out []string
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "█") && !strings.Contains(line, "░") {
			continue // member rows always carry the bar cell; the header never does
		}
		if strings.Contains(line, "⛓ 链上") || strings.Contains(line, "◇ 邻近") ||
			strings.Contains(line, "⛓ on-chain") || strings.Contains(line, "◇ adjacent") {
			out = append(out, line)
		}
	}
	return out
}

// --- 准入四臂 (design §2.2, per-arm) -------------------------------------------

// TestElimGateSharedArm — B1: the overview admission consumes the SAME shared
// arm the badge/lead valid-seat gate reads (one implementation): no-data,
// overflow-fold, context_only, target_self_state and zero-eff rows are out on
// both faces.
func TestElimGateSharedArm(t *testing.T) {
	base := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
		Node: elimChainNode("E-x", "worker-1", "runnable_wait", "runnable", 1, 5.0, 10)}
	if !runtimeTraceProjRowSharedSeatArm(base) || !runtimeTraceProjElimEligible(base) {
		t.Fatalf("baseline chain rank row must pass the shared arm and the overview gate")
	}
	for name, mutate := range map[string]func(*runtimeTraceProjTreeRow){
		"no data":       func(r *runtimeTraceProjTreeRow) { r.HasData = false },
		"overflow fold": func(r *runtimeTraceProjTreeRow) { r.Node.OnChainOverflowFold = true },
		"context only":  func(r *runtimeTraceProjTreeRow) { r.Node.Tier = types.TraceCausalTierContextOnly },
		"self symptom":  func(r *runtimeTraceProjTreeRow) { r.Node.Tier = types.TraceCausalTierTargetSelfState },
		"zero eff":      func(r *runtimeTraceProjTreeRow) { r.Node.EffectiveImpactMS = 0 },
	} {
		row := base
		mutate(&row)
		if runtimeTraceProjRowSharedSeatArm(row) {
			t.Fatalf("%s: shared arm must reject", name)
		}
		if runtimeTraceProjElimEligible(row) {
			t.Fatalf("%s: overview gate must reject through the SAME shared arm", name)
		}
	}
	// data_gap belt (typed tier — diagnostics never become eliminable amounts).
	gap := base
	gap.Node.Tier = types.TraceCausalTierDataGap
	if runtimeTraceProjElimEligible(gap) {
		t.Fatalf("data_gap rows must never enter the overview")
	}
}

// TestElimGateRankItemArm — 种群臂: only typed rank-item rows enter; a ✦-form
// semantic row without an adopted seat and a bare hop row stay out (ADJ-MINT
// fail-honest — the overview never invents a population member).
func TestElimGateRankItemArm(t *testing.T) {
	hop := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
		Node: elimChainNode("E-h", "worker-2", "runnable_delay", "runnable", 0, 4.0, 20)}
	hop.Node.Predicate = "wakeup_causal_impact"
	if runtimeTraceProjElimEligible(hop) {
		t.Fatalf("a bare chain hop row is not a rank item and must stay out")
	}
	// The folded-twin carriage keeps the rank identity in the population.
	folded := hop
	folded.RankFoldPeers = []runtimeTraceProjRankFoldPeer{{Rank: 3, Confidence: 0.7}}
	if !runtimeTraceProjElimEligible(folded) {
		t.Fatalf("a chain row that folded its rank twin carries the rank identity and must enter")
	}
	// The semantic entity that ADOPTED its twin's seat enters; the ✦-only
	// mention-floor form (no adopted seat) stays out — and by the ✦ 不可达
	// theorem it could never displace a TOP5 member anyway: an on-chain
	// semantic row without a seat is outranked by ≥N seated chain rows whose
	// eff is not below it (design §2.5, structural proof recorded here).
	// 措辞补注 (RNB-2 件4, §29.88 R1, 2026-07-15): the theorem covers ONLY
	// seatless ✦ rows. A SEATED on-chain semantic member cut by TOP5 is a
	// different shape — it stays in the population and re-enters through the
	// chain semantic fallback seat (方案A, TestElimChainSemanticFallback*);
	// the fallback transcribes an existing member and contradicts nothing
	// here (the theorem never claimed seated rows unreachable).
	sem := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowSemantic,
		Node: elimChainNode("E-s", "worker-3", "class_verification", "", 0, 3.0, 30)}
	sem.Node.Predicate = "trace_semantic_span"
	sem.Node.SemanticClass = "class_verification"
	if runtimeTraceProjElimEligible(sem) {
		t.Fatalf("a seatless ✦ semantic row is not in the rank population")
	}
	adopted := sem
	adopted.Node.Rank = 2
	if !runtimeTraceProjElimEligible(adopted) {
		t.Fatalf("the semantic entity that adopted its rank twin's seat must enter")
	}
}

// TestElimGateChannelArm — 通道臂: chain and ◇ enter; ▒ never does, on either
// the typed relevance or the stanza placement authority.
func TestElimGateChannelArm(t *testing.T) {
	row := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowChain,
		Node: elimChainNode("E-c", "worker-4", "runnable_wait", "runnable", 1, 6.0, 40)}
	if !runtimeTraceProjElimEligible(row) {
		t.Fatalf("chain-channel rank row must enter")
	}
	adj := row
	adj.Kind = runtimeTraceProjTreeRowAdjacent
	adj.Node.ChainRelevance = "adjacent"
	if !runtimeTraceProjElimEligible(adj) {
		t.Fatalf("◇ adjacent rank row must enter (two same-ruler channels)")
	}
	bg := row
	bg.Kind = runtimeTraceProjTreeRowBackground
	bg.Node.ChainRelevance = "background"
	if runtimeTraceProjElimEligible(bg) {
		t.Fatalf("▒ background must never enter the overview (基石 C)")
	}
	// Stanza placement outranks a stale relevance (display channel authority).
	staleBg := row
	staleBg.Kind = runtimeTraceProjTreeRowBackground
	if runtimeTraceProjElimEligible(staleBg) {
		t.Fatalf("a ▒-seated row with stale on-chain relevance must still be out")
	}
}

// TestElimGateCaliberArm — 口径臂+值形臂: count-additivity and composite-score
// rows are ⌗ material on both the typed tier and the shared registry arm.
func TestElimGateCaliberArm(t *testing.T) {
	count := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowAdjacent,
		Node: uxr1AdjacentNode("E-pg", "page_cache_churn", "page_cache_churn", 1, 0.600, 50)}
	if runtimeTraceProjElimEligible(count) {
		t.Fatalf("count-additivity row must never join the wall-clock ranking (registry arm)")
	}
	composite := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowAdjacent,
		Node: uxr1AdjacentNode("E-bio", "block_io_by_inode", "block_io_by_inode", 2, 0.198, 60)}
	if runtimeTraceProjElimEligible(composite) {
		t.Fatalf("composite-score row must never join the wall-clock ranking (registry arm)")
	}
	tierOnly := runtimeTraceProjTreeRow{HasData: true, Kind: runtimeTraceProjTreeRowAdjacent,
		Node: uxr1AdjacentNode("E-cs", "worker-5", "io_latency", 3, 0.5, 70)}
	tierOnly.Node.Tier = types.TraceCausalTierCaliberSide
	if runtimeTraceProjElimEligible(tierOnly) {
		t.Fatalf("a typed ⌗ caliber_side tier must reject even when the token is wall clock")
	}
}

// --- 键与序 (design §1.2) --------------------------------------------------------

// TestElimBoardPureEffOrder — §29.61.12 ② (INV-SUPPLY 件④, 2026-07-14).
// EVOLUTION RECORD: the board WAS one pure eff-desc list; it is now
// causal-tier BLOCKED — the ⛓ chain block whole before the ◇ adjacent block,
// eff DESC within each block. Confidence still NEVER participates (R-d: the
// low-conf large row keeps the chain block's head) — the M-C eff×conf
// mutation stays red on the in-block assertion.
func TestElimBoardPureEffOrder(t *testing.T) {
	projection := elimBoardProjection()
	// Confidence inversion probe: the largest chain row gets the LOWEST
	// confidence — a composite eff×conf key would demote it below #2.
	projection.OnChainCauses[0].Confidence = 0.1
	projection.OnChainCauses[1].Confidence = 0.99
	model, _ := elimRenderOverview(t, projection, true)
	board := runtimeTraceProjElimBoard(model)
	if len(board) < 3 {
		t.Fatalf("fixture must field ≥3 eligible members, got %d", len(board))
	}
	for i := 1; i < len(board); i++ {
		if board[i].channelRank < board[i-1].channelRank {
			t.Fatalf("◇ block must never precede the ⛓ block: %d before %d", i-1, i)
		}
		if board[i].channelRank == board[i-1].channelRank &&
			board[i].row.Node.EffectiveImpactMS > board[i-1].row.Node.EffectiveImpactMS {
			t.Fatalf("within one block the board must order by published eff desc: %d before %d", i-1, i)
		}
	}
	if board[0].row.Node.EffectiveImpactMS != 26.392 {
		t.Fatalf("the low-confidence 26.392 row must keep the chain block head (置信不进键): %+v", board[0].row.Node)
	}
	// §29.61.12 ②: block ordering — a ◇ member NUMERICALLY LARGER than every
	// chain member still renders after the whole chain block (post-RSPA the
	// credential-less ◇ remainder must not visually bury proven causality).
	dominant := elimBoardProjection()
	dominant.AdjacentCauses[1].EffectiveImpactMS = 54.0
	dominant.AdjacentCauses[1].ImpactMS = 54.0
	dominant.AdjacentCauses[1].CumulativeImpactMS = 54.0
	model2, fence2 := elimRenderOverview(t, dominant, true)
	board2 := runtimeTraceProjElimBoard(model2)
	if len(board2) < 3 || board2[0].row.Node.EffectiveImpactMS != 26.392 ||
		board2[len(board2)-1].channelRank != 1 ||
		board2[len(board2)-1].row.Node.EffectiveImpactMS != 54.0 {
		t.Fatalf("the dominant ◇ 54.0 member must sort after the whole ⛓ block: %+v", board2)
	}
	members := elimOverviewMemberLines(fence2)
	if len(members) < 3 || !strings.Contains(members[0], "26.392ms") ||
		!strings.Contains(members[len(members)-1], "54.000ms") {
		t.Fatalf("render order must follow the blocked board:\n%s", fence2)
	}
	// 满格尺=全区最大值 (链上条短=诚实): the dominant ◇ row wears the FULL
	// bar; the chain block head does not.
	fullBar := strings.Repeat("█", runtimeTraceProjElimBarWidth)
	if !strings.Contains(members[len(members)-1], fullBar) {
		t.Fatalf("the section maximum must wear the full bar wherever it sits:\n%s", members[len(members)-1])
	}
	if strings.Contains(members[0], fullBar) {
		t.Fatalf("a chain head below the section maximum must render a short (honest) bar:\n%s", members[0])
	}
	// 表头承诺句随改 (表头禁撒谎) + §29.61.12 ① 记号词距. ELIM-V2 EVOLUTION
	// RECORD (2026-07-18): the promise speaks the direction-section layout
	// plus the standing anti-addition declaration; the retired 块内值降序
	// wording leaves with the flat layout.
	// 用户显示裁定 (2026-07-19): the form promises are deliberate multi-line
	// rows, each wearing the block glyph, zero indent (刻意更新非静默).
	if !strings.Contains(fence2, "⛓ 链上块先 · 节=修复方向(节序=节内最大可消降序)\n⛓ 方向间收益不可相加 · 节内值降序\n⛓ 零序数·零佩戴 · 定位走 [E#] · 满格=本区TOP1") {
		t.Fatalf("the header promise must state the direction-section ordering:\n%s", fence2)
	}
	if strings.Contains(fence2, "纯值降序") || strings.Contains(fence2, "块内值降序") {
		t.Fatalf("the retired flat-order promises must leave the header:\n%s", fence2)
	}
	if !strings.Contains(members[0], "⛓ 链上 · ") || !strings.Contains(members[len(members)-1], "◇ 邻近 · ") {
		t.Fatalf("channel identity words must carry the glyph-word space:\n%s", fence2)
	}
}

// --- 形制 (design §2.3-§2.5) ----------------------------------------------------

// TestElimOverviewMemberFormAndSameValueAsHome — R-g 第二发布面守护: every
// member line transcribes the row's published eff VERBATIM (%.3f, the same
// field the home row publishes), wears the channel identity word and its E#
// pointer; the fixture's TOP1/TOP2 are the chain rows in eff order and the ◇
// wall-clock row rides the same list.
func TestElimOverviewMemberFormAndSameValueAsHome(t *testing.T) {
	model, fence := elimRenderOverview(t, elimBoardProjection(), true)
	if fence == "" {
		t.Fatalf("rank-observed board must render the overview")
	}
	members := elimOverviewMemberLines(fence)
	if len(members) != 3 {
		t.Fatalf("expected exactly 3 members (2 chain + 1 ◇ wall clock), got %d:\n%s", len(members), fence)
	}
	board := runtimeTraceProjElimBoard(model)
	for i, entry := range board {
		want := fmt.Sprintf("%.3fms", entry.row.Node.EffectiveImpactMS)
		if !strings.Contains(members[i], want) {
			t.Fatalf("member %d must transcribe the home published eff %s verbatim:\n%s", i, want, members[i])
		}
	}
	if !strings.Contains(members[0], "⛓ 链上") || !strings.Contains(members[0], "26.392ms") ||
		!strings.Contains(members[0], "[E") {
		t.Fatalf("TOP1 must be the ⛓ 26.392 row with its E# pointer:\n%s", members[0])
	}
	if !strings.Contains(members[2], "◇ 邻近") || !strings.Contains(members[2], "0.710ms") {
		t.Fatalf("the ◇ wall-clock member must ride the same list:\n%s", members[2])
	}
	// §29.50.5 partition-seat pin: members never exceed the valued home rows.
	valued := 0
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for _, row := range rows {
			if row.HasData && row.Node.EffectiveImpactMS > 0 {
				valued++
			}
		}
	}
	if len(board) > valued {
		t.Fatalf("overview members (%d) must never exceed valued home rows (%d) — no resurrected twins", len(board), valued)
	}
}

// TestElimOverviewZeroOrdinalZeroBadge — §29.42.1 verbatim + R7 boundary: the
// overview carries no seat ordinals, no channel chip words, no ❶..❺ badge and
// no window-share % (the § 29.27 ban: eff may contain discounted components).
func TestElimOverviewZeroOrdinalZeroBadge(t *testing.T) {
	for _, zh := range []bool{true, false} {
		_, fence := elimRenderOverview(t, elimBoardProjection(), zh)
		if fence == "" {
			t.Fatalf("overview must render")
		}
		for _, banned := range []string{
			tracefence.SeatChannelChainZH + "#", tracefence.SeatChannelAdjacentZH + "#",
			tracefence.SeatChannelChainEN + " #", tracefence.SeatChannelAdjacentEN + " #",
			"%",
		} {
			if strings.Contains(fence, banned) {
				t.Fatalf("overview must never print %q (zh=%v):\n%s", banned, zh, fence)
			}
		}
		for _, badge := range tracefence.BadgeGlyphs() {
			if strings.Contains(fence, badge) {
				t.Fatalf("overview must never wear a badge %q (zh=%v):\n%s", badge, zh, fence)
			}
		}
	}
}

// TestElimOverviewNeverSums — 墙钟不可加和 (design R2 double enforcement):
// the overview prints per-row values only; no cross-row total vocabulary.
func TestElimOverviewNeverSums(t *testing.T) {
	_, fence := elimRenderOverview(t, elimBoardProjection(), true)
	for _, banned := range []string{"合计可省", "总计", "Σ"} {
		if strings.Contains(fence, banned) {
			t.Fatalf("overview must never sum across rows (%q):\n%s", banned, fence)
		}
	}
}

// TestElimOverviewExclusionFootnotes — 排除脚注义务 (排除≠消失): the ⌗ rows
// are named with the shared registry value-class word, the target-self
// symptom rows get their counted pointer, and the ▒ pointer names the count
// and the caliber boundary.
func TestElimOverviewExclusionFootnotes(t *testing.T) {
	_, fence := elimRenderOverview(t, elimBoardProjection(), true)
	// §29.104.18.2 件1 (2026-07-17): two ⌗ seats → the multi-seat form
	// (hoisted-boilerplate head + one seat per indented line).
	if !strings.Contains(fence, "· 不参与汇排(口径旁栏,非墙钟,不占序数):") ||
		!strings.Contains(fence, "计数当量") || !strings.Contains(fence, "综合评分") {
		t.Fatalf("⌗ rows must be footnoted with their registry value-class words:\n%s", fence)
	}
	if !strings.Contains(fence, "自身等待症状行 1 行") {
		t.Fatalf("the target-self symptom row must be footnoted as symptom-face:\n%s", fence)
	}
	if !strings.Contains(fence, "▒ 背景压力 2 行(跨线程/他线程口径,不计入链上归因)见背景段") {
		t.Fatalf("the ▒ pointer line must name the count and the caliber boundary:\n%s", fence)
	}
}

// TestElimOverviewAdjacentPointerForNonRankRows — O-5 (ADJ-MINT 未裁期间的
// fail-honest 形): the ◇ stanza holds a VALUED row that is NOT in the
// rank-item population (the 41006 E29 ✦-only / critical_blocking form) and
// the channel fielded no member — the overview points at the largest such
// row instead of inventing a population member; when ADJ-MINT later mints
// them as rank items they enter as members with zero changes here.
func TestElimOverviewAdjacentPointerForNonRankRows(t *testing.T) {
	projection := elimBoardProjection()
	// Replace the ◇ rank rows with a single valued NON-rank stanza row.
	blocking := uxr1AdjacentNode("E-cb", "critical_blocking", "blocking_span", 0, 0, 140)
	blocking.Predicate = "critical_blocking"
	blocking.Rank = 0
	blocking.EffectiveImpactMS = 0
	blocking.ImpactMS = 2.502
	blocking.CumulativeImpactMS = 2.502
	projection.AdjacentCauses = []types.TraceCausalProjectionNode{blocking}
	_, fence := elimRenderOverview(t, projection, true)
	if !strings.Contains(fence, "◇ 邻近段最大持值行 2.502ms 见邻近段") {
		t.Fatalf("O-5: the ◇ non-rank valued row must surface as a pointer line:\n%s", fence)
	}
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "2.502") {
			t.Fatalf("O-5: the non-rank row must never be invented into the member list:\n%s", line)
		}
	}
}

// TestElimOverviewE30BackgroundNeverEnters — the E30/54.x adversarial pin
// (design §7.2 三分验证 + R-f): magnitude-dominant ▒ rows (54.701/54.608)
// never appear as members even though their unit is ms — same unit ≠ same
// ruler; they stay behind the ▒ pointer.
func TestElimOverviewE30BackgroundNeverEnters(t *testing.T) {
	_, fence := elimRenderOverview(t, elimBoardProjection(), true)
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "54.701") || strings.Contains(line, "54.608") {
			t.Fatalf("▒ magnitude must never surface as an overview member:\n%s", line)
		}
	}
	if !strings.Contains(fence, "26.392") {
		t.Fatalf("fixture sanity: the true TOP1 renders:\n%s", fence)
	}
}

// TestElimOverviewAdjacentMaxFallback — ◇最大保底 (design §2.5): with five
// larger chain rows the ◇ member drops out of TOP5 and re-enters as the
// dedicated fallback row.
//
// EVOLUTION RECORD (件⑤ user ruling 2026-07-14, witness 20260714-164033 ◎
// 板): the ◇最大/◇max LEAD MARKER is retired (value + bar + the header's
// 满格=本区TOP1 promise already carry the signal; the marker broke the value
// grid) — the pin now asserts the never-buried SEAT itself: the largest ◇
// member still renders below TOP5 as an ordinary member line with its own
// value/channel, flush on the shared grid.
func TestElimOverviewAdjacentMaxFallback(t *testing.T) {
	projection := elimBoardProjection()
	projection.OnChainCauses = append(projection.OnChainCauses,
		elimChainNode("E-c3", "worker-a", "runnable_wait", "runnable", 3, 9.0, 400),
		elimChainNode("E-c4", "worker-b", "runnable_wait", "runnable", 4, 8.0, 410),
		elimChainNode("E-c5", "worker-c", "runnable_wait", "runnable", 5, 7.0, 420),
	)
	_, fence := elimRenderOverview(t, projection, true)
	if strings.Contains(fence, "◇最大") || strings.Contains(fence, "◇max") {
		t.Fatalf("the ◇最大 lead marker is retired (件⑤):\n%s", fence)
	}
	members := elimOverviewMemberLines(fence)
	if len(members) != runtimeTraceProjElimTopN+1 {
		t.Fatalf("TOP5 + one fallback seat expected, got %d members:\n%s", len(members), fence)
	}
	fallback := members[len(members)-1]
	if !strings.Contains(fallback, "0.710ms") || !strings.Contains(fallback, "◇ 邻近") {
		t.Fatalf("the fallback SEAT must survive with the ◇ member's own value/channel:\n%s", fallback)
	}
	// 件⑤ grid: with the lead field gone every member line is flush on the
	// bare value grid — the right-aligned `%9.3f` value ends with "ms " at
	// column 9 on every line, fallback included (the retired lead field would
	// shift it right by the marker width).
	for _, line := range members {
		if strings.Index(line, "ms ") != 9 {
			t.Fatalf("member lines must sit flush on one value grid (lead field retired):\n%q", line)
		}
	}
}

// TestElimOverviewEmptyChainHonestLine — 空链诚实行 (design §2.5/§4): a
// chainless board (4165 isomorph) states the empty chain channel, transcribes
// the typed flat-lane cause, and states the crown consequence; the ◇ members
// still rank.
func TestElimOverviewEmptyChainHonestLine(t *testing.T) {
	projection := uxr1FourOneSixFiveProjection()
	projection.RootCauseFamilyObserved = true
	_, fence := elimRenderOverview(t, projection, true)
	if fence == "" {
		t.Fatalf("the chainless rank board must still render an overview")
	}
	if !strings.Contains(fence, "链上:本窗无链上持值行") ||
		!strings.Contains(fence, "无已证链上可消除量,主根因不加冕") {
		t.Fatalf("the empty-chain honest line must state the channel state and the crown consequence:\n%s", fence)
	}
	members := elimOverviewMemberLines(fence)
	if len(members) != 2 {
		t.Fatalf("the ◇ wall-clock members must rank (io_latency + runnable), got %d:\n%s", len(members), fence)
	}
	if !strings.Contains(members[0], "0.099ms") || !strings.Contains(members[0], "◇ 邻近") {
		t.Fatalf("◇ TOP1 must be the io_latency wall-clock row:\n%s", members[0])
	}
	// 收尾件2 (P2-2, §29.61.12 表头禁撒谎 both directions): a chainless board
	// promises its single ◇ block honestly and NEVER prints the ⛓ glyph it
	// has no rows for. ELIM-V2 (2026-07-18): the standing anti-addition
	// declaration rides the ◇-only head too (its rows wear ·方向=X words).
	// 用户显示裁定 (2026-07-19): ◇-only boards follow the same multi-line
	// glyph-worn promise form.
	if !strings.Contains(fence, "◇ 邻近块·块内值降序\n◇ 方向间收益不可相加\n◇ 零序数·零佩戴 · 定位走 [E#] · 满格=本区TOP1") {
		t.Fatalf("the chainless header must promise the ◇ block form:\n%s", fence)
	}
	if strings.Contains(fence, "⛓") {
		t.Fatalf("a chainless ◎ face must not print the ⛓ glyph anywhere:\n%s", fence)
	}
	// ELIM-V2 修补轮 件4 absence pin: the ◇ block head 「◇ 邻近(条件可消上界 ·
	// 不入方向守恒)」 is a SEPARATOR between the ▸ sections and the adjacent
	// block — a ◇-only board has nothing to separate, so the head is absent
	// (the coexistence gate: sections>0 ∧ adjacent>0; strip it → red here).
	if strings.Contains(fence, "(条件可消上界") {
		t.Fatalf("件4: the ◇ block head must not render on a ◇-only board (separator role):\n%s", fence)
	}
}

// TestElimOverviewEmptyBoardHonestLine — 空板形: the rank family ran and
// admitted nothing → one honest line, never a silent disappearance.
func TestElimOverviewEmptyBoardHonestLine(t *testing.T) {
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		BackgroundCauses: []types.TraceCausalProjectionNode{
			uxr1BackgroundNode("E-b", "wc_srvinit_0-37254", "d_state_or_io_wait", "d_state_or_io_wait", "d_sleep", 54.608, 210),
		},
	}
	_, fence := elimRenderOverview(t, projection, true)
	if !strings.Contains(fence, "窗内可消除量:无同尺持值行(详见背景/义务通道)") {
		t.Fatalf("the empty board must render the honest single line:\n%s", fence)
	}
	// 收尾件2 (P2-2): a board that admitted nothing has no member ordering to
	// promise — the ordering/scale promise line is ABSENT (and with it the ⛓
	// glyph a memberless face has no rows for).
	if strings.Contains(fence, "块内值降序") || strings.Contains(fence, tracefence.ScaleMarkZH) {
		t.Fatalf("the empty board must not carry the ordering/scale promise line:\n%s", fence)
	}
	if strings.Contains(fence, "⛓") {
		t.Fatalf("an empty ◎ face must not print the ⛓ glyph:\n%s", fence)
	}
}

// TestElimOverviewAbsentWithoutRankFamily — board absent ≠ 空板: a run that
// never observed the root_cause_rank family renders NO overview face at all
// (absence never fabricates a board summary).
func TestElimOverviewAbsentWithoutRankFamily(t *testing.T) {
	projection := elimBoardProjection()
	projection.RootCauseFamilyObserved = false
	_, fence := elimRenderOverview(t, projection, true)
	if fence != "" {
		t.Fatalf("no rank family → no overview face:\n%s", fence)
	}
}

// TestElimOverviewSelfQualifierWordDrop — 裁定⑥词面: the typed self-basis row
// wears 自身·确定性优化 with NO 候选 word (the on-chain self row is a proven
// caliber); a ◇ semantic row keeps the 确定性优化·候选 identity family.
func TestElimOverviewSelfQualifierWordDrop(t *testing.T) {
	projection := elimBoardProjection()
	sem := elimChainNode("E-sem", "VSyncGenerator-2270", "class_verification", "", 2, 13.006, 500)
	sem.SemanticClass = "class_verification"
	sem.OnChainBasis = "self_deterministic_span"
	sem.Causality = "self_deterministic"
	projection.OnChainCauses[1] = sem
	_, fence := elimRenderOverview(t, projection, true)
	selfLine := ""
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "13.006ms") {
			selfLine = line
		}
	}
	if selfLine == "" {
		t.Fatalf("the self semantic member must rank:\n%s", fence)
	}
	if !strings.Contains(selfLine, "⛓ 链上") || !strings.Contains(selfLine, "自身·确定性优化") {
		t.Fatalf("the self member must wear the chain word + the Stage-1 self qualifier:\n%s", selfLine)
	}
	if strings.Contains(selfLine, "候选") {
		t.Fatalf("裁定⑥: the 候选 word must drop off the on-chain self row:\n%s", selfLine)
	}
	// The ◇ semantic counterpart keeps the conditional identity family.
	adjProjection := elimBoardProjection()
	adjSem := uxr1AdjacentNode("E-asem", "Jit thread pool-17284", "jit_compile", 2, 2.388, 510)
	adjSem.SemanticClass = "jit_compile"
	adjProjection.AdjacentCauses[1] = adjSem
	_, adjFence := elimRenderOverview(t, adjProjection, true)
	adjLine := ""
	for _, line := range elimOverviewMemberLines(adjFence) {
		if strings.Contains(line, "2.388ms") {
			adjLine = line
		}
	}
	if adjLine == "" || !strings.Contains(adjLine, "确定性优化·候选") {
		t.Fatalf("the ◇ semantic member keeps the 确定性优化·候选 identity word:\n%s", adjFence)
	}
}

// TestElimOverviewDiscountCaliberNote — 收尾件1 (冷读改进, 值面承诺): a ◎ row
// whose published eff is a DISCOUNTED value carries its home caliber word —
// the W1 TOP1 live instance (running supply-deficit seat: eff 3.175 vs raw
// 3.860) rendered wordless. Transcription only (the 行3 short-word composer
// runtimeTraceProjSupplyDiscountShortWord is the shared byte source); the
// single-max event form rides the same lane; pure wall-clock seats stay
// word-free (negative half).
func TestElimOverviewDiscountCaliberNote(t *testing.T) {
	projection := elimBoardProjection()
	// The W1 TOP1 form: a running seat published at its supply-fold deficit.
	deficit := &projection.OnChainCauses[0]
	deficit.TypeToken = "running"
	deficit.Object = "running"
	deficit.StateKind = "running"
	deficit.ImpactMS = 3.860
	deficit.CumulativeImpactMS = 7.400
	deficit.EffectiveImpactMS = 3.175
	deficit.SupplyFoldComputed = true
	deficit.SupplyFoldDeficitMS = 3.175
	deficit.SupplyFoldIdealMS = 0.685
	deficit.SupplyFoldKnownMS = 3.860
	deficit.SupplyFoldReferenceClass = "big"
	model, fence := elimRenderOverview(t, projection, true)
	deficitLine := ""
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "3.175ms") {
			deficitLine = line
		}
	}
	if deficitLine == "" {
		t.Fatalf("the deficit seat must rank:\n%s", fence)
	}
	// R5 (§29.88.12 单基准, 2026-07-15): the home caliber word is the
	// unified 全域最大核最高频 basis form (the 按大核满频 word retired).
	if !strings.Contains(deficitLine, "折算,按全域最大核最高频") {
		t.Fatalf("a discounted ◎ value must carry its home caliber word:\n%s", deficitLine)
	}
	// The word's legend entry travels with it (词条-图例双向).
	if !model.Marks.has(runtimeTraceProjMarkCaliberGlobalMaxFmax) {
		t.Fatalf("the transcribed caliber word must light its legend mark")
	}
	// Negative half: the pure wall-clock ◇ seat (eff == window projection)
	// carries NO caliber claim.
	for _, line := range elimOverviewMemberLines(fence) {
		if strings.Contains(line, "0.710ms") && strings.Contains(line, "折算") {
			t.Fatalf("an undiscounted seat must not claim a caliber word:\n%s", line)
		}
	}
	// Event-fold form: single-max published value speaks the 单次最大 word.
	// RNB-5B 件⑥: the form requires the typed wire-fold source bit (positive
	// arm; the coincidence trigger is retired — negative pin in
	// answer_document_projection_rnb5b_test.go).
	event := elimBoardProjection()
	fold := &event.OnChainCauses[1]
	fold.MergedCount = 3
	fold.MergedMinMS = 4.100
	fold.MergedMaxMS = 13.006
	fold.ImpactMS = 21.300
	fold.MergedWireFold = true
	_, eventFence := elimRenderOverview(t, event, true)
	eventLine := ""
	for _, line := range elimOverviewMemberLines(eventFence) {
		if strings.Contains(line, "13.006ms") {
			eventLine = line
		}
	}
	if eventLine == "" || !strings.Contains(eventLine, "单次最大(4.100~13.006ms,共3次)") {
		t.Fatalf("a single-max ◎ value must carry the 单次最大 word:\n%s", eventFence)
	}
}

// TestElimOverviewFamilyCaliberTranscription — 口径注记逐字转录: a family fold
// row's overview line carries the SAME single-source caliber word its home
// row publishes (合计(共N段,同线程)).
func TestElimOverviewFamilyCaliberTranscription(t *testing.T) {
	projection := elimBoardProjection()
	fam := &projection.OnChainCauses[1]
	fam.FamilyMemberCount = 14
	fam.FamilyFoldCaliber = "sum_disjoint"
	_, fence := elimRenderOverview(t, projection, true)
	if !strings.Contains(fence, "合计(共14段,同线程)") {
		t.Fatalf("the family member must transcribe its home caliber word verbatim:\n%s", fence)
	}
}

// --- INV-SUPPLY 件①/件③ (§29.61.11/.11a, 2026-07-14) ---------------------------

// elimInvSupplyCompoundProjection — the 090607 witness ❶ seat shape: an
// inversion cause seat whose published supply-fold deficit (7.296) dominates
// its published effective attribution (7.081 = runnable(全额) 0.109 +
// running(折算) 6.972; 行3 identity balances at print precision).
func elimInvSupplyCompoundProjection() types.TraceCausalProjection {
	projection := elimBoardProjection()
	// The ⌗ count/composite ◇ rows are irrelevant here — keep the wall-clock
	// ◇ member only (the compound seat + a plain board around it).
	projection.AdjacentCauses = projection.AdjacentCauses[1:2]
	inv := elimChainNode("E-inv", "CompThread_0-2955", "priority_inversion_candidate", "running", 1, 7.081, 100)
	inv.PriorityInversionCandidate = true
	inv.ImpactMS = 8.294
	inv.CumulativeImpactMS = 8.294
	inv.GatedRunnableMS = 0.109
	inv.GatedRunningDeficitMS = 6.972
	inv.SupplyFoldComputed = true
	inv.SupplyFoldDeficitMS = 7.296
	inv.SupplyFoldIdealMS = 0.998
	inv.SupplyFoldKnownMS = 8.294
	projection.OnChainCauses[0] = inv
	return projection
}

// TestElimInversionCompoundWordSameBytes — 件① 行2 复合词 + ◎ 转录同词: the
// typed dominance criterion mints 优先级反转候选·供给缺口主导 on the tree 行2
// AND the ◎ class-word slot from ONE composer (byte-identical); below the
// threshold both faces keep their pre-INV-SUPPLY words byte-identically, and
// the legend entry renders exactly with the word (词条-图例双向).
func TestElimInversionCompoundWordSameBytes(t *testing.T) {
	projection := elimInvSupplyCompoundProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree := runtimeTraceProjTreeFence(model, true)
	overview := runtimeTraceProjElimOverviewFence(projection, model, true)
	compound := tracefence.InversionCandidateWordZH + "·" + tracefence.SupplyGapDominantWordZH
	if !strings.Contains(tree, compound+"·"+tracefence.SeatChannelChainZH+"#1") {
		t.Fatalf("行2 must wear the compound type word before its seat chip:\n%s", tree)
	}
	seatLine := ""
	for _, line := range elimOverviewMemberLines(overview) {
		if strings.Contains(line, "7.081ms") {
			seatLine = line
		}
	}
	if seatLine == "" || !strings.Contains(seatLine, compound) {
		t.Fatalf("◎ must transcribe the SAME compound word bytes (同词):\n%s", overview)
	}
	if !model.Marks.has(runtimeTraceProjMarkSupplyGapDominant) {
		t.Fatalf("the compound word must light its legend mark")
	}
	if lead := runtimeTraceProjLeadText(projection, model, "zh", true); !strings.Contains(lead, "`供给缺口主导`") {
		t.Fatalf("the compound word's legend entry must render with it:\n%s", lead)
	}
	// EN face: raw wire token + spaced en suffix (D2 discipline).
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enTree := runtimeTraceProjTreeFence(enModel, false)
	enOverview := runtimeTraceProjElimOverviewFence(projection, enModel, false)
	enCompound := "priority_inversion_candidate · " + tracefence.SupplyGapDominantWordEN
	// The en tree 行2 may wrap between words (space-boundary atoms) — the
	// despaced surface carries the whole compound; the ◎ member line never
	// wraps and must carry it verbatim.
	if !strings.Contains(vs2Despace(enTree), vs2Despace(enCompound)) || !strings.Contains(enOverview, enCompound) {
		t.Fatalf("the en compound must ride both faces:\n%s\n%s", enTree, enOverview)
	}
	// Sub-threshold negative (typed criterion, candidate 50%): deficit 3.0 <
	// 7.081×0.50 — both faces keep the bare word; no legend entry.
	below := elimInvSupplyCompoundProjection()
	below.OnChainCauses[0].SupplyFoldDeficitMS = 3.0
	belowModel := buildRuntimeTraceProjTreeModel(below, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	belowTree := runtimeTraceProjTreeFence(belowModel, true)
	belowOverview := runtimeTraceProjElimOverviewFence(below, belowModel, true)
	if strings.Contains(belowTree, tracefence.SupplyGapDominantWordZH) ||
		strings.Contains(belowOverview, tracefence.SupplyGapDominantWordZH) {
		t.Fatalf("below the threshold the compound word must not mint:\n%s\n%s", belowTree, belowOverview)
	}
	if !strings.Contains(belowTree, tracefence.InversionCandidateWordZH+"·"+tracefence.SeatChannelChainZH+"#1") {
		t.Fatalf("below the threshold 行2 keeps the bare inversion word:\n%s", belowTree)
	}
	if lead := runtimeTraceProjLeadText(below, belowModel, "zh", true); strings.Contains(lead, "`供给缺口主导`") {
		t.Fatalf("no compound word → no legend entry:\n%s", lead)
	}
}

// TestElimCompositionLeverageNote — 件③ ◎ 分杠杆注: the compound seat's ◎ row
// carries the 可消除构成 note directly under it, transcribing the SAME 行3
// component bytes by elimination lever (调度修复=runnable(全额), 频点/热策略=
// running(折算)); a constituent display — never a Σ row, no total claim; and
// non-compound seats render no note.
func TestElimCompositionLeverageNote(t *testing.T) {
	projection := elimInvSupplyCompoundProjection()
	model, fence := elimRenderOverview(t, projection, true)
	lines := strings.Split(fence, "\n")
	// EVOLUTION RECORD (RNB-1 C-3, §29.88.11 R7a, 2026-07-14): the note
	// relocated from an interstitial sub-line under its seat row into the
	// dedicated 构成拆解 section — content bytes unchanged, packaging is now
	// `  [E#] ` (the E# replaces adjacency as the binding), and the bar
	// region is PURE seat rows (件⑤⑥ 栅格纯度; the 件⑥ 12-column indent form
	// retired with its position — negative arm below pins the invariant).
	noteAt, headAt := -1, -1
	for i, line := range lines {
		if line == "  [E2] 可消除构成: 调度修复 0.109ms + 频点/热策略 6.972ms" {
			noteAt = i
		}
		if strings.HasPrefix(line, "· 构成拆解(按 [E#] 索引):") {
			headAt = i
		}
	}
	if noteAt < 1 || headAt < 1 || noteAt != headAt+1 {
		t.Fatalf("the compound seat's note must live in the 构成拆解 section under its head:\n%s", fence)
	}
	// C-3 ⑤ 负向 pin (bar 区无行间子行): no `·`-led sub-line may sit between
	// two seat rows — the bar region is pure.
	seatRow := func(line string) bool { return strings.Contains(line, "█") || strings.Contains(line, "░") }
	for i := 1; i < len(lines)-1; i++ {
		trimmed := strings.TrimLeft(lines[i], " ")
		if strings.HasPrefix(trimmed, "·") && lines[i] != strings.TrimLeft(lines[i], " ") &&
			i > 0 && seatRow(lines[i-1]) && i+1 < len(lines) && seatRow(lines[i+1]) {
			t.Fatalf("C-3 ⑤: interstitial sub-line between seat rows (bar 区必须纯席行): %q\n%s", lines[i], fence)
		}
	}
	// 零求和红线: a constituent display — no total, no Σ vocabulary.
	if strings.Contains(lines[noteAt], "=") || strings.Contains(fence, "Σ") || strings.Contains(fence, "总计") {
		t.Fatalf("the note must never claim a total:\n%s", fence)
	}
	if !model.Marks.has(runtimeTraceProjMarkElimComposition) {
		t.Fatalf("the note must light its legend mark")
	}
	if lead := runtimeTraceProjLeadText(projection, model, "zh", true); !strings.Contains(lead, "可消除构成") {
		t.Fatalf("the note's legend entry must render with it:\n%s", lead)
	}
	// Negative: the plain board (no compound seat) renders no note.
	_, plain := elimRenderOverview(t, elimBoardProjection(), true)
	if strings.Contains(plain, "可消除构成") {
		t.Fatalf("non-compound seats must not mint the note:\n%s", plain)
	}
	// EN face rides the same lane.
	enModel := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	enFence := runtimeTraceProjElimOverviewFence(projection, enModel, false)
	if !strings.Contains(enFence, "· composition breakdown (indexed by [E#]):") ||
		!strings.Contains(enFence, "] eliminable composition: scheduling fix 0.109ms + frequency/thermal policy 6.972ms") {
		t.Fatalf("en composition-breakdown section missing:\n%s", enFence)
	}
	_ = enModel
}

// TestElimInvSupplyDonghuEngineRealWitness — the INV-SUPPLY witness on
// ENGINE-REAL records (§29.53 产线实铸形 red line; zero LLM, zero dispatch
// variance): the real donghu.ftrace 090607 window (target .ugc.aweme.lite-
// 17267, 13762.791708..13763.024898) mints the ❶ CompThread_0-2955 inversion
// seat. EVOLUTION RECORD (R5 §29.88.12 单基准单算法, 2026-07-15): this seat
// IS the §29.88.12 witness — it used to publish TWO conversion numbers
// (eff 7.081 = 0.109 + gated 6.972「按下游消费核」 beside supply-fold
// deficit 7.296「按大核满频」). The unified fold now prices the running
// component and the deficit as ONE number: eff 7.405 = runnable(全额) 0.109
// + running(折算) 7.296, deficit 7.296 — 同席一套折算数,同源可互推.
// The report must show, at once:
//
//   - 行2: the compound type word 优先级反转候选·供给缺口主导 before the
//     根因排序#1 chip (件①);
//   - ◎ overview: the SAME compound bytes on the 7.405ms member (同词) plus
//     the 可消除构成 leverage note transcribing the 行3 split (件③);
//   - §29.61.12: the blocked-order header promise and the spaced channel
//     words on the real board (件④).
func TestElimInvSupplyDonghuEngineRealWitness(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "donghu.ftrace", "p-"+view, "r", "", at)...)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "donghu 090607 witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	compound := tracefence.InversionCandidateWordZH + "·" + tracefence.SupplyGapDominantWordZH
	// 件① 行2: the compound word rides the inversion seat's identity row.
	// EVOLUTION RECORD (ELIM-SELF-FIX 件1 §29.93.1, 2026-07-15): #1→#2 — the
	// target's own running supply-fold deficit seat (157.248ms window
	// running, deficit 58.320ms) now mints and takes ❶; the CompThread
	// compound seat keeps its word/value one slot down.
	if !strings.Contains(md, compound+"·"+tracefence.SeatChannelChainZH+"#2") {
		t.Fatalf("行2 must wear the compound word on the real inversion seat:\n%s", md)
	}
	// The self running fold seat leads the board (ELIM-SELF Form-1 修根:
	// the would-be 8.2× gap the ◎ board used to miss is now its #1 seat).
	if !strings.Contains(md, "供给折算影响 58.320ms") {
		t.Fatalf("the self running fold seat must publish its 58.320ms deficit:\n%s", md)
	}
	// SELF-ALL 同形纪律 (件1 修复轮): the SELF-stanza row renders the same 行3
	// 「=」grammar as the flat-tree face — raw 157.248 on 行1 must never sit
	// beside a ranked 58.320 without the row-level breakdown.
	if !strings.Contains(md, "有效归因 58.320ms = running(折算,按全域最大核最高频) 58.320ms") {
		t.Fatalf("the self stanza must render the running seat's 行3 breakdown:\n%s", md)
	}
	// ◎ region: same word + the leverage note with the REAL 行3 bytes.
	elimAt := strings.Index(md, tracefence.ElimOpener)
	if elimAt < 0 {
		t.Fatalf("the ◎ overview must render:\n%s", md)
	}
	elim := md[elimAt:]
	if end := strings.Index(elim[len(tracefence.ElimOpener):], "```"); end >= 0 {
		elim = elim[:len(tracefence.ElimOpener)+end+3]
	}
	seatLine := ""
	for _, line := range elimOverviewMemberLines(elim) {
		if strings.Contains(line, "7.405ms") {
			seatLine = line
		}
	}
	if seatLine == "" || !strings.Contains(seatLine, compound) {
		t.Fatalf("◎ must transcribe the compound word on the 7.081 seat (同词):\n%s", elim)
	}
	// EVOLUTION RECORD (RNB-1 C-3, §29.88.11 R7a, 2026-07-14): the note bytes
	// live in the 构成拆解 section with the `  [E#] ` packaging (byte-
	// identical content, indexed to the seat row).
	if !strings.Contains(elim, "· 构成拆解(按 [E#] 索引):") ||
		!strings.Contains(elim, "] 可消除构成: 调度修复 0.109ms + 频点/热策略 7.296ms") {
		t.Fatalf("the ◎ 构成拆解 section must transcribe the real 行3 split:\n%s", elim)
	}
	// 件④: blocked-order header promise + spaced channel words (ELIM-V2
	// EVOLUTION RECORD 2026-07-18: the promise speaks the direction-section
	// layout).
	// 用户显示裁定 (2026-07-19): multi-line glyph-worn promise form.
	if !strings.Contains(elim, "⛓ 链上块先 · 节=修复方向(节序=节内最大可消降序)\n⛓ 方向间收益不可相加 · 节内值降序") {
		t.Fatalf("the blocked-order header promise must render:\n%s", elim)
	}
	if !strings.Contains(seatLine, "⛓ 链上 · ") {
		t.Fatalf("the member rows must wear the spaced channel word:\n%s", seatLine)
	}
	// 行3 itself stays byte-stable next to the compound (truth row intact).
	if !strings.Contains(strings.ReplaceAll(md, " ", ""), "有效归因7.405ms=runnable(全额)0.109ms+running(折算)7.296ms") {
		t.Fatalf("the 行3 identity must keep the real split:\n%s", md)
	}
}

// TestElimTopNAlignedWithBadgeTopN — R-c: K moves only with §29.27.1 (joint
// re-ruling); the two constants are one value by construction.
func TestElimTopNAlignedWithBadgeTopN(t *testing.T) {
	if runtimeTraceProjElimTopN != runtimeTraceProjBadgeTopN {
		t.Fatalf("TOP-K must stay aligned with the badge TOP N (联动重裁,禁单边扩): %d vs %d",
			runtimeTraceProjElimTopN, runtimeTraceProjBadgeTopN)
	}
}

// TestElimOverviewLegendLockstep — 图例承诺面: the ◎ legend entry renders
// exactly when the overview renders (mark ⟺ entry through the dynamic
// legend), and the fence opener carries the typed elim info token.
func TestElimOverviewLegendLockstep(t *testing.T) {
	projection := elimBoardProjection()
	model, fence := elimRenderOverview(t, projection, true)
	if !strings.HasPrefix(fence, tracefence.ElimOpener+"\n") {
		t.Fatalf("overview fence must open with the typed elim token: %q", strings.SplitN(fence, "\n", 2)[0])
	}
	if !model.Marks.has(runtimeTraceProjMarkElimOverview) {
		t.Fatalf("the ◎ mark must record at the overview emission site")
	}
	lead := runtimeTraceProjLeadText(projection, model, "zh", true)
	if !strings.Contains(lead, "`◎` = 窗内可消除量总览") {
		t.Fatalf("the ◎ legend promise sentence must render with the overview:\n%s", lead)
	}
	// Negative half: no overview (no rank family) → no ◎ legend entry.
	bare := elimBoardProjection()
	bare.RootCauseFamilyObserved = false
	bareModel, bareFence := elimRenderOverview(t, bare, true)
	if bareFence != "" {
		t.Fatalf("fixture: overview must be absent")
	}
	if bareLead := runtimeTraceProjLeadText(bare, bareModel, "zh", true); strings.Contains(bareLead, "`◎`") {
		t.Fatalf("no overview → no ◎ legend entry:\n%s", bareLead)
	}
}

// --- HTML face (fence classifier / anchors / stanza head) ------------------------

// TestElimOverviewPreviewClassifierAndAnchors — the ◎ fence classifies on its
// OWN typed token (grid render + hook class), its textContent stays
// byte-identical, the projection tree's anchor census is NOT broken by the
// second fence (the pre-ELIM count identity would have killed every anchor),
// and the overview's [E#] tokens link through the HOST fence's pairing.
func TestElimOverviewPreviewClassifierAndAnchors(t *testing.T) {
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	binder := projV3Obs("root-binder", "root_cause_primary", "root_cause_primary:binder",
		"binder:42591_4-42712", "sleep_wait", "38.400", 38.4, 104233, 104391,
		"rank=1", "tier=primary", "chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=1",
		"effective_impact_ms=36.100", "dominant_state=s_sleep")
	obs := []types.ObservationRecord{
		binder,
		projV3Obs("hop-disp", "wakeup_causal_impact", "wakeup_causal_impact:disp",
			"DispatchQueue-771", "runnable_delay", "9.800", 9.8, 76001, 76310,
			"chain_relevance=on_chain", "causality=on_wakeup_chain", "chain_depth=2", "dominant_state=runnable"),
		{
			ID: "path", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard, Predicate: "wakeup_chain", ClaimKey: "wakeup_chain:path",
			Object: "DispatchQueue-771 -> binder:42591_4-42712 -> oney.hmn.berlin-42591",
		},
	}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "样例摘要。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	if !strings.Contains(md, tracefence.ElimOpener) {
		t.Fatalf("rank-bearing report must carry the ◎ fence:\n%s", md)
	}
	html, err := preview.RenderMarkdownHTML([]byte(md))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `<pre class="trace-projection-tree trace-elim-overview"`) {
		t.Fatalf("the ◎ fence must classify with its own hook class:\n%s", html)
	}
	// md/html 双面同位 (user ruling 2026-07-13): the overview leads the tree
	// on BOTH faces.
	if strings.Index(md, tracefence.ElimOpener) > strings.Index(md, tracefence.Opener) {
		t.Fatalf("md face: the ◎ overview must precede the tree fence")
	}
	if strings.Index(html, "trace-elim-overview") > strings.Index(html, `aria-label="Trace causal projection tree"`) {
		t.Fatalf("html face: the ◎ overview must precede the tree fence")
	}
	// The projection anchor lane survives the second fence: detail/evidence
	// targets exist and the overview's E# links resolve on the same prefix.
	if !strings.Contains(html, `id="trace-e1"`) {
		t.Fatalf("the anchor census must survive the ◎ fence (pre-ELIM it bailed out whole):\n%s", html)
	}
	overviewAt := strings.Index(html, "trace-elim-overview")
	if overviewAt < 0 || !strings.Contains(html[overviewAt:], `<a class="trace-eref"`) {
		t.Fatalf("the overview's [E#] pointers must link through the host pairing:\n%s", html)
	}
	// ◎ head line wears the stanza-head class (same family as ◇/▒ heads).
	if !strings.Contains(html, `trace-stanza-head`) {
		t.Fatalf("the ◎ head must wear the stanza-head class:\n%s", html)
	}
	// textContent byte identity on the overview region (decoration only).
	if !strings.Contains(md, "◎ 窗内可消除量总览") {
		t.Fatalf("md face must carry the ◎ head verbatim")
	}
}

// --- W1 engine-real witness (donghu JIT self-seat form) --------------------------

// TestElimOverviewDonghuJitEngineRealWitness — the W1 display witness on
// ENGINE-REAL records (§29.53 产线实铸形 red line; zero LLM, zero dispatch
// variance): the real donghu.ftrace aweme-window rank result (the Stage-1
// channel-flip witness: Jit thread pool-17284's own JIT family, on_chain via
// the typed self basis, 根因排序#2, eff 2.388 unchanged) flows through the
// REAL tool observation emitters into the answer document. The report must
// show the SELF SEAT on BOTH faces at once:
//
//   - main board (tree fence): ❷ + 根因排序#2 + 自身·确定性优化 on the ✦ row;
//   - ◎ overview: the 2.388ms ⛓ 链上 member with the self qualifier and NO
//     候选 word — value verbatim, ordered under the larger chain seats.
func TestElimOverviewDonghuJitEngineRealWitness(t *testing.T) {
	const trace = "../../eval/fixtures/real_traces/donghu.ftrace"
	if _, err := os.Stat(trace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), trace)
	if err != nil {
		t.Fatal(err)
	}
	query := tracequery.Query{PID: 17284, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		MaxDepth: 4, MinDurationMs: 0.5, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 12}
	at := time.Unix(1751600000, 0).UTC()
	var obs []types.ObservationRecord
	for _, view := range []string{"wakeup_chain", "root_cause_rank"} {
		q := query
		q.View = view
		result := tracequery.Run(idx, q)
		obs = append(obs, traceQueryTypedObservations(result, "donghu.ftrace", "p-"+view, "r", "", at)...)
	}
	bus := newBusForMutationTest()
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent: types.IntentTrace, Scenario: types.ScenarioPerformanceBottleneck,
	}}
	bus.ToolResults = []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: obs}}
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "donghu witness。"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("apply: %v %s", err, res.Summary)
	}
	md := render.RenderAnswerDocument(bus.Mutable.AnswerDocumentV2(), "zh")
	// Main-board coexistence: seat #2 + badge + the Stage-1 qualifier.
	for _, want := range []string{"❷", "根因排序#2", "自身·确定性优化"} {
		if !strings.Contains(md, want) {
			t.Fatalf("main board must keep the Stage-1 self seat face (%q):\n%s", want, md)
		}
	}
	// The ◎ overview renders with the self member.
	elimAt := strings.Index(md, tracefence.ElimOpener)
	if elimAt < 0 {
		t.Fatalf("engine-real rank report must render the ◎ overview:\n%s", md)
	}
	// Position (user ruling 2026-07-13): the overview LEADS — it renders
	// BEFORE the projection tree fence (先执摘后细节; the E# pointers are
	// forward references into the tree/board below).
	if treeAt := strings.Index(md, tracefence.Opener); treeAt < 0 || elimAt > treeAt {
		t.Fatalf("the ◎ overview must render before the projection tree (elim@%d tree@%d)", elimAt, treeAt)
	}
	elim := md[elimAt:]
	if end := strings.Index(elim[len(tracefence.ElimOpener):], "```"); end >= 0 {
		elim = elim[:len(tracefence.ElimOpener)+end+3]
	}
	selfLine := ""
	for _, line := range elimOverviewMemberLines(elim) {
		if strings.Contains(line, "2.388ms") {
			selfLine = line
		}
	}
	if selfLine == "" {
		t.Fatalf("the self family member (2.388ms verbatim) must ride the overview:\n%s", elim)
	}
	if !strings.Contains(selfLine, "⛓ 链上") || !strings.Contains(selfLine, "自身·确定性优化") {
		t.Fatalf("the overview self member must wear ⛓ 链上 + the self qualifier:\n%s", selfLine)
	}
	if strings.Contains(selfLine, "候选") {
		t.Fatalf("裁定⑥: no 候选 word on the on-chain self member:\n%s", selfLine)
	}
	// Zero ordinals / zero badges inside the overview even on the real board.
	for _, banned := range []string{"根因排序#", "❶", "❷", "❸"} {
		if strings.Contains(elim, banned) {
			t.Fatalf("the overview must stay ordinal-free on the real board (%q):\n%s", banned, elim)
		}
	}
	// The self member sits BELOW at least one larger chain seat (pure eff
	// order — the donghu board's #1 outranks the 2.388 family).
	members := elimOverviewMemberLines(elim)
	selfIdx := -1
	for i, line := range members {
		if strings.Contains(line, "2.388ms") {
			selfIdx = i
		}
	}
	if selfIdx < 1 {
		t.Fatalf("the 2.388 self member must rank below the larger chain seats (got index %d):\n%s", selfIdx, elim)
	}
}

// --- C4 占窗% column (rider, §29.61 d / 裁定⑤) ----------------------------------

// TestC4WindowShareColumn — the optimization table's % column: same value
// source as the ms cell over the analysis window, semantic-class rows only,
// subordinate member/fold cells and windowless renders stay "—".
func TestC4WindowShareColumn(t *testing.T) {
	projection := types.TraceCausalProjection{
		RootCauseFamilyObserved: true,
		WindowStartTs:           1000.000, WindowEndTs: 1000.150, // 150ms window
		SemanticSpans: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleSemanticSpan, EvidenceID: "E-sp",
			Subject: "worker-9", Object: "class_verification", TypeToken: "class_verification",
			Predicate: "trace_semantic_span", SemanticClass: "class_verification",
			SpanName: "VerifyClass com.example.Foo", ImpactMS: 13.006, CumulativeImpactMS: 13.006,
			EffectiveImpactMS: 12.900, LineStart: 100, LineEnd: 120, Confidence: 0.8,
		}},
	}
	evidence := newRuntimeTraceCausalProjectionEvidenceIndex()
	model := buildRuntimeTraceProjTreeModel(projection, evidence, true)
	columns, rows := runtimeTraceSemanticOptimizationParts(projection, evidence, model.WindowMS, true)
	if len(columns) != 6 || columns[4] != "占窗%" {
		t.Fatalf("C4 must carry the 占窗%% column: %v", columns)
	}
	if len(rows) == 0 || len(rows[0].Cells) != 6 {
		t.Fatalf("C4 rows must carry six cells: %+v", rows)
	}
	// 12.900/150 = 8.6% — the SAME published value the ms cell prints.
	if rows[0].Cells[3] != "12.900ms" || rows[0].Cells[4] != "8.6%" {
		t.Fatalf("%% must divide the ms cell's own value by the window: %v", rows[0].Cells)
	}
	// Windowless render: no denominator, no claim.
	_, noWin := runtimeTraceSemanticOptimizationParts(projection, evidence, 0, true)
	if noWin[0].Cells[4] != "—" {
		t.Fatalf("windowless C4 must not mint a share: %v", noWin[0].Cells)
	}
	// Family subordinate rows stay dash-celled (SPANTOP-1 件5: header + ONE
	// counted pointer row — the member rows left this table).
	fam := projection
	fam.SemanticSpans[0].FamilyMemberCount = 3
	fam.SemanticSpans[0].FamilyFoldCaliber = "sum_disjoint"
	fam.SemanticSpans[0].FamilyMemberRoster = []string{"a", "b", "c"}
	_, famRows := runtimeTraceSemanticOptimizationParts(fam, evidence, model.WindowMS, true)
	if len(famRows) != 2 {
		t.Fatalf("family form must render header + counted pointer row: %+v", famRows)
	}
	if famRows[0].Cells[4] == "—" {
		t.Fatalf("the family header row (semantic class) must carry its share: %v", famRows[0].Cells)
	}
	for i, row := range famRows[1:] {
		if row.Cells[4] != "—" {
			t.Fatalf("subordinate row %d must not mint a share: %v", i+1, row.Cells)
		}
	}
}

// --- RNB-1 R7 (§29.88.10, 2026-07-14) pins -----------------------------------

// rnbR7DualSeatProjection — the witness 20260714-230952 JankManager shape: an
// inversion-lane seat (eff 0.423 = gated runnable ONLY, zero running
// component, supply fold present with a dominant deficit) beside the runnable
// census seat of the SAME thread publishing the µs-equal 0.423.
func rnbR7DualSeatProjection() types.TraceCausalProjection {
	projection := elimBoardProjection()
	projection.AdjacentCauses = projection.AdjacentCauses[1:2]
	inv := elimChainNode("E31", "JankManager-9655", "priority_inversion_candidate", "runnable", 2, 0.423, 100)
	inv.PriorityInversionCandidate = true
	inv.GatedRunnableMS = 0.423
	inv.GatedRunningDeficitMS = 0
	inv.SupplyFoldComputed = true
	inv.SupplyFoldDeficitMS = 4.596
	inv.SupplyFoldIdealMS = 0.9
	inv.SupplyFoldKnownMS = 5.5
	runnable := elimChainNode("E32", "JankManager-9655", "runnable_wait", "runnable", 3, 0.423, 120)
	projection.OnChainCauses = append(projection.OnChainCauses, inv, runnable)
	return projection
}

// C-1: the ◎ board converges the same-thread same-value dual seats to ONE
// seat — the inversion identity survives, the runnable twin's E# joins its
// bracket; a value-diverging pair keeps both seats (negative arm).
func TestElimR7DualSeatConvergence(t *testing.T) {
	projection := rnbR7DualSeatProjection()
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	overview := runtimeTraceProjElimOverviewFence(projection, model, true)
	seatLines := 0
	converged := ""
	for _, line := range elimOverviewMemberLines(overview) {
		if strings.Contains(line, "JankManager-9655") {
			seatLines++
			converged = line
		}
	}
	if seatLines != 1 {
		t.Fatalf("C-1: same-thread same-value dual seats must converge to ONE ◎ seat, got %d:\n%s", seatLines, overview)
	}
	if !strings.Contains(converged, "0.423ms") || !strings.Contains(converged, "+") {
		t.Fatalf("C-1: the converged seat keeps the value and merges both E#s ([E#+E#]): %q", converged)
	}
	// Negative arm: a diverging value keeps both seats (precise µs equality,
	// never fuzzy proximity).
	diverged := rnbR7DualSeatProjection()
	diverged.OnChainCauses[len(diverged.OnChainCauses)-1].EffectiveImpactMS = 0.424
	diverged.OnChainCauses[len(diverged.OnChainCauses)-1].ImpactMS = 0.424
	diverged.OnChainCauses[len(diverged.OnChainCauses)-1].CumulativeImpactMS = 0.424
	model = buildRuntimeTraceProjTreeModel(diverged, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	overview = runtimeTraceProjElimOverviewFence(diverged, model, true)
	seatLines = 0
	for _, line := range elimOverviewMemberLines(overview) {
		if strings.Contains(line, "JankManager-9655") {
			seatLines++
		}
	}
	if seatLines != 2 {
		t.Fatalf("C-1 negative arm: µs-diverging values must keep both seats, got %d:\n%s", seatLines, overview)
	}
}

// C-2②: the 「供给缺口主导」 compound suffix requires a running-折算
// component in the eff composition — a zero-supply composition (witness E31:
// eff = runnable(全额) only) may not wear it even ABOVE the ratio threshold;
// the two-component form (runnable.txt E19 shape) keeps the word (negative
// pin: 既有多分量形不受伤).
func TestElimR7CompoundWordRequiresSupplyComponent(t *testing.T) {
	zeroSupply := types.TraceCausalProjectionNode{
		TypeToken: "priority_inversion_candidate", PriorityInversionCandidate: true,
		StateKind: "runnable", EffectiveImpactMS: 0.423,
		GatedRunnableMS: 0.423, GatedRunningDeficitMS: 0,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 4.596,
	}
	if _, ok := runtimeTraceProjInversionSupplyGapCompoundWord(zeroSupply, true); ok {
		t.Fatalf("C-2②: a zero-supply composition must not wear 供给缺口主导 (词面自相矛盾)")
	}
	twoComponent := zeroSupply
	twoComponent.EffectiveImpactMS = 1.752
	twoComponent.GatedRunnableMS = 0.093
	twoComponent.GatedRunningDeficitMS = 1.659
	twoComponent.SupplyFoldDeficitMS = 1.911
	twoComponent.SupplyFoldIdealMS = 1.219
	if word, ok := runtimeTraceProjInversionSupplyGapCompoundWord(twoComponent, true); !ok ||
		!strings.Contains(word, tracefence.SupplyGapDominantWordZH) {
		t.Fatalf("C-2② negative arm: the two-component form keeps the compound word: %q ok=%v", word, ok)
	}
}

// C-2①: the ◎ composition note renders only when the split actually splits
// (≥2 levers) — a single-component note is the row value re-printed (zero
// information); the two-component form keeps its note byte-form (negative
// pin).
func TestElimR7SingleComponentCompositionNoteSuppressed(t *testing.T) {
	marks := &runtimeTraceProjMarkSet{}
	single := runtimeTraceProjTreeRow{HasData: true, Node: types.TraceCausalProjectionNode{
		TypeToken: "priority_inversion_candidate", PriorityInversionCandidate: true,
		StateKind: "runnable", EffectiveImpactMS: 1.659,
		GatedRunnableMS: 0, GatedRunningDeficitMS: 1.659,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 1.911, SupplyFoldIdealMS: 1.219,
	}}
	if note, ok := runtimeTraceProjElimCompositionNoteLine(single, marks, true); ok {
		t.Fatalf("C-2①: a single-component composition note must not render: %q", note)
	}
	two := runtimeTraceProjTreeRow{HasData: true, Node: types.TraceCausalProjectionNode{
		TypeToken: "priority_inversion_candidate", PriorityInversionCandidate: true,
		StateKind: "runnable", EffectiveImpactMS: 1.752,
		GatedRunnableMS: 0.093, GatedRunningDeficitMS: 1.659,
		SupplyFoldComputed: true, SupplyFoldDeficitMS: 1.911, SupplyFoldIdealMS: 1.219,
	}}
	note, ok := runtimeTraceProjElimCompositionNoteLine(two, marks, true)
	if !ok || !strings.Contains(note, "调度修复 0.093ms") || !strings.Contains(note, "频点/热策略 1.659ms") {
		t.Fatalf("C-2① negative arm: the two-component note keeps rendering: %q ok=%v", note, ok)
	}
}

// TestElimHeadPromisesMultiLineGlyphWorn — 用户显示裁定 (2026-07-19, witness
// 20260719-161405 L146-147): the ◎ head form promises render as deliberate
// multi-line rows — every promise line wears the block glyph as its FIRST
// rune (同佩图标), zero indent, and sits inside the row width cap naturally
// (no wrap-engine bisected continuation). All four arms (zh/EN ×
// chain/adjacent) obey the same rule.
func TestElimHeadPromisesMultiLineGlyphWorn(t *testing.T) {
	model := runtimeTraceProjTreeModel{Target: "VSyncGenerator-2270"}
	for _, tc := range []struct {
		name         string
		zh           bool
		chainPresent bool
		glyph        string
	}{
		{"zh_chain", true, true, "⛓"},
		{"zh_adjacent", true, false, "◇"},
		{"en_chain", false, true, "⛓"},
		{"en_adjacent", false, false, "◇"},
	} {
		lines := runtimeTraceProjElimHead(model, tc.zh, true, tc.chainPresent, false)
		if len(lines) != 4 {
			t.Fatalf("%s: want title+3 deliberate promise lines, got %d:\n%s", tc.name, len(lines), strings.Join(lines, "\n"))
		}
		for i, line := range lines[1:] {
			if !strings.HasPrefix(line, tc.glyph+" ") {
				t.Fatalf("%s: promise line %d must wear the %s glyph unindented: %q", tc.name, i+1, tc.glyph, line)
			}
			if w := runewidth.StringWidth(line); w > runtimeTraceProjTreeRowMaxWidth {
				t.Fatalf("%s: promise line %d overflows the width cap (%d > %d) — it would wrap into a bare-headed continuation: %q",
					tc.name, i+1, w, runtimeTraceProjTreeRowMaxWidth, line)
			}
		}
	}
	// withForm=false (honest empty board) keeps the single title+ruler line.
	if lines := runtimeTraceProjElimHead(model, true, false, true, false); len(lines) != 1 {
		t.Fatalf("the form-less head stays one line: %v", lines)
	}
}
