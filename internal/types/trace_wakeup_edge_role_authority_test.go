package types

import "testing"

func TestBuildTraceWakeupEdgeRoleAuthoritiesBindsEndpointValues(t *testing.T) {
	rm := &RequestModel{RuntimeTargets: []RuntimeTarget{{
		Kind: RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit",
	}}}
	ledger := ObservationLedger{Records: []ObservationRecord{
		traceWakeupEdgeRoleTestRecord("edge-1", "worker-200", "app-100", "20/ohos_cfs", "52/ohos_rt"),
		traceWakeupEdgeRoleTestRecord("edge-other", "other-300", "other-400", "30/ohos_cfs", "31/ohos_cfs"),
	}}
	got := BuildTraceWakeupEdgeRoleAuthorities(ledger, rm)
	if len(got) != 1 {
		t.Fatalf("expected one target-bound edge, got %+v", got)
	}
	edge := got[0]
	if edge.Waker != "worker-200" || edge.Wakee != "app-100" ||
		edge.WakerPriority != "20/ohos_cfs" || edge.WakeePriority != "52/ohos_rt" ||
		edge.WakerCPU != "2" || edge.WakeeTargetCPU != "1" || edge.CPURelation != "cross_cpu" {
		t.Fatalf("endpoint roles or values drifted: %+v", edge)
	}
}

func TestBuildTraceWakeupEdgeRoleAuthoritiesFailsClosedOnConflictingDuplicate(t *testing.T) {
	rm := &RequestModel{RuntimeTargets: []RuntimeTarget{{
		Kind: RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit",
	}}}
	first := traceWakeupEdgeRoleTestRecord("edge-1", "worker-200", "app-100", "20/ohos_cfs", "52/ohos_rt")
	second := first
	second.RichNotes = append([]string(nil), first.RichNotes...)
	second.RichNotes[1] = TraceNoteKeyWakerPriority + "=41/ohos_rt"
	ledger := ObservationLedger{Records: []ObservationRecord{first, second}}
	if got := BuildTraceWakeupEdgeRoleAuthorities(ledger, rm); len(got) != 0 {
		t.Fatalf("conflicting duplicate must fail closed, got %+v", got)
	}
}

func TestBuildTraceWakeupEdgeRoleAuthoritiesKeepsOnlyExplicitWindowScope(t *testing.T) {
	start, end := 1.0, 1.01
	rm := &RequestModel{
		RuntimeTargets: []RuntimeTarget{{
			Kind: RuntimeTargetKindThread, PID: 100, Thread: "app-100", Source: "user_explicit",
		}},
		RuntimeArtifactScopeProfile: &RuntimeArtifactScopeProfile{
			RequestedScope: RuntimeArtifactScopeExplicitWindow,
			TimeStart:      &start,
			TimeEnd:        &end,
			SourceQuote:    "1.000000..1.010000",
		},
	}
	requested := traceWakeupEdgeRoleTestRecord("requested", "worker-200", "app-100", "20/ohos_cfs", "52/ohos_rt")
	stale := requested
	stale.ID = "trace_query:coarse#wakeup_chain_edge:1"
	stale.ObservedAt = "stale"
	ledger := ObservationLedger{Records: []ObservationRecord{
		requested,
		{
			ID: "trace_query:window#root_cause_primary:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: requested.SourceRef,
			Predicate: "root_cause_primary", Subject: "worker-200", Object: "runnable",
			RichNotes: []string{TraceNoteKeySelectedWindow + "=1.000000..1.010000"},
		},
		stale,
		{
			ID: "trace_query:coarse#root_cause_primary:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: ClaimGroundingHard, SourceRef: requested.SourceRef,
			Predicate: "root_cause_primary", Subject: "worker-200", Object: "runnable",
			RichNotes: []string{TraceNoteKeySelectedWindow + "=0.000000..2.000000"},
		},
	}}
	got := BuildTraceWakeupEdgeRoleAuthorities(ledger, rm)
	if len(got) != 1 || got[0].Scope != "trace_query:window" {
		t.Fatalf("explicit window must exclude stale exploration scope, got %+v", got)
	}
}

func traceWakeupEdgeRoleTestRecord(id, waker, wakee, wakerPriority, wakeePriority string) ObservationRecord {
	return ObservationRecord{
		ID: "trace_query:window#wakeup_chain_edge:1", Origin: AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: "/tmp/customer.systrace"},
		Predicate: "wakeup_chain_edge", Subject: waker, Object: wakee,
		RichNotes: []string{
			TraceNoteKeyWakeupTs + "=1.010000",
			TraceNoteKeyWakerPriority + "=" + wakerPriority,
			TraceNoteKeyWakeePriority + "=" + wakeePriority,
			TraceNoteKeyWakerPrioritySource + "=closed_range_stable",
			TraceNoteKeyWakeePrioritySource + "=",
			TraceNoteKeyWakeePriorityAuthority + "=exact_at_point",
			TraceNoteKeyWakeupWakerCPU + "=2",
			TraceNoteKeyWakeupWakeeTargetCPU + "=1",
			TraceNoteKeyWakeupCPURelation + "=cross_cpu",
		},
		ObservedAt: id,
	}
}
