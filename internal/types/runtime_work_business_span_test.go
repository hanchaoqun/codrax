package types

import (
	"fmt"
	"reflect"
	"testing"
)

func measuredBusinessSpanFixture() ObservationRecord {
	return ObservationRecord{
		ID: "trace_query:work#trace_business_span:1", Producer: "trace_query",
		Origin: AnswerEvidenceOriginRuntimeArtifact, Role: AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard, ProvenanceLane: ObservationProvenanceArtifactSpan,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/trace/a", ArtifactID: "capture-a", QueryScopeID: "query-a", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.1},
		Span:      ObservationSpan{LineStart: 4, LineEnd: 8, StartTs: 1, EndTs: 1.05},
		Predicate: TraceBusinessSpanPredicate, ClaimKey: TraceBusinessSpanPredicate + ":Business-X",
		Subject: "worker-200", Object: "Business-X", Value: "50.000", Unit: "ms",
		RichNotes:   []string{TraceNoteKeySelectedWindow + "=1.000000..1.100000"},
		SupportRefs: []string{"/trace/a:4-8"},
	}
}

func businessSpanContractInput(rows ...ObservationRecord) ObservationLedgerInput {
	return ObservationLedgerInput{ToolResults: []ToolResult{{ToolName: "trace_query", Success: true, Observations: rows}}}
}

func TestRuntimeWorkBusinessSpanRetainsFactWithoutCausalAuthority(t *testing.T) {
	input := businessSpanContractInput(measuredBusinessSpanFixture())
	contract := BuildRuntimeWorkRelationContract(input, true)
	if contract == nil || len(contract.Rows) != 1 || contract.Rows[0].MeasuredDurationMS != 50 || contract.Rows[0].WorkLabel != "Business-X" {
		t.Fatalf("missing ordinary exact work row: %+v", contract)
	}
	row := contract.Rows[0]
	if row.Credential != "none" || !reflect.DeepEqual(row.AllowedConclusions, []RuntimeWorkRelationConclusion{RuntimeWorkRelationConclusionRelationUnproven}) {
		t.Fatalf("measured duration acquired relation authority: %+v", row)
	}
	for _, conclusion := range []RuntimeWorkRelationConclusion{RuntimeWorkRelationConclusionTargetSelfWorkObserved, RuntimeWorkRelationConclusionRelatedCausalityUnproven, RuntimeWorkRelationConclusionCausalContributionSupported} {
		if BindRuntimeWorkRelationReceipt(&AnswerRuntimeWorkRelationReceipt{ObservationID: row.ObservationID, Conclusion: conclusion}, contract) {
			t.Fatalf("ordinary interval accepted stronger conclusion %q", conclusion)
		}
	}
	if BuildRuntimeWorkRelationContract(input, false) != nil {
		t.Fatal("ordinary facts created an unrequested relationship obligation")
	}
	for _, projection := range CompileTraceCausalProjectionSet(CompileObservationLedger(input)).Projections {
		if projection.PrimaryRootCause != nil || len(projection.PrimaryRootCauses)+len(projection.RankedSeats)+len(projection.OnChainCauses)+len(projection.SemanticSpans)+len(projection.WakeupPath) != 0 {
			t.Fatalf("ordinary interval created causal/semantic content: %+v", projection)
		}
	}
}

func TestRuntimeWorkBusinessSpanUsesSharedProducerIdentity(t *testing.T) {
	for _, producer := range []string{"trace_query", "trace_query:run2"} {
		t.Run(producer, func(t *testing.T) {
			row := measuredBusinessSpanFixture()
			row.Producer = producer
			contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(row), true)
			if contract == nil || len(contract.Rows) != 1 || contract.Rows[0].ObservationID != row.ID {
				t.Fatalf("deterministic producer alias lost its measured work: %+v", contract)
			}
		})
	}
}

func TestRuntimeWorkBusinessSpanDedupKeepsWindowAndCaptureAxes(t *testing.T) {
	a := measuredBusinessSpanFixture()
	copyQuery := a
	copyQuery.ID, copyQuery.SourceRef.QueryScopeID = "query-b-span", "query-b"
	b := a
	b.ID, b.SourceRef.Path, b.SourceRef.ArtifactID = "capture-b-span", "/trace/b", "capture-b"
	c := a
	c.ID, c.RichNotes = "other-window-span", []string{TraceNoteKeySelectedWindow + "=0.900000..1.100000"}
	c.SourceRef.QueryScopeID, c.SourceRef.QueryWindowStartTs = "query-c", 0.9
	contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(a, copyQuery, b, c), true)
	if contract == nil || len(contract.Rows) != 3 {
		t.Fatalf("duplicate queries multiplied work or distinct source/windows collapsed: %+v", contract)
	}
}

