package tool

// trace_query_supplement.go — SUPP-CORE (DISPATCH-IND 批1, 2026-07-14): the
// post-explore deterministic trace_query supplement.
//
// Disease (§29.55.5 件B root sentence): report quality depended on WHICH
// trace_query views the model happened to dispatch — missing record families
// meant whole typed answer faces (anchored tree / self segment / rank seats /
// four-state account / censuses / ◎ overview) structurally failed to mint on
// otherwise-healthy runs (h2 20260713-223753 witness). The fix decouples core
// family PRESENCE from model sampling: after the model's investigation
// completion is ACCEPTED, the system detects missing core families on the
// SAME compiled ledger the renderer consumes (boolean precise signals),
// derives fully TYPED parameters (exactly-one runtime target; tolerance-equal
// model-call windows — F1 precedent: never last-wins, never prose), and
// re-runs at most TWO engine views through the EXISTING tool runner
// ((&TraceQuery{}).Execute — single value source, byte-identical caveat/
// notes/census emission to a model call; no simplified side-cast engine).
//
// Completion-gate ownership (§29.60): this is a render-assembly-side system
// action — it never re-opens model collection, never requeues, never resets
// the accepted completion, burns zero model rounds.
//
// Lane discipline: results enter ONLY the dedicated MutableState slot →
// ObservationLedgerInput.SystemTraceSupplementResults (never bus.ToolResults,
// never the dispatch buffer), so the explore transcript stays byte-identical
// and provenance stays structural (every compiled record is stamped
// SystemSupplement=true). Fail-open: ANY missing typed signal ⇒ skip with
// byte-identical output. Triggered runs disclose on the answer-side caveat
// lane (single total line, R5) and on the operator log (performance
// disclosure).

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// traceSupplement* are the codrax.yaml budget knobs (trace_supplement_*
// prefix group), injected one-shot from cmd/root.go like the other
// tool-layer knobs. Enabled defaults ON (explicit
// `trace_supplement_enabled: false` is the kill switch, write_enabled
// style). MaxColdBytes bounds the COLD lane only: when the run has zero
// successful trace_query results (no warm engine index cache), a supplement
// against a trace file larger than the budget is skipped instead of paying a
// full cold parse at render-assembly time.
//
// P1 运营缺口修 (2026-07-14): the warm lane needs its own fuses — rank/stats
// view cost scales with in-window events, so a user-specified huge window
// could stall render assembly unboundedly (wall time AND accumulator
// memory). Two precise gates, both fail-open family (never silently truncate
// the window, never guess a smaller one):
//   - MaxWindowSpanS gates the DERIVED window's span BEFORE any engine call:
//     an over-budget span skips the whole supplement with an HONEST
//     answer-side disclosure (skip reason=window_span_exceeded).
//   - MaxDuration is the BETWEEN-VIEW deadline: after each completed view,
//     an over-deadline supplement skips the remaining views (skip
//     reason=duration_budget_exceeded) while KEEPING every completed view's
//     observations — partial results are already-recorded deterministic
//     facts and are never dropped mid-flight. In-view cancellation needs
//     engine context cooperation and is deliberately out of scope (filed
//     separately; candidate = threading TraceQuery.Execute's bus context
//     into tracequery.Run).
var (
	traceSupplementEnabled            = true
	traceSupplementMaxColdBytes int64 = 2 << 30
	// Duration/span defaults per user ruling (修复轮, 2026-07-14): 20s
	// between-view deadline, 120s window-span budget.
	traceSupplementMaxDuration    = 20 * time.Second
	traceSupplementMaxWindowSpanS = 120.0
)

// SetTraceQuerySupplementConfig is the one-shot config injection point
// (cmd/root.go). Non-positive values keep the code defaults.
func SetTraceQuerySupplementConfig(enabled bool, maxColdBytes int64, maxDuration time.Duration, maxWindowSpanS float64) {
	traceSupplementEnabled = enabled
	if maxColdBytes > 0 {
		traceSupplementMaxColdBytes = maxColdBytes
	}
	if maxDuration > 0 {
		traceSupplementMaxDuration = maxDuration
	}
	if maxWindowSpanS > 0 {
		traceSupplementMaxWindowSpanS = maxWindowSpanS
	}
}

