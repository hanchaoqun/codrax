package operation

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOperationMemoryStoreRoundTripAndPrompt(t *testing.T) {
	store := NewMemoryStore(filepath.Join(t.TempDir(), "memory.jsonl"))
	snapshot := CapabilitySnapshot{RepoRoot: "/repo", OS: "linux", Arch: "amd64", Shell: "bash"}
	entry := MemoryEntry{
		Workspace:    "/repo",
		OS:           "linux",
		Arch:         "amd64",
		Shell:        "bash",
		Capability:   "computer_operation",
		Command:      "demo-tool --help",
		Outcome:      "executed",
		OutputKind:   "help_text",
		Summary:      "Usage: demo-tool --input FILE",
		PayloadRefs:  []string{"/tmp/help.txt"},
		Lessons:      []string{"This command completed successfully."},
		FailureClass: "",
	}
	if err := store.Append(entry); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := store.RecentMatches(snapshot, 3)
	if err != nil {
		t.Fatalf("RecentMatches: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("matches=%d want 1: %+v", len(got), got)
	}
	rendered := RenderMemoryForPrompt(got)
	for _, want := range []string{
		"## operation_memory",
		"Historical command-operation lessons",
		"demo-tool --help",
		"output_kind=",
		"payload_ref=/tmp/help.txt",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered memory missing %q:\n%s", want, rendered)
		}
	}
}

func TestOperationMemoryStoreSkipsExpiredAndMismatched(t *testing.T) {
	store := NewMemoryStore(filepath.Join(t.TempDir(), "memory.jsonl"))
	now := time.Now().UTC()
	if err := store.Append(
		MemoryEntry{
			CreatedAt: now.Add(-48 * time.Hour),
			ExpiresAt: now.Add(-24 * time.Hour),
			Workspace: "/repo",
			OS:        "linux",
			Arch:      "amd64",
			Command:   "old",
			Outcome:   "failed",
			Summary:   "expired",
		},
		MemoryEntry{
			Workspace: "/other",
			OS:        "darwin",
			Arch:      "arm64",
			Command:   "other",
			Outcome:   "executed",
			Summary:   "different workspace",
		},
	); err != nil {
		t.Fatalf("Append: %v", err)
	}
	got, err := store.RecentMatches(CapabilitySnapshot{RepoRoot: "/repo", OS: "linux", Arch: "amd64"}, 5)
	if err != nil {
		t.Fatalf("RecentMatches: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no usable entries, got %+v", got)
	}
}

func TestBuildMemoryEntriesSummarizesFailure(t *testing.T) {
	plan := CommandOperationPlan{
		ID:      "op",
		WorkDir: "/repo",
		Steps: []CommandStep{{
			ID:      "s1",
			Program: "missing-tool",
			Args:    []string{"--version"},
		}},
	}
	result := CommandOperationResult{
		PlanID: "op",
		Status: StatusFailed,
		StepResults: []CommandStepResult{{
			StepID:       "s1",
			Status:       StatusFailed,
			Error:        "executable file not found",
			FailureClass: "command_not_found",
		}},
	}
	entries := BuildMemoryEntries(plan, result, CapabilitySnapshot{RepoRoot: "/repo", OS: "linux", Arch: "amd64"})
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1", len(entries))
	}
	entry := entries[0]
	if entry.Command != "missing-tool --version" || entry.FailureClass != "command_not_found" {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.OutputKind != "error_output" {
		t.Fatalf("OutputKind=%q want error_output: %+v", entry.OutputKind, entry)
	}
	if len(entry.Lessons) == 0 || !strings.Contains(entry.Lessons[0], "not available") {
		t.Fatalf("missing command-not-found lesson: %+v", entry.Lessons)
	}
}

func TestBuildMemoryEntriesIncludesVerification(t *testing.T) {
	plan := CommandOperationPlan{
		ID:      "op",
		WorkDir: "/repo",
		Steps: []CommandStep{{
			ID:      "s1",
			Program: "mkdir",
			Args:    []string{"-p", "out"},
		}},
	}
	result := CommandOperationResult{
		PlanID: "op",
		Status: StatusExecuted,
		StepResults: []CommandStepResult{{
			StepID: "s1",
			Status: StatusExecuted,
			Verification: OperationVerificationResult{
				Status:  VerificationVerified,
				Kind:    "dir_exists",
				Path:    "/repo/out",
				Summary: "/repo/out exists as a directory",
			},
		}},
	}
	entries := BuildMemoryEntries(plan, result, CapabilitySnapshot{RepoRoot: "/repo", OS: "linux", Arch: "amd64"})
	if len(entries) != 1 {
		t.Fatalf("entries=%d want 1", len(entries))
	}
	entry := entries[0]
	if entry.VerifyStatus != VerificationVerified || !strings.Contains(entry.VerifySummary, "directory") {
		t.Fatalf("verification not preserved: %+v", entry)
	}
	rendered := RenderMemoryForPrompt(entries)
	if !strings.Contains(rendered, "verify=verified") {
		t.Fatalf("rendered memory missing verification:\n%s", rendered)
	}
}
