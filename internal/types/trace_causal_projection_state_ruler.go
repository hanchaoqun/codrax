package types

import "strings"

// HMC377 round 12 (R11P-2, owner ruling R-377-12 ⑤): two rulers of one
// thread's same-state account are never summed.
//
// A rank board publishes the analysed thread's scheduler-state account on two
// rulers. The window_stats census seats partition the thread's whole
// in-window state account and add up to it; a wakeup_chain seat measures the
// same thread's same state on the chain, so its instants lie inside that
// account. The engine's own family fold keeps the two producers apart for this
// reason (tracequery foldSameThreadTypeRankFamilies, the ORD mint-source key),
// and the board's self two-ruler record says they never add. The projection's
// ×N same-kind merge keyed only on (subject, object), so once both rulers of
// one thread reached one group it summed them: 105 网易云音乐 容器重启后冷启动 #5
// (TID 43012, [2149.324822,2149.524822)) 169.059 + 0.934 census + 100.748
// wakeup_chain = 270.741 ms for a 200 ms window (raw ready 169.993).
//
// The merged node's value is the account ruler: its census members merge as
// before and a wakeup_chain seat of the same account joins the row as
// lossless evidence only. Identity is typed — the seat's mint source (the
// engine's rank item Source on the source note), the rank board identity and
// the ranked member's thread coordinate — never a value comparison.

type traceRankStateRuler uint8

const (
	traceRankStateRulerNone traceRankStateRuler = iota
	// traceRankStateRulerCensus is a window_stats census seat: a part of
	// the thread's whole in-window state account.
	traceRankStateRulerCensus
	// traceRankStateRulerWakeupChain is a wakeup_chain seat of the same
	// thread's state: a subset of the census account's instants.
	traceRankStateRulerWakeupChain
)

// The engine's mint-source tokens for the two rulers (tracequery
// rootCauseItem sources: "window_stats", and the wakeup_chain view with its
// sub-lanes such as "wakeup_chain.causal_impacts").
const (
	traceRankSourceWindowStats = "window_stats"
	traceRankSourceWakeupChain = "wakeup_chain"
)

// WakeupChainStateSeat reports a rank-board seat minted on the wakeup_chain
// ruler of its thread's state account (HMC377 round 13, R-377-13 ②): its
// instants lie inside the window_stats census account of the same thread and
// state, so it is another ruler of that account, never a part of it. The
// typed mint source the ×N merge reads (traceRankStateRulerOf) decides.
func (n TraceCausalProjectionNode) WakeupChainStateSeat() bool {
	return n.stateRuler == traceRankStateRulerWakeupChain
}

// traceRankStateRulerOf reads a rank-board record's ruler from its typed
// source note. Records outside the rank-board family carry no ruler.
func traceRankStateRulerOf(predicate string, notes []string) traceRankStateRuler {
	if !strings.HasPrefix(strings.TrimSpace(predicate), "root_cause_") {
		return traceRankStateRulerNone
	}
	switch source := strings.TrimSpace(traceCausalProjectionRichNoteValue(notes, TraceNoteKeySource)); {
	case source == traceRankSourceWindowStats:
		return traceRankStateRulerCensus
	case source == traceRankSourceWakeupChain || strings.HasPrefix(source, traceRankSourceWakeupChain+"."):
		return traceRankStateRulerWakeupChain
	}
	return traceRankStateRulerNone
}

// traceCausalProjectionSameStateAccount reports two seats of one thread's
// one state account inside one merge group (whose key already fixes the
// state family, the seat's type token): one rank board (target, parameters
// and window) published both, and they rank the same thread (TID and TGID of
// the rank member). A seat's boards are the one it wears and every board the
// same-fact merge recorded as a co-publisher, so the decision does not depend
// on which board's copy survived that merge (the order the boards were asked
// in).
func traceCausalProjectionSameStateAccount(a, b TraceCausalProjectionNode) bool {
	if a.RankSubjects == nil || b.RankSubjects == nil {
		return false
	}
	am, bm := a.RankSubjects.Member, b.RankSubjects.Member
	if am.TID <= 0 || am.TID != bm.TID || am.TGID != bm.TGID {
		return false
	}
	for _, boardA := range traceCausalProjectionPublishingRankBoards(a) {
		for _, boardB := range traceCausalProjectionPublishingRankBoards(b) {
			if boardA.sameBoard(boardB) {
				return true
			}
		}
	}
	return false
}

