package tracequery

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

// perfSamplingSourceGeneration is shared by the population key and the
// display-only member bridge. Keep the existing ID byte contract unchanged.
func perfSamplingSourceGeneration(source TraceArtifactSource) string {
	return samplingID(source.SourcePath, source.sourceIdentity.CacheToken(), source.SourceSHA256, fmt.Sprint(source.SourceBytes), fmt.Sprint(source.SourceModUnixNano))
}

// PerfSamplingTreeBoundSource recovers a single physical sampling member from
// a native query's frozen V2 source universe. The private generation witness
// is required: JSON-decoded provenance, path resemblance and header fields
// cannot mint a member binding. The query engine already verified the complete
// manifest, child digest and held generation before publishing this Result.
// This grants only display selection, never cross-file thread identity,
// causal admission, population completeness or a current-file read receipt.
func PerfSamplingTreeBoundSource(result Result) (TraceArtifactSource, bool) {
	p := result.PerfSamplingTree
	if p == nil || !ValidPerfSamplingTree(*p) || p.Status != "available" || p.SampleCount == 0 ||
		p.SourcePath == "" || p.SourcePath != result.SourcePath || len(p.Cohorts) == 0 || p.OmittedCohorts != 0 {
		return TraceArtifactSource{}, false
	}
	member := p.Cohorts[0].SourcePath
	if member == "" || member == p.SourcePath {
		return TraceArtifactSource{}, false // direct-file compatibility needs no bridge
	}
	var selected TraceArtifactSource
	found := false
	seen := make(map[string]bool, len(result.TraceArtifacts))
	capture := ""
	for _, source := range result.TraceArtifacts {
		if source.SourcePath == "" || source.SourcePath == p.SourcePath || seen[source.SourcePath] ||
			source.BundleSchema != tracebundle.SchemaV2 || tracebundle.ValidateCaptureID(source.CaptureID) != nil ||
			tracebundle.ValidateSHA256(source.SourceSHA256) != nil || !source.sourceIdentity.Strong() ||
			source.SourceBytes != source.sourceIdentity.Size() || source.SourceModUnixNano != source.sourceIdentity.ModUnixNano() {
			return TraceArtifactSource{}, false
		}
		seen[source.SourcePath] = true
		if capture != "" && source.CaptureID != capture {
			return TraceArtifactSource{}, false
		}
		capture = source.CaptureID
		if source.SourcePath == member {
			selected, found = source, true
		}
	}
	if !found {
		return TraceArtifactSource{}, false
	}
	generation := perfSamplingSourceGeneration(selected)
	for _, cohort := range p.Cohorts {
		if cohort.SourcePath != member || cohort.SourceGeneration != generation {
			return TraceArtifactSource{}, false
		}
	}
	return selected, true
}
