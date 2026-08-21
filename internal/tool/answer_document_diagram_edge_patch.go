package tool

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the resource bound explicit in both the projected JSON schema and the
// executor. A normal diagram repair may legitimately need one operation per
// visible edge (including body-only edges that were rejected for missing
// anchors), so the old limit of 16 made some fully validated transactions
// impossible to express. The surrounding answer/block/body size limits remain
// the primary payload bounds; this cap is a final defence against pathological
// edit arrays.
const maxModelAuthoredDiagramEdgeEdits = 128

// applyModelAuthoredDiagramAtomicEdits turns model-declared edge and boundary
// operations into full block replacements over the previous model-authored
// carrier. This is a structural patch compiler, not an answer normalizer: the
// model supplies every target tuple, operation, replacement tuple, visible
// label, and uncertainty boundary. The compiler preserves all unmentioned
// block fields and Mermaid lines, then the ordinary relation and participant
// evidence gates validate the merged document.
func applyModelAuthoredDiagramAtomicEdits(
	prev *types.AnswerDocumentV2,
	patch *types.AnswerDocumentV2Patch,
	edits []emitAnswerDiagramEdgeEdit,
	boundaries []emitAnswerDiagramBoundaryReplacement,
	lease *types.AnswerDiagramRelationRepairLease,
) error {
	if prev == nil || patch == nil {
		return fmt.Errorf("previous answer and patch are required")
	}
	if len(edits) == 0 && len(boundaries) == 0 {
		return nil
	}
	if len(edits) > maxModelAuthoredDiagramEdgeEdits {
		return fmt.Errorf("too many edits: got %d, max %d", len(edits), maxModelAuthoredDiagramEdgeEdits)
	}
	previous := make(map[string]types.AnswerBlock, len(prev.Blocks))
	ambiguous := make(map[string]bool)
	for _, block := range prev.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		if _, exists := previous[id]; exists {
			ambiguous[id] = true
			continue
		}
		previous[id] = block
	}
	claimed := make(map[string]string)
	for _, id := range patch.UnchangedBlockIDs {
		claimed[strings.TrimSpace(id)] = "unchanged_block_ids"
	}
	for _, block := range patch.ReplaceBlocks {
		claimed[strings.TrimSpace(block.ID)] = "replace_blocks"
	}
	for _, block := range patch.AddBlocks {
		claimed[strings.TrimSpace(block.ID)] = "add_blocks"
	}
	for _, id := range patch.RemoveBlockIDs {
		claimed[strings.TrimSpace(id)] = "remove_block_ids"
	}

	working := make(map[string]types.AnswerBlock)
	order := make([]string, 0, len(edits)+len(boundaries))
	loadBlock := func(blockID string, index int, field string) (types.AnswerBlock, error) {
		if block, exists := working[blockID]; exists {
			return block, nil
		}
		if op, exists := claimed[blockID]; exists {
			return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q conflicts with %s", field, index, blockID, op)
		}
		base, ok := previous[blockID]
		if !ok || ambiguous[blockID] {
			return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q does not uniquely select a previous block", field, index, blockID)
		}
		if base.Kind != types.BlockDiagram || base.Diagram == nil || strings.TrimSpace(base.Diagram.Body) == "" {
			return types.AnswerBlock{}, fmt.Errorf("%s[%d] block_id=%q is not an existing diagram carrier", field, index, blockID)
		}
		block := cloneAtomicDiagramPatchBlock(base)
		working[blockID] = block
		order = append(order, blockID)
		return block, nil
	}
	for i, edit := range edits {
		blockID := strings.TrimSpace(edit.BlockID)
		if blockID == "" {
			return fmt.Errorf("edit[%d] has empty block_id", i)
		}
		block, err := loadBlock(blockID, i, "diagram_edge_edits")
		if err != nil {
			return err
		}
		if err := applyOneModelAuthoredDiagramEdgeEdit(&block, edit, lease); err != nil {
			return fmt.Errorf("edit[%d] block_id=%q: %w", i, blockID, err)
		}
		working[blockID] = block
	}
	boundarySeen := make(map[string]bool, len(boundaries))
	for i, replacement := range boundaries {
		blockID := strings.TrimSpace(replacement.BlockID)
		if blockID == "" {
			return fmt.Errorf("diagram_boundary_replacements[%d] has empty block_id", i)
		}
		if boundarySeen[blockID] {
			return fmt.Errorf("diagram_boundary_replacements[%d] duplicates block_id=%q", i, blockID)
		}
		boundarySeen[blockID] = true
		block, err := loadBlock(blockID, i, "diagram_boundary_replacements")
		if err != nil {
			return err
		}
		seenParticipants := make(map[string]bool, len(replacement.ParticipantBoundaries))
		for j, boundary := range replacement.ParticipantBoundaries {
			participant := strings.TrimSpace(boundary.Participant)
			if participant == "" || boundary.Status != types.DiagramParticipantBoundaryUnproven {
				return fmt.Errorf("diagram_boundary_replacements[%d].participant_boundaries[%d] requires participant and status=unproven", i, j)
			}
			if seenParticipants[participant] {
				return fmt.Errorf("diagram_boundary_replacements[%d] duplicates participant=%q", i, participant)
			}
			seenParticipants[participant] = true
		}
		block.ParticipantBoundaries = append([]types.DiagramParticipantBoundary(nil), replacement.ParticipantBoundaries...)
		working[blockID] = block
	}
	for _, blockID := range order {
		patch.ReplaceBlocks = append(patch.ReplaceBlocks, working[blockID])
	}
	return nil
}

