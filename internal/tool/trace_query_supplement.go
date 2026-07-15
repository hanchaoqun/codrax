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
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

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
//   - MaxDuration is the supplement's ONE wall-clock duration budget with
//     two enforcement rungs. Between views: after each completed view, an
//     over-deadline supplement skips the remaining views (skip
//     reason=duration_budget_exceeded) while KEEPING every completed view's
//     observations — partial results are already-recorded deterministic
//     facts and are never dropped mid-flight. IN-view (SUPP-CANCEL,
//     2026-07-14, closes the filing this note used to carry): the same
//     budget rides a context deadline through Query.WithRunContext into
//     tracequery.Run's cooperative sampling points, so a single over-budget
//     view cancels mid-run — completed result sections publish, unfinished
//     sections are discarded whole by the engine (禁半账), and the canceled
//     views are disclosed through meta.CanceledViews (禁裸丢).
var (
	traceSupplementEnabled            = true
	traceSupplementMaxColdBytes int64 = 2 << 30
	// Duration/span defaults per user ruling (修复轮, 2026-07-14): 20s
	// between-view deadline, 120s window-span budget.
	traceSupplementMaxDuration    = 20 * time.Second
	traceSupplementMaxWindowSpanS = 120.0
)

// traceSupplementAfterViewHook is a TEST-ONLY seam invoked after each
// completed windowed view (nil in production — never set outside tests). The
// between-view deadline branch is otherwise reachable only through a
// wall-clock race against the SUPP-CANCEL in-view deadline context, so the
// deterministic pin for "completed views are kept, the remainder skips with
// disclosure" injects its budget overrun here.
var traceSupplementAfterViewHook func(view string)

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
	// ("families_present" is the healthy no-op). Values come from the
	// types.TraceSupplementReason* closed set (SUPP-HYG P3-4).
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
//  3. ENTITIES fallback (SUPP-TARGET §29.90.1, 2026-07-15) — ONLY when the
//     typed RuntimeTargets lane minted NOTHING AT ALL (h2 20260714-221545
//     witness: a classifier variant copied the thread identity into the
//     entities list but skipped the typed lane, and the model's own calls
//     were pattern-only, so both lanes above were empty and three hard
//     answer faces silently failed to mint): a thread-shaped `name-pid`
//     entity that parses unambiguously (precise gate below) recovers the
//     target; meta.TargetSource discloses the lane as "entities_fallback".
//     Targets that EXIST but are ambiguous never reach this rung — an
//     ambiguous typed lane is a deliberate skip and must not be overridden
//     by the noisier entities face.
//  4. Anything else (ambiguous / absent / unparseable) ⇒ fail-open skip. 禁猜.
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
	rawTargets := 0
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		rawTargets += len(rm.RuntimeTargets)
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
	if rawTargets == 0 {
		if target, ok := traceSupplementEntitiesFallbackTarget(ctx); ok {
			logging.Info("[trace_supplement] target derived from analyzer entities fallback pid=%d thread=%q (typed runtime_targets lane empty; source=%s)",
				target.PID, target.Thread, traceSupplementTargetSourceEntitiesFallback)
			return target, traceSupplementTargetSourceEntitiesFallback, true
		}
	}
	return traceQueryRequestTarget{}, "", false
}

// traceSupplementTargetSourceEntitiesFallback is the meta.TargetSource /
// derivation-lane label for rung 3 of traceSupplementDeriveTarget
// (disclosure/audit only, beside "user" and "cursor").
const traceSupplementTargetSourceEntitiesFallback = "entities_fallback"