// TraceQuerySupplementOutcome reports one supplement attempt for the
// orchestrator hook's log line. Attempted=false means the per-task latch was
// already consumed (or no mutable state exists) and nothing happened.
type TraceQuerySupplementOutcome struct {
	Attempted bool
	// Executed lists the canonical views the supplement ran, in order.
	Executed []string
	// SkipReason is the typed fail-open reason when Executed is empty
	// ("families_present" is the healthy no-op).
	SkipReason string
	// Elapsed is the total engine wall time of the executed views.
	Elapsed time.Duration
}

// traceSupplementFamilyPresence is the boolean core-family detector output —
// presence of each core record family on the compiled ledger (typed
// predicates / ClaimKey prefixes only; no ranker scores, no similarity).
type traceSupplementFamilyPresence struct {
	Rank                bool // root_cause_* ClaimKey prefix (rank seats / partition account / SELF-* seats)
	Chain               bool // wakeup_chain / wakeup_chain_edge predicates (anchored tree / drilldown)
	WindowStates        bool // target_window_states predicate (four-state account / supply faces)
	Critical            bool // critical_blocking predicate
	BlockedReasonCensus bool // blocked_reason_census predicate (wait-object faces)
	WakeupEdgeCensus    bool // wakeup_edge_census predicate (waker faces)
}

func traceSupplementFamilies(ledger types.ObservationLedger) traceSupplementFamilyPresence {
	var f traceSupplementFamilyPresence
	for _, record := range ledger.Records {
		if strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_") {
			f.Rank = true
		}
		switch strings.TrimSpace(record.Predicate) {
		case "wakeup_chain", "wakeup_chain_edge":
			f.Chain = true
		case "target_window_states":
			f.WindowStates = true
		case "critical_blocking":
			f.Critical = true
		case "blocked_reason_census":
			f.BlockedReasonCensus = true
		case "wakeup_edge_census":
			f.WakeupEdgeCensus = true
		}
	}
	return f
}

// traceSupplementViews maps missing families to the minimal engine view set.
// One root_cause_rank call fills FOUR result fields (WakeupChain +
// WindowStats + SchedulerLatency + RootCauseRank — tracequery/query.go view
// dispatch), which mint the rank/chain/window-states families plus both
// censuses; critical_blocking_calls fills CriticalBlocking. ≤2 executions.
func traceSupplementViews(f traceSupplementFamilyPresence) []string {
	var views []string
	if !f.Rank || !f.Chain || !f.WindowStates || !f.BlockedReasonCensus || !f.WakeupEdgeCensus {
		views = append(views, "root_cause_rank")
	}
	if !f.Critical {
		views = append(views, "critical_blocking_calls")
	}
	return views
}

// traceSupplementTarget derives the supplement's typed target. Priority (R2
// ruling: user intent first):
//
//  1. USER lane — request-model RuntimeTargets excluding the exploration
//     cursor source, unified to ONE unambiguous target (below).
//  2. CURSOR lane — the model's own explicit trace_query pid/thread targets
//     (RuntimeTargetSourceExplicitToolCall): the model explored one thread
//     consistently, so the supplement follows it.
//  3. Anything else (ambiguous / absent) ⇒ fail-open skip. 禁猜.
//
// Lane unification (h4 first-trip witness, 2026-07-14): a lane is
// unambiguous when its entries share exactly ONE distinct positive pid —
// the integer pid is the precise signal; the model-authored thread SPELLING
// may vary across calls for the same pid (".ugc.aweme.lite 17267" vs
// "….lite-17267") and is kept only when unanimous, else the target goes
// pid-only. Thread-only entries (no pid) must be exact-string unique;
// mixed pids or mixed thread-only labels stay ambiguous ⇒ skip.
func traceSupplementDeriveTarget(ctx *types.BusContext) (traceQueryRequestTarget, string, bool) {
	var userLane, cursorLane []traceQueryRequestTarget
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		for _, runtimeTarget := range rm.RuntimeTargets {
			target := traceQueryRequestTarget{
				PID:    runtimeTarget.PID,
				Thread: strings.TrimSpace(runtimeTarget.Thread),
				Source: strings.TrimSpace(runtimeTarget.Source),
			}
			if !traceQueryTypedRuntimeTargetSafe(target) {
				continue
			}
			if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
				cursorLane = append(cursorLane, target)
			} else {
				userLane = append(userLane, target)
			}
		}
	}
	if ctx.AnalysisIR != nil {
		collect(&ctx.AnalysisIR.RequestModel)
	}
	if ctx.Mutable != nil {
		collect(ctx.Mutable.RequestModel())
	}
	if target, ok := traceSupplementUnifyLaneTargets(userLane); ok {
		return target, "user", true
	}
	if len(userLane) == 0 {
		if target, ok := traceSupplementUnifyLaneTargets(cursorLane); ok {
			return target, "cursor", true
		}
	}
	return traceQueryRequestTarget{}, "", false
}

