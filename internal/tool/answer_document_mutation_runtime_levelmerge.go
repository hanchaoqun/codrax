package tool

// answer_document_mutation_runtime_levelmerge.go — LEVELMERGE-1 件2/件3
// display stamps (方案 P 区间分账 + 聚合席↔成员两向互指, user rulings
// 2026-07-18; ledger real_trace_campaign_20260705.md §29.104 residual lane,
// scout report levelkey_verify §2.4/§3.4).
//
// 件2: the engine's gated-share split rows arrive with the typed claim-seat
// line intervals (Node.GatedShareClaimSeats — the claiming priority-inversion
// seats' own LineStart..LineEnd). The stamp resolves each interval to the
// rendered inversion row's [E#] tag ALL-OR-NOTHING (any span unresolvable or
// ambiguous → zero refs, the 行2 sentence keeps the generic 本线程反转席
// noun — 宁漏勿假指). Values never move here; wording input only.
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
	"strconv"
	"strings"
)

// runtimeTraceProjParseClaimSeatSpan parses one typed "start..end" claim-seat
// line interval. Strict form only — anything else reports ok=false.
func runtimeTraceProjParseClaimSeatSpan(span string) (int, int, bool) {
	head, tail, ok := strings.Cut(strings.TrimSpace(span), "..")
	if !ok {
		return 0, 0, false
	}
	start, err := strconv.Atoi(strings.TrimSpace(head))
	if err != nil {
		return 0, 0, false
	}
	end, err := strconv.Atoi(strings.TrimSpace(tail))
	if err != nil {
		return 0, 0, false
	}
	return start, end, true
}

// runtimeTraceProjStampGatedShareSplit resolves the 件2 claim-seat pointers.
func runtimeTraceProjStampGatedShareSplit(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
	boardIndex := runtimeTraceProjRankBoardIndexFor(all)
	for _, row := range all {
		if row.Node.GatedShareFullMS <= 0 && row.Node.GatedShareOverlapDisclosureMS <= 0 {
			continue
		}
		if len(row.Node.GatedShareClaimSeats) == 0 {
			continue
		}
		refs := make([]string, 0, len(row.Node.GatedShareClaimSeats))
		complete := true
		for _, span := range row.Node.GatedShareClaimSeats {
			start, end, ok := runtimeTraceProjParseClaimSeatSpan(span)
			if !ok {
				complete = false
				break
			}
			var partner *runtimeTraceProjTreeRow
			ambiguous := false
			for _, cand := range all {
				if cand == row || !cand.HasData {
					continue
				}
				if strings.TrimSpace(cand.Node.TypeToken) != "priority_inversion_candidate" {
					continue
				}
				if cand.Node.LineStart != start || cand.Node.LineEnd != end {
					continue
				}
				if runtimeTraceCausalProjectionCanonicalNode(cand.Node.Subject) !=
					runtimeTraceCausalProjectionCanonicalNode(row.Node.Subject) {
					continue
				}
				// 修补轮 件4 (2026-07-18): same-board gate through the ONE
				// shared board index — symmetric with the 件3 crossref pass
				// below (XLANE-3 让位红线: cross-board rows never cross-
				// point). A cross-named-board row with the same line interval
				// never resolves the claim pointer; when no same-board
				// partner remains the sentence keeps the generic noun.
				if !runtimeTraceProjSemanticSubsetSameBoard(boardIndex, cand, row) {
					continue
				}
				if partner != nil {
					ambiguous = true
					break
				}
				partner = cand
			}
			if partner == nil || ambiguous || strings.TrimSpace(partner.EvidenceTag) == "" {
				complete = false
				break
			}
			refs = append(refs, strings.TrimSpace(partner.EvidenceTag))
		}
		if !complete || len(refs) == 0 {
			continue // all-or-nothing: generic noun instead of a partial list
		}
		row.GatedShareClaimRefs = refs
	}
}

// runtimeTraceProjStampAggregateMemberCrossRefs stamps the 件3 two-way
// pointers.
func runtimeTraceProjStampAggregateMemberCrossRefs(model *runtimeTraceProjTreeModel) {
	all := runtimeTraceProjSMR1AllRows(model)
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
