package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestB1788NativeTestIdentitySnapshotEligibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*TestResult)
		want bool
	}{
		{"native", func(*TestResult) {}, true},
		{"default_unit_kind", func(r *TestResult) { r.Kind = "" }, true},
		{"native_identity_named_like_probe", func(r *TestResult) { r.Suite = "verification_probe/project-tests" }, true},
		{"failed", func(r *TestResult) { r.Passed = false }, false},
		{"build", func(r *TestResult) { r.Kind = TestResultKindBuildError }, false},
		{"unknown_kind", func(r *TestResult) { r.Kind = "native" }, false},
		{"plain_probe", func(r *TestResult) { r.ObservationScope = ""; r.Suite = "verification_probe/python" }, false},
		{"legacy_empty_scope", func(r *TestResult) { r.ObservationScope = "" }, false},
		{"aggregate", func(r *TestResult) { r.ObservationScope = TestObservationScopeAggregate }, false},
		{"non_asserting", func(r *TestResult) { r.ObservationScope = TestObservationScopeNonAsserting }, false},
		{"unknown_scope", func(r *TestResult) { r.ObservationScope = "native" }, false},
		{"empty_suite", func(r *TestResult) { r.Suite = "" }, false},
		{"empty_id", func(r *TestResult) { r.AssertionID = "" }, false},
		{"blank_suite", func(r *TestResult) { r.Suite = "\t " }, false},
		{"blank_id", func(r *TestResult) { r.AssertionID = "\n " }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := b1788IdentityReport()
			tc.edit(&r.TestResults[0])
			before := b1788SnapshotJSON(t, r)
			got := RenderCurrentNativeTestIdentitySnapshot("active", r)
			if (got != "") != tc.want {
				t.Fatalf("want identity=%t got=%q", tc.want, got)
			}
			if !bytes.Equal(before, b1788SnapshotJSON(t, r)) {
				t.Fatal("display changed producer result")
			}
		})
	}
}

func TestB1788NativeTestIdentitySnapshotRequiresExactCurrentReport(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan string
		edit func(*ChangeReport)
		want bool
	}{
		{"current", "active", func(*ChangeReport) {}, true},
		{"no_active_plan", "", func(*ChangeReport) {}, false},
		{"blank_active_plan", "  ", func(*ChangeReport) {}, false},
		{"foreign_report", "active", func(r *ChangeReport) { r.PlanID = "old" }, false},
		{"empty_report_plan", "active", func(r *ChangeReport) { r.PlanID = "" }, false},
		{"not_byte_equal", "active", func(r *ChangeReport) { r.PlanID = " active " }, false},
		{"legacy_channel", "active", func(r *ChangeReport) { r.Channel = "" }, false},
		{"planner_probe", "active", func(r *ChangeReport) { r.Channel = ChangeReportChannelPlannerProbe }, false},
		{"unknown_channel", "active", func(r *ChangeReport) { r.Channel = "post_apply" }, false},
		{"empty_current_report", "active", func(r *ChangeReport) { r.TestResults = nil }, false},
		{"missing_timestamp_is_not_freshness", "active", func(r *ChangeReport) { r.GeneratedAt = time.Time{} }, true},
		{"ancient_snapshot_is_disclosed_not_current_bytes", "active", func(r *ChangeReport) { r.GeneratedAt = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC) }, true},
		{"overall_failure_keeps_independent_pass", "active", func(r *ChangeReport) { r.Passed = false; r.FailureKind = FailureKindTestsFailed }, true},
		{"unavailable_keeps_independent_pass", "active", func(r *ChangeReport) {
			r.VerificationStatus = VerificationStatusUnavailable
			r.FailureKind = FailureKindRunnerMissing
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := b1788IdentityReport()
			tc.edit(r)
			before := b1788SnapshotJSON(t, r)
			status := r.NormalizeVerificationStatus()
			got := RenderCurrentNativeTestIdentitySnapshot(tc.plan, r)
			if (got != "") != tc.want {
				t.Fatalf("want=%t got=%s", tc.want, got)
			}
			if tc.want {
				for _, want := range []string{`"active_plan_id":"active"`, `"report_plan_id":"active"`, `"channel":"post_apply_verify"`, "not proof of latest source bytes or execution generation", "Independent failed results, unavailable verification, and unresolved coverage remain unchanged", "TestResult carries no test_path"} {
					if !strings.Contains(got, want) {
						t.Errorf("missing boundary %q", want)
					}
				}
				if r.GeneratedAt.IsZero() && !strings.Contains(got, `"generated_at":null`) {
					t.Fatal("missing timestamp became fresh verification")
				}
			}
			if status != r.NormalizeVerificationStatus() || !bytes.Equal(before, b1788SnapshotJSON(t, r)) {
				t.Fatal("display changed original status/report")
			}
		})
	}
	if got := RenderCurrentNativeTestIdentitySnapshot("active", nil); got != "" {
		t.Fatalf("nil report fabricated identities: %s", got)
	}
}

