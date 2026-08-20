package tool

// answer_document_mutation_runtime_elim.go — ELIM-1 「◎ 窗内可消除量总览」
// (rank_order_v2_design_20260712.md, GREENLIT user ruling 2026-07-12; ledger
// docs/design/real_trace_campaign_20260705.md §29.42.1 baseline / §29.61 b /
// §29.64 Stage-2 remit; RANK-U Stage 2 commits C+D).
//
// The overview is a READ-ONLY NAVIGATION layer over the root-cause board
// (§29.42.4 出厂权属: it transcribes typed published values and NEVER mints
// ordinals, badges, sums or crowns — the main board below stays byte-level
// authoritative for seats and wear). Position (EVOLUTION RECORD, user ruling
// 2026-07-13 mid-batch): the overview renders BEFORE the projection tree
// (先执摘后细节), superseding the GREENLIT draft's 「树 fence 后/明细表前」
// placement; its E#/seat pointers are forward references into the tree and
// board below (evidence ordinals are allocated at model build, so assembly
// order is position-independent):
//
//   - population = typed VALUED RANK-ITEM rows of the rendered model (chain ∪
//     ◇ adjacent channels, the two same-ruler channels — 目标线程窗内墙钟 ms);
//   - order = causal-tier blocks first (§29.61.12 ② user ruling 2026-07-14:
//     the ⛓ chain block renders WHOLE before the ◇ adjacent block — post-RSPA
//     the credential-less ◇ remainder seats can numerically dwarf the
//     credentialed causes and pure value order buried the proven causality),
//     then published EffectiveImpactMS DESC within each block (§29.22.1 同一
//     权威字段; the design EXPLICITLY REJECTS an eff×conf composite key and
//     any confidence demotion — R-d, §7.30 S1 「排序合成分数不得以 ms 硬事实
//     发布」), then home render order, then LineStart;
//   - TOP5 (K=5 aligned with §29.27.1; widening K requires a joint re-ruling,
//     design R-c) + ◇-max fallback row + honest empty-chain line + exclusion
//     footnote (⌗ caliber-side / target-self-state rows: 排除≠消失) + ▒
//     background pointer line;
//   - zero ordinals, zero badges (§29.42.1 verbatim; not even a transcribed
//     home chip — design §4 / open item O-1 resolved NO): the overview is NOT
//     a fourth §29.27.1 mark surface (design R7 boundary), so the 三面记号一致
//     invariant carries no obligation here;
//   - never a sum: wall-clock rows of one thread may overlap — 墙钟不可加和
//     (design R2, 层Σ被否先例), doubly enforced by the renderer emitting no
//     total and the pins scanning for none.
//
// Admission (design §2.2) is FOUR PRECISE TYPED ARMS — never a prose or score
// heuristic (CLAUDE.md: precise signals for hard gates):
//
//	共享臂  runtimeTraceProjRowSharedSeatArm (B1 extraction, one impl with the
//	        badge/lead valid-seat gate) + the data_gap tier guard;
//	种群臂  typed rank-item rows only (root_cause_ predicate family, a folded
//	        rank-lane twin, or the semantic entity that ADOPTED its twin's
//	        seat) — ✦-only / critical_blocking stanza rows are NOT invented
//	        into the population (ADJ-MINT fail-honest, design §6.3/O-5: the
//	        ◇ side degrades to a pointer line instead);
//	通道臂  display ordinal channel ∈ {chain, adjacent} (single source
//	        runtimeTraceProjRowOrdinalChannel; ▒ background NEVER enters —
//	        基石 C: cross-thread calibers have no defined in-window eliminable
//	        amount; E30/54.x adversarial forms are structurally unreachable);
//	口径臂+值形臂  the SHARED non-wall-clock value-caliber arm: typed
//	        ObservationRecord.Unit, registry token class, family count clamp,
//	        plus the typed ⌗ caliber_side tier guard for stale persisted forms.
//	        Count equivalents and composite scores are footnote material,
//	        never 汇排 members.
//
// Wording: the row transcription mints NO new per-row vocabulary — value
// (verbatim %.3f), channel identity (⛓ 链上/◇ 邻近 — glyph + the existing
// channel nouns, single emitter below), subject display name, class word,
// existing caliber words (family fold ladder), the Stage-1 self qualifier
// 「目标自身·确定性优化」 (§29.61.1 ruling ⑥: the ◇-era 「候选」 word drops when
// the row is on-chain; the ◇ epistemics live in the ◎ legend sentence, not
// per row — design §3.2, §29.36.4 冗余判据) and the row's E# pointer. The only
// new words are the ◎ region name and its legend sentence (design §3.4).
import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// runtimeTraceProjElimRankItemRow is the 种群臂: the row is a typed VALUED
// RANK-ITEM (the engine's root_cause_rank lane), in any of its four rendered
// carriages —
//
//	(a) the rank record itself (Predicate root_cause_<tier>/…, the exact
//	    family the observation producers mint — never a prose parse);
//	(b) a chain-lane row that FOLDED its same-segment rank twin (RNB — the
//	    rank identity rides RankFoldPeers);
//	(c) a semantic entity that ADOPTED its rank twin's seat at the SEM twin
//	    fold (survivor carries Rank / BackgroundRank; adjacent survivors keep
//	    the ◇ ordinal);
//	(d) ELIM-GAP 件A (§29.104.15, 2026-07-16): ANY row carrying the typed
//	    engine seat itself — Node.Rank > 0, the SAME precise signal the badge
//	    authority, the lead election and the 成因节点 gate read
//	    (runtimeTraceProjCauseNodeRow / runtimeTraceProjRowValidSeat; §29.30.1
//	    单门原则 ◎-side completion). The cust_total_del witness E15(+2) seat
//	    #2 rode the R1 same-fact absorb backfill (§29.67: the chain-view
//	    survivor adopts its rank twin's Rank, keeps its hop predicate) through
//	    the ×N merge — carriages (a)-(c) all rejected it (no root_cause_
//	    prefix, RankFoldPeers empty because RNB refuses merged hosts, no
//	    SemanticClass) and the board silently lost 根因排序#2. This arm covers
//	    every present and future Rank carrier (predicate seats, RNB folds, R1
//	    backfill, R2 member carriage, XLANE-1 absorb) by construction.
//
// Rows outside the rank population (✦-only mention-floor rows, raw
// critical_blocking stanza rows, hop/context rows) are honestly OUT — the
// overview never invents a population member (ADJ-MINT fail-honest; when
// ADJ-MINT lands and mints them as rank items they enter with zero changes
// here).
func runtimeTraceProjElimRankItemRow(row runtimeTraceProjTreeRow) bool {
	if strings.HasPrefix(strings.TrimSpace(row.Node.Predicate), "root_cause_") {
		return true
	}
	if len(row.RankFoldPeers) > 0 {
		return true
	}
	if row.Node.Rank > 0 {
		return true
	}
	if strings.TrimSpace(row.Node.SemanticClass) != "" &&
		(row.Node.Rank > 0 || row.Node.BackgroundRank > 0) {
		return true
	}
	return false
}

// runtimeTraceProjElimCaliberSideRow is the single overview-side boundary
// between wall-clock values and non-wall-clock/unknown-caliber values. The
// shared value helper consumes the authoritative Unit as well as registry and
// family carriers; IsCaliberSideRow retains the fail-closed belt for legacy
// persisted rows whose exact non-wall-clock class is unavailable. This helper
// changes display admission only — never rank, tier, channel, or value.
func runtimeTraceProjElimCaliberSideRow(node types.TraceCausalProjectionNode) bool {
	return node.IsCaliberSideRow() ||
		runtimeTraceProjNonWallClockValueCaliber(node) ||
		tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(node.TypeToken)) != tracequery.CausalCaliberSideNone
}

// runtimeTraceProjElimEligible is the four-arm ◎ admission gate (design §2.2).
// The seat arm (displayed seat 1..5) deliberately does NOT participate — the
// population is 持值 rank-item rows regardless of chips/badges (design fixed
// the "transcribe the ◇ chip" gap: ◇ rows may hold values without chips).
func runtimeTraceProjElimEligible(row runtimeTraceProjTreeRow) bool {
	// XLANE-1 裁定① (§29.104.17 ①, 用户批复 2026-07-16): a represented-by-
	// chain-seat demoted satellite (typed engine marker — the RSPA demotion
	// runs BEFORE rank assignment, so the ◇ row still carries its full-value
	// account and a Rank) is EXCLUDED from the ◎ population: its same physical
	// time is already fully represented by the thread's on-chain seat, and a
	// full-value ◇ bar beside that seat is the C-1 §29.88.10 visual double
	// count — contradicting the row's own tree sentence 「同段物理时间已由链上
	// 席全额代表…不重复参赛」. The dedicated disclosure footnote names every
	// excluded row (已由链上席代表(降道) — the assembler; 排除≠消失), the
	// closure-identity structure pin gains the lane, and the value face, the
	// engine ordinals, the tree-face seat and its mutual-pointer sentences
	// stay untouched.
	if row.Node.ChainAnchorRepresentedByChainSeat {
		return false
	}
	// XLANE-2 件1 (§29.104.1/.2 定谳④, 2026-07-17): a member-subset demoted
	// semantic seat (display verdict — complete typed line-range set ⊂ a
	// same-board same-subject seat's set) is EXCLUDED the same way: its
	// physical spans are already fully represented by the superset seat, and
	// a full-value bar beside it is the same visual double count. The
	// dedicated subset footnote names every excluded row (排除≠消失); values,
	// engine ordinals and the tree-face seat stay untouched.
	if strings.TrimSpace(row.SemanticMemberSubsetOf) != "" {
		return false
	}
	// LEVELMERGE-1 件2 (方案 P, 2026-07-18): the demoted gated-share
	// CONSTITUENT row is EXCLUDED the same way — its claimed share is already
	// counted inside the same thread's priority-inversion seat's gated
	// composite, and a full-value ◇ bar beside that seat is the identical
	// visual double count (its own tree sentence says 不参赛、不与之相加).
	// The dedicated disclosure footnote names every excluded row (排除≠消失);
	// the residual (B) seat keeps participating with its residual value.
	if row.Node.GatedShareConstituentSeat {
		return false
	}
	return runtimeTraceProjElimEligibleSansRepresented(row)
}

// runtimeTraceProjElimGatedConstituentExcluded reports a row the LEVELMERGE-1
// 件2 constituent arm alone keeps off the ◎ population — the disclosure
// footnote's census predicate (same gate body, so the footnote and the
// exclusion can never fork). A constituent row never carries the represented
// or subset markers (engine clone of a chain aggregate seat), so the three
// exclusion footnotes can never double-count one row.
func runtimeTraceProjElimGatedConstituentExcluded(row runtimeTraceProjTreeRow) bool {
	if !row.Node.GatedShareConstituentSeat || row.Node.ChainAnchorRepresentedByChainSeat ||
		strings.TrimSpace(row.SemanticMemberSubsetOf) != "" {
		return false
	}
	return runtimeTraceProjElimEligibleSansRepresented(row)
}

// runtimeTraceProjElimMemberSubsetExcluded reports a row the XLANE-2 件1
// subset arm alone keeps off the ◎ face — either off the population (a seated
// subset row) or off the semantic census footnote (the witness E35/E49
// seatless form). The dedicated footnote's census predicate (same gate
// bodies, so the footnote and the exclusions can never fork).
func runtimeTraceProjElimMemberSubsetExcluded(row runtimeTraceProjTreeRow) bool {
	if strings.TrimSpace(row.SemanticMemberSubsetOf) == "" ||
		row.Node.ChainAnchorRepresentedByChainSeat {
		return false
	}
	return runtimeTraceProjElimEligibleSansRepresented(row) || runtimeTraceProjElimSemanticCensusRow(row)
}

// runtimeTraceProjElimRepresentedExcluded reports a row the 裁定① arm alone
// keeps off the ◎ population — the disclosure footnote's census predicate
// (same gate body, so the footnote and the exclusion can never fork).
func runtimeTraceProjElimRepresentedExcluded(row runtimeTraceProjTreeRow) bool {
	return row.Node.ChainAnchorRepresentedByChainSeat && runtimeTraceProjElimEligibleSansRepresented(row)
}

// runtimeTraceProjElimSemanticCensusRow is the SEATLESS semantic census
// population predicate (RNB-2 件4 W4-a footnote lane, factored 2026-07-17 for
// XLANE-2 件1 so the census scan and the subset footnote can never fork): a
// valued ⛓/◇ semantic row outside the rank population and off the ⌗
// caliber-side lane.
func runtimeTraceProjElimSemanticCensusRow(row runtimeTraceProjTreeRow) bool {
	if !row.HasData || row.Node.OnChainOverflowFold {
		return false
	}
	if strings.TrimSpace(row.Node.SemanticClass) == "" || runtimeTraceProjElimRankItemRow(row) {
		return false
	}
	if runtimeTraceProjElimCaliberSideRow(row.Node) {
		return false
	}
	if runtimeTraceProjNodeDisplayImpact(row.Node) <= 0 {
		return false
	}
	switch runtimeTraceProjRowOrdinalChannel(row) {
	case runtimeTraceProjOrdinalChannelChain, runtimeTraceProjOrdinalChannelAdjacent:
		return true
	}
	return false
}

// runtimeTraceProjElimEligibleSansRepresented is the pre-裁定① admission body
// (every arm except the represented exclusion above).
func runtimeTraceProjElimEligibleSansRepresented(row runtimeTraceProjTreeRow) bool {
	// Value caliber outranks every special carriage. In particular, a stale or
	// future micro-fold marker must never let a composite score/count equivalent
	// enter the millisecond ruler merely because the fold arm returns early.
	if runtimeTraceProjElimCaliberSideRow(row.Node) {
		return false
	}
	// RNB-5B 件⑦ (§29.96.2 终判⑦, 2026-07-15): the micro anchored-cut-seat
	// fold row IS the board representation of its folded ⛓ members (they were
	// individually eligible before the fold; 零静默消失) — admitted on its
	// typed marker, rendered by the dedicated fold line in
	// runtimeTraceProjElimRowLine.
	if row.HasData && row.Node.MicroAnchorFold && row.Node.EffectiveImpactMS > 0 {
		return true
	}
	// 共享臂 (B1) + the data_gap belt: data blind spots are diagnostics, never
	// eliminable amounts (they arrive value-less today; the typed tier guard
	// keeps stale persisted forms out — defense in depth, same style as the
	// valid-seat channel belt).
	if !runtimeTraceProjRowSharedSeatArm(row) ||
		strings.TrimSpace(row.Node.Tier) == types.TraceCausalTierDataGap {
		return false
	}
	if !runtimeTraceProjElimRankItemRow(row) {
		return false
	}
	// 通道臂 — the display channel authority (stanza placement outranks stale
	// relevance, §29.36.2 三面同一来源).
	switch runtimeTraceProjRowOrdinalChannel(row) {
	case runtimeTraceProjOrdinalChannelChain, runtimeTraceProjOrdinalChannelAdjacent:
	default:
		return false
	}
	return true
}

// runtimeTraceProjElimEntry is one assembled overview member: the row plus its
// typed home-channel sort keys (channelRank: chain=0 ◇=1; homeOrder: the
// row's render position inside its home channel — key 3 transcribes the
// engine's own order, never re-ranking by confidence).
type runtimeTraceProjElimEntry struct {
	row         runtimeTraceProjTreeRow
	channelRank int
	homeOrder   int
}

// runtimeTraceProjElimBoard collects and orders the full eligible population
// (pre-TOP5 — the caller slices; 排除脚注 material is collected separately).
// 榜语义定谳 (RULE3-1 件12⑧, §29.185②, 2026-07-21): the eliminable board is
// an OPTIMIZATION-REMINDER face — 能优化的尽量提醒; counterfactual validity
// never hides a seat here (invalid shares disclose beside the seat, stage
// two).
// Chain-channel rows live in TreeRows and SelfRows (self-cause four-family
// rows are ranked contenders); adjacent-channel rows live in the ◇ stanza.
// The population is post-aggregation single seats (§29.50.5: merged [E#(+N)]
// rows enter once; folded twins never resurrect — pinned: overview rows ≤
// home valued rows).
func runtimeTraceProjElimBoard(model runtimeTraceProjTreeModel) []runtimeTraceProjElimEntry {
	var entries []runtimeTraceProjElimEntry
	order := 0
	collect := func(rows []runtimeTraceProjTreeRow) {
		for i := range rows {
			row := rows[i]
			if !runtimeTraceProjElimEligible(row) {
				continue
			}
			// The overview is a selected-window principal value surface. A
			// neighboring drilldown board may remain visible as context, but it
			// cannot keep a rank seat or enter a direction subtotal here.
			if model.PrincipalWindowAuthoritative && !types.TraceCausalProjectionNodeMatchesPrincipalWindow(
				row.Node, model.WindowStartTs, model.WindowEndTs,
			) {
				continue
			}
			channelRank := 0
			if runtimeTraceProjRowOrdinalChannel(row) == runtimeTraceProjOrdinalChannelAdjacent {
				channelRank = 1
			}
			order++
			entries = append(entries, runtimeTraceProjElimEntry{row: row, channelRank: channelRank, homeOrder: order})
		}
	}
	collect(model.SelfRows)
	collect(model.TreeRows)
	collect(model.Adjacent)
	// RNB-1 C-1 (§29.88.10 R7-1, 2026-07-14): same-thread same-value dual
	// seats converge to ONE ◎ seat. The inversion lane and the runnable
	// census lane can each seat the SAME physical time (witness
	// 20260714-230952: JankManager-9655 0.423 ×2 — E31 inversion seat + E32
	// runnable seat); the board is a value index, so the pair reads as a
	// visual double count even under the 零求和 rule. Precise signals only
	// (same-fact absorb 判例, §29.83 件① / 树面「同段两车道已合并为一行」
	// 先例): same channel ∧ same canonical subject ∧ µs-equal published eff ∧
	// the typed same-physical-time proof (成员值 µs 全等 class — the
	// inversion seat's eff is PURELY its gated runnable overlap and µs-equals
	// the runnable seat's whole published account: an equal-measure subset of
	// the same runnable segments is the same physical time). The inversion
	// seat survives (the causal identity face), the runnable twin's E# joins
	// its bracket ([E#+E#] — the tree merged-lane form); the detail faces
	// keep both lanes' accounts untouched.
	entries = runtimeTraceProjElimConvergeDualSeats(entries)
	// §29.61.12 ② (用户裁定 2026-07-14, INV-SUPPLY 件④). EVOLUTION RECORD:
	// the board order was ONE pure eff-desc list (纯值降序, chain-first only
	// on exact ties); it is now CAUSAL-TIER BLOCKED — the ⛓ chain block
	// renders whole before the ◇ adjacent block, each block internally
	// eff-desc (语义更贴切: post-RSPA the credential-less ◇ remainder seats
	// can numerically dwarf the credentialed ⛓ causes — h6 witness ◇ 33.159
	// vs ⛓ 3.598 — and pure value order let them visually bury the proven
	// causality). The header promise sentence and the ◎ legend entry moved
	// in lockstep; the bar ruler stays the SECTION-WIDE maximum (short chain
	// bars are honest — see the fence assembler).
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.channelRank != b.channelRank {
			return a.channelRank < b.channelRank // ⛓ 块在前 (typed channel)
		}
		if a.row.Node.EffectiveImpactMS != b.row.Node.EffectiveImpactMS {
			return a.row.Node.EffectiveImpactMS > b.row.Node.EffectiveImpactMS
		}
		if a.homeOrder != b.homeOrder {
			return a.homeOrder < b.homeOrder // transcribed engine order
		}
		return a.row.Node.LineStart < b.row.Node.LineStart
	})
	return entries
}

