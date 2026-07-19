package tracequery

import "strings"

// target_window_state_account.go — §29.27 ruling ② (COV-4, ledger
// docs/design/real_trace_campaign_20260705.md, user ruling 2026-07-11): the
// causal-projection coverage account expands from a pure-wait denominator to
// the FOCUSED THREAD's full-window four-state wall-clock partition. The five
// ENGINE lanes (running / runnable / sleep / d_state / io_wait) are the exact
// ThreadTimeline decomposition and DO sum (TotalMs = Σ five lanes): io_wait
// here is the D-opened interval carve-out the timeline itself reclassifies
// (D+iowait blocked_reason), i.e. a partition member. The DISPLAY folds
// io_wait back into its D-state term and shows it as the 「其中 IO等待」
// attribution label, so the USER-FACING account has exactly four addends
// (复核 B-2: the additive/label boundary sits between engine partition and
// display folding — this comment states which is which). SleepIOWaitMs, by
// contrast, is a pure REFINEMENT overlay (复核 A-1, G12 §29.13 platform
// semantics: Harmony IO waits sleep in S and the kernel emits a separate
// iowait=1 blocked_reason marker — the interval STAYS an S sleep and never
// reclassifies): its wall clock is already inside SleepMs, it never adds to
// TotalMs, and it feeds only the sleep term's 「其中 IO等待」 label. The
// display renders the account ONLY when the partition balances the analysis
// window at display precision (不平衡拒渲不造数, the engine never fabricates
// a residual lane for unobserved prefixes/stopped/dead time).
//
// Discipline:
//   - every lane is the exact ThreadTimeline decomposition already used by
//     the causal skeleton's target_state layer (one timeline scan per bundle,
//     shared — PERF #21 lesson: no second event rescan);
//   - DeterministicRunningMs is union(target-thread semantic-span member
//     intervals ∩ target running intervals) through the SAME foldInterval
//     algebra the §29.26③-1 on-chain semantic intersection uses (交集机械
//     复用) — wall clock, never a converted value;
//   - the span population is the bundle's own stats.TraceSpans (the same
//     population the semantic rank lanes consume), so the account can never
//     claim deterministic work the report has no evidence row for;
//   - SleepIOWaitMs pairs S-opened intervals with iowait>0 blocked_reason
//     markers through the SAME finder the D-state enrich consumes
//     (findBlockedReasonForWithSelection) — G12 single attribution untouched:
//     DStateTop/IOWaitTop and every duration lane book exactly what they
//     booked before (S never reclassifies, no additive lane grows).

// TargetWindowStateAccount is the focused thread's full-window scheduling
// state partition over the bundle's analysis window.
type TargetWindowStateAccount struct {
	Thread   ThreadRef  `json:"thread"`
	Window   TimeWindow `json:"window"`
	WindowMs float64    `json:"window_ms"`
	// The five active-state lanes (ms, window-clamped). TotalMs = Σ(five
	// lanes); it equals WindowMs only when the timeline covered the whole
	// window (complete head carry-in, no unobserved/stopped/dead gaps).
	RunningMs  float64 `json:"running_ms,omitempty"`
	RunnableMs float64 `json:"runnable_ms,omitempty"`
	SleepMs    float64 `json:"sleep_ms,omitempty"`
	DStateMs   float64 `json:"d_state_ms,omitempty"`
	IOWaitMs   float64 `json:"io_wait_ms,omitempty"`
	TotalMs    float64 `json:"total_ms,omitempty"`
	// SleepIOWaitMs is the 复核 A-1 REFINEMENT overlay: the wall clock of the
	// S-opened sleep intervals whose wakeup paired an iowait>0
	// sched_blocked_reason marker (G12 §29.13 Harmony platform IO-wait form).
	// Already contained in SleepMs — NEVER an addend of TotalMs; display
	// consumes it as the sleep term's 「其中 IO等待」 label only.
	SleepIOWaitMs float64 `json:"sleep_io_wait_ms,omitempty"`
	// DeterministicRunningMs is the wall-clock union of the focused thread's
	// OWN semantic-span (deterministic-optimization class) intervals
	// intersected with its running intervals, inside the window.
	DeterministicRunningMs float64 `json:"deterministic_running_ms,omitempty"`
	// HeadCarryMs / HeadCarryState (§29.140 G6, ANSWERFACE-1 件2): the
	// window-head prefix segment carried from the RECOVERED pre-window
	// scheduler state (TimelineHeadState.Status=="recovered") — the span
	// from the window start to the first in-window scheduler event of the
	// focused thread has no in-window event coverage of its own; its state
	// is sound carry-in reconstruction, and its wall clock is ALREADY inside
	// the lane named by HeadCarryState (running/runnable/sleep/d_state/
	// io_wait). Disclosure only, never an addend (禁静默折入 — the display
	// annotates the receiving term instead of silently folding).
	HeadCarryMs    float64 `json:"head_carry_ms,omitempty"`
	HeadCarryState string  `json:"head_carry_state,omitempty"`
	// TailOpenMs / TailOpenState: same disclosure family for the window-tail
	// suffix flushed from the final still-open interval (EndLine==0 — the
	// state was proven by its opening event but no in-window closing event
	// bounds it; the timeline extends it to the window end). Wall clock
	// already inside the lane named by TailOpenState.
	TailOpenMs    float64 `json:"tail_open_ms,omitempty"`
	TailOpenState string  `json:"tail_open_state,omitempty"`
	FragmentCount int     `json:"fragment_count,omitempty"`
	LineStart     int     `json:"line_start,omitempty"`
	LineEnd       int     `json:"line_end,omitempty"`
}

