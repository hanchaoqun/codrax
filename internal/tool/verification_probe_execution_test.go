package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1616ProbeActualInvocationAxes(t *testing.T) {
	root := t.TempDir()
	if !VerificationProbeRuntimeAvailable("python", root) {
		t.Skip("Python unavailable")
	}
	interp := pythonRuntimeInterpreter(root, root, root)
	resolved, err := exec.LookPath(interp)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alternate", filepath.Base(resolved))
	if err := os.Mkdir(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(resolved, alias); err != nil {
		t.Skipf("executable symlink unavailable: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	probe := types.VerificationProbe{ID: "p", Language: "python", Code: "assert True", TimeoutSeconds: 10}
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root}
	run := func(t *testing.T, binary string, args []string, wd string, timeout time.Duration, p types.VerificationProbe) *types.VerificationProbeExecutionReceipt {
		t.Helper()
		execCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		cmd := exec.CommandContext(execCtx, binary, args...)
		cmd.Dir = wd
		res := runExternalVerificationProbe(ctx, p, externalVerificationProbeInput{
			ID: p.ID, Language: p.Language, Command: cmd, ExecCtx: execCtx, Timeout: timeout,
			WorkingDir: ".", Source: "pre_suite_verification_probe", CommandText: "same display",
		})
		if !res.Report.Passed || res.Commands[0].ProbeExecution == nil {
			t.Fatalf("real process did not pass: %+v", res)
		}
		return res.Commands[0].ProbeExecution
	}
	base := run(t, interp, []string{"-c", probe.Code}, root, 10*time.Second, probe)
	for _, tc := range []struct {
		name, binary, wd string
		args             []string
		timeout          time.Duration
	}{
		{"same", interp, root, []string{"-c", probe.Code}, 10 * time.Second},
		{"argv", interp, root, []string{"-B", "-c", probe.Code}, 10 * time.Second},
		{"executable_path", alias, root, []string{"-c", probe.Code}, 10 * time.Second},
		{"working_dir", interp, filepath.Join(root, "child"), []string{"-c", probe.Code}, 10 * time.Second},
		{"actual_timeout", interp, root, []string{"-c", probe.Code}, 9 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := run(t, tc.binary, tc.args, tc.wd, tc.timeout, probe)
			if (got.InvocationSHA256 == base.InvocationSHA256) != (tc.name == "same") {
				t.Fatalf("invocation identity axis not preserved: base=%+v got=%+v", base, got)
			}
			if got.ExecutionID == base.ExecutionID {
				t.Fatal("execution instance must be fresh")
			}
		})
	}
}

func TestB1616ProbeFailedStartAndConfigProduceNoReceipt(t *testing.T) {
	root := t.TempDir()
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root}
	probe := types.VerificationProbe{ID: "p", Language: "python", Code: "assert True"}
	execCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := exec.CommandContext(execCtx, filepath.Join(root, "missing-executable"))
	cmd.Dir = root
	res := runExternalVerificationProbe(ctx, probe, externalVerificationProbeInput{ID: "p", Language: "python", Command: cmd, ExecCtx: execCtx, Timeout: time.Second, WorkingDir: ".", Source: "pre_suite_verification_probe"})
	if res.Report.Passed || res.Commands[0].ProbeExecution != nil {
		t.Fatal("failed start must not mint a receipt")
	}
	probe.WorkingDir = "../outside"
	config := runSingleVerificationProbe(ctx, probe, "pre_suite_verification_probe")
	if config.Report.Passed || config.Commands[0].ProbeExecution != nil {
		t.Fatal("configuration refusal must not mint a receipt")
	}
}

// This checks the Java producer's two real subprocess handoffs without
// claiming that a host lacking a JDK executed Java bytecode. The fake compiler
// writes its -d artifact; the fake runtime checks that artifact and ready file.
func TestB1616JavaProducerTemporaryRolesWithHermeticProcesses(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("POSIX process fixture")
	}
	bin := t.TempDir()
	compiler := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-d" ]; then shift; out="$1"; fi
  shift
done
test -n "$out" || exit 4
touch "$out/CodraxVerificationProbe.class"
`
	runtime := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-cp" ]; then shift; classpath="$1"; fi
  shift
done
out="${classpath%%:*}"
test -f "$out/CodraxVerificationProbe.class" || exit 4
test -f ready
`
	for name, source := range map[string]string{"javac": compiler, "java": runtime} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(source), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	domain, first, second := t.TempDir(), t.TempDir(), t.TempDir()
	probe := types.VerificationProbe{ID: "p", Language: "java", WorkingDir: ".", Code: "assert true;", TimeoutSeconds: 10}
	old := b1616RunProbeInDomain(t, first, domain, "old", probe)
	if old.Passed || len(old.ExecutedCommands) != 2 {
		t.Fatalf("expected compiler pass and runtime failure: %+v", old)
	}
	if err := os.WriteFile(filepath.Join(second, "ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	current := b1616RunProbeInDomain(t, second, domain, "new", probe)
	if !current.Passed || len(current.ExecutedCommands) != 2 {
		t.Fatalf("expected two successful real processes: %+v", current)
	}
	b1616AssertHistoricalCommand(t, old, current, true)
	for i := range old.ExecutedCommands {
		a, b := old.ExecutedCommands[i].ProbeExecution, current.ExecutedCommands[i].ProbeExecution
		if a == nil || b == nil || a.InvocationSHA256 != b.InvocationSHA256 || a.WorkingDir == b.WorkingDir || a.ExecutionID == b.ExecutionID {
			t.Fatalf("Java leg %d lost stable role or actual audit identity", i)
		}
	}
}

func TestB1616GeneratedJavaWrapperDigestIsExecutionIdentity(t *testing.T) {
	root := t.TempDir()
	if !VerificationProbeRuntimeAvailable("python", root) {
		t.Skip("Python unavailable for the real process identity fixture")
	}
	probe := types.VerificationProbe{ID: "p", Language: "java", Code: "assert true;"}
	sourceCode := javaVerificationProbeSource(probe.Code)
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root}
	var previous *types.VerificationProbeExecutionReceipt
	for _, generated := range []string{sourceCode, sourceCode + "\n// updated generated wrapper\n"} {
		// Only the generated source changes. This fixture checks the common
		// receipt key with a real terminal process, not Java language behavior.
		cmd := exec.Command(pythonRuntimeInterpreter(root, root, root), "-c", "pass")
		cmd.Dir = root
		start := time.Now()
		err := cmd.Run()
		if err != nil {
			t.Fatal(err)
		}
		receipt := verificationProbeExecutionReceipt(ctx, probe, cmd, time.Second, start, err, verificationProbeInvocationRoles{GeneratedSourceSHA256: verificationProbeExecutionDigest(generated)})
		if receipt == nil {
			t.Fatal("terminal receipt absent")
		}
		if previous != nil {
			if previous.DefinitionSHA256 != receipt.DefinitionSHA256 {
				t.Fatal("fixture must preserve planner definition")
			}
			if previous.InvocationSHA256 == receipt.InvocationSHA256 {
				t.Fatal("different generated Java wrapper bytes collapsed to the same execution identity")
			}
		}
		previous = receipt
	}
}
