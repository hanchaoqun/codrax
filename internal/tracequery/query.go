package tracequery

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

func Run(idx *Index, q Query) Result {
	q = normalizeQuery(idx, q)
	res := Result{
		View:       q.View,
		SourcePath: idx.Path,
		LineCount:  idx.LineCount,
		EventCount: len(idx.Events),
		TimeStart:  q.TimeStart,
		TimeEnd:    q.TimeEnd,
	}
	switch q.View {
	case "thread_timeline":
		tl := ThreadTimeline(idx, q)
		res.Timeline = &tl
		res.EvidencePack = evidenceFromTimeline(tl)
	case "window_stats":
		stats := ComputeWindowStats(idx, q)
		res.WindowStats = &stats
		res.EvidencePack = evidenceFromStats(stats)
	case "wakeup_chain":
		chain := BuildWakeupChain(idx, q)
		res.WakeupChain = &chain
		if q.IncludeWindowStats {
			stats := ComputeWindowStats(idx, q)
			res.WindowStats = &stats
		}
		res.EvidencePack = evidenceFromChain(chain)
	case "evidence_pack":
		chain := BuildWakeupChain(idx, q)
		stats := ComputeWindowStats(idx, q)
		res.WakeupChain = &chain
		res.WindowStats = &stats
		res.EvidencePack = append(evidenceFromChain(chain), evidenceFromStats(stats)...)
	default:
		res.View = "event_search"
		res.Events = EventSearch(idx, q)
		res.EvidencePack = evidenceFromEvents(res.Events)
	}
	res.Caveats = append(res.Caveats, resultCaveats(idx, q, res)...)
	return res
}

func normalizeQuery(idx *Index, q Query) Query {
	q.View = strings.TrimSpace(q.View)
	if q.View == "" {
		q.View = "event_search"
	}
	if q.MaxDepth <= 0 {
		q.MaxDepth = 6
	}
	if q.MaxBranches <= 0 {
		q.MaxBranches = 8
	}
	if q.MinDurationMs <= 0 {
		q.MinDurationMs = 1
	}
	if q.Limit <= 0 {
		q.Limit = 40
	}
	if q.TimeStart == 0 && q.LineStart == 0 && idx != nil {
		q.TimeStart = idx.FirstTs
	}
	if q.TimeEnd == 0 && q.LineEnd == 0 && idx != nil {
		q.TimeEnd = idx.LastTs
	}
	if q.View == "wakeup_chain" && !q.IncludeWindowStats {
		q.IncludeWindowStats = true
	}
	return q
}

func EventSearch(idx *Index, q Query) []EventView {
	if idx == nil {
		return nil
	}
	typeSet := make(map[EventType]bool, len(q.EventTypes))
	for _, t := range q.EventTypes {
		if t != "" {
			typeSet[t] = true
		}
	}
	var events []Event
	for _, ev := range idx.Events {
		if !eventInQuery(ev, q, typeSet) {
			continue
		}
		events = append(events, ev)
		if len(events) >= q.Limit {
			break
		}
	}
	raw := loadRawLines(idx.Path, eventLines(events))
	out := make([]EventView, 0, len(events))
	for _, ev := range events {
		out = append(out, EventView{Event: ev, Raw: raw[ev.Line]})
	}
	return out
}

func eventInQuery(ev Event, q Query, typeSet map[EventType]bool) bool {
	if q.LineStart > 0 && ev.Line < q.LineStart {
		return false
	}
	if q.LineEnd > 0 && ev.Line > q.LineEnd {
		return false
	}
	if q.LineStart == 0 && q.LineEnd == 0 {
		if q.TimeStart > 0 && ev.Ts < q.TimeStart {
			return false
		}
		if q.TimeEnd > 0 && ev.Ts > q.TimeEnd {
			return false
		}
	}
	if len(typeSet) > 0 && !typeSet[ev.Type] {
		return false
	}
	if q.PID > 0 && !eventMentionsPID(ev, q.PID) {
		return false
	}
	if strings.TrimSpace(q.Thread) != "" && !eventMentionsThread(ev, q.Thread) {
		return false
	}
	return true
}

func eventMentionsPID(ev Event, pid int) bool {
	return ev.PID == pid || ev.TGID == pid || ev.PrevPID == pid || ev.NextPID == pid || ev.WakeePID == pid
}