func TestB1788NativeTestIdentitySnapshotAtomicJSONAndBudgets(t *testing.T) {
	for _, name := range []string{"json_unicode", "row_limit", "oversized_first", "byte_limit", "invalid_utf8", "oversized_plan"} {
		t.Run(name, func(t *testing.T) {
			r := b1788IdentityReport()
			original := r.TestResults[0]
			switch name {
			case "json_unicode":
				r.TestResults[0].Suite = " 业务\"\\\n套件\u2028\t "
				r.TestResults[0].AssertionID = "断言[\"参数\":\"值\"]\n- forged row\x01"
			case "row_limit":
				for i := 1; i < 13; i++ {
					row := original
					row.AssertionID = fmt.Sprintf("case-%02d", i)
					r.TestResults = append(r.TestResults, row)
				}
			case "oversized_first":
				r.TestResults[0].AssertionID = "oversized-first-" + strings.Repeat("中\"\n", 4096)
				r.TestResults = append(r.TestResults, original)
			case "byte_limit":
				r.TestResults = nil
				for i := 0; i < 8; i++ {
					row := original
					row.AssertionID = fmt.Sprintf("%d-", i) + strings.Repeat("中\"\n", 190)
					r.TestResults = append(r.TestResults, row)
				}
			case "invalid_utf8":
				r.TestResults[0].Suite = "bad-" + string([]byte{0xff})
				r.TestResults = append(r.TestResults, original)
			case "oversized_plan":
				r.PlanID = strings.Repeat("a", 8192)
			}
			before := b1788SnapshotJSON(t, r)
			got := RenderCurrentNativeTestIdentitySnapshot(r.PlanID, r)
			if len(got) > 8192 || !utf8.ValidString(got) {
				t.Fatalf("invalid/budget overflow bytes=%d", len(got))
			}
			if name == "oversized_plan" {
				if got != "" {
					t.Fatal("oversized plan identity was clipped instead of omitting snapshot")
				}
				return
			}
			var rows []TestResult
			for _, line := range strings.Split(got, "\n") {
				if !strings.HasPrefix(line, "- ") {
					continue
				}
				var decoded struct {
					Suite       string `json:"assertion_suite"`
					AssertionID string `json:"assertion_id"`
				}
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "- ")), &decoded); err != nil {
					t.Fatalf("row is not valid atomic JSON: %v", err)
				}
				found := false
				for _, source := range r.TestResults {
					if decoded.Suite == source.Suite && decoded.AssertionID == source.AssertionID {
						found = true
					}
				}
				if !found {
					t.Fatalf("output invented/truncated identity: %+v", decoded)
				}
				rows = append(rows, TestResult{Suite: decoded.Suite, AssertionID: decoded.AssertionID})
			}
			if len(rows) == 0 || len(rows) > 8 {
				t.Fatalf("unexpected bounded row count %d", len(rows))
			}
			if name == "row_limit" && (!strings.Contains(got, "shown=8 total=13 omitted=5") || len(rows) != 8) {
				t.Fatal("row cap silently loses count")
			}
			if name == "oversized_first" && (len(rows) != 1 || rows[0].AssertionID != original.AssertionID || !strings.Contains(got, "shown=1 total=2 omitted=1")) {
				t.Fatal("oversized identity blocked later short row or was partially emitted")
			}
			if name == "byte_limit" && len(rows) == 8 {
				t.Fatal("fixture must exercise byte budget, not only row budget")
			}
			if !bytes.Equal(before, b1788SnapshotJSON(t, r)) {
				t.Fatal("render mutated source identities")
			}
		})
	}
}

func TestB1788NativeTestIdentitySnapshotDoesNotBindPTO(t *testing.T) {
	p, r := b1575PairConflictFixture("active")
	r.Channel = ChangeReportChannelPostApplyVerify
	p.ProjectTestObservations[0].AssertionID = "not-the-executed-assertion"
	r.VerificationConfidence = nil
	before := b1788SnapshotJSON(t, []any{p, r})
	profile := BuildVerificationProofProfile(p, r)
	if profile.Status != VerificationProofWeak {
		t.Fatalf("fixture must start with unbound native pass: %+v", profile)
	}
	if got := RenderCurrentNativeTestIdentitySnapshot(p.ID, r); !strings.Contains(got, `"assertion_id":"test_value"`) {
		t.Fatal("executed native identity missing")
	}
	if !bytes.Equal(before, b1788SnapshotJSON(t, []any{p, r})) || BuildVerificationProofProfile(p, r).Status != VerificationProofWeak {
		t.Fatal("identity disclosure patched declaration or promoted proof")
	}
}

func b1788IdentityReport() *ChangeReport {
	return &ChangeReport{PlanID: "active", Channel: ChangeReportChannelPostApplyVerify, Passed: true,
		GeneratedAt: time.Date(2026, 9, 21, 4, 0, 0, 123, time.UTC),
		TestResults: []TestResult{{Kind: TestResultKindUnit, ObservationScope: TestObservationScopeAssertion, Suite: "project.Widget", AssertionID: "test_value", Passed: true}},
	}
}

func b1788SnapshotJSON(t *testing.T, value any) []byte {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
