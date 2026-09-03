package types

import (
	"strings"
	"testing"
)

// trace_root_cause_description_test.go — SIDECAR-NARR-1 (customer feedback
// 2026-09-03): the model's plain-language description rides beside the typed
// evidence, bounded and free of internal references.
func TestValidateTraceRootCauseDescription(t *testing.T) {
	got, err := ValidateTraceRootCauseDescription("  同进程 GC 线程 HeapTaskDaemon   执行并发标记约 12 ms，\nUIThread 在此期间等待堆锁。 ", "candidate-gc", "root_causes[0]")
	if err != nil || got != "同进程 GC 线程 HeapTaskDaemon 执行并发标记约 12 ms， UIThread 在此期间等待堆锁。" {
		t.Fatalf("description must be compacted: %q %v", got, err)
	}
	if got, err := ValidateTraceRootCauseDescription("   ", "c", "f"); err != nil || got != "" {
		t.Fatalf("blank description is simply absent: %q %v", got, err)
	}
	if _, err := ValidateTraceRootCauseDescription(strings.Repeat("长", TraceRootCauseDescriptionMaxRunes+1), "c", "f"); err == nil {
		t.Fatal("over-long description must be rejected")
	}
	for _, leak := range []string{"见 .codrax/blob/x/attached_trace.txt", "trace_query:trace-query-result-1e05.json#root_cause_rank:1", "如 attached_trace.txt 第 2892 行"} {
		if _, err := ValidateTraceRootCauseDescription(leak, "c", "f"); err == nil {
			t.Fatalf("internal reference must be refused: %q", leak)
		}
	}
	if _, err := ValidateTraceRootCauseDescription("候选 candidate-gc 耗时最长", "candidate-gc", "f"); err == nil {
		t.Fatal("quoting the internal candidate id must be refused")
	}
}

func TestNormalizeTraceRootCauseReportKeepsDescriptionBesideTypedEvidence(t *testing.T) {
	impact := 0.0124
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{{
			CandidateID: "candidate-gc", Category: TraceRootCauseGCLongPause, ThreadName: "HeapTaskDaemon",
			ImpactSeconds: &impact, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven,
			Description: "同进程 GC 线程 HeapTaskDaemon 执行并发标记约 12 ms，UIThread 在此期间等待堆锁。",
			Evidence:    []string{"HeapTaskDaemon 在目标窗口内的链上有效归因为 12.400 ms"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := report.RootCauses[0]
	if item.Description == "" || item.Summary != "GC耗时长" || len(item.Evidence) != 1 {
		t.Fatalf("description must ride beside the fixed summary and typed evidence: %+v", item)
	}
}
