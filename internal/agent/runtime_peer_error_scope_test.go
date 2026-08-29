package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnalyzerPeerErrorClassificationBoundaryUsesValidatedTopology(t *testing.T) {
	bundle := &types.LogBundle{Errors: []types.LogError{
		{Type: "Error", Frames: []types.LogFrame{{Func: "NativeBridge.invokeOhSum"}}},
		{Type: "panic", Frames: []types.LogFrame{{Func: "demo.bridge.ohSum"}}},
	}}
	ctx := &types.AgentContext{LogTriage: bundle}

	got := renderAnalyzerPeerErrorClassificationBoundary(ctx)
	for _, want := range []string{
		"multiple top-level peer error occurrences",
		"bounded_fact_set",
		"other_observed_value",
		"cross-occurrence relation is unproven",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("peer-error analyzer boundary missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"NativeBridge.invokeOhSum", "demo.bridge.ohSum", "root cause is"} {
		if strings.Contains(got, banned) {
			t.Fatalf("peer-error analyzer boundary must not select a case-specific conclusion %q:\n%s", banned, got)
		}
	}
}

func TestRenderAnalyzerPeerErrorClassificationBoundarySkipsSingleError(t *testing.T) {
	ctx := &types.AgentContext{LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "panic"}}}}
	if got := renderAnalyzerPeerErrorClassificationBoundary(ctx); got != "" {
		t.Fatalf("single-error artifact must not receive peer-error topology guidance:\n%s", got)
	}
}

func TestLogTriageShapeTeachingKeepsPeerErrorsOutOfCauseAndObservations(t *testing.T) {
	got := types.LogTriageJSONShapeFirstTeaching
	for _, want := range []string{
		"Adjacent top-level error headers are separate errors[] entries",
		"bridge/native wording",
		"Do not duplicate an error header or its stack as an observation",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log-triage shape teaching missing %q:\n%s", want, got)
		}
	}
}
