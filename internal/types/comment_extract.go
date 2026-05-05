package types

import (
	"path/filepath"
	"strings"
)

// comment_extract.go — Plan 2 v2 (2026-05-05). Deterministic
// leading-doc-comment extractor used by emit_evidence to pair every
// definition / registration anchor with a parallel
// evidence_kind=mechanism evidence whose summary captures the
// component's documented role/purpose.
//
// Why deterministic: the LLM-driven Plan 2 (skill prompt) only
// pairs sometimes; this hook guarantees role-description evidence
// reaches the pool whenever the source carries a leading doc
// comment, regardless of LLM compliance.
//
// Cross-language coverage uses standard SE comment vocabularies
// (// , #, /** */, """ ... """) — universal industry convention,
// not eval-fitted keywords. Languages without coverage degrade to
// no-op (LLM still has the prompt path).
//
// Length filter:
//   - max 6 output lines, 300 output chars
//   - reject input blocks > 12 lines or > 800 chars (typical
//     license / multi-paragraph docs — unlikely to be a focused
//     role description). Structural threshold, no keyword match.

// commentStyle classifies a file extension into a comment-syntax
// family. Pure structural mapping; no language-specific behaviour
// beyond "what does a comment line look like".
type commentStyle int

const (
	commentStyleNone         commentStyle = iota
	commentStyleDoubleSlash               // // and ///
	commentStyleHash                      // #
	commentStyleBlock                     // /** ... */ (Java, JSDoc)
	commentStyleHTML                      // <!-- ... -->
)

// extCommentStyle returns the comment style for a file extension.
// Python (.py / .pyi) reports styleHash so line-level # fallback
// works; its docstring path is handled separately in
// ExtractLeadingDocComment via isPython.
func extCommentStyle(ext string) commentStyle {
	switch strings.ToLower(ext) {
	case ".go", ".rs", ".swift", ".kt", ".kts", ".scala",
		".js", ".jsx", ".mjs", ".cjs",
		".ts", ".tsx", ".ets",
		".c", ".h", ".cc", ".cpp", ".cxx", ".hpp", ".hh",
		".m", ".mm", ".cu", ".cuh",
		".cj":
		return commentStyleDoubleSlash
	case ".java":
		// Java prefers Javadoc /** ... */ blocks; fall back to // when
		// block missing. Block extractor handles either path.
		return commentStyleBlock
	case ".py", ".pyi", ".rb", ".sh", ".bash", ".zsh",
		".yaml", ".yml", ".toml", ".ini", ".cfg",
		".pl", ".pm", ".tf":
		return commentStyleHash
	case ".html", ".htm", ".xml", ".md", ".vue":
		return commentStyleHTML
	}
	return commentStyleNone
}

// isPython reports whether the extension is a Python source file —
// triggers docstring extraction first, line-comment fallback second.
func isPython(ext string) bool {
	switch strings.ToLower(ext) {
	case ".py", ".pyi":
		return true
	}
	return false
}

const (
	// maxOutChars / maxOutLines bound the returned doc-comment text.
	// Chosen to fit in one prompt section without crowding the
	// downstream finalizer's evidence rendering. Trimmed at rune
	// boundary.
	maxOutChars = 300
	maxOutLines = 6

	// Input-side rejection thresholds. Doc comments above 12 lines
	// or 800 chars are typically license headers / multi-paragraph
	// API docs — unlikely to be a focused role description and
	// would poison evidence pool. Structural threshold (no keyword
	// check).
	maxInLines = 12
	maxInChars = 800
)

// ExtractLeadingDocComment scans source bytes to find the
// documentation comment associated with the definition / registration
// at lineStart (1-based). Returns (text, startLine) where text is
// the head of the doc comment trimmed to maxOutChars / maxOutLines,
// and startLine is the 1-based line number of the comment's first
// line. Empty text + 0 means no comment found / rejected by length
// filter / unsupported language.
//
// Algorithm:
//
//  1. For Python: try docstring on the line(s) after lineStart
//     (function body's first statement). If found, return.
//  2. Otherwise, walk UPWARD from lineStart-1 through any blank
//     line (single blank tolerated to allow "decorator over def"
//     idiom; multi-blank stops the walk).
//  3. Collect contiguous comment lines matching the file's comment
//     style. Stop at first non-comment line.
//  4. Reverse to reading order, strip prefixes, apply length filter.
func ExtractLeadingDocComment(content []byte, lineStart int, path string) (text string, startLine int) {
	if lineStart <= 0 || len(content) == 0 {
		return "", 0
	}
	lines := strings.Split(string(content), "\n")
	if lineStart > len(lines) {
		return "", 0
	}
	ext := strings.ToLower(filepath.Ext(path))

	if isPython(ext) {
		if t, l := extractPythonDocstring(lines, lineStart); t != "" {
			return t, l
		}
		// fall through to # comment fallback below
	}

	style := extCommentStyle(ext)
	switch style {
	case commentStyleDoubleSlash:
		return extractAboveLinePrefix(lines, lineStart, "//")
	case commentStyleHash:
		return extractAboveLinePrefix(lines, lineStart, "#")
	case commentStyleBlock:
		// Java: try /** ... */ first; fall back to // if block absent.
		if t, l := extractBlockCommentAbove(lines, lineStart); t != "" {
			return t, l
		}
		return extractAboveLinePrefix(lines, lineStart, "//")
	case commentStyleHTML:
		return extractHTMLBlockAbove(lines, lineStart)
	}
	return "", 0
}

