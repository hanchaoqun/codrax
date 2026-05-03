// Package types defines the core data types and Graph query methods
// that are shared by every other repomap sub-package (index, retrieve,
// render). It holds:
//
//   - structural types: Graph, FileInfo, Symbol, Relation, Import,
//     Metadata, UnresolvedImport, SymbolID, MethodKey, RelationEndpoint,
//     ViewParams
//   - language constants + DetectLanguage / GetSitterLanguage / IsExported
//   - MakeSymbolID / DeriveSymbolID / SymbolKey / AppendUnique helpers
//   - Graph methods for in-graph navigation: FilesImporting,
//     FilesImportedBy, SymbolsInFile, CallersOf, CallersOfID,
//     ResolveCallTarget, TransitiveDeps, TransitiveReverseDeps
//
// Methods are defined here rather than in retrieve/ because Go
// requires receiver methods to live in the same package as the
// receiver type. retrieve/ therefore treats types.Graph as opaque
// and uses its methods for navigation, adding rank/query functions
// of its own as free functions.
package types

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
	Name      string `json:"name"`
	Kind      string `json:"kind"` // package, function, method, type, interface, class, struct, enum, const, var, field, trait
	File      string `json:"file"` // relative path
	Line      int    `json:"line"` // 1-based
	EndLine   int    `json:"end_line"`
	Exported  bool   `json:"exported"`
	Receiver  string `json:"receiver,omitempty"` // for methods
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"`    // first line of doc comment
	Parent    string `json:"parent,omitempty"` // containing type/class
	Arity     int    `json:"arity,omitempty"`  // param count for functions/methods, 0 otherwise

	// ReturnTypeNames is the deduplicated set of bare type-name
	// tokens parsed from this function/method's return signature.
	// Phase 6 stage 20 (2026-05-03) replacement for the retired
	// IdentifierFactoryPrefixes naming-convention heuristic. By
	// reading the return type structurally (not guessing from
	// `New` / `Create` / etc. prefix), the system gets:
	//
	//   - Correct positives: any function returning *Foo / Foo /
	//     []Foo / Box<Foo> / Result<Foo, E> contributes "Foo" to
	//     ReturnTypeNames regardless of function name. Cross-language
	//     factory naming (Chinese 创建Foo, Rust Foo::new, Java Bean
	//     getFoo) all caught.
	//
	//   - Correct negatives: getFooDescription returning string
	//     does NOT contribute "Foo" — the prefix table would have
	//     incorrectly matched it.
	//
	// Empty for non-functions, methods without return types
	// (procedures), and Tier 3+ regex-only fallback parses where
	// return-type extraction is unavailable. callers consume via
	// containsIdentifier in internal/agent/explorer.go.
	//
	// Wrapping types (pointer, array, slice, generic) are stripped
	// to bare type names. See returnTypeNamesFromGoType /
	// equivalents in extractors. Multiple return values
	// contribute distinct entries deduplicated case-sensitively.
	ReturnTypeNames []string `json:"return_type_names,omitempty"`

	// ID is the canonical drift-proof identity. Re-derived at
	// BuildGraph time from Name/Receiver/Parent/Arity + the containing
	// FileInfo.Language and FileInfo.Package, so it is NOT persisted
	// in the cache — omitted from JSON to keep the cache schema
	// stable across versions that introduce the ID format.
	ID SymbolID `json:"-"`
}

// Import represents an import/include/require statement.
type Import struct {
	Raw   string `json:"raw"`  // raw import string as written
	Path  string `json:"path"` // cleaned path
	Alias string `json:"alias,omitempty"`
	File  string `json:"file"` // which file contains this import
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

	// ParseTier records which fallback tier produced this FileInfo's
	// symbols. 1 = primary grammar (best); 2 = secondary grammar
	// (e.g. ArkTS riding on TS); 3 = regex-only; 4 = path-only (no
	// symbols at all). Languages with a single grammar (Go, Java,
	// Rust, …) leave this at 0 (== "not applicable, treat as Tier 1").
	// Used by retrieve.rank to discount lower-confidence parses so
	// they cannot outrank fully-parsed Tier-1 files.
	//
	// See internal/tool/repomap/index/parse_fallback.go for the
	// canonical tier assignment + the rank discount weights.
	ParseTier int `json:"parse_tier,omitempty"`

	// FallbackReason is a single-line free-form note attached to
	// downgrades (Tier > 1). Empty for Tier 1 files. Surfaced into
	// the build log at WARN level (red line L-Fallback-1 — no
	// silent degradation).
	FallbackReason string `json:"fallback_reason,omitempty"`

	// LineFeatures is the per-line typed AST node-shape index
	// populated by tree-sitter extractors. Keyed by 1-based line
	// number; the slice contains every distinct LineFeature value
	// observed on that line. Phase 6 stage 18 (2026-05-03):
	// replaces the source-shape token tables that used to drive
	// concrete_values producer's isEvidenceLine and the decision-
	// block scanner's isBlockTerminator. Empty / nil map means
	// "AST features not available" (regex-only Tier 3+ fallback);
	// callers treat absence as "no signal" rather than guessing.
	LineFeatures map[int][]LineFeature `json:"line_features,omitempty"`
}

