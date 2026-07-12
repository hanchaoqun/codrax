package tracequery

import (
	"strconv"
	"testing"
)

func binderPairingSend(line int, ts float64, pid, transaction int) Event {
	return Event{
		Line: line, Ts: ts, Type: EventBinderTransaction, Name: "binder_transaction", PID: pid, Comm: "sender",
		FieldText:    "transaction=" + strconv.Itoa(transaction) + " dest_proc=20 dest_thread=21 reply=0 flags=0x0 code=0x1",
		BinderFields: &BinderFields{TransactionID: transaction, DestProc: 20, DestThread: 21},
	}
}

func binderPairingReceive(line int, ts float64, pid, transaction int) Event {
	return Event{
		Line: line, Ts: ts, Type: EventBinderReceived, Name: "binder_transaction_received", PID: pid, Comm: "receiver",
		FieldText: "transaction=" + strconv.Itoa(transaction), BinderFields: &BinderFields{TransactionID: transaction},
	}
}

func binderPairingIndex(events ...Event) *Index {
	return &Index{Path: "/trace/binder.systrace", TimestampOrder: TraceTimestampOrderMonotonic, Events: events}
}

func TestBinderPairingSuppressesOverlappingTransactionCohort(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		binderPairingSend(2, 1.001, 11, 7),
		binderPairingReceive(3, 1.002, 20, 7),
		binderPairingReceive(4, 1.003, 21, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "binder_pairing_ambiguous=true") {
		t.Fatalf("overlapping binder cohort was FIFO/reuse guessed: %+v", got)
	}
}

func TestBinderPairingSequentialIdentityReuseRecovers(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		binderPairingReceive(2, 1.001, 20, 7),
		binderPairingSend(3, 1.002, 10, 7),
		binderPairingReceive(4, 1.004, 20, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 2 || got.Edges[0].ReceiveLine != 2 || got.Edges[1].ReceiveLine != 4 {
		t.Fatalf("sequential binder identity did not recover deterministically: %+v", got)
	}
}

func TestBinderPairingSequentialReuseRollbackPoisonsExactLane(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 2.000, 10, 7),
		binderPairingReceive(2, 3.000, 20, 7),
		binderPairingSend(3, 1.000, 10, 7),
		binderPairingReceive(4, 1.500, 20, 7),
		binderPairingSend(5, 4.000, 11, 8),
		binderPairingReceive(6, 5.000, 21, 8),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 5.1})
	if len(got.Edges) != 1 || got.Edges[0].TransactionID != 8 || got.Edges[0].ReceiveLine != 6 || !containsSubstring(got.Caveats, "timestamp_rollbacks=1") {
		t.Fatalf("sequential Binder rollback was rescued or leaked to sibling: %+v", got)
	}
}

func TestBinderPairingWireAliasesPopulateCanonicalEdgeIdentity(t *testing.T) {
	t.Parallel()
	idx := buildTraceIndex(t, "binder-alias.systrace", "sender-10 (10) [001] .... 1.000000: binder_transaction: debug_id=42 dest_node=0 dest_proc=20 dest_thread=21 reply=0 flags=0x0 code=0x1\nreceiver-20 (20) [001] .... 1.001000: binder_transaction_received: transaction_id=42\n")
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || got.Edges[0].TransactionID != 42 || got.Edges[0].ReceiveLine == 0 {
		t.Fatalf("Binder alias decoder and typed edge identity diverged: %+v", got)
	}
}

func TestBinderRetainedTypedIDUsesStrictPairingAliases(t *testing.T) {
	t.Parallel()
	valid, ok := ParseLine(1, `sender-10 (10) [001] .... 1.000000: binder_transaction: note="x transaction=7 y" transaction=8 dest_proc=20 dest_thread=21 reply=0 flags=0x0 code=0x1`, newStringInterner())
	if !ok || valid.BinderFields == nil || valid.BinderFields.TransactionID != 8 {
		t.Fatalf("quoted metadata stole the retained Binder transaction identity: %+v ok=%t", valid, ok)
	}
	duplicate, ok := ParseLine(2, `sender-10 (10) [001] .... 1.001000: binder_transaction: transaction=8 transaction_id=9 dest_proc=20 dest_thread=21 reply=0 flags=0x0 code=0x1`, newStringInterner())
	if !ok || duplicate.BinderFields == nil || duplicate.BinderFields.TransactionID != 0 {
		t.Fatalf("ambiguous Binder aliases survived in retained typed fields: %+v ok=%t", duplicate, ok)
	}
}

