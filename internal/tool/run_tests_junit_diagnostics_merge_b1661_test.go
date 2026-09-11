package tool

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These are real child processes implementing the Maven/CTest report protocols,
// not native Java/C++ assertion execution. The receipt must describe the bytes
// actually parsed by RunTests, without changing their existing proof strength.
func TestB1661PublicJUnitDiagnosticsSurviveInstalledReport(t *testing.T) {
	for _, runner := range []string{"java", "cmake"} {
		for _, outcome := range []string{"pass", "failure", "skipped", "missing"} {
			t.Run(runner+"/"+outcome, func(t *testing.T) {
				b1661ExecuteJUnitFixture(t, runner, outcome, 1)
			})
		}
	}
	t.Run("java/two_report_files", func(t *testing.T) {
		b1661ExecuteJUnitFixture(t, "java", "pass", 2)
	})
}

func TestB1661MergeCurrentRunnerReportsPreservesOtherFieldsAndCarry(t *testing.T) {
	reports := []*types.ChangeReport{
		b1661ExecuteJUnitFixture(t, "java", "pass", 1),
		nil,
		b1661ExecuteJUnitFixture(t, "cmake", "failure", 1),
	}
	before := b1661JSON(t, reports)
	var without []*types.ChangeReport
	var receipts []types.VerificationDiagnostic
	for _, report := range reports {
		if report == nil {
			without = append(without, nil)
			continue
		}
		copy := *report
		copy.VerificationDiagnostics = nil
		without = append(without, &copy)
		receipts = append(receipts, b1661Receipts(report.VerificationDiagnostics)...)
	}
	merged := mergeChangeReports(reports)
	if got := b1661Receipts(merged.VerificationDiagnostics); !reflect.DeepEqual(got, receipts) || len(got) != 2 {
		t.Errorf("each actual runner receipt must survive once in input order: got %+v want %+v", got, receipts)
	}
	// finishReport also carries probe diagnostics. Replaying that same input
	// must not duplicate either the old diagnostic lane or current receipts.
	probe := types.VerificationDiagnostic{Source: "verification_probe", Category: "probe_authoring", Severity: "warning", ReasonCode: "verification_probe_name_error", Detail: "fixture diagnostic"}
	carried := append([]types.VerificationDiagnostic{probe}, merged.VerificationDiagnostics...)
	merged.VerificationDiagnostics = mergeVerificationDiagnostics(merged.VerificationDiagnostics, carried)
	once := append([]types.VerificationDiagnostic(nil), merged.VerificationDiagnostics...)
	merged.VerificationDiagnostics = mergeVerificationDiagnostics(merged.VerificationDiagnostics, carried)
	if !reflect.DeepEqual(once, merged.VerificationDiagnostics) || len(b1661Receipts(once)) != 2 {
		t.Error("carried diagnostics duplicated or erased current source receipts")
	}
	merged.VerificationDiagnostics = nil
	want := mergeChangeReports(without)
	if !reflect.DeepEqual(merged, want) {
		t.Errorf("merge changed fields beyond diagnostics:\ngot %+v\nwant %+v", merged, want)
	}
	passed, total := merged.Score()
	if passed != 1 || total != 2 || merged.Passed || merged.FailureKind != types.FailureKindTestsFailed || merged.TestResults[1].ObservationScope != types.TestObservationScopeAssertion {
		t.Errorf("original score/failure/scope changed: %+v score=%d/%d", merged, passed, total)
	}
	if !bytes.Equal(before, b1661JSON(t, reports)) {
		t.Error("merge modified its input report, command, or confidence objects")
	}
}

