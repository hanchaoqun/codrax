package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
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
      "enum": ["go", "node", "python", "rust", "java", "ruby", "cmake", "meson", "make", "hvigor", "cjpm"],
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

		cmdStr, extraFile := buildRunCommand(runner, p.Suite, runnerRoot)
		if extraFile != "" {
			defer os.Remove(extraFile)
		}

		execCtx, cancel := context.WithTimeout(context.Background(), timeout)
		cmd := NewShellCommandContext(execCtx, cmdStr)
		cmd.Dir = runnerRoot
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		runErr := cmd.Run()
		cancel()
		output := buf.String()
		combinedOutputs = append(combinedOutputs, renderRunnerOutputSection(plan, output))

		if execCtx.Err() == context.DeadlineExceeded {
			_, ref := StoreBlob(ctx, t.Name()+"-timeout", strings.Join(combinedOutputs, "\n\n"))
			return types.ToolResult{
				ToolName:  t.Name(),
				Success:   false,
				Summary:   fmt.Sprintf("[run_tests: %s] command timed out after %v (set timeout_seconds to bump)", runnerPlanLabel(ctx.RepoRoot, plan), timeout),
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
		if !report.Passed {
			out.Passed = false
		}
		if report.BuildFailed {
			out.BuildFailed = true
		}
		if report.FailureSummary != "" {
			failureSummaries = append(failureSummaries, report.FailureSummary)
		}
	}
	out.RegressionAssertions = dedupStrings(out.RegressionAssertions)
	out.PreexistingAssertions = dedupStrings(out.PreexistingAssertions)
	out.FixedAssertions = dedupStrings(out.FixedAssertions)
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
func buildRunCommand(runner, suite, repoRoot string) (string, string) {
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
		if filter == "" {
			return fmt.Sprintf("pytest --json-report --json-report-file=%q", tmpFile), tmpFile
		}
		return fmt.Sprintf("pytest %q --json-report --json-report-file=%q", filter, tmpFile), tmpFile
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