func TestBinderPairingSameTimestampHonorsPhysicalOrder(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingReceive(1, 1.000, 20, 7),
		binderPairingSend(2, 1.000, 10, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || got.Edges[0].ReceiveLine != 0 || got.Edges[0].LatencyMs != 0 {
		t.Fatalf("receive preceding send at the same timestamp was paired: %+v", got)
	}
}

func TestBinderPairingNeverCrossesPhysicalArtifacts(t *testing.T) {
	t.Parallel()
	idx := &Index{
		Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/a.systrace", LocalLineCount: 1, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/b.systrace", LocalLineCount: 1, VirtualLineBase: 1, CausalCompatible: true},
		},
		Events: []Event{binderPairingSend(1, 1.000, 10, 7), binderPairingReceive(2, 1.001, 20, 7)},
	}
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || got.Edges[0].ReceiveLine != 0 || got.Edges[0].LatencyMs != 0 {
		t.Fatalf("binder endpoints crossed physical artifacts: %+v", got)
	}
}

func TestBinderPairingIdleEndpointQuarantinesExactLane(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 0, 7),
		binderPairingSend(2, 1.001, 10, 7),
		binderPairingReceive(3, 1.002, 20, 7),
		binderPairingSend(4, 1.003, 10, 8),
		binderPairingReceive(5, 1.004, 20, 8),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || got.Edges[0].TransactionID != 8 || !containsSubstring(got.Caveats, "binder_pairing_exact_lane_quarantined=true") {
		t.Fatalf("idle binder endpoint did not quarantine only its exact lane: %+v", got)
	}
}

func TestBinderPairingUnkeyableEndpointFailsFamilyClosed(t *testing.T) {
	t.Parallel()
	bad := binderPairingSend(1, 1.000, 10, 0)
	idx := binderPairingIndex(
		bad,
		binderPairingSend(2, 1.001, 10, 7),
		binderPairingReceive(3, 1.002, 20, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "binder_pairing_source_fail_closed=true") {
		t.Fatalf("unkeyable binder endpoint was deleted then rescued: %+v", got)
	}
}

func TestBinderPairingQueryScopeDoesNotImportOrPoisonOutsideEndpoints(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		binderPairingReceive(2, 2.000, 20, 7),
		binderPairingSend(3, 1.010, 10, 8),
		binderPairingReceive(4, 1.020, 20, 8),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 2 {
		t.Fatalf("out-of-window endpoint changed scoped edge count: %+v", got)
	}
	for _, edge := range got.Edges {
		if edge.TransactionID == 7 && (edge.ReceiveLine != 0 || edge.LatencyMs != 0) {
			t.Fatalf("window-external receive was imported: %+v", edge)
		}
		if edge.TransactionID == 8 && edge.ReceiveLine != 4 {
			t.Fatalf("in-window clean pair lost: %+v", edge)
		}
	}
	if containsSubstring(got.Caveats, "binder_pairing_fail_closed=true") || containsSubstring(got.Caveats, "binder_pairing_ambiguous=true") {
		t.Fatalf("legal outside endpoints changed scoped publication: %+v", got.Caveats)
	}
}

func TestBinderPairingTimeWindowReplaysHeadCarryInAndTailCarryThrough(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		binderPairingSend(2, 2.100, 10, 7),
		binderPairingReceive(3, 2.200, 20, 7),
		binderPairingReceive(4, 3.100, 20, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: 2, TimeEnd: 3, TimeStartSet: true, TimeEndSet: true})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "binder_pairing_ambiguous=true") {
		t.Fatalf("window-head open Binder send was cropped then rescued as a pair: %+v", got)
	}
}

func TestBinderPairingLineWindowReplaysHeadCarryInAndTailCarryThrough(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		binderPairingSend(2, 1.100, 10, 7),
		binderPairingReceive(3, 1.200, 20, 7),
		binderPairingReceive(4, 1.300, 20, 7),
	)
	got := BuildIPCGraph(idx, Query{LineStart: 2, LineEnd: 3})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "binder_pairing_ambiguous=true") {
		t.Fatalf("line-window carry-in Binder send was cropped then rescued as a pair: %+v", got)
	}
}

