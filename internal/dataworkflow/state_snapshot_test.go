package dataworkflow

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestWorkflowStateSnapshotDerivesStageAndAllowedActions(t *testing.T) {
	snapshot := WorkflowStateSnapshot{
		StageFacts: StageFacts{
			MaterialCoverageSufficient: true,
			ContributionLedgerRequired: true,
			EntityStageMaterialized:    true,
		},
	}
	if snapshot.NextStage() != StagePrepareContributionInputs {
		t.Fatalf("NextStage=%q, want %q", snapshot.NextStage(), StagePrepareContributionInputs)
	}
	kinds := strings.Join(snapshot.AllowedNextActions(), ",")
	for _, want := range []string{
		string(dataquery.DataActionDeriveFields),
		string(dataquery.DataActionFilterRecords),
		string(dataquery.DataActionComputeContribs),
	} {
		if !strings.Contains(kinds, want) {
			t.Fatalf("AllowedNextActions=%q, want %q", kinds, want)
		}
	}
}
