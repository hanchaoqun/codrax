package repomap

import (
	"strings"
	"unicode"
)

// QueryToken kinds. Exported so the matcher in rank.go can branch on
// them if it needs to (e.g. weight CJK differently from Latin ident).
const (
	TokenIdentifier = "ident"
	TokenCJK        = "cjk"
	TokenNumber     = "number"
)

// QueryToken is one searchable unit produced by TokenizeQuery. A
// single raw query input (`BuildAgentContext narrowed view`,
// `有多少个agent可以调用subagent`, `propose_sub_agents tool`) expands
// into a deduplicated slice of these — the matcher in rank.go scores
// each document by how many tokens hit, weighted by token.Weight.
//
// Weight conventions:
//   - 1.0 — primary token, i.e. the whole whitespace-separated raw
//     identifier lowered. Represents the user's literal intent.
//   - 0.5 — secondary sub-token emitted from CamelCase / snake_case /
//     kebab-case / dotted / slashed decomposition of a primary.
//     Gives partial credit for matching any individual word inside a
//     multi-word identifier without letting those words dominate the
//     primary hit.
//   - 1.0 — CJK bi-gram (or singleton uni-gram for length-1 runs). CJK
//     has no natural word boundary so bi-grams are the atomic match
//     unit, not secondary fallbacks.
type QueryToken struct {
	Text   string
	Kind   string
	Weight float64
}

// TokenizeQuery splits a search query into primary tokens plus
// CamelCase / underscore / kebab sub-tokens plus CJK bi-grams. The
// result is deduplicated by (kind, text) — repeating a token in the
// query does not amplify its match weight.
//
// Whitespace and standalone punctuation are discarded. Characters
// considered part of a Latin identifier (letter, digit, `_`, `-`,
// `.`, `/`, `:`) keep a run together as a single primary token;
// CamelCase and separator-based decomposition then emits the
// secondary sub-tokens.
//
// CJK runs (Han, Hiragana, Katakana, Hangul) are not split on
// whitespace or punctuation — they are tokenized into overlapping
// 2-rune bi-grams, the canonical atomic unit for CJK information
// retrieval. A single-rune run emits one uni-gram.
func TokenizeQuery(query string) []QueryToken {
	var out []QueryToken
	seen := make(map[string]bool)
	add := func(text, kind string, weight float64) {
		if text == "" {
			return
		}
		key := kind + "\x00" + text
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, QueryToken{Text: text, Kind: kind, Weight: weight})
	}

	runes := []rune(query)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if isCJKRune(r) {
			j := i + 1
			for j < len(runes) && isCJKRune(runes[j]) {
				j++
			}
			emitCJKBigrams(string(runes[i:j]), add)
			i = j
			continue
		}
		if isIdentRune(r) {
			j := i + 1
			for j < len(runes) && isIdentRune(runes[j]) && !isCJKRune(runes[j]) {
				j++
			}
			raw := string(runes[i:j])
			lowered := strings.ToLower(raw)
			primaryKind := TokenIdentifier
			if isAllDigits(lowered) {
				primaryKind = TokenNumber
			}
			add(lowered, primaryKind, 1.0)
			subs := splitIdentifier(raw)
			if len(subs) > 1 {
				for _, sub := range subs {
					kind := TokenIdentifier
					if isAllDigits(sub) {
						kind = TokenNumber
					}
					add(sub, kind, 0.5)
				}
			}
			i = j
			continue
		}
		// Standalone punctuation (commas, quotes, parens) — skip.
		i++
	}
	return out
}

// isCJKRune reports whether r belongs to any of the scripts treated
// as CJK by the bi-gram tokenizer: Han, Hiragana, Katakana, Hangul.
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// isIdentRune reports whether r is part of a Latin-family identifier
// run. Accepts letters, digits, and the common separators `_`, `-`,
// `.`, `/`, `:` so dotted and slashed identifiers (file paths,
// namespaces) stay as one primary token before sub-decomposition.
func isIdentRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '_', '-', '.', '/', ':':
		return true
	}
	return false
}

// isAllDigits reports whether every rune in s is an ASCII digit.
// Used to reclassify all-digit sub-tokens as TokenNumber.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// emitCJKBigrams walks a CJK run and emits overlapping 2-rune
// bi-grams (or a single uni-gram when the run is length 1). Each
// bi-gram is a primary-weight CJK token.
func emitCJKBigrams(run string, add func(text, kind string, weight float64)) {
	rs := []rune(run)
	if len(rs) == 0 {
		return
	}
	if len(rs) == 1 {
		add(string(rs), TokenCJK, 1.0)
		return
	}
	for i := 0; i+1 < len(rs); i++ {
		add(string(rs[i:i+2]), TokenCJK, 1.0)
	}
}

// splitIdentifier decomposes a single raw identifier token into its
// constituent sub-words. It runs in two phases:
//
//  1. Split on non-letter-non-digit runes (`_`, `-`, `.`, `/`, `:`,
//     etc.). This handles snake_case, kebab-case, dotted paths, and
//     slash paths.
//  2. Run splitCamel on each phase-1 chunk to decompose CamelCase
//     and TitleCase into their individual words, respecting the
//     HTTPServer-style rule where a run of uppercase letters
//     followed by a lowercase letter breaks before the last
//     uppercase (HTTPServer → HTTP, Server).
//
// Every output sub-token is lowercased.
func splitIdentifier(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) == 0 {
			return
		}
		for _, w := range splitCamel(string(cur)) {
			out = append(out, strings.ToLower(w))
		}
		cur = cur[:0]
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// splitCamel decomposes a single letter-and-digit chunk into
// CamelCase / TitleCase sub-words. Does not lowercase — callers do.
func splitCamel(s string) []string {
	if s == "" {
		return nil
	}
	runes := []rune(s)
	var out []string
	i := 0
	for i < len(runes) {
		j := i + 1
		if unicode.IsUpper(runes[i]) {
			// Walk subsequent uppercase letters (TitleCase run).
			for j < len(runes) && unicode.IsUpper(runes[j]) {
				j++
			}
			if j < len(runes) && unicode.IsLower(runes[j]) {
				// Back off one uppercase so the last upper starts
				// the next word (HTTPServer → HTTP + Server).
				if j-i > 1 {
					j--
				}
				// Extend into the lowercase tail.
				for j < len(runes) && unicode.IsLower(runes[j]) {
					j++
				}
			}
		} else {
			// Lowercase or digit run — extend until the next uppercase.
			for j < len(runes) && !unicode.IsUpper(runes[j]) {
				j++
			}
		}
		out = append(out, string(runes[i:j]))
		i = j
	}
	return out
}
