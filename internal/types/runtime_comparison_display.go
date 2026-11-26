package types

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var frameMeasurementSentenceEnd = regexp.MustCompile(`[。！？\n]|[.!?](?:[ \t]+|$)`)

// reconcileFrameMeasurementDisplay works on the rendered snapshot. Only replace
// a relayoutWindow attribution contradicted by the bound baseline measurement.
// Discard dependent wording in that sentence rather than relabeling it as if
// measure were a window transaction or a submission interval.
func reconcileFrameMeasurementDisplay(doc *AnswerDocumentV2, measure TraceComparisonMeasurement, frame *TraceComparisonMeasurement, baseline map[string]TraceComparisonMeasurement, language string) {
	const alias = "relayoutWindow"
	duration := fmt.Sprintf("%.3f", float64(*measure.Value)/1e6)
	start := fmt.Sprintf("%.6f", float64(measure.StartNS)/1e9)
	end := fmt.Sprintf("%.6f", float64(measure.EndNS)/1e9)
	valuePattern := regexp.MustCompile(`(?:^|[^0-9.])` + regexp.QuoteMeta(duration) + `[ \t]*ms\b`)
	// Text without timestamps is only attributable when the baseline side
	// is explicit and its observed relayout duration rules out this value.
	var distinctRelayout bool
	for _, m := range baseline {
		if !strings.Contains(m.Metric, alias) || m.Caliber != "closed_sync_span" ||
			m.ThreadID != measure.ThreadID || m.Source != measure.Source || m.Generation != measure.Generation ||
			m.StartNS < frame.StartNS || m.EndNS > frame.EndNS {
			continue
		}
		if fmt.Sprintf("%.3f", float64(*m.Value)/1e6) == duration {
			distinctRelayout = false
			break
		}
		distinctRelayout = true
	}
	conflicts := func(s string, baselineProse bool) bool {
		if !strings.Contains(s, alias) || strings.Contains(s, measure.Metric) || !valuePattern.MatchString(s) {
			return false
		}
		if strings.Contains(s, start) && strings.Contains(s, end) {
			return true
		}
		_, afterAlias, _ := strings.Cut(s, alias)
		return baselineProse && distinctRelayout && valuePattern.MatchString(afterAlias) &&
			(strings.Contains(s, "基线") || strings.Contains(strings.ToLower(s), "baseline"))
	}
	chinese := strings.HasPrefix(strings.ToLower(language), "zh")
	correction := "The baseline `measure` span took " + duration + " ms."
	if chinese {
		correction = "基线帧的 `measure` 区间耗时 " + duration + " ms。"
	}
	for i := range doc.Blocks {
		block := &doc.Blocks[i]
		if block.Kind == BlockTable {
			block.Items = slices.DeleteFunc(slices.Clone(block.Items), func(item AnswerBlockItem) bool {
				return len(item.Cells) > 1 && strings.Contains(item.Cells[0], alias) &&
					conflicts(strings.Join(item.Cells, " "), false)
			})
		}
		if !strings.Contains(block.Text, alias) {
			continue
		}
		var text strings.Builder
		from := 0
		replaced := false
		for _, at := range frameMeasurementSentenceEnd.FindAllStringIndex(block.Text, -1) {
			sentence := block.Text[from:at[1]]
			if conflicts(sentence, true) {
				if !replaced {
					text.WriteString(correction)
					delimiter := block.Text[at[0]:at[1]]
					if delimiter == "\n" {
						text.WriteString(delimiter)
					} else if len(delimiter) > 1 && delimiter[0] == '.' {
						text.WriteString(delimiter[1:])
					}
					replaced = true
				}
			} else {
				text.WriteString(sentence)
			}
			from = at[1]
		}
		if last := block.Text[from:]; conflicts(last, true) {
			if !replaced {
				text.WriteString(correction)
			}
		} else {
			text.WriteString(last)
		}
		block.Text = text.String()
		if block.RuntimeComparison != nil && block.RuntimeComparison.Bound != nil {
			block.RuntimeComparison.Bound.Explanation = block.Text
		}
	}
}
