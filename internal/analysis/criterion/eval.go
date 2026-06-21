package criterion

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Eval evaluates a single Criterion against env. Unknown Kind is
// reported via Result.UnknownKind — callers at runtime should treat
// that as a programmer error (gate is supposed to catch it first).
func Eval(c types.Criterion, env Env) Result {
	k := Kind(c.Kind)
	if !IsRegistered(k) {
		return Result{UnknownKind: true, Kind: k, Expr: c.Expr,
			Detail: fmt.Sprintf("criterion kind %q is not registered", c.Kind)}
	}
	r := dispatch(k, c.Expr, env)
	r.Kind = k
	r.Expr = c.Expr
	return r
}

// EvalAll evaluates every criterion and returns:
//
//   - allOK=true when every criterion is Satisfied AND no unknown
//     kinds were seen.
//   - failed is the list of unsatisfied and unknown-kind results; a
//     caller can walk it to build a retry hint or panic on unknown.
//
// When any criterion has UnknownKind=true, allOK is false regardless
// of the other criteria — the IR is not trusted.
func EvalAll(cs []types.Criterion, env Env) (allOK bool, failed []Result) {
	allOK = true
	for _, c := range cs {
		r := Eval(c, env)
		if r.UnknownKind || !r.Satisfied {
			allOK = false
			failed = append(failed, r)
		}
	}
	return allOK, failed
}

// dispatch is the central switch. Each case is an independent
// evaluator — intentionally flat rather than a map+closure so every
// handler is grep-discoverable.
func dispatch(k Kind, expr string, env Env) Result {
	switch k {
	case KindSymbolPresent:
		return evalSymbolPresent(expr, env)
	case KindNoCallSites:
		return evalNoCallSites(expr, env)
	case KindAnswerSetBounded:
		return evalAnswerSetBounded(expr, env)
	case KindAnswerSetUnbounded:
		return evalAnswerSetUnbounded(env)
	case KindMultipleResolutionChains:
		return evalMultipleResolutionChains(env)
	case KindUserClauseUnresolved:
		return evalUserClauseUnresolved(expr, env)
	case KindUntrustedReachesSink:
		return evalUntrustedReachesSink(env)
	case KindInvariantBroken:
		return evalInvariantBroken(expr, env)
	case KindNoRelevantEvidence:
		return evalNoRelevantEvidence(env)
	case KindSignalPresent:
		return evalSignalPresent(expr, env)
	case KindHasEnoughFacts:
		return evalHasEnoughFacts(env)
	case KindAllHypothesesDecided:
		return evalAllHypothesesDecided(env)
	case KindContractSatisfied:
		return evalContractSatisfied(env)
	case KindBudgetExhausted:
		return evalBudgetExhausted(env)
	case KindEvidenceCount:
		return evalEvidenceCount(expr, env)
	case KindCitationCountGE:
		return evalCitationCountGE(expr, env)
	case KindExtractInputReady:
		return evalExtractInputReady(env)
	case KindSourceClassUniverseIncomplete:
		return evalSourceClassUniverseIncomplete(env)
	case KindContainsSymbol:
		return evalContainsSymbol(expr, env)
	case KindRegexMatch:
		return evalRegexMatch(expr, env)
	case KindCounterfactualBranchesDecided:
		return evalCounterfactualBranchesDecided(env)
	case KindRelationAbsent:
		return evalRelationAbsent(expr, env)
	case KindPlanReady:
		return evalPlanReady(expr, env)
	case KindPatchApplies:
		return evalPatchApplies(expr, env)
	case KindTestsPass:
		return evalTestsPass(expr, env)
	case KindNoRegression:
		return evalNoRegression(expr, env)
	case KindExternalArtifactDecoded:
		return evalExternalArtifactDecoded(expr, env)
	}
	return Result{UnknownKind: true,
		Detail: fmt.Sprintf("no handler for kind %q (internal bug: registered but unreachable)", k)}
}

// ── Write-mode evaluators (real implementations; B2 shipped them) ──
//
// Each evaluator short-circuits to Satisfied=true when its env slot
// is zero-valued — this preserves read-mode byte-identity under L3:
// read-mode pipelines never populate ChangePlan / ChangeReport /
// WriteClosure, so an accidentally-scheduled write criterion cannot
// produce a spurious false negative that would perturb the read
// pipeline's retry budget. Real bodies interrogate
// WriteClosure.AppliedSet (evalPatchApplies),
// ChangeReport.TestResults (evalTestsPass), and the
// BaselineReport vs ChangeReport diff (evalNoRegression).

