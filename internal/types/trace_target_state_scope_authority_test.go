package types

import (
	"fmt"
	"math"
	"testing"
)

func TestBuildTraceTargetStateScopeAuthoritiesPreservesThreadLocalScope(t *testing.T) {
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{
		{
			ArtifactPath:  "/tmp/tieba.systrace",
			ArtifactLabel: "tieba.systrace",
			TargetStateAccount: &TraceCausalProjectionTargetStateAccount{
				Subject:       "com.baidu.tieba-59566",
				WindowStartTs: 34579.472865,
				WindowEndTs:   34579.587805,
				RunningMS:     26.946,
				RunnableMS:    3.636,
				SleepMS:       84.358,
				TotalMS:       114.940,
				EvidenceID:    "target-state",
			},
		},
		{
			ArtifactPath: "/tmp/empty.systrace",
		},
	}}

	got := BuildTraceTargetStateScopeAuthorities(set)
	if len(got) != 1 {
		t.Fatalf("authority count=%d, want 1: %+v", len(got), got)
	}
	if got[0].Subject != "com.baidu.tieba-59566" ||
		got[0].RunningMS != 26.946 ||
		got[0].RunnableMS != 3.636 ||
		got[0].TotalMS != 114.940 ||
		got[0].CoverageStatus != "complete" ||
		got[0].UnaccountedMS != 0 {
		t.Fatalf("thread-local account drifted: %+v", got[0])
	}
}

func TestBuildTraceTargetStateScopeAuthoritiesDisclosesPartialCoverageAndRejectsOverWindow(t *testing.T) {
	build := func(total float64) TraceCausalProjectionSet {
		return TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
			ArtifactPath: "/tmp/trace.systrace", ArtifactLabel: "trace.systrace",
			TargetStateAccount: &TraceCausalProjectionTargetStateAccount{
				Subject: "worker-200", WindowStartTs: 1, WindowEndTs: 1.1,
				RunningMS: 40, SleepMS: total - 40, TotalMS: total,
				TailOpenMS: 5, TailOpenState: "sleep", EvidenceID: "state",
			},
		}}}
	}
	got := BuildTraceTargetStateScopeAuthorities(build(90))
	if len(got) != 1 || got[0].CoverageStatus != "partial_unaccounted" ||
		math.Abs(got[0].WindowMS-100) > 0.001 ||
		math.Abs(got[0].UnaccountedMS-10) > 0.001 ||
		got[0].TailOpenMS != 5 || got[0].TailOpenState != "sleep" {
		t.Fatalf("partial state coverage must stay typed: %+v", got)
	}
	if over := BuildTraceTargetStateScopeAuthorities(build(100.01)); len(over) != 0 {
		t.Fatalf("over-window state account must fail closed: %+v", over)
	}
}

func TestBuildTraceTargetStateScopeAuthoritiesFromLedgerUsesTypedExplicitWindowWithoutCausalProjection(t *testing.T) {
	ledger := ObservationLedger{
		Records: []ObservationRecord{
			requestedWindowAuthorityRecord(
				"state-exploration", "target_window_states", "ui-100", "state_partition", "50.000",
				"selected_window=10.020000..10.070000", "running=10.000", "runnable=5.000",
				"sleep=35.000", "d_state=0.000", "io_wait=0.000", "total=50.000",
			),
			requestedWindowAuthorityRecord(
				"state-requested", "target_window_states", "main-100", "state_partition", "100.000",
				"selected_window=10.000000..10.100000", "running=20.000", "runnable=10.000",
				"sleep=70.000", "d_state=0.000", "io_wait=0.000", "sleep_io_wait=3.000", "total=100.000",
			),
		},
		AnchorUserEntities:          []AnchorUserEntity{{Value: "ui-100", TypedLane: true}},
		RuntimeArtifactScopeProfile: requestedWindowAuthorityProfile(),
	}
	got := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger)
	if len(got) != 1 {
		t.Fatalf("finite explicit-window authority count=%d, want 1: %+v", len(got), got)
	}
	if got[0].Subject != "main-100" || got[0].RunningMS != 20 || got[0].SleepMS != 70 ||
		got[0].SleepIOWaitMS != 3 || got[0].TotalMS != 100 || got[0].CoverageStatus != "complete" {
		t.Fatalf("finite explicit-window state account drifted: %+v", got[0])
	}
}

