package types

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Archived capability labels are projections, not new executor receipts.
// This historical-report matrix deliberately does not infer behavior from
// command text, make dependency names, a declared driver language, or prose.
func TestB1678StoredOpaqueRunnerCapabilityDoesNotGrantExecution(t *testing.T) {
	for _, tc := range []struct {
		name, runner     string
		capability, want VerificationCapability
	}{
		{"make old behavior", "make", VerificationCapabilityTargetBehavior, VerificationCapabilityUnknown},
		{"make old execution", "make", VerificationCapabilityTargetExecution, VerificationCapabilityUnknown},
		{"make missing capability", "make", "", VerificationCapabilityUnknown},
		{"make unknown", "make", VerificationCapabilityUnknown, VerificationCapabilityUnknown},
		{"existing cross-language static", "make", VerificationCapabilitySourceStatic, VerificationCapabilitySourceStatic},
		{"syntax observation", "make", VerificationCapabilitySyntaxOnly, VerificationCapabilitySyntaxOnly},
		{"native python unchanged", "python", VerificationCapabilityTargetBehavior, VerificationCapabilityTargetBehavior},
		{"native Go unchanged", "go", VerificationCapabilityTargetBehavior, VerificationCapabilityTargetBehavior},
		{"native CTest unchanged", "cmake", VerificationCapabilityTargetBehavior, VerificationCapabilityTargetBehavior},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &ChangeReport{
				PlanID: "historical-plan", Passed: true,
				TestResults:         []TestResult{{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAggregate, Suite: "check", AssertionID: "suite-result", Passed: true}},
				ExecutedCommands:    []ExecutedCommand{{Runner: tc.runner, WorkingDir: ".", Suite: "check", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0, Command: "make behavior_verified_target_execution", Source: "target_behavior", CoveredPaths: []string{"widget.py"}}},
				ChangedPathCoverage: []ChangedPathVerificationCoverage{{Path: "widget.py", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationProjectRunner, Capability: tc.capability, Runner: tc.runner, Source: "declared_coverage_test_surface", LanguageFamilies: []VerificationLanguageFamily{VerificationLanguagePython}}},
			}
			before, _ := json.Marshal(report)
			rows := EffectiveChangedPathVerificationCoverage(nil, report)
			if len(rows) != 1 || rows[0].Capability != tc.want || rows[0].Status != ChangedPathVerificationCovered {
				t.Errorf("effective capability=%+v want=%v without losing declared path check", rows, tc.want)
			}
			wantExecution := tc.want == VerificationCapabilityTargetBehavior || tc.want == VerificationCapabilityTargetExecution
			if got := report.HasTargetExecutionCoverage(); got != wantExecution {
				t.Errorf("execution=%v want=%v", got, wantExecution)
			}
			profile := BuildVerificationProofProfile(nil, report)
			ledger := BuildVerificationProofLedger(nil, report, nil)
			if !wantExecution && (profile.Status == VerificationProofStrong || ledger.State == VerificationProofLedgerVerified) {
				t.Errorf("unknown/static archived evidence became verified: profile=%+v ledger=%+v", profile, ledger)
			}
			after, _ := json.Marshal(report)
			if !bytes.Equal(before, after) {
				t.Error("projection altered process, assertion, scope or stored capability")
			}
		})
	}
}

func TestB1678HistoricalNativeRecoveryRequiresExactRecordedScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ExecutedCommand)
		want bool
	}{
		{"exact native scope", func(*ExecutedCommand) {}, true},
		{"failed native command", func(c *ExecutedCommand) { c.ExitCode = 1 }, false},
		{"timeout", func(c *ExecutedCommand) { c.Outcome = ExecutedCommandOutcomeTimeout }, false},
		{"skipped suite", func(c *ExecutedCommand) { c.Outcome = ExecutedCommandOutcomeSuiteSkipped }, false},
		{"missing scope receipt", func(c *ExecutedCommand) { c.CoveredPaths = nil }, false},
		{"sibling path", func(c *ExecutedCommand) { c.CoveredPaths = []string{"pkg/sibling.py"} }, false},
		{"different working directory", func(c *ExecutedCommand) { c.WorkingDir = "other" }, false},
		{"directory prefix is not containment", func(c *ExecutedCommand) { c.WorkingDir = "pk" }, false},
		{"escaping directory", func(c *ExecutedCommand) { c.WorkingDir = "../pkg" }, false},
		{"absolute directory", func(c *ExecutedCommand) { c.WorkingDir = "/pkg" }, false},
		{"incompatible language", func(c *ExecutedCommand) { c.Runner, c.Framework = "go", "" }, false},
		{"unknown runner cannot borrow framework", func(c *ExecutedCommand) { c.Runner, c.Framework = "future-runner", "python" }, false},
		{"plain probe is not native protocol", func(c *ExecutedCommand) { c.Runner, c.Framework = "verification_probe", "python" }, false},
		{"second opaque aggregate", func(c *ExecutedCommand) { c.Runner, c.Framework = "make", "" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := ExecutedCommand{Runner: "python", Framework: "unittest", WorkingDir: "pkg", Suite: "tests/test_widget.py", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0, CoveredPaths: []string{"pkg/widget.py"}, Command: "arbitrary text must not establish authority"}
			tc.edit(&command)
			report := &ChangeReport{PlanID: "known-plan", Passed: true,
				ExecutedCommands:    []ExecutedCommand{command},
				ChangedPathCoverage: []ChangedPathVerificationCoverage{{Path: "pkg/widget.py", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior, Runner: "make"}},
			}
			for _, plan := range []*ChangePlan{nil, {ID: report.PlanID}, {ID: "foreign-plan"}} {
				want := tc.want && (plan == nil || plan.ID == report.PlanID)
				before, _ := json.Marshal(report)
				rows := EffectiveChangedPathVerificationCoverage(plan, report)
				if got := rows[0].Capability == VerificationCapabilityTargetBehavior; got != want {
					t.Errorf("native scope recovery=%v want=%v plan=%+v rows=%+v command=%+v", got, want, plan, rows, command)
				}
				after, _ := json.Marshal(report)
				if !bytes.Equal(before, after) {
					t.Error("historical source report mutated")
				}
			}
		})
	}
}
