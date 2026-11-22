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
// state family, the seat's type token): the same rank board (target,
// parameters and window) and the same ranked thread (TID and TGID of the rank
// member).
func traceCausalProjectionSameStateAccount(a, b TraceCausalProjectionNode) bool {
	if a.RankSubjects == nil || b.RankSubjects == nil {
		return false
	}
	am, bm := a.RankSubjects.Member, b.RankSubjects.Member
	if am.TID <= 0 || am.TID != bm.TID || am.TGID != bm.TGID {
		return false
	}
	boardA, okA := traceSameFactRankBoardOf(a)
	boardB, okB := traceSameFactRankBoardOf(b)
	return okA && okB && boardA.sameBoard(boardB)
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
// never as a value. The row keeps its own rank identity, as every absorb
// survivor does.
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
		aggregate.MeasurementOrigins = ConcatTraceSchedulerMeasurementOrigins(aggregate.MeasurementOrigins, member.MeasurementOrigins)
		appendID(member.EvidenceID)
		for _, id := range member.MergedEvidenceIDs {
			appendID(id)
		}
	}
	return aggregate
}
