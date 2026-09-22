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

// These children implement the pytest command/report protocol. They are real
// subprocesses, not a pytest installation or proof that Python assertions ran.
func TestPytestInvocationPublicDoesNotBorrowOldGreenReport(t *testing.T) {
	root := newPytestInvocationProtocolFixture(t, "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" >> fixture-args.txt\nprintf 'current invocation did not produce a report\\n'\nexit 0\n")
	oldPath := filepath.Join(root, ".codrax-pytest-report.json")
	old := []byte(`{"exitcode":0,"summary":{"passed":1,"total":1},"tests":[{"nodeid":"tests/test_value.py::test_old","outcome":"passed"}]}`)
	oldTime := time.Unix(946684800, 0)
	if err := os.WriteFile(oldPath, old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	report := executePytestInvocationProtocol(t, root, "tests/test_value.py")
	args, err := os.ReadFile(filepath.Join(root, "fixture-args.txt"))
	if err != nil || !strings.Contains(string(args), "--json-report-file=") {
		t.Fatalf("actual protocol child must receive the report argument: %q %v", args, err)
	}
	if report.Passed || report.BuildFailed || len(report.TestResults) != 0 || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Errorf("no current JSON or parseable text may borrow old PASS or assertion rows: %+v", report)
	}
	after, readErr := os.ReadFile(oldPath)
	info, statErr := os.Stat(oldPath)
	if readErr != nil || statErr != nil || !bytes.Equal(after, old) || !info.ModTime().Equal(oldTime) {
		t.Errorf("prior project report bytes and mtime must remain untouched: read=%v stat=%v", readErr, statErr)
	}
}

func TestPytestInvocationPublicCurrentEvidenceAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, old, current, text string
		exit, textExit           int
		wantPass                 bool
		wantRows                 int
		wantNoTests              bool
	}{
		{name: "old_red_current_green", old: "failed", current: "passed", wantPass: true, wantRows: 1},
		{name: "old_green_current_red", old: "passed", current: "failed", exit: 1, wantRows: 1},
		{name: "current_green_nonzero_command", current: "passed", exit: 1, wantRows: 1},
		{name: "missing_json_text_green", old: "failed", text: "tests/test_value.py::test_current PASSED [100%]\n=== 1 passed in 0.01s ===", wantPass: true, wantRows: 1},
		{name: "missing_json_text_red", old: "passed", text: "tests/test_value.py::test_current FAILED [100%]\n=== 1 failed in 0.01s ===", textExit: 1, wantRows: 1},
		{name: "current_zero_tests", old: "passed", current: "zero", exit: 5, wantPass: true, wantNoTests: true},
		{name: "current_skipped", old: "failed", current: "skipped", wantPass: true, wantRows: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newPytestInvocationProtocolFixture(t, pytestInvocationProtocolScript(tc.current, tc.exit, tc.text, tc.textExit))
			oldPath := filepath.Join(root, ".codrax-pytest-report.json")
			oldTime := time.Unix(946684800, 0)
			var old []byte
			if tc.old != "" {
				old = []byte(pytestInvocationProtocolJSON(tc.old, "tests/test_value.py::test_old"))
				if err := os.WriteFile(oldPath, old, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
					t.Fatal(err)
				}
			}
			report := executePytestInvocationProtocol(t, root, "tests/test_value.py")
			if report.Passed != tc.wantPass || len(report.TestResults) != tc.wantRows {
				t.Errorf("current command and current report/text must determine outcome: %+v", report)
			}
			for _, row := range report.TestResults {
				if row.AssertionID != "test_current" || row.Suite != "tests/test_value.py" {
					t.Errorf("stale or misbound assertion survived: %+v", row)
				}
				if tc.current == "skipped" && row.ObservationScope == types.TestObservationScopeAssertion {
					t.Errorf("skipped row gained assertion authority: %+v", row)
				}
			}
			if tc.wantNoTests && len(report.NoTestsRunners) == 0 {
				t.Errorf("explicit zero-test result lost its distinct no-tests channel: %+v", report)
			}
			if tc.current == "passed" && tc.exit != 0 {
				foundNonzero := false
				for _, cmd := range report.ExecutedCommands {
					if cmd.Framework == pythonFrameworkPytest && cmd.ExitCode == tc.exit {
						foundNonzero = true
					}
				}
				if !foundNonzero || len(report.TestResults) != 1 || !report.TestResults[0].Passed {
					t.Errorf("retain measured green row and nonzero command separately without invented failed assertion: %+v", report)
				}
			}
			if tc.text != "" {
				args, err := os.ReadFile(filepath.Join(root, "fixture-args.txt"))
				if err != nil || !strings.Contains(string(args), "\n-v\n") {
					t.Errorf("must actually execute text retry when current JSON is missing: %q %v", args, err)
				}
				fallback := false
				for _, cmd := range report.ExecutedCommands {
					fallback = fallback || cmd.Source == "parser_error_fallback"
				}
				if !fallback {
					t.Errorf("text recovery needs an execution record: %+v", report.ExecutedCommands)
				}
			}
			if old != nil {
				after, readErr := os.ReadFile(oldPath)
				info, statErr := os.Stat(oldPath)
				if readErr != nil || statErr != nil || !bytes.Equal(after, old) || !info.ModTime().Equal(oldTime) {
					t.Errorf("old report must survive current invocation and cleanup unchanged: read=%v stat=%v", readErr, statErr)
				}
			}
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.Passed != tc.wantPass || len(restored.TestResults) != tc.wantRows {
				t.Errorf("persisted report must not regain stale/current-command-invalid PASS: %+v", restored)
			}
			observation := types.ProjectTestObservation{TestPath: "tests/test_value.py", AssertionSuite: "tests/test_value.py", AssertionID: "test_current"}
			positive := projectTestObservationExecuted(observation, &restored)
			negative := len(projectTestObservationExecutionMatches(observation, &restored, false)) > 0
			wantPositive := tc.wantPass && tc.wantRows > 0 && tc.current != "skipped"
			wantNegative := tc.current == "failed" || tc.textExit == 1
			if positive != wantPositive || negative != wantNegative {
				t.Errorf("persisted assertion/command join borrowed unrelated or nonexecuted proof: positive=%t/%t negative=%t/%t", positive, wantPositive, negative, wantNegative)
			}
		})
	}
}

