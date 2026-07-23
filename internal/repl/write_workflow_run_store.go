package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

// WriteWorkflowRunStore persists outer write-controller runs under
// <planDir>/workflows/. It mirrors PlanGroupStore's atomic-write shape while
// keeping controller metadata separate from ChangePlan files.
type WriteWorkflowRunStore struct {
	mu          sync.Mutex
	workflowDir string
}

func NewWriteWorkflowRunStore(planDir string) *WriteWorkflowRunStore {
	if planDir == "" {
		planDir = ".codrax/plans"
	}
	return &WriteWorkflowRunStore{workflowDir: filepath.Join(planDir, "workflows")}
}

func (s *WriteWorkflowRunStore) WorkflowDir() string {
	if s == nil {
		return ""
	}
	return s.workflowDir
}

func (s *WriteWorkflowRunStore) Save(run *types.WriteWorkflowRun) (string, error) {
	if s == nil {
		return "", fmt.Errorf("WriteWorkflowRunStore.Save: nil store")
	}
	if run == nil {
		return "", fmt.Errorf("WriteWorkflowRunStore.Save: nil run")
	}
	normalized := types.NormalizeWriteWorkflowRun(*run)
	if strings.TrimSpace(normalized.RunID) == "" {
		return "", fmt.Errorf("WriteWorkflowRunStore.Save: run.RunID empty")
	}
	if !validStoreIDPattern.MatchString(normalized.RunID) {
		return "", fmt.Errorf("WriteWorkflowRunStore.Save: invalid run id %q", normalized.RunID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.workflowDir, 0o755); err != nil {
		return "", fmt.Errorf("WriteWorkflowRunStore.Save: mkdir %s: %w", s.workflowDir, err)
	}
	if err := s.saveContextPacks(&normalized); err != nil {
		return "", err
	}
	path := filepath.Join(s.workflowDir, normalized.RunID+".json")
	if err := types.WriteWorkflowRunToFile(&normalized, path); err != nil {
		return "", err
	}
	if err := s.saveLoopEvents(&normalized); err != nil {
		return "", err
	}
	return path, nil
}

func (s *WriteWorkflowRunStore) saveLoopEvents(run *types.WriteWorkflowRun) error {
	if s == nil || run == nil {
		return nil
	}
	events := loopkernel.EventsFromWriteWorkflowRun(*run)
	if len(events) == 0 {
		return nil
	}
	path := filepath.Join(s.workflowDir, "events", run.RunID+".json")
	if err := loopkernel.WriteLoopEventsToFile(events, path); err != nil {
		return fmt.Errorf("WriteWorkflowRunStore.Save loop events %s: %w", run.RunID, err)
	}
	return nil
}

func (s *WriteWorkflowRunStore) saveContextPacks(run *types.WriteWorkflowRun) error {
	if s == nil || run == nil || len(run.ContextPacks) == 0 {
		return nil
	}
	contextDir := filepath.Join(s.workflowDir, "contexts", run.RunID)
	for i := range run.ContextPacks {
		pack := types.NormalizeWriteContextPack(run.ContextPacks[i])
		if len(pack.Items) == 0 {
			continue
		}
		packID := workflowContextPackArtifactID(run.RunID, pack, i)
		pack.PackID = packID
		run.ContextPacks[i] = pack
		attachContextPackRefToRunBatch(run, pack.BatchID, packID)
		if err := types.WriteContextPackToFile(&pack, filepath.Join(contextDir, packID+".json")); err != nil {
			return fmt.Errorf("WriteWorkflowRunStore.Save context pack %s: %w", packID, err)
		}
	}
	return nil
}

func workflowContextPackArtifactID(runID string, pack types.WriteContextPack, idx int) string {
	parts := []string{runID, pack.BatchID, pack.PackID, pack.SourceStage}
	var b strings.Builder
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('-')
		}
		b.WriteString(part)
	}
	if b.Len() == 0 {
		fmt.Fprintf(&b, "%s-context-%d", runID, idx+1)
	}
	out := make([]rune, 0, b.Len())
	lastDash := false
	for _, r := range strings.ToLower(b.String()) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash {
				out = append(out, '-')
				lastDash = true
			}
		default:
			if !lastDash {
				out = append(out, '-')
				lastDash = true
			}
		}
	}
	id := strings.Trim(string(out), "-")
	if id == "" {
		id = fmt.Sprintf("%s-context-%d", runID, idx+1)
	}
	if len(id) > 120 {
		id = id[:120]
		id = strings.TrimRight(id, "-")
	}
	return id
}

