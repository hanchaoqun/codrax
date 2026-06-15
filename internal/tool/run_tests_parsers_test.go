package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ── Go parser tests ──────────────────────────────────────────────

func TestParseGoTestJSONLines_AllPassed(t *testing.T) {
	// Real-ish go-test-json output: two packages, one test each.
	output := `{"Time":"2024-01-01T00:00:00Z","Action":"run","Package":"foo","Test":"TestA"}
{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"foo","Test":"TestA","Elapsed":0.5}
{"Time":"2024-01-01T00:00:01Z","Action":"pass","Package":"foo","Elapsed":0.5}
{"Time":"2024-01-01T00:00:02Z","Action":"run","Package":"bar","Test":"TestB"}
{"Time":"2024-01-01T00:00:03Z","Action":"pass","Package":"bar","Test":"TestB","Elapsed":0.3}
{"Time":"2024-01-01T00:00:03Z","Action":"pass","Package":"bar","Elapsed":0.3}
`
	report, err := parseGoTestJSONLines(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Error("report.Passed should be true when all tests pass")
	}
	if len(report.TestResults) != 2 {
		t.Errorf("TestResults len = %d, want 2", len(report.TestResults))
	}
	for _, r := range report.TestResults {
		if !r.Passed {
			t.Errorf("expected all passed, got failure: %+v", r)
		}
	}
}

func TestParseGoTestJSONLines_MixedPassFail(t *testing.T) {
	output := `{"Action":"run","Package":"pkg","Test":"TestPass"}
{"Action":"pass","Package":"pkg","Test":"TestPass","Elapsed":0.01}
{"Action":"run","Package":"pkg","Test":"TestFail"}
{"Action":"output","Package":"pkg","Test":"TestFail","Output":"    pkg_test.go:10: expected 1 got 2\n"}
{"Action":"fail","Package":"pkg","Test":"TestFail","Elapsed":0.01}
{"Action":"fail","Package":"pkg","Elapsed":0.02}
`
	report, err := parseGoTestJSONLines(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("report.Passed should be false when any test fails")
	}
	if len(report.TestResults) != 2 {
		t.Fatalf("TestResults len = %d, want 2", len(report.TestResults))
	}
	// Find the failing test and verify detail captured.
	var fail *types.TestResult
	for i := range report.TestResults {
		if !report.TestResults[i].Passed {
			fail = &report.TestResults[i]
			break
		}
	}
	if fail == nil {
		t.Fatal("expected a failing test in results")
	}
	if !strings.Contains(fail.FailureDetail, "expected 1 got 2") {
		t.Errorf("FailureDetail should include t.Errorf output; got %q", fail.FailureDetail)
	}
	if report.FailureSummary == "" {
		t.Error("FailureSummary should be populated on failure")
	}
}

