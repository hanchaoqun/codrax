package tracediag

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const r2aTraceDiagRawPerfCaveat = "tracebundle_raw_perf_capture_completeness authority=manifest_advisory capture_hard_gate=false positive_evidence=preserve absence_policy=require_quality_caveat census_scope=observed_perf_record_stream device_capture_completeness=not_claimed valid=true applicability=raw_perf_artifact_and_receipt artifact=capture.perftrace query_ready=false capture_state=inventory_only capture_quality_issue=true effective_clock_evidence=none sample_aggregation=none clock_alignment=none thread_attribution=none root_cause_rank=none census_participation=capture_quality_only sample_records=physical:0,accepted:0,rejected:0 lost_records=physical:1,accepted:1,rejected:0 lost_sample_records=physical:2,accepted:2,rejected:0 aux_records=physical:0,accepted:0,rejected:0 lost_events=exact:7 lost_samples=unknown:aggregate_overflow aux_bytes=not_reported"

func TestRawPerfCaptureKeyFirstIsCompactWithoutLosingTotalsOrBoundary(t *testing.T) {
	line := rawPerfCaptureKeyFirstLine(r2aTraceDiagRawPerfCaveat, 1, 1)
	if len(line) > maxRenderedTokenBytes || strings.Contains(line, "截断") {
		t.Fatalf("raw perf key-first line exceeded its bounded no-truncation contract: bytes=%d\n%s", len(line), line)
	}
	for _, want := range []string{
		"entry=1/1", "state=inventory_only", "ready=false", "issue=true", "clock=none",
		"scope=record_stream_only", "device=not_claimed",
		"rec=a/r", "s=0/0", "l=1/0|events:exact:7",
		"ls=2/0|samples:unknown:aggregate_overflow",
		"x=0/0|bytes:not_reported", "auth=manifest_advisory",
		"gate=false", "absence=must_qualify",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("compact raw perf line lost %q:\n%s", want, line)
		}
	}
	if totalsAt, authorityAt := strings.Index(line, "l=1/0"), strings.Index(line, "auth="); totalsAt < 0 || authorityAt < 0 || totalsAt >= authorityAt {
		t.Fatalf("loss evidence was not prioritized before fixed authority tail: %s", line)
	}
}

func TestRawPerfCaptureKeyFirstWorstCaseUint64StillDoesNotTruncate(t *testing.T) {
	const max = "18446744073709551615"
	maxRecord := "physical:" + max + ",accepted:" + max + ",rejected:" + max
	caveat := strings.NewReplacer(
		"physical:0,accepted:0,rejected:0", maxRecord,
		"physical:1,accepted:1,rejected:0", maxRecord,
		"physical:2,accepted:2,rejected:0", maxRecord,
		"exact:7", "exact:"+max,
		"unknown:aggregate_overflow", "exact:"+max,
		"not_reported", "exact:"+max,
	).Replace(r2aTraceDiagRawPerfCaveat)
	line := rawPerfCaptureKeyFirstLine(caveat, 256, 256)
	if len(line) > maxRenderedTokenBytes || strings.Contains(line, "截断") ||
		!strings.Contains(line, "x="+max+"/"+max+"|bytes:exact:"+max) ||
		!strings.Contains(line, "scope=record_stream_only device=not_claimed") ||
		!strings.Contains(line, "auth=manifest_advisory gate=false absence=must_qualify") {
		t.Fatalf("uint64-max key-first line lost its final evidence/authority tail: bytes=%d\n%s", len(line), line)
	}
}

func TestNonEventRawPerfCaptureGetsFirstPostMetaSeatAndNoEngineTwin(t *testing.T) {
	step := &Step{View: "window_stats", effMaxLines: 2}
	res := &tracequery.Result{
		View: "window_stats",
		Caveats: []string{
			"ordinary caveat",
			r2aTraceDiagRawPerfCaveat,
			r2aTraceDiagRawPerfCaveat,
		},
	}
	body := renderStepBody(step, stepOutcome{result: res})
	report := strings.Join(body.lines, "\n")
	if got := strings.Count(report, "key_first.perf_capture"); got != 1 || !strings.Contains(report, "l=1/0|events:exact:7") {
		t.Fatalf("minimum non-event budget lost or repeated raw quality boundary: count=%d\n%s", got, report)
	}
	if strings.Contains(report, "engine_caveat") && strings.Contains(report, tracequery.RawPerfCaptureCompletenessCaveatToken) {
		t.Fatalf("raw advisory re-entered the ordinary engine roster:\n%s", report)
	}
}

