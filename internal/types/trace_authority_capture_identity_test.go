package types

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func b1618BlockingRecord(id, path string, start, end float64) ObservationRecord {
	r := traceBlockingWallClockAuthorityRecord(id, "binder_wait", (end-start)*1000, start, end)
	r.SourceRef.Path = path
	r.SourceRef.ArtifactID = "trace_query"
	return r
}

func TestB1618BlockingCaptureIdentityAndOrdering(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	a := b1618BlockingRecord("a", "/capture/A/trace.ftrace", 10, 10.001409)
	b := b1618BlockingRecord("b", "/capture/B/trace.ftrace", 10.002, 10.003409)
	var first []TraceBlockingWallClockAuthority
	for _, records := range [][]ObservationRecord{{a, b}, {b, a}} {
		got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: records}, &rm)
		if len(got) != 2 {
			t.Fatalf("two physical captures must not become one wall-clock account: %+v", got)
		}
		for _, authority := range got {
			if authority.ArtifactKey == "" || len(authority.Occurrences) != 1 || math.Abs(authority.ObservedMS-1.409) > .000001 || authority.CoverageStatus != "complete" {
				t.Fatalf("capture-scoped value/identity drift: %+v", authority)
			}
		}
		if got[0].ArtifactKey == got[1].ArtifactKey || got[0].ArtifactLabel == got[1].ArtifactLabel {
			t.Fatalf("distinct capture identities and their colliding display names must be distinguishable: %+v", got)
		}
		if first != nil && !reflect.DeepEqual(first, got) {
			t.Fatalf("input order changed capture account: first=%+v got=%+v", first, got)
		}
		first = got
	}
}

func TestB1618BlockingIdentityCompatibilityAndUnion(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	a := b1618BlockingRecord("a", "/capture/A/trace.ftrace", 10, 10.002)
	b := b1618BlockingRecord("b", a.SourceRef.Path, 10.001, 10.003)
	duplicate := a
	duplicate.ID = "duplicate"
	got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{a, b, duplicate}}, &rm)
	if len(got) != 1 || math.Abs(got[0].ObservedMS-3) > .000001 || len(got[0].Occurrences) != 2 || len(got[0].Occurrences[0].RecordIDs) != 2 {
		t.Fatalf("same-capture duplicate and overlap union must remain intact: %+v", got)
	}
	for _, tc := range []struct {
		name string
		edit func(*ObservationRecord)
		want int
	}{
		{"posix_case_is_identity", func(r *ObservationRecord) { r.SourceRef.Path = "/capture/a/trace.ftrace" }, 2},
		{"verified_capture_alias", func(r *ObservationRecord) {
			r.SourceRef.Path = "/materialized/attached_trace.txt"
			r.SourceRef.CaptureIdentityPath = a.SourceRef.Path
		}, 1},
		{"distinct_artifact_id_does_not_override_capture", func(r *ObservationRecord) { r.SourceRef.ArtifactID = "runtime_artifact:different_label" }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := b
			tc.edit(&r)
			if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{a, r}}, &rm); len(got) != tc.want {
				t.Fatalf("capture binding mismatch: %+v", got)
			}
		})
	}
	for _, id := range []string{"", "trace_query", "attached_trace"} {
		r := a
		r.SourceRef.Path, r.SourceRef.ArtifactID, r.SupportRefs = "", id, nil
		if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{r}}, &rm); len(got) != 0 {
			t.Errorf("pathless lane token %q must not invent capture identity: %+v", id, got)
		}
	}
	r := a
	r.SourceRef.Path, r.SourceRef.ArtifactID = "", "capture-id-123"
	if got := BuildTraceBlockingWallClockAuthorities(ObservationLedger{Records: []ObservationRecord{r}}, &rm); len(got) != 1 || got[0].ArtifactKey == "" {
		t.Fatalf("specific artifact ID compatibility lost: %+v", got)
	}
}

func b1618IPCCohort(setID, path, result string, transactions []int, peers []string) []ObservationRecord {
	set := traceIPCRequestCensusAuthorityRecord(setID, "ipc_request_census", fmt.Sprint(len(transactions)), 10, 11,
		TraceNoteKeyIPCRequestCensusStatus+"=complete", TraceNoteKeyIPCSyncRequestCount+"="+fmt.Sprint(len(transactions)),
		TraceNoteKeyIPCOnewayRequestCount+"=0", TraceNoteKeyIPCUnknownRequestCount+"=0")
	set.SourceRef.Path, set.SourceRef.PayloadRef, set.SourceRef.RawRef = path, result+".json", result+".txt"
	set.SourceRef.ArtifactID = "trace_query"
	set.RichNotes[0] = TraceNoteKeySelectedWindow + "=10.000000..11.000000"
	rows := []ObservationRecord{set}
	for i, tx := range transactions {
		row := set
		row.ID = fmt.Sprintf("%s-row%d", setID, i)
		row.Predicate, row.ClaimKey, row.Value, row.Object = "ipc_request_edge", "ipc_request_edge", fmt.Sprint(tx), peers[i]
		row.Span = ObservationSpan{StartTs: 10 + float64(i+1)/100, EndTs: 10 + float64(i+1)/100 + .0001, LineStart: 100 + i*10, LineEnd: 102 + i*10}
		row.RichNotes = []string{TraceNoteKeySelectedWindow + "=10.000000..11.000000", TraceNoteKeyIPCTransactionID + "=" + fmt.Sprint(tx), TraceNoteKeyIPCCallSemantics + "=sync_request", TraceNoteKeyIPCFlags + "=0x10", TraceNoteKeyIPCFlagsKnown + "=true", TraceNoteKeyIPCCode + "=0x19", TraceNoteKeyIPCCodeKnown + "=true", TraceNoteKeyIPCReceiverSource + "=matched_receive"}
		rows = append(rows, row)
	}
	return rows
}

