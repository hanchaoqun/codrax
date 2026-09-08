package ground

import (
	"strings"

	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	codraxtypes "github.com/hanchaoqun/codrax/internal/types"
)

// isLineComment returns true when the given (1-based) line in fileLines
// is a pure comment line — no executable tokens of interest live there,
// only comment text. Used by Tier 1 to reject anchors whose LineStart
// points at a line that only MENTIONS the anchor_symbol inside a
// comment: the claim cites prose, not code. Falling through to Tier 2
// (symbol_table) and recovery tiers lets the grounder search for the
// real definition via repomap instead of rubber-stamping the comment.
//
// Multiline classification consumes only an observed, contiguous lexical
// prefix beginning at physical line 1, within the bounded walk. A missing
// prefix or a walk cap is not a synthetic start-of-file. In that case only
// the existing standalone line forms remain available. False means no
// comment-only proof, not proof of executable code; grounding's other
// checks still own source/call/definition authority.
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
	if comment, known := observedCommentLine(fileLines, line, source, lang); known {
		return comment
	}
	if isConfigSoloComment(trimmed, source) {
		return true
	}
	if isLineSoloComment(trimmed, lang) {
		return true
	}
	return false
}

// LineLooksCommentOnly exposes the comment-only detector to callers
// outside the ground package that need to distinguish implementation
// anchors from illustrative doc / comment mentions using the same
// language-aware logic as Tier 1 grounding.
func LineLooksCommentOnly(fileLines map[int]string, line int, source string) bool {
	return isLineComment(fileLines, line, source)
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
	// Lua single-line: --
	if hasLuaLineComment(lang) && strings.HasPrefix(trimmed, "--") {
		if end, width := commentLuaLongOpen(trimmed, 2); width > 0 {
			return soloDelimitedComment(trimmed, 2+width, end, "--")
		}
		return true
	}
	// Python / shell / YAML / Ruby / TOML: # at line start
	if hasHashLineComment(lang) && strings.HasPrefix(trimmed, "#") {
		return true
	}
	// A leading block opener is solo only when no code follows its close.
	// A decorated `* text` without an observed opener is not proof: it can
	// also be a dereference/assignment in C-family source.
	if hasCFamilyLineComment(lang) {
		if trimmed == "*" || trimmed == "*/" {
			return true
		}
		// Opener on its own line counts as comment too.
		if strings.HasPrefix(trimmed, "/*") {
			return soloDelimitedComment(trimmed, 2, "*/", "//")
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
			strings.HasPrefix(trimmed, "--") ||
			trimmed == "*" || trimmed == "*/" {
			return true
		}
		if strings.HasPrefix(trimmed, "/*") {
			return soloDelimitedComment(trimmed, 2, "*/", "//")
		}
	}
	return false
}

func isConfigSoloComment(trimmed, source string) bool {
	if trimmed == "" || !codraxtypes.LooksLikeConfigFilePath(source) {
		return false
	}
	switch {
	case strings.HasPrefix(trimmed, "#"),
		strings.HasPrefix(trimmed, ";"),
		strings.HasPrefix(trimmed, "//"):
		return true
	case strings.HasPrefix(trimmed, "/*"):
		return soloDelimitedComment(trimmed, 2, "*/", "//")
	case strings.HasPrefix(trimmed, "<!--"):
		return soloDelimitedComment(trimmed, 4, "-->", "")
	case trimmed == "*/", trimmed == "-->":
		return true
	}
	return false
}

func soloDelimitedComment(line string, afterOpen int, close, lineMarker string) bool {
	i := strings.Index(line[afterOpen:], close)
	if i < 0 {
		return true
	}
	tail := strings.TrimSpace(line[afterOpen+i+len(close):])
	return tail == "" || lineMarker != "" && strings.HasPrefix(tail, lineMarker)
}

// This is a bounded comment recognizer, not a full language parser. Literal
// bodies are opaque (not executable proof and not comment proof); exact local
// closing delimiters let later ordinary code/comments resume classification.
type commentLexState struct {
	end              string
	comment          bool
	escapes          bool
	depth            int
	nested           bool
	opaque           bool
	pythonDepth      int
	pythonContinued  bool
	ambiguousLiteral bool
	escapePrefix     string
}

func observedCommentLine(lines map[int]string, line int, source, lang string) (bool, bool) {
	const maxWalk = 200
	if line < 1 || line-1 > maxWalk {
		return false, false
	}
	for i := 1; i <= line; i++ {
		if _, ok := lines[i]; !ok {
			return false, false
		}
	}
	var state commentLexState
	result := false
	for i := 1; i <= line; i++ {
		result = scanObservedCommentLine(lines[i], source, lang, &state)
	}
	return result, true
}

func scanObservedCommentLine(raw, source, lang string, state *commentLexState) bool {
	// No parser/state checkpoint is available for an ambiguous advanced
	// literal. Keep this bounded observed continuation unknown rather than
	// guess its closing delimiter and demote subsequent code as a comment.
	if state.ambiguousLiteral {
		return false
	}
	config := codraxtypes.LooksLikeConfigFilePath(source)
	cFamily := hasCFamilyBlockComment(lang) || lang == "" || config
	python := lang == repomaptypes.LangPython || lang == ""
	pythonDepth, wasContinued := state.pythonDepth, state.pythonContinued
	continued := false
	defer func() { state.pythonDepth = pythonDepth; state.pythonContinued = continued }()
	lua := hasLuaBlockComment(lang) || lang == ""
	hasComment, hasCode := false, false
	for i := 0; i < len(raw); {
		if state.end != "" {
			if commentLiteralHasUnparsedInterpolation(raw[i:], lang, state) {
				state.ambiguousLiteral = true
				return false
			}
			if state.comment && !state.opaque {
				hasComment = true
			} else {
				hasCode = true
			}
			if state.escapes && raw[i] == '\\' {
				i += 2
				continue
			}
			if state.escapePrefix != "" && strings.HasPrefix(raw[i:], state.escapePrefix) {
				i += len(state.escapePrefix) + 1
				continue
			}
			if state.nested && strings.HasPrefix(raw[i:], "/*") {
				state.depth++
				i += 2
				// The bounded Cangjie/unknown recognizer does not claim
				// nested-comment grammar authority; retain this region opaque.
				if lang == repomaptypes.LangCangjie || lang == "" {
					state.opaque = true
					hasCode = true
				}
				continue
			}
			if strings.HasPrefix(raw[i:], state.end) {
				i += len(state.end)
				state.depth--
				if state.depth == 0 {
					*state = commentLexState{}
				}
				continue
			}
			i++
			continue
		}
		if raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\r' {
			i++
			continue
		}
		if lang == repomaptypes.LangRuby && (strings.HasPrefix(raw[i:], "<<") || raw[i] == '%' && i+1 < len(raw) && strings.ContainsRune("qQrwWiIx", rune(raw[i+1]))) || lang == repomaptypes.LangCangjie && raw[i] == '#' {
			state.ambiguousLiteral = true
			return false
		}
		if lua && strings.HasPrefix(raw[i:], "--") {
			if end, width := commentLuaLongOpen(raw, i+2); width > 0 {
				*state = commentLexState{end: end, comment: true, depth: 1}
				hasComment = true
				i += 2 + width
				continue
			}
			hasComment = true
			break
		}
		if cFamily && strings.HasPrefix(raw[i:], "//") || (hasHashLineComment(lang) || lang == "" || config) && raw[i] == '#' || config && raw[i] == ';' {
			hasComment = true
			break
		}
		if cFamily && strings.HasPrefix(raw[i:], "/*") {
			*state = commentLexState{end: "*/", comment: true, depth: 1, nested: lang == repomaptypes.LangRust || lang == repomaptypes.LangKotlin || lang == repomaptypes.LangSwift || lang == repomaptypes.LangCangjie || lang == ""}
			hasComment = true
			i += 2
			continue
		}
		if config && strings.HasPrefix(raw[i:], "<!--") {
			*state = commentLexState{end: "-->", comment: true, depth: 1}
			hasComment = true
			i += 4
			continue
		}
		if end, width := commentRawLiteralOpen(raw, i, lang); width > 0 {
			*state = commentLexState{end: end, depth: 1}
			if lang == repomaptypes.LangSwift {
				if at := strings.IndexByte(end, '#'); at >= 0 {
					state.escapePrefix = "\\" + end[at:]
				}
			}
			hasCode = true
			i += width
			continue
		}
		if lua {
			if end, width := commentLuaLongOpen(raw, i); width > 0 {
				*state = commentLexState{end: end, depth: 1}
				hasCode = true
				i += width
				continue
			}
		}
		if strings.HasPrefix(raw[i:], `"""`) || strings.HasPrefix(raw[i:], `'''`) {
			if lang == repomaptypes.LangPython && commentPythonFStringPrefix(raw, i) {
				state.ambiguousLiteral = true
				return false
			}
			end := raw[i : i+3]
			// Prefixed Python strings may contain interpolation. They are
			// deliberately opaque, never promoted to doc/comment evidence.
			comment := python && !hasCode && pythonDepth == 0 && !wasContinued && (i == 0 || !commentIdentifierByte(raw[i-1]))
			*state = commentLexState{end: end, comment: comment, escapes: true, depth: 1}
			if comment {
				hasComment = true
			} else {
				hasCode = true
			}
			i += 3
			continue
		}
		if raw[i] == '"' || raw[i] == '\'' || raw[i] == '`' {
			if lang == repomaptypes.LangPython && commentPythonFStringPrefix(raw, i) {
				state.ambiguousLiteral = true
				return false
			}
			if raw[i] == '\'' && lang == repomaptypes.LangRust && precedenceQuoteStart(raw, i, source) == 0 {
				hasCode = true
				i++
				continue
			}
			if raw[i] == '\'' && lang == repomaptypes.LangCpp && i > 0 && i+1 < len(raw) && raw[i-1] >= '0' && raw[i-1] <= '9' && raw[i+1] >= '0' && raw[i+1] <= '9' {
				hasCode = true
				i++
				continue
			}
			*state = commentLexState{end: raw[i : i+1], depth: 1, escapes: raw[i] != '`' || lang != repomaptypes.LangGo}
			hasCode = true
			i++
			continue
		}
		if raw[i] == '/' && (lang == repomaptypes.LangJavaScript || lang == repomaptypes.LangTypeScript || lang == repomaptypes.LangArkTS) {
			if end := commentJSRegexEnd(raw, i); end > i {
				hasCode = true
				i = end
				continue
			}
		}
		if python {
			switch raw[i] {
			case '(', '[', '{':
				pythonDepth++
			case ')', ']', '}':
				if pythonDepth > 0 {
					pythonDepth--
				}
			case '\\':
				continued = strings.TrimSpace(raw[i+1:]) == ""
			}
		}
		hasCode = true
		i++
	}
	return hasComment && !hasCode
}

