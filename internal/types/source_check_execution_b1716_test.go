package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestB1716LegacySourceCheckReportWithdrawsStoredAuthority(t *testing.T) {
	for _, status := range []VerificationStatus{VerificationStatusUnavailable, VerificationStatusFailed} {
		t.Run(string(status), func(t *testing.T) {
			report := &ChangeReport{
				PlanID: "source-plan", Passed: status != VerificationStatusFailed, VerificationStatus: status,
				ExecutedCommands:       []ExecutedCommand{{Runner: "node", Source: "syntax_preflight", Outcome: ExecutedCommandOutcomeSyntaxCheckFallback, ExitCode: 0, CoveredPaths: []string{"plan.js"}}},
				VerificationConfidence: []VerificationConfidenceRecord{{Source: "syntax_preflight", Category: "source_compile", Status: "satisfied", ReasonCode: "source_compile_ok"}},
				ChangedPathCoverage:    []ChangedPathVerificationCoverage{{Path: "plan.js", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationSourceCheck, Capability: VerificationCapabilitySyntaxOnly, Runner: "node", Source: "syntax_preflight"}},
			}
			if status == VerificationStatusUnavailable {
				report.NoTestsRunners = []string{"node"}
			}
			stored, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored ChangeReport
			if err := json.Unmarshal(stored, &restored); err != nil {
				t.Fatal(err)
			}
			effective := EffectiveVerificationProbeReport(nil, &restored)
			if effective.VerificationConfidence[0].Status == "satisfied" {
				t.Error("legacy zero exit without an execution receipt still grants source_compile satisfied")
			}
			row := effective.ChangedPathCoverage[0]
			if row.Status != ChangedPathVerificationUncovered || row.Capability != VerificationCapabilityUnknown {
				t.Errorf("legacy source-check coverage still has authority: %+v", row)
			}
			if effective.Passed != report.Passed || effective.VerificationStatus != status {
				t.Errorf("authority projection changed the overall verdict: %+v", effective)
			}
			profile := BuildVerificationProofProfile(nil, &restored)
			if profile.SyntaxOnlyPaths != 0 {
				t.Errorf("legacy report still counted syntax proof: %+v", profile)
			}
			again := EffectiveVerificationProbeReport(nil, effective)
			if !reflect.DeepEqual(effective, again) {
				t.Fatal("effective source authority is not idempotent")
			}
			after, err := json.Marshal(&restored)
			if err != nil {
				t.Fatal(err)
			}
			if string(after) != string(stored) {
				t.Fatal("effective view mutated the persisted report")
			}
		})
	}
}

