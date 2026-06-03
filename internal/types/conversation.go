package types

import (
	"strings"
)

// ConversationCurrentMarker is the delimiter the REPL injects between the
// prior-conversation memory block and the current user request. See
// internal/repl/repl.go:dispatch for the producer.
const ConversationCurrentMarker = "## Current request\n"

// StripConversationPrefix returns only the "current request" portion of a
// REPL-assembled Objective string, stripping the conversation-memory prefix.
// When no marker is found (single-shot mode, or an empty-conversation REPL
// turn), returns the input unchanged.
//
// This is the single source of truth for REPL prefix stripping; both the
// analyzer's TermGraph normalisation and the explorer's ERM entity
// extraction route through it so the prior conversation never contaminates
// entity/keyword ranking.
func StripConversationPrefix(s string) string {
	if idx := strings.Index(s, ConversationCurrentMarker); idx >= 0 {
		return strings.TrimSpace(s[idx+len(ConversationCurrentMarker):])
	}
	return s
}

// SplitConversation splits a REPL-assembled Objective into (prior, current).
// When the marker is absent (single-shot mode) prior is empty and current is
// the input verbatim. When present, prior is everything before the marker
// with surrounding whitespace trimmed, current is everything after with
// surrounding whitespace trimmed.
func SplitConversation(s string) (prior, current string) {
	idx := strings.Index(s, ConversationCurrentMarker)
	if idx < 0 {
		return "", s
	}
	prior = strings.TrimSpace(s[:idx])
	current = strings.TrimSpace(s[idx+len(ConversationCurrentMarker):])
	return prior, current
}

// replCommandAliases maps the REPL's accepted slash and backslash
// command spellings onto the canonical slash form that handleSlash
// consumes. Keeping this in types lets the REPL, orchestrator, and
// analyzer share one definition of "this line is a control input, not
// a code question".
var replCommandAliases = map[string]string{
	"/q":          "/quit",
	"/quit":       "/quit",
	"/exit":       "/exit",
	"/clear":      "/clear",
	"/history":    "/history",
	"/compact":    "/compact",
	"/help":       "/help",
	"/h":          "/help",
	"/log":        "/log",
	"/htrace":     "/htrace",
	"/atrace":     "/htrace", // alias — Android-flavored spelling
	"/paste":      "/paste",
	"/version":    "/version",
	"/v":          "/version",
	"/chat":       "/chat",
	"/mode":       "/mode",
	"/plan":       "/plan",
	"/approve":    "/approve",
	"/reject":     "/reject",
	"/operation":  "/operation",
	"/verify":     "/verify",
	"/worktree":   "/worktree",
	"/merge":      "/merge",
	"/branch":     "/branch",
	"/env":        "/env",
	"\\env":       "/env",
	"/baseline":   "/baseline",
	"\\baseline":  "/baseline",
	"/phase":      "/phase",
	"\\phase":     "/phase",
	"/pitfalls":   "/pitfalls",
	"\\pitfalls":  "/pitfalls",
	"/mermaid":    "/mermaid",
	"\\mermaid":   "/mermaid",
	"/cancel":     "/cancel",
	"\\cancel":    "/cancel",
	"/repos":      "/repos",
	"\\repos":     "/repos",
	"/workflow":   "/workflow",
	"\\q":         "/quit",
	"\\quit":      "/quit",
	"\\exit":      "/exit",
	"\\clear":     "/clear",
	"\\history":   "/history",
	"\\compact":   "/compact",
	"\\help":      "/help",
	"\\h":         "/help",
	"\\log":       "/log",
	"\\htrace":    "/htrace",
	"\\atrace":    "/htrace", // alias
	"\\paste":     "/paste",
	"\\version":   "/version",
	"\\v":         "/version",
	"\\chat":      "/chat",
	"\\mode":      "/mode",
	"\\plan":      "/plan",
	"\\approve":   "/approve",
	"\\reject":    "/reject",
	"\\operation": "/operation",
	"\\workflow":  "/workflow",
	"\\verify":    "/verify",
	"\\worktree":  "/worktree",
	"\\merge":     "/merge",
	"\\branch":    "/branch",
}

// NormalizeREPLCommandAlias returns the canonical slash-command form
// for a known REPL control input, preserving any trailing arguments.
// Returns "" when the line is not a known local REPL command.
func NormalizeREPLCommandAlias(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	cmd, ok := replCommandAliases[strings.ToLower(fields[0])]
	if !ok {
		return ""
	}
	if len(fields) == 1 {
		return cmd
	}
	rest := strings.TrimSpace(line[len(fields[0]):])
	if rest == "" {
		return cmd
	}
	return cmd + " " + rest
}

// IsREPLControlInput reports whether the line is a local REPL command
// such as `/quit` or `\q`. Control inputs are not analyzable user
// requests and should not inherit entities from Prior Conversation.
func IsREPLControlInput(line string) bool {
	return NormalizeREPLCommandAlias(line) != ""
}

// CanonicalREPLCommands returns the set of canonical slash commands
// (the target side of replCommandAliases). Callers that maintain a
// parallel list — notably internal/repl's autocomplete suggestions —
// use this to lint against drift. Returning a fresh slice each call
// so the caller cannot mutate the registry.
func CanonicalREPLCommands() []string {
	seen := map[string]bool{}
	for _, target := range replCommandAliases {
		seen[target] = true
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	return out
}

// IsContinuation is the LEGACY keyword-heuristic continuation
// detector. Pre-commit-48 it was the production decision; commit 48
// replaced it with the LLM continuation_classifier (see
// internal/orchestrator/continuation_classifier.go). The classifier
// has graceful fallback only when the adapter is unavailable, which
// in production never happens (cmd/root.go always wires it from
// providers.yaml :: agents.continuation_classifier, defaulting to
// the default LLM). User feedback (2026-05-01): the keyword fallback
// is dead code AND violates the no-keyword-table red line, so the
// path was simplified.
//
// Now: returns false unconditionally. The function is preserved as
// a stable signature for any external caller; downstream code should
// route through the orchestrator's classifyContinuationCached path.
func IsContinuation(current, prior string) bool {
	prior = strings.TrimSpace(prior)
	current = strings.TrimSpace(current)
	if prior == "" || current == "" {
		return false
	}
	// All keyword-table + pronoun-regex rules removed. The LLM
	// classifier is authoritative; absent classifier defaults to
	// "fresh" (false) — the safe direction (over-isolating a real
	// continuation costs less than poisoning a fresh question with
	// stale prior-turn entities).
	return false
}
