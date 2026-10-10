package orchestrator

import (
	"context"
	"time"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
	"github.com/hanchaoqun/codrax/internal/outputdump"
	"github.com/hanchaoqun/codrax/internal/types"
)

// namedTraceContentAdmissionProfile joins the same content-only navigation used
// by the classifier to the existing typed admission path. It does not publish
// this profile, attach files, discover directories, or manufacture a material.
// The caller publishes it only after ordinary preparation and admission.
func namedTraceContentAdmissionProfile(ctx context.Context, bus *types.BusContext, request, anchor string, profile types.RuntimeArtifactPreflightProfile) types.RuntimeArtifactPreflightProfile {
	if bus.AnalysisIR == nil || bus.TraceInputPreparer == nil {
		return profile
	}
	policy := bus.AnalysisIR.RequestModel.ExternalObservationPolicy
	if policy == nil || (!policy.ExcludesCurrentSource() && !policy.ArtifactCitationsExternalOnly()) {
		return profile
	}
	known := map[string]bool{}
	for _, artifact := range profile.Artifacts {
		if artifact.Carrier == "request_path" && artifact.RuntimeArtifactKind() == "trace" {
			known[typedNamedTraceAdmissionPathKey(resolveTypedNamedTraceSource(artifact.Source, bus.RepoRoot))] = true
		}
	}
	paths := outputdump.NamedInputPathsFromRequest(request, bus.RepoRoot, 8)
	// This short probe context never reaches preparation, tool execution, or
	// the LLM. An exhausted optional probe is unknown, not an input rejection.
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, path := range paths {
		key := typedNamedTraceAdmissionPathKey(resolveTypedNamedTraceSource(path, bus.RepoRoot))
		if known[key] {
			continue
		}
		candidate := hitraceconv.ProbeInputRoutingCapability(probeCtx, path, anchor)
		if candidate.NativeReader != "trace_query" {
			continue
		}
		known[key] = true
		profile.Artifacts = append(profile.Artifacts, types.RuntimeArtifactPreflightArtifact{
			Kind: "trace", Source: path, Carrier: "request_path",
			Detail: "named native content; ordinary preparation and complete input admission required",
		})
	}
	return types.NormalizeRuntimeArtifactPreflightProfile(profile)
}