func TestB1716SourceCheckExecutionReceiptQualification(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ExecutedCommand)
		want bool
	}{
		{"fallback", func(*ExecutedCommand) {}, true},
		{"preflight", func(c *ExecutedCommand) { c.Outcome = ExecutedCommandOutcomeSyntaxPreflight }, true},
		{"reordered exact set", func(c *ExecutedCommand) {
			c.CoveredPaths = []string{"b.js", "pkg/a.js"}
			c.SourceCheckExecution.CheckedPaths = []string{"pkg/a.js", "b.js"}
		}, true},
		{"missing legacy receipt", func(c *ExecutedCommand) { c.SourceCheckExecution = nil }, false},
		{"unknown version", func(c *ExecutedCommand) { c.SourceCheckExecution.Version++ }, false},
		{"unstarted", func(c *ExecutedCommand) { c.SourceCheckExecution.Started = false }, false},
		{"incomplete", func(c *ExecutedCommand) { c.SourceCheckExecution.Completed = false }, false},
		{"unknown exit", func(c *ExecutedCommand) { c.SourceCheckExecution.ExitCodeKnown = false }, false},
		{"failed receipt", func(c *ExecutedCommand) { c.SourceCheckExecution.ExitCode = 1 }, false},
		{"failed command", func(c *ExecutedCommand) { c.ExitCode = 1 }, false},
		{"ordinary project command", func(c *ExecutedCommand) { c.Outcome = ExecutedCommandOutcomeExecuted }, false},
		{"synthetic no tests", func(c *ExecutedCommand) { c.Outcome = ExecutedCommandOutcomeSyntheticNoTests }, false},
		{"empty scope is observation only", func(c *ExecutedCommand) { c.CoveredPaths = nil; c.SourceCheckExecution.CheckedPaths = nil }, false},
		{"scope disagreement", func(c *ExecutedCommand) { c.SourceCheckExecution.CheckedPaths = []string{"pkg/b.js"} }, false},
		{"case sensitive", func(c *ExecutedCommand) { c.SourceCheckExecution.CheckedPaths = []string{"pkg/A.js"} }, false},
		{"covered superset", func(c *ExecutedCommand) { c.CoveredPaths = append(c.CoveredPaths, "pkg/b.js") }, false},
		{"receipt superset", func(c *ExecutedCommand) {
			c.SourceCheckExecution.CheckedPaths = append(c.SourceCheckExecution.CheckedPaths, "pkg/b.js")
		}, false},
		{"absolute", func(c *ExecutedCommand) {
			c.CoveredPaths = []string{"/pkg/a.js"}
			c.SourceCheckExecution.CheckedPaths = c.CoveredPaths
		}, false},
		{"escape", func(c *ExecutedCommand) {
			c.CoveredPaths = []string{"../pkg/a.js"}
			c.SourceCheckExecution.CheckedPaths = c.CoveredPaths
		}, false},
		{"noncanonical", func(c *ExecutedCommand) {
			c.CoveredPaths = []string{"pkg/../a.js"}
			c.SourceCheckExecution.CheckedPaths = c.CoveredPaths
		}, false},
		{"windows absolute", func(c *ExecutedCommand) {
			c.CoveredPaths = []string{"C:/pkg/a.js"}
			c.SourceCheckExecution.CheckedPaths = c.CoveredPaths
		}, false},
		{"backslash", func(c *ExecutedCommand) {
			c.CoveredPaths = []string{`pkg\a.js`}
			c.SourceCheckExecution.CheckedPaths = c.CoveredPaths
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := b1716SuccessfulSourceCommand("pkg/a.js")
			tc.edit(&cmd)
			if got := SourceCheckCommandSucceeded(cmd); got != tc.want {
				t.Fatalf("succeeded=%v want=%v: %+v receipt=%+v", got, tc.want, cmd, cmd.SourceCheckExecution)
			}
			blob, err := json.Marshal(cmd)
			if err != nil {
				t.Fatal(err)
			}
			var restored ExecutedCommand
			if err := json.Unmarshal(blob, &restored); err != nil {
				t.Fatal(err)
			}
			if got := SourceCheckCommandSucceeded(restored); got != tc.want {
				t.Fatalf("JSON roundtrip succeeded=%v want=%v: %s", got, tc.want, blob)
			}
			var wire map[string]json.RawMessage
			if err := json.Unmarshal(blob, &wire); err != nil {
				t.Fatal(err)
			}
			if cmd.SourceCheckExecution != nil && len(wire["source_check_execution"]) == 0 {
				t.Fatal("receipt lost its public snake_case field")
			}
		})
	}
}

