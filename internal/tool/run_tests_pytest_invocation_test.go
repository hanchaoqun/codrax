package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

const pytestHelperGreenJSON = `{"exitcode":0,"summary":{"passed":1,"total":1},"tests":[{"nodeid":"tests/test_value.py::Values::test_current","outcome":"passed","duration":0.125}]}`

func newPytestHelperInvocation(t *testing.T, root string) (*pytestInvocation, string) {
	t.Helper()
	plan := runnerPlan{Runner: "python", Framework: pythonFrameworkPytest, Root: root, Suite: "tests/test_value.py"}
	preview, extra := buildRunCommandForPlan(plan, plan.Suite, root)
	inv, command, bound, err := preparePytestRunnerInvocation(plan, preview, extra)
	if err != nil || inv == nil || bound != inv.reportPath || strings.Contains(command, "--json-report-file="+fmt.Sprintf("%q", extra)) {
		t.Fatalf("prepare did not bind the actual builder output: inv=%+v command=%q bound=%q err=%v", inv, command, bound, err)
	}
	return inv, command
}

func pytestHelperWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPytestInvocationHelperBuilderAndIsolation(t *testing.T) {
	root := t.TempDir()
	plan := runnerPlan{Runner: "python", Framework: pythonFrameworkPytest, Root: root, Suite: "tests/test_value.py"}
	for i := 0; i < 2; i++ {
		command, extra := buildRunCommandForPlan(plan, plan.Suite, root)
		if command == "" || extra != filepath.Join(root, ".codrax-pytest-report.json") {
			t.Fatal("builder preview changed")
		}
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("pure builder allocated files: %v %v", entries, err)
	}
	legacy := filepath.Join(root, ".codrax-pytest-report.json")
	pytestHelperWrite(t, legacy, "existing project report")
	oldTime := time.Unix(946684800, 0)
	if err := os.Chtimes(legacy, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	first, firstCommand := newPytestHelperInvocation(t, root)
	second, secondCommand := newPytestHelperInvocation(t, root)
	defer first.cleanup()
	defer second.cleanup()
	if first.directory == second.directory || first.reportPath == second.reportPath || firstCommand == secondCommand {
		t.Fatal("two actual invocation allocations shared their output")
	}
	for _, inv := range []*pytestInvocation{first, second} {
		info, err := os.Lstat(inv.directory)
		if err != nil || !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm() != 0700) {
			t.Fatalf("not a private invocation directory: %v %v", info, err)
		}
		if _, err := os.Stat(inv.reportPath); !os.IsNotExist(err) {
			t.Fatalf("prepare precreated a report: %v", err)
		}
		if report, digest, err := inv.readReport("", firstCommand, nil); err == nil || report != nil || digest != "" {
			t.Fatal("missing current report borrowed the project report")
		}
		pytestHelperWrite(t, inv.reportPath, pytestHelperGreenJSON)
		// Identical current bytes (even an old mtime) are legal. Ownership is
		// the fresh invocation path, not content novelty or a timestamp guess.
		if err := os.Chtimes(inv.reportPath, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
		report, digest, err := inv.readReport("", firstCommand, nil)
		expected := sha256.Sum256([]byte(pytestHelperGreenJSON))
		if err != nil || report == nil || !report.Passed || len(report.TestResults) != 1 || digest != hex.EncodeToString(expected[:]) {
			t.Fatalf("fresh same-byte report: %+v %q %v", report, digest, err)
		}
		row := report.TestResults[0]
		if row.Suite != "tests/test_value.py::Values" || row.AssertionID != "test_current" || row.Duration != 125*time.Millisecond || row.ObservationScope != types.TestObservationScopeAssertion {
			t.Fatalf("parsed different bytes: %+v", row)
		}
	}
	first.cleanup()
	if _, err := os.Stat(first.directory); !os.IsNotExist(err) {
		t.Fatalf("first owned directory survived: %v", err)
	}
	if data, err := os.ReadFile(second.reportPath); err != nil || string(data) != pytestHelperGreenJSON {
		t.Fatalf("first cleanup touched second invocation: %q %v", data, err)
	}
	second.cleanup()
	if data, err := os.ReadFile(legacy); err != nil || string(data) != "existing project report" {
		t.Fatalf("cleanup touched legacy report: %q %v", data, err)
	}
	if info, err := os.Stat(legacy); err != nil || !info.ModTime().Equal(oldTime) {
		t.Fatalf("legacy timestamp changed: %v %v", info, err)
	}
}

func TestPytestInvocationHelperPreparationBoundaries(t *testing.T) {
	for _, scenario := range []string{"unittest_passthrough", "missing_argument", "duplicate_argument", "codrax_symlink", "tmp_symlink"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			plan := runnerPlan{Runner: "python", Framework: pythonFrameworkPytest, Root: root}
			command, extra := buildRunCommandForPlan(plan, "", root)
			switch scenario {
			case "unittest_passthrough":
				plan.Framework = pythonFrameworkUnittest
			case "missing_argument":
				command = "python -m pytest"
			case "duplicate_argument":
				command += " --json-report-file=" + fmt.Sprintf("%q", extra)
			case "codrax_symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, ".codrax")); err != nil {
					t.Fatal(err)
				}
			case "tmp_symlink":
				if err := os.Mkdir(filepath.Join(root, ".codrax"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), filepath.Join(root, ".codrax", "tmp")); err != nil {
					t.Fatal(err)
				}
			}
			inv, bound, boundExtra, err := preparePytestRunnerInvocation(plan, command, extra)
			if scenario == "unittest_passthrough" {
				if inv != nil || err != nil || bound != command || boundExtra != extra {
					t.Fatal("unrelated framework changed")
				}
				return
			}
			if inv != nil || err == nil {
				t.Fatalf("unsafe/malformed allocation accepted: %+v %v", inv, err)
			}
		})
	}
}

