package tool

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── RSpec ─────────────────────────────────────────────────────────

const rspecAllPassed = `{
  "version": "3.12.0",
  "summary": {
    "duration": 0.01,
    "example_count": 2,
    "failure_count": 0,
    "pending_count": 0
  },
  "examples": [
    {
      "description": "returns true",
      "full_description": "Foo#bar returns true",
      "status": "passed",
      "file_path": "./spec/foo_spec.rb",
      "line_number": 5,
      "run_time": 0.003
    },
    {
      "description": "handles nil",
      "full_description": "Foo#baz handles nil",
      "status": "passed",
      "file_path": "./spec/foo_spec.rb",
      "line_number": 12,
      "run_time": 0.004
    }
  ]
}
`

func TestParseRSpecJSON_AllPassed(t *testing.T) {
	report, err := parseRSpecJSON(rspecAllPassed)
	if err != nil {
		t.Fatalf("parseRSpecJSON: %v", err)
	}
	if !report.Passed {
		t.Error("all-passed rspec should be Passed=true")
	}
	if len(report.TestResults) != 2 {
		t.Errorf("want 2 results; got %d", len(report.TestResults))
	}
	if report.TestResults[0].AssertionID != "Foo#bar returns true" {
		t.Errorf("AssertionID = %q; want 'Foo#bar returns true'", report.TestResults[0].AssertionID)
	}
	if report.TestResults[0].Suite != "./spec/foo_spec.rb" {
		t.Errorf("Suite = %q; want file_path", report.TestResults[0].Suite)
	}
}

const rspecMixed = `
.F.
{
  "version": "3.12.0",
  "summary": {
    "duration": 0.05,
    "example_count": 3,
    "failure_count": 1,
    "pending_count": 0
  },
  "examples": [
    {"description":"ok1","full_description":"Foo ok1","status":"passed","file_path":"./spec/a.rb","run_time":0.01},
    {"description":"broken","full_description":"Foo broken","status":"failed","file_path":"./spec/a.rb","run_time":0.02,"exception":{"class":"RuntimeError","message":"boom"}},
    {"description":"ok2","full_description":"Foo ok2","status":"passed","file_path":"./spec/a.rb","run_time":0.01}
  ]
}
`

func TestParseRSpecJSON_MixedWithFailure(t *testing.T) {
	report, err := parseRSpecJSON(rspecMixed)
	if err != nil {
		t.Fatalf("parseRSpecJSON: %v", err)
	}
	if report.Passed {
		t.Error("with failure_count>0 Passed should be false")
	}
	if len(report.TestResults) != 3 {
		t.Fatalf("want 3 results; got %d", len(report.TestResults))
	}
	// Find the failing one and check its detail.
	var broken *struct{}
	for _, tr := range report.TestResults {
		if !tr.Passed {
			broken = &struct{}{}
			if !strings.Contains(tr.FailureDetail, "RuntimeError") {
				t.Errorf("FailureDetail should name exception class; got %q", tr.FailureDetail)
			}
			if !strings.Contains(tr.FailureDetail, "boom") {
				t.Errorf("FailureDetail should include exception message; got %q", tr.FailureDetail)
			}
		}
	}
	if broken == nil {
		t.Error("expected one failed TestResult; found none")
	}
	if !strings.Contains(report.FailureSummary, "1 of 3") {
		t.Errorf("FailureSummary should state count; got %q", report.FailureSummary)
	}
}

func TestParseRSpecJSON_PendingIsPassed(t *testing.T) {
	// RSpec "pending" is not a failure; treat as Passed=true so
	// CritTestsPass doesn't trip on skipped examples.
	payload := `{
  "summary": {"example_count": 1, "failure_count": 0, "pending_count": 1},
  "examples": [
    {"description":"todo","full_description":"Foo todo","status":"pending","file_path":"./spec/p.rb","run_time":0.0}
  ]
}`
	report, err := parseRSpecJSON(payload)
	if err != nil {
		t.Fatalf("parseRSpecJSON: %v", err)
	}
	if !report.Passed {
		t.Error("pending-only run should be Passed=true")
	}
	if !report.TestResults[0].Passed {
		t.Error("pending example should not flip Passed=false")
	}
}

