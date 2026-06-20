package reasoninggraph

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

const readReasoningGraphEventRefLimit = 128

func EventsFromReadAnswerDocument(ctx *types.BusContext, doc *types.AnswerDocumentV2) []ReasoningEvent {
	graphID := readReasoningGraphID(ctx)
	var events []ReasoningEvent
	seq := int64(0)
	add := func(kind ReasoningEventKind, nodeID string, nodeKind ReasoningNodeKind, reason string, payload any, refs []ReasoningEvidenceRef) {
		seq++
		events = append(events, NormalizeEvent(ReasoningEvent{
			GraphID:      graphID,
			NodeID:       nodeID,
			NodeKind:     nodeKind,
			Sequence:     seq,
			Kind:         kind,
			ReasonCode:   strings.TrimSpace(reason),
			EvidenceRefs: refs,
			Payload:      eventPayload(payload),
		}))
	}

	add(ReasoningEventGraphSeeded, "read:run", ReasoningNodeAnalyze, "read_graph_seeded", nil, nil)
	if ctx != nil && ctx.AnalysisIR != nil {
		add(ReasoningEventReadAnalysisProjected, "read:analyze", ReasoningNodeAnalyze, "analysis_ir_projected", map[string]any{
			"scenario":              string(ctx.AnalysisIR.RequestModel.Scenario),
			"intent":                string(ctx.AnalysisIR.RequestModel.Intent),
			"subtopic_count":        len(ctx.AnalysisIR.RequestModel.SubTopics),
			"required_file_count":   len(ctx.AnalysisIR.EvidencePlan.RequiredFiles),
			"task_graph_node_count": len(ctx.AnalysisIR.TaskGraph.Nodes),
		}, nil)
	}

	turnA := readReasoningTurnA(ctx)
	if turnA != nil {
		if len(turnA.EvidenceItems) > 0 || len(turnA.ReadFiles) > 0 || len(turnA.ToolResults) > 0 {
			add(ReasoningEventReadEvidenceProjected, "read:explore", ReasoningNodeExplore, "turn_a_evidence_projected", map[string]any{
				"evidence_item_count": len(turnA.EvidenceItems),
				"read_file_count":     len(turnA.ReadFiles),
				"tool_result_count":   len(turnA.ToolResults),
			}, readEvidenceRefsFromEvidenceItems(turnA.EvidenceItems, "read:explore", 64))
		}
		if len(turnA.AcceptedAggregateFacts) > 0 {
			add(ReasoningEventReadAggregateProjected, "read:extract", ReasoningNodeExtract, "turn_a_aggregate_facts_projected", map[string]any{
				"aggregate_fact_count": len(turnA.AcceptedAggregateFacts),
			}, nil)
		}
		if types.SourceLocalizationReviewHasSignal(turnA.SourceLocalization) {
			review := types.NormalizeSourceLocalizationReview(*turnA.SourceLocalization)
			add(ReasoningEventReadLocalizationProjected, "read:explore", ReasoningNodeExplore, readLocalizationReason(review), map[string]any{
				"status":                  string(review.Status),
				"source_path_count":       len(review.SourcePaths),
				"owner_supported_count":   len(review.OwnerSupportedPaths),
				"owner_missing_count":     len(review.OwnerMissingPaths),
				"missing_path_count":      len(review.MissingPaths),
				"localization_source":     review.Source,
				"localization_reason_set": append([]string(nil), review.ReasonCodes...),
			}, readEvidenceRefsFromLocalization(review, "read:explore", 64))
		}
		if snapshot, ok := loopkernel.ProofSnapshotFromReadTurnA(turnA); ok && snapshot.Authority.State != "" {
			add(ReasoningEventAuthorityProjected, "read:explore", ReasoningNodeExplore, snapshot.Authority.ReasonCode, snapshot, readEvidenceRefsFromReadProofSnapshot(snapshot, "read:explore", 64))
		}
	}

	if doc != nil {
		if types.SourceLocalizationReviewHasSignal(doc.ReadSourceLocalization) {
			review := types.NormalizeSourceLocalizationReview(*doc.ReadSourceLocalization)
			add(ReasoningEventReadLocalizationProjected, "read:finalize", ReasoningNodeFinalize, readLocalizationReason(review), map[string]any{
				"status":                string(review.Status),
				"source_path_count":     len(review.SourcePaths),
				"owner_supported_count": len(review.OwnerSupportedPaths),
				"owner_missing_count":   len(review.OwnerMissingPaths),
				"missing_path_count":    len(review.MissingPaths),
			}, readEvidenceRefsFromLocalization(review, "read:finalize", 64))
		}
		if doc.ReadNavigationCoverage != nil {
			coverage := types.NormalizeRepoMapNavigationCoverage(*doc.ReadNavigationCoverage)
			if coverage.State != "" && coverage.State != types.RepoMapNavigationCoverageNotRequired {
				add(ReasoningEventReadNavigationProjected, "read:finalize", ReasoningNodeFinalize, readNavigationReason(coverage), map[string]any{
					"state":                string(coverage.State),
					"reason_code":          coverage.ReasonCode,
					"required_route_count": len(coverage.RequiredRoutes),
					"covered_route_count":  len(coverage.CoveredRoutes),
					"missing_route_count":  len(coverage.MissingRoutes),
				}, readEvidenceRefsFromNavigation(coverage, "read:finalize", 64))
			}
		}
		if doc.ReadLocalizerFollowup != nil {
			followup := types.NormalizeReadLocalizerFollowup(*doc.ReadLocalizerFollowup)
			if followup.State == types.ReadLocalizerFollowupNeeded {
				add(ReasoningEventReadLocalizationProjected, "read:finalize", followupNodeKind(followup), followup.ReasonCode, map[string]any{
					"state":                      string(followup.State),
					"candidate_path_count":       len(followup.CandidatePaths),
					"missing_route_count":        len(followup.MissingRoutes),
					"evidence_requirement_count": len(followup.EvidenceRequirements),
				}, readEvidenceRefsFromFollowup(followup, "read:finalize", 64))
			}
		}
		if len(doc.Blocks) > 0 || len(doc.Citations) > 0 {
			add(ReasoningEventReadAnswerProjected, "read:finalize", ReasoningNodeFinalize, "answer_document_projected", map[string]any{
				"block_count":          len(doc.Blocks),
				"citation_count":       len(doc.Citations),
				"owner_anchor_count":   len(doc.ReadOwnerAnchors),
				"has_localizer_follow": doc.ReadLocalizerFollowup != nil,
			}, append(readEvidenceRefsFromCitations(doc.Citations, "read:finalize", 64), readEvidenceRefsFromOwnerAnchors(doc.ReadOwnerAnchors, "read:finalize", 64)...))
		}
	}
	return NormalizeEvents(events)
}