func TestB1716MixedPersistedSourceCheckAuthorityIsPerPath(t *testing.T) {
	good := b1716SuccessfulSourceCommand("pkg/a.js")
	failed := b1716SuccessfulSourceCommand("pkg/b.js")
	failed.ExitCode, failed.SourceCheckExecution.ExitCode = 1, 1
	unknown := b1716SuccessfulSourceCommand("pkg/c.js")
	unknown.SourceCheckExecution = nil
	coverage := func(path string) ChangedPathVerificationCoverage {
		return ChangedPathVerificationCoverage{Path: path, Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationSourceCheck, Capability: VerificationCapabilitySyntaxOnly, Runner: good.Runner, Source: good.Source}
	}
	confidence := func(path string) VerificationConfidenceRecord {
		return VerificationConfidenceRecord{Source: good.Source, Category: "source_compile", Status: "satisfied", ReasonCode: "source_compile_ok", ChangedSymbolRefs: []string{"path:" + path}}
	}
	report := &ChangeReport{
		PlanID: "source-plan", Passed: false, VerificationStatus: VerificationStatusFailed, BuildFailed: true,
		ExecutedCommands: []ExecutedCommand{good, failed, unknown},
		ChangedPathCoverage: []ChangedPathVerificationCoverage{coverage("pkg/a.js"), coverage("pkg/b.js"), coverage("pkg/c.js"),
			{Path: "native.go", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationProjectRunner, Capability: VerificationCapabilityTargetBehavior, Runner: "go"},
			{Path: "probe.rb", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationProbe, Capability: VerificationCapabilitySourceStatic, Runner: "verification_probe"},
			{Path: "meta.ts", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationDeclaredProjectCheck, Capability: VerificationCapabilitySyntaxOnly, Runner: "make"},
		},
		VerificationConfidence: []VerificationConfidenceRecord{confidence("pkg/a.js"), confidence("pkg/b.js"), confidence("pkg/c.js"),
			{Source: "project", Category: "project_test", Status: "satisfied"},
			{Source: good.Source, Category: "source_compile", Status: "satisfied", Detail: "all plan-touched source parsed"},
		},
	}
	stored, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored ChangeReport
	if err := json.Unmarshal(stored, &restored); err != nil {
		t.Fatal(err)
	}
	effective := EffectiveVerificationProbeReport(&ChangePlan{ID: report.PlanID}, &restored)
	for i, row := range effective.ChangedPathCoverage {
		wantCovered := i != 1 && i != 2
		if (row.Status == ChangedPathVerificationCovered) != wantCovered {
			t.Errorf("row %d has wrong path authority: %+v", i, row)
		}
		if i >= 3 && !reflect.DeepEqual(row, report.ChangedPathCoverage[i]) {
			t.Errorf("independent project/probe row changed: %+v", row)
		}
	}
	for i, rec := range effective.VerificationConfidence {
		wantSatisfied := i != 1 && i != 2
		if (rec.Status == "satisfied") != wantSatisfied {
			t.Errorf("confidence %d borrowed/lost sibling authority: %+v", i, rec)
		}
	}
	if effective.VerificationConfidence[4].Detail == report.VerificationConfidence[4].Detail {
		t.Fatal("generic old success retained a whole-plan claim")
	}
	if effective.Passed || effective.VerificationStatus != VerificationStatusFailed || !effective.BuildFailed {
		t.Fatal("successful sibling erased failed aggregate")
	}
	if !reflect.DeepEqual(effective, EffectiveVerificationProbeReport(nil, effective)) {
		t.Fatal("mixed effective projection is not idempotent")
	}
	after, err := json.Marshal(&restored)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(stored) {
		t.Fatal("mixed effective view mutated stored evidence")
	}
	foreign := EffectiveVerificationProbeReport(&ChangePlan{ID: "another-plan"}, &restored)
	if foreign.ChangedPathCoverage[0].Status == ChangedPathVerificationCovered || foreign.VerificationConfidence[0].Status == "satisfied" {
		t.Fatal("foreign plan borrowed a source-check receipt")
	}
}

func b1716SuccessfulSourceCommand(path string) ExecutedCommand {
	return ExecutedCommand{Runner: "node", Source: "syntax_preflight", Outcome: ExecutedCommandOutcomeSyntaxCheckFallback,
		ExitCode: 0, CoveredPaths: []string{path},
		SourceCheckExecution: &SourceCheckExecutionReceipt{Version: SourceCheckExecutionReceiptVersion, Started: true, Completed: true, ExitCodeKnown: true, ExitCode: 0, CheckedPaths: []string{path}},
	}
}

func TestB1716SourceCheckCoverageRequiresExactRowIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangedPathVerificationCoverage)
		want bool
	}{
		{"exact", func(*ChangedPathVerificationCoverage) {}, true},
		{"foreign source", func(r *ChangedPathVerificationCoverage) { r.Source = "other_attempt" }, false},
		{"foreign runner", func(r *ChangedPathVerificationCoverage) { r.Runner = "ruby" }, false},
		{"sibling path", func(r *ChangedPathVerificationCoverage) { r.Path = "pkg/b.js" }, false},
		{"path case", func(r *ChangedPathVerificationCoverage) { r.Path = "pkg/A.js" }, false},
		{"legacy empty caliber exact", func(r *ChangedPathVerificationCoverage) { r.Caliber = "" }, true},
		{"legacy empty caliber sibling", func(r *ChangedPathVerificationCoverage) { r.Caliber = ""; r.Path = "pkg/b.js" }, false},
		{"receipt cannot grant execution", func(r *ChangedPathVerificationCoverage) { r.Capability = VerificationCapabilityTargetExecution }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := b1716SuccessfulSourceCommand("pkg/a.js")
			row := ChangedPathVerificationCoverage{Path: "pkg/a.js", Status: ChangedPathVerificationCovered, Caliber: ChangedPathVerificationSourceCheck, Capability: VerificationCapabilitySyntaxOnly, Runner: cmd.Runner, Source: cmd.Source}
			tc.edit(&row)
			report := &ChangeReport{ExecutedCommands: []ExecutedCommand{cmd}, ChangedPathCoverage: []ChangedPathVerificationCoverage{row}}
			got := EffectiveChangedPathVerificationCoverage(nil, report)[0]
			if (got.Status == ChangedPathVerificationCovered) != tc.want {
				t.Fatalf("coverage=%+v want covered=%v", got, tc.want)
			}
			if tc.want && got.Capability != VerificationCapabilitySyntaxOnly {
				t.Fatalf("source receipt inflated to %+v", got)
			}
		})
	}
}

