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
