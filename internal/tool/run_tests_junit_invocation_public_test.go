package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A real child process implements the Maven reporting protocol here; it is not
// a Maven installation or a claim that native Java assertions ran on this host.
// Both generations contain the same test identity. Only the executor's current
// reporting namespace may supply positive OR negative behavior evidence.
func TestB1651MavenCurrentInvocationOwnsJUnitEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, old, current string
		exit               int
	}{
		{"current_pass", "", "pass", 0},
		{"current_failure", "", "failure", 1},
		{"current_skipped", "", "skipped", 0},
		{"current_pass_nonzero_command", "", "pass", 1},
		{"old_pass_only", "pass", "", 0},
		{"old_failure_only", "failure", "", 1},
		{"old_failure_current_pass", "failure", "pass", 0},
		{"old_pass_current_failure", "pass", "failure", 1},
		{"old_and_current_identical_pass", "pass", "pass", 0},
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
			write("pom.xml", "<project/>\n", 0o644)
			write("src/main/java/example/Value.java", "package example; class Value { int value() { return 1; } }\n", 0o644)
			write("src/test/java/example/ValueTest.java", "package example; class ValueTest { @Test void checks() {} }\n", 0o644)
			child := func(outcome string) string {
				switch outcome {
				case "failure":
					return `<failure type="AssertionError">wrong value</failure>`
				case "skipped":
					return `<skipped message="disabled"/>`
				default:
					return ""
				}
			}
			oldPath := filepath.Join(root, "target/surefire-reports/TEST-legacy.xml")
			var oldBytes []byte
			oldTime := time.Unix(946684800, 0)
			if tc.old != "" {
				oldBytes = []byte(`<testsuite name="example.ValueTest"><testcase classname="example.ValueTest" name="checks">` + child(tc.old) + `</testcase></testsuite>`)
				write("target/surefire-reports/TEST-legacy.xml", string(oldBytes), 0o644)
				if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
					t.Fatal(err)
				}
			}
			script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > fixture-executed-args.txt\nsuffix=''\nfor arg in \"$@\"; do\n case \"$arg\" in -Dsurefire.reportNameSuffix=*) suffix=${arg#*=};; esac\ndone\n"
			if tc.current != "" {
				script += "mkdir -p target/surefire-reports\nattr=''\nfile=''\nif [ -n \"$suffix\" ]; then attr=\"($suffix)\"; file=\"-$suffix\"; fi\n"
				script += "printf '<testsuite name=\"example.ValueTest%s\"><testcase classname=\"example.ValueTest%s\" name=\"checks\" time=\"0.125\">%s</testcase></testsuite>\\n' \"$attr\" \"$attr\" '" + child(tc.current) + "' > \"target/surefire-reports/TEST-example.ValueTest$file.xml\"\n"
			}
			script += fmt.Sprintf("printf 'fixture command completed\\n'\nexit %d\n", tc.exit)
			write("fake-bin/mvn", script, 0o755)
			t.Setenv("PATH", filepath.Join(root, "fake-bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			plan := &types.ChangePlan{
				ID: "b1651-maven-invocation", Status: types.PlanStatusApplied,
				TargetPaths:       []string{"src/main/java/example/Value.java"},
				Changes:           []types.FileChange{{Path: "src/main/java/example/Value.java", Kind: "patch"}},
				BehaviorContracts: []types.WriteBehaviorContract{{ID: "value-contract", Kind: "observable", Required: true}},
				ProjectTestObservations: []types.ProjectTestObservation{{
					ID: "value-test", TestPath: "src/test/java/example/ValueTest.java",
					AssertionSuite: "example.ValueTest", AssertionID: "example.ValueTest#checks", ContractRefs: []string{"value-contract"},
				}},
			}
			mu := types.NewMutableState("current Maven report source")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
			_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "java"}))
			if err != nil {
				t.Fatal(err)
			}
			args, err := os.ReadFile(filepath.Join(root, "fixture-executed-args.txt"))
			if err != nil || !strings.Contains(string(args), "-Dtest=example.ValueTest") {
				t.Fatalf("exact selected class was not executed: %q %v", args, err)
			}
			if oldBytes != nil {
				after, err := os.ReadFile(oldPath)
				info, statErr := os.Stat(oldPath)
				if err != nil || statErr != nil || !bytes.Equal(after, oldBytes) || !info.ModTime().Equal(oldTime) {
					t.Fatalf("must preserve user reports, not delete/move/rewrite them: %v %v", err, statErr)
				}
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatal("no installed report")
			}
			wantRows := 0
			if tc.current != "" {
				wantRows = 1
			}
			if len(report.TestResults) != wantRows {
				t.Errorf("only current invocation rows may affect counts and outcome: want %d got %+v", wantRows, report.TestResults)
			}
			if tc.current == "" && (report.Passed || report.BuildFailed || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable) {
				t.Errorf("unbound reports are unavailable, not passed or inferred build failure: %+v", report)
			}
			if tc.current != "" && report.Passed != (tc.current != "failure" && tc.exit == 0) {
				t.Errorf("old reports contaminated current suite verdict: %+v", report)
			}
			observation := plan.ProjectTestObservations[0]
			positive := projectTestObservationExecuted(observation, report)
			negative := len(projectTestObservationExecutionMatches(observation, report, false)) != 0
			if positive != (tc.current == "pass" && tc.exit == 0) || negative != (tc.current == "failure") {
				t.Errorf("wrong current positive/negative evidence: current=%s positive=%t negative=%t", tc.current, positive, negative)
			}
			// The real installed confidence is serialized and re-consumed. No
			// hand-authored result, command or confidence fills the proof lane.
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			covered := false
			for _, row := range types.BuildVerificationProofLedger(plan, &restored, nil).Obligations {
				if row.ContractRef == "value-contract" && row.Status == types.VerificationProofLedgerItemCovered {
					covered = true
				}
			}
			if covered != (tc.current == "pass" && tc.exit == 0) {
				t.Errorf("persisted proof borrowed a different execution: current=%s covered=%t", tc.current, covered)
			}
		})
	}
}

