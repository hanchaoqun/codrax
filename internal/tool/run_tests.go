package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
//   - Go:     `go test -json ./...` 鈥?native JSONL output
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
// filesystem-based 鈥?no LLM judgment, no git blame.
//
// L3 red line: Execute MUST NOT invoke ground.BuildContext or
// ground.GroundItem. Test outputs are structured pass/fail data,
// not citations. Enforced by write_mode_red_lines_test.go.
//
// Classified ReadOnly + NonEvidenceTool. Tests read the repo and
// produce verdicts but do not mutate the worktree 鈥?the file I/O
// tests do happen through run_tests, but only under test-framework
// control (e.g. tmpdir fixtures) which the runner owns, not codrax.
type RunTests struct {
	ReadOnly
	NonEvidenceTool
}

type runnerPlan struct {
	Runner   string
	Root     string
	Manifest string
	Priority int
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

	// WorkingDir is the LLM-decided test root, repo-relative. Empty
	// or "." means RepoRoot. A nested module path (e.g. "backend/")
	// scopes the run to a sub-project. Validated to be inside
	// RepoRoot (no escapes via ".." or absolute paths) before any
	// command runs.
	WorkingDir string `json:"working_dir,omitempty"`
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
		"Supports Go (go test -json), Node (jest/vitest --json), Python (pytest --json-report plugin required), Rust (cargo test text), Java (Maven/Gradle JUnit XML), Kotlin (via the Java Gradle path 鈥?build.gradle.kts recognised), Ruby (RSpec --format json), CMake (ctest --output-junit; requires pre-configured build dir), Meson (meson test --xunit-file), raw Makefile (make check/test; pass/fail from exit code), HarmonyOS ArkTS via hvigor (hvigorw test 鈫?JUnit XML), and HarmonyOS Cangjie via cjpm (cjpm test 鈫?cargo-style text)."
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
    "working_dir": {
      "type": "string",
      "description": "Test root, repo-relative path. Empty or \".\" = repo root. Use to scope the run to a sub-project / module dir. Must be inside the worktree."
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
		return errResult(t.Name(), "run_tests requires BusContext.Mutable"), nil
	}
	if ctx.RepoRoot == "" {
		return errResult(t.Name(),
			"run_tests requires ctx.RepoRoot — orchestrator must have swapped to a worktree before verify"), nil
	}

	var p runTestsParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return errResult(t.Name(), fmt.Sprintf("invalid params: %v", err)), err
		}
	}

	timeout := 300 * time.Second
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}

	// Two paths to a runner plan list:
	//   (1) LLM supplied `runner` (preferred) — short-circuit
	//       manifest detection. The verifier agent has already
	//       inspected the repo (list_files / read_file / repo_map)
	//       and committed to a runner; the system validates the
	//       choice against the whitelist + working_dir against the
	//       worktree boundary, then dispatches.
	//   (2) Empty `runner` — fall back to legacy manifest keyword
	//       detect. Brittle for bare-directory repos; kept as a
	//       backstop so old eval cases / direct CLI calls keep
	//       working.
	var plans []runnerPlan
	if strings.TrimSpace(p.Runner) != "" {
		choice, rej := resolveLLMRunnerChoice(ctx.RepoRoot, p.Runner, p.WorkingDir)
		if rej != "" {
			return errResult(t.Name(), rej), nil
		}
		plans = []runnerPlan{choice}
		logging.Info("[run_tests] LLM-selected runner=%s working_dir=%s (manifest auto-detect bypassed)",
			choice.Runner, runnerPlanRel(ctx.RepoRoot, choice))
	} else {
		plans = detectRunnerPlans(ctx.RepoRoot)
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
	for _, plan := range plans {
		runnerRoot := plan.Root
		runner := plan.Runner

		if runner == "cmake" || runner == "meson" {
			if detectNativeBuildDir(runnerRoot) == "" {
				msg := fmt.Sprintf("%s project detected at %s but no configured build directory found "+
					"(looked for build/, Build/, builddir/, out/, cmake-build-debug/, cmake-build-release/). "+
					"Configure the project first (e.g. `cmake -S . -B build` or `meson setup builddir`) and build it, "+
					"then re-run verify.", runner, runnerRoot)
				projectReports = append(projectReports, qualifyChangeReport(makeExecutionFailureReport("build", msg, true), plan, ctx.RepoRoot))
				combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, msg))
				continue
			}
		}

		// MainRepoRoot is threaded through for python venv probing:
		// during verify the worktree is a detached-HEAD checkout that
		// excludes gitignored paths (`.venv/` typically is), so the
		// venv lives at the user's main repo root, not under the
		// worktree. pythonInterpreter probes both roots in order.
		cmdStr, extraFile := buildRunCommand(runner, p.Suite, runnerRoot, ctx.MainRepoRoot)

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

		// Resource-exhaustion exits get classified explicitly so the
		// verify→plan retry hint surfaces "OOM" / "CPU limit" / "wall
		// timeout" — not the generic "tests failed" the planner would
		// otherwise see and re-derive a wrong corrective direction
		// from. The clearForReplan path consumes ChangeReport.FailureKind
		// in the heuristic hint.
		switch supRes.ExitKind {
		case SupervisedExitTimeout:
			_, ref := StoreBlob(ctx, t.Name()+"-timeout", strings.Join(combinedOutputs, "\n\n"))
			report := makeResourceExhaustionReport("timeout", fmt.Sprintf(
				"command timed out after %v (set timeout_seconds to bump)", timeout))
			ctx.Mutable.SetChangeReport(qualifyChangeReport(report, plan, ctx.RepoRoot))
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] command timed out after %v (set timeout_seconds to bump)", runnerPlanLabel(ctx.RepoRoot, plan), timeout),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		case SupervisedExitOOM:
			_, ref := StoreBlob(ctx, t.Name()+"-oom", strings.Join(combinedOutputs, "\n\n"))
			report := makeResourceExhaustionReport("oom", fmt.Sprintf(
				"command killed by memory cap (limit=%d MiB) — the test code allocated more than the configured ceiling. "+
					"Either the test fixture has an unbounded allocation (most common cause), the test data is genuinely too large "+
					"(raise verify_mem_limit_mb in codrax.yaml), or the test harness leaks memory across cases.",
				caps.MemoryLimitBytes/(1024*1024)))
			ctx.Mutable.SetChangeReport(qualifyChangeReport(report, plan, ctx.RepoRoot))
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] killed by memory cap (limit=%d MiB) — see ChangeReport.FailureSummary for retry guidance", runnerPlanLabel(ctx.RepoRoot, plan), caps.MemoryLimitBytes/(1024*1024)),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		case SupervisedExitCPULimit:
			_, ref := StoreBlob(ctx, t.Name()+"-cpu", strings.Join(combinedOutputs, "\n\n"))
			report := makeResourceExhaustionReport("cpu_limit", fmt.Sprintf(
				"command killed by CPU-time cap (limit=%ds) — the test code burned more CPU than the configured ceiling. "+
					"Most common cause is an infinite loop without a sleep or yield; less commonly a quadratic-or-worse algorithm "+
					"running on a large fixture.", caps.CPULimitSeconds))
			ctx.Mutable.SetChangeReport(qualifyChangeReport(report, plan, ctx.RepoRoot))
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
		if missing, missingBinary, missingReason := detectRunnerMissing(runner, runErr, output); missing {
			// Surface which detection signal fired so operators (and
			// support engineers triaging "but pytest IS installed!"
			// reports) can tell apart the three failure modes —
			// shell exit 127, Go's exec.ErrNotFound, or output-pattern
			// match — and inspect the matching pattern when it's #3.
			logging.Info("[run_tests] %s runner_missing detected: binary=%q reason=%s exit=%d",
				runnerPlanLabel(ctx.RepoRoot, plan), missingBinary, missingReason, execExit)
			_, ref := StoreBlob(ctx, t.Name()+"-runner-missing", strings.Join(combinedOutputs, "\n\n"))
			report := makeRunnerMissingReport(runner, missingBinary, output, ctx.Language, missingReason, execExit)
			ctx.Mutable.SetChangeReport(qualifyChangeReport(report, plan, ctx.RepoRoot))
			summary := runnerMissingToolResultSummary(
				ctx.Language, runnerPlanLabel(ctx.RepoRoot, plan),
				missingBinary, runnerInstallHint(runner, ctx.Language),
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

		report, err := parseRunnerOutput(runner, output, extraFile, cmdStr, runErr)
		if err != nil {
			logging.Warning("[run_tests] parser error for %s: %v", runnerPlanLabel(ctx.RepoRoot, plan), err)
			_, ref := StoreBlob(ctx, t.Name()+"-unparsed", strings.Join(combinedOutputs, "\n\n"))
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] output parser failed: %v — raw output stored for inspection", runnerPlanLabel(ctx.RepoRoot, plan), err),
				RawRef:    ref,
				Timestamp: time.Now(),
			}, nil
		}
		projectReports = append(projectReports, qualifyChangeReport(report, plan, ctx.RepoRoot))
	}

	report := mergeChangeReports(projectReports)
	if plan := ctx.Mutable.ChangePlan(); plan != nil {
		report.PlanID = plan.ID
	}
	report.GeneratedAt = time.Now()
	ctx.Mutable.SetChangeReport(report)

	summary := renderAggregateTestSummary(ctx.RepoRoot, plans, projectReports, report)
	_, ref := StoreBlob(ctx, t.Name(), strings.Join(combinedOutputs, "\n\n"))

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