// traceSupplementEntitiesFallbackTarget parses the analyzer's typed entities
// list for EXACTLY ONE thread-shaped `name-pid` identity (SUPP-TARGET
// §29.90.1). Precise parse gate — every miss is a silent skip (宁缺勿假):
//
//   - split at the LAST dash: the tail must be ALL digits (1..7 of them) and
//     parse to a pid in (0, RuntimeTargetMaxPID] — the name part may itself
//     contain dashes or trailing digits ("CompThread_0-2955" splits to name
//     "CompThread_0", pid 2955);
//   - the name part must contain at least one letter — bare numeric ranges
//     ("100-200": line spans, windows) never become targets;
//   - the whole verbatim label must pass the shared typed-target safety gate
//     (length / forbidden characters);
//   - uniqueness: all parseable entities must agree on ONE distinct pid, or
//     the whole fallback is abandoned; same pid under different spellings
//     keeps the pid and drops the label (the lane-unification precedent).
func traceSupplementEntitiesFallbackTarget(ctx *types.BusContext) (traceQueryRequestTarget, bool) {
	if ctx == nil {
		return traceQueryRequestTarget{}, false
	}
	var entities []string
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		entities = append(entities, rm.AnalyzerHints.Entities...)
	}
	if ctx.AnalysisIR != nil {
		collect(&ctx.AnalysisIR.RequestModel)
	}
	if ctx.Mutable != nil {
		collect(ctx.Mutable.RequestModel())
	}
	pid := 0
	thread := ""
	threadUnanimous := true
	for _, entity := range entities {
		label := strings.TrimSpace(entity)
		i := strings.LastIndexByte(label, '-')
		if i <= 0 || i == len(label)-1 {
			continue
		}
		name, digits := label[:i], label[i+1:]
		if len(digits) > 7 || strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			continue
		}
		if !strings.ContainsFunc(name, unicode.IsLetter) {
			continue
		}
		parsed, err := strconv.Atoi(digits)
		if err != nil || parsed <= 0 || parsed > traceQueryMaxInheritedPID {
			continue
		}
		candidate := traceQueryRequestTarget{PID: parsed, Thread: label}
		if !traceQueryTypedRuntimeTargetSafe(candidate) {
			continue
		}
		if pid == 0 {
			pid = parsed
			thread = label
			continue
		}
		if parsed != pid {
			// Two distinct thread-shaped pids on the entities face —
			// ambiguous, the fallback is abandoned whole.
			return traceQueryRequestTarget{}, false
		}
		if !strings.EqualFold(thread, label) {
			threadUnanimous = false
		}
	}
	if pid == 0 {
		return traceQueryRequestTarget{}, false
	}
	if !threadUnanimous {
		thread = ""
	}
	return traceQueryRequestTarget{PID: pid, Thread: thread, Source: "analyzer_hints_entities"}, true
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

// traceSupplementCensusLiteParams is the C-lite call shape (SA-F2 批4,
// 2026-07-14): a WINDOWLESS whole-trace event_search — time bounds are
// deliberately absent (omitting the fields keeps Execute's optional-window
// semantics; a literal 0 would be a claimed bound).
type traceSupplementCensusLiteParams struct {
	View         string `json:"view"`
	Pattern      string `json:"pattern"`
	TraceFlavor  string `json:"trace_flavor,omitempty"`
	CoreTopology string `json:"core_topology,omitempty"`
	Platform     string `json:"platform,omitempty"`
}

// traceSupplementCensusLitePattern is the C-lite literal search token: the
// event_search pattern is a case-insensitive literal substring, and the
// survey-derived generator signals (VSyncGenerator comm, hwc_vsync_threa
// comm, GenerateVsyncCount period print) all carry it, as do the consumer
// callbacks — one token covers the whole family's matched set.
const traceSupplementCensusLitePattern = "vsync"

// traceSupplementVsyncFamilyHit reports whether the request's TYPED analyzer
// keyword/entity face names the vsync/frame family (SA-F2 批4 "命中 VSync/帧类
// family" gate). Reads only analyzer-emitted typed lists (never RawRequest);
// precise verbatim tokens:
//   - substring hits: unambiguous family words (a keyword CONTAINING them is
//     family-scoped by construction);
//   - exact hits: short words whose substring form would false-fire
//     ("frame" ⊂ "framework", "帧" is kept exact-or-compound via the
//     substring list's compound forms).
func traceSupplementVsyncFamilyHit(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	var terms []string
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		terms = append(terms, rm.AnalyzerHints.Keywords...)
		terms = append(terms, rm.AnalyzerHints.Entities...)
		terms = append(terms, rm.AnalyzerHints.PrimaryEntities...)
	}
	if ctx.AnalysisIR != nil {
		collect(&ctx.AnalysisIR.RequestModel)
	}
	if ctx.Mutable != nil {
		collect(ctx.Mutable.RequestModel())
	}
	// 修复轮 件4 (P3, 2026-07-14):「卡顿」moved OUT of the exact set — bare
	// 卡顿 is the generic stutter word (IO/lock/GC stutter questions all
	// carry it) and would false-arm the vsync census scan on non-frame
	// questions; genuine frame-stutter analyzer keyword lists carry a frame
	// word (掉帧/帧率/卡帧/vsync/…) that the substring family already hits.
	substrings := []string{"vsync", "choreographer", "doframe", "垂直同步", "掉帧", "丢帧", "跳帧", "帧率", "帧节拍", "卡帧", "jank"}
	exact := map[string]bool{"frame": true, "frames": true, "帧": true}
	for _, term := range terms {
		lower := strings.ToLower(strings.TrimSpace(term))
		if lower == "" {
			continue
		}
		if exact[lower] {
			return true
		}
		for _, token := range substrings {
			if strings.Contains(lower, token) {
				return true
			}
		}
	}
	return false
}

