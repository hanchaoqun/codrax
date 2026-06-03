package repl

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/operation"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestOperationPendingStoreRoundTripCommandPlan(t *testing.T) {
	store := NewOperationPendingStore(t.TempDir())
	plan := operation.CommandOperationPlan{
		ID:           "op-test",
		RequestText:  "run custom tool",
		Status:       operation.StatusReady,
		RiskLevel:    "medium",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      ".",
		CreatedAt:    time.Now().UTC(),
		Steps: []operation.CommandStep{{
			ID:           "s1",
			Title:        "custom",
			Program:      "corp-tool",
			AutoApproval: operation.StepAutoManual,
			RiskLevel:    "medium",
		}},
	}
	if err := store.SaveCommandPlan(plan); err != nil {
		t.Fatalf("SaveCommandPlan: %v", err)
	}
	snap, ok, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok || snap.Kind != operationPendingKindCommandPlan || snap.CommandPlan == nil {
		t.Fatalf("snapshot=%+v ok=%v", snap, ok)
	}
	if snap.CommandPlan.ID != "op-test" || len(snap.CommandPlan.Steps) != 1 {
		t.Fatalf("restored plan=%+v", snap.CommandPlan)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if _, ok, err := store.Load(); err != nil || ok {
		t.Fatalf("after clear ok=%v err=%v", ok, err)
	}
}

func TestREPLRestoresPendingOperationInBanner(t *testing.T) {
	pendingStore := NewOperationPendingStore(t.TempDir())
	plan := operation.CommandOperationPlan{
		ID:           "op-restore",
		Status:       operation.StatusReady,
		RiskLevel:    "medium",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      ".",
		CreatedAt:    time.Now().UTC(),
		Steps: []operation.CommandStep{{
			ID:           "s1",
			Title:        "probe",
			Program:      "corp-tool",
			AutoApproval: operation.StepAutoManual,
			RiskLevel:    "medium",
		}},
	}
	if err := pendingStore.SaveCommandPlan(plan); err != nil {
		t.Fatalf("SaveCommandPlan: %v", err)
	}
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = memStore.Close() })
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                &requestCapturingRunner{},
		Store:                 memStore,
		Render:                renderNothing,
		RepoRoot:              ".",
		Branch:                "main",
		In:                    strings.NewReader("/exit\n"),
		Out:                   out,
		Language:              "en",
		OperationPendingStore: pendingStore,
	})
	if r.pendingOperation == nil || r.pendingOperation.ID != "op-restore" {
		t.Fatalf("pending operation was not restored: %+v", r.pendingOperation)
	}
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if !strings.Contains(out.String(), "Pending operation: op-restore") {
		t.Fatalf("banner did not show pending operation:\n%s", out.String())
	}
}

func TestApproveUsesRestoredOperationBeforeWriteGate(t *testing.T) {
	pendingStore := NewOperationPendingStore(t.TempDir())
	plan := operation.CommandOperationPlan{
		ID:           "op-approve",
		RequestText:  "show go version",
		Status:       operation.StatusReady,
		RiskLevel:    "low",
		ApprovalMode: operation.ApprovalManual,
		WorkDir:      ".",
		CreatedAt:    time.Now().UTC(),
		Steps: []operation.CommandStep{{
			ID:           "s1",
			Title:        "show go version",
			Program:      "go",
			Args:         []string{"version"},
			AutoApproval: operation.StepAutoManual,
			RiskLevel:    "low",
		}},
	}
	if err := pendingStore.SaveCommandPlan(plan); err != nil {
		t.Fatalf("SaveCommandPlan: %v", err)
	}
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = memStore.Close() })
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                 &requestCapturingRunner{},
		Store:                  memStore,
		Render:                 renderNothing,
		RepoRoot:               ".",
		Branch:                 "main",
		In:                     strings.NewReader("/approve\n/exit\n"),
		Out:                    out,
		Language:               "en",
		OperationCommandPolicy: operation.DefaultCommandPolicy(),
		OperationPendingStore:  pendingStore,
		WriteEnabled:           false,
	})
	if err := r.Loop(); err != nil {
		t.Fatalf("Loop: %v", err)
	}
	printed := out.String()
	if strings.Contains(printed, "rejected: write_enabled") || strings.Contains(printed, "已被拒绝: write_enabled") {
		t.Fatalf("/approve fell through to write-mode gate:\n%s", printed)
	}
	if r.pendingOperation != nil {
		t.Fatalf("pending operation was not cleared: %+v", r.pendingOperation)
	}
	if _, err := os.Stat(pendingStore.Path()); !os.IsNotExist(err) {
		t.Fatalf("pending store was not cleared; stat err=%v", err)
	}
	if !strings.Contains(printed, "go version") {
		t.Fatalf("operation result missing:\n%s", printed)
	}
}
