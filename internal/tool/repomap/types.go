package repomap

import (
	"strconv"
	"strings"
	"time"
)

// SymbolID is the canonical, drift-proof identity for a symbol. Format:
//
//	<lang>::<pkg>::<receiver>::<name>::<arity>
//
// Every segment uses the ASCII `::` separator. Empty segments are
// rendered as the empty string; a bare function in package "agent"
// called `buildAnalysisIR` with 1 param is `go::agent::::buildAnalysisIR::1`.
//
// Arity is the parameter count for functions/methods and 0 for types,
// consts, vars, and fields. Go disallows overloading so package +
// receiver + name is already unique for Go; arity is kept for
// consistency across languages that allow overloading (Java, C++)
// and so the format is self-describing.
//
// SymbolID is a string alias rather than a struct so it can be used
// as a map key directly, JSON-serialized trivially, and compared
// with ==. Callers should not construct IDs by string concatenation;
// use MakeSymbolID.
type SymbolID string

// MakeSymbolID builds a canonical SymbolID. `lang` is the repomap
// language tag ("go", "python", "java", ...). `pkg` is the containing
// package/module; empty for single-file scripts. `receiver` is the
// containing type for methods; empty for bare functions and types.
// `arity` is the parameter count; pass 0 for non-callable symbols.
//
// Input strings are NOT sanitized — the caller is responsible for
// passing clean segments. Tree-sitter extractors satisfy this by
// construction; external callers should not build IDs manually.
func MakeSymbolID(lang, pkg, receiver, name string, arity int) SymbolID {
	var b strings.Builder
	b.Grow(len(lang) + len(pkg) + len(receiver) + len(name) + 10)
	b.WriteString(lang)
	b.WriteString("::")
	b.WriteString(pkg)
	b.WriteString("::")
	b.WriteString(receiver)
	b.WriteString("::")
	b.WriteString(name)
	b.WriteString("::")
	b.WriteString(strconv.Itoa(arity))
	return SymbolID(b.String())
}

// Symbol represents an extracted code symbol.
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // package, function, method, type, interface, class, struct, enum, const, var, field, trait
	File     string `json:"file"` // relative path
	Line     int    `json:"line"` // 1-based
	EndLine  int    `json:"end_line"`
	Exported bool   `json:"exported"`
	Receiver string `json:"receiver,omitempty"` // for methods
	Signature string `json:"signature,omitempty"`
	Doc      string `json:"doc,omitempty"` // first line of doc comment
	Parent   string `json:"parent,omitempty"` // containing type/class
	Arity    int    `json:"arity,omitempty"` // param count for functions/methods, 0 otherwise

	// ID is the canonical drift-proof identity. Re-derived at
	// BuildGraph time from Name/Receiver/Parent/Arity + the containing
	// FileInfo.Language and FileInfo.Package, so it is NOT persisted
	// in the cache — omitted from JSON to keep the cache schema
	// stable across versions that introduce the ID format.
	ID SymbolID `json:"-"`
}

// Import represents an import/include/require statement.
type Import struct {
	Raw   string `json:"raw"`   // raw import string as written
	Path  string `json:"path"`  // cleaned path
	Alias string `json:"alias,omitempty"`
	File  string `json:"file"`  // which file contains this import
	Line  int    `json:"line"`
}

// RelationEndpoint is the canonical endpoint of a Relation. `ID` is
// populated when the endpoint resolves to a single SymbolID; empty
// when unresolved (external calls, unknown receiver variables,
// package-level calls that fail type lookup, etc.).
//
// `Name` is the raw identifier text at the source location (the
// method name for calls, the type name for type_usage, the import
// path for imports). `Receiver` is the raw receiver text for method
// calls — e.g. for `x.Execute()` it's `"x"`, for `pkg.Fn()` it's
// `"pkg"`. Parsers that cannot determine a receiver leave it empty.
//
// Introduced in Phase 1 to replace the flat `Relation.From/To`
// string carrier; the legacy strings remain in parallel until the
// P1.4 deletion step so consumers can migrate without flag days.
type RelationEndpoint struct {
	ID       SymbolID `json:"id,omitempty"`
	Name     string   `json:"name,omitempty"`
	Receiver string   `json:"receiver,omitempty"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
}

// Relation represents a relationship between code entities.
type Relation struct {
	Kind string `json:"kind"` // import, call, reference, type_usage, inheritance, embedding
	From string `json:"from"` // file or file:symbol (legacy string form, deleted in P1.4)
	To   string `json:"to"`   // file or file:symbol (legacy string form, deleted in P1.4)
	File string `json:"file"` // file where the relation is observed
	Line int    `json:"line"`

	// ToEP is the structured endpoint populated by Phase 1+ extractors.
	// Carries the receiver text (for method calls) and, once resolved,
	// the SymbolID of the target. P1.2b rewrites CallersOf/rank to
	// read ToEP. Legacy consumers still read To.
	ToEP RelationEndpoint `json:"to_ep,omitempty"`
}

// FileInfo holds all extracted data for a single source file.
type FileInfo struct {
	RelPath     string     `json:"rel_path"`
	Language    string     `json:"language"`
	Package     string     `json:"package,omitempty"`
	Size        int64      `json:"size"`
	Hash        string     `json:"hash"` // content hash for cache invalidation
	Symbols     []Symbol   `json:"symbols,omitempty"`
	Imports     []Import   `json:"imports,omitempty"`
	Relations   []Relation `json:"relations,omitempty"`
	IsSpecial   bool       `json:"is_special,omitempty"`
	SpecialType string     `json:"special_type,omitempty"` // build_config, dockerfile, ci, etc.
}

// MethodKey is the (package, receiver, name) tuple used to resolve
// a call site to a concrete method without full type inference.
// Because Go disallows overloading, the tuple is unique per package;
// in languages that allow overloading two entries may collide and
// the first-wins policy from SymbolByID applies.
type MethodKey struct {
	Pkg      string
	Receiver string // empty for bare package-level functions
	Name     string
}

// Graph is the complete repository index.
type Graph struct {
	Root        string                `json:"root"`
	Files       []*FileInfo           `json:"-"`
	FileIndex   map[string]*FileInfo  `json:"-"` // rel path → FileInfo
	SymbolDefs  map[string][]*Symbol  `json:"-"` // symbol name → all definitions (legacy; kept while consumers migrate)
	SymbolByID  map[SymbolID]*Symbol  `json:"-"` // canonical drift-proof index: one SymbolID → one definition
	MethodIndex map[MethodKey]*Symbol `json:"-"` // (pkg, receiver, name) → method def; used by the receiver-aware call resolver
	ImportGraph map[string][]string   `json:"-"` // file → imported file paths
	ReverseImports map[string][]string `json:"-"` // file → files that import it
	Scores      map[string]float64    `json:"-"` // key → importance score
	QueryScores map[string]float64    `json:"-"` // key → query match score (>0 only for files matching the query)
	Metadata    Metadata              `json:"metadata"`
}

// Metadata holds scan-level statistics.
type Metadata struct {
	ScanTime     time.Time      `json:"scan_time"`
	FileCount    int            `json:"file_count"`
	SymbolCount  int            `json:"symbol_count"`
	RelationCount int           `json:"relation_count"`
	Languages    map[string]int `json:"languages"`    // language → file count
	SpecialFiles []string       `json:"special_files"` // notable files (go.mod, etc.)
}

// ViewParams controls what a view generator produces.
type ViewParams struct {
	Query      string // search query for task_map
	TargetFile string // file path for edit_impact
	EntryPoint string // symbol or file for call_path
	TopN       int    // max items to show (0 = default)
}
