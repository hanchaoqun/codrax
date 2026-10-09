package render

import "strings"

// protectMermaidLiteralLabelNewlines fills a terminal-library gap before its
// parser runs: both Render and the flow parser replace every backslash-n, even
// the second slash of an escaped literal backslash. An authored literal \n is
// two display cells, so reuse the CJK adapter's collision-safe two-byte pool.
// Ordinary quoted escapes and persisted Mermaid source are not changed.
func protectMermaidLiteralLabelNewlines(body string, adapter *cjkAdapter) (string, map[string]string, error) {
	if !strings.Contains(body, `\\n`) {
		return body, nil, nil
	}
	var b strings.Builder
	b.Grow(len(body))
	quoted := false
	restore := make(map[string]string)
	for i := 0; i < len(body); {
		if body[i] == '\r' || body[i] == '\n' {
			quoted = false
		}
		if body[i] == '"' {
			quoted = !quoted
		}
		if quoted && body[i] == '\\' && i+1 < len(body) {
			if strings.HasPrefix(body[i:], `\\n`) {
				placeholder, err := adapter.allocatePlaceholder()
				if err != nil {
					return body, nil, err
				}
				restore[placeholder] = `\n`
				b.WriteString(placeholder)
				i += len(`\\n`)
				continue
			}
			b.WriteString(body[i : i+2])
			i += 2
			continue
		}
		b.WriteByte(body[i])
		i++
	}
	return b.String(), restore, nil
}
