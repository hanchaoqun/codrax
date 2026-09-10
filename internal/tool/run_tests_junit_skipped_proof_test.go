package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises the real RunTests process/parser/report path with a bounded
// Maven protocol fixture. It does not claim that the host has a Java compiler
// or that the fixture's XML is evidence of an independently executed Java test.
func TestB1650JUnitSkippedDoesNotProveBehaviorThroughRunTests(t *testing.T) {
	for _, skipped := range []bool{false, true} {
		t.Run(fmt.Sprintf("skipped=%t", skipped), func(t *testing.T) {
			root := t.TempDir()
			write := func(rel, body string, mode os.FileMode) {
				t.Helper()
				p := filepath.Join(root, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			write("pom.xml", "<project/>\n", 0o644)
			write("src/main/java/example/Value.java", "package example; class Value { int value() { return 1; } }\n", 0o644)
			write("src/test/java/example/ValueTest.java", "package example; class ValueTest { @Test void checks() {} }\n", 0o644)
			child := ""
			if skipped {
				child = `<skipped message="not executed"/>`
			}
			xml := `<testsuite name="example.ValueTest" tests="1"><testcase classname="example.ValueTest" name="checks">` + child + `</testcase></testsuite>`
			write("fake-bin/mvn", "#!/bin/sh\nset -eu\nmkdir -p target/surefire-reports\nprintf '%s\\n' '"+xml+"' > target/surefire-reports/TEST-example.ValueTest.xml\nprintf '%s\\n' fixture-runner-finished\n", 0o755)
			t.Setenv("PATH", filepath.Join(root, "fake-bin")+string(os.PathListSeparator)+os.Getenv("PATH"))

			plan := &types.ChangePlan{
				ID: "b1561-junit-skip", Status: types.PlanStatusApplied,
				TargetPaths:       []string{"src/main/java/example/Value.java"},
				Changes:           []types.FileChange{{Path: "src/main/java/example/Value.java", Kind: "patch"}},
				BehaviorContracts: []types.WriteBehaviorContract{{ID: "value-contract", Kind: "observable", Required: true}},
				ProjectTestObservations: []types.ProjectTestObservation{{
					ID: "value-test", TestPath: "src/test/java/example/ValueTest.java",
					AssertionSuite: "example.ValueTest", AssertionID: "example.ValueTest#checks", ContractRefs: []string{"value-contract"},
				}},
			}
			mu := types.NewMutableState("JUnit skipped behavior scope")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
			result, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{}))
			if err != nil || !result.Success {
				t.Fatalf("public runner failed: err=%v result=%+v", err, result)
			}
			report := mu.ChangeReport()
			if report == nil || !report.Passed || len(report.TestResults) != 1 {
				t.Fatalf("skip must remain a non-failing suite result: %+v", report)
			}
			row := report.TestResults[0]
			if row.AssertionID != "example.ValueTest#checks" || row.Suite != "example.ValueTest" || !row.Passed {
				t.Fatalf("runner identity/pass semantics changed: %+v", row)
			}
			wantScope := types.TestObservationScopeAssertion
			if skipped {
				wantScope = types.TestObservationScopeNonAsserting
			}
			if row.ObservationScope != wantScope {
				t.Fatalf("known skip must have explicit non-asserting scope: %+v", row)
			}
			executed := false
			for _, cmd := range report.ExecutedCommands {
				if cmd.Runner == "java" && cmd.Outcome == types.ExecutedCommandOutcomeExecuted && cmd.ExitCode == 0 && strings.Contains(cmd.Command, "-Dtest=") {
					executed = true
				}
			}
			if !executed {
				t.Fatalf("missing actual exact Java candidate invocation: %+v", report.ExecutedCommands)
			}
			// Same production projector used by the verifier after RunTests.
			report.VerificationConfidence = verificationConfidenceRecordsFromReport(plan, report)
			covered := verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed")
			ledger := types.BuildVerificationProofLedger(plan, report, nil)
			coveredObligation := false
			for _, item := range ledger.Obligations {
				if item.ContractRef == "value-contract" && item.Status == types.VerificationProofLedgerItemCovered {
					coveredObligation = true
				}
			}
			if covered != !skipped || coveredObligation != !skipped {
				t.Fatalf("skipped=%t acquired behavior proof: row=%+v confidence=%+v ledger=%+v", skipped, row, report.VerificationConfidence, ledger)
			}
			if skipped && !verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "missing", "project_test_assertion_not_observed") {
				t.Fatalf("skip must retain precise missing-assertion debt: %+v", report.VerificationConfidence)
			}
		})
	}
}