// evalPlanReady is satisfied when the planner has emitted a
// ChangePlan and its PendingApplies queue is non-empty — i.e. there
// are concrete file-level changes staged for the apply stage to
// consume. Day-5 real implementation: reads
// env.WriteClosure.PendingApplies length, which emit_change_plan
// populates on every successful emission.
//
// Read-mode short-circuit: when env.WriteClosure is nil the caller
// is a read-mode pipeline and has no concept of "plan ready". The
// trivially-satisfied branch preserves the L1 byte-identity rule.
//
// Expr is optional; when set to a non-empty string the evaluator
// additionally enforces that at least that many pending applies
// exist (e.g. `>=3` or the integer `3`). B0 only supports bare
// integer thresholds — regex / glob comparators land with B1.
func evalPlanReady(expr string, env Env) Result {
	if env.WriteClosure == nil {
		return Result{Satisfied: true, Detail: "plan_ready: no WriteClosure attached (read mode); trivially satisfied"}
	}
	pending := env.WriteClosure.PendingApplies()
	if len(pending) == 0 {
		return Result{
			Satisfied: false,
			Detail:    "plan_ready: no pending applies — planner has not emitted a ChangePlan yet",
		}
	}
	// Optional threshold. "" or invalid int = no threshold check.
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Result{
			Satisfied: true,
			Detail:    fmt.Sprintf("plan_ready: %d pending applies", len(pending)),
		}
	}
	// Parse `>=N` / `>N` / bare int forms via the shared helper.
	// Other write evaluators reuse substring filtering on expr
	// instead of integer thresholds so they don't go through here.
	threshold, op, ok := parseIntThreshold(expr)
	if !ok {
		return Result{
			Satisfied: true, // tolerant: bad threshold syntax falls back to non-empty check
			Detail:    fmt.Sprintf("plan_ready: %d pending applies (threshold %q unparseable, treating as non-empty check)", len(pending), expr),
		}
	}
	got := len(pending)
	var pass bool
	switch op {
	case ">=":
		pass = got >= threshold
	case ">":
		pass = got > threshold
	case "<=":
		pass = got <= threshold
	case "<":
		pass = got < threshold
	case "==", "=":
		pass = got == threshold
	default:
		pass = got >= threshold
	}
	return Result{
		Satisfied: pass,
		Detail:    fmt.Sprintf("plan_ready: %d pending applies %s %d", got, op, threshold),
	}
}

// parseIntThreshold extracts a comparator + integer from a
// criterion Expr like `>=3`, `>5`, or `3`. Returns (value, op, ok).
// Used today by evalPlanReady; extracted so B1's wider write-mode
// evaluators can reuse without re-parsing.
func parseIntThreshold(expr string) (int, string, bool) {
	expr = strings.TrimSpace(expr)
	ops := []string{">=", "<=", "==", ">", "<", "="}
	for _, op := range ops {
		if strings.HasPrefix(expr, op) {
			n, err := strconv.Atoi(strings.TrimSpace(expr[len(op):]))
			if err != nil {
				return 0, "", false
			}
			return n, op, true
		}
	}
	n, err := strconv.Atoi(expr)
	if err != nil {
		return 0, "", false
	}
	return n, ">=", true
}

// evalPatchApplies is satisfied when every ChangeUnit in the current
// plan has landed in the worktree — i.e.
// `AppliedSet ⊇ plan.TargetPaths`. Read-mode short-circuits to
// satisfied because no plan exists to apply.
//
// Expr semantics:
//   - empty     — check all plan.TargetPaths
//   - non-empty — treat as a path substring; only target paths
//     containing the substring are checked (useful for a SuccessCriteria
//     that only cares about a subset of the plan, e.g. `only the
//     config file changes landed`). Full-glob support is B4.
func evalPatchApplies(expr string, env Env) Result {
	if env.ChangePlan == nil {
		return Result{Satisfied: true, Detail: "patch_applies: no ChangePlan in env (read mode or pre-plan dispatch); trivially satisfied"}
	}
	if env.WriteClosure == nil {
		return Result{Satisfied: true, Detail: "patch_applies: no WriteClosure in env; trivially satisfied"}
	}
	applied := env.WriteClosure.AppliedSet()
	targets := env.ChangePlan.TargetPaths
	filter := strings.TrimSpace(expr)
	var considered, missing []string
	for _, p := range targets {
		if filter != "" && !strings.Contains(p, filter) {
			continue
		}
		considered = append(considered, p)
		if !applied[p] {
			missing = append(missing, p)
		}
	}
	if len(considered) == 0 {
		return Result{
			Satisfied: true,
			Detail:    fmt.Sprintf("patch_applies: filter %q matched 0/%d target paths; trivially satisfied", filter, len(targets)),
		}
	}
	if len(missing) == 0 {
		return Result{
			Satisfied: true,
			Detail:    fmt.Sprintf("patch_applies: %d/%d target paths applied", len(considered), len(targets)),
		}
	}
	return Result{
		Satisfied: false,
		Detail: fmt.Sprintf("patch_applies: %d of %d target paths not yet applied: %s",
			len(missing), len(considered), strings.Join(missing, ", ")),
	}
}

