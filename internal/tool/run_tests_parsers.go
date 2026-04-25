package tool

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// parseRunnerOutput dispatches to the runner-specific parser and
// returns a populated ChangeReport. extraFile is a path the runner
// wrote its JSON/XML to (pytest / ctest / meson); empty for stdout
// parsers. cmdStr is included in ChangeReport failure narratives so
// the operator can see exactly what command ran. runErr is the
// *exec.Cmd.Run error — populated for every runner but only the
// make parser (no structured output) inspects it.
func parseRunnerOutput(runner, stdout, extraFile, cmdStr string, runErr error) (*types.ChangeReport, error) {
	switch runner {
	case "go":
		return parseGoTestJSONLines(stdout)
	case "node":
		return parseJestJSON(stdout)
	case "python":
		return parsePytestJSONReport(extraFile, stdout, cmdStr)
	case "rust":
		return parseCargoTestText(stdout)
	case "java":
		// extraFile is the report directory (populated by
		// locateJUnitReportDir in run_tests.go's Execute).
		return parseJUnitXMLDir(extraFile, stdout)
	case "cmake", "meson":
		// extraFile is a single JUnit XML file written by
		// ctest --output-junit or meson test --xunit-file.
		// parseJUnitXMLDir's filepath.Walk handles a single-file
		// root cleanly — the .xml suffix gate accepts our tmpfile.
		return parseJUnitXMLDir(extraFile, stdout)
	case "ruby":
		return parseRSpecJSON(stdout)
	case "make":
		return parseMakeOutput(stdout, runErr)
	case "hvigor":
		// HarmonyOS hvigor emits JUnit XML via the same reporting
		// mechanism as Gradle; locateJUnitReportDir populates
		// extraFile with the report root. Reuse the JUnit parser
		// path without modification — the XML schema is identical.
		return parseJUnitXMLDir(extraFile, stdout)
	case "cjpm":
		// Cangjie's cjpm emits Cargo-shaped text output (`test
		// result: ok. N passed; N failed`) from its test command.
		// The cargo parser handles both the exact Cargo line and
		// the cjpm variant because the result footer grammar is
		// identical — no dedicated parser required.
		return parseCargoTestText(stdout)
	case "swift":
		// Swift Package Manager XCTest output is not cargo-shaped.
		// We fall back to the make-style exit-code parser for
		// pass/fail; the build-error regex set picks up Swift
		// compile errors via the .swift file extension on a generic
		// regex that covers them via the same path as Java/Kotlin.
		return parseSwiftOutput(stdout, runErr)
	}
	return nil, fmt.Errorf("parseRunnerOutput: unknown runner %q", runner)
}

// parseMakeOutput synthesises a ChangeReport from `make <target>`
// execution. Raw Makefiles have no structured test protocol so we
// collapse the whole run into a single `make-test` TestResult whose
// Passed flag is the process exit status. On failure, FailureDetail
// carries an extract of the stdout/stderr error markers (same
// generic scanner the Java/CMake build-failure paths use).
//
// This sacrifices per-test visibility — a `make check` that runs
// 200 cases will produce one composite row — but it's faithful to
// what Make tells us. Projects that want finer granularity should
// configure their Makefile test target to emit JUnit XML and use
// the cmake/meson runner shape; otherwise the aggregate verdict is
// the only honest signal.
//
// runErr is the *exec.Cmd.Run result: nil → target succeeded, any
// error → target failed (including exit codes, signal termination,
// and "No rule to make target" from an absent `check`/`test`).
func parseMakeOutput(stdout string, runErr error) (*types.ChangeReport, error) {
	passed := runErr == nil
	var (
		detail      string
		failSummary string
	)
	if !passed {
		detail = narrativeBuildErrorExcerpt(stdout)
		if detail == "" {
			// Nothing marker-like in the output — at least carry the
			// error string itself so the retry hint isn't empty.
			detail = runErr.Error()
		}
		firstLine := strings.SplitN(detail, "\n", 2)[0]
		if len(firstLine) > 200 {
			firstLine = firstLine[:200] + "…"
		}
		failSummary = "make target failed: " + firstLine
	}
	report := &types.ChangeReport{
		TestResults: []types.TestResult{{
			Kind:          types.TestResultKindUnit,
			AssertionID:   "make-test",
			Suite:         "make",
			Passed:        passed,
			FailureDetail: detail,
		}},
		Passed:         passed,
		FailureSummary: failSummary,
	}
	return report, nil
}

// parseSwiftOutput handles Swift Package Manager XCTest output. The
// SPM verdict shape is:
//
//	Test Suite 'All tests' passed at 2024-01-01 00:00:00.
//	     Executed N tests, with M failures (X unexpected) in T (W) seconds
//
// We extract per-test rows from `Test Case '-[Suite testName]' passed/
// failed` lines (cap to a useful subset) and fall back to exit-code
// pass/fail when the regex finds nothing. Build errors before tests
// ran are picked up by parseBuildErrors via the .swift extension on
// the generic file:line:error: shape.
func parseSwiftOutput(stdout string, runErr error) (*types.ChangeReport, error) {
	results := []types.TestResult{}
	for _, m := range reSwiftTestCase.FindAllStringSubmatch(stdout, -1) {
		// m: [full, suite, name, status]
		passed := m[3] == "passed"
		results = append(results, types.TestResult{
			Kind:        types.TestResultKindUnit,
			AssertionID: m[2],
			Suite:       m[1],
			Passed:      passed,
		})
	}
	allPassed := runErr == nil
	for _, r := range results {
		if !r.Passed {
			allPassed = false
		}
	}
	// Compile-error path: zero per-test rows + non-nil runErr.
	buildFailed := false
	if len(results) == 0 && runErr != nil {
		buildErrs := parseBuildErrors(stdout)
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   firstBuildErrorAssertionID(buildErrs),
			Suite:         "build",
			Passed:        false,
			FailureDetail: truncateDetail(stdout, 4000),
			BuildErrors:   buildErrs,
		})
		allPassed = false
		buildFailed = true
	}
	report := &types.ChangeReport{
		TestResults: results,
		Passed:      allPassed && len(results) > 0 && !buildFailed,
		BuildFailed: buildFailed,
	}
	if !report.Passed {
		if buildFailed {
			report.FailureSummary = renderBuildFailureSummary("Swift",
				results[0].BuildErrors,
				strings.SplitN(narrativeBuildErrorExcerpt(stdout), "\n", 2)[0])
		} else {
			report.FailureSummary = "Swift tests failed (XCTest); see per-test detail."
		}
	}
	return report, nil
}

