package mermaidcompat

import "strings"

var sequenceArrowOperators = []string{
	"-->>+", "-->>-", "->>+", "->>-",
	"--)+", "--)-", "-)+", "-)-",
	"--x+", "--x-", "-x+", "-x-",
	"-->+", "-->-", "->+", "->-",
	"-->>", "->>", "-->", "->", "--x", "-x", "--)", "-)",
}

// Edge is the (from, to[, label]) tuple extracted from one Mermaid edge
// declaration. The fields are syntax-level surfaces; semantic relation
// inference lives outside this package.
type Edge struct {
	From      string
	To        string
	Label     string
	Operator  string
	FromLabel string
	ToLabel   string
	FromShape string
	ToShape   string
}

// NodeDecl is the (identifier, visible label) pair extracted from a
// Mermaid node declaration or sequence participant declaration.
type NodeDecl struct {
	Ident string
	Label string
}

const NodeShapeDecision = "decision"

// ParseEdges scans a Mermaid body and returns every edge declaration it
// can recognise. It intentionally covers the flowchart / sequenceDiagram
// shapes Codrax asks finalizers to emit, not the whole Mermaid grammar.
func ParseEdges(body string) []Edge {
	var edges []Edge
	sequenceBody := false
	for _, raw := range strings.Split(body, "\n") {
		line := strings.ToLower(strings.TrimSpace(raw))
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		sequenceBody = strings.HasPrefix(line, "sequencediagram")
		break
	}
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "classDef") || strings.HasPrefix(line, "click") {
			continue
		}
		var from, to, label, operator string
		var ok bool
		if sequenceBody {
			from, to, operator, ok = splitSequenceEdgeLine(line)
		} else {
			from, to, label, operator, ok = SplitEdgeLine(line)
		}
		if !ok {
			continue
		}
		if sequenceBody {
			if target, message, found := splitSequenceEdgeTargetMessage(to); found {
				to = target
				if label == "" {
					label = message
				}
			}
		}
		var fromLabel, toLabel, fromShape, toShape string
		from, fromLabel, fromShape = ParseNodeTokenWithShape(from)
		to, toLabel, toShape = ParseNodeTokenWithShape(to)
		if from == "" || to == "" {
			continue
		}
		edges = append(edges, Edge{
			From:      from,
			To:        to,
			Label:     label,
			Operator:  operator,
			FromLabel: fromLabel,
			ToLabel:   toLabel,
			FromShape: fromShape,
			ToShape:   toShape,
		})
	}
	return edges
}

// splitSequenceEdgeLine selects the first arrow in source order and leaves
// the remainder opaque for splitSequenceEdgeTargetMessage. Message text may
// legitimately contain Mermaid-looking arrow bytes; those are not a second
// edge and must not replace the actor-to-actor invocation being parsed.
func splitSequenceEdgeLine(line string) (from, to, operator string, ok bool) {
	idx, operator := FindSequenceArrow(line)
	if idx < 0 {
		return "", "", "", false
	}
	from = strings.TrimSpace(line[:idx])
	to = strings.TrimSpace(line[idx+len(operator):])
	if from == "" || to == "" {
		return "", "", "", false
	}
	return from, to, operator, true
}

// FindSequenceArrow is the single Mermaid sequence-operator table shared by
// semantic parsing and terminal rendering. It selects the first operator in
// source order and the longest operator at that byte position, so activation
// suffixes are not mistaken for participant bytes.
func FindSequenceArrow(line string) (int, string) {
	idx := -1
	operator := ""
	for _, candidate := range sequenceArrowOperators {
		at := strings.Index(line, candidate)
		if at < 0 || (idx >= 0 && at > idx) || (at == idx && len(candidate) <= len(operator)) {
			continue
		}
		idx = at
		operator = candidate
	}
	return idx, operator
}

// SequenceArrowBase removes the activation/deactivation suffix while keeping
// the structural arrow kind. Evidence consumers use this when reply semantics
// depend on `-->>`, while renderers retain the full operator.
func SequenceArrowBase(operator string) string {
	operator = strings.TrimSpace(operator)
	if len(operator) > 1 && (operator[len(operator)-1] == '+' || operator[len(operator)-1] == '-') {
		return operator[:len(operator)-1]
	}
	return operator
}