// detectRunner returns the first runnable project discovered under the
// repo. Retained as a thin convenience wrapper for unit tests; Execute
// uses detectRunnerPlans so multi-project repos are verified end-to-end.
func detectRunner(repoRoot string) string {
	plans := detectRunnerPlans(repoRoot)
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
func resolveLLMRunnerChoice(repoRoot, runner, workingDir string) (runnerPlan, string) {
	runner = strings.ToLower(strings.TrimSpace(runner))
	if _, ok := allowedRunners[runner]; !ok {
		return runnerPlan{}, fmt.Sprintf(
			"run_tests rejected: runner=%q is not one of the supported runners (%s)",
			runner, strings.Join(allowedRunnerList(), ", "))
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

	return runnerPlan{
		Runner:   runner,
		Root:     target,
		Manifest: "(LLM-selected)",
		Priority: 0, // LLM choice always wins over auto-detect priorities
	}, ""
}

func detectRunnerPlans(repoRoot string) []runnerPlan {
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		rootAbs = repoRoot
	}
	manifests := supportedRunnerManifests()
	manifestIndex := make(map[string]runnerManifest, len(manifests))
	for _, m := range manifests {
		manifestIndex[strings.ToLower(m.File)] = m
	}

	plansByRoot := make(map[string]runnerPlan)
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
		if prev, ok := plansByRoot[root]; !ok || plan.Priority < prev.Priority || (plan.Priority == prev.Priority && plan.Manifest < prev.Manifest) {
			plansByRoot[root] = plan
		}
		return nil
	})

	plans := make([]runnerPlan, 0, len(plansByRoot))
	for _, plan := range plansByRoot {
		plans = append(plans, plan)
	}
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
		return relI < relJ
	})
	return plans
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
	return fmt.Sprintf("%s@%s", plan.Runner, runnerPlanRel(repoRoot, plan))
}

