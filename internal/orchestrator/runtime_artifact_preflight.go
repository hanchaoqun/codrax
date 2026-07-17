package orchestrator

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/types"
)

func runtimeArtifactPreflightProfileForRun(request, repoRoot, attachedLog, attachedTrace string) types.RuntimeArtifactPreflightProfile {
	requestArtifacts := mergeRuntimeArtifactPreflightRequestArtifacts(
		outputdump.RuntimeArtifactsFromRequest(request),
		runtimeArtifactsFromRequestRepoRoot(request, repoRoot),
	)
	attachmentArtifacts := outputdump.MergeRuntimeArtifacts(
		outputdump.RuntimeArtifactsFromAttachment("log", attachedLog),
		outputdump.RuntimeArtifactsFromAttachment("trace", attachedTrace),
	)
	if len(requestArtifacts) == 0 && len(attachmentArtifacts) == 0 {
		return types.RuntimeArtifactPreflightProfile{}
	}
	items := make([]types.RuntimeArtifactPreflightArtifact, 0, len(requestArtifacts)+len(attachmentArtifacts))
	appendItem := func(artifact outputdump.RuntimeArtifact, carrier string) {
		items = append(items, types.RuntimeArtifactPreflightArtifact{
			Kind:    artifact.Kind,
			Source:  artifact.Source,
			Bytes:   artifact.Bytes,
			Detail:  artifact.Detail,
			Carrier: carrier,
		})
	}
	for _, artifact := range requestArtifacts {
		carrier := "bundle_child"
		if artifact.Carrier == outputdump.RuntimeArtifactCarrierDirectRequestPath {
			carrier = "request_path"
		}
		appendItem(artifact, carrier)
	}
	for _, artifact := range attachmentArtifacts {
		carrier := "attachment"
		if artifact.Carrier == outputdump.RuntimeArtifactCarrierBundleChild {
			carrier = "bundle_child"
		}
		appendItem(artifact, carrier)
	}
	return types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		Active:                   true,
		SourceNavigationOptional: true,
		ReasonCode:               types.RuntimeArtifactPreflightReasonDetected,
		Artifacts:                items,
		RepoSourceCensus:         repoSourceCensusForRun(repoRoot),
	})
}

// mergeRuntimeArtifactPreflightRequestArtifacts keeps one typed root per
// user-named source even when the repo-root resolver calls it "trace" while
// metadata inspection refines the same physical file to "tracebundle". Bundle
// children remain distinct derived carriers. The richer metadata group is
// passed first; later deterministic stat data may only fill missing size/detail.
func mergeRuntimeArtifactPreflightRequestArtifacts(groups ...[]outputdump.RuntimeArtifact) []outputdump.RuntimeArtifact {
	rootBySource := map[string]int{}
	var out []outputdump.RuntimeArtifact
	for _, group := range groups {
		for _, artifact := range outputdump.MergeRuntimeArtifacts(group) {
			if artifact.Carrier != outputdump.RuntimeArtifactCarrierDirectRequestPath {
				out = append(out, artifact)
				continue
			}
			key := strings.TrimSpace(artifact.Source)
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if index, ok := rootBySource[key]; ok {
				if out[index].Bytes <= 0 && artifact.Bytes > 0 {
					out[index].Bytes = artifact.Bytes
				}
				if strings.TrimSpace(out[index].Detail) == "" {
					out[index].Detail = artifact.Detail
				}
				continue
			}
			rootBySource[key] = len(out)
			out = append(out, artifact)
		}
	}
	return out
}

const repoSourceCensusMaxEntries = 4096

var (
	errRepoSourceCensusSourceFound  = errors.New("repo source census: source file found")
	errRepoSourceCensusTooLarge     = errors.New("repo source census: entry cap exceeded")
	errRepoSourceCensusUnverifiable = errors.New("repo source census: symlink makes reachable source unverifiable")
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
		if d.Type()&fs.ModeSymlink != 0 {
			// WalkDir does not follow symlinks, so anything behind this
			// entry is invisible to the walk — yet read_file/grep can
			// reach it (bazel/buck outputs, monorepo overlay layouts
			// symlink whole source trees). Claiming "zero current source"
			// on partial visibility would waive floors that are actually
			// satisfiable; make the census inert instead.
			return errRepoSourceCensusUnverifiable
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
		if filegeneration.IsWindowsNamedPipePath(resolved) {
			// Never let deterministic run-entry profiling stat a Windows pipe
			// namespace. It is not a regular runtime artifact and may block.
			continue
		}
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
		tokenKey := token
		if runtime.GOOS == "windows" {
			tokenKey = strings.ToLower(tokenKey)
		}
		key := strings.ToLower(kind) + "\x00" + tokenKey
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, outputdump.RuntimeArtifact{
			Kind:    kind,
			Source:  token,
			Bytes:   safeInt64ToInt(info.Size()),
			Detail:  "referenced in request; resolved against repo root",
			Carrier: outputdump.RuntimeArtifactCarrierDirectRequestPath,
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
