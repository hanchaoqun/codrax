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
//	口径臂+值形臂  the SHARED registry ruler arm (V2-P0 §6.1 同一实现):
//	        CausalTokenCaliberSideClass == None on the display token, plus the
//	        typed ⌗ caliber_side tier guard for stale persisted forms — count
//	        equivalents and composite scores are footnote material, never
//	        汇排 members.
//
// Wording: the row transcription mints NO new per-row vocabulary — value
// (verbatim %.3f), channel identity (⛓ 链上/◇ 邻近 — glyph + the existing
// channel nouns, single emitter below), subject display name, class word,
// existing caliber words (family fold ladder), the Stage-1 self qualifier
// 「自身·确定性优化」 (§29.61.1 ruling ⑥: the ◇-era 「候选」 word drops when
// the row is on-chain; the ◇ epistemics live in the ◎ legend sentence, not
// per row — design §3.2, §29.36.4 冗余判据) and the row's E# pointer. The only
// new words are the ◎ region name and its legend sentence (design §3.4).
import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/mattn/go-runewidth"
)

// runtimeTraceProjElimRankItemRow is the 种群臂: the row is a typed VALUED
// RANK-ITEM (the engine's root_cause_rank lane), in any of its three rendered
// carriages —
//
//	(a) the rank record itself (Predicate root_cause_<tier>/…, the exact
//	    family the observation producers mint — never a prose parse);
//	(b) a chain-lane row that FOLDED its same-segment rank twin (RNB — the
//	    rank identity rides RankFoldPeers);
//	(c) a semantic entity that ADOPTED its rank twin's seat at the SEM twin
//	    fold (survivor carries Rank / BackgroundRank; adjacent survivors keep
//	    the ◇ ordinal).
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
	if strings.TrimSpace(row.Node.SemanticClass) != "" &&
		(row.Node.Rank > 0 || row.Node.BackgroundRank > 0) {
		return true
	}
	return false
}

