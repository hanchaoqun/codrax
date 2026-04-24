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
// wrote its JSON to (pytest workflow); empty for stdout parsers.
// cmdStr is included in ChangeReport failure narratives so the
// operator can see exactly what command ran.
func parseRunnerOutput(runner, stdout, extraFile, cmdStr string) (*types.ChangeReport, error) {
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
	case "ruby":
		return parseRSpecJSON(stdout)
	}
	return nil, fmt.Errorf("parseRunnerOutput: unknown runner %q", runner)
}

// ── Go: go test -json ────────────────────────────────────────────
//
// Output format: one JSON object per line. Relevant Action values:
//   "run"    — test is starting (ignore)
//   "pass"   — test passed (record with Passed=true)
//   "fail"   — test failed (record with Passed=false; subsequent
//              "output" actions carry the failure message which we
//              collect into FailureDetail)
//   "skip"   — test was skipped (not counted as failure)
//   "output" — arbitrary stdout line; we accumulate per-test output
//              so failing tests get their t.Errorf messages preserved
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
	for _, status := range pkgStatus {
		if status == "fail" {
			allPassed = false
			break
		}
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      allPassed && len(results) > 0,
	}
	if !report.Passed {
		report.FailureSummary = buildGoFailureSummary(results, pkgStatus)
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
	NumPassedTests int `json:"numPassedTests"`
	NumFailedTests int `json:"numFailedTests"`
	NumTotalTests  int `json:"numTotalTests"`
	Success        bool `json:"success"`
	TestResults    []struct {
		Name              string `json:"name"` // test file path
		AssertionResults  []struct {
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
		NodeID   string  `json:"nodeid"`      // e.g. "tests/test_foo.py::test_bar"
		Outcome  string  `json:"outcome"`     // passed | failed | skipped | error
		Duration float64 `json:"duration"`    // seconds
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
//   running 3 tests
//   test tests::foo ... ok
//   test tests::bar ... FAILED
//   test tests::baz ... ignored
//
//   failures:
//
//   ---- tests::bar stdout ----
//   ...traceback...
//
//   failures:
//       tests::bar
//
//   test result: FAILED. 2 passed; 1 failed; 0 ignored; ...
//
// Parser extracts each `test name ... status` line + the aggregate
// `test result:` line. Failure details come from the "---- name
// stdout ----" block.
var (
	reCargoTestLine = regexp.MustCompile(`^test ([\w:<>]+) \.\.\. (ok|FAILED|ignored)\b`)
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

	if len(results) == 0 && aggMatch == nil {
		// Neither per-test nor aggregate lines — cargo probably
		// failed to build. Return a single synthetic failure.
		results = append(results, types.TestResult{
			AssertionID:   "cargo-build",
			Suite:         "cargo",
			Passed:        false,
			FailureDetail: truncateDetail(stdout, 4000),
		})
		allPassed = false
	}

	report := &types.ChangeReport{
		TestResults: results,
		Passed:      allPassed && len(results) > 0,
	}
	if !report.Passed {
		if aggFailed > 0 {
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
	Name      string         `xml:"name,attr"`
	ClassName string         `xml:"classname,attr"`
	Time      string         `xml:"time,attr"`
	Failure   *junitFailure  `xml:"failure"`
	Error     *junitFailure  `xml:"error"`
	Skipped   *junitSkipped  `xml:"skipped"`
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