// traceSupplementObservationsCarryVsyncCensus reports whether any of the
// given records carries the SA-F2 generator-census predicate (the C-lite
// absence gate's precise signal — 修复轮 件2, 2026-07-14).
func traceSupplementObservationsCarryVsyncCensus(records []types.ObservationRecord) bool {
	for _, record := range records {
		if strings.TrimSpace(record.Predicate) == "vsync_generator_census" {
			return true
		}
	}
	return false
}

// traceSupplementResultsCarryVsyncCensus is the ToolResult-set form of the
// census-presence signal.
func traceSupplementResultsCarryVsyncCensus(results []types.ToolResult) bool {
	for _, result := range results {
		if traceSupplementObservationsCarryVsyncCensus(result.Observations) {
			return true
		}
	}
	return false
}

// traceSupplementExecuteCensusLite runs the SA-F2 批4 C-lite pass: ONE
// windowless whole-trace streaming event_search (single-pass scan, zero heavy
// views) minting the generator census. Fail-open everywhere: any gate/engine
// failure — or a scan that produced no census record (never a claim of
// absence: an unmatched scan is data coverage, not device behavior) — returns
// ok=false and the caller's lane stands unchanged. The caller owns the
// Begin/End execution bracket and the meta/results bookkeeping.
func traceSupplementExecuteCensusLite(ctx *types.BusContext, path, laneReason string) (types.ToolResult, time.Duration, bool) {
	// The pass is one streaming scan over the trace file; the cold byte
	// budget bounds it the same way it bounds the full supplement's cold
	// lane.
	if info, err := os.Stat(path); err == nil && info.Size() > traceSupplementMaxColdBytes {
		logging.Warning("[trace_supplement] census-lite skip reason=%s size=%d budget=%d path=%s", types.TraceSupplementReasonColdBudgetExceeded, info.Size(), traceSupplementMaxColdBytes, path)
		return types.ToolResult{}, 0, false
	}
	params := traceSupplementCensusLiteParams{
		View:    "event_search",
		Pattern: traceSupplementCensusLitePattern,
	}
	raw, err := json.Marshal(params)
	if err != nil {
		logging.Warning("[trace_supplement] census-lite params marshal failed: %v", err)
		return types.ToolResult{}, 0, false
	}
	start := time.Now()
	result, execErr := (&TraceQuery{}).Execute(ctx, raw)
	elapsed := time.Since(start)
	if execErr != nil {
		logging.Warning("[trace_supplement] census-lite view=event_search failed elapsed=%s err=%v", elapsed.Round(time.Millisecond), execErr)
		return types.ToolResult{}, elapsed, false
	}
	if !result.Success {
		logging.Warning("[trace_supplement] census-lite view=event_search rejected by engine elapsed=%s (not counted, not disclosed): %s",
			elapsed.Round(time.Millisecond), traceSupplementSummaryHead(result.Summary))
		return types.ToolResult{}, elapsed, false
	}
	if !traceSupplementObservationsCarryVsyncCensus(result.Observations) {
		logging.Info("[trace_supplement] census-lite view=event_search found no generator census (lane=%s stands; output unchanged)", laneReason)
		return types.ToolResult{}, elapsed, false
	}
	ctx.Mutable.RegisterTraceQueryBlobRefsFromToolResult(result)
	return result, elapsed, true
}