// splitSequenceEdgeTargetMessage separates the message delimiter in a
// sequence edge target such as `Service: call(arg)`. It deliberately runs
// only for sequenceDiagram bodies and ignores namespace separators (`::`).
// Flowchart labels are never scanned for `:`: Rust/C++/Ruby/Cangjie callable
// identities and ordinary `key: value` node labels must remain byte-exact.
func splitSequenceEdgeTargetMessage(raw string) (target, message string, ok bool) {
	for i := 0; i < len(raw); i++ {
		if raw[i] != ':' {
			continue
		}
		if (i > 0 && raw[i-1] == ':') || (i+1 < len(raw) && raw[i+1] == ':') {
			continue
		}
		target = strings.TrimSpace(raw[:i])
		message = strings.TrimSpace(raw[i+1:])
		if target == "" || message == "" {
			return raw, "", false
		}
		return target, message, true
	}
	return raw, "", false
}

// SplitEdgeLine attempts to split one Mermaid statement into (from, to)
// on the first arrow operator it finds. It also captures the first
// pipe-delimited label, as in A -->|label| B.
func SplitEdgeLine(line string) (string, string, string, string, bool) {
	operators := []string{
		"-->>", "-.->", "-->", "==>", "->>", "---", "==", "->",
	}
	idx, op := findFlowchartEdgeOperator(line, operators)
	if idx >= 0 {
		from := strings.TrimSpace(line[:idx])
		to := strings.TrimSpace(line[idx+len(op):])
		pipeLabel := ""
		if strings.HasPrefix(to, "|") {
			if end := strings.Index(to[1:], "|"); end >= 0 {
				pipeLabel = strings.TrimSpace(to[1 : end+1])
				to = strings.TrimSpace(to[end+2:])
			}
		}
		if from == "" || to == "" {
			return "", "", "", "", false
		}
		to = strings.SplitN(to, ";", 2)[0]
		to = strings.SplitN(to, ":::", 2)[0]
		to = strings.TrimSpace(to)
		if strings.Contains(to, "-->") || strings.Contains(to, "==>") || strings.Contains(to, "->>") {
			f2, t2, innerLabel, innerOp, ok := SplitEdgeLine(to)
			if ok {
				if innerLabel != "" {
					return f2, t2, innerLabel, innerOp, true
				}
				return f2, t2, pipeLabel, innerOp, true
			}
		}
		return from, to, pipeLabel, op, true
	}
	return "", "", "", "", false
}

