package index

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// rustImportResolver resolves Rust `use` declarations and bare `mod`
// file references against a workspace of Cargo crates discovered via
// Cargo.toml files in ctx.SpecialFiles.
//
// For each Cargo.toml it reads the `[package].name` key (and optional
// `[lib].path` override) and locates the crate's root file at
// src/lib.rs or src/main.rs relative to the Cargo.toml directory.
// Files under a crate's source tree are mapped to that crate by
// longest-prefix directory match.
//
// Resolution of `use a::b::c::Type`:
//   - `self`, `super`, `crate` prefixes anchor the walk to the
//     current file's directory, parent directory, or crate root.
//   - Any other leading segment is looked up in the workspace crate
//     table; when matched the walk starts at that crate's root,
//     otherwise the import is classified `rust_external`.
//   - Intermediate segments probe `<dir>/<seg>.rs` then
//     `<dir>/<seg>/mod.rs`, advancing `currentDir` to `<dir>/<seg>`
//     on success so Rust 2018+ inline submodule layouts work.
//   - Walk stops at the first segment that is not a module file
//     (types, values, glob `*`, group `{...}`), returning the last
//     resolved file.
//
// Known Phase 2a scope gaps, carried into later phases:
//   - No inline `mod X { ... }` block handling beyond file level.
//   - No `pub use` re-export chasing.
//   - Glob imports and `{a, b}` groups resolve to the parent module
//     file only, not to individual items.
type rustImportResolver struct {
	crates    map[string]*rustCrate
	fileCrate map[string]string // file RelPath → crate name
}

type rustCrate struct {
	name     string
	rootDir  string // e.g. "crate_a/src" (forward-slash, repo-relative)
	rootFile string // e.g. "crate_a/src/lib.rs"
}

type cargoTomlInfo struct {
	packageName string
	libPath     string
}

func (r *rustImportResolver) Language() string { return types.LangRust }

func (r *rustImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	r.crates = make(map[string]*rustCrate)
	r.fileCrate = make(map[string]string)

	for _, sp := range ctx.SpecialFiles {
		if filepath.Base(sp) != "Cargo.toml" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(g.Root, sp))
		if err != nil {
			continue
		}
		info := parseCargoToml(data)
		if info.packageName == "" {
			continue // workspace-only Cargo.toml, no [package] section
		}

		cargoDir := filepath.ToSlash(filepath.Dir(sp))
		if cargoDir == "." {
			cargoDir = ""
		}

		// Root-file discovery: explicit [lib].path first, then src/
		// lib.rs, then src/main.rs.
		rootFile := ""
		tryRoot := func(p string) bool {
			norm := filepath.ToSlash(filepath.Clean(p))
			if strings.HasPrefix(norm, "./") {
				norm = norm[2:]
			}
			if _, ok := ctx.FileIndex[norm]; ok {
				rootFile = norm
				return true
			}
			return false
		}
		if info.libPath != "" {
			tryRoot(filepath.Join(cargoDir, info.libPath))
		}
		if rootFile == "" {
			tryRoot(filepath.Join(cargoDir, "src", "lib.rs"))
		}
		if rootFile == "" {
			tryRoot(filepath.Join(cargoDir, "src", "main.rs"))
		}
		if rootFile == "" {
			continue
		}

		r.crates[info.packageName] = &rustCrate{
			name:     info.packageName,
			rootDir:  filepath.ToSlash(filepath.Dir(rootFile)),
			rootFile: rootFile,
		}
	}

	// Assign each Rust file to the crate whose rootDir is the longest
	// prefix of the file's directory.
	for _, fi := range g.Files {
		if fi.Language != types.LangRust {
			continue
		}
		fileDir := filepath.ToSlash(filepath.Dir(fi.RelPath))
		bestCrate := ""
		bestLen := -1
		for name, c := range r.crates {
			if c.rootDir == fileDir || strings.HasPrefix(fileDir, c.rootDir+"/") {
				if len(c.rootDir) > bestLen {
					bestLen = len(c.rootDir)
					bestCrate = name
				}
			}
		}
		if bestCrate != "" {
			r.fileCrate[fi.RelPath] = bestCrate
		}
	}
	return nil
}

