package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestActionCapabilitiesExposeDependencyRanks(t *testing.T) {
	cases := map[dataquery.DataActionKind]int{
		dataquery.DataActionDeriveRules:       1,
		dataquery.DataActionDeriveFields:      2,
		dataquery.DataActionNormalizeEntities: 2,
		dataquery.DataActionJoinRecords:       3,
		dataquery.DataActionComputeContribs:   4,
		dataquery.DataActionReconcile:         5,
		dataquery.DataActionAssembleAnswer:    6,
	}
	for kind, want := range cases {
		if got := DependencyRank(kind); got != want {
			t.Fatalf("DependencyRank(%s) = %d, want %d", kind, got, want)
		}
	}
}

func TestActionCapabilitiesExposeLedgerOutputs(t *testing.T) {
	if !ProducesLedger(dataquery.DataActionDeriveRules, LedgerRuleCoverage) {
		t.Fatal("derive_rules should produce rule coverage")
	}
	if !ProducesLedger(dataquery.DataActionQualifyRecords, LedgerDecisions) {
		t.Fatal("qualify_records should produce decisions")
	}
	if !ProducesLedger(dataquery.DataActionComputeContribs, LedgerContributions) {
		t.Fatal("compute_contributions should produce contributions")
	}
	if !ProducesLedger(dataquery.DataActionReconcile, LedgerReconcile) {
		t.Fatal("reconcile_artifacts should produce reconcile")
	}
	if ProducesLedger(dataquery.DataActionFilterRecords, LedgerContributions) {
		t.Fatal("filter_records should not be marked as producing contributions")
	}
}

func TestActionCapabilitiesMarkCustomTransformAsLeafFallback(t *testing.T) {
	if !IsLeafFallback(dataquery.DataActionCustomTransform) {
		t.Fatal("custom_transform should be marked as leaf fallback")
	}
	if IsLeafFallback(dataquery.DataActionComputeContribs) {
		t.Fatal("typed compute action should not be leaf fallback")
	}
	if NormalizeActionKind("") != dataquery.DataActionCustomTransform {
		t.Fatal("empty action kind should normalize to custom_transform for legacy planner safety")
	}
}

func TestActionCapabilitiesExposeInputPathContracts(t *testing.T) {
	contract, ok := InputPathContract(dataquery.DataActionDeriveFields)
	if !ok {
		t.Fatal("derive_fields should expose an input path contract")
	}
	if contract.Min != 1 || contract.Max != 1 || !contract.SingleRecordSet {
		t.Fatalf("derive_fields input contract=%+v, want one single-record-set input", contract)
	}
	if !RequiresInputPaths(dataquery.DataActionComputeContribs) {
		t.Fatal("compute_contributions should require an input path")
	}
	if !SingleRecordSetInput(dataquery.DataActionFilterRecords) {
		t.Fatal("filter_records should be a single-record-set action")
	}
	if _, ok := InputPathContract(dataquery.DataActionReconcile); ok {
		t.Fatal("reconcile_artifacts should not require direct input paths")
	}
}

func TestSplitFirstDependencyRankKeepsDiscoveryWithFirstRank(t *testing.T) {
	actions := []dataquery.DataAction{
		{ID: "inventory", Kind: dataquery.DataActionMaterialInventory},
		{ID: "extract", Kind: dataquery.DataActionExtractRecords},
		{ID: "rules", Kind: dataquery.DataActionDeriveRules},
		{ID: "normalize", Kind: dataquery.DataActionNormalizeEntities},
	}
	prefix, remainder, split := SplitFirstDependencyRank(actions)
	if !split {
		t.Fatal("split=false, want initial multi-rank DAG split")
	}
	if len(prefix) != 3 || prefix[2].ID != "rules" {
		t.Fatalf("prefix=%+v, want discovery plus first non-zero rank", prefix)
	}
	if len(remainder) != 1 || remainder[0].ID != "normalize" {
		t.Fatalf("remainder=%+v, want later rank", remainder)
	}
}

func TestSplitFirstDependencyRankDoesNotSplitSameRank(t *testing.T) {
	actions := []dataquery.DataAction{
		{ID: "derive_a", Kind: dataquery.DataActionDeriveFields},
		{ID: "derive_b", Kind: dataquery.DataActionDeriveFields},
	}
	_, _, split := SplitFirstDependencyRank(actions)
	if split {
		t.Fatal("same dependency rank should not split")
	}
}

func TestSplitInitialDiscoveryDependencyRankRequiresDiscoveryPrefix(t *testing.T) {
	actions := []dataquery.DataAction{
		{ID: "rules", Kind: dataquery.DataActionDeriveRules},
		{ID: "reconcile", Kind: dataquery.DataActionReconcile},
	}
	_, _, split := SplitInitialDiscoveryDependencyRank(actions)
	if split {
		t.Fatal("initial discovery rank split should not hide more precise prerequisite checks")
	}
}
