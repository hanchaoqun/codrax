package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlannerTaskFraming_RendersIRFields verifies the planner reads
// WriteAnalysisIR fields and renders them in the prompt section.
// Each LLM-emit field (kind, scope, summary, risk, anchors,
// constraints, outcomes) should appear in the rendered text.
func TestPlannerTaskFraming_RendersIRFields(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			RawRequest: "the user request",
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeMicro,
				Summary: "wire a --quiet flag through cmd/root.go",
			},
			Risk: types.WriteRiskProfile{
				AffectsPublicAPI:   true,
				ChangesBuildSystem: false,
				Overall:            types.RiskBandLow,
			},
			ScopeAnchors: []string{"cmd/root.go", "internal/render"},
			Constraints: []types.WriteConstraint{
				{Kind: "preserve_api", Target: "*.Public", Note: "do not break existing flags"},
			},
			ExpectedOutcomes: []string{
				"the --quiet flag suppresses INFO output",
				"existing tests still pass",
			},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.buildTaskFramingSection(ctx)
	for _, want := range []string{
		"## Task framing",
		"wire a --quiet flag through cmd/root.go",
		"feature",
		"micro",
		"low",
		"affects-public-api",
		"cmd/root.go, internal/render",
		"preserve_api",
		"do not break existing flags",
		"the --quiet flag suppresses INFO output",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered task framing missing %q; got:\n%s", want, got)
		}
	}
}

// TestPlannerTaskFraming_NoIRReturnsEmpty pins the degraded path:
// when WriteAnalysisIR is absent (write_analyzer didn't run or
// degraded), the planner falls through with no task-framing
// section, equivalent to pre-commit-1 behaviour.
func TestPlannerTaskFraming_NoIRReturnsEmpty(t *testing.T) {
	mu := types.NewMutableState("the user request")
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	if got := eval.buildTaskFramingSection(ctx); got != "" {
		t.Errorf("nil IR should yield empty section; got %q", got)
	}
}

// TestPlannerTaskFraming_NilCtxSafe pins defensive guards.
func TestPlannerTaskFraming_NilCtxSafe(t *testing.T) {
	eval := &plannerEvaluator{}
	if got := eval.buildTaskFramingSection(nil); got != "" {
		t.Errorf("nil ctx should yield empty section; got %q", got)
	}
}

