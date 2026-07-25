package types

// system_trace_supplement.go — SUPP-CORE (DISPATCH-IND 批1, 2026-07-14):
// typed carriers for the post-explore deterministic trace_query supplement.
//
// Disease family (§29.55.5 件B / §29.59 F2 / §29.63 ruling): every trace
// answer face compiles from ONE pipeline whose only input is "whichever
// trace_query views the model happened to dispatch", so report quality
// varied run-to-run with model sampling. The supplement decouples core
// record-family PRESENCE from model sampling: after the model's completion
// is ACCEPTED (never re-opening collection — §29.60 completion-gate
// ownership), the system re-runs at most two engine views with fully TYPED
// parameters and merges the observations through a dedicated lane.
//
// Red lines honored here:
//   - precise signals only: boolean family presence, exactly-one typed
//     target, tolerance-equal typed call windows. Any missing signal =
//     fail-open skip with byte-identical output (禁猜).
//   - provenance honesty: supplement results ride their own MutableState
//     slot + ObservationLedgerInput field, and every compiled record is
//     stamped SystemSupplement=true (audit lane); the user-visible surface
//     is ONE total disclosure line (R5: no per-face labels).
//   - explore zero displacement: results never enter bus.ToolResults /
//     dispatchToolResults, and the explore-stage prompt feed excludes the
//     lane (internal/context/builder.go).

// TraceQueryCallWindow is one explicit, typed trace_query call window as the
// model authored it (normalized seconds). Recorded by the trace_query tool at
// Execute time for every strict-decoded call carrying both time_start and
// time_end; consumed only by the SUPP-CORE window derivation.
type TraceQueryCallWindow struct {
	// View is the canonical view name of the call (tracequery.CanonicalViewName).
	View string `json:"view,omitempty"`
	// TimeStart / TimeEnd are the call's normalized requested window bounds in
	// trace seconds (the engine's requested_window, not the lookup-tolerance
	// widened bounds).
	TimeStart float64 `json:"time_start,omitempty"`
	TimeEnd   float64 `json:"time_end,omitempty"`
	// TraceFlavor / CoreTopology / Platform echo the call's typed enum params
	// verbatim so the supplement can inherit them when unanimous across all
	// recorded calls (schema-typed values only; empty = engine default).
	TraceFlavor  string `json:"trace_flavor,omitempty"`
	CoreTopology string `json:"core_topology,omitempty"`
	Platform     string `json:"platform,omitempty"`
}

