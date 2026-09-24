package tracequery

import (
	"strings"
	"testing"
)

func TestSchedulerConcurrencyCapabilityFullPopulationContract(t *testing.T) {
	catalog, err := TraceCapabilities("window_stats", true)
	if err != nil {
		t.Fatal(err)
	}
	var metric *MetricCapability
	for i := range catalog.Metrics {
		if catalog.Metrics[i].ID == "scheduler_concurrency" {
			metric = &catalog.Metrics[i]
		}
	}
	if metric == nil {
		t.Fatal("scheduler metric missing")
	}
	outputs := map[string]string{}
	for _, o := range metric.Outputs {
		outputs[o.Section] += " " + strings.Join(o.Fields, " ") + " " + o.Caliber
	}
	for path, parts := range map[string][]string{
		"window_stats.scheduler_concurrency.groups.members":             {"source_path", "start_local_line", "actual_start_ts", "overlapping same-TID"},
		"window_stats.scheduler_concurrency.groups.distribution.depths": {"duration_ms", "window_share", "not a distribution of bucket peaks"},
		"window_stats.scheduler_concurrency.groups.distribution":        {"p50_threads", "p95_threads", "p99_threads", "before"},
		"window_stats.scheduler_concurrency.groups.buckets":             {"independent", "empty buckets", "short final"},
	} {
		for _, part := range parts {
			if !strings.Contains(outputs[path], part) {
				t.Errorf("%s contract missing %q", path, part)
			}
		}
	}
	if SchedulerConcurrencyMemberLimit != 16 || SchedulerConcurrencyDepthLimit != 32 || SchedulerConcurrencyBucketLimit != 32 {
		t.Fatal("review independent display budgets before changing")
	}
}