func TestB1618IPCCaptureCohortAndTransactionIdentity(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	a := b1618IPCCohort("set-A", "/capture/A/trace.ftrace", "/results/A", []int{7}, []string{"server-51"})
	b := b1618IPCCohort("set-B", "/capture/B/trace.ftrace", "/results/B", []int{7, 9}, []string{"server-52", "server-52"})
	var first []TraceIPCRequestCensusAuthority
	for _, records := range [][]ObservationRecord{append(append([]ObservationRecord{}, a...), b...), append(append([]ObservationRecord{}, b...), a...)} {
		got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm)
		if len(got) != 2 {
			t.Fatalf("two captures must retain two complete IPC accounts, not cross-file native rows: %+v", got)
		}
		for _, authority := range got {
			wantCount, wantPeer := 1, "server-51"
			if authority.SourceRecordID == "set-B" {
				wantCount, wantPeer = 2, "server-52"
			}
			if authority.ArtifactKey == "" || authority.SourceRecordID == "" || authority.TotalRequests != wantCount || authority.SyncRequests != wantCount || len(authority.SyncRoster) != wantCount || authority.CoverageStatus != "complete" {
				t.Fatalf("capture census lost exact source/count: %+v", authority)
			}
			for _, row := range authority.SyncRoster {
				if row.Peer != wantPeer || row.Flags != "0x10" || !row.FlagsKnown || row.Code != "0x19" || !row.CodeKnown {
					t.Fatalf("native row fields borrowed across captures: %+v", authority)
				}
			}
		}
		if got[0].ArtifactKey == got[1].ArtifactKey || got[0].ArtifactLabel == got[1].ArtifactLabel {
			t.Fatalf("capture keys/display labels collide: %+v", got)
		}
		if first != nil && !reflect.DeepEqual(first, got) {
			t.Fatalf("order elected a different census: first=%+v got=%+v", first, got)
		}
		first = got
	}
}

func TestB1618IPCSameCaptureDifferentResultsNeverCompleteEachOther(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	a := b1618IPCCohort("set-A", "/capture/trace.ftrace", "/results/A", []int{7}, []string{"server-51"})
	b := b1618IPCCohort("set-B", a[0].SourceRef.Path, "/results/B", []int{7, 9}, []string{"server-52", "server-52"})
	for _, records := range [][]ObservationRecord{append(append([]ObservationRecord{}, a...), b...), append(append([]ObservationRecord{}, b...), a...)} {
		got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm)
		if len(got) != 2 {
			t.Fatalf("different result cohorts must not overwrite or repair each other: %+v", got)
		}
		for _, authority := range got {
			if authority.SourceRecordID == "set-A" && (authority.TotalRequests != 1 || len(authority.SyncRoster) != 1 || authority.SyncRoster[0].Peer != "server-51") {
				t.Fatalf("A took B's counts/peer: %+v", authority)
			}
			if authority.SourceRecordID == "set-B" && (authority.TotalRequests != 2 || len(authority.SyncRoster) != 2 || authority.SyncRoster[0].Peer != "server-52") {
				t.Fatalf("B took A's native fields: %+v", authority)
			}
		}
	}
	got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: []ObservationRecord{a[0], b[1]}}, &rm)
	if len(got) != 1 || got[0].CoverageStatus == "complete" || len(got[0].SyncRoster) != 0 {
		t.Fatalf("foreign cohort row repaired missing roster: %+v", got)
	}
	missingRefSet, missingRefRow := a[0], a[1]
	missingRefSet.SourceRef.PayloadRef, missingRefSet.SourceRef.RawRef = "", ""
	missingRefRow.SourceRef.PayloadRef, missingRefRow.SourceRef.RawRef = "", ""
	got = BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: []ObservationRecord{missingRefSet, missingRefRow}}, &rm)
	if len(got) != 1 || got[0].CoverageStatus == "complete" {
		t.Fatalf("missing result identity must not claim a complete assembled roster: %+v", got)
	}
}