// windowStateAccountLane maps a timeline interval state to the account's
// published lane token (the JSON field family running/runnable/sleep/
// d_state/io_wait). stopped/dead/unknown own no lane (§7.11 B-1 ruling) and
// return "" — a boundary segment in those states is never disclosed as a
// lane fold because the account never booked it.
func windowStateAccountLane(state ThreadState) string {
	switch state {
	case StateRunning:
		return "running"
	case StateRunnable:
		return "runnable"
	case StateSSleep:
		return "sleep"
	case StateDSleep:
		return "d_state"
	case StateIOWait:
		return "io_wait"
	default:
		return ""
	}
}

// stampWindowStateBoundaryFolds publishes the account's window-boundary
// extrapolated components (§29.140 G6 typed disclosure). Both detections are
// PRECISE signals: the head arm fires only on the typed recovered head-state
// carry (HeadState.Status=="recovered" and the earliest interval opening at
// the window edge), the tail arm only on the final flush interval's typed
// EndLine==0 (no in-window closing event). A single interval covering the
// whole window counts once (head arm wins; the tail arm skips the same
// interval) so the two disclosures never double-book one span.
func stampWindowStateBoundaryFolds(account *TargetWindowStateAccount, tl TimelineResult) {
	if account == nil || len(tl.Intervals) == 0 {
		return
	}
	const edgeTol = 1e-9
	headIdx := -1
	if tl.HeadState != nil && tl.HeadState.Status == "recovered" {
		earliest := -1
		for i, it := range tl.Intervals {
			if earliest < 0 || it.StartTs < tl.Intervals[earliest].StartTs {
				earliest = i
			}
		}
		if earliest >= 0 {
			first := tl.Intervals[earliest]
			if first.StartTs <= account.Window.StartTs+edgeTol && first.DurationMs > 0 {
				if lane := windowStateAccountLane(first.State); lane != "" {
					account.HeadCarryMs = first.DurationMs
					account.HeadCarryState = lane
					headIdx = earliest
				}
			}
		}
	}
	latest := -1
	for i, it := range tl.Intervals {
		if it.EndLine != 0 || it.DurationMs <= 0 {
			continue
		}
		if latest < 0 || it.EndTs > tl.Intervals[latest].EndTs {
			latest = i
		}
	}
	if latest >= 0 && latest != headIdx {
		last := tl.Intervals[latest]
		if last.EndTs >= account.Window.EndTs-edgeTol {
			if lane := windowStateAccountLane(last.State); lane != "" {
				account.TailOpenMs = last.DurationMs
				account.TailOpenState = lane
			}
		}
	}
}

// targetWindowTimeline runs the focused thread's timeline decomposition over
// the analysis window — the ONE scan shared by the causal skeleton's
// target_state layer and the §29.27② state account.
func targetWindowTimeline(idx *Index, q Query, target ThreadRef, window TimeWindow) (TimelineResult, bool) {
	if idx == nil || (target.PID <= 0 && strings.TrimSpace(target.Comm) == "") {
		return TimelineResult{}, false
	}
	if window.EndTs <= window.StartTs {
		return TimelineResult{}, false
	}
	tq := q
	tq.View = ""
	tq.PID = target.PID
	tq.Thread = target.Comm
	tq.ThreadInput = ""
	tq.TimeStart = window.StartTs
	tq.TimeEnd = window.EndTs
	return ThreadTimeline(idx, tq), true
}

