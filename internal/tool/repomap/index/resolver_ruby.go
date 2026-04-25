package index

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// rubyImportResolver maps Ruby load-path imports (`require "foo/bar"`)
// and caller-relative imports (`require_relative "../support/helper"`)
// onto repo-local .rb files. The extractor already normalises the
// imported string literal, so resolution is path-based rather than
// symbol-based.
type rubyImportResolver struct {
	loadPathIndex map[string][]string
}

func (r *rubyImportResolver) Language() string { return types.LangRuby }

func (r *rubyImportResolver) Prepare(g *types.Graph, _ *ResolverContext) error {
	r.loadPathIndex = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangRuby {
			continue
		}
		for _, key := range rubyRequireKeys(fi.RelPath) {
			r.loadPathIndex[key] = types.AppendUnique(r.loadPathIndex[key], fi.RelPath)
		}
	}
	return nil
}

func (r *rubyImportResolver) Resolve(_ *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := normalizeRubyImportPath(imp.Path)
	if path == "" {
		return nil
	}

	if rubyImportIsRelative(imp.Raw, path) {
		if resolved := probeRubyRelative(ctx, fi.RelPath, path); len(resolved) > 0 {
			return resolved
		}
	}

	if files := r.loadPathIndex[path]; len(files) > 0 {
		return files
	}
	if files := r.loadPathIndex[strings.TrimPrefix(path, "lib/")]; len(files) > 0 {
		return files
	}
	if resolved := probeRubyRepo(ctx, path); len(resolved) > 0 {
		return resolved
	}

	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: imp.Path, Reason: "ruby_external",
	})
	return nil
}

func rubyRequireKeys(relPath string) []string {
	relPath = filepath.ToSlash(relPath)
	base := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	if base == "" {
		return nil
	}
	keys := []string{base}
	if strings.HasPrefix(base, "lib/") {
		keys = append(keys, strings.TrimPrefix(base, "lib/"))
	}
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.Trim(strings.TrimSpace(filepath.ToSlash(key)), "/")
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func normalizeRubyImportPath(path string) string {
	path = strings.TrimSpace(filepath.ToSlash(path))
	path = strings.Trim(path, `"'`)
	path = strings.TrimSuffix(path, ".rb")
	return strings.Trim(path, "/")
}

func rubyImportIsRelative(raw, path string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return strings.Contains(raw, "require_relative") ||
		strings.HasPrefix(path, ".") ||
		strings.Contains(raw, "load")
}

func probeRubyRelative(ctx *ResolverContext, relFile, importPath string) []string {
	dir := filepath.ToSlash(filepath.Dir(relFile))
	base := filepath.ToSlash(filepath.Clean(filepath.Join(dir, importPath)))
	return probeRubyCandidates(ctx, base)
}

func probeRubyRepo(ctx *ResolverContext, importPath string) []string {
	for _, base := range []string{
		importPath,
		filepath.ToSlash(filepath.Join("lib", importPath)),
	} {
		if resolved := probeRubyCandidates(ctx, base); len(resolved) > 0 {
			return resolved
		}
	}
	return nil
}

func probeRubyCandidates(ctx *ResolverContext, base string) []string {
	base = strings.Trim(strings.TrimSpace(filepath.ToSlash(base)), "/")
	if base == "" {
		return nil
	}
	for _, candidate := range []string{base, base + ".rb"} {
		if _, ok := ctx.FileIndex[candidate]; ok {
			return []string{candidate}
		}
	}
	return nil
}
