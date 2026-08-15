package types

import "testing"

func TestBuildTraceWakeupCPUTopologyAuthoritiesUsesExactTypedRows(t *testing.T) {
	row := func(id, waker, wakee, wakerCPU, wakeeCPU, relation string) ObservationRecord {
		return ObservationRecord{
			ID: id, Origin: AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
			Predicate: "wakeup_chain_edge", Subject: waker, Object: wakee,
			RichNotes: []string{
				TraceNoteKeyWakeupWakerCPU + "=" + wakerCPU,
				TraceNoteKeyWakeupWakeeTargetCPU + "=" + wakeeCPU,
				TraceNoteKeyWakeupCPURelation + "=" + relation,
			},
		}
	}
	cross := row("cross", "worker-200", "app-100", "2", "1", "cross_cpu")
	same := row("same", "producer-300", "consumer-400", "3", "3", "same_cpu")
	unknown := row("unknown", "unknown-500", "app-100", "4", "5", "unknown")
	inconsistent := row("inconsistent", "bad-600", "app-100", "6", "7", "same_cpu")
	soft := row("soft", "soft-700", "app-100", "7", "8", "cross_cpu")
	soft.GroundingPolicy = ClaimGroundingSoft

	got := BuildTraceWakeupCPUTopologyAuthorities(ObservationLedger{Records: []ObservationRecord{
		cross, cross, same, unknown, inconsistent, soft,
	}})
	if len(got) != 2 {
		t.Fatalf("authorities = %+v, want deduped exact same/cross rows only", got)
	}
	if got[0].Waker != "worker-200" || got[0].Wakee != "app-100" ||
		got[0].WakerCPU != 2 || got[0].WakeeTargetCPU != 1 || got[0].Relation != TraceWakeupCPUTopologyCrossCPU {
		t.Fatalf("cross-CPU authority = %+v", got[0])
	}
	if got[1].Waker != "producer-300" || got[1].Wakee != "consumer-400" ||
		got[1].WakerCPU != 3 || got[1].WakeeTargetCPU != 3 || got[1].Relation != TraceWakeupCPUTopologySameCPU {
		t.Fatalf("same-CPU authority = %+v", got[1])
	}
}

func TestBuildTraceWakeupCPUTopologyAuthoritiesCapsStableOrder(t *testing.T) {
	var records []ObservationRecord
	for i := 0; i < TraceWakeupCPUTopologyAuthorityLimit+2; i++ {
		records = append(records, ObservationRecord{
			ID: "edge", Origin: AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
			Predicate: "wakeup_chain_edge", Subject: string(rune('a' + i)), Object: "target",
			RichNotes: []string{
				TraceNoteKeyWakeupWakerCPU + "=2",
				TraceNoteKeyWakeupWakeeTargetCPU + "=1",
				TraceNoteKeyWakeupCPURelation + "=cross_cpu",
			},
		})
	}
	got := BuildTraceWakeupCPUTopologyAuthorities(ObservationLedger{Records: records})
	if len(got) != TraceWakeupCPUTopologyAuthorityLimit || got[0].Waker != "a" || got[len(got)-1].Waker != "c" {
		t.Fatalf("stable capped authorities = %+v", got)
	}
}
