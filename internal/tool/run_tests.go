package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RunTests executes the project's test suite inside the active
// worktree and stores a structured ChangeReport on Mutable. The
// B1.3 implementation covers four runners deterministically:
//
//   - Go:     `go test -json ./...` — native JSONL output
//   - Node:   `npm test -- --json` (jest) / `vitest --reporter=json`
//   - Python: `pytest --json-report --json-report-file=...`
//     (requires the pytest-json-report plugin; falls back
//     to fail-loud error on missing plugin so the operator
//     knows to install it)
//   - Rust:   `cargo test` (stable text output; parser extracts
//     pass/fail counts + failed test names via regex)
//
// The runner is detected by sniffing the worktree root for
// language-tagged manifest files (go.mod / package.json /
// pyproject.toml or pytest.ini / Cargo.toml). Detection is purely
// filesystem-based — no LLM judgment, no git blame.
//
// L3 red line: Execute MUST NOT invoke ground.BuildContext or
// ground.GroundItem. Test outputs are structured pass/fail data,
// not citations. Enforced by write_mode_red_lines_test.go.
//
// Classified ReadOnly + NonEvidenceTool. Tests read the repo and
// produce verdicts but do not mutate the worktree — the file I/O
// tests do happen through run_tests, but only under test-framework
// control (e.g. tmpdir fixtures) which the runner owns, not codrax.
type RunTests struct {
	ReadOnly
	NonEvidenceTool
}

type runnerPlan struct {
	Runner    string
	Root      string
	Manifest  string
	Priority  int
	Framework string
	Suite     string
}

const maxTestSurfaceEscalations = 3

type pytestTextFallbackResult struct {
	Report   *types.ChangeReport
	ParseErr error
	Output   string
	Command  string
	ExitCode int
	Duration time.Duration
}

type runnerManifest struct {
	File     string
	Runner   string
	Priority int
}

// runTestsParams is the wire-level payload. All fields optional.
type runTestsParams struct {
	// Suite is a language-specific selector. Empty = all tests.
	// Semantics:
	//   Go:     package pattern (default "./...")
	//   Node:   test name / path pattern
	//   Python: pytest node-id or file path
	//   Rust:   test name filter passed after --
	Suite string `json:"suite,omitempty"`

	// TimeoutSeconds overrides the default per-suite timeout
	// (default 300 = 5 minutes). Operators bump for large suites.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`

	// Runner is the LLM-decided test runner choice. Empty = let the
	// system fall back to manifest-keyword auto-detect (the legacy
	// path; brittle for repos without a canonical manifest, e.g. a
	// bare Python directory with `*_test.py` files but no
	// pyproject.toml — see eval/results: grade-school task).
	//
	// Non-empty MUST be one of the whitelisted runners
	// (allowedRunners). The verifier agent inspects the repo first
	// (list_files / read_file / repo_map) and supplies its choice
	// here. The system then short-circuits manifest detection,
	// validates the choice against the whitelist, and dispatches the
	// canonical command template — same template the auto-detect
	// path uses, so safety + determinism are preserved while runner
	// SELECTION moves from a hard-coded keyword table to LLM
	// reasoning. Symmetric to log_triage / perf_triage: LLM
	// extracts, system validates + executes.
	Runner string `json:"runner,omitempty"`

	// Framework refines a runner when the runner has multiple common
	// test frameworks behind the same language command. Currently
	// only meaningful for runner=python, where empty means the system
	// selects pytest vs unittest from typed repo/test-surface signals.
	// A non-empty framework with an empty runner is treated as an
	// explicit python runner choice, because the framework enum is
	// already a typed verifier decision and must bypass manifest
	// auto-detect just like runner=python does.
	Framework string `json:"framework,omitempty"`

	// WorkingDir is the LLM-decided test root, repo-relative. Empty
	// or "." means RepoRoot. A nested module path (e.g. "backend/")
	// scopes the run to a sub-project. Validated to be inside
	// RepoRoot (no escapes via ".." or absolute paths) before any
	// command runs.
	WorkingDir string `json:"working_dir,omitempty"`

	// DryRun is the Module E plan-stage probe flag. When true during
	// StagePlan, the tool runs against the user's current repo surface
	// and stores the resulting ChangeReport on
	// Mutable.PlanStageProbeReports rather than Mutable.ChangeReport.
	// Lets the planner test the existing suite for a baseline
	// understanding before emitting a plan, without polluting the
	// verify-stage's authoritative outcome channel.
	//
	// Outside plan stage the flag is ignored (the verify stage
	// always wants the result on the main ChangeReport channel).
	// run_tests is a read-only operation by nature — it never
	// writes files — so dry_run is a CHANNEL choice, not a safety
	// gate.
	DryRun bool `json:"dry_run,omitempty"`
}

// allowedRunners is the whitelist of runner identifiers the LLM may
// supply via runTestsParams.Runner. Mirrors the cases handled in
// buildRunCommand — adding a new runner requires touching both
// places (and the verifier skill prompt) so the contract stays
// explicit.
var allowedRunners = map[string]struct{}{
	"go":     {},
	"node":   {},
	"python": {},
	"rust":   {},
	"java":   {},
	"ruby":   {},
	"swift":  {},
	"cmake":  {},
	"meson":  {},
	"make":   {},
	"hvigor": {},
	"cjpm":   {},
}

const (
	pythonFrameworkPytest   = "pytest"
	pythonFrameworkUnittest = "unittest"
	pythonFrameworkDjango   = "django"
)

