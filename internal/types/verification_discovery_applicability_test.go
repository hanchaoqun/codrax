package types

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func withEmptyDefaultDiscovery(r *ChangeReport) {
	for i := range r.ExistingTestExecutions {
		id := r.ExecutedCommands[i].InvocationID
		if id == "" {
			id = r.ExistingTestExecutions[i].TestPath
		}
		r.ExecutedCommands[i].InvocationID, r.TestResults[i].InvocationID = id, id
	}
	r.TestSurface.Candidates = append(r.TestSurface.Candidates, TestSurfaceCandidate{
		ID: "python/unittest@.", Runner: "python", Framework: "unittest", WorkingDir: ".", HasTestSignal: true, Source: "test-file convention",
	})
	r.ExecutedCommands = append(r.ExecutedCommands, ExecutedCommand{
		Runner: "python", Framework: "unittest", WorkingDir: ".", Source: "test_surface_default",
		InvocationID: "default-discovery", Command: "display-only discovery command", Outcome: ExecutedCommandOutcomeZeroTests,
	})
}

func TestVerificationLedgerScopedProofWithEmptyDefaultDiscovery(t *testing.T) {
	for _, registered := range []bool{false, true} {
		name := "explicit_existing_file"
		if registered {
			name = "read_only_registration"
		}
		t.Run(name, func(t *testing.T) {
			p, r := existingIntentConsumerFixture()
			if registered {
				p, r = nativeRegistrationProofFixture(t)
			}
			withEmptyDefaultDiscovery(r)
			p, r = b1575PairConflictReload(t, p, r)
			before := existingIntentConsumerBytes(t, []any{p, r})
			got := BuildVerificationProofLedger(p, r, nil)
			if got.CapabilityUnavailableCount != 0 || !verificationProofLedgerHasCapability(got, "executed_command", VerificationProofLedgerItemAdvisory, "default_discovery_no_additional_requirement") {
				t.Errorf("unscoped convention discovery vetoed exact report-local native proof: %+v", got.Capabilities)
			}
			if registered && !BehaviorContractRefHasVerificationWitness(p, r, "value") {
				t.Fatal("native contract receipt was lost")
			}
			if after := existingIntentConsumerBytes(t, []any{p, r}); after != before {
				t.Fatal("ledger rewrote the native report or plan")
			}
		})
	}
}

