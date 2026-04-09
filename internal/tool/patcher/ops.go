package patcher

import (
	"errors"
	"strings"
)

// applyWrite replaces file content entirely. Content is assumed to already be
// \n-normalized by the caller.
func applyWrite(before string, exists bool, content string) (after string, status Status, msg string) {
	if exists && before == content {
		return before, StatusUnchanged, "file content unchanged"
	}
	return content, StatusApplied, "file written"
}

// applyEdit replaces a unique occurrence of oldString with newString.
// It surfaces no_match, ambiguous_match, already_applied, and unchanged
// explicitly. All input strings are assumed to be \n-normalized.
func applyEdit(before, oldString, newString string) (string, Status, string, int, error) {
	if oldString == "" {
		return before, "", "", 0, errors.New("old_string is required for edit")
	}

	positions := findAllMatches(before, oldString)

	switch len(positions) {
	case 0:
		// Idempotency: the new content might already be present.
		if newString != "" && strings.Contains(before, newString) && !strings.Contains(before, oldString) {
			return before, StatusAlreadyApplied, "new_string already present; old_string not found", 0, nil
		}
		return before, StatusNoMatch, "old_string not found", 0, nil
	case 1:
		start := positions[0]
		end := start + len(oldString)
		after := before[:start] + newString + before[end:]
		if after == before {
			return before, StatusUnchanged, "content unchanged after edit", 1, nil
		}
		return after, StatusApplied, "edited successfully", 1, nil
	default:
		return before, StatusAmbiguous, "old_string matched more than once", len(positions), nil
	}
}

// applyInsert handles both insert_before and insert_after with a single
// implementation. The anchor is located by exact match, and idempotency is
// detected by checking whether the content is already adjacent to the anchor.
// All input strings are assumed to be \n-normalized.
func applyInsert(before, anchor, content string, insertBefore bool) (string, Status, string, int, error) {
	if anchor == "" {
		return before, "", "", 0, errors.New("old_string (anchor) is required for insert")
	}

	positions := findAllMatches(before, anchor)

	switch len(positions) {
	case 0:
		// Idempotency: content might already be inserted somewhere adjacent
		// to the anchor, which is why the anchor alone can no longer be found.
		var composite string
		if insertBefore {
			composite = content + anchor
		} else {
			composite = anchor + content
		}
		if content != "" && strings.Contains(before, composite) {
			return before, StatusAlreadyApplied, "content already inserted adjacent to anchor", 0, nil
		}
		return before, StatusNoMatch, "anchor (old_string) not found", 0, nil
	case 1:
		start := positions[0]
		end := start + len(anchor)

		// Idempotency: content is already present immediately adjacent to
		// the anchor. This catches the case where the anchor still exists
		// and the caller retries.
		if insertBefore && hasSuffixAt(before, content, start) {
			return before, StatusAlreadyApplied, "content already present before anchor", 1, nil
		}
		if !insertBefore && hasPrefixAt(before, content, end) {
			return before, StatusAlreadyApplied, "content already present after anchor", 1, nil
		}

		var after string
		var msg string
		if insertBefore {
			after = before[:start] + content + before[start:]
			msg = "inserted before anchor"
		} else {
			after = before[:end] + content + before[end:]
			msg = "inserted after anchor"
		}
		if after == before {
			return before, StatusUnchanged, "content was empty", 1, nil
		}
		return after, StatusApplied, msg, 1, nil
	default:
		return before, StatusAmbiguous, "anchor (old_string) matched more than once", len(positions), nil
	}
}

// findAllMatches returns the byte offsets of every non-overlapping exact
// occurrence of sub in s.
func findAllMatches(s, sub string) []int {
	if sub == "" {
		return nil
	}
	var out []int
	offset := 0
	for offset <= len(s) {
		idx := strings.Index(s[offset:], sub)
		if idx < 0 {
			break
		}
		pos := offset + idx
		out = append(out, pos)
		offset = pos + len(sub)
	}
	return out
}

// hasSuffixAt reports whether s[:end] ends with suffix.
func hasSuffixAt(s, suffix string, end int) bool {
	if suffix == "" {
		return false
	}
	if end-len(suffix) < 0 {
		return false
	}
	return s[end-len(suffix):end] == suffix
}

// hasPrefixAt reports whether s[start:] starts with prefix.
func hasPrefixAt(s, prefix string, start int) bool {
	if prefix == "" {
		return false
	}
	if start+len(prefix) > len(s) {
		return false
	}
	return s[start:start+len(prefix)] == prefix
}