// evalTestsPass is satisfied when every TestResult in the current
// ChangeReport has Passed=true. Read-mode / pre-verify short-
// circuits to satisfied because no report exists yet.
//
// Build-error fast-fail: when ChangeReport.BuildFailed is set, the
// criterion fails with a build-summary message regardless of any
// per-test counts. This avoids the misleading "1 of 1 tests failed:
// <synthetic-build-id>" output and surfaces "build failed before
// tests ran" directly to the retry-hint pipeline.
//
// Expr semantics:
//   - empty     — require every test in report to pass
//   - non-empty — treat as a substring filter on AssertionID;
//     only matching tests must pass (useful when a SuccessCriteria
//     names a specific test suite). Full regex is B4.
func evalTestsPass(expr string, env Env) Result {
	if env.ChangeReport == nil {
		return Result{Satisfied: true, Detail: "tests_pass: no ChangeReport in env (read mode or pre-verify); trivially satisfied"}
	}
	if env.ChangeReport.BuildFailed {
		summary := env.ChangeReport.FailureSummary
		if summary == "" {
			summary = "build failed before tests ran"
		}
		return Result{Satisfied: false, Detail: "tests_pass: " + summary}
	}
	filter := strings.TrimSpace(expr)
	report := env.ChangeReport
	var considered, failed []string
	for _, tr := range report.TestResults {
		if tr.Kind == types.TestResultKindBuildError {
			// Build-error rows are not unit tests; the BuildFailed
			// fast-fail above already handles them. Skip here so
			// they don't show up in the per-test rejection list.
			continue
		}
		if filter != "" && !strings.Contains(tr.AssertionID, filter) {
			continue
		}
		considered = append(considered, tr.AssertionID)
		if !tr.Passed {
			failed = append(failed, tr.AssertionID)
		}
	}
	if len(considered) == 0 {
		return Result{
			Satisfied: true,
			Detail:    fmt.Sprintf("tests_pass: filter %q matched 0/%d test results; trivially satisfied", filter, len(report.TestResults)),
		}
	}
	if len(failed) == 0 {
		return Result{
			Satisfied: true,
			Detail:    fmt.Sprintf("tests_pass: %d/%d tests passed", len(considered), len(report.TestResults)),
		}
	}
	// Cap the names list so a massive failure doesn't balloon the
	// retry-hint message. The full list is always recoverable from
	// Mutable.ChangeReport for richer surfaces.
	const maxNamesInDetail = 5
	shown := failed
	if len(shown) > maxNamesInDetail {
		shown = shown[:maxNamesInDetail]
	}
	return Result{
		Satisfied: false,
		Detail: fmt.Sprintf("tests_pass: %d of %d tests failed: %s%s",
			len(failed), len(considered),
			strings.Join(shown, ", "),
			func() string {
				if len(failed) > maxNamesInDetail {
					return fmt.Sprintf(" (+%d more)", len(failed)-maxNamesInDetail)
				}
				return ""
			}()),
	}
}

// evalNoRegression is satisfied when no test that passed in the
// baseline now fails, AND no MetricDelta exceeds its threshold.
// Read-mode / missing-baseline short-circuits to satisfied.
//
// Expr semantics:
//   - empty     — check all tests + metrics
//   - non-empty — substring filter on AssertionID / metric name
//
// Design note: pre-apply baseline capture is opt-in (the apply stage hook
// skips it when codrax.yaml has baseline_capture_enabled: false or
// no test runner was detected). Absence of a baseline is NOT a
// regression — if we can't compare, we can't assert regression.
func evalNoRegression(expr string, env Env) Result {
	if env.ChangeReport == nil {
		return Result{Satisfied: true, Detail: "no_regression: no ChangeReport (read mode or pre-verify); trivially satisfied"}
	}
	if env.BaselineReport == nil {
		return Result{Satisfied: true, Detail: "no_regression: no BaselineReport captured; regression check skipped (trivially satisfied)"}
	}
	filter := strings.TrimSpace(expr)
	// Build baseline index: AssertionID → Passed. Skip build-error
	// rows because they aren't comparable with unit tests in the
	// new structured Kind enum.
	baselinePassed := make(map[string]bool, len(env.BaselineReport.TestResults))
	for _, tr := range env.BaselineReport.TestResults {
		if tr.Kind == types.TestResultKindBuildError {
			continue
		}
		baselinePassed[tr.AssertionID] = tr.Passed
	}
	var regressed []string
	// When emit_test_results populated structured RegressionAssertions,
	// trust the LLM's classification — it has both halves of the diff
	// + the plan's TargetPaths to judge whether a failure was caused
	// by this change. The fallback baseline-vs-current map is the
	// conservative path for runs where the verifier didn't classify.
	if len(env.ChangeReport.RegressionAssertions) > 0 {
		for _, id := range env.ChangeReport.RegressionAssertions {
			if filter != "" && !strings.Contains(id, filter) {
				continue
			}
			regressed = append(regressed, id)
		}
	} else {
		for _, tr := range env.ChangeReport.TestResults {
			if tr.Kind == types.TestResultKindBuildError {
				continue
			}
			if filter != "" && !strings.Contains(tr.AssertionID, filter) {
				continue
			}
			if !tr.Passed && baselinePassed[tr.AssertionID] {
				regressed = append(regressed, tr.AssertionID)
			}
		}
	}
	// Metric regressions. MetricDelta.Threshold > 0 means operator
	// supplied a ceiling; skip metrics with zero threshold (not
	// configured). Regressed when (Current - Baseline) > Threshold.
	var metricRegressions []string
	for name, md := range env.ChangeReport.MetricDeltas {
		if filter != "" && !strings.Contains(name, filter) {
			continue
		}
		if md.Threshold <= 0 {
			continue
		}
		if (md.Current - md.Baseline) > md.Threshold {
			metricRegressions = append(metricRegressions,
				fmt.Sprintf("%s: %.3g→%.3g (+%.3g > %.3g)", name, md.Baseline, md.Current, md.Current-md.Baseline, md.Threshold))
		}
	}
	if len(regressed) == 0 && len(metricRegressions) == 0 {
		return Result{Satisfied: true, Detail: "no_regression: no test or metric regression detected"}
	}
	var parts []string
	if len(regressed) > 0 {
		const maxNames = 5
		shown := regressed
		suffix := ""
		if len(shown) > maxNames {
			shown = shown[:maxNames]
			suffix = fmt.Sprintf(" (+%d more)", len(regressed)-maxNames)
		}
		parts = append(parts, fmt.Sprintf("%d test(s) regressed: %s%s", len(regressed), strings.Join(shown, ", "), suffix))
	}
	if len(metricRegressions) > 0 {
		parts = append(parts, "metric regressions: "+strings.Join(metricRegressions, "; "))
	}
	return Result{
		Satisfied: false,
		Detail:    "no_regression: " + strings.Join(parts, " | "),
	}
}