func TestBinderPairingRegressedOutsideOverlapCannotRescueExactLane(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		binderPairingSend(2, 10.000, 10, 7),
		binderPairingReceive(3, 2.000, 20, 7),
		binderPairingReceive(4, 11.000, 20, 7),
		binderPairingSend(5, 2.100, 11, 8),
		binderPairingReceive(6, 2.200, 21, 8),
	)
	idx.TimestampOrder = TraceTimestampOrderRegressed
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 2.5, TimeStartSet: true, TimeEndSet: true})
	if len(got.Edges) != 1 || got.Edges[0].TransactionID != 8 || got.Edges[0].ReceiveLine != 6 ||
		!containsSubstring(got.Caveats, "timestamp_rollbacks=1") {
		t.Fatalf("window-external regressed overlap was cropped or leaked to a clean lane: %+v", got)
	}
}

func TestBinderPairingWindowExternalMalformedEndpointCannotBeDeletedThenRescued(t *testing.T) {
	t.Parallel()
	bad := binderPairingSend(2, 10.000, 10, 0)
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		bad,
		binderPairingReceive(3, 2.000, 20, 7),
	)
	idx.TimestampOrder = TraceTimestampOrderRegressed
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 2.5, TimeStartSet: true, TimeEndSet: true})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "binder_pairing_source_fail_closed=true") {
		t.Fatalf("window-external malformed Binder endpoint was deleted then bridged: %+v", got)
	}
}

func TestBinderPairingFullReplayKeepsLegalSequentialReuse(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7), binderPairingReceive(2, 1.500, 20, 7),
		binderPairingSend(3, 2.000, 10, 7), binderPairingReceive(4, 2.500, 20, 7),
		binderPairingSend(5, 3.100, 10, 7), binderPairingReceive(6, 3.200, 20, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: 2, TimeEnd: 3, TimeStartSet: true, TimeEndSet: true})
	if len(got.Edges) != 1 || got.Edges[0].SendLine != 3 || got.Edges[0].ReceiveLine != 4 ||
		containsSubstring(got.Caveats, "binder_pairing_ambiguous=true") || containsSubstring(got.Caveats, "binder_pairing_exact_lane_quarantined=true") {
		t.Fatalf("legal sequential transaction reuse was over-suppressed by full replay: %+v", got)
	}
}

func TestBinderPairingLineBoundsOverrideConflictingTimeBounds(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(binderPairingSend(1, 1, 10, 7), binderPairingReceive(2, 2, 20, 7))
	got := BuildIPCGraph(idx, Query{LineStart: 1, LineEnd: 2, TimeStart: 100, TimeEnd: 200, TimeStartSet: true, TimeEndSet: true})
	if len(got.Edges) != 1 || got.Edges[0].ReceiveLine != 2 || !near(got.Edges[0].LatencyMs, 1000, .001) {
		t.Fatalf("conflicting time bounds overrode authoritative Binder line bounds: %+v", got)
	}
}

func TestBinderPairingExplicitZeroTimeEndSelectsNoPositiveEndpoint(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(binderPairingSend(1, 1, 10, 7), binderPairingReceive(2, 2, 20, 7))
	got := BuildIPCGraph(idx, Query{TimeEnd: 0, TimeEndSet: true})
	if len(got.Edges) != 0 {
		t.Fatalf("explicit zero time end admitted positive Binder endpoints: %+v", got)
	}
}

func TestBinderPairingSourceGlobalPoisonIsLocal(t *testing.T) {
	t.Parallel()
	bad := binderPairingSend(1, 1.000, 10, 0)
	idx := &Index{
		Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/a.systrace", LocalLineCount: 1, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/b.systrace", LocalLineCount: 2, VirtualLineBase: 100, CausalCompatible: true},
		},
		Events: []Event{bad, binderPairingSend(101, 1.001, 11, 7), binderPairingReceive(102, 1.002, 20, 7)},
	}
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || got.Edges[0].ReceiveLine != 102 || !containsSubstring(got.Caveats, "binder_pairing_source_fail_closed=true") {
		t.Fatalf("source-local poison leaked across artifacts: %+v", got)
	}
}

func TestBinderPairingPhysicalOrderRejectsTimestampRollback(t *testing.T) {
	t.Parallel()
	// Composite indexes are timestamp-sorted. Virtual line order retains the
	// source-local physical order: send(line1,ts2) preceded receive(line2,ts1).
	idx := binderPairingIndex(
		binderPairingReceive(2, 1.000, 20, 7),
		binderPairingSend(1, 2.000, 10, 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 2.1})
	if len(got.Edges) != 0 || !containsSubstring(got.Caveats, "timestamp_rollbacks=1") {
		t.Fatalf("physical-order timestamp rollback minted Binder latency: %+v", got)
	}
}

