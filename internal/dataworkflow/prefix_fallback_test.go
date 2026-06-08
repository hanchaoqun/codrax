package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestIntraBatchDependencyPrefixFallbackSplitsProducerBeforeConsumer(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:             "extract",
		Kind:           dataquery.DataActionExtractRecords,
		OutputArtifact: "records.json",
	}, {
		ID:         "derive",
		Kind:       dataquery.DataActionDeriveFields,
		InputPaths: []string{"records.json"},
	}}}
	prefix, remainder, ok := IntraBatchDependencyPrefixFallback(plan)
	if !ok {
		t.Fatal("fallback ok=false")
	}
	if len(prefix.Actions) != 1 || prefix.Actions[0].ID != "extract" || !prefix.ContinueAfter {
		t.Fatalf("prefix=%+v", prefix)
	}
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "derive" || !remainder.ContinueAfter {
		t.Fatalf("remainder=%+v", remainder)
	}
}

func TestStagePrefixFallbackWithRemainderUsesAllowedRank(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "derive",
		Kind: dataquery.DataActionDeriveFields,
	}, {
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}
	prefix, remainder, ok := StagePrefixFallbackWithRemainder(StagePrefixFallbackInput{
		Plan:                plan,
		Guard:               NewGuardResult("blocked", "error", RepairNeedsTypedAction, "blocked"),
		HasSuccessfulResult: true,
		State: WorkflowStateView{
			AllowedNextActions:         []string{string(dataquery.DataActionDeriveFields), string(dataquery.DataActionComputeContribs)},
			ContributionLedgerRequired: true,
			ReconcileRequired:          true,
		},
	})
	if !ok {
		t.Fatal("fallback ok=false")
	}
	if len(prefix.Actions) != 1 || prefix.Actions[0].ID != "derive" {
		t.Fatalf("prefix=%+v", prefix)
	}
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "compute" {
		t.Fatalf("remainder=%+v", remainder)
	}
}

func TestExecutablePrefixFallbackUsesStagingGuard(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "a"}, {ID: "b"}, {ID: "bad"}}}
	prefix, ok := ExecutablePrefixFallback(ExecutablePrefixFallbackInput{
		Plan:  plan,
		Guard: NewGuardResult("blocked", "error", RepairNeedsTypedAction, "blocked"),
		StagingGuard: func(candidate dataquery.TaskPlan) GuardResult {
			if len(candidate.Actions) >= 3 {
				return NewGuardResult("bad_suffix", "error", RepairNeedsTypedAction, "bad suffix")
			}
			return GuardResult{}
		},
	})
	if !ok {
		t.Fatal("fallback ok=false")
	}
	if len(prefix.Actions) != 2 || prefix.Actions[1].ID != "b" || !prefix.ContinueAfter {
		t.Fatalf("prefix=%+v", prefix)
	}
}
