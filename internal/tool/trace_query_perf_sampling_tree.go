package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

const TracePerfSamplingTreePredicate = "perf_sampling_tree_observation"

func traceQueryPerfSamplingTreeSchema(schema json.RawMessage) json.RawMessage {
	var obj map[string]any
	if json.Unmarshal(schema, &obj) != nil {
		return schema
	}
	props, _ := obj["properties"].(map[string]any)
	view, _ := props["view"].(map[string]any)
	if view == nil {
		return schema
	}
	description, _ := view["description"].(string)
	view["description"] = description + " " + tracequery.PerfSamplingTreeTeaching
	props["sampling_depth"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 256, "description": "perf_sampling_tree only: displayed prefix depth, default 4; never changes the cohort denominator."}
	props["sampling_root"] = map[string]any{"type": "string", "description": "perf_sampling_tree only: exact node ID from a prior same-source/window query to expand. Omission shows roots; unmatched IDs do not fall back to full trees."}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if e.Encode(obj) != nil {
		return schema
	}
	return b.Bytes()
}

func (t *TraceQuery) streamPerfSamplingTree(ctx *types.BusContext, p traceQueryParams, path, sourceLabel, callCaveat string, window traceQueryNormalizedWindow) (types.ToolResult, bool) {
	if tracequery.CanonicalViewName(p.View) != tracequery.ViewPerfSamplingTree {
		return types.ToolResult{}, false
	}
	q := traceQueryBuildQuery(ctx, p, sourceLabel, path, window.RequestedStart, window.RequestedEnd)
	result, err := tracequery.StreamPerfSamplingTree(contextFromBus(ctx), path, q)
	if err != nil {
		if traceQueryIsCancellation(err) {
			return traceQueryCancellationResult(p.View, path, err), true
		}
		return types.ToolResult{ToolName: t.Name(), Success: false, Summary: "Sampling tree query failed: " + err.Error(), Timestamp: time.Now()}, true
	}
	traceQueryAppendCallCaveats(&result, callCaveat)
	payload, failure := traceQueryMarshalPayload(t.Name(), result)
	if failure != nil {
		return *failure, true
	}
	payloadRef := StoreBlobArtifact(ctxWorkDir(ctx), t.Name(), "trace-query-result.json", string(payload))
	preview, rawRef := StoreBlob(ctx, t.Name(), traceQuerySummary(result, p, sourceLabel, payloadRef))
	if rawRef == "" {
		rawRef = payloadRef
	}
	now := time.Now()
	return traceQuerySamplingTreeRepairResult(types.ToolResult{ToolName: t.Name(), Success: true, Summary: preview, RawRef: rawRef, Timestamp: now, Observations: traceQueryTypedObservations(result, sourceLabel, payloadRef, rawRef, "", now, q), TraceQuerySourceRead: traceQuerySourceReadCandidate(result)}, result, q), true
}

func boundedPerfSamplingTree(p tracequery.PerfSamplingTreeResult) tracequery.PerfSamplingTreeResult {
	data, _ := json.Marshal(p)
	var copy tracequery.PerfSamplingTreeResult
	_ = json.Unmarshal(data, &copy)
	for {
		data, _ = json.Marshal(copy)
		if len(data) <= 48<<10 && len(TracePerfSamplingTreeText(copy)) <= 48<<10 {
			return copy
		}
		if len(copy.Cohorts) == 0 {
			return copy
		}
		last := &copy.Cohorts[len(copy.Cohorts)-1]
		if len(last.Nodes) > 0 {
			removed := last.Nodes[len(last.Nodes)-1]
			last.Nodes = last.Nodes[:len(last.Nodes)-1]
			last.OmittedNodes++
			for i := range last.Nodes {
				if last.Nodes[i].ID == removed.ParentID {
					last.Nodes[i].OmittedChildren++
				}
			}
		} else {
			copy.Cohorts = copy.Cohorts[:len(copy.Cohorts)-1]
			copy.OmittedCohorts++
		}
	}
}

