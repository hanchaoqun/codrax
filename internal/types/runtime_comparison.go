package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// TraceComparisonMeasurement is a numeric native observation. A marker has
// only a start coordinate. Complete spans and state accounts also have an
// end coordinate and a value in nanoseconds.
type TraceComparisonMeasurement struct {
	ID           string `json:"measurement_id"`
	Source       string `json:"source_ref"`
	Query        string `json:"query_ref"`
	Generation   string `json:"source_generation"`
	Metric       string `json:"metric"`
	Caliber      string `json:"caliber"`
	Coverage     string `json:"coverage"`
	ThreadID     int    `json:"thread_id"`
	StartLine    int    `json:"start_line,omitempty"`
	EndLine      int    `json:"end_line,omitempty"`
	StartNS      int64  `json:"start_ns"`
	EndNS        int64  `json:"end_ns"`
	QueryStartNS *int64 `json:"query_start_ns,omitempty"`
	QueryEndNS   *int64 `json:"query_end_ns,omitempty"`
	Status       string `json:"status"`
	Value        *int64 `json:"value"`
}

type RuntimeComparisonOperand struct {
	MeasurementID string `json:"measurement_id,omitempty"`
	StartID       string `json:"start_id,omitempty"`
	EndID         string `json:"end_id,omitempty"`
}

type AnswerRuntimeComparisonReceipt struct {
	MissingMetric string                    `json:"missing_metric,omitempty"`
	Baseline      *RuntimeComparisonOperand `json:"baseline,omitempty"`
	Current       *RuntimeComparisonOperand `json:"current,omitempty"`
	// Only the current run's binding may populate this pointer.
	Bound *TraceComparisonFact `json:"-"`
}

func (r *AnswerRuntimeComparisonReceipt) Clone() *AnswerRuntimeComparisonReceipt {
	if r == nil {
		return nil
	}
	out := *r
	if r.Baseline != nil {
		v := *r.Baseline
		out.Baseline = &v
	}
	if r.Current != nil {
		v := *r.Current
		out.Current = &v
	}
	if r.Bound != nil {
		out.Bound = cloneTraceComparisonJSON(r.Bound)
	}
	return &out
}

type TraceComparisonFact struct {
	ID             string                      `json:"comparison_id"`
	Metric         string                      `json:"metric"`
	QuantityKind   string                      `json:"quantity_kind"`
	Baseline       *TraceComparisonMeasurement `json:"baseline"`
	Current        *TraceComparisonMeasurement `json:"current"`
	Operation      string                      `json:"operation"`
	Unit           string                      `json:"unit"`
	Delta          *int64                      `json:"delta"`
	RelativeChange *float64                    `json:"relative_change_pct"`
	Status         string                      `json:"status"`
	Reason         string                      `json:"reason_code,omitempty"`
	RelativeReason string                      `json:"relative_change_reason,omitempty"`
	Summary        string                      `json:"summary"`
	Explanation    string                      `json:"explanation,omitempty"`
	CausalStatus   string                      `json:"causal_status"`
}

type TraceComparisonReport struct {
	SchemaVersion int                    `json:"schema_version"`
	Status        string                 `json:"status"`
	Comparisons   []*TraceComparisonFact `json:"comparisons"`
}

func cloneTraceComparisonJSON[T any](in *T) *T {
	if in == nil {
		return nil
	}
	raw, _ := json.Marshal(in)
	var out T
	_ = json.Unmarshal(raw, &out)
	return &out
}

func (r *TraceComparisonReport) Clone() *TraceComparisonReport { return cloneTraceComparisonJSON(r) }

func (r *TraceComparisonReport) Add(fact *TraceComparisonFact) {
	r.SchemaVersion = 1
	r.Comparisons = append(r.Comparisons, cloneTraceComparisonJSON(fact))
	if len(r.Comparisons) == 1 {
		r.Status = "unavailable"
		if fact.Status == "available" {
			r.Status = "available"
		}
		return
	}
	if (r.Status == "available") != (fact.Status == "available") {
		r.Status = "partial"
	}
}

func TraceComparisonTimeNS(seconds float64) (int64, bool) {
	n := math.Round(seconds * 1e9)
	return int64(n), n >= 0 && n < float64(math.MaxInt64)
}

func traceComparisonID(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return fmt.Sprintf("cmp-%x", sum[:12])
}

// Make instance-numbered frame markers comparable without modifying any
// evidence coordinate or guessing that different business names match.
func TraceComparisonMarkerName(name string) string {
	for _, prefix := range []string{"Choreographer#doFrame", "DrawFrames"} {
		if suffix, ok := strings.CutPrefix(name, prefix+" "); ok {
			if _, err := strconv.ParseUint(suffix, 10, 64); err == nil {
				return prefix
			}
		}
	}
	return name
}

