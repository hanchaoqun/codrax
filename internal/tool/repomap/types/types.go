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
	"sync"
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
	Kind      string `json:"kind"` // function, method, type, interface, class, struct, enum, const, var, field, trait, route, … (open vocabulary; extractors add framework kinds)
	File      string `json:"file"` // relative path
	Line      int    `json:"line"` // 1-based
	EndLine   int    `json:"end_line"`
	Exported  bool   `json:"exported"`
	Receiver  string `json:"receiver,omitempty"` // for methods
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"`    // first line of doc comment
	Parent    string `json:"parent,omitempty"` // containing type/class
	Arity     int    `json:"arity,omitempty"`  // param count for functions/methods, 0 otherwise

	// RequiredMethods is the deduplicated set of "name(arity)"
	// method signatures an interface / trait declaration requires.
	// Populated only for Kind=interface / Kind=trait symbols by the
	// AST extractor; empty for concrete types. Phase 6 P0 batch
	// (2026-05-03) — typed contract data behind the Implements
	// post-pass that drives ImplementersOf queries.
	//
	// Format per entry: "<methodName>(<arity>)" — e.g.
	// "Observe(2)" for Observe(ctx, obs). Receiver-name and
	// parameter-type detail are intentionally omitted; arity is the
	// minimum precision needed for Go-style structural matching.
	// Cross-language note: Java / Kotlin use Symbol.Parent on each
	// abstract method symbol to express the same contract; Go's
	// interface_type body has no separate method symbols, so this
	// field is the canonical Go-side carrier.
	RequiredMethods []string `json:"required_methods,omitempty"`

	// Implements is the list of interface SymbolIDs that this
	// concrete type satisfies via its method set. Phase 6 P0 batch
	// (2026-05-03) — populated by the populateImplementers post-
	// pass after BuildGraph, by matching concrete-type method sets
	// against interface RequiredMethods. Empty for interface /
	// trait symbols themselves.
	//
	// Solves the structural-typing problem in Go: a type T satisfies
	// interface I iff T has every method I requires, regardless of
	// whether T's source mentions I textually. byte-grep for "I"
	// across the repo finds ~half of T's because the rest are
	// structurally-typed without an explicit "implements I" comment
	// or interface-conversion expression. Implements drives the
	// Graph.ImplementersOf API which downstream consumers (explorer,
	// concrete_values producer) read instead of byte-grep.
	Implements []SymbolID `json:"implements,omitempty"`

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
// method name for calls, the type name for type_usage, the host type
// for inheritance/embedding). `Receiver` is the raw receiver text for
// method calls — e.g. for `x.Execute()` it's `"x"`, for `pkg.Fn()`
// it's `"pkg"`. Parsers that cannot determine a receiver leave it
// empty. For FromEP, `Name` is the source-side symbol when one exists
// (the subtype for inheritance, the embedding host for embedding);
// call/type_usage sources legitimately have no symbol name and carry
// only File/Line — renderers derive the enclosing symbol positionally.
type RelationEndpoint struct {
	ID       SymbolID `json:"id,omitempty"`
	Name     string   `json:"name,omitempty"`
	Receiver string   `json:"receiver,omitempty"`
	File     string   `json:"file,omitempty"`
	Line     int      `json:"line,omitempty"`
}

// Relation provenance values: the construction-stage origin of an
// edge, persisted on every relation. Resolution-stage attribution
// (MethodIndex hits, name-only fallbacks) is computed at query/render
// time and never persisted — fullScan streams relations into the
// cache during parsing, before BuildGraph indices exist, so a
// persisted resolution claim would diverge between scan modes.
const (
	// ProvenanceTreeSitter marks edges read from a primary or
	// secondary tree-sitter grammar AST.
	ProvenanceTreeSitter = "tree_sitter"
	// ProvenanceCangjieParser marks edges from the hand-rolled
	// Cangjie recursive-descent parser (AST-grade precision).
	ProvenanceCangjieParser = "cangjie_parser"
	// ProvenanceRegexFallback marks edges salvaged by regex when no
	// AST was available for the construct.
	ProvenanceRegexFallback = "regex_fallback"
	// ProvenanceRouteResolver marks framework route -> handler edges
	// recognised from static registration forms.
	ProvenanceRouteResolver = "route_resolver"
)

