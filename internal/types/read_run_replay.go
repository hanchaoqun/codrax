package types

import (
	"sort"
	"strings"
)

// ReadRunReplayReasonCode is the closed reason family for transforming
// snapshot node state into dispatch-safe resume state. It is derived only from
// typed task graph/status/attempt/artifact data.
type ReadRunReplayReasonCode string

const (
	ReadRunReplayReasonPendingPreserved       ReadRunReplayReasonCode = "pending_preserved"
	ReadRunReplayReasonRequeuedPreserved      ReadRunReplayReasonCode = "requeued_preserved"
	ReadRunReplayReasonRunningInterrupted     ReadRunReplayReasonCode = "running_interrupted"
	ReadRunReplayReasonDoneNoRequiredArtifact ReadRunReplayReasonCode = "done_no_required_artifact"
	ReadRunReplayReasonDoneArtifactsValid     ReadRunReplayReasonCode = "done_artifacts_valid"
	ReadRunReplayReasonDoneArtifactMissing    ReadRunReplayReasonCode = "done_required_artifact_missing"
	ReadRunReplayReasonFailedRetryAvailable   ReadRunReplayReasonCode = "failed_retry_available"
	ReadRunReplayReasonFailedRetryExhausted   ReadRunReplayReasonCode = "failed_retry_exhausted"
)

// ReadRunReplayInput is the deterministic input for explicit read-run resume.
// Runtime payloads remain in their existing typed carriers; this view uses only
// artifact refs and task graph declarations to avoid a second content authority.
type ReadRunReplayInput struct {
	TaskGraph     TaskGraph
	NodeStatuses  map[string]NodeExecStatus
	NodeAttempts  map[string]int
	NodeArtifacts []NodeArtifactRecord
}

// ReadRunReplayTransition records how one snapshot node status should hydrate.
type ReadRunReplayTransition struct {
	NodeID         string                  `json:"node_id,omitempty"`
	FromStatus     NodeExecStatus          `json:"from_status,omitempty"`
	ToStatus       NodeExecStatus          `json:"to_status,omitempty"`
	ReasonCode     ReadRunReplayReasonCode `json:"reason_code,omitempty"`
	AttemptsUsed   int                     `json:"attempts_used,omitempty"`
	MaxAttempts    int                     `json:"max_attempts,omitempty"`
	ArtifactValid  bool                    `json:"artifact_valid,omitempty"`
	ArtifactRefIDs []string                `json:"artifact_ref_ids,omitempty"`
}

// DeriveReadRunReplayTransitions converts raw snapshot status into resume-safe
// status. Unknown node ids should already be rejected by the caller; this
// helper ignores them so it cannot panic or create a new graph authority.
func DeriveReadRunReplayTransitions(input ReadRunReplayInput) []ReadRunReplayTransition {
	statuses := cloneNodeExecStatusMap(input.NodeStatuses)
	if len(statuses) == 0 {
		return nil
	}
	attempts := cloneNodeExecAttemptMap(input.NodeAttempts)
	nodes := readRunReplayNodesByID(input.TaskGraph)
	ordered := readRunReplayStatusNodeIDs(input.TaskGraph, statuses)
	ledger := NormalizeNodeArtifactLedger(NodeArtifactLedger{Records: input.NodeArtifacts})
	out := make([]ReadRunReplayTransition, 0, len(ordered))
	for _, nodeID := range ordered {
		node, ok := nodes[nodeID]
		if !ok {
			continue
		}
		from := NormalizeNodeExecStatus(statuses[nodeID])
		used := attempts[nodeID]
		max := readRunReplayMaxAttempts(node)
		transition := ReadRunReplayTransition{
			NodeID:       nodeID,
			FromStatus:   from,
			ToStatus:     from,
			AttemptsUsed: used,
			MaxAttempts:  max,
		}
		switch from {
		case NodeExecRunning:
			transition.ToStatus = NodeExecRequeued
			transition.ReasonCode = ReadRunReplayReasonRunningInterrupted
		case NodeExecDone:
			valid, reason, refs := readRunReplayDoneArtifactState(node, ledger)
			transition.ArtifactValid = valid
			transition.ArtifactRefIDs = refs
			transition.ReasonCode = reason
			if !valid {
				transition.ToStatus = NodeExecRequeued
			}
		case NodeExecFailed:
			if max <= 0 || used < max {
				transition.ToStatus = NodeExecRequeued
				transition.ReasonCode = ReadRunReplayReasonFailedRetryAvailable
			} else {
				transition.ToStatus = NodeExecFailed
				transition.ReasonCode = ReadRunReplayReasonFailedRetryExhausted
			}
		case NodeExecRequeued:
			transition.ToStatus = NodeExecRequeued
			transition.ReasonCode = ReadRunReplayReasonRequeuedPreserved
		default:
			transition.ToStatus = NodeExecPending
			transition.ReasonCode = ReadRunReplayReasonPendingPreserved
		}
		out = append(out, normalizeReadRunReplayTransition(transition))
	}
	return out
}

