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
// model-call windows or one unique recorded enclosing anchor window — never
// last-wins, never prose, never a synthetic union), and
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
// byte-identical output. ONE carve (G4-ENGINE, 2026-07-20, §29.145 filing):
// when the WINDOW signal is the missing one, the typed target exists, and
// the request's typed analyzer face names the D-state/blocked_reason family,
// the supplement runs one WINDOWLESS root_cause_rank (the engine's own
// whole-trace default window — a documented caliber, not a guessed window)
// so the blocked_reason↔D-segment pairing faces reach the ledger on the
// event_search-only shape; every other missing-signal lane skips unchanged.
// Triggered runs disclose on the answer-side caveat lane (single total
// line, R5) and on the operator log (performance disclosure).

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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

// traceSupplementFallbackParamsHook is a TEST-ONLY seam (返工 P2-B,
// 2026-07-20) receiving the EXACT wire bytes dispatched for each G4-ENGINE
// windowless-fallback engine call (nil in production — never set outside
// tests). The wire-shape pin asserts the DISPATCHED params — not merely the
// marshal helper's output — carry no time_start/time_end key, so a
// dispatch-site bypass of the helper (marshaling the windowed struct's
// zero-valued claimed bounds inline) goes red.
var traceSupplementFallbackParamsHook func(raw []byte)

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
	IOLatency           bool // exact request/storage latency predicates
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
		case "io_latency", "io_latency_coverage", "storage_latency_by_layer", "block_io_by_inode":
			f.IOLatency = true
		}
	}
	return f
}

// traceSupplementFamiliesForRequestedScope answers a stricter question than
// traceSupplementFamilies when the analyzer has validated an explicit user
// window and/or the supplement has derived one typed target: which core
// families are present for THAT window AND THAT target? A complete family set
// from a wider/narrower or target-free model probe remains useful exploration
// evidence, but cannot suppress the deterministic requested-scope supplement.
//
// selected_window is carried on several records from one trace_query result,
// not necessarily every family row. We therefore elect matching results by a
// producer-owned result identity (payload/raw ref, then the typed ID prefix)
// and count every row from the elected result. No request/model prose, label
// similarity, or timestamp-envelope inference participates.
func traceSupplementFamiliesForRequestedScope(
	ledger types.ObservationLedger,
	scope *types.RuntimeArtifactScopeProfile,
	target traceQueryRequestTarget,
	targetKnown bool,
) traceSupplementFamilyPresence {
	if scope == nil && !targetKnown {
		return traceSupplementFamilies(ledger)
	}
	start, end, explicit := scope.ExplicitTimeWindow()
	if !explicit && !targetKnown {
		return traceSupplementFamilies(ledger)
	}
	scopedResults := map[string]bool{}
	scopedRecords := map[int]bool{}
	targetResults := map[string]bool{}
	targetRecords := map[int]bool{}
	for i, record := range ledger.Records {
		if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact ||
			!types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) {
			continue
		}
		if explicit {
			recordStart, recordEnd, ok := types.TraceCausalProjectionSelectedWindowNote(record.RichNotes)
			if !ok ||
				math.Abs(recordStart-start) > types.TraceCausalProjectionSameWindowToleranceS ||
				math.Abs(recordEnd-end) > types.TraceCausalProjectionSameWindowToleranceS {
				continue
			}
		}
		if key := traceSupplementResultIdentity(record); key != "" {
			scopedResults[key] = true
			if targetKnown && traceSupplementRecordCarriesTargetAuthority(record, target) {
				targetResults[key] = true
			}
		} else {
			scopedRecords[i] = true
			if targetKnown && traceSupplementRecordCarriesTargetAuthority(record, target) {
				targetRecords[i] = true
			}
		}
	}
	filtered := types.ObservationLedger{AnchorUserEntities: ledger.AnchorUserEntities}
	for i, record := range ledger.Records {
		key := traceSupplementResultIdentity(record)
		scopeMatch := scopedRecords[i] || (key != "" && scopedResults[key])
		targetMatch := !targetKnown || targetRecords[i] || (key != "" && targetResults[key])
		if scopeMatch && targetMatch {
			filtered.Records = append(filtered.Records, record)
		}
	}
	return traceSupplementFamilies(filtered)
}

// traceSupplementRecordCarriesTargetAuthority recognizes result-owned target
// carriers only.  A ranked row's Subject is the candidate, not the board
// target, so candidate-label coincidence is deliberately insufficient.  The
// accepted carriers are emitted from typed engine fields: the target state
// account subject, the rank board target, and frame target resolution.
func traceSupplementRecordCarriesTargetAuthority(record types.ObservationRecord, target traceQueryRequestTarget) bool {
	var label string
	switch {
	case strings.TrimSpace(record.Predicate) == "target_window_states":
		label = record.Subject
	case strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "root_cause_"):
		label = traceSupplementRichNoteValue(record.RichNotes, types.TraceNoteKeyRankBoardTarget)
	case strings.TrimSpace(record.Predicate) == "frame_target_resolution":
		label = record.Subject
	default:
		return false
	}
	return traceSupplementTargetLabelMatches(label, target)
}