func traceQueryPerfSamplingTreeObservations(p *tracequery.PerfSamplingTreeResult, ref types.ObservationSourceRef, scope, at string) []types.ObservationRecord {
	if p == nil || !tracequery.ValidPerfSamplingTree(*p) {
		return nil
	}
	copy := boundedPerfSamplingTree(*p)
	data, err := json.Marshal(copy)
	if err != nil {
		return nil
	}
	ref.QueryWindowKnown = true
	ref.QueryWindowStartTs = p.Window.StartTs
	ref.QueryWindowEndTs = p.Window.EndTs
	if pid, thread, valid := tracequery.NormalizePerfSamplingSelector(ref.QueryTargetPID, ref.QueryTargetThread); valid {
		ref.QueryTargetPID, ref.QueryTargetThread = pid, thread
	}
	return []types.ObservationRecord{{ID: "trace_query:" + scope + "#perf_sampling_tree", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", Role: types.AnswerAggregateRoleSupportingCoverage, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan, SourceRef: ref, Span: types.ObservationSpan{StartTs: p.Window.StartTs, EndTs: p.Window.EndTs}, ClaimKey: TracePerfSamplingTreePredicate, Predicate: TracePerfSamplingTreePredicate, Subject: "sampled prefix tree", Object: p.Status, Summary: "Source-bound sampled prefixes and independent cohort weights; not invocation or causal evidence", RichNotes: []string{types.TraceNoteKeyPerfSamplingTree + "=" + string(data)}, ObservedAt: at, Confidence: 1}}
}

func DecodeTracePerfSamplingTree(r types.ObservationRecord) (tracequery.PerfSamplingTreeResult, bool) {
	var p tracequery.PerfSamplingTreeResult
	ref := r.SourceRef
	if r.Negative || r.Predicate != TracePerfSamplingTreePredicate || r.ClaimKey != r.Predicate || r.Subject != "sampled prefix tree" || r.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(r.Producer) || r.Role != types.AnswerAggregateRoleSupportingCoverage || r.GroundingPolicy != types.ClaimGroundingHard || r.ProvenanceLane != types.ObservationProvenanceArtifactSpan || ref.Kind != types.ObservationSourceRuntimeArtifact || ref.Path == "" || ref.QueryScopeID == "" || ref.PayloadRef == "" || !ref.QueryWindowKnown || !ref.QueryLineRangeKnown || ref.QueryLineStart != 0 || ref.QueryLineEnd != 0 {
		return p, false
	}
	count := 0
	for _, note := range r.RichNotes {
		if strings.HasPrefix(note, types.TraceNoteKeyPerfSamplingTree+"=") {
			count++
			raw := json.RawMessage(strings.TrimPrefix(note, types.TraceNoteKeyPerfSamplingTree+"="))
			if !traceRootCauseReportHasUniqueNestedKeys(raw) || json.Unmarshal(raw, &p) != nil {
				return p, false
			}
		}
	}
	return p, count == 1 && tracequery.ValidPerfSamplingTree(p) && p.SourcePath == ref.Path && p.Status == r.Object && p.TargetPID == ref.QueryTargetPID && p.TargetThread == ref.QueryTargetThread && p.TargetScope == ref.QueryTargetScope && p.Window.StartTs == ref.QueryWindowStartTs && p.Window.EndTs == ref.QueryWindowEndTs && p.Window.StartTs == r.Span.StartTs && p.Window.EndTs == r.Span.EndTs
}

func TracePerfSamplingTreeText(p tracequery.PerfSamplingTreeResult) string {
	var b strings.Builder
	end := ")"
	if p.Window.EndInclusive {
		end = "]"
	}
	fmt.Fprintf(&b, "Sampling prefix tree: status=%s reason=%s window=[%.9f,%.9f%s end_inclusive=%t samples=%d cohorts=%d omitted_cohorts=%d\n", p.Status, p.Reason, p.Window.StartTs, p.Window.EndTs, end, p.Window.EndInclusive, p.SampleCount, p.CohortCount, p.OmittedCohorts)
	for _, c := range p.Cohorts {
		weight := func(n int64) string {
			if c.WeightStatus != "exact" {
				return "unknown"
			}
			return fmt.Sprint(n)
		}
		fmt.Fprintf(&b, "cohort=%s source=%q generation=%s producer=%q event=%q unit=%q kind=%q weight_status=%s samples=%d total_weight=%s stack_weight=%s unavailable_weight=%s nodes=%d omitted_nodes=%d\n", c.ID, c.SourcePath, c.SourceGeneration, c.Producer, c.Event, c.WeightUnit, c.SampleKind, c.WeightStatus, c.SampleCount, weight(c.TotalWeight), weight(c.StackWeight), weight(c.UnavailableWeight), c.NodeCount, c.OmittedNodes)
		reasons, _ := json.Marshal(c.UnavailableReasons)
		fmt.Fprintf(&b, "unavailable_stacks=%d reasons=%s\n", c.UnavailableSampleCount, reasons)
		if c.ThreadIdentity != nil {
			fmt.Fprintf(&b, "thread_tid=%d generation=%d comm=%q\n", c.ThreadIdentity.TID, c.ThreadIdentity.Generation, c.ThreadIdentity.DisplayComm)
		}
		for _, n := range c.Nodes {
			if n.Kind == "context_marker" {
				b.WriteString("context_marker (not a function frame): ")
			}
			fmt.Fprintf(&b, "node=%s parent=%s depth=%d symbol=%q dso=%q ip=%q inclusive=%d self=%d samples=%d self_samples=%d cohort_percent=%.6f children=%d omitted_children=%d references=%d omitted_references=%d\n", n.ID, n.ParentID, n.Depth, n.Symbol, n.DSO, n.IP, n.InclusiveWeight, n.SelfWeight, n.InclusiveSamples, n.SelfSamples, n.Percent, n.ChildCount, n.OmittedChildren, n.ReferenceCount, n.OmittedReferences)
			for _, r := range n.References {
				fmt.Fprintf(&b, "  raw_ref line=%d frame_index=%d source_ref=%q source_depth=%d\n", r.Line, r.FrameIndex, r.SourceRef, r.SourceDepth)
			}
		}
	}
	for _, caveat := range p.Caveats {
		if caveat != tracequery.PerfSamplingTreeTeaching {
			fmt.Fprintf(&b, "caveat=%s\n", caveat)
		}
	}
	b.WriteString(tracequery.PerfSamplingTreeTeaching + " Self weight is the last recorded frame, not proof of complete unwinding.\n")
	return b.String()
}