// allowedRunnerList returns the sorted runner whitelist for prompt /
// rejection text. Computed once on demand; tiny enough to skip
// caching.
func allowedRunnerList() []string {
	out := make([]string, 0, len(allowedRunners))
	for r := range allowedRunners {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// Name returns the stable tool identifier.
func (t *RunTests) Name() string { return "run_tests" }

// Description is one sentence + the supported-runner list so
// operators reading logs know the scope without reading code.
func (t *RunTests) Description() string {
	return "Run the detected project test suites inside the active worktree and emit a structured ChangeReport. " +
		"Supports Go (go test -json), Node (jest/vitest --json), Python (pytest --json-report or standard-library unittest), Rust (cargo test text), Java (Maven/Gradle JUnit XML), Kotlin (via the Java Gradle path — build.gradle.kts recognised), Ruby (RSpec --format json), CMake (ctest --output-junit; requires pre-configured build dir), Meson (meson test --xunit-file), raw Makefile (make check/test; pass/fail from exit code), HarmonyOS ArkTS via hvigor (hvigorw test → JUnit XML), and HarmonyOS Cangjie via cjpm (cjpm test → cargo-style text)."
}

// Parameters returns the JSON schema.
func (t *RunTests) Parameters() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "suite": {
      "type": "string",
      "description": "Language-specific test selector. Go: package pattern (default ./...). Node: test name/path pattern. Python: pytest node-id. Rust: filter string. Empty = run all tests."
    },
    "timeout_seconds": {
      "type": "integer",
      "description": "Per-suite timeout override (default 300)."
    },
    "runner": {
      "type": "string",
      "enum": ["go", "node", "python", "rust", "java", "ruby", "swift", "cmake", "meson", "make", "hvigor", "cjpm"],
      "description": "Test runner you have decided to use after inspecting the repo. STRONGLY PREFERRED — if you supply this, the system skips manifest auto-detect and runs your choice directly (works for repos without a canonical manifest, e.g. a bare Python directory with *_test.py files). Empty falls back to manifest auto-detect (brittle; misses bare repos)."
    },
    "framework": {
      "type": "string",
      "enum": ["pytest", "unittest", "django"],
      "description": "Optional Python framework refinement. If set without runner, codrax treats it as runner=python and skips manifest auto-detect. Empty lets codrax select from typed repository signals. Use unittest for Python standard-library unittest suites; use pytest for pytest suites; use django for Django source trees with tests/runtests.py."
    },
    "working_dir": {
      "type": "string",
      "description": "Test root, repo-relative path. Empty or \".\" = repo root. Use to scope the run to a sub-project / module dir. Must be inside the worktree."
    },
    "dry_run": {
      "type": "boolean",
      "description": "Planning probe mode. When true while preparing a change plan, the tool runs against the user's main repo without requiring a worktree, and the result is recorded separately from the final verification result. Use this to learn what the existing test suite reports BEFORE emitting your plan so you can write a plan that responds to observed behaviour. Outside change planning the flag is ignored."
    }
  }
}`)
}

// Execute runs the 3-step flow: detect runner → run command →
// parse output → install Mutable.ChangeReport. On any step-level
// failure, returns a descriptive ToolResult.Summary so the verify
// stage surfaces the reason cleanly.
func (t *RunTests) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if ctx == nil || ctx.Mutable == nil {
		return errResult(t.Name(), "run_tests requires a writable run context (the orchestrator did not provide one)"), nil
	}

	var p runTestsParams
	if len(params) > 0 {
		if _, decodeFailure, err := decodeStrictToolParams(t.Name(), params, t.Parameters(), &p, nil); err != nil {
			return *decodeFailure, err
		}
	}
	if rej := validateRunTestsSuiteSelector(p.Suite); rej != "" {
		return errResult(t.Name(), rej), nil
	}

	// Module E: plan-stage dry-run probes run against MainRepoRoot
	// (no worktree provisioned in plan-only mode); reports flow to a
	// separate probe channel so the verify stage's authoritative
	// outcome is never polluted. Controller apply workflows also run
	// a StagePlan dispatch while ctx.Mode==ModeApply, so the hard
	// channel boundary must read the typed pipeline stage, not the
	// user-facing write mode.
	dryRunProbe := isRunTestsDryRunProbe(ctx, p)
	if !dryRunProbe && inheritRunTestsScopeFromVerifyFailureHandoff(ctx, &p) {
		logging.Info("[run_tests] inherited scoped verify target from typed verify-failure handoff: runner=%s framework=%s working_dir=%s suite=%q",
			strings.TrimSpace(p.Runner), strings.TrimSpace(p.Framework), strings.TrimSpace(p.WorkingDir), strings.TrimSpace(p.Suite))
		if rej := validateRunTestsSuiteSelector(p.Suite); rej != "" {
			return errResult(t.Name(), rej), nil
		}
	}

	if ctx.RepoRoot == "" {
		// In dry-run probe mode the planner has no worktree to swap
		// to; fall back to MainRepoRoot if available so the probe
		// can still execute. Outside dry-run we keep the strict
		// requirement so a misconfigured Run can't quietly run
		// tests against the wrong directory.
		if dryRunProbe && ctx.MainRepoRoot != "" {
			ctx.RepoRoot = ctx.MainRepoRoot
		} else {
			return errResult(t.Name(),
				"run_tests requires ctx.RepoRoot — orchestrator must have swapped to a worktree before verify"), nil
		}
	}

	timeout := 300 * time.Second
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}

	// Two paths to a runner plan list:
	//   (1) Typed verifier choice (`runner`, or Python `framework`
	//       without runner) — short-circuit manifest detection. The
	//       verifier agent has already inspected the repo (list_files /
	//       read_file / repo_map) and committed to a test lane; the
	//       system validates the choice against the whitelist +
	//       working_dir against the worktree boundary, then dispatches.
	//   (2) Empty typed choice — fall back to legacy manifest keyword
	//       detect. Brittle for bare-directory repos; kept as a
	//       backstop so old eval cases / direct CLI calls keep
	//       working.
	// Multi-repo: confine manifest discovery to ActiveSubRepo's
	// root only when that root is inside the active verification
	// tree. Repo-scoped write mode rewrites RepoRoot to the selected
	// sub-repo, and apply/verify later rewrites RepoRoot again to the
	// worktree. In those phases ActiveSubRepo.RootAbs can still point
	// at the original scratch directory; using it would verify stale
	// files. The containment check keeps discovery anchored to the
	// current worktree while preserving the multi-repo parent case.
	walkRoot := activeSubRepoWalkRoot(ctx)

	// Typed test surface: the runnable-candidate inventory for this
	// verification root. Computed once per Execute from filesystem
	// signals only; attached to every outgoing ChangeReport for audit,
	// and consulted by the typed dead-end escalation below (a zero-test
	// or runner-missing outcome must not stand as verification while an
	// unexecuted candidate with real test work exists).
	surface := BuildTestSurface(ctx.RepoRoot, walkRoot)
	if rej := validateRunTestsSuiteSelectorAgainstSurface(p.Suite, surface); rej != "" {
		return errResult(t.Name(), rej), nil
	}

	// planSources tags each queued plan with its typed provenance for
	// the ExecutedCommands audit rows; executedKeys feeds the
	// escalation picker; surfaceEscalations bounds dead-end recovery
	// without assuming only one runner can be unavailable or empty.
	planSources := map[string]string{}
	executedKeys := map[string]bool{}
	surfaceEscalations := 0
	var executedCmds []types.ExecutedCommand

	var plans []runnerPlan
	selectedRunner := strings.TrimSpace(p.Runner)
	selectedSource := "llm_choice"
	if selectedRunner == "" && strings.TrimSpace(p.Framework) != "" {
		selectedRunner = "python"
		selectedSource = "llm_framework_choice"
	}
	if selectedRunner != "" {
		choice, rej := resolveLLMRunnerChoice(ctx.RepoRoot, selectedRunner, p.Framework, p.WorkingDir)
		if rej != "" {
			return errResult(t.Name(), rej), nil
		}
		choice.Suite = strings.TrimSpace(p.Suite)
		plans = []runnerPlan{choice}
		planSources[testSurfaceCandidateKey(choice.Runner, choice.Framework, runnerPlanRel(ctx.RepoRoot, choice))] = selectedSource
		logging.Info("[run_tests] typed-selected runner=%s framework=%s working_dir=%s source=%s (manifest auto-detect bypassed)",
			choice.Runner, choice.Framework, runnerPlanRel(ctx.RepoRoot, choice), selectedSource)
	} else {
		plans = detectRunnerPlans(ctx.RepoRoot, walkRoot)
		if len(plans) == 0 {
			return errResult(t.Name(),
				"run_tests: no supported test runner detected in "+ctx.RepoRoot+
					" — supply the `runner` parameter (one of: "+strings.Join(allowedRunnerList(), ", ")+
					") after inspecting the repo with list_files / read_file / repo_map. "+
					"Manifest auto-detect looked recursively for go.mod / package.json / pyproject.toml / pytest.ini / setup.py / Cargo.toml / Package.swift / pom.xml / build.gradle[.kts] / Gemfile / CMakeLists.txt / meson.build / Makefile / oh-package.json5 / hvigorfile.ts / cjpm.toml and found none — common cause: a bare-directory repo (e.g. exercise stub) without a canonical manifest."), nil
		}
		logging.Info("[run_tests] manifest auto-detect found %d runnable project(s) in %s", len(plans), ctx.RepoRoot)
	}

	var (
		projectReports  []*types.ChangeReport
		combinedOutputs []string
	)
	// finishReport attaches the typed execution evidence (surface +
	// command rows + timestamp) to an outgoing ChangeReport before it
	// is installed. Every install site in this Execute goes through it
	// so early-return reports (timeout / OOM / runner missing / parser
	// error) carry the same durable evidence as the aggregate path.
	finishReport := func(report *types.ChangeReport) *types.ChangeReport {
		if report == nil {
			return nil
		}
		report.ExecutedCommands = append([]types.ExecutedCommand(nil), executedCmds...)
		surfaceCopy := surface
		surfaceCopy.Candidates = append([]types.TestSurfaceCandidate(nil), surface.Candidates...)
		report.TestSurface = &surfaceCopy
		if report.GeneratedAt.IsZero() {
			report.GeneratedAt = time.Now()
		}
		report.EnsureVerificationStatus()
		return report
	}
	var preSuiteProbe *verificationProbeRunResult
	preSuiteProbeConsumed := false
	if shouldRunPreSuiteVerificationProbes(ctx, dryRunProbe) {
		if probe, ok := runPlanVerificationProbes(ctx, "pre_suite_verification_probe"); ok {
			preSuiteProbe = probe
			executedCmds = append(executedCmds, probe.Commands...)
			if strings.TrimSpace(probe.Output) != "" {
				combinedOutputs = append(combinedOutputs, probe.Output)
			}
			probeStatus := probe.Report.NormalizeVerificationStatus()
			switch probeStatus {
			case types.VerificationStatusPassed:
				if cand := selectedSurfaceCandidate(surface); cand != nil && cand.HasTestSignal {
					executedCmds = append(executedCmds, types.ExecutedCommand{
						Runner:     cand.Runner,
						Framework:  cand.Framework,
						WorkingDir: cand.WorkingDir,
						Command:    cand.Command,
						Source:     "probe_primary_suite_skipped",
						Outcome:    "suite_skipped",
					})
				}
				report := probe.Report
				_, ref := StoreBlob(ctx, t.Name()+"-verification-probes", strings.Join(combinedOutputs, "\n\n"))
				installRunTestsReport(ctx, finishReport(report), dryRunProbe)
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   true,
					Summary:   renderVerificationProbePrimarySummary(report, surface),
					RawRef:    ref,
					Timestamp: time.Now(),
				}, nil
			case types.VerificationStatusFailed:
				report := probe.Report
				_, ref := StoreBlob(ctx, t.Name()+"-verification-probes", strings.Join(combinedOutputs, "\n\n"))
				installRunTestsReport(ctx, finishReport(report), dryRunProbe)
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   renderVerificationProbePrimarySummary(report, surface),
					RawRef:    ref,
					Timestamp: time.Now(),
				}, nil
			case types.VerificationStatusUnavailable:
				logging.Warning("[run_tests] pre-suite verification_probe unavailable; continuing to typed project test surface")
			}
		}
	}
	// escalateToSurfaceCandidate appends the highest-ranked unexecuted
	// candidate with typed test work to the plan queue, bounded by
	// maxTestSurfaceEscalations. Trigger sites are typed dead ends:
	// zero test work, parser-confirmed zero tests despite a typed test
	// signal, or a missing runner binary while test work exists
	// elsewhere. The model's explicit runner choice always runs first;
	// this only adds candidates after prior choices produced no real
	// verification.
	escalateToSurfaceCandidate := func(source string) *runnerPlan {
		if surfaceEscalations >= maxTestSurfaceEscalations {
			return nil
		}
		preferredRunner := ""
		if source == "zero_tests_escalation" {
			preferredRunner = preferredRunnerFromChangePlan(ctx)
		}
		cand := nextTestSurfaceEscalationForRunner(surface, executedKeys, preferredRunner)
		if cand == nil {
			return nil
		}
		surfaceEscalations++
		next := runnerPlanFromSurfaceCandidate(ctx.RepoRoot, *cand)
		planSources[testSurfaceCandidateKey(next.Runner, next.Framework, runnerPlanRel(ctx.RepoRoot, next))] = source
		logging.Info("[run_tests] test-surface escalation (%s): queueing %s after current candidate produced no real verification",
			source, cand.ID)
		return &next
	}
	planSourceFor := func(plan runnerPlan) string {
		if src, ok := planSources[testSurfaceCandidateKey(plan.Runner, plan.Framework, runnerPlanRel(ctx.RepoRoot, plan))]; ok {
			return src
		}
		return "auto_detect"
	}
	syntaxPreflightDone := map[string]bool{}
	for planIdx := 0; planIdx < len(plans); planIdx++ {
		plan := plans[planIdx]
		runnerRoot := plan.Root
		runner := plan.Runner
		executedKeys[testSurfaceCandidateKey(runner, plan.Framework, runnerPlanRel(ctx.RepoRoot, plan))] = true

		if runner == "cmake" || runner == "meson" {
			if detectNativeBuildDir(runnerRoot) == "" {
				msg := fmt.Sprintf("%s project detected at %s but no configured build directory found "+
					"(looked for build/, Build/, builddir/, out/, cmake-build-debug/, cmake-build-release/). "+
					"Configure the project first (e.g. `cmake -S . -B build` or `meson setup builddir`) and build it, "+
					"then re-run verify.", runner, runnerRoot)
				projectReports = append(projectReports, qualifyChangeReport(makeExecutionFailureReport("build", msg, true), plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, msg))
				executedCmds = append(executedCmds, types.ExecutedCommand{
					Runner:     runner,
					Framework:  plan.Framework,
					WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
					Source:     planSourceFor(plan),
					Outcome:    "not_configured",
				})
				continue
			}
		}

		// Pre-flight: when the runner has nothing to run, don't run it.
		// Unified flow per language:
		//
		//   1. Detect "is there test work for this runner under root?"
		//      Python uses the broader signal (test files + pytest.ini
		//      / pyproject.toml / setup.cfg / tox.ini / noxfile.py).
		//      Other runners use file-name discovery conventions.
		//   2. If test work exists → fall through to runner invoke.
		//   3. If absent AND a syntax-check fallback exists for this
		//      runner (python: py_compile, node: node --check,
		//      ruby: ruby -wc) → run the fallback on plan-touched
		//      files of the right extension. Pass means the plan's
		//      newly-added code parses; fail means real syntax errors
		//      that block any test runner anyway.
		//   4. If absent AND no fallback applies (or no plan files
		//      match) → emit synthetic Passed with NoTestsRunners.
		//
		// This was the customer-reported scenario: `guess_number.py`
		// added in a repo with zero pytest infrastructure. Pre-fix we
		// invoked pytest, which failed because pytest was not
		// installed on this host's python3. Post-fix we run
		// py_compile on the plan file → succeeds → verify passes.
		if runnerHasNoTestWork(runner, runnerRoot) {
			label := runnerPlanLabel(ctx.RepoRoot, plan)
			if preSuiteProbe != nil && !preSuiteProbeConsumed {
				preSuiteProbeConsumed = true
				projectReports = append(projectReports, qualifyChangeReport(preSuiteProbe.Report, plan, ctx.RepoRoot))
				continue
			}
			if probe, ok := runPlanVerificationProbes(ctx, "no_tests_verification_probe"); ok {
				executedCmds = append(executedCmds, probe.Commands...)
				projectReports = append(projectReports, qualifyChangeReport(probe.Report, plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, probe.Output)
				continue
			}
			exts := syntaxCheckExtensions(runner)
			if len(exts) > 0 {
				files := planFilesByExt(ctx, runnerRoot, exts)
				if len(files) > 0 {
					logging.Info("[run_tests] %s pre-flight: no %s test infrastructure under %s — running %s syntax-check fallback on %d plan file(s)",
						label, runner, runnerRoot, runner, len(files))
					report, syntaxOutput, ok := runSyntaxCheckFallback(ctx, label, runnerRoot, runner, files)
					if ok {
						projectReports = append(projectReports, qualifyChangeReport(report, plan, ctx.RepoRoot))
						combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, syntaxOutput))
						executedCmds = append(executedCmds, types.ExecutedCommand{
							Runner:     runner,
							Framework:  plan.Framework,
							WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
							Source:     planSourceFor(plan),
							Outcome:    "syntax_check_fallback",
						})
						// Syntax fallback proves the changed files parse,
						// but it is not a full verification contract when
						// the typed surface still has an unexecuted runnable
						// candidate (for example Makefile check). Queue that
						// candidate so fallback success cannot prematurely
						// stand in for package/test verification.
						if report != nil && report.Passed {
							if next := escalateToSurfaceCandidate("syntax_check_fallback_escalation"); next != nil {
								combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan,
									fmt.Sprintf("[run_tests: %s] syntax-check fallback passed, but typed test surface still has runnable work — continuing with %s",
										label, runnerPlanLabel(ctx.RepoRoot, *next))))
								plans = append(plans, *next)
							}
						}
						continue
					}
				}
			}
			logging.Info("[run_tests] %s pre-flight: no %s test work under %s — emitting synthetic Passed (NoTestsRunners) without invoking runner",
				label, runner, runnerRoot)
			synthetic := &types.ChangeReport{
				Passed:         true,
				NoTestsRunners: []string{runner},
			}
			projectReports = append(projectReports, qualifyChangeReport(synthetic, plan, ctx.RepoRoot))
			combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan,
				fmt.Sprintf("[run_tests: %s] no %s test work in %s; runner not invoked",
					label, runner, runnerRoot)))
			executedCmds = append(executedCmds, types.ExecutedCommand{
				Runner:     runner,
				Framework:  plan.Framework,
				WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
				Source:     planSourceFor(plan),
				Outcome:    "synthetic_no_tests",
			})
			// Typed dead end: this candidate had zero test work. When the
			// surface still holds an unexecuted candidate with real test
			// work, queue it so a zero-test outcome never stands as the
			// whole verification while a runnable contract exists.
			if next := escalateToSurfaceCandidate("no_tests_escalation"); next != nil {
				plans = append(plans, *next)
			}
			continue
		}

		if report, syntaxOutput, ok := runSyntaxPreflightForPlan(ctx, runnerPlanLabel(ctx.RepoRoot, plan), runnerRoot, runner, syntaxPreflightDone); ok {
			combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, syntaxOutput))
			executedCmds = append(executedCmds, types.ExecutedCommand{
				Runner:     runner,
				Framework:  plan.Framework,
				WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
				Source:     planSourceFor(plan),
				Outcome:    "syntax_preflight",
			})
			if report != nil && !report.Passed {
				installRunTestsReport(ctx, finishReport(qualifyChangeReport(report, plan, ctx.RepoRoot)), dryRunProbe)
				_, ref := StoreBlob(ctx, t.Name()+"-syntax-preflight", strings.Join(combinedOutputs, "\n\n"))
				return types.ToolResult{
					ToolName:  t.Name(),
					Success:   false,
					Summary:   fmt.Sprintf("[run_tests: %s] syntax preflight failed before project tests; raw output stored for inspection", runnerPlanLabel(ctx.RepoRoot, plan)),
					RawRef:    ref,
					Timestamp: time.Now(),
				}, nil
			}
		}

		// MainRepoRoot is threaded through for python venv probing:
		// during verify the worktree is a detached-HEAD checkout that
		// excludes gitignored paths (`.venv/` typically is), so the
		// venv lives at the user's main repo root, not under the
		// worktree. pythonInterpreter probes both roots in order.
		if plan.Runner == "python" && plan.Framework == pythonFrameworkDjango && strings.TrimSpace(plan.Suite) == "" {
			if inferred := inferDjangoSuiteFromChangePlan(ctx, ctx.RepoRoot); inferred != "" {
				plan.Suite = inferred
				logging.Info("[run_tests] python/django@%s inferred scoped suite %q from typed change paths",
					runnerPlanRel(ctx.RepoRoot, plan), inferred)
			}
		}
		cmdStr, extraFile := buildRunCommandForPlan(plan, plan.Suite, ctx.MainRepoRoot)

		// Same root cause as the python venv lookup — but for runners
		// where the dep is consumed by name from cwd (Node's
		// node_modules, Ruby's vendor, hvigor's oh_modules), we have
		// to put the dep AT cwd (= worktree). linkProjectDeps probes
		// the main repo for these gitignored dirs and symlinks them
		// into the worktree before exec. Best-effort — failures log
		// at warning and the runner's native "missing deps" error
		// still surfaces.
		linkProjectDeps(ctx.MainRepoRoot, runnerRoot, runner)
		if extraFile != "" {
			defer os.Remove(extraFile)
		}

		// Wrap command with platform-appropriate resource caps + run
		// under the cross-platform process supervisor. Together these
		// guarantee:
		//   - context cancel kills the entire descendant tree (Unix:
		//     SIGKILL on negative pgid; Windows: TerminateJobObject)
		//   - memory cap (Unix: ulimit -v; Windows: JobMemoryLimit)
		//     so a runaway test can't OOM the host
		//   - CPU-time cap (Unix: ulimit -t; Windows:
		//     PerJobUserTimeLimit) so a CPU burner can't ignore the
		//     wall-clock timeout on multi-core hosts
		caps := verifyResourceCaps()
		wrappedCmd := wrapShellCommandWithCaps(cmdStr, caps)
		// Pre-exec diagnostic: surface the resolved subprocess command
		// + cwd so the operator can reproduce the failing run manually.
		// Without this the only "what was actually executed" record was
		// in the parser-output blob (compressed and not labelled by
		// command line). Customer reports of "verify says X is missing
		// but I just installed it" become tractable once the exact
		// invocation is in the log.
		logging.Info("[run_tests] %s exec: %s (cwd=%s timeout=%v)",
			runnerPlanLabel(ctx.RepoRoot, plan), cmdStr, runnerRoot, timeout)
		execCtx, cancel := context.WithTimeout(context.Background(), timeout)
		cmd := NewShellCommandContext(execCtx, wrappedCmd)
		cmd.Dir = runnerRoot
		cmd.Env = runnerExecutionEnv(runner, ctx.RepoRoot, runnerRoot, ctx.MainRepoRoot)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		execStart := time.Now()
		supRes := SupervisedRun(execCtx, cmd, caps)
		execDuration := time.Since(execStart)
		cancel()
		runErr := supRes.Err
		output := buf.String()
		// Post-exec diagnostic: exit code + output size + stderr-ish
		// excerpt. The 300-byte head is enough to spot "command not
		// found" / "ModuleNotFoundError" / "BUILD FAILED" without
		// flooding the log; the full output is still in the blob.
		execExit := extractExitCode(runErr)
		logging.Info("[run_tests] %s exit=%d duration=%v output_bytes=%d excerpt=%q",
			runnerPlanLabel(ctx.RepoRoot, plan), execExit, execDuration, len(output),
			truncateForLog(output, 300))
		combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, output))
		executedCmds = append(executedCmds, types.ExecutedCommand{
			Runner:     runner,
			Framework:  plan.Framework,
			WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
			Command:    cmdStr,
			ExitCode:   execExit,
			DurationMS: execDuration.Milliseconds(),
			Source:     planSourceFor(plan),
			Outcome:    "executed",
		})
		setLastExecOutcome := func(outcome string) {
			if len(executedCmds) > 0 {
				executedCmds[len(executedCmds)-1].Outcome = outcome
			}
		}
		setLastExecReasonCode := func(reasonCode string) {
			reasonCode = strings.TrimSpace(reasonCode)
			if reasonCode != "" && len(executedCmds) > 0 {
				executedCmds[len(executedCmds)-1].ReasonCode = reasonCode
			}
		}

		// Resource-exhaustion exits get classified explicitly so the
		// verify→plan retry hint surfaces "OOM" / "CPU limit" / "wall
		// timeout" — not the generic "tests failed" the planner would
		// otherwise see and re-derive a wrong corrective direction
		// from. The clearForReplan path consumes ChangeReport.FailureKind
		// in the heuristic hint.
		switch supRes.ExitKind {
		case SupervisedExitTimeout:
			setLastExecOutcome("timeout")
			_, ref := StoreBlob(ctx, t.Name()+"-timeout", strings.Join(combinedOutputs, "\n\n"))
			report := makeResourceExhaustionReport("timeout", fmt.Sprintf(
				"command timed out after %v (set timeout_seconds to bump)", timeout))
			if ref != "" {
				report.FailureSummaryBlobRef = ref
			}
			installRunTestsReport(ctx, finishReport(qualifyChangeReport(report, plan, ctx.RepoRoot)), dryRunProbe)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] command timed out after %v (set timeout_seconds to bump)", runnerPlanLabel(ctx.RepoRoot, plan), timeout),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		case SupervisedExitOOM:
			setLastExecOutcome("oom")
			_, ref := StoreBlob(ctx, t.Name()+"-oom", strings.Join(combinedOutputs, "\n\n"))
			// Neutral structural label — no prescribed-cause list.
			// The model reads ChangeReport.FailureDetail (raw stderr)
			// and decides the fix. Pre-2026-04-30 this string carried
			// a "Either ... or ... or ..." prescription which is
			// system-side classification masquerading as context.
			report := makeResourceExhaustionReport("oom", fmt.Sprintf(
				"command killed by memory cap (limit=%d MiB).",
				caps.MemoryLimitBytes/(1024*1024)))
			if ref != "" {
				report.FailureSummaryBlobRef = ref
			}
			installRunTestsReport(ctx, finishReport(qualifyChangeReport(report, plan, ctx.RepoRoot)), dryRunProbe)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] killed by memory cap (limit=%d MiB) — see ChangeReport.FailureSummary for retry guidance", runnerPlanLabel(ctx.RepoRoot, plan), caps.MemoryLimitBytes/(1024*1024)),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		case SupervisedExitCPULimit:
			setLastExecOutcome("cpu_limit")
			_, ref := StoreBlob(ctx, t.Name()+"-cpu", strings.Join(combinedOutputs, "\n\n"))
			// Neutral structural label — see OOM branch above for
			// the rationale. Raw stderr is in FailureDetail.
			report := makeResourceExhaustionReport("cpu_limit", fmt.Sprintf(
				"command killed by CPU-time cap (limit=%ds).", caps.CPULimitSeconds))
			if ref != "" {
				report.FailureSummaryBlobRef = ref
			}
			installRunTestsReport(ctx, finishReport(qualifyChangeReport(report, plan, ctx.RepoRoot)), dryRunProbe)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] killed by CPU-time cap (limit=%ds) — see ChangeReport.FailureSummary for retry guidance", runnerPlanLabel(ctx.RepoRoot, plan), caps.CPULimitSeconds),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		}

		// Runner-binary-missing detection. Distinct from build failure
		// (compiler ran but project didn't compile) and tests-failed
		// (runner ran tests that reported red): the runner BINARY
		// itself isn't on PATH (`pytest: command not found`, exit 127,
		// stderr "executable file not found"). Re-running the planner
		// can't install software — fail-loud with a clean install hint
		// and let the verify→plan retry loop short-circuit on the new
		// FailureKindRunnerMissing tag.
		if missing, missingBinary, missingReason := detectRunnerMissingForPlan(plan, runErr, output); missing {
			// Surface which detection signal fired so operators (and
			// support engineers triaging "but pytest IS installed!"
			// reports) can tell apart the three failure modes —
			// shell exit 127, Go's exec.ErrNotFound, or output-pattern
			// match — and inspect the matching pattern when it's #3.
			label := runnerPlanLabel(ctx.RepoRoot, plan)
			logging.Info("[run_tests] %s runner_missing detected: binary=%q reason=%s exit=%d",
				label, missingBinary, missingReason, execExit)
			// Tier on tests_exist (matches the user's design fix C):
			// runner_missing + no tests in repo → downgrade to a
			// warning Pass, NOT a verify_failed. The plan didn't need
			// the runner to begin with (e.g. plan creates a bare
			// script in a repo with no test infrastructure); blocking
			// on a missing binary is a false negative.
			testsExist := !runnerHasNoTestWork(runner, runnerRoot)
			if !testsExist {
				logging.Warning("[run_tests] %s runner_missing AND no test work present (binary=%q) — downgrading to synthetic Passed with warning; install %s to enable real test execution",
					label, missingBinary, missingBinary)
				_, ref := StoreBlob(ctx, t.Name()+"-runner-missing-no-tests", strings.Join(combinedOutputs, "\n\n"))
				warning := warningPassedReport(runner, missingBinary, missingReason, execExit, output, ctx.Language)
				projectReports = append(projectReports, qualifyChangeReport(warning, plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan,
					fmt.Sprintf("[run_tests: %s] runner_missing %q, but no test work to do — verify pass-with-warning",
						label, missingBinary)))
				_ = ref
				// A missing binary with zero test work is also a typed
				// dead end — queue the next runnable candidate when the
				// surface has one, so the warning pass never stands
				// alone while a real test contract exists elsewhere.
				if next := escalateToSurfaceCandidate("runner_missing_escalation"); next != nil {
					plans = append(plans, *next)
				}
				continue
			}
			// Typed dead end with test work present: the runner binary is
			// missing but the surface may hold a different runnable
			// candidate (e.g. a Makefile check target when mvn is not
			// installed). Record the missing-runner outcome as command
			// evidence and continue with the escalation candidate instead
			// of failing the whole verification on an uninstallable tool.
			setLastExecOutcome("runner_missing")
			if next := escalateToSurfaceCandidate("runner_missing_escalation"); next != nil {
				combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan,
					fmt.Sprintf("[run_tests: %s] runner binary %q missing (%s, exit=%d) — continuing with %s",
						label, missingBinary, missingReason, execExit, next.Runner)))
				plans = append(plans, *next)
				continue
			}
			if probe, ok := runPlanVerificationProbes(ctx, "runner_missing_verification_probe"); ok {
				executedCmds = append(executedCmds, probe.Commands...)
				projectReports = append(projectReports, qualifyChangeReport(probe.Report, plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, probe.Output)
				continue
			}
			_, ref := StoreBlob(ctx, t.Name()+"-runner-missing", strings.Join(combinedOutputs, "\n\n"))
			report := makeRunnerMissingReport(runner, missingBinary, output, ctx.Language, missingReason, execExit)
			// env_recommend integration: when enabled, replace the
			// hardcoded hint with the new pipeline's diagnose +
			// recommend output. Falls through to the legacy hint on
			// disable / error so behaviour is byte-identical with
			// env_recommend_enabled=false.
			hint := runnerInstallHintForPlan(plan, missingBinary, ctx.Language)
			if envRec := envRecommendOutput(ctx, runner, output); envRec != "" {
				hint = envRec
			}
			report.FailureSummary = updateFailureSummaryWithHint(report.FailureSummary, hint)
			installRunTestsReport(ctx, finishReport(qualifyChangeReport(report, plan, ctx.RepoRoot)), dryRunProbe)
			summary := runnerMissingToolResultSummary(
				ctx.Language, label,
				missingBinary, hint,
				missingReason, execExit, output)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   summary,
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		}

		if runner == "java" {
			reportDir := locateJUnitReportDir(runnerRoot)
			if reportDir == "" {
				projectReports = append(projectReports, qualifyChangeReport(makeBuildFailureReport("Java", output), plan, ctx.RepoRoot))
				continue
			}
			extraFile = reportDir
		}
		if runner == "hvigor" {
			reportDir := locateJUnitReportDir(runnerRoot)
			if reportDir == "" {
				projectReports = append(projectReports, qualifyChangeReport(makeBuildFailureReport("hvigor", output), plan, ctx.RepoRoot))
				continue
			}
			extraFile = reportDir
		}
		if runner == "cmake" || runner == "meson" {
			produced := false
			if extraFile != "" {
				if info, err := os.Stat(extraFile); err == nil && info.Size() > 0 {
					produced = true
				}
			}
			if !produced {
				label := "CMake"
				if runner == "meson" {
					label = "Meson"
				}
				projectReports = append(projectReports, qualifyChangeReport(makeBuildFailureReport(label, output), plan, ctx.RepoRoot))
				continue
			}
		}

		report, err := parseRunnerOutputForPlan(plan, output, extraFile, cmdStr, runErr)
		if err != nil {
			reasonCode := parserErrorReasonCodeForPlan(plan, output, err)
			setLastExecOutcome("parser_error")
			setLastExecReasonCode(reasonCode)
			logging.Warning("[run_tests] parser error for %s reason=%s: %v", runnerPlanLabel(ctx.RepoRoot, plan), reasonCode, err)
			if shouldAttemptPytestTextFallback(plan) {
				fallback := runPytestTextFallback(ctx, plan, timeout, caps)
				if fallback != nil {
					fallbackReasonCode := ""
					fallbackOutcome := "executed"
					if fallback.ParseErr != nil {
						fallbackOutcome = "parser_error"
						fallbackReasonCode = parserErrorReasonCodeForPlan(plan, fallback.Output, fallback.ParseErr)
						if fallbackReasonCode != "" && fallbackReasonCode != reasonCode {
							reasonCode = reasonCode + "," + fallbackReasonCode
						}
					}
					executedCmds = append(executedCmds, types.ExecutedCommand{
						Runner:     runner,
						Framework:  plan.Framework,
						WorkingDir: runnerPlanRel(ctx.RepoRoot, plan),
						Command:    fallback.Command,
						ExitCode:   fallback.ExitCode,
						DurationMS: fallback.Duration.Milliseconds(),
						Source:     "parser_error_fallback",
						Outcome:    fallbackOutcome,
						ReasonCode: fallbackReasonCode,
					})
					combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, fallback.Output))
					if fallback.ParseErr == nil && fallback.Report != nil {
						logging.Info("[run_tests] %s pytest text fallback parsed verdict passed=%v total=%d",
							runnerPlanLabel(ctx.RepoRoot, plan), fallback.Report.Passed, len(fallback.Report.TestResults))
						projectReports = append(projectReports, qualifyChangeReport(fallback.Report, plan, ctx.RepoRoot))
						continue
					}
					if fallback.ParseErr != nil {
						err = fmt.Errorf("%v; pytest text fallback parser failed: %v", err, fallback.ParseErr)
					}
				}
			}
			if probe, ok := runPlanVerificationProbes(ctx, "parser_error_verification_probe"); ok {
				executedCmds = append(executedCmds, probe.Commands...)
				projectReports = append(projectReports, qualifyChangeReport(probe.Report, plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, probe.Output)
				continue
			}
			_, ref := StoreBlob(ctx, t.Name()+"-unparsed", strings.Join(combinedOutputs, "\n\n"))
			// A parser error must still leave a typed, durable report —
			// the audit chain and the verify state machine read reports,
			// not tool summaries. It is not authoritative evidence of a
			// code regression, so classify it separately from red tests.
			parserReport := &types.ChangeReport{
				Passed:            false,
				FailureKind:       types.FailureKindParserError,
				FailureReasonCode: strings.TrimSpace(reasonCode),
				FailureSummary:    fmt.Sprintf("runner output parser failed (reason_code=%s): %v", reasonCode, err),
			}
			if ref != "" {
				parserReport.FailureSummaryBlobRef = ref
			}
			installRunTestsReport(ctx, finishReport(qualifyChangeReport(parserReport, plan, ctx.RepoRoot)), dryRunProbe)
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] output parser failed: %v — raw output stored for inspection", runnerPlanLabel(ctx.RepoRoot, plan), err),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		}
		if reportHasNoExecutedTests(report) && runnerPlanHasTypedTestSignal(surface, plan, ctx.RepoRoot) {
			setLastExecOutcome("zero_tests")
			label := runnerPlanLabel(ctx.RepoRoot, plan)
			logging.Info("[run_tests] %s parser reported zero executed tests despite typed test signal", label)
			if next := escalateToSurfaceCandidate("zero_tests_escalation"); next != nil {
				projectReports = append(projectReports, qualifyChangeReport(report, plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan,
					fmt.Sprintf("[run_tests: %s] runner reported zero tests despite typed test signal — continuing with %s",
						label, runnerPlanLabel(ctx.RepoRoot, *next))))
				plans = append(plans, *next)
				continue
			}
			logging.Info("[run_tests] %s zero-test outcome has no remaining typed escalation candidate — preserving as NoTestsRunners", label)
			projectReports = append(projectReports, qualifyChangeReport(report, plan, ctx.RepoRoot))
			continue
		}
		projectReports = append(projectReports, qualifyChangeReport(report, plan, ctx.RepoRoot))
	}

	report := mergeChangeReports(projectReports)
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		report.PlanID = plan.ID
	}
	report.GeneratedAt = time.Now()

	summary := renderAggregateTestSummary(ctx.RepoRoot, plans, projectReports, report)
	_, ref := StoreBlob(ctx, t.Name(), strings.Join(combinedOutputs, "\n\n"))
	// Module D: propagate the blob ref onto the report so the
	// planner's iteration-history section + retry hint can point
	// the next attempt at the FULL stderr (paged via read_file
	// offset/limit) when the summary alone isn't enough to decide.
	if ref != "" {
		report.FailureSummaryBlobRef = ref
	}
	installRunTestsReport(ctx, finishReport(report), dryRunProbe)

	success := report.Passed
	logging.Info("[run_tests] projects=%d passed=%v total=%d failed=%d",
		len(projectReports), report.Passed, len(report.TestResults), countFailed(report.TestResults))

	return types.ToolResult{
		ToolName:  t.Name(),
		Success:   success,
		Summary:   summary,
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}

func validateRunTestsSuiteSelector(suite string) string {
	selector := strings.TrimSpace(suite)
	if selector == "" {
		return ""
	}
	for _, tok := range strings.Fields(selector) {
		if strings.HasPrefix(tok, "-") {
			return fmt.Sprintf("run_tests rejected: suite=%q contains CLI option token %q. The `suite` field is a test selector only; remove runner flags from `suite` so the executor can build the command from typed parameters.", suite, tok)
		}
	}
	return ""
}

func validateRunTestsSuiteSelectorAgainstSurface(suite string, surface types.TestSurface) string {
	selector := strings.TrimSpace(suite)
	if selector == "" {
		return ""
	}
	for _, cand := range surface.Candidates {
		id := strings.TrimSpace(cand.ID)
		if id == "" || selector != id {
			continue
		}
		framework := strings.TrimSpace(cand.Framework)
		if framework == "" {
			framework = "(empty)"
		}
		workingDir := strings.TrimSpace(cand.WorkingDir)
		if workingDir == "" {
			workingDir = "."
		}
		return fmt.Sprintf("run_tests rejected: suite=%q is a typed TestSurface candidate id, not a language test selector. Select that candidate with runner=%q, framework=%q, working_dir=%q and leave suite empty, or set suite to a real test file/node-id/filter.", suite, strings.TrimSpace(cand.Runner), framework, workingDir)
	}
	return ""
}

func shouldRunPreSuiteVerificationProbes(ctx *types.BusContext, dryRunProbe bool) bool {
	return ctx != nil && ctx.PipelineStage == types.StageVerify && !dryRunProbe
}

func selectedSurfaceCandidate(surface types.TestSurface) *types.TestSurfaceCandidate {
	if strings.TrimSpace(surface.SelectedID) != "" {
		for i := range surface.Candidates {
			if surface.Candidates[i].ID == surface.SelectedID {
				cand := surface.Candidates[i]
				return &cand
			}
		}
	}
	if len(surface.Candidates) == 0 {
		return nil
	}
	cand := surface.Candidates[0]
	return &cand
}

func renderVerificationProbePrimarySummary(report *types.ChangeReport, surface types.TestSurface) string {
	status := report.NormalizeVerificationStatus()
	passed, total := report.Score()
	if total < 0 {
		total = 0
	}
	if passed < 0 {
		passed = 0
	}
	verdict := "UNAVAILABLE"
	switch status {
	case types.VerificationStatusPassed:
		verdict = "PASSED"
	case types.VerificationStatusFailed:
		verdict = "FAILED"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[run_tests: verification_probes verdict=%s] bounded verification probes: %d/%d passed.",
		verdict, passed, total)
	if cand := selectedSurfaceCandidate(surface); cand != nil && cand.HasTestSignal {
		if status == types.VerificationStatusPassed {
			fmt.Fprintf(&b, " Project suite candidate %s was recorded in TestSurface and skipped as a hard gate because the plan supplied passing bounded probes.", cand.ID)
		} else {
			fmt.Fprintf(&b, " Project suite candidate %s remains available in TestSurface for follow-up diagnostics.", cand.ID)
		}
	}
	if status != types.VerificationStatusPassed && strings.TrimSpace(report.FailureSummary) != "" {
		fmt.Fprintf(&b, " %s", strings.TrimSpace(report.FailureSummary))
	}
	return b.String()
}

func inheritRunTestsScopeFromVerifyFailureHandoff(ctx *types.BusContext, p *runTestsParams) bool {
	if ctx == nil || ctx.Mutable == nil || p == nil || ctx.PipelineStage != types.StageVerify {
		return false
	}
	handoff := ctx.Mutable.VerifyFailureHandoff()
	if handoff == nil {
		return false
	}
	cmd, ok := latestExecutedCommandForVerifyScope(handoff.Executed, p)
	if !ok {
		return false
	}
	changed := false
	if strings.TrimSpace(p.Runner) == "" {
		p.Runner = strings.TrimSpace(cmd.Runner)
		changed = true
	}
	if strings.TrimSpace(p.Framework) == "" && strings.TrimSpace(cmd.Framework) != "" {
		p.Framework = strings.TrimSpace(cmd.Framework)
		changed = true
	}
	if strings.TrimSpace(p.WorkingDir) == "" && strings.TrimSpace(cmd.WorkingDir) != "" {
		p.WorkingDir = strings.TrimSpace(cmd.WorkingDir)
		changed = true
	}
	if strings.TrimSpace(p.Suite) == "" {
		if suite := uniqueVerifyFailureSuite(handoff.FailingTests); suite != "" {
			p.Suite = suite
			changed = true
		}
	}
	return changed
}

func latestExecutedCommandForVerifyScope(commands []types.ExecutedCommand, p *runTestsParams) (types.ExecutedCommand, bool) {
	wantRunner := ""
	wantFramework := ""
	wantWorkingDir := ""
	if p != nil {
		wantRunner = strings.TrimSpace(p.Runner)
		wantFramework = strings.TrimSpace(p.Framework)
		if strings.TrimSpace(p.WorkingDir) != "" {
			wantWorkingDir = normalizeRunTestsWorkingDir(p.WorkingDir)
		}
	}
	selectedKey := ""
	var selected types.ExecutedCommand
	for i := len(commands) - 1; i >= 0; i-- {
		cmd := commands[i]
		runner := strings.TrimSpace(cmd.Runner)
		if _, ok := allowedRunners[runner]; !ok {
			continue
		}
		if strings.TrimSpace(cmd.Outcome) != "" && strings.TrimSpace(cmd.Outcome) != "executed" {
			continue
		}
		if wantRunner != "" && wantRunner != runner {
			continue
		}
		if wantFramework != "" && strings.TrimSpace(cmd.Framework) != "" && wantFramework != strings.TrimSpace(cmd.Framework) {
			continue
		}
		if wantWorkingDir != "" && normalizeRunTestsWorkingDir(cmd.WorkingDir) != wantWorkingDir {
			continue
		}
		key := runner + "\x00" + strings.TrimSpace(cmd.Framework) + "\x00" + normalizeRunTestsWorkingDir(cmd.WorkingDir)
		if selectedKey == "" {
			selectedKey = key
			selected = cmd
			continue
		}
		if selectedKey != key {
			return types.ExecutedCommand{}, false
		}
	}
	if selectedKey == "" {
		return types.ExecutedCommand{}, false
	}
	return selected, true
}

func uniqueVerifyFailureSuite(results []types.TestResult) string {
	unique := ""
	for _, result := range results {
		if result.Passed || result.Kind == types.TestResultKindBuildError {
			continue
		}
		suite := strings.TrimSpace(result.Suite)
		if suite == "" {
			continue
		}
		if !verifyFailureSuiteReusableAsSelector(suite) {
			continue
		}
		if unique == "" {
			unique = suite
			continue
		}
		if unique != suite {
			return ""
		}
	}
	return unique
}

func verifyFailureSuiteReusableAsSelector(suite string) bool {
	suite = strings.TrimSpace(suite)
	if suite == "" {
		return false
	}
	switch suite {
	case "unittest", "py_compile", "node_check", "ruby_check", "runner_missing", "build", "make-test":
		return false
	}
	if strings.HasPrefix(suite, "unittest.loader._FailedTest") {
		return false
	}
	if strings.HasPrefix(suite, "verification_probe/") {
		return false
	}
	return true
}

func normalizeRunTestsWorkingDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" || dir == "." {
		return "."
	}
	return filepath.ToSlash(filepath.Clean(dir))
}

func shouldAttemptPytestTextFallback(plan runnerPlan) bool {
	if plan.Runner != "python" {
		return false
	}
	return plan.Framework != pythonFrameworkUnittest && plan.Framework != pythonFrameworkDjango
}

func runPytestTextFallback(ctx *types.BusContext, plan runnerPlan, timeout time.Duration, caps SupervisedRunOptions) *pytestTextFallbackResult {
	if ctx == nil {
		return nil
	}
	commands := buildPlainPytestFallbackCommandsForPlan(plan, plan.Suite, ctx.MainRepoRoot)
	if len(commands) == 0 {
		return nil
	}
	var last *pytestTextFallbackResult
	for i, cmdStr := range commands {
		result := runPytestTextFallbackCommand(ctx, plan, timeout, caps, cmdStr, i+1, len(commands))
		if result == nil {
			continue
		}
		if result.ParseErr == nil && result.Report != nil {
			return result
		}
		last = result
	}
	return last
}

func runPytestTextFallbackCommand(ctx *types.BusContext, plan runnerPlan, timeout time.Duration, caps SupervisedRunOptions, cmdStr string, attempt, total int) *pytestTextFallbackResult {
	if strings.TrimSpace(cmdStr) == "" {
		return nil
	}
	label := runnerPlanLabel(ctx.RepoRoot, plan)
	if total > 1 {
		logging.Info("[run_tests] %s parser_error fallback exec[%d/%d]: %s (cwd=%s timeout=%v)",
			label, attempt, total, cmdStr, plan.Root, timeout)
	} else {
		logging.Info("[run_tests] %s parser_error fallback exec: %s (cwd=%s timeout=%v)",
			label, cmdStr, plan.Root, timeout)
	}
	execCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := NewShellCommandContext(execCtx, wrapShellCommandWithCaps(cmdStr, caps))
	cmd.Dir = plan.Root
	cmd.Env = runnerExecutionEnv(plan.Runner, ctx.RepoRoot, plan.Root, ctx.MainRepoRoot)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	execStart := time.Now()
	supRes := SupervisedRun(execCtx, cmd, caps)
	duration := time.Since(execStart)
	output := buf.String()
	exitCode := extractExitCode(supRes.Err)
	logging.Info("[run_tests] %s parser_error fallback exit=%d duration=%v output_bytes=%d excerpt=%q",
		label, exitCode, duration, len(output), truncateForLog(output, 300))
	report, parseErr := parsePytestTextOutput(output, cmdStr, supRes.Err)
	return &pytestTextFallbackResult{
		Report:   report,
		ParseErr: parseErr,
		Output:   output,
		Command:  cmdStr,
		ExitCode: exitCode,
		Duration: duration,
	}
}

// detectRunner returns the first runnable project discovered under the
// repo. Retained as a thin convenience wrapper for unit tests; Execute
// uses detectRunnerPlans so multi-project repos are verified end-to-end.
func detectRunner(repoRoot string) string {
	plans := detectRunnerPlans(repoRoot, "")
	if len(plans) == 0 {
		return ""
	}
	return plans[0].Runner
}

// resolveLLMRunnerChoice validates an LLM-supplied (runner,
// working_dir) pair and turns it into a runnerPlan. Returns a
// non-empty rejection string on any of:
//   - runner not in the whitelist
//   - working_dir contains a `..` traversal segment after Clean
//   - working_dir resolved absolute path falls outside RepoRoot
//   - working_dir does not exist or is not a directory
//
// The rejection text is operator-readable so the verifier's retry
// can correct course (same pattern as the emit_change_plan
// validators). Manifest existence is NOT checked — that's the whole
// point of the LLM path: the LLM may run pytest on a bare directory
// even when `pyproject.toml` is missing.
func resolveLLMRunnerChoice(repoRoot, runner, framework, workingDir string) (runnerPlan, string) {
	runner = strings.ToLower(strings.TrimSpace(runner))
	if _, ok := allowedRunners[runner]; !ok {
		return runnerPlan{}, fmt.Sprintf(
			"run_tests rejected: runner=%q is not one of the supported runners (%s)",
			runner, strings.Join(allowedRunnerList(), ", "))
	}
	framework = strings.ToLower(strings.TrimSpace(framework))
	if framework != "" {
		if runner != "python" {
			return runnerPlan{}, fmt.Sprintf(
				"run_tests rejected: framework=%q is only supported with runner=python", framework)
		}
		switch framework {
		case pythonFrameworkPytest, pythonFrameworkUnittest, pythonFrameworkDjango:
		default:
			return runnerPlan{}, fmt.Sprintf(
				"run_tests rejected: framework=%q is not one of the supported python frameworks (pytest, unittest, django)", framework)
		}
	}

	dir := strings.TrimSpace(workingDir)
	if dir == "" {
		dir = "."
	}
	cleaned := filepath.Clean(dir)
	if filepath.IsAbs(cleaned) {
		return runnerPlan{}, fmt.Sprintf(
			"run_tests rejected: working_dir=%q must be repo-relative, not absolute", workingDir)
	}
	for _, seg := range strings.Split(filepath.ToSlash(cleaned), "/") {
		if seg == ".." {
			return runnerPlan{}, fmt.Sprintf(
				"run_tests rejected: working_dir=%q escapes the repository (contains \"..\")", workingDir)
		}
	}

	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		rootAbs = repoRoot
	}
	target := filepath.Join(rootAbs, cleaned)
	rel, err := filepath.Rel(rootAbs, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return runnerPlan{}, fmt.Sprintf(
			"run_tests rejected: working_dir=%q resolves outside the repository (%s)", workingDir, target)
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return runnerPlan{}, fmt.Sprintf(
			"run_tests rejected: working_dir=%q does not exist or is not a directory", workingDir)
	}

	if runner == "python" {
		framework = normalizePythonFrameworkChoice(target, framework)
	}

	return runnerPlan{
		Runner:    runner,
		Root:      target,
		Manifest:  "(LLM-selected)",
		Priority:  0, // LLM choice always wins over auto-detect priorities
		Framework: framework,
	}, ""
}

// detectRunnerPlans walks `walkRoot` (or `repoRoot` when walkRoot is
// empty / equal) discovering manifest files, and returns runnerPlans
// labelled relative to `repoRoot`. Multi-repo callers (BusContext.
// ActiveSubRepo non-nil) pass the active sub-repo's RootAbs as
// walkRoot so the walk is confined to that sub-tree — preventing the
// MEDIUM-HIGH leak where a sibling sub-repo's manifest hijacks
// runner detection (design §2 / §4.6 Phase 5).
//
// repoRoot stays the LABEL anchor (runnerPlanRel computes paths
// relative to it). Single-repo / non-multi-repo callers pass walkRoot
// = "" and behaviour is byte-identical to the pre-multi-repo signature.
func detectRunnerPlans(repoRoot, walkRoot string) []runnerPlan {
	candidates := detectRunnerPlanCandidates(repoRoot, walkRoot)
	plansByRoot := make(map[string]runnerPlan)
	for _, plan := range candidates {
		if prev, ok := plansByRoot[plan.Root]; !ok || plan.Priority < prev.Priority || (plan.Priority == prev.Priority && plan.Manifest < prev.Manifest) {
			plansByRoot[plan.Root] = plan
		}
	}
	plans := make([]runnerPlan, 0, len(plansByRoot))
	for _, plan := range plansByRoot {
		plans = append(plans, plan)
	}
	sortRunnerPlans(repoRoot, plans)
	return plans
}

// detectRunnerPlanCandidates returns every manifest hit without the
// one-runner-per-root collapse detectRunnerPlans applies for execution. The
// typed TestSurface needs the full inventory: an alternative runnable
// contract at the same root (e.g. a Makefile next to a pom.xml) is exactly
// what the zero-test / missing-runner escalation has to see. Within one root
// the candidates are deduped per runner (lowest manifest priority wins) so
// Makefile/makefile/GNUmakefile do not produce three make entries.
func detectRunnerPlanCandidates(repoRoot, walkRoot string) []runnerPlan {
	if walkRoot == "" {
		walkRoot = repoRoot
	}
	rootAbs, err := filepath.Abs(walkRoot)
	if err != nil {
		rootAbs = walkRoot
	}
	manifests := supportedRunnerManifests()
	manifestIndex := make(map[string]runnerManifest, len(manifests))
	for _, m := range manifests {
		manifestIndex[strings.ToLower(m.File)] = m
	}

	plansByRootRunner := make(map[string]runnerPlan)
	_ = filepath.Walk(rootAbs, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			if shouldSkipRunnerDir(rootAbs, path, info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		manifest, ok := manifestIndex[strings.ToLower(info.Name())]
		if !ok {
			return nil
		}
		root := filepath.Dir(path)
		plan := runnerPlan{Runner: manifest.Runner, Root: root, Manifest: info.Name(), Priority: manifest.Priority}
		if plan.Runner == "python" {
			plan.Framework = detectPythonTestFramework(root)
		}
		key := root + "\x00" + plan.Runner
		if prev, ok := plansByRootRunner[key]; !ok || plan.Priority < prev.Priority || (plan.Priority == prev.Priority && plan.Manifest < prev.Manifest) {
			plansByRootRunner[key] = plan
		}
		return nil
	})

	plans := make([]runnerPlan, 0, len(plansByRootRunner))
	for _, plan := range plansByRootRunner {
		plans = append(plans, plan)
	}
	sortRunnerPlans(repoRoot, plans)
	return plans
}

// sortRunnerPlans orders plans by manifest priority, then depth relative to
// repoRoot, then path — the pre-existing detection order.
func sortRunnerPlans(repoRoot string, plans []runnerPlan) {
	sort.Slice(plans, func(i, j int) bool {
		relI := runnerPlanRel(repoRoot, plans[i])
		relJ := runnerPlanRel(repoRoot, plans[j])
		if plans[i].Priority != plans[j].Priority {
			return plans[i].Priority < plans[j].Priority
		}
		depthI := strings.Count(relI, "/")
		depthJ := strings.Count(relJ, "/")
		if relI == "." {
			depthI = -1
		}
		if relJ == "." {
			depthJ = -1
		}
		if depthI != depthJ {
			return depthI < depthJ
		}
		if relI != relJ {
			return relI < relJ
		}
		return plans[i].Runner < plans[j].Runner
	})
}

func supportedRunnerManifests() []runnerManifest {
	return []runnerManifest{
		{File: "go.mod", Runner: "go", Priority: 1},
		{File: "oh-package.json5", Runner: "hvigor", Priority: 2},
		{File: "build-profile.json5", Runner: "hvigor", Priority: 3},
		{File: "hvigorfile.ts", Runner: "hvigor", Priority: 4},
		{File: "cjpm.toml", Runner: "cjpm", Priority: 5},
		{File: "package.json", Runner: "node", Priority: 6},
		{File: "pyproject.toml", Runner: "python", Priority: 7},
		{File: "pytest.ini", Runner: "python", Priority: 8},
		{File: "setup.py", Runner: "python", Priority: 9},
		{File: "Cargo.toml", Runner: "rust", Priority: 10},
		{File: "Package.swift", Runner: "swift", Priority: 11},
		{File: "pom.xml", Runner: "java", Priority: 12},
		{File: "build.gradle", Runner: "java", Priority: 13},
		{File: "build.gradle.kts", Runner: "java", Priority: 14},
		{File: "Gemfile", Runner: "ruby", Priority: 15},
		{File: "CMakeLists.txt", Runner: "cmake", Priority: 16},
		{File: "meson.build", Runner: "meson", Priority: 17},
		{File: "Makefile", Runner: "make", Priority: 18},
		{File: "makefile", Runner: "make", Priority: 19},
		{File: "GNUmakefile", Runner: "make", Priority: 20},
	}
}

func shouldSkipRunnerDir(rootAbs, path, name string) bool {
	if samePath(path, rootAbs) {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch strings.ToLower(name) {
	case "node_modules", "vendor", "dist", "target", "out", "build", "builddir",
		"cmake-build-debug", "cmake-build-release", ".gradle", ".idea", ".vscode":
		return true
	}
	return false
}

func samePath(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func activeSubRepoWalkRoot(ctx *types.BusContext) string {
	if ctx == nil || ctx.ActiveSubRepo == nil || strings.TrimSpace(ctx.ActiveSubRepo.RootAbs) == "" {
		return ""
	}
	repoRoot, err := filepath.Abs(ctx.RepoRoot)
	if err != nil {
		repoRoot = ctx.RepoRoot
	}
	activeRoot, err := filepath.Abs(ctx.ActiveSubRepo.RootAbs)
	if err != nil {
		activeRoot = ctx.ActiveSubRepo.RootAbs
	}
	if !pathWithinRoot(repoRoot, activeRoot) {
		return ""
	}
	return activeRoot
}

func pathWithinRoot(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if samePath(root, target) {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == "" {
		return err == nil
	}
	if filepath.IsAbs(rel) {
		return false
	}
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

func runnerPlanRel(repoRoot string, plan runnerPlan) string {
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		rootAbs = repoRoot
	}
	rel, err := filepath.Rel(rootAbs, plan.Root)
	if err != nil || rel == "" {
		return "."
	}
	return filepath.ToSlash(rel)
}

func runnerPlanLabel(repoRoot string, plan runnerPlan) string {
	runner := plan.Runner
	if plan.Runner == "python" && strings.TrimSpace(plan.Framework) != "" {
		runner += "/" + strings.TrimSpace(plan.Framework)
	}
	return fmt.Sprintf("%s@%s", runner, runnerPlanRel(repoRoot, plan))
}

func renderRunnerOutputSection(plan runnerPlan, output string) string {
	var b strings.Builder
	runner := plan.Runner
	if plan.Runner == "python" && strings.TrimSpace(plan.Framework) != "" {
		runner += "/" + strings.TrimSpace(plan.Framework)
	}
	fmt.Fprintf(&b, "### %s (%s)\n", runner, filepath.ToSlash(plan.Root))
	b.WriteString(output)
	return b.String()
}

func reportHasNoExecutedTests(report *types.ChangeReport) bool {
	return report != nil && len(report.NoTestsRunners) > 0 && len(report.TestResults) == 0
}

func runnerPlanHasTypedTestSignal(surface types.TestSurface, plan runnerPlan, repoRoot string) bool {
	key := testSurfaceCandidateKey(plan.Runner, plan.Framework, runnerPlanRel(repoRoot, plan))
	for _, cand := range surface.Candidates {
		if !cand.HasTestSignal {
			continue
		}
		if testSurfaceCandidateKey(cand.Runner, cand.Framework, cand.WorkingDir) == key {
			return true
		}
	}
	return false
}

func makeZeroTestsFailureReport(plan runnerPlan, output string) *types.ChangeReport {
	label := strings.TrimSpace(plan.Runner)
	if label == "" {
		label = "test runner"
	}
	detail := fmt.Sprintf("%s reported zero executed tests even though the typed test surface showed runnable test work.", label)
	if strings.TrimSpace(output) != "" {
		detail += "\n\n" + stdoutHead(output, 4000)
	}
	return &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   "zero_tests",
			Suite:         label,
			Passed:        false,
			FailureDetail: detail,
		}},
		Passed:         false,
		FailureSummary: detail,
		FailureKind:    types.FailureKindTestsFailed,
		NoTestsRunners: []string{label},
	}
}

// isMoreSevereFailureKind orders FailureKind values for the merge
// rule: a resource-exhaustion exit on any single project should
// dominate the aggregate FailureKind so the verifier's retry hint
// calls it out, even when other projects in the same run reported
// only normal red tests.
//
// Order (most → least severe): oom > cpu_limit > timeout > crash >
// build_failure > tests_failed > "".
func isMoreSevereFailureKind(candidate, current types.FailureKind) bool {
	severity := func(k types.FailureKind) int {
		switch k {
		case types.FailureKindOOM:
			return 6
		case types.FailureKindCPULimit:
			return 5
		case types.FailureKindTimeout:
			return 4
		case types.FailureKindCrash:
			return 3
		case types.FailureKindBuildFailure:
			return 2
		case types.FailureKindTestsFailed:
			return 1
		}
		return 0
	}
	return severity(candidate) > severity(current)
}

// installRunTestsReport routes a finished ChangeReport to the right
// Mutable channel based on whether this is a plan-stage dry-run
// probe (Module E) or a real verify-stage run. Probe reports go to
// PlanStageProbeReports so the verify-stage's authoritative
// outcome is never polluted; verify reports go to ChangeReport as
// always. Single seam keeps the dry_run plumbing localised.
func installRunTestsReport(ctx *types.BusContext, report *types.ChangeReport, dryRunProbe bool) {
	if ctx == nil || ctx.Mutable == nil || report == nil {
		return
	}
	if report.GeneratedAt.IsZero() {
		report.GeneratedAt = time.Now()
	}
	if dryRunProbe {
		report.Channel = types.ChangeReportChannelPlannerProbe
		ctx.Mutable.AppendPlanStageProbeReport(report)
		return
	}
	report.Channel = types.ChangeReportChannelPostApplyVerify
	ctx.Mutable.SetChangeReport(report)
}

func isRunTestsDryRunProbe(ctx *types.BusContext, params runTestsParams) bool {
	if ctx == nil || !params.DryRun {
		return false
	}
	if ctx.PipelineStage == types.StagePlan {
		return true
	}
	return ctx.PipelineStage == "" && ctx.Mode == types.ModePlan
}

func qualifyChangeReport(report *types.ChangeReport, plan runnerPlan, repoRoot string) *types.ChangeReport {
	if report == nil {
		return nil
	}
	label := runnerPlanLabel(repoRoot, plan)
	if runnerPlanRel(repoRoot, plan) == "." {
		label = plan.Runner
	}
	if runnerPlanRel(repoRoot, plan) != "." {
		for i := range report.TestResults {
			if report.TestResults[i].AssertionID != "" {
				report.TestResults[i].AssertionID = label + "::" + report.TestResults[i].AssertionID
			}
			if report.TestResults[i].Suite != "" {
				report.TestResults[i].Suite = label + "::" + report.TestResults[i].Suite
			}
		}
	}
	if report.FailureSummary != "" {
		report.FailureSummary = "[" + label + "] " + report.FailureSummary
	}
	// Backfill FailureKind for parser-produced reports that didn't
	// classify themselves. Resource-exhaustion reports already set it
	// (makeResourceExhaustionReport); this branch covers the
	// "ordinary red test" / "build failure" cases the per-language
	// parsers produce.
	if !report.Passed && report.FailureKind == "" {
		if report.BuildFailed {
			report.FailureKind = types.FailureKindBuildFailure
		} else {
			report.FailureKind = types.FailureKindTestsFailed
		}
	}
	return report
}

func mergeChangeReports(reports []*types.ChangeReport) *types.ChangeReport {
	out := &types.ChangeReport{Passed: true}
	var failureSummaries []string
	var failureReasonCodes []string
	for _, report := range reports {
		if report == nil {
			continue
		}
		out.TestResults = append(out.TestResults, report.TestResults...)
		if len(report.MetricDeltas) > 0 {
			if out.MetricDeltas == nil {
				out.MetricDeltas = make(map[string]types.MetricDelta, len(report.MetricDeltas))
			}
			for k, v := range report.MetricDeltas {
				out.MetricDeltas[k] = v
			}
		}
		out.RegressionAssertions = append(out.RegressionAssertions, report.RegressionAssertions...)
		out.PreexistingAssertions = append(out.PreexistingAssertions, report.PreexistingAssertions...)
		out.FixedAssertions = append(out.FixedAssertions, report.FixedAssertions...)
		out.NoTestsRunners = append(out.NoTestsRunners, report.NoTestsRunners...)
		if !report.Passed {
			out.Passed = false
		}
		if report.BuildFailed {
			out.BuildFailed = true
		}
		if report.FailureSummary != "" {
			failureSummaries = append(failureSummaries, report.FailureSummary)
		}
		failureReasonCodes = appendFailureReasonCodes(failureReasonCodes, report.FailureReasonCode)
		// FailureKind precedence: resource-exhaustion kinds (oom,
		// cpu_limit, timeout) win over build_failure / tests_failed
		// because they describe a more specific failure mode the
		// retry hint must call out. First-set-wins below that, so
		// build_failure on project A doesn't get masked by
		// tests_failed on project B.
		if out.FailureKind == "" || isMoreSevereFailureKind(report.FailureKind, out.FailureKind) {
			if report.FailureKind != "" {
				out.FailureKind = report.FailureKind
			}
		}
	}
	out.RegressionAssertions = dedupStrings(out.RegressionAssertions)
	out.PreexistingAssertions = dedupStrings(out.PreexistingAssertions)
	out.FixedAssertions = dedupStrings(out.FixedAssertions)
	out.NoTestsRunners = dedupStrings(out.NoTestsRunners)
	if len(failureSummaries) > 0 {
		out.FailureSummary = strings.Join(failureSummaries, " | ")
	}
	if len(failureReasonCodes) > 0 {
		out.FailureReasonCode = strings.Join(dedupStrings(failureReasonCodes), ",")
	}
	out.EnsureVerificationStatus()
	return out
}

func appendFailureReasonCodes(dst []string, raw string) []string {
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			dst = append(dst, part)
		}
	}
	return dst
}

func renderAggregateTestSummary(repoRoot string, plans []runnerPlan, reports []*types.ChangeReport, aggregate *types.ChangeReport) string {
	if len(plans) == 1 && len(reports) == 1 {
		return renderTestSummary(plans[0].Runner, reports[0])
	}
	passedProjects := 0
	for _, report := range reports {
		if report != nil && report.Passed {
			passedProjects++
		}
	}
	var b strings.Builder
	status := aggregate.NormalizeVerificationStatus()
	totalAssertions, failedAssertions := countAvailableReportAssertions(reports)
	if status == types.VerificationStatusUnavailable {
		fmt.Fprintf(&b, "[run_tests: projects=%d verdict=UNAVAILABLE] local verification unavailable; %d/%d project(s) produced a usable pass signal; %d diagnostic row(s), %d real assertion(s).",
			len(plans), passedProjects, len(plans), len(aggregate.TestResults), totalAssertions)
	} else {
		fmt.Fprintf(&b, "[run_tests: projects=%d] %d/%d project(s) passed; %d assertion(s), %d failed.",
			len(plans), passedProjects, len(plans), totalAssertions, failedAssertions)
	}
	failedShown := 0
	for idx, report := range reports {
		if report == nil || report.Passed || failedShown >= 3 {
			continue
		}
		fmt.Fprintf(&b, " %s", runnerPlanLabel(repoRoot, plans[idx]))
		if report.FailureSummary != "" {
			fmt.Fprintf(&b, ": %s", report.FailureSummary)
		}
		failedShown++
	}
	return b.String()
}

func makeExecutionFailureReport(suite, detail string, buildFailed bool) *types.ChangeReport {
	return &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   suite,
			Suite:         suite,
			Passed:        false,
			FailureDetail: detail,
		}},
		Passed:         false,
		BuildFailed:    buildFailed,
		FailureSummary: detail,
	}
}

// makeResourceExhaustionReport builds a ChangeReport for a
// supervisor-classified resource-exhaustion exit (timeout / OOM /
// cpu_limit). The verifier's heuristic hint (clearForReplan) reads
// FailureKind to switch the retry narrative away from "tests failed"
// to a kind-specific corrective direction the planner can act on.
//
// Why a synthetic TestResult: ChangeReport consumers (the
// best-known-good latch + score-based selectFinalReport) iterate over
// TestResults; an empty slice would let a regression-with-resource-
// exhaustion silently lose to a previous iteration that had at least
// one passing test. The synthetic build_error row participates in
// counts but is intentionally never confused with a unit test
// (Kind=TestResultKindBuildError).
func makeResourceExhaustionReport(kind, detail string) *types.ChangeReport {
	var fk types.FailureKind
	switch kind {
	case "timeout":
		fk = types.FailureKindTimeout
	case "oom":
		fk = types.FailureKindOOM
	case "cpu_limit":
		fk = types.FailureKindCPULimit
	default:
		fk = types.FailureKindCrash
	}
	return &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   string(fk),
			Suite:         string(fk),
			Passed:        false,
			FailureDetail: detail,
		}},
		Passed:         false,
		BuildFailed:    fk == types.FailureKindOOM || fk == types.FailureKindTimeout || fk == types.FailureKindCPULimit,
		FailureSummary: detail,
		FailureKind:    fk,
	}
}

// detectRunnerMissing classifies whether the runner's primary binary
// is absent from the execution environment. Returns (true, binaryName)
// when the failure mode is "tool not installed" rather than "tool ran
// and reported red". Three signals trigger detection:
//
//  1. Exit code 127 — POSIX convention for "command not found" set
//     by sh / bash when invoking an unknown binary. Both Linux and
//     macOS shells produce this.
//  2. Go's os/exec returns exec.ErrNotFound when LookPath fails on
//     a direct (non-shell-wrapped) invocation. We unwrap runErr's
//     chain to spot it.
//  3. Output (stderr captured via combined buffer) contains the
//     shell-emitted "command not found" / "not found" / "executable
//     file not found" patterns. Matched defensively against the
//     runner's primary binary name so a test's literal "X not found"
//     output doesn't false-positive.
//
// runnerPrimaryBinary maps each runner identifier to its expected
// CLI binary so we know which substring to anchor detection on. The
// shell wrapper inside Execute does `sh -c "<cmdStr>"`; we look at
// runErr's exit code first (cheapest), then fall back to output
// pattern matching for environments where the supervisor swallowed
// the exit code.
func detectRunnerMissing(runner string, runErr error, output string) (bool, string, string) {
	bin := runnerPrimaryBinary(runner)
	if bin == "" {
		return false, "", ""
	}
	// 1. Exit code 127 from the shell wrapper.
	if runErr != nil {
		if exitCode := extractExitCode(runErr); exitCode == 127 {
			return true, bin, "exit_code_127"
		}
		// 2. Direct exec.ErrNotFound (rare with shell wrapper but
		//    cheap to check; covers future direct-Cmd refactors).
		if errors.Is(runErr, exec.ErrNotFound) {
			return true, bin, "exec.ErrNotFound"
		}
	}
	// 3. Stderr/stdout patterns. We anchor on bin to keep false-
	//    positive rate low — a test that prints "foo not found" in
	//    its assertion message must NOT trip detection. The patterns
	//    cover sh / bash / dash / zsh / Windows cmd shapes.
	patterns := []string{
		bin + ": command not found",
		bin + ": not found",
		bin + " not found",
		"executable file not found",
		"is not recognized as an internal or external command", // Windows cmd
		"No such file or directory: '" + bin + "'",
	}
	if runner == "python" {
		// pythonInterpreter resolves to `python3` / `python` / a
		// venv path, NOT bare `pytest`. The shell can therefore
		// fail with the interpreter name in its missing-binary
		// message, AND Python itself can fail with a distinct
		// "No module named pytest" error when the interpreter
		// resolves but pytest isn't installed for it. Both shapes
		// indicate operator-fixable env issues that re-running
		// the planner can't address.
		patterns = append(patterns,
			"python: command not found",
			"python: not found",
			"python3: command not found",
			"python3: not found",
			"No module named pytest",
			"No module named 'pytest'",
		)
	}
	lower := strings.ToLower(output)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true, bin, "pattern: " + p
		}
	}
	return false, "", ""
}

func detectRunnerMissingForPlan(plan runnerPlan, runErr error, output string) (bool, string, string) {
	bin := runnerPrimaryBinaryForPlan(plan)
	if bin == "" {
		return false, "", ""
	}
	if missing, missingBin, reason := detectRunnerMissing(plan.Runner, runErr, output); missing {
		if plan.Runner == "python" && plan.Framework == pythonFrameworkUnittest && missingBin == "pytest" {
			return false, "", ""
		}
		return true, missingBin, reason
	}
	if exitCode := extractExitCode(runErr); exitCode == 127 || exitCode == 126 {
		return true, bin, fmt.Sprintf("exit_code_%d", exitCode)
	}
	if outputIndicatesMissingBinary(output, bin) {
		return true, bin, "output_pattern"
	}
	return false, "", ""
}

func outputIndicatesMissingBinary(output, bin string) bool {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return false
	}
	patterns := []string{
		bin + ": command not found",
		bin + ": not found",
		bin + " not found",
		"executable file not found",
		"is not recognized as an internal or external command",
		"No such file or directory: '" + bin + "'",
	}
	lower := strings.ToLower(output)
	for _, p := range patterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// extractExitCode best-effort pulls the process exit code out of an
// exec error chain. *exec.ExitError is the typical wrapper; other
// shapes return -1.
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ee.ProcessState != nil {
			return ee.ProcessState.ExitCode()
		}
	}
	return -1
}

// truncateForLog returns a single-line excerpt suitable for embedding
// in an INFO log line or a model-facing summary: trims surrounding
// whitespace, replaces interior newlines + carriage returns with the
// pilcrow visible-marker so multi-line output stays readable on one
// log line, and caps to maxBytes (UTF-8 safe — backs up to a rune
// boundary instead of slicing mid-rune).
func truncateForLog(s string, maxBytes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", " ¶ ")
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && cut < len(s) {
		b := s[cut]
		// Don't slice mid-UTF-8: continuation bytes have the high
		// two bits = 10. Step back until we land on a leading byte
		// or ASCII.
		if b&0xC0 != 0x80 {
			break
		}
		cut--
	}
	return s[:cut] + "…"
}

// runnerPrimaryBinary returns the CLI binary name we expect the
// runner to invoke (i.e. the first token of the canonical command
// template in buildRunCommand). Empty when the runner uses a
// project-local launcher we can't predict (e.g. `npm test` calls
// whatever package.json's "test" script defines).
func runnerPrimaryBinary(runner string) string {
	switch runner {
	case "go":
		return "go"
	case "python":
		return "pytest"
	case "rust":
		return "cargo"
	case "java":
		// Maven first, then Gradle wrapper. We can't know which
		// without inspecting the project; default to mvn since
		// `pom.xml` is the more common project shape, and the
		// Gradle wrapper (`./gradlew`) is a project-local script
		// that almost always exists when build.gradle is present.
		return "mvn"
	case "ruby":
		return "bundle"
	case "swift":
		return "swift"
	case "cmake":
		return "ctest"
	case "meson":
		return "meson"
	case "make":
		return "make"
	case "hvigor":
		return "hvigorw"
	case "cjpm":
		return "cjpm"
	case "node":
		// `npm test` — npm is the binary; the test script is
		// project-defined. If npm is missing we want to detect.
		return "npm"
	}
	return ""
}

func runnerPrimaryBinaryForPlan(plan runnerPlan) string {
	if plan.Runner == "python" && (plan.Framework == pythonFrameworkUnittest || plan.Framework == pythonFrameworkDjango) {
		return "python"
	}
	return runnerPrimaryBinary(plan.Runner)
}

// hasPythonTestInfrastructure returns true when the given root
// contains any signal that this project intends to run Python tests.
// The detector is a superset of file-name discovery (per fix B in
// the user's design spec): test source files (test_*.py / *_test.py /
// conftest.py) AND project configuration hints (pytest.ini /
// pyproject.toml / setup.cfg / tox.ini / noxfile.py) all count. Any
// one signal flips the result to true.
//
// Rationale: a pyproject.toml without test files still implies the
// operator has a pytest workflow they expect to drive — bare
// scaffolding repos rarely have one. Conversely, a repo with only
// a single .py file and no manifests is almost always a "throwaway
// script" plan; running pytest there is wasted work and on hosts
// without pytest installed produces a false-negative verify_failed.
func hasPythonTestInfrastructure(root string) bool {
	if root == "" {
		return false
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return false
	}
	// Top-level config files take precedence — cheaper to stat than
	// to walk. Walk is the fallback for nested test sources.
	configFiles := []string{
		"pytest.ini", "pyproject.toml", "setup.cfg",
		"tox.ini", "noxfile.py", "conftest.py", "runtests.py",
	}
	for _, name := range configFiles {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	stop := errors.New("found-test-infra")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			switch name {
			case ".git", ".hg", ".svn",
				"node_modules", "vendor", "target",
				"dist", "build", "out", ".tox",
				".venv", "venv", "env", ".virtualenv",
				"__pycache__", ".pytest_cache", ".mypy_cache",
				".idea", ".vscode", ".gradle", ".mvn",
				".codrax":
				return filepath.SkipDir
			}
			return nil
		}
		switch {
		case name == "conftest.py":
			return stop
		case strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py"):
			return stop
		case strings.HasSuffix(name, "_test.py"):
			return stop
		case name == "runtests.py":
			return stop
		case name == "pytest.ini" || name == "pyproject.toml" ||
			name == "setup.cfg" || name == "tox.ini" || name == "noxfile.py":
			return stop
		}
		return nil
	})
	return errors.Is(err, stop)
}

func detectPythonTestFramework(root string) string {
	if hasDjangoTestRunner(root) {
		return pythonFrameworkDjango
	}
	if hasPythonPytestConfig(root) {
		return pythonFrameworkPytest
	}
	if hasPythonUnittestSignal(root) {
		return pythonFrameworkUnittest
	}
	return pythonFrameworkPytest
}

func normalizePythonFrameworkChoice(root, framework string) string {
	framework = strings.ToLower(strings.TrimSpace(framework))
	if hasDjangoTestRunner(root) {
		return pythonFrameworkDjango
	}
	if framework != "" {
		return framework
	}
	return detectPythonTestFramework(root)
}

func hasDjangoTestRunner(root string) bool {
	if root == "" {
		return false
	}
	for _, candidate := range []string{
		filepath.Join(root, "runtests.py"),
		filepath.Join(root, "tests", "runtests.py"),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func hasPythonPytestConfig(root string) bool {
	if root == "" {
		return false
	}
	stop := errors.New("found pytest config")
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if shouldSkipPythonProbeDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		switch name {
		case "pytest.ini", "pyproject.toml", "setup.cfg", "tox.ini", "noxfile.py", "conftest.py":
			return stop
		}
		return nil
	})
	return errors.Is(err, stop)
}

func hasPythonUnittestSignal(root string) bool {
	if root == "" {
		return false
	}
	stop := errors.New("found unittest signal")
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if info != nil && info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if shouldSkipPythonProbeDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if !(strings.HasPrefix(name, "test_") && strings.HasSuffix(name, ".py")) && !strings.HasSuffix(name, "_test.py") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if len(data) > 64*1024 {
			data = data[:64*1024]
		}
		text := string(data)
		if strings.Contains(text, "unittest.TestCase") ||
			strings.Contains(text, "import unittest") ||
			strings.Contains(text, "from unittest") {
			return stop
		}
		return nil
	})
	return errors.Is(err, stop)
}

func shouldSkipPythonProbeDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn",
		"node_modules", "vendor", "target",
		"dist", "build", "out", ".tox",
		".venv", "venv", "env", ".virtualenv",
		"__pycache__", ".pytest_cache", ".mypy_cache",
		".idea", ".vscode", ".gradle", ".mvn",
		".codrax":
		return true
	}
	return false
}

// runnerHasNoTestWork is the unified "is there test work for this
// runner under root?" predicate used by both the pre-flight skip
// and the runner_missing tier (fix C). For python it consults
// hasPythonTestInfrastructure (then a separate typed framework
// decision chooses pytest or unittest); for other runners it falls
// through to hasNoTestSources's file-name walk. Returns true when
// there is NO test work.
func runnerHasNoTestWork(runner, root string) bool {
	if runner == "python" {
		return !hasPythonTestInfrastructure(root)
	}
	return hasNoTestSources(runner, root)
}

func preferredRunnerFromChangePlan(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return ""
	}
	seen := map[string]bool{}
	for _, p := range append(append([]string{}, plan.TargetPaths...), plan.AppliedPaths...) {
		runner := runnerForPath(p)
		if runner != "" {
			seen[runner] = true
		}
	}
	if len(seen) != 1 {
		return ""
	}
	for runner := range seen {
		return runner
	}
	return ""
}

func runnerForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	switch ext {
	case ".py":
		return "python"
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return "node"
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".java", ".kt", ".kts":
		return "java"
	case ".rb":
		return "ruby"
	case ".swift":
		return "swift"
	case ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp":
		return "cmake"
	}
	return ""
}

func runnerExecutionEnv(runner, repoRoot, runnerRoot, mainRoot string) []string {
	return runnerExecutionEnvForBase(os.Environ(), runner, repoRoot, runnerRoot, mainRoot)
}

func runnerExecutionEnvForBase(base []string, runner, repoRoot, runnerRoot, mainRoot string) []string {
	env := append([]string(nil), base...)
	if runnerRoot != "" {
		env = withEnvValue(env, "PWD", runnerRoot)
	}
	if runner != "python" {
		return env
	}
	pythonPath := pythonWorktreePythonPath(envValue(env, "PYTHONPATH"), repoRoot, runnerRoot, mainRoot)
	env = withEnvValue(env, "PYTHONPATH", pythonPath)
	return env
}

func pythonWorktreePythonPath(existing, repoRoot, runnerRoot, mainRoot string) string {
	var out []string
	seen := make(map[string]bool)
	mainKey := cleanAbsPathKey(mainRoot)
	add := func(path string, dropMain bool) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		key := cleanAbsPathKey(path)
		if key == "" {
			key = path
		}
		if dropMain && mainKey != "" && key == mainKey {
			return
		}
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, path)
	}

	for _, root := range pythonWorktreeImportRoots(repoRoot, runnerRoot) {
		add(root, false)
	}
	for _, entry := range filepath.SplitList(existing) {
		add(entry, true)
	}
	return strings.Join(out, string(os.PathListSeparator))
}

func pythonWorktreeImportRoots(repoRoot, runnerRoot string) []string {
	var roots []string
	seen := make(map[string]bool)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		key := cleanAbsPathKey(path)
		if key == "" {
			key = path
		}
		if seen[key] {
			return
		}
		seen[key] = true
		roots = append(roots, path)
	}
	addSourceLayoutRoot := func(root string) {
		root = strings.TrimSpace(root)
		if root == "" {
			return
		}
		for _, child := range []string{"src", "lib"} {
			candidate := filepath.Join(root, child)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				add(candidate)
			}
		}
		add(root)
	}

	addSourceLayoutRoot(repoRoot)
	if cleanAbsPathKey(runnerRoot) != cleanAbsPathKey(repoRoot) {
		addSourceLayoutRoot(runnerRoot)
	}
	return roots
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	return ""
}

func withEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	next := make([]string, 0, len(env)+1)
	replaced := false
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			if !replaced {
				next = append(next, prefix+value)
				replaced = true
			}
			continue
		}
		next = append(next, item)
	}
	if !replaced {
		next = append(next, prefix+value)
	}
	return next
}

func cleanAbsPathKey(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func inferDjangoSuiteFromChangePlan(ctx *types.BusContext, repoRoot string) string {
	if ctx == nil || ctx.Mutable == nil || strings.TrimSpace(repoRoot) == "" {
		return ""
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return ""
	}
	paths := append(append([]string{}, plan.TargetPaths...), plan.AppliedPaths...)
	for _, raw := range dedupeStringsPreserveOrder(paths) {
		rel := cleanRepoRelPath(raw)
		if rel == "" || !strings.HasSuffix(strings.ToLower(rel), ".py") {
			continue
		}
		if strings.HasPrefix(rel, "tests/") {
			return djangoSuiteSelector(rel)
		}
	}
	for _, raw := range dedupeStringsPreserveOrder(paths) {
		rel := cleanRepoRelPath(raw)
		if rel == "" || !strings.HasSuffix(strings.ToLower(rel), ".py") || strings.HasPrefix(rel, "tests/") {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(rel), filepath.Ext(rel))
		if suite := findUniqueDjangoTestFileSuite(repoRoot, rel, base); suite != "" {
			return suite
		}
		if suite := findUniqueDjangoTestLabelSuite(repoRoot, rel, base); suite != "" {
			return suite
		}
	}
	return ""
}

func findUniqueDjangoTestFileSuite(repoRoot, rel, sourceBase string) string {
	sourceBase = strings.TrimSpace(sourceBase)
	if sourceBase == "" {
		return ""
	}
	testsRoot := filepath.Join(repoRoot, "tests")
	names := map[string]bool{
		"test_" + sourceBase + ".py": true,
		sourceBase + "_tests.py":     true,
	}
	var matches []string
	_ = filepath.WalkDir(testsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".codrax", "__pycache__":
				return filepath.SkipDir
			}
			return nil
		}
		if !names[d.Name()] {
			return nil
		}
		testRelNative, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		testRel := filepath.ToSlash(testRelNative)
		if !djangoTestFileLabelMatchesSource(testRel, rel, sourceBase) {
			return nil
		}
		matches = append(matches, testRel)
		return nil
	})
	if len(matches) != 1 {
		return ""
	}
	return djangoSuiteSelector(matches[0])
}

func djangoTestFileLabelMatchesSource(testRel, sourceRel, sourceBase string) bool {
	label := djangoImmediateTestLabel(testRel)
	if label == "" {
		return true
	}
	return djangoSuiteTargetTokens(sourceRel, sourceBase)[djangoSuiteToken(label)]
}

func djangoImmediateTestLabel(testRel string) string {
	rel := cleanRepoRelPath(testRel)
	if !strings.HasPrefix(rel, "tests/") {
		return ""
	}
	rest := strings.TrimPrefix(rel, "tests/")
	parts := strings.Split(rest, "/")
	if len(parts) <= 1 {
		return ""
	}
	return parts[0]
}

func findUniqueDjangoTestLabelSuite(repoRoot, rel, sourceBase string) string {
	testsRoot := filepath.Join(repoRoot, "tests")
	entries, err := os.ReadDir(testsRoot)
	if err != nil {
		return ""
	}
	targets := djangoSuiteTargetTokens(rel, sourceBase)
	var matches []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "__pycache__" {
			continue
		}
		if targets[djangoSuiteToken(name)] {
			matches = append(matches, name)
		}
	}
	if len(matches) != 1 {
		return ""
	}
	return matches[0]
}

func djangoSuiteTargetTokens(rel, sourceBase string) map[string]bool {
	targets := make(map[string]bool)
	add := func(s string) {
		token := djangoSuiteToken(s)
		if token != "" {
			targets[token] = true
		}
	}
	add(sourceBase)
	for _, part := range strings.Split(cleanRepoRelPath(rel), "/") {
		part = strings.TrimSuffix(part, filepath.Ext(part))
		switch part {
		case "", "django", "contrib":
			continue
		default:
			add(part)
		}
	}
	return targets
}

func djangoSuiteToken(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "test_")
	s = strings.TrimSuffix(s, "_tests")
	s = strings.TrimSuffix(s, "_test")
	s = strings.TrimSuffix(s, "tests")
	s = strings.TrimSuffix(s, "test")
	if len(s) > 3 && strings.HasSuffix(s, "s") {
		s = strings.TrimSuffix(s, "s")
	}
	return strings.Trim(s, "_-. ")
}

func cleanRepoRelPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

func dedupeStringsPreserveOrder(values []string) []string {
	seen := make(map[string]bool, len(values))
	var out []string
	for _, value := range values {
		key := strings.TrimSpace(value)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	return out
}

// planFilesByExt returns the absolute paths of every plan TargetPath
// whose lowercased basename ends in one of the supplied extensions
// (each ext is matched as a suffix, so callers pass ".py", ".js",
// etc.) AND whose path resolves inside `root` (typically the
// worktree). Used by the per-runner syntax-check fallbacks to pick
// files for the alternate verification when no test infrastructure
// exists.
func planFilesByExt(ctx *types.BusContext, root string, exts []string) []string {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	plan := ctx.Mutable.ChangePlan()
	if plan == nil {
		return nil
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		rootAbs = root
	}
	var out []string
	seen := make(map[string]bool, len(plan.TargetPaths))
	for _, p := range plan.TargetPaths {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		matched := false
		for _, ext := range exts {
			if strings.HasSuffix(lower, ext) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		var abs string
		if filepath.IsAbs(p) {
			abs = filepath.Clean(p)
		} else {
			abs = filepath.Clean(filepath.Join(rootAbs, p))
		}
		// Reject anything that resolves outside the worktree —
		// belt-and-braces, since plan validation already enforces
		// repo-relative + no-escape.
		if rel, err := filepath.Rel(rootAbs, abs); err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if info, err := os.Stat(abs); err != nil || info.IsDir() {
			continue
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// planPythonFilesUnder is the python-specific shorthand for
// planFilesByExt(ctx, root, []string{".py"}). Kept named so
// runPyCompileFallback's intent reads cleanly.
func planPythonFilesUnder(ctx *types.BusContext, root string) []string {
	return planFilesByExt(ctx, root, []string{".py"})
}

// syntaxCheckExtensions returns the file extensions (with leading
// dot, lowercase) whose source we know how to parse with the
// runner's syntax-check tool. Empty result = no fallback for this
// runner, caller emits synthetic Passed instead.
func syntaxCheckExtensions(runner string) []string {
	switch runner {
	case "python":
		return []string{".py"}
	case "node":
		// node --check parses CommonJS / ESM JavaScript. NOT TypeScript
		// (needs tsc + project config). Skip .ts/.tsx here so we don't
		// produce false-positive errors on TS-only repos.
		return []string{".js", ".jsx", ".mjs", ".cjs"}
	case "ruby":
		return []string{".rb"}
	}
	return nil
}

// runSyntaxCheckFallback dispatches to the runner-specific syntax
// checker (py_compile / node --check / ruby -c). Returns
// (report, output, ok) — ok=false means this runner has no syntax-
// check fallback (the caller emits synthetic Passed).
func runSyntaxCheckFallback(ctx *types.BusContext, label, runnerRoot, runner string, files []string) (*types.ChangeReport, string, bool) {
	switch runner {
	case "python":
		report, output := runPyCompileFallback(ctx, label, runnerRoot, files)
		return report, output, true
	case "node":
		report, output := runNodeCheckFallback(ctx, label, runnerRoot, files)
		return report, output, true
	case "ruby":
		report, output := runRubyCheckFallback(ctx, label, runnerRoot, files)
		return report, output, true
	}
	return nil, "", false
}

func runSyntaxPreflightForPlan(ctx *types.BusContext, label, runnerRoot, runner string, done map[string]bool) (*types.ChangeReport, string, bool) {
	exts := syntaxCheckExtensions(runner)
	if len(exts) == 0 {
		return nil, "", false
	}
	files := planFilesByExt(ctx, runnerRoot, exts)
	if len(files) == 0 {
		return nil, "", false
	}
	key := runner + "\x00" + runnerRoot + "\x00" + strings.Join(files, "\x00")
	if done != nil {
		if done[key] {
			return nil, "", false
		}
		done[key] = true
	}
	report, output, ok := runSyntaxCheckFallback(ctx, label, runnerRoot, runner, files)
	if !ok {
		return nil, "", false
	}
	output = strings.Replace(output,
		"fallback (no test infrastructure detected; runner not invoked)",
		"syntax preflight (before project test runner)",
		1)
	if report != nil && !report.Passed {
		lang := ""
		if ctx != nil {
			lang = ctx.Language
		}
		if isZh(lang) {
			report.FailureSummary = fmt.Sprintf("%s 语法预检在 %d 个 plan 文件中发现 %d 处语法/解析错误 —— 项目测试尚未运行;请先修复列出的源文件。",
				runner, len(files), len(report.TestResults))
		} else {
			report.FailureSummary = fmt.Sprintf("%s syntax preflight found %d syntax/parse error(s) across %d plan file(s) before project tests were run; fix the listed source files first.",
				runner, len(report.TestResults), len(files))
		}
	}
	return report, output, true
}

// runNodeCheckFallback runs `node --check <file>` per JS file. node
// --check is a syntax-only parse that catches missing semicolons,
// unmatched braces, malformed expressions — the same class of errors
// that block any test runner from importing the file. Used when the
// repo has no node test infrastructure but the plan added .js files;
// the operator's intent is "did this code parse?", not "do my tests
// pass" (there are no tests).
func runNodeCheckFallback(ctx *types.BusContext, label, runnerRoot string, files []string) (*types.ChangeReport, string) {
	lang := ""
	if ctx != nil {
		lang = ctx.Language
	}
	bin := "node"
	if _, err := exec.LookPath(bin); err != nil {
		// Fall back to a synthetic Passed report — node missing AND
		// no tests means there is nothing meaningful we can verify
		// here; the operator gets a warning via the renderer.
		var skipSummary string
		if isZh(lang) {
			skipSummary = "node --check 兜底跳过:PATH 上没有 node 二进制;同时也无 node 测试基础设施,verify 标记 pass-with-warning。要验证 JS 代码请先安装 Node.js。"
		} else {
			skipSummary = "node --check fallback skipped: node binary not on PATH; no node test infrastructure either, so verify pass-with-warning. Install Node.js if you intend to verify JS code."
		}
		return &types.ChangeReport{
				Passed:         true,
				NoTestsRunners: []string{"node"},
				FailureSummary: skipSummary,
			},
			fmt.Sprintf("[run_tests: %s] node --check fallback skipped: node binary not on PATH\n", label)
	}
	var (
		failures []types.TestResult
		output   strings.Builder
	)
	output.WriteString(fmt.Sprintf("[run_tests: %s] node --check fallback (no node test infrastructure detected; runner not invoked)\n", label))
	for _, f := range files {
		cmd := exec.Command(bin, "--check", f)
		cmd.Dir = runnerRoot
		out, err := cmd.CombinedOutput()
		rel, relErr := filepath.Rel(runnerRoot, f)
		if relErr != nil {
			rel = f
		}
		if err == nil {
			output.WriteString(fmt.Sprintf("  ok    %s\n", rel))
			continue
		}
		failures = append(failures, types.TestResult{
			Kind:          types.TestResultKindBuildError,
			Suite:         "node_check",
			AssertionID:   rel,
			Passed:        false,
			FailureDetail: strings.TrimSpace(string(out)),
		})
		output.WriteString(fmt.Sprintf("  FAIL  %s: %v\n%s\n", rel, err,
			truncateForLog(string(out), 300)))
	}
	if len(failures) == 0 {
		return &types.ChangeReport{
			Passed:         true,
			NoTestsRunners: []string{"node"},
		}, output.String()
	}
	var summary string
	if isZh(lang) {
		summary = fmt.Sprintf("node --check 兜底在 %d 个 plan 文件中发现 %d 处语法错误;由于仓内无 node 测试基础设施,本次未跑 jest/vitest。",
			len(files), len(failures))
	} else {
		summary = fmt.Sprintf("node --check fallback found %d syntax error(s) across %d plan file(s); jest/vitest was not run because no node test infrastructure exists in the repo.",
			len(failures), len(files))
	}
	return &types.ChangeReport{
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: summary,
		FailureKind:    types.FailureKindBuildFailure,
		TestResults:    failures,
	}, output.String()
}

// runRubyCheckFallback runs `ruby -wc <file>` per .rb file. Same
// rationale as runNodeCheckFallback: when no rspec/minitest layout
// exists but the plan added .rb sources, the meaningful
// verification is "does this Ruby parse" — that's what -c does.
func runRubyCheckFallback(ctx *types.BusContext, label, runnerRoot string, files []string) (*types.ChangeReport, string) {
	lang := ""
	if ctx != nil {
		lang = ctx.Language
	}
	bin := "ruby"
	if _, err := exec.LookPath(bin); err != nil {
		var skipSummary string
		if isZh(lang) {
			skipSummary = "ruby -c 兜底跳过:PATH 上没有 ruby 二进制;同时也无 ruby 测试基础设施,verify 标记 pass-with-warning。要验证 .rb 代码请先安装 Ruby。"
		} else {
			skipSummary = "ruby -c fallback skipped: ruby binary not on PATH; no ruby test infrastructure either, so verify pass-with-warning. Install Ruby if you intend to verify .rb code."
		}
		return &types.ChangeReport{
				Passed:         true,
				NoTestsRunners: []string{"ruby"},
				FailureSummary: skipSummary,
			},
			fmt.Sprintf("[run_tests: %s] ruby -c fallback skipped: ruby binary not on PATH\n", label)
	}
	var (
		failures []types.TestResult
		output   strings.Builder
	)
	output.WriteString(fmt.Sprintf("[run_tests: %s] ruby -wc fallback (no ruby test infrastructure detected; runner not invoked)\n", label))
	for _, f := range files {
		cmd := exec.Command(bin, "-wc", f)
		cmd.Dir = runnerRoot
		out, err := cmd.CombinedOutput()
		rel, relErr := filepath.Rel(runnerRoot, f)
		if relErr != nil {
			rel = f
		}
		if err == nil {
			output.WriteString(fmt.Sprintf("  ok    %s\n", rel))
			continue
		}
		failures = append(failures, types.TestResult{
			Kind:          types.TestResultKindBuildError,
			Suite:         "ruby_check",
			AssertionID:   rel,
			Passed:        false,
			FailureDetail: strings.TrimSpace(string(out)),
		})
		output.WriteString(fmt.Sprintf("  FAIL  %s: %v\n%s\n", rel, err,
			truncateForLog(string(out), 300)))
	}
	if len(failures) == 0 {
		return &types.ChangeReport{
			Passed:         true,
			NoTestsRunners: []string{"ruby"},
		}, output.String()
	}
	var summary string
	if isZh(lang) {
		summary = fmt.Sprintf("ruby -wc 兜底在 %d 个 plan 文件中发现 %d 处语法错误;由于仓内无 ruby 测试基础设施,本次未跑 rspec/minitest。",
			len(files), len(failures))
	} else {
		summary = fmt.Sprintf("ruby -wc fallback found %d syntax error(s) across %d plan file(s); rspec/minitest was not run because no ruby test infrastructure exists in the repo.",
			len(failures), len(files))
	}
	return &types.ChangeReport{
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: summary,
		FailureKind:    types.FailureKindBuildFailure,
		TestResults:    failures,
	}, output.String()
}

// runPyCompileFallback runs `python3 -m py_compile <file>` for each
// supplied python file under the worktree, then synthesises a
// ChangeReport reflecting compile success/failure. Used when the
// plan creates a bare python script in a repo with no test
// infrastructure — pytest would fail (no tests + maybe not even
// installed) but py_compile is the syntax-level "did this code parse"
// check that maps onto the operator's actual intent.
//
// Returns (report, output-text) where output-text is the human-
// readable section appended to combinedOutputs. The interpreter is
// resolved via pythonInterpreter so the venv probe + Fix A interleaved
// order applies here too — the operator's main-repo .venv with pytest
// installed will also have py_compile (every venv does).
func runPyCompileFallback(ctx *types.BusContext, label, runnerRoot string, pyFiles []string) (*types.ChangeReport, string) {
	mainRoot := ""
	lang := ""
	if ctx != nil {
		mainRoot = ctx.MainRepoRoot
		lang = ctx.Language
	}
	interp, asModule := pythonInterpreter(runnerRoot, mainRoot)
	exePath := interp
	fixedArgs := []string(nil)
	if asModule && interp != "" && interp != "python3" && interp != "python" {
		// Project venv path (or another concrete interpreter path) already
		// won the pythonInterpreter probe; keep it so py_compile runs in the
		// same environment as the repo's tests would.
		exePath = interp
	} else if runner, ok := resolvePythonDryBuildRunner(); ok {
		// Reuse the dry-build probe so Windows PATH shims like
		// WindowsApps/python3 don't get mistaken for a usable interpreter.
		exePath = runner.ExePath
		fixedArgs = append([]string{}, runner.FixedArgs...)
	} else {
		// Bare `pytest` resolution as the python-interpreter implies the
		// host has no python3/python at all. py_compile cannot run on
		// pytest. Fall back to literal python3 and let it fail loudly if
		// it's also missing — the operator needs the env fix either way.
		if strings.TrimSpace(exePath) == "" || exePath == "pytest" {
			exePath = "python3"
		}
	}
	var (
		failures []types.TestResult
		output   strings.Builder
	)
	output.WriteString(fmt.Sprintf("[run_tests: %s] py_compile fallback (no test infrastructure detected; runner not invoked)\n",
		label))
	for _, f := range pyFiles {
		args := append(append([]string{}, fixedArgs...), "-m", "py_compile", f)
		cmd := exec.Command(exePath, args...)
		cmd.Dir = runnerRoot
		out, err := cmd.CombinedOutput()
		rel, relErr := filepath.Rel(runnerRoot, f)
		if relErr != nil {
			rel = f
		}
		if err == nil {
			output.WriteString(fmt.Sprintf("  ok    %s\n", rel))
			continue
		}
		failureDetail := strings.TrimSpace(string(out))
		if failureDetail == "" {
			failureDetail = err.Error()
		}
		failures = append(failures, types.TestResult{
			Kind:          types.TestResultKindBuildError,
			Suite:         "py_compile",
			AssertionID:   rel,
			Passed:        false,
			FailureDetail: failureDetail,
		})
		logText := strings.TrimSpace(string(out))
		if logText == "" {
			logText = failureDetail
		}
		output.WriteString(fmt.Sprintf("  FAIL  %s: %v\n%s\n", rel, err,
			truncateForLog(logText, 300)))
	}
	if len(failures) == 0 {
		return &types.ChangeReport{
			Passed:         true,
			NoTestsRunners: []string{"python"},
		}, output.String()
	}
	var summary string
	if isZh(lang) {
		summary = fmt.Sprintf("py_compile 兜底在 %d 个 plan 文件中发现 %d 处语法/导入错误 —— 请修复列出的 Python 文件;由于仓内无 pytest 基础设施,本次未跑 pytest。",
			len(pyFiles), len(failures))
	} else {
		summary = fmt.Sprintf("py_compile fallback found %d syntax/import error(s) across %d plan file(s) — fix the listed Python files; pytest was not run because no test infrastructure exists in the repo.",
			len(failures), len(pyFiles))
	}
	return &types.ChangeReport{
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: summary,
		FailureKind:    types.FailureKindBuildFailure,
		TestResults:    failures,
	}, output.String()
}

// warningPassedReport synthesises a Passed=true ChangeReport that
// also carries an advisory FailureSummary noting the runner was
// missing but no test work was needed. Used by the fix-C tier path:
// runner_missing + tests_exist=false → don't fail verify, but DO
// warn the operator so they can install the runner if they intended
// to add tests later.
func warningPassedReport(runner, missingBinary, reason string, exitCode int, output, lang string) *types.ChangeReport {
	excerpt := truncateForLog(output, 200)
	var summary string
	if isZh(lang) {
		summary = fmt.Sprintf(
			"runner %q 的主二进制 %q 不在 PATH(子进程 exit=%d, 信号=%s),但工作目录无 %s 测试基础设施 —— 无测试任务可跑,verify 标记 pass-with-warning。如果后续要加测试,请安装 %s。实际输出片段: %s",
			runner, missingBinary, exitCode, reason, runner, missingBinary, excerpt)
	} else {
		summary = fmt.Sprintf(
			"runner %q's primary binary %q not on PATH (subprocess exit=%d, trigger=%s), but the working dir has no %s test infrastructure — no test work to do, verify pass-with-warning. Install %s if you plan to add tests later. Actual output excerpt: %s",
			runner, missingBinary, exitCode, reason, runner, missingBinary, excerpt)
	}
	return &types.ChangeReport{
		Passed:         true,
		NoTestsRunners: []string{runner},
		FailureSummary: summary,
	}
}

// hasNoTestSources returns true when a recursive scan of root finds
// zero files matching the runner's standard test-file discovery
// conventions. Used as a pre-flight before invoking the runner: if
// the repo has nothing to test, there is no point in launching the
// runner — and on hosts where the runner binary is missing the
// invocation would falsely fail verify with FailureKindRunnerMissing,
// which is the customer-reported scenario this gates against.
//
// Returns false (the safe default — invoke the runner) for runners
// whose discovery is too project-shape-dependent to reliably pre-check:
// rust (#[test] inline annotations), cmake (CMakeLists.txt-driven),
// meson (meson.build-driven), make (target-driven), hvigor / cjpm
// (HarmonyOS / Cangjie project conventions vary).
//
// The walk skips well-known irrelevant directories (.git, build
// caches, dependency vendors) so it stays cheap on large monorepos.
// Total cost on a clean source tree is one or two stat passes — the
// happy path bails early on the first matching file.
func hasNoTestSources(runner, root string) bool {
	matcher := testFileMatcher(runner)
	if matcher == nil {
		return false
	}
	if root == "" {
		return false
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return false
	}
	stop := errors.New("found-test-file")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path == root {
				return nil
			}
			switch name {
			case ".git", ".hg", ".svn",
				"node_modules", "vendor", "target",
				"dist", "build", "out", ".tox",
				".venv", "venv", "env", ".virtualenv",
				"__pycache__", ".pytest_cache", ".mypy_cache",
				".idea", ".vscode", ".gradle", ".mvn",
				".codrax":
				return filepath.SkipDir
			}
			return nil
		}
		if matcher(path, name) {
			return stop
		}
		return nil
	})
	if errors.Is(err, stop) {
		return false
	}
	return true
}

// testFileMatcher returns the per-runner predicate used by
// hasNoTestSources. Returning nil signals "this runner does not opt
// into pre-flight skip" — the caller falls through to invoking the
// runner. Conventions are deliberately conservative: a false-positive
// (matcher fires on a non-test file) produces a wasted runner exec;
// a false-negative (matcher misses a real test) produces an incorrect
// pre-flight pass. The latter is the worse failure mode, so the
// matchers err on the side of NOT firing.
func testFileMatcher(runner string) func(path, name string) bool {
	switch runner {
	case "go":
		return func(_, name string) bool {
			return strings.HasSuffix(name, "_test.go")
		}
	case "python":
		return func(_, name string) bool {
			if name == "conftest.py" {
				return true
			}
			if strings.HasSuffix(name, ".py") &&
				(strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "_test.py")) {
				return true
			}
			return false
		}
	case "node":
		return func(path, name string) bool {
			lower := strings.ToLower(name)
			for _, frag := range []string{".test.", ".spec."} {
				if strings.Contains(lower, frag) && (strings.HasSuffix(lower, ".js") ||
					strings.HasSuffix(lower, ".jsx") ||
					strings.HasSuffix(lower, ".ts") ||
					strings.HasSuffix(lower, ".tsx") ||
					strings.HasSuffix(lower, ".mjs") ||
					strings.HasSuffix(lower, ".cjs")) {
					return true
				}
			}
			// jest's __tests__/ directory: any JS/TS file inside
			// counts as a test by convention. Walk up the path looking
			// for the dir component.
			for _, comp := range strings.Split(filepath.ToSlash(path), "/") {
				if comp == "__tests__" {
					return strings.HasSuffix(lower, ".js") ||
						strings.HasSuffix(lower, ".jsx") ||
						strings.HasSuffix(lower, ".ts") ||
						strings.HasSuffix(lower, ".tsx") ||
						strings.HasSuffix(lower, ".mjs") ||
						strings.HasSuffix(lower, ".cjs")
				}
			}
			return false
		}
	case "ruby":
		return func(_, name string) bool {
			return strings.HasSuffix(name, "_spec.rb") || strings.HasSuffix(name, "_test.rb")
		}
	case "swift":
		return func(_, name string) bool {
			return strings.HasSuffix(name, "Tests.swift") || strings.HasSuffix(name, "Test.swift")
		}
	case "java":
		return func(_, name string) bool {
			if !strings.HasSuffix(name, ".java") {
				return false
			}
			base := strings.TrimSuffix(name, ".java")
			return strings.HasPrefix(base, "Test") ||
				strings.HasSuffix(base, "Test") ||
				strings.HasSuffix(base, "Tests") ||
				strings.HasSuffix(base, "IT") || // Maven Failsafe integration test
				strings.HasSuffix(base, "ITCase") ||
				strings.HasSuffix(base, "TestCase")
		}
	}
	return nil
}

// pythonInterpreter resolves the Python invocation to use for the
// repository under test. Returns (path-or-name, asModule) where
// asModule tells the caller whether to invoke as
// `<interp> -m pytest` (the recommended pytest documentation form;
// works for any python+pytest install and adds CWD to sys.path) or
// to fall back to a bare `pytest` invocation.
//
// roots is the ordered list of repo paths to probe for a venv —
// caller typically passes (worktreeRoot, mainRepoRoot). The first
// root with a recognised venv layout wins. Empty / duplicate roots
// are tolerated. Why both roots: during verify the worktree is a
// detached-HEAD checkout that excludes gitignored paths (`.venv/`
// is conventionally gitignored), so the venv exists only at the
// user's main repo. The pre-fix code probed the worktree and missed
// it; threading mainRepoRoot through fixes the customer-reported
// "I just installed pytest in .venv but codrax says runner_missing".
//
// Priority order:
//
//  1. Project venv binaries (per root, in roots order):
//     `.venv/bin/python` (Unix) / `.venv\Scripts\python.exe`
//     (Windows), then `venv/`, `env/`, `.virtualenv/` variants. When
//     a venv exists, its python sees the project's installed
//     dependencies; running pytest from outside the venv would
//     either miss the project's deps or use a different interpreter
//     than `pip install` populated.
//  2. Standalone `pytest` on PATH — preferred over `python3 -m pytest`
//     because the pytest entry-point script's shebang already points
//     at the python interpreter that pip installed pytest under, so
//     `import pytest` is guaranteed to succeed. The earlier order
//     (python3 -m pytest first) silently fails on hosts where the
//     resolved python3 is a different build from the one pip used —
//     e.g. customer-reported `/opt/python/bin/python3 -m pytest` →
//     "No module named pytest" when pytest itself was installed via
//     the system python (`apt install python3-pytest` /
//     `pip install --user pytest` on a different python). The
//     "python -m" CWD-on-sys.path benefit is preserved because pytest
//     adds rootdir / conftest dirs to sys.path during its own
//     rootdir discovery, so source-only repos still resolve their
//     own packages without `pip install -e .`.
//  3. `python3` from PATH — fallback when standalone pytest is
//     missing. Modern Linux/macOS distros ship `python3` only (the
//     `python` symlink to Python 2 was removed). asModule=true so
//     the caller invokes `python3 -m pytest`.
//  4. `python` from PATH — older systems / Windows.
//  5. Bare `pytest` (asModule=false) — last resort, only when no
//     python interpreter at all resolves. The runner_missing detector
//     catches this if pytest is also absent.
//
// Cross-platform: filepath.Join uses the OS separator, so on
// Windows the venv probe produces backslash paths. filepath.ToSlash
// normalises to forward slashes for shell consumption — both the
// codrax-default sh wrapper (Git for Windows) and cmd.exe accept
// forward-slash paths on Windows, and avoiding backslashes
// sidesteps shell-escape ambiguity (`\v`, `\t` etc as escape codes
// in some sh implementations).
func pythonInterpreter(roots ...string) (string, bool) {
	venvDirs := []string{".venv", "venv", "env", ".virtualenv"}
	venvSubpaths := []string{
		filepath.Join("bin", "python"),
		filepath.Join("bin", "python3"),
		filepath.Join("Scripts", "python.exe"),
	}
	seen := make(map[string]bool, len(roots))
	dedupedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		dedupedRoots = append(dedupedRoots, root)
	}
	// Interleaved probe order (fix A in the user design spec): all
	// roots probed for `.venv` first, then all roots probed for
	// `venv`, then `env`, then `.virtualenv`. This ensures that when
	// the user has a `.venv` at the main repo root AND a stray
	// `venv` at the worktree (or vice versa), the more conventional
	// `.venv` wins regardless of which root it lives in. The
	// pre-fix order (all dirs of root[0], then all dirs of root[1])
	// would prefer worktree's `venv` over mainRepo's `.venv` — which
	// is wrong: `.venv` is the modern convention, mainRepo is where
	// the user typed `pip install pytest`.
	for _, dir := range venvDirs {
		for _, root := range dedupedRoots {
			for _, sub := range venvSubpaths {
				candidate := filepath.Join(root, dir, sub)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					logging.Info("[run_tests/python] interpreter resolved: venv=%q (root=%q dir=%q sub=%q; probed roots=%v interleaved)",
						filepath.ToSlash(candidate), root, dir, sub, dedupedRoots)
					return filepath.ToSlash(candidate), true
				}
			}
		}
	}
	// Standalone `pytest` wins over `<python> -m pytest` because its
	// shebang resolves to the python pip installed it under.
	if path, err := exec.LookPath("pytest"); err == nil {
		logging.Info("[run_tests/python] interpreter resolved: PATH pytest=%q (no venv at: %v; preferred over `python -m pytest` because pytest's shebang knows its own python)",
			path, dedupedRoots)
		return "pytest", false
	}
	if path, err := exec.LookPath("python3"); err == nil {
		logging.Info("[run_tests/python] interpreter resolved: PATH python3=%q (no venv at: %v; standalone pytest absent)",
			path, dedupedRoots)
		return "python3", true
	}
	if path, err := exec.LookPath("python"); err == nil {
		logging.Info("[run_tests/python] interpreter resolved: PATH python=%q (no venv at: %v; standalone pytest + python3 absent)",
			path, dedupedRoots)
		return "python", true
	}
	logging.Warning("[run_tests/python] no python interpreter found (probed venv at: %v; pytest / python3 / python all absent from PATH); falling back to bare pytest invocation which will fail loudly",
		dedupedRoots)
	return "pytest", false
}

func pythonRuntimeInterpreter(roots ...string) string {
	venvDirs := []string{".venv", "venv", "env", ".virtualenv"}
	venvSubpaths := []string{
		filepath.Join("bin", "python"),
		filepath.Join("bin", "python3"),
		filepath.Join("Scripts", "python.exe"),
	}
	seen := make(map[string]bool, len(roots))
	dedupedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		dedupedRoots = append(dedupedRoots, root)
	}
	for _, dir := range venvDirs {
		for _, root := range dedupedRoots {
			for _, sub := range venvSubpaths {
				candidate := filepath.Join(root, dir, sub)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					logging.Info("[run_tests/python] runtime resolved: venv=%q (root=%q dir=%q sub=%q; probed roots=%v interleaved)",
						filepath.ToSlash(candidate), root, dir, sub, dedupedRoots)
					return filepath.ToSlash(candidate)
				}
			}
		}
	}
	if path, err := exec.LookPath("python3"); err == nil {
		logging.Info("[run_tests/python] runtime resolved: PATH python3=%q (no venv at: %v)", path, dedupedRoots)
		return "python3"
	}
	if path, err := exec.LookPath("python"); err == nil {
		logging.Info("[run_tests/python] runtime resolved: PATH python=%q (no venv/python3 at: %v)", path, dedupedRoots)
		return "python"
	}
	logging.Warning("[run_tests/python] no python runtime found (probed venv at: %v; python3 / python absent from PATH); falling back to python3 which will fail loudly",
		dedupedRoots)
	return "python3"
}

// runnerInstallHint maps a runner to a short install suggestion the
// user-facing error embeds. Bilingual zh (default) / en. Drift-guard
// test TestRunnerInstallHint_AllRunnersCovered asserts every
// allowedRunners entry has a non-empty hint in zh.
//
// Inline shell commands are prefixed with `!` so the operator can
// copy-paste them directly into the codrax REPL prompt — the bang
// shell-out runs the command in REPL's CWD, which is the main repo
// for the typical /approve flow. Pure URLs / package-manager hints
// without a runnable command stay plain prose.
func runnerInstallHint(runner, lang string) string {
	zh := isZh(lang)
	switch runner {
	case "go":
		if zh {
			return "从 https://go.dev/dl/ 安装 Go(或用发行版包管理器)"
		}
		return "install Go from https://go.dev/dl/ (or your distro's package manager)"
	case "python":
		if zh {
			return "在项目 venv 里装(`!python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report`)或用系统 python(`!pip install pytest pytest-json-report`);codrax 会自动识别仓根的 .venv / venv / env / .virtualenv 目录(`!` 前缀让 codrax 直接执行系统命令)"
		}
		return "install pytest in the project venv (`!python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report`) or for the system python (`!pip install pytest pytest-json-report`); codrax auto-detects .venv / venv / env / .virtualenv directories at the repo root (the `!` prefix runs commands directly from the codrax prompt)"
	case "rust":
		if zh {
			return "从 https://rustup.rs/ 安装 Rust + Cargo"
		}
		return "install Rust + Cargo from https://rustup.rs/"
	case "java":
		if zh {
			return "安装 Maven(`mvn`)或使用项目自带的 Gradle wrapper(`!./gradlew test`)"
		}
		return "install Maven (`mvn`) or use the project's Gradle wrapper (`!./gradlew test`)"
	case "ruby":
		if zh {
			return "在 codrax 提示符执行 `!gem install bundler` 装 Bundler,然后 `!bundle install`(`!` 前缀让 codrax 直接执行系统命令)"
		}
		return "from the codrax prompt run `!gem install bundler` and then `!bundle install` (the `!` prefix shells out directly)"
	case "swift":
		if zh {
			return "从 https://swift.org/download/ 安装 Swift 工具链"
		}
		return "install the Swift toolchain from https://swift.org/download/"
	case "cmake":
		if zh {
			return "从 https://cmake.org/download/ 安装 CMake(自带 ctest),并先配置好 build 目录(`!cmake -S . -B build`)"
		}
		return "install CMake (provides ctest) from https://cmake.org/download/ and configure a build dir first (`!cmake -S . -B build`)"
	case "meson":
		if zh {
			return "用 `!pip install meson` 安装 Meson(并装 ninja),再配置 build 目录(`!meson setup builddir`)"
		}
		return "install Meson with `!pip install meson` (and ninja) and configure a build dir first (`!meson setup builddir`)"
	case "make":
		if zh {
			return "用发行版包管理器装 GNU make"
		}
		return "install GNU make from your distro's package manager"
	case "hvigor":
		if zh {
			return "安装 HarmonyOS DevEco 命令行工具(自带 hvigorw)"
		}
		return "install the HarmonyOS DevEco command-line tools (provides hvigorw)"
	case "cjpm":
		if zh {
			return "安装 Cangjie 工具链(自带 cjpm)"
		}
		return "install the Cangjie toolchain (provides cjpm)"
	case "node":
		if zh {
			return "从 https://nodejs.org/ 安装 Node.js(自带 npm)"
		}
		return "install Node.js (https://nodejs.org/) which bundles npm"
	}
	if zh {
		return "安装该 runner 的 CLI 二进制"
	}
	return "install the runner's CLI binary"
}

func runnerInstallHintForPlan(plan runnerPlan, missingBinary, lang string) string {
	if plan.Runner == "python" && plan.Framework == pythonFrameworkUnittest {
		if isZh(lang) {
			return "安装 Python 3 或确保项目 venv 里的 python 可执行；unittest 是 Python 标准库，不需要安装 pytest"
		}
		return "install Python 3 or ensure the project venv python is executable; unittest is in the Python standard library and does not require pytest"
	}
	if plan.Runner == "python" && strings.TrimSpace(missingBinary) != "" && strings.TrimSpace(missingBinary) != "pytest" {
		if isZh(lang) {
			return "安装 Python 3 或修复项目 venv；pytest 项目还需要 pytest 与 pytest-json-report"
		}
		return "install Python 3 or repair the project venv; pytest projects also need pytest and pytest-json-report"
	}
	return runnerInstallHint(plan.Runner, lang)
}

// runnerMissingToolResultSummary builds the bilingual ToolResult.
// Summary text rendered to the user's terminal when a runner
// binary is missing. Mirrors the makeRunnerMissingReport prose
// (which is what gets stored on ChangeReport) so the user sees
// consistent wording in both surfaces.
//
// reason / exitCode / output are diagnostic context — the trigger
// signal that fired (exit_code_127 / exec.ErrNotFound / "pattern: …"),
// the actual subprocess exit code, and a short stderr excerpt. Without
// these the only signal the LLM and user got was a generic install
// hint, which masked cases like "python3 -m pytest exits 1 with
// 'No module named pytest' under a different python than the one
// pip installed pytest into" — diagnosable only with the actual
// exit code + stderr in front of the operator.
func runnerMissingToolResultSummary(lang, label, missingBinary, hint, reason string, exitCode int, output string) string {
	excerpt := truncateForLog(output, 300)
	if isZh(lang) {
		return fmt.Sprintf(
			"[run_tests: %s] 子进程退出码=%d, 触发信号=%s, 主二进制=%q 在本环境不可调用 —— 重新跑 verify 之前请先安装;%s\n实际输出片段: %s",
			label, exitCode, reason, missingBinary, hint, excerpt)
	}
	return fmt.Sprintf(
		"[run_tests: %s] subprocess exit=%d trigger=%s runner binary %q not invokable in this env — install it before re-running verify; %s\nactual output excerpt: %s",
		label, exitCode, reason, missingBinary, hint, excerpt)
}

// isZh mirrors the REPL helper of the same name. Duplicated rather
// than imported to avoid an internal/tool → internal/repl edge.
func isZh(lang string) bool {
	return !strings.EqualFold(strings.TrimSpace(lang), "en")
}

// makeRunnerMissingReport produces a ChangeReport tagged with
// FailureKindRunnerMissing so downstream consumers (the verify→plan
// retry suppressor in particular) can route on a single field. The
// failure is BuildFailed=true because the runner never reached test
// execution — same lifecycle slot as a compile error.
//
// Bilingual: lang=zh (default) renders the FailureSummary in
// Simplified Chinese; lang=en renders English. This text reaches
// the user's terminal via the verify-failure renderer, so it must
// match the rest of the REPL chrome's language.
func makeRunnerMissingReport(runner, binary, output, lang, reason string, exitCode int) *types.ChangeReport {
	excerpt := strings.TrimSpace(output)
	if len(excerpt) > 600 {
		excerpt = excerpt[:600] + "\n…[output truncated]"
	}
	hint := runnerInstallHint(runner, lang)
	var summary string
	if isZh(lang) {
		summary = fmt.Sprintf(
			"runner %q 的主二进制 %q 未在本环境安装(子进程 exit=%d, 触发信号=%s) —— %s。安装后重新跑 verify;planner 无法修复缺失的依赖。",
			runner, binary, exitCode, reason, hint)
	} else {
		summary = fmt.Sprintf(
			"runner %q's primary binary %q is not installed in this environment "+
				"(subprocess exit=%d, trigger=%s) — %s. "+
				"Re-run verify after installing the tool; the planner cannot fix a missing dependency.",
			runner, binary, exitCode, reason, hint)
	}
	return &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   string(types.FailureKindRunnerMissing),
			Suite:         "runner_missing",
			Passed:        false,
			FailureDetail: excerpt,
		}},
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: summary,
		FailureKind:    types.FailureKindRunnerMissing,
	}
}

func makeBuildFailureReport(label, output string) *types.ChangeReport {
	excerpt := narrativeBuildErrorExcerpt(output)
	buildErrs := parseBuildErrors(output)
	snippet := ""
	if excerpt != "" {
		snippet = strings.SplitN(excerpt, "\n", 2)[0]
	}
	failSummary := renderBuildFailureSummary(label, buildErrs, snippet)
	if excerpt == "" {
		excerpt = failSummary
	}
	return &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   firstBuildErrorAssertionID(buildErrs),
			Suite:         "build",
			Passed:        false,
			FailureDetail: excerpt,
			BuildErrors:   buildErrs,
		}},
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: failSummary,
	}
}

// detectNativeBuildDir locates a pre-configured build directory for
// a CMake or Meson project. Both tools require an out-of-tree build
// dir with their generator artifacts (Ninja files / Makefiles /
// CTestTestfile.cmake for CMake; meson-info/ for Meson); run_tests
// does NOT run the configure step itself (too slow, too many knobs).
// Returns the first match from the candidates list, or "" if the
// project hasn't been configured yet.
//
// Candidates cover the common in-repo layouts:
//
//   - "build" / "Build"        —cmake -S . -B build convention
//   - "builddir"               —meson default
//   - "out"                    —Google/Android-style
//   - "cmake-build-debug" and
//     "cmake-build-release"    —CLion IDE defaults
//
// Operators using a non-standard layout (e.g. sibling build dirs
// outside the repo root) can't be auto-detected; they must invoke
// tests via a project-specific script and fall through to the
// make runner.
func detectNativeBuildDir(repoRoot string) string {
	candidates := []string{
		"build", "Build",
		"builddir",
		"out",
		"cmake-build-debug",
		"cmake-build-release",
	}
	for _, name := range candidates {
		abs := filepath.Join(repoRoot, name)
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			continue
		}
		// Peek for a generator sentinel so an empty dir left over from
		// a failed configure doesn't count. CMake drops
		// CMakeCache.txt; Meson drops meson-info/. We accept either —
		// the parser step tolerates whichever output shape the command
		// produces.
		if _, err := os.Stat(filepath.Join(abs, "CMakeCache.txt")); err == nil {
			return abs
		}
		if _, err := os.Stat(filepath.Join(abs, "meson-info")); err == nil {
			return abs
		}
	}
	return ""
}

// detectJavaBuildSystem returns "maven" when pom.xml exists, else
// "gradle" when build.gradle[.kts] exists. Called by
// buildRunCommand when runner=="java" so the command and the
// post-exec report locator both know which layout to target.
func detectJavaBuildSystem(repoRoot string) string {
	if _, err := os.Stat(filepath.Join(repoRoot, "pom.xml")); err == nil {
		return "maven"
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "build.gradle")); err == nil {
		return "gradle"
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "build.gradle.kts")); err == nil {
		return "gradle"
	}
	return ""
}

// locateJUnitReportDir walks well-known JUnit XML output locations
// and returns the FIRST non-empty directory it finds. Returns ""
// when no candidate directory carries XML (build failed before tests
// ran; caller surfaces a clear error).
//
// Priority order:
//
//  1. Maven's target/surefire-reports (Maven projects with a stale
//     Gradle build/ dir from an earlier toolchain switch shouldn't
//     accidentally point us there).
//  2. Gradle's build/test-results/test.
//  3. HarmonyOS hvigor commonly drops JUnit XML under one of:
//     `entry/build/default/intermediates/test/test-results/`
//     `<module>/build/intermediates/test/test-results/`
//     The candidate list covers the documented Stage Model layouts
//     plus a generic fallback at .codrax/test-results/.
//  4. Final fallback: bounded walk of the worktree looking for any
//     directory whose name ends in `test-results` / `test-result` /
//     `surefire-reports` and contains `.xml` files. Walk depth is
//     capped at 6 levels so we don't traverse node_modules / .git.
//
// Multi-module caveat: polyglot Java/Gradle repos may have multiple
// surefire-reports directories under submodules. The parser's XML
// walk handles that —we return the highest non-empty match and let
// parseJUnitXMLDir recurse.
func locateJUnitReportDir(repoRoot string) string {
	candidates := []string{
		filepath.Join(repoRoot, "target", "surefire-reports"),
		filepath.Join(repoRoot, "build", "test-results", "test"),
		// HarmonyOS hvigor —Stage Model entry module.
		filepath.Join(repoRoot, "entry", "build", "default", "intermediates", "test", "test-results"),
		filepath.Join(repoRoot, "entry", "build", "intermediates", "test", "test-results"),
		filepath.Join(repoRoot, "build", "intermediates", "test", "test-results"),
		filepath.Join(repoRoot, ".codrax", "test-results"),
	}
	for _, dir := range candidates {
		if dirHasXML(dir) {
			return dir
		}
	}
	// Bounded fallback walk. Catches non-standard hvigor layouts and
	// custom test-runner configurations without hardcoding every
	// possible output dir. Depth-cap 6 keeps the walk cheap on large
	// monorepos; skip-list excludes common large/unrelated dirs.
	if found := walkJUnitDir(repoRoot, 6); found != "" {
		return found
	}
	return ""
}

// dirHasXML reports whether dir exists and contains at least one
// `*.xml` file at its top level (we don't recurse —a directory with
// only nested .xml is a hint we're at the wrong level and should
// keep walking).
func dirHasXML(dir string) bool {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
			return true
		}
	}
	return false
}

// walkJUnitDir descends from root looking for a directory whose
// basename matches the JUnit-output convention (`test-results`,
// `surefire-reports`, `test-report`) AND that contains at least one
// .xml file. Returns the first match, or "" on no match.
//
// Skip list excludes common large dirs that won't carry test
// reports (node_modules, .git, vendor, dist, etc.) so the walk stays
// cheap even on monorepos.
func walkJUnitDir(root string, maxDepth int) string {
	skip := map[string]struct{}{
		"node_modules": {},
		".git":         {},
		"vendor":       {},
		"dist":         {},
		"out":          {},
		".gradle":      {},
		".idea":        {},
		".vscode":      {},
	}
	matchNames := []string{"test-results", "surefire-reports", "test-report"}
	var found string
	var walk func(dir string, depth int) bool
	walk = func(dir string, depth int) bool {
		if depth > maxDepth || found != "" {
			return false
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if _, skipped := skip[e.Name()]; skipped {
				continue
			}
			full := filepath.Join(dir, e.Name())
			for _, m := range matchNames {
				if strings.HasSuffix(e.Name(), m) {
					if dirHasXML(full) {
						found = full
						return true
					}
					// Sometimes the XML is one level deeper (Gradle
					// puts files under <suite>/ TEST-*.xml). Peek a
					// single child level for any .xml.
					if firstXMLDescendant(full, 2) {
						found = full
						return true
					}
				}
			}
			if walk(full, depth+1) {
				return true
			}
		}
		return false
	}
	walk(root, 0)
	return found
}

// firstXMLDescendant returns true if any .xml file lives within
// the given dir, walking up to maxDepth levels (1 = direct children
// only). Used by walkJUnitDir to detect Gradle-style nested layouts
// where the test-results dir contains a `test/` subfolder before the
// XML files.
func firstXMLDescendant(dir string, maxDepth int) bool {
	if maxDepth < 0 {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			if strings.HasSuffix(strings.ToLower(e.Name()), ".xml") {
				return true
			}
			continue
		}
		if firstXMLDescendant(filepath.Join(dir, e.Name()), maxDepth-1) {
			return true
		}
	}
	return false
}

// buildRunCommand assembles the shell command string for a runner.
// Returns (command, extraFile) —extraFile is a temp file path
// the runner writes its JSON output to (pytest-json-report uses
// a file argument; other runners print to stdout). Empty extraFile
// means parse from stdout.
// buildRunCommand assembles the shell command string for a runner.
// repoRoot is the runner's working dir (worktree during verify);
// mainRoot is the original repo path (BusContext.MainRepoRoot)
// — only used by the python branch, where the user's venv often
// lives at mainRoot/.venv/ but never reaches the worktree because
// .venv/ is gitignored.
func buildRunCommandForPlan(plan runnerPlan, suite, mainRoot string) (string, string) {
	return buildRunCommandWithFramework(plan.Runner, plan.Framework, suite, plan.Root, mainRoot)
}

func buildPlainPytestFallbackCommandForPlan(plan runnerPlan, suite, mainRoot string) string {
	commands := buildPlainPytestFallbackCommandsForPlan(plan, suite, mainRoot)
	if len(commands) == 0 {
		return ""
	}
	return commands[0]
}

func buildPlainPytestFallbackCommandsForPlan(plan runnerPlan, suite, mainRoot string) []string {
	interp, asModule := pythonInterpreter(plan.Root, mainRoot)
	filter := shellQuotePytestSelectorsForRoot(suite, plan.Root)
	prefix := "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 "
	var commands []string
	if asModule {
		if filter == "" {
			return []string{prefix + fmt.Sprintf("%s -m pytest -v", interp)}
		}
		return []string{prefix + fmt.Sprintf("%s -m pytest -v %s", interp, filter)}
	}
	if filter == "" {
		commands = append(commands, prefix+fmt.Sprintf("%s -v", interp))
	} else {
		commands = append(commands, prefix+fmt.Sprintf("%s -v %s", interp, filter))
	}
	moduleInterp := pythonRuntimeInterpreter(plan.Root, mainRoot)
	var moduleCmd string
	if filter == "" {
		moduleCmd = prefix + fmt.Sprintf("%s -m pytest -v", moduleInterp)
	} else {
		moduleCmd = prefix + fmt.Sprintf("%s -m pytest -v %s", moduleInterp, filter)
	}
	duplicate := false
	for _, cmd := range commands {
		if cmd == moduleCmd {
			duplicate = true
			break
		}
	}
	if !duplicate {
		commands = append(commands, moduleCmd)
	}
	return commands
}

func shellQuotePytestSelectors(suite string) string {
	return shellQuotePytestSelectorsForRoot(suite, "")
}

func shellQuotePytestSelectorsForRoot(suite, repoRoot string) string {
	selectors := splitPytestSuiteSelectors(suite)
	if len(selectors) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		selector = normalizePytestSelectorForRoot(selector, repoRoot)
		quoted = append(quoted, shellQuoteWord(selector))
	}
	return strings.Join(quoted, " ")
}

func normalizePytestSelectorForRoot(selector, repoRoot string) string {
	selector = strings.TrimSpace(selector)
	repoRoot = strings.TrimSpace(repoRoot)
	if selector == "" || repoRoot == "" {
		return selector
	}
	pathPart, suffix, ok := splitPytestSelectorPath(selector)
	if !ok || pathPart == "" || filepath.IsAbs(pathPart) {
		return selector
	}
	pathPart = filepath.ToSlash(filepath.Clean(pathPart))
	if pathPart == "." || strings.HasPrefix(pathPart, "../") || strings.Contains(pathPart, "/../") {
		return selector
	}
	if pytestSelectorFileExists(filepath.Join(repoRoot, filepath.FromSlash(pathPart))) {
		return selector
	}
	parts := strings.Split(pathPart, "/")
	for i := 1; i < len(parts); i++ {
		candidate := strings.Join(parts[i:], "/")
		if candidate == "" || !pytestSelectorFileExists(filepath.Join(repoRoot, filepath.FromSlash(candidate))) {
			continue
		}
		return candidate + suffix
	}
	return selector
}

func splitPytestSelectorPath(selector string) (pathPart, suffix string, ok bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.ContainsAny(selector, " \t\r\n") {
		return "", "", false
	}
	pathPart = selector
	if idx := strings.Index(selector, "::"); idx >= 0 {
		pathPart = selector[:idx]
		suffix = selector[idx:]
	}
	if !strings.Contains(filepath.ToSlash(pathPart), ".py") {
		return "", "", false
	}
	return pathPart, suffix, true
}

func pytestSelectorFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func splitPytestSuiteSelectors(suite string) []string {
	selector := strings.TrimSpace(suite)
	if selector == "" {
		return nil
	}
	fields := strings.Fields(selector)
	if len(fields) <= 1 {
		return []string{selector}
	}
	for _, field := range fields {
		if !looksLikePytestSelectorToken(field) {
			return []string{selector}
		}
	}
	return fields
}

func looksLikePytestSelectorToken(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	return strings.Contains(s, "::") || strings.Contains(filepath.ToSlash(s), ".py")
}

func buildRunCommand(runner, suite, repoRoot, mainRoot string) (string, string) {
	framework := ""
	if runner == "python" {
		framework = detectPythonTestFramework(repoRoot)
	}
	return buildRunCommandWithFramework(runner, framework, suite, repoRoot, mainRoot)
}

func buildRunCommandWithFramework(runner, framework, suite, repoRoot, mainRoot string) (string, string) {
	switch runner {
	case "go":
		pkg := strings.TrimSpace(suite)
		if pkg == "" {
			pkg = "./..."
		}
		return fmt.Sprintf("go test -json %s", pkg), ""
	case "node":
		// Prefer project's npm script when it exists (respects
		// monorepo / workspace config). Add --json to whatever
		// the script runs; jest and vitest both accept it.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "npm test -- --json --silent", ""
		}
		return fmt.Sprintf("npm test -- --json --silent -t %q", filter), ""
	case "python":
		if framework == pythonFrameworkDjango {
			interp := pythonRuntimeInterpreter(repoRoot, mainRoot)
			script := "runtests.py"
			if _, err := os.Stat(filepath.Join(repoRoot, script)); err != nil {
				script = filepath.Join("tests", "runtests.py")
			}
			filter := djangoSuiteSelector(suite)
			if filter == "" {
				return fmt.Sprintf("%s %s -v 1", interp, shellQuoteWord(script)), ""
			}
			return fmt.Sprintf("%s %s %s -v 1", interp, shellQuoteWord(script), shellQuoteWord(filter)), ""
		}
		if framework == pythonFrameworkUnittest {
			interp := pythonRuntimeInterpreter(repoRoot, mainRoot)
			filter := strings.TrimSpace(suite)
			if filter == "" {
				return fmt.Sprintf("%s -m unittest discover -v", interp), ""
			}
			if strings.HasSuffix(filter, ".py") || strings.Contains(filter, "/") {
				return fmt.Sprintf("%s -m unittest discover -s %q -v", interp, filter), ""
			}
			return fmt.Sprintf("%s -m unittest %q -v", interp, filter), ""
		}
		// pytest-json-report writes to a file specified by
		// --json-report-file. We use a temp file in the repo's
		// worktree so the report doesn't escape.
		tmpFile := filepath.Join(repoRoot, ".codrax-pytest-report.json")
		// Resolve interpreter: prefer the project's venv when one
		// exists (so pytest runs against the project's installed
		// deps, not whatever is on PATH); fall back to system
		// python3 / python; last resort bare pytest. The python -m
		// pytest form is preferred over bare pytest because (1) it
		// adds CWD to sys.path so source-only repos can import
		// their own package without `pip install -e .`, and (2)
		// it works regardless of whether the pytest entry-point
		// script is on PATH (only the module needs to be importable).
		//
		// Probe order: worktree first (rare — venv tracked in git),
		// main repo root second (typical — .venv/ gitignored, lives
		// at mainRoot only). Caller passes both so verify-stage code
		// running inside the worktree still finds the venv at the
		// user's mainRoot.
		interp, asModule := pythonInterpreter(repoRoot, mainRoot)
		filterArgs := shellQuotePytestSelectorsForRoot(suite, repoRoot)
		if asModule {
			if filterArgs == "" {
				return fmt.Sprintf("%s -m pytest --json-report --json-report-file=%q", interp, tmpFile), tmpFile
			}
			return fmt.Sprintf("%s -m pytest %s --json-report --json-report-file=%q", interp, filterArgs, tmpFile), tmpFile
		}
		// Bare pytest fallback (no python interpreter resolvable).
		if filterArgs == "" {
			return fmt.Sprintf("%s --json-report --json-report-file=%q", interp, tmpFile), tmpFile
		}
		return fmt.Sprintf("%s %s --json-report --json-report-file=%q", interp, filterArgs, tmpFile), tmpFile
	case "rust":
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "cargo test", ""
		}
		return fmt.Sprintf("cargo test %q", filter), ""
	case "swift":
		// Swift Package Manager. `swift test` emits text output that
		// the cargo-style parser handles cleanly because both follow
		// the `test result: ok. N passed; N failed` footer convention.
		// `--parallel` would break parsing; intentionally omitted.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "swift test", ""
		}
		return fmt.Sprintf("swift test --filter %q", filter), ""
	case "java":
		// Maven + Gradle both write JUnit XML to a fixed
		// subdirectory; the parser's post-exec hook walks those
		// dirs regardless of which tool ran. The "extraFile"
		// channel isn't used here because the XML files are
		// multi-file in a directory, not one named file. The
		// command itself just needs to be `mvn test` / `gradle
		// test` so the reports get produced.
		filter := strings.TrimSpace(suite)
		build := detectJavaBuildSystem(repoRoot)
		switch build {
		case "maven":
			if filter == "" {
				// -Dsurefire.useFile=true is the Maven default;
				// we rely on target/surefire-reports/*.xml.
				return "mvn -B -q test", ""
			}
			return fmt.Sprintf("mvn -B -q -Dtest=%q test", filter), ""
		case "gradle":
			if filter == "" {
				return "./gradlew --no-daemon --console=plain test", ""
			}
			return fmt.Sprintf("./gradlew --no-daemon --console=plain test --tests %q", filter), ""
		}
		// Unknown Java layout —let command fail and the parser
		// surface a clear error. Shouldn't happen because
		// detectRunner matched one of pom.xml / build.gradle[.kts].
		return "mvn -B -q test", ""
	case "ruby":
		// RSpec with --format json prints a single top-level
		// JSON object to stdout. `bundle exec` ensures the
		// project's Gemfile-pinned rspec version is used.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "bundle exec rspec --format json", ""
		}
		return fmt.Sprintf("bundle exec rspec --format json %q", filter), ""
	case "cmake":
		// ctest emits JUnit XML via --output-junit (CMake >= 3.21).
		// The build dir must already be configured + built; Execute
		// falls back to a clear error when detectNativeBuildDir
		// returns "". Filter maps to -R <regex> which ctest treats as
		// a POSIX ERE applied to the test name.
		buildDir := detectNativeBuildDir(repoRoot)
		if buildDir == "" {
			// Command will not be built; Execute refuses before
			// reaching this path. Keep a harmless no-op here so a
			// caller bypassing Execute sees a clear failure.
			return "", ""
		}
		// Tmpfile must end in .xml so parseJUnitXMLDir's suffix gate
		// accepts it when filepath.Walk passes it through.
		tmpFile := filepath.Join(repoRoot, ".codrax-ctest-report.xml")
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return fmt.Sprintf("ctest --test-dir %q --output-junit %q --output-on-failure", buildDir, tmpFile), tmpFile
		}
		return fmt.Sprintf("ctest --test-dir %q --output-junit %q --output-on-failure -R %q",
			buildDir, tmpFile, filter), tmpFile
	case "meson":
		// meson test writes JUnit XML when --xunit-file is set. The
		// command runs from the build dir via -C. Filter maps to a
		// trailing positional arg (test name substring).
		buildDir := detectNativeBuildDir(repoRoot)
		if buildDir == "" {
			return "", ""
		}
		tmpFile := filepath.Join(repoRoot, ".codrax-meson-report.xml")
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return fmt.Sprintf("meson test -C %q --xunit-file %q --print-errorlogs", buildDir, tmpFile), tmpFile
		}
		return fmt.Sprintf("meson test -C %q --xunit-file %q --print-errorlogs %q",
			buildDir, tmpFile, filter), tmpFile
	case "make":
		// Raw Makefile projects have no structured test output.
		// Prefer `make check` (GNU convention) over `make test` by
		// peeking at the Makefile for the target name; if neither is
		// obvious, default to `check` since it's the autotools /
		// GNU-ism most mature Makefile test suites follow. Parser
		// uses exit code for pass/fail and stdout for failure detail.
		target := detectMakeTestTarget(repoRoot)
		filter := strings.TrimSpace(suite)
		if filter != "" {
			// User supplied a specific target name.
			target = filter
		}
		return fmt.Sprintf("make %s", target), ""
	case "hvigor":
		// HarmonyOS ArkTS projects use hvigorw (wrapper for hvigor,
		// analogous to ./gradlew). `test` task runs unit + component
		// tests and emits JUnit XML into test results dirs that the
		// Java parser's post-exec JUnit walker already picks up.
		//
		// Unlike Java's pom.xml/build.gradle, hvigor sometimes only
		// ships the wrapper at repo root; a missing hvigorw falls
		// through to `hvigor` directly (PATH). Both are accepted; a
		// test environment without either still reaches the tool via
		// exit-code-only failure in Execute.
		filter := strings.TrimSpace(suite)
		wrapper := "hvigorw"
		if _, err := os.Stat(filepath.Join(repoRoot, "hvigorw")); err != nil {
			wrapper = "hvigor"
		}
		if filter == "" {
			return fmt.Sprintf("%s --no-daemon test", wrapper), ""
		}
		return fmt.Sprintf("%s --no-daemon test --tests %q", wrapper, filter), ""
	case "cjpm":
		// Cangjie's package manager. cjpm 1.0.0 LTS does not document
		// a stable `--json` reporter flag, so the runner uses the
		// default text output; parseCjpmText parses the `test result:
		// ok. N passed; N failed` footer (reused from cargo parser).
		// A future cjpm release that adds `--json` can be probed by
		// parsing `cjpm test --help` at runtime; for now the text
		// path is the single source of truth.
		filter := strings.TrimSpace(suite)
		if filter == "" {
			return "cjpm test", ""
		}
		return fmt.Sprintf("cjpm test --filter %q", filter), ""
	}
	return "", ""
}

