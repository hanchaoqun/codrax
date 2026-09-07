package tool

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
	"github.com/hanchaoqun/codrax/internal/types"
)

func protectedBaselineExactPath(raw string) (string, bool) {
	// Pattern-looking bytes can be literal filename bytes. Their presence
	// disables only the naming-convention shortcut, not exact observed files.
	if strings.ContainsAny(raw, "\x00\r\n") || strings.HasSuffix(strings.ReplaceAll(strings.TrimSpace(raw), `\`, "/"), "/") {
		return "", false
	}
	return canonpath.CanonicalRepoRelativeIdentity(raw)
}

func repositoryReadPhysicalIdentity(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return ""
	}
	physical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return ""
	}
	return filepath.Clean(physical)
}

func recordSuccessfulRepositoryRead(ctx *types.BusContext, sourceRepoRoot, fsPath string, result types.ToolResult) {
	if ctx == nil || ctx.Mutable == nil || !result.Success || result.ToolName != "read_file" || result.RuntimeArtifactRead != nil || result.ReadCoverage == nil {
		return
	}
	coverage := result.ReadCoverage
	path, ok := protectedBaselineExactPath(coverage.Path)
	if !ok || result.RawRef == "" || coverage.RawRef != result.RawRef {
		return
	}
	root := repositoryReadPhysicalIdentity(sourceRepoRoot)
	actualFile := repositoryReadPhysicalIdentity(fsPath)
	if root == "" || actualFile == "" {
		return
	}
	rel, within := repoRelativePathWithinRoot(root, actualFile)
	if !within || rel == "" {
		return
	}
	info, err := os.Stat(actualFile)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	ctx.Mutable.RecordDispatchRepositoryFileRead(root, path, result.RawRef)
}

func protectedBaselineObservedInCurrentRepository(ctx *types.BusContext, target string) bool {
	if ctx == nil || ctx.Mutable == nil {
		return false
	}
	root := repositoryReadPhysicalIdentity(ctx.RepoRoot)
	if root == "" {
		return false
	}
	actualFile := repositoryReadPhysicalIdentity(filepath.Join(ctx.RepoRoot, filepath.FromSlash(target)))
	if rel, within := repoRelativePathWithinRoot(root, actualFile); !within || rel == "" {
		return false
	}
	info, err := os.Stat(actualFile)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	for _, result := range ctx.Mutable.DispatchToolResults() {
		coverage := result.ReadCoverage
		if !result.Success || result.ToolName != "read_file" || result.RuntimeArtifactRead != nil || coverage == nil ||
			coverage.LineStart <= 0 || coverage.LineEnd < coverage.LineStart || coverage.TotalLines < coverage.LineEnd ||
			result.RawRef == "" || coverage.RawRef != result.RawRef {
			continue
		}
		path, ok := protectedBaselineExactPath(coverage.Path)
		if ok && path == target && ctx.Mutable.HasDispatchRepositoryFileRead(root, path, result.RawRef) {
			return true
		}
	}
	return false
}

func protectedBaselineTargetIsDirectory(ctx *types.BusContext, target string) bool {
	if ctx == nil || strings.TrimSpace(ctx.RepoRoot) == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(ctx.RepoRoot, filepath.FromSlash(target)))
	return err == nil && info.IsDir()
}
