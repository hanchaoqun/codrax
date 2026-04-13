package index

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// cppImportResolver handles C and C++ #include directives.
//
// The extractor (extract_c.go) strips the surrounding `"…"` / `<…>`
// before handing the path to us, so this resolver cannot tell a
// project-relative include from a system include at the syntax
// level. Instead it tries, in order:
//
//  1. Relative to the including file's directory — catches the
//     typical `#include "foo.h"` and `#include "sub/leaf.h"`.
//  2. As a repo-root-relative path — catches `#include
//     "include/proj/foo.h"` when callers already prefix their
//     includes with a project root.
//  3. Longest-suffix match against a per-file suffix index built at
//     Prepare time. Every source/header RelPath is inserted into
//     the index under every one of its tail-path suffixes, so
//     `#include "net/http.h"` matches `src/net/http.h` even when
//     another unrelated `http.h` exists elsewhere in the repo. This
//     preserves the pre-refactor basename match as the last
//     fallback tier without losing the path-aware disambiguation.
//  4. Anything that still misses (`stdio.h`, `<iostream>`, vendored
//     third-party headers not in the repo tree) is classified
//     `cpp_external` — the honest "not in this repo" bucket shared
//     with the other Phase 2a resolvers.
//
// One struct instance is shared between the types.LangC and types.LangCpp map
// entries; Prepare is idempotent against repeated calls so the
// shared-instance registration is safe.
type cppImportResolver struct {
	// suffixIndex: every tail path of every C/C++ file → the
	// RelPaths that share that suffix. Includes the full RelPath
	// itself as a suffix of length-1-segments for exact match.
	suffixIndex map[string][]string
}

func (r *cppImportResolver) Language() string { return types.LangC }

func (r *cppImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	if r.suffixIndex != nil {
		// Shared across types.LangC and types.LangCpp — only build once.
		return nil
	}
	r.suffixIndex = make(map[string][]string)

	for _, fi := range g.Files {
		if fi.Language != types.LangC && fi.Language != types.LangCpp {
			continue
		}
		rel := filepath.ToSlash(fi.RelPath)
		segs := strings.Split(rel, "/")
		for i := 0; i < len(segs); i++ {
			key := strings.Join(segs[i:], "/")
			if key == "" {
				continue
			}
			r.suffixIndex[key] = types.AppendUnique(r.suffixIndex[key], rel)
		}
	}
	return nil
}

func (r *cppImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := filepath.ToSlash(imp.Path)
	if path == "" {
		return nil
	}

	// 1. Relative to the including file's directory.
	fileDir := filepath.ToSlash(filepath.Dir(fi.RelPath))
	candidate := filepath.ToSlash(filepath.Clean(filepath.Join(fileDir, path)))
	if strings.HasPrefix(candidate, "./") {
		candidate = candidate[2:]
	}
	if _, ok := ctx.FileIndex[candidate]; ok {
		return []string{candidate}
	}

	// 2. As a repo-root-relative path.
	if _, ok := ctx.FileIndex[path]; ok {
		return []string{path}
	}

	// 3. Longest-suffix match against the precomputed index.
	if files, ok := r.suffixIndex[path]; ok && len(files) > 0 {
		return files
	}

	// 4. External.
	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: imp.Path, Reason: "cpp_external",
	})
	return nil
}