// SystemTraceSupplementMeta is the typed disclosure record for one executed
// supplement: which views ran, on which window/target, and how long the
// engine work took. It feeds the single answer-side disclosure caveat and
// the operator log line; it is never model-authored text.
type SystemTraceSupplementMeta struct {
	// Views are the canonical view names the supplement executed, in
	// execution order.
	Views []string `json:"views,omitempty"`
	// ViewValueObservations — R3B-C2 (§13.8, 2026-07-25): per-Views value
	// observation counts (total observations minus the diagnostic family).
	// The fifth replay disclosed "成文前确定性补跑 根因排序…" while the
	// ledger compiled ZERO causal rows — a Success=true view with an empty
	// value account must not read as a provenance claim. Disclosure only.
	ViewValueObservations []int `json:"view_value_observations,omitempty"`
	// SkipReason / SkippedViews carry the P1 budget-fuse disclosure lanes
	// (2026-07-14). SkipReason values come from the TraceSupplementReason*
	// closed set (trace_supplement_reasons.go — SUPP-HYG P3-4):
	// "window_span_exceeded" = the whole supplement was skipped before any
	// engine call because the derived window's span exceeded WindowBudgetS
	// (Views empty, SkippedViews = the planned view set);
	// "duration_budget_exceeded" = the duration budget fired (between views
	// after ≥1 completed view — Views = the kept completions, SkippedViews =
	// the remainder — or in-view via the SUPP-CANCEL deadline);
	// "canceled_by_caller" (SUPP-HYG P3-D) = the engine work was canceled by
	// the CALLER's context (user abort), not by the budget. All are HONEST
	// disclosed skips — unlike the silent fail-open family (no target / no
	// window), the user named this window, so the answer must say the
	// supplement did not (fully) run for it.
	SkipReason   string   `json:"skip_reason,omitempty"`
	SkippedViews []string `json:"skipped_views,omitempty"`
	// WindowBudgetS echoes the span budget for the window_span_exceeded
	// disclosure wording.
	WindowBudgetS float64 `json:"window_budget_s,omitempty"`
	// WindowStart / WindowEnd are the typed-derived query window (seconds).
	WindowStart float64 `json:"window_start,omitempty"`
	WindowEnd   float64 `json:"window_end,omitempty"`
	// TargetPID / TargetThread are the typed-derived target.
	TargetPID    int    `json:"target_pid,omitempty"`
	TargetThread string `json:"target_thread,omitempty"`
	// TargetSource names the typed lane the target came from
	// ("user" = user-source RuntimeTargets, "cursor" = the model's own
	// consistent explicit tool-call targets, "entities_fallback" = the
	// SUPP-TARGET §29.90.1 rung — the typed RuntimeTargets lane minted
	// nothing and ONE unambiguous thread-shaped `name-pid` entity from the
	// analyzer's typed entities list recovered the target).
	// Disclosure/audit only.
	TargetSource string `json:"target_source,omitempty"`
	// ElapsedMS is the total engine wall time of the executed views
	// (performance disclosure — log surface).
	ElapsedMS int64 `json:"elapsed_ms,omitempty"`
	// CensusLite (SA-F2 / C-lite, DISPATCH-IND 批4 + 修复轮 件2, 2026-07-14)
	// marks the windowless lightweight census arm: whenever the request's
	// typed analyzer keywords hit the vsync/frame family AND the compiled
	// ledger carries no generator-census record, the system runs ONE
	// whole-trace streaming event_search (single-pass scan, zero heavy
	// views) so the generator census reaches the evidence face — on the
	// derivation-failure lanes (S13), the families_present no-op, the
	// span-budget skip, the execution_failed lane, AND as an adjunct to a
	// windowed success whose executed views minted no census. Views carries
	// ONLY the windowed executions (empty on the lite-only lanes); the
	// disclosure renderer derives the lite clause from this flag + pattern.
	CensusLite bool `json:"census_lite,omitempty"`
	// CensusLitePattern is the literal search pattern the C-lite pass used
	// (audit/disclosure).
	CensusLitePattern string `json:"census_lite_pattern,omitempty"`
	// WindowlessFallback (G4-ENGINE, 2026-07-20, §29.145 filing) marks the
	// D-state/blocked_reason-family whole-trace fallback lane: window
	// derivation failed (the model's calls carried no consistent analysis
	// window — the event_search-only c2 shape), a typed target exists, and
	// the request's typed analyzer keyword/entity face names the D-state
	// family — so the supplement ran ONE root_cause_rank with NO time
	// bounds (the engine's own whole-trace default window; every minted
	// face carries its selected_window verbatim, so no derived-window claim
	// is ever made). WindowStart/WindowEnd stay zero on this lane; the
	// disclosure renderer derives the windowless clause from this flag.
	WindowlessFallback bool `json:"windowless_fallback,omitempty"`
	// WindowlessFallbackReason echoes the window-derivation failure that
	// authorized the fallback (TraceSupplementReason* closed set:
	// no_typed_window | window_inconsistent). Audit/disclosure only.
	WindowlessFallbackReason string `json:"windowless_fallback_reason,omitempty"`
	// CanceledViews (SUPP-CANCEL, 2026-07-14) lists the canonical view names
	// whose ENGINE RUN hit in-view cooperative cancellation under the
	// supplement's duration-budget deadline (the same
	// trace_supplement_max_duration_ms knob that drives the between-view
	// skip; in-view the remaining budget rides a context deadline into
	// tracequery.Run). A canceled view appears in Views too when it still
	// recorded ≥1 complete face; a canceled view with zero recorded
	// observations stays out of Views (a disclosed "re-run" with an empty
	// account would be a false provenance claim — 修复轮 件2 rule) but is
	// disclosed here (禁裸丢). Unfinished faces were discarded whole by the
	// engine (禁半账).
	CanceledViews []string `json:"canceled_views,omitempty"`
	// DurationBudgetS echoes the duration budget for the cancellation
	// disclosure wording (WindowBudgetS twin).
	DurationBudgetS float64 `json:"duration_budget_s,omitempty"`
}

