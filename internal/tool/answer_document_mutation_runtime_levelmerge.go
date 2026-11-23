package tool

// answer_document_mutation_runtime_levelmerge.go — LEVELMERGE-1 件2/件3
// display stamps (方案 P 区间分账 + 聚合席↔成员两向互指, user rulings
// 2026-07-18; ledger real_trace_campaign_20260705.md §29.104 residual lane,
// scout report levelkey_verify §2.4/§3.4).
//
// 件2: the engine's gated-share split rows arrive with their typed
// gated-share claim (HMC377 round 11: resolved by the engine's one resolver;
// Node.ShareClaims). The stamp names the claiming inversion seats' [E#] tags
// from the claim's holder records ALL-OR-NOTHING (any holder undrawn → zero
// refs, the 行2 sentence keeps the generic 本线程反转席 noun — 宁漏勿假指).
// Values never move here; wording input only.
//
// 件3: the aggregate-seat ↔ member-occurrence two-way cross-reference (the
// census §1.3-1 missing pair). Typed identity only, reproducing the engine's
// own ORD-A membership predicate on the display face:
//   member  = a wakeup_causal_impact VIEW row with ChainDepth > 0 of the
//             same (subject, StateKind) — exactly the aggregation admission
//             predicate (PID>0 ∧ ChainDepth>0 ∧ dominant state non-empty);
//   seat    = the SINGLE root_cause_-lane row of that (subject, StateKind)
//             with ChainDepth > 0 (the depth-blind aggregate seat; a demoted
//             gated-share constituent clone never takes the seat role).
// ≥2 members (the aggregate admission threshold), exactly one seat candidate
// (ambiguity skips), every ref resolved or none on the seat-side listing
// (XLANE-2 all-or-nothing 纪律), same-board gate through the ONE shared board
// index (XLANE-3 让位红线 — cross-board rows never cross-point).

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjStampGatedShareSplit resolves the 件2 claim-seat pointers.
//
// HMC377 round 11 (batch-owner ruling R-377-11 ①): the split's carved share is
// a typed claim the engine's one resolver decides (rank_share_claims.go), so
// the pointer reads that claim — the claiming seat is named only while the
// claim resolves whole to it (its holder records drawn here, all-or-nothing),
// never by matching the claim-seat line intervals against drawn rows (the
// former lookup decided the relation a second time). A residual whose carved
// share resolved to the published successor of its withdrawn claim seat names
// that successor when it is drawn as this thread's split constituent row (its
// own typed marker — a positive proof); otherwise no row is named.
func runtimeTraceProjStampGatedShareSplit(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	index := runtimeTraceProjDrawnRecordIndex(model, all)
	boardIndex := runtimeTraceProjRankBoardIndexFor(all)
	for _, row := range all {
		if !row.HasData || (row.Node.GatedShareFullMS <= 0 && row.Node.GatedShareOverlapDisclosureMS <= 0 && !row.Node.GatedShareConstituentSeat) {
			continue
		}
		var claim *types.TraceCausalProjectionShareClaim
		for i := range row.Node.ShareClaims {
			if row.Node.ShareClaims[i].Part == string(tracequery.RootCauseShareClaimGatedShare) {
				claim = &row.Node.ShareClaims[i]
				break
			}
		}
		if claim == nil || !claim.Resolved || len(claim.HolderEvidenceIDs) == 0 {
			continue
		}
		var holders []runtimeTraceProjDrawnRecord
		for _, id := range claim.HolderEvidenceIDs {
			drawn, ok := index[strings.TrimSpace(id)]
			// 修补轮 件4 (2026-07-18), kept: same-board gate through the ONE
			// shared board index (XLANE-3 让位红线: cross-board rows never
			// cross-point).
			if !ok || drawn.row == nil || drawn.row == row || strings.TrimSpace(drawn.tag) == "" ||
				!runtimeTraceProjSemanticSubsetSameBoard(boardIndex, drawn.row, row) {
				holders = nil
				break
			}
			holders = append(holders, drawn)
		}
		if len(holders) == 0 {
			continue // all-or-nothing: generic noun instead of a partial list
		}
		if claim.Successor {
			// HMC377 round 12 (register R11T-3, mutant T13): the drawn row's
			// own constituent marker is the proof — a record drawn folded
			// into a row that wears the split's constituent family is drawn
			// as that row, so a separate "drawn as its own row" check
			// distinguished no reachable shape and is removed.
			if len(holders) == 1 && holders[0].row.Node.GatedShareConstituentSeat {
				row.GatedShareSuccessorRef = strings.TrimSpace(holders[0].tag)
			}
			continue
		}
		refs := make([]string, 0, len(holders))
		seen := map[string]bool{}
		for _, h := range holders {
			if tag := strings.TrimSpace(h.tag); !seen[tag] {
				seen[tag] = true
				refs = append(refs, tag)
			}
		}
		row.GatedShareClaimRefs = refs
	}
}

