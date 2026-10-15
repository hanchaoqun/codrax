package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// RenderRuntimeEventMeasurementBlock shares the native fact surface with
// reviewers. A handled but stale selection returns a warning, not model prose.
func RenderRuntimeEventMeasurementBlock(block types.AnswerBlock, language string) (string, bool) {
	var b strings.Builder
	handled := renderV2RuntimeEventMeasurement(&b, block, normalizeAnswerDocLang(language))
	return b.String(), handled
}

func renderV2RuntimeEventMeasurement(b *strings.Builder, block types.AnswerBlock, lang answerDocLang) bool {
	receipt := block.RuntimeMeasurement
	if receipt == nil || (receipt.EventSelection == nil && (receipt.BoundTable == nil || !receipt.BoundTable.HasNativeSamplingFacts())) || (block.Kind != types.BlockSummary && block.Kind != types.BlockDiagram) {
		return false
	}
	if !receipt.IsBound() || (receipt.BoundTable.EventData == nil && !receipt.BoundTable.HasNativeSamplingFacts()) {
		if lang == answerDocLangZH {
			b.WriteString("事件选集未绑定到当前证据，无法展示可信字段。\n\n")
		} else {
			b.WriteString("The event selection is not bound to current evidence; trusted fields are unavailable.\n\n")
		}
		return true
	}
	table := receipt.BoundTable
	if title := strings.TrimSpace(block.Title); title != "" {
		fmt.Fprintf(b, "**%s**\n\n", measurementPlainText(title))
	}
	if table.Label != "" {
		fmt.Fprintf(b, "%s\n\n", measurementPlainText(table.Label))
	}
	if len(table.Rows) == 0 {
		if lang == answerDocLangZH {
			b.WriteString("该选集没有保留事件记录。字段：")
		} else {
			b.WriteString("No event records are retained in this selection. Fields: ")
		}
		for i, column := range table.Columns {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(measurementPlainText(column))
		}
		b.WriteString("\n\n")
	} else if block.Kind == types.BlockDiagram {
		renderRuntimeEventFactDiagram(b, table, lang)
	} else {
		for _, row := range table.Rows {
			b.WriteString("- ")
			b.WriteString(strings.Join(runtimeEventDisplayCells(table.Columns, row), "; "))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	for _, note := range table.Notes {
		fmt.Fprintf(b, "%s\n\n", measurementPlainText(note))
	}
	return true
}

func runtimeEventDisplayCells(columns, row []string) []string {
	cells := make([]string, len(columns))
	for i, column := range columns {
		cells[i] = measurementPlainText(column) + ": " + measurementPlainText(row[i])
	}
	return cells
}

// Each selected native row/group is one disconnected fact node. Position is
// layout only: no event order, correlation or causal edge is inferred here.
func renderRuntimeEventFactDiagram(b *strings.Builder, table *types.RuntimeMeasurementTable, lang answerDocLang) {
	var body strings.Builder
	body.WriteString("flowchart TB\n")
	var literalRows []string
	var labelErr error
	for i, row := range table.Rows {
		cells := runtimeEventDisplayCells(table.Columns, row)
		literalRows = append(literalRows, strings.Join(cells, "; "))
		label, err := mermaidcompat.NormalizeExplicitNodeLabel(strings.Join(cells, "<br/>"))
		if err != nil {
			labelErr = err
		}
		fmt.Fprintf(&body, "    N%d[\"%s\"]\n", i+1, label)
	}
	if labelErr != nil {
		reason := "Event labels could not be safely encoded; all selected facts are preserved as text"
		if lang == answerDocLangZH {
			reason = "事件字段无法安全编码为图；全部选中事实已保留为文本"
		}
		b.WriteString(mermaidFailureFence(strings.Join(literalRows, "\n"), reason))
		b.WriteString("\n\n")
		return
	}
	fmt.Fprintf(b, "```mermaid\n%s```\n\n", body.String())
	if lang == answerDocLangZH {
		b.WriteString("每个方框是一条保留记录或一个分组；排列不表示时序、依赖或因果关系。\n\n")
	} else {
		b.WriteString("Each box is one retained record or group; placement does not imply chronology, dependency or causality.\n\n")
	}
}
