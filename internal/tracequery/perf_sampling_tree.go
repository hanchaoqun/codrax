package tracequery

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const ViewPerfSamplingTree = "perf_sampling_tree"
const PerfSamplingTreeTeaching = "perf_sampling_tree builds lossless root-to-leaf sampling prefix trees from exact producer frames, retaining system/UI frames, DSO/IP and source references. Population selectors are time_start/time_end and pid/thread (thread scope); time windows are supported. Content/event/line filters are unsupported for this full-population tree. Direct calls return a typed repair for explicit same-source/window retry; legacy views retain valid filters and may have an unavailable companion tree. Never silently drop an intentional filter or treat unavailable as measured zero. Independent source/generation/event/unit cohorts retain their original denominator including missing/legacy stacks. sampling_depth, sampling_root and limit only expand/collapse display; omitted nodes and references remain counted. Inclusive and self weights are sampling quantities, never projected wall-clock milliseconds, source-code call edges, or root-cause authority. Legacy semicolon strings have unavailable frame boundaries. Reader-facing trees use function/library labels; opaque node/cohort IDs are selectors, not prose labels."

type PerfSamplingTreeResult struct {
	SourcePath     string               `json:"source_path"`
	Window         ResourceStackWindow  `json:"window"`
	Status         string               `json:"status"`
	Reason         string               `json:"reason,omitempty"`
	TargetPID      int                  `json:"target_pid,omitempty"`
	TargetThread   string               `json:"target_thread,omitempty"`
	TargetScope    string               `json:"target_scope,omitempty"`
	SampleCount    int                  `json:"sample_count"`
	CohortCount    int                  `json:"cohort_count"`
	OmittedCohorts int                  `json:"omitted_cohorts"`
	DisplayDepth   int                  `json:"display_depth"`
	DisplayRoot    string               `json:"display_root,omitempty"`
	Cohorts        []PerfSamplingCohort `json:"cohorts,omitempty"`
	Caveats        []string             `json:"caveats,omitempty"`
}

type PerfSamplingCohort struct {
	ID                     string              `json:"id"`
	SourcePath             string              `json:"source_path"`
	SourceGeneration       string              `json:"source_generation"`
	Producer               string              `json:"producer"`
	Event                  string              `json:"event"`
	WeightUnit             string              `json:"weight_unit"`
	SampleKind             string              `json:"sample_kind"`
	Clock                  string              `json:"clock"`
	ThreadIdentity         *PerfThreadIdentity `json:"thread_identity,omitempty"`
	WeightStatus           string              `json:"weight_status"`
	SampleCount            int                 `json:"sample_count"`
	TotalWeight            int64               `json:"total_weight"`
	StackSampleCount       int                 `json:"stack_sample_count"`
	StackWeight            int64               `json:"stack_weight"`
	UnavailableSampleCount int                 `json:"unavailable_sample_count"`
	UnavailableWeight      int64               `json:"unavailable_weight"`
	UnavailableReasons     map[string]int      `json:"unavailable_reasons,omitempty"`
	NodeCount              int                 `json:"node_count"`
	OmittedNodes           int                 `json:"omitted_nodes"`
	Nodes                  []PerfSamplingNode  `json:"nodes,omitempty"`
}

type PerfSamplingReference struct {
	Line        int    `json:"line"`
	FrameIndex  int    `json:"frame_index"`
	SourceRef   string `json:"source_ref"`
	SourceDepth int64  `json:"source_depth"`
}

type PerfSamplingNode struct {
	Kind              string                  `json:"kind,omitempty"`
	ID                string                  `json:"id"`
	ParentID          string                  `json:"parent_id,omitempty"`
	Depth             int                     `json:"depth"`
	Symbol            string                  `json:"symbol"`
	DSO               string                  `json:"dso"`
	IP                string                  `json:"ip"`
	InclusiveSamples  int                     `json:"inclusive_samples"`
	SelfSamples       int                     `json:"self_samples"`
	InclusiveWeight   int64                   `json:"inclusive_weight"`
	SelfWeight        int64                   `json:"self_weight"`
	Percent           float64                 `json:"percent"`
	ChildCount        int                     `json:"child_count"`
	OmittedChildren   int                     `json:"omitted_children"`
	ReferenceCount    int                     `json:"reference_count"`
	OmittedReferences int                     `json:"omitted_references"`
	References        []PerfSamplingReference `json:"references,omitempty"`
}

