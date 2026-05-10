// Package agent — qualified_name.go (2026-05-10).
//
// Multi-language qualified-name normalisation for entity → symbol
// lookup against repomap.Graph.SymbolDefs. The graph stores symbols
// by their bare name (e.g. "Run", "RunWith", "GateCheck"), but
// LLM-emitted analyzer entities carry package / namespace / scope
// qualifiers (e.g. "gate.Run", "Foo::Bar::baz", "Module#method").
//
// Without this normalisation, forEachMatchingDef silently fails to
// match a dotted entity against any graph symbol, then downstream
// gates (primaryEntityFiles, evidence filter) miss the cited file
// and downstream rendering shows unrelated typed_graph noise to the
// finalizer. s1a is the canonical failure: AnalyzerHints.Entities =
// ["gate.Run", "GateCheck", "retryable"], graph keys "Run" / "RunWith"
// / "GateCheck" — only "GateCheck" matched, primaryFiles narrowed to
// the wrong file, every grounded explorer evidence about the real
// 9 checks was filtered away.
//
// ── Cross-language separator coverage ──
//
// Languages and their fully-qualified-name conventions:
//
//	Go              .   pkg.Func, recv.Method (recv stored separately)
//	Java / Kotlin   .   com.foo.Bar.method
//	Python          .   module.Class.method
//	Swift           .   Module.Type.method
//	Scala           .   pkg.Class.method
//	ArkTS / TS / JS .   module.Class.method
//	Cangjie         .   pkg.Type.method
//	C++             ::  std::vector::push_back
//	Rust            ::  mod::Type::method
//	Ruby            ::  Foo::Bar (module separator)
//	                #   Foo::Bar#baz (instance method)
//	                .   Foo::Bar.baz (class method)
//	C               .   rare; struct field access only
//	Proto           .   package.MessageType.field
//
// All separators are normalised to a single canonical form before
// segmentation: `::` → `.`, `#` → `.`. Mixed forms like
// `Foo::Bar#baz` correctly produce segments [Foo, Bar, baz].
package agent

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// expandEntityNameAliases produces the lowercased lookup keys for an
// entity that may carry a package / namespace / scope qualifier.
//
// Returns:
//   - keys:   one or more lowercased keys to look up in
//     graph.SymbolDefs. Always includes the original (lowercased)
//     entity. When the entity has separator-delimited segments,
//     each strict-suffix sub-name is added (e.g. "a.b.c" yields
//     "a.b.c", "c", "b.c"). Ordered most-specific → least-specific
//     so callers can prefer specificity if they want, though the
//     caller in forEachMatchingDef treats all keys equally and
//     disambiguates via prefix.
//   - prefix: the lowercased package / namespace / parent-type
//     segments that PRECEDE the bare-name part. The caller checks
//     these against Symbol.Receiver / Symbol.Parent / FileInfo.Package
//     to disambiguate "which Run did the user mean".
//     Empty when the entity has no separator.
//
// Examples (return values shown lowercased):
//
//	"gate.Run"           → keys=["gate.run", "run"], prefix=["gate"]
//	"RunWith"            → keys=["runwith"], prefix=[]
//	"Foo::Bar::baz"      → keys=["foo.bar.baz", "baz", "bar.baz"], prefix=["foo","bar"]
//	"com.foo.Bar.method" → keys=["com.foo.bar.method", "method", "bar.method", "foo.bar.method"], prefix=["com","foo","bar"]
//	"Foo::Bar#baz"       → keys=["foo.bar.baz", "baz", "bar.baz"], prefix=["foo","bar"]
//	""                   → keys=nil, prefix=nil
//
// Note: keys do NOT include the empty string. Internal whitespace
// is trimmed segment-by-segment.
func expandEntityNameAliases(entity string) (keys []string, prefix []string) {
	entity = strings.TrimSpace(entity)
	if entity == "" {
		return nil, nil
	}
	// Normalise: `::` and `#` → `.` (single separator). Don't touch
	// `->` because in the rare C++ pointer-member-access form,
	// `obj->method` always has the bare method name as a graph
	// symbol, not a multi-segment chain. Treating `->` as a
	// separator would split `Object->Field` into wrong segments.
	canon := strings.ReplaceAll(entity, "::", ".")
	canon = strings.ReplaceAll(canon, "#", ".")

	// Split into segments and drop blanks (defensive — `..` would
	// otherwise produce an empty segment).
	rawSegments := strings.Split(canon, ".")
	segments := make([]string, 0, len(rawSegments))
	for _, s := range rawSegments {
		if t := strings.TrimSpace(s); t != "" {
			segments = append(segments, t)
		}
	}
	if len(segments) == 0 {
		return nil, nil
	}

	// Always include the canonical (joined) form. Use lowercased.
	full := strings.ToLower(strings.Join(segments, "."))
	seen := map[string]bool{full: true}
	keys = []string{full}

	// When multi-segment, also emit each strict-suffix slice as a
	// candidate key. The bare last segment ("c" for "a.b.c") is
	// the most-likely match in graph.SymbolDefs because the graph
	// stores symbols by bare name. Multi-segment suffixes ("b.c")
	// catch graph keys that happen to embed a dot (rare but seen
	// in proto-generated identifiers).
	for i := 1; i < len(segments); i++ {
		suffix := strings.ToLower(strings.Join(segments[i:], "."))
		if suffix == "" || seen[suffix] {
			continue
		}
		seen[suffix] = true
		keys = append(keys, suffix)
	}

	// Prefix = everything before the last segment. Lowercased.
	if len(segments) > 1 {
		prefix = make([]string, 0, len(segments)-1)
		for _, p := range segments[:len(segments)-1] {
			prefix = append(prefix, strings.ToLower(p))
		}
	}
	return keys, prefix
}

