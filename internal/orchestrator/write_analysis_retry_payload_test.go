package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderWriteAnalysisRetryPayloadPreservesAcceptedFields(t *testing.T) {
	ir := &types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			RawRequest:   "repair precedence without changing the predicate",
			Task:         types.WriteTask{Kind: types.WriteTaskBugfix, Scope: types.ScopeMicro, Summary: "repair precedence"},
			Risk:         types.WriteRiskProfile{Overall: types.RiskBandLow},
			ScopeAnchors: []string{"src/worker.c"},
			Constraints: []types.WriteConstraint{{
				Kind: "preserve_behavior", Target: "callback branch", Note: "keep existing predicate",
			}},
			ExpectedOutcomes: []string{"keep `(status = callback()) != 0` while fixing grouping"},
			BehaviorContracts: []types.WriteBehaviorContract{{
				ID: "callback-status", Kind: types.WriteBehaviorObservable, Operator: types.WriteBehaviorOpEquals,
				Expected: "-42", Required: true, Source: "write_analyzer",
			}},
		},
		PhaseProposal:   types.PhaseProposal{Split: "single"},
		PitfallsApplied: []string{"operator_precedence"},
		RepoFacts:       types.WriteRepoFacts{PrimaryLanguage: "c", HasTests: true},
	}

	got := renderWriteAnalysisRetryPayload(ir)
	for _, want := range []string{
		"## Previous structured payload (repair base)",
		"preserve task, risk, scope anchors, constraints, expected outcomes, unrelated behavior contracts",
		`"raw_request": "repair precedence without changing the predicate"`,
		`"expected_outcomes": [`,
		"keep `(status = callback()) != 0` while fixing grouping",
		`"id": "callback-status"`,
		`"applicable_pitfalls": [`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("retry payload missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "repo_facts") || strings.Contains(got, "prescan_files") {
		t.Fatalf("retry payload must contain only model-emitted input fields:\n%s", got)
	}
}

func TestRenderWriteAnalysisRetryPayloadNilIsEmpty(t *testing.T) {
	if got := renderWriteAnalysisRetryPayload(nil); got != "" {
		t.Fatalf("nil rejected IR must not invent a repair base: %q", got)
	}
}
