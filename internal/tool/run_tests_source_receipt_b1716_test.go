package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A skipped parser is neither a syntax witness nor changed-source coverage.
// Exercise the public producer, not a fabricated command or confidence row.
func TestB1716MissingNodeCannotMintSourceCompileProofPublic(t *testing.T) {
	root := t.TempDir()
	b1716SourceReceiptWriteFile(t, root, "package.json", "{}\n")
	b1716SourceReceiptWriteFile(t, root, "plan.js", "function unfinished( {\n")
	// An empty PATH makes absence deterministic without a fake language oracle.
	t.Setenv("PATH", t.TempDir())
	if path, err := exec.LookPath("node"); err == nil {
		t.Fatalf("fixture accidentally exposes node: %s", path)
	}
	plan, report := b1716SourceReceiptRunPublic(t, root, "plan.js")
	// Overall unavailable is a separate boundary; it cannot repair a false
	// positive syntax row inside the same report.
	if report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Errorf("missing parser/no tests became complete verification: %+v", report)
	}
	b1716SourceReceiptAssertPublicEvidence(t, plan, report, nil, []string{"plan.js"})
	ledger := types.BuildVerificationProofLedger(plan, report, nil)
	if ledger.CapabilityFailedCount != 0 {
		t.Errorf("missing parser was published as an executed failed capability: %+v", ledger)
	}
}

func TestB1716RealNodeSourceCompileProofPublic(t *testing.T) {
	b1716SourceReceiptExposeRealNodeOnly(t)
	root := t.TempDir()
	b1716SourceReceiptWriteFile(t, root, "package.json", "{}\n")
	// This parses successfully but would fail if executed. The resulting
	// witness must stay syntax-only, never target execution or behavior.
	b1716SourceReceiptWriteFile(t, root, "plan.js", "const value = 1;\nthrow new Error('syntax check must not execute');\n")
	plan, report := b1716SourceReceiptRunPublic(t, root, "plan.js")
	if report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Errorf("source-only checking became complete verification: %+v", report)
	}
	b1716SourceReceiptAssertPublicEvidence(t, plan, report, []string{"plan.js"}, nil)
}

func TestB1716RealNodeSuccessDoesNotCoverMissingTypeScriptCheckerPublic(t *testing.T) {
	b1716SourceReceiptExposeRealNodeOnly(t)
	root := t.TempDir()
	b1716SourceReceiptWriteFile(t, root, "package.json", "{}\n")
	b1716SourceReceiptWriteFile(t, root, "a-checked.js", "const value = 1;\n")
	b1716SourceReceiptWriteFile(t, root, "b-unchecked.ts", "export const value: = ;\n")
	if compiler, ok := resolveTypeScriptCompiler(root); ok {
		t.Fatalf("fixture accidentally exposes TypeScript compiler: %s", compiler)
	}
	plan, report := b1716SourceReceiptRunPublic(t, root, "a-checked.js", "b-unchecked.ts")
	if report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Errorf("missing TypeScript checker/no tests became complete verification: %+v", report)
	}
	// Both files belong to the Node source-check family. Success for its JS
	// parser cannot cover the skipped TS file, or be erased by that skip.
	b1716SourceReceiptAssertPublicEvidence(t, plan, report, []string{"a-checked.js"}, []string{"b-unchecked.ts"})
}

func TestB1716RealNodeFailurePreservesCompletedSiblingProofPublic(t *testing.T) {
	b1716SourceReceiptExposeRealNodeOnly(t)
	root := t.TempDir()
	b1716SourceReceiptWriteFile(t, root, "package.json", "{}\n")
	b1716SourceReceiptWriteFile(t, root, "a-checked.js", "const value = 1;\n")
	b1716SourceReceiptWriteFile(t, root, "b-broken.js", "function unfinished( {\n")
	plan, report := b1716SourceReceiptRunPublic(t, root, "a-checked.js", "b-broken.js")
	if report.NormalizeVerificationStatus() != types.VerificationStatusFailed || !report.BuildFailed {
		t.Errorf("real parser error lost its failed build verdict: %+v", report)
	}
	foundDiagnostic := false
	for _, result := range report.TestResults {
		if result.Kind == types.TestResultKindBuildError && !result.Passed && result.AssertionID == "b-broken.js" {
			foundDiagnostic = true
		}
	}
	if !foundDiagnostic {
		t.Errorf("real parser error lost its file-specific diagnostic: %+v", report.TestResults)
	}
	// Batch failure must not erase an independently completed successful
	// sibling, and that sibling must not cover the file that failed parsing.
	b1716SourceReceiptAssertPublicEvidence(t, plan, report, []string{"a-checked.js"}, []string{"b-broken.js"})
}

