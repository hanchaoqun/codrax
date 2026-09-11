package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func b1658WorkflowRequest() RequestModel {
	return RequestModel{
		Intent: IntentExplain, Scenario: ScenarioArchitectureExplain, PredicateAxis: AxisFlow,
		Predicates: SemanticPredicates{HasPerMemberTable: true},
		RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
			IsDimensionedAnswer: true, Confidence: 0.95,
			Dimensions: []RequestedAnswerDimension{{Index: 1, Role: RequestedAnswerDimensionStageWorkflow, Required: true}},
		},
	}
}

func TestB1658WorkflowFacetFallbackRetainsFactsWithoutMemberObligations(t *testing.T) {
	rm := b1658WorkflowRequest()
	rm.Predicates.IsCategoryEnumeration = true
	item := EvidenceItem{ID: "decl", Kind: EvidenceDirect, Scope: ScopeLine, Source: "pipeline.cj", LineStart: 10,
		AnchorKind: AnchorDefinition, Subject: "OptionalStep", GroundingStatus: GroundingGrounded}
	plan := &AnswerSurfacePlan{SurfaceEvidence: []EvidenceItem{item}, FacetCoverage: &FacetCoverageContract{
		Family: QFEnumeration, Required: []FacetRequirement{{Kind: FacetEnumerationItem, SourceCandidate: []string{item.ID}}}}}
	got := BuildAnswerSupportPlan(rm, plan)
	lane := answerSupportLaneByKind(got, SupportLanePrincipalEvidence)
	if got == nil || got.Family != QFEnumeration || lane == nil || len(lane.Entries) != 1 {
		t.Fatalf("grounded source facts were lost: %+v", got)
	}
	if got.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyEnrichmentOnly || len(PrincipalSupportMemberObligations(got)) != 0 {
		t.Fatalf("source-only fallback revived a required member set: %+v", got)
	}
	if !strings.Contains(lane.Guidance, "not membership or order") || strings.Contains(lane.Guidance, "each principal item must") {
		t.Fatalf("fallback teaching contradicts its scope: %s", lane.Guidance)
	}
	plan.stepBackboneFromAcceptedSymbolSlate = true
	got = BuildAnswerSupportPlan(rm, plan)
	if got.PrincipalMemberCoverage != PrincipalMemberCoveragePolicyRequired || len(PrincipalSupportMemberObligations(got)) != 1 {
		t.Fatalf("independently accepted explicit member slate lost its established contract: %+v", got)
	}
}

func TestB1658WorkflowCoordinatesDoNotAuthorizeMembership(t *testing.T) {
	for _, file := range []string{"pipeline.go", "pipeline.c", "pipeline.cpp", "Pipeline.java", "Pipeline.kt", "pipeline.ts", "pipeline.ets", "pipeline.cj", "pipeline.py", "pipeline.rs"} {
		t.Run(file, func(t *testing.T) {
			rm := b1658WorkflowRequest()
			fact := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer,
				Label: "model selected stages", Members: []string{"Prepare", "Dispatch", "Legacy"}, Value: "3",
				Provenance: "model_emitted", SupportRefs: []string{file + ":10", file + ":20", file + ":30"}}
			set := EnumerationDisplaySet{Rows: []EnumerationDisplayRow{
				{Member: "Prepare", HasCitation: true, Source: file, LineStart: 10},
				{Member: "Dispatch", HasCitation: true, Source: file, LineStart: 20},
				{Member: "Legacy", HasCitation: true, Source: file, LineStart: 30},
			}}
			before, _ := json.Marshal(fact)
			if AnswerAggregateFactAuthorizesPrincipalContract(fact, &rm) {
				t.Error("source-coordinate existence was promoted to current workflow membership")
			}
			if EnumerationDisplaySetAuthorizesPrincipalContract(&rm, fact, set) {
				t.Error("display-set inherited unsupported membership through its fact branch")
			}
			after, _ := json.Marshal(fact)
			if string(before) != string(after) {
				t.Fatal("membership qualification must not edit the model's set")
			}
			fact.SupportRefs = nil
			if EnumerationDisplaySetAuthorizesPrincipalContract(&rm, fact, set) {
				t.Error("per-row citation fallback revived unsupported workflow membership")
			}
			if PrincipalMemberSetRequiresTypedRelationAuthority(rm) || RequiresRelationMemberSetHandoff(rm) {
				t.Fatal("a workflow table must not acquire a new call-chain/handoff hard contract")
			}
			if !ShouldCompileEnumerationDisplaySetsForRequest(rm) {
				t.Fatal("scope qualification must not disable tables or supporting citation compilation")
			}
		})
	}
}

func TestB1658WorkflowAuthorityKeepsIndependentEvidenceLanes(t *testing.T) {
	rm := b1658WorkflowRequest()
	base := AnswerAggregateFact{Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer,
		Members: []string{"A", "B"}, SupportRefs: []string{"a.go:10", "b.go:20"}, Value: "2"}
	for _, marker := range []string{TypedRelationPrincipalMemberSetAggregateProvenance, SourceInventoryPrincipalRowSetAggregateProvenance} {
		fact := base
		fact.Provenance = marker
		if !AnswerAggregateFactAuthorizesPrincipalContract(fact, &rm) || !EnumerationDisplaySetAuthorizesPrincipalContract(&rm, fact, EnumerationDisplaySet{}) {
			t.Fatalf("independent system-verified proof lane lost: %s", marker)
		}
	}
	for _, ref := range []string{"trace_query:principal_state:E7", "git_log[0]", "mcp_resource: mcp://fixture/flow#L7"} {
		fact := base
		fact.SupportRefs = append([]string{ref}, base.SupportRefs...)
		if !AnswerAggregateFactAuthorizesPrincipalContract(fact, &rm) {
			t.Errorf("explicit non-source origin must retain its own established authority: %s", ref)
		}
	}
	for _, kind := range []AnswerAggregateKind{AnswerAggregateScalar, AnswerAggregateTotalCount, AnswerAggregateNegativeObservation} {
		fact := base
		fact.Kind = kind
		if !AnswerAggregateFactAuthorizesPrincipalContract(fact, &rm) {
			t.Errorf("non-member fact was unexpectedly reclassified: %s", kind)
		}
	}
	if !AnswerAggregateFactAuthorizesPrincipalContract(base, nil) {
		t.Fatal("legacy caller without request model must retain its established source handling")
	}
	for _, name := range []string{"enumeration", "optional dimension", "non-workflow table"} {
		t.Run(name, func(t *testing.T) {
			plain := b1658WorkflowRequest()
			switch name {
			case "enumeration":
				plain.Intent = IntentEnumerate
			case "optional dimension":
				plain.RequestedAnswerDimensions.Dimensions[0].Required = false
			case "non-workflow table":
				plain.RequestedAnswerDimensions = nil
			}
			if !AnswerAggregateFactAuthorizesPrincipalContract(base, &plain) {
				t.Fatal("ordinary source inventory or unrelated table lost its source-coordinate authority")
			}
		})
	}
}