// extractAboveLinePrefix walks upward from the line above lineStart
// collecting contiguous lines whose trimmed-left form starts with
// prefix. A single blank line is tolerated (skipped); a second
// blank or a non-comment line ends the walk.
func extractAboveLinePrefix(lines []string, lineStart int, prefix string) (string, int) {
	idx := lineStart - 2 // 0-based line above the definition
	if idx < 0 {
		return "", 0
	}
	collected := make([]string, 0, maxInLines+1)
	blankBudget := 1
	firstLine := 0
	for ; idx >= 0; idx-- {
		raw := lines[idx]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			if blankBudget == 0 {
				break
			}
			blankBudget--
			continue
		}
		if !strings.HasPrefix(trimmed, prefix) {
			break
		}
		collected = append(collected, raw)
		firstLine = idx + 1 // convert back to 1-based
		// reset blank budget after seeing a comment line so
		// "comment / blank / comment" counts as one block via the
		// budget allowance.
		blankBudget = 1
	}
	if len(collected) == 0 {
		return "", 0
	}
	if len(collected) > maxInLines {
		return "", 0
	}
	// Reverse to reading order.
	for i, j := 0, len(collected)-1; i < j; i, j = i+1, j-1 {
		collected[i], collected[j] = collected[j], collected[i]
	}
	return finalizeCommentBlock(collected, prefix, firstLine)
}

// extractBlockCommentAbove parses a /** ... */ block (Javadoc /
// JSDoc style) immediately above lineStart. Returns the inner
// content with leading * stripped.
func extractBlockCommentAbove(lines []string, lineStart int) (string, int) {
	idx := lineStart - 2
	// Skip up to 1 blank line.
	if idx >= 0 && strings.TrimSpace(lines[idx]) == "" {
		idx--
	}
	if idx < 0 || !strings.HasSuffix(strings.TrimSpace(lines[idx]), "*/") {
		return "", 0
	}
	// Walk upward to find /**.
	end := idx
	start := -1
	for j := end; j >= 0; j-- {
		if strings.HasPrefix(strings.TrimSpace(lines[j]), "/**") ||
			strings.HasPrefix(strings.TrimSpace(lines[j]), "/*") {
			start = j
			break
		}
		if end-j+1 > maxInLines {
			return "", 0
		}
	}
	if start < 0 {
		return "", 0
	}
	collected := make([]string, 0, end-start+1)
	for j := start; j <= end; j++ {
		collected = append(collected, lines[j])
	}
	return finalizeBlockComment(collected, start+1)
}

// extractHTMLBlockAbove parses a <!-- ... --> block above lineStart.
func extractHTMLBlockAbove(lines []string, lineStart int) (string, int) {
	idx := lineStart - 2
	if idx >= 0 && strings.TrimSpace(lines[idx]) == "" {
		idx--
	}
	if idx < 0 || !strings.HasSuffix(strings.TrimSpace(lines[idx]), "-->") {
		return "", 0
	}
	end := idx
	start := -1
	for j := end; j >= 0; j-- {
		if strings.Contains(lines[j], "<!--") {
			start = j
			break
		}
		if end-j+1 > maxInLines {
			return "", 0
		}
	}
	if start < 0 {
		return "", 0
	}
	collected := make([]string, 0, end-start+1)
	for j := start; j <= end; j++ {
		collected = append(collected, lines[j])
	}
	joined := strings.Join(collected, " ")
	joined = strings.ReplaceAll(joined, "<!--", "")
	joined = strings.ReplaceAll(joined, "-->", "")
	joined = strings.TrimSpace(joined)
	if joined == "" {
		return "", 0
	}
	return clipText(joined), start + 1
}

