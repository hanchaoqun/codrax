package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryCountOnlyIOCaliberIsConsistentAcrossFaces(t *testing.T) {
	result := countOnlyIOCaliberResult()
	summary := traceQuerySummary(result, traceQueryParams{View: "frame_root_cause_bundle"}, "customer.systrace", "")
	for _, want := range []string{
		"io_pressure io_pressure_signal=blocked_reason_iowait_count_only activity_index=180.000",
		"evidence_quality=activity_marker_only",
		"score_caliber=count_weighted_activity_index",
		"pressure_conclusion=pressure_unproven",
		"block_max=0.000ms storage_max=0.000ms",
		"iowait_blocked=36 d_state=0.000ms io_wait=0.000ms",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("raw summary missing %q:\n%s", want, summary)
		}
	}

	records := traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	var direct, root *types.ObservationRecord
	for i := range records {
		switch {
		case strings.Contains(records[i].ID, "#io_pressure:1"):
			direct = &records[i]
		case records[i].Predicate == "root_cause_context_only" && records[i].Object == "io_pressure":
			root = &records[i]
		}
	}
	if direct == nil || root == nil {
		t.Fatalf("direct/root IO records missing: direct=%+v root=%+v", direct, root)
	}
	for label, record := range map[string]*types.ObservationRecord{"direct": direct, "root": root} {
		notes := strings.Join(record.RichNotes, " ")
		for _, want := range []string{
			"io_pressure_signal=blocked_reason_iowait_count_only",
			"io_pressure_evidence_quality=activity_marker_only",
			"io_pressure_score_caliber=count_weighted_activity_index",
			"io_pressure_conclusion=pressure_unproven",
			"io_pressure_iowait_blocked_count=36",
			"io_pressure_block_max_ms=0.000",
			"io_pressure_storage_max_ms=0.000",
			"d_state=0.000",
			"io_wait=0.000",
		} {
			if !strings.Contains(notes, want) {
				t.Fatalf("%s typed record missing %q: %+v", label, want, *record)
			}
		}
	}
}

func TestSystemProjectionLabelsCountOnlyIOAsActivityNotPressure(t *testing.T) {
	records := traceQueryTypedObservations(countOnlyIOCaliberResult(), "customer.systrace", "payload", "raw", "", time.Unix(0, 0).UTC())
	var root []types.ObservationRecord
	for _, record := range records {
		if record.Predicate == "root_cause_context_only" && record.Object == "io_pressure" {
			root = append(root, record)
		}
	}
	root = append(root, types.ObservationRecord{
		ID:              "primary-control",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_primary",
		ClaimKey:        "root_cause_primary",
		Subject:         "worker-20",
		Object:          "runnable",
		Value:           "1.000",
		Unit:            "ms",
		RichNotes: []string{
			"tier=primary", "rank=1", "type=runnable", "impact_ms=1.000",
			"cumulative_impact_ms=1.000", "effective_impact_ms=1.000",
			"chain_relevance=on_chain", "causality=on_wakeup_chain",
			"selected_window=1.000000..2.000000",
		},
		Confidence: 0.8,
	})
	projection := types.TraceCausalProjectionFromObservationRecords(root)
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, want := range []string{
		"窗口IO活动标记(聚合)",
		"证据口径=activity_marker_only",
		"signal=blocked_reason_iowait_count_only",
		"iowait=36",
		"D态=0.000ms",
		"iowait=0.000ms",
		"块/存储最大延迟=0.000/0.000ms",
		"score_caliber=count_weighted_activity_index",
		"pressure_conclusion=pressure_unproven",
		"不证明高IO压力",
	} {
		if !strings.Contains(fence, want) {
			t.Fatalf("system projection missing %q:\n%s", want, fence)
		}
	}
	if strings.Contains(fence, "窗口IO压力(聚合)") {
		t.Fatalf("count-only markers regained the pressure label:\n%s", fence)
	}
}

func countOnlyIOCaliberResult() tracequery.Result {
	pressure := tracequery.IOPressureSummary{
		Signal:             "blocked_reason_iowait_count_only",
		EvidenceQuality:    tracequery.IOPressureEvidenceQualityActivityMarkerOnly,
		ScoreCaliber:       tracequery.IOPressureScoreCaliberCountWeightedActivityIndex,
		Score:              180,
		IOWaitBlockedCount: 36,
		LineStart:          10,
		LineEnd:            50,
		Summary:            "io activity signal=blocked_reason_iowait_count_only activity_index=180.000 evidence_quality=activity_marker_only score_caliber=count_weighted_activity_index block_max=0.000ms storage_max=0.000ms file_bytes=0 file_events=0 page_cache_churn=0 iowait_blocked=36 d_state=0.000ms io_wait=0.000ms",
	}
	item := tracequery.RootCauseRankItem{
		Type:                          "io_pressure",
		SubjectKind:                   tracequery.RootCauseSubjectKindAggregateMetric,
		Tier:                          tracequery.RootCauseTierContextOnly,
		CumulativeImpactMs:            180,
		ImpactMs:                      180,
		Source:                        "window_stats.io_pressure_summary",
		Causality:                     "background",
		ChainRelevance:                "background",
		LineStart:                     10,
		LineEnd:                       50,
		IOPressureSignal:              pressure.Signal,
		IOPressureEvidenceQuality:     pressure.EvidenceQuality,
		IOPressureScoreCaliber:        pressure.ScoreCaliber,
		IOPressureIOWaitBlockedCount:  pressure.IOWaitBlockedCount,
		IOPressureBlockMaxLatencyMs:   pressure.BlockMaxLatencyMs,
		IOPressureStorageMaxLatencyMs: pressure.StorageMaxLatencyMs,
		IOPressureFileIOBytes:         pressure.FileIOBytes,
		IOPressureFileIOEvents:        pressure.FileIOEvents,
		IOPressurePageCacheChurn:      pressure.PageCacheChurn,
		Summary:                       pressure.Summary,
		Confidence:                    0.70,
	}
	return tracequery.Result{
		View: "frame_root_cause_bundle",
		WindowStats: &tracequery.WindowStats{
			IOPressureSummary: &pressure,
		},
		RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 1, EndTs: 2},
			Items:  []tracequery.RootCauseRankItem{item},
		},
	}
}
