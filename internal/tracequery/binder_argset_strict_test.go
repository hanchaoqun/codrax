package tracequery

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const binderStrictBaseFields = "transaction=42 dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3"

func binderStrictFieldsWith(field, replacement string) string {
	parts := make([]string, 0, 8)
	for _, token := range strings.Fields(binderStrictBaseFields) {
		if strings.HasPrefix(token, field+"=") {
			continue
		}
		parts = append(parts, token)
	}
	if replacement != "" {
		parts = append(parts, replacement)
	}
	return strings.Join(parts, " ")
}

func TestBinderArgsetStrictSingleAuthorityStructure(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(raw)
	}
	parseSource := read("parse.go")
	for _, required := range []string{
		`case "binder_transaction", "binder_transaction_received":`,
		`s.kv, s.binderTyped = parseBinderTransactionTypedFields(rawType, fields)`,
		`ev.BinderFields = s.binderTyped.binderFields(intern)`,
	} {
		if !strings.Contains(parseSource, required) {
			t.Fatalf("Binder endpoint lineScan authority drifted; missing %q", required)
		}
	}
	for _, retired := range []string{
		`DestProc:      atoi(kv["dest_proc"])`,
		`DestThread:    atoi(kv["dest_thread"])`,
		`Reply:         atoi(kv["reply"])`,
	} {
		if strings.Contains(parseSource, retired) {
			t.Fatalf("Binder hard field re-entered generic parseKV/LWW authority: %q", retired)
		}
	}

	ipcSource := read("ipc.go")
	if strings.Count(ipcSource, "CallSemantics:") != 1 || !strings.Contains(ipcSource, "semantics := BinderCallSemanticsUnknown") {
		t.Fatalf("Binder call semantics must retain one production mint in ipcEdgeFromSend")
	}
	querySource := read("query.go")
	if !strings.Contains(querySource, "edge.CallSemantics != BinderCallSemanticsSyncRequest || !edge.BlockingCandidate") {
		t.Fatalf("Binder wait mint must positively require proven sync_request semantics")
	}
	attributionSource := read("binder_attribution.go")
	if !strings.Contains(attributionSource, "binderReplyDestination{source: source, tid: destPID}") {
		t.Fatalf("Binder reply completion lookup lost its physical-source key")
	}
}

func binderStrictTypedFieldKnown(typed binderTransactionTypedFields, field string) bool {
	switch field {
	case "dest_proc":
		return typed.DestProcKnown
	case "dest_thread":
		return typed.DestThreadKnown
	case "reply":
		return typed.ReplyKnown
	case "flags":
		return typed.FlagsKnown
	case "code":
		return typed.CodeKnown
	default:
		return false
	}
}

func TestBinderArgsetStrictMalformedOccurrenceMatrix(t *testing.T) {
	fields := []struct {
		name     string
		valid    string
		fraction string
		negative string
		overflow string
		conflict string
	}{
		{name: "dest_proc", valid: "100", fraction: "100.0", negative: "-1", overflow: "2147483648", conflict: "99"},
		{name: "dest_thread", valid: "101", fraction: "101.0", negative: "-1", overflow: "2147483648", conflict: "99"},
		{name: "reply", valid: "0", fraction: "0.0", negative: "-1", overflow: "2147483648", conflict: "1"},
		{name: "flags", valid: "0x10", fraction: "0x10.0", negative: "-1", overflow: "0x100000000", conflict: "0x11"},
		{name: "code", valid: "0x3", fraction: "0x3.0", negative: "-1", overflow: "0x100000000", conflict: "0x4"},
	}
	for _, field := range fields {
		field := field
		mutations := []struct {
			name        string
			replacement string
		}{
			{name: "missing"},
			{name: "quoted", replacement: field.name + "=\"" + field.valid + "\""},
			{name: "fraction", replacement: field.name + "=" + field.fraction},
			{name: "junk", replacement: field.name + "=junk"},
			{name: "negative", replacement: field.name + "=" + field.negative},
			{name: "overflow", replacement: field.name + "=" + field.overflow},
			{name: "duplicate_identical", replacement: field.name + "=" + field.valid + " " + field.name + "=" + field.valid},
			{name: "duplicate_conflict", replacement: field.name + "=" + field.valid + " " + field.name + "=" + field.conflict},
		}
		for _, mutation := range mutations {
			mutation := mutation
			t.Run(field.name+"/"+mutation.name, func(t *testing.T) {
				body := binderStrictFieldsWith(field.name, mutation.replacement)
				_, typed := parseBinderTransactionTypedFields(string(EventBinderTransaction), body)
				if !typed.LexValid || !typed.TransactionKnown || typed.TransactionID != 42 {
					t.Fatalf("malformed non-key field poisoned the independent transaction identity: body=%q typed=%+v", body, typed)
				}
				if binderStrictTypedFieldKnown(typed, field.name) {
					t.Fatalf("malformed %s occurrence minted a known scalar: body=%q typed=%+v", field.name, body, typed)
				}
				for _, sibling := range fields {
					if sibling.name != field.name && !binderStrictTypedFieldKnown(typed, sibling.name) {
						t.Fatalf("malformed %s occurrence poisoned independent %s: body=%q typed=%+v", field.name, sibling.name, body, typed)
					}
				}
			})
		}
	}
}