func cloneAtomicDiagramPatchBlock(in types.AnswerBlock) types.AnswerBlock {
	out := in
	out.EdgeAnchors = append([]types.DiagramEdgeAnchor(nil), in.EdgeAnchors...)
	if in.Diagram != nil {
		diagram := *in.Diagram
		out.Diagram = &diagram
	}
	return out
}

func applyOneModelAuthoredDiagramEdgeEdit(
	block *types.AnswerBlock,
	edit emitAnswerDiagramEdgeEdit,
	lease *types.AnswerDiagramRelationRepairLease,
) error {
	if block == nil || block.Diagram == nil {
		return fmt.Errorf("diagram carrier is unavailable")
	}
	action := strings.ToLower(strings.TrimSpace(edit.Action))
	if action != "relabel" && action != "remove" && action != "replace" && action != "add" {
		return fmt.Errorf("unsupported action %q", edit.Action)
	}
	occurrence := edit.Occurrence
	if occurrence == 0 {
		occurrence = 1
	}
	if occurrence < 1 {
		return fmt.Errorf("occurrence must be at least 1")
	}
	if edit.BodyOccurrence < 0 {
		return fmt.Errorf("body_occurrence must be at least 1")
	}
	if action == "add" {
		if edit.Match != nil {
			return fmt.Errorf("action=add must omit match")
		}
		if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
			return err
		}
		if occurrence != 1 || edit.BodyOccurrence != 0 {
			return fmt.Errorf("action=add does not accept occurrence or body_occurrence")
		}
		if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
			return fmt.Errorf("action=add requires edge.visible_label authored by the model")
		}
		for _, anchor := range block.EdgeAnchors {
			if atomicDiagramAnchorSameTuple(anchor, *edit.Edge) {
				return fmt.Errorf("action=add duplicates an existing exact anchor")
			}
		}
		line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
		if err != nil {
			return err
		}
		body := strings.TrimRight(block.Diagram.Body, "\n")
		block.Diagram.Body = body + "\n" + line + "\n"
		block.EdgeAnchors = append(block.EdgeAnchors, *edit.Edge)
		return nil
	}
	if edit.Match == nil {
		return fmt.Errorf("action=%s requires match", action)
	}
	if err := validateAtomicDiagramAnchor(edit.Match, "match"); err != nil {
		return err
	}
	anchorIndex, anchorPairOccurrence, anchorErr := findAtomicDiagramAnchor(block.EdgeAnchors, *edit.Match, occurrence)
	bodyOnly := false
	if anchorErr != nil {
		if action == "relabel" {
			return anchorErr
		}
		if !atomicDiagramBodyOnlyFailureAuthorized(lease, block.ID, *edit.Match) {
			return anchorErr
		}
		// The producer-owned retry delta names this exact failed visible
		// relation, while the failure itself is that no matching prior
		// anchor exists. Another relation may legitimately share the same
		// visible endpoint pair (for example a grounded return beside an
		// unanchored call), so pair-level anchor presence must not veto this
		// body-only lane. body_occurrence remains mandatory whenever the
		// visible pair is ambiguous, and unrelated anchors stay untouched.
		// The model still chooses remove/replace and every replacement
		// field; the compiler merely makes that declared operation
		// executable against the previous model-authored Mermaid AST.
		bodyOnly = true
	}
	anchorWithoutBody := anchorErr == nil &&
		(action == "remove" || action == "replace") &&
		atomicDiagramAnchorWithoutBodyFailureAuthorized(lease, block.ID, block.EdgeAnchors[anchorIndex])
	if anchorWithoutBody {
		if edit.BodyOccurrence != 0 {
			return fmt.Errorf("body_occurrence must be omitted for an anchor with no visible Mermaid edge")
		}
		switch action {
		case "remove":
			if edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
				return fmt.Errorf("action=remove must omit edge and visible_label")
			}
			block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
		case "replace":
			if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
				return err
			}
			if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
				return fmt.Errorf("action=replace requires edge.visible_label authored by the model")
			}
			line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
			if err != nil {
				return err
			}
			body := strings.TrimRight(block.Diagram.Body, "\n")
			block.Diagram.Body = body + "\n" + line + "\n"
			block.EdgeAnchors[anchorIndex] = *edit.Edge
		}
		return nil
	}
	bodyOccurrence, err := atomicDiagramBodyOccurrence(
		block.Diagram.Body, block.EdgeAnchors, edit.Match.FromNode, edit.Match.ToNode,
		anchorPairOccurrence, edit.BodyOccurrence, bodyOnly,
	)
	if err != nil {
		return err
	}
	lineIndex, err := findAtomicMermaidEdgeLine(block.Diagram.Body, edit.Match.FromNode, edit.Match.ToNode, bodyOccurrence)
	if err != nil {
		return err
	}
	lines := strings.Split(block.Diagram.Body, "\n")
	switch action {
	case "remove":
		if edit.Edge != nil || strings.TrimSpace(edit.VisibleLabel) != "" {
			return fmt.Errorf("action=remove must omit edge and visible_label")
		}
		lines = append(lines[:lineIndex], lines[lineIndex+1:]...)
		if !bodyOnly {
			block.EdgeAnchors = append(block.EdgeAnchors[:anchorIndex], block.EdgeAnchors[anchorIndex+1:]...)
		}
	case "relabel":
		if edit.Edge != nil {
			return fmt.Errorf("action=relabel must omit edge")
		}
		label := strings.TrimSpace(edit.VisibleLabel)
		if label == "" || strings.ContainsAny(label, "\r\n") {
			return fmt.Errorf("action=relabel requires one non-empty single-line visible_label")
		}
		updated, err := relabelAtomicMermaidEdgeLine(block.Diagram.Body, lines[lineIndex], label)
		if err != nil {
			return err
		}
		lines[lineIndex] = updated
		block.EdgeAnchors[anchorIndex].VisibleLabel = label
	case "replace":
		if err := validateAtomicDiagramAnchor(edit.Edge, "edge"); err != nil {
			return err
		}
		if strings.TrimSpace(edit.Edge.VisibleLabel) == "" {
			return fmt.Errorf("action=replace requires edge.visible_label authored by the model")
		}
		line, err := renderAtomicMermaidEdgeLine(block.Diagram.Body, *edit.Edge)
		if err != nil {
			return err
		}
		indent := lines[lineIndex][:len(lines[lineIndex])-len(strings.TrimLeft(lines[lineIndex], " \t"))]
		lines[lineIndex] = indent + strings.TrimLeft(line, " \t")
		if bodyOnly {
			block.EdgeAnchors = append(block.EdgeAnchors, *edit.Edge)
		} else {
			block.EdgeAnchors[anchorIndex] = *edit.Edge
		}
	}
	block.Diagram.Body = strings.Join(lines, "\n")
	return nil
}

