package types

import "sync/atomic"

// A span classified only by the platform marker vocabulary is new evidence. It
// may fill a bounded face, but off the dependency chain of the analysed target
// it never takes a seat from a row that would have been published without it.
// The vocabulary and the classifier live in the trace engine, which depends on
// this package; the engine registers the one predicate here. Nothing about the
// row travels on the wire, so persisted ledgers and replays order identically.
var traceSemanticSeatYieldingSpan atomic.Pointer[func(spanName string) bool]

// RegisterTraceSemanticSeatYieldingSpan installs the engine's predicate: the
// named span's semantic class comes only from the platform marker vocabulary.
func RegisterTraceSemanticSeatYieldingSpan(predicate func(spanName string) bool) {
	traceSemanticSeatYieldingSpan.Store(&predicate)
}

// traceCausalProjectionNodeVocabularyClassified reports a row of
// vocabulary-classified work. A merged row never is one: it can carry rows that
// were published without the vocabulary.
func traceCausalProjectionNodeVocabularyClassified(node TraceCausalProjectionNode) bool {
	predicate := traceSemanticSeatYieldingSpan.Load()
	return predicate != nil && node.SpanName != "" && node.MergedCount <= 1 && (*predicate)(node.SpanName)
}

// TraceCausalProjectionNodeYieldsBoundedSeat reports that the node is an
// off-chain row of vocabulary-classified work. A bounded face seats such a row
// after every other row.
func TraceCausalProjectionNodeYieldsBoundedSeat(node TraceCausalProjectionNode) bool {
	return !traceCausalProjectionNodeOnChain(node) && traceCausalProjectionNodeVocabularyClassified(node)
}

// TraceCausalProjectionBoundedSeatLimit returns the capacity of a bounded face
// over nodes: on-chain rows of vocabulary-classified work are additional to
// limit, so no row that would have been published without the vocabulary loses
// its seat to one of them. A limit of zero or less stays unbounded.
func TraceCausalProjectionBoundedSeatLimit(nodes []TraceCausalProjectionNode, limit int) int {
	if limit <= 0 {
		return limit
	}
	for _, node := range nodes {
		if traceCausalProjectionNodeOnChain(node) && traceCausalProjectionNodeVocabularyClassified(node) {
			limit++
		}
	}
	return limit
}

// TraceCausalProjectionYieldingNodesLast returns nodes with the yielding rows
// moved behind all others; both groups keep their relative order.
func TraceCausalProjectionYieldingNodesLast(nodes []TraceCausalProjectionNode) []TraceCausalProjectionNode {
	var kept, yielding []TraceCausalProjectionNode
	for _, node := range nodes {
		if TraceCausalProjectionNodeYieldsBoundedSeat(node) {
			yielding = append(yielding, node)
		} else {
			kept = append(kept, node)
		}
	}
	if len(yielding) == 0 {
		return nodes
	}
	return append(kept, yielding...)
}

// A classified marker also says what its thread was doing: performing the
// work, or parked until work on another thread ended. The engine's classifier
// decides that once, as the stance of the marker name, and registers the one
// predicate here for the same reason as the seat predicate above: the answer
// is a pure function of the record's span name, so nothing about it travels
// on the wire and persisted ledgers replay identically.
var traceSemanticSpanRecordsWait atomic.Pointer[func(spanName string) bool]

// RegisterTraceSemanticSpanRecordsWait installs the engine's predicate: the
// named span records its thread waiting for other work.
func RegisterTraceSemanticSpanRecordsWait(predicate func(spanName string) bool) {
	traceSemanticSpanRecordsWait.Store(&predicate)
}

// TraceCausalProjectionNodeRecordsWait reports a row that records its thread
// waiting for other work: its value is waiting time, and no work the thread
// executed. Every reader lane asks this one function.
//
// A row of one record is decided by the engine's stance of its marker name. A
// merged row is decided by the rows it merged and never by a name: the name
// it carries is one member's, and a cross-thread fold carries none. Each merge
// decides it from all its members (MergedRecordsWait).
func TraceCausalProjectionNodeRecordsWait(node TraceCausalProjectionNode) bool {
	if node.MergedRecordsWait {
		return true
	}
	if node.MergedCount > 1 {
		return false
	}
	predicate := traceSemanticSpanRecordsWait.Load()
	return predicate != nil && (*predicate)(node.SpanName)
}

// TraceCausalProjectionMembersRecordWait is the stance of a row merged from
// members: a row of waits only when every member records a wait. A merged
// member answers with its own decided stance.
func TraceCausalProjectionMembersRecordWait(members []TraceCausalProjectionNode) bool {
	for _, member := range members {
		if !TraceCausalProjectionNodeRecordsWait(member) {
			return false
		}
	}
	return len(members) > 0
}
