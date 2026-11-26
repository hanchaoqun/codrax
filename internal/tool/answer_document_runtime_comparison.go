package tool

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func repairRuntimeComparisonSummaries(doc *types.AnswerDocumentV2, view *types.AnswerSemanticView, ctx *types.BusContext) int {
	singleSummary := false
	for _, req := range view.RequiredBlocks {
		if req.Required && req.Kind == types.BlockSummary && req.MaxCount == 1 && len(req.AlternativeKinds) == 0 && len(req.FacetIDs) == 0 {
			singleSummary = true
			break
		}
	}
	if !singleSummary {
		return 0
	}
	lead, additional := -1, false
	for i, b := range doc.Blocks {
		if b.Kind == types.BlockSummary {
			if lead < 0 {
				lead = i
			} else {
				additional = true
			}
		}
	}
	if !additional {
		return 0
	}
	catalog := types.BuildRuntimeComparisonCatalog(ctx)
	moved := moveComparisonSummariesToSections(doc, lead, catalog)
	if moved > 0 {
		markMissingFrameSubmissionComparison(doc, catalog, ctx.Language)
	}
	return moved
}

func moveComparisonSummariesToSections(doc *types.AnswerDocumentV2, lead int, catalog *types.RuntimeComparisonCatalog) int {
	if catalog == nil {
		return 0
	}
	moved := 0
	for i := range doc.Blocks {
		b := &doc.Blocks[i]
		if i <= lead || b.Kind != types.BlockSummary || b.RuntimeComparison == nil {
			continue
		}
		selector := b.RuntimeComparison.Clone()
		mixed := false
		for _, op := range []*types.RuntimeComparisonOperand{selector.Baseline, selector.Current} {
			if op != nil && op.MeasurementID != "" && op.StartID != "" && op.EndID != "" {
				op.StartID, op.EndID = "", ""
				mixed = true
			}
		}
		explanation := b.Text
		if mixed {
			// The conflicting endpoint selection gives no authority to
			// attach prose about that interval to the scalar measurements.
			explanation = ""
		}
		fact, err := catalog.Bind(b.ID, explanation, selector)
		if err != nil || (mixed && fact.Status != "available") {
			continue
		}
		b.Kind, b.Text, b.RuntimeComparison = types.BlockSection, explanation, selector
		b.TraceCausalClaimCaliber = ""
		moved++
	}
	return moved
}

func markMissingFrameSubmissionComparison(doc *types.AnswerDocumentV2, catalog *types.RuntimeComparisonCatalog, language string) {
	hasMetric := func(side map[string]types.TraceComparisonMeasurement, metric string) bool {
		for _, m := range side {
			if m.Metric == metric {
				return true
			}
		}
		return false
	}
	if !hasMetric(catalog.Baseline, "Choreographer#doFrame") || !hasMetric(catalog.Current, "Choreographer#doFrame") ||
		(hasMetric(catalog.Baseline, "queueBuffer") && hasMetric(catalog.Current, "queueBuffer")) {
		return
	}
	const metric = "doFrame → queueBuffer"
	id := "frame-submission-evidence"
	for {
		collision := false
		for _, b := range doc.Blocks {
			if b.RuntimeComparison != nil && strings.Contains(b.RuntimeComparison.MissingMetric, "queueBuffer") {
				return
			}
			collision = collision || b.ID == id
		}
		if !collision {
			break
		}
		id += "-status"
	}
	explanation := "Insufficient evidence for the submission interval: the retained measurements do not contain a queueBuffer endpoint on both sides."
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		explanation = "提交链证据不足：保留的测量未覆盖两侧的 queueBuffer 端点。"
	}
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID: id, Kind: types.BlockSection, Text: explanation,
		SystemGeneratedKind: types.AnswerSystemGeneratedEvidenceSupplement,
		RuntimeComparison: &types.AnswerRuntimeComparisonReceipt{MissingMetric: metric},
	})
}

