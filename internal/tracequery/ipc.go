package tracequery

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func BuildIPCGraph(idx *Index, q Query) IPCGraphResult {
	q = normalizeQuery(idx, q)
	res := IPCGraphResult{Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if idx == nil {
		res.Caveats = append(res.Caveats, "trace index is empty")
		return res
	}

	receives := map[int][]Event{}
	auxByTx := map[int][]BinderEventSummary{}
	var auxEvents []BinderEventSummary
	for _, ev := range idx.Events {
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		switch {
		case ev.Type == EventBinderReceived:
			if ev.BinderTransactionID > 0 {
				receives[ev.BinderTransactionID] = append(receives[ev.BinderTransactionID], ev)
			}
		case isBinderAuxEventType(ev.Type):
			summary := binderEventSummaryFromEvent(ev)
			if binderEventMentionsQuery(ev, q) {
				auxEvents = append(auxEvents, summary)
			}
			if ev.BinderTransactionID > 0 {
				auxByTx[ev.BinderTransactionID] = append(auxByTx[ev.BinderTransactionID], summary)
			}
		}
	}
	for id := range receives {
		sort.SliceStable(receives[id], func(i, j int) bool {
			return receives[id][i].Ts < receives[id][j].Ts
		})
	}

	unmatchedReceives := 0
	matchedReceives := map[int]bool{}
	sendOnly := 0
	for _, send := range idx.Events {
		if send.Type != EventBinderTransaction {
			continue
		}
		if !eventLineInWindow(send, q) || !timeInWindow(send.Ts, q) {
			continue
		}
		edge := ipcEdgeFromSend(send)
		if send.BinderTransactionID > 0 {
			if recv, ok := chooseBinderReceive(send, receives[send.BinderTransactionID]); ok {
				edge.Receiver = threadRefFromEvent(recv)
				edge.ReceiveTs = recv.Ts
				edge.ReceiveLine = recv.Line
				if recv.Ts >= send.Ts {
					edge.LatencyMs = (recv.Ts - send.Ts) * 1000
				}
				edge.Confidence = 0.92
				matchedReceives[recv.Line] = true
			}
		}
		endpointOnly := false
		if edge.Receiver.PID == 0 && edge.DestThread > 0 {
			edge.Receiver = ThreadRef{PID: edge.DestThread, TGID: edge.DestProc}
			edge.Confidence = 0.62
			edge.Caveats = append(edge.Caveats, "receiver inferred from dest_thread/dest_proc; no matching binder_transaction_received row in selected window")
			endpointOnly = true
		}
		if edge.Receiver.PID == 0 {
			edge.Confidence = 0.45
			edge.Caveats = append(edge.Caveats, "binder transaction has no receiver row or dest_thread hint in selected window")
			endpointOnly = true
		}
		if edge.Oneway {
			edge.Caveats = append(edge.Caveats, "flags suggest an asynchronous/oneway binder call; do not treat it as blocking without scheduler evidence")
		}
		edge.Caveats = append(edge.Caveats, binderAuxCaveatsForEdge(edge, auxByTx)...)
		if ipcEdgeMentionsQuery(edge, q) {
			res.Edges = append(res.Edges, edge)
			if endpointOnly {
				sendOnly++
			}
		}
	}
	for _, group := range receives {
		for _, recv := range group {
			if !matchedReceives[recv.Line] && binderReceiveMentionsQuery(recv, q) {
				unmatchedReceives++
			}
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
		return recv.PID == q.PID || recv.TGID == q.PID
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(recv.Comm), needle)
}

func ipcEdgeFromSend(send Event) IPCEdge {
	oneway := binderFlagsOneway(send.BinderFlags)
	syncLike := !oneway
	return IPCEdge{
		TransactionID:     send.BinderTransactionID,
		Sender:            threadRefFromEvent(send),
		DestProc:          send.BinderDestProc,
		DestThread:        send.BinderDestThread,
		SendTs:            send.Ts,
		SendLine:          send.Line,
		Reply:             send.BinderReply,
		Flags:             send.BinderFlags,
		Code:              send.BinderCode,
		Oneway:            oneway,
		SyncLike:          syncLike,
		BlockingCandidate: syncLike,
		Confidence:        0.55,
	}
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
	item := BinderEventSummary{
		Type:             ev.Type,
		Thread:           threadRefFromEvent(ev),
		TransactionID:    ev.BinderTransactionID,
		DebugID:          ev.BinderDebugID,
		DataSize:         ev.BinderDataSize,
		OffsetsSize:      ev.BinderOffsetsSize,
		ExtraBuffersSize: ev.BinderExtraSize,
		Tag:              ev.BinderLockTag,
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
		return ev.PID == q.PID || ev.TGID == q.PID
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(ev.Comm), needle) || strings.Contains(strings.ToLower(ev.FieldText), needle)
}

func binderAuxCaveatsForEdge(edge IPCEdge, auxByTx map[int][]BinderEventSummary) []string {
	if edge.TransactionID <= 0 {
		return nil
	}
	var out []string
	for _, aux := range auxByTx[edge.TransactionID] {
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

func chooseBinderReceive(send Event, candidates []Event) (Event, bool) {
	if len(candidates) == 0 {
		return Event{}, false
	}
	for _, recv := range candidates {
		if recv.Ts >= send.Ts {
			return recv, true
		}
	}
	return candidates[0], true
}

func threadRefFromEvent(ev Event) ThreadRef {
	return ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}
}

func ipcEdgeMentionsQuery(edge IPCEdge, q Query) bool {
	if q.PID > 0 {
		return edge.Sender.PID == q.PID ||
			edge.Sender.TGID == q.PID ||
			edge.Receiver.PID == q.PID ||
			edge.Receiver.TGID == q.PID ||
			edge.DestProc == q.PID ||
			edge.DestThread == q.PID
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(edge.Sender.Comm), needle) ||
		strings.Contains(strings.ToLower(edge.Receiver.Comm), needle)
}

func binderFlagsOneway(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	base := 10
	if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
		raw = raw[2:]
		base = 16
	}
	n, err := strconv.ParseInt(raw, base, 64)
	if err != nil {
		return false
	}
	return n&0x1 != 0
}
