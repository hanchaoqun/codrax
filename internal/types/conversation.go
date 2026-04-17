package types

import "strings"

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
