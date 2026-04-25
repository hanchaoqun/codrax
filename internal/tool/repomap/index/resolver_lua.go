package index

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// luaImportResolver resolves both module-style requires
// (`require "foo.bar"`) and file-style loads (`dofile "foo/bar.lua"`).
// The resolver keeps a small alias table derived from repo paths so
// different Lua layouts (`lua/`, `lib/`, plain nested files) all map
// onto the same file set.
type luaImportResolver struct {
	moduleIndex map[string][]string
}

func (r *luaImportResolver) Language() string { return types.LangLua }

func (r *luaImportResolver) Prepare(g *types.Graph, _ *ResolverContext) error {
	r.moduleIndex = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangLua {
			continue
		}
		for _, key := range luaModuleKeys(fi.RelPath) {
			r.moduleIndex[key] = types.AppendUnique(r.moduleIndex[key], fi.RelPath)
		}
	}
	return nil
}

func (r *luaImportResolver) Resolve(_ *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := strings.TrimSpace(filepath.ToSlash(imp.Path))
	path = strings.Trim(path, `"'`)
	if path == "" {
		return nil
	}

	if luaImportIsRelative(imp.Raw, path) {
		if resolved := probeLuaRelative(ctx, fi.RelPath, path); len(resolved) > 0 {
			return resolved
		}
	}

	for _, key := range luaLookupKeys(path) {
		if files := r.moduleIndex[key]; len(files) > 0 {
			return files
		}
	}
	if resolved := probeLuaRepo(ctx, path); len(resolved) > 0 {
		return resolved
	}

	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: imp.Path, Reason: "lua_external",
	})
	return nil
}

func luaModuleKeys(relPath string) []string {
	relPath = filepath.ToSlash(relPath)
	base := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	if base == "" {
		return nil
	}
	var keys []string
	keys = append(keys, base)
	if strings.HasPrefix(base, "lua/") {
		keys = append(keys, strings.TrimPrefix(base, "lua/"))
	}
	if strings.HasPrefix(base, "lib/") {
		keys = append(keys, strings.TrimPrefix(base, "lib/"))
	}
	if strings.HasSuffix(base, "/init") {
		keys = append(keys, strings.TrimSuffix(base, "/init"))
	}
	out := make([]string, 0, len(keys)*2)
	seen := make(map[string]bool, len(keys)*2)
	for _, key := range keys {
		key = strings.Trim(strings.TrimSpace(filepath.ToSlash(key)), "/")
		if key == "" {
			continue
		}
		for _, variant := range []string{key, strings.ReplaceAll(key, "/", ".")} {
			if seen[variant] {
				continue
			}
			seen[variant] = true
			out = append(out, variant)
		}
	}
	return out
}

func luaLookupKeys(path string) []string {
	path = strings.TrimSpace(strings.TrimSuffix(path, ".lua"))
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	keys := []string{path}
	if strings.Contains(path, "/") {
		keys = append(keys, strings.ReplaceAll(path, "/", "."))
	}
	if strings.Contains(path, ".") {
		keys = append(keys, strings.ReplaceAll(path, ".", "/"))
	}
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.Trim(key, "/")
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func luaImportIsRelative(raw, path string) bool {
	raw = strings.ToLower(strings.TrimSpace(raw))
	return strings.Contains(raw, "dofile") ||
		strings.Contains(raw, "load") ||
		strings.HasPrefix(path, ".") ||
		strings.HasSuffix(strings.ToLower(path), ".lua")
}

func probeLuaRelative(ctx *ResolverContext, relFile, importPath string) []string {
	dir := filepath.ToSlash(filepath.Dir(relFile))
	base := filepath.ToSlash(filepath.Clean(filepath.Join(dir, importPath)))
	return probeLuaCandidates(ctx, base)
}

func probeLuaRepo(ctx *ResolverContext, importPath string) []string {
	for _, base := range []string{
		importPath,
		filepath.ToSlash(filepath.Join("lua", importPath)),
		filepath.ToSlash(filepath.Join("lib", importPath)),
	} {
		if resolved := probeLuaCandidates(ctx, base); len(resolved) > 0 {
			return resolved
		}
	}
	return nil
}

func probeLuaCandidates(ctx *ResolverContext, base string) []string {
	base = strings.Trim(strings.TrimSpace(filepath.ToSlash(base)), "/")
	if base == "" {
		return nil
	}
	for _, candidate := range []string{
		base,
		base + ".lua",
		base + "/init.lua",
	} {
		if _, ok := ctx.FileIndex[candidate]; ok {
			return []string{candidate}
		}
	}
	return nil
}
