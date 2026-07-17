package tracequery

import (
	"fmt"
	"sort"
	"strings"
)

func BuildIPCGraph(idx *Index, q Query) IPCGraphResult {
	q = normalizeQuery(idx, q)
	res := IPCGraphResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}
	res.Caveats = append(res.Caveats, traceMarkIntegrityCaveats(idx, q)...)
	interfaceJoinFailClosed := traceMarkUnknownEmitterFailureForQuery(idx, q)
	if interfaceJoinFailClosed {
		res.Caveats = append(res.Caveats, "trace_mark_interface_join_fail_closed=true; binder edges remain available, but transact-span interface joins are omitted because a malformed trace_mark endpoint has an unknown emitter, could not materialize as an Event, or overflowed the bounded witness ledger")
	}
	// Audit #41 (§29.25 处置委托 2026-07-10): the transact-interface join is a
	// strictly order-dependent per-PID B-push/E-pop stack walk. A physical
	// trace_span lane rollback makes that order untrustworthy — and, because a
	// composite index canonically re-sorts rows by (Ts, Line) while a direct
	// single-file query keeps physical order, the SAME capture used to yield
	// different interface attributions from the two supported entry points.
	// Both entry points preserve the physical-order poison (direct: lazy
	// event-scan on a non-monotonic index; composite: child-local reaudit
	// preserved before the canonical sort), so consuming it here fail-closes
	// the join identically on both. Binder edges themselves remain available.
	if !interfaceJoinFailClosed {
		if violation := durationOrderFailureForFamily(idx, q, durationOrderTraceSpan); violation != nil {
			interfaceJoinFailClosed = true
			res.Caveats = append(res.Caveats, "trace_mark_interface_join_fail_closed=true; binder edges remain available, but transact-span interface joins are omitted because the trace_span lane order is not trustworthy for this query: "+violation.reason())
		}
	}
	filterQ := q
	if filterQ.PID <= 0 && strings.TrimSpace(firstNonEmpty(filterQ.ThreadInput, filterQ.Thread)) != "" {
		resolution := resolveThreadSelection(idx, filterQ)
		if resolution.Ambiguous {
			res.Caveats = append(res.Caveats, threadResolutionCaveat(idx, filterQ))
			return res
		}
		if resolution.Thread.PID > 0 {
			filterQ.PID = resolution.Thread.PID
			filterQ.Thread = ""
			filterQ.ThreadInput = ""
		}
	}
	if filterQ.PID > 0 {
		if conflict := threadIncarnationConflictForQuery(idx, q, 0); conflict != nil {
			res.Caveats = append(res.Caveats, "thread_identity_fail_closed=true; "+conflict.reason()+"; IPC edges are omitted because the target numeric TID spans task incarnations")
			return res
		}
	}

	binderAudit := auditBinderPairing(idx, q)
	res.Caveats = append(res.Caveats, binderAudit.caveats()...)
	auxByLane := map[string][]BinderEventSummary{}
	var auxEvents []BinderEventSummary
	// C2 (2026-07-03, tightened 2026-07-12): deterministic
	// transact-interface join. Userspace wraps Binder sends in a
	// `transact[Interface:code]` trace-mark span on the SAME physical-source
	// thread. Replay is therefore grouped by source and ordered by physical
	// Line, never by the composite index's canonical timestamp order. A
	// timestamp rollback poisons only that precise source-thread join lane;
	// unresolved provenance fail-closes all interface joins while preserving
	// Binder edges. The joined name remains a verbatim span-name witness, not
	// a prose inference.
	type transactSourceThread struct {
		source string
		pid    int
	}
	openTransact := map[transactSourceThread][]string{}
	ifaceBySendLine := map[int]string{}
	unresolvedInterfaceRows := 0
	interfaceRowsBySource := map[string][]int{}
	for eventIndex, ev := range idx.Events {
		if q.runCancel.tick() {
			break
		}
		_, lifecycleReset := schedulerLifecycleResetPID(ev)
		traceStackRelevant := ev.Type == EventTraceMark && (ev.SpanAction == "B" || ev.SpanAction == "E" || traceMarkEventMalformed(ev))
		if !traceStackRelevant && ev.Type != EventBinderTransaction && !lifecycleReset {
			continue
		}
		source, sourceOK := tracePairingSourceIdentity(idx, ev)
		if !sourceOK {
			unresolvedInterfaceRows++
			continue
		}
		interfaceRowsBySource[source] = append(interfaceRowsBySource[source], eventIndex)
	}
	interfaceSources := make([]string, 0, len(interfaceRowsBySource))
	for source := range interfaceRowsBySource {
		interfaceSources = append(interfaceSources, source)
	}
	sort.Strings(interfaceSources)
	poisonedInterfaceKeys := map[transactSourceThread]bool{}
	interfaceTimestampRollbacks := 0
	for _, source := range interfaceSources {
		eventIndexes := interfaceRowsBySource[source]
		sort.SliceStable(eventIndexes, func(i, j int) bool {
			left, right := idx.Events[eventIndexes[i]], idx.Events[eventIndexes[j]]
			if left.Line != right.Line {
				return left.Line < right.Line
			}
			return left.Ts < right.Ts
		})
		lastTs := map[transactSourceThread]float64{}
		seenTs := map[transactSourceThread]bool{}
		for _, eventIndex := range eventIndexes {
			ev := idx.Events[eventIndex]
			pid := ev.PID
			if resetPID, reset := schedulerLifecycleResetPID(ev); reset {
				pid = resetPID
			}
			key := transactSourceThread{source: source, pid: pid}
			if seenTs[key] && ev.Ts < lastTs[key] {
				if !poisonedInterfaceKeys[key] {
					interfaceTimestampRollbacks++
				}
				poisonedInterfaceKeys[key] = true
			}
			lastTs[key] = ev.Ts
			seenTs[key] = true
		}
		for _, eventIndex := range eventIndexes {
			ev := idx.Events[eventIndex]
			if resetPID, reset := schedulerLifecycleResetPID(ev); reset {
				key := transactSourceThread{source: source, pid: resetPID}
				delete(openTransact, key)
				continue
			}
			key := transactSourceThread{source: source, pid: ev.PID}
			if poisonedInterfaceKeys[key] {
				delete(openTransact, key)
				continue
			}
			if ev.Type == EventTraceMark {
				if traceMarkEventMalformed(ev) {
					delete(openTransact, key)
					continue
				}
				if interfaceJoinFailClosed {
					continue
				}
				switch ev.SpanAction {
				case "B":
					if name, ok := transactSpanInterface(ev.SpanName); ok {
						openTransact[key] = append(openTransact[key], name)
					} else {
						openTransact[key] = append(openTransact[key], "")
					}
				case "E":
					if stack := openTransact[key]; len(stack) > 0 {
						openTransact[key] = stack[:len(stack)-1]
					}
				}
				continue
			}
			if ev.Type == EventBinderTransaction {
				if stack := openTransact[key]; len(stack) > 0 {
					for i := len(stack) - 1; i >= 0; i-- {
						if stack[i] != "" {
							ifaceBySendLine[ev.Line] = stack[i]
							break
						}
					}
				}
			}
		}
	}
	if interfaceTimestampRollbacks > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("trace_mark_interface_join_timestamp_regressed=true; source_thread_lanes=%d; Binder edges remain available, but affected transact-span interface joins were omitted because their physical endpoint stream moved backwards in time", interfaceTimestampRollbacks))
	}
	if unresolvedInterfaceRows > 0 {
		ifaceBySendLine = map[int]string{}
		res.Caveats = append(res.Caveats, fmt.Sprintf("trace_mark_interface_join_provenance_unresolved=true; rows=%d; binder edges remain available, but transact-span interface joins were omitted because relevant rows could not be mapped to exactly one physical source artifact", unresolvedInterfaceRows))
	}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			break
		}
		if !pairingEventInsideQuery(ev, q) {
			continue
		}
		if isBinderAuxEventType(ev.Type) {
			summary := binderEventSummaryFromEvent(ev)
			if binderEventMentionsQuery(ev, filterQ) {
				auxEvents = append(auxEvents, summary)
			}
			if lane, ok := binderAuxPairingLane(idx, ev); ok {
				auxByLane[lane] = append(auxByLane[lane], summary)
			}
		}
	}

	unmatchedReceives := 0
	sendOnly := 0
	type binderArgsetDegradation struct {
		rows, destination, reply, flags, code int
	}
	degraded := binderArgsetDegradation{}
	for _, send := range idx.Events {
		if q.runCancel.tick() {
			break
		}
		if send.Type != EventBinderTransaction {
			continue
		}
		if !pairingEventInsideQuery(send, q) {
			continue
		}
		laneKey, sendUsable := binderAudit.sendLane(send.Line)
		if !sendUsable {
			continue
		}
		edge := ipcEdgeFromSend(send)
		if source, ok := binderAudit.sourceForSend(send.Line); ok {
			edge.physicalSource = source
		}
		if bf := send.BinderFields; bf != nil && bf.argsetParsed {
			badDest := !bf.binderDestinationKnown()
			badReply := !bf.binderReplyKnown()
			_, _, flagsKnown := bf.binderFlags()
			_, codeKnown := bf.binderCode()
			badFlags := !flagsKnown
			badCode := !codeKnown
			if badDest || badReply || badFlags || badCode {
				degraded.rows++
				if badDest {
					degraded.destination++
				}
				if badReply {
					degraded.reply++
				}
				if badFlags {
					degraded.flags++
				}
				if badCode {
					degraded.code++
				}
			}
		}
		if iface := ifaceBySendLine[send.Line]; iface != "" {
			edge.Interface = iface
		}
		if recv, ok := binderAudit.matchForSend(send.Line); ok {
			edge.Receiver = threadRefFromEvent(recv)
			edge.ReceiveTs = recv.Ts
			edge.ReceiveLine = recv.Line
			edge.LatencyMs = (recv.Ts - send.Ts) * 1000
			edge.Confidence = 0.92
			edge.ReceiverSource = BinderReceiverSourceMatchedReceive
		} else if binderAudit.receivePrecededSend(send.Line) {
			edge.Caveats = append(edge.Caveats, "matching binder_transaction_received row(s) precede the send in physical order; temporally impossible matches were rejected")
		}
		endpointOnly := false
		if edge.Receiver.PID == 0 && edge.DestinationHintKnown && edge.DestThread > 0 {
			edge.Receiver = ThreadRef{PID: edge.DestThread, TGID: edge.DestProc}
			edge.Confidence = 0.62
			edge.ReceiverSource = BinderReceiverSourceDestHint
			edge.Caveats = append(edge.Caveats, "receiver inferred from dest_thread/dest_proc; no matching binder_transaction_received row in selected window")
			endpointOnly = true
		}
		if edge.Receiver.PID == 0 {
			edge.Confidence = 0.45
			edge.ReceiverSource = BinderReceiverSourceUnresolved
			edge.Caveats = append(edge.Caveats, "binder transaction has no receiver row or dest_thread hint in selected window")
			endpointOnly = true
		}
		if edge.CallSemantics == BinderCallSemanticsOnewayRequest {
			edge.Caveats = append(edge.Caveats, "flags suggest an asynchronous/oneway binder call; do not treat it as blocking without scheduler evidence")
		}
		edge.Caveats = append(edge.Caveats, binderAuxCaveatsForEdge(edge, auxByLane[laneKey])...)
		if ipcEdgeMentionsQuery(edge, filterQ) {
			res.Edges = append(res.Edges, edge)
			if endpointOnly {
				sendOnly++
			}
		}
	}
	for _, recv := range idx.Events {
		if q.runCancel.tick() {
			break
		}
		if recv.Type != EventBinderReceived || !pairingEventInsideQuery(recv, q) {
			continue
		}
		if binderAudit.receiveUsable(recv.Line) && !binderAudit.matchedReceives[recv.Line] && binderReceiveMentionsQuery(recv, filterQ) {
			unmatchedReceives++
		}
	}
	sort.SliceStable(res.Edges, func(i, j int) bool {
		if res.Edges[i].SendTs != res.Edges[j].SendTs {
			return res.Edges[i].SendTs < res.Edges[j].SendTs
		}
		return res.Edges[i].SendLine < res.Edges[j].SendLine
	})
	sort.SliceStable(auxEvents, func(i, j int) bool {
		if auxEvents[i].Ts != auxEvents[j].Ts {
			return auxEvents[i].Ts < auxEvents[j].Ts
		}
		return auxEvents[i].Line < auxEvents[j].Line
	})
	if q.Limit > 0 && len(res.Edges) > q.Limit {
		last := res.Edges[q.Limit-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "ipc_graph",
			Dimension:       CompactionDimensionEdges,
			Total:           len(res.Edges),
			Emitted:         q.Limit,
			LastEmittedTs:   last.SendTs,
			LastEmittedLine: last.SendLine,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("ipc graph compacted from %d to %d edge(s)", len(res.Edges), q.Limit))
		res.Edges = res.Edges[:q.Limit]
	}
	if q.Limit > 0 && len(auxEvents) > q.Limit {
		last := auxEvents[q.Limit-1]
		res.Compactions = append(res.Compactions, ViewCompaction{
			View:            "ipc_graph",
			Dimension:       CompactionDimensionEvents,
			Total:           len(auxEvents),
			Emitted:         q.Limit,
			LastEmittedTs:   last.Ts,
			LastEmittedLine: last.Line,
		})
		res.Caveats = append(res.Caveats, fmt.Sprintf("binder auxiliary events compacted from %d to %d row(s)", len(auxEvents), q.Limit))
		auxEvents = auxEvents[:q.Limit]
	}
	res.BinderEvents = auxEvents
	if degraded.rows > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("binder_argset_degraded=true; rows=%d destination_unknown=%d reply_unknown=%d flags_unknown=%d code_unknown=%d; affected dimensions were withheld instead of defaulting malformed or missing values", degraded.rows, degraded.destination, degraded.reply, degraded.flags, degraded.code))
	}
	if sendOnly > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d binder transaction row(s) were endpoint-only because matching receive rows were not visible in the selected window", sendOnly))
	}
	if unmatchedReceives > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d binder_transaction_received row(s) had no matching send row in the selected window", unmatchedReceives))
	}
	if len(res.Edges) == 0 && len(res.BinderEvents) == 0 && len(res.Caveats) == 0 {
		res.Caveats = append(res.Caveats, "no binder IPC edges were found in the selected trace window")
	}
	return res
}

