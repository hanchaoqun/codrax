package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// emitTestResultsFixture installs a minimal ChangeReport on Mutable so
// emit_test_results.Execute has a target to annotate. Returns the
// BusContext for the test to mutate.
func emitTestResultsFixture(report *types.ChangeReport) *types.BusContext {
	mu := types.NewMutableState("test")
	if report != nil {
		mu.SetChangeReport(report)
	}
	return &types.BusContext{Mutable: mu}
}

// Required-key contract: missing any of the three classification
// arrays MUST be rejected, even when the JSON is otherwise valid.
// The schema's "required" hint isn't always enforced upstream, so
// Execute does its own check.
func TestEmitTestResults_MissingArrayRejected(t *testing.T) {
	ctx := emitTestResultsFixture(&types.ChangeReport{
		Passed: true,
	})
	tool := &EmitTestResults{}

	cases := []struct {
		name   string
		params string
	}{
		{
			"missing_regression",
			`{"preexisting_assertions": [], "fixed_assertions": []}`,
		},
		{
			"missing_preexisting",
			`{"regression_assertions": [], "fixed_assertions": []}`,
		},
		{
			"missing_fixed",
			`{"regression_assertions": [], "preexisting_assertions": []}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := tool.Execute(ctx, json.RawMessage(c.params))
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if res.Success {
				t.Errorf("missing key %s should not Success", c.name)
			}
			if !strings.Contains(res.Summary, "required key") {
				t.Errorf("Summary should explain the rejection; got %q", res.Summary)
			}
		})
	}
}

// Empty arrays for all three keys are valid when present (e.g. all
// tests passed; nothing to classify).
func TestEmitTestResults_EmptyArraysValid(t *testing.T) {
	ctx := emitTestResultsFixture(&types.ChangeReport{Passed: true})
	tool := &EmitTestResults{}
	params := `{"regression_assertions": [], "preexisting_assertions": [], "fixed_assertions": [], "failure_summary": "all good"}`
	res, err := tool.Execute(ctx, json.RawMessage(params))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Errorf("empty arrays should be accepted; got %q", res.Summary)
	}
	report := ctx.Mutable.ChangeReport()
	if len(report.RegressionAssertions) != 0 {
		t.Errorf("RegressionAssertions should be empty/nil; got %v", report.RegressionAssertions)
	}
}

// Populated arrays write through to ChangeReport. Dedup of duplicates
// + filter of empty strings happens server-side so downstream
// evaluators don't have to defend.
func TestEmitTestResults_StructuredClassification(t *testing.T) {
	ctx := emitTestResultsFixture(&types.ChangeReport{
		Passed: false,
		TestResults: []types.TestResult{
			{Kind: types.TestResultKindUnit, AssertionID: "TestA", Passed: false},
			{Kind: types.TestResultKindUnit, AssertionID: "TestB", Passed: false},
			{Kind: types.TestResultKindUnit, AssertionID: "TestC", Passed: true},
		},
	})
	tool := &EmitTestResults{}
	params := `{
		"regression_assertions": ["TestA", "TestA", ""],
		"preexisting_assertions": ["TestB"],
		"fixed_assertions": ["TestZ"],
		"failure_summary": "TestA is the regression; TestB was already broken."
	}`
	res, err := tool.Execute(ctx, json.RawMessage(params))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Errorf("classification call should succeed; got %q", res.Summary)
	}
	report := ctx.Mutable.ChangeReport()
	if len(report.RegressionAssertions) != 1 || report.RegressionAssertions[0] != "TestA" {
		t.Errorf("RegressionAssertions should dedup + filter empty; got %v", report.RegressionAssertions)
	}
	if len(report.PreexistingAssertions) != 1 || report.PreexistingAssertions[0] != "TestB" {
		t.Errorf("PreexistingAssertions = %v; want [TestB]", report.PreexistingAssertions)
	}
	if len(report.FixedAssertions) != 1 || report.FixedAssertions[0] != "TestZ" {
		t.Errorf("FixedAssertions = %v; want [TestZ]", report.FixedAssertions)
	}
	if !strings.Contains(report.FailureSummary, "TestA is the regression") {
		t.Errorf("narrative should land in FailureSummary; got %q", report.FailureSummary)
	}
}

// Schema declares required keys — validate via the JSON schema
// itself, not just runtime. Catches drift if the schema and the
// runtime check go out of sync.
func TestEmitTestResults_SchemaDeclaresRequired(t *testing.T) {
	tool := &EmitTestResults{}
	schema := tool.Parameters()
	var s map[string]any
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	req, ok := s["required"].([]any)
	if !ok {
		t.Fatal("schema missing required array")
	}
	want := map[string]bool{
		"regression_assertions":  true,
		"preexisting_assertions": true,
		"fixed_assertions":       true,
	}
	for _, k := range req {
		ks, _ := k.(string)
		delete(want, ks)
	}
	if len(want) > 0 {
		t.Errorf("schema required missing keys: %v", want)
	}
}

// No ChangeReport on Mutable → reject. run_tests must run first.
func TestEmitTestResults_RejectsWithoutReport(t *testing.T) {
	ctx := emitTestResultsFixture(nil)
	tool := &EmitTestResults{}
	params := `{"regression_assertions": [], "preexisting_assertions": [], "fixed_assertions": []}`
	res, err := tool.Execute(ctx, json.RawMessage(params))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success {
		t.Error("missing ChangeReport should not Success")
	}
}