func attachContextPackRefToRunBatch(run *types.WriteWorkflowRun, batchID, packID string) {
	batchID = strings.TrimSpace(batchID)
	packID = strings.TrimSpace(packID)
	if run == nil || batchID == "" || packID == "" {
		return
	}
	for i := range run.Batches {
		if run.Batches[i].ID != batchID {
			continue
		}
		for _, existing := range run.Batches[i].ContextPackIDs {
			if existing == packID {
				return
			}
		}
		run.Batches[i].ContextPackIDs = append(run.Batches[i].ContextPackIDs, packID)
		return
	}
}

func (s *WriteWorkflowRunStore) Load(id string) (*types.WriteWorkflowRun, error) {
	if s == nil {
		return nil, fmt.Errorf("WriteWorkflowRunStore.Load: nil store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("WriteWorkflowRunStore.Load: empty id")
	}
	if !validStoreIDPattern.MatchString(id) {
		return nil, fmt.Errorf("WriteWorkflowRunStore.Load: invalid run id %q (must match [a-zA-Z0-9_-]+)", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return types.LoadWriteWorkflowRunFromFile(filepath.Join(s.workflowDir, id+".json"))
}

func (s *WriteWorkflowRunStore) Clear(id string) error {
	if s == nil {
		return fmt.Errorf("WriteWorkflowRunStore.Clear: nil store")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("WriteWorkflowRunStore.Clear: empty id")
	}
	if !validStoreIDPattern.MatchString(id) {
		return fmt.Errorf("WriteWorkflowRunStore.Clear: invalid run id %q", id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.workflowDir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("WriteWorkflowRunStore.Clear: remove %s: %w", path, err)
	}
	contextDir := filepath.Join(s.workflowDir, "contexts", id)
	if err := os.RemoveAll(contextDir); err != nil {
		return fmt.Errorf("WriteWorkflowRunStore.Clear: remove %s: %w", contextDir, err)
	}
	eventPath := filepath.Join(s.workflowDir, "events", id+".json")
	if err := os.Remove(eventPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("WriteWorkflowRunStore.Clear: remove %s: %w", eventPath, err)
	}
	return nil
}

type WorkflowRunInfo struct {
	ID                string
	Path              string
	Status            string
	ActiveBatchID     string
	ActiveBatchStatus string
	ActiveSliceID     string
	ActiveSliceStatus string
	CompletedSlices   int
	TotalSlices       int
	Batches           int
	ContextPacks      int
	NextState         string
	NextAction        string
	RequiresUser      bool
	ModTime           int64
}

func (s *WriteWorkflowRunStore) List() ([]WorkflowRunInfo, error) {
	if s == nil {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.workflowDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("WriteWorkflowRunStore.List: read %s: %w", s.workflowDir, err)
	}
	out := make([]WorkflowRunInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		id := strings.TrimSuffix(name, ".json")
		if !validStoreIDPattern.MatchString(id) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		var run types.WriteWorkflowRun
		if data, rerr := os.ReadFile(filepath.Join(s.workflowDir, name)); rerr == nil {
			_ = json.Unmarshal(data, &run)
		}
		run = types.NormalizeWriteWorkflowRun(run)
		if run.RunID == "" {
			run.RunID = id
		}
		next := types.DeriveWriteWorkflowNextActionView(run)
		status := string(run.Status)
		activeBatchID := run.ActiveBatchID
		activeBatchStatus := string(next.BatchStatus)
		if status == "" || activeBatchID == "" || len(run.Batches) == 0 {
			var probe struct {
				Status        string     `json:"status"`
				ActiveBatchID string     `json:"active_batch_id"`
				Batches       []struct{} `json:"batches"`
				ContextPacks  []struct{} `json:"context_packs"`
			}
			if data, rerr := os.ReadFile(filepath.Join(s.workflowDir, name)); rerr == nil {
				_ = json.Unmarshal(data, &probe)
			}
			if status == "" {
				status = probe.Status
			}
			if activeBatchID == "" {
				activeBatchID = probe.ActiveBatchID
			}
			if len(run.Batches) == 0 && len(probe.Batches) > 0 {
				run.Batches = make([]types.WriteWorkflowBatch, len(probe.Batches))
			}
			if len(run.ContextPacks) == 0 && len(probe.ContextPacks) > 0 {
				run.ContextPacks = make([]types.WriteContextPack, len(probe.ContextPacks))
			}
		}
		out = append(out, WorkflowRunInfo{
			ID:                id,
			Path:              filepath.Join(s.workflowDir, name),
			Status:            status,
			ActiveBatchID:     activeBatchID,
			ActiveBatchStatus: activeBatchStatus,
			ActiveSliceID:     next.ActiveSliceID,
			ActiveSliceStatus: string(next.ActiveSliceStatus),
			CompletedSlices:   next.CompletedSlices,
			TotalSlices:       next.TotalSlices,
			Batches:           len(run.Batches),
			ContextPacks:      len(run.ContextPacks),
			NextState:         string(next.State),
			NextAction:        string(next.PrimaryAction),
			RequiresUser:      next.RequiresUser,
			ModTime:           info.ModTime().UnixNano(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime > out[j].ModTime
	})
	return out, nil
}

func (s *WriteWorkflowRunStore) FindActiveRun() (*types.WriteWorkflowRun, error) {
	infos, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if info.Status == string(types.WriteWorkflowRunComplete) || info.Status == string(types.WriteWorkflowRunBlocked) {
			continue
		}
		run, err := s.Load(info.ID)
		if err == nil && run != nil {
			return run, nil
		}
	}
	return nil, nil
}

// FindActiveRunMatching is the WFID-1 identity-aware finder used by the write
// controller's auto-resume lane. It walks the same ModTime-ordered candidate
// list as FindActiveRun but returns a run only when the single-point identity
// gate (types.MatchWriteWorkflowRepoIdentity) matches the current context, or
// when the run carries a one-shot explicit-resume authorization stamped by
// /workflow resume that is valid for the current context (root-bound: a token
// minted in another repo context never authorizes here — the run falls
// through to the identity gate and surfaces as a typed skip instead).
// Mismatched active runs come back as typed skips so the caller can fail
// closed with a message naming them. FindActiveRun keeps its legacy semantics
// for the explicit REPL surfaces (/workflow show|resume|clear and plan
// binding), which act on "the saved active run" by user request rather than
// auto-resuming it.
func (s *WriteWorkflowRunStore) FindActiveRunMatching(current types.WriteWorkflowRepoIdentity) (*types.WriteWorkflowRun, []types.WriteWorkflowIdentitySkip, error) {
	infos, err := s.List()
	if err != nil {
		return nil, nil, err
	}
	var skips []types.WriteWorkflowIdentitySkip
	for _, info := range infos {
		if info.Status == string(types.WriteWorkflowRunComplete) || info.Status == string(types.WriteWorkflowRunBlocked) {
			continue
		}
		run, err := s.Load(info.ID)
		if err != nil || run == nil {
			continue
		}
		if types.WriteWorkflowRunExplicitResumeAuthorizationValid(run, current.CanonicalRepoRoot) {
			return run, skips, nil
		}
		decision := types.MatchWriteWorkflowRepoIdentity(run.Identity, current)
		if decision.Match {
			return run, skips, nil
		}
		skips = append(skips, types.WriteWorkflowIdentitySkip{
			RunID:      run.RunID,
			Goal:       run.Goal,
			ReasonCode: decision.ReasonCode,
			Detail:     decision.Detail,
		})
	}
	return nil, skips, nil
}

// ClearResumeAuthorizationsExcept implements the orchestrator's
// WriteWorkflowResumeAuthorizationSweeper capability: after any successful
// write-turn loadOrSeed, residual one-shot explicit-resume tokens on every
// run except the one consumed this turn are cleared and persisted, making
// "one-shot" literal — a stamped token survives at most until the next write
// turn, whichever run and whichever lane (identity match, adoption, fresh
// seed, --plan-file import) that turn takes. Returns the number of runs whose
// token was cleared.
func (s *WriteWorkflowRunStore) ClearResumeAuthorizationsExcept(exceptRunID string) (int, error) {
	if s == nil {
		return 0, nil
	}
	exceptRunID = strings.TrimSpace(exceptRunID)
	infos, err := s.List()
	if err != nil {
		return 0, err
	}
	cleared := 0
	for _, info := range infos {
		if info.ID == exceptRunID {
			continue
		}
		run, err := s.Load(info.ID)
		if err != nil || run == nil {
			continue
		}
		if run.ResumeAuthorization == "" && run.ResumeAuthorizedRepoRoot == "" && run.ResumeAuthorizedAt.IsZero() {
			continue
		}
		run.ResumeAuthorization = ""
		run.ResumeAuthorizedRepoRoot = ""
		run.ResumeAuthorizedAt = time.Time{}
		if _, err := s.Save(run); err != nil {
			return cleared, err
		}
		cleared++
	}
	return cleared, nil
}
