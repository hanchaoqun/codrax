package tool

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBusinessSpanSchedulerExplorerSummaryPublic(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("business summary")}
	for _, view := range []string{"wakeup_chain", "root_cause_rank", "window_stats"} {
		t.Run(view, func(t *testing.T) {
			result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": view, "pid": 100, "time_start": 1, "time_end": 1.051})
			payload := businessSpanSchedulerPublicPayload(t, result)
			before, _ := json.Marshal(payload)
			for _, want := range []string{
				`marker_state_account "OpenDocument" owner=app-main-100 interval=1.000000..1.050000 running=5.000ms runnable=1.000ms sleep=44.000ms`,
				`marker_state_account "LoadDocumentIndex" owner=document-worker-200 interval=1.004500..1.044500 running=8.000ms runnable=1.000ms sleep=31.000ms`,
				"marker-local states, not wider-query totals", "states do not prove a wait mechanism",
			} {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("public explorer summary lost %q", want)
				}
			}
			if payload.WindowStats.Window.StartTs != 1 || payload.WindowStats.Window.EndTs != 1.051 {
				t.Fatal("query window was replaced")
			}
			after, _ := json.Marshal(payload)
			if string(before) != string(after) {
				t.Fatal("summary mutated native evidence")
			}
		})
	}
}

func TestBusinessSpanSchedulerSummaryKeepsTypedOwnershipAndUnknown(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("summary boundaries")}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "time_start": 1, "time_end": 1.051})
	payload := businessSpanSchedulerPublicPayload(t, result)
	var original tracequery.TraceSpanSummary
	for _, span := range payload.WindowStats.TraceSpans {
		if span.Name == "OpenDocument" {
			original = span
		}
	}
	if original.SchedulerStates == nil {
		t.Fatal("no native account")
	}
	encoded, _ := json.Marshal(original)
	fresh := func() tracequery.TraceSpanSummary {
		var span tracequery.TraceSpanSummary
		if err := json.Unmarshal(encoded, &span); err != nil {
			t.Fatal(err)
		}
		return span
	}
	for name, mutate := range map[string]func(*tracequery.TraceSpanSummary){
		"async":          func(s *tracequery.TraceSpanSummary) { s.Kind = "async" },
		"missing":        func(s *tracequery.TraceSpanSummary) { s.SchedulerStates = nil },
		"foreign source": func(s *tracequery.TraceSpanSummary) { s.SourcePath += ".other" },
		"foreign owner":  func(s *tracequery.TraceSpanSummary) { s.Thread.PID++ },
		"foreign window": func(s *tracequery.TraceSpanSummary) { s.StartTs += .001 },
		"invalid totals": func(s *tracequery.TraceSpanSummary) { s.SchedulerStates.RunningMs += 1 },
		"nonfinite":      func(s *tracequery.TraceSpanSummary) { s.SchedulerStates.RunningMs = math.NaN() },
	} {
		t.Run(name, func(t *testing.T) {
			span := fresh()
			mutate(&span)
			if got := traceQueryBusinessSpanSchedulerSummary(span); got != "" {
				t.Fatalf("unbound account published: %s", got)
			}
		})
	}
	for _, coverage := range []string{"complete", "partial", "unavailable"} {
		t.Run(coverage, func(t *testing.T) {
			span := fresh()
			span.Name = "OtherBusiness"
			span.SchedulerStates.Coverage = coverage
			if coverage == "partial" {
				span.SchedulerStates.RunningMs--
				span.SchedulerStates.AccountedMs--
			}
			if coverage == "unavailable" {
				span.SchedulerStates.RunningMs, span.SchedulerStates.RunnableMs, span.SchedulerStates.SleepMs, span.SchedulerStates.AccountedMs = 0, 0, 0, 0
				span.SchedulerStates.MeasurementDomain = nil
			}
			before, _ := json.Marshal(span)
			got := traceQueryBusinessSpanSchedulerSummary(span)
			if !strings.Contains(got, `marker_state_account "OtherBusiness" owner=app-main-100 interval=1.000000..1.050000`) {
				t.Fatalf("missing generic owner/window: %s", got)
			}
			if coverage == "unavailable" {
				if !strings.Contains(got, "unavailable, not zero") || strings.Contains(got, "running=0") {
					t.Fatal(got)
				}
			} else if !strings.Contains(got, coverage+" coverage") || !strings.Contains(got, "is included in sleep, not an addend") {
				t.Fatal(got)
			}
			after, _ := json.Marshal(span)
			if string(before) != string(after) {
				t.Fatal("summary mutated evidence")
			}
		})
	}
	var preview strings.Builder
	spans := make([]tracequery.TraceSpanSummary, traceQueryWidthStateDrilldownSummaryCap()+2)
	for i := range spans {
		spans[i] = fresh()
	}
	writeTraceBusinessSpanSchedulerPreview(&preview, spans, "full-evidence.json")
	if strings.Count(preview.String(), "- marker_state_account ") != traceQueryWidthStateDrilldownSummaryCap() ||
		!strings.Contains(preview.String(), "remaining accounts are in payload_ref=full-evidence.json (not an all-trace inventory)") {
		t.Fatal("bounded preview lost capacity disclosure or continuation")
	}
}
