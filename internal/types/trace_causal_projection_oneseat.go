package types

// trace_causal_projection_oneseat.go — v5 P1 件① B.2 一段一席引擎铸造
// (causal_tree_v5_design_20260711.md B.2 / 备-1; 账本 real_trace_campaign
// §29.56 遗留头位, 2026-07-13).
//
// EVOLUTION RECORD (v3 D1 → CR-2 P5 → v5 P1 件①): v3 (trace projection
// presentation v3, 2026-07-02) promised 「每节点全章恰 2 次(树 1 行 + 明细
// 1 行)」. The census (§29.47.6 witness 14704 等) broke that promise with
// same-segment double seats across the candidate / raw-state / context
// lanes. CR-2 组② P5 (2026-07-12) built the DISPLAY-LAYER forerunner
// (runtimeTraceProjFoldSameSegmentContextMirrors — render-side convergence,
// engine wire untouched); its own EVOLUTION RECORD contracted that the
// engine-side one-seat mint retires that lane's work when it lands. THIS
// file is that landing: the convergence now happens in the ENGINE
// aggregation order, so every render face (tree fence, metric tables,
// evidence index, prose feeds) inherits one seat per physical segment —
// no display-layer dedup backstop.
//
// 勘正 (2026-07-13, 实测 — the F3 lesson executed): the batch map's premise
// "the engine misses these carriers entirely" was PARTIALLY wrong. At HEAD
// cd2ca239 the R1 same-fact merge already converges the VALUED equality
// twin (14704 archetype: candidate wakeup_causal_impact row + raw
// root_evidence copy, same subject + %.3f value + exact line span) —
// proven by a records-level probe with production-exact emission shapes.
// The REAL residual engine gaps, each carried by an arm below:
//   - arm A: raw twins R1 cannot key — valueless raw copies (R1's key
//     requires a positive Impact/Cumulative) and keepers whose value lives
//     on the Effective/Actual lanes only;
//   - arm B: raw member RE-ISSUES of an ×N merged seat (25846 witness:
//     three root_evidence rows re-publishing the three member segments of
//     one ×3 sleep seat — spans/values match MEMBERS, never the seat, so
//     R1/V4/R2 all structurally miss);
//   - arm C: post-R2 merged-twin seats — one physical segment SET
//     R2-merged independently on two lanes (WO-D3 witnesses 42729 E9/E15 ·
//     62930 E9/E19: 13.418 ×3 published twice, word faces forked purely by
//     typed-token completeness; S8-TPF 链/▒ 通道漂移 same-token pair).
//
// 红线 (all three arms):
//   - 假设永不并 / 精确信号硬折: every gate is a typed exact comparison
//     (µs/3dp value identity, verbatim line spans, typed query windows,
//     registered state words). Ambiguity (0 or ≥2 partners) fails open to
//     the honest multi-row render.
//   - W-A 双账并存不可推翻 (§29.50.3): diverging cumulative accounts and
//     diverging published effective calibers are TWO account systems —
//     arm C refuses those pairs (they stay two rows; the display-layer
//     account-relation/mirror-tag arms keep covering them).
//   - 墙钟不可加和: a fold NEVER re-sums. E# union only; MergedCount /
//     extrema / every published account stay byte-identical on the keeper
//     (never ×3+×3→×6 — the vnote family-key-merge prohibition).
//   - 分区席相容 (§29.50.5): proof-partition sibling seats (cause 席 +
//     余数席) carry DIFFERENT values/identities by construction — the value
//     gates exclude them; they are proof partitions of one segment set, not
//     duplicate seats.
//   - 诚实席不折: undrillable / missing_wakeup / trace_gap raw rows carry
//     the ⊘链止/◌ honesty semantics and never converge here (their ms
//     de-duplication stays the display pointer arms' business).
//   - 注册表单源: no causal-token lane table is copied into this package.
//     State-family agreement reads the types-registered scheduler-state
//     word universe (trace_state_kinds.go) plus verbatim token identity;
//     tokens outside both stay unprovable and fail open.

import (
	"fmt"
	"math"
	"strings"
)