// ── individual evaluators ─────────────────────────────────────────

func evalSymbolPresent(expr string, env Env) Result {
	sym := strings.TrimSpace(expr)
	if sym == "" {
		return Result{Satisfied: true, Detail: "empty symbol — treated as trivially satisfied"}
	}
	// Phase 6 stage 13 (2026-05-03) — typed equality replaces
	// substring search. Match the symbol against EvidenceItem's
	// typed identifier slots (Subject / Object / AnchorSymbol)
	// using lowercase equality OR NormalizedSurfaceSymbolTail
	// equality. The AnswerSymbol slate matches via Name equality.
	// The retired containsLower path matched substrings (so a
	// needle "Ctx" trivially passed on a slot value
	// "RequestContext"); the typed-equality path requires the
	// slot to NAME the symbol or its qualified-tail.
	needle := strings.ToLower(sym)
	needleTail := strings.ToLower(types.NormalizedSurfaceSymbolTail(sym))
	matchSlot := func(slot string) bool {
		if slot == "" {
			return false
		}
		low := strings.ToLower(slot)
		if low == needle {
			return true
		}
		if needleTail != "" {
			if low == needleTail {
				return true
			}
			if tail := strings.ToLower(types.NormalizedSurfaceSymbolTail(slot)); tail != "" && tail == needleTail {
				return true
			}
		}
		return false
	}
	for _, e := range env.Evidence {
		if matchSlot(e.Subject) || matchSlot(e.Object) || matchSlot(e.AnchorSymbol) {
			return Result{Satisfied: true, Detail: "symbol found in evidence typed slot"}
		}
	}
	for _, s := range env.AnswerSymbols {
		if matchSlot(s.Name) {
			return Result{Satisfied: true, Detail: "symbol found in answer slate"}
		}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("symbol %q not present in any typed evidence slot or answer slate", sym)}
}

func evalNoCallSites(expr string, env Env) Result {
	sym := strings.TrimSpace(expr)
	if sym == "" {
		return Result{Satisfied: true, Detail: "empty symbol — trivially no call sites"}
	}
	// Phase 6 stage 13 (2026-05-03) — typed evidence pool replaces
	// substring search on free-form ToolResult.Summary text. A
	// call site is any EvidenceItem with AnchorKind=AnchorCall
	// whose typed slots (Subject / Object / AnchorSymbol) name
	// the symbol — equality / tail-equality, not substring. The
	// retired ToolResult.Summary scan matched grep / read_file
	// raw output prose, which produced false positives for any
	// occurrence of the symbol name in nearby comments / imports
	// / docstrings rather than actual call structures.
	needle := strings.ToLower(sym)
	needleTail := strings.ToLower(types.NormalizedSurfaceSymbolTail(sym))
	matchSlot := func(slot string) bool {
		if slot == "" {
			return false
		}
		low := strings.ToLower(slot)
		if low == needle {
			return true
		}
		if needleTail != "" {
			if low == needleTail {
				return true
			}
			if tail := strings.ToLower(types.NormalizedSurfaceSymbolTail(slot)); tail != "" && tail == needleTail {
				return true
			}
		}
		return false
	}
	hits := 0
	for _, e := range env.Evidence {
		if e.AnchorKind != types.AnchorCall {
			continue
		}
		if matchSlot(e.Subject) || matchSlot(e.Object) || matchSlot(e.AnchorSymbol) {
			hits++
		}
	}
	if hits == 0 {
		return Result{Satisfied: true, Detail: "no AnchorKind=call evidence items name the symbol"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d typed call-site evidence item(s) name %q", hits, sym)}
}

func evalAnswerSetBounded(expr string, env Env) Result {
	n := len(env.AnswerSymbols)
	op, threshold, ok := parseComparison(expr)
	if !ok {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("malformed comparison %q (expect e.g. <=5, >=2, ==3)", expr)}
	}
	satisfied := compareInt(n, op, threshold)
	return Result{Satisfied: satisfied,
		Detail: fmt.Sprintf("|answer_symbols|=%d %s %d", n, op, threshold)}
}

func evalAnswerSetUnbounded(env Env) Result {
	if len(env.AnswerChains) > 50 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("answer chains exploded to %d (>50)", len(env.AnswerChains))}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("answer chains still bounded (%d ≤ 50)", len(env.AnswerChains))}
}

