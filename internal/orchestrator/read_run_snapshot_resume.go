package orchestrator

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	readRunSnapshotSeedReasonSchemaMismatch        = "schema_mismatch"
	readRunSnapshotSeedReasonRepoMismatch          = "repo_mismatch"
	readRunSnapshotSeedReasonTaskGraphMismatch     = "task_graph_mismatch"
	readRunSnapshotSeedReasonNodeMismatch          = "node_status_mismatch"
	readRunSnapshotSeedReasonNodeAttemptMismatch   = "node_attempt_mismatch"
	readRunSnapshotSeedReasonNodeArtifactMismatch  = "node_artifact_mismatch"
	readRunSnapshotSeedReasonRequestFingerprint    = "request_fingerprint_mismatch"
	readRunSnapshotSeedReasonRepoFingerprint       = "repo_fingerprint_mismatch"
	readRunSnapshotSeedReasonAttachmentFingerprint = "attachment_fingerprint_mismatch"
	readRunSnapshotSeedReasonActiveStateMismatch   = "active_state_mismatch"
	readRunSnapshotSeedReasonNoAnalysisIR          = "analysis_ir_missing"
)

// SetReadRunSnapshotSeed installs a one-shot typed resume seed. The seed is
// explicit: routine read mode never auto-loads the latest snapshot.
func (o *Orchestrator) SetReadRunSnapshotSeed(snapshot *types.ReadRunSnapshot) {
	if o == nil {
		return
	}
	if snapshot == nil {
		o.readRunSnapshotSeed = nil
		return
	}
	normalized := types.NormalizeReadRunSnapshot(*snapshot)
	o.readRunSnapshotSeed = &normalized
}

func (o *Orchestrator) applyReadRunSnapshotSeedToTaskState() bool {
	if err := o.applyReadRunSnapshotSeed(); err != nil {
		logging.Warning("[orchestrator] read run snapshot seed rejected: %v", err)
		if o != nil && o.busCtx != nil && o.busCtx.TaskState.LastError == "" {
			o.busCtx.TaskState.LastError = err.Error()
		}
		return false
	}
	return true
}