func TestPytestInvocationHelperFilesystemOwnership(t *testing.T) {
	for _, scenario := range []string{"directory_symlink", "directory_replaced", "report_symlink", "report_is_directory", "malformed_file", "unread_file", "read_then_replace_report", "read_then_symlink_report", "read_then_replace_directory", "extra_file"} {
		t.Run(scenario, func(t *testing.T) {
			inv, command := newPytestHelperInvocation(t, t.TempDir())
			outside := t.TempDir()
			victim := filepath.Join(outside, "report.json")
			pytestHelperWrite(t, victim, pytestHelperGreenJSON)
			wantRead := strings.HasPrefix(scenario, "read_then_") || scenario == "extra_file"
			switch scenario {
			case "directory_symlink", "directory_replaced":
				if err := os.Rename(inv.directory, inv.directory+"-original"); err != nil {
					t.Fatal(err)
				}
				if scenario == "directory_symlink" {
					if err := os.Symlink(outside, inv.directory); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Mkdir(inv.directory, 0700); err != nil {
						t.Fatal(err)
					}
					pytestHelperWrite(t, inv.reportPath, pytestHelperGreenJSON)
				}
			case "report_symlink":
				if err := os.Symlink(victim, inv.reportPath); err != nil {
					t.Fatal(err)
				}
			case "report_is_directory":
				if err := os.Mkdir(inv.reportPath, 0700); err != nil {
					t.Fatal(err)
				}
			case "malformed_file":
				pytestHelperWrite(t, inv.reportPath, "not pytest JSON")
			default:
				pytestHelperWrite(t, inv.reportPath, pytestHelperGreenJSON)
			}
			if scenario != "unread_file" {
				report, digest, err := inv.readReport("", command, nil)
				if (err == nil && report != nil) != wantRead {
					t.Errorf("read accepted=%v want=%v err=%v", err == nil && report != nil, wantRead, err)
				}
				if !wantRead && digest != "" {
					t.Error("rejected bytes gained a digest receipt")
				}
			}
			switch scenario {
			case "read_then_replace_report", "read_then_symlink_report":
				if err := os.Rename(inv.reportPath, inv.reportPath+"-original"); err != nil {
					t.Fatal(err)
				}
				if scenario == "read_then_symlink_report" {
					if err := os.Symlink(victim, inv.reportPath); err != nil {
						t.Fatal(err)
					}
				} else {
					pytestHelperWrite(t, inv.reportPath, "another producer's file")
				}
			case "read_then_replace_directory":
				if err := os.Rename(inv.directory, inv.directory+"-original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(inv.directory, 0700); err != nil {
					t.Fatal(err)
				}
				pytestHelperWrite(t, inv.reportPath, "another producer's file")
			case "extra_file":
				pytestHelperWrite(t, filepath.Join(inv.directory, "unrelated.txt"), "must survive")
			}
			inv.cleanup()
			if data, err := os.ReadFile(victim); err != nil || string(data) != pytestHelperGreenJSON {
				t.Errorf("cleanup touched external bytes: %q %v", data, err)
			}
			if scenario == "extra_file" {
				if data, err := os.ReadFile(filepath.Join(inv.directory, "unrelated.txt")); err != nil || string(data) != "must survive" {
					t.Errorf("cleanup removed extra file: %q %v", data, err)
				}
				if _, err := os.Lstat(inv.reportPath); !os.IsNotExist(err) {
					t.Errorf("owned report not cleaned: %v", err)
				}
			} else if _, err := os.Lstat(inv.reportPath); err != nil {
				t.Errorf("cleanup removed unknown/replaced entry: %v", err)
			}
		})
	}
}

