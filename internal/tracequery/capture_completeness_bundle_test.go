package tracequery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTraceBundleCaptureCompletenessRoundTripPreservesPositiveEvents(t *testing.T) {
	dir := t.TempDir()
	systrace := filepath.Join(dir, "capture.systrace")
	bundle := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(systrace, []byte(`app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=001
`), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{
  "version":"test",
  "systrace":"capture.systrace",
  "artifacts":[{"type":"systrace","path":"capture.systrace"}],
  "trace_db_coverage":[{
    "family":"capture_completeness",
    "table":"stat",
    "role":"capture_completeness",
    "found":true,
    "rows_read":5,
    "capture_completeness":{
      "state":"parser_self_audit_degraded",
      "rows_accepted":5,
      "received":1,
      "data_lost":2,
      "error_issues":2,
      "nonzero_issue_rows":1,
      "issues":[{"event_name":"sched:wakeup,source=forged","stat_type":"data_lost","count":2,"source":"trace","severity":"error"}]
    }
  }]
}`
	if err := os.WriteFile(bundle, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), bundle)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != EventSchedWakeup {
		t.Fatalf("capture loss side-channel removed positive event: %+v", idx.Events)
	}
	caveats := strings.Join(idx.Caveats, "\n")
	for _, want := range []string{
		"family=capture_completeness table=stat role=capture_completeness",
		"capture_state=parser_self_audit_degraded",
		"capture_scope=trace_streamer_parser_self_audit",
		"capture_trust=manifest_advisory",
		"capture_hard_gate=false",
		"capture_absence_policy=require_quality_caveat",
		"capture_positive_evidence=preserve",
		"capture_received_proves_source_complete=false",
		"capture_loss_scope=global_absence_quality",
		"capture_not_match_scope=context_dependent_absence_quality",
		"capture_other_scope=global_absence_quality",
		"capture_data_lost=2",
		"sched%3Awakeup%2Csource%3Dforged:data_lost:2:error:trace",
	} {
		if !strings.Contains(caveats, want) {
			t.Fatalf("capture caveat missing %q:\n%s", want, caveats)
		}
	}
	result := Run(idx, Query{View: "event_search", EventTypes: []EventType{EventSchedWakeup}, Limit: 10})
	if len(result.Events) != 1 || !strings.Contains(strings.Join(result.Caveats, "\n"), "capture_state=parser_self_audit_degraded") {
		t.Fatalf("result did not preserve positive event plus capture caveat: %+v", result)
	}
}

func TestTraceBundleCaptureCompletenessRejectsSpoofedPayloads(t *testing.T) {
	validUnknown := func() *traceBundleCaptureCompleteness {
		return &traceBundleCaptureCompleteness{State: "unknown", IntegrityIssues: []string{"missing_table"}}
	}
	tests := []struct {
		name string
		rows []traceBundleCoverage
		want string
	}{
		{
			name: "nested payload on non authority",
			rows: []traceBundleCoverage{{
				Family: "scheduler", Table: "sched_slice", Role: "query_ready_export",
				CaptureCompleteness: &traceBundleCaptureCompleteness{State: "parser_self_audit_clean", RowsAccepted: 5},
			}},
			want: "",
		},
		{
			name: "missing nested payload",
			rows: []traceBundleCoverage{{Family: "capture_completeness", Table: "stat", Role: "capture_completeness"}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "clean contradicts loss",
			rows: []traceBundleCoverage{{
				Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 5,
				CaptureCompleteness: &traceBundleCaptureCompleteness{
					State: "parser_self_audit_clean", RowsAccepted: 5, DataLost: 1, ErrorIssues: 1,
				},
			}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "negative structural count",
			rows: []traceBundleCoverage{{
				Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 5,
				CaptureCompleteness: &traceBundleCaptureCompleteness{State: "parser_self_audit_clean", RowsAccepted: -1},
			}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "unknown integrity token is not an authority",
			rows: []traceBundleCoverage{{
				Family: "capture_completeness", Table: "stat", Role: "capture_completeness",
				CaptureCompleteness: &traceBundleCaptureCompleteness{State: "unknown", IntegrityIssues: []string{"trust_me"}},
			}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "non canonical top32 compaction",
			rows: []traceBundleCoverage{{
				Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 45,
				CaptureCompleteness: &traceBundleCaptureCompleteness{
					State: "parser_self_audit_degraded", RowsAccepted: 45, DataLost: 33, WarnIssues: 33,
					NonzeroIssueRows: 33, IssuesCompacted: 32,
					Issues: []traceBundleCaptureCompletenessIssue{{
						EventName: "event_00", StatType: "data_lost", Count: 1, Source: "trace", Severity: "warn",
					}},
				},
			}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "duplicate event stat issue examples",
			rows: []traceBundleCoverage{{
				Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 5,
				CaptureCompleteness: &traceBundleCaptureCompleteness{
					State: "parser_self_audit_degraded", RowsAccepted: 5, DataLost: 3,
					WarnIssues: 1, FatalIssues: 2, NonzeroIssueRows: 2,
					Issues: []traceBundleCaptureCompletenessIssue{
						{EventName: "event", StatType: "data_lost", Count: 2, Source: "trace", Severity: "fatal"},
						{EventName: "event", StatType: "data_lost", Count: 1, Source: "trace", Severity: "warn"},
					},
				},
			}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "issue events exceed cohort count",
			rows: []traceBundleCoverage{{
				Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 5,
				CaptureCompleteness: &traceBundleCaptureCompleteness{
					State: "parser_self_audit_degraded", RowsAccepted: 5, DataLost: 1, NotMatch: 1,
					WarnIssues: 2, NonzeroIssueRows: 2,
					Issues: []traceBundleCaptureCompletenessIssue{
						{EventName: "event_a", StatType: "data_lost", Count: 1, Source: "trace", Severity: "warn"},
						{EventName: "event_b", StatType: "not_match", Count: 1, Source: "trace", Severity: "warn"},
					},
				},
			}},
			want: "invalid_bundle_capture_payload",
		},
		{
			name: "duplicate authority",
			rows: []traceBundleCoverage{
				{Family: "capture_completeness", Table: "stat", Role: "capture_completeness", CaptureCompleteness: validUnknown()},
				{Family: "capture_completeness", Table: "stat", Role: "capture_completeness", CaptureCompleteness: validUnknown()},
			},
			want: "duplicate_capture_authority",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			caveats := traceBundleCoverageCaveats("tracebundle_trace_db_coverage", test.rows)
			joined := strings.Join(caveats, "\n")
			if test.want == "" {
				if strings.Contains(joined, "capture_state=") {
					t.Fatalf("non-authority nested payload was rendered:\n%s", joined)
				}
				return
			}
			if !strings.Contains(joined, "capture_state=unknown") || !strings.Contains(joined, test.want) {
				t.Fatalf("spoof did not fail closed with %q:\n%s", test.want, joined)
			}
			if strings.Count(joined, "family=capture_completeness table=stat role=capture_completeness") != 1 {
				t.Fatalf("capture authority must be singular after normalization:\n%s", joined)
			}
		})
	}
}

func TestTraceBundleCaptureCompletenessCanonicalizesAuthorityEnvelope(t *testing.T) {
	coverage := traceBundleCoverage{
		Family: "capture_completeness", Table: "stat", Role: "capture_completeness",
		FieldSources: map[string]string{
			"capture_state": "parser_self_audit_clean capture_hard_gate=true " + strings.Repeat("x", 64*1024),
		},
		ColumnsPresent:    []string{"capture_state=parser_self_audit_clean", strings.Repeat("y", 64*1024)},
		PeakBufferedBytes: ^uint64(0), TempBytes: 1<<63 - 1, ElapsedUS: 1<<63 - 1,
		CaptureCompleteness: &traceBundleCaptureCompleteness{State: "unknown", IntegrityIssues: []string{"missing_table"}},
	}
	caveats := traceBundleCoverageCaveats("tracebundle_trace_db_coverage", []traceBundleCoverage{coverage})
	if len(caveats) != 1 {
		t.Fatalf("canonical capture caveat count=%d: %+v", len(caveats), caveats)
	}
	caveat := caveats[0]
	if len(caveat) > 2048 || strings.Contains(caveat, "parser_self_audit_clean") ||
		strings.Contains(caveat, "capture_hard_gate=true") || strings.Contains(caveat, "field_sources") ||
		strings.Contains(caveat, "peak_buffered") || !strings.Contains(caveat, "capture_state=unknown") ||
		!strings.Contains(caveat, "capture_hard_gate=false") {
		t.Fatalf("capture authority envelope was not canonicalized:\n%s", caveat)
	}
}

func TestTraceBundleCaptureCompletenessAcceptsCanonicalTop32Compaction(t *testing.T) {
	issues := make([]traceBundleCaptureCompletenessIssue, 0, 32)
	for i := 0; i < 32; i++ {
		issues = append(issues, traceBundleCaptureCompletenessIssue{
			EventName: fmt.Sprintf("event_%02d", i), StatType: "data_lost", Count: 1, Source: "trace", Severity: "warn",
		})
	}
	coverage := traceBundleCoverage{
		Family: "capture_completeness", Table: "stat", Role: "capture_completeness", Found: true, RowsRead: 165,
		CaptureCompleteness: &traceBundleCaptureCompleteness{
			State: "parser_self_audit_degraded", RowsAccepted: 165, DataLost: 33, WarnIssues: 33,
			NonzeroIssueRows: 33, Issues: issues, IssuesCompacted: 1,
		},
	}
	caveat := strings.Join(traceBundleCoverageCaveats("tracebundle_trace_db_coverage", []traceBundleCoverage{coverage}), "\n")
	if !strings.Contains(caveat, "capture_state=parser_self_audit_degraded") ||
		strings.Contains(caveat, "invalid_bundle_capture_payload") || !strings.Contains(caveat, "capture_issues_compacted=1") {
		t.Fatalf("canonical top-32 payload was rejected:\n%s", caveat)
	}
}

func TestTraceBundleCaptureCompletenessPrioritySeatDoesNotDisplacePrefix(t *testing.T) {
	rows := make([]traceBundleCoverage, 0, traceBundleCoverageCaveatLimit+1)
	for i := 0; i < traceBundleCoverageCaveatLimit; i++ {
		rows = append(rows, traceBundleCoverage{Family: "regular", Table: "table", Found: true})
	}
	rows = append(rows, traceBundleCoverage{
		Family: "capture_completeness", Table: "stat", Role: "capture_completeness",
		CaptureCompleteness: &traceBundleCaptureCompleteness{State: "unknown", IntegrityIssues: []string{"missing_table"}},
	})
	caveats := traceBundleCoverageCaveats("tracebundle_trace_db_coverage", rows)
	if len(caveats) != traceBundleCoverageCaveatLimit+2 {
		t.Fatalf("priority seat bound mismatch: got=%d caveats=%+v", len(caveats), caveats)
	}
	if !strings.Contains(caveats[traceBundleCoverageCaveatLimit], "capture_state=unknown") ||
		!strings.Contains(caveats[len(caveats)-1], "priority_emitted=1") {
		t.Fatalf("capture priority seat missing: %+v", caveats)
	}
	traceCoverage := strings.Join(traceBundleCoverageCaveats("tracebundle_trace_coverage", rows), "\n")
	if strings.Contains(traceCoverage, "capture_state=") ||
		strings.Contains(traceCoverage, "family=capture_completeness") ||
		strings.Contains(traceCoverage, "role=capture_completeness") {
		t.Fatalf("trace_coverage impersonated trace_db capture authority:\n%s", traceCoverage)
	}
}