// traceSupplementUnifyLaneTargets reduces one lane to a single unambiguous
// target per the pid-first rule documented on traceSupplementDeriveTarget.
func traceSupplementUnifyLaneTargets(lane []traceQueryRequestTarget) (traceQueryRequestTarget, bool) {
	if len(lane) == 0 {
		return traceQueryRequestTarget{}, false
	}
	pid := 0
	thread := ""
	threadUnanimous := true
	threadOnly := ""
	for _, target := range lane {
		if target.PID > 0 {
			if pid == 0 {
				pid = target.PID
			} else if target.PID != pid {
				return traceQueryRequestTarget{}, false
			}
			if target.Thread != "" {
				if thread == "" {
					thread = target.Thread
				} else if !strings.EqualFold(thread, target.Thread) {
					threadUnanimous = false
				}
			}
			continue
		}
		// Thread-only entry: exact-string uniqueness (case-insensitive).
		if threadOnly == "" {
			threadOnly = target.Thread
		} else if !strings.EqualFold(threadOnly, target.Thread) {
			return traceQueryRequestTarget{}, false
		}
	}
	if pid > 0 {
		// A thread-only entry beside a pid entry is ambiguous unless its
		// label matches the pid entries' unanimous label.
		if threadOnly != "" && (!threadUnanimous || !strings.EqualFold(threadOnly, thread)) {
			return traceQueryRequestTarget{}, false
		}
		if !threadUnanimous {
			thread = ""
		}
		return traceQueryRequestTarget{PID: pid, Thread: thread, Source: lane[0].Source}, true
	}
	if threadOnly != "" {
		return traceQueryRequestTarget{Thread: threadOnly, Source: lane[0].Source}, true
	}
	return traceQueryRequestTarget{}, false
}

// traceSupplementAnchorCapableView reports whether a canonical view's records
// can anchor the projection 关注窗口 (the F1 two-family whitelist's producer
// views: wakeup chain family + root_cause_* — trace_causal_projection.go
// traceCausalProjectionSelectedWindowAnchorFamily).
func traceSupplementAnchorCapableView(view string) bool {
	switch view {
	case "root_cause_rank", "wakeup_chain", "frame_root_cause_bundle":
		return true
	}
	return false
}

// traceSupplementScopedStatsView reports whether a canonical view's call
// window carries target-scoped ANALYSIS-window semantics (the model asks
// "account for this window"), as opposed to event_search-style locator
// probes whose windows routinely narrow to single occurrences.
func traceSupplementScopedStatsView(view string) bool {
	switch view {
	case "window_stats", "perf_stats", "thread_timeline",
		"scheduler_latency_stats", "critical_blocking_calls":
		return true
	}
	return false
}

