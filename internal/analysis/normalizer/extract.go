package normalizer

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// surface is an intermediate extraction record produced by extract.go
// and consumed by canonicalize.go. Kind is a preliminary classification
// from the regex that matched; canonicalize.go may upgrade / correct
// it using repo symbol lookups and the rule tables.
type surface struct {
	text string
	kind surfaceKind
	// order preserves first-seen position so the canonical output is
	// stable across regex ordering changes.
	order int
}

type surfaceKind int

const (
	kindCamel surfaceKind = iota
	kindSnake
	kindDotted
	kindConfigPath
	kindFlag
	kindLiteral
	kindHan
	kindEnWord
)

// Extraction regexes. These are intentionally independent — the same
// surface string may be matched by more than one pattern (e.g. an
// identifier that also looks like a dotted path); dedup happens in
// canonicalize() by canonical ID, not by surface text.

var (
	// CamelCase: at least one lower→upper boundary, otherwise we'd
	// match every capitalized word including stopwords ("How", "What").
	reCamel = regexp.MustCompile(`\b[A-Za-z][a-z0-9]*[A-Z][A-Za-z0-9]*\b`)

	// snake_case: at least one underscore with letters on both sides.
	reSnake = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9]*_[a-zA-Z0-9_]+\b`)

	// Dotted: package.Func, a.b.c. Avoids matching the end of a
	// sentence by requiring at least two dots *or* a known code
	// extension handled below.
	reDotted = regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9]*(?:\.[a-zA-Z][a-zA-Z0-9]*){1,}\b`)

	// Config file paths: contains a slash OR ends with a known code
	// extension. Matches `internal/agent/explorer.go`, `codrax.yaml`,
	// `config/orchestrator.yaml`.
	reConfigPath = regexp.MustCompile(`\b[a-zA-Z0-9_./-]+\.(go|yaml|yml|json|toml|md|sh|py|ts|tsx|js|rs|java|rb)\b`)
	rePathSlash  = regexp.MustCompile(`\b[a-zA-Z0-9_.-]+(?:/[a-zA-Z0-9_.-]+){1,}\b`)

	// CLI flags: -foo or --bar-baz.
	reFlag = regexp.MustCompile(`(?:^|[^a-zA-Z0-9])(--?[a-zA-Z][a-zA-Z0-9-]*)`)

	// Quoted literals. We capture the inner text so the literal itself
	// (without quotes) becomes the surface.
	reLiteralDouble = regexp.MustCompile(`"([^"\\\n]{1,64})"`)
	reLiteralSingle = regexp.MustCompile(`'([^'\\\n]{1,64})'`)
	reLiteralBack   = regexp.MustCompile("`([^`\\n]{1,64})`")

	// Plain ASCII words of length ≥4 that could be English concepts.
	// Case is preserved so the stopword filter can treat CamelCase
	// that survived reCamel's stricter pattern differently.
	reEnWord = regexp.MustCompile(`\b[A-Za-z]{4,}\b`)
)

// extractSurfaces runs every pattern over the raw input and returns
// the combined, deduplicated surface list in first-seen order.
func extractSurfaces(raw string) []surface {
	var out []surface
	seen := make(map[string]int) // text → index in out

	add := func(text string, kind surfaceKind) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		if idx, ok := seen[text]; ok {
			// Keep the strongest kind (lower iota = more structural).
			if kind < out[idx].kind {
				out[idx].kind = kind
			}
			return
		}
		seen[text] = len(out)
		out = append(out, surface{text: text, kind: kind, order: len(out)})
	}

	// Order matters: more specific patterns first so they claim the
	// strongest classification before weaker patterns see the surface.
	for _, m := range reConfigPath.FindAllString(raw, -1) {
		add(m, kindConfigPath)
	}
	for _, m := range rePathSlash.FindAllString(raw, -1) {
		add(m, kindConfigPath)
	}
	for _, m := range reFlag.FindAllStringSubmatch(raw, -1) {
		if len(m) >= 2 {
			add(m[1], kindFlag)
		}
	}
	for _, m := range reLiteralDouble.FindAllStringSubmatch(raw, -1) {
		if len(m) >= 2 {
			add(m[1], kindLiteral)
		}
	}
	for _, m := range reLiteralSingle.FindAllStringSubmatch(raw, -1) {
		if len(m) >= 2 {
			add(m[1], kindLiteral)
		}
	}
	for _, m := range reLiteralBack.FindAllStringSubmatch(raw, -1) {
		if len(m) >= 2 {
			add(m[1], kindLiteral)
		}
	}
	for _, m := range reCamel.FindAllString(raw, -1) {
		add(m, kindCamel)
	}
	for _, m := range reSnake.FindAllString(raw, -1) {
		add(m, kindSnake)
	}
	for _, m := range reDotted.FindAllString(raw, -1) {
		// Skip dotted matches that we already captured as config paths.
		if _, ok := seen[m]; ok {
			continue
		}
		// Skip if every segment is a stopword (e.g. "how.to.do").
		segs := strings.Split(m, ".")
		alpha := 0
		for _, s := range segs {
			if len(s) >= 2 {
				alpha++
			}
		}
		if alpha >= 2 {
			add(m, kindDotted)
		}
	}

	// Chinese concept runs: maximal Han sequences.
	out = append(out, extractHanRuns(raw, len(out))...)
	for _, s := range out {
		if _, ok := seen[s.text]; !ok {
			seen[s.text] = len(seen)
		}
	}

	// English concept words, filtered by stopword list so "explain how
	// the explorer stops" yields just "explorer" (and the CamelCase
	// match for it if any).
	for _, m := range reEnWord.FindAllString(raw, -1) {
		lower := strings.ToLower(m)
		if isEnglishStopword(lower) {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		add(m, kindEnWord)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

// extractHanRuns returns maximal Han-character sequences of length ≥2
// that are not entirely in the Chinese stopword list. baseOrder is the
// starting ordinal so the caller can merge with the main surface list
// without reshuffling.
func extractHanRuns(raw string, baseOrder int) []surface {
	var out []surface
	var buf strings.Builder
	flush := func() {
		if buf.Len() == 0 {
			return
		}
		text := buf.String()
		buf.Reset()
		runes := []rune(text)
		if len(runes) < 2 {
			return
		}
		// Peel off leading/trailing stopword characters so "请解释"
		// becomes "解释". This is a crude but effective filter.
		for len(runes) > 0 && isChineseStopwordRune(runes[0]) {
			runes = runes[1:]
		}
		for len(runes) > 0 && isChineseStopwordRune(runes[len(runes)-1]) {
			runes = runes[:len(runes)-1]
		}
		if len(runes) < 2 {
			return
		}
		out = append(out, surface{
			text:  string(runes),
			kind:  kindHan,
			order: baseOrder + len(out),
		})
	}
	for _, r := range raw {
		if unicode.Is(unicode.Han, r) {
			buf.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}
