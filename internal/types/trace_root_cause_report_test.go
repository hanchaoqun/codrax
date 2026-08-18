package types

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func traceImpact(seconds float64) *float64 { return &seconds }

func TestNormalizeTraceRootCauseReportKeepsDynamicCausesAndFixesRuntimeFields(t *testing.T) {
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{
			{
				Rank: 99, Category: TraceRootCauseCPUSchedulingDelay, ThreadName: " RenderThread ",
				ImpactSeconds: traceImpact(0.0124), Summary: "model wording is ignored",
				Evidence: []string{"  runnable 12.4 ms，期间没有获得 CPU  "},
			},
			{
				Category: TraceRootCauseLockContention, ResourceName: "ClassLinker classes lock",
				ImpactSeconds: traceImpact(0.0081),
				Evidence:      []string{"Worker 等待该锁 8.1 ms", "owner tid 42 在窗口内持续运行"},
			},
			{
				Category: TraceRootCauseSynchronousBinder, ThreadName: "UIThread",
				ImpactSeconds: traceImpact(0.003),
				Evidence:      []string{"同步 Binder 等待 3 ms"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RootCauses) != 3 {
		t.Fatalf("dynamic cause count=%d, want 3", len(report.RootCauses))
	}
	wantSummaries := []string{
		"RenderThread线程CPU调度延迟",
		"ClassLinker classes lock锁竞争",
		"UIThread线程同步binder",
	}
	for index, cause := range report.RootCauses {
		if cause.Rank != index+1 {
			t.Fatalf("root_causes[%d].rank=%d, want %d", index, cause.Rank, index+1)
		}
		if cause.Summary != wantSummaries[index] {
			t.Fatalf("root_causes[%d].summary=%q, want %q", index, cause.Summary, wantSummaries[index])
		}
		if cause.ImpactSeconds == nil || *cause.ImpactSeconds <= 0 {
			t.Fatalf("root_causes[%d].impact_seconds=%v", index, cause.ImpactSeconds)
		}
	}
	if !reflect.DeepEqual(report.RootCauses[0].Evidence, []string{"runnable 12.4 ms，期间没有获得 CPU"}) {
		t.Fatalf("free evidence was not compacted losslessly: %#v", report.RootCauses[0].Evidence)
	}
}

func TestNormalizeTraceRootCauseReportAllowsEmptyEvidenceSizedList(t *testing.T) {
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses:    []*TraceRootCauseItemV2{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.RootCauses == nil || len(report.RootCauses) != 0 {
		t.Fatalf("empty supported-cause list was not preserved: %#v", report.RootCauses)
	}
}

func TestNormalizeTraceRootCauseReportRejectsMissingCauseIdentity(t *testing.T) {
	_, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{{
			Category: TraceRootCauseIOBlocking, ImpactSeconds: traceImpact(0.001),
			Evidence: []string{"observed wait"},
		}},
	})
	if err == nil {
		t.Fatal("thread-scoped cause without thread_name must fail")
	}
}

func TestNormalizeTraceRootCauseReportRejectsInvalidImpactSeconds(t *testing.T) {
	for _, test := range []struct {
		name   string
		impact *float64
	}{
		{name: "missing", impact: nil},
		{name: "zero", impact: traceImpact(0)},
		{name: "negative", impact: traceImpact(-0.1)},
		{name: "nan", impact: traceImpact(math.NaN())},
		{name: "infinite", impact: traceImpact(math.Inf(1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
				SchemaVersion: TraceRootCauseReportSchemaVersion,
				RootCauses: []*TraceRootCauseItemV2{{
					Category: TraceRootCauseGCLongPause, ImpactSeconds: test.impact,
					Evidence: []string{"GC 覆盖目标窗口"},
				}},
			})
			if err == nil || !strings.Contains(err.Error(), "impact_seconds") {
				t.Fatalf("invalid impact accepted or wrong error: %v", err)
			}
		})
	}
}

func TestNormalizeTraceRootCauseReportRejectsDuplicateCauseAnywhereInList(t *testing.T) {
	_, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{
			{Category: TraceRootCauseGCLongPause, ImpactSeconds: traceImpact(0.02), Evidence: []string{"first"}},
			{Category: TraceRootCauseCPUSchedulingDelay, ThreadName: "RenderThread", ImpactSeconds: traceImpact(0.01), Evidence: []string{"second"}},
			{Category: TraceRootCauseGCLongPause, ImpactSeconds: traceImpact(0.005), Evidence: []string{"duplicate"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicates root_causes[0]") {
		t.Fatalf("duplicate dynamic cause was not rejected: %v", err)
	}
}
