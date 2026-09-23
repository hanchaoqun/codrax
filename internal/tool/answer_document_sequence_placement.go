package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hanchaoqun/codrax/internal/mermaidcompat"
	"github.com/hanchaoqun/codrax/internal/types"
)

const sequencePlacementTeaching = "A new sequence message requires one placement_ref from the current sequence_placement_choices. This chooses an exact gap in the current diagram, including its existing branch; it proves no relation or timing. Multiple messages choosing the same gap follow their order in diagram_edge_edits. Existing-message replacement stays in place and omits placement_ref. Missing or stale positions never fall back to the end."

// These positions identify source gaps, not relations or semantic events.
// The model chooses a position independently of its allowed relation tuple.
type sequenceInsertionPosition struct {
	Ref     string `json:"placement_ref"`
	BlockID string `json:"block_id"`
	Before  string `json:"before"`
	After   string `json:"after"`
	line    int
}

func sequenceInsertionPositions(block types.AnswerBlock) []sequenceInsertionPosition {
	if block.Diagram == nil || atomicDiagramHeader(block.Diagram.Body) != "sequencediagram" {
		return nil
	}
	lines := strings.Split(block.Diagram.Body, "\n")
	fingerprint := types.AnswerDiagramParticipantVisibilityFingerprint(block)
	if fingerprint == "" {
		return nil
	}
	var out []sequenceInsertionPosition
	headerSeen, declarationDepth := false, 0
	inParticipantBox := false
	last := ""
	appendPosition := func(line int, before string) {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", fingerprint, line)))
		out = append(out, sequenceInsertionPosition{Ref: fmt.Sprintf("sp1-%x", sum[:12]), BlockID: block.ID, Before: before, After: last, line: line})
	}
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !headerSeen {
			headerSeen = true
			last = line
			continue
		}
		// Extended participant JSON is one declaration, even when it spans
		// physical lines. Never publish a gap inside that lexical statement.
		if declarationDepth > 0 {
			declarationDepth += sequenceDeclarationBraceDelta(line)
			last = line
			continue
		}
		if len(mermaidcompat.SequenceParticipantDeclarations(line)) > 0 || strings.HasPrefix(line, "participant ") || strings.HasPrefix(line, "actor ") {
			if at := strings.Index(line, "@{"); at >= 0 {
				declarationDepth = sequenceDeclarationBraceDelta(line[at+1:])
			}
			last = line
			continue
		}
		// A participant box contains declarations, not sequence messages.
		// Read source tokens only; a participant named box may still send a
		// message and must not open a declaration region.
		if inParticipantBox {
			if strings.Fields(line)[0] == "end" {
				inParticipantBox = false
			}
			last = line
			continue
		}
		if strings.Fields(line)[0] == "box" && len(mermaidcompat.ParseEdges("sequenceDiagram\n"+line)) == 0 {
			inParticipantBox = true
			last = line
			continue
		}
		appendPosition(i, line)
		last = line
	}
	if declarationDepth == 0 && !inParticipantBox {
		end := len(lines)
		if lines[len(lines)-1] == "" {
			end--
		}
		appendPosition(end, "")
	}
	return out
}

func sequenceDeclarationBraceDelta(line string) int {
	depth := 0
	quoted, escaped := false, false
	for _, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if quoted && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		if r == '{' {
			depth++
		}
		if r == '}' {
			depth--
		}
	}
	return depth
}

func atomicSequenceEditCreatesStatement(block types.AnswerBlock, edit emitAnswerDiagramEdgeEdit, leases ...*types.AnswerDiagramRelationRepairLease) bool {
	if block.Diagram == nil || atomicDiagramHeader(block.Diagram.Body) != "sequencediagram" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(edit.Action)) {
	case "add":
		return !edit.metadataAttach
	case "replace":
		if edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierStaleAnchor {
			return true
		}
		if edit.Match == nil {
			return false
		}
		if len(leases) > 0 && atomicDiagramAnchorWithoutBodyFailureAuthorized(leases[0], block.ID, *edit.Match) {
			return true
		}
		return atomicDiagramBodyPairCount(block.Diagram.Body, edit.Match.FromNode, edit.Match.ToNode) == 0
	default:
		return false
	}
}

// A private marker pins each chosen gap while the existing source-local
// transaction removes/replaces other statements. It is a comment understood
// by the shared syntax parser, never a persisted carrier. One marker per edit
// (in original request order) preserves same-gap ordering even if metadata
// removals must execute in reverse occurrence order.
type sequencePlacementPlan struct {
	positions map[string][]sequencePlannedInsertion
}
type sequencePlannedInsertion struct {
	line, order int
	marker      string
	statement   *string
}

