package mermaidcompat

import (
	"regexp"
	"strings"
)

var markdownFenceRe = regexp.MustCompile("(?s)```([^\n]*)\n(.*?)\n```")

// NormalizeMarkdownMermaidFences applies the source-level Mermaid repair pass
// to Markdown fenced code blocks before they are persisted or handed to a full
// Mermaid renderer. It is intentionally conservative: non-Mermaid fences stay
// byte-identical, while explicit Mermaid fences and diagram-shaped bare/text
// fences are rewritten as standard ```mermaid fences with normalized source.
func NormalizeMarkdownMermaidFences(markdown string) string {
	if markdown == "" || !strings.Contains(markdown, "```") || !markdownMayContainMermaid(markdown) {
		return markdown
	}
	return markdownFenceRe.ReplaceAllStringFunc(markdown, normalizeMarkdownMermaidFence)
}

func normalizeMarkdownMermaidFence(match string) string {
	nl := strings.Index(match, "\n")
	bodyEnd := strings.LastIndex(match, "\n```")
	if nl < 0 || bodyEnd <= nl {
		return match
	}
	info := strings.TrimSpace(match[3:nl])
	body := match[nl+1 : bodyEnd]
	normalized, ok := normalizeMarkdownMermaidBody(info, body)
	if !ok {
		return match
	}
	normalized = strings.TrimRight(normalized, "\n")
	return "```mermaid\n" + normalized + "\n```"
}

func normalizeMarkdownMermaidBody(info, body string) (string, bool) {
	firstInfo := firstToken(info)
	switch {
	case strings.EqualFold(firstInfo, "mermaid"):
		body = prependMarkdownMermaidInfoDirective(info, body)
		return NormalizeSourceForMarkdown(body), true
	case InfoLineStartsWithKeyword(info):
		body = prependMarkdownMermaidInfoDirective(info, body)
		return NormalizeSourceForMarkdown(body), true
	case (info == "" || strings.EqualFold(info, "text")) && LooksLikeBody(body):
		return NormalizeSourceForMarkdown(body), true
	default:
		return "", false
	}
}

func prependMarkdownMermaidInfoDirective(info, body string) string {
	directive, _ := InfoLineDirective(info)
	if directive == "" {
		return body
	}
	directiveToken := firstToken(directive)
	bodyToken := firstToken(firstNonEmptyLine(body))
	if directiveToken == "" || strings.EqualFold(directiveToken, bodyToken) {
		return body
	}
	if strings.TrimSpace(body) == "" {
		return directive
	}
	return directive + "\n" + body
}

func markdownMayContainMermaid(markdown string) bool {
	if strings.Contains(markdown, "```mermaid") {
		return true
	}
	for _, kw := range KnownKeywords() {
		if strings.Contains(markdown, kw) {
			return true
		}
	}
	return false
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
