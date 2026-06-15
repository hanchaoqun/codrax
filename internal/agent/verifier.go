package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// verifierEvaluator drives the B1 verify-stage agent. The verify
// workflow is DETERMINISTIC by design — run_tests does the actual
// work (detect runner, execute command, parse output, install
// Mutable.ChangeReport). The LLM's role is optional: it may emit
// emit_test_results to ADD a narrative FailureSummary to the
// already-installed report. If the LLM skips the emit, the
// verifier's ParseOutput auto-promotes Mutable.ChangeReport's
// Passed / FailureSummary from the parser output.
//
// This mirrors the B1 Q2 design where apply agent is a "dumb
// marshaller": the LLM's judgment-bearing work happens in the
// planner stage; apply and verify execute the plan mechanically.
// The LLM at verify may contribute narrative context (e.g.
// summarise which failure mode hit) but is never the source of
// truth for pass/fail verdicts.
//
// Stop condition: Mutable.ChangeReport != nil. run_tests installs
// it synchronously in Execute, so the LLM typically sees a
// populated report on its FIRST turn and either:
//
//	(a) calls emit_test_results with narrative
//	(b) returns empty content → soft-stop → ShouldStop fires
//
// Either path ends the loop on iteration 0 or 1.
type verifierEvaluator struct {
	// mu is captured in BuildInitialInstruction. ShouldStop
	// consults Mutable.ChangeReport to decide when the report
	// is complete.
	mu *types.MutableState

	// Iteration caps populated from AgentSettings at construction —
	// the static floor used when no per-dispatch override is present.
	// See types.AgentSettings.VerifierSoftIterCap / VerifierHardIterCap.
	defaultSoftIterCap int
	defaultHardIterCap int

	// Per-dispatch caps captured in BuildInitialInstruction from
	// ctx.VerifierSoftIterCapOverride. Zero = "no override seen,
	// fall back to the defaults." Mirrors plannerEvaluator's
	// dispatch-override pattern so multi-language monorepo plans
	// (N target paths needing N runner invocations) get enough
	// soft-cap headroom without bumping into the 5-iter static default.
	dispatchSoftIterCap int
	dispatchHardIterCap int
}

