package types

import "testing"

func traceIPCRequestCensusAuthorityRecord(id, predicate, value string, start, end float64, notes ...string) ObservationRecord {
	return ObservationRecord{
		ID:              id,
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef: ObservationSourceRef{
			Kind:       ObservationSourceRuntimeArtifact,
			ArtifactID: "attached_trace",
		},
		Span:      ObservationSpan{StartTs: start, EndTs: end},
		ClaimKey:  predicate,
		Predicate: predicate,
		Subject:   ".ugc.aweme.lite-17267",
		Value:     value,
		RichNotes: append([]string{
			TraceNoteKeySelectedWindow + "=13762.791708..13763.024898",
		}, notes...),
	}
}

func TestBuildTraceIPCRequestCensusAuthoritiesSeparatesRequestCountAndPreservesNativeFields(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	set := traceIPCRequestCensusAuthorityRecord(
		"set", "ipc_request_census", "15", 13762.791708, 13763.024898,
		TraceNoteKeyIPCRequestCensusStatus+"=complete",
		TraceNoteKeyIPCSyncRequestCount+"=5",
		TraceNoteKeyIPCOnewayRequestCount+"=10",
		TraceNoteKeyIPCUnknownRequestCount+"=0",
	)
	row := traceIPCRequestCensusAuthorityRecord(
		"row", "ipc_request_edge", "12145859", 13762.835811, 13762.835943,
		TraceNoteKeyIPCTransactionID+"=12145859",
		TraceNoteKeyIPCCallSemantics+"=sync_request",
		TraceNoteKeyIPCFlags+"=0x10",
		TraceNoteKeyIPCFlagsKnown+"=true",
		TraceNoteKeyIPCCode+"=0x19",
		TraceNoteKeyIPCCodeKnown+"=true",
		TraceNoteKeyIPCReceiverSource+"=matched_receive",
	)
	row.Object = "binder:496_9-10961"
	rows := []ObservationRecord{set}
	for i := 0; i < 5; i++ {
		copy := row
		copy.ID = "row-" + string(rune('a'+i))
		copy.Value = string(rune('1' + i))
		copy.RichNotes = append([]string(nil), row.RichNotes...)
		copy.RichNotes[1] = TraceNoteKeyIPCTransactionID + "=" + string(rune('1'+i))
		copy.Span.StartTs += float64(i)
		copy.Span.EndTs += float64(i)
		rows = append(rows, copy)
	}
	// Keep the customer transaction as one exact row in the roster.
	rows[1] = row

	got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: rows}, &rm)
	if len(got) != 1 {
		t.Fatalf("expected one IPC request authority: %+v", got)
	}
	authority := got[0]
	if authority.TotalRequests != 15 || authority.SyncRequests != 5 ||
		authority.OnewayRequests != 10 || authority.UnknownRequests != 0 ||
		authority.CoverageStatus != "complete" || len(authority.SyncRoster) != 5 {
		t.Fatalf("IPC request census drifted: %+v", authority)
	}
	first := authority.SyncRoster[0]
	if first.TransactionID != 12145859 || first.Code != "0x19" || !first.CodeKnown ||
		first.Flags != "0x10" || !first.FlagsKnown || first.Peer != "binder:496_9-10961" {
		t.Fatalf("native IPC fields crossed rows: %+v", first)
	}
}

func TestBuildTraceIPCRequestCensusAuthoritiesRejectsBrokenPartitionAndDowngradesMissingRoster(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	broken := traceIPCRequestCensusAuthorityRecord(
		"broken", "ipc_request_census", "6", 1, 2,
		TraceNoteKeyIPCRequestCensusStatus+"=complete",
		TraceNoteKeyIPCSyncRequestCount+"=2",
		TraceNoteKeyIPCOnewayRequestCount+"=3",
		TraceNoteKeyIPCUnknownRequestCount+"=0",
	)
	if got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: []ObservationRecord{broken}}, &rm); len(got) != 0 {
		t.Fatalf("broken request partition must fail closed: %+v", got)
	}
	incomplete := broken
	incomplete.Value = "5"
	got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: []ObservationRecord{incomplete}}, &rm)
	if len(got) != 1 || got[0].CoverageStatus != "counts_complete_sync_roster_incomplete" {
		t.Fatalf("missing sync roster must downgrade field coverage: %+v", got)
	}
}