// traceCausalProjectionPublishingRankBoards is every rank board that published
// the account a row displays: the board it wears and its recorded
// co-publishers.
func traceCausalProjectionPublishingRankBoards(node TraceCausalProjectionNode) []TraceSameFactRankBoard {
	var boards []TraceSameFactRankBoard
	if own, ok := traceSameFactRankBoardOf(node); ok {
		boards = append(boards, own)
	}
	return append(boards, node.CoPublishedRankBoards()...)
}

// traceCausalProjectionStateRulerSplit separates the members of one merge
// group into the account members (every member that is not a wakeup_chain
// seat of a census account in the group) and the wakeup_chain seats whose
// census account is also a member. subset is nil when the group holds no
// such pair, and then account is members unchanged.
func traceCausalProjectionStateRulerSplit(nodes []TraceCausalProjectionNode, members []int) (account, subset []int) {
	inside := map[int]bool{}
	for _, m := range members {
		if nodes[m].stateRuler != traceRankStateRulerWakeupChain {
			continue
		}
		for _, c := range members {
			if nodes[c].stateRuler == traceRankStateRulerCensus && traceCausalProjectionSameStateAccount(nodes[c], nodes[m]) {
				inside[m] = true
				break
			}
		}
	}
	if len(inside) == 0 {
		return members, nil
	}
	for _, idx := range members {
		if inside[idx] {
			subset = append(subset, idx)
		} else {
			account = append(account, idx)
		}
	}
	return account, subset
}

// traceCausalProjectionMergeStateAccountRuler is the merge of a group that
// holds both rulers of one account: the account members merge through the one
// merge body, seeded on the first account member (a single account member is
// the row itself), and each wakeup_chain seat inside that account joins as
// lossless evidence — its id, its own merged ids and its measurement origins —
// never as a value. The row carries the seat it absorbed, so the seat's
// ordinal stays on it (R-377-13 ③ badges follow their seat): a seat with a
// smaller ordinal supplies the row's Rank and its board/window identity
// through the ×N merge's one rank-supplier rule
// (traceCausalProjectionAdoptRankSupplier).
//
// EVOLUTION RECORD (HMC377 round 13, R-377-13 ③, review-12 R12P-3): old → new
// = the row kept the account members' own best ordinal and the absorbed seat's
// ordinal was published nowhere → the smaller of the two is the row's
// ordinal, as on every ×N merge. 14 real windows lost a TOP5 badge
// (n13w #ActionReaper-65009 ➌ absorbed into the #6 census row, n04w..w42w1 ➋,
// x10w..x14w): ~/hmc-work/hmc377/fix12/review-population/evidence/D3_badges.txt.
func traceCausalProjectionMergeStateAccountRuler(nodes []TraceCausalProjectionNode, account, subset []int, backgroundLane bool) TraceCausalProjectionNode {
	aggregate := nodes[account[0]]
	if len(account) > 1 {
		aggregate = traceCausalProjectionMergeSameKindMemberValues(nodes, account[0], account, backgroundLane)
	}
	aggregate.MergedEvidenceIDs = append([]string(nil), aggregate.MergedEvidenceIDs...)
	absorbed := map[string]bool{traceCausalProjectionCanonicalNode(aggregate.EvidenceID): true}
	for _, id := range aggregate.MergedEvidenceIDs {
		absorbed[traceCausalProjectionCanonicalNode(id)] = true
	}
	appendID := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || absorbed[traceCausalProjectionCanonicalNode(id)] {
			return
		}
		absorbed[traceCausalProjectionCanonicalNode(id)] = true
		aggregate.MergedEvidenceIDs = append(aggregate.MergedEvidenceIDs, id)
	}
	for _, idx := range subset {
		member := nodes[idx]
		traceCausalProjectionAdoptRankSupplier(&aggregate, member)
		aggregate.MeasurementOrigins = ConcatTraceSchedulerMeasurementOrigins(aggregate.MeasurementOrigins, member.MeasurementOrigins)
		appendID(member.EvidenceID)
		for _, id := range member.MergedEvidenceIDs {
			appendID(id)
		}
	}
	return aggregate
}
