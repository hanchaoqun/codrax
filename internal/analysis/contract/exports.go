package contract

// Public re-exports of token-form-aware identifier helpers used by
// other packages (notably internal/orchestrator block validators)
// so they can apply the same snake/camel/kebab/Pascal/SCREAMING_SNAKE
// equivalence rule that contract.containsSymbol uses internally.
//
// The unexported helpers stay package-private to keep the
// contract.Check API minimal; these exports name a stable surface
// for cross-package callers without leaking the broader checker
// internals.

// FlattenIdentifier strips `_` and `-` separators and lowercases
// ASCII letters, leaving an unambiguous "flat" representation that
// is invariant across snake_case, camelCase, PascalCase, kebab-case,
// SCREAMING_SNAKE_CASE, and flatcase. See flattenIdentifier for the
// full coverage rationale.
func FlattenIdentifier(s string) string { return flattenIdentifier(s) }

// IsIdentifierShaped reports whether s consists solely of ASCII
// identifier characters (letters, digits, _, -). Use this as the
// gate before applying flat-form fallback matching: only
// identifier-shaped tokens benefit from the fallback; multi-word
// phrases keep strict semantics.
func IsIdentifierShaped(s string) bool { return isIdentifierShaped(s) }

// ShouldOracleGateInclude reports whether a token is shaped like a
// code identifier (CamelCase / camelCase / snake_case ≥ 4 chars).
// Used by callers that want to apply SymbolOracle existence
// validation only on identifier-shaped tokens (multi-word phrases
// and short tokens bypass the gate to preserve substring
// permissiveness for prose content).
func ShouldOracleGateInclude(s string) bool { return shouldOracleGateInclude(s) }