type RuntimeComparisonCatalog struct {
	Baseline map[string]TraceComparisonMeasurement
	Current  map[string]TraceComparisonMeasurement
}

// BuildRuntimeComparisonCatalog accepts only current native producer results
// whose original capture matches the request's explicit side ownership.
func BuildRuntimeComparisonCatalog(bus *BusContext) *RuntimeComparisonCatalog {
	input := ObservationLedgerInputFromBusContext(bus, 0)
	if input.RequestModel == nil || input.RequestModel.RuntimeArtifactScopeProfile == nil {
		return nil
	}
	profile := input.RequestModel.RuntimeArtifactScopeProfile
	if !profile.Comparison.Valid() {
		return nil
	}
	out := &RuntimeComparisonCatalog{map[string]TraceComparisonMeasurement{}, map[string]TraceComparisonMeasurement{}}
	add := func(result ToolResult) {
		hasOperands := false
		for _, row := range result.Observations {
			if len(row.TraceComparisonMeasurements) > 0 {
				hasOperands = true
				break
			}
		}
		if !hasOperands {
			return
		}
		if _, ok := nativeFactCurrentPublication(input.runtimeMeasurementPairConsumer, result); !ok {
			return
		}
		for _, row := range result.Observations {
			if len(row.TraceComparisonMeasurements) == 0 {
				continue
			}
			original, replay := runtimeMeasurementOriginalSourceReplay(input, result, row.SourceRef)
			if original == "" {
				continue
			}
			side := out.Current
			if RuntimeArtifactComparisonPathEqual(original, profile.Comparison.Baseline.Path, input.RepoRoot) {
				side = out.Baseline
			} else if !RuntimeArtifactComparisonPathEqual(original, profile.Comparison.Current.Path, input.RepoRoot) {
				continue
			}
			for _, measurement := range row.TraceComparisonMeasurements {
				end := measurement.EndNS
				if end == measurement.StartNS {
					end++
				}
				if !profile.ComparisonWindowAllows(original, input.RepoRoot, float64(measurement.StartNS)/1e9, float64(end)/1e9) {
					continue
				}
				measurement.Status = "available"
				measurement.Source = original
				measurement.Query = row.SourceRef.QueryScopeID
				measurement.Generation = replay.source.identity.CacheToken()
				if replay.preparedSource != nil && replay.preparedSource.material != nil {
					measurement.Generation = replay.preparedSource.material.GenerationCacheKey()
				}
				// Coordinates and source generation identify the operand across repeated
				// observations. Retain one full query witness for the chosen instance.
				key := measurement
				key.Query = ""
				key.QueryStartNS, key.QueryEndNS = nil, nil
				measurement.ID = traceComparisonID(key)
				if _, exists := side[measurement.ID]; !exists {
					side[measurement.ID] = measurement
				}
			}
		}
	}
	for _, group := range [][]ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range group {
			add(result)
			for _, child := range result.RuntimeMeasurementPair.comparisonResults {
				if child != nil {
					add(*child)
				}
			}
		}
	}
	return out
}

func comparisonOperand(selection *RuntimeComparisonOperand, catalog map[string]TraceComparisonMeasurement) (*TraceComparisonMeasurement, error) {
	if selection == nil {
		return nil, nil
	}
	if selection.MeasurementID != "" && selection.StartID == "" && selection.EndID == "" {
		value, ok := catalog[selection.MeasurementID]
		if !ok || value.Value == nil {
			return nil, fmt.Errorf("unknown or non-scalar measurement %q", selection.MeasurementID)
		}
		return &value, nil
	}
	if selection.MeasurementID != "" || selection.StartID == "" || selection.EndID == "" {
		return nil, fmt.Errorf("select one measurement_id or both marker endpoints")
	}
	start, startOK := catalog[selection.StartID]
	end, endOK := catalog[selection.EndID]
	isMarker := func(m TraceComparisonMeasurement) bool {
		return m.Caliber == "marker_begin" || m.Caliber == "marker_instant"
	}
	if !startOK || !endOK || !isMarker(start) || !isMarker(end) ||
		start.Source != end.Source || start.Generation != end.Generation || start.ThreadID != end.ThreadID || end.StartNS <= start.StartNS || end.StartLine <= start.StartLine {
		return nil, fmt.Errorf("marker endpoints must be ordered and belong to the same source, generation and thread")
	}
	value := end.StartNS - start.StartNS
	start.ID = traceComparisonID([2]string{start.ID, end.ID})
	start.Query += " | " + end.Query
	// The exact interval is delimited by two endpoint receipts, which may
	// have different acquisition windows.
	start.QueryStartNS, start.QueryEndNS = nil, nil
	start.EndNS, start.EndLine = end.StartNS, end.StartLine
	start.Metric += " → " + end.Metric
	start.Caliber, start.Coverage, start.Value = start.Caliber+"_to_"+end.Caliber, "endpoints_observed", &value
	return &start, nil
}

