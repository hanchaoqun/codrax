package patcher

import (
	"fmt"
	"strings"
)

// generateDiff produces a compact unified-diff-style preview showing the
// changed region plus the given number of context lines on each side. Input
// is expected to be \n-normalized. No external dependency is used: we trim
// common prefix and suffix lines and print the remaining window.
func generateDiff(before, after string, contextLines int) string {
	if before == after {
		return ""
	}

	beforeLines := splitKeepEmpty(before)
	afterLines := splitKeepEmpty(after)

	prefix := 0
	for prefix < len(beforeLines) && prefix < len(afterLines) &&
		beforeLines[prefix] == afterLines[prefix] {
		prefix++
	}

	suffix := 0
	for suffix < len(beforeLines)-prefix && suffix < len(afterLines)-prefix &&
		beforeLines[len(beforeLines)-1-suffix] == afterLines[len(afterLines)-1-suffix] {
		suffix++
	}

	if contextLines < 0 {
		contextLines = 0
	}
	ctxStart := prefix - contextLines
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEndBefore := len(beforeLines) - suffix + contextLines
	if ctxEndBefore > len(beforeLines) {
		ctxEndBefore = len(beforeLines)
	}

	var b strings.Builder

	// Leading context (from before, identical in after).
	for i := ctxStart; i < prefix; i++ {
		fmt.Fprintf(&b, "  %s\n", beforeLines[i])
	}
	// Removed lines.
	for i := prefix; i < len(beforeLines)-suffix; i++ {
		fmt.Fprintf(&b, "- %s\n", beforeLines[i])
	}
	// Added lines.
	for i := prefix; i < len(afterLines)-suffix; i++ {
		fmt.Fprintf(&b, "+ %s\n", afterLines[i])
	}
	// Trailing context.
	for i := len(beforeLines) - suffix; i < ctxEndBefore; i++ {
		fmt.Fprintf(&b, "  %s\n", beforeLines[i])
	}

	return b.String()
}

// splitKeepEmpty splits text on "\n" without swallowing the final empty
// segment, so adding or removing a trailing newline shows up in the diff.
func splitKeepEmpty(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}
