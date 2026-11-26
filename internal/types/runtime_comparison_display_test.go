package types

import (
	"strings"
	"testing"
)

func TestRuntimeComparisonDisplayCorrectsConflictingBaselineLabel(t *testing.T) {
	value, relayout := int64(4_781_000), int64(7_370_000)
	measure := TraceComparisonMeasurement{
		Metric: "measure", Source: "baseline", Generation: "capture", ThreadID: 7,
		StartNS: 100_000_000, EndNS: 104_781_000, Value: &value,
	}
	observedRelayout := measure
	observedRelayout.Metric, observedRelayout.Caliber = "relayoutWindow", "closed_sync_span"
	observedRelayout.StartNS, observedRelayout.EndNS, observedRelayout.Value = 105_000_000, 112_370_000, &relayout
	frame := measure
	frame.StartNS, frame.EndNS = 99_000_000, 200_000_000
	wrong := "基线 relayoutWindow 耗时 4.781ms。"
	right := "基线 relayoutWindow 耗时 7.370 ms。"
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{Kind: BlockSection, Text: wrong + right, RuntimeComparison: &AnswerRuntimeComparisonReceipt{Bound: &TraceComparisonFact{Explanation: wrong + right}}},
		{Kind: BlockTable, Items: []AnswerBlockItem{
			{Cells: []string{"relayoutWindow", "无数据", "0.100000–0.104781，4.781 ms"}},
			{Cells: []string{"relayoutWindow", "无数据", "0.105000–0.112370，7.370 ms"}},
		}},
	}}
	reconcileFrameMeasurementDisplay(doc, measure, &frame, map[string]TraceComparisonMeasurement{"relayout": observedRelayout}, "zh-CN")
	got := doc.Blocks[0]
	if strings.Contains(got.Text, wrong) || !strings.Contains(got.Text, "基线帧的 `measure` 区间耗时 4.781 ms。") ||
		!strings.Contains(got.Text, right) || got.RuntimeComparison.Bound.Explanation != got.Text {
		t.Fatalf("incorrect displayed explanation: %q", got.Text)
	}
	if items := doc.Blocks[1].Items; len(items) != 1 || !strings.Contains(items[0].Cells[2], "7.370 ms") {
		t.Fatalf("unexpected display rows: %+v", items)
	}
}
