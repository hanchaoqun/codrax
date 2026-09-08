package tracequery

import "sort"

// TargetWindowBinderWaitInventory accounts for the verified, reply-associated
// closed waits in the constructed target timeline. It is independent of chain
// selection and is NOT a root-cause qualification or a claim that every sleep
// has a known mechanism. Counts partition positive S/D/IO timeline intervals;
// ConfirmedMs is their window-clamped union, before the display cap. A complete
// scan describes retained index evidence, never completeness of trace capture.
type TargetWindowBinderWaitInventory struct {
	Thread                     ThreadRef                          `json:"thread"`
	Window                     TimeWindow                         `json:"window"`
	Scope                      string                             `json:"scope"`
	ScanStatus                 string                             `json:"scan_status"`
	OutputStatus               string                             `json:"output_status"`
	TargetSleepCount           int                                `json:"target_sleep_count"`
	ConfirmedCount             int                                `json:"confirmed_count"`
	ConfirmedMs                float64                            `json:"confirmed_ms"`
	UnresolvedCandidateCount   int                                `json:"unresolved_candidate_count"`
	RemainingUnassociatedCount int                                `json:"remaining_unassociated_count"`
	Emitted                    int                                `json:"emitted"`
	HeadState                  *TimelineHeadState                 `json:"head_state,omitempty"`
	CausalAttributionStatus    string                             `json:"causal_attribution_status"`
	UnresolvedReasons          []string                           `json:"unresolved_reasons,omitempty"`
	Occurrences                []TargetWindowBinderWaitOccurrence `json:"occurrences"`
}

// TargetWindowBinderWaitOccurrence preserves the scheduler interval and the
// four exact Binder endpoints, plus an independently checked physical wakeup.
// Request and reply transaction IDs need not be equal. Source ownership is the
// enclosing index's global line ledger; no filename or namespace alias is
// inferred, and none of these supporting observations creates a chain edge.
type TargetWindowBinderWaitOccurrence struct {
	Ordinal int `json:"ordinal"`
	Interval
	Peer                 ThreadRef `json:"peer"`
	ClosureStatus        string    `json:"closure_status"`
	RequestTransactionID int       `json:"request_transaction_id"`
	ReplyTransactionID   int       `json:"reply_transaction_id"`
	RequestSendLine      int       `json:"request_send_line"`
	RequestReceiveLine   int       `json:"request_receive_line"`
	ReplySendLine        int       `json:"reply_send_line"`
	ReplyReceiveLine     int       `json:"reply_receive_line"`
	ClosureLine          int       `json:"closure_line"`
	RequestSendTs        float64   `json:"request_send_ts"`
	RequestReceiveTs     float64   `json:"request_receive_ts"`
	ReplySendTs          float64   `json:"reply_send_ts"`
	ReplyReceiveTs       float64   `json:"reply_receive_ts"`
	ClosureTs            float64   `json:"closure_ts"`
}

const targetWindowBinderWaitInventoryCap = 32

type binderInventoryRequest struct {
	send, receive, replySend, replyReceive Event
	source                                 string
	semantics                              BinderCallSemantics
	ambiguous                              bool
	clientScope                            threadGenerationScope
}

