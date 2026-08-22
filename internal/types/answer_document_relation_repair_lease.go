package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	AnswerDiagramRelationRepairDeltaVersion                 = 1
	AnswerDiagramRelationRepairDeltaMaxEntries              = 128
	AnswerDiagramRelationRepairDeltaMaxJSONBytes            = 64 * 1024
	AnswerDiagramRelationRepairOrdinaryValidationMaxEntries = 16
)

// AnswerDiagramRelationRepairFailure is the producer-owned tuple for one
// source-diagram relation that failed the typed authority check. It contains
// structural fields only: request text, reasoning prose, answer prose, and
// Mermaid labels are deliberately outside this contract.
type AnswerDiagramRelationRepairFailure struct {
	FailureRef     string                                   `json:"failure_ref,omitempty"`
	BlockID        string                                   `json:"block_id,omitempty"`
	Issue          string                                   `json:"issue"`
	RelationKind   DiagramRelationKind                      `json:"relation_kind,omitempty"`
	FromNode       string                                   `json:"from_node"`
	ToNode         string                                   `json:"to_node"`
	FromIdentity   string                                   `json:"from_identity,omitempty"`
	ToIdentity     string                                   `json:"to_identity,omitempty"`
	TargetCarrier  AnswerDiagramRelationRepairTargetCarrier `json:"target_carrier,omitempty"`
	AllowedActions []AnswerDiagramRelationRepairAction      `json:"allowed_actions,omitempty"`
	RelatedIssues  []string                                 `json:"related_issues,omitempty"`
	BodyOccurrence int                                      `json:"body_occurrence,omitempty"`
}

// AnswerDiagramRelationRepairTargetCarrier tells the retrying model which
// exact carrier the opaque failure ref selects. This is structural capability
// metadata derived from the rejected document, never a suggested repair.
type AnswerDiagramRelationRepairTargetCarrier string

const (
	AnswerDiagramRelationRepairCarrierUnknown     AnswerDiagramRelationRepairTargetCarrier = "unknown"
	AnswerDiagramRelationRepairCarrierPriorAnchor AnswerDiagramRelationRepairTargetCarrier = "prior_anchor"
	// PriorAnchorMetadata identifies one exact structured anchor whose visible
	// body occurrence cannot be selected without guessing among repeated
	// from/to messages. The only safe local capability is removing that anchor
	// metadata; visible edges remain model-owned and require their own exact
	// visible_body_edge failure refs.
	AnswerDiagramRelationRepairCarrierPriorAnchorMetadata AnswerDiagramRelationRepairTargetCarrier = "prior_anchor_metadata"
	AnswerDiagramRelationRepairCarrierVisibleBodyEdge     AnswerDiagramRelationRepairTargetCarrier = "visible_body_edge"
	AnswerDiagramRelationRepairCarrierStaleAnchor         AnswerDiagramRelationRepairTargetCarrier = "stale_anchor"
	AnswerDiagramRelationRepairCarrierLabelPair           AnswerDiagramRelationRepairTargetCarrier = "label_pair"
)

// AnswerDiagramRelationRepairAction is one atomic operation that the current
// carrier can execute. The model still chooses among these actions and authors
// replacement endpoints and reader-facing labels; an opaque ref may restore
// only the exact technical relation/identity tuple hidden by schema.
type AnswerDiagramRelationRepairAction string

const (
	AnswerDiagramRelationRepairActionRelabel AnswerDiagramRelationRepairAction = "relabel"
	AnswerDiagramRelationRepairActionRemove  AnswerDiagramRelationRepairAction = "remove"
	AnswerDiagramRelationRepairActionReplace AnswerDiagramRelationRepairAction = "replace"
	// Attach binds one model-selected typed addition candidate to one exact
	// model-authored visible-body occurrence. It is published only as a paired
	// failure_ref+addition_ref capability; neither ref can select the other.
	AnswerDiagramRelationRepairActionAttach AnswerDiagramRelationRepairAction = "attach"
)

func (f AnswerDiagramRelationRepairFailure) AllowsAction(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	for _, allowed := range f.AllowedActions {
		if action == string(allowed) {
			return true
		}
	}
	return false
}

// CanRemoveVisibleBodyOccurrence reports whether this exact live capability
// can delete one exact Mermaid body edge occurrence. Orphan cleanup is a
// stronger claim than "some failure on the same endpoint pair is removable":
// stale/prior-anchor-metadata removals change hidden metadata only, and one
// occurrence-agnostic ref must not be counted once for every repeated edge.
//
// Callers still own one-to-one ref consumption across a participant's incident
// edges. This method only checks one candidate occurrence and deliberately
// reads no request text, model prose, visible label, or relation semantics.
func (f AnswerDiagramRelationRepairFailure) CanRemoveVisibleBodyOccurrence(
	blockID, fromNode, toNode string,
	bodyOccurrence, samePairTotal int,
) bool {
	if bodyOccurrence < 1 || samePairTotal < bodyOccurrence ||
		strings.TrimSpace(f.FailureRef) == "" ||
		strings.TrimSpace(f.BlockID) != strings.TrimSpace(blockID) ||
		strings.TrimSpace(f.FromNode) != strings.TrimSpace(fromNode) ||
		strings.TrimSpace(f.ToNode) != strings.TrimSpace(toNode) ||
		!f.AllowsAction(string(AnswerDiagramRelationRepairActionRemove)) {
		return false
	}
	switch f.TargetCarrier {
	case AnswerDiagramRelationRepairCarrierVisibleBodyEdge,
		AnswerDiagramRelationRepairCarrierPriorAnchor:
		// These are the only two carrier classes whose remove executor also
		// removes a visible Mermaid occurrence. StaleAnchor and
		// PriorAnchorMetadata intentionally preserve the body.
	default:
		return false
	}
	if f.BodyOccurrence > 0 {
		return f.BodyOccurrence == bodyOccurrence
	}
	// A zero occurrence is unambiguous only when the rejected base contains
	// exactly one visible edge for this endpoint pair. Reusing it across a
	// duplicate pair would publish an orphan capability the executor cannot
	// fulfill with one opaque ref.
	return samePairTotal == 1
}

// AssignAnswerDiagramRelationRepairFailureRefs binds each structural failure
// to the exact diagram carrier that produced it. The opaque reference is only
// a retry selector: admission still requires membership in the live lease and
// all ordinary relation/evidence checks continue to run after the edit. A
// changed typed anchor snapshot therefore gets a different reference, while
// repeated validation of the same rejected draft remains stable. Mermaid
// source, visible labels, request text, reasoning, and answer prose are never
// read into this selector.
func AssignAnswerDiagramRelationRepairFailureRefs(
	base *AnswerDocumentV2,
	failures []AnswerDiagramRelationRepairFailure,
) []AnswerDiagramRelationRepairFailure {
	out := append([]AnswerDiagramRelationRepairFailure(nil), failures...)
	for i := range out {
		out[i].TargetCarrier, out[i].AllowedActions = answerDiagramRelationRepairFailureCapabilities(base, out[i])
		out[i].FailureRef = answerDiagramRelationRepairFailureRef(base, out[i])
	}
	return out
}

