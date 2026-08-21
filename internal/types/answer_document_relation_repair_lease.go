package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const (
	AnswerDiagramRelationRepairDeltaVersion      = 1
	AnswerDiagramRelationRepairDeltaMaxEntries   = 128
	AnswerDiagramRelationRepairDeltaMaxJSONBytes = 64 * 1024
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
}

// AnswerDiagramRelationRepairTargetCarrier tells the retrying model which
// exact carrier the opaque failure ref selects. This is structural capability
// metadata derived from the rejected document, never a suggested repair.
type AnswerDiagramRelationRepairTargetCarrier string

const (
	AnswerDiagramRelationRepairCarrierUnknown         AnswerDiagramRelationRepairTargetCarrier = "unknown"
	AnswerDiagramRelationRepairCarrierPriorAnchor     AnswerDiagramRelationRepairTargetCarrier = "prior_anchor"
	AnswerDiagramRelationRepairCarrierVisibleBodyEdge AnswerDiagramRelationRepairTargetCarrier = "visible_body_edge"
	AnswerDiagramRelationRepairCarrierStaleAnchor     AnswerDiagramRelationRepairTargetCarrier = "stale_anchor"
	AnswerDiagramRelationRepairCarrierLabelPair       AnswerDiagramRelationRepairTargetCarrier = "label_pair"
)

// AnswerDiagramRelationRepairAction is one atomic operation that the current
// carrier can execute. The model still chooses among these actions and authors
// every replacement tuple and reader-facing label.
type AnswerDiagramRelationRepairAction string

const (
	AnswerDiagramRelationRepairActionRelabel AnswerDiagramRelationRepairAction = "relabel"
	AnswerDiagramRelationRepairActionRemove  AnswerDiagramRelationRepairAction = "remove"
	AnswerDiagramRelationRepairActionReplace AnswerDiagramRelationRepairAction = "replace"
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
	issue := strings.TrimSpace(failure.Issue)
	switch issue {
	case "missing_call_anchor", DiagramRelationFailureMissingGroundedCallAnchor, "missing_relation_anchor":
		if strings.TrimSpace(failure.FromNode) != "" && strings.TrimSpace(failure.ToNode) != "" {
			return AnswerDiagramRelationRepairCarrierVisibleBodyEdge, []AnswerDiagramRelationRepairAction{
				AnswerDiagramRelationRepairActionRemove, AnswerDiagramRelationRepairActionReplace,
			}
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
			return AnswerDiagramRelationRepairCarrierPriorAnchor, []AnswerDiagramRelationRepairAction{
				AnswerDiagramRelationRepairActionRemove, AnswerDiagramRelationRepairActionReplace,
			}
		}
		return AnswerDiagramRelationRepairCarrierUnknown, nil
	}
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
		// remove/replace capability.
		if failure.TargetCarrier == AnswerDiagramRelationRepairCarrierLabelPair && len(issues) > 1 {
			for _, issue := range issues {
				if !answerDiagramRelationRepairLabelOnlyIssue(issue) {
					failure.TargetCarrier = AnswerDiagramRelationRepairCarrierPriorAnchor
					failure.AllowedActions = []AnswerDiagramRelationRepairAction{
						AnswerDiagramRelationRepairActionRemove, AnswerDiagramRelationRepairActionReplace,
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
			string(AnswerDiagramRelationRepairFailureEffectiveRelation(failure)),
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
	BlockID      string              `json:"block_id"`
	RelationKind DiagramRelationKind `json:"relation_kind"`
	FromIdentity string              `json:"from_identity"`
	ToIdentity   string              `json:"to_identity"`
	Source       string              `json:"source"`
}

// AnswerDiagramRelationRepairDelta is the single producer-owned carrier for
// every relation failure emitted by one pre-emit validation cycle. Keeping the
// schema in types prevents the producer and retry consumer from drifting.
type AnswerDiagramRelationRepairDelta struct {
	Version               int                                    `json:"version"`
	Failures              []AnswerDiagramRelationRepairFailure   `json:"failures"`
	PreserveUnlistedEdges bool                                   `json:"preserve_unlisted_edges"`
	AllowedAdditions      []AnswerDiagramRelationRepairCandidate `json:"allowed_additions,omitempty"`
	CandidateAlternatives string                                 `json:"candidate_alternatives,omitempty"`
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
// allowed edge from Mermaid text. Any non-empty malformed/oversized sibling
// makes the result empty so callers cannot install a misleading partial hard
// lease while the visible rejection asks the model to repair a larger set.
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
			!delta.PreserveUnlistedEdges || len(delta.Failures) == 0 {
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
	if validDeltaCount == 0 || len(failureByKey) == 0 {
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
	Version          int                                     `json:"version"`
	Failures         []AnswerDiagramRelationRepairFailure    `json:"failures"`
	AllowedAdditions []AnswerDiagramRelationRepairCandidate  `json:"allowed_additions,omitempty"`
	Blocks           []AnswerDiagramRelationRepairLeaseBlock `json:"blocks"`
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
func NewAnswerDiagramRelationRepairLease(
	base *AnswerDocumentV2,
	failures []AnswerDiagramRelationRepairFailure,
	allowedAdditions []AnswerDiagramRelationRepairCandidate,
) *AnswerDiagramRelationRepairLease {
	if base == nil || len(failures) == 0 ||
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
	if len(clean) == 0 {
		return nil
	}
	sort.SliceStable(clean, func(i, j int) bool {
		return answerDiagramRelationRepairFailureKey(clean[i]) < answerDiagramRelationRepairFailureKey(clean[j])
	})
	allowed := make([]AnswerDiagramRelationRepairCandidate, 0, len(allowedAdditions))
	allowedSeen := make(map[string]bool, len(allowedAdditions))
	for _, candidate := range allowedAdditions {
		candidate.BlockID = strings.TrimSpace(candidate.BlockID)
		candidate.FromIdentity = strings.TrimSpace(candidate.FromIdentity)
		candidate.ToIdentity = strings.TrimSpace(candidate.ToIdentity)
		candidate.Source = strings.TrimSpace(candidate.Source)
		if !targetBlocks[candidate.BlockID] || !candidate.RelationKind.IsValid() ||
			candidate.FromIdentity == "" || candidate.ToIdentity == "" || candidate.Source == "" {
			return nil
		}
		key := answerDiagramRelationRepairCandidateKey(candidate)
		if allowedSeen[key] {
			continue
		}
		allowedSeen[key] = true
		allowed = append(allowed, candidate)
	}
	sort.SliceStable(allowed, func(i, j int) bool {
		return answerDiagramRelationRepairCandidateKey(allowed[i]) < answerDiagramRelationRepairCandidateKey(allowed[j])
	})
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
	for _, failure := range clean {
		if failure.FailureRef == "" {
			return nil
		}
	}
	return &AnswerDiagramRelationRepairLease{
		Version: 1, Failures: clean, AllowedAdditions: allowed, Blocks: blocks,
	}
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