// atomicDiagramAnchorWithoutBodyFailureAuthorized admits only the exact
// producer-owned stale-anchor failure from the immediately preceding repair
// lease. The model still chooses whether to remove that anchor or replace it
// with a complete visible edge. No Mermaid label, request text, reasoning, or
// answer prose participates in this permission.
func atomicDiagramAnchorWithoutBodyFailureAuthorized(
	lease *types.AnswerDiagramRelationRepairLease,
	blockID string,
	anchor types.DiagramEdgeAnchor,
) bool {
	if lease == nil || lease.Version != 1 || strings.TrimSpace(blockID) == "" {
		return false
	}
	for _, failure := range lease.Failures {
		if failure.Issue != diagramCallEdgeIssueAnchorWithoutBodyEdge ||
			strings.TrimSpace(failure.BlockID) != strings.TrimSpace(blockID) {
			continue
		}
		if failure.RelationKind.IsValid() && failure.RelationKind != anchor.RelationKind {
			continue
		}
		if strings.TrimSpace(failure.FromNode) != "" || strings.TrimSpace(failure.ToNode) != "" {
			if strings.TrimSpace(failure.FromNode) != strings.TrimSpace(anchor.FromNode) ||
				strings.TrimSpace(failure.ToNode) != strings.TrimSpace(anchor.ToNode) {
				continue
			}
		}
		if strings.TrimSpace(failure.FromIdentity) != "" || strings.TrimSpace(failure.ToIdentity) != "" {
			if strings.TrimSpace(failure.FromIdentity) != strings.TrimSpace(anchor.FromIdentity) ||
				strings.TrimSpace(failure.ToIdentity) != strings.TrimSpace(anchor.ToIdentity) {
				continue
			}
		}
		return true
	}
	return false
}