func TestBinderPairingBudgetBoundsTotalSequentialEndpoints(t *testing.T) {
	t.Parallel()
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 1), binderPairingReceive(2, 1.001, 20, 1),
		binderPairingSend(3, 1.002, 10, 2), binderPairingReceive(4, 1.003, 20, 2),
	)
	audit := auditBinderPairingWithBudget(idx, Query{TimeStart: .9, TimeEnd: 1.1}, 3)
	if !audit.familyGlobal || !audit.budgetExceeded || audit.endpointCount != 4 {
		t.Fatalf("sequential endpoints bypassed total budget: %+v", audit)
	}
}

func TestBinderPairingCaseDriftIsInventory(t *testing.T) {
	t.Parallel()
	upper := binderPairingReceive(2, 1.001, 20, 7)
	upper.Name = "BINDER_TRANSACTION_RECEIVED"
	idx := binderPairingIndex(binderPairingSend(1, 1.000, 10, 7), upper)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || got.Edges[0].ReceiveLine != 0 || got.Edges[0].LatencyMs != 0 {
		t.Fatalf("case-drift Binder row closed exact endpoint: %+v", got)
	}
}

func TestBinderAuxMetadataStaysInsideSequentialCohort(t *testing.T) {
	t.Parallel()
	aux := func(line int, ts float64, typ EventType, name string, transaction int) Event {
		return Event{
			Line: line, Ts: ts, Type: typ, Name: name, PID: 10, Comm: "sender",
			FieldText:    "transaction=" + strconv.Itoa(transaction),
			BinderFields: &BinderFields{TransactionID: transaction, DataSize: int64(line)},
		}
	}
	idx := binderPairingIndex(
		binderPairingSend(1, 1.000, 10, 7),
		aux(2, 1.001, EventBinderAllocBuf, "binder_transaction_alloc_buf", 7),
		binderPairingReceive(3, 1.002, 20, 7),
		binderPairingSend(4, 1.003, 10, 7),
		aux(5, 1.004, EventBinderReply, "binder_reply", 7),
		binderPairingReceive(6, 1.005, 20, 7),
		aux(7, 1.0045, EventBinderAllocBuf, "binder_transaction_alloc_buf", 7),
	)
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 2 || !containsSubstring(got.Edges[0].Caveats, "alloc buffer row at line 2") || containsSubstring(got.Edges[0].Caveats, "reply row at line 5") ||
		!containsSubstring(got.Edges[1].Caveats, "reply row at line 5") || containsSubstring(got.Edges[1].Caveats, "alloc buffer row at line 2") || containsSubstring(got.Edges[1].Caveats, "line 7") {
		t.Fatalf("Binder aux metadata crossed sequential cohorts: %+v", got.Edges)
	}
}

