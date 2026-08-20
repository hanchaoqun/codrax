package types

import (
	"fmt"
	"sort"
	"strings"
)

// AnswerDiagramRelationRepairFailure is the producer-owned tuple for one
// source-diagram relation that failed the typed authority check. It contains
// structural fields only: request text, reasoning prose, answer prose, and
// Mermaid labels are deliberately outside this contract.
type AnswerDiagramRelationRepairFailure struct {
	BlockID      string              `json:"block_id,omitempty"`
	Issue        string              `json:"issue"`
	RelationKind DiagramRelationKind `json:"relation_kind,omitempty"`
	FromNode     string              `json:"from_node"`
	ToNode       string              `json:"to_node"`
	FromIdentity string              `json:"from_identity,omitempty"`
	ToIdentity   string              `json:"to_identity,omitempty"`
}

// AnswerDiagramRelationRepairLeaseBlock snapshots the structured relation
// topology of one block at the start of a bounded repair turn. Visible labels
// are retained in the snapshot for audit, but lease comparison intentionally
// excludes VisibleLabel so reader-facing wording remains model-owned.
type AnswerDiagramRelationRepairLeaseBlock struct {
	BlockID     string              `json:"block_id"`
	BaseAnchors []DiagramEdgeAnchor `json:"base_anchors,omitempty"`
}

// AnswerDiagramRelationRepairLease prevents a local relation repair from
// silently becoming a whole-graph rewrite. The model may remove a named failed
// relation or replace it on the same endpoint pair; every unlisted structured
// relation remains unchanged. The lease never authors, deletes, relabels, or
// reconnects an edge itself.
type AnswerDiagramRelationRepairLease struct {
	Version  int                                     `json:"version"`
	Failures []AnswerDiagramRelationRepairFailure    `json:"failures"`
	Blocks   []AnswerDiagramRelationRepairLeaseBlock `json:"blocks"`
}

// AnswerDiagramRelationRepairScopeViolation is a compact typed explanation of
// a patch that escaped its local relation-repair scope.
type AnswerDiagramRelationRepairScopeViolation struct {
	BlockID  string `json:"block_id"`
	Issue    string `json:"issue"`
	FromNode string `json:"from_node,omitempty"`
	ToNode   string `json:"to_node,omitempty"`
}

// NewAnswerDiagramRelationRepairLease freezes the precise graph carrier that
// the next patch is allowed to repair. Empty/invalid failures produce nil so a
// malformed diagnostic can never create a hard gate.
func NewAnswerDiagramRelationRepairLease(base *AnswerDocumentV2, failures []AnswerDiagramRelationRepairFailure) *AnswerDiagramRelationRepairLease {
	if base == nil || len(failures) == 0 {
		return nil
	}
	clean := make([]AnswerDiagramRelationRepairFailure, 0, len(failures))
	targetBlocks := make(map[string]bool)
	for _, failure := range failures {
		failure.BlockID = strings.TrimSpace(failure.BlockID)
		failure.Issue = strings.TrimSpace(failure.Issue)
		failure.FromNode = strings.TrimSpace(failure.FromNode)
		failure.ToNode = strings.TrimSpace(failure.ToNode)
		failure.FromIdentity = strings.TrimSpace(failure.FromIdentity)
		failure.ToIdentity = strings.TrimSpace(failure.ToIdentity)
		if failure.BlockID == "" || failure.Issue == "" || failure.FromNode == "" || failure.ToNode == "" {
			continue
		}
		clean = append(clean, failure)
		targetBlocks[failure.BlockID] = true
	}
	if len(clean) == 0 {
		return nil
	}
	blocks := make([]AnswerDiagramRelationRepairLeaseBlock, 0, len(base.Blocks))
	for _, block := range base.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" || (len(block.EdgeAnchors) == 0 && !targetBlocks[id]) {
			continue
		}
		blocks = append(blocks, AnswerDiagramRelationRepairLeaseBlock{
			BlockID:     id,
			BaseAnchors: append([]DiagramEdgeAnchor(nil), block.EdgeAnchors...),
		})
	}
	return &AnswerDiagramRelationRepairLease{Version: 1, Failures: clean, Blocks: blocks}
}