// TraceViewCancellation is the ToolResult mirror of the trace engine's typed
// in-view cooperative-cancellation record (SUPP-CANCEL, 2026-07-14). See
// ToolResult.TraceViewCancellation for the publication contract. SUPP-HYG
// P3-D (2026-07-14): a context fire during the PARSE phase (index build,
// before any view work) mints this record too — View/Reason only,
// ScannedUnits/DiscardedFaces zero — so the cancellation in-presence signal
// is typed on both fire lanes and consumers never fall back to racy ambient
// context checks.
type TraceViewCancellation struct {
	// View is the canonical view name of the canceled run.
	View string `json:"view"`
	// Reason is the context error class: "deadline_exceeded" | "canceled".
	Reason string `json:"reason"`
	// ScannedUnits is the engine's cooperative sampling counter value at the
	// fire (scan units — indexed events / visit steps).
	ScannedUnits int64 `json:"scanned_units,omitempty"`
	// DiscardedFaces lists the result faces discarded whole because their
	// construction had not completed before the fire.
	DiscardedFaces []string `json:"discarded_faces,omitempty"`
}

// RecordTraceQueryCallWindow appends one typed call window to the run-scoped
// registry. Append-only; duplicates are kept (call order is the deterministic
// tiebreak for the supplement's representative-window pick).
func (m *MutableState) RecordTraceQueryCallWindow(w TraceQueryCallWindow) {
	if m == nil {
		return
	}
	if !(w.TimeEnd > w.TimeStart) || w.TimeEnd <= 0 || w.TimeStart < 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.traceQueryCallWindows = append(m.traceQueryCallWindows, w)
}

// TraceQueryCallWindows returns a snapshot of the recorded call windows in
// call order.
func (m *MutableState) TraceQueryCallWindows() []TraceQueryCallWindow {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.traceQueryCallWindows) == 0 {
		return nil
	}
	out := make([]TraceQueryCallWindow, len(m.traceQueryCallWindows))
	copy(out, m.traceQueryCallWindows)
	return out
}

// MarkSystemTraceSupplementAttempted flips the one-shot latch and reports
// whether THIS call was the first attempt. The supplement runs at most once
// per task; a false return means another attempt already happened (executed
// or fail-open skipped) and the caller must not re-run.
func (m *MutableState) MarkSystemTraceSupplementAttempted() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.systemTraceSupplementAttempted {
		return false
	}
	m.systemTraceSupplementAttempted = true
	return true
}

// SystemTraceSupplementAttempted reports the latch state (test/audit surface).
func (m *MutableState) SystemTraceSupplementAttempted() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemTraceSupplementAttempted
}

// SetSystemTraceSupplement atomically stores an executed supplement's results
// and disclosure meta, replacing any previous value, and bumps the answer
// surface revision so cached answer-surface plans recompile against the
// enriched ledger.
func (m *MutableState) SetSystemTraceSupplement(meta SystemTraceSupplementMeta, results []ToolResult) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	metaCopy := meta
	metaCopy.Views = append([]string(nil), meta.Views...)
	metaCopy.SkippedViews = append([]string(nil), meta.SkippedViews...)
	metaCopy.CanceledViews = append([]string(nil), meta.CanceledViews...)
	m.systemTraceSupplementMeta = &metaCopy
	m.systemTraceSupplementResults = append([]ToolResult(nil), results...)
	m.bumpAnswerSurfaceRevisionLocked()
}

