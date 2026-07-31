package types

import "testing"

func targetWaitAuthorityFixtureRecord(id string, thirdStart, thirdEnd string) ObservationRecord {
	count := 3
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		ClaimKey:        "target_window_wait_occurrences:main-59566",
		Subject:         "main-59566",
		Predicate:       "target_window_wait_occurrences",
		Object:          "complete",
		Value:           "3",
		ResultCount:     &count,
		RichNotes: []string{
			TraceNoteKeyTargetWaitOccurrencePrompt + "=status=complete,emitted=3,total=3",
			TraceNoteKeyTargetWaitOccurrencePromptSum + "=0.635",
			TraceNoteKeyTargetWaitOccurrence + "=#1 state=io_wait 34579.451701..34579.451839 duration=0.138ms iowait=1 caller=sync_buffer_read_wi lines=1-2 reason_line=3",
			TraceNoteKeyTargetWaitOccurrence + "=#2 state=io_wait 34579.452934..34579.453081 duration=0.147ms iowait=1 caller=sync_buffer_read_wi lines=4-5 reason_line=6",
			TraceNoteKeyTargetWaitOccurrence + "=#3 state=io_wait " + thirdStart + ".." + thirdEnd + " duration=0.350ms iowait=1 caller=sync_buffer_read_wi lines=7-8 reason_line=9",
		},
	}
}

func targetWaitAuthorityFixtureRequestModel() RequestModel {
	return RequestModel{RuntimeTargets: []RuntimeTarget{{
		Kind:   RuntimeTargetKindThread,
		PID:    59566,
		Thread: "main-59566",
		Source: "user_explicit",
	}}}
}

func TestBuildTargetWaitOccurrenceAuthoritiesDedupesCompleteTargetRoster(t *testing.T) {
	record := targetWaitAuthorityFixtureRecord("trace_query:one", "34579.471372", "34579.471722")
	duplicate := record
	duplicate.ID = "trace_query:two"
	got := BuildTargetWaitOccurrenceAuthorities(
		ObservationLedger{Records: []ObservationRecord{record, duplicate}},
		func() *RequestModel {
			rm := targetWaitAuthorityFixtureRequestModel()
			return &rm
		}(),
	)
	if len(got) != 1 || got[0].Count != 3 || len(got[0].Rows) != 3 ||
		got[0].Rows[2].StartToken() != "34579.471372" ||
		got[0].Rows[2].EndToken() != "34579.471722" {
		t.Fatalf("complete target authority not preserved: %+v", got)
	}
}

func TestBuildTargetWaitOccurrenceAuthoritiesFailsOpenOnConflictingCompleteRosters(t *testing.T) {
	one := targetWaitAuthorityFixtureRecord("trace_query:one", "34579.471372", "34579.471722")
	two := targetWaitAuthorityFixtureRecord("trace_query:two", "34579.471723", "34579.471876")
	rm := targetWaitAuthorityFixtureRequestModel()
	if got := BuildTargetWaitOccurrenceAuthorities(
		ObservationLedger{Records: []ObservationRecord{one, two}},
		&rm,
	); len(got) != 0 {
		t.Fatalf("conflicting complete rosters must not become hard authority: %+v", got)
	}
}

func TestCheckTargetWaitOccurrencePrincipalConsistencyRejectsWrongIntervalRelation(t *testing.T) {
	record := targetWaitAuthorityFixtureRecord("trace_query:one", "34579.471372", "34579.471722")
	rm := targetWaitAuthorityFixtureRequestModel()
	authorities := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{record}}, &rm)
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{ID: "summary", Kind: BlockSummary, Text: "3 段 io_wait，总量 0.635ms"},
		{
			ID: "rows", Kind: BlockOrderedList, SurfaceRole: SurfacePrincipal,
			Items: []AnswerBlockItem{
				{ID: "one", Text: "34579.451701..34579.451839，0.138ms"},
				{ID: "two", Text: "34579.452934..34579.453081，0.147ms"},
				{ID: "three", Text: "34579.471723..34579.471876，0.350ms"},
			},
		},
	}}
	issues := CheckTargetWaitOccurrencePrincipalConsistency(authorities, doc)
	if len(issues) != 1 || len(issues[0].Missing) != 1 || len(issues[0].Conflicts) != 1 {
		t.Fatalf("wrong interval relation was not rejected precisely: %+v", issues)
	}
}

func TestCheckTargetWaitOccurrencePrincipalConsistencyAcceptsExactRoster(t *testing.T) {
	record := targetWaitAuthorityFixtureRecord("trace_query:one", "34579.471372", "34579.471722")
	rm := targetWaitAuthorityFixtureRequestModel()
	authorities := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{record}}, &rm)
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "rows", Kind: BlockOrderedList, SurfaceRole: SurfacePrincipal,
		Items: []AnswerBlockItem{
			{ID: "one", Text: "34579.451701..34579.451839，0.138ms"},
			{ID: "two", Text: "34579.452934..34579.453081，0.147ms"},
			{ID: "three", Text: "34579.471372..34579.471722，0.350ms"},
		},
	}}}
	if issues := CheckTargetWaitOccurrencePrincipalConsistency(authorities, doc); len(issues) != 0 {
		t.Fatalf("exact roster should pass: %+v", issues)
	}
}

func TestCheckTargetWaitOccurrencePrincipalConsistencyDoesNotConfuseEqualDurations(t *testing.T) {
	record := targetWaitAuthorityFixtureRecord("trace_query:one", "34579.471372", "34579.471722")
	record.RichNotes[3] = TraceNoteKeyTargetWaitOccurrence + "=#2 state=io_wait 34579.452934..34579.453072 duration=0.138ms iowait=1 caller=sync_buffer_read_wi lines=4-5 reason_line=6"
	rm := targetWaitAuthorityFixtureRequestModel()
	authorities := BuildTargetWaitOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{record}}, &rm)
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID: "rows", Kind: BlockOrderedList, SurfaceRole: SurfacePrincipal,
		Items: []AnswerBlockItem{
			{ID: "one", Text: "34579.451701..34579.451839，0.138ms"},
			{ID: "two", Text: "34579.452934..34579.453072，0.138ms"},
			{ID: "three", Text: "34579.471372..34579.471722，0.350ms"},
		},
	}}}
	if issues := CheckTargetWaitOccurrencePrincipalConsistency(authorities, doc); len(issues) != 0 {
		t.Fatalf("equal durations with distinct exact intervals should pass: %+v", issues)
	}
}