// runtimeTraceProjElimDualSeatSameSegment is the C-1 typed same-physical-time
// proof between an inversion-lane seat and a runnable-lane seat of one thread
// (WO-A1 成员值 µs 全等 judgment class): the inversion seat's eff carries NO
// running component (GatedRunningDeficitMS == 0) and its gated runnable
// overlap µs-equals the runnable seat's whole published account — an
// equal-measure subset of the same runnable segments is the same physical
// time. Never a fuzzy value-proximity match.
func runtimeTraceProjElimDualSeatSameSegment(a, b types.TraceCausalProjectionNode) (secondIsInversion, ok bool) {
	inv, run := a, b
	invSecond := false
	if !runtimeTraceCausalProjectionInversionRow(inv) {
		inv, run = b, a
		invSecond = true
	}
	if !runtimeTraceCausalProjectionInversionRow(inv) || runtimeTraceCausalProjectionInversionRow(run) {
		return false, false
	}
	if strings.TrimSpace(strings.ToLower(run.StateKind)) != "runnable" {
		return false, false
	}
	if !runtimeTraceProjRound3Equal(inv.EffectiveImpactMS, run.EffectiveImpactMS) {
		return false, false
	}
	if inv.GatedRunningDeficitMS != 0 || inv.GatedRunnableMS <= 0 ||
		!runtimeTraceProjRound3Equal(inv.GatedRunnableMS, run.EffectiveImpactMS) {
		return false, false
	}
	return invSecond, true
}

// runtimeTraceProjElimConvergeDualSeats folds C-1 pairs inside one channel:
// the inversion seat survives, the runnable twin's evidence tag joins its
// bracket. Pairs only (a third same-value seat stays — ambiguity keeps rows,
// 禁猜); every other entry passes through byte-identically.
func runtimeTraceProjElimConvergeDualSeats(entries []runtimeTraceProjElimEntry) []runtimeTraceProjElimEntry {
	out := entries[:0]
	for _, entry := range entries {
		converged := false
		for k := range out {
			if out[k].channelRank != entry.channelRank {
				continue
			}
			if runtimeTraceCausalProjectionCanonicalNode(out[k].row.Node.Subject) !=
				runtimeTraceCausalProjectionCanonicalNode(entry.row.Node.Subject) {
				continue
			}
			secondIsInversion, ok := runtimeTraceProjElimDualSeatSameSegment(out[k].row.Node, entry.row.Node)
			if !ok {
				continue
			}
			keptTag := strings.TrimSpace(out[k].row.EvidenceTag)
			newTag := strings.TrimSpace(entry.row.EvidenceTag)
			if secondIsInversion {
				// The later (inversion) row survives in the earlier slot —
				// keep the earlier home order (transcribed engine order).
				home := out[k].homeOrder
				out[k].row = entry.row
				out[k].homeOrder = home
				keptTag, newTag = newTag, keptTag
			}
			if keptTag != "" && newTag != "" {
				out[k].row.EvidenceTag = keptTag + "+" + newTag
			} else if newTag != "" {
				out[k].row.EvidenceTag = newTag
			}
			converged = true
			break
		}
		if !converged {
			out = append(out, entry)
		}
	}
	return out
}

// runtimeTraceProjElimTopN is the overview population bound (design §2.5:
// K=5 aligned with the §29.27.1 badge TOP N; the two constants may only move
// together through a joint re-ruling — pinned).
const runtimeTraceProjElimTopN = runtimeTraceProjBadgeTopN

// runtimeTraceProjElimAdjacentTopN is the ◇ zone's own display bound
// (OMGCLEAN-1, §29.175.9 用户裁定: ◇ 邻近区=多行 TOP3 + 尾部计数 — the three
// non-direction zones ◈/◇/▒ share the TOP3 多行制; everything past the
// display counts into the 未入榜/尾部 disclosures, 排除≠消失).
const runtimeTraceProjElimAdjacentTopN = 3

// --- ELIM-V2 方向分组制 (设计终稿 elim_v2_spec.md, 用户授权 2026-07-18) --------
//
// The ⛓ chain block renders in FIX-DIRECTION SECTIONS (节=修复方向): section
// order = per-section max eliminable DESC, section-internal order = the
// board's published-eff order untouched, the unresolved/composite tail
// section always last (fail-open — the display NEVER re-derives a direction
// from type names; the section key is the engine-published registry token
// verbatim, AXIOM-V2 件1 单一权威). The ◇ adjacent block stays WHOLE and
// unsectioned after the chain block (§29.61.12 ② preserved) with a per-row
// ·方向=X transcription word. 防跨方向相加三层: the head declaration
// 方向间收益不可相加 (恒发 with the form promise), the ·∩[E#] chip on real
// typed overlap pairs only (件2 wire carrier; carrier absent → nothing), and
// ONE merged pair footnote — the authoritative full 互指句 stays on the tree
// rows (§29.42.4 出厂权属: ◎ transcribes, never mints).

// runtimeTraceProjElimSection is one rendered chain-block direction section.
type runtimeTraceProjElimSection struct {
	direction string // registry token verbatim; "" = 方向未定/复合 tail
	entries   []runtimeTraceProjElimEntry
	maxEff    float64
}

// runtimeTraceProjElimSectionsFor groups the RENDERED chain members by their
// engine-published fix direction. Unknown/unresolved tokens fall into the ""
// tail section (fail-open; a token outside the display word table is treated
// as unresolved — 显示侧零词面推断, the word table is the closed set).
func runtimeTraceProjElimSectionsFor(chain []runtimeTraceProjElimEntry) []runtimeTraceProjElimSection {
	index := map[string]int{}
	var sections []runtimeTraceProjElimSection
	for _, entry := range chain {
		direction := strings.TrimSpace(entry.row.Node.FixDirection)
		if _, ok := runtimeTraceProjFixDirectionWord(direction, true); !ok {
			direction = ""
		}
		at, ok := index[direction]
		if !ok {
			at = len(sections)
			index[direction] = at
			sections = append(sections, runtimeTraceProjElimSection{direction: direction})
		}
		sections[at].entries = append(sections[at].entries, entry)
		if eff := entry.row.Node.EffectiveImpactMS; eff > sections[at].maxEff {
			sections[at].maxEff = eff
		}
	}
	sort.SliceStable(sections, func(i, j int) bool {
		// 未定/复合 tail section is ALWAYS last (⛓ 块内、◇ 前 — fail-open
		// material never outranks resolved directions).
		if (sections[i].direction == "") != (sections[j].direction == "") {
			return sections[j].direction == ""
		}
		return sections[i].maxEff > sections[j].maxEff // 节序=节内最大可消降序
	})
	return sections
}

// runtimeTraceProjElimSectionArithmetic is the 小计阶梯 verdict (防假算术:
// arithmetic only on proof, silence otherwise — 宁漏勿假指).
type runtimeTraceProjElimSectionArithmetic = types.TraceAnswerDirectionArithmetic

const (
	// elimSectionArithmeticNone — single seat / 未定节 / carrier absent (L3)
	// / cross-board members: zero arithmetic, the head speaks 最大可消 only.
	elimSectionArithmeticNone = types.TraceAnswerDirectionArithmeticNone
	// elimSectionArithmeticSubtotal — L1: every member carries a faithful
	// typed envelope and the envelopes are pairwise exclusive → Σ 小计.
	elimSectionArithmeticSubtotal = types.TraceAnswerDirectionArithmeticSubtotal
	// elimSectionArithmeticOverlap — L2: faithful envelopes measurably
	// overlap → the seat count plus 合计不可直加, never a Σ.
	elimSectionArithmeticOverlap = types.TraceAnswerDirectionArithmeticOverlap
)

// runtimeTraceProjElimEnvelopeToleranceMs mirrors the engine checker's
// µs-scale float tolerance (directionConservationToleranceMs — the L1/L2 fork
// fires on real shared wall clock, never on float dust).
const runtimeTraceProjElimEnvelopeToleranceMs = types.TraceCausalProjectionFaithfulEnvelopeOverlapToleranceMS

// runtimeTraceProjElimSectionLadder resolves the section's arithmetic tier.
// PRECISE typed signals only:
//
//   - L1 (Σ 小计) requires ≥2 seats, a resolved direction, one board, and a
//     FAITHFUL per-seat envelope (typed StartTs/EndTs present, MergedCount ≤ 1
//     — a merged row's envelope may understate its cross-window account, so
//     merged carriers step down; family hull envelopes contain every member
//     span and stay in) with all envelopes pairwise exclusive: envelope
//     disjointness ⇒ support disjointness (support ⊆ envelope), so the Σ of
//     published values double-bills no physical time. subtotal = Σ of the
//     µs-rounded member values — reconstructible from the rendered rows
//     (原始值可见性三问③: integer-µs identity, pinned).
//   - L2 (合计不可直加) fires when faithful envelopes measurably overlap.
//   - everything else (missing envelope, merged carrier, cross-board, 未定节,
//     single seat) publishes NO arithmetic (L3 载体缺席 → 零算术).
//
// 修补轮 件3 (2026-07-18): the one-board proof reads the WHOLE board's
// identity context — under a multi-board ruler head, or when a member's own
// board identity is MISSING while the board carries named targets (the
// {空,具名} mixed form: the bare seat could belong to ANY of them), the
// single-board premise is unproven → L3 (缺席不进算术, 宁漏勿假).
func runtimeTraceProjElimSectionLadder(section runtimeTraceProjElimSection, multiBoardRuler, boardHasNamedTargets bool) (runtimeTraceProjElimSectionArithmetic, float64) {
	members := make([]types.TraceCausalProjectionNode, 0, len(section.entries))
	for _, entry := range section.entries {
		members = append(members, entry.row.Node)
	}
	return types.TraceAnswerDirectionSectionArithmetic(
		section.direction, members, multiBoardRuler, boardHasNamedTargets,
	)
}

// runtimeTraceProjElimPrintedUs returns the integer µs of a value's PRINTED
// %.3f face (修补轮 件6③ — the subtotal's one rounding authority is the byte
// face the member row publishes, identical to the pin-side reconstruction:
// parse the printed decimal back, then the +0.5 floor lands on an exact
// integer because a 3-decimal decimal's nearest binary sits within 2⁻²⁰ µs).
func runtimeTraceProjElimPrintedUs(v float64) int64 {
	printed, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 3, 64), 64)
	if err != nil { // unreachable on FormatFloat output; honest fallback
		printed = v
	}
	return int64(printed*1000 + 0.5)
}

// runtimeTraceProjElimSectionHeadLine renders one ▸ section head (节头即答案:
// direction word + 最大可消 恒发; seat count and the subtotal ladder attach
// only on their proof tiers; a single-seat section keeps the bare head —
// 单席节头不合并进席行, 委托默认). The 最大可消 value is the section's
// largest member value VERBATIM (%.3f — the same bytes its member row prints;
// 原始值可见性三问①: the original lives on the row below).
//
// OMGCLEAN-1 件7 (§29.175 处置, 2026-07-20). EVOLUTION RECORD — 涉既裁位移④
// (§29.133 修补轮件G 原裁: every member line names its board anchor under the
// multi-board ruler — 「on the multi-target-board form every member line
// names its board anchor」): the per-row obligation survives; it merely
// HOISTS when unanimity makes the repetition pure 套话 (§29.175.1) —
// hoistedAnchor non-empty = every section member carries the SAME typed board
// target (canonical unanimity, computed by the assembler) — the ·板锚 chip
// hoists onto the section head ONCE and the member rows drop theirs; a mixed
// or partially-bare section passes "" and keeps the §29.133 件G per-row
// anchors byte-identically. Same verbatim label and legend home as the row
// chip (one word family, one mark).
func runtimeTraceProjElimSectionHeadLine(section runtimeTraceProjElimSection, multiBoardRuler, boardHasNamedTargets bool, hoistedAnchor string, marks *runtimeTraceProjMarkSet, zh bool) string {
	marks.mark(runtimeTraceProjMarkElimDirectionSection)
	word, resolved := runtimeTraceProjFixDirectionWord(section.direction, zh)
	if !resolved {
		// OMGCLEAN-1 件1 (§29.175 裁定② rename; design G3, 2026-07-20).
		// EVOLUTION RECORD: the tail word was 「方向未定/复合」/"direction
		// unresolved/composite" — the user read it as an unfinished-analysis
		// claim (「既然都有可消除量了,为啥还是 向未定」). The new word states
		// set-membership (outside the six-direction closed set) instead of an
		// analysis state; the honest fail-open semantics live on the legend
		// entry. The internal mark name (…ElimDirectionUnresolved) is an
		// identifier, not a word face, and deliberately keeps its name.
		marks.mark(runtimeTraceProjMarkElimDirectionUnresolved)
		if zh {
			word = "其他方向"
		} else {
			word = "other directions"
		}
	}
	var b strings.Builder
	if zh {
		b.WriteString(tracefence.ElimSectionGlyph + " " + word + fmt.Sprintf(" · 最大可消 %.3fms", section.maxEff))
	} else {
		b.WriteString(tracefence.ElimSectionGlyph + " " + word + fmt.Sprintf(" · max eliminable %.3fms", section.maxEff))
	}
	tier, subtotal := runtimeTraceProjElimSectionLadder(section, multiBoardRuler, boardHasNamedTargets)
	switch tier {
	case elimSectionArithmeticSubtotal:
		marks.mark(runtimeTraceProjMarkElimSectionSubtotal)
		if zh {
			b.WriteString(fmt.Sprintf(" · %d席 · 小计 %.3fms(区间互斥)", len(section.entries), subtotal))
		} else {
			b.WriteString(fmt.Sprintf(" · %d seats · subtotal %.3fms (disjoint intervals)", len(section.entries), subtotal))
		}
	case elimSectionArithmeticOverlap:
		marks.mark(runtimeTraceProjMarkElimSectionNonAddable)
		if zh {
			b.WriteString(fmt.Sprintf(" · %d席 · 成员区间重叠,合计不可直加", len(section.entries)))
		} else {
			b.WriteString(fmt.Sprintf(" · %d seats · member intervals overlap; do not add", len(section.entries)))
		}
	}
	if hoistedAnchor != "" {
		if zh {
			b.WriteString(" ·板锚 " + hoistedAnchor)
		} else {
			b.WriteString(" · board " + hoistedAnchor)
		}
		marks.mark(runtimeTraceProjMarkRankBoardAnchor)
	}
	return b.String()
}

// runtimeTraceProjElimSectionHoistedAnchor (件7) resolves a section's hoisted
// board anchor: the verbatim target label when EVERY member carries a typed
// RankBoardTarget and the canonical forms all agree — "" otherwise (a bare or
// mixed roster keeps per-row anchors; absence never hoists a claim).
func runtimeTraceProjElimSectionHoistedAnchor(section runtimeTraceProjElimSection) string {
	label := ""
	key := ""
	for _, entry := range section.entries {
		target := strings.TrimSpace(entry.row.Node.RankBoardTarget)
		if target == "" {
			return ""
		}
		canonical := runtimeTraceCausalProjectionCanonicalNode(target)
		if key == "" {
			key, label = canonical, target
			continue
		}
		if canonical != key {
			return ""
		}
	}
	return label
}

// runtimeTraceProjElimAdjacentBlockHeadLine renders the ◇ block head that
// separates the direction sections from the unsectioned adjacent block
// (条件可消上界 never enters the direction conservation population —
// axiom_v2 链上硬纪律 1; emitted only when both structures are present, the
// separator role).
func runtimeTraceProjElimAdjacentBlockHeadLine(marks *runtimeTraceProjMarkSet, zh bool) string {
	marks.mark(runtimeTraceProjMarkElimAdjacentBlockHead)
	if zh {
		return runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelAdjacent, true) + "(条件可消上界 · 不入方向守恒)"
	}
	return runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelAdjacent, false) + " (conditional upper bound · outside direction conservation)"
}

