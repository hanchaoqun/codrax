package types

import (
	"sort"
	"strings"
)

// RuntimeArtifactEnumerationAuthority is the shared exact coverage boundary
// for deterministic trace queries and paged reads of runtime artifacts. It is
// intentionally independent from model prose and from display compaction: an
// emitted page/roster cannot authorize an exhaustive answer when any accepted
// producer result declares its enumeration incomplete.
type RuntimeArtifactEnumerationAuthority struct {
	Incomplete bool
	Scopes     []string
	Boundaries []ToolEnumerationBoundary
}

// BuildRuntimeArtifactEnumerationAuthority compiles the one enumeration view
// consumed by both the model-facing handoff and the deterministic answer
// appendix. Keeping the decision here prevents those two surfaces from
// disagreeing about which runtime results are in scope.
func BuildRuntimeArtifactEnumerationAuthority(results []ToolResult) RuntimeArtifactEnumerationAuthority {
	var out RuntimeArtifactEnumerationAuthority
	seenScopes := map[string]bool{}
	seenBoundaries := map[ToolEnumerationBoundary]bool{}
	for _, result := range results {
		toolName := strings.TrimSpace(result.ToolName)
		inScope := RuntimeObservationProducerIsDeterministicQuery(toolName) ||
			(strings.EqualFold(toolName, "read_file") && result.RuntimeArtifactRead != nil)
		if !inScope || result.EnumerationAuthority == nil ||
			strings.TrimSpace(result.EnumerationAuthority.Status) != "incomplete" {
			continue
		}
		out.Incomplete = true
		for _, boundary := range result.EnumerationAuthority.Boundaries {
			boundary = runtimeArtifactPublicEnumerationBoundary(result, boundary)
			if !seenBoundaries[boundary] {
				seenBoundaries[boundary] = true
				out.Boundaries = append(out.Boundaries, boundary)
			}
			scope := strings.TrimSpace(boundary.Scope)
			if scope == "" || seenScopes[scope] {
				continue
			}
			seenScopes[scope] = true
			out.Scopes = append(out.Scopes, scope)
		}
	}
	sort.Strings(out.Scopes)
	sort.Slice(out.Boundaries, func(i, j int) bool {
		a, b := out.Boundaries[i], out.Boundaries[j]
		if a.Scope != b.Scope {
			return a.Scope < b.Scope
		}
		if a.Dimension != b.Dimension {
			return a.Dimension < b.Dimension
		}
		if a.Emitted != b.Emitted {
			return a.Emitted < b.Emitted
		}
		if a.TotalKnown != b.TotalKnown {
			return !a.TotalKnown
		}
		if a.Total != b.Total {
			return a.Total < b.Total
		}
		return a.Reason < b.Reason
	})
	return out
}

const runtimeTraceQueryResultPageScope = "trace_query_result_page"

// runtimeArtifactPublicEnumerationBoundary keeps private storage identity out
// of the shared model/answer coverage surface. A paged read of a trace_query
// spill is still an exact incomplete-enumeration boundary, but its filesystem
// path is an implementation detail rather than a query view or customer
// artifact identity. The producer-published TraceQueryBlob bit is the only
// discriminator here; no path or model-prose classification participates.
func runtimeArtifactPublicEnumerationBoundary(result ToolResult, boundary ToolEnumerationBoundary) ToolEnumerationBoundary {
	if strings.EqualFold(strings.TrimSpace(result.ToolName), "read_file") &&
		result.RuntimeArtifactRead != nil && result.RuntimeArtifactRead.TraceQueryBlob {
		boundary.Scope = runtimeTraceQueryResultPageScope
	}
	return boundary
}