// traceSupplementDeriveWindow derives the supplement's typed query window
// from the model's recorded explicit call windows (R4: no window ⇒ skip, the
// engine never guesses a default window). Three-lane ladder, each lane keyed
// on the typed canonical view enum; within the winning lane ALL windows must
// agree within the shared ±1ms same-window tolerance, else skip — never
// last-wins, never majority/frequency (F1 micro-probe anchor precedent):
//
//  1. Anchor-capable calls (rank/chain family — same doctrine as the
//     projection anchor whitelist).
//  2. Scoped-analysis stats/timeline calls (h2-20260714-013012 run-1
//     witness: models legitimately mix the main analysis window on
//     window_stats with event_search micro-probe drill-downs; the stats
//     lane IS the analysis window, the locator probes are not).
//  3. Every recorded call window.
//
// The representative window is the FIRST recorded window of the winning lane
// (deterministic call order).
func traceSupplementDeriveWindow(windows []types.TraceQueryCallWindow) (types.TraceQueryCallWindow, bool) {
	var anchorLane, statsLane []types.TraceQueryCallWindow
	for _, w := range windows {
		view := tracequery.CanonicalViewName(w.View)
		if traceSupplementAnchorCapableView(view) {
			anchorLane = append(anchorLane, w)
		}
		if traceSupplementScopedStatsView(view) {
			statsLane = append(statsLane, w)
		}
	}
	if len(anchorLane) > 0 {
		return traceSupplementConsistentWindow(anchorLane)
	}
	if len(statsLane) > 0 {
		return traceSupplementConsistentWindow(statsLane)
	}
	return traceSupplementConsistentWindow(windows)
}