func eventMentionsThread(ev Event, thread string) bool {
	thread = strings.ToLower(strings.TrimSpace(thread))
	if thread == "" {
		return true
	}
	for _, v := range []string{ev.Comm, ev.PrevComm, ev.NextComm, ev.WakeeComm, ev.SpanName, ev.FieldText} {
		if strings.Contains(strings.ToLower(v), thread) {
			return true
		}
	}
	return false
}

func ThreadTimeline(idx *Index, q Query) TimelineResult {
	target := resolveThread(idx, q)
	res := TimelineResult{
		Thread: target,
		Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
	}
	if target.PID == 0 && target.Comm == "" {
		res.Caveats = append(res.Caveats, "target thread not found; provide pid or a thread name visible in the trace")
		return res
	}
	var runningStart float64
	var runningLine int
	var offStart float64
	var offLine int
	var offState string
	var wake *Event
	for _, ev := range idx.Events {
		if q.LineStart > 0 || q.LineEnd > 0 {
			if q.LineStart > 0 && ev.Line < q.LineStart {
				continue
			}
			if q.LineEnd > 0 && ev.Line > q.LineEnd {
				continue
			}
		}
		switch ev.Type {
		case EventSchedWakeup, EventSchedWaking:
			if threadMatches(target, ev.WakeePID, ev.WakeeComm) && offStart > 0 && ev.Ts >= offStart {
				copy := ev
				wake = &copy
			}
		case EventSchedSwitch:
			if threadMatches(target, ev.NextPID, ev.NextComm) {
				if offStart > 0 {
					res.Intervals = append(res.Intervals, offCPUIntervals(target, offStart, ev.Ts, offLine, ev.Line, offState, wake)...)
				}
				offStart, offLine, offState, wake = 0, 0, "", nil
				runningStart = ev.Ts
				runningLine = ev.Line
			}
			if threadMatches(target, ev.PrevPID, ev.PrevComm) {
				if runningStart > 0 {
					res.Intervals = append(res.Intervals, makeInterval(target, StateRunning, runningStart, ev.Ts, runningLine, ev.Line, ""))
				}
				runningStart = 0
				offStart = ev.Ts
				offLine = ev.Line
				offState = ev.PrevState
				wake = nil
			}
		}
	}
	if runningStart > 0 {
		res.Intervals = append(res.Intervals, makeInterval(target, StateRunning, runningStart, q.TimeEnd, runningLine, 0, ""))
	}
	if offStart > 0 {
		res.Intervals = append(res.Intervals, offCPUIntervals(target, offStart, q.TimeEnd, offLine, 0, offState, wake)...)
	}
	res.Intervals = clampIntervals(res.Intervals, q)
	if len(res.Intervals) == 0 {
		res.Caveats = append(res.Caveats, "no scheduler interval for the target thread was found in the selected window")
	}
	return res
}

func offCPUIntervals(thread ThreadRef, start, end float64, startLine, endLine int, prevState string, wake *Event) []Interval {
	if end <= start {
		return nil
	}
	state := stateFromPrevState(prevState)
	if wake != nil && wake.Ts > start && wake.Ts < end && state != StateRunnable {
		return []Interval{
			makeInterval(thread, state, start, wake.Ts, startLine, wake.Line, prevState),
			makeIntervalWithWake(thread, StateRunnable, wake.Ts, end, wake.Line, endLine, prevState, wake.Line),
		}
	}
	return []Interval{makeInterval(thread, state, start, end, startLine, endLine, prevState)}
}

func makeInterval(thread ThreadRef, state ThreadState, start, end float64, startLine, endLine int, prevState string) Interval {
	return makeIntervalWithWake(thread, state, start, end, startLine, endLine, prevState, 0)
}

func makeIntervalWithWake(thread ThreadRef, state ThreadState, start, end float64, startLine, endLine int, prevState string, wakeLine int) Interval {
	if end < start {
		end = start
	}
	return Interval{
		Thread:       thread,
		State:        state,
		StartTs:      start,
		EndTs:        end,
		DurationMs:   (end - start) * 1000,
		StartLine:    startLine,
		EndLine:      endLine,
		WakeupLine:   wakeLine,
		PrevStateRaw: prevState,
		Summary:      fmt.Sprintf("%s for %.3f ms", state, (end-start)*1000),
	}
}

