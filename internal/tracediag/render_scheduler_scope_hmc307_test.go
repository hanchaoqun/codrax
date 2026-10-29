package tracediag

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const schedulerContextScopeAddition = "ContextScope|*tracequery.SchedulerContextScope|context_scope,omitempty"

func stripSchedulerContextScopeAddition(schema string) (string, error) {
	var fields []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == schedulerContextScopeAddition {
			count++
			continue
		}
		fields = append(fields, field)
	}
	if count != 1 {
		return "", fmt.Errorf("expected exactly one reviewed scheduler scope, got %d", count)
	}
	return strings.Join(fields, ";"), nil
}

func schedulerSchemaBeforeContextScope(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.SchedulerLatencyItem{}) && typ != reflect.TypeOf(tracequery.RunnableContextSummary{}) {
		return schema
	}
	previous, err := stripSchedulerContextScopeAddition(schema)
	if err != nil {
		t.Fatal(err)
	}
	return previous
}

func TestHMC307SchedulerScopeExactEvolutionAndDisposition(t *testing.T) {
	// This formerly unpinned leaf is reviewed in full; scheduler items also
	// pass their unchanged historical MeasurementSources fingerprint below.
	const previousRunnable = "Thread|tracequery.ThreadRef|thread;RunnableWaitMs|float64|runnable_wait_ms,omitempty;CPU|int|cpu;CoreClass|string|core_class,omitempty;Frequency|int64|frequency,omitempty;Priority|int|priority,omitempty;PriorityClass|string|priority_class,omitempty;SameCPUBusyMs|float64|same_cpu_busy_ms,omitempty;SameCPUIdleMs|float64|same_cpu_idle_ms,omitempty;OtherCPUIdleMs|float64|other_cpu_idle_ms,omitempty;HighPriorityRunningMs|float64|high_priority_running_ms,omitempty;HighPriorityRunningOverlapMs|float64|high_priority_running_overlap_ms,omitempty;SystemOrKernelRunningMs|float64|system_or_kernel_running_ms,omitempty;SystemOrKernelRunningOverlapMs|float64|system_or_kernel_running_overlap_ms,omitempty;SystemOrKernelCompetitorCount|int|system_or_kernel_competitor_count,omitempty;SameCPUTopRunning|[]tracequery.ThreadDuration|same_cpu_top_running,omitempty;TopBackgroundThreads|[]tracequery.ThreadCPULoadSummary|top_background_threads,omitempty;SameProcessLoad|*tracequery.ProcessCPULoadSummary|same_process_load,omitempty;TopBackgroundProcess|*tracequery.ProcessCPULoadSummary|top_background_process,omitempty;CPUConstraint|*tracequery.CPUConstraintSummary|cpu_constraint,omitempty;Verdict|string|verdict,omitempty;Confidence|float64|confidence,omitempty;LineStart|int|line_start,omitempty;LineEnd|int|line_end,omitempty;Summary|string|summary,omitempty"
	_, runnable := detailSchemaFingerprint(reflect.TypeOf(tracequery.RunnableContextSummary{}))
	if schedulerSchemaBeforeContextScope(t, reflect.TypeOf(tracequery.RunnableContextSummary{}), runnable) != previousRunnable || runnable != schedulerContextScopeAddition+";"+previousRunnable {
		t.Fatalf("non-scope runnable field changed: %s", runnable)
	}
	const wantScope = "QueryWindow|tracequery.TimeWindow|query_window;QueryWindowKnown|bool|query_window_known;QueryLineStart|int|query_line_start,omitempty;QueryLineEnd|int|query_line_end,omitempty;WaitInterval|tracequery.TimeWindow|wait_interval;WaitIntervalBoundary|string|wait_interval_boundary;QueryWindowFields|[]string|query_window_fields;WaitIntervalFields|[]string|wait_interval_fields;OtherCPUIdleUnit|string|other_cpu_idle_unit;CPUBusyIdleStatus|string|cpu_busy_idle_status;CPUBusyIdleReason|string|cpu_busy_idle_reason,omitempty"
	_, scope := detailSchemaFingerprint(reflect.TypeOf(tracequery.SchedulerContextScope{}))
	if scope != wantScope {
		t.Fatalf("unreviewed scheduler scope DTO: %s", scope)
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(tracequery.SchedulerContextScope{}), reflect.TypeOf(tracequery.SchedulerLatencyItem{}), reflect.TypeOf(tracequery.RunnableContextSummary{})} {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.PkgPath == "" && (typ.Name() == "SchedulerContextScope" || f.Name == "ContextScope") && policySkipsDetailField(&nonEventDetailPolicy, typ, f.Name) {
				t.Fatalf("scope detail hidden: %s.%s", typ, f.Name)
			}
		}
	}
	for _, value := range []any{tracequery.SchedulerLatencyItem{}, tracequery.RunnableContextSummary{}} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "context_scope") {
			t.Fatalf("legacy absence invented scope: %s", raw)
		}
	}
}

func TestHMC307SchedulerScopeInverseFailsClosed(t *testing.T) {
	for _, bad := range []string{
		"Thread|tracequery.ThreadRef|thread",
		schedulerContextScopeAddition + ";" + schedulerContextScopeAddition,
		strings.Replace(schedulerContextScopeAddition, "*tracequery.SchedulerContextScope", "string", 1),
		strings.Replace(schedulerContextScopeAddition, "context_scope,omitempty", "context_scope", 1),
	} {
		if _, err := stripSchedulerContextScopeAddition(bad); err == nil {
			t.Fatalf("missing/duplicate/type/tag drift admitted: %s", bad)
		}
	}
	future := schedulerContextScopeAddition + ";Future|string|future"
	if got, err := stripSchedulerContextScopeAddition(future); err != nil || got != "Future|string|future" {
		t.Fatal("future changes must remain visible to the old full fingerprint")
	}
}

func TestHMC307SchedulerScopeNativeRendering(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"scheduler_latency_stats", "window_stats"} {
		res := tracequery.Run(idx, tracequery.Query{View: view, PID: 100, TimeStart: .999, TimeEnd: 1.051, MinDurationMs: .05, Limit: 32})
		before, _ := json.Marshal(res)
		var lines []string
		renderResultDetailWithPolicy(&res, func(line string) { lines = append(lines, line) }, &nonEventDetailPolicy)
		out := strings.Join(lines, "\n")
		for _, want := range []string{"query_window=[0.999000..1.051000]", "wait_interval=[1.045000..1.046000]", "wait_interval_boundary=start_inclusive_end_exclusive", "query_window_fields", "other_cpu_idle_ms", "other_cpu_idle_unit=cpu_ms", "query_window_known=true"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s lost %q: %s", view, want, out)
			}
		}
		after, _ := json.Marshal(res)
		if string(before) != string(after) {
			t.Fatal("display changed native facts")
		}
	}
}
