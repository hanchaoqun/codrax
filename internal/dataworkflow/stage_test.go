package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestNextStageFollowsLedgerProgression(t *testing.T) {
	tests := []struct {
		name  string
		facts StageFacts
		want  string
	}{
		{name: "needs material coverage", facts: StageFacts{}, want: StageCoverRequiredMaterials},
		{name: "needs rules", facts: StageFacts{MaterialCoverageSufficient: true, RuleCoverageRequired: true}, want: StageDeriveRules},
		{name: "needs entity resolution", facts: StageFacts{MaterialCoverageSufficient: true, EntityResolutionRequired: true}, want: StageNormalizeOrEnrichEntities},
		{name: "needs contributions", facts: StageFacts{MaterialCoverageSufficient: true, EntityResolutionRequired: true, EntityStageMaterialized: true, ContributionLedgerRequired: true}, want: StagePrepareContributionInputs},
		{name: "needs reconcile", facts: StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true}, want: StageReconcileArtifacts},
		{name: "needs answer projection", facts: StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true, HasReconcile: true}, want: StageEmitOutputContractAnswer},
		{name: "complete", facts: StageFacts{MaterialCoverageSufficient: true, ContributionLedgerRequired: true, ContributionRecords: 1, ReconcileRequired: true, HasReconcile: true, HasAnswer: true}, want: StageComplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextStage(tt.facts); got != tt.want {
				t.Fatalf("NextStage=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestAllowedNextActionContractsLiveInWorkflowIR(t *testing.T) {
	contracts := AllowedNextActionContracts(StagePrepareContributionInputs)
	kinds := strings.Join(ActionKindsFromContracts(contracts), ",")
	for _, want := range []string{
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionQualifyRecords),
		string(dataquery.DataActionComputeContribs),
	} {
		if !strings.Contains(kinds, want) {
			t.Fatalf("allowed kinds=%s, want %s", kinds, want)
		}
	}
	filtered := strings.Join(ActionKindsFromContracts(FilterCustomTransformContracts(AllowedNextActionContracts(StageNormalizeOrEnrichEntities))), ",")
	if strings.Contains(filtered, string(dataquery.DataActionCustomTransform)) {
		t.Fatalf("filtered contracts still include custom_transform: %s", filtered)
	}
}

func TestMissingValidationStagesUseTypedFacts(t *testing.T) {
	got := strings.Join(MissingValidationStages(StageFacts{
		MaterialCoverageSufficient: true,
		RuleCoverageRequired:       true,
		DecisionRecordsRequired:    true,
		EntityResolutionRequired:   true,
		ContributionLedgerRequired: true,
		ReconcileRequired:          true,
	}), ",")
	want := "rule_coverage,entity_resolution,decision_records,contribution_ledger,reconcile,final_answer"
	if got != want {
		t.Fatalf("MissingValidationStages=%q, want %q", got, want)
	}
}

func TestTerminalWorkflowGuardResultUsesSharedStageFacts(t *testing.T) {
	guard := TerminalWorkflowGuardResult("complete", StageFacts{
		MaterialCoverageSufficient: true,
		ContributionLedgerRequired: true,
	}, []string{string(dataquery.DataActionComputeContribs)})
	if guard.Code != "unfinished_validation_stage" || len(guard.Violations) != 1 {
		t.Fatalf("guard=%+v, want unfinished validation guard", guard)
	}
	if !strings.Contains(guard.Message, "terminal status") || !strings.Contains(guard.Message, "compute_contributions") {
		t.Fatalf("message=%q, want terminal status and legal action hint", guard.Message)
	}
}