// ValidateAnswerDiagramRelationRepairLease checks only typed edge-anchor
// topology. It does not inspect Mermaid source, node/edge labels, request text,
// reasoning, or final prose. Ordinary diagram evidence validation remains the
// authority for whether a corrected relation is true.
func ValidateAnswerDiagramRelationRepairLease(lease *AnswerDiagramRelationRepairLease, merged *AnswerDocumentV2) []AnswerDiagramRelationRepairScopeViolation {
	if lease == nil || lease.Version != 1 || merged == nil || len(lease.Failures) == 0 {
		return nil
	}
	resultBlocks := make(map[string][]DiagramEdgeAnchor, len(merged.Blocks))
	for _, block := range merged.Blocks {
		id := strings.TrimSpace(block.ID)
		if id != "" {
			resultBlocks[id] = append(resultBlocks[id], block.EdgeAnchors...)
		}
	}
	baseBlocks := make(map[string][]DiagramEdgeAnchor, len(lease.Blocks))
	for _, block := range lease.Blocks {
		id := strings.TrimSpace(block.BlockID)
		if id != "" {
			baseBlocks[id] = append(baseBlocks[id], block.BaseAnchors...)
		}
	}

	blockIDs := make(map[string]bool, len(baseBlocks)+len(resultBlocks))
	for id := range baseBlocks {
		blockIDs[id] = true
	}
	for id, anchors := range resultBlocks {
		if len(anchors) > 0 {
			blockIDs[id] = true
		}
	}
	orderedIDs := make([]string, 0, len(blockIDs))
	for id := range blockIDs {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Strings(orderedIDs)

	var violations []AnswerDiagramRelationRepairScopeViolation
	for _, blockID := range orderedIDs {
		base := baseBlocks[blockID]
		result := resultBlocks[blockID]
		baseCounts := answerDiagramRelationAnchorCounts(base)
		resultCounts := answerDiagramRelationAnchorCounts(result)

		removedFailedBudget := 0
		missingBaseFailureBudget := 0
		countedRemovedKeys := make(map[string]bool)
		countedMissingFailures := make(map[string]bool)
		for _, failure := range lease.Failures {
			if failure.BlockID != blockID {
				continue
			}
			matchedBase := false
			for _, anchor := range base {
				if answerDiagramRelationFailureMatchesAnchor(failure, anchor) {
					matchedBase = true
					key := answerDiagramRelationAnchorSemanticKey(anchor)
					if !countedRemovedKeys[key] && resultCounts[key] < baseCounts[key] {
						removedFailedBudget += baseCounts[key] - resultCounts[key]
						countedRemovedKeys[key] = true
					}
					break
				}
			}
			missingKey := answerDiagramRelationFailurePairKey(failure)
			if !matchedBase && !countedMissingFailures[missingKey] {
				missingBaseFailureBudget++
				countedMissingFailures[missingKey] = true
			}
		}

		// Every removed base relation must be explicitly named by failures[].
		for key, count := range baseCounts {
			missing := count - resultCounts[key]
			if missing <= 0 {
				continue
			}
			anchor, ok := answerDiagramRelationAnchorByKey(base, key)
			if !ok || answerDiagramRelationAnchorMatchesAnyFailure(blockID, anchor, lease.Failures) {
				continue
			}
			violations = append(violations, AnswerDiagramRelationRepairScopeViolation{
				BlockID: blockID, Issue: "unlisted_relation_removed",
				FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
			})
		}

		newBudget := removedFailedBudget + missingBaseFailureBudget
		newUsed := 0
		for key, count := range resultCounts {
			extra := count - baseCounts[key]
			if extra <= 0 {
				continue
			}
			anchor, ok := answerDiagramRelationAnchorByKey(result, key)
			for i := 0; ok && i < extra; i++ {
				if !answerDiagramRelationAnchorMatchesAnyFailure(blockID, anchor, lease.Failures) {
					violations = append(violations, AnswerDiagramRelationRepairScopeViolation{
						BlockID: blockID, Issue: "unlisted_relation_added",
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					})
					continue
				}
				newUsed++
				if newUsed > newBudget {
					violations = append(violations, AnswerDiagramRelationRepairScopeViolation{
						BlockID: blockID, Issue: "failed_relation_expanded",
						FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
					})
				}
			}
		}
	}
	return answerDiagramRelationRepairUniqueViolations(violations, 8)
}

func answerDiagramRelationAnchorSemanticKey(anchor DiagramEdgeAnchor) string {
	return strings.Join([]string{
		strings.TrimSpace(anchor.FromNode), strings.TrimSpace(anchor.ToNode),
		strings.TrimSpace(anchor.FromIdentity), strings.TrimSpace(anchor.ToIdentity),
		strings.TrimSpace(string(anchor.RelationKind)),
	}, "\x00")
}

func answerDiagramRelationAnchorCounts(anchors []DiagramEdgeAnchor) map[string]int {
	out := make(map[string]int, len(anchors))
	for _, anchor := range anchors {
		out[answerDiagramRelationAnchorSemanticKey(anchor)]++
	}
	return out
}

func answerDiagramRelationAnchorByKey(anchors []DiagramEdgeAnchor, key string) (DiagramEdgeAnchor, bool) {
	for _, anchor := range anchors {
		if answerDiagramRelationAnchorSemanticKey(anchor) == key {
			return anchor, true
		}
	}
	return DiagramEdgeAnchor{}, false
}

func answerDiagramRelationFailureMatchesAnchor(failure AnswerDiagramRelationRepairFailure, anchor DiagramEdgeAnchor) bool {
	if answerDiagramRelationSameUnorderedPair(failure.FromNode, failure.ToNode, anchor.FromNode, anchor.ToNode) {
		return true
	}
	return failure.FromIdentity != "" && failure.ToIdentity != "" &&
		strings.TrimSpace(anchor.FromIdentity) != "" && strings.TrimSpace(anchor.ToIdentity) != "" &&
		answerDiagramRelationSameUnorderedPair(failure.FromIdentity, failure.ToIdentity, anchor.FromIdentity, anchor.ToIdentity)
}

func answerDiagramRelationAnchorMatchesAnyFailure(blockID string, anchor DiagramEdgeAnchor, failures []AnswerDiagramRelationRepairFailure) bool {
	for _, failure := range failures {
		if failure.BlockID == blockID && answerDiagramRelationFailureMatchesAnchor(failure, anchor) {
			return true
		}
	}
	return false
}

func answerDiagramRelationSameUnorderedPair(aFrom, aTo, bFrom, bTo string) bool {
	aFrom, aTo = strings.TrimSpace(aFrom), strings.TrimSpace(aTo)
	bFrom, bTo = strings.TrimSpace(bFrom), strings.TrimSpace(bTo)
	return (aFrom == bFrom && aTo == bTo) || (aFrom == bTo && aTo == bFrom)
}

func answerDiagramRelationFailurePairKey(failure AnswerDiagramRelationRepairFailure) string {
	left, right := strings.TrimSpace(failure.FromNode), strings.TrimSpace(failure.ToNode)
	if right < left {
		left, right = right, left
	}
	identityLeft, identityRight := strings.TrimSpace(failure.FromIdentity), strings.TrimSpace(failure.ToIdentity)
	if identityRight < identityLeft {
		identityLeft, identityRight = identityRight, identityLeft
	}
	return strings.Join([]string{failure.BlockID, left, right, identityLeft, identityRight}, "\x00")
}

func answerDiagramRelationRepairUniqueViolations(in []AnswerDiagramRelationRepairScopeViolation, capN int) []AnswerDiagramRelationRepairScopeViolation {
	seen := make(map[string]bool, len(in))
	out := make([]AnswerDiagramRelationRepairScopeViolation, 0, len(in))
	for _, violation := range in {
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s", violation.BlockID, violation.Issue, violation.FromNode, violation.ToNode)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, violation)
		if capN > 0 && len(out) >= capN {
			break
		}
	}
	return out
}

func cloneAnswerDiagramRelationRepairLease(in *AnswerDiagramRelationRepairLease) *AnswerDiagramRelationRepairLease {
	if in == nil {
		return nil
	}
	out := &AnswerDiagramRelationRepairLease{Version: in.Version}
	out.Failures = append([]AnswerDiagramRelationRepairFailure(nil), in.Failures...)
	if len(in.Blocks) > 0 {
		out.Blocks = make([]AnswerDiagramRelationRepairLeaseBlock, len(in.Blocks))
		for i, block := range in.Blocks {
			out.Blocks[i] = AnswerDiagramRelationRepairLeaseBlock{
				BlockID: block.BlockID, BaseAnchors: append([]DiagramEdgeAnchor(nil), block.BaseAnchors...),
			}
		}
	}
	return out
}