// runTraceSupplementCensusLite is the STANDALONE C-lite arm (SA-F2 批4 +
// 修复轮 件2, 2026-07-14): invoked on every lane where the FULL windowed
// supplement did not store results — the derivation-failure family (S13: no
// typed target / no typed window / inconsistent windows), the healthy
// families_present no-op, and the execution_failed lane. The trigger gate
// (vsync-family keywords hit ∧ census family absent from the compiled ledger)
// is the CALLER's censusLiteWanted signal; this helper runs the pass and
// stores a lite-only supplement (meta.Views empty — no windowed view ran).
func runTraceSupplementCensusLite(ctx *types.BusContext, path, sourceLabel, laneReason string, out *TraceQuerySupplementOutcome) bool {
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	result, elapsed, ok := traceSupplementExecuteCensusLite(ctx, path, laneReason)
	if !ok {
		return false
	}
	meta := types.SystemTraceSupplementMeta{
		CensusLite:        true,
		CensusLitePattern: traceSupplementCensusLitePattern,
		ElapsedMS:         elapsed.Milliseconds(),
	}
	ctx.Mutable.SetSystemTraceSupplement(meta, []types.ToolResult{result})
	logging.Info("[trace_supplement] census-lite executed view=event_search pattern=%s elapsed=%s source=%s (windowless single-pass generator census; lane=%s)",
		traceSupplementCensusLitePattern, elapsed.Round(time.Millisecond), sourceLabel, laneReason)
	out.Executed = []string{"event_search"}
	out.Elapsed = elapsed
	return true
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
		return skip(types.TraceSupplementReasonDisabled)
	}
	// Attached-trace gate: reuse the tool's own source resolution (attached
	// blob or the exactly-one request-referenced trace artifact — Q3 gate).
	path, sourceLabel, reject := resolveTraceQuerySource(ctx, traceQueryParams{})
	if reject != nil || strings.TrimSpace(path) == "" {
		return skip(types.TraceSupplementReasonNoAttachedTrace)
	}
	// SUPP-CANCEL (2026-07-14): ONE wall-clock duration budget per supplement
	// attempt — the same trace_supplement_max_duration_ms knob that drives
	// the between-view skip now ALSO rides a context deadline into every
	// engine call (windowed views AND the C-lite streaming scan — 批4 P3-5),
	// so a single long view cancels cooperatively in-view instead of
	// stalling report assembly unboundedly. execCtx is a ShallowClone with
	// only the context swapped: Mutable/WorkDir and every other exported
	// field alias the caller's, so the recorder brackets and blob lanes are
	// untouched (never `cp := *ctx` — BusContext carries a cache mutex,
	// copylocks vet pin).
	dctx, cancelDeadline := context.WithDeadline(contextFromBus(ctx), time.Now().Add(traceSupplementMaxDuration))
	defer cancelDeadline()
	execCtx := ctx.ShallowClone()
	execCtx.Ctx = dctx
	// Family detection runs on the SAME compiled ledger the renderer
	// consumes (single value source for presence/absence).
	input := types.ObservationLedgerInputFromBusContext(ctx, types.ObservationExtractLedgerEvidenceLimit)
	preLedger := types.CompileObservationLedger(input)
	families := traceSupplementFamilies(preLedger)
	views := traceSupplementViews(families)
	// SA-F2 批4 C-lite trigger gate (修复轮 件2 扩形, 2026-07-14): vsync/frame
	// family keywords hit (typed analyzer face) ∧ the generator-census family
	// is ABSENT from the compiled ledger. The arm fires on EVERY lane where
	// the census would otherwise stay absent — derivation failures (S13),
	// the healthy families_present no-op (run-1 cold-read residual: the
	// model's own windowed dispatches minted the core families but none of
	// its searches matched a generator row), the span-budget skip, the
	// execution_failed lane, and the windowed success path whose executed
	// views did not mint the census. A ledger that already carries the
	// census keeps every lane byte-identical.
	censusLiteWanted := traceSupplementVsyncFamilyHit(ctx) &&
		!traceSupplementObservationsCarryVsyncCensus(preLedger.Records)
	if len(views) == 0 {
		if censusLiteWanted && runTraceSupplementCensusLite(execCtx, path, sourceLabel, types.TraceSupplementReasonFamiliesPresent, &out) {
			return out
		}
		return skip(types.TraceSupplementReasonFamiliesPresent)
	}
	target, targetSource, ok := traceSupplementDeriveTarget(ctx)
	if !ok {
		// The windowless census arm covers the derivation-failure family (no
		// target / no window / inconsistent windows) — a full re-run is
		// impossible, but a single-pass generator census needs neither
		// target nor window.
		if censusLiteWanted && runTraceSupplementCensusLite(execCtx, path, sourceLabel, types.TraceSupplementReasonNoTypedTarget, &out) {
			return out
		}
		return skip(types.TraceSupplementReasonNoTypedTarget)
	}
	callWindows := ctx.Mutable.TraceQueryCallWindows()
	window, ok := traceSupplementDeriveWindow(callWindows)
	if !ok {
		reason := types.TraceSupplementReasonWindowInconsistent
		if len(callWindows) == 0 {
			reason = types.TraceSupplementReasonNoTypedWindow
		}
		if censusLiteWanted && runTraceSupplementCensusLite(execCtx, path, sourceLabel, reason, &out) {
			return out
		}
		return skip(reason)
	}
	// P1 window-span budget gate: the warm lane's view cost scales with
	// in-window events, so an over-budget span skips the WHOLE supplement
	// before any engine call — with an honest answer-side disclosure (the
	// user asked about that window; the report must say the supplement did
	// not run for it) instead of a silent truncation or a guessed sub-window.
	if span := window.TimeEnd - window.TimeStart; span > traceSupplementMaxWindowSpanS {
		logging.Warning("[trace_supplement] skip reason=%s span=%.6fs budget=%.6fs window=%.6f..%.6f",
			types.TraceSupplementReasonWindowSpanExceeded, span, traceSupplementMaxWindowSpanS, window.TimeStart, window.TimeEnd)
		out.SkipReason = types.TraceSupplementReasonWindowSpanExceeded
		meta := types.SystemTraceSupplementMeta{
			SkipReason:    types.TraceSupplementReasonWindowSpanExceeded,
			SkippedViews:  views,
			WindowStart:   window.TimeStart,
			WindowEnd:     window.TimeEnd,
			WindowBudgetS: traceSupplementMaxWindowSpanS,
			TargetPID:     target.PID,
			TargetThread:  target.Thread,
			TargetSource:  targetSource,
		}
		var spanResults []types.ToolResult
		// 修复轮 件2: the census arm still runs on the span-budget skip — the
		// single-pass scan is not the per-window view cost the budget guards
		// (cold byte budget still applies inside), and the census family
		// would otherwise stay absent for this run.
		if censusLiteWanted {
			ctx.Mutable.BeginSystemTraceSupplementExecution()
			result, elapsed, ok := traceSupplementExecuteCensusLite(execCtx, path, types.TraceSupplementReasonWindowSpanExceeded)
			ctx.Mutable.EndSystemTraceSupplementExecution()
			if ok {
				meta.CensusLite = true
				meta.CensusLitePattern = traceSupplementCensusLitePattern
				meta.ElapsedMS = elapsed.Milliseconds()
				spanResults = []types.ToolResult{result}
				out.Executed = []string{"event_search"}
				out.Elapsed = elapsed
				logging.Info("[trace_supplement] census-lite executed view=event_search pattern=%s elapsed=%s source=%s (windowless single-pass generator census; lane=%s)",
					traceSupplementCensusLitePattern, elapsed.Round(time.Millisecond), sourceLabel, types.TraceSupplementReasonWindowSpanExceeded)
			}
		}
		ctx.Mutable.SetSystemTraceSupplement(meta, spanResults)
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
			logging.Warning("[trace_supplement] skip reason=%s size=%d budget=%d path=%s", types.TraceSupplementReasonColdBudgetExceeded, info.Size(), traceSupplementMaxColdBytes, path)
			out.SkipReason = types.TraceSupplementReasonColdBudgetExceeded
			return out
		}
	}
	start := time.Now()
	results := make([]types.ToolResult, 0, len(views))
	executed := make([]string, 0, len(views))
	var skippedViews []string
	var canceledViews []string
	afterView := traceSupplementAfterViewHook
	// 件4: suppress the Execute-side cursor/window recorders for the
	// supplement's own calls (feedback-loop guard).
	ctx.Mutable.BeginSystemTraceSupplementExecution()
	defer ctx.Mutable.EndSystemTraceSupplementExecution()
	for i, view := range views {
		// P1 between-view deadline: after a completed view, an over-deadline
		// supplement skips the REMAINING views only — completed views'
		// observations are already-recorded deterministic facts and are
		// never dropped mid-flight. (In-view cancellation is the OTHER rung
		// of the same budget — delivered by SUPP-CANCEL 2026-07-14: execCtx
		// carries the deadline into the engine's cooperative sampling
		// points, classified right after the Execute call below.)
		if len(executed) > 0 && time.Since(start) > traceSupplementMaxDuration {
			skippedViews = append([]string(nil), views[i:]...)
			logging.Warning("[trace_supplement] skip reason=%s elapsed=%s budget=%s completed=%s skipped=%s",
				types.TraceSupplementReasonDurationBudgetExceeded, time.Since(start).Round(time.Millisecond), traceSupplementMaxDuration, strings.Join(executed, ","), strings.Join(skippedViews, ","))
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
		result, execErr := (&TraceQuery{}).Execute(execCtx, raw)
		callElapsed := time.Since(callStart)
		if execErr != nil {
			logging.Warning("[trace_supplement] view=%s failed elapsed=%s err=%v", view, callElapsed.Round(time.Millisecond), execErr)
			continue
		}
		// SUPP-CANCEL (2026-07-14): cancellation lanes.
		// (a) canceled BEFORE any engine work could publish (parse-phase fire
		//     ⇒ Success=false carrying the Execute-minted typed
		//     TraceViewCancellation — SUPP-HYG P3-D closed the race window
		//     where this lane keyed on the ambient dctx.Err() and an ordinary
		//     engine reject landing after the deadline expiry was misfiled as
		//     canceled) — nothing recorded, but the skip is DISCLOSED
		//     (禁裸丢), unlike the ordinary engine-reject fail-open below;
		// (b) canceled IN-VIEW with the typed engine record — completed
		//     faces (if any) are real observations and count as executed;
		//     zero-observation cancellations must not claim a re-run
		//     (修复轮 件2 rule) but are disclosed as canceled.
		if !result.Success {
			if result.TraceViewCancellation != nil {
				canceledViews = append(canceledViews, view)
				logging.Warning("[trace_supplement] view=%s canceled before completion reason=%s elapsed=%s (disclosed, nothing recorded): %s",
					view, result.TraceViewCancellation.Reason, callElapsed.Round(time.Millisecond), traceSupplementSummaryHead(result.Summary))
				continue
			}
			// 修复轮 件2 (2026-07-14): a Success=false typed reject (density
			// guard, parse failure, source reject …) contributes ZERO ledger
			// records, so it must not count as executed and must not mint a
			// disclosure line — a disclosed "补跑" with an empty account would
			// be a false provenance claim. Same treatment as execErr: log
			// WARN, fail open.
			logging.Warning("[trace_supplement] view=%s rejected by engine elapsed=%s (not counted, not disclosed): %s",
				view, callElapsed.Round(time.Millisecond), traceSupplementSummaryHead(result.Summary))
			continue
		}
		if vc := result.TraceViewCancellation; vc != nil {
			canceledViews = append(canceledViews, view)
			if len(result.Observations) == 0 {
				logging.Warning("[trace_supplement] view=%s canceled in-view by the duration budget with no complete face elapsed=%s reason=%s (disclosed, nothing recorded)",
					view, callElapsed.Round(time.Millisecond), vc.Reason)
				continue
			}
			logging.Warning("[trace_supplement] view=%s canceled in-view by the duration budget elapsed=%s reason=%s discarded=%s (complete faces recorded, unfinished faces discarded whole)",
				view, callElapsed.Round(time.Millisecond), vc.Reason, strings.Join(vc.DiscardedFaces, ","))
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
		if afterView != nil {
			afterView(view)
		}
	}
	if len(executed) == 0 {
		out.Elapsed = time.Since(start)
		// SUPP-CANCEL: nothing was recorded but the cancellation canceled ≥1
		// view — the user's window did not get its re-run, so the skip is
		// DISCLOSED through the meta lane (禁裸丢), never the silent
		// execution_failed fail-open. SUPP-HYG P3-D (2026-07-14): the reason
		// forks on the precise errors.Is class of the deadline context — this
		// zero-execution lane previously stamped the duration-budget wording
		// unconditionally, mislabeling a caller/user abort (context.Canceled
		// riding the bus context) as a budget overrun.
		if len(canceledViews) > 0 {
			skipReason := types.TraceSupplementReasonDurationBudgetExceeded
			if !errors.Is(dctx.Err(), context.DeadlineExceeded) {
				skipReason = types.TraceSupplementReasonCanceledByCaller
			}
			meta := types.SystemTraceSupplementMeta{
				SkipReason:      skipReason,
				SkippedViews:    skippedViews,
				CanceledViews:   canceledViews,
				DurationBudgetS: traceSupplementMaxDuration.Seconds(),
				WindowStart:     window.TimeStart,
				WindowEnd:       window.TimeEnd,
				TargetPID:       target.PID,
				TargetThread:    target.Thread,
				TargetSource:    targetSource,
				ElapsedMS:       out.Elapsed.Milliseconds(),
			}
			ctx.Mutable.SetSystemTraceSupplement(meta, nil)
			out.SkipReason = skipReason
			logging.Warning("[trace_supplement] canceled with zero recorded views reason=%s canceled=%s budget=%s (disclosed)",
				skipReason, strings.Join(canceledViews, ","), traceSupplementMaxDuration)
			return out
		}
		// 修复轮 件2: engine-rejected windowed views leave the census family
		// absent too — the standalone lite arm salvages the generator
		// account (its own meta; nothing else was stored).
		if censusLiteWanted && runTraceSupplementCensusLite(execCtx, path, sourceLabel, types.TraceSupplementReasonExecutionFailed, &out) {
			return out
		}
		return skip(types.TraceSupplementReasonExecutionFailed)
	}
	// 修复轮 件2 (有窗趟武装, cold-read 残差修根): the windowed views ran but
	// none of them minted the generator census (e.g. the derived window does
	// not cover the generator burst, or rank was already present and only
	// critical_blocking re-ran). One extra single-pass scan appends the
	// census result to the SAME supplement (combined disclosure) — gated on
	// the between-view deadline like any other step, and on the census
	// staying absent from the executed results (redundant scans never run).
	censusLiteRan := false
	if censusLiteWanted && !traceSupplementResultsCarryVsyncCensus(results) &&
		time.Since(start) <= traceSupplementMaxDuration {
		if result, _, ok := traceSupplementExecuteCensusLite(execCtx, path, types.TraceSupplementReasonWindowedCensusAbsent); ok {
			results = append(results, result)
			censusLiteRan = true
			executed = append(executed, "event_search")
			logging.Info("[trace_supplement] census-lite executed view=event_search pattern=%s source=%s (windowless single-pass generator census; adjunct to executed views %s)",
				traceSupplementCensusLitePattern, sourceLabel, strings.Join(executed[:len(executed)-1], ","))
		}
	}
	out.Elapsed = time.Since(start)
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
	if censusLiteRan {
		// The lite adjunct is not a windowed view: meta.Views keeps ONLY the
		// windowed executions (the disclosure's windowed sentence must not
		// claim the whole-trace scan ran on the derived window); the lite
		// flag + pattern carry the adjunct's own disclosure clause.
		meta.Views = executed[:len(executed)-1]
		meta.CensusLite = true
		meta.CensusLitePattern = traceSupplementCensusLitePattern
	}
	if len(skippedViews) > 0 {
		meta.SkipReason = types.TraceSupplementReasonDurationBudgetExceeded
		meta.SkippedViews = skippedViews
	}
	if len(canceledViews) > 0 {
		meta.CanceledViews = canceledViews
		meta.DurationBudgetS = traceSupplementMaxDuration.Seconds()
		// SUPP-HYG P3-D: the mixed lane (≥1 completed view + ≥1 canceled
		// view) carries the same errors.Is fork as the zero-execution lane —
		// the canceled-tail wording must never blame the duration budget for
		// a caller abort.
		if meta.SkipReason == "" {
			if errors.Is(dctx.Err(), context.DeadlineExceeded) {
				meta.SkipReason = types.TraceSupplementReasonDurationBudgetExceeded
			} else if dctx.Err() != nil {
				meta.SkipReason = types.TraceSupplementReasonCanceledByCaller
			}
		}
	}
	ctx.Mutable.SetSystemTraceSupplement(meta, results)
	logging.Info("[trace_supplement] executed views=%s total_elapsed=%s (deterministic assembly-time supplement; observations enter the dedicated system lane)",
		strings.Join(executed, ","), out.Elapsed.Round(time.Millisecond))
	return out
}