func TestParseGoTestJSONLines_Skipped(t *testing.T) {
	output := `{"Action":"run","Package":"pkg","Test":"TestSkip"}
{"Action":"skip","Package":"pkg","Test":"TestSkip","Elapsed":0}
{"Action":"skip","Package":"pkg","Elapsed":0}
`
	report, err := parseGoTestJSONLines(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Skipped tests count as passed for verify purposes.
	if !report.Passed {
		t.Error("skip-only report should Passed=true")
	}
	if len(report.TestResults) != 1 {
		t.Errorf("len = %d, want 1", len(report.TestResults))
	}
	if !report.TestResults[0].Passed {
		t.Error("skipped test should have Passed=true")
	}
}

func TestParseGoTestJSONLines_BuildError(t *testing.T) {
	// No JSON lines — cargo/go build error path dumps text to stderr.
	output := `# pkg
./foo.go:10: undefined: Foo
FAIL    pkg [build failed]
`
	report, err := parseGoTestJSONLines(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// No events → no tests → Passed=false (empty results is a fail).
	if report.Passed {
		t.Error("build error path should not report Passed")
	}
}

func TestParseUnittestOutput_LoaderOnlyIsParserError(t *testing.T) {
	output := `requests (unittest.loader._FailedTest.requests) ... ERROR
test_requests (unittest.loader._FailedTest.test_requests) ... ERROR

======================================================================
ERROR: requests (unittest.loader._FailedTest.requests)
----------------------------------------------------------------------
ImportError: Failed to import test module: requests
Traceback (most recent call last):
  File "/usr/lib/python3.11/unittest/loader.py", line 154, in loadTestsFromName
    module = __import__(module_name)
ImportError: cannot import name 'MutableMapping' from 'collections'

----------------------------------------------------------------------
Ran 2 tests in 0.001s

FAILED (errors=2)
`
	report, err := parseUnittestOutput(output, fmt.Errorf("exit status 1"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Fatal("loader-only unittest result should not pass")
	}
	if report.FailureKind != types.FailureKindParserError {
		t.Fatalf("FailureKind = %q, want %q", report.FailureKind, types.FailureKindParserError)
	}
	if !strings.Contains(report.FailureSummary, "collection/import failed") {
		t.Fatalf("FailureSummary = %q", report.FailureSummary)
	}
}

func TestParseUnittestOutput_RealTestFailureStaysUnclassified(t *testing.T) {
	output := `test_ok (tests.test_demo.DemoTest.test_ok) ... ok
test_bad (tests.test_demo.DemoTest.test_bad) ... FAIL

======================================================================
FAIL: test_bad (tests.test_demo.DemoTest.test_bad)
----------------------------------------------------------------------
AssertionError: 1 != 2

----------------------------------------------------------------------
Ran 2 tests in 0.001s

FAILED (failures=1)
`
	report, err := parseUnittestOutput(output, fmt.Errorf("exit status 1"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Fatal("real unittest failure should not pass")
	}
	if report.FailureKind != "" {
		t.Fatalf("parser should not classify real test failure directly, got %q", report.FailureKind)
	}
}

func TestParseGoTestJSONLines_PackageBuildFailWithPassingSibling(t *testing.T) {
	output := `{"ImportPath":"github.com/example/root.test","Action":"build-output","Output":"# github.com/example/root\n"}
{"ImportPath":"github.com/example/root.test","Action":"build-output","Output":"mux_test.go:1672:45: unknown escape sequence\n"}
{"ImportPath":"github.com/example/root.test","Action":"build-fail"}
{"Time":"2026-06-10T15:57:06Z","Action":"start","Package":"github.com/example/root"}
{"Time":"2026-06-10T15:57:06Z","Action":"output","Package":"github.com/example/root","Output":"FAIL\tgithub.com/example/root [setup failed]\n"}
{"Time":"2026-06-10T15:57:06Z","Action":"fail","Package":"github.com/example/root","Elapsed":0,"FailedBuild":"github.com/example/root.test"}
{"Time":"2026-06-10T15:57:07Z","Action":"run","Package":"github.com/example/root/middleware","Test":"TestOK"}
{"Time":"2026-06-10T15:57:07Z","Action":"pass","Package":"github.com/example/root/middleware","Test":"TestOK","Elapsed":0.01}
{"Time":"2026-06-10T15:57:07Z","Action":"pass","Package":"github.com/example/root/middleware","Elapsed":0.01}
`
	report, err := parseGoTestJSONLines(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Fatal("mixed package build failure must not pass because a sibling package had passing tests")
	}
	if !report.BuildFailed {
		t.Fatal("BuildFailed should be true when any package reports build-fail")
	}
	var buildRow *types.TestResult
	var passingUnit int
	for i := range report.TestResults {
		row := &report.TestResults[i]
		if row.Kind == types.TestResultKindBuildError {
			buildRow = row
		}
		if row.Kind == types.TestResultKindUnit && row.Passed {
			passingUnit++
		}
	}
	if buildRow == nil {
		t.Fatalf("expected a synthetic build_error row; got %+v", report.TestResults)
	}
	if buildRow.Passed {
		t.Fatal("build_error row must be failed")
	}
	if len(buildRow.BuildErrors) == 0 {
		t.Fatalf("build_error row should carry parsed BuildErrors; got %+v", buildRow)
	}
	if got := buildRow.BuildErrors[0]; got.File != "mux_test.go" || got.Line != 1672 || got.Column != 45 || !strings.Contains(got.Message, "unknown escape sequence") {
		t.Fatalf("unexpected BuildError: %+v", got)
	}
	if passingUnit != 1 {
		t.Fatalf("expected sibling passing unit test to remain recorded; got %d", passingUnit)
	}
	if !strings.Contains(report.FailureSummary, "unknown escape sequence") {
		t.Fatalf("FailureSummary should lead with build failure detail; got %q", report.FailureSummary)
	}
}

func TestParseGoTestJSONLines_MalformedLineIgnored(t *testing.T) {
	output := `{"Action":"run","Package":"pkg","Test":"TestA"}
this is not json at all, should be ignored silently
{"Action":"pass","Package":"pkg","Test":"TestA","Elapsed":0.1}
`
	report, err := parseGoTestJSONLines(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(report.TestResults) != 1 {
		t.Errorf("malformed line should be skipped, not break parse; got %d results", len(report.TestResults))
	}
}

// ── Jest parser tests ────────────────────────────────────────────

func TestParseJestJSON_AllPassed(t *testing.T) {
	output := `{
	"numPassedTests": 2,
	"numFailedTests": 0,
	"numTotalTests": 2,
	"success": true,
	"testResults": [
		{
			"name": "tests/foo.test.js",
			"assertionResults": [
				{"title": "should add", "status": "passed", "duration": 5, "ancestorTitles": ["math"]},
				{"title": "should subtract", "status": "passed", "duration": 3, "ancestorTitles": ["math"]}
			]
		}
	]
}`
	report, err := parseJestJSON(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Error("Passed should be true")
	}
	if len(report.TestResults) != 2 {
		t.Errorf("TestResults len = %d, want 2", len(report.TestResults))
	}
	// Assertion IDs should be "describe > it" format.
	if !strings.HasPrefix(report.TestResults[0].AssertionID, "math >") {
		t.Errorf("AssertionID should include ancestor; got %q", report.TestResults[0].AssertionID)
	}
}

func TestParseJestJSON_WithFailures(t *testing.T) {
	output := `{
	"numPassedTests": 1,
	"numFailedTests": 1,
	"numTotalTests": 2,
	"success": false,
	"testResults": [
		{
			"name": "tests/bar.test.js",
			"assertionResults": [
				{"title": "passes", "status": "passed", "duration": 1, "ancestorTitles": []},
				{"title": "fails", "status": "failed", "duration": 2,
				 "failureMessages": ["Expected 1 to equal 2\nat line 10"],
				 "ancestorTitles": ["nested"]}
			]
		}
	]
}`
	report, err := parseJestJSON(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("Passed should be false")
	}
	// Find the failing test.
	var fail *types.TestResult
	for i := range report.TestResults {
		if !report.TestResults[i].Passed {
			fail = &report.TestResults[i]
			break
		}
	}
	if fail == nil {
		t.Fatal("expected failing test")
	}
	if !strings.Contains(fail.FailureDetail, "Expected 1 to equal 2") {
		t.Errorf("FailureDetail should include assertion error; got %q", fail.FailureDetail)
	}
}

func TestParseJestJSON_MixedWithStderr(t *testing.T) {
	// npm test often prefixes with "> project@1.0.0 test" lines.
	output := `> myproj@1.0.0 test
> jest --json --silent

{"numPassedTests":1,"numFailedTests":0,"numTotalTests":1,"success":true,"testResults":[{"name":"a.test.js","assertionResults":[{"title":"ok","status":"passed","duration":1,"ancestorTitles":[]}]}]}
`
	report, err := parseJestJSON(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed || len(report.TestResults) != 1 {
		t.Errorf("should parse JSON from mixed output; got passed=%v len=%d", report.Passed, len(report.TestResults))
	}
}

func TestParseJestJSON_NoJSON(t *testing.T) {
	_, err := parseJestJSON("no json here at all")
	if err == nil {
		t.Error("should error when no JSON object found")
	}
}

// ── pytest parser tests ──────────────────────────────────────────

func TestParsePytestJSONReport_AllPassed(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	payload := `{
		"exitcode": 0,
		"summary": {"passed": 2, "failed": 0, "skipped": 0, "error": 0, "total": 2},
		"tests": [
			{"nodeid": "tests/test_a.py::test_foo", "outcome": "passed", "duration": 0.05,
			 "call": {"longrepr": ""}},
			{"nodeid": "tests/test_a.py::test_bar", "outcome": "passed", "duration": 0.03,
			 "call": {"longrepr": ""}}
		]
	}`
	if err := os.WriteFile(reportPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	report, err := parsePytestJSONReport(reportPath, "", "pytest ...")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Error("Passed should be true")
	}
	if len(report.TestResults) != 2 {
		t.Errorf("len = %d, want 2", len(report.TestResults))
	}
	// Suite + name split on "::".
	if report.TestResults[0].Suite != "tests/test_a.py" {
		t.Errorf("Suite = %q, want 'tests/test_a.py'", report.TestResults[0].Suite)
	}
	if report.TestResults[0].AssertionID != "test_foo" {
		t.Errorf("AssertionID = %q, want 'test_foo'", report.TestResults[0].AssertionID)
	}
}

func TestParsePytestJSONReport_MissingReportIsInfrastructureDiagnostic(t *testing.T) {
	stdout := "ImportError while loading conftest 'tests/conftest.py'\nModuleNotFoundError: No module named 'demo_dep'"
	_, err := parsePytestJSONReport("/tmp/nonexistent-b1-test.json", stdout, "pytest ...")
	if err == nil {
		t.Fatal("should error when report file missing")
	}
	msg := err.Error()
	if !strings.Contains(msg, "pytest did not produce the requested JSON report") {
		t.Errorf("error should describe missing report generically; got %q", msg)
	}
	if !strings.Contains(msg, "collection/import/startup") {
		t.Errorf("error should include collection/import/startup diagnostic; got %q", msg)
	}
	if !strings.Contains(msg, "pytest-json-report") {
		t.Errorf("error should still mention the plugin as one possible cause; got %q", msg)
	}
}

func TestParsePytestJSONReport_WithFailures(t *testing.T) {
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")
	payload := `{
		"exitcode": 1,
		"summary": {"passed": 1, "failed": 1, "skipped": 0, "error": 0, "total": 2},
		"tests": [
			{"nodeid": "tests/test_x.py::test_ok", "outcome": "passed", "duration": 0.01,
			 "call": {"longrepr": ""}},
			{"nodeid": "tests/test_x.py::test_bad", "outcome": "failed", "duration": 0.02,
			 "call": {"longrepr": "AssertionError: expected 1 got 2\n  at line 42"}}
		]
	}`
	if err := os.WriteFile(reportPath, []byte(payload), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	report, err := parsePytestJSONReport(reportPath, "", "pytest")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("Passed should be false")
	}
	for _, r := range report.TestResults {
		if r.AssertionID == "test_bad" && !strings.Contains(r.FailureDetail, "expected 1 got 2") {
			t.Errorf("failing test should carry longrepr; got %q", r.FailureDetail)
		}
	}
}

func TestParsePytestTextOutput_AllPassed(t *testing.T) {
	output := `============================= test session starts ==============================
collected 2 items

testing/code/test_excinfo.py::test_excinfo_str PASSED                  [ 50%]
testing/code/test_source.py::test_source PASSED                        [100%]

============================== 2 passed in 0.12s ===============================`
	report, err := parsePytestTextOutput(output, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 pytest", nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Fatal("Passed should be true")
	}
	if len(report.TestResults) != 2 {
		t.Fatalf("len(TestResults) = %d, want 2", len(report.TestResults))
	}
	if report.TestResults[0].Suite != "testing/code/test_excinfo.py" {
		t.Fatalf("Suite = %q", report.TestResults[0].Suite)
	}
	if report.TestResults[0].AssertionID != "test_excinfo_str" {
		t.Fatalf("AssertionID = %q", report.TestResults[0].AssertionID)
	}
}

func TestParsePytestTextOutput_FailureSummary(t *testing.T) {
	output := `============================= test session starts ==============================
collected 2 items

tests/test_a.py::test_ok PASSED                                         [ 50%]
tests/test_a.py::test_bad FAILED                                        [100%]

=================================== FAILURES ===================================
___________________________________ test_bad ___________________________________
E       AssertionError: expected 1 got 2
=========================== 1 failed, 1 passed in 0.08s ===========================`
	report, err := parsePytestTextOutput(output, "pytest", fmt.Errorf("exit status 1"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Fatal("Passed should be false")
	}
	if report.FailureKind != "" {
		t.Fatalf("parser should not backfill FailureKind directly, got %q", report.FailureKind)
	}
	if !strings.Contains(report.FailureSummary, "1 failed") {
		t.Fatalf("FailureSummary = %q", report.FailureSummary)
	}
	var found bool
	for _, tr := range report.TestResults {
		if tr.AssertionID == "test_bad" {
			found = true
			if tr.Passed {
				t.Fatal("test_bad should be failed")
			}
			if !strings.Contains(tr.FailureDetail, "AssertionError") {
				t.Fatalf("FailureDetail = %q", tr.FailureDetail)
			}
		}
	}
	if !found {
		t.Fatalf("missing failed row: %+v", report.TestResults)
	}
}

func TestParsePytestTextOutput_NoTests(t *testing.T) {
	report, err := parsePytestTextOutput("collected 0 items\n\n============================ no tests ran in 0.01s =============================",
		"pytest", nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Fatal("NoTests should be a passed unverified runner report")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "python" {
		t.Fatalf("NoTestsRunners = %+v", report.NoTestsRunners)
	}
}

func TestParsePytestTextOutput_StartupCrashIsParserError(t *testing.T) {
	output := `Traceback (most recent call last):
  File "<frozen importlib._bootstrap>", line 1072, in _find_spec
AttributeError: 'AssertionRewritingHook' object has no attribute 'find_spec'
`
	if _, err := parsePytestTextOutput(output, "pytest", fmt.Errorf("exit status 1")); err == nil {
		t.Fatal("startup crash without pytest summary should remain parser_error")
	}
}

func TestParsePytestTextOutput_CollectionErrorWithoutCasesIsParserError(t *testing.T) {
	output := `============================= test session starts ==============================
collecting ... collected 0 items / 1 error

==================================== ERRORS ====================================
________________________ ERROR collecting conftest.py ________________________
ImportError: cannot import name 'Mapping' from 'collections'
=========================== 1 error in 0.19s ===========================`
	if _, err := parsePytestTextOutput(output, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 pytest -v", fmt.Errorf("exit status 2")); err == nil {
		t.Fatal("collection/import errors without executed test rows should remain parser_error")
	}
}

func TestBuildPlainPytestFallbackCommandIsVerbose(t *testing.T) {
	plan := runnerPlan{Runner: "python", Framework: pythonFrameworkPytest, Root: t.TempDir(), Suite: "tests/test_a.py::test_bad"}
	got := buildPlainPytestFallbackCommandForPlan(plan, plan.Suite, plan.Root)
	if !strings.Contains(got, "PYTEST_DISABLE_PLUGIN_AUTOLOAD=1 ") {
		t.Fatalf("fallback command must disable third-party pytest plugin autoload, got %q", got)
	}
	if !strings.Contains(got, " -v ") {
		t.Fatalf("fallback command must be verbose so text parsing sees case rows, got %q", got)
	}
	if !strings.Contains(got, "tests/test_a.py::test_bad") {
		t.Fatalf("fallback command lost suite selector: %q", got)
	}
}

// ── Cargo parser tests ───────────────────────────────────────────

func TestParseCargoTestText_AllPassed(t *testing.T) {
	output := `
running 3 tests
test tests::foo ... ok
test tests::bar ... ok
test tests::baz ... ok

test result: ok. 3 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out
`
	report, err := parseCargoTestText("rust", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Error("Passed should be true")
	}
	if len(report.TestResults) != 3 {
		t.Errorf("len = %d, want 3", len(report.TestResults))
	}
}

func TestParseCargoTestText_MixedWithFailures(t *testing.T) {
	output := `
running 3 tests
test tests::a ... ok
test tests::b ... FAILED
test tests::c ... ok

failures:

---- tests::b stdout ----
thread 'tests::b' panicked at 'assertion failed: x == y', src/lib.rs:42:5
note: run with RUST_BACKTRACE=1 for a backtrace.

failures:
    tests::b

test result: FAILED. 2 passed; 1 failed; 0 ignored; 0 measured; 0 filtered out
`
	report, err := parseCargoTestText("rust", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("Passed should be false")
	}
	var fail *types.TestResult
	for i := range report.TestResults {
		if !report.TestResults[i].Passed {
			fail = &report.TestResults[i]
			break
		}
	}
	if fail == nil {
		t.Fatal("expected failing test")
	}
	if fail.AssertionID != "tests::b" {
		t.Errorf("failing test name = %q, want tests::b", fail.AssertionID)
	}
	if !strings.Contains(fail.FailureDetail, "assertion failed") {
		t.Errorf("FailureDetail should include panic message; got %q", fail.FailureDetail)
	}
}

func TestParseCargoTestText_IgnoredIsPassed(t *testing.T) {
	output := `
running 2 tests
test tests::run_me ... ok
test tests::ignore_me ... ignored

test result: ok. 1 passed; 0 failed; 1 ignored; 0 measured; 0 filtered out
`
	report, err := parseCargoTestText("rust", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Error("Passed should be true when only ignored + passed")
	}
}

func TestParseCargoTestText_BuildError(t *testing.T) {
	output := "\nerror[E0308]: mismatched types\n --> src/lib.rs:10:5\nerror: aborting due to previous error\nerror: could not compile `mycrate`.\n"
	report, err := parseCargoTestText("rust", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("build error should not Passed")
	}
	if !report.BuildFailed {
		t.Error("ChangeReport.BuildFailed should be true on cargo build failure")
	}
	// Should synthesize a Kind=BuildError row (no string convention).
	if len(report.TestResults) != 1 {
		t.Fatalf("expected exactly one synthetic build-error row; got %d", len(report.TestResults))
	}
	tr := report.TestResults[0]
	if tr.Kind != types.TestResultKindBuildError {
		t.Errorf("Kind = %q; want %q", tr.Kind, types.TestResultKindBuildError)
	}
	if tr.Suite != "build" {
		t.Errorf("Suite = %q; want \"build\"", tr.Suite)
	}
}

// ── detectRunner tests ───────────────────────────────────────────

func TestDetectRunner(t *testing.T) {
	cases := []struct {
		name    string
		seeds   []string
		wantTag string
	}{
		{"go.mod", []string{"go.mod"}, "go"},
		{"package.json", []string{"package.json"}, "node"},
		{"pyproject.toml", []string{"pyproject.toml"}, "python"},
		{"pytest.ini", []string{"pytest.ini"}, "python"},
		{"setup.py", []string{"setup.py"}, "python"},
		{"Cargo.toml", []string{"Cargo.toml"}, "rust"},
		{"empty dir", []string{}, ""},
		{"go precedence over js", []string{"go.mod", "package.json"}, "go"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range c.seeds {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
					t.Fatalf("seed %s: %v", f, err)
				}
			}
			got := detectRunner(dir)
			if got != c.wantTag {
				t.Errorf("detectRunner = %q, want %q", got, c.wantTag)
			}
		})
	}
}

// ── parseRunnerOutput dispatcher ─────────────────────────────────

func TestParseRunnerOutput_UnknownRunner(t *testing.T) {
	_, err := parseRunnerOutput("nonexistent", "output", "", "", nil)
	if err == nil {
		t.Error("should error on unknown runner")
	}
}

// TestBuildGoFailureSummary_NoMisleadingCompileHintWhenAssertionsFail
// pins the bug fix from eval Batch G trinary task. Earlier the
// summary unconditionally appended "typically compile or init
// errors" whenever Go marked a package as fail — but Go marks the
// whole package failed even when failure is just a per-test
// assertion. The misleading text seeded wrong-direction critiques
// into the reflector pipeline, wasting 3 retries on "signature
// mismatch" diagnoses while the actual bug was wrong arithmetic.
func TestBuildGoFailureSummary_NoMisleadingCompileHintWhenAssertionsFail(t *testing.T) {
	results := []types.TestResult{
		{AssertionID: "TestParseTrinary", Passed: false, FailureDetail: "got 1, want 3"},
	}
	pkgStatus := map[string]string{"trinary": "fail"}
	got := buildGoFailureSummary(results, pkgStatus)
	if strings.Contains(got, "typically compile or init errors") {
		t.Errorf("when per-test failures exist, summary must NOT claim 'compile or init errors' (misleads reflector); got %q", got)
	}
	if !strings.Contains(got, "1 test(s) failed") {
		t.Errorf("summary should report per-test failure count; got %q", got)
	}
	if !strings.Contains(got, "package-level fail status") {
		t.Errorf("summary should clarify the Go package-fail-on-any-test convention; got %q", got)
	}
}

// TestBuildGoFailureSummary_TrueCompileErrorPreservesHint covers the
// reverse path: when the package failed with NO per-test events
// (genuine compile error before tests run), the "typically compile
// or init errors" qualifier IS the right diagnostic for the
// reflector. Don't accidentally remove it.
func TestBuildGoFailureSummary_TrueCompileErrorPreservesHint(t *testing.T) {
	results := []types.TestResult{} // no per-test entries
	pkgStatus := map[string]string{"trinary": "fail"}
	got := buildGoFailureSummary(results, pkgStatus)
	if !strings.Contains(got, "typically compile or init errors") {
		t.Errorf("zero per-test entries + package fail = real compile error; summary should keep the qualifier; got %q", got)
	}
}

// ── NoTestsRunners coverage across all 12 runners ──────────────────
//
// "No tests collected" is a clean run with zero work, not a verify
// failure. Every runner's parser must surface NoTestsRunners and keep
// Passed=true when its underlying tool reports zero discoverable
// tests; otherwise a plan that touches a non-test file in a project
// without a test suite triggers a false-positive verify→plan retry.
//
// Provenance: pytest exit 5 ("no tests collected") on a Python script
// in a Go-only repo produced FailureSummary="exitcode=5 — 0 of 0
// total" → verify_failed → planner hallucinated test_*.py to "fix"
// the absent suite. Generalising the signal closes the same hole for
// 12 runners at once.

func TestParseGoTestJSONLines_NoTests(t *testing.T) {
	// `go test ./...` on a package with no _test.go files prints
	// nothing JSON-shaped (test2json emits zero events).
	report, err := parseGoTestJSONLines("")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (no tests is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "go" {
		t.Errorf("NoTestsRunners = %v; want [go]", report.NoTestsRunners)
	}
	if report.FailureSummary != "" {
		t.Errorf("FailureSummary should be empty on Passed=true; got %q", report.FailureSummary)
	}
}

func TestParseJestJSON_NoTests(t *testing.T) {
	// Jest with no test files matched: numTotalTests=0, success=false
	// (jest defaults to failing without --passWithNoTests).
	output := `{
		"numPassedTests": 0,
		"numFailedTests": 0,
		"numTotalTests": 0,
		"success": false,
		"testResults": []
	}`
	report, err := parseJestJSON(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (no tests is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "node" {
		t.Errorf("NoTestsRunners = %v; want [node]", report.NoTestsRunners)
	}
}

func TestParsePytestJSONReport_ExitCode5(t *testing.T) {
	// pytest exit 5 = "no tests were collected" — a clean run.
	tmp := t.TempDir()
	reportFile := filepath.Join(tmp, "pytest.json")
	body := `{"exitcode": 5, "summary": {"passed": 0, "failed": 0, "error": 0, "skipped": 0, "total": 0}, "tests": []}`
	if err := os.WriteFile(reportFile, []byte(body), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	report, err := parsePytestJSONReport(reportFile, "no tests ran", "pytest")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (exit 5 is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "python" {
		t.Errorf("NoTestsRunners = %v; want [python]", report.NoTestsRunners)
	}
	if report.FailureSummary != "" {
		t.Errorf("FailureSummary should be empty on Passed=true; got %q", report.FailureSummary)
	}
}

func TestParsePytestJSONReport_ZeroTestsExitCode2And4AreUnverified(t *testing.T) {
	for _, exitCode := range []int{2, 4} {
		t.Run(fmt.Sprintf("exit_%d", exitCode), func(t *testing.T) {
			tmp := t.TempDir()
			reportFile := filepath.Join(tmp, "pytest.json")
			body := fmt.Sprintf(`{"exitcode": %d, "summary": {"passed": 0, "failed": 0, "error": 0, "skipped": 0, "total": 0}, "tests": []}`, exitCode)
			if err := os.WriteFile(reportFile, []byte(body), 0o644); err != nil {
				t.Fatalf("write report: %v", err)
			}
			report, err := parsePytestJSONReport(reportFile, "no tests ran", "pytest bad-selector")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !report.Passed {
				t.Fatalf("zero-test exit %d should not be a code failure: %+v", exitCode, report)
			}
			if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "python" {
				t.Fatalf("NoTestsRunners = %v; want [python]", report.NoTestsRunners)
			}
			if report.FailureSummary != "" {
				t.Fatalf("FailureSummary should be empty for no-tests exit %d, got %q", exitCode, report.FailureSummary)
			}
		})
	}
}

func TestParseCargoTestText_NoTests(t *testing.T) {
	// `cargo test` on a crate with no #[test] items.
	output := `running 0 tests

test result: ok. 0 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out
`
	report, err := parseCargoTestText("rust", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (no tests is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "rust" {
		t.Errorf("NoTestsRunners = %v; want [rust]", report.NoTestsRunners)
	}
}

func TestParseCargoTestText_NoTests_CJPM(t *testing.T) {
	// cjpm reuses the cargo parser; runnerLabel must propagate.
	output := `test result: ok. 0 passed; 0 failed
`
	report, err := parseCargoTestText("cjpm", output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (no tests is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "cjpm" {
		t.Errorf("NoTestsRunners = %v; want [cjpm]", report.NoTestsRunners)
	}
}

func TestParseRSpecJSON_NoTests(t *testing.T) {
	output := `{"summary": {"example_count": 0, "failure_count": 0, "pending_count": 0}, "examples": []}`
	report, err := parseRSpecJSON(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (no specs is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "ruby" {
		t.Errorf("NoTestsRunners = %v; want [ruby]", report.NoTestsRunners)
	}
}

func TestParseSwiftOutput_NoTests(t *testing.T) {
	// SwiftPM ran cleanly but found no XCTest cases. runErr=nil.
	report, err := parseSwiftOutput("", nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Errorf("Passed = false; want true (no tests is not a failure)")
	}
	if len(report.NoTestsRunners) != 1 || report.NoTestsRunners[0] != "swift" {
		t.Errorf("NoTestsRunners = %v; want [swift]", report.NoTestsRunners)
	}
}
