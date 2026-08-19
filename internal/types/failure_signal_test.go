package types

import (
	"strings"
	"testing"
)

func TestExtractTestFailureSignal_PytestDiffCarriesCompactDelta(t *testing.T) {
	detail := `self = <xarray.tests.test_formatting.TestFormatting object at 0x133e7f250>

    def test_diff_array_repr(self):
        actual = formatting.diff_array_repr(da_a, da_b, "identical")
        try:
>           assert actual == expected
E           AssertionError: assert 'Left and rig...ription: desc' == 'Left and rig...ription: desc'
E             Skipping 153 identical leading characters in diff, use -v to show
E             - [4, 5, 6]], dtype=int64)
E             + [4, 5, 6]])

xarray/tests/test_formatting.py:261: AssertionError`

	got := ExtractTestFailureSignal(TestResult{
		AssertionID:   "test_diff_array_repr",
		Suite:         "xarray/tests/test_formatting.py::TestFormatting",
		Passed:        false,
		FailureDetail: detail,
	}, 900)

	if got.AssertionID != "test_diff_array_repr" {
		t.Fatalf("AssertionID = %q", got.AssertionID)
	}
	if got.Location != "xarray/tests/test_formatting.py:261" {
		t.Fatalf("Location = %q", got.Location)
	}
	for _, want := range []string{
		"assert actual == expected",
		"AssertionError",
		"dtype=int64",
	} {
		if !strings.Contains(got.Signal, want) {
			t.Fatalf("Signal missing %q:\n%s", want, got.Signal)
		}
	}
	rendered := RenderTestFailureSignal(got)
	if !strings.Contains(rendered, "location=xarray/tests/test_formatting.py:261") ||
		!strings.Contains(rendered, "signal=") {
		t.Fatalf("rendered signal missing compact fields: %q", rendered)
	}
}

func TestExtractTestFailureSignal_ExpectedGotPair(t *testing.T) {
	got := ExtractTestFailureSignal(TestResult{
		AssertionID:   "TestValue",
		Suite:         "pkg",
		Passed:        false,
		FailureDetail: "widget_test.go:12: expected 42, got 7",
	}, 200)

	if got.Location != "widget_test.go:12" {
		t.Fatalf("Location = %q", got.Location)
	}
	if got.Expected != "42" || got.Actual != "7" {
		t.Fatalf("expected/actual = %q/%q", got.Expected, got.Actual)
	}
	if got.LeftOperand != "" || got.RightOperand != "" || got.OperandRoles != "" {
		t.Fatalf("explicit expected/got must not also mint unlabeled operands: %+v", got)
	}
}

func TestExtractTestFailureSignal_PythonTracebackLocation(t *testing.T) {
	got := ExtractTestFailureSignal(TestResult{
		AssertionID: "test_iterable_boundfield_select",
		Suite:       "forms_tests.tests.test_forms.FormsTestCase.test_iterable_boundfield_select",
		Passed:      false,
		FailureDetail: `Traceback (most recent call last):
  File "/tmp/repo/tests/forms_tests/tests/test_forms.py", line 723, in test_iterable_boundfield_select
    self.assertEqual(fields[0].id_for_label, 'id_name_0')
AssertionError: None != 'id_name_0'`,
	}, 400)

	if got.Location != "/tmp/repo/tests/forms_tests/tests/test_forms.py:723" {
		t.Fatalf("Location = %q", got.Location)
	}
	if got.Expected != "" || got.Actual != "" {
		t.Fatalf("bare Python comparison has no expected/actual role authority: %+v", got)
	}
	if got.ComparisonOperator != "!=" || got.LeftOperand != "None" || got.RightOperand != "id_name_0" ||
		got.OperandRoles != TestFailureOperandRolesUnlabeled {
		t.Fatalf("unlabeled comparison projection wrong: %+v", got)
	}
	rendered := RenderTestFailureSignal(got)
	for _, want := range []string{
		`comparison_operator="!="`,
		`left_operand="None"`,
		`right_operand="id_name_0"`,
		`operand_roles=unlabeled`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered unlabeled comparison missing %q: %s", want, rendered)
		}
	}
	if strings.Contains(rendered, "expected=") || strings.Contains(rendered, "actual=") {
		t.Fatalf("unlabeled comparison was mislabeled in rendered guidance: %s", rendered)
	}
}

func TestExtractTestFailureSignal_UnittestListsDifferPreservesOrderedOperandsWithoutRoles(t *testing.T) {
	detail := `AssertionError: Lists differ: [300, 300, 10] != [300]`
	got := ExtractTestFailureSignal(TestResult{
		AssertionID:   "test_newline_run",
		Suite:         "tests/test_tokenizer.py",
		FailureDetail: detail,
	}, 400)

	if got.Expected != "" || got.Actual != "" {
		t.Fatalf("unittest operand order must not be relabeled expected/actual: %+v", got)
	}
	if got.ComparisonOperator != "!=" || got.LeftOperand != "Lists differ: [300, 300, 10]" ||
		got.RightOperand != "[300]" || got.OperandRoles != TestFailureOperandRolesUnlabeled {
		t.Fatalf("ordered operands lost: %+v", got)
	}
}