func AnswerReasoningGraphSummaryFromReadAnswerDocument(ctx *types.BusContext, doc *types.AnswerDocumentV2) *types.AnswerReasoningGraphSummary {
	events := EventsFromReadAnswerDocument(ctx, doc)
	if len(events) == 0 {
		return nil
	}
	view := ReduceEvents(events)
	out := types.NormalizeAnswerReasoningGraphSummary(types.AnswerReasoningGraphSummary{
		GraphID:          strings.TrimSpace(view.GraphID),
		EventCount:       view.EventCount,
		LastEventKind:    string(view.LastEventKind),
		LastReasonCode:   strings.TrimSpace(view.LastReasonCode),
		EventRefs:        readReasoningGraphEventRefs(events, readReasoningGraphEventRefLimit),
		ReadEventCount:   len(view.ReadEvents),
		ToolEventCount:   len(view.ToolEvents),
		RepairEventCount: len(view.RepairEvents),
		LLMEventCount:    len(view.LLMEvents),
		NodeCount:        len(view.Nodes),
		EvidenceRefCount: len(view.EvidenceRefs),
	})
	if doc != nil {
		out.AnswerBlockCount = len(doc.Blocks)
		out.CitationCount = len(doc.Citations)
	}
	return types.CloneAnswerReasoningGraphSummaryPtr(&out)
}