func TestBinderArgsetStrictCanonicalBoundariesAndQuotedPseudoKeys(t *testing.T) {
	body := `note="dest_proc=999 dest_thread=999 reply=0 flags=0x11 code=0x2" transaction=2147483647 dest_proc=2147483647 dest_thread=0 reply=1 flags=0xffffffff code=0xffffffff`
	_, typed := parseBinderTransactionTypedFields(string(EventBinderTransaction), body)
	if !typed.LexValid || !typed.TransactionKnown || !typed.DestProcKnown || !typed.DestThreadKnown ||
		!typed.ReplyKnown || !typed.FlagsKnown || !typed.CodeKnown {
		t.Fatalf("canonical boundary fields were lost or quoted pseudo-keys reopened authority: %+v", typed)
	}
	if typed.TransactionID != 2147483647 || typed.DestProc != 2147483647 || typed.DestThread != 0 || typed.Reply != 1 ||
		typed.Flags != "0xffffffff" || typed.Code != "0xffffffff" || typed.FlagsValue != ^uint32(0) || typed.CodeValue != ^uint32(0) {
		t.Fatalf("canonical Binder boundary values changed: %+v", typed)
	}
}

func TestBinderTransactionIdentityStrictDomain(t *testing.T) {
	cases := []struct {
		name        string
		declaration string
	}{
		{name: "missing"},
		{name: "quoted", declaration: `transaction="42"`},
		{name: "fraction", declaration: "transaction=42.0"},
		{name: "junk", declaration: "transaction=bad"},
		{name: "negative", declaration: "transaction=-1"},
		{name: "zero", declaration: "transaction=0"},
		{name: "overflow", declaration: "transaction=2147483648"},
		{name: "duplicate_identical", declaration: "transaction=42 transaction=42"},
		{name: "duplicate_conflict", declaration: "transaction=42 debug_id=43"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := strings.TrimSpace(tc.declaration + " dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3")
			_, typed := parseBinderTransactionTypedFields(string(EventBinderTransaction), fields)
			if typed.TransactionKnown || typed.TransactionID != 0 {
				t.Fatalf("invalid Binder transaction identity was admitted: fields=%q typed=%+v", fields, typed)
			}
			verdict := DecodePairingEndpoint(string(EventBinderTransaction), fields, 20)
			if !verdict.Recognized || verdict.KeyKnown || verdict.PayloadAdmitted {
				t.Fatalf("invalid Binder transaction minted a pairing key: fields=%q verdict=%+v", fields, verdict)
			}
		})
	}

	_, padded := parseBinderTransactionTypedFields(string(EventBinderTransaction), "transaction=00042 dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3")
	if !padded.TransactionKnown || padded.TransactionID != 42 {
		t.Fatalf("established zero-padded transaction compatibility drifted: %+v", padded)
	}
}

