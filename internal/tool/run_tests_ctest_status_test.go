package tool

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The real child below implements CTest's --output-junit protocol. It is not
// native CTest or a claim that C++ assertions ran. CMake v3.31.0 writes
// name=classname, status=run/fail/notrun/disabled; only notrun gets <skipped/>.
// https://github.com/Kitware/CMake/blob/v3.31.0/Source/CTest/cmCTestTestHandler.cxx#L2505-L2530
// CTest is currently excluded from impactCandidateSupportsSuite: this test
// explicitly preserves that PTO gate instead of inventing a Java candidate to
// claim that the mislabeled CTest result discharged a required contract.
func TestB1653CTestPublicStatusPreservesRowsWithoutInventingAssertionScope(t *testing.T) {
	for _, tc := range []struct {
		name, status, child string
		exit                int
		passed, asserting   bool
	}{
		{"disabled", "disabled", "", 0, true, false},
		{"run", "run", "", 0, true, true},
		{"fail", "fail", `<failure message="Failed"/>`, 8, false, true},
		{"fail_error", "fail", `<error message="error"/>`, 8, false, true},
		{"notrun", "notrun", `<skipped message="SKIP_RETURN_CODE"/>`, 0, true, false},
		{"notrun_without_skipped", "notrun", "", 0, true, false},
		{"legacy_skipped", "", `<skipped/>`, 0, true, false},
		{"legacy_no_status", "", "", 0, true, true},
		{"legacy_failure", "", `<failure message="legacy"/>`, 8, false, true},
		{"unknown", "future_status", "", 0, true, false},
		{"unknown_failure", "future_status", `<failure message="failed"/>`, 8, false, false},
		{"fail_without_failure", "fail", "", 0, true, false},
		{"run_with_failure", "run", `<failure message="conflict"/>`, 8, false, false},
		{"run_with_skipped", "run", `<skipped/>`, 0, true, false},
		{"fail_with_skipped", "fail", `<failure message="conflict"/><skipped/>`, 8, false, false},
		{"disabled_with_failure", "disabled", `<failure message="conflict"/>`, 8, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(rel, body string, mode os.FileMode) {
				t.Helper()
				path := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			write("CMakeLists.txt", "enable_testing()\n", 0o644)
			write("build/CMakeCache.txt", "CMAKE_HOME_DIRECTORY:INTERNAL="+root+"\n", 0o644)
			write("build/CTestTestfile.cmake", "add_test(check /bin/true)\n", 0o644)
			write("src/value.cpp", "int value() { return 1; }\n", 0o644)
			write("tests/check.cpp", "int main() { return 0; }\n", 0o644)
			statusAttr := ""
			if tc.status != "" {
				statusAttr = fmt.Sprintf(` status="%s"`, tc.status)
			}
			body := `<testsuite name="CTestFixture" tests="1"><testcase name="check" classname="check" time="0.125"` + statusAttr + `>` + tc.child + `<properties/><system-out/></testcase></testsuite>`
			script := "#!/bin/sh\nset -eu\nreport=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = --output-junit ]; then shift; report=$1; fi\n shift\ndone\ntest -n \"$report\"\nprintf '%s' \"$report\" > fixture-report-path.txt\nprintf '%s' '" + body + "' > \"$report\"\n"
			script += fmt.Sprintf("exit %d\n", tc.exit)
			write("bin/ctest", script, 0o755)
			t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			plan := &types.ChangePlan{ID: "b1653-ctest-status", Status: types.PlanStatusApplied,
				TargetPaths: []string{"src/value.cpp"}, Changes: []types.FileChange{{Path: "src/value.cpp", Kind: "patch"}},
				BehaviorContracts:       []types.WriteBehaviorContract{{ID: "value-contract", Kind: "observable", Required: true}},
				ProjectTestObservations: []types.ProjectTestObservation{{ID: "check-observation", TestPath: "tests/check.cpp", AssertionSuite: "CTestFixture", AssertionID: "check#check", ContractRefs: []string{"value-contract"}}},
			}
			mut := types.NewMutableState("CTest status scope")
			mut.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mut, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
			_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "cmake"}))
			if err != nil {
				t.Fatal(err)
			}
			pathBytes, err := os.ReadFile(filepath.Join(root, "fixture-report-path.txt"))
			if err != nil || !strings.Contains(string(pathBytes), "/junit-invocation-") {
				t.Fatalf("current child/output namespace missing: %q %v", pathBytes, err)
			}
			report := mut.ChangeReport()
			if report == nil || len(report.TestResults) != 1 {
				t.Fatalf("real current testcase was not parsed: %+v", report)
			}
			row := report.TestResults[0]
			var genericSuite junitTestSuite
			if err := xml.Unmarshal([]byte(body), &genericSuite); err != nil {
				t.Fatal(err)
			}
			genericRow := junitCasesToResults(genericSuite)[0]
			withoutScope := row
			withoutScope.ObservationScope = genericRow.ObservationScope
			if !reflect.DeepEqual(withoutScope, genericRow) {
				t.Fatalf("CTest status changed fields beyond scope: got=%+v generic=%+v", withoutScope, genericRow)
			}
			if row.AssertionID != "check#check" || row.Suite != "CTestFixture" || row.Passed != tc.passed || report.Passed != tc.passed || row.Duration.Milliseconds() != 125 {
				t.Fatalf("fixture identity/value preconditions failed: row=%+v report=%+v", row, report)
			}
			actualCommand := false
			for _, cmd := range report.ExecutedCommands {
				if cmd.Runner == "cmake" && cmd.Outcome == types.ExecutedCommandOutcomeExecuted && cmd.ExitCode == tc.exit && strings.Contains(cmd.Command, string(pathBytes)) {
					actualCommand = true
				}
			}
			if !actualCommand {
				t.Fatalf("real child command receipt missing: %+v", report.ExecutedCommands)
			}
			// JSON uses the installed report, not reconstructed rows or confidence.
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			for _, current := range []*types.ChangeReport{report, &restored} {
				if projectTestObservationExecuted(plan.ProjectTestObservations[0], current) || len(projectTestObservationExecutionMatches(plan.ProjectTestObservations[0], current, false)) != 0 {
					t.Error("the existing unsupported CTest PTO selector gate changed")
				}
				found := false
				for _, obligation := range types.BuildVerificationProofLedger(plan, current, nil).Obligations {
					if obligation.ContractRef == "value-contract" {
						found = true
						if obligation.Status == types.VerificationProofLedgerItemCovered {
							t.Error("CTest unexpectedly covered a real PTO contract")
						}
					}
				}
				if !found {
					t.Error("test omitted the real required-contract proof domain")
				}
			}
			want := types.TestObservationScopeNonAsserting
			if tc.asserting {
				want = types.TestObservationScopeAssertion
			}
			t.Logf("actual row status=%q Passed=%t scope=%q; installed/JSON PTO remains unproved", tc.status, row.Passed, row.ObservationScope)
			if row.ObservationScope != want || restored.TestResults[0].ObservationScope != want {
				t.Errorf("CTest status must qualify assertion scope without changing Passed: want %q got %+v", want, row)
			}
		})
	}
}

func TestB1653GenericJUnitStatusCompatibility(t *testing.T) {
	for _, status := range []string{"", "run", "disabled", "custom_framework_status"} {
		root := t.TempDir()
		body := `<testsuite name="generic"><testcase name="checks" status="` + status + `"/></testsuite>`
		if err := os.WriteFile(filepath.Join(root, "TEST.xml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		report, err := parseJUnitXMLDir("java", root, "")
		if err != nil || len(report.TestResults) != 1 || report.TestResults[0].ObservationScope != types.TestObservationScopeAssertion || !report.Passed {
			t.Fatalf("CTest adapter must not reinterpret another framework's status %q: %+v %v", status, report, err)
		}
	}
}
