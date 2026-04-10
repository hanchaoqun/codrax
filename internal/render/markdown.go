package render

import (
	"regexp"
	"strings"
)

// renderMarkdown converts a markdown string into ANSI-styled terminal
// output. It handles headers, code blocks, inline code, bold, italic,
// lists, and horizontal rules. The implementation is intentionally
// simple — no AST, just line-by-line processing with minimal state.
func (s *style) renderMarkdown(text string) string {
	if !s.enabled {
		return text
	}

	lines := strings.Split(text, "\n")
	var out []string
	inCodeBlock := false
	codeLang := ""

	for _, line := range lines {
		// Fenced code blocks.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !inCodeBlock {
				inCodeBlock = true
				codeLang = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				header := "code"
				if codeLang != "" {
					header = codeLang
				}
				out = append(out, s.Dim("    ┌─ "+header+" ─"))
			} else {
				inCodeBlock = false
				codeLang = ""
				out = append(out, s.Dim("    └─"))
			}
			continue
		}
		if inCodeBlock {
			out = append(out, s.Dim("    │ ")+line)
			continue
		}

		// Headers.
		if strings.HasPrefix(line, "### ") {
			out = append(out, s.BoldCyan("   "+strings.TrimPrefix(line, "### ")))
			continue
		}
		if strings.HasPrefix(line, "## ") {
			out = append(out, s.BoldCyan("  "+strings.TrimPrefix(line, "## ")))
			continue
		}
		if strings.HasPrefix(line, "# ") {
			out = append(out, s.BoldCyan(" "+strings.TrimPrefix(line, "# ")))
			continue
		}

		// Horizontal rule.
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			out = append(out, s.Dim("    "+strings.Repeat("─", 40)))
			continue
		}

		// Unordered list items.
		if matched, indent, rest := matchListItem(line); matched {
			out = append(out, indent+"  "+s.Cyan("•")+" "+s.inlineMarkdown(rest))
			continue
		}

		// Ordered list items.
		if matched, indent, num, rest := matchOrderedItem(line); matched {
			out = append(out, indent+"  "+s.Cyan(num+".")+" "+s.inlineMarkdown(rest))
			continue
		}

		// Regular line with inline formatting.
		out = append(out, s.inlineMarkdown(line))
	}

	return strings.Join(out, "\n")
}

var (
	reInlineCode = regexp.MustCompile("`([^`]+)`")
	reBold       = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reBoldAlt    = regexp.MustCompile(`__([^_]+)__`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])\*([^*]+)\*(?:[^*]|$)`)
)

// inlineMarkdown applies inline formatting: bold, inline code.
func (s *style) inlineMarkdown(line string) string {
	// Inline code — apply first so code content isn't processed further.
	line = reInlineCode.ReplaceAllStringFunc(line, func(m string) string {
		inner := m[1 : len(m)-1]
		return s.apply(fgBrightYellow, inner)
	})
	// Bold.
	line = reBold.ReplaceAllStringFunc(line, func(m string) string {
		inner := m[2 : len(m)-2]
		return s.Bold(inner)
	})
	line = reBoldAlt.ReplaceAllStringFunc(line, func(m string) string {
		inner := m[2 : len(m)-2]
		return s.Bold(inner)
	})
	return line
}

// matchListItem checks if a line is an unordered list item (- or *).
func matchListItem(line string) (matched bool, indent, rest string) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 2 {
		return false, "", ""
	}
	if (trimmed[0] == '-' || trimmed[0] == '*') && trimmed[1] == ' ' {
		indent = line[:len(line)-len(trimmed)]
		return true, indent, trimmed[2:]
	}
	return false, "", ""
}

// matchOrderedItem checks if a line is an ordered list item (1. 2. etc).
func matchOrderedItem(line string) (matched bool, indent, num, rest string) {
	trimmed := strings.TrimLeft(line, " \t")
	indent = line[:len(line)-len(trimmed)]
	for i, ch := range trimmed {
		if ch >= '0' && ch <= '9' {
			continue
		}
		if ch == '.' && i > 0 && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
			return true, indent, trimmed[:i], trimmed[i+2:]
		}
		break
	}
	return false, "", "", ""
}