func TestParseRSpecJSON_NoJSON(t *testing.T) {
	_, err := parseRSpecJSON("total garbage output no braces")
	if err == nil {
		t.Fatal("garbage input should error cleanly")
	}
	if !strings.Contains(err.Error(), "no JSON") {
		t.Errorf("err should name the missing-JSON case; got %v", err)
	}
}

// ── JUnit XML ─────────────────────────────────────────────────────

const junitTwoSuitesMixed = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.foo.BarTest" tests="3" failures="1" errors="0" skipped="1" time="0.42">
  <testcase name="testHappy" classname="com.foo.BarTest" time="0.12"/>
  <testcase name="testFail" classname="com.foo.BarTest" time="0.20">
    <failure message="expected 1 got 2" type="AssertionError">at com.foo.BarTest.testFail(BarTest.java:42)</failure>
  </testcase>
  <testcase name="testSkipped" classname="com.foo.BarTest" time="0.0">
    <skipped/>
  </testcase>
</testsuite>`

const junitSingleSuiteAllPass = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.foo.BazTest" tests="2" failures="0" errors="0" skipped="0" time="0.1">
  <testcase name="t1" classname="com.foo.BazTest" time="0.05"/>
  <testcase name="t2" classname="com.foo.BazTest" time="0.05"/>
</testsuite>`

// seedJUnitDir drops the given xml contents into a fresh directory
// so parseJUnitXMLDir can walk it like a real Maven/Gradle output.
func seedJUnitDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

func TestParseJUnitXMLDir_SingleSuite(t *testing.T) {
	dir := seedJUnitDir(t, map[string]string{
		"TEST-com.foo.BazTest.xml": junitSingleSuiteAllPass,
	})
	report, err := parseJUnitXMLDir(dir, "")
	if err != nil {
		t.Fatalf("parseJUnitXMLDir: %v", err)
	}
	if !report.Passed {
		t.Error("all-pass should yield Passed=true")
	}
	if len(report.TestResults) != 2 {
		t.Errorf("want 2 results; got %d", len(report.TestResults))
	}
	if report.TestResults[0].AssertionID != "com.foo.BazTest#t1" {
		t.Errorf("AssertionID = %q; want 'com.foo.BazTest#t1'", report.TestResults[0].AssertionID)
	}
}

func TestParseJUnitXMLDir_MixedFailuresAndSkipped(t *testing.T) {
	dir := seedJUnitDir(t, map[string]string{
		"TEST-com.foo.BarTest.xml": junitTwoSuitesMixed,
	})
	report, err := parseJUnitXMLDir(dir, "")
	if err != nil {
		t.Fatalf("parseJUnitXMLDir: %v", err)
	}
	if report.Passed {
		t.Error("one failure should flip Passed=false")
	}
	if len(report.TestResults) != 3 {
		t.Fatalf("want 3 results (2 pass + 1 fail + 1 skipped=passed); got %d", len(report.TestResults))
	}
	// Skipped case should be Passed=true.
	var skipped *bool
	for _, tr := range report.TestResults {
		if tr.AssertionID == "com.foo.BarTest#testSkipped" {
			b := tr.Passed
			skipped = &b
		}
	}
	if skipped == nil {
		t.Fatal("expected testSkipped entry")
	}
	if !*skipped {
		t.Error("skipped case should be Passed=true")
	}
	// Failing case should have detail.
	for _, tr := range report.TestResults {
		if tr.AssertionID == "com.foo.BarTest#testFail" {
			if !strings.Contains(tr.FailureDetail, "AssertionError") {
				t.Errorf("failure detail should include type; got %q", tr.FailureDetail)
			}
			if !strings.Contains(tr.FailureDetail, "expected 1 got 2") {
				t.Errorf("failure detail should include message; got %q", tr.FailureDetail)
			}
		}
	}
	if !strings.Contains(report.FailureSummary, "1 of 3") {
		t.Errorf("FailureSummary should state count; got %q", report.FailureSummary)
	}
}

