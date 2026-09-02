package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

// Rank-0 rows are lossless side-lane disclosures, not election candidates.
// They must remain visible without consuming the root_cause_rank candidate
// budget. A separate small cap bounds their wire cost. When both typed lanes
// are populated, split the seats evenly so a flood of target symptoms cannot
// hide every data-gap disclosure (or vice versa).
const rootCauseRankZeroSeatDisclosureCap = 4
const rootCauseRankZeroSeatPerLaneReservedCap = rootCauseRankZeroSeatDisclosureCap / 2

// rootCauseRankRemainderSideCap bounds the RSPA ◇ remainder sub-lane
// (§29.61.10): a remainder seat is the complement half of an on-board ⛓
// anchored account — the same-source bipartition (identity/relation sentence)
// needs both halves visible, so remainder seats ride the side lane instead of
// competing for candidate seats (they'd otherwise sort behind every on-chain
// row and silently truncate — 零静默消失). Bounded by the migrated-thread
// count in practice; the cap is a defensive wire bound (overflow stays
// disclosed through the side-row caveat counts).
const rootCauseRankRemainderSideCap = 8

// rootCauseRankDemotedSideCap bounds the RNB-1 R4 lane-demoted sub-lane
// (§29.88 R4 / D1 修复轮, 2026-07-14): a credential-lane-demoted seat
// (affinity satellite / inversion-retyped seat / unprovable low_frequency
// satellite) is ◇-family like the remainder seats — as a plain candidate it
// sorts behind every on-chain row and structurally dies at the candidate cap,
// which made the「值零动,通道位归位」promise false on the WIRE (donghu 2955
// witness: all three demoted rows 47.678/22.408/16.013 vanished). Mirrored
// cap, same overflow disclosure through the side-row caveat counts.
const rootCauseRankDemotedSideCap = rootCauseRankRemainderSideCap

// B829 minted this side lane for "relation-only" semantic work (effective=0).
// CROWNSEM-1 (§40.28 ①, 2026-09-02) restored R3/R4 pricing, so a credentialed
// host_wakeup_edge_pre_span seat now competes on the board by its pre-edge
// share; the side lane remains for the residual effective==0 semantic forms
// (rootCauseItemIsRelationOnlySemantic) so they never evict target-state/
// data-gap disclosures from their older shared cap.
const rootCauseRankRelationSemanticSideCap = 8

// rootCauseRankSelfSideCap bounds the ELIM-SELF-FIX 件2 selfSide sub-lane
// (§29.93.1 Form-2 + §29.93.3 全族收编, 2026-07-15): in a degenerate window
// (no cross-thread chain) the sort is channel-blind and the candidate cap
// can kill EVERY one of the target's own on-chain seats while background
// rows fill the board (tieba head-window witness: the self running /
// runnable / io_wait seats died at pre-truncation positions 18/20/24 behind
// 12 background rows) — the RNB-1 D1 death shape, so the same bounded side
// lane protects them. ALL self families ride it (running / runnable / D-IO /
// IO facets / semantic — the predicate is subject+channel+eff, never a
// token list).
// EVOLUTION RECORD (终判① §29.96.2, 2026-07-15): cap 4 → 6. The §29.93.1
// cap 4 (照抄 D1 模式) was in tension with the §29.93.3 全族收编 zero-
// silent-disappearance promise: the self lane serves ≥5 families (four
// state families + semantic), so a full-family overflow at cap 4 silently
// dropped the 5th/6th seat. 6 = four state families + semantic + 1 headroom
// (a family may hold two seats, e.g. the D-state/io_wait pair).
const rootCauseRankSelfSideCap = 6

func rootEvidenceRankSeatKey(root RootEvidence) string {
	return fmt.Sprintf("%s|%s|%d|%d|%.9f", strings.TrimSpace(root.Type), threadKey(root.Thread), root.LineStart, root.LineEnd, root.DurationMs)
}