type samplingNodeAcc struct {
	PerfSamplingNode
	children map[string]*samplingNodeAcc
}
type samplingCohortAcc struct {
	PerfSamplingCohort
	roots map[string]*samplingNodeAcc
}

func samplingID(parts ...string) string {
	b, _ := json.Marshal(parts)
	return fmt.Sprintf("sampling:%x", sha256.Sum256(b))
}

func buildPerfSamplingTree(idx *Index, q Query) *PerfSamplingTreeResult {
	p := &PerfSamplingTreeResult{SourcePath: idx.Path, Window: ResourceStackWindow{StartTs: q.TimeStart, EndTs: q.TimeEnd, EndInclusive: q.timeEndBackfilled}, Status: "unavailable", TargetPID: q.PID, TargetThread: q.Thread, TargetScope: q.TargetScope, DisplayRoot: q.SamplingRoot}
	if p.TargetScope == "" {
		p.TargetScope = TargetScopeThread
	}
	fail := func(reason string) *PerfSamplingTreeResult {
		p.Reason = reason
		p.SampleCount = 0
		p.CohortCount = 0
		p.Cohorts = nil
		return p
	}
	if idx.Windowed || idx.RelationScoped {
		return fail("requires_complete_source_scan")
	}
	if _, _, valid := NormalizePerfSamplingSelector(q.PID, perfSamplingSelectorInput(q)); !valid {
		return fail("invalid_thread_selector")
	}
	if !isFiniteTraceNumber(q.TimeStart) || !isFiniteTraceNumber(q.TimeEnd) || q.TimeEnd < q.TimeStart || q.TimeEnd == q.TimeStart && !q.timeEndBackfilled {
		return fail("requires_finite_window")
	}
	if len(PerfSamplingTreeUnsupportedSelectors(q)) != 0 {
		return fail("requires_time_and_thread_selectors_only")
	}
	depth := q.SamplingDepth
	if depth <= 0 {
		depth = 4
	}
	if depth > tracewire.MaxPerfFrames {
		depth = tracewire.MaxPerfFrames
	}
	p.DisplayDepth = depth
	limit := q.Limit
	if limit <= 0 {
		limit = 64
	}
	if limit > 256 {
		limit = 256
	}
	population := newPerfSamplePopulation(idx, q)
	ledger := population.ledger
	p.Caveats = append(p.Caveats, ledger.caveats()...)
	_, identityAddressed := perfTimelineThreadSelector(q)
	if identityAddressed && idx.threadIncarnationFailuresCapped {
		p.Caveats = append(p.Caveats, "thread_identity_fail_closed=true; lifecycle_audit_truncated; identity-addressed sampling tree is omitted because the bounded lifecycle audit cannot prove its selected generation")
		return fail("typed_identity_unproven")
	}
	cohorts := map[string]*samplingCohortAcc{}
	cohortIDs := map[perfCohortKey]string{}
	totalNodes := 0
	for ordinal, ev := range idx.Events {
		if q.runCancel.tick() {
			return fail("scan_canceled")
		}
		key, identity, known, admitted := population.admit(ordinal, nil)
		if !admitted {
			continue
		}
		pf := ev.PerfFields
		if pf == nil {
			pf = &PerfFields{}
		}
		source, found := samplingSourceForEvent(idx, ev)
		if !found {
			return fail("sample_source_generation_unavailable")
		}
		cohortKey := population.cohortKey(ev, key, known)
		id := cohortIDs[cohortKey]
		c := cohorts[id]
		if c == nil {
			scope := population.scope(cohortKey, identity)
			id = scope.ID
			cohortIDs[cohortKey] = id
			c = &samplingCohortAcc{PerfSamplingCohort: PerfSamplingCohort{ID: id, SourcePath: scope.SourcePath, SourceGeneration: scope.SourceGeneration, Producer: scope.Producer, Event: cohortKey.event, WeightUnit: cohortKey.weightUnit, SampleKind: scope.SampleKind, Clock: scope.Clock, ThreadIdentity: scope.ThreadIdentity, WeightStatus: "exact", UnavailableReasons: map[string]int{}}, roots: map[string]*samplingNodeAcc{}}
			cohorts[id] = c
		}
		weight := perfSampleEffectiveWeight(ev)
		p.SampleCount++
		c.SampleCount++
		if c.TotalWeight > math.MaxInt64-weight {
			c.WeightStatus = "aggregate_overflow"
		} else {
			c.TotalWeight += weight
		}
		frames, reason := perfSampleFrameData(pf)
		if reason != "" {
			c.UnavailableSampleCount++
			c.UnavailableReasons[reason]++
			if c.UnavailableWeight <= math.MaxInt64-weight {
				c.UnavailableWeight += weight
			}
			continue
		}
		c.StackSampleCount++
		if c.StackWeight <= math.MaxInt64-weight {
			c.StackWeight += weight
		}
		children := c.roots
		parent := ""
		for n, f := range frames.Frames {
			if q.runCancel.tick() {
				return fail("scan_canceled")
			}
			frameKey := samplingID(f.Kind, f.Symbol, f.DSO, f.IP)
			node := children[frameKey]
			if node == nil {
				totalNodes++
				if totalNodes > 131072 {
					return fail("sampling_tree_node_budget_exceeded")
				}
				node = &samplingNodeAcc{PerfSamplingNode: PerfSamplingNode{ID: samplingID(c.ID, parent, frameKey), ParentID: parent, Depth: n, Kind: f.Kind, Symbol: f.Symbol, DSO: f.DSO, IP: f.IP}, children: map[string]*samplingNodeAcc{}}
				children[frameKey] = node
				c.NodeCount++
			}
			node.InclusiveSamples++
			if node.InclusiveWeight <= math.MaxInt64-weight {
				node.InclusiveWeight += weight
			}
			if n == len(frames.Frames)-1 {
				node.SelfSamples++
				if node.SelfWeight <= math.MaxInt64-weight {
					node.SelfWeight += weight
				}
			}
			node.ReferenceCount++
			if len(node.References) < 4 {
				node.References = append(node.References, PerfSamplingReference{Line: ev.Line - source.VirtualLineBase, FrameIndex: n, SourceRef: f.SourceRef, SourceDepth: f.SourceDepth})
			}
			parent = node.ID
			children = node.children
		}
	}
	if q.runCancel.sample() {
		return fail("scan_canceled")
	}
	p.Caveats = append(p.Caveats, population.identityCoverageCaveats()...)
	if population.selectorWithheld {
		p.Caveats = append(p.Caveats, perfThreadSelectorWithheldCaveat(q.PID > 0))
		return fail("typed_identity_unproven")
	}
	ids := make([]string, 0, len(cohorts))
	for id := range cohorts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	p.Status = "available"
	p.CohortCount = len(ids)
	for _, id := range ids {
		c := cohorts[id]
		if len(p.Cohorts) >= 64 {
			p.OmittedCohorts++
			continue
		}
		if c.WeightStatus != "exact" {
			c.TotalWeight = 0
			c.StackWeight = 0
			c.UnavailableWeight = 0
			c.OmittedNodes = c.NodeCount
			p.Cohorts = append(p.Cohorts, c.PerfSamplingCohort)
			continue
		}
		var visit func(map[string]*samplingNodeAcc, bool, int)
		visit = func(children map[string]*samplingNodeAcc, selected bool, base int) {
			keys := make([]string, 0, len(children))
			for k := range children {
				keys = append(keys, k)
			}
			sort.Slice(keys, func(i, j int) bool {
				a, b := children[keys[i]], children[keys[j]]
				if a.InclusiveWeight != b.InclusiveWeight {
					return a.InclusiveWeight > b.InclusiveWeight
				}
				return keys[i] < keys[j]
			})
			for _, k := range keys {
				node := children[k]
				active := selected
				if q.SamplingRoot != "" && node.ID == q.SamplingRoot {
					active = true
					base = node.Depth
				}
				if active && node.Depth-base < depth && len(c.Nodes) < limit {
					v := node.PerfSamplingNode
					v.ChildCount = len(node.children)
					v.OmittedChildren = v.ChildCount
					v.OmittedReferences = v.ReferenceCount - len(v.References)
					if c.TotalWeight > 0 {
						v.Percent = 100 * float64(v.InclusiveWeight) / float64(c.TotalWeight)
					}
					c.Nodes = append(c.Nodes, v)
				}
				visit(node.children, active, base)
			}
		}
		visit(c.roots, q.SamplingRoot == "", 0)
		for i := range c.Nodes {
			for _, n := range c.Nodes {
				if n.ParentID == c.Nodes[i].ID {
					c.Nodes[i].OmittedChildren--
				}
			}
		}
		c.OmittedNodes = c.NodeCount - len(c.Nodes)
		p.Cohorts = append(p.Cohorts, c.PerfSamplingCohort)
	}
	p.Caveats = append(p.Caveats, PerfSamplingTreeTeaching, "self_weight is weight at the last recorded frame, not proof of complete unwinding; empty selected windows never fall back to the full capture")
	return p
}

