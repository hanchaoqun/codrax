package orchestrator

import (
	"strings"
	"testing"
)

// TestExtractFailureSignal_PytestFormat covers the original Batch E
// regression: pytest's first line is `self = <Test fixture>`, hiding
// the actual `E AssertionError: ...` line. The new extractor must
// surface the E-marked + > -marked lines together.
func TestExtractFailureSignal_PytestFormat(t *testing.T) {
	detail := `self = <robot_name_test.RobotNameTest testMethod=test_reset_name>

    def test_reset_name(self):
        seed = "Totally random."
        random.seed(seed)
        robot = Robot()
        name = robot.name
        random.seed(seed)
        robot.reset()
        name2 = robot.name
>       self.assertNotEqual(name, name2)
E       AssertionError: 'FV566' == 'FV566'

robot_name_test.py:46: AssertionError`
	got := ExtractFailureSignal(detail, 600)
	if !strings.Contains(got, "self.assertNotEqual(name, name2)") {
		t.Errorf("must surface the > marker line; got %q", got)
	}
	if !strings.Contains(got, "AssertionError: 'FV566' == 'FV566'") {
		t.Errorf("must surface the E marker line with actual values; got %q", got)
	}
	if strings.Contains(got, "self = <robot_name_test") {
		t.Errorf("must NOT surface the fixture self= header; got %q", got)
	}
}

// TestExtractFailureSignal_GoTestFormat: go test output uses
// "--- FAIL:" header + tab-indented assertion message.
func TestExtractFailureSignal_GoTestFormat(t *testing.T) {
	detail := `=== RUN   TestParseTrinary
--- FAIL: TestParseTrinary (0.00s)
    trinary_test.go:27: ParseTrinary("0000201") returned error "overflow", Error not expected
    trinary_test.go:27: ParseTrinary("2021110") returned error "overflow", Error not expected
FAIL`
	got := ExtractFailureSignal(detail, 600)
	if !strings.Contains(got, "--- FAIL: TestParseTrinary") {
		t.Errorf("must surface the FAIL header; got %q", got)
	}
	if !strings.Contains(got, "trinary_test.go:27: ParseTrinary") {
		t.Errorf("must surface the indented assertion message; got %q", got)
	}
}

// TestExtractFailureSignal_PanicFormat: Go panic, Java
// Exception, Python Traceback all matched by Priority 3.
func TestExtractFailureSignal_PanicFormat(t *testing.T) {
	detail := `goroutine 17 [running]:
testing.tRunner.func1.2({0x4d8b40, 0x500378})
        /usr/local/go/src/testing/testing.go:1350
panic: runtime error: index out of range [3] with length 2 [recovered]
        panic({0x4d8b40, 0x500378})
        /usr/local/go/src/runtime/panic.go:884 +0x213
main.process(...)
        /tmp/trinary/main.go:42 +0x123`
	got := ExtractFailureSignal(detail, 600)
	if !strings.Contains(got, "panic: runtime error: index out of range") {
		t.Errorf("must surface panic line; got %q", got)
	}
	if !strings.Contains(got, "main.process") {
		t.Errorf("must surface the next stack frame; got %q", got)
	}
}

// TestExtractFailureSignal_FallbackKeepsFirstLine: when no marker
// matches AND the first line isn't a known fixture-noise pattern,
// the extractor returns everything verbatim. Earlier the code
// stripped the first line unconditionally and lost real errors on
// non-stdlib runners.
func TestExtractFailureSignal_FallbackKeepsFirstLine(t *testing.T) {
	detail := "expected 42, got 7\nmore stack info"
	got := ExtractFailureSignal(detail, 600)
	if !strings.Contains(got, "expected 42, got 7") {
		t.Errorf("fallback must keep first line when not fixture-noise; got %q", got)
	}
}

// TestExtractFailureSignal_FixtureNoiseStripped: when the first line
// matches a known fixture noise pattern (`self = `, `=== RUN`) and
// no error markers were detected, the extractor strips ONLY that
// first line.
func TestExtractFailureSignal_FixtureNoiseStripped(t *testing.T) {
	detail := "self = <Foo bar baz>\nactual error here\nstack frame"
	got := ExtractFailureSignal(detail, 600)
	if strings.Contains(got, "self = <Foo bar baz>") {
		t.Errorf("fixture noise first line should be stripped; got %q", got)
	}
	if !strings.Contains(got, "actual error here") {
		t.Errorf("must keep subsequent error line; got %q", got)
	}
}

// TestExtractFailureSignal_EmptyInput: an empty FailureDetail
// returns "" so callers can skip the row cleanly.
func TestExtractFailureSignal_EmptyInput(t *testing.T) {
	if got := ExtractFailureSignal("", 600); got != "" {
		t.Errorf("empty input should return empty; got %q", got)
	}
	if got := ExtractFailureSignal("   \n  \n", 600); got != "" {
		t.Errorf("whitespace-only input should return empty; got %q", got)
	}
}

// TestExtractFailureSignal_CapEnforced: cap is honored even when
// the extracted signal is longer. Truncation marker present.
func TestExtractFailureSignal_CapEnforced(t *testing.T) {
	detail := strings.Repeat("E       this is an error line that we keep adding\n", 50)
	got := ExtractFailureSignal(detail, 200)
	if len(got) > 220 { // cap + ellipsis padding
		t.Errorf("cap not honored: len=%d", len(got))
	}
	if !strings.Contains(got, "…") {
		t.Errorf("truncation marker missing; got %q", got)
	}
}
