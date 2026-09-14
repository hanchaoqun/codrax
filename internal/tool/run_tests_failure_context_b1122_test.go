package tool

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestB1122MakeFailureRetainsShortOutputAndBoundsLongOutputByPosition(t *testing.T) {
	short := "command prelude\n  slot=leaf; observed=41; expected=42\n"
	report, err := parseMakeOutput("check", short, errors.New("exit status 2"))
	if err != nil || report.TestResults[0].FailureDetail != short {
		t.Fatalf("short raw output changed: report=%+v err=%v", report, err)
	}
	if report.FailureSummary != "make target failed: check" || report.BuildFailed || report.Passed {
		t.Fatalf("a command prelude was promoted to a diagnosis or build verdict: %+v", report)
	}
	long := "SOURCE-HEAD\n" + strings.Repeat("多语言进度行\n", 5000) + "SOURCE-TAIL: observed=41 expected=42\n"
	report, err = parseMakeOutput("check", long, errors.New("exit status 2"))
	if err != nil {
		t.Fatal(err)
	}
	detail := report.TestResults[0].FailureDetail
	if len(detail) > 8*1024 || !utf8.ValidString(detail) || !strings.HasPrefix(detail, "SOURCE-HEAD\n") || !strings.HasSuffix(detail, "SOURCE-TAIL: observed=41 expected=42\n") || !strings.Contains(detail, "runner output excerpt: omitted ") {
		t.Fatalf("long raw output was lost or silently truncated: bytes=%d detail=%q", len(detail), detail)
	}
}

func TestB1122MakeKeepsShortUnittestBytesAndEstablishedLongFailureBlock(t *testing.T) {
	block := strings.Join([]string{
		"======================================================================",
		"FAIL: test_value (tests.test_value.ValueTest)",
		"----------------------------------------------------------------------",
		"Traceback (most recent call last):",
		`  File "/repo/tests/test_value.py", line 7, in test_value`,
		"    self.assertEqual(value, 42)",
		"AssertionError: 41 != 42 : slot=leaf; observed=41; expected=42",
		"",
		"----------------------------------------------------------------------",
		"Ran 1 test in 0.001s",
		"",
		"FAILED (failures=1)",
	}, "\n")
	for _, long := range []bool{false, true} {
		output := "python3 -m unittest discover\n" + block + "\nmake: *** [check] Error 1\n"
		if long {
			output = strings.Repeat("progress before failure\n", 600) + output + strings.Repeat("progress after failure\n", 600)
		}
		report, err := parseMakeOutput("check", output, errors.New("exit status 2"))
		if err != nil || report.Passed || report.BuildFailed {
			t.Fatalf("long=%t: changed native verdict: report=%+v err=%v", long, report, err)
		}
		detail := report.TestResults[0].FailureDetail
		if !long && detail != output {
			t.Fatal("short embedded unittest output was not preserved byte-for-byte")
		}
		if long && detail != unittestAggregateFailureDetail(output) {
			t.Fatal("long output replaced the established unittest failure-block view")
		}
		for _, want := range []string{`File "/repo/tests/test_value.py", line 7`, "slot=leaf; observed=41; expected=42"} {
			if !strings.Contains(detail, want) {
				t.Fatalf("long=%t: lost established failure identity/detail %q", long, want)
			}
		}
	}
}
