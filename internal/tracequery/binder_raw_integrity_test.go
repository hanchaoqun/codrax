package tracequery

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func binderRawFailureForLine(idx *Index, line int) *durationOrderViolation {
	if idx == nil {
		return nil
	}
	for i := range idx.durationOrderFailures {
		failure := &idx.durationOrderFailures[i]
		if failure.Family == durationOrderBinder && failure.Line == line {
			return failure
		}
	}
	return nil
}

func binderRawEdgeTransactionIDs(graph IPCGraphResult) []int {
	out := make([]int, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		out = append(out, edge.TransactionID)
	}
	return out
}

func TestBinderParserRejectedRawKnownLaneCannotBeBridged(t *testing.T) {
	trace := strings.Join([]string{
		`client-10 (10) [001] .... 1.000000: binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=1`,
		`client-10 (10) [bad] .... 1.100000: binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=1`,
		`server-20 (20) [002] .... 1.200000: binder_transaction_received: transaction=7`,
		`client-11 (11) [001] .... 1.300000: binder_transaction: transaction=8 dest_node=1 dest_proc=21 dest_thread=21 reply=0 flags=0x0 code=1`,
		`server-21 (21) [002] .... 1.400000: binder_transaction_received: transaction=8`,
	}, "\n") + "\n"
	idx := buildTraceIndex(t, "binder-known-raw-hole.systrace", trace)
	failure := binderRawFailureForLine(idx, 2)
	if failure == nil || failure.LaneKey == "" || failure.SourcePath != canonicalTraceIndexPath(idx.Path) || !containsString(failure.Fields, "header_cpu") {
		t.Fatalf("known-key parser-rejected Binder row did not mint an exact physical barrier: %+v", failure)
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.5})
	if got := binderRawEdgeTransactionIDs(graph); len(got) != 1 || got[0] != 8 || !containsSubstring(graph.Caveats, "binder_pairing_exact_lane_quarantined=true") {
		t.Fatalf("Binder raw hole was bridged or legal sibling lost: transactions=%v graph=%+v", got, graph)
	}
}

func TestBinderParserRejectedRawUnknownKeyFailsOnlyPhysicalSource(t *testing.T) {
	idx := buildTraceIndex(t, "binder-unknown-raw-hole.systrace", strings.Join([]string{
		`client-10 (10) [bad] .... 1.000000: binder_transaction: malformed`,
		`client-11 (11) [001] .... 1.100000: binder_transaction: transaction=8 dest_node=1 dest_proc=21 dest_thread=21 reply=0 flags=0x0 code=1`,
		`server-21 (21) [002] .... 1.200000: binder_transaction_received: transaction=8`,
	}, "\n")+"\n")
	failure := binderRawFailureForLine(idx, 1)
	if failure == nil || failure.LaneKey != "" || failure.SourcePath != canonicalTraceIndexPath(idx.Path) || !containsString(failure.Fields, "canonical_pairing_identity") {
		t.Fatalf("unknown-key parser-rejected Binder row did not mint a source-family barrier: %+v", failure)
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.3})
	if len(graph.Edges) != 0 || !containsSubstring(graph.Caveats, "binder_pairing_source_fail_closed=true") {
		t.Fatalf("unknown-key Binder raw row did not fail its physical source closed: %+v", graph)
	}
}

