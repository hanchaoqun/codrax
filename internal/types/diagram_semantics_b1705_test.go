package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestB1705CompiledDiagramKindsDoNotInventSemanticMinimums(t *testing.T) {
	families := map[string]func() *AnalysisIR{
		"architecture": irForArchitecture, "call_chain": irForCallChain,
		"generic": irForGeneric, "root_cause_trace": irForRootCauseTrace,
		"configuration": irForConfigPrecedence, "role": irForRoleLookup,
		"enumeration": irForEnumeration, "comparison": irForComparison,
	}
	for family, build := range families {
		for _, kind := range []DiagramKind{DiagramFlow, DiagramSequence, DiagramCallDAG, DiagramArchitecture} {
			t.Run(family+"/"+string(kind), func(t *testing.T) {
				view := BuildAnswerSemanticView(build(), planRequiringDiagramKind(kind))
				if view.DiagramPlan == nil || !view.DiagramPlan.Required || view.DiagramPlan.Kind != kind {
					t.Fatalf("required visual form lost: %+v", view.DiagramPlan)
				}
				wantStructure := family != "root_cause_trace" && kind != DiagramArchitecture
				if view.DiagramPlan.RequireStructuralEdge != wantStructure {
					t.Errorf("structural requirement=%t, want %t", view.DiagramPlan.RequireStructuralEdge, wantStructure)
				}
				for _, relation := range view.DiagramPlan.EdgeRelations {
					if relation.Kind != DiagramRelObserve || relation.Min != 0 || family != "root_cause_trace" {
						t.Errorf("visual form manufactured a semantic relation: %+v", relation)
					}
				}
			})
		}
	}
}

func TestB1705CompiledDiagramTeachingDoesNotDemandDisplayInferredRelations(t *testing.T) {
	for family, build := range map[string]func() *AnalysisIR{
		"architecture": irForArchitecture, "call_chain": irForCallChain, "configuration": irForConfigPrecedence,
	} {
		for _, kind := range []DiagramKind{DiagramFlow, DiagramCallDAG} {
			t.Run(family+"/"+string(kind), func(t *testing.T) {
				view := BuildAnswerSemanticView(build(), planRequiringDiagramKind(kind))
				req := requiredDiagramRequirement(view)
				if req == nil {
					t.Fatal("compiled finalizer diagram teaching missing")
				}
				want := "any guards or branches must be proved, not added to fill the visual form"
				if kind == DiagramCallDAG {
					want = "Callback or registration handoffs are not themselves proof of a direct call"
				}
				if !strings.Contains(req.Rationale, want) || strings.Contains(req.Rationale, "cited call evidence") {
					t.Errorf("compiled teaching still couples layout and semantic evidence: %s", req.Rationale)
				}
				if !strings.Contains(req.Rationale, "cite evidence for each") {
					t.Errorf("teaching lost per-relation citation responsibility: %s", req.Rationale)
				}
			})
		}
	}
}

func TestB1705ExplicitDiagramRelationsSurviveKindSwitchAndCopy(t *testing.T) {
	for _, relations := range [][]DiagramEdgeRelationContract{
		{{Kind: DiagramRelCall, Min: 1, ClaimForm: ClaimCallEdge}, {Kind: DiagramRelObserve, Min: 0, ClaimForm: ClaimExternalObservation}},
		{{Kind: DiagramRelPrecedence, Min: 2, ClaimForm: ClaimPrecedenceRole}},
	} {
		for _, kind := range []DiagramKind{DiagramFlow, DiagramSequence, DiagramCallDAG, DiagramArchitecture} {
			t.Run(string(relations[0].Kind)+"/"+string(kind), func(t *testing.T) {
				input := append([]DiagramEdgeRelationContract(nil), relations...)
				plan := diagramPlanFor(planRequiringDiagramKind(kind), DiagramSequence, nil, nil, input)
				if plan.Kind != kind || !reflect.DeepEqual(plan.EdgeRelations, relations) {
					t.Fatalf("presentation changed an explicit semantic contract: %+v, want %+v", plan, relations)
				}
				input[0].Min = 99
				if !reflect.DeepEqual(plan.EdgeRelations, relations) {
					t.Fatal("diagram plan retained the caller's mutable relation slice")
				}
			})
		}
	}
}
