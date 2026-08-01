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
	byPath                map[string]runtimeArtifactPreflightSource
	repoRoot              string
	attachedTraceSource   runtimeArtifactPreflightSource
	attachedTraceResolved bool
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
	attachedTraceByKey := map[string]runtimeArtifactPreflightSource{}
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
		entry, exists := index.byPath[key]
		if !exists {
			entry = runtimeArtifactPreflightSource{
				path:         source,
				artifactID:   "runtime_artifact:" + RuntimeArtifactHashString(kind+"\x00"+source),
				artifactKind: kind,
			}
			index.byPath[key] = entry
		}
		if strings.EqualFold(strings.TrimSpace(artifact.Carrier), "attachment") &&
			kind == "trace" && runtimeArtifactAttachmentSourceIsAddressable(source) {
			attachedTraceByKey[key] = entry
		}
	}
	if len(attachedTraceByKey) == 1 {
		for _, source := range attachedTraceByKey {
			index.attachedTraceSource = source
			index.attachedTraceResolved = true
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
	if !ok && index.recordUsesAttachedTraceMaterialization(record, canonicalPath) {
		artifact, ok = index.attachedTraceSource, true
	}
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
	record.SourceRef.CaptureIdentityPath = artifact.path
	// Keep the producer's existing grounding strength. In particular,
	// trace_query publishes hard pair-atomic rows; origin enrichment must not
	// soften those rows and silently remove them from causal projection.
	return record
}

// recordUsesAttachedTraceMaterialization recognizes only Codrax's typed
// attachment-materialization lane. A user file that merely happens to be
// named attached_trace.txt carries ArtifactID=trace_query and cannot enter.
// Multiple attachment candidates leave attachedTraceResolved false and fail
// open to distinct paths.
func (index runtimeArtifactPreflightSourceIndex) recordUsesAttachedTraceMaterialization(record ObservationRecord, canonicalPath string) bool {
	if !index.attachedTraceResolved ||
		!strings.EqualFold(strings.TrimSpace(record.SourceRef.ArtifactID), "attached_trace") ||
		!strings.EqualFold(strings.TrimSpace(record.SourceRef.ArtifactKind), "trace") {
		return false
	}
	return ReservedRuntimeArtifactBlobKind(canonicalPath) == "trace"
}

func runtimeArtifactAttachmentSourceIsAddressable(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "", "-", "(inline)", "attached_trace", "harmony_hitrace", "android_atrace", "generic_ftrace":
		return false
	default:
		return true
	}
}

// RuntimeArtifactCaptureIdentityPath returns the physical-capture identity
// when the ledger could prove one, otherwise the producer's addressable path.
// Runtime-artifact grouping consumers share this chokepoint; citation and raw
// read consumers must continue to use Path.
func RuntimeArtifactCaptureIdentityPath(ref ObservationSourceRef) string {
	if identity := strings.TrimSpace(ref.CaptureIdentityPath); identity != "" {
		return identity
	}
	return strings.TrimSpace(ref.Path)
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