// runtimeTraceProjElimCrossDirectionFootnote builds the ∩ pair rows of the
// auxiliary zone (三层防相加的第三层): one row per deduped resolved pair with
// its typed overlap wall clock. The full mutual sentence's authority stays on
// the tree rows and the ∩ legend entry (◎ 只转录 — the per-row clause is the
// short 收益不叠加 fact plus the pointer; §29.175.14 同级行纪律 replaced the
// former one merged multi-pair line). Zero resolved pairs → zero rows
// (载体缺席不发, 宁漏勿假指).
func runtimeTraceProjElimCrossDirectionFootnote(rendered []runtimeTraceProjElimEntry, marks *runtimeTraceProjMarkSet, zh bool) []runtimeTraceProjElimAuxRow {
	seen := map[string]bool{}
	type overlapRow struct {
		row       runtimeTraceProjElimAuxRow
		overlapMS float64
	}
	var pairs []overlapRow
	var rows []runtimeTraceProjElimAuxRow
	for _, entry := range rendered {
		tag := strings.TrimSpace(entry.row.EvidenceTag)
		if tag == "" {
			continue
		}
		for _, clause := range entry.row.CrossDirectionOverlapClauses {
			ref := strings.TrimSpace(clause.Ref)
			if ref == "" {
				continue
			}
			a, b := tag, ref
			if b < a {
				a, b = b, a
			}
			key := fmt.Sprintf("%s∩%s@%.3f", a, b, clause.OverlapMS)
			if seen[key] {
				continue
			}
			seen[key] = true
			// 双复核修复 件6 (冷读 CR5/对抗 CR-10, §29.175.8 定稿逐字,
			// 2026-07-21). EVOLUTION RECORD: the content column led with the
			// 「重叠」 prefix word and closed on the 「全句见树行」 pointer —
			// the 定稿 form puts the VALUE right after the pair id (值在首段)
			// and speaks the one ruled short clause 「修其一,另一席收益随之
			// 收缩,不叠加」; the full mutual sentence's authority stays on the
			// tree rows + the ∩ legend entry. Two resolved pairs stay two
			// complete same-level rows (合并不做 — 候裁, 勿动).
			if zh {
				pairs = append(pairs, overlapRow{overlapMS: clause.OverlapMS,
					row: runtimeTraceProjElimAuxRow{label: "∩ 重叠对",
						content: fmt.Sprintf("[%s]∩[%s] %.3fms · 修其一,另一席收益随之收缩,不叠加", tag, ref, clause.OverlapMS)}})
			} else {
				pairs = append(pairs, overlapRow{overlapMS: clause.OverlapMS,
					row: runtimeTraceProjElimAuxRow{label: "∩ overlap",
						content: fmt.Sprintf("[%s]∩[%s] %.3fms · fix one and the other seat's gain shrinks; never additive", tag, ref, clause.OverlapMS)}})
			}
		}
	}
	// A2 件12 (§29.192, 2026-07-21): the ∩ family is a proliferable aux
	// family — TOP3 by overlap value (desc, stable on ties) + an honest tail
	// count (与 ◈◇▒ 同文法). Each rendered pair keeps the §29.182③ 定稿
	// complete-sentence form; the full mutual sentences' authority stays on
	// the tree rows, so the tail points there.
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].overlapMS > pairs[j].overlapMS })
	const elimOverlapPairTopN = 3
	for i, pair := range pairs {
		if i >= elimOverlapPairTopN {
			break
		}
		rows = append(rows, pair.row)
	}
	if rest := len(pairs) - elimOverlapPairTopN; rest > 0 {
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "∩ 重叠对",
				content: fmt.Sprintf("另有 %d 对见树行", rest)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "∩ overlap",
				content: fmt.Sprintf("%d more pair(s) — see the tree rows", rest)})
		}
	}
	if len(rows) > 0 {
		marks.mark(runtimeTraceProjMarkElimCrossDirectionChip)
	}
	return rows
}

// runtimeTraceProjElimConservationLines transcribes the AXIOM-V2 件3 checker
// verdict as the ◎ 守恒尾行 (委托默认,待人工追认):
//
//   - violation findings (typed Node.DirectionConservationExcess, identical
//     across the member seats — deduped per tuple) render one per-direction
//     disclosure line each (立案素材; §29.104.13 非致命不硬拦 — 纯披露);
//   - the clean shape renders the standing pass line 「各方向支撑区间并集皆 ≤
//     窗W ms(检查器)」 — gated on a typed proof the checker generation ran
//     (≥1 board seat carries the engine-published fix direction; the stamp
//     and the checker ship on ONE finalize tail) plus a known window (绝不猜
//     窗). Legacy boards without the direction generation render neither.
func runtimeTraceProjElimConservationLines(model runtimeTraceProjTreeModel, board []runtimeTraceProjElimEntry, chainRendered bool, zh bool) []runtimeTraceProjElimAuxRow {
	// 修补轮 件6① key shape: the engine mints ONE finding per (thread,
	// direction) group — the dedup key carries the carrying seat's thread
	// anchor (Subject, board-target fallback) beside the tuple, so two
	// different-thread groups that coincidentally publish identical numbers
	// keep their two disclosure lines (one per engine group, 键形对齐).
	type findingKey struct {
		anchor    string
		direction string
		sumUs     int64
		windowUs  int64
		seats     int
	}
	seen := map[findingKey]bool{}
	var findings []*types.TraceCausalProjectionDirectionConservation
	directionGeneration := false
	scanNode := func(node types.TraceCausalProjectionNode) {
		finding := node.DirectionConservationExcess
		if finding == nil {
			return
		}
		anchor := runtimeTraceCausalProjectionCanonicalNode(node.Subject)
		if anchor == "" {
			anchor = runtimeTraceCausalProjectionCanonicalNode(node.RankBoardTarget)
		}
		key := findingKey{
			anchor:    anchor,
			direction: finding.Direction,
			sumUs:     int64(finding.SumMS*1000 + 0.5),
			windowUs:  int64(finding.WindowMS*1000 + 0.5),
			seats:     finding.SeatCount,
		}
		if seen[key] {
			return
		}
		seen[key] = true
		findings = append(findings, finding)
	}
	for _, entry := range board {
		if entry.channelRank == 0 && strings.TrimSpace(entry.row.Node.FixDirection) != "" {
			directionGeneration = true
		}
		scanNode(entry.row.Node)
	}
	// 修补轮 件7 (2026-07-18): the violation scan covers the PRE-EXCLUSION
	// population — the three ◎ exclusion arms (represented / member-subset /
	// gated-constituent) demote a row off the value board, but the checker
	// verdict it carries is engine truth about the direction population and
	// must never vanish with the seat (排除≠消失 extends to the 守恒 tail: a
	// violating group whose every carrier is display-excluded would otherwise
	// flip the tail to the PASS claim). Same census predicates as the
	// disclosure footnotes (one gate body, the scan can never fork from them);
	// the pass gate itself still reads the RENDERED population only.
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if runtimeTraceProjElimRepresentedExcluded(rows[i]) ||
				runtimeTraceProjElimMemberSubsetExcluded(rows[i]) ||
				runtimeTraceProjElimGatedConstituentExcluded(rows[i]) {
				scanNode(rows[i].Node)
			}
		}
	}
	label := "守恒"
	if !zh {
		label = "conservation"
	}
	if len(findings) > 0 {
		sort.SliceStable(findings, func(i, j int) bool {
			if findings[i].Direction != findings[j].Direction {
				return findings[i].Direction < findings[j].Direction
			}
			return findings[i].SumMS > findings[j].SumMS
		})
		var rows []runtimeTraceProjElimAuxRow
		for _, finding := range findings {
			word, ok := runtimeTraceProjFixDirectionWord(finding.Direction, zh)
			if !ok {
				word = finding.Direction // honest verbatim token (absence never renames)
			}
			if zh {
				rows = append(rows, runtimeTraceProjElimAuxRow{label: label, content: fmt.Sprintf(
					"违例:方向 %s 支撑区间并集合计 %.3fms > 窗 %.3fms(%d席,同线程)——同段时间重复计费(检查器,仅披露不改值)",
					word, finding.SumMS, finding.WindowMS, finding.SeatCount)})
			} else {
				rows = append(rows, runtimeTraceProjElimAuxRow{label: label, content: fmt.Sprintf(
					"excess: direction %s support-interval unions sum %.3fms > window %.3fms (%d seats, one thread) — same time double-billed (checker; disclosure only, values unchanged)",
					word, finding.SumMS, finding.WindowMS, finding.SeatCount)})
			}
		}
		model.Marks.mark(runtimeTraceProjMarkElimConservation)
		return rows
	}
	if !chainRendered || !directionGeneration || model.WindowMS <= 0 {
		return nil
	}
	model.Marks.mark(runtimeTraceProjMarkElimConservation)
	// 双复核修复 件6 (冷读 CR5/对抗 CR-10, §29.175.8 定稿逐字, 2026-07-21).
	// EVOLUTION RECORD: the pass row spoke 「…并集皆 ≤ 窗 …(检查器)」 — the
	// 「(检查器)」 tail was an internal machinery name (件10 sweep 精神) and
	// 「皆」 padded the clause; the 定稿 pass form closes on the bare ✓. The
	// checker semantics stay on the 守恒 legend entry; violation rows keep
	// their per-direction disclosure form unchanged.
	if zh {
		return []runtimeTraceProjElimAuxRow{{label: label,
			content: fmt.Sprintf("各方向支撑区间并集 ≤ 窗 %.3fms ✓", model.WindowMS)}}
	}
	return []runtimeTraceProjElimAuxRow{{label: label,
		content: fmt.Sprintf("every direction's support-interval union ≤ window %.3fms ✓", model.WindowMS)}}
}

// runtimeTraceProjElimChannelWord is the ONE emitter of the overview channel
// identity word (design §3: 单源转录 ChainRelevance — the display channel
// authority; glyphs are the existing channel marks, nouns the existing
// channel vocabulary; 禁词面匹配, single emission point).
//
// §29.61.12 ① (用户裁定 2026-07-14, INV-SUPPLY 件④). EVOLUTION RECORD: the
// glyph and the channel noun gained a separating space (`⛓链上` → `⛓ 链上`,
// `◇邻近` → `◇ 邻近`; en likewise) — 记号词距, the md face is the authority
// and every consumer (rows, header promise, legend backticks, pins) moves in
// lockstep through THIS one emitter plus the legend entry.
func runtimeTraceProjElimChannelWord(channel string, zh bool) string {
	if channel == runtimeTraceProjOrdinalChannelAdjacent {
		if zh {
			return "◇ 邻近"
		}
		return "◇ adjacent"
	}
	if zh {
		return "⛓ 链上"
	}
	return "⛓ on-chain"
}

// runtimeTraceProjElimCaliberNote transcribes the row's published caliber
// word VERBATIM from the existing single-source vocabulary (design §2.3 口径
// 注记逐字转录, 零新铸; RANK-U Stage 2 收尾件1, 2026-07-13: a ◎ row carrying
// a DISCOUNTED/lower-bound/single-max value must say so — the W1 TOP1
// 「3.175ms · running」 rendered wordless while its home 行3 spoke
// 折算,按全域最大核最高频 over a 3.860 raw). Arms, most specific first, every word
// from its existing single-source composer with its legend mark lit at THIS
// emission site (词条-图例双向; caliber-group entries render in stable
// catalog order, so the extra emission never reorders the legend):
//
//  1. on-chain semantic dual caliber — 链上计入(共N段,同线程);
//  2. family fold ladder — 合计/成员最大/计数合计(共N段,同线程);
//  3. event fold — 单次最大(a~b,共N次);
//  4. running supply deficit — 折算,按全域最大核最高频[,下界] (R5 单基准);
//  5. generic published-eff≠window-projection — the bare 折算 discriminator
//     word (its legend entry teaches exactly this shown-when-different rule).
//
// "" when the published eff IS the window projection (pure wall clock — the
// negative pin: no caliber claim on an undiscounted seat).
func runtimeTraceProjElimCaliberNote(row runtimeTraceProjTreeRow, marks *runtimeTraceProjMarkSet, zh bool) string {
	node := row.Node
	if _, ok := runtimeTraceProjSemanticChainDualCaliber(node); ok {
		marks.mark(runtimeTraceProjMarkFamilyChainIntersection)
		return runtimeTraceProjSemanticChainIntersectionWord(node, zh)
	}
	if word, caliberMark, ok := runtimeTraceProjFamilyCaliberWord(node, zh); ok && runtimeTraceProjFamilyRow(node) &&
		!runtimeTraceProjFamilyValueIsGatedComposite(node) {
		// GATED-CAL 件1② twin (§29.104.16.1 M3, 2026-07-16): a family seat
		// whose published value is the gated product must not transcribe the
		// 合计(共N段) fold word here either — it falls to the composite arm
		// below (same typed predicate as the 行3 side; the two faces cannot
		// fork).
		runtimeTraceProjMarkFamilyCaliber(marks, caliberMark)
		return word
	}
	if runtimeTraceProjCauseEventFoldRow(row) {
		marks.mark(runtimeTraceProjMarkCaliberSingleMax)
		return runtimeTraceProjSingleMaxCaliberWord(node, zh)
	}
	if runtimeTraceProjCauseRunningDeficitArm(node) {
		word, wordMarks := runtimeTraceProjSupplyDiscountShortWord(node, zh)
		for _, m := range wordMarks {
			marks.mark(m)
		}
		if node.SupplyFoldUnknownMS > 0 {
			if zh {
				word += ",下界"
			} else {
				word += ", lower bound"
			}
			marks.mark(runtimeTraceProjMarkCaliberLowerBound)
		}
		return word
	}
	// GATED-CAL 件1⑤ 注记臂精确门 (§29.104.16.1 M3-d, 2026-07-16): a gated
	// composite's published value is runnable(全额)+running(折算) — the
	// generic eff≠projection arm below wore the bare 折算 word over it (a
	// single-caliber claim over a value whose runnable component counts IN
	// FULL), and the eff==projection shape wore no note at all (the ◎ 裸
	// runnable witness). Precise typed gate, one word source with the other
	// three faces; both pure shapes keep their arms byte-identically (pure
	// full → no note; pure discounted → arm 4 / the generic arm).
	if runtimeTraceProjGatedCompositeSeat(node) {
		marks.mark(runtimeTraceProjMarkGatedCompositeCaliber)
		// OMGCLEAN-1 件8 (§29.175.1 席行套话剥离): the ◎ seat row wears the
		// bare 构成 short mark — the ",见明细" pointer tail is stripped 套话
		// (the tree/detail faces keep the full 构成,见明细 word; the shared
		// legend entry teaches both forms, same mark).
		if zh {
			return "构成"
		}
		return "composition"
	}
	if node.EffectiveImpactMS > 0 && node.ImpactMS > 0 &&
		!runtimeTraceProjRound3Equal(node.EffectiveImpactMS, node.ImpactMS) {
		marks.mark(runtimeTraceProjMarkStanzaDiscount)
		if zh {
			return "折算"
		}
		return "discounted"
	}
	return ""
}

// runtimeTraceProjElimClassWord resolves the row's type identity word for the
// overview line — the semantic family class word (with its merge-count chip)
// on semantic rows, else the same cause/type display word the home row
// renders. Never a new coinage.
//
// OMGCLEAN-1 件11 终版 (§29.175.17, 2026-07-20): diagnosis=true (the ⛓/◇
// seat-row face) consumes the 判词文法 verdict mapping over the SAME typed
// token the word derived from — 一族一词根+·限定后缀, bare kernel state words
// retire from the board face (树状态面/state_churn/明细 keep the raw words;
// the ⌗ caliber footnote and the ▒ background rows pass diagnosis=false and
// keep every existing word byte-identically). The mapping single point is
// runtimeTraceProjElimVerdictTokenWord (typelabels.go); 优先级反转·* /
// 语义类 words 维持 through their earlier arms below.
//
// INV-SUPPLY 件① (§29.61.11, 2026-07-14): a supply-gap-dominant inversion
// seat transcribes its 行2 compound type word 优先级反转候选·供给缺口主导 in
// this slot — SAME composer, byte-identical (◎ 转录同词, 零新词源); the
// pre-INV-SUPPLY state word (running/runnable) that rode here said less than
// the seat's own 行2 and is superseded on exactly these seats.
func runtimeTraceProjElimClassWord(row runtimeTraceProjTreeRow, zh, diagnosis bool, marks *runtimeTraceProjMarkSet) string {
	node := row.Node
	if word, ok := runtimeTraceProjInversionSupplyGapCompoundWord(node, zh); ok {
		if node.MergedCount > 1 {
			word += runtimeTraceProjMergeCountChip(node.MergedCount, zh)
		}
		return word
	}
	// GATED-CAL 件1⑤ 类词臂推广 (§29.104.16.1 M3-d; INV-SUPPLY §29.61.11 总览
	// 侧 generalized, 2026-07-16): EVERY gated-composite inversion seat
	// transcribes its 行2 category word — the sub-threshold composite used to
	// fall through to the bare state word (the ◎ 裸 runnable witness:
	// 「3.429ms … · runnable」 over a runnable(全额)+running(折算) composite),
	// while only the DOMINANT form got the compound word above. Same composer
	// as the tree 行2 (runtimeTraceProjCauseCategoryWord — 转录同词, zero new
	// word source); single-component gated seats keep their existing words
	// byte-identically (precise typed gate).
	//
	// A5 反转词位 ◎ 臂 (sweep M8-d §29.104.16.1; the same INV-SUPPLY 转录同词
	// discipline, 2026-07-17). EVOLUTION RECORD: the gate widens from the
	// composite seats to EVERY priority-inversion family seat — a NON-composite
	// flag seat (pure runnable-full account) mirrored the 行1 composition word
	// through the RowCauseWordToken path below, so the ◎ strong seat showed the
	// weak word (cust_span_vs_prio: 「8.608ms … · runnable [E8]」 while the
	// seat's own 行2 says 优先级反转候选). Every family seat now transcribes
	// its 行2 word: composite → 构成词族 unchanged, flag → 优先级反转候选,
	// runnable-overlap token → 优先级反转·可运行等待 (per-token composer);
	// non-family seats keep every existing path byte-identically.
	if runtimeTraceProjInversionFamilyNode(node) {
		if word, _ := runtimeTraceProjCauseCategoryWord(node, row.Kind, zh); word != "" {
			if node.MergedCount > 1 {
				word += runtimeTraceProjMergeCountChip(node.MergedCount, zh)
			}
			return word
		}
	}
	if strings.TrimSpace(node.SemanticClass) != "" {
		word := runtimeTraceProjFamilySemanticClassWord(node, zh)
		if word == "" {
			word = strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseName(node.SemanticClass, zh))
		}
		if node.FamilyMemberCount > 1 {
			word += runtimeTraceProjMergeCountChip(node.FamilyMemberCount, zh)
		}
		return word
	}
	if word, token := runtimeTraceProjRowCauseWordToken(row, zh); word != "" {
		// 件11: the verdict mapping consumes the composer's OWN typed dedupe
		// token (the identity the word derived from — a span/blocking word
		// never carries a mapped state/latency token, so those identities
		// pass through untouched).
		if diagnosis {
			if verdict, ok := runtimeTraceProjElimVerdictTokenWord(node, token, zh); ok {
				word = verdict
				if marks != nil {
					marks.mark(runtimeTraceProjMarkElimVerdictGrammar)
				}
			}
		}
		if node.MergedCount > 1 {
			word += runtimeTraceProjMergeCountChip(node.MergedCount, zh)
		}
		return word
	}
	word := strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
	if diagnosis {
		if verdict, ok := runtimeTraceProjElimVerdictTokenWord(node,
			runtimeTraceCausalProjectionCauseDisplayToken(node), zh); ok {
			word = verdict
			if marks != nil {
				marks.mark(runtimeTraceProjMarkElimVerdictGrammar)
			}
		}
	}
	return word
}