func ReadRunReplayNodeStatuses(transitions []ReadRunReplayTransition) map[string]NodeExecStatus {
	if len(transitions) == 0 {
		return nil
	}
	out := make(map[string]NodeExecStatus, len(transitions))
	for _, transition := range transitions {
		transition = normalizeReadRunReplayTransition(transition)
		if transition.NodeID == "" {
			continue
		}
		out[transition.NodeID] = transition.ToStatus
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeReadRunReplayTransition(t ReadRunReplayTransition) ReadRunReplayTransition {
	t.NodeID = strings.TrimSpace(t.NodeID)
	t.FromStatus = NormalizeNodeExecStatus(t.FromStatus)
	t.ToStatus = NormalizeNodeExecStatus(t.ToStatus)
	t.ReasonCode = ReadRunReplayReasonCode(strings.TrimSpace(string(t.ReasonCode)))
	if t.AttemptsUsed < 0 {
		t.AttemptsUsed = 0
	}
	if t.MaxAttempts < 0 {
		t.MaxAttempts = 0
	}
	t.ArtifactRefIDs = compactReadRunStrings(t.ArtifactRefIDs...)
	return t
}

func readRunReplayNodesByID(g TaskGraph) map[string]TaskNode {
	out := make(map[string]TaskNode, len(g.Nodes))
	for _, node := range g.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			out[id] = node
		}
	}
	return out
}

func readRunReplayStatusNodeIDs(g TaskGraph, statuses map[string]NodeExecStatus) []string {
	seen := make(map[string]bool, len(statuses))
	out := make([]string, 0, len(statuses))
	for _, node := range g.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			continue
		}
		if _, ok := statuses[id]; ok {
			seen[id] = true
			out = append(out, id)
		}
	}
	var extras []string
	for id := range statuses {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			extras = append(extras, id)
		}
	}
	sort.Strings(extras)
	return append(out, extras...)
}

func readRunReplayMaxAttempts(node TaskNode) int {
	if node.MaxRetries <= 0 {
		return 0
	}
	return node.MaxRetries + 1
}

func readRunReplayDoneArtifactState(node TaskNode, ledger NodeArtifactLedger) (bool, ReadRunReplayReasonCode, []string) {
	required := readRunReplaySupportedOutputKinds(node.Outputs)
	if len(required) == 0 {
		return true, ReadRunReplayReasonDoneNoRequiredArtifact, nil
	}
	records := ledger.RecordsByProducer(node.ID)
	var refs []string
	for _, record := range records {
		record = NormalizeNodeArtifactRecord(record)
		if !record.Valid || record.Direction == RuntimeArtifactConsumed {
			continue
		}
		kind := NormalizeRuntimeArtifactKind(record.Artifact.Kind)
		if !required[kind] {
			continue
		}
		refs = append(refs, record.Artifact.ID)
	}
	refs = compactReadRunStrings(refs...)
	if len(refs) == 0 {
		return false, ReadRunReplayReasonDoneArtifactMissing, nil
	}
	return true, ReadRunReplayReasonDoneArtifactsValid, refs
}

func readRunReplaySupportedOutputKinds(outputs []string) map[RuntimeArtifactKind]bool {
	required := map[RuntimeArtifactKind]bool{}
	for _, output := range outputs {
		switch strings.TrimSpace(output) {
		case taskArtifactInputEvidenceItems:
			required[RuntimeArtifactEvidenceItem] = true
		case taskArtifactInputAnswerChains:
			required[RuntimeArtifactAnswerChain] = true
		case taskArtifactInputAggregateFacts:
			required[RuntimeArtifactAggregateFact] = true
		case taskArtifactInputAnalysisRefinementHandoff:
			required[RuntimeArtifactAnalysisRefinementHandoff] = true
		}
	}
	if len(required) == 0 {
		return nil
	}
	return required
}