func traceSupplementTargetLabelMatches(label string, target traceQueryRequestTarget) bool {
	label = strings.TrimSpace(label)
	if label == "" {
		return false
	}
	pid, name, parsed := tracequery.ParseThreadSelectorIdentity(label)
	if target.PID > 0 {
		return parsed && pid == target.PID
	}
	want := strings.TrimSpace(target.Thread)
	if want == "" {
		return false
	}
	if parsed && strings.TrimSpace(name) != "" {
		return strings.EqualFold(strings.TrimSpace(name), want)
	}
	return strings.EqualFold(label, want)
}

func traceSupplementRichNoteValue(notes []string, key string) string {
	prefix := strings.TrimSpace(key) + "="
	if prefix == "=" {
		return ""
	}
	for _, note := range notes {
		note = strings.TrimSpace(note)
		if strings.HasPrefix(note, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(note, prefix))
		}
	}
	return ""
}

func traceSupplementResultIdentity(record types.ObservationRecord) string {
	if value := strings.TrimSpace(record.SourceRef.PayloadRef); value != "" {
		return "payload:" + value
	}
	if value := strings.TrimSpace(record.SourceRef.RawRef); value != "" {
		return "raw:" + value
	}
	id := strings.TrimSpace(record.ID)
	if at := strings.IndexByte(id, '#'); at > 0 {
		return "id:" + id[:at]
	}
	return ""
}

// traceSupplementViews maps missing families to the minimal engine view set.
// One root_cause_rank call fills FOUR result fields (WakeupChain +
// WindowStats + SchedulerLatency + RootCauseRank — tracequery/query.go view
// dispatch), which mint the rank/chain/window-states families plus both
// censuses; critical_blocking_calls fills CriticalBlocking. A typed
// frame-family request without present frame evidence uses the single
// frame_root_cause_bundle superset instead. ≤2 executions.
func traceSupplementViews(f traceSupplementFamilyPresence, frameFamily, frameEvidencePresent bool) []string {
	// NW-02: the generic rank/chain families cannot substitute for a frame
	// timeline/deadline investigation. One frame bundle fills every generic
	// family below as well as the frame face, so it is both the correct and
	// the minimal heavy-view choice.
	if frameFamily && !frameEvidencePresent {
		return []string{"frame_root_cause_bundle"}
	}
	var views []string
	if !f.Rank || !f.Chain || !f.WindowStates || !f.BlockedReasonCensus || !f.WakeupEdgeCensus {
		views = append(views, "root_cause_rank")
	}
	if !f.Critical {
		views = append(views, "critical_blocking_calls")
	}
	return views
}

// traceSupplementViewsForRequest narrows deterministic supplementation to the
// answer family the typed request actually asks for. The shared report-shape
// authority runs before family/view inference: a bounded fact set must not be
// widened merely because a relation/frame label or an exact time range is also
// present. A D-state/blocked-reason runtime fact keeps its deliberately narrow
// five-state account and blocked-reason census; other bounded facts need no
// deterministic supplement. Exact windows constrain where a view runs; they
// never choose which answer family runs.
func traceSupplementViewsForRequest(ctx *types.BusContext, f traceSupplementFamilyPresence, frameFamily, frameEvidencePresent bool) []string {
	if decided, allowed := runtimeTraceReportShapeAuthority(ctx); decided && !allowed {
		if traceSupplementNarrowIOLatencyQuestion(ctx) {
			if !f.IOLatency {
				return []string{"window_stats"}
			}
			return nil
		}
		if traceSupplementNarrowDStateQuestion(ctx) {
			// A local model window cannot satisfy a full-artifact bounded census;
			// re-run the same narrow view without promoting to causal families.
			if scope := traceSupplementRequestedArtifactScope(ctx); scope.FullArtifact() {
				return []string{"window_stats"}
			}
			if !f.WindowStates || !f.BlockedReasonCensus {
				return []string{"window_stats"}
			}
		}
		return nil
	}
	if frameFamily && !frameEvidencePresent {
		return []string{"frame_root_cause_bundle"}
	}
	if traceSupplementNarrowDStateQuestion(ctx) {
		// This is the compatibility lane for pre-profile RequestModels. Exact
		// windows retain their historical causal supplement only when no typed
		// bounded-fact declaration made the narrow decision above.
		if scope := traceSupplementRequestedArtifactScope(ctx); scope != nil {
			if _, _, ok := scope.ExplicitTimeWindow(); ok {
				return traceSupplementViews(f, false, frameEvidencePresent)
			}
		}
		// EVAL-B1-R13: a quote-anchored user scope outranks whichever local
		// query window happened to mint the same families. Re-run the minimal
		// state view at that user scope even when a narrow local account is
		// already present; local complete is not artifact complete.
		if scope := traceSupplementRequestedArtifactScope(ctx); scope.FullArtifact() {
			return []string{"window_stats"}
		}
		if !f.WindowStates || !f.BlockedReasonCensus {
			return []string{"window_stats"}
		}
		return nil
	}
	return traceSupplementViews(f, false, frameEvidencePresent)
}

