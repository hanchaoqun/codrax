package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWriteExistingTestIntentPublicNativeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, path, body            string
		extras                      map[string]string
		wantSatisfied               bool
		wantAssertions, wantSkipped int
	}{
		{name: "non_conventional_filename_with_native_candidate", path: "packages/widget/checks/arbitrary_cases.py", body: existingTestIntentPublicTests, extras: map[string]string{"packages/widget/checks/__init__.py": "", "packages/widget/tests/test_other.py": "import unittest\nclass Other(unittest.TestCase):\n    def test_wrong_file(self): self.fail('not selected')\n"}, wantSatisfied: true, wantAssertions: 3},
		{name: "literal_dollar_filename", path: "packages/widget/tests/test_$FLAVOR.py", body: existingTestIntentPublicTests, extras: map[string]string{"packages/widget/tests/test_other.py": "raise RuntimeError('wrong file selected')\n"}, wantSatisfied: true, wantAssertions: 3},
		{name: "all_skipped", body: strings.Replace(existingTestIntentPublicTests, "class IncrementTest", "@unittest.skip('intentional skip')\nclass IncrementTest", 1), wantSkipped: 3},
		{name: "mixed_skipped", body: strings.Replace(existingTestIntentPublicTests, "    def test_zero", "    @unittest.skip('intentional skip')\n    def test_zero", 1), wantSatisfied: true, wantAssertions: 2, wantSkipped: 1},
		{name: "zero_tests", body: "import unittest\n"},
		{name: "load_tests_redirects_to_other_file", body: "import unittest\nfrom tests import other\ndef load_tests(loader, tests, pattern):\n    return loader.loadTestsFromModule(other)\n", extras: map[string]string{"packages/widget/tests/other.py": existingTestIntentPublicTests}, wantAssertions: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FLAVOR", "other")
			path := tc.path
			if path == "" {
				path = existingTestIntentPublicTestPath
			}
			got := runExistingTestIntentPublicFixture(t, types.WriteConstraintRunExistingTest, false, false, path, tc.body, tc.extras)
			if got.plan == nil || got.report == nil || got.verifyCalls != 1 {
				t.Fatalf("actual Run did not reach verification: %+v", got)
			}
			confidence := types.ExistingTestExecutionConfidence(got.plan, got.report)
			if len(confidence) != 1 || (confidence[0].Status == "satisfied") != tc.wantSatisfied {
				t.Fatalf("native file execution=%+v wantSatisfied=%v report=%+v", confidence, tc.wantSatisfied, got.report)
			}
			if got.report.Passed != tc.wantSatisfied {
				t.Fatalf("execution debt did not govern report: report=%v workflow=%s", got.report.Passed, got.workflowStatus)
			}
			if !tc.wantSatisfied && (got.report.NormalizeVerificationStatus() != types.VerificationStatusUnavailable || got.completionVerdict != types.WriteWorkflowCompletionUnverified) {
				t.Fatalf("transparent unverified completion required: report=%s completion=%s", got.report.NormalizeVerificationStatus(), got.completionVerdict)
			}
			if tc.wantSatisfied && got.workflowStatus != types.WriteWorkflowRunComplete {
				t.Fatalf("successful execution did not complete: %s", got.workflowStatus)
			}
			ledger := types.BuildVerificationProofLedger(got.plan, got.report, nil)
			found := false
			for _, obligation := range ledger.Obligations {
				if obligation.Category == types.ExistingTestExecutionCategory {
					found = true
					if (obligation.Status == types.VerificationProofLedgerItemCovered) != tc.wantSatisfied {
						t.Fatalf("execution obligation lost: %+v", obligation)
					}
				}
			}
			if !found {
				t.Fatal("execution target vanished from final proof ledger")
			}
			if len(types.CoveredWriteBehaviorContractIDs(got.plan.BehaviorContracts, got.report.VerificationConfidence)) != 0 {
				t.Fatal("execution scope promoted planning-only behavior")
			}
			assertions, skipped, probePassed := 0, 0, false
			for _, row := range got.report.TestResults {
				if row.Suite == "verification_probe/python" {
					probePassed = probePassed || row.Passed
					continue
				}
				if row.ObservationScope == types.TestObservationScopeAssertion {
					assertions++
				}
				if row.ObservationScope == types.TestObservationScopeNonAsserting {
					skipped++
				}
			}
			if !probePassed || assertions != tc.wantAssertions || skipped != tc.wantSkipped {
				t.Fatalf("native result accounting changed: probe=%v assertions=%d skipped=%d", probePassed, assertions, skipped)
			}
			if tc.wantSatisfied && (len(got.report.ExistingTestExecutions) != 1 || got.report.ExistingTestExecutions[0].AssertionCount != tc.wantAssertions) {
				t.Fatalf("receipt overstated skipped/other-file results: %+v", got.report.ExistingTestExecutions)
			}
			t.Logf("ACTUAL_RUN_BOUNDARY native_assertions=%d skipped=%d confidence=%s report=%s workflow=%s", assertions, skipped, confidence[0].Status, got.report.NormalizeVerificationStatus(), got.workflowStatus)
		})
	}
}

