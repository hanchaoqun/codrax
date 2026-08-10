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
		// Sequence participant/actor declarations are presentation metadata,
		// never message edges. Their visible labels may legitimately contain
		// language operators such as C/C++ `sink_->write`; looking for an arrow
		// before recognizing the declaration would mint a fictitious
		// `sink_ -> write` invocation and pollute typed endpoint identity.
		if sequenceBody && len(SequenceParticipantDeclarations(line)) > 0 {
			continue
		}
		if sequenceBody {
			// Sequence control/decorative statements may carry arbitrary display
			// text, including source-language arrows such as `A -> B`.  They do
			// not declare messages and therefore must not mint semantic edges.
			// Match only a complete leading grammar token so a participant named
			// `Note` can still emit the real message `Note->>A: call()`.
			if sequenceLineIsNonMessageDirective(line) {
				continue
			}
			from, to, operator, ok := splitSequenceEdgeLine(line)
			if !ok {
				continue
			}
			label := ""
			if target, message, found := splitSequenceEdgeTargetMessage(to); found {
				to = target
				label = message
			}
			edges = appendParsedEdge(edges, from, to, label, operator)
			continue
		}
		// Mermaid permits a compact flow chain such as A --> B --> C.  Each
		// hop is a visible relation and therefore has to survive as its own
		// parsed edge.  SplitEdgeLine intentionally retains its historical
		// single-edge API for normalizers and legacy callers; the evidence
		// parser uses the complete chain view here.
		for _, edge := range splitFlowchartEdgeChainLine(line) {
			edges = appendParsedEdge(edges, edge.from, edge.to, edge.label, edge.operator)
		}
	}
	return edges
}

// sequenceLineIsNonMessageDirective identifies Mermaid sequence statements
// whose remaining bytes are presentation/control payload rather than a
// participant-to-participant message.  It is deliberately syntax-only: no
// user wording, model prose, source language, or endpoint name participates in
// the decision.  A keyword is recognized only at a token boundary (end of line
// or ASCII whitespace), preserving messages sent by participants whose names
// happen to equal a directive.
func sequenceLineIsNonMessageDirective(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	for _, keyword := range []string{
		"note",
		"loop", "alt", "else", "opt", "par", "and", "critical", "option", "break", "rect", "end",
		"activate", "deactivate", "autonumber",
		"box", "link", "links", "properties", "details", "title",
		"create", "destroy",
	} {
		if lower == keyword {
			return true
		}
		if len(lower) > len(keyword) && lower[:len(keyword)] == keyword && isASCIIWhitespace(lower[len(keyword)]) {
			return true
		}
	}
	return false
}

func isASCIIWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' || ch == '\f' || ch == '\v'
}

type flowchartEdgeTokens struct {
	from     string
	to       string
	label    string
	operator string
}

func appendParsedEdge(edges []Edge, from, to, label, operator string) []Edge {
	var fromLabel, toLabel, fromShape, toShape string
	from, fromLabel, fromShape = ParseNodeTokenWithShape(from)
	to, toLabel, toShape = ParseNodeTokenWithShape(to)
	if from == "" || to == "" {
		return edges
	}
	return append(edges, Edge{
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

// splitFlowchartEdgeChainLine decomposes one Mermaid flow statement into all
// of its directed hops. It is syntax-only: quoted/node-shape arrow bytes stay
// protected by findFlowchartEdgeOperator, and an unprotected semicolon ends
// the statement instead of accidentally bridging two independent statements.
func splitFlowchartEdgeChainLine(line string) []flowchartEdgeTokens {
	operators := []string{"-->>", "-.->", "-->", "==>", "->>", "---", "==", "->"}
	idx, operator := findFlowchartEdgeOperator(line, operators)
	if idx < 0 {
		return nil
	}
	from := strings.TrimSpace(line[:idx])
	tail := strings.TrimSpace(line[idx+len(operator):])
	if from == "" || tail == "" {
		return nil
	}

	var out []flowchartEdgeTokens
	for from != "" && tail != "" {
		label := ""
		if strings.HasPrefix(tail, "|") {
			if end := strings.Index(tail[1:], "|"); end >= 0 {
				label = strings.TrimSpace(tail[1 : end+1])
				tail = strings.TrimSpace(tail[end+2:])
			}
		}
		nextAt, nextOperator := findFlowchartEdgeOperator(tail, operators)
		semicolonAt := findFlowchartStatementSeparator(tail)
		if semicolonAt >= 0 && (nextAt < 0 || semicolonAt < nextAt) {
			to := strings.TrimSpace(tail[:semicolonAt])
			if to != "" {
				out = append(out, flowchartEdgeTokens{from: from, to: to, label: label, operator: operator})
			}
			break
		}
		if nextAt < 0 {
			to := strings.TrimSpace(tail)
			if to != "" {
				out = append(out, flowchartEdgeTokens{from: from, to: to, label: label, operator: operator})
			}
			break
		}
		to := strings.TrimSpace(tail[:nextAt])
		if to == "" {
			break
		}
		out = append(out, flowchartEdgeTokens{from: from, to: to, label: label, operator: operator})
		from = to
		operator = nextOperator
		tail = strings.TrimSpace(tail[nextAt+len(nextOperator):])
	}
	return out
}

func findFlowchartStatementSeparator(line string) int {
	var quote byte
	escaped := false
	depthSquare, depthParen, depthBrace := 0, 0, 0
	for i := 0; i < len(line); i++ {
		ch := line[i]
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
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		switch ch {
		case '[':
			depthSquare++
		case ']':
			if depthSquare > 0 {
				depthSquare--
			}
		case '(':
			depthParen++
		case ')':
			if depthParen > 0 {
				depthParen--
			}
		case '{':
			depthBrace++
		case '}':
			if depthBrace > 0 {
				depthBrace--
			}
		case ';':
			if depthSquare == 0 && depthParen == 0 && depthBrace == 0 {
				return i
			}
		}
	}
	return -1
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
	// Mermaid flowcharts also permit a node to be declared by one bare,
	// syntax-safe identifier on its own statement line. This form matters for
	// honest disconnected participants: requiring a shape solely so another
	// subsystem can observe the node would make our accepted Mermaid grammar
	// narrower than the renderer's. Keep this exact and non-semantic — it
	// contributes a visible node declaration only, never an edge or relation.
	if len(out) == 0 {
		if decl, ok := standaloneFlowNodeDeclaration(line); ok {
			out = append(out, decl)
		}
	}
	return out
}

func standaloneFlowNodeDeclaration(line string) (NodeDecl, bool) {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	if trimmed == "" || strings.ContainsAny(trimmed, " \t\r\n") || !flowchartNodeIDIsSafe(trimmed) {
		return NodeDecl{}, false
	}
	// These tokens own statement grammar even when a malformed/partial line
	// contains no arguments. Do not let a control/header statement discharge
	// a participant-presence obligation by masquerading as a node.
	switch strings.ToLower(trimmed) {
	case "flowchart", "graph", "sequencediagram", "end", "subgraph", "direction",
		"classdef", "class", "style", "click", "linkstyle":
		return NodeDecl{}, false
	}
	return NodeDecl{Ident: trimmed, Label: trimmed}, true
}