func TestBuildTraceTargetStateScopeAuthoritiesFromLedgerFailsClosedWithoutTypedTargetOrRequestedWindow(t *testing.T) {
	record := requestedWindowAuthorityRecord(
		"state-requested", "target_window_states", "ui-100", "state_partition", "100.000",
		"selected_window=10.000000..10.100000", "running=20.000", "runnable=10.000",
		"sleep=70.000", "d_state=0.000", "io_wait=0.000", "total=100.000",
	)
	for _, ledger := range []ObservationLedger{
		{Records: []ObservationRecord{record}, RuntimeArtifactScopeProfile: requestedWindowAuthorityProfile()},
		{
			Records:            []ObservationRecord{record},
			AnchorUserEntities: []AnchorUserEntity{{Value: "ui-100", TypedLane: true}},
		},
		{
			Records:                     []ObservationRecord{record},
			AnchorUserEntities:          []AnchorUserEntity{{Value: "worker-200", TypedLane: true}},
			RuntimeArtifactScopeProfile: requestedWindowAuthorityProfile(),
		},
	} {
		if got := BuildTraceTargetStateScopeAuthoritiesFromLedger(ledger); len(got) != 0 {
			t.Fatalf("finite authority must fail closed without both exact scope and target: %+v", got)
		}
	}
}

func TestBuildTraceTargetWaitSummaryAuthoritiesUsesCompleteSameResultRows(t *testing.T) {
	target := "CompThread_0-2955"
	ref := ObservationSourceRef{
		Kind: ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace",
		Path: "/tmp/attached_trace.txt",
	}
	count := 3
	aggregate := ObservationRecord{
		ID:              "trace_query:scope#target_window_wait_occurrences",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		SourceRef:       ref,
		Span:            ObservationSpan{StartTs: 10, EndTs: 10.1},
		Predicate:       "target_window_wait_occurrences",
		Subject:         target,
		Object:          "complete",
		Value:           "3",
		ResultCount:     &count,
	}
	records := []ObservationRecord{aggregate}
	for i, bounds := range [][3]float64{
		{10.001, 10.002, 1.000},
		{10.010, 10.012, 2.000},
		{10.020, 10.023, 3.000},
	} {
		records = append(records, ObservationRecord{
			ID:              fmt.Sprintf("trace_query:scope#target_window_wait_occurrence:%d", i+1),
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ref,
			Span:            ObservationSpan{StartTs: bounds[0], EndTs: bounds[1]},
			Predicate:       "target_window_wait_occurrence",
			Subject:         target,
			Object:          "state=d_sleep;iowait=0;caller=dma_fence_default_w",
			Value:           fmt.Sprintf("%.3f", bounds[2]),
			Unit:            "ms",
		})
	}
	rm := RequestModel{RuntimeTargets: []RuntimeTarget{{
		Kind: RuntimeTargetKindThread, PID: 2955, Thread: target,
	}}}
	got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, &rm)
	if len(got) != 1 || got[0].Count != 3 || got[0].DStateOccurrences != 3 ||
		got[0].WallClockMS != 6 || len(got[0].Callers) != 1 ||
		got[0].Callers[0] != "dma_fence_default_w" ||
		got[0].ArtifactLabel != "attached_trace.txt" ||
		len(got[0].Occurrences) != 3 ||
		got[0].Occurrences[2].CanonicalLine() !=
			"#3 state=d_sleep 10.020000..10.023000 duration=3.000ms iowait=0 caller=dma_fence_default_w" {
		t.Fatalf("complete typed wait summary drifted: %+v", got)
	}
}