// LineFeature tags a typed AST node-shape observation at a
// specific source line. Phase 6 stage 18 (2026-05-03) replacement
// for the explorer's source-shape token tables.
//
// The closed enum normalises across tree-sitter grammars: Go's
// `return_statement`, Python's `return_statement`, Rust's
// `return_expression` all collapse to LineFeatureReturnStmt;
// similarly for break / raise / throw. Per-language extractors
// own the mapping and only emit values from this enum.
//
// Values are intentionally coarse — they describe SHAPE, not
// semantics. Whether a `call_expression` is a registration call
// vs a regular function call is a separate concern decided
// downstream by matching the call's target name against typed
// helpers (e.g. yaml-tunable RegistrationFunctionNameTokens).
type LineFeature string

const (
	// LineFeatureReturnStmt — `return X` / `return X, err` / bare
	// `return`. Maps to tree-sitter return_statement /
	// return_expression depending on grammar.
	LineFeatureReturnStmt LineFeature = "return_stmt"

	// LineFeatureBreakStmt — `break` / `break label`. Loop
	// terminator.
	LineFeatureBreakStmt LineFeature = "break_stmt"

	// LineFeatureRaiseStmt — `raise X` (Python). Exception terminator.
	LineFeatureRaiseStmt LineFeature = "raise_stmt"

	// LineFeatureThrowStmt — `throw X` (Java / JS / TS / Rust
	// macro `throw_stmt!`). Exception terminator. Held distinct
	// from RaiseStmt for grammars that have both shapes.
	LineFeatureThrowStmt LineFeature = "throw_stmt"

	// LineFeatureCallExpression — function or method call
	// `foo(...)` / `obj.method(...)`. Used by concrete_values
	// producer to flag registration-shape lines via call-target
	// name lookup against typed helpers.
	LineFeatureCallExpression LineFeature = "call_expression"

	// LineFeatureNewExpression — constructor invocation `new Foo(...)`
	// (Java / JS / TS / C++) or factory-prefix call (Go's NewFoo,
	// CreateFoo, MakeFoo). Composite-value creation marker.
	LineFeatureNewExpression LineFeature = "new_expression"

	// LineFeatureCompositeLiteral — `Type{...}` / `[]Type{...}` /
	// `&Type{...}` (Go), `Foo { field: ... }` (Rust),
	// `{ key: val }` (JS object literal). Establishes a new
	// composite value.
	LineFeatureCompositeLiteral LineFeature = "composite_literal"

	// LineFeatureArrowFunction — JS / TS arrow `(x) => y` or
	// Rust closure shape. Lambda body marker.
	LineFeatureArrowFunction LineFeature = "arrow_function"
)

// IsBlockTerminator reports whether `f` is a control-flow
// terminator shape (return / break / raise / throw). Phase 6
// stage 18 typed replacement for the explorer's
// isBlockTerminator string-prefix table.
func (f LineFeature) IsBlockTerminator() bool {
	switch f {
	case LineFeatureReturnStmt, LineFeatureBreakStmt,
		LineFeatureRaiseStmt, LineFeatureThrowStmt:
		return true
	}
	return false
}

// IsEvidenceShape reports whether `f` describes a source-line
// shape worth deeper analysis by the concrete_values producer
// (return / call / new / composite literal / arrow function).
// Phase 6 stage 18 typed replacement for the explorer's
// isEvidenceLine token tables.
func (f LineFeature) IsEvidenceShape() bool {
	switch f {
	case LineFeatureReturnStmt,
		LineFeatureCallExpression,
		LineFeatureNewExpression,
		LineFeatureCompositeLiteral,
		LineFeatureArrowFunction:
		return true
	}
	return false
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
	Root           string                `json:"root"`
	Files          []*FileInfo           `json:"-"`
	FileIndex      map[string]*FileInfo  `json:"-"` // rel path → FileInfo
	SymbolDefs     map[string][]*Symbol  `json:"-"` // symbol name → all definitions (legacy; kept while consumers migrate)
	SymbolByID     map[SymbolID]*Symbol  `json:"-"` // canonical drift-proof index: one SymbolID → one definition
	MethodIndex    map[MethodKey]*Symbol `json:"-"` // (pkg, receiver, name) → method def; used by the receiver-aware call resolver
	ImportGraph    map[string][]string   `json:"-"` // file → imported file paths
	ReverseImports map[string][]string   `json:"-"` // file → files that import it
	Scores         map[string]float64    `json:"-"` // key → importance score
	QueryScores    map[string]float64    `json:"-"` // key → query match score (>0 only for files matching the query)
	Metadata       Metadata              `json:"metadata"`
}

// Metadata holds scan-level statistics.
type Metadata struct {
	ScanTime          time.Time          `json:"scan_time"`
	FileCount         int                `json:"file_count"`
	SymbolCount       int                `json:"symbol_count"`
	RelationCount     int                `json:"relation_count"`
	Languages         map[string]int     `json:"languages"`     // language → file count
	SpecialFiles      []string           `json:"special_files"` // notable files (go.mod, etc.)
	UnresolvedImports []UnresolvedImport `json:"unresolved_imports,omitempty"`
}

// UnresolvedImport records an import statement that no registered
// ImportResolver could map to a target file in the graph. Populated
// by resolveImportGraph so consumers (the eval harness, diagnostics)
// can compute per-import import_edge_accuracy without re-walking the
// resolver.
type UnresolvedImport struct {
	File   string `json:"file"`             // source file RelPath
	Raw    string `json:"raw"`              // imp.Path as written
	Reason string `json:"reason,omitempty"` // resolver language / failure tag
}

// ViewParams controls what a view generator produces.
type ViewParams struct {
	Query      string // search query for task_map
	TargetFile string // file path for edit_impact
	EntryPoint string // symbol or file for call_path
	TopN       int    // max items to show (0 = default)
}