func evalMultipleResolutionChains(env Env) Result {
	n := 0
	for _, e := range env.Evidence {
		if e.Predicate == "config_resolution" {
			n++
		}
	}
	if n > 1 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("%d config_resolution evidence items", n)}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("only %d config_resolution evidence items", n)}
}

func evalUserClauseUnresolved(expr string, env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR available"}
	}
	needle := strings.TrimSpace(expr)
	for _, a := range env.IR.RequestModel.Ambiguities {
		if needle != "" && !strings.EqualFold(strings.TrimSpace(a.Clause), needle) {
			continue
		}
		if strings.TrimSpace(a.Resolution) == "" {
			return Result{Satisfied: true,
				Detail: fmt.Sprintf("ambiguity %q has no resolution", a.Clause)}
		}
	}
	return Result{Satisfied: false, Detail: "all matching ambiguities resolved"}
}

func evalUntrustedReachesSink(env Env) Result {
	// Phase 6 stage 13 (2026-05-03) — retired the hardcoded
	// keyword table {"untrusted", "sink", "boundary"} that was
	// substring-matched against e.Predicate + e.Summary. That
	// table was a direct red-line violation of
	// `feedback_no_custom_keyword_matching.md` — keyword tables
	// don't generalise across domains and a synonym
	// ("trustless", "exit point", "crossing") trivially evaded
	// the gate. The criterion now reads ONLY typed signals:
	//
	//   - RiskMatrix.Security.Level >= 4 (LLM-emitted risk dim)
	//   - len(env.Evidence) > 0  (explorer captured *something*)
	//
	// The keyword scan was a gloss on top of the level check; in
	// every corpus run sampled it added zero precision over the
	// "evidence emitted at all" signal because Security>=4 only
	// lands when the LLM identified an untrusted-input concern,
	// and any evidence emitted under that flag IS by construction
	// about that concern. The retired path falsely rejected
	// runs where the LLM phrased its evidence Predicate as
	// "trustless_input_passed_to_db" (no listed keyword) and
	// falsely accepted runs where any unrelated evidence's
	// Summary happened to contain "boundary" as a verb.
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR"}
	}
	if env.IR.RequestModel.RiskMatrix.Security.Level < 4 {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("security risk level %d < 4", env.IR.RequestModel.RiskMatrix.Security.Level)}
	}
	if len(env.Evidence) == 0 {
		return Result{Satisfied: false, Detail: "security risk >= 4 but no evidence captured under that flag"}
	}
	return Result{Satisfied: true, Detail: fmt.Sprintf("security risk >= %d and %d evidence item(s) captured", 4, len(env.Evidence))}
}

func evalInvariantBroken(expr string, env Env) Result {
	// Phase 6 stage 13 (2026-05-03) — retired the secondary
	// substring filter on e.Summary / e.Subject. The Predicate
	// enum equality (`invariant_violation` / `invariant_broken`)
	// is a precise typed signal; the substring needle was a
	// parameter-level filter that masquraded as a refinement but
	// in practice false-rejected legitimate invariant violations
	// whose Subject / Summary phrased the invariant differently
	// from the expr. If callers need to match a specific
	// invariant, they should pass it as a typed Subject equality
	// check (matchSlot equality, like evalSymbolPresent).
	needle := strings.ToLower(strings.TrimSpace(expr))
	for _, e := range env.Evidence {
		pred := strings.ToLower(e.Predicate)
		if pred != "invariant_violation" && pred != "invariant_broken" {
			continue
		}
		if needle == "" {
			return Result{Satisfied: true, Detail: "invariant violation in evidence"}
		}
		// Typed-slot equality — same discipline as
		// evalSymbolPresent.
		needleTail := strings.ToLower(types.NormalizedSurfaceSymbolTail(needle))
		matchSlot := func(slot string) bool {
			if slot == "" {
				return false
			}
			low := strings.ToLower(slot)
			if low == needle {
				return true
			}
			if needleTail != "" {
				if low == needleTail {
					return true
				}
				if tail := strings.ToLower(types.NormalizedSurfaceSymbolTail(slot)); tail != "" && tail == needleTail {
					return true
				}
			}
			return false
		}
		if matchSlot(e.Subject) || matchSlot(e.Object) || matchSlot(e.AnchorSymbol) {
			return Result{Satisfied: true, Detail: "invariant violation in evidence (typed slot match)"}
		}
	}
	return Result{Satisfied: false, Detail: "no invariant_violation evidence"}
}

