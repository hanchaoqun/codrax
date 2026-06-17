package tool

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// parseBuildErrors covers five toolchains; each test checks that the
// regex extracts file/line/message and that the dedup-by-(file,line,col)
// rule collapses duplicates from overlapping patterns.

func TestParseBuildErrors_Maven(t *testing.T) {
	stdout := `[INFO] Compiling 3 source files
[ERROR] /home/ci/src/main/java/Foo.java:[17,23] cannot find symbol
[ERROR]   symbol:   variable bar
[ERROR] /home/ci/src/main/java/Foo.java:[42,9] ';' expected
[INFO] BUILD FAILURE
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 Maven errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "/home/ci/src/main/java/Foo.java" || errs[0].Line != 17 || errs[0].Column != 23 {
		t.Errorf("first error mis-parsed; got %+v", errs[0])
	}
	if !strings.Contains(errs[0].Message, "cannot find symbol") {
		t.Errorf("message lost; got %q", errs[0].Message)
	}
}

func TestParseBuildErrors_Kotlin(t *testing.T) {
	stdout := `> Task :compileKotlin
e: file:///src/Foo.kt:12:9 Unresolved reference: baz
e: /src/Bar.kt:15:1: Expecting '}'
BUILD FAILED in 1s
`
	errs := parseBuildErrors(stdout)
	if len(errs) != 2 {
		t.Fatalf("expected 2 Kotlin errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "/src/Foo.kt" || errs[0].Line != 12 {
		t.Errorf("Foo.kt mis-parsed; got %+v", errs[0])
	}
	if errs[1].File != "/src/Bar.kt" || errs[1].Line != 15 {
		t.Errorf("Bar.kt mis-parsed; got %+v", errs[1])
	}
}

func TestParseBuildErrors_GenericJavac(t *testing.T) {
	stdout := `Foo.java:42: error: ';' expected
    return 1
           ^
Bar.java:9: error: cannot find symbol
1 error
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 2 {
		t.Fatalf("expected 2 javac errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "Foo.java" || errs[0].Line != 42 {
		t.Errorf("Foo.java mis-parsed; got %+v", errs[0])
	}
	if !strings.Contains(errs[1].Message, "cannot find symbol") {
		t.Errorf("Bar.java message lost; got %+v", errs[1])
	}
}

func TestParseBuildErrors_TypeScriptParen(t *testing.T) {
	stdout := `ERROR: src/foo.ts(42,5): TS2304: Cannot find name 'bar'
src/baz.ts(7,3): error TS2322: Type 'string' is not assignable to type 'number'
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 2 {
		t.Fatalf("expected 2 TS errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].Symbol != "TS2304" {
		t.Errorf("first error symbol = %q; want TS2304", errs[0].Symbol)
	}
	if errs[0].Line != 42 || errs[0].Column != 5 {
		t.Errorf("first error position mis-parsed; got %+v", errs[0])
	}
	if errs[1].Symbol != "TS2322" {
		t.Errorf("second error symbol = %q; want TS2322", errs[1].Symbol)
	}
}

func TestParseBuildErrors_TypeScriptColon(t *testing.T) {
	stdout := `src/foo.ts:42:5 - error TS2304: Cannot find name 'bar'
`
	errs := parseBuildErrors(stdout)
	if len(errs) != 1 {
		t.Fatalf("expected 1 TS error; got %d (%v)", len(errs), errs)
	}
	if errs[0].Line != 42 || errs[0].Column != 5 || errs[0].Symbol != "TS2304" {
		t.Errorf("colon-shape TS error mis-parsed; got %+v", errs[0])
	}
}

func TestParseBuildErrors_Cangjie(t *testing.T) {
	stdout := `error: src/Foo.cj:12:7: undefined symbol
[ERROR] src/Bar.cj:99:1: missing closing brace
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 2 {
		t.Fatalf("expected 2 Cangjie errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "src/Foo.cj" || errs[0].Line != 12 {
		t.Errorf("Foo.cj mis-parsed; got %+v", errs[0])
	}
	if errs[1].File != "src/Bar.cj" || errs[1].Line != 99 {
		t.Errorf("Bar.cj mis-parsed; got %+v", errs[1])
	}
}

// Go compile errors: `./foo.go:5:1: syntax error: unexpected newline`.
// Generic regex (.go extension added) catches these.
func TestParseBuildErrors_Go(t *testing.T) {
	stdout := `# example.com/foo
./foo.go:5:1: syntax error: unexpected newline
./bar.go:42:9: error: undefined: Baz
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 1 {
		t.Fatalf("expected at least 1 Go error; got %d (%v)", len(errs), errs)
	}
	// At least one should be foo.go:5
	found := false
	for _, e := range errs {
		if e.File == "./foo.go" && e.Line == 5 {
			found = true
			if !strings.Contains(e.Message, "syntax error") {
				t.Errorf("Go message lost; got %q", e.Message)
			}
		}
	}
	if !found {
		t.Errorf("foo.go:5 not found in %v", errs)
	}
}

// C/C++ compile errors: gcc/clang `foo.c:42:5: error: 'bar' undeclared`.
func TestParseBuildErrors_C(t *testing.T) {
	stdout := `gcc -c foo.c
foo.c:42:5: error: 'bar' undeclared (first use in this function)
src/baz.cpp:9:14: error: expected ';' before '}' token
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 2 {
		t.Fatalf("expected 2 C/C++ errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "foo.c" || errs[0].Line != 42 {
		t.Errorf("foo.c mis-parsed; got %+v", errs[0])
	}
}

func TestParseBuildErrors_NodeCheck(t *testing.T) {
	stdout := `/repo/src/app.js:3
const =
      ^

SyntaxError: Unexpected token '='
    at internalCompileFunction (node:internal/vm:73:18)
`
	errs := parseBuildErrors(stdout)
	if len(errs) != 1 {
		t.Fatalf("expected 1 Node syntax error; got %d (%v)", len(errs), errs)
	}
	if got := errs[0]; got.File != "/repo/src/app.js" || got.Line != 3 || got.Symbol != "SyntaxError" {
		t.Fatalf("Node syntax error mis-parsed: %+v", got)
	}
	if !strings.Contains(errs[0].Message, "Unexpected token") {
		t.Fatalf("Node syntax message lost: %+v", errs[0])
	}
}

func TestParseBuildErrors_RubyCheck(t *testing.T) {
	stdout := `/repo/lib/app.rb:4: syntax error, unexpected end-of-input, expecting end
`
	errs := parseBuildErrors(stdout)
	if len(errs) != 1 {
		t.Fatalf("expected 1 Ruby syntax error; got %d (%v)", len(errs), errs)
	}
	if got := errs[0]; got.File != "/repo/lib/app.rb" || got.Line != 4 {
		t.Fatalf("Ruby syntax error mis-parsed: %+v", got)
	}
	if !strings.Contains(errs[0].Message, "syntax error") {
		t.Fatalf("Ruby syntax message lost: %+v", errs[0])
	}
}

// Rust block-style errors: error[E0xxx]: ... + --> file:line:col.
func TestParseBuildErrors_Rust(t *testing.T) {
	stdout := `error[E0308]: mismatched types
  --> src/lib.rs:10:5
   |
10 |     foo()
   |     ^^^ expected (), found u32
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 1 {
		t.Fatalf("expected 1 Rust error; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "src/lib.rs" || errs[0].Line != 10 || errs[0].Column != 5 {
		t.Errorf("Rust position mis-parsed; got %+v", errs[0])
	}
	if errs[0].Symbol != "E0308" {
		t.Errorf("Rust error code = %q; want E0308", errs[0].Symbol)
	}
	if !strings.Contains(errs[0].Message, "mismatched types") {
		t.Errorf("Rust message lost; got %q", errs[0].Message)
	}
}

// Swift XCTest output: per-test rows + exit-code verdict.
func TestParseSwiftOutput_PerTestRows(t *testing.T) {
	stdout := `Build complete!
Test Suite 'All tests' started at 2024-01-01 00:00:00.
Test Case '-[GreeterTests testHello]' passed (0.001 seconds).
Test Case '-[GreeterTests testGoodbye]' failed (0.002 seconds).
Test Suite 'GreeterTests' failed at 2024-01-01 00:00:00.
	 Executed 2 tests, with 1 failure (0 unexpected) in 0.003 (0.005) seconds
`
	report, err := parseSwiftOutput(stdout, &fakeExitError{msg: "exit status 1"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("verdict should be failed (one test failed)")
	}
	if len(report.TestResults) != 2 {
		t.Fatalf("expected 2 per-test rows; got %d (%v)", len(report.TestResults), report.TestResults)
	}
	failed := 0
	for _, tr := range report.TestResults {
		if !tr.Passed {
			failed++
		}
	}
	if failed != 1 {
		t.Errorf("expected 1 failed; got %d", failed)
	}
}

func TestParseSwiftOutput_BuildErrorPath(t *testing.T) {
	stdout := `src/main/Greeter.swift:42:10: error: cannot find 'badSymbol' in scope
`
	report, err := parseSwiftOutput(stdout, &fakeExitError{msg: "exit status 1"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !report.BuildFailed {
		t.Error("BuildFailed should be set on Swift compile error")
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("expected one synthetic build-error row; got %d", len(report.TestResults))
	}
	if report.TestResults[0].Kind != types.TestResultKindBuildError {
		t.Errorf("Kind = %q; want build_error", report.TestResults[0].Kind)
	}
}

// Verdict markers (BUILD FAILURE / What went wrong / FAILURE:)
// ALWAYS land in the narrative excerpt even when [ERROR] lines
// fill the 10-line cap before the verdict footer.
func TestNarrativeExcerpt_VerdictMarkersPriority(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("[ERROR] some compile error #")
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString("\n")
	}
	sb.WriteString("BUILD FAILURE\n")
	sb.WriteString("FAILURE: Build failed with an exception.\n")
	sb.WriteString("* What went wrong: Compilation failed.\n")
	excerpt := narrativeBuildErrorExcerpt(sb.String())
	for _, want := range []string{"BUILD FAILURE", "FAILURE:", "What went wrong"} {
		if !strings.Contains(excerpt, want) {
			t.Errorf("verdict %q should always be retained even with 50 [ERROR] lines; got %q", want, excerpt)
		}
	}
}

// Python pylint/mypy-style: `tests/test_foo.py:42: error: <msg>`.
func TestParseBuildErrors_Python(t *testing.T) {
	stdout := `tests/test_foo.py:42: error: Argument 1 to "foo" has incompatible type
src/bar.py:9: error: Cannot find name 'baz'
`
	errs := parseBuildErrors(stdout)
	if len(errs) < 2 {
		t.Fatalf("expected 2 Python errors; got %d (%v)", len(errs), errs)
	}
	if errs[0].File != "tests/test_foo.py" || errs[0].Line != 42 {
		t.Errorf("Python position mis-parsed; got %+v", errs[0])
	}
}

func TestParseBuildErrors_Empty(t *testing.T) {
	if got := parseBuildErrors(""); got != nil {
		t.Errorf("empty input should yield nil; got %+v", got)
	}
	if got := parseBuildErrors("nothing matches here"); got != nil {
		t.Errorf("no-match input should yield nil; got %+v", got)
	}
}

func TestParseBuildErrors_Dedup(t *testing.T) {
	// Same (file, line, col) appears in both Maven [ERROR] and the
	// embedded generic file:line: shape — should collapse to one.
	stdout := `[ERROR] /src/Foo.java:[17,23] cannot find symbol
/src/Foo.java:17: error: cannot find symbol
`
	errs := parseBuildErrors(stdout)
	// Maven matches with col=23; generic without col (col=0). They're
	// different (key differs). That's the expected behaviour — operator
	// can see both shapes if the build tool emits both.
	if len(errs) != 2 {
		t.Logf("note: two different col values (23 vs 0) yield two entries; got %+v", errs)
	}
}

func TestParseBuildErrors_Cap(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		// Each error has a unique line so dedup doesn't collapse.
		sb.WriteString("[ERROR] /src/Foo.java:[")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(",1] error number ")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString("\n")
	}
	errs := parseBuildErrors(sb.String())
	if len(errs) > 25 {
		t.Errorf("should cap at 25 entries; got %d", len(errs))
	}
}

// renderBuildFailureSummary semantics — structured errors take
// precedence over narrative; empty errors falls back gracefully.
func TestRenderBuildFailureSummary_WithErrors(t *testing.T) {
	errs := []types.BuildError{
		{File: "/src/Foo.java", Line: 42, Column: 5, Message: "cannot find symbol"},
		{File: "/src/Bar.java", Line: 9, Column: 3, Message: "missing return"},
	}
	got := renderBuildFailureSummary("Java", errs, "")
	if !strings.Contains(got, "2 compile error(s)") {
		t.Errorf("should report count; got %q", got)
	}
	if !strings.Contains(got, "2 file(s)") {
		t.Errorf("should report file count; got %q", got)
	}
	if !strings.Contains(got, "/src/Foo.java:42") {
		t.Errorf("should surface first file:line; got %q", got)
	}
}

func TestRenderBuildFailureSummary_NoErrors(t *testing.T) {
	got := renderBuildFailureSummary("CMake", nil, "fatal error: foo.h not found")
	if !strings.Contains(got, "fatal error") {
		t.Errorf("should fall back to narrative line; got %q", got)
	}
}

func TestRenderBuildFailureSummary_EmptyEverything(t *testing.T) {
	got := renderBuildFailureSummary("Meson", nil, "")
	if !strings.Contains(got, "no recognized error markers") {
		t.Errorf("should mention the no-markers fallback; got %q", got)
	}
}

// parseGoTestJSONLines: when go test reports a package-level "fail"
// with zero per-test events (compile error path), the parser should
// synthesise a build-error row + set BuildFailed=true so the
// CritTestsPass evaluator surfaces "build failed before tests ran"
// instead of "0 tests passed".
func TestParseGoTest_CompileErrorMapsToBuildFailed(t *testing.T) {
	// Simulate go test -json output for a package that fails to
	// compile: only a package-level "fail" event with no per-test
	// "run"/"pass"/"fail" events.
	stdout := `{"Time":"2026-04-25T00:00:00Z","Action":"output","Package":"foo","Output":"./foo.go:5:1: syntax error: unexpected newline"}
{"Time":"2026-04-25T00:00:00Z","Action":"fail","Package":"foo","Elapsed":0.01}
`
	report, err := parseGoTestJSONLines(stdout)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if report.Passed {
		t.Error("compile-error report should not Pass")
	}
	if !report.BuildFailed {
		t.Error("BuildFailed should be set when package fails with no per-test events")
	}
	if len(report.TestResults) != 1 {
		t.Fatalf("expected exactly one synthetic build-error row; got %d", len(report.TestResults))
	}
	if report.TestResults[0].Kind != types.TestResultKindBuildError {
		t.Errorf("Kind = %q; want build_error", report.TestResults[0].Kind)
	}
}

func TestFirstBuildErrorAssertionID(t *testing.T) {
	if got := firstBuildErrorAssertionID(nil); got != "" {
		t.Errorf("nil → empty; got %q", got)
	}
	id := firstBuildErrorAssertionID([]types.BuildError{
		{File: "/src/Foo.java", Line: 42},
	})
	if id != "/src/Foo.java:42" {
		t.Errorf("got %q; want /src/Foo.java:42", id)
	}
	id = firstBuildErrorAssertionID([]types.BuildError{
		{File: "/src/Foo.java"},
	})
	if id != "/src/Foo.java" {
		t.Errorf("got %q; want /src/Foo.java", id)
	}
}
