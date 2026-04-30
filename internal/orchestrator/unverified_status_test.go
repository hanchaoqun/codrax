package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlanTouchesNonTestCode_NoIR_ProductionFile pins the
// fallback path: when WriteAnalysisIR is absent, plan.Changes
// inspection drives the decision. A change to a non-test source
// file means real production code was touched.
func TestPlanTouchesNonTestCode_NoIR_ProductionFile(t *testing.T) {
	mu := types.NewMutableState("test")
	mu.SetChangePlan(&types.ChangePlan{
		Changes: []types.FileChange{
			{Kind: "modify", Path: "internal/foo/foo.go"},
		},
	})
	bus := &types.BusContext{Mutable: mu}
	if !planTouchesNonTestCode(bus) {
		t.Error("plan modifying internal/foo/foo.go should be 'touches non-test code'")
	}
}

// TestPlanTouchesNonTestCode_NoIR_OnlyTests pins the no-test-code
// case: a plan that ONLY changes test files is benign.
func TestPlanTouchesNonTestCode_NoIR_OnlyTests(t *testing.T) {
	mu := types.NewMutableState("test")
	mu.SetChangePlan(&types.ChangePlan{
		Changes: []types.FileChange{
			{Kind: "create", Path: "internal/foo/foo_test.go"},
			{Kind: "create", Path: "tests/test_module.py"},
		},
	})
	bus := &types.BusContext{Mutable: mu}
	if planTouchesNonTestCode(bus) {
		t.Error("plan modifying only test files should NOT be 'touches non-test code'")
	}
}

// TestPlanTouchesNonTestCode_IRTaskKind pins the LLM-driven path:
// WriteAnalysisIR.Task.Kind == WriteTaskTest skips file inspection
// and returns false, regardless of what the file paths look like.
func TestPlanTouchesNonTestCode_IRTaskKind(t *testing.T) {
	cases := []types.WriteTaskKind{
		types.WriteTaskTest,
		types.WriteTaskDocs,
		types.WriteTaskConfig,
	}
	for _, kind := range cases {
		t.Run(string(kind), func(t *testing.T) {
			mu := types.NewMutableState("test")
			mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
				Request: types.WriteRequestModel{
					Task: types.WriteTask{Kind: kind},
				},
			})
			// Even with a production file in the plan, the IR's
			// declared kind takes precedence.
			mu.SetChangePlan(&types.ChangePlan{
				Changes: []types.FileChange{
					{Kind: "modify", Path: "main.go"},
				},
			})
			bus := &types.BusContext{Mutable: mu}
			if planTouchesNonTestCode(bus) {
				t.Errorf("Task.Kind=%s should not be 'touches non-test code'", kind)
			}
		})
	}
}

// TestPlanTouchesNonTestCode_IRTaskKind_Feature pins the
// "touches production" case explicitly: kind=feature/bugfix/
// refactor → file inspection runs and a non-test file means yes.
func TestPlanTouchesNonTestCode_IRTaskKind_Feature(t *testing.T) {
	for _, kind := range []types.WriteTaskKind{
		types.WriteTaskFeature, types.WriteTaskBugfix, types.WriteTaskRefactor,
	} {
		t.Run(string(kind), func(t *testing.T) {
			mu := types.NewMutableState("test")
			mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{
				Request: types.WriteRequestModel{
					Task: types.WriteTask{Kind: kind},
				},
			})
			mu.SetChangePlan(&types.ChangePlan{
				Changes: []types.FileChange{
					{Kind: "modify", Path: "internal/foo/foo.go"},
				},
			})
			bus := &types.BusContext{Mutable: mu}
			if !planTouchesNonTestCode(bus) {
				t.Errorf("Task.Kind=%s with production-file change should be 'touches non-test code'", kind)
			}
		})
	}
}

// TestIsTestPath_Conventions verifies the canonical filename
// patterns each test runner uses for discovery.
func TestIsTestPath_Conventions(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"foo_test.go", true},                           // Go
		{"main.go", false},                              // Go production
		{"test_module.py", true},                        // Python pytest
		{"module_test.py", true},                        // Python pytest
		{"src/main.py", false},                          // Python production
		{"tests/integration_test.py", true},             // Pytest dir
		{"foo.spec.ts", true},                           // Jest TS
		{"foo.test.tsx", true},                          // Jest TSX
		{"src/Foo.ts", false},                           // TS production
		{"FooTest.java", true},                          // JUnit
		{"FooTests.java", true},                         // JUnit alt
		{"FooSpec.kt", true},                            // Kotlin spec
		{"FooTest.kt", true},                            // Kotlin
		{"foo_spec.rb", true},                           // RSpec
		{"FooTests.swift", true},                        // SwiftPM
		{"src/main/java/Main.java", false},              // Java production
		{"spec/lib/foo_spec.rb", true},                  // RSpec dir
		{"__tests__/component.js", true},                // Jest dir convention
		{"docs/README.md", false},                       // doc
		{"internal/agent/explorer.go", false},           // Go production
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := isTestPath(tc.path); got != tc.want {
				t.Errorf("isTestPath(%q) = %v; want %v", tc.path, got, tc.want)
			}
		})
	}
}