func binderStrictGraph(t *testing.T, fields string, matched bool) IPCGraphResult {
	t.Helper()
	intern := newStringInterner()
	send, ok := ParseLine(1, "client-20 (20) [001] .... 1.000000: binder_transaction: "+fields, intern)
	if !ok {
		t.Fatalf("Binder send fixture did not parse: %q", fields)
	}
	events := []Event{send}
	if matched {
		recv, recvOK := ParseLine(2, "binder:100_1-101 (100) [002] .... 1.001000: binder_transaction_received: transaction=42", intern)
		if !recvOK {
			t.Fatal("Binder receive fixture did not parse")
		}
		events = append(events, recv)
	}
	return BuildIPCGraph(binderPairingIndex(events...), Query{TimeStart: 0.9, TimeEnd: 1.1})
}

func binderStrictOnlyEdge(t *testing.T, graph IPCGraphResult) IPCEdge {
	t.Helper()
	if len(graph.Edges) != 1 {
		t.Fatalf("expected one retained Binder edge, got edges=%+v caveats=%+v", graph.Edges, graph.Caveats)
	}
	return graph.Edges[0]
}

func binderStrictEdgeWire(t *testing.T, edge IPCEdge) map[string]any {
	t.Helper()
	raw, err := json.Marshal(edge)
	if err != nil {
		t.Fatalf("marshal Binder edge: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("decode Binder edge JSON: %v", err)
	}
	return wire
}

func binderStrictRequireWireBool(t *testing.T, wire map[string]any, key string, want bool) {
	t.Helper()
	got, ok := wire[key].(bool)
	if !ok || got != want {
		t.Fatalf("Binder edge %s must be the explicit public bool %t, got value=%#v wire=%+v", key, want, wire[key], wire)
	}
}

func binderStrictRequireWireString(t *testing.T, wire map[string]any, key, want string) {
	t.Helper()
	got, ok := wire[key].(string)
	if !ok || got != want {
		t.Fatalf("Binder edge %s must be %q, got value=%#v wire=%+v", key, want, wire[key], wire)
	}
}

func TestBinderCallSemanticsFourStatePublicContract(t *testing.T) {
	cases := []struct {
		name       string
		fields     string
		semantics  string
		replyKnown bool
		flagsKnown bool
		oneway     bool
		syncLike   bool
		blocking   bool
	}{
		{name: "sync_request", fields: binderStrictBaseFields, semantics: "sync_request", replyKnown: true, flagsKnown: true, syncLike: true, blocking: true},
		{name: "oneway_request", fields: binderStrictFieldsWith("flags", "flags=0x11"), semantics: "oneway_request", replyKnown: true, flagsKnown: true, oneway: true},
		{name: "reply", fields: binderStrictFieldsWith("reply", "reply=1"), semantics: "reply", replyKnown: true, flagsKnown: true},
		{name: "reply_with_unknown_flags", fields: strings.Replace(binderStrictFieldsWith("reply", "reply=1"), " flags=0x10", "", 1), semantics: "reply", replyKnown: true, flagsKnown: false},
		{name: "unknown", fields: binderStrictFieldsWith("flags", ""), semantics: "unknown", replyKnown: true, flagsKnown: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			edge := binderStrictOnlyEdge(t, binderStrictGraph(t, tc.fields, true))
			if edge.Oneway != tc.oneway || edge.SyncLike != tc.syncLike || edge.BlockingCandidate != tc.blocking {
				t.Fatalf("%s compatibility booleans diverged: edge=%+v", tc.semantics, edge)
			}
			wire := binderStrictEdgeWire(t, edge)
			binderStrictRequireWireString(t, wire, "call_semantics", tc.semantics)
			binderStrictRequireWireBool(t, wire, "destination_hint_known", true)
			binderStrictRequireWireBool(t, wire, "reply_known", tc.replyKnown)
			binderStrictRequireWireBool(t, wire, "flags_known", tc.flagsKnown)
			binderStrictRequireWireBool(t, wire, "code_known", true)
			binderStrictRequireWireString(t, wire, "receiver_source", "matched_receive")
		})
	}
}