// runtimeTraceProjElimQualifier is the row's identity qualifier slot:
//
//   - ⛓ chain-channel seat rows → the §29.187① credential-tier family
//     (强→弱: 唤醒锚定/目标自身/交集证明/成员继承, tracefence table ③d) —
//     every ⛓ seat row wears exactly one, refined by ·限定 suffixes where the
//     pre-§29.187 chip carried one (·确定性优化);
//   - ◇ semantic rows → 确定性优化·候选 (§29.61 b identity word family: the
//     conditional-upper-bound word stays on the adjacent side; outside the
//     credential family).
//
// EVOLUTION RECORD (§29.187① 四字族定案, 2026-07-21): the pre-ruling chips
// rename into the family — 边锚定→唤醒锚定, 自身(·确定性优化)→目标自身(·确定
// 性优化), 身份继承→成员继承, 包络凭证→交集证明 — and the previously BARE
// per-segment/interval-credential ⛓ rows now wear ·交集证明 (每 ⛓ 席行恰佩
// 其一): the on-chain admission itself is the interval adjudication's
// conservative keep, so the word claims nothing the lane has not already
// proven; the envelope-vs-per-segment granularity distinction lives on the
// tree-face full words (交集证明(包络级) vs the bare per-segment rows).
// Standing exception (XLANE-1 件3 §29.104.2 定谳⑤ survives the family): a
// foreign-subject fused self row wears NO family word — 目标自身 would lie
// about this board's target and no other family word describes its own-board
// self basis (edge shape flagged to the ruling pool, 2026-07-21).
func runtimeTraceProjElimQualifier(row runtimeTraceProjTreeRow, channel string, zh bool, marks *runtimeTraceProjMarkSet) string {
	family := func(word string) string {
		if marks != nil {
			marks.mark(runtimeTraceProjMarkChainCredentialTierFamily)
		}
		return word
	}
	// XLANE-1 件3 (§29.104.2 定谳⑤, 2026-07-15): both 目标自身· words are
	// target-exclusive — a foreign-subject row (another step's legitimate
	// self seat fused into this tree) never wears them on the ◎ face either
	// (same canonical subject==tree-target gate as the Row2 site).
	if strings.TrimSpace(row.Node.OnChainBasis) == "self_deterministic_span" && !row.SelfQualifierForeignSubject {
		if zh {
			return family(tracefence.CredentialTierTargetSelfZH + "·确定性优化")
		}
		return family(tracefence.CredentialTierTargetSelfEN + "·deterministic-optimization")
	}
	// SELF-ALL (§29.61.2, 2026-07-13): the wall-clock self basis wears its own
	// qualifier — same single-field fork, no 候选 word (the seat is a proven
	// wall-clock amount, not a conditional upper bound).
	// RNB-5B 默认小件c (§29.95 UX-4): family-arm self seats wear it too (the
	// model-build stamp, same word both faces).
	// OMGCLEAN-1 件8 (§29.175.1 席行套话剥离, 2026-07-20). EVOLUTION RECORD:
	// the ◎ chip carries the bare family word (the wall-clock seat is the
	// board's DEFAULT caliber — 默认不标,仅折算标); the tree 行2 face keeps
	// its full qualifier word untouched.
	if (strings.TrimSpace(row.Node.OnChainBasis) == "self_wall_clock_interval" || row.SelfWallClockQualifier) &&
		!row.SelfQualifierForeignSubject {
		if zh {
			return family(tracefence.CredentialTierTargetSelfZH)
		}
		return family(tracefence.CredentialTierTargetSelfEN)
	}
	// RNB-5B 默认小件e (§29.97 冷读观察③, 2026-07-15): the R3 edge-anchored
	// seat's qualification chip — same single-field fork family as the two
	// self chips above. The full credential sentence (唤醒锚定(宿主→目标)…)
	// lives on the tree 行2; the board carries the short membership word so
	// the reader can tell WHY a non-target semantic seat sits on the ⛓ block.
	// ONCHAIN-3c (2026-07-19): the state-seat sibling basis wears the same
	// chip word (same credential, same membership reason; the value-form
	// difference lives on the 行2 sentence, not the chip).
	switch strings.TrimSpace(row.Node.OnChainBasis) {
	case "host_wakeup_edge_pre_span", "host_wakeup_edge_pre_state":
		if zh {
			return family(tracefence.CredentialTierWakeupAnchoredZH)
		}
		return family(tracefence.CredentialTierWakeupAnchoredEN)
	}
	foreignSelf := row.SelfQualifierForeignSubject &&
		(strings.TrimSpace(row.Node.OnChainBasis) == "self_deterministic_span" ||
			strings.TrimSpace(row.Node.OnChainBasis) == "self_wall_clock_interval" ||
			row.SelfWallClockQualifier)
	// CHAINGUARD-1 件3 (§29.204.1 spec §3③ chip 引擎同源, 2026-07-22): when
	// the engine census verdict rides the row, the chips MAP THE ENUM instead
	// of re-deriving the typed bits (CHAINGUARD-F4 病根: the display
	// hand-copy could drift from the engine admission — the isplogcat 零 chip
	// 穿透). Five-value contract (dual-review F-2 路径显式化, 2026-07-22 —
	// this ONE switch now speaks every census verdict):
	//   none             → bare. The typed violation record: the seat carried
	//                      ZERO credential stamps, so it never wears a
	//                      fabricated credential claim (it is also barred
	//                      from board/badge/crown by the census second seat
	//                      gate).
	//   member_inherited → 成员继承 family word.
	//   interval_proven  → 交集证明 family word.
	//   target_self      → 目标自身 family word. Rows carrying the actual
	//                      self basis/qualifier are worded by the basis arms
	//                      ABOVE first (·确定性优化 suffix and the XLANE-1
	//                      foreignSelf typed display exception keep reading
	//                      OnChainBasis — the whitelisted display-side words,
	//                      spec §3③ verbatim latitude; word faces identical);
	//                      the explicit arm closes the basis-less residue
	//                      (e.g. the R8 SubjectIsAnalysisTarget mint without
	//                      a display self-qualifier) with the SAME family
	//                      word, and the XLANE-1 定谳⑤ foreignSelf exclusion
	//                      still holds (目标自身 is target-exclusive).
	//   wakeup_anchored  → 唤醒锚定 family word (basis-carrying rows hit the
	//                      host-edge basis arm above first; same word).
	// Absent census ("" — pre-census artifacts, chainless boards): the legacy
	// bit re-derivation below stays byte-identical (渐进兼容).
	switch strings.TrimSpace(row.Node.ChainCredentialCensus) {
	case "none":
		return ""
	case "target_self":
		if channel == runtimeTraceProjOrdinalChannelChain &&
			strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" &&
			!row.Node.ChainCredentialLaneDemoted && !foreignSelf {
			if zh {
				return family(tracefence.CredentialTierTargetSelfZH)
			}
			return family(tracefence.CredentialTierTargetSelfEN)
		}
	case "wakeup_anchored":
		if channel == runtimeTraceProjOrdinalChannelChain &&
			strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" &&
			!row.Node.ChainCredentialLaneDemoted {
			if zh {
				return family(tracefence.CredentialTierWakeupAnchoredZH)
			}
			return family(tracefence.CredentialTierWakeupAnchoredEN)
		}
	case "member_inherited":
		if channel == runtimeTraceProjOrdinalChannelChain &&
			strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" && !row.Node.ChainCredentialLaneDemoted {
			if zh {
				return family(tracefence.CredentialTierMemberInheritedZH)
			}
			return family(tracefence.CredentialTierMemberInheritedEN)
		}
	case "interval_proven":
		if channel == runtimeTraceProjOrdinalChannelChain &&
			strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" &&
			!row.Node.ChainCredentialLaneDemoted && !foreignSelf {
			if zh {
				return family(tracefence.CredentialTierIntervalProvenZH)
			}
			return family(tracefence.CredentialTierIntervalProvenEN)
		}
	}
	// RULE3-1 件9 (§29.183 G2, 2026-07-21) + §29.187① rename: the weak-tier
	// chips on the ◎ ⛓ seat rows — the tree face already wears the honest
	// full words 成员继承(链窗级,无区间凭证) / 交集证明(包络级); the ◎ chip
	// speaks the family root. Same typed gates as the tree-face word emitters
	// (零新引擎信号); additive disclosure only — admission/values/ordinals
	// untouched.
	if row.Node.ChainIdentityInheritance && !row.Node.ChainCredentialLaneDemoted &&
		!row.Node.ChainCredentialEnvelopeLevel && len(row.Node.ChainCredentialSegments) == 0 &&
		strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" {
		if zh {
			return family(tracefence.CredentialTierMemberInheritedZH)
		}
		return family(tracefence.CredentialTierMemberInheritedEN)
	}
	// §29.187① completeness arm (每 ⛓ 席行恰佩其一): every remaining ⛓
	// seat row on the typed on-chain verdict wears ·交集证明 — the
	// envelope-level keep (the pre-§29.187 包络凭证 chip) and the per-segment
	// / interval-credential keeps (previously bare) share the family word;
	// granularity stays on the tree-face full words. Gated on the EXPLICIT
	// typed on_chain relevance — chainless boards (empty relevance rides the
	// chain channel fail-open) never mint a credential claim. The narrow
	// foreignSelf exclusion keeps the XLANE-1 定谳⑤ suppression exact: ONLY a
	// foreign-subject row whose admission basis is a SELF basis stays bare
	// (its credential is its own board's self admission — 交集证明 would
	// misstate the mechanism, 目标自身 is ruled off; edge shape flagged to
	// the ruling pool); ordinary foreign-subject dependency seats (every
	// non-target worker) wear the family word their typed gates earn.
	if channel == runtimeTraceProjOrdinalChannelChain &&
		strings.TrimSpace(row.Node.ChainRelevance) == "on_chain" &&
		!row.Node.ChainCredentialLaneDemoted && !foreignSelf {
		if zh {
			return family(tracefence.CredentialTierIntervalProvenZH)
		}
		return family(tracefence.CredentialTierIntervalProvenEN)
	}
	if channel == runtimeTraceProjOrdinalChannelAdjacent && strings.TrimSpace(row.Node.SemanticClass) != "" {
		if zh {
			return "确定性优化·候选"
		}
		return "deterministic-optimization·candidate"
	}
	return ""
}

// runtimeTraceProjElimSubject resolves the row's subject display name through
// the shared display helper (comm-truncation placeholders never leak); fold
// rows without a subject fall back to the row-name composer's own noun.
func runtimeTraceProjElimSubject(row runtimeTraceProjTreeRow, zh bool) string {
	if subject := strings.TrimSpace(row.Node.Subject); subject != "" {
		return runtimeTraceCausalProjectionDisplayNodeName(subject, zh)
	}
	return strings.TrimSpace(runtimeTraceProjRowName(row, zh))
}

// runtimeTraceProjElimBarCells renders the relative magnitude bar (design
// §2.4: full scale = 本区 TOP1, a PURE visual magnitude — deliberately NOT the
// tree's 占窗% ruler, and NEVER a percentage: the population may contain
// supply-discounted eff components, and §29.27 bans discounted values from
// wall-clock percentages).
const runtimeTraceProjElimBarWidth = 12

func runtimeTraceProjElimBarCells(value, top float64) string {
	if value <= 0 || top <= 0 {
		return strings.Repeat("░", runtimeTraceProjElimBarWidth)
	}
	filled := int(value/top*runtimeTraceProjElimBarWidth + 0.5)
	if filled < 1 {
		filled = 1
	}
	if filled > runtimeTraceProjElimBarWidth {
		filled = runtimeTraceProjElimBarWidth
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", runtimeTraceProjElimBarWidth-filled)
}

// runtimeTraceProjElimRowLine renders ONE overview member line (value · bar ·
// subject · class[·caliber] [E#]), flush on one value/bar grid.
//
// 件⑤ (user ruling 2026-07-14, witness 20260714-164033 ◎ 板). EVOLUTION
// RECORD: the ◇最大/◇max fallback LEAD MARKER and its 恒定记号场 lead field
// are RETIRED — the value itself, the relative bar and the scale promise
// already carry the magnitude signal twice, and the marker only broke the
// grid. The §2.5 fallback SEAT semantics are untouched.
//
// OMGCLEAN-1 件8 (§29.175.1 席行套话剥离, 2026-07-20). EVOLUTION RECORD: the
// per-row channel word (⛓ 链上 / ◇ 邻近) is STRIPPED — under the five-zone
// layout the row's zone position states its channel (▸ direction sections =
// on-chain, the ◇ zone head = adjacent; 区位自明), and the glyph+noun pair
// repeated on every row was the exact 套话 the ruling names. The channel
// word emitter itself stays the single source for the heads and legends.
// anchorHoisted=true (件7 typed unanimity) suppresses the per-row 板锚 chip —
// the section head carries it once; mixed sections keep per-row anchors.
func runtimeTraceProjElimRowLine(entry runtimeTraceProjElimEntry, top float64, marks *runtimeTraceProjMarkSet, zh, boardAnchors, anchorHoisted bool) string {
	row := entry.row
	channel := runtimeTraceProjRowOrdinalChannel(row)
	// 修补轮 件G (2026-07-16): on the multi-target-board form every member
	// line names its board anchor (the head's single-thread ruler claim is
	// retired there) — same verbatim label and legend home as the 件2 seat
	// chip half; a row without the typed target stays bare (absence never
	// wears a board claim). 件7: hoisted sections carry it on the head.
	anchor := ""
	if boardAnchors && !anchorHoisted {
		if target := strings.TrimSpace(row.Node.RankBoardTarget); target != "" {
			if zh {
				anchor = " ·板锚 " + target
			} else {
				anchor = " · board " + target
			}
			marks.mark(runtimeTraceProjMarkRankBoardAnchor)
		}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%9.3fms ", row.Node.EffectiveImpactMS))
	b.WriteString(runtimeTraceProjElimBarCells(row.Node.EffectiveImpactMS, top))
	// RNB-5B 件⑦: the micro anchored-cut-seat fold's board line — label +
	// account-sum caliber + 见明细 pointer; the zone position carries the
	// preserved credential semantics (件8: channel word stripped).
	if row.Node.MicroAnchorFold {
		marks.mark(runtimeTraceProjMarkMicroAnchorFold)
		// 修复轮 U1: the threshold formats from the ONE constant (单点).
		note := fmt.Sprintf("·合计(账目合计,均<%.1fms)见明细", runtimeTraceProjMicroAnchorFoldMs)
		if !zh {
			note = fmt.Sprintf(" · total (account sum, each <%.1fms); see the detail blocks", runtimeTraceProjMicroAnchorFoldMs)
		}
		line := " " + runtimeTraceProjMicroAnchorFoldName(row.Node, zh) + note + anchor
		if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
			line += " [" + tag + "]"
		}
		return b.String() + line
	}
	sep := " · "
	b.WriteString(" " + runtimeTraceProjElimSubject(row, zh))
	if class := runtimeTraceProjElimClassWord(row, zh, true, marks); class != "" {
		b.WriteString(sep + class)
		// INV-SUPPLY 件①: the compound word's legend follows its ◎ emission
		// site too (词条-图例双向; the tree 行2 site marks independently).
		if _, ok := runtimeTraceProjInversionSupplyGapCompoundWord(row.Node, zh); ok {
			marks.mark(runtimeTraceProjMarkSupplyGapDominant)
		}
	}
	if note := runtimeTraceProjElimCaliberNote(row, marks, zh); note != "" {
		b.WriteString(" ·" + note)
	}
	if qual := runtimeTraceProjElimQualifier(row, channel, zh, marks); qual != "" {
		b.WriteString(" ·" + qual)
	}
	// ELIM-V2 ◇ 行内方向词 (委托默认, 2026-07-18): the unsectioned adjacent
	// block still names each row's fix direction — the SAME single word table
	// the section heads speak (词面单点); an unresolved direction renders
	// nothing (fail-open, absence never guesses).
	if channel == runtimeTraceProjOrdinalChannelAdjacent {
		if word, ok := runtimeTraceProjFixDirectionWord(row.Node.FixDirection, zh); ok {
			if zh {
				b.WriteString(" ·方向=" + word)
			} else {
				b.WriteString(" · direction=" + word)
			}
			marks.mark(runtimeTraceProjMarkElimAdjacentDirectionWord)
		}
	}
	// CASE3-D4 伴生 (§29.84 件④, 2026-07-14): a merged member row whose value
	// spans multiple query windows says so beside its seat — the ◎ ruler
	// promises 窗内 wall clock, and a silent multi-window Σ would read as one
	// window's amount. Same one-word emitter as the seat chip's qualifier;
	// per-member windows stay on the detail 窗来源 lane.
	if word, ok := runtimeTraceProjMergedMemberWindowSpanWord(row.Node, zh); ok {
		b.WriteString(sep + word)
		marks.mark(runtimeTraceProjMarkMergedMemberWindowSpan)
	}
	b.WriteString(anchor)
	// ELIM-V2 ∩ chip (2026-07-18): one chip per RESOLVED cross-direction
	// overlap partner — transcribed from the SAME model-build clauses that
	// drive the tree row's full 互指句 (both-with-tree by construction; the
	// wire carrier absent → no chip, 宁漏勿假指).
	for _, clause := range row.CrossDirectionOverlapClauses {
		if ref := strings.TrimSpace(clause.Ref); ref != "" {
			b.WriteString(" ·∩[" + ref + "]")
			marks.mark(runtimeTraceProjMarkElimCrossDirectionChip)
		}
	}
	if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
		b.WriteString(" [" + tag + "]")
	}
	return b.String()
}

