package agent

import "strings"

// logicalFactSource derives a stable logical source locator from a tool result.
// It prefers repository-relative file paths parsed from tool summaries and falls
// back to a tool locator when no file path is available.
func logicalFactSource(r string, toolName string) string {
	first := strings.TrimSpace(strings.SplitN(r, "\n", 2)[0])
	if strings.HasPrefix(first, "[") && strings.Contains(first, "]") {
		bracket := first[1:strings.Index(first, "]")]
		if idx := strings.Index(bracket, ":"); idx > 0 {
			candidate := strings.TrimSpace(bracket[:idx])
			if looksLikePath(candidate) {
				return candidate
			}
		}
	}
	return "tool:" + toolName
}

func looksLikePath(s string) bool {
	if s == "" {
		return false
	}
	if strings.Contains(s, "/") || strings.Contains(s, ".") {
		return true
	}
	return false
}
