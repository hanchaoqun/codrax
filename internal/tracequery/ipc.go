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
	for _, ev := range idx.Events {
		if ev.Type != EventBinderReceived {
			continue
		}
		if !eventLineInWindow(ev, q) || !timeInWindow(ev.Ts, q) {
			continue
		}
		if ev.BinderTransactionID > 0 {
			receives[ev.BinderTransactionID] = append(receives[ev.BinderTransactionID], ev)
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
	if q.Limit > 0 && len(res.Edges) > q.Limit {
		res.Caveats = append(res.Caveats, fmt.Sprintf("ipc graph compacted from %d to %d edge(s)", len(res.Edges), q.Limit))
		res.Edges = res.Edges[:q.Limit]
	}
	if sendOnly > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d binder transaction row(s) were endpoint-only because matching receive rows were not visible in the selected window", sendOnly))
	}
	if unmatchedReceives > 0 {
		res.Caveats = append(res.Caveats, fmt.Sprintf("%d binder_transaction_received row(s) had no matching send row in the selected window", unmatchedReceives))
	}
	if len(res.Edges) == 0 && len(res.Caveats) == 0 {
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
	return IPCEdge{
		TransactionID: send.BinderTransactionID,
		Sender:        threadRefFromEvent(send),
		DestProc:      send.BinderDestProc,
		DestThread:    send.BinderDestThread,
		SendTs:        send.Ts,
		SendLine:      send.Line,
		Reply:         send.BinderReply,
		Flags:         send.BinderFlags,
		Code:          send.BinderCode,
		Oneway:        binderFlagsOneway(send.BinderFlags),
		Confidence:    0.55,
	}
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
