package tracediag

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestRenderStepBodyWindowStatsKeyFirstSurvivesSmallLineCap(t *testing.T) {
	step := &Step{View: "window_stats", effMaxLines: 7}
	res := &tracequery.Result{
		View: "window_stats",
		Caveats: []string{
			"typed completeness caveat must precede ordinary detail",
		},
		Compactions: []tracequery.ViewCompaction{{
			View: "window_stats", Dimension: "rows", Total: 20, Emitted: 8,
			LastEmittedTs: 5.007, LastEmittedLine: 700,
		}},
		WindowStats: &tracequery.WindowStats{
			Window: tracequery.TimeWindow{StartTs: 5, EndTs: 5.007},
			SchedulerHeadCoverage: &tracequery.SchedulerHeadCoverage{
				Status: "unknown", BoundaryTs: 5, Reason: "missing pre-window checkpoint",
				MissingCPUCount: 1, MissingCPUs: []int{7},
				MissingThreadCount: 1, MissingThreadPIDs: []int{100},
			},
			CounterQuality: &tracequery.TraceCounterQualitySummary{
				Rows: 12, ValidIdentityRows: 10, NumericRows: 9, InvalidRows: 2,
				NonNumericRows: 1, DerivedInvalidSeries: 1, TotalSeries: 4,
				TotalSeriesStatus: "exact", PublishedSeries: 2, SuppressedSeries: 1,
				TruncatedSeries: 1, SeriesBudget: 3, SeriesBudgetExceeded: true,
				OverflowRows: 2, BaselinePolicy: "first_numeric", UnitPolicy: "verbatim",
			},
			StorageLatencyByLayer: []tracequery.StorageLatencySummary{{
				Layer: "block", Count: 6, PairedCount: 2, UnpairedStartCount: 1,
				UnpairedDoneCount: 1, AmbiguousCohortCount: 1, PairingSuppressedCount: 2,
			}},
			WorkqueueActivity: []tracequery.WorkqueueActivity{{
				Count: 3, PairedCount: 1, UnpairedStartCount: 1,
				AmbiguousCohortCount: 1, PairingSuppressedCount: 1,
			}},
			CPU: []tracequery.CPUStats{{
				CPU: 7, BusyMs: 6.5,
				FrequencyResidency: []tracequery.CPUFrequencyResidency{
					{Frequency: 400000, DurationMs: 1},
					{Frequency: 800000, DurationMs: 2},
					{Frequency: 1600000, DurationMs: 3},
				},
			}},
		},
	}

	body := renderStepBody(step, stepOutcome{result: res})
	report := strings.Join(body.lines, "\n")
	for _, want := range []string{
		"engine_diagnostics(引擎原文去重账目): caveats=1 compactions=1",
		"key_first.completeness window_stats: status=unknown",
		"key_first.counter_quality window_stats: rows=12",
		"key_first.pairing window_stats: storage={groups=1 paired=2 unpaired_start=1 unpaired_done=1 ambiguous=1 suppressed=2}",
		"engine_caveat source=result",
		"引擎截断记录: source=result view=window_stats",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("small-cap report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "frequency_residency") || strings.Contains(report, "1600000") {
		t.Fatalf("bulk CPU residency displaced key-first fields:\n%s", report)
	}
	if len(body.lines) != step.EffectiveMaxLines() || body.total <= len(body.lines) {
		t.Fatalf("cap accounting lines=%d total=%d cap=%d", len(body.lines), body.total, step.EffectiveMaxLines())
	}
}

func TestRenderStepBodyRootCapabilityAndSplitKeyFirst(t *testing.T) {
	step := &Step{View: "root_cause_rank", effMaxLines: 4}
	res := &tracequery.Result{
		View:    "root_cause_rank",
		Caveats: []string{"root caveat stays ahead of ordinary rank detail"},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Items: []tracequery.RootCauseRankItem{{
				Rank: 1, Type: "priority_inversion_candidate", Summary: "ordinary rank detail must be capped",
				GatedCapabilitySource: "freq_only", GatedClusterTopology: "keyed_rail",
				SupplyFoldBasis: &tracequery.SupplyFoldBasis{
					KnownMs: 1.2, UnknownMs: 0.1, CapabilitySource: "freq_only",
					ClusterTopologySource: "keyed_rail",
					CapabilitySplitAudit:  "cpu2↔cpu3 @5.001000 判定臂=mid_stream_divergence",
				},
			}},
		},
	}

	body := renderStepBody(step, stepOutcome{result: res})
	report := strings.Join(body.lines, "\n")
	for _, want := range []string{
		"key_first.capability: carriers=2",
		`capability_source=shown=1/1["freq_only"]`,
		`cluster_topology=shown=1/1["keyed_rail"]`,
		"cpu2↔cpu3 @5.001000 判定臂=mid_stream_divergence",
		"engine_caveat source=result",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("root key-first report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "ordinary rank detail must be capped") {
		t.Fatalf("ordinary rank detail displaced capability audit:\n%s", report)
	}
	if body.total <= len(body.lines) {
		t.Fatalf("fixture did not exercise truncation: lines=%d total=%d", len(body.lines), body.total)
	}
}

func TestRenderV2DiscoveryDecisionSurfacePrecedesCandidateRoster(t *testing.T) {
	spec := &WindowDiscovery{effMaxLines: 5}
	endpoint := &tracequery.WindowDiscoveryEndpointProvenance{Action: "B", SourcePath: "/customer/private/trace.systrace", Line: 10, Ts: 1, EmitterPID: 20, Name: "VerifyClass"}
	result := &tracequery.WindowDiscoveryResult{
		Complete: true, IdentityComplete: true, ParseComplete: true, ScannedLineCount: 100,
		EndpointCount: 20, ScopedEndpointCount: 2, RetainedCandidateCount: 20,
		SelectionBasis: "soft semantic selection; no causal claim",
		Families:       []tracequery.WindowDiscoveryFamilyStats{{Family: tracequery.WindowDiscoveryFamilyTraceSync, EndpointCount: 20, StartCount: 10, DoneCount: 10, CompletedPairCount: 10}},
		Windows: []tracequery.DiscoveredWindow{{
			Ordinal: 1, CandidateRank: 20, CandidateWindow: 1, Family: tracequery.WindowDiscoveryFamilyTraceSync, Kind: "exact_pair",
			StartTs: .995, EndTs: 1.005, CoreStartTs: 1, CoreEndTs: 1.001, CoreLineStart: 10, CoreLineEnd: 11,
			PairingStatus: tracequery.WindowDiscoveryPairingCompleteExact, CarryClass: tracequery.WindowDiscoveryInsidePair,
			SemanticClass: "class_verification", StartEndpoint: endpoint, EndEndpoint: &tracequery.WindowDiscoveryEndpointProvenance{Action: "E", SourcePath: endpoint.SourcePath, Line: 11, Ts: 1.001, EmitterPID: 20},
		}},
		Caveats: []string{"typed quality caveat must remain visible"},
	}
	for i := 0; i < 20; i++ {
		result.Candidates = append(result.Candidates, tracequery.WindowDiscoveryCandidate{Rank: i + 1, Family: tracequery.WindowDiscoveryFamilyTraceSync, Kind: "pairing_issue"})
	}
	body := renderV2DiscoveryBody(spec, result)
	report := strings.Join(body.lines, "\n")
	for _, want := range []string{"generated_window ordinal=1", "semantic_class=class_verification", "typed quality caveat must remain visible"} {
		if !strings.Contains(report, want) {
			t.Fatalf("small-cap discovery report lost decision surface %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "candidate rank=") {
		t.Fatalf("candidate detail displaced the discovery decision surface:\n%s", report)
	}
	if strings.Contains(report, "/customer/private/") {
		t.Fatalf("endpoint rendering leaked an absolute source path:\n%s", report)
	}
	if len(body.lines) != spec.EffectiveMaxLines() || body.total <= len(body.lines) {
		t.Fatalf("discovery cap accounting lines=%d total=%d cap=%d", len(body.lines), body.total, spec.EffectiveMaxLines())
	}
}

func TestNonEventKeyFirstTypedFieldsAreNotRepeatedByGenericWalker(t *testing.T) {
	step := &Step{View: "window_stats", effMaxLines: 100}
	res := &tracequery.Result{
		View: "window_stats",
		WindowStats: &tracequery.WindowStats{
			CounterQuality: &tracequery.TraceCounterQualitySummary{
				Rows: 1, TotalSeriesStatus: "exact", BaselinePolicy: "first", UnitPolicy: "raw",
			},
			StorageLatencyByLayer: []tracequery.StorageLatencySummary{{
				Layer: "block", Count: 2, PairedCount: 1, UnpairedDoneCount: 1,
			}},
			PerfSamples: &tracequery.PerfContext{Quality: &tracequery.PerfQualitySummary{
				InputIntegrityIssues: []tracequery.PerfValueCount{{Value: "cpu_duplicate_conflict", SampleCount: 2, Period: 2}},
				ParserCaveats:        []tracequery.PerfValueCount{{Value: "lost_records=1", SampleCount: 2, Period: 2}},
			}},
		},
	}

	body := renderStepBody(step, stepOutcome{result: res})
	report := strings.Join(body.lines, "\n")
	if got := strings.Count(report, "key_first.counter_quality"); got != 1 {
		t.Fatalf("counter quality render count=%d:\n%s", got, report)
	}
	if strings.Contains(report, "paired_count=") || strings.Contains(report, "unpaired_done_count=") {
		t.Fatalf("generic detail repeated key-first pairing fields:\n%s", report)
	}
	if got := strings.Count(report, "cpu_duplicate_conflict"); got != 1 || !strings.Contains(report, "input_integrity_issues=1") {
		t.Fatalf("perf input integrity was hidden or repeated: count=%d\n%s", got, report)
	}
	if got := strings.Count(report, "lost_records=1"); got != 1 || !strings.Contains(report, "parser_caveats=1") {
		t.Fatalf("raw parser caveat was hidden or repeated: count=%d\n%s", got, report)
	}
}

func TestNonEventPrioritySchemaPins(t *testing.T) {
	types := make([]reflectTypeName, 0, len(nonEventPrioritySchemaPins))
	for typ := range nonEventPrioritySchemaPins {
		types = append(types, reflectTypeName{name: typ.PkgPath() + "." + typ.Name(), typ: typ})
	}
	sort.Slice(types, func(i, j int) bool { return types[i].name < types[j].name })
	for _, item := range types {
		got, schema := detailSchemaFingerprint(item.typ)
		want := nonEventPrioritySchemaPins[item.typ]
		if got != want {
			t.Errorf("%s schema drift: got=%s want=%s\ncurrent_schema=%s", item.name, got, want, schema)
		}
	}
}

type reflectTypeName struct {
	name string
	typ  reflect.Type
}