func b1716SourceReceiptWriteFile(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func b1716SourceReceiptExposeRealNodeOnly(t *testing.T) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("positive source-receipt control requires a real Node.js parser")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	node, err = filepath.EvalSymlinks(node)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "node"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.Symlink(node, filepath.Join(bin, name)); err != nil {
		t.Skipf("cannot isolate the real Node.js parser with a symlink: %v", err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("NODE_OPTIONS", "")
	if _, err := exec.LookPath("node"); err != nil {
		t.Fatalf("isolated real Node.js parser is unavailable: %v", err)
	}
	if path, err := exec.LookPath("tsc"); err == nil {
		t.Fatalf("isolated parser PATH unexpectedly exposes tsc: %s", path)
	}
}

func b1716SourceReceiptRunPublic(t *testing.T, root string, paths ...string) (*types.ChangePlan, *types.ChangeReport) {
	t.Helper()
	if !runnerHasNoTestWork("node", root) {
		t.Fatal("fixture unexpectedly supplies Node test work")
	}
	plan := &types.ChangePlan{ID: "b1716-source-receipt", Status: types.PlanStatusApplied, TargetPaths: paths}
	mu := types.NewMutableState("B1716 public source-check receipts")
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir()}
	result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"node"}`))
	if err != nil {
		t.Fatalf("public Execute: %v", err)
	}
	report := mu.ChangeReport()
	if report == nil {
		t.Fatalf("public Execute did not publish a report: %+v", result)
	}
	return plan, report
}

func b1716SourceReceiptAssertPublicEvidence(t *testing.T, plan *types.ChangePlan, report *types.ChangeReport, checked, unchecked []string) {
	t.Helper()
	wantSourceProof := len(checked) > 0
	b1716SourceReceiptAssertConfidence(t, report.VerificationConfidence, wantSourceProof)
	b1716SourceReceiptAssertCoverage(t, report.ChangedPathCoverage, checked, unchecked)
	for _, result := range report.TestResults {
		if result.Kind != types.TestResultKindBuildError {
			t.Errorf("source checking minted a test assertion: %+v", result)
		}
	}

	// Persist and reload the actual public report, preserving whatever typed
	// execution evidence its producer supplies. Current read projections must
	// neither inflate a skipped file nor discard a genuine sibling receipt.
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored types.ChangeReport
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	b1716SourceReceiptAssertConfidence(t, types.EffectiveVerificationConfidence(plan, &restored), wantSourceProof)
	b1716SourceReceiptAssertCoverage(t, types.EffectiveChangedPathVerificationCoverage(plan, &restored), checked, unchecked)
	profile := types.BuildVerificationProofProfile(plan, &restored)
	if profile.SyntaxOnlyPaths != len(checked) || profile.TargetExecutionPaths != 0 || profile.TargetBehaviorPaths != 0 {
		t.Errorf("source-only proof profile does not match completed path checks: checked=%v unchecked=%v profile=%+v", checked, unchecked, profile)
	}
}

func b1716SourceReceiptAssertConfidence(t *testing.T, records []types.VerificationConfidenceRecord, want bool) {
	t.Helper()
	found := false
	for _, record := range records {
		if record.Category == "source_compile" && record.Status == "satisfied" {
			found = true
			if record.ReasonCode != "source_compile_ok" {
				t.Errorf("successful source check lost its typed reason: %+v", record)
			}
		}
	}
	if found != want {
		t.Errorf("source compile proof=%v, want %v from actual successful parsing: %+v", found, want, records)
	}
}

func b1716SourceReceiptAssertCoverage(t *testing.T, coverage []types.ChangedPathVerificationCoverage, checked, unchecked []string) {
	t.Helper()
	want := make(map[string]bool, len(checked)+len(unchecked))
	for _, path := range checked {
		want[path] = true
	}
	for _, path := range unchecked {
		want[path] = false
	}
	seen := make(map[string]bool, len(want))
	for _, row := range coverage {
		covered, relevant := want[row.Path]
		if !relevant {
			continue
		}
		if seen[row.Path] {
			t.Errorf("duplicate changed-path coverage row: %+v", row)
		}
		seen[row.Path] = true
		if covered {
			if row.Status != types.ChangedPathVerificationCovered || row.Caliber != types.ChangedPathVerificationSourceCheck || row.Capability != types.VerificationCapabilitySyntaxOnly {
				t.Errorf("actual successful parser did not retain syntax-only coverage: %+v", row)
			}
		} else if row.Status != types.ChangedPathVerificationUncovered {
			t.Errorf("source path without a successful parser is covered: %+v", row)
		}
	}
	for path := range want {
		if !seen[path] {
			t.Errorf("public coverage ledger omitted recognized changed path %s: %+v", path, coverage)
		}
	}
}

func TestB1716TypeScriptProcessScopeDoesNotBorrowPlanPaths(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(fmt.Sprintf("tsconfig_%v", project), func(t *testing.T) {
			root, bin := t.TempDir(), b1716SourceReceiptProtocolPath(t)
			b1716SourceReceiptWriteFile(t, root, "checked.ts", "export const checked = 1;\n")
			b1716SourceReceiptWriteFile(t, root, "excluded.ts", "export const excluded: = ;\n")
			if project {
				b1716SourceReceiptWriteFile(t, root, "tsconfig.json", `{"files":["checked.ts"],"exclude":["excluded.ts"]}`)
			}
			log := b1716SourceReceiptProtocolTool(t, bin, "tsc", "", 0)
			file := filepath.Join(root, "checked.ts")
			inputs := []string{file}
			if project {
				inputs = append(inputs, filepath.Join(root, "excluded.ts"))
			}
			report, _ := runTypeScriptCompileFallback(&types.BusContext{}, "b1716", root, inputs)
			if report == nil || !report.Passed {
				t.Fatalf("successful protocol process did not retain its result: %+v", report)
			}
			args := []string{"--noEmit", "--pretty", "false"}
			var checked []string
			unchecked := []string{"excluded.ts"}
			if project {
				unchecked = append(unchecked, "checked.ts")
			} else {
				args = append(args, file)
				checked = []string{"checked.ts"}
			}
			b1716SourceReceiptAssertArgs(t, log, args)
			plan := b1716SourceReceiptProjectProvider(t, root, "node", report, "checked.ts", "excluded.ts")
			b1716SourceReceiptAssertProcess(t, report, "tsc", 0, checked)
			b1716SourceReceiptAssertPublicEvidence(t, plan, report, checked, unchecked)
		})
	}
}

func TestB1716JavaKotlinMixedToolAvailabilityHasExactScope(t *testing.T) {
	for _, tc := range []struct {
		name   string
		java   bool
		kotlin bool
	}{
		{name: "both_available", java: true, kotlin: true},
		{name: "java_project_success_kotlin_missing", java: true},
		{name: "java_missing_kotlin_file_success", kotlin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, bin := t.TempDir(), b1716SourceReceiptProtocolPath(t)
			b1716SourceReceiptWriteFile(t, root, "pom.xml", "<project/>\n")
			b1716SourceReceiptWriteFile(t, root, "Main.java", "class Main {}\n")
			b1716SourceReceiptWriteFile(t, root, "checked.kt", "val checked = 1\n")
			b1716SourceReceiptWriteFile(t, root, "excluded.kt", "val excluded = 2\n")
			var javaLog, kotlinLog string
			if tc.java {
				javaLog = b1716SourceReceiptProtocolTool(t, bin, "mvn", "", 0)
			}
			if tc.kotlin {
				kotlinLog = b1716SourceReceiptProtocolTool(t, bin, "kotlinc", "", 0)
			}
			report, _ := runJavaCompileFallback(&types.BusContext{}, "b1716", root, []string{filepath.Join(root, "Main.java"), filepath.Join(root, "checked.kt")})
			if report == nil || !report.Passed {
				t.Fatalf("successful/missing-tool aggregate lost completed observations: %+v", report)
			}
			plan := b1716SourceReceiptProjectProvider(t, root, "java", report, "Main.java", "checked.kt", "excluded.kt")
			if got, want := len(report.ExecutedCommands), b1716SourceReceiptBoolInt(tc.java)+b1716SourceReceiptBoolInt(tc.kotlin); got != want {
				t.Errorf("missing tool fabricated a process or completed receipt was lost: got=%d want=%d commands=%+v", got, want, report.ExecutedCommands)
			}
			unchecked := []string{"Main.java", "excluded.kt"}
			var checked []string
			if tc.java {
				b1716SourceReceiptAssertArgs(t, javaLog, []string{"-B", "-q", "compile", "-DskipTests=true", "-o"})
				b1716SourceReceiptAssertProcess(t, report, "mvn", 0, nil)
			}
			if tc.kotlin {
				args := b1716SourceReceiptReadArgs(t, kotlinLog)
				if len(args) != 4 || args[0] != "-d" || args[1] == "" || args[2] != "-nowarn" || args[3] != filepath.Join(root, "checked.kt") {
					t.Errorf("Kotlin process did not receive exactly its one source input: %q", args)
				}
				checked = []string{"checked.kt"}
				b1716SourceReceiptAssertProcess(t, report, "kotlinc", 0, checked)
			} else {
				unchecked = append(unchecked, "checked.kt")
			}
			b1716SourceReceiptAssertPublicEvidence(t, plan, report, checked, unchecked)
		})
	}
}

func TestB1716NonzeroSourceProcessWithoutCompilerDiagnosticIsNotPass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		runner   string
		binary   string
		manifest string
		path     string
		provider func(*types.BusContext, string, string, []string) (*types.ChangeReport, string)
	}{
		{name: "java", runner: "java", binary: "mvn", manifest: "pom.xml", path: "Main.java", provider: runJavaCompileFallback},
		{name: "kotlin", runner: "java", binary: "kotlinc", path: "checked.kt", provider: runJavaCompileFallback},
		{name: "swift", runner: "swift", binary: "swift", manifest: "Package.swift", path: "Sources/checked.swift", provider: runSwiftCompileFallback},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, bin := t.TempDir(), b1716SourceReceiptProtocolPath(t)
			if tc.manifest != "" {
				b1716SourceReceiptWriteFile(t, root, tc.manifest, "fixture\n")
			}
			b1716SourceReceiptWriteFile(t, root, tc.path, "fixture\n")
			guard := ""
			if tc.binary == "swift" {
				guard = "if [ \"$#\" -ne 1 ] || [ \"$1\" != build ]; then exit 91; fi\n"
			}
			log := b1716SourceReceiptProtocolTool(t, bin, tc.binary, guard, 23)
			report, _ := tc.provider(&types.BusContext{}, "b1716", root, []string{filepath.Join(root, tc.path)})
			if report == nil || report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
				t.Fatalf("nonzero checker without a source diagnostic became pass or product failure: %+v", report)
			}
			if report.BuildFailed || len(report.TestResults) != 0 {
				t.Errorf("opaque process failure fabricated a compiler diagnostic: %+v", report)
			}
			if tc.binary == "swift" {
				b1716SourceReceiptAssertArgs(t, log, []string{"build"})
			}
			plan := b1716SourceReceiptProjectProvider(t, root, tc.runner, report, tc.path)
			b1716SourceReceiptAssertProcess(t, report, tc.binary, 23, nil)
			b1716SourceReceiptAssertPublicEvidence(t, plan, report, nil, []string{tc.path})
		})
	}
}

func TestB1716NodePreflightPublicRetainsSyntaxOnlyWhenNpmMissing(t *testing.T) {
	b1716SourceReceiptExposeRealNodeOnly(t)
	root := t.TempDir()
	b1716SourceReceiptWriteFile(t, root, "package.json", `{"scripts":{"test":"jest"},"devDependencies":{"jest":"*"}}`)
	b1716SourceReceiptWriteFile(t, root, "present.test.js", "test('fixture', () => {});\n")
	b1716SourceReceiptWriteFile(t, root, "plan.js", "throw new Error('preflight must not execute source');\n")
	if runnerHasNoTestWork("node", root) {
		t.Fatal("fixture must enter the public preflight lane, not no-test fallback")
	}
	if path, err := exec.LookPath("npm"); err == nil {
		t.Fatalf("fixture accidentally exposes npm: %s", path)
	}
	plan := &types.ChangePlan{ID: "b1716-source-preflight", Status: types.PlanStatusApplied, TargetPaths: []string{"plan.js"}}
	mu := types.NewMutableState("B1716 public preflight receipt")
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: t.TempDir()}
	if _, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"node"}`)); err != nil {
		t.Fatalf("public preflight Execute: %v", err)
	}
	report := mu.ChangeReport()
	if report == nil || report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Fatalf("missing npm must remain unavailable despite successful source preflight: %+v", report)
	}
	found := false
	for _, command := range report.ExecutedCommands {
		if command.Outcome == types.ExecutedCommandOutcomeSyntaxPreflight && types.SourceCheckCommandSucceeded(command) {
			found = true
		}
	}
	if !found {
		t.Errorf("public run_tests did not publish a real successful preflight command: %+v", report.ExecutedCommands)
	}
	b1716SourceReceiptAssertPublicEvidence(t, plan, report, []string{"plan.js"}, nil)
}