// runtimeTraceProjElimCompositionNoteLine renders the INV-SUPPLY 件③
// (§29.61.11, 2026-07-14) per-seat eliminable-composition leverage note for a
// compound-word seat: 「X ms 调度修复 + Y ms 频点/热策略」 (值前置 form,
// 双复核 件13) under the 构成拆解 aux label — the seat's
// OWN 行3 attribution split (runtimeTraceProjInversionComponents, the SAME
// balance-gated builder, so the bytes can never disagree with 行3 and an
// unbalanced split refuses to render — 拒渲绝不造数) transcribed by
// elimination lever: 调度修复 = the runnable(全额) component, 频点/热策略 =
// the running(折算) component. A constituent display of ONE seat's value —
// never a Σ row, never added across rows (零求和红线; the renderer emits no
// total and no "=" claim). ok=false on every non-compound seat: the note
// exists exactly where the compound word says the supply gap dominates.
func runtimeTraceProjElimCompositionNoteLine(row runtimeTraceProjTreeRow, marks *runtimeTraceProjMarkSet, zh bool) (string, bool) {
	if _, compound := runtimeTraceProjInversionSupplyGapCompoundWord(row.Node, zh); !compound {
		return "", false
	}
	components, _, ok := runtimeTraceProjInversionComponents(row.Node, row.FreqOnlyCauseHoisted, zh)
	if !ok {
		return "", false
	}
	var parts []string
	for _, c := range components {
		// §18 G3② (双维度审计, 2026-07-28): the per-component lever words
		// source from THE direction word table (tracefence.FixDirectionWord)
		// — the composition note used to speak a third vocabulary
		// (调度修复/频点/热策略) beside the seat's registry direction and the
		// six-direction section heads, with no on-page reconciliation.
		lever := ""
		switch c.Word {
		case "runnable":
			lever, _ = tracefence.FixDirectionWord("scheduling_supply", zh)
		case "running":
			lever, _ = tracefence.FixDirectionWord("frequency_thermal", zh)
		default:
			// A component outside the two-lever vocabulary has no leverage
			// label — the note refuses to guess (absence never invents).
			return "", false
		}
		// 双复核修复 件13 (冷读 CR11, §29.175.8 值在首段, 2026-07-21).
		// EVOLUTION RECORD: segments spoke 「调度修复 0.109ms」 — the value
		// now LEADS each segment (「0.109ms 调度修复」), the aux row's first
		// segment is a value, and the EN row fits the 100-cell budget.
		parts = append(parts, runtimeTraceProjFmtMS(c.InMS)+" "+lever)
	}
	if len(parts) == 0 {
		return "", false
	}
	// RNB-1 C-2① (§29.88.10 R7-2, 2026-07-14): a SINGLE-component note is the
	// row value re-printed under a heading (witness 20260714-230952 E31:
	// 「可消除构成: 调度修复 0.423ms」 = the seat value itself, zero
	// information) — the note renders only when the split actually splits
	// (≥2 levers). With C-2②'s constitutive precondition a compound seat
	// always carries the running lever, so this arm now guards the
	// running-only degenerate twin the same way.
	if len(parts) < 2 {
		return "", false
	}
	marks.mark(runtimeTraceProjMarkElimComposition)
	// 件⑥ (user ruling 2026-07-14, witness 20260714-164033 ◎ 板) EVOLUTION
	// RECORD → RNB-1 C-3 (§29.88.11 R7a, 2026-07-14) EVOLUTION RECORD: the
	// 件⑥ 12-column value-field indent form is RETIRED WITH ITS POSITION —
	// the note relocates into the dedicated 构成拆解 aux rows after the seat
	// rows (the E# replaces adjacency as the binding). 双复核修复 件13 (冷读
	// CR11, 2026-07-21) EVOLUTION RECORD: the 「可消除构成: 」 head word is
	// retired — the aux LABEL 构成拆解/composition already names the family
	// (席行套话剥离 twin), the note is the bare value-first lever join, and
	// the family's legend probe follows the label form.
	return strings.Join(parts, " + "), true
}

// runtimeTraceProjElimValueFieldWidth is the member line's value-field width
// in columns (`%9.3fms ` = 9 + len("ms") + 1 trailing space) — the bar's
// start column. EVOLUTION RECORD (RNB-1 C-3, §29.88.11 R7a): the composition
// note left the bar region (its 件⑥ indent role retired); the constant stays
// as the bar-column authority for the member-line geometry.
const runtimeTraceProjElimValueFieldWidth = 12

// runtimeTraceProjElimHead composes the overview head line (region name +
// ruler declaration + the R16② form promises + bar scale). The ruler subject
// resolves typed-only: the ⊚ target, else the flat-lane analysis anchor,
// else the generic noun (absence never invents a thread). withForm=false
// (the honest empty-board shape) drops the ordering/scale promise line — a
// board that admitted nothing has no member ordering to promise. chainPresent
// forks the block promise (表头禁撒谎 both directions): a chain-bearing board
// promises 「⛓ 链上块先·块内值降序」; a ◇-only board promises its single
// block honestly (「◇ 邻近块·块内值降序」) and never prints the ⛓ glyph it
// has no rows for (the empty-chain honest line below the members states the
// missing channel).
func runtimeTraceProjElimHead(model runtimeTraceProjTreeModel, zh, withForm, chainPresent, multiBoardRuler bool) []string {
	target := strings.TrimSpace(model.Target)
	if target == "" {
		target = strings.TrimSpace(model.FlatAnchorThread)
	}
	if target != "" {
		target = runtimeTraceCausalProjectionDisplayNodeName(target, zh)
	}
	// §29.61.12 ② (INV-SUPPLY 件④). EVOLUTION RECORD: the promise sentence
	// follows the block ordering (表头禁撒谎 — 「纯值降序」 would lie over a
	// blocked list): 「纯值降序」 → 「⛓ 链上块先·块内值降序」, composed from
	// the ONE channel-word emitter (零新词源 for the glyph+noun identity).
	// The scale promise stays 满格=本区TOP1: the full bar remains the
	// BOARD-WIDE largest value wherever its row sits (链上条短=诚实).
	// ELIM-V2 修补轮 件5 (2026-07-18): the EN face says "board TOP1" — the
	// former "section TOP1" now collides with the ▸ fix-direction sections
	// (◎ 全区语义: the ruler is the whole board, not one ▸ section); zh 本区
	// has no such collision (节 is the section word) and stays byte-stable.
	//
	// 修补轮 件G (2026-07-16, donghu fused witness: the head claimed
	// 尺=CompThread_0-2955 while 9163-board seats sat under it — 同尺宣称假):
	// when the member rows span target boards other than the head subject,
	// the ruler line speaks the multi-board form and each member line wears
	// its 板锚 anchor; the single-board head stays byte-identical.
	// ELIM-V2 (2026-07-18): the form promise follows the direction-section
	// layout (表头禁撒谎 — 「块内值降序」 would lie over a sectioned chain
	// block): a chain-bearing board promises 节=修复方向 with its section
	// order plus the standing anti-addition declaration 方向间收益不可相加
	// (恒发 with the form line, 三层防相加之一); a ◇-only board keeps its
	// single-block promise and carries the same declaration (its rows wear
	// the ·方向=X words). The promise composes from SEGMENTS packed greedily
	// under the width cap (NEW-10 wholeness: facts split at ` · ` seams,
	// never truncate).
	// 用户显示裁定 (2026-07-19, witness 20260719-161405 L146-147): the form
	// promises render as DELIBERATE MULTI-LINE rows — each line wears the
	// block glyph (同佩图标), zero indent, semantically grouped — instead of
	// one long line the wrap engine bisects into a bare-headed continuation
	// (「堆积在一行,不好看」). The glyph derives from the ONE channel-word
	// emitter (first rune — zero second glyph source); every promise line is
	// composed to sit inside the row width cap naturally (structure-pinned).
	var title, ruler string
	var promises []string
	if zh {
		if target == "" {
			target = "关注线程"
		}
		title = tracefence.ElimGlyph + " 窗内可消除量总览"
		ruler = "尺=" + target + " 窗内墙钟ms"
		if multiBoardRuler {
			ruler = "尺=各板目标线程 窗内墙钟ms·跨板不可相加"
		}
		if chainPresent {
			chainWord := runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelChain, true)
			// RUN2FIX-A 件1 (§29.174 处置②, runnable_2:139/:148/:150 witness):
			// the section sort has ALWAYS parked the tail section LAST
			// regardless of its value (runtimeTraceProjElimSectionsFor —
			// design intent, fail-open material never outranks resolved
			// directions), and the promise states the tail-last rule verbatim
			// (表头禁撒谎; 词面单点, zh/en + both legend faces).
			// OMGCLEAN-1 件1+件3 (§29.175 裁定②/.1, 2026-07-20). EVOLUTION
			// RECORD — 涉既裁位移① (§29.147 件F 原裁 verbatim: 「这一行太长了,
			// 换行看起来也不好看,可以考虑多行(佩戴同样的图标不缩进 是否更好看
			// 一些?),而不是堆积在一行,不好看」— 件F deliberately chose the
			// multi-line form; §29.175.1 verbatim: 「窗内可消除量总览 一定要
			// 精简且关键」 now compresses it): the tail is named 「其他方向」
			// (件1 rename — the promise speaks the new word); the §29.147 件F
			// three-line promise form compresses to ONE line under the 头部
			// 两行 preview ruling —
			// the retired line ③ (零序数·零佩戴·定位走[E#]·满格=各区TOP1)
			// moved onto the ◎ legend entry, and the scale word itself moved
			// 本区→各区 in lockstep with the per-zone bar normalization
			// (§29.175.9 承诺词同改).
			promises = []string{
				chainWord + "块先 · 节=修复方向(其他方向恒末,余按节内最大可消降序)· 节内值降序 · 方向间收益不可相加",
			}
		} else {
			adjacentWord := runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelAdjacent, true)
			promises = []string{
				adjacentWord + "块·块内值降序 · 方向间收益不可相加",
			}
		}
	} else {
		if target == "" {
			target = "focused thread"
		}
		title = tracefence.ElimGlyph + " Eliminable-in-window overview"
		ruler = "ruler = " + target + " in-window wall-clock ms"
		if multiBoardRuler {
			ruler = "ruler = each board's target thread, in-window wall-clock ms · never add across boards"
		}
		if chainPresent {
			chainWord := runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelChain, false)
			glyph := string([]rune(chainWord)[0])
			// RUN2FIX-A 件1 EN face: line 1 speaks the tail section's FULL
			// name (the bidirectional legend probe token) — the one-line
			// composite overflows the 100-cell promise cap, so the EN face
			// keeps a structure-pinned TWO-line form where zh fits one.
			// OMGCLEAN-1 件1+件3: tail word co-move ("other directions"); the
			// retired line ③ lives on the ◎ legend entry (件3, zh 同批).
			promises = []string{
				chainWord + " block first · sections = fix direction (other directions tail last)",
				glyph + " rest by max-eliminable desc · value desc within section · gains never add across directions",
			}
		} else {
			adjacentWord := runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelAdjacent, false)
			promises = []string{
				adjacentWord + " block · value desc within block · gains never add across directions",
			}
		}
	}
	sep := " · "
	if !withForm {
		return []string{title + sep + ruler}
	}
	return append([]string{title + sep + ruler}, promises...)
}

// --- OMGCLEAN-1 — 辅助 — zone (§29.175.6 区域序终裁 zone ⑤; 件9 两列文法
// §29.175.8/.11/.13/.14, 2026-07-20) --------------------------------------
//
// The former ◎ footnote tail converges into ONE auxiliary zone with a
// two-column grammar: a fixed-width LABEL column (词面闭集: ∩ 重叠对 / 守恒 /
// 未入榜 / 未入榜最大 / 口径旁栏 + the conditional family words; only the
// functional cross-reference glyph ∩ survives on a label — the decorative ⌗
// is stripped, §29.175.13 降噪) and a CONTENT column (value in the first
// segment · one short clause · pointer — full semantics live on the legend).
// Group order (§29.175.8): 对账组 first (∩ 重叠对 / 守恒 — they verify the
// zones above), 另账组 second (未入榜 / 未入榜最大 / 口径旁栏 / conditional
// disclosures — accounts outside the zones above). Rows are same-level `· `
// list rows; an over-budget account splits into sibling rows, never a wrapped
// continuation (§29.175.14 双行纪律 — the 未入榜/未入榜最大 pair is the
// canonical split). 排除≠消失: every former footnote family keeps its counted
// row here; nothing is deleted.

// runtimeTraceProjElimAuxRow is one auxiliary-zone list row.
type runtimeTraceProjElimAuxRow struct {
	label   string
	content string
}

// runtimeTraceProjElimAuxZoneLines renders the — 辅助 — zone: the zone head
// plus every row on the shared label-column width (per-fence fixed width =
// the widest rendered label; display-cell padding via runewidth so CJK and
// ASCII labels align). nil rows → nil (the zone is simply absent).
func runtimeTraceProjElimAuxZoneLines(rows []runtimeTraceProjElimAuxRow, marks *runtimeTraceProjMarkSet, zh bool) []string {
	if len(rows) == 0 {
		return nil
	}
	marks.mark(runtimeTraceProjMarkElimAuxZone)
	width := 0
	for _, row := range rows {
		if w := runewidth.StringWidth(row.label); w > width {
			width = w
		}
	}
	// 双复核修复 件1 (冷读 CR1/对抗 CR-3, §29.175.8 定稿逐字, 2026-07-21): the
	// zone head itself ANNOUNCES the two groups — 「对账与另账」 — plus the
	// zone-wide no-ordinal fact; the bare 「— 辅助 —」 head left the 对账/另账
	// grouping invisible on the face (the announcement lived only on the
	// legend).
	head := "— 辅助 · 对账与另账(不占序数) —"
	if !zh {
		head = "— auxiliary · reconciliation & side accounts (no ordinal) —"
	}
	lines := []string{head}
	for _, row := range rows {
		pad := strings.Repeat(" ", width-runewidth.StringWidth(row.label))
		lines = runtimeTraceProjElimAppendNotes(lines, "· "+row.label+pad+"  "+row.content)
	}
	return lines
}

// runtimeTraceProjElimRepresentedFootnote (XLANE-1 裁定①, §29.104.17 ①,
// 用户批复 2026-07-16) builds the represented-satellite exclusion's dedicated
// disclosure row (排除≠消失): one counted row naming every row the 种群臂
// kept out, with [E#] pointers into the detail blocks where the full
// 锚定份由链席代表 sentence lives. The §29.112 closure identity extends with
// this lane (rendered + cut counts + represented == the pre-exclusion
// population; structure pin). The existing tree-face legend entry teaches the
// word family; its mark records here too so the wordface and its legend can
// never decouple (词条-图例双向). ok=false when no row is excluded.
// OMGCLEAN-1 件9: the line re-shapes into the two-column aux grammar (label
// 已由链上席代表, content count+pointer); the family word and count survive.
func runtimeTraceProjElimRepresentedFootnote(model runtimeTraceProjTreeModel, zh bool) (runtimeTraceProjElimAuxRow, bool) {
	count := 0
	var tags []string
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if !runtimeTraceProjElimRepresentedExcluded(rows[i]) {
				continue
			}
			count++
			if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" {
				tags = append(tags, "["+tag+"]")
			}
		}
	}
	if count == 0 {
		return runtimeTraceProjElimAuxRow{}, false
	}
	model.Marks.mark(runtimeTraceProjMarkChainAnchorRepresented)
	tagList := ""
	if len(tags) > 0 {
		tagList = " " + strings.Join(tags, runtimeTraceProjElimJoinSep(zh))
	}
	if zh {
		return runtimeTraceProjElimAuxRow{label: "已由链上席代表",
			content: fmt.Sprintf("%d 行(整席不入链上榜),见明细%s", count, tagList)}, true
	}
	return runtimeTraceProjElimAuxRow{label: "represented",
		content: fmt.Sprintf("%d row(s) (whole seat off the on-chain board) — see the detail blocks%s", count, tagList)}, true
}

