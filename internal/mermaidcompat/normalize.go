package mermaidcompat

import "strings"

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