// Relation confidence values share ONE scale with the parse-tier
// discounts (index.TierDiscount): a second numeric vocabulary for the
// same parse-quality axis would drift. Confidence is a NOISY signal —
// it may shade rendering and soft rank discounting only, and MUST
// NEVER feed an emit-time hard reject, contract.Check gate, or any
// value-threshold comparison (architectural red line: precise signals
// for hard gates). Load validation checks presence only
// (Confidence > 0): encoding/json cannot distinguish a missing
// float64 from 0.0, and producers are forbidden from writing 0.0 —
// an edge whose confidence cannot be honestly assigned is not
// emitted at all.
const (
	// ConfidenceAST: primary-grammar AST edges (tree-sitter Tier 1,
	// Cangjie parser). Mirrors TierDiscount(1).
	ConfidenceAST = 1.00
	// ConfidenceSecondaryGrammar: edges parsed by a neighbouring
	// grammar (ArkTS riding the TS grammar). Mirrors TierDiscount(2).
	ConfidenceSecondaryGrammar = 0.85
	// ConfidenceRouteLiteral: framework route edges recognised from
	// literal registration forms (P0.4 lane).
	ConfidenceRouteLiteral = 0.70
	// ConfidenceRegexSalvage: regex-recovered edges. Mirrors
	// TierDiscount(3).
	ConfidenceRegexSalvage = 0.60
	// ConfidencePathOnly: path-only weak signals (no producer today;
	// reserved). Mirrors TierDiscount(4+).
	ConfidencePathOnly = 0.30
)