// traceCausalProjectionRound3Equal is the 3-decimal display-value identity —
// the types-side twin of the display layer's runtimeTraceProjRound3Equal
// (one rounding convention, mirrored so the retired display arm and this
// engine mint can never disagree on the same pair).
func traceCausalProjectionRound3Equal(a, b float64) bool {
	return math.Round(a*1000) == math.Round(b*1000)
}

// traceCausalProjectionNodeDisplayValue mirrors the display layer's
// main-line value fallback chain (Impact → Cumulative → Effective → Actual)
// so the equality gates below compare what the reader would actually see.
func traceCausalProjectionNodeDisplayValue(node TraceCausalProjectionNode) float64 {
	if node.ImpactMS > 0 {
		return node.ImpactMS
	}
	if node.CumulativeImpactMS > 0 {
		return node.CumulativeImpactMS
	}
	if node.EffectiveImpactMS > 0 {
		return node.EffectiveImpactMS
	}
	if node.ActualImpactMS > 0 {
		return node.ActualImpactMS
	}
	return 0
}

// traceCausalProjectionRawEvidenceLane reports whether node is a raw-state
// root_evidence lane row eligible for one-seat convergence: the reduced-shape
// wakeup witness (typed lane identity = the system-minted observation ID),
// unmerged, unranked, with a bare state/type token on Predicate. Honesty
// seats (undrillable / missing_wakeup / trace_gap) never qualify — the
// ⊘链止/◌ face must stay.
func traceCausalProjectionRawEvidenceLane(node TraceCausalProjectionNode) bool {
	if !strings.Contains(node.EvidenceID, "root_evidence:") {
		return false
	}
	if node.MergedCount > 1 || node.OnChainOverflowFold || node.Rank != 0 {
		return false
	}
	predicate := traceCausalProjectionCanonicalNode(node.Predicate)
	if predicate == "" || predicate == "missing_wakeup" || predicate == "trace_gap" {
		return false
	}
	if node.Undrillable() || strings.TrimSpace(node.TraceGapKind) != "" {
		return false
	}
	return true
}

// traceCausalProjectionOneSeatWindowsCompatible is the SFD F1 cross-window
// veto shared by all three arms: two valid-but-different typed query windows
// are two measurements and never one seat; absence never vetoes.
func traceCausalProjectionOneSeatWindowsCompatible(a, b TraceCausalProjectionNode) bool {
	if !traceCausalProjectionIntervalValid(a.QueryWindowStartTs, a.QueryWindowEndTs) ||
		!traceCausalProjectionIntervalValid(b.QueryWindowStartTs, b.QueryWindowEndTs) {
		return true
	}
	return math.Abs(a.QueryWindowStartTs-b.QueryWindowStartTs) <= TraceCausalProjectionSameWindowToleranceS &&
		math.Abs(a.QueryWindowEndTs-b.QueryWindowEndTs) <= TraceCausalProjectionSameWindowToleranceS
}

// traceCausalProjectionOneSeatKeeperEligible gates the arm-A KEEPER role
// (修复轮 件1, 对抗复核 F1, 2026-07-13): the keeper-side mirror of the raw
// lane's honesty exclusions. Without it a ◌ missing_wakeup honesty seat
// (same subject + exact span as the raw witness beside it) could absorb a
// valueless raw copy and get a CONTRADICTORY scheduler-state word
// back-filled onto its wire StateKind — every render face and prose feed
// would inherit a chain-stop honesty seat wearing 「running」. Two legs,
// both precise:
//   - honesty seats never keep (Undrillable / typed TraceGapKind /
//     missing_wakeup / trace_gap predicate tokens — the ⊘链止/◌ face);
//   - 两把尺: a keeper carrying a typed NON-state token that resolves
//     neither a registered scheduler-state word nor a duration-state family
//     (⌗ composite/count calibers, foreign lanes) never absorbs a raw
//     wall-clock witness here — value coincidence across rulers must not
//     transplant E#/state words. Fail-open narrowing: such pairs simply
//     stay two rows (R1's own valued lane is untouched by this gate).
func traceCausalProjectionOneSeatKeeperEligible(node TraceCausalProjectionNode) bool {
	if node.MergedCount > 1 || node.OnChainOverflowFold {
		return false
	}
	if node.Undrillable() || strings.TrimSpace(node.TraceGapKind) != "" {
		return false
	}
	switch traceCausalProjectionCanonicalNode(node.Predicate) {
	case "missing_wakeup", "trace_gap":
		return false
	}
	if token := traceCausalProjectionCanonicalNode(node.TypeToken); token != "" {
		if traceCausalProjectionCanonicalStateWord(token) == "" &&
			traceCausalProjectionNodeStateFamily(node) == "" {
			return false
		}
	}
	return true
}

