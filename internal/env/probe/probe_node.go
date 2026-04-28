package probe

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// probeNodes finds node + its accompanying package managers on
// PATH. A typical Node install ships npm; pnpm / yarn / bun are
// independently installable. The probe records whichever subset
// the operator has so Recommender can pick the right install
// command.
func probeNodes(facts *types.EnvFacts) []types.NodeInterp {
	path, err := exec.LookPath("node")
	if err != nil {
		return nil
	}
	if real, rerr := filepath.EvalSymlinks(path); rerr == nil {
		path = real
	}
	n := types.NodeInterp{Path: path}
	if v, err := captureCmd(path, "--version"); err == nil {
		// node prints "v20.11.0\n"
		n.Version = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	}
	for _, pm := range []string{"npm", "pnpm", "yarn", "bun"} {
		if facts.HasPkgManager(pm) {
			n.PkgManagers = append(n.PkgManagers, pm)
		}
	}
	return []types.NodeInterp{n}
}
