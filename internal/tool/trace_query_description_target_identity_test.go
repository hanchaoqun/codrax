package tool

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// Only the inherited-target cardinality teaching changed. Reverse this exact
// independently literal delta before every older byte/hash comparison.
func traceQueryDescriptionBeforeTargetIdentityEvolution(t *testing.T, description string) string {
	t.Helper()
	const prior = "if omitted and the structured request model exposes exactly one runtime_targets entry, trace_query inherits only that typed pid/thread"
	const current = "if omitted and the structured request model exposes one unambiguous typed target after canonical identity comparison, trace_query inherits only that typed pid/thread (same-kind numeric IDs remain one target when a display name is omitted; different IDs or process/thread identities remain ambiguous)"
	if strings.Count(description, current) != 1 || strings.Contains(description, prior) {
		t.Fatal("target identity teaching must be the unique approved correction")
	}
	return strings.Replace(description, current, prior, 1)
}

func TestTraceQueryDescriptionTargetIdentityOnlyEvolution(t *testing.T) {
	prior := traceQueryDescriptionBeforeTargetIdentityEvolution(t, (&TraceQuery{}).Description())
	prior = traceQueryDescriptionBeforeBusinessTreeOwnerPriorityEvolution(t, prior)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got != "8e04b6fba800ed378a871668b299a32aaa44ef9a4eb494246fea0c62e2194c5c" {
		t.Fatalf("description changed beyond typed-target identity correction: %s", got)
	}
}
