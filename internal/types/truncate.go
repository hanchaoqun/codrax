package types

import "unicode/utf8"

// Rune-safe string truncation helpers (G18 per-CLASS closure).
//
// Background: the legacy pattern `s[:n] + "…"` cuts at a BYTE offset.
// When s contains multibyte UTF-8 (CJK, emoji) the cut can land inside
// a rune and the result carries a broken tail that renders as U+FFFD
// mojibake ("…找出导致帧延迟的事件``…" witness, 2026-07 customer
// revisit). Every truncation-with-ellipsis site MUST go through one of
// the helpers below instead of slicing bytes directly.
//
// Two cap semantics exist in the codebase — pick per call site:
//
//  1. BYTE-BUDGET semantics (TruncateBytesEllipsis / CutPrefixRuneSafe /
//     CutSuffixRuneSafe): the cap is a byte budget (LLM-context cost,
//     terminal-width proxy, storage cap). The kept text is at most
//     maxBytes bytes, backed off to the nearest rune boundary so no
//     partial rune is emitted. For pure-ASCII input this is
//     byte-identical to the legacy `s[:n]` shape, which keeps existing
//     golden/test pins intact. Note the appended "…" is NOT counted
//     against the budget — exactly like the legacy `s[:n] + "…"` shape.
//
//  2. RUNE-COUNT semantics (TruncateRunesEllipsis): the cap is a number
//     of user-visible characters. The RESULT (including the ellipsis)
//     holds at most maxRunes runes: strings within the cap pass through
//     unchanged, longer strings are cut to maxRunes-1 runes plus "…".
//     Use for new display surfaces where "N characters" is the contract;
//     migrating a legacy byte-cap site to this variant CHANGES its
//     ASCII behavior (off by one + shorter cap), so legacy sites keep
//     variant 1 unless deliberately re-specified.
//
// Neither variant validates the whole input: pre-existing invalid UTF-8
// passes through untouched. The guarantee is only that the truncation
// itself never MANUFACTURES a broken rune at the cut point.

// TruncateBytesEllipsis caps s at a byte budget, rune-safely, and
// appends "…" when truncation happened. It mirrors the legacy
// `if len(s) > max { s = s[:max] + "…" }` shape byte-for-byte on
// pure-ASCII input. maxBytes <= 0 means "no budget configured" and
// returns s unchanged (matching the dominant legacy guard
// `if max <= 0 || len(s) <= max { return s }`).
func TruncateBytesEllipsis(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	return CutPrefixRuneSafe(s, maxBytes) + "…"
}

// TruncateRunesEllipsis caps s at maxRunes user-visible characters.
// The result — INCLUDING the appended ellipsis — never exceeds
// maxRunes runes: if s already fits it is returned unchanged,
// otherwise the first maxRunes-1 runes are kept and "…" is appended.
// maxRunes <= 0 keeps nothing and returns "".
func TruncateRunesEllipsis(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if len(s) <= maxRunes {
		// ≤ maxRunes bytes implies ≤ maxRunes runes.
		return s
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	i, kept := 0, 0
	for kept < maxRunes-1 {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		kept++
	}
	return s[:i] + "…"
}

// CutPrefixRuneSafe returns the longest prefix of s that fits in
// maxBytes bytes and ends on a rune boundary. No ellipsis is added —
// this is the composition primitive for sites that decorate the cut
// themselves (strings.TrimSpace(...) + "…", " …[truncated]" suffixes,
// silent caps, custom markers). maxBytes <= 0 returns "".
func CutPrefixRuneSafe(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	// s[cut] is the first byte NOT kept. While it is a UTF-8
	// continuation byte the cut splits a rune; back off until the
	// dropped byte starts a rune (or ASCII), so the kept prefix ends
	// on a complete rune. Dropping the lead byte too is what the old
	// hand-rolled truncateForPrompt got wrong (dangling lead byte).
	for cut > 0 && (s[cut]&0xC0) == 0x80 {
		cut--
	}
	return s[:cut]
}

// CutSuffixRuneSafe returns the longest suffix of s that fits in
// maxBytes bytes and starts on a rune boundary. Companion primitive to
// CutPrefixRuneSafe for head+tail middle-elision previews. maxBytes <=
// 0 returns "".
func CutSuffixRuneSafe(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	// Advance past continuation bytes so the kept suffix starts at a
	// rune's first byte.
	for start < len(s) && (s[start]&0xC0) == 0x80 {
		start++
	}
	return s[start:]
}
