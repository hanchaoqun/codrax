package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// IsolateUnvalidatedDiagramFences recognizes Markdown code carriers, not claims
// or request keywords. Bare/text/directive fences that the existing Mermaid
// normalizer would upgrade are covered too; ordinary code stays byte-identical.
func IsolateUnvalidatedDiagramFences(text, language string) (string, int) {
	lines := strings.SplitAfter(text, "\n")
	var out strings.Builder
	count := 0
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
			out.WriteString(lines[i])
			i++
			continue
		}
		width := 0
		for width < len(line) && line[width] == line[0] {
			width++
		}
		if width < 3 {
			out.WriteString(lines[i])
			i++
			continue
		}
		info := strings.TrimSpace(line[width:])
		end := i + 1
		for ; end < len(lines); end++ {
			close := strings.TrimSpace(lines[end])
			if len(close) >= width && strings.Trim(close, string(line[0])) == "" {
				break
			}
		}
		body := strings.Join(lines[i+1:end], "")
		fields := strings.Fields(info)
		mermaid := len(fields) > 0 && strings.EqualFold(fields[0], "mermaid")
		mermaid = mermaid || mermaidcompat.InfoLineStartsWithKeyword(info) || ((info == "" || strings.EqualFold(info, "text")) && mermaidcompat.LooksLikeBody(body))
		if end < len(lines) {
			end++
		}
		if mermaid {
			out.WriteString(RecoveryDiagramIsolationNotice(language) + "\n")
			count++
		} else {
			out.WriteString(strings.Join(lines[i:end], ""))
		}
		i = end
	}
	return out.String(), count
}
