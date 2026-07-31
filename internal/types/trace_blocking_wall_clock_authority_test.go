package types

import "testing"

func traceBlockingWallClockAuthorityRecord(id, typ string, value, start, end float64, notes ...string) ObservationRecord {
	record := traceValueOccurrenceAuthorityRecord(id, typ, value, start, end)
	record.Role = AnswerAggregateRoleSupportingCoverage
	record.ClaimKey = "critical_blocking:" + typ
	record.Predicate = "critical_blocking"
	record.Object = typ
	record.RichNotes = append([]string{
		"type=" + typ,
		"selected_window=13762.791708..13763.024898",
	}, notes...)
	return record
}

func TestBuildTraceBlockingWallClockAuthoritiesSeparatesBlockingFromTransportLatency(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	blocking := traceBlockingWallClockAuthorityRecord(
		"blocking", "binder_wait", 1.409, 13762.835861, 13762.837270,
		"peer=binder:496_9-10961", "flags=0x10", "blocking_candidate=true",
	)
	duplicate := blocking
	duplicate.ID = "blocking-rank-copy"
	transport := blocking
	transport.ID = "ipc-transport"
	transport.Predicate = "ipc_graph"
	transport.ClaimKey = "ipc_graph:sync_request"
	transport.Span = ObservationSpan{StartTs: 13762.834345, EndTs: 13762.835903}
	transport.Value = "1.558"

	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{
		transport, duplicate, blocking,
	}}, &rm)
	if len(got) != 1 {
		t.Fatalf("expected one blocking-wall-clock authority, got %+v", got)
	}
	authority := got[0]
	if authority.Type != "binder_wait" || authority.ObservedMS < 1.4089 || authority.ObservedMS > 1.4091 ||
		authority.CoverageStatus != "complete" || len(authority.Occurrences) != 1 {
		t.Fatalf("blocking wall clock drifted or included transport latency: %+v", authority)
	}
	occurrence := authority.Occurrences[0]
	if occurrence.StartTs != 13762.835861 || occurrence.EndTs != 13762.837270 ||
		occurrence.Peer != "binder:496_9-10961" || len(occurrence.RecordIDs) != 2 {
		t.Fatalf("blocking occurrence identity/richness drifted: %+v", occurrence)
	}
}

func TestBuildTraceBlockingWallClockAuthoritiesUnionsOverlapsAndMarksTruncation(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	first := traceBlockingWallClockAuthorityRecord(
		"first", "futex", 2.000, 10.000, 10.002,
		TraceNoteKeyCapacityTruncated+"=true",
	)
	second := traceBlockingWallClockAuthorityRecord("second", "futex", 2.000, 10.001, 10.003)

	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{first, second}}, &rm)
	if len(got) != 1 || got[0].CoverageStatus != "lower_bound_capacity_truncated" ||
		got[0].ObservedMS < 2.9999 || got[0].ObservedMS > 3.0001 ||
		len(got[0].Occurrences) != 2 {
		t.Fatalf("overlap union/lower-bound authority drifted: %+v", got)
	}
}

func TestBuildTraceBlockingWallClockAuthoritiesAdmitsExactTargetSelfStateWhenCriticalEnvelopeIsWider(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	critical := traceBlockingWallClockAuthorityRecord(
		"critical", "binder_wait", 1.409, 13762.835811, 13762.837270,
		"peer=binder:496_9-10961", "flags=0x10",
	)
	targetSelf := traceBlockingWallClockAuthorityRecord(
		"target-self", "binder_wait", 1.409, 13762.835861, 13762.837270,
		"peer=binder:496_9-10961", "flags=0x10",
	)
	targetSelf.ClaimKey = "root_cause_target_self_state"
	targetSelf.Predicate = "root_cause_target_self_state"
	targetSelf.RichNotes = append(targetSelf.RichNotes, TraceNoteKeyTier+"="+TraceCausalTierTargetSelfState)

	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{
		critical, targetSelf,
	}}, &rm)
	if len(got) != 1 || got[0].ObservedMS < 1.4089 || got[0].ObservedMS > 1.4091 ||
		len(got[0].Occurrences) != 1 || got[0].Occurrences[0].StartTs != 13762.835861 {
		t.Fatalf("exact target-self occurrence must survive wider critical envelope: %+v", got)
	}
}

func TestBuildTraceBlockingWallClockAuthoritiesRejectsEnvelopeNonTargetAndMissingWindow(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	envelope := traceBlockingWallClockAuthorityRecord("envelope", "blocking_span", 1.000, 1.000, 1.010)
	other := traceBlockingWallClockAuthorityRecord("other", "binder_wait", 1.000, 2.000, 2.001)
	other.Subject = "other-99"
	noWindow := traceBlockingWallClockAuthorityRecord("no-window", "binder_wait", 1.000, 3.000, 3.001)
	noWindow.RichNotes = []string{"type=binder_wait"}

	if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{
		envelope, other, noWindow,
	}}, &rm); len(got) != 0 {
		t.Fatalf("non-owned/mis-scoped blocking rows must be rejected: %+v", got)
	}
}
