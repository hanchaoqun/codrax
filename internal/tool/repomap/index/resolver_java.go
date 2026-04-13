package index

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// javaImportResolver resolves Java imports by exact package-declaration
// match against every types.FileInfo's declared `fi.Package`. An import of
// `foo.bar.Baz` splits into a package part (`foo.bar`) and a symbol
// part (`Baz`); we look the package up in packageIndex and — when
// possible — filter the candidate files to those whose Symbols slice
// contains a class / interface / enum named `Baz`.
//
// Imports that don't match any indexed package are classified as
// `java_external`, collapsing stdlib / third-party / unknown into one
// honest "not in this repo" bucket. A later phase that parses pom.xml
// or build.gradle can refine this by detecting internal vs external
// modules from the build config.
//
// Wildcard imports (`foo.bar.*`) resolve to every file in the package
// with no symbol filter.
type javaImportResolver struct {
	packageIndex map[string][]string // fi.Package → []RelPath
	// typeInFile[package+"."+className] → []RelPath (files that
	// declare a top-level class/interface/enum with that name in
	// that package). Built lazily to avoid double-iterating symbols.
	typeInFile map[string][]string
}

func (r *javaImportResolver) Language() string { return types.LangJava }

func (r *javaImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	r.packageIndex = make(map[string][]string)
	r.typeInFile = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangJava {
			continue
		}
		if fi.Package == "" {
			continue
		}
		r.packageIndex[fi.Package] = append(r.packageIndex[fi.Package], fi.RelPath)
		for i := range fi.Symbols {
			s := &fi.Symbols[i]
			if s.Kind == "class" || s.Kind == "interface" || s.Kind == "enum" {
				key := fi.Package + "." + s.Name
				r.typeInFile[key] = append(r.typeInFile[key], fi.RelPath)
			}
		}
	}
	return nil
}

func (r *javaImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := imp.Path
	if path == "" {
		return nil
	}

	// Wildcard import: foo.bar.* → every file in the package.
	if strings.HasSuffix(path, ".*") {
		pkg := strings.TrimSuffix(path, ".*")
		if files, ok := r.packageIndex[pkg]; ok {
			return files
		}
		ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
			File: fi.RelPath, Raw: path, Reason: "java_external",
		})
		return nil
	}

	// Typed import: split on the last dot into (package, class).
	dot := strings.LastIndex(path, ".")
	if dot <= 0 {
		// Single-segment import without a package (invalid Java in
		// practice, but record as external so the harness count is
		// stable).
		ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
			File: fi.RelPath, Raw: path, Reason: "java_external",
		})
		return nil
	}
	pkg := path[:dot]
	class := path[dot+1:]

	// Prefer the type-in-file filter when the package is known.
	if files, ok := r.typeInFile[pkg+"."+class]; ok {
		return files
	}

	// Package exists in the repo but we could not find a matching
	// type. Return every file in the package as a best-effort
	// (static imports, nested types, and non-public helpers all
	// live in the same file set).
	if files, ok := r.packageIndex[pkg]; ok {
		return files
	}

	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: path, Reason: "java_external",
	})
	return nil
}
