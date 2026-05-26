package types

import (
	"strings"
	"unicode"
)

const DefaultModelSurfaceDraftPromptRunes = 12000

// LooksLikeModelAuthoredVisibleSurfaceDraft detects rich, user-visible answer
// surfaces by presentation structure, not by user-question terms or answer
// semantics. It is used only to preserve model-authored tables, diagrams, and
// organized drafts as advisory/fallback surfaces; it must not create factual
// gates or rewrite the model's answer.
func LooksLikeModelAuthoredVisibleSurfaceDraft(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" || IsPlaceholderLikeModelDraft(content) {
		return false
	}
	if modelSurfaceHasMarkdownTable(content) ||
		modelSurfaceHasFence(content) ||
		modelSurfaceHasBoxDrawing(content) {
		return true
	}
	return modelSurfaceListLineCount(content) >= 5 && len([]rune(content)) >= 500
}

// IsPlaceholderLikeModelDraft detects internal placeholder/scaffolding tokens
// that should never outrank an earlier rich draft or verified evidence.
func IsPlaceholderLikeModelDraft(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	lower := strings.ToLower(content)
	return strings.Contains(lower, "placeholder_reasoning_placeholder") ||
		strings.Contains(lower, "{{placeholder") ||
		strings.Contains(lower, "reasoning_placeholder")
}

// TrimModelAuthoredVisibleSurfaceDraft bounds a preserved model surface for
// prompts/fallbacks without altering short tables or diagrams. Empty maxRunes
// means DefaultModelSurfaceDraftPromptRunes.
func TrimModelAuthoredVisibleSurfaceDraft(content string, maxRunes int) string {
	content = strings.TrimSpace(content)
	if content == "" || IsPlaceholderLikeModelDraft(content) {
		return ""
	}
	if maxRunes <= 0 {
		maxRunes = DefaultModelSurfaceDraftPromptRunes
	}
	runes := []rune(content)
	if len(runes) <= maxRunes {
		return content
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "\n\n[model-authored draft clipped for prompt budget]"
}

func modelSurfaceHasFence(content string) bool {
	return strings.Contains(content, "```")
}

func modelSurfaceHasBoxDrawing(content string) bool {
	for _, r := range content {
		switch r {
		case '┌', '┐', '└', '┘', '│', '─', '├', '┤', '┬', '┴', '┼', '╭', '╮', '╰', '╯', '═', '║':
			return true
		}
	}
	return false
}

func modelSurfaceHasMarkdownTable(content string) bool {
	lines := strings.Split(content, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if !modelSurfaceLooksLikeTableRow(lines[i]) {
			continue
		}
		if modelSurfaceLooksLikeTableSeparator(lines[i+1]) {
			return true
		}
	}
	return false
}

func modelSurfaceLooksLikeTableRow(line string) bool {
	line = strings.TrimSpace(line)
	return strings.Count(line, "|") >= 2
}

func modelSurfaceLooksLikeTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if strings.Count(line, "|") < 2 {
		return false
	}
	for _, r := range line {
		switch r {
		case '|', ':', '-', ' ', '\t':
			continue
		default:
			return false
		}
	}
	return strings.Contains(line, "---")
}

func modelSurfaceListLineCount(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if modelSurfaceLooksLikeListLine(strings.TrimSpace(line)) {
			count++
		}
	}
	return count
}

func modelSurfaceLooksLikeListLine(line string) bool {
	if strings.HasPrefix(line, "- ") ||
		strings.HasPrefix(line, "* ") ||
		strings.HasPrefix(line, "+ ") {
		return true
	}
	digitRunes := 0
	for _, r := range line {
		if unicode.IsDigit(r) {
			digitRunes++
			continue
		}
		return digitRunes > 0 && r == '.'
	}
	return false
}
