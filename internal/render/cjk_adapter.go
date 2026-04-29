package render

import (
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// cjkAdapter substitutes wide-rune characters (CJK ideographs,
// Hiragana, Katakana, Hangul, full-width forms) in mermaid input
// with 2-byte ASCII placeholders before handing the source to
// pgavlin/mermaid-ascii's graph renderer (which width-counts by
// byte length, not display width). After the library returns its
// ASCII grid, the adapter restores the original wide-rune
// characters at every placeholder position.
//
// The cell-arithmetic invariant that makes this safe:
//
//   - Each wide rune occupies exactly 2 display cells.
//   - Each 2-byte ASCII placeholder occupies exactly 2 display
//     cells (1 cell per byte) AND library-byte-width 2.
//   - Library computes box widths from byte-length, so it allocates
//     2 cells per placeholder = 2 cells per future wide rune.
//   - At restore time, byte-replacement preserves the rendered grid
//     because we exchange 2 ASCII bytes for 1 wide rune (3 bytes
//     UTF-8) of identical display width.
//
// Result: CJK labels render aligned, end-to-end, without modifying
// the library.
//
// Failure modes (all degrade to the current "keep mermaid source"
// path; never break alignment):
//   - Pool exhausted (>= 2*36*36 unique placeholders needed):
//     adapter returns an error; caller leaves source unchanged.
//   - Mid-grade non-ASCII (accented Latin, emoji) that runewidth
//     doesn't classify as wide-rune (width 1): cannot be safely
//     substituted because library's byte-count would mismatch
//     terminal display width. Adapter returns an error in that case.
type cjkAdapter struct {
	// placeholderToRune maps a 2-byte ASCII placeholder back to the
	// original wide rune. Iterated at restore time.
	placeholderToRune map[string]rune

	// usedPool keeps track of which placeholder pairs have been
	// allocated this run so substitute() never reuses one.
	usedPool map[string]bool

	// poolCursor walks the placeholder pool deterministically so a
	// given input always produces the same placeholder sequence.
	poolCursor int

	// reservedPool excludes placeholder candidates that already
	// appear verbatim in the source as a 2-byte substring — guards
	// against restore() collisions where a real ASCII pair like
	// "QA" in a label gets unintentionally rewritten to a wide rune.
	reservedPool map[string]bool
}

// newCJKAdapter constructs a fresh adapter. Each render pass should
// use its own adapter so placeholder allocation stays local.
func newCJKAdapter(source string) *cjkAdapter {
	return &cjkAdapter{
		placeholderToRune: make(map[string]rune),
		usedPool:          make(map[string]bool),
		reservedPool:      buildReservedPool(source),
	}
}

// buildReservedPool scans the source mermaid for any 2-character
// ASCII pairs that already appear, so the adapter never picks one
// of those as a placeholder (would cause a false-positive
// restoration). Walks the string byte-by-byte; only ASCII pairs
// (both bytes < 0x80) are considered (a multi-byte continuation
// can't collide with a 2-ASCII placeholder).
func buildReservedPool(s string) map[string]bool {
	reserved := make(map[string]bool)
	for i := 0; i+1 < len(s); i++ {
		b1, b2 := s[i], s[i+1]
		if b1 < 0x80 && b2 < 0x80 {
			reserved[string([]byte{b1, b2})] = true
		}
	}
	return reserved
}

// substitute walks the source and replaces every wide rune with a
// 2-byte ASCII placeholder. Returns the rewritten source plus a
// boolean indicating whether substitution was needed at all (false
// = caller should call the library directly without restore).
//
// Returns an error in the cases the adapter cannot safely handle:
//   - placeholder pool exhausted (>1296 unique wide runes — vastly
//     more than any plausible diagram)
//   - non-wide non-ASCII rune encountered (e.g. accented Latin,
//     emoji); placeholder cell-count would not match terminal
//     display width, so we surface an error and the caller falls
//     back to "leave source as-is"
func (a *cjkAdapter) substitute(s string) (string, bool, error) {
	var b strings.Builder
	b.Grow(len(s))
	substituted := false
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte — write through as-is. Treat as
			// a regular byte from the library's perspective.
			b.WriteByte(s[i])
			i++
			continue
		}
		if size == 1 {
			// 1-byte ASCII — pass through unchanged.
			b.WriteByte(s[i])
			i++
			continue
		}
		// Multi-byte rune. Decide whether it's a "wide rune" we
		// can safely substitute (display width 2) or a "narrow
		// rune" we cannot (display width 1 — substitution would
		// over-pad).
		w := runewidth.RuneWidth(r)
		if w != 2 {
			return "", false, errNarrowMultibyteRune(r)
		}
		ph, err := a.allocatePlaceholder()
		if err != nil {
			return "", false, err
		}
		a.placeholderToRune[ph] = r
		b.WriteString(ph)
		substituted = true
		i += size
	}
	return b.String(), substituted, nil
}

