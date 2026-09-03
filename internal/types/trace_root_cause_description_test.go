package types

import (
	"strings"
	"testing"
)

// trace_root_cause_description_test.go — SIDECAR-NARR-1 (customer feedback
// 2026-09-03): the model's plain-language description rides beside the typed
// evidence, bounded and free of internal references.
func TestValidateTraceRootCauseDescription(t *testing.T) {
	got, err := ValidateTraceRootCauseDescription("  同进程 GC 线程 HeapTaskDaemon   执行并发标记约 12 ms，\nUIThread 在此期间等待堆锁。 ", []string{"candidate-gc"}, "root_causes[0]")
	if err != nil || got != "同进程 GC 线程 HeapTaskDaemon 执行并发标记约 12 ms， UIThread 在此期间等待堆锁。" {
		t.Fatalf("description must be compacted: %q %v", got, err)
	}
	if got, err := ValidateTraceRootCauseDescription("   ", []string{"c"}, "f"); err != nil || got != "" {
		t.Fatalf("blank description is simply absent: %q %v", got, err)
	}
	if _, err := ValidateTraceRootCauseDescription(strings.Repeat("长", TraceRootCauseDescriptionMaxRunes+1), []string{"c"}, "f"); err == nil {
		t.Fatal("over-long description must be rejected")
	}
	// The closed internal-reference grammar (复核): blob/output paths and
	// stamps, trace_query result AND raw-offload names, tmp dirs, ranking
	// anchors, attached copies, evidence badges, receipt ids.
	for _, leak := range []string{
		"见 .codrax/blob/x/attached_trace.txt",
		"trace_query:trace-query-result-1e05.json#root_cause_rank:1",
		"如 attached_trace.txt 第 2892 行",
		"详见 trace_query-9f3a2b1c.txt 第 40 行",
		"产物 20260903-013218.970-5266 中的第 3 席",
		"文件 /private/tmp/claude-501/x/trace.txt",
		"证据 [E7(+2)] 与 E12 显示",
		"对应 observation-3f2 的记录",
		"候选 candidate-sched 耗时最长",
	} {
		if leak := TraceRootCauseDescriptionInternalReference(leak, []string{"candidate-other"}); leak == "" {
			t.Fatalf("internal reference must be detected: %q", leak)
		}
	}
	// Any roster id — not only the item's own — is refused.
	if _, err := ValidateTraceRootCauseDescription("与 cand-x 相比更长", []string{"cand-gc", "cand-x"}, "f"); err == nil {
		t.Fatal("quoting another roster candidate id must be refused")
	}
	if leak := TraceRootCauseDescriptionInternalReference("RenderThread 等待 CPU 调度约 12 ms，主线程随之被推迟 3 帧", nil); leak != "" {
		t.Fatalf("plain prose must pass: %q", leak)
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