// runtimeTraceProjElimMemberSubsetFootnote (XLANE-2 件1, §29.104.1/.2 定谳④,
// 2026-07-17) renders the member-subset exclusion's dedicated disclosure
// footnote (排除≠消失, the 裁定① represented-footnote precedent): one counted
// line naming every row the subset arm keeps off the ◎ face — population and
// semantic census alike — with [E#] pointers into the detail blocks where the
// full 为[E#]成员子集 pointer sentence lives. The closure identity extends
// with this lane. ok=false when no row is excluded (zero rows → zero bytes).
func runtimeTraceProjElimMemberSubsetFootnote(model runtimeTraceProjTreeModel, zh bool) (runtimeTraceProjElimAuxRow, bool) {
	count := 0
	var tags []string
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if !runtimeTraceProjElimMemberSubsetExcluded(rows[i]) {
				continue
			}
			count++
			if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" {
				tags = append(tags, "["+tag+"]")
			}
		}
	}
	if count == 0 {
		return runtimeTraceProjElimAuxRow{}, false
	}
	model.Marks.mark(runtimeTraceProjMarkSemanticMemberSubset)
	tagList := ""
	if len(tags) > 0 {
		tagList = " " + strings.Join(tags, runtimeTraceProjElimJoinSep(zh))
	}
	if zh {
		return runtimeTraceProjElimAuxRow{label: "语义席成员子集",
			content: fmt.Sprintf("%d 行(整席不入链上榜),见明细%s", count, tagList)}, true
	}
	return runtimeTraceProjElimAuxRow{label: "member subset",
		content: fmt.Sprintf("%d row(s) (whole seat off the on-chain board) — see the detail blocks%s", count, tagList)}, true
}

// runtimeTraceProjElimGatedConstituentFootnote (LEVELMERGE-1 件2, 2026-07-18)
// renders the gated-share constituent exclusion's dedicated disclosure
// footnote (排除≠消失, the 裁定① represented-footnote precedent): one counted
// line naming every constituent row the 种群臂 keeps off the ◎ face, with
// [E#] pointers into the detail blocks where the full 分账构成份 sentence
// lives. The closure identity extends with this lane. ok=false when no row is
// excluded (zero rows → zero bytes).
func runtimeTraceProjElimGatedConstituentFootnote(model runtimeTraceProjTreeModel, zh bool) (runtimeTraceProjElimAuxRow, bool) {
	count := 0
	var tags []string
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows, model.Adjacent} {
		for i := range rows {
			if !runtimeTraceProjElimGatedConstituentExcluded(rows[i]) {
				continue
			}
			count++
			if tag := strings.TrimSpace(rows[i].EvidenceTag); tag != "" {
				tags = append(tags, "["+tag+"]")
			}
		}
	}
	if count == 0 {
		return runtimeTraceProjElimAuxRow{}, false
	}
	model.Marks.mark(runtimeTraceProjMarkGatedShareSplit)
	tagList := ""
	if len(tags) > 0 {
		tagList = " " + strings.Join(tags, runtimeTraceProjElimJoinSep(zh))
	}
	if zh {
		return runtimeTraceProjElimAuxRow{label: "分账构成份",
			content: fmt.Sprintf("%d 行(已计入反转席),见明细%s", count, tagList)}, true
	}
	return runtimeTraceProjElimAuxRow{label: "split share",
		content: fmt.Sprintf("%d row(s) (already counted at the inversion seat) — see the detail blocks%s", count, tagList)}, true
}

// runtimeTraceProjElimOverviewFence renders the ◎ overview fence (design §2,
// RANK-U Stage 2 commit D). "" when the run never observed the root_cause_
// rank family (typed projection flag — a board that never existed has no
// overview face; a board that ran but admitted nothing renders the honest
// empty-board line instead: 排除≠消失, absence of板≠空板).
// runtimeTraceProjElimChainRoster is the ◎ chain-side selection result shared
// by the fence assembler and the A2 件1 next-step direction lane (单一值源:
// one selection authority — a second hand copy of the TOP-slice + semantic-
// fallback walk would let the two faces fork on a seat).
type runtimeTraceProjElimChainRoster struct {
	board               []runtimeTraceProjElimEntry
	renderedChain       []runtimeTraceProjElimEntry
	semanticFallbackIdx int
	chainPresent        bool
}

// runtimeTraceProjElimChainRosterFor computes the fence's chain-side seat
// selection (verbatim extraction of the assembler's walk, A2 件1 refactor —
// selection semantics byte-identical).
func runtimeTraceProjElimChainRosterFor(model runtimeTraceProjTreeModel) runtimeTraceProjElimChainRoster {
	roster := runtimeTraceProjElimChainRoster{semanticFallbackIdx: -1}
	roster.board = runtimeTraceProjElimBoard(model)
	board := roster.board
	top := board
	if len(top) > runtimeTraceProjElimTopN {
		top = top[:runtimeTraceProjElimTopN]
	}
	var semanticFallback *runtimeTraceProjElimEntry
	semanticInTop := false
	for i := range top {
		if top[i].channelRank == 0 && strings.TrimSpace(top[i].row.Node.SemanticClass) != "" {
			semanticInTop = true
			break
		}
	}
	if !semanticInTop {
		for i := runtimeTraceProjElimTopN; i < len(board); i++ {
			if board[i].channelRank != 0 || strings.TrimSpace(board[i].row.Node.SemanticClass) == "" {
				continue
			}
			semanticFallback = &board[i]
			roster.semanticFallbackIdx = i
			break
		}
	}
	for _, entry := range top {
		if entry.channelRank == 0 {
			roster.renderedChain = append(roster.renderedChain, entry)
		}
	}
	if semanticFallback != nil {
		roster.renderedChain = append(roster.renderedChain, *semanticFallback)
	}
	for i := range board {
		if board[i].channelRank == 0 {
			roster.chainPresent = true
			break
		}
	}
	return roster
}

// runtimeTraceProjElimBoardScope is the single board-identity premise for the
// visible section subtotal and the pre-final model handoff. It reads the full
// post-aggregation board, so a TOP-N slice cannot hide a cross-board member.
func runtimeTraceProjElimBoardScope(model runtimeTraceProjTreeModel, board []runtimeTraceProjElimEntry) (bool, map[string]bool) {
	rulerSubject := strings.TrimSpace(model.Target)
	if rulerSubject == "" {
		rulerSubject = strings.TrimSpace(model.FlatAnchorThread)
	}
	rulerSubjectKey := runtimeTraceCausalProjectionCanonicalNode(rulerSubject)
	multiBoardRuler := false
	namedTargets := map[string]bool{}
	for i := range board {
		label := strings.TrimSpace(board[i].row.Node.RankBoardTarget)
		if label == "" {
			continue
		}
		namedTargets[runtimeTraceCausalProjectionCanonicalNode(label)] = true
		if rulerSubjectKey != "" && runtimeTraceCausalProjectionCanonicalNode(label) != rulerSubjectKey {
			multiBoardRuler = true
		}
	}
	if rulerSubjectKey == "" && len(namedTargets) >= 2 {
		multiBoardRuler = true
	}
	return multiBoardRuler, namedTargets
}

// TraceAnswerDecisionDirectionSections exports the exact deterministic
// eliminable-overview population as typed prompt authority. It deliberately
// builds the same tree model, runs the same admission/convergence/TOP-N plus
// semantic-fallback roster, and consumes the same arithmetic predicate as the
// rendered ▸ section heads. The model therefore cannot be told "leader only"
// when the answer appendix publishes an exact disjoint subtotal (or vice
// versa). This function does not author visible answer prose.
func TraceAnswerDecisionDirectionSections(projection types.TraceCausalProjection) []types.TraceAnswerDirectionSection {
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	roster := runtimeTraceProjElimChainRosterFor(model)
	multiBoardRuler, namedTargets := runtimeTraceProjElimBoardScope(model, roster.board)
	sections := runtimeTraceProjElimSectionsFor(roster.renderedChain)
	out := make([]types.TraceAnswerDirectionSection, 0, len(sections))
	for _, section := range sections {
		if strings.TrimSpace(section.direction) == "" {
			continue
		}
		published := types.TraceAnswerDirectionSection{Direction: section.direction}
		for _, entry := range section.entries {
			node := entry.row.Node
			published.Members = append(published.Members, node)
			if published.Leader.Rank == 0 || node.EffectiveImpactMS > published.Leader.EffectiveImpactMS ||
				(node.EffectiveImpactMS == published.Leader.EffectiveImpactMS && node.Rank < published.Leader.Rank) {
				published.Leader = node
			}
			if ref := types.TraceAnswerRelationMemberRef(node); ref != "" {
				published.MemberRefs = append(published.MemberRefs, ref)
			}
		}
		published.Arithmetic, published.SubtotalMS = types.TraceAnswerDirectionSectionArithmetic(
			published.Direction, published.Members, multiBoardRuler, len(namedTargets) > 0,
		)
		out = append(out, published)
	}
	return out
}

func runtimeTraceProjElimOverviewFence(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	if !projection.RootCauseFamilyObserved {
		return ""
	}
	roster := runtimeTraceProjElimChainRosterFor(model)
	board := roster.board
	// RNB-2 件4 (ELIM-SEM 方案A, §29.88 R1 用户裁定, 2026-07-15) semantic
	// fallback + OMGCLEAN-1 zone populations (§29.175.6/.9): the chain-side
	// TOP-slice walk lives in runtimeTraceProjElimChainRosterFor (A2 件1
	// extraction — the next-step direction lane reads the SAME selection;
	// semantics and bytes unchanged, full rationale on the helper).
	renderedChain := roster.renderedChain
	semanticFallbackIdx := roster.semanticFallbackIdx
	var renderedAdjacent []runtimeTraceProjElimEntry
	adjacentTotal := 0
	for i := range board {
		if board[i].channelRank != 1 {
			continue
		}
		adjacentTotal++
		if len(renderedAdjacent) < runtimeTraceProjElimAdjacentTopN {
			renderedAdjacent = append(renderedAdjacent, board[i])
		}
	}
	// ELIM-GAP 件B (§29.104.15): per-channel cut-count disclosure — 排除≠消失
	// covers value-cut seats; the counts feed the auxiliary 未入榜 row (件4:
	// per-channel counts preserved inside one row). ◇ 计数口径 (§29.175.9):
	// everything outside the TOP3 display counts (展示外全额入明细计数).
	chainCut := 0
	for i := runtimeTraceProjElimTopN; i < len(board); i++ {
		if i == semanticFallbackIdx {
			continue
		}
		if board[i].channelRank == 0 {
			chainCut++
		}
	}
	adjacentCut := adjacentTotal - len(renderedAdjacent)
	chainPresent := roster.chainPresent
	// 修补轮 件G: the multi-board ruler fires exactly when ≥1 member row's
	// typed board target canonically differs from the head's ruler subject
	// (the single-thread ruler claim is then FALSE), or — subject-less flat
	// heads — when the members span ≥2 distinct named boards. Typed inputs
	// only; identity-less rows never flip the head.
	multiBoardRuler, namedTargets := runtimeTraceProjElimBoardScope(model, board)
	model.Marks.mark(runtimeTraceProjMarkElimOverview)
	// RUN2FIX-A 件1: the chain-bearing form promise now names the
	// 方向未定/复合 tail rule, so the tail section's legend entry teaches on
	// every such board (词条-图例双向 sweep contract: fence word ⇔ legend
	// entry — the promise emits the word even when no tail section renders).
	if len(board) > 0 && chainPresent {
		model.Marks.mark(runtimeTraceProjMarkElimDirectionUnresolved)
	}
	var lines []string
	lines = append(lines, runtimeTraceProjElimHead(model, zh, len(board) > 0, chainPresent, multiBoardRuler)...)
	if len(board) == 0 {
		// 空板形 (design §2.5): the board ran and admitted nothing — one
		// honest line, no silent-disappearance path.
		if zh {
			lines = append(lines, tracefence.ElimGlyph+" 窗内可消除量:无同尺持值行(详见背景/义务通道)")
		} else {
			lines = append(lines, tracefence.ElimGlyph+" eliminable in window: no same-ruler valued rows (see the background / obligation channels)")
		}
		// 裁定① (§29.104.17 ①) + XLANE-2 件1 + LEVELMERGE-1 件2 + PARTSPLIT-1
		// (§29.150④): a board emptied entirely by the exclusion arms still
		// discloses the excluded rows and the unranked-max account — the empty
		// line alone would be the silent-disappearance path these disclosures
		// exist to close (排除≠消失 has no empty-board exception). OMGCLEAN-1
		// 件9: the disclosures ride the same — 辅助 — zone grammar here.
		var aux []runtimeTraceProjElimAuxRow
		aux = append(aux, runtimeTraceProjElimGatedCompositeEdgeShareMentionRows(model, zh)...)
		// SELFRUN-DISC (§29.192① (b)): the 「量不了」 absence row must survive
		// the empty-board form too — a target with NO frequency data is
		// exactly the board most likely to admit nothing, and silence here
		// would be the "no loss" face this disclosure exists to kill.
		aux = append(aux, runtimeTraceProjElimSelfFoldUnmeasuredRows(model, zh)...)
		if row, ok := runtimeTraceProjElimRepresentedFootnote(model, zh); ok {
			aux = append(aux, row)
		}
		if row, ok := runtimeTraceProjElimMemberSubsetFootnote(model, zh); ok {
			aux = append(aux, row)
		}
		if row, ok := runtimeTraceProjElimGatedConstituentFootnote(model, zh); ok {
			aux = append(aux, row)
		}
		if zone := runtimeTraceProjElimAuxZoneLines(aux, model.Marks, zh); len(zone) > 0 {
			lines = append(lines, "")
			lines = append(lines, zone...)
		}
		return runtimeTraceProjElimClose(lines, model.Marks)
	}
	// OMGCLEAN-1 bar rulers (§29.175.9 用户裁定: 满格=各区TOP1, 承诺词同改 —
	// the ◎ legend's scale sentence). EVOLUTION RECORD: the ruler was the
	// BOARD-WIDE maximum (§29.61.12 ②) — under the five-zone layout the
	// global ruler kept every ◇/small-value zone bar near-empty, so each
	// bar-wearing zone (⛓ / ◇ — the eliminable-amount dimension; ◈/▒ wear no
	// bar, §29.175.7) normalizes to its OWN zone's largest rendered value.
	chainTop, adjacentTop := 0.0, 0.0
	for _, entry := range renderedChain {
		if v := entry.row.Node.EffectiveImpactMS; v > chainTop {
			chainTop = v
		}
	}
	for _, entry := range renderedAdjacent {
		if v := entry.row.Node.EffectiveImpactMS; v > adjacentTop {
			adjacentTop = v
		}
	}
	// RNB-1 C-3 (§29.88.11 R7a, 2026-07-14): the bar region renders PURE seat
	// rows — zero interstitial sub-lines. Composition notes collect in board
	// seat order and render as 构成拆解 rows of the auxiliary zone.
	var decomp []runtimeTraceProjElimAuxRow
	appendMember := func(entry runtimeTraceProjElimEntry, topValue float64, anchorHoisted bool) {
		lines = append(lines, runtimeTraceProjElimRowLine(entry, topValue, model.Marks, zh, multiBoardRuler, anchorHoisted))
		if note, ok := runtimeTraceProjElimCompositionNoteLine(entry.row, model.Marks, zh); ok {
			tag := strings.TrimSpace(entry.row.EvidenceTag)
			if tag == "" {
				tag = "-"
			}
			label := "构成拆解"
			if !zh {
				label = "composition"
			}
			// 双复核修复 件13 (冷读 CR11, §29.175.8 值在首段, 2026-07-21).
			// EVOLUTION RECORD: the content column opened on the [E#] pointer —
			// the value-first note now leads and the pointer closes the row.
			decomp = append(decomp, runtimeTraceProjElimAuxRow{label: label, content: note + " [" + tag + "]"})
		}
	}
	// Zone ① — ⛓ 方向节区 (§29.175.6 区域序终裁): the chain members render in
	// fix-direction sections (§29.61.12 ② block order preserved — the board
	// sorts the whole ⛓ block first, so the slice split transcribes the block
	// order byte-for-byte; the 件4 semantic fallback joins its own direction's
	// section, eff ≤ every TOP5 chain eff keeps sections internally eff-desc).
	// 件7: a section whose members unanimously carry ONE typed board target
	// hoists the ·板锚 chip onto its head; mixed sections keep per-row chips.
	sections := runtimeTraceProjElimSectionsFor(renderedChain)
	for _, section := range sections {
		hoist := ""
		if multiBoardRuler {
			hoist = runtimeTraceProjElimSectionHoistedAnchor(section)
		}
		lines = append(lines, runtimeTraceProjElimSectionHeadLine(section, multiBoardRuler, len(namedTargets) > 0, hoist, model.Marks, zh))
		for _, entry := range section.entries {
			appendMember(entry, chainTop, hoist != "")
		}
	}
	if !chainPresent {
		lines = append(lines, runtimeTraceProjElimEmptyChainLine(model, zh))
	}
	// Zone ② — ◈ 业务线索独立多行区 (§29.175.6: the name-dimension zone sits
	// between the direction zone and ◇; no bar — 名维度视觉区分, §29.175.7).
	if zone := runtimeTraceProjElimBusinessZoneLines(model, zh); len(zone) > 0 {
		lines = append(lines, "")
		lines = append(lines, zone...)
	}
	// Zone ③ — ◇ 邻近 TOP3 (§29.175.9): value desc within the zone, bar worn
	// (eliminable-amount dimension), zone-local ruler, tail count.
	if len(renderedAdjacent) > 0 {
		lines = append(lines, "")
		lines = append(lines, runtimeTraceProjElimAdjacentBlockHeadLine(model.Marks, zh))
		for _, entry := range renderedAdjacent {
			appendMember(entry, adjacentTop, false)
		}
		if adjacentCut > 0 {
			if zh {
				lines = runtimeTraceProjElimAppendNotes(lines, fmt.Sprintf("  另有 %d 行见明细", adjacentCut))
			} else {
				lines = runtimeTraceProjElimAppendNotes(lines, fmt.Sprintf("  %d more row(s) — see the detail table", adjacentCut))
			}
		}
	}
	// Zone ④ — ▒ 背景 TOP3 (§29.175.7): no bar (背景语境), tail count.
	if zone := runtimeTraceProjElimBackgroundZoneLines(model, zh); len(zone) > 0 {
		lines = append(lines, "")
		lines = append(lines, zone...)
	}
	// Zone ⑤ — — 辅助 — (§29.175.8 两列文法): 对账组 first (∩ overlap pairs +
	// the conservation verdict — they verify the zones above), 另账组 second
	// (未入榜 / 未入榜最大 / 口径旁栏 / conditional disclosures / 构成拆解 —
	// accounts outside the zones above). §29.112 closure identity: rendered +
	// cut counts + exclusion rows == the pre-exclusion population, unchanged.
	var aux []runtimeTraceProjElimAuxRow
	rendered := append(append([]runtimeTraceProjElimEntry{}, renderedChain...), renderedAdjacent...)
	aux = append(aux, runtimeTraceProjElimCrossDirectionFootnote(rendered, model.Marks, zh)...)
	aux = append(aux, runtimeTraceProjElimConservationLines(model, board, len(renderedChain) > 0, zh)...)
	// ELIM-GAP 件B 计数披露 (§29.104.15 件4 evolution). EVOLUTION RECORD —
	// 涉既裁位移③ (§29.104.15 件B 原裁: one counted line per channel —
	// 「⛓/◇ 持值行另有 N 行未入榜」 per-channel lines): the per-channel COUNT
	// obligation survives verbatim inside ONE merged 未入榜 row (§29.175.14
	// word form 「· 未入榜 ⛓ N 行 · ◇ M 行,见明细」). The former
	// 「(TOP5 值切)」 parenthetical retires with the zone layout (◇ counts
	// now follow the zone's own TOP3, §29.175.9).
	if chainCut > 0 || adjacentCut > 0 {
		var parts []string
		if zh {
			if chainCut > 0 {
				parts = append(parts, fmt.Sprintf("⛓ %d 行", chainCut))
			}
			if adjacentCut > 0 {
				parts = append(parts, fmt.Sprintf("◇ %d 行", adjacentCut))
			}
			aux = append(aux, runtimeTraceProjElimAuxRow{label: "未入榜",
				content: strings.Join(parts, " · ") + ",见明细"})
		} else {
			if chainCut > 0 {
				parts = append(parts, fmt.Sprintf("⛓ %d row(s)", chainCut))
			}
			if adjacentCut > 0 {
				parts = append(parts, fmt.Sprintf("◇ %d row(s)", adjacentCut))
			}
			aux = append(aux, runtimeTraceProjElimAuxRow{label: "unranked",
				content: strings.Join(parts, " · ") + " — see the detail table"})
		}
	}
	// 件6+件9 (§29.175.10/.14): the 未入榜最大 rows (the former PARTSPLIT ◎
	// mention block, compressed — the sibling row of 未入榜 by design).
	aux = append(aux, runtimeTraceProjElimGatedCompositeEdgeShareMentionRows(model, zh)...)
	aux = append(aux, runtimeTraceProjElimAuxAccountRows(model, board, zh)...)
	// SELFRUN-DISC (§29.192① (b)): the self supply-fold 「量不了」 absence
	// disclosure row (另账组 conditional disclosure — distinguishes
	// "unmeasurable" from "no loss"; no seat, no ordinal, no census).
	aux = append(aux, runtimeTraceProjElimSelfFoldUnmeasuredRows(model, zh)...)
	if row, ok := runtimeTraceProjElimRepresentedFootnote(model, zh); ok {
		aux = append(aux, row)
	}
	// XLANE-2 件1: the member-subset exclusion's dedicated disclosure row
	// (排除≠消失 — covers seated subset rows kept off the population AND
	// seatless ones kept off the semantic census).
	if row, ok := runtimeTraceProjElimMemberSubsetFootnote(model, zh); ok {
		aux = append(aux, row)
	}
	// LEVELMERGE-1 件2: the gated-share constituent exclusion's dedicated
	// disclosure row.
	if row, ok := runtimeTraceProjElimGatedConstituentFootnote(model, zh); ok {
		aux = append(aux, row)
	}
	// A2 件12 (§29.192): 构成拆解 is a proliferable aux family — TOP3 in
	// board seat order (the board is value-desc, so the slice IS the value
	// TOP3) + an honest tail count; the full decompositions stay on the 行3/
	// detail faces.
	const elimDecompTopN = 3
	if len(decomp) > elimDecompTopN {
		rest := len(decomp) - elimDecompTopN
		decomp = decomp[:elimDecompTopN]
		label := "构成拆解"
		content := fmt.Sprintf("另有 %d 项见明细", rest)
		if !zh {
			label = "composition"
			content = fmt.Sprintf("%d more — see the detail blocks", rest)
		}
		decomp = append(decomp, runtimeTraceProjElimAuxRow{label: label, content: content})
	}
	aux = append(aux, decomp...)
	if zone := runtimeTraceProjElimAuxZoneLines(aux, model.Marks, zh); len(zone) > 0 {
		lines = append(lines, "")
		lines = append(lines, zone...)
	}
	return runtimeTraceProjElimClose(lines, model.Marks)
}

