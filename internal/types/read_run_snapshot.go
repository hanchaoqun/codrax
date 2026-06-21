package types

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ReadRunSnapshotSchemaVersion = 3

// ReadRunSnapshot is the typed audit/resume substrate for read-mode execution.
// It records durable structured artifacts only; no scheduler gate may consume
// rendered answer prose, model rationale, or prompt text from this surface.
type ReadRunSnapshot struct {
	SchemaVersion    int                        `json:"schema_version"`
	RunID            string                     `json:"run_id"`
	CreatedAt        time.Time                  `json:"created_at,omitempty"`
	Request          string                     `json:"request,omitempty"`
	RepoRoot         string                     `json:"repo_root,omitempty"`
	TaskGraphHash    string                     `json:"task_graph_hash,omitempty"`
	TaskNodeCount    int                        `json:"task_node_count,omitempty"`
	NodeStatuses     map[string]NodeExecStatus  `json:"node_statuses,omitempty"`
	NodeAttempts     map[string]int             `json:"node_attempts,omitempty"`
	ReadSet          []string                   `json:"read_set,omitempty"`
	ReadRanges       map[string][]LineRange     `json:"read_ranges,omitempty"`
	FileTotals       map[string]int             `json:"file_totals,omitempty"`
	AcceptedEvidence []AcceptedEvidenceRef      `json:"accepted_evidence,omitempty"`
	NodeArtifacts    []NodeArtifactRecord       `json:"node_artifacts,omitempty"`
	SourceInventory  SourceInventoryObservation `json:"source_inventory,omitempty"`
	ProgressDecision ProgressDecision           `json:"progress_decision,omitempty"`
}

func ReadRunSnapshotFromBusContext(ctx *BusContext, runID string) ReadRunSnapshot {
	snapshot := ReadRunSnapshot{
		SchemaVersion: ReadRunSnapshotSchemaVersion,
		RunID:         strings.TrimSpace(runID),
		CreatedAt:     time.Now().UTC(),
	}
	if ctx == nil {
		return NormalizeReadRunSnapshot(snapshot)
	}
	snapshot.RepoRoot = strings.TrimSpace(ctx.RepoRoot)
	if ctx.AnalysisIR != nil {
		snapshot.TaskGraphHash = ReadTaskGraphHash(ctx.AnalysisIR.TaskGraph)
		snapshot.TaskNodeCount = len(ctx.AnalysisIR.TaskGraph.Nodes)
		if snapshot.Request == "" {
			snapshot.Request = strings.TrimSpace(ctx.AnalysisIR.RequestModel.RawRequest)
		}
	}
	if ctx.Mutable == nil {
		return NormalizeReadRunSnapshot(snapshot)
	}
	if snapshot.RepoRoot == "" {
		snapshot.RepoRoot = strings.TrimSpace(ctx.Mutable.RepoRoot())
	}
	if snapshot.Request == "" {
		snapshot.Request = strings.TrimSpace(ctx.Mutable.Objective())
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure != nil {
		snapshot.NodeStatuses = closure.NodeExecStatuses()
		snapshot.NodeAttempts = closure.NodeExecAttempts()
		snapshot.ReadSet = readRunSortedReadSet(closure.ReadSet())
		snapshot.ReadRanges = closure.ReadRangesSnapshot()
		snapshot.FileTotals = closure.FileTotalLinesSnapshot()
		snapshot.AcceptedEvidence = closure.AcceptedEvidenceRefs()
		snapshot.NodeArtifacts = closure.NodeArtifactRecords()
		snapshot.ProgressDecision = closure.LatestProgressDecision()
	}
	snapshot.SourceInventory = SourceInventoryObservationFromMutable(ctx.Mutable)
	return NormalizeReadRunSnapshot(snapshot)
}

func NormalizeReadRunSnapshot(snapshot ReadRunSnapshot) ReadRunSnapshot {
	if snapshot.SchemaVersion == 0 {
		snapshot.SchemaVersion = ReadRunSnapshotSchemaVersion
	}
	snapshot.RunID = strings.TrimSpace(snapshot.RunID)
	snapshot.Request = strings.TrimSpace(snapshot.Request)
	snapshot.RepoRoot = strings.TrimSpace(snapshot.RepoRoot)
	snapshot.TaskGraphHash = strings.TrimSpace(snapshot.TaskGraphHash)
	snapshot.NodeStatuses = cloneNodeExecStatusMap(snapshot.NodeStatuses)
	snapshot.NodeAttempts = cloneNodeExecAttemptMap(snapshot.NodeAttempts)
	snapshot.ReadSet = compactReadRunStrings(snapshot.ReadSet...)
	snapshot.ReadRanges = cloneLineRangeMap(snapshot.ReadRanges)
	snapshot.FileTotals = cloneIntMap(snapshot.FileTotals)
	snapshot.AcceptedEvidence = cloneAcceptedEvidenceRefs(snapshot.AcceptedEvidence)
	snapshot.NodeArtifacts = NormalizeNodeArtifactRecords(snapshot.NodeArtifacts)
	snapshot.SourceInventory = CloneSourceInventoryObservation(snapshot.SourceInventory)
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	} else {
		snapshot.CreatedAt = snapshot.CreatedAt.UTC()
	}
	return snapshot
}

func WriteReadRunSnapshotToFile(snapshot *ReadRunSnapshot, path string) error {
	if snapshot == nil {
		return fmt.Errorf("WriteReadRunSnapshotToFile: nil snapshot")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("WriteReadRunSnapshotToFile: empty path")
	}
	normalized := NormalizeReadRunSnapshot(*snapshot)
	if normalized.RunID == "" {
		return fmt.Errorf("WriteReadRunSnapshotToFile: run_id empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("WriteReadRunSnapshotToFile: mkdir %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return fmt.Errorf("WriteReadRunSnapshotToFile: marshal: %w", err)
	}
	if err := AtomicWriteFileSync(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("WriteReadRunSnapshotToFile: write %s: %w", path, err)
	}
	return nil
}

func LoadReadRunSnapshotFromFile(path string) (*ReadRunSnapshot, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("LoadReadRunSnapshotFromFile: read %s: %w", path, err)
	}
	var snapshot ReadRunSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("LoadReadRunSnapshotFromFile: parse %s: %w", path, err)
	}
	normalized := NormalizeReadRunSnapshot(snapshot)
	if normalized.RunID == "" {
		return nil, fmt.Errorf("LoadReadRunSnapshotFromFile: %s has empty run_id", path)
	}
	return &normalized, nil
}

func ReadTaskGraphHash(graph TaskGraph) string {
	data, err := json.Marshal(graph)
	if err != nil || len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readRunSortedReadSet(files map[string]bool) []string {
	out := make([]string, 0, len(files))
	for file, ok := range files {
		file = strings.TrimSpace(file)
		if ok && file != "" {
			out = append(out, file)
		}
	}
	sort.Strings(out)
	return out
}

func compactReadRunStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
