package types

import (
	"strings"
	"testing"
	"time"
)

func TestB1616LegacyProbeIdentityNeverUsesPlaceholder(t *testing.T) {
	for _, cmd := range []ExecutedCommand{
		{Runner: "verification_probe", Command: "opaque", Outcome: ExecutedCommandOutcomeExecuted},
		{Runner: "python", Source: "pre_suite_verification_probe", Command: "opaque", Outcome: ExecutedCommandOutcomeExecuted},
		{Runner: "python", Suite: "verification_probe/python", Command: "opaque", Outcome: ExecutedCommandOutcomeExecuted},
	} {
		if got := verificationProofCommandIdentity(cmd); got != "" {
			t.Fatalf("legacy probe acquired exact identity: %q", got)
		}
	}
	for _, runner := range []string{"make", "cmake", "cargo", "pytest"} {
		cmd := ExecutedCommand{Runner: runner, WorkingDir: ".", Suite: "test", Command: runner + " test", Outcome: ExecutedCommandOutcomeExecuted}
		want := verificationProofLedgerStableID("command_identity", cmd.Runner, cmd.Framework, cmd.WorkingDir, cmd.Suite, cmd.Command)
		if got := verificationProofCommandIdentity(cmd); got != want {
			t.Fatalf("ordinary project identity changed: %q", got)
		}
	}
}

func TestB1616ProbeCapabilityRowsRetainDistinctActualExecutions(t *testing.T) {
	now := time.Now()
	receipt := VerificationProbeExecutionReceipt{Version: 1, DefinitionSHA256: strings.Repeat("a", 64), InvocationSHA256: strings.Repeat("b", 64), ExecutionID: "first", StartedAt: now, FinishedAt: now.Add(time.Second), RepositoryRoot: "/repo", Executable: "/python", Args: []string{"python", "-c", "wrapper"}, WorkingDir: "/repo"}
	a := ExecutedCommand{Runner: "verification_probe", Source: "pre_suite_verification_probe", Command: "identical display", Outcome: ExecutedCommandOutcomeExecuted, ProbeExecution: &receipt}
	second := receipt
	second.ExecutionID = "second"
	second.DefinitionSHA256 = strings.Repeat("c", 64)
	b := a
	b.ProbeExecution = &second
	report := &ChangeReport{PlanID: "current", Passed: true, ExecutedCommands: []ExecutedCommand{a, b}}
	ledger := BuildVerificationProofLedger(nil, report, nil)
	count := 0
	for _, item := range ledger.Capabilities {
		if item.Kind == "executed_command" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("same display swallowed distinct receipts: %+v", ledger.Capabilities)
	}
}
