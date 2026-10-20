package agent

import (
	"encoding/json"
	"sort"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Display identities are deliberately separate from query/cohort selectors.
// A numeric native TID already owns this sampling population; its optional
// display name does not filter it. Only a fully resolved, current population
// may share presentation. Original selectors and receipts are never rewritten.
// An available producer tree excludes content/event/line filters. Unavailable
// filtered companions never qualify a legacy flat row or a measurement table.
func nativeSamplingPresentationKey(source string, r types.ObservationRecord, tree tracequery.PerfSamplingTreeResult) string {
	if source == "" || tree.Status != "available" || tree.TargetPID <= 0 || tree.OmittedCohorts != 0 || tree.DisplayRoot != "" {
		return ""
	}
	if _, valid := tool.TraceQueryAggregateSelectionSubject(r); !valid {
		return ""
	}
	copy := tree
	copy.TargetThread = ""
	copy.Cohorts = append([]tracequery.PerfSamplingCohort(nil), tree.Cohorts...)
	for i, cohort := range tree.Cohorts {
		if cohort.SourceGeneration == "" || cohort.ThreadIdentity == nil || cohort.ThreadIdentity.TID != tree.TargetPID || cohort.ThreadIdentity.Generation <= 0 || cohort.OmittedNodes != 0 {
			return ""
		}
		copy.Cohorts[i].ID = ""
		copy.Cohorts[i].Nodes = append([]tracequery.PerfSamplingNode(nil), cohort.Nodes...)
		ids := map[string]string{}
		for j, node := range cohort.Nodes {
			if node.OmittedChildren != 0 {
				return ""
			}
			// Exact parent structure is retained with local positions, never
			// flattened into equal symbols or equal numeric weights.
			id, _ := json.Marshal(j)
			ids[node.ID] = string(id)
		}
		for j, node := range cohort.Nodes {
			copy.Cohorts[i].Nodes[j].ID = ids[node.ID]
			if node.ParentID != "" {
				parent, found := ids[node.ParentID]
				if !found {
					return ""
				}
				copy.Cohorts[i].Nodes[j].ParentID = parent
			}
		}
	}
	// Cohorts are independent populations. Their producer order can depend on
	// selector-derived IDs; sorting whole cohorts changes no tree ordering or
	// denominator and never combines two events, units or thread instances.
	sort.Slice(copy.Cohorts, func(i, j int) bool {
		a, _ := json.Marshal(copy.Cohorts[i])
		b, _ := json.Marshal(copy.Cohorts[j])
		return string(a) < string(b)
	})
	ref := r.SourceRef
	data, _ := json.Marshal([]any{source, ref.Kind, ref.Path, ref.CaptureIdentityPath, ref.ArtifactID, ref.ArtifactKind,
		ref.QueryLineRangeKnown, ref.QueryLineStart, ref.QueryLineEnd,
		ref.TimeDomain, ref.CanonicalTimeDomain, ref.ClockAlignment, ref.ClockCalibrated, ref.ClockOffsetSec, ref.ClockSlope, copy})
	return string(data)
}

// Recompile the actual native results through the same ledger enrichment.
// Source display aliases and tool positions are not guessed or erased to join.
func (p *answerDocNativePresentationReceipt) prepareSamplingIdentities(ctx *types.AgentContext) {
	if p == nil || ctx == nil || p.samplingIdentities != nil {
		return
	}
	p.samplingIdentities = map[string]string{}
	p.samplingCohortIDs = map[string]map[string]bool{}
	p.nativeSources = map[string]string{}
	queryConflicts := map[string]bool{}
	input := types.ObservationLedgerInputFromAgentContext(ctx, 0)
	rawSources, conflicts := map[string]string{}, map[string]bool{}
	var groups [2][]types.ToolResult
	for gi, results := range [][]types.ToolResult{input.ToolResults, input.SystemTraceSupplementResults} {
		for _, result := range results {
			var records []types.ObservationRecord
			for _, r := range result.Observations {
				if nativePresentationFamily(r) == "" {
					continue
				}
				source := types.RuntimeObservationPresentationSourceKey(input, result, r)
				key := traceEventPublicationRecordKey(r)
				if prior, found := rawSources[key]; found && prior != source {
					conflicts[key] = true
				}
				rawSources[key] = source
				records = append(records, r)
			}
			copy := result
			copy.Observations = records
			groups[gi] = append(groups[gi], copy)
		}
	}
	// Compile one record at a time to retain the exact raw-to-enriched binding.
	for gi, results := range groups {
		for index, result := range results {
			for _, raw := range result.Observations {
				key := traceEventPublicationRecordKey(raw)
				if rawSources[key] == "" || conflicts[key] {
					continue
				}
				copies := make([]types.ToolResult, index+1)
				copies[index] = result
				copies[index].Observations = []types.ObservationRecord{raw}
				in := types.ObservationLedgerInput{RepoRoot: input.RepoRoot, RuntimeArtifactPreflight: input.RuntimeArtifactPreflight, RequestModel: input.RequestModel}
				if gi == 0 {
					in.ToolResults = copies
				} else {
					in.SystemTraceSupplementResults = copies
				}
				for _, r := range types.CompileObservationLedger(in).Records {
					p.nativeSources[traceEventPublicationRecordKey(r)] = rawSources[key]
					if tree, valid := tool.DecodeTracePerfSamplingTree(r); valid {
						qkey := nativePresentationQueryKey(r)
						identity := nativeSamplingPresentationKey(rawSources[key], r, tree)
						if prior, found := p.samplingIdentities[qkey]; found && prior != identity {
							queryConflicts[qkey] = true
						}
						p.samplingIdentities[qkey] = identity
						p.samplingCohortIDs[qkey] = map[string]bool{}
						for _, cohort := range tree.Cohorts {
							p.samplingCohortIDs[qkey][cohort.ID] = true
						}
					}
				}
			}
		}
	}
	for key := range queryConflicts {
		p.samplingIdentities[key] = ""
	}
}
