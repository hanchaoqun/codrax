package mermaidcompat

import "strings"

// NormalizeSequenceMultilineNotes repairs physical line breaks in a Note's
// display text. Only adjacent plain-text continuations at the same or deeper
// indentation are folded. Messages, declarations, control blocks, comments,
// blank lines and ambiguous syntax end the continuation. No participants,
// relations, labels or measurements are inferred or corrected here.
func NormalizeSequenceMultilineNotes(body string) string {
	if !isSequenceDiagram(body) {
		return body
	}
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	changed := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		out = append(out, line)
		if !sequenceNoteHasDisplayText(strings.TrimSpace(line)) {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		for i+1 < len(lines) {
			next := lines[i+1]
			text := strings.TrimSpace(next)
			if !strings.HasPrefix(next, indent) || !sequenceNotePlainContinuation(text) {
				break
			}
			out[len(out)-1] = strings.TrimRight(out[len(out)-1], " \t\r") + "<br/>" + text
			i++
			changed = true
		}
	}
	if !changed {
		return body
	}
	return strings.Join(out, "\n")
}

func sequenceNoteHasDisplayText(line string) bool {
	colon := strings.IndexByte(line, ':')
	if colon < 0 || strings.TrimSpace(line[colon+1:]) == "" {
		return false
	}
	header := strings.TrimSpace(line[:colon])
	lower := strings.ToLower(header)
	for _, prefix := range []string{"note over ", "note left of ", "note right of "} {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		targets := strings.Split(strings.TrimSpace(header[len(prefix):]), ",")
		if len(targets) > 2 || (prefix != "note over " && len(targets) != 1) {
			return false
		}
		for _, target := range targets {
			if !safeExplicitSequenceNodeIdentifier(strings.TrimSpace(target)) {
				return false
			}
		}
		return true
	}
	return false
}

func sequenceNotePlainContinuation(text string) bool {
	if text == "" || strings.HasPrefix(text, "%%") ||
		sequenceLineIsNonMessageDirective(text) ||
		len(SequenceParticipantDeclarations(text)) > 0 ||
		strings.ContainsAny(text, ";{}[]`:") ||
		FirstKeywordIn(text) != "" || strings.EqualFold(text, "stop") {
		return false
	}
	if at, _ := FindSequenceArrow(text); at >= 0 {
		return false
	}
	if at, _ := FindFlowchartArrow(text); at >= 0 {
		return false
	}
	return true
}