// runtimeTraceProjElimClose seals the ◎ fence. SMALL3-1 件3 (§29.196④ settle
// of the A2R-8 filing, 2026-07-21; UX-2 的 ◎ 版). EVOLUTION RECORD: the ◎
// width governor (runtimeTraceProjElimNoteLines below) covered only the
// footnote/note families — bar seat rows, ▸ direction heads, the ▒ zone head
// and ◈ business rows rendered whole past the 100-cell budget (A2R-8 witness:
// customer runnable_2 157-160-cell rows; baseline census 59 over-budget ◎
// lines across twelve flagship dumps, zh 101-112 / EN up to 155). Per the
// §29.175.14 discipline the splittable same-level families (aux `· ` rows)
// keep their split-first governor at their emission sites; every remaining
// structural single-row family now folds through the SHARED A2 fold device
// (runtimeTraceProjFoldHeadLine — same ⤷ marker, same breakpoint whitelist,
// same mark → legend/mini-key wiring) as a seal-time pass: content is
// byte-whole (wrap, never truncation), continuations indent two cells past
// the host line's lead, and a line already within the budget stays
// byte-identical. A single unbreakable over-wide atom renders whole (the fold
// device's honest fail-open) — no such row exists in the census.
func runtimeTraceProjElimClose(lines []string, marks *runtimeTraceProjMarkSet) string {
	folded := make([]string, 0, len(lines))
	for _, line := range lines {
		if runewidth.StringWidth(line) <= runtimeTraceProjTreeRowMaxWidth {
			folded = append(folded, line)
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))] + "  "
		folded = append(folded, strings.Split(runtimeTraceProjFoldHeadLine(line, indent, marks), "\n")...)
	}
	return tracefence.ElimOpener + "\n" + strings.Join(folded, "\n") + "\n```"
}

// runtimeTraceProjElimNoteLines is the ◎ region's width governor
// (§29.104.18.2 件2 / §29.114 P2, 2026-07-17): footnote (`· `) and
// note/seat/decomp (`  `) lines over the 100-cell cap break at token
// boundaries through runtimeTraceProjWrapDisplay — the SAME engine the tree
// faces wrap with, so the §29.114 no-break disciplines (CJK word runs,
// registered compounds, value+caliber super-atoms, E# references, `·` never
// dangles at EOL) all carry over. Bar/member/head lines are STRUCTURAL and
// are never routed here (the callers route only the footnote/note families —
// classification is by emission site, never by sniffing bar bytes).
// Continuations indent two cells past the lead; the wrap keeps a break space
// at its chunk boundary and the emitted physical line trims it (件①(f)/C1
// discipline). Lines within the cap return byte-identically, so every short
// single-board fence is untouched.
func runtimeTraceProjElimNoteLines(line string) []string {
	var lead, cont string
	switch {
	case strings.HasPrefix(line, "· "):
		lead, cont = "· ", "  "
	case strings.HasPrefix(line, "  "):
		lead, cont = "  ", "    "
	default:
		return []string{line}
	}
	if runewidth.StringWidth(line) <= runtimeTraceProjTreeRowMaxWidth {
		return []string{line}
	}
	width := runtimeTraceProjTreeRowMaxWidth - runewidth.StringWidth(cont)
	if width < runtimeTraceProjTreeNameMinWidth {
		width = runtimeTraceProjTreeNameMinWidth
	}
	chunks := runtimeTraceProjWrapDisplay(line[len(lead):], width)
	out := make([]string, 0, len(chunks))
	prefix := lead
	for i, chunk := range chunks {
		if i > 0 {
			chunk = strings.TrimLeft(chunk, " ")
		}
		out = append(out, strings.TrimRight(prefix+chunk, " "))
		prefix = cont
	}
	return out
}

// runtimeTraceProjElimAppendNotes routes footnote/note lines through the ◎
// width governor (件2): one call site shape for every note family.
func runtimeTraceProjElimAppendNotes(lines []string, notes ...string) []string {
	for _, note := range notes {
		lines = append(lines, runtimeTraceProjElimNoteLines(note)...)
	}
	return lines
}

// runtimeTraceProjElimEmptyChainLine is the honest empty-chain form (design
// §2.5/§4 honest-fallback): the chain channel admitted no valued row — say
// so, transcribe the typed flat-lane cause when one exists (single source:
// the same tracefence banner constants the tree head prints, ⊘ stripped),
// and state the crown consequence.
func runtimeTraceProjElimEmptyChainLine(model runtimeTraceProjTreeModel, zh bool) string {
	cause := ""
	if strings.TrimSpace(model.Target) == "" {
		cause = strings.TrimPrefix(runtimeTraceProjFlatFallbackHeader(model, zh), tracefence.GlyphUndrillable+" ")
	}
	if zh {
		line := "链上:本窗无链上持值行"
		if cause != "" {
			line += " · " + cause
		}
		return line + " —— 无已证链上可消除量,主根因不加冕"
	}
	line := "on-chain: no on-chain valued row in this window"
	if cause != "" {
		line += " · " + cause
	}
	return line + " — no proven on-chain eliminable amount; no primary-cause crown"
}