func TestPytestInvocationHelperBoundedSparseReport(t *testing.T) {
	inv, command := newPytestHelperInvocation(t, t.TempDir())
	file, err := os.OpenFile(inv.reportPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(pytestInvocationMaxBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if report, digest, err := inv.readReport("", command, nil); err == nil || report != nil || digest != "" {
		t.Fatal("oversized sparse report accepted")
	}
	inv.cleanup()
	if info, err := os.Stat(inv.reportPath); err != nil || info.Size() != pytestInvocationMaxBytes+1 {
		t.Fatalf("unknown oversized file removed: %v %v", info, err)
	}
}

// These subprocesses implement pytest's report-file/exit-code protocol; they do
// not install pytest, execute Python assertions, or stand in for Run/Execute.
func TestPytestInvocationHelperCurrentExitProtocol(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("native protocol shell unavailable")
	}
	for _, tc := range []struct {
		name                        string
		jsonExit, actualExit, total int
		wantPass, wantNoTests       bool
	}{
		{"zero_match_2", 2, 2, 0, true, true}, {"zero_match_4", 4, 4, 0, true, true}, {"zero_match_5", 5, 5, 0, true, true},
		{"zero_json_0_current_1", 0, 1, 0, false, true}, {"zero_json_5_current_1", 5, 1, 0, false, true},
		{"green_current_0", 0, 0, 1, true, false}, {"green_current_1", 0, 1, 1, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, command := newPytestHelperInvocation(t, t.TempDir())
			defer inv.cleanup()
			data := fmt.Sprintf(`{"exitcode":%d,"summary":{"total":0},"tests":[]}`, tc.jsonExit)
			if tc.total == 1 {
				data = pytestHelperGreenJSON
			}
			// Include whitespace so the digest must describe the read bytes, not
			// a re-serialized interpretation of their fields.
			data = " \n" + data + "\n"
			cmd := exec.Command("sh", "-c", `printf '%s' "$2" > "$1"; exit "$3"`, "pytest-protocol", inv.reportPath, data, fmt.Sprint(tc.actualExit))
			output, runErr := cmd.CombinedOutput()
			if extractExitCode(runErr) != tc.actualExit {
				t.Fatalf("native protocol exit mismatch: %v %s", runErr, output)
			}
			report, digest, err := inv.readReport(string(output), command, runErr)
			expected := sha256.Sum256([]byte(data))
			if err != nil || report == nil || digest != hex.EncodeToString(expected[:]) {
				t.Fatalf("current protocol read: %+v %q %v", report, digest, err)
			}
			if report.Passed != tc.wantPass || len(report.TestResults) != tc.total || (len(report.NoTestsRunners) > 0) != tc.wantNoTests {
				t.Fatalf("current process/JSON mismatch: %+v", report)
			}
			if !tc.wantPass && (report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable || report.FailureReasonCode != "pytest_current_command_failed" || report.FailureKind != types.FailureKindVerificationIncomplete) {
				t.Fatalf("nonzero current process became green/test failure: %+v", report)
			}
			if tc.total == 1 && (!report.TestResults[0].Passed || report.TestResults[0].ObservationScope != types.TestObservationScopeAssertion) {
				t.Fatal("nonzero command fabricated a failed assertion")
			}
			if actual, err := os.ReadFile(inv.reportPath); err != nil || !bytes.Equal(actual, []byte(data)) {
				t.Fatal("reader changed native bytes")
			}
		})
	}
}