func binderReceiveMentionsQuery(recv Event, q Query) bool {
	if q.PID > 0 {
		return recv.PID == q.PID
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(recv.Comm), strings.TrimSpace(q.Thread))
}

func ipcEdgeFromSend(send Event) IPCEdge {
	bf := binderFieldsForEdge(send)
	transactionID := 0
	if bf.binderTransactionKnown() {
		transactionID = bf.TransactionID
	}
	destKnown := bf.binderDestinationKnown()
	destProc, destThread := 0, 0
	if destKnown {
		destProc, destThread = bf.DestProc, bf.DestThread
	}
	replyKnown := bf.binderReplyKnown()
	reply := 0
	if replyKnown {
		reply = bf.Reply
	}
	flagsValue, flags, flagsKnown := bf.binderFlags()
	code, codeKnown := bf.binderCode()
	semantics := BinderCallSemanticsUnknown
	oneway, syncLike, blocking := false, false, false
	switch {
	case replyKnown && reply == 1:
		semantics = BinderCallSemanticsReply
	case !replyKnown || !flagsKnown:
		semantics = BinderCallSemanticsUnknown
	case binderFlagsOneway(flagsValue):
		semantics, oneway = BinderCallSemanticsOnewayRequest, true
	default:
		semantics, syncLike, blocking = BinderCallSemanticsSyncRequest, true, true
	}
	edge := IPCEdge{
		TransactionID:        transactionID,
		Sender:               threadRefFromEvent(send),
		DestProc:             destProc,
		DestThread:           destThread,
		SendTs:               send.Ts,
		SendLine:             send.Line,
		Reply:                reply,
		Flags:                flags,
		Code:                 code,
		CallSemantics:        semantics,
		DestinationHintKnown: destKnown,
		ReplyKnown:           replyKnown,
		FlagsKnown:           flagsKnown,
		CodeKnown:            codeKnown,
		ReceiverSource:       BinderReceiverSourceUnresolved,
		Oneway:               oneway,
		SyncLike:             syncLike,
		BlockingCandidate:    blocking,
		Confidence:           0.55,
	}
	if !destKnown {
		edge.Caveats = append(edge.Caveats, "binder destination tuple is absent, duplicate, malformed, or out of range; dest_thread fallback was withheld")
	}
	if !replyKnown {
		edge.Caveats = append(edge.Caveats, "binder reply field is absent, duplicate, malformed, or outside {0,1}; call semantics are unknown")
	}
	if !flagsKnown {
		edge.Caveats = append(edge.Caveats, "binder flags field is absent, duplicate, malformed, or outside uint32; call semantics are unknown and cannot be a blocking candidate")
	}
	if !codeKnown {
		edge.Caveats = append(edge.Caveats, "binder code field is absent, duplicate, malformed, or outside canonical uint32 hex; command disclosure was withheld")
	}
	return edge
}

