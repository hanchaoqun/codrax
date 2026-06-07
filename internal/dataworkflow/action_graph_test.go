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

func TestActionGraphNormalizesRolePathsBeforeProjection(t *testing.T) {
	raw := dataquery.DataAction{
		Kind: dataquery.DataActionNormalizeEntities,
		Params: map[string]string{
			"source_path":    "records.json",
			"reference_path": "reference.json",
		},
	}
	normalized := raw
	normalized.InputPaths = []string{"records.json", "reference.json"}

	node := ActionNodeFor(raw, ActionStatusReady)
	if len(node.InputAliases) != 2 || node.InputAliases[0] != "records.json" || node.InputAliases[1] != "reference.json" {
		t.Fatalf("InputAliases=%v, want role paths projected as action inputs", node.InputAliases)
	}
	if ActionIdempotencyKey(raw) != ActionIdempotencyKey(normalized) {
		t.Fatalf("idempotency key should be stable between raw role params and normalized input_paths")
	}
}

func TestReduceActionGraphProjectsEventsReadyAndLimit(t *testing.T) {
	events := []ActionEvent{
		{Actions: []dataquery.DataAction{{ID: "old", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"old.csv"}}}, Status: ActionStatusExecuted},
		{Actions: []dataquery.DataAction{{ID: "failed", Kind: dataquery.DataActionFilterRecords, InputPaths: []string{"records.json"}}}, Status: ActionStatusFailed},
	}
	current := []dataquery.DataAction{{
		ID:   "normalize",
		Kind: dataquery.DataActionNormalizeEntities,
		Params: map[string]string{
			"source_path":    "records.json",
			"reference_path": "reference.json",
		},
	}}

	graph := ReduceActionGraph(events, current, 1)
	if len(graph.Executed) != 1 || graph.Executed[0].ID != "failed" || graph.Executed[0].Status != ActionStatusFailed {
		t.Fatalf("Executed=%+v, want only latest failed event after limit", graph.Executed)
	}
	if len(graph.Ready) != 1 || graph.Ready[0].Status != ActionStatusReady {
		t.Fatalf("Ready=%+v, want one ready node", graph.Ready)
	}
	if len(graph.Ready[0].InputAliases) != 2 || graph.Ready[0].InputAliases[0] != "records.json" || graph.Ready[0].InputAliases[1] != "reference.json" {
		t.Fatalf("Ready input aliases=%v, want normalized role paths", graph.Ready[0].InputAliases)
	}
}