func TestRuntimeWorkBusinessSpanDedupPreservesPreciseQueryWindows(t *testing.T) {
	a, b := measuredBusinessSpanFixture(), measuredBusinessSpanFixture()
	a.SourceRef.QueryWindowEndTs = 1.1000001
	b.ID, b.SourceRef.QueryScopeID, b.SourceRef.QueryWindowEndTs = "precise-other-window", "query-precise", 1.1000002
	contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(a, b), true)
	if contract == nil || len(contract.Rows) != 2 {
		t.Fatalf("distinct precise query windows collapsed through rounded display: %+v", contract)
	}
}

func TestRuntimeWorkBusinessSpanRejectsUnmeasuredOrWrongScopeRows(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ObservationRecord)
	}{
		{"model row", func(r *ObservationRecord) { r.Producer = "aggregate_facts" }},
		{"wrong origin", func(r *ObservationRecord) { r.Origin = AnswerEvidenceOriginSystemInference }},
		{"inferred lane", func(r *ObservationRecord) { r.ProvenanceLane = ObservationProvenanceInferredUpstreamPossibility }},
		{"no source", func(r *ObservationRecord) { r.SourceRef.Path = "" }},
		{"no physical pair", func(r *ObservationRecord) { r.Span.LineEnd = 0 }},
		{"missing window", func(r *ObservationRecord) { r.RichNotes = nil }},
		{"wrong unit", func(r *ObservationRecord) { r.Unit = "cpu·ms" }},
		{"wrong duration", func(r *ObservationRecord) { r.Value = "100" }},
		{"nan", func(r *ObservationRecord) { r.Value = "NaN" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := measuredBusinessSpanFixture()
			tc.edit(&r)
			if got := BuildRuntimeWorkRelationContract(businessSpanContractInput(r), true); got != nil {
				t.Fatalf("unsupported row became selectable: %+v", got)
			}
		})
	}
	start, end := 1.01, 1.02
	input := businessSpanContractInput(measuredBusinessSpanFixture())
	input.RequestModel = &RequestModel{RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "1.01..1.02"}}
	if got := BuildRuntimeWorkRelationContract(input, true); got != nil {
		t.Fatalf("older broad query became a narrow-window selectable work row: %+v", got)
	}
	start, end = 1, 1.1
	secondStart, secondEnd := 2.0, 2.1
	input.RequestModel.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{RequestedScope: RuntimeArtifactScopeExplicitWindow, TimeWindows: []RuntimeArtifactTimeWindow{{TimeStart: &start, TimeEnd: &end, SourceQuote: "first"}, {TimeStart: &secondStart, TimeEnd: &secondEnd, SourceQuote: "second"}}}
	r := measuredBusinessSpanFixture()
	r.SourceRef.QueryWindowStartTs, r.SourceRef.QueryWindowEndTs = 0.9, 2.2
	input.ToolResults = []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{r}}}
	if got := BuildRuntimeWorkRelationContract(input, true); got != nil {
		t.Fatalf("multi-window query envelope leaked a child work choice: %+v", got)
	}
}

func TestRuntimeWorkBusinessSpanChoiceCapMatchesVisibleFacts(t *testing.T) {
	var rows []ObservationRecord
	for i := 0; i < TraceBusinessSpanFactLimit+2; i++ {
		r := measuredBusinessSpanFixture()
		r.ID, r.Object = fmt.Sprintf("span-%02d", i), fmt.Sprintf("Work-%02d", i)
		rows = append(rows, r)
	}
	contract := BuildRuntimeWorkRelationContract(businessSpanContractInput(rows...), true)
	if contract == nil || len(contract.Rows) != TraceBusinessSpanFactLimit {
		t.Fatalf("choice cap diverged: %+v", contract)
	}
	for _, row := range contract.Rows {
		if row.WorkLabel == "Work-16" || row.WorkLabel == "Work-17" {
			t.Fatalf("hidden tail work became a choice: %+v", row)
		}
	}
}
