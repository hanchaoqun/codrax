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

// These are real Python and Make executions, not a simulated runner or a
// model-comparator verdict. The observation is allowed to explain a failure,
// but must not alter the existing continuation or authority policy.
func TestRunTestsFailureObservationPublicHandoff(t *testing.T) {
	for _, binary := range []string{"python3", "make"} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Skipf("native protocol fixture requires %s: %v", binary, err)
		}
	}
	for _, tc := range []struct {
		name           string
		failedProbes   int
		suiteFails     bool
		artifactDenied bool
	}{
		{name: "failed_probe_then_native_suite_passes", failedProbes: 1},
		{name: "same_class_failures_keep_old_no_continuation", failedProbes: 2},
		{name: "native_suite_failure_stays_failed", failedProbes: 1, suiteFails: true},
		{name: "artifact_unwritable_does_not_change_suite_pass", failedProbes: 1, artifactDenied: true},
		{name: "artifact_unwritable_does_not_change_suite_failure", failedProbes: 1, suiteFails: true, artifactDenied: true},
		{name: "native_only_does_not_invent_observations"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			expected := 41
			if tc.suiteFails {
				expected = 99
			}
			files := map[string]string{
				"widget.py":           "VALUE = 41\n",
				"tests/__init__.py":   "",
				"tests/test_value.py": fmt.Sprintf("import unittest\nimport widget\n\nclass ValueTest(unittest.TestCase):\n    def test_value(self):\n        self.assertEqual(widget.VALUE, %d)\n", expected),
				"Makefile":            ".PHONY: check\ncheck: widget.py tests/test_value.py\n\tPYTHONPATH=. python3 -B -m unittest tests.test_value -v\n",
			}
			for path, content := range files {
				full := filepath.Join(root, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			workDir := t.TempDir()
			if tc.artifactDenied {
				workDir = filepath.Join(workDir, "not-a-directory")
				if err := os.WriteFile(workDir, []byte("owned failure-injection sentinel"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			plan := &types.ChangePlan{
				ID: "plan-failure-observation", Status: types.PlanStatusApplied,
				TargetPaths: []string{"widget.py"}, AppliedPaths: []string{"widget.py"},
				Changes: []types.FileChange{{Path: "widget.py", Kind: "modify", NewContent: files["widget.py"]}},
			}
			for i := 0; i < tc.failedProbes; i++ {
				id := fmt.Sprintf("wrong-comparator-%d", i+1)
				plan.VerificationProbes = append(plan.VerificationProbes, types.VerificationProbe{
					ID: id, Language: "python", WorkingDir: ".", TimeoutSeconds: 5,
					Code:              fmt.Sprintf("import widget\nprint('raw-output-%s', flush=True)\nassert widget.VALUE == %d, 'detail-%s expected %d got %%r' %% widget.VALUE\n", id, 42+i, id, 42+i),
					ChangedSymbolRefs: []string{"path:widget.py"},
				})
			}
			mu := types.NewMutableState("failure observation handoff")
			mu.SetChangePlan(plan)
			ctx := &types.BusContext{Mutable: mu, Mode: types.ModeApply, PipelineStage: types.StageVerify, RepoRoot: root, MainRepoRoot: root, WorkDir: workDir}
			planBefore := failureObservationJSON(t, plan)
			result, err := (&RunTests{}).Execute(ctx, json.RawMessage(`{"runner":"make","suite":"check"}`))
			if err != nil {
				t.Fatalf("public Execute: %v", err)
			}
			report := mu.ChangeReport()
			if report == nil {
				t.Fatal("public runner did not install report")
			}
			// Freeze the actual durable JSON boundary before inspecting either
			// the new observational carrier or its existing authority consumers.
			wire := failureObservationJSON(t, report)
			var restored types.ChangeReport
			if err := json.Unmarshal(wire, &restored); err != nil {
				t.Fatal(err)
			}
			report = &restored
			wantSuite := tc.failedProbes < 2
			wantPass := wantSuite && !tc.suiteFails
			if result.Success != wantPass || report.Passed != wantPass {
				t.Fatalf("observation changed process/suite verdict: success=%t passed=%t want=%t report=%s", result.Success, report.Passed, wantPass, wire)
			}
			wantRows := 1
			if !wantSuite {
				wantRows = tc.failedProbes
			}
			if len(report.TestResults) != wantRows || countFailed(report.TestResults) != map[bool]int{true: 0, false: wantRows}[wantPass] {
				t.Fatalf("original aggregate/probe counts changed: %+v", report.TestResults)
			}
			if wantSuite && report.TestResults[0].ObservationScope != types.TestObservationScopeAggregate {
				t.Errorf("Make aggregate row was promoted to an assertion: %+v", report.TestResults)
			}
			suiteRan, continued, failedCommands := false, false, 0
			for _, cmd := range report.ExecutedCommands {
				if cmd.Runner == "make" && cmd.Outcome == types.ExecutedCommandOutcomeExecuted {
					suiteRan = true
				}
				if cmd.Source == verificationProbeContinuationSourceProbeSuiteContinued {
					continued = true
					if cmd.ReasonCode != "probe_non_authoritative" {
						t.Errorf("unexpected continuation reason: %+v", cmd)
					}
				}
				if cmd.Runner == "verification_probe" && cmd.ExitCode != 0 {
					failedCommands++
				}
			}
			if suiteRan != wantSuite || continued != (tc.failedProbes == 1) || failedCommands != tc.failedProbes {
				t.Fatalf("old continuation/real failures changed: suite=%t continuation=%t failedProbes=%d commands=%+v", suiteRan, continued, failedCommands, report.ExecutedCommands)
			}
			var observations []types.VerificationFailureObservation
			comparatorRows := 0
			for _, diag := range report.VerificationDiagnostics {
				if diag.Category != "probe_comparator_authority" {
					continue
				}
				comparatorRows++
				if diag.Severity != "warning" || diag.Outcome != "observed_failure" || diag.ReasonCode != "model_authored_probe_comparator_unverified" {
					t.Errorf("observation changed classification: %+v", diag)
				}
				observations = append(observations, diag.FailureObservations...)
			}
			wantComparatorRows := 0
			if tc.failedProbes > 0 {
				wantComparatorRows = 1 // Keep the old same-class dedup/count policy.
			}
			if comparatorRows != wantComparatorRows {
				t.Errorf("outer diagnostic count changed: got=%d want=%d", comparatorRows, wantComparatorRows)
			}
			if len(observations) != tc.failedProbes {
				t.Errorf("non-authoritative failure details lost after Execute/JSON/dedup: got=%+v want=%d", observations, tc.failedProbes)
			}
			seen := map[string]bool{}
			for _, observation := range observations {
				knownID := false
				for _, probe := range plan.VerificationProbes {
					knownID = knownID || probe.ID == observation.AssertionID
				}
				if !knownID {
					t.Errorf("observation invented an assertion identity: %+v", observation)
				}
				if seen[observation.AssertionID] {
					t.Errorf("duplicate observed failure %q", observation.AssertionID)
				}
				seen[observation.AssertionID] = true
				if observation.Suite != "verification_probe/python" || !strings.Contains(observation.FailureDetail, "detail-"+observation.AssertionID) || !strings.Contains(observation.FailureDetail, "got 41") {
					t.Errorf("original assertion/suite/detail not retained: %+v", observation)
				}
				for _, testResult := range report.TestResults {
					if testResult.AssertionID == observation.AssertionID && testResult.FailureDetail != observation.FailureDetail {
						t.Errorf("observation rewrote original failure detail: original=%q observation=%q", testResult.FailureDetail, observation.FailureDetail)
					}
				}
				if tc.artifactDenied {
					if observation.OutputRef != "" {
						t.Errorf("failed artifact write fabricated ref: %+v", observation)
					}
					continue
				}
				if observation.OutputRef == "" {
					t.Errorf("short original output has no complete readable ref: %+v", observation)
					continue
				}
				raw, readErr := os.ReadFile(observation.OutputRef)
				if readErr != nil {
					t.Errorf("output ref unreadable: %v", readErr)
					continue
				}
				if len(raw) >= 32768 || !bytes.Contains(raw, []byte("raw-output-"+observation.AssertionID)) || !bytes.Contains(raw, []byte("detail-"+observation.AssertionID)) {
					t.Errorf("short output artifact incomplete: bytes=%d raw=%q", len(raw), raw)
				}
				readResult, readErr := (&ReadFile{}).Execute(ctx, failureObservationJSON(t, map[string]any{"path": observation.OutputRef}))
				if readErr != nil || !readResult.Success || !strings.Contains(readResult.Summary, "detail-"+observation.AssertionID) {
					t.Errorf("published output ref is not readable through the public tool: err=%v result=%+v", readErr, readResult)
				}
			}
			// Display rows are observational only. Strip them in a copy and
			// compare existing authority and continuation consumers exactly.
			var without types.ChangeReport
			if err := json.Unmarshal(wire, &without); err != nil {
				t.Fatal(err)
			}
			for i := range without.VerificationDiagnostics {
				without.VerificationDiagnostics[i].FailureObservations = nil
			}
			if !reflect.DeepEqual(types.EffectiveVerificationConfidence(plan, report), types.EffectiveVerificationConfidence(plan, &without)) ||
				!reflect.DeepEqual(types.BuildVerificationProofLedger(plan, report, nil), types.BuildVerificationProofLedger(plan, &without, nil)) ||
				verificationProbeDiagnosticsAreNonAuthoritative(report) != verificationProbeDiagnosticsAreNonAuthoritative(&without) {
				t.Error("failure display metadata changed existing proof/confidence/continuation authority")
			}
			if !bytes.Equal(wire, failureObservationJSON(t, report)) || !bytes.Equal(planBefore, failureObservationJSON(t, plan)) {
				t.Error("observation consumers mutated the original plan/report")
			}
			for path, content := range files {
				after, readErr := os.ReadFile(filepath.Join(root, path))
				if readErr != nil || string(after) != content {
					t.Errorf("original fixture source changed: %s err=%v", path, readErr)
				}
			}
			if tc.artifactDenied {
				untouched, readErr := os.ReadFile(workDir)
				if readErr != nil || string(untouched) != "owned failure-injection sentinel" {
					t.Errorf("artifact failure overwrote the blocking file: %q err=%v", untouched, readErr)
				}
			}
		})
	}
}

func failureObservationJSON(t *testing.T, value any) []byte {
	t.Helper()
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}
