package types

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func traceSemanticSeatYieldForTest(t *testing.T) {
	t.Helper()
	previous := traceSemanticSeatYieldingSpan.Load()
	RegisterTraceSemanticSeatYieldingSpan(func(name string) bool { return strings.HasPrefix(name, "vocabulary") })
	t.Cleanup(func() { traceSemanticSeatYieldingSpan.Store(previous) })
}

func traceSemanticSeatNode(subject, span, relevance string) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{Subject: subject, SpanName: span, ChainRelevance: relevance, ImpactMS: 1}
}

func traceSemanticSeatSubjects(nodes []TraceCausalProjectionNode) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.Subject)
	}
	return out
}

func TestTraceCausalProjectionNodeYieldsBoundedSeatOnlyOffChain(t *testing.T) {
	traceSemanticSeatYieldForTest(t)
	merged := traceSemanticSeatNode("m", "vocabulary wait", "adjacent")
	merged.MergedCount = 3
	selfDeterministic := traceSemanticSeatNode("s", "vocabulary wait", "")
	selfDeterministic.Causality = "self_deterministic"
	for _, c := range []struct {
		label string
		node  TraceCausalProjectionNode
		want  bool
	}{
		{"off-chain vocabulary row", traceSemanticSeatNode("a", "vocabulary wait", "adjacent"), true},
		{"background vocabulary row", traceSemanticSeatNode("b", "vocabulary wait", "background"), true},
		{"on-chain vocabulary row", traceSemanticSeatNode("c", "vocabulary wait", "on_chain"), false},
		{"the target's own work", selfDeterministic, false},
		{"another off-chain row", traceSemanticSeatNode("d", "VerifyClass x", "background"), false},
		{"a merged row", merged, false},
		{"a row without a span", traceSemanticSeatNode("e", "", "background"), false},
	} {
		if got := TraceCausalProjectionNodeYieldsBoundedSeat(c.node); got != c.want {
			t.Errorf("%s: %v, want %v", c.label, got, c.want)
		}
	}
}

func TestTraceCausalProjectionWithoutThePredicateNothingYields(t *testing.T) {
	previous := traceSemanticSeatYieldingSpan.Load()
	traceSemanticSeatYieldingSpan.Store(nil)
	t.Cleanup(func() { traceSemanticSeatYieldingSpan.Store(previous) })
	if TraceCausalProjectionNodeYieldsBoundedSeat(traceSemanticSeatNode("a", "vocabulary wait", "adjacent")) {
		t.Fatal("no predicate registered: no row can be identified")
	}
}

// Off-chain capacity goes to every other row first; yielding rows take what is
// left, after them. On-chain rows are never bounded.
func TestTraceCausalProjectionSemanticSpansYieldOffChainCapacity(t *testing.T) {
	traceSemanticSeatYieldForTest(t)
	nodes := []TraceCausalProjectionNode{
		traceSemanticSeatNode("chain", "vocabulary wait", "on_chain"),
		traceSemanticSeatNode("v1", "vocabulary wait", "adjacent"),
		traceSemanticSeatNode("b1", "VerifyClass a", "adjacent"),
		traceSemanticSeatNode("v2", "vocabulary wait", "background"),
		traceSemanticSeatNode("b2", "VerifyClass b", "background"),
		traceSemanticSeatNode("b3", "VerifyClass c", "background"),
	}
	for limit, want := range map[int][]string{
		2: {"chain", "b1", "b2"},
		3: {"chain", "b1", "b2", "b3"},
		4: {"chain", "b1", "b2", "b3", "v1"},
		5: {"chain", "b1", "b2", "b3", "v1", "v2"},
	} {
		if got := traceSemanticSeatSubjects(traceCausalProjectionSelectSemanticSpans(nodes, limit)); !reflect.DeepEqual(got, want) {
			t.Errorf("limit %d: got %v, want %v", limit, got, want)
		}
	}
}

// Inside a context bucket a yielding row follows even a smaller value seat, so
// the bucket cap and its fold reach it first.
func TestTraceCausalProjectionContextBucketSeatsYieldingRowsLast(t *testing.T) {
	traceSemanticSeatYieldForTest(t)
	seat := traceSemanticSeatNode("seat", "VerifyClass a", "background")
	seat.Rank, seat.ImpactMS = 1, 2
	yielding := traceSemanticSeatNode("yield", "vocabulary wait", "background")
	yielding.Rank, yielding.ImpactMS = 2, 900
	context := traceSemanticSeatNode("context", "", "background")
	got := traceSemanticSeatSubjects(traceCausalProjectionSortContextBucket([]TraceCausalProjectionNode{yielding, context, seat}, nil))
	if !reflect.DeepEqual(got, []string{"seat", "context", "yield"}) {
		t.Fatalf("bucket order %v", got)
	}
}

func TestTraceCausalProjectionBoundedSeatLimitAddsOnChainVocabularyRows(t *testing.T) {
	traceSemanticSeatYieldForTest(t)
	nodes := []TraceCausalProjectionNode{
		traceSemanticSeatNode("a", "vocabulary wait", "on_chain"),
		traceSemanticSeatNode("b", "vocabulary work", "on_chain"),
		traceSemanticSeatNode("c", "vocabulary wait", "background"),
		traceSemanticSeatNode("d", "VerifyClass x", "on_chain"),
	}
	for limit, want := range map[int]int{8: 10, 1: 3, 0: 0, -1: -1} {
		if got := TraceCausalProjectionBoundedSeatLimit(nodes, limit); got != want {
			t.Errorf("limit %d: got %d, want %d", limit, got, want)
		}
	}
	ordered := TraceCausalProjectionYieldingNodesLast(nodes)
	if got := fmt.Sprint(traceSemanticSeatSubjects(ordered)); got != "[a b d c]" {
		t.Fatalf("order %s", got)
	}
}
