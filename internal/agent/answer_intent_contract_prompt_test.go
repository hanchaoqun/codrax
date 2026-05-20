package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderAnswerDocUnifiedIntentContract_HistoryNarrativeDoesNotAskForScalar(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioGeneric,
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"Evidence Origin / Requested Output Boundary",
		"`vcs_metadata`",
		"`summary`, `mechanism`",
		"not a request to emit a `scalar` block",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("history narrative intent contract prompt missing %q:\n%s", want, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "requested outputs:") && strings.Contains(line, "`scalar`") {
			t.Fatalf("history narrative should not list scalar as requested output:\n%s", got)
		}
	}
}

func TestRenderAnswerDocUnifiedIntentContract_HistoryDiagramShowsMixedOrigins(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:      types.IntentExplain,
				Scenario:    types.ScenarioArchitectureExplain,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
				Predicates: types.SemanticPredicates{
					IsHistoryLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{Required: true, RequiredKind: types.DiagramFlow},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"`current_source`, `vcs_metadata`, `vcs_diff`",
		"`summary`, `mechanism`, `diagram`",
		"Keep historical diff facts and current-checkout source facts in separate lanes",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("history diagram intent contract prompt missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAnswerDocUnifiedIntentContract_RuntimeArtifactAvoidsRepoCitationPressure(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true},
				LogTriage: &types.LogBundle{
					Errors: []types.LogError{{Type: "panic"}},
				},
			},
		},
	}
	got := renderAnswerDocUnifiedIntentContract(ctx)
	for _, want := range []string{
		"`runtime_artifact`",
		"`summary`, `diagnostic`",
		"without current-repo citations",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("runtime artifact intent contract prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "`current_source`") {
		t.Fatalf("external-only runtime artifact should not list current_source:\n%s", got)
	}
}