func isBinderAuxEventType(typ EventType) bool {
	switch typ {
	case EventBinderAllocBuf, EventBinderLock, EventBinderLocked, EventBinderUnlock, EventBinderReply:
		return true
	default:
		return false
	}
}

func binderEventSummaryFromEvent(ev Event) BinderEventSummary {
	bf := ev.BinderFields
	if bf == nil {
		bf = &BinderFields{}
	}
	item := BinderEventSummary{
		Type:             ev.Type,
		Thread:           threadRefFromEvent(ev),
		TransactionID:    bf.TransactionID,
		DebugID:          bf.DebugID,
		DataSize:         bf.DataSize,
		OffsetsSize:      bf.OffsetsSize,
		ExtraBuffersSize: bf.ExtraSize,
		Tag:              bf.LockTag,
		Ts:               ev.Ts,
		Line:             ev.Line,
	}
	item.Summary = binderAuxSummary(item)
	return item
}

func binderAuxSummary(item BinderEventSummary) string {
	parts := []string{fmt.Sprintf("%s on %s", item.Type, threadLabel(item.Thread))}
	if item.TransactionID > 0 {
		parts = append(parts, fmt.Sprintf("transaction=%d", item.TransactionID))
	} else if item.DebugID > 0 {
		parts = append(parts, fmt.Sprintf("debug_id=%d", item.DebugID))
	}
	if item.DataSize > 0 || item.OffsetsSize > 0 || item.ExtraBuffersSize > 0 {
		parts = append(parts, fmt.Sprintf("data=%d offsets=%d extra=%d", item.DataSize, item.OffsetsSize, item.ExtraBuffersSize))
	}
	if strings.TrimSpace(item.Tag) != "" {
		parts = append(parts, "tag="+item.Tag)
	}
	return strings.Join(parts, " ")
}

