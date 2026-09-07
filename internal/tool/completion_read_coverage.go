package tool

import (
	"os"
	"path/filepath"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Both ends use the physical current repository and its exact relative file
// identity. A basename, grep result, or another repository cannot settle this
// display debt. Failure to resolve an advisory remains an unknown scope.
func completionReadSourceIdentity(ctx *types.BusContext, fsPath string) (string, string) {
	if ctx == nil {
		return "", ""
	}
	root := repositoryReadPhysicalIdentity(ctx.RepoRoot)
	file := repositoryReadPhysicalIdentity(fsPath)
	if root == "" || file == "" {
		return "", ""
	}
	rel, within := repoRelativePathWithinRoot(root, file)
	info, err := os.Stat(file)
	if !within || rel == "" || err != nil || !info.Mode().IsRegular() {
		return "", ""
	}
	return root, filepath.ToSlash(rel)
}

func appendAdvisoryReadCoverageCaveat(ctx *types.BusContext, advisory []types.PendingRead) {
	if ctx == nil || ctx.Mutable == nil || len(advisory) == 0 {
		return
	}
	scopes := make([]types.ScopedReadCoverage, 0, len(advisory))
	for _, pending := range advisory {
		path := pending.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(ctx.RepoRoot, filepath.FromSlash(path))
		}
		root, rel := completionReadSourceIdentity(ctx, path)
		scopes = append(scopes, types.ScopedReadCoverage{RepositoryRoot: root, Path: rel, LineRanges: pending.LineRanges})
	}
	ctx.Mutable.EvidenceClosure().AppendScopedReadCoverageCaveat(types.CompletionCaveat{
		Lane:       types.DowngradeLaneForcedReadCoverage,
		ReasonCode: "coverage_reads_demoted",
		Reason:     "coverage-class read suggestions were demoted to advisory at the accepted completion",
	}, scopes)
}

func recordCompletionReadCoverage(ctx *types.BusContext, fsPath string, result types.ToolResult) {
	if ctx == nil || ctx.Mutable == nil || !result.Success || result.ToolName != "read_file" ||
		result.RuntimeArtifactRead != nil || result.ReadCoverage == nil || result.RawRef == "" ||
		result.ReadCoverage.RawRef != result.RawRef {
		return
	}
	root, rel := completionReadSourceIdentity(ctx, fsPath)
	if root == "" || rel == "" {
		return
	}
	coverage := *result.ReadCoverage
	coverage.Path = rel
	ctx.Mutable.EvidenceClosure().RecordScopedReadCoverage(root, coverage)
}
