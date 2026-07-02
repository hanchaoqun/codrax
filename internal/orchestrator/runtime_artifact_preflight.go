package orchestrator

import (
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
	})
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