// rootEvidenceDStateTwinFamilyKey is the mutation-invariant seat key for the
// d_state/io_wait RootEvidence family (ENG audit #65, §29.25 处置委托
// 2026-07-10). expandChain mutates the D-state twin IN PLACE after
// construction when a sched_blocked_reason resolves (Type→"io_wait" iff
// reason.IOWait>0, LineEnd→reason.Line whenever the reason row is found), so
// the exact rootEvidenceRankSeatKey diverges from the seed minted off the
// unmutated constructor output and the same physical occurrence seated twice.
// Thread, LineStart and DurationMs are never touched by that mutation and
// identify the occurrence; the family fold applies only to the two D-state
// family type tokens, so no other RootEvidence lane can be over-suppressed.
// The "dstate_twin" prefix cannot collide with rootEvidenceRankSeatKey (no
// type token spells it and the field counts differ).
func rootEvidenceDStateTwinFamilyKey(root RootEvidence) (string, bool) {
	switch strings.TrimSpace(root.Type) {
	case "d_state_or_io_wait", "io_wait":
		return fmt.Sprintf("dstate_twin|%s|%d|%.9f", threadKey(root.Thread), root.LineStart, root.DurationMs), true
	}
	return "", false
}

// dropZeroTraceGapRankRowsForValuedThreads is the CHAIN-BUDGET 返工 P1-2①
// 同窗去重 rank half (2026-07-18): a zero-value trace_gap rank row claims a
// data blind spot for its thread inside the selected board window, but when
// the SAME thread also holds a valued rank row (published effective > 0) on
// the SAME board, the window-scope claim is self-contradictory — the thread
// demonstrably has data here; the "gap" is only a sub-window artifact of one
// chain-node expansion. Such rows stop minting a rank seat (the wakeup_chain
// view keeps its per-build RootEvidence disclosure lossless — same
// view-lane-untouched shape as the rootEvidenceStateOwnedByWindowStats
// suppression). Threads with NO valued row keep their gap disclosure
// byte-identically (G2: real blind spots stay published). Typed precise
// signals only: the mint-time type token, all-zero published value channels,
// and a same-threadKey effective comparison.
func dropZeroTraceGapRankRowsForValuedThreads(items []RootCauseRankItem) []RootCauseRankItem {
	var valuedThreads map[string]bool
	for i := range items {
		if items[i].Type == "trace_gap" {
			continue
		}
		if rootCauseEffectiveImpactMs(items[i]) > 0 {
			if valuedThreads == nil {
				valuedThreads = map[string]bool{}
			}
			valuedThreads[threadKey(items[i].Thread)] = true
		}
	}
	if len(valuedThreads) == 0 {
		return items
	}
	out := make([]RootCauseRankItem, 0, len(items))
	for i := range items {
		if items[i].Type == "trace_gap" &&
			items[i].EffectiveImpactMs == 0 && items[i].CumulativeImpactMs == 0 &&
			valuedThreads[threadKey(items[i].Thread)] {
			continue
		}
		out = append(out, items[i])
	}
	if len(out) == len(items) {
		return items
	}
	return out
}

func rootCauseRankItemIsZeroSeatDisclosure(item RootCauseRankItem) bool {
	if item.Type == "trace_gap" {
		return true
	}
	if rootCauseEffectiveImpactMs(item) <= 0 {
		return true
	}
	// V2-P0 行级尺守卫 (design §6.1, 2026-07-12): a caliber-side row is
	// structurally Rank=0 (the ordinal guard in assignRootCauseRanksAndTiers
	// keys on the same shared registry arm), so it rides the bounded
	// side-disclosure lane instead of consuming a candidate board seat.
	if rootCauseOrdinalChannel(item) != rootCauseOrdinalChannelBackground &&
		CausalTokenCaliberSideClass(item.Type) != CausalCaliberSideNone {
		return true
	}
	return item.SubjectIsAnalysisTarget && rootCauseItemIsTargetWaitSymptomType(item)
}

