package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryPriorityPressurePublishesRawSystemKernelSeparately(t *testing.T) {
	target := tracequery.ThreadRef{Comm: "app", PID: 100}
	rt := tracequery.ThreadRef{Comm: "rt159", PID: 159, TGID: 10}
	raw := tracequery.ThreadRef{Comm: "sys160", PID: 160, TGID: 10}
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 5, EndTs: 5.102},
		CPUPressure: []tracequery.CPUPressureStats{{
			CPU: 0, RunnableWaitMs: 102, RunningMs: 102,
			HighPriorityRunningMs:          12,
			HighPriorityRunningOverlapMs:   12,
			SystemOrKernelRunningMs:        90,
			SystemOrKernelRunningOverlapMs: 90,
			SystemOrKernelCompetitorCount:  2,
		}},
		ThreadCPULoad: []tracequery.ThreadCPULoadSummary{{
			Thread: rt, RunningMs: 12, HighPriorityRunningMs: 12,
			CPU: 0, Priority: 159, PriorityClass: "ohos_rt",
		}, {
			Thread: raw, RunningMs: 90, SystemOrKernelRunningMs: 90,
			CPU: 0, Priority: 160, PriorityClass: "system_or_kernel",
		}},
		RunnableContext: []tracequery.RunnableContextSummary{{
			Thread: target, RunnableWaitMs: 102, CPU: 0,
			Priority: 159, PriorityClass: "ohos_rt",
			HighPriorityRunningMs:          12,
			HighPriorityRunningOverlapMs:   12,
			SystemOrKernelRunningMs:        90,
			SystemOrKernelRunningOverlapMs: 90,
			SystemOrKernelCompetitorCount:  2,
			Verdict:                        "cpu_pressure", Summary: "typed pressure split",
		}},
		ProcessCPULoad: []tracequery.ProcessCPULoadSummary{{
			Process: tracequery.ThreadRef{Comm: "system", PID: 10}, ThreadCount: 2, RunningMs: 102,
			HighPriorityRunningMs: 12, SystemOrKernelRunningMs: 90,
			TopThread: raw, TopThreadMs: 90, Summary: "typed process split",
		}},
		SupplyPressureSummary: &tracequery.SupplyPressureSummary{
			Signal: "cpu_pressure", CPUPressureMs: 114, RunnableWaitMs: 102,
			HighPriorityRunningMs:          12,
			SystemOrKernelRunningMs:        90,
			SystemOrKernelRunningOverlapMs: 90,
			SystemOrKernelCompetitorCount:  2,
			Summary:                        "typed supply split",
		},
	}
	result := tracequery.Result{
		View: "window_stats", SourcePath: "berlin.systrace",
		TraceFlavor: string(tracequery.TraceFlavorHarmonyHitrace),
		WindowStats: &stats,
	}
	summary := traceQuerySummary(result, traceQueryParams{}, "berlin.systrace", "")
	for _, want := range []string{
		"priority_rule=harmony_larger_numeric_higher_1_40_CFS_41_159_RT_gt159_raw",
		"high_prio_running=12.000ms",
		"high_prio_overlap=12.000ms",
		"system_or_kernel_running=90.000ms",
		"system_or_kernel_overlap=90.000ms",
		"system_or_kernel_competitors=2",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("rendered trace query omitted %q:\n%s", want, summary)
		}
	}

	records := traceQueryTypedWindowStatsObservations(
		stats,
		types.ObservationSourceRef{ArtifactID: "runtime_artifact:berlin"},
		"berlin",
		"2026-07-10T00:00:00Z",
	)
	wantByPredicate := map[string][]string{
		"thread_cpu_load":  {"high_prio_running=12.000", "system_or_kernel_running=90.000"},
		"runnable_context": {"high_prio_overlap=12.000", "system_or_kernel_overlap=90.000", "system_or_kernel_competitors=2"},
		"process_cpu_load": {"high_prio_running=12.000", "system_or_kernel_running=90.000"},
		"cpu_pressure":     {"high_prio=12.000", "system_or_kernel_running=90.000", "system_or_kernel_overlap=90.000", "system_or_kernel_competitors=2"},
	}
	seen := map[string]bool{}
	notesByPredicate := map[string][]string{}
	for _, record := range records {
		if _, ok := wantByPredicate[record.Predicate]; !ok {
			continue
		}
		seen[record.Predicate] = true
		notesByPredicate[record.Predicate] = append(notesByPredicate[record.Predicate], record.RichNotes...)
	}
	for predicate, wants := range wantByPredicate {
		notes := strings.Join(notesByPredicate[predicate], "\n")
		for _, want := range wants {
			if !strings.Contains(notes, want) {
				t.Fatalf("%s observation omitted typed note %q: %s", predicate, want, notes)
			}
		}
	}
	for predicate := range wantByPredicate {
		if !seen[predicate] {
			t.Fatalf("missing typed %s observation: %+v", predicate, records)
		}
	}
}