// traceCausalProjectionOneSeatTwinKey is the same-segment identity the
// equality arm keys on — the SFD-precedent twin key (canonical subject +
// exact engine line span), byte-isomorphic to the display layer's
// runtimeTraceProjSameSegmentTwinKey so the retirement diff is provable.
func traceCausalProjectionOneSeatTwinKey(node TraceCausalProjectionNode) string {
	if node.LineStart <= 0 || node.LineEnd < node.LineStart {
		return ""
	}
	if !traceCausalProjectionKnownSubject(node.Subject) {
		return ""
	}
	return traceCausalProjectionCanonicalNode(node.Subject) +
		"\x00" + fmt.Sprintf("%d\x00%d", node.LineStart, node.LineEnd)
}

// traceCausalProjectionOneSeatBuckets enumerates the projection's render
// buckets in R1 priority order — the shared scan order of all three arms.
func traceCausalProjectionOneSeatBuckets(out *TraceCausalProjection) []*[]TraceCausalProjectionNode {
	return []*[]TraceCausalProjectionNode{
		&out.PrimaryRootCauses,
		&out.OnChainCauses,
		&out.SupportingHops,
		&out.AdjacentCauses,
		&out.BackgroundCauses,
	}
}

// traceCausalProjectionOneSeatDropByEvidenceID removes EVERY bucket copy of
// the folded raw/twin row (a loser leaves all its seats; the keeper's own
// cross-bucket copies stay — renderers dedupe survivors by node key).
func traceCausalProjectionOneSeatDropByEvidenceID(out *TraceCausalProjection, id string) {
	key := traceCausalProjectionCanonicalNode(id)
	if key == "" {
		return
	}
	for _, bucket := range traceCausalProjectionOneSeatBuckets(out) {
		kept := (*bucket)[:0]
		for _, node := range *bucket {
			if traceCausalProjectionCanonicalNode(node.EvidenceID) == key {
				continue
			}
			kept = append(kept, node)
		}
		*bucket = kept
	}
}

// traceCausalProjectionOneSeatAbsorbEvidence unions the loser's evidence ids
// into EVERY bucket copy of the keeper (id-keyed), and back-fills the state
// word onto keepers that carry none (词位取最高车道: the raw word fills a
// gap, never overrides). stateWord must already be canonical-registered or
// empty. No account field ever moves here.
func traceCausalProjectionOneSeatAbsorbEvidence(out *TraceCausalProjection, keeperID string, loser TraceCausalProjectionNode, stateWord string) {
	keeperKey := traceCausalProjectionCanonicalNode(keeperID)
	if keeperKey == "" {
		return
	}
	for _, bucket := range traceCausalProjectionOneSeatBuckets(out) {
		for i := range *bucket {
			node := &(*bucket)[i]
			if traceCausalProjectionCanonicalNode(node.EvidenceID) != keeperKey {
				continue
			}
			absorbed := map[string]bool{keeperKey: true}
			for _, id := range node.MergedEvidenceIDs {
				absorbed[traceCausalProjectionCanonicalNode(id)] = true
			}
			appendID := func(raw string) {
				raw = strings.TrimSpace(raw)
				if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
					return
				}
				absorbed[traceCausalProjectionCanonicalNode(raw)] = true
				node.MergedEvidenceIDs = append(node.MergedEvidenceIDs, raw)
			}
			appendID(loser.EvidenceID)
			for _, id := range loser.MergedEvidenceIDs {
				appendID(id)
			}
			if node.StateKind == "" && stateWord != "" {
				node.StateKind = stateWord
			}
		}
	}
}

// --- Arm A: raw same-segment twin convergence (equality arm) -----------------