// buildTargetWindowBinderWaitInventory uses the existing exact physical
// transaction pairing grammar, not the chain's deliberately broader candidate
// heuristic. Reply IDs are different from request IDs: only a unique pending
// synchronous call to the physically observed receiver may consume a reply.
// Ambiguous/reentrant calls remain unresolved instead of being popped FIFO.
func buildTargetWindowBinderWaitInventory(idx *Index, q Query, tl TimelineResult, window TimeWindow) *TargetWindowBinderWaitInventory {
	sleep := buildTargetWindowSleepInventory(tl, window)
	if idx == nil || sleep == nil || tl.Thread.PID <= 0 || q.runCancel.fired() {
		return nil
	}
	defer func() {
		if q.runCancel.fired() {
			q.runCancel.discardFace("target_window_binder_wait_inventory")
		}
	}()
	out := &TargetWindowBinderWaitInventory{
		Thread: tl.Thread, Window: window, Scope: "indexed_target_verified_closed_waits",
		ScanStatus: "complete", OutputStatus: "complete", TargetSleepCount: sleep.Total,
		HeadState: sleep.HeadState, CausalAttributionStatus: "not_assessed",
		Occurrences: make([]TargetWindowBinderWaitOccurrence, 0),
	}
	reasons := map[string]bool{}
	// The selected window gates accounting, not endpoint pairing. In
	// particular, a reply can be received after its physical waking event and
	// outside the selected window. Preserve cooperative cancellation.
	audit := auditBinderPairing(idx, Query{runCancel: q.runCancel})
	if q.runCancel.fired() {
		return nil
	}
	globalUnsafe := audit.familyGlobal || idx.RelationScoped
	if globalUnsafe {
		out.ScanStatus = "incomplete"
		reasons["binder_endpoint_topology_incomplete"] = true
	}
	sourceUnsafe := map[string]bool{}
	for source := range audit.globalSources {
		sourceUnsafe[source] = true
	}
	// A rejected request must not disappear and make a remaining request
	// appear unique. Pairing's exact-lane quarantine is retained; this wider
	// *accounting* proof additionally withholds the affected source's pending
	// set when the rejected endpoint's client identity cannot be established.
	for _, failure := range idx.durationOrderFailures {
		if failure.Family != durationOrderBinder {
			continue
		}
		if source, ok := durationPairingFailureSource(idx, failure); ok {
			sourceUnsafe[source] = true
		} else {
			globalUnsafe = true
		}
	}
	type sourceEvent struct {
		event  Event
		source string
	}
	var endpoints []sourceEvent
	lineEvents := map[int][]Event{}
	targetWakes := map[float64][]sourceEvent{}
	wakesBySource := map[string][]Event{}
	schedulerAudit := binderInventorySchedulerAudit{failures: append([]schedulerOrderViolation(nil), idx.schedulerOrderFailures...), capped: idx.schedulerOrderFailuresCapped}
	cpuOrder, pidOrder := newSchedulerOrderTracker(), newSchedulerOrderTracker()
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		// One bounded order audit serves every occurrence. Re-scanning the
		// complete index separately for every small wait would be quadratic.
		for _, failure := range auditSchedulerOrderEvent(cpuOrder, pidOrder, ev) {
			if schedulerOrderViolationRelevantToQuery(&failure, Query{}, tl.Thread.PID) {
				if len(schedulerAudit.failures) < schedulerOrderFailureCap {
					schedulerAudit.failures = append(schedulerAudit.failures, failure)
				} else {
					schedulerAudit.capped = true
				}
			}
		}
		if ev.Type == EventSchedSwitch && ev.PrevPID == tl.Thread.PID {
			lineEvents[ev.Line] = append(lineEvents[ev.Line], ev)
		}
		if ev.Type == EventSchedWakeup && ev.WakeePID == tl.Thread.PID && !schedWakeupStartsNewIncarnation(ev) {
			if source, ok := tracePairingSourceIdentity(idx, ev); ok {
				targetWakes[ev.Ts] = append(targetWakes[ev.Ts], sourceEvent{ev, source})
				wakesBySource[source] = append(wakesBySource[source], ev)
			}
		}
		if ev.Type != EventBinderTransaction && ev.Type != EventBinderReceived {
			continue
		}
		source, ok := tracePairingSourceIdentity(idx, ev)
		if !ok {
			globalUnsafe = true
			continue
		}
		verdict := binderEndpointVerdictForEvent(ev)
		key, keyOK := verdict.LaneKey(source)
		if !keyOK || audit.poisonedKeys[key] || audit.suppressedSends[ev.Line] {
			sourceUnsafe[source] = true
		}
		endpoints = append(endpoints, sourceEvent{ev, source})
	}
	for source := range wakesBySource {
		sort.SliceStable(wakesBySource[source], func(i, j int) bool { return wakesBySource[source][i].Line < wakesBySource[source][j].Line })
	}
	if globalUnsafe || len(sourceUnsafe) > 0 {
		out.ScanStatus = "incomplete"
		reasons["binder_endpoint_set_unproven"] = true
	}
	if schedulerAudit.capped {
		out.ScanStatus = "incomplete"
		reasons["scheduler_order_audit_incomplete"] = true
	}
	sort.SliceStable(endpoints, func(i, j int) bool {
		if endpoints[i].source != endpoints[j].source {
			return endpoints[i].source < endpoints[j].source
		}
		return endpoints[i].event.Line < endpoints[j].event.Line
	})
	// Incoming synchronous requests are precise evidence of a callback or
	// nested dispatch. Do not interpret a nested stack by naming or proximity.
	incoming := map[int]bool{}
	for _, endpoint := range endpoints {
		if endpoint.event.Type == EventBinderTransaction && ipcEdgeFromSend(endpoint.event).CallSemantics == BinderCallSemanticsSyncRequest {
			if receive, ok := audit.matchForSend(endpoint.event.Line); ok && receive.PID == tl.Thread.PID {
				incoming[receive.Line] = true
			}
		}
	}
	var requests []*binderInventoryRequest
	pending := map[string][]*binderInventoryRequest{}
	for _, endpoint := range endpoints {
		if q.runCancel.tick() {
			return nil
		}
		ev, source := endpoint.event, endpoint.source
		if globalUnsafe {
			// No pair from a globally incomplete endpoint audit is eligible.
			// Preserve raw target candidates without constructing an unbounded
			// pending cross product that cannot possibly yield a proof.
			if ev.Type == EventBinderTransaction && ev.PID == tl.Thread.PID {
				semantics := ipcEdgeFromSend(ev).CallSemantics
				if semantics == BinderCallSemanticsSyncRequest || semantics == BinderCallSemanticsUnknown {
					requests = append(requests, &binderInventoryRequest{send: ev, source: source, semantics: semantics,
						clientScope: threadGenerationScopeAt(idx, tl.Thread.PID, ev.Ts, ev.Line)})
				}
			}
			continue
		}
		active := pending[source][:0]
		for _, request := range pending[source] {
			if q.runCancel.tick() {
				return nil
			}
			if request.replyReceive.Line > 0 && request.replyReceive.Line < ev.Line {
				continue
			}
			generation := request.clientScope
			if generation.known && !generation.contains(ev.Ts, ev.Line) {
				continue // an explicit lifecycle reset ends this pending domain
			}
			active = append(active, request)
		}
		pending[source] = active
		if incoming[ev.Line] {
			for _, request := range active {
				if q.runCancel.tick() {
					return nil
				}
				request.ambiguous = true
			}
		}
		if ev.Type != EventBinderTransaction {
			continue
		}
		edge := ipcEdgeFromSend(ev)
		if ev.PID == tl.Thread.PID && (edge.CallSemantics == BinderCallSemanticsSyncRequest || edge.CallSemantics == BinderCallSemanticsUnknown) {
			request := &binderInventoryRequest{send: ev, source: source, semantics: edge.CallSemantics, ambiguous: len(active) > 0,
				clientScope: threadGenerationScopeAt(idx, tl.Thread.PID, ev.Ts, ev.Line)}
			request.receive, _ = audit.matchForSend(ev.Line)
			for _, previous := range active {
				if q.runCancel.tick() {
					return nil
				}
				previous.ambiguous = true
			}
			requests = append(requests, request)
			pending[source] = append(active, request)
			continue
		}
		if edge.CallSemantics != BinderCallSemanticsReply {
			if edge.CallSemantics == BinderCallSemanticsUnknown {
				for _, request := range active {
					if request.receive.PID == ev.PID {
						request.ambiguous = true
					}
				}
			}
			continue
		}
		receive, ok := audit.matchForSend(ev.Line)
		if !ok || receive.PID != tl.Thread.PID || !binderInventoryDestinationMatches(edge, receive, true) {
			// An earlier incomplete or contradictory reply cannot be erased
			// and replaced by a later conveniently matching reply.
			if receive.PID == tl.Thread.PID || edge.DestThread == tl.Thread.PID || !edge.DestinationHintKnown {
				for _, request := range active {
					if request.receive.PID == ev.PID {
						request.ambiguous = true
					}
				}
			}
			continue
		}
		var candidates []*binderInventoryRequest
		for _, request := range active {
			if q.runCancel.tick() {
				return nil
			}
			if request.replySend.Line == 0 && request.receive.PID == ev.PID &&
				binderInventoryPointBefore(request.receive, ev) && request.semantics == BinderCallSemanticsSyncRequest {
				candidates = append(candidates, request)
			}
		}
		if len(candidates) == 1 {
			request := candidates[0]
			request.replySend, request.replyReceive = ev, receive
			request.ambiguous = request.ambiguous || len(active) != 1
		} else if len(candidates) > 1 {
			for _, request := range candidates {
				request.ambiguous = true
			}
		}
	}
	requestsBySource := map[string][]*binderInventoryRequest{}
	for _, request := range requests {
		requestsBySource[request.source] = append(requestsBySource[request.source], request)
	}
	requestIndexes := map[string]*binderInventoryRequestIndex{}
	for source, items := range requestsBySource {
		requestIndexes[source] = newBinderInventoryRequestIndex(items)
	}
	allRequests := newBinderInventoryRequestIndex(requests)
	var union []foldInterval
	for _, interval := range tl.Intervals {
		if q.runCancel.tick() {
			return nil
		}
		if !binderInventorySleep(interval) {
			continue
		}
		openEvents := lineEvents[interval.StartLine]
		var open Event
		source := ""
		if len(openEvents) == 1 {
			open = openEvents[0]
			source, _ = tracePairingSourceIdentity(idx, open)
		}
		index := requestIndexes[source]
		if source == "" {
			index = allRequests // unknown source cannot borrow a guessed namespace
		}
		candidates := index.candidates(interval, q.runCancel, nil)
		if len(candidates) == 0 {
			out.RemainingUnassociatedCount++
			continue
		}
		reason := "multiple_pending_requests"
		var row TargetWindowBinderWaitOccurrence
		if len(candidates) == 1 {
			request := candidates[0]
			var wakes []Event
			for _, candidate := range targetWakes[interval.ActualEndTs] {
				if candidate.source == source {
					wakes = append(wakes, candidate.event)
				}
			}
			row, reason = binderInventoryClosedOccurrence(idx, tl.Thread, interval, open, wakes, request, schedulerAudit)
			if reason == "" {
				// Timeline's last-wakeup carry must not turn an earlier physical
				// wake followed by a repeated notification into extra sleep time.
				ordered := wakesBySource[source]
				first := sort.Search(len(ordered), func(i int) bool { return ordered[i].Line > open.Line })
				if first >= len(ordered) || ordered[first].Line != row.ClosureLine {
					reason = "earlier_target_wakeup_observed"
				}
			}
			if globalUnsafe || sourceUnsafe[request.source] {
				reason = "binder_endpoint_set_unproven"
			}
		}
		if reason != "" {
			out.UnresolvedCandidateCount++
			reasons[reason] = true
			continue
		}
		out.ConfirmedCount++
		union = append(union, foldInterval{start: interval.StartTs, end: interval.EndTs})
		out.Occurrences = append(out.Occurrences, row)
	}
	if q.runCancel.fired() {
		return nil
	}
	out.ConfirmedMs, _ = foldIntervalUnionMs(union)
	sort.SliceStable(out.Occurrences, func(i, j int) bool {
		a, b := out.Occurrences[i], out.Occurrences[j]
		if a.StartTs != b.StartTs {
			return a.StartTs < b.StartTs
		}
		return a.StartLine < b.StartLine
	})
	if len(out.Occurrences) > targetWindowBinderWaitInventoryCap {
		out.OutputStatus = "incomplete"
		out.Occurrences = out.Occurrences[:targetWindowBinderWaitInventoryCap]
	}
	for i := range out.Occurrences {
		out.Occurrences[i].Ordinal = i + 1
	}
	out.Emitted = len(out.Occurrences)
	for reason := range reasons {
		out.UnresolvedReasons = append(out.UnresolvedReasons, reason)
	}
	sort.Strings(out.UnresolvedReasons)
	return out
}