func TestParseJUnitXMLDir_MultiFileAggregation(t *testing.T) {
	// Simulate Maven multi-module: two suites in separate files.
	dir := seedJUnitDir(t, map[string]string{
		"TEST-com.foo.BarTest.xml": junitTwoSuitesMixed,     // 3 cases
		"TEST-com.foo.BazTest.xml": junitSingleSuiteAllPass, // 2 cases
	})
	report, err := parseJUnitXMLDir(dir, "")
	if err != nil {
		t.Fatalf("parseJUnitXMLDir: %v", err)
	}
	if len(report.TestResults) != 5 {
		t.Errorf("want 5 combined results; got %d", len(report.TestResults))
	}
	if report.Passed {
		t.Error("combined run has a failure → Passed=false")
	}
}

func TestParseJUnitXMLDir_NestedSubdirs(t *testing.T) {
	// Gradle multi-module writes under build/test-results/test/
	// with nested subdirectories per module. The walker recurses.
	dir := seedJUnitDir(t, map[string]string{
		"module-a/TEST-com.foo.A.xml": junitSingleSuiteAllPass,
		"module-b/TEST-com.foo.B.xml": junitSingleSuiteAllPass,
	})
	report, err := parseJUnitXMLDir(dir, "")
	if err != nil {
		t.Fatalf("parseJUnitXMLDir: %v", err)
	}
	if len(report.TestResults) != 4 {
		t.Errorf("want 4 results from 2 subdirs × 2 cases; got %d", len(report.TestResults))
	}
}

func TestParseJUnitXMLDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := parseJUnitXMLDir(dir, "stdout hint here")
	if err == nil {
		t.Fatal("empty dir should error")
	}
	if !strings.Contains(err.Error(), "no test cases parsed") {
		t.Errorf("err should explain zero cases; got %v", err)
	}
	if !strings.Contains(err.Error(), "stdout hint here") {
		t.Errorf("err should include stdout hint; got %v", err)
	}
}

func TestParseJUnitXMLDir_MalformedFileSkipped(t *testing.T) {
	dir := seedJUnitDir(t, map[string]string{
		"TEST-valid.xml":     junitSingleSuiteAllPass,
		"TEST-malformed.xml": "<not-xml",
	})
	report, err := parseJUnitXMLDir(dir, "")
	if err != nil {
		t.Fatalf("parseJUnitXMLDir: %v", err)
	}
	// Valid file's 2 cases should still be harvested.
	if len(report.TestResults) != 2 {
		t.Errorf("want 2 results from valid file; got %d", len(report.TestResults))
	}
}

// ── detectRunner + buildRunCommand for new languages ──────────────

func TestDetectRunner_JavaMaven(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := detectRunner(dir); got != "java" {
		t.Errorf("detectRunner(pom.xml) = %q, want 'java'", got)
	}
}

func TestDetectRunner_JavaGradle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.gradle"), []byte("plugins { id 'java' }"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := detectRunner(dir); got != "java" {
		t.Errorf("detectRunner(build.gradle) = %q, want 'java'", got)
	}
}

func TestDetectRunner_JavaGradleKts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "build.gradle.kts"), []byte(""), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := detectRunner(dir); got != "java" {
		t.Errorf("detectRunner(build.gradle.kts) = %q, want 'java'", got)
	}
}

func TestDetectRunner_Ruby(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Gemfile"), []byte("source 'https://rubygems.org'"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if got := detectRunner(dir); got != "ruby" {
		t.Errorf("detectRunner(Gemfile) = %q, want 'ruby'", got)
	}
}

func TestDetectJavaBuildSystem(t *testing.T) {
	mavenDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(mavenDir, "pom.xml"), []byte("<project/>"), 0o644); err != nil {
		t.Fatalf("seed pom: %v", err)
	}
	if got := detectJavaBuildSystem(mavenDir); got != "maven" {
		t.Errorf("maven detect = %q, want 'maven'", got)
	}

	gradleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(gradleDir, "build.gradle"), []byte(""), 0o644); err != nil {
		t.Fatalf("seed gradle: %v", err)
	}
	if got := detectJavaBuildSystem(gradleDir); got != "gradle" {
		t.Errorf("gradle detect = %q, want 'gradle'", got)
	}

	emptyDir := t.TempDir()
	if got := detectJavaBuildSystem(emptyDir); got != "" {
		t.Errorf("empty dir should yield ''; got %q", got)
	}
}