func TestBuildTraceTargetWaitSummaryAuthoritiesElectsRequestedScopeWithoutTimestampGuessing(t *testing.T) {
	target := "com.baidu.tieba-59566"
	ref := ObservationSourceRef{
		Kind: ObservationSourceRuntimeArtifact, ArtifactID: "attached_trace",
		Path: "/tmp/attached_trace.txt",
	}
	makeRoster := func(scope string, start, end float64, durations []float64, supplement bool) []ObservationRecord {
		count := len(durations)
		aggregate := ObservationRecord{
			ID:     "trace_query:" + scope + "#target_window_wait_occurrences",
			Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: ClaimGroundingHard, SourceRef: ref,
			Span:      ObservationSpan{StartTs: start, EndTs: end},
			Predicate: "target_window_wait_occurrences", Subject: target,
			Object: "complete", Value: fmt.Sprintf("%d", count), ResultCount: &count,
			SystemSupplement: supplement,
		}
		records := []ObservationRecord{aggregate}
		cursor := start + 0.001
		for i, duration := range durations {
			rowEnd := cursor + duration/1000
			records = append(records, ObservationRecord{
				ID:     fmt.Sprintf("trace_query:%s#target_window_wait_occurrence:%d", scope, i+1),
				Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
				GroundingPolicy: ClaimGroundingHard, SourceRef: ref,
				Span:      ObservationSpan{StartTs: cursor, EndTs: rowEnd},
				Predicate: "target_window_wait_occurrence", Subject: target,
				Object: "state=io_wait;iowait=1;caller=sync_buffer_read_wi",
				Value:  fmt.Sprintf("%.3f", duration), Unit: "ms",
				SystemSupplement: supplement,
			})
			cursor = rowEnd + 0.001
		}
		return records
	}

	narrow := makeRoster("narrow", 10, 10.02, []float64{0.2}, false)
	full := makeRoster("full", 10, 10.1, []float64{0.2, 0.3}, true)
	records := append(append([]ObservationRecord(nil), narrow...), full...)
	rm := RequestModel{
		RuntimeTargets: []RuntimeTarget{{
			Kind: RuntimeTargetKindThread, PID: 59566, Thread: target,
		}},
		RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{
			RequestedScope: RuntimeArtifactScopeFullArtifact,
			SourceQuote:    "这份 trace",
		},
	}
	got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, &rm)
	if len(got) != 2 ||
		!got[0].IsRequestedScopePrincipal() ||
		got[0].WindowEndTs != 10.1 ||
		got[1].RequestedScopeRole != TraceTargetWaitScopeSupportingExploration {
		t.Fatalf("full-artifact supplement must lead and the narrow query must remain supporting: %+v", got)
	}

	// A non-supplement query can earn the same role only through a SAME-result
	// deterministic full-artifact coverage sibling.
	for i := range records {
		records[i].SystemSupplement = false
	}
	records = append(records, ObservationRecord{
		ID:     "trace_query:full#runtime_artifact_scope_coverage",
		Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard, SourceRef: ref,
		Predicate: RuntimeArtifactScopeCoveragePredicate,
		Object:    string(RuntimeArtifactScopeFullArtifact),
		Scope:     string(RuntimeArtifactScopeFullArtifact),
	})
	got = BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, &rm)
	if len(got) != 2 || !got[0].IsRequestedScopePrincipal() || got[0].WindowEndTs != 10.1 {
		t.Fatalf("same-result full coverage did not elect the matching rowset: %+v", got)
	}

	// A coverage record from another query cannot crown the wider rowset by
	// size alone.
	records[len(records)-1].ID = "trace_query:other#runtime_artifact_scope_coverage"
	got = BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, &rm)
	for _, authority := range got {
		if authority.IsRequestedScopePrincipal() {
			t.Fatalf("unrelated coverage record minted requested-scope authority: %+v", got)
		}
	}

	explicitStart, explicitEnd := 10.0, 10.02
	rm.RuntimeArtifactScopeProfile = &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &explicitStart, TimeEnd: &explicitEnd, SourceQuote: "10..10.02",
	}
	got = BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: records}, &rm)
	if len(got) != 2 || !got[0].IsRequestedScopePrincipal() ||
		got[0].WindowEndTs != explicitEnd ||
		got[1].RequestedScopeRole != TraceTargetWaitScopeSupportingExploration {
		t.Fatalf("explicit requested window election drifted: %+v", got)
	}
}

func TestBuildTraceTargetWaitSummaryAuthoritiesFailsClosedOnMissingOrConflictingRows(t *testing.T) {
	target := "worker-200"
	count := 2
	ref := ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "trace"}
	aggregate := ObservationRecord{
		ID: "trace_query:scope#target_window_wait_occurrences", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: ref,
		Span: ObservationSpan{StartTs: 1, EndTs: 2}, Predicate: "target_window_wait_occurrences",
		Subject: target, Object: "complete", Value: "2", ResultCount: &count,
	}
	row := ObservationRecord{
		ID: "trace_query:scope#target_window_wait_occurrence:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: ref,
		Span: ObservationSpan{StartTs: 1.1, EndTs: 1.101}, Predicate: "target_window_wait_occurrence",
		Subject: target, Object: "state=d_sleep;iowait=0;caller=fence", Value: "1.000", Unit: "ms",
	}
	rm := RequestModel{RuntimeTargets: []RuntimeTarget{{Kind: RuntimeTargetKindThread, PID: 200, Thread: target}}}
	if got := BuildTraceTargetWaitSummaryAuthorities(ObservationLedger{Records: []ObservationRecord{aggregate, row}}, &rm); len(got) != 0 {
		t.Fatalf("missing occurrence row must fail closed: %+v", got)
	}
	conflict := row
	conflict.Value = "2.000"
	if got := BuildTraceTargetWaitSummaryAuthorities(
		ObservationLedger{Records: []ObservationRecord{aggregate, row, conflict}}, &rm,
	); len(got) != 0 {
		t.Fatalf("conflicting duplicate occurrence row must fail closed: %+v", got)
	}
	second := row
	second.ID = "trace_query:scope#target_window_wait_occurrence:2"
	second.Span = ObservationSpan{StartTs: 1.2, EndTs: 1.201}
	second.SourceRef.ArtifactID = "other-trace"
	if got := BuildTraceTargetWaitSummaryAuthorities(
		ObservationLedger{Records: []ObservationRecord{aggregate, row, second}}, &rm,
	); len(got) != 0 {
		t.Fatalf("cross-artifact rows must not complete a roster: %+v", got)
	}
	second.SourceRef = ref
	second.Span = ObservationSpan{StartTs: 2.1, EndTs: 2.101}
	if got := BuildTraceTargetWaitSummaryAuthorities(
		ObservationLedger{Records: []ObservationRecord{aggregate, row, second}}, &rm,
	); len(got) != 0 {
		t.Fatalf("out-of-window occurrence rows must fail closed: %+v", got)
	}
}