func TestBinderBadDestinationMatchedAndSendOnlyBranches(t *testing.T) {
	badDest := binderStrictFieldsWith("dest_proc", "dest_proc=100 dest_proc=100")

	t.Run("matched_receive_survives", func(t *testing.T) {
		edge := binderStrictOnlyEdge(t, binderStrictGraph(t, badDest, true))
		if edge.Receiver.PID != 101 || edge.Receiver.TGID != 100 || edge.ReceiveLine != 2 {
			t.Fatalf("bad destination hint deleted or replaced the exact matched receive: %+v", edge)
		}
		wire := binderStrictEdgeWire(t, edge)
		binderStrictRequireWireBool(t, wire, "destination_hint_known", false)
		binderStrictRequireWireString(t, wire, "receiver_source", "matched_receive")
	})

	t.Run("send_only_cannot_forge_receiver", func(t *testing.T) {
		edge := binderStrictOnlyEdge(t, binderStrictGraph(t, badDest, false))
		if edge.Receiver.PID != 0 || edge.Receiver.TGID != 0 {
			t.Fatalf("invalid destination hint forged a send-only receiver: %+v", edge)
		}
		wire := binderStrictEdgeWire(t, edge)
		binderStrictRequireWireBool(t, wire, "destination_hint_known", false)
		binderStrictRequireWireString(t, wire, "receiver_source", "unresolved")
	})

	t.Run("valid_send_only_uses_dest_hint", func(t *testing.T) {
		edge := binderStrictOnlyEdge(t, binderStrictGraph(t, binderStrictBaseFields, false))
		if edge.Receiver.PID != 101 || edge.Receiver.TGID != 100 {
			t.Fatalf("valid destination hint did not resolve the send-only receiver: %+v", edge)
		}
		wire := binderStrictEdgeWire(t, edge)
		binderStrictRequireWireBool(t, wire, "destination_hint_known", true)
		binderStrictRequireWireString(t, wire, "receiver_source", "dest_hint")
	})
}

func TestBinderBadDestinationCannotMatchEventSearchPID(t *testing.T) {
	intern := newStringInterner()
	bad, ok := ParseLine(1, "client-20 (20) [001] .... 1.000000: binder_transaction: transaction=42 dest_proc=100 dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3", intern)
	if !ok {
		t.Fatal("bad-destination inventory row did not parse")
	}
	if eventMentionsPID(bad, 101) {
		t.Fatalf("invalid destination tuple forged an event_search TID match: %+v", bad.BinderFields)
	}
	good, ok := ParseLine(2, "client-20 (20) [001] .... 1.001000: binder_transaction: transaction=43 dest_proc=100 dest_thread=101 reply=0 flags=0x10 code=0x3", intern)
	if !ok || !eventMentionsPID(good, 101) {
		t.Fatalf("valid destination tuple lost its event_search TID match: %+v", good.BinderFields)
	}
}

func binderStrictWaitTrace(fields string) string {
	return fmt.Sprintf(`
     client-20   (   20) [001] .... 3.000000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=53
     client-20   (   20) [001] .... 3.010000: binder_transaction: %s
 binder:100_1-101 (  100) [002] .... 3.012000: binder_transaction_received: transaction=42
     client-20   (   20) [001] .... 3.015000: sched_switch: prev_comm=client prev_pid=20 prev_prio=53 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
 binder:100_1-101 (  100) [002] .... 3.020000: sched_wakeup: comm=client pid=20 prio=53 target_cpu=001
     client-20   (   20) [001] .... 3.030000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=20 next_prio=53
`, fields)
}

func binderStrictWaitChain(t *testing.T, name, fields string) ChainResult {
	t.Helper()
	idx := buildTraceIndex(t, name+".ftrace", binderStrictWaitTrace(fields))
	return BuildWakeupChain(idx, Query{PID: 20, TimeStart: 3.0, TimeEnd: 3.04, MaxDepth: 3, MaxBranches: 4, MinDurationMs: 1})
}

