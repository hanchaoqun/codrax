package impact

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// GraphProviderFromSearchGraphForRoots binds graph-local paths to an explicitly
// authorized repository root. Main checkout and current worktree may both be
// supplied by the controller; their names never establish equivalence. A child
// graph stays partial, and no graph data is changed or loaded by this adapter.
// The legacy constructor remains graph-local for existing direct callers.
func GraphProviderFromSearchGraphForRoots(handle any, roots ...string) GraphProvider {
	g, _ := handle.(*rmtypes.Graph)
	if g == nil {
		return nil
	}
	graphRoot := existingGraphRoot(g.Root)
	if graphRoot == "" {
		return nil
	}
	anchor := ""
	for _, raw := range roots {
		root := existingGraphRoot(raw)
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, graphRoot)
		if err == nil && (rel == "." || validGraphRelative(filepath.ToSlash(rel)) != "") && len(root) > len(anchor) {
			anchor = root
		}
	}
	if anchor == "" {
		return nil
	}
	prefix, _ := filepath.Rel(anchor, graphRoot)
	if prefix == "." {
		prefix = ""
	}
	return rootedRepomapGraphProvider{local: repomapGraphProvider{graph: g}, prefix: filepath.ToSlash(prefix)}
}

func existingGraphRoot(raw string) string {
	if !filepath.IsAbs(raw) {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return ""
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return ""
	}
	return filepath.Clean(resolved)
}

func validGraphRelative(raw string) string {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.Contains(raw, `\`) || path.IsAbs(raw) {
		return ""
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

type rootedRepomapGraphProvider struct {
	local  repomapGraphProvider
	prefix string
}

func (p rootedRepomapGraphProvider) localPath(repoPath string) string {
	key := validGraphRelative(repoPath)
	if key == "" {
		return ""
	}
	if p.prefix != "" {
		if !strings.HasPrefix(key, p.prefix+"/") {
			return ""
		}
		key = strings.TrimPrefix(key, p.prefix+"/")
	}
	if p.local.graph.FileIndex[key] == nil {
		return ""
	}
	return key
}

func (p rootedRepomapGraphProvider) repoPath(local string) string {
	key := validGraphRelative(local)
	if key == "" || p.local.graph.FileIndex[key] == nil {
		return ""
	}
	return path.Join(p.prefix, key)
}

func (p rootedRepomapGraphProvider) paths(rows []string) []string {
	var out []string
	for _, row := range rows {
		if mapped := p.repoPath(row); mapped != "" {
			out = append(out, mapped)
		}
	}
	return sortedStrings(out)
}

func (p rootedRepomapGraphProvider) Imports(file string) []string {
	if local := p.localPath(file); local != "" {
		return p.paths(p.local.Imports(local))
	}
	return nil
}
func (p rootedRepomapGraphProvider) ReverseImports(file string) []string {
	if local := p.localPath(file); local != "" {
		return p.paths(p.local.ReverseImports(local))
	}
	return nil
}
func (p rootedRepomapGraphProvider) RelatedTests(file string) []string {
	if local := p.localPath(file); local != "" {
		return p.paths(p.local.RelatedTests(local))
	}
	return nil
}
func (p rootedRepomapGraphProvider) SymbolsInFile(file string) []SymbolRef {
	local := p.localPath(file)
	if local == "" {
		return nil
	}
	var out []SymbolRef
	for _, row := range p.local.SymbolsInFile(local) {
		if mapped := p.repoPath(row.File); mapped != "" {
			row.File = mapped
			out = append(out, row)
		}
	}
	return out
}
func (p rootedRepomapGraphProvider) LineFeaturesInRange(file string, start, end int) []string {
	if local := p.localPath(file); local != "" {
		return p.local.LineFeaturesInRange(local, start, end)
	}
	return nil
}
