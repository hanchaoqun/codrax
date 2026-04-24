package tool

import (
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

func TestParsePytestJSONReport_PluginMissing(t *testing.T) {
	// Report file doesn't exist → plugin-missing hint.
	_, err := parsePytestJSONReport("/tmp/nonexistent-b1-test.json", "pytest stdout here", "pytest ...")
	if err == nil {
		t.Fatal("should error when report file missing")
	}
	if !strings.Contains(err.Error(), "pytest-json-report") {
		t.Errorf("error should hint at plugin install; got %q", err.Error())
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

// ── Cargo parser tests ───────────────────────────────────────────

func TestParseCargoTestText_AllPassed(t *testing.T) {
	output := `
running 3 tests
test tests::foo ... ok
test tests::bar ... ok
test tests::baz ... ok

test result: ok. 3 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out
`
	report, err := parseCargoTestText(output)
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
	report, err := parseCargoTestText(output)
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
	report, err := parseCargoTestText(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.Passed {
		t.Error("Passed should be true when only ignored + passed")
	}
}

func TestParseCargoTestText_BuildError(t *testing.T) {
	output := "\nerror[E0308]: mismatched types\n --> src/lib.rs:10:5\nerror: aborting due to previous error\nerror: could not compile `mycrate`.\n"
	report, err := parseCargoTestText(output)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("build error should not Passed")
	}
	// Should synthesize a "cargo-build" failure entry.
	if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "cargo-build" {
		t.Errorf("expected synthetic cargo-build failure; got %+v", report.TestResults)
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
	_, err := parseRunnerOutput("nonexistent", "output", "", "")
	if err == nil {
		t.Error("should error on unknown runner")
	}
}