func atomicDiagramBodyOccurrence(
	body string,
	anchors []types.DiagramEdgeAnchor,
	fromNode, toNode string,
	anchorPairOccurrence, declaredBodyOccurrence int,
	bodyOnly bool,
) (int, error) {
	bodyCount := atomicDiagramBodyPairCount(body, fromNode, toNode)
	if bodyCount == 0 {
		return 0, fmt.Errorf("Mermaid body has no matching edge for %s->%s", fromNode, toNode)
	}
	if declaredBodyOccurrence > 0 {
		if declaredBodyOccurrence > bodyCount {
			return 0, fmt.Errorf("body_occurrence=%d exceeds %d visible edge(s) for %s->%s", declaredBodyOccurrence, bodyCount, fromNode, toNode)
		}
		return declaredBodyOccurrence, nil
	}
	if bodyCount == 1 {
		return 1, nil
	}
	if !bodyOnly {
		anchorCount := atomicDiagramAnchorPairCount(anchors, fromNode, toNode)
		if anchorCount == bodyCount && anchorPairOccurrence > 0 && anchorPairOccurrence <= bodyCount {
			return anchorPairOccurrence, nil
		}
	}
	return 0, fmt.Errorf(
		"Mermaid body has %d edges for %s->%s that do not map one-to-one to prior anchors; set body_occurrence explicitly",
		bodyCount, fromNode, toNode,
	)
}

func atomicDiagramBodyPairCount(body, fromNode, toNode string) int {
	fromNode, toNode = strings.TrimSpace(fromNode), strings.TrimSpace(toNode)
	count := 0
	for _, edge := range mermaidcompat.ParseEdges(body) {
		if strings.TrimSpace(edge.From) == fromNode && strings.TrimSpace(edge.To) == toNode {
			count++
		}
	}
	return count
}