func samplingSourceForEvent(idx *Index, ev Event) (TraceArtifactSource, bool) {
	if len(idx.TraceArtifacts) == 0 {
		return TraceArtifactSource{SourcePath: idx.Path, SourceBytes: idx.Size}, true
	}
	for _, s := range idx.TraceArtifacts {
		if ev.Line > s.VirtualLineBase && ev.Line <= s.VirtualLineBase+s.LocalLineCount {
			return s, true
		}
	}
	return TraceArtifactSource{}, false
}

// ValidPerfSamplingTree validates the bounded observational envelope. Parent
// IDs describe only recorded sampling-prefix structure, never runtime calls.
func ValidPerfSamplingTree(p PerfSamplingTreeResult) bool {
	if p.SourcePath == "" || !isFiniteTraceNumber(p.Window.StartTs) || !isFiniteTraceNumber(p.Window.EndTs) || p.Window.EndTs < p.Window.StartTs || p.SampleCount < 0 || p.CohortCount < len(p.Cohorts) || p.OmittedCohorts != p.CohortCount-len(p.Cohorts) {
		return false
	}
	if p.Status == "unavailable" {
		return p.Reason != "" && len(p.Cohorts) == 0 && p.SampleCount == 0
	}
	if p.Status != "available" || p.Reason != "" {
		return false
	}
	seen := map[string]bool{}
	samples := 0
	for _, c := range p.Cohorts {
		if c.ID == "" || seen[c.ID] || c.SourcePath == "" || c.SourceGeneration == "" || c.SampleCount != c.StackSampleCount+c.UnavailableSampleCount || c.SampleCount < 0 || c.NodeCount < len(c.Nodes) || c.OmittedNodes != c.NodeCount-len(c.Nodes) {
			return false
		}
		seen[c.ID] = true
		samples += c.SampleCount
		if c.WeightStatus == "exact" {
			if c.TotalWeight < 0 || c.StackWeight < 0 || c.UnavailableWeight < 0 || c.StackWeight > c.TotalWeight || c.UnavailableWeight != c.TotalWeight-c.StackWeight {
				return false
			}
		} else if c.WeightStatus != "aggregate_overflow" || c.TotalWeight != 0 || len(c.Nodes) != 0 {
			return false
		}
		nodes := map[string]bool{}
		for _, n := range c.Nodes {
			if n.ID == "" || nodes[n.ID] || n.Depth < 0 || n.InclusiveSamples < 1 || n.SelfSamples < 0 || n.SelfSamples > n.InclusiveSamples || n.InclusiveWeight < 0 || n.InclusiveWeight > c.TotalWeight || n.SelfWeight < 0 || n.SelfWeight > n.InclusiveWeight || n.ChildCount < 0 || n.OmittedChildren < 0 || n.OmittedChildren > n.ChildCount || n.ReferenceCount != n.InclusiveSamples || n.OmittedReferences != n.ReferenceCount-len(n.References) || !isFiniteTraceNumber(n.Percent) {
				return false
			}
			nodes[n.ID] = true
			for _, r := range n.References {
				if r.Line < 1 || r.FrameIndex != n.Depth || r.SourceDepth < 0 || strings.TrimSpace(r.SourceRef) == "" {
					return false
				}
			}
		}
	}
	return samples <= p.SampleCount && (p.OmittedCohorts > 0 || samples == p.SampleCount)
}
