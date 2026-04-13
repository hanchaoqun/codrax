// Package repomap is the top-level facade for the tree-sitter-
// powered repository index. The real implementation lives in three
// sub-packages introduced by the Phase 4 three-layer split:
//
//   - types/    — structural types (Graph, FileInfo, Symbol, ...)
//                 and Graph-receiver methods for in-graph navigation
//   - index/    — graph construction, cache I/O, scanner, parser,
//                 per-language extractors and import resolvers
//   - retrieve/ — rank scoring, multilingual query tokenizer
//   - render/   — view data builders and markdown renderer
//
// This top-level package re-exports the small set of identifiers
// that external callers (agents, dataflow, cmd, tests, eval harness)
// already depend on, via Go type aliases and thin function wrappers.
// External code continues to use `repomap.Graph`, `repomap.Symbol`,
// `repomap.BuildOrLoadGraph`, etc. unchanged — the three-layer
// split is invisible at the import boundary.
package repomap

import (
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/render"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/retrieve"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// ---- type re-exports ------------------------------------------------

type (
	Graph            = types.Graph
	FileInfo         = types.FileInfo
	Symbol           = types.Symbol
	Import           = types.Import
	Relation         = types.Relation
	RelationEndpoint = types.RelationEndpoint
	Metadata         = types.Metadata
	UnresolvedImport = types.UnresolvedImport
	SymbolID         = types.SymbolID
	MethodKey        = types.MethodKey
	ViewParams       = types.ViewParams
	FileEntry        = index.FileEntry
	ViewData         = render.ViewData
	ViewSection      = render.ViewSection
	ViewItem         = render.ViewItem
)

// ---- language constant re-exports -----------------------------------

const (
	LangGo         = types.LangGo
	LangPython     = types.LangPython
	LangJavaScript = types.LangJavaScript
	LangTypeScript = types.LangTypeScript
	LangJava       = types.LangJava
	LangRust       = types.LangRust
	LangC          = types.LangC
	LangCpp        = types.LangCpp
)

// ---- function re-exports --------------------------------------------
//
// All exported functions below are thin wrappers that forward to the
// sub-package implementation. The indirection is kept deliberate so
// the root repomap package owns the public API surface and sub-
// packages remain free to evolve their internal signatures without
// breaking external callers.

// BuildGraph constructs the full repository graph from parsed files.
func BuildGraph(repoRoot string, files []*FileInfo) *Graph {
	return index.BuildGraph(repoRoot, files)
}

// ScanFiles discovers source files in a repository.
func ScanFiles(repoRoot string) ([]FileEntry, error) {
	return index.ScanFiles(repoRoot)
}

// ParseFiles parses the given file entries and returns a slice of
// populated FileInfo records.
func ParseFiles(entries []FileEntry, repoRoot string) []*FileInfo {
	return index.ParseFiles(entries, repoRoot)
}

// SymbolKey returns a stable "file:Receiver.Name" key for a symbol.
func SymbolKey(s *Symbol) string {
	return types.SymbolKey(s)
}

// MakeSymbolID returns the canonical drift-proof SymbolID for a
// (lang, pkg, receiver, name, arity) tuple.
func MakeSymbolID(lang, pkg, receiver, name string, arity int) SymbolID {
	return types.MakeSymbolID(lang, pkg, receiver, name, arity)
}

// SetCacheDir overrides the base directory for repo map caches.
func SetCacheDir(dir string) { index.SetCacheDir(dir) }

// IsSpecialFile reports whether a file path is a recognised project
// file (go.mod, Cargo.toml, tsconfig.json, ...).
func IsSpecialFile(name string) (bool, string) {
	return index.IsSpecialFile(name)
}

// GenerateView produces a markdown view of the graph for the given
// view type. Routes through the structured ViewData path for view
// types that have migrated and falls back to the legacy direct-
// markdown path otherwise.
func GenerateView(g *Graph, viewType string, params ViewParams) string {
	return render.GenerateView(g, viewType, params)
}

// GenerateViewData returns the structured ViewData for a view type,
// or nil when the view still uses the legacy markdown-direct path.
func GenerateViewData(g *Graph, viewType string, params ViewParams) *ViewData {
	return render.GenerateViewData(g, viewType, params)
}

// RenderMarkdown translates a structured ViewData into the
// corresponding markdown string.
func RenderMarkdown(d *ViewData) string {
	return render.RenderMarkdown(d)
}

// RankGraph scores every file in the graph by importance, optionally
// biased by a query string. Exposed so keyword-search consumers in
// the agent package can re-rank a previously built graph without
// rescanning.
func RankGraph(g *Graph, query string) {
	retrieve.RankGraph(g, query)
}

// TopFiles returns the N files with the highest score, descending.
func TopFiles(g *Graph, topN int) []*FileInfo {
	return retrieve.TopFiles(g, topN)
}