var reSwiftTestCase = regexp.MustCompile(
	`(?m)^Test Case '-\[(\S+) (\S+)\]' (passed|failed)`)

// ── Go: go test -json ────────────────────────────────────────────
//
// Output format: one JSON object per line. Relevant Action values:
//
//	"run"    — test is starting (ignore)
//	"pass"   — test passed (record with Passed=true)
//	"fail"   — test failed (record with Passed=false; subsequent
//	           "output" actions carry the failure message which we
//	           collect into FailureDetail)
//	"skip"   — test was skipped (not counted as failure)
//	"output" — arbitrary stdout line; we accumulate per-test output
//	           so failing tests get their t.Errorf messages preserved
//
// Reference: https://pkg.go.dev/cmd/test2json
type goTestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test,omitempty"`
	Elapsed float64   `json:"Elapsed,omitempty"`
	Output  string    `json:"Output,omitempty"`
}

func parseGoTestJSONLines(stdout string) (*types.ChangeReport, error) {
	// Accumulate per-test (pkg+name) state as events stream in.
	// Go emits nested sub-tests as separate Test names (e.g.
	// "TestFoo" and "TestFoo/SubBar"); we treat each as its own
	// assertion for the report.
	type accum struct {
		suite     string
		name      string
		passed    bool
		elapsed   time.Duration
		output    strings.Builder
		completed bool
	}
	tests := make(map[string]*accum)
	keyOf := func(pkg, test string) string { return pkg + "::" + test }

	// ANSI-like status events also use Action="pass"/"fail" at the
	// package level (Test field empty). We track these separately
	// so a package-level fail without test names still surfaces.
	pkgStatus := make(map[string]string)

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	// Lift default buffer so a long line (rare but possible with
	// massive t.Errorf) doesn't split-fail.
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	lineCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		if !strings.HasPrefix(line, "{") {
			continue // build errors / uncaptured stderr; skip
		}
		var ev goTestEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// A malformed line shouldn't abort the whole parse;
			// most go-test-json output is well-formed and a
			// single bad line is usually a corruption artifact.
			continue
		}
		if ev.Test == "" {
			// Package-level event.
			switch ev.Action {
			case "pass", "fail", "skip":
				pkgStatus[ev.Package] = ev.Action
			}
			continue
		}
		key := keyOf(ev.Package, ev.Test)
		a, ok := tests[key]
		if !ok {
			a = &accum{suite: ev.Package, name: ev.Test}
			tests[key] = a
		}
		switch ev.Action {
		case "output":
			a.output.WriteString(ev.Output)
		case "pass":
			a.passed = true
			a.completed = true
			a.elapsed = time.Duration(ev.Elapsed * float64(time.Second))
		case "fail":
			a.passed = false
			a.completed = true
			a.elapsed = time.Duration(ev.Elapsed * float64(time.Second))
		case "skip":
			// Skipped tests are counted as passed for verify
			// (a skip is not a failure). Mark completed so
			// they appear in the report.
			a.passed = true
			a.completed = true
			a.elapsed = time.Duration(ev.Elapsed * float64(time.Second))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parseGoTestJSONLines: scanner: %w", err)
	}

	// Build results. Sort by (suite, name) so the output is stable
	// for testing and human inspection. Actually skip sorting —
	// Go test output is already deterministic enough, and keeping
	// insertion order preserves the runner's test execution order.
	results := make([]types.TestResult, 0, len(tests))
	for _, a := range tests {
		if !a.completed {
			// Test started but never finished — build error or
			// timeout. Record as failed with the accumulated output
			// so the cause surfaces.
			a.passed = false
		}
		detail := ""
		if !a.passed {
			detail = strings.TrimSpace(a.output.String())
			// Cap failure detail so the report stays readable.
			if len(detail) > 4000 {
				detail = detail[:4000] + "\n…[failure detail truncated]"
			}
		}
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindUnit,
			AssertionID:   a.name,
			Suite:         a.suite,
			Passed:        a.passed,
			Duration:      a.elapsed,
			FailureDetail: detail,
		})
	}

	allPassed := true
	for _, r := range results {
		if !r.Passed {
			allPassed = false
			break
		}
	}
	// Package-level failures without any test events are still
	// failures (e.g. compile errors before any test runs).
	pkgFail := false
	for _, status := range pkgStatus {
		if status == "fail" {
			allPassed = false
			pkgFail = true
			break
		}
	}

	// Compile-error path: zero per-test events + at least one
	// package-level fail = the package never compiled. Synthesise a
	// build-error row so evalTestsPass sees BuildFailed and surfaces
	// "build failed before tests ran" instead of "0 tests passed".
	buildFailed := false
	if len(results) == 0 && pkgFail {
		buildErrs := parseBuildErrors(stdout)
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   firstBuildErrorAssertionID(buildErrs),
			Suite:         "build",
			Passed:        false,
			FailureDetail: truncateDetail(stdout, 4000),
			BuildErrors:   buildErrs,
		})
		buildFailed = true
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      allPassed && len(results) > 0 && !buildFailed,
		BuildFailed: buildFailed,
	}
	if !report.Passed {
		if buildFailed {
			report.FailureSummary = renderBuildFailureSummary("Go",
				results[0].BuildErrors,
				strings.SplitN(narrativeBuildErrorExcerpt(stdout), "\n", 2)[0])
		} else {
			report.FailureSummary = buildGoFailureSummary(results, pkgStatus)
		}
	}
	return report, nil
}

