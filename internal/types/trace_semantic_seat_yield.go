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