// traceCausalProjectionConvergeRawSegmentTwins folds a raw root_evidence copy
// of ONE segment into the richer row of the SAME fingerprint — the engine
// home of the CR-2 P5 equality arm (witness 14704 E1/E2). Fingerprint = the
// twin key (canonical subject + exact line span) PLUS display-value identity
// at 3 decimals — a raw copy whose display value differs is a different
// account and never folds (fail-open, 宁两行勿假并); a VALUELESS raw copy
// also folds (the shape R1's value-keyed merge structurally cannot reach).
// State-word consistency mirrors the display gate verbatim: a keeper with a
// state word only absorbs a raw whose Predicate token IS that word; a
// keeper without one adopts the raw's word only when it is a registered
// scheduler-state word (raw state 降入 [状态x] 槽, B.2).
func traceCausalProjectionConvergeRawSegmentTwins(out *TraceCausalProjection) {
	type group struct {
		rawIDs    map[string]bool
		keeperIDs map[string]bool
		raw       TraceCausalProjectionNode
		keeper    TraceCausalProjectionNode
	}
	groups := map[string]*group{}
	for _, bucket := range traceCausalProjectionOneSeatBuckets(out) {
		for _, node := range *bucket {
			key := traceCausalProjectionOneSeatTwinKey(node)
			if key == "" {
				continue
			}
			id := traceCausalProjectionCanonicalNode(node.EvidenceID)
			if id == "" {
				continue
			}
			g := groups[key]
			if g == nil {
				g = &group{rawIDs: map[string]bool{}, keeperIDs: map[string]bool{}}
				groups[key] = g
			}
			switch {
			case traceCausalProjectionRawEvidenceLane(node):
				if !g.rawIDs[id] {
					g.rawIDs[id] = true
					g.raw = node
				}
			// 件1 (F1): honesty seats (◌/⊘链止) and non-state caliber rows
			// are NEITHER raw nor keeper — they stay inert and the pair
			// renders honestly as two rows.
			case traceCausalProjectionOneSeatKeeperEligible(node):
				if !g.keeperIDs[id] {
					g.keeperIDs[id] = true
					g.keeper = node
				}
			}
		}
	}
	for _, g := range groups {
		// Exactly one raw and one keeper identity (distinct-ID count — a
		// keeper's own cross-bucket copies are ONE identity); anything else
		// is ambiguity and fails open (SFD donor-conflict rule).
		if len(g.rawIDs) != 1 || len(g.keeperIDs) != 1 {
			continue
		}
		raw, keeper := g.raw, g.keeper
		if traceCausalProjectionCanonicalNode(raw.EvidenceID) == traceCausalProjectionCanonicalNode(keeper.EvidenceID) {
			continue
		}
		// µs-value equality on the DISPLAY chain: the raw copy re-publishes
		// the keeper's own magnitude; valueless raw copies also fold.
		rawValue := traceCausalProjectionNodeDisplayValue(raw)
		if rawValue > 0 && !traceCausalProjectionRound3Equal(rawValue, traceCausalProjectionNodeDisplayValue(keeper)) {
			continue
		}
		// State-word consistency (display-gate verbatim mirror).
		if keeper.StateKind != "" && keeper.StateKind != strings.TrimSpace(raw.Predicate) {
			continue
		}
		if !traceCausalProjectionOneSeatWindowsCompatible(raw, keeper) {
			continue
		}
		stateWord := ""
		if keeper.StateKind == "" {
			stateWord = traceCausalProjectionCanonicalStateWord(strings.TrimSpace(raw.Predicate))
		}
		traceCausalProjectionOneSeatAbsorbEvidence(out, keeper.EvidenceID, raw, stateWord)
		traceCausalProjectionOneSeatDropByEvidenceID(out, raw.EvidenceID)
	}
}

// --- Arm B: raw member re-issue convergence ----------------------------------

// traceCausalProjectionStateWordFamily maps a REGISTERED scheduler-state word
// to its duration family (sleep 族 / d·io 族 / runnable 族). This reads the
// types-registered state-word universe only — never a causal-token lane copy
// (注册表单源: token lanes stay in tracequery's registry; tokens that are
// not registered state words resolve through verbatim token identity or not
// at all).
func traceCausalProjectionStateWordFamily(word string) string {
	switch word {
	case TraceStateKindRunnable:
		return "runnable"
	case TraceStateKindSleep, TraceStateKindSSleep, TraceStateKindSleepWait:
		return "sleep"
	case TraceStateKindDSleep, TraceStateKindDState, TraceStateKindIOWait, TraceStateKindUninterruptibleSleep:
		return "dio"
	}
	return ""
}

