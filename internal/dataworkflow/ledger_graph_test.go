package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildLedgerGraphExposesRequiredDependencyContract(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	})
	if graph.NextStage != StageDeriveRules {
		t.Fatalf("NextStage=%q, want %q", graph.NextStage, StageDeriveRules)
	}
	if graph.FirstMissing != string(LedgerRuleCoverage) {
		t.Fatalf("FirstMissing=%q, want rule coverage", graph.FirstMissing)
	}
	if graph.RuleCoverage.Status != LedgerStatusMissing || !containsString(graph.RuleCoverage.ProducesActions, string(dataquery.DataActionDeriveRules)) {
		t.Fatalf("RuleCoverage=%+v, want missing derive_rules producer", graph.RuleCoverage)
	}
	if graph.Contributions.Status != LedgerStatusBlockedByPrerequisite || !containsString(graph.Contributions.MissingPrerequisites, string(LedgerRuleCoverage)) {
		t.Fatalf("Contributions=%+v, want blocked by rule coverage", graph.Contributions)
	}
	if graph.Reconcile.Status != LedgerStatusBlockedByPrerequisite || !containsString(graph.Reconcile.MissingPrerequisites, string(LedgerContributions)) {
		t.Fatalf("Reconcile=%+v, want blocked by contributions", graph.Reconcile)
	}
}

func TestBuildLedgerGraphDoesNotRegressSatisfiedDownstreamLedgers(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		EntityResolutionRequired:   true,
		EntityResolutionRecords:    3,
		EntityStageMaterialized:    true,
		ContributionLedgerRequired: true,
		ContributionRecords:        2,
		ReconcileRequired:          true,
		HasReconcile:               true,
	})
	if graph.NextStage != StageDeriveRules {
		t.Fatalf("NextStage=%q, want catch-up derive_rules", graph.NextStage)
	}
	if graph.EntityResolutions.Status != LedgerStatusSatisfied || graph.Contributions.Status != LedgerStatusSatisfied || graph.Reconcile.Status != LedgerStatusSatisfied {
		t.Fatalf("graph=%+v, downstream ledgers should stay satisfied while rules catch up", graph)
	}
	if graph.FinalProjection.Status != LedgerStatusBlockedByPrerequisite || !containsString(graph.FinalProjection.MissingPrerequisites, string(LedgerRuleCoverage)) {
		t.Fatalf("FinalProjection=%+v, want blocked by missing rule coverage", graph.FinalProjection)
	}
}

func TestBuildLedgerGraphBlocksLedgersOnMissingMaterialFloor(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	})
	if graph.NextStage != StageCoverRequiredMaterials {
		t.Fatalf("NextStage=%q, want material coverage", graph.NextStage)
	}
	if graph.Contributions.Status != LedgerStatusBlockedByPrerequisite || !containsString(graph.Contributions.MissingPrerequisites, LedgerPrerequisiteMaterials) {
		t.Fatalf("Contributions=%+v, want blocked by material prerequisite", graph.Contributions)
	}
}

func TestLedgerGraphDependenciesAreOrderedAndActionable(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
		ContributionRecords:        1,
		ReconcileRequired:          true,
	})
	if graph.FirstMissing != string(LedgerReconcile) {
		t.Fatalf("FirstMissing=%q, want reconcile", graph.FirstMissing)
	}
	var ledgers []string
	for _, dep := range graph.Dependencies {
		ledgers = append(ledgers, dep.Ledger)
	}
	text := strings.Join(ledgers, ",")
	if !strings.Contains(text, "rule_coverage,decisions,entity_resolutions,contributions,reconcile,final_projection") {
		t.Fatalf("dependency order=%s", text)
	}
	if graph.Reconcile.Status != LedgerStatusMissing || !containsString(graph.Reconcile.ProducesActions, string(dataquery.DataActionReconcile)) {
		t.Fatalf("Reconcile=%+v, want actionable reconcile dependency", graph.Reconcile)
	}
}

func TestLedgerGraphCompletionGuardUsesTypedDependency(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
	})
	guard := LedgerGraphCompletionGuardResult(graph)
	if guard.Code != "missing_workflow_ledger" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want missing ledger guard", guard)
	}
	if guard.Violations[0].ActionKind != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("violation=%+v, want compute_contributions repair action", guard.Violations[0])
	}
}