func TestWriteExistingTestIntentPublicNativeFixtureEvents(t *testing.T) {
	for _, tc := range []struct {
		name, body               string
		failure                  types.FailureKind
		assertions, nonAsserting int
	}{
		{name: "set_up_class_error", body: "import unittest\nclass Tests(unittest.TestCase):\n    @classmethod\n    def setUpClass(cls): raise RuntimeError('native setup error')\n    def test_value(self): self.assertTrue(True)\n", failure: types.FailureKindTestsFailed, nonAsserting: 1},
		{name: "tear_down_class_error", body: "import unittest\nclass Tests(unittest.TestCase):\n    @classmethod\n    def tearDownClass(cls): raise RuntimeError('native teardown error')\n    def test_value(self): self.assertTrue(True)\n", failure: types.FailureKindTestsFailed, assertions: 1, nonAsserting: 1},
		{name: "class_setup_skip", body: "import unittest\nclass Tests(unittest.TestCase):\n    @classmethod\n    def setUpClass(cls): raise unittest.SkipTest('native class skip')\n    def test_value(self): self.assertTrue(True)\n", failure: types.FailureKindVerificationIncomplete, nonAsserting: 1},
		{name: "loader_import_error", body: "import unittest\nraise ImportError('native import unavailable')\n", failure: types.FailureKindParserError, assertions: 1},
		{name: "observer_overflow_keeps_native_failure", body: "import unittest\nclass Tests(unittest.TestCase):\n    def test_failed(self): self.fail('native failure survives observer overflow')\ndef passed(self): self.assertTrue(True)\nfor i in range(512): setattr(Tests, 'test_%04d' % i, passed)\n", failure: types.FailureKindTestsFailed, assertions: 513},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runExistingTestIntentPublicFixture(t, types.WriteConstraintRunExistingTest, false, false, existingTestIntentPublicTestPath, tc.body, nil)
			if got.report == nil || got.report.Passed || got.report.FailureKind != tc.failure {
				t.Fatalf("native classification lost: report=%+v", got.report)
			}
			confidence := types.ExistingTestExecutionConfidence(got.plan, got.report)
			if len(confidence) != 1 || confidence[0].Status != "missing" || len(got.report.ExistingTestExecutions) != 0 {
				t.Fatalf("fixture event borrowed execution: %+v", confidence)
			}
			assertions, nonAsserting := 0, 0
			for _, row := range got.report.TestResults {
				if row.Suite == "verification_probe/python" {
					continue
				}
				if row.ObservationScope == types.TestObservationScopeAssertion {
					assertions++
				}
				if row.ObservationScope == types.TestObservationScopeNonAsserting {
					nonAsserting++
				}
			}
			if assertions != tc.assertions || nonAsserting != tc.nonAsserting {
				t.Fatalf("native rows mismatch: %+v", got.report.TestResults)
			}
			if tc.failure == types.FailureKindTestsFailed && got.workflowStatus == types.WriteWorkflowRunComplete {
				t.Fatal("actual native error became successful completion")
			}
			if tc.failure == types.FailureKindParserError && (got.report.FailureReasonCode != "unittest_loader_import_error" || got.completionVerdict != types.WriteWorkflowCompletionUnverified) {
				t.Fatalf("loader error classification changed: %+v", got.report)
			}
			if tc.failure == types.FailureKindTestsFailed || tc.failure == types.FailureKindParserError {
				traceback := false
				for _, row := range got.report.TestResults {
					traceback = traceback || strings.Contains(row.FailureDetail, "Traceback")
				}
				if !traceback {
					t.Fatal("native failure traceback lost")
				}
			}
		})
	}
}

func TestWriteExistingTestIntentPublicNativeTimeout(t *testing.T) {
	got := runExistingTestIntentPublicFixture(t, types.WriteConstraintRunExistingTest, false, false, existingTestIntentPublicTestPath, "import unittest, time\nclass Tests(unittest.TestCase):\n    def test_value(self):\n        time.sleep(5)\n        self.assertTrue(True)\n", nil, map[string]any{"timeout_seconds": 1})
	if got.report == nil || got.report.Passed || got.report.FailureKind != types.FailureKindTimeout {
		t.Fatalf("native timeout downgraded: %+v", got.report)
	}
	probePassed := false
	// The existing early-timeout report keeps executed commands, not the
	// pre-suite result rows. Verify the actual completed probe invocation.
	for _, command := range got.report.ExecutedCommands {
		if command.Runner == "verification_probe" && command.Source == "pre_suite_verification_probe" && command.ProbeExecution != nil && command.Outcome == types.ExecutedCommandOutcomeExecuted && command.ExitCode == 0 {
			probePassed = true
		}
	}
	if !probePassed {
		t.Fatal("fixture probe must genuinely pass before timeout")
	}
	confidence := types.ExistingTestExecutionConfidence(got.plan, got.report)
	if len(confidence) != 1 || confidence[0].Status != "missing" || len(got.report.ExistingTestExecutions) != 0 {
		t.Fatalf("timeout borrowed execution: %+v", confidence)
	}
	if got.completionVerdict == types.WriteWorkflowCompletionVerified {
		t.Fatal("timeout became all-verified")
	}
}
