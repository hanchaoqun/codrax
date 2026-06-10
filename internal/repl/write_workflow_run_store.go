package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	path := filepath.Join(s.workflowDir, normalized.RunID+".json")
	if err := types.WriteWorkflowRunToFile(&normalized, path); err != nil {
		return "", err
	}
	return path, nil
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
	return nil
}

type WorkflowRunInfo struct {
	ID            string
	Path          string
	Status        string
	ActiveBatchID string
	Batches       int
	ContextPacks  int
	ModTime       int64
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
		var probe struct {
			Status        string     `json:"status"`
			ActiveBatchID string     `json:"active_batch_id"`
			Batches       []struct{} `json:"batches"`
			ContextPacks  []struct{} `json:"context_packs"`
		}
		if data, rerr := os.ReadFile(filepath.Join(s.workflowDir, name)); rerr == nil {
			_ = json.Unmarshal(data, &probe)
		}
		out = append(out, WorkflowRunInfo{
			ID:            id,
			Path:          filepath.Join(s.workflowDir, name),
			Status:        probe.Status,
			ActiveBatchID: probe.ActiveBatchID,
			Batches:       len(probe.Batches),
			ContextPacks:  len(probe.ContextPacks),
			ModTime:       info.ModTime().UnixNano(),
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
