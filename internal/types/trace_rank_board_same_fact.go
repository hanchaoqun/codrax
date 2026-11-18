package types

import (
	"math"
	"strconv"
	"strings"
)

// TraceSameFactRankBoard names one more rank board that published the very
// account a projection row displays.
//
// The same-fact merge keeps one row per observed wall-clock fact. When two
// rank boards (two queries over the same target and window with different
// rank parameters, or two targets that both rank one thread) publish that
// fact, the surviving row can wear only one board's identity; the other
// board's publication stays reachable through its evidence ids, but its board
// membership would be lost. A per-board account computed afterwards — "what
// did THIS board publish for the target" — would then run over an arbitrary
// split of the shared facts instead of each board's own rows.
//
// The entry keeps that membership: the board's typed identity triple exactly
// as its own publication carried it, and the account value it vouches for.
// It is a display-membership input for per-board account faces only. It is
// never a rank, seat, value, tier or causal input, never moves a row between
// lanes, and absence keeps every consumer on the row's own board identity.
type TraceSameFactRankBoard struct {
	Target                 string
	ParamsFingerprint      string
	QueryWindowStartTs     float64
	QueryWindowEndTs       float64
	RankQueryWindowStartTs float64
	RankQueryWindowEndTs   float64
	// AccountMS is the row value this board published. An entry speaks for a
	// row only while the row still displays that value (same three-decimal
	// rule as the same-fact identity itself), so a later fold that changes
	// the row's value silently retires every inherited entry.
	AccountMS float64
}

func traceSameFactAccountKey(ms float64) string {
	return strconv.FormatFloat(ms, 'f', 3, 64)
}

func traceSameFactRankBoardOf(node TraceCausalProjectionNode) (TraceSameFactRankBoard, bool) {
	target := strings.TrimSpace(node.RankBoardTarget)
	if target == "" || node.ImpactMS <= 0 {
		return TraceSameFactRankBoard{}, false
	}
	return TraceSameFactRankBoard{
		Target:                 target,
		ParamsFingerprint:      strings.TrimSpace(node.RankBoardParamsFingerprint),
		QueryWindowStartTs:     node.QueryWindowStartTs,
		QueryWindowEndTs:       node.QueryWindowEndTs,
		RankQueryWindowStartTs: node.RankQueryWindowStartTs,
		RankQueryWindowEndTs:   node.RankQueryWindowEndTs,
		AccountMS:              node.ImpactMS,
	}, true
}

// window is the query window that names the board: the publication's own, else
// the pair a merge preserved for its rank ordinal (the board-chip rule).
func (board TraceSameFactRankBoard) window() (float64, float64, bool) {
	if TraceCausalProjectionWindowPresent(board.QueryWindowStartTs, board.QueryWindowEndTs) {
		return board.QueryWindowStartTs, board.QueryWindowEndTs, true
	}
	if TraceCausalProjectionWindowPresent(board.RankQueryWindowStartTs, board.RankQueryWindowEndTs) {
		return board.RankQueryWindowStartTs, board.RankQueryWindowEndTs, true
	}
	return 0, 0, false
}

func (board TraceSameFactRankBoard) sameBoard(other TraceSameFactRankBoard) bool {
	if board.Target != other.Target || board.ParamsFingerprint != other.ParamsFingerprint {
		return false
	}
	start, end, ok := board.window()
	otherStart, otherEnd, otherOK := other.window()
	if ok != otherOK {
		return false
	}
	return !ok || (math.Abs(start-otherStart) <= TraceCausalProjectionSameWindowToleranceS &&
		math.Abs(end-otherEnd) <= TraceCausalProjectionSameWindowToleranceS)
}

// CoPublishedRankBoards returns the rank boards recorded as publishers of the
// account this row currently displays, beside the board identity the row
// wears. (On a merged ×N row the roster is re-derived from the members and
// may name the row's own board again; consumers match boards, so that entry
// is redundant, never a second board.) Entries whose vouched value no longer
// equals the row's value are dropped — fail closed: an unproven board never
// gains a row.
func (n TraceCausalProjectionNode) CoPublishedRankBoards() []TraceSameFactRankBoard {
	if len(n.SameFactRankBoards) == 0 || n.ImpactMS <= 0 {
		return nil
	}
	account := traceSameFactAccountKey(n.ImpactMS)
	var out []TraceSameFactRankBoard
	for _, board := range n.SameFactRankBoards {
		if strings.TrimSpace(board.Target) == "" || traceSameFactAccountKey(board.AccountMS) != account {
			continue
		}
		out = append(out, board)
	}
	return out
}

