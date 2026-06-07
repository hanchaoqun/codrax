package dataworkflow

import (
	"strings"
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

func TestReduceActionGraphStateProjectsDeferredAndBlocked(t *testing.T) {
	deferred := []dataquery.DataAction{{
		ID:         "join_later",
		Kind:       dataquery.DataActionJoinRecords,
		InputPaths: []string{"left.json", "right.json"},
	}}
	blockedAction := dataquery.DataAction{
		ID:             "filter_bad",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "filtered.json",
	}
	violation := NewActionInputViolation(
		"field_contract_violation",
		"error",
		RepairNeedsTypedAction,
		blockedAction,
		"records.json",
		[]string{"status"},
		"missing status",
		nil,
	)

	graph := ReduceActionGraphState(ActionGraphInput{
		Deferred: deferred,
		Blocked:  []WorkflowViolation{violation},
	})
	if len(graph.Deferred) != 1 || graph.Deferred[0].Status != ActionStatusDeferred || graph.Deferred[0].ID != "join_later" {
		t.Fatalf("Deferred=%+v, want deferred join node", graph.Deferred)
	}
	if len(graph.Blocked) != 1 || graph.Blocked[0].Status != ActionStatusBlocked || graph.Blocked[0].IdempotencyKey == "" {
		t.Fatalf("Blocked=%+v, want typed blocked node", graph.Blocked)
	}
}

func TestBlockedActionNodesFromViolationsProjectsTypedRepairState(t *testing.T) {
	nodes := BlockedActionNodesFromViolations([]WorkflowViolation{{
		Code:          "field_contract_violation",
		ActionID:      "filter",
		ActionKind:    string(dataquery.DataActionFilterRecords),
		InputAlias:    "records.json",
		MissingFields: []string{"status"},
		Reason:        "missing status field",
	}})

	if len(nodes) != 1 {
		t.Fatalf("nodes=%+v, want one blocked node", nodes)
	}
	got := nodes[0]
	if got.Status != ActionStatusBlocked || got.ID != "filter" || got.Kind != string(dataquery.DataActionFilterRecords) {
		t.Fatalf("node=%+v, want blocked filter node", got)
	}
	if len(got.InputAliases) != 1 || got.InputAliases[0] != "records.json" {
		t.Fatalf("InputAliases=%v", got.InputAliases)
	}
	if got.IdempotencyKey == "" || !strings.Contains(got.BlockedReason, "field_contract_violation") {
		t.Fatalf("node=%+v, want stable blocked key and reason", got)
	}
}