func djangoSuiteSelector(suite string) string {
	s := strings.TrimSpace(suite)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "./")
	s = strings.TrimPrefix(s, ".\\")
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.ReplaceAll(s, "::", ".")
	if strings.HasSuffix(s, ".py") {
		s = strings.TrimSuffix(s, ".py")
	}
	if idx := strings.Index(s, ".py."); idx >= 0 {
		s = s[:idx] + s[idx+3:]
	}
	s = strings.TrimPrefix(s, "tests/")
	s = strings.TrimPrefix(s, "tests.")
	s = strings.ReplaceAll(s, "/", ".")
	for strings.Contains(s, "..") {
		s = strings.ReplaceAll(s, "..", ".")
	}
	return strings.Trim(s, ".")
}

// detectMakeTestTarget reads the Makefile head and returns the test
// target name the project uses. Preference order: "check" (GNU /
// autotools convention) > "test" (CMake-generated / npm-style) >
// "tests" (occasional plural). Defaults to "check" when neither
// target is present —`make check` is the widely-accepted standard
// and a missing target surfaces a clean "No rule to make target
// 'check'" error the parser can relay.
//
// Note: this is a best-effort grep for `^<target>:`. A Makefile that
// builds its test target dynamically (via included fragments or
// $(eval)) may hide the target from this scan; those projects can
// override via the Suite parameter.
func detectMakeTestTarget(repoRoot string) string {
	target, _ := detectMakeTestTargetFound(repoRoot)
	return target
}

