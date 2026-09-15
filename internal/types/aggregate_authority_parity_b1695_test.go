package types

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func b1695SourceMemberFact(path string) AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind: AnswerAggregateMemberSet, Role: AnswerAggregateRolePrincipalAnswer,
		Label: "selected members", Value: "1", Members: []string{"Member"},
		MemberNotes: []string{"model-authored explanation"},
		SupportRefs: []string{path + ":12"}, Provenance: "model_emitted",
	}
}

func b1695RelationRequests() map[string]RequestModel {
	return map[string]RequestModel{
		"relation-enumeration": {Intent: IntentEnumerate, Predicates: SemanticPredicates{IsRelationalLookup: true}},
		"source-call-chain":    {Intent: IntentTrace, PredicateAxis: AxisCall, AnalyzerHints: AnalyzerHints{Kind: string(ReqCallChain)}},
		"conceptual-workflow": {
			Intent: IntentExplain, Scenario: ScenarioArchitectureExplain, PredicateAxis: AxisFlow,
			Predicates: SemanticPredicates{HasPerMemberTable: true},
			RequestedAnswerDimensions: &RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true, Confidence: .95,
				Dimensions: []RequestedAnswerDimension{{Index: 1, Role: RequestedAnswerDimensionStageWorkflow, Required: true}},
			},
		},
	}
}

func TestB1695SourceCoordinatesCannotProveUnprovenMembershipInLedger(t *testing.T) {
	for name, request := range b1695RelationRequests() {
		for _, path := range []string{"member.go", "member.c", "member.cpp", "Member.java", "member.rs", "member.py", "member.ts", "member.ets", "member.cj"} {
			t.Run(name+"/"+path, func(t *testing.T) {
				fact := b1695SourceMemberFact(path)
				before, _ := json.Marshal(fact)
				if AnswerAggregateFactAuthorizesPrincipalContract(fact, &request) {
					t.Fatal("fixture must lack the existing principal membership permission")
				}
				ledger := CompileObservationLedger(ObservationLedgerInput{
					RequestModel: &request, AggregateFacts: []AnswerAggregateFact{fact},
					EvidenceItems: []EvidenceItem{{
						ID: "member-definition", Kind: EvidenceDirect, Scope: ScopeLine,
						Source: path, LineStart: 12, AnchorKind: AnchorDefinition,
						Subject: "Member", GroundingStatus: GroundingGrounded,
					}},
				})
				record := findObservationRecord(t, ledger, "aggregate:0#current_source")
				if record.ClaimAuthority != ObservationClaimAuthorityModelInference {
					t.Errorf("a source coordinate was upgraded into membership proof: %+v", record)
				}
				if record.SourceRef.Path != path || record.Span.LineStart != 12 ||
					record.GroundingStatus != GroundingGrounded || record.Role != AnswerAggregateRolePrincipalAnswer ||
					record.Value != fact.Value || !reflect.DeepEqual(record.SupportRefs, fact.SupportRefs) || len(record.ModelNotes) != 1 {
					t.Fatalf("qualification lost valid coordinates or the model's retained claim: %+v", record)
				}
				after, _ := json.Marshal(fact)
				if string(before) != string(after) {
					t.Fatal("qualification rewrote the model-authored fact")
				}
			})
		}
	}
}