func answerDiagramRelationRepairFailureCapabilities(
	base *AnswerDocumentV2,
	failure AnswerDiagramRelationRepairFailure,
) (AnswerDiagramRelationRepairTargetCarrier, []AnswerDiagramRelationRepairAction) {
	if failure.TargetCarrier == AnswerDiagramRelationRepairCarrierPriorAnchorMetadata {
		if len(answerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure)) == 1 {
			return AnswerDiagramRelationRepairCarrierPriorAnchorMetadata, []AnswerDiagramRelationRepairAction{
				AnswerDiagramRelationRepairActionRemove,
			}
		}
		return AnswerDiagramRelationRepairCarrierUnknown, nil
	}
	issue := strings.TrimSpace(failure.Issue)
	switch issue {
	case "missing_call_anchor", DiagramRelationFailureMissingGroundedCallAnchor, "missing_relation_anchor":
		if strings.TrimSpace(failure.FromNode) != "" && strings.TrimSpace(failure.ToNode) != "" {
			actions := []AnswerDiagramRelationRepairAction{AnswerDiagramRelationRepairActionRemove}
			// A failure-ref replacement schema deliberately withholds hidden
			// relation/identity fields. Advertise replace only when this exact
			// producer row can restore those fields without guessing. An
			// untyped body edge may instead receive a separately selected typed
			// candidate through the paired attach capability installed below.
			if answerDiagramRelationRepairFailureHasReplacementTuple(failure) {
				actions = append(actions, AnswerDiagramRelationRepairActionReplace)
			}
			return AnswerDiagramRelationRepairCarrierVisibleBodyEdge, actions
		}
		return AnswerDiagramRelationRepairCarrierUnknown, nil
	case "typed_anchor_without_visible_edge":
		if len(answerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure)) == 1 {
			return AnswerDiagramRelationRepairCarrierStaleAnchor, []AnswerDiagramRelationRepairAction{
				AnswerDiagramRelationRepairActionRemove, AnswerDiagramRelationRepairActionReplace,
			}
		}
		return AnswerDiagramRelationRepairCarrierUnknown, nil
	case "diagram_visible_label_mismatch", "diagram_typed_recipe_missing_visible_label", "diagram_visible_label_raw_relation_kind":
		if len(answerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure)) == 1 {
			return AnswerDiagramRelationRepairCarrierLabelPair, []AnswerDiagramRelationRepairAction{
				AnswerDiagramRelationRepairActionRelabel,
			}
		}
		return AnswerDiagramRelationRepairCarrierUnknown, nil
	default:
		if len(answerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure)) == 1 {
			actions := []AnswerDiagramRelationRepairAction{AnswerDiagramRelationRepairActionRemove}
			// Only a closed set of structural issues may retain the carrier through
			// a model-authored replacement. Evidence-negative and unknown issues are
			// remove-only: restoring their hidden tuple would preserve an unproved
			// relation under new wording. The executor restores only fields
			// deliberately hidden by the opaque-ref schema.
			if answerDiagramRelationRepairIssueAllowsReplacement(issue) {
				actions = append(actions, AnswerDiagramRelationRepairActionReplace)
			}
			return AnswerDiagramRelationRepairCarrierPriorAnchor, actions
		}
		return AnswerDiagramRelationRepairCarrierUnknown, nil
	}
}

func answerDiagramRelationRepairIssueAllowsReplacement(issue string) bool {
	switch strings.TrimSpace(issue) {
	case "call_reply_operator_conflict", "sequence_relation_reply_operator_conflict",
		"typed_endpoints_collapsed_to_self_edge", "edge_anchor_node_identity_conflict":
		return true
	default:
		// Fail closed for newly introduced issue values. A producer must classify
		// a defect as an exact structural replacement case before the lease may
		// expose replace; remove remains universally executable.
		return false
	}
}

func answerDiagramRelationRepairFailureHasReplacementTuple(failure AnswerDiagramRelationRepairFailure) bool {
	return AnswerDiagramRelationRepairFailureEffectiveRelation(failure).IsValid() &&
		strings.TrimSpace(failure.FromIdentity) != "" &&
		strings.TrimSpace(failure.ToIdentity) != ""
}

// AnswerDiagramRelationRepairFailureCanAttachCandidate is the single precise
// compatibility predicate shared by lease publication, schema projection and
// execution. It reads only typed identities, direction, relation kind, block
// ownership and the exact visible occurrence. No Mermaid label, request text,
// reasoning or final prose participates.
func AnswerDiagramRelationRepairFailureCanAttachCandidate(
	failure AnswerDiagramRelationRepairFailure,
	candidate AnswerDiagramRelationRepairCandidate,
) bool {
	if failure.TargetCarrier != AnswerDiagramRelationRepairCarrierVisibleBodyEdge ||
		failure.BodyOccurrence < 1 ||
		strings.TrimSpace(failure.BlockID) == "" ||
		strings.TrimSpace(failure.BlockID) != strings.TrimSpace(candidate.BlockID) ||
		!candidate.RelationKind.IsValid() ||
		strings.TrimSpace(failure.FromIdentity) == "" ||
		strings.TrimSpace(failure.ToIdentity) == "" ||
		strings.TrimSpace(candidate.FromIdentity) == "" ||
		strings.TrimSpace(candidate.ToIdentity) == "" {
		return false
	}
	if relation := AnswerDiagramRelationRepairFailureEffectiveRelation(failure); relation.IsValid() &&
		relation != candidate.RelationKind {
		return false
	}
	return AnswerCodeIdentitySurfacesEquivalent(failure.FromIdentity, candidate.FromIdentity) &&
		AnswerCodeIdentitySurfacesEquivalent(failure.ToIdentity, candidate.ToIdentity)
}

