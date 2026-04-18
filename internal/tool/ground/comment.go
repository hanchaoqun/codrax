package ground

import (
	"strings"

	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// isLineComment returns true when the given (1-based) line in fileLines
// is a pure comment line — no executable tokens of interest live there,
// only comment text. Used by Tier 1 to reject anchors whose LineStart
// points at a line that only MENTIONS the anchor_symbol inside a
// comment: the claim cites prose, not code. Falling through to Tier 2
// (symbol_table) and recovery tiers lets the grounder search for the
// real definition via repomap instead of rubber-stamping the comment.
//
// Language detection is via repomap/types.DetectLanguage on the source
// path. Unknown extensions fall back to the permissive union of all
// supported single-line markers — safer to err on "reject as comment"
// for Tier 1 (the recovery tiers have stricter signals anyway).
//
// Block comments (`/* ... */`, Python `""" ... """` / `''' ... '''`)
// are detected by walking backward through the contiguous suffix of
// fileLines immediately preceding `line`: if an opener occurred with
// no matching closer on an intervening line, the current line is
// inside the block. The walk halts at the first gap in fileLines
// (LineIndex is sparse per read_file output) which is conservative —
// a block opener that was never read cannot be inferred, so the check
// returns false in that case.
func isLineComment(fileLines map[int]string, line int, source string) bool {
	if fileLines == nil || line <= 0 {
		return false
	}
	raw, ok := fileLines[line]
	if !ok {
		return false
	}
	lang := repomaptypes.DetectLanguage(source)
	trimmed := strings.TrimSpace(raw)

	if isLineSoloComment(trimmed, lang) {
		return true
	}
	if isInsideBlockComment(fileLines, line, lang) {
		return true
	}
	return false
}

// isLineSoloComment recognises a line whose entire body is a comment.
// A line like `x := 1 // note` is NOT a solo comment (the anchor could
// reasonably live in the executable prefix), so this is intentionally
// strict on the prefix.
func isLineSoloComment(trimmed, lang string) bool {
	if trimmed == "" {
		return false
	}
	// C-family single-line: //
	if hasCFamilyLineComment(lang) && strings.HasPrefix(trimmed, "//") {
		return true
	}
	// Python / shell / YAML / Ruby / TOML: # at line start
	if hasHashLineComment(lang) && strings.HasPrefix(trimmed, "#") {
		return true
	}
	// C-family block comment continuation: `* text`, `*/`, or bare `*`
	if hasCFamilyLineComment(lang) {
		if trimmed == "*" || trimmed == "*/" || strings.HasPrefix(trimmed, "* ") {
			return true
		}
		// Opener on its own line counts as comment too.
		if strings.HasPrefix(trimmed, "/*") {
			return true
		}
	}
	// Python docstring openers/closers on their own line.
	if lang == repomaptypes.LangPython {
		if trimmed == `"""` || trimmed == `'''` ||
			strings.HasPrefix(trimmed, `"""`) && strings.HasSuffix(trimmed, `"""`) && len(trimmed) >= 6 ||
			strings.HasPrefix(trimmed, `'''`) && strings.HasSuffix(trimmed, `'''`) && len(trimmed) >= 6 {
			return true
		}
	}
	// Unknown language: permissive union — anything starting with a
	// recognised single-line marker counts.
	if lang == "" {
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
			strings.HasPrefix(trimmed, "/*") || trimmed == "*" || trimmed == "*/" ||
			strings.HasPrefix(trimmed, "* ") {
			return true
		}
	}
	return false
}

// isInsideBlockComment walks backward through the contiguous prefix of
// fileLines from line-1 looking for an unclosed block-comment opener.
// Walks at most maxWalk lines to cap cost. The walk only uses lines
// present in fileLines (LineIndex is sparse): a gap aborts the walk
// conservatively (returns false), because an opener above a gap cannot
// be verified.
func isInsideBlockComment(fileLines map[int]string, line int, lang string) bool {
	const maxWalk = 200

	// C-family `/* ... */`.
	if hasCFamilyBlockComment(lang) || lang == "" {
		if scanBlockBackward(fileLines, line, maxWalk, "/*", "*/") {
			return true
		}
	}
	// Python triple-quoted docstrings.
	if lang == repomaptypes.LangPython || lang == "" {
		if scanBlockBackward(fileLines, line, maxWalk, `"""`, `"""`) {
			return true
		}
		if scanBlockBackward(fileLines, line, maxWalk, `'''`, `'''`) {
			return true
		}
	}
	return false
}

// scanBlockBackward walks fileLines[line-1], line-2, ... looking for an
// unclosed `open` marker (no matching `close` between it and `line`).
// Returns true when an unclosed opener is found within maxWalk lines.
//
// When open == close (Python triple-quotes), the pairing is sequential:
// odd markers opened, even markers closed. Counting markers from the
// nearest contiguous prefix up to `line-1` — an odd cumulative count
// means `line` is inside an open block.
func scanBlockBackward(fileLines map[int]string, line, maxWalk int, open, close string) bool {
	if line <= 1 || maxWalk <= 0 {
		return false
	}
	// Identical delimiters: parity scan.
	if open == close {
		count := 0
		limit := line - 1
		if limit < line-maxWalk {
			limit = line - maxWalk
		}
		for i := line - 1; i >= limit && i > 0; i-- {
			text, ok := fileLines[i]
			if !ok {
				return false // gap — cannot infer
			}
			count += strings.Count(text, open)
		}
		return count%2 == 1
	}
	// Distinct delimiters: walk back, count unmatched openers.
	limit := line - 1
	if limit < line-maxWalk {
		limit = line - maxWalk
	}
	unclosed := 0
	for i := line - 1; i >= limit && i > 0; i-- {
		text, ok := fileLines[i]
		if !ok {
			return false // gap — cannot infer
		}
		// Process right-to-left so closers at the end of a line close
		// openers on the same line before those openers count as new.
		closers := strings.Count(text, close)
		openers := strings.Count(text, open)
		// openers - closers on a given line tells net contribution.
		// Positive = more opens than closes below = an open propagates up.
		net := openers - closers
		unclosed += net
		if unclosed > 0 {
			return true
		}
		// Negative net (more closes than opens on this line) means a
		// block CLOSED somewhere on this line — if the outer walk
		// hadn't already balanced it, we'd have exited with
		// unclosed>0 on a prior iteration. Continue walking to find
		// the earlier opener.
	}
	return false
}

// hasCFamilyLineComment reports whether the language uses `//` as a
// single-line comment marker.
func hasCFamilyLineComment(lang string) bool {
	switch lang {
	case repomaptypes.LangGo, repomaptypes.LangJavaScript, repomaptypes.LangTypeScript,
		repomaptypes.LangJava, repomaptypes.LangRust,
		repomaptypes.LangC, repomaptypes.LangCpp:
		return true
	}
	return false
}

// hasCFamilyBlockComment reports whether the language supports `/* ... */`.
func hasCFamilyBlockComment(lang string) bool {
	return hasCFamilyLineComment(lang)
}

// hasHashLineComment reports whether the language uses `#` at the
// start of a line as a single-line comment marker.
func hasHashLineComment(lang string) bool {
	switch lang {
	case repomaptypes.LangPython:
		return true
	}
	// Unknown extensions (shell, YAML, TOML, Ruby, Makefile, etc.) fall
	// through to the empty-lang branch in isLineSoloComment which
	// permissively recognises `#`.
	return false
}
