package types

// SymbolLocation is one definition site of a symbol — repo-relative
// file path plus 1-indexed line range. Returned by SymbolLocator
// queries. Multiple SymbolLocations for the same name represent
// multiple definitions across the repo (e.g. a method named "Run"
// implemented by several distinct types).
type SymbolLocation struct {
	File    string
	Line    int
	EndLine int
}

// SymbolLocator is the "where is symbol X defined in this repo?"
// lookup. Strictly richer than SymbolOracle: SymbolOracle answers
// existence + parse-tier; SymbolLocator additionally returns each
// definition's (file, line) so the drift detector can compare
// log-side and current-code positions.
//
// Implemented by repomap-side wrappers (commit 3 wires
// repomap.NewSymbolLocator). Lives in this package so logtriage and
// other analysis-layer consumers depend only on the contract,
// preserving the no-cycle pattern established by SymbolOracle.
//
// Behaviour contract:
//   - LocateSymbol returns every definition of name in the repo,
//     ordered as the underlying Graph stores them (typically file
//     ascending, then line ascending, but callers MUST NOT rely on
//     a specific order beyond stable iteration).
//   - Returns nil (not empty slice) when no definitions match.
//   - Lookup is exact-name + case-sensitive (same as SymbolOracle).
//   - SymbolsInFile returns every symbol defined in the given
//     repo-relative file path. Returns nil when the file is not
//     indexed.
//
// nil receivers are tolerated (return nil) so callers do not need
// to nil-guard every call. Single-shot CLI flows and tests that
// don't build a repomap can pass nil to disable drift detection
// gracefully.
type SymbolLocator interface {
	LocateSymbol(name string) []SymbolLocation
	SymbolsInFile(file string) []SymbolLocation
}
