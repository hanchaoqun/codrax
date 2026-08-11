package types

import (
	"reflect"
	"testing"
)

func TestNormalizeTraceRootCauseReportKeepsFreeEvidenceAndFixesSummaries(t *testing.T) {
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV1{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCause1: &TraceRootCauseItemV1{
			Category: TraceRootCauseCPUSchedulingDelay, ThreadName: " RenderThread ",
			Summary:  "model wording is ignored",
			Evidence: []string{"  runnable 12.4 ms，期间没有获得 CPU  "},
		},
		RootCause2: &TraceRootCauseItemV1{
			Category: TraceRootCauseLockContention, ResourceName: "ClassLinker classes lock",
			Evidence: []string{"Worker 等待该锁 8.1 ms", "owner tid 42 在窗口内持续运行"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.RootCause1.Summary != "RenderThread线程CPU调度延迟" {
		t.Fatalf("unexpected root_cause_1 summary: %q", report.RootCause1.Summary)
	}
	if report.RootCause2.Summary != "ClassLinker classes lock锁竞争" {
		t.Fatalf("unexpected root_cause_2 summary: %q", report.RootCause2.Summary)
	}
	if !reflect.DeepEqual(report.RootCause1.Evidence, []string{"runnable 12.4 ms，期间没有获得 CPU"}) {
		t.Fatalf("free evidence was not compacted losslessly: %#v", report.RootCause1.Evidence)
	}
}

func TestNormalizeTraceRootCauseReportAllowsExplicitNullSecondCause(t *testing.T) {
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV1{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCause1: &TraceRootCauseItemV1{
			Category: TraceRootCauseGCLongPause,
			Evidence: []string{"GC pause 覆盖目标卡顿窗口，共 18.6 ms"},
		},
		RootCause2: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.RootCause2 != nil {
		t.Fatalf("unsupported second cause must stay null: %#v", report.RootCause2)
	}
}

func TestNormalizeTraceRootCauseReportRejectsMissingCauseIdentity(t *testing.T) {
	_, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV1{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCause1: &TraceRootCauseItemV1{
			Category: TraceRootCauseIOBlocking,
			Evidence: []string{"observed wait"},
		},
	})
	if err == nil {
		t.Fatal("thread-scoped cause without thread_name must fail")
	}
}
