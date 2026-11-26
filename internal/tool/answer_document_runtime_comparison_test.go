package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeComparisonPromptKeepsBothFrameMeasurements(t *testing.T) {
	catalog := &types.RuntimeComparisonCatalog{
		Baseline: map[string]types.TraceComparisonMeasurement{},
		Current:  map[string]types.TraceComparisonMeasurement{},
	}
	for _, side := range []map[string]types.TraceComparisonMeasurement{catalog.Baseline, catalog.Current} {
		for i := range 60 {
			id := fmt.Sprintf("other-%d", i)
			value := int64(1_000)
			side[id] = types.TraceComparisonMeasurement{ID: id, Metric: "other", Value: &value}
		}
	}
	baseline, current := int64(4_781_000), int64(27_157_000)
	catalog.Baseline["base"] = types.TraceComparisonMeasurement{ID: "base", Metric: "measure", Caliber: "closed_sync_span", StartNS: 20, Value: &baseline}
	catalog.Current["target"] = types.TraceComparisonMeasurement{ID: "target", Metric: "measure", Caliber: "closed_sync_span", StartNS: 30, Value: &current}
	prompt := renderRuntimeComparisonChoices(catalog)
	for _, want := range []string{"base measure", "4.781 ms", "target measure", "27.157 ms", "insufficient evidence"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	for _, side := range []map[string]types.TraceComparisonMeasurement{catalog.Baseline, catalog.Current} {
		if choices := runtimeComparisonChoices(side, false); len(choices) != 48 || choices[0].Metric != "measure" {
			t.Fatalf("frame measurement was not prioritized in the bounded selector roster")
		}
	}
}

func TestRuntimeComparisonSummaryRepairPreservesScalarSelection(t *testing.T) {
	base, target := int64(4_781_000), int64(27_157_000)
	catalog := &types.RuntimeComparisonCatalog{
		Baseline: map[string]types.TraceComparisonMeasurement{
			"b": {ID: "b", Source: "baseline", Metric: "measure", Caliber: "closed_sync_span", Value: &base},
			"bf": {Metric: "Choreographer#doFrame"},
		},
		Current: map[string]types.TraceComparisonMeasurement{
			"t": {ID: "t", Source: "target", Metric: "measure", Caliber: "closed_sync_span", Value: &target},
			"tf": {Metric: "Choreographer#doFrame"},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{ID: "lead", Kind: types.BlockSummary, Text: "Main answer"},
		{ID: "comparison", Kind: types.BlockSummary, Text: "An explanation for the conflicting endpoint selection", RuntimeComparison: &types.AnswerRuntimeComparisonReceipt{
			Baseline: &types.RuntimeComparisonOperand{MeasurementID: "b", StartID: "a", EndID: "z"},
			Current: &types.RuntimeComparisonOperand{MeasurementID: "t", StartID: "c", EndID: "y"},
		}},
		{ID: "invalid", Kind: types.BlockSummary, RuntimeComparison: &types.AnswerRuntimeComparisonReceipt{
			Baseline: &types.RuntimeComparisonOperand{MeasurementID: "b", StartID: "a", EndID: "z"},
			Current: &types.RuntimeComparisonOperand{MeasurementID: "unknown"},
		}},
	}}
	if n := moveComparisonSummariesToSections(doc, 0, catalog); n != 1 {
		t.Fatalf("moved %d summaries", n)
	}
	main, moved, invalid := doc.Blocks[0], doc.Blocks[1], doc.Blocks[2]
	if main.Kind != types.BlockSummary || main.Text != "Main answer" || invalid.Kind != types.BlockSummary ||
		moved.Kind != types.BlockSection || moved.ID != "comparison" || moved.Text != "" {
		t.Fatalf("unexpected repaired block layout: %+v", doc.Blocks)
	}
	b, c := moved.RuntimeComparison.Baseline, moved.RuntimeComparison.Current
	if *b != (types.RuntimeComparisonOperand{MeasurementID: "b"}) || *c != (types.RuntimeComparisonOperand{MeasurementID: "t"}) {
		t.Fatalf("wrong scalar selectors: %+v %+v", b, c)
	}
	markMissingFrameSubmissionComparison(doc, catalog, "zh-CN")
	markMissingFrameSubmissionComparison(doc, catalog, "zh-CN")
	if len(doc.Blocks) != 4 || doc.Blocks[3].RuntimeComparison.MissingMetric != "doFrame → queueBuffer" {
		t.Fatalf("missing submission status must be emitted once")
	}
}