func pytestInvocationProtocolScript(outcome string, exit int, fallback string, fallbackExit int) string {
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" >> fixture-args.txt\nreport=''\nfor arg in \"$@\"; do\n case \"$arg\" in --json-report-file=*) report=${arg#*=};; esac\ndone\n"
	script += "if [ -n \"$report\" ]; then\n printf '%s' \"$report\" > fixture-report-path.txt\n"
	if outcome != "" {
		script += " printf '%s' '" + pytestInvocationProtocolJSON(outcome, "tests/test_value.py::test_current") + "' > \"$report\"\n"
	}
	script += fmt.Sprintf(" printf 'current JSON protocol invocation\\n'\n exit %d\nfi\n", exit)
	script += "printf '%s\\n' '" + fallback + "'\n"
	return script + fmt.Sprintf("exit %d\n", fallbackExit)
}

func pytestInvocationProtocolJSON(outcome, nodeID string) string {
	if outcome == "zero" {
		return `{"exitcode":5,"summary":{"total":0},"tests":[]}`
	}
	exit := 0
	if outcome == "failed" {
		exit = 1
	}
	return fmt.Sprintf(`{"exitcode":%d,"summary":{"%s":1,"total":1},"tests":[{"nodeid":%q,"outcome":%q,"duration":0.125,"call":{"longrepr":"protocol assertion detail"}}]}`, exit, outcome, nodeID, outcome)
}