func evalNoRelevantEvidence(env Env) Result {
	if len(env.Evidence) == 0 {
		return Result{Satisfied: true, Detail: "no evidence items collected"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d evidence items present", len(env.Evidence))}
}

func evalSignalPresent(expr string, env Env) Result {
	switch strings.TrimSpace(expr) {
	case "has_enough_facts", "":
		return evalHasEnoughFacts(env)
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("unknown signal %q", expr)}
}

func evalHasEnoughFacts(env Env) Result {
	if env.Signals.HasEnoughFacts {
		return Result{Satisfied: true, Detail: "HasEnoughFacts=true"}
	}
	return Result{Satisfied: false, Detail: "HasEnoughFacts=false"}
}

func evalAllHypothesesDecided(env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR"}
	}
	undecided := 0
	for _, h := range env.IR.HypothesisSet {
		if h.Status == types.HypUnknown || h.Status == "" {
			undecided++
		}
	}
	if undecided == 0 {
		return Result{Satisfied: true, Detail: "all hypotheses decided"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d hypothesis(es) still unknown", undecided)}
}

func evalContractSatisfied(env Env) Result {
	if strings.TrimSpace(env.DraftAnswer) == "" {
		return Result{Satisfied: false, Detail: "no draft answer yet"}
	}
	if env.IR == nil {
		return Result{Satisfied: true, Detail: "draft exists; no IR to cross-check"}
	}
	if externalRuntimeCitationFloorWaived(env) {
		return Result{Satisfied: true, Detail: "external runtime artifact facts use typed external_observation carriers; repo citation floor waived"}
	}
	if env.IR.AnswerContract.CitationReq.Required && env.DraftCitations < env.IR.AnswerContract.CitationReq.MinCitations {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("citations %d < required %d", env.DraftCitations, env.IR.AnswerContract.CitationReq.MinCitations)}
	}
	return Result{Satisfied: true, Detail: "draft answer meets citation floor"}
}

func evalBudgetExhausted(env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: false, Detail: "no IR to read budget from"}
	}
	cap := env.IR.EvidencePlan.Budget.MaxReactIters
	if cap <= 0 {
		return Result{Satisfied: false, Detail: "no MaxReactIters set"}
	}
	if env.ReactItersUsed >= cap {
		return Result{Satisfied: true, Detail: fmt.Sprintf("react iters %d ≥ cap %d", env.ReactItersUsed, cap)}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("react iters %d < cap %d", env.ReactItersUsed, cap)}
}

func evalEvidenceCount(expr string, env Env) Result {
	op, threshold, ok := parseComparison(expr)
	if !ok {
		return Result{Satisfied: false, Detail: fmt.Sprintf("malformed comparison %q", expr)}
	}
	if artifactCount, waived := externalRuntimeEvidenceFloorWaived(env); waived {
		return Result{
			Satisfied: true,
			Detail: fmt.Sprintf(
				"external_observations=%d; repo evidence_count floor %s %d waived for observation-only artifact",
				artifactCount, op, threshold),
		}
	}
	n := len(env.Evidence)
	// Under the "soft" investigation-complete policy, the LLM's
	// explicit completion signal lowers the threshold to 1. The LLM
	// judged the evidence sufficient; we only insist on at least 1
	// item so we're not finalizing on an empty set.
	if env.InvestigationComplete && threshold > 1 && n >= 1 {
		return Result{
			Satisfied: true,
			Detail:    fmt.Sprintf("|evidence|=%d (investigation_complete override, original threshold %s %d)", n, op, threshold),
		}
	}
	return Result{
		Satisfied: compareInt(n, op, threshold),
		Detail:    fmt.Sprintf("|evidence|=%d %s %d", n, op, threshold),
	}
}

func evalExtractInputReady(env Env) Result {
	if len(env.Evidence) > 0 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("extract_input_ready: evidence_items=%d", len(env.Evidence))}
	}
	if len(env.AnswerChains) > 0 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("extract_input_ready: answer_chains=%d", len(env.AnswerChains))}
	}
	if len(env.AggregateFacts) > 0 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("extract_input_ready: aggregate_facts=%d", len(env.AggregateFacts))}
	}
	if artifactCount, waived := externalRuntimeEvidenceFloorWaived(env); waived && artifactCount > 0 {
		return Result{Satisfied: true, Detail: fmt.Sprintf("extract_input_ready: external_observations=%d", artifactCount)}
	}
	return Result{Satisfied: false, Detail: "extract_input_ready: no typed evidence, answer chains, aggregate facts, or external observations"}
}

func evalSourceClassUniverseIncomplete(env Env) Result {
	if !env.SourceInventoryActive {
		return Result{Satisfied: false, Detail: "source_class_universe_incomplete: no active source inventory"}
	}
	if env.SourceClassUniverseComplete {
		return Result{Satisfied: false, Detail: "source_class_universe_incomplete: universe complete"}
	}
	return Result{Satisfied: true, Detail: "source_class_universe_incomplete: active universe incomplete"}
}

func externalRuntimeEvidenceFloorWaived(env Env) (count int, waived bool) {
	if env.IR == nil || !env.IR.RequestModel.HasRuntimeArtifactWithoutRequiredCurrentSource() {
		if !env.ObservationOnlyCompletion {
			return 0, false
		}
		count = runtimeArtifactObservationCount(env.LogTriage, env.PerfTrace) + mcpResponseObservationCount(env.MCPResponses)
		if count <= 0 {
			return 0, false
		}
		return count, true
	}
	count = runtimeArtifactObservationCount(env.LogTriage, env.PerfTrace)
	if count <= 0 {
		return 0, false
	}
	return count, true
}