func (r *rustImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	rawPath := imp.Path
	path := cleanRustImportPath(rawPath)
	if path == "" {
		ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
			File: fi.RelPath, Raw: rawPath, Reason: "rust_external",
		})
		return nil
	}

	// Extractor emits bare `mod X;` as an types.Import with single-
	// identifier path. Resolve against the source file's own
	// directory using the Rust mod-file convention.
	if !strings.Contains(path, "::") {
		fileDir := filepath.ToSlash(filepath.Dir(fi.RelPath))
		if files := probeRustModule(ctx, fileDir, path); len(files) > 0 {
			return files
		}
		if c, ok := r.crates[path]; ok {
			return []string{c.rootFile}
		}
		ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
			File: fi.RelPath, Raw: rawPath, Reason: "rust_external",
		})
		return nil
	}

	segs := strings.Split(path, "::")

	// Anchor to a base directory depending on the first segment.
	var baseDir string
	var anchorRoot string // initial lastResolved value (crate root file)
	switch segs[0] {
	case "self":
		baseDir = currentRustModuleDir(fi.RelPath)
		segs = segs[1:]
	case "super":
		baseDir = filepath.ToSlash(filepath.Dir(currentRustModuleDir(fi.RelPath)))
		if baseDir == "." {
			baseDir = ""
		}
		segs = segs[1:]
	case "crate":
		crateName := r.fileCrate[fi.RelPath]
		c, ok := r.crates[crateName]
		if !ok {
			ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
				File: fi.RelPath, Raw: rawPath, Reason: "rust_internal_miss",
			})
			return nil
		}
		baseDir = c.rootDir
		anchorRoot = c.rootFile
		segs = segs[1:]
	default:
		// Another crate in the workspace?
		if c, ok := r.crates[segs[0]]; ok {
			baseDir = c.rootDir
			anchorRoot = c.rootFile
			segs = segs[1:]
		} else {
			ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
				File: fi.RelPath, Raw: rawPath, Reason: "rust_external",
			})
			return nil
		}
	}

	lastResolved := anchorRoot
	currentDir := baseDir
	for _, seg := range segs {
		if seg == "" || seg == "*" || strings.HasPrefix(seg, "{") {
			break
		}
		if resolved := probeRustModule(ctx, currentDir, seg); len(resolved) > 0 {
			lastResolved = resolved[0]
			currentDir = filepath.ToSlash(filepath.Join(currentDir, seg))
			continue
		}
		// seg is a type/value/unresolvable — stop walking.
		break
	}

	if lastResolved != "" {
		return []string{lastResolved}
	}
	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: rawPath, Reason: "rust_internal_miss",
	})
	return nil
}

// currentRustModuleDir returns the directory Rust uses to look up a
// file's child modules. For a file `<dir>/<name>.rs` where name is
// not one of the special bases (mod/lib/main), the module's children
// live under `<dir>/<name>/`. For `<dir>/mod.rs` and for crate roots
// (`lib.rs`, `main.rs`), children live directly under `<dir>/`.
func currentRustModuleDir(filePath string) string {
	dir := filepath.ToSlash(filepath.Dir(filePath))
	if dir == "." {
		dir = ""
	}
	base := strings.TrimSuffix(filepath.Base(filePath), ".rs")
	if base == "mod" || base == "lib" || base == "main" {
		return dir
	}
	if dir == "" {
		return base
	}
	return dir + "/" + base
}

// probeRustModule tries the two canonical layouts for `mod <name>`
// declared in a file living under <dir>: `<dir>/<name>.rs` then
// `<dir>/<name>/mod.rs`.
func probeRustModule(ctx *ResolverContext, dir, name string) []string {
	joinPath := func(parts ...string) string {
		out := filepath.ToSlash(filepath.Join(parts...))
		if strings.HasPrefix(out, "./") {
			out = out[2:]
		}
		return out
	}
	candidate := joinPath(dir, name+".rs")
	if _, ok := ctx.FileIndex[candidate]; ok {
		return []string{candidate}
	}
	candidate = joinPath(dir, name, "mod.rs")
	if _, ok := ctx.FileIndex[candidate]; ok {
		return []string{candidate}
	}
	return nil
}

// cleanRustImportPath normalizes an types.Import.Path as produced by
// rustExtractUse: strips the ` as alias` tail and the trailing
// `::{group}` or `::*` glob, returning only the module-prefix
// portion the resolver should walk.
func cleanRustImportPath(path string) string {
	path = strings.TrimSpace(path)
	// Strip a leading `pub ` if the extractor ever emits one.
	path = strings.TrimPrefix(path, "pub ")
	// Strip `use ` / trailing `;` if the raw text leaked through.
	path = strings.TrimPrefix(path, "use ")
	path = strings.TrimSuffix(path, ";")
	path = strings.TrimSpace(path)

	if i := strings.Index(path, " as "); i != -1 {
		path = path[:i]
	}
	if i := strings.Index(path, "::{"); i != -1 {
		path = path[:i]
	}
	path = strings.TrimSuffix(path, "::*")
	return strings.TrimSpace(path)
}

// parseCargoToml extracts the minimum Cargo.toml fields we need: the
// [package].name and [lib].path entries. A hand-written line parser
// keeps the repomap dependency footprint unchanged (stdlib + yaml.v3
// only).
func parseCargoToml(data []byte) cargoTomlInfo {
	var info cargoTomlInfo
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip trailing inline comment.
		if hash := strings.Index(val, "#"); hash != -1 {
			val = strings.TrimSpace(val[:hash])
		}
		switch section {
		case "package":
			if key == "name" {
				info.packageName = trimTomlString(val)
			}
		case "lib":
			if key == "path" {
				info.libPath = trimTomlString(val)
			}
		}
	}
	return info
}

func trimTomlString(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 {
		first := v[0]
		last := v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}
