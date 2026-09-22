package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

const WriteConstraintRunExistingTest = "run_existing_test"
const MaxRequiredExistingTests = 32
const MaxExistingTestExecutionReceipts = 64
const MaxExistingTestExecutionAssertions = 512
const ExistingTestExecutionCategory = "existing_test_execution"

const WriteExistingTestIntentTeaching = "Use kind=run_existing_test only when the user explicitly requires executing an existing native test file. First read_file that exact current-repository file, then put its single repo-relative path in target. This is an execution requirement, independent from preserve_regression_test, scope anchors, and behavior-contract proof. Do not infer it merely because tests exist. Keep it through planning retries even if behavior examples remain planning-only. The current analyzer has a bounded four-call prescan budget; do not promise arbitrary numbers of test files can be established in one dispatch. Native execution receipts currently support exact Python unittest files; other paths/protocols remain unverified. A probe, syntax check, zero tests, or skipped tests cannot replace this requirement."

// ExistingTestExecutionReceipt is produced from ONE native runner result before
// reports are merged. It is not accepted by any model emission schema. Counts
// describe this exact file-selected invocation, not arbitrary behavior claims.
// CommandIndex binds to the report's existing command ledger; source identity
// prevents an older delivery of the same plan from lending its execution.
type ExistingTestExecutionReceipt struct {
	PlanID               string   `json:"plan_id"`
	AppliedCommitSHA     string   `json:"applied_commit_sha"`
	PatchEffectID        string   `json:"patch_effect_id"`
	DiffFingerprint      string   `json:"diff_fingerprint"`
	TestPath             string   `json:"test_path"`
	CandidateID          string   `json:"candidate_id"`
	Runner               string   `json:"runner"`
	Framework            string   `json:"framework,omitempty"`
	WorkingDir           string   `json:"working_dir"`
	Suite                string   `json:"suite"`
	CommandIndex         int      `json:"command_index"`
	AssertionCount       int      `json:"assertion_count"`
	FailedAssertionCount int      `json:"failed_assertion_count"`
	TestFileSHA256       string   `json:"test_file_sha256"`
	CommandSHA256        string   `json:"command_sha256"`
	AssertionDigests     []string `json:"assertion_digests"`
}

// RequiredExistingTestPaths reads only the pinned analyzer snapshot, never a
// later Mutable request, plan prose, or a retired behavior-contract generation.
func RequiredExistingTestPaths(plan *ChangePlan) []string {
	if plan == nil || plan.WriteAnalysisIR == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range plan.WriteAnalysisIR.Request.Constraints {
		if c.Kind != WriteConstraintRunExistingTest {
			continue
		}
		p := strings.TrimSpace(c.Target)
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
		if len(out) > MaxRequiredExistingTests {
			// Malformed persisted input must remain debt, not disappear at a cap.
			out[MaxRequiredExistingTests] = ""
			break
		}
	}
	sort.Strings(out)
	return out
}

// ExistingTestExactFileSelector is deliberately narrower than directory
// containment. Only existing native protocols with file selectors are admitted.
// Package/class/aggregate selectors need their own exact binding; they remain
// unsupported here rather than borrowing another test's result in the cwd.
func ExistingTestExactFileSelector(runner, framework, workingDir, suite, testPath string) bool {
	if runner != "python" || framework != "unittest" {
		return false
	}
	for _, p := range []string{workingDir, suite, testPath} {
		if p == "" || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || strings.ContainsAny(p, "*?[]{}\n\r\x00") || len(p) > 4096 || path.Clean(p) != p {
			return false
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".." {
				return false
			}
		}
	}
	if path.Clean(testPath) != testPath || path.Clean(suite) != suite || testPath == "." || suite == "." {
		return false
	}
	joined := path.Join(workingDir, suite)
	return joined == testPath && joined != ".." && !strings.HasPrefix(joined, "../")
}

// ExistingTestExecutionConfidence is a fresh projection. Persisted confidence
// labels are never proof; only current-plan, current-delivery native receipts
// can satisfy the independent execution requirement.
func ExistingTestExecutionConfidence(plan *ChangePlan, report *ChangeReport) []VerificationConfidenceRecord {
	targets := RequiredExistingTestPaths(plan)
	if len(targets) == 0 {
		return nil
	}
	var out []VerificationConfidenceRecord
	invocations := NewNativeTestInvocationIndex(report)
	for _, target := range targets {
		status, reason := "missing", "required_existing_test_not_executed"
		if report != nil && len(report.ExistingTestExecutions) <= MaxExistingTestExecutionReceipts {
			for _, receipt := range report.ExistingTestExecutions {
				if receipt.TestPath != target || !existingTestExecutionReceiptMatches(plan, report, receipt, invocations) {
					continue
				}
				if receipt.FailedAssertionCount > 0 {
					status, reason = "failed", "required_existing_test_failed"
					break
				}
				status, reason = "satisfied", "required_existing_test_executed"
			}
		}
		severity := "info"
		if status != "satisfied" {
			severity = "warning"
		}
		out = append(out, VerificationConfidenceRecord{Source: WriteConstraintRunExistingTest + ":" + ExistingTestExecutionDigest(target), Category: ExistingTestExecutionCategory, Status: status, Severity: severity, ReasonCode: reason, Detail: fmt.Sprintf("Required native test file %q: %s. This is execution scope, not independent behavior-contract proof.", target, status)})
	}
	return out
}

