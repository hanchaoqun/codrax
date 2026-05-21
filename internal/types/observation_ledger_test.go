package types

import "testing"

func TestObservationLedgerExternalOriginsAreValidAndNonSource(t *testing.T) {
	for _, tc := range []struct {
		origin AnswerEvidenceOrigin
		source ObservationSourceKind
	}{
		{AnswerEvidenceOriginExternalDocument, ObservationSourceExternalDocument},
		{AnswerEvidenceOriginWebPage, ObservationSourceWebPage},
		{AnswerEvidenceOriginMCPResource, ObservationSourceMCPResource},
		{AnswerEvidenceOriginConnectorResource, ObservationSourceConnector},
	} {
		if !tc.origin.IsValid() {
			t.Fatalf("origin %q should be valid", tc.origin)
		}
		if got := ObservationSourceKindForOrigin(tc.origin); got != tc.source {
			t.Fatalf("source kind for %q = %q, want %q", tc.origin, got, tc.source)
		}
		if got := AnswerClaimBindingGroundingPolicy(tc.origin, AnswerAggregateRolePrincipalAnswer); got != ClaimGroundingRepairable {
			t.Fatalf("principal grounding policy for %q = %q, want repairable", tc.origin, got)
		}
		if got := AnswerClaimBindingGroundingPolicy(tc.origin, AnswerAggregateRoleSupportingCoverage); got != ClaimGroundingSoft {
			t.Fatalf("support grounding policy for %q = %q, want soft", tc.origin, got)
		}
	}
}

func TestAnswerAggregateFactEvidenceOrigins_ExternalResourceTokens(t *testing.T) {
	facts := []struct {
		token string
		want  AnswerEvidenceOrigin
	}{
		{"external_document", AnswerEvidenceOriginExternalDocument},
		{"web_page", AnswerEvidenceOriginWebPage},
		{"mcp_resource", AnswerEvidenceOriginMCPResource},
		{"connector_resource", AnswerEvidenceOriginConnectorResource},
	}
	for _, tc := range facts {
		fact := AnswerAggregateFact{
			Kind:  AnswerAggregateScalar,
			Label: "external fact",
			Value: "present",
			Dimensions: []AnswerAggregateDimension{{
				Name:  "origin",
				Value: tc.token,
			}},
		}
		got := AnswerAggregateFactEvidenceOrigins(fact, nil)
		if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("origin token %q projected to %+v, want %q", tc.token, got, tc.want)
		}
	}
}

func TestNormalizeAnswerAggregateFacts_NegativeObservationAllowsExternalOrigins(t *testing.T) {
	for _, origin := range []AnswerEvidenceOrigin{
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource,
	} {
		facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
			Kind:  AnswerAggregateNegativeObservation,
			Label: "external no-hit",
			Value: "0",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(origin)},
				{Name: "target", Value: "MissingThing"},
				{Name: "scope", Value: "bounded external resource"},
				{Name: "result_count", Value: "0"},
			},
		}})
		if err != nil {
			t.Fatalf("negative_observation should allow origin %q: %v", origin, err)
		}
		if len(facts) != 1 {
			t.Fatalf("facts len for %q = %d, want 1", origin, len(facts))
		}
		got := AnswerAggregateFactEvidenceOrigins(facts[0], nil)
		if len(got) != 1 || got[0] != origin {
			t.Fatalf("normalized origin for %q = %+v", origin, got)
		}
	}
}

func TestCompileObservationLedgerSkeletonIsSideEffectFree(t *testing.T) {
	input := ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:        "e1",
			Source:    "a.go",
			LineStart: 7,
			Summary:   "source fact",
		}},
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateScalar,
			Label: "value",
			Value: "42",
		}},
	}
	ledger := CompileObservationLedger(input)
	if !ledger.Empty() {
		t.Fatalf("Batch-1 skeleton should not populate records yet: %+v", ledger)
	}
	if len(input.EvidenceItems) != 1 || input.EvidenceItems[0].Summary != "source fact" {
		t.Fatalf("compiler mutated input: %+v", input)
	}
}
