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
				Rank: 99, Category: TraceRootCauseCPUSchedulingDelay, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ThreadName: " RenderThread ",
				ImpactSeconds: traceImpact(0.0124), Summary: "model wording is ignored",
				Evidence: []string{"  runnable 12.4 ms，期间没有获得 CPU  "},
			},
			{
				Category: TraceRootCauseLockContention, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ResourceName: "ClassLinker classes lock",
				ImpactSeconds: traceImpact(0.0081),
				Evidence:      []string{"Worker 等待该锁 8.1 ms", "owner tid 42 在窗口内持续运行"},
			},
			{
				Category: TraceRootCauseSynchronousBinder, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ThreadName: "UIThread",
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
			Category: TraceRootCauseIOBlocking, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ImpactSeconds: traceImpact(0.001),
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
					Category: TraceRootCauseGCLongPause, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ImpactSeconds: test.impact,
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
			{Category: TraceRootCauseGCLongPause, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ImpactSeconds: traceImpact(0.02), Evidence: []string{"first"}},
			{Category: TraceRootCauseCPUSchedulingDelay, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ThreadName: "RenderThread", ImpactSeconds: traceImpact(0.01), Evidence: []string{"second"}},
			{Category: TraceRootCauseGCLongPause, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ImpactSeconds: traceImpact(0.005), Evidence: []string{"duplicate"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicates root_causes[0]") {
		t.Fatalf("duplicate dynamic cause was not rejected: %v", err)
	}
}

func TestNormalizeTraceRootCauseReportLegacyIdentityDoesNotUseSummaryText(t *testing.T) {
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses: []*TraceRootCauseItemV2{
			{Category: TraceRootCauseComputeSupplyShortage, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ThreadName: "ui", ImpactSeconds: traceImpact(.002), Evidence: []string{"E1"}},
			{Category: TraceRootCauseComputeSupplyShortage, ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven, ThreadName: "worker", ImpactSeconds: traceImpact(.001), Evidence: []string{"E2"}},
		},
	})
	if err != nil || len(report.RootCauses) != 2 || report.RootCauses[0].Summary != report.RootCauses[1].Summary {
		t.Fatalf("typed identities may share their fixed display summary: report=%+v err=%v", report, err)
	}
}

// V1-4 (§40.26 ③): the partition key survives the candidate-id strip — two
// items identical except for their trace-file label are two causes on a
// legacy re-normalization; identical labels stay duplicates.
func TestNormalizeTraceRootCauseReportLegacyIdentityKeepsArtifactPartition(t *testing.T) {
	item := func(label string) *TraceRootCauseItemV2 {
		return &TraceRootCauseItemV2{Category: TraceRootCauseCPUSchedulingDelay, ThreadName: "RenderThread", ArtifactLabel: label,
			ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven,
			ImpactSeconds: traceImpact(0.004), Evidence: []string{"E"}}
	}
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*TraceRootCauseItemV2{item("a.systrace"), item("b.systrace")}})
	if err != nil || len(report.RootCauses) != 2 || report.RootCauses[0].ArtifactLabel != "a.systrace" || report.RootCauses[1].ArtifactLabel != "b.systrace" {
		t.Fatalf("same-named seats from two trace files must both survive with their labels: %+v err=%v", report, err)
	}
	if _, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*TraceRootCauseItemV2{item("a.systrace"), item("a.systrace")}}); err == nil || !strings.Contains(err.Error(), "duplicates root_causes[0]") {
		t.Fatalf("same label must still be a duplicate: %v", err)
	}
}

// SIDECAR-Q1 (§40.28 ②): both public qualifiers are closed-set and REQUIRED
// on every bound item; a frame-unproven seat carries the same words as the
// Markdown headline qualifier on its summary.
func TestNormalizeTraceRootCauseReportQualifiersAreExplicitAndClosedSet(t *testing.T) {
	base := func() *TraceRootCauseItemV2 {
		return &TraceRootCauseItemV2{Category: TraceRootCauseCPUSchedulingDelay, ThreadName: "RenderThread",
			ImpactSeconds: traceImpact(0.0085), Evidence: []string{"E1"},
			ImpactCaliber: TraceImpactCaliberEffectiveAttribution, CausalQualifier: TraceCausalQualifierProven}
	}
	for name, mutate := range map[string]func(*TraceRootCauseItemV2){
		"missing caliber":   func(i *TraceRootCauseItemV2) { i.ImpactCaliber = "" },
		"unknown caliber":   func(i *TraceRootCauseItemV2) { i.ImpactCaliber = "raw" },
		"missing qualifier": func(i *TraceRootCauseItemV2) { i.CausalQualifier = "" },
		"unknown qualifier": func(i *TraceRootCauseItemV2) { i.CausalQualifier = "maybe" },
	} {
		item := base()
		mutate(item)
		if _, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: []*TraceRootCauseItemV2{item}}); err == nil {
			t.Fatalf("%s must be rejected (qualifiers are never inferred from absence)", name)
		}
	}
	proven := base()
	unproven := base()
	unproven.ThreadName = "GLThread"
	unproven.CausalQualifier = TraceCausalQualifierFrameUnproven
	unproven.ImpactCaliber = TraceImpactCaliberWindowProjection
	report, err := NormalizeAndValidateTraceRootCauseReport(&TraceRootCauseReportV2{SchemaVersion: 2, RootCauses: []*TraceRootCauseItemV2{proven, unproven}})
	if err != nil {
		t.Fatal(err)
	}
	if report.RootCauses[0].Summary != "RenderThread线程CPU调度延迟" || report.RootCauses[0].CausalQualifier != TraceCausalQualifierProven {
		t.Fatalf("proven item drifted: %+v", report.RootCauses[0])
	}
	if report.RootCauses[1].Summary != "GLThread线程CPU调度延迟（帧因果未证）" ||
		report.RootCauses[1].CausalQualifier != TraceCausalQualifierFrameUnproven ||
		report.RootCauses[1].ImpactCaliber != TraceImpactCaliberWindowProjection {
		t.Fatalf("frame-unproven item must wear the headline qualifier words and its caliber: %+v", report.RootCauses[1])
	}
}