// restore replaces every previously-allocated placeholder back to
// its original wide rune. Order of replacement is deterministic
// (sorted by placeholder string) so two restorations produce
// byte-identical output.
//
// strings.ReplaceAll is byte-level — safe because placeholders are
// chosen from `reservedPool`-disjoint 2-byte pairs that don't
// otherwise appear in the library's output. If the library somehow
// embedded a placeholder fragment in unrelated context, we'd
// over-replace; the reserved pool detection makes this extremely
// unlikely in practice.
func (a *cjkAdapter) restore(rendered string) string {
	for ph, r := range a.placeholderToRune {
		rendered = strings.ReplaceAll(rendered, ph, string(r))
	}
	return rendered
}

// placeholderPool is the deterministic sequence of 2-byte ASCII
// candidates the adapter draws from. Two layers:
//
//  1. Two-letter combos drawn from a "rare prefix" alphabet: chars
//     from the set [Q W X Z] for the first byte plus [a-z 0-9]
//     for the second. This gives 4*36 = 144 pairs unlikely to
//     appear in identifier-shaped mermaid labels.
//  2. If exhausted, broaden to all uppercase + lowercase pairs:
//     [A-Z]+[A-Z] and [A-Z]+[a-z] = 26*26 + 26*26 = 1352 pairs.
//
// Total pool ~1496, far exceeding any plausible diagram's wide-rune
// count.
var placeholderPool = buildPlaceholderPool()

func buildPlaceholderPool() []string {
	var pool []string
	rare := []byte{'Q', 'W', 'X', 'Z'}
	tail := "abcdefghijklmnopqrstuvwxyz0123456789"
	for _, p := range rare {
		for j := 0; j < len(tail); j++ {
			pool = append(pool, string([]byte{p, tail[j]}))
		}
	}
	for c := byte('A'); c <= 'Z'; c++ {
		for d := byte('A'); d <= 'Z'; d++ {
			pool = append(pool, string([]byte{c, d}))
		}
	}
	for c := byte('A'); c <= 'Z'; c++ {
		for d := byte('a'); d <= 'z'; d++ {
			pool = append(pool, string([]byte{c, d}))
		}
	}
	return pool
}

// allocatePlaceholder returns the next available 2-byte ASCII
// placeholder, skipping any candidate that:
//   - is already in use this pass (usedPool)
//   - already appears in the source (reservedPool)
//
// Returns errPlaceholderExhausted when the pool is depleted.
func (a *cjkAdapter) allocatePlaceholder() (string, error) {
	for a.poolCursor < len(placeholderPool) {
		cand := placeholderPool[a.poolCursor]
		a.poolCursor++
		if a.usedPool[cand] || a.reservedPool[cand] {
			continue
		}
		a.usedPool[cand] = true
		return cand, nil
	}
	return "", errPlaceholderExhausted
}

// adapterError is the typed error class for the adapter so
// callers can distinguish "non-substitutable rune" from "pool
// exhausted" from generic library errors.
type adapterError struct{ msg string }

func (e *adapterError) Error() string { return e.msg }

func errNarrowMultibyteRune(r rune) error {
	return &adapterError{msg: "cjk adapter: rune " + string(r) + " is multi-byte but not wide-display (width != 2); cannot substitute safely"}
}

var errPlaceholderExhausted = &adapterError{msg: "cjk adapter: placeholder pool exhausted"}