func buildGoFailureSummary(results []types.TestResult, pkgStatus map[string]string) string {
	var b strings.Builder
	failed := 0
	for _, r := range results {
		if !r.Passed {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(&b, "%d test(s) failed across %d total.", failed, len(results))
	}
	pkgFails := 0
	for _, s := range pkgStatus {
		if s == "fail" {
			pkgFails++
		}
	}
	if pkgFails > 0 {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		fmt.Fprintf(&b, "%d package-level failure(s) — typically compile or init errors.", pkgFails)
	}
	if b.Len() == 0 {
		b.WriteString("No tests ran — empty or non-test-containing package.")
	}
	return b.String()
}

// ── Node: jest --json / vitest --reporter=json ───────────────────
//
// Both jest and vitest produce a single top-level JSON object with
// compatible shape — testResults[] per test file, each with
// assertionResults[] per test case. vitest's JSON shape is jest-
// compatible by design. One parser suffices.
type jestJSON struct {
	NumPassedTests int  `json:"numPassedTests"`
	NumFailedTests int  `json:"numFailedTests"`
	NumTotalTests  int  `json:"numTotalTests"`
	Success        bool `json:"success"`
	TestResults    []struct {
		Name             string `json:"name"` // test file path
		AssertionResults []struct {
			Title           string   `json:"title"`           // test case name
			Status          string   `json:"status"`          // passed | failed | skipped | pending
			Duration        float64  `json:"duration"`        // milliseconds
			FailureMessages []string `json:"failureMessages"` // stack traces
			AncestorTitles  []string `json:"ancestorTitles"`  // describe blocks
		} `json:"assertionResults"`
	} `json:"testResults"`
}

func parseJestJSON(stdout string) (*types.ChangeReport, error) {
	// jest mixes JSON with prose (e.g. npm wrapper warnings on
	// stderr). Find the first '{' and the last '}' to extract
	// the actual JSON blob.
	start := strings.Index(stdout, "{")
	end := strings.LastIndex(stdout, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("parseJestJSON: no JSON object found in output (len=%d)", len(stdout))
	}
	payload := stdout[start : end+1]

	var j jestJSON
	if err := json.Unmarshal([]byte(payload), &j); err != nil {
		return nil, fmt.Errorf("parseJestJSON: unmarshal: %w", err)
	}

	results := make([]types.TestResult, 0, j.NumTotalTests)
	for _, tr := range j.TestResults {
		for _, ar := range tr.AssertionResults {
			// Compose assertion ID as "describe > it" so users
			// see the full path (jest/vitest nesting is common).
			id := ar.Title
			if len(ar.AncestorTitles) > 0 {
				id = strings.Join(ar.AncestorTitles, " > ") + " > " + id
			}
			passed := ar.Status == "passed" || ar.Status == "skipped" || ar.Status == "pending"
			detail := ""
			if !passed && len(ar.FailureMessages) > 0 {
				detail = strings.Join(ar.FailureMessages, "\n")
				if len(detail) > 4000 {
					detail = detail[:4000] + "\n…[failure detail truncated]"
				}
			}
			results = append(results, types.TestResult{
				Kind:          types.TestResultKindUnit,
				AssertionID:   id,
				Suite:         tr.Name,
				Passed:        passed,
				Duration:      time.Duration(ar.Duration) * time.Millisecond,
				FailureDetail: detail,
			})
		}
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      j.Success && j.NumFailedTests == 0,
	}
	if !report.Passed {
		report.FailureSummary = fmt.Sprintf(
			"%d of %d Node tests failed (jest/vitest reporter).",
			j.NumFailedTests, j.NumTotalTests)
	}
	return report, nil
}

// ── Python: pytest-json-report plugin ─────────────────────────────
//
// Plugin output at the configured report file is a top-level JSON
// object with tests[] per assertion. Without the plugin installed
// the file never exists and we emit a clear installation hint.
type pytestJSON struct {
	Exitcode int `json:"exitcode"` // 0 = all passed
	Summary  struct {
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Skipped int `json:"skipped"`
		Error   int `json:"error"`
		Total   int `json:"total"`
	} `json:"summary"`
	Tests []struct {
		NodeID   string  `json:"nodeid"`   // e.g. "tests/test_foo.py::test_bar"
		Outcome  string  `json:"outcome"`  // passed | failed | skipped | error
		Duration float64 `json:"duration"` // seconds
		Call     struct {
			Longrepr string `json:"longrepr"` // failure traceback
		} `json:"call"`
	} `json:"tests"`
}

func parsePytestJSONReport(reportFile, stdout, cmdStr string) (*types.ChangeReport, error) {
	if reportFile == "" {
		return nil, fmt.Errorf("parsePytestJSONReport: no report file path supplied")
	}
	data, err := os.ReadFile(reportFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Plugin missing is the #1 reason for this; surface a
			// clean installation hint instead of a generic file-
			// not-found error.
			return nil, fmt.Errorf(
				"parsePytestJSONReport: report file %s missing — the pytest-json-report "+
					"plugin is required. Install with `pip install pytest-json-report` "+
					"(command ran: %s). First %d bytes of pytest stdout:\n%s",
				reportFile, cmdStr, minInt(len(stdout), 400), stdoutHead(stdout, 400))
		}
		return nil, fmt.Errorf("parsePytestJSONReport: read %s: %w", reportFile, err)
	}
	var p pytestJSON
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsePytestJSONReport: unmarshal %s: %w", reportFile, err)
	}

	results := make([]types.TestResult, 0, len(p.Tests))
	for _, tt := range p.Tests {
		// Split nodeid "file::test" into suite + assertion.
		suite, name := tt.NodeID, tt.NodeID
		if idx := strings.LastIndex(tt.NodeID, "::"); idx > 0 {
			suite = tt.NodeID[:idx]
			name = tt.NodeID[idx+2:]
		}
		passed := tt.Outcome == "passed" || tt.Outcome == "skipped"
		detail := ""
		if !passed {
			detail = tt.Call.Longrepr
			if len(detail) > 4000 {
				detail = detail[:4000] + "\n…[failure detail truncated]"
			}
		}
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindUnit,
			AssertionID:   name,
			Suite:         suite,
			Passed:        passed,
			Duration:      time.Duration(tt.Duration * float64(time.Second)),
			FailureDetail: detail,
		})
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      p.Exitcode == 0 && p.Summary.Failed == 0 && p.Summary.Error == 0,
	}
	if !report.Passed {
		report.FailureSummary = fmt.Sprintf(
			"pytest exitcode=%d — %d passed, %d failed, %d error, %d skipped of %d total.",
			p.Exitcode, p.Summary.Passed, p.Summary.Failed, p.Summary.Error,
			p.Summary.Skipped, p.Summary.Total)
	}
	return report, nil
}

