package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Use the public runner and durable JSON boundary: a passing opaque Make
// command must not erase an earlier probe's actual stderr, or promote it to a
// passing behavioral assertion. These are real, heterogeneous processes.
func TestRunTestsProbeExecutionObservationPublicB1702(t *testing.T) {
	for _, tc := range []struct {
		name, language, binary, target, code, detail string
		artifactDenied, suiteFails, nativeSuite      bool
	}{
		{name: "javascript_reference", language: "javascript", binary: "node", target: "widget.js", code: "throw new ReferenceError('B1702 missing dispatch receiver');", detail: "B1702 missing dispatch receiver"},
		{name: "javascript_syntax", language: "javascript", binary: "node", target: "widget.js", code: "const reserved = ;", detail: "Unexpected token"},
		{name: "python_import", language: "python", binary: "python3", target: "widget.py", code: "import b1702_nonexistent_dependency", detail: "b1702_nonexistent_dependency"},
		{name: "ruby_load", language: "ruby", binary: "ruby", target: "widget.rb", code: "require 'b1702_nonexistent_dependency'", detail: "b1702_nonexistent_dependency"},
		{name: "artifact_denied", language: "javascript", binary: "node", target: "widget.js", code: "throw new ReferenceError('B1702 missing dispatch receiver');", detail: "B1702 missing dispatch receiver", artifactDenied: true},
		{name: "suite_failure", language: "python", binary: "python3", target: "widget.py", code: "import b1702_nonexistent_dependency", detail: "b1702_nonexistent_dependency", suiteFails: true},
		{name: "native_suite_passes", language: "python", binary: "python3", target: "widget.py", code: "import b1702_nonexistent_dependency", detail: "b1702_nonexistent_dependency", nativeSuite: true},
		{name: "python_name_late_diagnostic", language: "python", binary: "python3", target: "widget.py", code: "missing_b1702_receiver()", detail: "missing_b1702_receiver", nativeSuite: true},
		{name: "python_syntax_late_diagnostic", language: "python", binary: "python3", target: "widget.py", code: "broken = ", detail: "invalid syntax", nativeSuite: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, binary := range []string{tc.binary, "make", "python3"} {
				if _, err := exec.LookPath(binary); err != nil {
					t.Skipf("native observation fixture requires %s: %v", binary, err)
				}
			}
			root, workDir := t.TempDir(), t.TempDir()
			files := map[string]string{
				"widget.js": "const VALUE = 41;\n", "widget.py": "VALUE = 41\n", "widget.rb": "VALUE = 41\n",
				"Makefile": fmt.Sprintf(".PHONY: check\ncheck:\n\tpython3 -c \"from pathlib import Path; assert 'VALUE' in Path('%s').read_text(); assert %s\"\n", tc.target, map[bool]string{true: "False", false: "True"}[tc.suiteFails]),
			}
			// Match the aggregate runner's language without turning its source
			// text check into a native behavior/assertion protocol.
			if tc.language == "javascript" {
				files["Makefile"] = ".PHONY: check\ncheck:\n\tnode -e \"if (!require('fs').readFileSync('widget.js', 'utf8').includes('VALUE')) process.exit(1)\"\n"
			} else if tc.language == "ruby" {
				files["Makefile"] = ".PHONY: check\ncheck:\n\truby -e \"raise 'source missing' unless File.read('widget.rb').include?('VALUE')\"\n"
			}
			if tc.nativeSuite {
				files["test_widget.py"] = "import unittest\nimport widget\nclass ValueTest(unittest.TestCase):\n    def test_value(self):\n        self.assertEqual(widget.VALUE, 41)\n"
				files["Makefile"] = ".PHONY: check\ncheck:\n\tpython3 -B -m unittest test_widget -v\n"
			}
			for name, source := range files {
				if err := os.WriteFile(filepath.Join(root, name), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.artifactDenied {
				workDir = filepath.Join(workDir, "owned-blocking-file")
				if err := os.WriteFile(workDir, []byte("keep this sentinel"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			probe := types.VerificationProbe{ID: "probe-actual-error", Language: tc.language, WorkingDir: ".", Code: tc.code, TimeoutSeconds: 10, ChangedSymbolRefs: []string{"path:" + tc.target}}
			plan := &types.ChangePlan{ID: "plan-probe-observation", Status: types.PlanStatusApplied, TargetPaths: []string{tc.target}, AppliedPaths: []string{tc.target}, Changes: []types.FileChange{{Path: tc.target, Kind: "modify", NewContent: files[tc.target]}}, VerificationProbes: []types.VerificationProbe{probe}}
			mu := types.NewMutableState("preserve execution output without granting proof")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: workDir}
			before := failureObservationJSON(t, plan)
			params := json.RawMessage(`{"runner":"make","suite":"check"}`)
			if tc.nativeSuite {
				params = json.RawMessage(`{"runner":"python","framework":"unittest"}`)
			}
			result, err := (&RunTests{}).Execute(ctx, params)
			if err != nil || mu.ChangeReport() == nil {
				t.Fatalf("public Execute: result=%+v err=%v", result, err)
			}
			report := mu.ChangeReport()
			wire := failureObservationJSON(t, report)
			if result.Success != tc.nativeSuite || report.Passed != tc.nativeSuite {
				t.Fatalf("old suite verdict changed: %s", wire)
			}
			if !tc.nativeSuite && !tc.suiteFails && report.FailureReasonCode != "changed_path_verification_uncovered" {
				t.Fatalf("opaque source check must not authorize changed paths: %s", wire)
			}
			probeRan, suiteRan := false, false
			var receipt *types.VerificationProbeExecutionReceipt
			for _, cmd := range report.ExecutedCommands {
				if cmd.Runner == "verification_probe" && cmd.ProbeExecution != nil {
					probeRan, receipt = true, cmd.ProbeExecution
				}
				if (cmd.Runner == "make" || (tc.nativeSuite && cmd.Runner == "python")) && cmd.Command != "" {
					suiteRan = true
				}
			}
			if !probeRan || !suiteRan {
				t.Fatalf("fixture must actually run probe and fallback: %s", wire)
			}
			if !tc.suiteFails && countFailed(report.TestResults) != 0 {
				t.Fatal("unavailable probe was inserted into passing suite results")
			}
			// Decode wire keys, keeping this regression
			// executable on the old code before the new Go carrier exists.
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(wire, &raw); err != nil {
				t.Fatal(err)
			}
			var rows []map[string]json.RawMessage
			if err := json.Unmarshal(raw["verification_diagnostics"], &rows); err != nil {
				t.Fatal(err)
			}
			observations := []map[string]string{}
			for _, row := range rows {
				if value, ok := row["probe_execution_observations"]; ok {
					var items []map[string]string
					if err := json.Unmarshal(value, &items); err != nil {
						t.Fatal(err)
					}
					observations = append(observations, items...)
				}
			}
			if len(observations) != 1 {
				t.Fatalf("actual error was lost at public report boundary: got %d observations; %s", len(observations), wire)
			}
			obs := observations[0]
			if obs["plan_id"] != plan.ID || obs["probe_id"] != probe.ID || obs["execution_id"] != receipt.ExecutionID || obs["definition_sha256"] != receipt.DefinitionSHA256 || obs["invocation_sha256"] != receipt.InvocationSHA256 {
				t.Errorf("observation not bound to current actual execution: %v receipt=%+v", obs, receipt)
			}
			if !strings.Contains(obs["output_excerpt"], tc.detail) || !strings.Contains(result.Summary, tc.detail) {
				t.Errorf("concrete error absent from observation/tool context: observation=%v summary=%s", obs, result.Summary)
			}
			if tc.artifactDenied {
				if obs["output_ref"] != "" {
					t.Error("unwritable artifact must not fabricate a reference")
				}
				data, readErr := os.ReadFile(workDir)
				if readErr != nil || string(data) != "keep this sentinel" {
					t.Fatalf("artifact failure modified unrelated file: %q %v", data, readErr)
				}
			} else {
				data, readErr := os.ReadFile(obs["output_ref"])
				if readErr != nil || !bytes.Contains(data, []byte(tc.detail)) || string(data) != obs["output_excerpt"] {
					t.Errorf("short exact stderr lacks durable artifact: %q %v", data, readErr)
				}
				read, readErr := (&ReadFile{}).Execute(ctx, failureObservationJSON(t, map[string]any{"path": obs["output_ref"]}))
				if readErr != nil || !read.Success || !strings.Contains(read.Summary, tc.detail) {
					t.Errorf("artifact not publicly readable: %+v %v", read, readErr)
				}
			}
			for i := range rows {
				delete(rows[i], "probe_execution_observations")
			}
			raw["verification_diagnostics"] = failureObservationJSON(t, rows)
			var stripped types.ChangeReport
			if err := json.Unmarshal(failureObservationJSON(t, raw), &stripped); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(types.EffectiveVerificationConfidence(plan, report), types.EffectiveVerificationConfidence(plan, &stripped)) || !reflect.DeepEqual(types.BuildVerificationProofLedger(plan, report, nil), types.BuildVerificationProofLedger(plan, &stripped, nil)) || verificationProbeDiagnosticsAreNonAuthoritative(report) != verificationProbeDiagnosticsAreNonAuthoritative(&stripped) {
				t.Error("display observation changed proof/confidence/continuation authority")
			}
			if !bytes.Equal(before, failureObservationJSON(t, plan)) || !bytes.Equal(wire, failureObservationJSON(t, report)) {
				t.Error("display mutated original plan/report")
			}
			for name, source := range files {
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || string(data) != source {
					t.Errorf("source mutation: %s %v", name, err)
				}
			}
		})
	}
}

func TestProbeExecutionObservationProducerAndMergeB1702(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("native receipt fixture requires node: %v", err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir()}
	probe := types.VerificationProbe{ID: "probe-original", Language: "javascript", WorkingDir: ".", Code: "throw new ReferenceError('original execution detail')", TimeoutSeconds: 10}
	baseline := runSingleVerificationProbe(ctx, probe, "pre_suite_verification_probe")
	if baseline.Report == nil || baseline.Report.Passed || len(baseline.Commands) != 1 || baseline.Commands[0].ProbeExecution == nil || len(baseline.Report.VerificationDiagnostics) != 1 {
		t.Fatalf("need a real unavailable execution: %+v", baseline)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*verificationProbeRunResult)
	}{
		{name: "valid"},
		{name: "already_passed", mutate: func(r *verificationProbeRunResult) { r.Report.Passed = true }},
		{name: "no_diagnostic", mutate: func(r *verificationProbeRunResult) { r.Report.VerificationDiagnostics = nil }},
		{name: "no_receipt", mutate: func(r *verificationProbeRunResult) { r.Commands[0].ProbeExecution = nil }},
		{name: "missing_execution", mutate: func(r *verificationProbeRunResult) { r.Commands[0].ProbeExecution.ExecutionID = "" }},
		{name: "changed_definition", mutate: func(r *verificationProbeRunResult) {
			r.Commands[0].ProbeExecution.DefinitionSHA256 = strings.Repeat("a", 64)
		}},
		{name: "invalid_invocation", mutate: func(r *verificationProbeRunResult) { r.Commands[0].ProbeExecution.InvocationSHA256 = "incomplete" }},
		{name: "unrelated_command", mutate: func(r *verificationProbeRunResult) { r.Commands[0].Command = "another producer command" }},
		{name: "unrelated_source", mutate: func(r *verificationProbeRunResult) { r.Commands[0].Source = "another source" }},
		{name: "unrelated_language", mutate: func(r *verificationProbeRunResult) { r.Commands[0].Framework = "ruby" }},
		{name: "unrelated_directory", mutate: func(r *verificationProbeRunResult) { r.Commands[0].WorkingDir = "another" }},
		{name: "unrelated_exit", mutate: func(r *verificationProbeRunResult) { r.Commands[0].ExitCode = 7 }},
		{name: "unrelated_runner", mutate: func(r *verificationProbeRunResult) { r.Commands[0].Runner = "make" }},
		{name: "empty_raw_output", mutate: func(r *verificationProbeRunResult) { r.Output = "" }},
		{name: "missing_output_origin", mutate: func(r *verificationProbeRunResult) { r.OutputCommandIndex = -1 }},
		{name: "out_of_range_output_origin", mutate: func(r *verificationProbeRunResult) { r.OutputCommandIndex = len(r.Commands) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var result verificationProbeRunResult
			if err := json.Unmarshal(failureObservationJSON(t, baseline), &result); err != nil {
				t.Fatal(err)
			}
			if tc.mutate != nil {
				tc.mutate(&result)
			}
			before := failureObservationJSON(t, result.Report)
			retainVerificationProbeExecutionOutput(ctx, "plan-original", probe, &result)
			count := 0
			for _, diagnostic := range result.Report.VerificationDiagnostics {
				count += len(diagnostic.ProbeExecutionObservations)
			}
			if tc.name != "valid" {
				if count != 0 || !bytes.Equal(before, failureObservationJSON(t, result.Report)) {
					t.Fatalf("unrelated/missing receipt changed diagnostics: %s", failureObservationJSON(t, result.Report))
				}
				return
			}
			if count != 1 {
				t.Fatal("complete native execution was not retained")
			}
			a := result.Report.VerificationDiagnostics[0]
			b := a
			b.ProbeExecutionObservations = types.MergeVerificationProbeExecutionObservations(a.ProbeExecutionObservations)
			b.ProbeExecutionObservations[0].ExecutionID = "another-actual-execution"
			merged := mergeVerificationDiagnostics([]types.VerificationDiagnostic{a}, []types.VerificationDiagnostic{b, a})
			if len(merged) != 1 || len(merged[0].ProbeExecutionObservations) != 2 {
				t.Fatalf("nested merge changed old diagnostic count or lost execution identity: %+v", merged)
			}
			if merged[0].Category != a.Category || merged[0].Outcome != a.Outcome || merged[0].ReasonCode != a.ReasonCode {
				t.Error("display merge reclassified original diagnostic")
			}
			merged[0].ProbeExecutionObservations[0].OutputExcerpt = "changed detached copy"
			if a.ProbeExecutionObservations[0].OutputExcerpt == "changed detached copy" || b.ProbeExecutionObservations[0].OutputExcerpt == "changed detached copy" {
				t.Fatal("merged display aliases original diagnostic")
			}
		})
	}
}

// This is a compiler-process protocol fixture, not a claim that a real JDK is
// installed. It exercises the public dry-run lane and the previously truncated
// raw javac output; existing compile classification/detail limits stay intact.
func TestJavaCompileProbeExecutionObservationProtocolB1702(t *testing.T) {
	bin, root, workDir := t.TempDir(), t.TempDir(), t.TempDir()
	rawOutput := "compiler-output-start\n" + strings.Repeat("类型检查上下文 = 1234567890\n", 400) + "compiler-output-end\n"
	for name, body := range map[string]string{
		"javac": "#!/bin/sh\nprintf '%s' '" + rawOutput + "'\nexit 1\n",
		"java":  "#!/bin/sh\nexit 99\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	mu := types.NewMutableState("preserve compiler output")
	mu.SetChangePlan(&types.ChangePlan{ID: "active-java-plan", Status: types.PlanStatusApplied})
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StagePlan, RepoRoot: root, MainRepoRoot: root, WorkDir: workDir}
	result, err := (&RunTests{}).Execute(ctx, failureObservationJSON(t, map[string]any{
		"dry_run":            true,
		"verification_probe": map[string]any{"id": "java_compile", "language": "java", "code": "throw new AssertionError(\"probe\");"},
	}))
	reports := mu.PlanStageProbeReports()
	if err != nil || result.Success || len(reports) != 1 {
		t.Fatalf("public dry-run result=%+v reports=%+v err=%v", result, reports, err)
	}
	report := reports[0]
	if report.FailureKind != types.FailureKindParserError || report.FailureReasonCode != "verification_probe_java_compile_error" || len(report.TestResults) != 1 || len(report.TestResults[0].FailureDetail) > 4010 {
		t.Fatalf("original compile classification/count/detail changed: %+v", report)
	}
	observations := types.CurrentReportProbeExecutionObservations(report)
	if len(observations) != 1 || !strings.Contains(observations[0].OutputExcerpt, "compiler-output-start") || !strings.Contains(observations[0].OutputExcerpt, "compiler-output-end") {
		t.Fatalf("late compiler diagnostic lost bounded raw output: %+v; plan=%q diagnostics=%+v receipt=%+v", observations, report.PlanID, report.VerificationDiagnostics, report.ExecutedCommands[0].ProbeExecution)
	}
	data, err := os.ReadFile(observations[0].OutputRef)
	if err != nil || string(data) != rawOutput {
		t.Fatalf("compiler complete output was truncated or fabricated: bytes=%d want=%d err=%v", len(data), len(rawOutput), err)
	}
	if !strings.Contains(result.Summary, "compiler-output-start") || !strings.Contains(result.Summary, "compiler-output-end") || mu.ChangeReport() != nil {
		t.Errorf("planner observation erased or promoted into post-apply authority: %s", result.Summary)
	}
}

func TestJavaRuntimeProbeOutputOriginProtocolB1702(t *testing.T) {
	bin, root, workDir := t.TempDir(), t.TempDir(), t.TempDir()
	for name, body := range map[string]string{
		"javac": "#!/bin/sh\nprintf 'compile succeeded\\n'\nexit 0\n",
		"java":  "#!/bin/sh\nprintf 'actual runtime error\\n' >&2\nexit 1\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx := &types.BusContext{RepoRoot: root, MainRepoRoot: root, WorkDir: workDir}
	probe := types.VerificationProbe{ID: "java_runtime", Language: "java", WorkingDir: ".", Code: "throw new AssertionError(\"probe\");", TimeoutSeconds: 10}
	result := runSingleVerificationProbe(ctx, probe, "planner_probe_verification_probe")
	if len(result.Commands) != 2 || result.OutputCommandIndex != 1 || result.Commands[0].ExitCode != 0 || result.Commands[1].ExitCode != 1 {
		t.Fatalf("protocol must execute compile then runtime failure: %+v", result)
	}
	retainVerificationProbeExecutionOutput(ctx, "java-plan", probe, &result)
	if len(result.ExecutionObservations) != 1 || result.ExecutionObservations[0].ExecutionID != result.Commands[1].ProbeExecution.ExecutionID || result.ExecutionObservations[0].OutputExcerpt != "actual runtime error\n" {
		t.Fatalf("runtime output was attached to its prerequisite compiler: %+v", result.ExecutionObservations)
	}
	artifacts, err := os.ReadDir(workDir)
	if err != nil || len(artifacts) != 1 {
		t.Fatalf("runtime output minted redundant/foreign artifacts: %+v %v", artifacts, err)
	}
}