func stateFromPrevState(prev string) ThreadState {
	prev = strings.TrimSpace(prev)
	switch {
	case strings.HasPrefix(prev, "D"):
		return StateDSleep
	case strings.HasPrefix(prev, "S"):
		return StateSSleep
	case strings.HasPrefix(prev, "R"):
		return StateRunnable
	default:
		return StateUnknown
	}
}

func clampIntervals(in []Interval, q Query) []Interval {
	var out []Interval
	for _, it := range in {
		if q.TimeStart > 0 && it.EndTs < q.TimeStart {
			continue
		}
		if q.TimeEnd > 0 && it.StartTs > q.TimeEnd {
			continue
		}
		if q.TimeStart > 0 && it.StartTs < q.TimeStart {
			it.StartTs = q.TimeStart
		}
		if q.TimeEnd > 0 && it.EndTs > q.TimeEnd {
			it.EndTs = q.TimeEnd
		}
		it.DurationMs = (it.EndTs - it.StartTs) * 1000
		if it.DurationMs >= 0 {
			out = append(out, it)
		}
	}
	return out
}

func resolveThread(idx *Index, q Query) ThreadRef {
	if q.PID > 0 {
		name := ""
		tgid := 0
		for _, ev := range idx.Events {
			if ev.PID == q.PID {
				name, tgid = ev.Comm, ev.TGID
				break
			}
			if ev.PrevPID == q.PID {
				name = ev.PrevComm
				break
			}
			if ev.NextPID == q.PID {
				name = ev.NextComm
				break
			}
			if ev.WakeePID == q.PID {
				name = ev.WakeeComm
				break
			}
		}
		return ThreadRef{Comm: name, PID: q.PID, TGID: tgid}
	}
	needle := strings.ToLower(strings.TrimSpace(q.Thread))
	if needle == "" {
		return ThreadRef{}
	}
	for _, ev := range idx.Events {
		for _, candidate := range []struct {
			comm string
			pid  int
			tgid int
		}{{ev.Comm, ev.PID, ev.TGID}, {ev.PrevComm, ev.PrevPID, 0}, {ev.NextComm, ev.NextPID, 0}, {ev.WakeeComm, ev.WakeePID, 0}} {
			if candidate.pid > 0 && strings.Contains(strings.ToLower(candidate.comm), needle) {
				return ThreadRef{Comm: candidate.comm, PID: candidate.pid, TGID: candidate.tgid}
			}
		}
	}
	return ThreadRef{Comm: q.Thread}
}

func threadMatches(ref ThreadRef, pid int, comm string) bool {
	if ref.PID > 0 && pid == ref.PID {
		return true
	}
	if ref.Comm != "" && comm != "" && strings.EqualFold(ref.Comm, comm) {
		return true
	}
	return false
}

func ComputeWindowStats(idx *Index, q Query) WindowStats {
	stats := WindowStats{
		Window:      TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd},
		EventCounts: map[EventType]int{},
	}
	byCPU := map[int][]Event{}
	for _, ev := range idx.Events {
		if !timeInWindow(ev.Ts, q) {
			continue
		}
		stats.EventCounts[ev.Type]++
		switch ev.Type {
		case EventSchedSwitch:
			byCPU[ev.CPU] = append(byCPU[ev.CPU], ev)
		case EventBlockIssue:
			stats.BlockIssueCount++
		case EventBlockComplete:
			stats.BlockCompleteCount++
		case EventBinderTransaction:
			stats.BinderCount++
		case EventIRQ:
			stats.IRQCount++
		case EventMemory:
			stats.MemoryEventCount++
		case EventCPUFrequency:
			cpu := ev.CPUForField
			if cpu == 0 {
				cpu = ev.CPU
			}
			stats.CPU = upsertCPUFreq(stats.CPU, cpu, ev.Frequency)
		}
	}
	running := map[string]ThreadDuration{}
	for cpu, events := range byCPU {
		sort.SliceStable(events, func(i, j int) bool { return events[i].Ts < events[j].Ts })
		var busy, idle float64
		for i, ev := range events {
			end := q.TimeEnd
			if i+1 < len(events) {
				end = events[i+1].Ts
			}
			start := ev.Ts
			if q.TimeStart > 0 && start < q.TimeStart {
				start = q.TimeStart
			}
			if q.TimeEnd > 0 && end > q.TimeEnd {
				end = q.TimeEnd
			}
			if end <= start {
				continue
			}
			dur := (end - start) * 1000
			if ev.NextPID == 0 || strings.Contains(strings.ToLower(ev.NextComm), "idle") {
				idle += dur
			} else {
				busy += dur
				key := fmt.Sprintf("%d/%s", ev.NextPID, ev.NextComm)
				td := running[key]
				td.Thread = ThreadRef{Comm: ev.NextComm, PID: ev.NextPID}
				td.DurationMs += dur
				if td.LineStart == 0 {
					td.LineStart = ev.Line
				}
				td.LineEnd = ev.Line
				running[key] = td
			}
		}
		stats.CPU = upsertCPUBusyIdle(stats.CPU, cpu, busy, idle)
	}
	for _, td := range running {
		stats.TopRunning = append(stats.TopRunning, td)
	}
	sort.SliceStable(stats.TopRunning, func(i, j int) bool { return stats.TopRunning[i].DurationMs > stats.TopRunning[j].DurationMs })
	if len(stats.TopRunning) > 8 {
		stats.TopRunning = stats.TopRunning[:8]
	}
	return stats
}

