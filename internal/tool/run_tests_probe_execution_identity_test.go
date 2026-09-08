package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The display command deliberately hides probe source. It is not executable
// identity: reusing a planner ID with a different comparator is a new probe.
func TestB1616ProbeChangedDefinitionDoesNotSupersedeHistoricalFailure(t *testing.T) {
	root := t.TempDir()
	if !VerificationProbeRuntimeAvailable("python", root) {
		t.Skip("Python runtime unavailable")
	}
	probe := types.VerificationProbe{ID: "same-id", Language: "python", WorkingDir: ".", Code: "assert False", TimeoutSeconds: 10}
	old := b1616RunProbe(t, root, "old", probe)
	probe.Code = "assert True"
	current := b1616RunProbe(t, root, "new", probe)
	if old.Passed || !current.Passed || len(old.ExecutedCommands) != 1 || len(current.ExecutedCommands) != 1 {
		t.Fatalf("expected real failed then passed probe executions: old=%+v new=%+v", old, current)
	}
	if old.ExecutedCommands[0].Command != current.ExecutedCommands[0].Command {
		t.Fatal("fixture must exercise the same display command")
	}
	b1616AssertHistoricalCommand(t, old, current, false)
}

func b1616RunProbe(t *testing.T, root, planID string, probe types.VerificationProbe) *types.ChangeReport {
	return b1616RunProbeInDomain(t, root, root, planID, probe)
}

func b1616RunProbeInDomain(t *testing.T, root, domain, planID string, probe types.VerificationProbe) *types.ChangeReport {
	t.Helper()
	mu := types.NewMutableState("probe identity")
	mu.SetChangePlan(&types.ChangePlan{ID: planID, VerificationProbes: []types.VerificationProbe{probe}})
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: domain}
	res, ok := runPlanVerificationProbes(ctx, "pre_suite_verification_probe")
	if !ok || res == nil || res.Report == nil {
		t.Fatal("actual probe producer did not produce a report")
	}
	return res.Report
}

func b1616AssertHistoricalCommand(t *testing.T, old, current *types.ChangeReport, superseded bool) {
	t.Helper()
	ledger := types.BuildVerificationProofLedger(nil, current, []types.VerificationProofArtifact{{Report: old}})
	failed := map[string]bool{}
	for _, cmd := range old.ExecutedCommands {
		if types.ExecutedCommandFailed(cmd) {
			failed[cmd.Command] = true
		}
	}
	for _, item := range ledger.Capabilities {
		if item.ReportPlanID == old.PlanID && item.Kind == "executed_command" && failed[item.Detail] {
			got := item.ReasonCode == "superseded_by_terminal_exact_command_pass"
			if got != superseded {
				t.Fatalf("historical probe exact-rerun=%v, want %v: %+v", got, superseded, item)
			}
			return
		}
	}
	t.Fatal("historical executed command disappeared")
}

func TestB1616ActualProbeDefinitionAxesAndReceiptHistory(t *testing.T) {
	root := t.TempDir()
	if !VerificationProbeRuntimeAvailable("python", root) {
		t.Skip("Python runtime unavailable")
	}
	ready := filepath.Join(root, "ready")
	probe := types.VerificationProbe{
		ID: "p", Language: "python", WorkingDir: ".", TimeoutSeconds: 10,
		Code:         fmt.Sprintf("from pathlib import Path\nassert Path(%q).exists()\nprint('ready')", ready),
		ContractRefs: []string{"c1"}, ChangedSymbolRefs: []string{"path:src.py"},
	}
	old := b1616RunProbe(t, root, "old", probe)
	if old.Passed || old.ExecutedCommands[0].ProbeExecution == nil {
		t.Fatal("expected real terminal failed receipt")
	}
	if err := os.WriteFile(ready, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*types.VerificationProbe)
		same   bool
	}{
		{"same_definition", func(*types.VerificationProbe) {}, true},
		{"id", func(p *types.VerificationProbe) { p.ID = "other" }, false},
		{"code", func(p *types.VerificationProbe) { p.Code += "\n# changed definition" }, false},
		{"language_spelling", func(p *types.VerificationProbe) { p.Language = "py" }, false},
		{"working_dir", func(p *types.VerificationProbe) { p.WorkingDir = "child" }, false},
		{"timeout", func(p *types.VerificationProbe) { p.TimeoutSeconds = 11 }, false},
		{"expected_stdout", func(p *types.VerificationProbe) { p.ExpectedStdout = []string{"ready"} }, false},
		{"contract", func(p *types.VerificationProbe) { p.ContractRefs = []string{"c2"} }, false},
		{"target_path", func(p *types.VerificationProbe) { p.ChangedSymbolRefs = []string{"path:other.py"} }, false},
		{"placement", func(p *types.VerificationProbe) { p.PlacementRefs = []string{"p2"} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := probe
			tc.change(&candidate)
			current := b1616RunProbe(t, root, "new", candidate)
			if !current.Passed {
				t.Fatalf("expected successful current execution: %+v", current)
			}
			b1616AssertHistoricalCommand(t, old, current, tc.same)
		})
	}
	current := b1616RunProbe(t, root, "new", probe)
	for _, name := range []string{"old_receipt_missing", "new_receipt_missing", "both_legacy", "incomplete", "future_version", "not_terminal", "same_execution", "older_execution", "failed_command", "unavailable_command"} {
		t.Run(name, func(t *testing.T) {
			clone := func(report *types.ChangeReport) *types.ChangeReport {
				data, err := json.Marshal(report)
				if err != nil {
					t.Fatal(err)
				}
				var out types.ChangeReport
				if err := json.Unmarshal(data, &out); err != nil {
					t.Fatal(err)
				}
				return &out
			}
			history, terminal := clone(old), clone(current)
			switch name {
			case "old_receipt_missing":
				history.ExecutedCommands[0].ProbeExecution = nil
			case "new_receipt_missing":
				terminal.ExecutedCommands[0].ProbeExecution = nil
			case "both_legacy":
				history.ExecutedCommands[0].ProbeExecution = nil
				terminal.ExecutedCommands[0].ProbeExecution = nil
			case "incomplete":
				terminal.ExecutedCommands[0].ProbeExecution.InvocationSHA256 = ""
			case "future_version":
				terminal.ExecutedCommands[0].ProbeExecution.Version++
			case "not_terminal":
				terminal.ExecutedCommands[0].ProbeExecution.FinishedAt = terminal.ExecutedCommands[0].ProbeExecution.StartedAt.Add(-1)
			case "same_execution":
				terminal.ExecutedCommands[0].ProbeExecution.ExecutionID = history.ExecutedCommands[0].ProbeExecution.ExecutionID
			case "older_execution":
				terminal.ExecutedCommands[0].ProbeExecution.StartedAt = history.ExecutedCommands[0].ProbeExecution.StartedAt.Add(-1)
			case "failed_command":
				terminal.ExecutedCommands[0].ExitCode = 1
			case "unavailable_command":
				terminal.ExecutedCommands[0].Outcome = types.ExecutedCommandOutcomeRunnerMissing
			}
			b1616AssertHistoricalCommand(t, history, terminal, false)
		})
	}
}

