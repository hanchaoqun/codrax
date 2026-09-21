package tool

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep admission and exact-path enrichment on the same per-probe candidates.
// These are static import relationships, not evidence that code was executed.
func verificationProbeCouplingTargets(repoRoot string, changes []types.FileChange, probe types.VerificationProbe, provider verificationProbeCouplingProvider) map[string]struct{} {
	targets := provider.TargetProducer(repoRoot, changes)
	if provider.Language != "python" {
		return targets
	}
	wd, _, err := resolveVerificationProbeWorkingDir(repoRoot, probe.WorkingDir, "python")
	if err != nil {
		return targets
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return targets
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil || !pythonProbePathInside(realRoot, wd, false) {
		return targets
	}
	// Reuse the runtime's exact PYTHONPATH roots (including supported src/lib
	// layouts); cwd itself is also among these roots for Python -c execution.
	for _, importRoot := range pythonWorktreeImportRoots(root, wd) {
		if !pythonProbePathInside(realRoot, importRoot, false) {
			continue
		}
		for _, change := range changes {
			path := filepath.ToSlash(strings.TrimSpace(change.Path))
			if change.Kind == "delete" || !strings.HasSuffix(path, ".py") || types.LooksLikeTestFilePath(path) || filepath.IsAbs(path) {
				continue
			}
			cleaned := filepath.Clean(filepath.FromSlash(path))
			if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
				continue
			}
			full := filepath.Join(root, filepath.FromSlash(path))
			if !pythonProbePathInside(realRoot, full, change.Kind == "create") {
				continue
			}
			rel, err := filepath.Rel(importRoot, full)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			stem := strings.TrimSuffix(filepath.ToSlash(rel), ".py")
			stem = strings.TrimSuffix(stem, "/__init__")
			valid := true
			for _, part := range strings.Split(stem, "/") {
				if !isPythonIdentifier(part) {
					valid = false
					break
				}
			}
			module := strings.ReplaceAll(stem, "/", ".")
			if valid {
				targets[module] = struct{}{}
			}
		}
	}
	return targets
}

// Canonical containment only constrains newly introduced aliases. It does not
// reinterpret or widen the existing repository/public-package relationships.
func pythonProbePathInside(realRoot, path string, allowMissing bool) bool {
	var realPath string
	for {
		var err error
		realPath, err = filepath.EvalSymlinks(path)
		if err == nil {
			break
		}
		if !allowMissing || !os.IsNotExist(err) {
			return false
		}
		// A planned create may have an absent suffix. Prove containment at
		// its nearest existing ancestor; never step past a dangling symlink.
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return false
		}
		parent := filepath.Dir(path)
		if parent == path {
			return false
		}
		path = parent
	}
	rel, err := filepath.Rel(realRoot, realPath)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
