package tool

import (
	"strings"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// SourceInventoryTrackedSourceFile is one repo-owned source file from the
// source-class universe used by source-inventory lenses. It is derived from
// repo paths, source roles, lens scopes, and the language matrix; it carries no
// user prose or model rationale.
type SourceInventoryTrackedSourceFile struct {
	Path     string
	Role     types.SourcePathRole
	Language string
}

// SourceInventoryTrackedSourceFilesForLens returns the scoped source files that
// define the source-inventory class universe. In a git repo this uses
// git-tracked files; outside git it falls back to the existing bounded
// filesystem walk with a completeness flag. Callers that need candidate rows
// should consume this view instead of walking the workspace again.
func SourceInventoryTrackedSourceFilesForLens(repoRoot string, query types.SourceInventoryLensQuery) ([]SourceInventoryTrackedSourceFile, bool) {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return nil, false
	}
	scopes := sourceInventorySourceClassUniverseScopes(query)
	files, complete := sourceInventoryTrackedRepoFiles(repoRoot)
	if len(files) == 0 {
		return nil, complete
	}
	out := make([]SourceInventoryTrackedSourceFile, 0, len(files))
	for _, rel := range files {
		rel = strings.Trim(strings.ReplaceAll(rel, `\`, `/`), "/")
		if rel == "" || !sourceInventoryFileInScopes(rel, scopes) {
			continue
		}
		lang := sourceInventoryTrackedSourceLanguage(repoRoot, rel)
		if lang == "" || !repotypes.IsSupportedReadLanguage(lang) {
			continue
		}
		role := types.ClassifySourcePathRole(rel)
		if role == types.SourcePathRoleUnknown {
			continue
		}
		out = append(out, SourceInventoryTrackedSourceFile{
			Path:     rel,
			Role:     role,
			Language: lang,
		})
	}
	return out, complete
}

func sourceInventoryTrackedSourceLanguage(repoRoot, rel string) string {
	lang := repotypes.DetectLanguage(rel)
	if lang == repotypes.LangTypeScript && repotypes.IsArkTSProject(repoRoot, rel) {
		lang = repotypes.LangArkTS
	}
	return strings.TrimSpace(lang)
}