// extractPythonDocstring attempts to read a Python triple-quoted
// docstring on the line(s) immediately following lineStart. Handles
// single-line ("""text""") and multi-line forms. Returns ("", 0)
// when no docstring or when the body's first statement is not a
// string.
func extractPythonDocstring(lines []string, lineStart int) (string, int) {
	// Body starts at lineStart (the def line) + 1.
	idx := lineStart // 0-based index of the line after def
	if idx >= len(lines) {
		return "", 0
	}
	// Scan forward through blank / decorator / continuation up to
	// 4 lines to find first non-empty statement.
	for skipped := 0; idx < len(lines) && skipped < 4; idx++ {
		raw := strings.TrimSpace(lines[idx])
		if raw == "" {
			skipped++
			continue
		}
		// Detect triple-quote start.
		quote := ""
		switch {
		case strings.HasPrefix(raw, `"""`):
			quote = `"""`
		case strings.HasPrefix(raw, `'''`):
			quote = `'''`
		default:
			return "", 0
		}
		stripped := strings.TrimPrefix(raw, quote)
		// Single-line: `"""text"""`.
		if endIdx := strings.Index(stripped, quote); endIdx >= 0 {
			text := strings.TrimSpace(stripped[:endIdx])
			if text == "" {
				return "", 0
			}
			return clipText(text), idx + 1
		}
		// Multi-line: collect up to maxInLines.
		collected := []string{stripped}
		for j := idx + 1; j < len(lines); j++ {
			line := lines[j]
			if k := strings.Index(line, quote); k >= 0 {
				collected = append(collected, line[:k])
				break
			}
			collected = append(collected, line)
			if len(collected) > maxInLines {
				return "", 0
			}
		}
		joined := strings.TrimSpace(strings.Join(collected, " "))
		joined = strings.Join(strings.Fields(joined), " ")
		if joined == "" {
			return "", 0
		}
		return clipText(joined), idx + 1
	}
	return "", 0
}

// finalizeCommentBlock strips prefix from each line, joins, applies
// length filter and trims.
func finalizeCommentBlock(reversed []string, prefix string, firstLine int) (string, int) {
	parts := make([]string, 0, len(reversed))
	totalChars := 0
	for _, raw := range reversed {
		l := strings.TrimSpace(raw)
		l = strings.TrimPrefix(l, prefix)
		// Some languages use //!  or /// for doc-comment marker
		// (Rust). Strip the extra char.
		l = strings.TrimPrefix(l, "/")
		l = strings.TrimPrefix(l, "!")
		l = strings.TrimSpace(l)
		parts = append(parts, l)
		totalChars += len(l) + 1 // +1 for join separator
		if totalChars > maxInChars {
			return "", 0
		}
	}
	if len(parts) > maxOutLines {
		parts = parts[:maxOutLines]
	}
	joined := strings.TrimSpace(strings.Join(parts, " "))
	joined = strings.Join(strings.Fields(joined), " ")
	if joined == "" {
		return "", 0
	}
	return clipText(joined), firstLine
}

// finalizeBlockComment strips /** */ markers and intermediate * from
// a Javadoc-style block.
func finalizeBlockComment(blockLines []string, firstLine int) (string, int) {
	parts := make([]string, 0, len(blockLines))
	totalChars := 0
	for _, raw := range blockLines {
		l := strings.TrimSpace(raw)
		l = strings.TrimPrefix(l, "/**")
		l = strings.TrimPrefix(l, "/*")
		l = strings.TrimSuffix(l, "*/")
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "*")
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		parts = append(parts, l)
		totalChars += len(l) + 1
		if totalChars > maxInChars {
			return "", 0
		}
	}
	if len(parts) == 0 {
		return "", 0
	}
	joined := strings.TrimSpace(strings.Join(parts, " "))
	joined = strings.Join(strings.Fields(joined), " ")
	return clipText(joined), firstLine
}

// clipText trims a fully-cleaned doc-comment string to maxOutChars,
// breaking on a rune boundary and (when possible) on a sentence /
// word boundary.
func clipText(s string) string {
	r := []rune(s)
	if len(r) <= maxOutChars {
		return s
	}
	cut := r[:maxOutChars]
	// Try to break at the last sentence terminator.
	for i := len(cut) - 1; i >= maxOutChars/2; i-- {
		switch cut[i] {
		case '.', '。', '!', '?', '\n':
			return strings.TrimSpace(string(cut[:i+1]))
		}
	}
	// Else break on space.
	for i := len(cut) - 1; i >= maxOutChars/2; i-- {
		if cut[i] == ' ' {
			return strings.TrimSpace(string(cut[:i])) + "…"
		}
	}
	return strings.TrimSpace(string(cut)) + "…"
}
