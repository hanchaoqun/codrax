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
//
// SymbolExistsFlat (Fix E, 2026-05-07) is the case-form-aware
// variant. It canonicalises `name` by stripping `_` / `-` and
// lowercasing ASCII (the same `contract.FlattenIdentifier` rule
// that drives must_include / must_exclude / acceptance / extractor
// match across the contract layer), then matches against any
// graph-indexed symbol whose own canonicalised name equals the
// query. So `pipeline_max_steps` (yaml-tag form), `PipelineMaxSteps`
// (Go-field form), `pipeline-max-steps` (CLI-flag form), and
// `PIPELINE_MAX_STEPS` (env-var form) all resolve to the SAME
// underlying symbol. minTier semantics are identical to
// SymbolExists: lowest tier across all matching definitions.
//
// Use SymbolExistsFlat when the question is "does this logical
// identifier name a real codebase entity, regardless of which
// surface form the LLM rendered?" — true for hallucination-gate
// validators (Fix C / Fix D) and any future check that asks the
// existence question against an LLM-emitted identifier.
//
// Use SymbolExists when the question is "does this exact-form
// name resolve?" — true for must_include / must_exclude / acceptance
// where the contract terms are themselves authoritative form
// declarations (the analyzer / contract author chose the form
// deliberately).
type SymbolOracle interface {
	SymbolExists(name string) (found bool, minTier int)
	SymbolExistsFlat(name string) (found bool, minTier int)
}

// QualifiedSymbolOracle (QNO batch, 2026-07-05) is the OPTIONAL
// extension for qualified-name existence checks. SymbolExists /
// SymbolExistsFlat index symbols by BARE name, so a package- /
// receiver-qualified spelling like "gate.Run", "Gate.Run",
// "(*Gate).Run", "mod::Type::method" or "Foo::Bar#baz" always
// misses even when the symbol exists (the s1a / A1 anchor-obligation
// boundary recorded in arch_stability_batch_plan_20260702.md §A1).
//
// Behaviour contract:
//   - The name is decomposed DETERMINISTICALLY by language separator
//     grammar (`::`/`#` → `.` segmentation; Go receiver-paren forms
//     `(*T).M` / `(r *T).M` reduce to the receiver type). No fuzzy,
//     similarity, or edit-distance matching anywhere — an
//     undecomposable or unresolvable name is an honest miss.
//   - Returns (true, minTier) only when the trailing segment resolves
//     to a graph symbol under SymbolExistsFlat's case-form-aware
//     equality AND every qualifier segment exactly matches one of the
//     symbol's typed scope levels: receiver, parent type, package
//     (whole or per-segment), or — ONLY when no package clause is
//     recorded for the file — the defining-directory basename (F3:
//     a recorded package is authoritative; dir names never widen it).
//   - Single-segment (unqualified) names return (false, 0): the
//     exact / flat lanes own those; this method answers ONLY the
//     qualified-form question.
//   - nil receivers tolerated, returning (false, 0), same as above.
//
// Known, accepted imprecision (F2, recorded 2026-07-05, no behaviour
// change intended):
//   - Qualifier matching folds case forms exactly like the flat lane
//     (FlattenIdentifier equality), so "GATE.Run" / "Gate.run" hit the
//     same symbol as "gate.Run" — receiver-vs-package spelling is
//     conflated when qualifiers differ only by case/separators. This
//     is a deliberate extension of the existing flat case-form
//     contract, NOT a second case rule; do not "fix" one side alone.
//   - minTier consumption is uneven by design: the entity provenance
//     classification discards it (existence-only question), while the
//     analyzer must-include support lane
//     (contractOracleHasReliableSymbol) applies the same tier<3
//     reliability floor as the exact/flat lanes. Latent only for the
//     provenance consumer.
//
// Kept as a separate interface (not a third SymbolOracle method) so
// the many existing hard-gate consumers of SymbolExistsFlat keep
// byte-identical behaviour; consumers opt in via type assertion.
type QualifiedSymbolOracle interface {
	QualifiedSymbolExists(name string) (found bool, minTier int)
}
