package render

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The accepted document owns a deep-copied table, so rendering never reads a
// mutable artifact, parses prose, recalculates a value, or chooses a member.
func renderV2RuntimeMeasurementTable(b *strings.Builder, block types.AnswerBlock, lang answerDocLang) {
	renderV2RuntimeMeasurementData(b, block, lang, nativeMeasurementDataPlacement{})
}

func renderV2RuntimeMeasurementData(b *strings.Builder, block types.AnswerBlock, lang answerDocLang, placement nativeMeasurementDataPlacement) {
	if block.RuntimeMeasurement == nil || !block.RuntimeMeasurement.IsBound() {
		if lang == answerDocLangZH {
			b.WriteString("测量表未绑定到当前证据，无法展示可信数值。\n\n")
		} else {
			b.WriteString("The measurement table is not bound to current evidence; trusted values are unavailable.\n\n")
		}
		return
	}
	table := block.RuntimeMeasurement.BoundTable
	sourceTitle, sourceNotes, owned := runtimeMeasurementSourcePresentation(table, lang)
	if owned {
		renderV2ListOrTableHeading(b, block, measurementPlainText(sourceTitle))
		for _, note := range sourceNotes {
			b.WriteString(measurementPlainText(note) + "\n\n")
		}
	} else if title := strings.TrimSpace(block.Title); title != "" {
		renderV2ListOrTableHeading(b, block, title)
	}
	if !owned && table.Label != "" {
		b.WriteString(measurementPlainText(table.Label))
		b.WriteString("\n\n")
	}
	writeRow := func(cells []string) {
		b.WriteString("|")
		for _, cell := range cells {
			fmt.Fprintf(b, " %s |", measurementPlainText(cell))
		}
		b.WriteByte('\n')
	}
	if placement.reference {
		if lang == answerDocLangZH {
			fmt.Fprintf(b, "数据行见上方**数据表 %d**。这里只复用显示；本次查询的来源、范围和下列说明仍独立保留，不合并总体或证据权限。\n", placement.number)
		} else {
			fmt.Fprintf(b, "Data rows are shown in **Data table %d** above. Only their display is reused; this query retains its own source, scope and notes below, without merging populations or evidence rights.\n", placement.number)
		}
	} else {
		if placement.number > 0 {
			if lang == answerDocLangZH {
				fmt.Fprintf(b, "**数据表 %d**\n\n", placement.number)
			} else {
				fmt.Fprintf(b, "**Data table %d**\n\n", placement.number)
			}
		}
		writeRow(table.Columns)
		separators := make([]string, len(table.Columns))
		for i := range separators {
			separators[i] = "---"
		}
		writeRow(separators)
		for _, row := range table.Rows {
			writeRow(row)
		}
	}
	b.WriteByte('\n')
	for _, note := range table.Notes {
		b.WriteString(measurementPlainText(note))
		b.WriteString("\n\n")
	}
}

func runtimeMeasurementSourcePresentation(table *types.RuntimeMeasurementTable, lang answerDocLang) (string, []string, bool) {
	language := "en"
	if lang == answerDocLangZH {
		language = "zh"
	}
	return table.NativeSourcePresentation(language)
}

// Measurement labels may originate in trace data. Keep them as literal table
// cells rather than admitting Markdown links, HTML, or extra rows as formatting.
func measurementPlainText(text string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", "|", "&#124;",
		"\\", "&#92;", "`", "&#96;", "*", "&#42;", "_", "&#95;",
		"[", "&#91;", "]", "&#93;", "\r", "&#13;", "\n", "&#10;",
	).Replace(text)
}
