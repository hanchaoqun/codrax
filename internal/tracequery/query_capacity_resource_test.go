package tracequery

import (
	"strings"
	"testing"
)

func TestNormalizeQueryClampsWakeupResourcesFromCapacityOnce(t *testing.T) {
	q := normalizeQuery(nil, Query{View: "wakeup_chain", MaxDepth: 999, MaxBranches: 888})
	wakeup := ViewCapacityFor("wakeup_chain")
	if q.MaxDepth != wakeup.MaxDepth || q.MaxBranches != wakeup.MaxBranches {
		t.Fatalf("normalized recursion resources = %d/%d, want capacity %d/%d", q.MaxDepth, q.MaxBranches, wakeup.MaxDepth, wakeup.MaxBranches)
	}
	q = normalizeQuery(nil, q)
	joined := strings.Join(q.normalizationCaveats, "\n")
	for _, token := range []string{"parameter=max_depth requested=999 effective=10", "parameter=max_branches requested=888 effective=8"} {
		if count := strings.Count(joined, token); count != 1 {
			t.Fatalf("clamp disclosure %q count = %d, want 1: %v", token, count, q.normalizationCaveats)
		}
	}
}

func TestWakeupChainDirectBuilderDisclosesCapacityClamp(t *testing.T) {
	chain := BuildWakeupChain(&Index{}, Query{View: "wakeup_chain", PID: 42, MaxDepth: 99, MaxBranches: 98})
	joined := strings.Join(chain.Caveats, "\n")
	for _, token := range []string{"parameter=max_depth requested=99 effective=10", "parameter=max_branches requested=98 effective=8"} {
		if !strings.Contains(joined, token) {
			t.Fatalf("direct wakeup-chain result omitted clamp disclosure %q: %+v", token, chain.Caveats)
		}
	}
}
