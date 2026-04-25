package index

import "github.com/hanchaoqun/codrax/internal/tool/repomap/types"

// swiftImportResolver maps Swift module imports to every file that
// belongs to the imported module/package. Swift import statements are
// module-scoped rather than file-scoped, so the best repo-local answer
// is the module's file set.
type swiftImportResolver struct {
	moduleIndex map[string][]string
}

func (r *swiftImportResolver) Language() string { return types.LangSwift }

func (r *swiftImportResolver) Prepare(g *types.Graph, _ *ResolverContext) error {
	r.moduleIndex = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangSwift || fi.Package == "" {
			continue
		}
		r.moduleIndex[fi.Package] = append(r.moduleIndex[fi.Package], fi.RelPath)
	}
	return nil
}

func (r *swiftImportResolver) Resolve(_ *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	if imp.Path == "" {
		return nil
	}
	if files := r.moduleIndex[imp.Path]; len(files) > 0 {
		return files
	}
	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: imp.Path, Reason: "swift_external",
	})
	return nil
}