func TestB1716SourceCheckConfidenceUsesExactSuccessfulScopeUnion(t *testing.T) {
	a, b := b1716SuccessfulSourceCommand("pkg/a.js"), b1716SuccessfulSourceCommand("pkg/b.js")
	for _, tc := range []struct {
		name, source string
		refs         []string
		want         bool
	}{
		{"one path", a.Source, []string{"path:pkg/a.js"}, true},
		{"two successful siblings", a.Source, []string{"path:pkg/a.js", "path:pkg/b.js"}, true},
		{"unobserved sibling", a.Source, []string{"path:pkg/a.js", "path:pkg/c.js"}, false},
		{"foreign source", "another_source", []string{"path:pkg/a.js"}, false},
		{"source parsing is not symbol proof", a.Source, []string{"symbol:pkg/a.js:business"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report := &ChangeReport{ExecutedCommands: []ExecutedCommand{a, b}, VerificationConfidence: []VerificationConfidenceRecord{{Source: tc.source, Category: "source_compile", Status: "satisfied", ChangedSymbolRefs: tc.refs}}}
			got := EffectiveVerificationConfidence(nil, report)[0]
			if (got.Status == "satisfied") != tc.want {
				t.Fatalf("confidence=%+v want satisfied=%v", got, tc.want)
			}
		})
	}
}

func TestB1716SourceCheckProofCommandClassRequiresExecutedReceipt(t *testing.T) {
	for _, outcome := range []string{ExecutedCommandOutcomeSyntaxCheckFallback, ExecutedCommandOutcomeSyntaxPreflight} {
		for _, tc := range []struct {
			name string
			edit func(*ExecutedCommand)
			want VerificationProofRunnerEvidence
		}{
			{"completed success", func(*ExecutedCommand) {}, VerificationProofRunnerSyntaxFallback},
			{"completed failure", func(c *ExecutedCommand) { c.ExitCode = 2; c.SourceCheckExecution.ExitCode = 2 }, VerificationProofRunnerSyntaxFallback},
			{"completed unknown inputs", func(c *ExecutedCommand) { c.CoveredPaths = nil; c.SourceCheckExecution.CheckedPaths = nil }, VerificationProofRunnerSyntaxFallback},
			{"legacy", func(c *ExecutedCommand) { c.SourceCheckExecution = nil }, VerificationProofRunnerUnavailable},
			{"never started", func(c *ExecutedCommand) { c.SourceCheckExecution.Started = false }, VerificationProofRunnerUnavailable},
			{"interrupted", func(c *ExecutedCommand) { c.SourceCheckExecution.Completed = false }, VerificationProofRunnerUnavailable},
			{"unknown exit", func(c *ExecutedCommand) { c.SourceCheckExecution.ExitCodeKnown = false }, VerificationProofRunnerUnavailable},
			{"contradictory exit", func(c *ExecutedCommand) { c.ExitCode = 1 }, VerificationProofRunnerUnavailable},
		} {
			t.Run(outcome+"/"+tc.name, func(t *testing.T) {
				cmd := b1716SuccessfulSourceCommand("pkg/a.js")
				cmd.Outcome = outcome
				tc.edit(&cmd)
				if got := verificationProofCommandClass(cmd); got != tc.want {
					t.Fatalf("class=%s want=%s, receipt=%+v", got, tc.want, cmd.SourceCheckExecution)
				}
				report := &ChangeReport{Passed: true, NoTestsRunners: []string{"node"}, ExecutedCommands: []ExecutedCommand{cmd}}
				profile := BuildVerificationProofProfile(nil, report)
				wantCount := 0
				if tc.want == VerificationProofRunnerSyntaxFallback {
					wantCount = 1
				}
				if profile.ProjectRunnerCommands != 0 || profile.SyntaxFallbackCommands != wantCount || profile.SyntaxOnlyPaths != 0 {
					t.Fatalf("execution classification created authority/counts: %+v", profile)
				}
			})
		}
	}
}