func readReasoningGraphID(ctx *types.BusContext) string {
	if ctx != nil {
		if traceID := strings.TrimSpace(ctx.TraceID); traceID != "" {
			return traceID
		}
	}
	return "read"
}

func readReasoningTurnA(ctx *types.BusContext) *types.TurnAArtifacts {
	if ctx == nil || ctx.Mutable == nil {
		return nil
	}
	return ctx.Mutable.TurnAArtifacts()
}

func readLocalizationReason(review types.SourceLocalizationReview) string {
	review = types.NormalizeSourceLocalizationReview(review)
	if len(review.ReasonCodes) > 0 {
		return review.ReasonCodes[0]
	}
	if review.Status != "" {
		return string(review.Status)
	}
	return "read_source_localization_projected"
}

func readNavigationReason(coverage types.RepoMapNavigationCoverage) string {
	coverage = types.NormalizeRepoMapNavigationCoverage(coverage)
	if coverage.ReasonCode != "" {
		return coverage.ReasonCode
	}
	if coverage.State != "" {
		return string(coverage.State)
	}
	return "read_navigation_projected"
}

func followupNodeKind(followup types.ReadLocalizerFollowup) ReasoningNodeKind {
	if len(followup.MissingRoutes) > 0 {
		return ReasoningNodeExplore
	}
	return ReasoningNodeFinalize
}

func readEvidenceRefsFromEvidenceItems(items []types.EvidenceItem, nodeID string, limit int) []ReasoningEvidenceRef {
	if limit <= 0 {
		limit = len(items)
	}
	var out []ReasoningEvidenceRef
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = types.StableEvidenceID(item)
		}
		if id == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          id,
			Kind:        string(item.Kind),
			NodeID:      nodeID,
			Priority:    "p1",
			Confidence:  readEvidenceConfidence(item),
			SourceStage: "explore",
			Consumer:    "final_answer",
			ReasonCode:  "turn_a_evidence_item",
			ArtifactRef: loopkernel.ArtifactRef{
				Kind: "source",
				ID:   readPathLineID(item.Source, item.LineStart),
				Path: strings.TrimSpace(item.Source),
			},
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func readEvidenceRefsFromReadProofSnapshot(snapshot loopkernel.ProofSnapshot, nodeID string, limit int) []ReasoningEvidenceRef {
	refs := snapshot.EvidenceRefs
	if limit <= 0 || limit > len(refs) {
		limit = len(refs)
	}
	out := make([]ReasoningEvidenceRef, 0, limit)
	for i, ref := range refs {
		if i >= limit {
			break
		}
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          ref,
			Kind:        string(types.TruthSignalProofCoverage),
			NodeID:      nodeID,
			Priority:    "p0",
			Confidence:  readProofSnapshotConfidence(snapshot.Authority),
			SourceStage: "explore",
			Consumer:    "scheduler",
			ReasonCode:  snapshot.Authority.ReasonCode,
			ArtifactRef: loopkernel.ArtifactRef{
				Kind: "proof",
				ID:   ref,
			},
		})
	}
	return out
}

func readProofSnapshotConfidence(authority loopkernel.ProofCoverageAuthorityView) string {
	switch authority.State {
	case loopkernel.ProofCoverageCovered:
		return "high"
	case loopkernel.ProofCoverageWeak, loopkernel.ProofCoverageUnavailable:
		return "medium"
	case loopkernel.ProofCoverageFailed, loopkernel.ProofCoverageMissing:
		return "low"
	default:
		return ""
	}
}