func (c *RuntimeComparisonCatalog) Bind(id, explanation string, r *AnswerRuntimeComparisonReceipt) (*TraceComparisonFact, error) {
	if c == nil {
		return nil, fmt.Errorf("runtime_comparison requires request-owned baseline and current Trace sources")
	}
	b, err := comparisonOperand(r.Baseline, c.Baseline)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	t, err := comparisonOperand(r.Current, c.Current)
	if err != nil {
		return nil, fmt.Errorf("current: %w", err)
	}
	if b == nil && t == nil && (strings.TrimSpace(r.MissingMetric) == "" || len(r.MissingMetric) > 192) {
		return nil, fmt.Errorf("select observed operands or specify the missing metric")
	}
	if (b != nil || t != nil) && r.MissingMetric != "" {
		return nil, fmt.Errorf("missing_metric is only for two missing operands")
	}
	return buildTraceComparisonFact(id, explanation, r.MissingMetric, b, t), nil
}

func (c *RuntimeComparisonCatalog) Revalidate(id, explanation string, r *AnswerRuntimeComparisonReceipt) (*TraceComparisonFact, error) {
	if r.Bound == nil {
		return nil, fmt.Errorf("no accepted comparison to revalidate")
	}
	var b, t *TraceComparisonMeasurement
	if c != nil {
		b, _ = comparisonOperand(r.Baseline, c.Baseline)
		t, _ = comparisonOperand(r.Current, c.Current)
	}
	return buildTraceComparisonFact(id, explanation, r.Bound.Metric, b, t), nil
}

func buildTraceComparisonFact(id, explanation, metric string, b, t *TraceComparisonMeasurement) *TraceComparisonFact {
	f := &TraceComparisonFact{ID: id, QuantityKind: "measured_duration", Baseline: b, Current: t, Operation: "current_minus_baseline", Unit: "ns", Status: "unavailable", Explanation: explanation, CausalStatus: "unresolved"}
	switch {
	case b == nil && t == nil:
		f.Metric, f.Reason = metric, "both_measurements_unavailable"
	case b == nil:
		f.Metric, f.Reason = t.Metric, "baseline_measurement_unavailable"
	case t == nil:
		f.Metric, f.Reason = b.Metric, "current_measurement_unavailable"
	case b.Source == t.Source || b.Metric != t.Metric || b.Caliber != t.Caliber:
		f.Metric, f.Status, f.Reason = t.Metric, "not_comparable", "incompatible_measurement"
	default:
		f.Metric = t.Metric
		delta := *t.Value - *b.Value
		f.Delta, f.Status = &delta, "available"
		if *b.Value != 0 {
			pct := float64(delta) / float64(*b.Value) * 100
			f.RelativeChange = &pct
		} else {
			f.RelativeReason = "baseline_is_zero"
		}
	}
	if f.Delta == nil {
		if f.Baseline == nil {
			f.Baseline = &TraceComparisonMeasurement{Metric: f.Metric, Status: "unavailable"}
		}
		if f.Current == nil {
			f.Current = &TraceComparisonMeasurement{Metric: f.Metric, Status: "unavailable"}
		}
		f.RelativeReason = "comparison_unavailable"
		f.Summary = fmt.Sprintf("%s — %s", f.Metric, f.Reason)
	} else {
		f.Summary = fmt.Sprintf("%s — 目标 %.3f ms，基线 %.3f ms，差值 %+.3f ms。", f.Metric, float64(*t.Value)/1e6, float64(*b.Value)/1e6, float64(*f.Delta)/1e6)
	}
	return f
}

// AnswerRuntimeComparisonSnapshot revalidates accepted comparison operands once
// before the final render. The rendered text and JSON share this projection.
func AnswerRuntimeComparisonSnapshot(doc *AnswerDocumentV2, bus *BusContext) *AnswerDocumentV2 {
	if doc == nil {
		return nil
	}
	var snapshot *AnswerDocumentV2
	var catalog *RuntimeComparisonCatalog
	for i, block := range doc.Blocks {
		if block.RuntimeComparison == nil || block.RuntimeComparison.Bound == nil {
			continue
		}
		if snapshot == nil {
			snapshot = cloneAnswerDocumentV2(doc)
			catalog = BuildRuntimeComparisonCatalog(bus)
		}
		current := &snapshot.Blocks[i]
		current.RuntimeComparison.Bound, _ = catalog.Revalidate(current.ID, current.Text, current.RuntimeComparison)
	}
	if snapshot != nil {
		appendFrameComparisonEvidence(snapshot, catalog, bus.Language)
		return snapshot
	}
	return doc
}

