package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	answerDocRuntimeTemporalCapsuleItemLimit = 12
	answerDocRuntimeTemporalCapsuleEdgeLimit = 8
	answerDocRuntimeTemporalCapsuleLimit     = 2
)

type answerDocRuntimeTemporalItemAuthority struct {
	ItemIndex                 int    `json:"item_index"`
	Node                      string `json:"node"`
	ItemStageRole             string `json:"item_stage_role"`
	OwningThreadRoleAuthority string `json:"owning_thread_role_authority"`
	InternalWorkAuthority     string `json:"internal_work_authority"`
}

// renderAnswerDocRuntimeTemporalDiagramCapsules gives the model a report-local,
// copy-ready authoring aid for a pure temporal frame result. It deliberately
// does not compile across trace_query calls: an earlier coarse probe and a
// later bounded report are different authority seats and must not lend rows to
// each other. The aid is prompt-only; the model may copy it or omit the diagram,
// and no system path inserts it into the answer.
func renderAnswerDocRuntimeTemporalDiagramCapsules(ctx *types.AgentContext) string {
	if ctx == nil {
		return ""
	}
	input := types.ObservationLedgerInputFromAgentContext(ctx, 1)
	results := append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...)
	return renderAnswerDocRuntimeTemporalDiagramCapsulesFromResults(results)
}

func renderAnswerDocRuntimeTemporalDiagramCapsulesFromResults(results []types.ToolResult) string {
	var b strings.Builder
	seen := map[string]bool{}
	emitted := 0
	for _, result := range results {
		capsule, ok := answerDocRuntimeTemporalDiagramCapsule(result)
		if !ok || seen[capsule] {
			continue
		}
		seen[capsule] = true
		b.WriteString(capsule)
		emitted++
		if emitted >= answerDocRuntimeTemporalCapsuleLimit {
			break
		}
	}
	return b.String()
}