func TestWindowedNonEventMaxTwoFoldsRawPerfBoundaryIntoMeta(t *testing.T) {
	step := &Step{View: "window_stats", effMaxLines: 2}
	res := &tracequery.Result{
		View: "window_stats", TimeStart: 1, TimeEnd: 2,
		Caveats: []string{r2aTraceDiagRawPerfCaveat, r2aTraceDiagRawPerfCaveat, "ordinary caveat"},
	}
	body := renderStepBody(step, stepOutcome{result: res})
	report := strings.Join(body.lines, "\n")
	if len(body.lines) != 2 || !strings.Contains(report, "perf_capture={entry=1/1") ||
		!strings.Contains(report, "lost_events=exact:7") || !strings.Contains(report, "scope=observed_perf_record_stream") {
		t.Fatalf("window metadata displaced the global raw perf boundary: lines=%d\n%s", len(body.lines), report)
	}
	if strings.Contains(report, "key_first.perf_capture") || strings.Contains(report, "engine_caveat") {
		t.Fatalf("windowed meta fallback duplicated the bounded raw perf advisory:\n%s", report)
	}
}

func TestEventSearchMaxThreeFoldsRawPerfBoundaryIntoHeader(t *testing.T) {
	if _, err := ParseScript([]byte("version: 1\nsteps: [{label: raw, view: event_search, max_lines: 3}]\n")); err != nil {
		t.Fatalf("max_lines=3 static compatibility regressed: %v", err)
	}
	step := &Step{View: "event_search", effMaxLines: 3}
	res := &tracequery.Result{
		View: "event_search", TimeStart: 1, TimeEnd: 2,
		Caveats: []string{r2aTraceDiagRawPerfCaveat, r2aTraceDiagRawPerfCaveat},
		Events: []tracequery.EventView{{
			Event: tracequery.Event{Line: 1, Ts: 1.1, Type: tracequery.EventSchedWakeup}, Raw: "raw witness",
		}},
	}
	body := renderEventSearchBody(step, res)
	report := strings.Join(body.lines, "\n")
	for _, want := range []string{
		"perf_capture={entry=1/1", "state=inventory_only", "ready=false", "issue=true",
		"lost_events=exact:7", "lost_samples=unknown:aggregate_overflow",
		"scope=observed_perf_record_stream", "device_complete=not_claimed",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("max_lines=3 header fallback lost %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "key_first.perf_capture") || strings.Contains(report, "引擎 caveat 原文: "+tracequery.RawPerfCaptureCompletenessCaveatToken) {
		t.Fatalf("header fallback duplicated the raw advisory:\n%s", report)
	}
}

func TestGeneratedEventSearchMaxFiveKeepsEndpointFloorAndRawBoundary(t *testing.T) {
	second := strings.ReplaceAll(r2aTraceDiagRawPerfCaveat, "artifact=capture.perftrace", "artifact=second.perftrace")
	step := &Step{View: "event_search", effMaxLines: 5, windowOrigin: &WindowProvenance{DiscoveryLabel: "d"}}
	res := &tracequery.Result{
		View: "event_search", TimeStart: 1, TimeEnd: 2,
		Caveats: []string{r2aTraceDiagRawPerfCaveat, second},
		Events: []tracequery.EventView{
			{Event: tracequery.Event{Line: 1, Ts: 1.1, Type: tracequery.EventBlockIssue}, Raw: "start endpoint"},
			{Event: tracequery.Event{Line: 2, Ts: 1.2, Type: tracequery.EventBlockComplete}, Raw: "done endpoint"},
		},
	}
	body := renderEventSearchBody(step, res)
	report := strings.Join(body.lines, "\n")
	if got := strings.Count(report, "type=block_rq_"); got != 2 {
		t.Fatalf("global quality disclosure displaced generated endpoint floor: got=%d\n%s", got, report)
	}
	if !strings.Contains(report, "perf_capture={entry=1/2") || !strings.Contains(report, "lost_events=exact:7") {
		t.Fatalf("bounded multi-artifact header did not disclose entry count and quality: %s", report)
	}
	if strings.Contains(report, "key_first.perf_capture") || strings.Contains(report, "引擎 caveat 原文: "+tracequery.RawPerfCaptureCompletenessCaveatToken) {
		t.Fatalf("generated header fallback duplicated raw quality facts:\n%s", report)
	}
}