func TestB1616ActualProbeRerunsAcrossTemporaryArtifactsAndWorktrees(t *testing.T) {
	for _, tc := range []struct{ name, language, code string }{
		{"python", "python", "from pathlib import Path\nassert Path('ready').exists()"},
		{"javascript", "javascript", "if (!require('fs').existsSync('ready')) throw new Error('not ready')"},
		{"ruby", "ruby", "raise 'not ready' unless File.exist?('ready')"},
		{"java", "java", "if (!java.nio.file.Files.exists(java.nio.file.Paths.get(\"ready\"))) throw new AssertionError(\"not ready\");"},
		{"go_source", "go", "package main\nimport \"os\"\nfunc main() { if _, err := os.Stat(\"ready\"); err != nil { panic(err) } }"},
		{"go_overlay", "go", "package example\nimport (\"os\"; \"testing\")\nfunc TestReady(t *testing.T) { if _, err := os.Stat(\"ready\"); err != nil { t.Fatal(err) } }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			domain, first, second := t.TempDir(), t.TempDir(), t.TempDir()
			if !VerificationProbeRuntimeAvailable(tc.language, first) {
				t.Skip("runtime unavailable")
			}
			for _, root := range []string{first, second} {
				if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n\ngo 1.22\n"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package example\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			probe := types.VerificationProbe{ID: "p", Language: tc.language, WorkingDir: ".", Code: tc.code, TimeoutSeconds: 30}
			old := b1616RunProbeInDomain(t, first, domain, "old", probe)
			if old.FailureKind == types.FailureKindRunnerMissing {
				t.Skipf("runtime advertised executable but actual producer reports unavailable: %s", old.FailureReasonCode)
			}
			if old.Passed {
				t.Fatal("missing ready must fail")
			}
			if err := os.WriteFile(filepath.Join(second, "ready"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			current := b1616RunProbeInDomain(t, second, domain, "new", probe)
			if !current.Passed {
				t.Fatalf("ready must pass: %+v", current)
			}
			b1616AssertHistoricalCommand(t, old, current, true)
			// No explicit common repository domain means no cross-root proof.
			unknownDomain := b1616RunProbeInDomain(t, second, "", "other", probe)
			b1616AssertHistoricalCommand(t, old, unknownDomain, false)
		})
	}
}

func TestB1616ActualBaselineExpectationRemainsAuxiliary(t *testing.T) {
	root := t.TempDir()
	if !VerificationProbeRuntimeAvailable("python", root) {
		t.Skip("Python unavailable")
	}
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root}
	probe := types.VerificationProbe{ID: "p", Language: "python", Code: "assert True", ExpectsBaselineFailure: true}
	before := runSingleVerificationProbe(ctx, probe, verificationProbeBaselineSource)
	if !before.Report.Passed {
		t.Fatalf("expected real baseline process success: %+v", before)
	}
	baseline := verificationProbeBaselineCommand(probe.ID, before)
	if baseline.Outcome != types.ExecutedCommandOutcomeExpectedFailureNotObserved || baseline.ProbeExecution == nil {
		t.Fatal("expected actual baseline projection to retain receipt and fail its expectation")
	}
	old := &types.ChangeReport{PlanID: "old", Passed: false, VerificationStatus: types.VerificationStatusFailed, ExecutedCommands: []types.ExecutedCommand{baseline}}
	after := runSingleVerificationProbe(ctx, probe, "pre_suite_verification_probe")
	current := after.Report
	current.PlanID = "new"
	current.ExecutedCommands = after.Commands
	if !current.Passed {
		t.Fatal("expected post-apply pass")
	}
	if types.ExecutedCommandFailed(baseline) {
		t.Fatal("baseline expectation is a separate confidence lane, not a command failure")
	}
	ledger := types.BuildVerificationProofLedger(nil, current, []types.VerificationProofArtifact{{Report: old}})
	for _, item := range ledger.Capabilities {
		if item.ReportPlanID == "old" && item.Kind == "executed_command" {
			if item.ReasonCode == "superseded_by_terminal_exact_command_pass" || item.Status != types.VerificationProofLedgerItemCovered {
				t.Fatalf("baseline auxiliary capability changed: %+v", item)
			}
			return
		}
	}
	t.Fatal("baseline auxiliary command disappeared")
}