func runtimeArtifactObservationCount(logBundle *types.LogBundle, perfBundle *types.PerfBundle) int {
	count := 0
	if logBundle != nil {
		count += len(logBundle.Meta.Signals)
		count += len(types.LogBundleErrorTypes(logBundle))
		count += len(types.LogBundleErrorMessages(logBundle))
		count += len(logBundle.Observations)
		types.WalkLogFrames(logBundle, func(types.LogFrame) {
			count++
		})
	}
	if perfBundle != nil {
		count += len(perfBundle.Meta.Signals)
		count += len(perfBundle.Frames)
		count += len(perfBundle.Janks)
		count += len(perfBundle.Stalls)
		if perfBundle.Startup != nil {
			count++
		}
	}
	return count
}

func mcpResponseObservationCount(responses []types.MCPResponse) int {
	count := 0
	for _, resp := range responses {
		if !resp.Success {
			continue
		}
		if len(resp.Observations) > 0 {
			count += len(resp.Observations)
			continue
		}
		if resp.ResourceURI != "" || resp.LineStart > 0 || resp.Row > 0 || resp.JSONPointer != "" || resp.Selector != "" || resp.PayloadRef != "" || resp.RowSetRef != "" || resp.PageRef != "" {
			count++
		}
	}
	return count
}

func evalCitationCountGE(expr string, env Env) Result {
	threshold, err := strconv.Atoi(strings.TrimSpace(expr))
	if err != nil {
		return Result{Satisfied: false, Detail: fmt.Sprintf("malformed integer %q", expr)}
	}
	if externalRuntimeCitationFloorWaived(env) {
		return Result{Satisfied: true, Detail: "external runtime artifact facts use typed external_observation carriers; repo citation floor waived"}
	}
	if env.DraftCitations >= threshold {
		return Result{Satisfied: true, Detail: fmt.Sprintf("%d citations ≥ %d", env.DraftCitations, threshold)}
	}
	if env.IR != nil {
		if effective, ok := types.EffectiveCitationFloorForPrincipalMemberSets(threshold, env.AggregateFacts, &env.IR.RequestModel); ok && env.DraftCitations >= effective {
			return Result{Satisfied: true, Detail: fmt.Sprintf("%d citations ≥ principal-member floor %d (global floor %d capped by exact member set)", env.DraftCitations, effective, threshold)}
		}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("%d citations < %d", env.DraftCitations, threshold)}
}

func externalRuntimeCitationFloorWaived(env Env) bool {
	if env.LogTriage != nil && env.LogTriage.IsExternalSource() {
		return true
	}
	if env.PerfTrace != nil && env.PerfTrace.IsExternalSource() {
		return true
	}
	return false
}

func evalContainsSymbol(expr string, env Env) Result {
	needle := strings.TrimSpace(expr)
	if needle == "" {
		return Result{Satisfied: true, Detail: "empty needle"}
	}
	if strings.Contains(env.DraftAnswer, needle) {
		return Result{Satisfied: true, Detail: "needle found in draft"}
	}
	return Result{Satisfied: false, Detail: fmt.Sprintf("needle %q not in draft", needle)}
}

func evalRegexMatch(expr string, env Env) Result {
	if strings.TrimSpace(expr) == "" {
		return Result{Satisfied: true, Detail: "empty pattern"}
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return Result{Satisfied: false, Detail: fmt.Sprintf("invalid regex: %v", err)}
	}
	if re.MatchString(env.DraftAnswer) {
		return Result{Satisfied: true, Detail: "pattern matches draft"}
	}
	return Result{Satisfied: false, Detail: "pattern does not match draft"}
}

func evalCounterfactualBranchesDecided(env Env) Result {
	if env.IR == nil {
		return Result{Satisfied: true, Detail: "no IR"}
	}
	for _, h := range env.IR.HypothesisSet {
		if h.Status == types.HypUnknown || h.Status == "" {
			// Only require decisions on hypotheses bound to
			// counterfactual nodes. If no such binding exists the
			// criterion trivially holds.
			bound := false
			for _, n := range env.IR.TaskGraph.Nodes {
				if !n.IsCounterfactual {
					continue
				}
				for _, hid := range n.Hypotheses {
					if hid == h.ID {
						bound = true
						break
					}
				}
				if bound {
					break
				}
			}
			if bound {
				return Result{Satisfied: false,
					Detail: fmt.Sprintf("counterfactual-bound hypothesis %q still unknown", h.ID)}
			}
		}
	}
	return Result{Satisfied: true, Detail: "all counterfactual-bound hypotheses decided"}
}