// runtimeTraceProjElimAuxAccountRows builds the 另账组 account rows of the
// auxiliary zone (排除≠消失, absence never guesses; OMGCLEAN-1 件9 two-column
// re-shape of the former mention-obligation footnotes):
//
//   - 口径旁栏 rows (V2-P0 协同): each excluded valued caliber-side row named
//     with its shared-registry value-class word (capped, counted overflow);
//     §29.175.13 降噪: the label is the PLAIN word — the ⌗ glyph and the
//     (非墙钟,不占序数) boilerplate stay on the tree/legend faces;
//   - 自身症状: target-self wait-symptom rows, one counted row (症状面, the
//     rows live in the target stanza + detail blocks);
//   - 邻近段最大 (O-5 pointer): the adjacent stanza holds valued
//     NON-population rows and the channel fielded no member — point at the
//     largest instead of inventing a member;
//   - 语义优化: the seatless semantic census rows (RNB-2 件4 W4-a/E30).
//
// (The former ▒ pointer line and ◈ footnote block are zones of their own now
// — §29.175.6/.7; their builders sit below.)
func runtimeTraceProjElimAuxAccountRows(model runtimeTraceProjTreeModel, board []runtimeTraceProjElimEntry, zh bool) []runtimeTraceProjElimAuxRow {
	var rows []runtimeTraceProjElimAuxRow
	const caliberCap = 3
	caliberListed := 0
	caliberTotal := 0
	// §29.104.18.2 件1 (2026-07-17) + CALSIDE-1 件1 (2026-07-19): the ⌗ seats
	// collect as STRUCTS carrying subject + semantic class word + the
	// single-source value form. OMGCLEAN-1 件9 (§29.175.8/.13/.14). EVOLUTION
	// RECORD: the one-line/head+indent dual form retires into same-level
	// 口径旁栏 aux rows — one row per seat (同级行纪律, never a continuation),
	// the label is the PLAIN 口径旁栏 word (⌗ glyph stripped on this face,
	// §29.175.13 降噪 — the tree 行2 ⌗ face and its legend entry stay), and
	// the (非墙钟,不占序数) boilerplate lives on the legend; the zh
	// single-source value forms still carry their class word at the point of
	// reading (计数当量X(非墙钟) …).
	type elimCaliberSeat struct {
		subject, class, value, tag string
		node                       types.TraceCausalProjectionNode
	}
	var caliberSeats []elimCaliberSeat
	selfCount := 0
	var selfTags []string
	adjacentEligible := false
	for i := range board {
		if board[i].channelRank == 1 {
			adjacentEligible = true
		}
	}
	var adjacentPointer *runtimeTraceProjTreeRow
	scan := func(rowsIn []runtimeTraceProjTreeRow) {
		for i := range rowsIn {
			row := rowsIn[i]
			if !row.HasData || row.Node.OnChainOverflowFold {
				continue
			}
			if row.Node.IsTargetSelfStateRow() {
				if row.Node.EffectiveImpactMS > 0 || runtimeTraceProjNodeDisplayImpact(row.Node) > 0 {
					selfCount++
					if tag := strings.TrimSpace(row.EvidenceTag); tag != "" && len(selfTags) < caliberCap {
						selfTags = append(selfTags, "["+tag+"]")
					}
				}
				continue
			}
			caliberSide := runtimeTraceProjElimCaliberSideRow(row.Node)
			if caliberSide && runtimeTraceProjElimRankItemRow(row) {
				switch runtimeTraceProjRowOrdinalChannel(row) {
				case runtimeTraceProjOrdinalChannelChain, runtimeTraceProjOrdinalChannelAdjacent,
					runtimeTraceProjOrdinalChannelBackground:
				default:
					continue
				}
				value := row.Node.EffectiveImpactMS
				if value <= 0 {
					value = runtimeTraceProjNodeDisplayImpact(row.Node)
				}
				if value <= 0 {
					continue
				}
				caliberTotal++
				if caliberListed < caliberCap {
					caliberListed++
					// §29.55 观察③ 两形一裁 (2026-07-14): every row on this
					// account is caliber-side (count / composite score) — its
					// magnitude is NOT wall-clock ms, so the value renders
					// through its single-source class-worded form.
					//
					// LT-HYG mark70 (§29.79 观察续档, 2026-07-14): this
					// emission point lights the SAME marks as the tree 行2
					// site (词条-图例双向) — under a folded ◇ stanza this
					// account can be the ONLY renderer of the ⌗ word family,
					// and an unlit mark decouples the 计数当量 wordface from
					// its legend entry.
					model.Marks.mark(runtimeTraceProjMarkCaliberSideRow)
					if runtimeTraceProjCountEquivalentValueCaliber(row.Node) {
						model.Marks.mark(runtimeTraceProjMarkFamilyCountEquivalent)
					}
					// DISPLAY-WRAP 件④(c) (§29.104.18.1 A6): the value carries
					// its caliber AT the point of reading — count/composite
					// classes adopt their single-source value forms (same
					// bytes as the tree 行1); an unresolved class keeps the
					// suffix-free number (the tier itself stays the precise
					// signal).
					valueText := fmt.Sprintf("%.3f", value)
					switch {
					case runtimeTraceProjCountEquivalentValueCaliber(row.Node):
						valueText = runtimeTraceProjCountEquivalentValueText(value, zh)
						if !zh {
							// 双复核修复 件3 (冷读 CR6, 2026-07-21). EVOLUTION
							// RECORD: the EN row appended a bare 「· 计数当量」
							// zh tail — count-equivalent already leads the
							// value form, so the residual was deleted; the
							// (not wall clock) qualifier compresses off this
							// aux row only (§29.175.8 一短句 + EN 禁续行 —
							// the legend + tree faces keep the full form; a
							// changed source form fails open to full bytes).
							valueText = strings.TrimSuffix(valueText, " (not wall clock)")
						}
					case runtimeTraceProjCompositeValueCaliber(row.Node):
						valueText = runtimeTraceProjCompositeScoreValueText(value, zh)
						if !zh {
							// Same 件3 compression on the composite twin (the
							// former 「· 综合评分」 residual is deleted with it).
							valueText = strings.Replace(valueText, ", not wall clock)", ")", 1)
						}
					}
					caliberSeats = append(caliberSeats, elimCaliberSeat{
						subject: runtimeTraceProjElimSubject(row, zh),
						class:   strings.TrimSpace(runtimeTraceProjElimClassWord(row, zh, false, nil)),
						value:   valueText,
						tag:     strings.TrimSpace(row.EvidenceTag),
						node:    row.Node,
					})
				}
			}
		}
	}
	scan(model.SelfRows)
	scan(model.TreeRows)
	scan(model.Adjacent)
	scan(model.Background)
	// RNB-2 件4 W4-a + E30 形 (§29.88 R1/W4, 2026-07-15): the SEATLESS
	// semantic-row census — one counted 语义优化 row per channel: count +
	// per-class breakdown + the largest value with its [E#] — a pointer row,
	// never a member (§29.42.4 zero minting; O-5 指针 default). Caliber-side
	// rows stay on the 口径旁栏 account.
	type elimSemanticCensus struct {
		count    int
		order    []string
		perClass map[string]int
		maxValue float64
		maxTag   string
	}
	semCensus := map[string]*elimSemanticCensus{}
	semScan := func(rowsIn []runtimeTraceProjTreeRow) {
		for i := range rowsIn {
			row := rowsIn[i]
			if !runtimeTraceProjElimSemanticCensusRow(row) {
				continue
			}
			// XLANE-2 件1: a member-subset demoted seat leaves the census —
			// its spans are the superset seat's account; the dedicated subset
			// disclosure row represents it instead (排除≠消失, no double
			// presence).
			if strings.TrimSpace(row.SemanticMemberSubsetOf) != "" {
				continue
			}
			value := runtimeTraceProjNodeDisplayImpact(row.Node)
			channel := runtimeTraceProjRowOrdinalChannel(row)
			census := semCensus[channel]
			if census == nil {
				census = &elimSemanticCensus{perClass: map[string]int{}}
				semCensus[channel] = census
			}
			census.count++
			classWord := runtimeTraceProjFamilySemanticClassWord(row.Node, zh)
			if classWord == "" {
				classWord = strings.TrimSpace(row.Node.SemanticClass)
			}
			if _, seen := census.perClass[classWord]; !seen {
				census.order = append(census.order, classWord)
			}
			census.perClass[classWord]++
			if value > census.maxValue {
				census.maxValue = value
				census.maxTag = strings.TrimSpace(row.EvidenceTag)
			}
		}
	}
	semScan(model.SelfRows)
	semScan(model.TreeRows)
	semScan(model.Adjacent)
	if !adjacentEligible {
		for i := range model.Adjacent {
			row := &model.Adjacent[i]
			if !row.HasData || row.Node.IsTargetSelfStateRow() || runtimeTraceProjElimRankItemRow(*row) {
				continue
			}
			if runtimeTraceProjElimCaliberSideRow(row.Node) {
				continue
			}
			if runtimeTraceProjNodeDisplayImpact(row.Node) <= 0 {
				continue
			}
			if adjacentPointer == nil || runtimeTraceProjNodeDisplayImpact(row.Node) > runtimeTraceProjNodeDisplayImpact(adjacentPointer.Node) {
				adjacentPointer = row
			}
		}
	}
	caliberLabel := "口径旁栏"
	if !zh {
		caliberLabel = "caliber sidebar"
	}
	for _, seat := range caliberSeats {
		lead := seat.subject + " "
		if seat.class != "" {
			lead = seat.subject + " · " + seat.class + " · "
		}
		content := lead + seat.value
		if seat.tag != "" {
			content += " [" + seat.tag + "]"
		}
		rows = append(rows, runtimeTraceProjElimAuxRow{label: caliberLabel, content: content})
	}
	if rest := caliberTotal - caliberListed; rest > 0 {
		// A2 件12 (§29.192): tail wording aligned to the ◈◇▒ family grammar
		// (「另有 N 行见明细」). The ⌗ display cap was ALREADY 3 (caliberCap);
		// rows keep engine order — a value sort would compare across
		// non-wall-clock units (计数当量 vs 综合评分, 跨单位禁比), deviation
		// recorded. The ENGINE caliberSide lane cap stays 4 (wire change out
		// of this batch's scope — 论证保留).
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: caliberLabel,
				content: fmt.Sprintf("另有 %d 行见明细", rest)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: caliberLabel,
				content: fmt.Sprintf("%d more row(s) — see the detail table", rest)})
		}
	}
	if selfCount > 0 {
		tagList := ""
		if len(selfTags) > 0 {
			tagList = " " + strings.Join(selfTags, runtimeTraceProjElimJoinSep(zh))
		}
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "自身症状",
				content: fmt.Sprintf("%d 行(症状面,非可消除量)见关注线程区%s", selfCount, tagList)})
		} else {
			// 双复核修复 件3 (冷读 CR3, 2026-07-21): EN clause compressed —
			// "(symptom face, not eliminable)" — so the row stays inside the
			// 100-cell budget (禁续行; full semantics on the legend).
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "self symptom",
				content: fmt.Sprintf("%d row(s) (symptom face, not eliminable) — see the target stanza%s", selfCount, tagList)})
		}
	}
	if adjacentPointer != nil {
		value := runtimeTraceProjNodeDisplayImpact(adjacentPointer.Node)
		tag := strings.TrimSpace(adjacentPointer.EvidenceTag)
		if tag != "" {
			tag = " [" + tag + "]"
		}
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "邻近段最大",
				content: fmt.Sprintf("%.3fms 见邻近段%s(不在根因排序,不参与汇排)", value, tag)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "adjacent max",
				content: fmt.Sprintf("%.3fms — see the adjacent stanza%s (outside the rank population, not ranked here)", value, tag)})
		}
	}
	// 件4 W4-a/E30: one counted semantic-census row per channel (◇ then ⛓ —
	// the ◇ witness family is the filed case; the ⛓ form is its symmetric
	// twin for on-chain seatless semantic rows).
	semLabel := "语义优化"
	if !zh {
		semLabel = "semantic leads"
	}
	for _, channel := range []string{runtimeTraceProjOrdinalChannelAdjacent, runtimeTraceProjOrdinalChannelChain} {
		census := semCensus[channel]
		if census == nil || census.count == 0 {
			continue
		}
		var classes []string
		for _, word := range census.order {
			if zh {
				classes = append(classes, fmt.Sprintf("%s%d", word, census.perClass[word]))
			} else {
				classes = append(classes, fmt.Sprintf("%s %d", word, census.perClass[word]))
			}
		}
		maxPart := fmt.Sprintf("%.3fms", census.maxValue)
		if census.maxTag != "" {
			maxPart += " [" + census.maxTag + "]"
		}
		// 双复核修复 件13 (冷读 CR11+CR3, 2026-07-21). EVOLUTION RECORD: the
		// rows closed on a SECOND parenthetical (「(未铸序数,不参与汇排)」/
		// 「(未入根因排序,不参与汇排)」 / "(no ordinal minted, not ranked
		// here)") — the densest aux rows on the face and the EN wrap culprits.
		// The clause sinks onto the — 辅助 — legend entry (另账行 blanket
		// no-ordinal/no-ranking sentence); the EN pointer compresses to keep
		// the row inside the 100-cell budget (禁续行).
		if channel == runtimeTraceProjOrdinalChannelAdjacent {
			if zh {
				rows = append(rows, runtimeTraceProjElimAuxRow{label: semLabel,
					content: fmt.Sprintf("◇ %d 行(%s,最大 %s)见邻近段",
						census.count, strings.Join(classes, "、"), maxPart)})
			} else {
				rows = append(rows, runtimeTraceProjElimAuxRow{label: semLabel,
					content: fmt.Sprintf("◇ %d row(s) (%s; largest %s) — see the adjacent stanza",
						census.count, strings.Join(classes, ", "), maxPart)})
			}
			continue
		}
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: semLabel,
				content: fmt.Sprintf("⛓ %d 行(%s,最大 %s)见主树语义行",
					census.count, strings.Join(classes, "、"), maxPart)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: semLabel,
				content: fmt.Sprintf("⛓ %d row(s) (%s; largest %s) — see the semantic rows",
					census.count, strings.Join(classes, ", "), maxPart)})
		}
	}
	// §18 G1 (双维度审计, user ruling 2026-07-28): the 排除≠消失 account for
	// UNPRICED genuine occupancy — an on-chain row whose raw window occupancy
	// is real but prices to zero on the eliminable dimension (e.g. full-
	// frequency running with no supply deficit, span-less) used to vanish
	// from every guidance face. One counted row keeps the raw-occupancy
	// dimension visible: the time is genuine, the lever is the thread's OWN
	// workload/business flow — a NEW fix direction to explore, never an
	// addend of this board. Population is precise: on-chain context-only
	// valued rows minus the families already accounted above (caliber-side
	// / self-symptom / semantic census).
	unpricedCount := 0
	unpricedMax := 0.0
	unpricedTag := ""
	unpricedScan := func(rowsIn []runtimeTraceProjTreeRow) {
		for i := range rowsIn {
			row := rowsIn[i]
			if !row.HasData || !row.Node.IsContextOnlyRow() {
				continue
			}
			if strings.TrimSpace(row.Node.ChainRelevance) != "on_chain" {
				continue
			}
			if row.Node.IsTargetSelfStateRow() || runtimeTraceProjElimSemanticCensusRow(row) {
				continue
			}
			if runtimeTraceProjElimCaliberSideRow(row.Node) {
				continue
			}
			value := runtimeTraceProjNodeDisplayImpact(row.Node)
			if value <= 0 {
				continue
			}
			unpricedCount++
			if value > unpricedMax {
				unpricedMax = value
				unpricedTag = strings.TrimSpace(row.EvidenceTag)
			}
		}
	}
	unpricedScan(model.TreeRows)
	if unpricedCount > 0 {
		maxPart := fmt.Sprintf("%.3fms", unpricedMax)
		if unpricedTag != "" {
			maxPart += " [" + unpricedTag + "]"
		}
		if zh {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "未计价占用",
				content: fmt.Sprintf("⛓ %d 行(最大 %s)·真实占时·杠杆=自身工作量(新方向)", unpricedCount, maxPart)})
		} else {
			rows = append(rows, runtimeTraceProjElimAuxRow{label: "unpriced occupancy",
				content: fmt.Sprintf("⛓ %d row(s) (largest %s) · genuine time · lever: own workload", unpricedCount, maxPart)})
		}
	}
	return rows
}

// runtimeTraceProjElimBackgroundZoneLines renders the ▒ 背景 zone (OMGCLEAN-1
// 件4, §29.175.7 用户裁定: 多行 TOP3, 窗内投影值降序, 尾部计数; no bar —
// 背景语境 is outside the eliminable-amount dimension, 基石 C). The head
// keeps the caliber-boundary declaration (跨线程/他线程口径,不计入链上归因 —
// never a value comparison against the chain rows); caliber-side background
// rows stay on their 口径旁栏 account (no double presence) yet keep counting
// into the zone total (the head count is the whole background stanza, as the
// former pointer line counted it). A row whose projection exceeds the window
// wears the honest 超窗 short mark (他线程口径 can legally exceed the target
// window). nil = no background rows = no zone.
func runtimeTraceProjElimBackgroundZoneLines(model runtimeTraceProjTreeModel, zh bool) []string {
	total := 0
	var candidates []runtimeTraceProjTreeRow
	for i := range model.Background {
		row := model.Background[i]
		if !row.HasData {
			continue
		}
		total++
		if runtimeTraceProjElimCaliberSideRow(row.Node) {
			continue
		}
		if runtimeTraceProjNodeDisplayImpact(row.Node) <= 0 {
			continue
		}
		candidates = append(candidates, row)
	}
	if total == 0 {
		return nil
	}
	// 双复核修复 件9 (冷读 CR8, 2026-07-21): the DEGENERATE form — the stanza
	// holds rows yet NONE carries a displayable in-window projected value
	// (caliber-side rows keep their 口径旁栏 account; valueless rows have no
	// value row to mint). The former head+tail pair read as double counting
	// (「1 行 … 详见背景段」 + 「另有 1 行见背景段」) — the zone collapses to
	// ONE honest line naming why no value row renders.
	if len(candidates) == 0 {
		if zh {
			return []string{fmt.Sprintf("▒ 背景压力 %d 行(无窗内投影值)见背景段", total)}
		}
		return []string{fmt.Sprintf("▒ %d background-pressure row(s) (no in-window projected value) — see the background stanza", total)}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return runtimeTraceProjNodeDisplayImpact(candidates[i].Node) > runtimeTraceProjNodeDisplayImpact(candidates[j].Node)
	})
	if len(candidates) > runtimeTraceProjElimAdjacentTopN {
		candidates = candidates[:runtimeTraceProjElimAdjacentTopN]
	}
	var lines []string
	if zh {
		lines = append(lines, fmt.Sprintf("▒ 背景压力 %d 行(跨线程/他线程口径,不计入链上归因,详见背景段)", total))
	} else {
		lines = append(lines, fmt.Sprintf("▒ %d background-pressure row(s) (cross-thread / other-thread calibers; never counted into the chain attribution — see the background stanza)", total))
	}
	for _, row := range candidates {
		value := runtimeTraceProjNodeDisplayImpact(row.Node)
		line := fmt.Sprintf("%9.3fms %s", value, runtimeTraceProjElimSubject(row, zh))
		if class := runtimeTraceProjElimClassWord(row, zh, false, nil); class != "" {
			line += " · " + class
		}
		if model.WindowMS > 0 && value > model.WindowMS+runtimeTraceProjElimEnvelopeToleranceMs {
			if zh {
				line += " ·超窗(他线程口径)"
			} else {
				line += " · over-window (other-thread caliber)"
			}
		}
		if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
			line += " [" + tag + "]"
		}
		lines = append(lines, line)
	}
	if rest := total - len(candidates); rest > 0 {
		if zh {
			lines = runtimeTraceProjElimAppendNotes(lines, fmt.Sprintf("  另有 %d 行见背景段", rest))
		} else {
			lines = runtimeTraceProjElimAppendNotes(lines, fmt.Sprintf("  %d more row(s) — see the background stanza", rest))
		}
	}
	return lines
}

// runtimeTraceProjElimBusinessZoneLines renders the ◈ 业务线索 zone
// (OMGCLEAN-1 件5 + rider3 + §29.175.6 区域序终裁). EVOLUTION RECORD —
// 涉既裁位移⑤ (§29.147 定形 established the ◎ footnote face with full
// per-family rows + the F2 non-additive clause; the §29.174 处置⑥ pool item
// UX-12② 「◈ 双面合并」 was marked 涉既裁承诺面,禁单方面动 — §29.175.5/.6
// is the fresh user mandate adjudicating it): the ◎ face becomes the
// compact TOP8 selection zone (双复核修复 F6: stale TOP3 → RULE3-1 件11
// §29.185③ TOP5; MENTION8-1 §29.203 TOP5 → TOP8), the tree ◈ block keeps
// the detailed roster
// (行号/凭证), and the F2 clause survives on its own head line both faces.
// Zone semantics: the name-dimension
// selection set (单次最长∪合计最长 TOP8, engine-selected — 双复核修复 F6:
// stale TOP3 → §29.185③ TOP5; MENTION8-1 §29.203 TOP8; the full promise
// word lives on the tree ◈ head + legend), ONE 定稿 head line (双复核 件8),
// one compact row per family (值·线程·span 名·次数(·单次最大) — no bar, no
// line numbers, no credential words: 行号/凭证细节 live on the tree ◈ block),
// and the honest not-listed tail count (双复核 件4 — same wording family as
// the tree face). nil = no valid mention = no zone (absence silent).
func runtimeTraceProjElimBusinessZoneLines(model runtimeTraceProjTreeModel, zh bool) []string {
	var rows []string
	for _, m := range model.BusinessSpanMentions {
		text, ok := runtimeTraceProjBusinessSpanMentionCompactRowText(m, model.BusinessSpanMentions, zh)
		if !ok {
			continue
		}
		rows = append(rows, text)
	}
	if len(rows) == 0 {
		return nil
	}
	model.Marks.mark(runtimeTraceProjMarkBusinessSpanMention)
	var lines []string
	// 双复核修复 件8 (对抗 CR-12 瘦身评估 + 主会话定稿, 2026-07-21). EVOLUTION
	// RECORD: the ◎ zone opened on TWO head lines (业务线索(选择规则 · 目的词)
	// + the full F2 clause line) — the 定稿 single head folds them with zero
	// information loss on the zone: 业务自查减时 = the purpose short form,
	// 非确定性优化类 = the rider3 exclusion boundary, 不占序数 + 族间不可相加
	// = the ordinal/F2 facts (the full selection-rule promise word and the
	// full F2 clause keep their seats on the tree ◈ head + legend, 词面单点).
	if zh {
		lines = append(lines, "◈ 业务线索 · 业务自查减时(非确定性优化类 · 不占序数 · 族间不可相加)")
	} else {
		lines = append(lines, "◈ business leads · self-check to cut time (non-semantic-class · no ordinal · families never add)")
	}
	lines = append(lines, rows...)
	if model.BusinessSpanMentionOmitted > 0 {
		// 双复核修复 件4 (冷读 CR4/对抗 CR-6 死指针, 2026-07-21). EVOLUTION
		// RECORD — authority artifact: §29.175.6 fixed the ◎ tail as the tree
		// pointer (「尾部计数指树」) while the tree block was still the rider2
		// full roster; rider3 (§29.175.5②) converged the tree roster onto the
		// SAME selection set, so 「见树◈块」 pointed at a block that lists the
		// identical families and itself says 未列出 — a dead pointer. 主会话裁
		// 双面诚实形: both faces speak the same honest not-listed count. 件14:
		// the EN n==1 plural branch.
		if zh {
			lines = runtimeTraceProjElimAppendNotes(lines, fmt.Sprintf("  另有 %d 族(≥显著地板)未列出", model.BusinessSpanMentionOmitted))
		} else if model.BusinessSpanMentionOmitted == 1 {
			lines = runtimeTraceProjElimAppendNotes(lines, "  1 more family (at/above the significance floor) is not listed")
		} else {
			lines = runtimeTraceProjElimAppendNotes(lines, fmt.Sprintf("  %d more families (at/above the significance floor) are not listed", model.BusinessSpanMentionOmitted))
		}
	}
	return lines
}
func runtimeTraceProjElimJoinSep(zh bool) string {
	if zh {
		return "、"
	}
	return ", "
}