// ── Rust: cargo test (text output) ───────────────────────────────
//
// Cargo stable has no stable JSON reporter (nightly gates
// --format=json behind -Z unstable-options). We parse the text
// output with regex. Cargo emits:
//
//	running 3 tests
//	test tests::foo ... ok
//	test tests::bar ... FAILED
//	test tests::baz ... ignored
//
//	failures:
//
//	---- tests::bar stdout ----
//	...traceback...
//
//	failures:
//	    tests::bar
//
//	test result: FAILED. 2 passed; 1 failed; 0 ignored; ...
//
// Parser extracts each `test name ... status` line + the aggregate
// `test result:` line. Failure details come from the "---- name
// stdout ----" block.
var (
	reCargoTestLine  = regexp.MustCompile(`^test ([\w:<>]+) \.\.\. (ok|FAILED|ignored)\b`)
	reCargoFailBlock = regexp.MustCompile(`(?s)---- ([\w:<>]+) stdout ----\n(.*?)(?:\n\n|\n-{4})`)
	reCargoAggregate = regexp.MustCompile(`test result: (ok|FAILED)\. (\d+) passed; (\d+) failed`)
)

func parseCargoTestText(stdout string) (*types.ChangeReport, error) {
	type cargoResult struct {
		name   string
		passed bool
	}
	var tests []cargoResult

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if m := reCargoTestLine.FindStringSubmatch(line); m != nil {
			passed := m[2] == "ok" || m[2] == "ignored"
			tests = append(tests, cargoResult{name: m[1], passed: passed})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parseCargoTestText: scanner: %w", err)
	}

	// Pull failure details from the "---- name stdout ----" blocks.
	failureDetails := make(map[string]string)
	for _, m := range reCargoFailBlock.FindAllStringSubmatch(stdout, -1) {
		name := m[1]
		detail := strings.TrimSpace(m[2])
		if len(detail) > 4000 {
			detail = detail[:4000] + "\n…[failure detail truncated]"
		}
		failureDetails[name] = detail
	}

	results := make([]types.TestResult, 0, len(tests))
	allPassed := true
	for _, t := range tests {
		if !t.passed {
			allPassed = false
		}
		detail := ""
		if !t.passed {
			detail = failureDetails[t.name]
		}
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindUnit,
			AssertionID:   t.name,
			Suite:         "cargo",
			Passed:        t.passed,
			FailureDetail: detail,
		})
	}

	// Cross-check against aggregate line. If the runner didn't
	// emit any per-test lines (build-error path, no tests ran),
	// aggregate may still report a verdict.
	aggMatch := reCargoAggregate.FindStringSubmatch(stdout)
	aggFailed := 0
	if aggMatch != nil {
		if aggMatch[1] == "FAILED" {
			allPassed = false
		}
		if n, err := strconv.Atoi(aggMatch[3]); err == nil {
			aggFailed = n
		}
	}

	buildFailed := false
	if len(results) == 0 && aggMatch == nil {
		// Neither per-test nor aggregate lines — cargo / cjpm probably
		// failed to build. Synthesise a structured build-error row.
		buildErrs := parseBuildErrors(stdout)
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindBuildError,
			AssertionID:   firstBuildErrorAssertionID(buildErrs),
			Suite:         "build",
			Passed:        false,
			FailureDetail: truncateDetail(stdout, 4000),
			BuildErrors:   buildErrs,
		})
		allPassed = false
		buildFailed = true
	}

	// Tag every successful unit row with Kind=Unit so consumers
	// reading `tr.Kind == TestResultKindUnit` (verifier prompt,
	// criterion eval) match without falling through the empty-string
	// default. Build-error rows already carry their kind above.
	for i := range results {
		if results[i].Kind == "" {
			results[i].Kind = types.TestResultKindUnit
		}
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      allPassed && len(results) > 0,
		BuildFailed: buildFailed,
	}
	if !report.Passed {
		if buildFailed {
			report.FailureSummary = renderBuildFailureSummary("cargo/cjpm",
				results[0].BuildErrors,
				strings.SplitN(narrativeBuildErrorExcerpt(stdout), "\n", 2)[0])
		} else if aggFailed > 0 {
			report.FailureSummary = fmt.Sprintf("cargo test: %d failure(s); see per-test detail.", aggFailed)
		} else {
			report.FailureSummary = "cargo test reported failure (build error or test panic); see raw stdout."
		}
	}
	return report, nil
}

