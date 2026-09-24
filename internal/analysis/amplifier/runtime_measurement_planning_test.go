package amplifier

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/budget"
	"github.com/hanchaoqun/codrax/internal/analysis/compiler"
	"github.com/hanchaoqun/codrax/internal/types"
)

func finiteRuntimeMeasurements() types.RequestModel {
	start, end := 2.0, 2.25
	rm := types.RequestModel{
		Intent: types.IntentEnumerate, Scenario: types.ScenarioGeneric,
		Predicates:    types.SemanticPredicates{IsCrossComponent: true, HasPerMemberTable: true},
		AnalyzerHints: types.AnalyzerHints{Entities: []string{"capture_alpha", "capture_beta", "capture_gamma", "capture_delta"}},
		PerfTrace:     &types.PerfBundle{Observations: []types.PerfObservation{{}}},
		RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration, types.RuntimeQuestionFactOtherObservedValue}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "2..2.25"},
		EnumerationBoundary:         &types.RequestedEnumerationBoundary{DeclaredCount: 19, SourceQuote: "nineteen dimensions"},
	}
	rm.RequestedAnswerDimensions = &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true}
	for i := 1; i <= 19; i++ {
		rm.RequestedAnswerDimensions.Dimensions = append(rm.RequestedAnswerDimensions.Dimensions, types.RequestedAnswerDimension{Index: i, Label: fmt.Sprintf("measurement_%d", i), Role: types.RequestedAnswerDimensionObservedValue, Required: true})
	}
	return rm
}

func TestRuntimeMeasurementPlanningNamesStayNavigation(t *testing.T) {
	for _, names := range [][]string{
		{"block_rq_issue", "block_rq_complete", "block_bio_queue", "block_bio_complete", "mmc_request_start", "mmc_request_done", "f2fs_sync_file_enter", "f2fs_sync_file_exit"},
		{"capture_alpha", "capture_beta", "capture_gamma", "capture_delta"},
		{"alpha_capture", "beta_capture", "gamma_capture", "delta_capture"},
		{"Alpha", "Beta", "Gamma", "Delta"},
	} {
		rm := finiteRuntimeMeasurements()
		rm.AnalyzerHints.Entities = names
		got, _ := AmplifyWithPlanningFacts(rm, PlanningFacts{SinglePhysicalRuntimeArtifact: true})
		if !reflect.DeepEqual(got, rm) {
			t.Fatalf("measurement planning rewrote declared answer obligations or names: %+v", got)
		}
		compiled := compiler.Compile(got, budget.BudgetSignals{})
		var evidence int
		for _, node := range compiled.TaskGraph.Nodes {
			if node.Type == types.NodeEvidence {
				evidence++
				for _, name := range names {
					found := false
					for _, hint := range node.SearchHints.EntityIDs {
						found = found || hint == name
					}
					if !found {
						t.Fatalf("measurement name %s lost navigation", name)
					}
				}
			}
		}
		if evidence != 1 {
			t.Fatalf("name spelling generated %d evidence tasks", evidence)
		}
	}
}

func TestRuntimeMeasurementPlanningRetainsIndependentQuestions(t *testing.T) {
	cases := map[string]func(*types.RequestModel, *PlanningFacts){
		"unknown material": func(_ *types.RequestModel, f *PlanningFacts) { f.SinglePhysicalRuntimeArtifact = false },
		"source required":  func(r *types.RequestModel, _ *PlanningFacts) { r.PerfTrace = nil },
		"explicit mixed source": func(r *types.RequestModel, _ *PlanningFacts) {
			r.ExternalObservationPolicy = &types.ExternalObservationPolicy{CurrentSourceMode: types.ExternalObservationCurrentSourceAllow}
		},
		"comparison buckets": func(r *types.RequestModel, _ *PlanningFacts) {
			r.Buckets = []types.QuestionBucket{{Label: "first", Index: 1}, {Label: "second", Index: 2}}
		},
		"different windows": func(r *types.RequestModel, _ *PlanningFacts) {
			p := r.RuntimeArtifactScopeProfile
			a, b := *p.TimeStart, *p.TimeEnd
			c, d := 3.0, 4.0
			p.TimeStart = nil
			p.TimeEnd = nil
			p.TimeWindows = []types.RuntimeArtifactTimeWindow{{TimeStart: &a, TimeEnd: &b, SourceQuote: "first"}, {TimeStart: &c, TimeEnd: &d, SourceQuote: "second"}}
		},
		"unknown window": func(r *types.RequestModel, _ *PlanningFacts) { r.RuntimeArtifactScopeProfile = nil },
		"comparison dimension": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionComparisonAxis
		},
		"relation dimension": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionRelationPath
		},
		"selector": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeBoundedSelector, SourceQuote: "frame"}
		},
		"causal": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeCausalDiagnosis
		},
		"effect verdict": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RuntimeQuestionProfile.Scope = types.RuntimeQuestionScopeBoundedEffectVerdict
		},
		"work relation": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RuntimeQuestionProfile.RuntimeWorkRelationRequested = true
		},
		"frame relation":       func(r *types.RequestModel, _ *PlanningFacts) { r.RuntimeQuestionProfile.FrameCausalityRequested = true },
		"diagnostic":           func(r *types.RequestModel, _ *PlanningFacts) { r.DiagnosticProfile.IsDiagnostic = true },
		"relational predicate": func(r *types.RequestModel, _ *PlanningFacts) { r.Predicates.IsRelationalLookup = true },
		"relationship family": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactDirectWaker}
		},
		"unknown family": func(r *types.RequestModel, _ *PlanningFacts) {
			r.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{"future"}
		},
		"missing family": func(r *types.RequestModel, _ *PlanningFacts) { r.RuntimeQuestionProfile.FactFamilies = nil },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			rm := finiteRuntimeMeasurements()
			facts := PlanningFacts{SinglePhysicalRuntimeArtifact: true}
			alter(&rm, &facts)
			got, _ := AmplifyWithPlanningFacts(rm, facts)
			if len(got.SubTopics) != 4 {
				t.Fatalf("lost legacy topic protection: %+v", got.SubTopics)
			}
		})
	}
	for _, explicit := range [][]types.SubTopic{{{Summary: "one explicit"}}, {{Summary: "first"}, {Summary: "second"}}} {
		rm := finiteRuntimeMeasurements()
		rm.SubTopics = explicit
		got, _ := AmplifyWithPlanningFacts(rm, PlanningFacts{SinglePhysicalRuntimeArtifact: true})
		if !reflect.DeepEqual(got.SubTopics, explicit) {
			t.Fatal("explicit topics changed")
		}
	}
	rm := finiteRuntimeMeasurements()
	got, _ := Amplify(rm)
	if len(got.SubTopics) != 4 {
		t.Fatal("legacy entry silently assumed material origin")
	}
	rm.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeFullArtifact, SourceQuote: "this trace"}
	got, _ = AmplifyWithPlanningFacts(rm, PlanningFacts{SinglePhysicalRuntimeArtifact: true})
	if len(got.SubTopics) != 0 {
		t.Fatal("full single source manufactured name-derived topics")
	}
}
