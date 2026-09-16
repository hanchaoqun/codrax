package types

import (
	"reflect"
	"testing"
)

func TestB1708DiagramPresentationPolicyIsNotPathAuthority(t *testing.T) {
	boundary := &CallChainEndpointBoundary{
		Disposition: CallChainEndpointNoDirectedPath, SourceEndpoint: "Entry", RequestedSink: "Wrapper",
		EvidenceCapsule: &CallChainEndpointEvidenceCapsule{Status: CallChainEndpointEvidenceSharedCalleeBoundary},
	}
	for _, tc := range []struct {
		name     string
		family   QuestionFamily
		required bool
		status   CallChainEndpointEvidenceStatus
		want     bool
	}{
		{"source diagram", QFCallChain, true, CallChainEndpointEvidenceSharedCalleeBoundary, true},
		{"definition-only endpoint", QFCallChain, true, CallChainEndpointEvidenceEndpointUnresolved, true},
		{"disjoint frontiers", QFCallChain, true, CallChainEndpointEvidenceDisjointFrontiers, true},
		{"no requested diagram", QFCallChain, false, CallChainEndpointEvidenceSharedCalleeBoundary, false},
		{"runtime trace remains separate", QFRootCauseTrace, true, CallChainEndpointEvidenceSharedCalleeBoundary, false},
		{"stale waiver cannot suppress actual path", QFCallChain, true, CallChainEndpointEvidenceDirectedPathPresent, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyBoundary := *boundary
			copyCapsule := *boundary.EvidenceCapsule
			copyCapsule.Status = tc.status
			copyBoundary.EvidenceCapsule = &copyCapsule
			view := &AnswerSemanticView{Family: tc.family, RequiredBlocks: []BlockRequirement{{Kind: BlockDiagram, Required: tc.required}}}
			if got := CallChainEndpointBoundaryAllowsDiagramSupport(view, &copyBoundary); got != tc.want {
				t.Fatalf("support presentation=%v want=%v", got, tc.want)
			}
			if len(CallChainEndpointBoundaryPrincipalEdges(&copyCapsule)) != 0 {
				t.Fatal("presentation policy must not create principal evidence")
			}
		})
	}
}

func TestB1708FacetProjectionPreservesSupportWithoutMutatingPlan(t *testing.T) {
	plan := &FacetCoverageContract{Required: []FacetRequirement{
		{Kind: FacetPrincipalPathEdge, SourceCandidate: []string{"edge", "support"}},
		{Kind: FacetDiagramSpine, SourceCandidate: []string{"edge", "support"}},
	}, Optional: []FacetRequirement{{Kind: FacetDiagramSpine, SourceCandidate: []string{"support"}}}}
	view := &AnswerSemanticView{Family: QFCallChain, FacetCoverage: plan, RequiredBlocks: []BlockRequirement{{Kind: BlockDiagram, Required: true}}}
	boundary := &CallChainEndpointBoundary{
		Disposition: CallChainEndpointNoDirectedPath, SourceEndpoint: "Entry", RequestedSink: "Wrapper",
		EvidenceCapsule: &CallChainEndpointEvidenceCapsule{Status: CallChainEndpointEvidenceSharedCalleeBoundary,
			SourcePath: []CallChainEvidenceEdge{{From: "Entry", To: "Shared", EvidenceID: "edge"}}},
	}
	projectCallChainEndpointBoundaryFacetAuthority(view, boundary)
	if !reflect.DeepEqual(view.FacetCoverage.Required[0].SourceCandidate, []string{"edge"}) ||
		!reflect.DeepEqual(view.FacetCoverage.Required[1].SourceCandidate, []string{"edge", "support"}) ||
		!reflect.DeepEqual(view.FacetCoverage.Optional[0].SourceCandidate, []string{"support"}) {
		t.Fatalf("principal and presentation evidence domains drifted: %+v", view.FacetCoverage)
	}
	if !reflect.DeepEqual(plan.Required[0].SourceCandidate, []string{"edge", "support"}) {
		t.Fatal("presentation projection mutated the upstream evidence contract")
	}
}
