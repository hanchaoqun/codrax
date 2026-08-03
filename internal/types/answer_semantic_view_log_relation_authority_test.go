package types

import "testing"

func operationalRootCauseIR(authorities ...string) *AnalysisIR {
	rows := make([]LogOperationalSemantic, 0, len(authorities))
	for _, authority := range authorities {
		rows = append(rows, LogOperationalSemantic{TransitionAuthority: authority})
	}
	return &AnalysisIR{RequestModel: RequestModel{
		Intent:    IntentRootCause,
		Scenario:  ScenarioRootCause,
		LogTriage: &LogBundle{OperationalSemantics: rows},
	}}
}

func principalRequirementOfKind(view *AnswerSemanticView, kind AnswerBlockKind) *BlockRequirement {
	if view == nil {
		return nil
	}
	for i := range view.RequiredBlocks {
		if view.RequiredBlocks[i].Kind == kind && view.RequiredBlocks[i].SurfaceRoleHint == SurfacePrincipal {
			return &view.RequiredBlocks[i]
		}
	}
	return nil
}

func TestRootCauseOperationalEventsWithoutTransitionUseIndependentFactContract(t *testing.T) {
	ir := operationalRootCauseIR(LogOperationalTransitionEventLocalOnly, LogOperationalTransitionEventLocalOnly)
	plan := &AnswerSurfacePlan{
		CurrentSourceEvidenceOrigin: true,
		Diagram:                     &DiagramContract{PreferredKinds: []DiagramKind{DiagramSequence}},
	}
	view := BuildAnswerSemanticView(ir, plan)
	list := principalRequirementOfKind(view, BlockBulletList)
	if list == nil {
		t.Fatalf("unproven cross-event relation must use an independent-fact bullet list: %+v", view.RequiredBlocks)
	}
	if containsString(list.FacetIDs, string(FacetPrincipalPathEdge)) ||
		!containsString(list.FacetIDs, string(FacetUncertaintyBoundary)) {
		t.Fatalf("independent-fact contract facets=%v", list.FacetIDs)
	}
	if principalRequirementOfKind(view, BlockOrderedList) != nil {
		t.Fatalf("unproven transition must not require an ordered cause chain: %+v", view.RequiredBlocks)
	}
	if view.DiagramPlan == nil {
		t.Fatal("preferred diagram plan missing")
	}
	if containsString(view.DiagramPlan.EdgeFacets, string(FacetPrincipalPathEdge)) {
		t.Fatalf("unproven transition diagram must not require a call edge: %+v", view.DiagramPlan)
	}
}

func TestRootCauseOperationalTypedTransitionKeepsCauseChain(t *testing.T) {
	ir := operationalRootCauseIR(LogOperationalTransitionEventLocalOnly, "producer_transition_id")
	view := BuildAnswerSemanticView(ir, &AnswerSurfacePlan{CurrentSourceEvidenceOrigin: true})
	list := principalRequirementOfKind(view, BlockOrderedList)
	if list == nil || !containsString(list.FacetIDs, string(FacetPrincipalPathEdge)) {
		t.Fatalf("typed transition must keep the ordered cause-chain contract: %+v", view.RequiredBlocks)
	}
}

func TestRootCauseSingleOperationalEventHasNoCrossEventFacetRewrite(t *testing.T) {
	ir := operationalRootCauseIR(LogOperationalTransitionEventLocalOnly)
	view := BuildAnswerSemanticView(ir, &AnswerSurfacePlan{CurrentSourceEvidenceOrigin: true})
	if principalRequirementOfKind(view, BlockOrderedList) == nil {
		t.Fatalf("single event has no cross-event relation to demote: %+v", view.RequiredBlocks)
	}
}

func TestRootCauseExplicitTraceWindowPreservesCausalContract(t *testing.T) {
	start, end := 10.0, 10.5
	ir := operationalRootCauseIR(LogOperationalTransitionEventLocalOnly, LogOperationalTransitionEventLocalOnly)
	ir.RequestModel.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.0..10.5",
	}
	view := BuildAnswerSemanticView(ir, &AnswerSurfacePlan{CurrentSourceEvidenceOrigin: true})
	list := principalRequirementOfKind(view, BlockOrderedList)
	if list == nil || !containsString(list.FacetIDs, string(FacetPrincipalPathEdge)) {
		t.Fatalf("explicit Trace window must preserve causal projection answer shape: %+v", view.RequiredBlocks)
	}
}