func binderStrictHasRootEvidenceType(chain ChainResult, kind string) bool {
	for _, item := range chain.RootEvidence {
		if item.Type == kind {
			return true
		}
	}
	return false
}

func TestBinderMalformedFlagsOrReplyCannotMintBinderWait(t *testing.T) {
	cases := []struct {
		name   string
		fields string
	}{
		{name: "flags_missing", fields: binderStrictFieldsWith("flags", "")},
		{name: "flags_overflow", fields: binderStrictFieldsWith("flags", "flags=0x100000000")},
		{name: "flags_duplicate", fields: binderStrictFieldsWith("flags", "flags=0x10 flags=0x10")},
		{name: "reply_missing", fields: binderStrictFieldsWith("reply", "")},
		{name: "reply_out_of_domain", fields: binderStrictFieldsWith("reply", "reply=2")},
		{name: "reply_duplicate", fields: binderStrictFieldsWith("reply", "reply=0 reply=0")},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			chain := binderStrictWaitChain(t, "binder_unknown_"+tc.name, tc.fields)
			edge := binderStrictOnlyEdge(t, IPCGraphResult{Edges: chain.IPCEdges, Caveats: chain.Caveats})
			if edge.Oneway || edge.SyncLike || edge.BlockingCandidate {
				t.Fatalf("unknown Binder call defaulted into a blocking candidate: %+v", edge)
			}
			if len(chain.BinderWaits) != 0 || binderStrictHasRootEvidenceType(chain, "binder_wait") {
				t.Fatalf("unknown Binder call minted causal wait evidence: waits=%+v roots=%+v", chain.BinderWaits, chain.RootEvidence)
			}
			wire := binderStrictEdgeWire(t, edge)
			binderStrictRequireWireString(t, wire, "call_semantics", "unknown")
		})
	}
}

func TestBinderReplyEdgeRetainedButNeverBlocking(t *testing.T) {
	chain := binderStrictWaitChain(t, "binder_reply_nonblocking", binderStrictFieldsWith("reply", "reply=1"))
	edge := binderStrictOnlyEdge(t, IPCGraphResult{Edges: chain.IPCEdges, Caveats: chain.Caveats})
	if edge.Reply != 1 || edge.ReceiveLine == 0 {
		t.Fatalf("reply edge or exact matched receive was deleted: %+v", edge)
	}
	if edge.Oneway || edge.SyncLike || edge.BlockingCandidate {
		t.Fatalf("reply edge entered request/blocking semantics: %+v", edge)
	}
	if len(chain.BinderWaits) != 0 || binderStrictHasRootEvidenceType(chain, "binder_wait") {
		t.Fatalf("reply edge minted BinderWait/root evidence: waits=%+v roots=%+v", chain.BinderWaits, chain.RootEvidence)
	}
	wire := binderStrictEdgeWire(t, edge)
	binderStrictRequireWireString(t, wire, "call_semantics", "reply")
	binderStrictRequireWireBool(t, wire, "reply_known", true)
}

