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
		"Advanced: `/read-runs resume read-audit-1`",
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

func TestReadRunsResumeDispatchesTypedSeed(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	snapshot := &types.ReadRunSnapshot{
		SchemaVersion: types.ReadRunSnapshotSchemaVersion,
		RunID:         "read-audit-1",
		Request:       "resume this typed read run",
		RepoRoot:      "/tmp/snapshot-repo",
		TaskGraphHash: strings.Repeat("b", 64),
		TaskNodeCount: 2,
		NodeStatuses: map[string]types.NodeExecStatus{
			"explore": types.NodeExecDone,
		},
		ReadSet: []string{"internal/agent/subagent_runtime.go"},
	}
	if _, err := store.Save(snapshot); err != nil {
		t.Fatalf("Save: %v", err)
	}
	runner := &readRunResumeRunner{result: "resumed answer"}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:               runner,
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/current-repo",
		Branch:               "feature/resume",
		Render:               renderReadRunResumeResult,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})
	r.currentMode = types.ModeApply

	r.handleReadRunsCmd("/read-runs resume read-audit-1")

	if len(runner.requests) != 1 || runner.requests[0] != snapshot.Request {
		t.Fatalf("Run requests = %#v, want %q", runner.requests, snapshot.Request)
	}
	if len(runner.repos) != 1 || runner.repos[0] != snapshot.RepoRoot {
		t.Fatalf("Run repos = %#v, want %q", runner.repos, snapshot.RepoRoot)
	}
	if len(runner.branches) != 1 || runner.branches[0] != "feature/resume" {
		t.Fatalf("Run branches = %#v", runner.branches)
	}
	if len(runner.seedRunIDs) != 2 || runner.seedRunIDs[0] != "read-audit-1" || runner.seedRunIDs[1] != "" {
		t.Fatalf("seed run ids = %#v, want installed then cleared", runner.seedRunIDs)
	}
	if len(runner.modeCalls) != 2 || runner.modeCalls[0] != types.ModeRead || runner.modeCalls[1] != types.ModeApply {
		t.Fatalf("mode calls = %#v, want read then restore apply", runner.modeCalls)
	}
	if got := strings.Join(runner.transcriptRequests, "|"); got != snapshot.Request+"|" {
		t.Fatalf("transcript requests = %#v, want request then clear", runner.transcriptRequests)
	}
	if got := out.String(); !strings.Contains(got, "resumed answer") {
		t.Fatalf("resume output missing answer:\n%s", got)
	}
}

func TestReadRunsResumeRequiresStableIDAndCapability(t *testing.T) {
	store := NewReadRunSnapshotStore(t.TempDir())
	out := &bytes.Buffer{}
	runner := &readRunResumeRunner{}
	r := New(Config{
		Runner:               runner,
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/current-repo",
		Branch:               "main",
		Render:               renderReadRunResumeResult,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})

	r.handleReadRunsCmd("/read-runs resume")
	if len(runner.requests) != 0 {
		t.Fatalf("Run should not be called without id: %#v", runner.requests)
	}
	if got := out.String(); !strings.Contains(got, "/read-runs resume <run-id>") {
		t.Fatalf("resume usage output unexpected:\n%s", got)
	}

	out.Reset()
	r.handleReadRunsCmd("/read-runs resume missing-run")
	if len(runner.requests) != 0 {
		t.Fatalf("Run should not be called for missing snapshot: %#v", runner.requests)
	}
	if got := out.String(); !strings.Contains(got, `snapshot "missing-run" not found`) {
		t.Fatalf("missing snapshot output unexpected:\n%s", got)
	}

	out.Reset()
	rNoSeed := New(Config{
		Runner:               stubRunner{},
		In:                   strings.NewReader(""),
		Out:                  out,
		RepoRoot:             "/tmp/current-repo",
		Branch:               "main",
		Render:               renderReadRunResumeResult,
		ReadRunSnapshotStore: store,
		Language:             "en",
	})
	rNoSeed.handleReadRunsCmd("/read-runs resume missing-run")
	if got := out.String(); !strings.Contains(got, "does not support typed read snapshot seeds") {
		t.Fatalf("unsupported runner output unexpected:\n%s", got)
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

type readRunResumeRunner struct {
	requests           []string
	repos              []string
	branches           []string
	seedRunIDs         []string
	modeCalls          []types.PipelineMode
	transcriptRequests []string
	result             string
}

func (r *readRunResumeRunner) SetReadRunSnapshotSeed(snapshot *types.ReadRunSnapshot) {
	if snapshot == nil {
		r.seedRunIDs = append(r.seedRunIDs, "")
		return
	}
	r.seedRunIDs = append(r.seedRunIDs, snapshot.RunID)
}

func (r *readRunResumeRunner) SetMode(mode types.PipelineMode) {
	r.modeCalls = append(r.modeCalls, mode)
}

func (r *readRunResumeRunner) SetOutputTranscriptRequest(request string) {
	r.transcriptRequests = append(r.transcriptRequests, request)
}

func (r *readRunResumeRunner) Run(request, repoRoot, branch string) (*types.BusContext, error) {
	r.requests = append(r.requests, request)
	r.repos = append(r.repos, repoRoot)
	r.branches = append(r.branches, branch)
	mut := types.NewMutableState(request)
	mut.SetResult(r.result)
	return &types.BusContext{Mutable: mut}, nil
}

func renderReadRunResumeResult(ctx *types.BusContext) string {
	if ctx == nil || ctx.Mutable == nil {
		return ""
	}
	return ctx.Mutable.Result()
}