func TestBinderRawBarrierKeepsCompositePhysicalCoordinates(t *testing.T) {
	dir := t.TempDir()
	child := filepath.Join(dir, "binder-raw-child.systrace")
	bundle := filepath.Join(dir, "binder-raw.tracebundle.json")
	writeBundleProvenanceFixture(t, child, strings.Join([]string{
		`client-10 (10) [001] .... 1.000000: binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=1`,
		`client-10 (10) [bad] .... 1.100000: binder_transaction: transaction=7 dest_node=1 dest_proc=20 dest_thread=20 reply=0 flags=0x0 code=1`,
		`server-20 (20) [002] .... 1.200000: binder_transaction_received: transaction=7`,
		`client-11 (11) [001] .... 1.300000: binder_transaction: transaction=8 dest_node=1 dest_proc=21 dest_thread=21 reply=0 flags=0x0 code=1`,
		`server-21 (21) [002] .... 1.400000: binder_transaction_received: transaction=8`,
	}, "\n")+"\n")
	writeBundleProvenanceFixture(t, bundle, `{"version":"test","systrace":"binder-raw-child.systrace","artifacts":[{"type":"systrace","path":"binder-raw-child.systrace"}]}`)
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	var failure *durationOrderViolation
	for i := range idx.durationOrderFailures {
		candidate := &idx.durationOrderFailures[i]
		if candidate.Family == durationOrderBinder && candidate.LocalLine == 2 {
			failure = candidate
			break
		}
	}
	if failure == nil || failure.LaneKey == "" || failure.SourcePath != canonicalTraceIndexPath(child) || failure.LocalLine != 2 {
		t.Fatalf("composite Binder raw barrier lost physical source/local-line provenance: %+v all=%+v", failure, idx.durationOrderFailures)
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.5})
	if got := binderRawEdgeTransactionIDs(graph); len(got) != 1 || got[0] != 8 {
		t.Fatalf("composite Binder raw hole crossed or sibling was lost: transactions=%v graph=%+v", got, graph)
	}
}

func TestBinderRawSourceBarrierDoesNotPoisonSiblingArtifact(t *testing.T) {
	const sourceA = "/trace/binder-a.systrace"
	const sourceB = "/trace/binder-b.systrace"
	event := func(line int, ts float64, pid, transaction int, received bool) Event {
		name, typ := "binder_transaction", EventBinderTransaction
		if received {
			name, typ = "binder_transaction_received", EventBinderReceived
		}
		return Event{Line: line, Ts: ts, PID: pid, TGID: pid, Comm: "binder", Name: name, Type: typ,
			FieldText: "transaction=" + strconv.Itoa(transaction), BinderFields: &BinderFields{TransactionID: transaction}}
	}
	idx := &Index{
		Path: "/trace/binder.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 102,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: sourceA, LocalLineCount: 3, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: sourceB, LocalLineCount: 2, VirtualLineBase: 100, CausalCompatible: true},
		},
		Events: []Event{
			event(1, 1.000, 10, 7, false), event(3, 1.200, 20, 7, true),
			event(101, 1.100, 11, 8, false), event(102, 1.300, 21, 8, true),
		},
		durationOrderFailures: []durationOrderViolation{{
			Family: durationOrderBinder, Issue: "endpoint_parse_incomplete", EventName: "binder_transaction",
			Fields: []string{"canonical_pairing_identity"}, Line: 2, SourcePath: sourceA,
		}},
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.4})
	if got := binderRawEdgeTransactionIDs(graph); len(got) != 1 || got[0] != 8 || !containsSubstring(graph.Caveats, "binder_pairing_source_fail_closed=true") {
		t.Fatalf("source-scoped Binder barrier leaked across artifacts or failed to disclose: transactions=%v graph=%+v", got, graph)
	}
}

func TestBinderRawAuditWitnessCapFailsClosed(t *testing.T) {
	idx := &Index{
		Path: "/trace/binder-cap.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 2,
		Events: []Event{
			{Line: 1, Ts: 1.0, PID: 10, TGID: 10, Comm: "client", Name: "binder_transaction", Type: EventBinderTransaction,
				FieldText: "transaction=7", BinderFields: &BinderFields{TransactionID: 7}},
			{Line: 2, Ts: 1.1, PID: 20, TGID: 20, Comm: "server", Name: "binder_transaction_received", Type: EventBinderReceived,
				FieldText: "transaction=7", BinderFields: &BinderFields{TransactionID: 7}},
		},
		durationOrderFailuresCapped: map[durationOrderFamily]bool{durationOrderBinder: true},
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.2})
	if len(graph.Edges) != 0 || !containsSubstring(graph.Caveats, "binder_pairing_fail_closed=true") || !containsSubstring(graph.Caveats, "raw_audit_capped=true") {
		t.Fatalf("capped Binder raw witness ledger failed open: %+v", graph)
	}
}