// SystemTraceSupplementResults returns a snapshot of the supplement's tool
// results (empty when the supplement never executed).
func (m *MutableState) SystemTraceSupplementResults() []ToolResult {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.systemTraceSupplementResults) == 0 {
		return nil
	}
	out := make([]ToolResult, len(m.systemTraceSupplementResults))
	copy(out, m.systemTraceSupplementResults)
	return out
}

// SystemTraceSupplementMeta returns a copy of the executed supplement's
// disclosure meta, or nil when the supplement never executed.
func (m *MutableState) SystemTraceSupplementMeta() *SystemTraceSupplementMeta {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.systemTraceSupplementMeta == nil {
		return nil
	}
	out := *m.systemTraceSupplementMeta
	out.Views = append([]string(nil), m.systemTraceSupplementMeta.Views...)
	out.SkippedViews = append([]string(nil), m.systemTraceSupplementMeta.SkippedViews...)
	out.CanceledViews = append([]string(nil), m.systemTraceSupplementMeta.CanceledViews...)
	return &out
}

// ResetSystemTraceSupplementForExploreReopen clears the supplement lane AND
// the per-task attempt latch (修复轮 件1, 2026-07-14): every lane that
// REOPENS explore after the supplement ran (FallbackBackToExplore's
// ResetForFallback, the contract-violation requeue family) must call this so
// (a) no explore-visible consumer face — transcript observation checkpoints,
// retry summaries, the completion authority snapshot, parallel
// early-convergence, the tier1 drill arm — can see supplement records during
// the reopened exploration (zero-displacement invariant), and (b) the
// supplement re-mints at the NEXT explore→extract boundary against the
// reopened investigation's ledger and call windows. The typed call-window
// registry is deliberately PRESERVED (sunk model history, ResetForFallback's
// evidence-preserved doctrine); derivation re-runs over the full set with
// the same fail-open rules. Reports whether anything was cleared.
func (m *MutableState) ResetSystemTraceSupplementForExploreReopen() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.systemTraceSupplementAttempted &&
		m.systemTraceSupplementMeta == nil &&
		len(m.systemTraceSupplementResults) == 0 {
		return false
	}
	m.systemTraceSupplementAttempted = false
	m.systemTraceSupplementMeta = nil
	m.systemTraceSupplementResults = nil
	m.bumpAnswerSurfaceRevisionLocked()
	return true
}

// BeginSystemTraceSupplementExecution / EndSystemTraceSupplementExecution
// bracket the supplement's engine calls (修复轮 件4, 2026-07-14). While the
// flag is up, trace_query's Execute-side recorders (exploration-cursor
// RuntimeTarget write-back + the call-window registry) skip the supplement's
// own calls: the supplement's target came FROM those lanes, and writing it
// back would be a feedback loop that can flip the exactly-one target
// determination on a revisit round. The supplement runs single-threaded at
// the explore→extract boundary, so a plain flag is race-free.
func (m *MutableState) BeginSystemTraceSupplementExecution() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemTraceSupplementInProgress = true
}

func (m *MutableState) EndSystemTraceSupplementExecution() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.systemTraceSupplementInProgress = false
}

// SystemTraceSupplementInProgress reports whether the supplement's engine
// calls are currently executing (consumed by trace_query's Execute-side
// recorders).
func (m *MutableState) SystemTraceSupplementInProgress() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.systemTraceSupplementInProgress
}

// RegisterTraceQueryBlobRefsFromToolResult registers every blob ref one
// trace_query result carries (RawRef + per-observation payload/raw refs) on
// the Q5-A escape-lane registry (修复轮 件3, 2026-07-14). It is the
// out-of-chokepoint mirror of AppendDispatchToolResult's registration for
// producers outside the agent dispatch chain — RegisterTraceQueryBlobRef's
// documented audience. Without it the supplement's records reference payload
// blobs the finalize model can see but never open.
func (m *MutableState) RegisterTraceQueryBlobRefsFromToolResult(r ToolResult) {
	if m == nil {
		return
	}
	refs := traceQueryPublishedBlobRefsFromToolResult(r)
	if len(refs) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ref := range refs {
		m.registerTraceQueryBlobRefLocked(ref)
	}
}
