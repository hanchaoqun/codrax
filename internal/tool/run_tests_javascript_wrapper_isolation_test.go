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

// Exercise the public tool, real Node process and installed report. The probe
// owns its source-level bindings; implementation details of the surrounding
// runner must not reserve otherwise legal JavaScript names.
func TestJavaScriptVerificationProbeWrapperIsolationPublic(t *testing.T) {
	if !VerificationProbeRuntimeAvailable("javascript") {
		t.Skip("real Node runtime unavailable")
	}
	for _, tc := range []struct {
		name    string
		code    string
		passed  bool
		outcome string
		reason  string
		status  types.VerificationStatus
	}{
		{name: "ordinary_module_execution", passed: true},
		{name: "fs", code: `const fs = require("fs"); if (typeof fs.readFileSync !== "function") throw new Error("fs unavailable");`, passed: true},
		{name: "vm", code: `const vm = require("vm"); if (typeof vm.runInThisContext !== "function") throw new Error("vm unavailable");`, passed: true},
		{name: "resultPath", code: `const resultPath = "probe-owned path"; require("assert/strict").equal(resultPath, "probe-owned path");`, passed: true},
		{name: "encoded", code: `const encoded = "probe-owned bytes"; require("assert/strict").equal(encoded, "probe-owned bytes");`, passed: true},
		{name: "writeResult", code: `const writeResult = () => "probe-owned function"; require("assert/strict").equal(writeResult(), "probe-owned function");`, passed: true},
		{name: "multiple_bindings", code: `
const fs = require("fs"), vm = require("vm"), resultPath = "local", encoded = "bytes";
const source = fs.readFileSync("widget.cjs", "utf8");
function writeResult() { return resultPath + encoded; }
require("assert/strict").equal(writeResult(), "localbytes");
require("assert/strict").ok(source.includes("answer"));
require("assert/strict").equal(vm.runInNewContext("6 * 7"), 42);
`, passed: true},
		{name: "probe_owns_global_names", code: `const process = "local", console = null, Buffer = null, JSON = null, Number = null, String = null; require("assert/strict").equal(process, "local");`, passed: true},
		{name: "global_names_do_not_mask_assertion", code: `const process = "local", console = null, Buffer = null, JSON = null, Number = null, String = null; require("assert/strict").equal(require("./widget.cjs").answer(), 43, "real assertion sentinel");`, outcome: types.ExecutedCommandOutcomeExecuted, status: types.VerificationStatusFailed},
		{name: "real_syntax_error", code: `const = ;`, outcome: types.ExecutedCommandOutcomeParserError, reason: "verification_probe_syntax_error", status: types.VerificationStatusUnavailable},
		{name: "real_assertion_failure", code: `require("assert/strict").equal(require("./widget.cjs").answer(), 43, "real assertion sentinel");`, outcome: types.ExecutedCommandOutcomeExecuted, status: types.VerificationStatusFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			wd := filepath.Join(root, "child")
			if err := os.Mkdir(wd, 0o755); err != nil {
				t.Fatal(err)
			}
			modulePath := filepath.Join(wd, "widget.cjs")
			moduleBytes := []byte("module.exports.answer = () => 42;\n")
			if err := os.WriteFile(modulePath, moduleBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			code := tc.code + fmt.Sprintf(`
require("assert/strict").equal(require("./widget.cjs").answer(), 42);
require("assert/strict").equal(require("fs").realpathSync(globalThis.process.cwd()), require("fs").realpathSync(%q));
require("assert/strict").equal(globalThis.process.argv.length, 1);
require("assert/strict").equal(globalThis.process.argv[0], globalThis.process.execPath);
require("assert/strict").ok(globalThis.process.execArgv.includes("-e"));
globalThis.console.log("probe-body-reached");
`, wd)
			probe := types.VerificationProbe{
				ID: "isolation-" + tc.name, Language: "javascript", WorkingDir: "child",
				Code: code, TimeoutSeconds: 10, ExpectedStdout: []string{"probe-body-reached"},
				ChangedSymbolRefs: []string{"path:child/widget.cjs"},
			}
			plan := &types.ChangePlan{
				ID: "plan-js-wrapper-" + tc.name, Status: types.PlanStatusApplied,
				TargetPaths: []string{"child/widget.cjs"}, AppliedPaths: []string{"child/widget.cjs"},
				VerificationProbes: []types.VerificationProbe{probe},
			}
			planBefore, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			mu := types.NewMutableState("JavaScript wrapper isolation")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{
				Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify,
				RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir(),
			}
			result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"node","working_dir":"child"}`))
			if err != nil {
				t.Fatalf("public RunTests.Execute: %v", err)
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatalf("public tool did not install report: %+v", result)
			}
			if result.Success != tc.passed || report.Passed != tc.passed {
				t.Errorf("real probe verdict success=%t report.Passed=%t, want %t; results=%+v commands=%+v", result.Success, report.Passed, tc.passed, report.TestResults, report.ExecutedCommands)
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
						t.Fatal("one invocation unexpectedly produced multiple probe commands")
					}
					command = &report.ExecutedCommands[i]
				}
			}
			if command == nil || command.ProbeExecution == nil {
				t.Fatalf("real terminal execution receipt missing: %+v", report.ExecutedCommands)
			}
			wantOutcome := tc.outcome
			if tc.passed {
				wantOutcome = types.ExecutedCommandOutcomeExecuted
			}
			if command.Outcome != wantOutcome || command.ReasonCode != tc.reason || (command.ExitCode == 0) != tc.passed {
				t.Errorf("actual command outcome changed: %+v", command)
			}
			if command.Framework != "javascript" || command.Source != "pre_suite_verification_probe" || command.WorkingDir != "child" {
				t.Errorf("invocation lane or relative cwd changed: %+v", command)
			}
			receipt := command.ProbeExecution
			if receipt.Version != types.VerificationProbeExecutionReceiptVersion || receipt.ExecutionID == "" || receipt.StartedAt.IsZero() || receipt.FinishedAt.Before(receipt.StartedAt) {
				t.Errorf("invalid real terminal receipt: %+v", receipt)
			}
			if receipt.WorkingDir != wd || receipt.RepositoryRoot != root || !filepath.IsAbs(receipt.Executable) || filepath.Base(receipt.Executable) != "node" {
				t.Errorf("execution identity/cwd changed: %+v", receipt)
			}
			if !reflect.DeepEqual(receipt.Args, []string{"node", "-e", javascriptVerificationProbeWrapper}) || receipt.DefinitionSHA256 != verificationProbeExecutionDigest(probe) || len(receipt.InvocationSHA256) != 64 {
				t.Errorf("receipt does not describe exact original probe and actual wrapper argv: %+v", receipt)
			}
			if len(report.TestResults) != 1 || report.TestResults[0].AssertionID != probe.ID || report.TestResults[0].Suite != "verification_probe/javascript" || report.TestResults[0].Passed != tc.passed {
				t.Errorf("original probe result identity/verdict changed: %+v", report.TestResults)
			} else if strings.Contains(tc.name, "assertion") && !strings.Contains(report.TestResults[0].FailureDetail, "real assertion sentinel") {
				t.Errorf("real assertion failure was masked: %+v", report.TestResults[0])
			}
			planAfter, err := json.Marshal(mu.ChangePlan())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(planBefore, planAfter) {
				t.Error("verification changed the model plan/probe bytes")
			}
			moduleAfter, err := os.ReadFile(modulePath)
			if err != nil || !bytes.Equal(moduleBytes, moduleAfter) {
				t.Errorf("verification changed production source: %v", err)
			}
			wire, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			originalCommands, err := json.Marshal(report.ExecutedCommands)
			if err != nil {
				t.Fatal(err)
			}
			restoredCommands, err := json.Marshal(restored.ExecutedCommands)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(restoredCommands, originalCommands) {
				t.Error("execution receipt did not survive report JSON round trip")
			}
		})
	}
}
