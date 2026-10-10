package types

import "testing"

func TestRuntimeRelationDiagramCompileAuthority(t *testing.T) {
	if RuntimeRelationDiagramUsesNativeAuthority(nil) {
		t.Fatal("missing request cannot select native diagram authority")
	}
	for _, tc := range []struct {
		name    string
		profile *RuntimeQuestionProfile
		native  bool
	}{
		{"relation", &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeRelationAnalysis}, true},
		{"relation_and_work", &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeRelationAnalysis, RuntimeWorkRelationRequested: true}, true},
		{"frame_relation", &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeRelationAnalysis, FrameCausalityRequested: true}, false},
		{"causal_work", &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeCausalDiagnosis, RuntimeWorkRelationRequested: true}, false},
		{"bounded_work", &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet, RuntimeWorkRelationRequested: true}, false},
		{"unspecified_work", &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeUnspecified, RuntimeWorkRelationRequested: true}, false},
		{"legacy", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ir := &AnalysisIR{RequestModel: RequestModel{Intent: IntentTrace, PerfTrace: &PerfBundle{}, RuntimeQuestionProfile: tc.profile}}
			if RuntimeRelationDiagramUsesNativeAuthority(&ir.RequestModel) != tc.native {
				t.Fatal("native diagram lane must use only the explicit non-frame relation profile")
			}
			for _, kind := range []DiagramKind{DiagramFlow, DiagramSequence, DiagramCallDAG, DiagramArchitecture} {
				view := BuildAnswerSemanticView(ir, planRequiringDiagramKind(kind))
				family := ResolveQuestionFamily(ir.RequestModel)
				if view.Family != family {
					t.Fatalf("diagram qualification must not reclassify the answer family: %s", view.Family)
				}
				if view.DiagramPlan == nil || view.DiagramPlan.RequireStructuralEdge != ((tc.native || family != QFRootCauseTrace) && kind != DiagramArchitecture) {
					t.Fatalf("kind=%s compiled structure=%+v", kind, view.DiagramPlan)
				}
				for _, relation := range view.DiagramPlan.EdgeRelations {
					if relation.Min != 0 {
						t.Fatalf("native qualification must not invent mandatory relation kinds: %+v", relation)
					}
				}
			}
		})
	}
}
