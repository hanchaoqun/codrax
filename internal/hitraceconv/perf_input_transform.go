package hitraceconv

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func clonePerfInputTransform(transform *PerfInputTransform) *PerfInputTransform {
	if transform == nil {
		return nil
	}
	clone := *transform
	return &clone
}

func validatePerfInputTransformShape(transform *PerfInputTransform) error {
	if transform == nil {
		return fmt.Errorf("perf input transform is absent")
	}
	if transform.Profile != perfInputTransformGzipV1 ||
		transform.SourceArtifactPath == "" || strings.TrimSpace(transform.SourceArtifactPath) != transform.SourceArtifactPath ||
		strings.ContainsRune(transform.SourceArtifactPath, 0) ||
		transform.SourceFormat != string(perfInputGzipPerfData) ||
		transform.DecodedFormat != string(perfInputLinuxPerfData) ||
		transform.SourceBytes < 18 || transform.SourceBytes > hiperfGzipMaxCompressedBytes ||
		transform.DecodedBytes < int64(len(perfMagic2)) || transform.DecodedBytes > hiperfGzipMaxDecodedBytes ||
		transform.DecodedBytes > transform.SourceBytes*hiperfGzipMaxCompressionRatio {
		return fmt.Errorf("perf input transform tuple is outside the closed gzip profile")
	}
	if err := tracebundle.ValidateSHA256(transform.SourceSHA256); err != nil {
		return fmt.Errorf("perf input transform source sha256: %w", err)
	}
	if err := tracebundle.ValidateSHA256(transform.DecodedSHA256); err != nil {
		return fmt.Errorf("perf input transform decoded sha256: %w", err)
	}
	return nil
}

func validatePerfInputTransformArtifacts(artifacts []Artifact) error {
	type sourceClaim struct {
		artifact Artifact
		count    int
	}
	sources := make(map[string]sourceClaim)
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactPerfData || artifact.Perf == nil || artifact.Perf.InputFormat != string(perfInputGzipPerfData) {
			continue
		}
		claim := sources[artifact.Path]
		claim.artifact = artifact
		claim.count++
		sources[artifact.Path] = claim
	}
	seenSource := make(map[string]bool)
	for _, artifact := range artifacts {
		if artifact.PerfTransform == nil {
			if artifact.Type == ArtifactPerfTrace && artifact.Perf != nil && artifact.Perf.InputFormat == string(perfInputGzipPerfData) {
				return fmt.Errorf("gzip-derived perftrace has no input transform provenance")
			}
			continue
		}
		if artifact.Type != ArtifactPerfTrace || artifact.Perf == nil || artifact.Perf.InputFormat != string(perfInputGzipPerfData) {
			return fmt.Errorf("perf input transform is attached outside a gzip-derived perftrace")
		}
		if err := validatePerfInputTransformShape(artifact.PerfTransform); err != nil {
			return err
		}
		claim, ok := sources[artifact.PerfTransform.SourceArtifactPath]
		if !ok || claim.count != 1 || seenSource[artifact.PerfTransform.SourceArtifactPath] {
			return fmt.Errorf("perf input transform has no unique gzip source artifact")
		}
		source := claim.artifact
		if source.Standalone == nil || source.Bytes != artifact.PerfTransform.SourceBytes ||
			source.SHA256 != artifact.PerfTransform.SourceSHA256 {
			return fmt.Errorf("perf input transform disagrees with its gzip source artifact")
		}
		seenSource[artifact.PerfTransform.SourceArtifactPath] = true
	}
	return nil
}
