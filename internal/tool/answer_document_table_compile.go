package tool

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/types"
)

// compileCitationBackedTableRows repairs a common local-model table
// degradation: the model declares a multi-column table via columns[],
// but emits only label/text/citation_ref row carriers and leaves
// items[].cells empty. Rendering that shape as-is creates visibly empty
// trailing columns. The repair is intentionally display-only and
// citation-backed: it never invents answer members, never changes
// citation refs, and only fills row cells from the existing item label,
// item text, and cited file:line/quote.
func compileCitationBackedTableRows(doc *types.AnswerDocumentV2) int {
	if doc == nil {
		return 0
	}
	fixed := 0
	for bi := range doc.Blocks {
		block := &doc.Blocks[bi]
		if !shouldCompileCitationBackedTableRows(*block, doc) {
			continue
		}
		zh := answerTableCompilePrefersZH(block.Columns)
		block.Columns = citationBackedTableColumns(zh)
		for ii := range block.Items {
			item := &block.Items[ii]
			label := strings.TrimSpace(item.Label)
			if label == "" {
				continue
			}
			location, quote := citationBackedTableCitationSurface(doc, item.CitationRef)
			note := strings.TrimSpace(item.Text)
			if note == "" {
				note = quote
			}
			item.Cells = []string{location, note}
			fixed++
		}
	}
	return fixed
}

func shouldCompileCitationBackedTableRows(block types.AnswerBlock, doc *types.AnswerDocumentV2) bool {
	if block.Kind != types.BlockTable || strings.TrimSpace(block.Text) != "" || len(block.Columns) < 3 || len(block.Items) == 0 {
		return false
	}
	visible := 0
	compileCandidates := 0
	for _, item := range block.Items {
		label := strings.TrimSpace(item.Label)
		text := strings.TrimSpace(item.Text)
		cells := nonEmptyAnswerTableCompileCells(item.Cells)
		if label == "" && text == "" && len(cells) == 0 {
			continue
		}
		visible++
		if label == "" || len(cells) > 0 {
			return false
		}
		if item.CitationRef >= 0 && item.CitationRef < len(doc.Citations) {
			compileCandidates++
		}
	}
	return visible > 0 && compileCandidates == visible
}

func nonEmptyAnswerTableCompileCells(cells []string) []string {
	out := make([]string, 0, len(cells))
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			out = append(out, cell)
		}
	}
	return out
}

func answerTableCompilePrefersZH(values []string) bool {
	for _, value := range values {
		for _, r := range value {
			if unicode.Is(unicode.Han, r) {
				return true
			}
		}
	}
	return false
}

func citationBackedTableColumns(zh bool) []string {
	if zh {
		return []string{"符号名称", "定义位置", "说明"}
	}
	return []string{"Name", "Location", "Notes"}
}

func citationBackedTableCitationSurface(doc *types.AnswerDocumentV2, ref int) (location string, quote string) {
	if doc == nil || ref < 0 || ref >= len(doc.Citations) {
		return "", ""
	}
	cit := doc.Citations[ref]
	file := strings.TrimSpace(cit.File)
	if file != "" && cit.Line > 0 {
		location = fmt.Sprintf("%s:%d", file, cit.Line)
	} else {
		location = file
	}
	quote = strings.TrimSpace(cit.Quote)
	return location, quote
}