func TestB1650JUnitScopePreservesSuiteOutcomeAndJSON(t *testing.T) {
	for _, tc := range []struct {
		name, child string
		passed      bool
		scope       types.TestObservationScope
	}{
		{"passed", "", true, types.TestObservationScopeAssertion},
		{"skipped", `<skipped message="disabled"/>`, true, types.TestObservationScopeNonAsserting},
		{"failure", `<failure type="AssertionError">wrong value</failure>`, false, types.TestObservationScopeAssertion},
		{"error", `<error type="Exception">runner error</error>`, false, types.TestObservationScopeAssertion},
		{"contradictory skipped failure", `<skipped/><failure>wrong value</failure>`, false, types.TestObservationScopeNonAsserting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := seedJUnitDir(t, map[string]string{"TEST-scope.xml": `<testsuite name="Suite"><testcase classname="Case" name="target" time="0.125">` + tc.child + `</testcase></testsuite>`})
			report, err := parseJUnitXMLDir("java", dir, "")
			if err != nil || len(report.TestResults) != 1 {
				t.Fatalf("parse: report=%+v err=%v", report, err)
			}
			row := report.TestResults[0]
			if report.Passed != tc.passed || row.Passed != tc.passed || row.ObservationScope != tc.scope || row.Suite != "Suite" || row.AssertionID != "Case#target" || row.Duration.Milliseconds() != 125 {
				t.Fatalf("only the explicit observation scope may differ for skip: %+v", report)
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(data, &restored); err != nil || !reflect.DeepEqual(report, &restored) {
				t.Fatalf("JSON lost result scope/identity: %s err=%v", data, err)
			}
		})
	}
	// Old reports have no skip marker. Do not retrospectively infer one from a
	// name, detail or Passed, or change their existing non-assertion status.
	for _, scope := range []types.TestObservationScope{"", types.TestObservationScopeAggregate, types.TestObservationScopeAssertion} {
		before := types.TestResult{ObservationScope: scope, AssertionID: "skipped", Suite: "legacy", Passed: true}
		data, err := json.Marshal(before)
		if err != nil {
			t.Fatal(err)
		}
		var after types.TestResult
		if err := json.Unmarshal(data, &after); err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("legacy scope must not be guessed or migrated: before=%+v after=%+v err=%v", before, after, err)
		}
	}
}

