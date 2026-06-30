package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func citationLanePromptFor(t *testing.T, rm types.RequestModel) string {
	t.Helper()
	mu := types.NewMutableState("")
	ctx := &types.AgentContext{
		Objective:  "trace question",
		Mutable:    mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
	}
	e := &extractorEvaluator{}
	return e.BuildInitialInstruction(ctx, nil)
}

func extractorPromptForContext(t *testing.T, ctx *types.AgentContext) string {
	t.Helper()
	e := &extractorEvaluator{}
	return e.BuildInitialInstruction(ctx, nil)
}

// With an external-only artifact and no current-source requirement,
// the prompt pre-states the artifact-local citation lane (the
// contract used to reach the model only inside a rejection).
func TestExtractorCitationLaneObservationOnly(t *testing.T) {
	rm := types.RequestModel{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Message: "fatal error: concurrent map writes"}},
		},
	}
	if rm.CurrentSourceLaneDecision().RequiresCurrentSource() {
		t.Fatalf("fixture must be observation-only")
	}
	prompt := citationLanePromptFor(t, rm)
	if !strings.Contains(prompt, "Hypothesis-verdict citation lanes") {
		t.Fatalf("artifact turn must pre-state the citation lanes section")
	}
	if !strings.Contains(prompt, "runtime_artifact:1-5") || !strings.Contains(prompt, "bare `runtime_artifact`") {
		t.Fatalf("observation-only lane must teach the exact artifact-local citation shapes")
	}
}

func TestExtractorCitationLaneSoftRuntimeSourceUsesArtifactCaveatLane(t *testing.T) {
	mu := types.NewMutableState("")
	rm := types.RequestModel{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{Message: "timeout while rendering"}},
		},
		CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"current parser mechanism"},
			Confidence:                          0.9,
		},
	}
	if !rm.CurrentSourceLaneDecision().RequiresCurrentSource() {
		t.Fatalf("fixture must request a soft current-source lane")
	}
	ctx := &types.AgentContext{
		Objective: "trace plus current parser mechanism",
		Mutable:   mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: rm,
			HypothesisSet: []types.Hypothesis{{
				ID:        "h1",
				Status:    types.HypUnknown,
				Statement: "current renderer path caused the timeout",
			}},
		},
	}

	prompt := extractorPromptForContext(t, ctx)
	if !strings.Contains(prompt, "Hypothesis-verdict citation lanes") {
		t.Fatalf("artifact turn must pre-state the citation lanes section")
	}
	for _, want := range []string{
		"observation-only verdicts about the attached log/trace",
		"runtime_artifact:1-5",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("soft runtime/source prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{
		"This question requires the current-source lane",
		"Pending hypotheses",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("soft runtime/source prompt must not hard-push current-source verdict work %q:\n%s", forbidden, prompt)
		}
	}

	out, err := (&extractorEvaluator{}).ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out == nil || !strings.Contains(out.StageReport, "runtime_artifact_without_required_current_source") {
		t.Fatalf("soft runtime/source stage report should use artifact/caveat lane, got: %+v", out)
	}
}

// When the question requires the current-source lane, the section
// states that artifact-local lines stay out of citation for
// current-code verdicts — the dominant first-attempt rejection class.
func TestExtractorCitationLaneCurrentSourceRequired(t *testing.T) {
	rm := types.RequestModel{
		LogTriage: &types.LogBundle{
			Errors:        []types.LogError{{Message: "fatal error: concurrent map writes"}},
			ResolvedFiles: []string{"internal/x/x.go"},
		},
	}
	if !rm.CurrentSourceLaneDecision().RequiresCurrentSource() {
		t.Fatalf("fixture must require the current-source lane")
	}
	prompt := citationLanePromptFor(t, rm)
	if !strings.Contains(prompt, "Hypothesis-verdict citation lanes") {
		t.Fatalf("artifact turn must pre-state the citation lanes section")
	}
	if !strings.Contains(prompt, "current-source lane") || !strings.Contains(prompt, "never in `citation` for a current-code verdict") {
		t.Fatalf("required lane must steer artifact lines out of citation")
	}
}

// No artifact → the section must not render at all.
func TestExtractorCitationLaneAbsentWithoutArtifact(t *testing.T) {
	prompt := citationLanePromptFor(t, types.RequestModel{})
	if strings.Contains(prompt, "Hypothesis-verdict citation lanes") {
		t.Fatalf("section must be artifact-gated")
	}
}
