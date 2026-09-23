package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Direct pre-read has no tool RawRef. Its already-observed empty bytes may
// settle whole-file display debt only after resolving the same physical
// repository/file identity used by ordinary source-read completion tracking.
func recordEmptyPreReadCoverage(closure *types.EvidenceClosure, repoRoot, file string) {
	if closure == nil || !filepath.IsAbs(repoRoot) {
		return
	}
	root, rootErr := filepath.EvalSymlinks(repoRoot)
	actual, fileErr := filepath.EvalSymlinks(filepath.Join(repoRoot, file))
	if rootErr != nil || fileErr != nil {
		return
	}
	rel, err := filepath.Rel(root, actual)
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	info, err := os.Stat(actual)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		return
	}
	closure.RecordScopedEmptyFileRead(root, filepath.ToSlash(rel))
}