// runtimeTraceProjElimEligible is the four-arm ◎ admission gate (design §2.2).
// The seat arm (displayed seat 1..5) deliberately does NOT participate — the
// population is 持值 rank-item rows regardless of chips/badges (design fixed
// the "transcribe the ◇ chip" gap: ◇ rows may hold values without chips).
func runtimeTraceProjElimEligible(row runtimeTraceProjTreeRow) bool {
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
	// 口径臂 + 值形臂 — ONE shared registry ruler arm (V2-P0 §6.1: count
	// additivity AND composite scores live on the ⌗ side), plus the typed
	// caliber_side tier for stale forms minted before the registry knew the
	// token. Wall-clock rows pass (CausalCaliberSideNone).
	if row.Node.IsCaliberSideRow() {
		return false
	}
	if tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken)) != tracequery.CausalCaliberSideNone {
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

// runtimeTraceProjElimTopN is the overview population bound (design §2.5:
// K=5 aligned with the §29.27.1 badge TOP N; the two constants may only move
// together through a joint re-ruling — pinned).
const runtimeTraceProjElimTopN = runtimeTraceProjBadgeTopN

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
// 折算,按大核满频 over a 3.860 raw). Arms, most specific first, every word
// from its existing single-source composer with its legend mark lit at THIS
// emission site (词条-图例双向; caliber-group entries render in stable
// catalog order, so the extra emission never reorders the legend):
//
//  1. on-chain semantic dual caliber — 链上计入(共N段,同线程);
//  2. family fold ladder — 合计/成员最大/计数合计(共N段,同线程);
//  3. event fold — 单次最大(a~b,共N次);
//  4. running supply deficit — 折算,按X核满频[,下界];
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
	if word, caliberMark, ok := runtimeTraceProjFamilyCaliberWord(node, zh); ok && runtimeTraceProjFamilyRow(node) {
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
// INV-SUPPLY 件① (§29.61.11, 2026-07-14): a supply-gap-dominant inversion
// seat transcribes its 行2 compound type word 优先级反转候选·供给缺口主导 in
// this slot — SAME composer, byte-identical (◎ 转录同词, 零新词源); the
// pre-INV-SUPPLY state word (running/runnable) that rode here said less than
// the seat's own 行2 and is superseded on exactly these seats.
func runtimeTraceProjElimClassWord(row runtimeTraceProjTreeRow, zh bool) string {
	node := row.Node
	if word, ok := runtimeTraceProjInversionSupplyGapCompoundWord(node, zh); ok {
		if node.MergedCount > 1 {
			word += runtimeTraceProjMergeCountChip(node.MergedCount, zh)
		}
		return word
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
	if word, _ := runtimeTraceProjRowCauseWordToken(row, zh); word != "" {
		if node.MergedCount > 1 {
			word += runtimeTraceProjMergeCountChip(node.MergedCount, zh)
		}
		return word
	}
	return strings.TrimSpace(runtimeTraceCausalProjectionDisplayCauseNameNode(node, zh))
}

// runtimeTraceProjElimQualifier is the row's identity qualifier slot:
//
//   - typed self basis (node.OnChainBasis, Stage-1 single field — never a
//     subject∧class∧relevance recomposition) → the Stage-1 qualifier word
//     自身·确定性优化 (ruling ⑥: the on-chain self row carries NO 候选 word);
//   - ◇ semantic rows → 确定性优化·候选 (§29.61 b identity word family: the
//     conditional-upper-bound word stays on the adjacent side only).
//
// Other rows carry none — the channel word plus the ◎ legend sentence carry
// the epistemics (design §3.2).
func runtimeTraceProjElimQualifier(row runtimeTraceProjTreeRow, channel string, zh bool) string {
	if strings.TrimSpace(row.Node.OnChainBasis) == "self_deterministic_span" {
		if zh {
			return "自身·确定性优化"
		}
		return "self·deterministic-optimization"
	}
	// SELF-ALL (§29.61.2, 2026-07-13): the wall-clock self basis wears its own
	// qualifier — same single-field fork, no 候选 word (the seat is a proven
	// wall-clock amount, not a conditional upper bound).
	if strings.TrimSpace(row.Node.OnChainBasis) == "self_wall_clock_interval" {
		if zh {
			return "自身·墙钟席"
		}
		return "self·wall-clock-seat"
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

// runtimeTraceProjElimRowLine renders ONE overview member line (design §2.3
// five elements: value · bar · channel word · subject · class[·caliber]
// [E#]); leadWidth is the constant lead field (恒定记号场 — the ◇最大
// fallback marker owns the same field on its row, so every value/bar column
// aligns on one grid).
func runtimeTraceProjElimRowLine(entry runtimeTraceProjElimEntry, top float64, lead string, leadWidth int, marks *runtimeTraceProjMarkSet, zh bool) string {
	row := entry.row
	channel := runtimeTraceProjRowOrdinalChannel(row)
	var b strings.Builder
	if pad := leadWidth - runewidth.StringWidth(lead); pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
	}
	b.WriteString(lead)
	b.WriteString(fmt.Sprintf("%9.3fms ", row.Node.EffectiveImpactMS))
	b.WriteString(runtimeTraceProjElimBarCells(row.Node.EffectiveImpactMS, top))
	b.WriteString(" " + runtimeTraceProjElimChannelWord(channel, zh))
	sep := " · "
	b.WriteString(sep + runtimeTraceProjElimSubject(row, zh))
	if class := runtimeTraceProjElimClassWord(row, zh); class != "" {
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
	if qual := runtimeTraceProjElimQualifier(row, channel, zh); qual != "" {
		b.WriteString(" ·" + qual)
	}
	if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
		b.WriteString(" [" + tag + "]")
	}
	return b.String()
}

// runtimeTraceProjElimCompositionNoteLine renders the INV-SUPPLY 件③
// (§29.61.11, 2026-07-14) per-seat eliminable-composition leverage note for a
// compound-word seat: 「可消除构成: 调度修复 X + 频点/热策略 Y」 — the seat's
// OWN 行3 attribution split (runtimeTraceProjInversionComponents, the SAME
// balance-gated builder, so the bytes can never disagree with 行3 and an
// unbalanced split refuses to render — 拒渲绝不造数) transcribed by
// elimination lever: 调度修复 = the runnable(全额) component, 频点/热策略 =
// the running(折算) component. A constituent display of ONE seat's value —
// never a Σ row, never added across rows (零求和红线; the renderer emits no
// total and no "=" claim). ok=false on every non-compound seat: the note
// exists exactly where the compound word says the supply gap dominates.
func runtimeTraceProjElimCompositionNoteLine(row runtimeTraceProjTreeRow, leadWidth int, marks *runtimeTraceProjMarkSet, zh bool) (string, bool) {
	if _, compound := runtimeTraceProjInversionSupplyGapCompoundWord(row.Node, zh); !compound {
		return "", false
	}
	components, _, ok := runtimeTraceProjInversionComponents(row.Node, zh)
	if !ok {
		return "", false
	}
	var parts []string
	for _, c := range components {
		lever := ""
		switch c.Word {
		case "runnable":
			if zh {
				lever = "调度修复"
			} else {
				lever = "scheduling fix"
			}
		case "running":
			if zh {
				lever = "频点/热策略"
			} else {
				lever = "frequency/thermal policy"
			}
		default:
			// A component outside the two-lever vocabulary has no leverage
			// label — the note refuses to guess (absence never invents).
			return "", false
		}
		parts = append(parts, lever+" "+runtimeTraceProjFmtMS(c.InMS))
	}
	if len(parts) == 0 {
		return "", false
	}
	marks.mark(runtimeTraceProjMarkElimComposition)
	head := "可消除构成: "
	if !zh {
		head = "eliminable composition: "
	}
	// Indent one step past the row lead so the note reads as the row's own
	// subordinate line (same "· " family as the tree's note lines), distinct
	// from the flush-left region footnotes.
	return strings.Repeat(" ", leadWidth) + "  · " + head + strings.Join(parts, " + "), true
}

// runtimeTraceProjElimFallbackLead is the ◇-max fallback row's lead-field
// marker (design §2.5: the largest conditional-upper-bound row is never
// buried by construction). Every other row pads the same constant field —
// 恒定记号场: one grid for every value/bar column.
func runtimeTraceProjElimFallbackLead(zh bool) string {
	if zh {
		return "◇最大 "
	}
	return "◇max "
}

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
func runtimeTraceProjElimHead(model runtimeTraceProjTreeModel, zh, withForm, chainPresent bool) []string {
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
	// SECTION-WIDE largest value wherever its row sits (链上条短=诚实).
	var title, ruler, form string
	if zh {
		if target == "" {
			target = "关注线程"
		}
		title = tracefence.ElimGlyph + " 窗内可消除量总览"
		ruler = "尺=" + target + " 窗内墙钟ms"
		blockWord := runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelChain, true) + "块先"
		if !chainPresent {
			blockWord = runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelAdjacent, true) + "块"
		}
		form = blockWord + "·块内值降序·零序数·零佩戴 · 定位走 [E#] · " + tracefence.ScaleMarkZH + "本区TOP1"
	} else {
		if target == "" {
			target = "focused thread"
		}
		title = tracefence.ElimGlyph + " Eliminable-in-window overview"
		ruler = "ruler = " + target + " in-window wall-clock ms"
		blockWord := runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelChain, false) + " block first"
		if !chainPresent {
			blockWord = runtimeTraceProjElimChannelWord(runtimeTraceProjOrdinalChannelAdjacent, false) + " block"
		}
		form = blockWord + " · value desc within block · zero ordinals · zero wear · locate via [E#] · " + tracefence.ScaleMarkEN + " section TOP1"
	}
	sep := " · "
	if !withForm {
		return []string{title + sep + ruler}
	}
	head := title + sep + ruler + sep + form
	if runewidth.StringWidth(head) <= runtimeTraceProjTreeRowMaxWidth {
		return []string{head}
	}
	// NEW-10-style wrap: both facts stay whole instead of truncating either.
	return []string{title + sep + ruler, form}
}

// runtimeTraceProjElimOverviewFence renders the ◎ overview fence (design §2,
// RANK-U Stage 2 commit D). "" when the run never observed the root_cause_
// rank family (typed projection flag — a board that never existed has no
// overview face; a board that ran but admitted nothing renders the honest
// empty-board line instead: 排除≠消失, absence of板≠空板).
func runtimeTraceProjElimOverviewFence(projection types.TraceCausalProjection, model runtimeTraceProjTreeModel, zh bool) string {
	if !projection.RootCauseFamilyObserved {
		return ""
	}
	board := runtimeTraceProjElimBoard(model)
	top := board
	if len(top) > runtimeTraceProjElimTopN {
		top = top[:runtimeTraceProjElimTopN]
	}
	// ◇-max fallback (design §2.5): adjacent-channel members exist but none
	// made TOP5 — append the largest one under its own lead marker.
	// §29.61.12 ② 核后如实注: under causal-tier blocking the ◇ block sorts
	// AFTER the whole ⛓ block and eff-desc within itself, so the first ◇
	// entry past TOP-K IS the ◇ maximum by construction — the fallback
	// guarantee is naturally satisfied and this scan is now a structural
	// identity (kept as the honest implementation).
	var fallback *runtimeTraceProjElimEntry
	adjacentInTop := false
	for i := range top {
		if top[i].channelRank == 1 {
			adjacentInTop = true
			break
		}
	}
	if !adjacentInTop {
		for i := runtimeTraceProjElimTopN; i < len(board); i++ {
			if board[i].channelRank == 1 {
				fallback = &board[i]
				break
			}
		}
	}
	chainPresent := false
	for i := range board {
		if board[i].channelRank == 0 {
			chainPresent = true
			break
		}
	}
	model.Marks.mark(runtimeTraceProjMarkElimOverview)
	var lines []string
	lines = append(lines, runtimeTraceProjElimHead(model, zh, len(board) > 0, chainPresent)...)
	if len(board) == 0 {
		// 空板形 (design §2.5): the board ran and admitted nothing — one
		// honest line, no silent-disappearance path.
		if zh {
			lines = append(lines, tracefence.ElimGlyph+" 窗内可消除量:无同尺持值行(详见背景/义务通道)")
		} else {
			lines = append(lines, tracefence.ElimGlyph+" eliminable in window: no same-ruler valued rows (see the background / obligation channels)")
		}
		return runtimeTraceProjElimClose(lines)
	}
	// §29.61.12 ② (INV-SUPPLY 件④): the bar ruler is the SECTION-WIDE maximum
	// over the whole board (满格尺保持全区最大值) — under causal-tier blocking
	// the list head is the largest ⛓ value, but a ◇ remainder seat (or the
	// fallback row) may be numerically larger; short chain bars against the
	// global ruler are the honest rendering (链上条短=诚实).
	topValue := 0.0
	for i := range board {
		if v := board[i].row.Node.EffectiveImpactMS; v > topValue {
			topValue = v
		}
	}
	leadWidth := 0
	if fallback != nil {
		leadWidth = runewidth.StringWidth(runtimeTraceProjElimFallbackLead(zh))
	}
	appendMember := func(entry runtimeTraceProjElimEntry, lead string) {
		lines = append(lines, runtimeTraceProjElimRowLine(entry, topValue, lead, leadWidth, model.Marks, zh))
		// INV-SUPPLY 件③: the compound seat's eliminable-composition leverage
		// note rides directly under its own row (adjacency binds it).
		if note, ok := runtimeTraceProjElimCompositionNoteLine(entry.row, leadWidth, model.Marks, zh); ok {
			lines = append(lines, note)
		}
	}
	for _, entry := range top {
		appendMember(entry, "")
	}
	if fallback != nil {
		appendMember(*fallback, runtimeTraceProjElimFallbackLead(zh))
	}
	if !chainPresent {
		lines = append(lines, runtimeTraceProjElimEmptyChainLine(model, zh))
	}
	lines = append(lines, runtimeTraceProjElimFootnotes(model, board, zh)...)
	return runtimeTraceProjElimClose(lines)
}

func runtimeTraceProjElimClose(lines []string) string {
	return tracefence.ElimOpener + "\n" + strings.Join(lines, "\n") + "\n```"
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

// runtimeTraceProjElimFootnotes renders the mention-obligation tail (design
// §2.5 排除脚注 + ▒/◇ pointer lines — 排除≠消失, absence never guesses):
//
//   - ⌗ caliber-side rows (V2-P0 协同): each excluded valued row is named
//     with its shared-registry value-class word (capped, counted overflow);
//   - target-self wait-symptom rows: one counted pointer (症状面, the rows
//     live in the target stanza + detail blocks);
//   - ◇ O-5 pointer: the adjacent stanza holds valued NON-population rows
//     (✦-form / critical_blocking — not rank items) and the channel fielded
//     no member: point at the largest instead of inventing a member;
//   - ▒ pointer: background rows exist — name the count and the caliber
//     boundary (基石 C), never a value comparison.
func runtimeTraceProjElimFootnotes(model runtimeTraceProjTreeModel, board []runtimeTraceProjElimEntry, zh bool) []string {
	var lines []string
	const caliberCap = 3
	caliberListed := 0
	caliberTotal := 0
	var caliberParts []string
	selfCount := 0
	var selfTags []string
	adjacentEligible := false
	for i := range board {
		if board[i].channelRank == 1 {
			adjacentEligible = true
		}
	}
	var adjacentPointer *runtimeTraceProjTreeRow
	scan := func(rows []runtimeTraceProjTreeRow) {
		for i := range rows {
			row := rows[i]
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
			caliberSide := row.Node.IsCaliberSideRow() ||
				tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken)) != tracequery.CausalCaliberSideNone
			if caliberSide && runtimeTraceProjElimRankItemRow(row) {
				switch runtimeTraceProjRowOrdinalChannel(row) {
				case runtimeTraceProjOrdinalChannelChain, runtimeTraceProjOrdinalChannelAdjacent:
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
					// footnote is caliber-side (count / composite score) —
					// its magnitude is NOT wall-clock ms, so the value
					// renders suffix-free; the adjacent ⌗ word carries the
					// class and the 非墙钟 qualifier.
					part := runtimeTraceProjElimSubject(row, zh) + " " + fmt.Sprintf("%.3f", value) +
						"·" + runtimeTraceProjCaliberSideWord(row.Node, zh)
					if tag := strings.TrimSpace(row.EvidenceTag); tag != "" {
						part += " [" + tag + "]"
					}
					caliberParts = append(caliberParts, part)
				}
			}
		}
	}
	scan(model.SelfRows)
	scan(model.TreeRows)
	scan(model.Adjacent)
	scan(model.Background)
	if !adjacentEligible {
		for i := range model.Adjacent {
			row := &model.Adjacent[i]
			if !row.HasData || row.Node.IsTargetSelfStateRow() || runtimeTraceProjElimRankItemRow(*row) {
				continue
			}
			if row.Node.IsCaliberSideRow() ||
				tracequery.CausalTokenCaliberSideClass(runtimeTraceCausalProjectionCanonicalNode(row.Node.TypeToken)) != tracequery.CausalCaliberSideNone {
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
	if len(caliberParts) > 0 {
		list := strings.Join(caliberParts, runtimeTraceProjElimJoinSep(zh))
		if rest := caliberTotal - caliberListed; rest > 0 {
			if zh {
				list += fmt.Sprintf("%s等共%d行", runtimeTraceProjElimJoinSep(zh), caliberTotal)
			} else {
				list += fmt.Sprintf("%s %d total", runtimeTraceProjElimJoinSep(zh), caliberTotal)
			}
		}
		if zh {
			lines = append(lines, "· 不参与汇排(口径):"+list)
		} else {
			lines = append(lines, "· not ranked here (caliber): "+list)
		}
	}
	if selfCount > 0 {
		tagList := ""
		if len(selfTags) > 0 {
			tagList = " " + strings.Join(selfTags, runtimeTraceProjElimJoinSep(zh))
		}
		if zh {
			lines = append(lines, fmt.Sprintf("· 不参与汇排:自身等待症状行 %d 行(症状面,非可消除量)见关注线程区%s", selfCount, tagList))
		} else {
			lines = append(lines, fmt.Sprintf("· not ranked here: %d target-self wait-symptom row(s) (symptom face, not an eliminable amount) — see the target stanza%s", selfCount, tagList))
		}
	}
	if adjacentPointer != nil {
		value := runtimeTraceProjNodeDisplayImpact(adjacentPointer.Node)
		tag := strings.TrimSpace(adjacentPointer.EvidenceTag)
		if tag != "" {
			tag = " [" + tag + "]"
		}
		if zh {
			lines = append(lines, fmt.Sprintf("· ◇ 邻近段最大持值行 %.3fms 见邻近段%s(不在根因排序种群,不参与汇排)", value, tag))
		} else {
			lines = append(lines, fmt.Sprintf("· ◇ largest valued adjacent-stanza row %.3fms — see the adjacent stanza%s (outside the rank population, not ranked here)", value, tag))
		}
	}
	background := 0
	for _, row := range model.Background {
		if row.HasData {
			background++
		}
	}
	if background > 0 {
		if zh {
			lines = append(lines, fmt.Sprintf("· ▒ 背景压力 %d 行(跨线程/他线程口径,不计入链上归因)见背景段", background))
		} else {
			lines = append(lines, fmt.Sprintf("· ▒ %d background-pressure row(s) (cross-thread / other-thread calibers, never counted into the chain attribution) — see the background stanza", background))
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
