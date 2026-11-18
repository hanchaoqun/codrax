package types

import (
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

func (board TraceSameFactRankBoard) sameBoard(other TraceSameFactRankBoard) bool {
	return board.Target == other.Target && board.ParamsFingerprint == other.ParamsFingerprint &&
		board.QueryWindowStartTs == other.QueryWindowStartTs && board.QueryWindowEndTs == other.QueryWindowEndTs &&
		board.RankQueryWindowStartTs == other.RankQueryWindowStartTs && board.RankQueryWindowEndTs == other.RankQueryWindowEndTs
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