// A bound doFrame pair identifies the two frame instances. Add only a phase
// that matches uniquely within both frames, without drawing a causal inference.
func appendFrameComparisonEvidence(doc *AnswerDocumentV2, catalog *RuntimeComparisonCatalog, language string) {
	doc.FrameComparisonLead = nil
	var frame *TraceComparisonFact
	for _, block := range doc.Blocks {
		if block.RuntimeComparison == nil {
			continue
		}
		fact := block.RuntimeComparison.Bound
		if fact == nil || fact.Status != "available" || fact.Metric != "Choreographer#doFrame" ||
			fact.Baseline.Caliber != "closed_sync_span" {
			continue
		}
		if frame != nil && (frame.Baseline.ID != fact.Baseline.ID || frame.Current.ID != fact.Current.ID) {
			return
		}
		frame = fact
	}
	if frame == nil {
		return
	}
	appendEvidence := func(id, explanation string, receipt *AnswerRuntimeComparisonReceipt) *TraceComparisonFact {
		for {
			if !slices.ContainsFunc(doc.Blocks, func(b AnswerBlock) bool { return b.ID == id }) {
				break
			}
			id += "-evidence"
		}
		fact, err := catalog.Bind(id, explanation, receipt)
		if err != nil {
			return nil
		}
		receipt.Bound = fact
		doc.Blocks = append(doc.Blocks, AnswerBlock{
			ID: id, Kind: BlockSection, Text: explanation,
			SystemGeneratedKind: AnswerSystemGeneratedEvidenceSupplement,
			RuntimeComparison:   receipt,
		})
		return fact
	}
	measure := func(side map[string]TraceComparisonMeasurement, parent *TraceComparisonMeasurement) string {
		id := ""
		for _, m := range side {
			if m.Metric != "measure" || m.Caliber != "closed_sync_span" || m.Source != parent.Source ||
				m.Generation != parent.Generation || m.ThreadID != parent.ThreadID ||
				m.StartLine <= parent.StartLine || m.EndLine >= parent.EndLine ||
				m.StartNS < parent.StartNS || m.EndNS > parent.EndNS {
				continue
			}
			if id != "" {
				return ""
			}
			id = m.ID
		}
		return id
	}
	const submission = "doFrame → queueBuffer"
	b, t := measure(catalog.Baseline, frame.Baseline), measure(catalog.Current, frame.Current)
	var measureFact *TraceComparisonFact
	hasSubmission := false
	for _, block := range doc.Blocks {
		if block.RuntimeComparison == nil {
			continue
		}
		fact := block.RuntimeComparison.Bound
		if fact == nil {
			continue
		}
		if measureFact == nil && fact.Status == "available" && fact.Metric == "measure" &&
			fact.Baseline.ID == b && fact.Current.ID == t {
			measureFact = fact
		}
		hasSubmission = hasSubmission || fact.Metric == submission || fact.Status == "available" && strings.Contains(fact.Metric, "→ queueBuffer")
	}
	chinese := strings.HasPrefix(strings.ToLower(language), "zh")
	if b != "" && t != "" {
		reconcileFrameMeasurementDisplay(doc, catalog.Baseline[b], frame.Baseline, catalog.Baseline, language)
	}
	if b != "" && t != "" && measureFact == nil {
		explanation := "Both durations are closed measure spans on the respective doFrame threads."
		if chinese {
			explanation = "两侧数值均取自各自 doFrame 主线程内完整闭合的 measure 区间。"
		}
		measureFact = appendEvidence("frame-measure-comparison", explanation, &AnswerRuntimeComparisonReceipt{
			Baseline: &RuntimeComparisonOperand{MeasurementID: b},
			Current:  &RuntimeComparisonOperand{MeasurementID: t},
		})
	}
	if b != "" && t != "" && measureFact != nil && *measureFact.Delta != 0 {
		doc.FrameComparisonLead = measureFact
	}
	hasEndpoint := func(side map[string]TraceComparisonMeasurement) bool {
		for _, m := range side {
			if m.Metric == "queueBuffer" {
				return true
			}
		}
		return false
	}
	if !hasSubmission && (!hasEndpoint(catalog.Baseline) || !hasEndpoint(catalog.Current)) {
		explanation := "Insufficient evidence for the submission interval: the retained measurements do not contain a queueBuffer endpoint on both sides."
		if chinese {
			explanation = "提交链证据不足：保留的测量未覆盖两侧的 queueBuffer 端点。"
		}
		appendEvidence("frame-submission-evidence", explanation, &AnswerRuntimeComparisonReceipt{MissingMetric: submission})
	}
}