func bindAnswerRuntimeComparisons(doc *types.AnswerDocumentV2, ctx *types.BusContext) error {
	var catalog *types.RuntimeComparisonCatalog
	loaded := false
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		receipt := block.RuntimeComparison
		if receipt == nil {
			continue
		}
		if block.Kind != types.BlockSummary && block.Kind != types.BlockSection {
			return fmt.Errorf("block %q: runtime_comparison requires a summary or section", block.ID)
		}
		if !loaded {
			catalog = types.BuildRuntimeComparisonCatalog(ctx)
			loaded = true
		}
		previous := receipt.Bound
		fact, err := catalog.Bind(block.ID, block.Text, receipt)
		if err != nil && previous != nil {
			// A surviving accepted block can lose an operand when its capture
			// changes before a later patch. An explicitly submitted new selector
			// has no previous binding and must still fail on unknown references.
			fact, err = catalog.Revalidate(block.ID, block.Text, receipt)
		}
		if err != nil {
			return fmt.Errorf("block %q runtime_comparison: %w", block.ID, err)
		}
		receipt.Bound = fact
	}
	return nil
}

func projectAnswerRuntimeComparison(raw json.RawMessage, ctx *types.BusContext, arrays ...string) json.RawMessage {
	catalog := types.BuildRuntimeComparisonCatalog(ctx)
	if catalog == nil {
		return raw
	}
	operandSchema := func(operands map[string]types.TraceComparisonMeasurement) map[string]any {
		describe := func(items []types.TraceComparisonMeasurement) (string, []string) {
			rows, ids := make([]string, 0, len(items)), make([]string, 0, len(items))
			for _, m := range items {
				ids = append(ids, m.ID)
				rows = append(rows, describeRuntimeComparisonChoice(m))
			}
			return strings.Join(rows, "; "), ids
		}
		one, scalarIDs := describe(runtimeComparisonChoices(operands, false))
		points, pointIDs := describe(runtimeComparisonChoices(operands, true))
		props := map[string]any{}
		if len(scalarIDs) > 0 {
			props["measurement_id"] = map[string]any{"type": "string", "enum": scalarIDs, "description": one}
		}
		if len(pointIDs) > 0 {
			props["start_id"] = map[string]any{"type": "string", "enum": pointIDs, "description": points}
			props["end_id"] = map[string]any{"type": "string", "enum": pointIDs}
		}
		return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return raw
	}
	props, _ := root["properties"].(map[string]any)
	comparison := map[string]any{"type": "object", "additionalProperties": false,
		"description": "Use a section for a standalone quantitative Trace comparison. Keep the existing measurement IDs when moving a comparison out of a surplus summary. Select the same metric and business role on both sides. Each side uses one measurement_id or two ordered marker IDs on the same thread. Omit a missing side. If both sides lack the requested metric, provide missing_metric alone. Put explanation in block.text; program renders both values and the difference. Do not repeat unbound comparison numbers in prose. A comparison alone does not establish a cause.",
		"properties":  map[string]any{"baseline": operandSchema(catalog.Baseline), "current": operandSchema(catalog.Current), "missing_metric": map[string]any{"type": "string", "maxLength": 192}}}
	for _, name := range arrays {
		array, _ := props[name].(map[string]any)
		items, _ := array["items"].(map[string]any)
		fields, _ := items["properties"].(map[string]any)
		if fields != nil {
			fields["runtime_comparison"] = comparison
			if kind, ok := fields["kind"].(map[string]any); ok {
				if kinds, ok := kind["enum"].([]any); ok && !slices.Contains(kinds, any("section")) {
					kind["enum"] = append(kinds, "section")
				}
			}
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return raw
	}
	return out
}

func runtimeComparisonChoices(operands map[string]types.TraceComparisonMeasurement, markers bool) []types.TraceComparisonMeasurement {
	choices := make([]types.TraceComparisonMeasurement, 0, len(operands))
	for _, item := range operands {
		if (item.Value == nil) == markers {
			choices = append(choices, item)
		}
	}
	sort.Slice(choices, func(i, j int) bool {
		a, b := choices[i], choices[j]
		rankA, rankB := runtimeComparisonChoiceRank(a.Metric), runtimeComparisonChoiceRank(b.Metric)
		if rankA != rankB {
			return rankA < rankB
		}
		if a.StartNS != b.StartNS {
			return a.StartNS < b.StartNS
		}
		return a.ID < b.ID
	})
	return choices[:min(48, len(choices))]
}

func runtimeComparisonChoiceRank(metric string) int {
	switch {
	case metric == "measure":
		return 0
	case strings.HasPrefix(metric, "Choreographer#doFrame"),
		strings.HasPrefix(metric, "DrawFrames"),
		strings.HasPrefix(metric, "draw-VRI"),
		strings.Contains(metric, "reportDrawFinished"),
		strings.Contains(metric, "queueBuffer"),
		metric == "traversal", metric == "layout":
		return 1
	default:
		return 2
	}
}

func describeRuntimeComparisonChoice(m types.TraceComparisonMeasurement) string {
	detail := fmt.Sprintf("%s %s tid=%d line=%d @%.6fs", m.ID, m.Metric, m.ThreadID, m.StartLine, float64(m.StartNS)/1e9)
	if m.Value != nil {
		detail += fmt.Sprintf("–%.6fs %.3f ms", float64(m.EndNS)/1e9, float64(*m.Value)/1e6)
	}
	return detail
}

// RenderRuntimeComparisonChoices preserves a bounded set of current, side-bound
// measurements in the required part of the Finalize prompt. It uses the same
// selector roster as the schema, and leaves business-role alignment to the model.
func RenderRuntimeComparisonChoices(ctx *types.AgentContext) string {
	catalog := types.BuildRuntimeComparisonCatalog(agentBusContextForAnswerPatchScope(ctx))
	return renderRuntimeComparisonChoices(catalog)
}

func renderRuntimeComparisonChoices(catalog *types.RuntimeComparisonCatalog) string {
	if catalog == nil {
		return ""
	}
	baseline := runtimeComparisonChoices(catalog.Baseline, false)
	current := runtimeComparisonChoices(catalog.Current, false)
	metricKey := func(m types.TraceComparisonMeasurement) [2]string { return [2]string{m.Metric, m.Caliber} }
	matchingChoices := func(side, other []types.TraceComparisonMeasurement) []string {
		keys := make(map[[2]string]bool, len(other))
		for _, m := range other {
			keys[metricKey(m)] = true
		}
		rows := make([]string, 0, 12)
		for _, m := range side {
			if keys[metricKey(m)] {
				rows = append(rows, "- "+describeRuntimeComparisonChoice(m))
				if len(rows) == cap(rows) {
					break
				}
			}
		}
		return rows
	}
	b, c := matchingChoices(baseline, current), matchingChoices(current, baseline)
	if len(b) == 0 || len(c) == 0 {
		return ""
	}
	return "## Bound Trace comparison measurements\n\n" +
		"For each reported quantitative difference supported by these measurements, put the matching baseline and current measurement_id in runtime_comparison on a section block. Preserve the measurement IDs and move the comparison to a section if a summary count repair is needed. " +
		"Use measurements of the same business phase within the relevant frame on each side. Matching names alone do not establish frame correspondence or causality. " +
		"The renderer computes the difference. For a frame comparison, if the submission interval lacks a verified endpoint or an admitted interval on either side, report insufficient evidence without a numeric delta. Use missing_metric alone when neither side can bind the interval.\n\n" +
		"### Baseline\n" + strings.Join(b, "\n") + "\n\n### Current\n" + strings.Join(c, "\n") + "\n\n"
}
