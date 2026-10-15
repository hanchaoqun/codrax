package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
)

// This editor reads only Mermaid grammar positions. Text after a Note/message
// colon, declaration labels and control-region descriptions are opaque bytes.
// The returned line map keeps native Note selections on the same statement.
func rewriteNativeSequenceParticipants(body string, replacements map[string]string) (string, map[int]int, error) {
	// The catalog may contain current native lanes that this diagram never
	// uses. Only an existing syntax-level source creates an actual merge.
	present := map[string]bool{}
	for _, d := range mermaidcompat.SequenceParticipantDeclarations(body) {
		present[d.Ident] = true
	}
	for _, e := range mermaidcompat.ParseEdges(body) {
		present[e.From], present[e.To] = true, true
	}
	for _, ref := range mermaidcompat.SequenceParticipantReferences(body) {
		present[ref] = true
	}
	merging := map[string]bool{}
	for from, to := range replacements {
		if from != to && present[from] {
			merging[from], merging[to] = true, true
		}
	}
	var out []string
	lineMap := map[int]int{}
	for i, raw := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(raw)
		decls := mermaidcompat.SequenceParticipantDeclarations(trimmed)
		if len(decls) == 1 && replacements[decls[0].Ident] != "" {
			continue
		}
		line := raw
		edges := mermaidcompat.ParseEdges("sequenceDiagram\n" + raw)
		if len(edges) == 1 {
			e := edges[0]
			// Inline activation belongs to the receiver; deactivation belongs
			// to the sender. Sharing either affected lifetime is not a pure
			// display edit, including when it is on the retained representative.
			if strings.HasSuffix(e.Operator, "+") && merging[e.To] || strings.HasSuffix(e.Operator, "-") && merging[e.From] {
				return "", nil, fmt.Errorf("message lifecycle at line %d requires an explicit diagram edit before sharing a lane", i+1)
			}
			if replacements[e.From] != "" || replacements[e.To] != "" {
				op := strings.Index(raw, e.Operator)
				if op < 0 || strings.TrimSpace(raw[:op]) != e.From {
					return "", nil, fmt.Errorf("cannot losslessly rewrite message at line %d", i+1)
				}
				rest := raw[op+len(e.Operator):]
				colon := strings.IndexByte(rest, ':')
				target := rest
				if colon >= 0 {
					target = rest[:colon]
				}
				if strings.TrimSpace(target) != e.To {
					return "", nil, fmt.Errorf("cannot losslessly rewrite message target at line %d", i+1)
				}
				from := rewriteNativeSequenceToken(raw[:op], e.From, replacements)
				to := rewriteNativeSequenceToken(target, e.To, replacements)
				line = from + e.Operator + to
				if colon >= 0 {
					line += rest[colon:]
				}
			}
		} else if refs := mermaidcompat.SequenceParticipantReferences("sequenceDiagram\n" + raw); len(refs) > 0 {
			// All supported non-message references are before the first colon.
			// Lifecycle statements cannot be coalesced: create/destroy and active
			// lifetimes are distinct semantics, not a display label operation.
			header, payload := raw, ""
			if colon := strings.IndexByte(raw, ':'); colon >= 0 {
				header, payload = raw[:colon], raw[colon:]
			}
			for _, ref := range refs {
				if !merging[ref] {
					continue
				}
				lower := strings.ToLower(strings.TrimSpace(header))
				if !strings.HasPrefix(lower, "note ") {
					return "", nil, fmt.Errorf("participant lifecycle/metadata at line %d requires an explicit diagram edit before sharing a lane", i+1)
				}
			}
			// Replace all exact reference tokens in one pass, so a selected alias
			// cannot be substituted twice through another mapping.
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "note ") {
				var err error
				header, err = rewriteNativeSequenceNoteHeader(header, refs, replacements)
				if err != nil {
					return "", nil, fmt.Errorf("line %d: %w", i+1, err)
				}
			}
			line = header + payload
		} else if len(decls) == 0 && !nativeSequenceSharingTransparentLine(trimmed) {
			return "", nil, fmt.Errorf("sequence statement at line %d is not safely covered by participant sharing; keep separate lanes or explicitly edit this diagram", i+1)
		}
		out = append(out, line)
		lineMap[i+1] = len(out)
	}
	return strings.Join(out, "\n"), lineMap, nil
}

func rewriteNativeSequenceToken(text, token string, replacements map[string]string) string {
	to := replacements[token]
	if to == "" {
		return text
	}
	start := strings.Index(text, token)
	return text[:start] + to + text[start+len(token):]
}

func rewriteNativeSequenceNoteHeader(header string, refs []string, replacements map[string]string) (string, error) {
	lower := strings.ToLower(header)
	start := strings.Index(lower, "note") + len("note")
	for start < len(header) && (header[start] == ' ' || header[start] == '\t') {
		start++
	}
	for _, prefix := range []string{"over ", "left of ", "right of "} {
		if !strings.HasPrefix(lower[start:], prefix) {
			continue
		}
		start += len(prefix)
		targets := strings.Split(header[start:], ",")
		seen := map[string]bool{}
		var retained []string
		for i, target := range targets {
			token := strings.TrimSpace(target)
			if !nativeParticipantNodeID.MatchString(token) {
				return "", fmt.Errorf("Note target has no exact rewritable identifier")
			}
			targets[i] = rewriteNativeSequenceToken(target, token, replacements)
			key := strings.TrimSpace(targets[i])
			if !seen[key] {
				retained = append(retained, targets[i])
				seen[key] = true
			}
		}
		return header[:start] + strings.Join(retained, ","), nil
	}
	return "", fmt.Errorf("Note participant syntax is unsupported for sharing")
}

func nativeSequenceSharingTransparentLine(line string) bool {
	if line == "" || strings.HasPrefix(line, "%%") {
		return true
	}
	keyword := strings.ToLower(strings.Fields(line)[0])
	switch keyword {
	case "sequencediagram", "autonumber", "alt", "else", "opt", "loop", "par", "and", "critical", "option", "break", "rect", "box", "end", "title":
		return true
	}
	return false
}
