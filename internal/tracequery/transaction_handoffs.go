package tracequery

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var transactionPair = regexp.MustCompile(`\[\s*([0-9]+)\s*,\s*([0-9]+)\s*\]`)
var transactionSubmit = regexp.MustCompile(`(?:^|[ ,])transactionFlag:\s*(\[\s*[0-9]+\s*,\s*[0-9]+\s*\])(?:$|[, ]|\t)`)

type transactionKey struct {
	tid int
	seq string
}

// Protocol interpretation is deliberately separate from question routing.
// The exact marker family and structured integer pair, not nearby names,
// establish a candidate. Sequence text stays exact above JSON float precision.
func transactionProtocol(ev Event) (string, []transactionKey, bool) {
	if ev.Type != EventTraceMark || (ev.SpanAction != "B" && ev.SpanAction != "I" && ev.SpanAction != "N") {
		return "", nil, false
	}
	name := ev.SpanName
	const submit = "H:MarshRSTransactionData"
	const consume = "H:RSMainThread::ProcessCommandUni"
	role, rest := "", ""
	if name == submit || strings.HasPrefix(name, submit+" ") {
		role, rest = "submission", strings.TrimPrefix(name, submit)
		matches := transactionSubmit.FindAllStringSubmatch(rest, -1)
		if len(matches) != 1 {
			return role, nil, true
		}
		rest = matches[0][1]
	} else if name == consume || strings.HasPrefix(name, consume+" ") {
		role, rest = "consumption", strings.TrimSpace(strings.TrimPrefix(name, consume))
	} else {
		return "", nil, false
	}
	matches := transactionPair.FindAllStringSubmatch(rest, -1)
	if len(matches) == 0 || strings.TrimSpace(transactionPair.ReplaceAllString(rest, "")) != "" {
		return role, nil, true
	}
	keys := make([]transactionKey, 0, len(matches))
	seen := map[transactionKey]bool{}
	for _, m := range matches {
		tid, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil || tid <= 0 {
			return role, nil, true
		}
		seq, err := strconv.ParseUint(m[2], 10, 64)
		if err != nil {
			return role, nil, true
		}
		key := transactionKey{int(tid), strconv.FormatUint(seq, 10)}
		// Duplicate mentions in one consumption are one observed event, not
		// several competing endpoints or proof of repeated execution.
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return role, keys, false
}

func buildTransactionHandoffs(idx *Index, q Query) *TransactionHandoffsResult {
	p := &TransactionHandoffsResult{Status: "available", SourcePath: idx.Path, Window: RenderingCandidatesWindow{q.TimeStart, q.TimeEnd, q.timeEndBackfilled}, TargetPID: q.PID, TargetThread: q.Thread, TargetScope: q.TargetScope, Caveats: []string{TransactionHandoffsTeaching}}
	fail := func(reason string) *TransactionHandoffsResult { p.Status, p.Reason = "unavailable", reason; return p }
	if idx.Windowed || idx.RelationScoped || !resourceStackSingleSource(idx) {
		return fail("requires_complete_identity_mapped_single_source")
	}
	if q.LineStart != 0 || q.LineEnd != 0 || q.SpanName != "" || q.Pattern != "" || len(q.Patterns) > 0 || len(q.EventTypes) > 0 || len(q.EventNames) > 0 || len(q.TraceMarkActions) > 0 || len(q.EventFieldFilters) > 0 {
		return fail("transaction_handoffs_does_not_accept_event_name_or_line_filters")
	}
	if !isFiniteTraceNumber(q.TimeStart) || !isFiniteTraceNumber(q.TimeEnd) || q.TimeEnd < q.TimeStart {
		return fail("invalid_query_window")
	}
	inWindow := func(ts float64) bool {
		return ts >= q.TimeStart && (ts < q.TimeEnd || ts == q.TimeEnd && q.timeEndBackfilled)
	}
	type acc struct{ s, c []TransactionEndpoint }
	groups := map[transactionKey]*acc{}
	keyReferences := 0
	owners := map[int]map[int]bool{}
	for _, ev := range idx.Events {
		if q.runCancel.tick() {
			return nil
		}
		role, keys, malformed := transactionProtocol(ev)
		if role == "" {
			continue
		}
		if malformed {
			if inWindow(ev.Ts) {
				p.MalformedProtocolEvents++
			}
			continue
		}
		if len(keys) > 65536-keyReferences {
			return fail("complete_source_retention_limit")
		}
		keyReferences += len(keys)
		co := ProjectTraceEventInventoryCoordinates(ev)
		sourcePath, sourceLine := idx.Path, ev.Line
		if len(idx.TraceArtifacts) > 0 {
			spans := idx.ResolveArtifactSpans(ev.Line, ev.Line)
			if len(spans) != 1 {
				return fail("endpoint_source_identity_unresolved")
			}
			sourcePath, sourceLine = spans[0].SourcePath, spans[0].LocalLineStart
		}
		e := TransactionEndpoint{SourcePath: sourcePath, SourceLine: sourceLine, Line: ev.Line, Ts: ev.Ts, TID: co.EmitterTID, TGID: co.EmitterTGID, Thread: ev.Comm, Name: ev.SpanName, InWindow: inWindow(ev.Ts)}
		if owners[e.TID] == nil {
			owners[e.TID] = map[int]bool{}
		}
		owners[e.TID][e.TGID] = true
		for _, key := range keys {
			a := groups[key]
			if a == nil {
				a = &acc{}
				groups[key] = a
			}
			if role == "submission" {
				a.s = append(a.s, e)
			} else {
				a.c = append(a.c, e)
			}
		}
	}
	keys := make([]transactionKey, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].tid != keys[j].tid {
			return keys[i].tid < keys[j].tid
		}
		return keys[i].seq < keys[j].seq
	})
	selected := func(e TransactionEndpoint) bool {
		if !e.InWindow {
			return false
		}
		if q.PID > 0 {
			if q.TargetScope == TargetScopeProcess {
				return e.TGID == q.PID
			}
			return e.TID == q.PID
		}
		return q.Thread == "" || q.Thread == e.Thread
	}
	windowS, windowC := map[int]bool{}, map[int]bool{}
	for _, key := range keys {
		a := groups[key]
		chosen := false
		for _, es := range [][]TransactionEndpoint{a.s, a.c} {
			for _, e := range es {
				chosen = chosen || selected(e)
			}
		}
		if !chosen {
			continue
		}
		h := TransactionHandoff{TID: key.tid, Sequence: key.seq, SubmissionCount: len(a.s), ConsumptionCount: len(a.c)}
		for _, e := range a.s {
			if e.InWindow {
				h.WindowSubmissions++
				windowS[e.Line] = true
			}
		}
		for _, e := range a.c {
			if e.InWindow {
				h.WindowConsumptions++
				windowC[e.Line] = true
			}
		}
		switch {
		case len(a.s) > 1 || len(a.c) > 1:
			h.Status, h.Reason = "ambiguous", "reused_key_or_multiple_endpoints_in_complete_source"
		case len(a.s) == 0:
			h.Status = "missing_submission"
		case len(a.c) == 0:
			h.Status = "missing_consumption"
		default:
			s, c := a.s[0], a.c[0]
			h.Status = "observed_unique_protocol_match"
			if s.TID != key.tid || s.TGID <= 0 || c.TID <= 0 || c.TGID <= 0 || len(owners[s.TID]) != 1 || len(owners[c.TID]) != 1 {
				h.Status, h.Reason = "identity_unverified", "source_emitter_or_process_identity_not_unique"
			} else if c.Ts < s.Ts || c.Ts == s.Ts && c.Line <= s.Line {
				h.Status, h.Reason = "order_unverified", "consumption_does_not_follow_submission"
			} else {
				interval := Query{TimeStart: s.Ts, TimeEnd: c.Ts, TimeStartSet: true, TimeEndSet: true}
				if conflict := threadIncarnationConflictForPIDSet(idx, interval, map[int]bool{s.TID: true, c.TID: true}); conflict != nil {
					h.Status, h.Reason = "identity_unverified", conflict.reason()
				}
			}
		}
		h.Submissions = append([]TransactionEndpoint(nil), a.s[:min(len(a.s), TransactionEndpointExamplesLimit)]...)
		h.Consumptions = append([]TransactionEndpoint(nil), a.c[:min(len(a.c), TransactionEndpointExamplesLimit)]...)
		h.OmittedSubmissions = len(a.s) - len(h.Submissions)
		h.OmittedConsumptions = len(a.c) - len(h.Consumptions)
		p.TotalKeys++
		if len(p.Handoffs) < ViewCapacityFor(ViewTransactionHandoffs).ClampLimit(q.Limit) {
			p.Handoffs = append(p.Handoffs, h)
		} else {
			p.OmittedKeys++
		}
	}
	p.WindowSubmissionEvents, p.WindowConsumptionEvents = len(windowS), len(windowC)
	p.Caveats = append(p.Caveats, fmt.Sprintf("Complete-source key multiplicity precedes window/target/display selection. Counts describe each endpoint's own window; one consumption containing several keys is counted once. Physical source lines identify endpoints. Keys=%d, displayed=%d.", p.TotalKeys, len(p.Handoffs)))
	return p
}