// sideValueFirstSort is the P1-2② 帽内先保真值行 fill order for one zero-seat
// sub-lane: rows whose published effective value is zero sort after every
// valued row (stable — zero rows keep their relative order). With valueDesc
// the valued prefix additionally orders by published effective desc (the
// targetSide lane mixes channels, so its blind channel-first order is not a
// value order); without it the valued rows keep their incoming relative
// order (caliber lane order is pinned by existing projections).
func sideValueFirstSort(rows []RootCauseRankItem, valueDesc bool) {
	if len(rows) < 2 {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		vi, vj := rootCauseEffectiveImpactMs(rows[i]), rootCauseEffectiveImpactMs(rows[j])
		if (vi > 0) != (vj > 0) {
			return vi > 0
		}
		if valueDesc && vi != vj {
			return vi > vj
		}
		return false
	})
}

// capSurvivalLanePriority is the CAPFIX-1 件1 cross-lane cap-survival class
// (user ruling verbatim 2026-07-19, §29.150①: 「一定要确保 优先保证 折算后提升
// 最大的 链上的 TOP 内容不丢失,非链上的 和背景 次之,尽量减少噪音影响。请按最优
// 方案推荐实施」). It DELEGATES to the same typed single source the ordinal
// allocator / display chip / stanza faces read (rootCauseOrdinalChannel —
// single source, now four faces), never a second lane implementation:
// ⛓ chain (incl. the self tokens and the chainless fail-open universe) → 0,
// ◇ adjacent → 1, ▒ background → 2. Zero-value/disclosure rows never reach
// the candidate pool at all (rootCauseRankItemIsZeroSeatDisclosure routes
// them to the bounded side lanes — 件3 零值恒末 is structural, pinned).
//
// XLANE-3 board-identity note (件1 硬纪律1): this survival key is NOT a query
// parameter — the board-params fingerprint closed set stays untouched
// (rootCauseBoardParamsFingerprint; the cap SIZE knob `limit` is already a
// member). The selection is a deterministic function of the same sorted pool
// the params identify, so two boards with equal fingerprints still publish
// one survivor set and no new-key-new-board split arises.
func capSurvivalLanePriority(item RootCauseRankItem) int {
	switch rootCauseOrdinalChannel(item) {
	case rootCauseOrdinalChannelChain:
		return 0
	case rootCauseOrdinalChannelAdjacent:
		return 1
	default:
		return 2
	}
}

// selectRootCauseRankCapSurvivors picks which candidates survive the board
// cap by the CAPFIX-1 cross-lane lexicographic key — ① on-chain valued seats
// (published eff desc) always first (invariant: an on-chain valued seat never
// dies to the cap while any non-chain/background row holds a candidate slot),
// ② ◇ adjacent valued seats (eff desc), ③ ▒ background valued seats
// (eff desc); within equal (lane, eff) the incoming board position decides
// (stable). The cap ONLY selects who appears — survivors are re-emitted in
// their ORIGINAL board order, so 榜序数 (根因排序#N / 邻近影响#N) and every
// in-lane sort key are untouched (排序语义零动).
//
// EVOLUTION RECORD (CAPFIX-1, 2026-07-19; replaces
// truncateRootCauseRankItemsStrict): the strict prefix cut inherited the
// survival order from the board sort — correct while the chain-aware sort is
// relevance-first, but the invariant lived in an implicit coupling (the
// ELIM-SELF-FIX degenerate-window death shape was exactly this coupling
// breaking on a channel-blind sort). On every currently-reachable stamped
// board this selection is survivor-identical to the old prefix (pinned by
// TestCapfix1ProductionSortPrefixEquivalence); the semantic-work displacement
// discipline is unchanged — semantic rows compete by the same published
// effective value on their lane, and a below-the-cut semantic row is still
// the independent semantic-optimization disclosure channel's job.
func selectRootCauseRankCapSurvivors(candidates []RootCauseRankItem, limit int) (survivors, capDead []RootCauseRankItem) {
	if limit <= 0 || len(candidates) <= limit {
		return candidates, nil
	}
	order := make([]int, len(candidates))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool {
		ia, ib := order[a], order[b]
		pa, pb := capSurvivalLanePriority(candidates[ia]), capSurvivalLanePriority(candidates[ib])
		if pa != pb {
			return pa < pb
		}
		va, vb := rootCauseEffectiveImpactMs(candidates[ia]), rootCauseEffectiveImpactMs(candidates[ib])
		if va != vb {
			return va > vb
		}
		return ia < ib
	})
	keep := make([]bool, len(candidates))
	for _, idx := range order[:limit] {
		keep[idx] = true
	}
	survivors = make([]RootCauseRankItem, 0, limit)
	capDead = make([]RootCauseRankItem, 0, len(candidates)-limit)
	for i := range candidates {
		if keep[i] {
			survivors = append(survivors, candidates[i])
		} else {
			capDead = append(capDead, candidates[i])
		}
	}
	return survivors, capDead
}