func readEvidenceRefsFromLocalization(review types.SourceLocalizationReview, nodeID string, limit int) []ReasoningEvidenceRef {
	if limit <= 0 {
		limit = len(review.EvidenceRefs) + len(review.Anchors)
	}
	var out []ReasoningEvidenceRef
	for _, ref := range review.EvidenceRefs {
		id := strings.TrimSpace(firstNonEmpty(ref.ID, readPathLineID(ref.Source, ref.LineStart), ref.Source, ref.OwnerSymbol))
		if id == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          id,
			Kind:        firstNonEmpty(ref.Kind, "localization_evidence"),
			NodeID:      nodeID,
			Priority:    "p1",
			Confidence:  "high",
			SourceStage: "explore",
			Consumer:    "final_answer",
			ReasonCode:  "source_localization_evidence",
			ArtifactRef: loopkernel.ArtifactRef{Kind: "source", ID: readPathLineID(ref.Source, ref.LineStart), Path: strings.TrimSpace(ref.Source)},
		})
		if len(out) >= limit {
			return NormalizeEvidenceRefs(out)
		}
	}
	for _, anchor := range review.Anchors {
		id := strings.TrimSpace(firstNonEmpty(anchor.AnchorSymbol, anchor.OwnerSymbol, anchor.Subject, anchor.Path))
		if id == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          id,
			Kind:        string(anchor.Kind),
			NodeID:      nodeID,
			Priority:    "p1",
			Confidence:  readAnchorConfidence(anchor),
			SourceStage: "explore",
			Consumer:    "final_answer",
			ReasonCode:  "source_localization_anchor",
			ArtifactRef: loopkernel.ArtifactRef{Kind: "source", ID: anchor.Path, Path: strings.TrimSpace(anchor.Path)},
		})
		if len(out) >= limit {
			break
		}
	}
	return NormalizeEvidenceRefs(out)
}

func readEvidenceRefsFromNavigation(coverage types.RepoMapNavigationCoverage, nodeID string, limit int) []ReasoningEvidenceRef {
	if limit <= 0 {
		limit = len(coverage.EvidenceRefs)
	}
	var out []ReasoningEvidenceRef
	for _, raw := range coverage.EvidenceRefs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          ref,
			Kind:        "repo_map_navigation",
			NodeID:      nodeID,
			Priority:    "p1",
			Confidence:  "medium",
			SourceStage: "explore",
			Consumer:    "final_answer",
			ReasonCode:  "repo_map_navigation_evidence",
			ArtifactRef: loopkernel.ArtifactRef{Kind: "repo_map", ID: ref},
		})
		if len(out) >= limit {
			break
		}
	}
	return NormalizeEvidenceRefs(out)
}

func readEvidenceRefsFromFollowup(followup types.ReadLocalizerFollowup, nodeID string, limit int) []ReasoningEvidenceRef {
	if limit <= 0 {
		limit = len(followup.CandidatePaths) + len(followup.EvidenceRequirements)
	}
	var out []ReasoningEvidenceRef
	for _, path := range followup.CandidatePaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          path,
			Kind:        "read_localizer_followup_candidate",
			NodeID:      nodeID,
			Priority:    "p2",
			Confidence:  "unverified",
			SourceStage: "finalize",
			Consumer:    "scheduler",
			ReasonCode:  followup.ReasonCode,
			ArtifactRef: loopkernel.ArtifactRef{Kind: "source", ID: path, Path: path},
		})
		if len(out) >= limit {
			return NormalizeEvidenceRefs(out)
		}
	}
	for i, req := range followup.EvidenceRequirements {
		req = strings.TrimSpace(req)
		if req == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          fmt.Sprintf("read_localizer_requirement:%d", i),
			Kind:        "read_localizer_followup_requirement",
			NodeID:      nodeID,
			Priority:    "p2",
			Confidence:  "unverified",
			SourceStage: "finalize",
			Consumer:    "scheduler",
			ReasonCode:  followup.ReasonCode,
		})
		if len(out) >= limit {
			break
		}
	}
	return NormalizeEvidenceRefs(out)
}