// Only JUnit above uses the full public executor. This matrix supplies each
// other parser's actual protocol and then the existing exact proof join; it
// does not claim that every corresponding language runtime ran on this host.
func TestB1650NonAssertingParserOutcomesCannotProveBehavior(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parse func(t *testing.T) (*types.ChangeReport, error)
	}{
		{"unittest skipped", func(t *testing.T) (*types.ChangeReport, error) {
			return parseUnittestOutput("test_value (ValueTest.test_value) ... skipped 'disabled'\nRan 1 test in 0.001s\nOK (skipped=1)\n", nil)
		}},
		{"go skipped", func(t *testing.T) (*types.ChangeReport, error) {
			return parseGoTestJSONLines("{\"Action\":\"run\",\"Package\":\"example\",\"Test\":\"TestValue\"}\n{\"Action\":\"skip\",\"Package\":\"example\",\"Test\":\"TestValue\"}\n{\"Action\":\"pass\",\"Package\":\"example\"}\n")
		}},
		{"jest skipped", func(t *testing.T) (*types.ChangeReport, error) {
			return parseJestJSON(`{"success":true,"numTotalTests":1,"numPendingTests":1,"testResults":[{"name":"value.test.js","assertionResults":[{"title":"value","status":"skipped"}]}]}`)
		}},
		{"jest pending", func(t *testing.T) (*types.ChangeReport, error) {
			return parseJestJSON(`{"success":true,"numTotalTests":1,"numPendingTests":1,"testResults":[{"name":"value.test.js","assertionResults":[{"title":"value","status":"pending"}]}]}`)
		}},
		{"pytest JSON skipped", func(t *testing.T) (*types.ChangeReport, error) {
			p := filepath.Join(t.TempDir(), "report.json")
			if err := os.WriteFile(p, []byte(`{"exitcode":0,"summary":{"total":1,"skipped":1},"tests":[{"nodeid":"value_test.py::test_value","outcome":"skipped"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			return parsePytestJSONReport(p, "", "pytest")
		}},
		{"pytest text skipped", func(t *testing.T) (*types.ChangeReport, error) {
			return &types.ChangeReport{Passed: true, TestResults: parsePytestTextCaseRows("value_test.py::test_value SKIPPED [100%]\n")}, nil
		}},
		{"pytest text XFAIL", func(t *testing.T) (*types.ChangeReport, error) {
			return &types.ChangeReport{Passed: true, TestResults: parsePytestTextCaseRows("value_test.py::test_value XFAIL [100%]\n")}, nil
		}},
		{"cargo ignored", func(t *testing.T) (*types.ChangeReport, error) {
			return parseCargoTestText("rust", "running 1 test\ntest tests::value ... ignored\ntest result: ok. 0 passed; 0 failed; 1 ignored\n")
		}},
		{"cjpm ignored", func(t *testing.T) (*types.ChangeReport, error) {
			return parseCargoTestText("cjpm", "running 1 test\ntest tests::value ... ignored\ntest result: ok. 0 passed; 0 failed; 1 ignored\n")
		}},
		{"RSpec pending", func(t *testing.T) (*types.ChangeReport, error) {
			return parseRSpecJSON(`{"summary":{"example_count":1,"failure_count":0,"pending_count":1},"examples":[{"full_description":"value","file_path":"spec/value_spec.rb","status":"pending"}]}`)
		}},
		{"RSpec unknown", func(t *testing.T) (*types.ChangeReport, error) {
			return parseRSpecJSON(`{"summary":{"example_count":1,"failure_count":0},"examples":[{"full_description":"value","file_path":"spec/value_spec.rb","status":"unrecognized"}]}`)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := tc.parse(t)
			if err != nil || report == nil || !report.Passed || len(report.TestResults) != 1 || !report.TestResults[0].Passed {
				t.Fatalf("protocol/legacy non-failing precondition: report=%+v err=%v", report, err)
			}
			row := report.TestResults[0]
			if row.ObservationScope != types.TestObservationScopeNonAsserting {
				t.Fatalf("protocol must retain explicit non-asserting scope: %+v", row)
			}
			plan := &types.ChangePlan{ID: "b1650-parser", BehaviorContracts: []types.WriteBehaviorContract{{ID: "value-contract", Kind: "observable", Required: true}}, ProjectTestObservations: []types.ProjectTestObservation{{ID: "value-test", TestPath: "tests/value.cpp", AssertionSuite: row.Suite, AssertionID: row.AssertionID, ContractRefs: []string{"value-contract"}}}}
			report.PlanID = plan.ID
			report.TestSurface = &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{ID: "make@.", Runner: "make", WorkingDir: ".", MakeTarget: "check", HasTestSignal: true, DeclaredCoveragePaths: []string{"tests/value.cpp"}}}}
			report.ExecutedCommands = []types.ExecutedCommand{{Runner: "make", WorkingDir: ".", Suite: "check", Command: "make check", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 0}}
			report.VerificationConfidence = verificationConfidenceRecordsFromReport(plan, report)
			if verificationConfidenceContains(report.VerificationConfidence, "project_test_contract_refs", "satisfied", "project_test_contract_ref_observed") {
				t.Errorf("non-asserting protocol outcome acquired proof: row=%+v confidence=%+v", row, report.VerificationConfidence)
			}
			ledger := types.BuildVerificationProofLedger(plan, report, nil)
			for _, item := range ledger.Obligations {
				if item.ContractRef == "value-contract" && item.Status == types.VerificationProofLedgerItemCovered {
					t.Errorf("non-asserting outcome covered ledger obligation: %+v", item)
				}
			}
		})
	}
}

func TestB1650ObservationScopeBothProofGatesRemainExact(t *testing.T) {
	for _, scope := range []types.TestObservationScope{"", types.TestObservationScopeAssertion, types.TestObservationScopeAggregate, types.TestObservationScopeNonAsserting, "future_scope"} {
		for _, passed := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/passed=%t", scope, passed), func(t *testing.T) {
				plan, report := projectFailureScopeFixture()
				report.TestResults[0].ObservationScope = scope
				report.TestResults[0].Passed = passed
				report.Passed = passed
				if passed {
					report.ExecutedCommands[0].ExitCode = 0
				}
				before, err := json.Marshal(report)
				if err != nil {
					t.Fatal(err)
				}
				matches := projectTestObservationExecutionMatches(plan.ProjectTestObservations[0], report, passed)
				wantMatch := scope == types.TestObservationScopeAssertion
				if (len(matches) > 0) != wantMatch {
					t.Fatalf("scope=%q must not gain execution authority: matches=%+v", scope, matches)
				}
				relevance := BuildVerifyFailureContractRelevance(report, plan)
				if (len(relevance.Hits) > 0) != (!passed && wantMatch) {
					t.Fatalf("non-asserting outcome must not retire a behavior expectation: %+v", relevance)
				}
				after, err := json.Marshal(report)
				if err != nil || string(before) != string(after) {
					t.Fatalf("read-only proof consumers changed report: before=%s after=%s err=%v", before, after, err)
				}
			})
		}
	}
}

func TestB1650UnknownOrIncompleteParserOutcomesCannotProveFailure(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parse func(t *testing.T) (*types.ChangeReport, error)
	}{
		{"go no terminal", func(t *testing.T) (*types.ChangeReport, error) {
			return parseGoTestJSONLines("{\"Action\":\"run\",\"Package\":\"example\",\"Test\":\"TestValue\"}\n")
		}},
		{"jest todo", func(t *testing.T) (*types.ChangeReport, error) {
			return parseJestJSON(`{"success":false,"numTotalTests":1,"numFailedTests":1,"testResults":[{"name":"value.test.js","assertionResults":[{"title":"value","status":"todo"}]}]}`)
		}},
		{"jest disabled", func(t *testing.T) (*types.ChangeReport, error) {
			return parseJestJSON(`{"success":false,"numTotalTests":1,"numFailedTests":1,"testResults":[{"name":"value.test.js","assertionResults":[{"title":"value","status":"disabled"}]}]}`)
		}},
		{"jest unknown", func(t *testing.T) (*types.ChangeReport, error) {
			return parseJestJSON(`{"success":false,"numTotalTests":1,"numFailedTests":1,"testResults":[{"name":"value.test.js","assertionResults":[{"title":"value","status":"unknown"}]}]}`)
		}},
		{"pytest unknown", b1650PytestOutcomeParser("unknown")},
		{"pytest xfailed", b1650PytestOutcomeParser("xfailed")},
		{"pytest xpassed", b1650PytestOutcomeParser("xpassed")},
		{"pytest text XPASS", func(t *testing.T) (*types.ChangeReport, error) {
			return parsePytestTextOutput("value_test.py::test_value XPASS [100%]\n=== 1 xpassed in 0.001s ===\n", "pytest", nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := tc.parse(t)
			if err != nil || report == nil || report.Passed || len(report.TestResults) != 1 || report.TestResults[0].Passed {
				t.Fatalf("existing unsuccessful outcome must remain unchanged: report=%+v err=%v", report, err)
			}
			row := report.TestResults[0]
			plan := &types.ChangePlan{ID: "b1650-unknown", ProjectTestObservations: []types.ProjectTestObservation{{ID: "case", TestPath: "tests/value.cpp", AssertionSuite: row.Suite, AssertionID: row.AssertionID, ContractRefs: []string{"contract"}}}}
			report.PlanID = plan.ID
			report.TestSurface = &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{Runner: "make", WorkingDir: ".", MakeTarget: "check", HasTestSignal: true, DeclaredCoveragePaths: []string{"tests/value.cpp"}}}}
			report.ExecutedCommands = []types.ExecutedCommand{{Runner: "make", WorkingDir: ".", Suite: "check", Outcome: types.ExecutedCommandOutcomeExecuted, ExitCode: 1}}
			if matches := projectTestObservationExecutionMatches(plan.ProjectTestObservations[0], report, false); len(matches) > 0 {
				t.Errorf("unknown/incomplete status became a measured assertion failure: row=%+v matches=%+v", row, matches)
			}
			if relevance := BuildVerifyFailureContractRelevance(report, plan); len(relevance.Hits) > 0 {
				t.Errorf("non-asserting status retired behavior contract: %+v", relevance)
			}
		})
	}
}

func b1650PytestOutcomeParser(outcome string) func(t *testing.T) (*types.ChangeReport, error) {
	return func(t *testing.T) (*types.ChangeReport, error) {
		p := filepath.Join(t.TempDir(), "report.json")
		data := fmt.Sprintf(`{"exitcode":1,"summary":{"total":1,"failed":1},"tests":[{"nodeid":"value_test.py::test_value","outcome":%q}]}`, outcome)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return parsePytestJSONReport(p, "", "pytest")
	}
}
