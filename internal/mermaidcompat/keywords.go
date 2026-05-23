package mermaidcompat

import "strings"

var supportedKeywords = []string{
	"flowchart",
	"graph",
	"sequenceDiagram",
}

var unsupportedKeywords = []string{
	"classDiagram",
	"stateDiagram",
	"stateDiagram-v2",
	"erDiagram",
	"journey",
	"gantt",
	"pie",
	"mindmap",
	"timeline",
	"gitGraph",
	"requirementDiagram",
	"C4Context",
}

var allKeywords = append(append([]string(nil), supportedKeywords...), unsupportedKeywords...)

// SupportedKeywords returns the Mermaid syntax families that the terminal
// ASCII renderer can draw directly.
func SupportedKeywords() []string {
	return append([]string(nil), supportedKeywords...)
}

// UnsupportedKeywords returns Mermaid syntax families that are valid Mermaid
// but outside the terminal ASCII renderer subset. Browser preview can still
// hand these to Mermaid.js.
func UnsupportedKeywords() []string {
	return append([]string(nil), unsupportedKeywords...)
}

// KnownKeywords returns every Mermaid syntax family Codrax routes as a
// diagram-like fence.
func KnownKeywords() []string {
	return append([]string(nil), allKeywords...)
}

// IsSupportedKeyword reports whether kw belongs to the terminal renderer
// subset.
func IsSupportedKeyword(kw string) bool {
	for _, s := range supportedKeywords {
		if s == kw {
			return true
		}
	}
	return false
}

// FirstKeywordIn returns the known Mermaid syntax keyword the line begins with.
// Matching is boundary-aware so "graph" does not shadow "graphTD".
func FirstKeywordIn(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	for _, kw := range allKeywords {
		if line == kw || strings.HasPrefix(line, kw+" ") || strings.HasPrefix(line, kw+"\t") {
			return kw
		}
	}
	return ""
}

// InfoLineDirective peels a Mermaid directive out of a fence info string.
// It accepts both "mermaid flowchart TD" and direct "flowchart TD" forms.
func InfoLineDirective(info string) (directive string, keyword string) {
	info = strings.TrimSpace(info)
	if info == "" {
		return "", ""
	}
	first := firstToken(info)
	switch {
	case strings.EqualFold(first, "mermaid"):
		rest := strings.TrimSpace(info[len(first):])
		if rest == "" {
			return "", ""
		}
		kw := FirstKeywordIn(rest)
		if kw == "" {
			return "", ""
		}
		return rest, kw
	default:
		kw := FirstKeywordIn(info)
		if kw == "" {
			return "", ""
		}
		return info, kw
	}
}

// InfoLineStartsWithKeyword reports whether the whole fence info string is a
// Mermaid directive such as "flowchart TD" or "classDiagram".
func InfoLineStartsWithKeyword(info string) bool {
	_, kw := InfoLineDirective(info)
	return kw != "" && !strings.EqualFold(firstToken(info), "mermaid")
}

// LooksLikeBody reports whether the first non-empty body line begins with a
// known Mermaid syntax family.
func LooksLikeBody(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return FirstKeywordIn(line) != ""
	}
	return false
}

func firstToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