// BuildInitialInstruction captures Mutable and renders a dynamic
// supplement with up to two sections:
//
//  1. AcceptanceTests — natural-language criteria the plan declared
//     (when non-empty). Informational only; no automated matching.
//
//  2. Baseline failures — present ONLY when the orchestrator captured
//     a pre-apply BaselineReport and that baseline had failing
//     tests. Gives the LLM the data it needs to distinguish a
//     regression (passed in baseline, fails now) from a pre-existing
//     failure (failed in baseline AND now) when drafting its
//     emit_test_results narrative.
//
// Returns "" when the plan has no acceptance tests AND there is no
// baseline with failures — keeping the happy-path prompt clean.
func (e *verifierEvaluator) BuildInitialInstruction(ctx *types.AgentContext, _ *skill.Config) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	e.mu = ctx.Mutable
	if ctx.VerifierSoftIterCapOverride > 0 {
		e.dispatchSoftIterCap = ctx.VerifierSoftIterCapOverride
		recoverySlack := e.defaultHardIterCap - e.defaultSoftIterCap
		if recoverySlack < 1 {
			recoverySlack = 1
		}
		e.dispatchHardIterCap = e.dispatchSoftIterCap + recoverySlack
	} else {
		e.dispatchSoftIterCap = 0
		e.dispatchHardIterCap = 0
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return "No ChangePlan installed — verify phase cannot proceed. Return without tool calls."
	}
	// Session-35 invariant: always emit a stage-stating directive so
	// the verifier LLM has an unambiguous anchor even when the plan
	// declares no AcceptanceTests and no baseline captured. The prior
	// "return empty string in the trivial case" shape left the
	// verifier leaning on the raw User Request section, which in
	// write mode carries the user's plan-shaped phrasing ("please
	// generate a plan to fix X") and conflicts with the verifier's
	// actual role (run tests). The User Request section is now
	// suppressed for StageVerify in builder.go, but the positive
	// directive belt + suspenders is worth keeping — the LLM should
	// never need to triangulate the stage intent from the system
	// prompt alone.
	var s strings.Builder
	s.WriteString("## Verify phase\n\n")
	s.WriteString("The plan has been applied to the worktree. " +
		"Your job is mechanical: call run_tests EXACTLY ONCE with the runner that matches the " +
		"plan-touched language, then stop. The system completes the stage as soon as run_tests " +
		"installs the test results.\n\n" +
		"Workflow:\n" +
		"  1. Briefly inspect the worktree (list_files / grep on test-file patterns) so you can pick the runner whose language matches the plan-touched files. Skip lengthy probes — one or two list_files calls is enough.\n" +
		"  2. Call run_tests EXACTLY ONCE with `runner=<choice>` and (optionally) `working_dir=<repo-relative dir>`. Supported runners: go / node / python / rust / java / ruby / swift / cmake / meson / make / hvigor / cjpm.\n" +
		"  3. STOP after run_tests returns. Read its tool result:\n" +
		"     - verdict=PASSED → done; the verify stage will succeed. If the result also lists `NoTestsRunners=[...]`, that means the selected runner found no direct test work or used a syntax fallback. run_tests automatically escalates to another typed runnable candidate before returning when one exists, so do not invent additional checks.\n" +
		"     - verdict=FAILED → optionally call emit_test_results once with a 1-4 sentence failure_summary narrative + classification arrays. Do not re-run tests; the verify→plan retry loop owns recovery.\n\n" +
		"Hard rules:\n" +
		"  - Do NOT call exec_command for ad-hoc syntax checks (py_compile, node --check, gofmt, etc.) AFTER run_tests has produced its results — run_tests owns syntax fallback, typed test-surface escalation, and the authoritative verdict.\n" +
		"  - Do NOT emit_change_plan (that was the plan stage).\n" +
		"  - Do NOT read files or shell out to construct a diff — the plan is already applied.\n" +
		"  - Do NOT re-run tests to chase flakiness — verify is fail-loud.\n")

	if pack := buildWriteContextPackPromptSection(ctx, types.WriteConsumerVerifier, "Priority write context pack", 10); pack != "" {
		s.WriteString("\n")
		s.WriteString(pack)
		s.WriteString("\n")
	}

	// Plan-touched paths section. Without this, the verifier picks a runner
	// from worktree manifests alone (go.mod wins on a polyglot repo where
	// the plan only created a Python script) and runs an unrelated test
	// suite. Surfacing TargetPaths + their extension set anchors the
	// runner choice to what the plan actually changed: pick the runner
	// whose language matches the touched files; only fall back to repo-
	// wide suites when the touched language has no plan-relevant tests.
	if len(plan.TargetPaths) > 0 {
		s.WriteString("\n## Plan-touched paths\n\n")
		s.WriteString("The plan modified ONLY these files in the worktree:\n\n")
		const maxTouched = 20
		shown := 0
		for _, p := range plan.TargetPaths {
			if shown >= maxTouched {
				fmt.Fprintf(&s, "- … (+%d more)\n", len(plan.TargetPaths)-shown)
				break
			}
			fmt.Fprintf(&s, "- %s\n", p)
			shown++
		}
		langs := collectPathLanguages(plan.TargetPaths)
		if len(langs) > 0 {
			fmt.Fprintf(&s, "\nLanguages touched: %s\n", strings.Join(langs, ", "))
			fmt.Fprintf(&s, "Pick the runner whose language matches FIRST. Manifests like "+
				"go.mod / Cargo.toml only matter when the plan's touched language has tests "+
				"in the repo. A plan that creates a single .py file in a Go repo MUST verify "+
				"with the python runner (or `python3 -m py_compile <file>` via exec_command if "+
				"no test fixture exists), NOT by running the unrelated Go suite.\n")
		}
	}
	if section := renderVerifierTestSurfaceSection(ctx.RepoRoot); section != "" {
		s.WriteString(section)
	}
	baseline := ctx.Mutable.BaselineReport()
	baselineFailures := types.CollectBaselineFailures(baseline)
	report := ctx.Mutable.ChangeReport()
	buildFailed := report != nil && report.BuildFailed
	if len(plan.AcceptanceTests) == 0 && len(baselineFailures) == 0 && !buildFailed {
		return s.String()
	}
	s.WriteString("\n")
	if len(plan.AcceptanceTests) > 0 {
		s.WriteString("## Plan acceptance criteria\n\n")
		s.WriteString("The plan declared these acceptance tests (natural-language):\n\n")
		for _, a := range plan.AcceptanceTests {
			s.WriteString("- " + a + "\n")
		}
		s.WriteString("\nAfter run_tests has produced its results, " +
			"you may OPTIONALLY call emit_test_results to add a short FailureSummary " +
			"narrative explaining how the actual test outcome relates to these criteria. " +
			"If all tests passed, no narrative is needed — return without calling emit_test_results.")
	}
	if len(baselineFailures) > 0 {
		if s.Len() > 0 {
			s.WriteString("\n\n")
		}
		s.WriteString("## Pre-existing baseline failures\n\n")
		s.WriteString("Before the plan was applied, the following test(s) were ALREADY failing " +
			"(not caused by this change — the snapshot was taken against the unmodified worktree):\n\n")
		s.WriteString(types.RenderBaselineFailureList(baselineFailures, 15))
		s.WriteString("\n## Output requirement\n\n" +
			"You MUST call emit_test_results with three classified arrays so downstream evaluators " +
			"have a structured signal:\n" +
			"  - regression_assertions: tests that PASSED in baseline but FAIL now (this plan caused them)\n" +
			"  - preexisting_assertions: tests in BOTH baseline-fail AND current-fail (unrelated to this plan)\n" +
			"  - fixed_assertions: tests in baseline-fail BUT PASSING now (a bonus side-effect)\n\n" +
			"Empty arrays are valid; the field is required-required (the schema rejects missing keys). " +
			"Do not narrate the classification in failure_summary — use the structured fields. " +
			"The Passed verdict from run_tests is authoritative (parser-driven) — do not try " +
			"to override it.")
	}
	// Build failures (this run) — surfaced when run_tests synthesised
	// a TestResultKindBuildError row because the build/compile step
	// failed before any test could run. Section is independent of
	// baseline; render whenever the current ChangeReport carries
	// BuildFailed=true so the LLM has concrete file:line targets.
	if buildFailed {
		if s.Len() > 0 {
			s.WriteString("\n\n")
		}
		s.WriteString("## Build failures (this run)\n\n")
		s.WriteString("The plan diff caused the build/compile step to fail. " +
			"Tests did NOT run. Below are the parsed compile errors — fix these first; " +
			"the verify gate cannot be passed until the build succeeds.\n\n")
		var errs []types.BuildError
		for _, tr := range report.TestResults {
			if tr.Kind == types.TestResultKindBuildError {
				errs = append(errs, tr.BuildErrors...)
			}
		}
		const maxBuildShown = 15
		shown := 0
		for _, e := range errs {
			if shown >= maxBuildShown {
				fmt.Fprintf(&s, "- … (+%d more)\n", len(errs)-shown)
				break
			}
			loc := e.File
			if e.Line > 0 {
				loc = fmt.Sprintf("%s:%d", e.File, e.Line)
			}
			if e.Column > 0 {
				loc = fmt.Sprintf("%s:%d", loc, e.Column)
			}
			if e.Symbol != "" {
				fmt.Fprintf(&s, "- %s — %s: %s\n", loc, e.Symbol, e.Message)
			} else {
				fmt.Fprintf(&s, "- %s — %s\n", loc, e.Message)
			}
			shown++
		}
		if len(errs) == 0 {
			s.WriteString("(No structured compile errors parsed; see ChangeReport.TestResults[0].FailureDetail for the raw build-tool excerpt.)\n")
		}
	}
	return s.String()
}