func answerDiagramRelationRepairCompiledFailures(
	base *AnswerDocumentV2,
	failures []AnswerDiagramRelationRepairFailure,
) []AnswerDiagramRelationRepairFailure {
	byCarrier := make(map[string]AnswerDiagramRelationRepairFailure, len(failures))
	issuesByCarrier := make(map[string]map[string]bool, len(failures))
	for _, failure := range failures {
		failure.TargetCarrier, failure.AllowedActions = answerDiagramRelationRepairFailureCapabilities(base, failure)
		failure.FailureRef = ""
		failure.RelatedIssues = nil
		carrierKey := answerDiagramRelationRepairFailureCarrierKey(base, failure)
		prior, exists := byCarrier[carrierKey]
		if !exists || answerDiagramRelationRepairFailureKey(failure) < answerDiagramRelationRepairFailureKey(prior) {
			byCarrier[carrierKey] = failure
		}
		if issuesByCarrier[carrierKey] == nil {
			issuesByCarrier[carrierKey] = make(map[string]bool)
		}
		issuesByCarrier[carrierKey][strings.TrimSpace(failure.Issue)] = true
	}
	keys := make([]string, 0, len(byCarrier))
	for key := range byCarrier {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]AnswerDiagramRelationRepairFailure, 0, len(keys))
	for _, key := range keys {
		failure := byCarrier[key]
		issues := make([]string, 0, len(issuesByCarrier[key]))
		for issue := range issuesByCarrier[key] {
			if issue != "" {
				issues = append(issues, issue)
			}
		}
		sort.Strings(issues)
		if len(issues) > 0 {
			failure.Issue = issues[0]
		}
		if len(issues) > 1 {
			failure.RelatedIssues = append([]string(nil), issues...)
		}
		// Several validator issues can describe one exact prior anchor. One
		// remove/replace then resolves every issue on that carrier; publishing
		// one ref per issue would make the second operation deterministically
		// stale inside the same atomic patch. A label-only carrier remains
		// relabel-only, while any non-label defect keeps the stronger
		// remove/replace capability when every non-label defect is structural.
		// An evidence-negative sibling narrows the merged carrier to remove-only:
		// capabilities compose by safe intersection, never by union.
		if failure.TargetCarrier == AnswerDiagramRelationRepairCarrierLabelPair && len(issues) > 1 {
			for _, issue := range issues {
				if !answerDiagramRelationRepairLabelOnlyIssue(issue) {
					failure.TargetCarrier = AnswerDiagramRelationRepairCarrierPriorAnchor
					failure.AllowedActions = []AnswerDiagramRelationRepairAction{AnswerDiagramRelationRepairActionRemove}
					allStructural := true
					for _, related := range issues {
						if !answerDiagramRelationRepairLabelOnlyIssue(related) &&
							!answerDiagramRelationRepairIssueAllowsReplacement(related) {
							allStructural = false
							break
						}
					}
					if allStructural {
						failure.AllowedActions = append(failure.AllowedActions, AnswerDiagramRelationRepairActionReplace)
					}
					break
				}
			}
		}
		failure.FailureRef = answerDiagramRelationRepairFailureRef(base, failure)
		out = append(out, failure)
	}
	return out
}

func answerDiagramRelationRepairFailureCarrierKey(base *AnswerDocumentV2, failure AnswerDiagramRelationRepairFailure) string {
	if candidates := answerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure); len(candidates) == 1 {
		return strings.Join([]string{
			strings.TrimSpace(failure.BlockID), "anchor", answerDiagramRelationAnchorSemanticKey(candidates[0]),
		}, "\x00")
	}
	if failure.TargetCarrier == AnswerDiagramRelationRepairCarrierVisibleBodyEdge {
		return strings.Join([]string{
			strings.TrimSpace(failure.BlockID), "body",
			strings.TrimSpace(failure.FromNode), strings.TrimSpace(failure.ToNode),
			string(AnswerDiagramRelationRepairFailureEffectiveRelation(failure)), fmt.Sprintf("%d", failure.BodyOccurrence),
		}, "\x00")
	}
	return "failure\x00" + answerDiagramRelationRepairFailureKey(failure)
}

func answerDiagramRelationRepairLabelOnlyIssue(issue string) bool {
	switch strings.TrimSpace(issue) {
	case "diagram_visible_label_mismatch", "diagram_typed_recipe_missing_visible_label", "diagram_visible_label_raw_relation_kind":
		return true
	default:
		return false
	}
}

// AnswerDiagramRelationRepairFailureBaseAnchorCandidates is the shared exact
// locator compiler used by both lease publication and atomic ref resolution.
// Validator-resolved identities may be more precise than a rejected anchor;
// node pair + relation select the carrier first, with exact identities used
// only to disambiguate repeated operations on the same visible pair.
func AnswerDiagramRelationRepairFailureBaseAnchorCandidates(
	base *AnswerDocumentV2,
	failure AnswerDiagramRelationRepairFailure,
) []DiagramEdgeAnchor {
	if base == nil {
		return nil
	}
	var anchors []DiagramEdgeAnchor
	count := 0
	for _, block := range base.Blocks {
		if strings.TrimSpace(block.ID) != strings.TrimSpace(failure.BlockID) {
			continue
		}
		count++
		anchors = append(anchors, block.EdgeAnchors...)
	}
	if count != 1 {
		return nil
	}
	return answerDiagramRelationRepairFailureAnchorCandidates(failure, anchors)
}

func answerDiagramRelationRepairFailureBaseAnchorCandidates(
	base *AnswerDocumentV2,
	failure AnswerDiagramRelationRepairFailure,
) []DiagramEdgeAnchor {
	return AnswerDiagramRelationRepairFailureBaseAnchorCandidates(base, failure)
}

func AnswerDiagramRelationRepairFailureAnchorCandidates(
	failure AnswerDiagramRelationRepairFailure,
	anchors []DiagramEdgeAnchor,
) []DiagramEdgeAnchor {
	return answerDiagramRelationRepairFailureAnchorCandidates(failure, anchors)
}

func answerDiagramRelationRepairFailureAnchorCandidates(
	failure AnswerDiagramRelationRepairFailure,
	anchors []DiagramEdgeAnchor,
) []DiagramEdgeAnchor {
	relation := AnswerDiagramRelationRepairFailureEffectiveRelation(failure)
	fromNode, toNode := strings.TrimSpace(failure.FromNode), strings.TrimSpace(failure.ToNode)
	fromIdentity, toIdentity := strings.TrimSpace(failure.FromIdentity), strings.TrimSpace(failure.ToIdentity)
	candidates := make([]DiagramEdgeAnchor, 0, 1)
	for _, anchor := range anchors {
		if relation.IsValid() && anchor.RelationKind != relation {
			continue
		}
		if fromNode != "" || toNode != "" {
			if fromNode != strings.TrimSpace(anchor.FromNode) || toNode != strings.TrimSpace(anchor.ToNode) {
				continue
			}
		} else if fromIdentity != "" || toIdentity != "" {
			if fromIdentity != strings.TrimSpace(anchor.FromIdentity) || toIdentity != strings.TrimSpace(anchor.ToIdentity) {
				continue
			}
		}
		candidates = append(candidates, anchor)
	}
	if len(candidates) <= 1 || fromIdentity == "" || toIdentity == "" {
		return candidates
	}
	exact := candidates[:0]
	for _, anchor := range candidates {
		if fromIdentity == strings.TrimSpace(anchor.FromIdentity) && toIdentity == strings.TrimSpace(anchor.ToIdentity) {
			exact = append(exact, anchor)
		}
	}
	if len(exact) == 1 {
		return exact
	}
	return candidates
}

