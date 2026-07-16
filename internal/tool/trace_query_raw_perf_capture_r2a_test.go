package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const r2aToolRawPerfCaptureCaveat = "tracebundle_raw_perf_capture_completeness authority=manifest_advisory capture_hard_gate=false positive_evidence=preserve absence_policy=require_quality_caveat census_scope=observed_perf_record_stream device_capture_completeness=not_claimed valid=true applicability=raw_perf_artifact_and_receipt artifact=capture.perftrace query_ready=false capture_state=inventory_only capture_quality_issue=true effective_clock_evidence=none sample_aggregation=none clock_alignment=none thread_attribution=none root_cause_rank=none census_participation=capture_quality_only sample_records=physical:0,accepted:0,rejected:0 lost_records=physical:1,accepted:1,rejected:0 lost_sample_records=physical:0,accepted:0,rejected:0 aux_records=physical:0,accepted:0,rejected:0 lost_events=exact:7 lost_samples=not_reported aux_bytes=not_reported"

func TestTraceQuerySummaryHoistsRawPerfCaptureOnceAndRemovesTailTwin(t *testing.T) {
	result := tracequery.Result{
		View: "event_search", SourcePath: "/tmp/capture.tracebundle.json",
		Caveats: []string{
			"ordinary caveat remains in the ordinary roster",
			r2aToolRawPerfCaptureCaveat,
			r2aToolRawPerfCaptureCaveat,
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "event_search"}, "path", "/tmp/payload.json")
	if got := strings.Count(summary, r2aToolRawPerfCaptureCaveat); got != 1 {
		t.Fatalf("raw capture disclosure count=%d, want one:\n%s", got, summary)
	}
	head := "raw_perf_capture_completeness=" + r2aToolRawPerfCaptureCaveat
	if !strings.Contains(summary, head) || strings.Contains(summary, "caveat="+r2aToolRawPerfCaptureCaveat) {
		t.Fatalf("raw capture was not exclusively hoisted:\n%s", summary)
	}
	if rawAt, ordinaryAt := strings.Index(summary, head), strings.Index(summary, "caveat=ordinary caveat"); rawAt < 0 || ordinaryAt < 0 || rawAt >= ordinaryAt {
		t.Fatalf("raw capture did not precede ordinary caveats: raw=%d ordinary=%d\n%s", rawAt, ordinaryAt, summary)
	}
	for _, want := range []string{
		"query_ready=false", "effective_clock_evidence=none", "sample_aggregation=none",
		"clock_alignment=none", "thread_attribution=none", "root_cause_rank=none",
		"lost_events=exact:7", "lost_samples=not_reported", "census_scope=observed_perf_record_stream",
		"device_capture_completeness=not_claimed",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("head disclosure lost %q:\n%s", want, summary)
		}
	}
}

func TestTraceQuerySummaryHoistsEveryDistinctRawPerfCaptureEntry(t *testing.T) {
	second := strings.ReplaceAll(r2aToolRawPerfCaptureCaveat, "artifact=capture.perftrace", "artifact=second.perftrace")
	result := tracequery.Result{
		View: "event_search", SourcePath: "/tmp/capture.tracebundle.json",
		Caveats: []string{r2aToolRawPerfCaptureCaveat, second, r2aToolRawPerfCaptureCaveat},
	}
	summary := traceQuerySummary(result, traceQueryParams{View: "event_search"}, "path", "/tmp/payload.json")
	if got := strings.Count(summary, "raw_perf_capture_completeness="); got != 2 ||
		strings.Count(summary, "artifact=capture.perftrace") != 1 ||
		strings.Count(summary, "artifact=second.perftrace") != 1 {
		t.Fatalf("distinct raw perf artifacts were omitted or duplicated: head_count=%d\n%s", got, summary)
	}
	if strings.Contains(summary, "caveat="+tracequery.RawPerfCaptureCompletenessCaveatToken) {
		t.Fatalf("raw perf entries re-entered the ordinary caveat tail:\n%s", summary)
	}
}

func TestTraceQueryDescriptionTeachesRawPerfInventoryBoundary(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		"tracebundle_raw_perf_capture_completeness", "exact:0", "not_reported", "unknown(reason)",
		"query_ready=false", "observed_perf_record_stream", "never proves device-side capture completeness",
		"never use that inventory for CPU aggregation, clock alignment, thread attribution, or root-cause ranking",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("trace_query description missing raw inventory contract %q", want)
		}
	}
}