// findFlowchartEdgeOperator finds an edge operator only in Mermaid syntax,
// never inside a node shape or quoted label. Code identities such as
// `sink_->write` are routinely rendered inside labels, and markdown
// normalization may add `<br/>`; treating those presentation bytes as an edge
// invents endpoints that do not exist in the model-authored graph.
func findFlowchartEdgeOperator(line string, operators []string) (int, string) {
	protected := make([]bool, len(line))
	var quote byte
	escaped := false
	depthSquare, depthParen, depthBrace := 0, 0, 0
	for i := 0; i < len(line); i++ {
		ch := line[i]
		insideShape := depthSquare > 0 || depthParen > 0 || depthBrace > 0
		if quote != 0 {
			protected[i] = true
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
		if ch == '\'' || ch == '"' || ch == '`' {
			protected[i] = true
			quote = ch
			continue
		}
		if insideShape {
			protected[i] = true
		}
		switch ch {
		case '[':
			depthSquare++
			protected[i] = true
		case ']':
			if depthSquare > 0 {
				depthSquare--
				protected[i] = true
			}
		case '(':
			depthParen++
			protected[i] = true
		case ')':
			if depthParen > 0 {
				depthParen--
				protected[i] = true
			}
		case '{':
			depthBrace++
			protected[i] = true
		case '}':
			if depthBrace > 0 {
				depthBrace--
				protected[i] = true
			}
		}
	}

	bestAt := -1
	bestOp := ""
	for _, op := range operators {
		for search := 0; search <= len(line)-len(op); {
			rel := strings.Index(line[search:], op)
			if rel < 0 {
				break
			}
			at := search + rel
			visible := true
			for i := at; i < at+len(op); i++ {
				if protected[i] {
					visible = false
					break
				}
			}
			if visible && (bestAt < 0 || at < bestAt || (at == bestAt && len(op) > len(bestOp))) {
				bestAt, bestOp = at, op
			}
			search = at + 1
		}
	}
	return bestAt, bestOp
}

// StripNodeShape collapses a node declaration with a shape wrapper to
// its identifier.
func StripNodeShape(tok string) string {
	id, _ := ParseNodeToken(tok)
	return id
}

func ParseNodeToken(tok string) (id, label string) {
	id, label, _ = ParseNodeTokenWithShape(tok)
	return id, label
}

func ParseNodeTokenWithShape(tok string) (id, label, shape string) {
	t := strings.TrimSpace(tok)
	if t == "" {
		return "", "", ""
	}
	if i := strings.Index(t, ":::"); i > 0 {
		t = strings.TrimSpace(t[:i])
	}
	cutAt := -1
	for i, r := range t {
		if r == '[' || r == '(' || r == '{' || r == '>' {
			cutAt = i
			break
		}
	}
	if cutAt > 0 {
		if t[cutAt] == '{' {
			shape = NodeShapeDecision
		}
		label = strings.TrimSpace(ExtractNodeLabel(t, cutAt))
		t = strings.TrimSpace(t[:cutAt])
	}
	t = strings.TrimPrefix(t, "&")
	return t, label, shape
}

func ExtractNodeLabel(tok string, openerAt int) string {
	if openerAt < 0 || openerAt >= len(tok) {
		return ""
	}
	open := tok[openerAt]
	close := byte(0)
	switch open {
	case '[':
		close = ']'
	case '(':
		close = ')'
	case '{':
		close = '}'
	case '>':
		close = ']'
	default:
		return ""
	}
	rest := tok[openerAt+1:]
	end := strings.LastIndexByte(rest, close)
	if end < 0 {
		return ""
	}
	label := strings.TrimSpace(rest[:end])
	label = strings.Trim(label, `"'`)
	return strings.TrimSpace(label)
}

func SequenceParticipantDeclarations(line string) []NodeDecl {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return nil
	}
	rest := ""
	switch {
	case strings.HasPrefix(trimmed, "participant "):
		rest = strings.TrimSpace(strings.TrimPrefix(trimmed, "participant "))
	case strings.HasPrefix(trimmed, "actor "):
		rest = strings.TrimSpace(strings.TrimPrefix(trimmed, "actor "))
	default:
		return nil
	}
	if rest == "" {
		return nil
	}
	if idx := strings.Index(rest, " as "); idx > 0 {
		ident := strings.TrimSpace(rest[:idx])
		label := strings.TrimSpace(rest[idx+4:])
		if ident == "" && label == "" {
			return nil
		}
		return []NodeDecl{{Ident: ident, Label: strings.Trim(label, `"'`)}}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return nil
	}
	ident := strings.TrimSpace(fields[0])
	if ident == "" {
		return nil
	}
	return []NodeDecl{{Ident: ident, Label: ident}}
}

// NodeDeclarationsAll walks a line and returns every node declaration it
// can recognise. A statement like A[Label A] --> B[Label B] produces two
// declarations.
func NodeDeclarationsAll(line string) []NodeDecl {
	openers := []struct{ open, close string }{
		{"[\"", "\"]"},
		{"((", "))"},
		{"{{", "}}"},
		{"(\"", "\")"},
		{"[", "]"},
		{"(", ")"},
		{"{", "}"},
		{">", "]"},
	}
	var out []NodeDecl
	cursor := 0
	for cursor < len(line) {
		bestPos := -1
		var bestOpener struct{ open, close string }
		for _, op := range openers {
			pos := strings.Index(line[cursor:], op.open)
			if pos < 0 {
				continue
			}
			absPos := cursor + pos
			if bestPos < 0 || absPos < bestPos {
				bestPos = absPos
				bestOpener = op
			}
		}
		if bestPos < 0 {
			break
		}
		identStart := bestPos
		for identStart > cursor {
			r := line[identStart-1]
			if r == ' ' || r == '\t' || r == '>' || r == ']' || r == ')' || r == '}' || r == '|' {
				break
			}
			identStart--
		}
		ident := strings.TrimSpace(line[identStart:bestPos])
		if ident == "" || strings.ContainsAny(ident, "-=>") {
			cursor = bestPos + len(bestOpener.open)
			continue
		}
		labelStart := bestPos + len(bestOpener.open)
		closeRel := strings.Index(line[labelStart:], bestOpener.close)
		if closeRel < 0 {
			cursor = bestPos + len(bestOpener.open)
			continue
		}
		labelEnd := labelStart + closeRel
		label := strings.TrimSpace(line[labelStart:labelEnd])
		label = strings.Trim(label, "\"'")
		out = append(out, NodeDecl{Ident: ident, Label: label})
		cursor = labelEnd + len(bestOpener.close)
	}
	return out
}