func TestB1618IPCExactDuplicateQueriesAndReusedTransactionID(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	a := b1618IPCCohort("A", "/capture/trace.ftrace", "/results/A", []int{7, 7}, []string{"server-51", "server-52"})
	b := b1618IPCCohort("B", a[0].SourceRef.Path, "/results/B", []int{7, 7}, []string{"server-51", "server-52"})
	for _, records := range [][]ObservationRecord{append(append([]ObservationRecord{}, a...), b...), append(append([]ObservationRecord{}, b...), a...)} {
		got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm)
		if len(got) != 1 || got[0].SourceRecordID != "A" || got[0].CoverageStatus != "complete" || len(got[0].SyncRoster) != 2 || got[0].SyncRoster[0].Peer != "server-51" || got[0].SyncRoster[1].Peer != "server-52" {
			t.Fatalf("distinct occurrences of a reused transaction ID must survive; repeated result must not double counts: %+v", got)
		}
	}
	conflict := a[1]
	conflict.ID, conflict.Object = "contradictory-row", "different-peer"
	got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: append(append([]ObservationRecord{}, a...), conflict)}, &rm)
	if len(got) != 1 || got[0].CoverageStatus == "complete" {
		t.Fatalf("same occurrence with contradictory native fields must not elect the first one: %+v", got)
	}
}

func TestB1618IPCCohortRequiresExactResultSource(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	for _, tc := range []struct {
		name string
		edit func(*ObservationRecord)
	}{
		{"other_payload", func(r *ObservationRecord) { r.SourceRef.PayloadRef = "/other.json" }},
		{"other_raw", func(r *ObservationRecord) { r.SourceRef.RawRef = "/other.txt" }},
		{"other_kind", func(r *ObservationRecord) { r.SourceRef.Kind = ObservationSourceModelClaim }},
		{"other_query_generation", func(r *ObservationRecord) { r.Producer = "trace_query:run2" }},
		{"other_observed_time", func(r *ObservationRecord) { r.ObservedAt = "2026-09-07T02:00:01Z" }},
		{"same_capture_other_carrier", func(r *ObservationRecord) {
			r.SourceRef.CaptureIdentityPath = r.SourceRef.Path
			r.SourceRef.Path = "/materialized/trace.txt"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := b1618IPCCohort("A", "/capture/trace.ftrace", "/results/A", []int{7}, []string{"peer"})
			records[0].ObservedAt, records[1].ObservedAt = "2026-09-07T02:00:00Z", "2026-09-07T02:00:00Z"
			tc.edit(&records[1])
			before, _ := json.Marshal(records)
			got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm)
			after, _ := json.Marshal(records)
			if len(got) != 1 || got[0].CoverageStatus == "complete" || len(got[0].SyncRoster) != 0 {
				t.Fatalf("foreign result row acquired same-cohort authority: %+v", got)
			}
			if string(before) != string(after) || !reflect.DeepEqual(got, BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm)) {
				t.Fatal("authority builder mutated input or is not idempotent")
			}
		})
	}
	for _, id := range []string{"", "trace_query", "attached_trace"} {
		records := b1618IPCCohort("A", "/capture/trace.ftrace", "/results/A", []int{7}, []string{"peer"})
		for i := range records {
			records[i].SourceRef.Path, records[i].SourceRef.ArtifactID, records[i].SupportRefs = "", id, nil
		}
		if got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm); len(got) != 0 {
			t.Errorf("generic artifact %q must not gain capture identity from a result ref: %+v", id, got)
		}
	}
}

func TestB1618IPCContradictoryCensusWithinOneResultCannotElectCounts(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	a := b1618IPCCohort("A", "/capture/trace.ftrace", "/results/A", []int{7}, []string{"peer"})
	b := b1618IPCCohort("B", a[0].SourceRef.Path, "/results/A", []int{7, 9}, []string{"peer", "peer"})
	for _, records := range [][]ObservationRecord{append(append([]ObservationRecord{}, a...), b...), append(append([]ObservationRecord{}, b...), a...)} {
		if got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: records}, &rm); len(got) != 0 {
			t.Fatalf("contradictory sets for one precise result cannot both claim a census: %+v", got)
		}
	}
}

func TestB1618IPCSameSendDifferentReceiveIsOneAmbiguousRequest(t *testing.T) {
	rm := traceValueOccurrenceAuthorityRequest()
	for _, tc := range []struct {
		name                   string
		changeLine, changeTime bool
	}{
		{"receive_line", true, false}, {"receive_time", false, true}, {"both", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			records := b1618IPCCohort("A", "/capture/trace.ftrace", "/results/A", []int{7, 7}, []string{"peer", "peer"})
			records[2].Span = records[1].Span
			if tc.changeLine {
				records[2].Span.LineEnd++
			}
			if tc.changeTime {
				records[2].Span.EndTs += .0001
			}
			for _, input := range [][]ObservationRecord{records, {records[2], records[1], records[0]}} {
				before, _ := json.Marshal(input)
				got := BuildTraceIPCRequestCensusAuthorities(ObservationLedger{Records: input}, &rm)
				if len(got) != 1 || got[0].TotalRequests != 2 || got[0].CoverageStatus == "complete" || len(got[0].SyncRoster) != 0 {
					t.Fatalf("two receive witnesses for the exact same send cannot become two requests: %+v", got)
				}
				after, _ := json.Marshal(input)
				if string(before) != string(after) {
					t.Fatal("conflict audit must preserve original census and rows")
				}
			}
		})
	}
}