// traceCausalProjectionNodeStateFamily resolves a node's duration-state
// family from its typed word lanes (StateKind first — the row identity —
// then the registered-word views of TypeToken / Object / Predicate).
func traceCausalProjectionNodeStateFamily(node TraceCausalProjectionNode) string {
	for _, word := range []string{
		node.StateKind,
		traceCausalProjectionCanonicalStateWord(strings.TrimSpace(node.TypeToken)),
		traceCausalProjectionCanonicalStateWord(strings.TrimSpace(node.Object)),
		traceCausalProjectionCanonicalStateWord(strings.TrimSpace(node.Predicate)),
	} {
		if family := traceCausalProjectionStateWordFamily(strings.ToLower(strings.TrimSpace(word))); family != "" {
			return family
		}
	}
	return ""
}

// traceCausalProjectionOneSeatFamilyAgree reports whether a raw re-issue and
// an ×N keeper provably belong to one duration family: registered
// state-word families agree, OR the raw's Predicate token IS the keeper's
// own type/cause token verbatim (same-token lane — covers legacy
// d_state_or_io_wait shapes without a registry copy). Unprovable → false.
func traceCausalProjectionOneSeatFamilyAgree(raw, keeper TraceCausalProjectionNode) bool {
	rawFamily := traceCausalProjectionNodeStateFamily(raw)
	if rawFamily != "" && rawFamily == traceCausalProjectionNodeStateFamily(keeper) {
		return true
	}
	predicate := traceCausalProjectionCanonicalNode(raw.Predicate)
	if predicate == "" {
		return false
	}
	return predicate == traceCausalProjectionCanonicalNode(keeper.TypeToken) ||
		predicate == traceCausalProjectionCanonicalNode(keeper.Object)
}

// traceCausalProjectionMergedMemberValues derives an ×N row's member value
// multiset where LOSSLESSLY derivable from the typed extrema/sum (the
// display layer's runtimeTraceProjSMR1MergedMemberValues, mirrored): count 2
// = {min,max}; count 3 with a clean positive Σ = {min, Σ−min−max, max};
// larger counts expose only the extrema (middle members underdetermined —
// fail-open, 宁漏勿猜). Valueless members kill the middle derivation.
func traceCausalProjectionMergedMemberValues(node TraceCausalProjectionNode) []float64 {
	if node.MergedCount < 2 || node.MergedMinMS <= 0 || node.MergedMaxMS < node.MergedMinMS {
		return nil
	}
	if node.MergedCount == 2 {
		return []float64{node.MergedMinMS, node.MergedMaxMS}
	}
	if node.MergedCount == 3 && node.MergedValuelessCount == 0 {
		sum := node.MergedSumMS
		if sum <= 0 && !node.MergedIntervalUnion && !node.MergedCrossWindowMax {
			sum = node.ImpactMS
		}
		if sum > 0 {
			middle := sum - node.MergedMinMS - node.MergedMaxMS
			if middle > 0 && middle >= node.MergedMinMS-TraceCausalProjectionSameValueTieMS &&
				middle <= node.MergedMaxMS+TraceCausalProjectionSameValueTieMS {
				return []float64{node.MergedMinMS, middle, node.MergedMaxMS}
			}
		}
	}
	return []float64{node.MergedMinMS, node.MergedMaxMS}
}

// traceCausalProjectionOneSeatValuesEqual is the µs-equality band shared with
// the display judgment lane (TraceCausalProjectionSameValueTieMS — the ONE
// exported DIAG A1 constant, never a new tolerance).
func traceCausalProjectionOneSeatValuesEqual(a, b float64) bool {
	return a > 0 && b > 0 && math.Abs(a-b) < TraceCausalProjectionSameValueTieMS
}