func AnswerDiagramRelationRepairFailureEffectiveRelation(failure AnswerDiagramRelationRepairFailure) DiagramRelationKind {
	if failure.RelationKind.IsValid() {
		return failure.RelationKind
	}
	switch strings.TrimSpace(failure.Issue) {
	case "call_edge_unproven", "call_edge_occurrence_unproven", "missing_call_anchor",
		DiagramRelationFailureMissingGroundedCallAnchor, "call_reply_operator_conflict":
		return DiagramRelCall
	case "registration_edge_unproven":
		return DiagramRelRegister
	case "type_relation_edge_unproven":
		return DiagramRelTypeRelation
	case "assignment_edge_unproven":
		return DiagramRelAssignment
	case "data_flow_edge_unproven":
		return DiagramRelDataFlow
	case "return_edge_unproven":
		return DiagramRelReturn
	case "callback_handoff_unproven":
		return DiagramRelCallback
	case "argument_flow_unproven":
		return DiagramRelArgumentFlow
	default:
		return failure.RelationKind
	}
}

func answerDiagramRelationRepairFailureRef(base *AnswerDocumentV2, failure AnswerDiagramRelationRepairFailure) string {
	if base == nil || strings.TrimSpace(failure.BlockID) == "" {
		return ""
	}
	var selected *AnswerBlock
	selectedCount := 0
	for i := range base.Blocks {
		if strings.TrimSpace(base.Blocks[i].ID) != strings.TrimSpace(failure.BlockID) {
			continue
		}
		selectedCount++
		selected = &base.Blocks[i]
	}
	failure.FailureRef = ""
	type refAnchor struct {
		FromNode     string              `json:"from_node,omitempty"`
		ToNode       string              `json:"to_node,omitempty"`
		FromIdentity string              `json:"from_identity,omitempty"`
		ToIdentity   string              `json:"to_identity,omitempty"`
		RelationKind DiagramRelationKind `json:"relation_kind,omitempty"`
	}
	type refBlock struct {
		ID          string          `json:"id"`
		Kind        AnswerBlockKind `json:"kind,omitempty"`
		EdgeAnchors []refAnchor     `json:"edge_anchors,omitempty"`
	}
	toRefBlock := func(block AnswerBlock) refBlock {
		out := refBlock{ID: strings.TrimSpace(block.ID), Kind: block.Kind}
		for _, anchor := range block.EdgeAnchors {
			out.EdgeAnchors = append(out.EdgeAnchors, refAnchor{
				FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
				FromIdentity: strings.TrimSpace(anchor.FromIdentity), ToIdentity: strings.TrimSpace(anchor.ToIdentity),
				RelationKind: anchor.RelationKind,
			})
		}
		return out
	}
	payload := struct {
		Version        int                                `json:"version"`
		Failure        AnswerDiagramRelationRepairFailure `json:"failure"`
		Block          *refBlock                          `json:"block,omitempty"`
		FallbackBlocks []refBlock                         `json:"fallback_blocks,omitempty"`
	}{
		Version: 1, Failure: failure,
	}
	if selectedCount == 1 {
		block := toRefBlock(*selected)
		payload.Block = &block
	} else {
		// Compatibility/fail-closed fallback for a malformed diagnostic whose
		// block id is absent or ambiguous in the base. The lease may still be
		// installed so existing diagnostics remain visible, but the atomic
		// executor cannot resolve this ref to a unique block/anchor and rejects.
		for _, block := range base.Blocks {
			payload.FallbackBlocks = append(payload.FallbackBlocks, toRefBlock(block))
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("rf1-%x", sum[:12])
}

// AnswerDiagramRelationRepairCandidate is one producer-owned relation tuple
// that the model may choose to add while clearing the current failure set.
// Node ids and visible labels stay outside the permission so layout and reader
// wording remain model-authored; the ordinary diagram authority gate still
// validates the selected endpoint mapping after the lease admits it.
type AnswerDiagramRelationRepairCandidate struct {
	AdditionRef  string              `json:"addition_ref,omitempty"`
	BlockID      string              `json:"block_id"`
	RelationKind DiagramRelationKind `json:"relation_kind"`
	FromIdentity string              `json:"from_identity"`
	ToIdentity   string              `json:"to_identity"`
	Source       string              `json:"source"`
}

type AnswerDiagramOrphanDispositionAction string

const (
	AnswerDiagramOrphanDispositionRemove AnswerDiagramOrphanDispositionAction = "remove_if_isolated"
	AnswerDiagramOrphanDispositionRetain AnswerDiagramOrphanDispositionAction = "retain_as_context"
)

// AnswerDiagramOrphanCleanupCandidate is a presentation-only decision derived
// from the rejected diagram's parsed topology after the relation lease is
// installed. It never authorizes an edge or chooses a disposition. If the
// model's selected relation edits actually isolate this declaration, the model
// must explicitly remove it or retain it with model-authored visible wording.
// The executor rechecks former incident edges, current isolation, requested
// participants, and uncertainty boundaries before applying either action.
type AnswerDiagramOrphanCleanupCandidate struct {
	BlockID        string                                 `json:"block_id"`
	ParticipantID  string                                 `json:"participant_id"`
	VisibleLabel   string                                 `json:"visible_label,omitempty"`
	AllowedActions []AnswerDiagramOrphanDispositionAction `json:"allowed_actions"`
}

func (c AnswerDiagramOrphanCleanupCandidate) AllowsAction(action string) bool {
	action = strings.TrimSpace(action)
	for _, allowed := range c.AllowedActions {
		if action == string(allowed) {
			return true
		}
	}
	return false
}

// AnswerDiagramRelationRepairDelta is the single producer-owned carrier for
// every relation failure emitted by one pre-emit validation cycle. Keeping the
// schema in types prevents the producer and retry consumer from drifting.
type AnswerDiagramRelationRepairDelta struct {
	Version                int                                    `json:"version"`
	Failures               []AnswerDiagramRelationRepairFailure   `json:"failures"`
	PreserveUnlistedEdges  bool                                   `json:"preserve_unlisted_edges"`
	AllowedAdditions       []AnswerDiagramRelationRepairCandidate `json:"allowed_additions,omitempty"`
	OptionalOrphanCleanups []AnswerDiagramOrphanCleanupCandidate  `json:"optional_orphan_cleanups,omitempty"`
	CandidateAlternatives  string                                 `json:"candidate_alternatives,omitempty"`
}

// AnswerDiagramRelationRepairFailureHasCompleteLocator accepts either the
// model's complete local node pair or the producer-owned complete technical
// identity pair. A half-present pair is always malformed, even when the other
// pair is complete, because silently discarding one endpoint would make audit
// and retry scope disagree.
func AnswerDiagramRelationRepairFailureHasCompleteLocator(failure AnswerDiagramRelationRepairFailure) bool {
	fromNode := strings.TrimSpace(failure.FromNode)
	toNode := strings.TrimSpace(failure.ToNode)
	fromIdentity := strings.TrimSpace(failure.FromIdentity)
	toIdentity := strings.TrimSpace(failure.ToIdentity)
	nodePresent := fromNode != "" || toNode != ""
	identityPresent := fromIdentity != "" || toIdentity != ""
	nodeComplete := fromNode != "" && toNode != ""
	identityComplete := fromIdentity != "" && toIdentity != ""
	return (!nodePresent || nodeComplete) && (!identityPresent || identityComplete) &&
		(nodeComplete || identityComplete)
}

// MergeAnswerDiagramRelationRepairDeltaJSON atomically unions all structured
// relation deltas from one validation cycle. It never derives a failure or an
// allowed edge from Mermaid text. A participant-only coverage rejection may
// legitimately publish additions without a failed prior edge: that is the
// current generation's executable capability for one already-grounded typed
// candidate. Any non-empty malformed/oversized sibling makes the result empty
// so callers cannot install a misleading partial hard lease while the visible
// rejection asks the model to repair a larger set.
func MergeAnswerDiagramRelationRepairDeltaJSON(rawDeltas []string) string {
	if len(rawDeltas) == 0 {
		return ""
	}
	failureByKey := make(map[string]AnswerDiagramRelationRepairFailure)
	additionByKey := make(map[string]AnswerDiagramRelationRepairCandidate)
	validDeltaCount := 0
	for _, raw := range rawDeltas {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if len(raw) > AnswerDiagramRelationRepairDeltaMaxJSONBytes {
			return ""
		}
		var delta AnswerDiagramRelationRepairDelta
		if err := json.Unmarshal([]byte(raw), &delta); err != nil ||
			delta.Version != AnswerDiagramRelationRepairDeltaVersion ||
			!delta.PreserveUnlistedEdges ||
			(len(delta.Failures) == 0 && len(delta.AllowedAdditions) == 0) {
			return ""
		}
		validDeltaCount++
		for _, failure := range delta.Failures {
			failure.FailureRef = strings.TrimSpace(failure.FailureRef)
			failure.BlockID = strings.TrimSpace(failure.BlockID)
			failure.Issue = strings.TrimSpace(failure.Issue)
			failure.FromNode = strings.TrimSpace(failure.FromNode)
			failure.ToNode = strings.TrimSpace(failure.ToNode)
			failure.FromIdentity = strings.TrimSpace(failure.FromIdentity)
			failure.ToIdentity = strings.TrimSpace(failure.ToIdentity)
			if failure.BlockID == "" || failure.Issue == "" ||
				!AnswerDiagramRelationRepairFailureHasCompleteLocator(failure) {
				return ""
			}
			key := answerDiagramRelationRepairFailureKey(failure)
			if prior, ok := failureByKey[key]; ok && prior.FailureRef != "" &&
				failure.FailureRef != "" && prior.FailureRef != failure.FailureRef {
				return ""
			}
			if prior, ok := failureByKey[key]; ok && failure.FailureRef == "" {
				failure.FailureRef = prior.FailureRef
			}
			failureByKey[key] = failure
			if len(failureByKey) > AnswerDiagramRelationRepairDeltaMaxEntries {
				return ""
			}
		}
		for _, candidate := range delta.AllowedAdditions {
			// Producer refs are never trusted across the merge/install boundary.
			// The live lease re-mints a selector against its exact patch base.
			candidate.AdditionRef = ""
			candidate.BlockID = strings.TrimSpace(candidate.BlockID)
			candidate.FromIdentity = strings.TrimSpace(candidate.FromIdentity)
			candidate.ToIdentity = strings.TrimSpace(candidate.ToIdentity)
			candidate.Source = strings.TrimSpace(candidate.Source)
			if candidate.BlockID == "" || !candidate.RelationKind.IsValid() ||
				candidate.FromIdentity == "" || candidate.ToIdentity == "" || candidate.Source == "" {
				return ""
			}
			key := answerDiagramRelationRepairCandidateKey(candidate)
			if prior, ok := additionByKey[key]; !ok || candidate.Source < prior.Source {
				additionByKey[key] = candidate
			}
			if len(additionByKey) > AnswerDiagramRelationRepairDeltaMaxEntries {
				return ""
			}
		}
	}
	if validDeltaCount == 0 || (len(failureByKey) == 0 && len(additionByKey) == 0) {
		return ""
	}
	failureKeys := make([]string, 0, len(failureByKey))
	for key := range failureByKey {
		failureKeys = append(failureKeys, key)
	}
	sort.Strings(failureKeys)
	failures := make([]AnswerDiagramRelationRepairFailure, 0, len(failureKeys))
	for _, key := range failureKeys {
		failures = append(failures, failureByKey[key])
	}
	additionKeys := make([]string, 0, len(additionByKey))
	for key := range additionByKey {
		additionKeys = append(additionKeys, key)
	}
	sort.Strings(additionKeys)
	additions := make([]AnswerDiagramRelationRepairCandidate, 0, len(additionKeys))
	for _, key := range additionKeys {
		additions = append(additions, additionByKey[key])
	}
	raw, err := json.Marshal(AnswerDiagramRelationRepairDelta{
		Version: AnswerDiagramRelationRepairDeltaVersion, Failures: failures,
		PreserveUnlistedEdges: true, AllowedAdditions: additions,
	})
	if err != nil || len(raw) > AnswerDiagramRelationRepairDeltaMaxJSONBytes {
		return ""
	}
	return string(raw)
}

// AnswerDiagramRelationRepairLeaseBlock snapshots the structured relation
// topology of one block at the start of a bounded repair turn. Visible labels
// are retained in the snapshot for audit, but lease comparison intentionally
// excludes VisibleLabel so reader-facing wording remains model-owned.
type AnswerDiagramRelationRepairLeaseBlock struct {
	BlockID     string              `json:"block_id"`
	Kind        AnswerBlockKind     `json:"kind,omitempty"`
	BaseAnchors []DiagramEdgeAnchor `json:"base_anchors,omitempty"`
}

// AnswerDiagramRelationRepairLease prevents a local relation repair from
// silently becoming a whole-graph rewrite. The model may remove a named failed
// relation or replace it on the same endpoint pair; every unlisted structured
// relation remains unchanged. The lease never authors, deletes, relabels, or
// reconnects an edge itself.
type AnswerDiagramRelationRepairLease struct {
	Version                     int                                             `json:"version"`
	Failures                    []AnswerDiagramRelationRepairFailure            `json:"failures"`
	AllowedAdditions            []AnswerDiagramRelationRepairCandidate          `json:"allowed_additions,omitempty"`
	OptionalOrphanCleanups      []AnswerDiagramOrphanCleanupCandidate           `json:"optional_orphan_cleanups,omitempty"`
	ParticipantBoundaryFailures []AnswerDiagramParticipantBoundaryRepairFailure `json:"participant_boundary_failures,omitempty"`
	OrdinaryValidationBlockIDs  []string                                        `json:"ordinary_validation_block_ids,omitempty"`
	Blocks                      []AnswerDiagramRelationRepairLeaseBlock         `json:"blocks"`
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
// the next patch is allowed to repair. A lease may contain failed prior edges,
// allowed typed additions, or both. The additions-only shape is necessary when
// participant coverage requires one already-grounded incident edge after the
// previous relation-failure lease was successfully consumed. An entirely
// empty or invalid capability set still produces nil.
func NewAnswerDiagramRelationRepairLease(
	base *AnswerDocumentV2,
	failures []AnswerDiagramRelationRepairFailure,
	allowedAdditions []AnswerDiagramRelationRepairCandidate,
) *AnswerDiagramRelationRepairLease {
	if base == nil || (len(failures) == 0 && len(allowedAdditions) == 0) ||
		len(failures) > AnswerDiagramRelationRepairDeltaMaxEntries ||
		len(allowedAdditions) > AnswerDiagramRelationRepairDeltaMaxEntries {
		return nil
	}
	clean := make([]AnswerDiagramRelationRepairFailure, 0, len(failures))
	targetBlocks := make(map[string]bool)
	failureSeen := make(map[string]bool, len(failures))
	for _, failure := range failures {
		failure.FailureRef = strings.TrimSpace(failure.FailureRef)
		failure.BlockID = strings.TrimSpace(failure.BlockID)
		failure.Issue = strings.TrimSpace(failure.Issue)
		failure.FromNode = strings.TrimSpace(failure.FromNode)
		failure.ToNode = strings.TrimSpace(failure.ToNode)
		failure.FromIdentity = strings.TrimSpace(failure.FromIdentity)
		failure.ToIdentity = strings.TrimSpace(failure.ToIdentity)
		if failure.BlockID == "" || failure.Issue == "" ||
			!AnswerDiagramRelationRepairFailureHasCompleteLocator(failure) {
			return nil
		}
		key := answerDiagramRelationRepairFailureKey(failure)
		if failureSeen[key] {
			continue
		}
		failureSeen[key] = true
		clean = append(clean, failure)
		targetBlocks[failure.BlockID] = true
	}
	if len(clean) == 0 && len(allowedAdditions) == 0 {
		return nil
	}
	sort.SliceStable(clean, func(i, j int) bool {
		return answerDiagramRelationRepairFailureKey(clean[i]) < answerDiagramRelationRepairFailureKey(clean[j])
	})
	allowed := make([]AnswerDiagramRelationRepairCandidate, 0, len(allowedAdditions))
	allowedSeen := make(map[string]bool, len(allowedAdditions))
	baseBlockCounts := make(map[string]int, len(base.Blocks))
	for _, block := range base.Blocks {
		if id := strings.TrimSpace(block.ID); id != "" {
			baseBlockCounts[id]++
		}
	}
	for _, candidate := range allowedAdditions {
		candidate.AdditionRef = ""
		candidate.BlockID = strings.TrimSpace(candidate.BlockID)
		candidate.FromIdentity = strings.TrimSpace(candidate.FromIdentity)
		candidate.ToIdentity = strings.TrimSpace(candidate.ToIdentity)
		candidate.Source = strings.TrimSpace(candidate.Source)
		if candidate.BlockID == "" || baseBlockCounts[candidate.BlockID] != 1 || !candidate.RelationKind.IsValid() ||
			candidate.FromIdentity == "" || candidate.ToIdentity == "" || candidate.Source == "" {
			return nil
		}
		// A failed carrier already owns its block. In the additions-only shape,
		// the candidate itself is the precise block-scoped capability.
		if len(clean) > 0 && !targetBlocks[candidate.BlockID] {
			return nil
		}
		if answerDiagramRelationRepairCandidateAlreadyAnchored(base, candidate) {
			continue
		}
		if len(clean) == 0 {
			targetBlocks[candidate.BlockID] = true
		}
		key := answerDiagramRelationRepairCandidateKey(candidate)
		if allowedSeen[key] {
			continue
		}
		allowedSeen[key] = true
		allowed = append(allowed, candidate)
	}
	if len(clean) == 0 && len(allowed) == 0 {
		return nil
	}
	sort.SliceStable(allowed, func(i, j int) bool {
		return answerDiagramRelationRepairCandidateKey(allowed[i]) < answerDiagramRelationRepairCandidateKey(allowed[j])
	})
	for i := range allowed {
		allowed[i].AdditionRef = answerDiagramRelationRepairCandidateRef(base, allowed[i])
		if allowed[i].AdditionRef == "" {
			return nil
		}
	}
	blocks := make([]AnswerDiagramRelationRepairLeaseBlock, 0, len(base.Blocks))
	for _, block := range base.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" || (len(block.EdgeAnchors) == 0 && !targetBlocks[id] && block.Kind != BlockDiagram) {
			continue
		}
		blocks = append(blocks, AnswerDiagramRelationRepairLeaseBlock{
			BlockID:     id,
			Kind:        block.Kind,
			BaseAnchors: append([]DiagramEdgeAnchor(nil), block.EdgeAnchors...),
		})
	}
	clean = answerDiagramRelationRepairCompiledFailures(base, clean)
	for i := range clean {
		for _, candidate := range allowed {
			if !AnswerDiagramRelationRepairFailureCanAttachCandidate(clean[i], candidate) {
				continue
			}
			clean[i].AllowedActions = append(clean[i].AllowedActions, AnswerDiagramRelationRepairActionAttach)
			break
		}
		// allowed_actions is part of the generation-scoped capability. Rebind
		// the opaque selector after the paired attach action is installed.
		clean[i].FailureRef = answerDiagramRelationRepairFailureRef(base, clean[i])
		if clean[i].FailureRef == "" {
			return nil
		}
	}
	return &AnswerDiagramRelationRepairLease{
		Version: 1, Failures: clean, AllowedAdditions: allowed, Blocks: blocks,
	}
}

// BindAnswerDiagramRelationRepairOrdinaryValidationBlocks grants a bounded
// same-generation exception for exact non-diagram relation carriers that a
// sibling validator has already required the model to correct. The diagram
// lease stops comparing those blocks with its old snapshot; the ordinary
// pre/post-emit relation validators remain the authority for every resulting
// anchor. This consumes only producer-owned block IDs plus the exact patch
// base and never inspects request text, model prose, Mermaid labels, or output.
func BindAnswerDiagramRelationRepairOrdinaryValidationBlocks(
	lease *AnswerDiagramRelationRepairLease,
	base *AnswerDocumentV2,
	blockIDs []string,
) bool {
	if lease == nil || base == nil || len(blockIDs) > AnswerDiagramRelationRepairOrdinaryValidationMaxEntries {
		return false
	}
	if len(blockIDs) == 0 {
		lease.OrdinaryValidationBlockIDs = nil
		return true
	}
	counts := make(map[string]int, len(base.Blocks))
	kinds := make(map[string]AnswerBlockKind, len(base.Blocks))
	for _, block := range base.Blocks {
		id := strings.TrimSpace(block.ID)
		if id == "" {
			continue
		}
		counts[id]++
		kinds[id] = block.Kind
	}
	seen := make(map[string]bool, len(blockIDs))
	clean := make([]string, 0, len(blockIDs))
	for _, raw := range blockIDs {
		id := strings.TrimSpace(raw)
		if id == "" || counts[id] != 1 || !answerDiagramRelationRepairOrdinaryValidationKind(kinds[id]) {
			return false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		clean = append(clean, id)
	}
	if len(clean) == 0 {
		return false
	}
	sort.Strings(clean)
	lease.OrdinaryValidationBlockIDs = clean
	return true
}

func answerDiagramRelationRepairOrdinaryValidationKind(kind AnswerBlockKind) bool {
	switch kind {
	case BlockOrderedList, BlockBulletList, BlockTable:
		return true
	default:
		return false
	}
}

// answerDiagramRelationRepairCandidateAlreadyAnchored is the lease boundary's
// defense in depth: an addition capability can never name a canonical tuple
// already present in the base carrier. Visible endpoints and labels are not
// part of this identity and remain model-owned.
func answerDiagramRelationRepairCandidateAlreadyAnchored(
	base *AnswerDocumentV2,
	candidate AnswerDiagramRelationRepairCandidate,
) bool {
	if base == nil {
		return false
	}
	for _, block := range base.Blocks {
		if strings.TrimSpace(block.ID) != strings.TrimSpace(candidate.BlockID) {
			continue
		}
		for _, anchor := range block.EdgeAnchors {
			if anchor.RelationKind == candidate.RelationKind &&
				strings.EqualFold(strings.TrimSpace(anchor.FromIdentity), strings.TrimSpace(candidate.FromIdentity)) &&
				strings.EqualFold(strings.TrimSpace(anchor.ToIdentity), strings.TrimSpace(candidate.ToIdentity)) {
				return true
			}
		}
	}
	return false
}

// answerDiagramRelationRepairCandidateRef binds one optional addition to the
// exact rejected carrier whose live lease may admit it. The model uses this
// opaque selector to choose a producer-owned relation candidate without
// retyping invisible canonical identities. Visible node ids, labels, ordering,
// and layout remain outside the hash and remain model-authored. As with a
// failure_ref, a changed typed anchor snapshot produces a different selector.
func answerDiagramRelationRepairCandidateRef(base *AnswerDocumentV2, candidate AnswerDiagramRelationRepairCandidate) string {
	if base == nil || strings.TrimSpace(candidate.BlockID) == "" ||
		!candidate.RelationKind.IsValid() || strings.TrimSpace(candidate.FromIdentity) == "" ||
		strings.TrimSpace(candidate.ToIdentity) == "" {
		return ""
	}
	candidate.AdditionRef = ""
	type refAnchor struct {
		FromNode     string              `json:"from_node,omitempty"`
		ToNode       string              `json:"to_node,omitempty"`
		FromIdentity string              `json:"from_identity,omitempty"`
		ToIdentity   string              `json:"to_identity,omitempty"`
		RelationKind DiagramRelationKind `json:"relation_kind,omitempty"`
	}
	type refBlock struct {
		ID          string          `json:"id"`
		Kind        AnswerBlockKind `json:"kind,omitempty"`
		EdgeAnchors []refAnchor     `json:"edge_anchors,omitempty"`
	}
	toRefBlock := func(block AnswerBlock) refBlock {
		out := refBlock{ID: strings.TrimSpace(block.ID), Kind: block.Kind}
		for _, anchor := range block.EdgeAnchors {
			out.EdgeAnchors = append(out.EdgeAnchors, refAnchor{
				FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
				FromIdentity: strings.TrimSpace(anchor.FromIdentity), ToIdentity: strings.TrimSpace(anchor.ToIdentity),
				RelationKind: anchor.RelationKind,
			})
		}
		return out
	}
	var selected *refBlock
	selectedCount := 0
	for _, block := range base.Blocks {
		if strings.TrimSpace(block.ID) != strings.TrimSpace(candidate.BlockID) {
			continue
		}
		selectedCount++
		value := toRefBlock(block)
		selected = &value
	}
	payload := struct {
		Version        int                                  `json:"version"`
		Candidate      AnswerDiagramRelationRepairCandidate `json:"candidate"`
		Block          *refBlock                            `json:"block,omitempty"`
		FallbackBlocks []refBlock                           `json:"fallback_blocks,omitempty"`
	}{Version: 1, Candidate: candidate}
	if selectedCount == 1 {
		payload.Block = selected
	} else {
		for _, block := range base.Blocks {
			payload.FallbackBlocks = append(payload.FallbackBlocks, toRefBlock(block))
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("ra1-%x", sum[:12])
}

// ValidateAnswerDiagramRelationRepairLease checks only typed edge-anchor
// topology. It does not inspect Mermaid source, node/edge labels, request text,
// reasoning, or final prose. Ordinary diagram evidence validation remains the
// authority for whether a corrected relation is true.
func ValidateAnswerDiagramRelationRepairLease(lease *AnswerDiagramRelationRepairLease, merged *AnswerDocumentV2) []AnswerDiagramRelationRepairScopeViolation {
	if lease == nil || lease.Version != 1 || merged == nil ||
		(len(lease.Failures) == 0 && len(lease.AllowedAdditions) == 0 && len(lease.ParticipantBoundaryFailures) == 0) {
		return nil
	}
	resultBlocks := make(map[string][]DiagramEdgeAnchor, len(merged.Blocks))
	resultKinds := make(map[string]AnswerBlockKind, len(merged.Blocks))
	for _, block := range merged.Blocks {
		id := strings.TrimSpace(block.ID)
		if id != "" {
			resultBlocks[id] = append(resultBlocks[id], block.EdgeAnchors...)
			resultKinds[id] = block.Kind
		}
	}
	baseBlocks := make(map[string][]DiagramEdgeAnchor, len(lease.Blocks))
	baseDiagramIDs := make(map[string]bool)
	for _, block := range lease.Blocks {
		id := strings.TrimSpace(block.BlockID)
		if id != "" {
			baseBlocks[id] = append(baseBlocks[id], block.BaseAnchors...)
			if block.Kind == BlockDiagram {
				baseDiagramIDs[id] = true
			}
		}
	}
	ordinaryValidationIDs := make(map[string]bool, len(lease.OrdinaryValidationBlockIDs))
	for _, raw := range lease.OrdinaryValidationBlockIDs {
		if id := strings.TrimSpace(raw); id != "" {
			ordinaryValidationIDs[id] = true
		}
	}
	var violations []AnswerDiagramRelationRepairScopeViolation
	if len(baseDiagramIDs) > 0 {
		baseDiagramOrder := make([]string, 0, len(baseDiagramIDs))
		for id := range baseDiagramIDs {
			baseDiagramOrder = append(baseDiagramOrder, id)
		}
		sort.Strings(baseDiagramOrder)
		for _, id := range baseDiagramOrder {
			kind, exists := resultKinds[id]
			switch {
			case !exists:
				violations = append(violations, AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "relation_diagram_carrier_removed"})
			case kind != BlockDiagram:
				violations = append(violations, AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "relation_diagram_carrier_kind_changed"})
			}
		}
		resultDiagramOrder := make([]string, 0, len(resultKinds))
		for id, kind := range resultKinds {
			if kind == BlockDiagram && !baseDiagramIDs[id] {
				resultDiagramOrder = append(resultDiagramOrder, id)
			}
		}
		sort.Strings(resultDiagramOrder)
		for _, id := range resultDiagramOrder {
			violations = append(violations, AnswerDiagramRelationRepairScopeViolation{BlockID: id, Issue: "relation_diagram_carrier_added"})
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

	for _, blockID := range orderedIDs {
		if ordinaryValidationIDs[blockID] {
			if kind, exists := resultKinds[blockID]; !exists || answerDiagramRelationRepairOrdinaryValidationKind(kind) {
				continue
			}
		}
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
		for _, key := range answerDiagramRelationSortedCountKeys(baseCounts) {
			count := baseCounts[key]
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
		allowedUsed := make(map[string]int)
		for _, key := range answerDiagramRelationSortedCountKeys(resultCounts) {
			count := resultCounts[key]
			extra := count - baseCounts[key]
			if extra <= 0 {
				continue
			}
			anchor, ok := answerDiagramRelationAnchorByKey(result, key)
			for i := 0; ok && i < extra; i++ {
				if !answerDiagramRelationAnchorMatchesAnyFailure(blockID, anchor, lease.Failures) {
					if candidate, listed := answerDiagramRelationAllowedCandidate(blockID, anchor, lease.AllowedAdditions); listed {
						candidateKey := answerDiagramRelationRepairCandidateKey(candidate)
						allowedUsed[candidateKey]++
						if allowedUsed[candidateKey] > 1 {
							violations = append(violations, AnswerDiagramRelationRepairScopeViolation{
								BlockID: blockID, Issue: "listed_relation_expanded",
								FromNode: strings.TrimSpace(anchor.FromNode), ToNode: strings.TrimSpace(anchor.ToNode),
							})
						}
						continue
					}
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

func answerDiagramRelationSortedCountKeys(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func answerDiagramRelationAllowedCandidate(
	blockID string,
	anchor DiagramEdgeAnchor,
	candidates []AnswerDiagramRelationRepairCandidate,
) (AnswerDiagramRelationRepairCandidate, bool) {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.BlockID) == blockID &&
			candidate.RelationKind == anchor.RelationKind &&
			strings.TrimSpace(candidate.FromIdentity) == strings.TrimSpace(anchor.FromIdentity) &&
			strings.TrimSpace(candidate.ToIdentity) == strings.TrimSpace(anchor.ToIdentity) {
			return candidate, true
		}
	}
	return AnswerDiagramRelationRepairCandidate{}, false
}

func answerDiagramRelationRepairCandidateKey(candidate AnswerDiagramRelationRepairCandidate) string {
	return strings.Join([]string{
		strings.TrimSpace(candidate.BlockID), strings.TrimSpace(string(candidate.RelationKind)),
		strings.TrimSpace(candidate.FromIdentity), strings.TrimSpace(candidate.ToIdentity),
	}, "\x00")
}

func answerDiagramRelationRepairFailureKey(failure AnswerDiagramRelationRepairFailure) string {
	return strings.Join([]string{
		strings.TrimSpace(failure.BlockID), strings.TrimSpace(failure.Issue),
		strings.TrimSpace(string(failure.RelationKind)),
		strings.TrimSpace(failure.FromNode), strings.TrimSpace(failure.ToNode),
		strings.TrimSpace(failure.FromIdentity), strings.TrimSpace(failure.ToIdentity),
		fmt.Sprintf("%d", failure.BodyOccurrence),
	}, "\x00")
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
	if len(in.Failures) > 0 {
		out.Failures = make([]AnswerDiagramRelationRepairFailure, len(in.Failures))
		for i, failure := range in.Failures {
			out.Failures[i] = failure
			out.Failures[i].AllowedActions = append([]AnswerDiagramRelationRepairAction(nil), failure.AllowedActions...)
			out.Failures[i].RelatedIssues = append([]string(nil), failure.RelatedIssues...)
		}
	}
	out.AllowedAdditions = append([]AnswerDiagramRelationRepairCandidate(nil), in.AllowedAdditions...)
	out.OrdinaryValidationBlockIDs = append([]string(nil), in.OrdinaryValidationBlockIDs...)
	if len(in.ParticipantBoundaryFailures) > 0 {
		out.ParticipantBoundaryFailures = make([]AnswerDiagramParticipantBoundaryRepairFailure, len(in.ParticipantBoundaryFailures))
		for i, failure := range in.ParticipantBoundaryFailures {
			out.ParticipantBoundaryFailures[i] = failure
			out.ParticipantBoundaryFailures[i].AllowedBoundaryActions = append(
				[]AnswerDiagramParticipantBoundaryRepairAction(nil), failure.AllowedBoundaryActions...,
			)
		}
	}
	if len(in.OptionalOrphanCleanups) > 0 {
		out.OptionalOrphanCleanups = make([]AnswerDiagramOrphanCleanupCandidate, len(in.OptionalOrphanCleanups))
		for i, candidate := range in.OptionalOrphanCleanups {
			out.OptionalOrphanCleanups[i] = candidate
			out.OptionalOrphanCleanups[i].AllowedActions = append(
				[]AnswerDiagramOrphanDispositionAction(nil), candidate.AllowedActions...,
			)
		}
	}
	if len(in.Blocks) > 0 {
		out.Blocks = make([]AnswerDiagramRelationRepairLeaseBlock, len(in.Blocks))
		for i, block := range in.Blocks {
			out.Blocks[i] = AnswerDiagramRelationRepairLeaseBlock{
				BlockID: block.BlockID, Kind: block.Kind,
				BaseAnchors: append([]DiagramEdgeAnchor(nil), block.BaseAnchors...),
			}
		}
	}
	return out
}