// detectMakeTestTargetFound is detectMakeTestTarget plus the typed
// found-signal: found=true only when the Makefile declares one of the
// conventional test targets as a top-level rule. The TestSurface uses the
// signal to rank a Makefile with a real check/test target above manifests
// whose trees carry no test work; the default-"check" fallback keeps the
// executor behaviour unchanged.
func detectMakeTestTargetFound(repoRoot string) (string, bool) {
	candidates := []string{"Makefile", "makefile", "GNUmakefile"}
	var makefilePath string
	for _, name := range candidates {
		p := filepath.Join(repoRoot, name)
		if _, err := os.Stat(p); err == nil {
			makefilePath = p
			break
		}
	}
	if makefilePath == "" {
		return "check", false
	}
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return "check", false
	}
	// Scan for the first target named "check", "test", or "tests"
	// that appears as a top-level rule line (starts at column 0).
	preference := []string{"check", "test", "tests"}
	lines := strings.Split(string(data), "\n")
	found := map[string]bool{}
	for _, ln := range lines {
		if len(ln) == 0 || ln[0] == ' ' || ln[0] == '\t' || ln[0] == '#' {
			continue
		}
		for _, target := range preference {
			// "<target>:" at line start —Makefile target definition.
			if strings.HasPrefix(ln, target+":") || strings.HasPrefix(ln, target+" :") {
				found[target] = true
			}
		}
	}
	for _, target := range preference {
		if found[target] {
			return target, true
		}
	}
	return "check", false
}

