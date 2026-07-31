package types

import (
	"runtime"
	"strings"

	"github.com/hanchaoqun/codrax/internal/canonpath"
)

// runtimeArtifactPreflightSourceIndex is the exact run-entry identity set for
// attached/resolved runtime artifacts. It prevents a repo-relative attachment
// from changing provenance merely because a later producer accessed it through
// a source-shaped tool.
//
// This is deliberately not a runtime-artifact path detector: .txt attachments
// are valid, and unrelated .log/.sys files are not attachments. Only a path
// present in RuntimeArtifactPreflight may match.
type runtimeArtifactPreflightSourceIndex struct {
	byPath   map[string]runtimeArtifactPreflightSource
	repoRoot string
}

type runtimeArtifactPreflightSource struct {
	path         string
	artifactID   string
	artifactKind string
}

func compileRuntimeArtifactPreflightSourceIndex(profile RuntimeArtifactPreflightProfile, repoRoot string) runtimeArtifactPreflightSourceIndex {
	normalized := NormalizeRuntimeArtifactPreflightProfile(profile)
	index := runtimeArtifactPreflightSourceIndex{
		byPath:   make(map[string]runtimeArtifactPreflightSource, len(normalized.Artifacts)),
		repoRoot: strings.TrimSpace(repoRoot),
	}
	for _, artifact := range normalized.Artifacts {
		source := canonicalRuntimeArtifactIdentityPath(artifact.Source, repoRoot)
		if source == "" {
			continue
		}
		kind := artifact.RuntimeArtifactKind()
		if kind == "" {
			kind = strings.TrimSpace(artifact.Kind)
		}
		key := runtimeArtifactIdentityPathKey(source)
		if _, exists := index.byPath[key]; exists {
			continue
		}
		index.byPath[key] = runtimeArtifactPreflightSource{
			path:         source,
			artifactID:   "runtime_artifact:" + RuntimeArtifactHashString(kind+"\x00"+source),
			artifactKind: kind,
		}
	}
	return index
}

func (index runtimeArtifactPreflightSourceIndex) requalify(record ObservationRecord) ObservationRecord {
	if len(index.byPath) == 0 {
		return record
	}
	sourcePath := strings.TrimSpace(record.SourceRef.Path)
	if sourcePath == "" {
		return record
	}
	canonicalPath := canonicalRuntimeArtifactIdentityPath(sourcePath, index.repoRoot)
	artifact, ok := index.byPath[runtimeArtifactIdentityPathKey(canonicalPath)]
	if !ok {
		return record
	}
	switch record.Origin {
	case AnswerEvidenceOriginCurrentSource,
		AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginSystemInference,
		AnswerEvidenceOriginRuntimeArtifact:
		// These are the only ledger origins a local read/search/evidence
		// producer can assign before the run-entry artifact identity is
		// consulted. Do not override VCS, web, connector, MCP, or command
		// namespaces merely because they happen to mention the same string.
	default:
		return record
	}
	record.Origin = AnswerEvidenceOriginRuntimeArtifact
	record.SourceRef.Kind = ObservationSourceRuntimeArtifact
	record.SourceRef.ArtifactID = artifact.artifactID
	record.SourceRef.ArtifactKind = artifact.artifactKind
	// Keep the producer's existing grounding strength. In particular,
	// trace_query publishes hard pair-atomic rows; origin enrichment must not
	// soften those rows and silently remove them from causal projection.
	return record
}

func canonicalRuntimeArtifactIdentityPath(source, repoRoot string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	return canonpath.CanonicalRepoRelative(source, repoRoot)
}

func runtimeArtifactIdentityPathKey(source string) string {
	source = canonpath.CanonicalRepoRelative(strings.TrimSpace(source), "")
	if runtime.GOOS == "windows" || looksLikeWindowsIdentityPath(source) {
		return strings.ToLower(source)
	}
	return source
}

func looksLikeWindowsIdentityPath(source string) bool {
	if len(source) >= 3 &&
		((source[0] >= 'a' && source[0] <= 'z') || (source[0] >= 'A' && source[0] <= 'Z')) &&
		source[1] == ':' && source[2] == '/' {
		return true
	}
	return strings.HasPrefix(source, "//")
}
