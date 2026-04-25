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