// renderTestSummary builds the one-paragraph string for the
// tool-result Summary. Shows aggregate counts + failed test names
// (if any, cap 10 so the Summary stays under a reasonable length).
// Consumed by the verifier agent's LLM turn as context; full
// per-test detail lives in Mutable.ChangeReport.TestResults.
func renderTestSummary(runner string, report *types.ChangeReport) string {
	total, failed := countTestAssertions(report.TestResults)
	passed := total - failed
	verdict := "PASSED"
	if !report.Passed {
		verdict = "FAILED"
	}
	if report.NormalizeVerificationStatus() == types.VerificationStatusUnavailable {
		verdict = "UNAVAILABLE"
		total = 0
		failed = 0
		passed = 0
	}
	var b strings.Builder
	if verdict == "UNAVAILABLE" {
		fmt.Fprintf(&b, "[run_tests: runner=%s verdict=%s] local verification unavailable; %d diagnostic row(s), %d real assertion(s)",
			runner, verdict, len(report.TestResults), total)
	} else {
		fmt.Fprintf(&b, "[run_tests: runner=%s verdict=%s] %d total, %d passed, %d failed",
			runner, verdict, total, passed, failed)
	}
	if len(report.NoTestsRunners) > 0 {
		fmt.Fprintf(&b,
			"\n\nNote: runner(s) %s completed cleanly but discovered zero test cases. "+
				"This is NOT a verify failure — the project either has no test fixture for "+
				"the touched language, or no tests match the selector. Verification fell "+
				"back to whatever non-test signals are available (e.g. compile / syntax check).",
			strings.Join(report.NoTestsRunners, ", "))
	}
	if verdict == "UNAVAILABLE" {
		if report.FailureSummary != "" {
			fmt.Fprintf(&b, "\n\nDiagnostic: %s", report.FailureSummary)
		}
		return b.String()
	}
	if failed > 0 {
		b.WriteString("\n\nFailed tests:")
		shown := 0
		for _, r := range report.TestResults {
			kind := r.Kind
			if kind == "" {
				kind = types.TestResultKindUnit
			}
			if r.Passed || kind != types.TestResultKindUnit {
				continue
			}
			shown++
			if shown > 10 {
				fmt.Fprintf(&b, "\n  ... (+%d more)", failed-10)
				break
			}
			fmt.Fprintf(&b, "\n  - %s (%s)", r.AssertionID, r.Suite)
			if r.FailureDetail != "" {
				// Clip failure detail to first line + 鈮?60 chars.
				line := strings.SplitN(r.FailureDetail, "\n", 2)[0]
				if len(line) > 160 {
					line = line[:160] + "..."
				}
				fmt.Fprintf(&b, "\n    %s", line)
			}
		}
	}
	return b.String()
}

