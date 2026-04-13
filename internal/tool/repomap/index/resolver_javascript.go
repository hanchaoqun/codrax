package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// jsImportResolver handles both JavaScript and TypeScript. It is
// registered twice in defaultResolvers() — once per language tag —
// with a shared struct value so Prepare only runs once worth of work
// per BuildGraph.
//
// Resolution order for a non-relative import path:
//  1. tsconfig/jsconfig `paths` alias rewrite (longest-prefix match).
//  2. Repo-local alias-rewritten candidate (via resolveJsCandidate).
//  3. Bare module (`react`, `lodash`, `@scope/pkg`) → js_external.
//
// Relative imports (`./foo`, `../bar`) are delegated to the legacy
// branch, which already walks the filesystem with the canonical JS
// extension + `/index.*` fallback matrix.
//
// package.json `exports` handling is intentionally out of scope for
// Phase 2a: real codebases overwhelmingly rely on tsconfig `paths`
// for internal aliasing; `exports` matters mainly for published
// packages and is a Phase 3 follow-up when cache versioning lands.
type jsImportResolver struct {
	aliases []jsAlias // sorted by prefix length descending
}

// jsAlias is a single rewrite rule compiled from one tsconfig `paths`
// entry. `prefix` is the alias key with a trailing `/*` stripped. If
// `isGlob` is true, the prefix matches when the import path equals
// `prefix` or starts with `prefix + "/"`; otherwise only exact match.
// `targets` are repo-root-relative directories (glob) or files
// (non-glob), already joined with tsconfig baseUrl and normalized.
type jsAlias struct {
	prefix  string
	isGlob  bool
	targets []string
}

// jsCommentRe strips // line comments and /* block */ comments from
// a tsconfig/jsconfig source before JSON unmarshal. JSONC support is
// not in encoding/json but tsconfig commonly has inline comments.
var jsCommentRe = regexp.MustCompile(`(?ms)//[^\n]*|/\*.*?\*/`)

func (r *jsImportResolver) Language() string { return types.LangJavaScript }

func (r *jsImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	if r.aliases != nil {
		// Shared across JS and TS registrations — only prepare once.
		return nil
	}
	r.aliases = []jsAlias{}

	for _, sp := range ctx.SpecialFiles {
		base := filepath.Base(sp)
		if base != "tsconfig.json" && base != "jsconfig.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(g.Root, sp))
		if err != nil {
			continue
		}
		stripped := jsCommentRe.ReplaceAllString(string(data), "")
		var cfg struct {
			CompilerOptions struct {
				BaseURL string              `json:"baseUrl"`
				Paths   map[string][]string `json:"paths"`
			} `json:"compilerOptions"`
		}
		if err := json.Unmarshal([]byte(stripped), &cfg); err != nil {
			continue
		}
		tsconfigDir := filepath.Dir(sp)
		baseURL := cfg.CompilerOptions.BaseURL
		if baseURL == "" {
			baseURL = "."
		}
		// baseURL is relative to the tsconfig directory. Normalize to
		// a repo-root-relative path with forward slashes.
		baseRoot := filepath.ToSlash(filepath.Clean(filepath.Join(tsconfigDir, baseURL)))
		if baseRoot == "." {
			baseRoot = ""
		}

		for key, targets := range cfg.CompilerOptions.Paths {
			alias := jsAlias{}
			if strings.HasSuffix(key, "/*") {
				alias.prefix = strings.TrimSuffix(key, "/*")
				alias.isGlob = true
			} else {
				alias.prefix = key
				alias.isGlob = false
			}
			for _, t := range targets {
				nt := t
				if alias.isGlob {
					nt = strings.TrimSuffix(nt, "/*")
				}
				joined := filepath.ToSlash(filepath.Clean(filepath.Join(baseRoot, nt)))
				alias.targets = append(alias.targets, joined)
			}
			r.aliases = append(r.aliases, alias)
		}
	}

	// Longest prefix first so nested aliases (`@/components/*` over
	// `@/*`) match before the shorter parent.
	sort.SliceStable(r.aliases, func(i, j int) bool {
		return len(r.aliases[i].prefix) > len(r.aliases[j].prefix)
	})
	return nil
}

func (r *jsImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := imp.Path
	if path == "" {
		return nil
	}

	// Relative imports: delegate to legacy. It already handles the
	// extension + /index.* matrix for the JS family.
	if strings.HasPrefix(path, ".") {
		return resolveImport(g, fi, imp, ctx.PkgToFiles, ctx.BasenameIndex)
	}

	// Alias rewrite: try each compiled alias in longest-prefix order.
	for _, a := range r.aliases {
		if a.isGlob {
			if !(path == a.prefix || strings.HasPrefix(path, a.prefix+"/")) {
				continue
			}
			suffix := ""
			if len(path) > len(a.prefix) {
				suffix = strings.TrimPrefix(path, a.prefix+"/")
			}
			for _, t := range a.targets {
				candidate := filepath.ToSlash(filepath.Clean(filepath.Join(t, suffix)))
				if resolved := resolveJsCandidate(ctx, candidate); len(resolved) > 0 {
					return resolved
				}
			}
		} else {
			if path != a.prefix {
				continue
			}
			for _, t := range a.targets {
				if resolved := resolveJsCandidate(ctx, t); len(resolved) > 0 {
					return resolved
				}
			}
		}
	}

	// Bare module — stdlib, third-party, or unaliased internal. All
	// honestly "not in this repo" from the harness's perspective.
	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: path, Reason: "js_external",
	})
	return nil
}

// jsExts is the canonical extension probe order, matching what the
// legacy relative branch already walks.
var jsExts = []string{".ts", ".tsx", ".d.ts", ".js", ".jsx", ".mjs", ".cjs"}

// jsIndexFiles is the bare-directory fallback list, mirroring Node
// and bundler conventions.
var jsIndexFiles = []string{
	"index.ts", "index.tsx", "index.js", "index.jsx",
	"index.mjs", "index.cjs", "index.d.ts",
}

// resolveJsCandidate tries every extension + index variant for a
// rewritten repo-root-relative base path. Returns all matches in
// priority order; callers take the first non-empty slice.
func resolveJsCandidate(ctx *ResolverContext, base string) []string {
	base = strings.TrimSuffix(base, "/")
	// Exact-path file (already has an extension, or is a directory
	// the user spelled with /index stripped).
	if _, ok := ctx.FileIndex[base]; ok {
		return []string{base}
	}
	for _, ext := range jsExts {
		candidate := base + ext
		if _, ok := ctx.FileIndex[candidate]; ok {
			return []string{candidate}
		}
	}
	for _, idx := range jsIndexFiles {
		candidate := base + "/" + idx
		if _, ok := ctx.FileIndex[candidate]; ok {
			return []string{candidate}
		}
	}
	return nil
}