func TestPlannerWorkflowSeed_RendersNextBatchFromPhaseProposal(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Kind:    types.WriteTaskFeature,
				Scope:   types.ScopeProject,
				Summary: "add OAuth provider support",
			},
			Constraints: []types.WriteConstraint{
				{Kind: "preserve_existing_flow", Target: "read-mode"},
			},
			ExpectedOutcomes: []string{
				"static providers still work",
				"OAuth providers cache tokens",
			},
		},
		PhaseProposal: types.PhaseProposal{
			Split: "sequential",
			Phases: []types.PhaseSeed{
				{Goal: "add provider config schema", RoughTargetPaths: []string{"internal/config/runtime.go", "providers.yaml.example"}},
				{Goal: "wire OAuth token cache", RoughTargetPaths: []string{"internal/llm"}},
			},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.buildWorkflowSeedSection(ctx)
	for _, want := range []string{
		"## Rolling write workflow",
		"add OAuth provider support",
		"strategy: rolling_batches",
		"max_batches: 2",
		"preserve_existing_flow: read-mode",
		"next_batch",
		"batch-1",
		"add provider config schema",
		"needs_code_exploration: true",
		"internal/config/runtime.go, providers.yaml.example",
		"static providers still work",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("workflow seed missing %q; got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "add OAuth token cache") {
		t.Errorf("workflow seed should not unfold later batches into current next_batch details; got:\n%s", got)
	}
}

func TestPlannerBuildInitialInstruction_IncludesWorkflowSeedAfterTaskFraming(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{
				Summary: "fix flaky retry loop",
			},
			ScopeAnchors:     []string{"internal/orchestrator"},
			ExpectedOutcomes: []string{"retry loop stops after success"},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.BuildInitialInstruction(ctx, nil)
	taskIdx := strings.Index(got, "## Task framing")
	workflowIdx := strings.Index(got, "## Rolling write workflow")
	if taskIdx < 0 || workflowIdx < 0 {
		t.Fatalf("expected task framing and workflow seed; got:\n%s", got)
	}
	if workflowIdx < taskIdx {
		t.Fatalf("workflow seed should render after task framing; got:\n%s", got)
	}
	if !strings.Contains(got, "batch-1") || !strings.Contains(got, "retry loop stops after success") {
		t.Fatalf("workflow seed did not include default batch and success criteria; got:\n%s", got)
	}
}

func TestPlannerWriteExplorationHandoff_RendersAfterWorkflowSeed(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Summary: "fix planner retry context"},
		},
	})
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:          "batch-1",
		Goal:             "patch planner handoff section",
		TargetFiles:      []string{"internal/agent/planner.go"},
		RelevantSymbols:  []string{"BuildInitialInstruction"},
		ExistingPatterns: []string{"planner sections are pure data"},
		Invariants:       []string{"do not bypass ChangePlan validation"},
		TestSurface:      []string{"go test ./internal/agent"},
		RiskNotes:        []string{"prompt order regressions affect write mode only"},
		Unknowns:         []string{"whether planner prompt includes handoff"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			Kind:      "mechanism",
			Source:    "internal/agent/planner.go",
			LineStart: 105,
			Subject:   "BuildInitialInstruction",
			Summary:   "composes planner sections",
		}},
		Confidence: "high",
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.BuildInitialInstruction(ctx, nil)
	workflowIdx := strings.Index(got, "## Rolling write workflow")
	handoffIdx := strings.Index(got, "## Prior code exploration handoff")
	investigationIdx := strings.Index(got, "## Pre-plan investigation seed")
	if workflowIdx < 0 || handoffIdx < 0 {
		t.Fatalf("expected workflow seed and handoff; got:\n%s", got)
	}
	if handoffIdx < workflowIdx {
		t.Fatalf("handoff should render after workflow seed; got:\n%s", got)
	}
	if investigationIdx >= 0 && investigationIdx < handoffIdx {
		t.Fatalf("handoff should render before generic investigation seed; got:\n%s", got)
	}
	for _, want := range []string{
		"patch planner handoff section",
		"internal/agent/planner.go",
		"BuildInitialInstruction @ internal/agent/planner.go:105",
		"planner sections are pure data",
		"do not bypass ChangePlan validation",
		"go test ./internal/agent",
		"prompt order regressions affect write mode only",
		"whether planner prompt includes handoff",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("handoff prompt missing %q; got:\n%s", want, got)
		}
	}
}

func TestPlannerWriteExplorationRequest_RendersBeforeHandoff(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Summary: "fix planner retry context"},
		},
	})
	mu.SetWriteExplorationRequest(&types.WriteExplorationRequest{
		BatchID:              "batch-1",
		Goal:                 "inspect planner retry inputs",
		ExplorationQuestions: []string{"where does retry state enter the planner?"},
		CandidatePaths:       []string{"internal/agent/planner.go"},
		Constraints:          []string{"read mode must remain unchanged"},
		EvidenceRequirements: []string{"file:line anchors for planner prompt sections"},
		MaxRounds:            3,
	})
	mu.SetWriteExplorationHandoff(&types.WriteExplorationHandoff{
		BatchID:     "batch-1",
		Goal:        "patch planner handoff section",
		TargetFiles: []string{"internal/agent/planner.go"},
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.BuildInitialInstruction(ctx, nil)
	reqIdx := strings.Index(got, "## Targeted source exploration request")
	handoffIdx := strings.Index(got, "## Prior code exploration handoff")
	if reqIdx < 0 || handoffIdx < 0 {
		t.Fatalf("expected request and handoff sections; got:\n%s", got)
	}
	if handoffIdx < reqIdx {
		t.Fatalf("request should render before handoff; got:\n%s", got)
	}
	for _, want := range []string{
		"inspect planner retry inputs",
		"where does retry state enter the planner?",
		"internal/agent/planner.go",
		"read mode must remain unchanged",
		"file:line anchors for planner prompt sections",
		"max_rounds: 3",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("request prompt missing %q; got:\n%s", want, got)
		}
	}
}

func TestPlannerWriteExplorationHandoff_AbsentWhenUnset(t *testing.T) {
	mu := types.NewMutableState("the user request")
	mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
		Request: types.WriteRequestModel{
			Task: types.WriteTask{Summary: "fix planner retry context"},
		},
	})
	ctx := &types.AgentContext{Mutable: mu}
	eval := &plannerEvaluator{}
	got := eval.BuildInitialInstruction(ctx, nil)
	if strings.Contains(got, "Prior code exploration handoff") {
		t.Fatalf("handoff section should be absent without typed handoff; got:\n%s", got)
	}
}
