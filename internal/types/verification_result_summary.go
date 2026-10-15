package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// VerificationResultSummary counts recorded result rows, not independent tests,
// methods, subtests or behavior proofs. Aggregate and assertion rows can overlap;
// a probe can also be an assertion. NonAsserting includes skip/pending/expected
// failure observations and must not be narrowed to "skipped" from prose.
type VerificationResultSummary struct {
	Total        int `json:"total"`
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	NonAsserting int `json:"non_asserting"`
}

// SummarizeVerificationResults returns nil for an absent report. An existing
// report with no rows has a known zero recorded-result count, not zero tests run.
func SummarizeVerificationResults(report *ChangeReport) *VerificationResultSummary {
	if report == nil {
		return nil
	}
	out := &VerificationResultSummary{Total: len(report.TestResults)}
	for _, row := range report.TestResults {
		switch {
		case row.ObservationScope == TestObservationScopeNonAsserting:
			out.NonAsserting++
		case row.Passed:
			out.Passed++
		default:
			out.Failed++
		}
	}
	return out
}

func (s VerificationResultSummary) valid() bool {
	return s.Total >= 0 && s.Passed >= 0 && s.Failed >= 0 && s.NonAsserting >= 0 &&
		s.Passed <= s.Total && s.Failed <= s.Total-s.Passed && s.NonAsserting == s.Total-s.Passed-s.Failed
}

// RecordedResultsText is shared by verification cards and durable finish facts.
// The all-pass wording preserves the existing card contract, including zero rows.
func (s VerificationResultSummary) RecordedResultsText(zh bool) string {
	if s.Passed == s.Total {
		if zh {
			return fmt.Sprintf("%d 条验证结果已记录为通过", s.Total)
		}
		return fmt.Sprintf("%d verification result(s) recorded as passed", s.Total)
	}
	if zh {
		return fmt.Sprintf("已记录 %d 条验证结果：%d 条标记通过，%d 条标记失败，%d 条非断言结果", s.Total, s.Passed, s.Failed, s.NonAsserting)
	}
	return fmt.Sprintf("%d verification result(s) recorded: %d marked passed, %d marked failed, %d non-asserting", s.Total, s.Passed, s.Failed, s.NonAsserting)
}

// WriteWorkflowVerificationResults is a persisted display snapshot from the
// report consumed by one verify attempt. ReportSHA256 identifies that report's
// canonical JSON for audit (not the pretty-printed file bytes); it is not a
// signature or proof authorization. No state,
// risk, verification or finish gate may consume this display-only snapshot.
type WriteWorkflowVerificationResults struct {
	AttemptID    string                    `json:"attempt_id"`
	PlanID       string                    `json:"plan_id"`
	ReportID     string                    `json:"report_id"`
	ReportSHA256 string                    `json:"report_sha256"`
	Results      VerificationResultSummary `json:"results"`
}

// BindWriteWorkflowVerificationResults snapshots only the exact current-plan,
// post-apply report. Missing/legacy/mismatched sources stay unknown, not zero.
func BindWriteWorkflowVerificationResults(planID string, attempt WriteWorkflowAttempt, report *ChangeReport) *WriteWorkflowVerificationResults {
	if report == nil || strings.TrimSpace(planID) == "" || report.PlanID != planID ||
		report.Channel != ChangeReportChannelPostApplyVerify || attempt.Kind != "verify" ||
		attempt.ID == "" || attempt.PlanID != planID || attempt.ReportID == "" || attempt.ArtifactRef != attempt.ReportID {
		return nil
	}
	wire, err := json.Marshal(report)
	if err != nil {
		return nil
	}
	digest := sha256.Sum256(wire)
	return &WriteWorkflowVerificationResults{AttemptID: attempt.ID, PlanID: planID, ReportID: attempt.ReportID,
		ReportSHA256: hex.EncodeToString(digest[:]), Results: *SummarizeVerificationResults(report)}
}

func normalizeWriteWorkflowVerificationResults(in *WriteWorkflowVerificationResults, attempt WriteWorkflowAttempt) *WriteWorkflowVerificationResults {
	if in == nil {
		return nil
	}
	out := *in
	digest, err := hex.DecodeString(out.ReportSHA256)
	if err != nil || len(digest) != sha256.Size || !out.Results.valid() || attempt.Kind != "verify" ||
		out.AttemptID == "" || out.AttemptID != attempt.ID || out.PlanID == "" || out.PlanID != attempt.PlanID ||
		out.ReportID == "" || out.ReportID != attempt.ReportID || out.ReportID != attempt.ArtifactRef {
		return nil
	}
	return &out
}

// CurrentWorkflowVerificationResults does not fall back to an older successful
// attempt or combine overlapping invocations/batches. It is display-only.
func CurrentWorkflowVerificationResults(batch WriteWorkflowBatch) *WriteWorkflowVerificationResults {
	for i := len(batch.Attempts) - 1; i >= 0; i-- {
		attempt := batch.Attempts[i]
		if attempt.Kind == "apply" {
			return nil
		}
		if attempt.Kind != "verify" {
			continue
		}
		out := normalizeWriteWorkflowVerificationResults(attempt.VerificationResults, attempt)
		if out == nil || batch.PlanID != out.PlanID || batch.VerifyRef != out.ReportID {
			return nil
		}
		return out
	}
	return nil
}
