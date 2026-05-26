package mermaidcompat

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// NormalizeSourceForMarkdown repairs Mermaid-spec-adjacent syntax slips that
// commonly parse in one renderer but fail in Mermaid.js or third-party
// renderers. It is intentionally source-level and meaning-preserving: callers
// can use the returned body as the markdown Mermaid source, not just as an
// internal terminal-rendering shim.
func NormalizeSourceForMarkdown(body string) string {
	original := body
	body = NormalizeSequenceParticipantMessagePrefixes(body)
	body = NormalizeSequenceStops(body)
	body = NormalizeFlowchartSubgraphTitles(body)
	body = NormalizeFlowchartDanglingPunctuation(body)
	body = NormalizeFlowchartMismatchedShapeClosers(body)
	body = NormalizeFlowchartUnsafeNodeIDs(body)
	body = NormalizeFlowchartNodeLabels(body)
	body = NormalizeFlowchartPipeLabels(body)
	if body != original {
		logging.Debug("[mermaidcompat] source repair applied before_bytes=%d after_bytes=%d", len(original), len(body))
	}
	return body
}

// NormalizeFlowchartMismatchedShapeClosers repairs a narrow but common
// Mermaid slip where the node shape opener is clear but the final closer comes
// from a sibling shape, e.g. `A{"ok"]` or `B("step"]`. The opener determines
// the intended shape, so replacing only the mismatched closing delimiter is a
// syntax-only repair. Running this before unsafe-node aliasing prevents the
// label inside a malformed shape from being mistaken for a separate node ID.
func NormalizeFlowchartMismatchedShapeClosers(body string) string {
	if !isFlowchartOrGraph(body) || !strings.ContainsAny(body, "[({>") {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		rewritten, ok := normalizeFlowchartMismatchedShapeClosersInLine(line)
		if ok {
			lines[i] = rewritten
			changed = true
		}
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

// NormalizeSequenceStops repairs a common small-model Mermaid mistake:
// a bare `stop` line inside sequenceDiagram. Mermaid supports stop-like
// control flow in other syntaxes, but sequence diagrams need every
// visible statement to be a message, note, activation, or block marker.
// Rewriting the bare token to a note preserves the user's visible
// meaning while keeping both browser Mermaid and the terminal renderer
// parseable.
func NormalizeSequenceStops(body string) string {
	if !isSequenceDiagram(body) || !strings.Contains(body, "stop") {
		return body
	}
	scope := sequenceNoteScope(body)
	if scope == "" {
		scope = "Sequence"
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.EqualFold(trimmed, "stop") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + "Note over " + scope + ": stop"
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

// NormalizeSequenceParticipantMessagePrefixes repairs a small-model
// Mermaid slip where a sequence message line is accidentally prefixed
// with `participant` or `actor`, for example:
//
//	participant A->>B: call
//
// The prefix is syntactically invalid for Mermaid and carries no extra
// semantic information. Removing only that prefix preserves the
// authored edge and lets browser Mermaid / mermaid-ascii parse the
// diagram. The guard requires a message arrow and a message label so
// real participant declarations, including declarations with quoted
// labels, are left untouched.
func NormalizeSequenceParticipantMessagePrefixes(body string) string {
	if !isSequenceDiagram(body) {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		trimmedLeft := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmedLeft)]
		prefix := ""
		switch {
		case strings.HasPrefix(trimmedLeft, "participant "):
			prefix = "participant "
		case strings.HasPrefix(trimmedLeft, "actor "):
			prefix = "actor "
		default:
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmedLeft, prefix))
		if !sequenceMessageLineLike(rest) {
			continue
		}
		lines[i] = indent + rest
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

// NormalizeFlowchartPipeLabels quotes pipe-delimited flowchart edge labels
// that contain characters Mermaid.js tokenizes as shape delimiters. For
// example, `A -->|success (x==true)| B` fails in Mermaid 11 because `(` starts
// a node-shape token inside an unquoted edge label. The standard Mermaid form
// is `A -->|"success (x==true)"| B`, which preserves the visible label and is
// accepted by browser and third-party Mermaid renderers.
func NormalizeFlowchartPipeLabels(body string) string {
	if !isFlowchartOrGraph(body) || !strings.Contains(body, "|") {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		rewritten, ok := normalizeFlowchartPipeLabelsInLine(line)
		if ok {
			lines[i] = rewritten
			changed = true
		}
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

// NormalizeFlowchartDanglingPunctuation removes punctuation that is clearly
// outside a flowchart statement. Small/local models sometimes end the final
// edge with a prose comma:
//
//	I --> J["done"],
//
// Mermaid.js treats that comma as syntax, while removing it preserves the
// authored edge and visible labels. The repair is deliberately narrow: it only
// applies to flowchart/graph bodies, only trims commas at physical line ends,
// and never edits punctuation inside quoted labels or node shapes.
func NormalizeFlowchartDanglingPunctuation(body string) string {
	if !isFlowchartOrGraph(body) || !strings.ContainsAny(body, ",，") {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		rewritten, ok := normalizeFlowchartDanglingPunctuationLine(line)
		if ok {
			lines[i] = rewritten
			changed = true
		}
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

func normalizeFlowchartDanglingPunctuationLine(line string) (string, bool) {
	trimmedRight := strings.TrimRightFunc(line, unicode.IsSpace)
	if trimmedRight == "" {
		return line, false
	}
	last := lastRune(trimmedRight)
	if last != ',' && last != '，' {
		return line, false
	}
	core := strings.TrimRightFunc(trimmedRight[:len(trimmedRight)-len(string(last))], unicode.IsSpace)
	if strings.TrimSpace(core) == "" || !flowchartLineCanCarryDanglingComma(core) {
		return line, false
	}
	suffix := line[len(trimmedRight):]
	return core + suffix, true
}

func flowchartLineCanCarryDanglingComma(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%%") ||
		flowchartLineIsHeader(trimmed) || trimmed == "end" ||
		strings.HasPrefix(trimmed, "classDef ") || strings.HasPrefix(trimmed, "linkStyle ") {
		return false
	}
	if flowchartLineStartsWithAny(trimmed, "click ", "style ", "class ") {
		return true
	}
	if strings.Contains(trimmed, "--") || strings.Contains(trimmed, "->") || strings.Contains(trimmed, "==") {
		return true
	}
	// Node declarations without edges can also be produced as list-like prose
	// lines (`A["x"],`). Accept only when the remaining line has balanced node
	// shape delimiters so a comma inside a label is not mistaken for syntax.
	return flowchartLikelyBalancedNodeStatement(trimmed)
}

func flowchartLikelyBalancedNodeStatement(line string) bool {
	if !strings.ContainsAny(line, "[({>") {
		return false
	}
	for i := 0; i < len(line); i++ {
		if span, ok := flowchartLabelShapeSpanAt(line, i); ok && span.end == len(line)-1 {
			return true
		}
	}
	return false
}

func lastRune(s string) rune {
	var last rune
	for _, r := range s {
		last = r
	}
	return last
}

// NormalizeFlowchartUnsafeNodeIDs aliases flowchart node identifiers that are
// not portable across Mermaid renderers, such as file paths (`../A.md`) or
// source locations (`pkg/a.go:12`). The visible label keeps the original text,
// while the generated ID is syntax-safe:
//
//	../A.md --> pkg/B.ts
//	codraxNode1["../A.md"] --> codraxNode2["pkg/B.ts"]
//
// This is a syntax repair only. It does not infer semantics or change edge
// direction; it simply gives Mermaid a valid internal node ID.
func NormalizeFlowchartUnsafeNodeIDs(body string) string {
	if !isFlowchartOrGraph(body) {
		return body
	}
	lines := strings.Split(body, "\n")
	aliases := map[string]string{}
	changed := false
	for i, line := range lines {
		rewritten, ok := normalizeFlowchartUnsafeNodeIDsInLine(line, aliases)
		if ok {
			lines[i] = rewritten
			changed = true
		}
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

// NormalizeFlowchartNodeLabels quotes unquoted flowchart node-shape labels
// that contain characters Mermaid.js tokenizes as shape delimiters. For
// example, `A[preStages\n(Conditional)]` fails because the `(` starts a new
// shape token inside an unquoted label. Mermaid's portable form is
// `A["preStages\n(Conditional)"]`, which preserves the visible label and
// parses in browser and third-party renderers.
func NormalizeFlowchartNodeLabels(body string) string {
	if !isFlowchartOrGraph(body) || !strings.ContainsAny(body, "([{>") {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		rewritten, ok := normalizeFlowchartNodeLabelsInLine(line)
		if ok {
			lines[i] = rewritten
			changed = true
		}
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

func normalizeFlowchartUnsafeNodeIDsInLine(line string, aliases map[string]string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%%") ||
		flowchartLineIsHeader(trimmed) ||
		strings.HasPrefix(trimmed, "subgraph ") || trimmed == "end" ||
		strings.HasPrefix(trimmed, "classDef ") || strings.HasPrefix(trimmed, "linkStyle ") {
		return line, false
	}
	if flowchartLineStartsWithAny(trimmed, "click ", "style ", "class ") {
		return normalizeFlowchartDirectiveNodeRef(line, aliases)
	}
	var b strings.Builder
	changed := false
	for i := 0; i < len(line); {
		if op := flowchartArrowAt(line, i); op != "" {
			b.WriteString(op)
			i += len(op)
			continue
		}
		ch := line[i]
		if ch == '|' {
			if end := findUnescapedPipe(line, i+1); end > i {
				b.WriteString(line[i : end+1])
				i = end + 1
				continue
			}
		}
		if end := flowchartShapeEnd(line, i); end > i {
			b.WriteString(line[i : end+1])
			i = end + 1
			continue
		}
		if flowchartNodeTokenDelimiter(ch) {
			b.WriteByte(ch)
			i++
			continue
		}
		start := i
		for i < len(line) {
			if flowchartArrowAt(line, i) != "" || flowchartNodeTokenDelimiter(line[i]) {
				break
			}
			i++
		}
		token := line[start:i]
		repl, ok := flowchartNodeTokenAlias(token, line, i, aliases)
		if ok {
			b.WriteString(repl)
			changed = true
			continue
		}
		b.WriteString(token)
	}
	if !changed {
		return line, false
	}
	return b.String(), true
}

func flowchartShapeEnd(s string, i int) int {
	if i < 0 || i >= len(s) {
		return -1
	}
	close := byte(0)
	switch s[i] {
	case '[':
		close = ']'
	case '(':
		close = ')'
	case '{':
		close = '}'
	case '>':
		close = ']'
	default:
		return -1
	}
	quote := byte(0)
	escaped := false
	for j := i + 1; j < len(s); j++ {
		ch := s[j]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == close {
			return j
		}
	}
	return -1
}

func flowchartNodeTokenDelimiter(ch byte) bool {
	switch ch {
	case ' ', '\t', '\r', '\n', '[', '(', '{', '>', ']', ')', '}', '|', '&', ';', ',':
		return true
	default:
		return false
	}
}

func normalizeFlowchartMismatchedShapeClosersInLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%%") ||
		flowchartLineIsHeader(trimmed) || strings.HasPrefix(trimmed, "subgraph ") || trimmed == "end" ||
		strings.HasPrefix(trimmed, "classDef ") || strings.HasPrefix(trimmed, "linkStyle ") ||
		flowchartLineStartsWithAny(trimmed, "click ", "style ", "class ") {
		return line, false
	}
	out := []byte(line)
	changed := false
	for i := 0; i < len(out); i++ {
		if op := flowchartArrowAt(string(out), i); op != "" {
			i += len(op) - 1
			continue
		}
		if out[i] == '|' {
			if end := findUnescapedPipe(string(out), i+1); end > i {
				i = end
				continue
			}
		}
		expected, ok := flowchartSingleShapeClose(out[i])
		if !ok {
			continue
		}
		if end := flowchartShapeEnd(string(out), i); end > i {
			i = end
			continue
		}
		mismatch := findFlowchartMismatchedShapeClose(string(out), i+1, expected)
		if mismatch < 0 {
			continue
		}
		out[mismatch] = expected
		changed = true
		i = mismatch
	}
	if !changed {
		return line, false
	}
	return string(out), true
}

func flowchartSingleShapeClose(open byte) (byte, bool) {
	switch open {
	case '[':
		return ']', true
	case '(':
		return ')', true
	case '{':
		return '}', true
	case '>':
		return ']', true
	default:
		return 0, false
	}
}

func findFlowchartMismatchedShapeClose(s string, start int, expected byte) int {
	quote := byte(0)
	escaped := false
	for i := start; i < len(s); i++ {
		ch := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if flowchartArrowAt(s, i) != "" {
			return -1
		}
		switch ch {
		case '[', '(', '{', '>':
			return -1
		case ']', ')', '}':
			if ch == expected {
				return -1
			}
			return i
		}
	}
	return -1
}

func flowchartArrowAt(s string, i int) string {
	if i < 0 || i >= len(s) {
		return ""
	}
	for _, op := range []string{"-.->", "-->>", "-->", "==>", "->>", "---", "==", "->"} {
		if strings.HasPrefix(s[i:], op) {
			return op
		}
	}
	return ""
}

func flowchartNodeTokenAlias(token, line string, end int, aliases map[string]string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" || flowchartNodeIDIsSafe(token) || !flowchartNodeIDNeedsAlias(token) {
		return "", false
	}
	alias := aliases[token]
	if alias == "" {
		alias = "codraxNode" + strconv.Itoa(len(aliases)+1)
		aliases[token] = alias
	}
	if flowchartTokenFollowedByShape(line, end) {
		return alias, true
	}
	return alias + `["` + escapeFlowchartNodeLabel(token) + `"]`, true
}

func normalizeFlowchartDirectiveNodeRef(line string, aliases map[string]string) (string, bool) {
	if len(aliases) == 0 {
		return line, false
	}
	leadingLen := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
	indent := line[:leadingLen]
	trimmed := strings.TrimSpace(line)
	fields := strings.Fields(trimmed)
	if len(fields) < 2 {
		return line, false
	}
	alias := aliases[fields[1]]
	if alias == "" {
		return line, false
	}
	restStart := strings.Index(trimmed, fields[1])
	if restStart < 0 {
		return line, false
	}
	return indent + trimmed[:restStart] + alias + trimmed[restStart+len(fields[1]):], true
}

func flowchartLineStartsWithAny(line string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func flowchartLineIsHeader(line string) bool {
	return line == "graph" || strings.HasPrefix(line, "graph ") ||
		line == "flowchart" || strings.HasPrefix(line, "flowchart ")
}

func flowchartTokenFollowedByShape(line string, pos int) bool {
	for pos < len(line) {
		switch line[pos] {
		case ' ', '\t':
			pos++
			continue
		case '[', '(', '{', '>':
			return true
		default:
			return false
		}
	}
	return false
}

func flowchartNodeIDIsSafe(id string) bool {
	if id == "" {
		return false
	}
	for i, r := range id {
		if i == 0 {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
				continue
			}
			return false
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func flowchartNodeIDNeedsAlias(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '_':
			continue
		default:
			return true
		}
	}
	return !flowchartNodeIDIsSafe(id)
}

func escapeFlowchartNodeLabel(label string) string {
	label = strings.ReplaceAll(label, `"`, "&quot;")
	return label
}

type flowchartShapeSpan struct {
	open       string
	close      string
	labelStart int
	labelEnd   int
	end        int
}

func normalizeFlowchartNodeLabelsInLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%%") ||
		flowchartLineIsHeader(trimmed) || trimmed == "end" ||
		strings.HasPrefix(trimmed, "classDef ") || strings.HasPrefix(trimmed, "linkStyle ") ||
		flowchartLineStartsWithAny(trimmed, "click ", "style ", "class ") {
		return line, false
	}
	var b strings.Builder
	changed := false
	for i := 0; i < len(line); {
		if op := flowchartArrowAt(line, i); op != "" {
			b.WriteString(op)
			i += len(op)
			continue
		}
		if line[i] == '|' {
			if end := findUnescapedPipe(line, i+1); end > i {
				b.WriteString(line[i : end+1])
				i = end + 1
				continue
			}
		}
		span, ok := flowchartLabelShapeSpanAt(line, i)
		if !ok {
			b.WriteByte(line[i])
			i++
			continue
		}
		label := line[span.labelStart:span.labelEnd]
		normalized, labelChanged := normalizeFlowchartNodeLabel(label)
		if !labelChanged {
			b.WriteString(line[i : span.end+1])
			i = span.end + 1
			continue
		}
		b.WriteString(span.open)
		b.WriteString(normalized)
		b.WriteString(span.close)
		changed = true
		i = span.end + 1
	}
	if !changed {
		return line, false
	}
	return b.String(), true
}

func flowchartLabelShapeSpanAt(s string, i int) (flowchartShapeSpan, bool) {
	if i < 0 || i >= len(s) {
		return flowchartShapeSpan{}, false
	}
	for _, pair := range []struct {
		open  string
		close string
	}{
		{open: "[(", close: ")]"},
		{open: "[[", close: "]]"},
		{open: "((", close: "))"},
		{open: "{{", close: "}}"},
		{open: "[", close: "]"},
		{open: "(", close: ")"},
		{open: "{", close: "}"},
		{open: ">", close: "]"},
	} {
		if !strings.HasPrefix(s[i:], pair.open) {
			continue
		}
		closeStart := findFlowchartShapeClose(s, i+len(pair.open), pair.close)
		if closeStart < 0 {
			return flowchartShapeSpan{}, false
		}
		return flowchartShapeSpan{
			open:       pair.open,
			close:      pair.close,
			labelStart: i + len(pair.open),
			labelEnd:   closeStart,
			end:        closeStart + len(pair.close) - 1,
		}, true
	}
	return flowchartShapeSpan{}, false
}

func findFlowchartShapeClose(s string, start int, close string) int {
	quote := byte(0)
	escaped := false
	for i := start; i <= len(s)-len(close); i++ {
		ch := s[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if strings.HasPrefix(s[i:], close) {
			return i
		}
	}
	return -1
}

func normalizeFlowchartNodeLabel(label string) (string, bool) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" || mermaidPipeLabelAlreadyQuoted(trimmed) || !mermaidPipeLabelNeedsQuotes(trimmed) {
		return label, false
	}
	prefixLen := len(label) - len(strings.TrimLeftFunc(label, unicode.IsSpace))
	suffixLen := len(label) - len(strings.TrimRightFunc(label, unicode.IsSpace))
	prefix := label[:prefixLen]
	suffix := label[len(label)-suffixLen:]
	inner := escapeFlowchartNodeLabel(strings.TrimSpace(label))
	return prefix + `"` + inner + `"` + suffix, true
}

func normalizeFlowchartPipeLabelsInLine(line string) (string, bool) {
	if strings.Contains(strings.TrimSpace(line), "%%") && strings.HasPrefix(strings.TrimSpace(line), "%%") {
		return line, false
	}
	var b strings.Builder
	changed := false
	for i := 0; i < len(line); {
		if line[i] != '|' {
			b.WriteByte(line[i])
			i++
			continue
		}
		end := findUnescapedPipe(line, i+1)
		if end < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		label := line[i+1 : end]
		normalized, ok := normalizeFlowchartPipeLabel(label)
		if ok {
			b.WriteByte('|')
			b.WriteString(normalized)
			b.WriteByte('|')
			changed = true
		} else {
			b.WriteString(line[i : end+1])
		}
		i = end + 1
	}
	if !changed {
		return line, false
	}
	return b.String(), true
}

func findUnescapedPipe(s string, start int) int {
	escaped := false
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '\\':
			escaped = !escaped
		case '|':
			if !escaped {
				return i
			}
			escaped = false
		default:
			escaped = false
		}
	}
	return -1
}

func normalizeFlowchartPipeLabel(label string) (string, bool) {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" || mermaidPipeLabelAlreadyQuoted(trimmed) || !mermaidPipeLabelNeedsQuotes(trimmed) {
		return label, false
	}
	prefixLen := len(label) - len(strings.TrimLeftFunc(label, unicode.IsSpace))
	suffixLen := len(label) - len(strings.TrimRightFunc(label, unicode.IsSpace))
	prefix := label[:prefixLen]
	suffix := label[len(label)-suffixLen:]
	inner := strings.TrimSpace(label)
	inner = strings.ReplaceAll(inner, `"`, "&quot;")
	return prefix + `"` + inner + `"` + suffix, true
}

func mermaidPipeLabelAlreadyQuoted(label string) bool {
	if len(label) < 2 {
		return false
	}
	return (label[0] == '"' && label[len(label)-1] == '"') ||
		(label[0] == '\'' && label[len(label)-1] == '\'')
}

func mermaidPipeLabelNeedsQuotes(label string) bool {
	for _, r := range label {
		switch r {
		case '(', ')', '[', ']', '{', '}':
			return true
		}
	}
	return false
}

// NormalizeFlowchartSubgraphTitles repairs the common LLM shape
// `subgraph Explorer System` into Mermaid's explicit
// `subgraph Explorer_System [Explorer System]` form. It only touches
// flowchart/graph bodies and only when the subgraph title has multiple bare
// tokens; already explicit Mermaid forms are preserved.
func NormalizeFlowchartSubgraphTitles(body string) string {
	if !strings.Contains(body, "subgraph") || !isFlowchartOrGraph(body) {
		return body
	}
	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		leading := len(line) - len(strings.TrimLeftFunc(line, unicode.IsSpace))
		indent := line[:leading]
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "subgraph ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "subgraph "))
		if !flowchartSubgraphTitleNeedsRepair(rest) {
			continue
		}
		lines[i] = indent + "subgraph " + flowchartSubgraphID(rest, i) + " [" + rest + "]"
		changed = true
	}
	if !changed {
		return body
	}
	return strings.Join(lines, "\n")
}

func sequenceMessageLineLike(line string) bool {
	arrowAt := -1
	for _, op := range []string{"-->>", "->>", "-->", "->", "--x", "-x", "--)", "-)"} {
		if idx := strings.Index(line, op); idx >= 0 && (arrowAt < 0 || idx < arrowAt) {
			arrowAt = idx
		}
	}
	if arrowAt <= 0 {
		return false
	}
	colon := strings.Index(line[arrowAt:], ":")
	return colon > 0 && strings.TrimSpace(line[:arrowAt]) != ""
}

func isSequenceDiagram(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line == "sequenceDiagram"
	}
	return false
}

func isFlowchartOrGraph(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return flowchartLineIsHeader(line)
	}
	return false
}

func flowchartSubgraphTitleNeedsRepair(rest string) bool {
	if rest == "" || strings.ContainsAny(rest, "[]\"") {
		return false
	}
	return len(strings.Fields(rest)) > 1
}

func flowchartSubgraphID(title string, lineIndex int) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range title {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	id := strings.Trim(b.String(), "_")
	if id == "" {
		id = "subgraph"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "sg_" + id
	}
	return id + "_" + strconv.Itoa(lineIndex+1)
}

func sequenceNoteScope(body string) string {
	var participants []string
	seen := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[0] != "participant" && fields[0] != "actor" {
			continue
		}
		id := strings.Trim(fields[1], "[]")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		participants = append(participants, id)
	}
	switch len(participants) {
	case 0:
		return ""
	case 1:
		return participants[0]
	default:
		return participants[0] + "," + participants[len(participants)-1]
	}
}