func upsertCPUBusyIdle(in []CPUStats, cpu int, busy, idle float64) []CPUStats {
	for i := range in {
		if in[i].CPU == cpu {
			in[i].BusyMs += busy
			in[i].IdleMs += idle
			return in
		}
	}
	return append(in, CPUStats{CPU: cpu, BusyMs: busy, IdleMs: idle})
}

func upsertCPUFreq(in []CPUStats, cpu, freq int) []CPUStats {
	for i := range in {
		if in[i].CPU == cpu {
			if freq > 0 {
				in[i].Frequency = freq
			}
			return in
		}
	}
	return append(in, CPUStats{CPU: cpu, Frequency: freq})
}

func BuildWakeupChain(idx *Index, q Query) ChainResult {
	q = normalizeQuery(idx, q)
	target := resolveThread(idx, q)
	res := ChainResult{Target: target, Window: TimeWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd}}
	if target.PID == 0 && target.Comm == "" {
		res.Caveats = append(res.Caveats, "target thread not found")
		return res
	}
	tq := q
	tq.PID = target.PID
	tq.Thread = target.Comm
	targetTimeline := ThreadTimeline(idx, tq)
	branches := interestingIntervals(targetTimeline.Intervals, q.MinDurationMs, q.MaxBranches)
	if len(branches) == 0 {
		visited := map[int]bool{}
		expandChain(idx, q, target, q.TimeStart, q.TimeEnd, 0, visited, &res, "")
		return res
	}
	for _, branch := range branches {
		visited := map[int]bool{}
		expandChain(idx, q, target, branch.StartTs, branch.EndTs, 0, visited, &res, "")
	}
	return res
}

