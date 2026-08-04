package types

import "testing"

func TestBuildTraceTargetWakeupCensusAuthoritiesCarriesCompleteTargetInventoryAndPreWakeSplit(t *testing.T) {
	row := func(ordinal, waker, count string) ObservationRecord {
		return ObservationRecord{
			ID:        "trace_query:fixture#wakeup_edge_census:" + ordinal,
			Producer:  "trace_query",
			Predicate: "wakeup_edge_census",
			Subject:   waker,
			Object:    "app-100",
			Value:     count,
			RichNotes: []string{
				TraceNoteKeyWakeupEdgeCensusTargetWakee + "=true",
				TraceNoteKeyWakeupEdgeCensusSleepExit + "=" + count,
				TraceNoteKeyWakeupEdgeCensusFirstTs + "=1.000000",
				TraceNoteKeyWakeupEdgeCensusLastTs + "=1.100000",
				TraceNoteKeySelectedWindow + "=1.000000..1.100000",
			},
		}
	}
	ledger := ObservationLedger{Records: []ObservationRecord{
		row("1", "CookieMonsterCl-200", "34"),
		row("2", "Binder-300", "1"),
		row("3", "Worker-400", "1"),
		{
			ID:        "trace_query:fixture#wakeup_edge_census:4",
			Producer:  "trace_query",
			Predicate: "wakeup_edge_census",
			Subject:   "off-chain-500",
			Object:    "peer-101",
			Value:     "99",
		},
	}}

	got := BuildTraceTargetWakeupCensusAuthorities(ledger)
	if len(got) != 1 {
		t.Fatalf("expected one target census authority, got %+v", got)
	}
	authority := got[0]
	if !authority.Complete || !authority.SplitAvailable {
		t.Fatalf("target census should be complete with a full split: %+v", authority)
	}
	if authority.TotalCount != 36 || authority.SleepExitCount != 36 ||
		authority.DExitCount != 0 || authority.OtherExitCount != 0 {
		t.Fatalf("wrong target totals: %+v", authority)
	}
	if len(authority.Pairs) != 3 || authority.Pairs[0].Waker != "CookieMonsterCl-200" ||
		authority.Pairs[0].Count != 34 {
		t.Fatalf("pair roster/order drifted: %+v", authority.Pairs)
	}
}

func TestBuildTraceTargetWakeupCensusAuthoritiesCollapsesExactCrossQueryRoster(t *testing.T) {
	row := func(scope, count string) ObservationRecord {
		return ObservationRecord{
			ID: scope + "#wakeup_edge_census:1", Producer: "trace_query",
			Predicate: "wakeup_edge_census", Subject: "worker-200", Object: "app-100", Value: count,
			RichNotes: []string{
				TraceNoteKeyWakeupEdgeCensusTargetWakee + "=true",
				TraceNoteKeyWakeupEdgeCensusSleepExit + "=" + count,
				TraceNoteKeyWakeupEdgeCensusFirstTs + "=1.010000",
				TraceNoteKeyWakeupEdgeCensusLastTs + "=1.010000",
				TraceNoteKeySelectedWindow + "=1.000000..1.010000",
			},
		}
	}
	ledger := ObservationLedger{Records: []ObservationRecord{
		row("trace_query:explore", "1"), row("trace_query:supplement", "1"),
	}}
	got := BuildTraceTargetWakeupCensusAuthorities(ledger)
	if len(got) != 1 {
		t.Fatalf("identical cross-query target census should compile once: %+v", got)
	}

	ledger.Records = append(ledger.Records, row("trace_query:conflict", "2"))
	got = BuildTraceTargetWakeupCensusAuthorities(ledger)
	if len(got) != 2 {
		t.Fatalf("different typed census totals must remain separate: %+v", got)
	}
}

func TestBuildTraceTargetWakeupCensusAuthoritiesFailsClosedOnExitPartitionConflict(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{{
		ID:        "trace_query:fixture#wakeup_edge_census:1",
		Producer:  "trace_query",
		Predicate: "wakeup_edge_census",
		Subject:   "waker-200",
		Object:    "app-100",
		Value:     "3",
		RichNotes: []string{
			TraceNoteKeyWakeupEdgeCensusTargetWakee + "=true",
			TraceNoteKeyWakeupEdgeCensusSleepExit + "=2",
			TraceNoteKeySelectedWindow + "=1.000000..1.100000",
		},
	}}}
	got := BuildTraceTargetWakeupCensusAuthorities(ledger)
	if len(got) != 1 || got[0].Complete {
		t.Fatalf("partition mismatch must fail closed: %+v", got)
	}
}
