package index

import (
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// protoImportResolver resolves protobuf imports by exact path,
// caller-relative include path, then repo-wide suffix match. That
// covers the common layouts used by proto include roots without
// inventing build-system-specific logic.
type protoImportResolver struct {
	suffixIndex map[string][]string
}

func (r *protoImportResolver) Language() string { return types.LangProto }

func (r *protoImportResolver) Prepare(g *types.Graph, _ *ResolverContext) error {
	r.suffixIndex = make(map[string][]string)
	for _, fi := range g.Files {
		if fi.Language != types.LangProto {
			continue
		}
		rel := filepath.ToSlash(fi.RelPath)
		parts := strings.Split(rel, "/")
		for i := 0; i < len(parts); i++ {
			key := strings.Join(parts[i:], "/")
			if key == "" {
				continue
			}
			r.suffixIndex[key] = types.AppendUnique(r.suffixIndex[key], rel)
		}
	}
	return nil
}

func (r *protoImportResolver) Resolve(_ *types.Graph, fi *types.FileInfo, imp types.Import, ctx *ResolverContext) []string {
	path := strings.TrimSpace(filepath.ToSlash(imp.Path))
	path = strings.Trim(path, `"'`)
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	if _, ok := ctx.FileIndex[path]; ok {
		return []string{path}
	}
	relCandidate := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(fi.RelPath), path)))
	if _, ok := ctx.FileIndex[relCandidate]; ok {
		return []string{relCandidate}
	}
	if files := r.suffixIndex[path]; len(files) > 0 {
		return files
	}
	ctx.Unresolved = append(ctx.Unresolved, types.UnresolvedImport{
		File: fi.RelPath, Raw: imp.Path, Reason: "proto_external",
	})
	return nil
}