// runtimeTraceProjStampAggregateMemberCrossRefs stamps the 件3 two-way
// pointers.
//
// HMC377 R-377-13 ① (batch-owner ruling 2026-11-22): an occurrence row whose
// producer-minted occurrence-group identity (MeasuredOccurrenceGroup) is
// the measured account of exactly one drawn aggregate seat of the same thread
// is that seat's constituent by typed proof, however many of the seat's other
// occurrences render as their own rows (a sibling occurrence drawn only inside
// an overflow fold row left the subject/state census below with a single
// member, and the occurrence stood beside its aggregate as a second additive
// line on the same instants). It wears the member pointer on that proof; the
// seat's constituent list keeps its all-or-nothing census below.
func runtimeTraceProjStampAggregateMemberCrossRefs(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	runtimeTraceProjStampAggregateMemberIdentityRefs(all)
	type family struct {
		members []*runtimeTraceProjTreeRow
		seats   []*runtimeTraceProjTreeRow
	}
	families := map[string]*family{}
	key := func(row *runtimeTraceProjTreeRow) string {
		state := strings.TrimSpace(row.Node.StateKind)
		if state == "" {
			return ""
		}
		subject := runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject)
		if subject == "" {
			return ""
		}
		return subject + "\x00" + state
	}
	for _, row := range all {
		if !row.HasData || row.Node.ChainDepth <= 0 {
			continue
		}
		k := key(row)
		if k == "" {
			continue
		}
		predicate := strings.TrimSpace(row.Node.Predicate)
		switch {
		case predicate == "wakeup_causal_impact":
			fam := families[k]
			if fam == nil {
				fam = &family{}
				families[k] = fam
			}
			fam.members = append(fam.members, row)
		case strings.HasPrefix(predicate, "root_cause_"):
			if row.Node.GatedShareConstituentSeat {
				continue // the demoted A clone never takes the seat role
			}
			fam := families[k]
			if fam == nil {
				fam = &family{}
				families[k] = fam
			}
			fam.seats = append(fam.seats, row)
		}
	}
	boardIndex := runtimeTraceProjRankBoardIndexFor(all)
	for _, fam := range families {
		// ≥2 OCCURRENCES = the engine aggregate admission threshold; the
		// display ×N fold may have merged several occurrence rows into one
		// rendered member row (MergedCount carries the occurrence count), so
		// the census counts occurrences, never rendered rows. A single
		// occurrence never formed an aggregate (its rank twin is the same
		// fact and the R1 merge already handles that pair).
		occurrenceTotal := 0
		for _, member := range fam.members {
			count := member.Node.MergedCount
			if count < 1 {
				count = 1
			}
			occurrenceTotal += count
		}
		if occurrenceTotal < 2 || len(fam.members) == 0 || len(fam.seats) != 1 {
			continue
		}
		seat := fam.seats[0]
		seatRef := strings.TrimSpace(seat.EvidenceTag)
		if seatRef == "" {
			continue
		}
		memberRefs := make([]string, 0, len(fam.members))
		complete := true
		for _, member := range fam.members {
			if !runtimeTraceProjSemanticSubsetSameBoard(boardIndex, member, seat) {
				complete = false
				break
			}
			ref := strings.TrimSpace(member.EvidenceTag)
			if ref == "" {
				complete = false
				break
			}
			memberRefs = append(memberRefs, ref)
		}
		if !complete {
			continue // all-or-nothing on the whole family (宁漏勿假指)
		}
		seat.AggregateMemberRefs = memberRefs
		for _, member := range fam.members {
			member.AggregateSeatRef = seatRef
		}
	}
}

// runtimeTraceProjStampAggregateMemberIdentityRefs is the typed member arm of
// runtimeTraceProjStampAggregateMemberCrossRefs (R-377-13 ①): the occurrence
// row's group identity equals the seat's measured account identity, the seat
// is the only drawn row of the thread that shows that account, and both sit
// on one rank board. The member pointer only; values never decide.
func runtimeTraceProjStampAggregateMemberIdentityRefs(all []*runtimeTraceProjTreeRow) {
	seats := map[string][]*runtimeTraceProjTreeRow{}
	for _, row := range all {
		if !row.HasData || row.Node.GatedShareConstituentSeat ||
			!strings.HasPrefix(strings.TrimSpace(row.Node.Predicate), "root_cause_") {
			continue
		}
		// The producer hash names the physical thread (it hashes the pid).
		key := strings.TrimSpace(row.Node.MeasuredStateFactKey())
		if key == "" || strings.TrimSpace(row.EvidenceTag) == "" {
			continue
		}
		seats[key] = append(seats[key], row)
	}
	if len(seats) == 0 {
		return
	}
	boardIndex := runtimeTraceProjRankBoardIndexFor(all)
	for _, row := range all {
		if !row.HasData || row.Node.ChainDepth <= 0 || row.Node.MergedCount > 1 || row.AggregateSeatRef != "" ||
			strings.TrimSpace(row.Node.Predicate) != "wakeup_causal_impact" || strings.TrimSpace(row.EvidenceTag) == "" {
			continue
		}
		group := row.Node.MeasuredOccurrenceGroup()
		if group == "" {
			continue
		}
		owners := seats[group]
		if len(owners) != 1 || owners[0] == row || !runtimeTraceProjSemanticSubsetSameBoard(boardIndex, row, owners[0]) {
			continue // ambiguity fails open
		}
		row.AggregateSeatRef = strings.TrimSpace(owners[0].EvidenceTag)
	}
}
