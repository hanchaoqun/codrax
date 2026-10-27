package types

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// TraceFrameEvidenceScope is a read-only composition view of one producer
// result. It is not a causal credential, a merged census, or persisted state.
// SelectedProjections binds only the exact observation IDs of elected leads;
// other results remain useful query-local measurements, not current verdicts.
type TraceFrameEvidenceScope struct {
	ResultIndex         int
	Source              ObservationSourceRef
	SourceKnown         bool
	Authority           TraceEvidenceAuthority
	SelectedProjections []int
}

func (s TraceFrameEvidenceScope) SelectedFor(index int) bool {
	for _, selected := range s.SelectedProjections {
		if selected == index {
			return true
		}
	}
	return false
}

func (s TraceFrameEvidenceScope) Locator() string {
	if !s.SourceKnown {
		return fmt.Sprintf("query result %d (source/window unbound; query-local only)", s.ResultIndex+1)
	}
	label := fmt.Sprintf("query result %d source=%q", s.ResultIndex+1, s.Source.Path)
	if s.Source.QueryWindowKnown {
		label += fmt.Sprintf(" query_window=%.9f..%.9f (endpoint inclusion unspecified)", s.Source.QueryWindowStartTs, s.Source.QueryWindowEndTs)
	} else {
		label += " query_window=unknown"
	}
	if s.Source.QueryTargetPID != 0 || s.Source.QueryTargetThread != "" {
		label += fmt.Sprintf(" target=%d/%q", s.Source.QueryTargetPID, s.Source.QueryTargetThread)
	}
	if len(s.SelectedProjections) == 0 {
		label += " (query-local; not the selected evidence)"
	}
	return label
}

// BuildTraceFrameEvidenceScopes never elects a lead or infers a matching scope
// from temporal proximity. Conflicting duplicate IDs cannot bind a result to
// a current projection. Missing locators/observations retain local disclosure.
func BuildTraceFrameEvidenceScopes(input ObservationLedgerInput, set TraceCausalProjectionSet) []TraceFrameEvidenceScope {
	results := append(append([]ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	type origin struct {
		key       runtimeDisplayQueryKey
		authority TraceEvidenceAuthority
	}
	origins, conflicts := map[string]origin{}, map[string]bool{}
	eligible := func(r ObservationRecord) bool {
		return r.Origin.IsValid() && r.Origin != AnswerEvidenceOriginUnknown && r.Origin != AnswerEvidenceOriginCurrentSource
	}
	for i, result := range results {
		if !result.Success || strings.TrimSpace(result.ToolName) != "trace_query" || result.TraceEvidenceAuthority == nil {
			continue
		}
		for j, record := range result.Observations {
			if !eligible(record) {
				continue
			}
			id := strings.TrimSpace(record.ID)
			if id == "" {
				id = fmt.Sprintf("tool:%d#%s:typed:%d", i, strings.TrimSpace(result.ToolName), j)
			}
			value := origin{runtimeDisplayKey(record.SourceRef), *result.TraceEvidenceAuthority}
			if old, ok := origins[id]; ok && (old.key != value.key || !reflect.DeepEqual(old.authority, value.authority)) {
				conflicts[id] = true
			}
			origins[id] = value
		}
	}
	var out []TraceFrameEvidenceScope
	for i, result := range results {
		if !result.Success || strings.TrimSpace(result.ToolName) != "trace_query" || result.TraceEvidenceAuthority == nil {
			continue
		}
		a := result.TraceEvidenceAuthority
		if a.FrameEvidenceStatus == "" && a.FrameItemCount == 0 && a.FrameFlowCausalConclusion == "" {
			continue
		}
		scope := TraceFrameEvidenceScope{ResultIndex: i, Authority: *a}
		ids := map[string]bool{}
		var key runtimeDisplayQueryKey
		consistent := true
		for j, record := range result.Observations {
			if !eligible(record) {
				continue
			}
			candidate := runtimeDisplayKey(record.SourceRef)
			if key == "" {
				key, scope.Source = candidate, record.SourceRef
			} else if key != candidate {
				consistent = false
			}
			id := strings.TrimSpace(record.ID)
			if id == "" {
				id = fmt.Sprintf("tool:%d#%s:typed:%d", i, strings.TrimSpace(result.ToolName), j)
			}
			if !conflicts[id] {
				ids[id] = true
			}
		}
		scope.SourceKnown = consistent && key != "" && scope.Source.Path != "" && scope.Source.QueryScopeID != ""
		if scope.SourceKnown {
			for pi, projection := range set.Projections {
				lead := traceCausalClaimProjectionLead(projection)
				if lead == nil {
					continue
				}
				for _, id := range append([]string{lead.EvidenceID}, lead.MergedEvidenceIDs...) {
					if ids[strings.TrimSpace(id)] {
						scope.SelectedProjections = append(scope.SelectedProjections, pi)
						break
					}
				}
			}
		}
		out = append(out, scope)
	}
	// Spend existing display budgets on current evidence first. Stable order
	// within each lane preserves query-local provenance and never elects one.
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].SelectedProjections) > 0 && len(out[j].SelectedProjections) == 0
	})
	return out
}

// TraceFrameSelectedStatus returns a status only when exact selected results
// agree. A query-local empty/present result cannot resolve a different scope.
func TraceFrameSelectedStatus(scopes []TraceFrameEvidenceScope, projectionIndex int) string {
	status := ""
	for _, scope := range scopes {
		if !scope.SelectedFor(projectionIndex) {
			continue
		}
		next := scope.Authority.FrameEvidenceStatus
		if next == "" || status != "" && status != next {
			return ""
		}
		status = next
	}
	return status
}

func TraceFrameCommonSelectedStatus(scopes []TraceFrameEvidenceScope, set TraceCausalProjectionSet) string {
	status := ""
	for i := range set.Projections {
		next := TraceFrameSelectedStatus(scopes, i)
		if next == "" || status != "" && status != next {
			return ""
		}
		status = next
	}
	return status
}
