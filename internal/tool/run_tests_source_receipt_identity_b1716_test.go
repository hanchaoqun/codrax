package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1716GoSourceCheckKeepsPackageExecutionIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("controlled Go process fixture requires /bin/sh")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"a-pass/main.go", "b-fail/main.go", "c-unchecked/main.go"}
	for _, path := range paths {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("package fixture\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	invocations := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("B1716_IDENTITY_PASS_DIR", filepath.Join(root, "a-pass"))
	t.Setenv("B1716_IDENTITY_FAIL_DIR", filepath.Join(root, "b-fail"))
	t.Setenv("B1716_IDENTITY_LOG", invocations)
	// This process controls only the execution receipt. It is not a Go
	// compiler oracle and does not claim the package's exact source inputs.
	script := `#!/bin/sh
if [ "$#" -ne 2 ] || [ "$1" != test ] || [ "$2" != -json ]; then
  exit 96
fi
printf '%s\n' "$PWD" >> "$B1716_IDENTITY_LOG"
if [ "$PWD" = "$B1716_IDENTITY_PASS_DIR" ]; then
  printf '%s\n' '{"Action":"skip","Package":"example.com/fixture/a-pass"}'
  exit 0
fi
if [ "$PWD" = "$B1716_IDENTITY_FAIL_DIR" ]; then
  printf '# example.com/fixture/b-fail\n./main.go:3:1: undefined: missingValue\nFAIL\texample.com/fixture/b-fail [build failed]\n'
  exit 17
fi
exit 97
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root}
	// Reverse inputs to ensure identity is tied to each actual directory,
	// rather than positional association with the plan's target paths.
	report, output := runGoCompileFallback(ctx, "go@.", root, []string{
		filepath.Join(root, paths[1]), filepath.Join(root, paths[0]),
	})
	if report == nil || report.Passed || !report.BuildFailed {
		t.Fatalf("actual nonzero package result was lost: report=%+v output=%s", report, output)
	}
	observed, err := os.ReadFile(invocations)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "a-pass") + "\n" + filepath.Join(root, "b-fail") + "\n"; string(observed) != want {
		t.Fatalf("unexpected real execution directories: got %q want %q", observed, want)
	}
	if len(report.ExecutedCommands) != 2 {
		t.Fatalf("want both leaf executions, got %+v", report.ExecutedCommands)
	}
	for i, dir := range []string{"a-pass", "b-fail"} {
		if got := report.ExecutedCommands[i].WorkingDir; got != filepath.Join(root, dir) {
			t.Errorf("leaf command %d lost actual cwd: %q", i, got)
		}
	}

	plan := &types.ChangePlan{ID: "b1716-go-package-identity", Status: types.PlanStatusApplied, TargetPaths: paths}
	runner := runnerPlan{Runner: "go", Root: root}
	for _, outcome := range []string{types.ExecutedCommandOutcomeSyntaxCheckFallback, types.ExecutedCommandOutcomeSyntaxPreflight} {
		t.Run(outcome, func(t *testing.T) {
			projected := *report
			projected.PlanID = plan.ID
			projected.ExecutedCommands = sourceCheckCommandsForReport(root, runner, "syntax_preflight", outcome, report)
			if len(projected.ExecutedCommands) != 2 {
				t.Fatalf("wrapper merged package executions: %+v", projected.ExecutedCommands)
			}
			if projected.ExecutedCommands[0].Command != projected.ExecutedCommands[1].Command {
				t.Fatalf("fixture must execute identical command text in different packages: %+v", projected.ExecutedCommands)
			}
			for i, dir := range []string{"a-pass", "b-fail"} {
				cmd := projected.ExecutedCommands[i]
				wantExit := []int{0, 17}[i]
				if cmd.WorkingDir != dir || cmd.ExitCode != wantExit || cmd.Outcome != outcome {
					t.Errorf("wrapper changed real package identity/result: %+v", cmd)
				}
				r := cmd.SourceCheckExecution
				if r == nil || !r.Started || !r.Completed || !r.ExitCodeKnown || r.ExitCode != wantExit {
					t.Errorf("real process receipt lost its completed exit: %+v", cmd)
				}
				if len(cmd.CoveredPaths) != 0 || (r != nil && len(r.CheckedPaths) != 0) || types.SourceCheckCommandSucceeded(cmd) {
					t.Errorf("package compilation without exact input manifest minted path authority: %+v", cmd)
				}
				for _, confidence := range verificationConfidenceFromCommand(cmd, types.VerificationStatusFailed) {
					if confidence.Category == "source_compile" && confidence.Status == "satisfied" {
						t.Errorf("unknown package inputs minted exact-source confidence: %+v", confidence)
					}
				}
			}
			applyChangedPathVerificationCoverageForPlan(ctx, plan, &projected, false)
			if len(projected.ChangedPathCoverage) != len(paths) {
				t.Fatalf("missing plan coverage rows: %+v", projected.ChangedPathCoverage)
			}
			for _, row := range projected.ChangedPathCoverage {
				if row.Status != types.ChangedPathVerificationUncovered {
					t.Errorf("unknown package input domain covered a plan path: %+v", row)
				}
			}
			ledger := types.BuildVerificationProofLedger(plan, &projected, nil)
			commands := make(map[string]types.VerificationProofLedgerItem)
			for _, item := range ledger.Capabilities {
				if item.Kind == "executed_command" {
					commands[item.Path] = item
				}
			}
			if len(commands) != 2 {
				t.Fatalf("ledger merged same-command/different-package receipts: %+v", ledger.Capabilities)
			}
			good, bad := commands["a-pass"], commands["b-fail"]
			if good.Status != types.VerificationProofLedgerItemCovered || bad.Status != types.VerificationProofLedgerItemFailed {
				t.Errorf("ledger borrowed or erased a package exit status: good=%+v bad=%+v", good, bad)
			}
			if good.ID == bad.ID || strings.TrimSpace(good.EvidenceRef) == "" || good.EvidenceRef == bad.EvidenceRef {
				t.Errorf("ledger execution identities collide across actual cwd: good=%+v bad=%+v", good, bad)
			}
		})
	}
}