// symbolMatchesQualifier reports whether `sym` plausibly belongs to
// any of the prefix segments parsed from a qualified entity name.
//
// "Plausibly belongs to" is loose by design — better recall than
// precision, because the downstream evidence filter only acts when
// AT LEAST one primary file is found, and a too-strict qualifier
// match here would silently reject a legitimate match and fall
// back to the analyzer's PrimaryEntities-only list (the very state
// this function exists to fix).
//
// A symbol matches when ANY prefix segment matches:
//
//   - Symbol.Receiver (lowercased) — Go method on a receiver type.
//     "gate.Run" + Symbol{Name=Run, Receiver=Gate} → match via "gate".
//
//   - Symbol.Parent (lowercased) — class / struct / interface that
//     contains the symbol (Java / Kotlin / Python / TS / etc.).
//     "Foo.bar" + Symbol{Name=bar, Parent=Foo} → match via "foo".
//
//   - FileInfo.Package (lowercased) for sym.File — last dot-segment
//     compared too, so a Python module path "pkg.mod.sub" matches a
//     prefix segment "sub".
//     "gate.Run" + Symbol{Name=Run, File=internal/analysis/gate/gate.go,
//     Package=gate} → match via "gate".
//
//   - The file's directory basename (lowercased), as a final fallback
//     for languages that don't carry an explicit package field
//     (C / Rust / Swift). "gate.Run" + Symbol.File="src/gate/run.c"
//     → directory "gate" → match.
//
// When prefix is empty (single-segment entity), the function
// returns true unconditionally — there's nothing to disambiguate.
func symbolMatchesQualifier(sym *repomap.Symbol, prefix []string, graph *repomap.Graph) bool {
	if len(prefix) == 0 {
		return true
	}
	if sym == nil {
		return false
	}
	candidates := make(map[string]bool, 6)
	if r := strings.ToLower(strings.TrimSpace(sym.Receiver)); r != "" {
		candidates[r] = true
	}
	if p := strings.ToLower(strings.TrimSpace(sym.Parent)); p != "" {
		candidates[p] = true
	}
	if graph != nil && sym.File != "" {
		if fi, ok := graph.FileIndex[sym.File]; ok && fi != nil {
			pkg := strings.ToLower(strings.TrimSpace(fi.Package))
			if pkg != "" {
				candidates[pkg] = true
				if dot := strings.LastIndexByte(pkg, '.'); dot >= 0 && dot < len(pkg)-1 {
					candidates[pkg[dot+1:]] = true
				}
				if slash := strings.LastIndexByte(pkg, '/'); slash >= 0 && slash < len(pkg)-1 {
					candidates[pkg[slash+1:]] = true
				}
			}
		}
		// Directory-basename fallback (C / Rust / Swift / any lang
		// without a Package field on FileInfo).
		if dirBase := dirBaseName(sym.File); dirBase != "" {
			candidates[strings.ToLower(dirBase)] = true
		}
	}
	for _, seg := range prefix {
		if seg == "" {
			continue
		}
		if candidates[seg] {
			return true
		}
	}
	return false
}

// dirBaseName returns the last directory segment of a forward-slash
// path. "internal/analysis/gate/gate.go" → "gate". Returns "" when
// the path is a bare filename or empty.
func dirBaseName(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	// Normalise Windows separators if any leaked in.
	p = strings.ReplaceAll(p, "\\", "/")
	slash := strings.LastIndexByte(p, '/')
	if slash <= 0 {
		return ""
	}
	dir := p[:slash]
	if last := strings.LastIndexByte(dir, '/'); last >= 0 {
		return dir[last+1:]
	}
	return dir
}

// Compile-time assertion — exercises repomaptypes import to keep
// "imported and not used" failures during refactors loud.
var _ = repomaptypes.LangGo