func traceSupplementConsistentWindow(windows []types.TraceQueryCallWindow) (types.TraceQueryCallWindow, bool) {
	if len(windows) == 0 {
		return types.TraceQueryCallWindow{}, false
	}
	first := windows[0]
	for _, w := range windows[1:] {
		if absFloat(w.TimeStart-first.TimeStart) > types.TraceCausalProjectionSameWindowToleranceS ||
			absFloat(w.TimeEnd-first.TimeEnd) > types.TraceCausalProjectionSameWindowToleranceS {
			return types.TraceQueryCallWindow{}, false
		}
	}
	return first, true
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// traceSupplementUnanimousEnum returns the single non-empty value of one
// typed enum param across all recorded calls, or "" when absent or mixed
// (mixed ⇒ engine default; the supplement never guesses between two
// model-authored values).
func traceSupplementUnanimousEnum(windows []types.TraceQueryCallWindow, pick func(types.TraceQueryCallWindow) string) string {
	value := ""
	for _, w := range windows {
		v := strings.TrimSpace(pick(w))
		if v == "" {
			continue
		}
		if value == "" {
			value = v
			continue
		}
		if v != value {
			return ""
		}
	}
	return value
}

// traceSupplementCallParams is the exact wire shape of one supplement engine
// call — the same JSON schema surface a model call uses, so Execute's strict
// decode, target echo, guard ladder, caveats, and census emission are
// byte-identical to the model lane.
type traceSupplementCallParams struct {
	View         string  `json:"view"`
	PID          int     `json:"pid,omitempty"`
	Thread       string  `json:"thread,omitempty"`
	TimeStart    float64 `json:"time_start"`
	TimeEnd      float64 `json:"time_end"`
	TraceFlavor  string  `json:"trace_flavor,omitempty"`
	CoreTopology string  `json:"core_topology,omitempty"`
	Platform     string  `json:"platform,omitempty"`
}

// traceSupplementSummaryHead returns the reject summary's first line,
// capped, for the operator WARN line.
func traceSupplementSummaryHead(summary string) string {
	line := summary
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if len(line) > 240 {
		line = line[:240] + "…"
	}
	return line
}

// traceQueryRecordCallWindow registers one explicit typed model-call window
// on the run-scoped MutableState registry (consumed only by the supplement's
// window derivation). Calls without BOTH explicit time bounds are ignored —
// auto-derived/pattern windows are engine artifacts, not model parameters.
func traceQueryRecordCallWindow(ctx *types.BusContext, p traceQueryParams, window traceQueryNormalizedWindow) {
	if ctx == nil || ctx.Mutable == nil {
		return
	}
	// 修复轮 件4 (2026-07-14): the supplement's own calls carry a
	// lane-DERIVED window — recording it back would be a feedback loop,
	// not model history.
	if ctx.Mutable.SystemTraceSupplementInProgress() {
		return
	}
	if !p.TimeStart.Set() || !p.TimeEnd.Set() {
		return
	}
	ctx.Mutable.RecordTraceQueryCallWindow(types.TraceQueryCallWindow{
		View:         tracequery.CanonicalViewName(p.View),
		TimeStart:    window.RequestedStart,
		TimeEnd:      window.RequestedEnd,
		TraceFlavor:  strings.TrimSpace(p.TraceFlavor),
		CoreTopology: strings.TrimSpace(p.CoreTopology),
		Platform:     strings.TrimSpace(p.Platform),
	})
}

// RunTraceQuerySystemSupplement performs the SUPP-CORE post-explore
// deterministic supplement on the attached trace. Called by the orchestrator
// exactly once per task at the explore→extract boundary (after the model's
// completion is accepted, before the extract dispatch). Every skip is
// fail-open: output stays byte-identical to a build without this feature.
func RunTraceQuerySystemSupplement(ctx *types.BusContext) TraceQuerySupplementOutcome {
	if ctx == nil || ctx.Mutable == nil {
		return TraceQuerySupplementOutcome{}
	}
	if !ctx.Mutable.MarkSystemTraceSupplementAttempted() {
		return TraceQuerySupplementOutcome{}
	}
	out := TraceQuerySupplementOutcome{Attempted: true}
	skip := func(reason string) TraceQuerySupplementOutcome {
		out.SkipReason = reason
		logging.Info("[trace_supplement] skip reason=%s (fail-open: output unchanged)", reason)
		return out
	}
	if !traceSupplementEnabled {
		return skip("disabled")
	}
	// Attached-trace gate: reuse the tool's own source resolution (attached
	// blob or the exactly-one request-referenced trace artifact — Q3 gate).
	path, sourceLabel, reject := resolveTraceQuerySource(ctx, traceQueryParams{})
	if reject != nil || strings.TrimSpace(path) == "" {
		return skip("no_attached_trace")
	}
	// Family detection runs on the SAME compiled ledger the renderer
	// consumes (single value source for presence/absence).
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	families := traceSupplementFamilies(types.CompileObservationLedger(input))
	views := traceSupplementViews(families)
	if len(views) == 0 {
		return skip("families_present")
	}
	target, targetSource, ok := traceSupplementDeriveTarget(ctx)
	if !ok {
		return skip("no_typed_target")
	}
	callWindows := ctx.Mutable.TraceQueryCallWindows()
	window, ok := traceSupplementDeriveWindow(callWindows)
	if !ok {
		if len(callWindows) == 0 {
			return skip("no_typed_window")
		}
		return skip("window_inconsistent")
	}
	// P1 window-span budget gate: the warm lane's view cost scales with
	// in-window events, so an over-budget span skips the WHOLE supplement
	// before any engine call — with an honest answer-side disclosure (the
	// user asked about that window; the report must say the supplement did
	// not run for it) instead of a silent truncation or a guessed sub-window.
	if span := window.TimeEnd - window.TimeStart; span > traceSupplementMaxWindowSpanS {
		logging.Warning("[trace_supplement] skip reason=window_span_exceeded span=%.6fs budget=%.6fs window=%.6f..%.6f",
			span, traceSupplementMaxWindowSpanS, window.TimeStart, window.TimeEnd)
		out.SkipReason = "window_span_exceeded"
		ctx.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{
			SkipReason:    "window_span_exceeded",
			SkippedViews:  views,
			WindowStart:   window.TimeStart,
			WindowEnd:     window.TimeEnd,
			WindowBudgetS: traceSupplementMaxWindowSpanS,
			TargetPID:     target.PID,
			TargetThread:  target.Thread,
			TargetSource:  targetSource,
		}, nil)
		return out
	}
	warm := false
	for _, result := range input.ToolResults {
		if result.Success && strings.EqualFold(strings.TrimSpace(result.ToolName), "trace_query") {
			warm = true
			break
		}
	}
	if !warm {
		if info, err := os.Stat(path); err == nil && info.Size() > traceSupplementMaxColdBytes {
			logging.Warning("[trace_supplement] skip reason=cold_budget_exceeded size=%d budget=%d path=%s", info.Size(), traceSupplementMaxColdBytes, path)
			out.SkipReason = "cold_budget_exceeded"
			return out
		}
	}
	start := time.Now()
	results := make([]types.ToolResult, 0, len(views))
	executed := make([]string, 0, len(views))
	var skippedViews []string
	// 件4: suppress the Execute-side cursor/window recorders for the
	// supplement's own calls (feedback-loop guard).
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	for i, view := range views {
		// P1 between-view deadline: after a completed view, an over-deadline
		// supplement skips the REMAINING views only — completed views'
		// observations are already-recorded deterministic facts and are
		// never dropped mid-flight. (In-view cancellation = engine context
		// cooperation, filed separately.)
		if len(executed) > 0 && time.Since(start) > traceSupplementMaxDuration {
			skippedViews = append([]string(nil), views[i:]...)
			logging.Warning("[trace_supplement] skip reason=duration_budget_exceeded elapsed=%s budget=%s completed=%s skipped=%s",
				time.Since(start).Round(time.Millisecond), traceSupplementMaxDuration, strings.Join(executed, ","), strings.Join(skippedViews, ","))
			break
		}
		params := traceSupplementCallParams{
			View:         view,
			PID:          target.PID,
			Thread:       target.Thread,
			TimeStart:    window.TimeStart,
			TimeEnd:      window.TimeEnd,
			TraceFlavor:  traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.TraceFlavor }),
			CoreTopology: traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.CoreTopology }),
			Platform:     traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.Platform }),
		}
		raw, err := json.Marshal(params)
		if err != nil {
			logging.Warning("[trace_supplement] view=%s params marshal failed: %v", view, err)
			continue
		}
		callStart := time.Now()
		result, execErr := (&TraceQuery{}).Execute(ctx, raw)
		callElapsed := time.Since(callStart)
		if execErr != nil {
			logging.Warning("[trace_supplement] view=%s failed elapsed=%s err=%v", view, callElapsed.Round(time.Millisecond), execErr)
			continue
		}
		// 修复轮 件2 (2026-07-14): a Success=false typed reject (density
		// guard, parse failure, source reject …) contributes ZERO ledger
		// records, so it must not count as executed and must not mint a
		// disclosure line — a disclosed "补跑" with an empty account would
		// be a false provenance claim. Same treatment as execErr: log WARN,
		// fail open.
		if !result.Success {
			logging.Warning("[trace_supplement] view=%s rejected by engine elapsed=%s (not counted, not disclosed): %s",
				view, callElapsed.Round(time.Millisecond), traceSupplementSummaryHead(result.Summary))
			continue
		}
		logging.Info("[trace_supplement] view=%s elapsed=%s window=%.6f..%.6f pid=%d thread=%q source=%s warm=%t",
			view, callElapsed.Round(time.Millisecond), window.TimeStart, window.TimeEnd, target.PID, target.Thread, sourceLabel, warm)
		// 修复轮 件3 (2026-07-14): register the result's payload blobs on the
		// Q5-A escape-lane registry — the supplement bypasses the
		// AppendDispatchToolResult chokepoint that performs this for model
		// calls, and an unregistered ref is visible on the finalize feed but
		// unreadable (read_file/grep allow-gate).
		ctx.Mutable.RegisterTraceQueryBlobRefsFromToolResult(result)
		executed = append(executed, view)
		results = append(results, result)
	}
	out.Elapsed = time.Since(start)
	if len(executed) == 0 {
		return skip("execution_failed")
	}
	out.Executed = executed
	meta := types.SystemTraceSupplementMeta{
		Views:        executed,
		WindowStart:  window.TimeStart,
		WindowEnd:    window.TimeEnd,
		TargetPID:    target.PID,
		TargetThread: target.Thread,
		TargetSource: targetSource,
		ElapsedMS:    out.Elapsed.Milliseconds(),
	}
	if len(skippedViews) > 0 {
		meta.SkipReason = "duration_budget_exceeded"
		meta.SkippedViews = skippedViews
	}
	ctx.Mutable.SetSystemTraceSupplement(meta, results)
	logging.Info("[trace_supplement] executed views=%s total_elapsed=%s (deterministic assembly-time supplement; observations enter the dedicated system lane)",
		strings.Join(executed, ","), out.Elapsed.Round(time.Millisecond))
	return out
}
