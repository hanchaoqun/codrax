package types

// trace_supplement_reasons.go — SUPP-HYG P3-4 (批4 立案, 2026-07-14): the
// typed CLOSED SET of skip/lane reasons for the post-explore deterministic
// trace_query supplement (internal/tool/trace_query_supplement.go).
//
// Disease: the reason family lived as bare string literals scattered across
// the production lane (skip calls / meta mints / census-lite lane labels),
// the disclosure consumer (answer_document_mutation_runtime.go SkipReason
// equality points), and the operator log lines — the same silent-typo class
// the NKR rich-note key registry closed (a drifted literal turns a disclosure
// branch or a log grep into wire silence, no compiler or test notices).
//
// Discipline (NKR 同构):
//   - every reason is a TraceSupplementReason* constant here; this table is
//     the single registry;
//   - all three faces (production mints, disclosure consumption, operator
//     logs) reference the constants — consumer-side literal equality points
//     are AST-pinned in internal/tool (trace_supplement_reason_pin_test.go);
//   - the registry golden test (trace_supplement_reasons_test.go) pins the
//     exact member set: adding a constant without registering it — or
//     registering a value without its constant — is red.
//
// Change protocol: a new reason lands as constant + registry row + golden row
// + its producer/consumer/log sites in ONE change.

// TraceSupplementReason* are the wire values carried by
// SystemTraceSupplementMeta.SkipReason / TraceQuerySupplementOutcome.SkipReason
// and by the census-lite lane labels (operator log surface). Untyped string
// constants by design: they assign and compare against the string-typed wire
// fields without conversion noise.
const (
	// TraceSupplementReasonFamiliesPresent — healthy no-op: every core record
	// family is already on the compiled ledger.
	TraceSupplementReasonFamiliesPresent = "families_present"
	// TraceSupplementReasonNoTypedTarget — target derivation failed (no
	// unambiguous typed runtime target; 禁猜 fail-open).
	TraceSupplementReasonNoTypedTarget = "no_typed_target"
	// TraceSupplementReasonNoTypedWindow — no explicit typed model-call window
	// was recorded (the engine never guesses a default window).
	TraceSupplementReasonNoTypedWindow = "no_typed_window"
	// TraceSupplementReasonWindowInconsistent — recorded windows disagree
	// beyond the shared same-window tolerance (never last-wins).
	TraceSupplementReasonWindowInconsistent = "window_inconsistent"
	// TraceSupplementReasonWindowSpanExceeded — the derived window's span
	// exceeds the warm-lane span budget; skipped before any engine call with
	// an honest answer-side disclosure.
	TraceSupplementReasonWindowSpanExceeded = "window_span_exceeded"
	// TraceSupplementReasonDurationBudgetExceeded — the supplement's one
	// wall-clock duration budget fired (between views, or in-view via the
	// SUPP-CANCEL context deadline).
	TraceSupplementReasonDurationBudgetExceeded = "duration_budget_exceeded"
	// TraceSupplementReasonColdBudgetExceeded — no warm engine index and the
	// trace file exceeds the cold byte budget.
	TraceSupplementReasonColdBudgetExceeded = "cold_budget_exceeded"
	// TraceSupplementReasonExecutionFailed — every planned view failed or was
	// rejected by the engine with zero recorded observations (silent
	// fail-open).
	TraceSupplementReasonExecutionFailed = "execution_failed"
	// TraceSupplementReasonDisabled — trace_supplement_enabled: false kill
	// switch.
	TraceSupplementReasonDisabled = "disabled"
	// TraceSupplementReasonNoAttachedTrace — source resolution found no
	// attached/unambiguous trace artifact.
	TraceSupplementReasonNoAttachedTrace = "no_attached_trace"
	// TraceSupplementReasonWindowedCensusAbsent — census-lite lane label: the
	// windowed views ran but none minted the generator census, so the
	// adjunct whole-trace scan ran.
	TraceSupplementReasonWindowedCensusAbsent = "windowed_census_absent"
	// TraceSupplementReasonCanceledByCaller — SUPP-HYG P3-D (2026-07-14): the
	// supplement's engine work was canceled by the CALLER's context
	// (context.Canceled — e.g. user abort riding the bus context), not by the
	// duration budget's deadline. Forked from
	// TraceSupplementReasonDurationBudgetExceeded so the disclosure never
	// blames a budget that did not fire.
	TraceSupplementReasonCanceledByCaller = "canceled_by_caller"
)

// traceSupplementReasonRegistry is the closed-set membership table. Keep in
// registration order = documentation order above.
var traceSupplementReasonRegistry = []string{
	TraceSupplementReasonFamiliesPresent,
	TraceSupplementReasonNoTypedTarget,
	TraceSupplementReasonNoTypedWindow,
	TraceSupplementReasonWindowInconsistent,
	TraceSupplementReasonWindowSpanExceeded,
	TraceSupplementReasonDurationBudgetExceeded,
	TraceSupplementReasonColdBudgetExceeded,
	TraceSupplementReasonExecutionFailed,
	TraceSupplementReasonDisabled,
	TraceSupplementReasonNoAttachedTrace,
	TraceSupplementReasonWindowedCensusAbsent,
	TraceSupplementReasonCanceledByCaller,
}

// TraceSupplementReasons returns the registered closed set in registration
// order (test/audit surface).
func TraceSupplementReasons() []string {
	return append([]string(nil), traceSupplementReasonRegistry...)
}

// TraceSupplementReasonRegistered reports closed-set membership.
func TraceSupplementReasonRegistered(reason string) bool {
	for _, registered := range traceSupplementReasonRegistry {
		if reason == registered {
			return true
		}
	}
	return false
}
