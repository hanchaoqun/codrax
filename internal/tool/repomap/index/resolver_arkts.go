package index

import (
	"strings"

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
}

func newArkTSImportResolver(shared *jsImportResolver) *arkTSImportResolver {
	return &arkTSImportResolver{tsDelegate: shared}
}

func (r *arkTSImportResolver) Language() string { return types.LangArkTS }

func (r *arkTSImportResolver) Prepare(g *types.Graph, ctx *ResolverContext) error {
	// Delegate Prepare; jsImportResolver dedupes internally so
	// double-call from both TS + ArkTS registrations is safe.
	if r.tsDelegate != nil {
		return r.tsDelegate.Prepare(g, ctx)
	}
	return nil
}

// Resolve returns target RelPaths. See package-level comment on
// black-hole rules.
func (r *arkTSImportResolver) Resolve(g *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := imp.Path
	if isArkTSBuiltin(path) {
		return nil // intentional black-hole
	}
	if strings.HasPrefix(path, "@bundle:") {
		// Bundle-qualified cross-module import — would require
		// oh-package.json5 parsing to map the bundle to a local
		// path. Today we black-hole and rely on basename glob in
		// the fallback chain.
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
