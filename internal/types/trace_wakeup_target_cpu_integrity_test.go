package types

import "testing"

func traceWakeupTargetCPUIntegrityTestRecord(id, artifact, predicate, subject, object string, start, end float64, notes ...string) ObservationRecord {
	return ObservationRecord{
		ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind: ObservationSourceRuntimeArtifact, Path: artifact, ArtifactID: artifact,
		},
		Span: ObservationSpan{StartTs: start, EndTs: end}, Predicate: predicate,
		Subject: subject, Object: object, RichNotes: notes,
	}
}

func TestBuildTraceWakeupTargetCPUIntegrityIsArtifactAndWindowScoped(t *testing.T) {
	integrityRow := traceWakeupTargetCPUIntegrityTestRecord(
		"trace_query:a#wakeup_target_cpu_integrity", "/tmp/a.systrace", "wakeup_target_cpu_integrity",
		"sched_wakeup.target_cpu", string(TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero), 10, 11,
		TraceNoteKeyWakeupTargetCPUIntegrityStatus+"="+string(TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero),
		TraceNoteKeyWakeupTargetCPUObservedCount+"=1697",
		TraceNoteKeyWakeupTargetCPUZeroCount+"=1697",
		TraceNoteKeyWakeupTargetCPUEmitterCPUCount+"=6",
	)
	affected := traceWakeupTargetCPUIntegrityTestRecord(
		"trace_query:a#wakeup_chain_edge:1", "/tmp/a.systrace", "wakeup_chain_edge",
		"worker-a", "app-a", 10.5, 10.5,
		TraceNoteKeyWakeupTs+"=10.500000", TraceNoteKeyWakeupWakerCPU+"=3",
		TraceNoteKeyWakeupWakeeTargetCPU+"=0", TraceNoteKeyWakeupCPURelation+"=cross_cpu",
	)
	otherWindow := traceWakeupTargetCPUIntegrityTestRecord(
		"trace_query:a2#wakeup_chain_edge:1", "/tmp/a.systrace", "wakeup_chain_edge",
		"worker-later", "app-a", 12, 12,
		TraceNoteKeyWakeupTs+"=12.000000", TraceNoteKeyWakeupWakerCPU+"=2",
		TraceNoteKeyWakeupWakeeTargetCPU+"=1", TraceNoteKeyWakeupCPURelation+"=cross_cpu",
	)
	otherArtifact := traceWakeupTargetCPUIntegrityTestRecord(
		"trace_query:b#wakeup_chain_edge:1", "/tmp/b.systrace", "wakeup_chain_edge",
		"worker-b", "app-b", 10.5, 10.5,
		TraceNoteKeyWakeupTs+"=10.500000", TraceNoteKeyWakeupWakerCPU+"=2",
		TraceNoteKeyWakeupWakeeTargetCPU+"=1", TraceNoteKeyWakeupCPURelation+"=cross_cpu",
	)
	ledger := ObservationLedger{Records: []ObservationRecord{integrityRow, affected, otherWindow, otherArtifact}}
	integrity := BuildTraceWakeupTargetCPUIntegrity(ledger)
	if !integrity.SuspectedDegraded() || integrity.ObservedCount != 1697 || integrity.EmitterCPUCount != 6 {
		t.Fatalf("typed integrity not compiled: %+v", integrity)
	}
	if !integrity.AffectsWakeupRecord(affected) {
		t.Fatal("same-artifact in-window edge must be qualified")
	}
	if integrity.AffectsWakeupRecord(otherWindow) || integrity.AffectsWakeupRecord(otherArtifact) {
		t.Fatal("a window/artifact-scoped advisory must not become session-sticky")
	}

	raw := BuildTraceWakeupCPUTopologyAuthorities(ledger)
	if len(raw) != 3 {
		t.Fatalf("raw observation compiler must preserve every tuple, got %+v", raw)
	}
	decision := BuildTraceWakeupCPUTopologyDecisionAuthorities(ledger, integrity)
	if len(decision) != 2 || decision[0].Waker == "worker-a" || decision[1].Waker == "worker-a" {
		t.Fatalf("decision authority must withhold only the affected tuple: %+v", decision)
	}
}

func TestBuildTraceWakeupTargetCPUIntegrityRejectsMalformedOrNonTypedRows(t *testing.T) {
	row := traceWakeupTargetCPUIntegrityTestRecord(
		"bad", "/tmp/a.systrace", "wakeup_target_cpu_integrity", "sched_wakeup.target_cpu",
		string(TraceWakeupTargetCPUIntegritySuspectedDegradedAllZero), 1, 2,
		TraceNoteKeyWakeupTargetCPUObservedCount+"=100",
		TraceNoteKeyWakeupTargetCPUZeroCount+"=99",
		TraceNoteKeyWakeupTargetCPUEmitterCPUCount+"=4",
	)
	if got := BuildTraceWakeupTargetCPUIntegrity(ObservationLedger{Records: []ObservationRecord{row}}); got.SuspectedDegraded() {
		t.Fatalf("an inconsistent census must stay silent: %+v", got)
	}
	row.RichNotes = []string{
		TraceNoteKeyWakeupTargetCPUObservedCount + "=100",
		TraceNoteKeyWakeupTargetCPUZeroCount + "=100",
		TraceNoteKeyWakeupTargetCPUEmitterCPUCount + "=4",
	}
	row.Producer = "model"
	if got := BuildTraceWakeupTargetCPUIntegrity(ObservationLedger{Records: []ObservationRecord{row}}); got.SuspectedDegraded() {
		t.Fatalf("model-authored rows must never create the advisory: %+v", got)
	}
}
