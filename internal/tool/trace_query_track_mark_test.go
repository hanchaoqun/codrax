package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryPublishesTrackAndInstantInventoryWithoutThreadOwnership(t *testing.T) {
	tool := &TraceQuery{}
	if description := tool.Description(); !strings.Contains(description, "G/H ASYNC_FOR_TRACK") || !strings.Contains(description, "trace_track_spans") || !strings.Contains(description, "zero-duration trace_instants") {
		t.Fatalf("tool description does not teach the isolated extended-marker contract: %s", description)
	}
	if schema := string(tool.Parameters()); !strings.Contains(schema, "B/E/C/S/F/G/H/N/I") || !strings.Contains(schema, "trace_track_spans") {
		t.Fatalf("tool schema does not advertise extended trace markers: %s", schema)
	}
	result := tracequery.Result{
		View:       "window_stats",
		SourcePath: "/trace/synthetic.systrace",
		TimeStart:  1,
		TimeEnd:    2,
		WindowStats: &tracequery.WindowStats{
			TraceTrackSpans: []tracequery.TraceTrackSpanSummary{{
				SourcePath: "/trace/synthetic.systrace", OwnerPID: 321,
				TrackName: "render-track", Name: "phase", Cookie: "42",
				BeginEmitter: tracequery.ThreadRef{Comm: "begin-writer", PID: 10},
				EndEmitter:   tracequery.ThreadRef{Comm: "end-writer", PID: 11},
				StartTs:      1.1, EndTs: 1.2, DurationMs: 100, StartLine: 7, EndLine: 9,
			}},
			TraceInstants: []tracequery.TraceInstantSummary{{
				SourcePath: "/trace/synthetic.systrace", Action: "N", OwnerPID: 321,
				TrackName: "render-track", Name: "checkpoint",
				Emitter: tracequery.ThreadRef{Comm: "point-writer", PID: 12},
				Payload: "N|321|render-track|checkpoint", Ts: 1.15, Line: 8,
			}},
		},
	}
	summary := traceQuerySummary(result, traceQueryParams{}, "attached_trace", "runtime_artifact:test")
	for _, want := range []string{
		"trace_track_span owner_pid=321 track=\"render-track\" name=\"phase\" cookie=42",
		"begin_emitter=begin-writer-10 end_emitter=end-writer-11",
		"trace_instant action=N owner_pid=321 track=\"render-track\" name=\"checkpoint\"",
		"payload=\"N|321|render-track|checkpoint\"",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
	if strings.Contains(summary, "trace_span begin-writer-10") {
		t.Fatalf("logical track span was rendered as emitter-owned trace_span:\n%s", summary)
	}
}