func renderRunnerOutputSection(plan runnerPlan, output string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "### %s (%s)\n", plan.Runner, filepath.ToSlash(plan.Root))
	b.WriteString(output)
	return b.String()
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
	return out
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
	fmt.Fprintf(&b, "[run_tests: projects=%d] %d/%d project(s) passed; %d assertion(s), %d failed.",
		len(plans), passedProjects, len(plans), len(aggregate.TestResults), countFailed(aggregate.TestResults))
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
//   1. Exit code 127 — POSIX convention for "command not found" set
//      by sh / bash when invoking an unknown binary. Both Linux and
//      macOS shells produce this.
//   2. Go's os/exec returns exec.ErrNotFound when LookPath fails on
//      a direct (non-shell-wrapped) invocation. We unwrap runErr's
//      chain to spot it.
//   3. Output (stderr captured via combined buffer) contains the
//      shell-emitted "command not found" / "not found" / "executable
//      file not found" patterns. Matched defensively against the
//      runner's primary binary name so a test's literal "X not found"
//      output doesn't false-positive.
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

// extractExitCode best-effort pulls the process exit code out of an
// exec error chain. *exec.ExitError is the typical wrapper; other
// shapes return -1.
func extractExitCode(err error) int {
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
//  2. `python3` from PATH — preferred over `python` because modern
//     Linux/macOS distros ship only `python3` (the `python` symlink
//     to Python 2 was removed). exec.LookPath probes the operator's
//     PATH, which exec.Cmd inherits to the worktree subprocess.
//  3. `python` from PATH — older systems / Windows.
//  4. Bare `pytest` — last resort, only when no python interpreter
//     resolves. The runner_missing detector catches this if pytest
//     is also absent.
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
	probedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		probedRoots = append(probedRoots, root)
		for _, dir := range venvDirs {
			for _, sub := range venvSubpaths {
				candidate := filepath.Join(root, dir, sub)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					logging.Info("[run_tests/python] interpreter resolved: venv=%q (root=%q dir=%q sub=%q)",
						filepath.ToSlash(candidate), root, dir, sub)
					return filepath.ToSlash(candidate), true
				}
			}
		}
	}
	if path, err := exec.LookPath("python3"); err == nil {
		logging.Info("[run_tests/python] interpreter resolved: PATH python3=%q (no venv at: %v)",
			path, probedRoots)
		return "python3", true
	}
	if path, err := exec.LookPath("python"); err == nil {
		logging.Info("[run_tests/python] interpreter resolved: PATH python=%q (no venv at: %v; python3 absent)",
			path, probedRoots)
		return "python", true
	}
	logging.Warning("[run_tests/python] no python interpreter found (probed venv at: %v; python3 / python absent from PATH); falling back to bare pytest",
		probedRoots)
	return "pytest", false
}