func atomicDiagramAnchorPairCount(anchors []types.DiagramEdgeAnchor, fromNode, toNode string) int {
	fromNode, toNode = strings.TrimSpace(fromNode), strings.TrimSpace(toNode)
	count := 0
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor.FromNode) == fromNode && strings.TrimSpace(anchor.ToNode) == toNode {
			count++
		}
	}
	return count
}

func atomicDiagramBodyOnlyFailureAuthorized(
	lease *types.AnswerDiagramRelationRepairLease,
	blockID string,
	match types.DiagramEdgeAnchor,
) bool {
	if lease == nil || lease.Version != 1 || strings.TrimSpace(blockID) == "" {
		return false
	}
	for _, failure := range lease.Failures {
		if failure.Issue != diagramCallEdgeIssueMissingAnchor && failure.Issue != diagramCallEdgeIssueMissingRelationAnchor {
			continue
		}
		if strings.TrimSpace(failure.BlockID) != strings.TrimSpace(blockID) ||
			strings.TrimSpace(failure.FromNode) != strings.TrimSpace(match.FromNode) ||
			strings.TrimSpace(failure.ToNode) != strings.TrimSpace(match.ToNode) {
			continue
		}
		if failure.RelationKind.IsValid() && failure.RelationKind != match.RelationKind {
			continue
		}
		return true
	}
	return false
}

func validateAtomicDiagramAnchor(anchor *types.DiagramEdgeAnchor, field string) error {
	if anchor == nil {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(anchor.FromNode) == "" || strings.TrimSpace(anchor.ToNode) == "" || !anchor.RelationKind.IsValid() {
		return fmt.Errorf("%s requires from_node, to_node, and a valid relation_kind", field)
	}
	if strings.ContainsAny(anchor.FromNode+anchor.ToNode+anchor.VisibleLabel, "\r\n") {
		return fmt.Errorf("%s fields must be single-line", field)
	}
	return nil
}

func atomicDiagramAnchorSameTuple(left, right types.DiagramEdgeAnchor) bool {
	return strings.TrimSpace(left.FromNode) == strings.TrimSpace(right.FromNode) &&
		strings.TrimSpace(left.ToNode) == strings.TrimSpace(right.ToNode) &&
		strings.TrimSpace(left.FromIdentity) == strings.TrimSpace(right.FromIdentity) &&
		strings.TrimSpace(left.ToIdentity) == strings.TrimSpace(right.ToIdentity) &&
		left.RelationKind == right.RelationKind
}

func findAtomicDiagramAnchor(anchors []types.DiagramEdgeAnchor, match types.DiagramEdgeAnchor, occurrence int) (int, int, error) {
	exactSeen := 0
	pairSeen := 0
	for i, anchor := range anchors {
		if strings.TrimSpace(anchor.FromNode) == strings.TrimSpace(match.FromNode) &&
			strings.TrimSpace(anchor.ToNode) == strings.TrimSpace(match.ToNode) {
			pairSeen++
		}
		if !atomicDiagramAnchorSameTuple(anchor, match) {
			continue
		}
		exactSeen++
		if exactSeen == occurrence {
			return i, pairSeen, nil
		}
	}
	return -1, 0, fmt.Errorf("match did not select occurrence %d of an exact prior anchor", occurrence)
}

func atomicDiagramHeader(body string) string {
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		return strings.ToLower(strings.Fields(line)[0])
	}
	return ""
}

func findAtomicMermaidEdgeLine(body, fromNode, toNode string, occurrence int) (int, error) {
	header := atomicDiagramHeader(body)
	if header == "" {
		return -1, fmt.Errorf("diagram body has no syntax header")
	}
	seen := 0
	lines := strings.Split(body, "\n")
	for i, raw := range lines {
		parsed := mermaidcompat.ParseEdges(header + "\n" + strings.TrimSpace(raw))
		for _, edge := range parsed {
			if strings.TrimSpace(edge.From) == strings.TrimSpace(fromNode) && strings.TrimSpace(edge.To) == strings.TrimSpace(toNode) {
				seen++
				if seen == occurrence {
					if len(parsed) != 1 {
						return -1, fmt.Errorf("matched Mermaid statement contains multiple edges; split it before an atomic edit")
					}
					return i, nil
				}
			}
		}
	}
	return -1, fmt.Errorf("Mermaid body has no matching edge occurrence %d for %s->%s", occurrence, fromNode, toNode)
}