// countFailed returns how many TestResult entries have Passed=false.
func countFailed(results []types.TestResult) int {
	n := 0
	for _, r := range results {
		if !r.Passed {
			n++
		}
	}
	return n
}

func countTestAssertions(results []types.TestResult) (total, failed int) {
	for _, r := range results {
		kind := r.Kind
		if kind == "" {
			kind = types.TestResultKindUnit
		}
		if kind != types.TestResultKindUnit {
			continue
		}
		total++
		if !r.Passed {
			failed++
		}
	}
	return total, failed
}

func countAvailableReportAssertions(reports []*types.ChangeReport) (total, failed int) {
	for _, report := range reports {
		if report == nil || report.NormalizeVerificationStatus() == types.VerificationStatusUnavailable {
			continue
		}
		t, f := countTestAssertions(report.TestResults)
		total += t
		failed += f
	}
	return total, failed
}

// synthesizeBuildFailureReport installs a single-TestResult
// ChangeReport describing a build/configure failure that short-
// circuited test execution. Shared by java / hvigor / cmake / meson
// branches when their expected XML artifact never materialised.
//
// The TestResult carries Kind=TestResultKindBuildError and structured
// BuildErrors[] parsed from stdout via parseBuildErrors (handles
// Maven javac, Kotlin, generic javac, TypeScript, Cangjie). When
// no patterns match, BuildErrors is nil but FailureDetail still
// holds a human-readable excerpt via narrativeBuildErrorExcerpt.
// ChangeReport.BuildFailed is set so evalTestsPass surfaces a clean
// "build failed before tests ran" message instead of "1 of 1 tests
// failed: <synthetic-id>".
//
// label is the human-readable toolchain name ("Java", "hvigor",
// "CMake", "Meson") for the user-facing failure summary. The
// returned ToolResult is what run_tests.Execute returns directly;
// caller does not call parseRunnerOutput when we short out here.
func synthesizeBuildFailureReport(
	ctx *types.BusContext,
	toolName, runner, label, output string,
) (types.ToolResult, error) {
	excerpt := narrativeBuildErrorExcerpt(output)
	buildErrs := parseBuildErrors(output)
	_, ref := StoreBlob(ctx, toolName+"-no-reports", output)
	failSummary := renderBuildFailureSummary(label, buildErrs,
		strings.SplitN(excerpt, "\n", 2)[0])
	report := &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   firstBuildErrorAssertionID(buildErrs),
			Suite:         "build",
			Passed:        false,
			FailureDetail: excerpt,
			BuildErrors:   buildErrs,
		}},
		Passed:         false,
		BuildFailed:    true,
		FailureSummary: failSummary,
		GeneratedAt:    time.Now(),
	}
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		report.PlanID = plan.ID
	}
	ctx.Mutable.SetChangeReport(report)
	return types.ToolResult{
		ToolName:  toolName,
		Success:   false,
		Summary:   fmt.Sprintf("[run_tests: runner=%s] %s", runner, failSummary),
		RawRef:    ref,
		Timestamp: time.Now(),
	}, nil
}
