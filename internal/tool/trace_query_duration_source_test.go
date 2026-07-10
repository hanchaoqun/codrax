package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQuerySummaryPublishesDurationSourceBasenames(t *testing.T) {
	const source = "/private/capture/source-a.systrace"
	result := tracequery.Result{
		View:       "span_window",
		SourcePath: "/private/capture/bundle.tracebundle.json",
		RootCauseRank: &tracequery.RootCauseRankResult{Items: []tracequery.RootCauseRankItem{{
			Rank: 1, Tier: "primary", Type: "semantic_compile", Thread: tracequery.ThreadRef{Comm: "app", PID: 10},
			Source: "trace_span", PhysicalSourcePath: source, ImpactMs: 1, CumulativeImpactMs: 1, EffectiveImpactMs: 1,
		}}},
		SpanWindows: []tracequery.TraceSpanSummary{{
			SourcePath: source, Thread: tracequery.ThreadRef{Comm: "app", PID: 10}, Name: "frame", Kind: "sync",
			StartTs: 1, EndTs: 1.001, DurationMs: 1, StartLine: 1, EndLine: 2,
		}},
		WindowStats: &tracequery.WindowStats{
			IOLatencies: []tracequery.IOLatencySummary{{
				SourcePath: source, Dev: "8,0", Op: "R", Sector: 1, Len: 8, DurationMs: 1, IssueLine: 3, CompleteLine: 4,
			}},
			TraceSpans: []tracequery.TraceSpanSummary{{
				SourcePath: source, Thread: tracequery.ThreadRef{Comm: "app", PID: 10}, Name: "jit", Kind: "sync", DurationMs: 1, StartLine: 5, EndLine: 6,
			}},
			StorageLatencyByLayer: []tracequery.StorageLatencySummary{{
				SourcePath: source, Layer: "block", Event: "block_rq", Dev: "8,0", PairedCount: 1, MaxLatencyMs: 1, LineStart: 7, LineEnd: 8,
			}},
			IRQActivity: []tracequery.InterruptActivity{{
				SourcePath: source, Kind: "irq", CPU: 0, Vector: 17, Name: "timer", PairedCount: 1, ActiveMs: 1, LineStart: 9, LineEnd: 10,
			}},
			SoftIRQActivity: []tracequery.InterruptActivity{{
				SourcePath: source, Kind: "softirq", CPU: 1, Vector: 3, Name: "NET_RX", PairedCount: 1, ActiveMs: 1, LineStart: 11, LineEnd: 12,
			}},
			IPIActivity: []tracequery.InterruptActivity{{
				SourcePath: source, Kind: "ipi", CPU: 2, Name: "reschedule", PairedCount: 1, ActiveMs: 1, LineStart: 13, LineEnd: 14,
			}},
			WorkqueueActivity: []tracequery.WorkqueueActivity{{
				SourcePath: source, Thread: tracequery.ThreadRef{Comm: "worker", PID: 20}, Work: "0xff", Function: "flush", PairedCount: 1, DurationMs: 1, LineStart: 15, LineEnd: 16,
			}},
			DMAFenceActivity: []tracequery.DMAFenceActivity{{
				SourcePath: source, Thread: tracequery.ThreadRef{Comm: "display", PID: 30}, Driver: "display", Timeline: "present", Seqno: "9", PairedCount: 1, WaitMs: 1, LineStart: 17, LineEnd: 18,
			}},
		},
	}

	got := traceQuerySummary(result, traceQueryParams{}, "runtime_artifact:test", "")
	for _, prefix := range []string{
		"- rank=",
		"- span ",
		"- io_latency ",
		"- trace_span ",
		"- storage_latency ",
		"- irq_activity ",
		"- softirq_activity ",
		"- ipi_activity ",
		"- workqueue_activity ",
		"- dma_fence_activity ",
	} {
		line := traceQuerySummaryLineWithPrefix(got, prefix)
		sourceField := "source=source-a.systrace"
		if prefix == "- rank=" {
			sourceField = "physical_source=source-a.systrace"
		}
		if !strings.Contains(line, sourceField) {
			t.Fatalf("%s row omitted source basename: %q\nsummary:\n%s", prefix, line, got)
		}
		if strings.Contains(line, source) {
			t.Fatalf("%s row leaked full source path instead of basename: %q", prefix, line)
		}
	}
}

func traceQuerySummaryLineWithPrefix(summary, prefix string) string {
	for _, line := range strings.Split(summary, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}