// ── helpers ──────────────────────────────────────────────────────

func truncateDetail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…[truncated]"
}

func stdoutHead(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// narrativeBuildErrorExcerpt scans build-tool stdout/stderr for lines
// that plausibly describe a compile or configuration failure and
// returns a bounded excerpt suitable for embedding in a TestResult's
// FailureDetail.
//
// Two-tier marker priority (the gap closed in 2026-04-25 audit): in
// large multi-module Maven / Gradle runs the [ERROR] prefix can match
// hundreds of lines BEFORE the verdict-shape "BUILD FAILURE" / "What
// went wrong" footer, and the 10-line cap would drop the verdict.
// We solve this by running TWO passes:
//
//   1. Verdict pass — pick up to 3 lines matching the small set of
//      verdict-shape markers (BUILD FAILURE, BUILD FAILED, FAILURE:,
//      What went wrong). These ALWAYS land first in the output.
//   2. Detail pass — fill remaining 7 slots with the per-error
//      markers ([ERROR], error:, Error:, e: , TS).
//
// Pairs with parseBuildErrors which extracts structured file:line:msg
// rows. The narrative excerpt is the human-readable companion that
// retains marker-less context (e.g. "Could not resolve dependency
// foo:bar:1.0").
//
// Output bounded to ~1500 chars and at most 10 matched lines total.
// When no markers match, falls back to the first 5 non-empty lines.
func narrativeBuildErrorExcerpt(stdout string) string {
	const (
		maxLines       = 10
		maxVerdictRows = 3
		maxChars       = 1500
	)
	if strings.TrimSpace(stdout) == "" {
		return ""
	}
	verdictMarkers := []string{
		"BUILD FAILURE",
		"BUILD FAILED",
		"FAILURE:",
		"What went wrong",
	}
	detailMarkers := []string{
		"[ERROR]",
		"error:",
		"Error:",
		" e: ",
	}
	matchAny := func(line string, markers []string) bool {
		for _, m := range markers {
			if strings.Contains(line, m) {
				return true
			}
		}
		return false
	}
	seen := make(map[string]struct{})
	picked := make([]string, 0, maxLines)
	lines := strings.Split(stdout, "\n")
	// Pass 1: verdict markers (priority — always retained).
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r\t ")
		if line == "" || !matchAny(line, verdictMarkers) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		picked = append(picked, trimmed)
		if len(picked) >= maxVerdictRows {
			break
		}
	}
	// Pass 2: detail markers fill remaining slots.
	for _, raw := range lines {
		if len(picked) >= maxLines {
			break
		}
		line := strings.TrimRight(raw, "\r\t ")
		if line == "" || !matchAny(line, detailMarkers) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		picked = append(picked, trimmed)
	}
	if len(picked) == 0 {
		// Marker-less fallback: first ~5 non-empty lines of head.
		head := stdoutHead(stdout, maxChars)
		for _, raw := range strings.Split(head, "\n") {
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			picked = append(picked, line)
			if len(picked) >= 5 {
				break
			}
		}
	}
	out := strings.Join(picked, "\n")
	if len(out) > maxChars {
		out = out[:maxChars] + "\n…[truncated]"
	}
	return out
}

