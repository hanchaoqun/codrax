package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestActionNodeForProjectsRankLedgersAndIdempotency(t *testing.T) {
	action := dataquery.DataAction{
		ID:             "compute_totals",
		Kind:           dataquery.DataActionComputeContribs,
		InputPaths:     []string{"records.json", "records.json"},
		OutputArtifact: "contribs.json",
		Params: map[string]string{
			"value_field": "amount",
			"group_key":   "category",
		},
	}
	node := ActionNodeFor(action, ActionStatusReady)
	if node.Kind != string(dataquery.DataActionComputeContribs) || node.DependencyRank != 4 {
		t.Fatalf("node=%+v, want compute_contributions rank 4", node)
	}
	if len(node.InputAliases) != 1 || node.InputAliases[0] != "records.json" {
		t.Fatalf("InputAliases=%v, want deduped records.json", node.InputAliases)
	}
	if node.OutputAlias != "contribs.json" || node.IdempotencyKey == "" {
		t.Fatalf("node=%+v, want output alias and idempotency key", node)
	}
	if len(node.CanProduceLedger) == 0 {
		t.Fatalf("CanProduceLedger=%v, want ledger capability", node.CanProduceLedger)
	}
}

func TestActionIdempotencyKeyIsStableAcrossParamOrder(t *testing.T) {
	a := dataquery.DataAction{
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "filtered.json",
		Params:         map[string]string{"b": "2", "a": "1"},
	}
	b := dataquery.DataAction{
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "filtered.json",
		Params:         map[string]string{"a": "1", "b": "2"},
	}
	if ActionIdempotencyKey(a) != ActionIdempotencyKey(b) {
		t.Fatalf("idempotency key should be stable across map iteration order")
	}
	b.Params["b"] = "3"
	if ActionIdempotencyKey(a) == ActionIdempotencyKey(b) {
		t.Fatalf("idempotency key should change when structural params change")
	}
}