// traceCausalProjectionAbsorbSameFactRankBoards records, on the survivor of a
// same-fact merge, the absorbed publication's rank board (and every board that
// publication had already absorbed). Only a NAMED board that published the
// SAME value joins: a twin that describes the fact with another account (a
// re-anchored or gated-share half) published a different number on its board
// and must not lend that board the survivor's value.
func traceCausalProjectionAbsorbSameFactRankBoards(survivor *TraceCausalProjectionNode, loser TraceCausalProjectionNode) {
	if survivor == nil || survivor.ImpactMS <= 0 {
		return
	}
	account := traceSameFactAccountKey(survivor.ImpactMS)
	own, ownNamed := traceSameFactRankBoardOf(*survivor)
	add := func(board TraceSameFactRankBoard) {
		if strings.TrimSpace(board.Target) == "" || traceSameFactAccountKey(board.AccountMS) != account {
			return
		}
		if ownNamed && own.sameBoard(board) {
			return
		}
		for _, existing := range survivor.SameFactRankBoards {
			if existing.sameBoard(board) && traceSameFactAccountKey(existing.AccountMS) == account {
				return
			}
		}
		// Copy on first growth: bucket copies of one node share the slice
		// header, and an in-place append could surface on a sibling copy.
		grown := make([]TraceSameFactRankBoard, 0, len(survivor.SameFactRankBoards)+1)
		grown = append(grown, survivor.SameFactRankBoards...)
		survivor.SameFactRankBoards = append(grown, board)
	}
	if board, ok := traceSameFactRankBoardOf(loser); ok {
		add(board)
	}
	for _, board := range loser.CoPublishedRankBoards() {
		add(board)
	}
}

// traceCausalProjectionMergedSameFactRankBoards re-derives the membership for
// a merged ×N row: a board published the merged account only when it
// published EVERY member (the members are distinct facts, so one member a
// board never published breaks that board's claim on the total). The seed
// member's own entries are never inherited as they stand.
func traceCausalProjectionMergedSameFactRankBoards(nodes []TraceCausalProjectionNode, members []int, mergedAccountMS float64) []TraceSameFactRankBoard {
	if len(members) == 0 || mergedAccountMS <= 0 {
		return nil
	}
	var common []TraceSameFactRankBoard
	for position, idx := range members {
		member := nodes[idx]
		var boards []TraceSameFactRankBoard
		if own, ok := traceSameFactRankBoardOf(member); ok {
			boards = append(boards, own)
		}
		boards = append(boards, member.CoPublishedRankBoards()...)
		if position == 0 {
			common = boards
			continue
		}
		kept := common[:0:0]
		for _, candidate := range common {
			for _, board := range boards {
				if candidate.sameBoard(board) {
					kept = append(kept, candidate)
					break
				}
			}
		}
		common = kept
		if len(common) == 0 {
			return nil
		}
	}
	out := make([]TraceSameFactRankBoard, 0, len(common))
	for _, board := range common {
		board.AccountMS = mergedAccountMS
		out = append(out, board)
	}
	return out
}

// TraceRankBoardPartAccount is one board's own part of a ×N row whose members
// were published by different rank boards.
//
// The same-kind merge groups by subject and object, not by board, so two boards
// that each measured one wait at their own threshold fold into one row whose
// value adds both measurements. That total is what the row displays, but it is
// what no board published: an account per board has to read each board's own
// part. The part is the same merge restricted to the members that board
// published, so it equals what the board would have displayed by itself.
//
// An entry with an empty Target is the part no named board claims (members
// without a rank board): it belongs to every board's account, like any other
// identity-less row.
type TraceRankBoardPartAccount struct {
	TraceSameFactRankBoard
	CumulativeMS float64
	EffectiveMS  float64
	// RowMS is the merged row value this split speaks for. A later change of
	// the row's value retires the entry.
	RowMS float64
}

// PublishedRankBoardParts returns the per-board parts recorded for the value
// this row currently displays.
func (n TraceCausalProjectionNode) PublishedRankBoardParts() []TraceRankBoardPartAccount {
	if len(n.RankBoardPartAccounts) == 0 || n.ImpactMS <= 0 {
		return nil
	}
	account := traceSameFactAccountKey(n.ImpactMS)
	var out []TraceRankBoardPartAccount
	for _, part := range n.RankBoardPartAccounts {
		if traceSameFactAccountKey(part.RowMS) == account {
			out = append(out, part)
		}
	}
	return out
}