func commentIdentifierByte(ch byte) bool {
	return ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

// These are literal-boundary uncertainty markers, not expression parsers.
// A nested quote inside an unparsed interpolation must never be mistaken for
// the literal's close and let payload delimiters mint comment authority.
func commentLiteralHasUnparsedInterpolation(text, lang string, state *commentLexState) bool {
	if state.comment {
		return false
	}
	switch lang {
	case repomaptypes.LangJavaScript, repomaptypes.LangTypeScript, repomaptypes.LangArkTS:
		return state.end == "`" && strings.HasPrefix(text, "${")
	case repomaptypes.LangKotlin, repomaptypes.LangCangjie:
		return strings.HasPrefix(state.end, "\"") && strings.HasPrefix(text, "${")
	case repomaptypes.LangSwift:
		prefix := state.escapePrefix
		if prefix == "" {
			prefix = "\\"
		}
		return strings.HasPrefix(state.end, "\"") && strings.HasPrefix(text, prefix+"(")
	case repomaptypes.LangJava:
		return strings.HasPrefix(state.end, "\"") && strings.HasPrefix(text, `\{`)
	case repomaptypes.LangRuby:
		return state.end == "\"" && strings.HasPrefix(text, "#{")
	}
	return false
}

func commentPythonFStringPrefix(line string, quote int) bool {
	start := quote
	for start > 0 && commentIdentifierByte(line[start-1]) {
		start--
	}
	switch strings.ToLower(line[start:quote]) {
	case "f", "fr", "rf":
		return true
	}
	return false
}

func commentLuaLongOpen(s string, i int) (string, int) {
	if i >= len(s) || s[i] != '[' {
		return "", 0
	}
	j := i + 1
	for j < len(s) && s[j] == '=' {
		j++
	}
	if j >= len(s) || s[j] != '[' {
		return "", 0
	}
	return "]" + s[i+1:j] + "]", j - i + 1
}

func commentRawLiteralOpen(s string, i int, lang string) (string, int) {
	if lang == repomaptypes.LangSwift && s[i] == '#' {
		j := i
		for j < len(s) && s[j] == '#' {
			j++
		}
		if j < len(s) && s[j] == '"' {
			quote := "\""
			if strings.HasPrefix(s[j:], `"""`) {
				quote = `"""`
			}
			return quote + s[i:j], j - i + len(quote)
		}
	}
	if lang == repomaptypes.LangRust && s[i] == 'r' {
		j := i + 1
		for j < len(s) && s[j] == '#' {
			j++
		}
		if j < len(s) && s[j] == '"' {
			return "\"" + s[i+1:j], j - i + 1
		}
	}
	if lang == repomaptypes.LangCpp && strings.HasPrefix(s[i:], `R"`) {
		j := i + 2
		for j < len(s) && j-i <= 18 && s[j] != '(' {
			if s[j] == ' ' || s[j] == '\t' || s[j] == '\\' || s[j] == ')' {
				return "", 0
			}
			j++
		}
		if j < len(s) && s[j] == '(' {
			return ")" + s[i+2:j] + "\"", j - i + 1
		}
	}
	return "", 0
}

// A complete same-line regex-shaped literal is opaque. This does not claim
// expression/regex authority (division-shaped ambiguity may lose comment
// classification, never gain it). No regex content can open a block comment.
func commentJSRegexEnd(s string, start int) int {
	class := false
	for i := start + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '[' {
			class = true
			continue
		}
		if s[i] == ']' {
			class = false
			continue
		}
		if s[i] == '/' && !class {
			return i + 1
		}
	}
	return -1
}

// hasCFamilyLineComment reports whether the language uses `//` as a
// single-line comment marker.
func hasCFamilyLineComment(lang string) bool {
	switch lang {
	case repomaptypes.LangGo, repomaptypes.LangJavaScript, repomaptypes.LangTypeScript,
		repomaptypes.LangArkTS, repomaptypes.LangCangjie,
		repomaptypes.LangJava, repomaptypes.LangKotlin, repomaptypes.LangRust,
		repomaptypes.LangSwift, repomaptypes.LangProto,
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
	case repomaptypes.LangPython, repomaptypes.LangRuby:
		return true
	}
	// Unknown extensions (shell, YAML, TOML, Ruby, Makefile, etc.) fall
	// through to the empty-lang branch in isLineSoloComment which
	// permissively recognises `#`.
	return false
}

func hasLuaLineComment(lang string) bool {
	return lang == repomaptypes.LangLua
}

func hasLuaBlockComment(lang string) bool {
	return lang == repomaptypes.LangLua
}
