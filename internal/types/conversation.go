package types

import (
	"regexp"
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
	"/q":        "/quit",
	"/quit":     "/quit",
	"/exit":     "/exit",
	"/clear":    "/clear",
	"/history":  "/history",
	"/compact":  "/compact",
	"/help":     "/help",
	"/h":        "/help",
	"\\q":       "/quit",
	"\\quit":    "/quit",
	"\\exit":    "/exit",
	"\\clear":   "/clear",
	"\\history": "/history",
	"\\compact": "/compact",
	"\\help":    "/help",
	"\\h":       "/help",
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

// continuationPrefixes are leading tokens that strongly suggest the
// current request is a continuation of the prior conversation rather
// than a new self-contained question. Matched case-insensitively as a
// whole-word prefix after leading punctuation/whitespace trimming.
//
// Chinese list covers the common connectives a native speaker uses to
// say "more on X" / "next question about X" / "follow up" without
// restating the subject. English list mirrors the same semantic
// categories. Items are grep-visible so adding dialect variants is a
// one-line change.
var continuationPrefixes = []string{
	// Chinese connectives + demonstratives used to refer to prior
	"再", "继续", "接着", "还有", "详细", "细节", "进一步", "深入",
	"那", "那么", "上面", "刚才", "刚刚",
	// English phrasings
	"more on", "more about", "detail", "continue", "also",
	"what about", "how about", "elaborate", "expand", "further",
	"tell me more", "go deeper", "dig deeper", "and what",
	"and how", "follow up", "next,",
}

// pronounOnlyHead matches a leading pronoun or demonstrative without
// a sibling identifier in the first window of the query. A query
// headed by "it" / "this" / "那" / "它" that lacks any CamelCase or
// snake_case identifier token in the first 40 runes is unresolvable
// without the prior conversation — we flag it as a continuation.
//
// Chinese and English branches are separate because Go's RE2 regexp
// treats non-ASCII characters as non-word, so \b (ASCII word
// boundary) does not fire between two CJK characters. The Chinese
// alternation lists longer prefixes first (这个 before 这) so the
// left-to-right picker does not short-circuit on a prefix match.
var pronounOnlyHead = regexp.MustCompile(`^(?i)(这个|那个|它们|他们|她们|它|他|她|这|那|上面|it\b|this\b|that\b|they\b|them\b)`)

// identifierTokenInWindow looks for CamelCase / snake_case / dotted
// identifier tokens in the first 40 runes of the query. Presence of
// such a token means the query names its own subject and does not
// rely on Prior Conversation resolution.
var identifierTokenInWindow = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*[._][A-Za-z0-9_]+|[A-Z][a-z]+[A-Z][A-Za-z0-9_]*|[a-z]+_[a-z_0-9]+`)

// IsContinuation reports whether current is a continuation of prior
// conversation rather than a new self-contained question. Returns
// false when prior is empty (single-shot mode) or when current is
// clearly self-standing (names its own identifiers, no continuation
// prefix, no unresolved pronoun). The heuristic is deliberately
// lenient: when in doubt, return false so downstream stages get the
// cleaner "code is the source of truth" signal instead of inheriting
// a potentially-wrong prior answer.
//
// The rules (OR-ed; any one matching returns true):
//
//  1. Leading continuation prefix — current (lowercased, leading
//     whitespace/punctuation stripped) starts with a token from
//     continuationPrefixes. Catches "再详细讲一下 X" / "more on X" /
//     "那它的 Y 是什么" / "continue with Z".
//
//  2. Leading pronoun without sibling identifier — first 40 runes of
//     current match pronounOnlyHead AND do not contain an
//     identifier-shaped token. Catches bare "它是怎么工作的" where the
//     subject is only knowable from Prior. Single-word pronouns with
//     an explicit CamelCase or dotted identifier ("they call
//     Foo.Bar") are NOT continuations — they name their own subject.
//
// Both rules intentionally skip prose like "explorer 是怎么调用 X" —
// that is a self-contained question even if it echoes a prior one.
// Identical-topic repeats without a continuation cue are treated as
// fresh questions; this is the desired behaviour because a user who
// wants to continue will use one of the cue words.
func IsContinuation(current, prior string) bool {
	prior = strings.TrimSpace(prior)
	current = strings.TrimSpace(current)
	if prior == "" || current == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimLeft(current, " \t\n.,?!，。？！、：:"))
	for _, pfx := range continuationPrefixes {
		if strings.HasPrefix(lower, strings.ToLower(pfx)) {
			return true
		}
	}
	head := current
	if len([]rune(head)) > 40 {
		head = string([]rune(head)[:40])
	}
	if pronounOnlyHead.MatchString(head) && !identifierTokenInWindow.MatchString(head) {
		return true
	}
	return false
}