func traceSupplementNarrowIOLatencyQuestion(ctx *types.BusContext) bool {
	if ctx == nil {
		return false
	}
	var profile *types.RuntimeQuestionProfile
	if ctx.AnalysisIR != nil {
		profile = ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile
	} else if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			profile = rm.RuntimeQuestionProfile
		}
	}
	return profile != nil && profile.CarriesBoundedFactFamilies() &&
		profile.RequestsFactFamily(types.RuntimeQuestionFactIOLatency)
}

func traceSupplementRequestedArtifactScope(ctx *types.BusContext) *types.RuntimeArtifactScopeProfile {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile != nil {
		return ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.RuntimeArtifactScopeProfile
		}
	}
	return nil
}

func traceSupplementNarrowDStateQuestion(ctx *types.BusContext) bool {
	// New analysis records carry exact, schema-validated fact families. They
	// outrank the historical AnalyzerHints vocabulary scan below: the latter is
	// retained only for old serialized requests and synthetic fixtures.
	if ctx != nil {
		var profile *types.RuntimeQuestionProfile
		if ctx.AnalysisIR != nil {
			profile = ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile
		} else if ctx.Mutable != nil {
			if rm := ctx.Mutable.RequestModel(); rm != nil {
				profile = rm.RuntimeQuestionProfile
			}
		}
		if profile != nil {
			if !profile.CarriesBoundedFactFamilies() {
				return false
			}
			for _, family := range profile.FactFamilies {
				switch family {
				case types.RuntimeQuestionFactTargetSchedulerState,
					types.RuntimeQuestionFactTargetWaitOccurrences,
					types.RuntimeQuestionFactRecordedReason:
					return true
				}
			}
			return false
		}
	}
	if !traceSupplementDStateFamilyHit(ctx) {
		return false
	}
	causal := false
	narrowStateFact := false
	collect := func(rm *types.RequestModel) {
		if rm == nil {
			return
		}
		family := types.ResolveQuestionFamily(*rm)
		if family == types.QFGeneric && types.IsNarrowRuntimeArtifactFactShape(*rm) {
			narrowStateFact = true
		}
		switch family {
		case types.QFRootCauseTrace, types.QFCallChain:
			causal = true
		}
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		collect(&ctx.AnalysisIR.RequestModel)
	}
	if ctx != nil && ctx.Mutable != nil {
		collect(ctx.Mutable.RequestModel())
	}
	// Old request records and hand-built fixtures may carry D-state keywords
	// without question_kind/predicate_axis. Absence is not authority to narrow;
	// only an explicit typed non-call shape activates this lane.
	return narrowStateFact && !causal
}

func traceSupplementFrameEvidencePresent(input types.ObservationLedgerInput) bool {
	results := make([]types.ToolResult, 0, len(input.ToolResults)+len(input.SystemTraceSupplementResults))
	results = append(results, input.ToolResults...)
	results = append(results, input.SystemTraceSupplementResults...)
	for _, result := range results {
		if result.TraceEvidenceAuthority != nil &&
			strings.TrimSpace(result.TraceEvidenceAuthority.FrameEvidenceStatus) == "present" {
			return true
		}
	}
	return false
}

// traceSupplementTarget derives the supplement's typed target. Priority (R2
// ruling: user intent first):
//
//  1. USER lane — runtime_target_profile=named_target plus request-model
//     RuntimeTargets(source=user_explicit), unified to ONE unambiguous target.
//  2. CURSOR lane — a model trace_query cursor is eligible only for a typed
//     diagnostic/root-cause/call-relation request or an explicit user window,
//     and only when the target profile did not declare named_target.
//  3. Anything else ⇒ fail-open skip. Analyzer entities are deliberately not
//     a target recovery lane: they are noisy soft hints, not identity authority.
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
				PID:         runtimeTarget.PID,
				Thread:      strings.TrimSpace(runtimeTarget.Thread),
				Source:      strings.TrimSpace(runtimeTarget.Source),
				TargetScope: traceSupplementRuntimeTargetScope(runtimeTarget.Kind),
			}
			// R3a (§13.2 no_touying 实证): the analyzer sometimes ships the
			// label ORIGINAL STRING ("name [pid]" / "name-pid") in Thread
			// with PID unset — the lane then ran the supplement as a
			// TargetPID=0 original-string target (the fourth replay's
			// 「目标 ss.hm.ugc.aweme [32788]」disclosure form). Parse the two
			// precise label shapes into typed pid+name; unparseable labels
			// stay as-is (禁猜).
			if target.PID <= 0 && target.Thread != "" {
				if pid, name, ok := traceSupplementParseThreadLabel(target.Thread); ok {
					target.PID = pid
					target.Thread = name
				}
			}
			if target.TargetScope == tracequery.TargetScopeProcess && target.PID <= 0 {
				continue
			}
			if !traceQueryTypedRuntimeTargetSafe(target) {
				continue
			}
			if types.RuntimeTargetIsExplorationCursorSource(target.Source) {
				cursorLane = append(cursorLane, target)
			} else if target.Source == "user_explicit" {
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
	profile := traceSupplementRequestedTargetProfile(ctx)
	if profile != nil && profile.NamedTarget() {
		if target, ok := traceSupplementUnifyLaneTargets(userLane); ok {
			return target, "user", true
		}
		return traceQueryRequestTarget{}, "", false
	}
	// Backward compatibility for pre-profile persisted RequestModels: an
	// already-typed user_explicit RuntimeTarget remains authoritative. Missing
	// typed targets never revive the retired entities fallback.
	if profile == nil {
		if target, ok := traceSupplementUnifyLaneTargets(userLane); ok {
			return target, "user", true
		}
	}
	if traceSupplementCursorTargetAllowed(ctx) {
		if target, ok := traceSupplementUnifyLaneTargets(cursorLane); ok {
			return target, "cursor", true
		}
	}
	return traceQueryRequestTarget{}, "", false
}