// traceCausalProjectionLineWithinEnvelope — the P2-3 necessity veto,
// mirrored: membership claims additionally require the inner row's line
// interval to sit INSIDE the outer row's envelope. NEGATIVE gate only
// (行号包络连通判禁令: containment never PROVES membership); missing
// intervals never veto.
func traceCausalProjectionLineWithinEnvelope(inner, outer TraceCausalProjectionNode) bool {
	if inner.LineStart <= 0 || inner.LineEnd < inner.LineStart ||
		outer.LineStart <= 0 || outer.LineEnd < outer.LineStart {
		return true
	}
	return inner.LineStart >= outer.LineStart && inner.LineEnd <= outer.LineEnd
}

// traceCausalProjectionConvergeRawMemberReissues folds a raw root_evidence
// re-issue of ONE member segment into the ×N seat that already counts it —
// the engine home of the CR-2 P5 member arm / WO-D1① (witness 25846: E1
// 105.794 ×3 beside raw re-issues 80.751 / 16.164 / 8.879 — a five-seat
// table summing to 292.3ms vs 真值 105.8). Membership = µs identity against
// a losslessly derivable member value (成员盘存隶属 by µs identity — 禁行号
// 包络连通判; the envelope is a NECESSITY veto only) + duration-family
// agreement + window compatibility. E# joins the bracket; MergedCount and
// every account stay byte-identical (吸收=佩记号,数值不重计). Runs AFTER R2
// so every ×N keeper shape (wire fold, R2 merge, marker fold) is final.
func traceCausalProjectionConvergeRawMemberReissues(out *TraceCausalProjection) {
	type keeperRef struct {
		node TraceCausalProjectionNode
	}
	// Collect distinct raw identities and distinct merged keepers.
	raws := map[string]TraceCausalProjectionNode{}
	var rawOrder []string
	keepers := map[string]TraceCausalProjectionNode{}
	var keeperOrder []string
	for _, bucket := range traceCausalProjectionOneSeatBuckets(out) {
		for _, node := range *bucket {
			id := traceCausalProjectionCanonicalNode(node.EvidenceID)
			if id == "" {
				continue
			}
			if traceCausalProjectionRawEvidenceLane(node) {
				if _, ok := raws[id]; !ok && traceCausalProjectionKnownSubject(node.Subject) {
					raws[id] = node
					rawOrder = append(rawOrder, id)
				}
				continue
			}
			if node.MergedCount > 1 && !node.OnChainOverflowFold {
				if _, ok := keepers[id]; !ok {
					keepers[id] = node
					keeperOrder = append(keeperOrder, id)
				}
			}
		}
	}
	if len(raws) == 0 || len(keepers) == 0 {
		return
	}
	for _, rawID := range rawOrder {
		raw := raws[rawID]
		rawValue := traceCausalProjectionNodeDisplayValue(raw)
		if rawValue <= 0 {
			continue
		}
		var host *keeperRef
		ambiguous := false
		for _, keeperID := range keeperOrder {
			keeper := keepers[keeperID]
			if traceCausalProjectionCanonicalNode(keeper.Subject) !=
				traceCausalProjectionCanonicalNode(raw.Subject) {
				continue
			}
			if !traceCausalProjectionOneSeatFamilyAgree(raw, keeper) {
				continue
			}
			if !traceCausalProjectionOneSeatWindowsCompatible(raw, keeper) {
				continue
			}
			if !traceCausalProjectionLineWithinEnvelope(raw, keeper) {
				continue
			}
			hit := false
			for _, member := range traceCausalProjectionMergedMemberValues(keeper) {
				if traceCausalProjectionOneSeatValuesEqual(rawValue, member) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
			if host != nil {
				ambiguous = true
				break
			}
			host = &keeperRef{node: keeper}
		}
		if host == nil || ambiguous {
			continue // fail-open: the honest extra row beats a guessed fold
		}
		traceCausalProjectionOneSeatAbsorbEvidence(out, host.node.EvidenceID, raw, "")
		traceCausalProjectionOneSeatDropByEvidenceID(out, raw.EvidenceID)
	}
}

// --- Arm C: post-R2 merged-twin seat convergence ------------------------------

// traceCausalProjectionMergedTwinFingerprint is the typed segment-SET
// identity of one ×N seat: canonical subject + member count + display /
// extrema at 3 decimals + the typed query window. Isomorphic to the display
// WO-D3 fingerprint. Empty for non-merged / fold / unknown-subject rows.
func traceCausalProjectionMergedTwinFingerprint(node TraceCausalProjectionNode) string {
	if node.MergedCount <= 1 || node.OnChainOverflowFold {
		return ""
	}
	if !traceCausalProjectionKnownSubject(node.Subject) {
		return ""
	}
	display := traceCausalProjectionNodeDisplayValue(node)
	if display <= 0 {
		return ""
	}
	return traceCausalProjectionCanonicalNode(node.Subject) + "\x00" +
		fmt.Sprintf("%d|%.3f|%.3f|%.3f|%.3f..%.3f", node.MergedCount, display,
			node.MergedMinMS, node.MergedMaxMS, node.QueryWindowStartTs, node.QueryWindowEndTs)
}

// traceCausalProjectionConvergeMergedTwinSeats folds TWO ×N seats with the
// identical segment-set fingerprint into one — the engine home of the WO-D3
// double-merged twin shape (one physical segment set R2-merged independently
// on two lanes). Gates, every one typed and fail-open:
//   - exactly two distinct identities under a fingerprint (≥3 fail open);
//   - token identity: equal canonical type tokens (incl. both absent), OR
//     the witnessed single-side-absence fork — one token absent AND the
//     present token resolves a duration-state family AND both objects are
//     sentinel/unresolved (the V4 exact-lane relaxation lifted post-R2; a
//     caliber-side ⌗ token resolves NO state family and fails open);
//   - W-A vetoes: both ranked-never (a rank seat is adjudicated seating),
//     cumulative accounts equal at 3dp when both published, effective
//     calibers equal at 3dp when both positive (diverging eff = the WO-D2
//     trunk/flat shape — stays two rows for the display fold's D4 dual-list
//     disclosure);
//   - cross-window veto via the fingerprint's window dimension.
//
// The keeper is the richer word lane (typed token > token-less; then R1
// bucket priority scan order). Fold = E# union + DuplicatePublications
// accounting (the existing V4 disclosure lane — the seat honestly says the
// segment set was published twice); MergedCount / extrema / sums / every
// account stay byte-identical (墙钟不可加和, never ×N+×N→×2N).
func traceCausalProjectionConvergeMergedTwinSeats(out *TraceCausalProjection) {
	type entry struct {
		node  TraceCausalProjectionNode
		order int
	}
	groups := map[string]map[string]entry{}
	var keyOrder []string
	order := 0
	for _, bucket := range traceCausalProjectionOneSeatBuckets(out) {
		for _, node := range *bucket {
			key := traceCausalProjectionMergedTwinFingerprint(node)
			if key == "" {
				continue
			}
			id := traceCausalProjectionCanonicalNode(node.EvidenceID)
			if id == "" {
				continue
			}
			g := groups[key]
			if g == nil {
				g = map[string]entry{}
				groups[key] = g
				keyOrder = append(keyOrder, key)
			}
			if _, ok := g[id]; !ok {
				g[id] = entry{node: node, order: order}
				order++
			}
		}
	}
	for _, key := range keyOrder {
		g := groups[key]
		if len(g) != 2 {
			continue // exactly the twin pair; ≥3 identities fail open
		}
		var pair []entry
		for _, e := range g {
			pair = append(pair, e)
		}
		if pair[0].order > pair[1].order {
			pair[0], pair[1] = pair[1], pair[0]
		}
		a, b := pair[0].node, pair[1].node
		if a.Rank != 0 || b.Rank != 0 {
			continue // adjudicated rank seating — never re-seated here
		}
		tokenA := traceCausalProjectionCanonicalNode(a.TypeToken)
		tokenB := traceCausalProjectionCanonicalNode(b.TypeToken)
		switch {
		case tokenA == tokenB:
			// Same-token twins (incl. both absent): the fingerprint plus the
			// shared identity word is the segment-set identity; objects must
			// agree or both be unresolved (two REAL distinct objects are two
			// accounts).
			objA := traceCausalProjectionCanonicalNode(a.Object)
			objB := traceCausalProjectionCanonicalNode(b.Object)
			if objA != objB &&
				(traceCausalProjectionKnownSubject(a.Object) || traceCausalProjectionKnownSubject(b.Object)) {
				continue
			}
		case tokenA == "" || tokenB == "":
			// Single-side token absence (WO-D3 witnessed fork): the present
			// token must resolve a duration-state family (⌗ caliber tokens
			// resolve none and fail open) and both objects must be
			// sentinel/unresolved (V4 relaxation mirror).
			tokened := a
			if tokenA == "" {
				tokened = b
			}
			if traceCausalProjectionNodeStateFamily(tokened) == "" {
				continue
			}
			if traceCausalProjectionKnownSubject(a.Object) || traceCausalProjectionKnownSubject(b.Object) {
				continue
			}
		default:
			continue // two DIFFERENT typed tokens = two accounts, fail open
		}
		// W-A veto: diverging cumulative scopes / effective calibers are two
		// account systems and never fold.
		if a.CumulativeImpactMS > 0 && b.CumulativeImpactMS > 0 &&
			!traceCausalProjectionRound3Equal(a.CumulativeImpactMS, b.CumulativeImpactMS) {
			continue
		}
		if a.EffectiveImpactMS > 0 && b.EffectiveImpactMS > 0 &&
			!traceCausalProjectionRound3Equal(a.EffectiveImpactMS, b.EffectiveImpactMS) {
			continue
		}
		keeper, loser := a, b
		if tokenA == "" && tokenB != "" {
			keeper, loser = b, a // 词位取最高车道: the typed token keeps the seat
		}
		traceCausalProjectionOneSeatAbsorbMergedTwin(out, keeper, loser)
		traceCausalProjectionOneSeatDropByEvidenceID(out, loser.EvidenceID)
	}
}

// traceCausalProjectionOneSeatAbsorbMergedTwin applies the merged-twin fold
// onto every bucket copy of the keeper: E# union, publication-count
// disclosure, empty-slot word back-fill (state word / eff adoption per the
// R1 one-fact doctrine). No summed account ever moves.
func traceCausalProjectionOneSeatAbsorbMergedTwin(out *TraceCausalProjection, keeper, loser TraceCausalProjectionNode) {
	keeperKey := traceCausalProjectionCanonicalNode(keeper.EvidenceID)
	if keeperKey == "" {
		return
	}
	for _, bucket := range traceCausalProjectionOneSeatBuckets(out) {
		for i := range *bucket {
			node := &(*bucket)[i]
			if traceCausalProjectionCanonicalNode(node.EvidenceID) != keeperKey {
				continue
			}
			absorbed := map[string]bool{keeperKey: true}
			for _, id := range node.MergedEvidenceIDs {
				absorbed[traceCausalProjectionCanonicalNode(id)] = true
			}
			appendID := func(raw string) {
				raw = strings.TrimSpace(raw)
				if raw == "" || absorbed[traceCausalProjectionCanonicalNode(raw)] {
					return
				}
				absorbed[traceCausalProjectionCanonicalNode(raw)] = true
				node.MergedEvidenceIDs = append(node.MergedEvidenceIDs, raw)
			}
			appendID(loser.EvidenceID)
			for _, id := range loser.MergedEvidenceIDs {
				appendID(id)
			}
			// V4 disclosure lane: the seat was published twice (existing
			// typed channel, zero new words).
			if node.DuplicatePublications < 1 {
				node.DuplicatePublications = 1
			}
			add := loser.DuplicatePublications
			if add < 1 {
				add = 1
			}
			node.DuplicatePublications += add
			// One-fact empty-slot back-fill (R1 doctrine): word lanes only.
			if node.StateKind == "" {
				node.StateKind = loser.StateKind
			}
			if node.TypeToken == "" {
				node.TypeToken = loser.TypeToken
			}
			if !node.PeriodicSource && node.EffectiveImpactMS <= 0 && loser.EffectiveImpactMS > 0 {
				node.EffectiveImpactMS = loser.EffectiveImpactMS
				node.EffectiveImpactPublished = node.EffectiveImpactPublished || loser.EffectiveImpactPublished
			}
			if loser.TargetImpactMS > node.TargetImpactMS {
				node.TargetImpactMS = loser.TargetImpactMS
			}
			for _, subject := range loser.MergedSubjects {
				traceCausalProjectionAppendMergedSubject(node, subject)
			}
		}
	}
}
