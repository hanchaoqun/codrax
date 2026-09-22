package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Real subprocesses implement the pytest command/report protocol; they do not
// claim a pytest installation or execution of Python test bodies. The public
// boundary under test is Execute -> parsed report -> durable assertion join.
func TestNativeInvocationPublicCannotBorrowAnotherCommandsExit(t *testing.T) {
	root := newPytestInvocationProtocolFixture(t, pytestInvocationProtocolScript("passed", 1, "", 0))
	first := executePytestInvocationProtocol(t, root, "tests/test_value.py")
	if first.Passed || len(first.TestResults) != 1 || !first.TestResults[0].Passed {
		t.Fatalf("fixture must retain the reported green row but nonzero command: %+v", first)
	}
	script := strings.ReplaceAll(pytestInvocationProtocolScript("passed", 0, "", 0), "test_current", "test_other")
	if err := os.WriteFile(filepath.Join(root, ".venv/bin/python"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	second := executePytestInvocationProtocol(t, root, "tests/test_value.py")
	if !second.Passed || len(second.TestResults) != 1 || second.TestResults[0].AssertionID != "test_other" {
		t.Fatalf("fixture must actually execute a second successful command: %+v", second)
	}
	combined := mergeChangeReports([]*types.ChangeReport{first, second})
	combined.TestSurface = first.TestSurface
	combined.ExecutedCommands = append(append([]types.ExecutedCommand(nil), first.ExecutedCommands...), second.ExecutedCommands...)
	data, err := json.Marshal(combined)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.ChangeReport
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	observation := types.ProjectTestObservation{ID: "current", TestPath: "tests/test_value.py", AssertionSuite: "tests/test_value.py", AssertionID: "test_current", ContractRefs: []string{"current-contract"}}
	if projectTestObservationExecuted(observation, &restored) {
		t.Fatal("a green row from a failed command borrowed the exit code of another successful invocation")
	}
	observation.AssertionID = "test_other"
	if !projectTestObservationExecuted(observation, &restored) {
		t.Fatal("the second invocation's own successful assertion must remain usable")
	}
	assertNativeInvocationRowsOwned(t, first)
	assertNativeInvocationRowsOwned(t, second)
	if first.TestResults[0].InvocationID == second.TestResults[0].InvocationID {
		t.Fatal("two actual executions must not share one invocation identity")
	}
}

func assertNativeInvocationRowsOwned(t *testing.T, report *types.ChangeReport) {
	t.Helper()
	index := types.NewNativeTestInvocationIndex(report)
	for rowIndex, row := range report.TestResults {
		if row.InvocationID == "" {
			t.Fatalf("actual native row has no invocation identity: %+v", row)
		}
		owners := 0
		for commandIndex := range report.ExecutedCommands {
			if index.Matches(commandIndex, rowIndex) {
				owners++
			}
		}
		if owners != 1 {
			t.Fatalf("actual native row needs one command owner, got %d: %+v", owners, row)
		}
	}
}

func TestNativeInvocationPublicTextFallbackOwnsItsRows(t *testing.T) {
	root := newPytestInvocationProtocolFixture(t, pytestInvocationProtocolScript("", 0,
		"tests/test_value.py::test_current PASSED [100%]\n=== 1 passed in 0.01s ===", 0))
	report := executePytestInvocationProtocol(t, root, "tests/test_value.py")
	if !report.Passed || len(report.TestResults) != 1 {
		t.Fatalf("text fallback fixture did not run: %+v", report)
	}
	assertNativeInvocationRowsOwned(t, report)
	for _, command := range report.ExecutedCommands {
		if command.Source == "parser_error_fallback" {
			if command.InvocationID != report.TestResults[0].InvocationID {
				t.Fatal("text row must belong to the real fallback command")
			}
		} else if command.InvocationID == report.TestResults[0].InvocationID {
			t.Fatal("text fallback row borrowed the failed JSON parser invocation")
		}
	}
}

func TestNativeInvocationJoinDoesNotCrossCommands(t *testing.T) {
	for _, passed := range []bool{true, false} {
		for _, mutation := range []string{"same", "second_owner", "different", "row_missing", "command_missing", "duplicate", "legacy_row_mixed", "legacy_command_mixed"} {
			t.Run(mutation+map[bool]string{true: "/pass", false: "/fail"}[passed], func(t *testing.T) {
				candidate := types.TestSurfaceCandidate{Runner: "python", Framework: "unittest", WorkingDir: "packages/widget", HasTestSignal: true}
				observation, report := scopeResolverFixture(t, candidate, "tests/test_value.py", "tests.test_value.ValueTest", "test_value", passed)
				report.ExecutedCommands[0].InvocationID = "command-current"
				report.TestResults[0].InvocationID = "command-current"
				want := mutation == "same" || mutation == "second_owner"
				switch mutation {
				case "second_owner":
					old := report.ExecutedCommands[0]
					old.InvocationID = "command-earlier"
					report.ExecutedCommands = append([]types.ExecutedCommand{old}, report.ExecutedCommands...)
				case "different":
					report.TestResults[0].InvocationID = "command-other"
				case "row_missing":
					report.TestResults[0].InvocationID = ""
				case "command_missing":
					report.ExecutedCommands[0].InvocationID = ""
				case "duplicate":
					report.ExecutedCommands = append(report.ExecutedCommands, report.ExecutedCommands[0])
				case "legacy_row_mixed", "legacy_command_mixed":
					report.ExecutedCommands[0].InvocationID, report.TestResults[0].InvocationID = "", ""
					if mutation == "legacy_row_mixed" {
						report.TestResults = append(report.TestResults, types.TestResult{InvocationID: "new-row"})
					} else {
						report.ExecutedCommands = append(report.ExecutedCommands, types.ExecutedCommand{InvocationID: "new-command"})
					}
				}
				before, _ := json.Marshal(report)
				matches := projectTestObservationExecutionMatches(observation, report, passed)
				if (len(matches) > 0) != want || (mutation == "second_owner" && len(matches) == 1 && matches[0].CommandIndex != 1) {
					t.Fatalf("wrong invocation join: want=%t got=%+v", want, matches)
				}
				if !passed {
					plan := &types.ChangePlan{ID: report.PlanID, ProjectTestObservations: []types.ProjectTestObservation{observation}}
					if hits := BuildVerifyFailureContractRelevance(report, plan).Hits; (len(hits) > 0) != want {
						t.Fatalf("failure relevance bypassed shared invocation ownership: %+v", hits)
					}
				}
				after, _ := json.Marshal(report)
				if string(before) != string(after) {
					t.Fatal("joining must not rewrite historical reports")
				}
			})
		}
	}
}
