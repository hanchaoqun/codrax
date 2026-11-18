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

// runtimeTraceProjRankBoardRefOf reads one roster entry's board identity.
func runtimeTraceProjRankBoardRefOf(board types.TraceSameFactRankBoard) runtimeTraceProjRankBoardRef {
	return runtimeTraceProjRankBoardRefOfNode(types.TraceCausalProjectionNode{
		RankBoardTarget:            board.Target,
		RankBoardParamsFingerprint: board.ParamsFingerprint,
		QueryWindowStartTs:         board.QueryWindowStartTs,
		QueryWindowEndTs:           board.QueryWindowEndTs,
		RankQueryWindowStartTs:     board.RankQueryWindowStartTs,
		RankQueryWindowEndTs:       board.RankQueryWindowEndTs,
	})
}

// runtimeTraceProjCoPublishedBoardRefs lists the OTHER named rank boards that
// published some or all of the account this node displays (HMC376): the rows
// that folded publications of several boards can wear only one board's
// identity. Typed rosters from the projection, self-checked against the node's
// current value there; empty on every row that was not folded across boards.
func runtimeTraceProjCoPublishedBoardRefs(node types.TraceCausalProjectionNode) []runtimeTraceProjRankBoardRef {
	var refs []runtimeTraceProjRankBoardRef
	for _, board := range node.CoPublishedRankBoards() {
		if ref := runtimeTraceProjRankBoardRefOf(board); ref.target != "" {
			refs = append(refs, ref)
		}
	}
	for _, part := range node.PublishedRankBoardParts() {
		if ref := runtimeTraceProjRankBoardRefOf(part.TraceSameFactRankBoard); ref.target != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

// runtimeTraceProjBoardRowAccount is what one rank board accounts for on one
// row, on the three value channels the per-board faces read.
type runtimeTraceProjBoardRowAccount struct {
	impactMS     float64
	cumulativeMS float64
	effectiveMS  float64
}

// runtimeTraceProjRowBoardAccount reads one board's own account of a row. A
// merged row whose members several boards published holds each board's own part
// (the row value is no single board's account); otherwise a board that wears
// the row or co-published it accounts for the whole row. ok=false means the
// board published nothing of this row.
func runtimeTraceProjRowBoardAccount(node types.TraceCausalProjectionNode, ref runtimeTraceProjRankBoardRef, worn bool) (runtimeTraceProjBoardRowAccount, bool) {
	parts := node.PublishedRankBoardParts()
	unclaimed := -1
	for i, part := range parts {
		if strings.TrimSpace(part.Target) == "" {
			unclaimed = i
			continue
		}
		if runtimeTraceProjRankBoardRefOf(part.TraceSameFactRankBoard).same(ref) {
			return runtimeTraceProjBoardRowAccount{part.AccountMS, part.CumulativeMS, part.EffectiveMS}, true
		}
	}
	whole := runtimeTraceProjBoardRowAccount{node.ImpactMS, node.CumulativeImpactMS, node.EffectiveImpactMS}
	if worn {
		return whole, true
	}
	for _, board := range node.CoPublishedRankBoards() {
		if runtimeTraceProjRankBoardRefOf(board).same(ref) {
			return whole, true
		}
	}
	if unclaimed >= 0 {
		part := parts[unclaimed]
		return runtimeTraceProjBoardRowAccount{part.AccountMS, part.CumulativeMS, part.EffectiveMS}, true
	}
	return runtimeTraceProjBoardRowAccount{}, false
}

// runtimeTraceProjPublishedBoard is one rank board that published something on
// a row population.
type runtimeTraceProjPublishedBoard struct {
	id  string // shared-index board ID; "" for a board no row of the population wears
	ref runtimeTraceProjRankBoardRef
}

// runtimeTraceProjPublishedBoardRoster lists every board that published on the
// rows: the boards the rows wear in board-ID order (the first owned entries),
// then the boards known only through folded publications in typed order.
func runtimeTraceProjPublishedBoardRoster(rows []*runtimeTraceProjTreeRow, boardIDs map[*runtimeTraceProjTreeRow]string) ([]runtimeTraceProjPublishedBoard, int) {
	var roster []runtimeTraceProjPublishedBoard
	seenID := map[string]bool{}
	var order []string
	for _, row := range rows {
		if id := boardIDs[row]; !seenID[id] {
			seenID[id] = true
			order = append(order, id)
		}
	}
	sort.Strings(order)
	for _, id := range order {
		for _, row := range rows {
			if boardIDs[row] == id {
				roster = append(roster, runtimeTraceProjPublishedBoard{id: id, ref: runtimeTraceProjRankBoardRefOfNode(row.Node)})
				break
			}
		}
	}
	owned := len(roster)
	var foldedOnly []runtimeTraceProjRankBoardRef
	for _, row := range rows {
	next:
		for _, ref := range runtimeTraceProjCoPublishedBoardRefs(row.Node) {
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
		roster = append(roster, runtimeTraceProjPublishedBoard{ref: ref})
	}
	return roster, owned
}

// runtimeTraceProjBoardEffectiveSums adds each board's own effective attribution
// over the seat population, so a board's sum does not depend on which of several
// parameter forks of one target kept a shared seat. A seat that a board of ANOTHER
// target also published stays on the board it wears: the two targets' accounts of
// one report are never pooled through a shared seat.
func runtimeTraceProjBoardEffectiveSums(seats []*runtimeTraceProjTreeRow, boardIDs map[*runtimeTraceProjTreeRow]string) []float64 {
	roster, _ := runtimeTraceProjPublishedBoardRoster(seats, boardIDs)
	sums := make([]float64, len(roster))
	for _, row := range seats {
		for i, board := range roster {
			worn := board.id != "" && boardIDs[row] == board.id
			if !worn && board.ref.target != strings.TrimSpace(row.Node.RankBoardTarget) {
				continue
			}
			if part, ok := runtimeTraceProjRowBoardAccount(row.Node, board.ref, worn); ok {
				sums[i] += part.effectiveMS
			}
		}
	}
	return sums
}

// runtimeTraceProjSymptomBoardVote is the outcome of the A2 per-board election
// over the admission candidates.
type runtimeTraceProjSymptomBoardVote struct {
	// excluded lists the candidate indices the winning board did not publish
	// (empty unless ≥2 named boards exist).
	excluded map[int]bool
	// parts holds the winning board's own part of a candidate row when that is
	// less than the whole row (a merge across boards).
	parts map[int]float64
	// carrier is a row index of the winning board (-1 = no named board).
	carrier int
	// winner carries the winning board's components: the carrier row's own node
	// when a candidate row wears the winning board, else a bare node with only
	// that board's components (the winner is then known through folded
	// publications alone, and the carrier row wears another board's identity).
	winner types.TraceCausalProjectionNode
	named  bool
}

// runtimeTraceProjSymptomBoardElection is the A2 per-board election of the
// symptom denominator over the admission candidates.
//
// A board's account is what that board PUBLISHED of every candidate row: the
// rows wearing its identity, the rows another board's identical publication was
// folded into, and its own part of a merge across boards (HMC376). Before this,
// the account was only the rows that still wore the board's identity, so which
// publication happened to survive a fold decided how the shared facts were split
// between the boards — and the "largest board" was elected over that split. Now
// the election reads each board's own complete account, whichever publication
// kept the rows.
//
// The board with the largest account wins; a tie keeps the legacy rule (the
// lexicographically smaller board ID among boards that own a row, then boards
// known only through folded publications in typed order). With no folded
// cross-board publication among the candidates every output is the legacy one.
func runtimeTraceProjSymptomBoardElection(model runtimeTraceProjTreeModel, measurementEligible []bool) runtimeTraceProjSymptomBoardVote {
	vote := runtimeTraceProjSymptomBoardVote{carrier: -1}
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
		return vote
	}
	boardIDs := runtimeTraceProjStableRankBoardIDs(named)
	roster, owned := runtimeTraceProjPublishedBoardRoster(named, boardIDs)
	sums := make([]float64, len(roster))
	account := func(row *runtimeTraceProjTreeRow, b runtimeTraceProjPublishedBoard) (runtimeTraceProjBoardRowAccount, bool) {
		return runtimeTraceProjRowBoardAccount(row.Node, b.ref, b.id != "" && boardIDs[row] == b.id)
	}
	for _, row := range named {
		for i := range roster {
			if part, ok := account(row, roster[i]); ok {
				sums[i] += part.impactMS
			}
		}
	}
	best := 0
	for i := 1; i < len(roster); i++ {
		if sums[i] > sums[best] {
			best = i
		}
	}
	for _, row := range named {
		if best < owned && boardIDs[row] != roster[best].id {
			continue // prefer a row that wears the winning board itself
		}
		if _, ok := account(row, roster[best]); !ok {
			continue
		}
		if vote.carrier < 0 || rowIdx[row] < vote.carrier {
			vote.carrier = rowIdx[row]
		}
	}
	vote.excluded = map[int]bool{}
	vote.parts = map[int]float64{}
	for _, row := range named {
		part, ok := account(row, roster[best])
		switch {
		case !ok:
			if len(roster) >= 2 {
				vote.excluded[rowIdx[row]] = true
			}
		case part.impactMS != row.Node.ImpactMS:
			vote.parts[rowIdx[row]] = part.impactMS
		}
	}
	vote.winner = roster[best].ref.carrierNode()
	if best < owned && vote.carrier >= 0 {
		vote.winner = model.SelfRows[vote.carrier].Node
	}
	vote.named = true
	return vote
}
