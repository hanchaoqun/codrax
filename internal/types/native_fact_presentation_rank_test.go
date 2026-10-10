package types

import (
	"fmt"
	"reflect"
	"testing"
)

func presentationTraceRecord(query, predicate string, ordinal int) ObservationRecord {
	return ObservationRecord{
		ID:     fmt.Sprintf("trace_query:%s#%s:%d", query, predicate, ordinal),
		Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		Role: AnswerAggregateRoleSupportingCoverage, GroundingPolicy: ClaimGroundingHard,
		ClaimAuthority: ObservationClaimAuthorityDirectObservation,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact,
			Path: "/captures/shared.trace", PayloadRef: "/results/" + query, QueryScopeID: query},
		Subject: "worker-77", Predicate: predicate, Summary: "Producer-owned observation",
	}
}

func TestNativeFactPresentationPreservesRequestedRankQuotas(t *testing.T) {
	rm := &RequestModel{
		RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 41, Thread: "target-41", Source: "user_explicit"}},
		RuntimeQuestionProfile: &RuntimeQuestionProfile{Scope: RuntimeQuestionScopeBoundedFactSet,
			FactFamilies: []RuntimeQuestionFactFamily{RuntimeQuestionFactCountOrDuration}},
	}
	var records []ObservationRecord
	wantIDs := map[string]bool{}
	for _, query := range []string{"first", "second"} {
		for i := 0; i < 2; i++ {
			record := presentationTraceRecord(query, "ipc_request_census", i)
			record.Subject = "target-41"
			count := i + 1
			record.ResultCount = &count
			records = append(records, record)
			wantIDs[record.ID] = true
		}
		// Several lower-priority groups must not trade their first rows for
		// a second row of an already-selected requested census group.
		for i := 0; i < 3; i++ {
			records = append(records, presentationTraceRecord(query, fmt.Sprintf("other_fact_%d", i), i))
		}
	}
	before := append([]ObservationRecord(nil), records...)
	got := PrioritizeObservationRecords(records, rm, nil, 6)
	for _, record := range got {
		delete(wantIDs, record.ID)
	}
	if len(got) != 6 || len(wantIDs) != 0 {
		t.Fatalf("source/group coverage displaced requested priority slots: missing=%v got=%v", wantIDs, got)
	}
	if !reflect.DeepEqual(records, before) {
		t.Fatal("display rebalancing mutated the accepted ledger")
	}
}

func TestNativeFactPresentationFairnessStaysInsideSelectedRank(t *testing.T) {
	var rows []ObservationRecord
	for _, query := range []string{"first", "second"} {
		for i := 0; i < 4; i++ {
			rows = append(rows, presentationTraceRecord(query, "same_rank", i))
		}
	}
	lower := presentationTraceRecord("third", "lower_rank", 0)
	lower.Summary = "" // No summary preference: a distinct lower rank.
	rows = append(rows, lower)
	current := ObservationRecord{ID: "current:anchor", Origin: AnswerEvidenceOriginCurrentSource}
	selected := []ObservationRecord{rows[0], current, rows[1], rows[2], rows[3], lower}
	before := append([]ObservationRecord(nil), selected...)
	got := rebalanceNativePresentationRecords(rows, selected, nil, nil)
	counts := map[string]int{}
	for _, record := range got {
		if IsNativeRuntimeFactPresentationRecord(record) {
			counts[record.SourceRef.QueryScopeID]++
		}
	}
	if len(got) != len(selected) || got[1].ID != current.ID || got[5].ID != lower.ID ||
		counts["first"] != 2 || counts["second"] != 2 || counts["third"] != 1 {
		t.Fatalf("rank quotas or non-native floor changed instead of same-rank fairness: %v; rows=%v", counts, got)
	}
	if !reflect.DeepEqual(selected, before) {
		t.Fatal("display rebalancing mutated the caller's selection")
	}
	// A rank that was awarded no slot cannot acquire one just because its
	// source is new. The two already-selected sources still share fairly.
	got = rebalanceNativePresentationRecords(rows, selected[:5], nil, nil)
	for _, record := range got {
		if record.ID == lower.ID {
			t.Fatal("previously unselected rank acquired a source-coverage slot")
		}
	}
}