func newSequencePlacementPlan(previous map[string]types.AnswerBlock, edits []resolvedAtomicDiagramEdgeEdit, lease *types.AnswerDiagramRelationRepairLease) (*sequencePlacementPlan, error) {
	plan := &sequencePlacementPlan{positions: make(map[string][]sequencePlannedInsertion)}
	for i := range edits {
		edit := &edits[i].edit
		block, exists := previous[edit.BlockID]
		if !exists {
			continue
		} // the existing exact block gate owns this error
		creates := atomicSequenceEditCreatesStatement(block, *edit, lease)
		if !creates {
			if strings.TrimSpace(edit.PlacementRef) != "" {
				return nil, fmt.Errorf("diagram_edge_edits[%d]: placement_ref is only for a new sequence statement", edits[i].originalIndex)
			}
			continue
		}
		ref := strings.TrimSpace(edit.PlacementRef)
		var selected *sequenceInsertionPosition
		for _, position := range sequenceInsertionPositions(block) {
			if position.Ref == ref {
				copyPosition := position
				selected = &copyPosition
				break
			}
		}
		if selected == nil {
			return nil, fmt.Errorf("diagram_edge_edits[%d]: new sequence statement requires one current placement_ref for block %q; missing, stale, or cross-block positions never append at the end", edits[i].originalIndex, block.ID)
		}
		marker := fmt.Sprintf("%%%% codrax-private-placement:%s:%d", ref, edits[i].originalIndex)
		for strings.Contains(block.Diagram.Body, marker) {
			marker += ":x"
		}
		edit.placementMarker = marker
		edit.placementStatement = new(string)
		plan.positions[block.ID] = append(plan.positions[block.ID], sequencePlannedInsertion{selected.line, edits[i].originalIndex, marker, edit.placementStatement})
	}
	for i := range edits {
		edit := &edits[i].edit
		if len(plan.positions[edit.BlockID]) == 0 || edit.placementMarker != "" || edit.Match == nil || edit.BodyOccurrence != 0 ||
			edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierPriorAnchorMetadata ||
			edit.failureRefCarrier == types.AnswerDiagramRelationRepairCarrierStaleAnchor ||
			atomicDiagramAnchorWithoutBodyFailureAuthorized(lease, edit.BlockID, *edit.Match) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(edit.Action)) {
		case "remove", "replace", "relabel":
		default:
			continue
		}
		base := previous[edit.BlockID]
		_, pairOccurrence, err := findAtomicDiagramEditAnchor(base.EdgeAnchors, *edit)
		if err != nil {
			continue // Existing body-only authorization owns non-anchor targets.
		}
		// Legacy omitted body_occurrence is legal only for the existing exact
		// single-edge / one-to-one anchor mapping. Freeze that baseline mapping
		// before a deferred addition appends metadata but not its body.
		edit.BodyOccurrence, err = atomicDiagramBodyOccurrence(base.Diagram.Body, base.EdgeAnchors, edit.Match.FromNode, edit.Match.ToNode, pairOccurrence, 0, false)
		if err != nil {
			return nil, fmt.Errorf("diagram_edge_edits[%d]: %w", edits[i].originalIndex, err)
		}
	}
	return plan, nil
}

func (p *sequencePlacementPlan) prepare(block *types.AnswerBlock) {
	if p == nil || block.Diagram == nil || len(p.positions[block.ID]) == 0 {
		return
	}
	positions := append([]sequencePlannedInsertion(nil), p.positions[block.ID]...)
	sort.SliceStable(positions, func(i, j int) bool {
		if positions[i].line != positions[j].line {
			return positions[i].line < positions[j].line
		}
		return positions[i].order < positions[j].order
	})
	lines := strings.Split(block.Diagram.Body, "\n")
	var out []string
	next := 0
	for i := 0; i <= len(lines); i++ {
		for next < len(positions) && positions[next].line == i {
			out = append(out, positions[next].marker)
			next++
		}
		if i < len(lines) {
			out = append(out, lines[i])
		}
	}
	block.Diagram.Body = strings.Join(out, "\n")
}

func (p *sequencePlacementPlan) finish(working map[string]types.AnswerBlock) error {
	for id, positions := range p.positions {
		block, ok := working[id]
		if !ok || block.Diagram == nil {
			continue
		}
		markers := make(map[string]*string, len(positions))
		for _, position := range positions {
			if position.statement == nil || *position.statement == "" {
				return fmt.Errorf("selected sequence placement has no completed model-authored statement")
			}
			markers[position.marker] = position.statement
		}
		lines := strings.Split(block.Diagram.Body, "\n")
		out := lines[:0]
		for _, line := range lines {
			if statement, found := markers[line]; found {
				out = append(out, *statement)
				delete(markers, line)
			} else {
				out = append(out, line)
			}
		}
		if len(markers) != 0 {
			return fmt.Errorf("selected sequence placement disappeared during the atomic transaction")
		}
		block.Diagram.Body = strings.Join(out, "\n")
		working[id] = block
	}
	return nil
}

