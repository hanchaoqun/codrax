package types

// SymbolOracle is the read-only "does symbol X exist in this
// repo?" lookup. Implemented by repomap.NewSymbolOracle (which
// wraps a built repomap.Graph); the interface lives in this
// package so consumers (logtriage, emit_answer_document, future
// answer-coherence validators) can depend on the contract
// without pulling in repomap (which would create import cycles
// for analysis/* packages that types depends on transitively).
//
// Pattern mirrors MemoryReader: thin interface in types, fat
// implementation outside.
//
// Behaviour contract:
//   - SymbolExists returns (true, minTier) when name resolves to
//     at least one Symbol; minTier is the LOWEST (=most reliable)
//     parse tier among matching definitions. Lower tier = more
//     reliable parse (Tier 1 = primary grammar; Tier 4 = path-only).
//   - Returns (false, 0) when no definition matches.
//   - Lookup is exact-name + case-sensitive; canonicalisation
//     (CamelCase / snake_case stripping, dotted-path splitting) is
//     the caller's job because different consumers normalise
//     differently.
//
// nil receivers are tolerated and return (false, 0) so callers
// can pass nil to disable validation without nil-checking each
// site (used by tests + single-shot CLI flows that don't build a
// repomap).
type SymbolOracle interface {
	SymbolExists(name string) (found bool, minTier int)
}