// rootCauseRankCapDeathLaneWords is the CAPFIX-1 件2 zh/EN lane word-face
// single point for the residual cap-death disclosure — both language faces
// live HERE and nowhere else, so they cannot drift apart.
func rootCauseRankCapDeathLaneWords(priority int) (en, zh string) {
	switch priority {
	case 0:
		return "on_chain", "链上"
	case 1:
		return "adjacent", "邻近"
	default:
		return "background", "背景"
	}
}

// rootCauseRankCapDeathSubject names the disclosed row: the thread label, or
// the typed metric token for aggregate-metric rows with no thread subject
// (§7.30 discipline — the subject IS the metric there, never "unknown").
func rootCauseRankCapDeathSubject(item RootCauseRankItem) string {
	if item.Thread.PID <= 0 && strings.TrimSpace(item.Thread.Comm) == "" {
		return item.Type
	}
	return threadLabel(item.Thread)
}

// rootCauseRankCapDeathValueClause is the CAPFIX-1 件2 residual cap-death
// value disclosure (R-2 席死于帽=披露道 precedent extended per §29.150①): the
// rows still dying at the cap AFTER the cross-lane survival priority — e.g.
// the 13th on-chain seat when all 12 slots hold higher-eff on-chain seats —
// upgrade the compaction caveat to a value+subject form with honest per-lane
// counts. The headline names the overall largest cap-dead row (published eff;
// strict > keeps the EARLIEST board position on ties — 平手取板序靠前,
// pinned); POOL2-1 件④ (§29.160④ user ruling 2026-07-20 「每道 top-2 点名」):
// the headline's lane additionally names its SECOND-largest casualty (「、次大
// <metric> <主体> <eff>ms」 zh / ", second largest …" EN — same lane, so no
// lane word repeats; a lane with only ONE cap-dead row adds no 次大;
// donghu2955 acceptance: 81.616 AND 76.800 both named). When an on-chain row
// died and is not already the headline, the largest on-chain casualty is
// additionally named so chain TOP content never loses its value disclosure
// (the ruling's chain-first concern) — that supplement stays top-1 (§29.160④
// 链上最大补充句不变). Both language faces carry the metric token (最大
// <metric> <主体> <eff>ms(道) — 冷读 P3-1 zh/EN 度量词对齐). selfPublished
// is the count of cap-displaced target self seats the bounded selfSide lane
// preserved (published wire forms — carved OUT of the 未入发布面 counts, 冷读
// P3-2): disclosed as a short note so the compacted X→Y arithmetic stays
// explainable even when every cap-dead row was self-preserved. Returned as a
// clause appended to the existing "compacted from X to Y" sentence — the
// Total>Emitted counts, the typed ViewCompaction record and every
// twin-visibility disclosure stay verbatim. Empty when nothing died at the
// cap and nothing was self-preserved.
func rootCauseRankCapDeathValueClause(capDead []RootCauseRankItem, selfPublished int) string {
	selfNote := ""
	if selfPublished > 0 {
		selfNote = fmt.Sprintf("; %d cap-displaced target self seat(s) published on the self side lane (另 %d 行自身侧道已发布)", selfPublished, selfPublished)
	}
	if len(capDead) == 0 {
		return selfNote
	}
	var laneCounts [3]int
	largest, largestChain := -1, -1
	for i := range capDead {
		priority := capSurvivalLanePriority(capDead[i])
		laneCounts[priority]++
		value := rootCauseEffectiveImpactMs(capDead[i])
		if largest < 0 || value > rootCauseEffectiveImpactMs(capDead[largest]) {
			largest = i
		}
		if priority == 0 && (largestChain < 0 || value > rootCauseEffectiveImpactMs(capDead[largestChain])) {
			largestChain = i
		}
	}
	headLane := capSurvivalLanePriority(capDead[largest])
	// POOL2-1 件④: the second-largest casualty WITHIN the headline's lane
	// (same strict > tie rule — 平手取板序靠前; -1 when the lane holds only
	// the headline row).
	second := -1
	for i := range capDead {
		if i == largest || capSurvivalLanePriority(capDead[i]) != headLane {
			continue
		}
		if second < 0 || rootCauseEffectiveImpactMs(capDead[i]) > rootCauseEffectiveImpactMs(capDead[second]) {
			second = i
		}
	}
	headEN, headZH := rootCauseRankCapDeathLaneWords(headLane)
	clause := fmt.Sprintf("; %d valued candidate row(s) did not enter the published board (on_chain=%d/adjacent=%d/background=%d), largest %s %s %.3fms (%s)",
		len(capDead), laneCounts[0], laneCounts[1], laneCounts[2],
		capDead[largest].Type, rootCauseRankCapDeathSubject(capDead[largest]),
		rootCauseEffectiveImpactMs(capDead[largest]), headEN)
	secondZH := ""
	if second >= 0 {
		clause += fmt.Sprintf(", second largest %s %s %.3fms",
			capDead[second].Type, rootCauseRankCapDeathSubject(capDead[second]),
			rootCauseEffectiveImpactMs(capDead[second]))
		secondZH = fmt.Sprintf("、次大 %s %s %.3fms",
			capDead[second].Type, rootCauseRankCapDeathSubject(capDead[second]),
			rootCauseEffectiveImpactMs(capDead[second]))
	}
	chainEN := ""
	chainZH := ""
	if largestChain >= 0 && largestChain != largest {
		chainEN = fmt.Sprintf(", largest on_chain %s %s %.3fms",
			capDead[largestChain].Type, rootCauseRankCapDeathSubject(capDead[largestChain]),
			rootCauseEffectiveImpactMs(capDead[largestChain]))
		chainZH = fmt.Sprintf(";链上最大 %s %s %.3fms",
			capDead[largestChain].Type,
			rootCauseRankCapDeathSubject(capDead[largestChain]),
			rootCauseEffectiveImpactMs(capDead[largestChain]))
	}
	clause += chainEN
	clause += fmt.Sprintf(" (另有 %d 行未入发布面(链上 %d/邻近 %d/背景 %d),最大 %s %s %.3fms(%s)%s%s)",
		len(capDead), laneCounts[0], laneCounts[1], laneCounts[2],
		capDead[largest].Type,
		rootCauseRankCapDeathSubject(capDead[largest]),
		rootCauseEffectiveImpactMs(capDead[largest]), headZH, secondZH, chainZH)
	return clause + selfNote
}

