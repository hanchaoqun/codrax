package repl

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/operation"
)

const (
	operationPendingStoreVersion = 1

	operationPendingKindCommandPlan          = "command_plan"
	operationPendingKindCommandClarification = "command_clarification"
	operationPendingKindProviderOperation    = "provider_operation"
)

// OperationPendingStore persists the single unresolved operation-lane decision
// for a REPL workspace. It is intentionally separate from PlanStore: ChangePlan
// approval changes source bytes in a worktree, while operation approval controls
// computer-operation/provider execution and must not be mixed with source-plan
// state.
type OperationPendingStore struct {
	mu   sync.Mutex
	path string
}

type operationPendingSnapshot struct {
	Version              int                             `json:"version"`
	Kind                 string                          `json:"kind"`
	SavedAt              time.Time                       `json:"saved_at"`
	CommandPlan          *operation.CommandOperationPlan `json:"command_plan,omitempty"`
	CommandClarification *pendingCommandClarification    `json:"command_clarification,omitempty"`
	ProviderOperation    *pendingProviderOperation       `json:"provider_operation,omitempty"`
	ProviderWorkflow     *operation.WorkflowInstance     `json:"provider_workflow,omitempty"`
}

// NewOperationPendingStore constructs a store rooted at dir. The directory is
// created lazily only when a pending operation actually needs persistence.
func NewOperationPendingStore(dir string) *OperationPendingStore {
	if dir == "" {
		dir = ".codrax/operations"
	}
	return &OperationPendingStore{path: filepath.Join(dir, "pending.json")}
}

func (s *OperationPendingStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *OperationPendingStore) SaveCommandPlan(plan operation.CommandOperationPlan) error {
	return s.save(operationPendingSnapshot{
		Version:     operationPendingStoreVersion,
		Kind:        operationPendingKindCommandPlan,
		SavedAt:     time.Now().UTC(),
		CommandPlan: &plan,
	})
}

func (s *OperationPendingStore) SaveCommandClarification(pending pendingCommandClarification) error {
	return s.save(operationPendingSnapshot{
		Version:              operationPendingStoreVersion,
		Kind:                 operationPendingKindCommandClarification,
		SavedAt:              time.Now().UTC(),
		CommandClarification: &pending,
	})
}

func (s *OperationPendingStore) SaveProviderOperation(pending pendingProviderOperation, workflow *operation.WorkflowInstance) error {
	snap := operationPendingSnapshot{
		Version:           operationPendingStoreVersion,
		Kind:              operationPendingKindProviderOperation,
		SavedAt:           time.Now().UTC(),
		ProviderOperation: &pending,
	}
	if workflow != nil {
		cp := *workflow
		snap.ProviderWorkflow = &cp
	}
	return s.save(snap)
}

func (s *OperationPendingStore) Load() (operationPendingSnapshot, bool, error) {
	if s == nil {
		return operationPendingSnapshot{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return operationPendingSnapshot{}, false, nil
	}
	if err != nil {
		return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: read %s: %w", s.path, err)
	}
	var snap operationPendingSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: unmarshal %s: %w", s.path, err)
	}
	if snap.Version != operationPendingStoreVersion {
		return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: unsupported version %d", snap.Version)
	}
	switch snap.Kind {
	case operationPendingKindCommandPlan:
		if snap.CommandPlan == nil {
			return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: command_plan missing payload")
		}
	case operationPendingKindCommandClarification:
		if snap.CommandClarification == nil {
			return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: command_clarification missing payload")
		}
	case operationPendingKindProviderOperation:
		if snap.ProviderOperation == nil {
			return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: provider_operation missing payload")
		}
	default:
		return operationPendingSnapshot{}, false, fmt.Errorf("OperationPendingStore.Load: unknown kind %q", snap.Kind)
	}
	return snap, true, nil
}

func (s *OperationPendingStore) Clear() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("OperationPendingStore.Clear: remove %s: %w", s.path, err)
	}
	return nil
}

func (s *OperationPendingStore) save(snap operationPendingSnapshot) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("OperationPendingStore.Save: mkdir %s: %w", filepath.Dir(s.path), err)
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("OperationPendingStore.Save: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("OperationPendingStore.Save: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("OperationPendingStore.Save: rename %s -> %s: %w", tmp, s.path, err)
	}
	return nil
}