// Build-error regexes — covers the toolchains run_tests supports.
// Order matters: more-specific patterns must match first so a
// fragment like "Bar.java:42" doesn't get swallowed by the generic
// fallback.
//
//   1. Maven javac:   "[ERROR] /path/Foo.java:[42,5] cannot find symbol"
//   2. Kotlin:        "e: file:///path/Foo.kt:42:5 Unresolved reference"
//   3. Generic:       "/path/Foo.{java,scala,kt,kts,ets,ts,tsx,go,
//                      rs,c,h,cc,cpp,cxx,hpp,hh,py}:42: error: …"
//                     Covers javac/scalac/Gradle, Go (`./foo.go:5:1:
//                     syntax error:`), GCC/Clang (`foo.c:42:5: error:`),
//                     pylint/mypy.
//   4. TS (paren):    "/path/foo.ts(42,5): error TS2304: …"
//   5. TS (colon):    "/path/foo.ts:42:5 - error TS2304: …"
//   6. Cangjie:       "error: /path/Bar.cj:42:5: …"
//   7. Rust (-->):    "  --> src/lib.rs:10:5"  (block-style; carries
//                     the location only — rustc puts the error code on
//                     a preceding `error[E0308]: …` line which
//                     extractRustErrorCodeAndMessage links back.)
//
// Each capture group: file, line, optional col, optional symbol/code,
// message. parseBuildErrors normalises into BuildError rows and
// dedups by (file, line, col).
var (
	reBuildMavenJavac = regexp.MustCompile(
		`\[ERROR\]\s+(\S+\.(?:java|scala|kt|kts)):\[(\d+),(\d+)\]\s+(.+)`)
	reBuildKotlin = regexp.MustCompile(
		`(?m)^\s*e:\s+(?:file://)?(\S+\.(?:kt|kts)):(\d+):(\d+):?\s+(.+)`)
	reBuildGenericFLine = regexp.MustCompile(
		`(?m)^\s*(?:\[ERROR\]\s+)?(\S+\.(?:java|scala|kt|kts|ets|ts|tsx|go|rs|c|h|cc|cpp|cxx|hpp|hh|py|swift|rb|lua|m|mm|cu|cuh|proto)):(\d+):(?:\s*\d+:)?\s*(?:error|ERROR):\s+(.+)`)
	reBuildTSParen = regexp.MustCompile(
		`(?m)^\s*(?:ERROR:\s+)?(\S+\.(?:ts|tsx|ets)):?\((\d+),(\d+)\):\s+(?:error\s+)?(?:(TS\d+):\s+)?(.+)`)
	reBuildTSColon = regexp.MustCompile(
		`(?m)^\s*(\S+\.(?:ts|tsx|ets)):(\d+):(\d+)\s+-\s+error\s+(?:(TS\d+):\s+)?(.+)`)
	reBuildCangjie = regexp.MustCompile(
		`(?m)^\s*(?:\[ERROR\]\s+|error:\s+)(\S+\.cj):(\d+):(\d+)?:?\s+(.+)`)
	reBuildRustArrow = regexp.MustCompile(
		`(?m)^\s*-->\s+(\S+\.rs):(\d+):(\d+)`)
	reBuildRustErrCode = regexp.MustCompile(
		`(?m)^error\[(E\d+)\]:\s+(.+)`)
	// Go errors don't always carry a literal "error:" keyword:
	// `./foo.go:5:1: syntax error: unexpected newline` is common.
	// Accept any message after the file:line(:col): prefix on .go
	// files. Tighter than the generic regex (Go-only file ext)
	// keeps false positives bounded.
	reBuildGoLine = regexp.MustCompile(
		`(?m)^\s*(\S+\.go):(\d+):(?:(\d+):)?\s+(.+)`)
)

