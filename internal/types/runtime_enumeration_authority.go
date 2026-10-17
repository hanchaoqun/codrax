package types

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RuntimeArtifactEnumerationAuthority is the shared exact coverage boundary
// for deterministic trace queries and paged reads of runtime artifacts. It is
// intentionally independent from model prose and from display compaction: a
// boundary constrains only its own producer result, never a sibling query or
// an independently published native collection.
type RuntimeArtifactEnumerationAuthority struct {
	Incomplete       bool
	Scopes           []string
	Boundaries       []ToolEnumerationBoundary
	ScopedBoundaries []RuntimeEnumerationBoundary
}

const (
	RuntimeEnumerationQueryResult  = "query_result"
	RuntimeEnumerationArtifactPage = "artifact_page"
)

// RuntimeEnumerationBoundary retains exact producer coordinates independently
// of the public scope label. Page bytes/lines are not query members. These
// locators identify a limitation; they confer no read, fact or causal authority.
type RuntimeEnumerationBoundary struct {
	CoverageKind              string
	Boundary                  ToolEnumerationBoundary
	BoundaryKnown             bool
	ResultRef                 string
	SourceRefs                []ObservationSourceRef
	ReadPage                  *ToolRuntimeArtifactRead
	UnlocatedResultOrdinal    int
	UnavailableSourceLocators int
}

// BuildRuntimeArtifactEnumerationAuthority compiles the one enumeration view
// consumed by both the model-facing handoff and the deterministic answer
// appendix. Keeping the decision here prevents those two surfaces from
// disagreeing about which runtime results are in scope.
func BuildRuntimeArtifactEnumerationAuthority(results []ToolResult) RuntimeArtifactEnumerationAuthority {
	var out RuntimeArtifactEnumerationAuthority
	seenScopes := map[string]bool{}
	seenBoundaries := map[string]bool{}
	for resultIndex, result := range results {
		toolName := strings.TrimSpace(result.ToolName)
		inScope := RuntimeObservationProducerIsDeterministicQuery(toolName) ||
			(strings.EqualFold(toolName, "read_file") && result.RuntimeArtifactRead != nil)
		if !inScope || result.EnumerationAuthority == nil ||
			strings.TrimSpace(result.EnumerationAuthority.Status) != "incomplete" {
			continue
		}
		out.Incomplete = true
		boundaries := result.EnumerationAuthority.Boundaries
		if len(boundaries) == 0 {
			boundaries = []ToolEnumerationBoundary{{}}
		}
		for boundaryIndex, boundary := range boundaries {
			boundary = runtimeArtifactPublicEnumerationBoundary(result, boundary)
			entry := runtimeEnumerationBoundaryForResult(result, boundary)
			entry.BoundaryKnown = len(result.EnumerationAuthority.Boundaries) > 0
			located := entry.ResultRef != "" || entry.ReadPage != nil && entry.ReadPage.RequestedPath != ""
			for _, source := range entry.SourceRefs {
				// A file alone cannot prove two calls selected the same set.
				located = located || source.PayloadRef != "" || source.QueryScopeID != ""
			}
			if !located {
				entry.UnlocatedResultOrdinal = resultIndex + 1
			}
			encoded, err := json.Marshal(entry)
			key := string(encoded)
			if err != nil {
				key = fmt.Sprintf("unencodable-result-%d-boundary-%d", resultIndex, boundaryIndex)
			}
			if !seenBoundaries[key] {
				seenBoundaries[key] = true
				out.Boundaries = append(out.Boundaries, boundary)
				out.ScopedBoundaries = append(out.ScopedBoundaries, entry)
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
	sort.SliceStable(out.ScopedBoundaries, func(i, j int) bool {
		a, b := out.ScopedBoundaries[i].Boundary, out.ScopedBoundaries[j].Boundary
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
	for i := range out.ScopedBoundaries {
		out.Boundaries[i] = out.ScopedBoundaries[i].Boundary
	}
	return out
}

func runtimeEnumerationBoundaryForResult(result ToolResult, boundary ToolEnumerationBoundary) RuntimeEnumerationBoundary {
	entry := RuntimeEnumerationBoundary{CoverageKind: RuntimeEnumerationQueryResult, Boundary: boundary, ResultRef: result.RawRef}
	if strings.EqualFold(strings.TrimSpace(result.ToolName), "read_file") && result.RuntimeArtifactRead != nil {
		entry.CoverageKind = RuntimeEnumerationArtifactPage
		page := *result.RuntimeArtifactRead
		entry.ReadPage = &page
	}
	seen := map[string]bool{}
	for _, record := range result.Observations {
		r := record.SourceRef
		// Keep producer-level population coordinates, not per-event lines,
		// labels, command transcripts or navigation mirrors. A limitation is
		// never a new source publication or an evidence grant.
		source := ObservationSourceRef{
			Kind: r.Kind, Path: r.Path, Commit: r.Commit, Repo: r.Repo,
			CaptureIdentityPath: r.CaptureIdentityPath, PayloadRef: r.PayloadRef,
			RawRef: r.RawRef, ArtifactID: r.ArtifactID,
			QueryScopeID: r.QueryScopeID, QueryWindowKnown: r.QueryWindowKnown,
			QueryWindowStartTs: r.QueryWindowStartTs, QueryWindowEndTs: r.QueryWindowEndTs,
			QueryTargetPID: r.QueryTargetPID, QueryTargetThread: r.QueryTargetThread, QueryTargetScope: r.QueryTargetScope,
			QueryLineRangeKnown: r.QueryLineRangeKnown, QueryLineStart: r.QueryLineStart, QueryLineEnd: r.QueryLineEnd,
			TimeDomain: r.TimeDomain, CanonicalTimeDomain: r.CanonicalTimeDomain,
			ClockAlignment: r.ClockAlignment, ClockCalibrated: r.ClockCalibrated,
			ClockOffsetSec: r.ClockOffsetSec, ClockSlope: r.ClockSlope,
		}
		encoded, err := json.Marshal(source)
		if err != nil {
			entry.UnavailableSourceLocators++
			continue
		}
		if seen[string(encoded)] || source == (ObservationSourceRef{}) {
			continue
		}
		seen[string(encoded)] = true
		var snapshot ObservationSourceRef
		if err := json.Unmarshal(encoded, &snapshot); err != nil {
			entry.UnavailableSourceLocators++
			continue
		}
		entry.SourceRefs = append(entry.SourceRefs, snapshot)
	}
	sort.Slice(entry.SourceRefs, func(i, j int) bool {
		a, _ := json.Marshal(entry.SourceRefs[i])
		b, _ := json.Marshal(entry.SourceRefs[j])
		return string(a) < string(b)
	})
	return entry
}

// WithoutScope is the common answer projection. Rebuild all compatibility
// fields together so a hidden ranking boundary cannot survive in another lane.
func (a RuntimeArtifactEnumerationAuthority) WithoutScope(scope string) RuntimeArtifactEnumerationAuthority {
	var out RuntimeArtifactEnumerationAuthority
	seen := map[string]bool{}
	for _, entry := range a.ScopedBoundaries {
		if strings.EqualFold(strings.TrimSpace(entry.Boundary.Scope), scope) {
			continue
		}
		out.ScopedBoundaries = append(out.ScopedBoundaries, entry)
		out.Boundaries = append(out.Boundaries, entry.Boundary)
		if s := strings.TrimSpace(entry.Boundary.Scope); s != "" && !seen[s] {
			seen[s] = true
			out.Scopes = append(out.Scopes, s)
		}
	}
	sort.Strings(out.Scopes)
	out.Incomplete = len(out.ScopedBoundaries) > 0
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
