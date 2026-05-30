package index

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// kotlinImportResolver resolves Kotlin imports by package declaration
// match. Mirrors the shape of javaImportResolver because both
// languages share the JVM `package a.b.c` namespace convention —
// the difference is which Language tag (Java vs Kotlin) and which
// file extension, not the resolution algorithm.
//
// Kotlin-specific considerations:
//
//   - An `import` may target a top-level function / val / object,
//     not just a class; so the type filter is looser than Java's.
//   - Wildcard imports (`import a.b.*`) resolve to every file in
//     the package.
//   - `import a.b.c as d` aliases are handled at the call-site, not
//     here — imp.Path is the pre-alias path.
//
// Mixed Java + Kotlin projects (typical on Android) are supported
// because both resolvers populate separate packageIndex maps; each
// sees only its own language's files.
type kotlinImportResolver struct {
	packageIndex map[string][]string // fi.Package → []RelPath
	// declInFile[package+"."+name] → []RelPath (files that declare a
	// top-level class / interface / object / function / val with that
	// name in that package). Built lazily.
	declInFile map[string][]string
}

func (r *kotlinImportResolver) Language() string { return types.LangKotlin }

func (r *kotlinImportResolver) Prepare(g *types.Graph, _ *ResolverContext) error {
	r.packageIndex = make(map[string][]string)
	r.declInFile = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangKotlin {
			continue
		}
		if fi.Package == "" {
			continue
		}
		r.packageIndex[fi.Package] = append(r.packageIndex[fi.Package], fi.RelPath)
		for i := range fi.Symbols {
			s := &fi.Symbols[i]
			if !isKotlinTopLevelDecl(s.Kind) {
				continue
			}
			key := fi.Package + "." + s.Name
			r.declInFile[key] = append(r.declInFile[key], fi.RelPath)
		}
	}
	return nil
}

// isKotlinTopLevelDecl reports whether `kind` is one of the Symbol
// kinds that can be imported across files. Kotlin allows importing
// classes, objects, top-level functions, val/var, and typealiases.
func isKotlinTopLevelDecl(kind string) bool {
	switch kind {
	case "class", "data-class", "sealed-class", "interface", "enum",
		"annotation", "object",
		"function", "suspend-function", "operator", "extension-function",
		"val", "var", "const",
		"type":
		return true
	}
	return false
}

func (r *kotlinImportResolver) Resolve(_ *types.Graph, _ *types.FileInfo, imp types.Import, _ *ResolverContext) []string {
	path := imp.Path
	if path == "" {
		return nil
	}
	// Wildcard: `import a.b.*` resolves to every file in package a.b.
	// After trimming `.*` the remainder IS the package name — look it
	// up directly (mirrors javaImportResolver). Do NOT split off a
	// trailing segment first: that would collapse `a.b.*` to package
	// `a`, the original defect this branch had.
	if strings.HasSuffix(path, ".*") {
		return r.packageIndex[strings.TrimSuffix(path, ".*")]
	}

	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		// Top-level symbol with no package — unusual; nothing to
		// resolve against our indexed packages.
		return nil
	}
	pkgPart := path[:idx]
	sym := path[idx+1:]

	if files := r.declInFile[pkgPart+"."+sym]; len(files) > 0 {
		return files
	}
	// Fall back to "any file in that package"; narrower than the
	// precise decl hit but better than declaring unresolved when a
	// Kotlin project uses member imports without our extractor
	// having surfaced the target (e.g. nested types, companion
	// object members).
	return r.packageIndex[pkgPart]
}