func TestB1695IndependentSourceAuthorityUsesExistingPrincipalPermission(t *testing.T) {
	requests := b1695RelationRequests()
	requests["ordinary-enumeration"] = RequestModel{Intent: IntentEnumerate}
	requests["ordinary-scalar"] = RequestModel{Intent: IntentExplain, Predicates: SemanticPredicates{IsScalarAnswer: true}}
	requests["external-only"] = RequestModel{Intent: IntentRootCause, Scenario: ScenarioPerformanceBottleneck, ExternalObservationPolicy: &ExternalObservationPolicy{
		CurrentSourceMode: ExternalObservationCurrentSourceExclude,
		ExclusionKind:     ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"only inspect the attached trace"},
	}}
	for name, request := range requests {
		if name == "external-only" && (!request.ExternalObservationPolicy.ExcludesCurrentSource() || PrincipalMemberSetRequiresTypedRelationAuthority(request)) {
			t.Fatal("external-only control must exercise explicit exclusion, not the source-relation restriction")
		}
		for _, kind := range []AnswerAggregateKind{AnswerAggregateMemberSet, AnswerAggregateScalar, AnswerAggregateTotalCount, AnswerAggregateNegativeObservation} {
			t.Run(fmt.Sprintf("%s/%s", name, kind), func(t *testing.T) {
				fact := b1695SourceMemberFact("members.go")
				fact.Kind = kind
				want := AnswerAggregateFactAuthorizesPrincipalContract(fact, &request)
				if got := aggregateFactHasIndependentTypedAuthority(fact, &request); got != want {
					t.Fatalf("source authority diverged from the existing principal admission: independent=%v principal=%v", got, want)
				}
			})
		}
	}
	fact := b1695SourceMemberFact("members.go")
	if !aggregateFactHasIndependentTypedAuthority(fact, nil) {
		t.Fatal("compatibility caller without request model lost its existing exact source lane")
	}
}

func TestB1695KeepsDistinctExternalPermissionAndIndependentProof(t *testing.T) {
	rm := RequestModel{Intent: IntentRootCause, Scenario: ScenarioPerformanceBottleneck}
	for _, ref := range []string{"trace_query:window_stats:E7", "git_log[0]", "mcp_resource: mcp://fixture/report#L7"} {
		t.Run(ref, func(t *testing.T) {
			fact := AnswerAggregateFact{Kind: AnswerAggregateScalar, Label: "retained observation", Value: "7", SupportRefs: []string{ref}}
			if !AnswerAggregateFactAuthorizesPrincipalContract(fact, &rm) {
				t.Fatal("explicit external support lost its established principal lane")
			}
			if aggregateFactHasIndependentTypedAuthority(fact, &rm) {
				t.Fatal("an external coordinate must not certify the whole model-authored aggregate")
			}
		})
	}
	for name, request := range b1695RelationRequests() {
		for _, provenance := range []string{TypedRelationPrincipalMemberSetAggregateProvenance, SourceInventoryPrincipalRowSetAggregateProvenance} {
			t.Run(name+"/"+provenance, func(t *testing.T) {
				// Unit-level qualification of a producer-owned marker. The agent
				// public tests separately exercise real producers and admission.
				fact := b1695SourceMemberFact("members.go")
				fact.Provenance = provenance
				fact.SupportRefs = nil
				if !aggregateFactHasIndependentTypedAuthority(fact, &request) || !AnswerAggregateFactAuthorizesPrincipalContract(fact, &request) {
					t.Fatal("existing system-verified marker lost its proof lane")
				}
			})
		}
	}
}

func TestB1695RuntimeProjectionKeepsProducerRowsAndOriginalModelPayload(t *testing.T) {
	rm := RequestModel{Intent: IntentRootCause, Scenario: ScenarioPerformanceBottleneck}
	facts := []AnswerAggregateFact{
		{Kind: AnswerAggregateScalar, Label: "model runtime restatement", Value: "7", SupportRefs: []string{"trace_query:window_stats:E7"}},
		b1695SourceMemberFact("source.go"),
	}
	before, _ := json.Marshal(facts)
	ledger := CompileObservationLedger(ObservationLedgerInput{
		RequestModel: &rm,
		ToolResults: []ToolResult{{ToolName: "trace_query", Success: true,
			Observations: typedTraceProjectionLedgerForAggregateAuthorityTest().Records}},
	})
	if !ledger.HasDirectRuntimeObservation() {
		t.Fatal("fixture must compile an existing producer-owned direct runtime observation")
	}
	ledgerBefore, _ := json.Marshal(ledger)
	kept, withheld := ProjectDirectRuntimeAggregateFacts(facts, &rm, ledger)
	if !withheld || len(kept) != 1 || kept[0].Label != "selected members" {
		t.Fatalf("existing runtime restatement/source distinction changed: %+v withheld=%v", kept, withheld)
	}
	after, _ := json.Marshal(facts)
	ledgerAfter, _ := json.Marshal(ledger)
	if string(before) != string(after) || string(ledgerBefore) != string(ledgerAfter) {
		t.Fatal("source qualification mutated model facts or deterministic trace observations")
	}
}