// buildTargetWindowStateAccount assembles the typed account from the shared
// timeline scan. nil when the timeline has no measurable intervals (the
// account is honestly absent — absence never fabricates zeros).
func buildTargetWindowStateAccount(idx *Index, tl TimelineResult, ok bool, target ThreadRef, window TimeWindow, stats *WindowStats) *TargetWindowStateAccount {
	if !ok {
		return nil
	}
	bd := summarizeThreadStateBreakdown(tl)
	if bd == nil {
		return nil
	}
	account := &TargetWindowStateAccount{
		Thread:        bd.Thread,
		Window:        window,
		WindowMs:      (window.EndTs - window.StartTs) * 1000,
		RunningMs:     bd.RunningMs,
		RunnableMs:    bd.RunnableMs,
		SleepMs:       bd.SleepMs,
		DStateMs:      bd.DStateMs,
		IOWaitMs:      bd.IOWaitMs,
		TotalMs:       bd.TotalMs,
		FragmentCount: bd.FragmentCount,
		LineStart:     bd.LineStart,
		LineEnd:       bd.LineEnd,
	}
	var running []foldInterval
	for _, it := range tl.Intervals {
		if it.State == StateRunning && it.EndTs > it.StartTs {
			running = append(running, foldInterval{start: it.StartTs, end: it.EndTs})
		}
		// 复核 A-1 (G12 §29.13 platform form): an S-opened sleep whose wakeup
		// pairs an iowait>0 blocked_reason marker is the Harmony IO wait —
		// book its wall clock into the SleepIOWaitMs refinement overlay. The
		// interval itself stays an S sleep (single attribution: SleepMs and
		// every published duration lane are untouched); the SAME finder the
		// D-state enrich consumes decides the pairing, so the two state
		// families can never disagree on what an IO wait is.
		// The shared blocked-reason matcher owns the closing-boundary tolerance;
		// this caller passes the physical interval unchanged so the allowance is
		// exactly 5µs rather than accidentally doubled.
		if idx != nil && it.State == StateSSleep && it.DurationMs > 0 {
			if reason := matchBlockedReasonForWithSelectionAtClosure(idx, Query{TimeStart: window.StartTs, TimeEnd: window.EndTs}, target, it.StartTs, it.EndTs, nil, false, it.EndLine > 0).Event; reason != nil && reason.IOWait > 0 {
				account.SleepIOWaitMs += it.DurationMs
			}
		}
	}
	account.DeterministicRunningMs = targetSemanticRunningMs(stats, target, window, running)
	stampWindowStateBoundaryFolds(account, tl)
	return account
}

// targetSemanticRunningMs computes union(target semantic-span member
// intervals) ∩ union(target running intervals) in ms — the 确定性工作 lane of
// the running partition. Same pairwise-intersection-then-union algebra as
// semanticTraceSpanChainIntersectionProjection (§29.26③-1 交集机械).
func targetSemanticRunningMs(stats *WindowStats, target ThreadRef, window TimeWindow, running []foldInterval) float64 {
	if stats == nil || len(running) == 0 {
		return 0
	}
	var members []foldInterval
	for _, span := range stats.TraceSpans {
		if !sameThreadRef(span.Thread, target) {
			continue
		}
		if strings.TrimSpace(span.SemanticClass) == "" && traceSpanSemanticClass(span.Name) == "" {
			continue
		}
		start, end, ok := overlapTimeWindow(span.StartTs, span.EndTs, window.StartTs, window.EndTs)
		if !ok {
			continue
		}
		members = append(members, foldInterval{start: start, end: end})
	}
	if len(members) == 0 {
		return 0
	}
	mergedMembers, _ := foldIntervalUnionWithDisjoint(members)
	mergedRunning, _ := foldIntervalUnionWithDisjoint(running)
	var intersections []foldInterval
	for _, member := range mergedMembers {
		for _, run := range mergedRunning {
			start := maxFloat(member.start, run.start)
			end := minFloat(member.end, run.end)
			if end > start {
				intersections = append(intersections, foldInterval{start: start, end: end})
			}
		}
	}
	ms, _ := foldIntervalUnionMs(intersections)
	return ms
}
