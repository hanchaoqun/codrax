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
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "derive" || remainder.ContinueAfter {
		t.Fatalf("remainder=%+v", remainder)
	}
}

func TestInitialRankPrefixFallbackMarksPrefixIntermediateAndPreservesTerminalSuffix(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:             "extract",
		Kind:           dataquery.DataActionExtractRecords,
		OutputArtifact: "records.json",
	}, {
		ID:             "derive",
		Kind:           dataquery.DataActionDeriveFields,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "derived.json",
	}, {
		ID:         "compute",
		Kind:       dataquery.DataActionComputeContribs,
		InputPaths: []string{"derived.json"},
	}}}
	prefix, remainder, ok := InitialRankPrefixFallback(InitialRankPrefixFallbackInput{Plan: plan})
	if !ok {
		t.Fatal("fallback ok=false")
	}
	if len(prefix.Actions) != 2 || !prefix.ContinueAfter {
		t.Fatalf("prefix=%+v, want intermediate extract/derive rank", prefix)
	}
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "compute" || remainder.ContinueAfter {
		t.Fatalf("remainder=%+v, want terminal compute suffix", remainder)
	}
}

func TestInitialRankPrefixFallbackKeepsTypedSuffixButReplansScript(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{
		{ID: "extract", Kind: dataquery.DataActionExtractRecords, InputPaths: []string{"users.json"}, OutputArtifact: "users"},
		{ID: "filter", Kind: dataquery.DataActionFilterRecords, InputPaths: []string{"users"}, OutputArtifact: "active"},
		{ID: "project", Kind: dataquery.DataActionCustomTransform, Script: "emit_result(json_records('active'))"},
		{ID: "after_script", Kind: dataquery.DataActionAssembleAnswer, InputPaths: []string{"project"}},
	}}
	prefix, remainder, ok := InitialRankPrefixFallback(InitialRankPrefixFallbackInput{Plan: plan})
	if !ok || len(prefix.Actions) != 2 || prefix.Actions[0].ID != "extract" || prefix.Actions[1].ID != "filter" {
		t.Fatalf("prefix=%+v ok=%v, want replay-safe typed prefix", prefix, ok)
	}
	if len(remainder.Actions) != 0 {
		t.Fatalf("remainder=%+v, actions after scripted boundary must be replanned", remainder)
	}
	if !prefix.ContinueAfter || !remainder.ContinueAfter {
		t.Fatalf("continue flags prefix=%v remainder=%v, want replan continuation", prefix.ContinueAfter, remainder.ContinueAfter)
	}
	if prefix.NextBatch != deferredScriptReplanInstruction || remainder.NextBatch != deferredScriptReplanInstruction {
		t.Fatalf("next_batch prefix=%q remainder=%q, want scripted replan instruction", prefix.NextBatch, remainder.NextBatch)
	}
}

func TestIntraBatchDependencyPrefixFallbackPreservesExplicitContinuation(t *testing.T) {
	plan := dataquery.TaskPlan{ContinueAfter: true, Actions: []dataquery.DataAction{{
		ID:             "extract",
		Kind:           dataquery.DataActionExtractRecords,
		OutputArtifact: "records.json",
	}, {
		ID:         "derive",
		Kind:       dataquery.DataActionDeriveFields,
		InputPaths: []string{"records.json"},
	}}}
	prefix, remainder, ok := IntraBatchDependencyPrefixFallback(plan)
	if !ok || !prefix.ContinueAfter || !remainder.ContinueAfter {
		t.Fatalf("prefix=%+v remainder=%+v ok=%v, want explicit continuation preserved", prefix, remainder, ok)
	}
}

func TestIntraBatchDependencyPrefixFallbackReplansScriptAfterTypedRemainder(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{
		{ID: "extract", Kind: dataquery.DataActionExtractRecords, OutputArtifact: "users"},
		{ID: "filter", Kind: dataquery.DataActionFilterRecords, InputPaths: []string{"users"}, OutputArtifact: "active"},
		{ID: "project", Kind: dataquery.DataActionCustomTransform, Script: "emit_result(json_records('active'))"},
	}}
	prefix, remainder, ok := IntraBatchDependencyPrefixFallback(plan)
	if !ok || len(prefix.Actions) != 1 || prefix.Actions[0].ID != "extract" {
		t.Fatalf("prefix=%+v ok=%v, want producer prefix", prefix, ok)
	}
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "filter" {
		t.Fatalf("remainder=%+v, want typed action before scripted boundary", remainder)
	}
	if !prefix.ContinueAfter || !remainder.ContinueAfter || remainder.NextBatch != deferredScriptReplanInstruction {
		t.Fatalf("prefix=%+v remainder=%+v, want continuation into replanning", prefix, remainder)
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
	if !prefix.ContinueAfter || remainder.ContinueAfter {
		t.Fatalf("continue flags prefix=%v remainder=%v, want intermediate prefix and terminal suffix", prefix.ContinueAfter, remainder.ContinueAfter)
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