func expandChain(idx *Index, q Query, thread ThreadRef, start, end float64, depth int, visited map[int]bool, res *ChainResult, parentID string) string {
	if depth >= q.MaxDepth {
		res.Caveats = append(res.Caveats, fmt.Sprintf("max_depth=%d reached at pid=%d", q.MaxDepth, thread.PID))
		return ""
	}
	if thread.PID > 0 && visited[thread.PID] {
		res.Caveats = append(res.Caveats, fmt.Sprintf("cycle detected at pid=%d", thread.PID))
		return ""
	}
	if thread.PID > 0 {
		visited[thread.PID] = true
		defer delete(visited, thread.PID)
	}
	tq := q
	tq.PID = thread.PID
	tq.Thread = thread.Comm
	tq.TimeStart = start
	tq.TimeEnd = end
	tl := ThreadTimeline(idx, tq)
	interesting := mostInterestingInterval(tl.Intervals, q.MinDurationMs)
	nodeID := fmt.Sprintf("n%d", len(res.Nodes)+1)
	node := ChainNode{ID: nodeID, Thread: thread, Window: TimeWindow{StartTs: start, EndTs: end}}
	if interesting != nil {
		node.Dominant = interesting.State
		node.DurationMs = interesting.DurationMs
		node.EvidenceLine = firstPositive(interesting.StartLine, interesting.WakeupLine, interesting.EndLine)
		node.Summary = interesting.Summary
	} else {
		node.Dominant = StateUnknown
		node.Summary = "no decisive scheduler interval found in aligned window"
	}
	res.Nodes = append(res.Nodes, node)
	if parentID != "" {
		// Edge is added by the caller once it knows the wakeup row.
	}
	if interesting == nil {
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "trace_gap", Thread: thread, Summary: node.Summary, Confidence: 0.6})
		return nodeID
	}
	switch interesting.State {
	case StateSSleep:
		wakeup := findWakeupFor(idx, thread, interesting.StartTs, interesting.EndTs)
		if wakeup == nil {
			res.RootEvidence = append(res.RootEvidence, RootEvidence{
				Type:       "missing_wakeup",
				Thread:     thread,
				DurationMs: interesting.DurationMs,
				LineStart:  interesting.StartLine,
				LineEnd:    interesting.EndLine,
				Summary:    "sleep interval has no matching sched_wakeup row in the selected trace window",
				Confidence: 0.7,
			})
			return nodeID
		}
		waker := ThreadRef{Comm: wakeup.Comm, PID: wakeup.PID, TGID: wakeup.TGID}
		childID := expandChain(idx, q, waker, interesting.StartTs, wakeup.Ts, depth+1, visited, res, nodeID)
		if childID != "" {
			res.Edges = append(res.Edges, WakeupEdge{
				From:         childID,
				To:           nodeID,
				Waker:        waker,
				Wakee:        thread,
				WakeupTs:     wakeup.Ts,
				WakeupLine:   wakeup.Line,
				LatencyMs:    (wakeup.Ts - interesting.StartTs) * 1000,
				EvidenceLine: wakeup.Line,
			})
		}
	case StateRunnable:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "runnable_wait", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread was runnable but not running; inspect CPU pressure and priority context", Confidence: 0.8})
	case StateDSleep:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "d_state_or_io_wait", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread slept in D state; IO or uninterruptible wait is a root-cause candidate", Confidence: 0.85})
	case StateRunning:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "running", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread was running in the aligned window; its own CPU work is root-cause evidence", Confidence: 0.75})
	default:
		res.RootEvidence = append(res.RootEvidence, RootEvidence{Type: "unknown_state", Thread: thread, DurationMs: interesting.DurationMs, LineStart: interesting.StartLine, LineEnd: interesting.EndLine, Summary: "thread state could not be classified from scheduler rows", Confidence: 0.5})
	}
	return nodeID
}

func mostInterestingInterval(intervals []Interval, minDurationMs float64) *Interval {
	candidates := interestingIntervals(intervals, minDurationMs, 1)
	if len(candidates) == 0 {
		return nil
	}
	return &candidates[0]
}

func interestingIntervals(intervals []Interval, minDurationMs float64, max int) []Interval {
	if max <= 0 {
		max = 1
	}
	var best *Interval
	score := func(s ThreadState) int {
		switch s {
		case StateDSleep:
			return 5
		case StateSSleep:
			return 4
		case StateRunnable:
			return 3
		case StateRunning:
			return 2
		default:
			return 1
		}
	}
	var out []Interval
	for i := range intervals {
		if intervals[i].DurationMs < minDurationMs {
			continue
		}
		if intervals[i].State == StateRunning {
			continue
		}
		out = append(out, intervals[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		si, sj := score(out[i].State), score(out[j].State)
		if si != sj {
			return si > sj
		}
		return out[i].DurationMs > out[j].DurationMs
	})
	if len(out) > max {
		out = out[:max]
	}
	if len(out) == 0 {
		for i := range intervals {
			if intervals[i].DurationMs >= minDurationMs {
				if best == nil || intervals[i].DurationMs > best.DurationMs {
					best = &intervals[i]
				}
			}
		}
		if best != nil {
			out = append(out, *best)
		}
	}
	return out
}

func findWakeupFor(idx *Index, thread ThreadRef, start, end float64) *Event {
	var best *Event
	for i := range idx.Events {
		ev := &idx.Events[i]
		if ev.Type != EventSchedWakeup && ev.Type != EventSchedWaking {
			continue
		}
		if ev.Ts < start || ev.Ts > end {
			continue
		}
		if threadMatches(thread, ev.WakeePID, ev.WakeeComm) {
			best = ev
		}
	}
	return best
}

func timeInWindow(ts float64, q Query) bool {
	if q.TimeStart > 0 && ts < q.TimeStart {
		return false
	}
	if q.TimeEnd > 0 && ts > q.TimeEnd {
		return false
	}
	return true
}

func eventLines(events []Event) []int {
	out := make([]int, 0, len(events))
	for _, ev := range events {
		if ev.Line > 0 {
			out = append(out, ev.Line)
		}
	}
	return out
}

func loadRawLines(path string, lines []int) map[int]string {
	out := map[int]string{}
	if len(lines) == 0 {
		return out
	}
	want := map[int]bool{}
	for _, n := range lines {
		want[n] = true
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufioNewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if want[lineNo] {
			out[lineNo] = sc.Text()
			if len(out) == len(want) {
				break
			}
		}
	}
	return out
}

func bufioNewScanner(f *os.File) *bufio.Scanner {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return sc
}

func evidenceFromTimeline(tl TimelineResult) []EvidenceFact {
	var out []EvidenceFact
	for _, it := range tl.Intervals {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(it.Thread),
			Predicate:  string(it.State),
			Summary:    it.Summary,
			LineStart:  it.StartLine,
			LineEnd:    it.EndLine,
			StartTs:    it.StartTs,
			EndTs:      it.EndTs,
			Confidence: 0.75,
		})
		if len(out) >= 12 {
			break
		}
	}
	return out
}

