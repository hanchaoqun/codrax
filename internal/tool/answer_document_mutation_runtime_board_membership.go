package tool

import (
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// runtimeTraceProjRankBoardRef is one rank board's typed identity triple as
// the display reads it: verbatim target and params-fingerprint halves plus the
// query-window half resolved by the same rule as the board chips
// (runtimeTraceProjRankChipWindow). Two refs name one board when the verbatim
// halves are equal and the windows agree under the shared cluster tolerance.
type runtimeTraceProjRankBoardRef struct {
	target   string
	fp       string
	winStart float64
	winEnd   float64
	winOK    bool
}

func runtimeTraceProjRankBoardRefOfNode(node types.TraceCausalProjectionNode) runtimeTraceProjRankBoardRef {
	ref := runtimeTraceProjRankBoardRef{
		target: strings.TrimSpace(node.RankBoardTarget),
		fp:     strings.TrimSpace(node.RankBoardParamsFingerprint),
	}
	ref.winStart, ref.winEnd, ref.winOK = runtimeTraceProjRankChipWindow(node)
	return ref
}

func (ref runtimeTraceProjRankBoardRef) same(other runtimeTraceProjRankBoardRef) bool {
	if ref.target != other.target || ref.fp != other.fp || ref.winOK != other.winOK {
		return false
	}
	return !ref.winOK ||
		(math.Abs(ref.winStart-other.winStart) <= types.TraceCausalProjectionSameWindowToleranceS &&
			math.Abs(ref.winEnd-other.winEnd) <= types.TraceCausalProjectionSameWindowToleranceS)
}

// carrierNode renders the ref as a node that carries nothing but the board
// components, for consumers that adopt a board from a node.
func (ref runtimeTraceProjRankBoardRef) carrierNode() types.TraceCausalProjectionNode {
	node := types.TraceCausalProjectionNode{RankBoardTarget: ref.target, RankBoardParamsFingerprint: ref.fp}
	if ref.winOK {
		node.QueryWindowStartTs, node.QueryWindowEndTs = ref.winStart, ref.winEnd
	}
	return node
}

// runtimeTraceProjCoPublishedBoardRefs lists the OTHER named rank boards that
// published the account this node displays (HMC376): the same-fact merge kept
// one row for a fact two boards published, and the row can wear only one
// board's identity. Typed roster from the projection, self-checked against the
// node's current value there; empty on every row that was not folded across
// boards.
func runtimeTraceProjCoPublishedBoardRefs(node types.TraceCausalProjectionNode) []runtimeTraceProjRankBoardRef {
	boards := node.CoPublishedRankBoards()
	if len(boards) == 0 {
		return nil
	}
	refs := make([]runtimeTraceProjRankBoardRef, 0, len(boards))
	for _, board := range boards {
		ref := runtimeTraceProjRankBoardRefOfNode(types.TraceCausalProjectionNode{
			RankBoardTarget:            board.Target,
			RankBoardParamsFingerprint: board.ParamsFingerprint,
			QueryWindowStartTs:         board.QueryWindowStartTs,
			QueryWindowEndTs:           board.QueryWindowEndTs,
			RankQueryWindowStartTs:     board.RankQueryWindowStartTs,
			RankQueryWindowEndTs:       board.RankQueryWindowEndTs,
		})
		if ref.target != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

// runtimeTraceProjSymptomBoardElection is the A2 per-board election of the
// symptom denominator over the admission candidates.
//
// A board's account is every candidate row that board PUBLISHED: the rows
// wearing its identity, plus the rows another board's identical publication
// was folded into (HMC376). Before this, the account was only the rows that
// still wore the board's identity, so which publication happened to survive
// the same-fact merge decided how the shared facts were split between the
// boards — and the "largest board" was elected over that split. Now the
// election reads each board's own complete account, whichever publication
// kept the rows.
//
// The board with the largest account wins; a tie keeps the legacy rule (the
// lexicographically smaller board ID among boards that own a row, then boards
// known only through folded publications in typed order). Returns the
// candidate indices the winning board did not publish (empty unless ≥2 named
// boards exist), a carrier row index of the winning board (-1 = no named
// board) and a node carrying the winning board's components: the carrier row's
// own node when a candidate row wears the winning board, else a bare node with
// only that board's components (the winner is then known through folded
// publications alone, and the carrier row wears another board's identity).
// With no folded cross-board publication among the candidates every output is
// the legacy one.
func runtimeTraceProjSymptomBoardElection(model runtimeTraceProjTreeModel, measurementEligible []bool) (map[int]bool, int, types.TraceCausalProjectionNode, bool) {
	candidates := runtimeTraceProjSymptomAdmissionCandidates(model, measurementEligible)
	var named []*runtimeTraceProjTreeRow
	rowIdx := map[*runtimeTraceProjTreeRow]int{}
	for _, i := range candidates {
		row := &model.SelfRows[i]
		if strings.TrimSpace(row.Node.RankBoardTarget) == "" {
			continue // identity-less: never provably foreign, always stays
		}
		named = append(named, row)
		rowIdx[row] = i
	}
	if len(named) == 0 {
		return nil, -1, types.TraceCausalProjectionNode{}, false
	}
	boardIDs := runtimeTraceProjStableRankBoardIDs(named)
	type board struct {
		id  string // shared-index board ID; "" for a board no candidate row wears
		ref runtimeTraceProjRankBoardRef
		sum float64
	}
	var roster []board
	seenID := map[string]bool{}
	var order []string
	for _, row := range named {
		if id := boardIDs[row]; !seenID[id] {
			seenID[id] = true
			order = append(order, id)
		}
	}
	sort.Strings(order)
	for _, id := range order {
		for _, row := range named {
			if boardIDs[row] == id {
				roster = append(roster, board{id: id, ref: runtimeTraceProjRankBoardRefOfNode(row.Node)})
				break
			}
		}
	}
	owned := len(roster)
	coRefs := make(map[*runtimeTraceProjTreeRow][]runtimeTraceProjRankBoardRef, len(named))
	var foldedOnly []runtimeTraceProjRankBoardRef
	for _, row := range named {
		refs := runtimeTraceProjCoPublishedBoardRefs(row.Node)
		coRefs[row] = refs
	next:
		for _, ref := range refs {
			for _, known := range roster {
				if known.ref.same(ref) {
					continue next
				}
			}
			for _, known := range foldedOnly {
				if known.same(ref) {
					continue next
				}
			}
			foldedOnly = append(foldedOnly, ref)
		}
	}
	sort.SliceStable(foldedOnly, func(i, j int) bool {
		a, b := foldedOnly[i], foldedOnly[j]
		if a.winOK != b.winOK {
			return a.winOK
		}
		if a.winStart != b.winStart {
			return a.winStart < b.winStart
		}
		if a.winEnd != b.winEnd {
			return a.winEnd < b.winEnd
		}
		if a.target != b.target {
			return a.target < b.target
		}
		return a.fp < b.fp
	})
	for _, ref := range foldedOnly {
		roster = append(roster, board{ref: ref})
	}
	published := func(row *runtimeTraceProjTreeRow, b board) bool {
		if b.id != "" && boardIDs[row] == b.id {
			return true
		}
		for _, ref := range coRefs[row] {
			if ref.same(b.ref) {
				return true
			}
		}
		return false
	}
	for _, row := range named {
		for i := range roster {
			if published(row, roster[i]) {
				roster[i].sum += row.Node.ImpactMS
			}
		}
	}
	best := 0
	for i := 1; i < len(roster); i++ {
		if roster[i].sum > roster[best].sum {
			best = i
		}
	}
	carrier := -1
	for _, row := range named {
		if best < owned && boardIDs[row] != roster[best].id {
			continue // prefer a row that wears the winning board itself
		}
		if !published(row, roster[best]) {
			continue
		}
		if carrier < 0 || rowIdx[row] < carrier {
			carrier = rowIdx[row]
		}
	}
	excluded := map[int]bool{}
	if len(roster) >= 2 {
		for _, row := range named {
			if !published(row, roster[best]) {
				excluded[rowIdx[row]] = true
			}
		}
	}
	winner := roster[best].ref.carrierNode()
	if best < owned && carrier >= 0 {
		winner = model.SelfRows[carrier].Node
	}
	return excluded, carrier, winner, true
}
