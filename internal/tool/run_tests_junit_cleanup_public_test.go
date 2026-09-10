package tool

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This runs the real RunTests child-process/CTest report path with a bounded
// protocol fixture, not native CTest or compiled C assertions. Every directory
// the child can rename or link is owned by this test. The external report is
// deliberately outside the invocation's directory and must survive all Execute
// defers, not merely junitInvocation.Cleanup called in isolation.
func TestB1651CTestPublicCleanupDoesNotFollowReplacedOutputDirectory(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
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
	victim := filepath.Join(outside, "ctest.xml")
	old := []byte(`<testsuite name="external"><testcase classname="ExternalTest" name="stale_pass"/></testsuite>`)
	if err := os.WriteFile(victim, old, 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Unix(946684800, 0)
	if err := os.Chtimes(victim, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("B1651_CLEANUP_FIXTURE_ROOT", canonicalRoot)
	t.Setenv("B1651_CLEANUP_FIXTURE_OUTSIDE", outside)
	write("bin/ctest", `#!/bin/sh
set -eu
report=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output-junit ]; then
    shift
    report=$1
  fi
  shift
done
case "$report" in
  "$B1651_CLEANUP_FIXTURE_ROOT"/.codrax/tmp/junit-invocation-*/ctest.xml) ;;
  *) exit 91 ;;
esac
owned=${report%/ctest.xml}
test -d "$owned"
test ! -L "$owned"
printf '%s' "$report" > fixture-report-path.txt
mv "$owned" "$owned-original"
ln -s "$B1651_CLEANUP_FIXTURE_OUTSIDE" "$owned"
printf '%s\n' fixture-replaced-private-output-directory
`, 0o755)
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	mut := types.NewMutableState("CTest output ownership cleanup")
	ctx := &types.BusContext{Mutable: mut, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	_, err = (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": "cmake"}))
	if err != nil {
		t.Fatal(err)
	}
	pathBytes, err := os.ReadFile(filepath.Join(root, "fixture-report-path.txt"))
	if err != nil {
		t.Fatalf("actual CTest protocol child did not receive the system output path: %v", err)
	}
	reportPath := string(pathBytes)
	if !strings.HasPrefix(reportPath, filepath.Join(canonicalRoot, ".codrax", "tmp", "junit-invocation-")) || filepath.Base(reportPath) != "ctest.xml" {
		t.Fatalf("child did not receive a private invocation output: %q", reportPath)
	}
	owned := filepath.Dir(reportPath)
	if target, err := os.Readlink(owned); err != nil || target != outside {
		t.Fatalf("the fixture did not replace the owned directory as intended: target=%q err=%v", target, err)
	}
	if _, err := os.Stat(owned + "-original"); err != nil {
		t.Fatalf("the original empty owned directory was not retained by the fixture: %v", err)
	}
	data, readErr := os.ReadFile(victim)
	info, statErr := os.Stat(victim)
	if readErr != nil || statErr != nil || !bytes.Equal(data, old) || !info.ModTime().Equal(oldTime) {
		t.Errorf("public Execute cleanup deleted or changed external ctest.xml: read=%v stat=%v bytes=%q", readErr, statErr, data)
	}
	report := mut.ChangeReport()
	if report == nil {
		t.Fatal("public RunTests did not install a verification report")
	}
	if report.Passed || report.BuildFailed || len(report.TestResults) != 0 || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable || report.FailureReasonCode != "junit_current_report_unavailable" {
		t.Errorf("replaced output directory borrowed an external result or invented build failure: %+v", report)
	}
	executed := false
	for _, command := range report.ExecutedCommands {
		if command.Runner == "cmake" && command.Outcome == types.ExecutedCommandOutcomeExecuted && command.ExitCode == 0 && strings.Contains(command.Command, reportPath) {
			executed = true
		}
	}
	if !executed {
		t.Errorf("fixture's successful actual command receipt was lost: %+v", report.ExecutedCommands)
	}
}