func readEvidenceRefsFromCitations(citations []types.Citation, nodeID string, limit int) []ReasoningEvidenceRef {
	if limit <= 0 {
		limit = len(citations)
	}
	var out []ReasoningEvidenceRef
	for i, citation := range citations {
		path := strings.TrimSpace(citation.File)
		if path == "" {
			continue
		}
		id := readPathLineID(path, citation.Line)
		if id == "" {
			id = fmt.Sprintf("citation:%d", i)
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          id,
			Kind:        "answer_citation",
			NodeID:      nodeID,
			Priority:    "p1",
			Confidence:  "high",
			SourceStage: "finalize",
			Consumer:    "final_answer",
			ReasonCode:  "answer_document_citation",
			ArtifactRef: loopkernel.ArtifactRef{Kind: "source_citation", ID: id, Path: path},
		})
		if len(out) >= limit {
			break
		}
	}
	return NormalizeEvidenceRefs(out)
}

func readEvidenceRefsFromOwnerAnchors(items []types.OwnerAnchorViewItem, nodeID string, limit int) []ReasoningEvidenceRef {
	if limit <= 0 {
		limit = len(items)
	}
	var out []ReasoningEvidenceRef
	for _, item := range types.NormalizeOwnerAnchorView(types.OwnerAnchorView{Items: items}, limit).Items {
		id := strings.TrimSpace(firstNonEmpty(item.ID, item.OwnerSymbol, item.AnchorSymbol, item.Subject, item.Path))
		if id == "" {
			continue
		}
		out = append(out, ReasoningEvidenceRef{
			ID:          id,
			Kind:        string(item.Kind),
			NodeID:      nodeID,
			Priority:    "p1",
			Confidence:  readOwnerAnchorConfidence(item),
			SourceStage: "finalize",
			Consumer:    "final_answer",
			ReasonCode:  "read_owner_anchor",
			ArtifactRef: loopkernel.ArtifactRef{Kind: "source", ID: item.Path, Path: strings.TrimSpace(item.Path)},
		})
		if len(out) >= limit {
			break
		}
	}
	return NormalizeEvidenceRefs(out)
}

func readReasoningGraphEventRefs(events []ReasoningEvent, limit int) []string {
	if limit <= 0 {
		limit = len(events)
	}
	refs := make([]string, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		ref := strings.TrimSpace(event.ID)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
		if len(refs) >= limit {
			break
		}
	}
	return refs
}

func readEvidenceConfidence(item types.EvidenceItem) string {
	switch item.GroundingStatus {
	case types.GroundingGrounded:
		return "high"
	case types.GroundingRecovered:
		return "medium"
	case types.GroundingUngrounded:
		return "low"
	default:
		if item.Confidence >= 0.8 {
			return "high"
		}
		if item.Confidence > 0 {
			return "medium"
		}
		return "unverified"
	}
}

func readAnchorConfidence(anchor types.SourceLocalizationAnchor) string {
	switch anchor.Strength {
	case types.SourceLocalizationAnchorOwner:
		return "high"
	case types.SourceLocalizationAnchorSupporting:
		return "medium"
	default:
		return "unverified"
	}
}

func readOwnerAnchorConfidence(anchor types.OwnerAnchorViewItem) string {
	switch anchor.Strength {
	case types.SourceLocalizationAnchorOwner:
		return "high"
	case types.SourceLocalizationAnchorSupporting:
		return "medium"
	default:
		return "unverified"
	}
}

func readPathLineID(path string, line int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if line > 0 {
		return fmt.Sprintf("%s:%d", path, line)
	}
	return path
}
