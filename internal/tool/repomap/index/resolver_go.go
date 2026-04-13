package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// goImportResolver resolves Go imports by parsing the repo's go.mod
// module declaration and mapping fully-qualified import paths to the
// files in their declared directory.
//
// Internal imports (those whose path starts with the module path) are
// looked up in an exact `<modulePath>/<reldir> → []file` map built at
// Prepare time. Module v2+ (`/v2` suffix on the module declaration
// itself) is handled because the suffix is part of modulePath and the
// directory layout doesn't include the version segment.
//
// External imports (stdlib, third-party modules) cannot resolve to any
// file inside a whole-repo-only graph. They are recorded as
// types.UnresolvedImport with Reason "go_external" so the eval harness can
// distinguish them from genuine internal misses.
type goImportResolver struct {
	modulePath  string
	pathToFiles map[string][]string
}

var goModuleRe = regexp.MustCompile(`^\s*module\s+(\S+)`)

func (r *goImportResolver) Language() string { return types.LangGo }

func (r *goImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	// Find the shallowest go.mod (the repo-root module). A deeper
	// go.mod indicates a nested module we do not own; Phase 2a
	// targets the single-root case and documents go.work as a
	// Phase 3 follow-up.
	var goModRel string
	for _, sp := range ctx.SpecialFiles {
		if filepath.Base(sp) != "go.mod" {
			continue
		}
		if goModRel == "" ||
			strings.Count(sp, string(filepath.Separator)) <
				strings.Count(goModRel, string(filepath.Separator)) {
			goModRel = sp
		}
	}
	if goModRel == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(g.Root, goModRel))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if m := goModuleRe.FindStringSubmatch(line); m != nil {
			r.modulePath = strings.TrimSpace(m[1])
			break
		}
	}
	if r.modulePath == "" {
		return nil
	}
	r.pathToFiles = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangGo {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(fi.RelPath))
		var importPath string
		if dir == "." || dir == "" {
			importPath = r.modulePath
		} else {
			importPath = r.modulePath + "/" + dir
		}
		r.pathToFiles[importPath] = append(r.pathToFiles[importPath], fi.RelPath)
	}
	return nil
}

func (r *goImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := imp.Path

	// Relative import — rare in modules but legal in pre-module
	// trees and test harness fixtures. Delegate to the legacy
	// relative branch which knows how to walk the filesystem.
	if strings.HasPrefix(path, ".") {
		return resolveImport(g, fi, imp, ctx.PkgToFiles, ctx.BasenameIndex)
	}

	// No go.mod discovered: fall back to the legacy heuristic so
	// repositories without a proper module declaration still get
	// approximate edges.
	if r.modulePath == "" || r.pathToFiles == nil {
		return resolveImport(g, fi, imp, ctx.PkgToFiles, ctx.BasenameIndex)
	}

	// Internal import: exact prefix match on the module path.
	if path == r.modulePath {
		return r.pathToFiles[path]
	}
	if strings.HasPrefix(path, r.modulePath+"/") {
		if files, ok := r.pathToFiles[path]; ok {
			return files
		}
		// v2+ sub-module imports may reference a path with an embedded
		// `/vN/` segment that the file layout doesn't actually use.
		if stripped := stripGoVersionSegment(path); stripped != path {
			if files, ok := r.pathToFiles[stripped]; ok {
				return files
			}
		}
		ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
			File: fi.RelPath, Raw: path, Reason: "go_internal_miss",
		})
		return nil
	}

	// External (stdlib or third-party module). Record with a
	// distinct reason so the harness can exclude it from the
	// internal accuracy denominator.
	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: path, Reason: "go_external",
	})
	return nil
}

// stripGoVersionSegment removes an embedded `/vN/` major version
// segment from a Go module import path, e.g.
// "example.com/foo/v2/bar" → "example.com/foo/bar". Only strips
// segments whose full text is `v` followed by an integer ≥ 2, so v1
// (no major-version suffix in Go module semantics), `v` alone, and
// words starting with v are preserved.
func stripGoVersionSegment(path string) string {
	parts := strings.Split(path, "/")
	out := parts[:0]
	for _, p := range parts {
		if len(p) >= 2 && p[0] == 'v' {
			if n, err := strconv.Atoi(p[1:]); err == nil && n >= 2 {
				continue
			}
		}
		out = append(out, p)
	}
	return strings.Join(out, "/")
}