func renderAtomicMermaidEdgeLine(body string, edge types.DiagramEdgeAnchor) (string, error) {
	label := strings.TrimSpace(edge.VisibleLabel)
	if label == "" || strings.ContainsAny(label, "\r\n") {
		return "", fmt.Errorf("edge.visible_label must be one non-empty line")
	}
	fromNode, toNode := strings.TrimSpace(edge.FromNode), strings.TrimSpace(edge.ToNode)
	switch atomicDiagramHeader(body) {
	case "sequencediagram":
		op := "->>"
		if edge.RelationKind == types.DiagramRelReturn {
			op = "-->>"
		}
		return "    " + fromNode + op + toNode + ": " + label, nil
	case "classdiagram":
		return "    " + fromNode + " --> " + toNode + " : " + label, nil
	case "flowchart", "graph":
		if strings.Contains(label, "|") {
			return "", fmt.Errorf("flowchart visible_label may not contain an unescaped pipe")
		}
		return "    " + fromNode + " -->|" + label + "| " + toNode, nil
	default:
		return "", fmt.Errorf("unsupported Mermaid body family for atomic edit")
	}
}

func relabelAtomicMermaidEdgeLine(body, rawLine, label string) (string, error) {
	header := atomicDiagramHeader(body)
	indent := rawLine[:len(rawLine)-len(strings.TrimLeft(rawLine, " \t"))]
	line := strings.TrimSpace(rawLine)
	switch header {
	case "sequencediagram":
		arrowAt, operator := mermaidcompat.FindSequenceArrow(line)
		if arrowAt < 0 {
			return "", fmt.Errorf("matched sequence statement has no arrow")
		}
		colon := atomicSequenceMessageColon(line, arrowAt+len(operator))
		if colon < 0 {
			return "", fmt.Errorf("matched sequence statement has no message label")
		}
		return indent + strings.TrimSpace(line[:colon]) + ": " + label, nil
	case "classdiagram":
		if at := strings.LastIndex(line, " : "); at >= 0 {
			return indent + strings.TrimSpace(line[:at]) + " : " + label, nil
		}
		return indent + line + " : " + label, nil
	case "flowchart", "graph":
		if strings.Contains(label, "|") {
			return "", fmt.Errorf("flowchart visible_label may not contain an unescaped pipe")
		}
		arrowAt, operator := mermaidcompat.FindFlowchartArrow(line)
		if arrowAt < 0 {
			return "", fmt.Errorf("matched flowchart statement has no arrow")
		}
		tailAt := arrowAt + len(operator)
		tail := line[tailAt:]
		trimmedTail := strings.TrimLeft(tail, " \t")
		spaceLen := len(tail) - len(trimmedTail)
		if strings.HasPrefix(trimmedTail, "|") {
			if end := strings.Index(trimmedTail[1:], "|"); end >= 0 {
				end++
				return indent + line[:tailAt] + tail[:spaceLen] + "|" + label + "|" + trimmedTail[end+1:], nil
			}
			return "", fmt.Errorf("matched flowchart statement has an unterminated label")
		}
		return indent + line[:tailAt] + "|" + label + "| " + strings.TrimSpace(tail), nil
	default:
		return "", fmt.Errorf("unsupported Mermaid body family for atomic relabel")
	}
}

func atomicSequenceMessageColon(line string, start int) int {
	for i := start; i < len(line); i++ {
		if line[i] != ':' {
			continue
		}
		if (i > 0 && line[i-1] == ':') || (i+1 < len(line) && line[i+1] == ':') {
			continue
		}
		return i
	}
	return -1
}