func evidenceFromStats(stats WindowStats) []EvidenceFact {
	var out []EvidenceFact
	for _, td := range stats.TopRunning {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(td.Thread),
			Predicate:  "running_time",
			Summary:    fmt.Sprintf("%s ran for %.3f ms in the selected window", threadLabel(td.Thread), td.DurationMs),
			LineStart:  td.LineStart,
			LineEnd:    td.LineEnd,
			Confidence: 0.7,
		})
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func evidenceFromChain(chain ChainResult) []EvidenceFact {
	var out []EvidenceFact
	for _, edge := range chain.Edges {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(edge.Waker),
			Predicate:  "wakes",
			Object:     threadLabel(edge.Wakee),
			Summary:    fmt.Sprintf("%s wakes %s at %.6f", threadLabel(edge.Waker), threadLabel(edge.Wakee), edge.WakeupTs),
			LineStart:  edge.WakeupLine,
			LineEnd:    edge.WakeupLine,
			StartTs:    edge.WakeupTs,
			EndTs:      edge.WakeupTs,
			Confidence: 0.85,
		})
	}
	for _, root := range chain.RootEvidence {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(root.Thread),
			Predicate:  root.Type,
			Summary:    root.Summary,
			LineStart:  root.LineStart,
			LineEnd:    root.LineEnd,
			Confidence: root.Confidence,
		})
	}
	return out
}

func evidenceFromEvents(events []EventView) []EvidenceFact {
	var out []EvidenceFact
	for _, ev := range events {
		out = append(out, EvidenceFact{
			Subject:    threadLabel(ThreadRef{Comm: ev.Comm, PID: ev.PID, TGID: ev.TGID}),
			Predicate:  string(ev.Type),
			Summary:    strings.TrimSpace(ev.Raw),
			LineStart:  ev.Line,
			LineEnd:    ev.Line,
			StartTs:    ev.Ts,
			EndTs:      ev.Ts,
			Confidence: 0.7,
		})
	}
	return out
}

func resultCaveats(idx *Index, q Query, res Result) []string {
	var out []string
	if idx != nil && idx.ParsedKnown == 0 {
		out = append(out, "no known ftrace scheduler/resource events were parsed; the file may need a future parser adapter")
	}
	if q.LineStart > 0 || q.LineEnd > 0 {
		out = append(out, "line-window filtering was used; time-window statistics only cover parsed rows inside that line window")
	}
	return out
}

func threadLabel(t ThreadRef) string {
	switch {
	case t.Comm != "" && t.PID > 0:
		return fmt.Sprintf("%s-%d", t.Comm, t.PID)
	case t.Comm != "":
		return t.Comm
	case t.PID > 0:
		return fmt.Sprintf("pid=%d", t.PID)
	default:
		return "unknown-thread"
	}
}

func firstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