func TestB1716UnstartedSourceCheckIsUnavailableBesidePassedProjectSuite(t *testing.T) {
	for _, outcome := range []string{ExecutedCommandOutcomeSyntaxCheckFallback, ExecutedCommandOutcomeSyntaxPreflight} {
		t.Run(outcome, func(t *testing.T) {
			missing := ExecutedCommand{Runner: "node", Source: "missing-source-tool", Outcome: outcome, ExitCode: -1, SourceCheckExecution: &SourceCheckExecutionReceipt{Version: SourceCheckExecutionReceiptVersion}}
			report := &ChangeReport{PlanID: "mixed-plan", Passed: true,
				TestResults:      []TestResult{{Suite: "go", AssertionID: "TestPassed", Passed: true, Kind: TestResultKindUnit}},
				ExecutedCommands: []ExecutedCommand{{Runner: "go", Source: "real-project-suite", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0}, missing},
			}
			if ExecutedCommandFailed(missing) {
				t.Error("unstarted source placeholder is a failed execution")
			}
			if got := ExecutedCommandUnavailableReasonCode(missing); got != string(FailureKindVerificationIncomplete) {
				t.Errorf("unstarted source reason=%q", got)
			}
			ledger := BuildVerificationProofLedger(nil, report, nil)
			seenProject, seenUnavailable := false, false
			for _, item := range ledger.Capabilities {
				if item.Kind != "executed_command" {
					continue
				}
				if item.Source == "missing-source-tool" {
					seenUnavailable = true
					if item.Status != VerificationProofLedgerItemUnavailable {
						t.Errorf("unstarted source capability=%+v", item)
					}
				}
				if item.Source == "real-project-suite" {
					seenProject = true
					if item.Status != VerificationProofLedgerItemCovered {
						t.Errorf("legitimate project success was erased: %+v", item)
					}
				}
			}
			if !seenProject || !seenUnavailable {
				t.Fatalf("missing mixed ledger entries: %+v", ledger)
			}
		})
	}
}

// A pre-receipt negative diagnostic stays negative. It is not upgraded into
// a proven execution, and cannot act as a permissive legacy success fallback.
func TestB1716LegacyNegativeSourceReportRetainedExecutionNotProven(t *testing.T) {
	cmd := ExecutedCommand{Runner: "node", Source: "syntax_preflight", Outcome: ExecutedCommandOutcomeSyntaxCheckFallback, ExitCode: 1}
	report := &ChangeReport{Passed: false, BuildFailed: true, VerificationStatus: VerificationStatusFailed, ExecutedCommands: []ExecutedCommand{cmd}}
	got := EffectiveVerificationProbeReport(nil, report)
	if got.NormalizeVerificationStatus() != VerificationStatusFailed || !ExecutedCommandFailed(cmd) {
		t.Fatal("legacy negative report was erased")
	}
	if SourceCheckCommandSucceeded(cmd) || verificationProofCommandClass(cmd) != VerificationProofRunnerUnavailable {
		t.Fatal("legacy negative report was promoted to a proven execution")
	}
	if BuildVerificationProofProfile(nil, got).Status != VerificationProofFailed {
		t.Fatal("legacy report failure disappeared from proof profile")
	}
}

func TestB1716CommandLedgerKeepsTypedExecutorScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ExecutedCommand)
	}{
		{"runner", func(c *ExecutedCommand) { c.Runner = "another-runner" }},
		{"framework", func(c *ExecutedCommand) { c.Framework = "another-framework" }},
		{"working directory", func(c *ExecutedCommand) { c.WorkingDir = "pkg/b" }},
		{"empty field positions", func(c *ExecutedCommand) { c.Framework, c.WorkingDir = c.WorkingDir, c.Framework }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := ExecutedCommand{Runner: "go", WorkingDir: "pkg/a", Command: "same invocation", Source: "auto_detect", Suite: "check", Outcome: ExecutedCommandOutcomeExecuted, ExitCode: 0}
			b := a
			tc.edit(&b)
			b.ExitCode = 17
			report := &ChangeReport{PlanID: "typed-executor-scope", Passed: false, ExecutedCommands: []ExecutedCommand{a, a, b}}
			ledger := BuildVerificationProofLedger(nil, report, nil)
			goodID, badID := verificationProofCommandLedgerID(report.PlanID, a), verificationProofCommandLedgerID(report.PlanID, b)
			if goodID == badID {
				t.Fatal("different typed executor scopes share a ledger ID")
			}
			seen := map[string]VerificationProofLedgerItemStatus{}
			for _, item := range ledger.Capabilities {
				if item.Kind == "executed_command" {
					seen[item.ID] = item.Status
				}
			}
			if len(seen) != 2 || seen[goodID] != VerificationProofLedgerItemCovered || seen[badID] != VerificationProofLedgerItemFailed {
				t.Fatalf("ledger lost scope or same-identity dedup: %+v", seen)
			}
		})
	}
}