func traceSupplementRequestedTargetProfile(ctx *types.BusContext) *types.RuntimeTargetProfile {
	if ctx == nil {
		return nil
	}
	if ctx.AnalysisIR != nil && ctx.AnalysisIR.RequestModel.RuntimeTargetProfile != nil {
		return ctx.AnalysisIR.RequestModel.RuntimeTargetProfile
	}
	if ctx.Mutable != nil {
		if rm := ctx.Mutable.RequestModel(); rm != nil {
			return rm.RuntimeTargetProfile
		}
	}
	return nil
}

func traceSupplementCursorTargetAllowed(ctx *types.BusContext) bool {
	allowed := false
	collect := func(rm *types.RequestModel) {
		if rm == nil || allowed {
			return
		}
		if _, _, ok := rm.RuntimeArtifactScopeProfile.ExplicitTimeWindow(); ok {
			allowed = true
			return
		}
		if rm.Intent == types.IntentRootCause ||
			rm.Predicates.IsDiagnosticQuestion ||
			rm.DiagnosticProfile.RequiresDiagnosticRootCause() {
			allowed = true
			return
		}
		switch types.ResolveQuestionFamily(*rm) {
		case types.QFRootCauseTrace, types.QFCallChain:
			allowed = true
		}
	}
	if ctx != nil && ctx.AnalysisIR != nil {
		collect(&ctx.AnalysisIR.RequestModel)
	}
	if ctx != nil && ctx.Mutable != nil {
		collect(ctx.Mutable.RequestModel())
	}
	return allowed
}

func traceSupplementRuntimeTargetScope(kind types.RuntimeTargetKind) string {
	switch types.NormalizeRuntimeTargetKind(kind) {
	case types.RuntimeTargetKindProcess:
		return tracequery.TargetScopeProcess
	case types.RuntimeTargetKindThread:
		return tracequery.TargetScopeThread
	default:
		return ""
	}
}

// traceSupplementValueObservationCount — R3B-C2 (§13.8, 2026-07-25): the
// disclosure line claimed "成文前确定性补跑 根因排序…" on the fifth replay
// while the compiled ledger held zero causal rows — a Success=true view whose
// observations are ONLY the diagnostic family (selector mismatch, lifecycle
// suppression) is an empty value account and must disclose itself as one.
func traceSupplementValueObservationCount(result types.ToolResult) int {
	census := traceSupplementViewFamilyCensus(result)
	return census.RootCauseRows + census.WakeupChainRows + census.TargetStateRows +
		census.CriticalBlockingRows + census.OtherRows
}

// traceSupplementViewFamilyCensus — AUD-02 (§14.3, 2026-07-25): the typed
// per-view family account behind the value total. A mixed N>0 cannot carry
// the §13.9 discrimination (states/census rows are value rows too); the
// root-cause family count is what the sixth replay reads. Exact predicate
// closed sets, no text heuristics.
func traceSupplementViewFamilyCensus(result types.ToolResult) types.TraceSupplementViewFamilyCensus {
	var census types.TraceSupplementViewFamilyCensus
	for _, observation := range result.Observations {
		predicate := strings.TrimSpace(observation.Predicate)
		switch {
		case predicate == "thread_selector_exact_name_mismatch" || predicate == "thread_incarnation_suppression":
			census.DiagnosticRows++
		case strings.HasPrefix(predicate, "root_cause_"):
			census.RootCauseRows++
		case predicate == "wakeup_chain" || predicate == "wakeup_chain_edge" ||
			predicate == "wakeup_causal_impact" || predicate == "wakeup_causal_aggregate" ||
			predicate == "wakeup_edge_census":
			census.WakeupChainRows++
		case predicate == "target_window_states" ||
			predicate == "target_cpu_running" ||
			predicate == "target_window_wait_occurrences" ||
			predicate == "target_window_wait_occurrence":
			census.TargetStateRows++
		case predicate == "critical_blocking":
			census.CriticalBlockingRows++
		default:
			census.OtherRows++
		}
	}
	return census
}

