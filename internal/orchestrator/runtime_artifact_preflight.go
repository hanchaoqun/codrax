package orchestrator

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeArtifactPreflightProfileForRun(request, repoRoot, attachedLog, attachedTrace string) types.RuntimeArtifactPreflightProfile {
	artifacts := outputdump.MergeRuntimeArtifacts(
		runtimeArtifactsFromRequestRepoRoot(request, repoRoot),
		outputdump.RuntimeArtifactsFromRequest(request),
		outputdump.RuntimeArtifactsFromAttachment("log", attachedLog),
		outputdump.RuntimeArtifactsFromAttachment("trace", attachedTrace),
	)
	if len(artifacts) == 0 {
		return types.RuntimeArtifactPreflightProfile{}
	}
	items := make([]types.RuntimeArtifactPreflightArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		carrier := "request_path"
		if strings.Contains(strings.ToLower(artifact.Detail), "attached") {
			carrier = "attachment"
		}
		items = append(items, types.RuntimeArtifactPreflightArtifact{
			Kind:    artifact.Kind,
			Source:  artifact.Source,
			Bytes:   artifact.Bytes,
			Detail:  artifact.Detail,
			Carrier: carrier,
		})
	}
	return types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		Active:                   true,
		SourceNavigationOptional: true,
		ReasonCode:               types.RuntimeArtifactPreflightReasonDetected,
		Artifacts:                items,
		RepoSourceCensus:         repoSourceCensusForRun(repoRoot),
	})
}

const repoSourceCensusMaxEntries = 4096

var (
	errRepoSourceCensusSourceFound = errors.New("repo source census: source file found")
	errRepoSourceCensusTooLarge    = errors.New("repo source census: entry cap exceeded")
)

// repoSourceCensusForRun deterministically classifies every regular file under
// repoRoot as runtime-artifact or current-source, by path shape only. It
// early-exits as soon as one current-source file is found — for ordinary
// source repositories this terminates almost immediately, and one source file
// already settles ZeroCurrentSourceRepo=false — and abandons the census
// (Completed=false, inert) past repoSourceCensusMaxEntries so huge artifact
// dumps cannot stall run entry. Hidden entries (dotfiles, .git, .codrax) are
// VCS/tool state, not citable investigation source, and are skipped. Only
// called when the preflight already detected a runtime artifact.
func repoSourceCensusForRun(repoRoot string) types.RuntimeArtifactRepoSourceCensus {
	repoRoot = strings.TrimSpace(repoRoot)
	if repoRoot == "" {
		return types.RuntimeArtifactRepoSourceCensus{}
	}
	census := types.RuntimeArtifactRepoSourceCensus{Completed: true}
	entries := 0
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == repoRoot {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		entries++
		if entries > repoSourceCensusMaxEntries {
			return errRepoSourceCensusTooLarge
		}
		if types.RuntimeArtifactPathKind(d.Name()) != "" {
			census.ArtifactFiles++
			return nil
		}
		census.SourceFiles++
		return errRepoSourceCensusSourceFound
	})
	if err != nil && !errors.Is(err, errRepoSourceCensusSourceFound) {
		return types.RuntimeArtifactRepoSourceCensus{}
	}
	return census
}

func runtimeArtifactsFromRequestRepoRoot(request, repoRoot string) []outputdump.RuntimeArtifact {
	repoRoot = strings.TrimSpace(repoRoot)
	if request == "" || repoRoot == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []outputdump.RuntimeArtifact
	for _, token := range types.RuntimeArtifactPathTokensInText(request) {
		kind := types.RuntimeArtifactPathKind(token)
		if kind == "" {
			continue
		}
		resolved := token
		if strings.HasPrefix(resolved, "~/") {
			home, err := os.UserHomeDir()
			if err != nil || home == "" {
				continue
			}
			resolved = filepath.Join(home, strings.TrimPrefix(resolved, "~/"))
		}
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(repoRoot, resolved)
		}
		resolved = filepath.Clean(resolved)
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			continue
		}
		key := strings.ToLower(kind + "\x00" + token)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, outputdump.RuntimeArtifact{
			Kind:   kind,
			Source: token,
			Bytes:  safeInt64ToInt(info.Size()),
			Detail: "referenced in request; resolved against repo root",
		})
	}
	return out
}

func safeInt64ToInt(v int64) int {
	if v <= 0 {
		return 0
	}
	if int64(int(v)) != v {
		return int(^uint(0) >> 1)
	}
	return int(v)
}
