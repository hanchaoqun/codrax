package normalizer

import (
	"math"
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
		key := NormalizeCodeKey(s.text)
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
		id := "code:" + NormalizeCodeKey(s.text)
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
		// Dual-gate promotion. An English word upgrades to TermSymbol
		// only when both sides agree the word is a load-bearing code
		// identifier in this query:
		//   gate A (subject)  — NormalizeCodeKey(surface) ∈ opts.Entities
		//   gate B (existence) — resolver returns ≥1 hit
		// When opts.Entities is empty, gate A is disabled so zero-value
		// Options still behave the pre-v3 way. Confidence is scaled by
		// the hit count: a single unambiguous hit keeps 1.0; N hits pay
		// a 1/(1+ln N) rarity penalty so common words (e.g. "config"
		// matching 30 symbols) settle below unambiguous ones without a
		// hand-curated allowlist.
		if opts.Resolver != nil && entityGateOpen(s.text, opts.Entities) {
			if hits := lookupSymbolWithMorphAlias(opts.Resolver, s.text); len(hits) > 0 {
				id := "code:" + NormalizeCodeKey(hits[0].Canonical)
				return id, types.CanonicalTerm{
					ID:         id,
					Surface:    hits[0].Canonical,
					Language:   "code",
					Kind:       types.TermSymbol,
					Domain:     hits[0].Domain,
					Confidence: rarityConfidence(len(hits)),
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

// entityGateOpen reports whether the English surface qualifies for
// promotion to TermSymbol under the dual-gate rule. Returns true when
// the entity list is empty (gate disabled) or when any entry matches
// surface under NormalizeCodeKey (case and underscore insensitive).
func entityGateOpen(surface string, entities []string) bool {
	if len(entities) == 0 {
		return true
	}
	target := NormalizeCodeKey(surface)
	if target == "" {
		return false
	}
	for _, e := range entities {
		if NormalizeCodeKey(e) == target {
			return true
		}
	}
	return false
}

func lookupSymbolWithMorphAlias(resolver SymbolResolver, surface string) []SymbolHit {
	if resolver == nil {
		return nil
	}
	if hits := resolver.LookupSymbol(surface); len(hits) > 0 {
		return hits
	}
	if morph, ok := resolver.(SymbolMorphAliasResolver); ok {
		return morph.LookupSymbolMorphAlias(surface)
	}
	return nil
}

// rarityConfidence maps the number of repomap definitions that match a
// surface to a CanonicalTerm.Confidence in (0, 1]. A single definition
// is unambiguous (1.0); the penalty follows 1/(1+ln N) so a 10-hit
// generic word drops to ~0.30 without a hand-tuned threshold.
func rarityConfidence(hits int) float32 {
	if hits <= 0 {
		return 0
	}
	if hits == 1 {
		return 1.0
	}
	return float32(1.0 / (1.0 + math.Log(float64(hits))))
}

// NormalizeCodeKey strips underscores / hyphens and lowercases so
// identifier variants collapse to a single canonical key. Digits
// preserved. Used by this package for TermGraph canonicalisation and
// by the agent package's ERM matching — exposed because both paths
// used to carry their own copy (agent.normalizeForMatch used 3-pass
// ReplaceAll; this version uses a single-pass rune walk).
func NormalizeCodeKey(s string) string {
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
