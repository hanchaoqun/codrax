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

func TestBuildTraceBlockingWallClockAuthoritiesAdmitsCompletionClosedSStateIOWait(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	record := traceValueOccurrenceAuthorityRecord("io-pair", "block_rq", 1.347, 13762.872568, 13762.873915)
	record.Predicate = "io_latency"
	record.ClaimKey = "io_latency:block_rq:12,80:480914568:128"
	record.RichNotes = []string{
		"selected_window=13762.791708..13763.024898",
		"complete_thread=udk-irq-12-92",
		"completion_woke_issuer=true",
		"issuer_blocked_state=s_sleep",
		"issuer_blocked_start=13762.872578",
		"issuer_blocked_end=13762.873915",
		"issuer_blocked=1.337",
		"causal_wait_caliber=completion_closed_issuer_blocked",
		"issuer_blocked_clock_scope=target_blocking_elapsed_wall_clock",
		"capacity_truncated=true",
	}

	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{record}}, &rm)
	if len(got) != 1 {
		t.Fatalf("completion-closed IO pair must publish target blocking wall clock: %+v", got)
	}
	authority := got[0]
	if authority.Type != "block_io_completion_closed_issuer_wait" ||
		authority.ObservedMS < 1.3369 || authority.ObservedMS > 1.3371 ||
		authority.CoverageStatus != "lower_bound_capacity_truncated" || len(authority.Occurrences) != 1 {
		t.Fatalf("completion-closed IO authority caliber drifted: %+v", authority)
	}
	occurrence := authority.Occurrences[0]
	if occurrence.Peer != "udk-irq-12-92" || occurrence.StartTs != 13762.872578 ||
		occurrence.EndTs != 13762.873915 || occurrence.Flags != "state=s_sleep;caliber=completion_closed_issuer_blocked" {
		t.Fatalf("completion-closed IO occurrence lost endpoints/state: %+v", occurrence)
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

func TestTracePositiveValueAuthoritiesRejectMissingEvidenceBoundary(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	missing := traceBlockingWallClockAuthorityRecord(
		"missing", "missing_wakeup", 3.584, 10.000, 10.003584,
		TraceNoteKeyCapacityTruncated+"=true",
	)
	missing.ClaimKey = "root_cause_target_self_state"
	missing.Predicate = "root_cause_target_self_state"
	missing.Object = "missing_wakeup"
	missing.RichNotes = append(missing.RichNotes, TraceNoteKeyTier+"="+TraceCausalTierTargetSelfState)

	if !TraceObservationIsEvidenceBoundary(missing) {
		t.Fatalf("typed missing_wakeup row must be classified as an evidence boundary: %+v", missing)
	}
	if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{missing}}, &rm); len(got) != 0 {
		t.Fatalf("absence evidence must not mint proven target blocking wall clock: %+v", got)
	}
	if got := BuildTraceValueOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{missing}}, &rm); len(got) != 0 {
		t.Fatalf("absence evidence must not mint a positive value-owner authority: %+v", got)
	}

	positive := missing
	positive.ID = "positive"
	positive.Object = "binder_wait"
	positive.RichNotes = []string{
		"type=binder_wait",
		"selected_window=13762.791708..13763.024898",
		TraceNoteKeyTier + "=" + TraceCausalTierTargetSelfState,
	}
	if TraceObservationIsEvidenceBoundary(positive) {
		t.Fatalf("positive typed wait was misclassified as an evidence boundary: %+v", positive)
	}
	if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{positive}}, &rm); len(got) != 1 {
		t.Fatalf("positive target-owned wait must keep blocking authority: %+v", got)
	}
	if got := BuildTraceValueOccurrenceAuthorities(ObservationLedger{Records: []ObservationRecord{positive}}, &rm); len(got) != 1 {
		t.Fatalf("positive target-owned wait must keep value-owner authority: %+v", got)
	}
}
