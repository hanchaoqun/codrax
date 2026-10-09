package types

import (
	"reflect"
	"strings"
	"testing"
)

func nativeFileExecutionConsumerFixture() (*ChangePlan, *ChangeReport) {
	p, r := existingIntentConsumerFixture()
	p.WriteAnalysisIR.Request.Constraints = nil
	p.AppliedCommitSHA = strings.Repeat("a", 40)
	p.PatchEffect.HeadRef = p.AppliedCommitSHA
	p.PatchEffect.Source = "applied_commit"
	p.PatchEffect.DiffFingerprint = strings.Repeat("b", 64)
	r.ExistingTestExecutions[0].SourcePlanID = p.ID
	r.ExistingTestExecutions[0].AppliedCommitSHA = p.AppliedCommitSHA
	r.ExistingTestExecutions[0].DiffFingerprint = p.PatchEffect.DiffFingerprint
	r.ExecutedCommands[0].InvocationID = "exact-file-invocation"
	r.TestResults[0].InvocationID = "exact-file-invocation"
	return p, r
}

func TestVerifiedNativeTestExecutionPathsScopeAndIdentity(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ChangePlan, *ChangeReport)
	}{
		{"missing_receipt", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions = nil }},
		{"wrong_plan", func(p *ChangePlan, _ *ChangeReport) { p.ID = "foreign" }},
		{"stale_source", func(p *ChangePlan, _ *ChangeReport) { p.AppliedCommitSHA = "other" }},
		{"stale_effect", func(p *ChangePlan, _ *ChangeReport) { p.PatchEffect.DiffFingerprint = "other" }},
		{"wrong_channel", func(_ *ChangePlan, r *ChangeReport) { r.Channel = "" }},
		{"missing_source_owner", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].SourcePlanID = "" }},
		{"invalid_delivery", func(p *ChangePlan, _ *ChangeReport) { p.PatchEffect.Source = "unknown" }},
		{"legacy", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands[0].InvocationID = ""
			r.TestResults[0].InvocationID = ""
		}},
		{"cross_invocation", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].InvocationID = "other" }},
		{"duplicate_invocation", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands = append(r.ExecutedCommands, r.ExecutedCommands[0])
		}},
		{"directory_suite", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands[0].Suite = "tests"
			r.ExistingTestExecutions[0].Suite = "tests"
		}},
		{"unknown_test_bytes", func(_ *ChangePlan, r *ChangeReport) { r.ExistingTestExecutions[0].TestFileSHA256 = "" }},
		{"wrong_result", func(_ *ChangePlan, r *ChangeReport) { r.TestResults[0].AssertionID = "other" }},
		{"skipped", func(_ *ChangePlan, r *ChangeReport) {
			r.TestResults[0].ObservationScope = TestObservationScopeNonAsserting
		}},
		{"failed", func(_ *ChangePlan, r *ChangeReport) {
			r.ExecutedCommands[0].ExitCode = 1
			r.TestResults[0].Passed = false
			r.ExistingTestExecutions[0].FailedAssertionCount = 1
			r.ExistingTestExecutions[0].AssertionDigests[0] = ExistingTestAssertionDigest(r.TestResults[0])
		}},
		{"over_cap", func(_ *ChangePlan, r *ChangeReport) {
			for len(r.ExistingTestExecutions) <= MaxExistingTestExecutionReceipts {
				r.ExistingTestExecutions = append(r.ExistingTestExecutions, r.ExistingTestExecutions[0])
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r := nativeFileExecutionConsumerFixture()
			want := []string{r.ExistingTestExecutions[0].TestPath}
			if got := VerifiedNativeTestExecutionPaths(p, r); !reflect.DeepEqual(got, want) {
				t.Fatalf("valid fixture: %v", got)
			}
			tc.edit(p, r)
			before := existingIntentConsumerBytes(t, []any{p, r})
			if got := VerifiedNativeTestExecutionPaths(p, r); len(got) != 0 {
				t.Fatalf("invalid identity lent file proof: %v", got)
			}
			if after := existingIntentConsumerBytes(t, []any{p, r}); after != before {
				t.Fatal("projection mutated inputs")
			}
		})
	}
}
