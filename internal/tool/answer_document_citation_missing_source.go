package tool

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A citation pool has no origin field. An explicit external-observation claim
// therefore prevents the new local-absence check from reclassifying its refs,
// including refs shared with source claims. Existing origin checks remain in
// charge; this is not a new way to grant evidence authority.
func currentSourceCitationExternalClaimRefs(doc *types.AnswerDocumentV2) map[int]bool {
	var refs map[int]bool
	for _, block := range doc.Blocks {
		for _, claim := range block.ClaimUses {
			if claim.ClaimForm != types.ClaimExternalObservation {
				continue
			}
			if refs == nil {
				refs = map[int]bool{}
			}
			for _, item := range block.Items {
				for _, ref := range types.AnswerBlockItemCitationRefs(item) {
					refs[ref] = true
				}
			}
			break
		}
	}
	return refs
}

// Only an authorized local positive-line source coordinate can be disproved
// by absence in this checkout. Read failures, aliases, external references and
// symlinks are not absence receipts. No source content is read here.
func currentSourceCitationProvenAbsent(ctx *types.BusContext, repoRoot string, cit types.Citation) bool {
	if cit.Line <= 0 || strings.TrimSpace(cit.NegativePattern) != "" ||
		(cit.Scope != "" && cit.Scope != types.ScopeLine && cit.Scope != types.ScopeLineRange) ||
		strings.Contains(cit.File, "://") || types.LooksLikeRuntimeArtifactPath(cit.File) {
		return false
	}
	path, ok := currentSourceCitationPath(repoRoot, cit.File)
	if !ok || types.IsSensitiveConfigFilePath(path) || ctx.TypedDenials.IsPathDenied(cit.File) || ctx.TypedDenials.IsPathDenied(path) {
		return false
	}
	if _, restricted := readFileAttachmentPolicy(ctx); restricted {
		return false
	}
	if _, isPublishedArtifact := resolveTraceQueryBlobRefPath(ctx, cit.File); isPublishedArtifact {
		return false
	}
	if gater, ok := ctx.MultiGraph.(types.MultiRepoActiveSetGater); ok && gater != nil {
		gate := gater.ResolveActiveSetPath(ctx, "read_file", cit.File, nil)
		resolved, valid := currentSourceCitationPath(repoRoot, gate.ResolvedPath)
		if !gate.Allowed || gate.AutoPrefixed || !valid || !toolPathsEqual(path, resolved) {
			return false
		}
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || types.IsSensitiveConfigFilePath(filepath.Join(root, rel)) {
		return false
	}
	// Recheck the identities already traversed before accepting a missing child.
	// This detects stable replacements; it is not an atomic filesystem snapshot.
	seen := map[string]os.FileInfo{root: rootInfo}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
				return false
			}
			for ancestor, original := range seen {
				now, checkErr := os.Lstat(ancestor)
				if checkErr != nil || now.Mode()&os.ModeSymlink != 0 || !os.SameFile(original, now) {
					return false
				}
			}
			return true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		seen[current] = info
	}
	return false
}