// traceRankBoardMemberPart is what one board published of one merge member. The
// member's own value when the board published the member whole, its recorded
// part when the member is itself a cross-board merge.
func traceRankBoardMemberPart(member TraceCausalProjectionNode, board TraceSameFactRankBoard) (TraceCausalProjectionNode, bool) {
	parts := member.PublishedRankBoardParts()
	unclaimed := -1
	for i, part := range parts {
		if strings.TrimSpace(part.Target) == "" {
			unclaimed = i
			continue
		}
		if part.sameBoard(board) {
			return traceRankBoardMemberWithAccount(member, part), true
		}
	}
	if own, ok := traceSameFactRankBoardOf(member); ok && own.sameBoard(board) {
		return member, true
	}
	for _, other := range member.CoPublishedRankBoards() {
		if other.sameBoard(board) {
			return member, true
		}
	}
	if unclaimed >= 0 {
		return traceRankBoardMemberWithAccount(member, parts[unclaimed]), true
	}
	if _, named := traceSameFactRankBoardOf(member); !named {
		return member, true // identity-less: never provably another board's
	}
	return TraceCausalProjectionNode{}, false
}

func traceRankBoardMemberWithAccount(member TraceCausalProjectionNode, part TraceRankBoardPartAccount) TraceCausalProjectionNode {
	member.ImpactMS = part.AccountMS
	member.CumulativeImpactMS = part.CumulativeMS
	member.EffectiveImpactMS = part.EffectiveMS
	return member
}

// traceCausalProjectionMergedRankBoardParts splits a merged ×N row into each
// board's own part. Nil when every named board published every member: the row
// value is then every publisher's account, as it always was.
func traceCausalProjectionMergedRankBoardParts(nodes []TraceCausalProjectionNode, members []int, backgroundLane bool, merged TraceCausalProjectionNode) []TraceRankBoardPartAccount {
	if len(members) < 2 || merged.ImpactMS <= 0 {
		return nil
	}
	var boards []TraceSameFactRankBoard
	addBoard := func(board TraceSameFactRankBoard) {
		if strings.TrimSpace(board.Target) == "" {
			return
		}
		for _, known := range boards {
			if known.sameBoard(board) {
				return
			}
		}
		boards = append(boards, board)
	}
	unnamedMember := false
	for _, idx := range members {
		member := nodes[idx]
		own, named := traceSameFactRankBoardOf(member)
		if named {
			addBoard(own)
		} else {
			unnamedMember = true
		}
		for _, board := range member.CoPublishedRankBoards() {
			addBoard(board)
		}
		for _, part := range member.PublishedRankBoardParts() {
			if strings.TrimSpace(part.Target) == "" {
				unnamedMember = true
			}
			addBoard(part.TraceSameFactRankBoard)
		}
	}
	if len(boards) == 0 {
		return nil
	}
	// One restricted merge: the members this board published, through the same
	// value calibers as the whole row.
	partOf := func(board TraceSameFactRankBoard, selected []TraceCausalProjectionNode) (TraceRankBoardPartAccount, bool) {
		if len(selected) == 0 {
			return TraceRankBoardPartAccount{}, false
		}
		value := selected[0]
		if len(selected) > 1 {
			indices := make([]int, len(selected))
			for i := range selected {
				indices[i] = i
			}
			value = traceCausalProjectionMergeSameKindMemberValues(selected, 0, indices, backgroundLane)
		}
		board.AccountMS = value.ImpactMS
		if board.AccountMS <= 0 {
			board.AccountMS = value.CumulativeImpactMS
		}
		if board.AccountMS <= 0 {
			return TraceRankBoardPartAccount{}, false
		}
		return TraceRankBoardPartAccount{TraceSameFactRankBoard: board, CumulativeMS: value.CumulativeImpactMS,
			EffectiveMS: value.EffectiveImpactMS, RowMS: merged.ImpactMS}, true
	}
	var parts []TraceRankBoardPartAccount
	split := false
	for _, board := range boards {
		var selected []TraceCausalProjectionNode
		for _, idx := range members {
			if member, ok := traceRankBoardMemberPart(nodes[idx], board); ok {
				selected = append(selected, member)
			}
		}
		split = split || len(selected) != len(members)
		if part, ok := partOf(board, selected); ok {
			parts = append(parts, part)
		}
	}
	if unnamedMember {
		var selected []TraceCausalProjectionNode
		for _, idx := range members {
			member := nodes[idx]
			if _, named := traceSameFactRankBoardOf(member); !named {
				selected = append(selected, member)
				continue
			}
			for _, part := range member.PublishedRankBoardParts() {
				if strings.TrimSpace(part.Target) == "" {
					selected = append(selected, traceRankBoardMemberWithAccount(member, part))
					break
				}
			}
		}
		if part, ok := partOf(TraceSameFactRankBoard{}, selected); ok {
			parts = append(parts, part)
			split = true
		}
	}
	if !split {
		return nil
	}
	return parts
}