func TestDetectJavaBuildSystem_MavenBeatsGradle(t *testing.T) {
	// Polyglot edge case: both pom.xml and build.gradle present.
	// Maven wins per docstring.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644)
	os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(""), 0o644)
	if got := detectJavaBuildSystem(dir); got != "maven" {
		t.Errorf("pom.xml + build.gradle coexist → maven wins; got %q", got)
	}
}

func TestBuildRunCommand_RubyRSpec(t *testing.T) {
	cmd, extra := buildRunCommand("ruby", "", "/tmp")
	if !strings.Contains(cmd, "bundle exec rspec") {
		t.Errorf("ruby command should be bundle-exec rspec; got %q", cmd)
	}
	if !strings.Contains(cmd, "--format json") {
		t.Errorf("ruby command should request JSON format; got %q", cmd)
	}
	if extra != "" {
		t.Errorf("ruby uses stdout; extraFile should be empty; got %q", extra)
	}

	cmdFilter, _ := buildRunCommand("ruby", "models/user_spec.rb", "/tmp")
	if !strings.Contains(cmdFilter, "models/user_spec.rb") {
		t.Errorf("filter should appear in command; got %q", cmdFilter)
	}
}

func TestBuildRunCommand_JavaMaven(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte("<project/>"), 0o644)
	cmd, extra := buildRunCommand("java", "", dir)
	if !strings.Contains(cmd, "mvn") || !strings.Contains(cmd, "test") {
		t.Errorf("maven command should be mvn test; got %q", cmd)
	}
	// extraFile empty; parser walks a directory populated post-exec.
	if extra != "" {
		t.Errorf("java uses post-exec dir scan; extraFile here should be empty; got %q", extra)
	}
}

func TestBuildRunCommand_JavaGradle(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "build.gradle"), []byte(""), 0o644)
	cmd, _ := buildRunCommand("java", "", dir)
	if !strings.Contains(cmd, "gradlew") {
		t.Errorf("gradle command should invoke gradlew; got %q", cmd)
	}
	if !strings.Contains(cmd, "test") {
		t.Errorf("gradle command should run test task; got %q", cmd)
	}
}

// ── locateJUnitReportDir ──────────────────────────────────────────

func TestLocateJUnitReportDir_MavenSurefire(t *testing.T) {
	repo := t.TempDir()
	reportDir := filepath.Join(repo, "target", "surefire-reports")
	os.MkdirAll(reportDir, 0o755)
	os.WriteFile(filepath.Join(reportDir, "TEST-foo.xml"), []byte(junitSingleSuiteAllPass), 0o644)
	got := locateJUnitReportDir(repo)
	if got != reportDir {
		t.Errorf("want %q, got %q", reportDir, got)
	}
}

func TestLocateJUnitReportDir_GradleTestResults(t *testing.T) {
	repo := t.TempDir()
	reportDir := filepath.Join(repo, "build", "test-results", "test")
	os.MkdirAll(reportDir, 0o755)
	os.WriteFile(filepath.Join(reportDir, "TEST-foo.xml"), []byte(junitSingleSuiteAllPass), 0o644)
	got := locateJUnitReportDir(repo)
	if got != reportDir {
		t.Errorf("want %q, got %q", reportDir, got)
	}
}

func TestLocateJUnitReportDir_EmptyDirIgnored(t *testing.T) {
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "target", "surefire-reports"), 0o755)
	// No .xml files inside — should skip.
	if got := locateJUnitReportDir(repo); got != "" {
		t.Errorf("empty dir should be ignored; got %q", got)
	}
}

func TestLocateJUnitReportDir_NoReports(t *testing.T) {
	repo := t.TempDir()
	if got := locateJUnitReportDir(repo); got != "" {
		t.Errorf("bare repo should yield ''; got %q", got)
	}
}

// ── narrativeBuildErrorExcerpt ──────────────────────────────────────