func TestLedgerGraphCompletionGuardReportsFinalProjection(t *testing.T) {
	graph := BuildLedgerGraph(StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
		ContributionRecords:        1,
		ReconcileRequired:          true,
		HasReconcile:               true,
	})
	guard := LedgerGraphCompletionGuardResult(graph)
	if guard.Code != "missing_workflow_ledger" || !strings.Contains(guard.ErrorText(), "final answer projection") {
		t.Fatalf("guard=%+v, want missing final projection", guard)
	}
}

func TestLedgerGraphMirrorsTypedOnlyProjectionPrerequisites(t *testing.T) {
	facts := StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		RuleCoverageRecords:        3,
		CustomTransformDisabled:    true,
	}
	graph := BuildLedgerGraph(facts)
	if !graph.Contributions.Required || !graph.Reconcile.Required {
		t.Fatalf("typed-only projection did not publish operational ledgers: contributions=%+v reconcile=%+v", graph.Contributions, graph.Reconcile)
	}
	if graph.FirstMissing != string(LedgerContributions) || graph.NextStage != StagePrepareContributionInputs {
		t.Fatalf("graph first/stage=%q/%q, want contributions/%q", graph.FirstMissing, graph.NextStage, StagePrepareContributionInputs)
	}
	if !containsString(graph.FinalProjection.MissingPrerequisites, string(LedgerContributions)) ||
		!containsString(graph.FinalProjection.MissingPrerequisites, string(LedgerReconcile)) {
		t.Fatalf("final projection prerequisites=%v, want contribution and reconcile", graph.FinalProjection.MissingPrerequisites)
	}
	if len(graph.FinalProjection.ProducesActions) != 0 {
		t.Fatalf("final projection prematurely advertised producer=%v", graph.FinalProjection.ProducesActions)
	}

	facts.ContributionRecords = 2
	graph = BuildLedgerGraph(facts)
	if graph.FirstMissing != string(LedgerReconcile) || graph.NextStage != StageReconcileArtifacts {
		t.Fatalf("post-contribution graph first/stage=%q/%q, want reconcile/%q", graph.FirstMissing, graph.NextStage, StageReconcileArtifacts)
	}
	facts.HasReconcile = true
	graph = BuildLedgerGraph(facts)
	if graph.FirstMissing != string(LedgerFinalProjection) ||
		!containsString(graph.FinalProjection.ProducesActions, string(dataquery.DataActionAssembleAnswer)) {
		t.Fatalf("post-reconcile final projection=%+v first=%q", graph.FinalProjection, graph.FirstMissing)
	}
}

func TestLedgerGraphDoesNotCallUnlinkedLateRuleGenerationComplete(t *testing.T) {
	facts := StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		RuleCoverageRecords:        3,
		RuleLedgerLinkageRequired:  true,
		DecisionRecordsRequired:    true,
		DecisionRecords:            2,
		ContributionLedgerRequired: true,
		ContributionRecords:        2,
		ReconcileRequired:          true,
		HasReconcile:               true,
		HasAnswer:                  true,
	}
	graph := BuildLedgerGraph(facts)
	if graph.NextStage != StageComputeContributions {
		t.Fatalf("next_stage=%q, want %q", graph.NextStage, StageComputeContributions)
	}
	if graph.Decisions.Present || graph.Contributions.Present || graph.Reconcile.Present || graph.FinalProjection.Present {
		t.Fatalf("unlinked dependency was presented as satisfied: %+v", graph)
	}
	if graph.FirstMissing != string(LedgerDecisions) {
		t.Fatalf("first_missing=%q, want decisions dependency reopened", graph.FirstMissing)
	}
	facts.RuleLedgerLinkageSatisfied = true
	graph = BuildLedgerGraph(facts)
	if graph.NextStage != StageComplete || !graph.Decisions.Present || !graph.Contributions.Present || !graph.Reconcile.Present || !graph.FinalProjection.Present {
		t.Fatalf("linked graph did not close: %+v", graph)
	}
}