func binderStrictReplyWriteOffIndex(t *testing.T, replyFields string, crossSource bool) (*Index, IPCEdge) {
	t.Helper()
	intern := newStringInterner()
	send, sendOK := ParseLine(1, "client-20 (20) [001] .... 5.000000: binder_transaction: "+binderStrictBaseFields, intern)
	recv, recvOK := ParseLine(2, "binder:900_1-901 (900) [002] .... 5.000100: binder_transaction_received: transaction=42", intern)
	replyLine := 3
	if crossSource {
		replyLine = 101
	}
	reply, replyOK := ParseLine(replyLine, "binder:900_1-901 (900) [002] .... 5.001000: binder_transaction: "+replyFields, intern)
	if !sendOK || !recvOK || !replyOK {
		t.Fatalf("source-scoped Binder fixture did not parse: send=%t recv=%t reply=%t", sendOK, recvOK, replyOK)
	}
	artifacts := []TraceArtifactSource{{SourcePath: "/trace/source-a.ftrace", LocalLineCount: 3, VirtualLineBase: 0, CausalCompatible: true}}
	if crossSource {
		artifacts[0].LocalLineCount = 2
		artifacts = append(artifacts, TraceArtifactSource{SourcePath: "/trace/source-b.ftrace", LocalLineCount: 1, VirtualLineBase: 100, CausalCompatible: true})
	}
	idx := &Index{
		Path:           "/trace/binder.tracebundle.json",
		LineCount:      replyLine,
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: artifacts,
		Events:         []Event{send, recv, reply},
	}
	graph := BuildIPCGraph(idx, Query{TimeStart: 4.9, TimeEnd: 5.1})
	for _, edge := range graph.Edges {
		if edge.TransactionID == 42 && edge.SendLine == 1 {
			return idx, edge
		}
	}
	t.Fatalf("source-scoped request edge missing: edges=%+v caveats=%+v", graph.Edges, graph.Caveats)
	return nil, IPCEdge{}
}

func binderStrictRunReplyWriteOff(idx *Index, edge IPCEdge) ([]BinderWaitSummary, string) {
	client := ThreadRef{Comm: "client", PID: 20, TGID: 20}
	chain := ChainResult{
		Nodes: []ChainNode{{
			Thread: client, Window: TimeWindow{StartTs: 5.050, EndTs: 5.060},
			Dominant: StateSSleep, DurationMs: 10, EvidenceLine: 10,
		}},
		Edges: []WakeupEdge{{
			Waker: ThreadRef{Comm: "binder:900_2", PID: 902, TGID: 900}, Wakee: client,
			WakeupTs: 5.060, WakeupLine: 11,
		}},
	}
	waits, _, caveat := findBinderWaitsForChain(idx, chain, []IPCEdge{edge}, nil)
	return waits, caveat
}

func TestBinderReplyWriteOffIsPhysicalSourceScoped(t *testing.T) {
	replyFields := "transaction=43 dest_proc=20 dest_thread=20 reply=1 flags=0x0 code=0x0"

	t.Run("same_source_reply_writes_off", func(t *testing.T) {
		idx, edge := binderStrictReplyWriteOffIndex(t, replyFields, false)
		waits, caveat := binderStrictRunReplyWriteOff(idx, edge)
		if len(waits) != 0 || !strings.Contains(caveat, "reply had already completed") {
			t.Fatalf("same-source strict reply did not write off stale request: waits=%+v caveat=%q", waits, caveat)
		}
	})

	t.Run("cross_source_reply_is_isolated", func(t *testing.T) {
		idx, edge := binderStrictReplyWriteOffIndex(t, replyFields, true)
		waits, caveat := binderStrictRunReplyWriteOff(idx, edge)
		if len(waits) != 1 || strings.Contains(caveat, "reply had already completed") {
			t.Fatalf("source B reply completed source A request: waits=%+v caveat=%q", waits, caveat)
		}
	})
}

func TestBinderMalformedReplyCannotWriteOffValidRequest(t *testing.T) {
	cases := []struct {
		name  string
		reply string
	}{
		{name: "out_of_domain", reply: "transaction=43 dest_proc=20 dest_thread=20 reply=2 flags=0x0 code=0x0"},
		{name: "negative", reply: "transaction=43 dest_proc=20 dest_thread=20 reply=-1 flags=0x0 code=0x0"},
		{name: "duplicate", reply: "transaction=43 dest_proc=20 dest_thread=20 reply=1 reply=1 flags=0x0 code=0x0"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			idx, edge := binderStrictReplyWriteOffIndex(t, tc.reply, false)
			waits, caveat := binderStrictRunReplyWriteOff(idx, edge)
			if len(waits) != 1 || strings.Contains(caveat, "reply had already completed") {
				t.Fatalf("malformed reply minted a completion proof: waits=%+v caveat=%q", waits, caveat)
			}
		})
	}
}