func existingTestExecutionReceiptMatches(plan *ChangePlan, report *ChangeReport, r ExistingTestExecutionReceipt, invocations *NativeTestInvocationIndex) bool {
	if plan == nil || plan.PatchEffect == nil || plan.ID == "" || report.PlanID != plan.ID || report.Channel != ChangeReportChannelPostApplyVerify ||
		r.PlanID != plan.ID || plan.AppliedCommitSHA == "" || r.AppliedCommitSHA != plan.AppliedCommitSHA ||
		plan.PatchEffect.PlanID != plan.ID || r.PatchEffectID == "" || r.PatchEffectID != plan.PatchEffect.RecordID ||
		r.DiffFingerprint == "" || r.DiffFingerprint != plan.PatchEffect.DiffFingerprint ||
		r.AssertionCount <= 0 || r.FailedAssertionCount < 0 || r.FailedAssertionCount > r.AssertionCount ||
		r.CommandIndex < 0 || r.CommandIndex >= len(report.ExecutedCommands) || report.TestSurface == nil {
		return false
	}
	if len(r.TestFileSHA256) != 64 || len(r.AssertionDigests) != r.AssertionCount || r.AssertionCount > MaxExistingTestExecutionAssertions {
		return false
	}
	if _, err := hex.DecodeString(r.TestFileSHA256); err != nil {
		return false
	}
	if !ExistingTestExactFileSelector(r.Runner, r.Framework, r.WorkingDir, r.Suite, r.TestPath) {
		return false
	}
	cmd := report.ExecutedCommands[r.CommandIndex]
	if cmd.Runner != r.Runner || cmd.Framework != r.Framework || cmd.WorkingDir != r.WorkingDir || cmd.Suite != r.Suite ||
		cmd.Outcome != ExecutedCommandOutcomeExecuted || cmd.ProbeExecution != nil || cmd.SourceCheckExecution != nil ||
		r.CommandSHA256 != ExistingTestExecutionDigest(cmd.Command) || cmd.Command == "" ||
		(cmd.ExitCode == 0) != (r.FailedAssertionCount == 0) {
		return false
	}
	rows := make(map[string][]bool)
	for resultIndex, row := range report.TestResults {
		if invocations.Matches(r.CommandIndex, resultIndex) && row.ObservationScope == TestObservationScopeAssertion && row.AssertionID != "" {
			digest := ExistingTestAssertionDigest(row)
			rows[digest] = append(rows[digest], row.Passed)
		}
	}
	failures := 0
	for _, digest := range r.AssertionDigests {
		matches := rows[digest]
		if len(matches) == 0 {
			return false
		}
		if !matches[0] {
			failures++
		}
		rows[digest] = matches[1:]
	}
	if failures != r.FailedAssertionCount {
		return false
	}
	matches := 0
	for _, c := range report.TestSurface.Candidates {
		if c.ID == r.CandidateID && c.HasTestSignal && c.Runner == r.Runner && c.Framework == r.Framework && c.WorkingDir == r.WorkingDir {
			matches++
		}
	}
	return matches == 1
}

func ExistingTestExecutionDigest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Digest the retained native identity and verdict, not presentation details or
// elapsed time. A receipt cannot lend assertion counts absent from this report.
func ExistingTestAssertionDigest(row TestResult) string {
	value, _ := json.Marshal([]any{row.Kind, row.ObservationScope, row.AssertionID, row.Suite, row.Passed})
	return ExistingTestExecutionDigest(string(value))
}

// EffectiveExistingTestExecutionReport does not alter history. Its owned fields
// are the independent execution confidence and, when missing, completion status.
// Native failures remain failures; an unavailable suite cannot be replaced by a
// successful probe. Legacy plans without this requirement are byte-compatible.
func EffectiveExistingTestExecutionReport(plan *ChangePlan, report *ChangeReport) *ChangeReport {
	if report == nil || len(RequiredExistingTestPaths(plan)) == 0 {
		return report
	}
	out := *report
	out.VerificationConfidence = make([]VerificationConfidenceRecord, 0, len(report.VerificationConfidence)+1)
	for _, rec := range report.VerificationConfidence {
		if rec.Category != ExistingTestExecutionCategory {
			out.VerificationConfidence = append(out.VerificationConfidence, rec)
		}
	}
	records := ExistingTestExecutionConfidence(plan, report)
	out.VerificationConfidence = append(out.VerificationConfidence, records...)
	missing, failed := false, false
	for _, rec := range records {
		missing = missing || rec.Status != "satisfied"
		failed = failed || rec.Status == "failed"
	}
	if failed && (out.Passed || out.FailureKind == FailureKindVerificationIncomplete || out.FailureKind == "") {
		out.Passed = false
		out.VerificationStatus = VerificationStatusFailed
		out.FailureKind = FailureKindTestsFailed
		out.FailureReasonCode = "required_existing_test_failed"
		out.FailureSummary = "Required existing native test assertions failed."
	}
	if missing && out.Passed {
		out.Passed = false
		out.VerificationStatus = VerificationStatusUnavailable
		out.FailureKind = FailureKindVerificationIncomplete
		out.FailureReasonCode = "required_existing_test_not_executed"
		out.FailureSummary = "Required existing native test execution is unverified; a bounded probe does not replace the requested test file."
	}
	return &out
}