// truncateRootCauseRankCandidatesAndSideRows applies `limit` only to rows that
// will receive a Rank>0 election seat. Rank-0 rows use an independent bounded
// disclosure lane and are appended after the sorted board; projection
// rendering places them in their typed self/data-gap sections, so this does
// not change candidate order or ordinals. capDead returns the valued
// candidates the cap killed AND left without any published wire form (board
// order) for the 件2 value disclosure; capDeadSelfPublished counts the
// cap-displaced target self seats the selfSide lane preserved instead
// (published — carved out of capDead, disclosed as the 冷读 P3-2 note).
func truncateRootCauseRankCandidatesAndSideRows(items []RootCauseRankItem, limit int) (out, capDead []RootCauseRankItem, capDeadSelfPublished, candidateTotal, candidateEmitted, sideTotal, sideEmitted int) {
	if len(items) == 0 {
		return nil, nil, 0, 0, 0, 0, 0
	}
	candidates := make([]RootCauseRankItem, 0, len(items))
	targetSide := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	gapSide := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	// V2-P0 (design §6.1, 2026-07-12): ⌗ caliber-side rows ride their OWN
	// bounded sub-lane — routing them into targetSide would crowd the
	// self-symptom/context disclosures out of the shared cap (donghu witness:
	// the pacing_idle context row evicted by two IO caliber rows).
	caliberSide := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	relationSemanticSide := make([]RootCauseRankItem, 0, rootCauseRankRelationSemanticSideCap)
	remainderSide := make([]RootCauseRankItem, 0, rootCauseRankRemainderSideCap)
	demotedSide := make([]RootCauseRankItem, 0, rootCauseRankDemotedSideCap)
	for _, item := range items {
		if item.ChainAnchorRemainderSeat {
			// RSPA ◇ remainder seats: complement halves of on-board anchored
			// accounts — side lane, never a candidate seat (see cap comment).
			sideTotal++
			remainderSide = append(remainderSide, item)
			continue
		}
		if item.ChainCredentialLaneDemoted || item.ChainAnchorRepresentedByChainSeat ||
			item.GatedShareConstituentSeat {
			// RNB-1 R4 lane-demoted seats (D1 修复轮): ◇-family disclosure
			// rows — mirrored side lane so「值零动,通道位归位」holds on the
			// published wire, never a candidate seat. XLANE-1 件1: the
			// represented-demoted satellite rides the same side lane (same
			// value-untouched ◇ disclosure family, different credential story).
			// LEVELMERGE-1 件2: the gated-share CONSTITUENT row (the A share
			// of the interval-accounting split) is ◇-family too — its value
			// is already carried by the inversion seat's gated composite, so
			// it discloses on the side lane and never consumes a candidate
			// seat (the residual B row keeps the competing aggregate seat).
			sideTotal++
			demotedSide = append(demotedSide, item)
			continue
		}
		if rootCauseRankItemIsZeroSeatDisclosure(item) {
			sideTotal++
			if item.Type == "trace_gap" {
				gapSide = append(gapSide, item)
			} else if rootCauseItemIsRelationOnlySemantic(item) {
				relationSemanticSide = append(relationSemanticSide, item)
			} else if rootCauseOrdinalChannel(item) != rootCauseOrdinalChannelBackground &&
				CausalTokenCaliberSideClass(item.Type) != CausalCaliberSideNone {
				caliberSide = append(caliberSide, item)
			} else {
				targetSide = append(targetSide, item)
			}
			continue
		}
		candidates = append(candidates, item)
	}
	candidateTotal = len(candidates)
	// ELIM-SELF-FIX 件2 selfSide (§29.93.1 修向② + §29.93.3 全族收编): the
	// analysis target's own on-chain seats that die at the candidate cap are
	// preserved on a bounded side lane instead of vanishing (零静默消失的自身
	// 特化). Election candidates that WIN a board seat are untouched — the
	// lane holds only the strict-truncation overflow, in sorted order.
	var selfSide []RootCauseRankItem
	candidates, capDead = selectRootCauseRankCapSurvivors(candidates, limit)
	if len(capDead) > 0 {
		// CAPFIX-1 件2 honesty: capDead discloses only rows with NO published
		// wire form — a cap-dead target self seat the bounded selfSide lane
		// preserves (§29.93.3, published with its chain ordinal) is NOT
		// "未入发布面" and leaves the disclosure count; selfSide overflow
		// beyond the lane cap stays counted (genuinely unpublished).
		silent := capDead[:0:0]
		for _, item := range capDead {
			if item.SubjectIsAnalysisTarget && rootCauseItemIsOnChain(item) &&
				rootCauseEffectiveImpactMs(item) > 0 {
				sideTotal++
				if len(selfSide) < rootCauseRankSelfSideCap {
					selfSide = append(selfSide, item)
					continue
				}
			}
			silent = append(silent, item)
		}
		capDead = silent
	}
	candidateEmitted = len(candidates)
	// CHAIN-BUDGET 返工 P1-2② 侧道排序降位 (2026-07-18): the bounded zero-seat
	// disclosure caps fill TRUE-VALUE rows first (帽内先保真值行) — the shared
	// board sort is channel-first and therefore value-blind inside this lane,
	// so a small newly-minted disclosure used to evict a much larger one
	// (donghu 17267: the new binder_wait 0.924 self row killed the
	// pacing_idle 15.758 context row at the halved cap). targetSide orders by
	// the published effective value (desc; zero-value rows keep their
	// original relative order after every valued row — the precise
	// zero-vs-valued signal, never a heuristic). gapSide/caliberSide only
	// stable-partition zero-value rows behind valued ones: gap rows are
	// all-zero by construction (order preserved) and the caliber lane's
	// valued ordering is pinned by existing projections, so no valued-row
	// reorder happens there.
	sideValueFirstSort(targetSide, true)
	sideValueFirstSort(gapSide, false)
	sideValueFirstSort(caliberSide, false)
	side := make([]RootCauseRankItem, 0, rootCauseRankZeroSeatDisclosureCap)
	if len(targetSide) > 0 && len(gapSide) > 0 {
		targetLimit := min(rootCauseRankZeroSeatPerLaneReservedCap, len(targetSide))
		gapLimit := min(rootCauseRankZeroSeatPerLaneReservedCap, len(gapSide))
		side = append(side, targetSide[:targetLimit]...)
		side = append(side, gapSide[:gapLimit]...)
	} else if len(targetSide) > 0 {
		targetLimit := min(rootCauseRankZeroSeatDisclosureCap, len(targetSide))
		side = append(side, targetSide[:targetLimit]...)
	} else {
		gapLimit := min(rootCauseRankZeroSeatDisclosureCap, len(gapSide))
		side = append(side, gapSide[:gapLimit]...)
	}
	if len(caliberSide) > 0 {
		caliberLimit := min(rootCauseRankZeroSeatDisclosureCap, len(caliberSide))
		side = append(side, caliberSide[:caliberLimit]...)
	}
	if len(relationSemanticSide) > 0 {
		relationLimit := min(rootCauseRankRelationSemanticSideCap, len(relationSemanticSide))
		side = append(side, relationSemanticSide[:relationLimit]...)
	}
	if len(remainderSide) > 0 {
		remainderLimit := min(rootCauseRankRemainderSideCap, len(remainderSide))
		side = append(side, remainderSide[:remainderLimit]...)
	}
	if len(demotedSide) > 0 {
		demotedLimit := min(rootCauseRankDemotedSideCap, len(demotedSide))
		side = append(side, demotedSide[:demotedLimit]...)
	}
	// selfSide is already bounded at collection time (sorted-order prefix).
	side = append(side, selfSide...)
	sideEmitted = len(side)
	out = make([]RootCauseRankItem, 0, candidateEmitted+sideEmitted)
	out = append(out, candidates...)
	out = append(out, side...)
	return out, capDead, len(selfSide), candidateTotal, candidateEmitted, sideTotal, sideEmitted
}