func b1661ExecuteJUnitFixture(t *testing.T, runner, outcome string, fileCount int) *types.ChangeReport {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string, mode os.FileMode) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
	child := ""
	switch outcome {
	case "failure":
		child = `<failure type="AssertionError">wrong value</failure>`
	case "skipped":
		child = `<skipped message="disabled"/>`
	}
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > fixture-executed-args.txt\n"
	if runner == "java" {
		write("pom.xml", "<project/>\n", 0o644)
		write("src/main/java/example/Value.java", "package example; class Value { int value() { return 1; } }\n", 0o644)
		write("src/test/java/example/ValueTest.java", "package example; class ValueTest { @Test void checks() {} @Test void other() {} }\n", 0o644)
		script += "suffix=''\nfor arg in \"$@\"; do\n case \"$arg\" in -Dsurefire.reportNameSuffix=*) suffix=${arg#*=};; esac\ndone\ntest -n \"$suffix\"\n"
		if outcome != "missing" {
			script += "mkdir -p target/surefire-reports\n"
			for i := 0; i < fileCount; i++ {
				name := "checks"
				if i != 0 {
					name = "other"
				}
				script += fmt.Sprintf("report=\"$PWD/target/surefire-reports/TEST-current-%d-$suffix.xml\"\nprintf '<testsuite name=\"example.ValueTest(%%s)\"><testcase classname=\"example.ValueTest(%%s)\" name=\"%s\" time=\"0.125\">%s</testcase></testsuite>\\n' \"$suffix\" \"$suffix\" > \"$report\"\ncp \"$report\" fixture-wire-%d.xml\nprintf '%%s' \"$report\" > fixture-path-%d.txt\n", i, name, child, i, i)
			}
		}
	} else {
		write("CMakeLists.txt", "enable_testing()\n", 0o644)
		write("build/CMakeCache.txt", "CMAKE_HOME_DIRECTORY:INTERNAL="+root+"\n", 0o644)
		write("build/CTestTestfile.cmake", "add_test(check /bin/true)\n", 0o644)
		script += "report=''\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = --output-junit ]; then shift; report=$1; fi\n shift\ndone\ntest -n \"$report\"\n"
		if outcome != "missing" {
			script += "printf '%s' '<testsuite name=\"current\"><testcase name=\"check\" time=\"0.125\">" + child + "</testcase></testsuite>' > \"$report\"\ncp \"$report\" fixture-wire-0.xml\nprintf '%s' \"$report\" > fixture-path-0.txt\n"
		}
	}
	exit := 0
	if outcome == "failure" {
		exit = 1
	}
	script += fmt.Sprintf("printf 'fixture command completed\\n'\nexit %d\n", exit)
	binary := "ctest"
	if runner == "java" {
		binary = "mvn"
	}
	write("bin/"+binary, script, 0o755)
	t.Setenv("PATH", filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	plan := &types.ChangePlan{ID: "b1661-current-receipt", Status: types.PlanStatusApplied}
	if runner == "java" {
		plan.TargetPaths = []string{"src/main/java/example/Value.java"}
		plan.Changes = []types.FileChange{{Path: plan.TargetPaths[0], Kind: "patch"}}
		plan.BehaviorContracts = []types.WriteBehaviorContract{{ID: "value-contract", Kind: "observable", Required: true}}
		plan.ProjectTestObservations = []types.ProjectTestObservation{{ID: "value-test", TestPath: "src/test/java/example/ValueTest.java", AssertionSuite: "example.ValueTest", AssertionID: "example.ValueTest#checks", ContractRefs: []string{"value-contract"}}}
	}
	planBefore := b1661JSON(t, plan)
	mu := types.NewMutableState("current JUnit receipt, not additional proof")
	mu.SetChangePlan(plan)
	ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root}
	if _, err := (&RunTests{}).Execute(ctx, runTestsJSONParams(t, map[string]any{"runner": runner})); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(root, "fixture-executed-args.txt")); err != nil {
		t.Fatalf("protocol child did not execute: %v", err)
	}
	report := mu.ChangeReport()
	if report == nil || len(report.ExecutedCommands) != 1 {
		t.Fatalf("missing installed report or actual command: %+v", report)
	}
	command := report.ExecutedCommands[0]
	if command.Runner != runner || command.ExitCode != exit || command.Outcome != types.ExecutedCommandOutcomeExecuted {
		t.Fatalf("actual command evidence changed: %+v", command)
	}
	wantRows := fileCount
	if outcome == "missing" {
		wantRows = 0
	}
	if len(report.TestResults) != wantRows || report.Passed != (outcome == "pass" || outcome == "skipped") || report.BuildFailed {
		t.Fatalf("test counts or suite verdict changed: %+v", report)
	}
	for _, row := range report.TestResults {
		wantScope := types.TestObservationScopeAssertion
		if outcome == "skipped" {
			wantScope = types.TestObservationScopeNonAsserting
		}
		if row.ObservationScope != wantScope || row.Passed != (outcome != "failure") || (outcome == "failure" && !strings.Contains(row.FailureDetail, "wrong value")) {
			t.Fatalf("parsed row semantics changed: %+v", row)
		}
	}
	if outcome == "missing" && report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable {
		t.Fatalf("missing receipt is not available evidence: %+v", report)
	}
	if !bytes.Equal(planBefore, b1661JSON(t, plan)) {
		t.Error("execution/diagnostic transport changed the model-owned plan")
	}
	wantReceipts := wantRows
	receipts := b1661Receipts(report.VerificationDiagnostics)
	if len(receipts) != wantReceipts {
		t.Errorf("current parsed-byte receipts lost at report merge: want %d, got %+v", wantReceipts, receipts)
	}
	for i := 0; i < wantReceipts; i++ {
		wire, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("fixture-wire-%d.xml", i)))
		if err != nil {
			t.Fatal(err)
		}
		path, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("fixture-path-%d.txt", i)))
		if err != nil {
			t.Fatal(err)
		}
		if runner == "java" {
			// The binder records the canonical report path; macOS's child PWD
			// may retain /var while the same file resolves through /private/var.
			canonical, err := filepath.EvalSymlinks(string(path))
			if err != nil {
				t.Fatal(err)
			}
			path = []byte(canonical)
		}
		found := false
		for _, receipt := range receipts {
			if strings.Contains(receipt.Detail, fmt.Sprintf("report_path=%q", path)) && strings.Contains(receipt.Detail, fmt.Sprintf("sha256=%x", sha256.Sum256(wire))) {
				found = true
				if receipt.Runner != command.Runner || receipt.Framework != command.Framework || receipt.WorkingDir != command.WorkingDir || receipt.Command != command.Command || receipt.Severity != "info" || !strings.Contains(receipt.Detail, "not additional assertion coverage") {
					t.Errorf("receipt no longer describes the actual source command: %+v", receipt)
				}
			}
		}
		if !found {
			t.Errorf("no preserved receipt for parsed file %s sha256=%x", path, sha256.Sum256(wire))
		}
	}
	before := b1661JSON(t, report)
	var restored types.ChangeReport
	if err := json.Unmarshal(before, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.VerificationDiagnostics, restored.VerificationDiagnostics) {
		t.Error("report JSON changed source diagnostics")
	}
	pack := types.WriteContextPackFromChangeReport(&restored)
	for _, consumer := range []types.WriteContextConsumer{types.WriteConsumerController, types.WriteConsumerPlanner, types.WriteConsumerVerifier} {
		count := 0
		for _, item := range pack.View(consumer, 100).Items {
			if item.Kind == "verification_diagnostic" && strings.Contains(item.Text, "reason_code=junit_current_report_bytes") {
				count++
			}
		}
		if count != wantReceipts {
			t.Errorf("%s context lost current report receipt: want %d, got %d", consumer, wantReceipts, count)
		}
	}
	without := restored
	without.VerificationDiagnostics = nil
	if !reflect.DeepEqual(types.BuildVerificationProofLedger(plan, &restored, nil), types.BuildVerificationProofLedger(plan, &without, nil)) {
		t.Error("audit receipt must not create additional proof")
	}
	if !reflect.DeepEqual(verificationConfidenceRecordsFromReport(plan, &restored), verificationConfidenceRecordsFromReport(plan, &without)) {
		t.Error("audit receipt must not change confidence")
	}
	if !bytes.Equal(before, b1661JSON(t, report)) {
		t.Error("JSON/context/proof consumers changed the installed report")
	}
	if runner == "java" {
		if got := projectTestObservationExecuted(plan.ProjectTestObservations[0], report); got != (outcome == "pass") {
			t.Errorf("receipt changed project assertion proof: got %t for %s", got, outcome)
		}
	}
	return report
}

func b1661Receipts(diagnostics []types.VerificationDiagnostic) []types.VerificationDiagnostic {
	var out []types.VerificationDiagnostic
	for _, diag := range diagnostics {
		if diag.Source == "junit_invocation_report" && diag.Category == "report_binding" && diag.ReasonCode == "junit_current_report_bytes" {
			out = append(out, diag)
		}
	}
	return out
}

func b1661JSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
