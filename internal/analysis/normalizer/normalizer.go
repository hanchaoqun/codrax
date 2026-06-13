// Package normalizer turns a raw natural-language + code user request
// into a canonical TermGraph that downstream stages (explorer, ERM,
// finalizer) can consume without re-deriving terminology every time.
//
// The normalizer is the sole writer of RequestModel.TermGraph and is
// the only place where "what words does the user mean" is decided.
// Explorer and ERM are forbidden from free-form keyword assembly under
// the Analyzer v3 design — they must resolve every term through
// normalizer.Resolve so callers and tests share the same surface set.
//
// Pipeline:
//
//	Normalize(raw, opts)
//	  ├── extract.go        — pattern-based surface extraction
//	  ├── canonicalize.go   — assign Kind / Language / canonical ID, dedup
//	  └── alias.go          — build synonym / translation / instanceof edges
//
// Rules live in rules_en.go and rules_zh.go. Repo-symbol grounding is
// exposed as a pluggable SymbolResolver so unit tests can run without
// touching the filesystem; the real repo resolver will be wired in
// during batch B5 alongside the analyzer agent rewrite.
package normalizer

import "github.com/hanchaoqun/codrax/internal/types"

// SymbolResolver is an optional hook that lets the normalizer enrich
// extracted terms with repo symbol-table knowledge: given a candidate
// surface, it returns the canonical symbol name(s) and domain tag the
// repo defines for that surface, or nil if unknown. Returning nil is
// always safe — the normalizer falls back to pattern-based kind
// assignment when the resolver declines.
type SymbolResolver interface {
	LookupSymbol(surface string) []SymbolHit
}

// SymbolMorphAliasResolver is an optional extension implemented by repo-backed
// resolvers that can confirm lexical morphology candidates such as
// extractor -> extract against the symbol table. Normalizer callers do not need
// to implement it; absence simply disables this fallback.
type SymbolMorphAliasResolver interface {
	LookupSymbolMorphAlias(surface string) []SymbolHit
}

// SymbolHit is one repo-grounded match for a surface. Domain is a
// free-form string such as "agent", "tool", "config" that downstream
// consumers may use for routing.
type SymbolHit struct {
	Canonical string
	Domain    string
}

// Options controls optional normalizer behaviors. Zero value is valid
// and produces a text-only TermGraph with no repo grounding.
type Options struct {
	// Resolver, if non-nil, is consulted during canonicalization to
	// upgrade concept terms to symbol terms when the repo defines a
	// matching identifier.
	Resolver SymbolResolver

	// PreserveLiterals keeps quoted literals in the term graph. When
	// false (default) literals are dropped because they are typically
	// too specific to help search ranking.
	PreserveLiterals bool

	// Entities is the LLM-emitted subject list from emit_analysis.
	// It gates the kindEnWord → TermSymbol promotion: an English word
	// only upgrades when it also appears (case/underscore-insensitive)
	// in this list AND the Resolver returns a hit. Empty or nil list
	// disables the entity gate (resolver hit alone is enough), which
	// matches the pre-v3 behavior and keeps zero-value Options valid.
	Entities []string
}

// Normalize builds a TermGraph from a raw user request. It is pure
// (no I/O) when Options.Resolver is nil, making it trivially testable.
//
// The result is deterministic for a given input: extraction is
// regex-driven, canonicalization uses stable keys, and alias edges
// are emitted in a fixed order.
func Normalize(raw string, opts Options) types.TermGraph {
	surfaces := extractSurfaces(raw)
	canonical, surfaceIndex := canonicalize(surfaces, opts)
	aliases := buildAliasGraph(surfaces, surfaceIndex)
	return types.TermGraph{
		Canonical: canonical,
		Aliases:   aliases,
	}
}

// Resolve returns every known surface form for a canonical term ID.
// The first element is the canonical surface; subsequent elements are
// alias sources in insertion order. Callers use this to expand a
// single canonical ID into a set of strings to feed grep / repo_map.
func Resolve(tg *types.TermGraph, canonicalID string) []string {
	if tg == nil {
		return nil
	}
	var out []string
	for i := range tg.Canonical {
		if tg.Canonical[i].ID == canonicalID {
			out = append(out, tg.Canonical[i].Surface)
			break
		}
	}
	for i := range tg.Aliases {
		if tg.Aliases[i].Target == canonicalID {
			out = append(out, tg.Aliases[i].Source)
		}
	}
	return out
}
