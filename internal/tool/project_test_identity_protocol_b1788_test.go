package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1788ActualPlanSchemasTeachNativeResultIdentity(t *testing.T) {
	var first map[string]string
	for name, raw := range map[string]json.RawMessage{
		"full": (&EmitChangePlan{}).Parameters(), "skeleton": (&EmitPlanSkeleton{}).Parameters(),
	} {
		t.Run(name, func(t *testing.T) {
			var schema struct {
				Properties map[string]struct {
					Description string `json:"description"`
					Items       struct {
						Properties map[string]struct {
							Description string `json:"description"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"properties"`
			}
			if err := json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			pto := schema.Properties["project_test_observations"]
			got := map[string]string{"binding": pto.Description, "suite": pto.Items.Properties["assertion_suite"].Description, "id": pto.Items.Properties["assertion_id"].Description}
			if !strings.Contains(got["binding"], types.NativeProjectTestObservationBindingTeaching) || got["suite"] != types.NativeProjectTestObservationSuiteTeaching || got["id"] != types.NativeProjectTestObservationAssertionIDTeaching {
				t.Error("actual schema must consume the shared teaching verbatim")
			}
			for field, required := range map[string][]string{
				"binding": {"first plan", "does not require running tests", "both", "runner[/framework]@working_dir::", "unchanged test file"},
				"suite":   {"TestResult.suite", "Go", "Package", "import path", "unittest", "pytest", "Jest", "Cargo", "JUnit", "RSpec", "Swift"},
				"id":      {"TestResult.assertion_id", "Go", "unittest", "pytest", "Jest", "joined by \" > \" (spaces included)", "Cargo", "JUnit", "RSpec", "Swift", "not source assertion code"},
			} {
				for _, text := range required {
					if !strings.Contains(got[field], text) {
						t.Errorf("ACTUAL_SCHEMA_IDENTITY: %s %s lacks %q", name, field, text)
					}
				}
			}
			if first == nil {
				first = got
			} else if !reflect.DeepEqual(first, got) {
				t.Fatal("complete/skeleton transport changed the shared identity teaching")
			}
		})
	}
}

// Protocol fixtures go through the real parser dispatcher and the existing
// report qualifier. These are not claims that eight runtime environments ran.
func TestB1788NativeResultIdentityProtocolsAndProjectScope(t *testing.T) {
	for _, tc := range []struct {
		name, runner, framework, stdout, artifact, suffix, suite, id string
	}{
		{"go", "go", "", "{\"Action\":\"pass\",\"Package\":\"example.org/widget\",\"Test\":\"TestValue/negative\"}\n{\"Action\":\"pass\",\"Package\":\"example.org/widget\"}\n", "", "", "example.org/widget", "TestValue/negative"},
		{"unittest", "python", "unittest", "test_value (tests.test_widget.ValueTest) ... ok\n\nRan 1 test in 0.001s\n\nOK\n", "", "", "tests.test_widget.ValueTest", "test_value"},
		{"pytest", "python", "pytest", "", `{"tests":[{"nodeid":"tests/test_widget.py::ValueTest::test_value[negative-case]","outcome":"passed"}]}`, ".json", "tests/test_widget.py::ValueTest", "test_value[negative-case]"},
		{"jest", "node", "jest", `{"numTotalTests":1,"numPassedTests":1,"numFailedTests":0,"testResults":[{"name":"/workspace/tests/widget.test.js","assertionResults":[{"title":"handles negative input","ancestorTitles":["widget","increment"],"status":"passed"}]}]}`, "", "", "/workspace/tests/widget.test.js", "widget > increment > handles negative input"},
		{"cargo", "rust", "", "running 1 test\ntest widget::tests::negative ... ok\n\ntest result: ok. 1 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out\n", "", "", "cargo", "widget::tests::negative"},
		{"junit", "java", "maven", "", `<testsuite name="widget suite" tests="1"><testcase classname="example.WidgetTest" name="negative[1]"/></testsuite>`, ".xml", "widget suite", "example.WidgetTest#negative[1]"},
		{"rspec", "ruby", "rspec", `{"examples":[{"full_description":"Widget increment handles negative input","file_path":"./spec/widget_spec.rb","status":"passed","run_time":0.001}],"summary":{"example_count":1,"failure_count":0,"pending_count":0}}`, "", "", "./spec/widget_spec.rb", "Widget increment handles negative input"},
		{"swift", "swift", "", "Test Case '-[WidgetTests.ValueTest testNegative]' passed (0.001 seconds).\n", "", "", "WidgetTests.ValueTest", "testNegative"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			artifact := ""
			if tc.artifact != "" {
				artifact = filepath.Join(root, "protocol"+tc.suffix)
				if err := os.WriteFile(artifact, []byte(tc.artifact), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, rel := range []string{".", "packages/widget", "packages/sibling"} {
				plan := runnerPlan{Runner: tc.runner, Framework: tc.framework, Root: filepath.Join(root, filepath.FromSlash(rel))}
				report, err := parseRunnerOutputForPlan(plan, tc.stdout, artifact, "protocol fixture (not executed)", nil)
				if err != nil || report == nil || len(report.TestResults) != 1 {
					t.Fatalf("parser fixture: err=%v report=%+v", err, report)
				}
				row := report.TestResults[0]
				if row.Suite != tc.suite || row.AssertionID != tc.id || !row.Passed || row.ObservationScope != types.TestObservationScopeAssertion {
					t.Fatalf("native protocol identity changed before qualification: %+v", row)
				}
				report = qualifyChangeReport(report, plan, root)
				prefix := ""
				if rel != "." {
					prefix = tc.runner
					if (tc.runner == "python" || tc.runner == "java") && tc.framework != "" {
						prefix += "/" + tc.framework
					}
					prefix += "@" + rel + "::"
				}
				qualified := report.TestResults[0]
				if qualified.Suite != prefix+tc.suite || qualified.AssertionID != prefix+tc.id || qualified.Passed != row.Passed || qualified.ObservationScope != row.ObservationScope {
					t.Errorf("%s qualified identity/scope changed: got %q / %q, want %q / %q", rel, qualified.Suite, qualified.AssertionID, prefix+tc.suite, prefix+tc.id)
				}
			}
		})
	}
}