// parseBuildErrors extracts structured BuildError rows from build-
// tool stdout. Returns nil when no patterns match (caller can still
// rely on narrativeBuildErrorExcerpt for human-readable text).
//
// Bounded to maxBuildErrors entries — projects with hundreds of
// compile errors after a typo only need the first handful surfaced
// for the LLM to fix. Dedup is by (file, line, column) so the same
// row matched by two regexes (e.g. Maven [ERROR] + the embedded
// generic file:line:error: shape) appears once.
//
// File paths are NOT canonicalised here — they're whatever the
// build tool emitted. Verifier/coder render them as-is so the LLM
// sees the same identifier the operator would see in a terminal.
func parseBuildErrors(stdout string) []types.BuildError {
	const maxBuildErrors = 25
	if strings.TrimSpace(stdout) == "" {
		return nil
	}
	type key struct {
		file string
		line int
		col  int
	}
	seen := make(map[key]struct{})
	out := make([]types.BuildError, 0, 8)

	add := func(file string, line, col int, symbol, message string) {
		file = strings.TrimSpace(file)
		message = strings.TrimSpace(message)
		if file == "" || message == "" {
			return
		}
		k := key{file: file, line: line, col: col}
		if _, dup := seen[k]; dup {
			return
		}
		seen[k] = struct{}{}
		out = append(out, types.BuildError{
			File: file, Line: line, Column: col,
			Symbol: strings.TrimSpace(symbol), Message: message,
		})
	}

	// 1. Maven javac
	for _, m := range reBuildMavenJavac.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		add(m[1], line, col, "", m[4])
	}
	// 2. Kotlin
	for _, m := range reBuildKotlin.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		add(m[1], line, col, "", m[4])
	}
	// 3. Generic javac/scalac/Gradle compile
	for _, m := range reBuildGenericFLine.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		line, _ := strconv.Atoi(m[2])
		add(m[1], line, 0, "", m[3])
	}
	// 4. TypeScript paren-shape
	for _, m := range reBuildTSParen.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		add(m[1], line, col, m[4], m[5])
	}
	// 5. TypeScript colon-shape
	for _, m := range reBuildTSColon.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		line, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		add(m[1], line, col, m[4], m[5])
	}
	// 6. Cangjie
	for _, m := range reBuildCangjie.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		line, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		add(m[1], line, col, "", m[4])
	}
	// 7. Rust block-style. The `-->` line carries file:line:col but no
	// message; we pair each occurrence with the most-recent
	// `error[Exxxx]: <message>` line above it. Cargo / rustc puts the
	// error code one or two lines before the location pointer.
	rustLocs := reBuildRustArrow.FindAllStringSubmatchIndex(stdout, -1)
	rustCodes := reBuildRustErrCode.FindAllStringSubmatchIndex(stdout, -1)
	// 8. Go-specific: matches `<file>.go:<line>(:<col>)?: <anything>`.
	// Run BEFORE rust block to avoid mis-attribution of `.rs:N:M` lines.
	for _, m := range reBuildGoLine.FindAllStringSubmatch(stdout, -1) {
		if len(out) >= maxBuildErrors {
			break
		}
		// Skip if the message is empty / whitespace.
		msg := strings.TrimSpace(m[4])
		if msg == "" {
			continue
		}
		line, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		add(m[1], line, col, "", msg)
	}
	if len(rustLocs) > 0 {
		for _, lm := range rustLocs {
			if len(out) >= maxBuildErrors {
				break
			}
			locStart := lm[0]
			file := stdout[lm[2]:lm[3]]
			line, _ := strconv.Atoi(stdout[lm[4]:lm[5]])
			col, _ := strconv.Atoi(stdout[lm[6]:lm[7]])
			// Find the closest `error[Exxxx]: ...` BEFORE this loc.
			var symbol, message string
			for _, cm := range rustCodes {
				if cm[0] >= locStart {
					break
				}
				symbol = stdout[cm[2]:cm[3]]
				message = stdout[cm[4]:cm[5]]
			}
			if message == "" {
				message = "rust compile error"
			}
			add(file, line, col, symbol, message)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// renderBuildFailureSummary builds the FailureSummary string for a
// build-failure ChangeReport. Pulls from structured BuildErrors[]
// when available (concise file:line + first error), falling back to
// the narrative excerpt's first line. Bounded to ~200 chars after
// the prefix so the verifier prompt + REPL render cleanly.
func renderBuildFailureSummary(label string, errs []types.BuildError, narrativeFirstLine string) string {
	if len(errs) == 0 {
		if narrativeFirstLine == "" {
			return label + " build failed before tests ran (no recognized error markers; see FailureDetail)."
		}
		clipped := narrativeFirstLine
		if len(clipped) > 200 {
			clipped = clipped[:200] + "…"
		}
		return fmt.Sprintf("%s build failed before tests ran: %s", label, clipped)
	}
	files := map[string]struct{}{}
	for _, e := range errs {
		files[e.File] = struct{}{}
	}
	first := errs[0]
	loc := first.File
	if first.Line > 0 {
		loc = fmt.Sprintf("%s:%d", first.File, first.Line)
	}
	msg := first.Message
	if len(msg) > 160 {
		msg = msg[:160] + "…"
	}
	return fmt.Sprintf("%s build failed: %d compile error(s) in %d file(s); first: %s — %s",
		label, len(errs), len(files), loc, msg)
}

// firstBuildErrorAssertionID returns "<file>:<line>" of the first
// BuildError, or "" when errs is empty. Used as a stable
// AssertionID for build-error TestResults so /history rendering
// shows operators where to look without inventing a string convention.
func firstBuildErrorAssertionID(errs []types.BuildError) string {
	if len(errs) == 0 {
		return ""
	}
	e := errs[0]
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d", e.File, e.Line)
	}
	return e.File
}

// ── Java: JUnit XML (Maven surefire / Gradle testreport) ──────────
//
// Maven writes per-test-class XML under target/surefire-reports/
// named TEST-<fully-qualified-class>.xml. Gradle's equivalent lives
// under build/test-results/test/. Both share the de-facto Ant
// JUnit schema:
//
//	<testsuite name="com.foo.BarTest" tests="3" failures="1" errors="0" skipped="0" time="0.42">
//	  <testcase name="testHappy" classname="com.foo.BarTest" time="0.12"/>
//	  <testcase name="testFail" classname="com.foo.BarTest" time="0.20">
//	    <failure message="expected X got Y" type="AssertionError">stack trace…</failure>
//	  </testcase>
//	  <testcase name="testSkipped" classname="com.foo.BarTest" time="0.0">
//	    <skipped/>
//	  </testcase>
//	</testsuite>
//
// We recurse the report dir (multi-module repos have nested output)
// and aggregate every <testcase> into a single ChangeReport.
type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure"`
	Error     *junitFailure `xml:"error"`
	Skipped   *junitSkipped `xml:"skipped"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitSkipped struct {
	Message string `xml:"message,attr"`
}

// parseJUnitXMLDir walks reportDir, parses every .xml file, and
// returns a combined ChangeReport. Files that don't parse as
// either <testsuite> or <testsuites> are skipped with a log line —
// IDE-plugin-generated XML sometimes slips non-conforming files
// into the output dir, and we don't want one bad file to fail the
// whole verify stage.
//
// reportDir is the directory returned by locateJUnitReportDir in
// run_tests.go (typically target/surefire-reports or
// build/test-results/test). stdout is included in the failure
// summary when the XML walk found zero testcases so the operator
// can see what mvn/gradle printed.
func parseJUnitXMLDir(reportDir, stdout string) (*types.ChangeReport, error) {
	if reportDir == "" {
		return nil, fmt.Errorf("parseJUnitXMLDir: empty report directory")
	}
	var results []types.TestResult
	walkErr := filepath.Walk(reportDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable; we'd rather partial than zero
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".xml") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		// Try <testsuites> wrapper first (Gradle + newer Maven), then
		// single <testsuite> (older Maven + plain Ant). Either shape
		// is legal; we try both rather than peeking at the root tag.
		var suitesWrap junitTestSuites
		if err := xml.Unmarshal(data, &suitesWrap); err == nil && len(suitesWrap.Suites) > 0 {
			for _, s := range suitesWrap.Suites {
				results = append(results, junitCasesToResults(s)...)
			}
			return nil
		}
		var single junitTestSuite
		if err := xml.Unmarshal(data, &single); err == nil && single.Name != "" {
			results = append(results, junitCasesToResults(single)...)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("parseJUnitXMLDir: walk %s: %w", reportDir, walkErr)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("parseJUnitXMLDir: no test cases parsed from %s (stdout head: %s)",
			reportDir, stdoutHead(stdout, 200))
	}

	failed := 0
	for _, r := range results {
		if !r.Passed {
			failed++
		}
	}
	report := &types.ChangeReport{
		TestResults: results,
		Passed:      failed == 0,
	}
	if !report.Passed {
		report.FailureSummary = fmt.Sprintf(
			"%d of %d Java test cases failed (JUnit XML from %s).",
			failed, len(results), filepath.Base(reportDir))
	}
	return report, nil
}

// junitCasesToResults converts one testsuite's cases into
// codrax-shape TestResult entries. AssertionID format is
// "<className>#<testName>" so duplicates across suites collide
// correctly (the reverse — "<testName> @ <className>" — would be
// ambiguous when two classes share a method name).
//
// Skipped cases are Passed=true per the codrax convention (they
// don't block the verify gate). Failure AND Error elements both
// map to Passed=false; their content is concatenated into
// FailureDetail with type + message + body if present.
func junitCasesToResults(s junitTestSuite) []types.TestResult {
	out := make([]types.TestResult, 0, len(s.TestCases))
	for _, tc := range s.TestCases {
		id := tc.Name
		if tc.ClassName != "" {
			id = tc.ClassName + "#" + tc.Name
		}
		passed := tc.Failure == nil && tc.Error == nil
		detail := ""
		switch {
		case tc.Failure != nil:
			detail = junitFailureDetail("failure", tc.Failure)
		case tc.Error != nil:
			detail = junitFailureDetail("error", tc.Error)
		case tc.Skipped != nil:
			// Skipped is not a failure; leave detail empty.
		}
		// Duration — attribute is a decimal string of seconds.
		dur := time.Duration(0)
		if secs, err := strconv.ParseFloat(tc.Time, 64); err == nil && secs > 0 {
			dur = time.Duration(secs * float64(time.Second))
		}
		out = append(out, types.TestResult{
			Kind:          types.TestResultKindUnit,
			AssertionID:   id,
			Suite:         s.Name,
			Passed:        passed,
			Duration:      dur,
			FailureDetail: detail,
		})
	}
	return out
}

// junitFailureDetail composes "<kind> [type]: message\n<body>" with
// the same 4000-char truncation the other parsers use so the Turn B
// prompt stays bounded.
func junitFailureDetail(kind string, f *junitFailure) string {
	if f == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(kind)
	if f.Type != "" {
		b.WriteString(" [" + f.Type + "]")
	}
	if f.Message != "" {
		b.WriteString(": " + f.Message)
	}
	if body := strings.TrimSpace(f.Body); body != "" {
		b.WriteString("\n")
		b.WriteString(body)
	}
	out := b.String()
	if len(out) > 4000 {
		out = out[:4000] + "\n…[failure detail truncated]"
	}
	return out
}

// ── Ruby: rspec --format json ──────────────────────────────────────
//
// RSpec emits a single top-level JSON object on stdout when invoked
// with `--format json`. Relevant fields:
//
//	summary.example_count    — total test cases
//	summary.failure_count    — failed count
//	summary.pending_count    — skipped / pending count
//	examples[].full_description — "Foo#bar does baz" (AssertionID)
//	examples[].file_path     — spec/foo_spec.rb (Suite)
//	examples[].status        — "passed" | "failed" | "pending"
//	examples[].run_time      — seconds (float)
//	examples[].exception     — {class, message, backtrace} on failure
//
// RSpec mixes progress dots with JSON when the project has a
// custom `.rspec` file; we find the outer JSON object the same way
// parseJestJSON does.
type rspecJSON struct {
	Summary struct {
		ExampleCount int `json:"example_count"`
		FailureCount int `json:"failure_count"`
		PendingCount int `json:"pending_count"`
	} `json:"summary"`
	Examples []struct {
		FullDescription string  `json:"full_description"`
		FilePath        string  `json:"file_path"`
		Status          string  `json:"status"`
		RunTime         float64 `json:"run_time"`
		Exception       *struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		} `json:"exception,omitempty"`
	} `json:"examples"`
}

func parseRSpecJSON(stdout string) (*types.ChangeReport, error) {
	start := strings.Index(stdout, "{")
	end := strings.LastIndex(stdout, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("parseRSpecJSON: no JSON object found in output (len=%d)", len(stdout))
	}
	payload := stdout[start : end+1]

	var r rspecJSON
	if err := json.Unmarshal([]byte(payload), &r); err != nil {
		return nil, fmt.Errorf("parseRSpecJSON: unmarshal: %w", err)
	}

	results := make([]types.TestResult, 0, r.Summary.ExampleCount)
	for _, ex := range r.Examples {
		// RSpec considers "pending" neither pass nor fail — we
		// treat it as Passed=true so it doesn't trigger
		// regression / tests_pass failure gates. Only the
		// "failed" status flips Passed to false.
		passed := ex.Status != "failed"
		detail := ""
		if !passed && ex.Exception != nil {
			detail = ex.Exception.Class + ": " + ex.Exception.Message
			if len(detail) > 4000 {
				detail = detail[:4000] + "\n…[failure detail truncated]"
			}
		}
		results = append(results, types.TestResult{
			Kind:          types.TestResultKindUnit,
			AssertionID:   ex.FullDescription,
			Suite:         ex.FilePath,
			Passed:        passed,
			Duration:      time.Duration(ex.RunTime * float64(time.Second)),
			FailureDetail: detail,
		})
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      r.Summary.FailureCount == 0,
	}
	if !report.Passed {
		report.FailureSummary = fmt.Sprintf(
			"%d of %d Ruby examples failed (rspec --format json).",
			r.Summary.FailureCount, r.Summary.ExampleCount)
	}
	return report, nil
}