// runnerInstallHint maps a runner to a short install suggestion
// the user-facing error embeds. Kept terse; the user can search
// the runner's docs for full instructions. Bilingual: zh (default)
// vs en. Drift-guard test TestRunnerInstallHint_AllRunnersCovered
// asserts every allowedRunners entry has a non-empty hint in zh.
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
			return "在项目 venv 里装(`python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report`)或用系统 python(`pip install pytest pytest-json-report`);codrax 会自动识别仓根的 .venv / venv / env / .virtualenv 目录"
		}
		return "install pytest in the project venv (`python3 -m venv .venv && .venv/bin/pip install pytest pytest-json-report`) or for the system python (`pip install pytest pytest-json-report`); codrax auto-detects .venv / venv / env / .virtualenv directories at the repo root"
	case "rust":
		if zh {
			return "从 https://rustup.rs/ 安装 Rust + Cargo"
		}
		return "install Rust + Cargo from https://rustup.rs/"
	case "java":
		if zh {
			return "安装 Maven(`mvn`)或使用项目自带的 Gradle wrapper(`./gradlew`)"
		}
		return "install Maven (`mvn`) or use the project's Gradle wrapper (`./gradlew`)"
	case "ruby":
		if zh {
			return "用 `gem install bundler` 装 Bundler,然后在仓根跑 `bundle install`"
		}
		return "install Bundler with `gem install bundler` and run `bundle install` in the repo"
	case "swift":
		if zh {
			return "从 https://swift.org/download/ 安装 Swift 工具链"
		}
		return "install the Swift toolchain from https://swift.org/download/"
	case "cmake":
		if zh {
			return "从 https://cmake.org/download/ 安装 CMake(自带 ctest),并先配置好 build 目录"
		}
		return "install CMake (provides ctest) from https://cmake.org/download/ and configure a build dir first"
	case "meson":
		if zh {
			return "用 `pip install meson` 安装 Meson(并装 ninja),再配置 build 目录"
		}
		return "install Meson with `pip install meson` (and ninja) and configure a build dir first"
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
//   - "build" / "Build"        鈥?cmake -S . -B build convention
//   - "builddir"               鈥?meson default
//   - "out"                    鈥?Google/Android-style
//   - "cmake-build-debug" and
//     "cmake-build-release"    鈥?CLion IDE defaults
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
		// CMakeCache.txt; Meson drops meson-info/. We accept either 鈥?
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
// walk handles that 鈥?we return the highest non-empty match and let
// parseJUnitXMLDir recurse.
func locateJUnitReportDir(repoRoot string) string {
	candidates := []string{
		filepath.Join(repoRoot, "target", "surefire-reports"),
		filepath.Join(repoRoot, "build", "test-results", "test"),
		// HarmonyOS hvigor 鈥?Stage Model entry module.
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
// `*.xml` file at its top level (we don't recurse 鈥?a directory with
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
// Returns (command, extraFile) 鈥?extraFile is a temp file path
// the runner writes its JSON output to (pytest-json-report uses
// a file argument; other runners print to stdout). Empty extraFile
// means parse from stdout.
// buildRunCommand assembles the shell command string for a runner.
// repoRoot is the runner's working dir (worktree during verify);
// mainRoot is the original repo path (BusContext.MainRepoRoot)
// — only used by the python branch, where the user's venv often
// lives at mainRoot/.venv/ but never reaches the worktree because
// .venv/ is gitignored.
func buildRunCommand(runner, suite, repoRoot, mainRoot string) (string, string) {
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
		// pytest-json-report writes to a file specified by
		// --json-report-file. We use a temp file in the repo's
		// worktree so the report doesn't escape.
		tmpFile := filepath.Join(repoRoot, ".codrax-pytest-report.json")
		filter := strings.TrimSpace(suite)
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
		if asModule {
			if filter == "" {
				return fmt.Sprintf("%s -m pytest --json-report --json-report-file=%q", interp, tmpFile), tmpFile
			}
			return fmt.Sprintf("%s -m pytest %q --json-report --json-report-file=%q", interp, filter, tmpFile), tmpFile
		}
		// Bare pytest fallback (no python interpreter resolvable).
		if filter == "" {
			return fmt.Sprintf("%s --json-report --json-report-file=%q", interp, tmpFile), tmpFile
		}
		return fmt.Sprintf("%s %q --json-report --json-report-file=%q", interp, filter, tmpFile), tmpFile
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
		// Unknown Java layout 鈥?let command fail and the parser
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

// detectMakeTestTarget reads the Makefile head and returns the test
// target name the project uses. Preference order: "check" (GNU /
// autotools convention) > "test" (CMake-generated / npm-style) >
// "tests" (occasional plural). Defaults to "check" when neither
// target is present 鈥?`make check` is the widely-accepted standard
// and a missing target surfaces a clean "No rule to make target
// 'check'" error the parser can relay.
//
// Note: this is a best-effort grep for `^<target>:`. A Makefile that
// builds its test target dynamically (via included fragments or
// $(eval)) may hide the target from this scan; those projects can
// override via the Suite parameter.
func detectMakeTestTarget(repoRoot string) string {
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
		return "check"
	}
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		return "check"
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
			// "<target>:" at line start 鈥?Makefile target definition.
			if strings.HasPrefix(ln, target+":") || strings.HasPrefix(ln, target+" :") {
				found[target] = true
			}
		}
	}
	for _, target := range preference {
		if found[target] {
			return target
		}
	}
	return "check"
}

// renderTestSummary builds the one-paragraph string for the
// tool-result Summary. Shows aggregate counts + failed test names
// (if any, cap 10 so the Summary stays under a reasonable length).
// Consumed by the verifier agent's LLM turn as context; full
// per-test detail lives in Mutable.ChangeReport.TestResults.
func renderTestSummary(runner string, report *types.ChangeReport) string {
	total := len(report.TestResults)
	failed := countFailed(report.TestResults)
	passed := total - failed
	verdict := "PASSED"
	if !report.Passed {
		verdict = "FAILED"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[run_tests: runner=%s verdict=%s] %d total, %d passed, %d failed",
		runner, verdict, total, passed, failed)
	if len(report.NoTestsRunners) > 0 {
		fmt.Fprintf(&b,
			"\n\nNote: runner(s) %s completed cleanly but discovered zero test cases. "+
				"This is NOT a verify failure — the project either has no test fixture for "+
				"the touched language, or no tests match the selector. Verification fell "+
				"back to whatever non-test signals are available (e.g. compile / syntax check).",
			strings.Join(report.NoTestsRunners, ", "))
	}
	if failed > 0 {
		b.WriteString("\n\nFailed tests:")
		shown := 0
		for _, r := range report.TestResults {
			if r.Passed {
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