// answerDocRuntimeTemporalDiagramCapsule accepts only a complete local typed
// row set. Counts that exceed the bounded authoring surface, missing rows, or
// mixed/untyped producers fail open by withholding the optional aid; they do
// not trigger a retry and they never manufacture an edge from prose.
func answerDocRuntimeTemporalDiagramCapsule(result types.ToolResult) (string, bool) {
	authority := result.TraceEvidenceAuthority
	if !result.Success || authority == nil ||
		strings.TrimSpace(authority.FrameFlowCausalConclusion) != "unproven" ||
		strings.TrimSpace(authority.FrameFlowRelationAuthority) != "temporal_sequence" ||
		authority.FrameItemCount <= 0 || authority.FrameFlowEdgeCount <= 0 ||
		authority.FrameItemCount > answerDocRuntimeTemporalCapsuleItemLimit ||
		authority.FrameFlowEdgeCount > answerDocRuntimeTemporalCapsuleEdgeLimit {
		return "", false
	}

	items := make([]types.ObservationRecord, 0, authority.FrameItemCount)
	edges := make([]types.ObservationRecord, 0, authority.FrameFlowEdgeCount)
	seenItems := map[string]bool{}
	seenEdges := map[string]bool{}
	for _, record := range result.Observations {
		if !answerDocRuntimeTemporalCapsuleRecord(record) {
			continue
		}
		predicate := strings.TrimSpace(record.Predicate)
		switch {
		case strings.HasPrefix(predicate, "frame_timeline_"):
			key := answerDocRuntimeTemporalRecordIdentity(record)
			if !seenItems[key] {
				seenItems[key] = true
				items = append(items, record)
			}
		case predicate == "frame_temporal_sequence":
			key := answerDocRuntimeTemporalRecordIdentity(record)
			if !seenEdges[key] {
				seenEdges[key] = true
				edges = append(edges, record)
			}
		}
	}
	if len(items) != authority.FrameItemCount || len(edges) != authority.FrameFlowEdgeCount {
		return "", false
	}

	aliases := map[string]string{}
	identities := make([]string, 0, len(items)+len(edges)*2)
	addIdentity := func(raw string) bool {
		identity := strings.TrimSpace(raw)
		if identity == "" || strings.ContainsAny(identity, "\r\n") {
			return false
		}
		if _, exists := aliases[identity]; !exists {
			aliases[identity] = fmt.Sprintf("t%d", len(identities)+1)
			identities = append(identities, identity)
		}
		return true
	}
	for _, item := range items {
		if !addIdentity(item.Subject) {
			return "", false
		}
	}
	for _, edge := range edges {
		if !addIdentity(edge.Subject) || !addIdentity(edge.Object) {
			return "", false
		}
	}

	anchors := make([]types.DiagramEdgeAnchor, 0, len(edges))
	for _, edge := range edges {
		anchors = append(anchors, types.DiagramEdgeAnchor{
			FromNode:     aliases[strings.TrimSpace(edge.Subject)],
			ToNode:       aliases[strings.TrimSpace(edge.Object)],
			RelationKind: types.DiagramRelTemporal,
		})
	}
	anchorJSON, err := json.Marshal(anchors)
	if err != nil {
		return "", false
	}
	itemAuthorities := make([]answerDocRuntimeTemporalItemAuthority, 0, len(items))
	for index, item := range items {
		itemAuthorities = append(itemAuthorities, answerDocRuntimeTemporalItemAuthority{
			ItemIndex:                 index + 1,
			Node:                      aliases[strings.TrimSpace(item.Subject)],
			ItemStageRole:             firstNonEmptyString(strings.TrimPrefix(strings.TrimSpace(item.Predicate), "frame_timeline_"), "item"),
			OwningThreadRoleAuthority: "not_provided_by_this_item",
			InternalWorkAuthority:     "not_provided_by_this_item",
		})
	}
	itemAuthorityJSON, err := json.Marshal(itemAuthorities)
	if err != nil {
		return "", false
	}

	var b strings.Builder
	b.WriteString("\n#### Copy-ready optional report-local temporal diagram\n\n")
	b.WriteString("- This optional authoring aid is built from one trace result's complete typed frame-item and temporal-edge rows. `item_authority_json` is the compact semantic ceiling for the Notes: item-stage role is not an owning-thread role or internal-work claim. It is authority guidance only, not an AnswerDocument field and not visible diagram text. The result does not prove a call, handoff, wait, completion dependency, or root cause. Copy the short Mermaid body and complete `edge_anchors_json` together, or omit the diagram; do not change Notes into arrows.\n\n")
	b.WriteString("```mermaid\nsequenceDiagram\n")
	for _, identity := range identities {
		fmt.Fprintf(&b, "  participant %s as %s\n", aliases[identity], answerDocMechanismMermaidLabel(identity))
	}
	for _, item := range items {
		role := strings.TrimPrefix(strings.TrimSpace(item.Predicate), "frame_timeline_")
		phase := strings.TrimSpace(item.Object)
		label := fmt.Sprintf("item_stage_role=%s", firstNonEmptyString(role, "item"))
		if phase != "" {
			label += "; phase=" + phase
		}
		label += fmt.Sprintf("; interval=%.6f..%.6f", item.Span.StartTs, item.Span.EndTs)
		if item.Span.LineStart > 0 {
			label += fmt.Sprintf("; lines=%d..%d", item.Span.LineStart, max(item.Span.LineStart, item.Span.LineEnd))
		}
		fmt.Fprintf(&b, "  Note over %s: %s\n",
			aliases[strings.TrimSpace(item.Subject)], answerDocMechanismMermaidLabel(label))
	}
	for _, edge := range edges {
		fmt.Fprintf(&b, "  %s-->>%s: %s\n",
			aliases[strings.TrimSpace(edge.Subject)], aliases[strings.TrimSpace(edge.Object)],
			types.RuntimeTraceTemporalDiagramEdgeLabel)
	}
	b.WriteString("```\n")
	fmt.Fprintf(&b, "- edge_anchors_json=`%s`\n", anchorJSON)
	fmt.Fprintf(&b, "- item_authority_json=`%s`\n", itemAuthorityJSON)
	return b.String(), true
}

func answerDocRuntimeTemporalCapsuleRecord(record types.ObservationRecord) bool {
	return record.Origin == types.AnswerEvidenceOriginRuntimeArtifact &&
		types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) &&
		record.GroundingPolicy == types.ClaimGroundingHard &&
		strings.HasPrefix(strings.TrimSpace(record.ClaimKey), "evidence_fact:")
}

func answerDocRuntimeTemporalRecordIdentity(record types.ObservationRecord) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%.9f\x00%.9f",
		strings.TrimSpace(record.SourceRef.CaptureIdentityPath),
		strings.TrimSpace(record.SourceRef.Path),
		strings.TrimSpace(record.Predicate),
		strings.TrimSpace(record.Subject),
		strings.TrimSpace(record.Object),
		record.Span.LineStart,
		record.Span.LineEnd,
		record.Span.StartTs,
		record.Span.EndTs,
	)
}