func TestNarrativeBuildErrorExcerpt_MavenError(t *testing.T) {
	stdout := `[INFO] Scanning for projects...
[INFO] Building foo 1.0
[INFO] --- maven-compiler-plugin:3.8.1:compile (default-compile) ---
[ERROR] /home/ci/src/main/java/Foo.java:[17,23] cannot find symbol
[ERROR]   symbol:   variable bar
[ERROR]   location: class Foo
[INFO] BUILD FAILURE
[INFO] Total time: 2.1 s
`
	got := narrativeBuildErrorExcerpt(stdout)
	if !strings.Contains(got, "[ERROR] /home/ci/src/main/java/Foo.java:[17,23] cannot find symbol") {
		t.Errorf("excerpt should carry the primary [ERROR] line; got %q", got)
	}
	if !strings.Contains(got, "BUILD FAILURE") {
		t.Errorf("excerpt should carry the BUILD FAILURE verdict; got %q", got)
	}
	if strings.Contains(got, "Scanning for projects") {
		t.Errorf("excerpt should NOT include [INFO] scaffolding; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_GradleError(t *testing.T) {
	stdout := `> Task :compileJava FAILED

FAILURE: Build failed with an exception.

* Where:
Build file '/ws/build.gradle' line: 42

* What went wrong:
Execution failed for task ':compileJava'.
> Compilation failed; see the compiler error output for details.

* Try:
Run with --stacktrace option to get the stack trace.

BUILD FAILED in 3s
`
	got := narrativeBuildErrorExcerpt(stdout)
	if !strings.Contains(got, "FAILURE: Build failed with an exception") {
		t.Errorf("excerpt should carry Gradle FAILURE header; got %q", got)
	}
	if !strings.Contains(got, "What went wrong") {
		t.Errorf("excerpt should carry What-went-wrong marker; got %q", got)
	}
	if !strings.Contains(got, "BUILD FAILED") {
		t.Errorf("excerpt should carry BUILD FAILED; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_JavacError(t *testing.T) {
	stdout := `Foo.java:42: error: ';' expected
    return 1
           ^
Bar.java:9: error: cannot find symbol
1 error
`
	got := narrativeBuildErrorExcerpt(stdout)
	if !strings.Contains(got, "Foo.java:42: error: ';' expected") {
		t.Errorf("excerpt should pick up javac error lines; got %q", got)
	}
	if !strings.Contains(got, "Bar.java:9: error: cannot find symbol") {
		t.Errorf("excerpt should pick up second javac error; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_KotlinError(t *testing.T) {
	stdout := `> Task :compileKotlin
 e: /src/Foo.kt: (12, 9): Unresolved reference: baz
 e: /src/Foo.kt: (15, 1): Expecting '}'
BUILD FAILED in 1s
`
	got := narrativeBuildErrorExcerpt(stdout)
	if !strings.Contains(got, "Unresolved reference: baz") {
		t.Errorf("excerpt should pick up kotlinc 'e:' error; got %q", got)
	}
	if !strings.Contains(got, "BUILD FAILED") {
		t.Errorf("excerpt should carry BUILD FAILED; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_Dedup(t *testing.T) {
	stdout := `[ERROR] foo broke
[ERROR] foo broke
[ERROR] foo broke
[ERROR] bar also broke
`
	got := narrativeBuildErrorExcerpt(stdout)
	// Dedup: only one "foo broke" line in the excerpt.
	first := strings.Index(got, "foo broke")
	last := strings.LastIndex(got, "foo broke")
	if first == -1 || first != last {
		t.Errorf("duplicate 'foo broke' lines should collapse to one; got %q", got)
	}
	if !strings.Contains(got, "bar also broke") {
		t.Errorf("unique lines should still render; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_LineCap(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("[ERROR] unique row ")
		sb.WriteString(strings.Repeat("x", i+1))
		sb.WriteString("\n")
	}
	got := narrativeBuildErrorExcerpt(sb.String())
	n := strings.Count(got, "[ERROR]")
	if n != 10 {
		t.Errorf("should cap at 10 lines; got %d", n)
	}
}

func TestNarrativeBuildErrorExcerpt_Empty(t *testing.T) {
	if got := narrativeBuildErrorExcerpt(""); got != "" {
		t.Errorf("empty input should yield empty excerpt; got %q", got)
	}
	if got := narrativeBuildErrorExcerpt("   \n\n\t"); got != "" {
		t.Errorf("whitespace-only input should yield empty excerpt; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_MarkerlessFallback(t *testing.T) {
	// No known markers — should fall back to head lines so the
	// operator always has SOMETHING to read.
	stdout := `something unexpected happened
cannot proceed
giving up
`
	got := narrativeBuildErrorExcerpt(stdout)
	if got == "" {
		t.Fatal("marker-less input should still yield a non-empty excerpt")
	}
	if !strings.Contains(got, "something unexpected") {
		t.Errorf("fallback should surface first non-empty line; got %q", got)
	}
}

func TestNarrativeBuildErrorExcerpt_CharCap(t *testing.T) {
	// A single matching line wider than the cap: must truncate.
	huge := "[ERROR] " + strings.Repeat("y", 5000)
	got := narrativeBuildErrorExcerpt(huge)
	if len(got) > 1600 {
		t.Errorf("excerpt should stay bounded; got %d chars", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("oversized excerpt should carry a truncation marker; got %q", got)
	}
}

// ── parseMakeOutput ────────────────────────────────────────────────

// fakeExitError implements error; enough to simulate a command that
// exited non-zero without pulling in os/exec for the unit test.
type fakeExitError struct{ msg string }

func (e *fakeExitError) Error() string { return e.msg }

func TestParseMakeOutput_SuccessExit(t *testing.T) {
	report, err := parseMakeOutput("all 42 tests passed\n", nil)
	if err != nil {
		t.Fatalf("parseMakeOutput: %v", err)
	}
	if !report.Passed {
		t.Error("nil runErr should flip Passed=true")
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("want 1 synthetic result; got %d", len(report.TestResults))
	}
	if report.TestResults[0].AssertionID != "make-test" {
		t.Errorf("AssertionID = %q; want make-test", report.TestResults[0].AssertionID)
	}
	if !report.TestResults[0].Passed {
		t.Error("synthetic row should be Passed=true")
	}
	if report.FailureSummary != "" {
		t.Errorf("passing run should have empty FailureSummary; got %q", report.FailureSummary)
	}
}

func TestParseMakeOutput_FailureWithExcerpt(t *testing.T) {
	stdout := `cc -c foo.c
foo.c:42: error: undeclared identifier 'bar'
make: *** [foo.o] Error 1
`
	report, err := parseMakeOutput(stdout, &fakeExitError{msg: "exit status 2"})
	if err != nil {
		t.Fatalf("parseMakeOutput: %v", err)
	}
	if report.Passed {
		t.Error("non-nil runErr should flip Passed=false")
	}
	if !strings.Contains(report.TestResults[0].FailureDetail, "undeclared identifier") {
		t.Errorf("FailureDetail should include javac-style error; got %q", report.TestResults[0].FailureDetail)
	}
	if !strings.Contains(report.FailureSummary, "make target failed") {
		t.Errorf("FailureSummary should name 'make target failed'; got %q", report.FailureSummary)
	}
}

func TestParseMakeOutput_FailureWithoutExcerpt(t *testing.T) {
	// No marker-matching lines and a mostly empty stdout — parser
	// must fall back to runErr's own text so the retry hint has
	// something concrete.
	report, err := parseMakeOutput("", &fakeExitError{msg: "exit status 2"})
	if err != nil {
		t.Fatalf("parseMakeOutput: %v", err)
	}
	if report.Passed {
		t.Error("non-nil runErr should flip Passed=false")
	}
	if !strings.Contains(report.TestResults[0].FailureDetail, "exit status 2") {
		t.Errorf("FailureDetail should fall back to runErr text; got %q", report.TestResults[0].FailureDetail)
	}
}

// ── detectRunner additions ─────────────────────────────────────────

func TestDetectRunner_CMake(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("project(foo)\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := detectRunner(dir); got != "cmake" {
		t.Errorf("CMakeLists.txt should map to 'cmake'; got %q", got)
	}
}

func TestDetectRunner_Meson(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "meson.build"), []byte("project('foo','c')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := detectRunner(dir); got != "meson" {
		t.Errorf("meson.build should map to 'meson'; got %q", got)
	}
}

func TestDetectRunner_Makefile(t *testing.T) {
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, name), []byte("check:\n\ttrue\n"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			if got := detectRunner(dir); got != "make" {
				t.Errorf("%s should map to 'make'; got %q", name, got)
			}
		})
	}
}

func TestDetectRunner_CMakeBeatsMakefile(t *testing.T) {
	// A CMake project typically has a generated Makefile; CMake's
	// top-level manifest must win so we produce structured XML
	// instead of dropping to the Make fallback.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "Makefile"), []byte("check:\n"), 0o644)
	if got := detectRunner(dir); got != "cmake" {
		t.Errorf("CMakeLists.txt should win over Makefile; got %q", got)
	}
}

// ── detectNativeBuildDir ──────────────────────────────────────────

func TestDetectNativeBuildDir_CMakeBuild(t *testing.T) {
	repo := t.TempDir()
	build := filepath.Join(repo, "build")
	os.MkdirAll(build, 0o755)
	os.WriteFile(filepath.Join(build, "CMakeCache.txt"), []byte("#"), 0o644)
	if got := detectNativeBuildDir(repo); got != build {
		t.Errorf("expected build/ with CMakeCache.txt; got %q", got)
	}
}

func TestDetectNativeBuildDir_MesonBuilddir(t *testing.T) {
	repo := t.TempDir()
	build := filepath.Join(repo, "builddir")
	os.MkdirAll(filepath.Join(build, "meson-info"), 0o755)
	if got := detectNativeBuildDir(repo); got != build {
		t.Errorf("expected builddir/ with meson-info/; got %q", got)
	}
}

func TestDetectNativeBuildDir_EmptyDirIgnored(t *testing.T) {
	// A dir named 'build/' with no sentinel files (leftover from a
	// failed configure run) should NOT be accepted — we'd run ctest
	// against an empty dir and get confusing errors.
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, "build"), 0o755)
	if got := detectNativeBuildDir(repo); got != "" {
		t.Errorf("empty build/ should be rejected; got %q", got)
	}
}

func TestDetectNativeBuildDir_IDEPattern(t *testing.T) {
	// CLion's default is cmake-build-debug.
	repo := t.TempDir()
	d := filepath.Join(repo, "cmake-build-debug")
	os.MkdirAll(d, 0o755)
	os.WriteFile(filepath.Join(d, "CMakeCache.txt"), []byte("#"), 0o644)
	if got := detectNativeBuildDir(repo); got != d {
		t.Errorf("CLion-style dir should be accepted; got %q", got)
	}
}

// ── detectMakeTestTarget ──────────────────────────────────────────

func TestDetectMakeTestTarget_PrefersCheck(t *testing.T) {
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "Makefile"), []byte(`
all: foo

check:
	@echo running tests

test:
	@echo wrong target
`), 0o644)
	if got := detectMakeTestTarget(repo); got != "check" {
		t.Errorf("check should beat test; got %q", got)
	}
}

func TestDetectMakeTestTarget_FallsToTest(t *testing.T) {
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "Makefile"), []byte(`
all: foo

test:
	@echo running tests
`), 0o644)
	if got := detectMakeTestTarget(repo); got != "test" {
		t.Errorf("test should be selected when check absent; got %q", got)
	}
}

func TestDetectMakeTestTarget_DefaultsToCheck(t *testing.T) {
	// Neither check nor test target present — we still default to
	// `check` so a Makefile without a test suite surfaces a clean
	// "No rule to make target 'check'" from make itself.
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "Makefile"), []byte("all:\n\ttrue\n"), 0o644)
	if got := detectMakeTestTarget(repo); got != "check" {
		t.Errorf("missing test target should default to 'check'; got %q", got)
	}
}

func TestDetectMakeTestTarget_IgnoresIndentedLines(t *testing.T) {
	// A `check:` appearing as a recipe line (preceded by tab) is NOT
	// a target definition — must be skipped.
	repo := t.TempDir()
	os.WriteFile(filepath.Join(repo, "Makefile"), []byte(`
all: foo
	echo "check: this is a recipe, not a target"

test:
	@echo running tests
`), 0o644)
	if got := detectMakeTestTarget(repo); got != "test" {
		t.Errorf("indented 'check:' should be ignored; got %q", got)
	}
}