func insertAtomicDiagramStatement(block *types.AnswerBlock, edit emitAnswerDiagramEdgeEdit, line string) error {
	if atomicDiagramHeader(block.Diagram.Body) != "sequencediagram" {
		block.Diagram.Body = strings.TrimRight(block.Diagram.Body, "\n") + "\n" + line + "\n"
		return nil
	}
	if edit.placementMarker == "" || edit.placementStatement == nil {
		return fmt.Errorf("new sequence statement requires a resolved current placement_ref")
	}
	lines := strings.Split(block.Diagram.Body, "\n")
	for _, value := range lines {
		if value == edit.placementMarker {
			// Body occurrences refer to the immutable base. Materializing a
			// same-pair message now could retarget a later removal/replacement.
			// Keep metadata ordering intact and publish all new bodies only
			// after every original-body edit has completed.
			*edit.placementStatement = line
			return nil
		}
	}
	return fmt.Errorf("selected sequence placement is unavailable in the current atomic transaction")
}

// Enrich the actual dispatch schema after lease narrowing. Selection refs are
// derived from the same immutable base as execution, not historical JSON.
func projectAnswerDocumentSequencePlacements(raw json.RawMessage, prev *types.AnswerDocumentV2, lease *types.AnswerDiagramRelationRepairLease) json.RawMessage {
	if prev == nil {
		return raw
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return raw
	}
	properties, _ := root["properties"].(map[string]any)
	edges, _ := properties["diagram_edge_edits"].(map[string]any)
	items, _ := edges["items"].(map[string]any)
	if items == nil {
		return raw
	}
	blocks := make(map[string]types.AnswerBlock)
	positions := make(map[string][]sequenceInsertionPosition)
	var roster []sequenceInsertionPosition
	for _, block := range prev.Blocks {
		blocks[block.ID] = block
		positions[block.ID] = sequenceInsertionPositions(block)
		roster = append(roster, positions[block.ID]...)
	}
	if len(roster) == 0 {
		if props, ok := items["properties"].(map[string]any); ok {
			delete(props, "placement_ref")
		}
		out, err := json.Marshal(root)
		if err != nil {
			return raw
		}
		return out
	}
	field := func(rows []sequenceInsertionPosition) map[string]any {
		refs := make([]any, 0, len(rows))
		for _, row := range rows {
			refs = append(refs, row.Ref)
		}
		return map[string]any{"type": "string", "enum": refs, "description": "Choose this block's exact current position from sequence_placement_choices."}
	}
	if branches, ok := items["oneOf"].([]any); ok {
		for _, rawBranch := range branches {
			branch, _ := rawBranch.(map[string]any)
			props, _ := branch["properties"].(map[string]any)
			readEnum := func(key string) string {
				schema, _ := props[key].(map[string]any)
				values, _ := schema["enum"].([]any)
				if len(values) != 1 {
					return ""
				}
				value, _ := values[0].(string)
				return value
			}
			edit := emitAnswerDiagramEdgeEdit{Action: readEnum("action"), AdditionRef: readEnum("addition_ref"), FailureRef: readEnum("failure_ref")}
			if edit.Action != "add" && edit.Action != "replace" {
				continue
			}
			// Ref resolution needs a model edge for actual add/replace; location
			// projection uses only the producer's block/carrier coordinates.
			if lease != nil {
				for _, candidate := range lease.AllowedAdditions {
					if candidate.AdditionRef == edit.AdditionRef {
						edit.BlockID = candidate.BlockID
					}
				}
				for _, failure := range lease.Failures {
					if failure.FailureRef == edit.FailureRef {
						edit.BlockID = failure.BlockID
						edit.failureRefCarrier = failure.TargetCarrier
						edit.Match = &types.DiagramEdgeAnchor{FromNode: failure.FromNode, ToNode: failure.ToNode}
					}
				}
			}
			if !atomicSequenceEditCreatesStatement(blocks[edit.BlockID], edit, lease) {
				continue
			}
			props["placement_ref"] = field(positions[edit.BlockID])
			required, _ := branch["required"].([]any)
			branch["required"] = append(required, "placement_ref")
		}
	} else if props, ok := items["properties"].(map[string]any); ok {
		props["placement_ref"] = field(roster)
	}
	encodedRoster, err := json.Marshal(roster)
	if err != nil {
		return raw
	}
	description, _ := edges["description"].(string)
	edges["description"] = description + " " + sequencePlacementTeaching + " sequence_placement_choices=" + string(encodedRoster)
	out, err := json.Marshal(root)
	if err != nil {
		return raw
	}
	return out
}