func TestPytestInvocationPublicPreservesMultipleSelectors(t *testing.T) {
	script := pytestInvocationProtocolScript("passed", 0, "", 0)
	twoRows := `{"exitcode":0,"summary":{"passed":2,"total":2},"tests":[{"nodeid":"tests/test_value.py::test_first","outcome":"passed"},{"nodeid":"tests/test_value.py::test_second","outcome":"passed"}]}`
	script = strings.Replace(script, pytestInvocationProtocolJSON("passed", "tests/test_value.py::test_current"), twoRows, 1)
	root := newPytestInvocationProtocolFixture(t, script)
	report := executePytestInvocationProtocol(t, root, "tests/test_value.py::test_first tests/test_value.py::test_second")
	if !report.Passed || len(report.TestResults) != 2 || report.TestResults[0].AssertionID != "test_first" || report.TestResults[1].AssertionID != "test_second" {
		t.Fatalf("all selected current rows must survive: %+v", report)
	}
	args, err := os.ReadFile(filepath.Join(root, "fixture-args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, selector := range []string{"tests/test_value.py::test_first", "tests/test_value.py::test_second"} {
		if strings.Count(string(args), "\n"+selector+"\n") != 1 {
			t.Errorf("selector must reach child as an independent argument: %q in %q", selector, args)
		}
	}
}

func TestPytestInvocationPublicConcurrentCallsOwnDifferentReports(t *testing.T) {
	script := "#!/bin/sh\nset -eu\nreport=''\nkey=''\nfor arg in \"$@\"; do\n case \"$arg\" in\n --json-report-file=*) report=${arg#*=};;\n tests/test_value.py::test_first) key=first;;\n tests/test_value.py::test_second) key=second;;\n esac\ndone\n[ -n \"$report\" ] && [ -n \"$key\" ] || exit 91\nprintf '%s' \"$report\" > \"fixture-report-$key.txt\"\n"
	script += "if [ \"$key\" = first ]; then\n printf '%s' '" + pytestInvocationProtocolJSON("passed", "tests/test_value.py::test_first") + "' > \"$report\"\nelse\n printf '%s' '" + pytestInvocationProtocolJSON("failed", "tests/test_value.py::test_second") + "' > \"$report\"\nfi\n"
	// Both current artifacts are published before either command completes.
	// The bounded barrier makes shared-file contamination deterministic, while
	// each process still has its own selector, exit code and recorded report path.
	script += ": > \"fixture-ready-$key\"\nattempt=0\nwhile [ ! -f fixture-ready-first ] || [ ! -f fixture-ready-second ]; do\n attempt=$((attempt + 1))\n [ \"$attempt\" -le 400 ] || exit 92\n sleep 0.01\ndone\n[ \"$key\" = first ] && exit 0\nexit 1\n"
	root := newPytestInvocationProtocolFixture(t, script)
	t.Cleanup(func() {
		first, err1 := os.ReadFile(filepath.Join(root, "fixture-report-first.txt"))
		second, err2 := os.ReadFile(filepath.Join(root, "fixture-report-second.txt"))
		if err1 != nil || err2 != nil || len(first) == 0 || len(second) == 0 || bytes.Equal(first, second) {
			t.Errorf("simultaneous calls must own distinct report paths: first=%q second=%q errors=%v/%v", first, second, err1, err2)
		}
	})
	for _, key := range []string{"first", "second"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			report := executePytestInvocationProtocol(t, root, "tests/test_value.py::test_"+key)
			if report.Passed != (key == "first") || len(report.TestResults) != 1 || report.TestResults[0].AssertionID != "test_"+key || report.TestResults[0].Passed != (key == "first") {
				t.Errorf("selector borrowed concurrent sibling evidence: key=%s report=%+v", key, report)
			}
		})
	}
}

func newPytestInvocationProtocolFixture(t *testing.T, script string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{".venv/bin", "tests"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		"pytest.ini":          "[pytest]\n",
		"tests/test_value.py": "def test_value():\n    assert 1 == 1\n",
		".venv/bin/python":    script,
	} {
		mode := os.FileMode(0o644)
		if path == ".venv/bin/python" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func executePytestInvocationProtocol(t *testing.T, root, suite string) *types.ChangeReport {
	t.Helper()
	mu := types.NewMutableState("pytest current invocation report ownership")
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, Language: "en"}
	_, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "python", "framework": "pytest", "suite": suite}))
	if err != nil {
		t.Fatal(err)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatal("public Execute did not install a report")
	}
	return report
}