// Relation represents a relationship between code entities. Every
// durable relation carries structured endpoints plus the three
// construction-stage diagnostic fields; cache loads reject payloads
// missing any of them (presence check, schema v4+).
type Relation struct {
	Kind string `json:"kind"` // call, reference, type_usage, inheritance, embedding

	// FromEP is the source endpoint. Name is set when the source is
	// a symbol (inheritance subtype, embedding host); call and
	// type_usage sources carry File/Line only.
	FromEP RelationEndpoint `json:"from_ep,omitempty"`
	// ToEP is the target endpoint: the callee for calls, the
	// supertype for inheritance, the used type for type_usage.
	ToEP RelationEndpoint `json:"to_ep,omitempty"`

	File string `json:"file"` // file where the relation is observed
	Line int    `json:"line"`

	// Confidence, Provenance and ResolvedBy are mandatory
	// construction-stage diagnostics. Confidence values come from the
	// Confidence* constants above (single scale with TierDiscount);
	// Provenance from the Provenance* constants; ResolvedBy is the
	// extractor-granular origin (e.g. "go_ast_selector_call",
	// "java_superclass", "gin_route_literal").
	Confidence float64           `json:"confidence"`
	Provenance string            `json:"provenance"`
	ResolvedBy string            `json:"resolved_by"`
	Metadata   map[string]string `json:"metadata,omitempty"`
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
	// symbols. 1 = primary grammar (best; for ArkTS the TS grammar
	// succeeding IS the primary path by chain decision); 2 = a
	// genuine secondary grammar (reserved — no current producer);
	// 3 = regex-only; 4 = path-only (no symbols at all). Languages
	// with a single grammar (Go, Java, Rust, …) leave this at 0
	// (== "not applicable, treat as Tier 1").
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

	// LineFeatureAssignment — variable / property assignment shape:
	// `x = y`, `x := y`, `obj.field = y`, `val x = y`,
	// `let x = y`, `const x = y`, etc. This is a source-shape
	// feature, not a semantic dataflow guarantee.
	LineFeatureAssignment LineFeature = "assignment"

	// LineFeatureMemberInitializer — field/object/designated
	// initializer entry inside a composite value: Go
	// `Field: value`, JS/TS/ArkTS `{ field: value }`, Rust
	// `field: value`, C/C++ `.field = value`, and equivalent
	// language-specific named-member initializer surfaces. This lets
	// downstream assignment evidence carry visible member labels
	// without pretending those labels are symbol definitions.
	LineFeatureMemberInitializer LineFeature = "member_initializer"

	// LineFeatureArrowFunction — JS / TS arrow `(x) => y` or
	// Rust closure shape. Lambda body marker.
	LineFeatureArrowFunction LineFeature = "arrow_function"

	// LineFeatureUnknownEffect — line contains a runtime-dynamic
	// dispatch / reflection / unsafe escape that defeats static
	// analysis. Phase 6 stage 27 (2026-05-03) typed replacement
	// for the per-language byte-token scans in
	// internal/analysis/dataflow/lower.go::detectUnknownEffect.
	//
	// Per-language patterns the AST walker recognises:
	//   - Go: defer_statement; call to reflect.* / unsafe.*
	//   - Python: call to getattr / setattr / importlib.*
	//   - JS/TS/ArkTS: call to Reflect.* / require with binary
	//     concatenation; member access expression with computed
	//     subscript followed by call (`obj[name]()` shape)
	//   - Java/Kotlin: call to Class.forName / java.lang.reflect.*
	//     / `::class` reference
	//   - Rust: macro_invocation; unsafe_block
	//   - Ruby: call to send / public_send / const_get /
	//     method_missing
	//   - Swift: call to Mirror / NSClassFromString /
	//     unsafeBitCast
	//   - Lua: call to load / loadfile / loadstring; subscript
	//     of _G
	//   - C/C++: pointer_expression (`->` / `*p`)
	//
	// Empty / Tier 3+ regex-only fallback files: this feature is
	// not emitted ⇒ detectUnknownEffect reports no escape ⇒
	// dataflow lowering proceeds without the unknown-effect flag.
	// Same typed-only contract as stages 18+.
	LineFeatureUnknownEffect LineFeature = "unknown_effect"

	// LineFeatureGuard — line opens a control-flow guard /
	// branch construct (if / else if / elif / unless / guard /
	// switch / case / when / match / until). Phase 6 stage 28
	// (2026-05-03) typed replacement for the byte-prefix table
	// in dataflow.detectGuard.
	//
	// Per-language tree-sitter node names this maps from:
	//   - if_statement (Go, Java, Rust, JS/TS, Swift, Python's
	//     `if_statement`, etc.)
	//   - else_clause / else_if_clause / elif_clause
	//   - switch_statement / switch_expression
	//   - case_clause / when_entry / match_arm
	//   - guard_statement (Swift)
	//   - until_statement (Ruby / Lua)
	//   - unless_statement (Ruby)
	//   - ternary_expression (`a ? b : c`)
	LineFeatureGuard LineFeature = "guard"
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
// (return / call / new / composite literal / assignment /
// member-initializer / arrow function).
// Phase 6 stage 18 typed replacement for the explorer's
// isEvidenceLine token tables.
func (f LineFeature) IsEvidenceShape() bool {
	switch f {
	case LineFeatureReturnStmt,
		LineFeatureCallExpression,
		LineFeatureNewExpression,
		LineFeatureCompositeLiteral,
		LineFeatureAssignment,
		LineFeatureMemberInitializer,
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
	RankIndex      *RankIndex            `json:"-"` // derived structural ranking index; rebuilt from Files/graph edges
	Scores         map[string]float64    `json:"-"` // key → importance score
	QueryScores    map[string]float64    `json:"-"` // key → query match score (>0 only for files matching the query)
	Metadata       Metadata              `json:"metadata"`

	// flatSymbolOnce / flatSymbolIndex memoize the flattened-identifier
	// lookup table at the GRAPH level. Oracles are constructed per
	// consumer (analyzer, every contract-check round, multigraph
	// fan-out); before this memo each oracle rebuilt the index from all
	// SymbolDefs — O(symbols) work plus one FlattenIdentifier per name —
	// on its first flat lookup. Unexported: never serialized; go vet's
	// copylocks check enforces that Graph stays pointer-passed.
	flatSymbolOnce  sync.Once      `json:"-"`
	flatSymbolIndex map[string]int `json:"-"`

	// receiverNameOnce / receiverNameIndex memoize the cross-package
	// (receiver, name) → first-defining symbol lookup used by the
	// call-target resolver's cross-package fallback. Before this index
	// that fallback scanned the entire MethodIndex (one entry per
	// method — tens of thousands) for EVERY unresolved call relation,
	// which made BuildGraph quadratic on large repos (~19s on the
	// codrax tree). Built once in file order so the pick is
	// deterministic (the old map-iteration pick was random).
	receiverNameOnce  sync.Once                   `json:"-"`
	receiverNameIndex map[receiverNameKey]*Symbol `json:"-"`
}

// receiverNameKey indexes a method/function by its receiver (or parent
// type) and name, ignoring package — the shape the cross-package call
// fallback resolves against.
type receiverNameKey struct {
	Receiver string
	Name     string
}

// resolveReceiverName returns the first symbol (in file order) whose
// receiver/parent and name match, memoized across the graph's lifetime.
func (g *Graph) resolveReceiverName(receiver, name string) *Symbol {
	if g == nil || name == "" {
		return nil
	}
	g.receiverNameOnce.Do(func() {
		idx := make(map[receiverNameKey]*Symbol)
		for _, fi := range g.Files {
			if fi == nil {
				continue
			}
			for i := range fi.Symbols {
				sym := &fi.Symbols[i]
				recv := sym.Receiver
				if recv == "" {
					recv = sym.Parent
				}
				if recv == "" || sym.Name == "" {
					continue
				}
				key := receiverNameKey{Receiver: recv, Name: sym.Name}
				if _, exists := idx[key]; !exists {
					idx[key] = sym
				}
			}
		}
		g.receiverNameIndex = idx
	})
	return g.receiverNameIndex[receiverNameKey{Receiver: receiver, Name: name}]
}

// FlatSymbolIndex returns the memoized flattened-identifier index,
// building it once per Graph via the supplied constructor. The
// constructor runs at most once across all oracles sharing this graph.
func (g *Graph) FlatSymbolIndex(build func() map[string]int) map[string]int {
	if g == nil {
		return nil
	}
	g.flatSymbolOnce.Do(func() {
		g.flatSymbolIndex = build()
	})
	return g.flatSymbolIndex
}

// RankIndex is the query-independent structural index used by
// retrieve.RankGraph. It is deliberately not persisted: cache files
// store source-level FileInfo records, and this compact index is
// rebuilt after BuildGraph resolves imports and symbol IDs.
//
// The index replaces several large temporary maps that RankGraph used
// to rebuild on every query: symbol-name ambiguity, call/reference
// fan-in counters, fan-out dampening input, and entrypoint detection.
type RankIndex struct {
	SymbolDefCount map[string]int
	CallCountByID  map[SymbolID]int
	RefCountByID   map[SymbolID]int
	CallCount      map[string]int
	RefCount       map[string]int
	FileFanout     map[string]float64
	Entrypoints    map[string]bool
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

	// FallbackFileCount is how many files were parsed below Tier 1
	// (ParseTier >= 2): their symbol/relation coverage may be
	// incomplete. Counted once at BuildGraph metadata assembly;
	// surfaced by the index-status banner.
	FallbackFileCount int `json:"fallback_file_count,omitempty"`

	// IndexStatus is the per-call observation of how THIS graph was
	// obtained. Runtime-only (never persisted — the cache stores
	// FileInfos, and a freshness claim must not outlive the call
	// that observed it). Metadata is value-copied by graph clones,
	// so reuse paths inherit and overwrite just the status.
	IndexStatus RepoMapIndexStatus `json:"-"`
}

// RepoMapIndexStatus reports how the served graph came to be, so the
// reading model can judge how much to lean on it. Freshness is a
// 3-state observation: "fresh" = this call reconciled the graph
// against on-disk content (full scan, hash-verified cache hit, or
// incremental merge); "reused" = served from in-memory state without
// re-reconciling (LRU resident clone, scoped projection); "" =
// unknown (zero value — renderers must say "unknown", never fabricate
// a source). There is deliberately NO "stale" state: disk loads
// always hash-reconcile in the same call, and noisier signals (HEAD
// drift) may inform fact lines only, never warnings (precise-signals
// red line).
type RepoMapIndexStatus struct {
	Source         string // IndexSource* constant; "" = unknown
	Freshness      string // IndexFreshness* constant; "" = unknown
	CacheWrittenAt string // manifest written_at when loaded from cache
	RepoHead       string // manifest repo_head when loaded from cache
	SchemaVersion  int    // manifest schema version when loaded from cache
	// RebuildReasons is non-empty when this call's full scan was
	// forced by a rejected cache (typed loader reasons).
	RebuildReasons []string
}

// IndexSource* name how the served graph was obtained. Neutral
// vocabulary: these strings are model-visible in the index-status
// banner and must not leak internal carrier terminology.
const (
	IndexSourceFullScan         = "full_scan"
	IndexSourceCacheHit         = "cache_hit"
	IndexSourceIncremental      = "incremental"
	IndexSourceInMemoryReuse    = "in_memory_reuse"
	IndexSourceScopedProjection = "scoped_projection"
)

// IndexFreshness* values for RepoMapIndexStatus.Freshness.
const (
	IndexFreshnessFresh  = "fresh"
	IndexFreshnessReused = "reused"
)

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
	Query                   string   // search query for task_map / relation_map source discovery
	TargetFile              string   // file path for edit_impact
	EntryPoint              string   // symbol or file for call_path
	Sources                 []string // source files/symbols/scopes for relation_map
	Scopes                  []string // repo-relative scopes for relation_map
	RelationKinds           []string // relation families for relation_map
	TopN                    int      // max items to show (0 = default)
	ShowSourceInventoryHint bool     // overview-only navigation hint for later-stage agents
	ViewProgress            func(step string, done, total int)
}
