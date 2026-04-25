package index

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// arkTSImportResolver handles HarmonyOS ArkTS imports. ArkTS reuses
// the TypeScript import syntax but introduces three classes of
// "builtin" paths that must never be resolved against the repo:
//
//   - `@ohos.*`      runtime APIs (ability, window, hilog, …)
//   - `@kit.*`       grouped runtime kits (camera, graphic, …)
//   - `@hms.*`       Huawei Mobile Services (pay, account, …)
//   - `@arkui.*`     UI framework internals
//   - `@system.*`    legacy lite device APIs
//
// These paths must be black-holed — returning nil tells the
// dispatcher that the import is "intentionally unresolved" rather
// than falling through to basename-glob fallback (which would mis-
// attribute a runtime import to a random repo file).
//
// For non-builtin paths the resolver delegates to the shared
// jsImportResolver: ArkTS uses the same relative path / alias
// semantics as TypeScript (oh-package.json5 overlays tsconfig.json
// alias behaviour), so we do not reinvent the path-matching wheel.
//
// oh-package.json5 parsing is deliberately NOT implemented here for
// Tier 1: the json5 format requires either a dependency or a hand
// written parser, both of which add risk for the narrow benefit of
// resolving bundle-qualified imports (`@bundle:<bundle>/...`). The
// arkTSImportResolver returns nil for `@bundle:` paths today so the
// dispatcher records them as UnresolvedImport; when a concrete user
// workflow needs bundle resolution we can add a json5 parser.
type arkTSImportResolver struct {
	// Wrap jsImportResolver for the common TS-style paths. Not a
	// pointer so shared state inside jsImportResolver stays shared
	// with the TypeScript resolver registration.
	tsDelegate *jsImportResolver

	// bundleMap maps a HarmonyOS bundle name → the repo-relative
	// directory that hosts the bundle's source. Populated in
	// Prepare by scanning every oh-package.json5 in the repo.
	//
	// Example: `@bundle:com.example.feature` resolves to the dir
	// whose oh-package.json5 declares `"name": "com.example.feature"`.
	// The presence of the dir lets `@bundle:com.example.feature/utils`
	// map to `<dir>/utils.ets` via the regular JS resolver.
	bundleMap map[string]string
}

func newArkTSImportResolver(shared *jsImportResolver) *arkTSImportResolver {
	return &arkTSImportResolver{tsDelegate: shared, bundleMap: map[string]string{}}
}

func (r *arkTSImportResolver) Language() string { return types.LangArkTS }

// Prepare scans every oh-package.json5 in the repo and builds the
// bundle-name → dir map. Also delegates to the shared jsImportResolver
// so tsconfig-paths alias behaviour stays intact for TS-shaped helpers
// inside an ArkTS project.
//
// A parse error on any oh-package.json5 is logged at Debug level but
// never fails Prepare — the resolver still works, it just can't
// resolve `@bundle:` for that bundle.
func (r *arkTSImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	if r.bundleMap == nil {
		r.bundleMap = map[string]string{}
	}
	for _, sp := range ctx.SpecialFiles {
		base := filepath.Base(sp)
		if base != "oh-package.json5" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(g.Root, sp))
		if err != nil {
			logging.Debug("resolver_arkts: read %s: %v", sp, err)
			continue
		}
		var parsed map[string]any
		if err := ParseJSON5(data, &parsed); err != nil {
			logging.Debug("resolver_arkts: json5 parse %s: %v", sp, err)
			continue
		}
		name, _ := parsed["name"].(string)
		if name == "" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(sp))
		// Register both the raw name and the canonical `@bundle:`
		// form so consumers can look up either spelling.
		r.bundleMap[name] = dir
		r.bundleMap["@bundle:"+name] = dir
	}

	// Delegate Prepare; jsImportResolver dedupes internally so
	// double-call from both TS + ArkTS registrations is safe.
	if r.tsDelegate != nil {
		return r.tsDelegate.Prepare(g, ctx)
	}
	return nil
}

// Resolve returns target RelPaths. See package-level comment on
// black-hole rules + bundle lookup.
func (r *arkTSImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := imp.Path
	if isArkTSBuiltin(path) {
		return nil // intentional black-hole
	}
	if strings.HasPrefix(path, "@bundle:") {
		// Bundle-qualified cross-module import. Split into bundle
		// name + (optional) inner subpath: `@bundle:com.x.y/sub/file`
		// → bundle=`@bundle:com.x.y`, sub=`sub/file`.
		bundleName := path
		sub := ""
		if slash := strings.Index(path[len("@bundle:"):], "/"); slash >= 0 {
			bundleName = path[:len("@bundle:")+slash]
			sub = path[len("@bundle:")+slash+1:]
		}
		dir, ok := r.bundleMap[bundleName]
		if !ok {
			return nil
		}
		// Build the candidate path inside the bundle dir and try
		// every ArkTS/TS extension + index convention through the
		// shared jsExts probe.
		candidate := dir
		if sub != "" {
			candidate = filepath.ToSlash(filepath.Join(dir, sub))
		}
		if resolved := resolveJsCandidate(ctx, candidate); len(resolved) > 0 {
			return resolved
		}
		return nil
	}
	if r.tsDelegate == nil {
		return nil
	}
	return r.tsDelegate.Resolve(g, fi, imp, ctx)
}

// isArkTSBuiltin reports whether `path` is a HarmonyOS runtime
// import that must not be resolved against the repo. The list of
// prefixes is intentionally explicit rather than using a generic
// `@<word>.` heuristic: TypeScript alias imports (`@/components/...`)
// use the same shape, and we must not black-hole those.
//
// Grep-able invariant: one producer for the list, consulted by the
// resolver AND the log_triage file resolver (see
// internal/analysis/logtriage/resolve_arkts.go).
func isArkTSBuiltin(path string) bool {
	switch {
	case strings.HasPrefix(path, "@ohos."):
		return true
	case strings.HasPrefix(path, "@kit."):
		return true
	case strings.HasPrefix(path, "@hms."):
		return true
	case strings.HasPrefix(path, "@arkui."):
		return true
	case strings.HasPrefix(path, "@system."):
		return true
	}
	return false
}