// collectPathLanguages returns a sorted, deduplicated list of language
// identifiers for the supplied paths. Reuses
// repomap/types.DetectLanguage (the canonical extension→language map
// used by the rest of the system) so the verifier prompt names the
// same language identifiers the runner registry recognises. Paths the
// repomap cannot classify are silently skipped.
func collectPathLanguages(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	for _, p := range paths {
		if lang := repomaptypes.DetectLanguage(p); lang != "" {
			seen[lang] = true
		}
	}
	out := make([]string, 0, len(seen))
	for lang := range seen {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

// ShouldStop terminates the ReAct loop as soon as a ChangeReport
// is installed on Mutable. run_tests populates it on first call
// so this is typically iter=0 or iter=1. Defensive cap at iter=5
// prevents runaway loops if the runner is mis-configured.
//
// Two-stage cap: at the soft cap (5), spare one extra iter when the
// LLM is retrying emit_test_results — the narrative payload can run
// kilobytes (Maven/Gradle test stdout summaries) and is occasionally
// truncated by streaming gateways. Without the recovery window, a
// flat cap here would discard the iter=5 clean retry before it
// installs the ChangeReport.
func (e *verifierEvaluator) ShouldStop(resp llm.Response, iteration int) bool {
	if e.mu == nil {
		return true
	}
	if e.mu.ChangeReport() != nil {
		return true
	}
	soft, hard := e.dispatchSoftIterCap, e.dispatchHardIterCap
	if soft <= 0 || hard <= soft {
		soft, hard = e.defaultSoftIterCap, e.defaultHardIterCap
	}
	return iterationCapShouldStop(resp, iteration,
		soft, hard,
		emitTestResultsToolName)
}

const emitTestResultsToolName = "emit_test_results"

// ParseOutput reads Mutable.ChangeReport and surfaces the verdict.
// When the report is missing (run_tests never ran or failed
// catastrophically), returns a fail-loud error so the orchestrator
// sets TaskState.LastError.
func (e *verifierEvaluator) ParseOutput(
	ctx *types.AgentContext,
	_ []llm.Message,
	_ []types.ToolResult,
	_ []types.MCPResponse,
) (*StageOutput, error) {
	out := &StageOutput{}
	if ctx == nil || ctx.Mutable == nil {
		out.MissingPiece = types.MissingFacts
		out.Error = "verifier: Mutable is nil"
		return out, fmt.Errorf("%s", out.Error)
	}
	report := ctx.Mutable.ChangeReport()
	if report == nil {
		out.MissingPiece = types.MissingFacts
		out.Error = "verifier: no ChangeReport installed — run_tests did not execute or its output failed to parse"
		return out, fmt.Errorf("%s", out.Error)
	}

	status := report.NormalizeVerificationStatus()
	if status == types.VerificationStatusPassed || status == types.VerificationStatusUnavailable {
		if status == types.VerificationStatusUnavailable {
			logging.Info(
				"[verifier] local verification unavailable: runners=%v results=%d failure_kind=%s",
				report.NoTestsRunners, len(report.TestResults), report.FailureKind)
		} else {
			logging.Info("[verifier] all tests passed: %d results", len(report.TestResults))
		}
		out.MissingPiece = types.MissingNone
		return out, nil
	}

	// Failed verify is NOT an agent-level error — it's a structured
	// outcome. The verifier returns MissingFacts + an Error so
	// the verify stage hook renders the failure for the user. When the
	// orchestrator's verify→plan retry loop is enabled
	// (pipeline_write_retry_budget > 0), the verify stage hook's caller
	// inspects the error and may dispatch a fresh planner round
	// with PlanningHint seeded from this report; otherwise the Run
	// terminates cleanly with the failure surfaced in LastError.
	failSummary := report.FailureSummary
	if failSummary == "" {
		failSummary = fmt.Sprintf("%d test(s) failed", countFailedResults(report.TestResults))
	}
	out.MissingPiece = types.MissingFacts
	out.Error = "verify failed: " + failSummary
	return out, fmt.Errorf("%s", out.Error)
}

// DetermineMissingPiece reflects ParseOutput's decision.
func (e *verifierEvaluator) DetermineMissingPiece(_ *types.AgentContext, output *StageOutput) types.MissingPiece {
	if output == nil {
		return types.MissingFacts
	}
	return output.MissingPiece
}

// NewVerifierAgent constructs the B1 verify-stage agent.
func NewVerifierAgent(deps *Dependencies) Agent {
	return NewBaseAgent(types.AgentVerifier, deps, &verifierEvaluator{
		defaultSoftIterCap: deps.AgentSettings.VerifierSoftIterCap,
		defaultHardIterCap: deps.AgentSettings.VerifierHardIterCap,
	})
}

// countFailedResults — helper (duplicated from tool.countFailed to
// avoid cross-package imports just for a 5-line counter).
func countFailedResults(results []types.TestResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}

// renderVerifierTestSurfaceSection renders the typed runnable-candidate
// inventory for the verification root so the runner choice starts from
// detected facts instead of language guesses. The same detectors feed
// run_tests itself, so the listed commands match what would execute. The
// section is soft guidance — run_tests still validates the model's choice
// and escalates typed dead ends (zero tests / syntax fallback / missing
// binary) to the next candidate with test work on its own.
func renderVerifierTestSurfaceSection(repoRoot string) string {
	if strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	surface := tool.BuildTestSurface(repoRoot, "")
	if len(surface.Candidates) == 0 {
		return ""
	}
	var s strings.Builder
	s.WriteString("\n## Detected test surface\n\n")
	s.WriteString("Runnable verification candidates detected from repository manifests, " +
		"test-file conventions, and Makefile targets, ranked with real test work first:\n\n")
	const maxCandidates = 8
	for i, c := range surface.Candidates {
		if i >= maxCandidates {
			fmt.Fprintf(&s, "- … (+%d more)\n", len(surface.Candidates)-i)
			break
		}
		signal := "no"
		if c.HasTestSignal {
			signal = "yes"
		}
		workingDir := c.WorkingDir
		if strings.TrimSpace(workingDir) == "" {
			workingDir = "."
		}
		line := fmt.Sprintf("- id=%s runner=%s working_dir=%s test_work=%s", c.ID, c.Runner, workingDir, signal)
		if c.Framework != "" {
			line += " framework=" + c.Framework
		}
		if c.Source != "" {
			line += " source=" + c.Source
		}
		if c.Runner == "make" && c.MakeTarget != "" {
			line += " target=" + c.MakeTarget
		}
		s.WriteString(line + "\n")
	}
	s.WriteString("\nPrefer the highest-ranked candidate with test_work=yes; pass its runner " +
		"(plus working_dir when it is not \".\"). Only deviate when the plan-touched language " +
		"clearly requires another listed candidate. If your choice reports zero tests, uses a " +
		"syntax fallback, or hits a missing binary, the system runs the next candidate with " +
		"test work automatically.\n")
	return s.String()
}
