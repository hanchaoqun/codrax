package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Run the public tool and actual Ruby process: ordinary probe definitions must
// not replace the surrounding runner's local state or receipt writer.
func TestRubyVerificationProbeWrapperIsolationPublic(t *testing.T) {
	if !VerificationProbeRuntimeAvailable("ruby") {
		t.Skip("real Ruby runtime unavailable")
	}
	for _, tc := range []struct {
		name, code, reason, detail string
		passed                     bool
		status                     types.VerificationStatus
	}{
		{name: "ordinary_top_level", passed: true},
		{name: "result_path_nil", code: "result_path = nil", passed: true},
		{name: "result_path_empty", code: "result_path = ''", passed: true},
		{name: "encoded", code: "encoded = 'probe bytes'; raise 'wrong local' unless encoded == 'probe bytes'", passed: true},
		{name: "write_result_method", code: "def write_result(value); value + 1; end; raise 'wrong method' unless write_result(41) == 42", passed: true},
		{name: "write_result_noop", code: "def write_result(*args); nil; end", passed: true},
		{name: "combined", code: "result_path = nil; encoded = 'local'; source = 'own source'; outcome = 'own outcome'; e = nil; write_result = ->(x) { x + 1 }; def write_result(x); x + 2; end; raise 'local/method collision' unless write_result.call(41) == 42 && write_result(40) == 42", passed: true},
		{name: "system_exit_zero", code: "result_path = nil; def write_result(x); x; end; puts 'probe-body-reached'; exit 0", passed: true},
		{name: "real_syntax_error", code: "if true", status: types.VerificationStatusUnavailable, reason: "verification_probe_syntax_error", detail: "SyntaxError"},
		{name: "real_exception", code: "result_path = nil; def write_result(x); x; end; raise 'real exception sentinel'", status: types.VerificationStatusFailed, reason: "verification_probe_exception", detail: "real exception sentinel"},
		{name: "real_assertion", code: "result_path = nil; def write_result(x); x; end; class AssertionError < StandardError; end; raise AssertionError, 'real assertion sentinel'", status: types.VerificationStatusFailed, detail: "real assertion sentinel"},
		{name: "real_load_error", code: "result_path = nil; def write_result(x); x; end; require './absent_probe_module'", status: types.VerificationStatusUnavailable, reason: "verification_probe_import_error", detail: "absent_probe_module"},
		{name: "system_exit_nonzero", code: "result_path = nil; def write_result(x); x; end; exit 7", status: types.VerificationStatusFailed, detail: "system_exit"},
		{name: "error_with_warn_method", code: "def warn(*args); nil; end; warn('probe-owned warning'); raise 'real reporting sentinel'", status: types.VerificationStatusFailed, reason: "verification_probe_exception", detail: "real reporting sentinel"},
		{name: "error_with_exit_method", code: "def exit(code); puts \"probe-owned-exit-#{code}\"; end; exit(0); raise 'real exit sentinel'", status: types.VerificationStatusFailed, reason: "verification_probe_exception", detail: "real exit sentinel"},
		{name: "system_exit_with_raise_method", code: "def raise(value = nil); puts 'probe-owned-raise'; end; raise('own call'); Kernel.exit(7)", status: types.VerificationStatusFailed, detail: "system_exit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			wd := filepath.Join(root, "child")
			if err := os.Mkdir(wd, 0o755); err != nil {
				t.Fatal(err)
			}
			modulePath := filepath.Join(wd, "widget.rb")
			moduleBytes := []byte("module Widget; def self.answer; 42; end; end\n")
			if err := os.WriteFile(modulePath, moduleBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			code := fmt.Sprintf(`require "./widget"
raise "wrong imported value" unless Widget.answer == 42
raise "wrong cwd" unless File.realpath(Dir.pwd) == File.realpath(%q)
raise "wrong argv" unless ARGV.empty? && $0 == "-e"
raise "wrong top-level self" unless self.to_s == "main"
raise "wrong filename" unless __FILE__ == "<codrax_verification_probe>"
probe_owned_local = 42
raise "wrong top-level binding" unless TOPLEVEL_BINDING.local_variable_get(:probe_owned_local) == 42
%s
puts "probe-body-reached"
`, wd, tc.code)
			probe := types.VerificationProbe{
				ID: "ruby-isolation-" + tc.name, Language: "ruby", WorkingDir: "child",
				Code: code, TimeoutSeconds: 10, ExpectedStdout: []string{"probe-body-reached"},
				ChangedSymbolRefs: []string{"path:child/widget.rb"},
			}
			plan := &types.ChangePlan{
				ID: "plan-ruby-wrapper-" + tc.name, Status: types.PlanStatusApplied,
				TargetPaths: []string{"child/widget.rb"}, AppliedPaths: []string{"child/widget.rb"},
				VerificationProbes: []types.VerificationProbe{probe},
			}
			planBefore, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			mu := types.NewMutableState("Ruby wrapper isolation")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir()}
			result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"ruby","working_dir":"child"}`))
			if err != nil {
				t.Fatalf("public RunTests.Execute: %v", err)
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatalf("public tool did not install report: %+v", result)
			}
			if result.Success != tc.passed || report.Passed != tc.passed {
				t.Errorf("verdict success=%t report.Passed=%t, want %t; results=%+v", result.Success, report.Passed, tc.passed, report.TestResults)
			}
			wantStatus := tc.status
			if tc.passed {
				wantStatus = types.VerificationStatusPassed
			}
			if got := report.NormalizeVerificationStatus(); got != wantStatus {
				t.Errorf("verification status=%s, want %s", got, wantStatus)
			}
			var command *types.ExecutedCommand
			for i := range report.ExecutedCommands {
				if report.ExecutedCommands[i].Runner == "verification_probe" {
					if command != nil {
						t.Fatal("one probe produced multiple command receipts")
					}
					command = &report.ExecutedCommands[i]
				}
			}
			if command == nil || command.ProbeExecution == nil {
				t.Fatalf("real execution receipt missing: %+v", report.ExecutedCommands)
			}
			wantOutcome := types.ExecutedCommandOutcomeExecuted
			if tc.status == types.VerificationStatusUnavailable {
				wantOutcome = types.ExecutedCommandOutcomeParserError
			}
			if command.Outcome != wantOutcome || command.ReasonCode != tc.reason || (command.ExitCode == 0) != tc.passed {
				t.Errorf("command classification changed: %+v", command)
			}
			if (tc.name == "system_exit_nonzero" || tc.name == "system_exit_with_raise_method") && command.ExitCode != 7 {
				t.Errorf("SystemExit status changed: %d", command.ExitCode)
			}
			if command.Framework != "ruby" || command.Source != "pre_suite_verification_probe" || command.WorkingDir != "child" {
				t.Errorf("invocation lane changed: %+v", command)
			}
			receipt := command.ProbeExecution
			if receipt.Version != types.VerificationProbeExecutionReceiptVersion || receipt.ExecutionID == "" || receipt.StartedAt.IsZero() || receipt.FinishedAt.Before(receipt.StartedAt) || receipt.WorkingDir != wd || receipt.RepositoryRoot != root {
				t.Errorf("invalid terminal execution identity: %+v", receipt)
			}
			if !reflect.DeepEqual(receipt.Args, []string{"ruby", "-e", rubyVerificationProbeWrapper}) || receipt.DefinitionSHA256 != verificationProbeExecutionDigest(probe) || len(receipt.InvocationSHA256) != 64 {
				t.Errorf("receipt does not describe original probe and actual argv: %+v", receipt)
			}
			for _, diagnostic := range report.VerificationDiagnostics {
				if diagnostic.ReasonCode == "verification_probe_unclassified" {
					t.Errorf("wrapper lost its structured outcome: %+v", diagnostic)
				}
			}
			if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != probe.ID || report.TestResults[0].Suite != "verification_probe/ruby" || report.TestResults[0].Passed != tc.passed {
				t.Errorf("probe result identity changed: %+v", report.TestResults)
			} else if tc.detail != "" && !strings.Contains(report.TestResults[0].FailureDetail, tc.detail) {
				t.Errorf("real error detail lost: %+v", report.TestResults[0])
			}
			planAfter, _ := json.Marshal(mu.ChangePlan())
			if !bytes.Equal(planBefore, planAfter) {
				t.Error("verification changed model plan/probe bytes")
			}
			moduleAfter, err := os.ReadFile(modulePath)
			if err != nil || !bytes.Equal(moduleBytes, moduleAfter) {
				t.Errorf("verification changed source bytes: %v", err)
			}
			wire, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			originalCommands, _ := json.Marshal(report.ExecutedCommands)
			restoredCommands, _ := json.Marshal(restored.ExecutedCommands)
			if !bytes.Equal(originalCommands, restoredCommands) {
				t.Error("execution receipt lost during report JSON round trip")
			}
		})
	}
}
