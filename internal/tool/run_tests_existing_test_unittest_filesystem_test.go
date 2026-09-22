package tool

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

const existingTestFilesystemReport = `{"rows":[],"overflow":false,"successful":true,"tests_run":0}`

func existingTestFilesystemInvocation(t *testing.T, testBodies ...string) (*types.BusContext, *existingTestUnittestInvocation, string) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	root := t.TempDir()
	testBody := "import unittest\n"
	if len(testBodies) > 0 {
		testBody = testBodies[0]
	}
	for file, body := range map[string]string{"widget.py": "def increment(x):\n    return x + 1\n", "test_widget.py": testBody} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root, Mode: types.ModeApply, PipelineStage: types.StageVerify, Mutable: types.NewMutableState("execute test")}
	plan := &types.ChangePlan{ID: "existing-test-filesystem", WriteAnalysisIR: &types.WriteAnalysisIR{Request: types.WriteRequestModel{Constraints: []types.WriteConstraint{{Kind: types.WriteConstraintRunExistingTest, Target: "test_widget.py"}}}}}
	b1575BindAppliedPythonLines(t, ctx, plan, "widget.py", []int{2})
	run, command := prepareExistingTestUnittestInvocation(ctx, runnerPlan{Runner: "python", Framework: "unittest", Root: root, Suite: "test_widget.py"})
	if run == nil {
		t.Fatal("actual invocation preparation failed")
	}
	return ctx, run, command
}

func TestExistingTestUnittestObserverWriteFailurePreservesNativeVerdict(t *testing.T) {
	for _, kind := range []string{"directory", "existing_file"} {
		for _, fails := range []bool{false, true} {
			name := kind + "_native_pass"
			if fails {
				name = kind + "_native_fail"
			}
			t.Run(name, func(t *testing.T) {
				create := "pathlib.Path(sys.argv[1]).mkdir()"
				if kind == "existing_file" {
					create = "pathlib.Path(sys.argv[1]).write_text('not an observation')"
				}
				assertion := "self.assertTrue(True)"
				if fails {
					assertion = "self.fail('actual native failure')"
				}
				body := "import unittest, pathlib, sys\nclass Tests(unittest.TestCase):\n    def test_result(self):\n        " + create + "\n        " + assertion + "\n"
				ctx, run, command := existingTestFilesystemInvocation(t, body)
				defer run.cleanup()
				// Use this prepared invocation's exact destination, not a second
				// model-provided command or an unrelated native test result.
				cmd := NewShellCommandContext(ctx.Context(), command)
				cmd.Dir = ctx.RepoRoot
				output, runErr := cmd.CombinedOutput()
				exitCode := 0
				if runErr != nil {
					exitCode = 1
				}
				if (runErr != nil) != fails {
					t.Errorf("observer replaced native exit: error=%v output=%s", runErr, output)
				}
				if strings.Count(string(output), "[codrax unittest] execution observation could not be written") != 1 || strings.Contains(string(output), "FileExistsError") {
					t.Errorf("write failure did not stay a bounded observation warning: %s", output)
				}
				if report, err := run.readReport(ctx, exitCode, string(output), runErr); err == nil || report != nil {
					t.Fatal("unwritten native observation was accepted")
				}
				fallback, err := parseUnittestOutput(string(output), runErr)
				if err != nil || fallback.Passed == fails {
					t.Errorf("native fallback verdict changed: %+v %v", fallback, err)
				}
				if len(run.rows) != 0 {
					t.Fatal("write failure published typed observer rows")
				}
				run.cleanup()
				if _, err := os.Lstat(run.reportPath); err != nil {
					t.Errorf("cleanup removed the entry that prevented observer creation: %v", err)
				}
			})
		}
	}
}

type existingTestFilesystemCountingReader struct{ read int }