func TestBinderAuxConflictingAliasesRemainInventoryOnly(t *testing.T) {
	t.Parallel()
	conflict := Event{
		Line: 2, Ts: 1.001, Type: EventBinderAllocBuf, Name: "binder_transaction_alloc_buf", PID: 10, Comm: "sender",
		FieldText: "transaction=7 debug_id=8 data_size=64 offsets_size=0", BinderFields: &BinderFields{TransactionID: 7, DataSize: 64},
	}
	idx := binderPairingIndex(binderPairingSend(1, 1, 10, 7), conflict, binderPairingReceive(3, 1.002, 20, 7))
	got := BuildIPCGraph(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(got.Edges) != 1 || containsSubstring(got.Edges[0].Caveats, "alloc buffer") || len(got.BinderEvents) == 0 {
		t.Fatalf("conflicting Binder aux aliases polluted edge or vanished from inventory: %+v", got)
	}
}

func TestBinderTransactInterfaceJoinIsPhysicalSourceScoped(t *testing.T) {
	t.Parallel()
	artifacts := []TraceArtifactSource{
		{SourcePath: "/trace/a.systrace", LocalLineCount: 10, VirtualLineBase: 0, CausalCompatible: true},
		{SourcePath: "/trace/b.systrace", LocalLineCount: 10, VirtualLineBase: 100, CausalCompatible: true},
	}
	span := func(line int, ts float64, name string) Event {
		return Event{Line: line, Ts: ts, Type: EventTraceMark, PID: 10, SpanPID: 10, SpanAction: "B", SpanName: "transact[" + name + "]", FieldText: "B|10|transact[" + name + "]"}
	}
	cross := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		span(1, 1, "source.A:1"), binderPairingSend(101, 1.001, 10, 7), binderPairingReceive(102, 1.002, 20, 7),
	}}
	crossGraph := BuildIPCGraph(cross, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(crossGraph.Edges) != 1 || crossGraph.Edges[0].Interface != "" {
		t.Fatalf("source-A transact span joined source-B Binder send: %+v", crossGraph.Edges)
	}
	same := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		span(101, 1, "source.B:2"), binderPairingSend(102, 1.001, 10, 8), binderPairingReceive(103, 1.002, 20, 8),
	}}
	sameGraph := BuildIPCGraph(same, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(sameGraph.Edges) != 1 || sameGraph.Edges[0].Interface != "source.B:2" {
		t.Fatalf("same-source transact interface join was lost: %+v", sameGraph.Edges)
	}
	reset := Event{Line: 2, Ts: 1.0005, Type: EventSchedSwitch, PrevPID: 10, PrevState: "X"}
	resetScoped := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		span(101, 1, "source.B:3"), reset, binderPairingSend(102, 1.001, 10, 9), binderPairingReceive(103, 1.002, 20, 9),
	}}
	resetGraph := BuildIPCGraph(resetScoped, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(resetGraph.Edges) != 1 || resetGraph.Edges[0].Interface != "source.B:3" {
		t.Fatalf("source-A lifecycle reset cleared source-B transact stack: %+v", resetGraph.Edges)
	}
	unresolvedReset := Event{Line: 99, Ts: 1.0005, Type: EventSchedSwitch, PrevPID: 10, PrevState: "X"}
	unresolved := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		span(101, 1, "source.B:4"), unresolvedReset, binderPairingSend(102, 1.001, 10, 10), binderPairingReceive(103, 1.002, 20, 10),
	}}
	unresolvedGraph := BuildIPCGraph(unresolved, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(unresolvedGraph.Edges) != 1 || unresolvedGraph.Edges[0].Interface != "" || !containsSubstring(unresolvedGraph.Caveats, "trace_mark_interface_join_provenance_unresolved=true") {
		t.Fatalf("unresolved lifecycle reset reused old interface or erased Binder edge: %+v", unresolvedGraph)
	}
	physical := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		binderPairingSend(102, 1, 10, 11), binderPairingReceive(103, 1.1, 20, 11), span(101, 2, "physical.order:5"),
	}}
	physicalGraph := BuildIPCGraph(physical, Query{TimeStart: .9, TimeEnd: 2.1})
	if len(physicalGraph.Edges) != 1 || physicalGraph.Edges[0].Interface != "" || !containsSubstring(physicalGraph.Caveats, "trace_mark_interface_join_timestamp_regressed=true") {
		t.Fatalf("physical source-thread rollback did not fail-close only the interface join: %+v", physicalGraph)
	}
	orderedAcrossSources := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		span(101, 1, "physical.order:6"),
		{Line: 1, Ts: 1.0005, Type: EventTraceMark, PID: 10, SpanPID: 10, SpanAction: "B", SpanName: "other.source:1", FieldText: "B|10|other.source:1"},
		binderPairingSend(102, 1.001, 10, 12),
		{Line: 2, Ts: 1.0015, Type: EventTraceMark, PID: 10, SpanPID: 10, SpanAction: "E", FieldText: "E|10"},
		binderPairingReceive(103, 1.002, 20, 12),
	}}
	orderedGraph := BuildIPCGraph(orderedAcrossSources, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(orderedGraph.Edges) != 1 || orderedGraph.Edges[0].Interface != "physical.order:6" || containsSubstring(orderedGraph.Caveats, "trace_mark_interface_join_timestamp_regressed=true") {
		t.Fatalf("healthy per-source physical replay lost its interface across canonical interleaving: %+v", orderedGraph)
	}
	benignUnresolvedCounter := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, TraceArtifacts: artifacts, Events: []Event{
		span(101, 1, "source.B:counter-safe"),
		{Line: 99, Ts: 1.0005, Type: EventTraceMark, PID: 10, SpanPID: 10, SpanAction: "C", SpanName: "queue_depth", SpanValue: "1", FieldText: "C|10|queue_depth|1"},
		binderPairingSend(102, 1.001, 10, 13), binderPairingReceive(103, 1.002, 20, 13),
	}}
	benignGraph := BuildIPCGraph(benignUnresolvedCounter, Query{TimeStart: .9, TimeEnd: 1.1})
	if len(benignGraph.Edges) != 1 || benignGraph.Edges[0].Interface != "source.B:counter-safe" || containsSubstring(benignGraph.Caveats, "trace_mark_interface_join_provenance_unresolved=true") {
		t.Fatalf("unrelated unresolved counter fail-closed a healthy interface join: %+v", benignGraph)
	}
}
