package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Discovery is more expansive than an individual file read. Only the active
// repository or a host-pinned path may authorize its traversal; model hints
// and strings found inside capture files cannot enlarge that set.
func traceCatalogAuthorizedRoot(ctx *types.BusContext, requested string) (string, error) {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return "", fmt.Errorf("capture discovery needs an active repository root")
	}
	if strings.TrimSpace(requested) == "" {
		requested = "."
	}
	if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
		gate := gater.ResolveActiveSetPath(ctx, "trace_catalog", requested, nil)
		if !gate.Allowed {
			return "", fmt.Errorf("%s", gate.RefusalProse)
		}
		requested = gate.ResolvedPath
	}
	root, err := filepath.EvalSymlinks(resolveToolPath(ctx, requested))
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	allowed := append([]string{ctx.RepoRoot}, ctx.UserPinnedFiles...)
	for _, scope := range allowed {
		base, err := filepath.EvalSymlinks(resolveToolPath(ctx, scope))
		if err != nil {
			continue
		}
		base, err = filepath.Abs(base)
		if err != nil {
			continue
		}
		info, err := os.Stat(base)
		if err != nil {
			continue
		}
		if root == base {
			return root, nil
		}
		if info.IsDir() {
			rel, err := filepath.Rel(base, root)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return root, nil
			}
		}
	}
	return "", fmt.Errorf("capture discovery root is outside the active repository and host-pinned paths; select that directory as the repository or explicitly pin it")
}