// These shell processes test the compiler invocation/receipt protocol only;
// they make no claim that Java, Kotlin, Swift or TypeScript was type-checked.
func b1716SourceReceiptProtocolPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("source-check process protocol fixtures use a POSIX shell")
	}
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	return bin
}

func b1716SourceReceiptProtocolTool(t *testing.T, bin, name, guard string, exitCode int) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "arguments")
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuoteWord(log) + "\n" + guard + fmt.Sprintf("exit %d\n", exitCode)
	if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return log
}

func b1716SourceReceiptReadArgs(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("expected real protocol process to record its invocation: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func b1716SourceReceiptAssertArgs(t *testing.T, log string, want []string) {
	t.Helper()
	if got := b1716SourceReceiptReadArgs(t, log); !reflect.DeepEqual(got, want) {
		t.Errorf("actual process arguments=%q want=%q", got, want)
	}
}

func b1716SourceReceiptProjectProvider(t *testing.T, root, runner string, report *types.ChangeReport, paths ...string) *types.ChangePlan {
	t.Helper()
	plan := &types.ChangePlan{ID: "b1716-provider-receipt", Status: types.PlanStatusApplied, TargetPaths: paths}
	report.PlanID = plan.ID
	report.ExecutedCommands = sourceCheckCommandsForReport(root, runnerPlan{Runner: runner, Root: root}, "b1716_protocol", types.ExecutedCommandOutcomeSyntaxCheckFallback, report)
	report.VerificationConfidence = verificationConfidenceRecordsFromReport(plan, report)
	applyChangedPathVerificationCoverageForPlan(&types.BusContext{RepoRoot: root}, plan, report, false)
	return plan
}

func b1716SourceReceiptAssertProcess(t *testing.T, report *types.ChangeReport, binary string, exitCode int, checked []string) {
	t.Helper()
	for _, command := range report.ExecutedCommands {
		if !strings.Contains(command.Command, binary) {
			continue
		}
		receipt := command.SourceCheckExecution
		if receipt == nil || receipt.Version != types.SourceCheckExecutionReceiptVersion || !receipt.Started || !receipt.Completed || !receipt.ExitCodeKnown || receipt.ExitCode != exitCode || command.ExitCode != exitCode {
			t.Errorf("real process lifecycle/exit was not retained: command=%+v receipt=%+v", command, receipt)
			return
		}
		if !reflect.DeepEqual(receipt.CheckedPaths, checked) || !reflect.DeepEqual(command.CoveredPaths, checked) {
			t.Errorf("actual process scope inflated or lost: checked=%v command=%+v receipt=%+v", checked, command, receipt)
		}
		if got, want := types.SourceCheckCommandSucceeded(command), exitCode == 0 && len(checked) > 0; got != want {
			t.Errorf("process qualification=%v want=%v: %+v", got, want, command)
		}
		return
	}
	t.Errorf("actual %s process missing from report: %+v", binary, report.ExecutedCommands)
}

func b1716SourceReceiptBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
