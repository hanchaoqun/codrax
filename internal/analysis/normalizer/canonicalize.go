package normalizer

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// canonicalize assigns every surface a stable canonical ID, a
// TermKind, a language tag, and (optionally) a repo-grounded domain.
// It returns the deduplicated canonical list and an index mapping
// every surface text to its canonical ID so alias.go can build edges
// without re-running classification.
//
// Dedup semantics: two surfaces collapse to the same canonical term
// when their canonical IDs match. The first-seen surface wins on
// surface text; later occurrences become aliases (handled in
// alias.go). Confidence is chosen by kind: repo-resolved symbols get
// 1.0, pattern-based symbols 0.9, config 0.85, concepts 0.7, literals
// 0.6.
func canonicalize(surfaces []surface, opts Options) ([]types.CanonicalTerm, map[string]string) {
	canonical := make([]types.CanonicalTerm, 0, len(surfaces))
	byID := make(map[string]int, len(surfaces)) // canonical ID → index in canonical
	index := make(map[string]string, len(surfaces))

	for _, s := range surfaces {
		if opts.PreserveLiterals == false && s.kind == kindLiteral {
			continue
		}
		id, term := classify(s, opts)
		if id == "" {
			continue
		}
		index[s.text] = id
		if _, exists := byID[id]; exists {
			continue
		}
		byID[id] = len(canonical)
		canonical = append(canonical, term)
	}
	return canonical, index
}

// classify maps a single surface to its canonical ID and term. The ID
// format is "<bucket>:<normalized-key>" where bucket is one of
// code / cfg / cmd / lit / en / zh. Normalization collapses case and
// underscores for code symbols so `TaskList`, `task_list`, and
// `TASKLIST` all share `code:tasklist`.
func classify(s surface, opts Options) (string, types.CanonicalTerm) {
	switch s.kind {
	case kindCamel, kindSnake:
		key := normalizeCodeKey(s.text)
		id := "code:" + key
		// Repo resolver may override with a known canonical surface.
		domain := ""
		surface := s.text
		confidence := float32(0.9)
		if opts.Resolver != nil {
			if hits := opts.Resolver.LookupSymbol(s.text); len(hits) > 0 {
				domain = hits[0].Domain
				if hits[0].Canonical != "" {
					surface = hits[0].Canonical
				}
				confidence = 1.0
			}
		}
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    surface,
			Language:   "code",
			Kind:       types.TermSymbol,
			Domain:     domain,
			Confidence: confidence,
		}
	case kindDotted:
		id := "code:" + normalizeCodeKey(s.text)
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    s.text,
			Language:   "code",
			Kind:       types.TermSymbol,
			Confidence: 0.85,
		}
	case kindConfigPath:
		id := "cfg:" + strings.ToLower(s.text)
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    s.text,
			Language:   "code",
			Kind:       types.TermConfig,
			Confidence: 0.85,
		}
	case kindFlag:
		id := "cmd:" + strings.ToLower(strings.TrimLeft(s.text, "-"))
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    s.text,
			Language:   "code",
			Kind:       types.TermCommand,
			Confidence: 0.85,
		}
	case kindLiteral:
		id := "lit:" + s.text
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    s.text,
			Language:   "code",
			Kind:       types.TermLiteral,
			Confidence: 0.6,
		}
	case kindHan:
		lower := strings.ToLower(s.text)
		id := "zh:" + lower
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    s.text,
			Language:   "zh",
			Kind:       types.TermConcept,
			Confidence: 0.7,
		}
	case kindEnWord:
		lower := strings.ToLower(s.text)
		// If the lowercase form also exists as a code symbol (repo
		// resolver says so), upgrade this to a symbol term.
		if opts.Resolver != nil {
			if hits := opts.Resolver.LookupSymbol(s.text); len(hits) > 0 {
				id := "code:" + normalizeCodeKey(hits[0].Canonical)
				return id, types.CanonicalTerm{
					ID:         id,
					Surface:    hits[0].Canonical,
					Language:   "code",
					Kind:       types.TermSymbol,
					Domain:     hits[0].Domain,
					Confidence: 1.0,
				}
			}
		}
		id := "en:" + lower
		return id, types.CanonicalTerm{
			ID:         id,
			Surface:    s.text,
			Language:   "en",
			Kind:       types.TermConcept,
			Confidence: 0.7,
		}
	}
	return "", types.CanonicalTerm{}
}

// normalizeCodeKey strips underscores and lowercases so identifier
// variants collapse to a single canonical key. It preserves digits.
func normalizeCodeKey(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '_' || r == '-' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r = r + ('a' - 'A')
		}
		b.WriteRune(r)
	}
	return b.String()
}