func (r *existingTestFilesystemCountingReader) Read(p []byte) (int, error) {
	left := 3*existingTestUnittestMaxReportBytes - r.read
	if left <= 0 {
		return 0, io.EOF
	}
	if len(p) > left {
		p = p[:left]
	}
	for i := range p {
		p[i] = ' '
	}
	r.read += len(p)
	return len(p), nil
}

func TestExistingTestUnittestActualReadBound(t *testing.T) {
	reader := &existingTestFilesystemCountingReader{}
	if data, err := readExistingTestUnittestBounded(reader); err == nil || data != nil {
		t.Fatal("oversized stream accepted")
	}
	if reader.read != existingTestUnittestMaxReportBytes+1 {
		t.Fatalf("read consumed %d bytes beyond its bound", reader.read)
	}
	if data, err := readExistingTestUnittestBounded(strings.NewReader(strings.Repeat(" ", existingTestUnittestMaxReportBytes))); err != nil || len(data) != existingTestUnittestMaxReportBytes {
		t.Fatalf("exact limit changed: len=%d err=%v", len(data), err)
	}
}

func TestExistingTestUnittestFilesystemOwnership(t *testing.T) {
	for _, scenario := range []string{"normal", "directory_symlink", "directory_replaced", "file_symlink", "file_replaced_after_read", "unexpected_extra_file", "oversized_report"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, run, _ := existingTestFilesystemInvocation(t)
			write := func(path, body string) {
				t.Helper()
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			outside := t.TempDir()
			victim := filepath.Join(outside, "result.json")
			write(victim, existingTestFilesystemReport)
			wantRead := scenario == "normal" || scenario == "file_replaced_after_read" || scenario == "unexpected_extra_file"
			switch scenario {
			case "directory_symlink", "directory_replaced":
				if err := os.Rename(run.directory, run.directory+"-original"); err != nil {
					t.Fatal(err)
				}
				if scenario == "directory_symlink" {
					if err := os.Symlink(outside, run.directory); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Mkdir(run.directory, 0700); err != nil {
						t.Fatal(err)
					}
					write(run.reportPath, existingTestFilesystemReport)
				}
			case "file_symlink":
				if err := os.Symlink(victim, run.reportPath); err != nil {
					t.Fatal(err)
				}
			case "oversized_report":
				write(run.reportPath, strings.Repeat(" ", (2<<20)+1))
			default:
				write(run.reportPath, existingTestFilesystemReport)
			}
			report, err := run.readReport(ctx, 0, "Ran 0 tests in 0.001s\nOK\n", nil)
			if (err == nil && report != nil) != wantRead {
				t.Errorf("read accepted=%v want=%v err=%v", err == nil && report != nil, wantRead, err)
			}
			if scenario == "file_replaced_after_read" {
				if err := os.Rename(run.reportPath, run.reportPath+"-original"); err != nil {
					t.Fatal(err)
				}
				write(run.reportPath, "replacement belongs to another producer")
			}
			if scenario == "unexpected_extra_file" {
				write(filepath.Join(run.directory, "unrelated.txt"), "must survive")
			}
			run.cleanup()
			data, err := os.ReadFile(victim)
			if err != nil || string(data) != existingTestFilesystemReport {
				t.Errorf("cleanup touched external report: data=%q err=%v", data, err)
			}
			switch scenario {
			case "normal":
				if _, err := os.Lstat(run.directory); !os.IsNotExist(err) {
					t.Errorf("normal owned directory not removed: %v", err)
				}
			case "directory_symlink", "directory_replaced", "file_symlink", "file_replaced_after_read":
				if _, err := os.Lstat(run.reportPath); err != nil {
					t.Errorf("cleanup removed replacement path: %v", err)
				}
			case "unexpected_extra_file":
				if data, err := os.ReadFile(filepath.Join(run.directory, "unrelated.txt")); err != nil || string(data) != "must survive" {
					t.Errorf("cleanup removed unrelated file: %q %v", data, err)
				}
				if _, err := os.Lstat(run.reportPath); !os.IsNotExist(err) {
					t.Errorf("owned report not removed: %v", err)
				}
			}
		})
	}
}