// traceSupplementParseThreadLabel parses the two precise thread-label shapes
// into (pid, bare name): the bracket form "name [pid]" and the hyphen form
// "name-pid" (same digit/letter/bound rules as the entities fallback). Any
// other shape returns ok=false — the label is kept verbatim, never guessed.
func traceSupplementParseThreadLabel(label string) (int, string, bool) {
	parse := func(name, digits string) (int, string, bool) {
		name = strings.TrimSpace(name)
		if name == "" || !strings.ContainsFunc(name, unicode.IsLetter) {
			return 0, "", false
		}
		if digits == "" || len(digits) > 7 ||
			strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, "", false
		}
		pid, err := strconv.Atoi(digits)
		if err != nil || pid <= 0 || pid > traceQueryMaxInheritedPID {
			return 0, "", false
		}
		return pid, name, true
	}
	if strings.HasSuffix(label, "]") {
		if i := strings.LastIndex(label, "["); i > 0 {
			if pid, name, ok := parse(label[:i], label[i+1:len(label)-1]); ok {
				return pid, name, true
			}
		}
	}
	if i := strings.LastIndexByte(label, '-'); i > 0 && i < len(label)-1 {
		if pid, name, ok := parse(label[:i], label[i+1:]); ok {
			return pid, name, true
		}
	}
	return 0, "", false
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
	targetScope := ""
	for _, target := range lane {
		scope := strings.TrimSpace(target.TargetScope)
		if scope != "" {
			if targetScope == "" {
				targetScope = scope
			} else if targetScope != scope {
				return traceQueryRequestTarget{}, false
			}
		}
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
		return traceQueryRequestTarget{
			PID: pid, Thread: thread, Source: lane[0].Source, TargetScope: targetScope,
		}, true
	}
	if threadOnly != "" {
		if targetScope == tracequery.TargetScopeProcess {
			return traceQueryRequestTarget{}, false
		}
		return traceQueryRequestTarget{
			Thread: threadOnly, Source: lane[0].Source, TargetScope: targetScope,
		}, true
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
// engine never guesses a default window; the G4-ENGINE D-state windowless
// fallback is NOT a window derivation — it omits the bounds entirely and the
// engine's whole-trace default applies, see RunTraceQuerySystemSupplement).
// Three-lane ladder, each lane keyed
// on the typed canonical view enum. Within the anchor lane, tolerance-equal
// windows agree; otherwise exactly one RECORDED window must enclose all
// others. Every other disagreement skips — never last-wins, never
// majority/frequency, never a synthetic union (F1 micro-probe precedent):
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
		if window, ok := traceSupplementConsistentWindow(anchorLane); ok {
			return window, true
		}
		return traceSupplementUniqueEnclosingWindow(anchorLane)
	}
	if len(statsLane) > 0 {
		return traceSupplementConsistentWindow(statsLane)
	}
	return traceSupplementConsistentWindow(windows)
}

