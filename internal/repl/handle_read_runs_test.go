package repl

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReadRunsListShowClear(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	snapshot := &types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-audit-1",
		CreatedAt:     time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
		Request:       "which agent can call subagents",
		RepoRoot:      "/tmp/repo",
		TaskGraphHash: strings.Repeat("a", 64),
		TaskNodeCount: 3,
		NodeStatuses: map[string]types.NodeExecStatus{
			"explore": types.NodeExecDone,
			"final":   types.NodeExecPending,
		},
		ReadSet: []string{"internal/agent/subagent_runtime.go", "internal/agent/subagent.go"},
		AcceptedEvidence: []types.AcceptedEvidenceRef{{
			ID:        "ev-subagent",
			Source:    "internal/agent/subagent_runtime.go",
			LineStart: 218,
		}},
		ProgressDecision: types.ProgressDecision{
			ShouldReplan: true,
			ReasonCode:   types.ProgressReasonContinue,
		},
		SourceInventory: types.SourceInventoryObservation{
			Active:   true,
			Complete: true,
			SourceClasses: []types.SourceInventorySourceClassCount{{
				Role:     types.SourcePathRoleProduction,
				Count:    2,
				Complete: true,
			}},
		},
	}
	if _, err := store.Save(snapshot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:               stubRunner{},
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/repo",
		Branch:               "main",
		Render:               renderNothing,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})

	r.handleReadRunsCmd("/read-runs list")
	got := out.String()
	for _, want := range []string{"Read run snapshots", "`read-audit-1`", "reads=2", "evidence=1", "graph=aaaaaaaaaaaa"} {
		if !strings.Contains(got, want) {
			t.Fatalf("list output missing %q:\n%s", want, got)
		}
	}

	out.Reset()
	r.handleReadRunsCmd("/read-runs show read-audit-1")
	got = out.String()
	for _, want := range []string{
		"Read run `read-audit-1`",
		"Schema: `1`",
		"Repo: `/tmp/repo`",
		"Task graph: hash=`aaaaaaaaaaaa` nodes=3",
		"Node statuses: pending=1 done=1",
		"Read files: 2",
		"Accepted evidence refs: 1",
		"Progress: reason=`progress_delta_continue`",
		"should_replan=t",
		"Source inventory: complete=true",
		"classes: production=2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("show output missing %q:\n%s", want, got)
		}
	}

	out.Reset()
	r.handleReadRunsCmd("/read-runs clear read-audit-1")
	got = out.String()
	if !strings.Contains(got, "read-runs cleared: read-audit-1") {
		t.Fatalf("clear output unexpected:\n%s", got)
	}
	loaded, err := store.Load("read-audit-1")
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if loaded != nil {
		t.Fatalf("snapshot should be cleared, got %+v", loaded)
	}
}

func TestReadRunsCommandDisabledWithoutStore(t *testing.T) {
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:   stubRunner{},
		In:       strings.NewReader(""),
		Out:      out,
		RepoRoot: "/tmp/repo",
		Branch:   "main",
		Render:   renderNothing,
		Language: "en",
	})
	r.handleReadRunsCmd("/read-runs list")
	if got := out.String(); !strings.Contains(got, "/read-runs disabled") {
		t.Fatalf("disabled output unexpected:\n%s", got)
	}
}
