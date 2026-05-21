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

func TestCompileObservationLedger_CompilesExistingCarriers(t *testing.T) {
	input := ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:        "e1",
			Source:    "a.go",
			LineStart: 7,
			LineEnd:   8,
			Summary:   "source fact",
			Salience:  SalienceLoadBearing,
		}},
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateNegativeObservation,
			Label: "history no-hit",
			Value: "0",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginVCSMetadata)},
				{Name: "target", Value: "RemovedFeature"},
				{Name: "scope", Value: "HEAD~10..HEAD"},
				{Name: "result_count", Value: "0"},
			},
		}},
		ToolResults: []ToolResult{{
			ToolName: "git_log",
			Success:  true,
			Summary:  "[git_log: evidence_origin=vcs_metadata count=1]\nabc123 latest feature",
		}},
		LogBundle: &LogBundle{Observations: []LogObservation{{
			Kind:      LogObservationRetryCycle,
			Subject:   "finalizer",
			Summary:   "finalizer retried",
			LineStart: 12,
		}}},
		PerfBundle: &PerfBundle{Observations: []PerfObservation{{
			Kind:       "gc",
			Subject:    "GC span",
			Summary:    "GC lasted 8ms",
			LineStart:  5,
			DurationMs: 8,
		}}},
		MCPResponses: []MCPResponse{{
			ServerName: "obs",
			Method:     "read_resource",
			Success:    true,
			Summary:    "cluster status is green",
		}},
	}
	ledger := CompileObservationLedger(input)
	if ledger.Empty() {
		t.Fatal("ledger should contain compiled records")
	}
	assertObservationRecord(t, ledger, "evidence:e1", AnswerEvidenceOriginCurrentSource, ObservationSourceCurrentSource)
	assertObservationRecord(t, ledger, "aggregate:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "tool:0#vcs_metadata", AnswerEvidenceOriginVCSMetadata, ObservationSourceVCSMetadata)
	assertObservationRecord(t, ledger, "log:observation:0", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact)
	assertObservationRecord(t, ledger, "perf:observation:0", AnswerEvidenceOriginRuntimeArtifact, ObservationSourceRuntimeArtifact)
	assertObservationRecord(t, ledger, "mcp:0", AnswerEvidenceOriginMCPResource, ObservationSourceMCPResource)
	if got := findObservationRecord(t, ledger, "evidence:e1"); got.GroundingPolicy != ClaimGroundingHard || got.Span.LineStart != 7 || got.Span.LineEnd != 8 {
		t.Fatalf("source evidence record did not preserve hard policy/span: %+v", got)
	}
	if got := findObservationRecord(t, ledger, "aggregate:0#vcs_metadata"); !got.Negative || got.ResultCount == nil || *got.ResultCount != 0 || got.Scope != "HEAD~10..HEAD" {
		t.Fatalf("negative aggregate record not preserved: %+v", got)
	}
	if len(input.EvidenceItems) != 1 || input.EvidenceItems[0].Summary != "source fact" {
		t.Fatalf("compiler mutated input: %+v", input)
	}
}

func TestCompileObservationLedger_MixedDiffAndCurrentSourceStaySeparate(t *testing.T) {
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:        "current",
			Source:    "internal/current.go",
			LineStart: 21,
			Summary:   "current implementation still calls Apply",
			Salience:  SalienceLoadBearing,
		}},
		ToolResults: []ToolResult{{
			ToolName: "git_diff",
			Success:  true,
			Summary:  "[git_diff: evidence_origin=vcs_diff ref=HEAD~1]\n- old path\n+ new path",
		}},
	})

	current := findObservationRecord(t, ledger, "evidence:current")
	diff := findObservationRecord(t, ledger, "tool:0#vcs_diff")
	if current.Origin != AnswerEvidenceOriginCurrentSource || current.SourceRef.Path != "internal/current.go" {
		t.Fatalf("current-source record drifted: %+v", current)
	}
	if diff.Origin != AnswerEvidenceOriginVCSDiff || diff.SourceRef.Kind != ObservationSourceVCSDiff {
		t.Fatalf("diff record drifted: %+v", diff)
	}
	if diff.GroundingPolicy == ClaimGroundingHard {
		t.Fatalf("diff support record must not become current-source hard gate: %+v", diff)
	}
}

func TestCompileObservationLedger_MixedPositiveAndNegativeExternalFactsStayTargetScoped(t *testing.T) {
	facts, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateScalar,
			Label: "matched incident",
			Value: "INC-7",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginMCPResource)},
				{Name: "target", Value: "open incident"},
				{Name: "scope", Value: "resource://obs/incidents"},
			},
		},
		{
			Kind:  AnswerAggregateNegativeObservation,
			Label: "no fatal alert",
			Value: "0",
			Role:  AnswerAggregateRolePrincipalAnswer,
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: string(AnswerEvidenceOriginMCPResource)},
				{Name: "target", Value: "fatal alert"},
				{Name: "scope", Value: "resource://obs/incidents"},
				{Name: "result_count", Value: "0"},
			},
		},
	})
	if err != nil {
		t.Fatalf("facts should normalize: %v", err)
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{AggregateFacts: facts})

	positive := findObservationRecord(t, ledger, "aggregate:0#mcp_resource")
	negative := findObservationRecord(t, ledger, "aggregate:1#mcp_resource")
	if positive.Negative {
		t.Fatalf("positive fact became negative: %+v", positive)
	}
	if !negative.Negative || negative.Subject != "fatal alert" {
		t.Fatalf("negative fact lost target scope: %+v", negative)
	}
	if positive.Subject == negative.Subject {
		t.Fatalf("positive and negative facts collapsed to one target: %+v / %+v", positive, negative)
	}
}

func assertObservationRecord(t *testing.T, ledger ObservationLedger, id string, origin AnswerEvidenceOrigin, source ObservationSourceKind) {
	t.Helper()
	record := findObservationRecord(t, ledger, id)
	if record.Origin != origin {
		t.Fatalf("%s origin = %q, want %q", id, record.Origin, origin)
	}
	if record.SourceRef.Kind != source {
		t.Fatalf("%s source kind = %q, want %q", id, record.SourceRef.Kind, source)
	}
}

func findObservationRecord(t *testing.T, ledger ObservationLedger, id string) ObservationRecord {
	t.Helper()
	for _, record := range ledger.Records {
		if record.ID == id {
			return record
		}
	}
	t.Fatalf("record %q not found in %+v", id, ledger.Records)
	return ObservationRecord{}
}