// traceSupplementUniqueEnclosingWindow elects one RECORDED anchor window
// only when it contains every other anchor window. Tolerance-equal duplicate
// outers are one authority. Incomparable outers remain ambiguous; no union is
// synthesized and no call-order/frequency heuristic participates.
func traceSupplementUniqueEnclosingWindow(windows []types.TraceQueryCallWindow) (types.TraceQueryCallWindow, bool) {
	var candidates []types.TraceQueryCallWindow
	tol := types.TraceCausalProjectionSameWindowToleranceS
	for _, candidate := range windows {
		containsAll := true
		for _, other := range windows {
			if candidate.TimeStart > other.TimeStart+tol ||
				candidate.TimeEnd < other.TimeEnd-tol {
				containsAll = false
				break
			}
		}
		if !containsAll {
			continue
		}
		duplicate := false
		for _, current := range candidates {
			if absFloat(candidate.TimeStart-current.TimeStart) <= tol &&
				absFloat(candidate.TimeEnd-current.TimeEnd) <= tol {
				duplicate = true
				break
			}
		}
		if !duplicate {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) != 1 {
		return types.TraceQueryCallWindow{}, false
	}
	return candidates[0], true
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
	TargetScope  string  `json:"target_scope,omitempty"`
	TimeStart    float64 `json:"time_start"`
	TimeEnd      float64 `json:"time_end"`
	TraceFlavor  string  `json:"trace_flavor,omitempty"`
	CoreTopology string  `json:"core_topology,omitempty"`
	Platform     string  `json:"platform,omitempty"`
}

// traceSupplementWindowlessCallParams is the G4-ENGINE fallback call shape
// (2026-07-20, §29.145 filing): one windowed-view call with NO time bounds —
// the fields are deliberately absent (omitting them keeps Execute's
// optional-window semantics = the engine's own whole-trace default window; a
// literal 0 would be a claimed bound, the C-lite precedent).
type traceSupplementWindowlessCallParams struct {
	View         string `json:"view"`
	PID          int    `json:"pid,omitempty"`
	Thread       string `json:"thread,omitempty"`
	TraceFlavor  string `json:"trace_flavor,omitempty"`
	CoreTopology string `json:"core_topology,omitempty"`
	Platform     string `json:"platform,omitempty"`
}

func traceSupplementTargetScopeForView(target traceQueryRequestTarget, view string) string {
	if target.TargetScope != tracequery.TargetScopeProcess {
		return ""
	}
	switch tracequery.CanonicalViewName(view) {
	case "span_window", "frame_window", "render_pipeline", "frame_timeline",
		"frame_flow", "frame_root_cause_bundle":
		return tracequery.TargetScopeProcess
	default:
		return ""
	}
}

// traceSupplementMarshalWindowlessFallbackParams builds the G4-ENGINE
// fallback call's wire bytes. Time bounds are deliberately ABSENT (the
// whole-trace engine default; a literal 0 would be a claimed bound) — the
// wire-shape pin asserts the marshaled params carry no time_start/time_end
// key at all.
func traceSupplementMarshalWindowlessFallbackParams(view string, target traceQueryRequestTarget, callWindows []types.TraceQueryCallWindow) ([]byte, error) {
	return json.Marshal(traceSupplementWindowlessCallParams{
		View:         view,
		PID:          target.PID,
		Thread:       target.Thread,
		TraceFlavor:  traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.TraceFlavor }),
		CoreTopology: traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.CoreTopology }),
		Platform:     traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.Platform }),
	})
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

// traceSupplementDStateFamilyHit reports whether the request's TYPED
// analyzer keyword/entity face names the D-state / uninterruptible /
// blocked_reason family (G4-ENGINE, 2026-07-20 —
// traceSupplementVsyncFamilyHit 同构: reads only analyzer-emitted typed
// lists, never RawRequest; precise verbatim tokens):
//   - substring hits: unambiguous family words ("blocked_reason" must keep
//     hitting inside "sched_blocked_reason" — never boundary-checked);
//   - boundary-checked hits (返工 P2-C, 2026-07-20): the connected ASCII
//     forms "iowait"/"io_wait"/"d-state" are word-bounded — a plain
//     substring falsely armed on audio_wait / audiowait / radio_wait /
//     card-state hosts (the neighbor byte must not be [a-z0-9_]);
//   - exact hits: short spaced forms whose substring form would false-fire
//     ("d state" ⊂ "thread state", "io wait" ⊂ "audio wait" — kept
//     exact-only, the vsync arm's "frame"⊂"framework" precedent).
//
// A miss is a silent skip (宁漏勿假指) — the only consequence of the gate is
// whether the deterministic windowless fallback below ADDS evidence; it
// never blocks or re-judges anything.
func traceSupplementDStateFamilyHit(ctx *types.BusContext) bool {
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
	substrings := []string{"d状态", "d 状态", "不可中断", "uninterruptible", "io等待", "io 等待", "blocked_reason", "sched_blocked"}
	bounded := []string{"iowait", "io_wait", "d-state"}
	exact := map[string]bool{"d state": true, "io wait": true}
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
		for _, token := range bounded {
			if traceSupplementTokenWithWordBoundary(lower, token) {
				return true
			}
		}
	}
	return false
}

// traceSupplementTokenWithWordBoundary reports whether token occurs in s
// (already lowered) with NO [a-z0-9_] byte touching either side — the P2-C
// word-boundary carve for connected ASCII family tokens. CJK neighbors are
// multi-byte (≥0x80) and therefore valid boundaries by construction
// ("等待iowait严重" hits; "audio_wait" does not).
func traceSupplementTokenWithWordBoundary(s, token string) bool {
	for from := 0; ; {
		i := strings.Index(s[from:], token)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(token)
		beforeOK := start == 0 || !traceSupplementWordByte(s[start-1])
		afterOK := end == len(s) || !traceSupplementWordByte(s[end])
		if beforeOK && afterOK {
			return true
		}
		from = start + 1
	}
}

func traceSupplementWordByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// traceSupplementDStateFallbackWanted gates the G4-ENGINE windowless
// whole-trace fallback: the D-state family is named on the typed analyzer
// face AND the compiled ledger is missing the census or rank family (the
// two faces carrying the blocked_reason↔D-segment typed pairing — the
// pid-keyed census inventory and the self D/IO seat Σ). Called only after
// target derivation succeeded and window derivation failed.
func traceSupplementDStateFallbackWanted(ctx *types.BusContext, families traceSupplementFamilyPresence) bool {
	if traceSupplementNarrowDStateQuestion(ctx) {
		return !families.WindowStates || !families.BlockedReasonCensus
	}
	// Only legacy RequestModels without the typed question profile reach the
	// vocabulary compatibility detector. New records are decided entirely by
	// schema-validated fact-family enums above.
	if !traceSupplementDStateFamilyHit(ctx) {
		return false
	}
	return !families.BlockedReasonCensus || !families.Rank
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
	// A physical input-admission terminal means the investigation has no
	// authorized trace source. The system supplement shares that authority
	// boundary with model-dispatched tools; bypassing the run latch here can
	// publish evidence that contradicts the terminal repair.
	if _, terminal := ctx.Mutable.TraceInputAdmissionTerminal(types.StageExplore); terminal {
		return skip(types.TraceSupplementReasonInputAdmissionTerminal)
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
	requestedArtifactScope := traceSupplementRequestedArtifactScope(ctx)
	target, targetSource, targetOK := traceSupplementDeriveTarget(ctx)
	families := traceSupplementFamiliesForRequestedScope(preLedger, requestedArtifactScope, target, targetOK)
	frameFamily := traceSupplementVsyncFamilyHit(ctx)
	views := traceSupplementViewsForRequest(ctx, families, frameFamily, traceSupplementFrameEvidencePresent(input))
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
	if !targetOK {
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
	requestedFullArtifact := requestedArtifactScope.FullArtifact() && traceSupplementNarrowDStateQuestion(ctx)
	if start, end, explicit := requestedArtifactScope.ExplicitTimeWindow(); explicit {
		window = types.TraceQueryCallWindow{
			View:      "requested_artifact_scope",
			TimeStart: start,
			TimeEnd:   end,
		}
		ok = true
	}
	if requestedFullArtifact {
		window = types.TraceQueryCallWindow{}
	}
	// G4-ENGINE (2026-07-20, §29.145 filing "blocked_reason↔D 段 typed 配对"
	// engine lane): on the derivation-failure family (the event_search-only
	// c2 shape — locator probes record no consistent analysis window) a
	// D-state-family question loses BOTH pairing faces (pid-keyed
	// blocked_reason census + self D/IO seat Σ) to a silent skip, and the
	// census-consumption teaching has no census to bind. When the typed
	// analyzer face names the family, run ONE windowless root_cause_rank
	// instead — the engine's own whole-trace default window, each face
	// carrying its selected_window verbatim (no derived-window claim; the
	// span budget below is defined over DERIVED windows only, so the
	// cold-byte budget and the SUPP-CANCEL duration deadline are this
	// lane's fuses). 禁猜 stands for every other question family: the
	// silent skip is unchanged when the gate misses.
	windowlessFallback := false
	windowlessReason := ""
	if !ok && !requestedFullArtifact {
		reason := types.TraceSupplementReasonWindowInconsistent
		if len(callWindows) == 0 {
			reason = types.TraceSupplementReasonNoTypedWindow
		}
		// NW-02 判词: a frame investigation is meaningless without a window.
		// When the frame bundle was selected (frame family hit, frame evidence
		// absent), the G4 windowless generic rank must NOT impersonate it —
		// the honest exit is the typed skip (census-lite lane unaffected).
		frameBundleSelected := len(views) == 1 && views[0] == "frame_root_cause_bundle"
		if !frameBundleSelected && traceSupplementDStateFallbackWanted(ctx, families) {
			windowlessFallback = true
			windowlessReason = reason
			if traceSupplementNarrowDStateQuestion(ctx) {
				views = []string{"window_stats"}
			} else {
				views = []string{"root_cause_rank"}
			}
			logging.Info("[trace_supplement] windowless d-state fallback armed reason=%s (whole-trace engine default window; typed D-state family hit, census/rank family absent)", reason)
		} else {
			if frameBundleSelected {
				logging.Info("[trace_supplement] skip reason=%s (frame bundle selected; a windowless generic fallback may not substitute for the frame investigation)", reason)
			}
			if censusLiteWanted && runTraceSupplementCensusLite(execCtx, path, sourceLabel, reason, &out) {
				return out
			}
			return skip(reason)
		}
	}
	// P1 window-span budget gate: the warm lane's view cost scales with
	// in-window events, so an over-budget span skips the WHOLE supplement
	// before any engine call — with an honest answer-side disclosure (the
	// user asked about that window; the report must say the supplement did
	// not run for it) instead of a silent truncation or a guessed sub-window.
	if span := window.TimeEnd - window.TimeStart; !requestedFullArtifact && span > traceSupplementMaxWindowSpanS {
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
		if _, _, explicit := requestedArtifactScope.ExplicitTimeWindow(); explicit {
			meta.RequestedArtifactScope = types.RuntimeArtifactScopeExplicitWindow
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
	valueObservations := make([]int, 0, len(views))
	valueFamilies := make([]types.TraceSupplementViewFamilyCensus, 0, len(views))
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
		var raw []byte
		var err error
		if windowlessFallback || requestedFullArtifact {
			raw, err = traceSupplementMarshalWindowlessFallbackParams(view, target, callWindows)
			if err == nil && traceSupplementFallbackParamsHook != nil {
				traceSupplementFallbackParamsHook(raw)
			}
		} else {
			raw, err = json.Marshal(traceSupplementCallParams{
				View:         view,
				PID:          target.PID,
				Thread:       target.Thread,
				TargetScope:  traceSupplementTargetScopeForView(target, view),
				TimeStart:    window.TimeStart,
				TimeEnd:      window.TimeEnd,
				TraceFlavor:  traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.TraceFlavor }),
				CoreTopology: traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.CoreTopology }),
				Platform:     traceSupplementUnanimousEnum(callWindows, func(w types.TraceQueryCallWindow) string { return w.Platform }),
			})
		}
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
		if requestedFullArtifact {
			logging.Info("[trace_supplement] view=%s elapsed=%s requested_scope=full_artifact windowless=true pid=%d thread=%q source=%s warm=%t (user-scope authority overrides narrower exploratory query windows)",
				view, callElapsed.Round(time.Millisecond), target.PID, target.Thread, sourceLabel, warm)
		} else if windowlessFallback {
			logging.Info("[trace_supplement] view=%s elapsed=%s windowless=true reason=%s pid=%d thread=%q source=%s warm=%t (whole-trace engine default window)",
				view, callElapsed.Round(time.Millisecond), windowlessReason, target.PID, target.Thread, sourceLabel, warm)
		} else {
			logging.Info("[trace_supplement] view=%s elapsed=%s window=%.6f..%.6f pid=%d thread=%q source=%s warm=%t",
				view, callElapsed.Round(time.Millisecond), window.TimeStart, window.TimeEnd, target.PID, target.Thread, sourceLabel, warm)
		}
		// 修复轮 件3 (2026-07-14): register the result's payload blobs on the
		// Q5-A escape-lane registry — the supplement bypasses the
		// AppendDispatchToolResult chokepoint that performs this for model
		// calls, and an unregistered ref is visible on the finalize feed but
		// unreadable (read_file/grep allow-gate).
		ctx.Mutable.RegisterTraceQueryBlobRefsFromToolResult(result)
		executed = append(executed, view)
		valueObservations = append(valueObservations, traceSupplementValueObservationCount(result))
		valueFamilies = append(valueFamilies, traceSupplementViewFamilyCensus(result))
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
			if requestedFullArtifact {
				meta.RequestedArtifactScope = types.RuntimeArtifactScopeFullArtifact
			} else if _, _, explicit := requestedArtifactScope.ExplicitTimeWindow(); explicit {
				meta.RequestedArtifactScope = types.RuntimeArtifactScopeExplicitWindow
			}
			if windowlessFallback {
				// G4-ENGINE: the canceled-only disclosure must speak the
				// windowless caliber, never a fabricated 0..0 window.
				meta.WindowlessFallback = true
				meta.WindowlessFallbackReason = windowlessReason
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
		// 返工 P3-③ (2026-07-20): on the G4 windowless-fallback shape no
		// WINDOWED view ever ran — the operator-log lane label must name the
		// true derivation-failure lane, not claim a windowed run existed.
		liteLane := types.TraceSupplementReasonWindowedCensusAbsent
		if windowlessFallback {
			liteLane = windowlessReason
		}
		if result, _, ok := traceSupplementExecuteCensusLite(execCtx, path, liteLane); ok {
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
		Views:                   executed,
		ViewValueObservations:   valueObservations,
		ViewObservationFamilies: valueFamilies,
		WindowStart:             window.TimeStart,
		WindowEnd:               window.TimeEnd,
		TargetPID:               target.PID,
		TargetThread:            target.Thread,
		TargetSource:            targetSource,
		ElapsedMS:               out.Elapsed.Milliseconds(),
	}
	if requestedFullArtifact {
		meta.RequestedArtifactScope = types.RuntimeArtifactScopeFullArtifact
	} else if _, _, explicit := requestedArtifactScope.ExplicitTimeWindow(); explicit {
		meta.RequestedArtifactScope = types.RuntimeArtifactScopeExplicitWindow
	}
	if windowlessFallback {
		meta.WindowlessFallback = true
		meta.WindowlessFallbackReason = windowlessReason
	}
	if censusLiteRan {
		// The lite adjunct is not a windowed view: meta.Views keeps ONLY the
		// windowed executions (the disclosure's windowed sentence must not
		// claim the whole-trace scan ran on the derived window); the lite
		// flag + pattern carry the adjunct's own disclosure clause.
		// AUD-01 (§14.2, 2026-07-25): the lite adjunct only ever appended to
		// `executed` — valueObservations/valueFamilies pair with the windowed
		// views alone, so trimming them here silently dropped the LAST
		// windowed view's count and resurrected the uncounted 确定性补跑
		// face C2 exists to kill. Views is the ONLY slice that gets the trim.
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
