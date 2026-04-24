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
		"TEST-com.foo.BarTest.xml": junitTwoSuitesMixed, // 3 cases
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