func TestVerificationLedgerEmptyDefaultDiscoveryStillRequiresExactScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *ChangeReport, *ExecutedCommand, *TestSurfaceCandidate)
	}{
		{"no_requirement", func(p *ChangePlan, _ *ChangeReport, _ *ExecutedCommand, _ *TestSurfaceCandidate) {
			p.WriteAnalysisIR = nil
		}},
		{"no_receipt", func(_ *ChangePlan, r *ChangeReport, _ *ExecutedCommand, _ *TestSurfaceCandidate) {
			r.ExistingTestExecutions = nil
		}},
		{"stale_generation", func(p *ChangePlan, _ *ChangeReport, _ *ExecutedCommand, _ *TestSurfaceCandidate) {
			p.AppliedCommitSHA = "next-apply"
		}},
		{"another_report", func(_ *ChangePlan, r *ChangeReport, _ *ExecutedCommand, _ *TestSurfaceCandidate) { r.PlanID = "other" }},
		{"another_lane", func(_ *ChangePlan, r *ChangeReport, _ *ExecutedCommand, _ *TestSurfaceCandidate) { r.Channel = "" }},
		{"partial_scope", func(_ *ChangePlan, r *ChangeReport, _ *ExecutedCommand, _ *TestSurfaceCandidate) {
			r.ExistingTestExecutions = r.ExistingTestExecutions[:1]
		}},
		{"explicit_origin", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			cmd.Source = "llm_choice"
		}},
		{"explicit_suite", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			cmd.Suite = "tests"
		}},
		{"configured_discovery", func(_ *ChangePlan, _ *ChangeReport, _ *ExecutedCommand, c *TestSurfaceCandidate) {
			c.Source = "pyproject.toml"
		}},
		{"declared_check", func(_ *ChangePlan, _ *ChangeReport, _ *ExecutedCommand, c *TestSurfaceCandidate) {
			c.DeclaredCoveragePaths = []string{"test_top.py"}
		}},
		{"candidate_ambiguity", func(_ *ChangePlan, r *ChangeReport, _ *ExecutedCommand, c *TestSurfaceCandidate) {
			other := *c
			other.Source = "setup.py"
			r.TestSurface.Candidates = append(r.TestSurface.Candidates, other)
		}},
		{"sibling_discovery", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, c *TestSurfaceCandidate) {
			cmd.WorkingDir, c.WorkingDir = "packages/wid", "packages/wid"
		}},
		{"same_package", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			cmd.WorkingDir = "packages/widget"
		}},
		{"parser_failure", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			cmd.Outcome = ExecutedCommandOutcomeParserError
		}},
		{"failed_exit", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) { cmd.ExitCode = 1 }},
		{"missing_invocation", func(_ *ChangePlan, _ *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			cmd.InvocationID = ""
		}},
		{"reused_invocation", func(_ *ChangePlan, r *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			cmd.InvocationID = r.ExecutedCommands[0].InvocationID
		}},
		{"unexpected_result", func(_ *ChangePlan, r *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			r.TestResults = append(r.TestResults, TestResult{InvocationID: cmd.InvocationID, Suite: "unscoped skip"})
		}},
		{"ambiguous_ledger_key", func(_ *ChangePlan, r *ChangeReport, cmd *ExecutedCommand, _ *TestSurfaceCandidate) {
			other := *cmd
			other.InvocationID = "another-default-discovery"
			r.ExecutedCommands = append(r.ExecutedCommands, other)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := existingIntentConsumerFixture("packages/widget/tests/test_a.py", "packages/widget/tests/test_b.py")
			withEmptyDefaultDiscovery(r)
			tc.edit(p, r, &r.ExecutedCommands[len(r.ExecutedCommands)-1], &r.TestSurface.Candidates[len(r.TestSurface.Candidates)-1])
			got := BuildVerificationProofLedger(p, r, nil)
			if got.CapabilityUnavailableCount == 0 || verificationProofLedgerHasCapability(got, "executed_command", VerificationProofLedgerItemAdvisory, "default_discovery_no_additional_requirement") {
				t.Fatalf("scope or execution debt was hidden: %+v", got.Capabilities)
			}
		})
	}
}

func TestVerificationLedgerEmptyDefaultDiscoverySealedNativeReplay(t *testing.T) {
	dir := os.Getenv("CODRAX_HMC373_NATIVE_ARCHIVE")
	if dir == "" {
		t.Skip("requires the sealed previous real write-eval native reports")
	}
	load := func(name string, value any) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, value); err != nil {
			t.Fatal(err)
		}
	}
	var source, followup ChangePlan
	var sourceReport, followupReport ChangeReport
	load("0127-plan-1794206006699178000-15143.json", &source)
	load("0128-plan-1794206006699178000-15143.report.json", &sourceReport)
	load("0131-plan-1794206101074717000-15143.json", &followup)
	load("0132-plan-1794206101074717000-15143.report.json", &followupReport)
	for _, pair := range []VerificationProofArtifact{{Plan: &source, Report: &sourceReport}, {Plan: &followup, Report: &followupReport}} {
		got := BuildVerificationProofLedger(pair.Plan, pair.Report, nil)
		if got.CapabilityUnavailableCount != 0 || !verificationProofLedgerHasCapability(got, "executed_command", VerificationProofLedgerItemAdvisory, "default_discovery_no_additional_requirement") {
			t.Errorf("report %s loses exact nested execution: %+v", pair.Plan.ID, got.Capabilities)
		}
	}
	got := BuildVerificationProofLedger(&followup, &followupReport, []VerificationProofArtifact{{Plan: &source, Report: &sourceReport}})
	if got.CapabilityUnavailableCount != 0 || got.State != VerificationProofLedgerVerified {
		t.Fatalf("real cumulative report remains falsely unavailable: %s, %d (%v)", got.State, got.CapabilityUnavailableCount, got.ReasonCodes)
	}
}
