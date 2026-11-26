package types

import "testing"

func TestRuntimeComparisonBinding(t *testing.T) {
	ptr := func(n int64) *int64 { return &n }
	baseline := TraceComparisonMeasurement{ID: "b", Source: "baseline.trace", Metric: "thread.runnable_wall_time", Caliber: "target_thread_state_partition", Value: ptr(15)}
	current := TraceComparisonMeasurement{ID: "c", Source: "current.trace", Metric: baseline.Metric, Caliber: baseline.Caliber, Value: ptr(40)}
	c := &RuntimeComparisonCatalog{map[string]TraceComparisonMeasurement{"b": baseline}, map[string]TraceComparisonMeasurement{"c": current}}
	b, r := &RuntimeComparisonOperand{MeasurementID: "b"}, &RuntimeComparisonOperand{MeasurementID: "c"}
	receipt := &AnswerRuntimeComparisonReceipt{Baseline: b, Current: r}
	assert := func(want int64) {
		t.Helper()
		f, err := c.Bind("test", "explanation", receipt)
		if err != nil || f.Delta == nil || *f.Delta != want || f.CausalStatus != "unresolved" {
			t.Fatalf("bad comparison: %+v, %v", f, err)
		}
		receipt.Bound = f
	}
	assert(25)
	current.Value = ptr(15)
	c.Current["c"] = current
	assert(0)
	current.Value = ptr(10)
	c.Current["c"] = current
	assert(-5)
	baseline.Value = ptr(0)
	c.Baseline["b"] = baseline
	assert(10)
	if receipt.Bound.RelativeChange != nil || receipt.Bound.RelativeReason != "baseline_is_zero" {
		t.Fatal("zero baseline had relative change")
	}
	delete(c.Current, "c")
	if _, err := c.Bind("test", "explanation", receipt); err == nil {
		t.Fatal("fresh selector accepted an unknown ID")
	}
	f, err := c.Revalidate("test", "explanation", receipt)
	if err != nil || f.Delta != nil || f.Baseline.Value == nil || f.Current.Value != nil {
		t.Fatalf("bad stale projection: %+v %v", f, err)
	}
	receipt.Current = nil
	missing, err := c.Bind("test", "explanation", receipt)
	if err != nil || missing.Delta != nil || missing.Reason != "current_measurement_unavailable" {
		t.Fatalf("bad missing projection: %+v %v", missing, err)
	}
	clone := receipt.Clone()
	*clone.Bound.Delta = 99
	if *receipt.Bound.Delta != 10 {
		t.Fatal("comparison draft aliases accepted delta")
	}
}

func TestRuntimeComparisonEndpointOwnership(t *testing.T) {
	catalog := map[string]TraceComparisonMeasurement{
		"a": {ID: "a", Source: "trace", Generation: "v", ThreadID: 7, Metric: "a", Caliber: "marker_begin", StartLine: 10, StartNS: 10},
		"b": {ID: "b", Source: "trace", Generation: "v", ThreadID: 7, Metric: "b", Caliber: "marker_begin", StartLine: 20, StartNS: 35},
	}
	operand := &RuntimeComparisonOperand{StartID: "a", EndID: "b"}
	value, err := comparisonOperand(operand, catalog)
	if err != nil || *value.Value != 25 || value.StartLine != 10 || value.EndLine != 20 {
		t.Fatalf("bad marker interval: %+v %v", value, err)
	}
	bad := catalog["b"]
	bad.ThreadID = 8
	catalog["b"] = bad
	if _, err := comparisonOperand(operand, catalog); err == nil {
		t.Fatal("cross-thread interval accepted")
	}
}

func TestRuntimeComparisonAddsUniqueFrameMeasure(t *testing.T) {
	span := func(id, source, metric string, tid, from, to int) TraceComparisonMeasurement {
		value := int64(to - from)
		return TraceComparisonMeasurement{
			ID: id, Source: source, Generation: "capture", Metric: metric,
			Caliber: "closed_sync_span", ThreadID: tid, StartLine: from, EndLine: to,
			StartNS: int64(from), EndNS: int64(to), Value: &value,
		}
	}
	bFrame := span("bf", "baseline", "Choreographer#doFrame", 1, 10, 100)
	tFrame := span("tf", "target", "Choreographer#doFrame", 2, 200, 300)
	bMeasure := span("bm", "baseline", "measure", 1, 20, 30)
	tMeasure := span("tm", "target", "measure", 2, 210, 240)
	catalog := &RuntimeComparisonCatalog{
		Baseline: map[string]TraceComparisonMeasurement{
			"bf": bFrame, "bm": bMeasure,
			"earlier": span("earlier", "baseline", "measure", 1, 3, 8),
		},
		Current: map[string]TraceComparisonMeasurement{
			"tf": tFrame, "tm": tMeasure,
			"other-thread": span("other-thread", "target", "measure", 3, 212, 250),
		},
	}
	receipt := &AnswerRuntimeComparisonReceipt{
		Baseline: &RuntimeComparisonOperand{MeasurementID: bFrame.ID},
		Current:  &RuntimeComparisonOperand{MeasurementID: tFrame.ID},
	}
	var err error
	receipt.Bound, err = catalog.Bind("frame", "", receipt)
	if err != nil {
		t.Fatal(err)
	}
	newDoc := func() *AnswerDocumentV2 {
		return &AnswerDocumentV2{Blocks: []AnswerBlock{{ID: "frame", Kind: BlockSection, RuntimeComparison: receipt}}}
	}
	doc := newDoc()
	appendFrameComparisonEvidence(doc, catalog, "zh-CN")
	appendFrameComparisonEvidence(doc, catalog, "zh-CN")
	if len(doc.Blocks) != 3 {
		t.Fatalf("want one measure and one submission evidence block, got %d blocks", len(doc.Blocks))
	}
	measure, submission := doc.Blocks[1].RuntimeComparison.Bound, doc.Blocks[2].RuntimeComparison.Bound
	if measure.Baseline.ID != "bm" || measure.Current.ID != "tm" || *measure.Delta != 20 || measure.CausalStatus != "unresolved" {
		t.Fatalf("unexpected frame measure: %+v", measure)
	}
	if doc.FrameComparisonLead != measure {
		t.Fatal("the lead must reference the exact published measure comparison")
	}
	if submission.Status != "unavailable" || submission.Delta != nil {
		t.Fatalf("unexpected submission result: %+v", submission)
	}
	catalog.Current["second"] = span("second", "target", "measure", 2, 250, 260)
	doc = newDoc()
	appendFrameComparisonEvidence(doc, catalog, "zh-CN")
	if len(doc.Blocks) != 2 || doc.Blocks[1].RuntimeComparison.Bound.Status != "unavailable" || doc.FrameComparisonLead != nil {
		t.Fatal("ambiguous on-frame measure should not produce a duration comparison")
	}
}
