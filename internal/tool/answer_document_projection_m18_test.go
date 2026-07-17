package tool

// answer_document_projection_m18_test.go — RANKDIS-M18 report-face pins.
// io_pressure deliberately remains an aggregate context row (zero rank-seat
// behavior change), while its published magnitude moved to the typed
// composite_score caliber. The shared projection renderer must therefore use
// the suffix-free comprehensive-score word on the tree, key-metric table and
// comparison panel, without deriving an ms/window pseudo-density.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func m18IOPressureProjectionRecords() []types.ObservationRecord {
	return []types.ObservationRecord{
		{
			ID:              "m18-wall-clock-control",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_primary",
			ClaimKey:        "root_cause_primary",
			Subject:         "worker-20",
			Object:          "runnable",
			Value:           "4.000",
			Unit:            "ms",
			Span:            types.ObservationSpan{LineStart: 10, LineEnd: 20},
			RichNotes: []string{
				"tier=primary", "rank=1", "type=runnable", "impact_ms=4.000",
				"cumulative_impact_ms=4.000", "effective_impact_ms=4.000",
				"chain_relevance=on_chain", "causality=on_wakeup_chain",
				"selected_window=5.000000..5.007000",
			},
			Confidence: 0.8,
		},
		{
			ID:              "m18-io-pressure-context",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRoleSupportingCoverage,
			GroundingPolicy: types.ClaimGroundingHard,
			Predicate:       "root_cause_context_only",
			ClaimKey:        "root_cause_context_only",
			Object:          "io_pressure",
			Value:           "61.540",
			Unit:            types.TraceObservationUnitCompositeScore,
			Span:            types.ObservationSpan{LineStart: 30, LineEnd: 40},
			RichNotes: []string{
				"tier=context_only", "type=io_pressure", "subject_kind=aggregate_metric",
				"impact_score=61.540", "cumulative_impact_score=61.540",
				"effective_impact_score=0.000", "chain_relevance=background",
				"selected_window=5.000000..5.007000",
			},
			Confidence: 0.7,
		},
	}
}

func TestM18IOPressureContextProjectionKeepsCompositeValueCaliber(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(m18IOPressureProjectionRecords())
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		var pressure *types.TraceCausalProjectionNode
		for i := range model.Background {
			if model.Background[i].Node.Object == "io_pressure" {
				pressure = &model.Background[i].Node
				break
			}
		}
		if pressure == nil {
			t.Fatalf("zh=%v: io_pressure context row must retain its background placement: %+v", zh, model.Background)
		}
		if !pressure.IsContextOnlyRow() || pressure.Rank != 0 {
			t.Fatalf("zh=%v: value-caliber fix must not change context-only seating: %+v", zh, *pressure)
		}
		if !runtimeTraceProjCompositeValueCaliber(*pressure) {
			t.Fatalf("zh=%v: typed composite_score Unit must drive the report value caliber: %+v", zh, *pressure)
		}

		wantScore := runtimeTraceProjCompositeScoreValueText(61.540, zh)
		fence := runtimeTraceProjTreeFence(model, zh)
		if !strings.Contains(fence, wantScore) {
			t.Fatalf("zh=%v: tree must carry the composite value word %q:\n%s", zh, wantScore, fence)
		}
		if strings.Contains(fence, "61.540ms") || strings.Contains(fence, "avg queue") || strings.Contains(fence, "≈均值") {
			t.Fatalf("zh=%v: composite score must not regain an ms suit or score/window density:\n%s", zh, fence)
		}
		if !strings.Contains(fence, "4.000ms") {
			t.Fatalf("zh=%v: ordinary wall-clock rows must remain byte-identical:\n%s", zh, fence)
		}

		_, rows := runtimeTraceProjDetailTable(model, zh)
		var pressureCells []string
		for _, row := range rows {
			joined := strings.Join(row.Cells, " | ")
			if strings.Contains(joined, "io_pressure") || strings.Contains(joined, "IO压力") || strings.Contains(joined, "IO pressure") {
				pressureCells = row.Cells
				break
			}
		}
		if pressureCells == nil {
			t.Fatalf("zh=%v: io_pressure row missing from the shared key-metric table: %+v", zh, rows)
		}
		joined := strings.Join(pressureCells, " | ")
		if !strings.Contains(joined, wantScore) || strings.Contains(joined, "61.540ms") || strings.Contains(joined, "cross-thread cumulative") || strings.Contains(joined, "跨线程累计") {
			t.Fatalf("zh=%v: table must use only the composite value caliber, got %q", zh, joined)
		}

		cell, densityWindow := runtimeTraceProjCompareBackgroundPressureCell(model, zh)
		if cell != wantScore || densityWindow != 0 {
			t.Fatalf("zh=%v: comparison panel must publish score without pseudo-density, cell=%q window=%.3f", zh, cell, densityWindow)
		}
	}
}

func TestM18BackgroundComparisonNeverOrdersScoreAgainstDuration(t *testing.T) {
	records := append([]types.ObservationRecord(nil), m18IOPressureProjectionRecords()...)
	records = append(records, types.ObservationRecord{
		ID:              "m18-supply-pressure-duration",
		Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            types.AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: types.ClaimGroundingHard,
		Predicate:       "root_cause_context_only",
		ClaimKey:        "root_cause_context_only",
		Object:          "supply_pressure",
		Value:           "2.000",
		Unit:            "ms",
		Span:            types.ObservationSpan{LineStart: 50, LineEnd: 60},
		RichNotes: []string{
			"tier=context_only", "type=supply_pressure", "subject_kind=aggregate_metric",
			"impact_ms=2.000", "cumulative_impact_ms=2.000", "effective_impact_ms=0.000",
			"chain_relevance=background", "selected_window=5.000000..5.007000",
		},
		Confidence: 0.7,
	})
	projection := types.TraceCausalProjectionFromObservationRecords(records)
	for _, zh := range []bool{true, false} {
		model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), zh)
		cell, densityWindow := runtimeTraceProjCompareBackgroundPressureCell(model, zh)
		if densityWindow <= 0 || !strings.Contains(cell, "2.000ms") {
			t.Fatalf("zh=%v: duration aggregate must retain the comparison/density lane, cell=%q window=%.3f", zh, cell, densityWindow)
		}
		if strings.Contains(cell, "61.540") || strings.Contains(cell, "composite score") || strings.Contains(cell, "综合评分") {
			t.Fatalf("zh=%v: the numerically larger score must never compete against the duration aggregate, got %q", zh, cell)
		}
	}
}

func TestM18CompositeDisplayHelperPreservesLegacyBlockIOFallback(t *testing.T) {
	projection := types.TraceCausalProjectionFromObservationRecords(v2p0CaliberRecords())
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, runtimeTraceProjCompositeScoreValueText(0.198, true)) || strings.Contains(fence, "0.198ms") {
		t.Fatalf("legacy block_io caliber-side rows must keep their suffix-free composite word:\n%s", fence)
	}
}