// evalRelationAbsent is the falsification counterpart to the
// anchored-on-set hypothesis the HDP planner emits for multi-subject
// explain/trace questions (e.g. "A 和 B 的关系"). Expr is a
// comma-separated pair "A,B"; the criterion fires (rejected) when no
// evidence item mentions BOTH symbols in Subject / Object / Summary.
// A single evidence row tying the two together is sufficient to keep
// the relation hypothesis alive — we are only rejecting the cases
// where an entire turn's worth of evidence failed to find any bridge.
//
// Malformed Expr (missing comma, empty side) falls loudly: the
// criterion returns Satisfied=false with a Detail the retry hint can
// surface.
func evalRelationAbsent(expr string, env Env) Result {
	parts := strings.SplitN(strings.TrimSpace(expr), ",", 2)
	if len(parts) != 2 {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("malformed relation expr %q (expect \"A,B\")", expr)}
	}
	a := strings.TrimSpace(parts[0])
	b := strings.TrimSpace(parts[1])
	if a == "" || b == "" {
		return Result{Satisfied: false,
			Detail: fmt.Sprintf("malformed relation expr %q (empty side)", expr)}
	}
	// Phase 6 stage 13 (2026-05-03) — retired the
	// `Subject + Object + Summary + Predicate` blob substring
	// scan. A typed evidence-pair match is structurally a
	// (Subject, Object) or (Subject, AnchorSymbol) tuple equality
	// — the typed slots already encode "this row links symbol X
	// to symbol Y". The retired blob path matched substrings on
	// concatenated free-form fields, so an unrelated identifier
	// in a Summary string trivially satisfied the gate.
	matchSlot := func(slot, needle string) bool {
		if slot == "" || needle == "" {
			return false
		}
		lowSlot := strings.ToLower(slot)
		lowNeedle := strings.ToLower(needle)
		if lowSlot == lowNeedle {
			return true
		}
		needleTail := strings.ToLower(types.NormalizedSurfaceSymbolTail(needle))
		if needleTail != "" {
			if lowSlot == needleTail {
				return true
			}
			if slotTail := strings.ToLower(types.NormalizedSurfaceSymbolTail(slot)); slotTail != "" && slotTail == needleTail {
				return true
			}
		}
		return false
	}
	for _, e := range env.Evidence {
		slots := []string{e.Subject, e.Object, e.AnchorSymbol}
		hasA, hasB := false, false
		for _, slot := range slots {
			if matchSlot(slot, a) {
				hasA = true
			}
			if matchSlot(slot, b) {
				hasB = true
			}
		}
		if hasA && hasB {
			return Result{Satisfied: false,
				Detail: fmt.Sprintf("evidence %q links %q and %q via typed slots", e.ID, a, b)}
		}
	}
	return Result{Satisfied: true,
		Detail: fmt.Sprintf("no evidence links %q and %q", a, b)}
}

// ── small helpers ──────────────────────────────────────────────

// containsLower was deleted 2026-05-03 (Phase 6 stage 13) along
// with its callers. The substring-on-typed-slot heuristic was
// migrated to matchSlot equality (lowercase + NormalizedSurfaceSymbolTail
// equality) inside the evaluators that needed it.

// parseComparison accepts "<=N", ">=N", "==N", "<N", ">N", "=N",
// or a bare integer (treated as "==N"). Returns the operator and
// integer threshold.
func parseComparison(expr string) (op string, threshold int, ok bool) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return "", 0, false
	}
	// Two-char operators first.
	for _, cand := range []string{"<=", ">=", "==", "!="} {
		if strings.HasPrefix(s, cand) {
			v, err := strconv.Atoi(strings.TrimSpace(s[len(cand):]))
			if err != nil {
				return "", 0, false
			}
			return cand, v, true
		}
	}
	// Single-char operators.
	for _, cand := range []string{"<", ">", "="} {
		if strings.HasPrefix(s, cand) {
			v, err := strconv.Atoi(strings.TrimSpace(s[len(cand):]))
			if err != nil {
				return "", 0, false
			}
			norm := cand
			if cand == "=" {
				norm = "=="
			}
			return norm, v, true
		}
	}
	// Bare integer → "==".
	if v, err := strconv.Atoi(s); err == nil {
		return "==", v, true
	}
	return "", 0, false
}

func compareInt(n int, op string, threshold int) bool {
	switch op {
	case "<=":
		return n <= threshold
	case ">=":
		return n >= threshold
	case "==":
		return n == threshold
	case "!=":
		return n != threshold
	case "<":
		return n < threshold
	case ">":
		return n > threshold
	}
	return false
}

// defaultExternalArtifactFloor is retained only for runtime-config
// compatibility with older codrax.yaml files. G63 moved external
// artifact completeness out of DraftAnswer token-density checks and
// into the AnswerDocumentV2 typed carrier contract
// (`observed_artifact_fact` + `external_observation`).
var defaultExternalArtifactFloor = 0.4

// SetExternalArtifactFloor keeps the legacy configuration knob
// harmlessly accepted. The evaluator no longer reads the value because
// answer-text token floors are not an allowed hard/soft control signal.
func SetExternalArtifactFloor(f float64) {
	if f > 0 {
		defaultExternalArtifactFloor = f
	}
}

// evalExternalArtifactDecoded is a compatibility no-op. The criterion
// kind remains registered so old TaskNode.SuccessCriteria and
// codrax.yaml deployments do not fail as "unknown kind", but it must
// not inspect model-authored answer text. Runtime artifact coverage is
// enforced in orchestrator contract checks via typed AnswerDocumentV2
// fields and support lanes.
func evalExternalArtifactDecoded(expr string, env Env) Result {
	if env.LogTriage == nil && env.PerfTrace == nil {
		return Result{Satisfied: true,
			Detail: "no external artifact attached — compatibility criterion satisfied"}
	}
	return Result{Satisfied: true,
		Detail: "external artifact coverage is enforced by typed AnswerDocumentV2 observed_artifact_fact/external_observation carriers"}
}