func TestB1651CTestUsesOnlyItsPrivateInvocationReport(t *testing.T) {
	for _, produce := range []bool{true, false} {
		t.Run(fmt.Sprintf("produce=%t", produce), func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"build", "bin"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			write := func(rel, body string, mode os.FileMode) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, rel), []byte(body), mode); err != nil {
					t.Fatal(err)
				}
			}
			write("CMakeLists.txt", "enable_testing()\n", 0o644)
			write("build/CMakeCache.txt", "CMAKE_HOME_DIRECTORY:INTERNAL="+root+"\n", 0o644)
			write("build/CTestTestfile.cmake", "add_test(check /bin/true)\n", 0o644)
			old := `<testsuite name="old"><testcase name="stale"/></testsuite>`
			write(".codrax-ctest-report.xml", old, 0o644)
			script := "#!/bin/sh\nset -eu\nreport=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = --output-junit ]; then shift; report=$1; fi\n shift\ndone\nprintf '%s' \"$report\" > fixture-report-path.txt\n"
			if produce {
				script += "printf '%s' '<testsuite name=\"current\"><testcase name=\"check\" time=\"0.125\"/></testsuite>' > \"$report\"\n"
			}
			script += "exit 0\n"
			write("bin/ctest", script, 0o755)
			t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			mu := types.NewMutableState("CTest current execution")
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
			_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "cmake"}))
			if err != nil {
				t.Fatal(err)
			}
			pathBytes, err := os.ReadFile(filepath.Join(root, "fixture-report-path.txt"))
			if err != nil {
				t.Fatalf("actual CTest protocol child did not run: %v", err)
			}
			canonicalRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(filepath.Join(canonicalRoot, ".codrax", "tmp"), string(pathBytes))
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				t.Fatalf("report must be in private .codrax/tmp, not a fixed project file: %q", pathBytes)
			}
			if after, err := os.ReadFile(filepath.Join(root, ".codrax-ctest-report.xml")); err != nil || string(after) != old {
				t.Fatalf("must not delete or overwrite prior project report: %q %v", after, err)
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatal("missing current report")
			}
			if produce {
				if !report.Passed || len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "check" || report.TestResults[0].ObservationScope != types.TestObservationScopeAssertion {
					t.Fatalf("current report positive control lost: %+v", report)
				}
			} else if report.Passed || report.BuildFailed || len(report.TestResults) != 0 || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
				t.Fatalf("missing current artifact cannot borrow stale XML: %+v", report)
			}
			if _, err := os.Stat(string(pathBytes)); !os.IsNotExist(err) {
				t.Fatalf("owned report should be cleaned after parsing, got %v", err)
			}
		})
	}
}