func binderInventorySleep(interval Interval) bool {
	return interval.EndTs > interval.StartTs && (interval.State == StateSSleep || interval.State == StateDSleep || interval.State == StateIOWait)
}

func binderInventoryPointBefore(a, b Event) bool {
	return a.Line > 0 && b.Line > a.Line && finiteSleepInventoryTime(a.Ts) && finiteSleepInventoryTime(b.Ts) && b.Ts >= a.Ts
}

func binderInventoryDestinationMatches(edge IPCEdge, receive Event, requireThread bool) bool {
	if !edge.DestinationHintKnown || receive.PID <= 0 || (requireThread && edge.DestThread != receive.PID) {
		return false
	}
	if edge.DestThread > 0 && edge.DestThread != receive.PID {
		return false
	}
	return edge.DestProc <= 0 || (receive.TGID > 0 && edge.DestProc == receive.TGID)
}

func binderInventoryClosedOccurrence(idx *Index, target ThreadRef, interval Interval, open Event, wakes []Event, request *binderInventoryRequest, schedulerAudit binderInventorySchedulerAudit) (TargetWindowBinderWaitOccurrence, string) {
	var row TargetWindowBinderWaitOccurrence
	if request.semantics != BinderCallSemanticsSyncRequest {
		return row, "request_call_semantics_unknown"
	}
	if request.ambiguous {
		return row, "nested_or_multiple_pending_requests"
	}
	if request.receive.Line <= 0 {
		return row, "request_receive_missing"
	}
	if request.replySend.Line <= 0 || request.replyReceive.Line <= 0 {
		return row, "reply_pair_missing_or_ambiguous"
	}
	if !binderInventoryDestinationMatches(ipcEdgeFromSend(request.send), request.receive, false) {
		return row, "request_destination_identity_unproven"
	}
	if len(wakes) != 1 {
		return row, "physical_wakeup_missing_or_ambiguous"
	}
	wake := wakes[0]
	if open.Type != EventSchedSwitch || open.PrevPID != target.PID || open.Ts != interval.ActualStartTs ||
		(stateFromPrevState(open.PrevState) != StateSSleep && stateFromPrevState(open.PrevState) != StateDSleep) ||
		wake.Type != EventSchedWakeup || schedWakeupStartsNewIncarnation(wake) || wake.WakeePID != target.PID ||
		wake.PID != request.receive.PID || wake.PID != request.replySend.PID || wake.Ts != interval.ActualEndTs {
		return row, "physical_sleep_or_peer_wakeup_unproven"
	}
	if binderInventoryProcessConflict(request.send, request.replyReceive) || binderInventoryProcessConflict(request.receive, request.replySend, wake) {
		return row, "observed_process_identity_conflict"
	}
	for _, event := range []Event{open, wake, request.send, request.receive, request.replySend, request.replyReceive} {
		if source, ok := tracePairingSourceIdentity(idx, event); !ok || source != request.source {
			return row, "physical_source_mismatch"
		}
	}
	if !binderInventoryPointBefore(request.send, open) || !binderInventoryPointBefore(open, wake) ||
		!binderInventoryPointBefore(request.send, request.receive) || !binderInventoryPointBefore(request.receive, request.replySend) ||
		!binderInventoryPointBefore(request.replySend, wake) || !binderInventoryPointBefore(wake, request.replyReceive) {
		return row, "physical_order_unproven"
	}
	clientScope := request.clientScope
	peerScope := threadGenerationScopeAt(idx, request.receive.PID, request.receive.Ts, request.receive.Line)
	if !clientScope.contains(open.Ts, open.Line) || !clientScope.contains(wake.Ts, wake.Line) || !clientScope.contains(request.replyReceive.Ts, request.replyReceive.Line) ||
		!peerScope.contains(request.replySend.Ts, request.replySend.Line) || !peerScope.contains(wake.Ts, wake.Line) {
		return row, "thread_lifecycle_unproven"
	}
	// Query-window scheduler integrity alone does not certify the physical
	// opening/closure outside a clipped window. Reuse the same precise audit
	// over the actual span, retaining cancellation and source ledger checks.
	physicalQ := Query{TimeStart: request.send.Ts, TimeEnd: request.replyReceive.Ts, TimeStartSet: true, TimeEndSet: true}
	if schedulerAudit.fails(idx, physicalQ, target.PID) {
		return row, "scheduler_integrity_unproven"
	}
	row = TargetWindowBinderWaitOccurrence{
		Interval: interval, Peer: threadRefFromEvent(request.receive), ClosureStatus: "verified_reply_wakeup",
		RequestTransactionID: ipcEdgeFromSend(request.send).TransactionID, ReplyTransactionID: ipcEdgeFromSend(request.replySend).TransactionID,
		RequestSendLine: request.send.Line, RequestReceiveLine: request.receive.Line, ReplySendLine: request.replySend.Line, ReplyReceiveLine: request.replyReceive.Line,
		ClosureLine: wake.Line, RequestSendTs: request.send.Ts, RequestReceiveTs: request.receive.Ts,
		ReplySendTs: request.replySend.Ts, ReplyReceiveTs: request.replyReceive.Ts, ClosureTs: wake.Ts,
	}
	return row, ""
}

func binderInventoryProcessConflict(events ...Event) bool {
	known := 0
	for _, event := range events {
		if event.TGID <= 0 {
			continue // missing process metadata is not a contradictory identity
		}
		if known != 0 && known != event.TGID {
			return true
		}
		known = event.TGID
	}
	return false
}

type binderInventorySchedulerAudit struct {
	failures []schedulerOrderViolation
	capped   bool
}

func (audit binderInventorySchedulerAudit) fails(idx *Index, q Query, pid int) bool {
	if audit.capped || schedulerRowIntegrityFailureForQuery(idx, q, pid) != nil {
		return true
	}
	for _, failure := range audit.failures {
		if schedulerOrderViolationRelevantToQuery(&failure, q, pid) {
			return true
		}
	}
	return false
}