func (o *Orchestrator) applyReadRunSnapshotSeed() error {
	if o == nil || o.readRunSnapshotSeed == nil {
		return nil
	}
	snapshot := *o.readRunSnapshotSeed
	o.readRunSnapshotSeed = nil
	if o.busCtx == nil || o.busCtx.Mode != types.ModeRead {
		return nil
	}
	if snapshot.SchemaVersion != types.ReadRunSnapshotSchemaVersion {
		return readRunSnapshotSeedError(readRunSnapshotSeedReasonSchemaMismatch,
			"schema version %d does not match current %d", snapshot.SchemaVersion, types.ReadRunSnapshotSchemaVersion)
	}
	if o.busCtx.AnalysisIR == nil {
		return readRunSnapshotSeedError(readRunSnapshotSeedReasonNoAnalysisIR, "analysis IR is unavailable")
	}
	if !sameReadRunSnapshotRepoRoot(snapshot.RepoRoot, o.busCtx.RepoRoot) {
		return readRunSnapshotSeedError(readRunSnapshotSeedReasonRepoMismatch,
			"snapshot repo %q does not match current repo %q", snapshot.RepoRoot, o.busCtx.RepoRoot)
	}
	if err := o.validateReadRunSnapshotFingerprints(snapshot); err != nil {
		return err
	}
	activeState := types.NormalizeReadRunActiveState(snapshot.ActiveState)
	if decision := types.ValidateReadRunActiveState(activeState, o.busCtx.RepoRoot); !decision.Valid {
		return readRunSnapshotSeedError(readRunSnapshotSeedReasonActiveStateMismatch,
			"snapshot active state invalid: reason=%s field=%s detail=%s", decision.ReasonCode, decision.Field, decision.Detail)
	}
	expectedHash := types.ReadTaskGraphHash(o.busCtx.AnalysisIR.TaskGraph)
	if expectedHash == "" || strings.TrimSpace(snapshot.TaskGraphHash) != expectedHash {
		return readRunSnapshotSeedError(readRunSnapshotSeedReasonTaskGraphMismatch,
			"snapshot task graph %q does not match current task graph %q", snapshot.TaskGraphHash, expectedHash)
	}
	knownNodes := readTaskGraphNodeSet(o.busCtx.AnalysisIR.TaskGraph)
	for nodeID := range snapshot.NodeStatuses {
		if _, ok := knownNodes[nodeID]; !ok {
			return readRunSnapshotSeedError(readRunSnapshotSeedReasonNodeMismatch,
				"snapshot node status references unknown node %q", nodeID)
		}
	}
	for nodeID := range snapshot.NodeAttempts {
		if _, ok := knownNodes[nodeID]; !ok {
			return readRunSnapshotSeedError(readRunSnapshotSeedReasonNodeAttemptMismatch,
				"snapshot node attempts reference unknown node %q", nodeID)
		}
	}
	for _, artifact := range snapshot.NodeArtifacts {
		nodeID := strings.TrimSpace(artifact.ProducerNodeID)
		if nodeID != "" {
			if _, ok := knownNodes[nodeID]; !ok {
				return readRunSnapshotSeedError(readRunSnapshotSeedReasonNodeArtifactMismatch,
					"snapshot node artifact references unknown producer node %q", nodeID)
			}
		}
		consumerNodeID := strings.TrimSpace(artifact.ConsumerNodeID)
		if consumerNodeID != "" {
			if _, ok := knownNodes[consumerNodeID]; !ok {
				return readRunSnapshotSeedError(readRunSnapshotSeedReasonNodeArtifactMismatch,
					"snapshot node artifact references unknown consumer node %q", consumerNodeID)
			}
		}
		if artifact.Direction == types.RuntimeArtifactConsumed && consumerNodeID == "" {
			return readRunSnapshotSeedError(readRunSnapshotSeedReasonNodeArtifactMismatch,
				"snapshot node artifact consumed record is missing consumer node")
		}
	}
	if o.busCtx.Mutable == nil {
		o.busCtx.Mutable = types.NewMutableState(snapshot.Request)
		o.busCtx.Mutable.SetRepoRoot(o.busCtx.RepoRoot)
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	closure.IngestEvidenceReducerInput(types.EvidenceReducerInput{
		Class:                      types.EvidenceReducerInputReadRunSnapshotSeed,
		NodeStatuses:               snapshot.NodeStatuses,
		NodeAttempts:               snapshot.NodeAttempts,
		NodeArtifacts:              snapshot.NodeArtifacts,
		ReadSet:                    readRunSnapshotReadSetMap(snapshot.ReadSet),
		ReadRanges:                 snapshot.ReadRanges,
		FileTotalLines:             snapshot.FileTotals,
		ReplaceReadSet:             true,
		ReplaceReadRanges:          true,
		ReplaceFileTotalLines:      true,
		AcceptedEvidence:           snapshot.AcceptedEvidence,
		SourceInventoryObservation: snapshot.SourceInventory,
		ProgressDecision:           snapshot.ProgressDecision,
		HasProgressDecision:        true,
	}, o.busCtx.RepoRoot)
	logging.Info("[orchestrator] read run snapshot seed applied: run=%s nodes=%d attempts=%d artifacts=%d reads=%d accepted=%d",
		snapshot.RunID, len(snapshot.NodeStatuses), len(snapshot.NodeAttempts), len(snapshot.NodeArtifacts), len(snapshot.ReadSet), len(snapshot.AcceptedEvidence))
	o.readRunActiveSeed = activeState
	return nil
}

func (o *Orchestrator) validateReadRunSnapshotFingerprints(snapshot types.ReadRunSnapshot) error {
	if o == nil || o.busCtx == nil {
		return nil
	}
	if strings.TrimSpace(snapshot.RequestHash) != "" {
		currentRequest := currentReadRunSnapshotRequest(o.busCtx)
		currentHash := types.ReadRunRequestHash(currentRequest)
		if currentHash == "" || currentHash != strings.TrimSpace(snapshot.RequestHash) {
			return readRunSnapshotSeedError(readRunSnapshotSeedReasonRequestFingerprint,
				"snapshot request hash %q does not match current request hash %q", snapshot.RequestHash, currentHash)
		}
	}
	currentRepo := readRunCurrentRepoFingerprint(o.busCtx.RepoRoot)
	if !types.ReadRunRepoFingerprintsEqual(snapshot.RepoFingerprint, currentRepo) {
		return readRunSnapshotSeedError(readRunSnapshotSeedReasonRepoFingerprint,
			"snapshot repo fingerprint %+v does not match current %+v", snapshot.RepoFingerprint, currentRepo)
	}
	currentAttachments := types.ReadRunAttachmentFingerprintsFromBusContext(o.busCtx)
	for _, snapAttachment := range types.NormalizeReadRunAttachmentFingerprints(snapshot.Attachments) {
		if !snapAttachment.Present {
			continue
		}
		currentAttachment, ok := types.ReadRunAttachmentFingerprintByKind(currentAttachments, snapAttachment.Kind)
		if !ok || !types.ReadRunAttachmentFingerprintsEqual(snapAttachment, currentAttachment) {
			return readRunSnapshotSeedError(readRunSnapshotSeedReasonAttachmentFingerprint,
				"snapshot attachment fingerprint kind=%q does not match current attachment", snapAttachment.Kind)
		}
	}
	return nil
}

func currentReadRunSnapshotRequest(ctx *types.BusContext) string {
	if ctx == nil {
		return ""
	}
	if ctx.AnalysisIR != nil {
		if request := strings.TrimSpace(ctx.AnalysisIR.RequestModel.RawRequest); request != "" {
			return request
		}
	}
	if ctx.Mutable != nil {
		return strings.TrimSpace(ctx.Mutable.Objective())
	}
	return ""
}

func readRunSnapshotSeedError(reason string, format string, args ...any) error {
	return fmt.Errorf("read run snapshot seed %s: %s", reason, fmt.Sprintf(format, args...))
}

func readTaskGraphNodeSet(graph types.TaskGraph) map[string]struct{} {
	out := make(map[string]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		id := strings.TrimSpace(node.ID)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out
}

func readRunSnapshotReadSetMap(readSet []string) map[string]bool {
	out := make(map[string]bool, len(readSet))
	for _, file := range readSet {
		file = strings.TrimSpace(file)
		if file != "" {
			out[file] = true
		}
	}
	return out
}

func sameReadRunSnapshotRepoRoot(snapshotRoot, currentRoot string) bool {
	return canonicalReadRunSnapshotRepoRoot(snapshotRoot) != "" &&
		canonicalReadRunSnapshotRepoRoot(snapshotRoot) == canonicalReadRunSnapshotRepoRoot(currentRoot)
}

func canonicalReadRunSnapshotRepoRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if eval, err := filepath.EvalSymlinks(root); err == nil {
		root = eval
	}
	return filepath.Clean(root)
}