func binderEventMentionsQuery(ev Event, q Query) bool {
	if q.PID > 0 {
		return ev.PID == q.PID
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(ev.Comm), strings.TrimSpace(q.Thread))
}

func binderAuxCaveatsForEdge(edge IPCEdge, auxItems []BinderEventSummary) []string {
	if edge.TransactionID <= 0 || edge.ReceiveLine <= 0 {
		return nil
	}
	var out []string
	for _, aux := range auxItems {
		if aux.Line < edge.SendLine || aux.Line > edge.ReceiveLine || aux.Ts < edge.SendTs || aux.Ts > edge.ReceiveTs {
			continue
		}
		switch aux.Type {
		case EventBinderAllocBuf:
			out = append(out, fmt.Sprintf("binder alloc buffer row at line %d: data_size=%d offsets_size=%d extra_buffers_size=%d", aux.Line, aux.DataSize, aux.OffsetsSize, aux.ExtraBuffersSize))
		case EventBinderLock, EventBinderLocked, EventBinderUnlock:
			out = append(out, fmt.Sprintf("%s row at line %d tag=%s", aux.Type, aux.Line, aux.Tag))
		case EventBinderReply:
			out = append(out, fmt.Sprintf("binder reply row at line %d", aux.Line))
		}
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func threadRefFromEvent(ev Event) ThreadRef {
	return ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}
}

func ipcEdgeMentionsQuery(edge IPCEdge, q Query) bool {
	if q.PID > 0 {
		return edge.Sender.PID == q.PID ||
			edge.Receiver.PID == q.PID ||
			edge.DestThread == q.PID
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(edge.Sender.Comm), strings.TrimSpace(q.Thread)) ||
		strings.EqualFold(strings.TrimSpace(edge.Receiver.Comm), strings.TrimSpace(q.Thread))
}

func binderFlagsOneway(flags uint32) bool {
	return flags&0x1 != 0
}

// transactSpanInterface extracts the interface token from a userspace
// binder wrapper span name of the exact form `transact[<Interface:code>]`.
// Verbatim structural parse — anything else reports ok=false.
func transactSpanInterface(spanName string) (string, bool) {
	spanName = strings.TrimSpace(spanName)
	const prefix = "transact["
	if !strings.HasPrefix(spanName, prefix) || !strings.HasSuffix(spanName, "]") {
		return "", false
	}
	inner := strings.TrimSpace(spanName[len(prefix) : len(spanName)-1])
	if inner == "" {
		return "", false
	}
	return inner, true
}
