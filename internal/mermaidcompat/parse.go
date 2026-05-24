package mermaidcompat

import "strings"

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
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") || strings.HasPrefix(line, "classDef") || strings.HasPrefix(line, "click") {
			continue
		}
		var seqLabel string
		if idx := strings.Index(line, ":"); idx > 0 {
			seqLabel = strings.TrimSpace(line[idx+1:])
			line = strings.TrimSpace(line[:idx])
		}
		from, to, label, operator, ok := SplitEdgeLine(line)
		if !ok {
			continue
		}
		if label == "" {
			label = seqLabel
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

// SplitEdgeLine attempts to split one Mermaid statement into (from, to)
// on the first arrow operator it finds. It also captures the first
// pipe-delimited label, as in A -->|label| B.
func SplitEdgeLine(line string) (string, string, string, string, bool) {
	var pipeLabel string
	for {
		i := strings.Index(line, "|")
		if i < 0 {
			break
		}
		j := strings.Index(line[i+1:], "|")
		if j < 0 {
			break
		}
		if pipeLabel == "" {
			pipeLabel = strings.TrimSpace(line[i+1 : i+1+j])
		}
		line = line[:i] + " " + line[i+1+j+1:]
	}
	operators := []string{
		"-->>", "-.->", "-->", "==>", "->>", "---", "==", "->",
	}
	for _, op := range operators {
		idx := strings.Index(line, op)
		if idx < 0 {
			continue
		}
		from := strings.TrimSpace(line[:idx])
		to := strings.TrimSpace(line[idx+len(op):])
		if from == "" || to == "" {
			continue
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
